const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const root = path.join(__dirname, '../assets');

// A deterministic DOM/Range fixture, not a browser or layout acceptance test.
class Evt {
  constructor(type, opts = {}) { Object.assign(this, opts); this.type = type; this.defaultPrevented = !!opts.defaultPrevented; }
  preventDefault() { this.defaultPrevented = true; }
  stopPropagation() {}
  stopImmediatePropagation() { this.stopped = true; }
}
class El {
  constructor(name = 'div', type = 1) { this.nodeName = name.toUpperCase(); this.tagName = this.nodeName; this.nodeType = type; this.childNodes = []; this.attrs = {}; this.listeners = {}; this.style = {}; this.className = ''; this.scrollHeight = 38; this.offsetWidth = 100; this.offsetHeight = 20; }
  get value() { return this._value || ''; } set value(v) { this._value = v; }
  get classList() { return { add: c => { this.className += ' ' + c; }, contains: c => this.className.split(/\s+/).includes(c), toggle() {} }; }
  get children() { return this.childNodes.filter(n => n.nodeType === 1); }
  get firstChild() { return this.childNodes[0]; }
  get previousSibling() { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) - 1]; }
  get nextSibling() { return this.parentNode?.childNodes[this.parentNode.childNodes.indexOf(this) + 1]; }
  get nextElementSibling() { let n = this.nextSibling; while (n && n.nodeType !== 1) n = n.nextSibling; return n; }
  appendChild(n) { this.childNodes.push(n); n.parentNode = this; return n; }
  insertAdjacentElement(_, n) { const p = this.parentNode; p.childNodes.splice(p.childNodes.indexOf(this) + 1, 0, n); n.parentNode = p; }
  remove() { this.parentNode.childNodes.splice(this.parentNode.childNodes.indexOf(this), 1); this.parentNode = null; }
  removeChild(n) { n.remove(); }
  set textContent(v) { this.childNodes = []; if (this.nodeType === 3) this.nodeValue = v; else if (v) this.appendChild(textNode(v)); }
  get textContent() { return this.nodeType === 3 ? this.nodeValue : this.childNodes.map(n => n.textContent).join(''); }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k] ?? null; }
  hasAttribute(k) { return k in this.attrs; }
  removeAttribute(k) { delete this.attrs[k]; }
  contains(n) { return this === n || this.childNodes.some(c => c.contains(n)); }
  matches(s) { return s.split('.').filter(Boolean).every(c => this.classList.contains(c)); }
  closest(s) { return this.matches(s) ? this : this.parentNode?.closest(s); }
  querySelector(s) { return this.querySelectorAll(s)[0] || null; }
  querySelectorAll(s) { const out = []; for (const c of this.children) { if (c.matches(s)) out.push(c); out.push(...c.querySelectorAll(s)); } return out; }
  addEventListener(k, f) { (this.listeners[k] ||= []).push(f); }
  dispatchEvent(e) { e.target = this; for (const f of this.listeners[e.type] || []) { f(e); if (e.stopped) break; } return !e.defaultPrevented; }
  focus() { doc.activeElement = this; } blur() { doc.activeElement = null; }
  getBoundingClientRect() { return { left: 10, top: 500, bottom: 520, height: 20 }; }
  get outerHTML() { return '<span>' + this.textContent + '</span>'; }
  cloneNode(deep) { const n = new El(this.nodeName, this.nodeType); n.nodeValue = this.nodeValue; n.attrs = { ...this.attrs }; n.className = this.className; n._value = this._value; if (deep) this.childNodes.forEach(c => n.appendChild(c.cloneNode(true))); return n; }
}
function textNode(s) { const n = new El('#text', 3); n.nodeValue = s; return n; }
function prefix(root, end, offset) {
  let stopped = false;
  function copy(n) {
    const c = n.cloneNode(false);
    if (n === end) {
      if (n.nodeType === 3) c.nodeValue = n.nodeValue.slice(0, offset);
      else n.childNodes.slice(0, offset).forEach(k => c.appendChild(k.cloneNode(true)));
      stopped = true; return c;
    }
    for (const k of n.childNodes) { if (stopped) break; c.appendChild(copy(k)); }
    return c;
  }
  return copy(root);
}
let currentRange = null;
const selection = { get rangeCount() { return currentRange ? 1 : 0; }, get anchorNode() { return currentRange?.startContainer; }, get focusNode() { return currentRange?.endContainer; }, getRangeAt() { return currentRange; }, removeAllRanges() { currentRange = null; }, addRange(r) { currentRange = r; } };
const doc = { body: new El('body'), activeElement: null, createElement: n => new El(n), createTextNode: textNode,
  execCommand() { return false; }, createRange() { return { selectNodeContents(n) { this.root = n; }, setStart(n, a) { this.startContainer = n; this.startOffset = a; }, setEnd(n, a) { this.endContainer = n; this.endOffset = a; }, cloneContents() { return prefix(this.root, this.endContainer, this.endOffset); }, collapse() {}, getBoundingClientRect() { return { left: 10, top: 500, bottom: 520, height: 20 }; } }; } };
