'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const source = fs.readFileSync(__dirname + '/assets/dashboard-tracker.js', 'utf8');
const block = source.slice(source.indexOf('  function tkErrText('), source.indexOf('  // tkRegistryWrite posts'));
const copy = x => JSON.parse(JSON.stringify(x));
function harness() {
  const requests = [], timers = [];
  const c = { tkApi: {}, tkOrder: [], tkHidden: [], tkInfo: {}, tkViews: {}, tkErrs: {}, tkSeen: {}, tkSubs: [],
    tkBusy: false, tkQueued: false, tkNext: [], tkHostErr: null, tkLastRev: null, tkNudgedRev: null,
    tkSupport: false, wsT: x => x, tkMount() {},
    tkUrl: (id, op) => '/tracker/stores/' + id + '/' + op,
    tkFetch(path, body, done, keep, signal, tag) { requests.push({ path, body, done, tag }); },
    setTimeout(fn) { const t = { fn }; timers.push(t); return t; },
    clearTimeout(t) { if (t) t.cancelled = true; }, window: { parent: { frames: [] } }
  };
  vm.createContext(c); vm.runInContext(block + source.slice(source.indexOf('  // tkRegistryWrite posts'), source.indexOf('  // The drawer hangs from the header')), c);
  let nudges = 0, notes = 0;
  c.tkReload = () => nudges++; c.tkSubs.push(() => notes++);
  return { c, requests, flush() { while (timers.length) { const t = timers.shift(); if (!t.cancelled) t.fn(); } }, nudges: () => nudges, notes: () => notes };
}
function row(id, version = 'v1', extra = {}) {
  return { detailVersion: version, data: { item: { id, title: id, thread: ['e-' + id] },
    assignments: { ['a-' + id]: { id: 'a-' + id, item: id } }, requests: {}, rules: {}, notices: [],
    time: { dueState: 'today' }, ...extra }, seen: { badge: ['a:' + id], read: ['r:' + id] } };
}
function answer(items = { a: row('a'), b: row('b') }, cursor = 'epoch.1', reset = true, globals = { itemOrder: Object.keys(items), my: { badge: ['a:a'] } }) {
  return { cursor, reset, items, removed: [], globals, feed: [] };
}
function bundle(version, body) {
  return { detailVersion: version, data: { body, bodyRev: [version], attachments: [],
    comments: { c: { id: 'c', body } }, events: [{ id: 'e-a', type: 'item.edit', status: 'flagged', reason: body }],
    html: { items: { a: '<p>' + body + '</p>' }, comments: { c: body } } } };
}

test('summary first, keyed delta, 304, seen union, removals and epoch reset', () => {
  const h = harness(), c = h.c;
  let changes = [];
  c.tkLoadView('s', changed => changes.push(changed));
  assert.equal(h.requests[0].path, '/tracker/stores/s/summary');
  h.requests.shift().done(200, answer());
  const untouched = c.tkViews.s.items.b, model = c.tkViews.s;
  assert.equal(c.tkViews.s.items.a.body, undefined);
  assert.equal(h.requests.length, 0, 'handles/feed do not load detail');
  c.tkAddMarks('s', ['local'], ['pending']);
  c.tkLoadView('s', changed => changes.push(changed));
  assert.equal(h.requests[0].path, '/tracker/stores/s/delta?since=epoch.1');
  h.requests.shift().done(304, {});
  assert.equal(c.tkViews.s, model);
  c.tkLoadView('s', changed => changes.push(changed));
  h.requests.shift().done(200, answer({ a: row('a', 'v2', { assignments: { anew: { item: 'a' } }, flagged: 2 }) }, 'epoch.2', false, { stats: { events: 4 } }));
  assert.equal(c.tkViews.s.items.b, untouched, 'unchanged keyed row retained');
  assert.equal(c.tkViews.s.assignments['a-a'], undefined);
  assert.equal(c.tkViews.s.assignments.anew.item, 'a');
  assert.equal(c.tkViews.s.itemFlags.a, 2);
  assert.deepEqual(copy(c.tkViews.s.itemOrder), ['a', 'b']);
  assert.deepEqual(copy(c.tkSeen.s.read), { 'r:a': true, 'r:b': true, pending: true });
  c.tkTakeProjection('s', { ...answer({}, 'epoch.3', false, {}), removed: ['a'] });
  assert.equal(c.tkViews.s.items.a, undefined);
  assert.equal(c.tkViews.s.assignments.anew, undefined);
  c.tkTakeProjection('s', answer({ z: row('z') }, 'new-epoch.1'));
  assert.deepEqual(Object.keys(c.tkViews.s.items), ['z']);
  assert.deepEqual(changes, [true, false, true]);
});

