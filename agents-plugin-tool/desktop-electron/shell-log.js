'use strict';
const fs = require('node:fs');
const path = require('node:path');

// Console messages can themselves contain URLs; don't persist their credentials.
function clean(value) {
  return String(value).replace(/\b(?:https?|file):\/\/[^\s"'<>]+/gi, raw => {
    try {
      const url = new URL(raw);
      url.username = ''; url.password = ''; url.search = ''; url.hash = '';
      return url.href;
    } catch { return '<invalid URL>'; }
  }).replace(/\b(cookie|authorization)(["']?\s*[:=]\s*)[^\r\n]+/gi, '$1$2[redacted]')
    .replace(/\b(bearer\s+)[^\s,;]+/gi, '$1[redacted]')
    .replace(/\b(login|token|password)(["']?\s*[:=]\s*)(?:"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|[^\s,;}]+)/gi, '$1$2[redacted]');
}

function createShellLog(userData) {
  const file = path.join(userData, 'logs', 'shell.log');
  try {
    if (fs.statSync(file).size > 1024 * 1024) fs.writeFileSync(file, '');
  } catch (error) {
    if (error.code !== 'ENOENT') console.error('Could not check shell log:', error.message);
  }
  return (event, fields = {}) => {
    const entry = { timestamp: new Date().toISOString(), event };
    for (const [key, value] of Object.entries(fields)) {
      if (value !== undefined) entry[key] = typeof value === 'string' ? clean(value) : value;
    }
    try {
      fs.mkdirSync(path.dirname(file), { recursive: true });
      fs.appendFileSync(file, `${JSON.stringify(entry)}\n`, { mode: 0o600 });
    } catch (error) { console.error('Could not write shell log:', error.message); }
  };
}
module.exports = { createShellLog };
