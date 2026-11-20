// Gallery model: storage migration, reconcile, ordinals, positions, layout.
// Run: node --test testdata/gallery-state.test.cjs (from internal/wsdashboard)
const test = require('node:test');
const assert = require('node:assert/strict');
const G = require('../assets/gallery.js');

const pagesOf = g => g.pages.map(p => G.leaves(p.root));

test('flat record migrates one pin per page in ascending pin order', () => {
  const raw = {
    cards: [
      { id: 'cA', kind: 'terminal', pinned: 30 },
      { id: 'cB', kind: 'web', pinned: 0 },
      { id: 'cC', kind: 'explorer', pinned: 10 },
    ],
    topics: [{ n: 'alpha', p: 20 }, { n: 'beta', p: 0, a: 5 }],
    bw: 700,
  };
  const g = G.load(raw);
  assert.deepEqual(pagesOf(g), [['c:cC'], ['t:alpha'], ['c:cA']]);
  assert.equal(g.v, 1);
  assert.equal(g.active, null);
  assert.equal(new Set(g.pages.map(p => p.id)).size, 3);
});

test('bare-array legacy record (cards only) migrates', () => {
  const g = G.load([{ id: 'x', kind: 'web', pinned: 2 }, { id: 'y', kind: 'web', pinned: 1 }, { id: 'z', kind: 'web' }]);
  assert.deepEqual(pagesOf(g), [['c:y'], ['c:x']]);
});

test('cardOk filters cards of unknown kinds out of the flat pins', () => {
  const g = G.load({ cards: [{ id: 'a', kind: 'gone', pinned: 1 }, { id: 'b', kind: 'web', pinned: 2 }] }, c => c.kind === 'web');
  assert.deepEqual(pagesOf(g), [['c:b']]);
});

test('empty or junk records give an empty gallery', () => {
  for (const raw of [null, undefined, 'x', 3, {}, { gallery: 'nope' }, { gallery: { pages: 'x' } }]) {
    const g = G.load(raw);
    assert.deepEqual(g.pages, []);
    assert.equal(g.active, null);
  }
});

const stored = () => ({
  v: 1, active: 'p2', rev: 7,
  pages: [
    { id: 'p1', root: { topic: 'alpha' } },
    { id: 'p2', root: { axis: 'row', ratio: 0.3, a: { card: 'cA' }, b: { axis: 'col', ratio: 0.6, a: { topic: 'beta' }, b: { card: 'cB' } } } },
  ],
});

test('gallery agreeing with the flat flags loads unchanged', () => {
  const g = G.reconcile(stored(), [
    { ref: 't:alpha', p: 1 }, { ref: 'c:cA', p: 2 }, { ref: 't:beta', p: 3 }, { ref: 'c:cB', p: 4 },
  ]);
  assert.deepEqual(G.record(g), stored());
});

test('reconcile: a cleared pin leaves its leaf and collapses its split', () => {
  const g = G.reconcile(stored(), [{ ref: 't:alpha', p: 1 }, { ref: 'c:cA', p: 2 }, { ref: 'c:cB', p: 4 }]);
  assert.deepEqual(g.pages[1].root, { axis: 'row', ratio: 0.3, a: { card: 'cA' }, b: { card: 'cB' } });
});

test('reconcile: a deleted item that emptied its page removes the page', () => {
  const g = G.reconcile(stored(), [{ ref: 'c:cA', p: 2 }, { ref: 't:beta', p: 3 }, { ref: 'c:cB', p: 4 }]);
  assert.deepEqual(g.pages.map(p => p.id), ['p2']);
  assert.equal(g.active, 'p2');
});

test('reconcile: the active page gone falls back to home', () => {
  const g = G.reconcile(stored(), [{ ref: 't:alpha', p: 1 }]);
  assert.deepEqual(pagesOf(g), [['t:alpha']]);
  assert.equal(g.active, null);
});

test('reconcile: newly flat-pinned items append as pages in ascending pin order', () => {
  const g = G.reconcile(stored(), [
    { ref: 't:alpha', p: 1 }, { ref: 'c:cA', p: 2 }, { ref: 't:beta', p: 3 }, { ref: 'c:cB', p: 4 },
    { ref: 'c:new2', p: 900 }, { ref: 't:new1', p: 500 },
  ]);
  assert.deepEqual(pagesOf(g).slice(2), [['t:new1'], ['c:new2']]);
  assert.ok(g.pages.slice(2).every(p => !['p1', 'p2'].includes(p.id)));
  assert.ok(g.rev > 7);
});

