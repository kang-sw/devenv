'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { connectDaemon } = require('../daemon-controller');
const { fixture } = require('./daemon-fixture.cjs');
const URL = 'http://127.0.0.1:39418/'; // Only a value in injected discovery; never bound.
const config = f => ({ appRoot: f.appRoot, userData: f.userData, platform: 'darwin' });
const env = f => ({ WS_CACHE_HOME: path.join(f.base, 'isolated-cache'), WS_CONFIG_HOME: path.join(f.base, 'config'),
  WS_RSRC_ROOT: '/unrelated/resources', WS_SKILLS_ROOT: '/unrelated/skills',
  WS_MCP_CONFIG_MANIFESTS: '/unrelated/config', WS_MCP_RUNTIME_BINARY: '/unrelated/binary',
  WS_MCP_PROJECT_ROOT: '/unrelated/project', WS_MCP_DASHBOARD_DEV_SOURCE: '/unrelated/assets',
  WS_HOSTED_SESSION: '1', WS_HOSTED_SESSION_KEY: 'private', WS_HOSTED_SESSION_ROW: '/session/row', WS_MCP_HARNESS_PID: '22' });
test('REST-only attach to a healthy daemon does not select, stage or launch; no executable inspection', async t => {
  const f = fixture(t);
  const root = path.join(f.base, 'isolated-cache');
  fs.mkdirSync(path.join(root, 'dashboard'), { recursive: true });
  const server = http.createServer((req, res) => {
    assert.equal(req.url, '/health');
    res.end(JSON.stringify({ service: 'ws-dashboard', pid: process.pid, version: '0.99.0', lan: true }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(async () => { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); });
  fs.writeFileSync(path.join(root, 'dashboard/endpoint.json'), JSON.stringify({ port: server.address().port, pid: process.pid }));
  for (const platform of ['win32', 'darwin']) {
    assert.equal(await connectDaemon({ ...config(f), platform }, { env: env(f),
      select() { assert.fail('healthy attach must not read or validate a candidate'); },
      stage() { assert.fail('healthy attach must not stage'); },
      launch() { assert.fail('healthy attach must not execute anything'); } }), `http://127.0.0.1:${server.address().port}/`);
  }
});
test('missing macOS daemon starts only staged dashboard, preserves config/cache, strips hosted identity', async t => {
  const f = fixture(t, 'darwin');
  let running = false, calls = 0, launches = 0, stagedRoot;
  const options = { env: env(f), discover: async root => {
    assert.equal(root, options.env.WS_CACHE_HOME);
    calls++;
    if (!running) throw new Error('not healthy');
    return URL;
  }, launch: async (binary, opts) => {
    launches++;
    assert.equal(opts.cwd, path.dirname(binary));
    stagedRoot = opts.cwd;
    assert(stagedRoot.startsWith(path.join(f.userData, 'daemon-stages') + path.sep));
    assert.equal(fs.readFileSync(binary, 'utf8'), 'fixture bytes, not an executable');
    assert.equal(opts.env.WS_CACHE_HOME, options.env.WS_CACHE_HOME);
    assert.equal(opts.env.WS_CONFIG_HOME, options.env.WS_CONFIG_HOME);
    for (const name of ['WS_HOSTED_SESSION', 'WS_HOSTED_SESSION_KEY', 'WS_HOSTED_SESSION_ROW', 'WS_MCP_HARNESS_PID',
      'WS_MCP_PROJECT_ROOT', 'WS_MCP_DASHBOARD_DEV_SOURCE']) assert.equal(opts.env[name], undefined, name);
    for (const name of ['WS_RSRC_ROOT', 'WS_SKILLS_ROOT', 'CLAUDE_PLUGIN_ROOT', 'WS_MCP_RUNTIME_DIR']) {
      assert(opts.env[name].startsWith(stagedRoot), name);
    }
    assert.equal(opts.env.WS_MCP_RUNTIME_BINARY, binary);
    assert.deepEqual(opts.env.WS_MCP_CONFIG_MANIFESTS.split(';'),
      ['config-manifest.json', 'dashboard-config-manifest.json'].map(file => path.join(stagedRoot, file)));
    assert(fs.existsSync(path.join(stagedRoot, 'claude-agents/effort-low.md')));
    assert(fs.existsSync(path.join(stagedRoot, 'bin/ws-mcp-launcher.py')));
    fs.rmSync(f.appRoot, { recursive: true }); // Replaced Electron app can disappear while daemon is alive.
    running = true;
  } };
  assert.equal(await connectDaemon(config(f), options), URL);
  assert.equal(launches, 1);
  assert(calls >= 3);
  assert(fs.existsSync(path.join(stagedRoot, 'rsrc/hello/hello.md')));
  assert(fs.existsSync(path.join(stagedRoot, '.claude-plugin/plugin.json')));
});
test('development source stages its own package, invalid explicit selection never falls back', async t => {
  const f = fixture(t, 'darwin');
  f.choose({ schema_version: 1, root: f.dev.root });
  let staged;
  const deps = { env: env(f), discover: async () => { if (!staged) throw Error('missing'); return URL; },
    launch: async (binary, opts) => { staged = opts.cwd; assert.notEqual(binary, f.dev.binary); assert.equal(fs.readFileSync(path.join(staged, 'runtime.json'), 'utf8'), fs.readFileSync(f.dev.contractFile, 'utf8')); } };
  assert.equal(await connectDaemon(config(f), deps), URL);
  fs.rmSync(f.dev.pluginFile);
  staged = null;
  deps.launch = () => assert.fail('invalid dev cannot launch');
  await assert.rejects(connectDaemon(config(f), deps), /development daemon/);
});
test('raced lock winner is attached without LAN reconfiguration, even if first probes fail', async t => {
  const f = fixture(t, 'darwin');
  let probes = 0, entrants = 0, losers = 0, winner = null;
  const deps = { env: env(f), discover: async () => {
    probes++;
    if (probes <= 4 || !winner) throw Error('transient health failure');
    assert.equal(winner.lan, true, 'attached daemon retains its LAN mode');
    return winner.url;
  }, launch: async (_binary, opts) => {
    entrants++;
    assert.deepEqual(opts.env.WS_MCP_CONFIG_MANIFESTS.split(';').map(file => path.basename(file)),
      ['config-manifest.json', 'dashboard-config-manifest.json']);
    // Model the Go dashboard entry: both child processes start, only one
    // StartDaemon takes the lock; the loser exits without changing the winner.
    if (winner) losers++;
    else winner = { url: URL, lan: true };
  }, sleep: async () => {}, maxPolls: 5 };
  assert.deepEqual(await Promise.all([connectDaemon(config(f), deps), connectDaemon(config(f), deps)]), [URL, URL]);
  assert.equal(entrants, 2);
  assert.equal(losers, 1);
  assert.deepEqual(winner, { url: URL, lan: true });
});
test('plugin winner discovered while staging attaches with zero executable launches', async t => {
  const f = fixture(t, 'darwin');
  let probes = 0;
  assert.equal(await connectDaemon(config(f), { env: env(f),
    discover: async () => { if (++probes === 1) throw Error('transient health failure'); return URL; },
    launch: () => assert.fail('concurrent healthy winner must not be started over') }), URL);
  assert.equal(probes, 2);
});
test('hung/uncertain endpoint, failed spawn and retry do not stop any process or prune complete stages', async t => {
  const f = fixture(t, 'darwin');
  let count = 0, running = false;
  const deps = { env: env(f), discover: async () => { if (!running) throw Error('endpoint uncertain'); return URL; },
    launch: async () => { count++; }, sleep: async () => {}, maxPolls: 2 };
  await assert.rejects(connectDaemon(config(f), deps), /may hold the lock.*no process was stopped/);
  assert.equal(count, 1);
  const stages = fs.readdirSync(path.join(f.userData, 'daemon-stages'));
  deps.launch = async () => { throw Error('spawn refused'); };
  await assert.rejects(connectDaemon(config(f), deps), /spawn refused/);
  assert.equal(fs.readdirSync(path.join(f.userData, 'daemon-stages')).length, stages.length + 1);
  deps.launch = async () => { count++; running = true; };
  assert.equal(await connectDaemon(config(f), deps), URL);
  assert.equal(count, 2);
  assert.equal(fs.readdirSync(path.join(f.userData, 'daemon-stages')).length, stages.length + 2);
});
test('Windows refuses missing start before any package read or spawn, but attaches a healthy winner', async t => {
  const f = fixture(t);
  let healthy = false, checks = 0;
  const deps = { env: env(f), discover: async () => { checks++; if (healthy) return URL; throw Error('missing'); },
    select() { assert.fail('no Windows candidate selection before lifetime approval'); },
    launch() { assert.fail('no Windows launch before lifetime approval'); } };
  await assert.rejects(connectDaemon({ ...config(f), platform: 'win32' }, deps), /independent daemon lifetime is unavailable/);
  assert.equal(checks, 2);
  healthy = true;
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32' }, deps), URL);
  assert.equal(checks, 3);
});
