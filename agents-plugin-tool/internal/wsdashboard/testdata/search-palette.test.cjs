// Run locally: node agents-plugin-tool/internal/wsdashboard/testdata/search-palette.test.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const tracker = readAssetSource('wsdashboard', 'tracker.js');
const dashboard = readAssetSource('wsdashboard', 'dashboard.js');
function block(source, from, to) {
  const start = source.indexOf(from), end = source.indexOf(to, start);
  assert(start >= 0 && end > start, `missing block: ${from}`);
  return source.slice(start, end);
}
const plain = v => JSON.parse(JSON.stringify(v));
// wsT stub: the key, with its arguments shown so a test can tell them apart
const wsT = (k, ...a) => a.length ? k + '(' + a.join(',') + ')' : k;

// --- the renderer's palette block, over a DOM stub ---------------------------
const events = [], nodes = {};
let width = 1240;
function node(id) {
  return nodes[id] ||= { id, hidden: true, value: '', innerHTML: '', textContent: '', isConnected: true, attrs: {},
    setAttribute(k, v) { this.attrs[k] = String(v); }, getAttribute(k) { return this.attrs[k]; },
    focus() { events.push(['focus', id]); }, select() { events.push(['select', id]); },
    querySelectorAll() { return []; }, querySelector() { return null; },
    getBoundingClientRect() { return { width }; },
    showModal() { events.push(['modal']); }, close() { events.push(['close']); } };
}
let timers = [];
function flush() { const t = timers; timers = []; t.forEach(f => f()); }
const ctx = vm.createContext({ console, Date, AbortController, document: { activeElement: node('opener') },
  $: node, O: {}, X: {}, VS: {}, H: null, ui: { store: 'store' }, order: ['store', 'cold'],
  vss: { store: { store: { label: 'Palette tracker' }, itemOrder: ['a', 'b', 'c'], items: {
    a: { title: 'Palette tracker task', alias: '1', status: 'open' },
    b: { title: 'Palette hidden', hidden: true, status: 'open' },
    c: { title: 'Running work', alias: '3', status: 'doing' }
  } } }, wsT, esc: s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;'),
  attr: (k, v) => ` data-${k}="${v}"`, SI: { svg: () => '' },
  setTimeout: f => { timers.push(f); return timers.length; }, clearTimeout: () => { timers = []; },
  location: {}, window: { open: (...a) => events.push(['window', ...a]) },
  openItemWindow: (...args) => { events.push(['item', ...plain(args)]); return args[1] !== 'b'; }, closeItemWindow: () => {},
  toast: msg => events.push(['toast', msg]), openDialog: (...args) => events.push(['dialog', ...args]),
  AD: { view: n => ctx.vss[n] || null } });
vm.runInContext(block(tracker, '  // One palette:', '  // The immediate custom tooltip:'), ctx);
const qs = ctx.qs, S = () => qs.S;
function type(v) { node('tkqin').value = v; ctx.qRun(); }
function press(key, extra = {}) { const e = { key, code: '', preventDefault() { e.prevented = true; }, stopPropagation() {}, ...extra }; ctx.qKey(e); return e; }
function selKey(v) { return ctx.qView(v)[ctx.qIdx(v)].key; }

// The fuzzy scorer.
assert.equal(ctx.qScore('abc', 'ac').score > 0, true);
assert.equal(ctx.qScore('abc', 'ca'), null);
assert.equal(ctx.qScore('ABC', 'abc').score, ctx.qScore('abc', 'abc').score);
assert(ctx.qScore('abc', 'abc').score > ctx.qScore('a---b---c', 'abc').score);
assert(ctx.qScore('abc later', 'abc').score > ctx.qScore('later abc', 'abc').score);
assert.equal(ctx.qScore('a' + 'x'.repeat(100) + 'b', 'ab', true), null);
assert(ctx.qScore('a' + 'x'.repeat(100) + 'b abc', 'abc', true), 'compact later occurrence survives');
assert(ctx.qScore('한글 검색', '한검'), 'Unicode subsequences');

// Layout steps follow the palette's width.
assert.deepEqual([599, 600, 999, 1000].map(ctx.qStepOf), ['narrow', 'medium', 'medium', 'wide']);

// The board slot: (n) is exact, a bare number fuzzy-matches ids with prefixes first, .n is text.
const ids = ['154', '15', '153', '1501', '2153'].map(id => ({ group: 3, kind: 'board-item', title: 'question ' + id, id }));
ids.push({ group: 4, kind: 'board-answer', id: '153', title: 'answer' });
assert.deepEqual(plain(ctx.qBoard(ids, '(153)').map(x => x.id)), ['153']);
assert.deepEqual(plain(ctx.qBoard(ids, '(999)')), []);
assert.deepEqual(plain(ctx.qBoard(ids, '153').map(x => x.id)), ['153', '2153'], 'bare number: prefix first, items only');
assert.deepEqual(plain(ctx.qBoard(ids, '15').map(x => x.id)).slice(0, 4).sort(), ['15', '1501', '153', '154']);
assert.deepEqual(plain(ctx.qBoard(ids, '.999')), [], '.n is no longer an id lookup');
const recent = [{ group: 3, kind: 'board-item', id: '1', title: 'old', at: 1 }, { group: 1, kind: 'board-topic', topic: 't', title: 'topic', at: 2 },
  { group: 3, kind: 'board-item', id: '2', title: 'new', at: 3 }, { group: 0, kind: 'session', title: 'session' }, { group: 2, kind: 'board-body', title: 'body' }];
assert.deepEqual(plain(ctx.qBoard(recent, '').map(x => x.title)), ['topic', 'new', 'old'], 'empty: topics, then items newest first');
assert(!ctx.qBoard(recent, 'session').length, 'sessions belong to the strip');
const entryRows = [1, 2, 3, 4, 5].reverse().map(group => ({ group, kind: group === 5 ? 'board-entry' : 'other', topic: 't', entry: group === 5 ? 'entry-a' : undefined, title: 'needle' }));
assert.deepEqual(plain(ctx.qBoard(entryRows, 'needle').map(r => r.group)), [1, 2, 3, 4, 5], 'entries sort last on equal score');
assert(!ctx.qBoard(entryRows, '').some(r => r.group === 5), 'entries are not empty-query recents');
assert.equal(ctx.qBoard([{ group: 5, title: 'a' + 'x'.repeat(100) + 'b' }], 'ab').length, 0, 'entries use compact prose matching');
assert.equal(ctx.qBoard([{ group: 5, title: 'prefix '.repeat(30) + 'needle' }], 'needle').length, 1, 'late entry text is searchable');
assert.notEqual(ctx.qKeyOf('board', { kind: 'board-entry', topic: 't', entry: 'entry-a' }), ctx.qKeyOf('board', { kind: 'board-entry', topic: 't', entry: 'entry-b' }), 'same-topic entry selection keys are distinct');
assert.notEqual(ctx.qKeyOf('board', { kind: 'board-entry', topic: 't', entry: 'entry-a' }), ctx.qKeyOf('board', { kind: 'board-entry', topic: 'other', entry: 'entry-a' }), 'keys include the owning topic');
qs.q = 'needle';
const excerptTitle = 'prefix '.repeat(30) + 'needle ' + 'suffix '.repeat(30);
assert(ctx.qRowHtml('board', { group: 5, title: excerptTitle }).includes(ctx.qExcerpt(excerptTitle, 'needle')), 'entry renderer uses an excerpt');
assert(!ctx.qRowHtml('board', { group: 5, title: excerptTitle }).includes(excerptTitle));
qs.q = '';

