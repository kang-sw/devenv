'use strict';

// The ticket map's pure state logic (assets/dashboard-ticketmap.js, the
// block between "tm:pure-begin" and "tm:pure-end"), run under vm with no DOM:
// prefs parsing, the stored view, the derived relation map and its bucket
// precedence, search matching, focus history, and the Esc layering.
// Run: node --test internal/wsdashboard/testdata/ticketmap-*.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const src = fs.readFileSync(path.resolve(__dirname, '..', 'assets', 'dashboard-ticketmap.js'), 'utf8');
function section(text, start, end) {
  const i = text.indexOf(start), j = text.indexOf(end, i + start.length);
  if (i < 0 || j < 0) throw new Error('missing section ' + start);
  return text.slice(i, j);
}
const ctx = vm.createContext({});
vm.runInContext(section(src, '// tm:pure-begin', '// tm:pure-end'), ctx);
// Values built in the vm realm compare by JSON: a cross-realm object never
// deep-equals strictly.
const plain = v => JSON.parse(JSON.stringify(v));
const arr = s => Array.from(s);

test('prefs parse every field and fall back per field', () => {
  const def = plain(ctx.tmDefaults());
  assert.deepEqual(def, {
    simTh: 15, simUnlinked: false, simRefs: true, simEdgeTh: 24,
    st: { ready: true, todo: true, idea: true, done: true, dropped: true },
    edge: { parent: true, blocked: true, related: false, sim: true }
  });
  for (const raw of [null, '', '{', 'not json', '[1,2]', '42', 'null', '"x"']) {
    assert.deepEqual(plain(ctx.tmPrefs(raw)), def, 'invalid ' + raw);
  }
  const ok = ctx.tmPrefs(JSON.stringify({ simTh: 30, simUnlinked: true, simRefs: false, simEdgeTh: 40,
    st: { idea: false, dropped: false }, edge: { related: true, sim: false } }));
  assert.deepEqual(plain(ok), {
    simTh: 30, simUnlinked: true, simRefs: false, simEdgeTh: 40,
    st: { ready: true, todo: true, idea: false, done: true, dropped: false },
    edge: { parent: true, blocked: true, related: true, sim: false }
  });
  const bad = ctx.tmPrefs(JSON.stringify({ simTh: 9, simEdgeTh: 61, simUnlinked: 'yes', simRefs: 0,
    st: { ready: 'no', todo: false, bogus: false }, edge: [true] }));
  assert.equal(bad.simTh, 15, 'below the slider range');
  assert.equal(bad.simEdgeTh, 24, 'above the edge range');
  assert.equal(bad.simUnlinked, false);
  assert.equal(bad.simRefs, true);
  assert.deepEqual(plain(bad.st), { ready: true, todo: false, idea: true, done: true, dropped: true });
  assert.equal(bad.st.bogus, undefined);
  assert.deepEqual(plain(bad.edge), def.edge);
  assert.equal(ctx.tmPrefs(JSON.stringify({ simTh: 60 })).simTh, 60, 'range is inclusive');
  assert.equal(ctx.tmPrefs(JSON.stringify({ simTh: 10 })).simTh, 10, 'range is inclusive');
  assert.equal(ctx.tmPrefs(JSON.stringify({ simTh: Infinity })).simTh, 15);
  assert.equal(ctx.tmPrefs(JSON.stringify({ simTh: '20' })).simTh, 15, 'a string is not a number');
});

test('the first open shows View B, later the last view', () => {
  assert.equal(ctx.tmView(null), 'B');
  assert.equal(ctx.tmView(''), 'B');
  assert.equal(ctx.tmView('x'), 'B');
  assert.equal(ctx.tmView('c'), 'B');
  assert.equal(ctx.tmView('B'), 'B');
  assert.equal(ctx.tmView('C'), 'C');
});

