// Tracker org editor state and staged changes.
  // Pure state and validation helpers for the admin org-chart editor modal (Decisions 4-12, 18).
  function orgEdInit(members, viewer, orgs) {
    var ot = orgTree(members), baseLeads = {}, baseAdmins = {}, M = members || {};
    Object.keys(ot.parentOf).forEach(function (id) {
      baseLeads[id] = ot.parentOf[id] || '';
      baseAdmins[id] = !!(M[id] && M[id].admin);
    });
    var baseOrgs = {}, baseOrgOrder = [];
    var olist = Array.isArray(orgs) ? orgs : (orgs ? Object.keys(orgs).map(function (k) { return orgs[k]; }) : []);
    olist.forEach(function (o) {
      if (!o || !o.id) return;
      baseOrgs[o.id] = { id: o.id, name: o.name || '', head: o.head || '', removed: o.removed || null };
      baseOrgOrder.push(o.id);
    });
    return {
      viewer: viewer || '', baseLeads: baseLeads, baseAdmins: baseAdmins,
      baseOrgs: baseOrgs, baseOrgOrder: baseOrgOrder,
      stagedLeads: {}, stagedAdmins: {}, stagedOrgs: {},
      fold: {}, goneOpen: {}, editingOrg: null, q: '',
      drag: null, dragKind: null, over: null, refuse: null, lastUndoErr: null, errors: {}, saving: false
    };
  }
  function orgEdProjectedOrgs(ed) {
    var out = {}, bo = ed.baseOrgs || {}, so = ed.stagedOrgs || {};
    Object.keys(bo).forEach(function (id) {
      var o = bo[id], st = so[id];
      if (o.removed) return;
      if (st && st.op === 'delete') return;
      if (st && st.op === 'edit') {
        out[id] = { id: id, name: st.name, head: st.head || '', removed: null, staged: 'edit' };
      } else {
        out[id] = { id: id, name: o.name, head: o.head || '', removed: null, staged: null };
      }
    });
    Object.keys(so).forEach(function (id) {
      var st = so[id];
      if (st && st.op === 'create') {
        out[id] = { id: id, name: st.name, head: st.head || '', removed: null, staged: 'create' };
      }
    });
    return out;
  }
  function orgEdAllDisplayOrgs(ed) {
    var out = [], bo = ed.baseOrgs || {}, so = ed.stagedOrgs || {}, seen = {};
    (ed.baseOrgOrder || Object.keys(bo)).forEach(function (id) {
      var o = bo[id];
      if (!o || o.removed) return;
      seen[id] = true;
      var st = so[id];
      if (st && st.op === 'delete') {
        out.push({ id: id, name: o.name, head: o.head || '', removed: null, staged: 'delete' });
      } else if (st && st.op === 'edit') {
        out.push({ id: id, name: st.name, head: st.head || '', removed: null, staged: 'edit' });
      } else {
        out.push({ id: id, name: o.name, head: o.head || '', removed: null, staged: null });
      }
    });
    Object.keys(so).forEach(function (id) {
      if (seen[id]) return;
      var st = so[id];
      if (st && st.op === 'create') {
        out.push({ id: id, name: st.name, head: st.head || '', removed: null, staged: 'create' });
      }
    });
    return out;
  }
  function orgEdHeadOf(ed, memberId) {
    if (!memberId) return null;
    var proj = orgEdProjectedOrgs(ed), ids = Object.keys(proj);
    for (var i = 0; i < ids.length; i++) {
      if (proj[ids[i]].head === memberId) return proj[ids[i]];
    }
    return null;
  }
  // Unicode case-folding key for org-name uniqueness, at least as strict as Go's strings.EqualFold
  // (collapses e.g. Greek σ/ς/Σ and German ß/ẞ into a single fold key).
  function orgNameFoldKey(s) {
    return String(s || '').trim().toUpperCase().toLowerCase().toUpperCase().toLowerCase();
  }
  function orgEdNameClash(ed, name, excludeId) {
    var low = orgNameFoldKey(name);
    if (!low) return null;
    var proj = orgEdProjectedOrgs(ed), ids = Object.keys(proj);
    for (var i = 0; i < ids.length; i++) {
      var o = proj[ids[i]];
      if (o.id !== excludeId && orgNameFoldKey(o.name) === low) return o;
    }
    return null;
  }
  function orgEdCreateOrg(ed, name, head, rng) {
    var trimmed = String(name || '').trim();
    if (!trimmed) return { ok: false, reason: 'blank_name' };
    var clash = orgEdNameClash(ed, trimmed, '');
    if (clash) return { ok: false, reason: 'dup_name', other: clash };
    var target = head || '';
    if (target) {
      if (!Object.prototype.hasOwnProperty.call(ed.baseLeads, target)) return { ok: false, reason: 'unknown_member' };
      var held = orgEdHeadOf(ed, target);
      if (held) return { ok: false, reason: 'already_head', other: held };
    }
    var used = {};
    Object.keys(ed.baseOrgs || {}).forEach(function (id) { used[id] = true; });
    Object.keys(ed.stagedOrgs || {}).forEach(function (id) { used[id] = true; });
    var id = orgMintId(used, rng);
    ed.stagedOrgs[id] = { op: 'create', name: trimmed, head: target };
    delete ed.errors['org:' + id];
    ed.refuse = null;
    return { ok: true, id: id };
  }
  function orgEdRenameOrg(ed, orgId, newName) {
    var proj = orgEdProjectedOrgs(ed), cur = proj[orgId];
    if (!cur) return { ok: false, reason: 'unknown_org' };
    var trimmed = String(newName || '').trim();
    if (!trimmed) return { ok: false, reason: 'blank_name' };
    var clash = orgEdNameClash(ed, trimmed, orgId);
    if (clash) return { ok: false, reason: 'dup_name', other: clash };
    var st = ed.stagedOrgs[orgId];
    if (st && st.op === 'create') {
      st.name = trimmed;
    } else {
      var base = ed.baseOrgs[orgId];
      if (trimmed === base.name && cur.head === base.head) delete ed.stagedOrgs[orgId];
      else ed.stagedOrgs[orgId] = { op: 'edit', name: trimmed, head: cur.head };
    }
    delete ed.errors['org:' + orgId];
    return { ok: true };
  }
  function orgEdStageOrgHead(ed, orgId, newHead) {
    var proj = orgEdProjectedOrgs(ed), cur = proj[orgId];
    if (!cur) return { ok: false, reason: 'unknown_org' };
    var target = newHead || '';
    if (target) {
      if (!Object.prototype.hasOwnProperty.call(ed.baseLeads, target)) return { ok: false, reason: 'unknown_member' };
      var held = orgEdHeadOf(ed, target);
      if (held && held.id !== orgId) return { ok: false, reason: 'already_head', other: held };
    }
    var st = ed.stagedOrgs[orgId];
    if (st && st.op === 'create') {
      st.head = target;
    } else {
      var base = ed.baseOrgs[orgId];
      if (cur.name === base.name && target === base.head) delete ed.stagedOrgs[orgId];
      else ed.stagedOrgs[orgId] = { op: 'edit', name: cur.name, head: target };
    }
    delete ed.errors['org:' + orgId];
    ed.refuse = null;
    return { ok: true };
  }
  function orgEdDeleteOrg(ed, orgId) {
    var st = ed.stagedOrgs[orgId];
    if (st && st.op === 'create') {
      delete ed.stagedOrgs[orgId];
      delete ed.errors['org:' + orgId];
      if (ed.editingOrg === orgId) ed.editingOrg = null;
      return { ok: true };
    }
    var base = ed.baseOrgs && ed.baseOrgs[orgId];
    if (!base || base.removed) return { ok: false, reason: 'unknown_org' };
    ed.stagedOrgs[orgId] = { op: 'delete' };
    delete ed.errors['org:' + orgId];
    if (ed.editingOrg === orgId) ed.editingOrg = null;
    ed.refuse = null;
    return { ok: true };
  }
  function orgEdProjectedLeads(ed) {
    var out = {};
    Object.keys(ed.baseLeads).forEach(function (id) { out[id] = ed.baseLeads[id]; });
    Object.keys(ed.stagedLeads).forEach(function (id) { out[id] = ed.stagedLeads[id]; });
    return out;
  }
  function orgEdProjectedAdmins(ed) {
    var out = {};
    Object.keys(ed.baseAdmins).forEach(function (id) { out[id] = !!ed.baseAdmins[id]; });
    Object.keys(ed.stagedAdmins).forEach(function (id) { out[id] = !!ed.stagedAdmins[id]; });
    return out;
  }
  function orgEdAdminCount(ed) {
    var pa = orgEdProjectedAdmins(ed), n = 0;
    Object.keys(pa).forEach(function (id) { if (pa[id]) n++; });
    return n;
  }
  function orgEdIsInSubtree(leadsMap, candidate, memberId) {
    var seen = {};
    for (var x = candidate; x && !seen[x]; x = leadsMap[x]) {
      if (x === memberId) return true;
      seen[x] = true;
    }
    return false;
  }
  function orgEdCanRemoveAdmin(ed, id) {
    var pa = orgEdProjectedAdmins(ed);
    return !pa[id] || orgEdAdminCount(ed) > 1;
  }
  function orgEdStageLead(ed, memberId, newLead) {
    var target = newLead || '';
    if (!Object.prototype.hasOwnProperty.call(ed.baseLeads, memberId)) return { ok: false, reason: 'unknown_member' };
    var leads = orgEdProjectedLeads(ed);
    if (target && orgEdIsInSubtree(leads, target, memberId)) return { ok: false, reason: 'cycle' };
    if (target === (ed.baseLeads[memberId] || '')) delete ed.stagedLeads[memberId];
    else ed.stagedLeads[memberId] = target;
    delete ed.errors['lead:' + memberId];
    ed.refuse = null;
    return { ok: true };
  }
  function orgEdToggleAdmin(ed, id) {
    if (!Object.prototype.hasOwnProperty.call(ed.baseAdmins, id)) return { ok: false, reason: 'unknown_member' };
    var pa = orgEdProjectedAdmins(ed), cur = !!pa[id];
    if (cur && !orgEdCanRemoveAdmin(ed, id)) return { ok: false, reason: 'last_admin' };
    var next = !cur;
    if (next === !!ed.baseAdmins[id]) delete ed.stagedAdmins[id];
    else ed.stagedAdmins[id] = next;
    delete ed.errors['admin:' + id];
    return { ok: true };
  }
  function orgEdUndo(ed, key) {
    var sep = String(key || '').indexOf(':');
    if (sep < 0) return false;
    var kind = key.slice(0, sep), id = key.slice(sep + 1);
    if (kind === 'lead') {
      if (!Object.prototype.hasOwnProperty.call(ed.stagedLeads, id)) return false;
      var nextLeads = orgEdProjectedLeads(ed);
      var orig = ed.baseLeads[id] || '';
      delete nextLeads[id];
      if (orig && orgEdIsInSubtree(nextLeads, orig, id)) return false;
      delete ed.stagedLeads[id];
      delete ed.errors[key];
      return true;
    }
    if (kind === 'admin') {
      if (!Object.prototype.hasOwnProperty.call(ed.stagedAdmins, id)) return false;
      var wasAdd = ed.stagedAdmins[id] === true;
      delete ed.stagedAdmins[id];
      delete ed.errors[key];
      // Undoing a staged add_admin could leave 0 admins if an existing admin's removal was also staged;
      // restore staged removals so the editor never holds a state the server refuses.
      if (wasAdd && orgEdAdminCount(ed) < 1) {
        Object.keys(ed.stagedAdmins).forEach(function (aid) {
          if (ed.stagedAdmins[aid] === false) { delete ed.stagedAdmins[aid]; delete ed.errors['admin:' + aid]; }
        });
      }
      return true;
    }
    if (kind === 'org') {
      var st = ed.stagedOrgs && ed.stagedOrgs[id];
      if (!st) return false;
      ed.lastUndoErr = null;
      if (st.op === 'create') {
        delete ed.stagedOrgs[id];
        delete ed.errors[key];
        if (ed.editingOrg === id) ed.editingOrg = null;
        return true;
      }
      var base = ed.baseOrgs && ed.baseOrgs[id];
      if (!base) return false;
      if (base.head) {
        var held = orgEdHeadOf(ed, base.head);
        if (held && held.id !== id) {
          ed.lastUndoErr = { key: key, reason: 'already_head', other: held, head: base.head };
          return false;
        }
      }
      var clash = orgEdNameClash(ed, base.name, id);
      if (clash) {
        ed.lastUndoErr = { key: key, reason: 'dup_name', other: clash };
        return false;
      }
      delete ed.stagedOrgs[id];
      delete ed.errors[key];
      return true;
    }
    return false;
  }
  function orgEdUndoAll(ed) {
    ed.stagedLeads = {};
    ed.stagedAdmins = {};
    ed.stagedOrgs = {};
    ed.editingOrg = null;
    ed.errors = {};
    ed.refuse = null;
    ed.lastUndoErr = null;
  }
  function orgEdChanges(ed) {
    var list = [];
    Object.keys(ed.stagedOrgs || {}).sort().forEach(function (id) {
      var st = ed.stagedOrgs[id], base = ed.baseOrgs && ed.baseOrgs[id];
      if (st.op === 'create') {
        list.push({ key: 'org:' + id, kind: 'org_create', org: id, name: st.name, head: st.head || '' });
      } else if (st.op === 'delete') {
        list.push({ key: 'org:' + id, kind: 'org_delete', org: id, name: base ? base.name : id, head: base ? (base.head || '') : '' });
      } else if (st.op === 'edit') {
        list.push({
          key: 'org:' + id, kind: 'org_edit', org: id,
          fromName: base ? base.name : '', toName: st.name,
          fromHead: base ? (base.head || '') : '', toHead: st.head || ''
        });
      }
    });
    Object.keys(ed.stagedLeads).sort().forEach(function (id) {
      list.push({ key: 'lead:' + id, kind: 'lead', member: id, from: ed.baseLeads[id] || '', to: ed.stagedLeads[id] || '' });
    });
    Object.keys(ed.stagedAdmins).sort().forEach(function (id) {
      list.push({ key: 'admin:' + id, kind: ed.stagedAdmins[id] ? 'add_admin' : 'remove_admin', member: id });
    });
    return list;
  }
  function orgEdCount(ed) {
    return Object.keys(ed.stagedLeads).length + Object.keys(ed.stagedAdmins).length + Object.keys(ed.stagedOrgs || {}).length;
  }
  function orgEdCloseNeedsConfirm(ed) {
    return !!ed && orgEdCount(ed) > 0;
  }
  // Save-time pre-flight check and deterministic single-batch event builder (Decisions 7, 8 & 12).
  // Order: add_admin -> org_remove -> org head releases -> topological org_set (with vacate on
  // head-swap cycles) -> set_lead (ascending final depth) -> remove_admin (viewer very last).
