// Gallery model: the 2D split tree.
// Run: node --test testdata/gallery-tree.test.cjs (from internal/wsdashboard)
const test = require('node:test');
const assert = require('node:assert/strict');
const G = require('../assets/gallery.js');

const L = r => G.leaf(r);
const S = (axis, a, b, ratio = 0.5) => ({ axis, ratio, a, b });

test('splitAt: each edge zone splits on its axis and side', () => {
  const t = L('c:a');
  assert.deepEqual(G.splitAt(t, 'c:a', 'left', 'c:n'), S('row', L('c:n'), L('c:a')));
  assert.deepEqual(G.splitAt(t, 'c:a', 'right', 'c:n'), S('row', L('c:a'), L('c:n')));
  assert.deepEqual(G.splitAt(t, 'c:a', 'top', 't:n'), S('col', L('t:n'), L('c:a')));
  assert.deepEqual(G.splitAt(t, 'c:a', 'bottom', 't:n'), S('col', L('c:a'), L('t:n')));
});

test('splitAt recurses to the target leaf only', () => {
  const t = S('row', L('c:a'), S('col', L('c:b'), L('c:c'), 0.7), 0.3);
  assert.deepEqual(G.splitAt(t, 'c:c', 'right', 'c:n'), S('row', L('c:a'), S('col', L('c:b'), S('row', L('c:c'), L('c:n')), 0.7), 0.3));
});

test('remove collapses the parent split and normalizes', () => {
  const t = S('row', L('c:a'), S('col', L('c:b'), L('c:c'), 0.7), 0.3);
  assert.deepEqual(G.removeRef(t, 'c:b'), S('row', L('c:a'), L('c:c'), 0.3));
  assert.deepEqual(G.removeRef(t, 'c:a'), S('col', L('c:b'), L('c:c'), 0.7));
  assert.equal(G.removeRef(L('c:a'), 'c:a'), null);
});

test('ratio clamping', () => {
  assert.equal(G.clampRatio(0), G.C.RATIO_MIN);
  assert.equal(G.clampRatio(1), G.C.RATIO_MAX);
  assert.equal(G.clampRatio(0.42), 0.42);
  assert.equal(G.clampRatio(NaN), 0.5);
  let g = G.place(G.dock(G.empty(), 'c:a'), 'c:b', 'c:a', 'right');
  const id = g.pages[0].id;
  g = G.setRatio(g, id, '', 0.02);
  assert.equal(g.pages[0].root.ratio, G.C.RATIO_MIN);
  g = G.setRatio(g, id, '', 0.66);
  assert.equal(g.pages[0].root.ratio, 0.66);
  assert.equal(G.setRatio(g, id, 'a', 0.3), g); // a leaf has no ratio
});

test('place: an edge drop splits the target, moving the dragged pane off its page', () => {
  let g = G.dock(G.dock(G.empty(), 'c:a'), 'c:b');
  g = G.place(g, 'c:b', 'c:a', 'top');
  assert.equal(g.pages.length, 1);
  assert.deepEqual(g.pages[0].root, S('col', L('c:b'), L('c:a')));
  assert.equal(G.position(g, false), 0);
  // a board item (not yet docked) drops on an edge
  g = G.place(g, 't:x', 'c:a', 'left');
  assert.deepEqual(g.pages[0].root, S('col', L('c:b'), S('row', L('t:x'), L('c:a'))));
});

test('place within a page moves a pane to another edge', () => {
  let g = G.place(G.dock(G.empty(), 'c:a'), 'c:b', 'c:a', 'right');
  g = G.place(g, 'c:a', 'c:b', 'bottom');
  assert.deepEqual(g.pages[0].root, S('col', L('c:b'), L('c:a')));
});

test('center drop swaps two docked panes, across pages too', () => {
  let g = G.place(G.dock(G.empty(), 'c:a'), 'c:b', 'c:a', 'right');
  g = G.dock(g, 'c:c');
  const swapped = G.place(g, 'c:c', 'c:a', 'center');
  assert.deepEqual(swapped.pages[0].root, S('row', L('c:c'), L('c:b')));
  assert.deepEqual(G.leaves(swapped.pages[1].root), ['c:a']);
  assert.equal(G.position(swapped, false), 0);
  // a board item has nothing to swap with
  assert.equal(G.place(g, 't:x', 'c:a', 'center'), g);
  // a pane dropped on itself does nothing
  assert.equal(G.place(g, 'c:a', 'c:a', 'left'), g);
});

test('unpinning one pane of a multi-pane page collapses; the last pane removes the page', () => {
  let g = G.place(G.place(G.dock(G.empty(), 'c:a'), 'c:b', 'c:a', 'right'), 'c:c', 'c:b', 'bottom');
  g = G.dock(g, 'c:d');
  g = G.undock(g, 'c:b');
  assert.deepEqual(g.pages[0].root, S('row', L('c:a'), L('c:c')));
  g = G.undock(G.undock(g, 'c:a'), 'c:c');
  assert.deepEqual(g.pages.map(p => G.leaves(p.root)), [['c:d']]);
});

test('moving a pane across pages removes its emptied page and re-splits there', () => {
  let g = G.dock(G.dock(G.empty(), 'c:a'), 'c:b');
  g = G.place(g, 'c:a', 'c:b', 'left');
  assert.deepEqual(g.pages.map(p => G.leaves(p.root)), [['c:a', 'c:b']]);
});

test('depth-first ordinal derivation follows a before b', () => {
  let g = G.dock(G.empty(), 'c:a');
  g = G.place(g, 'c:b', 'c:a', 'left'); // b | a
  g = G.place(g, 'c:c', 'c:a', 'top');  // b | (c / a)
  g = G.dock(g, 't:z');
  assert.deepEqual(G.ordinals(g), { 'c:b': 1, 'c:c': 2, 'c:a': 3, 't:z': 4 });
});

test('zone: nearest edge, and the center only where allowed', () => {
  const r = { left: 0, top: 0, width: 100, height: 100 };
  assert.equal(G.zone(r, 5, 50), 'left');
  assert.equal(G.zone(r, 95, 50), 'right');
  assert.equal(G.zone(r, 50, 5), 'top');
  assert.equal(G.zone(r, 50, 95), 'bottom');
  assert.equal(G.zone(r, 50, 50), 'left'); // tie, no center: an edge
  assert.equal(G.zone(r, 50, 50, true), 'center');
  assert.equal(G.zone(r, 20, 50, true), 'left');
});

test('nodeAt walks a path', () => {
  const t = S('row', L('c:a'), S('col', L('c:b'), L('c:c')));
  assert.deepEqual(G.nodeAt(t, 'b'), S('col', L('c:b'), L('c:c')));
  assert.deepEqual(G.nodeAt(t, 'ba'), L('c:b'));
  assert.equal(G.nodeAt(t, 'aa'), null);
});
