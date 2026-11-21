// Offline VM contract only: no server, browser, executable probe or child process.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const tick = () => new Promise(setImmediate);
const { readAssetSource } = require('./asset-source.cjs');

function fixture(cacheLimit = 0) {
  const go = fs.readFileSync(path.join(__dirname, '../shell_frame_test.go'), 'utf8');
  const harness = go.match(/const shellGlueHarness = `([\s\S]*?)\n`/)[1];
  const js = readAssetSource('wsdashboard', 'dashboard.js');
  new vm.Script(js); // Also parse the complete shipped fragment concatenation.
  const glue = js.match(/\n  var shFrames = \[\][\s\S]*?\n  \/\/ shFrameInit wires/)[0];
  const init = js.match(/\n  function shShellInit\(\) \{[\s\S]*?\n  \}\n/)[0];
  let clock = 0, timerId = 0;
  const timers = new Map();
  const context = vm.createContext({ URL, console,
    setTimeout(fn, ms) { const id = ++timerId; timers.set(id, {fn, at: clock + ms}); return id; },
    clearTimeout(id) { timers.delete(id); }
  });
  context.advance = ms => {
    clock += ms;
    for (const [id, timer] of timers) if (timer.at <= clock) { timers.delete(id); timer.fn(); }
  };
  context.timerCount = () => timers.size;
  vm.runInContext(harness + historyHarness + readAssetSource('wsdashboard', 'shell.js') + `
    window.productionCacheLimit = window.WsShell.cacheLimit;
    window.WsShell.cacheLimit = ${cacheLimit};
    root.getAttribute = function (key) { return attrs[key] || null; };
    function cacheSafety(frame) {
      frame.contentWindow.wsCanCacheSession = function () { frame.saved++; return !frame.unsafe && !frame.tool && !frame.draft; };
      frame.contentWindow.wsPrepareSessionLeave = function () {
        frame.prepared = (frame.prepared || 0) + 1;
        return frame.delay || !!frame.draft;
      };
      frame.contentWindow.wsFinishSessionLeave = function () { frame.finished = true; };
      return frame;
    }
    var decisions = [];
    function wsT(k) { return k; }
    window.wsConfirm = function (message) { return new Promise(function (done) { decisions.push(done); }); };
    var OriginalFrame = Frame;
    Frame = function (src) { return cacheSafety(new OriginalFrame(src)); };
    Frame.prototype = OriginalFrame.prototype;
    var originalLoad = Frame.prototype.load;
    Frame.prototype.load = function () {
      if (!this.holdHeader)
        from(this.contentWindow, {wsShell: 'head', html: '', qs: '', pad: 0, top: 40});
      originalLoad.call(this);
    };
    cacheSafety(first);
  ` + '\n(function () {\n' + glue + init + `
    first.contentDocument.readyState = 'complete';
    shShellInit();
    shOn.head = {html: '', qs: '', pad: 0, top: 40};
    window.testShell = {
      open: shOpen,
      count: function () { return shFrames.length; },
      current: function () { return shOn && shOn.el; },
      pending: function () { return shWant && shWant.el; }
    };
  })();`, context);
  return context;
}
// Model actual history entries and traversal, not just the address bar.
const historyHarness = `
var entries = [{url: location.href, state: null}], position = 0;
function address(url) { var u = new URL(url, ORIGIN); location.pathname = u.pathname; location.search = u.search; location.hash = u.hash; }
history = {
  get state() { return entries[position].state; },
  pushState: function (state, title, url) {
    pushed.push(url); entries.splice(position + 1); entries.push({url: new URL(url, ORIGIN).href, state: state}); position++; address(entries[position].url);
  },
  replaceState: function (state, title, url) { entries[position] = {url: new URL(url, ORIGIN).href, state: state}; address(entries[position].url); },
  deferred: false, traversals: [],
  go: function (delta) {
    var next = position + delta;
    if (next < 0 || next >= entries.length || next === position) return;
    function traverse() {
      position = next; address(entries[position].url);
      listeners.popstate.forEach(function (fn) { fn({state: history.state}); });
    }
    if (this.deferred) this.traversals.push(traverse); else traverse();
  },
  flush: function () { while (this.traversals.length) this.traversals.shift()(); }
};
function historyPaths() { return entries.map(function (e) { var u = new URL(e.url); return u.pathname + u.hash; }); }

`;
function plain(value) { return JSON.parse(JSON.stringify(value)); }
async function answer(c, accepted) {
  await tick();
  const decide = c.decisions.shift();
  assert.ok(decide, 'there is an open confirmation');
  decide(accepted);
  await tick();
}
async function runBase() {
  const c = fixture(), s = c.window.testShell, first = c.first;
  const pure = fs.readFileSync(path.join(__dirname, '../shell_test.go'), 'utf8').match(/const shellJSCases = `([\s\S]*?)\n`/)[1];
  vm.runInContext(pure, c);
  assert.deepEqual(plain(c.bad), []);
  assert.equal(s.current(), first);
  assert.deepEqual(plain(first.got), ['sync', 'settings']);
  const beforeHead = c.renameChecks;
  c.from(first.contentWindow, { wsShell: 'head', html: '<b>a</b>', qs: '', pad: 0, top: 40 });
  assert.deepEqual(plain(c.log), ['swap:head:<b>a</b>']);
  assert.equal(c.renameChecks, beforeHead + 1, 'the mirrored header retries a pending rename');
  for (const [source, origin, data] of [
    [first.contentWindow, 'http://evil:1', { wsShell: 'nav', href: '/s/evil' }],
    [{}, c.ORIGIN, { wsShell: 'nav', href: '/s/evil' }],
    [first.contentWindow, c.ORIGIN, { wsShell: 'head', html: 1, qs: '', pad: 0, top: 40 }],
  ]) c.from(source, data, origin);
  assert.deepEqual(plain(c.pushed), []);
  assert.equal(c.renameChecks, beforeHead + 1);

  first.draft = true;
  c.click('/s/b');
  assert.equal(first.prepared, 1);
  assert.equal(first.inert, true);
  assert.equal(s.current(), first);
  assert.equal(c.frameOf('/s/b'), undefined);
  await answer(c, false);
  assert.equal(first.inert, false);
  assert.equal(first.removed, false);
  assert.equal(c.location.pathname, '/s/a');
  s.open('/s/a#files', true);
  assert.equal(first.contentWindow.location.hash, '#files');
  assert.equal(first.prepared, 1, 'same-session routes need no new decision');

  c.click('/s/b');
  await answer(c, true);
  assert.equal(first.finished, true);
  assert.equal(first.removed, true);
  const b = c.frameOf('/s/b');
  b.load();
  assert.equal(s.current(), b);
  assert.deepEqual(plain(b.got), ['settings', 'shown:true']);
  const head = c.log.length, renames = c.renameChecks;
  c.from(first.contentWindow, { wsShell: 'nav', href: '/s/stray' });
  c.from(first.contentWindow, { wsShell: 'head', html: '<b>stale</b>', qs: '', pad: 0, top: 40 });
  first.load();
  assert.equal(c.log.length, head);
  assert.equal(c.renameChecks, renames);
  assert.equal(s.current(), b);

  for (let i = 0; i < 10; i++) {
    await switchTo(c, '/s/n' + i);
    assert.equal(s.count(), 1);
  }
  c.click('/s/loading');
  await tick();
  const superseded = c.frameOf('/s/loading');
  c.click('/s/latest');
  await tick();
  assert.equal(superseded.removed, true);
  superseded.load();
  c.frameOf('/s/latest').load();
  assert.equal(s.current(), c.frameOf('/s/latest'));
  assert.equal(c.click('/s/other', { ctrlKey: true }), false);
  assert.equal(c.click('/tracker/other'), false);
  assert.equal(c.click('https://example.com/'), false);
}
async function switchTo(c, path) {
  c.click(path);
  await tick();
  const next = c.frameOf(path);
  next.load();
  return next;
}
async function testHistoryCancellation() {
  const c = fixture(), s = c.window.testShell;
  const b = await switchTo(c, '/s/b');
  b.draft = true;
  c.history.go(-1);
  assert.deepEqual(plain(c.historyPaths()), ['/s/a', '/s/b'], 'pending Back preserves both entries');
  await answer(c, false);
  assert.equal(c.position, 1);
  assert.equal(s.current(), b);
  c.history.go(-1);
  await answer(c, true);
  c.frameOf('/s/a').load();
  c.history.go(1);
  await tick();
  c.frameOf('/s/b').load();
  assert.equal(s.current(), c.frameOf('/s/b'));
  const last = await switchTo(c, '/s/c');
  last.draft = true;
  c.history.go(-1);
  await tick();
  c.history.go(-1);
  await answer(c, true);
  assert.equal(s.current(), last, 'a decision for the first Back cannot approve the second destination');
  await answer(c, false);
  assert.equal(c.position, 2);
  assert.deepEqual(plain(c.historyPaths()), ['/s/a', '/s/b', '/s/c']);
}
async function testRapidPickAndDelayedSave() {
  const c = fixture(), a = c.first;
  let finish;
  a.delay = new Promise(resolve => { finish = resolve; });
  a.draft = true;
  c.click('/s/b');
  c.click('/s/c');
  assert.equal(c.decisions.length, 0);
  assert.equal(c.frameOf('/s/b'), undefined);
  assert.equal(a.removed, false);
  a.delay = null;
  finish(true);
  await tick();
  assert.equal(a.prepared, 2, 'only the latest pick may ask after the delayed save');
  await answer(c, true);
  assert.ok(c.frameOf('/s/c'));
  assert.equal(c.frameOf('/s/b'), undefined);
}
async function testRestoreTimeout() {
  const c = fixture(), b = await switchTo(c, '/s/b');
  b.draft = true;
  c.history.go(-1);
  c.history.go = () => {}; // A joint frame traversal may emit no shell popstate.
  await answer(c, false);
  c.window.testShell.open('/s/c', true);
  c.advance(999);
  assert.equal(b.prepared, 1);
  b.draft = false;
  c.advance(1);
  await tick();
  assert.equal(b.prepared, 2);
  assert.ok(c.frameOf('/s/c'));
}
function cachedSwitch(c, path) {
  c.click(path);
  const next = c.window.testShell.pending();
  if (next) next.load();
  return c.window.testShell.current();
}
function testCacheReuse() {
  const c = fixture(5), s = c.window.testShell, a = c.first;
  assert.equal(c.window.productionCacheLimit, 5, 'production bound is a code constant');
  a.contentWindow.localReadingState = { value: 'not rebuilt' };
  c.click('/s/b');
  assert.equal(a.inert, true, 'no new edit can race cache admission during a cold destination load');
  const b = s.pending(); assert.ok(b); b.load();
  assert.equal(a.removed, false); assert.equal(s.count(), 2);
  assert.equal(a.got.at(-1), 'shown:false');
  b.contentWindow.typed = 'persisted draft';
  const returned = cachedSwitch(c, '/s/a');
  assert.equal(returned, a); assert.equal(a.inert, false);
  assert.equal(a.contentWindow.localReadingState.value, 'not rebuilt');
  assert.equal(s.count(), 2); assert.equal(b.removed, false);
  assert.ok(a.got.includes('theme')); assert.equal(a.got.at(-1), 'shown:true');
  s.open('/s/a#files', true); assert.equal(a.contentWindow.location.hash, '#files');
  assert.equal(s.current(), a);
  const pushes = c.pushed.length;
  c.history.go(-2); assert.equal(s.current(), b, 'Back reuses the same Window');
  c.history.go(1); assert.equal(s.current(), a, 'Forward reuses the same Window');
  assert.equal(c.pushed.length, pushes);
  assert.equal(b.contentWindow.typed, 'persisted draft');
}
function testCacheEvictionAndRaces() {
  const c = fixture(2), s = c.window.testShell, a = c.first;
  const b = cachedSwitch(c, '/s/b');
  const third = cachedSwitch(c, '/s/c');
  assert.equal(a.removed, true); assert.equal(s.count(), 2);
  assert.equal(cachedSwitch(c, '/s/b'), b, 'hit updates recency');
  const fourth = cachedSwitch(c, '/s/d');
  assert.equal(third.removed, true); assert.equal(b.removed, false); assert.equal(s.count(), 2);
  const log = c.log.length;
  c.from(a.contentWindow, {wsShell: 'nav', href: '/s/stray'}); a.load();
  assert.equal(c.log.length, log); assert.equal(s.current(), fourth);
  c.click('/s/loading'); const superseded = s.pending();
  c.click('/s/latest'); assert.equal(superseded.removed, true);
  const latest = s.pending(); superseded.load(); latest.load();
  assert.equal(s.current(), latest); assert.equal(s.count(), 2);
  // Cancelling a loading pick by returning to the current page unlocks it.
  c.click('/s/unused'); const unused = s.pending(); c.click('/s/latest');
  assert.equal(unused.removed, true); assert.equal(latest.inert, false);
}
async function testCacheAdmissionExpiry() {
  for (const cancel of [false, true]) {
    const c = fixture(5), s = c.window.testShell, a = c.first;
    c.click('/s/b');
    const ready = s.pending();
    a.draft = true;
    ready.load();
    assert.equal(s.current(), a, 'eligibility is checked again before hiding');
    assert.equal(a.got.includes('shown:false'), false);
    assert.equal(a.inert, true);
    await answer(c, !cancel);
    if (cancel) {
      assert.equal(c.location.pathname, '/s/a');
      assert.equal(s.current(), a);
      assert.equal(a.removed, false);
      assert.equal(a.inert, false);
      assert.equal(ready.removed, true);
      ready.load();
      assert.equal(s.current(), a);
    } else {
      assert.equal(a.removed, true);
      assert.equal(s.current(), ready, 'keep the ready destination after acceptance');
    }
  }
  const c = fixture(5), a = c.first;
  a.tool = true;
  c.click('/s/b');
  await tick();
  assert.equal(c.decisions.length, 0, 'clean tool pages are loaded fresh without a draft decision');
  assert.equal(a.removed, true);
}
function testHeaderReadiness() {
  for (const headFirst of [false, true]) {
    const c = fixture(5), s = c.window.testShell, a = c.first;
    c.from(a.contentWindow, {wsShell: 'head', html: '<b>old</b>', qs: '', pad: 0, top: 40});
    c.click('/s/b');
    const b = s.pending();
    b.holdHeader = true;
    const header = {wsShell: 'head', html: '<b>new</b>', qs: '', pad: 0, side: 24, top: 40};
    if (headFirst) c.from(b.contentWindow, header); else b.load();
    assert.equal(s.current(), a, 'both document load and header are required');
    assert.equal(c.top.style.minHeight, '40px');
    assert.equal(c.log.at(-1), 'swap:head:<b>old</b>', 'do not clear the old header');
    if (headFirst) b.load(); else c.from(b.contentWindow, header);
    assert.equal(s.current(), b);
    assert.equal(c.log.includes('swap:head:<b>new</b>'), true);
    assert.equal(c.top.style['--sh-stage-side'], '24px');
    assert.equal(a.got.at(-1), 'shown:false');
    assert.equal(c.timerCount(), 0, 'a real header cancels its fallback');
  }
}
async function testHeaderFallback() {
  const c = fixture(5), s = c.window.testShell, a = c.first;
  c.from(a.contentWindow, {wsShell: 'head', html: '<b>old</b>', qs: '', pad: 0, top: 40});
  c.click('/s/b');
  const b = s.pending(); b.holdHeader = true; b.load();
  c.advance(399);
  assert.equal(s.current(), a, 'keep old pixels during the bounded wait');
  c.advance(1);
  assert.equal(s.current(), b, 'a loaded page that never posts head must still appear');
  assert.equal(c.top.style.minHeight, '40px', 'fallback keeps the header box');
  assert.equal(c.log.at(-1), 'swap:head:', 'fallback removes stale session controls');
  assert.equal(c.timerCount(), 0);
  assert.equal(cachedSwitch(c, '/s/a'), a);
  assert.equal(cachedSwitch(c, '/s/b'), b, 'a fallback-shown cached page needs no second wait');
  c.from(b.contentWindow, {wsShell: 'head', html: '<b>late</b>', qs: '', pad: 0, top: 40});
  assert.equal(c.log.at(-1), 'swap:head:<b>late</b>', 'a late header still repairs the chrome');

  c.click('/s/superseded');
  const superseded = s.pending(); superseded.holdHeader = true; superseded.load();
  assert.equal(c.timerCount(), 1);
  c.click('/s/latest');
  assert.equal(superseded.removed, true);
  assert.equal(c.timerCount(), 0, 'superseding a pending destination cancels its timer');
  const latest = s.pending(); latest.load(); c.advance(400);
  assert.equal(s.current(), latest, 'a discarded timeout cannot regain authority');

  const d = fixture(5), ds = d.window.testShell, old = d.first;
  d.click('/s/unsafe');
  const target = ds.pending(); target.holdHeader = true; target.load();
  old.draft = true; d.advance(400);
  assert.equal(ds.current(), old, 'fallback still rechecks native leave eligibility');
  await answer(d, false); d.advance(400);
  assert.equal(target.removed, true);
  assert.equal(ds.current(), old, 'Stay cancels a headerless destination too');
}
const cases = {
  'header handoff waits for load and head in either order': testHeaderReadiness,
  'headerless destinations have a bounded and cancellable fallback': testHeaderFallback,
  'fresh frames, same-session routes and stale events': runBase,
  'Stay restores Back/Forward history and stale decisions cannot navigate': testHistoryCancellation,
  'rapid picks wait for the save and confirm only their current destination': testRapidPickAndDelayedSave,
  'missing restoration popstate times out and flushes navigation': testRestoreTimeout,
  'bounded cache reuses documents, hashes and history': testCacheReuse,
  'cache LRU eviction, stale authority and superseded loads': testCacheEvictionAndRaces,
  'late draft state during a cold load keeps the source until consent': testCacheAdmissionExpiry
};
module.exports = async function () { for (const run of Object.values(cases)) await run(); };
if (require.main === module) {
  const test = require('node:test');
  for (const [name, run] of Object.entries(cases)) test(name, run);
}
