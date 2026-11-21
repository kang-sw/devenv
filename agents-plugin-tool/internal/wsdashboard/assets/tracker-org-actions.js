// Tracker org save events and item org rules.
  function orgEdPreflightAndBuild(ed, currentMembers, viewer, currentOrgs) {
    var M = currentMembers || {}, who = viewer || ed.viewer || '';
    var me = M[who];
    if (!me || me.removed || !me.admin) {
      return { ok: false, errors: { _viewer: { code: 'not_admin' } }, dropped: [], events: [], indexMap: [] };
    }
    var curTree = orgTree(M), curParent = curTree.parentOf;
    var curOrgs = {};
    var osrc = currentOrgs !== undefined ? currentOrgs : (ed.baseOrgs || {});
    var olist = Array.isArray(osrc) ? osrc : Object.keys(osrc || {}).map(function (k) { return osrc[k]; });
    olist.forEach(function (o) {
      if (o && o.id) curOrgs[o.id] = { id: o.id, name: o.name || '', head: o.head || '', removed: o.removed || null };
    });
    var errors = {}, dropped = [], activeLeads = {}, activeAdmins = {}, activeDeletes = [], activeSets = {};
    Object.keys(ed.stagedLeads).sort().forEach(function (id) {
      var key = 'lead:' + id, target = ed.stagedLeads[id] || '', curM = M[id];
      if (!curM || curM.removed) { errors[key] = { code: 'removed_member' }; return; }
      if (target && (!M[target] || M[target].removed)) { errors[key] = { code: 'removed_target', target: target }; return; }
      var curLead = curParent[id] || '', baseLead = ed.baseLeads[id] || '';
      if (curLead === target) { dropped.push(key); return; }
      if (curLead !== baseLead) { errors[key] = { code: 'stale_base', actual: curLead }; return; }
      activeLeads[id] = target;
    });
    Object.keys(ed.stagedAdmins).sort().forEach(function (id) {
      var key = 'admin:' + id, want = !!ed.stagedAdmins[id], curM = M[id];
      if (!curM || curM.removed) { errors[key] = { code: 'removed_member' }; return; }
      if (want === !!curM.admin) { dropped.push(key); return; }
      activeAdmins[id] = want;
    });
    Object.keys(ed.stagedOrgs || {}).sort().forEach(function (id) {
      var key = 'org:' + id, st = ed.stagedOrgs[id], cur = curOrgs[id], base = ed.baseOrgs && ed.baseOrgs[id];
      if (st.op === 'delete') {
        if (!cur || cur.removed) { dropped.push(key); return; }
        if (base && (cur.name !== base.name || (cur.head || '') !== (base.head || ''))) {
          errors[key] = { code: 'stale_base' }; return;
        }
        activeDeletes.push(id);
        return;
      }
      var wantName = String(st.name || '').trim(), wantHead = st.head || '';
      if (st.op === 'create') {
        if (cur) { errors[key] = { code: 'org_exists' }; return; }
        if (!wantName) { errors[key] = { code: 'blank_name' }; return; }
        if (wantHead && (!M[wantHead] || M[wantHead].removed)) { errors[key] = { code: 'removed_head', target: wantHead }; return; }
        activeSets[id] = { op: 'create', name: wantName, head: wantHead };
        return;
      }
      if (!cur || cur.removed) { errors[key] = { code: 'removed_org' }; return; }
      if (!wantName) { errors[key] = { code: 'blank_name' }; return; }
      if (wantHead && (!M[wantHead] || M[wantHead].removed)) { errors[key] = { code: 'removed_head', target: wantHead }; return; }
      if (cur.name === wantName && (cur.head || '') === wantHead) { dropped.push(key); return; }
      if (base && (cur.name !== base.name || (cur.head || '') !== (base.head || ''))) {
        errors[key] = { code: 'stale_base' }; return;
      }
      activeSets[id] = { op: 'edit', name: wantName, head: wantHead };
    });
    var finalLeads = {};
    Object.keys(curParent).forEach(function (id) { finalLeads[id] = curParent[id]; });
    Object.keys(activeLeads).forEach(function (id) { finalLeads[id] = activeLeads[id]; });
    var finalTree = orgTree(M, finalLeads);
    var unreach = {};
    finalTree.unreachable.forEach(function (id) { unreach[id] = true; });
    Object.keys(activeLeads).forEach(function (id) {
      if (unreach[id] || (activeLeads[id] && orgEdIsInSubtree(finalLeads, activeLeads[id], id))) {
        errors['lead:' + id] = { code: 'cycle' };
      }
    });
    var finalAdminCount = 0;
    Object.keys(curParent).forEach(function (id) {
      var isAdm = Object.prototype.hasOwnProperty.call(activeAdmins, id) ? activeAdmins[id] : !!(M[id] && M[id].admin);
      if (isAdm) finalAdminCount++;
    });
    if (finalAdminCount < 1) {
      Object.keys(activeAdmins).forEach(function (id) {
        if (activeAdmins[id] === false) errors['admin:' + id] = { code: 'last_admin' };
      });
    }
    var finalOrgs = {}, delSet = {};
    activeDeletes.forEach(function (id) { delSet[id] = true; });
    Object.keys(curOrgs).forEach(function (id) {
      var o = curOrgs[id];
      if (!o.removed && !delSet[id]) finalOrgs[id] = { id: id, name: o.name, head: o.head || '' };
    });
    Object.keys(activeSets).forEach(function (id) {
      finalOrgs[id] = { id: id, name: activeSets[id].name, head: activeSets[id].head || '' };
    });
    var nameSeen = {}, headSeen = {};
    Object.keys(finalOrgs).sort().forEach(function (id) {
      var o = finalOrgs[id], low = orgNameFoldKey(o.name);
      if (nameSeen[low]) {
        if (activeSets[id]) errors['org:' + id] = { code: 'dup_name' };
        if (activeSets[nameSeen[low]]) errors['org:' + nameSeen[low]] = { code: 'dup_name' };
      } else nameSeen[low] = id;
      if (o.head) {
        if (headSeen[o.head]) {
          if (activeSets[id]) errors['org:' + id] = { code: 'already_head', target: o.head };
          if (activeSets[headSeen[o.head]]) errors['org:' + headSeen[o.head]] = { code: 'already_head', target: o.head };
        } else headSeen[o.head] = id;
      }
    });
    if (Object.keys(errors).length) {
      return { ok: false, errors: errors, dropped: dropped, events: [], indexMap: [] };
    }
    var events = [], indexMap = [];
    // 1. roles.add_admin first
    Object.keys(activeAdmins).sort().forEach(function (id) {
      if (activeAdmins[id] === true) {
        events.push({ type: 'roles.add_admin', payload: { member: id }, origin: 'human' });
        indexMap.push('admin:' + id);
      }
    });
    // 2. roles.org_remove
    var simOrgs = {};
    Object.keys(curOrgs).forEach(function (id) {
      if (!curOrgs[id].removed) simOrgs[id] = { name: curOrgs[id].name, head: curOrgs[id].head || '' };
    });
    activeDeletes.sort().forEach(function (id) {
      events.push({ type: 'roles.org_remove', payload: { org: id }, origin: 'human' });
      indexMap.push('org:' + id);
      delete simOrgs[id];
    });
    function headOwner(h) {
      if (!h) return '';
      var ids = Object.keys(simOrgs);
      for (var i = 0; i < ids.length; i++) if (simOrgs[ids[i]].head === h) return ids[i];
      return '';
    }
    function nameOwner(n) {
      var low = orgNameFoldKey(n), ids = Object.keys(simOrgs);
      for (var i = 0; i < ids.length; i++) if (orgNameFoldKey(simOrgs[ids[i]].name) === low) return ids[i];
      return '';
    }
    // 3. Pure head releases first (frees heads before any take-up)
    var remaining = Object.keys(activeSets).sort();
    var nextRem = [];
    remaining.forEach(function (id) {
      var want = activeSets[id], sim = simOrgs[id];
      if (sim && sim.head && !want.head) {
        var no = nameOwner(want.name);
        if (!no || no === id) {
          events.push({ type: 'roles.org_set', payload: { org: id, name: want.name, head: '' }, origin: 'human' });
          indexMap.push('org:' + id);
          simOrgs[id] = { name: want.name, head: '' };
          return;
        }
        events.push({ type: 'roles.org_set', payload: { org: id, name: sim.name, head: '' }, origin: 'human' });
        indexMap.push('org:' + id);
        simOrgs[id].head = '';
      }
      nextRem.push(id);
    });
    remaining = nextRem;
    // 4. Topological org_set (head take-ups, renames, creates), breaking head/name cycles with a vacate event
    var guard = remaining.length * 3 + 4;
    while (remaining.length && guard-- > 0) {
      var ready = [];
      remaining.forEach(function (id) {
        var want = activeSets[id];
        var ho = headOwner(want.head), no = nameOwner(want.name);
        if ((!ho || ho === id) && (!no || no === id)) ready.push(id);
      });
      if (ready.length) {
        var remSet = {};
        ready.forEach(function (id) {
          var want = activeSets[id];
          events.push({ type: 'roles.org_set', payload: { org: id, name: want.name, head: want.head }, origin: 'human' });
          indexMap.push('org:' + id);
          simOrgs[id] = { name: want.name, head: want.head };
          remSet[id] = true;
        });
        remaining = remaining.filter(function (id) { return !remSet[id]; });
        continue;
      }
      var vacId = '';
      for (var i = 0; i < remaining.length; i++) {
        var cid = remaining[i], h = simOrgs[cid] && simOrgs[cid].head;
        if (h && remaining.some(function (r) { return r !== cid && activeSets[r].head === h; })) { vacId = cid; break; }
      }
      if (vacId) {
        events.push({ type: 'roles.org_set', payload: { org: vacId, name: simOrgs[vacId].name, head: '' }, origin: 'human' });
        indexMap.push('org:' + vacId);
        simOrgs[vacId].head = '';
        continue;
      }
      var renId = remaining[0];
      if (simOrgs[renId]) {
        var tmp = simOrgs[renId].name + ' #' + renId;
        while (nameOwner(tmp)) tmp += '_';
        events.push({ type: 'roles.org_set', payload: { org: renId, name: tmp, head: simOrgs[renId].head }, origin: 'human' });
        indexMap.push('org:' + renId);
        simOrgs[renId].name = tmp;
      }
    }
    // 5. roles.set_lead in ascending depth of the final tree
    Object.keys(activeLeads).sort(function (a, b) {
      var da = finalTree.depthOf[a] || 0, db = finalTree.depthOf[b] || 0;
      return da !== db ? da - db : (a < b ? -1 : a > b ? 1 : 0);
    }).forEach(function (id) {
      var target = activeLeads[id];
      events.push({ type: 'roles.set_lead', payload: { member: id, leads: target ? [target] : [] }, origin: 'human' });
      indexMap.push('lead:' + id);
    });
    // 6. roles.remove_admin last, saving viewer's own removal very last
    var rms = Object.keys(activeAdmins).filter(function (id) { return activeAdmins[id] === false; }).sort(function (a, b) {
      if (a === who && b !== who) return 1;
      if (b === who && a !== who) return -1;
      return a < b ? -1 : a > b ? 1 : 0;
    });
    rms.forEach(function (id) {
      events.push({ type: 'roles.remove_admin', payload: { member: id }, origin: 'human' });
      indexMap.push('admin:' + id);
    });
    return { ok: true, errors: {}, dropped: dropped, events: events, indexMap: indexMap };
  }
  function orgEdMapRefusal(indexMap, refusal) {
    if (!refusal) return { key: '_batch', reason: '' };
    var idx = typeof refusal.index === 'number' ? refusal.index : -1;
    var key = idx >= 0 && idx < indexMap.length ? indexMap[idx] : '_batch';
    return { key: key, reason: refusal.reason || '' };
  }
  // Pure item-window org helpers (Decisions 10, 11, 19 & 20).
  function itemOrgRules(it, rules) {
    if (!it || !rules) return [];
    var list = Array.isArray(rules) ? rules : Object.keys(rules).map(function (k) { return rules[k]; });
    return list.filter(function (ru) {
      return ru && !ru.ended && ru.org && ru.item === it.id;
    }).sort(function (a, b) {
      var ta = a.ts || '', tb = b.ts || '';
      return ta !== tb ? (ta < tb ? -1 : 1) : (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
    });
  }
  function ruleCoversMember(ru, memberId, ot, oi, members) {
    if (!ru || ru.ended || ru.state !== 'live' || !memberId) return false;
    var root = ru.root || '';
    if (ru.org && oi) {
      var o = oi.byId && oi.byId[ru.org];
      var M = members || {};
      if (!o || o.removed || !o.head || (members && (!M[o.head] || M[o.head].removed))) return false;
      root = o.head;
    }
    if (!root) return false;
    if (memberId === root) return !!ru.includeRoot;
    var parentOf = (ot && ot.parentOf) || {};
    var cur = parentOf[memberId] || '', steps = 1, seen = {};
    seen[memberId] = true;
    while (cur && !seen[cur]) {
      if (cur === root) return ru.levels == null || steps <= ru.levels;
      seen[cur] = true;
      cur = parentOf[cur] || '';
      steps++;
    }
    return false;
  }
  function itemOrgAssignInfo(it, rules, assignments, orgs, members, viewer, canAssignItem) {
    if (!it) return [];
    var M = members || {}, oi = orgIndex(orgs, M), ot = oi.tree;
    var rlist = Array.isArray(rules) ? rules : Object.keys(rules || {}).map(function (k) { return rules[k]; });
    var openRules = rlist.filter(function (ru) { return ru && !ru.ended && ru.item === it.id; });
    var orgRules = itemOrgRules(it, openRules);
    if (!orgRules.length) return [];
    var amap = assignments || {};
    var activeRecs = [], seenRec = {};
    (it.assignments || []).concat(it.pending || []).forEach(function (aid) {
      var r = typeof aid === 'string' ? amap[aid] : aid;
      if (!r || !r.id || seenRec[r.id]) return;
      seenRec[r.id] = true;
      if ((r.state === 'live' || r.state === 'pending') && !r.fallback) activeRecs.push(r);
    });
    var canAsg = canAssignItem !== undefined ? !!canAssignItem : (!it.hidden && it.author === viewer);
    return orgRules.map(function (ru) {
      var org = oi.byId[ru.org] || { id: ru.org, name: ru.org, head: '', removed: 'deleted' };
      var deleted = !!org.removed;
      var vacant = !deleted && (!org.head || !(M[org.head] && !M[org.head].removed));
      var recs = activeRecs.filter(function (r) { return r.rule === ru.id; });
      var onlyThrough = [], stayed = [];
      recs.forEach(function (r) {
        if (r.ruleRequest) { onlyThrough.push(r); return; }
        var coveredElsewhere = openRules.some(function (o) {
          return o.id !== ru.id && ruleCoversMember(o, r.to, ot, oi, M);
        });
        if (coveredElsewhere) stayed.push(r);
        else onlyThrough.push(r);
      });
      var canRemove = !it.hidden && ru.by === viewer;
      return {
        rule: ru, org: org, orgId: ru.org, deleted: deleted, vacant: vacant,
        pending: ru.state === 'pending', recs: recs, onlyThrough: onlyThrough, stayed: stayed,
        canRemove: canRemove, canRetarget: canRemove && canAsg
      };
    });
  }
  function itemOrgCCInfo(it, orgs, members) {
    if (!it || !Array.isArray(it.orgCC) || !it.orgCC.length) return [];
    var M = members || {}, oi = orgIndex(orgs, M), srcs = it.ccSources || {};
    var ccSet = {};
    (it.cc || []).forEach(function (m) { ccSet[m] = true; });
    return it.orgCC.map(function (reg) {
      var org = oi.byId[reg.org] || { id: reg.org, name: reg.org, head: '', removed: 'deleted' };
      var deleted = !!org.removed;
      var vacant = !deleted && (!org.head || !(M[org.head] && !M[org.head].removed));
      var added = (reg.added || []).filter(function (m) {
        return M[m] ? !M[m].removed && (!it.cc || ccSet[m]) : true;
      });
      var onlyThrough = [], stayed = [];
      added.forEach(function (m) {
        var s = srcs[m];
        var otherOrgs = s && Array.isArray(s.orgs) ? s.orgs.filter(function (oid) { return oid !== reg.org; }) : [];
        if (s && (s.personal || otherOrgs.length > 0)) stayed.push(m);
        else onlyThrough.push(m);
      });
      return {
        org: org, orgId: reg.org, deleted: deleted, vacant: vacant,
        added: added, onlyThrough: onlyThrough, stayed: stayed,
        canRemove: !it.hidden, canRetarget: !it.hidden
      };
    });
  }
  function itemDeletedOrgSummary(it, rules, orgs, viewer) {
    if (!it) return { assignDeletedCount: 0, ccDeletedCount: 0, needsCleanup: false };
    var oi = orgIndex(orgs, {});
    function isDel(oid) { var o = oi.byId[oid]; return !o || !!o.removed; }
    var assignDeletedCount = 0, authorHasDeletedAssign = false;
    itemOrgRules(it, rules).forEach(function (ru) {
      if (isDel(ru.org)) {
        assignDeletedCount++;
        if (ru.by === viewer) authorHasDeletedAssign = true;
      }
    });
    var ccDeletedCount = 0;
    (it.orgCC || []).forEach(function (reg) {
      if (reg && reg.org && isDel(reg.org)) ccDeletedCount++;
    });
    var needsCleanup = !it.hidden && (ccDeletedCount > 0 || authorHasDeletedAssign);
    return { assignDeletedCount: assignDeletedCount, ccDeletedCount: ccDeletedCount, needsCleanup: needsCleanup };
  }
  function orgAssignUnassignEvent(itemId, ru) {
    var p = { item: itemId, rule: ru.id };
    var root = ru.assignRoot || ru.root;
    if (root) p.root = root;
    return ['item.unassign', p];
  }
  function orgCCUnccEvent(itemId, orgId) {
    return ['item.uncc_org', { item: itemId, org: orgId }];
  }
