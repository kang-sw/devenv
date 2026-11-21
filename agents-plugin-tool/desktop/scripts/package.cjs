'use strict';
// The only supported Phase 2 packaged build. Cargo builds remain input-free.
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { copyPackage } = require('../../desktop-electron/daemon-stage');

const desktop = path.resolve(__dirname, '..');
const repo = path.resolve(desktop, '../..');

function checkBinary(binary, platform) {
  const bytes = fs.readFileSync(binary);
  if (platform === 'darwin') {
    // 64-bit little-endian Mach-O, arm64 (the supported macOS package).
    if (bytes.length < 8 || bytes.readUInt32LE(0) !== 0xfeedfacf || bytes.readUInt32LE(4) !== 0x0100000c)
      throw new Error('Expected a darwin-arm64 Mach-O daemon');
  } else {
    const offset = bytes.length >= 64 ? bytes.readUInt32LE(0x3c) : bytes.length;
    if (bytes.length < offset + 6 || bytes.toString('ascii', 0, 2) !== 'MZ' ||
        bytes.readUInt32LE(offset) !== 0x4550 || bytes.readUInt16LE(offset + 4) !== 0x8664)
      throw new Error('Expected a win32-x64 PE daemon');
  }
  const python = process.env.PYTHON || (process.platform === 'win32' ? 'python' : 'python3');
  const inventory = execFileSync(python, [path.join(repo, 'agents-plugin-tool/scripts/generate-package-assets.py'), '--inventory']);
  if (!bytes.includes(inventory)) throw new Error('Daemon lacks this checkout’s embedded package inventory');
}

function resourceMap(root) {
  const map = {};
  function walk(dir, rel = '') {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const name = path.join(rel, entry.name), source = path.join(root, name);
      if (entry.isDirectory()) walk(source, name);
      else if (entry.isFile()) map[source] = `daemon/${name.split(path.sep).join('/')}`;
      else throw new Error(`Unexpected daemon package entry: ${source}`);
    }
  }
  walk(root);
  return map;
}

function build({ input = process.env.WERKSCHAFT_DAEMON_INPUT, platform = process.platform } = {}) {
  if (!['darwin', 'win32'].includes(platform)) throw new Error('Packaging requires macOS or Windows');
  if (!input || !path.isAbsolute(input)) throw new Error('WERKSCHAFT_DAEMON_INPUT must name an absolute assembled package');
  if ((platform === 'darwin' && process.arch !== 'arm64') || (platform === 'win32' && process.arch !== 'x64'))
    throw new Error('Packaging requires darwin-arm64 or win32-x64');
  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-tauri-package-'));
  try {
    const daemon = path.join(stage, 'daemon');
    const selected = copyPackage(input, daemon, 'bundled', platform);
    checkBinary(selected.binary, platform);
    const config = path.join(stage, 'tauri.conf.json');
    fs.writeFileSync(config, JSON.stringify({ bundle: { resources: resourceMap(daemon) } }));
    const cli = require.resolve('@tauri-apps/cli/tauri.js');
    const args = [cli, 'build', '--ci', '--config', config];
    if (platform === 'darwin') args.push('--bundles', 'app', '--no-sign');
    else args.push('--no-bundle');
    args.push('--', '--locked');
    execFileSync(process.execPath, args, { cwd: desktop, stdio: 'inherit' });
    const release = path.join(desktop, 'target', 'release');
    if (platform === 'darwin') {
      const output = path.join(release, 'bundle/macos/Werkschaft.app/Contents/Resources/daemon');
      // Compare and revalidate the actual bundle, not just the input.
      comparePackage(daemon, output, platform);
      console.log(`Packaged app: ${path.dirname(path.dirname(path.dirname(output)))}`);
    } else {
      // Tauri's unbundled resource_dir is the executable's directory on Windows.
      const output = path.join(release, 'phase2-smoke');
      const target = path.join(output, 'daemon');
      if (fs.existsSync(output)) throw new Error(`Remove the previous smoke output before rebuilding: ${output}`);
      fs.mkdirSync(output, { recursive: true });
      fs.copyFileSync(path.join(release, 'werkschaft-desktop.exe'), path.join(output, 'werkschaft-desktop.exe'));
      copyPackage(daemon, target, 'bundled', platform);
      comparePackage(daemon, target, platform);
      console.log(`Windows smoke payload: ${output}`);
    }
  } finally { fs.rmSync(stage, { recursive: true, force: true }); }
}

function comparePackage(source, target, platform) {
  const expected = resourceMap(source), actual = resourceMap(target);
  const sourceNames = Object.values(expected).sort(), targetNames = Object.values(actual).sort();
  if (JSON.stringify(sourceNames) !== JSON.stringify(targetNames)) throw new Error('Packaged daemon file list differs from input');
  for (const [file, name] of Object.entries(expected)) {
    const destination = path.join(target, ...name.slice('daemon/'.length).split('/'));
    if (!fs.readFileSync(file).equals(fs.readFileSync(destination))) throw new Error(`Packaged daemon differs: ${name}`);
  }
  require('../../desktop-electron/daemon-selection').validateDaemonRoot(target, 'bundled', platform);
}

if (require.main === module) {
  try { build(); } catch (error) { console.error(error.message); process.exitCode = 1; }
}
module.exports = { build, checkBinary, resourceMap, comparePackage };
