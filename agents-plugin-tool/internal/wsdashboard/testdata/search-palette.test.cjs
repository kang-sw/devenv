// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assets = path.join(__dirname, '..', 'assets');
const tracker = fs.readFileSync(path.join(assets, 'tracker.js'), 'utf8');
const dashboard = fs.readFileSync(path.join(assets, 'dashboard.js'), 'utf8');
function block(source, from, to) {
  const start = source.indexOf(from), end = source.indexOf(to, start);
  assert(start >= 0 && end > start, `missing block: ${from}`);
  return source.slice(start, end);
}
const events = [], nodes = {};
function node(id) {
  return nodes[id] ||= { id, hidden: true, value: '', isConnected: true,
    setAttribute(k, v) { this[k] = v; }, focus() { events.push(['focus', id]); },
    querySelector() { return null; }, showModal() { events.push(['modal']); }, close() { events.push(['close']); } };
}
const ctx = vm.createContext({ console, document: { activeElement: node('opener') },
  $: node, O: {}, X: {}, VS: {}, ui: { store: 'store' }, order: ['store'],
  vss: { store: { store: { label: 'Palette tracker' }, itemOrder: ['a', 'b'], items: {
    a: { title: 'Palette tracker task', alias: '1', status: 'open' },
    b: { title: 'Palette hidden', hidden: true }
  } } }, wsT: k => k, esc: String, attr: () => '', SI: { svg: () => '' },
  location: {}, openItemWindow: (...args) => { events.push(['item', ...args]); return false; },
  toast: msg => events.push(['toast', msg]), openDialog: (...args) => events.push(['dialog', ...args]) });
vm.runInContext(block(tracker, '  // One palette:', '  // The immediate custom tooltip:'), ctx);
const plain = v => JSON.parse(JSON.stringify(v));
assert.equal(ctx.qScore('abc', 'ac').score > 0, true);
assert.equal(ctx.qScore('abc', 'ca'), null);
assert.equal(ctx.qScore('ABC', 'abc').score, ctx.qScore('abc', 'abc').score);
assert(ctx.qScore('abc', 'abc').score > ctx.qScore('a---b---c', 'abc').score);
assert(ctx.qScore('abc later', 'abc').score > ctx.qScore('later abc', 'abc').score);
assert.equal(ctx.qScore('a' + 'x'.repeat(100) + 'b', 'ab', true), null);
assert(ctx.qScore('a' + 'x'.repeat(100) + 'b abc', 'abc', true), 'compact later occurrence survives');
assert(ctx.qScore('한글 검색', '한검'), 'Unicode subsequences');
const rows = [5, 4, 3, 2, 1, 0].map(group => ({ group, title: 'Palette', kind: group === 3 ? 'board-item' : 'test', id: '153' }));
assert.deepEqual(plain(ctx.qRank(rows, 'plt').map(x => x.group)), [0, 1, 2, 3, 4, 5]);
const ids = ['154', '15', '153', '1501'].map(id => ({ group: 3, kind: 'board-item', title: 'question', id }));
ids.push({ group: 4, kind: 'board-answer', id: '153', title: 'answer' });
assert.deepEqual(plain(ctx.qRank(ids, '.153').map(x => x.id)), ['153']);
assert.deepEqual(plain(ctx.qRank(ids.filter(x => x.id !== '15'), '.15').map(x => x.id)), ['153', '154', '1501']);
assert.deepEqual(plain(ctx.qRank(ids, '.999')), []);
const recent = [{ group: 3, title: 'old', at: 1 }, { group: 1, title: 'topic', at: 2 }, { group: 3, title: 'new', at: 3 }, { group: 0, title: 'session' }];
assert.deepEqual(plain(ctx.qRank(recent, '').map(x => x.title)), ['topic', 'new', 'old']);
const many = Array.from({length: 30}, (_, i) => ({group: 0, title: 'Palette ' + i})).concat(rows);
assert.deepEqual(plain([...new Set(ctx.qRank(many, 'palette').map(x => x.group))]), [0, 1, 2, 3, 4, 5]);
function key(key, extra = {}) { return { key, target: { closest: () => null }, ...extra }; }
assert.equal(ctx.qShortcut(key('/')), true);
assert.equal(ctx.qShortcut(key('/', {shiftKey: true})), true, 'slash on shifted keyboard layouts');
for (const selector of ['input', 'textarea', 'select', '[contenteditable]']) {
  assert.equal(ctx.qShortcut(key('/', {target: {closest: s => s.includes(selector.split('[')[0]) ? {} : null}})), false);
}
assert.equal(ctx.qShortcut(key('k', {ctrlKey: true})), true);
assert.equal(ctx.qShortcut(key(' ', {ctrlKey: true, code: 'Space'})), true);
assert.equal(ctx.qShortcut(key('k', {ctrlKey: true, isComposing: true})), false);
// Terminal cards (the .xterm helper textarea) and the composer (a textarea)
// keep Ctrl+K / Ctrl+Space; the page body still opens the palette.
function inside(selector) { return { closest: s => s.split(',').map(x => x.trim().split(':')[0]).includes(selector) ? {} : null }; }
for (const target of [inside('.xterm'), inside('textarea'), inside('[contenteditable]')]) {
  assert.equal(ctx.qShortcut(key('k', {ctrlKey: true, target})), false, 'Ctrl+K from an editor');
  assert.equal(ctx.qShortcut(key(' ', {ctrlKey: true, code: 'Space', target})), false, 'Ctrl+Space from an editor');
}
assert.equal(ctx.qShortcut(key('k', {ctrlKey: true, target: inside('body')})), true, 'Ctrl+K from the page body');
assert.equal(ctx.qShortcut(key(' ', {ctrlKey: true, code: 'Space', target: inside('body')})), true, 'Ctrl+Space from the page body');
assert.equal(ctx.qShortcut(key('/', {keyCode: 229})), false);
assert.equal(ctx.qShortcut(key('/', {altKey: true})), false);
ctx.O.searchRows = () => rows;
ctx.O.searchPick = x => events.push(['board', x.id]);
ctx.qOpen();
assert.equal(ctx.qs.open, true);
assert.equal(node('tkqin').value, '');
assert(events.some(x => x[0] === 'modal'));
node('tkqin').value = 'palette'; ctx.qRender();
assert.deepEqual(plain(ctx.qs.items.map(x => x.group)), [0, 1, 2, 3, 4, 5, 5, 5]);
assert(!ctx.qs.items.some(x => x.title === 'Palette hidden'));
ctx.qPick(3);
assert(events.some(x => x[0] === 'board' && x[1] === '153'));
assert(events.some(x => x[0] === 'focus' && x[1] === 'opener'));
ctx.qOpen(); node('tkqin').value = 'tracker task'; ctx.qRender(); ctx.qPick(0);
assert(events.some(x => x[0] === 'item' && x[1] === 'store' && x[2] === 'a'));
assert(events.some(x => x[0] === 'toast'), 'missing tracker item keeps hidden-item toast');
ctx.qOpen(); node('tkqin').value = 'tracker'; ctx.qRender(); ctx.qPick(ctx.qs.items.findIndex(x => x.kind === 'store'));
assert.equal(ctx.location.hash, '#/store');
ctx.X.qsActions = () => [{key: 'support', title: 'Palette support'}];
ctx.X.dialogs = {support: {}};
ctx.qOpen(); node('tkqin').value = 'support'; ctx.qRender(); ctx.qPick(0);
assert(events.some(x => x[0] === 'dialog' && x[1] === 'support'));
let updateRows;
ctx.O.searchRows = update => { updateRows = update; return rows; };
ctx.qOpen(); node('tkqin').value = '.999'; ctx.qRender();
updateRows([{group: 3, kind: 'board-item', id: '999', title: 'Older closed item'}]);
assert.deepEqual(plain(ctx.qs.items.map(x => x.id)), ['999'], 'closed-page arrivals refresh the current query');
ctx.qClose();
// The production keyboard handler must reject IME confirmation before selection.
const inputKey = block(tracker, "    $('tkqin').addEventListener('keydown'", '    // the caret moved without an edit');
assert(inputKey.indexOf('e.isComposing || e.keyCode === 229') < inputKey.indexOf("e.key === 'ArrowDown'"));
assert(tracker.includes("$('tkqpop').addEventListener('cancel'"));
assert(!tracker.includes('class="tk-qpop"'), 'old anchored popover removed');

