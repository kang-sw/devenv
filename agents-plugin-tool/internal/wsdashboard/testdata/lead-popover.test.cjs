// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/lead-popover.test.cjs
const assert = require('node:assert/strict');
const { readAssetSource } = require('./asset-source.cjs');
const vm = require('node:vm');
const source = readAssetSource('wsdashboard', 'dashboard.js');
const maker = source.match(/  var LEAD_MODES = [\s\S]*?\n  function rpLeadMake\(\) \{[\s\S]*?\n  \}\n/);
assert(maker, 'production lead choices, rows and popover maker exist');
const rail = readAssetSource('wsdashboard', 'rail.js');
const legacy = [
  { harness: 'claude', available: true, models: [{ id: 'opus' }], efforts: ['high'] },
  { harness: 'pi', available: true, models: [{ id: 'pi-model' }], efforts: ['off'] }
];

function fixture(harness, choices = legacy, delayed = false) {
  const nodes = [], events = [], requests = [], storage = {};
  function mk(tag, cls, text) {
    const n = { tag, cls, textContent: text || '', value: '', hidden: false, disabled: false,
      attrs: {}, children: [], listeners: {},
      appendChild(child) { child.parentNode = this; this.children.push(child); return child; },
      setAttribute(k, v) { this.attrs[k] = String(v); },
      getAttribute(k) { return this.attrs[k] || ''; },
      addEventListener(k, fn) { this.listeners[k] = fn; },
      focus() { events.push('focus'); } };
    Object.defineProperty(n, 'textContent', {
      get() { return this.text || ''; },
      set(value) { this.text = value; this.children = []; }
    });
    n.textContent = text || '';
    const classes = new Set((cls || '').split(' '));
    n.classList = {
      toggle(k, on) { if (on) classes.add(k); else classes.delete(k); },
      contains(k) { return classes.has(k); }
    };
    nodes.push(n);
    return n;
  }
  let active, answerChoices, seq = 0;
  const ctx = vm.createContext({
    mk, wsT: k => k, rpPop: () => mk('div'), rpBtn: (cls, text) => mk('button', cls, text),
    supChoices: delayed ? null : { harnesses: choices },
    rpId: () => 'row-' + ++seq,
    tkFetch: (route, options, done) => { assert.equal(route, '/support/choices'); answerChoices = () => done(200, { harnesses: choices }); },
    window: {}, Date: { now: () => 12345 }, RENAME_NEW: 'ws.dashboard.renameNew',
    sessionStorage: { setItem(key, value) { storage[key] = value; events.push('rename-intent'); } },
    railOpen(open) { assert.equal(open, false); events.push('rail-close'); },
    railPopPlace() {}, notifAsk() {}, gIcon: () => mk('svg'),
    rpLive: (p, gen) => active === p && p.gen === gen,
    railPopClose(refocus) { assert.equal(refocus, false); active.el.hidden = true; active = null; events.push('close'); },
    saveDraft: () => events.push('draft'), go: href => events.push(['navigate', href]),
    toast: text => events.push(['toast', text]), leadErr: (st, text) => text || 'HTTP ' + st,
    railPost: (route, body, done, timeout) => requests.push({ route, body, done, timeout })
  });
  vm.runInContext(rail, ctx);
  ctx.WsRail = ctx.window.WsRail;
  vm.runInContext(source.match(/\n  function supHarness\(name\) \{[\s\S]*?\n  \}\n/)[0] + maker[0], ctx);
  const p = active = ctx.rpLeadMake();
  p.open('/repo', 'repo', mk('button'));
  p.el.hidden = false;
  const harnessGroup = nodes.find(n => n.listeners.click && n.attrs['aria-label'] === 'js.sup.harness');
  function pick(key) {
    const button = harnessGroup.children.find(n => n.attrs['data-harness'] === key);
    assert(button, 'harness button exists: ' + key);
    harnessGroup.listeners.click({ target: { closest: () => button } });
  }
  if (harness) pick(harness);
  const selects = nodes.filter(n => n.tag === 'select');
  return { p, nodes, events, requests, storage, pick, harnessGroup,
    choicesReady() { answerChoices(); },
    model: selects[0], mode: selects[1], effort: selects[2],
    newRows(key, values) { const rows = ctx.leadRows(mk('div'), () => {}); rows.set(key, values); return rows; },
    start: nodes.find(n => n.tag === 'button' && n.textContent === 'js.lead.start'),
    status: nodes.find(n => n.tag === 'span' && n.cls === 'spnote'),
    error: nodes.find(n => n.cls === 'spnote err'),
    dismiss() { active = null; p.el.hidden = true; } };
}