let clock = 0, timers = [], tid = 0;
function advance(ms) { clock += ms; for (;;) { const due = timers.filter(t => t.at <= clock); if (!due.length) return; timers = timers.filter(t => t.at > clock); due.forEach(t => t.f()); } }
const context = vm.createContext({ console, document: doc, Event: Evt, KeyboardEvent: Evt, MutationObserver: class { observe() {} }, getComputedStyle: () => ({}), innerWidth: 1000, innerHeight: 800, wsT: k => k, setTimeout(f, ms) { const id = ++tid; timers.push({ id, at: clock + ms, f }); return id; }, clearTimeout(id) { timers = timers.filter(t => t.id !== id); } });
context.window = context; context.getSelection = () => selection; context.addEventListener = () => {}; context.removeEventListener = () => {};
vm.runInContext(fs.readFileSync(path.join(root, 'mention.js'), 'utf8'), context);
vm.runInContext(fs.readFileSync(path.join(root, 'capsules.js'), 'utf8'), context);
const C = context.WsCapsules;
const plain = v => JSON.parse(JSON.stringify(v));
function make(value = '') { const field = doc.body.appendChild(new El('textarea')); field.value = value; C.attach(field); return field; }
function key(field, key, opts = {}) { const e = new Evt('keydown', { key, ...opts }); field.wsc.editor.dispatchEvent(e); return e; }
function fn(name) { const src = readAssetSource('wsdashboard', 'dashboard.js'); const start = src.indexOf('  function ' + name + '('); assert.notEqual(start, -1, name); return src.slice(start, src.indexOf('\n  }', start) + 4); }

assert.deepEqual(plain(C.token('Please /lrv', 11, '/')), { start: 7, end: 11, q: 'lrv' });
assert.equal(C.token('Please /lrv later', 11, '/'), null, 'non-whitespace suffix refuses a picker');
assert.deepEqual(plain(C.token('Please /lrv  \n', 11, '/')), { start: 7, end: 11, q: 'lrv' });
assert.equal(C.token('path/lrv', 8, '/'), null, 'a slash must start its token');
assert.deepEqual(plain(C.token('See #cux', 8, '#')), { start: 4, end: 8, q: 'cux' });
const names = ['lead-review', 'lrv', 'lrv-extra', 'xlrv', 'later-review'];
assert.deepEqual(plain(C.rank(names, 'lrv', r => r)), ['lrv', 'lrv-extra', 'xlrv', 'lead-review', 'later-review']);
assert.deepEqual(plain(C.rank(['Composer UX', 'Command menu experience', 'Unrelated'], 'cux', r => r)), ['Composer UX', 'Command menu experience']);

