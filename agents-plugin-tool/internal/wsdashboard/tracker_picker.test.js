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
  // Only personal sources start in picks; org-only han-sa (via deleted o-plat) is NOT auto-picked as personal.
  assert.deepEqual(init.picks, { 'choi-jw': true });

  // No-op on open: dirty is false, han-sa stays effective via o-plat, kang-yr (excluded) stays uncovered and unchecked
  const orgCtx = { origOrgCc: init.origOrgCc, ccSources: init.ccSources };
  const evOpen = J(ctx.pmCcEval(idx, init.origCc, init.picks, init.groups, h2o, orgCtx));
  assert.equal(evOpen.dirty, false);
  assert.equal(evOpen.effectiveCount, 4);
  assert.equal(evOpen.effective['han-sa'], true);
  assert.equal(evOpen.retained['han-sa'], true);
  assert.equal(evOpen.cov.personBy['kang-yr'], undefined);
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, init.picks, init.groups, h2o, orgCtx)), []);

  // Explicitly unpicking org-only han-sa emits personal item.uncc(han-sa)
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, init.picks, init.groups, h2o, Object.assign({}, orgCtx, { unpicks: { 'han-sa': true } }))), [
    ['item.uncc', { item: 'i1', member: 'han-sa' }],
  ]);

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

// ----- Phase 2 review regressions -----

test('Phase 2 review: moved member with retained org CC (sole source A; A+B provenance) never fabricates personal CC or uncc', () => {
  const orgsAB = [
    { id: 'o-dev', name: 'SW개발실', head: 'kim-hn', removed: null },
    { id: 'o-sw1', name: 'SW개발1팀', head: 'lee-dy', removed: null },
    { id: 'o-sw2', name: 'SW개발2팀', head: 'park-sy', removed: null },
  ];
  const oi = J(ctx.orgIndex(orgsAB, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;

  // Case A: han-sa moved out of o-sw1 (now under park-sy / o-sw2), still CC'd ONLY via o-sw1 (sole source A)
  const itSole = {
    id: 'i1',
    cc: ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr', 'han-sa'],
    orgCC: [{ org: 'o-sw1', added: ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr', 'han-sa'] }],
    ccSources: {
      'lee-dy': { personal: false, orgs: ['o-sw1'] },
      'choi-jw': { personal: false, orgs: ['o-sw1'] },
      'jung-mj': { personal: false, orgs: ['o-sw1'] },
      'kang-yr': { personal: false, orgs: ['o-sw1'] },
      'han-sa': { personal: false, orgs: ['o-sw1'] },
    },
  };
  const initSole = J(ctx.pmCcInit(idx, itSole, h2o));
  assert.deepEqual(initSole.picks, {}); // han-sa NOT auto-picked as personal
  assert.deepEqual(initSole.groups, { 'lee-dy': true });
  const ctxSole = { origOrgCc: initSole.origOrgCc, ccSources: initSole.ccSources, ccExcluded: initSole.ccExcluded };
  // On open: han-sa is retained & effective, 0 events
  const evSoleOpen = J(ctx.pmCcEval(idx, initSole.origCc, initSole.picks, initSole.groups, h2o, ctxSole));
  assert.equal(evSoleOpen.dirty, false);
  assert.equal(evSoleOpen.effective['han-sa'], true);
  assert.equal(evSoleOpen.retained['han-sa'], true);
  // Unchecking o-sw1 emits ONLY item.uncc_org(o-sw1) — never item.cc(han-sa) and never item.uncc(han-sa)
  const evSoleUncheck = J(ctx.pmCcEval(idx, initSole.origCc, initSole.picks, {}, h2o, ctxSole));
  assert.equal(evSoleUncheck.effective['han-sa'], undefined);
  assert.ok(evSoleUncheck.removed.includes('han-sa'));
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, initSole.origCc, initSole.picks, {}, h2o, ctxSole)), [
    ['item.uncc_org', { item: 'i1', org: 'o-sw1' }],
  ]);

  // Case B: han-sa has A+B provenance (o-sw1 + o-sw2); oh-sj also moved out of both and has A+B provenance
  const itAB = {
    id: 'i1',
    cc: ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr', 'park-sy', 'yoon-th', 'han-sa', 'oh-sj'],
    orgCC: [
      { org: 'o-sw1', added: ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr', 'han-sa', 'oh-sj'] },
      { org: 'o-sw2', added: ['park-sy', 'yoon-th', 'han-sa', 'oh-sj'] },
    ],
    ccSources: {
      'lee-dy': { personal: false, orgs: ['o-sw1'] },
      'choi-jw': { personal: false, orgs: ['o-sw1'] },
      'jung-mj': { personal: false, orgs: ['o-sw1'] },
      'kang-yr': { personal: false, orgs: ['o-sw1'] },
      'park-sy': { personal: false, orgs: ['o-sw2'] },
      'yoon-th': { personal: false, orgs: ['o-sw2'] },
      'han-sa': { personal: false, orgs: ['o-sw1', 'o-sw2'] },
      'oh-sj': { personal: false, orgs: ['o-sw1', 'o-sw2'] },
    },
  };
  const initAB = J(ctx.pmCcInit(idx, itAB, h2o));
  assert.deepEqual(initAB.picks, {});
  assert.deepEqual(initAB.groups, { 'lee-dy': true, 'park-sy': true });
  const ctxAB = { origOrgCc: initAB.origOrgCc, ccSources: initAB.ccSources, ccExcluded: initAB.ccExcluded };
  // Unchecking o-sw1 while keeping o-sw2 emits ONLY item.uncc_org(o-sw1); han-sa and oh-sj stay effective via o-sw2
  const evABUncheckA = J(ctx.pmCcEval(idx, initAB.origCc, initAB.picks, { 'park-sy': true }, h2o, ctxAB));
  assert.equal(evABUncheckA.effective['han-sa'], true);
  assert.equal(evABUncheckA.effective['oh-sj'], true);
  assert.equal(evABUncheckA.retained['oh-sj'], true);
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, initAB.origCc, initAB.picks, { 'park-sy': true }, h2o, ctxAB)), [
    ['item.uncc_org', { item: 'i1', org: 'o-sw1' }],
  ]);
  // Unchecking BOTH o-sw1 and o-sw2 still never emits item.uncc(han-sa) or item.uncc(oh-sj)
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, initAB.origCc, initAB.picks, {}, h2o, ctxAB)), [
    ['item.uncc_org', { item: 'i1', org: 'o-sw1' }],
    ['item.uncc_org', { item: 'i1', org: 'o-sw2' }],
  ]);
});

