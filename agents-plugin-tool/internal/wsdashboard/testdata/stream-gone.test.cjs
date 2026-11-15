const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const source = fs.readFileSync(path.join(__dirname, '../assets/dashboard.js'), 'utf8');
const stream = source.match(/\n  var sseOn = true;\n[\s\S]*?\n  \/\/ showRegions applies/)[0];

function fixture(framed = false) {
  let now = 0;
  const timers = [], sockets = [], probes = [], banners = [], swaps = [];
  const drafts = { composer: 'unsent', answers: ['choice'], images: ['image'], notes: ['note'] };
  const dom = { html: '<p>last good board</p>', drafts };
  const ctx = {
    shell: false, framed, mode: 'session', rid: 'rid1', heldNow: 'board.1',
    window: {}, location: { protocol: 'http:', host: 'isolated.invalid' },
    q: () => [], wsT: k => k, trackerLost() {},
    held: () => ctx.heldNow, banner: text => banners.push(text),
    pageHidden: () => false, swap: (...args) => swaps.push(args),
    boardInputFull: () => null,
    setTimeout(fn, delay) { timers.push({ at: now + delay, fn }); },
    fetch(url, options) {
      let resolve, reject;
      const promise = new Promise((a, b) => { resolve = a; reject = b; });
      probes.push({ url, options, resolve, reject, cancelled: false });
      return promise;
    },
    WebSocket: function (url) {
      this.url = url;
      this.close = () => { this.closed = true; if (this.onclose) this.onclose(); };
      this.fail = () => { this.onerror(); this.onclose(); };
      this.msg = event => this.onmessage({ data: JSON.stringify({ event, data: {} }) });
      sockets.push(this);
    }
  };
  ctx.window.WebSocket = ctx.WebSocket;
  vm.runInNewContext(stream, ctx);
  ctx.connect();
  return {
    ctx, sockets, probes, banners, swaps, dom, timers,
    advance(ms) {
      now += ms;
      for (;;) {
        const index = timers.findIndex(t => t.at <= now);
        if (index < 0) break;
        timers.splice(index, 1)[0].fn();
      }
    },
    async reply(status) {
      const probe = probes.at(-1);
      probe.resolve({ status, ok: status >= 200 && status < 300,
        body: { cancel() { probe.cancelled = true; return Promise.resolve(); } } });
      await this.flush();
    },
    async flush() { for (let i = 0; i < 5; i++) await Promise.resolve(); }
  };
}

for (const framed of [false, true]) {
  test(`gone waits, probes once and recovers with current hashes (frame=${framed})`, async () => {
    const f = fixture(framed);
    const original = JSON.stringify(f.dom);
    f.sockets[0].msg('gone');
    f.sockets[0].fail();
    f.sockets[0].msg('gone');
    assert.equal(f.sockets[0].closed, true);
    assert.equal(f.ctx.sseOn, false);
    assert.deepEqual(f.banners, []);
    f.advance(2999);
    assert.equal(f.probes.length, 0);
    f.advance(1);
    assert.equal(f.probes.length, 1);
    assert.equal(f.probes[0].url, '/s/rid1/events' + (framed ? '?frame=1' : ''));
    assert.equal(f.probes[0].options.cache, 'no-store');
    f.ctx.heldNow = 'board.9,head.4';
    await f.reply(200);
    assert.equal(f.probes[0].cancelled, true);
    assert.equal(f.sockets.length, 2);
    assert.match(f.sockets[1].url, /h=board.9%2Chead.4/);
    assert.equal(f.sockets[1].url.includes('&frame=1'), framed);
    f.sockets[1].onopen();
    assert.equal(f.ctx.sseOn, true);
    f.advance(30000);
    assert.equal(f.sockets.length, 2);
    assert.equal(f.probes.length, 1);
    assert.deepEqual(f.banners, []);
    assert.deepEqual(f.swaps, []);
    assert.equal(JSON.stringify(f.dom), original);
  });
}

test('confirmed 404 banners once and never reconnects', async () => {
  const f = fixture();
  f.sockets[0].msg('gone');
  f.advance(3000);
  await f.reply(404);
  f.advance(60000);
  assert.deepEqual(f.banners, ['js.banner.gone']);
  assert.equal(f.probes.length, 1);
  assert.equal(f.probes[0].cancelled, true);
  assert.equal(f.sockets.length, 1);
  assert.equal(f.ctx.sseOn, false);
  assert.equal(f.dom.drafts.composer, 'unsent');
});

for (const outcome of ['network', 503, 500, 403]) {
  test(`probe ${outcome} retries without a gone banner, later absence terminates`, async () => {
    const f = fixture();
    f.sockets[0].msg('gone');
    f.advance(3000);
    if (outcome === 'network') {
      f.probes[0].reject(new Error('offline'));
      await f.flush();
    } else await f.reply(outcome);
    assert.deepEqual(f.banners, []);
    assert.equal(f.timers.length, 1);
    f.advance(2999);
    assert.equal(f.probes.length, 1);
    f.advance(1);
    assert.equal(f.probes.length, 2);
    await f.reply(404);
    f.advance(60000);
    assert.deepEqual(f.banners, ['js.banner.gone']);
    assert.equal(f.sockets.length, 1);
    assert.equal(f.probes.length, 2);
  });
}

test('503 probe can recover, and ordinary WebSocket errors still retry once', async () => {
  const f = fixture();
  f.sockets[0].msg('gone');
  f.advance(3000);
  await f.reply(503);
  f.advance(3000);
  await f.reply(200);
  assert.equal(f.sockets.length, 2);
  f.sockets[1].fail();
  assert.equal(f.timers.length, 1);
  f.advance(3000);
  assert.equal(f.sockets.length, 3);
  assert.equal(f.probes.length, 2);
  assert.deepEqual(f.banners, []);
});