const f = make('Please /lrv'); f.focus();
f.wsc.insert('skill', 'lead-review', 'lead-review', { start: 7, end: 11 });
assert.equal(f.value, 'Please lead-review ');
assert.equal(f.wsc.editor.textContent, 'Please /lead-review ');
assert.equal(C.command(f), null, 'mid-text skill identity never invokes');
assert.deepEqual(plain(f.wsc.snapshot()), [{ start: 7, end: 18, kind: 'skill', id: 'lead-review', label: 'lead-review' }]);
assert.equal(f._value, f.value, 'native backing value is current for board clone snapshots');
f.setSelectionRange(18, 18); assert.equal(f.selectionStart, 18);
assert.equal(key(f, 'Backspace').defaultPrevented, true);
assert.equal(f.value, 'Please  ', 'one Backspace removes the whole capsule');
assert.equal(f.wsc.snapshot().length, 0);

f.value = '/lrv'; f.wsc.insert('skill', 'lead-review', 'lead-review', { start: 0, end: 4 });
assert.deepEqual(plain(C.command(f)), { name: 'lead-review', end: 11 });
f.value = ' ' + f.value;
assert.equal(C.command(f), null, 'even leading whitespace makes a capsule ordinary text');
f.setSelectionRange(1, 1); key(f, 'Delete'); assert.equal(f.value, '  ', 'Delete removes an adjacent capsule');
f.value = 'See #cux'; f.wsc.insert('topic', 'composer-ux', 'Composer UX', { start: 4, end: 8 });
assert.equal(f.value, 'See [composer-ux] '); assert.equal(f.wsc.editor.textContent, 'See #Composer UX ');
const saved = plain(f.wsc.snapshot()), draft = f.value;
f.value = ''; f.value = draft; f.wsc.restore(saved);
assert.deepEqual(plain(f.wsc.snapshot()), saved, 'local draft identity and labels restore');
f.setSelectionRange(6, 8); f.setRangeText('x', 6, 8, 'end');
assert.equal(f.value, 'See x ', 'programmatic partial selection replaces the complete capsule');
f.value = draft; f.wsc.restore(saved); f.setSelectionRange(17, 17);
const before = f.value; key(f, 'Backspace', { isComposing: true }); key(f, 'Backspace', { keyCode: 229 });
assert.equal(f.value, before, 'IME confirmation cannot delete a capsule');
// The fallback path also preserves plain insertion and newlines when execCommand is unavailable.
f.value = 'first'; f.setSelectionRange(5, 5); key(f, 'Enter', { shiftKey: true }); assert.equal(f.value, 'first\n');
f.wsc.editor.dispatchEvent(new Evt('paste', { clipboardData: { types: ['text/plain'], getData: () => '<b>plain</b>' } }));
assert.equal(f.value, 'first\n<b>plain</b>', 'paste remains plain text, never markup or capsule identity');
let forwarded = 0; f.addEventListener('keydown', () => forwarded++);
key(f, 'k', { ctrlKey: true, defaultPrevented: true }); assert.equal(forwarded, 0, 'an upstream keyboard owner is not invoked twice');
let composition = [];
f.addEventListener('input', e => composition.push(e.isComposing));
f.wsc.editor.dispatchEvent(new Evt('compositionstart'));
f.wsc.editor.dispatchEvent(new Evt('input'));
f.wsc.editor.dispatchEvent(new Evt('compositionend'));
assert.equal(composition.at(-1), true, 'IME input reaches owners without opening pickers');