test('reconcile drops malformed nodes, duplicate refs, and duplicate page ids; clamps ratios', () => {
  const g = G.reconcile({
    pages: [
      { id: 'p1', root: { axis: 'row', ratio: 7, a: { card: 'a' }, b: { card: 'b' } } },
      { id: 'p1', root: { card: 'c' } },
      { id: 'p3', root: { axis: 'col', ratio: 'x', a: { card: 'a' }, b: { bogus: 1 } } },
      null,
      { root: { card: 'c' } },
    ],
  }, [{ ref: 'c:a', p: 1 }, { ref: 'c:b', p: 2 }, { ref: 'c:c', p: 3 }]);
  assert.deepEqual(g.pages[0], { id: 'p1', root: { axis: 'row', ratio: G.C.RATIO_MAX, a: { card: 'a' }, b: { card: 'b' } } });
  assert.deepEqual(pagesOf(g), [['c:a', 'c:b'], ['c:c']]);
});

test('ordinals are the 1-based depth-first order across pages', () => {
  const g = G.reconcile(stored(), [
    { ref: 't:alpha', p: 1 }, { ref: 'c:cA', p: 2 }, { ref: 't:beta', p: 3 }, { ref: 'c:cB', p: 4 },
  ]);
  assert.deepEqual(G.ordinals(g), { 't:alpha': 1, 'c:cA': 2, 't:beta': 3, 'c:cB': 4 });
  const moved = G.movePage(g, 1, 0);
  assert.deepEqual(G.ordinals(moved), { 'c:cA': 1, 't:beta': 2, 'c:cB': 3, 't:alpha': 4 });
});

test('dual-written ordinals reload to the same pages (flat round trip)', () => {
  let g = G.dock(G.dock(G.dock(G.empty(), 'c:a'), 't:b'), 'c:c');
  g = G.append(g, 'c:c', 0);
  const ord = G.ordinals(g);
  const flat = Object.keys(ord).map(ref => ({ ref, p: ord[ref] }));
  // A previous build drops gallery: flat migration gives one pin per page.
  assert.deepEqual(pagesOf(G.reconcile(null, flat)), [['c:a'], ['c:c'], ['t:b']]);
  // This build keeps it.
  assert.deepEqual(pagesOf(G.reconcile(G.record(g), flat)), pagesOf(g));
});

test('dock appends a page before + and shows it; undock drops an emptied page', () => {
  let g = G.dock(G.empty(), 'c:a');
  g = G.dock(g, 't:b');
  assert.deepEqual(pagesOf(g), [['c:a'], ['t:b']]);
  assert.equal(G.position(g, false), 1);
  g = G.undock(g, 't:b');
  assert.deepEqual(pagesOf(g), [['c:a']]);
  assert.equal(G.position(g, false), 0);
  g = G.undock(g, 'c:a');
  assert.deepEqual(g.pages, []);
  assert.equal(G.position(g, false), 0); // + is position 0 with no pages
  assert.equal(G.position(g, true), -1);
});

test('undock of the last page while + shows leaves nothing shown; narrow stays home', () => {
  let g = G.activate(G.dock(G.empty(), 'c:a'), 1, false);
  assert.equal(g.active, G.PLUS);
  g = G.undock(g, 'c:a');
  assert.deepEqual(g.pages, []);
  assert.equal(g.active, null);
  assert.equal(G.position(g, true), -1);
  assert.equal(G.position(g, false), 0); // + is position 0 with no pages
});

test('reconcile with zero pages leaves nothing shown, + included', () => {
  let g = G.reconcile({ active: '+', pages: [] }, []);
  assert.deepEqual(g.pages, []);
  assert.equal(g.active, null);
  assert.equal(G.position(g, true), -1);
  g = G.reconcile({ active: '+', pages: [{ id: 'p1', root: { card: 'x' } }, { id: 'p2', root: { axis: 'row', ratio: 0.5, a: { topic: 'y' }, b: { card: 'z' } } }] }, []);
  assert.deepEqual(g.pages, []);
  assert.equal(g.active, null);
  assert.equal(G.position(g, true), -1);
  // + stays when a page remains.
  g = G.reconcile({ active: '+', pages: [{ id: 'p1', root: { card: 'x' } }] }, [{ ref: 'c:x', p: 1 }]);
  assert.equal(g.active, G.PLUS);
  assert.equal(G.position(g, true), 1);
});

test('narrow position on + with zero pages is home', () => {
  const g = { v: 1, rev: 0, active: G.PLUS, pages: [] };
  assert.equal(G.position(g, true), -1);
  assert.equal(G.position(g, false), 0);
});