test('newer detail before delta and older detail after delta never render mismatched payloads', () => {
  const h = harness(), c = h.c;
  c.tkTakeProjection('s', answer({ a: row('a') }));
  assert.equal(c.tkNeedDetails('s', ['a']), false); h.flush();
  h.requests.shift().done(200, { items: { a: bundle('v2', 'future') } });
  assert.equal(c.tkViews.s.items.a.body, undefined);
  assert.deepEqual(Object.keys(c.tkViews.s.comments), []);
  assert.deepEqual(Object.keys(c.tkViews.s.html.items), []);
  assert.equal(h.nudges(), 1);
  c.tkTakeProjection('s', answer({ a: row('a', 'v2') }, 'epoch.2', false, {}));
  assert.equal(c.tkViews.s.items.a.body, 'future');
  assert.equal(c.tkViews.s.html.items.a, '<p>future</p>');
  c.tkTakeProjection('s', answer({ a: row('a', 'v3') }, 'epoch.3', false, {}));
  assert.equal(c.tkViews.s.items.a.body, undefined, 'old detail is evicted at summary change');
  assert.deepEqual(Object.keys(c.tkViews.s.comments), []);
  c.tkNeedDetails('s', ['a']); h.flush();
  h.requests.shift().done(200, { items: { a: bundle('v2', 'late old') } });
  assert.equal(c.tkViews.s.items.a.body, undefined);
  assert.equal(c.tkDetailReady('s', 'a'), false);
  c.tkTakeProjection('s', answer({ a: row('a', 'v3') }, 'epoch.4', false, {}));
  let done = 0; c.tkNeedDetails('s', ['a'], () => done++); h.flush();
  h.requests.shift().done(200, { items: { a: bundle('v3', 'matching') } });
  assert.equal(c.tkViews.s.items.a.body, 'matching');
  assert.equal(c.tkViews.s.events[0].reason, 'matching');
  assert.equal(done, 1);
  // A still-older request cannot displace the matching bundle.
  c.tkProjections.s.held.a = bundle('v1', 'stale');
  c.tkTakeProjection('s', answer({ a: row('a', 'v3') }, 'epoch.5', false, {}));
  assert.equal(c.tkViews.s.items.a.body, 'matching');
});

test('visible ids batch at 100, deduplicate, settle once, ignore retired responses', () => {
  const h = harness(), c = h.c, rows = {};
  for (let i = 0; i < 205; i++) rows['i' + i] = row('i' + i);
  c.tkTakeProjection('s', answer(rows));
  let done = 0;
  c.tkNeedDetails('s', ['i0', 'i1'], () => done++);
  c.tkNeedDetails('s', Object.keys(rows)); c.tkNeedDetails('s', ['i0']); h.flush();
  assert.deepEqual(h.requests.map(r => new URL(r.path, 'http://test').searchParams.getAll('id').length), [100, 100, 5]);
  const first = h.requests.shift();
  first.done(200, { items: { i0: bundle('v1', 'zero') } });
  assert.equal(done, 0);
  const second = h.requests.shift();
  second.done(200, { items: { i1: bundle('v1', 'one') } });
  assert.equal(done, 1);
  const old = h.requests.shift();
  c.tkTakeProjection('s', answer({ z: row('z') }, 'new.1'));
  old.done(200, { items: { i202: bundle('v1', 'gone') } });
  assert.deepEqual(Object.keys(c.tkViews.s.items), ['z']);
});

test('old daemon fallback keeps legacy ETag and missing delta/detail fall back too', () => {
  for (const missing of ['summary', 'delta', 'detail']) {
    const h = harness(), c = h.c;
    if (missing !== 'summary') c.tkTakeProjection('s', answer({ a: row('a') }));
    if (missing === 'detail') { c.tkNeedDetails('s', ['a']); h.flush(); }
    else c.tkLoadView('s', () => {});
    assert(h.requests[0].path.includes('/' + missing));
    h.requests.shift().done(404, {});
    assert.equal(h.requests[0].path, '/tracker/stores/s/view');
    h.requests.shift().done(200, { view: { legacy: true }, seen: { badge: ['old'], read: [] } }, 'W/"legacy"', '{"view":{"legacy":true}}');
    c.tkLoadView('s', () => {});
    const next = h.requests.shift();
    assert.equal(next.path, '/tracker/stores/s/view'); assert.equal(next.tag, 'W/"legacy"');
    next.done(304, {});
    assert.equal(c.tkViews.s.legacy, true);
    assert.equal(c.tkProjections.s, undefined);
  }
});

test('concurrent reads and hidden-store responses cannot regress or resurrect a model', () => {
  const h = harness(), c = h.c;
  c.tkTakeProjection('s', answer());
  c.tkLoadView('s', () => {}); const slow = h.requests.shift();
  c.tkLoadView('s', () => {}); const fast = h.requests.shift();
  fast.done(200, answer({ a: row('a', 'v3') }, 'epoch.3', false, {}));
  slow.done(200, answer({ a: row('a', 'v2') }, 'epoch.2', false, {}));
  assert.equal(c.tkProjections.s.cursor, 'epoch.3');
  c.tkLoadView('s', () => {}); const hidden = h.requests.shift();
  c.tkReadAll(() => {}, () => {});
  h.requests.shift().done(200, { stores: [{ id: 's', hidden: true }] });
  hidden.done(200, answer());
  assert.equal(c.tkViews.s, undefined);
  assert.equal(c.tkProjections.s, undefined);
});

