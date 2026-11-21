// Offline navigation contracts: production functions only, no daemon/process.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const dashboard = readAssetSource('wsdashboard', 'dashboard.js');
function func(source, name, indent = '  ') {
  const found = source.match(new RegExp('\\n' + indent + 'function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n' + indent + 'function ' + name + '\\([\\s\\S]*?\\n' + indent + '\\}\\n'));
  assert.ok(found, 'missing production function ' + name);
  return found[0];
}
function environment(pending) {
  const events = [], nodes = {}, oldNodes = {};
  let context;
  function node(id, topic) {
    const n = { id, tagName: id.startsWith('topic-') ? 'SECTION' : 'DIV', isConnected: true,
      hasAttribute: key => key === 'data-topic', getAttribute: () => 'topic',
      closest(selector) { return selector === '.wb-history' ? null : topic || n; },
      matches: selector => /^(item|check)-/.test(id) && selector.includes('.card.policy[data-id]'),
      querySelector: selector => { const match = /^\[id="([^"]+)"\]$/.exec(selector); return match ? nodes[match[1]] || null : null; },
      classList: { remove() {}, add() {}, toggle() {}, contains() { return false; } },
      scrollIntoView() { assert.equal(nodes[n.id], n, 'scroll the applied live node'); events.push('scroll:' + n.id); },
      setAttribute() {}, focus() { events.push('focus:' + id); }
    };
    return n;
  }
  function populate() {
    const topic = nodes['topic-aa'] = node('topic-aa');
    nodes.p33 = node('p33', topic); nodes['item-44'] = node('item-44', topic);
  }
  populate();
  if (pending) { Object.assign(oldNodes, nodes); delete nodes.p33; delete nodes['item-44']; }
  context = vm.createContext({ assert, events, window: { wbPill: {}, WsCards: null },
    doc: { body: { classList: { contains: () => false } } }, root: { getAttribute: () => null },
    $: id => nodes[id], boardBoxes: () => [{}], boardQ: () => Object.values(nodes).filter(n => n.tagName === 'SECTION'),
    inBoard: n => !!n && Object.values(nodes).includes(n), q: () => [],
    grid: null, live: false, tkApi: null, effectJumpEpoch: 0,
    boardPending: pending ? { T: [{}] } : {}, boardTopHeld: pending ? { T: {} } : {},
    boardChanges: pending ? [{ names: ['T'] }] : [], boardReleaseFrame: 0, boardRelease: false,
    boardLatest: { hash: 'latest', html: {} }, boardHome() {}, boardFlash() {},
    swap() {
      assert.equal(context.boardRelease, true, 'navigation uses explicit badge apply');
      events.push('apply'); Object.values(nodes).forEach(n => { n.isConnected = false; }); populate();
      context.boardPending = {}; context.boardTopHeld = {}; context.boardChanges = [];
    }, setTimeout() {}, setTab() {}, tabShown: () => true,
    loadClosed() { throw Error('applied topic must exist before closed-page lookup'); },
    go() { throw Error('not a session navigation'); }
  });
  vm.runInContext(['boardPendingNames', 'boardPendingBadge', 'boardApplyPending', 'boardFlushViewed', 'boardJump', 'effectTopic', 'effectJump', 'palettePick', 'focusTopicNote'].map(name => func(dashboard, name)).join('') + '\nboardPendingBadge();', context);
  context.window.wbHistory = { loadTarget(root, id) {
    assert.equal(Object.keys(context.boardPending).length, 0, 'apply before materialization');
    events.push('load:' + id); if (nodes[id]) nodes[id].scrollIntoView(); return nodes[id] || null;
  } };
  return { context, nodes, oldNodes, events };
}

for (const pending of [false, true]) for (const id of ['topic-aa', 'p33', 'item-44']) {
  for (const entry of ['boardJump', 'effectJump']) test(entry + ' applies before navigation, pending=' + pending + ', ' + id, () => {
    const { context, nodes, events } = environment(pending);
    if (entry === 'boardJump') context.boardJump(id); else context.effectJump('topic-aa', id);
    assert.equal(events.filter(e => e === 'apply').length, pending ? 1 : 0);
    assert.ok(events.includes('scroll:' + id));
    if (pending) assert.equal(events[0], 'apply');
    assert.equal(context.window.wbPill.pending, 0);
    assert.deepEqual(Object.keys(context.boardTopHeld), []);
    if (entry === 'effectJump') assert.ok(events.includes('focus:' + id));
  });
}

test('invalid board navigation and session palette navigation do not apply updates', () => {
  const { context, events } = environment(true);
  context.boardJump('p0'); context.effectJump('other/session', 'p33');
  context.go = () => events.push('session'); context.palettePick({ kind: 'session', href: '/s/other' });
  assert.deepEqual(events, ['session']);
  assert.equal(context.window.wbPill.pending, 1);
});

