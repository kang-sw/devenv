const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const root = path.join(__dirname, '../assets');
const dash = readAssetSource('wsdashboard', 'dashboard.js');
const cards = fs.readFileSync(path.join(root, 'cards.js'), 'utf8');
function fn(src, name) {
  const m = src.match(new RegExp('\\n  function ' + name + '\\([^]*?\\n  \\}\\n'));
  assert.ok(m, name); return m[0];
}
class El {
  constructor(tag = 'div', cls = '') { this.tagName = tag.toUpperCase(); this.nodeType = 1; this.className = cls; this.children = []; this.attrs = {}; this.value = ''; this.listeners = {}; }
  get classList() { return { contains: c => this.className.split(' ').includes(c) }; }
  set textContent(v) { this.text = v; this.children = []; }
  get textContent() { return this.text || ''; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k] ?? null; }
  removeAttribute(k) { delete this.attrs[k]; }
  hasAttribute(k) { return this.getAttribute(k) !== null; }
  querySelector(s) { return this.querySelectorAll(s)[0] || null; }
  appendChild(n) { n.remove(); n.parentNode = this; this.children.push(n); return n; }
  insertBefore(n, ref) { if (!ref) return this.appendChild(n); n.remove(); n.parentNode = this; this.children.splice(this.children.indexOf(ref), 0, n); return n; }
  remove() { if (this.parentNode) this.parentNode.children.splice(this.parentNode.children.indexOf(this), 1); this.parentNode = null; }
  all() { return this.children.flatMap(n => [n, ...n.all()]); }
  matches(s) { return s.startsWith('.') ? this.classList.contains(s.slice(1)) : s === 'section.topic[data-topic]' ? this.tagName === 'SECTION' && this.classList.contains('topic') : s.startsWith('[') && this.getAttribute(s.slice(1, -1)) !== null; }
  closest(s) { for (let n = this; n; n = n.parentNode) if (n.matches(s)) return n; return null; }
  querySelectorAll(s) { return this.all().filter(n => n.matches(s)); }
  addEventListener(k, f) { this.listeners[k] = f; }
  focus() { doc.activeElement = this; }
  setSelectionRange(a, b) { this.selectionStart = a; this.selectionEnd = b; }
}
const ctext = new El('textarea'), cchips = new El(), board = new El(), pins = new El();
const doc = { activeElement: null, createElement: t => new El(t), createTextNode: s => { const n = new El(); n.textContent = s; return n; }, getElementById: id => id === 'ctext' ? ctext : null, addEventListener: (_, f) => { context.click = f; } };
const context = vm.createContext({ console, doc, document: doc, ctext, cchips, board, pins, colOn: true, topics: {}, maxed: null, window: {}, closeRequests: [], bmentions: [], wpicks: [], wconsoles: [], sending: false, cmdSel: null, imgs: [], boardOwn: {}, posted: [], warns: [], draftSaves: 0 });
const noop = () => {};
Object.assign(context, {
  wsT: (k, topic) => topic === undefined ? k : `${k}:${topic}`,
  q: (s, n) => n.querySelectorAll(s), child: (n, cls) => n.children.find(c => c.classList.contains(cls)) || null,
  countTopics: noop, chipList: () => [], topicNotes: () => [], queueDraft: () => context.draftSaves++,
  sendText: () => ctext.value, followConvo: noop, boardFlushViewed: noop, clearTrash: noop, clearMentions: noop,
  grow: noop, renderImgs: noop, closeTopicNotes: noop, clearDraft: noop,
  noteWarn: s => context.warns.push(s), post: (route, body, done) => { context.posted.push({ route, body }); context.done = done; },
  inBoard: n => { for (; n; n = n.parentNode) if (n === board || n === pins) return true; return false; },
  syncChips: () => context.renderChips(),
  allNums: () => ({}), markerNums: () => ({}), shiftMentions: noop, restoreTopics: () => [], validMentions: () => [], validPicks: () => [], validConsoles: () => [], imgSeq: 0,
  holdSync: false, syncMarkers: noop, MARKER: '\\[Image #([1-9][0-9]{0,5})\\]'
});
vm.runInContext(['mk', 'licon', 'ibtn', 'decorate'].map(n => fn(cards, n)).join('\n') + ['validCloseRequests', 'focusCloseRequest', 'addCloseRequest', 'removeCloseRequest', 'renderChips', 'sendMessage', 'restoreHeld', 'restoreHeldNow'].map(n => fn(dash, n)).join('\n'), context);
// Only the production delegated click block: the rest owns unrelated menus.
vm.runInContext(dash.slice(dash.indexOf('  doc.addEventListener("click", function (e) {', dash.indexOf('// --- clicks:')), dash.indexOf('  // --- link actions:')), context);
function topic(key, closed, host = board) {
  const sec = new El('section', 'topic' + (closed ? ' closed' : '')); sec.setAttribute('data-topic', key);
  const head = new El('div', 'tophead'), bar = new El('div', 'tbar'); head.appendChild(bar); sec.appendChild(head); host.appendChild(sec);
  bar.appendChild(new El('span', 'topic-state-badge')); bar.appendChild(new El('button', 'tnote-toggle'));
  return { sec, bar };
}
const a = topic('a"<&>', false), b = topic('b', false, pins), closed = topic('closed', true);
context.decorate(); context.decorate();
const close = row => row.bar.children.find(n => n.classList.contains('t-close-request'));
assert.equal(a.bar.children.at(-1), close(a)); assert.equal(b.bar.children.at(-1), close(b)); assert.equal(close(closed), undefined);
assert.equal(a.bar.children.filter(n => n.classList.contains('t-close-request')).length, 1);
assert.ok(a.bar.children.some(n => n.classList.contains('topic-state-badge')));
const lookup = doc.getElementById; doc.getElementById = () => null;
context.decorate(); assert.equal(close(a), undefined, 'no dead action on a page without a composer');
doc.getElementById = lookup; context.decorate();
context.topics['a"<&>'] = { folded: true, el: a.sec };
context.decorate(); assert.equal(a.sec.getAttribute('data-size'), 'fold'); assert.equal(a.bar.children.at(-1), close(a));
assert.equal(close(a).getAttribute('aria-label'), 'js.closerequest.add');
const press = n => context.click({ target: n });
a.sec.setAttribute('data-max', '');
let restored = 0;
a.bar.children.find(n => n.classList.contains('d-min')).click = () => { restored++; a.sec.removeAttribute('data-max'); };
press(close(a)); const reason = doc.activeElement;
assert.equal(restored, 1, 'fullscreen is restored through its existing back action before focusing the composer');
assert.equal(reason.getAttribute('data-close-reason'), 'a"<&>'); assert.equal(context.posted.length, 0, 'x is not a board mutation');
reason.value = 'keep & <reason>'; reason.listeners.input(); reason.setSelectionRange(2, 5);
context.renderChips(); assert.deepEqual([doc.activeElement.selectionStart, doc.activeElement.selectionEnd], [2, 5], 'reconciliation retains caret');
press(close(a)); assert.equal(context.closeRequests.length, 1); assert.equal(doc.activeElement.value, 'keep & <reason>');
press(close(b)); assert.equal(context.closeRequests.length, 2);
press(cchips.querySelectorAll('[data-unclose-request]')[0]); assert.equal(context.closeRequests.length, 1);
a.sec.className += ' closed'; context.decorate(); assert.equal(close(a), undefined);
// A disappearing/closed topic does not discard its already attached request.
b.sec.remove(); context.sendMessage(); assert.equal(context.posted.at(-1).body.closeRequests[0].topic, 'b'); assert.equal(context.posted.at(-1).body.text, '');
context.done(false, 'offline'); assert.equal(context.closeRequests.length, 1, 'failed send retains requests');
context.sendMessage(); context.addCloseRequest('new');
const pending = context.closeRequests[0]; pending.reason = 'changed in flight';
context.done(true, {}); assert.deepEqual(Array.from(context.closeRequests, r => r.topic), ['b', 'new']);
context.sendMessage(); context.removeCloseRequest('b'); context.addCloseRequest('b'); context.done(true, {});
assert.deepEqual(Array.from(context.closeRequests, r => r.topic), ['b'], 'remove/re-add is a new intent, not a sent reference');
context.sendMessage(); context.done(true, {}); assert.equal(context.closeRequests.length, 0);
// Cancel/interrupt restore dedupes held topics, newest held reason wins, draft wins over held.
context.addCloseRequest('draft'); context.closeRequests[0].reason = 'mine';
context.restoreHeld([{ sentAt: 2, closeRequests: [{ topic: 'held', reason: 'newest' }, { topic: 'draft', reason: 'old' }] }, { sentAt: 1, closeRequests: [{ topic: 'held', reason: 'older' }] }]);
assert.deepEqual(JSON.parse(JSON.stringify(context.closeRequests)), [{ topic: 'held', reason: 'newest' }, { topic: 'draft', reason: 'mine' }]);
assert.deepEqual(JSON.parse(JSON.stringify(context.validCloseRequests([null, { topic: 'bad\n', reason: '' }, { topic: 'x' }]))), []);
const css = readAssetSource('wsdashboard', 'dashboard.css');
assert.match(css, /:root\[data-overview\] \.r-board :is\(\.t-close-request,/);
console.log('close requests: header order, badges, pinned/closed, focus/caret, dedupe/removal, stale sends, failure/races and held restore passed');
