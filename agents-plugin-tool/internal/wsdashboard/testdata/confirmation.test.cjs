const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');

const asset = name => readAssetSource('wsdashboard', name);
function block(file, start, end) {
  const source = asset(file), from = source.indexOf(start), to = source.indexOf(end, from + start.length);
  assert(from >= 0 && to > from, start);
  return source.slice(from, to);
}

function page(globals = {}) {
  const window = new EventTarget(), document = {};
  function el(tag, cls, text) {
    return Object.assign(new EventTarget(), {
      tag, className: cls, textContent: text, children: [], attrs: {}, isConnected: true,
      appendChild(child) { this.children.push(child); child.parentNode = this; return child; },
      setAttribute(k, v) { this.attrs[k] = v; },
      remove() {
        if (this.parentNode) this.parentNode.children = this.parentNode.children.filter(child => child !== this);
        this.isConnected = false;
      },
      focus() { document.activeElement = this; },
      click() { this.dispatchEvent(new Event('click')); },
      showModal() { this.open = true; },
      close() { this.open = false; setImmediate(() => this.dispatchEvent(new Event('close'))); }
    });
  }
  document.body = el('body');
  document.createElement = el;
  const context = vm.createContext({ window, document, mk: el, wsT: k => k, ...globals });
  vm.runInContext(asset('confirm.js'), context);
  return { window, document, context, el, confirm: window.wsConfirm,
    dialog: () => document.body.children.find(child => child.tag === 'dialog'),
    escape() {
      window.dispatchEvent(Object.assign(new Event('keydown', { cancelable: true }), { key: 'Escape' }));
    }
  };
}
const actions = dialog => dialog.children[1].children;
const settled = () => new Promise(resolve => setImmediate(resolve));

test('confirmation starts on Cancel, returns one decision, and owns Escape', async () => {
  const h = page(), opener = h.el('button');
  opener.focus();
  let shortcut = 0;
  h.window.addEventListener('keydown', () => shortcut++);
  const choice = h.confirm('<unsafe>', { accept: 'Leave' });
  const dialog = h.dialog(), [no, yes] = actions(dialog);
  assert.equal(dialog.children[0].textContent, '<unsafe>');
  assert.equal(dialog.attrs['aria-labelledby'], dialog.children[0].id);
  assert.equal(dialog.className, 'rdeldlg ws-confirm');
  assert.equal(h.document.activeElement, no);
  assert.equal(no.textContent, 'js.confirm.cancel');
  assert.equal(yes.textContent, 'Leave');
  assert.equal(await h.confirm('ignored'), false);
  h.escape();
  assert.equal(await choice, false);
  assert.equal(shortcut, 0);
  assert.equal(h.document.activeElement, opener);
  assert.equal(h.dialog(), undefined);

  const leave = h.confirm('leave', { opener, cancel: 'Stay' });
  const [stay, accept] = actions(h.dialog());
  assert.equal(stay.textContent, 'Stay');
  assert.equal(accept.textContent, 'js.confirm.accept');
  accept.click();
  assert.equal(await h.confirm('still closing'), false);
  assert.equal(await leave, true);

  for (const dismiss of [d => d.close(), d => d.dispatchEvent(new Event('cancel')), d => actions(d)[0].click()]) {
    const answer = h.confirm('leave', { opener });
    dismiss(h.dialog());
    assert.equal(await answer, false);
  }
  const gone = h.confirm('leave', { opener });
  opener.remove();
  h.escape();
  assert.equal(await gone, false);
  assert.notEqual(h.document.activeElement, opener);
});

test('terminal card and tab close a captured target only after acceptance', async () => {
  const posts = [], gone = [], removed = [], tabs = new Map();
  const h = page({
    hooks: { post: (route, body, done) => posts.push({ route, done }) },
    gone: card => gone.push(card), tabFor: id => tabs.get(id),
    removeTab: id => removed.push(id), toast() {}
  });
  const src = block('terminal.js', '  async function closing(', '  function maximized(') +
    block('terminal.js', '  async function closeTab(', '  function openTab(');
  vm.runInContext(src, h.context);
  const opener = h.el('button');
  for (const tab of [false, true]) {
    const term = { t: {}, data: { tid: 'a/b' }, el: h.el('div'), closeBtn: opener };
    tabs.set('a/b', term);
    const close = () => tab ? h.context.closeTab('a/b', opener) : h.context.closing(term, opener);

    let decision = close();
    assert.equal(h.document.activeElement, actions(h.dialog())[0]);
    actions(h.dialog())[0].click();
    await decision;
    assert.equal(posts.length, 0);
    decision = close();
    h.escape();
    await decision;
    assert.equal(posts.length, 0);

    decision = close();
    await close();
    actions(h.dialog())[1].click();
    await decision;
    assert.equal(posts.length, 1);
    assert.equal(posts[0].route, 'terminals/a%2Fb/close');
    await close();
    assert.equal(posts.length, 1);
    assert.equal(gone.length + removed.length, 0);
    posts.pop().done(true);
    assert.equal(tab ? removed.pop() : gone.pop(), tab ? 'a/b' : term);

    term.closing = term.t.closing = false;
    decision = close();
    if (tab) tabs.set('a/b', { t: {} });
    else term.data.tid = 'other';
    actions(h.dialog())[1].click();
    await decision;
    assert.equal(posts.length, 0);

    tabs.set('a/b', term);
    term.data.tid = 'a/b';
    term.t.exited = true;
    await close();
    assert.equal(h.dialog(), undefined);
    assert.equal(posts.pop().route, 'terminals/a%2Fb/close');
  }
});

