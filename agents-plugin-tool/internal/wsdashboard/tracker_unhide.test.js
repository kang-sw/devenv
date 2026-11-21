'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./testdata/asset-source.cjs');

const source = readAssetSource('wsdashboard', 'tracker.js');

function section(start, end) {
  const a = source.indexOf(start);
  const b = end ? source.indexOf(end, a) : source.length;
  assert.ok(a >= 0 && b > a, `missing section ${start} .. ${end}`);
  return source.slice(a, b);
}

function harness() {
  const dispatched = [];
  const toasts = [];
  const c = {
    navigator: { platform: 'MacIntel' },
    document: {
      getElementById: () => null,
      activeElement: null,
      body: { children: [], classList: { toggle() {}, add() {}, remove() {} } }
    },
    wsT: (k, ...args) => (args.length ? `${k}:${args.join('|')}` : k),
    wsTn: (k, ...args) => (args.length ? `${k}:${args.join('|')}` : k),
    CSS: { escape: s => String(s) },
    setTimeout: () => 0,
    clearTimeout: () => {},
    history: { pushState() {}, replaceState() {} },
    location: { hash: '', pathname: '/', search: '' },
    window: {}
  };
  vm.createContext(c);
  // Load pure helpers from tracker.js through tracker-window.js plus commit/quick/menuHtml/openItemWindow.
  vm.runInContext(section('  function alias(id)', '  // Maps each org id'), c);
  vm.runInContext(section('// Tracker view state, shared markup, events, and seen marks.', '  // ------------------------------------------------------- viewer / badge state'), c);
  vm.runInContext(section('// Tracker item row rendering and edit controls.', '  // ------------------------------------------------------------------ cards'), c);
  vm.runInContext(section('  // ------------------------------------------------------------- Items panel', '  // Does the given scope'), c);
  vm.runInContext(section('  function kindFilter(it)', '  function cardActivity()'), c);
  vm.runInContext(section('  function commentHtml(n, v, it)', '  // The edit mode (ui.iedit)'), c);
  vm.runInContext(section('  function headActs(it)', '  // The item window\'s id button'), c);
  vm.runInContext(section('  function mi(act, label, data, o)', '  // The handoff step\'s people'), c);
  vm.runInContext(section('  function commit(events, then, prep, fail)', '  function submit(form)'), c);
  vm.runInContext(section('  function quick(k, a)', '  // ------------------------------------------------ admin org-chart editor modal'), c);
  vm.runInContext(section('  function openItemWindow(storeId, itemId, opts)', '  // uiCommand performs'), c);

  c.VS = {
    now: '2026-11-21T12:00:00Z',
    viewer: { member: 'maya', stream: 'maya.pc', isMember: true, admin: false, reach: {} },
    members: {
      maya: { id: 'maya', name: 'Maya', avatar: 'MA', leads: [], profile: { org: '', family: '', given: 'Maya' } },
      lena: { id: 'lena', name: 'Lena', avatar: 'LE', leads: [], profile: { org: '', family: '', given: 'Lena' } },
      ada: { id: 'ada', name: 'Ada', avatar: 'AD', admin: true, leads: [], profile: { org: '', family: '', given: 'Ada' } }
    },
    priority: { names: ['Low', 'Normal', 'High'], default: 1 },
    attachments: { max_bytes: 1000000 },
    labels: [],
    orgs: [],
    items: {},
    assignments: {},
    requests: {},
    rules: {},
    comments: {},
    events: [],
    notices: [],
    itemOrder: [],
    html: { items: {}, comments: {} },
    my: {
      badge: [], assignments: [], given: [], team: [], created: [],
      received: { pending: [], answered: [], sent: [] },
      today: { work: [], pending: [], confirms: [] },
      requests: { incoming: [], outgoing: [] },
      related: { cc: [], mentioned: [], closed: [] },
      ccNotices: []
    }
  };
  c.EV = {};
  c.AD = {
    stores: () => ['s'],
    view: id => (id === 's' ? c.VS : null),
    seen: () => ({ badge: {}, read: {} }),
    dispatch: (store, events, done) => {
      dispatched.push({ store, events });
      if (c._dispatchReply) done(c._dispatchReply(store, events));
      else done({ ok: true, results: events.map((e, i) => ({ id: 'e' + i, alias: 'e' + i, type: e.type, status: 'applied', reason: '' })) });
    }
  };
  c.CAPS = { origin: false, attach: false };
  c.X = {};
  c.O = {};
  c.MN = null;
  c.ui.store = 's';
  c.toast = html => toasts.push(html);
  c.render = () => { c._renders = (c._renders || 0) + 1; };
  c.keepItemDraft = () => {};
  c.restoreItemDraft = () => null;
  c.returnSel = () => null;
  c.closeMenu = () => { c.MN = null; };
  c.closeDialog = () => {};
  c.syncNow = () => {};
  c.applyRoute = () => {};
  c.idCopyBtn = it => `<button data-id="${it.alias}"></button>`;
  c.itemRef = id => `<a data-item="${id}">${id}</a>`;
  c.mvMenuRoots = id => [id];
  c.mentionChips = () => '';
  c.attChips = () => '';
  c.refLinks = s => s;
  c.refLinksHtml = s => s;
  c.newBtn = () => '';
  c.newRowHtml = () => '';
  c.subRowHtml = () => '';
  c.editorHtml = () => '';
  c.labelPickHtml = () => '';
  c.openPicker = () => {};
  c.viewerOf = () => c.VS.viewer.stream;
  c.document.getElementById = id => (id === 'tkwin' ? { firstElementChild: { focus() {} } } : null);

  return { c, dispatched, toasts };
}

