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
const J = x => JSON.parse(JSON.stringify(x));

// Mirrors wstracker sequential validation for admin, org_set, org_remove, and set_lead.
function simulateServer(initialMembers, initialOrgs, viewer, events) {
  const members = J(initialMembers);
  const orgs = {};
  (initialOrgs || []).forEach(o => { orgs[o.id] = J(o); });
  for (let i = 0; i < events.length; i++) {
    const ev = events[i];
    if (!members[viewer] || members[viewer].removed || !members[viewer].admin) {
      return { ok: false, refusal: { index: i, type: ev.type, reason: 'viewer not admin' } };
    }
    const p = ev.payload || {};
    if (ev.type === 'roles.add_admin') {
      const m = members[p.member];
      if (!m || m.removed || m.admin) return { ok: false, refusal: { index: i, type: ev.type, reason: 'bad add_admin' } };
      m.admin = true;
    } else if (ev.type === 'roles.remove_admin') {
      const m = members[p.member];
      const cnt = Object.values(members).filter(x => !x.removed && x.admin).length;
      if (!m || m.removed || !m.admin || cnt <= 1) return { ok: false, refusal: { index: i, type: ev.type, reason: 'bad remove_admin' } };
      m.admin = false;
    } else if (ev.type === 'roles.org_remove') {
      const o = orgs[p.org];
      if (!o || o.removed) return { ok: false, refusal: { index: i, type: ev.type, reason: 'org missing or removed' } };
      o.removed = 'ev-' + i;
      o.head = '';
    } else if (ev.type === 'roles.org_set') {
      const name = String(p.name || '');
      if (!name.trim() || name.trim() !== name) return { ok: false, refusal: { index: i, type: ev.type, reason: 'bad name' } };
      if (orgs[p.org] && orgs[p.org].removed) return { ok: false, refusal: { index: i, type: ev.type, reason: 'removed org' } };
      const dup = Object.values(orgs).find(o => !o.removed && o.id !== p.org && o.name.toLowerCase() === name.toLowerCase());
      if (dup) return { ok: false, refusal: { index: i, type: ev.type, reason: `dup name ${name}` } };
      const head = p.head || '';
      if (head) {
        if (!members[head] || members[head].removed) return { ok: false, refusal: { index: i, type: ev.type, reason: 'removed head' } };
        const held = Object.values(orgs).find(o => !o.removed && o.id !== p.org && o.head === head);
        if (held) return { ok: false, refusal: { index: i, type: ev.type, reason: `${head} already heads ${held.id}` } };
      }
      orgs[p.org] = { id: p.org, name, head, removed: null };
    } else if (ev.type === 'roles.set_lead') {
      const m = members[p.member];
      if (!m || m.removed) return { ok: false, refusal: { index: i, type: ev.type, reason: 'bad member' } };
      const leads = p.leads || [];
      if (leads.length) {
        let cur = leads[0];
        const seen = new Set();
        while (cur && !seen.has(cur)) {
          if (cur === p.member) return { ok: false, refusal: { index: i, type: ev.type, reason: 'cycle' } };
          seen.add(cur);
          cur = (members[cur] && members[cur].leads && members[cur].leads[0]) || '';
        }
      }
      m.leads = leads.slice();
    }
  }
  return { ok: true, members, orgs };
}

function fixture() {
  const members = {
    'kim-hn': { leads: [], admin: true, removed: false },
    'lee-dy': { leads: ['kim-hn'], admin: true, removed: false },
    'choi-jw': { leads: ['lee-dy'], admin: false, removed: false },
    'jung-mj': { leads: ['lee-dy'], admin: false, removed: false },
    'park-sy': { leads: ['kim-hn'], admin: false, removed: false },
    'han-sa': { leads: ['park-sy'], admin: false, removed: false },
    'oh-sj': { leads: [], admin: false, removed: false },
  };
  const orgs = [
    { id: 'o-dev', name: '개발본부', head: 'kim-hn', removed: null },
    { id: 'o-sw1', name: 'SW개발1팀', head: 'lee-dy', removed: null },
    { id: 'o-sw2', name: 'SW개발2팀', head: 'park-sy', removed: null },
    { id: 'o-plat', name: '플랫폼팀', head: '', removed: 'ev-del-plat' },
  ];
  return { members, orgs };
}

