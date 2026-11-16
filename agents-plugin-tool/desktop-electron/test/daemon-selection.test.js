'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { fixture, sha } = require('./daemon-fixture.cjs');
const { SETTING } = require('../daemon-selection');
test('bundled and explicit dev roots bind matching binary, contract, resources and skills without executing bytes', t => {
  const f = fixture(t);
  const bundled = f.select();
  assert.equal(bundled.source, 'bundled');
  assert.equal(bundled.binary, f.bundled.binary);
  assert.deepEqual(bundled.env, { WS_RSRC_ROOT: path.join(f.bundled.root, 'rsrc'), WS_SKILLS_ROOT: path.join(f.bundled.root, 'skills') });
  f.choose({ schema_version: 1, root: f.dev.root });
  const selected = f.select();
  assert.equal(selected.source, 'development');
  assert.equal(selected.binary, f.dev.binary);
  assert.equal(selected.metadata.version, '0.50.3-dev');
  assert.equal(selected.env.WS_RSRC_ROOT, path.join(f.dev.root, 'rsrc'));
  assert.equal(selected.env.WS_SKILLS_ROOT, path.join(f.dev.root, 'skills'));
  assert.equal(fs.existsSync(path.join(f.userData, 'update-state.json')), false);
});
test('selection is shell-local and fails closed on malformed, relative or missing dev input', t => {
  const f = fixture(t, 'darwin');
  for (const value of [{ root: f.dev.root }, { schema_version: 1, root: '../selected' },
    { schema_version: 1, root: f.dev.root, accepted: true }, { schema_version: 2, root: f.dev.root }]) {
    f.choose(value); assert.throws(f.select, /Invalid daemon selection/);
  }
  fs.writeFileSync(path.join(f.userData, SETTING), '{');
  assert.throws(f.select, /Invalid daemon selection/);
  f.choose({ schema_version: 1, root: path.join(f.base, 'missing') });
  assert.throws(f.select, /Cannot use development daemon/);
  assert(fs.existsSync(f.bundled.binary), 'a bad dev selection cannot silently fall back to bundled');
});
test('binary hash, version, optional protocol and inventory gates match the launcher', t => {
  const f = fixture(t);
  const { bundled: b } = f;
  const save = (file, value) => b.save(file, value);
  function rejected(change, file = b.binary + '.metadata.json') {
    save(file, change); assert.throws(f.select, /Cannot use bundled daemon/);
  }
  fs.appendFileSync(b.binary, 'tampered'); assert.throws(f.select, /hash mismatch/);
  fs.writeFileSync(b.binary, 'fixture bytes, not an executable');
  for (const version of ['0.50.0', '0.51.1', 'garbage']) rejected({ ...b.metadata, version });
  rejected({ ...b.metadata, mcp_protocol: 'old' });
  rejected({ ...b.metadata, tools: [] });
  rejected({ ...b.metadata, commands: [] });
  save(b.binary + '.metadata.json', { ...b.metadata, tools: ['runtime.read', 'extra'] });
  assert.equal(f.select().metadata.tools.length, 2, 'non-exact contracts permit extra capabilities');
  save(b.contractFile, { ...b.contract, runtime_capabilities: { match: 'exact' } });
  assert.throws(f.select, /unexpected tools/);
  save(b.binary + '.metadata.json', { schema_version: 1, version: '0.50.1', binary_sha256: b.metadata.binary_sha256 });
  assert.equal(f.select().metadata.tools, undefined, 'legacy producer inventories/protocol are unknown, not empty');
});
test('bundled and dev must be complete hostable Claude packages (Pi-only is not enough)', t => {
  for (const platform of ['win32', 'darwin']) {
    const f = fixture(t, platform);
    for (const selected of [f.bundled, f.dev]) {
      if (selected === f.dev) f.choose({ schema_version: 1, root: selected.root });
      const reject = /Cannot use (bundled|development) daemon/;
      fs.rmSync(selected.pluginFile);
      assert.throws(f.select, reject);
      selected.save(selected.pluginFile, { name: 'pi' });
      assert.throws(f.select, reject);
      selected.save(selected.pluginFile, selected.plugin);
      for (const file of ['bin/ws-mcp-launcher.py', 'config-manifest.json', 'dashboard-config-manifest.json', 'claude-agents/effort-low.md']) {
        const target = path.join(selected.root, file), bytes = fs.readFileSync(target);
        fs.rmSync(target); assert.throws(f.select, reject, file);
        fs.writeFileSync(target, bytes);
      }
      selected.save(selected.pluginFile, { ...selected.plugin, agents: ['./claude-agents/../bad.md'] });
      assert.throws(f.select, /Invalid Claude plugin reference/);
      selected.save(selected.pluginFile, selected.plugin);
    }
  }
});
test('wsflow package uses its named MCP server and matching runtime contract', t => {
  const f = fixture(t);
  for (const selected of [f.bundled, f.dev]) {
    if (selected === f.dev) f.choose({ schema_version: 1, root: selected.root });
    selected.save(selected.pluginFile, { ...selected.plugin, name: 'wsflow',
      mcpServers: { wsflow: selected.plugin.mcpServers.ws } });
    assert.throws(f.select, /plugin\/runtime mismatch/);
    selected.save(selected.contractFile, { ...selected.contract, plugin: 'wsflow' });
    assert.equal(f.select().contract.plugin, 'wsflow');
    selected.save(selected.pluginFile, { ...selected.plugin, name: 'wsflow' });
    assert.throws(f.select, /Incomplete Claude plugin references/, 'the ws server cannot stand in for wsflow');
  }
});
test('resource/skills paths and manifest bytes must belong to the selected root', t => {
  const f = fixture(t);
  for (const tree of ['rsrc', 'skills']) {
    const extra = path.join(f.bundled.root, tree, 'unexpected', tree === 'rsrc' ? 'extra.md' : 'SKILL.md');
    fs.mkdirSync(path.dirname(extra)); fs.writeFileSync(extra, 'unlisted skill or resource');
    assert.throws(f.select, /Unlisted .* file/, 'unlisted skills are readable by Go without manifest checks');
    fs.rmSync(path.dirname(extra), { recursive: true });
    const file = path.join(f.bundled.root, tree, 'hello', tree === 'rsrc' ? 'hello.md' : 'SKILL.md');
    fs.appendFileSync(file, 'tampered');
    assert.throws(f.select, /hash mismatch/);
    fs.writeFileSync(file, 'hello\r\nworld\r\n');
    const manifest = path.join(f.bundled.root, tree, 'manifest.json');
    const original = fs.readFileSync(manifest);
    f.bundled.save(manifest, { schema_version: 1, files: { '../outside': sha('outside') } });
    assert.throws(f.select, /Invalid .* manifest entry/);
    fs.writeFileSync(manifest, original);
    fs.rmSync(manifest); assert.throws(f.select, /Cannot use bundled daemon/);
    fs.writeFileSync(manifest, original);
  }
});
