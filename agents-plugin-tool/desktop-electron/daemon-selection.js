'use strict';
// Read-only launch input selection. Nothing here starts, installs or accepts a daemon.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');

const MAX_JSON = 4 * 1024 * 1024; // runtime.json contains the tool inventory.
const SETTING = 'daemon-selection.json';

function readJSON(file) {
  const stat = fs.statSync(file);
  if (!stat.isFile() || stat.size > MAX_JSON) throw new Error(`Invalid or oversized ${file}`);
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}
function digest(file, normalize = false) {
  const hash = crypto.createHash('sha256');
  const fd = fs.openSync(file, 'r');
  try {
    const buffer = Buffer.alloc(1024 * 1024);
    let size;
    if (normalize) {
      // Resource manifests hash text after CRLF normalization (wsrsrc/hashHex).
      const bytes = fs.readFileSync(fd);
      hash.update(bytes.toString('utf8').replace(/\r\n/g, '\n'));
    } else while ((size = fs.readSync(fd, buffer, 0, buffer.length, null)) !== 0) {
      hash.update(buffer.subarray(0, size));
    }
  } finally { fs.closeSync(fd); }
  return hash.digest('hex');
}
function version(value) {
  if (typeof value !== 'string') return null;
  const parts = value.trim().split('-', 1)[0].split('.');
  if (parts.length !== 3 || parts.some(part => !/^\d+$/.test(part))) return null;
  const core = parts.map(Number);
  return core.every(Number.isSafeInteger) ? core : null;
}
function names(contract, field) {
  return Object.keys(contract[field] || {}).sort();
}
function inventory(metadata, contract, field, exact) {
  if (metadata[field] == null) return; // absent means unknown, not empty
  if (!Array.isArray(metadata[field]) || metadata[field].some(item => typeof item !== 'string')) {
    throw new Error(`Invalid daemon ${field} inventory`);
  }
  const actual = new Set(metadata[field]);
  for (const name of names(contract, field)) if (!actual.has(name)) throw new Error(`Daemon missing required ${field}: ${name}`);
  if (exact) for (const name of actual) if (!Object.hasOwn(contract[field] || {}, name)) {
    throw new Error(`Daemon exposes unexpected ${field}: ${name}`);
  }
}
function validateTree(root, label) {
  const manifest = readJSON(path.join(root, 'manifest.json'));
  if (manifest?.schema_version !== 1 || !manifest.files || Array.isArray(manifest.files) ||
      typeof manifest.files !== 'object' || !Object.keys(manifest.files).length) {
    throw new Error(`Invalid ${label} manifest`);
  }
  for (const [name, expected] of Object.entries(manifest.files)) {
    if (!name || name.startsWith('/') || name.includes('\\') || name.split('/').some(part => part === '..' || part === '.' || !part) ||
        !/^[a-f0-9]{64}$/.test(expected)) throw new Error(`Invalid ${label} manifest entry: ${name}`);
    const file = path.join(root, ...name.split('/'));
    if (!fs.realpathSync(file).startsWith(fs.realpathSync(root) + path.sep)) throw new Error(`${label} path escapes its root: ${name}`);
    if (!fs.statSync(file).isFile() || digest(file, true) !== expected) throw new Error(`${label} hash mismatch: ${name}`);
  }
  const entries = Object.keys(manifest.files);
  if (label === 'skills' && !entries.some(name => name.endsWith('/SKILL.md')) ||
      label === 'rsrc' && !entries.some(name => name.endsWith('.md'))) throw new Error(`Empty ${label} content`);
}
function selectDaemon({ appRoot, userData, platform = process.platform }) {
  if (!path.isAbsolute(appRoot) || !path.isAbsolute(userData)) throw new Error('Daemon app and user data paths must be absolute');
  const setting = path.join(userData, SETTING);
  let root = path.join(appRoot, 'daemon'), source = 'bundled';
  if (fs.existsSync(setting)) {
    let selection;
    try { selection = readJSON(setting); }
    catch (error) { throw new Error(`Invalid daemon selection at ${setting}: ${error.message}`); }
    if (selection?.schema_version !== 1 || typeof selection.root !== 'string' ||
        !path.isAbsolute(selection.root) || Object.keys(selection).sort().join(',') !== 'root,schema_version') {
      throw new Error(`Invalid daemon selection at ${setting}: expected absolute development root`);
    }
    root = selection.root; source = 'development';
  }
  const binary = path.join(root, platform === 'win32' ? 'ws-mcp.exe' : 'ws-mcp');
  try {
    const contract = readJSON(path.join(root, 'runtime.json'));
    const metadata = readJSON(binary + '.metadata.json');
    if (!fs.statSync(binary).isFile() || metadata?.schema_version !== 1 || !version(metadata.version) ||
        !/^[a-f0-9]{64}$/.test(metadata.binary_sha256) || digest(binary) !== metadata.binary_sha256) {
      throw new Error('daemon binary metadata missing, invalid or hash mismatch');
    }
    const actual = version(metadata.version), expected = version(contract.plugin_version);
    if (!expected || (source === 'development' ? actual[0] !== expected[0] || actual[1] !== expected[1] :
      actual.some((part, index) => part !== expected[index]))) throw new Error('daemon version/contract mismatch');
    if (Object.hasOwn(metadata, 'mcp_protocol') && metadata.mcp_protocol !== String(contract.mcp_protocol || '')) {
      throw new Error('daemon MCP protocol mismatch');
    }
    const exact = String(contract.runtime_capabilities?.match || '').toLowerCase() === 'exact';
    inventory(metadata, contract, 'tools', exact);
    inventory(metadata, contract, 'commands', exact);
    const rsrc = path.join(root, 'rsrc'), skills = path.join(root, 'skills');
    validateTree(rsrc, 'rsrc');
    validateTree(skills, 'skills');
    return { source, binary, contract, metadata, env: { WS_RSRC_ROOT: rsrc, WS_SKILLS_ROOT: skills } };
  } catch (error) { throw new Error(`Cannot use ${source} daemon at ${root}: ${error.message}`); }
}
module.exports = { selectDaemon, SETTING };
