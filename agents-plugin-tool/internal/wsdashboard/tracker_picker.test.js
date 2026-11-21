const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./testdata/asset-source.cjs');

const trackerSource = readAssetSource('wsdashboard', 'tracker.js');
const pickerSource = readAssetSource('wsdashboard', 'tracker-picker.js');
function section(src, start, end) {
  const a = src.indexOf(start), b = src.indexOf(end, a);
  assert.ok(a >= 0 && b > a, `missing section ${start} .. ${end}`);
  return src.slice(a, b);
}

const ctx = {};
vm.createContext(ctx);
vm.runInContext(section(trackerSource, '  function orgTree(', '  // Live, doing or blocked'), ctx);
vm.runInContext(section(pickerSource, '  // --- picker modal pure logic', '  // --- end picker modal pure logic'), ctx);
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

// ----- Phase 2: Org targets, existing registrations, Decision 11/20 gating, vacant orgs -----

const ORGS = [
  { id: 'o-dev', name: 'SW개발실', head: 'kim-hn', removed: null },
  { id: 'o-sw1', name: 'SW개발1팀', head: 'lee-dy', removed: null },
  { id: 'o-qa', name: 'QA팀', head: '', removed: null },
  { id: 'o-gone-head', name: '인프라팀', head: 'song-hj', removed: null },
  { id: 'o-plat', name: '플랫폼팀', head: '', removed: 'ev-del' },
];

test('Phase 2: pmVacantOrgs lists live headless/removed-head orgs, skips live-headed and removed orgs, supports search and onlySelected', () => {
  const oi = J(ctx.orgIndex(ORGS, MEMBERS));
  const vac = J(ctx.pmVacantOrgs(oi, '', false));
  assert.deepEqual(vac.map(o => o.id).sort(), ['o-gone-head', 'o-qa']);

  const qHit = J(ctx.pmVacantOrgs(oi, 'qa', false));
  assert.deepEqual(qHit.map(o => o.id), ['o-qa']);

  const qMiss = J(ctx.pmVacantOrgs(oi, 'sw개발1팀', false));
  assert.deepEqual(qMiss, []);

  // Hidden in "selected only" mode
  assert.deepEqual(J(ctx.pmVacantOrgs(oi, '', true)), []);
});