test('orgIndex derives live/removed partitions, sizeOf, nearestOrg, and orgSubtree', () => {
  const { members, orgs } = fixture();
  const oi = ctx.orgIndex(orgs, members);
  assert.deepEqual(J(oi.live.map(o => o.id)), ['o-dev', 'o-sw1', 'o-sw2']);
  assert.deepEqual(J(oi.removed.map(o => o.id)), ['o-plat']);
  assert.equal(oi.sizeOf('o-dev'), 6);
  assert.equal(oi.sizeOf('o-sw1'), 3);
  assert.equal(oi.sizeOf('o-sw2'), 2);
  assert.equal(oi.sizeOf('o-plat'), 0);
  assert.equal(oi.nearestOrg('choi-jw').id, 'o-sw1');
  assert.equal(oi.nearestOrg('lee-dy').id, 'o-sw1');
  assert.equal(oi.nearestOrg('kim-hn').id, 'o-dev');
  assert.equal(oi.nearestOrg('oh-sj'), null);
  assert.deepEqual(J(oi.orgSubtree('o-sw1')), ['lee-dy', 'choi-jw', 'jung-mj']);
});

test('orgMintId generates o-<4 base36> and avoids live, staged, and removed ids', () => {
  const seq = [0, 0, 0, 0, 0, 0, 0, 1 / 36]; // first try collides with o-0000, second produces o-0001
  let i = 0;
  const id = ctx.orgMintId({ 'o-0000': true }, () => seq[i++] || 0);
  assert.equal(id, 'o-0001');
  assert.match(id, /^o-[0-9a-z]{4}$/);
});

test('org editor staging: create, rename, head assign, refuse existing head, delete, undo conflict', () => {
  const { members, orgs } = fixture();
  const ed = ctx.orgEdInit(members, 'kim-hn', orgs);
  const c = J(ctx.orgEdCreateOrg(ed, 'QA팀', ''));
  assert.equal(c.ok, true);
  assert.equal(ctx.orgEdCount(ed), 1);

  // Refuses assigning lee-dy who already heads SW개발1팀 (Decision 6 / 9)
  const bad = J(ctx.orgEdStageOrgHead(ed, c.id, 'lee-dy'));
  assert.equal(bad.ok, false);
  assert.equal(bad.reason, 'already_head');
  assert.equal(bad.other.id, 'o-sw1');

  // Releasing lee-dy from o-sw1 first allows assigning lee-dy to QA팀
  assert.equal(ctx.orgEdStageOrgHead(ed, 'o-sw1', '').ok, true);
  assert.equal(ctx.orgEdStageOrgHead(ed, c.id, 'lee-dy').ok, true);
  assert.equal(ctx.orgEdCount(ed), 2);

  // Undoing o-sw1 while lee-dy is now held by QA팀 is refused cleanly
  assert.equal(ctx.orgEdUndo(ed, 'org:o-sw1'), false);
  assert.equal(ed.lastUndoErr.reason, 'already_head');

  // Undoing QA팀 succeeds, then undoing o-sw1 succeeds
  assert.equal(ctx.orgEdUndo(ed, 'org:' + c.id), true);
  assert.equal(ctx.orgEdUndo(ed, 'org:o-sw1'), true);
  assert.equal(ctx.orgEdCount(ed), 0);

  // Case-insensitive duplicate name rejected against live orgs, allowed against deleted org
  assert.equal(ctx.orgEdCreateOrg(ed, 'sw개발1팀', '').reason, 'dup_name');
  assert.equal(ctx.orgEdCreateOrg(ed, '플랫폼팀', '').ok, true);
});