function makeItem(id, extra = {}) {
  return {
    id, alias: id, kind: 'task', title: 'Title ' + id, author: 'maya', ts: '2026-11-21T10:00:00Z',
    lastActivity: '2026-11-21T10:00:00Z', status: 'todo', hidden: false, canUnhide: false,
    assignments: [], pending: [], children: [], thread: [], backrefs: [], chain: [id],
    depth: 0, parent: null, labels: [], priority: 1, priorityLevel: 1, priorityName: 'Normal',
    concurrent: 0, project: '', due: null, start: null, rollup: { total: 0, counts: {}, blocked: [] },
    requests: [], refs: [], mentions: [], cc: [], ...extra
  };
}

test('canUnhide and item menu mirror hide location and honor backend canUnhide', () => {
  const { c } = harness();
  const vis = makeItem('i1', { author: 'maya', hidden: false, canUnhide: false });
  const hidOwn = makeItem('i2', { author: 'maya', hidden: true, canUnhide: true });
  const hidChildBlocked = makeItem('i3', { author: 'maya', hidden: true, canUnhide: false, parent: 'i2' });
  const hidRemoved = makeItem('i4', { author: 'lena', hidden: true, canUnhide: true });
  c.VS.items = { i1: vis, i2: hidOwn, i3: hidChildBlocked, i4: hidRemoved };

  assert.equal(c.canHide(vis), true);
  assert.equal(c.canUnhide(vis), false);
  assert.equal(c.canHide(hidOwn), false);
  assert.equal(c.canUnhide(hidOwn), true);
  assert.equal(c.canUnhide(hidChildBlocked), false);
  assert.equal(c.canUnhide(makeItem('i5', { hidden: true, canUnhide: undefined })), false);

  c.MN = { kind: 'item', item: 'i2', step: 'main' };
  const ownMenu = c.menuHtml();
  assert.ok(ownMenu.includes('data-act="iunhide"'), ownMenu);
  assert.ok(ownMenu.includes('js.tk.act.unhide'), ownMenu);
  assert.ok(!ownMenu.includes('js.tk.act.unhide.admin'), ownMenu);
  assert.ok(!ownMenu.includes('data-act="ihide"'), ownMenu);

  c.MN = { kind: 'item', item: 'i3', step: 'main' };
  assert.ok(!c.menuHtml().includes('data-act="iunhide"'));

  c.VS.viewer = { member: 'ada', stream: 'ada.pc', isMember: true, admin: true, reach: {} };
  c.VS.members.lena.removed = 'ev-rm';
  c.MN = { kind: 'item', item: 'i4', step: 'main' };
  const adminMenu = c.menuHtml();
  assert.ok(adminMenu.includes('data-act="iunhide"'), adminMenu);
  assert.ok(adminMenu.includes('js.tk.act.unhide.admin'), adminMenu);
});

