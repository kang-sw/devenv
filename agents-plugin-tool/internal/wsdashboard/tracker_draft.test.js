const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

const source = fs.readFileSync(path.join(__dirname, 'assets/tracker.js'), 'utf8');
function section(start, end) {
  const a = source.indexOf(start), b = source.indexOf(end, a);
  assert.ok(a >= 0 && b > a);
  return source.slice(a, b);
}
function storage() {
  const data = new Map();
  return { getItem: k => data.has(k) ? data.get(k) : null, setItem: (k, v) => data.set(k, v), removeItem: k => data.delete(k), data };
}
function harness(localStorage = storage()) {
  const ui = { store: 'a', itemDrafts: Object.create(null), eatt: {}, edpv: {}, iedit: null };
  const elements = { 'iw-etitle': { value: 'Title' }, 'iw-ebody': { value: 'Body' }, 'iw-eproj': { value: '' }, tkwinbody: { querySelector: () => null } };
  const item = { id: '1', title: 'Title', body: 'Body', project: '', labels: [], mentions: [] };
  const c = { ui, localStorage, PFX: 'ws.tracker.', VS: { items: { '1': item } }, $: id => elements[id], render() {}, toast() {}, wsT: x => x,
    sameLabels: (a, b) => JSON.stringify(a) === JSON.stringify(b), labelsOver: (a, b) => b, labelPicks: (a, b) => b,
    keptMentions: () => [], withPicks: () => [], refsIn: () => [], savedAtts: () => [],
    commit: (events, then, prep) => { c.events = events; c.finish = prep; } };
  vm.createContext(c);
  vm.runInContext(section('  function draftBucket(', '  function startCedit('), c);
  function edit() { return { item: '1', title: 'Title', origTitle: 'Title', body: 'Body', seen: 'Body', base: ['rev'], project: '', origProject: '', labels: [], lorig: [], mpick: [] }; }
  return { c, ui, elements, edit, localStorage };
}

test('drafts isolate stores and items and preserve merge metadata and uploads', () => {
  const { c, ui, edit } = harness();
  ui.iedit = edit(); ui.iedit.body = 'changed'; ui.iedit.mpick.push('member');
  ui.eatt['iw-ebody'] = [{ oid: 'image' }]; c.keepItemDraft();
  assert.equal(c.restoreItemDraft('b', '1'), null);
  assert.equal(c.restoreItemDraft('a', '2'), null);
  const restored = c.restoreItemDraft('a', '1');
  assert.equal(restored.body, 'changed'); assert.equal(restored.seen, 'Body');
  assert.equal(restored.base[0], 'rev'); assert.equal(restored.mpick[0], 'member');
  assert.equal(ui.eatt['iw-ebody'][0].oid, 'image'); assert.equal(restored.restored, true);
  restored.body = 'later'; assert.equal(c.restoreItemDraft('a', '1').body, 'changed');
});

test('reload restores isolated durable drafts and discard removes storage', () => {
  const store = storage(), first = harness(store);
  first.ui.iedit = first.edit(); first.ui.iedit.title = 'draft a'; first.c.keepItemDraft();
  first.ui.store = 'b'; first.ui.iedit.title = 'draft b'; first.c.keepItemDraft();
  first.ui.store = 'a'; first.ui.iedit.item = '2'; first.ui.iedit.title = 'draft 2'; first.c.keepItemDraft();
  const reloaded = harness(store);
  assert.equal(reloaded.c.restoreItemDraft('a', '1').title, 'draft a');
  assert.equal(reloaded.c.restoreItemDraft('b', '1').title, 'draft b');
  assert.equal(reloaded.c.restoreItemDraft('a', '2').title, 'draft 2');
  reloaded.ui.iedit = reloaded.c.restoreItemDraft('a', '1'); reloaded.c.endEdit();
  assert.equal(harness(store).c.restoreItemDraft('a', '1'), null);
  assert.equal(harness(store).c.restoreItemDraft('b', '1').title, 'draft b');
});

