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
const norm = x => JSON.parse(JSON.stringify(x));

// Mirrors wstracker/tracker.go sequential validation (add_admin, set_lead cycle guard, remove_admin last-admin guard)
function simulateServerDispatch(initialMembers, viewer, events) {
  const members = norm(initialMembers);
  for (let i = 0; i < events.length; i++) {
    const ev = events[i];
    if (!members[viewer] || members[viewer].removed || !members[viewer].admin) {
      return { ok: false, refusal: { index: i, type: ev.type, reason: 'viewer is not an admin' } };
    }
    const id = ev.payload.member;
    const m = members[id];
    if (!m || m.removed) return { ok: false, refusal: { index: i, type: ev.type, reason: 'member removed or unknown' } };
    if (ev.type === 'roles.add_admin') {
      if (m.admin) return { ok: false, refusal: { index: i, type: ev.type, reason: 'already admin' } };
      m.admin = true;
    } else if (ev.type === 'roles.remove_admin') {
      if (!m.admin) return { ok: false, refusal: { index: i, type: ev.type, reason: 'not an admin' } };
      const count = Object.values(members).filter(x => !x.removed && x.admin).length;
      if (count <= 1) return { ok: false, refusal: { index: i, type: ev.type, reason: 'cannot remove the last admin' } };
      m.admin = false;
    } else if (ev.type === 'roles.set_lead') {
      const leads = ev.payload.leads || [];
      if (leads.length) {
        const target = leads[0];
        if (!members[target] || members[target].removed) return { ok: false, refusal: { index: i, type: ev.type, reason: 'lead removed or unknown' } };
        let cur = target;
        const seen = new Set();
        while (cur && !seen.has(cur)) {
          if (cur === id) return { ok: false, refusal: { index: i, type: ev.type, reason: 'lead cycle' } };
          seen.add(cur);
          cur = (members[cur].leads && members[cur].leads[0]) || '';
        }
      }
      m.leads = leads.slice();
    }
  }
  return { ok: true, members };
}

function sampleMembers() {
  return {
    'kim-hn': { leads: [], admin: true, removed: false },
    'lee-dy': { leads: ['kim-hn'], admin: true, removed: false },
    'choi-jw': { leads: ['lee-dy'], admin: false, removed: false },
    'jung-mj': { leads: ['lee-dy'], admin: false, removed: false },
    'park-sy': { leads: ['kim-hn'], admin: false, removed: false },
    'han-sa': { leads: ['park-sy'], admin: false, removed: false },
    'oh-sj': { leads: [], admin: false, removed: false },
  };
}

test('batch ordering: add_admin first, set_lead ascending final depth (including lead swap & 3-node reversal), remove_admin last with self very last', () => {
  const members = {
    a: { leads: [], admin: true, removed: false },
    b: { leads: ['a'], admin: true, removed: false },
    c: { leads: ['b'], admin: false, removed: false },
  };
  const ed = ctx.orgEdInit(members, 'a');
  // Reverse 3-node chain a -> b -> c into c -> b -> a, promote c to admin, remove both b and self (a)
  assert.equal(ctx.orgEdStageLead(ed, 'c', '').ok, true);
  assert.equal(ctx.orgEdStageLead(ed, 'b', 'c').ok, true);
  assert.equal(ctx.orgEdStageLead(ed, 'a', 'b').ok, true);
  assert.equal(ctx.orgEdToggleAdmin(ed, 'c').ok, true); // add_admin c
  assert.equal(ctx.orgEdToggleAdmin(ed, 'b').ok, true); // remove_admin b
  assert.equal(ctx.orgEdToggleAdmin(ed, 'a').ok, true); // remove_admin a (self)

  const plan = norm(ctx.orgEdPreflightAndBuild(ed, members, 'a'));
  assert.equal(plan.ok, true);
  assert.deepEqual(plan.events.map(e => [e.type, e.payload.member, e.payload.leads]), [
    ['roles.add_admin', 'c', undefined],
    ['roles.set_lead', 'c', []],
    ['roles.set_lead', 'b', ['c']],
    ['roles.set_lead', 'a', ['b']],
    ['roles.remove_admin', 'b', undefined],
    ['roles.remove_admin', 'a', undefined],
  ]);
  assert.deepEqual(plan.indexMap, ['admin:c', 'lead:c', 'lead:b', 'lead:a', 'admin:b', 'admin:a']);

  // Sequential server validation must accept every event without intermediate cycle or losing admin rights
  const sim = simulateServerDispatch(members, 'a', plan.events);
  assert.equal(sim.ok, true);
  assert.deepEqual(sim.members.c.leads, []);
  assert.deepEqual(sim.members.b.leads, ['c']);
  assert.deepEqual(sim.members.a.leads, ['b']);
  assert.equal(sim.members.c.admin, true);
  assert.equal(sim.members.b.admin, false);
  assert.equal(sim.members.a.admin, false);
});

