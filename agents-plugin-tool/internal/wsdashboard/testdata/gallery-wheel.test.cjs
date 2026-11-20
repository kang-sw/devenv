// Gallery model: bounce-snap gate, spring-loaded hover, and rail wheel timing,
// scripted with explicit timestamps.
// Run: node --test testdata/gallery-wheel.test.cjs (from internal/wsdashboard)
const test = require('node:test');
const assert = require('node:assert/strict');
const G = require('../assets/gallery.js');
const C = G.C;

// At rest on the bottom boundary with a page past it.
const atBottom = (dy, now, o = {}) => Object.assign({ dy, now, canUp: true, canDown: false, prev: true, next: true }, o);

test('constants are the prototype v4 values', () => {
  assert.deepEqual(
    [C.SNAP, C.TENSION_MAX, C.RUBBER_PX, C.READY, C.IDLE_MS, C.INERTIA_MS, C.MICRO_PX, C.MICRO_MS, C.LOCK_MS, C.OVERSHOOT_PX, C.SPRING_MS, C.RAIL_WHEEL_MS, C.EPS, C.WIDE, C.SLOT],
    [320, 125, 26, 0.85, 240, 170, 5, 220, 420, 28, 300, 140, 2, 1600, 2200]);
});

test('tension accumulates min(125, |dy|) and snaps at 320 (100%)', () => {
  const g = new G.Gate();
  let r = g.wheel(atBottom(100, 0));
  assert.equal(r.kind, 'pull');
  assert.equal(r.pct, 100 / 320);
  r = g.wheel(atBottom(400, 50)); // capped at 125
  assert.equal(r.pct, 225 / 320);
  assert.equal(r.ready, false);
  r = g.wheel(atBottom(50, 100)); // 275 / 320 = 86%: ready from 85%
  assert.equal(r.kind, 'pull');
  assert.equal(r.ready, true);
  r = g.wheel(atBottom(45, 150)); // 320
  assert.deepEqual(r, { kind: 'flip', dir: 1 });
});

test('rubber band is 26 * (1 - exp(-2.3 * pct)), below 26px', () => {
  assert.equal(G.rubber(0), 0);
  assert.ok(Math.abs(G.rubber(1) - 26 * (1 - Math.exp(-2.3))) < 1e-9);
  assert.ok(G.rubber(1) < 26);
  const g = new G.Gate();
  const r = g.wheel(atBottom(160, 0));
  assert.ok(Math.abs(r.shift - G.rubber(125 / 320)) < 1e-9);
});

test('a pull idle 240ms below 100% springs back and resets tension', () => {
  const g = new G.Gate();
  g.wheel(atBottom(125, 0));
  assert.equal(g.due(), 240);
  assert.equal(g.tick(239), null);
  assert.equal(g.tick(240), 'spring');
  const r = g.wheel(atBottom(125, 500));
  assert.equal(r.pct, 125 / 320); // started over
});

test('continued wheel inside 240ms keeps the tension', () => {
  const g = new G.Gate();
  g.wheel(atBottom(125, 0));
  g.wheel(atBottom(125, 230));
  assert.equal(g.tick(400), null);
  assert.deepEqual(g.wheel(atBottom(125, 460)), { kind: 'flip', dir: 1 });
});

test('inertia guard: a boundary event within 170ms of an inner scroll is a micro-bounce', () => {
  const g = new G.Gate();
  assert.deepEqual(g.wheel(atBottom(80, 0, { canDown: true })), { kind: 'pass', reset: false });
  let r = g.wheel(atBottom(80, 16));
  assert.deepEqual(r, { kind: 'micro', dir: 1, shift: 5, edge: false });
  r = g.wheel(atBottom(80, 180)); // 164ms after the last: still momentum
  assert.equal(r.kind, 'micro');
  assert.equal(g.due(), 180 + 220);
  assert.equal(g.tick(399), null);
  assert.equal(g.tick(400), 'clear');
  r = g.wheel(atBottom(80, 400)); // 220ms gap >= 170: at rest, tension charges
  assert.equal(r.kind, 'pull');
  assert.equal(r.pct, 80 / 320);
});

test('inner scrolling uses a pass and resets a showing pull', () => {
  const g = new G.Gate();
  g.wheel(atBottom(100, 0));
  assert.deepEqual(g.wheel(atBottom(-30, 10, { canUp: true })), { kind: 'pass', reset: true });
  assert.equal(g.due(), null);
});

test('refractory 420ms after a flip ignores wheel events', () => {
  const g = new G.Gate();
  g.wheel(atBottom(125, 0)); g.wheel(atBottom(125, 10));
  assert.equal(g.wheel(atBottom(125, 20)).kind, 'flip');
  assert.equal(g.wheel(atBottom(125, 100)).kind, 'ignore');
  assert.equal(g.wheel(atBottom(125, 439)).kind, 'ignore');
  assert.equal(g.wheel(atBottom(125, 440)).kind, 'pull');
});

test('a reversed direction resets the tension', () => {
  const g = new G.Gate();
  g.wheel(atBottom(125, 0));
  g.wheel(atBottom(125, 10));
  assert.deepEqual(g.wheel(atBottom(-40, 20, { canUp: false })), { kind: 'reset' });
  const r = g.wheel(atBottom(-125, 30, { canUp: false }));
  assert.equal(r.kind, 'pull');
  assert.equal(r.dir, -1);
  assert.equal(r.pct, 125 / 320);
});

test('an edge with no page past it is a micro-bounce that never charges or flips', () => {
  const g = new G.Gate();
  for (let t = 0; t < 2000; t += 30) {
    const r = g.wheel(atBottom(125, t, { next: false }));
    assert.deepEqual(r, { kind: 'micro', dir: 1, shift: 5, edge: true });
  }
  assert.equal(g.acc, 0);
  const top = g.wheel(atBottom(-125, 5000, { canUp: false, prev: false }));
  assert.equal(top.kind, 'micro');
  assert.equal(top.edge, true);
});

test('an inert gate (null neighbor) leaves the event alone', () => {
  const g = new G.Gate();
  assert.deepEqual(g.wheel(atBottom(125, 0, { next: null })), { kind: 'none' });
  assert.equal(g.due(), null);
  assert.deepEqual(g.wheel(atBottom(0.5, 10)), { kind: 'none' });
});

test('a custom snap threshold', () => {
  const g = new G.Gate({ snap: 200 });
  g.wheel(atBottom(125, 0));
  assert.equal(g.wheel(atBottom(75, 10)).kind, 'flip');
});

test('spring-loaded hover fires after 300ms on the same target', () => {
  const h = new G.Hover();
  assert.equal(h.over('p1', 0), true);
  assert.equal(h.over('p1', 100), false); // same target keeps its start
  assert.equal(h.fire(299), null);
  assert.equal(h.fire(300), 'p1');
  assert.equal(h.fire(400), null); // fired once
  h.over('p1', 1000);
  h.over('p2', 1200); // moving to another dot restarts
  assert.equal(h.fire(1300), null);
  assert.equal(h.fire(1500), 'p2');
});

test('a drop on + before 300ms finds the hover unfired (the drop creates the page)', () => {
  const h = new G.Hover();
  h.over('+', 0);
  assert.equal(h.fire(250), null);
  h.clear();
  assert.equal(h.fire(400), null);
});

test('rail wheel: one tick per page, debounced 140ms', () => {
  assert.equal(G.railStep(-Infinity, 0), true);
  assert.equal(G.railStep(0, 139), false);
  assert.equal(G.railStep(0, 140), true);
});