test('dispatch reports actual item and global verdicts, not a guessed applied status', () => {
  const h = harness(), c = h.c;
  c.tkTakeProjection('s', answer({ a: row('a') }));
  let result;
  c.tkAdapter.dispatch('s', [{ type: 'item.edit', payload: { item: 'a' } }], r => { result = r; });
  h.requests.shift().done(200, { ids: ['e-a'] });
  h.requests.shift().done(200, answer({ a: row('a', 'v2') }, 'epoch.2', false, {}));
  const request = h.requests.shift(); assert(request.path.includes('/detail?'));
  request.done(200, { items: { a: bundle('v2', 'real flagged reason') } });
  assert.equal(result.results[0].status, 'flagged'); assert.equal(result.results[0].reason, 'real flagged reason');
  result = null;
  c.tkAdapter.dispatch('s', [{ type: 'roles.remove_label', payload: { label: 'l' } }], r => { result = r; });
  h.requests.shift().done(200, { ids: ['roles'] });
  h.requests.shift().done(304, {});
  const history = h.requests.shift(); assert(history.path.endsWith('/view'));
  history.done(200, { view: { events: [{ id: 'roles', type: 'roles.remove_label', status: 'flagged', reason: 'global reason' }] } });
  assert.equal(result.results[0].reason, 'global reason');
});

test('active search follows in-place projection generations and rejects overtaken answers', () => {
  const h = harness(), c = h.c;
  c.tkTakeProjection('s', answer({ a: row('a') }));
  c.VS = c.tkViews.s; c.ui = { store: 's', psr: {} }; c.AD = c.tkAdapter;
  vm.runInContext(fs.readFileSync(__dirname + '/assets/tracker-panel.js', 'utf8'), c);
  c.render = () => { c.SQ = 'needle'; c.SIDS = c.ui.psr.s.ids; };
  c.searchFor('s', 'needle'); h.flush(); h.requests.shift().done(200, { ids: [] });
  assert.equal(c.matchItem(c.VS.items.a), false);
  const changed = row('a', 'v2'); changed.data.item.title = 'needle';
  c.tkTakeProjection('s', answer({ a: changed }, 'epoch.2', false, {}));
  c.searchFor('s', 'needle'); h.flush();
  assert(h.requests[0].path.endsWith('/search?q=needle'), 'constant query reissues after delta');
  h.requests.shift().done(200, { ids: ['a'] });
  assert.equal(c.matchItem(c.VS.items.a), true);
  c.tkTakeProjection('s', answer({ a: row('a', 'v3') }, 'epoch.3', false, {}));
  c.searchFor('s', 'needle'); h.flush(); const old = h.requests.shift();
  c.tkTakeProjection('s', answer({ a: row('a', 'v4') }, 'epoch.4', false, {}));
  old.done(200, { ids: ['b'] });
  assert.deepEqual(copy(c.ui.psr.s.ids), { a: true }, 'response overtaken before render is ignored');
  c.searchFor('s', 'needle');
  old.done(200, { ids: ['b'] });
  assert.deepEqual(copy(c.ui.psr.s.ids), { a: true }, 'response overtaken before new timer fires is ignored');
  h.flush(); h.requests.shift().done(200, { ids: [] });
  assert.equal(c.matchItem(c.VS.items.a), false, 'changed membership is displayed');
});

test('errors retry only explicitly; history remains demand-only and separate from detail', () => {
  const h = harness(), c = h.c;
  c.tkTakeProjection('s', answer({ a: row('a') }));
  c.tkNeedDetails('s', ['a']); h.flush(); h.requests.shift().done(503, { error: 'busy' });
  c.tkNeedDetails('s', ['a']); h.flush(); assert.equal(h.requests.length, 0);
  assert.equal(c.tkProjections.s.detailError, 'busy');
  delete c.tkProjections.s.tried.a;
  c.tkNeedDetails('s', ['a']); h.flush(); assert.equal(h.requests.length, 1);
  h.requests.shift().done(200, { items: { a: bundle('v1', 'detail') } });
  assert.equal(c.tkHistory('s'), false);
  h.requests.shift().done(503, { error: 'history busy' });
  assert.equal(c.tkHistory('s'), false); assert.equal(h.requests.length, 0, 'history failure must not spin');
  assert.equal(c.tkAdapter.historyError('s'), 'history busy');
  c.tkAdapter.retryHistory('s');
  assert.equal(h.requests.length, 1);
  h.requests.shift().done(200, { view: { events: [{ id: 'roles', payload: { secret: 'history' } }] } });
  assert.equal(c.tkHistory('s'), true);
  assert.equal(c.tkViews.s.historyEvents[0].id, 'roles');
  assert.equal(c.tkViews.s.events[0].id, 'e-a');
  assert.equal(c.tkViews.s.items.a.body, 'detail');
});