test('Phase 2: ordinary CC picker emits item.cc_org for org heads, keeps snapshot for non-org leads, and supports 1-person orgs', () => {
  const orgsWithSolo = ORGS.concat([{ id: 'o-solo', name: '보안팀', head: 'oh-sj', removed: null }]);
  const oi = J(ctx.orgIndex(orgsWithSolo, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;

  // lee-dy heads o-sw1 (org target); park-sy has no org (snapshot); oh-sj heads 1-person o-solo (org target)
  const ev = J(ctx.pmCcEval(idx, ['choi-jw'], { 'choi-jw': true }, { 'lee-dy': true, 'park-sy': true, 'oh-sj': true }, h2o, { origOrgCc: [], ccSources: { 'choi-jw': { personal: true, orgs: [] } } }));
  assert.equal(ev.dirty, true);
  assert.deepEqual(ev.addedOrgs.map(a => a.org), ['o-sw1', 'o-solo']);
  assert.deepEqual(ev.ccMembers, ['park-sy', 'yoon-th', 'han-sa']);
  assert.deepEqual(ev.unccMembers, []);

  const events = J(ctx.pmCcEvents('i1', idx, ['choi-jw'], { 'choi-jw': true }, { 'lee-dy': true, 'park-sy': true, 'oh-sj': true }, h2o, { origOrgCc: [], ccSources: { 'choi-jw': { personal: true, orgs: [] } } }));
  assert.deepEqual(events, [
    ['item.cc_org', { item: 'i1', org: 'o-sw1' }],
    ['item.cc_org', { item: 'i1', org: 'o-solo' }],
    ['item.cc', { item: 'i1', member: 'park-sy' }],
    ['item.cc', { item: 'i1', member: 'yoon-th' }],
    ['item.cc', { item: 'i1', member: 'han-sa' }],
  ]);

  // Upgrading already-CC'd members to an org CC is dirty and emits only item.cc_org
  const allSw1 = ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr'];
  const srcAll = {};
  allSw1.forEach(m => { srcAll[m] = { personal: true, orgs: [] }; });
  const upg = J(ctx.pmCcEvents('i1', idx, allSw1, { 'lee-dy': true, 'choi-jw': true, 'jung-mj': true, 'kang-yr': true }, { 'lee-dy': true }, h2o, { origOrgCc: [], ccSources: srcAll }));
  assert.deepEqual(upg, [['item.cc_org', { item: 'i1', org: 'o-sw1' }]]);
});

test('Phase 2: existing CC org registrations open checked, unchecking emits item.uncc_org without excluding members, and respects exclusions', () => {
  const oi = J(ctx.orgIndex(ORGS, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;

  // o-sw1 is registered: lee-dy & jung-mj only via o-sw1; choi-jw personal + o-sw1; kang-yr was excluded (not in it.cc);
  // han-sa only via deleted o-plat (non-toggleable).
  const it = {
    cc: ['lee-dy', 'choi-jw', 'jung-mj', 'han-sa'],
    orgCC: [
      { org: 'o-sw1', added: ['lee-dy', 'choi-jw', 'jung-mj'] },
      { org: 'o-plat', added: ['han-sa'] },
    ],
    ccSources: {
      'lee-dy': { personal: false, orgs: ['o-sw1'] },
      'choi-jw': { personal: true, orgs: ['o-sw1'] },
      'jung-mj': { personal: false, orgs: ['o-sw1'] },
      'han-sa': { personal: false, orgs: ['o-plat'] },
    },
  };

  const init = J(ctx.pmCcInit(idx, it, h2o));
  assert.deepEqual(init.groups, { 'lee-dy': true });
  assert.deepEqual(init.picks, { 'choi-jw': true, 'han-sa': true });

  // No-op on open: dirty is false, kang-yr (excluded) stays uncovered and unchecked
  const orgCtx = { origOrgCc: init.origOrgCc, ccSources: init.ccSources };
  const evOpen = J(ctx.pmCcEval(idx, init.origCc, init.picks, init.groups, h2o, orgCtx));
  assert.equal(evOpen.dirty, false);
  assert.equal(evOpen.effectiveCount, 4);
  assert.equal(evOpen.cov.personBy['kang-yr'], undefined);
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, init.picks, init.groups, h2o, orgCtx)), []);

  // Re-checking excluded kang-yr emits personal item.cc to clear exclusion
  const picksReadd = Object.assign({}, init.picks, { 'kang-yr': true });
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, picksReadd, init.groups, h2o, orgCtx)), [
    ['item.cc', { item: 'i1', member: 'kang-yr' }],
  ]);

  // Unchecking o-sw1 emits ONLY item.uncc_org (no item.uncc on lee-dy/jung-mj; choi-jw stays personal)
  const evRm = J(ctx.pmCcEval(idx, init.origCc, init.picks, {}, h2o, orgCtx));
  assert.equal(evRm.dirty, true);
  assert.deepEqual(evRm.removed, ['lee-dy', 'jung-mj']);
  assert.deepEqual(evRm.unccMembers, []);
  assert.ok(evRm.chips.some(c => c.t === 'rmg' && c.org === 'o-sw1'));
  assert.equal(evRm.chips.some(c => c.t === 'rm'), false);
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, init.picks, {}, h2o, orgCtx)), [
    ['item.uncc_org', { item: 'i1', org: 'o-sw1' }],
  ]);

  // Unchecking o-sw1 while keeping jung-mj personally emits uncc_org + item.cc(jung-mj)
  const picksKeepJung = Object.assign({}, init.picks, { 'jung-mj': true });
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, picksKeepJung, {}, h2o, orgCtx)), [
    ['item.uncc_org', { item: 'i1', org: 'o-sw1' }],
    ['item.cc', { item: 'i1', member: 'jung-mj' }],
  ]);

  // Checking parent o-dev while o-sw1 stays checked emits only item.cc_org(o-dev), leaving o-sw1 untouched
  const groupsWithParent = { 'kim-hn': true, 'lee-dy': true };
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, init.picks, groupsWithParent, h2o, orgCtx)), [
    ['item.cc_org', { item: 'i1', org: 'o-dev' }],
  ]);
});

