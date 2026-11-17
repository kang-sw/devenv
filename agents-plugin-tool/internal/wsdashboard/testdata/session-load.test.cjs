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

test('M1: reconcile uses the shared request time, preserving newer cards', async () => {
  let now = 100, answer;
  const old = {data: {tid: 'old'}, t: {born: 90}}, recent = {data: {tid: 'new'}, t: {born: 110}};
  const c = vm.createContext({KIND: 'terminal', worktree: '', listWait: null, listSeq: 0, folders: null, Date: {now: () => now},
    route: () => '/terminals', wsT: k => k, cards: {list: () => [old, recent]}, gone: card => { card.removed = true; },
    fetch: () => new Promise(resolve => { answer = resolve; })});
  vm.runInContext(fn(terminal, 'list') + fn(terminal, 'reconcile'), c);
  c.list(() => {}); now = 120; c.reconcile();
  answer({ok: true, json: async () => ({terminals: [], orphans: []})}); await tick();
  assert.equal(old.removed, true, 'the older absent card still reconciles');
  assert.equal(recent.removed, undefined, 'a card born after the actual shared request survives');
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
  const node = (top = 0) => ({scrollTop: top, shown: true, clientTop: 2, clientHeight: 100, scrollHeight: 400,
    firstElementChild: {getBoundingClientRect: () => ({top: 200})}, getBoundingClientRect: () => ({top: 100})});
  const nodes = {convo: node(140), bcol: node(620), 'r-tree': node(350)};
  Object.keys(nodes).forEach(id => { nodes[id].id = id; });
  let boardContentTop = 628;
  const topic = {key: 'topic-a'}, panel = {id: 'p1', closest: () => topic,
    getBoundingClientRect() { const top = 102 + boardContentTop - nodes.bcol.scrollTop; return {top, bottom: top + 100, height: 100}; }};
  const box = {children: []};
  function entries(ids) {
    box.children = ids.map((id, i) => ({getAttribute: () => id,
      getBoundingClientRect() { const top = 102 + i * 20 - nodes.convo.scrollTop; return {top, bottom: top + 20, height: 20}; }}));
    nodes.convo.scrollHeight = ids.length * 20;
  }
  entries(Array.from({length: 20}, (_, i) => 'e' + i));
  const doc = {scrollingElement: node(900),
    addEventListener(type, cb) { listeners[type] = cb; }, removeEventListener(type) { delete listeners[type]; }};
  const reloadKey = 'ws.dashboard.board-reload.' + pathname + '?frame=1';
  const c = vm.createContext({ shell: false, framed: true, location: {pathname, hash, search: '?frame=1'}, BOARD_RELOAD: reloadKey,
    PANES: ['convo', 'bcol', 'r-tree', 'bpins', 'drbody'], convo: nodes.convo, entriesBox: () => box,
    atBottom: () => nodes.convo.scrollTop + nodes.convo.clientHeight >= nodes.convo.scrollHeight - 24,
    boardBoxes: () => [nodes.bcol], boardLivePanels: () => [panel], boardScroller: () => nodes.bcol, boardWithSnap: fn => fn(),
    boardPanelKey: n => n.id, topicKey: n => n.key, q: () => [topic],
    paneScroll: {}, stick: true, doc, $: id => nodes[id], visible: el => !!el?.shown,
    requestAnimationFrame(cb) { frames.push(cb); }, pinConvo() { if (c.stick) nodes.convo.scrollTop = 999; },
    sessionStorage: {getItem(k) { return storage[k] ?? null; }, setItem(k, v) { storage[k] = v; }}, addEventListener() {}
  });
  c.window = c;
  c.wbHistory = {keeping: el => !!el.owned,
    keep: () => ({scroller: nodes.bcol, viewport: () => ({top: 101})}),
    place(ref, options) {
      placements.push({ref, options, position() {
        return ref.getBoundingClientRect().top - 102 + nodes.bcol.scrollTop - (options.readingOffset ?? options.offset);
      }});
    }
  };
  vm.runInContext(read('wsdashboard', 'dashboard-session-view.js'), c);
  return {c, nodes, doc, frames, listeners, placements, panel, reloadKey, entries,
    growBoard(delta) { boardContentTop += delta; }, flush() { frames.splice(0).forEach(fn => fn()); }};
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
  assert.equal(back.placements.length, 1); assert.equal(back.placements[0].ref, back.panel);
  assert.equal(back.placements[0].options.readingOffset, 8);
  const hidden = scrollFixture(storage, '/s/c'); hidden.c.restoreSessionView(); hidden.nodes['r-tree'].shown = false;
  hidden.c.paneScroll['r-tree'] = 777; hidden.c.wsSaveSessionView();
  const reveal = scrollFixture(storage, '/s/c'); reveal.c.restoreSessionView(); reveal.flush();
  assert.equal(reveal.nodes['r-tree'].scrollTop, 777, 'hidden pane retains its last visible offset');
});