function payload() {
  const node = (stem, status, cluster, title, extra) => Object.assign({ stem, status, cluster, title, kw: [], preview: '', path: 'ai-docs/tickets/' + status + '/' + stem + '.md' }, extra || {});
  return {
    meta: { open: 5, refs: 1 },
    clusters: [{ id: 0, label: 'Orchard · Grove', hue: 205 }, { id: 1, label: '', hue: 260 }, { id: 2, label: '', misc: true, hue: 300 }],
    nodes: [
      node('260101-epic-orchard', 'ready', 0, 'Orchard platform', { kw: [['orchard', 0], ['grove/plan.go', 1]] }),
      node('260102-feat-pruning', 'todo', 0, 'Pruning schedule', { preview: '## Goal\nTrim the apple rows' }),
      node('260103-feat-harvest', 'todo', 1, 'Harvest crates'),
      node('260104-bug-irrigation', 'idea', 1, 'Irrigation leak'),
      node('260105-chore-sheds', 'ready', 2, ''),
      node('260100-feat-old-fence', 'done', 0, 'Old fence'),
    ],
    edges: [
      [1, 0, 'parent', '', ''],
      [1, 2, 'blocked-by', '', '#phase2'],
      [2, 0, 'related', 'shares crates', ''],
      [0, 2, 'blocked-by', '', ''],
      [3, 5, 'blocked-by', '', ''],
      [0, 0, 'parent', '', ''],
      [0, 9, 'related', '', ''],
      [0, 3, 'bogus', '', ''],
    ],
    sim: [[0, 1, 0.41, [['orchard', 0]], 0], [0, 3, 0.12, [], 1], [0, 2, 0.41, [], 0]],
    clusterKeywords: {}
  };
}
const label = c => c.misc ? 'Misc' : c.label || 'Cluster ' + (c.id + 1);

test('the derived graph carries inverse relations, deg, blockedOpen and the haystack', () => {
  const g = ctx.tmDerive(payload(), label);
  const n = g.nodes;
  assert.equal(n.length, 6);
  assert.deepEqual(plain(g.count), { ready: 2, todo: 2, idea: 1, done: 1, dropped: 0 });
  assert.equal(g.byStem['260103-feat-harvest'], 2);
  assert.deepEqual(arr(n[0].X.get(1).rels), ['child'], 'parent declared on 1 is a child on 0');
  assert.deepEqual(arr(n[1].X.get(0).rels), ['parent']);
  assert.deepEqual(arr(n[2].X.get(1).rels), ['blocks'], 'blocked-by inverse is blocks');
  assert.deepEqual(arr(n[0].X.get(2).rels).sort(), ['blocked-by', 'related'], 'related both ways plus a blocked-by');
  assert.deepEqual(arr(n[2].X.get(0).rels).sort(), ['blocks', 'related']);
  assert.deepEqual(plain(n[2].X.get(0).notes), [{ note: 'shares crates', side: 'out', rel: 'related' }]);
  assert.deepEqual(plain(n[0].X.get(2).notes), [{ note: 'shares crates', side: 'in', rel: 'related' }]);
  assert.equal(n[0].X.has(0), false, 'self edges drop');
  assert.equal(n[0].X.has(9), false, 'out-of-range edges drop');
  assert.equal(n[0].X.has(3), false, 'unknown relations drop');
  assert.equal(n[0].deg, 2, 'deg counts distinct explicit neighbours');
  assert.equal(n[4].deg, 0);
  assert.equal(n[0].blockedOpen, true, 'blocked by an open ticket');
  assert.equal(n[3].blockedOpen, false, 'blocked by a done ref only');
  assert.equal(n[0].simMap.get(3).men, true);
  assert.equal(n[3].simMap.get(0).s, 0.12, 'similarity is symmetric');
  assert.deepEqual(plain(n[0].sim.map(s => s.j)), [1, 2, 3], 'sorted by score, then stem');
  assert.equal(n[4].title, '260105-chore-sheds', 'an empty title falls back to the stem');
  assert.ok(n[0].hay.includes('grove/plan.go') && n[0].hay.includes('orchard · grove'), n[0].hay);
  assert.ok(n[1].hay.includes('trim the apple rows'), 'preview text is searchable');
  assert.ok(n[2].hay.includes('cluster 2'), 'an unnamed cluster label is searchable');
  assert.ok(n[4].hay.includes('misc'));
  assert.equal(n[5].open, false);
  assert.equal(n[0].open, true);
});

