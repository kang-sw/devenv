const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');

const source = readAssetSource('wsdashboard', 'dashboard-composer-delivery.js');
const section = source.match(/\n  \/\/ --- outbox [^]*?\n  function outboxSync\([^]*?\n  \}\n/)[0];

function fixture() {
  function el() {
    const node = { children: [], parentNode: null, attrs: {}, className: '',
      setAttribute(k, v) { this.attrs[k] = v; }, getAttribute(k) { return this.attrs[k]; },
      appendChild(child) { return this.insertBefore(child, null); },
      insertBefore(child, before) { if (child.parentNode) child.parentNode.removeChild(child); const i = this.children.indexOf(before); this.children.splice(i < 0 ? this.children.length : i, 0, child); child.parentNode = this; return child; },
      removeChild(child) { this.children.splice(this.children.indexOf(child), 1); child.parentNode = null; },
      addEventListener() {}, classList: { contains: name => node.className.split(' ').includes(name) } };
    Object.defineProperty(node, 'firstChild', { get() { return this.children[0]; } });
    return node;
  }
  let live = el(), user = '', now = 0, nextTimer = 0;
  const timers = new Map(), released = [];
  const context = vm.createContext({
    doc: { createElement: el, querySelector: s => s === '#r-convlive .clive' ? live : null },
    q: () => user ? [{ getAttribute: () => user }] : [],
    ctext: { value: '', closest: () => null }, imgs: [], imgTrash: {}, imgSeq: 0,
    Date: { now: () => now }, Math, setTimeout: (fn, ms) => { const id = ++nextTimer; timers.set(id, { fn, at: now + ms }); return id; },
    clearTimeout: id => timers.delete(id), release: p => released.push(p),
    saveDraft: () => Promise.resolve(true), followConvo() {}, boardFlushViewed() {}, chipList: () => [], topicNotes: () => [],
    wsT: x => x, boardOwn: {}, draftDetached: false, noteWarn() {}, window: {},
  });
  vm.runInContext(section, context);
  function message() { return { body: {}, pics: [], own: [], user, back: { text: 'hello' }, row: el() }; }
  return { context, el, message, released, get live() { return live; }, replaceLive() { live = el(); },
    setUser(value) { user = value; }, advance(ms) { now += ms; for (const [id, timer] of [...timers]) if (timer.at <= now) { timers.delete(id); timer.fn(); } }, timers };
}

test('optimistic acknowledgement expires without another render', () => {
  const f = fixture(), m = f.message(); m.pics.push({ n: 1 }); f.context.outbox.push(m);
  f.context.outboxAcked(m, 'held');
  assert.equal(f.live.children.length, 1);
  f.advance(29999); assert.equal(f.live.children.length, 1);
  f.advance(1); assert.equal(f.live.children.length, 0); assert.equal(f.context.outbox.length, 0); assert.equal(f.released.length, 1);
});

test('server render and delivery before acknowledgement prevent a duplicate', () => {
  for (const mode of ['held', 'delivered', 'empty']) {
    const f = fixture(), m = f.message(); m.live = f.live; f.context.outbox.push(m);
    if (mode === 'held') { const held = f.el(); held.className = 'held'; held.setAttribute('data-entry', 'held'); f.live.appendChild(held); }
    if (mode === 'delivered') f.setUser('new-entry');
    if (mode === 'empty') f.replaceLive();
    f.context.outboxAcked(m, 'held');
    assert.equal(f.context.outbox.length, 0, mode);
    assert.equal(f.live.children.length, mode === 'held' ? 1 : 0, mode);
    assert.equal(f.timers.size, 0, mode);
  }
});

test('invalid acknowledgement ids retain an unknown recovery rather than an empty bubble', async () => {
  for (const response of [{}, { id: '' }, { id: '  ' }, { id: 'invalid' }, { id: {} }, { id: 42 }]) {
    const f = fixture(), m = f.message(); m.recoveryState = 'queued'; f.context.outbox.push(m);
    f.context.post = (route, body, done) => done(true, response);
    f.context.showRecoveredDeliveries = () => {};
    f.context.outboxPump();
    await Promise.resolve();
    assert.equal(m.paused, true); assert.equal(m.recoveryState, 'unknown');
    assert.equal(f.live.children.length, 0); assert.equal(f.context.outbox.length, 1);
  }
});
