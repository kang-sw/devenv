// cardsDOM is supplied by files_test.go; production FILES is never reimplemented.
El.prototype.contains = function (n) { for (; n; n = n.parentNode) if (n === this) return true; return false; };
El.prototype.removeEventListener = function (t, fn) { this.listeners[t] = (this.listeners[t] || []).filter(f => f !== fn); };
El.prototype.focus = function () { this.ownerDocument.activeElement = this; };
El.prototype.select = function () {};
El.prototype.scrollIntoView = function () {};
El.prototype.getClientRects = function () { for (var n = this; n; n = n.parentNode) if (n.hidden) return []; return [1]; };
var insert = El.prototype.insertBefore;
El.prototype.insertBefore = function (n, ref) {
  if (n.tagName === 'FRAGMENT') { n.children.forEach(k => this.insertBefore(k, ref)); return n; }
  return insert.call(this, n, ref);
};
Doc.prototype.createDocumentFragment = function () { return new El('fragment', this); };
Doc.prototype.createTextNode = function (s) { var n = new El('text', this); n.textContent = s; return n; };
Doc.prototype.removeEventListener = El.prototype.removeEventListener;
Doc.prototype.fire = El.prototype.fire;
Doc.prototype.querySelector = function (s) { return this.documentElement.querySelector(s); };
El.prototype.click = function () { this.fire('click', { target: this, stopPropagation: function () {} }); };
var requests = [], pendingTimers = new Map(), nextTimer = 0;
function later(fn) { pendingTimers.set(++nextTimer, fn); return nextTimer; }
function cancel(id) { pendingTimers.delete(id); }
async function settle() { for (var i = 0; i < 12; i++) await Promise.resolve(); }
async function timers() { var fs = Array.from(pendingTimers.values()); pendingTimers.clear(); fs.forEach(f => f()); await settle(); }
function fetchControlled(url, options) {
  return new Promise((resolve, reject) => requests.push({ url: new URL(url, 'http://fixture.invalid'), options, resolve, reject }));
}
function take(rid, route, dir) {
  var r = requests.find(r => !r.taken && r.url.pathname === '/s/' + rid + '/fs/' + route && (dir === undefined || (r.url.searchParams.get('dir') || '') === dir));
  if (!r) throw new Error('missing request ' + rid + '/' + route + '/' + dir);
  r.taken = true; return r;
}
async function answer(r, body) { r.resolve({ ok: true, json: () => body }); await settle(); }
async function refuse(r) { r.resolve({ ok: false, status: 404, text: () => Promise.resolve('refused') }); await settle(); }
function listing(root, entries, session) { return { root, name: root.split('/').pop(), abs: root, sep: '/', session: !!session, entries: entries || [{ name: 'src', dir: true }, { name: 'main.go' }, { name: '.env' }, { name: 'ignored', dir: true, ign: true }] }; }
function event(n, type, extra) { var e = Object.assign({ target: n, preventDefault() { this.defaultPrevented = true; }, stopPropagation() {} }, extra); n.fire(type, e); return e; }
function key(n, key) { return event(n, 'keydown', { key }); }
function slash(n) { var e = { target: n, key: '/', preventDefault() { this.defaultPrevented = true; } }; doc.listeners.keydown.slice().forEach(f => f(e)); }
var doc = new Doc(), win = { WsCards: { icon: name => '<svg><use href="#l-' + name + '"/></svg>' }, AbortController, innerWidth: 1200, innerHeight: 900, listeners: {}, addEventListener: El.prototype.addEventListener, removeEventListener: El.prototype.removeEventListener };
new Function('document', 'window', 'wsT', 'wsTn', 'fetch', 'AbortController', 'setTimeout', 'clearTimeout', FILES)(doc, win, (k, n) => k === 'js.files.selected.count' ? n + ' selected' : k, (k, n) => k === 'js.files.count' ? n + (n === 1 ? ' item' : ' items') : k, fetchControlled, AbortController, later, cancel);
function explorer(rid, local = true, options = {}) {
  var el = doc.body.appendChild(new El('section', doc)); el.className = 'fpane';
  var store = options.store || {}, previews = [];
  win.WsPreview = { state: () => '', open: (path, o) => { var record = {}; if (o.root) record.root = o.root; if (o.line) record.line = o.line; if (o.keep) record.keep = true; previews.push([path, record]); } };
  var instance = win.WsFiles.create(el, Object.assign({ rid, local, load: k => store[k], save: (k, v) => store[k] = v }, options));
  return { el, instance, store, previews, q: el.querySelector('.fq'), tree: el.querySelector('.ftree'), hits: el.querySelector('.fhits'), at: s => el.querySelector(s), all: s => el.querySelectorAll(s) };
}
function rootRequest(x, path) { if (x.at('.froot-pop').hidden) x.at('.fchange-root').click(); x.at('.froot-pop .fpin').value = path; x.at('.fgo .pr').click(); }
async function rootFilterChecks() {
  var x = explorer('root-filter'), peer = explorer('root-filter-peer');
  x.instance.shown(); peer.instance.shown();
  await answer(take('root-filter', 'list'), listing('/project', undefined, true));
  await answer(take('root-filter-peer', 'list'), listing('/peer', undefined, true));
  var offered = { roots: [
    { path: '/project', branch: 'main', pooled: true },
    { path: '/pool/ab-build', branch: 'impl/a-very-long-branch/topic', pooled: true },
    { path: '/other-a', branch: 'xaxb' },
    { path: '/ab-first', branch: 'other' },
    { path: '/nonpool-detached', detached: true },
    { path: '/cab', branch: 'topic' },
    { path: '/ab-second', branch: 'other' },
    { path: '/pool/beta', branch: 'lab', pooled: true },
    { path: '/pool/detached', detached: true, pooled: true }
  ] };
  x.at('.fchange-root').click(); await answer(take('root-filter', 'worktrees'), offered);
  peer.at('.fchange-root').click(); await answer(take('root-filter-peer', 'worktrees'), { roots: [{ path: '/peer' }, { path: '/untouched', branch: 'keep' }] });
  var pop = x.at('.froot-pop'), filter = x.at('.froot-filter');
  function paths() { return x.all('.froot-list button').map(n => n.title); }
  function search(q) { filter.value = q; event(filter, 'input'); }
  function press(k, target = filter) { return event(pop, 'keydown', { key: k, target }); }
  check('current pooled root is pinned with a check above filter and scroll list', [x.at('.froot-current button').title, x.at('.froot-check').textContent, pop.children.indexOf(x.at('.froot-current')) < pop.children.indexOf(filter), pop.children.indexOf(filter) < pop.children.indexOf(x.at('.froot-list'))], ['/project', '✓', true, true]);
  check('only pool membership groups rows, preserving base order including detached', paths(), ['/other-a', '/ab-first', '/nonpool-detached', '/cab', '/ab-second', '/pool/ab-build', '/pool/beta', '/pool/detached']);
  var sep = x.at('.froot-separator');
  check('exactly one separator separates non-pooled from pooled', [x.all('.froot-separator').length, sep.getAttribute('role'), sep.previousElementSibling.title, sep.nextElementSibling.title], [1, 'separator', '/ab-second', '/pool/ab-build']);
  check('branch markup isolates LTR text inside the front-trimmed branch column', [x.at('.froot-list .froot-branch bdi').tagName, x.all('.froot-branch bdi').map(n => n.textContent).includes('impl/a-very-long-branch/topic')], ['BDI', true]);
  search('AB');
  check('fuzzy matches name and branch, ranks prefix contiguous before scattered with stable ties and non-pooled first', paths(), ['/ab-first', '/ab-second', '/cab', '/other-a', '/pool/ab-build', '/pool/beta']);
  check('filter never removes the pinned current row', x.at('.froot-current button').title, '/project');
  search('tpc');
  check('subsequence branch filtering ranks earlier matches first inside each group', paths(), ['/cab', '/pool/ab-build']);
  search('npdt');
  check('subsequence folder filtering works without a matching branch', paths(), ['/nonpool-detached']);
  check('one remaining group has no orphan separator', x.all('.froot-separator').length, 0);
  search('no-such-worktree');
  check('empty filter results retain pinned row and manual path input', [paths(), x.all('.froot-separator').length, x.at('.froot-pop .fpin').value], [[], 0, '/project']);
  check('Enter with no matches does not reselect current', [press('Enter').defaultPrevented, pop.hidden], [true, false]);
  search('ab');
  press('ArrowUp'); check('ArrowUp from filter reaches last visible row', doc.activeElement.title, '/pool/beta');
  press('ArrowDown', doc.activeElement); check('ArrowDown wraps to pinned row', doc.activeElement.title, '/project');
  press('ArrowDown', doc.activeElement); check('ArrowDown reaches first ranked row', doc.activeElement.title, '/ab-first');
  press('Enter', doc.activeElement); var picked = take('root-filter', 'list');
  check('keyboard Enter uses existing guarded root navigation', picked.url.searchParams.get('root'), '/ab-first');
  await answer(picked, listing('/ab-first'));
  check('filter and root selection stay isolated from another card', [peer.at('.froot-filter').value, peer.at('.froot-pop .fpin').value, peer.all('.froot-row').map(n => n.title), peer.instance.location().root], ['', '/peer', ['/peer', '/untouched'], '']);
  x.at('.fchange-root').click(); await answer(take('root-filter', 'worktrees'), offered);
  check('reopen clears filter and focuses it', [filter.value, doc.activeElement === filter], ['', true]);
  search('tpc'); press('Enter'); var filteredPick = take('root-filter', 'list');
  check('Enter directly from filter picks first ranked match rather than pinned root', filteredPick.url.searchParams.get('root'), '/cab');
  await answer(filteredPick, listing('/cab'));
  x.at('.fchange-root').click(); await answer(take('root-filter', 'worktrees'), offered);
  press('Escape'); check('Escape closes and restores root-switcher focus', [pop.hidden, doc.activeElement === x.at('.fchange-root')], [true, true]);
  x.at('.fchange-root').click(); await answer(take('root-filter', 'worktrees'), offered);
  x.at('.froot-pop .fpin').value = '/manual'; key(x.at('.froot-pop .fpin'), 'Enter');
  await refuse(take('root-filter', 'list'));
  check('manual path Enter still validates and displays refusal per card', [x.at('.ferr').textContent, pop.hidden, peer.at('.ferr').textContent], ['js.files.root.refused', false, '']);
  key(x.at('.froot-pop .fpin'), 'Enter'); await answer(take('root-filter', 'list'), listing('/manual'));
  check('manual path Enter still accepts an arbitrary folder', x.instance.location().root, '/manual');
  x.instance.destroy(); peer.instance.destroy();
}
async function sidebarToneChecks() {
  var x = explorer('sidebar-tone'); x.instance.shown();
  check('folder header is present while loading without a fabricated item count', [x.at('.ftree-head').textContent, x.all('.fcount').length], ['js.files.folders', 0]);
  var root = listing('/sidebar-tone', [{ name: 'src', dir: true }, { name: 'docs', dir: true }, { name: 'README.md' }, { name: '.env' }, { name: 'ignored', dir: true, ign: true }], true);
  await answer(take('sidebar-tone', 'list', ''), root);
  check('localized count follows the last row and counts only visible entries', [x.at('.fcount').textContent, x.at('.fdirectory').children.slice(-1)[0] === x.at('.fcount'), x.at('.fcount').getAttribute('role'), x.all('.ftree-head').length], ['4 items', true, null, 1]);
  x.all('.fhd button')[2].click();
  var dot = x.all('.fpop input')[1], ign = x.all('.fpop input')[0];
  dot.checked = true; event(dot, 'change');
  check('count tracks hidden-dot filtering', [x.at('.fcount').textContent, x.all('.fentry').length], ['3 items', 3]);
  ign.checked = true; event(ign, 'change');
  check('count tracks show-ignored filtering', [x.at('.fcount').textContent, x.all('.fentry').length], ['4 items', 4]);
  key(x.at('.fpop'), 'Escape');

  x.instance.navigate('src/pkg/deep');
  check('direct deep navigation first requests only its target', requests.filter(r => !r.taken && r.url.pathname === '/s/sidebar-tone/fs/list').map(r => r.url.searchParams.get('dir')), ['src/pkg/deep']);
  await answer(take('sidebar-tone', 'list', 'src/pkg/deep'), listing('/sidebar-tone/src/pkg/deep', [{ name: 'leaf.go' }]));
  check('sidebar requests only missing ancestor listings, not siblings or descendants', requests.filter(r => !r.taken && r.url.pathname === '/s/sidebar-tone/fs/list').map(r => r.url.searchParams.get('dir')).sort(), ['src', 'src/pkg']);
  await answer(take('sidebar-tone', 'list', 'src/pkg'), listing('/sidebar-tone/src/pkg', [{ name: 'deep', dir: true }, { name: 'sibling', dir: true }]));
  await answer(take('sidebar-tone', 'list', 'src'), listing('/sidebar-tone/src', [{ name: 'pkg', dir: true }, { name: 'other', dir: true }]));
  check('out-of-order ancestors reveal the current directory without opening its descendants', [x.at('[data-path="src"]').getAttribute('aria-expanded'), x.at('[data-path="src/pkg"]').getAttribute('aria-expanded'), x.at('[data-path="src/pkg/deep"]').getAttribute('aria-expanded'), x.at('[data-path="src/pkg/deep"]').getAttribute('aria-current'), x.at('.fcount').textContent, x.all('.ftree-head').length], ['true', 'true', 'false', 'location', '1 item', 1]);
  check('ancestor completion does not fetch unrelated folders', requests.filter(r => !r.taken && r.url.pathname === '/s/sidebar-tone/fs/list').length, 0);
  x.all('.fhd button')[0].click();
  check('explicit collapse still works', !!x.at('[data-path="src/pkg/deep"]'), false);
  x.instance.navigate('src/pkg/deep');
  check('cached navigation expands ancestors without refetching', [!!x.at('[data-path="src/pkg/deep"]'), requests.filter(r => !r.taken && r.url.pathname === '/s/sidebar-tone/fs/list').length], [true, 0]);
  x.all('.fview-modes button')[4].click();
  check('columns share cached ancestors and each count follows its entries', [x.all('.fcount').map(n => n.textContent), requests.filter(r => !r.taken && r.url.pathname === '/s/sidebar-tone/fs/list').length], [['4 items', '2 items', '2 items', '1 item'], 0]);
  x.all('.fview-modes button')[0].click();
  x.instance.navigate('docs');
  var empty = listing('/sidebar-tone/docs', []); empty.more = true;
  await answer(take('sidebar-tone', 'list', 'docs'), empty);
  check('empty and truncated notices remain alongside a zero count', [x.at('.fcount').textContent, x.all('.fdirectory .fnote').map(n => n.textContent)], ['0 items', ['js.files.empty', 'js.files.more']]);

  x.instance.newTab({ dir: 'docs/nested' });
  await answer(take('sidebar-tone', 'list', ''), root);
  await answer(take('sidebar-tone', 'list', 'docs/nested'), listing('/sidebar-tone/docs/nested', [{ name: 'kept.go' }]));
  var staleTabAncestor = take('sidebar-tone', 'list', 'docs');
  x.all('.ftab-select')[0].click();
  await answer(staleTabAncestor, listing('/sidebar-tone/docs', [{ name: 'late', dir: true }]));
  check('late ancestor listing cannot alter another tab', [x.instance.location().dir, !!x.at('[data-path="docs/late"]'), x.at('.fcount').textContent], ['docs', false, '0 items']);
  x.all('.ftab-select')[1].click();
  await answer(take('sidebar-tone', 'list', 'docs'), listing('/sidebar-tone/docs', [{ name: 'nested', dir: true }]));
  check('resumed tab retries its missing ancestor and reveals its restored directory', [x.at('[data-path="docs"]').getAttribute('aria-expanded'), x.at('[data-path="docs/nested"]').getAttribute('aria-current'), x.at('.fcount').textContent], ['true', 'location', '1 item']);

  x.instance.navigate('fresh/mid/end');
  await answer(take('sidebar-tone', 'list', 'fresh/mid/end'), listing('/sidebar-tone/fresh/mid/end', []));
  var staleParent = take('sidebar-tone', 'list', 'fresh/mid'), staleGrandparent = take('sidebar-tone', 'list', 'fresh');
  rootRequest(x, '/replacement');
  await answer(take('sidebar-tone', 'list', ''), listing('/replacement', [{ name: 'new.go' }]));
  await answer(staleParent, listing('/sidebar-tone/fresh/mid', [{ name: 'end', dir: true }]));
  await answer(staleGrandparent, listing('/sidebar-tone/fresh', [{ name: 'mid', dir: true }]));
  check('late ancestors cannot repaint a replacement root', [x.instance.location(), x.all('.fr[data-path]').length, x.at('.fcount').textContent, x.all('.ftree-head').length], [{ root: '/replacement', dir: '' }, 0, '1 item', 1]);
  x.instance.destroy();
}
async function pathBarChecks() {
  var notices = [], p = explorer('paths', true, { toast: s => notices.push(s) }), peer = explorer('path-peer');
  p.instance.shown(); peer.instance.shown();
  await answer(take('paths', 'list', ''), listing('/paths', undefined, true));
  await answer(take('path-peer', 'list', ''), listing('/peer', undefined, true));
  function shortcut(target, modifiers) {
    var e = Object.assign({ target, key: 'l', preventDefault() { this.defaultPrevented = true; } }, modifiers);
    doc.listeners.keydown.slice().forEach(f => f(e)); return !!e.defaultPrevented;
  }
  function enter(path) { p.at('.fcrumb-trail').click(); p.at('.fpath').value = path; key(p.at('.fpath'), 'Enter'); return take('paths', 'list', path); }
  async function atPath(request, dir) { await answer(request, Object.assign(listing('/paths/' + dir, [{ name: 'kept.go' }], true), { dir })); }
  check('path controls start with no history and up disabled at root', [p.at('.fback').disabled, p.at('.fforward').disabled, p.at('.fback').nextSibling === p.at('.fforward')], [true, true, true]);
  var barButtons = p.at('.fbar-ctl').children.filter(n => n.tagName === 'BUTTON');
  check('back and forward precede existing up', barButtons.map(n => n.title), ['js.files.back', 'js.files.forward', 'js.files.up']);
  check('up disabled at root', barButtons[2].disabled, true);
  p.at('.fcrumb-trail').click();
  check('trail empty space edits in place with a relative path', [p.at('.fcrumb-trail').hidden, p.at('.fpath').hidden, p.at('.fpath').value, doc.activeElement === p.at('.fpath')], [true, false, '', true]);
  p.at('.fpath').value = 'cancelled'; key(p.at('.fpath'), 'Escape');
  check('Escape restores breadcrumb focus without a request', [p.at('.fpath').hidden, p.at('.fcrumb-trail').hidden, doc.activeElement === p.at('.fcrumb-trail'), requests.filter(r => !r.taken && r.url.pathname === '/s/paths/fs/list').length], [true, false, true, 0]);
  p.at('.fcrumb-trail').click(); p.at('.fpath').value = 'blurred'; event(p.at('.fpath'), 'blur');
  check('blur discards editing without navigation', [p.at('.fpath').hidden, p.instance.location().dir], [true, '']);
  peer.el.setAttribute('data-max', '');
  check('Ctrl+L belongs to focused instance even with another maximized', shortcut(p.q, { ctrlKey: true }), true);
  check('shortcut edits only its own instance', [p.at('.fpath').hidden, peer.at('.fpath').hidden], [false, true]);
  key(p.at('.fpath'), 'Escape');
  check('Cmd+L belongs to peer instance', shortcut(peer.tree, { metaKey: true }), true);
  check('peer shortcut does not edit first instance', [p.at('.fpath').hidden, peer.at('.fpath').hidden], [true, false]);
  key(peer.at('.fpath'), 'Escape');
  check('browser shortcut untouched outside explorers, even with maximized pane', shortcut(doc.body, { ctrlKey: true }), false);
  peer.el.removeAttribute('data-max');
  check('modified shortcut untouched', shortcut(p.tree, { ctrlKey: true, shiftKey: true }), false);

  var relative = enter('src/./pkg/..');
  check('typed relative path goes unchanged to guarded route without changing roots', [relative.url.searchParams.get('dir'), relative.url.searchParams.has('root'), p.instance.location().dir], ['src/./pkg/..', false, '']);
  await atPath(relative, 'src');
  check('accepted directory uses server canonical relative path', [p.instance.location().dir, p.at('.fcrumb-trail .fcrumb').textContent, p.at('.fback').disabled], ['src', 'src', false]);
  p.at('.fcrumb-trail').click();
  check('editing starts at active relative directory', p.at('.fpath').value, 'src');
  event(p.at('.fpath'), 'blur');
  p.at('[data-entry="src/kept.go"]').click(); p.at('.fdirectory').scrollTop = 29; event(p.at('.fdirectory'), 'scroll');
  for (var path of ['../outside', '/outside', 'link', 'missing', 'kept.go', 'src']) {
    await refuse(enter(path));
    check('refusal preserves listing selection scroll root and history: ' + path, [p.instance.location(), !!p.at('[data-entry="src/kept.go"]'), p.instance.selection().path, p.at('.fdirectory').scrollTop, p.at('.fforward').disabled, notices.slice(-1)[0]], [{ root: '', dir: 'src' }, true, 'src/kept.go', 29, true, 'js.files.root.refused']);
  }
  var supersededPath = enter('obsolete');
  var absolute = enter('/paths/src/pkg'); await atPath(absolute, 'src/pkg');
  await atPath(supersededPath, 'obsolete');
  check('absolute in-root path uses canonical directory and ignores older typed answers', p.instance.location(), { root: '', dir: 'src/pkg' });
  event(p.at('.fcrumb-trail'), 'click', { target: p.all('.fcrumb-trail .fcrumb')[0] });
  check('bubbling crumb clicks do not open editor', p.at('.fpath').hidden, true);
  p.all('.fcrumb-trail .fcrumb')[0].click();
  check('breadcrumb navigation still works rather than starting editor', [p.instance.location().dir, p.at('.fpath').hidden], ['src', true]);
  p.at('.fback').click(); await atPath(take('paths', 'list', 'src/pkg'), 'src/pkg');
  p.at('.fforward').click(); await atPath(take('paths', 'list', 'src'), 'src');
  check('back and forward traverse rather than append history', [p.instance.location().dir, p.at('.fforward').disabled], ['src', true]);
  p.at('.fback').click(); await atPath(take('paths', 'list', 'src/pkg'), 'src/pkg');
  await atPath(enter('docs'), 'docs');
  check('new path truncates forward branch', [p.instance.location().dir, p.at('.fforward').disabled], ['docs', true]);
  p.at('.fback').click(); await atPath(take('paths', 'list', 'src/pkg'), 'src/pkg');
  barButtons[2].click(); check('up goes to canonical parent', p.instance.location().dir, 'src');
  barButtons[2].click(); check('up reaches root and disables itself', [p.instance.location().dir, barButtons[2].disabled], ['', true]);
  await atPath(enter('.'), '');
  p.at('.fback').click(); await atPath(take('paths', 'list', 'src'), 'src');
  check('same canonical path never appends duplicate history', p.instance.location().dir, 'src');
  p.at('.fback').click(); await atPath(take('paths', 'list', 'src/pkg'), 'src/pkg');
  p.at('.fback').click(); await refuse(take('paths', 'list', 'src'));
  check('refused history destination leaves cursor and listing intact', [p.instance.location().dir, p.at('.fback').disabled, p.at('.fforward').disabled], ['src/pkg', false, false]);
  p.at('.fback').click(); var staleHistory = take('paths', 'list', 'src');
  p.instance.navigate(''); await atPath(staleHistory, 'src');
  check('late history response cannot undo newer navigation', p.instance.location().dir, '');

  var staleTabPath = enter('late-tab');
  p.instance.newTab({ dir: 'docs' });
  await answer(take('paths', 'list', ''), listing('/paths', undefined, true));
  await atPath(take('paths', 'list', 'docs'), 'docs'); await atPath(staleTabPath, 'late-tab');
  check('new inner tab has independent initial history; late old path is ignored', [p.instance.location().dir, p.at('.fback').disabled, p.at('.fforward').disabled], ['docs', true, true]);
  p.instance.navigate('');
  p.all('.ftab-select')[0].click();
  check('first tab restores history without second tab entries', [p.instance.location().dir, p.at('.fback').disabled], ['', false]);
  p.at('.fback').click(); await atPath(take('paths', 'list', 'src/pkg'), 'src/pkg');
  p.all('.ftab-select')[1].click();
  p.at('.fback').click(); await atPath(take('paths', 'list', 'docs'), 'docs');
  check('second tab retains its own cursor and forward branch', [p.instance.location().dir, p.at('.fback').disabled, p.at('.fforward').disabled, peer.at('.fback').disabled], ['docs', true, false, true]);
  var staleRootPath = enter('late-root'); rootRequest(p, '/replacement');
  await answer(take('paths', 'list', ''), listing('/replacement'));
  await atPath(staleRootPath, 'late-root');
  check('accepted root change resets history and drops prior-root navigation', [p.instance.location(), p.at('.fback').disabled, p.at('.fforward').disabled], [{ root: '/replacement', dir: '' }, true, true]);
  var windowsPath = enter('C:\\work\\root\\folder');
  check('host-native absolute paths reach server unchanged', windowsPath.url.searchParams.get('dir'), 'C:\\work\\root\\folder');
  await atPath(windowsPath, 'folder');
  p.instance.hidden(); var saved = JSON.parse(JSON.stringify(p.store));
  check('history never enters persisted browser record', Object.keys(JSON.parse(saved['ws.dashboard.files.tabs.paths']).tabs[1]).some(k => /history|paths|index/.test(k)), false);
  var lateDestroyedPath = enter('late-destroy'); p.instance.destroy();
  await atPath(lateDestroyedPath, 'late-destroy');
  check('late typed response cannot rebuild destroyed explorer', p.el.children.length, 0);
  p = explorer('paths', true, { store: saved }); p.instance.shown();
  await answer(take('paths', 'list', ''), listing('/replacement'));
  await atPath(take('paths', 'list', 'folder'), 'folder');
  check('storage restores directory but starts fresh history', [p.instance.location().dir, p.at('.fback').disabled, p.at('.fforward').disabled], ['folder', true, true]);
  p.instance.destroy(); peer.instance.destroy();
}
async function multiSelectChecks() {
  var sent = [], copied = [], primary = [], actions = [];
  var p = explorer('multi', true, { send: path => sent.push(path), copy: text => copied.push(text), post: (...args) => actions.push(args), onSelection: s => primary.push(s.path) });
  var peer = explorer('multi-peer');
  p.instance.shown(); peer.instance.shown();
  var entries = [{ name: 'folder', dir: true }, { name: 'a.txt' }, { name: 'b.txt' }, { name: 'c.txt' }, { name: 'd.txt' }, { name: '.hidden' }, { name: 'ignored', ign: true }];
  await answer(take('multi', 'list'), listing('/multi', entries, true));
  await answer(take('multi-peer', 'list'), listing('/peer', entries, true));
  var selected = () => p.all('.fentry').filter(row => row.classList.contains('sel')).map(row => row.getAttribute('data-entry'));
  function click(path, modifiers = {}) { var row = p.at('[data-entry="' + path + '"]'); row.focus(); event(row, 'click', modifiers); }
  function shortcut(target, key, modifiers = {}) {
    target.focus(); var e = Object.assign({ target, key, preventDefault() { this.defaultPrevented = true; } }, modifiers);
    doc.listeners.keydown.slice().forEach(fn => fn(e)); return !!e.defaultPrevented;
  }
  click('a.txt'); check('plain click selects one', selected(), ['a.txt']);
  click('c.txt', { ctrlKey: true }); check('Ctrl click adds and retains primary compatibility', [selected(), p.instance.selection().path, primary.slice(-1)[0]], [['a.txt', 'c.txt'], 'c.txt', 'c.txt']);
  click('a.txt', { metaKey: true }); check('Cmd click toggles off while keeping last clicked primary', [selected(), p.instance.selection().path], [['c.txt'], 'a.txt']);
  click('d.txt', { shiftKey: true }); check('Shift range uses last toggled anchor', selected(), ['a.txt', 'b.txt', 'c.txt', 'd.txt']);
  click('b.txt', { shiftKey: true }); check('repeated Shift keeps anchor and replaces range', selected(), ['a.txt', 'b.txt']);
  click('d.txt', { metaKey: true }); click('c.txt', { shiftKey: true, ctrlKey: true });
  check('Shift Ctrl adds range to existing selection', selected(), ['a.txt', 'b.txt', 'c.txt', 'd.txt']);
  click('c.txt'); check('plain click replaces multi-selection', selected(), ['c.txt']);
  click('c.txt', { ctrlKey: true });
  check('toggling the final row off clears membership and disables batch actions', [selected(), p.all('.fattachment button').map(b => b.disabled)], [[], [true, true]]);
  check('Ctrl A outside explorers stays native', shortcut(doc.body, 'a', { ctrlKey: true }), false);
  var unfocused = { target: p.tree, key: 'a', ctrlKey: true, preventDefault() { this.defaultPrevented = true; } };
  doc.listeners.keydown.slice().forEach(fn => fn(unfocused));
  check('select all requires actual focus inside the instance', [!!unfocused.defaultPrevented, selected()], [false, []]);
  peer.el.setAttribute('data-max', '');
  check('Cmd A selects focused explorer even with peer maximized', shortcut(p.tree, 'a', { metaKey: true }), true);
  var visible = entries.filter(e => !e.ign).map(e => e.name);
  check('select all selects visible rows and only its own instance', [selected(), peer.all('.fentry.sel').length, p.at('.fdirectory').getAttribute('aria-multiselectable')], [visible, 0, 'true']);
  check('footer localizes the selection count', p.at('.fselection').textContent, '6 selected');
  var savedTab = JSON.parse(p.store['ws.dashboard.files.tabs.multi']).tabs[0];
  check('persisted selection stays a primary path without a selection set', [typeof savedTab.selection, Object.keys(savedTab).some(k => /sels|anchor|selections/.test(k))], ['string', false]);
  for (var tag of ['input', 'textarea', 'div']) {
    var input = p.el.appendChild(new El(tag, doc)); if (tag === 'div') input.isContentEditable = true;
    check(tag + ' keeps native select-all and Escape', [shortcut(input, 'a', { ctrlKey: true }), shortcut(input, 'Escape'), selected()], [false, false, visible]);
    p.el.removeChild(input);
  }
  check('peer focus does not select this explorer', shortcut(peer.tree, 'a', { ctrlKey: true }), true);
  check('Escape clears only focused instance', [shortcut(p.tree, 'Escape'), selected(), peer.all('.fentry.sel').length], [true, [], 6]);
  peer.el.removeAttribute('data-max');
  click('c.txt'); click('a.txt', { ctrlKey: true }); click('folder', { ctrlKey: true });
  var batch = ['folder', 'a.txt', 'c.txt'];
  win.WsTerm = { create: (...args) => actions.push(args) };
  function menu(path = 'a.txt') { event(p.at('[data-entry="' + path + '"]'), 'contextmenu', { clientX: 10, clientY: 20 }); return doc.querySelector('.fmenu'); }
  function menuItem(label) { return doc.querySelector('.fmenu').querySelectorAll('button').find(b => b.textContent === label); }
  menu();
  check('context click inside selection preserves the set', selected(), batch);
  check('single-only menu actions are present and disabled', ['js.menu.preview', 'js.preview.openext', 'js.files.menu.reveal', 'js.files.menu.terminal'].map(k => menuItem(k).disabled), [true, true, true, true]);
  menuItem('js.menu.preview').click(); check('disabled menu action cannot run', [actions.length, !!doc.querySelector('.fmenu')], [0, true]);
  check('open menu consumes Escape rather than clearing selection', shortcut(doc.querySelector('.fmenu'), 'Escape'), false);
  key(doc.querySelector('.fmenu'), 'Escape'); check('closing menu keeps selected rows', selected(), batch);
  menu(); menuItem('js.menu.copyrel').click();
  menu(); menuItem('js.menu.copyabs').click();
  menu(); menuItem('js.files.send').click();
  check('menu copies newline paths and sends each path in visible order', [copied, sent], [[batch.join('\n'), batch.map(path => '/multi/' + path).join('\n')], batch]);
  p.all('.fattachment button')[0].click(); p.all('.fattachment button')[1].click();
  check('footer batch actions keep host signatures', [sent.slice(3), copied.slice(-1)[0]], [batch, batch.join('\n')]);
  menu('d.txt'); check('context click outside set replaces selection', selected(), ['d.txt']); key(doc.querySelector('.fmenu'), 'Escape');
  p.all('.fhd button')[2].click();
  check('open popover prevents global Escape clearing selection', [shortcut(p.tree, 'Escape'), selected()], [false, ['d.txt']]);
  key(p.at('.fpop'), 'Escape');
  p.at('.fcrumb-trail').click(); check('path editor keeps native select-all', shortcut(p.at('.fpath'), 'a', { metaKey: true }), false); key(p.at('.fpath'), 'Escape');
  delete win.WsTerm;

  for (var i = 0; i < 4; i++) {
    p.all('.fview-modes button')[i].click(); click('a.txt'); click('c.txt', { ctrlKey: true });
    check('multi-select in directory view ' + i, selected(), ['a.txt', 'c.txt']);
    var box = p.at('.fdirectory'); box.getBoundingClientRect = () => ({ left: 10, top: 10, right: 300, bottom: 300 });
    p.all('.fentry').forEach((row, index) => row.getBoundingClientRect = () => ({ left: 20, right: 200, top: 20 + index * 30, bottom: 40 + index * 30 }));
    function pointer(type, target, x, y, modifiers = {}) { return event(target, type, Object.assign({ button: 0, pointerId: 1, clientX: x, clientY: y }, modifiers)); }
    pointer('pointerdown', box, 250, 45); pointer('pointermove', doc, 15, 108);
    check('marquee intersects rows in view ' + i, [selected(), !!doc.querySelector('.fmarquee')], [['a.txt', 'b.txt'], true]);
    pointer('pointerup', doc, 15, 108); event(box, 'click');
    check('marquee survives release click and removes rectangle ' + i, [selected(), !!doc.querySelector('.fmarquee')], [['a.txt', 'b.txt'], false]);
    pointer('pointerdown', box, 250, 135, { metaKey: true }); pointer('pointermove', doc, 15, 165); pointer('pointerup', doc, 15, 165); event(box, 'click');
    check('Cmd marquee adds to existing selection ' + i, selected(), ['a.txt', 'b.txt', 'd.txt']);
    var row = p.at('[data-entry="c.txt"]'); pointer('pointerdown', box, 40, 115, { target: row }); pointer('pointermove', doc, 100, 220); pointer('pointerup', doc, 100, 220);
    check('row drag never starts marquee ' + i, [selected(), !!doc.querySelector('.fmarquee')], [['a.txt', 'b.txt', 'd.txt'], false]);
    event(box, 'click'); check('plain whitespace click clears ' + i, selected(), []);
  }
  p.all('.fview-modes button')[4].click(); click('a.txt'); click('c.txt', { ctrlKey: true, shiftKey: true });
  check('column modifiers remain plain single selection', [selected(), p.at('.fdirectory').getAttribute('aria-multiselectable'), shortcut(p.tree, 'a', { ctrlKey: true })], [['c.txt'], 'false', false]);
  p.all('.fview-modes button')[0].click(); click('a.txt'); click('c.txt', { ctrlKey: true });
  p.instance.navigate('folder'); await answer(take('multi', 'list', 'folder'), listing('/multi/folder', [{ name: 'inside.txt' }], true));
  check('directory navigation clears full selection', [selected(), p.instance.selection().path], [[], 'folder']);
  p.instance.navigate(''); click('a.txt'); click('c.txt', { ctrlKey: true });
  p.instance.newTab({ dir: '' }); await answer(take('multi', 'list'), listing('/multi', entries, true));
  p.all('.ftab-select')[0].click(); check('live tabs retain the full set without persisting it', selected(), ['a.txt', 'c.txt']);
  var directory = p.at('.fdirectory');
  event(directory, 'pointerdown', { button: 0, pointerId: 7, clientX: 250, clientY: 45 });
  event(doc, 'pointermove', { pointerId: 7, clientX: 15, clientY: 108 });
  shortcut(directory, 'Escape'); check('Escape cancels a live marquee', [selected(), !!doc.querySelector('.fmarquee')], [[], false]);
  p.instance.destroy(); peer.instance.destroy();
  check('marquee document listeners are retired', ['pointermove', 'pointerup', 'pointercancel'].map(k => doc.listeners[k].length), [0, 0, 0]);
}
(async function () {
  await multiSelectChecks();
  // Candidate-root refusal must not cancel legitimate active-root work.
  var race = explorer('race'); race.instance.shown();
  var initial = take('race', 'list');
  rootRequest(race, '/invalid'); await refuse(take('race', 'list'));
  await answer(initial, listing('/race', undefined, true));
  check('refused candidate keeps pending initial root load', [race.at('.froot').textContent, !!race.at('[data-path="src"]'), race.at('.ferr').textContent], ['race', true, 'js.files.root.refused']);
  event(race.tree, 'click', { target: race.at('[data-path="src"]') });
  var expansion = take('race', 'list', 'src');
  rootRequest(race, '/invalid-again'); await refuse(take('race', 'list'));
  await answer(expansion, listing('/race/src', [{ name: 'kept.go' }]));
  check('refused candidate keeps pending expansion and current root', [race.at('.froot').textContent, !!race.at('[data-entry="src/kept.go"]')], ['race', true]);
  rootRequest(race, '/older-success'); var staleCandidate = take('race', 'list');
  rootRequest(race, '/newer-refusal'); await refuse(take('race', 'list'));
  await answer(staleCandidate, listing('/older-success'));
  check('latest refused attempt still supersedes an older candidate', race.at('.froot').textContent, 'race');
  rootRequest(race, '/destroyed-refusal'); var lateRefusal = take('race', 'list');
  race.instance.destroy(); await refuse(lateRefusal);
  check('late root refusal cannot touch a destroyed instance', race.el.children.length, 0);

  var a = explorer('alpha'), b = explorer('beta');
  a.instance.shown(); b.instance.shown();
  await answer(take('beta', 'list'), listing('/beta', undefined, true));
  await answer(take('alpha', 'list'), listing('/alpha', undefined, true));
  check('root tabs use folder names, never the return-to-session action label', [a.at('.ftab-select .fn').textContent, b.at('.ftab-select .fn').textContent], ['alpha', 'beta']);
  check('search mode chip is inside the field and name mode hides content options', [a.at('.fsr .fseg') === a.at('.fseg'), a.at('.fopts').hidden, b.at('.fopts').hidden], [true, true, true]);
  check('compact navigation and tools share the tab row', [!!a.at('.fbar .ftabs'), !!a.at('.fbar .flocation'), a.all('.fbar .fhd button').length], [true, true, 4]);
  check('out-of-order independent roots', [a.at('.froot').textContent, b.at('.froot').textContent], ['alpha', 'beta']);
  event(a.at('[data-path="src"]'), 'unused');
  event(a.tree, 'click', { target: a.at('[data-path="src"]') });
  await answer(take('alpha', 'list', 'src'), listing('/alpha/src', [{ name: 'a.go' }]));
  event(a.at('[data-entry="src/a.go"]'), 'click');
  check('expansion and selection stay local', [a.instance.selection().path, b.instance.selection().path, b.at('[data-path="src"]').getAttribute('aria-expanded')], ['src/a.go', '.', 'false']);
  a.all('.fhd button')[2].click();
  var ign = a.all('.fpop input')[0]; ign.checked = true; event(ign, 'change');
  check('view and storage isolated', [!!a.at('[data-path="ignored"]'), !!b.at('[data-path="ignored"]'), Object.keys(a.store).length > 0, Object.keys(b.store).some(k => k.includes('.view'))], [true, false, true, false]);
  key(a.at('.fpop'), 'Escape');
  check('popover Escape returns owner focus', doc.activeElement === a.all('.fhd button')[2], true);
  event(a.tree, 'contextmenu', { target: a.at('[data-path="src"]'), clientX: 20, clientY: 30 });
  check('row menu opens', !!doc.querySelector('.fmenu'), true);
  key(doc.querySelector('.fmenu'), 'Escape');
  check('menu Escape returns row focus', doc.activeElement === a.at('[data-path="src"]'), true);
  slash(a.tree); check('target pane owns slash', doc.activeElement === a.q, true);
  b.el.setAttribute('data-max', ''); slash(doc.body); check('active sheet owns slash', doc.activeElement === b.q, true); b.el.removeAttribute('data-max');
  a.tree.scrollTop = 73; event(a.tree, 'scroll'); b.tree.scrollTop = 19; event(b.tree, 'scroll');
  a.instance.hidden(); a.instance.shown();
  await answer(take('alpha', 'list', 'src'), listing('/alpha/src', [{ name: 'a.go' }]));
  await answer(take('alpha', 'list', ''), listing('/alpha', undefined, true));
  check('reopen refresh preserves expansion selection and scroll', [a.tree.scrollTop, b.tree.scrollTop, !!a.at('[data-entry="src/a.go"]'), a.instance.selection().path], [73, 19, true, 'src/a.go']);
  a.all('.fseg button')[1].click(); a.all('.fopts button').forEach(n => n.click());
  check('only content mode shows Aa and regex options', [a.at('.fopts').hidden, b.at('.fopts').hidden, a.all('.fopts button').length], [false, true, 2]);
  a.q.value = 'needle'; event(a.q, 'input'); await timers(); var old = take('alpha', 'search');
  b.q.value = 'beta'; event(b.q, 'input'); await timers(); var br = take('beta', 'find');
  check('mode query options independent', [a.el.getAttribute('data-fm'), b.el.getAttribute('data-fm'), old.url.searchParams.get('case'), old.url.searchParams.get('regex'), br.url.searchParams.get('q')], ['content', 'name', '1', '1', 'beta']);
  a.q.value = 'new'; event(a.q, 'input'); await timers(); var fresh = take('alpha', 'search');
  check('only superseded search aborted', [old.options.signal.aborted, br.options.signal.aborted], [true, false]);
  await answer(fresh, { files: [{ path: 'fresh.go', matches: [{ line: 7, hit: 'new' }] }] });
  await answer(old, { files: [{ path: 'stale.go' }] });
  await answer(br, { items: [{ path: 'beta.go' }] });
  check('late results cannot overwrite either owner', [!!a.at('[data-path="fresh.go"]'), !!a.at('[data-path="stale.go"]'), !!b.at('[data-path="beta.go"]')], [true, false, true]);
  a.hits.scrollTop = 31; event(a.hits, 'scroll'); b.hits.scrollTop = 17; event(b.hits, 'scroll');
  a.instance.hidden(); a.instance.shown();
  var staleRefreshRoot = take('alpha', 'list', ''), staleRefreshDir = take('alpha', 'list', 'src');
  check('search scroll belongs to each instance', [a.hits.scrollTop, b.hits.scrollTop], [31, 17]);
  a.q.value = 'before root'; event(a.q, 'input'); await timers(); var staleRootSearch = take('alpha', 'search');
  rootRequest(a, '/older'); var older = take('alpha', 'list'); rootRequest(a, '/newer'); var newer = take('alpha', 'list');
  await answer(newer, listing('/newer')); await timers(); var rooted = take('alpha', 'search');
  await answer(older, listing('/older'));
  await answer(staleRefreshRoot, listing('/alpha', [{ name: 'wrong-root.go' }], true));
  await answer(staleRefreshDir, listing('/alpha/src', [{ name: 'wrong-child.go' }]));
  await answer(staleRootSearch, { files: [{ path: 'wrong-search.go' }] });
  check('root replacement aborts search and invalidates listing generations', [staleRootSearch.options.signal.aborted, !!a.at('[data-path="wrong-root.go"]'), !!a.at('[data-path="src/wrong-child.go"]'), !!a.at('[data-path="wrong-search.go"]')], [true, false, false, false]);
  check('latest root wins and root/session routing preserved', [a.at('.froot').textContent, rooted.url.pathname, rooted.url.searchParams.get('root'), b.at('.froot').textContent], ['newer', '/s/alpha/fs/search', '/newer', 'beta']);
  rootRequest(a, '/last'); var lateRoot = take('alpha', 'list');
  var beforeDoc = doc.listeners.keydown.length, beforeWin = win.listeners.resize.length;
  a.instance.destroy(); a.instance.destroy();
  await answer(lateRoot, listing('/last')); await answer(rooted, { files: [{ path: 'destroyed.go' }] });
  check('destroy retires handlers and late root/search', [a.el.children.length, rooted.options.signal.aborted, doc.listeners.keydown.length, win.listeners.resize.length], [0, true, beforeDoc - 1, beforeWin - 1]);
  slash(b.tree); check('surviving owner shortcut works', doc.activeElement === b.q, true);
  var remote = explorer('remote', false); remote.store['ws.dashboard.files.root.remote'] = '/unauthorized'; remote.instance.shown();
  var rr = take('remote', 'list');
  check('remote UI cannot offer change-root or restore local root', [remote.at('.fchange-root').disabled, rr.url.searchParams.has('root')], [true, false]);
  await answer(rr, listing('/remote', undefined, true));
  event(b.tree, 'click', { target: b.at('[data-path="src"]') });
  var lateDir = take('beta', 'list', 'src');
  b.instance.destroy(); remote.instance.destroy();
  await answer(lateDir, listing('/beta/src', [{ name: 'after-destroy.go' }]));
  check('late child listing cannot rebuild a destroyed explorer', b.el.children.length, 0);
  check('all global listeners retired', [doc.listeners.keydown.length, doc.listeners.mousedown.length, doc.listeners.wheel.length, win.listeners.resize.length, win.listeners.blur.length, pendingTimers.size], [0, 0, 0, 0, 0, 0]);
  var sent = [], copied = [], opened = [], events = [];
  var core = explorer('core', true, { send: p => sent.push(p), copy: p => copied.push(p), openCard: p => opened.push(p) });
  core.instance.on('navigation', p => events.push(['nav', p.path]));
  core.instance.on('selection', p => events.push(['sel', p.path]));
  check('footer exposes attachment and copy actions before a selection is available', [core.all('.ffooter .fattachment button').length, core.at('.fselection').textContent, core.all('.fattachment button').map(n => n.disabled)], [2, '', [true, true]]);
  core.instance.shown();
  await answer(take('core', 'list', ''), listing('/core', [{ name: 'src', dir: true }, { name: 'photo space.png' }, { name: 'report.pdf' }, { name: 'unsafe.svg' }, { name: 'linked.png', link: true }] , true));
  check('side tree lists only folders while the directory pane keeps files', [core.all('.fr[data-path]').map(n => n.getAttribute('data-path')), core.all('.fentry').map(n => n.getAttribute('data-entry'))], [['src'], ['src', 'photo space.png', 'report.pdf', 'unsafe.svg', 'linked.png']]);
  core.all('.fview-modes button').forEach((button, i) => {
    button.click();
    check('side tree stays folders-only in view ' + i, core.all('.fr[data-path]').map(n => n.getAttribute('data-path')), ['src']);
  });
  core.all('.fview-modes button')[0].click();
  event(core.at('[data-entry="src"]'), 'auxclick', { button: 1 });
  check('directory grid middle-click also opens a background tab', [core.all('.ftab-select').length, core.instance.location().dir], [2, '']);
  core.all('.ftab-close')[1].click();
  event(core.tree, 'auxclick', { target: core.at('[data-path="src"]'), button: 1 });
  check('middle-click folder opens background tab without request or navigation', [core.all('.ftab-select').length, core.all('.ftab-select')[0].getAttribute('aria-selected'), core.instance.location().dir, requests.filter(r => !r.taken && r.url.pathname === '/s/core/fs/list').length], [2, 'true', '', 0]);
  var selectedRows = () => core.all('.fentry').filter(n => n.classList.contains('sel') || n.getAttribute('aria-selected') !== 'false').map(n => n.getAttribute('data-entry'));
  event(core.all('.fentry').find(n => n.getAttribute('data-entry') === 'report.pdf'), 'click');
  check('clicking one directory row selects only that row', selectedRows(), ['report.pdf']);
  event(core.all('.fentry').find(n => n.getAttribute('data-entry') === 'photo space.png'), 'click');
  check('clicking another directory row moves the single selection', selectedRows(), ['photo space.png']);
  core.all('.fattachment button')[0].click(); core.all('.fattachment button')[1].click();
  check('footer follows the selected path and retains both labeled actions', [core.at('.fselection b').textContent, core.at('.fselection').title, core.all('.fattachment button').map(n => n.textContent)], ['photo space.png', '/core/photo space.png', ['js.files.send', 'js.files.copy']]);
  check('browse actions emit session-relative path and selection event', [sent, copied, events.some(e => e[0] === 'sel' && e[1] === 'photo space.png')], [['photo space.png'], ['photo space.png'], true]);
  core.all('.fview-modes button')[3].click();
  check('gallery side tree still lists only folders', core.all('.fr[data-path]').map(n => n.getAttribute('data-path')), ['src']);
  check('gallery uses guarded raw route only for supported unlinked image', core.all('.fthumb').map(n => n.src), ['/s/core/fs/raw?path=photo%20space.png']);
  core.at('.fdirectory').scrollTop = 41; event(core.at('.fdirectory'), 'scroll');
  core.tree.scrollTop = 17; event(core.tree, 'scroll');
  core.all('.ftab-select')[1].click();
  check('loading tab retires previous actionable rows and browse buttons', [core.all('.fentry').length, core.all('.fr[data-path]').length, core.all('.fattachment button')[0].disabled], [0, 0, true]);
  await answer(take('core', 'list', ''), listing('/core', undefined, true));
  await answer(take('core', 'list', 'src'), listing('/core/src', [{ name: 'child.go' }], true));
  check('background tab activates at its folder', core.instance.location(), { root: '', dir: 'src' });
  check('current side-tree folder is highlighted and breadcrumb names each segment', [core.at('[data-path="src"]').classList.contains('cur'), core.at('[data-path="src"]').getAttribute('aria-current'), core.at('.fcrumb-root .froot').textContent, core.at('.fcrumb-trail .fcrumb').textContent], [true, 'location', 'core', 'src']);
  core.at('.fcrumb-trail .fcrumb').click();
  check('breadcrumb keeps directory navigation reachable', core.instance.location().dir, 'src');
  check('expanded side tree omits nested files but the directory pane keeps them', [core.all('.fr[data-path]').map(n => n.getAttribute('data-path')), !!core.at('[data-entry="src/child.go"]')], [['src'], true]);
  core.all('.fview-modes button')[1].click(); core.at('.fopen-card').click();
  check('open-as-card forwards active directory', opened, [{ root: '', dir: 'src' }]);
  core.all('.ftab-select')[0].click();
  check('tabs restore selection mode and scroll independently', [core.at('.fdirectory').getAttribute('data-view'), core.instance.selection().path, core.at('.fdirectory').scrollTop, core.tree.scrollTop], ['gallery', 'photo space.png', 41, 17]);
  core.all('.ftab-select')[1].click();
  check('second tab retains its own view', core.at('.fdirectory').getAttribute('data-view'), 'list');
  core.instance.hidden(); var roundtrip = JSON.parse(JSON.stringify(core.store)); core.instance.destroy();
  var restored = explorer('core', true, { store: roundtrip }); restored.instance.shown();
  await answer(take('core', 'list', ''), listing('/core', undefined, true));
  await answer(take('core', 'list', 'src'), listing('/core/src', [{ name: 'child.go' }], true));
  check('storage roundtrip restores tabs active directory and mode', [restored.all('.ftab-select').length, restored.all('.ftab-select')[1].getAttribute('aria-selected'), restored.instance.location().dir, restored.at('.fdirectory').getAttribute('data-view')], [2, 'true', 'src', 'list']);
  restored.all('.ftab-close')[0].click();
  check('closing background tab preserves active tab and disables last close', [restored.all('.ftab-select').length, restored.instance.location().dir, restored.at('.ftab-close').disabled], [1, 'src', true]);
  restored.instance.destroy();

  var columns = explorer('columns'); columns.instance.shown();
  var columnRoot = listing('/columns', [{ name: 'src', dir: true }, { name: 'docs', dir: true }, { name: 'README.md' }, { name: 'linked', dir: true, link: true }, { name: 'gone', dir: true, gone: true }], true);
  var columnSrc = listing('/columns/src', [{ name: 'pkg', dir: true }, { name: 'main.go' }]);
  var columnPkg = listing('/columns/src/pkg', [{ name: 'first.go' }, { name: 'last.go' }]);
  await answer(take('columns', 'list', ''), columnRoot);
  function entry(path) { return columns.at('[data-entry="' + path + '"]'); }
  function columnPaths() { return columns.all('.fcolumn').map(n => n.getAttribute('data-column')); }
  columns.all('.fview-modes button')[4].click();
  check('column switcher is compact and accessible', [columns.all('.fview-modes button')[4].html.includes('#l-columns-3'), columns.all('.fview-modes button')[4].getAttribute('aria-label'), columns.all('.fview-modes button')[4].getAttribute('aria-pressed')], [true, 'js.files.view.column', 'true']);
  check('column mode begins at root with files and folders, sidebar inert', [columnPaths(), columns.all('.fcolumn .fentry').map(n => n.getAttribute('data-entry')), columns.at('.fworkspace').getAttribute('data-view'), columns.tree.inert], [[''], ['src', 'docs', 'README.md', 'linked', 'gone'], 'column', true]);
  entry('src').click();
  await answer(take('columns', 'list', 'src'), columnSrc);
  check('single folder selection opens next column and highlights with chevron', [columnPaths(), entry('src').getAttribute('aria-selected'), !!entry('src').querySelector('.fcolumn-chevron'), columns.instance.location().dir, columns.instance.selection().path], [['', 'src'], 'true', true, 'src', 'src']);
  columns.all('.fcolumn')[0].scrollTop = 23; event(columns.all('.fcolumn')[0], 'scroll');
  entry('src/pkg').click(); await answer(take('columns', 'list', 'src/pkg'), columnPkg);
  check('nested folder levels retain highlighted ancestor and files in every level', [columnPaths(), entry('src').getAttribute('aria-selected'), entry('src/pkg').getAttribute('aria-selected'), !!entry('README.md'), !!entry('src/main.go'), !!entry('src/pkg/first.go'), columns.all('.fcolumn')[0].scrollTop], [['', 'src', 'src/pkg'], 'true', 'true', true, true, true, 23]);
  entry('src/pkg/first.go').click();
  await answer(take('columns', 'preview'), { kind: 'text', text: 'package main' });
  check('column single click previews', columns.previews, [['src/pkg/first.go', {}]]);
  event(entry('src/pkg/first.go'), 'dblclick');
  await answer(take('columns', 'preview'), { kind: 'text', text: 'package main' });
  check('column double click opens for real', [columns.instance.selection().path, entry('src/pkg/first.go').getAttribute('aria-selected'), columns.previews[1]], ['src/pkg/first.go', 'true', ['src/pkg/first.go', { keep: true }]]);
  key(entry('src/pkg/first.go'), 'ArrowDown');
  check('down moves focus only without preview or navigation', [columns.instance.selection().path, doc.activeElement.getAttribute('data-entry'), columns.instance.location().dir, columns.previews.length], ['src/pkg/first.go', 'src/pkg/last.go', 'src/pkg', 2]);
  key(entry('src/pkg/last.go'), 'ArrowUp'); key(entry('src/pkg/first.go'), 'ArrowLeft');
  check('up and left move within and between columns', [columns.instance.selection().path, doc.activeElement.getAttribute('data-entry')], ['src/pkg/first.go', 'src/pkg']);
  key(entry('src/pkg'), 'ArrowRight');
  check('right focuses the selected child column entry', doc.activeElement.getAttribute('data-entry'), 'src/pkg/first.go');
  key(entry('src/pkg/first.go'), 'Enter');
  await answer(take('columns', 'preview'), { kind: 'text', text: 'package main' });
  check('Enter opens selected file for real', [columns.previews.length, columns.previews[2]], [3, ['src/pkg/first.go', { keep: true }]]);
  columns.all('.fcolumn')[2].scrollTop = 67; event(columns.all('.fcolumn')[2], 'scroll');
  columns.at('.fdirectory').scrollLeft = 178; event(columns.at('.fdirectory'), 'scroll');
  columns.all('.fview-modes button')[0].click();
  check('leaving column mode restores sidebar and same directory selection', [columns.tree.inert, columns.at('.fworkspace').getAttribute('data-view'), columns.instance.location().dir, columns.instance.selection().path], [false, 'details', 'src/pkg', 'src/pkg/first.go']);
  columns.at('.fdirectory').scrollTop = 37; event(columns.at('.fdirectory'), 'scroll');
  columns.all('.fview-modes button')[4].click();
  check('returning to columns retains independent horizontal and vertical scroll', [columns.at('.fdirectory').scrollLeft, columns.all('.fcolumn')[0].scrollTop, columns.all('.fcolumn')[2].scrollTop, columns.instance.selection().path], [178, 23, 67, 'src/pkg/first.go']);
  columns.instance.newTab({ dir: '', view: 'list' }); await answer(take('columns', 'list', ''), columnRoot);
  columns.all('.ftab-select')[0].click();
  check('column tab retains mode path selection and scroll across other tab', [columnPaths(), columns.instance.selection().path, columns.at('.fdirectory').getAttribute('data-view'), columns.at('.fdirectory').scrollLeft, columns.all('.fcolumn')[2].scrollTop], [['', 'src', 'src/pkg'], 'src/pkg/first.go', 'column', 178, 67]);
  columns.all('.fview-modes button')[0].click();
  check('non-column scroll also survives column and tab switches', columns.at('.fdirectory').scrollTop, 37);
  columns.all('.fview-modes button')[4].click(); columns.instance.hidden(); columns.instance.shown();
  await answer(take('columns', 'list', ''), columnRoot);
  await answer(take('columns', 'list', 'src'), columnSrc);
  await answer(take('columns', 'list', 'src/pkg'), columnPkg);
  check('refresh keeps every column level and selection', [columnPaths(), !!entry('src/main.go'), columns.instance.selection().path], [['', 'src', 'src/pkg'], true, 'src/pkg/first.go']);
  var columnStore = JSON.parse(JSON.stringify(columns.store)); columns.instance.destroy();
  columns = explorer('columns', true, { store: columnStore }); columns.instance.shown();
  await answer(take('columns', 'list', ''), columnRoot);
  await answer(take('columns', 'list', 'src/pkg'), columnPkg);
  check('restored deep folder has one column per ancestor before lazy load', columnPaths(), ['', 'src', 'src/pkg']);
  await answer(take('columns', 'list', 'src'), columnSrc);
  check('column mode state persists through storage and lazy ancestor restoration', [columnPaths(), !!entry('src/main.go'), columns.instance.selection().path, columns.at('.fdirectory').scrollLeft, columns.all('.fcolumn')[2].scrollTop], [['', 'src', 'src/pkg'], true, 'src/pkg/first.go', 178, 67]);
  entry('README.md').click();
  await answer(take('columns', 'preview'), { kind: 'text', text: 'readme' });
  check('single click on ancestor file drops trailing columns and previews', [columnPaths(), columns.instance.location().dir, columns.instance.selection().path, columns.previews], [[''], '', 'README.md', [['README.md', {}]]]);
  entry('linked').click(); event(entry('linked'), 'dblclick');
  entry('gone').click(); event(entry('gone'), 'dblclick');
  check('symlink and gone folders stay selectable but never open or preview', [columnPaths(), columns.instance.selection().path, columns.previews.length, requests.filter(r => !r.taken && r.url.pathname === '/s/columns/fs/list').length], [[''], 'gone', 1, 0]);
  entry('docs').click();
  var cappedColumn = listing('/columns/docs', []); cappedColumn.more = true;
  await answer(take('columns', 'list', 'docs'), cappedColumn);
  check('column listing retains existing empty and truncated notices', columns.all('.fcolumn')[1].querySelectorAll('.fnote').map(n => n.textContent), ['js.files.empty', 'js.files.more']);
  columns.instance.destroy();

  var columnRace = explorer('column-race', true, { location: { dir: 'src/pkg', view: 'column' } }); columnRace.instance.shown();
  await answer(take('column-race', 'list', ''), columnRoot);
  await answer(take('column-race', 'list', 'src/pkg'), columnPkg);
  var staleAncestor = take('column-race', 'list', 'src');
  columnRace.instance.newTab({ dir: '', view: 'list' });
  await answer(take('column-race', 'list', ''), columnRoot);
  await answer(staleAncestor, columnSrc);
  check('late ancestor cannot repaint another tab', [columnRace.at('.fdirectory').getAttribute('data-view'), columnRace.instance.location().dir, columnRace.all('.fcolumn').length, !!columnRace.at('[data-entry="src/main.go"]')], ['list', '', 0, false]);
  columnRace.instance.destroy();

  var legacy = explorer('legacy', true, { store: { 'ws.dashboard.files.root.legacy': '/other', 'ws.dashboard.files.view': JSON.stringify({ '/other': { ign: true } }) }, send: p => sent.push(p), copy: p => copied.push(p) });
  legacy.instance.shown(); var lr = take('legacy', 'list', '');
  check('legacy root record still loads', lr.url.searchParams.get('root'), '/other');
  await answer(lr, listing('/other', [{ name: 'ignored.png', ign: true }]));
  check('changed-root tab uses the actual folder name', legacy.at('.ftab-select .fn').textContent, 'other');
  check('legacy view record still loads', !!legacy.at('[data-entry="ignored.png"]'), true);
  event(legacy.at('[data-entry="ignored.png"]'), 'click'); legacy.all('.fattachment button')[0].click(); legacy.all('.fattachment button')[1].click();
  check('outside-root send and copy use absolute host path', [sent.slice(-1), copied.slice(-1)], [['/other/ignored.png'], ['/other/ignored.png']]);
  var retired = 0, off = legacy.instance.on('selection', () => retired++), disposed = 0;
  legacy.instance.setAttachment((slot, api) => { slot.appendChild(new El('button', doc)); check('attachment gets same core API', api === legacy.instance, true); return () => disposed++; });
  off(); event(legacy.at('[data-entry="ignored.png"]'), 'click');
  legacy.instance.setAttachment(() => () => disposed++); legacy.instance.destroy();
  check('swappable attachment disposes on replacement and destroy; unsubscribe works', [disposed, retired], [2, 0]);

  var tabRace = explorer('tab-race'); tabRace.instance.shown(); var oldRoot = take('tab-race', 'list');
  tabRace.instance.newTab({ dir: 'src' }); var currentRoot = take('tab-race', 'list');
  await answer(currentRoot, listing('/tab-race', undefined, true)); var restoreDir = take('tab-race', 'list', 'src');
  tabRace.instance.navigate(''); await answer(restoreDir, listing('/tab-race/src', [{ name: 'late.go' }], true));
  await answer(oldRoot, listing('/obsolete'));
  check('tab switch and later navigation reject stale root and restore answers', [tabRace.instance.location(), !!tabRace.at('[data-entry="src/late.go"]')], [{ root: '', dir: '' }, false]);
  tabRace.instance.newTab({ dir: 'src' }); var closingRoot = take('tab-race', 'list');
  tabRace.all('.ftab-close')[2].click(); await answer(closingRoot, listing('/closed'));
  check('closed active tab cannot repaint surviving tab', tabRace.at('.froot').textContent, 'tab-race');
  tabRace.instance.destroy();
  var hostile = explorer('hostile', false, { store: { 'ws.dashboard.files.tabs.hostile': JSON.stringify({ active: 0, tabs: [{ root: '/secret', dir: '../escape', view: 'invalid' }] }) } });
  hostile.instance.shown(); var hr = take('hostile', 'list');
  check('remote restored records cannot select local root or traversal', [hr.url.searchParams.has('root'), hr.url.searchParams.has('dir'), hostile.instance.location()], [false, false, { root: '', dir: '' }]);
  await answer(hr, listing('/hostile', undefined, true)); hostile.instance.destroy();
  var candidate = explorer('candidate'); candidate.instance.shown(); var initialCandidate = take('candidate', 'list');
  rootRequest(candidate, '/chosen'); var choice = take('candidate', 'list');
  await answer(initialCandidate, listing('/candidate', undefined, true));
  await answer(choice, listing('/chosen'));
  check('initial root completion cannot cancel an in-flight candidate root', candidate.instance.location().root, '/chosen');
  candidate.instance.destroy();
  var navRace = explorer('nav-race'); navRace.instance.shown();
  await answer(take('nav-race', 'list'), listing('/nav-race', undefined, true));
  event(navRace.tree, 'click', { target: navRace.at('[data-path="src"]') }); var pendingFolder = take('nav-race', 'list', 'src');
  navRace.instance.navigate(''); await answer(pendingFolder, listing('/nav-race/src', [{ name: 'late.go' }], true));
  check('late sidebar expansion cannot undo a later directory navigation', navRace.instance.location().dir, '');
  navRace.instance.destroy();
  // Phase 2: same-session windows and their inner tabs own search independently.
  var left = explorer('search-tabs'), right = explorer('search-tabs', true, { location: { root: '/right' } });
  var searchPreviews = []; win.WsPreview = { state: () => '', open: (path, o) => { var record = {}; if (o.root) record.root = o.root; if (o.line) record.line = o.line; searchPreviews.push([path, record]); } };
  left.instance.shown(); await answer(take('search-tabs', 'list'), listing('/project', undefined, true));
  right.instance.shown(); await answer(take('search-tabs', 'list'), listing('/right'));
  left.q.value = 'original'; event(left.q, 'input'); await timers(); var originalSearch = take('search-tabs', 'find');
  right.q.value = 'parallel'; event(right.q, 'input'); await timers(); var parallelSearch = take('search-tabs', 'find');
  left.instance.newTab({ root: '/alternate', dir: '' }); var alternateRoot = take('search-tabs', 'list');
  check('tab switch aborts only its own window request and resets search controls', [originalSearch.options.signal.aborted, parallelSearch.options.signal.aborted, left.q.value, left.el.getAttribute('data-fm')], [true, false, '', 'name']);
  await answer(originalSearch, { items: [{ path: 'late-original.go' }] });
  check('switched tab drops late search before new root loads', !!left.at('[data-path="late-original.go"]'), false);
  await answer(alternateRoot, listing('/alternate'));
  left.all('.fseg button')[1].click(); left.all('.fopts button').forEach(n => n.click());
  left.q.value = 'needle.*'; event(left.q, 'input'); await timers(); var alternateSearch = take('search-tabs', 'search');
  check('active tab root scopes content search with its own options', [alternateSearch.url.searchParams.get('root'), alternateSearch.url.searchParams.get('case'), alternateSearch.url.searchParams.get('regex')], ['/alternate', '1', '1']);
  left.all('.ftab-select')[0].click(); await timers(); var resumedOriginal = take('search-tabs', 'find');
  check('restoring name mode hides content options again', left.at('.fopts').hidden, true);
  check('returning tab restores its query mode and options, not another tab or window', [left.q.value, left.el.getAttribute('data-fm'), left.all('.fopts button').map(n => n.getAttribute('aria-pressed')), resumedOriginal.url.searchParams.has('root'), alternateSearch.options.signal.aborted, parallelSearch.options.signal.aborted], ['original', 'name', ['false', 'false'], false, true, false]);
  await answer(alternateSearch, { files: [{ path: 'late-alternate.go', matches: [{ line: 3, hit: 'needle' }] }] });
  await answer(resumedOriginal, { items: [{ path: 'original.go' }] });
  await answer(parallelSearch, { items: [{ path: 'parallel.go' }] });
  check('two windows and tabs reject cross-root results', [!!left.at('.fhits [data-path="original.go"]'), !!left.at('[data-path="late-alternate.go"]'), !!right.at('.fhits [data-path="parallel.go"]')], [true, false, true]);
  event(left.hits, 'click', { target: left.at('.hit') }); event(right.hits, 'click', { target: right.at('.hit') });
  await answer(take('search-tabs', 'preview'), { kind: 'text', text: 'original' });
  await answer(take('search-tabs', 'preview'), { kind: 'text', text: 'parallel' });
  check('name hits use existing preview with the originating root', searchPreviews, [['original.go', {}], ['parallel.go', { root: '/right' }]]);
  left.hits.scrollTop = 57; event(left.hits, 'scroll');
  left.all('.ftab-select')[1].click(); await timers(); var resumedAlternate = take('search-tabs', 'search');
  check('unfinished content tab restarts with restored options', [left.q.value, left.el.getAttribute('data-fm'), resumedAlternate.url.searchParams.get('root'), resumedAlternate.url.searchParams.get('regex')], ['needle.*', 'content', '/alternate', '1']);
  await answer(resumedAlternate, { files: [{ path: 'match.go', matches: [{ line: 19, pre: '<tag>', hit: 'needle', post: '</tag>' }] }] });
  event(left.hits, 'click', { target: left.at('.hl') });
  var textPreview = take('search-tabs', 'preview');
  check('content preflight retains root and text query', [textPreview.url.searchParams.get('root'), textPreview.url.searchParams.get('text')], ['/alternate', '1']);
  await answer(textPreview, { kind: 'text', text: 'needle' });
  check('content hit previews its matching line and tab root', searchPreviews.slice(-1), [['match.go', { root: '/alternate', line: 19 }]]);
  check('content excerpts render as text rather than HTML', [left.at('.tx').children[0].textContent, left.at('.tx mark').textContent], ['<tag>', 'needle']);
  left.all('.ftab-select')[0].click();
  check('completed results and scroll restore only on their tab without a request', [left.q.value, !!left.at('.fhits [data-path="original.go"]'), left.hits.scrollTop, pendingTimers.size], ['original', true, 57, 0]);
  left.all('.ftab-select')[0].click();
  check('selecting the already-active tab keeps search intact', [left.q.value, !!left.at('.hit'), pendingTimers.size], ['original', true, 0]);
  left.all('.fseg button')[1].click();
  check('mode change immediately removes obsolete actionable hits', [left.all('.hit').length, left.el.getAttribute('data-fm')], [0, 'content']);
  await timers(); var superseded = take('search-tabs', 'search');
  left.q.value = 'newer'; event(left.q, 'input');
  check('query invalidates in-flight request before debounce expires', superseded.options.signal.aborted, true);
  await answer(superseded, { files: [{ path: 'debounce-stale.go' }] });
  check('late answer cannot repaint during debounce', left.all('.hit').length, 0);
  await timers(); var newerSearch = take('search-tabs', 'search');
  left.all('.fseg button')[0].click();
  check('mode invalidates in-flight content request immediately', newerSearch.options.signal.aborted, true);
  await refuse(newerSearch);
  check('superseded failure cannot show an error on the new mode', left.all('.fhits .err').length, 0);
  await timers(); var modeSearch = take('search-tabs', 'find');
  key(left.q, 'Escape'); await answer(modeSearch, { items: [{ path: 'escaped.go' }] });
  check('Escape cancels query and restores browsing without touching other window', [modeSearch.options.signal.aborted, left.q.value, left.hits.hidden, left.at('.fworkspace').hidden, !!right.at('.hit')], [true, '', true, false, true]);

  left.q.value = 'hidden'; event(left.q, 'input'); await timers(); var hiddenSearch = take('search-tabs', 'find');
  right.q.value = 'still-running'; event(right.q, 'input'); await timers(); var survivingSearch = take('search-tabs', 'find');
  left.instance.hidden();
  check('hiding one window cancels only its unfinished search', [hiddenSearch.options.signal.aborted, survivingSearch.options.signal.aborted], [true, false]);
  await answer(hiddenSearch, { items: [{ path: 'hidden-stale.go' }] });
  check('hidden window drops in-flight answer and keeps its query', [!!left.at('.hit'), left.q.value], [false, 'hidden']);
  left.instance.shown(); var refreshSearchRoot = take('search-tabs', 'list'); await timers(); var shownSearch = take('search-tabs', 'find');
  await answer(refreshSearchRoot, listing('/project', undefined, true));
  await answer(shownSearch, { items: [{ path: 'shown.go' }] });
  await answer(survivingSearch, { items: [{ path: 'surviving.go' }] });
  check('returning window restarts unfinished query independently', [shownSearch.url.searchParams.get('q'), !!left.at('[data-path="shown.go"]'), !!right.at('[data-path="surviving.go"]')], ['hidden', true, true]);
  left.q.value = 'background-close'; event(left.q, 'input'); await timers(); var backgroundClose = take('search-tabs', 'find');
  left.all('.ftab-close')[1].click();
  check('closing background tab does not cancel active search', backgroundClose.options.signal.aborted, false);
  await answer(backgroundClose, { items: [{ path: 'kept.go' }] });

  left.instance.newTab({ root: '/closing' }); await answer(take('search-tabs', 'list'), listing('/closing'));
  // Fetch implementations without AbortController still need token isolation.
  win.AbortController = null;
  left.q.value = 'closing'; event(left.q, 'input'); await timers(); var closingSearch = take('search-tabs', 'find');
  left.all('.ftab-close')[1].click(); await answer(closingSearch, { items: [{ path: 'closed-stale.go' }] });
  check('closed active tab drops results even without AbortController', [closingSearch.options.signal, left.q.value, !!left.at('.fhits [data-path="kept.go"]'), !!left.at('[data-path="closed-stale.go"]')], [null, 'background-close', true, false]);
  left.q.value = 'no-controller'; event(left.q, 'input'); await timers(); var noController = take('search-tabs', 'find');
  left.q.value = 'replacement'; event(left.q, 'input'); await refuse(noController);
  check('query token rejects stale failures without AbortController', left.all('.fhits .err').length, 0);
  left.instance.hidden(); await timers();
  check('hidden window cancels pending debounce without issuing fetch', requests.filter(r => !r.taken && r.url.pathname === '/s/search-tabs/fs/find').length, 0);
  win.AbortController = AbortController;
  left.instance.shown(); await answer(take('search-tabs', 'list'), listing('/project', undefined, true)); await timers(); var closingWindow = take('search-tabs', 'find');
  left.instance.destroy(); await answer(closingWindow, { items: [{ path: 'destroyed-search.go' }] });
  check('closed window aborts request and drops late answer without affecting peer', [closingWindow.options.signal.aborted, left.el.children.length, !!right.at('[data-path="surviving.go"]')], [true, 0, true]);
  right.q.value = 'cancel-before-start'; event(right.q, 'input'); right.instance.destroy(); await timers();
  check('destroy cancels pending debounce', requests.filter(r => !r.taken && r.url.pathname === '/s/search-tabs/fs/find').length, 0);
  await sidebarToneChecks();
  await pathBarChecks();
  // Root suggestions belong to the open popover, never another tab or owner.
  var notices = [], choices = explorer('root-choices', true, { toast: s => notices.push(s) });
  choices.instance.shown(); var initialChoiceRoot = take('root-choices', 'list');
  choices.at('.fchange-root').click();
  var firstChoices = take('root-choices', 'worktrees');
  check('root list request is session scoped', firstChoices.url.searchParams.has('root'), false);
  var offered = { roots: [{ path: '/project', branch: 'main', detached: false }, { path: '/linked', branch: 'topic', detached: false }, { path: '/detached', branch: '', detached: true }] };
  await answer(firstChoices, offered);
  check('root choices can precede the initial listing and still mark the session root', choices.at('.froot-current button').getAttribute('aria-pressed'), 'true');
  await answer(initialChoiceRoot, listing('/project', undefined, true));
  check('root rows show folder branch detached and current root', choices.all('.froot-row').map(n => [n.querySelector('.froot-name').textContent, n.querySelector('.froot-branch').textContent, n.title, n.getAttribute('aria-pressed')]), [['project', 'main', '/project', 'true'], ['linked', 'topic', '/linked', 'false'], ['detached', 'js.wt.detached', '/detached', 'false']]);
  check('root suggestions precede the retained free-text input', choices.at('.froot-pop').children.indexOf(choices.at('.froot-list')) < choices.at('.froot-pop').children.indexOf(choices.at('.froot-pop .fpin')), true);
  choices.all('.froot-list button')[0].click(); var linkedChoice = take('root-choices', 'list');
  check('suggestion switches through the existing root parameter', linkedChoice.url.searchParams.get('root'), '/linked');
  await answer(linkedChoice, listing('/linked'));
  choices.at('.fchange-root').click(); await answer(take('root-choices', 'worktrees'), offered);
  check('reopening pins the accepted root above remaining suggestions', choices.all('.froot-row').map(n => [n.title, n.getAttribute('aria-pressed')]), [['/linked', 'true'], ['/project', 'false'], ['/detached', 'false']]);
  choices.all('.froot-list button')[0].click(); var homeChoice = take('root-choices', 'list');
  check('session suggestion restores implicit session routing', homeChoice.url.searchParams.has('root'), false);
  await answer(homeChoice, listing('/project', undefined, true));
  choices.at('.fchange-root').click(); var staleChoices = take('root-choices', 'worktrees');
  key(choices.at('.froot-pop'), 'Escape'); choices.at('.fchange-root').click(); var freshChoices = take('root-choices', 'worktrees');
  await answer(freshChoices, offered); await answer(staleChoices, { roots: [{ path: '/stale' }] });
  check('late roots cannot overwrite a reopened popover', choices.all('.froot-row').length, 3);
  key(choices.at('.froot-pop'), 'Escape'); choices.at('.fchange-root').click(); var tabChoices = take('root-choices', 'worktrees');
  choices.instance.newTab({ root: '/linked' }); await answer(take('root-choices', 'list'), listing('/linked'));
  await answer(tabChoices, offered);
  check('tab switch closes popover before its late roots return', choices.at('.froot-pop').hidden, true);
  choices.at('.fchange-root').click(); await refuse(take('root-choices', 'worktrees'));
  check('failed root suggestions leave free-text change usable', [choices.at('.froot-list').hidden, choices.at('.froot-pop .fpin').value], [true, '/linked']);
  choices.at('.froot-pop .fpin').value = '/arbitrary'; choices.at('.fgo .pr').click(); await answer(take('root-choices', 'list'), listing('/arbitrary'));
  check('unlisted existing folder can still be selected', choices.instance.location().root, '/arbitrary');
  choices.at('.fchange-root').click(); var deadChoices = take('root-choices', 'worktrees'); choices.instance.destroy(); await answer(deadChoices, offered);
  check('destroy drops late roots without rebuilding the owner', choices.el.children.length, 0);
  var restoredGone = explorer('gone-root', true, { toast: s => notices.push(s), store: { 'ws.dashboard.files.tabs.gone-root': JSON.stringify({ active: 0, tabs: [{ root: '/removed', dir: 'old' }] }) } });
  restoredGone.instance.shown(); var removedListing = take('gone-root', 'list');
  restoredGone.at('.fchange-root').click(); await answer(take('gone-root', 'worktrees'), offered);
  await refuse(removedListing); await answer(take('gone-root', 'list'), listing('/project', undefined, true));
  check('fallback updates the current marker in an already-open root popover', [restoredGone.at('.froot-current button').title, restoredGone.at('.froot-current button').getAttribute('aria-pressed')], ['/project', 'true']);
  check('refused persisted worktree falls back and notifies', [restoredGone.instance.location(), notices, JSON.parse(restoredGone.store['ws.dashboard.files.tabs.gone-root']).tabs[0].root], [{ root: '', dir: '' }, ['js.files.root.dropped'], '']);
  restoredGone.instance.destroy();
  var remoteChoices = explorer('remote-choices', false); remoteChoices.instance.shown(); await answer(take('remote-choices', 'list'), listing('/project', undefined, true));
  // The fake DOM deliberately fires even a disabled button; do not disclose a list.
  remoteChoices.at('.fchange-root').click();
  check('remote viewer has disabled root control and no suggestions or enumeration', [remoteChoices.at('.fchange-root').disabled, remoteChoices.at('.froot-list').hidden, requests.some(r => r.url.pathname === '/s/remote-choices/fs/worktrees')], [true, true, false]);
  remoteChoices.instance.destroy();
  await rootFilterChecks();
  await workspacePickerChecks();
  check('extended core leaves no global handlers or timers', [doc.listeners.keydown.length, win.listeners.resize.length, pendingTimers.size], [0, 0, 0]);
  // Run both production modules together: activation preflights before card
  // creation, and a failing player reuses the originating explorer's row menu.
  El.prototype.offsetWidth = 120; El.prototype.offsetHeight = 120;
  var media = explorer('media'), previewCards = [], previewKind, openedCards = 0;
  media.instance.shown();
  var playable = [['mp4', 'video'], ['webm', 'video'], ['mov', 'video'], ['ogv', 'video'], ['m4v', 'video'],
    ['mp3', 'audio'], ['wav', 'audio'], ['ogg', 'audio'], ['oga', 'audio'], ['flac', 'audio'], ['m4a', 'audio'], ['aac', 'audio'], ['opus', 'audio'],
    ['png', 'image'], ['jpg', 'image'], ['jpeg', 'image'], ['gif', 'image'], ['webp', 'image'], ['avif', 'image'], ['bmp', 'image'], ['ico', 'image']];
  var binaries = ['file.mkv', 'file.avi', 'file.zip', 'file.exe', 'unknown.custom'];
  var otherPreviews = [['report.pdf', { kind: 'pdf', src: '/report.pdf' }], ['main.go', { kind: 'text', text: 'package main' }],
    ['readme.md', { kind: 'markdown', html: '<p>readme</p>' }], ['change.diff', { kind: 'diff', diff: [] }], ['page.html', { kind: 'frame', frame: '/cap/page.html' }]];
  var mediaEntries = playable.map(([ext]) => ({ name: 'play.' + ext })).concat(binaries.map(name => ({ name })), otherPreviews.map(([name]) => ({ name })), [{ name: 'large.txt' }]);
  await answer(take('media', 'list'), listing('/media', mediaEntries, true));
  Object.assign(win.WsCards, {
    register(name, kind) { previewKind = kind; }, list() { return previewCards; }, relayout() {},
    open(kind, data) {
      openedCards++;
      var card = { data, el: doc.body.appendChild(new El('section', doc)), body: new El('div', doc),
        title(v) { this.head = v; }, actions() {}, paint() {}, bump() {}, fold() {}, show() {}, save() {},
        close() { previewKind.close(this); previewCards.splice(previewCards.indexOf(this), 1); this.el.parentNode.removeChild(this.el); } };
      card.name = card.el.appendChild(new El('span', doc));
      card.el.appendChild(card.body); previewCards.push(card); previewKind.open(card); return card;
    }
  });
  doc.documentElement.setAttribute('data-rid', 'media');
  doc.documentElement.setAttribute('data-sysopen', '');
  doc.dispatchEvent = e => event(doc, e.type);
  new Function('document', 'window', 'wsT', 'fetch', PREVIEW)(doc, win, k => k, fetchControlled);
  function visibleMediaRows() {
    media.at('.fdirectory').getBoundingClientRect = () => ({ left: 10, right: 510, top: 10, bottom: 600 });
    media.all('.fentry').forEach(row => row.getBoundingClientRect = () => ({ left: 20, right: 490, top: 30, bottom: 50 }));
  }
  function activateMedia(path, keyboard) {
    visibleMediaRows();
    var row = media.at('[data-entry="' + path + '"]');
    if (keyboard) key(row, 'Enter'); else event(row, 'dblclick');
  }
  for (var path of binaries) {
    activateMedia(path, path === 'file.exe');
    await answer(take('media', 'preview'), { kind: 'none', reason: 'localized binary reason', reason_code: 'binary' });
    var menu = doc.querySelector('.fmenu');
    check(path + ' activates row menu without ever creating a preview', [openedCards, previewCards.length, !!menu, menu.style.left, menu.style.top], [0, 0, true, '20px', '54px']);
    check(path + ' retains existing system and copy actions', menu.querySelectorAll('button').map(n => n.textContent), ['js.menu.preview', 'js.preview.openext', 'js.files.menu.reveal', 'js.menu.copyrel', 'js.menu.copyabs']);
    key(menu, 'Escape');
  }
  for (var [ext, kind] of playable) {
    activateMedia('play.' + ext);
    var request = take('media', 'preview');
    await answer(request, { kind, src: '/s/media/fs/raw?path=play.' + ext });
    var card = previewCards[0], player = card.body.querySelector(kind === 'image' ? 'img' : kind);
    check(ext + ' opens native preview from preflight with no duplicate fetch', [!!player, requests.filter(r => !r.taken && r.url.pathname === '/s/media/fs/preview').length, !!doc.querySelector('.fmenu')], [true, 0, false]);
    if (kind !== 'image') {
      event(player, 'error');
      check(ext + ' playback error closes preview and opens row menu', [previewCards.length, !!doc.querySelector('.fmenu'), card.turn], [0, true, -1]);
      key(doc.querySelector('.fmenu'), 'Escape');
    } else card.close();
  }
  for (var [name, previewAnswer] of otherPreviews) {
    activateMedia(name);
    await answer(take('media', 'preview'), previewAnswer);
    check(name + ' retains existing preview kind', [previewCards[0].el.getAttribute('data-preview-kind'), !!doc.querySelector('.fmenu')], [previewAnswer.kind === 'text' ? 'code' : previewAnswer.kind, false]);
    previewCards[0].close();
  }
  activateMedia('large.txt');
  await answer(take('media', 'preview'), { kind: 'none', reason: 'too large' });
  check('too-large text retains its existing fallback preview', [previewCards[0].body.querySelector('.pv-none') !== null, !!doc.querySelector('.fmenu')], [true, false]);
  previewCards[0].close();
  activateMedia('play.mp3');
  await answer(take('media', 'preview'), { kind: 'audio', src: '/s/media/fs/raw?path=play.mp3' });
  var offscreen = previewCards[0];
  check('preflight answer and menu callback never enter saved data', offscreen.data, { path: 'play.mp3' });
  previewKind.open(offscreen);
  await answer(take('media', 'preview'), { kind: 'audio', src: '/s/media/fs/raw?path=play.mp3' });
  var oldPlayer = offscreen.body.querySelector('audio');
  media.at('[data-entry="play.mp3"]').getBoundingClientRect = () => ({ left: 20, right: 490, top: -100, bottom: -80 });
  event(oldPlayer, 'error');
  check('offscreen row keeps localized fallback instead of a misplaced menu', [previewCards.length, offscreen.body.querySelector('.pv-none') !== null, !!doc.querySelector('.fmenu')], [1, true, false]);
  offscreen.close();
  activateMedia('play.mov');
  await answer(take('media', 'preview'), { kind: 'video', src: '/s/media/fs/raw?path=play.mov' });
  var removed = previewCards[0], removedPlayer = removed.body.querySelector('video');
  var removedRow = media.at('[data-entry="play.mov"]');
  removedRow.parentNode.removeChild(removedRow);
  event(removedPlayer, 'error');
  check('removed row keeps fallback without closing preview', [previewCards.length, removed.body.querySelector('.pv-none') !== null, !!doc.querySelector('.fmenu')], [1, true, false]);
  removed.close();
  activateMedia('file.zip'); var supersededPreview = take('media', 'preview');
  activateMedia('play.mp4'); var currentPreview = take('media', 'preview');
  await answer(currentPreview, { kind: 'video', src: '/current.mp4' });
  await answer(supersededPreview, { kind: 'none', reason_code: 'binary' });
  check('late binary preflight cannot replace a newer preview with a menu', [previewCards.length, !!doc.querySelector('.fmenu')], [1, false]);
  previewCards[0].close();
  media.q.value = 'unknown'; event(media.q, 'input'); await timers();
  await answer(take('media', 'find'), { items: [{ path: 'deep/unknown.custom' }] });
  media.hits.getBoundingClientRect = () => ({ left: 10, right: 510, top: 10, bottom: 600 });
  media.at('.hit').getBoundingClientRect = () => ({ left: 20, right: 490, top: 30, bottom: 50 });
  event(media.hits, 'click', { target: media.at('.hit') });
  await answer(take('media', 'preview'), { kind: 'none', reason_code: 'binary' });
  check('binary search hit uses its visible row menu, not a preview', [previewCards.length, !!doc.querySelector('.fmenu')], [0, true]);
  key(doc.querySelector('.fmenu'), 'Escape'); key(media.q, 'Escape');
  activateMedia('file.zip'); var destroyedPreview = take('media', 'preview');
  media.instance.destroy();
  await answer(destroyedPreview, { kind: 'none', reason_code: 'binary' });
  check('destroyed explorer cannot open a late preview or menu', [previewCards.length, !!doc.querySelector('.fmenu')], [0, false]);
  process.stdout.write(bad.join('\n'));
})().catch(e => { console.error(e); process.exitCode = 1; });