test('sim list is capped at 25', () => {
  const p = payload();
  for (let k = 0; k < 40; k++) p.nodes.push({ stem: '2601' + String(10 + k) + '-feat-extra', status: 'idea', cluster: 2, title: 'x' });
  p.sim = p.nodes.slice(1).map((_, k) => [0, k + 1, 0.5, [], 0]);
  const g = ctx.tmDerive(p, label);
  assert.equal(g.nodes[0].simMap.size, p.nodes.length - 1);
  assert.equal(g.nodes[0].sim.length, 25);
});

test('bucket precedence is parent, blocked-by, child, blocks, related', () => {
  const b = rels => ctx.tmBucket(new Set(rels));
  assert.equal(b(['related', 'parent']), 'parent');
  assert.equal(b(['blocks', 'blocked-by', 'child']), 'blocked-by');
  assert.equal(b(['related', 'blocks', 'child']), 'child');
  assert.equal(b(['related', 'blocks']), 'blocks');
  assert.equal(b(['related']), 'related');
  assert.equal(b([]), null);
});

test('search matches every term, case-insensitively', () => {
  const g = ctx.tmDerive(payload(), label);
  assert.equal(ctx.tmHits(g.nodes, ''), null);
  assert.equal(ctx.tmHits(g.nodes, '   '), null);
  assert.deepEqual(arr(ctx.tmHits(g.nodes, 'ORCHARD')).sort(), [0, 1, 5], 'title, stem and cluster label');
  assert.deepEqual(arr(ctx.tmHits(g.nodes, 'orchard  pruning')), [1], 'AND across terms');
  assert.deepEqual(arr(ctx.tmHits(g.nodes, 'orchard nothing')), []);
  assert.deepEqual(plain(ctx.tmTerms('  A  b ')), ['a', 'b']);
  const S = { st: { ready: true, todo: true, idea: true, done: true, dropped: true }, cl: new Set(), hits: ctx.tmHits(g.nodes, 'orchard'), kw: null };
  assert.equal(ctx.tmFirstHit(S, g.nodes), 0, 'ready first');
  S.st.ready = false;
  assert.equal(ctx.tmShown(S, g.nodes[0]), false, 'status filter');
  assert.equal(ctx.tmFirstHit(S, g.nodes), 1, 'then todo');
  S.cl.add(0);
  assert.equal(ctx.tmFirstHit(S, g.nodes), null, 'hidden cluster');
  S.cl.clear(); S.st.ready = true; S.hits = null;
  assert.equal(ctx.tmShown(S, g.nodes[3]), true);
  S.kw = { term: 'crate', cluster: 1, ids: new Set([2]) };
  assert.equal(ctx.tmShown(S, g.nodes[3]), false, 'keyword filter');
  assert.equal(ctx.tmShown(S, g.nodes[2]), true);
});

test('history pushes, truncates, and moves back and forward', () => {
  const S = { hist: [], hp: -1 };
  assert.equal(ctx.tmHistPush(S, 4), true);
  assert.equal(ctx.tmHistPush(S, 4), false, 'the current entry is not pushed again');
  ctx.tmHistPush(S, 7);
  ctx.tmHistPush(S, 9);
  assert.deepEqual(plain(S), { hist: [4, 7, 9], hp: 2 });
  assert.equal(ctx.tmHistMove(S, 1), null, 'no forward at the end');
  assert.equal(ctx.tmHistMove(S, -1), 7);
  assert.equal(ctx.tmHistMove(S, -1), 4);
  assert.equal(ctx.tmHistMove(S, -1), null, 'no back at the start');
  assert.equal(S.hp, 0);
  assert.equal(ctx.tmHistMove(S, 1), 7);
  ctx.tmHistPush(S, 2);
  assert.deepEqual(plain(S), { hist: [4, 7, 2], hp: 2 }, 'a push truncates the forward entries');
  ctx.tmHistPush(S, 4);
  assert.deepEqual(plain(S), { hist: [4, 7, 2, 4], hp: 3 }, 'an older ticket is pushed again');
});