for (const pending of [false, true]) test('palette and topic-note navigation use the same apply hook, pending=' + pending, () => {
  for (const entry of ['palette', 'note']) {
    const { context, nodes, events } = environment(pending);
    if (entry === 'palette') context.palettePick({ kind: 'board-topic', topic: 'topic' });
    else {
      context.topicNote = () => ({ hidden: false, closest: () => nodes['topic-aa'], focus() { events.push('note'); } });
      context.focusTopicNote('topic');
    }
    assert.equal(events.filter(e => e === 'apply').length, pending ? 1 : 0);
    assert.ok(events.includes('scroll:topic-aa'));
    if (pending) assert.equal(events[0], 'apply');
  }
});

for (const entry of ['fragment', 'effect', 'palette']) test(entry + ' delegates its resolved target to board navigation after selecting the board', () => {
  const { context, nodes, events } = environment(false);
  context.grid = {}; context.live = true;
  context.tabShown = () => false;
  context.setTab = tab => events.push('tab:' + tab);
  context.window.wbHistory.navigate = node => { events.push('navigate:' + node.id); return node; };
  if (entry === 'fragment') context.boardJump('topic-aa');
  else if (entry === 'effect') context.effectJump('topic-aa', 'topic-aa');
  else context.palettePick({ kind: 'board-item', topic: 'topic', id: '44' });
  const id = entry === 'palette' ? 'item-44' : 'topic-aa';
  assert.ok(events.includes('navigate:' + id));
  assert.ok(!events.includes('scroll:' + id), 'the helper owns landing geometry');
  if (entry !== 'fragment') assert.ok(events.indexOf('tab:board') < events.indexOf('navigate:' + id));
});

test('palette retained entry relies on history materialization and preserves palette focus', () => {
  const { context, events } = environment(false);
  context.window.wbHistory.navigate = node => { events.push('navigate:' + node.id); return node; };
  context.palettePick({ kind: 'board-entry', topic: 'topic', entry: 'p33' }, { keep: true });
  assert.ok(events.includes('load:p33'));
  assert.ok(!events.includes('navigate:p33'), 'loadTarget already navigates');
  assert.ok(!events.includes('focus:p33'));
});

for (const entry of ['fragment', 'effect']) test(entry + ' selects the board before materializing and resolves item/check aliases', () => {
  const { context, nodes, events } = environment(false);
  nodes['check-44'] = nodes['item-44']; nodes['check-44'].id = 'check-44'; delete nodes['item-44'];
  context.grid = {}; context.tabShown = () => false;
  context.setTab = tab => events.push('tab:' + tab);
  context.window.wbHistory.navigate = node => { events.push('navigate:' + node.id); return node; };
  if (entry === 'fragment') context.boardJump('item-44');
  else context.effectJump('topic-aa', 'item-44');
  assert.equal(events[0], 'tab:board');
  assert.ok(events.includes('navigate:check-44') || events.includes('load:check-44'));
});

for (const entry of ['fragment', 'effect']) test(entry + ' waits for a lazy check alias before landing', async () => {
  const { context, nodes, events } = environment(false);
  const target = nodes['item-44']; target.id = 'check-44'; delete nodes['item-44'];
  let finish, loaded = false;
  const pending = new Promise(resolve => { finish = resolve; });
  context.window.wbHistory.loadTarget = (root, id) => {
    events.push('load:' + id);
    if (id === 'check-44' && loaded) { nodes[id] = target; events.push('land:' + id); return target; }
    return null;
  };
  context.window.wbHistory.loading = id => id === 'check-44' && !loaded ? pending : null;
  context.loadClosed = () => { throw Error('alias lookup must not fetch closed topics'); };
  const results = [];
  if (entry === 'fragment') assert.equal(context.boardJump('item-44'), true);
  else context.effectJump('topic-aa', 'item-44', (topic, exact) => results.push([topic, exact]));
  assert.deepEqual(results, []);
  assert.ok(!events.includes('land:check-44'));
  loaded = true; finish(true);
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(events.includes('land:check-44'));
  if (entry === 'effect') assert.deepEqual(results, [[true, true]]);
});

