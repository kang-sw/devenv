// Production modules over the shared cardsDOM, with HTTP and textarea adapters.
El.prototype.contains = function (n) { for (; n; n = n.parentNode) if (n === this) return true; return false; };
El.prototype.removeEventListener = function (t, fn) { this.listeners[t] = (this.listeners[t] || []).filter(f => f !== fn); };
El.prototype.focus = function () { this.ownerDocument.activeElement = this; };
El.prototype.select = function () {};
El.prototype.scrollIntoView = function () {};
El.prototype.getClientRects = function () { for (var n = this; n; n = n.parentNode) if (n.hidden) return []; return [1]; };
El.prototype.setSelectionRange = function (a, b) { this.selectionStart = a; this.selectionEnd = b; };
El.prototype.setRangeText = function (s, a, b) { this.value = this.value.slice(0, a) + s + this.value.slice(b); this.selectionStart = this.selectionEnd = a + s.length; };
El.prototype.dispatchEvent = function (e) { this.fire(e.type, e); };
El.prototype.click = function () { this.fire('click', { target: this, preventDefault() {}, stopPropagation() {} }); };
var insert = El.prototype.insertBefore;
El.prototype.insertBefore = function (n, ref) {
  if (n.tagName === 'FRAGMENT') { n.children.forEach(k => this.insertBefore(k, ref)); return n; }
  return insert.call(this, n, ref);
};
Doc.prototype.createDocumentFragment = function () { return new El('fragment', this); };
Doc.prototype.createTextNode = function (s) { var n = new El('text', this); n.textContent = s; return n; };
Doc.prototype.removeEventListener = El.prototype.removeEventListener;
Doc.prototype.querySelector = function (s) { return this.documentElement.querySelector(s); };
async function settle() { for (var i = 0; i < 30; i++) await Promise.resolve(); }
function fire(n, type, extra) { var e = Object.assign({ target: n, preventDefault() {}, stopPropagation() {} }, extra); n.fire(type, e); }
function fixture(store) {
  var doc = new Doc(), posts = [], requests = [], reveals = [], copies = [];
  var win = { listeners: {}, innerWidth: 2400, innerHeight: 1000, AbortController, getSelection: () => null,
    addEventListener: El.prototype.addEventListener, removeEventListener: El.prototype.removeEventListener };
  function node(tag, cls) { var n = doc.createElement(tag); n.className = cls || ''; return n; }
  var board = doc.body.appendChild(node('div')), convo = doc.body.appendChild(node('div'));
  convo.id = 'convo'; board.id = 'r-board';
  var fpane = doc.body.appendChild(node('section', 'fpane')), chead = doc.body.appendChild(node('div'));
  fpane.id = 'files'; chead.appendChild(node('span', 'fsum'));
  var ctext = doc.body.appendChild(node('textarea')); ctext.id = 'ctext'; ctext.value = ''; ctext.setSelectionRange(0, 0);
  var inputs = 0, submits = 0; ctext.addEventListener('input', () => inputs++); doc.addEventListener('submit', () => submits++);
  doc.documentElement.setAttribute('data-sysopen', '');
  function load(k) { return store[k] || null; }
  function save(k, v) { if (v === null) delete store[k]; else store[k] = v; }
  function fetchFS(url) {
    requests.push(url); var u = new URL(url, 'http://fixture.invalid'), root = u.searchParams.get('root') || '/session', dir = u.searchParams.get('dir') || '';
    if (u.pathname.endsWith('/fs/worktrees')) return Promise.resolve({ ok: true, json: () => ({ roots: [{ path: '/session', branch: 'main', detached: false }, { path: '/other', branch: 'work', detached: false }] }) });
    return Promise.resolve({ ok: true, json: () => ({ root, abs: root + (dir ? '/' + dir : ''), name: root.split('/').pop(), session: root === '/session', sep: '/', entries: dir ? [{ name: 'child.go' }] : [{ name: 'src', dir: true }, { name: 'main.go' }, { name: 'space name.png' }] }) });
  }
  new Function('document', 'window', 'wsT', 'getComputedStyle', 'requestAnimationFrame', CARDS)(doc, win, wsT, () => ({ overflowY: 'visible' }), () => {});
  new Function('document', 'window', 'wsT', 'wsTn', 'fetch', FILES)(doc, win, wsT, wsT, fetchFS);
  new Function('document', 'window', MENTION)(doc, win);
  var owners = new Function('doc', 'window', 'root', 'live', 'fpane', 'chead', 'rid', 'post', 'toast', 'copyText', 'load', 'save', 'ctext',
    'var filesTab, filesSheet, layoutMode = "tabs";\n' + PAGE_FILES + '\nreturn { tab: filesTab, sheet: filesSheet };')(
      doc, win, doc.documentElement, true, fpane, chead, 'session', (...a) => posts.push(a), () => {}, p => copies.push(p), load, save, ctext);
  var W = win.WsCards;
  W.mount(board, { key: 'ws.dashboard.cards.session', load, save, toolbar: { files: owners.sheet }, reveal: () => reveals.push(owners.sheet.el.hidden) });
  return { doc, win, W, board, owners, ctext, store, posts, requests, reveals, copies, inputs: () => inputs, submits: () => submits };
}
function entries(c) { return c.body.querySelectorAll('.fentry'); }
function mode(c, i) { c.body.querySelectorAll('.fview-modes button')[i].click(); }
function selected(c) { return c.explorer.selection(); }
function draft(p, s) { p.ctext.value = s; p.ctext.setSelectionRange(s.length, s.length); }
function escape(p) { p.doc.fire = El.prototype.fire; p.doc.fire('keydown', { target: p.doc.body, key: 'Escape', preventDefault() {}, stopPropagation() {} }); }
function clickEntry(c, path) { entries(c).find(n => n.getAttribute('data-entry') === path).click(); }
function send(c) { c.body.querySelectorAll('.fattachment button')[0].click(); }
function copy(c) { c.body.querySelectorAll('.fattachment button')[1].click(); }
(async function () {
  var p = fixture({}); p.owners.tab.shown(); await settle();
  var tabPane = p.doc.getElementById('files');
  check('Files tab keeps compact breadcrumb and external root summary in sync', [tabPane.querySelector('.fcrumb-root .froot').textContent, p.doc.querySelector('.fsum .froot').textContent, tabPane.querySelector('.ftab-select .fn').textContent], ['session', 'session', 'session']);
  p.owners.tab.navigate('src'); await settle();
  p.doc.getElementById('files').querySelector('.fopen-card').click(); await settle();
  var a = p.W.list('explorer')[0];
  check('Files tab opens a registered explorer at active directory', [p.W.list('explorer').length, a.explorer.location()], [1, { root: '', dir: 'src' }]);
  a.explorer.newTab({ root: '/other', dir: '' }); await settle(); mode(a, 3); clickEntry(a, 'space name.png');
  a.body.querySelector('.fdirectory').scrollTop = 55; fire(a.body.querySelector('.fdirectory'), 'scroll');
  draft(p, 'Inspect'); send(a); copy(a);
  check('outside-root mention uses mention.js quoting and never sends', [p.ctext.value, p.posts.length, p.submits(), p.inputs(), p.copies], ['Inspect @`/other/space name.png` ', 0, 0, 1, ['/other/space name.png']]);
  check('gallery raw URL keeps the card root', a.body.querySelector('.fthumb').src, '/s/session/fs/raw?path=space%20name.png&root=%2Fother');
  var b = p.win.WsFiles.open({ root: '', dir: '' }); await settle(); mode(b, 2); clickEntry(b, 'main.go');
  check('explorer card root tabs retain folder names for session and alternate roots', [a.body.querySelectorAll('.ftab-select .fn').map(n => n.textContent), b.body.querySelector('.ftab-select .fn').textContent], [['src', 'other'], 'session']);
  check('explorer card footer exposes selected path, attach, and copy', [b.body.querySelector('.fselection b').textContent, b.body.querySelectorAll('.ffooter .fattachment button').map(n => n.textContent)], ['main.go', ['js.files.send', 'js.files.copy']]);
  check('five view icons use the embedded Lucide sprite and accessible names', b.body.querySelectorAll('.fview-modes button').map(n => [n.html.includes('#l-'), !!n.getAttribute('aria-label')]), Array(5).fill([true, true]));
  b.body.querySelectorAll('.fseg button')[1].click();
  check('content-only toggles are isolated between Files tab and explorer cards', [tabPane.querySelector('.fopts').hidden, a.body.querySelector('.fopts').hidden, b.body.querySelector('.fopts').hidden], [true, true, false]);
  b.body.querySelectorAll('.fseg button')[0].click();
  check('returning explorer card to name mode hides content-only toggles', b.body.querySelector('.fopts').hidden, true);
  draft(p, ''); send(b);
  check('session-root mention stays relative and does not mutate another owner', [p.ctext.value, selected(a).absolute, selected(b).path, p.owners.tab.location().dir], ['@main.go ', '/other/space name.png', 'main.go', 'src']);
  check('two cards have isolated ID-keyed records', [a.id !== b.id, a.explorerKey !== b.explorerKey, !!p.store[a.explorerKey], !!p.store[b.explorerKey]], [true, true, true, true]);

  // Reuse the maximize sheet without replacing either the card or its core.
  var sameCore = a.explorer, samePane = a.body.querySelector('.fpane');
  a.acts.querySelector('.bc-float').click();
  check('explorer-only float uses capped shared sheet and keeps state', [a.el.hasAttribute('data-max'), a.el.hasAttribute('data-float'), a.normalWidth, a.el.style.props['--sheet-inset'], a.explorer === sameCore, a.body.querySelector('.fpane') === samePane, selected(a).path], [true, true, 1100, '650px', true, true, 'space name.png']);
  a.maxBtn.click();
  check('float can enter existing fullscreen width and extend', [a.el.hasAttribute('data-float'), a.normalWidth, a.el.style.props['--sheet-inset']], [false, 1920, '240px']);
  a.widthBtn.click(); check('fullscreen extend uses viewport inset', a.el.style.props['--sheet-inset'], '16px');
  a.acts.querySelector('.bc-float').click();
  check('fullscreen can return to float without changing browser state', [a.normalWidth, a.explorer === sameCore, selected(a).path], [1100, true, 'space name.png']);
  escape(p); check('Escape restores card flow and clears float', [a.el.hasAttribute('data-max'), a.el.hasAttribute('data-float'), a.el.style.height, p.doc.querySelector('.bc-scrim').hidden], [false, false, '', true]);
  p.W.register('other', { open() {} }); var other = p.W.open('other', {}); p.W.float(other);
  check('other kinds cannot float', other.el.hasAttribute('data-max'), false); other.close();

  // The toolbar opens its own persistent sheet, never the left Files tab.
  // Split-layout docking is covered by wide-explorer and folder-sheet-dock.
  var toggle = p.W.toolbar().querySelector('.board-files');
  toggle.click(); await settle();
  check('toolbar sheet shares compact rows and footer without losing actions', [!!p.owners.sheet.el.querySelector('.fbar .ftabs'), !!p.owners.sheet.el.querySelector('.fsr .fseg'), p.owners.sheet.el.querySelector('.fopts').hidden, p.owners.sheet.el.querySelectorAll('.ffooter .fattachment button').length], [true, true, true, 2]);
  check('toolbar toggle keeps sheet behavior', [p.owners.sheet.el.hidden, p.owners.sheet.el.hasAttribute('data-max'), toggle.getAttribute('aria-expanded')], [false, true, 'true']);
  fire(p.owners.sheet.el.querySelector('.fentry'), 'dblclick'); await settle();
  // Wrap open only to observe order, leaving the actual registered implementation in use.
  var nativeOpen = p.win.WsFiles.open, beforeCreation = [];
  p.win.WsFiles.open = function (location) { beforeCreation.push(p.owners.sheet.el.hidden && !p.owners.sheet.el.hasAttribute('data-max')); return nativeOpen(location); };
  p.owners.sheet.el.querySelector('.fopen-card').click(); await settle();
  var sheetCard = p.W.list('explorer')[2];
  check('toolbar dismisses before card creation and reveal at active directory', [beforeCreation, p.reveals.slice(-1), sheetCard.explorer.location().dir, p.owners.tab.location().dir], [[true], [true], 'src', 'src']);
  p.win.WsFiles.open = nativeOpen;
  toggle.click(); await settle(); toggle.click();
  check('toolbar remains usable after card launch', p.owners.sheet.el.hidden, true);
  check('Files tab, sheet, and cards keep separate storage namespaces', [Object.keys(p.store).some(k => k === 'ws.dashboard.files.tabs.session'), Object.keys(p.store).some(k => k === 'ws.dashboard.toolbar.ws.dashboard.files.tabs.session'), a.explorerKey !== 'ws.dashboard.files.tabs.session'], [true, true, true]);

  var aID = a.id, bID = b.id, aKey = a.explorerKey, bKey = b.explorerKey;
  var saved = JSON.parse(JSON.stringify(p.store));
  var reload = fixture(saved); await settle();
  var ra = reload.W.list('explorer').find(c => c.id === aID), rb = reload.W.list('explorer').find(c => c.id === bID);
  check('board reload restores all explorer IDs and per-card tabs mode selection scroll', [reload.W.list('explorer').length, ra.explorer.location(), ra.body.querySelectorAll('.ftab-select').length, ra.body.querySelector('.fdirectory').getAttribute('data-view'), selected(ra).path, ra.body.querySelector('.fdirectory').scrollTop, rb.body.querySelector('.fdirectory').getAttribute('data-view'), selected(rb).path], [3, { root: '/other', dir: '' }, 2, 'gallery', 'space name.png', 55, 'icon', 'main.go']);
  ra.acts.querySelector('.bc-float').click(); ra.close();
  check('close destroys the core removes only its state and releases sheet', [aKey in saved, bKey in saved, reload.W.list('explorer').some(c => c.id === aID), reload.doc.querySelector('.bc-scrim').hidden], [false, true, false, true]);
  var reload2 = fixture(JSON.parse(JSON.stringify(saved))); await settle();
  check('closed card cannot resurrect on reload', reload2.W.list('explorer').some(c => c.id === aID), false);
  var loadingCard = reload2.win.WsFiles.open({ root: '/pending', dir: 'src' }), loadingKey = loadingCard.explorerKey;
  loadingCard.close(); await settle();
  check('late root response cannot recreate a closed card storage record', loadingKey in reload2.store, false);
  var rootsPage = fixture({}); rootsPage.owners.tab.shown(); await settle();
  var rootCard = rootsPage.win.WsFiles.open({ root: '', dir: '' }), peerCard = rootsPage.win.WsFiles.open({ root: '', dir: '' }); await settle();
  rootCard.explorer.newTab({ root: '', dir: '' }); await settle();
  var untouchedPeer = rootsPage.store[peerCard.explorerKey], untouchedFiles = rootsPage.store['ws.dashboard.files.tabs.session'];
  rootCard.body.querySelector('.fchange-root').click(); await settle();
  rootCard.body.querySelector('.froot-list button').click(); await settle();
  check('allowed root changes only the active tab of its own card', [rootCard.explorer.location(), peerCard.explorer.location(), rootsPage.owners.tab.location(), rootsPage.store[peerCard.explorerKey] === untouchedPeer, rootsPage.store['ws.dashboard.files.tabs.session'] === untouchedFiles], [{ root: '/other', dir: '' }, { root: '', dir: '' }, { root: '', dir: '' }, true, true]);
  var rootSaved = JSON.parse(JSON.stringify(rootsPage.store)), rootReload = fixture(rootSaved); await settle();
  var restoredCard = rootReload.W.list('explorer').find(c => c.id === rootCard.id);
  check('allowed root and untouched sibling tab persist per card', [restoredCard.explorer.location(), restoredCard.body.querySelectorAll('.ftab-select').length], [{ root: '/other', dir: '' }, 2]);
  restoredCard.body.querySelectorAll('.ftab-select')[0].click();
  check('switching back retains the original tab root', restoredCard.explorer.location(), { root: '', dir: '' });
  for (var f of [p, reload, reload2, rootsPage, rootReload]) { f.W.list().slice().forEach(c => c.close()); f.owners.tab.destroy(); }
  process.stdout.write(bad.join('\n'));
})().catch(e => { console.error(e); process.exitCode = 1; });
