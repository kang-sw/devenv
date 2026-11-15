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
var doc = new Doc(), win = { WsCards: { icon: () => '' }, AbortController, innerWidth: 1200, innerHeight: 900, listeners: {}, addEventListener: El.prototype.addEventListener, removeEventListener: El.prototype.removeEventListener };
new Function('document', 'window', 'wsT', 'wsTn', 'fetch', 'AbortController', 'setTimeout', 'clearTimeout', FILES)(doc, win, k => k, k => k, fetchControlled, AbortController, later, cancel);
function explorer(rid, local = true, options = {}) {
  var el = doc.body.appendChild(new El('section', doc)); el.className = 'fpane';
  var store = options.store || {}, previews = [];
  win.WsPreview = { open: (path, o) => previews.push([path, o]) };
  var instance = win.WsFiles.create(el, Object.assign({ rid, local, load: k => store[k], save: (k, v) => store[k] = v }, options));
  return { el, instance, store, previews, q: el.querySelector('.fq'), tree: el.querySelector('.ftree'), hits: el.querySelector('.fhits'), at: s => el.querySelector(s), all: s => el.querySelectorAll(s) };
}
function rootRequest(x, path) { if (x.at('.froot-pop').hidden) x.all('.fhd button')[0].click(); x.at('.fpin').value = path; x.at('.fgo .pr').click(); }
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
  check('out-of-order independent roots', [a.at('.froot').textContent, b.at('.froot').textContent], ['alpha', 'beta']);
  event(a.at('[data-path="src"]'), 'unused');
  event(a.tree, 'click', { target: a.at('[data-path="src"]') });
  await answer(take('alpha', 'list', 'src'), listing('/alpha/src', [{ name: 'a.go' }]));
  event(a.at('[data-entry="src/a.go"]'), 'click');
  check('expansion and selection stay local', [a.instance.selection().path, b.instance.selection().path, b.at('[data-path="src"]').getAttribute('aria-expanded')], ['src/a.go', '.', 'false']);
  a.all('.fhd button')[3].click();
  var ign = a.all('.fpop input')[0]; ign.checked = true; event(ign, 'change');
  check('view and storage isolated', [!!a.at('[data-path="ignored"]'), !!b.at('[data-path="ignored"]'), Object.keys(a.store).length > 0, Object.keys(b.store).some(k => k.includes('.view'))], [true, false, true, false]);
  key(a.at('.fpop'), 'Escape');
  check('popover Escape returns owner focus', doc.activeElement === a.all('.fhd button')[3], true);
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
  check('remote UI cannot offer change-root or restore local root', [remote.all('.fhd button')[0].hidden, rr.url.searchParams.has('root')], [true, false]);
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

  var legacy = explorer('legacy', true, { store: { 'ws.dashboard.files.root.legacy': '/other', 'ws.dashboard.files.view': JSON.stringify({ '/other': { ign: true } }) }, send: p => sent.push(p), copy: p => copied.push(p) });
  legacy.instance.shown(); var lr = take('legacy', 'list', '');
  check('legacy root record still loads', lr.url.searchParams.get('root'), '/other');
  await answer(lr, listing('/other', [{ name: 'ignored.png', ign: true }]));
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
  check('extended core leaves no global handlers or timers', [doc.listeners.keydown.length, win.listeners.resize.length, pendingTimers.size], [0, 0, 0]);
  process.stdout.write(bad.join('\n'));
})().catch(e => { console.error(e); process.exitCode = 1; });