// The tracker slot: hidden items dropped; no query lists the items in progress.
assert.deepEqual(plain(ctx.qTracker('palette').map(x => x.item)), ['a']);
assert.deepEqual(plain(ctx.qTracker('').map(x => x.item)), ['c']);
assert.deepEqual(plain(ctx.qTracker('status:doing').map(x => x.item)), [], 'the tracker slot sends no grammar');

// Session switching remains in the existing rail, not in the search sheet.
const rows = [...ids, { group: 1, kind: 'board-topic', topic: 'pal', title: 'Palette topic', at: 5 }];
qs.rows = rows;
assert.equal(ctx.qStrip, undefined, 'the deleted command-strip path is absent');

// Shortcuts: / outside text fields; Ctrl+K, Ctrl+Space and Cmd+K anywhere but a
// terminal (owner (973)/(974): editors no longer keep them).
function key(key, extra = {}) { return { key, target: { closest: () => null }, ...extra }; }
assert.equal(ctx.qShortcut(key('/')), true);
assert.equal(ctx.qShortcut(key('/', { shiftKey: true })), true, 'slash on shifted keyboard layouts');
for (const selector of ['input', 'textarea', 'select', '[contenteditable]']) {
  assert.equal(ctx.qShortcut(key('/', { target: { closest: s => s.includes(selector.split('[')[0]) ? {} : null } })), false);
}
assert.equal(ctx.qShortcut(key('k', { ctrlKey: true })), true);
assert.equal(ctx.qShortcut(key(' ', { ctrlKey: true, code: 'Space' })), true);
assert.equal(ctx.qShortcut(key('k', { ctrlKey: true, isComposing: true })), false);
function inside(selector) { return { closest: s => s.split(',').map(x => x.trim().split(':')[0]).includes(selector) ? {} : null }; }
assert.equal(ctx.qShortcut(key('k', { ctrlKey: true, target: inside('.xterm') })), false, 'Ctrl+K from a terminal');
assert.equal(ctx.qShortcut(key(' ', { ctrlKey: true, code: 'Space', target: inside('.xterm') })), false, 'Ctrl+Space from a terminal');
assert.equal(ctx.qShortcut(key('k', { metaKey: true, target: inside('.xterm') })), false, 'Cmd+K from a terminal');
for (const target of [inside('textarea'), inside('[contenteditable]'), inside('input')]) {
  assert.equal(ctx.qShortcut(key('k', { ctrlKey: true, target })), true, 'Ctrl+K from an editor');
  assert.equal(ctx.qShortcut(key(' ', { ctrlKey: true, code: 'Space', target })), true, 'Ctrl+Space from an editor');
  assert.equal(ctx.qShortcut(key('k', { metaKey: true, target })), true, 'Cmd+K from an editor');
}
assert.equal(ctx.qShortcut(key('k', { metaKey: true })), true, 'Cmd+K');
assert.equal(ctx.qShortcut(key(' ', { metaKey: true, code: 'Space' })), false, 'Cmd+Space stays the system\'s');
assert.equal(ctx.qShortcut(key('k', { metaKey: true, ctrlKey: true })), false, 'Ctrl+Cmd+K');
assert.equal(ctx.qShortcut(key('k', { metaKey: true, shiftKey: true })), false, 'Cmd+Shift+K');
assert.equal(ctx.qShortcut(key('/', { metaKey: true })), false, 'Cmd+/');
assert.equal(ctx.qShortcut(key('k', { ctrlKey: true, target: inside('body') })), true, 'Ctrl+K from the page body');
assert.equal(ctx.qShortcut(key('/', { keyCode: 229 })), false);
assert.equal(ctx.qShortcut(key('/', { altKey: true })), false);

// --- one open palette, with the host's hooks --------------------------------
const calls = [], pend = [], searches = [];
function later(name) { return (q, signal) => new Promise((ok, no) => { calls.push([name, q]); pend.push({ name, q, signal, ok, no }); }); }
function settle(name, value, fail) { const i = pend.findIndex(p => p.name === name); assert(i >= 0, 'pending ' + name); const p = pend.splice(i, 1)[0]; fail ? p.no(value) : p.ok(value); return p; }
const tick = () => new Promise(r => setImmediate(r));
ctx.AD.search = (id, q, done, o) => { calls.push(['search', id, q, !!o.excerpts]); searches.push({ id, q, done, signal: o.signal }); };
let local = true, updateRows;
Object.assign(ctx.O, { searchRows: u => { updateRows = u; return qs.rows.length ? qs.rows : rows; }, searchPick: (r, o) => events.push(['board', r.id || r.topic || r.href, plain(o || {})]),
  fileFind: later('find'), fileSearch: later('search'), fileList: later('list'), fileLocal: () => local, fileScope: () => 'repo',
  filePreview: (p, o) => events.push(['preview', p, plain(o)]), fileOpen: (n, d) => events.push(['open', n, d]) });

