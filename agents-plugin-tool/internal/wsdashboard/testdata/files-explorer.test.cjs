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
new Function('document', 'window', 'wsT', 'wsTn', 'fetch', 'AbortController', 'setTimeout', 'clearTimeout', FILES)(doc, win, k => k, k => k, fetchControlled, AbortController, later, cancel);
function explorer(rid, local = true, options = {}) {
  var el = doc.body.appendChild(new El('section', doc)); el.className = 'fpane';
  var store = options.store || {}, previews = [];
  win.WsPreview = { open: (path, o) => { var record = {}; if (o.root) record.root = o.root; if (o.line) record.line = o.line; previews.push([path, record]); } };
  var instance = win.WsFiles.create(el, Object.assign({ rid, local, load: k => store[k], save: (k, v) => store[k] = v }, options));
  return { el, instance, store, previews, q: el.querySelector('.fq'), tree: el.querySelector('.ftree'), hits: el.querySelector('.fhits'), at: s => el.querySelector(s), all: s => el.querySelectorAll(s) };
}
function rootRequest(x, path) { if (x.at('.froot-pop').hidden) x.at('.fchange-root').click(); x.at('.fpin').value = path; x.at('.fgo .pr').click(); }
(async function () {
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
  event(core.all('.fentry').find(n => n.getAttribute('data-entry') === 'photo space.png'), 'click');
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
  entry('src/pkg/first.go').click(); event(entry('src/pkg/first.go'), 'dblclick');
  await answer(take('columns', 'preview'), { kind: 'text', text: 'package main' });
  check('column files select and reuse existing preview path', [columns.instance.selection().path, entry('src/pkg/first.go').getAttribute('aria-selected'), columns.previews], ['src/pkg/first.go', 'true', [['src/pkg/first.go', {}]]]);
  key(entry('src/pkg/first.go'), 'ArrowDown');
  check('down selects next entry without changing folder', [columns.instance.selection().path, doc.activeElement.getAttribute('data-entry'), columns.instance.location().dir], ['src/pkg/last.go', 'src/pkg/last.go', 'src/pkg']);
  key(entry('src/pkg/last.go'), 'ArrowUp'); key(entry('src/pkg/first.go'), 'ArrowLeft');
  check('up and left move within and between columns', [columns.instance.selection().path, doc.activeElement.getAttribute('data-entry')], ['src/pkg/first.go', 'src/pkg']);
  key(entry('src/pkg'), 'ArrowRight');
  check('right focuses the selected child column entry', doc.activeElement.getAttribute('data-entry'), 'src/pkg/first.go');
  key(entry('src/pkg/first.go'), 'Enter');
  await answer(take('columns', 'preview'), { kind: 'text', text: 'package main' });
  check('Enter previews selected file', columns.previews.length, 2);
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
  check('selecting ancestor file drops trailing columns without preview on single click', [columnPaths(), columns.instance.location().dir, columns.instance.selection().path, columns.previews.length], [[''], '', 'README.md', 0]);
  entry('linked').click(); event(entry('linked'), 'dblclick');
  entry('gone').click(); event(entry('gone'), 'dblclick');
  check('symlink and gone folders stay selectable but never open or preview', [columnPaths(), columns.instance.selection().path, columns.previews.length, requests.filter(r => !r.taken && r.url.pathname === '/s/columns/fs/list').length], [[''], 'gone', 0, 0]);
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
  var searchPreviews = []; win.WsPreview = { open: (path, o) => { var record = {}; if (o.root) record.root = o.root; if (o.line) record.line = o.line; searchPreviews.push([path, record]); } };
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
    register(name, kind) { previewKind = kind; }, list() { return previewCards; },
    open(kind, data) {
      openedCards++;
      var card = { data, el: doc.body.appendChild(new El('section', doc)), body: new El('div', doc),
        title(v) { this.head = v; }, actions() {}, paint() {}, bump() {}, fold() {}, show() {}, save() {},
        close() { previewKind.close(this); previewCards.splice(previewCards.indexOf(this), 1); this.el.parentNode.removeChild(this.el); } };
      card.el.appendChild(card.body); previewCards.push(card); previewKind.open(card); return card;
    }
  });
  doc.documentElement.setAttribute('data-rid', 'media');
  doc.documentElement.setAttribute('data-sysopen', '');
  new Function('document', 'window', 'wsT', 'fetch', PREVIEW)(doc, win, k => k, fetchControlled);
  function visibleMediaRows() {
    media.at('.fdirectory').getBoundingClientRect = () => ({ left: 10, right: 510, top: 10, bottom: 600 });
    media.all('.fentry').forEach(row => row.getBoundingClientRect = () => ({ left: 20, right: 490, top: 30, bottom: 50 }));
  }
  function activateMedia(path, keyboard) {
    visibleMediaRows();
    var row = media.at('[data-entry="' + path + '"]');
    if (keyboard) key(row, 'Enter'); else { row.click(); event(row, 'dblclick'); }
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
