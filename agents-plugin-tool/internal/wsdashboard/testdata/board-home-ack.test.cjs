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
for (const latched of [[], ['T20'], ['T20', 'NEW']]) {
  test('hand return home preserves only click-latched signals: ' + (latched.join(', ') || 'none'), () => {
    const c = vm.createContext({ assert });
    vm.runInContext(`
      var latched = ${JSON.stringify(latched)};
      var bcol = { scrollTop: 5 }, doc = { scrollingElement: { scrollTop: 0 } };
      var window = { IntersectionObserver: function () {}, wbPill: { update: function () {} } };
      function $(id) { return id === 'bcol' ? bcol : null; }
      function boardHome() {}
      var flashed = [];
      function boardFlash(names, inView) { assert.equal(inView, true); flashed = flashed.concat(names); }
      var cancelled = [];
      function cancelAnimationFrame(id) { cancelled.push(id); }
      var boardHomeWrites = new WeakSet(), boardReleaseFrame = 7;
      var boardPending = Object.create(null), boardTopHeld = Object.create(null);
      latched.forEach(function (k) { boardPending[k] = true; boardTopHeld[k] = { node: null }; });
      boardPending.T04 = [{}];
      var pendingBefore = boardPending, heldBefore = boardTopHeld;
      // Repeated revisions of a latched topic must still count only once.
      var boardChanges = [{ names: latched.concat('T04') }, { names: latched.slice() }];
    ` + funcs(['boardPendingNames', 'boardPendingBadge', 'boardViewedScroll']) + `
      boardPendingBadge(); assert.equal(window.wbPill.pending, latched.length + 1);
      bcol.scrollTop = 0;
      boardViewedScroll({ type: 'scroll', target: bcol });
      assert.equal(window.wbPill.pending, latched.length, 'click-only latches retain their badge count');
      assert.deepEqual(boardPendingNames(), latched, 'only scroll-releasable signals are acknowledged');
      assert.deepEqual(flashed, ['T04'], 'latched topics do not flash as applied');
      assert.deepEqual(boardChanges.flatMap(function (c) { return c.names; }), latched.concat(latched));
      assert.equal(boardPending, pendingBefore, 'acknowledgement does not release staged data');
      assert.equal(boardTopHeld, heldBefore, 'acknowledgement preserves the latch ledger');
      assert.deepEqual(Object.keys(boardTopHeld), latched);
      assert.deepEqual(Object.keys(boardPending), latched.concat('T04'));
      assert.equal(boardReleaseFrame, 0);
      assert.deepEqual(cancelled, [7]);
    `, c);
  });
}
