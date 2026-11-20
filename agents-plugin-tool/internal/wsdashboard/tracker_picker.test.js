const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./testdata/asset-source.cjs');

const source = readAssetSource('wsdashboard', 'tracker.js');
function section(start, end) {
  const a = source.indexOf(start), b = source.indexOf(end, a);
  assert.ok(a >= 0 && b > a, `missing section ${start} .. ${end}`);
  return source.slice(a, b);
}

const ctx = {};
vm.createContext(ctx);
vm.runInContext(section('  function orgTree(', '  // Live, doing or blocked'), ctx);
vm.runInContext(section('  // --- picker modal pure logic', '  // --- end picker modal pure logic'), ctx);
const J = v => JSON.parse(JSON.stringify(v));

const MEMBERS = {
  'kim-hn': { leads: [], children: ['lee-dy', 'park-sy'], removed: false },
  'lee-dy': { leads: ['kim-hn'], children: ['choi-jw', 'jung-mj', 'kang-yr'], removed: false },
  'choi-jw': { leads: ['lee-dy'], children: [], removed: false },
  'jung-mj': { leads: ['lee-dy'], children: [], removed: false },
  'kang-yr': { leads: ['lee-dy'], children: [], removed: false },
  'park-sy': { leads: ['kim-hn'], children: ['yoon-th', 'han-sa'], removed: false },
  'yoon-th': { leads: ['park-sy'], children: [], removed: false },
  'han-sa': { leads: ['park-sy'], children: [], removed: false },
  'oh-sj': { leads: [], children: [], removed: false },
  'song-hj': { leads: ['kim-hn'], children: [], removed: true },
};

const OT = J(ctx.orgTree(MEMBERS));
const IDX = J(ctx.pmIndex(OT));