test('undock keeps the shown page when another page empties', () => {
  let g = G.dock(G.dock(G.dock(G.empty(), 'c:a'), 'c:b'), 'c:c');
  g = G.activate(g, 2, false);
  g = G.undock(g, 'c:a');
  assert.equal(G.position(g, false), 1);
  assert.deepEqual(G.leaves(g.pages[1].root), ['c:c']);
});

test('dock of an already docked ref moves it to a new page', () => {
  let g = G.append(G.dock(G.dock(G.empty(), 'c:a'), 'c:b'), 'c:b', 0);
  assert.deepEqual(pagesOf(g), [['c:a', 'c:b']]);
  g = G.dock(g, 'c:a');
  assert.deepEqual(pagesOf(g), [['c:b'], ['c:a']]);
});

test('append to a page dot stacks below; past the pages is a new page', () => {
  let g = G.dock(G.dock(G.empty(), 'c:a'), 'c:b');
  g = G.append(g, 't:x', 0);
  assert.deepEqual(g.pages[0].root, { axis: 'col', ratio: 0.5, a: { card: 'a' }, b: { topic: 'x' } });
  assert.equal(G.position(g, false), 0);
  g = G.append(g, 'c:b', 0); // b's page empties and goes
  assert.deepEqual(pagesOf(g), [['c:a', 't:x', 'c:b']]);
  g = G.append(g, 'c:a', 5);
  assert.deepEqual(pagesOf(g), [['t:x', 'c:b'], ['c:a']]);
  assert.equal(G.append(g, 'c:a', 1), g); // already there
});

test('positions: home, pages, +, and activation clamps by view', () => {
  let g = G.dock(G.dock(G.empty(), 'c:a'), 'c:b');
  g = G.activate(g, -1, true);
  assert.equal(g.active, null);
  assert.equal(G.position(g, true), -1);
  assert.equal(G.position(g, false), 0);
  g = G.activate(g, 9, false);
  assert.equal(g.active, G.PLUS);
  assert.equal(G.position(g, true), 2);
  g = G.activate(g, -1, false);
  assert.equal(G.position(g, false), 0);
  assert.equal(g.active, g.pages[0].id);
});

test('movePage reorders and keeps the shown page', () => {
  let g = G.dock(G.dock(G.dock(G.empty(), 'c:a'), 'c:b'), 'c:c');
  g = G.activate(g, 0, false);
  g = G.movePage(g, 0, 2);
  assert.deepEqual(pagesOf(g), [['c:b'], ['c:c'], ['c:a']]);
  assert.equal(G.position(g, false), 2);
});

test('neighbors: edges are false, inert narrow without pages is null', () => {
  const none = G.empty(), one = G.dock(none, 'c:a');
  assert.deepEqual(G.neighbors(none, -1, true), { prev: null, next: null });
  assert.deepEqual(G.neighbors(one, -1, true), { prev: false, next: true });
  assert.deepEqual(G.neighbors(one, 0, true), { prev: true, next: true });
  assert.deepEqual(G.neighbors(one, 1, true), { prev: true, next: false });
  assert.deepEqual(G.neighbors(one, 0, false), { prev: false, next: true });
  assert.deepEqual(G.neighbors(one, 1, false), { prev: true, next: false });
});

test('layout mode mapping: tabs, below 1600, 1600-2199, from 2200', () => {
  const L = o => G.layout(Object.assign({ tabs: false, pages: 0, dragging: false }, o));
  // tabs is narrow at any width
  assert.deepEqual(L({ width: 3000, tabs: true, pages: 2 }), { narrow: true, column: false, overlay: false, rail: true });
  // below 1600 is narrow; its rail hides with no pages unless a drag runs
  assert.deepEqual(L({ width: 1599 }), { narrow: true, column: false, overlay: false, rail: false });
  assert.deepEqual(L({ width: 1200, dragging: true }), { narrow: true, column: false, overlay: false, rail: true });
  // 1600-2199: a docked page shows the column and the session rail overlays
  assert.deepEqual(L({ width: 1600, pages: 1 }), { narrow: false, column: true, overlay: true, rail: true });
  assert.deepEqual(L({ width: 2199, pages: 1 }), { narrow: false, column: true, overlay: true, rail: true });
  assert.deepEqual(L({ width: 1800 }), { narrow: false, column: false, overlay: false, rail: false });
  // from 2200: no reserved empty column; the rail docks beside a gallery
  assert.deepEqual(L({ width: 2200 }), { narrow: false, column: false, overlay: false, rail: false });
  assert.deepEqual(L({ width: 2400, pages: 1 }), { narrow: false, column: true, overlay: false, rail: true });
  // a drag opens the column with zero pages so + can take a first dock
  assert.deepEqual(L({ width: 2400, dragging: true }), { narrow: false, column: true, overlay: false, rail: true });
});