test('show hidden toggle defaults off and reveals dimmed hidden items in place across mine and all', () => {
  const { c } = harness();
  const vis = makeItem('i1', { title: 'Visible parent', children: ['i2'], assignments: ['a1'] });
  const hidChild = makeItem('i2', { title: 'Hidden child', parent: 'i1', hidden: true, canUnhide: true, assignments: ['a2'] });
  const hidRoot = makeItem('i3', { title: 'Hidden root', hidden: true, canUnhide: true, children: ['i4'] });
  const hidGrand = makeItem('i4', { title: 'Hidden grand', parent: 'i3', hidden: true, canUnhide: false });
  c.VS.items = { i1: vis, i2: hidChild, i3: hidRoot, i4: hidGrand };
  c.VS.itemOrder = ['i1', 'i2', 'i3', 'i4'];
  c.VS.assignments = {
    a1: { id: 'a1', item: 'i1', by: 'maya', to: 'maya', state: 'live', status: 'todo', fallback: true },
    a2: { id: 'a2', item: 'i2', by: 'maya', to: 'maya', state: 'live', status: 'todo', fallback: true }
  };
  c.VS.my.assignments = ['a1'];

  assert.equal(c.showHidden(), false);
  const offMine = c.listMine();
  assert.ok(offMine.includes('Visible parent'));
  assert.ok(!offMine.includes('Hidden child'));
  assert.ok(!offMine.includes('Hidden root'));

  const offAll = c.listAll();
  assert.ok(offAll.includes('Visible parent'));
  assert.ok(!offAll.includes('Hidden child'));
  assert.ok(!offAll.includes('Hidden root'));

  const offSub = c.subtasksHtml(vis);
  assert.ok(!offSub.includes('Hidden child'));

  c.panelState().showHidden = true;
  assert.equal(c.showHidden(), true);

  const onMine = c.listMine();
  assert.ok(onMine.includes('Hidden child') && onMine.includes('tk-hidden'), onMine);
  assert.ok(onMine.includes('Hidden root'), onMine);
  assert.ok(onMine.includes('Hidden grand'), onMine);

  const onAll = c.listAll();
  assert.ok(onAll.includes('Hidden child') && onAll.includes('js.tk.chip.hidden'), onAll);
  assert.ok(onAll.includes('Hidden root') && onAll.includes('Hidden grand'), onAll);

  const onSub = c.subtasksHtml(vis);
  assert.ok(onSub.includes('Hidden child') && onSub.includes('tk-hidden'), onSub);
});

test('opening a hidden item while toggle is off returns false and shows the not-visible toast', () => {
  const { c, toasts } = harness();
  c.VS.items.h1 = makeItem('h1', { hidden: true, canUnhide: true });
  c.VS.itemOrder = ['h1'];

  assert.equal(c.openItemWindow('s', 'h1'), false);
  assert.equal(c.ui.win, false);
  c.showItem('h1');
  assert.equal(c.ui.win, false);
  assert.deepEqual(toasts, ['js.tk.toast.hiddenitem']);

  c.panelState().showHidden = true;
  assert.equal(c.openItemWindow('s', 'h1'), true);
  assert.equal(c.ui.win, true);
  assert.equal(c.ui.item, 'h1');
});