test('storage exceptions and corrupt records fall back safely', () => {
  const unavailable = { getItem() { throw Error('blocked'); }, setItem() { throw Error('full'); }, removeItem() { throw Error('blocked'); } };
  const h = harness(unavailable); h.ui.iedit = h.edit(); h.ui.iedit.body = 'safe';
  h.c.keepItemDraft(); assert.equal(h.c.restoreItemDraft('a', '1').body, 'safe');
  h.c.endEdit(); assert.equal(h.c.restoreItemDraft('a', '1'), null);
  const store = storage(), corrupt = harness(store);
  store.setItem(corrupt.c.draftKey('a', '1'), '{bad');
  assert.equal(corrupt.c.restoreItemDraft('a', '1'), null);
  store.setItem(corrupt.c.draftKey('a', '2'), JSON.stringify({ item: '2' }));
  assert.equal(corrupt.c.restoreItemDraft('a', '2'), null);
});

test('window replacement and closing restore the correct draft', () => {
  const { c, ui, edit } = harness();
  const views = { a: { items: { '1': {}, '2': {} } }, b: { items: { '1': {} } } };
  Object.assign(c, { AD: { stores: () => ['a', 'b'], view: s => views[s] }, document: { activeElement: null, querySelector: () => null },
    returnSel: () => null, MN: null, syncNow() {}, history: { pushState() {} }, applyRoute() {},
    $: id => id === 'tkwin' ? { firstElementChild: { focus() {} } } : null });
  vm.runInContext(section('  function openItemWindow(', '  // openNotify('), c);
  vm.runInContext(section('  function closeItemWindow()', '  // The store page:'), c);
  ui.win = true; ui.item = '1'; ui.iedit = edit(); ui.iedit.title = 'unsaved';
  c.openItemWindow('a', '2'); assert.equal(ui.iedit, null);
  c.openItemWindow('b', '1'); assert.equal(ui.iedit, null);
  c.openItemWindow('a', '1'); assert.equal(ui.iedit.title, 'unsaved'); assert.ok(ui.iedit.restored);
  c.closeItemWindow(); c.openItemWindow('a', '1'); assert.equal(ui.iedit.title, 'unsaved');
  assert.match(source, /Draft restored/); assert.match(source, /k === 'iedit-discard'/);
});

test('unchanged edits and explicit discard do not leave drafts', () => {
  const { c, ui, edit } = harness();
  ui.iedit = edit(); c.keepItemDraft(); assert.equal(c.restoreItemDraft('a', '1'), null);
  ui.iedit.title = 'new'; c.keepItemDraft(); c.endEdit();
  assert.equal(c.restoreItemDraft('a', '1'), null);
});

test('only applied save clears draft; a later edit survives an in-flight save', () => {
  const { c, ui, elements, edit } = harness();
  ui.iedit = edit(); ui.iedit.body = elements['iw-ebody'].value = 'changed';
  c.saveEdit(); assert.equal(c.events[0][1].base[0], 'rev');
  c.finish([{ status: 'refused' }]); assert.ok(c.restoreItemDraft('a', '1'));
  c.saveEdit(); ui.iedit.body = 'newer'; c.keepItemDraft(); c.finish([{ status: 'applied' }]);
  assert.equal(c.restoreItemDraft('a', '1').body, 'newer');
  elements['iw-ebody'].value = 'newer'; c.saveEdit(); c.finish([{ status: 'applied' }]);
  assert.equal(c.restoreItemDraft('a', '1'), null); assert.equal(ui.iedit, null);
});

test('real commit passes applied results to save and retains drafts on dispatch failure', () => {
  const store = storage(), h = harness(store), { c, ui, elements } = h;
  c.AD = { dispatch: (id, events, done) => { c.done = done; } };
  c.esc = x => x; c.wrote = () => {};
  vm.runInContext(section('  function commit(events, then, prep, fail)', '  // A written batch:'), c);
  ui.iedit = h.edit(); ui.iedit.body = elements['iw-ebody'].value = 'changed';
  c.saveEdit(); c.done({ ok: false, error: 'refused' });
  assert.equal(harness(store).c.restoreItemDraft('a', '1').body, 'changed');
  c.saveEdit(); c.done({ ok: true, results: [{ id: 'event', alias: '1', type: 'item.edit', status: 'flagged', reason: 'conflict' }] });
  assert.ok(harness(store).c.restoreItemDraft('a', '1'));
  c.saveEdit(); c.done({ ok: true, results: [{ id: 'event', alias: '1', type: 'item.edit', status: 'applied', reason: '' }] });
  assert.equal(harness(store).c.restoreItemDraft('a', '1'), null);
});