// Browser-produced line blocks and filler breaks are text, not phantom lines.
f.wsc.editor.textContent = '';
const a = f.wsc.editor.appendChild(new El('div')); a.appendChild(textNode('first'));
const b = f.wsc.editor.appendChild(new El('div')); b.appendChild(new El('br'));
f.wsc.editor.dispatchEvent(new Evt('input')); assert.equal(f.value, 'first\n');
f.wsc.editor.textContent = ''; f.wsc.editor.appendChild(new El('br'));
f.wsc.editor.dispatchEvent(new Evt('input')); assert.equal(f.value, '');
function lines(values, tag = 'div') {
  f.wsc.editor.textContent = '';
  for (const value of values) {
    const line = f.wsc.editor.appendChild(new El(tag));
    line.appendChild(value ? textNode(value) : new El('br'));
  }
  f.wsc.editor.dispatchEvent(new Evt('input'));
}
lines(['first', '', 'last']); assert.equal(f.value, 'first\n\nlast', 'an interior blank block is never collapsed');
f.setSelectionRange(7, 7); assert.equal(f.selectionStart, 7, 'caret offsets include the blank block boundary');
f.wsc.editor.dispatchEvent(new Evt('paste', { clipboardData: { getData: () => 'X' } }));
assert.equal(f.value, 'first\n\nXlast', 'editing after an interior blank line preserves its position');
lines(['first', '', '', 'last']); assert.equal(f.value, 'first\n\n\nlast', 'consecutive blank blocks each retain a line boundary');
lines(['', '', 'last'], 'p'); assert.equal(f.value, '\n\nlast', 'leading blank paragraph lines are preserved');
lines(['first', 'last']); f.wsc.editor.firstChild.appendChild(new El('br'));
f.wsc.editor.dispatchEvent(new Evt('input')); assert.equal(f.value, 'first\nlast', 'a trailing caret filler does not add a phantom blank line');
f.wsc.editor.textContent = 'first'; f.wsc.editor.appendChild(new El('br')); f.wsc.editor.appendChild(new El('br'));
f.wsc.editor.dispatchEvent(new Evt('input')); assert.equal(f.value, 'first\n');
f.setSelectionRange(6, 6); assert.equal(f.selectionStart, 6, 'caret offsets use the original DOM, not a truncated filler-break clone');

// Topics and image-marker removal retain surrounding references and identities.
const imgText = '[Image #1] [a] [Image #2] [b]';
const rs = [{ start: 11, end: 14, kind: 'topic', id: 'a', label: 'A' }, { start: 26, end: 29, kind: 'topic', id: 'b', label: 'B' }];
// Derive the last range from its actual source offset to keep this fixture readable.
rs[1].start = imgText.indexOf('[b]'); rs[1].end = rs[1].start + 3;
const stripped = plain(C.stripMarkers(imgText, rs, [1, 2]));
assert.equal(stripped.text, '[a] [b]'); assert.deepEqual(stripped.ranges.map(r => [r.id, r.start, r.end]), [['a', 0, 3], ['b', 4, 7]]);

// Exercise the actual reusable listbox and topic-only Enter ownership.
const note = make('#cux'); note.focus(); let candidates = [{ id: 'composer-ux', label: 'Composer UX' }];
const popup = C.topicPicker(note, done => done(candidates)); let sends = 0;
note.addEventListener('keydown', e => { if (e.key === 'Enter') sends++; });
note.setSelectionRange(4, 4); popup.sync(); advance(100);
assert.equal(key(note, 'Enter').defaultPrevented, true);
assert.equal(note.value, '[composer-ux] '); assert.equal(sends, 0, 'picker Enter inserts and never submits');
note.value = '/lrv'; note.setSelectionRange(4, 4); popup.sync(); advance(100);
assert.equal(popup.key(new Evt('keydown', { key: 'Enter' })), false, 'notes have no slash picker');
note.isConnected = false; const beforeSweep = doc.body.querySelectorAll('.wsm-pop').length; C.sweep();
assert.equal(doc.body.querySelectorAll('.wsm-pop').length, beforeSweep - 1, 'removed notes release their body popup and resize listener');

// The actual command source uses the same caret contract and fuzzy ranking.
context.cmdSel = null; context.commandList = () => [{ name: 'lead-review' }, { name: 'compact' }];
vm.runInContext(fn('slashToken') + '\n' + fn('commandRows'), context);
assert.deepEqual(plain(context.slashToken('Please /lrv', 11)), { start: 7, end: 11, q: 'lrv' });
assert.equal(context.slashToken('Please /lrv next', 11), null);
assert.deepEqual(plain(context.commandRows(context.commandList(), 'lrv')).map(r => r.name), ['lead-review']);
context.commandList = () => []; assert.equal(context.slashToken('/lrv', 4), null);