test('hidden comments stay tombstones when toggle is off and render dimmed with unhide when on', () => {
  const { c } = harness();
  const it = makeItem('i1', { thread: ['c1', 'c2'] });
  c.VS.items.i1 = it;
  c.VS.comments = {
    c1: { id: 'c1', item: 'i1', author: 'maya', body: 'My hidden comment', hidden: 'ev-hide-1', canUnhide: true, mentions: [], attachments: [] },
    c2: { id: 'c2', item: 'i1', author: 'lena', body: 'Lena hidden comment', hidden: 'ev-hide-2', mentions: [], attachments: [] }
  };
  const n1 = { id: 'c1', alias: 'c1', type: 'comment.add', member: 'maya', ts: '2026-11-21T10:01:00Z', status: 'applied', origin: 'human', markers: [], payload: { item: 'i1', body: 'My hidden comment' } };
  const n2 = { id: 'c2', alias: 'c2', type: 'comment.add', member: 'lena', ts: '2026-11-21T10:02:00Z', status: 'applied', origin: 'human', markers: [], payload: { item: 'i1', body: 'Lena hidden comment' } };

  // Toggle off: tombstone as today, no unhide button.
  const off1 = c.commentHtml(n1, 'maya', it);
  assert.ok(off1.includes('class="hiddenc"'), off1);
  assert.ok(!off1.includes('tk-hidden'), off1);
  assert.ok(!off1.includes('data-act="cunhide"'), off1);

  // Toggle on: dimmed in place; unhide appears only where canUnhide is true.
  c.panelState().showHidden = true;
  const on1 = c.commentHtml(n1, 'maya', it);
  assert.ok(on1.includes('tk-hidden'), on1);
  assert.ok(on1.includes('My hidden comment'), on1);
  assert.ok(!on1.includes('class="hiddenc"'), on1);
  assert.ok(on1.includes('data-act="cunhide"') && on1.includes('data-cid="c1"'), on1);

  const on2 = c.commentHtml(n2, 'maya', it);
  assert.ok(on2.includes('tk-hidden'), on2);
  assert.ok(on2.includes('Lena hidden comment'), on2);
  assert.ok(!on2.includes('data-act="cunhide"'), on2);
});

test('quick unhide actions dispatch expected events and 409 refusal surfaces refusal.reason', () => {
  const { c, dispatched, toasts } = harness();
  const attrEl = map => ({ getAttribute: k => map[k] || null, hasAttribute: k => k in map });
  const copy = x => JSON.parse(JSON.stringify(x));

  c.quick('iunhide', attrEl({ 'data-item': 'maya.pc#2' }));
  assert.deepEqual(copy(dispatched[0]), {
    store: 's',
    events: [{ type: 'item.unhide', payload: { item: 'maya.pc#2' }, origin: 'human' }]
  });

  c.quick('cunhide', attrEl({ 'data-cid': 'maya.pc#5' }));
  assert.deepEqual(copy(dispatched[1]), {
    store: 's',
    events: [{ type: 'comment.unhide', payload: { comment: 'maya.pc#5' }, origin: 'human' }]
  });

  // 409 refusal with refusal.reason surfaces the reason via toast.
  c._dispatchReply = () => ({
    ok: false,
    error: 'event 0 (item.unhide) would be flagged: unhide the parent first',
    refusal: { index: 0, type: 'item.unhide', reason: 'unhide the parent first' }
  });
  c.quick('iunhide', attrEl({ 'data-item': 'maya.pc#3' }));
  assert.equal(toasts[toasts.length - 1], 'unhide the parent first');

  // Also works when adapter returns refusal.reason without error.
  c._dispatchReply = () => ({ ok: false, refusal: { index: 0, type: 'item.unhide', reason: 'unhide the parent first' } });
  c.quick('iunhide', attrEl({ 'data-item': 'maya.pc#3' }));
  assert.equal(toasts[toasts.length - 1], 'unhide the parent first');
});

test('describe formats item.unhide and comment.unhide events', () => {
  const { c } = harness();
  assert.equal(c.describe({ type: 'item.unhide', payload: { item: 'i1' } }, true), 'js.tk.ev.unhide:');
  assert.equal(c.describe({ type: 'comment.unhide', payload: { comment: 'maya.pc#4' } }, true), 'js.tk.ev.cunhide:maya.pc-4');
  assert.equal(c.sysKindKey('comment.unhide'), 'js.tk.tr.k.comment');
  assert.equal(c.sysKindKey('item.unhide'), 'js.tk.tr.k.hide');
});
