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
  // Electron extracts ASAR entries to temp for open/openSync (including fd
  // reads), but readFileSync by path reads the archive without extraction.
  const bytes = fs.readFileSync(file);
  // Resource manifests hash text after CRLF normalization (wsrsrc/hashHex).
  return crypto.createHash('sha256').update(normalize
    ? bytes.toString('utf8').replace(/\r\n/g, '\n') : bytes).digest('hex');
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
  const listed = new Set(Object.keys(manifest.files));
  function walk(dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const file = path.join(dir, entry.name);
      if (entry.isDirectory()) { walk(file); continue; }
      const name = path.relative(root, file).split(path.sep).join('/');
      if (name === 'manifest.json') continue;
      if (!listed.has(name)) throw new Error(`Unlisted ${label} file: ${name}`);
    }
  }
  walk(root);
  const entries = [...listed];
  if (label === 'skills' && !entries.some(name => name.endsWith('/SKILL.md')) ||
      label === 'rsrc' && !entries.some(name => name.endsWith('.md'))) throw new Error(`Empty ${label} content`);
}
function packageFiles(root) {
  const plugin = readJSON(path.join(root, '.claude-plugin', 'plugin.json'));
  if (!['ws', 'wsflow'].includes(plugin?.name)) throw new Error('Invalid Claude plugin manifest (expected ws or wsflow)');
  const server = plugin.mcpServers?.[plugin.name];
  if (!Array.isArray(plugin.agents) || !plugin.agents.length || !server ||
      !Array.isArray(server.args)) throw new Error('Incomplete Claude plugin references');
  const prefix = '${CLAUDE_PLUGIN_ROOT}/';
  const args = server.args;
  const manifests = server.env?.WS_MCP_CONFIG_MANIFESTS?.split(';');
  if (!args.includes(prefix + 'bin/ws-mcp-launcher.py') ||
      !Array.isArray(manifests) || manifests.length !== 2 ||
      !manifests.includes(prefix + 'config-manifest.json') ||
      !manifests.includes(prefix + 'dashboard-config-manifest.json')) {
    throw new Error('Incomplete Claude launcher or config references');
  }
  const references = [...plugin.agents, ...args.filter(arg => arg.startsWith(prefix)), ...manifests];
  for (const ref of references) {
    if (typeof ref !== 'string') throw new Error('Invalid Claude plugin reference');
    const relative = ref.startsWith(prefix) ? ref.slice(prefix.length) : ref.startsWith('./') ? ref.slice(2) : '';
    if (!relative || relative.includes('\\') || relative.split('/').some(part => !part || part === '.' || part === '..')) {
      throw new Error(`Invalid Claude plugin reference: ${ref}`);
    }
    const file = path.join(root, ...relative.split('/'));
    if (!fs.realpathSync(file).startsWith(fs.realpathSync(root) + path.sep) || !fs.statSync(file).isFile()) {
      throw new Error(`Invalid Claude plugin file: ${relative}`);
    }
    if (relative.endsWith('.json')) readJSON(file);
    else fs.readFileSync(file); // A referenced text file must be readable, not merely present.
  }
  return plugin;
}
function validateDaemonRoot(root, source, platform) {
  if (!path.isAbsolute(root)) throw new Error('Daemon root must be absolute');
  const binary = path.join(root, platform === 'win32' ? 'ws-mcp.exe' : 'ws-mcp');
  try {
    const plugin = packageFiles(root);
    const contract = readJSON(path.join(root, 'runtime.json'));
    if (contract.plugin !== plugin.name) throw new Error('Claude plugin/runtime mismatch');
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
    return { source, root, binary, contract, metadata, env: { WS_RSRC_ROOT: rsrc, WS_SKILLS_ROOT: skills } };
  } catch (error) { throw new Error(`Cannot use ${source} daemon at ${root}: ${error.message}`); }
}
function selectDaemon({ appRoot, userData, platform = process.platform, packaged = true }) {
  if (!path.isAbsolute(appRoot) || !path.isAbsolute(userData)) throw new Error('Daemon app and user data paths must be absolute');
  const setting = path.join(userData, SETTING);
  // build.cjs unpacks the entire daemon tree. Use its physical path so even
  // staging's copyFileSync never asks Electron to extract an archive entry.
  const bundledRoot = appRoot.endsWith('.asar') ? appRoot + '.unpacked' : appRoot;
  let root = path.join(bundledRoot, 'daemon'), source = 'bundled';
  if (fs.existsSync(setting)) {
    let selection;
    try { selection = readJSON(setting); }
    catch (error) { throw new Error(`Invalid daemon selection at ${setting}: ${error.message}`); }
    if (selection?.schema_version !== 1 || typeof selection.root !== 'string' ||
        !path.isAbsolute(selection.root) || Object.keys(selection).sort().join(',') !== 'root,schema_version') {
      throw new Error(`Invalid daemon selection at ${setting}: expected absolute development root`);
    }
    root = selection.root; source = 'development';
  } else if (!packaged && !fs.existsSync(root)) {
    // The desktop assembler defaults to the wsflow package in the checkout.
    root = path.resolve(appRoot, '../../agents-plugin-wsflow'); source = 'development';
    try { return validateDaemonRoot(root, source, platform); }
    catch (error) {
      throw new Error(`${error.message}. To prepare a bundled input instead, run from the repository root: node agents-plugin-tool/desktop-electron/scripts/assemble-daemon-input.cjs --binary <absolute-path-to-ws-mcp> --out agents-plugin-tool/desktop-electron/daemon --platform ${platform} (with matching binary metadata).`);
    }
  }
  return validateDaemonRoot(root, source, platform);
}
module.exports = { selectDaemon, validateDaemonRoot, SETTING };