test('Phase 2: ordinary assign picker emits org-carrying rules for org heads, plain rules for non-org leads, and supports Decision 11/20 unchecking', () => {
  const orgsWithSolo = ORGS.concat([{ id: 'o-solo', name: '보안팀', head: 'oh-sj', removed: null }]);
  const oi = J(ctx.orgIndex(orgsWithSolo, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;
  const reach = { 'lee-dy': 1, 'choi-jw': 1, 'jung-mj': 1, 'kang-yr': 1, 'park-sy': 1, 'yoon-th': 1, 'han-sa': 1 };

  // 1. Fresh assign: o-sw1 (org head lee-dy), plain lead park-sy (no org), 1-person org oh-sj (outside reach)
  const evNew = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach,
    picks: {}, groups: { 'lee-dy': true, 'park-sy': true, 'oh-sj': true },
    headToOrg: h2o,
  }));
  const batchNew = J(ctx.pmAssignEvents('i1', evNew.targets, 'assignee', 'kim-hn', reach, h2o, evNew.removedRules));
  assert.deepEqual(batchNew, [
    ['item.assign', {
      item: 'i1',
      targets: [
        { root: 'lee-dy', include_root: true, org: 'o-sw1' },
        { root: 'park-sy', include_root: true },
      ],
      completion: 'assigner',
    }],
    ['item.assign', {
      item: 'i1',
      targets: [{ root: 'oh-sj', include_root: true, org: 'o-solo' }],
      completion: 'assignee',
    }],
  ]);

  // 2. Existing org assign rules + Decision 20 gating:
  //    r-sw1 authored by kim-hn (removable by kim-hn, locked for lee-dy);
  //    r-solo authored by lee-dy (locked for kim-hn).
  const it = { id: 'i1', hidden: false, assignments: ['a1', 'a2', 'a3'], pending: [] };
  const rules = {
    'r-sw1': { id: 'r-sw1', item: 'i1', by: 'kim-hn', root: 'lee-dy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
    'r-solo': { id: 'r-solo', item: 'i1', by: 'lee-dy', root: 'oh-sj', assignRoot: 'oh-sj', org: 'o-solo', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T02:00:00Z' },
  };
  const assignments = {
    a1: { id: 'a1', item: 'i1', to: 'lee-dy', state: 'live', rule: 'r-sw1' },
    a2: { id: 'a2', item: 'i1', to: 'choi-jw', state: 'live', rule: 'r-sw1' },
    a3: { id: 'a3', item: 'i1', to: 'oh-sj', state: 'live', rule: 'r-solo' },
  };

  const initKim = J(ctx.pmAssignOrgInit(idx, it, rules, h2o, 'kim-hn'));
  assert.equal(initKim.origOrgAssign['lee-dy'].canRemove, true);
  assert.equal(initKim.origOrgAssign['oh-sj'].canRemove, false);
  assert.deepEqual(initKim.groups, { 'lee-dy': true });

  // Locks on open: o-sw1 NOT locked (removable), o-solo IS locked (authored by lee-dy)
  const locksOpen = J(ctx.pmAssignLocks(idx, it, rules, assignments, orgsWithSolo, MEMBERS, 'kim-hn', h2o, initKim.origOrgAssign, initKim.groups, null, false));
  assert.equal(!!locksOpen.lockedG['lee-dy'], false);
  assert.equal(!!locksOpen.lockedG['oh-sj'], true);
  assert.equal(!!locksOpen.lockedP['lee-dy'], true);
  assert.equal(!!locksOpen.lockedP['choi-jw'], true);

  // No-op on open
  const evOpen = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach, lockedPeople: locksOpen.lockedP, lockedGroups: locksOpen.lockedG,
    picks: {}, groups: initKim.groups, headToOrg: h2o, origOrgAssign: initKim.origOrgAssign,
  }));
  assert.equal(evOpen.dirty, false);
  assert.deepEqual(evOpen.targets, []);
  assert.deepEqual(evOpen.removedRules, []);

  // Unchecking o-sw1 unlocks lee-dy & choi-jw and emits item.unassign before any new item.assign (Decision 20)
  const groupsSwap = { 'kim-hn': true }; // uncheck o-sw1, check o-dev
  const locksSwap = J(ctx.pmAssignLocks(idx, it, rules, assignments, orgsWithSolo, MEMBERS, 'kim-hn', h2o, initKim.origOrgAssign, groupsSwap, null, false));
  assert.equal(!!locksSwap.lockedP['lee-dy'], false);
  assert.equal(!!locksSwap.lockedP['choi-jw'], false);
  assert.equal(!!locksSwap.lockedP['oh-sj'], true);

  const evSwap = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach, lockedPeople: locksSwap.lockedP, lockedGroups: locksSwap.lockedG,
    picks: {}, groups: groupsSwap, headToOrg: h2o, origOrgAssign: initKim.origOrgAssign,
  }));
  assert.equal(evSwap.dirty, true);
  assert.equal(evSwap.removedRules.length, 1);
  assert.ok(evSwap.chips.some(c => c.t === 'rmg' && c.org === 'o-sw1'));

  const batchSwap = J(ctx.pmAssignEvents('i1', evSwap.targets, 'assignee', 'kim-hn', reach, h2o, evSwap.removedRules));
  assert.deepEqual(batchSwap, [
    ['item.unassign', { item: 'i1', rule: 'r-sw1', root: 'lee-dy' }],
    ['item.assign', { item: 'i1', targets: [{ root: 'kim-hn', include_root: true, org: 'o-dev' }], completion: 'assigner' }],
  ]);

  // Remove-only uncheck emits only item.unassign
  const locksRmOnly = J(ctx.pmAssignLocks(idx, it, rules, assignments, orgsWithSolo, MEMBERS, 'kim-hn', h2o, initKim.origOrgAssign, {}, null, false));
  const evRmOnly = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach, lockedPeople: locksRmOnly.lockedP, lockedGroups: locksRmOnly.lockedG,
    picks: {}, groups: {}, headToOrg: h2o, origOrgAssign: initKim.origOrgAssign,
  }));
  assert.equal(evRmOnly.dirty, true);
  assert.deepEqual(J(ctx.pmAssignEvents('i1', evRmOnly.targets, 'assignee', 'kim-hn', reach, h2o, evRmOnly.removedRules)), [
    ['item.unassign', { item: 'i1', rule: 'r-sw1', root: 'lee-dy' }],
  ]);
});