test('pmIndex builds display order, lead flags, self-inclusive subtrees, and ancestor chains from orgTree', () => {
  assert.deepEqual(IDX.order, [
    'kim-hn', 'lee-dy', 'choi-jw', 'jung-mj', 'kang-yr',
    'park-sy', 'yoon-th', 'han-sa', 'oh-sj',
  ]);
  assert.equal(!!IDX.listed['song-hj'], false);
  assert.deepEqual(Object.keys(IDX.isLead).sort(), ['kim-hn', 'lee-dy', 'park-sy']);
  assert.equal(IDX.count['kim-hn'], 8);
  assert.equal(IDX.count['lee-dy'], 4);
  assert.equal(IDX.count['park-sy'], 3);
  assert.equal(IDX.count['oh-sj'], 1);
  assert.deepEqual(IDX.subtree['lee-dy'], ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr']);
  assert.deepEqual(IDX.ancestors['choi-jw'], ['lee-dy', 'kim-hn']);
  assert.equal(IDX.desc['kim-hn']['choi-jw'], 2);
  assert.equal(IDX.desc['lee-dy']['choi-jw'], 1);
});

test('pmCover marks subtree persons and nested lower lead groups as covered while preserving inner state', () => {
  const c1 = J(ctx.pmCover(IDX, { 'lee-dy': true }));
  assert.deepEqual(c1.topGroups, ['lee-dy']);
  assert.equal(c1.personBy['lee-dy'], 'lee-dy');
  assert.equal(c1.personBy['choi-jw'], 'lee-dy');
  assert.equal(c1.groupBy['lee-dy'], undefined);

  // Turning on parent group covers lower lead's group toggle too; outermost wins
  const c2 = J(ctx.pmCover(IDX, { 'kim-hn': true, 'lee-dy': true }));
  assert.deepEqual(c2.topGroups, ['kim-hn']);
  assert.equal(c2.groupBy['lee-dy'], 'kim-hn');
  assert.equal(c2.personBy['lee-dy'], 'kim-hn');
  assert.equal(c2.personBy['choi-jw'], 'kim-hn');
  assert.equal(c2.groupBy['kim-hn'], undefined);
  assert.equal(c2.personBy['oh-sj'], undefined);

  // Turning parent group off restores lower group immediately
  const c3 = J(ctx.pmCover(IDX, { 'lee-dy': true }));
  assert.deepEqual(c3.topGroups, ['lee-dy']);
  assert.equal(c3.groupBy['lee-dy'], undefined);
});

test('pmCcEval & pmCcEvents expand group snapshots, diff over listed live members only, and keep removed/unlisted CC untouched', () => {
  // Initial CC includes kim-hn, choi-jw, oh-sj, plus removed song-hj and unlisted ghost-1
  const origCc = ['kim-hn', 'choi-jw', 'oh-sj', 'song-hj', 'ghost-1'];
  // User keeps kim-hn & choi-jw, unchecks oh-sj, picks han-sa & yoon-th, turns on lee-dy group
  const picks = { 'kim-hn': true, 'choi-jw': true, 'han-sa': true, 'yoon-th': true };
  const groups = { 'lee-dy': true };

  const ev = J(ctx.pmCcEval(IDX, origCc, picks, groups));
  assert.equal(ev.effectiveCount, 7); // kim-hn + lee-dy(4) + yoon-th + han-sa
  assert.deepEqual(ev.added, ['lee-dy', 'jung-mj', 'kang-yr', 'yoon-th', 'han-sa']);
  // oh-sj was unchecked; song-hj (removed) and ghost-1 (unlisted) must NOT be unCC'd
  assert.deepEqual(ev.removed, ['oh-sj']);
  assert.equal(ev.dirty, true);

  // Chips: group chip for lee-dy, uncovered person chips (choi-jw covered so no chip), plus rm chip for oh-sj
  assert.deepEqual(ev.chips, [
    { t: 'g', id: 'lee-dy', n: 4 },
    { t: 'p', id: 'kim-hn', wasOrig: true },
    { t: 'p', id: 'yoon-th', wasOrig: false },
    { t: 'p', id: 'han-sa', wasOrig: false },
    { t: 'rm', id: 'oh-sj' },
  ]);

  const events = J(ctx.pmCcEvents('i1', IDX, origCc, picks, groups));
  assert.deepEqual(events, [
    ['item.cc', { item: 'i1', member: 'lee-dy' }],
    ['item.cc', { item: 'i1', member: 'jung-mj' }],
    ['item.cc', { item: 'i1', member: 'kang-yr' }],
    ['item.cc', { item: 'i1', member: 'yoon-th' }],
    ['item.cc', { item: 'i1', member: 'han-sa' }],
    ['item.uncc', { item: 'i1', member: 'oh-sj' }],
  ]);

  // No-op when picks match listed origCc and no groups
  const noop = J(ctx.pmCcEvents('i1', IDX, origCc, { 'kim-hn': true, 'choi-jw': true, 'oh-sj': true }, {}));
  assert.deepEqual(noop, []);
});

test('pmRuleCovered respects includeRoot, levels, viewer, and live state', () => {
  const rules = [
    { item: 'i1', by: 'lee-dy', state: 'live', root: 'lee-dy', includeRoot: false, levels: null },
    { item: 'i1', by: 'lee-dy', state: 'pending', root: 'park-sy', includeRoot: true, levels: null },
    { item: 'i1', by: 'kim-hn', state: 'live', root: 'kim-hn', includeRoot: true, levels: 1 },
  ];
  // Same root without includeRoot does NOT cover {root: lee-dy, include_root: true}
  assert.equal(ctx.pmRuleCovered(IDX, rules, 'i1', 'lee-dy', 'lee-dy'), false);
  // Pending rule does NOT lock
  assert.equal(ctx.pmRuleCovered(IDX, rules, 'i1', 'lee-dy', 'park-sy'), false);
  // Level-limited rule does NOT cover whole subtree
  assert.equal(ctx.pmRuleCovered(IDX, rules, 'i1', 'kim-hn', 'lee-dy'), false);

  const rules2 = [
    { item: 'i1', by: 'kim-hn', state: 'live', root: 'kim-hn', includeRoot: false, levels: null },
    { item: 'i1', by: 'lee-dy', state: 'live', root: 'lee-dy', includeRoot: true, levels: null },
  ];
  // Strict descendant of an unlimited rule IS covered even if parent rule had includeRoot: false
  assert.equal(ctx.pmRuleCovered(IDX, rules2, 'i1', 'kim-hn', 'lee-dy'), true);
  assert.equal(ctx.pmRuleCovered(IDX, rules2, 'i1', 'kim-hn', 'kim-hn'), false);
  // Same root with includeRoot: true IS covered
  assert.equal(ctx.pmRuleCovered(IDX, rules2, 'i1', 'lee-dy', 'lee-dy'), true);
});

test('pmAssignEval & pmAssignEvents suppress live-group covered picks, keep pending-group covered picks, split completion classes, and match chips to payload', () => {
  // Viewer is lee-dy; reach = { choi-jw: 1, jung-mj: 1, kang-yr: 1 }
  const reach = { 'choi-jw': 1, 'jung-mj': 1, 'kang-yr': 1 };

  // Case 1 (Mockup frame B): kim-hn locked, lee-dy group (live) + choi-jw (covered by live group) + han-sa (pending person)
  const r1 = J(ctx.pmAssignEval(IDX, {
    viewer: 'lee-dy',
    reach,
    lockedPeople: { 'kim-hn': true },
    lockedGroups: {},
    picks: { 'kim-hn': true, 'choi-jw': true, 'han-sa': true },
    groups: { 'lee-dy': true },
    compl: null,
  }));
  assert.equal(r1.visCov.personBy['choi-jw'], 'lee-dy');
  assert.equal(r1.payCov.personBy['choi-jw'], 'lee-dy');
  assert.equal(r1.below, true);
  assert.equal(r1.mode, 'mixed');
  // Payload drops locked kim-hn and live-covered choi-jw; chips === targets
  assert.deepEqual(r1.targets, [
    { t: 'r', root: 'lee-dy', include_root: true, levels: null, pend: false, n: 4 },
    { t: 'p', id: 'han-sa', pend: true },
  ]);
  assert.deepEqual(r1.chips, [
    { t: 'g', id: 'lee-dy', n: 4, pend: false },
    { t: 'p', id: 'han-sa', pend: true },
  ]);
  const ev1 = J(ctx.pmAssignEvents('i1', r1.targets, null, 'lee-dy', reach));
  assert.deepEqual(ev1, [
    ['item.assign', { item: 'i1', targets: [{ root: 'lee-dy', include_root: true }], completion: 'assigner' }],
    ['item.assign', { item: 'i1', targets: ['han-sa'], completion: 'assignee' }],
  ]);

  // Case 2 (Pending group coverage, Decision 5 & 7):
  // Viewer lee-dy selects pending kim-hn group AND live lee-dy group AND live choi-jw pick.
  // Visually kim-hn covers lee-dy & choi-jw; in payload, kim-hn is PENDING so it does NOT suppress lee-dy (live),
  // while lee-dy (live) DOES suppress choi-jw! And chip band matches payload 1-to-1.
  const r2 = J(ctx.pmAssignEval(IDX, {
    viewer: 'lee-dy',
    reach,
    lockedPeople: {},
    lockedGroups: {},
    picks: { 'choi-jw': true, 'han-sa': true },
    groups: { 'kim-hn': true, 'lee-dy': true },
    compl: null,
  }));
  assert.equal(r2.visCov.groupBy['lee-dy'], 'kim-hn');
  assert.equal(r2.visCov.personBy['han-sa'], 'kim-hn');
  assert.equal(r2.payCov.groupBy['lee-dy'], undefined);
  assert.equal(r2.payCov.personBy['choi-jw'], 'lee-dy');
  assert.equal(r2.payCov.personBy['han-sa'], undefined);
  assert.deepEqual(r2.targets, [
    { t: 'r', root: 'kim-hn', include_root: true, levels: null, pend: true, n: 8 },
    { t: 'r', root: 'lee-dy', include_root: true, levels: null, pend: false, n: 4 },
    { t: 'p', id: 'han-sa', pend: true },
  ]);
  assert.deepEqual(r2.chips.map(c => `${c.t}:${c.id}:${c.pend}`), [
    'g:kim-hn:true',
    'g:lee-dy:false',
    'p:han-sa:true',
  ]);
});

test('pmAssignEval honors norule (quick-add draft) and single (topic)', () => {
  const draft = J(ctx.pmAssignEval(IDX, {
    viewer: 'lee-dy',
    reach: { 'choi-jw': 1 },
    picks: { 'choi-jw': true, 'oh-sj': true },
    groups: { 'lee-dy': true },
    norule: true,
    compl: 'assigner',
  }));
  assert.deepEqual(draft.targets.map(t => t.id), ['choi-jw', 'oh-sj']);
  assert.equal(draft.mode, 'assigner');

  const topic = J(ctx.pmAssignEval(IDX, {
    viewer: 'lee-dy',
    reach: {},
    picks: { 'park-sy': true, 'oh-sj': true },
    groups: { 'park-sy': true },
    norule: true,
    single: true,
  }));
  assert.deepEqual(topic.targets.map(t => t.id), ['park-sy']);
});

test('pmFilter keeps context ancestors expanded for matching descendants and filters unplaced', () => {
  const f1 = J(ctx.pmFilter(OT, 'han', false, () => true, (id, q) => id.includes(q)));
  assert.equal(f1.hasAny, true);
  assert.deepEqual(Object.keys(f1.matched), ['han-sa']);
  assert.equal(f1.show['kim-hn'], true);
  assert.equal(f1.show['park-sy'], true);
  assert.equal(f1.show['han-sa'], true);
  assert.equal(f1.show['lee-dy'], undefined);
  assert.equal(f1.expand['kim-hn'], true);
  assert.equal(f1.expand['park-sy'], true);

  const f2 = J(ctx.pmFilter(OT, '', true, id => id === 'oh-sj', () => true));
  assert.equal(f2.hasAny, true);
  assert.deepEqual(Object.keys(f2.show), ['oh-sj']);
});
