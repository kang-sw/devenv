// Offline contract: shipped board functions only, no daemon or child process.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const test = require('node:test');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function funcs(names) {
  return names.map(name => {
    const found = source.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
    assert.ok(found, 'missing shipped function ' + name);
    return found[0];
  }).join('');
}
for (const documentMode of [false, true]) for (const latched of [[], ['T20'], ['T20', 'NEW']]) {
  test('hand return home keeps all pending signals until apply: ' + (documentMode ? 'document' : 'board') + ', latches=' + (latched.join(',') || 'none'), () => {
    const c = vm.createContext({ assert });
    vm.runInContext(`
      var latched = ${JSON.stringify(latched)};
      var ref = {}, bcol = { scrollTop: 5, querySelector: function () { return ref; } };
      var doc = { scrollingElement: { scrollTop: 5 } }, scroller = ${documentMode ? 'doc.scrollingElement' : 'bcol'};
      var jumps = 0, swaps = 0, rendered = 'old';
      var window = { IntersectionObserver: function () {}, wbPill: { update: function () {} }, wbHistory: {
        jump: function (node, opt) {
          jumps++;
          assert.equal(node, ref); assert.equal(opt.top, 0);
          opt.prepare();
        }
      } };
      function $(id) { return id === 'bcol' ? bcol : null; }
      var flashed = [];
      function boardFlash(names, inView) { flashed.push({ names: names, inView: inView }); }
      var cancelled = [];
      function cancelAnimationFrame(id) { cancelled.push(id); }
      var boardHomeWrites = new WeakSet(), boardReleaseFrame = 7, boardRelease = false;
      var boardPending = Object.create(null), boardTopHeld = Object.create(null);
      var boardLatest = { hash: 'latest', html: { content: 'new' } };
      latched.forEach(function (k) { boardPending[k] = true; boardTopHeld[k] = { node: null }; });
      boardPending.T04 = [{}];
      // Repeated revisions of a topic must still count only once.
      var boardChanges = [{ names: latched.concat('T04') }, { names: latched.slice() }];
      var pendingBefore = boardPending, heldBefore = boardTopHeld, changesBefore = boardChanges;
      function swap(name, hash, input, still) {
        swaps++;
        assert.equal(boardRelease, true, 'explicit navigation owns the release');
        assert.equal(name, 'board'); assert.equal(hash, 'latest'); assert.equal(still, true);
        rendered = input.content;
        // Model admission; release and badge bookkeeping use shipped functions.
        boardPending = Object.create(null); boardTopHeld = Object.create(null);
        boardRecordPending();
      }
    ` + funcs(['boardPendingNames', 'boardPendingBadge', 'boardViewedScroll', 'boardHome', 'boardFlushViewed', 'boardRecordPending']) + `
      boardPendingBadge(); assert.equal(window.wbPill.pending, latched.length + 1);
      scroller.scrollTop = 0;
      boardViewedScroll({ type: 'scroll', target: ${documentMode ? 'doc' : 'bcol'} });
      assert.equal(window.wbPill.pending, latched.length + 1, 'staged content keeps its full badge count at home');
      assert.deepEqual(boardPendingNames(), latched.concat('T04'));
      assert.deepEqual(flashed, [], 'nothing flashes as applied while still held');
      assert.equal(boardChanges, changesBefore, 'scrolling must not silently acknowledge any staged change');
      assert.equal(boardPending, pendingBefore);
      assert.equal(boardTopHeld, heldBefore);
      assert.equal(rendered, 'old'); assert.equal(swaps, 0); assert.equal(jumps, 0, 'raw scroll must not start explicit navigation');
      assert.equal(boardReleaseFrame, 0); assert.deepEqual(cancelled, [7]);
      // Repeated top ticks, including a known owner correction, stay harmless.
      boardHomeWrites.add(scroller);
      boardViewedScroll({ type: 'scroll', target: scroller });
      boardViewedScroll({ type: 'scroll', target: scroller });
      assert.equal(window.wbPill.pending, latched.length + 1);
      assert.equal(jumps, 0); assert.equal(swaps, 0);
      window.wbPill.home();
      assert.equal(jumps, 1); assert.equal(swaps, 1); assert.equal(rendered, 'new');
      assert.equal(window.wbPill.pending, 0, 'badge clears only after applying the snapshot');
      assert.deepEqual(boardPendingNames(), []);
      assert.deepEqual(Object.keys(boardTopHeld), []);
      assert.deepEqual(flashed, [{ names: latched.concat('T04'), inView: false }]);
    `, c);
  });
}
