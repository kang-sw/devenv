'use strict';
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');

const MAX_JSON = 64 * 1024;
function cacheRoot(env = process.env, cwd = process.cwd()) {
  let root = env.WS_CACHE_HOME;
  if (!root) {
    if (!env.HOME) throw new Error('Home directory is unavailable');
    root = path.join(env.HOME, '.cache/ws@kang-sw-devenv');
  }
  root = path.resolve(cwd, root);
  try { return fs.realpathSync(root); } catch { return root; }
}
function readEndpoint(root) {
  let fd;
  try { fd = fs.openSync(path.join(root, 'dashboard', 'endpoint.json'), 'r'); }
  catch { throw new Error('No readable dashboard endpoint; start the dashboard normally'); }
  let raw;
  try {
    const buffer = Buffer.alloc(MAX_JSON + 1);
    let size = 0, n;
    while (size < buffer.length && (n = fs.readSync(fd, buffer, size, buffer.length - size, null))) size += n;
    if (size > MAX_JSON) throw new Error('Dashboard endpoint is too large');
    raw = buffer.subarray(0, size).toString('utf8');
  } finally { fs.closeSync(fd); }
  let value;
  try { value = JSON.parse(raw); } catch { throw new Error('Invalid dashboard endpoint'); }
  if (!value || !Number.isInteger(value.port) || value.port < 1 || value.port > 65535 ||
      !Number.isInteger(value.pid) || value.pid < 1 || value.pid > 0xffffffff) {
    throw new Error('Invalid dashboard endpoint port or PID');
  }
  // Return only identity, never credentials, parse errors, or endpoint bytes.
  return { port: value.port, pid: value.pid };
}
function checkHealth(ep, timeout = 2000) {
  return new Promise((resolve, reject) => {
    const fail = message => { request.destroy(); reject(new Error(message)); };
    // node:http is direct (no environment proxy) and never follows redirects.
    const request = http.get({ hostname: '127.0.0.1', port: ep.port, path: '/health', agent: false }, response => {
      if (response.statusCode !== 200) return fail('Dashboard health check did not return HTTP 200');
      let size = 0;
      const chunks = [];
      response.on('data', chunk => {
        size += chunk.length;
        if (size > MAX_JSON) return fail('Invalid dashboard health response');
        chunks.push(chunk);
      });
      response.on('error', () => fail('Invalid dashboard health response'));
      response.on('end', () => {
        let health;
        try { health = JSON.parse(Buffer.concat(chunks).toString('utf8')); }
        catch { return fail('Invalid dashboard health response'); }
        if (!health || health.service !== 'ws-dashboard' || health.pid !== ep.pid) {
          return fail('Health response is not from the recorded dashboard daemon');
        }
        resolve(`http://127.0.0.1:${ep.port}/`);
      });
    });
    const timer = setTimeout(() => fail('Dashboard health check failed; start the dashboard normally'), timeout);
    request.on('close', () => clearTimeout(timer));
    request.on('error', () => reject(new Error('Dashboard health check failed; start the dashboard normally')));
  });
}
async function discover(root = cacheRoot()) {
  const ep = readEndpoint(root);
  const url = await checkHealth(ep);
  const after = readEndpoint(root);
  if (ep.port !== after.port || ep.pid !== after.pid) throw new Error('Dashboard endpoint changed; retry discovery');
  return url;
}
module.exports = { cacheRoot, readEndpoint, checkHealth, discover, MAX_JSON };