test('Phase 2 review: global exclusion from projected ccExcluded (with events: []) stays uncovered when selecting a parent org and can be personally re-added', () => {
  const oi = J(ctx.orgIndex(ORGS, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;

  // han-sa was previously CC'd and unCC'd (server projects ccExcluded: ['han-sa']); no orgs registered and no hydrated events
  assert.deepEqual(J(ctx.pmCcExcluded({ id: 'i1', cc: [], ccExcluded: null })), {});
  assert.deepEqual(J(ctx.pmCcExcluded({ id: 'i1', cc: [] })), {});
  const it = { id: 'i1', cc: [], orgCC: [], ccSources: {}, ccExcluded: ['han-sa'] };
  const init = J(ctx.pmCcInit(idx, it, h2o));
  assert.equal(init.ccExcluded['han-sa'], true);
  const orgCtx = { origOrgCc: init.origOrgCc, ccSources: init.ccSources, ccExcluded: init.ccExcluded };

  // Selecting parent org o-dev (kim-hn, 8 members) must NOT cover or preview-add excluded han-sa
  const evParent = J(ctx.pmCcEval(idx, init.origCc, {}, { 'kim-hn': true }, h2o, orgCtx));
  assert.equal(evParent.cov.personBy['han-sa'], undefined);
  assert.equal(evParent.effective['han-sa'], undefined);
  assert.equal(evParent.added.includes('han-sa'), false);
  assert.equal(evParent.effectiveCount, 7); // 8 in o-dev minus excluded han-sa
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, {}, { 'kim-hn': true }, h2o, orgCtx)), [
    ['item.cc_org', { item: 'i1', org: 'o-dev' }],
  ]);

  // Personally toggling han-sa while o-dev is selected emits item.cc(han-sa) alongside item.cc_org(o-dev)
  const evReadd = J(ctx.pmCcEval(idx, init.origCc, { 'han-sa': true }, { 'kim-hn': true }, h2o, orgCtx));
  assert.equal(evReadd.effective['han-sa'], true);
  assert.equal(evReadd.added.includes('han-sa'), true);
  assert.equal(evReadd.effectiveCount, 8);
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, { 'han-sa': true }, { 'kim-hn': true }, h2o, orgCtx)), [
    ['item.cc_org', { item: 'i1', org: 'o-dev' }],
    ['item.cc', { item: 'i1', member: 'han-sa' }],
  ]);
});

test('Phase 2 review: plain-lead snapshot emits item.cc for every listed subtree member lacking a personal source, including org-only members', () => {
  const oi = J(ctx.orgIndex(ORGS, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;

  // park-sy has NO org; subtree is [park-sy, yoon-th, han-sa].
  // park-sy: CC'd only via o-dev; yoon-th: personal + o-dev; han-sa: CC'd only via deleted o-plat.
  const it = {
    id: 'i1',
    cc: ['park-sy', 'yoon-th', 'han-sa'],
    orgCC: [{ org: 'o-plat', added: ['han-sa'] }],
    ccSources: {
      'park-sy': { personal: false, orgs: ['o-plat'] },
      'yoon-th': { personal: true, orgs: ['o-plat'] },
      'han-sa': { personal: false, orgs: ['o-plat'] },
    },
  };
  const init = J(ctx.pmCcInit(idx, it, h2o));
  assert.deepEqual(init.picks, { 'yoon-th': true });
  const orgCtx = { origOrgCc: init.origOrgCc, ccSources: init.ccSources, ccExcluded: init.ccExcluded };

  // Turning on plain-lead park-sy "이하 전체" emits item.cc for park-sy and han-sa (lacking personal), skips yoon-th (already personal)
  const ev = J(ctx.pmCcEval(idx, init.origCc, init.picks, { 'park-sy': true }, h2o, orgCtx));
  assert.equal(ev.dirty, true);
  assert.deepEqual(ev.ccMembers, ['park-sy', 'han-sa']);
  assert.deepEqual(J(ctx.pmCcEvents('i1', idx, init.origCc, init.picks, { 'park-sy': true }, h2o, orgCtx)), [
    ['item.cc', { item: 'i1', member: 'park-sy' }],
    ['item.cc', { item: 'i1', member: 'han-sa' }],
  ]);
});

test('Phase 2 review: removed-head assign overlap locks descendants reachable from removed assignRoot', () => {
  // boss was removed, but lee-dy still carries leads: ['boss'] (server state_fold.go:421-449 keeps leads[]).
  // Display tree drops the boss -> lee-dy edge, so idx.desc['boss'] is absent, yet server check.go refuses rules below boss.
  const membersRemovedHead = {
    'boss': { leads: [], children: [], removed: 'ev-rm-boss' },
    'kim-hn': { leads: [], children: ['park-sy'], removed: false },
    'lee-dy': { leads: ['boss'], children: ['choi-jw', 'jung-mj'], removed: false },
    'choi-jw': { leads: ['lee-dy'], children: [], removed: false },
    'jung-mj': { leads: ['lee-dy'], children: [], removed: false },
    'park-sy': { leads: ['kim-hn'], children: ['han-sa'], removed: false },
    'han-sa': { leads: ['park-sy'], children: [], removed: false },
  };
  const orgs = [
    { id: 'o-old', name: '구개발실', head: '', removed: null }, // vacated when boss was removed
    { id: 'o-sw1', name: 'SW개발1팀', head: 'lee-dy', removed: null },
    { id: 'o-sw2', name: 'SW개발2팀', head: 'park-sy', removed: null },
  ];
  const oi = J(ctx.orgIndex(orgs, membersRemovedHead));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;
  assert.equal(idx.desc['boss'], undefined); // confirm display tree dropped removed lead

  const it = { id: 'i1', hidden: false, assignments: [], pending: [] };
  const rules = {
    'r-boss': { id: 'r-boss', item: 'i1', by: 'kim-hn', root: 'boss', assignRoot: 'boss', org: 'o-old', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
  };
  assert.equal(ctx.pmRuleCovered(idx, rules, 'i1', 'kim-hn', 'lee-dy', membersRemovedHead), true);
  assert.equal(ctx.pmRuleCovered(idx, rules, 'i1', 'kim-hn', 'choi-jw', membersRemovedHead), true);
  assert.equal(ctx.pmRuleCovered(idx, rules, 'i1', 'kim-hn', 'park-sy', membersRemovedHead), false);

  const locks = J(ctx.pmAssignLocks(idx, it, rules, {}, orgs, membersRemovedHead, 'kim-hn', h2o, {}, {}, null, false));
  assert.equal(!!locks.lockedG['lee-dy'], true); // cannot offer/emit item.assign for lee-dy / o-sw1
  assert.equal(!!locks.lockedG['park-sy'], false); // unrelated branch stays selectable

  const ev = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach: { 'lee-dy': 1, 'park-sy': 1 },
    lockedPeople: locks.lockedP, lockedGroups: locks.lockedG,
    picks: {}, groups: { 'lee-dy': true, 'park-sy': true },
    headToOrg: h2o,
  }));
  assert.deepEqual(ev.targets.map(t => t.root), ['park-sy']);
});

