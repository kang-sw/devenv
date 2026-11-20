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

  // --- picker modal state & renderer
  var PM = null;

  function pmMatch(id, qq) {
    var m = VS && VS.members[id];
    var hay = (nm(id) + ' ' + id + ' ' + (m && m.profile && m.profile.org ? m.profile.org : '')).toLowerCase();
    return hay.indexOf(qq) >= 0;
  }

  function openPicker(kind, ctx, opener) {
    if (!VS) return;
    ctx = ctx || {};
    var it = ctx.item ? VS.items[ctx.item] : null;
    if (!ctx.draft && !it) return;
    var ot = orgTree(VS.members), idx = pmIndex(ot);
    var picks = {}, origCc = [];
    if (kind === 'cc') {
      origCc = listOf(it.cc).slice();
      origCc.forEach(function (id) { if (idx.listed[id]) picks[id] = true; });
    }
    PM = {
      kind: kind,
      item: ctx.item || null,
      draft: !!ctx.draft,
      ed: !!ctx.ed,
      store: ui.store,
      ot: ot,
      idx: idx,
      origCc: origCc,
      picks: picks,
      groups: {},
      lockedP: {},
      lockedG: {},
      norule: false,
      single: false,
      compl: null,
      shut: {},
      q: '',
      only: false,
      focusId: null,
      err: '',
      busy: false,
      backEl: opener || null,
      back: opener ? returnSel(opener) : null
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
    var title = PM.kind === 'cc' ? wsT("js.tk.pm.cc.title") : wsT("js.tk.pm.assign.title");
    var sub = it ? '<code>' + esc(it.alias) + '</code> ' + esc(it.title) : '';
    var hasTools = PM.kind === 'cc';
    return '<div class="pm-dlg" role="document" aria-label="' + esc(title) + '">' +
      '<div class="pm-dh"><h2>' + title + '</h2>' + (sub ? '<span class="pm-sub">' + sub + '</span>' : '') +
      '<span class="grow"></span><button type="button" class="pm-x" data-pm="close" aria-label="' + esc(wsT("js.tk.common.close")) + '">&times;</button></div>' +
      '<div class="pm-tb"><input type="search" id="tkpmq" autocomplete="off" spellcheck="false" placeholder="' + esc(wsT("js.tk.pm.search")) + '" aria-label="' + esc(wsT("js.tk.pm.search")) + '">' +
      (hasTools ? '<button type="button" class="pm-btn mini" data-pm="expand">' + wsT("js.tk.pm.expand") + '</button>' +
        '<button type="button" class="pm-btn mini" id="tkpm-only" data-pm="only" aria-pressed="false" title="' + esc(wsT("js.tk.pm.only.tip")) + '">' + wsT("js.tk.pm.only") + '</button>' : '') + '</div>' +
      '<div class="pm-sel"><span class="pm-lab">' + wsT("js.tk.pm.selected") + '</span><div class="pm-chips" id="tkpm-chips"></div></div>' +
      '<div class="pm-body" id="tkpm-body"></div>' +
      '<div class="pm-err" id="tkpm-err" role="alert" hidden></div>' +
      '<div class="pm-df" id="tkpm-foot"></div>' +
      '</div>';
  }

  function pmCcEvalCur() {
    return pmCcEval(PM.idx, PM.origCc, PM.picks, PM.groups);
  }

  function pmCcChipsHtml(ev) {
    if (!ev.chips.length) return '<span class="pm-chip was">' + wsT("js.tk.pm.none") + '</span>';
    return ev.chips.map(function (c) {
      if (c.t === 'g') {
        return '<button type="button" class="pm-chip org" data-pm="chip" data-k="g:' + esc(c.id) + '" title="' + esc(wsT("js.tk.pm.chip.clear")) + '">' +
          wsTn("js.tk.pm.chip.cc.grp", c.n, esc(nm(c.id))) + '</button>';
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
    var covBy = ev.cov.personBy[id] || '';
    var covered = !!covBy;
    var on = !!PM.picks[id], eff = on || covered, was = !!origMap[id];
    var cls = 'pm-nd';
    if (covered) cls += ' cvr';
    if (!isLead && on && !covered) cls += ' on';
    if (!isLead && covered) cls += ' cov';
    if (eff && !was && !covered) cls += ' add';
    if (!eff && was) cls += ' rm';
    if (PM.focusId === id) cls += ' focus';
    var covTip = covered ? ' title="' + esc(wsT("js.tk.pm.cov.tip", nm(covBy))) + '"' : '';
    var col1 = isLead ? '<button type="button" class="pm-tg" data-pm="tog" data-id="' + esc(id) + '" aria-label="' + esc(wsT("js.tk.pm.toggle")) + '">&#9662;</button>' : '<span></span>';
    var col2 = isLead ? '<span></span>' : '<span class="pm-ck" data-pm="self" data-id="' + esc(id) + '" role="checkbox" aria-checked="' + (eff ? 'true' : 'false') + '"' + (covered ? ' aria-disabled="true"' : '') + covTip + '></span>';
    var who = '<span class="pm-who"' + (isLead ? '' : ' data-pm="self" data-id="' + esc(id) + '"') + covTip + '><span class="pm-nm">' + esc(nm(id)) + '</span>' +
      (id === v ? '<span class="pm-chip you">' + wsT("js.tk.pm.you") + '</span>' : '') +
      '<span class="pm-o">' + pmRowSub(id) + '</span></span>';
    var rt = '';
    if (was && eff) rt += '<span class="pm-chip was">' + wsT("js.tk.pm.cc.was") + '</span>';
    else if (!was && eff && !covered) rt += '<span class="pm-chip add">' + wsT("js.tk.pm.cc.add") + '</span>';
    else if (was && !eff) rt += '<span class="pm-chip rm">' + wsT("js.tk.pm.cc.rm") + '</span>';
    if (isLead) {
      var gCov = ev.cov.groupBy[id] || '', gOn = !!PM.groups[id];
      var gTip = gCov ? ' title="' + esc(wsT("js.tk.pm.cov.tip", nm(gCov))) + '"' : '';
      rt += '<button type="button" class="pm-opt self' + (on && !covered ? ' on' : '') + (covered ? ' cov' : '') + '" data-pm="self" data-id="' + esc(id) + '" aria-pressed="' + (eff ? 'true' : 'false') + '"' + (covered ? ' aria-disabled="true"' : '') + covTip + '><span class="pm-ck"></span>' + wsT("js.tk.pm.self") + '</button>';
      rt += '<button type="button" class="pm-opt grp' + (gOn && !gCov ? ' on' : '') + (gCov ? ' cov' : '') + '" data-pm="grp" data-id="' + esc(id) + '" aria-pressed="' + ((gOn || !!gCov) ? 'true' : 'false') + '"' + (gCov ? ' aria-disabled="true"' : '') + gTip + '><span class="pm-ck"></span><span class="k">' + wsTn("js.tk.pm.grp", PM.idx.count[id] || 1) + '</span></button>';
    }
    return '<div class="' + cls + '" data-id="' + esc(id) + '"' + (isLead ? ' data-lead="1"' : '') + (was ? ' data-was="1"' : '') + '>' +
      col1 + col2 + avHtml(id, false, false) + who + '<span class="pm-rt">' + rt + '</span></div>';
  }

  function pmTreeHtml(flt, rowFn) {
    if (!flt.hasAny) return '<p class="pm-empty">' + wsT("js.tk.pm.empty") + '</p>';
    function walk(node) {
      if (!node || node.cycle || !flt.show[node.id]) return '';
      var ch = (node.children || []).map(walk).join('');
      var shut = PM.shut[node.id] && !flt.expand[node.id];
      return '<li' + (shut ? ' class="shut"' : '') + ' data-node="' + esc(node.id) + '">' +
        rowFn(node.id, !!PM.idx.isLead[node.id]) + (ch ? '<ul>' + ch + '</ul>' : '') + '</li>';
    }
    var h = '';
    var rootsHtml = (PM.ot.roots || []).map(walk).join('');
    if (rootsHtml) h += '<ul class="pm-org">' + rootsHtml + '</ul>';
    var unplaced = (PM.ot.unplaced || []).concat(PM.ot.unreachable || []).filter(function (id) { return !!flt.show[id]; });
    if (unplaced.length) {
      h += '<div class="pm-unas"><h4>' + wsT("js.tk.pm.unplaced") + '</h4>' +
        unplaced.map(function (id) { return rowFn(id, false); }).join('') + '</div>';
    }
    return h;
  }

  function pmDraw() {
    if (!PM) return;
    var origMap = {};
    PM.origCc.forEach(function (id) { origMap[id] = true; });
    var ev = pmCcEvalCur();
    var flt = pmFilter(PM.ot, PM.q, PM.only, function (id) {
      return !!PM.picks[id] || !!PM.groups[id] || !!ev.effective[id] || !!origMap[id];
    }, pmMatch);
    var chips = $('tkpm-chips');
    if (chips) chips.innerHTML = pmCcChipsHtml(ev);
    var body = $('tkpm-body');
    if (body) {
      var st = body.scrollTop;
      body.innerHTML = pmTreeHtml(flt, function (id, isLead) { return pmCcRowHtml(id, isLead, ev, origMap); });
      body.scrollTop = st;
    }
    var onlyBtn = $('tkpm-only');
    if (onlyBtn) {
      onlyBtn.setAttribute('aria-pressed', PM.only ? 'true' : 'false');
      onlyBtn.classList.toggle('on', PM.only);
    }
    var err = $('tkpm-err');
    if (err) {
      err.textContent = PM.err || '';
      err.hidden = !PM.err;
    }
    var foot = $('tkpm-foot');
    if (foot) {
      var cnt = ev.added.length + ev.removed.length;
      var applyLbl = cnt ? wsTn("js.tk.pm.cc.apply.n", cnt) : wsT("js.tk.pm.cc.apply");
      foot.innerHTML = '<div class="pm-row"><span class="pm-sum">' +
        wsT("js.tk.pm.cc.sum", ev.effectiveCount, ev.added.length, ev.removed.length) +
        '</span><span class="grow"></span>' +
        '<button type="button" class="pm-btn" data-pm="close">' + wsT("js.tk.common.cancel") + '</button>' +
        '<button type="button" class="pm-btn primary" id="tkpm-apply" data-pm="apply"' + (!ev.dirty || PM.busy ? ' disabled' : '') + '>' + applyLbl + '</button></div>';
    }
  }

  function pmClick() {}
  function pmInput() {}
  function pmKey() {}
  // --- end picker modal state & renderer
