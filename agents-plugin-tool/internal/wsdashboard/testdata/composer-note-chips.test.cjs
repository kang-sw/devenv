// Offline contract for the production board-input/chip synchronization path.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function func(name) {
  const match = source.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
  assert(match, 'missing production function ' + name);
  return match[0];
}
let rebuilds = 0;
class El {
  constructor(tag = 'span') { this.tagName = tag; this.childNodes = []; this.attrs = {}; this.listeners = {}; }
  appendChild(node) { node.parentNode = this; this.childNodes.push(node); return node; }
  get children() { return this.childNodes.filter(n => n.tagName !== '#text'); }
  get firstChild() { return this.childNodes[0]; }
  set textContent(value) { if (this === strip) rebuilds++; this.childNodes = []; if (value) this.appendChild(text(value)); }
  get textContent() { return this.tagName === '#text' ? this.nodeValue : this.childNodes.map(n => n.textContent).join(''); }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k] ?? null; }
  querySelectorAll(selector) {
    const matches = n => selector[0] === '.' ? (n.className || '').split(' ').includes(selector.slice(1)) : selector.slice(1, -1) in n.attrs;
    return this.children.flatMap(n => (matches(n) ? [n] : []).concat(n.querySelectorAll(selector)));
  }
  querySelector(s) { return this.querySelectorAll(s)[0] || null; }
  addEventListener(k, fn) { this.listeners[k] = fn; }
  focus() { doc.activeElement = this; }
  setSelectionRange(a, b) { this.selectionStart = a; this.selectionEnd = b; }
}
function text(value) { const n = new El('#text'); n.nodeValue = value; return n; }
const strip = new El('div'), note = { value: 'first' }, topic = { value: 'topic first', getAttribute() { return 'topic-1'; } };
let answer = '';
const item = {
  getAttribute() { return '7'; }, hasAttribute() { return false; },
  querySelector(s) { return s === 'textarea.note' ? note : s.startsWith('.opt') && answer ? { getAttribute() { return answer; } } : null; }
};
const doc = { createElement: tag => new El(tag), createTextNode: text, activeElement: null };
const context = vm.createContext({ doc, TextEncoder, wsT: k => k, cimgs: null,
  $: id => id === 'cchips' ? strip : null,
  q: (s, el) => el.querySelectorAll(s), region: () => true,
  boardQ: s => s === '.tnote[data-topic]' ? [topic] : [item],
  queueDraft() {}, effectJump() {}
});
// Use the real state owners, reader, synchronization and complete renderer.
const composer = source.slice(source.indexOf('  // --- composer:'), source.indexOf('  function selectionNote('));
vm.runInContext(composer + func('topicNotes') + '\nfunction selectionNote() {}', context);
context.closeRequests.push({ topic: 'close-me', reason: 'keep focus' });
context.bmentions.push({ topic: 'reference', text: 'retained reference' });
context.syncChips();
const original = strip.children.slice(), itemText = original[0].querySelector('.anote').firstChild, topicText = original[1].querySelector('.anote').firstChild;
const reason = strip.querySelector('[data-close-reason]');
reason.focus(); reason.setSelectionRange(2, 4);
const before = rebuilds;
for (let i = 0; i < 50; i++) {
  note.value += '가'; topic.value += '한';
  context.syncChips();
  assert.deepEqual(strip.children, original, 'all chip nodes survive note input');
}
assert.equal(rebuilds, before, 'unchanged chip set never clears the strip');
assert.equal(original[0].querySelector('.anote').firstChild, itemText, 'update the existing text node');
assert.equal(itemText.nodeValue, note.value);
assert.equal(topicText.nodeValue, topic.value);
assert.equal(doc.activeElement, reason);
assert.deepEqual([reason.selectionStart, reason.selectionEnd], [2, 4]);
context.syncChips(); assert.equal(rebuilds, before, 'input/change with identical values is a no-op');
answer = 'ok'; context.syncChips();
assert.equal(original[0].childNodes[1].nodeValue, ' ok', 'answer changes remain visible');
assert.equal(rebuilds, before);
note.value = ''; topic.value = '  '; answer = ''; context.syncChips();
assert.equal(strip.querySelector('[data-unchip]'), null, 'clearing a note removes its chip');
assert.equal(strip.querySelector('[data-tchip]'), null, 'whitespace-only topic notes have no chip');
assert.equal(context.bmentions.length, 1, 'references remain held');
note.value = 'new'; topic.value = 'new topic'; context.syncChips();
assert.equal(strip.querySelector('[data-unchip]').getAttribute('data-unchip'), '7');
assert.equal(strip.querySelector('[data-tchip]').getAttribute('data-tchip'), 'topic-1');
assert.equal(context.chipList()[0].note, 'new', 'send/draft state stays synchronous');
assert.equal(context.topicNotes()[0].value, 'new topic');
// The wide pin hint must not ask body :has() to scan unrelated mutations.
const css = readAssetSource('wsdashboard', 'dashboard.css');
assert(!css.includes('body:not(.tk-paged):not(:has(#grid'));
assert(css.includes('body:not(.tk-paged):not([data-dashboard-layout="tabs"]) .bpzone'));
context.grid = { setAttribute(k, v) { this[k] = v; } };
context.doc.body = new El('body'); context.live = context.worktree = context.cardBoard = false;
context.q = () => [];
vm.runInContext(func('setLayout'), context);
for (const mode of ['tabs', 'split', 'auto', 'invalid']) {
  context.setLayout(mode);
  assert.equal(context.doc.body.getAttribute('data-dashboard-layout'), mode === 'invalid' ? 'auto' : mode);
  assert.equal(context.grid['data-layout'], context.doc.body.getAttribute('data-dashboard-layout'));
}
console.log('composer note chips: in-place text, stable references/focus, add/remove, synchronous send state and layout mirror passed');