for (const harness of ['claude', 'pi']) {
  const f = fixture(harness);
  f.start.listeners.click();
  assert.equal(f.p.busy, true);
  assert.equal(f.start.disabled, true);
  assert.equal(f.status.textContent, 'js.lead.starting');
  f.start.listeners.click();
  assert.equal(f.requests.length, 1, 'busy prevents duplicate creation');
  assert.equal(f.requests[0].route, '/agents/leads');
  assert.equal(f.requests[0].body.harness, harness);
  assert.equal(f.requests[0].timeout, 20000, 'a spawn that never answers ends instead of staying busy');
  // A completed HTTP response is sufficient: no SSE/attach event or timer.
  f.requests[0].done(201, { id: 'new/session' }, '');
  assert.equal(f.p.el.hidden, true);
  assert.equal(f.p.busy, false);
  assert.equal(f.start.disabled, false);
  assert.equal(f.status.textContent, '');
  assert.deepEqual(f.events, ['close', 'rail-close', 'draft', 'rename-intent', ['navigate', '/s/new%2Fsession']]);
  assert.deepEqual(JSON.parse(f.storage['ws.dashboard.renameNew']), { id: 'new/session', at: 12345 },
    'successful creation hands the raw session ID and timestamp to post-navigation rename');

  for (const status of [400, 500, 0]) {
    const e = fixture(harness);
    e.start.listeners.click();
    e.requests[0].done(status, {}, 'launch failed');
    assert.equal(e.p.el.hidden, false);
    assert.equal(e.p.busy, false);
    assert.equal(e.start.disabled, false);
    assert.equal(e.status.textContent, '');
    assert.equal(e.error.textContent, 'launch failed');
    assert.deepEqual(e.events, []);
    assert.deepEqual(e.storage, {}, 'failed creation must not schedule rename');
    e.start.listeners.click();
    assert.equal(e.requests.length, 2, 'failure permits retry');
    assert.equal(e.error.textContent, '');
  }

  const dismissed = fixture(harness);
  dismissed.start.listeners.click();
  dismissed.dismiss();
  dismissed.requests[0].done(201, { id: 'created' }, '');
  assert.deepEqual(dismissed.events, ['rail-close', 'draft', 'rename-intent', ['navigate', '/s/created']], 'late success still opens the created session without closing another popover');
  assert.deepEqual(JSON.parse(dismissed.storage['ws.dashboard.renameNew']), { id: 'created', at: 12345 });
}
// Lead-marked descriptors replace, rather than extend, the fixed buttons.
const custom = { harness: 'custom', profile: '', lead: true, available: true,
  models: [{ id: 'custom-model' }], efforts: ['deep'], permission_modes: ['review', 'auto', 'plan'] };
const unavailable = { harness: 'missing', profile: '', lead: true, available: false,
  error: 'not installed', permission_modes: ['review'] };
