// Offline source/behavior contracts: no server, child process or ws binary.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const { readAssetSource: read } = require('./asset-source.cjs');
const terminal = read('wsdashboard', 'terminal.js');
function fn(source, name) {
  const start = source.indexOf('  function ' + name + '(');
  assert(start >= 0, name);
  return source.slice(start, source.indexOf('\n  }', start) + 4);
}
const tick = async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); };

test('terminal list shares only concurrent requests, including failure/retry', async () => {
  const requests = [], results = [];
  const c = vm.createContext({ worktree: '', listWait: null, folders: null, route: x => '/terminals' + x, wsT: k => k,
    fetch(url) { return new Promise((resolve, reject) => requests.push({ url, resolve, reject })); }
  });
  vm.runInContext(fn(terminal, 'list'), c);
  c.list((err, v) => results.push([err, v])); c.list((err, v) => results.push([err, v]));
  assert.equal(requests.length, 1);
  const data = {terminals: [{id: 'one'}], folders: {root: '/p'}};
  requests[0].resolve({ok: true, json: async () => data}); await tick();
  assert.equal(results.length, 2); assert.equal(results[0][1], data); assert.equal(results[1][1], data);
  c.list((err, v) => results.push([err, v])); assert.equal(requests.length, 2, 'completed results are never cached');
  requests[1].reject(Error('offline')); await tick();
  assert.equal(results.at(-1)[0], 'js.post.unreachable');
  c.list(() => {}); assert.equal(requests.length, 3, 'failed read releases the in-flight slot');
});

test('off-screen terminal delays backlog; near folded cards keep their preview', async () => {
  const observed = [], started = [];
  let emulators = 0, kind;
  const el = () => ({hidden: false, appendChild() {}, setAttribute() {}, addEventListener() {},
    classList: {contains() { return false; }}});
  const c = vm.createContext({ KIND: 'terminal', mk: el, wsT: k => k, status() {}, base() {}, closing() {},
    foldLine() {}, later() {}, maximized() { return false; }, setTimeout() {}, clearTimeout() {},
    window: {}, cards: {icon: () => '', register(_, value) { kind = value; }},
    emulator() { emulators++; return Promise.resolve(); }, start(card) { started.push(card); card.t.term = {dispose() {}}; },
    IntersectionObserver: class { constructor(cb) { this.cb = cb; observed.push(this); } observe() {} disconnect() { this.stopped = true; } }
  });
  c.window.IntersectionObserver = c.IntersectionObserver;
  vm.runInContext(fn(terminal, 'open') + fn(terminal, 'activate') + fn(terminal, 'stop') + terminal.match(/  cards.register\(KIND, \{[\s\S]*?\n  \}\);/)[0], c);
  const card = {data: {tid: 'one'}, body: el(), el: el(), folded: false};
  kind.open(card); kind.sized(card);
  assert.equal(emulators, 0, 'no font/script/emulator/socket backlog before near-view');
  observed[0].cb([{isIntersecting: false}]); assert.equal(emulators, 0);
  card.folded = true; observed[0].cb([{isIntersecting: false}]); assert.equal(emulators, 0, 'off-screen folded cards remain idle');
  observed[0].cb([{isIntersecting: true}]); kind.sized(card); await tick();
  card.folded = false; kind.sized(card);
  assert.equal(emulators, 1); assert.deepEqual(started, [card]); assert.equal(observed[0].stopped, true);
  const removed = {data: {tid: 'two'}, body: el(), el: el(), folded: false};
  kind.open(removed); kind.close(removed); observed[1].cb([{isIntersecting: true}]); await tick();
  assert.equal(emulators, 1, 'removed lazy cards never activate');
  assert.equal(observed[1].stopped, true);
  assert(terminal.includes('s.last ? s.last + 1 : 0'), 'fresh emulators still replay from zero');
});

function scrollFixture(storage = {}, pathname = '/s/a', hash = '') {
  const frames = [], listeners = {}, placements = [];
  const node = (top = 0) => ({scrollTop: top, shown: true, clientTop: 2,
    firstElementChild: {getBoundingClientRect: () => ({top: 200})}, getBoundingClientRect: () => ({top: 100})});
  const nodes = {convo: node(140), bcol: node(620), 'r-tree': node(350)};
  const doc = {scrollingElement: node(900),
    addEventListener(type, cb) { listeners[type] = cb; }, removeEventListener(type) { delete listeners[type]; }};
  const c = vm.createContext({ shell: false, framed: true, location: {pathname, hash}, PANES: ['convo', 'bcol', 'r-tree', 'bpins', 'drbody'],
    paneScroll: {}, stick: false, doc, $: id => nodes[id], visible: el => !!el?.shown,
    requestAnimationFrame(cb) { frames.push(cb); }, pinConvo() { if (c.stick) nodes.convo.scrollTop = 999; },
    sessionStorage: {getItem(k) { return storage[k] ?? null; }, setItem(k, v) { storage[k] = v; }}, addEventListener() {}
  });
  c.window = c; c.wbHistory = {keeping: el => !!el.owned, place: (ref, options) => placements.push({ref, options})};
  vm.runInContext(read('wsdashboard', 'dashboard-session-view.js'), c);
  return {c, nodes, doc, frames, listeners, placements, flush() { frames.splice(0).forEach(fn => fn()); }};
}