test('Phase 2: re-headed org assign rule attaches to current head, emits assignRoot on uncheck, and locks old assignRoot until unchecked', () => {
  // o-sw1 was assigned when lee-dy was head (assignRoot: lee-dy), then re-headed to park-sy
  const orgsRehead = [
    { id: 'o-dev', name: 'SW개발실', head: 'kim-hn', removed: null },
    { id: 'o-sw1', name: 'SW개발1팀', head: 'park-sy', removed: null },
  ];
  const oi = J(ctx.orgIndex(orgsRehead, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;
  const it = { id: 'i1', hidden: false, assignments: [], pending: [] };
  const rules = {
    'r-re': { id: 'r-re', item: 'i1', by: 'kim-hn', root: 'park-sy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
  };
  const init = J(ctx.pmAssignOrgInit(idx, it, rules, h2o, 'kim-hn'));
  assert.deepEqual(init.groups, { 'park-sy': true });
  // While park-sy (o-sw1) stays checked, lee-dy is locked by r-re's assignRoot, while park-sy is toggleable
  const locksKeep = J(ctx.pmAssignLocks(idx, it, rules, {}, orgsRehead, MEMBERS, 'kim-hn', h2o, init.origOrgAssign, init.groups, null, false));
  assert.equal(!!locksKeep.lockedG['park-sy'], false);
  assert.equal(!!locksKeep.lockedG['lee-dy'], true);
  // Once park-sy is unchecked, lee-dy unlocks and item.unassign carries assignRoot (lee-dy)
  const locksDrop = J(ctx.pmAssignLocks(idx, it, rules, {}, orgsRehead, MEMBERS, 'kim-hn', h2o, init.origOrgAssign, {}, null, false));
  assert.equal(!!locksDrop.lockedG['lee-dy'], false);
  const evDrop = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach: { 'lee-dy': 1, 'park-sy': 1 },
    lockedPeople: locksDrop.lockedP, lockedGroups: locksDrop.lockedG,
    picks: {}, groups: {}, headToOrg: h2o, origOrgAssign: init.origOrgAssign,
  }));
  assert.deepEqual(J(ctx.pmAssignEvents('i1', evDrop.targets, 'assignee', 'kim-hn', {}, h2o, evDrop.removedRules)), [
    ['item.unassign', { item: 'i1', rule: 'r-re', root: 'lee-dy' }],
  ]);
});