// The real composer submission functions keep structured commands separate.
Object.assign(context, { ctext: f, cmdSel: null, sending: false, imgs: [], posted: [], warns: [], chips: {}, bmentions: [], wpicks: [], wconsoles: [] });
context.sendText = () => f.value; context.followConvo = () => {}; context.noteWarn = s => context.warns.push(s); context.clearDraft = () => {}; context.grow = () => {}; context.clearMentions = () => {};
context.post = (route, body, done) => { context.posted.push(plain(body)); done(true, {}); };
context.chipList = () => []; context.topicNotes = () => []; context.boardOwn = {}; context.boardFlushViewed = () => {}; context.clearTrash = () => {}; context.closeTopicNotes = () => {}; context.renderChips = () => {}; context.renderImgs = () => {};
vm.runInContext(fn('sendInlineCommand') + '\n' + fn('sendMessage'), context);
f.value = '/lrv'; f.wsc.insert('skill', 'lead-review', 'lead-review', { start: 0, end: 4 }); f.setRangeText('check [composer-ux]', f.value.length, f.value.length);
context.sendMessage(); assert.deepEqual(context.posted, [{ command: 'lead-review', args: 'check [composer-ux]' }]);
f.value = 'Please /lrv'; f.wsc.insert('skill', 'lead-review', 'lead-review', { start: 7, end: 11 });
context.sendMessage(); assert.equal(context.posted.at(-1).text, 'Please lead-review'); assert.equal('command' in context.posted.at(-1), false);
f.value = '/lrv'; f.wsc.insert('skill', 'lead-review', 'lead-review', { start: 0, end: 4 }); context.imgs = [{ n: 1 }]; const count = context.posted.length;
context.sendMessage(); assert.equal(context.posted.length, count); assert.equal(context.warns.at(-1), 'js.cmd.images');
context.imgs = []; const answers = [{ id: '12', answer: 'ok' }], topicNotes = [{ topic: 'other', value: 'note' }];
context.chipList = () => answers; context.topicNotes = () => topicNotes; context.bmentions = [{ topic: 'other', text: 'selection' }]; context.wpicks = [{ selector: '#x' }];
context.sendMessage(); assert.deepEqual(context.posted.at(-1), { command: 'lead-review', args: '' });
assert.equal(answers.length, 1); assert.equal(topicNotes.length, 1); assert.equal(context.bmentions.length, 1); assert.equal(context.wpicks.length, 1, 'command-only requests leave other components held for a later message');

// Candidate loading follows every unopened closed page of this selected session.
vm.runInContext(fn('capsuleTopics'), context);
(async () => {
  context.rid = 'selected'; context.shell = false; context.capsuleTopicLoad = null;
  context.region = () => ({ getAttribute: () => 'revision' });
  const topic = (id, label) => ({ getAttribute: () => id, querySelector: () => ({ label }) });
  context.paletteText = n => n.label; let from = 10, urls = [];
  context.boardQ = s => s.startsWith('button') ? [{ getAttribute: () => String(from) }] : [topic('open', 'Open label')];
  context.frag = html => ({ topics: [topic('closed-' + html, 'Closed label ' + html)], querySelector: () => html === '10' ? { getAttribute: () => '20' } : null });
  context.q = (_, n) => n.topics;
  context.fetch = async url => { urls.push(url); return { ok: true, json: async () => ({ html: url.split('=').at(-1) }) }; };
  const rows = await new Promise(resolve => context.capsuleTopics(resolve));
  assert.deepEqual(plain(rows), [{ id: 'open', label: 'Open label' }, { id: 'closed-10', label: 'Closed label 10' }, { id: 'closed-20', label: 'Closed label 20' }]);
  assert.deepEqual(urls, ['/s/selected/closed?from=10', '/s/selected/closed?from=20']);
  context.rid = ''; assert.deepEqual(plain(await new Promise(resolve => context.capsuleTopics(resolve))), []);
  assert.equal(urls.length, 2, 'no session issues no topic requests');
  console.log('capsules: token, fuzzy, atomic DOM/caret, IME, drafts, notes, command wire, attachments and closed pagination passed');
})().catch(e => { console.error(e); process.exitCode = 1; });
