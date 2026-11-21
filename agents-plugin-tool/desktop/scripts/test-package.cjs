'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { build, checkBinary, resourceMap, comparePackage } = require('./package.cjs');

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
