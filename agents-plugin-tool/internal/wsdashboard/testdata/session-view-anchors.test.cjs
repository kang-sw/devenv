// Offline contract for the session view's board anchor (dashboard-session-view.js
// sessionBoardAnchors), saved on the shell's session-switch click path.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function func(name) {
  const m = source.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
  assert.ok(m, name); return m[0];
}
const parts = source.match(/var BOARD_CV_PARTS = ("[^"]*");/);
assert.ok(parts, 'BOARD_CV_PARTS');
const PART = JSON.parse(parts[1]).split(', ')[0];

// A layout model: any geometry or visibility read finds layout dirty at most
// once, unless it reads inside a skipped part, whose forced display-lock
// update leaves layout dirty again for the next read.
function board(partTops, opt) {
  opt = opt || {};
  const stats = { forced: 0, skippedReads: 0, dirty: true };
  const scroller = { id: 'board', scrollTop: 120 };
  function node(sels, parent, rect) {
    const n = { sels, parentElement: parent, rect, kids: [], hidden: false };
    if (parent) parent.kids.push(n);
    n.matches = sel => sel.split(/,\s*/).some(s => n.sels.includes(s));
    n.closest = sel => { for (let p = n; p; p = p.parentElement) if (p.matches(sel)) return p; return null; };
    n.contains = o => { for (let p = o; p; p = p.parentElement) if (p === n) return true; return false; };
    n.classList = { contains: c => n.sels.includes('.' + c) };
    const read = () => {
      if (stats.dirty) { stats.forced++; stats.dirty = false; }
      for (let p = n.parentElement; p; p = p.parentElement) if (p.skipped) { stats.skippedReads++; stats.dirty = true; break; }
    };
    n.getBoundingClientRect = () => { read(); return n.rect; };
    n.checkVisibility = () => { read(); return true; };
    return n;
  }
  const box = node([], null, null), panels = [];
  partTops.forEach((top, i) => {
    const topic = node(['.topic', 'section.topic[data-topic]'], box, null);
    topic.key = 't' + i;
    const part = node([PART], topic, { top, bottom: top + 470, height: 470 });
    part.skipped = top > 1200 || top + 470 < -600;
    for (let j = 0; j < 4; j++) {
      const t = top + 10 + j * 115, h = opt.height ? opt.height(i, j) : 110;
      const p = node(['.entry-panel'], part, { top: t, bottom: t + h, height: h });
      p.id = 't' + i + 'p' + j; panels.push(p);
    }
  });
  const w = {
    keep: () => ({ scroller, viewport: () => ({ top: 0, bottom: 600 }) }),
    snapshot: () => ({ scroller: () => scroller }),
    rendered: n => n.checkVisibility(),
  };
  const c = vm.createContext({
    Map, Set, window: { wbHistory: w }, doc: { scrollingElement: null }, BOARD_CV_PARTS: JSON.parse(parts[1]), boardSnap: null,
    boardBoxes: () => [box], visible: () => true, topicKey: t => t.key, boardPanelKey: n => n.id,
    q: (sel, el) => { const out = []; (function walk(n) { n.kids.forEach(k => { if (k.matches(sel)) out.push(k); walk(k); }); })(el); return out; },
  });
  vm.runInContext(func('boardWithSnap') + func('boardScroller') + func('boardLivePanels') + func('sessionBoardAnchors') + 'this.anchors = sessionBoardAnchors;', c);
  return { stats, anchors: () => JSON.parse(JSON.stringify(c.anchors())) };
}

test('the anchor save forces one layout and reads no panel inside a skipped part', () => {
  const tops = Array.from({ length: 40 }, (_, i) => -480 + i * 480);
  const b = board(tops);
  assert.deepEqual(b.anchors(), [{ scroller: 'board', top: 120, topic: 't1', key: 't1p0', offset: 9 }]);
  assert.equal(b.stats.forced, 1);
  assert.equal(b.stats.skippedReads, 0);
});

test('with no panel starting in the view, every panel is read as before', () => {
  // Part 1 reaches the view but holds only empty panels; the anchor is the
  // first panel of part 2, below the view.
  const b = board([-480, 0, 700, 1500], { height: (i) => i === 1 ? 0 : 110 });
  assert.deepEqual(b.anchors(), [{ scroller: 'board', top: 120, topic: 't2', key: 't2p0', offset: 709 }]);
});