test('a consumed board link is not handled a second time', () => {
  assert.match(dashboard, /if \(e\.defaultPrevented \|\| !a \|\| !inBoard\(a\)/);
});

test('explicit recap resolves its applied topic before materializing the target', () => {
  const { context, nodes, events } = environment(true);
  const old = nodes['topic-aa'];
  context.document = { getElementById: id => nodes[id] };
  context.cancel = () => {}; context.FLASH = 2000;
  context.window.clearTimeout = () => {}; context.window.setTimeout = () => 1;
  context.window.wbHistory.loadTarget = (root, id) => {
    assert.equal(root, nodes['topic-aa']); events.push('materialize'); nodes[id].scrollIntoView(); return nodes[id];
  };
  vm.runInContext(func(readAssetSource('wswhiteboard', 'stream.js'), 'jump'), context);
  context.jump({ closest: () => old, getAttribute: () => 'p33' });
  assert.deepEqual(events, ['apply', 'materialize', 'scroll:p33']);
});

test('automatic history restoration does not acknowledge pending updates', () => {
  const { context, nodes, events } = environment(false);
  context.boardPending = { T: [{}] }; context.boardChanges = [{ names: ['T'] }]; context.boardPendingBadge();
  context.document = {}; context.valid = () => true;
  context.materialize = () => { events.push('materialize'); return nodes.p33; };
  context.openTarget = () => {}; context.panelState = null; context.navigate = n => n.scrollIntoView();
  vm.runInContext(func(readAssetSource('wswhiteboard', 'stream.js'), 'loadTarget'), context);
  context.loadTarget(nodes['topic-aa'], 'p33');
  assert.deepEqual(events, ['materialize', 'scroll:p33']);
  assert.equal(context.window.wbPill.pending, 1);
});

for (const entry of ['palette', 'effect', 'fragment']) test('Overview defers explicit ' + entry + ' navigation, then applies before lookup', () => {
  const { context, nodes, events } = environment(true), callbacks = [];
  let holding = true;
  context.window.WsOverview = { holding: () => holding, after: fn => callbacks.push(fn), pick: () => false };
  if (entry === 'palette') context.palettePick({ kind: 'board-topic', topic: 'topic' });
  else if (entry === 'effect') context.effectJump('topic-aa', 'p33');
  else assert.equal(context.boardJump('p33'), true, 'deferred fragments are handled, not native-scroll fallthrough');
  assert.deepEqual(events, []);
  assert.equal(callbacks.length, 1);
  holding = false; callbacks.shift()();
  assert.equal(events[0], 'apply');
  assert.ok(events.includes('scroll:' + (entry === 'palette' ? 'topic-aa' : 'p33')));
  assert.equal(context.window.wbPill.pending, 0);
});

test('direct Overview target selection applies before landing on its fresh identity', () => {
  const { context, nodes, events } = environment(true), callbacks = [];
  let holding = true;
  context.window.WsOverview = { holding: () => holding, after: fn => callbacks.push(fn) };
  Object.assign(context, { o: {}, on: true, busy: false, ITEM: 'section.topic', identity: n => n.id,
    find: id => nodes[id], clearance: () => 0, keep: n => n.scrollIntoView(), flash() {} });
  vm.runInContext(func(readAssetSource('wsdashboard', 'overview.js'), 'exit'), context);
  context.exit(nodes['topic-aa']);
  assert.deepEqual(events, []); assert.equal(callbacks.length, 1);
  holding = false; context.on = false; callbacks.shift()();
  assert.deepEqual(events, ['apply', 'scroll:topic-aa', 'focus:topic-aa']);
});

for (const overview of [false, true]) test('pull-in-place button applies without home navigation; overview=' + overview, () => {
  const { context, events } = environment(true);
  context.st = context.window.wbPill;
  context.st.home = () => { throw Error('pull must not navigate home'); };
  context.window.WsOverview = { holding: () => overview, after: () => { throw Error('refused pull must leave the pill alone'); } };
  vm.runInContext(func(readAssetSource('wswhiteboard', 'pill.js'), 'click'), context);
  context.click('top')({ target: { closest: selector => selector === '.oq-pull' ? {} : null } });
  assert.deepEqual(events, overview ? [] : ['apply'], 'no scroll, focus jump, or home navigation');
  assert.equal(context.window.wbPill.pending, overview ? 1 : 0);
});

test('pill segment rematerializes an applied history target rather than scrolling a detached node', () => {
  const { context, nodes, events } = environment(true);
  const old = { id: 'item-44', isConnected: true };
  const apply = context.window.wbPill.apply;
  context.st = { apply(retry) { const ready = apply(retry); if (ready) old.isConnected = false; return ready; } };
  context.target = { top: { open: old } }; context.d = context.doc;
  context.window.wbHistory.loadTarget = (root, id) => { events.push('materialize:' + id); nodes[id].scrollIntoView(); return nodes[id]; };
  const source = readAssetSource('wswhiteboard', 'pill.js');
  vm.runInContext(func(source, 'click'), context);
  context.click('top')({ target: { closest(selector) { return selector === '.oq-seg' ? { getAttribute: () => 'open' } : null; } } });
  assert.deepEqual(events, ['apply', 'materialize:item-44', 'scroll:item-44']);
});
