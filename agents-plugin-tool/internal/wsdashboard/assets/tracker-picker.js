  // ------------------------------------------------------- CC & assign picker modal

  // --- picker modal pure logic
  // Indexes the output of orgTree(members) for the CC and assign picker modal:
  // display order (forest preorder, then unplaced, then unreachable), lead flags,
  // self-inclusive subtrees, strict descendant depth maps, and ancestor chains.
  function pmIndex(ot) {
    ot = ot || { roots: [], unplaced: [], removed: [], unreachable: [], parentOf: {}, depthOf: {}, countOf: {} };
    var order = [], listed = {}, isLead = {}, subtree = {}, desc = {}, ancestors = {}, count = {};
    var parentOf = ot.parentOf || {};
    function walk(node, chain) {
      if (!node || node.cycle || listed[node.id]) return [];
      var id = node.id;
      listed[id] = true;
      order.push(id);
      ancestors[id] = chain.slice();
      var sub = [id], dmap = {}, ch = node.children || [], realKids = 0;
      var nextChain = [id].concat(chain);
      ch.forEach(function (c) {
        if (!c || c.cycle || listed[c.id]) return;
        realKids++;
        var csub = walk(c, nextChain);
        csub.forEach(function (cid) { sub.push(cid); });
        dmap[c.id] = 1;
        var cd = desc[c.id] || {};
        Object.keys(cd).forEach(function (did) { dmap[did] = cd[did] + 1; });
      });
      if (realKids > 0) isLead[id] = true;
      subtree[id] = sub;
      desc[id] = dmap;
      count[id] = sub.length;
      return sub;
    }
    (ot.roots || []).forEach(function (r) { walk(r, []); });
    (ot.unplaced || []).concat(ot.unreachable || []).forEach(function (id) {
      if (listed[id]) return;
      listed[id] = true;
      order.push(id);
      subtree[id] = [id];
      desc[id] = {};
      count[id] = 1;
      var chain = [], seen = {}, cur = parentOf[id];
      seen[id] = true;
      while (cur && !seen[cur]) { seen[cur] = true; chain.push(cur); cur = parentOf[cur]; }
      ancestors[id] = chain;
    });
    return { order: order, listed: listed, isLead: isLead, subtree: subtree, desc: desc, ancestors: ancestors, count: count };
  }
  // Computes covered state from active group toggles ({ leadId: true }):
  // - personBy[id]: outermost active group covering id (lead's own "본인" included)
  // - groupBy[leadId]: outermost strict-ancestor active group covering leadId's group toggle
  // - topGroups: active leads whose group toggle is not covered by a higher active group
  function pmCover(idx, groups, headToOrg) {
    var G = groups || {}, h2o = headToOrg || {}, personBy = {}, groupBy = {}, topGroups = [];
    function isGrp(id) { return !!(idx.isLead[id] || h2o[id]); }
    idx.order.forEach(function (id) {
      var anc = idx.ancestors[id] || [], up = '';
      for (var i = anc.length - 1; i >= 0; i--) {
        if (G[anc[i]] && isGrp(anc[i])) { up = anc[i]; break; }
      }
      if (up) {
        personBy[id] = up;
        if (isGrp(id)) groupBy[id] = up;
      } else if (G[id] && isGrp(id)) {
        personBy[id] = id;
        topGroups.push(id);
      }
    });
    return { personBy: personBy, groupBy: groupBy, topGroups: topGroups };
  }
  // Returns live orgs with no live head, sorted by name then id, filtered by optional query.
  function pmVacantOrgs(oi, q, onlySelected) {
    if (!oi || !oi.live || onlySelected) return [];
    var qq = String(q || '').trim().toLowerCase();
    var h2o = oi.headToOrg || {};
    return oi.live.filter(function (o) {
      if (!o || o.removed) return false;
      if (o.head && h2o[o.head] && h2o[o.head].id === o.id) return false;
      if (qq && (String(o.name || '') + ' ' + String(o.id || '')).toLowerCase().indexOf(qq) < 0) return false;
      return true;
    }).sort(function (a, b) {
      var an = String(a.name || ''), bn = String(b.name || '');
      return an !== bn ? an.localeCompare(bn) : (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
    });
  }
  // Derives the item's globally excluded CC members (matching wstracker state_cc.go:49-70):
  // a member excluded by an applied item.uncc stays excluded until a winning item.cc puts
  // them back in it.cc. Also honors an explicit it.ccExcluded map/list if present.
  function pmCcExcluded(it, events) {
    var out = {}, orig = {};
    if (!it) return out;
    (Array.isArray(it.cc) ? it.cc : []).forEach(function (id) { orig[id] = true; });
    if (it.ccExcluded) {
      if (Array.isArray(it.ccExcluded)) {
        it.ccExcluded.forEach(function (id) { if (id && !orig[id]) out[id] = true; });
      } else {
        Object.keys(it.ccExcluded).forEach(function (id) { if (it.ccExcluded[id] && !orig[id]) out[id] = true; });
      }
    }
    var itemId = it.id;
    if (itemId && Array.isArray(events)) {
      events.forEach(function (e) {
        if (!e || e.ignored || e.type !== 'item.uncc') return;
        if (e.status && e.status !== 'applied') return;
        var p = e.payload;
        if (p && p.item === itemId && p.member && !orig[p.member]) out[p.member] = true;
      });
    }
    return out;
  }
  // Derives initial CC picker picks and checked org groups from server per-source CC state.
  // Live headed orgs registered in it.orgCC start checked in groups; ONLY members with a
  // personal CC source start in picks. Members CC'd solely via org sources (including
  // moved-out members or deleted/vacant orgs) are never auto-picked as personal.
  function pmCcInit(idx, it, headToOrg, events) {
    var origCc = (it && Array.isArray(it.cc)) ? it.cc.slice() : [];
    var origOrgCc = (it && Array.isArray(it.orgCC)) ? it.orgCC.slice() : [];
    var ccSources = (it && it.ccSources) || {};
    var ccExcluded = pmCcExcluded(it, events);
    var h2o = headToOrg || {};
    var origOrgSet = {}, origOrgByHead = {}, origPersonal = {}, groups = {}, picks = {};
    origOrgCc.forEach(function (reg) {
      var oid = typeof reg === 'string' ? reg : (reg && reg.org);
      if (oid) origOrgSet[oid] = true;
    });
    idx.order.forEach(function (id) {
      var o = h2o[id];
      if (o && o.id && origOrgSet[o.id]) {
        groups[id] = true;
        origOrgByHead[id] = o.id;
      }
    });
    var hasSources = Object.keys(ccSources).length > 0;
    origCc.forEach(function (id) {
      if (!idx.listed[id]) return;
      var src = hasSources ? ccSources[id] : null;
      var personal = src ? !!src.personal : !hasSources;
      if (personal) {
        picks[id] = true;
        origPersonal[id] = true;
      }
    });
    return {
      origCc: origCc, origOrgCc: origOrgCc, ccSources: ccSources, ccExcluded: ccExcluded,
      origOrgByHead: origOrgByHead, origPersonal: origPersonal, picks: picks, unpicks: {}, groups: groups
    };
  }
  // Evaluates CC picker state from per-source CC state (personal vs org sources vs global
  // exclusion, matching wstracker state_cc.go:49-70, 113-119).
  // - Org heads emit item.cc_org / item.uncc_org without touching members' personal provenance.
  // - Globally excluded members are never covered by any org (existing or newly selected) and
  //   stay personally re-addable via item.cc.
  // - Plain leads (no org) emit personal item.cc for every listed subtree member lacking a
  //   personal source (including members currently CC'd only via an org).
  function pmCcEval(idx, origCc, picks, groups, headToOrg, orgCtx) {
    var P = picks || {}, U = (orgCtx && orgCtx.unpicks) || {}, G = groups || {}, h2o = headToOrg || {}, orig = {};
    (origCc || []).forEach(function (id) { orig[id] = true; });
    var origOrgSet = {}, origToggleOrgByHead = {};
    ((orgCtx && orgCtx.origOrgCc) || []).forEach(function (entry) {
      var oid = typeof entry === 'string' ? entry : (entry && entry.org);
      if (oid) origOrgSet[oid] = true;
    });
    idx.order.forEach(function (id) {
      var o = h2o[id];
      if (o && o.id && origOrgSet[o.id]) origToggleOrgByHead[id] = o.id;
    });
    var excluded = {};
    var rawExcl = orgCtx && orgCtx.ccExcluded;
    if (rawExcl) {
      if (Array.isArray(rawExcl)) {
        rawExcl.forEach(function (id) { if (id && !orig[id]) excluded[id] = true; });
      } else {
        Object.keys(rawExcl).forEach(function (id) { if (rawExcl[id] && !orig[id]) excluded[id] = true; });
      }
    }
    // Any live member inside an already-registered live org's subtree who is absent from
    // origCc was excluded on the server (fallback when event history is omitted).
    Object.keys(origToggleOrgByHead).forEach(function (hid) {
      (idx.subtree[hid] || [hid]).forEach(function (mid) {
        if (!orig[mid]) excluded[mid] = true;
      });
    });
    var rawCov = pmCover(idx, G, h2o);
    var cov = { personBy: {}, groupBy: rawCov.groupBy, topGroups: rawCov.topGroups };
    idx.order.forEach(function (id) {
      var gid = rawCov.personBy[id];
      if (!gid) return;
      var isOrgGrp = !!(h2o[gid] && h2o[gid].id);
      if (isOrgGrp && excluded[id]) return;
      cov.personBy[id] = gid;
    });
    var addedOrgs = [], addedOrgSet = {}, removedOrgs = [], removedOrgSet = {};
    cov.topGroups.forEach(function (gid) {
      var o = h2o[gid];
      if (o && o.id && !origOrgSet[o.id] && !addedOrgSet[o.id]) {
        addedOrgSet[o.id] = true;
        addedOrgs.push({ head: gid, org: o.id, name: o.name, n: idx.count[gid] || 1 });
      }
    });
    ((orgCtx && orgCtx.extraRemovedOrgs) || []).forEach(function (r) {
      if (r && r.org && !removedOrgSet[r.org]) {
        removedOrgSet[r.org] = true;
        removedOrgs.push(r);
      }
    });
    idx.order.forEach(function (id) {
      var oid = origToggleOrgByHead[id];
      if (oid && !G[id] && !removedOrgSet[oid]) {
        removedOrgSet[oid] = true;
        removedOrgs.push({ head: id, org: oid, name: h2o[id].name, n: idx.count[id] || 1 });
      }
    });
    var srcs = (orgCtx && orgCtx.ccSources) || null;
    var hasSources = !!(srcs && Object.keys(srcs).length > 0);
    var effective = {}, retained = {}, added = [], removed = [], ccMembers = [], unccMembers = [];
    var grpChips = [], addChips = [], rmOrgChips = [], rmChips = [], effectiveCount = 0;
    cov.topGroups.forEach(function (gid) {
      var o = h2o[gid];
      if (o && o.id) grpChips.push({ t: 'g', id: gid, org: o.id, name: o.name, n: idx.count[gid] || 1, wasOrig: !!origOrgSet[o.id] });
      else grpChips.push({ t: 'g', id: gid, n: idx.count[gid] || 1 });
    });
    removedOrgs.forEach(function (r) {
      rmOrgChips.push({ t: 'rmg', id: r.head, org: r.org, name: r.name, n: r.n });
    });
    idx.order.forEach(function (id) {
      var on = !!P[id], unpicked = !!U[id], covBy = cov.personBy[id] || '', covered = !!covBy;
      var covOrg = covBy ? h2o[covBy] : null;
      var coveredByOrg = !!(covOrg && covOrg.id);
      var coveredByPlainLead = !!(covered && !coveredByOrg);
      var was = !!orig[id];
      var src = hasSources ? srcs[id] : null;
      var hadPersonal = was && (src ? !!src.personal : !hasSources);
      var hasSurvivingOrg = !!(was && src && Array.isArray(src.orgs) && src.orgs.some(function (oid) {
        return !removedOrgSet[oid];
      }));
      var retainedByOrg = hasSurvivingOrg && !unpicked;
      var eff = on || covered || retainedByOrg;
      var wantsPersonal = (on && !coveredByOrg) || coveredByPlainLead;
      var shouldUncc = !eff && (hadPersonal || (unpicked && hasSurvivingOrg));
      if (eff) { effective[id] = true; effectiveCount++; }
      if (retainedByOrg) retained[id] = true;
      if (eff && !was) added.push(id);
      if (!eff && was) removed.push(id);
      if (wantsPersonal && !hadPersonal) ccMembers.push(id);
      if (shouldUncc) unccMembers.push(id);
      if ((on || retainedByOrg) && !covered) addChips.push({ t: 'p', id: id, wasOrig: was });
      if (shouldUncc) rmChips.push({ t: 'rm', id: id });
    });
    var eventCount = removedOrgs.length + addedOrgs.length + ccMembers.length + unccMembers.length;
    return {
      cov: cov,
      effective: effective,
      retained: retained,
      excluded: excluded,
      added: added,
      removed: removed,
      addedOrgs: addedOrgs,
      removedOrgs: removedOrgs,
      ccMembers: ccMembers,
      unccMembers: unccMembers,
      chips: grpChips.concat(addChips, rmOrgChips, rmChips),
      dirty: eventCount > 0,
      eventCount: eventCount,
      effectiveCount: effectiveCount
    };
  }
  // Builds atomic item.uncc_org / item.cc_org / item.cc / item.uncc event batch for CC picker apply.
  function pmCcEvents(itemId, idx, origCc, picks, groups, headToOrg, orgCtx) {
    var ev = pmCcEval(idx, origCc, picks, groups, headToOrg, orgCtx), out = [];
    ev.removedOrgs.forEach(function (r) { out.push(['item.uncc_org', { item: itemId, org: r.org }]); });
    ev.addedOrgs.forEach(function (a) { out.push(['item.cc_org', { item: itemId, org: a.org }]); });
    ev.ccMembers.forEach(function (id) { out.push(['item.cc', { item: itemId, member: id }]); });
    ev.unccMembers.forEach(function (id) { out.push(['item.uncc', { item: itemId, member: id }]); });
    return out;
  }
  // Maps each lead (including removed leads still referenced in live members' leads[])
  // to its live direct followers, matching wstracker state.go:children().
  function pmRawChildren(members) {
    var M = members || {}, c = {};
    Object.keys(M).forEach(function (id) {
      var m = M[id];
      if (!m || m.removed) return;
      (m.leads || []).forEach(function (l) {
        if (!l) return;
        (c[l] = c[l] || []).push(id);
      });
    });
    return c;
  }
  // Whether `target` is strictly below `from` within maxDepth hops in the server's reach
  // graph (state.go:reach), following live members' leads[] even when `from` is removed.
  function pmReaches(rawChildren, from, target, maxDepth) {
    if (!rawChildren || !from || !target || from === target) return false;
    var lim = (maxDepth != null && maxDepth !== 0) ? maxDepth : Infinity;
    var seen = {}, frontier = [from];
    seen[from] = true;
    for (var depth = 1; depth <= lim && frontier.length; depth++) {
      var next = [];
      for (var i = 0; i < frontier.length; i++) {
        var kids = rawChildren[frontier[i]] || [];
        for (var j = 0; j < kids.length; j++) {
          var k = kids[j];
          if (k === target) return true;
          if (!seen[k]) { seen[k] = true; next.push(k); }
        }
      }
      frontier = next;
    }
    return false;
  }
  // Whether existing live rule y covers proposed whole-subtree rule {root, include_root: true} (no levels).
  // Matches wstracker/check.go:510-519 & state.go:589-629,780-797: overlap is judged from the
  // assign-time root (assignRoot || root) over live members' raw leads[] (even if assignRoot is removed).
  function pmRuleCovers(idx, y, root, members) {
    if (!y || (y.levels != null && y.levels !== 0)) return false;
    var yRoot = y.assignRoot || y.root;
    if (!yRoot) return false;
    if (yRoot === root) return !!(y.includeRoot || y.include_root);
    if (members) return pmReaches(pmRawChildren(members), yRoot, root, null);
    return !!(idx.desc[yRoot] && idx.desc[yRoot][root] > 0);
  }
  // Whether any live rule of viewer on itemId already covers {root, include_root: true}.
  function pmRuleCovered(idx, rules, itemId, viewer, root, members) {
    var list = Array.isArray(rules) ? rules : Object.keys(rules || {}).map(function (k) { return rules[k]; });
    var rawC = members ? pmRawChildren(members) : null;
    return list.some(function (y) {
      if (!y || y.ended || y.item !== itemId || y.by !== viewer || y.state !== 'live') return false;
      if (y.levels != null && y.levels !== 0) return false;
      var yRoot = y.assignRoot || y.root;
      if (!yRoot) return false;
      if (yRoot === root) return !!(y.includeRoot || y.include_root);
      return rawC ? pmReaches(rawC, yRoot, root, null) : !!(idx.desc[yRoot] && idx.desc[yRoot][root] > 0);
    });
  }
  // Whether a live rule currently covers memberId per wstracker state.go:coveredBy (uses live
  // org head for org rules, and follows live members' raw leads[] even when non-org root is removed).
  function pmLiveRuleCoversMember(ru, memberId, oi, members, rawC) {
    if (!ru || ru.ended || ru.state !== 'live' || !memberId) return false;
    var M = members || {}, root = ru.root || '';
    if (ru.org && oi) {
      var o = oi.byId && oi.byId[ru.org];
      if (!o || o.removed || !o.head || (members && (!M[o.head] || M[o.head].removed))) return false;
      root = o.head;
    }
    if (!root) return false;
    if (memberId === root) return !!(ru.includeRoot && (!members || (M[root] && !M[root].removed)));
    return pmReaches(rawC || pmRawChildren(M), root, memberId, ru.levels != null && ru.levels !== 0 ? ru.levels : null);
  }
  // Pure completion class for a single assign target, matching complClass/asgClass.
  function pmComplClass(o, viewer, reach) {
    var id = o.t === 'r' ? o.root : o.id;
    return (o.t === 'r' && id === viewer) || !!(reach && reach[id]) ? 'assigner' : 'assignee';
  }
  function pmAsgClass(o, compl, viewer, reach) {
    return pmComplClass(o, viewer, reach) === 'assigner' ? 'assigner' : (compl || 'assignee');
  }
  // Collects open org assign rules on it mapped by each live org's current head.
  // Decision 20: only the rule's author (ru.by === viewer, non-hidden item) may uncheck it.
  function pmAssignOrgInit(idx, it, rules, headToOrg, viewer) {
    var origOrgAssign = {}, groups = {};
    if (!it || !rules) return { origOrgAssign: origOrgAssign, groups: groups };
    var h2o = headToOrg || {};
    var rlist = Array.isArray(rules) ? rules : Object.keys(rules).map(function (k) { return rules[k]; });
    var byOrg = {};
    rlist.forEach(function (ru) {
      if (!ru || ru.ended || !ru.org || ru.item !== it.id) return;
      (byOrg[ru.org] = byOrg[ru.org] || []).push(ru);
    });
    idx.order.forEach(function (id) {
      var o = h2o[id];
      if (!o || !o.id || !byOrg[o.id] || !byOrg[o.id].length) return;
      var allR = byOrg[o.id].slice().sort(function (a, b) {
        var ta = a.ts || '', tb = b.ts || '';
        return ta !== tb ? (ta < tb ? -1 : 1) : (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
      });
      var removable = !it.hidden ? allR.filter(function (ru) { return ru.by === viewer; }) : [];
      var unremovable = allR.filter(function (ru) { return !!it.hidden || ru.by !== viewer; });
      var canRemove = removable.length > 0 && unremovable.length === 0;
      origOrgAssign[id] = {
        org: o.id, name: o.name, rules: allR, removable: removable, unremovable: unremovable,
        canRemove: canRemove, pending: allR.every(function (ru) { return ru.state === 'pending'; })
      };
      if (canRemove) groups[id] = true;
    });
    return { origOrgAssign: origOrgAssign, groups: groups };
  }
  // Computes current locked people and locked groups in assign/retarget mode, accounting for
  // rules being removed in the same batch (retarget rule, unchecked removable org rules, or
  // extraSkipRules). A viewer-authored removable org rule stays unlocked even when a broader
  // plain rule covers it, since check.go:1307-1323 lets the author item.unassign {rule}.
  function pmAssignLocks(idx, it, rules, assignments, orgs, members, viewer, headToOrg, origOrgAssign, groups, retarget, norule, extraSkipRules) {
    var h2o = headToOrg || {}, origO = origOrgAssign || {}, G = groups || {};
    var skipSet = {};
    if (retarget && retarget.rule) skipSet[retarget.rule] = true;
    (extraSkipRules || []).forEach(function (ru) {
      var rid = typeof ru === 'string' ? ru : (ru && ru.id);
      if (rid) skipSet[rid] = true;
    });
    var activeRemOwner = {};
    Object.keys(origO).forEach(function (hid) {
      var info = origO[hid];
      if (!info || !info.canRemove) return;
      if (!G[hid]) {
        info.removable.forEach(function (ru) { skipSet[ru.id] = true; });
      } else {
        info.removable.forEach(function (ru) { activeRemOwner[ru.id] = hid; });
      }
    });
    var lockedP = pmRetargetLockedPeople(it, rules, assignments, orgs, members, skipSet);
    var lockedOrder = [];
    idx.order.forEach(function (id) { if (lockedP[id]) lockedOrder.push(id); });
    var lockedG = {};
    if (!norule && it) {
      var rlist = Array.isArray(rules) ? rules : Object.keys(rules || {}).map(function (k) { return rules[k]; });
      var keptRules = rlist.filter(function (ru) { return ru && !skipSet[ru.id]; });
      idx.order.forEach(function (id) {
        if (!(idx.isLead[id] || h2o[id])) return;
        var info = origO[id];
        if (info && !info.canRemove) { lockedG[id] = true; return; }
        if (info && info.canRemove) return;
        var ancSet = {};
        ancSet[id] = true;
        (idx.ancestors[id] || []).forEach(function (a) { ancSet[a] = true; });
        var checkRules = keptRules.filter(function (ru) {
          var own = activeRemOwner[ru.id];
          return !(own && ancSet[own]);
        });
        if (pmRuleCovered(idx, checkRules, it.id, viewer, id, members)) lockedG[id] = true;
      });
    }
    return { lockedP: lockedP, lockedOrder: lockedOrder, lockedG: lockedG };
  }
  // Evaluates assign picker state.
  // - visCov uses all unlocked active groups (live + pending) for the tree's covered display.
  // - payCov uses ONLY live active groups (root === viewer or in reach, or live existing org rules),
  //   so picks/sub-groups under a pending group remain in payload and keep their own chips.
  function pmAssignEval(idx, opts) {
    opts = opts || {};
    var viewer = opts.viewer || '', reach = opts.reach || {};
    var lockedP = opts.lockedPeople || {}, lockedG = opts.lockedGroups || {};
    var P = opts.picks || {}, G = opts.groups || {};
    var norule = !!opts.norule, single = !!opts.single, compl = opts.compl || null;
    var h2o = opts.headToOrg || {}, origO = opts.origOrgAssign || {};
    function isLive(id) { return id === viewer || !!reach[id]; }
    var activeG = {}, liveG = {};
    if (!norule) {
      idx.order.forEach(function (id) {
        if ((idx.isLead[id] || h2o[id]) && G[id] && !lockedG[id]) {
          activeG[id] = true;
          var oinfo = origO[id];
          if (oinfo && oinfo.canRemove ? !oinfo.pending : isLive(id)) liveG[id] = true;
        }
      });
    }
    var visCov = pmCover(idx, activeG, h2o);
    var payCov = pmCover(idx, liveG, h2o);
    var targets = [], removedRules = [], remSeen = {}, origOrgChips = [], rmOrgChips = [];
    (opts.extraRemovedRules || []).forEach(function (ru) {
      if (ru && ru.id && !remSeen[ru.id]) {
        remSeen[ru.id] = true;
        removedRules.push(ru);
      }
    });
    idx.order.forEach(function (id) {
      var oinfo = !norule ? origO[id] : null;
      if (oinfo && oinfo.canRemove) {
        if (!G[id]) {
          oinfo.removable.forEach(function (ru) {
            if (ru && ru.id && !remSeen[ru.id]) {
              remSeen[ru.id] = true;
              removedRules.push(ru);
            }
          });
          rmOrgChips.push({ t: 'rmg', id: id, org: oinfo.org, name: oinfo.name, n: idx.count[id] || 1 });
        } else if (activeG[id] && !payCov.groupBy[id]) {
          origOrgChips.push({ t: 'og', id: id, org: oinfo.org, name: oinfo.name, n: idx.count[id] || 1, pend: !!oinfo.pending });
        }
      } else if (activeG[id] && !payCov.groupBy[id]) {
        var org = h2o[id];
        var tr = { t: 'r', root: id, include_root: true, levels: null, pend: !isLive(id), n: idx.count[id] || 1 };
        if (org && org.id) { tr.org = org.id; tr.orgName = org.name; }
        targets.push(tr);
      }
      if (P[id] && !lockedP[id] && !payCov.personBy[id]) {
        targets.push({ t: 'p', id: id, pend: !isLive(id) });
      }
    });
    if (single && targets.length > 1) targets = [targets[0]];
    var below = targets.some(function (o) { return pmComplClass(o, viewer, reach) === 'assigner'; });
    var cseen = {}, ckeys = [];
    targets.forEach(function (o) {
      var c = pmAsgClass(o, compl, viewer, reach);
      if (!cseen[c]) { cseen[c] = true; ckeys.push(c); }
    });
    var mode = ckeys.length > 1 ? 'mixed' : ckeys[0] || compl || null;
    var targetChips = targets.map(function (o) {
      if (o.t === 'r') {
        var ch = { t: 'g', id: o.root, n: o.n, pend: o.pend };
        if (o.org) { ch.org = o.org; ch.name = o.orgName; }
        return ch;
      }
      return { t: 'p', id: o.id, pend: o.pend };
    });
    return {
      visCov: visCov, payCov: payCov, targets: targets, removedRules: removedRules,
      chips: origOrgChips.concat(targetChips, rmOrgChips),
      below: below, mode: mode, dirty: targets.length > 0 || removedRules.length > 0
    };
  }
  // Builds atomic item.unassign (first, Decision 20) + item.assign event batch from pmAssignEval,
  // attaching org ids to org-headed rules and grouping targets by completion class.
  function pmAssignEvents(itemId, targets, compl, viewer, reach, headToOrg, removedRules) {
    var h2o = headToOrg || {}, out = [];
    (removedRules || []).forEach(function (ru) {
      if (!ru || !ru.id) return;
      var p = { item: itemId, rule: ru.id };
      var rt = ru.assignRoot || ru.root;
      if (rt) p.root = rt;
      out.push(['item.unassign', p]);
    });
    var groups = [], by = {};
    (targets || []).forEach(function (o) {
      var c = pmAsgClass(o, compl, viewer, reach);
      if (!by[c]) groups.push(by[c] = { c: c, sel: [] });
      by[c].sel.push(o);
    });
    groups.forEach(function (g) {
      var people = g.sel.filter(function (o) { return o.t === 'p'; }).map(function (o) { return o.id; });
      var rules = g.sel.filter(function (o) { return o.t === 'r'; }).map(function (o) {
        var r = { root: o.root };
        if (o.include_root) r.include_root = true;
        var orgId = o.org || (h2o[o.root] && h2o[o.root].id);
        if (orgId) r.org = orgId;
        else if (o.levels != null) r.levels = o.levels;
        return r;
      });
      out.push(['item.assign', { item: itemId, targets: people.concat(rules), completion: g.c }]);
    });
    return out;
  }
  // Builds atomic retarget batch for replacing an org assign rule (Decision 11 & 8):
  // emits item.unassign first, then item.assign with org-linked rules for org heads,
  // plain lead rules for non-org leads, and personal member targets unchanged.
  function pmRetargetAssignEvents(itemId, retarget, targets, compl, viewer, reach, headToOrg) {
    var rem = retarget && retarget.rule ? [{ id: retarget.rule, assignRoot: retarget.root, root: retarget.root }] : [];
    return pmAssignEvents(itemId, targets, compl, viewer, reach, headToOrg, rem);
  }
  // Builds atomic retarget batch for replacing an org CC registration (Decision 11 & 8):
  // emits item.uncc_org first, then item.cc_org for org heads, expanded item.cc snapshots for
  // non-org leads, and item.cc for individual picks. Never emits personal item.uncc.
  function pmRetargetCcEvents(itemId, retarget, idx, picks, groups, headToOrg, ccExcluded) {
    var h2o = headToOrg || {}, P = picks || {}, G = groups || {}, excl = ccExcluded || {};
    var out = [['item.uncc_org', { item: itemId, org: retarget.org }]];
    var rawCov = pmCover(idx, G, h2o), sentMember = {}, sentOrg = {};
    var covPersonBy = {};
    idx.order.forEach(function (id) {
      var gid = rawCov.personBy[id];
      if (!gid) return;
      var isOrgGrp = !!(h2o[gid] && h2o[gid].id);
      if (isOrgGrp && excl[id]) return;
      covPersonBy[id] = gid;
    });
    rawCov.topGroups.forEach(function (gid) {
      var org = h2o[gid];
      if (org && org.id) {
        if (!sentOrg[org.id]) {
          sentOrg[org.id] = true;
          out.push(['item.cc_org', { item: itemId, org: org.id }]);
        }
      } else {
        (idx.subtree[gid] || [gid]).forEach(function (mid) {
          if (!sentMember[mid]) {
            sentMember[mid] = true;
            out.push(['item.cc', { item: itemId, member: mid }]);
          }
        });
      }
    });
    idx.order.forEach(function (id) {
      if (P[id] && !covPersonBy[id] && !sentMember[id]) {
        sentMember[id] = true;
        out.push(['item.cc', { item: itemId, member: id }]);
      }
    });
    return out;
  }
  // Filters orgTree visibility for search query q and "selected only" mode.
  // Returns { show: {id: true}, matched: {id: true}, expand: {id: true}, hasAny: bool }.
  function pmFilter(ot, q, onlySelected, isRowSelected, matchFn) {
    var qq = String(q || '').trim().toLowerCase(), active = !!qq || !!onlySelected;
    var show = {}, matched = {}, expand = {}, hasAny = false;
    function selfHit(id) {
      if (onlySelected && !isRowSelected(id)) return false;
      if (qq && !matchFn(id, qq)) return false;
      return true;
    }
    function walk(node) {
      if (!node || node.cycle) return false;
      var hit = selfHit(node.id), childHit = false;
      (node.children || []).forEach(function (c) {
        if (walk(c)) childHit = true;
      });
      if (!active || hit || childHit) {
        show[node.id] = true;
        if (hit) { matched[node.id] = true; hasAny = true; }
        if (active && childHit) expand[node.id] = true;
        return true;
      }
      return false;
    }
    (ot && ot.roots || []).forEach(walk);
    ((ot && ot.unplaced) || []).concat((ot && ot.unreachable) || []).forEach(function (id) {
      var hit = selfHit(id);
      if (!active || hit) {
        show[id] = true;
        if (hit) { matched[id] = true; hasAny = true; }
      }
    });
    return { show: show, matched: matched, expand: expand, hasAny: active ? hasAny : Object.keys(show).length > 0 };
  }
  // Computes locked people in assign/retarget picker. When skipping rules (skipRule string or
  // { ruleId: true } map), members holding records under a skipped rule stay unlocked unless
  // another surviving live rule on the item covers them (since item.unassign triggers
  // refreshItemRules before item.assign).
  function pmRetargetLockedPeople(it, rules, assignments, orgs, members, skipRule) {
    var lockedP = {};
    if (!it) return lockedP;
    var skipSet = {};
    if (typeof skipRule === 'string' && skipRule) skipSet[skipRule] = true;
    else if (skipRule && typeof skipRule === 'object') {
      Object.keys(skipRule).forEach(function (k) { if (skipRule[k]) skipSet[k] = true; });
    }
    var hasSkip = Object.keys(skipSet).length > 0;
    var amap = assignments || {}, M = members || {}, oi = orgIndex(orgs, M), rawC = hasSkip ? pmRawChildren(M) : null;
    var rlist = Array.isArray(rules) ? rules : Object.keys(rules || {}).map(function (k) { return rules[k]; });
    var surviving = hasSkip ? rlist.filter(function (ru) {
      return ru && !ru.ended && ru.state === 'live' && ru.item === it.id && !skipSet[ru.id];
    }) : [];
    var seen = {};
    (it.assignments || []).concat(it.pending || []).forEach(function (aid) {
      var r = typeof aid === 'string' ? amap[aid] : aid;
      if (!r || !r.id || seen[r.id]) return;
      seen[r.id] = true;
      if (r.state !== 'live' && r.state !== 'pending') return;
      if (hasSkip && r.rule && skipSet[r.rule]) {
        if (surviving.some(function (ru) { return pmLiveRuleCoversMember(ru, r.to, oi, M, rawC); })) lockedP[r.to] = true;
        return;
      }
      lockedP[r.to] = true;
    });
    return lockedP;
  }
  // Builds the atomic event batch at apply time by re-deriving org/head mappings, live rules,
  // and per-source CC state from live view state `vs` so concurrent rehead/vacate/delete
  // cannot produce a refused batch (wstracker check.go:476-485, 510-521, 1237-1242).
  function pmBuildApplyEvents(cur, vs) {
    if (!cur || !vs || cur.draft || !cur.item) return [];
    var liveIt = vs.items && vs.items[cur.item];
    if (!liveIt) return [];
    var liveMembers = vs.members || {};
    var liveOrgs = vs.orgs || [];
    var liveOi = orgIndex(liveOrgs, liveMembers);
    var liveIdx = pmIndex(liveOi.tree);
    var liveH2o = liveOi.headToOrg;
    var openH2o = cur.headToOrg || {};
    var openGroups = cur.groups || {};
    var openPicks = cur.picks || {};
    var openUnpicks = cur.unpicks || {};

    function mapNewGroups(skipOpenHead) {
      var g = {};
      Object.keys(openGroups).forEach(function (hid) {
        if (!openGroups[hid]) return;
        if (skipOpenHead && skipOpenHead[hid]) return;
        var openOrg = openH2o[hid];
        if (openOrg && openOrg.id) {
          var liveOrg = liveOi.byId[openOrg.id];
          if (liveOrg && !liveOrg.removed && liveOrg.head && liveH2o[liveOrg.head] && liveH2o[liveOrg.head].id === openOrg.id) {
            g[liveOrg.head] = true;
          }
          return;
        }
        if (liveIdx.listed[hid] && (liveIdx.isLead[hid] || liveH2o[hid])) {
          g[hid] = true;
        }
      });
      return g;
    }

    var livePicks = {};
    Object.keys(openPicks).forEach(function (id) {
      if (openPicks[id] && liveIdx.listed[id]) livePicks[id] = true;
    });

    if (cur.kind === 'cc') {
      var liveExcluded = pmCcExcluded(liveIt, vs.events);
      if (!Array.isArray(vs.events) && cur.ccExcluded) {
        var liveCcCheck = {};
        (Array.isArray(liveIt.cc) ? liveIt.cc : []).forEach(function (id) { liveCcCheck[id] = true; });
        Object.keys(cur.ccExcluded).forEach(function (id) {
          if (cur.ccExcluded[id] && !liveCcCheck[id]) liveExcluded[id] = true;
        });
      }
      if (cur.retarget) {
        var rtGroups = mapNewGroups(null);
        if (!Object.keys(rtGroups).length && !Object.keys(livePicks).length) return [];
        return pmRetargetCcEvents(cur.item, cur.retarget, liveIdx, livePicks, rtGroups, liveH2o, liveExcluded);
      }
      var liveOrigCc = Array.isArray(liveIt.cc) ? liveIt.cc.slice() : [];
      var liveOrigOrgCc = Array.isArray(liveIt.orgCC) ? liveIt.orgCC.slice() : [];
      var liveCcSources = liveIt.ccSources || {};
      var liveOrigOrgSet = {};
      liveOrigOrgCc.forEach(function (reg) {
        var oid = typeof reg === 'string' ? reg : (reg && reg.org);
        if (oid) liveOrigOrgSet[oid] = true;
      });
      var openOrigOrgByHead = cur.origOrgByHead || {};
      var uncheckedOrigOrgs = {};
      Object.keys(openOrigOrgByHead).forEach(function (oldHead) {
        var oid = openOrigOrgByHead[oldHead];
        if (oid && !openGroups[oldHead]) uncheckedOrigOrgs[oid] = oldHead;
      });
      var liveGroups = mapNewGroups(openOrigOrgByHead);
      var extraRemovedOrgs = [];
      Object.keys(uncheckedOrigOrgs).forEach(function (oid) {
        if (!liveOrigOrgSet[oid]) return;
        var liveOrg = liveOi.byId[oid];
        var head = (liveOrg && liveOrg.head && liveH2o[liveOrg.head] && liveH2o[liveOrg.head].id === oid) ? liveOrg.head : uncheckedOrigOrgs[oid];
        extraRemovedOrgs.push({ head: head, org: oid, name: (liveOrg && liveOrg.name) || oid, n: liveIdx.count[head] || 1 });
      });
      liveIdx.order.forEach(function (id) {
        var o = liveH2o[id];
        if (o && o.id && liveOrigOrgSet[o.id] && !uncheckedOrigOrgs[o.id]) {
          liveGroups[id] = true;
        }
      });
      var openOrigPersonal = {};
      if (cur.origPersonal) {
        Object.keys(cur.origPersonal).forEach(function (id) { if (cur.origPersonal[id]) openOrigPersonal[id] = true; });
      } else {
        var openSrcs = cur.ccSources || {}, hasOpenSources = Object.keys(openSrcs).length > 0;
        (cur.origCc || []).forEach(function (id) {
          var s = hasOpenSources ? openSrcs[id] : null;
          if (s ? !!s.personal : !hasOpenSources) openOrigPersonal[id] = true;
        });
      }
      var hasLiveSources = Object.keys(liveCcSources).length > 0;
      liveOrigCc.forEach(function (id) {
        if (!liveIdx.listed[id] || openOrigPersonal[id] || openUnpicks[id]) return;
        var src = hasLiveSources ? liveCcSources[id] : null;
        var personal = src ? !!src.personal : !hasLiveSources;
        if (personal) livePicks[id] = true;
      });
      return pmCcEvents(cur.item, liveIdx, liveOrigCc, livePicks, liveGroups, liveH2o, {
        origOrgCc: liveOrigOrgCc,
        ccSources: liveCcSources,
        ccExcluded: liveExcluded,
        unpicks: openUnpicks,
        extraRemovedOrgs: extraRemovedOrgs
      });
    }

    var liveViewer = (vs.viewer && vs.viewer.member) || '';
    var liveReach = (vs.viewer && vs.viewer.reach) || {};
    var liveRules = vs.rules || {};
    var liveRlist = Array.isArray(liveRules) ? liveRules : Object.keys(liveRules).map(function (k) { return liveRules[k]; });
    var liveRuleById = {};
    liveRlist.forEach(function (ru) { if (ru && ru.id) liveRuleById[ru.id] = ru; });
    var norule = !!cur.norule;
    var single = !!cur.single;

    var liveOrigOrgAssign = (!cur.retarget && !norule)
      ? pmAssignOrgInit(liveIdx, liveIt, liveRules, liveH2o, liveViewer).origOrgAssign
      : {};
    var openOrigOrgAssign = cur.origOrgAssign || {};
    var skipOpenHead = {}, extraRemovedRules = [], uncheckedOrgIds = {};
    Object.keys(openOrigOrgAssign).forEach(function (oldHead) {
      var info = openOrigOrgAssign[oldHead];
      if (!info || !info.canRemove) return;
      skipOpenHead[oldHead] = true;
      if (!openGroups[oldHead]) {
        uncheckedOrgIds[info.org] = true;
        (info.removable || []).forEach(function (ru) {
          var liveRu = ru && ru.id && liveRuleById[ru.id];
          if (liveRu && !liveRu.ended && liveRu.item === cur.item && liveRu.by === liveViewer && !liveIt.hidden) {
            extraRemovedRules.push(liveRu);
          }
        });
      }
    });
    var liveAsgGroups = mapNewGroups(skipOpenHead);
    Object.keys(liveOrigOrgAssign).forEach(function (hid) {
      var info = liveOrigOrgAssign[hid];
      if (info && info.canRemove && !uncheckedOrgIds[info.org]) {
        liveAsgGroups[hid] = true;
      }
    });
    var liveLocks = pmAssignLocks(liveIdx, liveIt, liveRules, vs.assignments, liveOrgs, liveMembers, liveViewer, liveH2o, liveOrigOrgAssign, liveAsgGroups, cur.retarget, norule, extraRemovedRules);
    var liveEv = pmAssignEval(liveIdx, {
      viewer: liveViewer, reach: liveReach,
      lockedPeople: liveLocks.lockedP, lockedGroups: liveLocks.lockedG,
      picks: livePicks, groups: liveAsgGroups,
      norule: norule, single: single, compl: cur.compl,
      headToOrg: liveH2o, origOrgAssign: liveOrigOrgAssign,
      extraRemovedRules: extraRemovedRules
    });
    if (cur.retarget) {
      if (!liveEv.targets.length) return [];
      return pmRetargetAssignEvents(cur.item, cur.retarget, liveEv.targets, cur.compl, liveViewer, liveReach, liveH2o);
    }
    if (!liveEv.dirty) return [];
    return pmAssignEvents(cur.item, liveEv.targets, cur.compl, liveViewer, liveReach, liveH2o, liveEv.removedRules);
  }
  // --- end picker modal pure logic

  // --- picker modal state & renderer
  var PM = null;

  function pmMatch(id, qq) {
    var m = VS && VS.members[id], hd = PM && PM.headToOrg && PM.headToOrg[id];
    var hay = (nm(id) + ' ' + id + ' ' + (m && m.profile && m.profile.org ? m.profile.org : '') + (hd ? ' ' + hd.name : '')).toLowerCase();
    return hay.indexOf(qq) >= 0;
  }

  function openPicker(kind, ctx, opener) {
    if (!VS) return;
    ctx = ctx || {};
    var it = ctx.item ? VS.items[ctx.item] : null;
    var draftObj = ctx.draft ? (ctx.ed ? ui.ned : ui.nrow[ui.store]) : null;
    if (!ctx.draft && !it) return;
    if (ctx.draft && !draftObj) return;
    var retarget = ctx.retarget || null;
    var oi = orgIndex(VS.orgs, VS.members), ot = oi.tree, idx = pmIndex(ot);
    var picks = {}, unpicks = {}, groups = {}, origCc = [], origOrgCc = [], ccSources = {}, ccExcluded = {}, origOrgByHead = {}, origPersonal = {};
    var origOrgAssign = {}, lockedP = {}, lockedOrder = [], lockedG = {};
    var norule = false, single = false, compl = null;
    if (kind === 'cc') {
      if (!retarget) {
        var ccInit = pmCcInit(idx, it, oi.headToOrg, VS.events);
        origCc = ccInit.origCc;
        origOrgCc = ccInit.origOrgCc;
        ccSources = ccInit.ccSources;
        ccExcluded = ccInit.ccExcluded;
        origOrgByHead = ccInit.origOrgByHead;
        origPersonal = ccInit.origPersonal;
        picks = ccInit.picks;
        unpicks = ccInit.unpicks;
        groups = ccInit.groups;
      } else {
        ccExcluded = pmCcExcluded(it, VS.events);
      }
    } else if (kind === 'assign') {
      if (ctx.draft) {
        norule = true; single = draftObj.kind === 'topic'; compl = draftObj.compl || null;
        listOf(draftObj.who).forEach(function (id) {
          if (idx.listed[id] && (!single || !Object.keys(picks).length)) picks[id] = true;
        });
      } else {
        norule = it.kind === 'topic'; single = it.kind === 'topic';
        if (!retarget && !norule) {
          var asgInit = pmAssignOrgInit(idx, it, VS.rules, oi.headToOrg, VS.viewer.member);
          origOrgAssign = asgInit.origOrgAssign;
          groups = asgInit.groups;
        }
        var locks = pmAssignLocks(idx, it, VS.rules, VS.assignments, VS.orgs, VS.members, VS.viewer.member, oi.headToOrg, origOrgAssign, groups, retarget, norule);
        lockedP = locks.lockedP;
        lockedOrder = locks.lockedOrder;
        lockedG = locks.lockedG;
      }
    }
    PM = {
      kind: kind, item: ctx.item || null, draft: !!ctx.draft, ed: !!ctx.ed, store: ui.store,
      retarget: retarget, oi: oi, headToOrg: oi.headToOrg,
      ot: ot, idx: idx, origCc: origCc, origOrgCc: origOrgCc, ccSources: ccSources, ccExcluded: ccExcluded,
      origOrgByHead: origOrgByHead, origPersonal: origPersonal, origOrgAssign: origOrgAssign,
      picks: picks, unpicks: unpicks, groups: groups,
      lockedP: lockedP, lockedOrder: lockedOrder, lockedG: lockedG,
      norule: norule, single: single, compl: compl,
      shut: {}, q: '', only: false, focusId: null, err: '', busy: false,
      backEl: opener || null, back: opener ? returnSel(opener) : null
    };
    hideTip();
    var dl = $('tkpm');
    dl.className = 'tk tk-pm' + (kind === 'assign' ? ' pm-asg' : ' pm-cc');
    dl.innerHTML = pmShellHtml();
    pmDraw();
    if (!dl.open) dl.showModal();
    var q = $('tkpmq');
    if (q) q.focus();
  }

  function pmClose(refocus) {
    var m = PM;
    PM = null;
    var dl = $('tkpm');
    if (dl) {
      if (dl.open) dl.close();
      dl.innerHTML = '';
    }
    if (!refocus || !m) return;
    var el = m.backEl && m.backEl.isConnected ? m.backEl : m.back && document.querySelector(m.back);
    if (el && !el.matches('a[href], button, input, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]');
    if (el) el.focus({ preventScroll: true });
  }

  function pmShellHtml() {
    var it = PM.item && VS.items[PM.item];
    var draftObj = PM.draft ? (PM.ed ? ui.ned : ui.nrow[PM.store]) : null;
    var rt = PM.retarget;
    var title = rt ? wsT(PM.kind === 'cc' ? "js.tk.pm.retarget.cc.title" : "js.tk.pm.retarget.asg.title", orgNameOf(rt.org))
      : (PM.kind === 'cc' ? wsT("js.tk.pm.cc.title") : wsT("js.tk.pm.assign.title"));
    var sub = it ? '<code>' + esc(it.alias) + '</code> ' + esc(it.title) :
      draftObj ? esc((draftObj.val || '').trim() || wsT("js.tk.new.item")) : '';
    var hasTools = PM.kind === 'cc' && !rt;
    var hasNote = !!rt || (PM.kind === 'assign' && !PM.norule);
    var noteTxt = rt ? wsT(PM.kind === 'cc' ? "js.tk.pm.retarget.cc.note" : "js.tk.pm.retarget.asg.note", esc(orgNameOf(rt.org)))
      : wsT("js.tk.pm.asg.note");
    return '<div class="pm-dlg" role="document" aria-label="' + esc(title) + '">' +
      '<div class="pm-dh"><h2>' + esc(title) + '</h2>' + (sub ? '<span class="pm-sub">' + sub + '</span>' : '') +
      '<span class="grow"></span><button type="button" class="pm-x" data-pm="close" aria-label="' + esc(wsT("js.tk.dlg.close")) + '">&times;</button></div>' +
      '<div class="pm-tb"><input type="search" id="tkpmq" autocomplete="off" spellcheck="false" placeholder="' + esc(wsT("js.tk.pm.search")) + '" aria-label="' + esc(wsT("js.tk.pm.search")) + '">' +
      (hasTools ? '<button type="button" class="pm-btn mini" data-pm="expand">' + wsT("js.tk.pm.expand") + '</button>' +
        '<button type="button" class="pm-btn mini" id="tkpm-only" data-pm="only" aria-pressed="false" title="' + esc(wsT("js.tk.pm.only.tip")) + '">' + wsT("js.tk.pm.only") + '</button>' : '') + '</div>' +
      '<div class="pm-sel"><span class="pm-lab">' + wsT("js.tk.pm.selected") + '</span><div class="pm-chips" id="tkpm-chips"></div></div>' +
      '<div class="pm-body" id="tkpm-body"></div>' +
      (hasNote ? '<div class="pm-note">' + noteTxt + '</div>' : '') +
      '<div class="pm-err" id="tkpm-err" role="alert" hidden></div>' +
      '<div class="pm-df" id="tkpm-foot"></div>' +
      '</div>';
  }

  function pmCcEvalCur() {
    var orgCtx = PM.retarget
      ? { ccExcluded: PM.ccExcluded }
      : { origOrgCc: PM.origOrgCc, ccSources: PM.ccSources, ccExcluded: PM.ccExcluded, unpicks: PM.unpicks };
    return pmCcEval(PM.idx, PM.origCc, PM.picks, PM.groups, PM.headToOrg, orgCtx);
  }

  function pmAssignEvalCur() {
    if (!PM.draft && PM.item && VS && VS.items[PM.item]) {
      var locks = pmAssignLocks(PM.idx, VS.items[PM.item], VS.rules, VS.assignments, VS.orgs, VS.members, VS.viewer.member, PM.headToOrg, PM.origOrgAssign, PM.groups, PM.retarget, PM.norule);
      PM.lockedP = locks.lockedP;
      PM.lockedOrder = locks.lockedOrder;
      PM.lockedG = locks.lockedG;
    }
    return pmAssignEval(PM.idx, {
      viewer: VS.viewer.member, reach: VS.viewer.reach || {},
      lockedPeople: PM.lockedP, lockedGroups: PM.lockedG,
      picks: PM.picks, groups: PM.groups,
      norule: PM.norule, single: PM.single, compl: PM.compl,
      headToOrg: PM.headToOrg, origOrgAssign: PM.origOrgAssign
    });
  }

  function pmGrpLabel(id, org) {
    var n = PM.idx.count[id] || 1;
    return org ? wsTn("js.tk.pm.grp.org", n, esc(org.name)) : wsTn("js.tk.pm.grp", n);
  }

  function pmCovTip(covBy) {
    var o = PM && PM.headToOrg && PM.headToOrg[covBy];
    return o ? wsT("js.tk.pm.cov.org.tip", o.name) : wsT("js.tk.pm.cov.tip", nm(covBy));
  }

  function pmAsgCovText(covBy, escaped) {
    var o = PM && PM.headToOrg && PM.headToOrg[covBy];
    var label = o ? o.name : nm(covBy);
    if (escaped) label = esc(label);
    return wsT(o ? "js.tk.pm.asg.cov.org" : "js.tk.pm.asg.cov", label);
  }

  function pmAsgPendTip(id) {
    var o = PM && PM.headToOrg && PM.headToOrg[id];
    return o ? wsT("js.tk.pm.asg.grp.org.pend.tip", o.name, nm(id)) : wsT("js.tk.pm.asg.grp.pend.tip", nm(id));
  }

  function pmAsgChipsHtml(ev) {
    var out = [];
    if (!PM.draft) {
      PM.idx.order.forEach(function (id) {
        var oinfo = PM.origOrgAssign && PM.origOrgAssign[id];
        if (oinfo && !oinfo.canRemove) {
          var by = oinfo.unremovable && oinfo.unremovable[0] ? nm(oinfo.unremovable[0].by) : '';
          out.push('<span class="pm-chip was" title="' + esc(wsT("js.tk.pm.asg.org.locked.tip", by)) + '">&#x1F512; ' + wsT("js.tk.pm.chip.asg.locked", esc(oinfo.name)) + '</span>');
        }
      });
      PM.lockedOrder.forEach(function (id) {
        var covHead = ev.visCov && ev.visCov.personBy[id];
        if (covHead && PM.origOrgAssign && PM.origOrgAssign[covHead]) return;
        out.push('<span class="pm-chip was" title="' + esc(wsT("js.tk.pm.asg.locked.tip")) + '">&#x1F512; ' + wsT("js.tk.pm.chip.asg.locked", esc(nm(id))) + '</span>');
      });
    }
    ev.chips.forEach(function (c) {
      if (c.t === 'og' || c.t === 'g') {
        var txt = c.org
          ? '&#x1F3E2; ' + (c.pend ? wsTn("js.tk.pm.chip.asg.org.pend", c.n, esc(c.name)) : wsTn("js.tk.pm.chip.asg.org", c.n, esc(c.name)))
          : (c.pend ? wsTn("js.tk.pm.chip.asg.grp.pend", c.n, esc(nm(c.id))) : wsTn("js.tk.pm.chip.asg.grp", c.n, esc(nm(c.id))));
        out.push('<button type="button" class="pm-chip ' + (c.pend ? 'pend' : 'org') + '" data-pm="chip" data-k="g:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.clear")) + '">' + txt + '</button>');
      } else if (c.t === 'rmg') {
        out.push('<button type="button" class="pm-chip rm" data-pm="chip" data-k="rmg:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.undo")) + '"><s>&minus; ' + esc(c.name) + '</s></button>');
      } else {
        var ptxt = c.pend ? wsT("js.tk.pm.chip.asg.pend", esc(nm(c.id))) : esc(nm(c.id));
        out.push('<button type="button" class="pm-chip ' + (c.pend ? 'pend' : 'add') + '" data-pm="chip" data-k="p:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.clear")) + '">' + ptxt + '</button>');
      }
    });
    return out.length ? out.join('') : '<span class="pm-chip was">' + wsT("js.tk.pm.none") + '</span>';
  }

  function pmAsgRowHtml(id, isLead, ev) {
    var v = VS.viewer.member, reach = VS.viewer.reach || {};
    var org = PM.headToOrg && PM.headToOrg[id];
    var showGrp = (isLead || !!org) && !PM.norule, locked = !!PM.lockedP[id];
    var covBy = !locked ? (ev.visCov.personBy[id] || '') : '', covered = !!covBy;
    var on = !locked && !covered && !!PM.picks[id], isLive = id === v || !!reach[id];
    var cls = 'pm-nd' + (covered ? ' cvr' : '') + (!showGrp && (on || locked) ? ' on' : '') + (!showGrp && covered ? ' cov' : '') + (!showGrp && locked ? ' lock' : '') + (PM.focusId === id ? ' focus' : '');
    var rowTip = locked ? ' title="' + esc(wsT("js.tk.pm.asg.locked.tip")) + '"' : covered ? ' title="' + esc(pmAsgCovText(covBy, false)) + '"' : '';
    var col1 = isLead ? '<button type="button" class="pm-tg" data-pm="tog" data-id="' + esc(id) + '" aria-label="' + esc(wsT("js.tk.pm.toggle")) + '">&#9662;</button>' : '<span></span>';
    var col2 = showGrp ? '<span></span>' : PM.single
      ? '<span class="pm-rdo' + (locked ? ' lock' : '') + '" data-pm="self" data-id="' + esc(id) + '" role="radio" aria-checked="' + (on || locked ? 'true' : 'false') + '"' + (locked ? ' aria-disabled="true"' : '') + rowTip + '></span>'
      : '<span class="pm-ck' + (locked ? ' lock' : '') + '" data-pm="self" data-id="' + esc(id) + '" role="checkbox" aria-checked="' + (on || covered || locked ? 'true' : 'false') + '"' + (locked || covered ? ' aria-disabled="true"' : '') + rowTip + '></span>';
    var subTxt = (!showGrp && covered) ? pmAsgCovText(covBy, true) : pmRowSub(id);
    var orgTag = org ? '<span class="tk-org-badge">' + esc(org.name) + '</span>' : '';
    var who = '<span class="pm-who"' + (showGrp ? '' : ' data-pm="self" data-id="' + esc(id) + '"') + rowTip + '><span class="pm-nm">' + esc(nm(id)) + '</span>' +
      orgTag + (id === v ? '<span class="pm-chip you">' + wsT("js.tk.pm.you") + '</span>' : '') + '<span class="pm-o">' + subTxt + '</span></span>';
    var rt = (locked ? '<span class="pm-chip">' + wsT("js.tk.pm.asg.locked") + '</span>' : '') +
      (on && !isLive ? '<span class="pm-chip pend" title="' + esc(wsT("js.tk.pm.asg.pend.tip")) + '">' + wsT("js.tk.pm.asg.pend") + '</span>' : '');
    if (showGrp) {
      var origInfo = PM.origOrgAssign && PM.origOrgAssign[id];
      var gLocked = !!PM.lockedG[id], gCov = !gLocked ? (ev.visCov.groupBy[id] || '') : '', gOn = !gLocked && !gCov && !!PM.groups[id];
      var gLockTip = (origInfo && !origInfo.canRemove && origInfo.unremovable && origInfo.unremovable[0])
        ? wsT("js.tk.pm.asg.org.locked.tip", nm(origInfo.unremovable[0].by))
        : wsT("js.tk.pm.asg.rule.locked.tip");
      var gTip = gLocked ? ' title="' + esc(gLockTip) + '"' : gCov ? ' title="' + esc(pmAsgCovText(gCov, false)) + '"' : (!isLive && !(origInfo && origInfo.canRemove && !origInfo.pending)) ? ' title="' + esc(pmAsgPendTip(id)) + '"' : '';
      rt += '<button type="button" class="pm-opt self' + (on || locked ? ' on' : '') + (locked ? ' lock' : '') + (covered ? ' cov' : '') + '" data-pm="self" data-id="' + esc(id) + '" aria-pressed="' + (on || covered || locked ? 'true' : 'false') + '"' + (locked || covered ? ' aria-disabled="true"' : '') + rowTip + '><span class="pm-ck"></span>' + wsT("js.tk.pm.self") + '</button>';
      rt += '<button type="button" class="pm-opt grp' + (gOn || gLocked ? ' on' : '') + (gLocked ? ' lock' : '') + (gCov ? ' cov' : '') + '" data-pm="grp" data-id="' + esc(id) + '" aria-pressed="' + (gOn || gLocked || !!gCov ? 'true' : 'false') + '"' + (gLocked || gCov ? ' aria-disabled="true"' : '') + gTip + '><span class="pm-ck"></span><span class="k">' + pmGrpLabel(id, org) + '</span></button>';
      if (gOn) {
        var gIsPend = origInfo && origInfo.canRemove ? origInfo.pending : !isLive;
        rt += gIsPend ? '<span class="pm-chip pend" title="' + esc(pmAsgPendTip(id)) + '">' + wsT("js.tk.pm.asg.pend") + '</span>' : '<span class="pm-chip rule">' + wsT("js.tk.pm.asg.rule") + '</span>';
      } else if (!gLocked && !gCov && origInfo && origInfo.canRemove) {
        rt += '<span class="pm-chip rm">' + wsT("js.tk.pm.cc.rm") + '</span>';
      }
    }
    return '<div class="' + cls + '" data-id="' + esc(id) + '"' + (showGrp ? ' data-lead="1"' : '') + '>' + col1 + col2 + avHtml(id, false, false) + who + '<span class="pm-rt">' + rt + '</span></div>';
  }

  function pmCcChipsHtml(ev) {
    if (!ev.chips.length) return '<span class="pm-chip was">' + wsT("js.tk.pm.none") + '</span>';
    return ev.chips.map(function (c) {
      if (c.t === 'g') {
        var lbl = c.org
          ? '&#x1F3E2; ' + wsTn("js.tk.pm.chip.cc.org", c.n, esc(c.name))
          : wsTn("js.tk.pm.chip.cc.grp", c.n, esc(nm(c.id)));
        return '<button type="button" class="pm-chip org" data-pm="chip" data-k="g:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.clear")) + '">' + lbl + '</button>';
      }
      if (c.t === 'rmg') {
        return '<button type="button" class="pm-chip rm" data-pm="chip" data-k="rmg:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.undo")) + '"><s>&minus; ' + esc(c.name) + '</s></button>';
      }
      if (c.t === 'rm') {
        return '<button type="button" class="pm-chip rm" data-pm="chip" data-k="p:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.undo")) + '"><s>&minus; ' + esc(nm(c.id)) + '</s></button>';
      }
      return '<button type="button" class="pm-chip ' + (c.wasOrig ? 'was' : 'add') + '" data-pm="chip" data-k="p:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.clear")) + '">' +
        (c.wasOrig ? '' : '+ ') + esc(nm(c.id)) + '</button>';
    }).join('');
  }

  function pmRowSub(id) {
    var o = orgOf(id);
    return o ? esc(o) + ' &middot; ' + esc(id) : esc(id);
  }

  function pmCcRowHtml(id, isLead, ev, origMap) {
    var v = VS.viewer.member;
    var org = PM.headToOrg && PM.headToOrg[id];
    var showGrp = isLead || !!org;
    var covBy = ev.cov.personBy[id] || '';
    var covered = !!covBy;
    var eff = !!ev.effective[id], retained = !!ev.retained[id], on = !!PM.picks[id] || retained, was = !!origMap[id];
    var cls = 'pm-nd';
    if (covered) cls += ' cvr';
    if (!showGrp && on && !covered) cls += ' on';
    if (!showGrp && covered) cls += ' cov';
    if (eff && !was && !covered) cls += ' add';
    if (!eff && was) cls += ' rm';
    if (PM.focusId === id) cls += ' focus';
    var covTip = covered ? ' title="' + esc(pmCovTip(covBy)) + '"' : '';
    var col1 = isLead ? '<button type="button" class="pm-tg" data-pm="tog" data-id="' + esc(id) + '" aria-label="' + esc(wsT("js.tk.pm.toggle")) + '">&#9662;</button>' : '<span></span>';
    var col2 = showGrp ? '<span></span>' : '<span class="pm-ck" data-pm="self" data-id="' + esc(id) + '" role="checkbox" aria-checked="' + (eff ? 'true' : 'false') + '"' + (covered ? ' aria-disabled="true"' : '') + covTip + '></span>';
    var orgTag = org ? '<span class="tk-org-badge">' + esc(org.name) + '</span>' : '';
    var who = '<span class="pm-who"' + (showGrp ? '' : ' data-pm="self" data-id="' + esc(id) + '"') + covTip + '><span class="pm-nm">' + esc(nm(id)) + '</span>' +
      orgTag + (id === v ? '<span class="pm-chip you">' + wsT("js.tk.pm.you") + '</span>' : '') +
      '<span class="pm-o">' + pmRowSub(id) + '</span></span>';
    var rt = '';
    if (was && eff) rt += '<span class="pm-chip was">' + wsT("js.tk.pm.cc.was") + '</span>';
    else if (!was && eff && !covered) rt += '<span class="pm-chip add">' + wsT("js.tk.pm.cc.add") + '</span>';
    else if (was && !eff) rt += '<span class="pm-chip rm">' + wsT("js.tk.pm.cc.rm") + '</span>';
    if (showGrp) {
      var gCov = ev.cov.groupBy[id] || '', gOn = !!PM.groups[id];
      var gTip = gCov ? ' title="' + esc(pmCovTip(gCov)) + '"' : '';
      rt += '<button type="button" class="pm-opt self' + (on && !covered ? ' on' : '') + (covered ? ' cov' : '') + '" data-pm="self" data-id="' + esc(id) + '" aria-pressed="' + (eff ? 'true' : 'false') + '"' + (covered ? ' aria-disabled="true"' : '') + covTip + '><span class="pm-ck"></span>' + wsT("js.tk.pm.self") + '</button>';
      rt += '<button type="button" class="pm-opt grp' + (gOn && !gCov ? ' on' : '') + (gCov ? ' cov' : '') + '" data-pm="grp" data-id="' + esc(id) + '" aria-pressed="' + ((gOn || !!gCov) ? 'true' : 'false') + '"' + (gCov ? ' aria-disabled="true"' : '') + gTip + '><span class="pm-ck"></span><span class="k">' + pmGrpLabel(id, org) + '</span></button>';
    }
    return '<div class="' + cls + '" data-id="' + esc(id) + '"' + (showGrp ? ' data-lead="1"' : '') + (was ? ' data-was="1"' : '') + '>' +
      col1 + col2 + avHtml(id, false, false) + who + '<span class="pm-rt">' + rt + '</span></div>';
  }

  function pmVacantRowHtml(o) {
    return '<li data-vac-org="' + esc(o.id) + '"><div class="pm-nd vac" aria-disabled="true" title="' + esc(wsT("js.tk.pm.vac.tip")) + '">' +
      '<span></span><span class="pm-ck"></span><i class="av org" aria-hidden="true">&#x1F3E2;</i>' +
      '<span class="pm-who"><span class="pm-nm">' + esc(o.name) + '</span><span class="pm-chip vac">' + esc(wsT("js.tk.org.tag.vacant")) + '</span></span>' +
      '<span class="pm-rt"><span class="pm-hint">' + esc(wsT("js.tk.pm.vac.hint")) + '</span></span></div></li>';
  }

  function pmTreeHtml(flt, rowFn, vacOrgs) {
    var vacList = vacOrgs || [];
    if (!flt.hasAny && !vacList.length) return '<p class="pm-empty">' + wsT("js.tk.pm.empty") + '</p>';
    function walk(node) {
      if (!node || node.cycle || !flt.show[node.id]) return '';
      var ch = (node.children || []).map(walk).join('');
      var shut = PM.shut[node.id] && !flt.expand[node.id];
      return '<li' + (shut ? ' class="shut"' : '') + ' data-node="' + esc(node.id) + '">' +
        rowFn(node.id, !!PM.idx.isLead[node.id]) + (ch ? '<ul>' + ch + '</ul>' : '') + '</li>';
    }
    var h = '';
    var rootsHtml = (PM.ot.roots || []).map(walk).join('') + vacList.map(pmVacantRowHtml).join('');
    if (rootsHtml) h += '<ul class="pm-org">' + rootsHtml + '</ul>';
    var unplaced = (PM.ot.unplaced || []).concat(PM.ot.unreachable || []).filter(function (id) { return !!flt.show[id]; });
    if (unplaced.length) {
      h += '<div class="pm-unas"><h4>' + wsT("js.tk.pm.unplaced") + '</h4>' +
        unplaced.map(function (id) { return rowFn(id, false); }).join('') + '</div>';
    }
    return h;
  }

  function pmAsgFootHtml(ev) {
    var left = PM.single ? '<span class="pm-sum">' + wsT("js.tk.ap.topic") + '</span>' :
      '<span class="pm-cm" role="radiogroup" aria-label="' + esc(wsT("js.tk.form.completion")) + '">' + ['assignee', 'assigner'].map(function (c) {
        var on = ev.mode === c;
        return '<button type="button" role="radio" class="' + (on ? 'on' : '') + '" aria-checked="' + (on ? 'true' : 'false') + '" data-pm="compl" data-c="' + c + '"' + (c === 'assignee' && ev.below ? ' hidden' : '') + '>' + esc(complText(c)) + '</button>';
      }).join('') + '</span>' +
      (ev.mode === 'mixed' ? '<span class="pm-sum" title="' + esc(wsT("js.tk.ap.compl.mixed.tip")) + '"><b>' + wsT("js.tk.ap.compl.mixed") + '</b></span>' : '');
    var remCnt = (ev.removedRules || []).length;
    var cnt = ev.targets.length + remCnt;
    var applyLbl = PM.retarget ? wsT("js.tk.pm.retarget.apply")
      : (ev.targets.length === 0 && remCnt > 0 ? wsTn("js.tk.pm.cc.apply.n", remCnt)
        : (cnt ? wsTn("js.tk.pm.asg.apply.n", cnt) : wsT("js.tk.pm.asg.apply")));
    return '<div class="pm-row">' + left + '<span class="grow"></span>' +
      '<button type="button" class="pm-btn" data-pm="close">' + wsT("js.tk.dlg.cancel") + '</button>' +
      '<button type="button" class="pm-btn primary" id="tkpm-apply" data-pm="apply"' + (!ev.dirty || PM.busy ? ' disabled' : '') + '>' + applyLbl + '</button></div>';
  }

  function pmDraw() {
    if (!PM) return;
    var isCc = PM.kind === 'cc', origMap = {}, ev, flt, rowFn, chipsHtml, footHtml;
    var vacOrgs = PM.norule ? [] : pmVacantOrgs(PM.oi, PM.q, isCc && PM.only);
    if (isCc) {
      PM.origCc.forEach(function (id) { origMap[id] = true; });
      ev = pmCcEvalCur();
      flt = pmFilter(PM.ot, PM.q, PM.only, function (id) {
        return !!PM.picks[id] || !!PM.groups[id] || !!ev.effective[id] || !!origMap[id] || !!(PM.origOrgByHead && PM.origOrgByHead[id]);
      }, pmMatch);
      chipsHtml = pmCcChipsHtml(ev);
      rowFn = function (id, isLead) { return pmCcRowHtml(id, isLead, ev, origMap); };
      var cnt = ev.eventCount;
      var ccApplyLbl = PM.retarget ? wsT("js.tk.pm.retarget.apply") : (cnt ? wsTn("js.tk.pm.cc.apply.n", cnt) : wsT("js.tk.pm.cc.apply"));
      var ccSumTxt = PM.retarget ? wsTn("js.tk.pm.retarget.cc.sum", ev.effectiveCount) : wsT("js.tk.pm.cc.sum", ev.effectiveCount, ev.added.length, ev.removed.length);
      footHtml = '<div class="pm-row"><span class="pm-sum">' + ccSumTxt +
        '</span><span class="grow"></span><button type="button" class="pm-btn" data-pm="close">' + wsT("js.tk.dlg.cancel") + '</button>' +
        '<button type="button" class="pm-btn primary" id="tkpm-apply" data-pm="apply"' + (!ev.dirty || PM.busy ? ' disabled' : '') + '>' +
        ccApplyLbl + '</button></div>';
    } else {
      ev = pmAssignEvalCur();
      flt = pmFilter(PM.ot, PM.q, false, function () { return true; }, pmMatch);
      chipsHtml = pmAsgChipsHtml(ev);
      rowFn = function (id, isLead) { return pmAsgRowHtml(id, isLead, ev); };
      footHtml = pmAsgFootHtml(ev);
    }
    var chips = $('tkpm-chips');
    if (chips) chips.innerHTML = chipsHtml;
    var body = $('tkpm-body');
    if (body) { var st = body.scrollTop; body.innerHTML = pmTreeHtml(flt, rowFn, vacOrgs); body.scrollTop = st; }
    var onlyBtn = $('tkpm-only');
    if (onlyBtn) { onlyBtn.setAttribute('aria-pressed', PM.only ? 'true' : 'false'); onlyBtn.classList.toggle('on', PM.only); }
    var err = $('tkpm-err');
    if (err) { err.textContent = PM.err || ''; err.hidden = !PM.err; }
    var foot = $('tkpm-foot');
    if (foot) foot.innerHTML = footHtml;
  }

  function pmVisibleIds() {
    var body = $('tkpm-body');
    if (!body) return [];
    var out = [];
    Array.prototype.forEach.call(body.querySelectorAll('.pm-nd[data-id]'), function (el) {
      var li = el.parentNode;
      if (li && li.parentNode && li.parentNode.closest && li.parentNode.closest('li.shut')) return;
      out.push(el.getAttribute('data-id'));
    });
    return out;
  }

  function pmTogMap(map, id) {
    if (map[id]) delete map[id]; else map[id] = true;
  }

  function pmToggleCcPerson(id) {
    var ev = pmCcEvalCur();
    if (ev.cov.personBy[id]) return false;
    PM.err = '';
    if (ev.effective[id]) {
      delete PM.picks[id];
      if (ev.retained[id]) PM.unpicks[id] = true;
    } else if (PM.unpicks[id]) {
      delete PM.unpicks[id];
      if (PM.origPersonal && PM.origPersonal[id]) PM.picks[id] = true;
      if (!pmCcEvalCur().effective[id]) PM.picks[id] = true;
    } else {
      PM.picks[id] = true;
    }
    return true;
  }

  function pmToggleSelf(id, fromKey) {
    if (!PM || !id) return;
    if (PM.kind === 'cc') {
      if (!pmToggleCcPerson(id)) return;
    } else {
      if (PM.lockedP[id] || pmAssignEvalCur().visCov.personBy[id]) return;
      PM.err = '';
      if (PM.single) {
        if (PM.picks[id]) delete PM.picks[id];
        else { PM.picks = {}; PM.picks[id] = true; }
      } else pmTogMap(PM.picks, id);
    }
    if (!fromKey) PM.focusId = null;
    pmDraw();
  }

  function pmToggleGrp(id) {
    if (!PM || !id || PM.norule) return;
    if (PM.kind === 'cc') {
      if (pmCcEvalCur().cov.groupBy[id]) return;
    } else {
      if (PM.lockedG[id] || pmAssignEvalCur().visCov.groupBy[id]) return;
    }
    PM.err = '';
    pmTogMap(PM.groups, id);
    PM.focusId = null;
    pmDraw();
  }

  function pmRestoreFocus(backEl, back) {
    var el = backEl && backEl.isConnected ? backEl : back && document.querySelector(back);
    if (el && !el.matches('a[href], button, input, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]');
    if (el) el.focus({ preventScroll: true });
  }

  function pmApply() {
    if (!PM || PM.busy) return;
    var cur = PM;
    if (cur.kind === 'assign' && cur.draft) {
      var ev = pmAssignEvalCur();
      if (!ev.targets.length) return;
      var n = cur.ed ? ui.ned : ui.nrow[cur.store];
      if (n) { n.who = ev.targets.map(function (o) { return o.id; }); n.compl = cur.compl; }
      var backId = cur.backEl && cur.backEl.id, ed = cur.ed;
      pmClose(false);
      render();
      var el = (backId && $(backId)) || $(ed ? 'tkned' : 'tknew');
      if (el) el.focus({ preventScroll: true });
      return;
    }
    var events = pmBuildApplyEvents(cur, VS);
    if (!events.length) {
      if (VS) {
        cur.oi = orgIndex(VS.orgs, VS.members);
        cur.headToOrg = cur.oi.headToOrg;
        cur.ot = cur.oi.tree;
        cur.idx = pmIndex(cur.ot);
        pmDraw();
      }
      return;
    }
    cur.busy = true;
    cur.err = '';
    pmDraw();
    AD.dispatch(cur.store, events.map(function (ev) {
      return { type: ev[0], payload: ev[1], origin: ev[2] || 'human' };
    }), function (res) {
      if (PM !== cur) return;
      if (!res || !res.ok) {
        cur.busy = false;
        var r = res && res.refusal && res.refusal.reason;
        cur.err = r ? String(r) : String((res && res.error) || wsT("js.tk.toast.refused"));
        pmDraw();
        return;
      }
      var backEl = cur.backEl, back = cur.back;
      pmClose(false);
      wrote(res.results || []);
      pmRestoreFocus(backEl, back);
    });
  }

  function pmClick(e) {
    e.stopPropagation();
    if (!PM) return;
    if (e.target === $('tkpm')) { pmClose(true); return; }
    var a = e.target.closest('[data-pm]');
    if (!a) {
      var row = e.target.closest('.pm-nd[data-id]');
      if (row && row.getAttribute('data-lead') !== '1' && e.target.closest('.av')) {
        pmToggleSelf(row.getAttribute('data-id'));
      }
      return;
    }
    var act = a.getAttribute('data-pm'), id = a.getAttribute('data-id');
    if (act === 'close') { pmClose(true); return; }
    if (act === 'expand') { PM.shut = {}; pmDraw(); return; }
    if (act === 'only') { PM.only = !PM.only; pmDraw(); return; }
    if (act === 'tog' && id) { pmTogMap(PM.shut, id); pmDraw(); return; }
    if (act === 'self' && id) { pmToggleSelf(id); return; }
    if (act === 'grp' && id) { pmToggleGrp(id); return; }
    if (act === 'compl') { PM.compl = a.getAttribute('data-c'); pmDraw(); return; }
    if (act === 'chip') {
      var k = a.getAttribute('data-k') || '', sep = k.indexOf(':');
      if (sep < 0) return;
      var kind = k.slice(0, sep), cid = k.slice(sep + 1);
      PM.err = '';
      if (kind === 'g') delete PM.groups[cid];
      else if (kind === 'rmg') PM.groups[cid] = true;
      else if (kind === 'p') {
        if (PM.kind === 'cc') pmToggleCcPerson(cid);
        else pmTogMap(PM.picks, cid);
      }
      pmDraw();
      return;
    }
    if (act === 'apply') pmApply();
  }

  function pmInput(e) {
    if (!PM) return;
    var t = e.target;
    if (t && t.id === 'tkpmq') {
      if (e.isComposing) return;
      PM.q = t.value || '';
      pmDraw();
    }
  }

  function pmKey(e) {
    if (!PM) return;
    e.stopPropagation();
    var k = e.key, t = e.target;
    if (k === 'Escape') { e.preventDefault(); pmClose(true); return; }
    if (e.isComposing || e.keyCode === 229) return;
    if ((k === 'ArrowLeft' || k === 'ArrowRight') && t && t.closest('.pm-cm')) {
      var nx = t.parentNode.querySelector('[data-c]:not([hidden]):not([data-c="' + t.getAttribute('data-c') + '"])');
      if (nx) { e.preventDefault(); PM.compl = nx.getAttribute('data-c'); pmDraw(); var nb = $('tkpm-foot') && $('tkpm-foot').querySelector('[data-c="' + PM.compl + '"]'); if (nb) nb.focus(); }
      return;
    }
    if (k === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); pmApply(); return; }
    if (k === 'ArrowDown' || k === 'ArrowUp') {
      var ids = pmVisibleIds();
      if (!ids.length) return;
      e.preventDefault();
      var at = PM.focusId ? ids.indexOf(PM.focusId) : -1;
      var nxIdx = at < 0 ? (k === 'ArrowDown' ? 0 : ids.length - 1) : (at + (k === 'ArrowDown' ? 1 : ids.length - 1)) % ids.length;
      PM.focusId = ids[nxIdx];
      pmDraw();
      var row = $('tkpm-body') && $('tkpm-body').querySelector('.pm-nd[data-id="' + CSS.escape(PM.focusId) + '"]');
      if (row && row.scrollIntoView) row.scrollIntoView({ block: 'nearest' });
      return;
    }
    if (k === 'Enter' && t && t.id === 'tkpmq') {
      e.preventDefault();
      var visIds = pmVisibleIds();
      var targetId = (PM.focusId && visIds.indexOf(PM.focusId) >= 0) ? PM.focusId : visIds[0];
      if (targetId) pmToggleSelf(targetId, true);
    }
  }
  // --- end picker modal state & renderer
