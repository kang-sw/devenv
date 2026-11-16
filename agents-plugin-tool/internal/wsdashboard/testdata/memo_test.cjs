// Minimal DOM and virtual clock, like the dashboard's existing asset harnesses.
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');
class El {
  constructor(tag = 'div', cls = '') { this.tagName = tag; this.className = cls; this.children = []; this.attrs = {}; this.on = {}; this.hidden = false; this.value = ''; this.style = { setProperty() {} }; }
  set textContent(s) { this.children = []; this.text = s; }
  get textContent() { return (this.text || '') + this.children.map(c => c.textContent).join(''); }
  appendChild(c) { if (c.parentNode) c.parentNode.children = c.parentNode.children.filter(x => x !== c); this.children.push(c); c.parentNode = this; return c; }
  // Layout stand-ins: a note is as tall as its text is long; the list is two columns wide.
  get offsetHeight() { return this.tagName === 'article' ? 20 + 10 * this.textContent.length : 0; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(type, fn) { (this.on[type] ||= []).push(fn); }
  fire(type, props = {}) { const e = { target: this, preventDefault() { this.defaultPrevented = true; }, stopPropagation() {}, ...props }; (this.on[type] || []).forEach(fn => fn(e)); }
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
for (const id of ['memo-input', 'memo-list', 'memo-workspace', 'memo-n', 'memo-status', 'memo-undo', 'memo-close']) ids[id] = new El(id === 'memo-input' ? 'textarea' : 'div');
ids['memo-undo'].appendChild(new El('button')); ids['memo-list'].clientWidth = 444;
const rail = new El(), ws = rail.appendChild(new El('section', 'rws'));
ws.setAttribute('data-ws', '/repo'); ws.appendChild(new El('span', 'nm')).textContent = 'repo';
const row = ws.appendChild(new El('div', 'rs')); row.setAttribute('data-rid', 'session');
const badge = ws.appendChild(new El('button', 'memo-count')); badge.appendChild(new El('span'));
let activeFrame = null;
const doc = { querySelector: sel => sel === '.sframe.on' ? activeFrame : null, getElementById: id => ids[id], createElement: tag => new El(tag), createTextNode: text => { const e = new El('#text'); e.textContent = text; return e; } };
let remote = [], nextID = 0, calls = [], local = true, pendingRead = null, failWrite = false;
const storage = new Map(), pickers = [];
const capsuleStub = {
  attach(field) { field.wsc = { editor: {} }; },
  topicPicker(field, source) {
    assert.ok(field.wsc, 'capsules attach before picker');
    assert.equal((field.on.keydown || []).length, 0, 'picker attaches before save key handler');
    const picker = { field, source, closed: 0, close() { this.closed++; } };
    pickers.push(picker);
    field.addEventListener('keydown', e => { if (e.pickerEnter) e.preventDefault(); });
    return picker;
  }
};
const window = { WsCapsules: capsuleStub, document: { documentElement: { getAttribute: () => 'standalone' } }, addEventListener() {}, sessionStorage: { setItem: (k,v) => storage.set(k,v), getItem: k => storage.get(k), removeItem: k => storage.delete(k) }, fetch: async (url, opts) => {
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
vm.runInNewContext(fs.readFileSync(path.join(root, 'assets/memo.js'), 'utf8'), { window, Promise, wsT: (k, ...a) => k + '|' + a.join('|'), setTimeout(fn, ms) { const id = ++timerID; timers.push({ id, at: now + ms, fn }); return id; }, clearTimeout(id) { timers = timers.filter(t => t.id !== id); } });
async function flush() { for (let i = 0; i < 15; i++) await Promise.resolve(); }
function advance(ms) { now += ms; for (const t of timers.filter(t => t.at <= now)) t.fn(); timers = timers.filter(t => t.at > now); }
(async () => {
  const panel = new El(), memo = window.WsMemo.mount(doc, rail, panel, () => {});
  memo.sync('session'); memo.open(true, false); await flush();
  assert.equal(focused, ids['memo-input']);
  const source = pickers[0].source;
  const candidates = [{ id: 'active-topic', label: 'Active topic' }];
  let received, delayed;
  activeFrame = { contentWindow: { document: { documentElement: { getAttribute: () => 'session' } }, wsCapsuleTopics(done) { done(candidates); } } };
  source(rows => { received = rows; });
  assert.equal(received, candidates, 'memo reads selected active frame topics');
  memo.sync(''); received = null; source(rows => { received = rows; });
  assert.equal(received.length, 0, 'no selected session has no candidates');
  memo.sync('session');
  activeFrame.contentWindow.wsCapsuleTopics = done => { delayed = done; };
  let delivered = false; source(() => { delivered = true; });
  const closed = pickers[0].closed;
  memo.sync('other-session'); delayed(candidates);
  assert.equal(delivered, false, 'delayed old-session topics are dropped');
  assert.ok(pickers[0].closed > closed, 'session change closes popup');
  received = null; source(rows => { received = rows; });
  assert.equal(received.length, 0, 'active frame with wrong rid is excluded');
  activeFrame = null; window.wsCapsuleTopics = done => done(candidates);
  memo.sync('standalone'); source(rows => { received = rows; });
  assert.equal(received, candidates, 'matching standalone session uses same-window source');
  memo.sync('session');
  ids['memo-input'].value = 'picker draft';
  for (const props of [{ keyCode: 229 }, { defaultPrevented: true }, { pickerEnter: true }]) {
    ids['memo-input'].fire('keydown', { key: 'Enter', ...props }); await flush();
    assert.equal(remote.length, 0, 'IME and picker-owned Enter do not save add draft');
  }
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
  area = ids['memo-list'].querySelector('textarea'); area.value = 'edited';
  assert.equal(pickers.at(-1).field, area, 'edit field has topic picker');
  for (const props of [{ keyCode: 229 }, { isComposing: true }, { defaultPrevented: true }, { pickerEnter: true }]) {
    area.fire('keydown', { key: 'Enter', ...props }); await flush();
    assert.equal(remote[0].text, 'newest', 'IME and picker-owned Enter do not save edit');
  }
  area.fire('keydown', { key: 'Enter' }); await flush();
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
  // The server caps trimmed text at 8000 UTF-8 bytes; a CJK char is 3 bytes but 1 UTF-16 unit.
  const posts = () => calls.filter(c => c[1] && c[1].method === 'POST').length;
  let before = posts();
  ids['memo-input'].value = '  ' + '가'.repeat(2667) + '  '; // 8001 bytes, 2667 units
  ids['memo-input'].fire('input');
  assert.equal(ids['memo-status'].textContent, 'js.memo.toolong|8001|8000', 'input over the byte cap says so');
  ids['memo-input'].fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(posts(), before, 'over-limit add is not POSTed');
  assert.equal(ids['memo-input'].value.includes('가'), true, 'refused draft is kept');
  ids['memo-input'].value = '가'.repeat(2666) + 'ab'; ids['memo-input'].fire('input');
  assert.equal(ids['memo-status'].textContent, '', 'refusal clears once the text fits');
  ids['memo-input'].fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(posts(), before + 1, 'exactly 8000 bytes is POSTed'); assert.equal(remote[0].text.length, 2668);
  ids['memo-list'].querySelector('.memo-text').fire('click');
  area = ids['memo-list'].querySelector('textarea'); area.value = '가'.repeat(2667) + 'a'; before = posts();
  area.fire('input'); assert.equal(ids['memo-status'].textContent, 'js.memo.toolong|8002|8000', 'edit input over the byte cap says so');
  area.fire('keydown', { key: 'Enter' }); await flush();
  assert.equal(posts(), before, 'over-limit edit is not POSTed'); assert.equal(ids['memo-list'].querySelector('textarea').value.length, 2668);
  area.value = '😀'.repeat(2000); area.fire('input'); // 4 bytes, 2 units each: 8000 bytes
  assert.equal(ids['memo-status'].textContent, '', 'a surrogate pair counts as 4 bytes');
  area.value = '😀'.repeat(2000) + 'a'; area.fire('input');
  assert.equal(ids['memo-status'].textContent, 'js.memo.toolong|8001|8000');
  area.fire('keydown', { key: 'Escape' });
  assert.equal(ids['memo-status'].textContent, '', 'cancelling an over-limit edit clears its refusal');
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

  // Masonry: shortest column first in list order, ties left; an edit save keeps
  // every note's column, add and delete re-place.
  const { layout } = window.WsMemo;
  assert.deepEqual({ ...layout(['a', 'b', 'c', 'd'], [100, 50, 30, 30], 2, null) }, { a: 0, b: 1, c: 1, d: 1 });
  assert.deepEqual({ ...layout(['a', 'b', 'c', 'd'], [100, 50, 60, 10], 2, null) }, { a: 0, b: 1, c: 1, d: 0 }, 'each note goes to the currently shorter column');
  assert.deepEqual({ ...layout(['a', 'b'], [40, 40], 2, null) }, { a: 0, b: 1 }, 'ties go left');
  assert.deepEqual({ ...layout(['a', 'b', 'c'], [10, 10, 10], 2, { a: 1, b: 1, c: 1 }) }, { a: 1, b: 1, c: 1 }, 'kept columns hold');
  assert.deepEqual({ ...layout(['a', 'b'], [10, 10], 1, { a: 1, b: 0 }) }, { a: 0, b: 0 }, 'a column that no longer exists is re-placed');
  const columns = () => ids['memo-list'].children.filter(c => c.className === 'memo-col').map(col => col.children.map(row => row.children[0].textContent));
  local = true; remote = [{ id: 'a', text: 'x'.repeat(30) }, { id: 'b', text: 'short' }, { id: 'c', text: 'mid text' }, { id: 'd', text: 'tiny' }];
  memo.open(true, false); await flush();
  assert.deepEqual(columns(), [['x'.repeat(30)], ['short', 'mid text', 'tiny']], 'load places shortest-column-first');
  assert.equal(ids['memo-n'].textContent, '4', 'header shows the memo count');
  ids['memo-list'].querySelectorAll('.memo-text').find(t => t.textContent === 'short').fire('click');
  area = ids['memo-list'].querySelector('textarea'); area.value = 'y'.repeat(60);
  assert.equal(area.parentNode.className, 'memo-note memo-editing');
  area.fire('keydown', { key: 'Enter' }); await flush();
  assert.deepEqual(columns(), [['x'.repeat(30)], ['y'.repeat(60), 'mid text', 'tiny']], 'saving an edit keeps placement');
  ids['memo-input'].value = 'n'; ids['memo-input'].fire('keydown', { key: 'Enter' }); await flush();
  assert.deepEqual(columns(), [['n', 'y'.repeat(60)], ['x'.repeat(30), 'mid text', 'tiny']], 'add re-places');
  ids['memo-list'].querySelector('.memo-delete').fire('click'); await flush();
  assert.deepEqual(columns(), [['x'.repeat(30), 'mid text', 'tiny'], ['y'.repeat(60)]], 'delete re-places');
  ids['memo-undo'].querySelector('button').fire('click'); await flush();
  assert.deepEqual(columns(), [['n', 'y'.repeat(60)], ['x'.repeat(30), 'mid text', 'tiny']], 'undo re-places');
  ids['memo-list'].clientWidth = 300; memo.open(true, false); await flush();
  assert.deepEqual(columns(), [['n', 'x'.repeat(30), 'y'.repeat(60), 'mid text', 'tiny']], 'a narrow pad has one column');
  remote = []; memo.open(true, false); await flush();
  assert.equal(ids['memo-list'].querySelector('.memo-empty').textContent, 'No memos yet.');
  assert.equal(ids['memo-n'].textContent, '0');
})().catch(e => { console.error(e); process.exitCode = 1; });
