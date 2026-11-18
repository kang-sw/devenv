// Offline serve contract: extract the production function without importing main().
// No daemon, Go command, network request, timer or temporary file is started.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { EventEmitter } = require('node:events');
const source = fs.readFileSync(path.resolve(__dirname, '../board-headless.mjs'), 'utf8');
const start = source.indexOf('async function serve(args) {');
const end = source.indexOf('// --- page helpers', start);
assert.ok(start >= 0 && end > start, 'missing production serve function');
const serveSource = source.slice(start, end);

function fixture({ platform = 'win32', authStatus = 200, missingLog = false } = {}) {
  const events = [], errors = [], logs = [], writes = [], exits = [], handlers = {};
  const tmp = '/isolated/board-headless-test', base = 'http://127.0.0.1:23456';
  const daemon = new EventEmitter(); daemon.pid = 1234; daemon.exitCode = null;
  daemon.kill = signal => {
    events.push(['kill', signal]); daemon.exitCode = 0;
    queueMicrotask(() => daemon.emit('exit', 0)); return true;
  };
  const context = vm.createContext({
    path: path.posix, os: { tmpdir: () => '/isolated' }, TOOL_DIR: '/repo/agents-plugin-tool',
    fs: {
      mkdtempSync(prefix) { assert.equal(prefix, '/isolated/board-headless-'); return tmp; },
      openSync(file, mode) { assert.equal(file, tmp + '/daemon.log'); assert.equal(mode, 'a'); return 99; },
      readFileSync(file, encoding) {
        assert.equal(encoding, 'utf8');
        if (file.endsWith('/endpoint.json')) return JSON.stringify({ pid: daemon.pid, port: 23456, token: 'isolated-machine-token' });
        assert.equal(file, tmp + '/daemon.log');
        if (missingLog) throw Error('log unavailable');
        return Array.from({ length: 25 }, (_, i) => 'daemon-line-' + (i + 1)).join('\r\n') + '\r\n';
      },
      mkdirSync() {},
      writeFileSync(file, text) { writes.push([file, JSON.parse(text)]); },
      rmSync(file, options) { assert.equal(file, tmp); assert.equal(options.recursive, true); events.push(['remove-temp']); }
    },
    process: { platform, on(name, fn) { handlers[name] = fn; }, exit(code) { exits.push(code); } },
    console: { error(message) { errors.push(message); } }, log(message) { logs.push(message); },
    childEnv(dir, extra) { assert.equal(dir, tmp); return { isolated: true, ...extra }; },
    run(command, args, options) { events.push(['build', command, Array.from(args), options]); },
    seedSession(dir, bin, kind, topics) {
      assert.equal(dir, tmp); events.push(['seed', bin, kind, topics]);
      return { rid: kind + '-rid', root: '/project/' + kind, standalone: 'file:///project/' + kind + '/index.html' };
    },
    spawn(bin, args, options) { events.push(['spawn', bin, Array.from(args), options]); return daemon; },
    async waitFor(fn, timeout, what) {
      events.push(['wait', what, timeout]);
      const value = await fn(); if (!value) throw Error('timed out waiting for ' + what); return value;
    },
    async fetch(url, options) {
      events.push(['fetch', url, options]);
      assert.ok(url.startsWith(base));
      if (url === base + '/health') return { ok: true };
      if (url === base + '/auth/cookie') return {
        ok: authStatus === 200, status: authStatus,
        json: async () => ({ name: 'ws_machine', value: 'signed-cookie' }),
        text: async () => 'login_required'
      };
      if (url === base + '/nudge') return { status: 204 };
      assert.match(url, /\/s\/(adapter|board)-rid\?frame=1$/);
      const authenticated = options && options.headers && options.headers.Cookie === 'ws_machine=signed-cookie';
      return { ok: authenticated, text: async () => url.includes('/adapter-rid') ? '<nav id="tabbar"></nav>' : '<main>board</main>' };
    },
    setInterval(fn, delay) { events.push(['interval', delay]); return 'heartbeat'; },
    clearInterval(id) { events.push(['clear-interval', id]); },
    setTimeout() { return 'kill-timeout'; }, clearTimeout() {}
  });
  vm.runInContext(serveSource, context);
  return { serve: args => context.serve(args), events, errors, logs, writes, exits, handlers, tmp, base };
}