(async () => {
  ctx.qOpen();
  assert.equal(qs.open, true);
  assert.equal(qs.step, 'wide');
  assert(!events.some(x => x[0] === 'modal'), 'search opens without a modal');
  assert.equal(ctx.qIdx(qs.cur), -1, 'opening never arms Enter');
  assert.equal(node('tkqin').value, '');
  assert.equal(S().files.state, 'idle', 'no query: async slots idle');
  assert.equal(timers.length, 0);

  // A query answers the local slots at once and the async slots after the debounce.
  type('palette topic');
  assert.deepEqual(plain(S().board.rows.map(x => x.title)), ['Palette topic']);
  assert.deepEqual(['files', 'content', 'tnote'].map(k => S()[k].state), ['run', 'run', 'run']);
  assert.equal(ctx.qState('files'), wsT('js.tk.qs.st.run', '0'));
  assert.equal(calls.length, 0, 'nothing is fetched before the debounce');
  flush();
  assert.deepEqual(plain(calls), [['find', 'palette topic'], ['search', 'palette topic'], ['search', 'store', 'palette topic', true], ['search', 'cold', 'palette topic', true]],
    'every hook gets the literal query; every store is searched with excerpts');
  const gen1 = pend.slice(), sig1 = searches.map(s => s.signal);

  // A keystroke aborts every request in flight, one per store too, and drops their late answers.
  settle('find', { items: [{ path: 'docs/a.md' }, { path: 'docs', dir: true }] }); await tick();
  assert.deepEqual(plain(S().files.rows.map(x => x.path)), ['docs/a.md'], 'directories are dropped from the files slot');
  type('palette');
  assert(gen1.every(p => p.signal.aborted), 'file requests aborted');
  assert(sig1.every(s => s.aborted), 'every store request aborted');
  assert.equal(S().files.stale, true, 'previous rows stay, dimmed, until the next batch');
  assert.equal(ctx.qState('files'), wsT('js.tk.qs.st.run', '…'));
  assert.deepEqual(plain(S().files.rows.map(x => x.path)), ['docs/a.md']);
  settle('search', { files: [{ path: 'old.go', matches: [{ line: 1, hit: 'x' }] }] });
  searches.splice(0, 2).forEach(s => s.done({ ok: true, ids: ['a'], hits: [{ id: 'a', title: 'stale', field: 'body' }] }));
  await tick();
  assert.equal(S().content.rows.length, 0, 'a stale generation is dropped');
  assert.equal(S().tnote.rows.length, 0);
  flush();

  // Local reranking clears selection; async batches retain an explicit key.
  qs.cur = 'board';
  press('ArrowDown');
  assert.equal(selKey('board'), 'board|board-topic:pal');
  qs.rows = [{ group: 1, kind: 'board-topic', topic: 'new', title: 'Palette new' }].concat(rows);
  ctx.qLocal();
  assert.equal(ctx.qIdx('board'), -1, 'local reranking disarms Enter');
  ctx.qHover({ getAttribute: () => '0' });
  assert.equal(ctx.qIdx('board'), -1, 'hover never arms Enter');
  qs.cur = 'files';
  settle('find', { items: [{ path: 'src/palette.js' }] }); await tick();
  assert.equal(S().files.stale, false);
  assert.deepEqual(plain(S().files.rows.map(x => x.path)), ['src/palette.js'], 'the first batch replaces the stale rows');
  assert.equal(S().files.state, 'done');
  press('ArrowDown');
  assert.equal(selKey('files'), 'files|src/palette.js');
  ctx.qBatch(qs.gen, 'files', [{ path: 'aaa.js', key: 'aaa.js' }], true);
  assert.equal(selKey('files'), 'files|src/palette.js', 'late async batches retain the arrow-selected key');
  assert.deepEqual(plain(S().files.rows.map(x => x.path)), ['src/palette.js', 'aaa.js'], 'batches append rather than replace');
  ctx.qFail(qs.gen, 'files', 'unavailable');
  ctx.qBatch(qs.gen, 'files', [{ path: 'aaa.js', key: 'aaa.js' }], true);
  assert.equal(ctx.qIdx('files'), -1, 'a missing selected key never selects its replacement');

  // Content: a capped answer shows the limit state.
  const many = Array.from({ length: 210 }, (_, i) => ({ path: 'f' + i + '.go', matches: [{ line: i + 1, pre: 'a ', hit: 'palette', post: ' b' }] }));
  settle('search', { files: many, more: true }); await tick();
  assert.equal(S().content.rows.length, 210);
  assert.equal(ctx.qState('content'), wsT('js.tk.qs.st.cap', '210'));
  assert(node('tkqlist').innerHTML.includes(wsT('js.tk.qs.more', '110')), 'the drawn rows stop at QMAX');

  // Tracker content: hidden hits dropped, at most 8 per store, a 400 errs this slot only.
  const hit = (id, extra) => ({ id, title: 'T' + id, alias: id, kind: 'task', status: 'open', field: 'comment', comment: 'c' + id, text: 'about palette', ...extra });
  searches.shift().done({ ok: true, ids: [], hits: [hit('a'), hit('b'), hit('h', { hidden: true }), ...Array.from({ length: 9 }, (_, i) => hit('m' + i))] });
  assert.equal(S().tnote.state, 'run', 'one store still pending');
  assert.equal(S().tnote.rows.length, 8);
  assert(!S().tnote.rows.some(r => r.item === 'b' || r.item === 'h'), 'hidden hits (by the hit or the loaded view) dropped');
  searches.shift().done({ ok: false, status: 404, error: 'gone' });
  assert.equal(S().tnote.state, 'done');
  assert.equal(S().tnote.more, true);
  assert.equal(ctx.qState('tnote'), wsT('js.tk.qs.st.cap', '8'));

  // Opening a tracker content hit: a loaded store opens at the comment, an unloaded one takes the route.
  ctx.qPickEntry({ slot: 'tnote', row: S().tnote.rows[0] }, true);
  assert(events.some(x => x[0] === 'item' && x[1] === 'store' && x[2] === 'a' && x[3].focus === 'ca'));
  assert.equal(qs.open, true, 'Ctrl+Enter keeps the palette open');
  ctx.qPickEntry({ slot: 'tnote', row: { store: 'cold', item: 'x 1', comment: 'c' } }, true);
  assert.equal(ctx.location.hash, '#/cold/x%201');

  // A grammar refusal (400) puts only the tracker content slot in its error state.
  type('status:bogus');
  flush();
  settle('find', { items: [] }); settle('search', { files: [] });
  searches.shift().done({ ok: false, status: 400, error: 'unknown status' });
  searches.shift().done({ ok: true, ids: [], hits: [hit('z')] });
  await tick();
  assert.equal(S().tnote.state, 'err');
  assert.equal(S().tnote.err, 'unknown status');
  assert.equal(S().tnote.rows.length, 0, 'a later store answer does not clear the error');
  assert.equal(S().files.state, 'done');
  type('status:hidden palette');
  flush();
  searches.shift().done({ ok: true, ids: [], hits: [hit('b'), hit('h', { hidden: true })] });
  assert.deepEqual(plain(S().tnote.rows.map(r => r.item)), ['b', 'h'], 'status:hidden keeps hidden hits');
  searches.splice(0);
  pend.splice(0);

  // Keys: Tab skips empty slots, Alt+1–5 use e.code; deleted strip shortcuts stay absent.
  type('palette'); flush();
  settle('find', { items: [] }); settle('search', { files: [] });
  searches.splice(0).forEach(s => s.done({ ok: true, ids: [], hits: [] }));
  await tick();
  qs.cur = 'board';
  press('Tab');
  assert.equal(qs.cur, 'tracker', 'Tab skips the empty files and content slots');
  assert.equal(ctx.qIdx('tracker'), -1, 'slot changes disarm Enter');
  press('Tab', { shiftKey: true });
  assert.equal(qs.cur, 'board');
  press('¢', { altKey: true, code: 'Digit4' });
  assert.equal(qs.cur, 'tracker', 'Alt+4 matched on e.code (macOS Option changes e.key)');
  const altGr = press('²', { altKey: true, ctrlKey: true, code: 'Digit2' });
  assert.equal(qs.cur, 'tracker', 'AltGr+2 (Ctrl+Alt) types instead of jumping');
  assert(!altGr.prevented);
  const altZero = press('º', { altKey: true, code: 'Digit0' });
  assert.equal(qs.cur, 'tracker');
  assert(!altZero.prevented, 'Alt+0 no longer routes to a deleted strip');
  press('ArrowUp');
  assert.equal(ctx.qIdx('tracker'), 0, 'the first arrow selects the first row');
  press('ArrowUp');
  assert.equal(qs.cur, 'tracker', 'Up at the first row stays in its result slot');
  assert.equal(ctx.qIdx('tracker'), 0);
  qs.cur = 'board';

  // Enter without an arrow selection does nothing.
  const before = events.length;
  press('Enter');
  press('Enter', { ctrlKey: true });
  press('Enter', { metaKey: true });
  assert.equal(events.length, before, 'Enter without selection does nothing');
  assert.equal(qs.open, true);
  type('  '); press('Enter');
  assert.equal(events.length, before, 'empty Enter does nothing');
  type('palette');
  assert.equal(ctx.qIdx('board'), -1, 'query edits clear selection');
  press('ArrowDown');
  // Ctrl/Cmd+Enter open the arrow-selected board target behind the sheet.
  press('Enter', { metaKey: true });
  assert.deepEqual(events.filter(x => x[0] === 'board').pop()[2], { keep: true });
  press('Enter', { ctrlKey: true });
  assert.deepEqual(events.filter(x => x[0] === 'board').pop()[2], { keep: true });
  assert.equal(qs.open, true);
  assert.deepEqual(events[events.length - 1], ['focus', 'tkqin']);
  press('Enter');
  assert.equal(qs.open, false);
  assert.deepEqual(events.filter(x => x[0] === 'board').pop()[2], { keep: false });

  // Reopen restores the last query, selected; Esc clears, then closes.
  ctx.qOpen();
  assert.equal(node('tkqin').value, 'palette');
  assert.deepEqual(events.slice(-2).map(x => x[0]), ['focus', 'select']);
  press('Escape');
  assert.equal(qs.open, true);
  assert.equal(node('tkqin').value, '');
  press('Escape');
  assert.equal(qs.open, false);

  // (n) focuses its slot, but exact matches still require an arrow.
  width = 820;
  ctx.qOpen();
  assert.equal(qs.step, 'medium');
  type('(153)');
  assert.deepEqual(plain(S().board.rows.map(x => x.id)), ['153']);
  assert.equal(qs.cur, 'board');
  assert.equal(ctx.qIdx('board'), -1);
  press('ArrowDown');
  assert.equal(selKey('board'), 'board|board-item:153');
  assert(node('tkqlist').innerHTML.includes('tk-qexact'));
  ctx.qClose();
  width = 420;
  ctx.qOpen();
  assert.equal(qs.step, 'narrow');
  type('(153)');
  assert.equal(qs.cur, 'all');
  assert.equal(ctx.qIdx('all'), -1);
  press('ArrowDown');
  assert.equal(selKey('all'), 'board|board-item:153');
  assert.equal(node('tkqtabs').hidden, false);

  // Path mode: absolute paths on a local viewer list one directory deep.
  assert.deepEqual(['/', '/tmp/x', 'C:\\Users', 'c:/x'].map(ctx.qAbsPath), [true, true, true, true]);
  assert.deepEqual(['', '~/x', 'rel/x', '//host/s', '\\\\host\\s', '\\\\?\\C:\\x', '\\\\.\\x', 'C:\\x\\con', 'C:\\aux.txt', 'C:x', '/a\u0000b'].map(ctx.qAbsPath),
    Array(11).fill(false));
  pend.splice(0); calls.splice(0); searches.splice(0);
  type('/tmp/fo');
  assert.deepEqual(plain(qs.path), { dir: '/tmp/', leaf: 'fo', sep: '/' });
  assert.equal(S().content.state, 'idle', 'path mode pauses content');
  assert.equal(ctx.qState('content'), wsT('js.tk.qs.st.path'));
  flush();
  assert.deepEqual(plain(calls.filter(c => c[0] !== 'search')), [['list', '/tmp/']]);
  settle('list', { entries: [{ name: 'bar', dir: false }, { name: 'xfoy' }, { name: 'food', dir: true }, { name: 'fo.txt' }, { name: 'fold', link: true }, { name: 'fogone', gone: true }] }); await tick();
  assert.deepEqual(plain(S().files.rows.map(r => r.name)), ['food', 'fo.txt', 'xfoy'], 'prefix matches first, then fuzzy; links and gone files dropped');
  qs.cur = 'files';
  press('ArrowDown');
  press('Enter');
  assert.equal(node('tkqin').value, '/tmp/food/', 'Enter on a folder completes the path');
  assert.equal(qs.open, true);
  flush();
  assert.equal(pend[pend.length - 1].q, '/tmp/food/');
  settle('list', { entries: [{ name: 'a.txt' }] }); await tick();
  press('ArrowDown');
  press('Enter', { ctrlKey: true });
  assert.deepEqual(events[events.length - 2], ['open', 'a.txt', '/tmp/food/'], 'Ctrl+Enter opens a file natively');
  assert.equal(qs.open, true);
  press('Enter');
  assert.equal(qs.open, false);
  assert.deepEqual(events.filter(x => x[0] === 'preview').pop(), ['preview', 'a.txt', { root: '/tmp/food/' }]);
  ctx.qOpen();
  assert.equal(qs.cur, 'files', 'reopening preserves the selected slot');
  type('C:\\Users\\');
  assert.deepEqual(plain(qs.path), { dir: 'C:\\Users\\', leaf: '', sep: '\\' });
  type('/nope/');
  flush();
  settle('list', { status: 404, text: 'no such directory' }, true); await tick();
  assert.equal(S().files.state, 'err');
  assert(node('tkqlist').innerHTML.includes(wsT('js.tk.qs.listfail', 'no such directory')), 'the refusal shows as an error row');
  // Leaving path mode with listed rows, and entering it with found files, drops the other kind's rows.
  type('/tmp/');
  flush();
  settle('list', { entries: [{ name: 'a.txt' }] }); await tick();
  assert.equal(S().files.rows.length, 1);
  type('a');
  assert.equal(S().files.rows.length, 0, 'path rows are not kept as stale found files');
  assert.equal(S().files.state, 'run');
  assert.equal(timers.length, 1, 'the next fetch is armed');
  flush();
  settle('find', { items: [{ path: 'src/a.go' }] }); await tick();
  type('/tmp/');
  assert.equal(S().files.rows.length, 0, 'found files are not kept as stale path rows');
  pend.splice(0); timers = [];
  // A LAN viewer never enters path mode: the text is an ordinary query.
  local = false; pend.splice(0); calls.splice(0); searches.splice(0);
  type('/tmp/fo');
  assert.equal(qs.path, null);
  flush();
  assert.deepEqual(plain(calls.filter(c => c.length === 2)), [['find', '/tmp/fo'], ['search', '/tmp/fo']]);
  local = true;
  // A content pick previews the file at its line.
  settle('find', { items: [] }); settle('search', { files: [{ path: 'a/b.go', matches: [{ line: 7, hit: 'fo' }] }] }); await tick();
  qs.cur = 'content';
  press('ArrowDown');
  press('Enter');
  assert.deepEqual(events.filter(x => x[0] === 'preview').pop(), ['preview', 'a/b.go', { line: 7 }]);
  assert.equal(qs.open, false);

  // Late host rows (an older closed page) re-rank the local slots for the open query.
  ctx.qOpen(); type('(999)');
  assert.equal(S().board.rows.length, 0);
  updateRows([{ group: 3, kind: 'board-item', id: '999', title: 'Older closed item' }]);
  assert.deepEqual(plain(S().board.rows.map(x => x.id)), ['999'], 'closed-page arrivals refresh the current query');
  ctx.qClose();
  // A pick of a hidden tracker item keeps the hidden-item toast.
  ctx.qOpen(); type('');
  ctx.qPickEntry({ slot: 'tracker', row: { store: 'store', item: 'b' } });
  assert(events.some(x => x[0] === 'toast'));

  // The production keyboard handler rejects IME confirmation before qKey.
  const inputKey = block(tracker, "    $('tkqin').addEventListener('keydown'", '    // the caret moved without an edit');
  assert(inputKey.indexOf('e.isComposing || e.keyCode === 229') < inputKey.indexOf('qKey(e)'));
  assert(!tracker.includes("$('tkqpop').addEventListener('cancel'"), 'no dialog cancel handler remains');

  // A fresh renderer restores browser-owned query/slot, never arrow selection.
  let saved = { query: 'palette', slot: 'tracker' };
  const hostOpen = [];
  ctx.qClose();
  ctx.O.searchState = { load: () => saved, save: value => { saved = plain(value); } };
  ctx.O.searchHost = { setOpen: open => hostOpen.push(open) };
  qs.loaded = false;
  ctx.qOpen();
  assert.equal(node('tkqin').value, 'palette');
  assert.equal(qs.cur, 'tracker');
  assert.equal(ctx.qIdx('tracker'), -1);
  press('ArrowDown');
  press('¢', { altKey: true, code: 'Digit1' });
  assert.equal(ctx.qIdx('tracker'), -1, 'Alt slot switches clear previous selection');
  assert.deepEqual(saved, { query: 'palette', slot: 'board' });
  const imeBefore = events.length;
  press('Enter', { isComposing: true });
  press('Enter', { keyCode: 229 });
  assert.equal(events.length, imeBefore, 'IME confirmation does not open a row');
  ctx.qClose();
  assert.deepEqual(hostOpen, [true, false], 'renderer delegates sheet open/close to host');

  // Session pages omit #tkqs when #shandle/searchHost owns the search trigger;
  // qOpen, qClose, and item-window openers must not throw when #tkqs is absent.
  const prevDollar = ctx.$, prevBack = ctx.document.activeElement;
  const fallbackEvents = [];
  const shandleNode = { id: 'shandle', isConnected: true, focus() { fallbackEvents.push(['focus', 'shandle']); } };
  ctx.$ = id => id === 'tkqs' ? null : id === 'shandle' ? shandleNode : node(id);
  ctx.document.activeElement = { isConnected: false, focus() { fallbackEvents.push(['focus', 'stale']); } };
  ctx.qOpen();
  ctx.qClose();
  assert.deepEqual(fallbackEvents, [['focus', 'shandle']], 'qClose falls back to #shandle when #tkqs is absent and opener is disconnected');
  ctx.qOpenHit({ store: 'store', item: 'a' });
  assert.equal(events.filter(x => x[0] === 'item').pop()[3].opener.id, 'shandle', 'qOpenHit falls back to #shandle opener');
  ctx.qPickEntry({ slot: 'tracker', row: { store: 'store', item: 'a' } }, true);
  assert.equal(events.filter(x => x[0] === 'item').pop()[3].opener.id, 'shandle', 'qPickEntry tracker row falls back to #shandle opener');
  // Also safe when neither #tkqs nor #shandle exists (isolated fixtures).
  ctx.$ = id => (id === 'tkqs' || id === 'shandle') ? null : node(id);
  ctx.qOpen();
  ctx.qClose();
  ctx.qOpenHit({ store: 'store', item: 'a' });
  assert.equal(events.filter(x => x[0] === 'item').pop()[3].opener, null, 'qOpenHit passes null opener when neither trigger exists');
  ctx.$ = prevDollar;
  ctx.document.activeElement = prevBack;

  await hostCases();
  if (process.argv[2]) require('./search-palette-dom.cjs')(dashboard, JSON.parse(fs.readFileSync(process.argv[2], 'utf8')));
  console.log('search palette: scorer, slots, ids, generations, keys, picks, path mode, and host hooks passed');
})().catch(e => { console.error(e); process.exit(1); });

