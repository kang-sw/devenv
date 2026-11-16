// Offline VM contract only: no server, browser, executable probe or child process.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
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
      frame.contentWindow.wsCanCacheSession = function () { frame.saved++; return !frame.unsafe && !frame.tool; };
      return frame;
    }
    var OriginalFrame = Frame;
    Frame = function (src) { return cacheSafety(new OriginalFrame(src)); };
    Frame.prototype = OriginalFrame.prototype;
    cacheSafety(first);
  ` + '\n(function () {\n' + glue + init + `
    first.contentDocument.readyState = 'complete';
    shShellInit();
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
function stay(frame) { from(frame.contentWindow, {wsShell: 'stay', attempt: frame.contentWindow.wsShellLeave}); }
`;
function plain(value) { return JSON.parse(JSON.stringify(value)); }
function runBase() {
  let c = fixture(), s = c.window.testShell, first = c.first;
  const pure = fs.readFileSync(path.join(__dirname, '../shell_test.go'), 'utf8').match(/const shellJSCases = `([\s\S]*?)\n`/)[1];
  vm.runInContext(pure, c);
  assert.deepEqual(plain(c.bad), [], 'URL, message-shape, color and retired-cache helper contracts');
  assert.equal(s.current(), first);
  assert.deepEqual(plain(first.got), ['sync', 'settings']);
  c.from(first.contentWindow, { wsShell: 'head', html: '<b>a</b>', qs: '', pad: 0, top: 40 });
  assert.deepEqual(plain(c.log), ['swap:head:<b>a</b>']);
  c.log.length = 0;
  for (const [source, origin, data] of [
    [first.contentWindow, 'http://evil:1', { wsShell: 'nav', href: '/s/evil' }],
    [{}, c.ORIGIN, { wsShell: 'nav', href: '/s/evil' }],
    [first.contentWindow, c.ORIGIN, { wsShell: 'head', html: 1, qs: '', pad: 0, top: 40 }],
  ]) c.from(source, data, origin);
  assert.equal(s.count(), 1);
  assert.deepEqual(plain(c.pushed), []);
  assert.deepEqual(plain(c.log), []);

  // The actual native confirmation is deliberately not mocked into production:
  // location.replace requests leave; no load means cancellation, a blank load
  // means acceptance. Draft save must occur before either possible outcome.
  first.cancelLeave = true;
  c.click('/s/b');
  assert.equal(first.saved, 1);
  assert.equal(first.removed, false);
  assert.equal(s.current(), first);
  assert.equal(c.location.pathname, '/s/a');
  assert.equal(s.count(), 1);
  assert.equal(c.frameOf('/s/b'), undefined);
  c.stay(first);
  first.cancelLeave = false;
  c.click('/s/b');
  assert.equal(first.saved, 2);
  assert.equal(first.pending, 'about:blank');
  assert.equal(first.removed, false, 'do not remove before the native leave guard accepts');
  first.load();
  let b = c.frameOf('/s/b');
  assert.ok(b);
  assert.equal(first.removed, true);
  assert.equal(s.count(), 1, 'old frame is gone before the new frame loads');
  assert.deepEqual(plain(c.live()), ['/s/b']);
  assert.deepEqual(plain(c.on()), ['/s/b?frame=1']);
  b.load();
  assert.equal(s.current(), b);
  assert.deepEqual(plain(b.got), ['settings', 'shown:true']);
  assert.deepEqual(plain(c.pushed), ['/s/b']);
  assert.equal(c.attrs['data-rid'], 'b');

  // No old Window may navigate or change the header, including a delayed load.
  const head = c.log.length;
  c.from(first.contentWindow, { wsShell: 'nav', href: '/s/stray' });
  c.from(first.contentWindow, { wsShell: 'head', html: '<b>stale</b>', qs: '', pad: 0, top: 40 });
  first.load();
  assert.equal(c.log.length, head);
  assert.equal(s.current(), b);

  // A burst while leave is pending loads only its latest target.
  c.from(b.contentWindow, { wsShell: 'nav', href: '/s/c' });
  c.from(b.contentWindow, { wsShell: 'nav', href: '/s/d' });
  b.load();
  assert.equal(c.frameOf('/s/c'), undefined);
  let d = c.frameOf('/s/d');
  assert.ok(d);
  d.load();
  assert.equal(s.current(), d);
  assert.equal(s.count(), 1);

  // Same-session hashes stay in the live document; back/forward to a different
  // session always loads fresh, never revives an old Window or adds history.
  s.open('/s/d#files', true);
  assert.equal(s.current(), d);
  assert.equal(d.saved, 0);
  assert.equal(d.contentWindow.location.hash, '#files');
  const pushes = c.pushed.length;
  c.history.go(-c.position);
  assert.equal(c.location.pathname, '/s/a', 'pending Back leaves its destination entry untouched');
  d.load();
  const freshA = c.frameOf('/s/a');
  assert.ok(freshA);
  assert.notEqual(freshA, first);
  freshA.load();
  assert.equal(c.location.pathname, '/s/a');
  assert.equal(c.pushed.length, pushes);

  // Loading targets can also be superseded; ten switches retain one frame.
  let previous = freshA;
  for (let i = 0; i < 10; i++) {
    c.click('/s/n' + i);
    previous.load();
    const next = c.frameOf('/s/n' + i);
    assert.ok(next);
    assert.equal(previous.removed, true);
    assert.equal(s.count(), 1);
    next.load();
    previous = next;
  }
  assert.equal(c.live().length, 1);
  c.click('/s/loading');
  previous.load();
  const superseded = c.frameOf('/s/loading');
  assert.equal(s.current(), null, 'target is not loaded yet');
  c.click('/s/latest');
  superseded.load();
  const latest = c.frameOf('/s/latest');
  assert.equal(superseded.removed, true, 'an unshown loading target is not cached');
  superseded.load(); // Late original target load has no authority.
  latest.load();
  assert.equal(s.current(), latest);
  assert.equal(s.count(), 1);
  assert.equal(c.click('/s/other', { ctrlKey: true }), false);
  assert.equal(c.click('/tracker/other'), false);
  assert.equal(c.click('https://example.com/'), false);

  const lifecycle = readAssetSource('wsdashboard', 'dashboard-lifecycle.js');
  assert.ok(lifecycle.includes('window.addEventListener("beforeunload", WsRail.leaveGuard('), 'existing native guard mechanism remains');
  assert.ok(!lifecycle.includes('WsShell.touch'));
  assert.ok(!lifecycle.includes('shOrder'));
  assert.equal(c.window.WsShell.touch, undefined);
  assert.equal(c.window.WsShell.LIMIT, undefined);
}
function testCancelledLeave() {
  const c = fixture(), s = c.window.testShell, a = c.first;
  a.cancelLeave = true;
  c.click('/s/b');
  assert.equal(a.saved, 1);
  c.stay(a);
  s.open('/s/a#files', true);
  assert.equal(a.saved, 1, 'I1: same-session route after Stay must not prompt/reload');
  assert.equal(a.contentWindow.location.hash, '#files');
  c.click('/s/a#files');
  assert.equal(a.saved, 1, 'I1: current rail row after Stay must not prompt/reload');
  assert.equal(s.current(), a);
  assert.equal(a.removed, false);
  // A stale cancellation from the prior attempt cannot cancel a later leave.
  const oldAttempt = a.contentWindow.wsShellLeave;
  a.cancelLeave = false; c.click('/s/c');
  c.from(a.contentWindow, { wsShell: 'stay', attempt: oldAttempt });
  a.load();
  assert.ok(c.frameOf('/s/c'));
}
function switchTo(c, path) {
  const previous = c.window.testShell.current();
  c.click(path); previous.load();
  const next = c.frameOf(path); next.load(); return next;
}
function testHistoryCancellation() {
  const c = fixture(), s = c.window.testShell;
  const b = switchTo(c, '/s/b');
  b.cancelLeave = true;
  c.history.go(-1);
  assert.deepEqual(plain(c.historyPaths()), ['/s/a', '/s/b'], 'I2: pending Back cannot overwrite its destination');
  c.stay(b);
  assert.equal(c.position, 1, 'I2: Stay restores the history position');
  assert.deepEqual(plain(c.historyPaths()), ['/s/a', '/s/b']);
  assert.equal(s.current(), b);
  b.cancelLeave = false;
  c.history.go(-1); b.load(); c.frameOf('/s/a').load();
  c.history.go(1); c.frameOf('/s/a').load(); const freshB = c.frameOf('/s/b'); freshB.load();
  assert.equal(s.current(), freshB, 'Back and Forward remain usable after cancellation');
  const last = switchTo(c, '/s/c');
  c.history.go(-1); c.history.go(-1);
  assert.deepEqual(plain(c.historyPaths()), ['/s/a', '/s/b', '/s/c'], 'I2: double Back loses no entry');
  last.load(); c.frameOf('/s/a').load();
  assert.equal(c.location.pathname, '/s/a');
  assert.equal(c.position, 0);
  assert.deepEqual(plain(c.historyPaths()), ['/s/a', '/s/b', '/s/c']);

  const cancelled = fixture();
  switchTo(cancelled, '/s/b'); const kept = switchTo(cancelled, '/s/c');
  kept.cancelLeave = true;
  cancelled.history.go(-1); cancelled.history.go(-1);
  cancelled.history.deferred = true; cancelled.stay(kept);
  cancelled.from(kept.contentWindow, {wsShell: 'hash', hash: '#files'});
  cancelled.window.testShell.open('/s/c', true);
  assert.deepEqual(plain(cancelled.historyPaths()), ['/s/a', '/s/b', '/s/c'], 'hash/pick during async restoration cannot overwrite or truncate entries');
  cancelled.history.flush();
  assert.equal(cancelled.position, 2, 'cancelled double Back restores the original position');
  assert.equal(kept.saved, 1, 'a current-row pick during restoration cannot reprompt');
  assert.deepEqual(plain(cancelled.historyPaths()), ['/s/a', '/s/b', '/s/c']);
}
function draftFixture(failure) {
  const lifecycle = readAssetSource('wsdashboard', 'dashboard-lifecycle.js');
  const draftBlock = lifecycle.slice(lifecycle.indexOf('  var draftStore ='), lifecycle.indexOf('  // --- iframe shell'));
  const dashboard = readAssetSource('wsdashboard', 'dashboard.js');
  const loadSave = dashboard.match(/  function load\(k\)[\s\S]*?\n  function save\(k, v\) \{[\s\S]*?\n  \}/)[0];
  const data = {}, handlers = {}, timers = [], posts = [];
  let fail = failure;
  const context = vm.createContext({ console, rid: 'a', shell: false, framed: true, mode: 'session', imgs: [],
    doc: { addEventListener() {} }, fpane: null, tkBusy: false, visible() { return false; }, clearTimeout() {}, setTimeout(fn) { timers.push(fn); },
    shPost(kind, value) { posts.push({kind, ...value}); },
    store: failure === 'denied' ? null : {
      getItem(key) { return data[key] ?? null; }, removeItem(key) { delete data[key]; },
      setItem(key, value) { if (fail) throw Error('quota'); data[key] = value; }
    }
  });
  context.window = context; context.wsShellLeave = 7;
  context.addEventListener = (type, fn) => { handlers[type] = fn; };
  vm.runInContext(readAssetSource('wsdashboard', 'rail.js') + loadSave + '\nvar railStore = {load: load, save: save};\n' + draftBlock, context);
  context.draft = {text: ''}; vm.runInContext('readDraft = function () { return draft; };', context);
  return { context, timers, posts, recover() { fail = false; },
    leave() { const event = { prompted: false, preventDefault() { this.prompted = true; } }; handlers.beforeunload(event); return event.prompted; }
  };
}
function testStorageFailure() {
  for (const mode of ['quota', 'denied']) {
    const f = draftFixture(mode);
    f.context.draft = {text: 'typed before debounce'};
    f.context.wsSaveSessionDraft();
    assert.equal(f.leave(), true, 'I3: failed ' + mode + ' save must invoke the native leave prompt');
    assert.equal(f.timers.length > 0, true, 'surviving beforeunload schedules a cancellation signal');
    f.timers.splice(0).forEach(fn => fn());
    assert.deepEqual(f.posts, [{kind: 'stay', attempt: 7}]);
    f.context.draft = {text: ''};
    assert.equal(f.leave(), false, 'no prompt for an empty draft despite storage failure');
    f.context.draft = {text: '', open: {topic: true}, tab: 'board'};
    assert.equal(f.leave(), false, 'view state alone is not unsent content');
  }
  const f = draftFixture('quota'); f.context.draft = {text: 'retry'};
  assert.equal(f.leave(), true, 'the guard synchronously saves even if no debounce fired');
  f.recover(); assert.equal(f.leave(), false, 'a successful retry clears the failure');
  f.context.imgs = [{n: 1}]; assert.equal(f.leave(), true, 'pasted images retain the existing guard');
}
function testAcceptedBlankWins() {
  const c = fixture();
  c.click('/s/b'); c.stay(c.first); c.first.load();
  const b = c.frameOf('/s/b');
  assert.ok(b, 'accepted blank load must swap even after stay for the same attempt');
  b.load(); assert.equal(c.window.testShell.current(), b);
  assert.equal(c.first.removed, true);
  assert.equal(c.location.pathname, '/s/b');
  for (const deferred of [false, true]) {
    const back = fixture(), previous = switchTo(back, '/s/b');
    back.history.go(-1);
    back.history.deferred = deferred;
    back.stay(previous); previous.load();
    assert.ok(back.frameOf('/s/a'), 'accepted Back must not strand a blank session');
    back.history.flush(); back.frameOf('/s/a').load();
    assert.equal(back.window.testShell.current(), back.frameOf('/s/a'));
    assert.equal(back.location.pathname, '/s/a', 'accepted Back wins over an earlier Stay traversal');
    assert.deepEqual(plain(back.historyPaths()), ['/s/a', '/s/b']);
    assert.equal(back.timerCount(), 0, 'completed restoration clears its fallback timer');
  }
}
function testRestoreTimeout() {
  const c = fixture(), b = switchTo(c, '/s/b');
  b.cancelLeave = true; c.history.go(-1);
  c.history.go = () => {}; // Joint frame history consumes traversal: no shell popstate.
  c.stay(b);
  c.window.testShell.open('/s/c', true);
  c.advance(999); assert.equal(b.saved, 1, 'queued pick waits briefly for popstate');
  b.cancelLeave = false;
  c.advance(1); assert.equal(b.saved, 2, 'missing popstate cannot strand every later navigation');
  b.load();
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
  b.draft = true; b.contentWindow.typed = 'persisted draft';
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
function testUnsafeCacheFallback() {
  for (const kind of ['unsafe', 'tool']) {
    const c = fixture(5), a = c.first, s = c.window.testShell;
    a[kind] = true; a.cancelLeave = true; c.click('/s/b');
    assert.equal(s.current(), a); assert.equal(c.frameOf('/s/b'), undefined);
    c.stay(a); assert.equal(a.removed, false);
    a.cancelLeave = false; c.click('/s/b'); a.load();
    assert.equal(a.removed, true); assert.ok(c.frameOf('/s/b'));
  }
  const f = draftFixture('quota');
  f.context.supChats = {}; f.context.q = () => [];
  f.context.draft = {text: 'cannot persist'};
  assert.equal(f.context.wsCanCacheSession(), false, 'failed storage never bypasses native leave');
  f.recover(); assert.equal(f.context.wsCanCacheSession(), true, 'saved text may be cached');
  f.context.imgs = [{n: 1}]; assert.equal(f.context.wsCanCacheSession(), false, 'unpersisted image stays guarded');
  f.context.imgs = []; f.context.q = () => [{}]; assert.equal(f.context.wsCanCacheSession(), false, 'tool/foreign frames stay fresh');
  f.context.q = () => []; f.context.supChats = {chat: {}}; assert.equal(f.context.wsCanCacheSession(), false, 'support state stays fresh');
  f.context.supChats = {}; f.context.WsTerm = {cacheSafe: () => false};
  assert.equal(f.context.wsCanCacheSession(), false, 'pending terminal list/create never enters cache');
  f.context.WsTerm.cacheSafe = () => true; assert.equal(f.context.wsCanCacheSession(), true);
}
function testCacheAdmissionExpiry() {
  for (const cancel of [false, true]) {
    const c = fixture(5), s = c.window.testShell, a = c.first;
    c.click('/s/b'); const ready = s.pending();
    a.tool = true; a.cancelLeave = cancel; ready.load();
    assert.equal(s.current(), a, 'eligibility is checked again before hiding');
    assert.equal(a.got.includes('shown:false'), false, 'unsafe outgoing runtime is never parked');
    assert.equal(a.inert, false, 'native leave remains interactive/cancellable');
    assert.equal(ready.got.at(-1), 'shown:false', 'ready destination is suspended while native leave awaits a decision');
    if (cancel) {
      c.stay(a);
      assert.equal(c.location.pathname, '/s/a'); assert.equal(s.current(), a); assert.equal(a.removed, false);
      assert.equal(ready.removed, true, 'Stay discards the never-shown destination runtime');
      assert.equal(s.pending(), null); assert.equal(s.count(), 1);
      // A tentative Stay may race an already accepted blank navigation.
      a.pending = 'about:blank'; a.load();
      const accepted = c.frameOf('/s/b'); assert.ok(accepted); assert.notEqual(accepted, ready);
      accepted.load(); assert.equal(s.current(), accepted); assert.equal(c.location.pathname, '/s/b');
      ready.load(); assert.equal(s.current(), accepted, 'discarded destination never regains authority');
    } else {
      assert.equal(a.pending, 'about:blank', 'late tool state takes the native destruction path');
      a.load(); assert.equal(a.removed, true); assert.equal(s.current(), ready, 'ready destination is reused after native acceptance');
    }
  }
}
function run() { runBase(); testCancelledLeave(); testHistoryCancellation(); testStorageFailure(); testAcceptedBlankWins(); testRestoreTimeout(); testCacheReuse(); testCacheEvictionAndRaces(); testUnsafeCacheFallback(); testCacheAdmissionExpiry(); }
module.exports = run;
if (require.main === module) {
  const test = require('node:test');
  test('fresh session frames, drafts, races and history', runBase);
  test('I1: Stay clears leave intent before same-session routes and rail clicks', testCancelledLeave);
  test('I2: cancelled Back restores position and double Back preserves entries', testHistoryCancellation);
  test('I3: denied/full storage prompts before discarding a typed draft', testStorageFailure);
  test('hardening: accepted blank load wins over a stale stay', testAcceptedBlankWins);
  test('hardening: missing restoration popstate times out and flushes navigation', testRestoreTimeout);
  test('bounded cache reuses documents, drafts, hashes and Back/Forward', testCacheReuse);
  test('cache LRU eviction, stale authority and superseded cold loads', testCacheEvictionAndRaces);
  test('unsafe images, storage, tools and support retain native fresh navigation', testUnsafeCacheFallback);
  test('late tool admission during a cold load stays native-guarded, including Stay history', testCacheAdmissionExpiry);
}