const noPermission = { harness: 'plain', profile: '', lead: true, available: true };
const harnessNames = f => f.harnessGroup.children.map(n => n.attrs['data-harness']);
const optionNames = sel => sel.children.map(n => n.value);
{
  const f = fixture(null, [...legacy, unavailable, custom, noPermission]);
  assert.deepEqual(harnessNames(f), ['missing', 'custom', 'plain']);
  assert.equal(f.p.harness, 'custom', 'first available admitted lead is selected');
  assert.equal(f.harnessGroup.children[0].disabled, true);
  f.pick('missing');
  assert.equal(f.p.harness, 'custom', 'unavailable lead cannot be selected');
  assert.deepEqual(optionNames(f.mode), ['review', 'auto', 'plan']);
  assert.equal(f.mode.value, 'review', 'custom default is its first choice, not Claude auto');
  assert.equal(f.mode.disabled, false);
  f.model.value = 'custom-model';
  f.effort.value = 'deep';
  f.mode.value = 'plan';
  f.mode.listeners.change();
  const note = f.nodes.find(n => n.id === f.mode.attrs['aria-describedby']);
  assert.equal(note.textContent, 'plan', 'custom permission names do not acquire Claude descriptions');
  assert.equal(note.classList.contains('warn'), false);
  const iso = f.nodes.find(n => n.attrs['data-iso'] === 'isolated');
  f.nodes.find(n => n.listeners.click && n.attrs['aria-label'] === 'js.lead.iso')
    .listeners.click({ target: { closest: () => iso } });
  f.start.listeners.click();
  assert.deepEqual(JSON.parse(JSON.stringify(f.requests[0].body)), {
    root: '/repo', harness: 'custom', plugin_isolation: 'isolated',
    model: 'custom-model', effort: 'deep', permission_mode: 'plan'
  });
  const rows = f.newRows('custom', { mode: 'plan' });
  rows.disable(true);
  assert.equal(rows.mode.disabled, true, 'busy session settings disable custom permission modes too');
  rows.disable(false);
  assert.equal(rows.mode.disabled, false);
}
{
  const f = fixture(null, [custom, noPermission]);
  f.pick('plain');
  assert.equal(f.mode.disabled, true);
  assert.equal(f.mode.parentNode.classList.contains('off'), true);
  assert.deepEqual(optionNames(f.mode), []);
  assert.equal(f.model.disabled, true, 'absent model choices do not enable a control');
  assert.equal(f.effort.disabled, true, 'absent effort choices do not enable a control');
  // Even stale values on unsupported controls never enter the request.
  f.mode.value = 'plan'; f.model.value = 'old-model'; f.effort.value = 'deep';
  f.start.listeners.click();
  assert.deepEqual(JSON.parse(JSON.stringify(f.requests[0].body)), {
    root: '/repo', harness: 'plain', plugin_isolation: 'default'
  });
  const rows = f.newRows('plain', {});
  rows.disable(true); rows.disable(false);
  assert.equal(rows.mode.disabled, true, 'a redraw does not enable absent permissions');
}
{
  const f = fixture(null, [{ ...noPermission, harness: 'constructor' }]);
  assert.equal(f.p.harness, 'constructor', 'backend names do not inherit object properties');
  assert.equal(f.effort.disabled, true);
}
{
  const f = fixture(null, [unavailable]);
  assert.equal(f.start.disabled, true);
  assert.equal(f.mode.disabled, true);
  f.start.listeners.click();
  assert.equal(f.requests.length, 0, 'no launch when every admitted lead is unavailable');
  f.p.focus();
  assert.deepEqual(f.events, ['focus']);
}
{
  const f = fixture(null, [{ harness: 'claude', lead: true, available: true }]);
  assert.equal(f.mode.disabled, true, 'descriptor permission absence also applies to Claude');
}
{
  const modes = ['default', 'auto', 'plan'];
  const f = fixture(null, legacy.map(h => ({ ...h, lead: true,
    ...(h.harness === 'claude' ? { permission_modes: modes } : {}) })));
  assert.deepEqual(harnessNames(f), ['claude', 'pi']);
  assert.equal(f.p.harness, 'claude');
  assert.deepEqual(optionNames(f.mode), modes, 'Claude also renders only descriptor modes');
  assert.equal(f.mode.value, 'auto', 'Claude retains its existing initial mode');
  f.pick('pi');
  assert.equal(f.mode.disabled, true, 'new Pi descriptor has no permission control');
  assert.deepEqual(optionNames(f.mode), []);
  f.start.listeners.click();
  assert.equal('permission_mode' in f.requests[0].body, false);
}
{
  const f = fixture(null, [...legacy, { ...custom, lead: false }]);
  assert.deepEqual(harnessNames(f), ['claude', 'pi'], 'without any lead marker retain the fixed controls');
  assert.equal(f.p.harness, 'claude');
  assert.deepEqual(optionNames(f.mode), ['auto', 'default', 'acceptEdits', 'plan', 'dontAsk', 'bypassPermissions']);
  assert.equal(f.mode.value, 'auto');
  f.pick('pi');
  assert.equal(f.mode.disabled, true);
  f.start.listeners.click();
  assert.equal('permission_mode' in f.requests[0].body, false, 'old Pi request stays mode-free');
  assert.equal(f.requests[0].body.plugin_isolation, 'default');
}
{
  const f = fixture(null, [{ ...legacy[0], available: false }, legacy[1]]);
  assert.equal(f.p.harness, 'pi', 'legacy fallback still picks Pi when Claude is unavailable');
}
{
  const f = fixture(null, [custom], true);
  assert.deepEqual(harnessNames(f), ['claude', 'pi']);
  f.choicesReady();
  assert.deepEqual(harnessNames(f), ['custom'], 'async choices replace fallback buttons');
  assert.equal(f.p.harness, 'custom');
  assert.deepEqual(optionNames(f.mode), ['review', 'auto', 'plan']);
  assert.equal(f.mode.value, 'review', 'a newly selected backend does not inherit the fallback mode');
}
// railPost itself: a request that never answers ends once, at the timeout,
// with the timeout text, and aborts the fetch; a late answer is ignored.
{
  const post = source.match(/\n  function railPost\(route, body, done, timeout\) \{[\s\S]*?\n  \}\n/);
  assert(post, 'production railPost exists');
  const timers = [], aborted = [], calls = [];
  let answer;
  const ctx = vm.createContext({
    wsT: k => k,
    setTimeout: (fn, ms) => { timers.push({ fn, ms }); return timers.length; },
    clearTimeout: id => { timers[id - 1].cleared = true; },
    window: {
      fetch: () => new Promise(resolve => { answer = resolve; }),
      AbortController: class { constructor() { this.signal = {}; } abort() { aborted.push(true); } }
    }
  });
  ctx.fetch = ctx.window.fetch;
  vm.runInContext(post[0], ctx);
  ctx.railPost('/agents/leads', {}, (st, v, s) => calls.push([st, s]), 20000);
  assert.equal(timers.length, 1);
  assert.equal(timers[0].ms, 20000);
  timers[0].fn();
  assert.deepEqual(calls, [[0, 'js.post.timeout']]);
  assert.equal(aborted.length, 1);
  answer({ ok: true, status: 201, text: () => Promise.resolve('{"id":"late"}') });
  setImmediate(() => {
    assert.deepEqual(calls, [[0, 'js.post.timeout']], 'a late answer after the timeout is ignored');
    // Without a timeout nothing is armed and the answer arrives as before.
    const plain = [];
    ctx.railPost('/x', {}, (st, v) => plain.push([st, v.id]));
    assert.equal(timers.length, 1);
    answer({ ok: true, status: 201, text: () => Promise.resolve('{"id":"ok"}') });
    setImmediate(() => {
      assert.deepEqual(plain, [[201, 'ok']]);
      console.log('lead popover cases passed');
    });
  });
}
