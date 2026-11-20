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
  function pmCover(idx, groups) {
    var G = groups || {}, personBy = {}, groupBy = {}, topGroups = [];
    idx.order.forEach(function (id) {
      var anc = idx.ancestors[id] || [], up = '';
      for (var i = anc.length - 1; i >= 0; i--) {
        if (G[anc[i]]) { up = anc[i]; break; }
      }
      if (up) {
        personBy[id] = up;
        if (idx.isLead[id]) groupBy[id] = up;
      } else if (G[id] && idx.isLead[id]) {
        personBy[id] = id;
        topGroups.push(id);
      }
    });
    return { personBy: personBy, groupBy: groupBy, topGroups: topGroups };
  }
  // Evaluates CC picker state over listed live members only. Unlisted/removed members in origCc
  // are never placed in removed. Group choices expand to their current subtree snapshot on apply.
  function pmCcEval(idx, origCc, picks, groups) {
    var P = picks || {}, G = groups || {}, orig = {};
    (origCc || []).forEach(function (id) { orig[id] = true; });
    var cov = pmCover(idx, G);
    var effective = {}, added = [], removed = [], grpChips = [], addChips = [], rmChips = [], effectiveCount = 0;
    cov.topGroups.forEach(function (gid) {
      grpChips.push({ t: 'g', id: gid, n: idx.count[gid] || 1 });
    });
    idx.order.forEach(function (id) {
      var on = !!P[id], covered = !!cov.personBy[id], eff = on || covered, was = !!orig[id];
      if (eff) { effective[id] = true; effectiveCount++; }
      if (eff && !was) added.push(id);
      if (!eff && was) removed.push(id);
      if (on && !covered) addChips.push({ t: 'p', id: id, wasOrig: was });
      if (!eff && was) rmChips.push({ t: 'rm', id: id });
    });
    return {
      cov: cov,
      effective: effective,
      added: added,
      removed: removed,
      chips: grpChips.concat(addChips, rmChips),
      dirty: added.length > 0 || removed.length > 0,
      effectiveCount: effectiveCount
    };
  }
  // Builds atomic item.cc / item.uncc event batch for CC picker apply.
  function pmCcEvents(itemId, idx, origCc, picks, groups) {
    var ev = pmCcEval(idx, origCc, picks, groups), out = [];
    ev.added.forEach(function (id) { out.push(['item.cc', { item: itemId, member: id }]); });
    ev.removed.forEach(function (id) { out.push(['item.uncc', { item: itemId, member: id }]); });
    return out;
  }
  // Whether existing live rule y covers proposed whole-subtree rule {root, include_root: true} (no levels).
  function pmRuleCovers(idx, y, root) {
    if (!y || y.levels != null) return false;
    if (y.root === root) return !!(y.includeRoot || y.include_root);
    return !!(idx.desc[y.root] && idx.desc[y.root][root] > 0);
  }
  // Whether any live rule of viewer on itemId already covers {root, include_root: true}.
  function pmRuleCovered(idx, rules, itemId, viewer, root) {
    var list = Array.isArray(rules) ? rules : Object.keys(rules || {}).map(function (k) { return rules[k]; });
    return list.some(function (y) {
      return y && y.item === itemId && y.by === viewer && y.state === 'live' && pmRuleCovers(idx, y, root);
    });
  }
  // Pure completion class for a single assign target, matching complClass/asgClass.
  function pmComplClass(o, viewer, reach) {
    var id = o.t === 'r' ? o.root : o.id;
    return (o.t === 'r' && id === viewer) || !!(reach && reach[id]) ? 'assigner' : 'assignee';
  }
  function pmAsgClass(o, compl, viewer, reach) {
    return pmComplClass(o, viewer, reach) === 'assigner' ? 'assigner' : (compl || 'assignee');
  }
  // Evaluates assign picker state.
  // - visCov uses all unlocked active groups (live + pending) for the tree's covered display.
  // - payCov uses ONLY live active groups (root === viewer or in reach), so picks/sub-groups under
  //   a pending group remain in payload and keep their own chips (chip band === payload).
  function pmAssignEval(idx, opts) {
    opts = opts || {};
    var viewer = opts.viewer || '', reach = opts.reach || {};
    var lockedP = opts.lockedPeople || {}, lockedG = opts.lockedGroups || {};
    var P = opts.picks || {}, G = opts.groups || {};
    var norule = !!opts.norule, single = !!opts.single, compl = opts.compl || null;
    function isLive(id) { return id === viewer || !!reach[id]; }
    var activeG = {}, liveG = {};
    if (!norule) {
      idx.order.forEach(function (id) {
        if (idx.isLead[id] && G[id] && !lockedG[id]) {
          activeG[id] = true;
          if (isLive(id)) liveG[id] = true;
        }
      });
    }
    var visCov = pmCover(idx, activeG);
    var payCov = pmCover(idx, liveG);
    var targets = [];
    idx.order.forEach(function (id) {
      if (activeG[id] && !payCov.groupBy[id]) {
        targets.push({ t: 'r', root: id, include_root: true, levels: null, pend: !isLive(id), n: idx.count[id] || 1 });
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
    var chips = targets.map(function (o) {
      return o.t === 'r' ? { t: 'g', id: o.root, n: o.n, pend: o.pend } : { t: 'p', id: o.id, pend: o.pend };
    });
    return { visCov: visCov, payCov: payCov, targets: targets, chips: chips, below: below, mode: mode, dirty: targets.length > 0 };
  }
  // Builds item.assign event batch from pmAssignEval targets, grouped by completion class.
  function pmAssignEvents(itemId, targets, compl, viewer, reach) {
    var groups = [], by = {};
    (targets || []).forEach(function (o) {
      var c = pmAsgClass(o, compl, viewer, reach);
      if (!by[c]) groups.push(by[c] = { c: c, sel: [] });
      by[c].sel.push(o);
    });
    return groups.map(function (g) {
      var people = g.sel.filter(function (o) { return o.t === 'p'; }).map(function (o) { return o.id; });
      var rules = g.sel.filter(function (o) { return o.t === 'r'; }).map(function (o) {
        var r = { root: o.root };
        if (o.include_root) r.include_root = true;
        if (o.levels != null) r.levels = o.levels;
        return r;
      });
      return ['item.assign', { item: itemId, targets: people.concat(rules), completion: g.c }];
    });
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
  // --- end picker modal pure logic