test('Decision 12 save order: head release before take-up, delete + reuse head & name, and 2-org head swap vacate', () => {
  const { members, orgs } = fixture();
  // Case A: 2-org head swap (o-sw1: lee-dy -> park-sy, o-sw2: park-sy -> lee-dy)
  const ed1 = ctx.orgEdInit(members, 'kim-hn', orgs);
  // Stage via temporary vacancy in editor, then build against original live orgs
  assert.equal(ctx.orgEdStageOrgHead(ed1, 'o-sw1', '').ok, true);
  assert.equal(ctx.orgEdStageOrgHead(ed1, 'o-sw2', 'lee-dy').ok, true);
  assert.equal(ctx.orgEdStageOrgHead(ed1, 'o-sw1', 'park-sy').ok, true);
  const plan1 = J(ctx.orgEdPreflightAndBuild(ed1, members, 'kim-hn', orgs));
  assert.equal(plan1.ok, true);
  assert.equal(plan1.events.length, 3);
  // First event must vacate one org's head so the swap never trips 409
  assert.equal(plan1.events[0].type, 'roles.org_set');
  assert.equal(plan1.events[0].payload.head, '');
  const sim1 = simulateServer(members, orgs, 'kim-hn', plan1.events);
  assert.equal(sim1.ok, true);
  assert.equal(sim1.orgs['o-sw1'].head, 'park-sy');
  assert.equal(sim1.orgs['o-sw2'].head, 'lee-dy');

  // Case B: delete o-sw2 and immediately reuse its head (park-sy) and name in a new org + lead change + admin toggle
  const ed2 = ctx.orgEdInit(members, 'kim-hn', orgs);
  assert.equal(ctx.orgEdDeleteOrg(ed2, 'o-sw2').ok, true);
  const c2 = J(ctx.orgEdCreateOrg(ed2, 'SW개발2팀', 'park-sy'));
  assert.equal(c2.ok, true);
  assert.equal(ctx.orgEdToggleAdmin(ed2, 'park-sy').ok, true); // add_admin
  assert.equal(ctx.orgEdToggleAdmin(ed2, 'kim-hn').ok, true); // remove_admin (self last)
  assert.equal(ctx.orgEdStageLead(ed2, 'oh-sj', 'park-sy').ok, true);
  const plan2 = J(ctx.orgEdPreflightAndBuild(ed2, members, 'kim-hn', orgs));
  assert.equal(plan2.ok, true);
  assert.deepEqual(plan2.events.map(e => e.type), [
    'roles.add_admin',
    'roles.org_remove',
    'roles.org_set',
    'roles.set_lead',
    'roles.remove_admin',
  ]);
  const sim2 = simulateServer(members, orgs, 'kim-hn', plan2.events);
  assert.equal(sim2.ok, true);
  assert.ok(sim2.orgs['o-sw2'].removed);
  assert.equal(sim2.orgs[c2.id].head, 'park-sy');

  // Case C: 3-org head rotation (o-dev -> lee-dy, o-sw1 -> park-sy, o-sw2 -> kim-hn)
  const ed3 = ctx.orgEdInit(members, 'kim-hn', orgs);
  assert.equal(ctx.orgEdStageOrgHead(ed3, 'o-dev', '').ok, true);
  assert.equal(ctx.orgEdStageOrgHead(ed3, 'o-sw2', 'kim-hn').ok, true);
  assert.equal(ctx.orgEdStageOrgHead(ed3, 'o-sw1', 'park-sy').ok, true);
  assert.equal(ctx.orgEdStageOrgHead(ed3, 'o-dev', 'lee-dy').ok, true);
  const plan3 = J(ctx.orgEdPreflightAndBuild(ed3, members, 'kim-hn', orgs));
  assert.equal(plan3.ok, true);
  const sim3 = simulateServer(members, orgs, 'kim-hn', plan3.events);
  assert.equal(sim3.ok, true);
  assert.equal(sim3.orgs['o-dev'].head, 'lee-dy');
  assert.equal(sim3.orgs['o-sw1'].head, 'park-sy');
  assert.equal(sim3.orgs['o-sw2'].head, 'kim-hn');
});

