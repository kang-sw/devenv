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
function listing(root, entries, session) { return { root, name: root.split('/').pop(), abs: root, sep: '/', session: !!session, entries: entries || [{ name: 'src', dir: true }, { name: 'main.go' }, { name: '.env' }, { name: 'ignored', ign: true }] }; }
function event(n, type, extra) { var e = Object.assign({ target: n, preventDefault() { this.defaultPrevented = true; }, stopPropagation() {} }, extra); n.fire(type, e); return e; }
function key(n, key) { return event(n, 'keydown', { key }); }
function slash(n) { var e = { target: n, key: '/', preventDefault() { this.defaultPrevented = true; } }; doc.listeners.keydown.slice().forEach(f => f(e)); }
var doc = new Doc(), win = { WsCards: { icon: () => '' }, AbortController, innerWidth: 1200, innerHeight: 900, listeners: {}, addEventListener: El.prototype.addEventListener, removeEventListener: El.prototype.removeEventListener };
new Function('document', 'window', 'wsT', 'wsTn', 'fetch', 'AbortController', 'setTimeout', 'clearTimeout', FILES)(doc, win, k => k, k => k, fetchControlled, AbortController, later, cancel);
function explorer(rid, local = true) {
  var el = doc.body.appendChild(new El('section', doc)); el.className = 'fpane';
  var store = {}, previews = [];
  win.WsPreview = { open: (path, o) => previews.push([path, o]) };
  var instance = win.WsFiles.create(el, { rid, local, load: k => store[k], save: (k, v) => store[k] = v });
  return { el, instance, store, previews, q: el.querySelector('.fq'), tree: el.querySelector('.ftree'), hits: el.querySelector('.fhits'), at: s => el.querySelector(s), all: s => el.querySelectorAll(s) };
}
function rootRequest(x, path) { if (x.at('.froot-pop').hidden) x.all('.fhd button')[0].click(); x.at('.fpin').value = path; x.at('.fgo .pr').click(); }
(async function () {
  var a = explorer('alpha'), b = explorer('beta');
  a.instance.shown(); b.instance.shown();
  await answer(take('beta', 'list'), listing('/beta', undefined, true));
  await answer(take('alpha', 'list'), listing('/alpha', undefined, true));
  check('out-of-order independent roots', [a.at('.froot').textContent, b.at('.froot').textContent], ['alpha', 'beta']);
  event(a.at('[data-path="src"]'), 'unused');
  event(a.tree, 'click', { target: a.at('[data-path="src"]') });
  await answer(take('alpha', 'list', 'src'), listing('/alpha/src', [{ name: 'a.go' }]));
  event(a.tree, 'click', { target: a.at('[data-path="src/a.go"]') });
  check('expansion and selection stay local', [!!a.at('[aria-selected="true"]'), !!b.at('[aria-selected="true"]'), b.at('[data-path="src"]').getAttribute('aria-expanded')], [true, false, 'false']);
  a.all('.fhd button')[3].click();
  var ign = a.all('.fpop input')[0]; ign.checked = true; event(ign, 'change');
  check('view and storage isolated', [!!a.at('[data-path="ignored"]'), !!b.at('[data-path="ignored"]'), Object.keys(a.store).length > 0, Object.keys(b.store).some(k => k.includes('.view'))], [true, false, true, false]);
  key(a.at('.fpop'), 'Escape');
  check('popover Escape returns owner focus', doc.activeElement === a.all('.fhd button')[3], true);
  event(a.tree, 'contextmenu', { target: a.at('[data-path="main.go"]'), clientX: 20, clientY: 30 });
  check('row menu opens', !!doc.querySelector('.fmenu'), true);
  key(doc.querySelector('.fmenu'), 'Escape');
  check('menu Escape returns row focus', doc.activeElement === a.at('[data-path="main.go"]'), true);
  slash(a.tree); check('target pane owns slash', doc.activeElement === a.q, true);
  b.el.setAttribute('data-max', ''); slash(doc.body); check('active sheet owns slash', doc.activeElement === b.q, true); b.el.removeAttribute('data-max');
  a.tree.scrollTop = 73; event(a.tree, 'scroll'); b.tree.scrollTop = 19; event(b.tree, 'scroll');
  a.instance.hidden(); a.instance.shown();
  await answer(take('alpha', 'list', 'src'), listing('/alpha/src', [{ name: 'a.go' }]));
  await answer(take('alpha', 'list', ''), listing('/alpha', undefined, true));
  check('reopen refresh preserves expansion selection and scroll', [a.tree.scrollTop, b.tree.scrollTop, !!a.at('[data-path="src/a.go"]'), !!a.at('[aria-selected="true"]')], [73, 19, true, true]);
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
  process.stdout.write(bad.join('\n'));
})().catch(e => { console.error(e); process.exitCode = 1; });
