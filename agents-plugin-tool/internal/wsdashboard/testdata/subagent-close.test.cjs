const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const src = readAssetSource('wsdashboard', 'dashboard.js');
const start = src.indexOf('  var subagentClosing =');
const end = src.indexOf('  // leadChoices reads', start);
assert(start >= 0 && end > start, 'shared close helper is present');
function setup(control = true) {
  const requests = [], events = [], notices = [], dialogs = [], listeners = {};
  let status = 200;
  function el(tag, cls, text) {
    return { tag, className: cls, textContent: text || '', children: [], attrs: {}, listeners: {}, isConnected: true,
      appendChild(c) { this.children.push(c); return c; }, setAttribute(k,v) { this.attrs[k] = v; },
      getAttribute(k) { return this.attrs[k]; }, hasAttribute(k) { return k in this.attrs; },
      addEventListener(k,f) { this.listeners[k] = f; }, focus() { this.focused = true; },
      showModal() { this.open = true; }, close() { this.open = false; }, remove() { this.isConnected = false; },
      click() { this.listeners.click?.({ target: this, preventDefault() {} }); } };
  }
  const document = { body: { appendChild(d) { dialogs.push(d); } }, querySelectorAll() { return []; },
    addEventListener(k,f) { listeners[k] = f; }, dispatchEvent(e) { events.push(e); } };
  const context = { document, root: { hasAttribute() { return control; } }, mk: el, rpBtn: (cls, text) => el('button', cls, text), rpId: () => 'dialog-id',
    wsT: (key, value) => value ? key + ':' + value : key, leadErr: (s,t) => t || `HTTP ${s}`,
    toast: msg => notices.push(msg), CustomEvent: function(type, opts) { this.type = type; this.detail = opts.detail; },
    fetch: (url, options) => { requests.push({url, options}); return Promise.resolve({status, text: () => Promise.resolve('')}); } };
  vm.createContext(context); vm.runInContext(src.slice(start, end), context);
  return { context, requests, events, notices, dialogs, listeners, el, setStatus(n) { status = n; } };
}
const settled = () => new Promise(resolve => setImmediate(resolve));
test('idle, done and failed close immediately, once, with encoded id', async () => {
  for (const state of ['idle', 'done', 'failed']) {
    const h = setup(), trigger = h.el('button');
    h.context.subagentClose('a/b', 'child', state, trigger);
    h.context.subagentClose('a/b', 'child', state, trigger);
    await settled();
    assert.equal(h.requests.length, 1); assert.equal(h.requests[0].url, '/agents/a%2Fb/close');
    assert.equal(h.requests[0].options.method, 'POST'); assert.equal(h.dialogs.length, 0);
    assert.equal(h.events[0].detail.replyId, 'a/b');
  }
});
test('active states cancel without sending and confirm exactly once', async () => {
  for (const state of ['thinking', 'tool', 'holding', 'permission']) {
    const h = setup(), trigger = h.el('button');
    h.context.subagentClose('child', 'Child', state, trigger);
    let d = h.dialogs.at(-1), row = d.children.at(-1);
    assert.equal(d.open, true); assert.match(d.children[1].textContent, new RegExp(state));
    row.children[0].click(); assert.equal(h.requests.length, 0); assert.equal(trigger.focused, true);
    h.context.subagentClose('child', 'Child', state, trigger);
    d = h.dialogs.at(-1); row = d.children.at(-1); row.children[1].click();
    h.context.subagentClose('child', 'Child', state, trigger);
    await settled(); assert.equal(h.requests.length, 1); assert.equal(d.open, false);
  }
});
test('local gate, failed request and retry', async () => {
  const denied = setup(false); denied.context.subagentClose('x', 'X', 'idle', denied.el('button'));
  assert.equal(denied.requests.length, 0); assert.equal(denied.dialogs.length, 0);
  const h = setup(), trigger = h.el('button'); h.setStatus(500);
  h.context.subagentClose('x', 'X', 'idle', trigger); await settled();
  assert.equal(trigger.disabled, false); assert.equal(h.notices.length, 1);
  h.setStatus(200); h.context.subagentClose('x', 'X', 'idle', trigger); await settled();
  assert.equal(h.requests.length, 2); assert.equal(h.events.length, 1);
});
test('child header delegates to the shared close action', async () => {
  const h = setup(), b = h.el('button');
  b.setAttribute('data-reply-id', 'child'); b.setAttribute('data-label', 'Child'); b.setAttribute('data-state', 'idle');
  h.listeners.click({target: {closest: selector => selector === '[data-child-close]' ? b : null}});
  await settled(); assert.equal(h.requests[0].url, '/agents/child/close');
});
