// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/webframe.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../assets/webframe.js'), 'utf8');
const HOST = 'http://localhost:4242', PROXY = 'http://localhost:5555';

// The wrapper frame between the card (its parent) and the proxied page
// (its inner frame #f), with both windows recording what they receive.
function boot({ beat = null, fetch } = {}) {
  const up = [], down = [], listeners = [], intervals = [];
  // Posts are copied out of the script's realm so deepEqual compares plain data.
  const copy = m => JSON.parse(JSON.stringify(m));
  const parent = { postMessage(m, origin) { up.push({ m: copy(m), origin }); } };
  const innerWin = { postMessage(m, origin) { down.push({ m: copy(m), origin }); } };
  const attrs = { 'data-proxy': PROXY, 'data-beat': beat };
  const ctx = {
    document: { documentElement: { getAttribute: n => attrs[n] ?? null }, getElementById: id => id === 'f' ? { contentWindow: innerWin } : null },
    location: { origin: HOST }, parent, fetch,
    setInterval: fn => intervals.push(fn),
    addEventListener: (type, fn) => listeners.push({ type, fn }),
  };
  ctx.window = ctx;
  vm.createContext(ctx);
  vm.runInContext(source, ctx);
  const message = (source, origin, data) => { for (const l of listeners) if (l.type === 'message') l.fn({ source, origin, data }); };
  return { ctx, parent, innerWin, up, down, intervals, message };
}

test('the wrapper posts frame once to its own origin', () => {
  const w = boot();
  assert.deepEqual(w.up, [{ m: { wsWeb: 'frame', proxy: PROXY }, origin: HOST }]);
  assert.deepEqual(w.intervals, [], 'no heartbeat without a beat path');
});

test('only an inner-frame message from the proxy origin is relayed up, to the wrapper origin', () => {
  const w = boot();
  w.up.length = 0;
  w.message(w.innerWin, PROXY, { wsWeb: 'pick', selector: '#a' });
  assert.deepEqual(w.up, [{ m: { wsWeb: 'pick', selector: '#a' }, origin: HOST }]);
  w.up.length = 0;
  w.message(w.innerWin, HOST, { wsWeb: 'pick' });
  w.message(w.innerWin, 'http://localhost:6666', { wsWeb: 'pick' });
  w.message({}, PROXY, { wsWeb: 'pick' });
  w.message(w.parent, PROXY, { wsWeb: 'pick' });
  assert.deepEqual(w.up, []);
  assert.deepEqual(w.down, []);
});

test('the page cannot post the wrapper\'s own types through the relay', () => {
  const w = boot();
  w.up.length = 0;
  for (const type of ['frame', 'gone', 'hasOwnProperty', '__proto__', 'other']) {
    w.message(w.innerWin, PROXY, { wsWeb: type, proxy: 'http://evil.example' });
  }
  assert.deepEqual(w.up, []);
  for (const type of ['ready', 'pick', 'picking', 'errors', 'console']) w.message(w.innerWin, PROXY, { wsWeb: type });
  assert.deepEqual(w.up.map(u => u.m.wsWeb), ['ready', 'pick', 'picking', 'errors', 'console']);
});

test('only a parent message from the wrapper origin is relayed down, to the proxy origin', () => {
  const w = boot();
  w.up.length = 0;
  w.message(w.parent, HOST, { wsWeb: 'attach' });
  assert.deepEqual(w.down, [{ m: { wsWeb: 'attach' }, origin: PROXY }]);
  w.down.length = 0;
  w.message(w.parent, 'http://localhost:6666', { wsWeb: 'attach' });
  w.message(w.parent, PROXY, { wsWeb: 'attach' });
  w.message({}, HOST, { wsWeb: 'attach' });
  w.message(w.innerWin, HOST, { wsWeb: 'attach' });
  assert.deepEqual(w.down, []);
  assert.deepEqual(w.up, []);
});

test('data that is not a wsWeb message is dropped both ways', () => {
  const w = boot();
  w.up.length = 0;
  for (const data of ['attach', null, 7, { type: 'attach' }, { wsWeb: 1 }]) {
    w.message(w.parent, HOST, data);
    w.message(w.innerWin, PROXY, data);
  }
  assert.deepEqual(w.up, []);
  assert.deepEqual(w.down, []);
});

test('the heartbeat posts gone when the registration is unknown', async () => {
  const calls = [];
  const w = boot({ beat: '/s/r/web/id/beat', fetch: (url, opts) => { calls.push([url, opts.method]); return Promise.resolve({ status: 404 }); } });
  assert.equal(w.intervals.length, 1);
  w.up.length = 0;
  w.intervals[0]();
  await new Promise(r => setImmediate(r));
  assert.deepEqual(calls, [['/s/r/web/id/beat', 'POST']]);
  assert.deepEqual(w.up, [{ m: { wsWeb: 'gone' }, origin: HOST }]);
});
