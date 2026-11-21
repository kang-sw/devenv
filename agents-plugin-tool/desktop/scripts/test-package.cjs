'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { build, checkBinary, resourceMap, comparePackage, releaseConfig } = require('./package.cjs');

test('maps dotfiles and nested files to their exact daemon resource paths', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-map-test-'));
  const other = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-map-test-'));
  try {
    fs.mkdirSync(path.join(root, '.claude-plugin'));
    fs.writeFileSync(path.join(root, '.claude-plugin', 'plugin.json'), '{}');
    fs.writeFileSync(path.join(root, 'ws-mcp'), 'binary');
    const map = resourceMap(root);
    assert.equal(map[path.join(root, '.claude-plugin', 'plugin.json')], 'daemon/.claude-plugin/plugin.json');
    assert.equal(map[path.join(root, 'ws-mcp')], 'daemon/ws-mcp');
    assert.throws(() => comparePackage(root, other, 'darwin'), /file list differs/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); fs.rmSync(other, { recursive: true, force: true }); }
});

test('rejects missing input and wrong architecture before invoking Tauri', () => {
  assert.throws(() => build({ input: 'relative', platform: 'darwin' }), /absolute assembled package/);
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-binary-test-'));
  try {
    const file = path.join(root, 'binary');
    fs.writeFileSync(file, 'not a binary');
    assert.throws(() => checkBinary(file, 'darwin'), /darwin-arm64/);
    assert.throws(() => checkBinary(file, 'win32'), /win32-x64/);
  } finally { fs.rmSync(root, { recursive: true, force: true }); }
});

test('release overlays retain the exact updater version and platform installer contracts', () => {
  const version = '0.50.8-canary.2';
  const pubkey = 'encoded-public-key';
  const resources = { '/prepared/daemon/ws-mcp': 'daemon/ws-mcp' };
  const mac = releaseConfig({ version, pubkey, resources, platform: 'darwin' });
  const windows = releaseConfig({ version, pubkey, resources, platform: 'win32' });
  for (const config of [mac, windows]) {
    assert.equal(config.version, version);
    assert.deepEqual(config.plugins.updater, { pubkey, requireSignedVersion: true, windows: { installMode: 'passive' } });
    assert.equal(config.bundle.createUpdaterArtifacts, true);
    assert.deepEqual(config.bundle.resources, resources);
  }
  assert.equal(mac.mainBinaryName, 'Werkschaft');
  assert.deepEqual(mac.bundle.targets, ['app']);
  assert.equal(windows.mainBinaryName, undefined);
  assert.deepEqual(windows.bundle.targets, ['nsis', 'msi']);
  assert.deepEqual(windows.bundle.windows, {
    webviewInstallMode: { type: 'embedBootstrapper', silent: true }, nsis: { installMode: 'currentUser' },
    allowDowngrades: true, wix: { version: '0.50.8' },
  });
});
