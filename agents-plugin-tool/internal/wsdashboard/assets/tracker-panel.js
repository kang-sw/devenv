  // ------------------------------------------------------------------ cards

  // The notification feed: one tab, list only, newest first. Every entry
  // is derived from the view state; an entry only opens the item window, and the
  // actions live on the item's row in the Items panel. Read state is local
  // UI state per viewer clone, like the handle badge's `seen`; no event is written
  // (in the product the daemon keeps it per clone on the local filesystem).
  var NK = { assigned: "js.tk.nk.assigned", pending: "js.tk.nk.pending", answer: "js.tk.nk.answer", reference: "js.tk.nk.reference", done: "js.tk.nk.done", blocked: "js.tk.nk.blocked", dropped: "js.tk.nk.dropped",
    confirm: "js.tk.nk.confirm", reply: "js.tk.nk.reply", due: "js.tk.nk.due", mention: "js.tk.nk.mention", request: "js.tk.nk.request", cc: "js.tk.nk.cc" };
  function snippet(s) { var chars = Array.from(String(s || '')); return '&ldquo;' + esc(chars.length > 80 ? chars.slice(0, 77).join('') + '...' : chars.join('')) + '&rdquo;'; }
  // Every entry also carries `ev`, the event it is about (none for a due date, which has
  // none), so a click can open the item window at what the entry refers to (focusTarget).
  var STATUS_EV = { 'item.status': 1, 'item.close': 1, 'item.confirm': 1, 'item.reject': 1, 'item.reopen': 1 };
  // item.status carries assignee when an org superior (or the assigner) wrote another member's assignment
  function assigneeOf(e) { return e.type === 'item.close' ? e.member : e.payload.assignee || e.member; }
  // the assignment record's latest status-side event: the one written at its lastUpdate, else the last one
  function statusEv(r) {
    var l = VS.events.filter(function (e) { return STATUS_EV[e.type] && e.status === 'applied' && e.payload.item === r.item && assigneeOf(e) === r.to; });
    var at = l.filter(function (e) { return e.ts === r.lastUpdate; });
    return (at.length ? at : l).slice(-1).map(function (e) { return e.id; })[0] || null;
  }
  // A CC notice's sentence (my.ccNotices: status writes and comments on an item the viewer is
  // CC'd on); a status write's note and a comment's text ride along as its tail.
  var CC_KEY = { 'item.status': "js.tk.feed.cc.status", 'item.close': "js.tk.feed.cc.close", 'item.confirm': "js.tk.feed.cc.confirm",
    'item.reject': "js.tk.feed.cc.reject", 'item.reopen': "js.tk.feed.cc.reopen", 'comment.add': "js.tk.feed.cc.comment" };
  function ccNoticeTxt(x) {
    var e = EV[x.event], p = (e && e.payload) || {}, by = esc(nm(x.by)), to = esc(nm(x.to));
    if (x.type === 'comment.add') { var c = VS.comments[x.event]; return wsT("js.tk.feed.cc.comment", by, snippet(c ? c.body : p.body)); }
    if (!CC_KEY[x.type]) return null;
    var txt = x.type === 'item.status' ? (x.by === x.to ? wsT("js.tk.feed.cc.status.own", by, esc(stLabel(x.status))) : wsT("js.tk.feed.cc.status", by, to, esc(stLabel(x.status)))) : wsT(CC_KEY[x.type], by, to);
    return txt + (p.note ? wsT("js.tk.ev.tail", snippet(p.note)) : '');
  }
  // The bounded summary feed carries plain strings, never HTML or details.
  function projectedFeedTxt(x, v) {
    var by = esc(nm(x.by)), to = esc(nm(x.to)), txt;
    switch (x.kind) {
    case 'assigned': return x.inherited && x.ruleLevels != null ? wsT("js.tk.feed.assigned.all", by, esc(x.ruleLevels)) : wsT("js.tk.feed.assigned", by);
    case 'due': return x.overdue ? wsT("js.tk.feed.overdue", esc(x.due)) : wsT("js.tk.due.today");
    case 'pending': return wsT(x.handoff ? "js.tk.feed.pending.handoff" : "js.tk.feed.pending", by);
    case 'request': txt = x.to === v ? wsT("js.tk.feed.request", by) : wsT("js.tk.feed.request.of", by, to); break;
    case 'done': txt = wsT("js.tk.feed.finished", to); break;
    case 'blocked': txt = wsT("js.tk.feed.blocked", to); break;
    case 'dropped': txt = wsT("js.tk.feed.dropped", to, esc(nm(x.statusBy || x.to))); break;
    case 'answer': txt = x.noticeKind === 'declined' ? wsT("js.tk.feed.declined", to) : wsT(x.handoff ? "js.tk.feed.returned.handoff" : "js.tk.feed.returned", esc(nm(x.endedBy)), to); break;
    case 'reference': txt = x.noticeKind === 'handoff' ? wsT("js.tk.feed.tookover", to, esc(nm(x.from))) : x.handoff ? wsT("js.tk.feed.accepted.handoff", to) : wsT("js.tk.feed.accepted", to, by); break;
    case 'confirm': return wsT("js.tk.feed.confirm", to);
    case 'reply': return wsT("js.tk.feed.reply", by, snippet(x.snippet));
    case 'mention': return x.bodyMention ? wsT("js.tk.feed.mention.body", by) : wsT("js.tk.feed.mention", by, snippet(x.snippet));
    case 'cc':
      if (x.type === 'comment.add') return wsT("js.tk.feed.cc.comment", by, snippet(x.snippet));
      if (!CC_KEY[x.type]) return null;
      txt = x.type === 'item.status' ? (x.by === x.to ? wsT("js.tk.feed.cc.status.own", by, esc(stLabel(x.status))) : wsT("js.tk.feed.cc.status", by, to, esc(stLabel(x.status)))) : wsT(CC_KEY[x.type], by, to); break;
    default: return null;
    }
    return txt + (x.snippet ? wsT("js.tk.ev.tail", snippet(x.snippet)) : '');
  }
  function feedEntries(v) {
    if (Array.isArray(VS.feed)) return VS.feed.map(function (x) {
      return { key: x.key, kind: x.kind, item: x.item, ts: String(x.ts || ''), html: projectedFeedTxt(x, v), ev: x.ev || null, author: x.author || '' };
    }).filter(function (x) { return x.html != null && VS.items[x.item] && !VS.items[x.item].hidden; });
    var out = [], t = today();
    function add(key, kind, item, ts, html, ev) {
      if (VS.items[item] && !VS.items[item].hidden) out.push({ key: key, kind: kind, item: item, ts: String(ts || ''), html: html, ev: ev || null });
    }
    VS.my.assignments.map(A).forEach(function (r) {
      var it = VS.items[r.item];
      if (it.kind === 'topic') return; // a held topic carries no work to start or finish: the store's "a:" badge leaves it out too
      // new forced assignments: the same rule as the handle badge's "a:" keys
      if (r.by !== v && r.forced && r.status === 'todo') add('a:' + r.id, 'assigned', r.item, r.ts, r.inherited && r.ruleLevels != null ? wsT("js.tk.feed.assigned.all", esc(nm(r.by)), esc(r.ruleLevels)) : wsT("js.tk.feed.assigned", esc(nm(r.by))), r.via);
      // near the due date, as the due chip computes it
      if (it.due && it.due <= t && !wrapped(r.status) && !wrapped(it.status))
        add('d:' + r.id + ':' + (it.due < t ? 'over' : 'today'), 'due', r.item, it.due + 'T00:00:00Z', it.due < t ? wsT("js.tk.feed.overdue", esc(it.due)) : wsT("js.tk.due.today"));
    });
    VS.my.received.pending.map(A).forEach(function (r) {
      add('p:' + r.id, 'pending', r.item, r.ts, r.handoffOf ? wsT("js.tk.feed.pending.handoff", esc(nm(r.sender))) : wsT("js.tk.feed.pending", esc(nm(r.sender))), r.via);
    });
    // take requests I may answer: the handle badge's "q:" keys
    listOf((VS.my.requests || {}).incoming).forEach(function (id) {
      var q = (VS.requests || {})[id];
      if (q) add('q:' + id, 'request', q.item, q.ts, (q.assignee === v ? wsT("js.tk.feed.request", esc(nm(q.by))) : wsT("js.tk.feed.request.of", esc(nm(q.by)), esc(nm(q.assignee)))) + (q.note ? wsT("js.tk.ev.tail", snippet(q.note)) : ''), id);
    });
    // the model's derived notices; done / blocked / dropped: work I assigned, set by someone else
    // (the assignee, or an org superior of theirs)
    VS.notices.forEach(function (x) {
      var r = A(x.assignment), txt;
      if (x.kind === 'done' || x.kind === 'blocked' || x.kind === 'dropped') {
        txt = x.kind === 'done' ? wsT("js.tk.feed.finished", esc(nm(x.to))) : x.kind === 'blocked' ? wsT("js.tk.feed.blocked", esc(nm(x.to))) : wsT("js.tk.feed.dropped", esc(nm(x.to)), esc(nm(r.statusBy || x.to)));
        add(x.key, x.kind, x.item, x.ts, txt + (r.note ? wsT("js.tk.ev.tail", snippet(r.note)) : ''), statusEv(r));
        return;
      }
      if (x.kind === 'declined') txt = wsT("js.tk.feed.declined", esc(nm(x.to)));
      else if (x.kind === 'returned') txt = r.handoffOf ? wsT("js.tk.feed.returned.handoff", esc(nm(x.endedBy)), esc(nm(x.to))) : wsT("js.tk.feed.returned", esc(nm(x.endedBy)), esc(nm(x.to)));
      else if (x.kind === 'accepted') txt = r.handoffOf ? wsT("js.tk.feed.accepted.handoff", esc(nm(x.to))) : wsT("js.tk.feed.accepted", esc(nm(x.to)), esc(nm(x.by)));
      else if (x.kind === 'handoff') txt = wsT("js.tk.feed.tookover", esc(nm(x.to)), esc(nm(x.from)));
      else return;
      add(x.key, x.kind === 'declined' || x.kind === 'returned' ? 'answer' : 'reference', x.item, x.ts, txt + (x.note ? wsT("js.tk.ev.tail", snippet(x.note)) : ''), (x.kind === 'declined' || x.kind === 'returned' ? r.ended : r.accepted).id);
    });
    VS.my.today.confirms.map(A).forEach(function (r) {
      add('c:' + r.id + ':' + r.lastUpdate, 'confirm', r.item, r.lastUpdate, wsT("js.tk.feed.confirm", esc(nm(r.to))), statusEv(r));
    });
    vals(VS.comments).forEach(function (c) {
      if (c.hidden || c.author === v) return;
      var q = c.reply_to && VS.comments[c.reply_to];
      if (q && q.author === v) add('r:' + c.id, 'reply', c.item, c.ts, wsT("js.tk.feed.reply", esc(nm(c.author)), snippet(c.body)), c.id);
      var ts = c.edited ? c.edited.ts : c.ts;
      if (c.mentions.indexOf(v) >= 0) add('m:' + c.id + ':' + ts, 'mention', c.item, ts, wsT("js.tk.feed.mention", esc(nm(c.author)), snippet(c.body)), c.id);
    });
    vals(VS.items).forEach(function (it) {
      var ts = it.edited ? it.edited.ts : it.ts;
      if (it.author !== v && it.mentions.indexOf(v) >= 0) add('m:' + it.id + ':' + ts, 'mention', it.id, ts, wsT("js.tk.feed.mention.body", esc(nm(it.author))), it.edited ? it.edited.id : it.id);
    });
    // CC notices: the view already leaves out what notifies on its own; they are not in the badge
    listOf(VS.my.ccNotices).forEach(function (x) {
      var txt = ccNoticeTxt(x);
      if (txt) add(x.key, 'cc', x.item, x.ts, txt, x.event);
    });
    return out.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : a.key < b.key ? -1 : 1; });
  }
  function readMap() { return seenOf(ui.store).read; }
  function markRead(keys) {
    var rm = readMap();
    keys = keys.filter(function (k) { return !rm[k]; });
    if (keys.length) AD.markSeen(ui.store, { badge: [], read: keys });
  }
  function cardNotify(v) {
    var list = feedEntries(v), read = readMap(), unread = list.filter(function (x) { return !read[x.key]; }).length;
    var h = list.length ? list.map(function (x) {
      return '<div class="row click nfeed' + (read[x.key] ? '' : ' unread') + '"' + attr('item', x.item) + attr('nkey', x.key) + attr('nkind', x.kind) + (x.ev ? attr('nev', x.ev) : '') + '><div class="r1">' +
        '<span class="chip nk">' + esc(wsT(NK[x.kind])) + '</span>' + isNew(x.key) + titleRef(x.item) + '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(x.ts)) + '</span></div>' +
        '<div class="r2">' + x.html + '</div></div>';
    }).join('') : '<p class="muted">' + wsT("js.tk.feed.empty") + '</p>';
    return card(wsT("js.tk.tab.notify"), wsT("js.tk.feed.chip", unread, list.length), h, unread ? btn('nread-all', wsT("js.tk.feed.readall"), {}) : '', 'card-notify');
  }

  // Mention chips and attachment chips. Loading is simulated (caps.attach): a real
  // client fetches the blob on demand.
  function fmtSize(n) { return n >= 1048576 ? wsT("js.tk.size.mib", (n / 1048576).toFixed(1)) : n >= 1024 ? wsT("js.tk.size.kib", Math.round(n / 1024)) : wsT("js.tk.size.b", n); }
  function mentionChips(ids) {
    ids = listOf(ids).filter(function (x) { return typeof x === 'string'; });
    return ids.length ? '<div class="mentions">' + ids.map(function (id) {
      return '<span class="chip mention" title="' + wsT("js.tk.att.mentions", esc(id)) + '">@' + esc(VS.members[id] ? VS.members[id].name : id) + '</span>';
    }).join('') + '</div>' : '';
  }
  function attChips(list, live) {
    list = listOf(list).filter(function (x) { return x && typeof x === 'object'; });
    if (!list.length) return '';
    var got = ui.loaded[seenKey(ui.store)] || {};
    return '<div class="atts">' + list.map(function (x) {
      var label = wsT("js.tk.att.file", esc(x.name), Number.isInteger(x.size) ? fmtSize(x.size) : '?');
      var meta = wsT("js.tk.att.meta", x.mime, x.size, x.oid);
      if (!live) return '<span class="chip att rejected" title="' + wsT("js.tk.att.rejected") + '">' + label + '</span>';
      if (!CAPS.attach) return '<span class="chip att fixed" title="' + esc(meta) + '">' + label + '</span>';
      var on = !!got[x.oid];
      return '<button type="button" class="chip att' + (on ? ' loaded' : '') + '"' + attr('act', 'att-load') + attr('oid', x.oid) +
        ' title="' + esc(on ? meta : wsT("js.tk.att.fetch", meta)) + '">' + label + ' &middot; ' + (on ? wsT("js.tk.att.loaded") : wsT("js.tk.att.notloaded")) + '</button>';
    }).join('') + '</div>';
  }

  // ------------------------------------------------------------- Items panel
 
  // One panel replaces My assignments, Received requests, Assigned by me, My team,
  // and Items: scope presets (SCOPES) crossed with the view modes each scope offers
  // (SCOPE_MODES). It opens on "mine"; the chosen scope and each scope's mode are
  // remembered per store in memory, like the side cards' modes, and never written.

  // The modes per scope and the "recently closed" fold are display state: they start from the stored values.
  // today and drill (a team head's count) are display state too, never stored.
  function panelState() {
    var n = ui.store;
    return (ui.panel[n] = ui.panel[n] || { scope: 'mine', modes: (ui.pmodes[n] = ui.pmodes[n] || {}), today: null, drill: null, q: '', closedOpen: ui.closedOpen[n] !== false, treeOpen: {} });
  }
  function scopeList() { return SCOPES.filter(function (x) { return x[0] !== 'team' || VS.my.team.length; }); }
  function scopeOf() { var s = panelState().scope; return scopeList().some(function (x) { return x[0] === s; }) ? s : 'mine'; }
  function panelMode(s) { var m = panelState().modes[s]; return m && SCOPE_MODES[s].some(function (x) { return x[0] === m; }) ? m : 'list'; }

  // Today as summary chips in the Items panel header (the Today card is gone):
  // my open work / waiting for my answer / waiting for my confirm, always visible
  // whatever scope is shown. A chip narrows the panel to its rows, as a filter on
  // the current scope and view mode; the same chip again clears it. When the current
  // scope shows none of them, the click moves the panel to the scope that holds them
  // (TODAY_HOME), so a chip never opens on an empty panel.
  // [key, chip name key, tooltip key, the tooltip's lead phrase key (the filter's name in sentences)]
  var TODAY = [['work', "js.tk.today.work", "js.tk.today.work.tip", "js.tk.today.work.what"],
    ['pending', "js.tk.today.pending", "js.tk.today.pending.tip", "js.tk.today.pending.what"],
    ['confirms', "js.tk.today.confirms", "js.tk.today.confirms.tip", "js.tk.today.confirms.what"]];
  var TODAY_HOME = { work: 'mine', pending: 'received', confirms: 'given' };
  var TF = null; // per render: the active Today filter { key, recs: {assignment id}, items: {item id} }, or null
  function todayFilter() {
    var k = panelState().today;
    if (!k || !VS.my.today[k]) return null;
    var f = { key: k, recs: {}, items: {} };
    VS.my.today[k].forEach(function (id) { f.recs[id] = true; f.items[A(id).item] = true; });
    return f;
  }
  // The search box (row 2 of the panel header) filters the current scope and combines with
  // the Today filter. With adapter.search the query is the tracker's search grammar, which
  // the adapter's host parses and answers with the matching ids (SIDS): the request goes out
  // SEARCH_DEBOUNCE ms after the last keystroke, and again at once when the store's view
  // changes (a write or a refresh can change what matches), so the ids never go stale. Until
  // the first answer the list stays unfiltered, and while a newer request runs it keeps the
  // last answer. A malformed query leaves the list unfiltered and shows the host's message
  // under the box (SERR). Without adapter.search the box matches a plain substring (the
  // mockup). SQ is the lower-cased query of the render in progress, '' for none or while no
  // answer filters the list.
  var SQ = '', SIDS = null, SERR = '', FILT = false; // per render: the query; the matching ids, null when not filtering by ids; the malformed-query message; whether the Today filter or a search is on
  var SEARCH_DEBOUNCE = 250;
  function matchItem(it) {
    if (!SQ) return true;
    if (SIDS) return !!SIDS[it.id];
    // the plain-substring fallback still honours one label:<name> token, the query a label chip sets
    var lq = /^label:"?([^"]*)"?$/.exec(SQ);
    if (lq) return (it.labels || []).some(function (x) { var l = labelOf(x); return !!l && l.name.toLowerCase() === lq[1]; });
    if (SQ === 'is:claimable') return !!it.claimable; // and the is:claimable token, the query of the search grammar
    return [it.title, it.body, it.project, it.id, it.alias].concat((it.labels || []).map(labelName)).some(function (x) { return String(x == null ? '' : x).toLowerCase().indexOf(SQ) >= 0; });
  }
  // A label chip's click: the Items search becomes label:"<name>", set whole (no debounce).
  function labelSearch(id) {
    var l = labelOf(id);
    if (!l) return;
    if (ui.win) closeItemWindow();
    panelState().q = 'label:"' + l.name + '"';
    searchState(ui.store).now = true;
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    ui.sel = null;
    render();
  }
  // An "open" chip's click: the Items panel shows every item, filtered to the claimable ones.
  function claimableFilter() {
    if (ui.win) closeItemWindow();
    panelState().scope = 'all'; ui.kind = 'claimable';
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    ui.sel = null;
    render();
  }
  // a store's search state: want / wantView / wantVersion, the query and view identity the last request was
  // made for; seq, the request counter (an answer to an older request is dropped); q, ids, err,
  // the last answer; timer, the pending debounced request; now, the next changed query was set
  // whole (a support chat's search command), not typed, so it skips the debounce
  function searchState(n) { return (ui.psr[n] = ui.psr[n] || { seq: 0, want: null, wantView: null, q: null, ids: null, err: '', timer: null, now: false }); }
  // Called by each render of the panel with the trimmed query: (re)sends the request when the
  // query or view generation changed since the last one, debounced when the query was typed.
  function searchFor(n, q) {
    var st = searchState(n), now = st.now;
    st.now = false;
    if (!q) { clearTimeout(st.timer); ui.psr[n] = null; return; }
    var version = VS.projectionCursor || null;
    if (st.want === q && st.wantView === VS && st.wantVersion === version) return;
    var typed = st.want !== q && !now;
    st.want = q; st.wantView = VS; st.wantVersion = version;
    clearTimeout(st.timer);
    // Invalidate an old answer as soon as another query/generation is
    // scheduled, not only when its debounce timer starts the next request.
    var seq = ++st.seq;
    st.timer = setTimeout(function () {
      st.timer = null;
      AD.search(n, q, function (res) {
        if (ui.psr[n] !== st || seq !== st.seq || (version && AD.view && (AD.view(n) || {}).projectionCursor !== version)) return;
        st.q = q;
        if (res && res.ok) { st.err = ''; st.ids = {}; (res.ids || []).forEach(function (id) { st.ids[id] = true; }); }
        else { st.err = String((res && res.error) || ''); st.ids = null; }
        if (ui.store === n) render();
      });
    }, typed ? SEARCH_DEBOUNCE : 0);
  }
  // a row with an assignment record passes on the record; an item row passes on the item
  function keepRec(r) { return (!TF || !!TF.recs[r.id]) && matchItem(VS.items[r.item]); }
  function keepItem(id) { return (!TF || !!TF.items[id]) && matchItem(VS.items[id]); }
  function todayBar() {
    var t = VS.my.today, on = panelState().today;
    return '<div class="todaybar" role="group" aria-label="' + wsT("js.tk.today") + '"><span class="tl">' + wsT("js.tk.today") + '</span>' + TODAY.map(function (x, i) {
      var n = t[x[0]].length;
      return (i ? '<span class="tsep">&middot;</span>' : '') + '<button type="button" class="tch' + (on === x[0] ? ' on' : '') + (n ? '' : ' zero') + '" aria-pressed="' + (on === x[0]) + '"' +
        attr('act', 'today') + attr('set', x[0]) + tipAttr(on === x[0] ? wsT("js.tk.today.tip.on", wsT(x[2]), n) : wsT("js.tk.today.tip.off", wsT(x[2]), n)) + '>' + esc(wsT(x[1])) + ' <b>' + n + '</b></button>';
    }).join('') + (on ? '<button type="button" class="tch tclr"' + attr('act', 'today') + attr('set', on) + ' aria-label="' + wsT("js.tk.today.clear") + '"' + tipAttr(wsT("js.tk.today.clear")) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') + '</div>';
  }
  function filterEmpty() {
    var x = TF && TODAY.filter(function (y) { return y[0] === TF.key; })[0], what = x ? esc(wsT(x[3])) : '', q = esc(panelState().q.trim());
    return '<div class="empty">' + (SQ && TF ? wsT("js.tk.panel.empty.both", q, what) : SQ ? wsT("js.tk.panel.empty.search", q) : wsT("js.tk.panel.empty.today", what)) + ' ' +
      (SQ ? btn('psearch-clear', wsT("js.tk.panel.clearsearch"), {}) + ' ' : '') + (TF ? btn('today', wsT("js.tk.panel.clearfilter"), { set: TF.key }) : '') + '</div>';
  }

  // Recently closed: done items leave the main list for a section at the bottom of the
  // list views, greyed, expanded by default, holding what closed in the last CLOSED_DAYS days
  // of the page clock; a search lists every age (and every match, whatever its age). The close
  // time is the last write of the done assignment (`lastUpdate`: the close or confirm event's
  // ts) for a record row, and the latest of that over an item's done live assignments for an
  // item row, `lastActivity` when the item carries none: the view state has no closedAt, and
  // none is added. CLOSED is the section being built by the scope's list: entries with the
  // close time, the row count, and the rows; null for a view mode with no section (kanban has
  // its done column, the timelines mute ended rows).
  var CLOSED_DAYS = 14, CLOSED = null;
  function closeTs(it) {
    var t = it.assignments.map(A).filter(function (r) { return r.state === 'live' && wrapped(r.status); }).map(function (r) { return String(r.lastUpdate); }).sort().pop();
    return t || String(it.lastActivity);
  }
  function recentlyClosed(item, rec) {
    var time = (VS.itemTime || {})[item];
    if (time) return rec ? !!time.recentAssignments[rec.id] : time.recentlyClosed;
    var ts = rec ? rec.lastUpdate : closeTs(VS.items[item]);
    return Date.parse(VS.now) - Date.parse(ts) <= CLOSED_DAYS * 86400000;
  }
  function addClosed(ts, n, html, recent) {
    if (!CLOSED) return;
    if (SQ || (recent == null ? Date.parse(VS.now) - Date.parse(ts) <= CLOSED_DAYS * 86400000 : recent)) CLOSED.entries.push({ ts: ts, n: n, html: html });
    else CLOSED.older += n;
  }
  function closedHtml() {
    if (!CLOSED || (!CLOSED.entries.length && !CLOSED.older)) return '';
    var on = panelState().closedOpen, n = CLOSED.entries.reduce(function (a, x) { return a + x.n; }, 0);
    var rows = CLOSED.entries.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : 0; }).map(function (x) { return x.html; }).join('');
    return '<section class="pclosed' + (on ? ' open' : '') + '"><button type="button" class="pc-head" aria-expanded="' + on + '"' + attr('act', 'closed-toggle') + attr('fk', 'closed') + '><svg class="i"><use href="#tki-chev"/></svg>' +
      (SQ ? wsT("js.tk.closed") : wsT("js.tk.closed.recent")) + ' <span class="subn">' + n + '</span><span class="muted">' + (SQ ? wsT("js.tk.closed.allages") : CLOSED.older ? wsT("js.tk.closed.lastolder", CLOSED_DAYS, CLOSED.older) : wsT("js.tk.closed.last", CLOSED_DAYS)) + '</span></button>' +
      (on ? '<div class="pc-rows">' + (rows || '<p class="muted">' + wsT("js.tk.closed.none") + '</p>') + '</div>' : '') + '</section>';
  }

  // Context trees: every scope list (and each sub-section of one) draws its matching rows as
  // a tree. Each match's ancestors stand above it as greyed context rows (not matches: the
  // scope counts leave them out, a click opens the item); a context node's children that are
  // in no match's chain fold into one "… N more" row; a match's own children show folded
  // ("N subtasks"). The tree is the matches plus their ancestors (the spine). A node is in one
  // of three fold states, kept per device in browser storage by store + item id (ui.tfold):
  //   default  the spine children show; the others are the "… N more" row, or, when no
  //            spine child shows, the folded "N subtasks" chip;
  //   'o'      every child shows;
  //   'f'      no child shows.
  // The "all" tree passes opt.rest, which draws a node's non-spine children (its closed units)
  // as the "closed N" row instead; there a node toggles between the default and 'f' only.
  // The kids of a node are its non-hidden children (opt.kids replaces that), in item order.
  // A match's chain of ancestors stops below the first hidden one, or the first opt.keep(id) refuses
  // (the "all" scope's kind filter), so neither shows as a context row; the match then roots there.
  function kidsShown(id) { var it = VS.items[id]; return it ? it.children.filter(function (k) { return VS.items[k] && !VS.items[k].hidden; }) : []; }
  function foldOf(id) { return ui.tfold[ui.store + '|' + id] || ''; }
  // rows: the scope's matches in its own order, each an irow option object ({ rk, item, rec?, more?, closed? }).
  // Returns the entries in display order: { k: 'node', id, depth, rows (the node's matches; none for a context
  // row), n (children it can show), open (they show: the fold control reads ▾), to (the fold state a click on
  // the control sets) }, { k: 'more', id, depth, n } (the "… N more" row), { k: 'html', html } (opt.rest's).
  // Roots are ordered by their first match in `rows`, children by item order.
  function ctxTree(rows, opt) {
    opt = opt || {};
    var kidsOf = opt.kids || kidsShown, byItem = {}, rank = {}, pos = {}, spine = {}, out = [];
    VS.itemOrder.forEach(function (id, i) { pos[id] = i; });
    rows.forEach(function (r, i) {
      (byItem[r.item] = byItem[r.item] || []).push(r);
      for (var cur = r.item, n = 0; cur && VS.items[cur] && n < 50; cur = VS.items[cur].parent, n++) {
        if (cur !== r.item && (VS.items[cur].hidden || (opt.keep && !opt.keep(cur)))) break;
        if (!(cur in rank)) rank[cur] = i;
      }
    });
    function inItemOrder(a, b) { return (pos[a] || 0) - (pos[b] || 0); }
    Object.keys(rank).forEach(function (id) { var p = VS.items[id].parent; if (p && p in rank) (spine[p] = spine[p] || []).push(id); });
    function walk(id, depth) {
      var S = (spine[id] || []).sort(inItemOrder), U = kidsOf(id).filter(function (k) { return !(k in rank); }), f = foldOf(id);
      var n = S.length + U.length, open = n > 0 && f !== 'f' && (f === 'o' || !!S.length || !!opt.rest);
      out.push({ k: 'node', id: id, depth: depth, rows: byItem[id] || [], n: n, open: open, to: open ? 'f' : opt.rest ? '' : 'o' });
      if (!open) return;
      (f === 'o' && !opt.rest ? S.concat(U).sort(inItemOrder) : S).forEach(function (k) { walk(k, depth + 1); });
      if (U.length && (opt.rest || f !== 'o')) out.push(opt.rest ? { k: 'html', html: opt.rest(id, U, depth + 1) } : { k: 'more', id: id, depth: depth + 1, n: U.length });
    }
    Object.keys(rank).filter(function (id) { var p = VS.items[id].parent; return !(p && p in rank); })
      .sort(function (a, b) { return rank[a] - rank[b] || inItemOrder(a, b); }).forEach(function (id) { walk(id, 0); });
    return out;
  }
  // A node's fold control (`tw`, left of the status icon; a spacer on a node without children) and, while
  // folded, its "N subtasks" chip after the title; both carry the same toggle. act: the data-act
  // (tfold in the lists and the timeline, wfold in the item window); fk: the focus key prefix.
  function foldCtl(e, act, fk) {
    if (!e.n) return { tw: '<span class="tw"></span>', sub: '' };
    var t = VS.items[e.id].title;
    function b(cls, inner, tab) {
      return '<button type="button" class="' + cls + '"' + attr('act', act) + attr('node', e.id) + attr('to', e.to) + attr('fk', cls + ':' + fk + e.id) +
        ' aria-expanded="' + e.open + '" aria-label="' + esc(wsT(e.open ? "js.tk.tree.fold" : "js.tk.tree.unfold", t)) + '"' + tipAttr(wsT(e.open ? "js.tk.tree.fold" : "js.tk.tree.unfold", t)) + (tab ? '' : ' tabindex="-1"') + '>' + inner + '</button>';
    }
    return { tw: b('tw', '<svg class="i"><use href="#tki-chev"/></svg>', true), sub: e.open ? '' : b('tsub', esc(wsTn("js.tk.tree.subtasks", e.n))) };
  }
  function moreRow(e, fk) {
    return '<button type="button" class="tmore" style="--ind:' + e.depth + '"' + attr('act', 'tfold') + attr('node', e.id) + attr('to', 'o') + attr('fk', 'tm:' + fk + e.id) +
      tipAttr(wsT("js.tk.tree.more.tip", e.n, VS.items[e.id].title)) + '>' + esc(wsT("js.tk.tree.more", e.n)) + '</button>';
  }
  // A context tree as item rows. pre: the section's row-key prefix (context rows get pre + 'ctx:' + id).
  // A tree with no nesting at all draws no fold column, so a flat list keeps its plain rows.
  function treeHtml(ents, pre) {
    var flat = ents.every(function (e) { return e.k === 'node' && !e.n && !e.depth; });
    return ents.map(function (e) {
      if (e.k === 'html') return e.html;
      if (e.k === 'more') return moreRow(e, pre);
      var c = flat ? { tw: null, sub: '' } : foldCtl(e, 'tfold', pre);
      if (!e.rows.length) return irow({ rk: pre + 'ctx:' + e.id, item: e.id, depth: e.depth, ctx: true, tw: c.tw, tsub: c.sub, more: wsT("js.tk.tree.ctx") });
      return e.rows.map(function (r, i) {
        return irow({ rk: r.rk, item: r.item, rec: r.rec, more: r.more, closed: r.closed, depth: e.depth, tw: flat ? null : i ? '<span class="tw"></span>' : c.tw, tsub: i ? '' : c.sub });
      }).join('');
    }).join('');
  }
  function treeRows(rows, pre, opt) { return treeHtml(ctxTree(rows, opt), pre); }
  function keepKind(id) { return kindFilter(VS.items[id]); }

  function listMine() {
    var mine = VS.my.assignments.map(A).filter(keepRec), open = mine.filter(function (r) { return !wrapped(r.status); }), done = mine.filter(function (r) { return wrapped(r.status); });
    var created = VS.my.created.filter(keepItem);
    done.forEach(function (r) { addClosed(String(r.lastUpdate), 1, irow({ rk: 'mine:' + r.id, item: r.item, rec: r, closed: true }), recentlyClosed(r.item, r)); });
    if (FILT && !mine.length && !created.length) return filterEmpty();
    var h = open.length ? treeRows(open.map(function (r) { return { rk: 'mine:' + r.id, item: r.item, rec: r }; }), 'mine:') : TF ? '' : SQ ? (created.length ? '' : '<p class="muted">' + wsT("js.tk.list.nomatch") + '</p>') : '<p class="muted">' + wsT("js.tk.list.mine.none") + '</p>';
    if (created.length) h += sub(wsT("js.tk.list.mine.created"), created.length) + treeRows(created.map(function (id) { return { rk: 'mine:c:' + id, item: id }; }), 'mine:c:');
    return h;
  }
  function answerTxt(x) {
    var r = A(x.assignment);
    return (x.kind === 'declined' ? wsT("js.tk.feed.declined", esc(nm(x.to))) :
      (r.handoffOf ? wsT("js.tk.feed.returned.handoff", esc(nm(x.endedBy)), esc(nm(x.to))) : wsT("js.tk.feed.returned", esc(nm(x.endedBy)), esc(nm(x.to)))) +
      (x.restores ? wsT("js.tk.list.restored", esc(nm(x.from))) : r.handoffOf ? wsT("js.tk.list.nobodyholds") : '')) +
      (x.note ? wsT("js.tk.ev.tail", '&ldquo;' + esc(x.note) + '&rdquo;') : '') + ', ' + esc(fmtTs(x.ts));
  }
  function listReceived() {
    var R = VS.my.received, h = '';
    var ans = VS.notices.filter(function (x) { return (x.kind === 'declined' || x.kind === 'returned') && keepRec(A(x.assignment)); });
    function only(ids) { return ids.filter(function (id) { return keepRec(A(id)); }); }
    var pend = only(R.pending), answered = only(R.answered), sent = only(R.sent);
    if (FILT && !ans.length && !pend.length && !answered.length && !sent.length) return filterEmpty();
    // each sub-section draws its own tree
    function rows(ids, pre, more) { return treeRows(ids.map(function (id) { var r = A(id); return { rk: pre + id, item: r.item, rec: r, more: more && more(r) }; }), pre); }
    if (ans.length) h += sub(wsT("js.tk.list.rcv.answers"), ans.length) + treeRows(ans.map(function (x) { return { rk: 'rcv:a:' + x.key, item: x.item, rec: A(x.assignment), more: answerTxt(x) }; }), 'rcv:a:');
    if (pend.length || !FILT) h += sub(wsT("js.tk.today.pending.what"), pend.length) + (pend.length ? rows(pend, 'rcv:p:') : '<p class="muted">' + wsT("js.tk.list.rcv.none") + '</p>');
    if (answered.length) h += sub(wsT("js.tk.list.rcv.answered"), answered.length) + rows(answered, 'rcv:d:');
    if (sent.length) h += sub(wsT("js.tk.list.rcv.sent"), sent.length) + rows(sent, 'rcv:s:', function (r) { return wsT("js.tk.list.rcv.sentto", esc(nm(r.to))); });
    return h;
  }
  // Related to me: the view's related lists (my.related), the CC section above the mentions
  // section, each in the view's order and foldable (relSecHtml); the done items either section
  // would hold (related.closed) go to the recently-closed section like any scope's closed rows.
  function listRelated() {
    var R = VS.my.related || {}, cc = listOf(R.cc).filter(keepItem), mn = listOf(R.mentioned).filter(keepItem);
    listOf(R.closed).filter(keepItem).forEach(function (id) { addClosed(closeTs(VS.items[id]), 1, irow({ rk: 'rel:x:' + id, item: id, closed: true }), recentlyClosed(id)); });
    if (FILT && !cc.length && !mn.length) return filterEmpty();
    return relSecHtml('cc', wsT("js.tk.list.rel.cc"), cc, wsT("js.tk.list.rel.cc.none")) + relSecHtml('mentioned', wsT("js.tk.list.rel.mentioned"), mn, wsT("js.tk.list.rel.mentioned.none"));
  }
  // One related section: a fold header with its count, open unless folded (panelState().relFold,
  // display state, never stored), then its rows as a context tree. Under a filter an empty section is left out.
  function relSecHtml(key, label, ids, none) {
    if (FILT && !ids.length) return '';
    var rf = panelState().relFold = panelState().relFold || {}, on = !rf[key], pre = 'rel:' + key + ':';
    return '<section class="rsec' + (on ? ' open' : '') + '"' + attr('rsec', key) + '><button type="button" class="rs-head" aria-expanded="' + on + '"' + attr('act', 'rel-fold') + attr('set', key) + attr('fk', 'rsec:' + key) + '>' +
      '<svg class="i"><use href="#tki-chev"/></svg>' + label + ' <span class="subn">' + ids.length + '</span></button>' +
      (on ? (ids.length ? treeRows(ids.map(function (id) { return { rk: pre + id, item: id }; }), pre) : '<p class="muted">' + none + '</p>') : '') + '</section>';
  }
  function listGiven() {
    var v = VS.viewer.member, given = VS.my.given.filter(keepItem), open = [];
    if (!VS.my.given.length) return '<div class="empty">' + wsT("js.tk.list.given.never") + '</div>';
    if (!given.length) return filterEmpty();
    given.forEach(function (id) {
      var lines = VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); }).sort(byRank).map(function (r) {
        return '<b>' + esc(nm(r.to)) + '</b>' + (r.depth ? ' <span class="muted">L' + r.depth + '</span>' : '') + ' ' + esc(SI.info(stKey(VS.items[id], r)).label) + (r.forced ? '' : ' <span class="muted">' + (r.handoffOf ? wsT("js.tk.list.given.handoff") : wsT("js.tk.list.given.sent")) + '</span>') +
          (r.note ? ' &ldquo;' + esc(r.note) + '&rdquo;' : '');
      });
      if (wrapped(VS.items[id].status)) return addClosed(closeTs(VS.items[id]), 1, irow({ rk: 'given:' + id, item: id, more: lines.join('<br>'), closed: true }), recentlyClosed(id));
      open.push({ rk: 'given:' + id, item: id, more: lines.join('<br>') });
    });
    return open.length ? treeRows(open, 'given:') : '<p class="muted">' + wsT("js.tk.list.given.none") + '</p>';
  }
  // My team as a group-by-person view: each group header carries that person's roll-up counts:
  // the load (live work at todo, doing, or blocked, a creator-fallback holding left out) with its
  // per-status breakdown, stale and overdue (both inside the load), in review, idea, fallback-held,
  // done, dropped, unaccepted, and waiting on others. The viewer's subtree comes first; an admin's members
  // outside it follow under their own heading, indented by depth from the org-tree root.
  // Under a Today filter or a search a group with no matching row is left out.
  // A count is a button: it narrows the list to that person's rows of that count (the drill,
  // panelState().drill = { who, key }, list mode only), and the same count again clears it.
  // The statuses a team row's load counts, in its breakdown's order (the view's roll-up counts
  // the same ones); every other status but idea, done, and dropped (in-review) shows as its own
  // count after stale and overdue, sorted.
  var TEAM_LOAD = ['todo', 'doing', 'blocked'];
  // The records a team head's count stands for, by the rules the view's counts use, so a count
  // and its rows agree. key: a status, 'load', 'stale', 'overdue', 'fallback', 'unaccepted', or
  // 'waiting'; recs: the assignment records by id; items: the items by id; day: today's date.
  function teamDrillRecs(p, key, recs, items, day) {
    if (key === 'unaccepted' || key === 'waiting') return (key === 'unaccepted' ? p.pending : p.waiting).map(function (id) { return recs[id]; }).filter(Boolean);
    return Object.keys(recs).map(function (id) { return recs[id]; }).filter(function (r) {
      var it = items[r.item];
      if (r.to !== p.id || r.state !== 'live' || !it || it.hidden || it.kind === 'topic') return false;
      if (key === 'fallback') return !!r.fallback && !wrapped(r.status);
      if (r.fallback) return false;
      if (key !== 'load' && key !== 'stale' && key !== 'overdue') return r.status === key;
      if (TEAM_LOAD.indexOf(r.status) < 0) return false;
      return key === 'load' || (key === 'stale' ? !!r.stale : !!(day && it.due && it.due < day));
    });
  }
  // The drill in effect: { p, key }, or null; a drill on a person no longer listed is dropped.
  function teamDrill() {
    var ps = panelState(), d = ps.drill, p = d && VS.my.team.filter(function (x) { return x.id === d.who; })[0];
    if (d && !p) ps.drill = null;
    return p ? { p: p, key: d.key } : null;
  }
  function teamCounts(p, on) {
    function cnt(key, txt, tip, err) {
      var sel = !!on && on.key === key;
      return '<button type="button" class="tcn' + (err ? ' err' : '') + (sel ? ' on' : '') + '" aria-pressed="' + sel + '"' + attr('act', 'tdrill') + attr('who', p.id) + attr('set', key) + attr('fk', 'td:' + p.id + ':' + key) +
        tipAttr((tip ? tip + '; ' : '') + wsT(sel ? "js.tk.team.drill.off" : "js.tk.team.drill.tip")) + '>' + txt + '</button>';
    }
    function st(s, n) { return cnt(s, wsT("js.tk.team.st", n, esc(SI.info(s).label)), '', s === 'blocked'); }
    var c = [], seen = { idea: true, done: true, dropped: true }, br = [], other = [];
    TEAM_LOAD.forEach(function (s) { if (p.counts[s]) br.push(st(s, p.counts[s])); seen[s] = true; });
    Object.keys(p.counts).sort().forEach(function (s) { if (!seen[s] && p.counts[s]) other.push(st(s, p.counts[s])); });
    if (p.load) c.push(cnt('load', wsT("js.tk.team.load", p.load), wsT("js.tk.team.load.tip")) + (br.length ? ' (' + br.join(' &middot; ') + ')' : ''));
    if (p.stale) c.push(cnt('stale', wsT("js.tk.team.stale", p.stale), wsT("js.tk.team.stale.tip", STALE_DAYS), true));
    if (p.overdue) c.push(cnt('overdue', wsT("js.tk.team.overdue", p.overdue), wsT("js.tk.team.overdue.tip"), true));
    c = c.concat(other);
    if (p.counts.idea) c.push(st('idea', p.counts.idea));
    if (p.fallback) c.push(cnt('fallback', wsT("js.tk.team.fallback", p.fallback), wsT("js.tk.team.fallback.tip")));
    if (p.counts.done) c.push(st('done', p.counts.done));
    if (p.counts.dropped) c.push(st('dropped', p.counts.dropped));
    if (p.pending.length) c.push(cnt('unaccepted', wsT("js.tk.team.unaccepted", p.pending.length), ''));
    if (p.waiting.length) c.push(cnt('waiting', wsT("js.tk.team.waiting", p.waiting.length), wsT("js.tk.team.waiting.tip")));
    return c.join(' &middot; ');
  }
  function listTeam() {
    var dr = teamDrill(), outside = false;
    var h = VS.my.team.map(function (p) {
      if (dr && dr.p !== p) return '';
      var m = VS.members[p.id], on = dr && dr.key, work, wait;
      if (dr) {
        var pos = {}, got = teamDrillRecs(p, on, VS.assignments, VS.items, today()).filter(keepRec);
        p.open.forEach(function (id, i) { pos[id] = i + 1; });
        got = got.map(function (r, i) { return { r: r, i: i }; }).sort(function (a, b) { return (pos[a.r.id] || 1e9) - (pos[b.r.id] || 1e9) || a.i - b.i; }).map(function (x) { return x.r.id; });
        work = on === 'waiting' ? [] : got; wait = on === 'waiting' ? got : [];
      } else {
        work = p.open.concat(p.pending).filter(function (id) { return keepRec(A(id)); }); wait = p.waiting.filter(function (id) { return keepRec(A(id)); });
      }
      if (FILT && !dr && !work.length && !wait.length) return ''; // a drilled group stays, with its clear control
      var g = '';
      if (p.outside && !outside) { outside = true; g += sub('<span' + tipAttr(wsT("js.tk.team.outside.tip")) + '>' + wsT("js.tk.team.outside") + '</span>', VS.my.team.filter(function (x) { return x.outside; }).length); }
      g += '<div class="tg' + (p.outside ? ' out' : '') + '" style="--d:' + (p.outside ? p.depth : p.depth - 1) + '"' + attr('person', p.id) + '><div class="tg-head">' + avHtml(p.id) + '<b>' + whoName(p.id) + '</b>' +
        '<span class="nid">' + esc(p.id) + (p.outside ? '' : ' L' + p.depth) + (m.children.length ? wsT("js.tk.team.islead") : '') + '</span><span class="cnt">' + teamCounts(p, dr) + '</span>' +
        (dr ? '<button type="button" class="ia"' + attr('act', 'tdrill') + attr('who', p.id) + attr('set', '') + attr('fk', 'td:x') + ' aria-label="' + esc(wsT("js.tk.team.drill.clear")) + '"' + tipAttr(wsT("js.tk.team.drill.clear")) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') +
        '<span class="grow"></span><span class="muted ts"' + tipAttr(wsT("js.tk.team.last.tip")) + '>' + esc(fmtTs(p.last)) + '</span></div>';
      if (dr && !work.length && !wait.length) return g + '<p class="muted">' + wsT("js.tk.team.drill.none") + '</p></div>';
      // one tree per person
      g += treeRows(work.map(function (id) { var r = A(id); return { rk: 'team:' + id, item: r.item, rec: r, closed: r.state === 'live' && wrapped(r.status) }; }).concat(wait.map(function (id) {
        var r = A(id);
        return { rk: 'team:w:' + id, item: r.item, rec: r, more: r.handoffOf ? wsT("js.tk.team.sent.handoff", esc(nm(p.id)), esc(nm(r.to))) : wsT("js.tk.team.sent", esc(nm(p.id)), esc(nm(r.to))) };
      })), 'team:' + p.id + ':');
      return g + '</div>';
    }).join('');
    return h || (FILT && VS.my.team.length ? filterEmpty() : '<div class="empty">' + wsT("js.tk.team.nobody") + '</div>');
  }
  function allRows(kanban) { return VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(kanban ? function (it) { return !it.hidden && kindFilter(it); } : itemFilter); }
  // The tree and closed items. Under the default "open" status, a done item never
  // leaves its place silently: a closed child among open siblings folds into one "closed N"
  // row under its parent (a click expands it); a closed parent with an open descendant stays in
  // the tree, greyed; a closed parent whose whole subtree is closed moves, as one unit, to the
  // "recently closed" section (the parent's own roll-up, done 3/5, still counts the children).
  // While a search or the Today filter is on, the rows are the matches instead: the open and greyed
  // ones as a context tree, the closed ones in the section, every age. Another status filter draws
  // its matches as a context tree too.
  function listAll() {
    var bar = '<div class="toolbar">' + itemFilterBar() + '</div>';
    if (ui.status !== 'open') {
      var rows = allRows().filter(function (it) { return keepItem(it.id); });
      return bar + (rows.length ? treeRows(rows.map(function (it) { return { rk: 'all:' + it.id, item: it.id }; }), 'all:', { keep: keepKind }) : FILT ? filterEmpty() : '<div class="empty">' + wsT("js.tk.list.all.nomatch") + '</div>');
    }
    var base = {}, order = [], kids = {}, openBelow = {};
    VS.itemOrder.forEach(function (id) { var it = VS.items[id]; if (!it.hidden && kindFilter(it)) { base[id] = it; order.push(id); } });
    order.forEach(function (id) { var par = base[id].parent; if (par && base[par]) (kids[par] = kids[par] || []).push(id); });
    function kidsOf(id) { return kids[id] || []; }
    function isOpen(id) { return itemFilter(base[id]); }
    function isClosed(id) { return base[id].kind !== 'topic' && wrapped(base[id].status); }
    function hasOpen(id) { if (!(id in openBelow)) openBelow[id] = kidsOf(id).some(function (k) { return isOpen(k) || hasOpen(k); }); return openBelow[id]; }
    function inTree(id) { return isOpen(id) || (isClosed(id) && hasOpen(id)); }   // open, or closed with something open below
    function isUnit(id) { return isClosed(id) && !hasOpen(id); }                  // closed, and so is everything below
    function parentOf(id) { var par = base[id].parent; return par && base[par] ? par : null; }
    function size(id) { return 1 + kidsOf(id).reduce(function (a, k) { return a + size(k); }, 0); }
    function latest(id) { return kidsOf(id).reduce(function (a, k) { var t = latest(k); return t > a ? t : a; }, isClosed(id) ? closeTs(base[id]) : ''); }
    function unitRecent(id) { return recentlyClosed(id) || kidsOf(id).some(unitRecent); }
    function unitRows(id, depth) {
      return irow({ rk: 'all:' + id, item: id, depth: depth, closed: true }) + kidsOf(id).map(function (k) { return unitRows(k, depth + 1); }).join('');
    }
    if (FILT) {
      var main = order.filter(function (id) { return inTree(id) && keepItem(id); }), shut = order.filter(function (id) { return isUnit(id) && keepItem(id); });
      shut.forEach(function (id) { addClosed(closeTs(base[id]), 1, irow({ rk: 'all:' + id, item: id, closed: true }), recentlyClosed(id)); });
      return bar + (main.length ? treeRows(main.map(function (id) { return { rk: 'all:' + id, item: id, closed: isClosed(id) }; }), 'all:', { keep: keepKind }) : shut.length ? '<p class="muted">' + wsT("js.tk.list.nomatch") + '</p>' : filterEmpty());
    }
    // the tree's matches are the open items and the greyed closed parents; a node's closed units are its "closed N" row
    function closedRow(id, cu, depth) {
      var it = base[id], on = !!panelState().treeOpen[id], n = cu.reduce(function (a, k) { return a + size(k); }, 0), r = it.rollup;
      return '<button type="button" class="csum' + (on ? ' open' : '') + '" style="--ind:' + depth + '" aria-expanded="' + on + '"' + attr('act', 'tree-closed') + attr('parent', id) + attr('fk', 'csum:' + id) +
        tipAttr(wsT(on ? "js.tk.list.csum.fold" : "js.tk.list.csum.show", it.title + (r.total ? ' ' + wsT("js.tk.list.csum.done", r.counts.done || 0, r.total) : ''))) + '><svg class="i"><use href="#tki-chev"/></svg>' + wsT("js.tk.closed") + ' <span class="subn">' + n + '</span></button>' +
        (on ? cu.map(function (k) { return unitRows(k, depth); }).join('') : '');
    }
    var tree = order.filter(inTree).map(function (id) { return { rk: 'all:' + id, item: id, closed: isClosed(id) }; });
    order.forEach(function (id) {
      if (isUnit(id) && !(parentOf(id) && (isUnit(parentOf(id)) || inTree(parentOf(id))))) addClosed(latest(id), size(id), unitRows(id, 0), unitRecent(id));
    });
    return bar + (tree.length ? treeHtml(ctxTree(tree, { kids: function (id) { return base[id] ? kidsOf(id).filter(function (k) { return inTree(k) || isUnit(k); }) : []; }, rest: closedRow, keep: function (id) { return !!base[id]; } }), 'all:') :
      '<div class="empty">' + wsT("js.tk.list.all.none") + '</div>');
  }
  function scopeChip(s) {
    if (s === 'mine') return wsT("js.tk.team.open", VS.my.assignments.map(A).filter(function (r) { return !wrapped(r.status) && VS.items[r.item].kind !== 'topic'; }).length);
    if (s === 'related') { var rl = VS.my.related || {}; return wsTn("js.tk.scope.chip.items", listOf(rl.cc).length + listOf(rl.mentioned).length); }
    if (s === 'received') return wsT("js.tk.scope.chip.pending", VS.my.received.pending.length);
    if (s === 'given') return wsTn("js.tk.scope.chip.items", VS.my.given.length) + (VS.my.today.confirms.length ? ', ' + wsT("js.tk.scope.chip.confirm", VS.my.today.confirms.length) : '');
    if (s === 'team') {
      var t = { below: 0, outside: 0, load: 0, stale: 0, overdue: 0, pending: 0 };
      VS.my.team.forEach(function (p) { t[p.outside ? 'outside' : 'below']++; t.load += p.load; t.stale += p.stale; t.overdue += p.overdue; t.pending += p.pending.length; });
      return [t.below || !t.outside ? wsT("js.tk.scope.chip.below", t.below) : '', t.outside ? wsT("js.tk.scope.chip.outside", t.outside) : '', wsT("js.tk.team.load", t.load),
        t.stale ? wsT("js.tk.team.stale", t.stale) : '', t.overdue ? wsT("js.tk.team.overdue", t.overdue) : '', t.pending ? wsT("js.tk.team.unaccepted", t.pending) : ''].filter(Boolean).join(', ');
    }
    return wsT("js.tk.scope.chip.of", allRows(panelMode('all') === 'kanban').length, Object.keys(VS.items).length);
  }
  var LISTS = { mine: listMine, related: listRelated, received: listReceived, given: listGiven, team: listTeam, all: listAll };
  function panelBody(s, m, v) { return m === 'list' ? LISTS[s](v) : modeBody(s, m, v); }
  // the panel as a spec: the header is two rows: scopes and the scope chip,
  // then the view icons and the search box. It is the card header when the panel stands alone,
  // or the tool area under the tab bar in the narrow layout.
  var VIEW_ICON = { "js.tk.mode.list": 'list', "js.tk.mode.tree": 'tree', "js.tk.mode.kanban": 'kanban', "js.tk.mode.gantt": 'timeline', "js.tk.mode.calendar": 'calendar' };
  function cardPanel(v) {
    var s = scopeOf(), m = panelMode(s), q = panelState().q;
    var qt = q.trim();
    TF = todayFilter(); SQ = ''; SIDS = null; SERR = '';
    if (qt && !AD.search) SQ = qt.toLowerCase();
    else if (qt) {
      searchFor(ui.store, qt);
      var sr = searchState(ui.store);
      if (sr.err) { if (sr.q === qt) SERR = sr.err; } // a newer query's answer is pending: no stale message
      else if (sr.ids) { SQ = qt.toLowerCase(); SIDS = sr.ids; }
    } else searchFor(ui.store, '');
    FILT = !!(TF || SQ);
    CLOSED = m === 'list' ? { entries: [], older: 0 } : null;
    MSEEN = {};
    var scopes = '<span class="scopes" role="tablist" aria-label="' + wsT("js.tk.panel.scope") + '">' + scopeList().map(function (x) {
      return '<button type="button" class="scp' + (x[0] === s ? ' on' : '') + '" role="tab" aria-selected="' + (x[0] === s) + '"' + attr('act', 'scope') + attr('scope', x[0]) + tipAttr(wsT(x[2])) + '>' + esc(wsT(x[1])) + '</button>';
    }).join('') + '</span>';
    var modes = SCOPE_MODES[s].length > 1 ? '<span class="vmodes" role="group" aria-label="' + wsT("js.tk.panel.view") + '">' + SCOPE_MODES[s].map(function (x) {
      return '<button type="button" class="vbtn' + (m === x[0] ? ' on' : '') + '" aria-pressed="' + (m === x[0]) + '" aria-label="' + esc(wsT(x[1])) + '"' + attr('act', 'pmode') + attr('mode', x[0]) + tipAttr(wsT(x[1])) +
        '><svg class="i"><use href="#tki-' + VIEW_ICON[x[1]] + '"/></svg></button>';
    }).join('') + '</span>' : '';
    var search = '<span class="psearch"><svg class="i"><use href="#tki-search"/></svg><input id="psearch" type="text" role="searchbox" autocomplete="off" spellcheck="false" value="' + esc(q) + '" placeholder="' + wsT("js.tk.panel.search") + '" aria-label="' + wsT("js.tk.panel.search.aria") + '">' +
      (q ? '<button type="button" class="psx"' + attr('act', 'psearch-clear') + ' aria-label="' + wsT("js.tk.panel.clearsearch") + '"' + tipAttr(wsT("js.tk.panel.clearsearch.tip")) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') + '</span>';
    // a new item: the hover insert between list rows (insShow), or the "+" that ends the panel, which
    // opens the input row in its place (the keyboard path; an empty list keeps only this one)
    var n = ui.nrow[ui.store], body = panelBody(s, m, v) + (n && !n.at ? newRowHtml() : newBtn()), closed = closedHtml();
    var serr = SERR ? '<div class="psearch-err" role="alert">' + esc(SERR) + '</div>' : '';
    // the selection keeps what this draw showed; a badge pinned atop the list (sticky in the
    // scrolling body, over a strip of its own so it never sits on the first row) counts it and clears it
    var ms = mselOf();
    if (ms) Object.keys(ms.ids).forEach(function (id) { if (!MSEEN[id]) delete ms.ids[id]; });
    if (!mselOn()) mselClear();
    var nsel = mselIds().length, mbar = nsel ? '<div class="mselbar" role="status"><span class="chip mselc">' + esc(wsTn("js.tk.sel.n", nsel)) + '<button type="button" class="psx"' + attr('act', 'msel-clear') + ' aria-label="' + wsT("js.tk.sel.clear") + '"' +
      tipAttr(wsT("js.tk.sel.clear")) + '><svg class="i"><use href="#tki-x"/></svg></button></span></div>' : '';
    TF = null; SQ = ''; SIDS = null; SERR = ''; FILT = false; CLOSED = null; MSEEN = null;
    return { title: wsT("js.tk.tab.items"), tools: scopes + '<span class="chip">' + esc(scopeChip(s)) + '</span><div class="prow2">' + modes + search + '</div>' + serr, body: mbar + '<div class="plist">' + body + '</div>' + closed, id: 'card-items', cls: 'panel',
      attrs: attr('scope', s) + attr('view', m) + (nsel ? attr('msel', nsel) : '') };
  }
  // With the adapter's wiki, the title is the Items | Wiki switch (wkSwitch); the Today bar belongs to the items
  function panelHtml(p, withToday) {
    return '<section class="card panel" id="card-items"' + (p.attrs || '') + '><header>' + (AD.wiki ? wkSwitch() : '<h2>' + wsT("js.tk.tab.items") + '</h2>') + p.tools + '</header>' + (withToday && !p.bodyCls ? todayBar() : '') +
      '<div class="cbody' + (p.bodyCls ? ' ' + p.bodyCls : '') + '">' + p.body + '</div>' + ribbonHtml() + '</section>';
  }
  // Does the given scope and mode show any row under the Today filter `key`? (a chip click moves to TODAY_HOME when not)
  // The search is left out: typing in the box never moves the panel to another scope.
  function scopeHasToday(key, s, m, v) {
    var keep = panelState().today;
    panelState().today = key; TF = todayFilter(); FILT = true; CLOSED = null;
    var has = /data-item=/.test(panelBody(s, m, v));
    TF = null; FILT = false; panelState().today = keep;
    return has;
  }

