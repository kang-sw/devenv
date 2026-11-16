'use strict';
// A prepared package is an input, never a recipe for a build or download.
const fs = require('node:fs');
const path = require('node:path');
const { validateDaemonRoot } = require('./daemon-selection');

function files(root) {
  const result = [];
  function walk(dir, relative = '') {
    if (!fs.lstatSync(dir).isDirectory()) throw new Error(`Daemon package contains a link or non-directory: ${dir}`);
    for (const name of fs.readdirSync(dir)) {
      const rel = path.join(relative, name), file = path.join(root, rel), stat = fs.lstatSync(file);
      if (stat.isDirectory()) walk(file, rel);
      else if (stat.isFile()) result.push(rel);
      else throw new Error(`Daemon package contains a link or special file: ${file}`);
    }
  }
  walk(root);
  return result.sort();
}
function copyPackage(root, destination, source, platform) {
  validateDaemonRoot(root, source, platform);
  const names = files(root); // Refuse links even in non-manifest package files.
  if (!fs.existsSync(destination)) fs.mkdirSync(destination, { mode: 0o700 });
  else if (!fs.lstatSync(destination).isDirectory() || fs.readdirSync(destination).length) {
    throw new Error('Daemon stage destination is not an empty directory');
  }
  for (const name of names) {
    const from = path.join(root, name), to = path.join(destination, name);
    fs.mkdirSync(path.dirname(to), { recursive: true });
    fs.copyFileSync(from, to);
    fs.chmodSync(to, fs.statSync(from).mode);
  }
  // A source changing during the copy, including an unlisted file, cannot
  // produce a stage whose selected bytes differ from the validated input.
  const original = files(root), staged = files(destination);
  if (JSON.stringify(original) !== JSON.stringify(staged) ||
      original.some(name => !fs.readFileSync(path.join(root, name)).equals(fs.readFileSync(path.join(destination, name))))) {
    throw new Error('Daemon package changed during staging');
  }
  return validateDaemonRoot(destination, source, platform);
}
function stageDaemon(selected, userData, platform = process.platform, appRoot) {
  if (!path.isAbsolute(userData)) throw new Error('Daemon user data path must be absolute');
  const container = path.join(userData, 'daemon-stages');
  // A replaced app bundle may disappear while its daemon is still running.
  for (const origin of [selected.root, appRoot].filter(Boolean)) {
    const rel = path.relative(origin, container);
    if (!rel.startsWith('..' + path.sep) && rel !== '..') throw new Error('Daemon stage must be outside the app and its source');
  }
  fs.mkdirSync(container, { recursive: true, mode: 0o700 });
  const name = `${selected.contract.plugin}-${selected.metadata.version}-${selected.metadata.binary_sha256.slice(0, 16)}`;
  const stage = fs.mkdtempSync(path.join(container, `${name}-`));
  try {
    // mkdtemp owns only this partial directory; never remove a complete or
    // another process's stage, even when its source app is uninstalled.
    return copyPackage(selected.root, stage, selected.source, platform);
  } catch (error) {
    fs.rmSync(stage, { recursive: true, force: true });
    throw error;
  }
}
module.exports = { copyPackage, stageDaemon };