for (const platform of ['win32', 'linux']) test('serve uses ' + platform + ' binary and machine-cookie-authenticated readiness', async () => {
  const f = fixture({ platform }); await f.serve({ topics: 3 });
  const expectedBin = f.tmp + (platform === 'win32' ? '/ws-mcp.exe' : '/ws-mcp');
  const build = f.events.find(e => e[0] === 'build');
  assert.deepEqual(build.slice(1, 3), ['go', ['build', '-o', expectedBin, './cmd/ws-mcp']]);
  assert.equal(build[3].cwd, '/repo/agents-plugin-tool'); assert.equal(build[3].env.isolated, true);
  const seeds = f.events.filter(e => e[0] === 'seed');
  assert.deepEqual(seeds.map(e => e.slice(1)), [[expectedBin, 'adapter', 3], [expectedBin, 'board', 3]]);
  const spawn = f.events.find(e => e[0] === 'spawn');
  assert.deepEqual(spawn.slice(1, 3), [expectedBin, ['dashboard', '--port', '0']]);
  assert.equal(spawn[3].env.WS_MCP_DASHBOARD_DEV_SOURCE, undefined, 'embedded assets are the default');
  assert.deepEqual(Array.from(spawn[3].stdio), ['ignore', 99, 99]);
  const requests = f.events.filter(e => e[0] === 'fetch');
  assert.deepEqual(requests.map(e => e[1].slice(f.base.length)), ['/health', '/auth/cookie', '/nudge', '/nudge', '/s/adapter-rid?frame=1', '/s/board-rid?frame=1']);
  assert.equal(requests[0][2], undefined, 'health is exempt');
  assert.equal(requests[1][2].method, 'POST');
  assert.equal(requests[1][2].headers.Authorization, 'Bearer isolated-machine-token');
  for (const request of requests.slice(2, 4)) {
    assert.equal(request[2].method, 'POST'); assert.equal(request[2].headers, undefined, 'nudge remains exempt');
  }
  for (const request of requests.slice(4)) {
    assert.equal(request[2].headers.Cookie, 'ws_machine=signed-cookie');
    assert.equal(request[2].headers.Authorization, undefined, 'page readiness uses minted cookie, not endpoint bearer');
  }
  assert.deepEqual(f.errors, []); assert.deepEqual(f.exits, []);
  assert.equal(f.writes.length, 3, 'two registry rows and one adapter snapshot');
  assert.ok(f.logs.includes('adapter page: ' + f.base + '/s/adapter-rid'));
  assert.ok(f.logs.includes('board page:   ' + f.base + '/s/board-rid'));
  f.handlers.SIGINT(); await Promise.resolve();
  assert.deepEqual(f.exits, [0]);
  assert.ok(f.events.some(e => e[0] === 'kill' && e[1] === 'SIGTERM'));
  assert.ok(f.events.some(e => e[0] === 'clear-interval' && e[1] === 'heartbeat'));
  assert.ok(f.events.some(e => e[0] === 'remove-temp'));
});

test('serve --dev-assets opts in to live dashboard assets', async () => {
  const f = fixture(); await f.serve({ 'dev-assets': true });
  const spawn = f.events.find(e => e[0] === 'spawn');
  assert.equal(spawn[3].env.WS_MCP_DASHBOARD_DEV_SOURCE, '/repo/agents-plugin-tool');
  f.handlers.SIGINT(); await Promise.resolve();
});

test('cookie refusal reports HTTP/body plus only the last 20 daemon log lines and stops before readiness', async () => {
  const f = fixture({ authStatus: 401 }); await f.serve({}); await Promise.resolve();
  assert.deepEqual(f.events.filter(e => e[0] === 'fetch').map(e => e[1].slice(f.base.length)), ['/health', '/auth/cookie']);
  assert.equal(f.errors[0], 'board-headless serve: localhost cookie: HTTP 401: login_required');
  assert.equal(f.errors[1], Array.from({ length: 20 }, (_, i) => 'daemon-line-' + (i + 6)).join('\n'));
  assert.equal(f.errors.length, 2); assert.deepEqual(f.exits, [1]);
  assert.equal(f.writes.length, 0); assert.equal(f.events.some(e => e[0] === 'interval'), false);
  assert.ok(f.events.some(e => e[0] === 'kill' && e[1] === 'SIGTERM'));
  assert.ok(f.events.some(e => e[0] === 'remove-temp'));
});

test('unavailable daemon log does not obscure cookie failure or skip error cleanup', async () => {
  const f = fixture({ authStatus: 403, missingLog: true }); await f.serve({ 'keep-tmp': true }); await Promise.resolve();
  assert.deepEqual(f.errors, ['board-headless serve: localhost cookie: HTTP 403: login_required']);
  assert.deepEqual(f.exits, [1]); assert.ok(f.logs.includes('kept ' + f.tmp));
  assert.equal(f.events.some(e => e[0] === 'remove-temp'), false);
});