// --- the host's palette pieces ---------------------------------------------
async function hostCases() {
  // Open, unanswered items have no answer chip; an unmounted frame reads data-srcdoc.
  const textNode = text => ({ cloneNode: () => ({ textContent: text }) });
  const unanswered = { id: 'item-42', hasAttribute: () => true, querySelector: () => textNode('Unanswered question') };
  // the board frame as a mounted iframe (srcdoc) and as an unmounted placeholder div (data-srcdoc)
  const frames = { iframe: { getAttribute: k => k === 'srcdoc' ? '<div class="doc-body">Mounted body</div>' : null },
    placeholder: { getAttribute: k => k === 'data-srcdoc' ? '<div class="doc-body">Deferred body</div>' : null } };
  let frame = null, parsedFrom = '';
  const boardTopic = { getAttribute: () => 'example', querySelector: s => s === '.tname' ? textNode('Example') : s === ':scope > .tdoc > .doc-body' ? { querySelector: f => f === '.doc-frame' ? frame : null } : null };
  const rowsCtx = vm.createContext({ framed: false, readItem: () => null, boardQ: () => [boardTopic],
    doc: { createElement: () => ({ set innerHTML(v) { parsedFrom = v; }, content: { querySelector: () => textNode(parsedFrom) } }) },
    q: (s, root) => s === '[id^="item-"]' && root === boardTopic ? [unanswered] : [] });
  vm.runInContext(block(dashboard, '  function paletteText(', '  function paletteClosed('), rowsCtx);
  for (const [kind, text] of [['iframe', 'Mounted body'], ['placeholder', 'Deferred body']]) {
    frame = frames[kind];
    const got = rowsCtx.paletteRows();
    assert.deepEqual(plain(got.map(x => x.group)), [1, 2, 3]);
    assert(got[1].title.includes(text), 'the ' + kind + ' frame body is searched');
  }

  // Entry extraction uses each article's own payload, not its nested versions.
  // History pages and timeline gaps stay inert while their contents are indexed.
  const leaf = (text, owner) => ({ textContent: text, closest: () => owner,
    cloneNode() { return leaf(this.textContent, owner); }, querySelector: () => null });
  function article(id, fields) {
    const e = { id, fields: {}, querySelector: () => null };
    for (const [key, text] of Object.entries(fields)) e.fields[key] = [leaf(text, e)];
    return e;
  }
  const log = article('entry-log', { '.entry-text': 'Live log phrase', 'time.entry-at': '10:01' });
  const system = article('entry-system', { '.entry-text': 'Folded system phrase', 'time.entry-at': '10:02' });
  const old = article('entry-version', { '.entry-recap': 'Earlier version', '.entry-body': 'Historical payload', 'time.entry-at': '09:00' });
  const panel = article('entry-panel', { '.entry-recap': 'Latest panel recap', '.entry-body': 'Latest body phrase', 'time.entry-at': '10:03' });
  for (const key of Object.keys(old.fields)) panel.fields[key] = (panel.fields[key] || []).concat(old.fields[key]);
  const earlier = article('entry-older', { '.entry-text': 'Older-page-only phrase', 'time.entry-at': '08:00' });
  // The time lives inside entry-text in real log rows. Remove from the clone,
  // not from the original article or its time chip.
  const timedLine = log.fields['.entry-text'][0];
  timedLine.textContent = '10:01 — Live log phrase';
  timedLine.cloneNode = () => {
    const copy = leaf(timedLine.textContent, log);
    copy.times = [{ remove() { copy.textContent = '— Live log phrase'; } }];
    return copy;
  };
  const gapEntry = article('e17', { '.entry-text': 'Unloaded-gap-only phrase', 'time.entry-at': '08:30' });
  const pageContent = { entries: [earlier] }, historyPage = { content: pageContent };
  const gapContent = { entries: [gapEntry] }, historyGap = { content: gapContent };
  const stream = { entries: [log, system, panel, old] };
  let ownBody = null;
  const topicWithStream = { getAttribute: () => 'stream-topic', querySelector: selector => selector === '.tname' ? leaf('Stream topic') :
    selector === ':scope > .tdoc > .doc-body' ? ownBody : selector === '.rtext' ? leaf('Topic recap') : null };
  function query(selector, root) {
    if (selector === '.topic-stream' && root === topicWithStream) return [stream];
    if (root === stream && selector.startsWith('template[')) {
      return selector.split(',').flatMap(s => s.trim() === 'template[data-history-page]' ? [historyPage] : s.trim() === 'template[data-history-gap]' ? [historyGap] : []);
    }
    if (selector === 'article.wb-entry') return root.entries || [];
    if (selector === 'time') return root.times || [];
    return root.fields && root.fields[selector] || [];
  }
  const entryCtx = vm.createContext({ framed: false, readItem: () => null, boardQ: () => [topicWithStream], q: query, doc: rowsCtx.doc });
  vm.runInContext(block(dashboard, '  function paletteText(', '  function paletteClosed('), entryCtx);
  const beforeIndex = JSON.stringify([stream, pageContent, gapContent, timedLine.textContent]);
  const indexed = plain(entryCtx.paletteRows());
  assert(!indexed.some(r => r.group === 2), 'a recap-only topic has no body row even when its stream has a body');
  assert.deepEqual(indexed.filter(r => r.group === 5).map(r => [r.entry, r.title]), [
    ['entry-log', 'Live log phrase'], ['entry-system', 'Folded system phrase'], ['entry-panel', 'Latest panel recap Latest body phrase'],
    ['entry-version', 'Earlier version Historical payload'], ['entry-older', 'Older-page-only phrase'], ['e17', 'Unloaded-gap-only phrase']]);
  assert.equal(indexed.find(r => r.entry === 'entry-log').hint, 'Stream topic · 10:01');
  assert.equal(JSON.stringify([stream, pageContent, gapContent, timedLine.textContent]), beforeIndex, 'indexing does not mutate live rows or history templates');
  ownBody = leaf('Topic-owned body');
  assert.equal(entryCtx.paletteRows().find(r => r.group === 2).title, 'Topic recap Topic-owned body', 'topic body excludes panel text');
  // Nested versions cannot donate their recap/body to a payload-less current panel.
  const emptyPanel = article('entry-empty', {});
  emptyPanel.fields = old.fields;
  assert.equal(entryCtx.paletteEntryText(emptyPanel), '');
  // A panel frame uses the same inert source path as a topic frame.
  const framedBody = leaf('opaque frame contents', panel);
  framedBody.querySelector = () => frames.placeholder;
  panel.fields['.entry-body'] = [framedBody, ...old.fields['.entry-body']];
  assert.equal(entryCtx.paletteEntryText(panel), 'Latest panel recap <div class="doc-body">Deferred body</div>');

  // Jump against a fresh node; the whole-topic owner must run before any
  // landing geometry or focus, without opening unrelated disclosures.
  let folded = true, active = 'palette';
  const fold = { tagName: 'DETAILS', open: false };
  const topic = { parentElement: {}, querySelector: s => s === '.tdoc' ? body : null, getAttribute: () => 'closed' };
  function landing(tagName, parentElement) {
    return { parentElement, tagName, classList: { remove() {}, add() {} },
      matches: () => false, setAttribute() {},
      focus() { assert.equal(folded, false, 'owner reveals before focus'); active = this; events.push(['board-focus']); },
      scrollIntoView() {
        assert.equal(folded, false, 'owner reveals before scroll');
        assert.equal(this === body ? body.open : fold.open, true, 'target disclosure opens before scroll');
        events.push(['scroll']);
      },
      get offsetWidth() { assert.equal(folded, false, 'owner reveals before flash geometry'); events.push(['width']); return 0; } };
  }
  const body = landing('DETAILS', topic), target = landing('DIV', fold);
  fold.parentElement = topic;
  let pickTopic = topic;
  const pickCtx = vm.createContext({ doc: { body: { classList: { contains: () => false } }, documentElement: {} },
    window: { WsCards: { unfoldTopic(node) { assert.equal(node, pickTopic); folded = false; events.push(['unfold']); } } },
    boardQ: () => pickTopic ? [pickTopic] : [], $: () => target, live: true, tabShown: () => false,
    setTab: tab => { assert.equal(folded, false, 'owner reveals before tab handoff'); events.push(['tab', tab]); },
    setTimeout: fn => fn(), location: {}, go: href => { pickCtx.location.href = href; } });
  vm.runInContext(block(dashboard, '  function palettePick(', "  // The palette's file hooks"), pickCtx);
  for (const kind of ['board-body', 'board-item', 'board-answer']) {
    for (const keep of [false, true]) {
      folded = true; fold.open = false; body.open = false; active = 'palette'; events.length = 0;
      pickCtx.palettePick({ kind, topic: 'closed', ...(kind === 'board-body' ? {} : { id: '153' }) }, { keep });
      assert.deepEqual(events[0], ['unfold'], kind + ' reveals through the owner first');
      assert(events.some(x => x[0] === 'scroll') && events.some(x => x[0] === 'width'));
      assert(events.some(x => x[0] === 'tab' && x[1] === 'board'));
      assert.equal(active, keep ? 'palette' : kind === 'board-body' ? body : target, 'keep retains palette focus');
      assert.equal(kind === 'board-body' ? fold.open : body.open, false, 'unrelated inner disclosure stays closed');
    }
  }
  // Both palette gestures use overview's exit/landing path; the host must
  // not scroll inside a folded topic, even when Ctrl+Enter keeps it open.
  const overviewPicks = [];
  pickCtx.window.WsOverview = { pick: (node, opt) => { overviewPicks.push([node === topic, !!(opt && opt.keep)]); return true; } };
  events.length = 0; folded = true; active = 'palette';
  pickCtx.palettePick({ kind: 'board-item', topic: 'closed', id: '153' }, { keep: true });
  pickCtx.palettePick({ kind: 'board-item', topic: 'closed', id: '153' });
  assert.deepEqual(overviewPicks, [[true, true], [true, false]]);
  assert.equal(events.length, 0, 'overview owns all landing scrolls and bypasses the fold owner');
  assert.equal(folded, true);
  assert.equal(active, 'palette');
  pickCtx.palettePick({ kind: 'session', href: '/s/other' });
  assert.equal(pickCtx.location.href, '/s/other');

  const entryTarget = { setAttribute(k, v) { assert.equal(k, 'tabindex'); assert.equal(v, '-1'); },
    focus(opt) { assert.equal(folded, false); assert.deepEqual(plain(opt), { preventScroll: true }); active = 'entry'; events.push(['entry-focus']); },
    scrollIntoView() { assert.fail('entry picks must never scroll the history clip'); },
    get offsetWidth() { assert.fail('loadTarget alone owns entry flash geometry'); } };
  const entryRow = { ...indexed.find(r => r.entry === gapEntry.id), topic: 'closed' };
  let loads = 0, paged = true;
  pickCtx.tkApi = { leavePage() { assert.equal(folded, false); events.push(['leave-page']); } };
  pickCtx.doc.body.classList.contains = () => paged;
  pickCtx.window.wbHistory = { loadTarget(owner, id, beforeGeometry) {
    loads++;
    assert.equal(owner, pickTopic, 'the current owning topic is the history root, including a pinned topic');
    assert.equal(id, entryRow.entry);
    beforeGeometry(owner); beforeGeometry(entryTarget);
    assert.equal(events.filter(e => e[0] === 'unfold').length, 1, 'prepare runs once despite repeated pre-geometry callbacks');
    assert.equal(events.filter(e => e[0] === 'leave-page').length, 1);
    assert.equal(folded, false, 'topic is revealed before history geometry');
    return entryTarget;
  } };
  pickCtx.window.WsOverview = { pick: () => false };
  for (const keep of [false, true]) {
    folded = true; active = 'palette'; events.length = 0; loads = 0;
    pickCtx.palettePick(entryRow, { keep });
    assert.equal(loads, 1, 'declined Overview goes straight to loadTarget');
    assert.equal(active, keep ? 'palette' : 'entry', 'modified entry Enter keeps focus in search');
    assert(!events.some(e => e[0] === 'scroll' || e[0] === 'width'), 'no generic entry scrolling or flash');
  }
  const after = [], handoffs = [];
  let holding = true;
  pickCtx.window.WsOverview = { pick(node, opt) { handoffs.push([node, opt]); return true; },
    after: fn => after.push(fn), holding: () => holding };
  events.length = 0; loads = 0; folded = true; active = 'palette';
  pickCtx.palettePick(entryRow, { keep: true });
  assert.equal(handoffs[0][0], topic);
  assert.equal(handoffs[0][1].keep, true);
  assert.equal(loads, 0);
  assert.equal(events.length, 0, 'no loadTarget, folds, page or tab writes before Overview after');
  after.shift()();
  assert.equal(handoffs.length, 2, 'an entering Overview repeats its handoff');
  assert.equal(loads, 0);
  assert.equal(events.length, 0, 'still-held callback does not reach history or fold owners');
  // The board (or pinned column) may be replaced while Overview holds swaps.
  pickTopic = { ...topic };
  holding = false;
  after.shift()();
  assert.equal(loads, 1, 'after exit, navigation reaches the entry in the replacement topic');
  assert.equal(active, 'palette');
  assert.equal(handoffs.length, 2, 'the completed exit is not handed off a third time');
  // Reduced motion exits synchronously, so after may navigate immediately.
  pickCtx.window.WsOverview = { pick: () => true, holding: () => false, after: fn => fn() };
  events.length = 0; loads = 0; folded = true; active = 'palette';
  pickCtx.palettePick(entryRow);
  assert.equal(loads, 1);
  assert.equal(active, 'entry', 'synchronous Overview exit still focuses the article without scrolling it');

  const originalPick = pickCtx.palettePick, fallbacks = [];
  pickCtx.palettePick = (row, opt) => fallbacks.push([plain(row), plain(opt)]);
  pickCtx.window.WsOverview = { pick: () => false };
  pickCtx.window.wbHistory.loadTarget = () => null;
  originalPick(entryRow, { keep: true });
  assert.deepEqual(fallbacks, [[{ kind: 'board-topic', topic: 'closed' }, { keep: true }]], 'a disappeared entry falls back to its topic');
  pickCtx.palettePick = originalPick;
  pickTopic = null;
  events.length = 0;
  pickCtx.palettePick(entryRow);
  assert.equal(events.length, 0, 'a disappeared topic is ignored');
  paged = false;

  let more = {}, updates = 0;
  const historyUpdates = [];
  const closedCtx = vm.createContext({ boardQ: () => more ? [more] : [], paletteRows: () => ['older rows'],
    paletteHistory: update => historyUpdates.push(update),
    loadClosed: () => ({ then(done) { more = null; done(true); } }) });
  vm.runInContext(block(dashboard, '  function paletteClosed(', '  function palettePick('), closedCtx);
  const update = rows => { assert.deepEqual(plain(rows), ['older rows']); updates++; };
  closedCtx.paletteClosed(update);
  assert.equal(updates, 1, 'closed pagination updates the palette and stops at the last page');
  assert.deepEqual(historyUpdates, [update], 'a loaded closed page hydrates history through the same update callback');

  // The file hooks: the session's fs routes with the literal query, list by root, refusals as {status, text}.
  const fetched = [], posts = [];
  const fsCtx = vm.createContext({ rid: 'r1',
    root: { hasAttribute: k => k === 'data-sysopen' }, region: () => ({ querySelector: () => ({ textContent: ' ~/repo ' }) }),
    window: { fetch: true, WsPreview: { open: (p, o) => posts.push(['preview', p, o]) } }, wsT, toast: t => posts.push(['toast', t]),
    post: (r, b, done) => { posts.push(['post', r, b]); done(false, 'denied'); },
    fetch: (url, o) => { fetched.push([url, !!o.signal]); return Promise.resolve(url.includes('nope') ? { ok: false, status: 400, text: () => Promise.resolve('bad root\n') } : { ok: true, json: () => Promise.resolve({ items: [] }) }); } });
  vm.runInContext(block(dashboard, '  function paletteFs(', '\n  function paletteSearch('), fsCtx);
  const hooks = fsCtx.paletteFiles({});
  assert.equal(hooks.fileLocal(), true);
  assert.equal(hooks.fileScope(), '~/repo');
  const sig = new AbortController().signal;
  hooks.fileFind('a b&c', sig); hooks.fileSearch('x', sig); hooks.fileList('/tmp/', sig);
  assert.deepEqual(fetched, [['/s/r1/fs/find?q=a%20b%26c', true], ['/s/r1/fs/search?q=x', true], ['/s/r1/fs/list?root=%2Ftmp%2F', true]]);
  await hooks.fileList('/nope/').then(() => assert.fail('a refusal rejects'), err => assert.deepEqual(plain(err), { status: 400, text: 'bad root' }));
  hooks.filePreview('a.txt', { root: '/tmp/' });
  hooks.fileOpen('a.txt', '/tmp/');
  assert.deepEqual(plain(posts), [['preview', 'a.txt', { root: '/tmp/' }], ['post', 'fs/act', { path: 'a.txt', action: 'open', root: '/tmp/' }], ['toast', wsT('js.toast.openfail', 'denied')]]);
  fsCtx.rid = '';
  assert.deepEqual(Object.keys(fsCtx.paletteFiles({ a: 1 })), ['a'], 'a page without a session has no file hooks');

  // Hosts retain persistence and shared-sheet hooks.
  const storage = {}, sheetCalls = [];
  const searchCtx = vm.createContext({ rid: '', root: { hasAttribute: () => false }, window: {}, sbody: {},
    drOpen: false, drMode: 'panels', load: k => storage[k], save: (k, v) => { storage[k] = v; },
    drSetOpen: (open, mode) => { sheetCalls.push([open, mode]); searchCtx.drOpen = open; if (mode) searchCtx.drMode = mode; } });
  vm.runInContext(block(dashboard, '  function paletteSearch(', '\n  function tkMount('), searchCtx);
  const remote = searchCtx.paletteSearch({});
  remote.searchState.save({ query: 'remember', slot: 'content' });
  const reloaded = searchCtx.paletteSearch({});
  assert.deepEqual(plain(reloaded.searchState.load()), { query: 'remember', slot: 'content' });
  reloaded.searchHost.setOpen(true); reloaded.searchHost.setOpen(true); reloaded.searchHost.setOpen(false);
  assert.deepEqual(sheetCalls, [[true, 'search'], [false, undefined]], 'sheet hooks are idempotent');
  storage['ws.dashboard.drawer.search.state'] = 'invalid JSON';
  assert.equal(searchCtx.paletteSearch({}).searchState.load(), null);
  assert(dashboard.includes('if (load(DR_SEARCH) === "1" || tkWantSearch) tkApi.openSearch();'), 'reload restores the open search sheet');

  // The shortcut before the first store list mounts the palette is a pending
  // open that mount honours, never a dropped key.
  const pendCtx = vm.createContext({ tkApi: null });
  vm.runInContext(block(dashboard, '  var tkWantSearch = false;', '\n  function trackerStart('), pendCtx);
  pendCtx.tkSearch();
  assert.equal(pendCtx.tkWantSearch, true, 'a shortcut before mount is kept');
  let opened = 0;
  pendCtx.tkApi = { openSearch: () => { opened++; } };
  pendCtx.tkSearch();
  assert.equal(opened, 1, 'after mount the shortcut opens at once');
  assert(/function trackerStart\(\) \{[\s\S]*?doc\.addEventListener\("keydown", function \(e\) \{\n      if \(tkApi \|\| e\.defaultPrevented \|\| !window\.WsTracker\.searchKey[\s\S]*?tkSearch\(\);[\s\S]*?\}, true\);\n    tkReload\(\);/.test(dashboard), 'the page counts the shortcut before mount with the palette\'s own target rule');

  // The store search adapter asks for excerpts and passes the abort signal and the status.
  const adapter = block(dashboard, '    search: function (id, q, done, o) {', "    // The store's wiki branches");
  const adCtx = vm.createContext({ tkUrl: (id, p) => '/t/' + id + '/' + p, tkErrText: (s, v) => v.error,
    tkFetch: (url, body, done, keep, signal) => { adCtx.seen = [url, !!signal]; done(400, { error: 'bad' }); } });
  vm.runInContext('var A = {' + adapter + '};', adCtx);
  let got;
  adCtx.A.search('st', 'a b', r => { got = r; }, { excerpts: true, signal: {} });
  assert.deepEqual(plain(adCtx.seen), ['/t/st/search?q=a%20b&excerpts=1', true]);
  assert.deepEqual(plain(got), { ok: false, error: 'bad', status: 400 });
  adCtx.A.search('st', 'x', () => {});
  assert.equal(adCtx.seen[0], '/t/st/search?q=x', 'the Items panel search is unchanged');
}
