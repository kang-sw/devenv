  // ------------------------------------------------------------- view modes

  // The kanban, activity timeline, due timeline, and calendar renderers draw the
  // body of the Items panel for one scope (its own rows; "all" keeps its filters), or of
  // the Activity card. The panel is the wide main zone, so no mode spans rows.
  function modeBody(key, m, v) {
    if (m === 'kanban') return kanbanBody(key, v);
    if (m === 'gantt') return ganttBody(key, dueRows(key, v));
    if (m === 'calendar') return calendarBody(key, dueRows(key, v));
    if (m === 'activity') return activityBody();
    return '';
  }
  // The assignment records a scope shows: mine (live plus pending to me), given (what I
  // assigned or handed to others, live or pending), team (followers' open and unaccepted work).
  // A Today filter keeps only its records. A topic record (its holder) has no status, so no scope shows it here.
  function assignRecs(key, v) {
    var recs;
    if (key === 'mine') recs = VS.my.assignments.concat(VS.my.received.pending).map(A);
    else if (key === 'given') recs = [].concat.apply([], VS.my.given.map(function (id) {
      return VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); });
    }));
    else recs = [].concat.apply([], VS.my.team.map(function (p) { return p.open.concat(p.pending); })).map(A);
    return recs.filter(function (r) { return keepRec(r) && VS.items[r.item].kind !== 'topic'; });
  }

  // Kanban: columns by status. Assignment scopes: cards are assignments (with the
  // pending column); "all": cards are items by item status, whose idea column's header "+"
  // opens an input row atop it (knewRowHtml). Dragging moves only the viewer's own
  // live assignment (item.status, or item.close on the done column).
  var KCOLS = { assign: ['pending', 'idea', 'todo', 'doing', 'blocked', 'in-review', 'done', 'dropped'],
    items: ['pending', 'idea', 'todo', 'doing', 'blocked', 'in-review', 'done', 'dropped'] };
  function kanbanBody(key, v) {
    var cols = KCOLS[key === 'all' ? 'items' : 'assign'], buckets = {};
    cols.forEach(function (c) { buckets[c] = []; });
    if (key === 'all') {
      var sh = showHidden();
      VS.itemOrder.forEach(function (id) {
        var it = VS.items[id];
        // the columns already are the statuses, so the status filter would only empty them: kind filter only
        if ((!sh && it.hidden) || it.kind === 'topic' || !buckets[it.status] || !kindFilter(it) || !keepItem(id)) return;
        var mineR = it.assignments.map(A).filter(function (r) { return r.to === v && r.state === 'live'; })[0];
        buckets[it.status].push({ item: id, drag: !it.hidden && mineR && !wrapped(mineR.status) ? id : null,
          sub: (it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return r.to + (r.state === 'pending' ? '?' : ''); }).join(', ') || wsT("js.tk.common.nobody")) });
      });
    } else {
      assignRecs(key, v).forEach(function (r) {
        var col = r.state === 'pending' ? 'pending' : r.status;
        if (!buckets[col]) return;
        buckets[col].push({ item: r.item, rec: r, drag: r.to === v && r.state === 'live' && !wrapped(r.status) ? r.item : null,
          who: key !== 'mine' ? r.to : null, sub: (key !== 'mine' ? esc(nm(r.to)) + ' &middot; ' : '') + source(r) });
      });
    }
    Object.keys(buckets).forEach(function (c) {
      buckets[c].sort(function (x, y) { var a = VS.items[x.item], b = VS.items[y.item]; return b.priorityLevel - a.priorityLevel || ((a.due || '9') < (b.due || '9') ? -1 : 1); });
    });
    var h = '<div class="toolbar">' + (key === 'all' ? itemFilterBar(true) : '') +
      '<span class="muted">' + wsT("js.tk.kanban.hint") + '</span></div><div class="kanban">';
    h += cols.map(function (c) {
      var rv = key === 'all' && c === 'idea', kn = rv && ui.knew[ui.store];
      return '<div class="kcol"' + attr('kcol', c) + '><div class="khead">' + stIcon(c) + '<b>' + esc(SI.info(c).label) + '</b> <span class="muted">' + buckets[c].length + '</span>' +
        (rv ? '<span class="grow"></span>' + ia('knew', 'plus', wsT("js.tk.kanban.new"), {}) : '') + '</div>' + (kn ? knewRowHtml(kn) : '') +
        buckets[c].map(function (x) {
          // quiet cards: title, then initials (the record's assignee off "mine"), priority, due; the source on hover
          var it = VS.items[x.item];
          return '<div class="kcard' + (x.drag ? ' drag' : '') + (it.hidden ? ' tk-hidden' : '') + '"' + (x.drag ? ' draggable="true"' + attr('drag', x.drag) : '') + attr('item', x.item) + attr('rk', 'k:' + c + ':' + (x.rec ? x.rec.id : x.item)) + tipHtml(x.sub) + '>' +
            '<div class="kc-t">' + titleRef(x.item) + (it.hidden ? ' <span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '') + '</div>' + moreBtn(x.item, 'kmore', true) + '<div class="kc-m">' + warnHtml(it) + dotHtml(it) + (x.who ? '<span class="who">' + avHtml(x.who) + '</span>' : whoHtml(it)) +
            '<span class="grow"></span>' + priHtml(it) + dueHtml(it, x.rec ? stKey(it, x.rec) : c) + '</div></div>';
        }).join('') + '</div>';
    }).join('');
    return h + '</div>' + lastWrite();
  }

  var FAMILIES = [['all', "js.tk.family.all"], ['item', "js.tk.family.item"], ['assign', "js.tk.family.assign"], ['comment', "js.tk.family.comment"], ['roles', "js.tk.family.roles"]];
  function family(t) {
    if (/^(roles|member)\./.test(t)) return 'roles';
    if (/^comment\./.test(t)) return 'comment';
    if (/^item\.(assign|accept|decline|return|handoff|unassign)$/.test(t)) return 'assign';
    return 'item';
  }
  function historyLoading() {
    var error = AD.historyError && AD.historyError(ui.store);
    return '<p role="status">' + esc(error || wsT('js.tk.loading')) + '</p>' + (error && AD.retryHistory ? btn('history-retry', wsT('js.preview.reload'), {}) : '');
  }
  function activityBody() {
    if (AD.history && !AD.history(ui.store)) return historyLoading();
    var f = ui.act, rows = (VS.historyEvents || VS.events).filter(function (n) {
      return (f.family === 'all' || family(n.type) === f.family) && (f.member === 'all' || n.member === f.member) &&
        (!f.flagged || n.status !== 'applied') && (!f.conc || n.markers.length);
    }).slice().sort(function (a, b) { return String(b.ts) < String(a.ts) ? -1 : String(b.ts) > String(a.ts) ? 1 : 0; });
    var people = [['all', wsT("js.tk.activity.everyone")]].concat(Object.keys(VS.members).map(function (id) { return [id, nmo(id)]; }));
    var h = '<div class="toolbar">' + sel('a-family', f.family, FAMILIES.map(function (x) { return [x[0], wsT(x[1])]; })) + sel('a-member', f.member, people) +
      actChips() + '<span class="grow"></span><span class="muted">' + wsT("js.tk.activity.count", rows.length) + '</span></div>';
    var day = null;
    h += '<ul class="plain atl">' + rows.map(function (n) {
      var d = String(n.ts).slice(0, 10), x = '';
      if (d !== day) { day = d; x = '<li class="day">' + esc(d) + '</li>'; }
      return x + evLine(n, false);
    }).join('') + '</ul>';
    return lastWrite() + h;
  }

  function dayNum(d) { return Math.floor(Date.parse(String(d).slice(0, 10) + 'T00:00:00Z') / 86400000); }
  function dayStr(n) { return new Date(n * 86400000).toISOString().slice(0, 10); }
  // The dated rows a due timeline or calendar draws for a scope: one per item on
  // "all" (its filters apply), one per assignment record on an assignment scope,
  // with the record's own state and, off "mine", the assignee. Render keys must
  // include that record's identity: several siblings can represent one item.
  function dueRows(key, v) {
    var sh = showHidden();
    function dated(it) { return it && it.due && (!it.hidden || sh) && it.kind !== 'topic'; }
    if (key === 'all') return VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(function (it) { return dated(it) && itemFilter(it) && keepItem(it.id); })
      .map(function (it) { return { item: it.id, status: it.status, rec: null, who: '' }; });
    return assignRecs(key, v).filter(function (r) { return dated(VS.items[r.item]); }).map(function (r) {
      return { item: r.item, status: stKey(null, r), rec: r, who: key === 'mine' ? '' : nm(r.to) };
    });
  }
  function whoTag(x) { return x.who ? '<span class="muted">' + esc(x.who) + '</span>' : ''; }
  // [first day, day after the last] of an item's own gantt bar, or null without a due date: from
  // its start date, else its first doing, else its creation, to its due day. A start after the due
  // (concurrent edits can fold one) gives a one-day bar at the due.
  function ganttSpan(it) {
    if (!it.due || it.kind === 'topic') return null;
    var e = dayNum(it.due) + 1;
    return [Math.min(dayNum(it.start || it.started || it.ts), e - 1), e];
  }
  // Due-date timeline: the scope's dated rows as the same context tree the list draws (the
  // fold state is shared with it). A row's bar runs from the item's start (ganttSpan) to its due day. A node with children also draws a thin summary bar spanning the
  // range of every dated item below it, whether they show or are folded away; a parent without
  // a due date of its own (a context row, say) is placed by that summary bar alone.
  function ganttBody(key, rows) {
    var bar = key === 'all' ? itemFilterBar() : '';
    if (!rows.length) return '<div class="toolbar">' + bar + '</div><div class="empty">' + wsT("js.tk.gantt.empty") + '</div>';
    var ents = ctxTree(rows, key === 'all' ? { keep: keepKind } : null), below = {};
    // [first day, day after the last] of an item's own bar, or null without a due date
    function own(id) { return ganttSpan(VS.items[id]); }
    function join(a, b) { return !a ? b : !b ? a : [Math.min(a[0], b[0]), Math.max(a[1], b[1])]; }
    function sub(id) { // the range of every dated item below id
      if (!(id in below)) below[id] = kidsShown(id).reduce(function (r, k) { return join(join(r, own(k)), sub(k)); }, null);
      return below[id];
    }
    var t0 = dayNum(today()), lo = t0, hi = t0 + 1;
    ents.forEach(function (e) { var r = e.k === 'node' && join(own(e.id), sub(e.id)); if (r) { lo = Math.min(lo, r[0]); hi = Math.max(hi, r[1]); } });
    var span = hi - lo, pct = function (d) { return ((d - lo) / span * 100).toFixed(2) + '%'; }, wid = function (r) { return (Math.max(1, r[1] - r[0]) / span * 100).toFixed(2) + '%'; };
    var axis = '';
    for (var d = lo; d < hi; d++) axis += '<span class="gtick' + (d === t0 ? ' now' : '') + '" style="left:' + pct(d) + '">' + (new Date(d * 86400000).getUTCDay() === 1 || d === lo ? esc(dayStr(d).slice(5)) : '') + '</span>';
    var h = '<div class="toolbar">' + bar + '<span class="muted">' + wsT("js.tk.gantt.hint", rows.length, esc(today())) + '</span></div>' +
      '<div class="gantt"><div class="grow-axis"><div class="glabel"></div><div class="gtrack axis">' + axis + '</div></div>';
    // x: the dueRows entry of a match, null for a context row; c: the fold control; sm: draw the summary bar
    function row(e, x, c, sm) {
      var it = VS.items[e.id], st = x ? x.status : stKey(it), r = own(e.id), s = sm && e.n ? sub(e.id) : null, over = it.due && it.due < today() && !wrapped(st);
      return '<div class="grow-row click' + (ENDED[st] ? ' ended' : '') + (x ? '' : ' ctx') + (it.hidden ? ' tk-hidden' : '') + '"' + attr('item', it.id) + attr('rk', 'g:' + it.id + (x && x.rec ? ':a:' + x.rec.id : '')) + '><div class="glabel" style="--ind:' + e.depth + '">' + c.tw + stBtn(st, it, x && x.rec) + titleRef(it.id) + (it.hidden ? '<span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '') + c.sub + (x ? whoTag(x) : '') + priHtml(it) + moreBtn(it.id, 'gmore', true) + '</div>' +
        '<div class="gtrack"><span class="gnow" style="left:' + pct(t0) + '"></span>' +
        (r ? '<span class="gbar st-' + esc(st) + (over ? ' over' : '') + '" style="left:' + pct(r[0]) + ';width:' + wid(r) + '" title="' +
          esc(it.start ? wsT("js.tk.gantt.bar.start", it.start, it.due) : wsT(it.started ? "js.tk.gantt.bar.started" : "js.tk.gantt.bar.created", String(it.started || it.ts).slice(0, 10), it.due)) + '">' + esc(it.due.slice(5)) + '</span>' : '') +
        (s ? '<span class="gsum" style="left:' + pct(s[0]) + ';width:' + wid(s) + '" title="' + esc(wsT("js.tk.gantt.sum", dayStr(s[0]), dayStr(s[1] - 1))) + '"></span>' : '') + '</div></div>';
    }
    h += ents.map(function (e) {
      if (e.k === 'more') return '<div class="grow-row"><div class="glabel" style="--ind:' + e.depth + '">' + moreRow({ id: e.id, n: e.n, depth: 0 }, 'g:') + '</div><div class="gtrack"></div></div>';
      var c = foldCtl(e, 'tfold', 'g:');
      if (!e.rows.length) return row(e, null, c, true);
      return e.rows.map(function (x, i) { return row(e, x, i ? { tw: '<span class="tw"></span>', sub: '' } : c, !i); }).join('');
    }).join('');
    return h + '</div>';
  }

  // the weekday heads, Sunday first: the calendar view and the range picker share them
  function calDow() {
    return [wsT("js.tk.cal.sun"), wsT("js.tk.cal.mon"), wsT("js.tk.cal.tue"), wsT("js.tk.cal.wed"), wsT("js.tk.cal.thu"), wsT("js.tk.cal.fri"), wsT("js.tk.cal.sat")];
  }
  function calendarBody(key, rows) {
    var m = ui.calMonth[key] || today().slice(0, 7), first = dayNum(m + '-01'), dow = new Date(first * 86400000).getUTCDay();
    var y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7)), days = new Date(Date.UTC(y, mo, 0)).getUTCDate();
    var byDay = {};
    rows.forEach(function (x) { var due = VS.items[x.item].due; (byDay[due] = byDay[due] || []).push(x); });
    var prev = new Date(Date.UTC(y, mo - 2, 1)).toISOString().slice(0, 7), next = new Date(Date.UTC(y, mo, 1)).toISOString().slice(0, 7);
    var n = Object.keys(byDay).filter(function (k) { return k.slice(0, 7) === m; }).reduce(function (s, k) { return s + byDay[k].length; }, 0);
    var h = '<div class="toolbar">' + btn('cal', '&lsaquo;', { month: prev, card: key }) + '<b>' + esc(m) + '</b>' + btn('cal', '&rsaquo;', { month: next, card: key }) +
      btn('cal', wsT("js.tk.cal.today"), { month: today().slice(0, 7), card: key }) + (key === 'all' ? itemFilterBar() : '') + '<span class="muted">' + wsT("js.tk.cal.count", n, esc(m)) + '</span></div>' +
      '<div class="cal">' + calDow().map(function (d) { return '<div class="cdow">' + d + '</div>'; }).join('');
    for (var i = 0; i < dow; i++) h += '<div class="cday off"></div>';
    for (var d = 1; d <= days; d++) {
      var ds = m + '-' + (d < 10 ? '0' : '') + d, list = byDay[ds] || [];
      h += '<div class="cday' + (ds === today() ? ' now' : '') + '"' + attr('day', ds) + '><div class="cnum">' + d + '</div>' + list.map(function (x) {
        var xit = VS.items[x.item];
        return '<div class="citem click' + (ENDED[x.status] ? ' ended' : '') + (xit && xit.hidden ? ' tk-hidden' : '') + '"' + attr('item', x.item) + attr('rk', 'c:' + ds + ':' + x.item + (x.rec ? ':a:' + x.rec.id : '')) + tipHtml(SI.info(x.status).label + ' · ' + xit.priorityName + (x.who ? ' · ' + esc(x.who) : '')) + '>' +
          stBtn(x.status, xit, x.rec) + titleRef(x.item) + moreBtn(x.item, 'cmore', true) + '</div>';
      }).join('') + '</div>';
    }
    return h + '</div>';
  }

  // ------------------------------------------------------- item reference card

  // A tracker card reference: an item rendered from the view state in a few lines,
  // opening its item window (a chat entry can carry one).
  function crefHtml(id) {
    var it = VS.items[id];
    var who = it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return nm(r.to) + (r.state === 'pending' ? '?' : ''); });
    return '<div class="cref click"' + attr('item', id) + attr('cref', id) + ' title="' + wsT("js.tk.cref.tip") + '"><div class="r1">' + stChip(it.status) + prChip(it) + dueChip(it) +
      '<span class="grow"></span><span class="al">' + esc(it.alias) + '</span></div><div class="ct">' + esc(it.title) + '</div>' +
      '<div class="r2">&rarr; ' + esc(who.join(', ') || wsT("js.tk.common.nobody")) + '</div></div>';
  }

  // --------------------------------------------------------------- sync state

  // The store's sync state is one slim ribbon at the bottom of the Items card: a status dot,
  // a short text, and the apply button only while writes are pending; with nothing to do it
  // dims to a quiet line. A click opens the status popover (syncPopHtml) with the diagnostics.
  function unpushedN(vs) { return vs.sync.unpushed.reduce(function (a, c) { return a + c.events.length; }, 0); }
  // "synced N min ago" against the view state's clock
  function syncedAgo(ts) {
    if (!ts) return wsT("js.tk.rib.never");
    var m = Math.floor((Date.parse(VS.now) - Date.parse(ts)) / 60000);
    if (!(m >= 1)) return wsT("js.tk.rib.now");
    return m < 60 ? wsTn("js.tk.rib.min", m) : m < 1440 ? wsTn("js.tk.rib.hour", Math.floor(m / 60)) : wsTn("js.tk.rib.day", Math.floor(m / 1440));
  }
  // A wiki sync failure (a wiki fetch or push the ledger's sync went on without) leaves the
  // store online; it marks the line and keeps it from dimming while it stands.
  function wikiWarns(vs) { return vs.store.remote && vs.sync.wikiWarnings ? vs.sync.wikiWarnings : []; }
  function ribbonHtml() {
    var s = VS.sync, remote = VS.store.remote, n = unpushedN(VS), ww = wikiWarns(VS), tone, txt;
    if (!remote) { tone = n ? 'pend' : 'local'; txt = wsT("js.tk.rib.local") + (n ? ' &middot; ' + wsTn("js.tk.rib.pending", n) : ''); }
    else if (!s.online) { tone = 'off'; txt = wsT("js.tk.sync.offline") + (n ? ' &middot; ' + wsTn("js.tk.rib.topush", n) : ''); }
    else if (n) { tone = 'pend'; txt = wsTn("js.tk.rib.topush", n); }
    else { tone = 'ok'; txt = esc(syncedAgo(s.syncedAt)); }
    if (ww.length && s.online) { tone = 'pend'; txt += ' &middot; ' + wsT("js.tk.sync.wikiwarn"); }
    var open = !!(MN && MN.kind === 'sync');
    return '<div class="ribbon r-' + tone + (n || ww.length ? '' : ' idle') + '"><button type="button" class="rib-main"' + attr('act', 'rib') + attr('fk', 'rib') + ' aria-haspopup="dialog" aria-expanded="' + open + '"' +
      tipAttr(wsT("js.tk.rib.tip") + (!s.online && remote && s.error ? ' (' + s.error + ')' : '') + (ww.length ? ' (' + ww.join('; ') + ')' : '')) + '><span class="sdot"></span><span class="rt">' + txt + '</span></button>' +
      (n ? btn('sync', remote ? wsT("js.tk.sync.now") : wsT("js.tk.sync.closebatch"), {}, 'primary') : '') + '</div>';
  }
  // The status popover: the clone ref, the remote state, the store's counts, the unpushed
  // batch contents, the host's syncTools, and the apply button.
  function syncPopHtml() {
    if (AD.history) AD.history(ui.store);
    var vs = VS, s = vs.sync, remote = vs.store.remote, events = unpushedN(vs), tools = X.syncTools ? X.syncTools(ui.store, vs, H) || '' : '';
    var nMem = Object.keys(vs.members).filter(function (id) { return !vs.members[id].removed; }).length;
    var h = '<div class="mh"><b>' + esc(vs.store.label) + '</b></div><div class="spop">';
    h += '<div>' + wsT("js.tk.sync.clone", '<code>' + esc(refOf(vs.viewer.stream)) + '</code>') + '</div>';
    h += '<div>' + (remote ? '<b' + (!s.online && s.error ? tipAttr(s.error) : '') + '>' + (s.online ? wsT("js.tk.sync.online") : wsT("js.tk.sync.offline")) + '</b> &middot; ' +
      wsT("js.tk.sync.synced", esc(fmtTs(s.syncedAt))) + (s.online ? '' : ' &middot; ' + wsT("js.tk.sync.retry")) : wsT("js.tk.sync.noremote")) + '</div>';
    if (!s.online && remote && s.error) h += '<div class="err">' + esc(s.error) + '</div>';
    wikiWarns(vs).forEach(function (w) { h += '<div class="err">' + esc(w) + '</div>'; });
    h += '<div class="muted">' + wsT("js.tk.drawer.stats", nMem, vs.stats.events, vs.stats.commits, vs.stats.flagged) + '</div>';
    h += '<div' + (events ? ' class="unp"' : '') + '>' + wsT("js.tk.sync.unpushed", events) + '</div>';
    if (remote && s.upstream) h += '<div class="muted"' + tipAttr(wsT("js.tk.sync.upstream.tip")) + '>' + wsT("js.tk.sync.upstream", s.upstream) + '</div>';
    if (s.unpushed.length) {
      h += '<div class="sbatch"><b>' + wsT("js.tk.sync.list", s.unpushed.map(function (c) { return c.open ? wsT("js.tk.sync.openn", c.events.length) : wsT("js.tk.sync.commitn", c.events.length); }).join(', ')) + '</b><ul class="plain">' +
        s.unpushed.map(function (c) {
          return '<li><b>' + (c.open ? wsT("js.tk.flag.open") : wsT("js.tk.sync.closed")) + '</b> <span class="muted">' + esc(fmtTs(c.ts)) + '</span>: ' + c.events.map(function (id) {
            var n = EV[id] || (VS.historyEvents || []).filter(function (e) { return e.id === id; })[0]; return esc(alias(id)) + (n ? ' ' + esc(n.type) : '');
          }).join(', ') + '</li>';
        }).join('') + '</ul><p class="muted">' + wsT("js.tk.sync.batchnote") + '</p></div>';
    }
    return h + '</div><div class="mtools">' + btn('sync', remote ? wsT("js.tk.sync.now") : wsT("js.tk.sync.closebatch"), {}, events ? 'primary' : '') + tools + '</div>';
  }

  // The drawer layout, switched by the window width (layoutOf):
  //   'a'      the Items panel (about two thirds) and one tab pane on the right holding
  //            Notifications, Support agent, Team, Activity, Data, one tab at a time;
  //   'd'      from about 1600px, three columns: Items | Notifications (always open) |
  //            the tab pane holding the remaining tabs;
  //   'narrow' below about 1000px, everything is one tab pane, Items first, with the
  //            Today chips at the top whatever tab is shown.
  // Only the shown tab is rendered. A hidden tab whose signature (tabSig) moved since it
  // was last shown carries a dot, like the dashboard's tab bar (`tdot`).
  // A tab is one icon button: the name is the tooltip and the aria-label, a dot rides its corner.
  // The host's extra tabs (ext.tabs) join the side tabs at mount (mergeTabs).
  // Built-in tab names are table keys (looked up in tabName); a host tab brings its own text.
  var TABS = { items: "js.tk.tab.items", wiki: "js.tk.wk.tab", notify: "js.tk.tab.notify", org: "js.tk.tab.org", activity: "js.tk.tab.activity" };
  var TAB_TIP = {};
  var TAB_ICON = { items: '#tki-tasks', wiki: '#tki-book', notify: '#tki-bell', org: '#tki-team', activity: '#tki-activity' };
  var SIDE = ['notify', 'org', 'activity'], XTAB = {};
  function mergeTabs() {
    (X.tabs || []).forEach(function (t) {
      XTAB[t.key] = t; TABS[t.key] = t.title; TAB_ICON[t.key] = t.icon;
      if (t.tip) TAB_TIP[t.key] = t.tip;
      var at = t.after ? SIDE.indexOf(t.after) : -1;
      if (at < 0) SIDE.push(t.key); else SIDE.splice(at + 1, 0, t.key);
    });
  }
  function tabName(k) { return XTAB[k] ? TABS[k] : wsT(TABS[k]); }
  function layoutOf() { var w = window.innerWidth; return w >= 1600 ? 'd' : w < 1000 ? 'narrow' : 'a'; }
  function tabState() { return (ui.tab[ui.store] = ui.tab[ui.store] || { side: 'notify', narrow: 'items' }); }
  function tabSig(k, v) {
    if (k === 'items') return VS.my.badge.join(',');
    if (k === 'notify') { var rm = readMap(); return feedEntries(v).filter(function (x) { return !rm[x.key]; }).map(function (x) { return x.key; }).join(','); }
    if (k === 'activity') return String(VS.stats.events);
    if (k === 'org') return JSON.stringify([VS.members, VS.orgs, VS.labels]);
    return '';
  }
  function tabSeen() { return (ui.tabSeen[seenKey(ui.store)] = ui.tabSeen[seenKey(ui.store)] || {}); }
  // the store's content is on screen: its drawer is open, or it is the store page
  function viewing() { return ui.page ? ui.page === ui.store : ui.open === ui.store; }
  function tabShown(k, v) { if (viewing()) tabSeen()[k] = tabSig(k, v); }
  // Unread notifications count as new on first sight; every other tab starts as seen.
  // Only an open drawer shows or records anything (a closed one is not being looked at).
  function tabDot(k, v) {
    if (!viewing()) return false;
    var seen = tabSeen(), sig = tabSig(k, v);
    if (!(k in seen)) seen[k] = k === 'notify' ? '' : sig;
    // notifications: only an unread entry not there when last shown (reading or answering one is not news)
    if (k === 'notify') { var was = seen[k].split(','); return sig.split(',').some(function (x) { return x && was.indexOf(x) < 0; }); }
    return !!sig && sig !== seen[k];
  }
  function paneHtml(tabs, active, v, specs, narrow) {
    var c = specs[active];
    var nav = '<nav class="ptabs" role="tablist" aria-label="' + wsT("js.tk.tab.aria") + '">' + tabs.map(function (k) {
      var on = k === active, dot = !on && tabDot(k, v);
      return '<button type="button" class="ptab" role="tab" aria-selected="' + on + '" aria-label="' + esc(dot ? wsT("js.tk.tab.new", tabName(k)) : tabName(k)) + '"' + attr('act', 'tab') + attr('tab', k) + tipAttr(TAB_TIP[k] || tabName(k)) + '>' +
        '<svg class="i"><use href="' + esc(TAB_ICON[k]) + '"/></svg><span class="tdot' + (dot ? ' on' : '') + '" aria-hidden="true"></span></button>';
    }).join('') + '</nav>';
    var tools = c.tools != null ? c.tools : (c.chip ? '<span class="chip">' + c.chip + '</span>' : '') + '<span class="grow"></span>' + c.extra;
    tabShown(active, v);
    return '<section class="card pane' + (c.cls ? ' ' + c.cls : '') + '" id="' + c.id + '"' + attr('tab', active) + (c.attrs || '') + (c.mode ? attr('view', c.mode) : '') + '><header>' + nav + '</header>' +
      (narrow && active === 'items' ? todayBar() : '') + '<div class="ptools">' + tools + '</div><div class="cbody' + (c.bodyCls ? ' ' + c.bodyCls : '') + '">' + c.body + '</div>' + (active === 'items' || active === 'wiki' ? ribbonHtml() : '') + '</section>';
  }
  // storeTitle is a store's display label with its id, "Label (id)", or
  // the id alone when the store has no label (StoreInfo.label then
  // repeats the name); storeTitleHtml mutes the id.
  function storeTitle(st) { return st.label && st.label !== st.name ? st.label + ' (' + st.name + ')' : st.name; }
  function storeTitleHtml(st) { return st.label && st.label !== st.name ? '<b>' + esc(st.label) + '</b> <span class="muted">(' + esc(st.name) + ')</span>' : '<b>' + esc(st.name) + '</b>'; }
  function drawerHtml(name, v) {
    var vs = VS;
    // the Items panel holds the items or, under the header's Items | Wiki switch, the wiki
    var MAKE = { items: function (v) { return AD.wiki && wk().mode === 'wiki' ? cardWiki(v, false) : cardPanel(v); }, wiki: function (v) { return cardWiki(v, true); }, notify: cardNotify, org: cardTeam, activity: cardActivity };
    var h = '<div class="tk-title">' + storeTitleHtml(vs.store) + storeMenuBtn(name) + '<span class="grow"></span>' +
      // the drawer links to the store page; the page links back to the session view
      (ui.page ? '<button class="btn" data-act="page-back"' + tipAttr(wsT("js.tk.drawer.back.tip")) + '><svg class="i"><use href="#tki-back"/></svg>' + wsT("js.tk.drawer.back") + '</button>' :
        '<a class="btn ib" href="#/' + esc(encodeURIComponent(name)) + '" aria-label="' + wsT("js.tk.drawer.page") + '"' + tipAttr(wsT("js.tk.drawer.page.tip", name)) + '><svg class="i"><use href="#tki-page"/></svg></a>') + '</div>';
    if (!vs.viewer.isMember) h += '<p class="err">' + wsT("js.tk.drawer.notmember", esc(v)) + '</p>';
    var L = ui.layout = layoutOf(), ts = tabState(), specs = {};
    function make(k) { return (specs[k] = specs[k] || (MAKE[k] ? MAKE[k](v) : XTAB[k].make(name, VS, H))); }
    if (L === 'narrow') {
      var all = ['items'].concat(AD.wiki ? ['wiki'] : []).concat(SIDE), act = all.indexOf(ts.narrow) >= 0 ? ts.narrow : 'items';
      make(act);
      return h + '<div class="tk-zones" data-layout="narrow"><div class="tk-col">' + paneHtml(all, act, v, specs, true) + '</div></div>';
    }
    var tabs = L === 'd' ? SIDE.slice(1) : SIDE, active = tabs.indexOf(ts.side) >= 0 ? ts.side : tabs[0];
    make(active);
    h += '<div class="tk-zones" data-layout="' + L + '"><div class="tk-col tk-main">' + panelHtml(make('items'), true) + '</div>';
    if (L === 'd') { h += '<div class="tk-col">' + cardHtml(make('notify')) + '</div>'; tabShown('notify', v); }
    tabShown('items', v);
    return h + '<div class="tk-col">' + paneHtml(tabs, active, v, specs, false) + '</div></div>';
  }

  // ------------------------------------------------------------ item window

  function commentHtml(n, v, it) {
    var p = n.payload, c = VS.comments[n.id] || {}, body = c.body != null ? c.body : p.body, reply = '', sh = showHidden(), hidShown = !!(c.hidden && sh);
    if (p.reply_to) {
      var q = VS.comments[p.reply_to] || (EV[p.reply_to] && { author: EV[p.reply_to].member, body: EV[p.reply_to].payload.body });
      reply = '<div class="quote"' + tipAttr(alias(p.reply_to)) + '>&#8617; ' + (q ? '<b>' + whoName(q.author) + '</b>: ' + (q.hidden && !sh ? '<i>' + wsT("js.tk.cmt.hiddenq") + '</i>' : esc(String(q.body).slice(0, 90))) : '') + '</div>';
    }
    var acts = '';
    if (n.status === 'applied' && !c.hidden && !it.hidden) {
      acts += btn('reply', wsT("js.tk.cmt.reply"), { cid: n.id });
      if (n.member === v) acts += btn('cedit', wsT("js.tk.cmt.edit"), { cid: n.id }) + btn('chide', wsT("js.tk.cmt.hide"), { cid: n.id });
    } else if (hidShown && n.status === 'applied' && c.canUnhide) {
      acts += btn('cunhide', wsT("js.tk.cmt.unhide"), { cid: n.id }, 'tk-unhide');
    }
    var live = n.status === 'applied' && !!VS.comments[n.id], ed = ui.cedit && ui.cedit.cid === n.id && live && !c.hidden;
    if (ed) acts = '';
    // an own comment being edited in place (ui.cedit): Cmd/Ctrl+Enter saves, Esc cancels
    var txt = ed ? '<div class="cedit">' + editorHtml('tkcedit', { value: ui.cedit.body, rows: 3, label: wsT("js.tk.cmt.edit"), pv: 'ctext' }) + '<div class="tb">' +
      btn('cedit-save', wsT("js.tk.dlg.save"), {}, 'primary') + btn('cedit-x', wsT("js.tk.dlg.cancel"), {}) + '<span class="muted">' + wsT("js.tk.cedit.hint", MAC ? '⌘Enter' : 'Ctrl+Enter') + '</span></div></div>' + mentionChips(c.mentions) + attChips(c.attachments, true) : (c.hidden && !sh) ? '<div class="hiddenc">' + wsT("js.tk.cmt.hidden", esc(alias(c.hidden))) + '<details><summary>' + wsT("js.tk.cmt.history") + '</summary>' + esc(body) + '</details></div>' :
      textBlock('ctext', 'comments', n.id, body) + mentionChips(live ? c.mentions : p.mentions) + attChips(live ? c.attachments : p.attachments, live);
    return '<li class="cmt' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (n.member === v ? ' mine' : '') + (hidShown ? ' tk-hidden' : '') + '"' + attr('ev', n.id) + tipAttr(n.alias) + '>' +
      '<div class="ch"><b>' + whoName(n.member) + '</b>' + originChip(n.origin) + '<span class="when">' + esc(fmtTs(n.ts)) + '</span>' +
      (hidShown ? '<span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '') +
      (c.edited ? '<span class="chip">' + wsT("js.tk.cmt.edited") + '</span>' : '') + flags(n) + '<span class="grow"></span>' + acts + '</div>' + reply + txt +
      (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(n) + supHtml(n) + '</li>';
  }

  // The item window: what the thread sheet and the row detail overlay held, in a
  // centred window over the page. Its actions are the header's context menu (the "⋯"
  // button, a right-click on the header, the status icon); no button row stands between
  // the header and the body. The assignees table keeps its per-assignee actions.
  // What an event refers to inside the open item window, found by the event's type:
  // a comment (added, edited, or hidden) -> that comment; a status, close, confirm, reject,
  // or reopen -> that assignee's row in Assignees; an assignment or handoff -> the holders
  // line of the property card; an accept -> the accepter's row; a decline, return, or unassign ->
  // the assignees row that ended (the holders line no longer lists it), or that member's row when
  // the list hid the ended record (visibleRecs); a CC add or removal -> that member's CC row, else
  // the CC section; a body edit or the
  // item's creation -> the body; an edit that only adds attachments -> the first new chip.
  // Anything else falls back to its own line in the thread, and null when the window has none.
  function focusTarget(root, evId) {
    var e = EV[evId];
    if (!e) return null;
    var p = e.payload || {}, by = function (sel, id) { return root.querySelector('[' + sel + '="' + CSS.escape(id) + '"]'); }, holders = root.querySelector('[data-focus="holders"]'), el = null;
    if (e.type === 'comment.add') el = by('data-ev', e.id);
    else if (e.type === 'comment.edit' || e.type === 'comment.hide' || e.type === 'comment.unhide') el = by('data-ev', p.comment);
    else if (STATUS_EV[e.type]) el = by('data-hmember', assigneeOf(e)) || holders;
    else if (e.type === 'item.assign' || e.type === 'item.handoff' || e.type === 'item.claim' || e.type === 'item.approve') el = holders;
    else if (e.type === 'item.request' || e.type === 'item.deny' || e.type === 'item.unrequest') el = by('data-req', e.type === 'item.request' ? e.id : p.request);
    else if (e.type === 'item.open' || e.type === 'item.unopen') el = root.querySelector('[data-focus="opening"]');
    else if (e.type === 'item.cc' || e.type === 'item.uncc') el = by('data-ccmember', p.member) || root.querySelector('[data-focus="cc"]');
    else if (e.type === 'item.cc_org' || e.type === 'item.uncc_org') el = by('data-ccorg', p.org) || root.querySelector('[data-focus="cc"]');
    else if (e.type === 'item.accept') el = by('data-hmember', e.member) || holders;
    else if (e.type === 'item.decline' || e.type === 'item.return' || e.type === 'item.unassign') {
      var gone = vals(VS.assignments).filter(function (r) { return r.ended && r.ended.id === e.id; })[0];
      el = gone && (by('data-rec', gone.id) || by('data-hmember', gone.to));
    }
    else if (e.type === 'item.create' || e.type === 'item.edit') {
      var atts = listOf(p.attachments).filter(function (x) { return x && x.oid; });
      el = !('body' in p) && atts.length ? by('data-oid', atts[0].oid) : root.querySelector('[data-focus="body"]');
    }
    el = el || by('data-ev', e.id);
    if (el && el.closest('.arow.more') && ui.aopen !== ui.item) { // the target sits in the folded rows: expand them
      ui.aopen = ui.item;
      var lst = root.querySelector('.alist'), tg = root.querySelector('[data-act="afold"]');
      if (lst) lst.classList.remove('folded');
      if (tg) tg.textContent = wsT("js.tk.asg.fewer");
    }
    var grpEl = el && el.closest('.arow.grp.shut');
    if (grpEl) { // the target sits inside a collapsed org group: expand it
      var gkey = grpEl.getAttribute('data-orgkey');
      if (gkey && ui.item) ui.iwOrgFold[ui.store + '|' + ui.item + '|' + gkey] = true;
      grpEl.classList.remove('shut');
      var gb = grpEl.querySelector('.tk-iw-fold');
      if (gb) gb.setAttribute('aria-expanded', 'true');
    }
    var runEl = el && el.closest('.tr-run.shut');
    if (runEl) { // the target sits in a collapsed thread run: expand it
      var rid = runEl.getAttribute('data-run');
      if (rid && ui.item) setRunCollapsed(threadFoldMap(ui.item), rid, false);
      runEl.classList.remove('shut');
      var rb = runEl.querySelector('.tr-rs button');
      if (rb) rb.setAttribute('aria-expanded', 'true');
    }
    return el;
  }
  // The item window's parts. The window is a header (title, chips, actions) over two
  // columns: the main one (body, subtasks, thread, composer) and a narrow side one (the property
  // card, the assignees). Below 820px of window width it is one column, in the order header ->
  // property card -> body -> subtasks -> assignees -> thread -> composer (CSS `order`).
  // How far up the lead chain a member is: 0 for a root, 1 for a root's follower, and so on.
  function leadDepth(id) {
    var d = 0, m = VS.members[id];
    while (m && (m.leads || [])[0] && d < 50) { d++; m = VS.members[m.leads[0]]; }
    return d;
  }
  var A_SHOWN = 4; // assignee rows shown before the "N more" toggle
  // The holders in one line: the count, then the live statuses other than todo, then pending.
  function holdersSummary(it) {
    var hs = holders(it), c = {}, parts = [];
    if (!hs.length) return '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>';
    hs.forEach(function (r) { var k = r.state === 'pending' ? 'pending' : r.status; c[k] = (c[k] || 0) + 1; });
    ['blocked', 'doing', 'in-review', 'done', 'dropped', 'pending'].forEach(function (k) { if (c[k]) parts.push(esc(SI.info(k).label) + ' ' + c[k]); });
    return '<b>' + hs.length + '</b>' + parts.map(function (x) { return ' <span class="muted">&middot;</span> ' + x; }).join('');
  }
  function ancestorsOf(it) {
    var out = [], cur = it.parent;
    while (cur && VS.items[cur] && out.length < 50) { out.unshift(cur); cur = VS.items[cur].parent; }
    return out;
  }
  // The property card's labels: an editor's are a button that opens the item menu's labels step
  // (labelStepHtml), where a toggle writes at once; anyone else's only show.
  function labelsProp(it) {
    var cs = labelChips(it);
    if (!canEdit(it)) return cs || '<span class="muted">-</span>';
    var names = (it.labels || []).map(labelOf).filter(Boolean).map(function (l) { return l.name; }).join(', '), t = wsT("js.tk.prop.labels.edit");
    return '<button type="button" class="plab" id="iw-plabels"' + attr('act', 'labels') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(names ? t + ': ' + names : t) + '"' + tipAttr(t) + '>' +
      (cs || '<span class="muted">' + wsT("js.tk.prop.labels.add") + '</span>') + '</button>';
  }
  function propertyCard(it) {
    var hs = holders(it), topic = it.kind === 'topic';
    return '<section class="pcard" aria-label="' + wsT("js.tk.prop.aria") + '"><h3>' + wsT("js.tk.prop.title") + '</h3><dl>' +
      '<dt>' + wsT("js.tk.prop.status") + '</dt><dd>' + (topic ? '<span class="muted">' + wsT("js.tk.prop.topic") + '</span>' : esc(SI.info(it.status).label) + ' <span class="muted">' + esc(SI.info(it.status).desc) + '</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.kind") + '</dt><dd>' + esc(kindName(it.kind)) + (it.pending.length ? ' <span class="muted">' + wsT("js.tk.prop.pending") + '</span>' : '') + (it.hidden ? ' <span class="muted">' + wsT("js.tk.prop.hidden") + '</span>' : '') + '</dd>' +
      (topic ? '<dt>' + wsT("js.tk.prop.holder") + '</dt><dd' + attr('focus', 'holders') + '>' + (holderNames(it) || '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>') + '</dd>' :
        '<dt>' + wsT("js.tk.prop.holders") + '</dt><dd' + attr('focus', 'holders') + '>' + holdersSummary(it) + '</dd>') +
      (it.chain.length > 1 ? '<dt>' + wsT("js.tk.prop.path") + '</dt><dd>' + esc(it.chain.join(' > ')) + '</dd>' : '') +
      (topic ? '' : '<dt>' + wsT("js.tk.prop.due") + '</dt><dd>' + (it.due || it.start ? esc(it.start && it.due ? it.start + ' – ' + it.due : it.due || wsT("js.tk.due.from", it.start)) : '<span class="muted">-</span>') + '</dd>') +
      '<dt>' + wsT("js.tk.prop.creator") + '</dt><dd>' + whoName(it.author) + (VS.members[it.author] && VS.members[it.author].admin ? ' <span class="chip admin">' + wsT("js.tk.team.admin") + '</span>' : '') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.project") + '</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.labels") + '</dt><dd>' + labelsProp(it) + '</dd>' +
      '<dt>' + wsT("js.tk.prop.parent") + '</dt><dd>' + (it.parent ? itemRef(it.parent) + ' ' + esc((VS.items[it.parent] || {}).title || '') : '<span class="muted">-</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.id") + '</dt><dd><code>' + esc(it.id) + '</code></dd>' +
      '<dt>' + wsT("js.tk.prop.dates") + '</dt><dd>' + wsT("js.tk.row.created", esc(fmtTs(it.ts))) + (it.edited ? '<br>' + wsT("js.tk.row.edited", esc(fmtTs(it.edited.ts))) : '') + '<br>' + wsT("js.tk.row.activity", esc(fmtTs(it.lastActivity))) + '</dd></dl></section>';
  }
  // Assignees: at most A_SHOWN rows, the highest in the lead chain first (shallowest lead depth;
  // then live before ended, then display name, then assignment order). What every row shares (the
  // provenance text and the close rule) is written once in the header; a row adds only what
  // differs. The rows past A_SHOWN are in the markup, hidden until the "N more" toggle (ui.aopen).
  // The assignees card's "+": who may assign sees it (the menu's rule: the creator, and on a topic
  // only while nobody holds it, as the store refuses a second holder). It opens the assign
  // popover (asgHtml, the item menu's assign step) anchored at itself.
  function canAssign(it, v) { return it.author === v && !it.hidden && (it.kind !== 'topic' || !holders(it).length); }
  function assignAddBtn(it, v) {
    if (!canAssign(it, v)) return '';
    return '<button type="button" class="ia a-add"' + attr('act', 'asg-add') + attr('item', it.id) + ' aria-haspopup="dialog" aria-label="' + esc(wsT("js.tk.act.assign")) + '"' + tipAttr(wsT("js.tk.act.assign")) + '><svg class="i"><use href="#tki-plus"/></svg></button>';
  }
  function openAssign(id, anchor) { itemMenu(id, anchor, null, { step: 'assign' }); }
  // A remove-a-person action on an assignee row (unassign, withdraw): a small red minus badge.
  function rmBadge(label, r) {
    return '<button type="button" class="rmb"' + attr('act', 'unassign') + attr('item', r.item) + attr('who', r.to) + ' aria-label="' + esc(label) + '"' + tipAttr(label) + '><svg class="i"><use href="#tki-minus"/></svg></button>';
  }
  // The records the assignees list shows: every live or pending record, and for a member who
  // holds neither only that member's latest ended record (latest by the parsed end time, ties to
  // the later record), so unassigning and reassigning never stacks rows. The member is the
  // record's `to`. The fold keeps every ended record; only this list hides them.
  function visibleRecs(recs) {
    var on = {}, last = {};
    recs.forEach(function (r) { if (r.state === 'live' || r.state === 'pending') on[r.to] = true; });
    recs.forEach(function (r, i) {
      if (r.state === 'live' || r.state === 'pending' || on[r.to]) return;
      var t = Date.parse(r.ended && r.ended.ts), b = last[r.to];
      if (isNaN(t)) t = -Infinity;
      if (!b || t >= b.t) last[r.to] = { r: r, t: t };
    });
    return recs.filter(function (r) { return r.state === 'live' || r.state === 'pending' || (last[r.to] && last[r.to].r === r); });
  }
  function asgOrgGroupHtml(it, g, gRows, renderRecRow) {
    var gkey = 'a:' + g.rule.id, fk = ui.store + '|' + it.id + '|' + gkey;
    var isOpen = fk in ui.iwOrgFold ? ui.iwOrgFold[fk] : g.deleted;
    var tag = g.deleted ? '<span class="tk-org-dtag">' + esc(wsT("js.tk.org.tag.deleted")) + '</span>'
      : g.vacant ? '<span class="tk-org-vac">' + esc(wsT("js.tk.org.tag.vacant")) + '</span>'
      : g.pending ? '<span class="chip sm pend">' + esc(wsT("js.tk.st.pending")) + '</span>' : '';
    var rmTip = g.stayed.length
      ? wsT("js.tk.iw.asg.rmorg.keep", g.onlyThrough.length, g.stayed.length)
      : wsTn("js.tk.iw.asg.rmorg.tip", g.onlyThrough.length, g.org.name);
    var root = g.rule.assignRoot || g.rule.root || '';
    var rmBtn = g.canRemove
      ? '<button type="button" class="rmb"' + attr('act', 'unassign') + attr('item', it.id) + attr('rule', g.rule.id) + attr('root', root) + ' aria-label="' + esc(wsT("js.tk.iw.asg.rmorg.aria", g.org.name)) + '"' + tipAttr(rmTip) + '><svg class="i"><use href="#tki-minus"/></svg></button>'
      : '';
    var foldBtn = '<button type="button" class="tk-iw-fold"' + attr('act', 'iw-ofold') + attr('item', it.id) + attr('key', gkey) + ' aria-expanded="' + (isOpen ? 'true' : 'false') + '">' +
      esc(wsTn("js.tk.iw.org.members", gRows.length)) + ' <span class="arw">' + (isOpen ? '&#9662;' : '&#9656;') + '</span></button>';
    var delBar = '';
    if (g.deleted) {
      var noteTxt = g.canRemove ? wsT("js.tk.iw.org.del.asg") : wsT("js.tk.iw.org.del.asg.byother", nm(g.rule.by));
      var acts = (g.canRetarget ? '<button type="button" class="btn mini primary"' + attr('act', 'retarget-asg') + attr('item', it.id) + attr('rule', g.rule.id) + attr('root', root) + attr('org', g.orgId) + '>' + esc(wsT("js.tk.iw.retarget")) + '</button>' : '') +
        (g.canRemove ? '<button type="button" class="btn mini"' + attr('act', 'unassign') + attr('item', it.id) + attr('rule', g.rule.id) + attr('root', root) + '>' + esc(wsT("js.tk.iw.asg.rmrule")) + '</button>' : '');
      delBar = '<div class="ar-diff muted">' + esc(noteTxt) + '</div>' + (acts ? '<div class="atb">' + acts + '</div>' : '');
    }
    var sub = gRows.length ? '<ul class="tk-iw-sub">' + gRows.map(function (x) { return renderRecRow(x, false, false); }).join('') + '</ul>' : '';
    return '<li class="arow grp' + (g.deleted ? ' float' : '') + (isOpen ? '' : ' shut') + '"' + attr('orgkey', gkey) + attr('asgorg', g.orgId) + '>' +
      '<div class="ar-main"><span class="tk-org-badge' + (g.deleted ? ' gone' : '') + '">' + esc(g.org.name) + '</span>' + tag + '<span class="grow"></span>' + foldBtn + rmBtn + '</div>' +
      delBar + sub + '</li>';
  }
  function assigneesHtml(v, it, recs) {
    recs = visibleRecs(recs);
    if (!recs.length) {
      if (!itemOrgRules(it, VS.rules).length) return '<section class="iw-asg"><h3>' + wsT("js.tk.asg.title") + '<span class="grow"></span>' + assignAddBtn(it, v) + '</h3><div class="empty">' + wsT("js.tk.asg.none") + '</div></section>';
    }
    var topic = it.kind === 'topic'; // a topic's record is its holder: no completion, no status
    var orgGroups = itemOrgAssignInfo(it, VS.rules, VS.assignments, VS.orgs, VS.members, v, canAssign(it, v));
    var rows = recs.map(function (r, i) { return { r: r, i: i, d: leadDepth(r.to), on: r.state === 'live' || r.state === 'pending', src: source(r), cm: topic ? null : complText(r.completion) }; });
    rows.sort(function (a, b) { return (b.on - a.on) || (a.d - b.d) || nm(a.r.to).localeCompare(nm(b.r.to)) || (a.i - b.i); });
    function common(key) { // the most frequent value, when at least two rows hold it
      var n = {}, best = null;
      rows.forEach(function (x) { n[x[key]] = (n[x[key]] || 0) + 1; if (n[x[key]] > 1 && (best === null || n[x[key]] > n[best])) best = x[key]; });
      return best;
    }
    var cs = common('src'), cc = common('cm'), open = ui.aopen === it.id, hid = rows.length - A_SHOWN, reqT = {};
    reqTargets(it).forEach(function (r) { reqT[r.id] = true; });
    var head = cs !== null || cc !== null ? '<div class="a-common muted">' + [cs, cc].filter(function (x) { return x !== null; }).join(' &middot; ') + '</div>' : '';
    function renderRecRow(x, foldMore, personalTag) {
      var r = x.r, b = '', rm = '';
      if (r.state === 'live' && topic && r.to === v) {
        // the holder releases the topic (unassigns their own record) or hands it off
        if (!it.hidden) b += btn('unassign', wsT("js.tk.act.release"), { item: r.item, who: v }) + btn('handoff', esc(wsT("js.tk.act.handoff")), { item: r.item });
      } else if (r.state === 'live') {
        if ((r.by === v || r.to === v) && wrapped(r.status)) b += btn('note', wsT("js.tk.asg.reopen"), { item: r.item, who: r.to, op: 'item.reopen' });
        if (!it.hidden && canReview(r)) b += btn('confirm', esc(wsT("js.tk.act.confirm", nm(r.to))), { item: r.item, who: r.to }, 'primary') + btn('note', esc(wsT("js.tk.act.reject", nm(r.to))), { item: r.item, who: r.to, op: 'item.reject' });
        var wa = wrapAuth(it, r);
        if (wa) b += btn('wrap', esc(wsT(wa === 'sup' ? "js.tk.act.wrap" : "js.tk.act.drop", nm(r.to))), { item: r.item, who: r.to });
        var departed = (VS.members[r.to] || {}).removed || (VS.members[r.by] || {}).removed;
        // a fallback holding (nobody else holds the task) is not unassigned or handed off: assign instead
        if (!r.fallback && (r.by === v || (departed && VS.viewer.admin))) rm = rmBadge(r.by === v ? wsT("js.tk.asg.unassign") : wsT("js.tk.asg.unassign.admin"), r);
        if (VS.viewer.admin && r.to !== v && !wrapped(r.status) && !r.fallback && !it.hidden) b += btn('handoff', wsT("js.tk.asg.handoff.admin"), { item: r.item, who: r.to });
        if (reqT[r.id]) b += btn('request', esc(wsT("js.tk.asg.request")), { item: r.item, who: r.to });
      } else if (r.state === 'pending' && r.to === v && (r.by === v || r.sender === v)) rm = rmBadge(wsT("js.tk.asg.withdraw"), r);
      var note = r.note || r.answerNote, diff = [];
      if (x.src !== cs) diff.push(cs !== null && x.src.indexOf(cs) === 0 ? x.src.slice(cs.length).trim() : x.src);
      if (x.cm !== cc && x.cm !== null) diff.push(x.cm);
      return '<li class="arow' + (!x.on ? ' gone' : '') + (foldMore ? ' more' : '') + '"' + attr('rec', r.id) + attr('hmember', r.to) + '>' +
        '<div class="ar-main">' + avHtml(r.to, r.state === 'pending') + '<span class="ar-name">' + whoName(r.to) + '</span>' +
        (personalTag ? '<span class="ar-tag muted">' + esc(wsT("js.tk.iw.asg.personal")) + '</span>' : '') +
        '<span class="ar-st">' + (topic && r.state === 'live' && !r.ended ? '<span class="chip">' + wsT("js.tk.asg.holder") + '</span>' : recChip(r)) + (r.ended ? ' <span class="muted">' + esc(alias(r.ended.id)) + endedBy(r) + '</span>' : '') + rm + '</span></div>' +
        (note ? '<div class="ar-note">&ldquo;' + esc(note) + '&rdquo;</div>' : '') +
        (diff.length ? '<div class="ar-diff muted">' + diff.join(' &middot; ') + '</div>' : '') +
        (b ? '<div class="tb">' + b + '</div>' : '') + '</li>';
    }
    var list = '', delCnt = 0;
    if (!orgGroups.length) {
      list = rows.map(function (x, k) { return renderRecRow(x, k >= A_SHOWN, false); }).join('');
    } else {
      hid = 0;
      var byRule = {}, groupedRec = {};
      orgGroups.forEach(function (g) { byRule[g.rule.id] = []; if (g.deleted) delCnt++; });
      rows.forEach(function (x) {
        if (x.on && x.r.rule && byRule[x.r.rule]) { byRule[x.r.rule].push(x); groupedRec[x.r.id] = true; }
      });
      list = orgGroups.map(function (g) { return asgOrgGroupHtml(it, g, byRule[g.rule.id], renderRecRow); }).join('') +
        rows.filter(function (x) { return !groupedRec[x.r.id]; }).map(function (x) { return renderRecRow(x, false, !x.r.rule && x.on); }).join('');
    }
    var delChip = delCnt ? ' <span class="chip flt">' + esc(wsTn("js.tk.iw.org.delcnt", delCnt)) + '</span>' : '';
    return '<section class="iw-asg"><h3>' + wsT("js.tk.asg.title") + ' <span class="cnt">' + rows.length + '</span>' + delChip + '<span class="grow"></span>' + assignAddBtn(it, v) + '</h3>' + head +
      '<ul class="alist' + (open || orgGroups.length ? '' : ' folded') + '">' + list + '</ul>' +
      (hid > 0 ? '<div class="a-fold">' + btn('afold', open ? wsT("js.tk.asg.fewer") : wsT("js.tk.asg.more", hid), {}) + '</div>' : '') + '</section>';
  }
  function ccOrgSectionHtml(it, cc, live) {
    var groups = itemOrgCCInfo(it, VS.orgs, VS.members), delCnt = 0, inOrg = {};
    var srcs = it.ccSources || {};
    groups.forEach(function (g) {
      if (g.deleted) delCnt++;
      g.added.forEach(function (m) { inOrg[m] = true; });
    });
    var add = live ? '<button type="button" class="ia a-add"' + attr('act', 'cc') + attr('item', it.id) + ' aria-haspopup="dialog" aria-label="' + esc(wsT("js.tk.cc.add")) + '"' + tipAttr(wsT("js.tk.cc.add")) + '><svg class="i"><use href="#tki-plus"/></svg></button>' : '';
    var delChip = delCnt ? ' <span class="chip flt">' + esc(wsTn("js.tk.iw.org.delcnt", delCnt)) + '</span>' : '';
    var rowsHtml = groups.map(function (g) {
      var gkey = 'c:' + g.orgId, fk = ui.store + '|' + it.id + '|' + gkey;
      var isOpen = fk in ui.iwOrgFold ? ui.iwOrgFold[fk] : g.deleted;
      var tag = g.deleted ? '<span class="tk-org-dtag">' + esc(wsT("js.tk.org.tag.deleted")) + '</span>'
        : g.vacant ? '<span class="tk-org-vac">' + esc(wsT("js.tk.org.tag.vacant")) + '</span>' : '';
      var rmTip = g.stayed.length
        ? wsT("js.tk.iw.cc.rmorg.keep", g.onlyThrough.length, g.stayed.length)
        : wsTn("js.tk.iw.cc.rmorg.tip", g.onlyThrough.length, g.org.name);
      var rmBtn = live ? '<button type="button" class="rmb"' + attr('act', 'uncc-org') + attr('item', it.id) + attr('org', g.orgId) + ' aria-label="' + esc(wsT("js.tk.iw.cc.rmorg.aria", g.org.name)) + '"' + tipAttr(rmTip) + '><svg class="i"><use href="#tki-minus"/></svg></button>' : '';
      var foldBtn = '<button type="button" class="tk-iw-fold"' + attr('act', 'iw-ofold') + attr('item', it.id) + attr('key', gkey) + ' aria-expanded="' + (isOpen ? 'true' : 'false') + '">' +
        esc(wsTn("js.tk.iw.org.members", g.added.length)) + ' <span class="arw">' + (isOpen ? '&#9662;' : '&#9656;') + '</span></button>';
      var delBar = g.deleted ? '<div class="ar-diff muted">' + esc(wsT("js.tk.iw.org.del.cc")) + '</div>' +
        (live ? '<div class="atb"><button type="button" class="btn mini primary"' + attr('act', 'retarget-cc') + attr('item', it.id) + attr('org', g.orgId) + '>' + esc(wsT("js.tk.iw.retarget")) + '</button>' +
          '<button type="button" class="btn mini"' + attr('act', 'uncc-org') + attr('item', it.id) + attr('org', g.orgId) + '>' + esc(wsT("js.tk.iw.cc.rmorg.btn")) + '</button></div>' : '') : '';
      var sub = g.added.length ? '<ul class="tk-iw-sub">' + g.added.map(function (id) {
        var rmt = wsT("js.tk.iw.cc.uncc.excl", nm(id));
        var rm = live ? '<button type="button" class="rmb"' + attr('act', 'uncc') + attr('item', it.id) + attr('who', id) + ' aria-label="' + esc(wsT("js.tk.cc.remove", nm(id))) + '"' + tipAttr(rmt) + '><svg class="i"><use href="#tki-minus"/></svg></button>' : '';
        return '<li class="arow"' + attr('ccmember', id) + '><div class="ar-main">' + avHtml(id) + '<span class="ar-name">' + whoName(id) + '</span><span class="ar-st">' + rm + '</span></div></li>';
      }).join('') + '</ul>' : '';
      return '<li class="arow grp' + (g.deleted ? ' float' : '') + (isOpen ? '' : ' shut') + '"' + attr('orgkey', gkey) + attr('ccorg', g.orgId) + '>' +
        '<div class="ar-main"><span class="tk-org-badge' + (g.deleted ? ' gone' : '') + '">' + esc(g.org.name) + '</span>' + tag + '<span class="grow"></span>' + foldBtn + rmBtn + '</div>' +
        delBar + sub + '</li>';
    }).join('');
    var personal = cc.filter(function (id) { return (srcs[id] && srcs[id].personal) || !inOrg[id]; });
    rowsHtml += personal.map(function (id) {
      var rmt = wsT("js.tk.iw.cc.uncc.excl", nm(id));
      var rm = live ? '<button type="button" class="rmb"' + attr('act', 'uncc') + attr('item', it.id) + attr('who', id) + ' aria-label="' + esc(wsT("js.tk.cc.remove", nm(id))) + '"' + tipAttr(rmt) + '><svg class="i"><use href="#tki-minus"/></svg></button>' : '';
      return '<li class="arow"' + attr('ccmember', id) + '><div class="ar-main">' + avHtml(id) + '<span class="ar-name">' + whoName(id) + '</span><span class="ar-tag muted">' + esc(wsT("js.tk.iw.cc.personal")) + '</span><span class="ar-st">' + rm + '</span></div></li>';
    }).join('');
    return '<section class="iw-asg iw-cc"' + attr('focus', 'cc') + '><h3>' + wsT("js.tk.cc.title") + ' <span class="cnt">' + cc.length + '</span>' + delChip + '<span class="grow"></span>' + add + '</h3><ul class="alist">' + rowsHtml + '</ul></section>';
  }
  // CC in the item window: the item's CC'd members (it.cc, in CC order), each with a remove
  // badge, and a "+" that opens the item menu's people step (item.cc). Any member may add or
  // remove anyone; a hidden item takes neither, and shows its list only when it has one.
  function ccHtml(it) {
    var cc = listOf(it.cc), orgCC = listOf(it.orgCC), live = !it.hidden;
    if (orgCC.length) return ccOrgSectionHtml(it, cc, live);
    if (!cc.length && !live) return '';
    var add = live ? '<button type="button" class="ia a-add"' + attr('act', 'cc') + attr('item', it.id) + ' aria-haspopup="dialog" aria-label="' + esc(wsT("js.tk.cc.add")) + '"' + tipAttr(wsT("js.tk.cc.add")) + '><svg class="i"><use href="#tki-plus"/></svg></button>' : '';
    return '<section class="iw-asg iw-cc"' + attr('focus', 'cc') + '><h3>' + wsT("js.tk.cc.title") + (cc.length ? ' <span class="cnt">' + cc.length + '</span>' : '') + '<span class="grow"></span>' + add + '</h3>' +
      (cc.length ? '<ul class="alist">' + cc.map(function (id) {
        var rm = live ? '<button type="button" class="rmb"' + attr('act', 'uncc') + attr('item', it.id) + attr('who', id) + ' aria-label="' + esc(wsT("js.tk.cc.remove", nm(id))) + '"' + tipAttr(wsT("js.tk.cc.remove", nm(id))) + '><svg class="i"><use href="#tki-minus"/></svg></button>' : '';
        return '<li class="arow"' + attr('ccmember', id) + '><div class="ar-main">' + avHtml(id) + '<span class="ar-name">' + whoName(id) + '</span><span class="ar-st">' + rm + '</span></div></li>';
      }).join('') + '</ul>' : '<div class="empty">' + wsT("js.tk.cc.none") + '</div>') + '</section>';
  }
  // Taking work in the item window: the opening (scope, opener, and Claim / Withdraw when the
  // viewer may) and the take requests on the item, pending first, each with the answers the
  // viewer may give. Drawn only when the item is open or has requests.
  var REQ_ST = { pending: "js.tk.req.st.pending", approved: "js.tk.req.st.approved", denied: "js.tk.req.st.denied", withdrawn: "js.tk.req.st.withdrawn", lapsed: "js.tk.req.st.lapsed" };
  function takeHtml(v, it) {
    var o = it.open, qs = reqsOf(it).slice().sort(function (a, b) { return (b.state === 'pending') - (a.state === 'pending') || (a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : 0); });
    if (!o && !qs.length) return '';
    var h = '<section class="iw-asg iw-take"><h3>' + wsT("js.tk.take.title") + (qs.length ? ' <span class="cnt">' + qs.length + '</span>' : '') + '</h3><ul class="alist">';
    if (o) {
      var b = (it.claimable ? btn('claim', esc(wsT("js.tk.act.claim")), { item: it.id }, 'primary') : '') + (canUnopen(it) ? btn('unopen', esc(wsT("js.tk.act.unopen")), { item: it.id }) : '');
      h += '<li class="arow"' + attr('focus', 'opening') + '><div class="ar-main">' + openChip(it) + '<span class="ar-name">' + (o.scope === 'store' ? wsT("js.tk.take.open.store", whoName(o.by)) : wsT("js.tk.take.open.team", whoName(o.by))) + '</span>' +
        '<span class="ar-st muted">' + esc(fmtTs(o.ts)) + '</span></div>' + (!it.claimable && o.by !== v ? '<div class="ar-diff muted">' + wsT("js.tk.open.tip.notmine") + '</div>' : '') + (b ? '<div class="tb">' + b + '</div>' : '') + '</li>';
    }
    h += qs.map(function (q) {
      var b = '';
      if (canApprove(it, q)) b += btn('approve', esc(wsT("js.tk.req.approve")), { item: it.id, req: q.id }, 'primary');
      if (canAnswer(it, q)) b += btn('note', esc(wsT("js.tk.req.deny")), { item: it.id, who: q.by, req: q.id, op: 'item.deny' });
      if (q.state === 'pending' && q.by === v) b += btn('unrequest', esc(wsT("js.tk.req.withdraw")), { item: it.id, req: q.id });
      return '<li class="arow' + (q.state === 'pending' ? '' : ' gone') + '"' + attr('req', q.id) + '><div class="ar-main">' + avHtml(q.by, q.state === 'pending') +
        '<span class="ar-name">' + wsT("js.tk.req.line", whoName(q.by), whoName(q.assignee)) + '</span><span class="ar-st"><span class="chip' + (q.state === 'pending' ? ' req' : ' ended') + '">' + esc(wsT(REQ_ST[q.state] || "js.tk.req.st.pending")) + '</span>' +
        (q.ended ? ' <span class="muted">' + esc(alias(q.ended.id)) + '</span>' : '') + '</span></div>' +
        (q.note ? '<div class="ar-note">&ldquo;' + esc(q.note) + '&rdquo;</div>' : '') + (q.answerNote ? '<div class="ar-note muted">&ldquo;' + esc(q.answerNote) + '&rdquo; (' + esc(nm(q.ended ? q.ended.by : '')) + ')</div>' : '') +
        (b ? '<div class="tb">' + b + '</div>' : '') + '</li>';
    }).join('');
    return h + '</ul></section>';
  }
  // Subtasks: the item's whole subtree, each row opening that item. Every node with children folds
  // with ▸/▾: the first level (the item's children) starts open, so their children show, and deeper
  // levels start folded. The fold state is the page's (ui.wfold), apart from the lists' stored one.
  // A node with children also carries its roll-up.
  // The section is drawn even without children when the viewer may add one (its "+" opens the
  // inline row here); a topic without children and without that right draws nothing.
  function subtasksHtml(it) {
    var add = ui.sub && ui.sub.rk === 'iw:' + it.id ? subRowHtml(0) : ''; // "add subtask" (header, menu, or the section's "+"): the input row tops the list
    var can = canSplit(it), sh = showHidden();
    function kids(id) { return (VS.items[id] ? VS.items[id].children : []).filter(function (k) { return VS.items[k] && (!VS.items[k].hidden || sh); }); }
    var topKids = kids(it.id);
    if (!topKids.length && !add && !can && it.kind === 'topic') return '';
    function rows(id, depth) {
      return kids(id).map(function (k) {
        var ki = VS.items[k], key = ui.store + '|' + k, subKids = kids(k), open = !!subKids.length && (key in ui.wfold ? ui.wfold[key] : depth === 0);
        var c = foldCtl({ id: k, n: subKids.length, open: open, to: '' }, 'wfold', 'w:');
        return '<div class="srow' + (wrapped(ki.status) ? ' gone' : '') + (ki.hidden ? ' tk-hidden' : '') + '"' + attr('item', k) + attr('rk', 'w:' + k) + ' style="--ind:' + depth + '">' + c.tw + stBtn(stKey(ki), ki) + titleRef(k) + c.sub + (ki.hidden ? '<span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '') +
          '<span class="grow"></span>' + (ki.hidden ? moreBtn(k, '', true) : '') + rollupHtml(ki) + whoHtml(ki) + '</div>' + (open ? rows(k, depth + 1) : '');
      }).join('');
    }
    var plus = can ? '<span class="grow"></span><button type="button" class="ia"' + attr('act', 'hsub') + attr('item', it.id) + ' aria-label="' + esc(wsT("js.tk.menu.sub")) + '"' + tipAttr(wsT("js.tk.menu.sub")) + '><svg class="i"><use href="#tki-plus"/></svg></button>' : '';
    return '<section class="iw-sub"><h3 class="prom">' + wsT("js.tk.win.subtasks") + ' <span class="cnt">' + topKids.length + '</span>' + rollupHtml(it) + plus + '</h3>' +
      (add || topKids.length ? '<div class="srows">' + add + rows(it.id, 0) + '</div>' : '') + '</section>';
  }
  // "Referenced by" (531): the items, then the comments, whose text references this one with a
  // #<alias> token (view backrefs), drawn only when there are any. An item row opens that item;
  // a comment row opens its own item's window at the comment.
  function backrefsHtml(it) {
    var l = listOf(it.backrefs).filter(function (id) { return VS.items[id] || VS.comments[id]; });
    if (!l.length) return '';
    return '<section class="iw-brefs"><h3 class="prom">' + wsT("js.tk.win.backrefs") + ' <span class="cnt">' + l.length + '</span></h3><div class="brows">' + l.map(function (id) {
      var x = VS.items[id], c = !x && VS.comments[id], host = c && VS.items[c.item];
      if (x) return '<a href="#" class="brow"' + attr('item', id) + '>' + stIcon(stKey(x)) + '<span class="bt">' + esc(x.title) + '</span><code class="al">' + esc(x.alias) + '</code></a>';
      return '<a href="#" class="brow"' + attr('act', 'bref') + attr('item', c.item) + attr('cid', id) + '><span class="sti"><svg class="i"><use href="#tki-chat"/></svg></span>' +
        '<span class="bt">' + esc(host ? host.title : alias(c.item)) + '</span><span class="bs">' + wsT("js.tk.bref.cmt", esc(nm(c.author))) + ' ' + snippet(c.body) + '</span></a>';
    }).join('') + '</div></section>';
  }
  // Thread run grouping and compact system lines (item window only; evLine stays
  // unchanged for the activity feed and wiki).
  var SYS_KIND_KEY = {
    'item.create': "js.tk.tr.k.create", 'item.edit': "js.tk.tr.k.edit",
    'item.cc': "js.tk.tr.k.cc", 'item.uncc': "js.tk.tr.k.uncc",
    'item.cc_org': "js.tk.tr.k.cc", 'item.uncc_org': "js.tk.tr.k.uncc",
    'item.assign': "js.tk.tr.k.assign", 'item.accept': "js.tk.tr.k.accept",
    'item.decline': "js.tk.tr.k.decline", 'item.return': "js.tk.tr.k.return",
    'item.handoff': "js.tk.tr.k.handoff", 'item.unassign': "js.tk.tr.k.unassign",
    'item.claim': "js.tk.tr.k.claim", 'item.open': "js.tk.tr.k.open", 'item.unopen': "js.tk.tr.k.open",
    'item.request': "js.tk.tr.k.request", 'item.approve': "js.tk.tr.k.request", 'item.deny': "js.tk.tr.k.request", 'item.unrequest': "js.tk.tr.k.request",
    'item.close': "js.tk.tr.k.close", 'item.confirm': "js.tk.tr.k.confirm", 'item.reject': "js.tk.tr.k.reject", 'item.reopen': "js.tk.tr.k.reopen",
    'item.hide': "js.tk.tr.k.hide", 'item.unhide': "js.tk.tr.k.hide", 'item.move': "js.tk.tr.k.move",
    'comment.add': "js.tk.tr.k.comment", 'comment.edit': "js.tk.tr.k.comment", 'comment.hide': "js.tk.tr.k.comment", 'comment.unhide': "js.tk.tr.k.comment"
  };
  function isThreadComment(n, itemId) { return n.type === 'comment.add' && n.thread === itemId; }
  function isThreadStatusBreak(n) { return n.type === 'item.status'; }
  function groupThreadEvents(evs, itemId) {
    var out = [], buf = [];
    function flush() {
      if (buf.length >= 2) out.push({ kind: 'run', id: buf[0].id, events: buf });
      else if (buf.length === 1) out.push({ kind: 'single', ev: buf[0] });
      buf = [];
    }
    evs.forEach(function (n) {
      if (!n) return;
      if (isThreadComment(n, itemId)) { flush(); out.push({ kind: 'comment', ev: n }); }
      else if (isThreadStatusBreak(n)) { flush(); out.push({ kind: 'status', ev: n }); }
      else buf.push(n);
    });
    flush();
    return out;
  }
  function runHasAlert(events) { return events.some(function (n) { return n.status === 'flagged' || !!(n.markers && n.markers.length); }); }
  function runDefaultCollapsed(events) { return events.length >= 4 && !runHasAlert(events); }
  function isRunCollapsed(run, foldMap) {
    return foldMap && Object.prototype.hasOwnProperty.call(foldMap, run.id) ? !!foldMap[run.id] : runDefaultCollapsed(run.events);
  }
  function setRunCollapsed(foldMap, runId, collapsed) { foldMap[runId] = !!collapsed; }
  function setAllRunsCollapsed(groups, foldMap, collapsed) { groups.forEach(function (g) { if (g.kind === 'run') foldMap[g.id] = !!collapsed; }); }
  function hasCollapsedRun(groups, foldMap) { return groups.some(function (g) { return g.kind === 'run' && isRunCollapsed(g, foldMap); }); }
  function fmtTime(ts) { return ts ? fmtTs(ts).slice(6) : '-'; }
  function fmtRunRange(events, full) {
    if (!events.length) return '';
    var a = fmtTs(events[0].ts), b = fmtTs(events[events.length - 1].ts);
    if (a === '-' && b === '-') return '';
    var sameDay = a.slice(0, 5) === b.slice(0, 5);
    if (full) return sameDay ? (a === b ? a : a + '\u2013' + b.slice(6)) : a + '\u2013' + b;
    if (sameDay) { var t1 = a.slice(6), t2 = b.slice(6); return t1 === t2 ? t1 : t1 + '\u2013' + t2; }
    return a + '\u2013' + b;
  }
  function sysGlyph(type) {
    if (type === 'item.cc' || type === 'item.cc_org') return { ch: '+', cls: 'cc' };
    if (type === 'item.uncc' || type === 'item.uncc_org') return { ch: '\u2212', cls: 'cc' };
    if (/^item\.(assign|accept|decline|return|handoff|unassign|claim|request|approve|deny|unrequest)$/.test(type)) return { ch: '\u2192', cls: 'as' };
    if (type === 'item.edit' || type === 'comment.edit') return { ch: '\u270e', cls: 'ed' };
    return { ch: '\u25cf', cls: '' };
  }
  function sysKindKey(type) { return SYS_KIND_KEY[type] || "js.tk.tr.k.misc"; }
  function runAuthorsText(events) {
    var ids = [];
    events.forEach(function (n) { if (n.member && ids.indexOf(n.member) < 0) ids.push(n.member); });
    if (!ids.length) return '';
    return ids.length === 1 ? nm(ids[0]) : wsTn("js.tk.tr.others", ids.length - 1, nm(ids[0]));
  }
  function runKindsText(events) {
    var order = [], counts = {};
    events.forEach(function (n) {
      var k = sysKindKey(n.type);
      if (!counts[k]) order.push(k);
      counts[k] = (counts[k] || 0) + 1;
    });
    return order.map(function (k) { return wsT(k) + ' ' + counts[k]; }).join(', ');
  }
  function plainTextOfHtml(html) {
    return String(html || '').replace(/<[^>]+>/g, '').replace(/&quot;/g, '"').replace(/&#39;/g, "'")
      .replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&middot;/g, '\u00b7').replace(/&rarr;/g, '\u2192')
      .replace(/&ldquo;/g, '\u201c').replace(/&rdquo;/g, '\u201d').replace(/&amp;/g, '&').replace(/\s+/g, ' ').trim();
  }
  function threadFoldMap(itemId) {
    var key = ui.store + '|' + itemId;
    ui.trfold = ui.trfold || {};
    return (ui.trfold[key] = ui.trfold[key] || Object.create(null));
  }
  function toggleThreadRun(runId) {
    var it = ui.item && VS && VS.items[ui.item];
    if (!it || !runId) return;
    var evs = it.thread.map(function (id) { return EV[id]; }).filter(Boolean);
    var run = groupThreadEvents(evs, it.id).filter(function (g) { return g.kind === 'run' && g.id === runId; })[0];
    if (!run) return;
    var fm = threadFoldMap(it.id);
    setRunCollapsed(fm, runId, !isRunCollapsed(run, fm));
    render();
  }
  function toggleThreadAll(wantOpen) {
    var it = ui.item && VS && VS.items[ui.item];
    if (!it) return;
    var evs = it.thread.map(function (id) { return EV[id]; }).filter(Boolean);
    setAllRunsCollapsed(groupThreadEvents(evs, it.id), threadFoldMap(it.id), !wantOpen);
    render();
  }
  function sysLineHtml(n, it, opts) {
    var desc = describe(n, n.parentThread === it.id ? 'parent' : true), gl = sysGlyph(n.type);
    var stCls = opts.statusBreak ? ' st-break' + (n.payload && n.payload.status ? ' tr-st-' + esc(n.payload.status) : '') : '';
    var cls = 'tr-sl' + (opts.inRun ? '' : ' wide') + (opts.sameAuthor ? ' same' : '') + stCls +
      (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers && n.markers.length ? ' concm' : '') +
      (n.origin && n.origin !== 'human' ? ' agent' : '');
    var full = plainTextOfHtml(whoName(n.member) + ' ' + desc);
    return '<li class="' + cls + '"' + attr('ev', n.id) + tipAttr(n.alias) + '>' +
      '<span class="tr-t">' + esc(opts.inRun ? fmtTime(n.ts) : fmtTs(n.ts)) + '</span>' +
      '<span class="tr-g' + (gl.cls ? ' ' + gl.cls : '') + '" aria-hidden="true">' + gl.ch + '</span>' +
      '<span class="tr-x" title="' + esc(full) + '"><span class="tr-who">' + whoName(n.member) + '</span>' + originChip(n.origin) + desc + flags(n) + '</span>' +
      (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(n) + supHtml(n) + '</li>';
  }
  function threadRunHtml(run, it, foldMap) {
    var shut = isRunCollapsed(run, foldMap), cnt = wsTn("js.tk.tr.count", run.events.length);
    var who = runAuthorsText(run.events), kinds = runKindsText(run.events);
    var range = fmtRunRange(run.events, false), fullRange = fmtRunRange(run.events, true);
    var tip = wsT(shut ? "js.tk.tr.tip.open" : "js.tk.tr.tip.close", fullRange);
    var hdr = '<span class="tr-n">' + esc(cnt) + '</span>' + (fullRange ? ' &middot; <span class="tr-t">' + esc(fullRange) + '</span>' : '');
    var sum = ['<span class="tr-n">' + esc(cnt) + '</span>'];
    if (who) sum.push(esc(who));
    if (kinds) sum.push(esc(kinds));
    if (range) sum.push('<span class="tr-t">' + esc(range) + '</span>');
    var rows = run.events.map(function (n, idx) {
      return sysLineHtml(n, it, { inRun: true, sameAuthor: idx > 0 && run.events[idx - 1].member === n.member, statusBreak: false });
    }).join('');
    return '<li class="tr-run' + (shut ? ' shut' : '') + '"' + attr('run', run.id) + attr('n', run.events.length) + '>' +
      '<div class="tr-rs"><button type="button"' + attr('act', 'trfold') + attr('run', run.id) + ' aria-expanded="' + (!shut) + '"' + tipAttr(tip) + '>' +
      '<span class="tr-ar" aria-hidden="true">&#9656;</span><span class="tr-hdr">' + hdr + '</span><span class="tr-sum">' + sum.join(' &middot; ') + '</span>' +
      '<span class="grow"></span><span class="tr-tw"><span class="tr-lbl-open">' + wsT("js.tk.tr.expand") + '</span><span class="tr-lbl-close">' + wsT("js.tk.tr.collapse") + '</span></span>' +
      '</button></div><ul>' + rows + '</ul></li>';
  }
  function threadSectionHtml(v, it) {
    var evs = it.thread.map(function (id) { return EV[id]; }).filter(Boolean);
    var groups = groupThreadEvents(evs, it.id), fm = threadFoldMap(it.id);
    var hasRuns = groups.some(function (g) { return g.kind === 'run'; });
    var anyShut = hasRuns && hasCollapsedRun(groups, fm);
    var allBtn = hasRuns ? '<span class="grow"></span><button type="button" class="tr-all"' + attr('act', 'trfold-all') + attr('all', anyShut ? 'open' : 'shut') + '>' + wsT(anyShut ? "js.tk.tr.expandall" : "js.tk.tr.collapseall") + '</button>' : '';
    return '<section class="iw-thread"><h3 class="prom">' + wsT("js.tk.win.thread") + ' <span class="cnt">' + evs.length + '</span>' + allBtn + '</h3><ul class="timeline">' +
      groups.map(function (g) {
        if (g.kind === 'comment') return commentHtml(g.ev, v, it);
        if (g.kind === 'run') return threadRunHtml(g, it, fm);
        return sysLineHtml(g.ev, it, { inRun: false, sameAuthor: false, statusBreak: g.kind === 'status' });
      }).join('') + '</ul></section>';
  }
  // The edit mode (ui.iedit) turns the title and the body into fields in place: Enter in the
  // title or Cmd/Ctrl+Enter in the body saves, Esc cancels.
  function windowHtml(v, it) {
    var recs = it.assignments.map(A), up = ancestorsOf(it), ed = ui.iedit && ui.iedit.item === it.id ? ui.iedit : null;
    var delSum = itemDeletedOrgSummary(it, VS.rules, VS.orgs, v);
    var cleanupChip = delSum.needsCleanup ? '<span class="chip flt"' + tipAttr(wsT("js.tk.iw.cleanup.tip")) + '>' + esc(wsT("js.tk.iw.cleanup")) + '</span>' : '';
    var h = '<div class="iw-top">' + (up.length ? '<nav class="iw-crumb" aria-label="' + wsT("js.tk.win.parents") + '">' + up.map(function (k) { return '<a href="#"' + attr('item', k) + '>' + esc(VS.items[k].title) + '</a>'; }).join('<span aria-hidden="true">&rsaquo;</span>') + '</nav>' : '') +
      '<div class="iw-head"' + attr('mitem', it.id) + attr('rk', 'iw:' + it.id) + '>' + stBtn(stKey(it), it) +
      (ed ? '<h2 id="iw-title" class="iw-edt"><input type="text" id="iw-etitle" data-keep autocomplete="off" value="' + esc(ed.title) + '" aria-label="' + wsT("js.tk.form.title") + '"></h2>' : '<h2 id="iw-title">' + esc(it.title) + '</h2>') +
      headActs(it) + moreBtn(it.id, 'iw-more', true) +
      '<button type="button" class="ia iw-x" data-act="win-close" aria-label="' + wsT("js.tk.win.close") + '"' + tipAttr(wsT("js.tk.win.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>' +
      '<div class="iw-chips">' + kindChip(it) + idCopyBtn(it) + (it.kind === 'topic' ? '' : stChip(it.status) + (canEdit(it) ? '' : prChip(it)) + dueChip(it) + rollupHtml(it)) + openChip(it) + cleanupChip + labelChips(it, 'lsearch') + warnHtml(it) + '</div></div>';
    h += '<div class="iw-grid"><div class="iw-main">';
    if (ed) h += '<div class="iw-body iw-edit">' + editorHtml('iw-ebody', { value: ed.body, rows: 5, label: wsT("js.tk.form.body"), pv: 'sh-body' }) +
      '<input type="text" id="iw-eproj" data-keep autocomplete="off" value="' + esc(ed.project) + '" placeholder="' + wsT("js.tk.form.project") + '" aria-label="' + wsT("js.tk.form.project") + '">' + labelPickHtml(ed) +
      (ed.restored ? '<div class="muted" role="status">Draft restored ' + btn('iedit-discard', 'Discard draft', {}) + '</div>' : '') +
      '<div class="tb">' + btn('iedit-save', wsT("js.tk.dlg.save"), {}, 'primary') + btn('iedit-x', wsT("js.tk.dlg.cancel"), {}) + '<span class="muted">' + wsT("js.tk.edit.hint") + '</span></div></div>';
    else if (it.body || it.mentions.length || it.attachments.length)
      h += '<div class="iw-body">' + (it.body ? textBlock('sh-body', 'items', it.id, it.body, attr('focus', 'body')) : '') + mentionChips(it.mentions) + attChips(it.attachments, true) + '</div>';
    h += subtasksHtml(it) + backrefsHtml(it);
    h += threadSectionHtml(v, it);

    // composer: a comment (optionally a quote reply); the status moved to the header's menu.
    // The box carries the @ button (bottom left) and the send button (bottom right); Cmd/Ctrl+Enter sends.
    // Typing @ (or the button) opens the member suggestion popup at the caret (see mentionSync).
    // The box is the shared editor (editorHtml) with the bar below its text.
    var rt = ui.replyTo && VS.comments[ui.replyTo] && VS.comments[ui.replyTo].item === it.id ? VS.comments[ui.replyTo] : null;
    var send = wsT("js.tk.composer.send", MAC ? '⌘Enter' : 'Ctrl+Enter');
    h += '<div class="composer2">' + (it.hidden ? '<span class="muted">' + wsT("js.tk.composer.hidden") + '</span>' : '') +
      (rt ? '<div class="replying">' + wsT("js.tk.composer.replying", esc(alias(rt.id)), esc(nm(rt.author)), esc(String(rt.body).slice(0, 70))) + ' ' + btn('reply-x', 'x', {}) + '</div>' : '') +
      draftHtml() +
      editorHtml('cbox', { cls: 'cbox', rows: 2, placeholder: wsT("js.tk.composer.placeholder", esc(nm(v))), pv: 'ctext', foot: '<div class="cbar">' +
      '<button type="button" id="cmention" class="cat"' + attr('act', 'mention') + ' aria-label="' + wsT("js.tk.composer.mention") + '"' + tipAttr(wsT("js.tk.composer.mention.tip")) + '>@</button>' +
      (CAPS.origin ? '<select id="corigin" title="' + wsT("js.tk.origin") + '"><option value="human">' + wsT("js.tk.origin.human") + '</option><option value="agent">' + wsT("js.tk.origin.agent") + '</option><option value="agent-auto">' + wsT("js.tk.origin.auto") + '</option></select>' : '') +
      '<span class="grow"></span><button type="button" class="csend" id="tkcsend"' + attr('act', 'comment') + attr('item', it.id) + ' aria-label="' + esc(send) + '"' + tipAttr(send) + '><svg class="i"><use href="#tki-send"/></svg></button></div>' }) +
      // with caps.attach, attachment metadata typed in by hand (a stand-in oid is computed)
      (CAPS.attach ? '<div class="cextra"><input id="catt-name" type="text" placeholder="' + wsT("js.tk.composer.attname") + '"><input id="catt-size" type="number" min="0" placeholder="' + wsT("js.tk.composer.attsize") + '">' +
      btn('att-add', wsT("js.tk.composer.attach"), {}) + '<span class="muted">' + wsT("js.tk.composer.limit", fmtSize(VS.attachments.max_bytes)) + '</span></div>' : '') + '</div>';
    h += '</div><div class="iw-side">' + propertyCard(it) + assigneesHtml(v, it, recs) + ccHtml(it) + takeHtml(v, it) + '</div></div>';
    return h;
  }
  // The header's common actions, left of the "⋯" menu: the viewer's status as a segmented control
  // (applies on click, no note; the menu keeps the status-with-note part), the priority chip (it
  // opens the menu's priority step; the chip row's read-only priority chip gives way to it), then
  // "add subtask" and "hand off" as icon buttons. Each shows under the same condition the menu uses.
  function headActs(it) {
    var mine = !it.hidden && mineLive(it), r = canStatus(it) && mine, h = '';
    if (r) {
      h += '<div class="iw-seg" role="group" aria-label="' + wsT("js.tk.prop.status") + '">' + ownStatusOpts(r).map(function (o) {
        // a disabled button shows no tooltip of its own, so the reason rides on a wrapper
        var b = '<button type="button" class="sg"' + attr('act', 'hstatus') + attr('item', it.id) + attr('status', o.s) + ' aria-pressed="' + (o.s === r.status) + '" aria-label="' + esc(o.label) + '"' + (o.dis ? ' disabled' : tipAttr(o.label)) + '>' +
          SI.svg(o.s) + '<span class="sl">' + esc(o.label) + '</span></button>';
        return o.dis ? '<span class="sgw"' + tipAttr(o.label + ': ' + o.why) + '>' + b + '</span>' : b;
      }).join('') + '</div>';
    }
    if (canEdit(it) && it.kind !== 'topic') {
      var lv = it.priorityLevel, d = VS.priority.default, pt = wsT("js.tk.win.pri", it.priorityName) + priStored(it);
      h += '<button type="button" class="hpri' + (lv > d ? ' hi' : lv < d ? ' lo' : '') + '"' + attr('act', 'pri') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(pt) + '"' + tipAttr(pt) + '>' +
        '<svg class="i"><use href="#tki-flag"/></svg><span>' + esc(it.priorityName) + '</span></button>';
    }
    if (canSplit(it)) h += '<button type="button" class="ia"' + attr('act', 'hsub') + attr('item', it.id) + ' aria-label="' + esc(wsT("js.tk.menu.sub")) + '"' + tipAttr(wsT("js.tk.menu.sub")) + '><svg class="i"><use href="#tki-plus"/></svg></button>';
    if (mine && !wrapped(mine.status) && !mine.fallback) h += '<button type="button" class="ia"' + attr('act', 'handoff') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.act.handoff")) + '"' + tipAttr(wsT("js.tk.act.handoff")) + '><svg class="i"><use href="#tki-handoff"/></svg></button>';
    if (canUnhide(it)) h += btn('iunhide', esc(it.author === VS.viewer.member ? wsT("js.tk.act.unhide") : wsT("js.tk.act.unhide.admin")), { item: it.id }, 'primary');
    return h ? '<div class="iw-acts">' + h + '</div>' : '';
  }
  // The item window's id button: "<kind> ID" copies the item's id (its tooltip) and says so for a moment.
  function idCopyBtn(it) {
    return '<button type="button" class="idcopy"' + attr('act', 'idcopy') + attr('id', it.alias) + ' aria-label="' + wsT("js.tk.win.idcopy.aria", esc(kindName(it.kind)), esc(it.alias)) + '"' + tipAttr(it.alias) + '>' +
      '<svg class="i"><use href="#tki-copy"/></svg><span>' + wsT("js.tk.win.idcopy", esc(kindName(it.kind))) + '</span></button>';
  }
  // Clipboard write; a page served over plain http (LAN) has no navigator.clipboard, so a
  // selected textarea and execCommand stand in. near is where the textarea goes: the rest
  // of the page is inert while the item window is open.
  function copyText(text, near, done) {
    function legacy() {
      var ta = document.createElement('textarea');
      ta.value = text; ta.setAttribute('readonly', ''); ta.style.position = 'fixed'; ta.style.opacity = '0';
      near.appendChild(ta); ta.select();
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
      ta.remove(); near.focus();
      done(ok);
    }
    if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(function () { done(true); }, legacy);
    else legacy();
  }
  function copyId(a) {
    copyText(a.getAttribute('data-id'), a.parentNode, function (ok) {
      if (!ok) return toast(esc(wsT("js.tk.win.copyfail")));
      var span = a.querySelector('span'), was = span.innerHTML;
      span.textContent = wsT("js.tk.win.copied"); a.classList.add('done');
      setTimeout(function () { if (a.isConnected) { span.innerHTML = was; a.classList.remove('done'); } }, 1200);
    });
  }
  // the attachments drafted for the next post; a picked mention shows as its @id in the text itself
  function draftHtml() {
    if (!ui.adraft.length) return '';
    return '<div class="cdraft">' +
      ui.adraft.map(function (x, i) { return '<span class="chip att">' + wsT("js.tk.att.file", esc(x.name), fmtSize(x.size)) + '</span>' + btn('att-x', 'x', { idx: i }); }).join('') +
      '<span class="muted">' + wsT("js.tk.composer.draft") + '</span></div>';
  }
  // A stand-in for the sha256 of the content: the drafted attachment has no content, so it hashes
  // the name and size into 64 hex digits. The real client hashes the file.
  function mockOid(s) {
    var h = '', x = 2166136261;
    for (var r = 0; r < 8; r++) { for (var i = 0; i < s.length; i++) x = Math.imul(x ^ s.charCodeAt(i) ^ r, 16777619) >>> 0; h += ('0000000' + x.toString(16)).slice(-8); }
    return h;
  }
  var MIME = { png: 'image/png', jpg: 'image/jpeg', pdf: 'application/pdf', txt: 'text/plain', zip: 'application/zip', zst: 'application/zstd', md: 'text/markdown' };
  function mimeOf(name) { return MIME[String(name).split('.').pop().toLowerCase()] || 'application/octet-stream'; }
  // A token in a text: tok with no word character, dot, hyphen, @, or # joined before it, and no word
  // character, hyphen, or # (or a dot that goes on into a word) after it, so @maya is not kept
  // by @mayan and #maya.pc-5 is not #maya.pc-50, while a sentence's closing dot is not part of it.
  function hasToken(body, tok) {
    var s = String(body), i = -1;
    while ((i = s.indexOf(tok, i + 1)) >= 0) {
      var b = s.charAt(i - 1), a = s.charAt(i + tok.length);
      if (!/[\w.@#-]/.test(b) && !/[\w#-]/.test(a) && !(a === '.' && /\w/.test(s.charAt(i + tok.length + 1)))) return true;
    }
    return false;
  }
  // An edit of a body keeps the mentions whose @id is still in the text and every attachment.
  function keptMentions(ids, body) { return listOf(ids).filter(function (id) { return hasToken(body, '@' + id); }); }
  // the stored mentions plus the ones an edit picked with @, each once
  function withPicks(ids, picks) { var out = listOf(ids).slice(); listOf(picks).forEach(function (id) { if (out.indexOf(id) < 0) out.push(id); }); return out; }
  // (531) The items a text references: each #<alias> (or #<item id>) token that names an item of the
  // store, in text order. A write carries them as refs, so a ref lasts while its token stays in
  // the text; the composer's # popup only types the token. A token ending in dots or hyphens is
  // also tried without them (the sentence's own punctuation).
  var REF_TOK = /(^|[^\w.@#-])#(\w[\w.-]*(?:#\d+)?)/g;
  function refItem(k, map) {
    var id;
    while (!(id = k.indexOf('#') > 0 ? (VS.items[k] ? k : null) : map[k] || null) && /[.-]$/.test(k)) k = k.slice(0, -1);
    return id ? { id: id, tok: k } : null;
  }
  function aliasMap() { var m = {}; Object.keys(VS.items).forEach(function (id) { m[VS.items[id].alias] = id; }); return m; }
  function refsIn(text) {
    var out = [], map = aliasMap(), m;
    REF_TOK.lastIndex = 0;
    while ((m = REF_TOK.exec(String(text)))) { var r = refItem(m[2], map); if (r && out.indexOf(r.id) < 0) out.push(r.id); }
    return out;
  }
  // The same tokens in escaped text as links that open the item, its title as the tooltip.
  function refLinks(h) {
    var map = null;
    return h.replace(REF_TOK, function (m, pre, key) {
      var r = refItem(key, map = map || aliasMap());
      if (!r) return m;
      return pre + '<a href="#" class="iref"' + attr('item', r.id) + tipAttr(VS.items[r.id].title) + '>#' + esc(r.tok) + '</a>' + key.slice(r.tok.length);
    });
  }
  // In the daemon's rendered markdown: the text nodes outside links and code.
  function refLinksHtml(h) {
    if (h.indexOf('#') < 0) return h;
    var t = document.createElement('template'), w, n, ns = [];
    t.innerHTML = h;
    w = document.createTreeWalker(t.content, NodeFilter.SHOW_TEXT);
    while ((n = w.nextNode())) if (n.nodeValue.indexOf('#') >= 0 && !(n.parentElement && n.parentElement.closest('a, code, pre'))) ns.push(n);
    ns.forEach(function (x) {
      var e = esc(x.nodeValue), r = refLinks(e);
      if (r === e) return;
      var sp = document.createElement('span'), f = document.createDocumentFragment();
      sp.innerHTML = r;
      while (sp.firstChild) f.appendChild(sp.firstChild);
      x.parentNode.replaceChild(f, x);
    });
    return t.innerHTML;
  }

