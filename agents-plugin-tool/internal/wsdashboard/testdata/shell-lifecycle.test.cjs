// Offline VM contract only: no server, browser, executable probe or child process.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');

function fixture() {
  const go = fs.readFileSync(path.join(__dirname, '../shell_frame_test.go'), 'utf8');
  const harness = go.match(/const shellGlueHarness = `([\s\S]*?)\n`/)[1];
  const js = readAssetSource('wsdashboard', 'dashboard.js');
  new vm.Script(js); // Also parse the complete shipped fragment concatenation.
  const glue = js.match(/\n  var shFrames = \[\][\s\S]*?\n  \/\/ shFrameInit wires/)[0];
  const init = js.match(/\n  function shShellInit\(\) \{[\s\S]*?\n  \}\n/)[0];
  const context = vm.createContext({ URL, console });
  vm.runInContext(harness + readAssetSource('wsdashboard', 'shell.js') + '\n(function () {\n' + glue + init + `
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
function plain(value) { return JSON.parse(JSON.stringify(value)); }
function run() {
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
  c.location.pathname = '/s/a'; c.location.hash = '';
  c.listeners.popstate.forEach(fn => fn({}));
  assert.equal(c.location.pathname, '/s/d', 'pending/cancelled leave keeps the current address');
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
  assert.ok(lifecycle.includes('if (framed) window.wsSaveSessionDraft = saveDraft;'));
  assert.ok(lifecycle.includes('window.addEventListener("beforeunload", WsRail.leaveGuard(function () { return imgs.some(function (p) { return !p.ref; }); }));'), 'existing native pasted-image guard stays unchanged');
  assert.ok(!lifecycle.includes('WsShell.touch'));
  assert.ok(!lifecycle.includes('shOrder'));
  assert.equal(c.window.WsShell.touch, undefined);
  assert.equal(c.window.WsShell.LIMIT, undefined);
}
module.exports = run;
if (require.main === module) require('node:test')('fresh session frames, native-guard cancellation, drafts, races and history', run);