test('batch ordering: 2-node lead swap under a shared parent never trips an intermediate cycle', () => {
  const members = {
    top: { leads: [], admin: true, removed: false },
    lead: { leads: ['top'], admin: false, removed: false },
    sub: { leads: ['lead'], admin: false, removed: false },
  };
  const ed = ctx.orgEdInit(members, 'top');
  assert.equal(ctx.orgEdStageLead(ed, 'sub', 'top').ok, true);
  assert.equal(ctx.orgEdStageLead(ed, 'lead', 'sub').ok, true);
  const plan = norm(ctx.orgEdPreflightAndBuild(ed, members, 'top'));
  assert.equal(plan.ok, true);
  assert.deepEqual(plan.events.map(e => [e.payload.member, e.payload.leads]), [
    ['sub', ['top']],
    ['lead', ['sub']],
  ]);
  assert.equal(simulateServerDispatch(members, 'top', plan.events).ok, true);
});

test('cycle detection refuses self and descendants for drag and select, updates after staged moves', () => {
  const ed = ctx.orgEdInit(sampleMembers(), 'kim-hn');
  const leads = ctx.orgEdProjectedLeads(ed);
  assert.equal(ctx.orgEdIsInSubtree(leads, 'jung-mj', 'lee-dy'), true);
  assert.equal(ctx.orgEdIsInSubtree(leads, 'lee-dy', 'lee-dy'), true);
  assert.equal(ctx.orgEdIsInSubtree(leads, 'park-sy', 'lee-dy'), false);
  assert.equal(ctx.orgEdStageLead(ed, 'lee-dy', 'jung-mj').ok, false);
  assert.equal(ctx.orgEdStageLead(ed, 'lee-dy', 'jung-mj').reason, 'cycle');
  // Move han-sa under lee-dy -> now lee-dy cannot move under han-sa
  assert.equal(ctx.orgEdStageLead(ed, 'han-sa', 'lee-dy').ok, true);
  assert.equal(ctx.orgEdStageLead(ed, 'lee-dy', 'han-sa').ok, false);
});

test('last-admin lock counts the final admin set (current + staged adds - staged removals)', () => {
  const sole = {
    a: { leads: [], admin: true, removed: false },
    b: { leads: ['a'], admin: false, removed: false },
  };
  const ed = ctx.orgEdInit(sole, 'a');
  assert.equal(ctx.orgEdCanRemoveAdmin(ed, 'a'), false);
  assert.equal(ctx.orgEdToggleAdmin(ed, 'a').reason, 'last_admin');
  // Staging add_admin(b) unlocks self-removal for a
  assert.equal(ctx.orgEdToggleAdmin(ed, 'b').ok, true);
  assert.equal(ctx.orgEdCanRemoveAdmin(ed, 'a'), true);
  assert.equal(ctx.orgEdToggleAdmin(ed, 'a').ok, true);
  assert.equal(ctx.orgEdAdminCount(ed), 1);
  // Undoing add_admin(b) restores remove_admin(a) so final admin count stays >= 1
  assert.equal(ctx.orgEdUndo(ed, 'admin:b'), true);
  assert.equal(ctx.orgEdAdminCount(ed), 1);
  assert.equal(ctx.orgEdCount(ed), 0);
});

test('409 refusal.index maps back to the exact staged change key', () => {
  const indexMap = ['admin:oh-sj', 'lead:han-sa', 'admin:lee-dy'];
  assert.deepEqual(norm(ctx.orgEdMapRefusal(indexMap, { index: 1, type: 'roles.set_lead', reason: 'cycle detected' })), {
    key: 'lead:han-sa',
    reason: 'cycle detected',
  });
  assert.deepEqual(norm(ctx.orgEdMapRefusal(indexMap, { index: 2, type: 'roles.remove_admin', reason: 'last admin' })), {
    key: 'admin:lee-dy',
    reason: 'last admin',
  });
  assert.deepEqual(norm(ctx.orgEdMapRefusal(indexMap, { index: 99, type: 'x', reason: 'bad' })), {
    key: '_batch',
    reason: 'bad',
  });
});

