// Runs production picker/core logic with controlled promises, not a visual check.
async function workspacePickerChecks() {
  var roots = { host: 'daemon-host', home: '/home/me', volumes: ['/Volumes/Data'] };
  var choices = [{ name: 'project space', dir: true }, { name: '.hidden', dir: true }, { name: 'ignored', dir: true, ign: true },
    { name: 'file.png' }, { name: 'link', dir: true, link: true }, { name: 'gone', dir: true, gone: true }];
  function entry(container, path) { return container.querySelectorAll('.fentry').find(n => n.getAttribute('data-entry') === path); }
  var changes = [], picker = explorer('host0', true, { directoriesOnly: true, routeBase: '/s/host0/', roots,
    location: { root: roots.home }, onNavigate: s => changes.push(s), onSelection: s => changes.push(s) });
  picker.instance.shown();
  var first = take('host0', 'list');
  check('picker always lists through the absolute anchor with an explicit root', [first.url.pathname, first.url.searchParams.get('root'), picker.instance.selection()], ['/s/host0/fs/list', roots.home, null]);
  await answer(first, listing(roots.home, choices));
  check('every discovered root is labeled with its daemon host', picker.all('.froot-choices option').map(n => [n.value, n.textContent]), [[roots.home, 'daemon-host · /home/me'], ['/Volumes/Data', 'daemon-host · /Volumes/Data']]);
  check('picker omits files and guarded entries, including ignored directories', [picker.all('.fentry').map(n => n.getAttribute('data-entry')), picker.all('.fr[data-path]').map(n => n.getAttribute('data-path'))], [['project space', '.hidden', 'ignored'], ['project space', '.hidden', 'ignored']]);
  check('hidden directories are marked in both core views', [picker.at('[data-entry=".hidden"]').classList.contains('fdot'), picker.at('[data-path=".hidden"]').classList.contains('fdot')], [true, true]);
  check('picker has no file search, root override or open-as-card entry', [picker.at('.fsr').hidden, picker.at('.fsr').contains(picker.at('.fseg')), picker.at('.fchange-root').disabled, picker.at('.fopen-card').hidden], [true, true, true, true]);
  var before = requests.length;
  picker.all('.fview-modes button')[3].click();
  event(entry(picker.el, 'project space'), 'click');
  check('picker selects an absolute host path', picker.instance.selection().absolute, '/home/me/project space');
  picker.q.value = 'file'; event(picker.q, 'input'); await timers(); slash(picker.tree);
  check('gallery and synthetic search never request file content', [requests.length, picker.all('.fthumb').length, picker.previews], [before, 0, []]);
  picker.instance.newTab({ root: '/Volumes/Data', selection: 'file.png' });
  check('switching root retires the old selection immediately', [picker.instance.selection(), changes[changes.length - 1]], [null, null]);
  await answer(take('host0', 'list'), listing('/Volumes/Data', choices));
  check('persisted file selection cannot be returned by the picker', picker.instance.selection().absolute, '/Volumes/Data');
  var selectRoots = picker.at('.froot-choices'); selectRoots.value = roots.home; event(selectRoots, 'change');
  check('pending shortcut cannot select under the previous root label', picker.instance.selection(), null);
  await answer(take('host0', 'list'), listing(roots.home, choices));
  check('root shortcut uses the same core and explicit host root', picker.instance.location().root, roots.home);
  selectRoots.value = '/Volumes/Data'; event(selectRoots, 'change'); await refuse(take('host0', 'list'));
  check('refused shortcut restores the prior root and its selection', [selectRoots.value, picker.instance.selection().absolute], [roots.home, roots.home]);
  picker.instance.newTab({ root: '/missing' }); await refuse(take('host0', 'list'));
  check('anchor refusal cannot fall back to an implicit session root', [picker.instance.selection(), requests.filter(r => !r.taken && r.url.pathname === '/s/host0/fs/list').length], [null, 0]);
  picker.instance.newTab({ root: '/closing' }); var late = take('host0', 'list');
  picker.instance.destroy(); await answer(late, listing('/closing', choices));
  check('picker teardown rejects late listings', picker.el.children.length, 0);
  var windows = explorer('host0', true, { directoriesOnly: true, routeBase: '/s/host0/', location: { root: 'C:\\' } });
  windows.instance.shown();
  await answer(take('host0', 'list'), Object.assign(listing('C:\\', choices), { sep: '\\' }));
  entry(windows.el, 'project space').click();
  check('picker preserves the daemon separator for Windows drive roots', windows.instance.selection().absolute, 'C:\\project space');
  windows.instance.destroy();

  for (var winPaths of [true, false]) {
    var discovered = winPaths ? { host: 'fixture-host', home: 'C:\\Users\\u', volumes: ['C:\\', 'D:\\'] }
      : { host: 'fixture-host', home: '/home/u', volumes: ['/', '/mnt/data'] };
    var nav = explorer('host0', true, { directoriesOnly: true, roots: discovered, location: { root: discovered.home } });
    function hostListing(root) {
      return Object.assign(listing(root, [{ name: 'child', dir: true }]), {
        root: root.replace(/\\/g, '/'), abs: root, sep: winPaths ? '\\' : '/', dir: ''
      });
    }
    nav.instance.shown(); await answer(take('host0', 'list'), hostListing(discovered.home));
    var up = nav.at('.fbar-ctl').children.filter(n => n.tagName === 'BUTTON')[2];
    check('local picker can leave home, and native dropdown remains selected', [up.disabled, nav.at('.froot-choices').value], [false, discovered.home]);
    var refusedSelect = nav.at('.froot-choices'); refusedSelect.value = discovered.volumes[1]; event(refusedSelect, 'change');
    await refuse(take('host0', 'list'));
    check('refused volume restores the native home option and selection', [refusedSelect.value, nav.instance.selection().absolute], [discovered.home, discovered.home]);
    up.click(); var parentReq = take('host0', 'list');
    check('parent navigation uses a validated explicit root, never dot-dot', [parentReq.url.searchParams.get('root'), parentReq.url.searchParams.get('dir'), nav.instance.selection()], [winPaths ? 'C:/Users' : '/home', null, null]);
    await answer(parentReq, hostListing(winPaths ? 'C:\\Users' : '/home'));
    check('intermediate parent is labeled in the dropdown', nav.at('.froot-choices').value, winPaths ? 'C:\\Users' : '/home');
    up.click(); await answer(take('host0', 'list'), hostListing(discovered.volumes[0]));
    check('up stops only at the filesystem root while Home stays available', [up.disabled, nav.at('.fhome').disabled, nav.instance.selection().absolute], [true, false, discovered.volumes[0]]);
    nav.at('.fhome').click(); var homeReq = take('host0', 'list');
    check('Home crosses roots in one click', homeReq.url.searchParams.get('root'), discovered.home);
    await answer(homeReq, hostListing(discovered.home));
    var rootSelect = nav.at('.froot-choices'); rootSelect.value = discovered.volumes[1]; event(rootSelect, 'change');
    await answer(take('host0', 'list'), hostListing(discovered.volumes[1]));
    nav.at('[data-entry="child"]').click();
    check('alternate volume selection keeps the native absolute path', nav.instance.selection().absolute, winPaths ? 'D:\\child' : '/mnt/data/child');
    nav.instance.destroy();
  }
  for (var scope of [{ rid: 'session-picker', local: true }, { rid: 'host0', local: false }]) {
    var confined = explorer(scope.rid, scope.local, { directoriesOnly: true, roots, location: { root: roots.home } });
    confined.instance.shown(); await answer(take(scope.rid, 'list'), listing(roots.home, choices));
    var confinedUp = confined.at('.fbar-ctl').children.filter(n => n.tagName === 'BUTTON')[2];
    check('session and nonlocal pickers do not acquire ancestor navigation', [confinedUp.disabled, confined.at('.fhome').disabled], [true, true]);
    confined.instance.destroy();
  }

  // Minimal popover/route seams; the actual rpWsMake/rpWsBrowse and core run.
  El.prototype.showModal = function () { this.open = true; };
  El.prototype.close = function () { this.open = false; };
  El.prototype.remove = function () { if (this.parentNode) this.parentNode.removeChild(this); };
  function mk(tag, cls, text) { var n = new El(tag, doc); n.className = cls || ''; if (text != null) n.textContent = text; return n; }
  function btn(cls, text) { var b = mk('button', cls, text); b.type = 'button'; return b; }
  var posts = [], live = null, closed = 0;
  function post(route, body, done) { posts.push({ route, body, done }); }
  var make = new Function('doc', 'window', 'root', 'mk', 'wsT', 'rpPop', 'rpBtn', 'rpInput', 'rpId', 'rpEnter', 'railPost', 'railErr', 'rpLive', 'railPopClose', 'toast', 'setTimeout', 'clearTimeout', 'fetch', PICKER + '\nreturn rpWsMake;')(
    doc, win, doc.documentElement, mk, k => k, () => mk('div', 'rpop'), btn, cls => mk('input', cls), () => 'test-id', () => {}, post,
    (status, text) => text || String(status), (p, gen) => live === p && p.gen === gen,
    () => { live.close(); live = null; closed++; }, () => {}, later, cancel, fetchControlled);
  function open(local) {
    if (local) doc.documentElement.setAttribute('data-sysopen', '1'); else doc.documentElement.removeAttribute('data-sysopen');
    var p = live = make(); doc.body.appendChild(p.el); p.open(); return p;
  }
  function browse(p) { return p.el.querySelectorAll('.spline button').find(n => n.textContent === 'js.rail.ws.browse'); }
  var remote = open(false);
  check('remote viewers retain paste/add without Browse', [!!remote.el.querySelector('.spq'), !!remote.el.querySelector('.go'), !!browse(remote)], [true, true, false]);
  remote.close(); remote.el.remove();
  var p = open(true), inp = p.el.querySelector('.spq'); inp.value = '/pasted/untouched';
  browse(p).click(); var abandoned = take('host0', 'roots');
  p.picker.el.querySelector('.fattachment button').click();
  await answer(abandoned, roots);
  check('Cancel before discovery leaves paste and preview alone', [p.picker, inp.value, posts.length], [null, '/pasted/untouched', 0]);
  browse(p).click(); await answer(take('host0', 'roots'), roots);
  var initialRoot = take('host0', 'list');
  check('consumer mounts the shared core at discovered home', [initialRoot.url.searchParams.get('root'), p.picker.el.querySelector('.fattachment .pr').disabled], [roots.home, true]);
  await answer(initialRoot, listing(roots.home, choices));
  var modal = p.picker.el;
  entry(modal, 'project space').click();
  modal.querySelectorAll('.fattachment button')[1].click();
  check('Cancel after browsing is still a no-op for paste/preview', [p.picker, inp.value, posts.length, modal.parentNode], [null, '/pasted/untouched', 0, null]);
  browse(p).click(); await answer(take('host0', 'roots'), roots); await answer(take('host0', 'list'), listing(roots.home, choices));
  modal = p.picker.el; entry(modal, 'project space').click(); modal.querySelector('.fattachment .pr').click();
  await timers();
  check('Select feeds the unchanged preview flow, never Add directly', [p.picker, inp.value, posts.map(r => [r.route, r.body])], [null, '/home/me/project space', [['/rail/workspaces/preview', { path: '/home/me/project space' }]]]);
  posts[0].done(200, { ok: true, root: '/home/me/project space' }, '');
  p.el.querySelector('.go').click();
  check('existing Add submits the selected directory', [posts[1].route, posts[1].body], ['/rail/workspaces/add', { path: '/home/me/project space' }]);
  posts[1].done(200, { message: 'added' }, '');
  check('successful Add keeps the existing close lifecycle', closed, 1);
  p.el.remove();
  p = open(true); inp = p.el.querySelector('.spq'); inp.value = '/fallback';
  browse(p).click(); var staleRoots = take('host0', 'roots');
  p.close(); await answer(staleRoots, roots);
  check('closing the owning popover retires pending discovery', [p.picker, requests.filter(r => !r.taken && r.url.pathname === '/s/host0/fs/list').length], [null, 0]);
  browse(p).click(); await answer(take('host0', 'roots'), roots);
  var staleList = take('host0', 'list');
  check('Escape first closes only the picker', [p.back(), !!p.picker, p.back()], [true, false, false]);
  await answer(staleList, listing(roots.home, choices));
  check('late consumer listing cannot reattach the modal', p.el.querySelectorAll('.fworkspace-picker').length, 0);
  browse(p).click(); await refuse(take('host0', 'roots'));
  check('discovery refusal preserves fallback paste input', [inp.value, p.picker.el.querySelector('.fnote').textContent], ['/fallback', 'js.files.failed']);
  p.close(); p.el.remove();
}