test('Phase 2 review: author-owned org rule stays unlocked with removal chip even under a broader plain rule', () => {
  const oi = J(ctx.orgIndex(ORGS, MEMBERS));
  const idx = J(ctx.pmIndex(oi.tree));
  const h2o = oi.headToOrg;
  const it = { id: 'i1', hidden: false, assignments: ['a-lee', 'a-choi'], pending: [] };
  const rules = {
    'r-plain': { id: 'r-plain', item: 'i1', by: 'kim-hn', root: 'kim-hn', assignRoot: 'kim-hn', org: null, includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
    'r-sw1': { id: 'r-sw1', item: 'i1', by: 'kim-hn', root: 'lee-dy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T02:00:00Z' },
  };
  const assignments = {
    'a-lee': { id: 'a-lee', item: 'i1', to: 'lee-dy', state: 'live', rule: 'r-sw1' },
    'a-choi': { id: 'a-choi', item: 'i1', to: 'choi-jw', state: 'live', rule: 'r-sw1' },
  };

  const init = J(ctx.pmAssignOrgInit(idx, it, rules, h2o, 'kim-hn'));
  assert.equal(init.origOrgAssign['lee-dy'].canRemove, true);
  assert.deepEqual(init.groups, { 'lee-dy': true });

  // Broader r-plain locks kim-hn & park-sy, but viewer-authored o-sw1 (lee-dy) stays unlocked!
  const locksOpen = J(ctx.pmAssignLocks(idx, it, rules, assignments, ORGS, MEMBERS, 'kim-hn', h2o, init.origOrgAssign, init.groups, null, false));
  assert.equal(!!locksOpen.lockedG['kim-hn'], true);
  assert.equal(!!locksOpen.lockedG['park-sy'], true);
  assert.equal(!!locksOpen.lockedG['lee-dy'], false);

  // Unchecking o-sw1 keeps lee-dy & choi-jw locked (r-plain still covers them), shows removal chip, and emits item.unassign {rule}
  const locksRm = J(ctx.pmAssignLocks(idx, it, rules, assignments, ORGS, MEMBERS, 'kim-hn', h2o, init.origOrgAssign, {}, null, false));
  assert.equal(!!locksRm.lockedG['lee-dy'], false);
  assert.equal(!!locksRm.lockedP['lee-dy'], true);
  assert.equal(!!locksRm.lockedP['choi-jw'], true);
  const evRm = J(ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach: { 'lee-dy': 1, 'choi-jw': 2 },
    lockedPeople: locksRm.lockedP, lockedGroups: locksRm.lockedG,
    picks: {}, groups: {}, headToOrg: h2o, origOrgAssign: init.origOrgAssign,
  }));
  assert.equal(evRm.dirty, true);
  assert.ok(evRm.chips.some(c => c.t === 'rmg' && c.org === 'o-sw1'));
  assert.deepEqual(J(ctx.pmAssignEvents('i1', evRm.targets, 'assignee', 'kim-hn', { 'lee-dy': 1 }, h2o, evRm.removedRules)), [
    ['item.unassign', { item: 'i1', rule: 'r-sw1', root: 'lee-dy' }],
  ]);
});

test('Phase 2 review: pmBuildApplyEvents validates open-time snapshot dependencies and flags stale instead of silently retargeting', () => {
  const oiOpen = J(ctx.orgIndex(ORGS, MEMBERS));
  const idxOpen = J(ctx.pmIndex(oiOpen.tree));
  const openH2o = oiOpen.headToOrg;

  // 1. Assign: o-sw1 (head lee-dy at open) concurrently re-headed to park-sy -> stale, never silently retargeted
  const curAsg = {
    kind: 'assign', item: 'i1', draft: false, norule: false, single: false, compl: 'assignee',
    oi: oiOpen, idx: idxOpen, headToOrg: openH2o, origOrgAssign: {}, picks: {}, groups: { 'lee-dy': true },
  };
  const liveUnchanged = {
    viewer: { member: 'kim-hn', reach: { 'lee-dy': 1, 'park-sy': 1 } },
    members: MEMBERS,
    orgs: ORGS,
    items: { i1: { id: 'i1', hidden: false, assignments: [], pending: [] } },
    rules: {}, assignments: {},
  };
  assert.deepEqual(J(ctx.pmBuildApplyEvents(curAsg, liveUnchanged)), {
    events: [['item.assign', { item: 'i1', targets: [{ root: 'lee-dy', include_root: true, org: 'o-sw1' }], completion: 'assigner' }]],
    stale: false,
  });

  const liveRehead = Object.assign({}, liveUnchanged, {
    orgs: [
      { id: 'o-dev', name: 'SW개발실', head: 'kim-hn', removed: null },
      { id: 'o-sw1', name: 'SW개발1팀', head: 'park-sy', removed: null },
    ],
  });
  assert.deepEqual(J(ctx.pmBuildApplyEvents(curAsg, liveRehead)), { events: [], stale: true });

  // 2. Assign: o-sw1 concurrently vacated -> stale, sends nothing even when another person was picked
  const curAsgVac = Object.assign({}, curAsg, { picks: { 'oh-sj': true } });
  const liveVacated = Object.assign({}, liveUnchanged, {
    orgs: [
      { id: 'o-dev', name: 'SW개발실', head: 'kim-hn', removed: null },
      { id: 'o-sw1', name: 'SW개발1팀', head: '', removed: null },
    ],
  });
  assert.deepEqual(J(ctx.pmBuildApplyEvents(curAsgVac, liveVacated)), { events: [], stale: true });

  // 3. CC: staged o-sw1 deleted before apply -> stale, sends nothing
  const curCc = {
    kind: 'cc', item: 'i1', draft: false, oi: oiOpen, idx: idxOpen, headToOrg: openH2o,
    origCc: ['kim-hn'], origOrgCc: [{ org: 'o-dev', added: ['kim-hn'] }],
    ccSources: { 'kim-hn': { personal: false, orgs: ['o-dev'] } },
    ccExcluded: {},
    origOrgByHead: { 'kim-hn': 'o-dev' },
    picks: { 'oh-sj': true }, unpicks: {}, groups: { 'lee-dy': true },
  };
  const liveCcMutated = {
    viewer: { member: 'kim-hn', reach: {} },
    members: MEMBERS,
    orgs: [
      { id: 'o-dev', name: 'SW개발실', head: '', removed: null },
      { id: 'o-sw1', name: 'SW개발1팀', head: '', removed: 'ev-del' },
    ],
    items: {
      i1: {
        id: 'i1', hidden: false, cc: ['kim-hn'],
        orgCC: [{ org: 'o-dev', added: ['kim-hn'] }],
        ccSources: { 'kim-hn': { personal: false, orgs: ['o-dev'] } },
      },
    },
    events: [],
  };
  assert.deepEqual(J(ctx.pmBuildApplyEvents(curCc, liveCcMutated)), { events: [], stale: true });
});