// Open, unanswered items have no answer chip (readItem returns null).
const textNode = text => ({cloneNode: () => ({textContent: text})});
const unanswered = {id: 'item-42', hasAttribute: () => true, querySelector: () => textNode('Unanswered question')};
const boardTopic = {getAttribute: () => 'example', querySelector: s => s === '.tname' ? textNode('Example') : null};
const rowsCtx = vm.createContext({boardQ: () => [boardTopic], readItem: () => null,
  q: (s, root) => s === '[id^="item-"]' && root === boardTopic ? [unanswered] : []});
vm.runInContext(block(dashboard, '  function paletteText(', '  function paletteClosed('), rowsCtx);
assert.deepEqual(plain(rowsCtx.paletteRows().map(x => x.group)), [1, 3]);

// Jump against a fresh node (the board may swap while the modal is open).
const fold = { tagName: 'DETAILS', open: false };
const topic = { parentElement: {}, querySelector: () => null, getAttribute: () => 'closed' };
const target = { parentElement: fold, tagName: 'DIV', classList: {remove() {}, add() {}},
  matches: () => false, setAttribute() {}, focus() { events.push(['board-focus']); },
  scrollIntoView() { events.push(['scroll', fold.open]); } };
fold.parentElement = topic;
const pickCtx = vm.createContext({ doc: { body: {classList: {contains: () => false}}, documentElement: {} },
  boardQ: () => [topic], $: () => target, live: true, tabShown: () => false,
  setTab: tab => events.push(['tab', tab]), setTimeout: fn => fn(), location: {} });
vm.runInContext(block(dashboard, '  function palettePick(', '\n  function tkMount('), pickCtx);
pickCtx.palettePick({kind: 'board-item', topic: 'closed', id: '153'});
assert(events.some(x => x[0] === 'scroll' && x[1] === true), 'enclosing closed fold opened before scroll');
assert(events.some(x => x[0] === 'tab' && x[1] === 'board'));
pickCtx.palettePick({kind: 'session', href: '/s/other'});
assert.equal(pickCtx.location.href, '/s/other');
let more = {}, updates = 0;
const closedCtx = vm.createContext({ boardQ: () => more ? [more] : [], paletteRows: () => ['older rows'],
  loadClosed: () => ({then(done) { more = null; done(true); }}) });
vm.runInContext(block(dashboard, '  function paletteClosed(', '  function palettePick('), closedCtx);
closedCtx.paletteClosed(rows => { assert.deepEqual(plain(rows), ['older rows']); updates++; });
assert.equal(updates, 1, 'closed pagination updates the palette and stops at the last page');
console.log('search palette: scorer, groups, exact/prefix IDs, recency, shortcuts, IME, focus, and picks passed');
