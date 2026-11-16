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
test('numeric REST core precedence, explicit one-shot equal swap, and cross-boundary refusal', async t => {
  const f = fixture(t, 'darwin');
  let live = { pid: 120, version: '0.50.1', lan: true, started_at: 'old', content_hash: 'old' };
  let launches = [], drains = 0, notices = [];
  const deps = { env: env(f), onNotice: s => notices.push(s),
    inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    preflight: async (binary, opts, args) => {
      assert.equal(fs.readFileSync(binary, 'utf8'), 'fixture bytes, not an executable');
      assert(opts.cwd.startsWith(path.join(f.userData, 'daemon-stages')));
      launches.push(args);
    },
    shutdown: async () => { drains++; live = { ...live, pid: 121, started_at: 'new', content_hash: f.bundled.metadata.binary_sha256 }; },
    sleep: async () => {} };
  live.version = '0.50.4';
  assert.equal(await connectDaemon(config(f), deps), URL);
  assert.equal(launches.length, 0);
  live.version = '0.50.1-dev';
  assert.equal(await connectDaemon(config(f), deps), URL);
  assert.equal(launches.length, 0);
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.equal(drains, 1);
  assert.deepEqual(launches, [['dashboard', '--port', '32000', '--lock-wait', '8s', '--lan']]);
  assert.equal(await connectDaemon(config(f), deps), URL);
  assert.equal(launches.length, 1, 'explicit replacement is consumed once');
  live = { ...live, pid: 122, started_at: 'plugin-winner', content_hash: 'different' };
  deps.shutdown = async () => { drains++; live = { ...live, pid: 123, started_at: 'plugin-next' }; };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.match(notices.at(-1), /different daemon won/);
  assert.equal(drains, 2);
  live.version = '0.49.9';
  assert.equal(await connectDaemon(config(f), deps), URL);
  assert.equal(launches.length, 2, 'unknown cross-minor clients keep incumbent');
  assert.match(notices.at(-1), /attached client versions are unknown.*update connected plugins/i);
});
test('macOS detached creation failure leaves incumbent serving before drain', async t => {
  const f = fixture(t, 'darwin');
  let notices = [];
  const deps = { env: env(f), onNotice: s => notices.push(s),
    inspect: async () => ({ url: URL, endpoint: { pid: 120, port: 32000 },
      health: { pid: 120, version: '0.50.1', started_at: 'old', lan: false } }),
    launch: async () => { throw Error('OS refused detached creation'); },
    shutdown: () => assert.fail('no authenticated drain on failed preflight') };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.match(notices.at(-1), /preflight failed.*not drained/);
});
test('approved OS-accepted macOS creation precedes drain without claiming lock readiness', async t => {
  const f = fixture(t, 'darwin');
  let live = { pid: 120, version: '0.50.1', lan: true, started_at: 'old' };
  const order = [];
  const deps = { env: env(f), inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    launch: async (_binary, _options, args) => { order.push('created'); assert(args.includes('--lock-wait')); assert(args.includes('--lan')); },
    shutdown: async () => { order.push('drained'); live = { ...live, pid: 121, started_at: 'new' }; } };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.deepEqual(order, ['created', 'drained']);
});
test('Windows strict starter attaches a newer cross-minor winner without a second spawn or drain', async t => {
  const f = fixture(t, 'win32');
  let live = { pid: 120, version: '0.50.1', lan: true, started_at: 'old' };
  let calls = 0, notice;
  const deps = { env: env(f), inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    onNotice: s => { notice = s; }, shutdown: () => assert.fail('Go owns drain'),
    launch: async (_binary, opts, args, wait) => {
      calls++;
      assert.equal(wait, true);
      assert(args.includes('--require-independent-lifetime'));
      assert.equal(opts.env.WS_CACHE_HOME, env(f).WS_CACHE_HOME);
      if (calls === 1) live = { pid: 122, version: '0.51.0', lan: false, started_at: 'raced' };
      else live = { pid: 123, version: '0.50.1', lan: true, started_at: 'successor' };
    } };
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32', replace: true }, deps), URL);
  assert.match(notice, /newer daemon won.*update connected plugins/i);
  assert.equal(calls, 1);
  assert.equal(live.version, '0.51.0'); // A plugin may win; never claim it was our successor.
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32', replace: true }, deps), URL);
  assert.equal(calls, 1); // A newer core stays; no downgrade.
  live = { pid: 120, version: '0.50.1', lan: true, started_at: 'old' };
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32', replace: true }, deps), URL);
  assert.equal(calls, 2);
  assert.equal(live.pid, 123);
  assert.match(notice, /newer daemon won/);
});
test('Windows strict starter failure attaches only a verified newer raced winner', async t => {
  const f = fixture(t, 'win32');
  let live = { pid: 120, version: '0.50.1', started_at: 'old' }, notice, calls = 0;
  const deps = { env: env(f), onNotice: text => { notice = text; },
    inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    launch: async () => { calls++; live = { pid: 121, version: '1.0.0', started_at: 'raced' }; throw Error('starter failed'); },
    shutdown: () => assert.fail('Go owns drain') };
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32', replace: true }, deps), URL);
  assert.match(notice, /newer daemon won.*update connected plugins/i);
  assert.equal(calls, 1);
  live = { pid: 120, version: '0.50.1', started_at: 'old' };
  deps.launch = async () => { calls++; live = { pid: 122, version: '0.49.9', started_at: 'older' }; throw Error('starter failed'); };
  await assert.rejects(connectDaemon({ ...config(f), platform: 'win32', replace: true }, deps), /older or unidentified daemon won/);
  assert.equal(calls, 2);
});
test('failed macOS drain attaches a newer winner but refuses an older or unidentified one', async t => {
  const f = fixture(t, 'darwin');
  let live = { pid: 120, version: '0.50.1', lan: false, started_at: 'old' };
  let notice;
  const deps = { env: env(f), inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    launch: async () => {}, onNotice: text => { notice = text; },
    shutdown: async () => { live = { pid: 121, version: '0.51.0', lan: true, started_at: 'raced' }; throw Error('stale endpoint'); } };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.match(notice, /newer daemon won.*update connected plugins/i);
  live = { pid: 120, version: '0.50.1', lan: false, started_at: 'old' };
  deps.shutdown = async () => { live = { pid: 121, version: '0.49.9', started_at: 'older' }; throw Error('stale endpoint'); };
  await assert.rejects(connectDaemon({ ...config(f), replace: true }, deps), /older or unidentified daemon won/);
  live = { pid: 120, version: '0.50.1', lan: false, started_at: 'old' };
  deps.shutdown = async () => { live = { pid: 121, version: 'unknown', started_at: 'unknown' }; throw Error('stale endpoint'); };
  await assert.rejects(connectDaemon({ ...config(f), replace: true }, deps), /older or unidentified daemon won/);
});
test('successor wait attaches numerically newer cross-major winner without another drain', async t => {
  const f = fixture(t, 'darwin');
  let live = { pid: 120, version: '0.50.1', started_at: 'old' }, drains = 0, notices = [];
  const deps = { env: env(f), onNotice: text => notices.push(text),
    inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    launch: async () => {}, shutdown: async () => { drains++; live = { pid: 121, version: '1.0.0', started_at: 'winner' }; } };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.equal(drains, 1);
  assert.match(notices.at(-1), /newer daemon won.*update connected plugins/i);
});
test('successor wait recognizes same-PID/start-time endpoint token rotation', async t => {
  const f = fixture(t, 'darwin');
  const root = env(f).WS_CACHE_HOME;
  fs.mkdirSync(path.join(root, 'dashboard'), { recursive: true });
  let token = 'a'.repeat(64), version = '0.50.1', drains = 0, notice;
  const server = http.createServer((req, res) => {
    assert.equal(req.url, '/health');
    res.end(JSON.stringify({ service: 'ws-dashboard', pid: process.pid, version, started_at: 'same' }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(async () => { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); });
  const writeEndpoint = () => fs.writeFileSync(path.join(root, 'dashboard/endpoint.json'),
    JSON.stringify({ pid: process.pid, port: server.address().port, token }));
  writeEndpoint();
  const deps = { env: env(f), onNotice: text => { notice = text; }, launch: async () => {},
    shutdown: async () => { drains++; token = 'b'.repeat(64); version = '1.0.0'; writeEndpoint(); } };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), `http://127.0.0.1:${server.address().port}/`);
  assert.equal(drains, 1);
  assert.match(notice, /newer daemon won.*update connected plugins/i);
});
test('failed preflight leaves incumbent serving, failed drain and late successor failure are distinct', async t => {
  const f = fixture(t, 'darwin');
  let live = { pid: 120, version: '0.50.1', lan: false, started_at: 'old' }, drains = 0, notice;
  const deps = { env: env(f), onNotice: value => { notice = value; },
    inspect: async () => ({ url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    preflight: async () => { throw Error('preflight refused'); },
    shutdown: async () => { drains++; }, sleep: async () => {}, maxPolls: 2 };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.match(notice, /preflight failed/);
  assert.equal(drains, 0);
  assert.equal(live.pid, 120);
  deps.preflight = async () => {};
  deps.shutdown = async () => { drains++; throw Error('no drain'); };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.match(notice, /outcome is uncertain/);
  deps.shutdown = async () => { drains++; live = { ...live, pid: 121, started_at: 'successor pending', version: '0.49.8' }; };
  await assert.rejects(connectDaemon({ ...config(f), replace: true }, deps), /older or unidentified daemon won/);
  assert.equal(drains, 2);
  live = { pid: 120, version: '0.50.1', lan: false, started_at: 'old' };
  deps.shutdown = async () => { drains++; live = { ...live, pid: 120 }; };
  await assert.rejects(connectDaemon({ ...config(f), replace: true }, deps), /predecessor survival is not guaranteed/);
});
test('raced cross-minor winner after staging is never drained; Windows strict starter owns the second guard', async t => {
  const f = fixture(t, 'darwin');
  let checks = 0, notices = [], live = { pid: 120, version: '0.50.0', lan: true, started_at: 'one' };
  const deps = { env: env(f), onNotice: s => notices.push(s),
    inspect: async () => { if (++checks === 2) live = { pid: 123, version: '0.51.0', lan: false, started_at: 'two' };
      return { url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }; },
    launch: () => assert.fail('no executable for raced winner'), shutdown: () => assert.fail('no drain for raced winner') };
  assert.equal(await connectDaemon(config(f), deps), URL);
  assert.equal(checks, 2);
  const windows = fixture(t, 'win32');
  live = { pid: 120, version: '0.50.1', lan: true, started_at: 'one' };
  checks = 0;
  let strictCalls = 0;
  assert.equal(await connectDaemon({ ...config(windows), platform: 'win32', replace: true },
    { ...deps, select: () => windows.select(), inspect: async () => ({ url: URL,
      endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }),
    launch: async (_binary, _opts, args, waitForExit) => {
      strictCalls++;
      assert.deepEqual(args, ['daemon-start', '--require-independent-lifetime', '--start-wait', '13s', '--replace']);
      assert.equal(waitForExit, true);
      // Go refused breakaway before drain; no default child process runs.
      throw Error('breakaway refused');
    } }), URL);
  assert.equal(strictCalls, 1);
  assert.match(notices.at(-1), /did not drain the incumbent/);
});
test('staging race attaches newer but refuses older or unidentified winners without a launch', async t => {
  const f = fixture(t, 'darwin');
  let live = { pid: 120, version: '0.50.1', started_at: 'old' }, checks = 0, notice;
  const deps = { env: env(f), onNotice: text => { notice = text; },
    inspect: async () => { if (++checks === 2) live = { pid: 121, version: '1.0.0', started_at: 'raced' };
      return { url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }; },
    launch: () => assert.fail('raced winner must not launch'), shutdown: () => assert.fail('raced winner must not drain') };
  assert.equal(await connectDaemon({ ...config(f), replace: true }, deps), URL);
  assert.match(notice, /newer daemon won.*update connected plugins/i);
  for (const version of ['0.49.9', 'unknown']) {
    live = { pid: 120, version: '0.50.1', started_at: 'old' }; checks = 0;
    deps.inspect = async () => { if (++checks === 2) live = { pid: 121, version, started_at: 'raced' };
      return { url: URL, endpoint: { pid: live.pid, port: 32000 }, health: { ...live } }; };
    await assert.rejects(connectDaemon({ ...config(f), replace: true }, deps), /older or unidentified daemon won/);
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
test('Windows strict missing start uses only injected starter, then attaches a healthy winner', async t => {
  const f = fixture(t);
  let healthy = false, checks = 0;
  let launches = 0;
  const deps = { env: env(f), discover: async () => { checks++; if (healthy) return URL; throw Error('missing'); },
    launch: async (_binary, _opts, args, waitForExit) => {
      launches++;
      assert.deepEqual(args, ['daemon-start', '--require-independent-lifetime', '--start-wait', '13s']);
      assert.equal(waitForExit, true);
      healthy = true;
    } };
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32' }, deps), URL);
  assert.equal(launches, 1);
  assert.equal(checks, 3);
  assert.equal(await connectDaemon({ ...config(f), platform: 'win32' }, { ...deps,
    select: () => assert.fail('healthy attach never selects'), launch: () => assert.fail('healthy attach never executes') }), URL);
  assert.equal(launches, 1);
});
