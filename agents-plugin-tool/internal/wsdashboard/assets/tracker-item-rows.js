// Tracker item row rendering and edit controls.
  // ---------------------------------------------------------------- item rows
 
  // An item row shows by default only what must be seen: status icon, title,
  // assignee initials, priority, due, a warning icon for flagged or concurrent writes,
  // and an unread dot. Everything else (kind, source clone and event id, dates, roll-up,
  // assignment path, notes) is the row's detail overlay, and the actions are icons; both
  // appear on hover. Keyboard focus and a tap-selection (ui.sel) show the same:
  // one CSS state, `.irow:is(:hover, :focus-within, .sel)`. (:focus-within, because
  // Chromium does not re-match :has(:focus-visible) in time when Tab moves from the row
  // into its buttons, so they hide mid-move and focus falls to the body.)
  // The overlay ignores the pointer, so moving down a list hovers the next row.

  // A topic has no status: its live record (its holder) shows the topic, a pending one stays pending.
  function stKey(it, r) { var topic = !!it && it.kind === 'topic'; return r ? (r.state !== 'live' ? r.state : topic ? 'topic' : r.status) : topic ? 'topic' : it.status; }
  function tipAttr(text) { return ' data-tip="' + esc(text) + '"'; }
  // data-tip from already-escaped HTML: drop the tags, keep the entities (the attribute decodes them)
  function tipHtml(h) { return ' data-tip="' + String(h).replace(/<[^>]*>/g, '').replace(/"/g, '&quot;') + '"'; }
  function stDesc(s) { var x = SI.info(s); return wsT("js.tk.st.tip", x.label, x.desc); }
  function stIcon(s) { return '<span class="sti"' + tipAttr(stDesc(s)) + '>' + SI.svg(s) + '</span>'; }
  // The avatar text comes from the view state (the profile rules live in the data layer).
  function initials(id) { var m = VS.members[id]; return m ? m.avatar : String(id).slice(0, 2).toUpperCase(); }
  // A person's relation to the viewer in the org tree, for the avatar color: {c: class suffix,
  // t: tooltip}. Up the viewer's lead chain is warm (r-u1 the direct lead .. r-u3 three or more
  // levels up), down the viewer's reach green (r-d1 direct reports .. r-d3), anyone else gray.
  function avRel(id) {
    var v = VS.viewer.member, m = VS.members[id], d = 0, me = VS.members[v], R = VS.viewer.reach || {};
    if (id === v) return { c: ' me', t: wsT("js.tk.av.self") };
    if (!m || m.removed) return { c: ' r-n', t: wsT("js.tk.av.none") };
    while (me && (me.leads || [])[0] && d < 50) {
      d++;
      if (me.leads[0] === id) return { c: ' r-u' + Math.min(d, 3), t: d === 1 ? wsT("js.tk.av.up1") : wsT("js.tk.av.up", d) };
      me = VS.members[me.leads[0]];
    }
    if (R[id]) return { c: ' r-d' + Math.min(R[id], 3), t: R[id] === 1 ? wsT("js.tk.av.dn1") : wsT("js.tk.av.dn", R[id]) };
    return { c: ' r-n', t: wsT("js.tk.av.none") };
  }
  // A person's avatar; every person avatar goes through here so the relation colors stay one rule.
  // tip: false where an enclosing tooltip already names the relation.
  function avHtml(id, pend, tip) {
    var r = avRel(id);
    return '<i class="av' + (pend ? ' pend' : '') + r.c + '"' + (tip === false ? '' : tipAttr(r.t)) + '>' + esc(initials(id)) + '</i>';
  }
  function holders(it) { return it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }); }
  // A topic's holder in markup (a pending handoff names its receiver as pending), '' when nobody holds it.
  function holderNames(it) { return holders(it).map(function (r) { return r.state === 'pending' ? wsT("js.tk.row.assignee.pending", whoName(r.to)) : whoName(r.to); }).join(', '); }
  // A list row's assignee, priority, or due part: a plain span (tooltip only), or, when step names
  // the item menu step the viewer may open from it (irow), a button that opens that step through
  // the click handler's STEPS dispatch, the pickers the quick-add row's chips and the item menu use.
  function partHtml(cls, tip, inner, it, step) {
    if (!step) return '<span class="' + cls + '"' + (tip ? tipAttr(tip) : '') + '>' + inner + '</span>';
    return '<button type="button" class="' + cls + ' rp"' + attr('act', step) + attr('item', it.id) + ' tabindex="-1" aria-haspopup="' + (step === 'assign' ? 'dialog' : 'menu') + '" aria-label="' + esc(tip) + '"' + tipAttr(tip) + '>' + inner + '</button>';
  }
  // an empty part's dim icon (.rpe), shown on the row's hover, focus, or tap-selection
  function partIcon(icon) { return '<svg class="i"><use href="#tki-' + icon + '"/></svg>'; }
  // ed: the viewer may assign (canAssign); the group then opens the assign step, an empty one as the dim icon
  function whoHtml(it, ed) {
    var as = holders(it), step = ed ? 'assign' : null;
    if (!as.length) return ed ? partHtml('who rpe', wsT("js.tk.act.assign"), partIcon('assign'), it, step) : '<span class="who"></span>';
    var shown = as.slice(0, 3), more = as.length - shown.length;
    // the avatars fill the group, so the group tooltip names each relation instead of one per avatar
    var tip = wsT("js.tk.row.assignees", as.map(function (r) { var n = wsT("js.tk.av.who", nmo(r.to), avRel(r.to).t); return r.state === 'pending' ? wsT("js.tk.row.assignee.pending", n) : n; }).join(', '));
    return partHtml('who', tip, shown.map(function (r) {
      return avHtml(r.to, r.state === 'pending', false);
    }).join('') + (more ? '<i class="av more">+' + more + '</i>' : ''), it, step);
  }
  // ed: the viewer may edit the task (canEdit); the level then opens the priority step
  function priHtml(it, ed) {
    if (!it || it.kind === 'topic') return '<span class="pri"></span>';
    var lv = it.priorityLevel, d = VS.priority.default;
    return partHtml('pri' + (lv > d ? ' hi' : lv < d ? ' lo' : ''), wsT("js.tk.pri.tip.name", lv, VS.priority.names.length - 1, it.priorityName) +
      (it.priority !== lv ? ' ' + wsT("js.tk.pri.stored", it.priority) : ''), esc(it.priorityName), it, ed ? 'pri' : null);
  }
  // Due: D-N within 7 days, D-day today, D+N past due (danger colour), else a short date.
  var ENDED = { done: 1, dropped: 1, declined: 1, returned: 1, handed: 1, withdrawn: 1 };
  // ed: the viewer may edit the task (canEdit); the date then opens the due step, an empty one as the dim icon.
  // A start date alone shows "from MM-DD" in the neutral badge; with a due as well the due's form
  // stays, marked as a range (rng), the start leading its tooltip.
  function dueHtml(it, st, ed) {
    if (!it || it.kind === 'topic') return '<span class="due2"></span>';
    if (!it.due && it.start) return partHtml('due2 from', wsT("js.tk.due.tip.from", it.start), esc(wsT("js.tk.due.from", it.start.slice(5))), it, ed ? 'due' : null);
    if (!it.due) return ed ? partHtml('due2 rpe', wsT("js.tk.menu.due"), partIcon('calendar'), it, 'due') : '<span class="due2"></span>';
    var state = itemDueState(it), n = dayNum(it.due) - dayNum(today()), t, c, tip;
    // The projection owns the boundary; the hourly global clock still supplies
    // the day distance (its UTC date is unchanged by hourly truncation).
    if (state === 'today') n = 0;
    else if (state === 'over') n = Math.min(-1, n);
    else if (state === 'future') n = Math.max(1, n);
    if (ENDED[st]) { t = it.due.slice(5); c = 'ended'; tip = wsT("js.tk.due.tip.ended", it.due, SI.info(st).label); }
    else if (n < 0) { t = wsT("js.tk.due.dplus", -n); c = 'over'; tip = wsTn("js.tk.due.tip.overby", -n, it.due); }
    else if (n === 0) { t = wsT("js.tk.due.dday"); c = 'now'; tip = wsT("js.tk.due.tip.today", it.due); }
    else if (n <= 7) { t = wsT("js.tk.due.dminus", n); c = 'soon'; tip = wsTn("js.tk.due.tip.in", n, it.due); }
    else { t = it.due.slice(5); c = ''; tip = wsT("js.tk.due.on", it.due); }
    if (it.start) { c += (c ? ' ' : '') + 'rng'; tip = wsT("js.tk.due.tip.from", it.start) + ' · ' + tip; }
    return partHtml('due2' + (c ? ' ' + c : ''), tip, esc(t), it, ed ? 'due' : null);
  }
  // a short start..due text for a menu value or a chip: "11-10–11-19", the due alone, or "from 11-10"
  function rangeText(s, d) { return s && d ? s.slice(5) + '–' + d.slice(5) : d ? d : s ? wsT("js.tk.due.from", s.slice(5)) : ''; }
  // A list row's label icon: an editor's way to the labels step (the chips themselves search,
  // lsearch); with the store's list empty only an admin, who can add a label there, gets it.
  function labelEdHtml(it) {
    if (!canEdit(it) || !((VS.labels || []).length || VS.viewer.admin)) return '';
    return partHtml('rlab rpe', wsT("js.tk.prop.labels.edit"), partIcon('tag'), it, 'labels');
  }
  // A list row's pencil: an editor's way into the row's edit (startRowEdit); it takes no room
  // until the row is hovered or focused, like the label icon.
  function titleEdHtml(it) {
    if (!canEdit(it)) return '';
    var tip = wsT("js.tk.act.edit");
    return '<button type="button" class="rp rpe rtit" tabindex="-1"' + attr('act', 'tedit') + attr('item', it.id) + ' aria-label="' + esc(tip) + '"' + tipAttr(tip) + '>' + partIcon('pen') + '</button>';
  }
  // A list row's edit (ui.ned = the new row's draft fields plus { item, rk, ed, canAsg, orig }): the
  // pencil turns the row into the new row's own markup (nrowChips, newMenu) prefilled from the item.
  // The chips only change the draft; Enter saves it, the x or Esc drops it. Like the new row it has
  // no blur or outside-click rule: it stays open until one of those. Saving sends what changed
  // (editRowEvents): one item.edit of the title, priority, due, and labels; the viewer's own status
  // as item.status (item.close for done), as the item menu does; and the assignees as item.assign
  // and item.unassign, only where the viewer may assign (canAssign: the creator).
  // editHeld: the people really assigned (live or pending), the fallback and rule requests left out.
  function editHeld(it) {
    var w = holders(it).filter(function (r) { return !r.fallback && !r.ruleRequest; }).map(function (r) { return r.to; });
    return w.filter(function (x, i) { return w.indexOf(x) === i; });
  }
  // the assignee chip's prefill: the people assigned; a task nobody else holds shows its creator
  // (the store's fallback holding), an unheld topic nobody (a topic has no fallback)
  function editWho(it) {
    var w = editHeld(it);
    return w.length || it.kind === 'topic' ? w : [it.author];
  }
  function startRowEdit(id, rk) {
    var it = VS.items[id];
    if (!it || !canEdit(it)) return;
    var r = mineLive(it), who = editWho(it), n = ui.ned = { item: id, rk: rk, ed: id, val: it.title, kind: it.kind, st: r ? r.status : it.status, who: who, compl: null,
      start: it.start || '', due: it.due || '', pri: it.priority != null ? it.priority : null, labels: (it.labels || []).slice(), canAsg: canAssign(it, VS.viewer.member) };
    // what the editor opened on: a save sends only the fields the user moved off these (editRowEvents)
    n.orig = { title: n.val, st: n.st, start: n.start, due: n.due, pri: n.pri, who: who.slice(), held: editHeld(it), labels: n.labels.slice() };
    render();
    var el = $('tkned');
    if (el) { el.focus(); el.select(); }
  }
  // The kind icon is plain: the store does not change an item's kind after creation (item.edit
  // has no kind field), so it names the kind rather than offering the new row's toggle. A click
  // anywhere in the row opens no item window (clickItem).
  function editRowHtml(o) {
    var n = ui.ned;
    return '<div class="nrow ned"' + (o.depth ? ' style="--ind:' + o.depth + '"' : '') + attr('item', n.item) + attr('rk', n.rk) + '><span class="nkind"' + tipAttr(kindName(n.kind)) + '>' + SI.svg(n.kind === 'topic' ? 'topic' : 'todo') + '</span>' +
      '<input type="text" id="tkned" data-keep autocomplete="off" value="' + esc(n.val) + '" aria-label="' + esc(wsT("js.tk.form.title")) + '">' +
      nrowChips(n) +
      '<button type="button" class="ia"' + attr('act', 'ned-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.new.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  // The events a save of draft n sends for item it ([] for none): only the fields the user changed
  // since the editor opened (n.orig) that still differ from the item, so a field another viewer
  // wrote meanwhile and the user left alone is kept.
  function editRowEvents(n, it, title) {
    var ev = [], ep = { item: it.id }, topic = it.kind === 'topic', r = mineLive(it), st = nrowSt(n), o = n.orig;
    if (title && title !== o.title.trim() && title !== it.title) ep.title = title;
    if (!topic) {
      var pri = nrowPri(n), due = n.due || null, start = n.start || null;
      if (n.pri !== o.pri && pri !== (it.priority != null ? it.priority : VS.priority.default)) ep.priority = pri;
      if (start !== (o.start || null) && start !== (it.start || null)) ep.start = start;
      if (due !== (o.due || null) && due !== (it.due || null)) ep.due = due;
    }
    // the draft's label picks (what it added and dropped since the prefill) over the item's labels
    // as they are now, so a label another viewer wrote meanwhile is kept; labels the list no longer
    // holds drop, as the edit form's save does
    var picked = labelsOver(it.labels, labelPicks(n.orig.labels, n.labels));
    if (!sameLabels(picked, it.labels)) ep.labels = picked;
    if (Object.keys(ep).length > 1) ev.push(['item.edit', ep]);
    if (st && r && st !== o.st && st !== r.status) ev.push(st === 'done' ? ['item.close', { item: it.id }] : ['item.status', { item: it.id, status: st }]);
    // the assignees as a delta over who is assigned now: item.assign for new picks since open who
    // hold nothing, item.unassign for the ones dropped from the prefill who still hold it. The
    // completion pick rides on the added people only: the store keeps a holding's completion, and
    // re-assigning a kept person to change it would end their record (status, acceptance).
    // A kept fallback prefill needs a real assignment when a new pick would displace the fallback.
    if (n.canAsg && n.who.slice().sort().join(' ') !== o.who.slice().sort().join(' ')) {
      var cur = editHeld(it), add = n.who.filter(function (x) { return o.held.indexOf(x) < 0 && cur.indexOf(x) < 0; });
      if (add.length) ev = ev.concat(asgEvents(it.id, add.map(function (id) { return { t: 'p', id: id }; }), n.compl));
      o.who.forEach(function (x) { if (n.who.indexOf(x) < 0 && cur.indexOf(x) >= 0) ev.push(['item.unassign', { item: it.id, assignee: x }]); });
    }
    return ev;
  }
  function endRowEdit() {
    var n = ui.ned;
    ui.ned = null;
    render();
    var a = n && document.querySelector('.irow[data-item="' + CSS.escape(n.item) + '"]');
    if (a) a.focus({ preventScroll: true });
  }
  function saveRowEdit() {
    var n = ui.ned, it = n && VS.items[n.item], el = $('tkned');
    if (!it) return;
    var ev = editRowEvents(n, it, (el ? el.value : n.val).trim()); // a blank title keeps the old one
    if (!ev.length) { toast(wsT("js.tk.toast.unchanged")); return endRowEdit(); }
    // only this save's draft closes: another row's edit opened while this one was in flight stays
    commit(ev, null, function () { if (ui.ned === n) ui.ned = null; });
  }
  var FLAGGED = {}, UNREAD = {}; // per render: item -> flagged writes in its own thread; item -> unread notification entries
  function warnHtml(it) {
    var f = FLAGGED[it.id] || 0, c = it.concurrent;
    if (!f && !c) return '';
    var tip = wsT("js.tk.row.warn", [f ? wsTn("js.tk.row.flagged", f) : '', c ? wsTn("js.tk.row.concurrent", c) : ''].filter(Boolean).join(', '));
    return '<span class="wicon' + (f ? ' flag' : '') + '"' + tipAttr(tip) + '><svg class="i"><use href="#tki-warn"/></svg></span>';
  }
  function dotHtml(it) {
    var n = UNREAD[it.id];
    return n ? '<span class="udot"' + tipAttr(wsTn("js.tk.row.unread", n)) + '></span>' : '';
  }

  // Row actions: every action the viewer may take on the item, from the view state, as
  // descriptors [act, icon, label, data, go]. The context menu (menuHtml) draws them; each
  // dispatches through the same data-act handlers.
  function dataAttrs(data) {
    var d = '';
    Object.keys(data || {}).forEach(function (k) { d += attr(k, data[k]); });
    return d;
  }
  function ia(act, icon, label, data, cls) {
    return '<button type="button" class="ia' + (cls ? ' ' + cls : '') + '"' + attr('act', act) + dataAttrs(data) + ' aria-label="' + esc(label) + '"' + tipAttr(label) + '><svg class="i"><use href="#tki-' + icon + '"/></svg></button>';
  }
  function actList(it) {
    var out = [];
    if (!it || it.hidden) return out;
    var v = VS.viewer.member, recs = it.assignments.map(A), id = it.id;
    if (recs.some(function (r) { return r.to === v && r.state === 'pending'; })) {
      if (canAccept(it)) out.push(['accept', 'accept', wsT("js.tk.act.accept"), { item: id }, 1]);
      out.push(['note', 'decline', wsT("js.tk.act.decline"), { item: id, op: 'item.decline' }]);
    }
    recs.forEach(function (r) {
      var lead = VS.members[r.to] && VS.members[r.to].leads[0], wr = wrapAuth(it, r);
      if (canReview(r))
        out.push(['confirm', 'confirm', wsT("js.tk.act.confirm", nm(r.to)), { item: id, who: r.to }, 1], ['note', 'reject', wsT("js.tk.act.reject", nm(r.to)), { item: id, who: r.to, op: 'item.reject' }]);
      if (wr) out.push(['wrap', wr === 'sup' ? 'status' : 'x', wsT(wr === 'sup' ? "js.tk.act.wrap" : "js.tk.act.drop", nm(r.to)), { item: id, who: r.to }]);
      if (r.state === 'live' && r.accepted && lead === v) out.push(['note', 'return', wsT("js.tk.act.return", nm(r.to)), { item: id, who: r.to, op: 'item.return' }]);
      // a rule request is withdrawn as its one rule, not as a person (the store refuses that)
      if (r.state === 'pending' && (r.by === v || r.sender === v) && r.to !== v) out.push(['unassign', 'withdraw', wsT("js.tk.act.withdraw", nm(r.to)), r.ruleRequest ? { item: id, root: r.to, rule: r.rule } : { item: id, who: r.to }]);
    });
    if (it.claimable) out.push(['claim', 'accept', wsT("js.tk.act.claim"), { item: id }, 1]);
    reqsOf(it).forEach(function (q) {
      if (canApprove(it, q)) out.push(['approve', 'accept', wsT("js.tk.act.approve", nm(q.by)), { item: id, req: q.id }, 1]);
      if (canAnswer(it, q)) out.push(['note', 'decline', wsT("js.tk.act.deny", nm(q.by)), { item: id, who: q.by, req: q.id, op: 'item.deny' }]);
      if (q.state === 'pending' && q.by === v) out.push(['unrequest', 'withdraw', wsT("js.tk.act.unrequest", nm(q.assignee)), { item: id, req: q.id }]);
    });
    return out;
  }
  // Resolving an in-review record (check.go item.confirm / item.reject): its assigner or any org
  // superior of its assignee, on any completion mode; a self-assigned one is its holder's.
  function canReview(r) {
    var v = VS.viewer.member;
    return r.state === 'live' && r.status === 'in-review' && (r.by === v || !!VS.viewer.reach[r.to]);
  }
  // A status write on another member's assignment (check.go item.status with assignee): 'sup' when
  // the viewer is an org superior of the assignee (blocked, done with no review, or dropped), 'drop'
  // when the viewer is only its assigner (dropped alone), else ''. Only on a live, not wrapped-up
  // task record (a creator-fallback holding included), never a pending one.
  var WRAP_SET = { sup: ['blocked', 'done', 'dropped'], drop: ['dropped'] };
  function wrapAuth(it, r) {
    var v = VS.viewer.member;
    if (!it || it.hidden || it.kind === 'topic' || !r || r.to === v || r.state !== 'live' || wrapped(r.status)) return '';
    return VS.viewer.reach[r.to] ? 'sup' : r.by === v ? 'drop' : '';
  }
  // The status choices on the viewer's own live assignment r (check.go item.status / item.close):
  // [{ s, label, dis, why }]. done is item.close, "request done" when the assigner confirms, and
  // refused while such a close is in review; dropped is the assignee's own only when they close it
  // themselves (completion assignee) or assigned it to themselves.
  var OWN_SET = ['idea', 'todo', 'doing', 'blocked', 'in-review', 'done', 'dropped'];
  function ownStatusOpts(r) {
    var v = VS.viewer.member, gate = r.completion === 'assigner';
    return OWN_SET.map(function (s) {
      var o = { s: s, label: s === 'done' && gate ? wsT("js.tk.menu.reqdone") : stLabel(s), dis: false, why: '' };
      if (s === 'done' && gate && r.status === 'in-review') { o.dis = true; o.why = wsT("js.tk.menu.inreview.why", nm(r.by)); }
      if (s === 'dropped' && gate && r.by !== v) { o.dis = true; o.why = wsT("js.tk.menu.drop.why", nm(r.by)); }
      return o;
    });
  }
  // The viewer's own live assignment on the item, if any.
  function mineLive(it) { var v = VS.viewer.member; return it.assignments.map(A).filter(function (r) { return r.to === v && r.state === 'live'; })[0]; }
  // item.edit, as the store judges it (check.go needCreatorOrAssignee): the creator or a live
  // assignee (an admin too once the creator was removed). Hiding stays the creator's (canHide).
  function canEdit(it) { return (isCreator(it) || !!mineLive(it)) && !it.hidden; }
  function canHide(it) { return isCreator(it) && !it.hidden; }
  function canUnhide(it) { return !!it && !!it.hidden && !!it.canUnhide; }
  // The creator, or an admin once the creator was removed (check.go creatorGone).
  function isCreator(it) {
    var gone = VS.viewer.admin && VS.members[it.author] && VS.members[it.author].removed;
    return it.author === VS.viewer.member || !!gone;
  }
  // Taking work, as the store judges it (check.go): the creator opens a task nobody but the
  // fallback holds (not a topic, not hidden, not already open, the fallback not done) to a scope,
  // and withdraws an open opening; claiming is items[].claimable, computed per viewer. A take
  // request names one live, explicit, not-done assignment of someone else, and is refused while
  // the viewer holds a live or pending record on the item or already asked for that assignment;
  // the named assignment's holder or the creator answers it (approves it only while that
  // assignment is not done or dropped), and its requester withdraws it.
  function fallbackRec(it) { return it.assignments.map(A).filter(function (r) { return r.fallback && r.state === 'live'; })[0] || null; }
  function holdsOn(it, who) { return it.assignments.map(A).some(function (r) { return r.to === who && (r.state === 'live' || r.state === 'pending') && !r.fallback && !r.ruleRequest; }); }
  function canOpen(it) {
    if (!isCreator(it) || it.hidden || it.kind === 'topic' || it.open) return false;
    var fb = fallbackRec(it);
    return !!fb && !wrapped(fb.status) && !it.assignments.map(A).some(function (r) { return (r.state === 'live' || r.state === 'pending') && !r.fallback && !r.ruleRequest; });
  }
  function canUnopen(it) { return !!it.open && isCreator(it); }
  function reqsOf(it) { var all = VS.requests || {}; return listOf(it.requests).map(function (id) { return all[id]; }).filter(Boolean); }
  function reqTargets(it) {
    var v = VS.viewer.member, asked = {};
    if (it.hidden || !VS.viewer.isMember || holdsOn(it, v)) return [];
    reqsOf(it).forEach(function (q) { if (q.by === v && q.state === 'pending') asked[q.assignment] = true; });
    return it.assignments.map(A).filter(function (r) { return r.state === 'live' && !r.fallback && !r.ruleRequest && !wrapped(r.status) && r.to !== v && !asked[r.id]; });
  }
  function canAnswer(it, q) { return q.state === 'pending' && (q.assignee === VS.viewer.member || isCreator(it)); }
  // An approval also needs the named assignment not wrapped up: a done or dropped record stays
  // live, so its requests never lapse, and the store refuses to hand it over (check.go
  // takeWrapped). A denial stays open.
  function canApprove(it, q) { var r = A(q.assignment); return canAnswer(it, q) && !(r && r.state === 'live' && wrapped(r.status)); }
  // Accepting answers every pending record of the viewer on the item; it also needs none of them
  // to be a handoff of a done or dropped assignment: a wrapped-up record stays live, so its handoff
  // never lapses, and the store refuses to pass it on (check.go handoffWrapped). A decline stays open.
  function canAccept(it) {
    var v = VS.viewer.member, mine = it.assignments.map(A).filter(function (r) { return r && r.to === v && r.state === 'pending'; });
    return mine.length > 0 && !mine.some(function (r) { var o = r.handoffOf && A(r.handoffOf); return !!o && o.state === 'live' && wrapped(o.status); });
  }
  // A subtask's parent is assigned to or created by its author.
  function canSplit(it) { return (!!mineLive(it) || it.author === VS.viewer.member) && !it.hidden; }
  function canStatus(it) { var r = !it.hidden && it.kind !== 'topic' && mineLive(it); return !!r && !wrapped(r.status); }
  // item.move, as the store judges it (check.go checkMove): the item rule is the creator or a live
  // assignee (an admin too once the creator was removed); only when the parent changes, the
  // destination rule: not hidden, the viewer's (created or live-assigned; a topic too), and not the
  // item or inside its own subtree. A same-parent reorder and a move to the top level take the item
  // rule alone. moveWhy(id, dest) is '' when the store takes the move, else the reason; dest null is
  // the top level. The dashboard offers no move the store would refuse.
  function canMove(it) {
    var gone = VS.viewer.admin && VS.members[it.author] && VS.members[it.author].removed;
    return !it.hidden && (it.author === VS.viewer.member || !!mineLive(it) || !!gone);
  }
  function moveWhy(id, dest) {
    var it = VS.items[id], d = dest ? VS.items[dest] : null;
    if (!it || it.hidden) return wsT("js.tk.mv.why.hidden", it ? it.alias : alias(id));
    if (!canMove(it)) return wsT("js.tk.mv.why.mine", it.alias);
    if (!dest || dest === (it.parent || null)) return '';
    if (!d || d.hidden) return wsT("js.tk.mv.why.hidden", d ? d.alias : alias(dest));
    if (d.author !== VS.viewer.member && !mineLive(d)) return wsT("js.tk.mv.why.mine", d.alias);
    for (var cur = d, n = 0; cur && n < 1000; cur = cur.parent ? VS.items[cur.parent] : null, n++) if (cur.id === id) return wsT("js.tk.mv.why.sub", it.alias);
    return '';
  }
  // a parent's children in item order (hidden ones included); null: the top level
  function sibsOf(parent) { return parent ? (VS.items[parent] ? VS.items[parent].children : []) : VS.itemOrder.filter(function (id) { return !VS.items[id].parent; }); }
  // What moves of a set of items: the ones with no ancestor in the set (the rest travel with it), in tree order.
  function moveRoots(ids) {
    var set = {}, pos = {};
    ids.forEach(function (id) { if (VS.items[id]) set[id] = true; });
    VS.itemOrder.forEach(function (id, i) { pos[id] = i; });
    return Object.keys(set).filter(function (id) {
      for (var p = VS.items[id].parent, n = 0; p && VS.items[p] && n < 1000; p = VS.items[p].parent, n++) if (set[p]) return false;
      return true;
    }).sort(function (a, b) { return (a in pos ? pos[a] : 1e9) - (b in pos ? pos[b] : 1e9); });
  }
  // Roots placed at slot { parent, after } (after: an id, '' first, undefined last): one item.move per
  // root the slot takes, each anchored after the one before, so they land together in tree order.
  // refused: [[id, reason]] for the others, which stay.
  function moveBatch(roots, slot) {
    var evs = [], refused = [], prev = slot.after;
    roots.forEach(function (id) {
      var why = moveWhy(id, slot.parent || null);
      if (why) { refused.push([id, why]); return; }
      var p = { item: id, parent: slot.parent || null };
      if (prev !== undefined) p.after = prev;
      evs.push(['item.move', p]);
      prev = id;
    });
    return { evs: evs, refused: refused };
  }
  // the move would leave the item where it is
  function moveNoop(id, slot) {
    var it = VS.items[id];
    if (!it || (it.parent || null) !== (slot.parent || null)) return false;
    var s = sibsOf(slot.parent || null), i = s.indexOf(id);
    return slot.after === undefined ? i === s.length - 1 : slot.after === '' ? i === 0 : i > 0 && s[i - 1] === slot.after;
  }
  // The write: the refused roots are named (after the result toast, so they stay readable), the rest move.
  function moveApply(roots, slot) {
    var b = moveBatch(roots, slot), why = b.refused.map(function (x) { return wsT("js.tk.mv.stays", esc(VS.items[x[0]] ? VS.items[x[0]].alias : alias(x[0])), esc(x[1])); }).join('<br>');
    if (!b.evs.length) { if (why) toast(why); return; }
    commit(b.evs, why ? function () { toast(why); } : null);
  }
  // Checkbox selection in the Items list (ui.msel[store] = { ids: { item id: true }, at: the anchor
  // row's key }). A row's checkbox shows on hover; clicking it enters selection mode, which lasts
  // while anything is selected (Esc, or the x of the badge atop the list, clears it). Selection
  // mode is multi-select: a plain row click toggles that row (and makes it the anchor), and
  // Shift-click adds the range from the anchor, keeping the rest; Ctrl/Cmd change nothing. The
  // item window opens from the menu or Enter. Ranges run over the rows shown, in their order, so a
  // folded subtree's rows are not in one. A selection is by item: an item shown twice is selected
  // in both places. Each panel draw keeps only what it drew (MSEEN), so a row a filter, a fold, or
  // a scope change took away leaves the selection.
  var MSEEN = null; // per panel draw: { item id: true } of the rows drawn
  function mselOf() { return ui.msel[ui.store] || null; }
  function mselHas(id) { var s = mselOf(); return !!s && !!s.ids[id]; }
  function mselIds() { var s = mselOf(); return s ? Object.keys(s.ids) : []; }
  function mselOn() { return mselIds().length > 0; }
  function mselClear() { delete ui.msel[ui.store]; }
  function mcbHtml(it, on) {
    return '<button type="button" class="mcb" role="checkbox" aria-checked="' + on + '"' + attr('act', 'msel') + attr('item', it.id) + ' tabindex="-1" aria-label="' + esc(wsT("js.tk.sel.aria", it.title)) + '"' +
      tipAttr(wsT("js.tk.sel.tip")) + '></button>';
  }
  // The pick on selection s ({ ids, at }) over the rows shown (keys: [item id, row key] in order)
  // for a click on row i: Shift with an anchor shown adds the range between them, anything else
  // toggles row i and makes it the anchor.
  function mselPick(s, keys, i, shift) {
    var j = -1;
    if (shift && s.at) keys.some(function (x, k) { if (x[1] === s.at) { j = k; return true; } return false; });
    if (j >= 0 && i >= 0) {
      keys.slice(Math.min(i, j), Math.max(i, j) + 1).forEach(function (x) { s.ids[x[0]] = true; });
      return s;
    }
    var id = keys[i] ? keys[i][0] : null;
    if (!id) return s;
    if (s.ids[id]) delete s.ids[id]; else s.ids[id] = true;
    s.at = keys[i][1];
    return s;
  }
  // A click on a list row in selection mode, or on a row's checkbox (which enters the mode); both
  // act alike (mselPick).
  function mselClick(row, e) {
    var area = row.closest('#card-items .cbody'), rows = area ? Array.prototype.slice.call(area.querySelectorAll('.irow')) : [row];
    var rk = row.getAttribute('data-rk'), s = ui.msel[ui.store] || (ui.msel[ui.store] = { ids: {}, at: null });
    mselPick(s, rows.map(function (x) { return [x.getAttribute('data-item'), x.getAttribute('data-rk')]; }), rows.indexOf(row), e.shiftKey);
    if (!mselOn()) mselClear();
    render();
    // the clicked row keeps focus, so Enter opens its item window
    var back = document.querySelector('#card-items [data-rk="' + CSS.escape(rk) + '"]');
    if (back) back.focus({ preventScroll: true });
  }
  // A click on a list row that no control of it took (a part's picker, the status icon, the "⋯"
  // menu dispatch by data-act first): the title opens the item window (true: the caller goes on to
  // open it), anywhere else only focuses the row, where Enter opens it.
  function rowClick(row, t) {
    if (t.closest('a.title')) return true;
    if (t.closest('input')) return false; // the title's in-place input keeps focus
    row.focus({ preventScroll: true });
    return false;
  }
  // The element whose item window a click no control took opens, or null: none from a form field
  // or the dialog, nor from a row's in-place edit (editRowHtml, .ned), which stays open until saved
  // or dropped, though it carries the row's data-item.
  function clickItem(t) {
    return t.closest('select, textarea, input, #tkdlg, #tkpm, .ned') ? null : t.closest('[data-item]');
  }
  // The "⋯" button every menu-capable row, card, entry, and the item window header carries.
  function moreBtn(id, cls, tab) {
    return '<button type="button" class="ia mbtn' + (cls ? ' ' + cls : '') + '"' + attr('act', 'menu') + attr('item', id) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.menu.more")) + '"' +
      tipAttr(wsT("js.tk.menu.more")) + (tab ? '' : ' tabindex="-1"') + '><svg class="i"><use href="#tki-more"/></svg></button>';
  }
  // A status icon: a button opening the menu's status part when the viewer can set the
  // item's status, else the plain icon.
  // rec: the assignment record a row shows; another member's record keeps the plain icon.
  function stBtn(s, it, rec) {
    if (!it || !canStatus(it) || (rec && rec.to !== VS.viewer.member)) return stIcon(s);
    return '<button type="button" class="sti stb"' + attr('act', 'mstatus') + attr('item', it.id) + ' tabindex="-1" aria-haspopup="menu" aria-label="' + esc(stDesc(s)) + '"' + tipAttr(stDesc(s)) + '>' + SI.svg(s) + '</button>';
  }

  // The detail overlay: what the row leaves out by default.
  function detailsHtml(it, r, extra) {
    var d = [], st = stKey(it, r), ist = stKey(it);
    d.push(kindChip(it) + ' ' + wsT("js.tk.row.by", '<span class="al">' + esc(it.alias) + '</span>', esc(nm(it.author))) + (it.project ? ' &middot; ' + esc(it.project) : ''));
    d.push('<b>' + esc(SI.info(st).label) + '</b> <span class="muted">' + esc(SI.info(st).desc) + '</span>' +
      (r && r.state === 'live' && r.statusOrigin && r.statusOrigin !== 'human' ? ' &middot; ' + wsT("js.tk.row.setby", esc(nm(r.statusBy)), esc(r.statusOrigin)) : '') +
      (r && ist !== st && it.kind !== 'topic' ? ' &middot; ' + wsT("js.tk.row.itemst", esc(SI.info(ist).label)) : '') +
      (r && r.stale ? ' &middot; ' + wsT("js.tk.row.stale", STALE_DAYS) : ''));
    var roll = rollupHtml(it);
    if (roll) d.push(roll);
    if (r) d.push(source(r) + (it.kind === 'topic' ? '' : ' &middot; ' + complText(r.completion)) + (it.chain.length > 1 ? ' &middot; ' + wsT("js.tk.row.chain", esc(it.chain.join(' > '))) : ''));
    else if (it.kind === 'topic') { if (holders(it).length) d.push(wsT("js.tk.row.heldby", holderNames(it))); }
    else d.push(holders(it).length ? '&rarr; ' + holders(it).map(function (x) { return esc(nm(x.to)) + ' <span class="muted">' + esc(SI.info(stKey(it, x)).label) + '</span>'; }).join(', ') : '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>');
    var note = r && (r.note ? [r.note, r.noteBy] : r.handoffNote ? [r.handoffNote, r.sender] : r.answerNote ? [r.answerNote, r.to] : null);
    if (note) d.push('&ldquo;' + esc(note[0]) + '&rdquo; <span class="muted">(' + esc(note[1]) + ')</span>');
    if (extra) d.push(extra);
    var f = FLAGGED[it.id] || 0;
    d.push('<span class="muted">' + wsT("js.tk.row.created", esc(fmtTs(it.ts))) + (it.edited ? ' &middot; ' + wsT("js.tk.row.edited", esc(fmtTs(it.edited.ts))) : '') + ' &middot; ' + wsT("js.tk.row.activity", esc(fmtTs(it.lastActivity))) +
      (r ? ' &middot; ' + wsT("js.tk.row.updated", esc(fmtTs(r.lastUpdate || r.ts))) : '') + (f ? ' &middot; ' + wsT("js.tk.row.nflagged", f) : '') + (it.concurrent ? ' &middot; ' + wsT("js.tk.row.nconcurrent", it.concurrent) : '') + '</span>');
    return d.map(function (x) { return '<div>' + x + '</div>'; }).join('');
  }
  // o: { rk (unique row key in the drawer), item, rec?, more?, depth?, closed? (greyed: done, in the "recently closed" section or kept in the tree),
  //      ctx? (a context row of a tree: an ancestor of a match, not a match itself), tw? (the tree's fold control, in place of the depth arrow), tsub? (the folded "N subtasks" chip) }
  // The row is the one row component every list and tree draws; data-item carries the item id.
  // Only its title opens the item window (rowClick); each part the viewer may change opens its own
  // picker (partHtml, labelEdHtml), the status icon the status menu (stBtn).
  // A row drags (a list drop moves it with its subtree, see mvDrop) when the viewer may move it, or
  // when it is selected: then the selection moves.
  function irow(o) {
    var it = VS.items[o.item], r = o.rec, st = stKey(it, r), ms = mselHas(it.id), ed = canEdit(it) && it.kind !== 'topic';
    if (MSEEN) MSEEN[it.id] = true;
    if (ui.ned && ui.ned.item === it.id && ui.ned.rk === o.rk) return editRowHtml(o); // the row is being edited in place
    return '<div class="irow' + (ui.sel === o.rk ? ' sel' : '') + (ms ? ' msel' : '') + (o.closed ? ' closed' : '') + (o.ctx ? ' ctx' : '') + (it.hidden ? ' tk-hidden' : '') + '" tabindex="0"' + (ms || canMove(it) ? ' draggable="true"' : '') +
      attr('item', it.id) + attr('rk', o.rk) + (r ? attr('rec', r.id) : '') +
      (o.depth ? ' style="--ind:' + o.depth + '"' : '') + '><div class="ir-main">' + mcbHtml(it, ms) + (o.tw != null ? o.tw : o.depth ? '<span class="ind">&#8627;</span>' : '') + stBtn(st, it, r) +
      '<a class="title" href="#" tabindex="-1"' + attr('item', it.id) + '>' + esc(it.title) + '</a>' + titleEdHtml(it) + (o.tsub || '') + (it.hidden ? '<span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '') + openChip(it) + labelChips(it, 'lsearch') + warnHtml(it) + dotHtml(it) + labelEdHtml(it) + '<span class="grow"></span>' +
      '<span class="ir-acts">' + moreBtn(it.id, '', true) + '</span>' + whoHtml(it, canAssign(it, VS.viewer.member)) + priHtml(it, ed) + dueHtml(it, st, ed) + '</div>' +
      '<div class="ir-more">' + detailsHtml(it, r, o.more) + '</div></div>';
  }
  function sub(label, n) { return '<div class="sub">' + label + (n != null ? ' <span class="subn">' + n + '</span>' : '') + '</div>'; }

