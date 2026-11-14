// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/lead-popover.test.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '..', 'assets', 'dashboard.js'), 'utf8');
const maker = source.match(/\n  function rpLeadMake\(\) \{[\s\S]*?\n  \}\n/);
assert(maker, 'production lead popover maker exists');

function fixture(harness) {
  const nodes = [], events = [], requests = [];
  function mk(tag, cls, text) {
    const n = { tag, cls, textContent: text || '', value: '', hidden: false, disabled: false,
      attrs: {}, children: [], listeners: {},
      appendChild(child) { this.children.push(child); return child; },
      setAttribute(k, v) { this.attrs[k] = String(v); },
      getAttribute(k) { return this.attrs[k] || ''; },
      addEventListener(k, fn) { this.listeners[k] = fn; },
      focus() { events.push('focus'); } };
    nodes.push(n);
    return n;
  }
  let active;
  const ctx = vm.createContext({
    mk, wsT: k => k, rpPop: () => mk('div'), rpBtn: (cls, text) => mk('button', cls, text),
    leadRows: () => ({ model: { value: '' }, effort: { value: '' }, mode: { value: 'auto' }, set() {}, notes() {} }),
    supHarness: () => ({ available: true }), leadChoices: done => done(),
    railPopPlace() {}, notifAsk() {}, gIcon: () => mk('svg'),
    rpLive: (p, gen) => active === p && p.gen === gen,
    railPopClose(refocus) { assert.equal(refocus, false); active.el.hidden = true; active = null; events.push('close'); },
    saveDraft: () => events.push('draft'), go: href => events.push(['navigate', href]),
    toast: text => events.push(['toast', text]), leadErr: (st, text) => text || 'HTTP ' + st,
    WsRail: { leadBody: (root, options) => ({ root, ...options }) },
    railPost: (route, body, done) => requests.push({ route, body, done })
  });
  vm.runInContext(maker[0], ctx);
  const p = active = ctx.rpLeadMake();
  p.open('/repo', 'repo', mk('button'));
  p.el.hidden = false;
  if (harness === 'pi') {
    const button = nodes.find(n => n.attrs['data-harness'] === 'pi');
    nodes.find(n => n.listeners.click && n.attrs['aria-label'] === 'js.sup.harness')
      .listeners.click({ target: { closest: () => button } });
  }
  return { p, nodes, events, requests,
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
  // A completed HTTP response is sufficient: no SSE/attach event or timer.
  f.requests[0].done(201, { id: 'new/session' }, '');
  assert.equal(f.p.el.hidden, true);
  assert.equal(f.p.busy, false);
  assert.equal(f.start.disabled, false);
  assert.equal(f.status.textContent, '');
  assert.deepEqual(f.events, ['close', 'draft', ['navigate', '/s/new%2Fsession']]);

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
    e.start.listeners.click();
    assert.equal(e.requests.length, 2, 'failure permits retry');
    assert.equal(e.error.textContent, '');
  }

  const dismissed = fixture(harness);
  dismissed.start.listeners.click();
  dismissed.dismiss();
  dismissed.requests[0].done(201, { id: 'created' }, '');
  assert.deepEqual(dismissed.events, ['draft', ['navigate', '/s/created']], 'late success still opens the created session without closing another popover');
}
console.log('lead popover cases passed');
