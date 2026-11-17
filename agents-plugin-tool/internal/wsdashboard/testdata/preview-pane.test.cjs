// outsideDOM comes from preview_outside_test.go; execute the complete renderer.
var kinds = {}, cards = [], requests = [], effects = [], closed = 0, registrations = 0;
function fetch(url, options) {
  return new Promise((resolve, reject) => requests.push({ url, options, resolve, reject }));
}
var window = { navigator: { pdfViewerEnabled: true }, WsCards: {
  register(name, kind) { registrations++; kinds[name] = kind; },
  icon: name => '<svg>' + name + '</svg>',
  list: () => cards,
  relayout() { effects.push('relayout'); },
  open(kind, data) {
    var card = { data, body: new El('div'), el: new El('div'),
      title(v) { this.head = v; }, actions() {}, paint() {},
      bump() { effects.push('bump'); }, fold() {}, show() { effects.push('show'); },
      save() { effects.push('save'); } };
    card.name = card.el.appendChild(new El('span'));
    cards.push(card); kinds[kind].open(card); return card;
  }
} };
// Model containment by the live tree, including detached rendered content.
El.prototype.contains = function (node) { return node === this || this.children.some(child => child.contains(node)); };
eval(PREVIEW);
async function settle() { for (var i = 0; i < 12; i++) await Promise.resolve(); }
async function answer(r, body) { r.resolve({ ok: true, json: () => body }); await settle(); }
(async function () {
  var api = window.WsPreview;
  var durable = api.open('same.md', { keep: true, answer: { kind: 'markdown', html: '<p>board</p>' } });
  var pinned = api.open('pinned.txt', { keep: true, answer: { kind: 'text', text: 'pinned' } }); pinned.pinned = true;
  var transient = api.open('slot.txt', { answer: { kind: 'text', text: 'slot' } });
  await settle();
  var data = cards.map(c => c.data), bodies = cards.map(c => c.body.firstChild);
  effects = [];
  var focusAtClose = [];
  var el = new El('section'), pane = api.createPane(el, hadFocus => { closed++; effects.push('closed'); focusAtClose.push(hadFocus); });
  var header = el.children[0], body = el.children[1], keep = header.children[2], exit = header.children[3];
  check('pane starts memory-only and hidden without registering another kind', [pane.current(), pane.close(), el.hidden, closed, registrations], [null, null, true, 0, 1]);
  var options = { root: '/other', line: 3, answer: { kind: 'text', text: 'one\ntwo\nthree' } };
  pane.open('src/same.md', options); await settle();
  check('pane uses shared text renderer and line navigation', [find(body, 'PRE').textContent, body.children[0].children[0].children[1].style.top, el.attrs['data-preview-kind']], ['one\ntwo\nthree', '36px', 'code']);
  check('pane title contains basename and full path', [header.children[0].textContent, header.children[1].textContent, header.children[1].title], ['same.md', 'src/same.md', 'src/same.md']);
  check('pane retains caller options without network refetch of preflight', [pane.current().path, pane.current().options === options, el.hidden, requests.length], ['src/same.md', true, false, 0]);
  for (var path of ['same.md', 'pinned.txt', 'slot.txt']) {
    pane.open(path, { answer: { kind: 'image', src: '/raw/' + path } }); await settle();
    check('pane independently renders even an existing card: ' + path, find(body, 'IMG').src, '/raw/' + path);
  }
  check('existing durable pinned and transient cards untouched', [cards.length, cards.every((c, i) => c.data === data[i] && c.body.firstChild === bodies[i]), cards.map(c => [!!c.preview, !!c.pinned]), effects], [3, true, [[false, false], [false, true], [true, false]], []]);

  pane.open('old.txt'); var old = requests.at(-1);
  pane.open('new.txt', { root: '/picked', line: 7 }); var fresh = requests.at(-1);
  check('pane load uses guarded root and text query', [fresh.url, fresh.options.credentials, fresh.options.cache], ['/s/rid1/fs/preview?path=new.txt&root=%2Fpicked&text=1', 'same-origin', 'no-store']);
  check('replacement clears previous body while pending', body.children.length, 0);
  await answer(fresh, { kind: 'text', text: 'new' });
  await answer(old, { kind: 'text', text: 'old' });
  check('late replaced answer cannot repaint pane', [find(body, 'PRE').textContent, pane.current().path], ['new', 'new.txt']);
  pane.open('late-close.txt'); var lateClose = requests.at(-1);
  var pendingOptions = pane.current().options, previous = pane.close();
  check('close clones options even without a preflight answer', previous.options === pendingOptions, false);
  await answer(lateClose, { kind: 'image', src: '/stale.png' });
  check('close returns path/options once and invalidates pending answer', [previous.path, previous.options, pane.current(), pane.close(), body.children.length, el.hidden, closed], ['late-close.txt', {}, null, null, 0, true, 1]);
  pane.open('before-reopen.txt'); var beforeReopen = requests.at(-1); pane.close();
  pane.open('after-reopen.txt'); var afterReopen = requests.at(-1);
  await answer(afterReopen, { kind: 'text', text: 'reopened' });
  await answer(beforeReopen, { kind: 'text', text: 'retired' });
  check('close/reopen never reuses an async generation', find(body, 'PRE').textContent, 'reopened');
  pane.open('stale-error.txt'); var staleError = requests.at(-1);
  pane.open('current.txt', { answer: { kind: 'text', text: 'current' } }); await settle();
  staleError.reject(404); await settle();
  check('stale rejection cannot replace current content', find(body, 'PRE').textContent, 'current');
  pane.open('missing.txt'); requests.at(-1).resolve({ ok: false, status: 404 }); await settle();
  check('pane shares missing-file fallback', body.textContent, 'js.preview.missing');

  pane.open('unsafe.svg', { answer: { kind: 'frame', frame: '/cap/svg' } }); await settle();
  check('pane shares opaque-origin frame sandbox', [find(body, 'IFRAME').attrs.sandbox, find(body, 'IFRAME').attrs.referrerpolicy], ['allow-scripts', 'no-referrer']);
  pane.open('song.mp3', { answer: { kind: 'audio', src: '/raw/song' } }); await settle();
  var player = find(body, 'AUDIO'); pane.close();
  pane.open('after-media.txt', { answer: { kind: 'text', text: 'safe' } }); await settle(); player.on.error();
  check('retired player cannot repaint reopened pane', find(body, 'PRE').textContent, 'safe');
  var prevented = false, stopped = false;
  el.on.keydown({ key: 'Enter', preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } });
  check('non-Escape keys leave pane open', [!!pane.current(), prevented, stopped], [true, false, false]);
  el.on.keydown({ key: 'Escape', target: keep, preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } });
  check('Escape from pane controls closes and consumes event', [pane.current(), el.hidden, prevented, stopped], [null, true, true, true]);
  pane.open('button.txt', { answer: { kind: 'text', text: 'close' } }); await settle(); exit.on.click();
  check('close button dismisses pane', [pane.current(), el.hidden], [null, true]);

  pane.open('content-focus.txt', { answer: { kind: 'text', text: 'focused content' } }); await settle();
  document.activeElement = find(body, 'PRE');
  el.on.keydown({ key: 'Escape', target: document.activeElement, preventDefault() {}, stopPropagation() {} });
  check('Escape snapshots focus before removing rendered content', [focusAtClose.at(-1), el.contains(document.activeElement), body.children.length], [true, false, 0]);
  var expired = { kind: 'frame', frame: '/cap/expired' }, transferOptions = { root: '/other', line: 4, answer: expired };
  pane.open('transfer.html', transferOptions); await settle();
  var transfer = pane.close();
  check('surface transfer drops stale answer from cloned options, preserving source', [transfer, transfer.options === transferOptions, transferOptions.answer === expired], [{ path: 'transfer.html', options: { root: '/other', line: 4 } }, false, true]);

  var nativeOpen = api.open, promotion = [], promoteOptions = { root: '/other', line: 9, answer: { kind: 'frame', frame: '/cap/old-keep' } };
  api.open = function (path, o) { promotion.push([path, o.keep, o.root, o.line, pane.current(), el.hidden]); return nativeOpen(path, o); };
  effects = [];
  pane.open('keep.html', promoteOptions); await settle();
  var beforeKeepRequests = requests.length; keep.on.click(); await settle();
  check('keep makes fresh guarded request instead of reusing old sandbox capability', [requests.length - beforeKeepRequests, requests.at(-1).url, find(cards.at(-1).body, 'IFRAME')], [1, '/s/rid1/fs/preview?path=keep.html&root=%2Fother&text=1', null]);
  await answer(requests.at(-1), { kind: 'frame', frame: '/cap/fresh-keep' });
  check('keep closes first then uses existing durable card path with fresh reply', [promotion, effects[0], cards.at(-1).preview, cards.at(-1).data, find(cards.at(-1).body, 'IFRAME').src], [[['keep.html', true, '/other', 9, null, true]], 'closed', false, { path: 'keep.html', root: '/other', line: 9 }, '/cap/fresh-keep']);
  check('keep does not mutate original options or persist preflight state', [promoteOptions.keep, promoteOptions.answer.frame, cards.at(-1).data.answer, cards.at(-1).data.mediaError], [undefined, '/cap/old-keep', undefined, undefined]);
  keep.on.click(); check('keep on a closed pane is inert', promotion.length, 1);
  process.stdout.write(bad.join('\n'));
})().catch(e => { console.error(e); process.exitCode = 1; });