test('a Refresh remaps the history by stem', () => {
  const stems = ['a', 'b', 'c', 'd'];
  const now = { b: 0, d: 1, a: 2 };
  const S = { hist: [0, 1, 2, 1, 3], hp: 3 };
  ctx.tmHistRemap(S, i => stems[i], s => now[s]);
  assert.deepEqual(plain(S), { hist: [2, 0, 1], hp: 1 }, 'a gone stem drops, a repeat collapses, hp follows the current entry');
  const T = { hist: [2, 3], hp: 1 };
  ctx.tmHistRemap(T, i => stems[i], s => now[s]);
  assert.deepEqual(plain(T), { hist: [1], hp: 0 }, 'a gone current entry falls back to the one before');
  const U = { hist: [2], hp: 0 };
  ctx.tmHistRemap(U, i => stems[i], () => undefined);
  assert.deepEqual(plain(U), { hist: [], hp: -1 });
});

test('Esc layering: the first match wins', () => {
  const base = { full: false, pop: false, inQuery: false, query: '', view: 'B', ego: false, kw: false, sel: null, focus: false };
  const step = o => ctx.tmEscStep(Object.assign({}, base, o));
  assert.equal(step({ full: true, pop: true, inQuery: true, query: 'x', focus: true }), 'full');
  assert.equal(step({ pop: true, inQuery: true, query: 'x', focus: true }), 'pop');
  assert.equal(step({ inQuery: true, query: 'x', focus: true }), 'query');
  assert.equal(step({ inQuery: true, query: '', focus: true }), 'unfocus', 'an empty query falls through');
  assert.equal(step({ inQuery: false, query: 'x', focus: true }), 'unfocus', 'a query outside the focused search falls through');
  assert.equal(step({ focus: true }), 'unfocus');
  assert.equal(step({}), 'close');
  assert.equal(step({ view: 'C', ego: true, kw: true, sel: 3 }), 'ego');
  assert.equal(step({ view: 'C', kw: true, sel: 3 }), 'kw');
  assert.equal(step({ view: 'C', sel: 0 }), 'deselect', 'node 0 is a selection');
  assert.equal(step({ view: 'C' }), 'close');
  assert.equal(step({ view: 'C', focus: true }), 'close', 'View B focus does not apply in C');
  assert.equal(step({ view: 'B', kw: true, sel: 3 }), 'close', 'View C layers do not apply in B');
});

test('the preview renderer escapes everything first', () => {
  const md = ctx.tmMd;
  assert.equal(md('## Goal <b>\n### Sub\n###### Deep'), '<h4>Goal &lt;b&gt;</h4><h5>Sub</h5><h5>Deep</h5>');
  assert.equal(md('- one `a<b>`\n* **two**\nplain'), '<ul><li>one <code>a&lt;b&gt;</code></li><li><b>two</b></li></ul><p>plain</p>');
  assert.equal(md('```\n<script>alert(1)</script>\n## not a heading\n```'), '<pre>&lt;script&gt;alert(1)&lt;/script&gt;\n## not a heading</pre>');
  assert.equal(md('<img src=x onerror="y">'), '<p>&lt;img src=x onerror=&quot;y&quot;&gt;</p>');
  assert.equal(md('`**not bold**`'), '<p><code>**not bold**</code></p>');
  assert.equal(md('# H1 stays text\n\n'), '<p># H1 stays text</p>');
  assert.equal(md('```\nopen fence'), '<pre>open fence</pre>');
  assert.equal(md(null), '');
});
