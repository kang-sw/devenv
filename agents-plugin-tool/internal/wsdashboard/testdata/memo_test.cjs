// Minimal DOM and virtual clock, like the dashboard's existing asset harnesses.
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
class El {
  constructor(tag = 'div', cls = '') { this.tagName = tag; this.className = cls; this.children = []; this.attrs = {}; this.on = {}; this.hidden = false; this.value = ''; this.style = { setProperty() {} }; }
  set textContent(s) { this.children = []; this.text = s; }
  get textContent() { return (this.text || '') + this.children.map(c => c.textContent).join(''); }
  appendChild(c) { this.children.push(c); c.parentNode = this; return c; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(type, fn) { (this.on[type] ||= []).push(fn); }
  fire(type, props = {}) { const e = { target: this, preventDefault() {}, stopPropagation() {}, ...props }; (this.on[type] || []).forEach(fn => fn(e)); }
  focus() { focused = this; } blur() { if (focused === this) focused = null; }
  matches(sel) { return sel.startsWith('.') ? this.className.split(' ').includes(sel.slice(1)) : this.tagName === sel; }
  closest(sel) { return this.matches(sel) ? this : this.parentNode?.closest(sel); }
  querySelectorAll(sel) {
    const bits = sel.split(' '), out = [];
    function walk(el) { for (const c of el.children) { if (c.matches(bits.at(-1)) && (bits.length === 1 || c.parentNode.closest(bits[0]))) out.push(c); walk(c); } }
    walk(this); return out;
  }
  querySelector(sel) { return this.querySelectorAll(sel)[0] || null; }
  getBoundingClientRect() { return { right: 352, width: 340, top: 12 }; }
}
let focused, timerID = 0, now = 0, timers = [];
const ids = {};
for (const id of ['memo-input', 'memo-list', 'memo-workspace', 'memo-status', 'memo-undo', 'memo-close']) ids[id] = new El(id === 'memo-input' ? 'textarea' : 'div');
ids['memo-undo'].appendChild(new El('button'));
const rail = new El(), ws = rail.appendChild(new El('section', 'rws'));
ws.setAttribute('data-ws', '/repo'); ws.appendChild(new El('span', 'nm')).textContent = 'repo';
const row = ws.appendChild(new El('div', 'rs')); row.setAttribute('data-rid', 'session');
const badge = ws.appendChild(new El('button', 'memo-count')); badge.appendChild(new El('span'));
const doc = { getElementById: id => ids[id], createElement: tag => new El(tag), createTextNode: text => { const e = new El('#text'); e.textContent = text; return e; } };
let remote = [], nextID = 0, calls = [], local = true, pendingRead = null, failWrite = false;
const storage = new Map();
const window = { addEventListener() {}, sessionStorage: { setItem: (k,v) => storage.set(k,v), getItem: k => storage.get(k), removeItem: k => storage.delete(k) }, fetch: async (url, opts) => {
  calls.push([url, opts]);
  if (opts.method === 'POST') {
    if (failWrite) { failWrite = false; return { ok: false, text: async () => 'save failed' }; }
    const req = JSON.parse(opts.body);
    if (req.action === 'add') remote.unshift({ id: String(++nextID), text: req.text.trim() });
    if (req.action === 'edit') remote.find(m => m.id === req.id).text = req.text.trim();
    if (req.action === 'delete') remote = remote.filter(m => m.id !== req.id);
  } else if (pendingRead) { const hold = pendingRead; pendingRead = null; return hold; }
  return { ok: true, json: async () => ({ memos: remote.map(m => ({ ...m })), writable: local }) };
} };
const root = fs.existsSync('assets/memo.js') ? '.' : 'agents-plugin-tool/internal/wsdashboard';
vm.runInNewContext(fs.readFileSync(path.join(root, 'assets/memo.js'), 'utf8'), { window, Promise, setTimeout(fn, ms) { const id = ++timerID; timers.push({ id, at: now + ms, fn }); return id; }, clearTimeout(id) { timers = timers.filter(t => t.id !== id); } });
async function flush() { for (let i = 0; i < 15; i++) await Promise.resolve(); }
function advance(ms) { now += ms; for (const t of timers.filter(t => t.at <= now)) t.fn(); timers = timers.filter(t => t.at > now); }
(async () => {
  const panel = new El(), memo = window.WsMemo.mount(doc, rail, panel, () => {});
  memo.sync('session'); memo.open(true, false); await flush();
  assert.equal(focused, ids['memo-input']);
  ids['memo-input'].value = '<b>plain</b> https://example.com\nsecond line';
  ids['memo-input'].fire('keydown', { key: 'Enter', shiftKey: true }); await flush();
  assert.equal(remote.length, 0, 'Shift+Enter stays a newline');
  ids['memo-input'].fire('keydown', { key: 'Enter', isComposing: true }); await flush();
  assert.equal(remote.length, 0, 'IME Enter does not save');
  ids['memo-input'].fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(remote.length, 1); assert.equal(ids['memo-input'].value, '');
  assert.equal(ids['memo-list'].querySelectorAll('a').length, 1, 'only URL is a link');
  assert.equal(ids['memo-list'].querySelectorAll('b').length, 0, 'HTML stays text');
  assert.equal(badge.querySelector('span').textContent, '1');
  ids['memo-input'].value = 'newest'; ids['memo-input'].fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(remote[0].text, 'newest');
  ids['memo-list'].querySelector('.memo-text').fire('click');
  let area = ids['memo-list'].querySelector('textarea'); area.value = 'cancelled'; area.fire('keydown', { key: 'Escape' });
  assert.equal(remote[0].text, 'newest'); assert.equal(ids['memo-list'].querySelector('textarea'), null);
  ids['memo-list'].querySelector('.memo-text').fire('click');
  area = ids['memo-list'].querySelector('textarea'); area.value = 'edited'; area.fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(remote[0].text, 'edited');
  ids['memo-list'].querySelector('.memo-text').fire('click');
  area = ids['memo-list'].querySelector('textarea'); area.value = 'unsaved'; failWrite = true;
  area.fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(ids['memo-list'].querySelector('textarea').value, 'unsaved', 'failed save keeps the edit');
  assert.equal(ids['memo-status'].textContent, 'save failed'); area.fire('keydown', { key: 'Escape' });
  ids['memo-list'].querySelector('.memo-delete').fire('click'); await flush();
  assert.equal(remote.length, 1); assert.equal(ids['memo-undo'].hidden, false);
  ids['memo-input'].value = 'unsent draft';
  ids['memo-undo'].querySelector('button').fire('click'); await flush();
  assert.equal(ids['memo-input'].value, 'unsent draft', 'undo preserves the new memo draft');
  assert.equal(remote.length, 2); assert.equal(remote[0].text, 'edited'); assert.equal(ids['memo-undo'].hidden, true);
  ids['memo-list'].querySelector('.memo-delete').fire('click'); await flush(); advance(5000);
  assert.equal(ids['memo-undo'].hidden, true, 'undo expires after five seconds');
  memo.remember(); assert.equal(memo.resume(), true); assert.equal(memo.resume(), false);
  memo.open(false, false); remote = [{ id: 'other', text: 'from another tab' }];
  memo.open(true, true); await flush(); assert.equal(ids['memo-list'].textContent.includes('from another tab'), true);
  local = false; memo.open(true, true); await flush();
  assert.equal(ids['memo-input'].hidden, true); assert.equal(ids['memo-list'].querySelectorAll('button').length, 0);
  assert.equal(ids['memo-status'].textContent, 'Read-only on LAN');
  const otherWS = rail.appendChild(new El('section', 'rws')); otherWS.setAttribute('data-ws', '/other');
  otherWS.appendChild(new El('span', 'nm')).textContent = 'other';
  const otherRow = otherWS.appendChild(new El('div', 'rs')); otherRow.setAttribute('data-rid', 'other-session');
  memo.sync('other-session'); await flush();
  assert.equal(ids['memo-workspace'].textContent, 'other');
  assert.equal(calls.at(-1)[0], '/rail/memos?workspace=%2Fother');
  let release;
  pendingRead = new Promise(resolve => { release = resolve; });
  memo.open(true, false); memo.open(false, false);
  release({ ok: true, json: async () => ({ memos: [], writable: true }) }); await flush();
  assert.notEqual(focused, ids['memo-input'], 'closed panel does not steal focus after delayed read');
})().catch(e => { console.error(e); process.exitCode = 1; });