test('I1: board content anchor follows late growth and pending reload restore wins', () => {
  const storage = {}, first = scrollFixture(storage);
  first.c.restoreSessionView(); first.c.wsSaveSessionView();
  const back = scrollFixture(storage); back.c.restoreSessionView(); back.flush();
  const anchor = back.placements[0];
  assert.equal(anchor.ref, back.panel); assert.equal(anchor.options.readingOffset, 8);
  assert.equal(anchor.position(), 620);
  back.growBoard(300);
  assert.equal(anchor.position(), 920, 'growing content above the panel must move its placement, not pin the old pixel');

  const reload = scrollFixture(storage);
  storage[reload.reloadKey] = JSON.stringify({at: Date.now(), page: '/s/a?frame=1', records: [{}]});
  reload.doc.scrollingElement.scrollTop = 3;
  // Model boardRestoreReload's earlier rAF: it places the keyed node then
  // consumes its record before the session-view callback gets its turn.
  reload.frames.push(() => { reload.c.wbHistory.place(reload.panel, {readingOffset: 17}); delete storage[reload.reloadKey]; });
  reload.c.restoreSessionView(); reload.flush();
  assert.equal(reload.placements.length, 1, 'session-view must not override the earlier reload anchor');
  assert.equal(reload.placements[0].options.readingOffset, 17);
  assert.equal(reload.doc.scrollingElement.scrollTop, 3, 'pending reload owns document placement too');
});

test('I2/M4: shifted conversation window anchors identity; missing/bottom follows and cancellation leaves stick alone', () => {
  const storage = {}, first = scrollFixture(storage);
  first.c.restoreSessionView(); first.c.wsSaveSessionView();
  const anchored = {...storage};
  const back = scrollFixture(storage); back.entries(Array.from({length: 20}, (_, i) => 'e' + (i + 4)));
  back.c.restoreSessionView(); back.flush();
  assert.equal(back.nodes.convo.scrollTop, 60, 'four shifted entries cannot move the reader from e7 to e11');
  assert.equal(back.c.stick, false);
  const missing = scrollFixture(anchored); missing.entries(Array.from({length: 20}, (_, i) => 'e' + (i + 30)));
  missing.c.restoreSessionView(); missing.flush();
  assert.equal(missing.nodes.convo.scrollTop, 999); assert.equal(missing.c.stick, true, 'missing entry returns to Phase 1 bottom-follow');
  first.nodes.convo.scrollTop = 999; first.c.wsSaveSessionView();
  const bottom = scrollFixture(storage); bottom.c.restoreSessionView(); bottom.flush();
  assert.equal(bottom.nodes.convo.scrollTop, 999); assert.equal(bottom.c.stick, true, 'at-bottom leave never restores a stale pixel');
  const cancelled = scrollFixture(anchored); cancelled.nodes.convo.scrollTop = 999;
  cancelled.c.restoreSessionView();
  assert.equal(cancelled.c.stick, true, 'do not turn off follow before restoration commits');
  cancelled.listeners.keydown(); cancelled.flush();
  assert.equal(cancelled.c.stick, true); assert.equal(cancelled.nodes.convo.scrollTop, 999);
  assert.deepEqual(Object.keys(cancelled.c.paneScroll), [], 'cancelled restoration must not seed pane offsets');
});

test('scroll restore respects hashes/input, bottom-follow, denied storage and cap', () => {
  const storage = {}, first = scrollFixture(storage); first.c.restoreSessionView(); first.nodes.convo.scrollTop = 999; first.c.wsSaveSessionView();
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
  const assets = fs.readFileSync(path.join(root, 'frame_assets.go'), 'utf8');
  assert(assets.includes('d.ScriptFile = add(string(d.Script), "js")'));
  assert(pages.includes('file, ok := a.files[rest]'));
  new vm.Script(read('wsdashboard', 'dashboard.js'));
  const lifecycle = read('wsdashboard', 'dashboard-lifecycle.js');
  assert(lifecycle.indexOf('wsSaveSessionView();') < lifecycle.indexOf('location.replace("about:blank")'));
  assert(lifecycle.includes('  restoreSessionView();\n  nearTop();'));
});