test('per-session scroll survives fresh frames; board uses its placement owner', () => {
  const storage = {}, first = scrollFixture(storage);
  first.c.restoreSessionView(); first.c.wsSaveSessionView();
  const other = scrollFixture(storage, '/s/b'); other.c.restoreSessionView(); other.nodes.convo.scrollTop = 12; other.c.wsSaveSessionView();
  const back = scrollFixture(storage); back.nodes.convo.scrollTop = 0;
  back.nodes.bcol.owned = true;
  Object.defineProperty(back.nodes.bcol, 'scrollTop', {get: () => 120, set() { throw Error('owned board scroll must not take direct writes'); }});
  back.c.restoreSessionView(); back.flush();
  assert.equal(back.nodes.convo.scrollTop, 140); assert.equal(back.doc.scrollingElement.scrollTop, 900);
  assert.equal(back.c.stick, false);
  assert.equal(back.placements.length, 1); assert.equal(back.placements[0].options.offset, -402);
  const hidden = scrollFixture(storage, '/s/c'); hidden.c.restoreSessionView(); hidden.nodes['r-tree'].shown = false;
  hidden.c.paneScroll['r-tree'] = 777; hidden.c.wsSaveSessionView();
  const reveal = scrollFixture(storage, '/s/c'); reveal.c.restoreSessionView(); reveal.flush();
  assert.equal(reveal.nodes['r-tree'].scrollTop, 777, 'hidden pane retains its last visible offset');
});

test('scroll restore respects hashes/input, bottom-follow, denied storage and cap', () => {
  const storage = {}, first = scrollFixture(storage); first.c.restoreSessionView(); first.c.stick = true; first.c.wsSaveSessionView();
  const hash = scrollFixture(storage, '/s/a', '#files'); hash.c.restoreSessionView(); assert.equal(hash.frames.length, 0);
  const back = scrollFixture(storage); back.c.restoreSessionView(); back.flush(); assert.equal(back.nodes.convo.scrollTop, 999);
  const input = scrollFixture(storage); input.c.restoreSessionView(); input.nodes.convo.scrollTop = 33; input.listeners.wheel(); input.flush();
  assert.equal(input.nodes.convo.scrollTop, 33, 'user input cancels startup placement');
  const denied = scrollFixture(); denied.c.sessionStorage.getItem = () => { throw Error('denied'); };
  denied.c.sessionStorage.setItem = () => { throw Error('denied'); };
  assert.doesNotThrow(() => { denied.c.restoreSessionView(); denied.c.wsSaveSessionView(); });
  for (let i = 0; i < 60; i++) { first.c.location.pathname = '/s/n' + i; first.c.wsSaveSessionView(); }
  const rows = JSON.parse(Object.values(storage)[0]); assert.equal(rows.length, 50); assert.equal(rows[0].page, '/s/n10');
});

test('framed main script uses one shared content-addressed static request', () => {
  const root = path.join(__dirname, '..');
  const pages = fs.readFileSync(path.join(root, 'pages.go'), 'utf8');
  const template = read('wsdashboard', 'dashboard.tmpl');
  assert.match(template, /\{\{if \.Frame\}\}<script src="\/s\/\{\{\.Assets.ScriptFile\}\}"><\/script>\{\{else\}\}<script>\{\{\.Assets.Script\}\}/);
  assert.match(pages, /sha256.Sum256\(\[\]byte\(a.data.Script\)\)/);
  assert(pages.includes('rest != a.data.ScriptFile'));
  new vm.Script(read('wsdashboard', 'dashboard.js'));
  const lifecycle = read('wsdashboard', 'dashboard-lifecycle.js');
  assert(lifecycle.indexOf('wsSaveSessionView();') < lifecycle.indexOf('location.replace("about:blank")'));
  assert(lifecycle.includes('  restoreSessionView();\n  nearTop();'));
});