// Minimal HTML tree parser for driving real picker DOM click + keyboard handlers in Node.
function parseMiniDom(html) {
  const VOID = new Set(['input', 'br', 'hr', 'img', 'meta', 'link']);
  const root = makeEl('div', {});
  const stack = [root];
  const re = /<(\/)?([a-zA-Z0-9-]+)([^>]*)>|([^<]+)/g;
  let m;
  while ((m = re.exec(String(html || '')))) {
    if (m[4]) {
      stack[stack.length - 1].textContent += m[4];
      continue;
    }
    const closing = !!m[1], tag = m[2].toLowerCase(), rawAttrs = m[3] || '';
    if (closing) {
      while (stack.length > 1 && stack[stack.length - 1].tag !== tag) stack.pop();
      if (stack.length > 1) stack.pop();
      continue;
    }
    const attrs = {};
    const are = /([a-zA-Z0-9_:-]+)(?:\s*=\s*"([^"]*)")?/g;
    let am;
    while ((am = are.exec(rawAttrs))) attrs[am[1]] = am[2] !== undefined ? am[2] : '';
    const el = makeEl(tag, attrs);
    const parent = stack[stack.length - 1];
    el.parentNode = parent;
    parent.children.push(el);
    if (!VOID.has(tag) && !rawAttrs.trim().endsWith('/')) stack.push(el);
  }
  return root;

  function makeEl(tag, attrs) {
    const el = {
      tag,
      attrs,
      id: attrs.id || '',
      parentNode: null,
      children: [],
      scrollTop: 0,
      textContent: '',
      hidden: Object.prototype.hasOwnProperty.call(attrs, 'hidden'),
      classList: { toggle() {}, add() {}, remove() {} },
      getAttribute(k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; },
      setAttribute(k, v) { this.attrs[k] = String(v); },
      focus() {},
      scrollIntoView() {},
      matches(sel) {
        return sel.split(',').some(part => matchSimple(this, part.trim()));
      },
      closest(sel) {
        let cur = this;
        while (cur) {
          if (cur.matches(sel)) return cur;
          cur = cur.parentNode;
        }
        return null;
      },
      querySelectorAll(sel) {
        const out = [];
        (function walk(n) {
          for (const c of n.children) {
            if (c.matches(sel)) out.push(c);
            walk(c);
          }
        })(this);
        return out;
      },
      querySelector(sel) { return this.querySelectorAll(sel)[0] || null; },
    };
    return el;
  }

  function matchSimple(el, sel) {
    if (!el || !sel) return false;
    let rest = sel;
    const tagM = rest.match(/^([a-zA-Z0-9-]+)/);
    if (tagM) {
      if (el.tag !== tagM[1].toLowerCase()) return false;
      rest = rest.slice(tagM[0].length);
    }
    const idM = rest.match(/^#([a-zA-Z0-9_-]+)/);
    if (idM) {
      if (el.id !== idM[1]) return false;
      rest = rest.slice(idM[0].length);
    }
    while (rest.length) {
      if (rest[0] === '.') {
        const cm = rest.match(/^\.([a-zA-Z0-9_-]+)/);
        if (!cm) return false;
        const cls = String(el.attrs.class || '').split(/\s+/);
        if (!cls.includes(cm[1])) return false;
        rest = rest.slice(cm[0].length);
      } else if (rest[0] === '[') {
        const bm = rest.match(/^\[([a-zA-Z0-9_:-]+)(?:="([^"]*)")?\]/);
        if (!bm) return false;
        if (!Object.prototype.hasOwnProperty.call(el.attrs, bm[1])) return false;
        if (bm[2] !== undefined && el.attrs[bm[1]] !== bm[2]) return false;
        rest = rest.slice(bm[0].length);
      } else {
        return false;
      }
    }
    return true;
  }
}

function makePickerDomHarness(vsInit) {
  const dispatched = [];
  const byId = {};
  function makeHost(id) {
    let html = '', tree = parseMiniDom('');
    const h = {
      id,
      open: false,
      className: '',
      value: '',
      scrollTop: 0,
      hidden: false,
      textContent: '',
      classList: { toggle() {}, add() {}, remove() {} },
      setAttribute() {},
      focus() {},
      showModal() { this.open = true; },
      close() { this.open = false; },
      get innerHTML() { return html; },
      set innerHTML(v) {
        html = String(v || '');
        tree = parseMiniDom(html);
        tree.querySelectorAll('[id]').forEach(n => {
          if (!byId[n.id]) byId[n.id] = makeHost(n.id);
        });
      },
      querySelector(sel) { return tree.querySelector(sel); },
      querySelectorAll(sel) { return tree.querySelectorAll(sel); },
      closest(sel) { return sel === '#' + id ? this : null; },
    };
    byId[id] = h;
    return h;
  }
  makeHost('tkpm');
  makeHost('tkpmq');
  makeHost('tkpm-chips');
  makeHost('tkpm-body');
  makeHost('tkpm-foot');
  makeHost('tkpm-err');
  makeHost('tkpm-only');

  const c = {
    VS: J(vsInit),
    ui: { store: 's', nrow: {}, ned: null },
    AD: {
      dispatch(store, events, cb) {
        dispatched.push({ store, events: J(events) });
        cb({ ok: true, results: [] });
      },
    },
    $: id => byId[id] || null,
    CSS: { escape: s => String(s) },
    document: { querySelector: () => null },
    esc: s => String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'),
    wsT: (k, ...a) => (a.length ? `${k}:${a.join('|')}` : k),
    wsTn: (k, n, ...a) => `${k}:${n}${a.length ? '|' + a.join('|') : ''}`,
    nm: id => id,
    orgOf: () => '',
    orgNameOf: id => id,
    avHtml: id => `<i class="av">${id}</i>`,
    complText: c => c,
    hideTip: () => {},
    returnSel: () => null,
    listOf: x => (Array.isArray(x) ? x : []),
    wrote: () => {},
    render: () => {},
  };
  vm.createContext(c);
  vm.runInContext(section(trackerSource, '  function orgTree(', '  // Live, doing or blocked'), c);
  vm.runInContext(section(pickerSource, '  // --- picker modal pure logic', '  // --- end picker modal state & renderer'), c);
  return { c, byId, dispatched };
}

test('Phase 2 review: vacant-org row ignores click and keyboard navigation/Enter, and excluded/author-lock UI interactions work end-to-end', () => {
  const vs = {
    viewer: { member: 'kim-hn', reach: { 'lee-dy': 1, 'choi-jw': 2, 'jung-mj': 2, 'kang-yr': 2, 'park-sy': 1, 'yoon-th': 2, 'han-sa': 2 } },
    members: MEMBERS,
    orgs: ORGS,
    items: {
      i1: {
        id: 'i1', alias: 'k-1', kind: 'task', title: 'Test', hidden: false,
        cc: [], orgCC: [], ccSources: {}, ccExcluded: ['han-sa'],
        assignments: ['a-lee'], pending: [],
      },
    },
    rules: {
      'r-plain': { id: 'r-plain', item: 'i1', by: 'kim-hn', root: 'kim-hn', assignRoot: 'kim-hn', org: null, includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
      'r-sw1': { id: 'r-sw1', item: 'i1', by: 'kim-hn', root: 'lee-dy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T02:00:00Z' },
    },
    assignments: {
      'a-lee': { id: 'a-lee', item: 'i1', to: 'lee-dy', state: 'live', rule: 'r-sw1' },
    },
    events: [],
  };

  // 1. Vacant row click + keyboard in CC modal
  const { c, byId, dispatched } = makePickerDomHarness(vs);
  c.openPicker('cc', { item: 'i1' });
  const body = byId['tkpm-body'];
  const vacLi = body.querySelector('li[data-vac-org="o-qa"]');
  assert.ok(vacLi, 'vacant org o-qa should render');
  const vacRow = vacLi.querySelector('.pm-nd.vac');
  assert.equal(vacRow.getAttribute('aria-disabled'), 'true');

  // Clicking checkbox, avatar, label, row, and li on vacant org must do nothing
  for (const target of [vacRow.querySelector('.pm-ck'), vacRow.querySelector('.av'), vacRow.querySelector('.pm-who'), vacRow, vacLi]) {
    c.pmClick({ target, stopPropagation() {} });
  }
  assert.deepEqual(J(c.PM.picks), {});
  assert.deepEqual(J(c.PM.groups), {});

  // Keyboard ArrowDown / ArrowUp cycles ONLY live member rows and never lands on vacant orgs
  const visited = [];
  for (let i = 0; i < IDX.order.length + 2; i++) {
    c.pmKey({ key: 'ArrowDown', target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
    visited.push(c.PM.focusId);
  }
  assert.equal(visited.includes('o-qa'), false);
  assert.equal(visited.includes('o-gone-head'), false);
  assert.equal(visited[0], 'kim-hn');

  // Filter search down to ONLY vacant org "qa": ArrowDown + Enter in search box must NOT toggle previous focusId (kim-hn) or vacant row
  byId['tkpmq'].value = 'qa';
  c.pmInput({ target: byId['tkpmq'], isComposing: false });
  assert.ok(byId['tkpm-body'].querySelector('li[data-vac-org="o-qa"]'));
  assert.equal(byId['tkpm-body'].querySelectorAll('.pm-nd[data-id]').length, 0);
  c.pmKey({ key: 'ArrowDown', target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
  c.pmKey({ key: 'Enter', target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
  c.pmKey({ key: 'Enter', metaKey: true, target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
  assert.deepEqual(J(c.PM.picks), {});
  assert.deepEqual(J(c.PM.groups), {});
  assert.equal(dispatched.length, 0);

  // 2. Excluded han-sa under newly selected parent org o-dev stays enabled & clickable in DOM
  byId['tkpmq'].value = '';
  c.pmInput({ target: byId['tkpmq'], isComposing: false });
  const devGrpBtn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="kim-hn"]');
  c.pmClick({ target: devGrpBtn, stopPropagation() {} });
  const hanRow = byId['tkpm-body'].querySelector('.pm-nd[data-id="han-sa"]');
  const hanCk = hanRow.querySelector('[data-pm="self"][data-id="han-sa"]');
  assert.equal(hanRow.matches('.cvr'), false);
  assert.equal(hanRow.matches('.add'), false);
  assert.equal(hanCk.getAttribute('aria-disabled'), null);
  assert.equal(hanCk.getAttribute('aria-checked'), 'false');
  c.pmClick({ target: hanCk, stopPropagation() {} });
  assert.equal(c.PM.picks['han-sa'], true);
  c.pmKey({ key: 'Enter', metaKey: true, target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
  assert.equal(dispatched.length, 1);
  assert.deepEqual(dispatched[0].events.map(e => [e.type, e.payload]), [
    ['item.cc_org', { item: 'i1', org: 'o-dev' }],
    ['item.cc', { item: 'i1', member: 'han-sa' }],
  ]);

  // 3. Author-owned org rule (r-sw1) under broader plain rule (r-plain) keeps clickable checkbox & removal chip in assign modal
  c.openPicker('assign', { item: 'i1' });
  const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
  assert.equal(sw1Btn.matches('.lock'), false);
  assert.equal(sw1Btn.getAttribute('aria-disabled'), null);
  c.pmClick({ target: sw1Btn, stopPropagation() {} });
  const rmChip = byId['tkpm-chips'].querySelector('[data-pm="chip"][data-k="rmg:lee-dy"]');
  assert.ok(rmChip, 'removal chip rmg:lee-dy should render after unchecking author-owned org rule');
  c.pmKey({ key: 'Enter', metaKey: true, target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
  assert.equal(dispatched.length, 2);
  assert.deepEqual(dispatched[1].events.map(e => [e.type, e.payload]), [
    ['item.unassign', { item: 'i1', rule: 'r-sw1', root: 'lee-dy' }],
  ]);
});

test('Phase 2 re-review: live apply stale checks (vacancy repeat-click, plain-lead gains org, peer unCC, rule replacement, retarget old rule gone)', () => {
  const baseVs = () => ({
    viewer: { member: 'kim-hn', reach: { 'lee-dy': 1, 'choi-jw': 2, 'jung-mj': 2, 'kang-yr': 2, 'park-sy': 1, 'yoon-th': 2, 'han-sa': 2 } },
    members: J(MEMBERS),
    orgs: J(ORGS),
    items: {
      i1: {
        id: 'i1', alias: 'k-1', kind: 'task', title: 'Stale checks', hidden: false,
        cc: ['choi-jw'],
        orgCC: [],
        ccSources: { 'choi-jw': { personal: true, orgs: [] } },
        ccExcluded: [],
        assignments: [], pending: [],
      },
    },
    rules: {},
    assignments: {},
    events: [],
  });

  // 2a. Second Apply after vacancy (CC & Assign): 1st and 2nd Apply both keep modal open, show stale notice, send nothing
  for (const kind of ['cc', 'assign']) {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker(kind, { item: 'i1' });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    // Concurrently vacate o-sw1 in live VS
    c.VS.orgs = c.VS.orgs.map(o => (o.id === 'o-sw1' ? Object.assign({}, o, { head: '' }) : o));
    c.pmApply();
    assert.equal(dispatched.length, 0, `${kind}: 1st apply after vacancy sends nothing`);
    assert.equal(byId['tkpm'].open, true, `${kind}: modal stays open after 1st stale apply`);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
    assert.equal(byId['tkpm-err'].hidden, false);
    // Clicking Apply a second time must produce the exact same stale refusal, never falling back to a plain-lead payload
    c.pmApply();
    assert.equal(dispatched.length, 0, `${kind}: 2nd apply after vacancy still sends nothing`);
    assert.equal(byId['tkpm'].open, true, `${kind}: modal stays open after 2nd stale apply`);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 2b. Plain lead (park-sy) gains an org before Apply (CC & Assign) -> stale notice, no silent org attachment
  for (const kind of ['cc', 'assign']) {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker(kind, { item: 'i1' });
    const parkBtn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="park-sy"]');
    c.pmClick({ target: parkBtn, stopPropagation() {} });
    c.VS.orgs = c.VS.orgs.concat([{ id: 'o-sw2', name: 'SW개발2팀', head: 'park-sy', removed: null }]);
    c.pmApply();
    assert.equal(dispatched.length, 0, `${kind}: plain lead gaining org sends nothing`);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 3a. Peer unCC of an untouched initial personal CC (choi-jw) while user adds oh-sj -> sends ONLY item.cc(oh-sj), never re-adds choi-jw
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1' });
    const ohCk = byId['tkpm-body'].querySelector('[data-pm="self"][data-id="oh-sj"]');
    c.pmClick({ target: ohCk, stopPropagation() {} });
    // Peer concurrently unCCs untouched choi-jw
    c.VS.items.i1.cc = [];
    c.VS.items.i1.ccSources = {};
    c.VS.items.i1.ccExcluded = ['choi-jw'];
    c.pmApply();
    assert.equal(dispatched.length, 1);
    assert.deepEqual(dispatched[0].events.map(e => [e.type, e.payload]), [
      ['item.cc', { item: 'i1', member: 'oh-sj' }],
    ]);
  }

  // 3b. Peer unCC of a TOUCHED member (user unchecks choi-jw OR adds o-sw1 covering choi-jw) -> stale notice, sends nothing
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1' });
    const choiCk = byId['tkpm-body'].querySelector('[data-pm="self"][data-id="choi-jw"]');
    c.pmClick({ target: choiCk, stopPropagation() {} });
    c.VS.items.i1.cc = [];
    c.VS.items.i1.ccSources = {};
    c.VS.items.i1.ccExcluded = ['choi-jw'];
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1' });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    c.VS.items.i1.cc = [];
    c.VS.items.i1.ccSources = {};
    c.VS.items.i1.ccExcluded = ['choi-jw'];
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 4. Owned rule r1 replaced by r2 in another window -> stale notice, never unassigns r2
  {
    const vs = baseVs();
    vs.rules = {
      r1: { id: 'r1', item: 'i1', by: 'kim-hn', root: 'lee-dy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
    };
    const { c, byId, dispatched } = makePickerDomHarness(vs);
    c.openPicker('assign', { item: 'i1' });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    // Concurrently end r1 and replace with r2
    c.VS.rules.r1.state = 'ended';
    c.VS.rules.r1.ended = 'ev-un-r1';
    c.VS.rules.r2 = { id: 'r2', item: 'i1', by: 'kim-hn', root: 'lee-dy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T02:00:00Z' };
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 5. Deleted-org assign retarget after concurrent unassign of the old rule -> stale notice, sends nothing
  {
    const vs = baseVs();
    vs.rules = {
      'r-del': { id: 'r-del', item: 'i1', by: 'kim-hn', root: 'han-sa', assignRoot: 'han-sa', org: 'o-plat', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-20T01:00:00Z' },
    };
    const { c, byId, dispatched } = makePickerDomHarness(vs);
    c.openPicker('assign', { item: 'i1', retarget: { rule: 'r-del', org: 'o-plat', root: 'han-sa' } });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    // Concurrently unassign r-del in another window
    delete c.VS.rules['r-del'];
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }
});

test('Phase 2 round-3 review: deleted-org CC retarget to existing live org, plain-lead subtree drift, and item-scoped assign state drift', () => {
  const baseVs = () => ({
    viewer: { member: 'kim-hn', reach: { 'lee-dy': 1, 'choi-jw': 2, 'jung-mj': 2, 'kang-yr': 2, 'park-sy': 1, 'yoon-th': 2, 'han-sa': 2 } },
    members: J(MEMBERS),
    orgs: J(ORGS),
    items: {
      i1: {
        id: 'i1', alias: 'k-1', kind: 'task', title: 'Primary', hidden: false,
        cc: ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr', 'han-sa'],
        orgCC: [
          { org: 'o-sw1', added: ['lee-dy', 'choi-jw', 'jung-mj', 'kang-yr'] },
          { org: 'o-plat', added: ['han-sa'] },
        ],
        ccSources: {
          'lee-dy': { personal: false, orgs: ['o-sw1'] },
          'choi-jw': { personal: false, orgs: ['o-sw1'] },
          'jung-mj': { personal: false, orgs: ['o-sw1'] },
          'kang-yr': { personal: false, orgs: ['o-sw1'] },
          'han-sa': { personal: false, orgs: ['o-plat'] },
        },
        ccExcluded: [],
        assignments: [], pending: [],
      },
      i2: {
        id: 'i2', alias: 'k-2', kind: 'task', title: 'Other item', hidden: false,
        cc: [], orgCC: [], ccSources: {}, ccExcluded: [],
        assignments: [], pending: [],
      },
    },
    rules: {},
    assignments: {},
    events: [],
  });

  // 1. Deleted-org CC retarget selecting an org already registered live on the item (no concurrency) must succeed (not stale)
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1', retarget: { org: 'o-plat' } });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    c.pmApply();
    assert.equal(dispatched.length, 1);
    assert.deepEqual(dispatched[0].events.map(e => [e.type, e.payload]), [
      ['item.uncc_org', { item: 'i1', org: 'o-plat' }],
      ['item.cc_org', { item: 'i1', org: 'o-sw1' }],
    ]);
  }
  // If that existing org was concurrently unregistered before apply -> stale
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1', retarget: { org: 'o-plat' } });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    c.VS.items.i1.orgCC = [{ org: 'o-plat', added: ['han-sa'] }];
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 2. Plain-lead CC group (park-sy, no org): follower added OR follower moved out before Apply -> stale notice, sends nothing
  for (const mutate of [
    vs => { vs.members['oh-sj'].leads = ['park-sy']; vs.members['park-sy'].children.push('oh-sj'); },
    vs => { vs.members['han-sa'].leads = ['lee-dy']; vs.members['park-sy'].children = ['yoon-th']; },
  ]) {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1' });
    const parkBtn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="park-sy"]');
    c.pmClick({ target: parkBtn, stopPropagation() {} });
    mutate(c.VS);
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 3a. Assign mode: concurrent same-author rule on i1 covering staged target -> stale notice, sends nothing
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('assign', { item: 'i1' });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    c.VS.rules['r-cov'] = { id: 'r-cov', item: 'i1', by: 'kim-hn', root: 'kim-hn', assignRoot: 'kim-hn', org: 'o-dev', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-21T01:00:00Z' };
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 3b. Assign mode: concurrent assignment of the selected person on i1 -> stale notice, sends nothing
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('assign', { item: 'i1' });
    const ohCk = byId['tkpm-body'].querySelector('[data-pm="self"][data-id="oh-sj"]');
    c.pmClick({ target: ohCk, stopPropagation() {} });
    c.VS.assignments['a-oh'] = { id: 'a-oh', item: 'i1', to: 'oh-sj', state: 'live', rule: null };
    c.VS.items.i1.assignments.push('a-oh');
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
  }

  // 3c. Assign mode: changes to rules/assignments on ANOTHER item (i2) must NOT trigger stale on i1
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('assign', { item: 'i1' });
    const ohCk = byId['tkpm-body'].querySelector('[data-pm="self"][data-id="oh-sj"]');
    c.pmClick({ target: ohCk, stopPropagation() {} });
    c.VS.rules['r-other'] = { id: 'r-other', item: 'i2', by: 'kim-hn', root: 'kim-hn', assignRoot: 'kim-hn', org: 'o-dev', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-21T01:00:00Z' };
    c.VS.assignments['a-other'] = { id: 'a-other', item: 'i2', to: 'oh-sj', state: 'live', rule: null };
    c.VS.items.i2.assignments.push('a-other');
    c.pmApply();
    assert.equal(dispatched.length, 1);
    assert.deepEqual(dispatched[0].events.map(e => [e.type, e.payload]), [
      ['item.assign', { item: 'i1', targets: ['oh-sj'], completion: 'assignee' }],
    ]);
  }
});

test('Phase 2 round-4 review: plain-lead CC subtree member CC source/exclusion drift and dispatch refusal UI handling', () => {
  const baseVs = () => ({
    viewer: { member: 'kim-hn', reach: { 'lee-dy': 1, 'choi-jw': 2, 'jung-mj': 2, 'kang-yr': 2, 'park-sy': 1, 'yoon-th': 2, 'han-sa': 2 } },
    members: J(MEMBERS),
    orgs: J(ORGS),
    items: {
      i1: {
        id: 'i1', alias: 'k-1', kind: 'task', title: 'Primary', hidden: false,
        cc: ['han-sa', 'choi-jw'],
        orgCC: [],
        ccSources: {
          'han-sa': { personal: true, orgs: [] },
          'choi-jw': { personal: true, orgs: [] },
        },
        ccExcluded: [],
        assignments: [], pending: [],
      },
    },
    rules: {},
    assignments: {},
    events: [],
  });

  // 1a. Plain-lead CC group (park-sy): han-sa in park-sy's subtree was personally CC'd at open;
  // peer unCCs han-sa before Apply -> stale notice, sends nothing (never leaves han-sa excluded).
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1' });
    const parkBtn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="park-sy"]');
    c.pmClick({ target: parkBtn, stopPropagation() {} });
    // Concurrently unCC han-sa (now excluded, personal source removed)
    c.VS.items.i1.cc = ['choi-jw'];
    c.VS.items.i1.ccSources = { 'choi-jw': { personal: true, orgs: [] } };
    c.VS.items.i1.ccExcluded = ['han-sa'];
    c.pmApply();
    assert.equal(dispatched.length, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
    assert.equal(byId['tkpm-err'].hidden, false);
  }

  // 1b. Peer unCC of an untouched member OUTSIDE park-sy's subtree (choi-jw) does NOT trip stale
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    c.openPicker('cc', { item: 'i1' });
    const parkBtn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="park-sy"]');
    c.pmClick({ target: parkBtn, stopPropagation() {} });
    c.VS.items.i1.cc = ['han-sa'];
    c.VS.items.i1.ccSources = { 'han-sa': { personal: true, orgs: [] } };
    c.VS.items.i1.ccExcluded = ['choi-jw'];
    c.pmApply();
    assert.equal(dispatched.length, 1);
    assert.deepEqual(dispatched[0].events.map(e => [e.type, e.payload]), [
      ['item.cc', { item: 'i1', member: 'park-sy' }],
      ['item.cc', { item: 'i1', member: 'yoon-th' }],
    ]);
  }

  // 2a. Server 409 refusal on dispatch (e.g. concurrent reporting-line change making assign target covered):
  // modal stays open, server refusal reason is shown in #tkpm-err, selection is kept, wrote is not called.
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    let wroteCalled = 0;
    c.wrote = () => { wroteCalled++; };
    c.AD.dispatch = (store, events, cb) => {
      dispatched.push({ store, events: J(events) });
      cb({ ok: false, error: 'HTTP 409', refusal: { index: 0, type: 'item.assign', reason: 'target lee-dy already covered by rule r1' } });
    };
    c.openPicker('assign', { item: 'i1' });
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    c.pmApply();
    assert.equal(dispatched.length, 1);
    assert.equal(wroteCalled, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'target lee-dy already covered by rule r1');
    assert.equal(byId['tkpm-err'].hidden, false);
    assert.equal(c.PM.busy, false);
    assert.equal(c.PM.groups['lee-dy'], true);
    const applyBtn = byId['tkpm-foot'].querySelector('#tkpm-apply');
    assert.ok(applyBtn);
    assert.equal(applyBtn.getAttribute('disabled'), null);
  }

  // 2b. Non-409 dispatch failure (no refusal object): modal stays open and shows res.error
  {
    const { c, byId, dispatched } = makePickerDomHarness(baseVs());
    let wroteCalled = 0;
    c.wrote = () => { wroteCalled++; };
    c.AD.dispatch = (store, events, cb) => {
      dispatched.push({ store, events: J(events) });
      cb({ ok: false, error: 'HTTP 500: internal error', refusal: null });
    };
    c.openPicker('cc', { item: 'i1' });
    const ohCk = byId['tkpm-body'].querySelector('[data-pm="self"][data-id="oh-sj"]');
    c.pmClick({ target: ohCk, stopPropagation() {} });
    c.pmApply();
    assert.equal(dispatched.length, 1);
    assert.equal(wroteCalled, 0);
    assert.equal(byId['tkpm'].open, true);
    assert.equal(byId['tkpm-err'].textContent, 'HTTP 500: internal error');
    assert.equal(byId['tkpm-err'].hidden, false);
    assert.equal(c.PM.busy, false);
    assert.equal(c.PM.picks['oh-sj'], true);
  }
});

test('Phase 2 polish: org-badge rows omit org name from subtitle and stale notice disables Apply while keeping Cancel/close working', () => {
  const depts = {
    'kim-hn': 'SW개발실',
    'lee-dy': 'SW개발1팀',
    'choi-jw': 'SW개발1팀',
    'park-sy': 'SW개발2팀',
    'oh-sj': '',
  };
  const makeVs = () => ({
    viewer: { member: 'kim-hn', reach: { 'lee-dy': 1, 'choi-jw': 2, 'jung-mj': 2, 'kang-yr': 2, 'park-sy': 1, 'yoon-th': 2, 'han-sa': 2 } },
    members: J(MEMBERS),
    orgs: J(ORGS),
    items: {
      i1: {
        id: 'i1', alias: 'k-1', kind: 'task', title: 'Polish test', hidden: false,
        cc: ['choi-jw'], orgCC: [],
        ccSources: { 'choi-jw': { personal: true, orgs: [] } },
        ccExcluded: [],
        assignments: [], pending: [],
      },
    },
    rules: {},
    assignments: {},
    events: [],
  });

  for (const kind of ['cc', 'assign']) {
    const { c, byId, dispatched } = makePickerDomHarness(makeVs());
    c.orgOf = id => depts[id] || '';
    c.openPicker(kind, { item: 'i1' });

    // Org-badged lead rows omit org name from subtitle (badge + id only)
    const leeRow = byId['tkpm-body'].querySelector('.pm-nd[data-id="lee-dy"]');
    assert.ok(leeRow.querySelector('.tk-org-badge'), `${kind}: lee-dy renders org badge`);
    assert.equal(leeRow.querySelector('.pm-o').textContent, 'lee-dy', `${kind}: badged lead subtitle is id only`);

    const kimRow = byId['tkpm-body'].querySelector('.pm-nd[data-id="kim-hn"]');
    assert.ok(kimRow.querySelector('.tk-org-badge'), `${kind}: kim-hn renders org badge`);
    assert.equal(kimRow.querySelector('.pm-o').textContent, 'kim-hn', `${kind}: badged root lead subtitle is id only`);

    // Rows without an org badge keep "<dept> · <id>" when dept exists, or "<id>" when empty
    const parkRow = byId['tkpm-body'].querySelector('.pm-nd[data-id="park-sy"]');
    assert.equal(parkRow.querySelector('.tk-org-badge'), null, `${kind}: plain lead has no org badge`);
    assert.equal(parkRow.querySelector('.pm-o').textContent, 'SW개발2팀 &middot; park-sy', `${kind}: plain lead keeps dept · id`);

    const choiRow = byId['tkpm-body'].querySelector('.pm-nd[data-id="choi-jw"]');
    assert.equal(choiRow.querySelector('.tk-org-badge'), null, `${kind}: regular member has no org badge`);
    assert.equal(choiRow.querySelector('.pm-o').textContent, 'SW개발1팀 &middot; choi-jw', `${kind}: regular member keeps dept · id`);

    const ohRow = byId['tkpm-body'].querySelector('.pm-nd[data-id="oh-sj"]');
    assert.equal(ohRow.querySelector('.pm-o').textContent, 'oh-sj', `${kind}: member without dept shows id only`);

    // Stage org target -> Apply is enabled before drift
    const sw1Btn = byId['tkpm-body'].querySelector('[data-pm="grp"][data-id="lee-dy"]');
    c.pmClick({ target: sw1Btn, stopPropagation() {} });
    const applyBefore = byId['tkpm-foot'].querySelector('#tkpm-apply');
    assert.ok(applyBefore);
    assert.equal(applyBefore.getAttribute('disabled'), null, `${kind}: Apply enabled before stale`);

    // Concurrent org vacancy -> stale apply disables Apply while notice shows, Cancel stays enabled and closes
    c.VS.orgs = c.VS.orgs.map(o => (o.id === 'o-sw1' ? Object.assign({}, o, { head: '' }) : o));
    c.pmApply();
    assert.equal(dispatched.length, 0, `${kind}: stale apply sends nothing`);
    assert.equal(byId['tkpm'].open, true, `${kind}: modal stays open on stale`);
    assert.equal(byId['tkpm-err'].textContent, 'js.tk.pm.stale');
    assert.equal(byId['tkpm-err'].hidden, false);
    const applyAfter = byId['tkpm-foot'].querySelector('#tkpm-apply');
    assert.ok(applyAfter);
    assert.notEqual(applyAfter.getAttribute('disabled'), null, `${kind}: Apply is disabled while stale notice is showing`);

    // Keyboard Cmd/Ctrl+Enter while stale also sends nothing
    c.pmKey({ key: 'Enter', metaKey: true, target: byId['tkpmq'], stopPropagation() {}, preventDefault() {} });
    assert.equal(dispatched.length, 0);

    // Cancel button remains enabled and closes the modal
    const cancelBtn = byId['tkpm-foot'].querySelector('[data-pm="close"]');
    assert.ok(cancelBtn);
    assert.equal(cancelBtn.getAttribute('disabled'), null, `${kind}: Cancel stays enabled while stale`);
    c.pmClick({ target: cancelBtn, stopPropagation() {} });
    assert.equal(byId['tkpm'].open, false, `${kind}: Cancel closes stale modal`);
  }
});
