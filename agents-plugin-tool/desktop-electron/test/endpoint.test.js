'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const { cacheRoot, readEndpoint, checkHealth, discover, inspect, shutdown, MAX_JSON } = require('../endpoint');

async function fixture(t, handle) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-endpoint-'));
  fs.mkdirSync(path.join(root, 'dashboard'));
  const server = http.createServer(handle);
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const ep = { port: server.address().port, pid: process.pid };
  const file = path.join(root, 'dashboard/endpoint.json');
  const write = value => fs.writeFileSync(file, typeof value === 'string' ? value : JSON.stringify(value));
  write({ ...ep, token: 'a'.repeat(64) });
  t.after(async () => { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); fs.rmSync(root, { recursive: true, force: true }); });
  return { root, ep, file, write };
}
test('cache override, relative root, symlink, default and missing home', t => {
  assert.equal(cacheRoot({ HOME: '/home/user' }, '/'), path.resolve('/home/user', '.cache', 'ws@kang-sw-devenv'));
  assert.equal(cacheRoot({ WS_CACHE_HOME: 'cache' }, '/work'), path.resolve('/work', 'cache'));
  assert.throws(() => cacheRoot({}, '/', () => ''), /Home directory/);
  // No HOME (Windows): fall back to the OS home directory; HOME still wins.
  assert.equal(cacheRoot({}, '/', () => '/users/ki'), path.resolve('/users/ki', '.cache', 'ws@kang-sw-devenv'));
  assert.equal(cacheRoot({ HOME: '/home/user' }, '/', () => '/users/ki'), path.resolve('/home/user', '.cache', 'ws@kang-sw-devenv'));
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'werkschaft-cache-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  try { fs.symlinkSync(root, root + '/link', 'junction'); }
  catch (error) { if (error.code !== 'EPERM') throw error; return; }
  assert.equal(cacheRoot({ WS_CACHE_HOME: root + '/link' }), fs.realpathSync(root));
});
test('identity-only discovery, no credentials, proxy, redirects or writes', async t => {
  let request;
  const f = await fixture(t, (req, res) => { request = req; res.end(JSON.stringify({ service: 'ws-dashboard', pid: process.pid })); });
  const before = fs.readFileSync(f.file);
  assert.deepEqual(readEndpoint(f.root), f.ep);
  assert.equal(await discover(f.root), `http://127.0.0.1:${f.ep.port}/`);
  assert.equal(request.url, '/health');
  assert.equal(request.headers.authorization, undefined);
  assert.equal(request.headers.cookie, undefined);
  assert.deepEqual(fs.readFileSync(f.file), before);
});
test('endpoint parser rejects missing, invalid, range, noninteger and oversized identities without leaking bytes', async t => {
  const f = await fixture(t, (_q, r) => r.end());
  for (const value of ['{NEVER-EXPOSE-THIS', null, {}, { port: 0, pid: 1 }, { port: 65536, pid: 1 }, { port: 12, pid: 0 }, { port: 12, pid: -1 }, { port: 1.5, pid: 1 }, { port: 1, pid: '2' }, { port: 1, pid: 4294967296 }]) {
    f.write(value);
    assert.throws(() => readEndpoint(f.root), error => /Invalid dashboard endpoint/.test(error.message) && !error.message.includes('NEVER'));
  }
  f.write('x'.repeat(MAX_JSON + 1));
  assert.throws(() => readEndpoint(f.root), /too large/);
  fs.unlinkSync(f.file);
  assert.throws(() => readEndpoint(f.root), /No readable/);
});
test('health rejects wrong service, PID, malformed, oversized, non-200 and redirects', async t => {
  let response;
  const f = await fixture(t, (_req, res) => { res.writeHead(response.status || 200, { Location: '/health' }); res.end(response.body); });
  for (const r of [
    { body: '{}' }, { body: JSON.stringify({ service: 'other', pid: process.pid }) },
    { body: JSON.stringify({ service: 'ws-dashboard', pid: process.pid + 1 }) },
    { body: '{' }, { body: 'x'.repeat(MAX_JSON + 1) },
    { status: 500, body: '{}' }, { status: 302, body: '{}' },
  ]) { response = r; await assert.rejects(checkHealth(f.ep), /health|Health/); }
});
test('health deadline is total, including a slow streaming body', async t => {
  const f = await fixture(t, (_req, res) => { res.writeHead(200); res.write('{'); });
  const before = Date.now();
  await assert.rejects(checkHealth(f.ep, 40), /health check failed/);
  assert(Date.now() - before < 1000);
});
test('REST inspection returns validated health without bearer; shutdown alone uses endpoint credential', async t => {
  let requests = [], accepted = false;
  const f = await fixture(t, (req, res) => {
    requests.push({ method: req.method, path: req.url, auth: req.headers.authorization });
    if (req.url === '/health') return res.end(JSON.stringify({ service: 'ws-dashboard', pid: process.pid,
      port: f.ep.port, version: '0.50.1', lan: true, started_at: 'start-a', content_hash: 'hash-a' }));
    if (req.url === '/shutdown' && req.method === 'POST' && req.headers.authorization === `Bearer ${'a'.repeat(64)}`) {
      accepted = true;
      res.writeHead(202); return res.end(JSON.stringify({ shutting_down: true }));
    }
    res.writeHead(401); res.end('{}');
  });
  const observed = await inspect(f.root);
  assert.equal(observed.health.version, '0.50.1');
  assert.equal(observed.health.lan, true);
  assert(!JSON.stringify(observed).includes('a'.repeat(64)));
  assert.deepEqual(requests.map(r => r.auth), [undefined]);
  await shutdown(f.root, observed);
  assert(accepted);
  assert.deepEqual(requests.map(r => r.path), ['/health', '/health', '/shutdown']);
  assert.deepEqual(requests.map(r => r.auth), [undefined, undefined, `Bearer ${'a'.repeat(64)}`]);
});
test('changed endpoint/identity prevents authenticated drain, no credentials in errors', async t => {
  let drains = 0;
  const f = await fixture(t, (req, res) => {
    if (req.url === '/shutdown') drains++;
    res.end(JSON.stringify({ service: 'ws-dashboard', pid: process.pid, started_at: 'first' }));
  });
  const seen = await inspect(f.root);
  f.write({ ...f.ep, token: 'b'.repeat(64) });
  await assert.rejects(shutdown(f.root, seen), error => !error.message.includes('b'.repeat(64)) && /changed/.test(error.message));
  assert.equal(drains, 0);
});
test('endpoint change during health is rejected, missing then retry succeeds', async t => {
  let change = true;
  const f = await fixture(t, (_req, res) => {
    if (change) f.write({ ...f.ep, pid: process.pid + 1 });
    res.end(JSON.stringify({ service: 'ws-dashboard', pid: process.pid }));
  });
  await assert.rejects(discover(f.root), /endpoint changed/);
  fs.unlinkSync(f.file);
  await assert.rejects(discover(f.root), /No readable/);
  change = false;
  f.write(f.ep);
  assert.equal(await discover(f.root), `http://127.0.0.1:${f.ep.port}/`);
});