test('staged-change undo, undo-all, and discard confirmation guard', () => {
  const ed = ctx.orgEdInit(sampleMembers(), 'kim-hn');
  assert.equal(ctx.orgEdCloseNeedsConfirm(ed), false);
  ctx.orgEdStageLead(ed, 'han-sa', 'lee-dy');
  ctx.orgEdStageLead(ed, 'oh-sj', 'park-sy');
  ctx.orgEdToggleAdmin(ed, 'lee-dy');
  assert.equal(ctx.orgEdCount(ed), 3);
  assert.equal(ctx.orgEdCloseNeedsConfirm(ed), true);
  // Single undo
  assert.equal(ctx.orgEdUndo(ed, 'lead:oh-sj'), true);
  assert.equal(ctx.orgEdCount(ed), 2);
  // Re-staging back to original lead clears the staged change automatically
  ctx.orgEdStageLead(ed, 'han-sa', 'park-sy');
  assert.equal(ctx.orgEdCount(ed), 1);
  // Undo all
  ctx.orgEdUndoAll(ed);
  assert.equal(ctx.orgEdCount(ed), 0);
  assert.equal(ctx.orgEdCloseNeedsConfirm(ed), false);
});

test('save-time pre-flight check (Decision 8): drops no-ops, blocks stale base, highlights all invalid changes at once', () => {
  const initial = sampleMembers();
  const ed = ctx.orgEdInit(initial, 'kim-hn');
  ctx.orgEdStageLead(ed, 'han-sa', 'lee-dy');    // base park-sy -> lee-dy
  ctx.orgEdStageLead(ed, 'oh-sj', 'park-sy');    // base '' -> park-sy
  ctx.orgEdStageLead(ed, 'choi-jw', 'park-sy');  // base lee-dy -> park-sy
  ctx.orgEdToggleAdmin(ed, 'park-sy');           // add_admin park-sy
  ctx.orgEdToggleAdmin(ed, 'lee-dy');            // remove_admin lee-dy

  // Meanwhile, current view changed before save:
  // - han-sa was ALREADY moved to lee-dy -> no-op (dropped)
  // - park-sy is ALREADY admin -> no-op (dropped)
  // - oh-sj was moved by another admin to kim-hn -> stale_base!
  // - choi-jw was removed -> removed_member!
  const current = norm(initial);
  current['han-sa'].leads = ['lee-dy'];
  current['park-sy'].admin = true;
  current['oh-sj'].leads = ['kim-hn'];
  current['choi-jw'].removed = true;

  const plan = norm(ctx.orgEdPreflightAndBuild(ed, current, 'kim-hn'));
  assert.equal(plan.ok, false);
  assert.deepEqual(plan.dropped.sort(), ['admin:park-sy', 'lead:han-sa']);
  // Both invalid changes are flagged at once (not stopping at the first)
  assert.equal(plan.errors['lead:oh-sj'].code, 'stale_base');
  assert.equal(plan.errors['lead:oh-sj'].actual, 'kim-hn');
  assert.equal(plan.errors['lead:choi-jw'].code, 'removed_member');

  // If only no-ops remain, ok: true with empty events and dropped list
  const ed2 = ctx.orgEdInit(initial, 'kim-hn');
  ctx.orgEdStageLead(ed2, 'han-sa', 'lee-dy');
  ctx.orgEdToggleAdmin(ed2, 'park-sy');
  const plan2 = norm(ctx.orgEdPreflightAndBuild(ed2, current, 'kim-hn'));
  assert.equal(plan2.ok, true);
  assert.deepEqual(plan2.events, []);
  assert.deepEqual(plan2.dropped.sort(), ['admin:park-sy', 'lead:han-sa']);

  // Last-admin error detected at pre-flight if another admin removed kim-hn's peer or only 1 admin left
  const ed3 = ctx.orgEdInit(initial, 'kim-hn');
  ctx.orgEdToggleAdmin(ed3, 'kim-hn'); // stage removing kim-hn while lee-dy was admin
  const currentOneAdmin = norm(initial);
  currentOneAdmin['lee-dy'].admin = false; // lee-dy was removed as admin meanwhile
  const plan3 = norm(ctx.orgEdPreflightAndBuild(ed3, currentOneAdmin, 'kim-hn'));
  assert.equal(plan3.ok, false);
  assert.equal(plan3.errors['admin:kim-hn'].code, 'last_admin');
});