test('Decision 10, 19 & 20: item-window org assign/CC derivations, coverage-based onlyThrough, and author gating', () => {
  const { members, orgs } = fixture();
  const it = {
    id: 'kim-hn#42', alias: 'kim-hn-42', author: 'kim-hn', hidden: false,
    assignments: ['a1', 'a2', 'a3', 'a4'], pending: [],
    cc: ['park-sy', 'han-sa', 'choi-jw', 'oh-sj'],
    ccSources: {
      'park-sy': { personal: false, orgs: ['o-sw2'] },
      'han-sa': { personal: true, orgs: ['o-sw2'] },
      'choi-jw': { personal: false, orgs: ['o-plat'] },
      'oh-sj': { personal: true, orgs: [] },
    },
    orgCC: [
      { org: 'o-sw2', added: ['park-sy', 'han-sa'] },
      { org: 'o-plat', added: ['choi-jw'] },
    ],
  };
  const rules = {
    'r-sw1': { id: 'r-sw1', item: 'kim-hn#42', by: 'kim-hn', root: 'lee-dy', assignRoot: 'lee-dy', org: 'o-sw1', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-19T01:00:00Z' },
    'r-plat': { id: 'r-plat', item: 'kim-hn#42', by: 'lee-dy', root: 'choi-jw', assignRoot: 'choi-jw', org: 'o-plat', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-19T02:00:00Z' },
  };
  const assignments = {
    a1: { id: 'a1', item: 'kim-hn#42', to: 'lee-dy', state: 'live', rule: 'r-sw1', fallback: false },
    a2: { id: 'a2', item: 'kim-hn#42', to: 'choi-jw', state: 'live', rule: 'r-plat', fallback: false },
    a3: { id: 'a3', item: 'kim-hn#42', to: 'jung-mj', state: 'live', rule: 'r-sw1', fallback: false },
    a4: { id: 'a4', item: 'kim-hn#42', to: 'oh-sj', state: 'live', rule: null, fallback: false },
  };

  // Author kim-hn sees canRemove/canRetarget on r-sw1 (authored by kim-hn), NOT on r-plat (authored by lee-dy)
  const asgByKim = J(ctx.itemOrgAssignInfo(it, rules, assignments, orgs, members, 'kim-hn', true));
  assert.equal(asgByKim.length, 2);
  assert.equal(asgByKim[0].orgId, 'o-sw1');
  assert.equal(asgByKim[0].deleted, false);
  assert.equal(asgByKim[0].canRemove, true);
  assert.equal(asgByKim[0].canRetarget, true);
  assert.equal(asgByKim[1].orgId, 'o-plat');
  assert.equal(asgByKim[1].deleted, true);
  assert.equal(asgByKim[1].canRemove, false);
  assert.equal(asgByKim[1].canRetarget, false);
  // choi-jw is also covered by live rule r-sw1 (lee-dy subtree), so removing r-plat leaves choi-jw stayed!
  assert.deepEqual(asgByKim[1].stayed.map(r => r.to), ['choi-jw']);
  assert.deepEqual(asgByKim[1].onlyThrough, []);

  // Rule author lee-dy who is NOT item creator canRemove r-plat, but cannot canRetarget (canAssignItem=false)
  const asgByLee = J(ctx.itemOrgAssignInfo(it, rules, assignments, orgs, members, 'lee-dy', false));
  assert.equal(asgByLee[1].canRemove, true);
  assert.equal(asgByLee[1].canRetarget, false);

  // CC org info: han-sa has personal CC too -> stayed; park-sy only through o-sw2 -> onlyThrough
  const ccInfo = J(ctx.itemOrgCCInfo(it, orgs, members));
  assert.equal(ccInfo.length, 2);
  assert.deepEqual(ccInfo[0].onlyThrough, ['park-sy']);
  assert.deepEqual(ccInfo[0].stayed, ['han-sa']);
  assert.equal(ccInfo[1].deleted, true);
  assert.equal(ccInfo[1].canRemove, true);
  assert.equal(ccInfo[1].canRetarget, true);

  // Deleted org summary & needsCleanup gating (Decision 20)
  // Item with ONLY a deleted assign rule authored by lee-dy: kim-hn sees assignDeletedCount=1, needsCleanup=false
  const itNoCcDel = Object.assign({}, it, { orgCC: [{ org: 'o-sw2', added: ['park-sy'] }] });
  const sumKim = J(ctx.itemDeletedOrgSummary(itNoCcDel, rules, orgs, 'kim-hn'));
  assert.equal(sumKim.assignDeletedCount, 1);
  assert.equal(sumKim.ccDeletedCount, 0);
  assert.equal(sumKim.needsCleanup, false);
  const sumLee = J(ctx.itemDeletedOrgSummary(itNoCcDel, rules, orgs, 'lee-dy'));
  assert.equal(sumLee.needsCleanup, true);
  // With deleted CC org present, every member sees needsCleanup=true
  const sumKimWithCc = J(ctx.itemDeletedOrgSummary(it, rules, orgs, 'kim-hn'));
  assert.equal(sumKimWithCc.ccDeletedCount, 1);
  assert.equal(sumKimWithCc.needsCleanup, true);
});

test('Decision 11 & 8: retarget batches always remove first, link org heads, keep non-org leads plain, and never uncc others', () => {
  const { members, orgs } = fixture();
  const oi = ctx.orgIndex(orgs, members);
  const idx = ctx.pmIndex(oi.tree);
  // Make oh-sj a 1-person org head to verify 1-person orgs also work as retarget targets
  const h2o = Object.assign({}, oi.headToOrg, { 'oh-sj': { id: 'o-qa', name: 'QA팀', head: 'oh-sj' } });

  // Assign retarget: replace r-plat with org head lee-dy (o-sw1), 1-person org head oh-sj (o-qa), and personal han-sa
  const reach = { 'lee-dy': true, 'choi-jw': true, 'jung-mj': true, 'park-sy': true, 'han-sa': true };
  const evAsg = ctx.pmAssignEval(idx, {
    viewer: 'kim-hn', reach,
    picks: { 'han-sa': true },
    groups: { 'lee-dy': true, 'oh-sj': true },
    headToOrg: h2o,
  });
  const batchAsg = J(ctx.pmRetargetAssignEvents('kim-hn#42', { rule: 'r-plat', root: 'choi-jw' }, evAsg.targets, 'assignee', 'kim-hn', reach, h2o));
  assert.deepEqual(batchAsg[0], ['item.unassign', { item: 'kim-hn#42', rule: 'r-plat', root: 'choi-jw' }]);
  assert.equal(batchAsg[1][0], 'item.assign');
  // lee-dy (in reach -> assigner) gets org: o-sw1; oh-sj (outside reach -> assignee) gets org: o-qa
  const allTargets = batchAsg.slice(1).flatMap(e => e[1].targets);
  assert.ok(allTargets.some(t => typeof t === 'object' && t.root === 'lee-dy' && t.org === 'o-sw1' && t.include_root === true));
  assert.ok(allTargets.some(t => typeof t === 'object' && t.root === 'oh-sj' && t.org === 'o-qa' && t.include_root === true));
  assert.ok(allTargets.includes('han-sa'));

  // Non-org lead retarget: strip o-sw2 from headToOrg so park-sy is a plain lead
  const h2oNoSw2 = { 'lee-dy': oi.headToOrg['lee-dy'] };
  const batchCc = J(ctx.pmRetargetCcEvents('kim-hn#42', { org: 'o-plat' }, idx, { 'oh-sj': true }, { 'lee-dy': true, 'park-sy': true }, h2oNoSw2));
  assert.deepEqual(batchCc, [
    ['item.uncc_org', { item: 'kim-hn#42', org: 'o-plat' }],
    ['item.cc_org', { item: 'kim-hn#42', org: 'o-sw1' }],
    ['item.cc', { item: 'kim-hn#42', member: 'park-sy' }],
    ['item.cc', { item: 'kim-hn#42', member: 'han-sa' }],
    ['item.cc', { item: 'kim-hn#42', member: 'oh-sj' }],
  ]);
  // Crucially: zero item.uncc events!
  assert.equal(batchCc.some(e => e[0] === 'item.uncc'), false);
});

test('Review regressions: pending record dedup, dead/vacant org non-coverage, and retarget lockedP with surviving live rules', () => {
  const { members, orgs } = fixture();
  const orgsWithVac = orgs.concat([{ id: 'o-vac', name: 'VacantOrg', head: '', removed: null }]);
  // ItemView places non-fallback pending records in BOTH it.assignments and it.pending (wstracker/view.go:784-790).
  const it = {
    id: 'kim-hn#50', alias: 'kim-hn-50', author: 'kim-hn', hidden: false,
    assignments: ['a-pend', 'a-plat', 'a-vac-only'],
    pending: ['a-pend'],
    cc: [], ccSources: {}, orgCC: [],
  };
  const rules = {
    'r-pend': { id: 'r-pend', item: 'kim-hn#50', by: 'kim-hn', root: 'park-sy', assignRoot: 'park-sy', org: 'o-sw2', includeRoot: true, levels: null, state: 'pending', ended: null, ts: '2026-11-19T01:00:00Z' },
    'r-plat': { id: 'r-plat', item: 'kim-hn#50', by: 'kim-hn', root: 'choi-jw', assignRoot: 'choi-jw', org: 'o-plat', includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-19T02:00:00Z' },
    'r-vac':  { id: 'r-vac',  item: 'kim-hn#50', by: 'kim-hn', root: 'lee-dy',  assignRoot: 'lee-dy',  org: 'o-vac',  includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-19T03:00:00Z' },
    'r-sw1':  { id: 'r-sw1',  item: 'kim-hn#50', by: 'kim-hn', root: 'lee-dy',  assignRoot: 'lee-dy',  org: 'o-sw1',  includeRoot: true, levels: null, state: 'live', ended: null, ts: '2026-11-19T04:00:00Z' },
  };
  const assignments = {
    'a-pend': { id: 'a-pend', item: 'kim-hn#50', to: 'park-sy', state: 'pending', rule: 'r-pend', ruleRequest: true, fallback: false },
    'a-plat': { id: 'a-plat', item: 'kim-hn#50', to: 'choi-jw', state: 'live', rule: 'r-plat', fallback: false },
    'a-vac-only': { id: 'a-vac-only', item: 'kim-hn#50', to: 'jung-mj', state: 'live', rule: 'r-plat', fallback: false },
  };

  // 1. Pending ruleRequest in both assignments and pending must NOT be double-counted.
  const info = J(ctx.itemOrgAssignInfo(it, rules, assignments, orgsWithVac, members, 'kim-hn', true));
  const pendInfo = info.find(x => x.rule.id === 'r-pend');
  assert.equal(pendInfo.recs.length, 1);
  assert.equal(pendInfo.onlyThrough.length, 1);

  // 2. Dead or vacant org rule whose stale root covers a member must NOT mark that member as stayed.
  const rulesDeadOnly = { 'r-plat': rules['r-plat'], 'r-vac': rules['r-vac'] };
  const infoDead = J(ctx.itemOrgAssignInfo(it, rulesDeadOnly, assignments, orgsWithVac, members, 'kim-hn', true));
  const platDead = infoDead.find(x => x.rule.id === 'r-plat');
  assert.deepEqual(platDead.stayed, []);
  assert.deepEqual(platDead.onlyThrough.map(r => r.to).sort(), ['choi-jw', 'jung-mj']);

  // 3. Retarget lockedP: skipping r-plat locks choi-jw & jung-mj when live r-sw1 covers them,
  //    but leaves them unlocked when only dead/vacant r-vac exists.
  const lockedWithLive = J(ctx.pmRetargetLockedPeople(it, rules, assignments, orgsWithVac, members, 'r-plat'));
  assert.equal(!!lockedWithLive['choi-jw'], true);
  assert.equal(!!lockedWithLive['jung-mj'], true);
  assert.equal(!!lockedWithLive['park-sy'], true);
  const lockedWithoutLive = J(ctx.pmRetargetLockedPeople(it, rulesDeadOnly, assignments, orgsWithVac, members, 'r-plat'));
  assert.equal(!!lockedWithoutLive['choi-jw'], false);
  assert.equal(!!lockedWithoutLive['jung-mj'], false);
});

test('Org helpers coverage: orgEdRenameOrg, orgEdMapRefusal, orgItemRefs, and 2-org name swap', () => {
  const { members, orgs } = fixture();
  const ed = ctx.orgEdInit(members, 'kim-hn', orgs);
  assert.equal(ctx.orgEdRenameOrg(ed, 'o-sw1', '   ').reason, 'blank_name');
  assert.equal(ctx.orgEdRenameOrg(ed, 'o-sw1', 'sw개발2팀').reason, 'dup_name');
  assert.equal(ctx.orgEdRenameOrg(ed, 'o-sw1', '플랫폼코어팀').ok, true);
  // Renaming back to base name removes the staged no-op
  assert.equal(ctx.orgEdRenameOrg(ed, 'o-sw1', 'SW개발1팀').ok, true);
  assert.equal(ctx.orgEdCount(ed), 0);

  // orgEdMapRefusal maps valid index to change key and out-of-range to _batch
  assert.deepEqual(J(ctx.orgEdMapRefusal(['org:o-sw1', 'lead:choi-jw'], { index: 1, reason: 'bad' })), { key: 'lead:choi-jw', reason: 'bad' });
  assert.deepEqual(J(ctx.orgEdMapRefusal(['org:o-sw1'], { index: 5, reason: 'oops' })), { key: '_batch', reason: 'oops' });

  // orgItemRefs counts assign + CC references per org and ignores hidden items
  const items = {
    'i1': { id: 'i1', alias: 'k-1', title: 'One', status: 'doing', hidden: false, orgCC: [{ org: 'o-plat', added: ['choi-jw'] }] },
    'i2': { id: 'i2', alias: 'k-2', title: 'Hidden', status: 'todo', hidden: true, orgCC: [{ org: 'o-plat', added: ['choi-jw'] }] },
  };
  const rmap = { 'r1': { id: 'r1', item: 'i1', org: 'o-plat', ended: null } };
  const refs = J(ctx.orgItemRefs(orgs, items, rmap, ['i1', 'i2']));
  assert.equal(refs['o-plat'].count, 1);
  assert.equal(refs['o-plat'].items[0].assign, true);
  assert.equal(refs['o-plat'].items[0].cc, true);

  // 2-org name swap via staging + server simulation
  const edSwap = ctx.orgEdInit(members, 'kim-hn', orgs);
  assert.equal(ctx.orgEdRenameOrg(edSwap, 'o-sw1', 'TempName').ok, true);
  assert.equal(ctx.orgEdRenameOrg(edSwap, 'o-sw2', 'SW개발1팀').ok, true);
  assert.equal(ctx.orgEdRenameOrg(edSwap, 'o-sw1', 'SW개발2팀').ok, true);
  const plan = ctx.orgEdPreflightAndBuild(edSwap, members, 'kim-hn', orgs);
  assert.equal(plan.ok, true, JSON.stringify(plan));
  assert.equal(plan.events.length, 3);
  const sim = simulateServer(members, orgs, 'kim-hn', plan.events);
  assert.equal(sim.ok, true, JSON.stringify(sim));
  assert.equal(sim.orgs['o-sw1'].name, 'SW개발2팀');
  assert.equal(sim.orgs['o-sw2'].name, 'SW개발1팀');
});

test('Lead review regressions: pmRuleCovered uses assignRoot & keeps dead/vacant org rules, and org name fold matches Greek σ/ς', () => {
  const { members, orgs } = fixture();
  const oi = ctx.orgIndex(orgs, members);
  const idx = ctx.pmIndex(oi.tree);

  // 1a. Surviving org rule assigned when head was lee-dy (assignRoot: lee-dy), later re-headed to park-sy (root: park-sy):
  //     server check.go judges overlap from assignRoot, so lee-dy (and descendants) MUST be locked, while park-sy is NOT locked.
  const reheadedRule = {
    'r-rehead': { id: 'r-rehead', item: 'kim-hn#60', by: 'kim-hn', org: 'o-sw1', root: 'park-sy', assignRoot: 'lee-dy', includeRoot: true, levels: null, state: 'live', ended: null },
  };
  assert.equal(ctx.pmRuleCovered(idx, reheadedRule, 'kim-hn#60', 'kim-hn', 'lee-dy'), true);
  assert.equal(ctx.pmRuleCovered(idx, reheadedRule, 'kim-hn#60', 'kim-hn', 'choi-jw'), true);
  assert.equal(ctx.pmRuleCovered(idx, reheadedRule, 'kim-hn#60', 'kim-hn', 'park-sy'), false);

  // 1b. Live rules whose org was deleted (o-plat) or vacated (o-vac) stay in check.go liveRules and lock their assignRoot.
  const deadAndVacRules = {
    'r-dead': { id: 'r-dead', item: 'kim-hn#60', by: 'kim-hn', org: 'o-plat', root: 'lee-dy', assignRoot: 'lee-dy', includeRoot: true, levels: null, state: 'live', ended: null },
    'r-vac':  { id: 'r-vac',  item: 'kim-hn#60', by: 'kim-hn', org: 'o-vac',  root: 'park-sy', assignRoot: 'park-sy', includeRoot: true, levels: null, state: 'live', ended: null },
  };
  assert.equal(ctx.pmRuleCovered(idx, deadAndVacRules, 'kim-hn#60', 'kim-hn', 'lee-dy'), true);
  assert.equal(ctx.pmRuleCovered(idx, deadAndVacRules, 'kim-hn#60', 'kim-hn', 'park-sy'), true);
  assert.equal(ctx.pmRuleCovered(idx, deadAndVacRules, 'kim-hn#60', 'kim-hn', 'kim-hn'), false);

  // 2. Org-name case-folding catches Greek σ vs ς (and ß vs ẞ) in both staging and preflight.
  const greekOrgs = orgs.concat([{ id: 'o-gr', name: 'Ομάδασ', head: '', removed: null }]);
  const ed = ctx.orgEdInit(members, 'kim-hn', greekOrgs);
  assert.equal(ctx.orgNameFoldKey('Ομάδασ'), ctx.orgNameFoldKey('Ομάδας'));
  assert.equal(ctx.orgNameFoldKey('Ομάδασ'), ctx.orgNameFoldKey('ΟΜΆΔΑΣ'));
  assert.equal(ctx.orgNameFoldKey('ß'), ctx.orgNameFoldKey('ẞ'));
  assert.equal(ctx.orgEdCreateOrg(ed, 'Ομάδας', '').reason, 'dup_name');
  assert.equal(ctx.orgEdRenameOrg(ed, 'o-sw1', 'Ομάδας').reason, 'dup_name');

  // Preflight also catches σ vs ς when another org was concurrently renamed on the live state.
  const edPre = ctx.orgEdInit(members, 'kim-hn', orgs);
  assert.equal(ctx.orgEdCreateOrg(edPre, 'Ομάδας', '').ok, true);
  const planPre = ctx.orgEdPreflightAndBuild(edPre, members, 'kim-hn', greekOrgs);
  assert.equal(planPre.ok, false);
  assert.ok(Object.values(planPre.errors).some(e => e.code === 'dup_name'));
});
