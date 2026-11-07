// Tracker mockup v2: the view. A dashboard-like frame (header, conversation
// placeholder, pull-down drawer with one handle per store) around the tracker
// cards, a thread side sheet, and small action dialogs. All state is computed in
// the browser by TrackerEngine (engine.js) from fixtures.js; no build, no fetch.
(function () {
  'use strict';

  var E = window.TrackerEngine;
  var replay = E.replay, reach = E.reach, alias = E.alias, itemList = E.itemList, recsOf = E.recsOf;
  var subtasksOf = E.subtasksOf, itemStatus = E.itemStatus, rollup = E.rollup, chain = E.chain;

  var stores = {}, order = [], gs = {};
  var ui = {
    store: null,          // the store the header control and the sheet refer to
    open: null,           // the store whose drawer is open, or null
    viewers: {},          // store -> member id the page is viewed as
    seen: {},             // "store|viewer" -> { badge key: true } already shown to that viewer
    fresh: {},            // badge keys that were new when the drawer last opened
    sync: {},             // store -> { online, lastSynced }
    item: null, sheet: false, last: null, replyTo: null,
    flaggedOnly: false, concOnly: false, kind: 'all', status: 'open', dataOpen: false, openLogs: {}
  };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }
  function $(id) { return document.getElementById(id); }
  function clone(x) { return JSON.parse(JSON.stringify(x)); }
  function attr(k, v) { return ' data-' + k + '="' + esc(v) + '"'; }

  // ------------------------------------------------------------- small views

  function nm(g, id) { return g.members[id] ? g.members[id].name : id; }
  function fmtTs(ts) { return ts ? String(ts).slice(5, 16).replace('T', ' ') : '-'; }
  function itemRef(id) { return '<a href="#"' + attr('item', id) + '>' + esc(alias(id)) + '</a>'; }
  function titleRef(g, id) {
    var it = g.items[id];
    return '<a class="title" href="#"' + attr('item', id) + '>' + esc(it ? it.title : alias(id)) + '</a>';
  }
  var ST_LABEL = { todo: 'todo', doing: 'doing', blocked: 'blocked', awaiting: 'awaiting confirm', done: 'done',
    unassigned: 'unassigned', pending: 'pending request', declined: 'declined', returned: 'returned' };
  function stChip(s) { return s ? '<span class="chip st-' + esc(s) + '">' + esc(ST_LABEL[s] || s) + '</span>' : ''; }
  function kindChip(it) {
    return '<span class="chip kind">' + esc(it.kind) + '</span>' + (it.request ? '<span class="chip req">request</span>' : '') +
      (it.hidden ? '<span class="chip hid">hidden</span>' : '');
  }
  function originChip(o) {
    return o === 'agent-auto' ? '<span class="chip agent">agent</span><span class="chip auto">auto</span>' :
      o === 'agent' ? '<span class="chip agent">agent</span>' : '';
  }
  function complText(c) { return c === 'assigner' ? 'assigner confirms' : 'assignee closes'; }
  function rollupHtml(g, it) {
    var r = rollup(g, it), h = '';
    if (r.total) h += '<span class="chip roll" title="children by status (manual status stays with the assignee)">done ' + (r.counts.done || 0) + '/' + r.total +
      (r.counts.blocked ? ' &middot; blocked ' + r.counts.blocked : '') + '</span>';
    if (r.blocked.length) h += '<span class="chip warn" title="blocked below: ' + esc(r.blocked.map(function (id) { return alias(id) + ' ' + g.items[id].title; }).join('; ')) + '">&#9888; blocked below</span>';
    return h;
  }
  function source(g, r) {
    if (r.request) return 'request from <b>' + esc(nm(g, g.items[r.item].request.by)) + '</b>, accepted';
    if (r.by === r.to) return 'self-assigned';
    return 'from <b>' + esc(nm(g, r.by)) + '</b>' + (r.inherited ? ' (inherited: everyone within L' + ((g.rules[r.via] || {}).levels || '?') + ', reached later)' : '');
  }
  function btn(act, label, data, cls) {
    var d = '';
    Object.keys(data || {}).forEach(function (k) { d += attr(k, data[k]); });
    return '<button class="btn mini' + (cls ? ' ' + cls : '') + '"' + attr('act', act) + d + '>' + label + '</button>';
  }
  function card(title, chip, body, extra) {
    return '<section class="card"><header><h2>' + title + '</h2>' + (chip ? '<span class="chip">' + chip + '</span>' : '') +
      '<span class="grow"></span>' + (extra || '') + '</header><div class="cbody">' + body + '</div></section>';
  }
  function itemDepth(g, it) {
    var d = 0, cur = it;
    while (cur && cur.parent && g.items[cur.parent] && d < 20) { d++; cur = g.items[cur.parent]; }
    return d;
  }
  function itemsInTreeOrder(g) {
    var out = [];
    (function walk(parent) {
      itemList(g).forEach(function (it) {
        if ((it.parent && g.items[it.parent] ? it.parent : null) !== parent) return;
        out.push(it); walk(it.id);
      });
    })(null);
    return out;
  }
  var RANK = { blocked: 0, doing: 1, todo: 2, awaiting: 3, done: 4 };
  function byRank(a, b) { return RANK[a.status] - RANK[b.status]; }

  function describe(g, n, inThread) {
    var p = n.p, ref = function (id) { return inThread ? '' : ' ' + itemRef(id); };
    switch (n.type) {
      case 'item.create':
        if (inThread === 'parent') return 'opened subtask thread ' + itemRef(n.id) + ' <b>' + esc(p.title) + '</b> ' + (g.items[n.id] ? stChip(itemStatus(g, g.items[n.id])) : '');
        return 'created ' + esc(p.kind) + (p.request ? ' as a <b>request to ' + esc(p.request) + '</b>' : '') + ' <b>' + esc(p.title) + '</b>' +
          (inThread ? '' : ' (' + itemRef(n.id) + ')') + (p.parent && !inThread ? ' under ' + itemRef(p.parent) : '');
      case 'item.edit':
        return 'edited' + ref(p.item) + ' ' + ['title', 'body', 'project'].filter(function (f) { return f in p; }).map(function (f) {
          return f + ' &rarr; "' + esc(String(p[f]).slice(0, 80)) + '"';
        }).join(', ');
      case 'item.hide': return 'hid' + ref(p.item) + ' (tombstone; history stays)';
      case 'item.assign':
        return 'assigned' + ref(p.item) + ' to ' + (p.mode === 'all' ? '<b>everyone within L' + esc(p.levels) + '</b> (live: later followers inherit)' :
          '<b>' + esc((p.people || []).map(function (x) { return x === n.author ? 'self' : x; }).join(', ')) + '</b>' + (p.levels ? ' (within L' + esc(p.levels) + ')' : '')) +
          ', ' + complText(p.completion);
      case 'item.unassign': return 'unassigned ' + esc(p.assignee || 'all of their assignees') + ref(p.item);
      case 'item.status': return 'set status ' + stChip(p.status) + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.close': return 'closed' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.confirm': return 'confirmed ' + esc(p.assignee) + '\'s close' + ref(p.item);
      case 'item.reject': return 'rejected ' + esc(p.assignee) + '\'s close (back to doing)' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.reopen': return 'reopened ' + esc(p.assignee || n.author) + '\'s assignment' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'request.accept': return 'accepted the request' + ref(p.item) + ' (now their own assignment)';
      case 'request.decline': return 'declined the request' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'request.return': return 'returned the accepted request' + ref(p.item) + ' (cancels the assignment)' + (p.note ? ': ' + esc(p.note) : '');
      case 'comment.add': return 'commented' + ref(p.item) + (inThread ? '' : ': ' + esc(String(p.body || '').slice(0, 90)));
      case 'comment.edit': return 'edited comment ' + esc(alias(p.comment)) + (inThread ? '' : ref(n.thread || ''));
      case 'comment.hide': return 'hid comment ' + esc(alias(p.comment)) + ' (tombstone; history stays)';
      case 'roles.add_member': return 'added member <b>' + esc((p.member || {}).id) + '</b> under ' + esc(((p.member || {}).leads || []).join(', ') || 'nobody');
      case 'roles.set_lead': return 'set lead of <b>' + esc(p.member) + '</b> to ' + esc((p.leads || []).join(', ') || 'nobody');
      case 'roles.add_admin': return 'made <b>' + esc(p.member) + '</b> an admin';
      case 'roles.remove_admin': return 'removed admin role from <b>' + esc(p.member) + '</b>';
      case 'roles.add_key': return 'added key <code>' + esc(p.key) + '</code> for ' + esc(p.member);
      case 'roles.revoke_key': return 'revoked key <code>' + esc(p.key) + '</code> of ' + esc(p.member);
      case 'roles.remove_member': return 'removed member <b>' + esc(p.member) + '</b>';
      default: return esc(n.type);
    }
  }
  function markerHtml(g, n) {
    return n.markers.map(function (m) {
      var o = g.byId[m.other], oa = esc(alias(m.other)) + ' (' + esc(o ? o.author : '?') + ')';
      if (m.kind === 'field') {
        return '<div class="conc"><b>concurrent change</b> to ' + esc(m.field) + ': this wrote <i>' + esc(m.mine) + '</i>, ' + oa + ' wrote <i>' + esc(m.theirs) +
          '</i> without seeing it. Shown: <b>' + esc(alias(m.winner)) + '</b> (later ts, then author id).</div>';
      }
      if (m.kind === 'removed') {
        return n.id === m.winner ?
          '<div class="conc"><b>concurrent change</b>: ' + oa + ' wrote to ' + esc(m.field) + ' without seeing this removal; that write is not in effect.</div>' :
          '<div class="conc"><b>concurrent change</b>: ' + oa + ' removed ' + esc(m.field) + ' without seeing this; valid when written, not in effect now.</div>';
      }
      return '<div class="conc"><b>concurrent change</b>: ' + esc(m.field) + ' was created while ' + oa + ' concurrently removed the parent assignment.</div>';
    }).join('');
  }
  function flags(n) {
    var c = n.commit, h = '';
    if (c._received === 'late') h += '<span class="chip late" title="reached this client after events written later">arrived late</span>';
    if (c._local) h += '<span class="chip local" title="written on this machine, not pushed yet">not pushed</span>';
    if (n.status === 'flagged') h += '<span class="chip flag">' + (n.revokedLate ? 'ignored' : 'flagged') + '</span>';
    if (n.status === 'waiting') h += '<span class="chip">waiting</span>';
    if (n.markers.length) h += '<span class="chip conc-c">concurrent</span>';
    return h;
  }
  function evLine(g, n, inThread) {
    return '<li class="ev' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (n.origin !== 'human' ? ' agent' : '') + '">' +
      '<span class="when">' + esc(fmtTs(n.ts)) + '</span><span class="al">' + esc(n.alias) + '</span> <b>' + esc(nm(g, n.author)) + '</b> ' + originChip(n.origin) +
      ' ' + describe(g, n, inThread) + ' ' + flags(n) + (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(g, n) + '</li>';
  }

  // ------------------------------------------------------- viewer / badge state

  function viewerOf(name) { return ui.viewers[name]; }
  function seenKey(name) { return name + '|' + viewerOf(name); }
  function badgeKeys(name) {
    var g = gs[name], v = viewerOf(name), out = [];
    Object.keys(g.asg).forEach(function (k) {
      var r = g.asg[k];
      if (r.to === v && !r.removed && r.status === 'todo' && r.by !== v) out.push(r.id);
    });
    itemList(g).forEach(function (it) { if (it.request && it.request.to === v && it.request.state === 'pending') out.push('req:' + it.id); });
    return out;
  }
  function newKeys(name) {
    var seen = ui.seen[seenKey(name)] || {};
    return badgeKeys(name).filter(function (k) { return !seen[k]; });
  }
  // Opening a store (or switching viewer inside an open drawer) shows what is new
  // once, then clears the handle badge, like the mailbox badge.
  function markSeen(name) {
    var seen = (ui.seen[seenKey(name)] = ui.seen[seenKey(name)] || {});
    ui.fresh = {};
    newKeys(name).forEach(function (k) { ui.fresh[k] = true; seen[k] = true; });
  }

  // ------------------------------------------------------------------ cards

  function workRow(g, r, withActs) {
    var it = g.items[r.item], ch = chain(g, it);
    var chainTxt = ch.length > 1 ? ' &middot; chain ' + esc(ch.slice().reverse().join(' > ')) : '';
    var acts = '';
    if (withActs && r.status !== 'done') {
      if (r.status !== 'doing') acts += btn('q-status', 'doing', { item: r.item, status: 'doing' });
      if (r.status !== 'blocked') acts += btn('q-status', 'blocked', { item: r.item, status: 'blocked' });
      acts += btn('status', 'Status...', { item: r.item });
      if (r.status !== 'awaiting') acts += btn('close', 'Close', { item: r.item });
      acts += btn('split', 'Split', { item: r.item });
    }
    return '<div class="row click"' + attr('item', r.item) + '><div class="r1">' + stChip(r.status) + (ui.fresh[r.id] ? '<span class="chip new">new</span>' : '') +
      titleRef(g, r.item) + rollupHtml(g, it) + '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(E.lastActivity(g, r.item))) + '</span></div>' +
      '<div class="r2">' + source(g, r) + ' &middot; ' + complText(r.completion) + chainTxt + (it.project ? ' &middot; ' + esc(it.project) : '') +
      (r.note ? '<br>&ldquo;' + esc(r.note) + '&rdquo; <span class="muted">(' + esc(r.noteBy) + ')</span>' : '') + '</div>' +
      (acts ? '<div class="r3">' + acts + '</div>' : '') + '</div>';
  }

  function cardMine(g, v) {
    var mine = Object.keys(g.asg).map(function (k) { return g.asg[k]; }).filter(function (r) { return r.to === v && !r.removed && !g.items[r.item].hidden; }).sort(byRank);
    var openR = mine.filter(function (r) { return r.status !== 'done'; }), done = mine.filter(function (r) { return r.status === 'done'; });
    var own = itemList(g).filter(function (i) { return i.author === v && !i.hidden && i.kind !== 'topic' && !i.request && !recsOf(g, i.id).length; });
    var h = openR.length ? openR.map(function (r) { return workRow(g, r, true); }).join('') : '<p class="muted">Nothing open is assigned to you.</p>';
    if (done.length) h += '<div class="sub">done</div>' + done.map(function (r) {
      return '<div class="row click"' + attr('item', r.item) + '><div class="r1">' + stChip('done') + titleRef(g, r.item) + '<span class="grow"></span><span class="muted">' + source(g, r) + '</span></div></div>';
    }).join('');
    if (own.length) h += '<div class="sub">created by me, not assigned</div>' + own.map(function (it) {
      return '<div class="row click"' + attr('item', it.id) + '><div class="r1">' + kindChip(it) + titleRef(g, it.id) + '<span class="grow"></span>' +
        btn('assign', 'Assign', { item: it.id }) + '</div></div>';
    }).join('');
    return card('My assignments', openR.length + ' open', h);
  }

  function cardRequests(g, v) {
    var reqs = itemList(g).filter(function (i) { return i.request && !i.hidden; });
    var inbox = reqs.filter(function (i) { return i.request.to === v; }), sent = reqs.filter(function (i) { return i.request.by === v; });
    if (!inbox.length && !sent.length) return '';
    var pend = inbox.filter(function (i) { return i.request.state === 'pending'; }), past = inbox.filter(function (i) { return i.request.state !== 'pending'; });
    function row(it, who, acts) {
      return '<div class="row click"' + attr('item', it.id) + '><div class="r1">' + stChip(itemStatus(g, it)) + (ui.fresh['req:' + it.id] ? '<span class="chip new">new</span>' : '') +
        titleRef(g, it.id) + '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(it.ts)) + '</span></div>' +
        '<div class="r2">' + who + (it.request.note ? ' &middot; &ldquo;' + esc(it.request.note) + '&rdquo;' : '') + '</div>' + (acts ? '<div class="r3">' + acts + '</div>' : '') + '</div>';
    }
    var h = pend.length ? pend.map(function (it) {
      return row(it, 'from <b>' + esc(nm(g, it.request.by)) + '</b> &middot; not counted in any roll-up until accepted',
        btn('req-accept', 'Accept', { item: it.id }, 'primary') + btn('note', 'Decline', { item: it.id, op: 'request.decline' }) + btn('open', 'Discuss', { item: it.id }));
    }).join('') : '<p class="muted">No pending requests to you.</p>';
    if (past.length) h += '<div class="sub">answered</div>' + past.map(function (it) { return row(it, 'from ' + esc(nm(g, it.request.by))); }).join('');
    if (sent.length) h += '<div class="sub">sent by me</div>' + sent.map(function (it) { return row(it, 'to <b>' + esc(nm(g, it.request.to)) + '</b>'); }).join('');
    return card('Received requests', pend.length + ' pending', h);
  }

  // Lead perspective: what each follower (down to 3 levels) works on, whoever assigned it.
  function cardMyTeam(g, v) {
    var R = reach(g.members, v, 3), ids = Object.keys(R);
    if (!ids.length) return '';
    var all = Object.keys(g.asg).map(function (k) { return g.asg[k]; });
    var totals = { open: 0, blocked: 0, pending: 0 };
    function person(id) {
      var recs = all.filter(function (r) { return r.to === id && !r.removed && !g.items[r.item].hidden; }).sort(byRank);
      var openR = recs.filter(function (r) { return r.status !== 'done'; }), done = recs.length - openR.length;
      var pend = itemList(g).filter(function (i) { return i.request && i.request.to === id && i.request.state === 'pending' && !i.hidden; });
      var last = recs.map(function (r) { return E.lastActivity(g, r.item); }).sort().pop() || '';
      var blocked = openR.filter(function (r) { return r.status === 'blocked'; }).length;
      totals.open += openR.length; totals.blocked += blocked; totals.pending += pend.length;
      var m = g.members[id];
      var h = '<div class="tm-person" style="--d:' + (R[id] - 1) + '"><div class="tm-head"><b>' + esc(m.name) + '</b><span class="nid">' + esc(id) + '</span>' +
        '<span class="chip">L' + R[id] + '</span>' + ((g.children[id] || []).length ? '<span class="chip">lead</span>' : '') +
        '<span class="muted">' + openR.length + ' open' + (done ? ', ' + done + ' done' : '') + '</span>' +
        (blocked ? '<span class="chip st-blocked">' + blocked + ' blocked</span>' : '') +
        (pend.length ? '<span class="chip st-pending">' + pend.length + ' pending request' + (pend.length > 1 ? 's' : '') + '</span>' : '') +
        '<span class="grow"></span><span class="muted ts" title="last update on their work">' + esc(fmtTs(last)) + '</span></div>';
      h += openR.map(function (r) {
        var it = g.items[r.item];
        return '<div class="tm-work click"' + attr('item', r.item) + '>' + stChip(r.status) + titleRef(g, r.item) + rollupHtml(g, it) +
          '<span class="muted">' + source(g, r) + (r.note ? ' &middot; &ldquo;' + esc(r.note) + '&rdquo;' : '') + '</span>' +
          '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(E.lastActivity(g, r.item))) + '</span></div>';
      }).join('');
      h += pend.map(function (it) {
        return '<div class="tm-work click pend"' + attr('item', it.id) + '>' + stChip('pending') + titleRef(g, it.id) +
          '<span class="muted">from ' + esc(nm(g, it.request.by)) + ', not accepted yet</span><span class="grow"></span><span class="muted ts">' + esc(fmtTs(it.ts)) + '</span></div>';
      }).join('');
      return h + '</div>' + (R[id] < 3 ? (g.children[id] || []).filter(function (k) { return k in R; }).map(person).join('') : '');
    }
    var body = (g.children[v] || []).filter(function (k) { return k in R; }).map(person).join('');
    return card('My team', ids.length + ' below you, ' + totals.open + ' open' + (totals.blocked ? ', ' + totals.blocked + ' blocked' : '') +
      (totals.pending ? ', ' + totals.pending + ' pending' : ''), '<p class="r2" style="margin:0 0 6px">Everything your followers (down to 3 levels) are working on, whoever assigned it.</p>' + body);
  }

  function cardGiven(g, v) {
    var items = itemList(g).filter(function (i) { return recsOf(g, i.id).some(function (r) { return r.by === v && r.to !== v; }); });
    var h = items.length ? items.map(function (it) {
      var as = recsOf(g, it.id).filter(function (r) { return r.by === v && r.to !== v; }).sort(byRank);
      return '<div class="row click"' + attr('item', it.id) + '><div class="r1">' + stChip(itemStatus(g, it)) + titleRef(g, it.id) + rollupHtml(g, it) + '</div>' +
        as.map(function (r) {
          return '<div class="assignee-line"><b>' + esc(nm(g, r.to)) + '</b>' + (r.depth ? '<span class="muted">L' + r.depth + '</span>' : '') + stChip(r.status) +
            (r.note ? '<span class="muted">&ldquo;' + esc(r.note) + '&rdquo;</span>' : '') +
            (r.status === 'awaiting' ? btn('confirm', 'Confirm', { item: r.item, who: r.to }, 'primary') + btn('note', 'Reject', { item: r.item, who: r.to, op: 'item.reject' }) : '') + '</div>';
        }).join('') + '</div>';
    }).join('') : '<div class="empty">You have not assigned anything to others.</div>';
    var waiting = Object.keys(g.asg).filter(function (k) { var r = g.asg[k]; return r.by === v && r.to !== v && !r.removed && r.status === 'awaiting'; }).length;
    return card('Assigned by me', items.length + (items.length === 1 ? ' item' : ' items') + (waiting ? ', ' + waiting + ' to confirm' : ''), h);
  }

  function cardTeam(st, g, v) {
    var probs = E.rolesProblems(st.roles), r = reach(g.members, v, 99), seen = {};
    var lead = (g.members[v].leads || [])[0], levels = {};
    Object.keys(r).forEach(function (k) { levels[r[k]] = (levels[r[k]] || 0) + 1; });
    var summary = 'Admins: ' + esc(Object.keys(g.admins).join(', ')) + '. You: ' + (lead ? 'lead <b>' + esc(lead) + '</b>' : 'no lead (root)') + ', ' +
      (Object.keys(r).length ? 'reach ' + Object.keys(levels).map(function (d) { return levels[d] + ' at L' + d; }).join(', ') : 'no followers') + '.';
    function node(id) {
      if (seen[id]) return '<li>' + esc(id) + ' (cycle guard)</li>';
      seen[id] = true;
      var m = g.members[id], kids = g.children[id] || [], rev = Object.keys(m.revoked);
      var chips = (id === v ? '<span class="chip you">you</span>' : '') + (g.admins[id] ? '<span class="chip admin">admin</span>' : '') +
        (id === lead ? '<span class="chip">your lead</span>' : '') + (r[id] ? '<span class="chip" title="levels below you">L' + r[id] + '</span>' : '') +
        (m.joined ? '<span class="chip" title="added by ' + esc(alias(m.joined)) + '">joined</span>' : '') +
        (rev.length ? '<span class="chip flag" title="revoked keys">revoked ' + esc(rev.join(', ')) + '</span>' : '');
      return '<li><div class="node' + (id === v ? ' me' : '') + '"><span class="nm">' + esc(m.name) + '</span><span class="nid">' + esc(id) + '</span>' + chips +
        '<span class="nid">' + esc(m.keys.join(', ')) + '</span></div>' + (kids.length ? '<ul>' + kids.map(node).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(g.members).filter(function (id) { var m = g.members[id], l = m.leads || []; return !m.removed && (!l.length || !g.members[l[0]] || g.members[l[0]].removed); });
    var h = '<p class="r2" style="margin:0 0 6px">' + summary + '</p>';
    if (probs.length) h += '<p class="err">roles file problems: ' + esc(probs.join('; ')) + '</p>';
    h += '<ul class="tree">' + roots.map(node).join('') + '</ul>';
    var gone = Object.keys(g.members).filter(function (id) { return g.members[id].removed; });
    if (gone.length) h += '<p class="r2">removed: ' + esc(gone.join(', ')) + '</p>';
    var lost = Object.keys(g.members).filter(function (id) { return !seen[id] && !g.members[id].removed; });
    if (lost.length) h += '<p class="err">unreachable from any root: ' + esc(lost.join(', ')) + '</p>';
    h += '<div class="toolbar" style="margin-top:8px">' + btn('admin', 'Admin: roles event', {}) +
      '<span class="muted">' + (g.admins[v] ? 'you are an admin' : 'flagged unless you are an admin') + '</span></div>';
    var nMem = Object.keys(g.members).filter(function (id) { return !g.members[id].removed; }).length;
    return card('Team', nMem + (nMem === 1 ? ' member' : ' members'), h);
  }

  function cardItems(g) {
    var rows = itemsInTreeOrder(g).filter(function (it) {
      if (ui.kind === 'request' ? !it.request : ui.kind !== 'all' && it.kind !== ui.kind) return false;
      var s = itemStatus(g, it);
      if (ui.status === 'hidden') return it.hidden;
      if (it.hidden) return false;
      if (ui.status === 'open') return s !== 'done' && s !== 'declined' && s !== 'returned';
      if (ui.status === 'done') return s === 'done';
      if (ui.status === 'blocked') return s === 'blocked' || rollup(g, it).blocked.length;
      return true;
    });
    function sel(id, val, list) {
      return '<select id="' + id + '">' + list.map(function (x) { return '<option' + (x === val ? ' selected' : '') + '>' + x + '</option>'; }).join('') + '</select>';
    }
    var h = '<div class="toolbar">kind ' + sel('f-kind', ui.kind, ['all', 'task', 'todo', 'topic', 'request']) +
      ' status ' + sel('f-status', ui.status, ['all', 'open', 'blocked', 'done', 'hidden']) + '</div>';
    h += rows.length ? rows.map(function (it) {
      var as = recsOf(g, it.id), d = itemDepth(g, it), nc = E.threadOf(g, it.id).filter(function (n) { return n.markers.length; }).length;
      var who = it.request ? '<span class="muted">&rarr; ' + esc(it.request.to) + '</span>' :
        as.length ? '<span class="muted">&rarr; ' + esc(as.length > 4 ? as.length + ' people' : as.map(function (a) { return a.to; }).join(', ')) + '</span>' : '';
      return '<div class="row click"' + attr('item', it.id) + ' style="padding-left:' + d * 14 + 'px"><div class="r1">' + (d ? '<span class="muted">&#8627;</span>' : '') +
        stChip(itemStatus(g, it)) + titleRef(g, it.id) + who + rollupHtml(g, it) + (nc ? '<span class="chip conc-c">concurrent ' + nc + '</span>' : '') +
        '<span class="grow"></span>' + kindChip(it) + '</div>' +
        '<div class="r2">' + esc(it.alias) + ' by ' + esc(nm(g, it.author)) + (it.project ? ' &middot; ' + esc(it.project) : '') + ' &middot; ' + esc(fmtTs(E.lastActivity(g, it.id))) + '</div></div>';
    }).join('') : '<div class="empty">No items match.</div>';
    return card('Items', rows.length + ' of ' + Object.keys(g.items).length, h);
  }

  function cardActivity(g) {
    var rows = g.log.filter(function (n) {
      return (!ui.flaggedOnly || n.status !== 'applied') && (!ui.concOnly || n.markers.length);
    }), cap = 50, shown = rows.slice(-cap).reverse();
    var flagged = g.log.filter(function (n) { return n.status !== 'applied'; }).length, conc = g.log.filter(function (n) { return n.markers.length; }).length;
    var h = '';
    if (ui.last) {
      h += '<div class="lastact">Last write (one commit): ' + ui.last.map(function (n) {
        return esc(n.alias) + ' ' + esc(n.type) + ' ' + (n.status === 'applied' ? '<span class="chip st-done">applied</span>' : '<span class="chip flag">' + esc(n.status) + '</span> ' + esc(n.reason));
      }).join('; ') + '</div>';
    }
    h += '<div class="toolbar"><label><input type="checkbox" id="flagged-only"' + (ui.flaggedOnly ? ' checked' : '') + '> flagged only</label>' +
      '<label><input type="checkbox" id="conc-only"' + (ui.concOnly ? ' checked' : '') + '> concurrent only</label>' +
      '<span class="grow"></span><span class="muted">newest first, ' + shown.length + ' of ' + rows.length + '</span></div>';
    h += shown.length ? '<ul class="plain">' + shown.map(function (n) { return evLine(g, n, false); }).join('') + '</ul>' : '<div class="empty">Nothing to show.</div>';
    return card('Activity', g.log.length + ' events, ' + flagged + ' flagged' + (conc ? ', ' + conc + ' concurrent' : ''), h);
  }

  function fmtCommits(a) { return '[\n' + a.map(function (e) { return '  ' + JSON.stringify(e); }).join(',\n') + '\n]'; }
  function fmtRoles(r) {
    return '{\n  "admins": ' + JSON.stringify(r.admins || []) + ',\n  "members": [\n' +
      r.members.map(function (m) { return '    ' + JSON.stringify(m); }).join(',\n') + '\n  ]\n}';
  }
  function cardData(st) {
    var owners = st.roles.members.map(function (m) { return m.id; });
    Object.keys(st.commits).forEach(function (k) { if (owners.indexOf(k) < 0) owners.push(k); });
    var h = '<details class="data" id="data-details"' + (ui.dataOpen ? ' open' : '') + '><summary>data (raw JSON, editable)</summary>' +
      '<p class="r2">Roles file (genesis) plus one commit list per member ref, and the commits still upstream. Edit, then apply; edits live in memory until reload.</p>' +
      '<div class="toolbar"><button class="btn mini primary" data-act="apply">apply</button><button class="btn mini" data-act="reset">reset store to fixtures</button></div>' +
      '<div class="sub">roles file</div><textarea id="raw-roles" rows="12" spellcheck="false">' + esc(fmtRoles(st.roles)) + '</textarea>';
    owners.forEach(function (o) {
      var cs = st.commits[o] || [];
      h += '<details' + attr('owner', o) + (ui.openLogs[o] ? ' open' : '') + '><summary>ref of ' + esc(o) + ' (' + cs.length + ' commits)</summary>' +
        '<textarea' + attr('owner', o) + ' rows="' + Math.min(18, cs.length + 3) + '" spellcheck="false">' + esc(fmtCommits(cs)) + '</textarea></details>';
    });
    h += '<details' + attr('owner', '@late') + (ui.openLogs['@late'] ? ' open' : '') + '><summary>upstream, not received yet (' + nLate(st) + ' commits)</summary>' +
      '<textarea id="raw-late" rows="8" spellcheck="false">' + esc(JSON.stringify(st.late, null, 1)) + '</textarea></details>';
    return card('Data', 'debug', h + '</details>');
  }

  // --------------------------------------------------------------- sync state

  function nLate(st) { return Object.keys(st.late || {}).reduce(function (s, k) { return s + st.late[k].length; }, 0); }
  function unpushed(st, v) { return (st.commits[v] || []).filter(function (c) { return c._local; }).length; }
  function syncHtml(name) {
    var st = stores[name], s = ui.sync[name], v = viewerOf(name), n = unpushed(st, v), up = nLate(st);
    var remote = st.def.sync && st.def.sync.remote;
    var h = '<div class="sync' + (remote && s.online ? '' : ' off') + '">';
    if (!remote) h += '<span class="sdot"></span><b>no remote</b> &middot; local only (same path as a long offline)';
    else h += '<span class="sdot"></span><b>' + (s.online ? 'online' : 'offline') + '</b> &middot; last synced ' + esc(fmtTs(s.lastSynced)) + (s.online ? '' : ' &middot; will retry');
    h += ' &middot; <span' + (n ? ' class="unp"' : '') + '>' + n + ' of my writes not yet pushed</span>';
    if (remote && up) h += ' &middot; <span class="muted" title="mock: commits other members pushed while this client was not looking">' + up + ' upstream commits not fetched</span>';
    if (remote) h += btn('sync', 'Sync now', {}) + btn(s.online ? 'go-offline' : 'go-online', s.online ? 'Go offline' : 'Come back online', {}, s.online ? '' : 'primary');
    return h + '</div>';
  }
  function nowTs(st) {
    var m = '';
    Object.keys(st.commits).forEach(function (k) { st.commits[k].forEach(function (c) { if (String(c.ts) > m) m = String(c.ts); }); });
    var now = new Date().toISOString().slice(0, 19) + 'Z';
    return now > m ? now : new Date(Date.parse(m) + 60000).toISOString().slice(0, 19) + 'Z';
  }
  // Sync: push my local commits, fetch what is upstream (the fixture's late commits).
  function doSync(name) {
    var st = stores[name], s = ui.sync[name];
    if (!st.def.sync.remote) return toast('No remote configured: writes stay local, exactly like a long offline.');
    if (!s.online) return toast('Offline: the sync is retried when you come back online (not an error).');
    var pushed = 0, got = 0;
    Object.keys(st.commits).forEach(function (k) { st.commits[k].forEach(function (c) { if (c._local) { delete c._local; pushed++; } }); });
    Object.keys(st.late).forEach(function (o) {
      st.late[o].forEach(function (c) { c._received = 'late'; (st.commits[o] = st.commits[o] || []).push(c); got++; });
    });
    st.late = {};
    s.lastSynced = nowTs(st);
    render();
    toast('Synced: pushed ' + pushed + ', fetched ' + got + ' commit' + (got === 1 ? '' : 's') + (got ? '. Late arrivals are checked against what their authors had seen; no earlier verdict changes.' : '.'));
  }

  function drawerHtml(st, g, v) {
    var flagged = g.log.filter(function (n) { return n.status !== 'applied'; }).length;
    var nMem = Object.keys(g.members).filter(function (id) { return !g.members[id].removed; }).length;
    return '<div class="dr-title"><b>' + esc(st.def.label) + '</b><span>' + nMem + ' members, ' + g.log.length + ' events from ' +
      Object.keys(st.commits).reduce(function (s, k) { return s + st.commits[k].length; }, 0) + ' commits, ' + flagged + ' flagged</span><span class="grow"></span>' +
      '<button class="btn primary" data-act="create"><svg class="i"><use href="#i-plus"/></svg>New</button></div>' + syncHtml(st.def.name) +
      '<div class="dr-cols"><div class="dr-col">' + cardMine(g, v) + cardRequests(g, v) + cardGiven(g, v) + '</div>' +
      '<div class="dr-col">' + cardMyTeam(g, v) + cardItems(g) + '</div>' +
      '<div class="dr-col">' + cardTeam(st, g, v) + cardActivity(g) + cardData(st) + '</div></div>';
  }

  // ------------------------------------------------------------ thread sheet

  function commentHtml(g, n, v, it) {
    var c = g.comments[n.id], p = n.p, body = c ? c.body : p.body, reply = '';
    if (p.reply_to) {
      var q = g.comments[p.reply_to] || (g.byId[p.reply_to] && { author: g.byId[p.reply_to].author, body: g.byId[p.reply_to].p.body, hidden: false });
      reply = '<div class="quote">&#8617; ' + esc(alias(p.reply_to)) + ' ' + (q ? '<b>' + esc(q.author) + '</b>: ' + (q.hidden ? '<i>hidden comment</i>' : esc(String(q.body).slice(0, 90))) : '') + '</div>';
    }
    var acts = '';
    if (n.status === 'applied' && c && !c.hidden && !it.hidden) {
      acts += btn('reply', 'Reply', { cid: n.id });
      if (c.author === v) acts += btn('cedit', 'Edit', { cid: n.id }) + btn('chide', 'Hide', { cid: n.id });
    }
    var txt = c && c.hidden ? '<div class="hiddenc">comment hidden by its author (' + esc(alias(c.hiddenBy.id)) + ')<details><summary>history keeps the text</summary>' + esc(body) + '</details></div>' :
      '<div class="ctext">' + esc(body) + '</div>';
    return '<li class="cmt' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (c && c.author === v ? ' mine' : '') + '">' +
      '<div class="ch"><b>' + esc(nm(g, n.author)) + '</b>' + originChip(n.origin) + '<span class="when">' + esc(fmtTs(n.ts)) + '</span><span class="al">' + esc(n.alias) + '</span>' +
      (c && c.edited ? '<span class="chip">edited</span>' : '') + flags(n) + '<span class="grow"></span>' + acts + '</div>' + reply + txt +
      (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(g, n) + '</li>';
  }

  function sheetHtml(g, v, it) {
    var recs = recsOf(g, it.id, true), active = recs.filter(function (r) { return !r.removed; }), mineR = active.filter(function (r) { return r.to === v; })[0];
    var req = it.request, lead = req && ((g.members[req.to] || {}).leads || [])[0];
    var acts = '';
    if (req && req.state === 'pending' && req.to === v) acts += btn('req-accept', 'Accept request', { item: it.id }, 'primary') + btn('note', 'Decline', { item: it.id, op: 'request.decline' });
    if (req && req.state === 'accepted') acts += btn('note', 'Return (lead of ' + esc(req.to) + ')', { item: it.id, op: 'request.return' }, lead === v ? '' : 'ghost');
    if (it.author === v && !it.hidden) acts += btn('edit', 'Edit', { item: it.id });
    if (it.author === v && it.kind !== 'topic' && !req && !it.hidden) acts += btn('assign', 'Assign', { item: it.id });
    if ((mineR || it.author === v) && it.kind !== 'topic' && !it.hidden) acts += btn('split', 'Split into subtask', { item: it.id });
    if (it.author === v && !it.hidden) acts += btn('ihide', 'Hide item', { item: it.id });

    var h = '<div class="sh-top"><div class="sh-head"><div>' + kindChip(it) + '</div><h2>' + esc(it.title) + '</h2>' +
      '<button class="btn mini" data-act="sheet-close" title="close (Esc)"><svg class="i"><use href="#i-x"/></svg></button></div>';
    h += '<dl class="sh-meta"><dt>id</dt><dd><code>' + esc(it.alias) + '</code> <span class="muted">(' + esc(it.id) + ')</span></dd>' +
      '<dt>status</dt><dd>' + (it.kind === 'topic' ? '<span class="muted">topic: discussion only</span>' : stChip(itemStatus(g, it)) + ' ' + rollupHtml(g, it)) + '</dd>' +
      (req ? '<dt>request</dt><dd>' + esc(req.by) + ' &rarr; <b>' + esc(req.to) + '</b> ' + (req.state === 'accepted' ? '<span class="chip st-doing">accepted</span>' : stChip(req.state)) + '</dd>' : '') +
      '<dt>creator</dt><dd>' + esc(nm(g, it.author)) + (g.admins[it.author] ? ' <span class="chip admin">admin</span>' : '') + '</dd>' +
      '<dt>project</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>parent</dt><dd>' + (it.parent ? itemRef(it.parent) + ' ' + esc((g.items[it.parent] || {}).title || '') : '<span class="muted">-</span>') + '</dd>' +
      '<dt>created</dt><dd>' + esc(fmtTs(it.ts)) + (it.edited ? ' &middot; edited ' + esc(fmtTs(it.edited.ts)) : '') + '</dd></dl>';
    h += it.body ? '<div class="sh-body">' + esc(it.body) + '</div>' : '';
    h += '<div class="sh-actions">' + (acts || '<span class="muted">No item actions for ' + esc(nm(g, v)) + '.</span>') + '</div>';

    if (it.kind !== 'topic') {
      h += '<h3>Assignees</h3>';
      if (!recs.length) h += '<div class="empty">' + (req ? 'Request ' + esc(req.state) + '; it becomes ' + esc(req.to) + '\'s own assignment on acceptance.' : 'Not assigned.') + '</div>';
      else h += '<table class="atable">' + recs.map(function (r) {
        var b = '';
        if (!r.removed) {
          if (r.by === v && r.status === 'awaiting') b += btn('confirm', 'Confirm', { item: r.item, who: r.to }, 'primary') + btn('note', 'Reject', { item: r.item, who: r.to, op: 'item.reject' });
          if ((r.by === v || r.to === v) && r.status === 'done') b += btn('note', 'Reopen', { item: r.item, who: r.to, op: 'item.reopen' });
          if (r.by === v && !r.request) b += btn('unassign', 'Unassign', { item: r.item, who: r.to });
        }
        return '<tr class="' + (r.removed ? 'gone' : '') + '"><td><b>' + esc(nm(g, r.to)) + '</b><br><span class="muted">' + source(g, r) + '</span></td><td>' +
          (r.removed ? '<span class="chip">' + (r.removed.type === 'request.return' ? 'returned' : 'unassigned') + ' ' + esc(alias(r.removed.id)) + '</span>' : stChip(r.status)) +
          '<br><span class="muted">' + complText(r.completion) + '</span></td><td>' + (r.note ? '&ldquo;' + esc(r.note) + '&rdquo;' : '<span class="muted">no note</span>') +
          '<div class="tb">' + b + '</div></td></tr>';
      }).join('') + '</table>';
    }
    var kids = subtasksOf(g, it.id, true);
    if (kids.length) {
      h += '<h3>Subtask threads</h3><div class="stree">' + kids.map(function (k) {
        return '<div class="click"' + attr('item', k.id) + '>' + stChip(itemStatus(g, k)) + titleRef(g, k.id) + ' <span class="muted">' + esc(k.author) +
          (recsOf(g, k.id).length ? ' &rarr; ' + esc(recsOf(g, k.id).map(function (a) { return a.to; }).join(', ')) : '') + '</span>' + rollupHtml(g, k) + kindChip(k) + '</div>';
      }).join('') + '</div>';
    }
    var evs = E.threadOf(g, it.id);
    h += '<h3>Thread (' + evs.length + ' events)</h3></div><ul class="timeline">' + evs.map(function (n) {
      if (n.type === 'comment.add' && n.thread === it.id) return commentHtml(g, n, v, it);
      return evLine(g, n, n.parentThread === it.id ? 'parent' : true);
    }).join('') + '</ul>';

    // composer: comment (optionally a quote reply) plus quick status actions on my own assignment
    var rt = ui.replyTo && g.comments[ui.replyTo] && g.comments[ui.replyTo].item === it.id ? g.comments[ui.replyTo] : null;
    var quick = '';
    if (mineR && mineR.status !== 'done') {
      quick = '<span class="muted">my status ' + stChip(mineR.status) + ' set:</span>' +
        ['todo', 'doing', 'blocked'].filter(function (s) { return s !== mineR.status; }).map(function (s) { return btn('q-status', s, { item: it.id, status: s, note: 1 }); }).join('') +
        (mineR.status !== 'awaiting' ? btn('q-close', 'Close', { item: it.id }) : '') + '<span class="muted">(text below becomes the note)</span>';
    }
    h += '<div class="composer2">' + (it.hidden ? '<span class="muted">thread hidden; comments are flagged</span>' : '') +
      (rt ? '<div class="replying">replying to ' + esc(rt.alias) + ' <b>' + esc(rt.author) + '</b>: ' + esc(String(rt.body).slice(0, 70)) + ' ' + btn('reply-x', 'x', {}) + '</div>' : '') +
      (quick ? '<div class="quick">' + quick + '</div>' : '') +
      '<div class="crow"><textarea id="cbox" rows="2" placeholder="Comment as ' + esc(nm(g, v)) + '..."></textarea><div class="ccol">' +
      '<select id="corigin" title="origin"><option value="human">human</option><option value="agent">agent</option><option value="agent-auto">agent-auto</option></select>' +
      btn('comment', 'Post', { item: it.id }, 'primary') + '</div></div></div>';
    return h;
  }

  // --------------------------------------------------------- dialog forms

  function curr() { var st = stores[ui.store]; return { st: st, g: replay(st), v: viewerOf(ui.store) }; }
  function optsHtml(list, empty, sel) {
    if (!list.length) return '<option value="">(' + empty + ')</option>';
    return list.map(function (x) { return '<option value="' + esc(x[0]) + '"' + (x[0] === sel ? ' selected' : '') + '>' + esc(x[1]) + '</option>'; }).join('');
  }
  function memberOpts(g, sel, filter) {
    return optsHtml(Object.keys(g.members).filter(function (id) { return !filter || filter(g.members[id]); }).map(function (id) { return [id, g.members[id].name + ' (' + id + ')']; }), 'nobody', sel);
  }
  function peopleHtml(g, v) {
    var r = reach(g.members, v, 99);
    var ids = Object.keys(r).sort(function (a, b) { return r[a] - r[b] || (a < b ? -1 : 1); });
    return '<label><input type="checkbox" name="people" value="' + esc(v) + '" data-level="0"> myself</label>' + ids.map(function (id) {
      return '<label><input type="checkbox" name="people" value="' + esc(id) + '" data-level="' + r[id] + '"> ' + esc(nm(g, id)) + ' <span class="muted">L' + r[id] + '</span></label>';
    }).join('');
  }
  function levelSel() { return '<select name="levels"><option>1</option><option>2</option><option>3</option></select>'; }
  function complRadios() {
    return '<div class="opts"><label><input type="radio" name="completion" value="assignee" checked> assignee closes</label>' +
      '<label><input type="radio" name="completion" value="assigner"> assigner confirms</label></div>';
  }
  function originSel() { return '<select name="origin"><option value="human">human</option><option value="agent">agent (on request)</option><option value="agent-auto">agent-auto (unprompted)</option></select>'; }
  function fld(label, inner) { return '<div class="fld"><span>' + label + '</span>' + inner + '</div>'; }
  var ROLE_TYPES = ['roles.add_member', 'roles.set_lead', 'roles.add_admin', 'roles.remove_admin', 'roles.add_key', 'roles.revoke_key', 'roles.remove_member'];

  function dialogHtml(kind, d) {
    var c = curr(), g = c.g, v = c.v, it = d.item && g.items[d.item];
    var mineOpen = Object.keys(g.asg).map(function (k) { return g.asg[k]; }).filter(function (r) { return r.to === v && !r.removed && r.status !== 'done'; })
      .map(function (r) { return [r.item, alias(r.item) + ' ' + g.items[r.item].title]; });
    var T;
    switch (kind) {
      case 'create':
        T = ['New item', 'One commit in ' + nm(g, v) + '\'s ref: item.create, plus item.assign to yourself when checked.',
          fld('kind', '<select name="kind"><option>task</option><option>todo</option><option>topic</option></select>') +
          fld('title', '<input type="text" name="title" required>') + fld('body', '<textarea name="body" rows="3"></textarea>') +
          fld('project reference (free text, optional)', '<input type="text" name="project">') +
          fld('request to (optional: anyone in the store; they accept or decline)', '<select name="request"><option value="">(not a request)</option>' + memberOpts(g, '', function (m) { return m.id !== v && !m.removed; }) + '</select>') +
          '<label class="fld inline"><input type="checkbox" name="self" checked> assign to myself (ignored for topics and requests)</label>', 'Create'];
        break;
      case 'edit':
        T = ['Edit ' + it.alias, 'item.edit by the creator; the thread keeps the old text.',
          fld('title', '<input type="text" name="title" value="' + esc(it.title) + '">') + fld('body', '<textarea name="body" rows="4">' + esc(it.body) + '</textarea>') +
          fld('project', '<input type="text" name="project" value="' + esc(it.project) + '">'), 'Save'];
        break;
      case 'assign':
        T = ['Assign ' + it.alias, 'item.assign. Only the creator assigns; targets within N levels below you (or yourself). "Everyone" stays live: later followers inherit it.',
          fld('who', '<div class="opts"><label><input type="radio" name="mode" value="all" checked> everyone within N levels</label><label><input type="radio" name="mode" value="chosen"> chosen people</label></div>') +
          fld('N levels below me', levelSel()) + fld('completion', complRadios()) +
          fld('chosen people', '<div class="people" id="people">' + peopleHtml(g, v) + '</div>') + '<div class="hint" id="hint"></div>', 'Assign'];
        break;
      case 'split':
        T = ['Split ' + it.alias + ' into a subtask', 'One commit: item.create (parent ' + it.alias + ') plus item.assign. Delegating down always goes through a subtask under your name.',
          fld('subtask title', '<input type="text" name="title" required>') + fld('N levels below me', levelSel()) + fld('completion', complRadios()) +
          fld('assign to', '<div class="people" id="people">' + peopleHtml(g, v) + '</div>') + '<div class="hint" id="hint"></div>', 'Split'];
        break;
      case 'status':
        T = ['Set status', 'item.status on your own assignment (done goes through Close).',
          fld('item', '<select name="target">' + optsHtml(mineOpen, 'nothing open assigned to you', d.item) + '</select>') +
          fld('status', '<select name="status"><option>doing</option><option>blocked</option><option>todo</option></select>') +
          fld('note (optional)', '<input type="text" name="note">') + fld('origin', originSel()), 'Set'];
        break;
      case 'close':
        T = ['Close', 'item.close. With "assigner confirms" it waits for the assigner.',
          fld('item', '<select name="target">' + optsHtml(mineOpen, 'nothing open assigned to you', d.item) + '</select>') +
          fld('note (optional)', '<input type="text" name="note">') + fld('origin', originSel()), 'Close'];
        break;
      case 'note': {
        var L = { 'item.reject': ['Reject ' + (d.who || '') + '\'s close', 'Back to doing; the note says why.'],
          'item.reopen': ['Reopen ' + (d.who || '') + '\'s assignment', 'A done assignment goes back to doing.'],
          'request.decline': ['Decline request', 'Closes the request as declined. Questions belong in the thread as comments.'],
          'request.return': ['Return accepted request', 'Only the recipient\'s direct lead may return it; it cancels the resulting assignment.'] }[d.op];
        T = [L[0] + ' (' + it.alias + ')', d.op + ': ' + L[1], fld('note', '<input type="text" name="note">') + '<input type="hidden" name="op" value="' + esc(d.op) + '">' +
          '<input type="hidden" name="who" value="' + esc(d.who || '') + '">', 'Send'];
        break;
      }
      case 'cedit': {
        var cm = g.comments[d.cid];
        T = ['Edit comment ' + cm.alias, 'comment.edit; only its author edits it.', fld('text', '<textarea name="body" rows="4">' + esc(cm.body) + '</textarea>'), 'Save'];
        break;
      }
      case 'admin':
        T = ['Roles event', 'Admin-only. Appended to ' + nm(g, v) + '\'s own ref; replay applies it to the roles file in causal order.',
          fld('type', '<select name="rtype">' + ROLE_TYPES.map(function (t) { return '<option>' + t + '</option>'; }).join('') + '</select>') +
          fld('member (existing)', '<select name="member">' + memberOpts(g, '') + '</select>') +
          fld('lead (set_lead, add_member)', '<select name="lead"><option value="">(none: root)</option>' + memberOpts(g, v) + '</select>') +
          fld('key (add_key, revoke_key, add_member)', '<input type="text" name="key" placeholder="e.g. jon-k3">') +
          fld('new member id and name (add_member)', '<div class="fld inline"><input type="text" name="nid" placeholder="id"><input type="text" name="nname" placeholder="name"></div>'), 'Append'];
        break;
    }
    return '<form' + attr('kind', kind) + attr('fitem', d.item || '') + attr('fcid', d.cid || '') + '><h2>' + T[0] + '</h2><p>' + T[1] + '</p>' + T[2] +
      '<div class="btns"><button type="button" class="btn" data-act="dlg-cancel">Cancel</button><button type="submit" class="btn primary">' + T[3] + '</button></div></form>';
  }

  function updateHint(form) {
    var hint = form.querySelector('#hint'), people = form.querySelector('#people');
    if (!hint) return;
    var c = curr(), g = c.g, v = c.v, kind = form.getAttribute('data-kind');
    var n = Number(form.elements.levels.value), mode = kind === 'split' ? 'chosen' : form.elements.mode.value;
    var r = reach(g.members, v, n), msg, warn = false;
    var chosen = Array.prototype.map.call(form.querySelectorAll('input[name="people"]:checked'), function (e) { return e.value; });
    Array.prototype.forEach.call(form.querySelectorAll('input[name="people"]'), function (e) { e.disabled = mode === 'all'; });
    people.classList.toggle('off', mode === 'all');
    if (mode === 'all') {
      var ids = Object.keys(r);
      msg = ids.length ? 'Assigns to ' + ids.length + ' people within L' + n + ' now, and to whoever joins that range later.' : 'Nobody is within L' + n + ' below you; the event will be flagged.';
      warn = !ids.length;
    } else if (!chosen.length) {
      msg = 'Choose at least one person; an empty selection is flagged.'; warn = true;
    } else {
      var beyond = chosen.filter(function (x) { return x !== v && !(x in r); });
      msg = 'Assigns to ' + chosen.map(function (i) { return i === v ? 'yourself' : nm(g, i); }).join(', ') + '.';
      if (beyond.length) { msg += ' ' + beyond.join(', ') + ' is beyond L' + n + '; the event will be flagged.'; warn = true; }
    }
    hint.textContent = msg; hint.classList.toggle('warn', warn);
  }

  function openDialog(kind, d) {
    var dl = $('dlg');
    dl.innerHTML = dialogHtml(kind, d || {});
    var f = dl.querySelector('form');
    updateHint(f);
    if (!dl.open) dl.showModal();
    var first = f.querySelector('input[type="text"], textarea, select');
    if (first) first.focus();
  }
  function closeDialog() { var d = $('dlg'); if (d.open) d.close(); }

  // ---------------------------------------------------------------- actions

  var toastTimer = null;
  function toast(html) {
    var t = $('toast');
    t.innerHTML = html; t.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.hidden = true; }, 6000);
  }

  function applyRaw() {
    var st = stores[ui.store], ta = $('raw-roles');
    if (!ta) return true;
    var roles, commits = {}, late;
    try { roles = JSON.parse(ta.value); } catch (e) { toast('JSON error in roles file: ' + esc(e.message)); return false; }
    if (!roles || !Array.isArray(roles.members)) { toast('roles file needs a "members" list'); return false; }
    var tas = document.querySelectorAll('textarea[data-owner]');
    for (var i = 0; i < tas.length; i++) {
      var o = tas[i].getAttribute('data-owner'), a;
      try { a = JSON.parse(tas[i].value); } catch (e2) { toast('JSON error in ref of ' + esc(o) + ': ' + esc(e2.message)); return false; }
      if (!Array.isArray(a)) { toast('ref of ' + esc(o) + ' must be a JSON list'); return false; }
      commits[o] = a;
    }
    try { late = JSON.parse($('raw-late').value); } catch (e3) { toast('JSON error in upstream commits: ' + esc(e3.message)); return false; }
    st.roles = roles; st.commits = commits; st.late = late || {};
    return true;
  }

  // One write = one commit in the viewer's ref holding one or more events. Events
  // are not pre-validated, so a form can deliberately produce a flagged event.
  function commit(events) {
    var st = stores[ui.store], g = replay(st), v = viewerOf(ui.store);
    var ref = (st.commits[v] = st.commits[v] || []), seq = g.heads[v] || 0, seen = {};
    Object.keys(g.heads).sort().forEach(function (m) { if (m !== v) seen[m] = g.heads[m]; });
    var m = g.members[v];
    var c = { author: v, ts: nowTs(st), signer: m && m.keys[0] || (m && Object.keys(m.revoked)[0]) || 'unsigned', seen: seen, events: [] };
    events.forEach(function (e) {
      var ev = { seq: ++seq, type: e[0] };
      if (e[2] && e[2] !== 'human') ev.origin = e[2];
      ev.payload = e[1];
      c.events.push(ev);
    });
    var s = ui.sync[ui.store];
    if (!st.def.sync.remote || !s.online) c._local = true;
    ref.push(c);
    var g2 = replay(st);
    ui.last = c.events.map(function (e) { return g2.byId[v + '#' + e.seq]; });
    closeDialog();
    render();
    toast(ui.last.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ': ' + (n.status === 'applied' ? 'applied' : '<span class="tf">' + esc(n.status.toUpperCase()) + '</span> ' + esc(n.reason));
    }).join('<br>') + (c._local ? '<br><span class="muted">held locally (not pushed)</span>' : ''));
    return ui.last;
  }
  function nextId() { var v = viewerOf(ui.store); return v + '#' + ((replay(stores[ui.store]).heads[v] || 0) + 1); }

  function submit(form) {
    if (!applyRaw()) return;
    var kind = form.getAttribute('data-kind'), item = form.getAttribute('data-fitem'), el = form.elements, v = viewerOf(ui.store);
    function val(n) { return el[n] ? el[n].value : ''; }
    function people() { return Array.prototype.map.call(form.querySelectorAll('input[name="people"]:checked'), function (e) { return e.value; }); }
    var evs = [], open = null;
    switch (kind) {
      case 'create': {
        var pl = { kind: val('kind'), title: val('title') };
        if (val('body')) pl.body = val('body');
        if (val('project')) pl.project = val('project');
        if (val('request')) pl.request = val('request');
        open = nextId();
        evs.push(['item.create', pl]);
        if (el.self.checked && pl.kind !== 'topic' && !pl.request) evs.push(['item.assign', { item: open, mode: 'chosen', people: [v], completion: 'assignee' }]);
        break;
      }
      case 'edit': {
        var it = curr().g.items[item], ep = { item: item };
        ['title', 'body', 'project'].forEach(function (f) { if (val(f) !== it[f]) ep[f] = val(f); });
        evs.push(['item.edit', ep]);
        break;
      }
      case 'assign': {
        var ap = { item: item, mode: val('mode'), levels: Number(val('levels')), completion: val('completion') };
        if (ap.mode === 'chosen') ap.people = people();
        evs.push(['item.assign', ap]);
        break;
      }
      case 'split':
        open = nextId();
        evs.push(['item.create', { kind: 'task', title: val('title'), parent: item }]);
        evs.push(['item.assign', { item: open, mode: 'chosen', levels: Number(val('levels')), people: people(), completion: val('completion') }]);
        break;
      case 'status': {
        if (!val('target')) return toast('Nothing to set.');
        var sp = { item: val('target'), status: val('status') };
        if (val('note')) sp.note = val('note');
        evs.push(['item.status', sp, val('origin')]);
        break;
      }
      case 'close': {
        if (!val('target')) return toast('Nothing to close.');
        var cp = { item: val('target') };
        if (val('note')) cp.note = val('note');
        evs.push(['item.close', cp, val('origin')]);
        break;
      }
      case 'note': {
        var np = { item: item };
        if (val('who')) np.assignee = val('who');
        if (val('note')) np.note = val('note');
        evs.push([val('op'), np]);
        break;
      }
      case 'cedit':
        evs.push(['comment.edit', { comment: form.getAttribute('data-fcid'), body: val('body') }]);
        break;
      case 'admin': {
        var t = val('rtype'), rp = {};
        if (t === 'roles.add_member') rp.member = { id: val('nid'), name: val('nname') || val('nid'), keys: val('key') ? [val('key')] : [], leads: val('lead') ? [val('lead')] : [] };
        else {
          rp.member = val('member');
          if (t === 'roles.set_lead') rp.leads = val('lead') ? [val('lead')] : [];
          if (t === 'roles.add_key' || t === 'roles.revoke_key') rp.key = val('key');
        }
        evs.push([t, rp]);
        break;
      }
    }
    commit(evs);
    if (open && (kind === 'create' || kind === 'split')) showItem(open);
  }

  function quick(k, a) {
    if (!applyRaw()) return;
    var item = a.getAttribute('data-item'), who = a.getAttribute('data-who'), cid = a.getAttribute('data-cid');
    var box = $('cbox'), note = box && a.getAttribute('data-note') ? box.value.trim() : '';
    var origin = $('corigin') ? $('corigin').value : 'human';
    switch (k) {
      case 'q-status': {
        var p = { item: item, status: a.getAttribute('data-status') };
        if (note) p.note = note;
        commit([['item.status', p, a.getAttribute('data-note') ? origin : 'human']]);
        break;
      }
      case 'q-close': commit([['item.close', note ? { item: item, note: note } : { item: item }, origin]]); break;
      case 'confirm': commit([['item.confirm', { item: item, assignee: who }]]); break;
      case 'unassign': commit([['item.unassign', { item: item, assignee: who }]]); break;
      case 'req-accept': commit([['request.accept', { item: item }]]); break;
      case 'ihide': commit([['item.hide', { item: item }]]); break;
      case 'chide': commit([['comment.hide', { comment: cid }]]); break;
      case 'comment': {
        if (!box || !box.value.trim()) return toast('Write a comment first.');
        var cp = { item: item, body: box.value.trim() };
        if (ui.replyTo) cp.reply_to = ui.replyTo;
        ui.replyTo = null;
        commit([['comment.add', cp, origin]]);
        break;
      }
    }
  }

  // ------------------------------------------------------------------ render

  function measure() {
    // handle width first: it pads the header, which can change how the header wraps and so where it ends
    var d = document.documentElement.style;
    d.setProperty('--drhw', Math.ceil($('drhandles').getBoundingClientRect().width) + 'px');
    d.setProperty('--drtop', Math.max(0, Math.round($('top').getBoundingClientRect().bottom)) + 'px');
  }

  function renderHandles() {
    order.forEach(function (name) {
      var b = $('handle-' + name), n = newKeys(name).length, badge = b.querySelector('.dr-badge'), s = ui.sync[name], st = stores[name];
      b.className = 'dr-handle' + (ui.store === name ? ' on' : '') + (ui.open === name ? ' open' : '');
      b.setAttribute('aria-expanded', String(ui.open === name));
      badge.hidden = !n; badge.textContent = n;
      b.querySelector('.hoff').hidden = !!(st.def.sync.remote && s.online);
      b.title = st.def.label + (n ? ': ' + n + ' new (assignments or requests)' : '') + (st.def.sync.remote ? (s.online ? '' : ' - offline') : ' - no remote');
    });
  }

  function render() {
    order.forEach(function (n) {
      gs[n] = replay(stores[n]);
      if (!gs[n].members[ui.viewers[n]]) ui.viewers[n] = Object.keys(gs[n].members)[0];
    });
    var st = stores[ui.store], g = gs[ui.store], v = viewerOf(ui.store);
    if (ui.item && !g.items[ui.item]) { ui.item = null; ui.sheet = false; }

    $('viewer').innerHTML = Object.keys(g.members).map(function (k) {
      var m = g.members[k], r = reach(g.members, k, 3), role = (m.leads || []).length ? (Object.keys(r).length ? 'lead' : 'member') : 'root';
      return '<option value="' + esc(k) + '"' + (k === v ? ' selected' : '') + '>' + esc(m.name + ' (' + k + ', ' + role + ')') + '</option>';
    }).join('');
    $('vstore').textContent = ui.store;
    renderHandles();
    $('drawer').classList.toggle('open', !!ui.open);

    var body = $('drbody'), sb = $('sheet').firstElementChild, top1 = body.scrollTop, top2 = sb.scrollTop;
    var draft = $('cbox') ? $('cbox').value : '';
    body.innerHTML = drawerHtml(st, g, v);
    body.scrollTop = top1;
    var open = ui.sheet && ui.item;
    $('sheet').classList.toggle('open', !!open);
    $('sheet').setAttribute('aria-hidden', String(!open));
    if (open) {
      sb.innerHTML = sheetHtml(g, v, g.items[ui.item]);
      if (!ui.itemChanged && !ui.clearDraft && $('cbox')) $('cbox').value = draft;
      sb.scrollTop = ui.itemChanged ? 0 : top2;
    }
    ui.itemChanged = false; ui.clearDraft = false;
    measure();
  }

  function resetStore(name) {
    var d = stores[name].def;
    stores[name].roles = clone(d.roles); stores[name].commits = clone(d.commits); stores[name].late = clone(d.late || {});
    ui.sync[name] = { online: true, lastSynced: d.sync && d.sync.syncedAt };
  }

  function openStore(name) {
    if (ui.open === name) { ui.open = null; render(); return; }
    if (ui.store !== name) { ui.item = null; ui.sheet = false; ui.last = null; ui.itemChanged = true; $('drbody').scrollTop = 0; }
    ui.store = name; ui.open = name;
    markSeen(name);
    render();
  }
  function showItem(id) { ui.item = id; ui.sheet = true; ui.itemChanged = true; ui.replyTo = null; render(); }

  var QUICK = { 'q-status': 1, 'q-close': 1, confirm: 1, unassign: 1, 'req-accept': 1, ihide: 1, chide: 1, comment: 1 };

  function init() {
    window.TRACKER_FIXTURES.stores.forEach(function (s) {
      stores[s.name] = { def: s }; order.push(s.name); resetStore(s.name); ui.viewers[s.name] = s.defaultViewer;
    });
    ui.store = order[0];

    // handles: the panels handle is a static stub; each store gets its own, stacking leftward
    var hh = '<button class="dr-handle stub" data-handle="panels" title="side panels (a stub; not part of this mockup)"><svg class="i"><use href="#i-panels"/></svg><span class="hl">panels</span><svg class="i"><use href="#i-chev"/></svg></button>';
    order.forEach(function (n) {
      hh += '<button class="dr-handle" id="handle-' + esc(n) + '"' + attr('handle', n) + ' aria-controls="drbody"><svg class="i"><use href="#i-tasks"/></svg><span class="hl">' + esc(n) +
        '</span><span class="hoff" title="offline or no remote" hidden>&#8856;</span><span class="dr-badge" hidden></span><svg class="i"><use href="#i-chev"/></svg></button>';
    });
    $('drhandles').innerHTML = hh;

    document.addEventListener('click', function (e) {
      var t = e.target, h = t.closest('[data-handle]');
      if (h) { if (h.getAttribute('data-handle') === 'panels') toast('The side panels handle is a stub in this mockup.'); else openStore(h.getAttribute('data-handle')); return; }
      var os = t.closest('[data-open-store]');
      if (os) { if (ui.open !== os.getAttribute('data-open-store')) openStore(os.getAttribute('data-open-store')); return; }
      var a = t.closest('[data-act]');
      if (a) {
        e.preventDefault(); e.stopPropagation();
        var k = a.getAttribute('data-act'), d = { item: a.getAttribute('data-item'), who: a.getAttribute('data-who'), op: a.getAttribute('data-op'), cid: a.getAttribute('data-cid') };
        if (k === 'apply') { if (applyRaw()) { render(); toast('Raw data applied.'); } }
        else if (k === 'reset') { resetStore(ui.store); ui.last = null; render(); toast('Store reset to fixtures.'); }
        else if (k === 'sheet-close') { ui.sheet = false; render(); }
        else if (k === 'dlg-cancel') closeDialog();
        else if (k === 'open') showItem(d.item);
        else if (k === 'sync') doSync(ui.store);
        else if (k === 'go-offline') { ui.sync[ui.store].online = false; render(); toast('Offline: new writes are held locally and counted as not pushed.'); }
        else if (k === 'go-online') { ui.sync[ui.store].online = true; doSync(ui.store); }
        else if (k === 'reply') { ui.replyTo = d.cid; render(); if ($('cbox')) $('cbox').focus(); }
        else if (k === 'reply-x') { ui.replyTo = null; render(); }
        else if (QUICK[k]) { if (k === 'comment' || a.getAttribute('data-note')) ui.clearDraft = true; quick(k, a); }
        else openDialog(k, d);
        return;
      }
      var it = t.closest('[data-item]');
      if (it && !t.closest('select, textarea, input, #dlg')) { e.preventDefault(); showItem(it.getAttribute('data-item')); return; }
      if (t.closest('a[href="#"]')) e.preventDefault();
    });
    document.addEventListener('submit', function (e) {
      var f = e.target;
      if (f.closest && f.closest('#dlg')) { e.preventDefault(); submit(f); }
    });
    document.addEventListener('change', function (e) {
      var t = e.target;
      if (t.id === 'viewer') {
        ui.viewers[ui.store] = t.value; ui.last = null;
        if (ui.open) markSeen(ui.store);
        render();
      } else if (t.id === 'f-kind') { ui.kind = t.value; render(); }
      else if (t.id === 'f-status') { ui.status = t.value; render(); }
      else if (t.id === 'flagged-only') { ui.flaggedOnly = t.checked; render(); }
      else if (t.id === 'conc-only') { ui.concOnly = t.checked; render(); }
      else if (t.closest && t.closest('#dlg form')) updateHint(t.closest('form'));
    });
    document.addEventListener('toggle', function (e) {
      var t = e.target;
      if (!t.tagName || t.tagName !== 'DETAILS') return;
      if (t.id === 'data-details') ui.dataOpen = t.open;
      else if (t.getAttribute('data-owner')) ui.openLogs[t.getAttribute('data-owner')] = t.open;
    }, true);
    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape' || $('dlg').open) return;
      if (ui.sheet) { ui.sheet = false; render(); } else if (ui.open) { ui.open = null; render(); }
    });
    window.addEventListener('resize', measure);
    render();
  }

  window.addEventListener('error', function (e) { var t = $('toast'); if (t) { t.hidden = false; t.textContent = 'script error: ' + e.message; } });
  init();
  // read-only hook for scripted checks (headless verification); not part of the model
  window.__tracker = { ui: ui, stores: stores, g: function () { return gs[ui.store]; } };
})();
