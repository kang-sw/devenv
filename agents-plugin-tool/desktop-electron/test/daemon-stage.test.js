'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { fixture } = require('./daemon-fixture.cjs');
const { stageDaemon } = require('../daemon-stage');
const { validateDaemonRoot } = require('../daemon-selection');
const { build } = require('../scripts/build.cjs');

test('staging retains complete versions outside the app; cleans only its own failed partial', t => {
  const f = fixture(t, 'darwin');
  const selected = f.select();
  const a = stageDaemon(selected, f.userData, 'darwin', f.appRoot);
  assert(a.root.includes('0.50.1-dev-'));
  const prior = fs.readdirSync(path.join(f.userData, 'daemon-stages'));
  // Windows without Developer Mode cannot create symlinks; still exercise
  // cleanup after a copy fails, which happens after the stage is owned.
  const copy = fs.copyFileSync;
  fs.copyFileSync = () => { throw Error('injected copy failure'); };
  try { assert.throws(() => stageDaemon(selected, f.userData, 'darwin', f.appRoot), /injected copy failure/); }
  finally { fs.copyFileSync = copy; }
  assert.deepEqual(fs.readdirSync(path.join(f.userData, 'daemon-stages')), prior);
  const b = stageDaemon(selected, f.userData, 'darwin', f.appRoot);
  assert.notEqual(a.root, b.root);
  fs.rmSync(f.appRoot, { recursive: true });
  for (const staged of [a, b]) {
    assert(fs.existsSync(path.join(staged.root, 'runtime.json')));
    assert(fs.existsSync(path.join(staged.root, 'bin/ws-mcp-launcher.py')));
    assert(fs.existsSync(path.join(staged.root, 'claude-agents/effort-low.md')));
    assert.equal(validateDaemonRoot(staged.root, 'bundled', 'darwin').binary, staged.binary);
  }
});
test('untrusted version suffix cannot escape or name a path outside the staging container', t => {
  const f = fixture(t, 'darwin');
  f.bundled.save(f.bundled.binary + '.metadata.json', { ...f.bundled.metadata, version: '0.50.1-../../outside' });
  const staged = stageDaemon(f.select(), f.userData, 'darwin', f.appRoot);
  assert.equal(path.dirname(staged.root), path.join(f.userData, 'daemon-stages'));
  assert(!path.basename(staged.root).includes('..'));
});
test('a symlinked userData inside the app cannot masquerade as out-of-bundle stage', t => {
  const f = fixture(t, 'darwin'), selected = f.select();
  assert.throws(() => stageDaemon(selected, f.appRoot, 'darwin', f.appRoot), /outside the app/);
  const linked = path.join(f.base, 'linked-userdata');
  try { fs.symlinkSync(f.appRoot, linked, 'junction'); }
  catch (error) {
    if (error.code !== 'EPERM') throw error;
    t.diagnostic('Windows symlink creation unavailable; direct in-bundle refusal covered');
    return;
  }
  assert.throws(() => stageDaemon(selected, linked, 'darwin', f.appRoot), /outside the app/);
});
test('builder accepts only a supplied validated input and copies entire package without running it', async t => {
  const f = fixture(t);
  const out = path.join(f.base, 'out'), cache = path.join(f.base, 'cache');
  fs.mkdirSync(out); fs.mkdirSync(cache);
  fs.writeFileSync(path.join(cache, 'electron-v44.11.3-win32-x64.zip'), 'offline cache fixture');
  let packaged = 0, archived = 0;
  const packageApp = async opts => {
    packaged++;
    assert.equal(opts.electronZipDir, cache);
    const dir = path.join(out, 'Werkschaft-win32-x64');
    fs.cpSync(opts.dir, dir, { recursive: true });
    const daemon = path.join(dir, 'daemon');
    assert.equal(fs.readFileSync(path.join(daemon, 'bin/ws-mcp-launcher.py'), 'utf8'), 'text-only fixture');
    assert(fs.existsSync(path.join(daemon, '.claude-plugin/plugin.json')));
    assert(fs.existsSync(path.join(daemon, 'claude-agents/effort-low.md')));
    assert(fs.existsSync(path.join(daemon, 'rsrc/hello/hello.md')));
    // Resolve every local JS helper from the packaged source, not the source tree.
    const pending = ['main.js'], visited = new Set();
    while (pending.length) {
      const file = pending.pop();
      if (visited.has(file)) continue;
      visited.add(file);
      const text = fs.readFileSync(path.join(dir, file), 'utf8');
      for (const match of text.matchAll(/require\(['"](\.\/.+?)['"]\)/g)) {
        const helper = path.normalize(path.join(path.dirname(file), match[1] + '.js'));
        assert(fs.existsSync(path.join(dir, helper)), `Missing packaged helper: ${helper}`);
        pending.push(helper);
      }
    }
    assert(visited.has('daemon-controller.js'));
    assert(visited.has('daemon-version.js'));
    assert.equal(typeof require(path.join(dir, 'daemon-controller.js')).connectDaemon, 'function');
    return [dir];
  };
  const run = (_tar, args) => { archived++; fs.writeFileSync(args[args.indexOf('-f') + 1], 'archive fixture'); };
  const options = { platform: 'win32', arch: 'x64', daemonInput: f.bundled.root, cache, packageApp, out, run };
  await assert.rejects(build({ ...options, daemonInput: undefined }), /preassembled daemonInput/);
  fs.rmSync(f.bundled.pluginFile);
  await assert.rejects(build(options), /Cannot use bundled daemon/);
  assert.equal(packaged, 0);
  f.bundled.save(f.bundled.pluginFile, f.bundled.plugin);
  await build(options);
  assert.equal(packaged, 1);
  assert.equal(archived, 1);
  assert.equal(fs.readFileSync(f.bundled.binary, 'utf8'), 'fixture bytes, not an executable');
});