test('card framework preserves the close veto while the hook is pending', async () => {
  const h = page();
  let answer, removed = 0;
  const button = h.el('button');
  const c = { closeBtn: button, k: { closing: (card, opener) => {
    assert.equal(card, c);
    assert.equal(opener, button);
    return new Promise(resolve => { answer = resolve; });
  } }, close: () => removed++ };
  h.context.c = c;
  vm.runInContext(block('cards.js', '    c.closeBtn.addEventListener("click"', '    c.wireGrip();'), h.context);
  button.click();
  assert.equal(removed, 0);
  answer(false);
  await settled();
  assert.equal(removed, 0);
  button.click();
  answer(true);
  await settled();
  assert.equal(removed, 1);
});

test('holder terminal stop revalidates its row and current page', async () => {
  const posts = [];
  const h = page({ rid: 'session', hdnote: null, hdLoad() {},
    post: (route, body, done) => posts.push({ route, done }) });
  vm.runInContext(block('dashboard-git.js', '  var hdTermClosing', '  function hdFail('), h.context);
  const row = h.context.hdTermRow({ id: 'holder', state: 'running' });
  const stop = row.children.at(-1).children.at(-1);
  stop.click();
  stop.click();
  h.escape();
  await settled();
  assert.equal(posts.length, 0);
  stop.click();
  h.context.rid = 'other';
  actions(h.dialog())[1].click();
  await settled();
  assert.equal(posts.length, 0);
  h.context.rid = 'session';
  stop.click();
  actions(h.dialog())[1].click();
  await settled();
  assert.equal(posts.length, 1);
  assert.equal(posts[0].route, 'terminals/holder/close');
  const refreshed = h.context.hdTermRow({ id: 'holder', state: 'running' });
  refreshed.children.at(-1).children.at(-1).click();
  assert.equal(h.dialog(), undefined);
  assert.equal(posts.length, 1);
});

test('label removal uses the confirmed store and label once', async () => {
  const writes = [], label = { id: 'L', name: 'Label' };
  const h = page({ ui: { store: 'A' }, VS: { viewer: { admin: true }, labels: [label] },
    commit: (events, done, prep, fail) => writes.push({ events, done, fail }) });
  const src = block('tracker-view-helpers.js', '  function labelOf(', '  // A removed label') +
    block('tracker-view-helpers.js', '  var labelRemoving', '  function labelChips(');
  vm.runInContext(src, h.context);
  const opener = h.el('button'), remove = () => h.context.labelRemove('L', opener);

  let decision = remove();
  await remove();
  h.escape();
  await decision;
  assert.equal(writes.length, 0);
  decision = remove();
  label.name = 'Renamed';
  actions(h.dialog())[1].click();
  await decision;
  assert.equal(writes.length, 0);
  decision = remove();
  h.context.ui.store = 'B';
  actions(h.dialog())[1].click();
  await decision;
  assert.equal(writes.length, 0);

  h.context.ui.store = 'A';
  decision = remove();
  actions(h.dialog())[1].click();
  await decision;
  await remove();
  assert.equal(writes.length, 1);
  assert.equal(JSON.stringify(writes[0].events), '[["roles.remove_label",{"label":"L"}]]');
  writes[0].fail();
  decision = remove();
  actions(h.dialog())[0].click();
  await decision;
  assert.equal(writes.length, 1);
});

test('org editor closure follows an accepted decision for that staged draft', async () => {
  const ed = { changes: ['old'] };
  let editor, continued = 0;
  const h = page({ ui: { store: 'A', orged: ed }, $: () => editor, hideTip() {},
    wsTn: (k, n) => k + n, orgEdCloseNeedsConfirm: e => e.changes.length > 0,
    orgEdCount: e => e.changes.length, orgEdChanges: e => e.changes });
  editor = h.el('dialog');
  editor.showModal();
  const close = block('tracker-forms.js', '  async function closeOrgEditor(', '  function orgEdRefuseHtml(');
  const roles = block('tracker-mount.js', "        else if (k === 'oe-roles')", "        else if (k === 'oe-org-new')");
  vm.runInContext(close + 'function roles(k, a) { if (false) {} ' + roles + ' }', h.context);
  h.context.openDialog = kind => { assert.equal(kind, 'admin'); continued++; };
  const opener = h.el('button');
  h.context.roles('oe-roles', opener);
  h.context.roles('oe-roles', opener);
  h.escape();
  await settled();
  assert.equal(h.context.ui.orged, ed);
  assert.equal(editor.open, true);
  assert.equal(continued, 0);

  h.context.roles('oe-roles', opener);
  ed.changes[0] = 'new';
  actions(h.dialog())[1].click();
  await settled();
  assert.equal(h.context.ui.orged, ed);
  assert.equal(editor.open, true);
  assert.equal(continued, 0);

  h.context.roles('oe-roles', opener);
  actions(h.dialog())[1].click();
  await settled();
  assert.equal(h.context.ui.orged, null);
  assert.equal(editor.open, false);
  assert.equal(continued, 1);

  h.context.ui.orged = ed;
  editor.showModal();
  const result = h.context.closeOrgEditor(true);
  assert.equal(editor.open, false);
  assert.equal(await result, true);
  assert.equal(h.dialog(), undefined);
});
