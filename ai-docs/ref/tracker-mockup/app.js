// Tracker mockup v3: the view. A dashboard-like frame (header, conversation
// placeholder, pull-down drawer with one handle per store) around the tracker
// views, a thread side sheet, and small action dialogs. The view reads only the
// plain view-state object a store handle returns (store.view) and writes only by
// dispatching events (store.dispatch); it never touches engine internals.
(function () {
  'use strict';

  var E = window.TrackerEngine, alias = E.alias, memberOf = E.memberOf;

  var stores = {}, defs = {}, order = [], vss = {}, EV = {};
  var ui = {
    store: null,          // the store the header control and the sheet refer to
    open: null,           // the store whose drawer is open, or null
    viewers: {},          // store -> clone stream the page is viewed as ("<member>.<clone>")
    clock: {},            // store -> mock "now"; each write advances it by a minute
    seen: {},             // "store|stream" -> { badge key: true } already shown to that viewer
    fresh: {},            // badge keys that were new when the drawer last opened
    read: {},             // "store|stream" -> { notification entry key: true } read by that viewer (local UI state, (334), (340))
    loaded: {},           // "store|stream" -> { attachment oid: true } "loaded" by that viewer's clone (simulated, local UI state)
    mdraft: [], adraft: [], // composer drafts: member ids picked with @, attachment metadata to post
    item: null, sheet: false, last: null, replyTo: null,
    cmodes: {}, calMonth: {}, // store -> { card key -> view mode }; card key -> calendar month (local UI state, never an event)
    act: { family: 'all', member: 'all', flagged: false, conc: false },
    kind: 'all', status: 'open', dataOpen: false, openLogs: {}
  };
  // (303) each card offers only the view modes that fit its content; a card missing here has one layout (no picker).
  var ASSIGN_MODES = [['list', 'list'], ['kanban', 'kanban'], ['gantt', 'due timeline'], ['calendar', 'calendar']];
  var CARD_MODES = { items: [['list', 'tree'], ['kanban', 'kanban'], ['gantt', 'due timeline'], ['calendar', 'calendar']],
    mine: ASSIGN_MODES, given: ASSIGN_MODES, myteam: ASSIGN_MODES, activity: [['list', 'list'], ['activity', 'activity timeline']] };
  var WIDE = { kanban: 1, gantt: 1, calendar: 1, activity: 1 }; // (304) a card in one of these spans the full row

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }
  function $(id) { return document.getElementById(id); }
  function attr(k, v) { return ' data-' + k + '="' + esc(v) + '"'; }
  function vals(o) { return Object.keys(o).map(function (k) { return o[k]; }); }

  // ------------------------------------------------------------- small views

  var VS = null; // the view state being rendered
  function nm(id) { return VS.members[id] ? VS.members[id].name : id; }
  function fmtTs(ts) { return ts ? String(ts).slice(5, 16).replace('T', ' ') : '-'; }
  function today() { return String(ui.clock[ui.store] || '').slice(0, 10); }
  function itemRef(id) { return '<a href="#"' + attr('item', id) + '>' + esc(alias(id)) + '</a>'; }
  function titleRef(id) {
    var it = VS.items[id];
    return '<a class="title" href="#"' + attr('item', id) + '>' + esc(it ? it.title : alias(id)) + '</a>';
  }
  var ST_LABEL = { todo: 'todo', doing: 'doing', blocked: 'blocked', awaiting: 'awaiting confirm', done: 'done',
    unassigned: 'unassigned', pending: 'pending', declined: 'declined', returned: 'returned', handed: 'handed off', withdrawn: 'withdrawn' };
  function stChip(s) { return s ? '<span class="chip st-' + esc(s) + '">' + esc(ST_LABEL[s] || s) + '</span>' : ''; }
  function recChip(r) {
    if (r.state !== 'live') return stChip(r.state);
    return stChip(r.status) + (r.statusOrigin && r.statusOrigin !== 'human' ? '<span class="chip agent" title="the current status was written by ' + esc(r.statusBy) + '\'s agent (' + esc(r.statusOrigin) + ')">by agent</span>' : '');
  }
  function prChip(it) {
    if (!it || it.kind === 'topic') return '';
    return '<span class="chip pr pr-' + it.priorityLevel + '" title="priority ' + it.priorityLevel + ' of ' + (VS.priority.names.length - 1) +
      (it.priority !== it.priorityLevel ? ' (stored ' + esc(it.priority) + ', outside the current range: shown as the top level)' : '') + '">' + esc(it.priorityName) + '</span>';
  }
  function dueChip(it) {
    if (!it || !it.due) return '';
    var over = it.due < today() && it.status !== 'done', now = it.due === today() && it.status !== 'done';
    return '<span class="chip due' + (over ? ' over' : now ? ' now' : '') + '" title="due ' + esc(it.due) + (over ? ', overdue' : now ? ', due today' : '') + '">' +
      (now ? 'due today' : 'due ' + esc(it.due.slice(5))) + '</span>';
  }
  function kindChip(it) {
    return '<span class="chip kind">' + esc(it.kind) + '</span>' + (it.pending.length ? '<span class="chip req">pending assignment</span>' : '') +
      (it.hidden ? '<span class="chip hid">hidden</span>' : '');
  }
  function originChip(o) {
    return o === 'agent-auto' ? '<span class="chip agent">agent</span><span class="chip auto">auto</span>' :
      o === 'agent' ? '<span class="chip agent">agent</span>' : '';
  }
  function complText(c) { return c === 'assigner' ? 'assigner confirms' : 'assignee closes'; }
  function rollupHtml(it) {
    var r = it.rollup, h = '';
    if (r.total) h += '<span class="chip roll" title="children by status (manual status stays with the assignee)">done ' + (r.counts.done || 0) + '/' + r.total +
      (r.counts.blocked ? ' &middot; blocked ' + r.counts.blocked : '') + '</span>';
    if (r.blocked.length) h += '<span class="chip warn" title="blocked below: ' + esc(r.blocked.map(function (id) { return alias(id) + ' ' + VS.items[id].title; }).join('; ')) + '">&#9888; blocked below</span>';
    return h;
  }
  var END_TEXT = { declined: 'declined', returned: 'returned by the lead', handed: 'handed on', unassigned: 'unassigned', withdrawn: 'withdrawn' };
  function endedBy(r) { return r.ended && VS.events.filter(function (e) { return e.id === r.ended.id; }).map(function (e) { return e.type === 'roles.remove_member' ? ' (member removed)' : ''; })[0] || ''; }
  function source(r) {
    if (r.handoffOf) return (r.sender === r.handoffFrom ? 'handoff from <b>' + esc(nm(r.sender)) + '</b>' : 'admin handoff of <b>' + esc(nm(r.handoffFrom)) + '</b>\'s assignment by <b>' + esc(nm(r.sender)) + '</b>') +
      ' (assigner ' + esc(nm(r.by)) + ')' + (r.state === 'pending' ? ', not accepted yet' : r.accepted ? ', accepted' + (r.state !== 'live' ? ', then ' + END_TEXT[r.state] : '') : END_TEXT[r.state] ? ', ' + END_TEXT[r.state] : '');
    if (r.restored) return 'from <b>' + esc(nm(r.by)) + '</b>, restored when the handoff was returned (' + esc(alias(r.restored.id)) + ')';
    if (r.to === r.by) return 'self-assigned';
    if (!r.forced) return 'sent by <b>' + esc(nm(r.by)) + '</b>, ' + (r.state === 'pending' ? 'not accepted yet' : r.accepted ? 'accepted' + (r.state !== 'live' ? ', then ' + END_TEXT[r.state] : '') : END_TEXT[r.state] || r.state);
    return 'from <b>' + esc(nm(r.by)) + '</b> (forced: their lead chain)' + (r.inherited ? ' (inherited: everyone within L' + (r.ruleLevels || '?') + ', reached later)' : '');
  }
  function btn(act, label, data, cls) {
    var d = '';
    Object.keys(data || {}).forEach(function (k) { d += attr(k, data[k]); });
    return '<button class="btn mini' + (cls ? ' ' + cls : '') + '"' + attr('act', act) + d + '>' + label + '</button>';
  }
  function card(title, chip, body, extra, id, mode, cls) {
    return '<section class="card' + (WIDE[mode] ? ' wide' : '') + (cls ? ' ' + cls : '') + '"' + (id ? ' id="' + id + '"' : '') + (mode ? attr('view', mode) : '') + '><header><h2>' + title + '</h2>' + (chip ? '<span class="chip">' + chip + '</span>' : '') +
      '<span class="grow"></span>' + (extra || '') + '</header><div class="cbody">' + body + '</div></section>';
  }
  // (305) the chosen mode is remembered per store, so it survives a viewer switch within the store
  function modeOf(key) { var m = (ui.cmodes[ui.store] || {})[key]; return CARD_MODES[key] && m ? m : 'list'; }
  function isWide(key) { return !!WIDE[modeOf(key)]; }
  function picker(key) {
    return '<span class="cmodes"' + attr('card', key) + '>' + CARD_MODES[key].map(function (m) {
      return '<button class="btn mini' + (modeOf(key) === m[0] ? ' primary' : '') + '"' + attr('act', 'cmode') + attr('card', key) + attr('mode', m[0]) + '>' + m[1] + '</button>';
    }).join('') + '</span>';
  }
  var RANK = { blocked: 0, doing: 1, todo: 2, awaiting: 3, done: 4 };
  function byRank(a, b) { return RANK[a.status] - RANK[b.status]; }
  function A(id) { return VS.assignments[id]; }

  var EDIT_FIELDS = ['title', 'body', 'project', 'due', 'priority'];
  function listOf(x) { return Array.isArray(x) ? x : []; }
  // Mentions and attachments a write carries, for one-line descriptions.
  function extrasTxt(p) {
    var m = listOf(p.mentions), a = listOf(p.attachments);
    return (m.length ? ' (mentions ' + esc(m.map(function (x) { return '@' + x; }).join(', ')) + ')' : '') +
      (a.length ? ' [' + a.length + ' attachment' + (a.length === 1 ? '' : 's') + ']' : '');
  }
  function describe(n, inThread) {
    var p = n.payload, ref = function (id) { return inThread ? '' : ' ' + itemRef(id); };
    switch (n.type) {
      case 'item.create':
        if (inThread === 'parent') return 'opened subtask thread ' + itemRef(n.id) + ' <b>' + esc(p.title) + '</b> ' + (VS.items[n.id] ? stChip(VS.items[n.id].status) : '');
        return 'created ' + esc(p.kind) + ' <b>' + esc(p.title) + '</b>' + (inThread ? '' : ' (' + itemRef(n.id) + ')') + (p.parent && !inThread ? ' under ' + itemRef(p.parent) : '') +
          (p.due ? ', due ' + esc(p.due) : '') + (p.priority != null ? ', priority ' + esc(VS.priority.names[p.priority] || p.priority) : '') + extrasTxt(p);
      case 'item.edit':
        return 'edited' + ref(p.item) + ' ' + EDIT_FIELDS.filter(function (f) { return f in p; }).map(function (f) {
          var v = f === 'priority' ? (VS.priority.names[p[f]] || p[f]) : p[f];
          return f + ' &rarr; "' + esc(v == null ? '(none)' : String(v).slice(0, 80)) + '"';
        }).join(', ') + extrasTxt(p);
      case 'item.hide': return 'hid' + ref(p.item) + ' (tombstone; history stays)';
      case 'item.assign':
        return 'assigned' + ref(p.item) + ' to ' + (p.mode === 'all' ? '<b>everyone within L' + esc(p.levels) + '</b> (forced, live: later followers inherit)' :
          '<b>' + esc((p.people || []).map(function (x) { return x === n.member ? 'self' : x; }).join(', ')) + '</b>') + ', ' + complText(p.completion);
      case 'item.handoff': return 'handed off ' + (p.assignee && p.assignee !== n.member ? '<b>' + esc(p.assignee) + '</b>\'s assignment (admin)' : 'their assignment') + ref(p.item) + ' to <b>' + esc(p.to) + '</b> (pending until accepted)' + (p.note ? ': ' + esc(p.note) : '');
      case 'item.accept': return 'accepted the assignment' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.decline': return 'declined the assignment' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.return': return 'returned ' + esc(p.assignee) + '\'s accepted assignment' + ref(p.item) + ' (ends it)' + (p.note ? ': ' + esc(p.note) : '');
      case 'item.unassign': return 'unassigned ' + esc(p.assignee || 'all of their assignees') + ref(p.item);
      case 'item.status': return 'set status ' + stChip(p.status) + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.close': return 'closed' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.confirm': return 'confirmed ' + esc(p.assignee) + '\'s close' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.reject': return 'rejected ' + esc(p.assignee) + '\'s close (back to doing)' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.reopen': return 'reopened ' + esc(p.assignee || n.member) + '\'s assignment' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'comment.add': return 'commented' + ref(p.item) + (inThread ? '' : ': ' + esc(String(p.body || '').slice(0, 90))) + extrasTxt(p);
      case 'comment.edit': return 'edited comment ' + esc(alias(p.comment)) + (inThread ? '' : ref(n.thread || '')) + extrasTxt(p);
      case 'comment.hide': return 'hid comment ' + esc(alias(p.comment)) + ' (tombstone; history stays)';
      case 'roles.add_member': return 'added member <b>' + esc((p.member || {}).id) + '</b> under ' + esc(((p.member || {}).leads || []).join(', ') || 'nobody');
      case 'roles.set_lead': return 'set lead of <b>' + esc(p.member) + '</b> to ' + esc((p.leads || []).join(', ') || 'nobody');
      case 'roles.add_admin': return 'made <b>' + esc(p.member) + '</b> an admin';
      case 'roles.remove_admin': return 'removed admin role from <b>' + esc(p.member) + '</b>';
      case 'roles.add_key': return 'added key <code>' + esc(p.key) + '</code> for ' + esc(p.member);
      case 'roles.revoke_key': return 'revoked key <code>' + esc(p.key) + '</code> of ' + esc(p.member);
      case 'roles.remove_member': return 'removed member <b>' + esc(p.member) + '</b>';
      case 'roles.set_priority_levels': return 'set priority levels to <b>' + esc((p.names || []).join(' / ')) + '</b>, default ' + esc((p.names || [])[p.default]);
      case 'roles.set_attachment_limit': return 'set the attachment limit to <b>' + esc(p.max_bytes) + ' bytes</b>' + (Number.isInteger(p.max_bytes) ? ' (' + fmtSize(p.max_bytes) + ')' : '');
      default: return esc(n.type);
    }
  }
  function markerHtml(n) {
    return n.markers.map(function (m) {
      var oa = esc(alias(m.other)) + ' (' + esc(m.otherMember || '?') + ')';
      if (m.kind === 'field') {
        return '<div class="conc"><b>concurrent change</b> to ' + esc(m.field) + ': this wrote <i>' + esc(m.mine) + '</i>, ' + oa + ' wrote <i>' + esc(m.theirs) +
          '</i> without seeing it. Shown: <b>' + esc(alias(m.winner)) + '</b> (later ts, then member, clone, seq).</div>';
      }
      if (m.kind === 'removed') {
        return n.id === m.winner ?
          '<div class="conc"><b>concurrent change</b>: ' + oa + ' wrote to ' + esc(m.field) + ' without seeing this; that write is not in effect.</div>' :
          '<div class="conc"><b>concurrent change</b>: ' + oa + ' ended ' + esc(m.field) + ' without seeing this; valid when written, not in effect now.</div>';
      }
      return '<div class="conc"><b>concurrent change</b>: ' + esc(m.field) + ' was created while ' + oa + ' concurrently removed the parent assignment.</div>';
    }).join('');
  }
  function flags(n) {
    var h = '';
    if (n.late) h += '<span class="chip late" title="reached this clone after events written later">arrived late</span>';
    if (n.open) h += '<span class="chip local" title="in the open batch of the local journal; not committed yet">open batch</span>';
    else if (n.local) h += '<span class="chip local" title="committed on this clone, not pushed yet">not pushed</span>';
    if (n.status === 'flagged') h += '<span class="chip flag">' + (n.ignored ? 'ignored' : 'flagged') + '</span>';
    if (n.status === 'waiting') h += '<span class="chip">waiting</span>';
    if (n.markers.length) h += '<span class="chip conc-c">concurrent</span>';
    return h;
  }
  // (289) a same-member write that lost to another clone's concurrent write: a quiet note, no conflict box
  function supHtml(n) {
    return (n.superseded || []).map(function (x) {
      return '<div class="sup muted">superseded by ' + esc(alias(x.by)) + ' (' + (x.why === 'human' ? 'a human write' : 'a later write') + ' from another clone): ' + esc(x.field) + '</div>';
    }).join('');
  }
  function evLine(n, inThread) {
    return '<li class="ev' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (n.origin !== 'human' ? ' agent' : '') + '"' + attr('ev', n.id) + '>' +
      '<span class="when">' + esc(fmtTs(n.ts)) + '</span><span class="al">' + esc(n.alias) + '</span> <b>' + esc(nm(n.member)) + '</b> ' + originChip(n.origin) +
      ' ' + describe(n, inThread) + ' ' + flags(n) + (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(n) + supHtml(n) + '</li>';
  }

  // ------------------------------------------------------- viewer / badge state

  function viewerOf(name) { return ui.viewers[name]; }
  function seenKey(name) { return name + '|' + viewerOf(name); }
  function newKeys(name) {
    var seen = ui.seen[seenKey(name)] || {};
    return vss[name].my.badge.filter(function (k) { return !seen[k]; });
  }
  // Opening a store (or switching viewer inside an open drawer) shows what is new
  // once, then clears the handle badge, like the mailbox badge.
  function markSeen(name) {
    var seen = (ui.seen[seenKey(name)] = ui.seen[seenKey(name)] || {});
    ui.fresh = {};
    newKeys(name).forEach(function (k) { ui.fresh[k] = true; seen[k] = true; });
  }
  function isNew(key) { return ui.fresh[key] ? '<span class="chip new">new</span>' : ''; }

  // ------------------------------------------------------------------ cards

  function workRow(r, withActs) {
    var it = VS.items[r.item];
    var chainTxt = it.chain.length > 1 ? ' &middot; chain ' + esc(it.chain.join(' > ')) : '';
    var acts = '';
    if (withActs && r.status !== 'done') {
      if (r.status !== 'doing') acts += btn('q-status', 'doing', { item: r.item, status: 'doing' });
      if (r.status !== 'blocked') acts += btn('q-status', 'blocked', { item: r.item, status: 'blocked' });
      acts += btn('status', 'Status...', { item: r.item });
      if (r.status !== 'awaiting') acts += btn('close', 'Close', { item: r.item });
      acts += btn('split', 'Split', { item: r.item }) + btn('handoff', 'Hand off...', { item: r.item });
    }
    return '<div class="row click"' + attr('item', r.item) + attr('rec', r.id) + '><div class="r1">' + recChip(r) + isNew('a:' + r.id) + prChip(it) +
      titleRef(r.item) + dueChip(it) + rollupHtml(it) + (r.stale ? '<span class="chip warn" title="no status update for ' + E.STALE_DAYS + '+ days">stale</span>' : '') +
      '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(it.lastActivity)) + '</span></div>' +
      '<div class="r2">' + source(r) + ' &middot; ' + complText(r.completion) + chainTxt + (it.project ? ' &middot; ' + esc(it.project) : '') +
      (r.note ? '<br>&ldquo;' + esc(r.note) + '&rdquo; <span class="muted">(' + esc(r.noteBy) + ')</span>' : '') + '</div>' +
      (acts ? '<div class="r3">' + acts + '</div>' : '') + '</div>';
  }
  function pendRow(r, acts) {
    var it = VS.items[r.item];
    return '<div class="row click"' + attr('item', r.item) + attr('rec', r.id) + '><div class="r1">' + recChip(r) + isNew('p:' + r.id) + prChip(it) + titleRef(r.item) + dueChip(it) +
      '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(r.ts)) + '</span></div>' +
      '<div class="r2">' + source(r) + ' &middot; ' + complText(r.completion) + (r.handoffNote ? ' &middot; &ldquo;' + esc(r.handoffNote) + '&rdquo;' : '') +
      (r.answerNote ? ' &middot; &ldquo;' + esc(r.answerNote) + '&rdquo;' : '') + '</div>' + (acts ? '<div class="r3">' + acts + '</div>' : '') + '</div>';
  }
  function answerBtns(r) {
    return btn('accept', 'Accept', { item: r.item }, 'primary') + btn('note', 'Decline', { item: r.item, op: 'item.decline' }) + btn('open', 'Discuss', { item: r.item });
  }

  // (331)-(334) The notification feed: one card, list only, newest first. Every entry
  // is derived from the view state; an entry only opens the item's thread sheet, while
  // Received requests and My team keep their notices and buttons. Read state is local
  // UI state per viewer clone, like the handle badge's `seen`; no event is written
  // (in the product the daemon keeps it per clone on the local filesystem, (340)).
  var NK = { assigned: 'assigned', pending: 'pending', answer: 'answer', reference: 'for reference', done: 'done', blocked: 'blocked',
    confirm: 'confirm', reply: 'reply', due: 'due', mention: 'mention' };
  function snippet(s) { s = String(s || ''); return '&ldquo;' + esc(s.length > 80 ? s.slice(0, 77) + '...' : s) + '&rdquo;'; }
  function feedEntries(v) {
    var out = [], t = today();
    function add(key, kind, item, ts, html) {
      if (VS.items[item] && !VS.items[item].hidden) out.push({ key: key, kind: kind, item: item, ts: String(ts || ''), html: html });
    }
    VS.my.assignments.map(A).forEach(function (r) {
      var it = VS.items[r.item];
      // new forced assignments: the same rule as the handle badge's "a:" keys
      if (r.by !== v && r.forced && r.status === 'todo') add('a:' + r.id, 'assigned', r.item, r.ts, '<b>' + esc(nm(r.by)) + '</b> assigned this to you' + (r.inherited ? ' (everyone within L' + esc(r.ruleLevels) + ')' : ''));
      // near the due date, as the due chip computes it
      if (it.due && it.due <= t && r.status !== 'done' && it.status !== 'done')
        add('d:' + r.id + ':' + (it.due < t ? 'over' : 'today'), 'due', r.item, it.due + 'T00:00:00Z', it.due < t ? 'overdue since ' + esc(it.due) : 'due today');
    });
    VS.my.received.pending.map(A).forEach(function (r) {
      add('p:' + r.id, 'pending', r.item, r.ts, '<b>' + esc(nm(r.sender)) + '</b> ' + (r.handoffOf ? 'hands you their assignment' : 'sent you a pending assignment') + '; answer it on Received requests');
    });
    VS.notices.forEach(function (x) {
      var r = A(x.assignment), txt;
      if (x.kind === 'declined') txt = '<b>' + esc(nm(x.to)) + '</b> declined what you sent';
      else if (x.kind === 'returned') txt = '<b>' + esc(nm(x.endedBy)) + '</b> (lead of ' + esc(nm(x.to)) + ') returned ' + esc(nm(x.to)) + '\'s accepted ' + (r.handoffOf ? 'handoff' : 'assignment');
      else if (x.kind === 'accepted') txt = '<b>' + esc(nm(x.to)) + '</b> accepted ' + (r.handoffOf ? 'a handoff' : 'an assignment from ' + esc(nm(x.by))) + ' (you are their lead)';
      else txt = '<b>' + esc(nm(x.to)) + '</b> took over ' + esc(nm(x.from)) + '\'s assignment you gave';
      add(x.key, x.kind === 'declined' || x.kind === 'returned' ? 'answer' : 'reference', x.item, x.ts, txt + (x.note ? ': ' + snippet(x.note) : ''));
    });
    vals(VS.assignments).forEach(function (r) {
      if (r.by !== v || r.to === v || r.state !== 'live' || (r.status !== 'done' && r.status !== 'blocked')) return;
      add('w:' + r.id + ':' + r.status + ':' + r.lastUpdate, r.status, r.item, r.lastUpdate,
        '<b>' + esc(nm(r.to)) + '</b> ' + (r.status === 'done' ? 'finished work you assigned' : 'is blocked on work you assigned') + (r.note ? ': ' + snippet(r.note) : ''));
    });
    VS.my.today.confirms.map(A).forEach(function (r) {
      add('c:' + r.id + ':' + r.lastUpdate, 'confirm', r.item, r.lastUpdate, '<b>' + esc(nm(r.to)) + '</b> closed this; it waits for your confirm');
    });
    vals(VS.comments).forEach(function (c) {
      if (c.hidden || c.author === v) return;
      var q = c.reply_to && VS.comments[c.reply_to];
      if (q && q.author === v) add('r:' + c.id, 'reply', c.item, c.ts, '<b>' + esc(nm(c.author)) + '</b> replied to your comment: ' + snippet(c.body));
      var ts = c.edited ? c.edited.ts : c.ts;
      if (c.mentions.indexOf(v) >= 0) add('m:' + c.id + ':' + ts, 'mention', c.item, ts, '<b>' + esc(nm(c.author)) + '</b> mentioned you: ' + snippet(c.body));
    });
    vals(VS.items).forEach(function (it) {
      var ts = it.edited ? it.edited.ts : it.ts;
      if (it.author !== v && it.mentions.indexOf(v) >= 0) add('m:' + it.id + ':' + ts, 'mention', it.id, ts, '<b>' + esc(nm(it.author)) + '</b> mentioned you in the item body');
    });
    return out.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : a.key < b.key ? -1 : 1; });
  }
  function readMap() { return (ui.read[seenKey(ui.store)] = ui.read[seenKey(ui.store)] || {}); }
  function cardNotify(v) {
    var list = feedEntries(v), read = readMap(), unread = list.filter(function (x) { return !read[x.key]; }).length;
    var h = list.length ? list.map(function (x) {
      return '<div class="row click nfeed' + (read[x.key] ? '' : ' unread') + '"' + attr('item', x.item) + attr('nkey', x.key) + attr('nkind', x.kind) + '><div class="r1">' +
        '<span class="chip nk">' + esc(NK[x.kind]) + '</span>' + isNew(x.key) + titleRef(x.item) + '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(x.ts)) + '</span></div>' +
        '<div class="r2">' + x.html + '</div></div>';
    }).join('') : '<p class="muted">Nothing for you right now.</p>';
    return card('Notifications', unread + ' unread of ' + list.length, h, unread ? btn('nread-all', 'Mark all read', {}) : '', 'card-notify');
  }

  // (335) mention chips and (338) attachment chips. Loading is simulated: the real
  // client fetches the blob on demand through a partial clone (a later ledger ticket).
  function fmtSize(n) { return n >= 1048576 ? (n / 1048576).toFixed(1) + ' MiB' : n >= 1024 ? Math.round(n / 1024) + ' KiB' : n + ' B'; }
  function mentionChips(ids) {
    ids = listOf(ids).filter(function (x) { return typeof x === 'string'; });
    return ids.length ? '<div class="mentions">' + ids.map(function (id) {
      return '<span class="chip mention" title="mentions ' + esc(id) + '">@' + esc(VS.members[id] ? VS.members[id].name : id) + '</span>';
    }).join('') + '</div>' : '';
  }
  function attChips(list, live) {
    list = listOf(list).filter(function (x) { return x && typeof x === 'object'; });
    if (!list.length) return '';
    var got = ui.loaded[seenKey(ui.store)] || {};
    return '<div class="atts">' + list.map(function (x) {
      var label = 'file ' + esc(x.name) + ' &middot; ' + (Number.isInteger(x.size) ? fmtSize(x.size) : '?');
      if (!live) return '<span class="chip att rejected" title="not stored: the write was flagged">' + label + '</span>';
      var on = !!got[x.oid];
      return '<button type="button" class="chip att' + (on ? ' loaded' : '') + '"' + attr('act', 'att-load') + attr('oid', x.oid) +
        ' title="' + esc(x.mime + ', ' + x.size + ' bytes, sha256 ' + x.oid + (on ? '' : '; click to fetch it (simulated)')) + '">' + label + ' &middot; ' + (on ? 'loaded' : 'not loaded') + '</button>';
    }).join('') + '</div>';
  }

  // Morning check: my open work by priority and due, what waits for my answer, what waits for my confirm.
  function cardToday() {
    var t = VS.my.today, h = '';
    h += '<div class="sub">my open work (priority, then due)</div>' + (t.work.length ? t.work.map(function (id) {
      var r = A(id), it = VS.items[r.item];
      return '<div class="row click"' + attr('item', r.item) + '><div class="r1">' + prChip(it) + recChip(r) + titleRef(r.item) + dueChip(it) +
        (r.stale ? '<span class="chip warn">stale</span>' : '') + '<span class="grow"></span><span class="muted">' + esc(nm(r.by)) + '</span></div></div>';
    }).join('') : '<p class="muted">Nothing open.</p>');
    if (t.pending.length) h += '<div class="sub">waiting for my answer</div>' + t.pending.map(function (id) { return pendRow(A(id), answerBtns(A(id))); }).join('');
    if (t.confirms.length) h += '<div class="sub">waiting for my confirm</div>' + t.confirms.map(function (id) {
      var r = A(id);
      return '<div class="row click"' + attr('item', r.item) + '><div class="r1">' + stChip('awaiting') + prChip(VS.items[r.item]) + titleRef(r.item) + '<span class="muted">' + esc(nm(r.to)) + '</span>' +
        '<span class="grow"></span>' + btn('confirm', 'Confirm', { item: r.item, who: r.to }, 'primary') + btn('note', 'Reject', { item: r.item, who: r.to, op: 'item.reject' }) + '</div>' +
        (r.note ? '<div class="r2">&ldquo;' + esc(r.note) + '&rdquo;</div>' : '') + '</div>';
    }).join('');
    return card('Today', t.work.length + ' open, ' + t.pending.length + ' to answer, ' + t.confirms.length + ' to confirm', h, '', 'card-today');
  }

  function cardMine(v) {
    var mine = VS.my.assignments.map(A);
    var openR = mine.filter(function (r) { return r.status !== 'done'; }), done = mine.filter(function (r) { return r.status === 'done'; });
    var m = modeOf('mine');
    if (m !== 'list') return card('My assignments', openR.length + ' open', modeBody('mine', m, v), picker('mine'), 'card-mine', m);
    var h = openR.length ? openR.map(function (r) { return workRow(r, true); }).join('') : '<p class="muted">Nothing open is assigned to you.</p>';
    if (done.length) h += '<div class="sub">done</div>' + done.map(function (r) {
      return '<div class="row click"' + attr('item', r.item) + '><div class="r1">' + stChip('done') + titleRef(r.item) + '<span class="grow"></span><span class="muted">' + source(r) + '</span></div></div>';
    }).join('');
    if (VS.my.created.length) h += '<div class="sub">created by me, not assigned</div>' + VS.my.created.map(function (id) {
      var it = VS.items[id];
      return '<div class="row click"' + attr('item', id) + '><div class="r1">' + stChip(it.status) + kindChip(it) + titleRef(id) + '<span class="grow"></span>' +
        btn('assign', 'Assign', { item: id }) + '</div></div>';
    }).join('');
    return card('My assignments', openR.length + ' open', h, picker('mine'), 'card-mine', m);
  }

  function cardRequests() {
    var R = VS.my.received, ans = VS.notices.filter(function (x) { return x.kind === 'declined' || x.kind === 'returned'; });
    if (!R.pending.length && !R.answered.length && !R.sent.length && !ans.length) return '';
    var h = ans.length ? '<div class="sub">answers to what I sent</div>' + ans.map(function (x) {
      var r = A(x.assignment);
      var txt = x.kind === 'declined' ? '<b>' + esc(nm(x.to)) + '</b> declined' :
        '<b>' + esc(nm(x.endedBy)) + '</b> (lead of ' + esc(nm(x.to)) + ') returned ' + esc(nm(x.to)) + '\'s accepted ' + (r.handoffOf ? 'handoff' : 'assignment') +
        (x.restores ? '; ' + esc(nm(x.from)) + '\'s assignment is restored' : r.handoffOf ? '; nobody holds it now' : '');
      return '<div class="tm-work click notice"' + attr('item', x.item) + attr('notice', x.key) + '>' + isNew(x.key) + recChip(r) + titleRef(x.item) + '<span class="muted">' + txt +
        (x.note ? ': &ldquo;' + esc(x.note) + '&rdquo;' : '') + ', ' + esc(fmtTs(x.ts)) + '</span></div>';
    }).join('') : '';
    h += R.pending.length ? R.pending.map(function (id) { return pendRow(A(id), answerBtns(A(id))); }).join('') : '<p class="muted">No pending assignments to you.</p>';
    if (R.answered.length) h += '<div class="sub">answered</div>' + R.answered.map(function (id) { return pendRow(A(id)); }).join('');
    if (R.sent.length) h += '<div class="sub">sent by me</div>' + R.sent.map(function (id) {
      var r = A(id);
      return pendRow(r, r.state === 'pending' ? '<span class="muted">to <b>' + esc(nm(r.to)) + '</b></span>' + btn('unassign', 'Withdraw', { item: r.item, who: r.to }) : '<span class="muted">to <b>' + esc(nm(r.to)) + '</b></span>');
    }).join('');
    return card('Received requests', R.pending.length + ' pending', '<p class="r2" style="margin:0 0 6px">Pending assignments: someone outside your lead chain, or a handoff. Not counted in any roll-up until accepted.</p>' + h, '', 'card-received');
  }

  // Lead perspective: notices, then what each follower (down to 3 levels) works on, whoever assigned it.
  function cardMyTeam(v) {
    var team = VS.my.team, notices = VS.notices.filter(function (x) { return x.kind === 'accepted' || x.kind === 'handoff'; });
    if (!team.length && !notices.length) return '';
    var totals = { open: 0, blocked: 0, pending: 0, stale: 0 }, h = '';
    if (notices.length) h += '<div class="sub">for reference: accepted assignments</div>' + notices.map(function (x) {
      var r = A(x.assignment);
      var txt = x.kind === 'accepted' ? '<b>' + esc(nm(x.to)) + '</b> accepted ' + (r.handoffOf ? 'a handoff from ' + esc(nm(r.sender)) : 'an assignment from ' + esc(nm(x.by))) + ' (you are their lead)' :
        '<b>' + esc(nm(x.to)) + '</b> took over ' + esc(nm(x.from)) + '\'s assignment you gave (handoff accepted)';
      return '<div class="tm-work click notice"' + attr('item', x.item) + attr('notice', x.key) + '>' + isNew(x.key) + recChip(r) + titleRef(x.item) + '<span class="muted">' + txt + ', ' + esc(fmtTs(x.ts)) + '</span>' +
        '<span class="grow"></span>' + (x.kind === 'accepted' && r.state === 'live' ? btn('note', 'Return', { item: x.item, who: x.to, op: 'item.return' }) : '') + '</div>';
    }).join('');
    h += team.map(function (p) {
      var m = VS.members[p.id];
      totals.open += p.open.length; totals.blocked += p.blocked; totals.pending += p.pending.length; totals.stale += p.stale;
      var x = '<div class="tm-person" style="--d:' + (p.depth - 1) + '"' + attr('person', p.id) + '><div class="tm-head"><b>' + esc(m.name) + '</b><span class="nid">' + esc(p.id) + '</span>' +
        '<span class="chip">L' + p.depth + '</span>' + (m.children.length ? '<span class="chip">lead</span>' : '') +
        '<span class="muted">' + p.open.length + ' open' + (p.done ? ', ' + p.done + ' done' : '') + '</span>' +
        (p.blocked ? '<span class="chip st-blocked">' + p.blocked + ' blocked</span>' : '') +
        (p.stale ? '<span class="chip warn">' + p.stale + ' stale</span>' : '') +
        (p.pending.length ? '<span class="chip st-pending">' + p.pending.length + ' unaccepted</span>' : '') +
        (p.waiting.length ? '<span class="chip st-pending" title="pending assignments they sent that nobody accepted yet">' + p.waiting.length + ' waiting on others</span>' : '') +
        '<span class="grow"></span><span class="muted ts" title="last update on their work">' + esc(fmtTs(p.last)) + '</span></div>';
      x += p.open.map(function (id) {
        var r = A(id), it = VS.items[r.item];
        return '<div class="tm-work click"' + attr('item', r.item) + '>' + recChip(r) + prChip(it) + titleRef(r.item) + dueChip(it) + rollupHtml(it) +
          (r.stale ? '<span class="chip warn">stale</span>' : '') +
          '<span class="muted">' + source(r) + (r.note ? ' &middot; &ldquo;' + esc(r.note) + '&rdquo;' : '') + '</span>' +
          '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(r.lastUpdate)) + '</span></div>';
      }).join('');
      x += p.pending.map(function (id) {
        var r = A(id);
        return '<div class="tm-work click pend"' + attr('item', r.item) + '>' + stChip('pending') + titleRef(r.item) +
          '<span class="muted">' + source(r) + '</span><span class="grow"></span><span class="muted ts">' + esc(fmtTs(r.ts)) + '</span></div>';
      }).join('');
      x += p.waiting.map(function (id) {
        var r = A(id);
        return '<div class="tm-work click pend"' + attr('item', r.item) + '>' + stChip('pending') + titleRef(r.item) +
          '<span class="muted">sent to ' + esc(nm(r.to)) + (r.handoffOf ? ' (handoff)' : '') + ', not accepted yet</span><span class="grow"></span><span class="muted ts">' + esc(fmtTs(r.ts)) + '</span></div>';
      }).join('');
      return x + '</div>';
    }).join('');
    var m = modeOf('myteam');
    return card('My team', team.length + ' below you, ' + totals.open + ' open' + (totals.blocked ? ', ' + totals.blocked + ' blocked' : '') +
      (totals.stale ? ', ' + totals.stale + ' stale' : '') + (totals.pending ? ', ' + totals.pending + ' unaccepted' : ''),
      m !== 'list' ? modeBody('myteam', m, v) :
      '<p class="r2" style="margin:0 0 6px">Everything your followers (down to 3 levels) are working on, whoever assigned it. Stale: doing or blocked with no status update for ' + E.STALE_DAYS + '+ days.</p>' + h, picker('myteam'), 'card-team', m);
  }

  function cardGiven(v) {
    var m = modeOf('given'), chip = VS.my.given.length + (VS.my.given.length === 1 ? ' item' : ' items') + (VS.my.today.confirms.length ? ', ' + VS.my.today.confirms.length + ' to confirm' : '');
    if (m !== 'list') return card('Assigned by me', chip, modeBody('given', m, v), picker('given'), 'card-given', m);
    var h = VS.my.given.length ? VS.my.given.map(function (id) {
      var it = VS.items[id];
      var as = it.assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); }).sort(byRank);
      return '<div class="row click"' + attr('item', id) + '><div class="r1">' + stChip(it.status) + prChip(it) + titleRef(id) + dueChip(it) + rollupHtml(it) + '</div>' +
        as.map(function (r) {
          return '<div class="assignee-line"><b>' + esc(nm(r.to)) + '</b>' + (r.depth ? '<span class="muted">L' + r.depth + '</span>' : '') + recChip(r) +
            (r.forced ? '' : '<span class="muted">' + (r.handoffOf ? 'handoff' : 'sent') + '</span>') +
            (r.note ? '<span class="muted">&ldquo;' + esc(r.note) + '&rdquo;</span>' : '') +
            (r.state === 'live' && r.status === 'awaiting' && r.by === v ? btn('confirm', 'Confirm', { item: r.item, who: r.to }, 'primary') + btn('note', 'Reject', { item: r.item, who: r.to, op: 'item.reject' }) : '') + '</div>';
        }).join('') + '</div>';
    }).join('') : '<div class="empty">You have not assigned anything to others.</div>';
    return card('Assigned by me', chip, h, picker('given'), 'card-given', m);
  }

  function cardTeam(v) {
    var me = VS.members[v] || {}, R = VS.viewer.reach, levels = {}, seen = {};
    Object.keys(R).forEach(function (k) { levels[R[k]] = (levels[R[k]] || 0) + 1; });
    var summary = 'Admins: ' + esc(VS.admins.join(', ')) + '. You: ' + (VS.viewer.lead ? 'lead <b>' + esc(VS.viewer.lead) + '</b>' : 'no lead (root)') + ', ' +
      (Object.keys(R).length ? 'reach ' + Object.keys(levels).map(function (d) { return levels[d] + ' at L' + d; }).join(', ') : 'no followers') +
      '. Priority levels: ' + VS.priority.names.map(function (x, i) { return esc(x) + (i === VS.priority.default ? ' (default)' : ''); }).join(' &lt; ') +
      '. Attachment limit: ' + fmtSize(VS.attachments.max_bytes) + ' (attachments.max_bytes ' + VS.attachments.max_bytes + ').';
    function node(id) {
      if (seen[id]) return '<li>' + esc(id) + ' (cycle guard)</li>';
      seen[id] = true;
      var m = VS.members[id];
      var chips = (id === v ? '<span class="chip you">you</span>' : '') + (m.admin ? '<span class="chip admin">admin</span>' : '') +
        (id === VS.viewer.lead ? '<span class="chip">your lead</span>' : '') + (R[id] ? '<span class="chip" title="levels below you">L' + R[id] + '</span>' : '') +
        (m.joined ? '<span class="chip" title="added by ' + esc(alias(m.joined)) + '">joined</span>' : '') +
        (m.revoked.length ? '<span class="chip flag" title="revoked keys">revoked ' + esc(m.revoked.map(function (x) { return x.key; }).join(', ')) + '</span>' : '') +
        ((VS.clones[id] || []).length > 1 ? '<span class="chip" title="clones (one ref each)">' + VS.clones[id].length + ' clones</span>' : '');
      return '<li><div class="node' + (id === v ? ' me' : '') + '"><span class="nm">' + esc(m.name) + '</span><span class="nid">' + esc(id) + '</span>' + chips +
        '<span class="nid">' + esc(m.keys.join(', ')) + '</span></div>' + (m.children.length ? '<ul>' + m.children.map(node).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(VS.members).filter(function (id) { var m = VS.members[id], l = m.leads; return !m.removed && (!l.length || !VS.members[l[0]] || VS.members[l[0]].removed); });
    var h = '<p class="r2" style="margin:0 0 6px">' + summary + '</p>';
    if (VS.rolesProblems.length) h += '<p class="err">roles file problems: ' + esc(VS.rolesProblems.join('; ')) + '</p>';
    h += '<ul class="tree">' + roots.map(node).join('') + '</ul>';
    var gone = Object.keys(VS.members).filter(function (id) { return VS.members[id].removed; });
    if (gone.length) h += '<p class="r2">removed: ' + esc(gone.join(', ')) + '</p>';
    var lost = Object.keys(VS.members).filter(function (id) { return !seen[id] && !VS.members[id].removed; });
    if (lost.length) h += '<p class="err">unreachable from any root: ' + esc(lost.join(', ')) + '</p>';
    h += '<div class="toolbar" style="margin-top:8px">' + btn('admin', 'Admin: roles event', {}) +
      '<span class="muted">' + (me.admin ? 'you are an admin' : 'flagged unless you are an admin') + '</span></div>';
    var nMem = Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).length;
    return card('Team', nMem + (nMem === 1 ? ' member' : ' members'), h, '', 'card-org');
  }

  function kindFilter(it) { return ui.kind === 'request' ? !!it.pending.length : ui.kind === 'all' || it.kind === ui.kind; }
  function itemFilter(it) {
    if (!kindFilter(it)) return false;
    var s = it.status;
    if (ui.status === 'hidden') return it.hidden;
    if (it.hidden) return false;
    if (ui.status === 'open') return s !== 'done' && s !== 'declined' && s !== 'returned';
    if (ui.status === 'done') return s === 'done';
    if (ui.status === 'blocked') return s === 'blocked' || it.rollup.blocked.length;
    return true;
  }
  function sel(id, val, list) {
    return '<select id="' + id + '">' + list.map(function (x) {
      var k = Array.isArray(x) ? x[0] : x, l = Array.isArray(x) ? x[1] : x;
      return '<option value="' + esc(k) + '"' + (k === val ? ' selected' : '') + '>' + esc(l) + '</option>';
    }).join('') + '</select>';
  }
  function itemFilterBar(kindOnly) {
    return 'kind ' + sel('f-kind', ui.kind, [['all', 'all'], ['task', 'task'], ['todo', 'todo'], ['topic', 'topic'], ['request', 'request (pending assignment)']]) +
      (kindOnly ? '' : ' status ' + sel('f-status', ui.status, ['all', 'open', 'blocked', 'done', 'hidden']));
  }
  function cardItems() {
    var m = modeOf('items'), rows = VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(m === 'kanban' ? function (it) { return !it.hidden && kindFilter(it); } : itemFilter);
    if (m !== 'list') return card('Items', rows.length + ' of ' + Object.keys(VS.items).length, modeBody('items', m), picker('items'), 'card-items', m);
    var h = '<div class="toolbar">' + itemFilterBar() + '</div>';
    h += rows.length ? rows.map(function (it) {
      var as = it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; });
      var who = as.length ? '<span class="muted">&rarr; ' + esc(as.length > 4 ? as.length + ' people' : as.map(function (a) { return a.to + (a.state === 'pending' ? '?' : ''); }).join(', ')) + '</span>' : '';
      return '<div class="row click"' + attr('item', it.id) + ' style="padding-left:' + it.depth * 14 + 'px"><div class="r1">' + (it.depth ? '<span class="muted">&#8627;</span>' : '') +
        stChip(it.status) + prChip(it) + titleRef(it.id) + dueChip(it) + who + rollupHtml(it) + (it.concurrent ? '<span class="chip conc-c">concurrent ' + it.concurrent + '</span>' : '') +
        '<span class="grow"></span>' + kindChip(it) + '</div>' +
        '<div class="r2">' + esc(it.alias) + ' by ' + esc(nm(it.author)) + (it.project ? ' &middot; ' + esc(it.project) : '') + ' &middot; ' + esc(fmtTs(it.lastActivity)) + '</div></div>';
    }).join('') : '<div class="empty">No items match.</div>';
    return card('Items', rows.length + ' of ' + Object.keys(VS.items).length, h, picker('items'), 'card-items', m);
  }

  function cardActivity() {
    var chip = VS.stats.events + ' events, ' + VS.stats.flagged + ' flagged' + (VS.stats.concurrent ? ', ' + VS.stats.concurrent + ' concurrent' : ''), m = modeOf('activity');
    if (m !== 'list') return card('Activity', chip, modeBody('activity', m), picker('activity'), 'card-activity', m);
    var rows = VS.events.filter(function (n) { return (!ui.act.flagged || n.status !== 'applied') && (!ui.act.conc || n.markers.length); }), cap = 50, shown = rows.slice(-cap).reverse();
    var h = lastWrite();
    h += '<div class="toolbar"><label><input type="checkbox" id="flagged-only"' + (ui.act.flagged ? ' checked' : '') + '> flagged only</label>' +
      '<label><input type="checkbox" id="conc-only"' + (ui.act.conc ? ' checked' : '') + '> concurrent only</label>' +
      '<span class="grow"></span><span class="muted">newest first, ' + shown.length + ' of ' + rows.length + '</span></div>';
    h += shown.length ? '<ul class="plain">' + shown.map(function (n) { return evLine(n, false); }).join('') + '</ul>' : '<div class="empty">Nothing to show.</div>';
    return card('Activity', chip, h, picker('activity'), 'card-activity', m);
  }
  function lastWrite() {
    if (!ui.last) return '';
    return '<div class="lastact">Last write (journal): ' + ui.last.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ' ' + (n.status === 'applied' ? '<span class="chip st-done">applied</span>' : '<span class="chip flag">' + esc(n.status) + '</span> ' + esc(n.reason));
    }).join('; ') + '</div>';
  }

  function fmtList(a) { return '[\n' + a.map(function (e) { return '  ' + JSON.stringify(e); }).join(',\n') + '\n]'; }
  function fmtRoles(r) {
    return '{\n  "admins": ' + JSON.stringify(r.admins || []) + ',\n' + (r.priority ? '  "priority": ' + JSON.stringify(r.priority) + ',\n' : '') + '  "members": [\n' +
      r.members.map(function (m) { return '    ' + JSON.stringify(m); }).join(',\n') + '\n  ]\n}';
  }
  function cardData() {
    var raw = stores[ui.store].raw();
    var h = '<details class="data" id="data-details"' + (ui.dataOpen ? ' open' : '') + '><summary>data (raw JSON, editable)</summary>' +
      '<p class="r2">Roles file (genesis), the pushed commits of every clone ref (the remote), and each clone\'s local state (what it fetched, its unpushed commits and open batch). Edit, then apply; edits live in memory until reload.</p>' +
      '<div class="toolbar"><button class="btn mini primary" data-act="apply">apply</button><button class="btn mini" data-act="reset">reset store to fixtures</button></div>' +
      '<div class="sub">roles file</div><textarea id="raw-roles" rows="12" spellcheck="false">' + esc(fmtRoles(raw.roles)) + '</textarea>';
    Object.keys(raw.remote).forEach(function (o) {
      var cs = raw.remote[o];
      h += '<details' + attr('owner', o) + (ui.openLogs[o] ? ' open' : '') + '><summary>remote ' + esc(E.refOf(o)) + ' (' + cs.length + ' commits)</summary>' +
        '<textarea' + attr('owner', o) + ' rows="' + Math.min(18, cs.length + 3) + '" spellcheck="false">' + esc(fmtList(cs)) + '</textarea></details>';
    });
    h += '<details' + attr('owner', '@clones') + (ui.openLogs['@clones'] ? ' open' : '') + '><summary>clone state (fetched counts, local commits, online)</summary>' +
      '<textarea id="raw-clones" rows="14" spellcheck="false">' + esc(JSON.stringify(raw.clones, null, 1)) + '</textarea></details>';
    return card('Data', 'debug', h + '</details>', '', 'card-data');
  }

  // ------------------------------------------------------------- view modes

  // (303) The kanban, activity timeline, due timeline, and calendar renderers draw a
  // card's body, scoped to that card's own rows and filters. A card in one of these
  // modes spans the full row of the cards layout (304).
  function modeBody(key, m, v) {
    if (m === 'kanban') return kanbanBody(key, v);
    if (m === 'gantt') return ganttBody(key, dueRows(key, v));
    if (m === 'calendar') return calendarBody(key, dueRows(key, v));
    if (m === 'activity') return activityBody();
    return '';
  }
  // The assignment records an assignment card shows: mine (live plus pending to me),
  // given (what I assigned or handed to others, live or pending), myteam (followers'
  // open and unaccepted work).
  function assignRecs(key, v) {
    if (key === 'mine') return VS.my.assignments.concat(VS.my.received.pending).map(A);
    if (key === 'given') return [].concat.apply([], VS.my.given.map(function (id) {
      return VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); });
    }));
    return [].concat.apply([], VS.my.team.map(function (p) { return p.open.concat(p.pending); })).map(A);
  }

  // Kanban: columns by status. Assignment cards: cards are assignments (with the
  // pending column); Items: cards are items. Dragging moves only the viewer's own
  // live assignment (item.status, or item.close on the done column).
  var KCOLS = { assign: ['pending', 'todo', 'doing', 'blocked', 'awaiting', 'done'],
    items: ['unassigned', 'pending', 'todo', 'doing', 'blocked', 'awaiting', 'done', 'declined', 'returned'] };
  function kanbanBody(key, v) {
    var cols = KCOLS[key === 'items' ? 'items' : 'assign'], buckets = {};
    cols.forEach(function (c) { buckets[c] = []; });
    if (key === 'items') {
      VS.itemOrder.forEach(function (id) {
        var it = VS.items[id];
        // the columns already are the statuses, so the Items status filter would only empty them: kind filter only
        if (it.hidden || it.kind === 'topic' || !buckets[it.status] || !kindFilter(it)) return;
        var mineR = it.assignments.map(A).filter(function (r) { return r.to === v && r.state === 'live'; })[0];
        buckets[it.status].push({ item: id, drag: mineR && mineR.status !== 'done' ? id : null,
          sub: (it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return r.to + (r.state === 'pending' ? '?' : ''); }).join(', ') || 'nobody') });
      });
    } else {
      assignRecs(key, v).forEach(function (r) {
        var col = r.state === 'pending' ? 'pending' : r.status;
        if (!buckets[col]) return;
        buckets[col].push({ item: r.item, rec: r, drag: r.to === v && r.state === 'live' && r.status !== 'done' ? r.item : null,
          sub: (key !== 'mine' ? esc(nm(r.to)) + ' &middot; ' : '') + source(r) });
      });
    }
    Object.keys(buckets).forEach(function (c) {
      buckets[c].sort(function (x, y) { var a = VS.items[x.item], b = VS.items[y.item]; return b.priorityLevel - a.priorityLevel || ((a.due || '9') < (b.due || '9') ? -1 : 1); });
    });
    var h = '<div class="toolbar">' + (key === 'items' ? itemFilterBar(true) : '') +
      '<span class="muted">drag a card to change the status of your own assignment (done = close)</span></div><div class="kanban">';
    h += cols.map(function (c) {
      return '<div class="kcol"' + attr('kcol', c) + '><div class="khead">' + stChip(c) + ' <span class="muted">' + buckets[c].length + '</span></div>' +
        buckets[c].map(function (x) {
          var it = VS.items[x.item];
          return '<div class="kcard' + (x.drag ? ' drag' : '') + '"' + (x.drag ? ' draggable="true"' + attr('drag', x.drag) : '') + attr('item', x.item) + '>' +
            '<div class="r1">' + prChip(it) + titleRef(x.item) + dueChip(it) + '</div><div class="r2">' + x.sub + '</div></div>';
        }).join('') + '</div>';
    }).join('');
    return h + '</div>' + lastWrite();
  }

  var FAMILIES = [['all', 'all events'], ['item', 'item fields and status'], ['assign', 'assignment (assign, accept, decline, return, handoff, unassign)'], ['comment', 'comments'], ['roles', 'roles']];
  function family(t) {
    if (/^roles\./.test(t)) return 'roles';
    if (/^comment\./.test(t)) return 'comment';
    if (/^item\.(assign|accept|decline|return|handoff|unassign)$/.test(t)) return 'assign';
    return 'item';
  }
  function activityBody() {
    var f = ui.act, rows = VS.events.filter(function (n) {
      return (f.family === 'all' || family(n.type) === f.family) && (f.member === 'all' || n.member === f.member) &&
        (!f.flagged || n.status !== 'applied') && (!f.conc || n.markers.length);
    }).slice().sort(function (a, b) { return String(b.ts) < String(a.ts) ? -1 : String(b.ts) > String(a.ts) ? 1 : 0; });
    var people = [['all', 'everyone']].concat(Object.keys(VS.members).map(function (id) { return [id, VS.members[id].name]; }));
    var h = '<div class="toolbar">' + sel('a-family', f.family, FAMILIES) + sel('a-member', f.member, people) +
      '<label><input type="checkbox" id="flagged-only"' + (f.flagged ? ' checked' : '') + '> flagged only</label>' +
      '<label><input type="checkbox" id="conc-only"' + (f.conc ? ' checked' : '') + '> concurrent only</label><span class="grow"></span><span class="muted">' + rows.length + ' events, by write time, newest first</span></div>';
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
  // The dated rows a due timeline or calendar draws for a card: one per item on
  // Items (its filters apply), one per assignment record on an assignment card,
  // with the record's own state and, off "mine", the assignee.
  function dueRows(key, v) {
    function dated(it) { return it && it.due && !it.hidden && it.kind !== 'topic'; }
    if (key === 'items') return VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(function (it) { return dated(it) && itemFilter(it); })
      .map(function (it) { return { item: it.id, status: it.status, chip: stChip(it.status), who: '' }; });
    return assignRecs(key, v).filter(function (r) { return dated(VS.items[r.item]); }).map(function (r) {
      return { item: r.item, status: r.state === 'live' ? r.status : r.state, chip: recChip(r), who: key === 'mine' ? '' : nm(r.to) };
    });
  }
  function whoTag(x) { return x.who ? '<span class="muted">' + esc(x.who) + '</span>' : ''; }
  // Due-date timeline: one bar per row, from the item's start (first doing, else
  // creation) to its due day.
  function ganttBody(key, rows) {
    var bar = key === 'items' ? itemFilterBar() : '';
    if (!rows.length) return '<div class="toolbar">' + bar + '</div><div class="empty">No items with a due date match.</div>';
    var t0 = dayNum(today()), lo = t0, hi = t0;
    rows.forEach(function (x) { var it = VS.items[x.item]; lo = Math.min(lo, dayNum(it.started || it.ts), dayNum(it.due)); hi = Math.max(hi, dayNum(it.due)); });
    hi += 1;
    var span = hi - lo, pct = function (d) { return ((d - lo) / span * 100).toFixed(2) + '%'; };
    var axis = '';
    for (var d = lo; d < hi; d++) axis += '<span class="gtick' + (d === t0 ? ' now' : '') + '" style="left:' + pct(d) + '">' + (new Date(d * 86400000).getUTCDay() === 1 || d === lo ? esc(dayStr(d).slice(5)) : '') + '</span>';
    var h = '<div class="toolbar">' + bar + '<span class="muted">' + rows.length + ' with a due date; bar: start (first doing, else creation) to due; the line is today (' + esc(today()) + ')</span></div>' +
      '<div class="gantt"><div class="grow-axis"><div class="glabel"></div><div class="gtrack axis">' + axis + '</div></div>';
    h += rows.map(function (x) {
      var it = VS.items[x.item], s = dayNum(it.started || it.ts), e = dayNum(it.due) + 1, over = it.due < today() && x.status !== 'done';
      return '<div class="grow-row click"' + attr('item', it.id) + '><div class="glabel">' + prChip(it) + x.chip + titleRef(it.id) + whoTag(x) + '</div>' +
        '<div class="gtrack"><span class="gnow" style="left:' + pct(t0) + '"></span><span class="gbar st-' + esc(x.status) + (over ? ' over' : '') + '" style="left:' + pct(Math.min(s, e - 1)) + ';width:' + ((Math.max(1, e - Math.min(s, e - 1))) / span * 100).toFixed(2) + '%" title="' +
        esc((it.started ? 'started ' : 'created ') + String(it.started || it.ts).slice(0, 10) + ' - due ' + it.due) + '">' + esc(it.due.slice(5)) + '</span></div></div>';
    }).join('');
    return h + '</div>';
  }

  function calendarBody(key, rows) {
    var m = ui.calMonth[key] || today().slice(0, 7), first = dayNum(m + '-01'), dow = (new Date(first * 86400000).getUTCDay() + 6) % 7;
    var y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7)), days = new Date(Date.UTC(y, mo, 0)).getUTCDate();
    var byDay = {};
    rows.forEach(function (x) { var due = VS.items[x.item].due; (byDay[due] = byDay[due] || []).push(x); });
    var prev = new Date(Date.UTC(y, mo - 2, 1)).toISOString().slice(0, 7), next = new Date(Date.UTC(y, mo, 1)).toISOString().slice(0, 7);
    var n = Object.keys(byDay).filter(function (k) { return k.slice(0, 7) === m; }).reduce(function (s, k) { return s + byDay[k].length; }, 0);
    var h = '<div class="toolbar">' + btn('cal', '&lsaquo;', { month: prev, card: key }) + '<b>' + esc(m) + '</b>' + btn('cal', '&rsaquo;', { month: next, card: key }) +
      btn('cal', 'today', { month: today().slice(0, 7), card: key }) + (key === 'items' ? itemFilterBar() : '') + '<span class="muted">' + n + ' due in ' + esc(m) + '</span></div>' +
      '<div class="cal">' + ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map(function (d) { return '<div class="cdow">' + d + '</div>'; }).join('');
    for (var i = 0; i < dow; i++) h += '<div class="cday off"></div>';
    for (var d = 1; d <= days; d++) {
      var ds = m + '-' + (d < 10 ? '0' : '') + d, list = byDay[ds] || [];
      h += '<div class="cday' + (ds === today() ? ' now' : '') + '"' + attr('day', ds) + '><div class="cnum">' + d + '</div>' + list.map(function (x) {
        return '<div class="citem click"' + attr('item', x.item) + '>' + x.chip + prChip(VS.items[x.item]) + titleRef(x.item) + whoTag(x) + '</div>';
      }).join('') + '</div>';
    }
    return h + '</div>';
  }

  // ------------------------------------------------------- chat card (placeholder)

  // (300) (302) (305) A placeholder for the support-agent chat child (the future
  // daemon-side local support agent): placement and a static sample only, with no
  // responder and no events. Entry shapes follow the dashboard conversation view
  // (user text, assistant text, a collapsed tool line) plus a tracker card reference
  // that renders an item from the view state and opens its thread sheet.
  function crefHtml(id) {
    var it = VS.items[id];
    var who = it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return nm(r.to) + (r.state === 'pending' ? '?' : ''); });
    return '<div class="cref click"' + attr('item', id) + attr('cref', id) + ' title="open the thread"><div class="r1">' + stChip(it.status) + prChip(it) + dueChip(it) +
      '<span class="grow"></span><span class="al">' + esc(it.alias) + '</span></div><div class="ct">' + esc(it.title) + '</div>' +
      '<div class="r2">&rarr; ' + esc(who.join(', ') || 'nobody') + '</div></div>';
  }
  // Real item ids from the current viewer's view state, so the references resolve:
  // the top open work first, then what waits for an answer or confirm, then what I gave.
  function chatRefs() {
    var pick = [];
    function add(id) { var it = VS.items[id]; if (it && !it.hidden && it.kind !== 'topic' && pick.indexOf(id) < 0) pick.push(id); }
    VS.my.today.work.concat(VS.my.today.pending, VS.my.today.confirms).forEach(function (id) { add(A(id).item); });
    VS.my.given.forEach(add);
    VS.itemOrder.forEach(add);
    return pick;
  }
  function chatMsg(who, label, html) {
    return '<div class="msg ' + who + '"><div class="mh"><b>' + label + '</b> ' + esc(fmtTs(ui.clock[ui.store]).slice(6)) + '</div><div class="bub">' + html + '</div></div>';
  }
  function cardChat() {
    var refs = chatRefs(), a = refs[0], b = refs[1];
    var log = '<div class="sysline">placeholder: support-agent chat child, a static sample (no responder, no events)</div>' +
      chatMsg('user', 'You', 'What should I look at first?') +
      '<details class="toolsline"><summary><span class="tsum">2 tools &middot; tracker view, tracker items</span><svg class="i"><use href="#i-chev"/></svg></summary>' +
      '<div class="brows"><div class="brow"><span class="tool">tracker view</span> ' + esc(nm(memberOf(viewerOf(ui.store)))) + '</div>' +
      '<div class="brow"><span class="tool">tracker items</span> open, by priority then due</div></div></details>' +
      chatMsg('lead', 'Support agent', a ? 'This one first, by priority and then due:' + crefHtml(a) : 'Nothing in this store involves you right now.') +
      chatMsg('user', 'You', 'Anything else?') +
      chatMsg('lead', 'Support agent', b ? 'This one is next:' + crefHtml(b) : 'Nothing else right now.');
    var comp = '<div class="chat-comp"><textarea rows="1" placeholder="Ask the support agent... (placeholder)" disabled></textarea>' +
      '<button class="csend" disabled><svg class="i"><use href="#i-send"/></svg></button></div>';
    return card('Support agent', 'placeholder', '<div class="chat-log">' + log + '</div>' + comp, '', 'card-chat', '', 'chat');
  }

  // --------------------------------------------------------------- sync state

  function syncHtml(name) {
    var vs = vss[name], s = vs.sync, remote = vs.store.remote, events = s.unpushed.reduce(function (a, c) { return a + c.events.length; }, 0);
    var h = '<div class="sync' + (remote && s.online ? '' : ' off') + '">';
    h += '<span class="muted">clone <code>' + esc(E.refOf(vs.viewer.stream)) + '</code></span> &middot; ';
    if (!remote) h += '<span class="sdot"></span><b>no remote</b> &middot; local only (same path as a long offline)';
    else h += '<span class="sdot"></span><b>' + (s.online ? 'online' : 'offline') + '</b> &middot; last synced ' + esc(fmtTs(s.syncedAt)) + (s.online ? '' : ' &middot; will retry');
    h += ' &middot; <span' + (events ? ' class="unp"' : '') + '>' + events + ' of my writes not yet pushed</span>';
    if (remote && s.upstream) h += ' &middot; <span class="muted" title="commits other clones pushed that this clone has not fetched">' + s.upstream + ' upstream commits not fetched</span>';
    if (remote) h += btn('sync', 'Sync now', {}) + btn(s.online ? 'go-offline' : 'go-online', s.online ? 'Go offline' : 'Come back online', {}, s.online ? '' : 'primary');
    else h += btn('sync', 'Close batch', {});
    h += '</div>';
    if (s.unpushed.length) {
      h += '<details class="unpushed"><summary>unpushed: ' + s.unpushed.map(function (c) { return c.open ? 'open batch (' + c.events.length + ')' : 'commit (' + c.events.length + ')'; }).join(', ') + '</summary><ul class="plain">' +
        s.unpushed.map(function (c) {
          return '<li><b>' + (c.open ? 'open batch' : 'closed commit') + '</b> <span class="muted">' + esc(fmtTs(c.ts)) + '</span>: ' + c.events.map(function (id) {
            var n = EV[id]; return esc(alias(id)) + (n ? ' ' + esc(n.type) : '');
          }).join(', ') + '</li>';
        }).join('') + '</ul><p class="r2">The open batch closes into one signed commit when this clone receives new events (its seen would change) or at push.</p></details>';
    }
    return h;
  }

  // The drawer has one layout, the cards layout (303). A card in a wide mode leaves its
  // column and spans the full row above the columns; the remaining cards flow below (304).
  var LAYOUT = [['notify', 'today', 'mine', 'received', 'given'], ['myteam', 'items'], ['chat', 'org', 'activity', 'data']];
  function drawerHtml(name, v) {
    var vs = VS, nMem = Object.keys(vs.members).filter(function (id) { return !vs.members[id].removed; }).length;
    var CARDS = { notify: cardNotify, today: cardToday, mine: cardMine, received: cardRequests, given: cardGiven, myteam: cardMyTeam, items: cardItems,
      chat: cardChat, org: cardTeam, activity: cardActivity, data: cardData };
    var h = '<div class="dr-title"><b>' + esc(vs.store.label) + '</b><span>' + nMem + ' members, ' + vs.stats.events + ' events from ' + vs.stats.commits + ' commits, ' +
      vs.stats.flagged + ' flagged</span><span class="grow"></span>' +
      '<button class="btn primary" data-act="create"><svg class="i"><use href="#i-plus"/></svg>New</button></div>' + syncHtml(name);
    if (!vs.viewer.isMember) h += '<p class="err">' + esc(v) + ' is not a member of this store (removed or never joined): writes are flagged.</p>';
    var wide = '', cols = LAYOUT.map(function (col) {
      return col.map(function (k) {
        var html = CARDS[k](v);
        if (html && isWide(k)) { wide += html; return ''; }
        return html;
      }).join('');
    }).filter(Boolean);
    return h + (wide ? '<div class="dr-wide">' + wide + '</div>' : '') +
      '<div class="dr-cols' + (cols.length < LAYOUT.length ? ' n' + cols.length : '') + '">' + cols.map(function (c) { return '<div class="dr-col">' + c + '</div>'; }).join('') + '</div>';
  }

  // ------------------------------------------------------------ thread sheet

  function commentHtml(n, v, it) {
    var p = n.payload, c = VS.comments[n.id] || {}, body = c.body != null ? c.body : p.body, reply = '';
    if (p.reply_to) {
      var q = VS.comments[p.reply_to] || (EV[p.reply_to] && { author: EV[p.reply_to].member, body: EV[p.reply_to].payload.body });
      reply = '<div class="quote">&#8617; ' + esc(alias(p.reply_to)) + ' ' + (q ? '<b>' + esc(q.author) + '</b>: ' + (q.hidden ? '<i>hidden comment</i>' : esc(String(q.body).slice(0, 90))) : '') + '</div>';
    }
    var acts = '';
    if (n.status === 'applied' && !c.hidden && !it.hidden) {
      acts += btn('reply', 'Reply', { cid: n.id });
      if (n.member === v) acts += btn('cedit', 'Edit', { cid: n.id }) + btn('chide', 'Hide', { cid: n.id });
    }
    var live = n.status === 'applied' && !!VS.comments[n.id];
    var txt = c.hidden ? '<div class="hiddenc">comment hidden by its author (' + esc(alias(c.hidden)) + ')<details><summary>history keeps the text</summary>' + esc(body) + '</details></div>' :
      '<div class="ctext">' + esc(body) + '</div>' + mentionChips(live ? c.mentions : p.mentions) + attChips(live ? c.attachments : p.attachments, live);
    return '<li class="cmt' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (n.member === v ? ' mine' : '') + '"' + attr('ev', n.id) + '>' +
      '<div class="ch"><b>' + esc(nm(n.member)) + '</b>' + originChip(n.origin) + '<span class="when">' + esc(fmtTs(n.ts)) + '</span><span class="al">' + esc(n.alias) + '</span>' +
      (c.edited ? '<span class="chip">edited</span>' : '') + flags(n) + '<span class="grow"></span>' + acts + '</div>' + reply + txt +
      (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(n) + supHtml(n) + '</li>';
  }

  function sheetHtml(v, it) {
    var recs = it.assignments.map(A), mineR = recs.filter(function (r) { return r.to === v && r.state === 'live'; })[0];
    var minePend = recs.filter(function (r) { return r.to === v && r.state === 'pending'; })[0];
    var acts = '', gone = VS.viewer.admin && VS.members[it.author] && VS.members[it.author].removed; // (292)
    if (minePend) acts += btn('accept', 'Accept assignment', { item: it.id }, 'primary') + btn('note', 'Decline', { item: it.id, op: 'item.decline' });
    if ((it.author === v || gone) && !it.hidden) acts += btn('edit', it.author === v ? 'Edit' : 'Edit (admin: creator removed)', { item: it.id });
    if (it.author === v && it.kind !== 'topic' && !it.hidden) acts += btn('assign', 'Assign', { item: it.id });
    if (mineR && mineR.status !== 'done' && !it.hidden) acts += btn('handoff', 'Hand off...', { item: it.id });
    if ((mineR || it.author === v) && it.kind !== 'topic' && !it.hidden) acts += btn('split', 'Split into subtask', { item: it.id });
    if ((it.author === v || gone) && !it.hidden) acts += btn('ihide', it.author === v ? 'Hide item' : 'Hide item (admin)', { item: it.id });

    var h = '<div class="sh-top"><div class="sh-head"><div>' + kindChip(it) + '</div><h2>' + esc(it.title) + '</h2>' +
      '<button class="btn mini" data-act="sheet-close" title="close (Esc)"><svg class="i"><use href="#i-x"/></svg></button></div>';
    h += '<dl class="sh-meta"><dt>id</dt><dd><code>' + esc(it.alias) + '</code> <span class="muted">(' + esc(it.id) + ')</span></dd>' +
      '<dt>status</dt><dd>' + (it.kind === 'topic' ? '<span class="muted">topic: discussion only</span>' : stChip(it.status) + ' ' + rollupHtml(it)) + '</dd>' +
      (it.kind !== 'topic' ? '<dt>priority</dt><dd>' + prChip(it) + '</dd><dt>due</dt><dd>' + (it.due ? dueChip(it) + ' ' + esc(it.due) : '<span class="muted">-</span>') + '</dd>' : '') +
      '<dt>creator</dt><dd>' + esc(nm(it.author)) + (VS.members[it.author] && VS.members[it.author].admin ? ' <span class="chip admin">admin</span>' : '') + '</dd>' +
      '<dt>project</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>parent</dt><dd>' + (it.parent ? itemRef(it.parent) + ' ' + esc((VS.items[it.parent] || {}).title || '') : '<span class="muted">-</span>') + '</dd>' +
      '<dt>created</dt><dd>' + esc(fmtTs(it.ts)) + (it.edited ? ' &middot; edited ' + esc(fmtTs(it.edited.ts)) : '') + '</dd></dl>';
    h += it.body ? '<div class="sh-body">' + esc(it.body) + '</div>' : '';
    h += mentionChips(it.mentions) + attChips(it.attachments, true);
    h += '<div class="sh-actions">' + (acts || '<span class="muted">No item actions for ' + esc(nm(v)) + '.</span>') + '</div>';

    if (it.kind !== 'topic') {
      h += '<h3>Assignees</h3>';
      if (!recs.length) h += '<div class="empty">Not assigned.</div>';
      else h += '<table class="atable">' + recs.map(function (r) {
        var b = '', lead = VS.members[r.to] && VS.members[r.to].leads[0];
        if (r.state === 'live') {
          if (r.by === v && r.status === 'awaiting') b += btn('confirm', 'Confirm', { item: r.item, who: r.to }, 'primary') + btn('note', 'Reject', { item: r.item, who: r.to, op: 'item.reject' });
          if ((r.by === v || r.to === v) && r.status === 'done') b += btn('note', 'Reopen', { item: r.item, who: r.to, op: 'item.reopen' });
          var departed = (VS.members[r.to] || {}).removed || (VS.members[r.by] || {}).removed;
          if (r.by === v || (departed && VS.viewer.admin)) b += btn('unassign', r.by === v ? 'Unassign' : 'Unassign (admin, member removed)', { item: r.item, who: r.to });
          if (r.accepted && lead === v) b += btn('note', 'Return (lead of ' + esc(r.to) + ')', { item: r.item, who: r.to, op: 'item.return' });
          if (VS.viewer.admin && r.to !== v && r.status !== 'done' && !it.hidden) b += btn('handoff', 'Hand off (admin)', { item: r.item, who: r.to }); // (286)
        } else if (r.state === 'pending') {
          if (r.to === v) b += btn('accept', 'Accept', { item: r.item }, 'primary') + btn('note', 'Decline', { item: r.item, op: 'item.decline' });
          if (r.by === v || r.sender === v) b += btn('unassign', 'Withdraw', { item: r.item, who: r.to });
        }
        return '<tr class="' + (r.state !== 'live' && r.state !== 'pending' ? 'gone' : '') + '"' + attr('rec', r.id) + '><td><b>' + esc(nm(r.to)) + '</b><br><span class="muted">' + source(r) + '</span></td><td>' +
          recChip(r) + (r.ended ? ' <span class="muted">' + esc(alias(r.ended.id)) + endedBy(r) + '</span>' : '') +
          '<br><span class="muted">' + complText(r.completion) + '</span></td><td>' + (r.note ? '&ldquo;' + esc(r.note) + '&rdquo;' : r.answerNote ? '&ldquo;' + esc(r.answerNote) + '&rdquo;' : '<span class="muted">no note</span>') +
          '<div class="tb">' + b + '</div></td></tr>';
      }).join('') + '</table>';
    }
    if (it.children.length) {
      h += '<h3>Subtask threads</h3><div class="stree">' + it.children.map(function (k) {
        var ki = VS.items[k], who = ki.assignments.map(A).filter(function (r) { return r.state === 'live'; }).map(function (r) { return r.to; });
        return '<div class="click"' + attr('item', k) + '>' + stChip(ki.status) + prChip(ki) + titleRef(k) + ' <span class="muted">' + esc(ki.author) +
          (who.length ? ' &rarr; ' + esc(who.join(', ')) : '') + '</span>' + rollupHtml(ki) + kindChip(ki) + '</div>';
      }).join('') + '</div>';
    }
    var evs = it.thread.map(function (id) { return EV[id]; });
    h += '<h3>Thread (' + evs.length + ' events)</h3></div><ul class="timeline">' + evs.map(function (n) {
      if (n.type === 'comment.add' && n.thread === it.id) return commentHtml(n, v, it);
      return evLine(n, n.parentThread === it.id ? 'parent' : true);
    }).join('') + '</ul>';

    // composer: comment (optionally a quote reply) plus quick status actions on my own assignment
    var rt = ui.replyTo && VS.comments[ui.replyTo] && VS.comments[ui.replyTo].item === it.id ? VS.comments[ui.replyTo] : null;
    var quick = '';
    if (mineR && mineR.status !== 'done') {
      quick = '<span class="muted">my status ' + stChip(mineR.status) + ' set:</span>' +
        ['todo', 'doing', 'blocked'].filter(function (s) { return s !== mineR.status; }).map(function (s) { return btn('q-status', s, { item: it.id, status: s, note: 1 }); }).join('') +
        (mineR.status !== 'awaiting' ? btn('q-close', 'Close', { item: it.id }) : '') + '<span class="muted">(text below becomes the note)</span>';
    }
    h += '<div class="composer2">' + (it.hidden ? '<span class="muted">thread hidden; comments are flagged</span>' : '') +
      (rt ? '<div class="replying">replying to ' + esc(alias(rt.id)) + ' <b>' + esc(rt.author) + '</b>: ' + esc(String(rt.body).slice(0, 70)) + ' ' + btn('reply-x', 'x', {}) + '</div>' : '') +
      (quick ? '<div class="quick">' + quick + '</div>' : '') + draftHtml() +
      '<div class="crow"><textarea id="cbox" rows="2" placeholder="Comment as ' + esc(nm(v)) + '..."></textarea><div class="ccol">' +
      '<select id="corigin" title="origin"><option value="human">human</option><option value="agent">agent</option><option value="agent-auto">agent-auto</option></select>' +
      btn('comment', 'Post', { item: it.id }, 'primary') + '</div></div>' +
      // (335) a simple @ picker; (338) attachment metadata typed in by hand (the mock computes a stand-in oid)
      '<div class="cextra"><select id="cmention" title="mention a member"><option value="">@ mention...</option>' +
      Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).map(function (id) { return '<option value="' + esc(id) + '">@' + esc(id) + ' ' + esc(VS.members[id].name) + '</option>'; }).join('') +
      '</select><input id="catt-name" type="text" placeholder="attach (mock): file name"><input id="catt-size" type="number" min="0" placeholder="bytes">' +
      btn('att-add', 'Attach', {}) + '<span class="muted">limit ' + fmtSize(VS.attachments.max_bytes) + '</span></div></div>';
    return h;
  }
  function draftHtml() {
    if (!ui.mdraft.length && !ui.adraft.length) return '';
    return '<div class="cdraft">' + ui.mdraft.map(function (id) { return '<span class="chip mention">@' + esc(nm(id)) + '</span>'; }).join('') +
      ui.adraft.map(function (x, i) { return '<span class="chip att">file ' + esc(x.name) + ' &middot; ' + fmtSize(x.size) + '</span>' + btn('att-x', 'x', { idx: i }); }).join('') +
      '<span class="muted">sent with the next post (a mention counts while its @id stays in the text)</span></div>';
  }
  // A stand-in for the sha256 of the content: the mock has no content, so it hashes
  // the name and size into 64 hex digits. The real client hashes the file.
  function mockOid(s) {
    var h = '', x = 2166136261;
    for (var r = 0; r < 8; r++) { for (var i = 0; i < s.length; i++) x = Math.imul(x ^ s.charCodeAt(i) ^ r, 16777619) >>> 0; h += ('0000000' + x.toString(16)).slice(-8); }
    return h;
  }
  var MIME = { png: 'image/png', jpg: 'image/jpeg', pdf: 'application/pdf', txt: 'text/plain', zip: 'application/zip', zst: 'application/zstd', md: 'text/markdown' };
  function mimeOf(name) { return MIME[String(name).split('.').pop().toLowerCase()] || 'application/octet-stream'; }
  // An edit of a body keeps the mentions whose @id is still in the text and every attachment.
  function keptMentions(ids, body) { return listOf(ids).filter(function (id) { return body.indexOf('@' + id) >= 0; }); }

  // --------------------------------------------------------- dialog forms

  function optsHtml(list, empty, cur) {
    if (!list.length) return '<option value="">(' + empty + ')</option>';
    return list.map(function (x) { return '<option value="' + esc(x[0]) + '"' + (String(x[0]) === String(cur) ? ' selected' : '') + '>' + esc(x[1]) + '</option>'; }).join('');
  }
  function memberOpts(cur, filter) {
    return optsHtml(Object.keys(VS.members).filter(function (id) { return !filter || filter(VS.members[id]); }).map(function (id) { return [id, VS.members[id].name + ' (' + id + ')']; }), 'nobody', cur);
  }
  function reachTag(v, id) { return id === v ? 'self' : VS.viewer.reach[id] ? 'forced, L' + VS.viewer.reach[id] : 'pending'; }
  // Anyone in the store: below me = forced (live at once); anyone else = pending until accepted.
  function peopleHtml(v) {
    var R = VS.viewer.reach, ids = Object.keys(VS.members).filter(function (id) { return id !== v && !VS.members[id].removed; });
    ids.sort(function (a, b) { return (R[a] || 99) - (R[b] || 99) || (a < b ? -1 : 1); });
    return '<label><input type="checkbox" name="people" value="' + esc(v) + '"> myself</label>' + ids.map(function (id) {
      return '<label><input type="checkbox" name="people" value="' + esc(id) + '"> ' + esc(nm(id)) + ' <span class="muted">' + reachTag(v, id) + '</span></label>';
    }).join('');
  }
  function levelSel() { return '<select name="levels"><option>1</option><option>2</option><option>3</option></select>'; }
  function complRadios() {
    return '<div class="opts"><label><input type="radio" name="completion" value="assignee" checked> assignee closes</label>' +
      '<label><input type="radio" name="completion" value="assigner"> assigner confirms</label></div>';
  }
  function prioSel(cur) {
    return '<select name="priority">' + VS.priority.names.map(function (x, i) { return '<option value="' + i + '"' + (i === cur ? ' selected' : '') + '>' + i + ' ' + esc(x) + '</option>'; }).join('') + '</select>';
  }
  function originSel() { return '<select name="origin"><option value="human">human</option><option value="agent">agent (on request)</option><option value="agent-auto">agent-auto (unprompted)</option></select>'; }
  function fld(label, inner) { return '<div class="fld"><span>' + label + '</span>' + inner + '</div>'; }
  var ROLE_TYPES = ['roles.add_member', 'roles.set_lead', 'roles.add_admin', 'roles.remove_admin', 'roles.add_key', 'roles.revoke_key', 'roles.remove_member', 'roles.set_priority_levels', 'roles.set_attachment_limit'];

  function dialogHtml(kind, d) {
    var v = memberOf(viewerOf(ui.store)), it = d.item && VS.items[d.item];
    var mineOpen = VS.my.assignments.map(A).filter(function (r) { return r.status !== 'done'; }).map(function (r) { return [r.item, alias(r.item) + ' ' + VS.items[r.item].title]; });
    var T;
    switch (kind) {
      case 'create':
        T = ['New item', 'One write to ' + esc(viewerOf(ui.store)) + '\'s journal: item.create, plus item.assign when someone is chosen. Someone below you is a forced assignment; anyone else gets a pending assignment (a request) they accept or decline.',
          fld('kind', '<select name="kind"><option>task</option><option>todo</option><option>topic</option></select>') +
          fld('title', '<input type="text" name="title" required>') + fld('body', '<textarea name="body" rows="3"></textarea>') +
          fld('project reference (free text, optional)', '<input type="text" name="project">') +
          '<div class="fld inline">' + fld('due (optional)', '<input type="date" name="due">') + fld('priority', prioSel(VS.priority.default)) + '</div>' +
          fld('assign to (ignored for topics)', '<select name="assignee"><option value="">(nobody)</option><option value="' + esc(v) + '" selected>myself</option>' +
            Object.keys(VS.members).filter(function (id) { return id !== v && !VS.members[id].removed; }).map(function (id) { return '<option value="' + esc(id) + '">' + esc(nm(id)) + ' (' + reachTag(v, id) + ')</option>'; }).join('') + '</select>') +
          fld('completion', complRadios()), 'Create'];
        break;
      case 'edit':
        T = ['Edit ' + it.alias, 'item.edit by the creator only; the thread keeps the old text. Others comment instead.',
          fld('title', '<input type="text" name="title" value="' + esc(it.title) + '">') + fld('body', '<textarea name="body" rows="4">' + esc(it.body) + '</textarea>') +
          fld('project', '<input type="text" name="project" value="' + esc(it.project) + '">') +
          '<div class="fld inline">' + fld('due', '<input type="date" name="due" value="' + esc(it.due || '') + '">') + fld('priority', prioSel(it.priority)) + '</div>', 'Save'];
        break;
      case 'assign':
        T = ['Assign ' + it.alias, 'item.assign by the creator. Chosen people below you are forced (live at once); anyone else gets a pending assignment. "Everyone within N" is forced-only and stays live: later followers inherit it.',
          fld('who', '<div class="opts"><label><input type="radio" name="mode" value="chosen" checked> chosen people</label><label><input type="radio" name="mode" value="all"> everyone within N levels</label></div>') +
          fld('N levels below me (everyone only)', levelSel()) + fld('completion', complRadios()) +
          fld('chosen people', '<div class="people" id="people">' + peopleHtml(v) + '</div>') + '<div class="hint" id="hint"></div>', 'Assign'];
        break;
      case 'split':
        T = ['Split ' + it.alias + ' into a subtask', 'One write: item.create (parent ' + it.alias + ') plus item.assign. Delegating down goes through a subtask under your name.',
          fld('subtask title', '<input type="text" name="title" required>') +
          '<div class="fld inline">' + fld('due (optional)', '<input type="date" name="due" value="' + esc(it.due || '') + '">') + fld('priority', prioSel(it.priorityLevel)) + '</div>' +
          fld('completion', complRadios()) + fld('assign to', '<div class="people" id="people">' + peopleHtml(v) + '</div>') + '<div class="hint" id="hint"></div>', 'Split'];
        break;
      case 'handoff':
        var hfrom = d.who || v, hrec = it.assignments.map(A).filter(function (r) { return r.to === hfrom && r.state === 'live'; })[0] || {};
        T = ['Hand off ' + (hfrom === v ? '' : esc(nm(hfrom)) + '\'s assignment on ') + it.alias,
          'item.handoff: a pending assignment to the new person, starting with the current status and note. When they accept, ' + (hfrom === v ? 'your' : esc(nm(hfrom)) + '\'s') +
          ' assignment ends and the assigner (' + esc(nm(hrec.by)) + ') is notified.' + (hfrom === v ? '' : ' Admin only: takeover of a member\'s work before removal.'),
          fld('to', '<select name="to">' + memberOpts('', function (m) { return m.id !== hfrom && !m.removed; }) + '</select>') + fld('note (optional)', '<input type="text" name="note">') +
          '<input type="hidden" name="who" value="' + esc(d.who || '') + '">', 'Hand off'];
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
          'item.decline': ['Decline the assignment', 'Ends the pending assignment as declined. Questions belong in the thread as comments.'],
          'item.return': ['Return ' + (d.who || '') + '\'s accepted assignment', 'Only the assignee\'s direct lead returns an accepted assignment; it ends it. The assigner may send again.'] }[d.op];
        T = [L[0] + ' (' + it.alias + ')', d.op + ': ' + L[1], fld('note', '<input type="text" name="note">') + '<input type="hidden" name="op" value="' + esc(d.op) + '">' +
          '<input type="hidden" name="who" value="' + esc(d.who || '') + '">', 'Send'];
        break;
      }
      case 'cedit': {
        var cm = VS.comments[d.cid];
        T = ['Edit comment ' + alias(d.cid), 'comment.edit; only its author edits it.', fld('text', '<textarea name="body" rows="4">' + esc(cm.body) + '</textarea>'), 'Save'];
        break;
      }
      case 'admin':
        T = ['Roles event', 'Admin-only. Written to ' + esc(viewerOf(ui.store)) + '\'s own journal; replay applies it to the roles file in causal order.',
          fld('type', '<select name="rtype">' + ROLE_TYPES.map(function (t) { return '<option>' + t + '</option>'; }).join('') + '</select>') +
          fld('member (existing)', '<select name="member">' + memberOpts('') + '</select>') +
          fld('lead (set_lead, add_member)', '<select name="lead"><option value="">(none: root)</option>' + memberOpts(v) + '</select>') +
          fld('key (add_key, revoke_key, add_member)', '<input type="text" name="key" placeholder="e.g. jon-k3">') +
          fld('new member id and name (add_member)', '<div class="fld inline"><input type="text" name="nid" placeholder="id"><input type="text" name="nname" placeholder="name"></div>') +
          fld('priority level names, lowest first (set_priority_levels)', '<input type="text" name="pnames" value="' + esc(VS.priority.names.join(', ')) + '">') +
          fld('default level (set_priority_levels)', '<input type="number" name="pdefault" min="0" value="' + VS.priority.default + '">') +
          fld('attachment limit in bytes (set_attachment_limit)', '<input type="number" name="maxbytes" min="1" value="' + VS.attachments.max_bytes + '">'), 'Append'];
        break;
    }
    return '<form' + attr('kind', kind) + attr('fitem', d.item || '') + attr('fcid', d.cid || '') + '><h2>' + T[0] + '</h2><p>' + T[1] + '</p>' + T[2] +
      '<div class="btns"><button type="button" class="btn" data-act="dlg-cancel">Cancel</button><button type="submit" class="btn primary">' + T[3] + '</button></div></form>';
  }

  function updateHint(form) {
    var hint = form.querySelector('#hint'), people = form.querySelector('#people');
    if (!hint) return;
    var v = memberOf(viewerOf(ui.store)), kind = form.getAttribute('data-kind');
    var mode = kind === 'split' ? 'chosen' : form.elements.mode.value, msg, warn = false;
    var chosen = Array.prototype.map.call(form.querySelectorAll('input[name="people"]:checked'), function (e) { return e.value; });
    Array.prototype.forEach.call(form.querySelectorAll('input[name="people"]'), function (e) { e.disabled = mode === 'all'; });
    people.classList.toggle('off', mode === 'all');
    if (mode === 'all') {
      var n = Number(form.elements.levels.value), ids = Object.keys(VS.viewer.reach).filter(function (id) { return VS.viewer.reach[id] <= n; });
      msg = ids.length ? 'Forced on ' + ids.length + ' people within L' + n + ' now, and on whoever joins that range later.' : 'Nobody is within L' + n + ' below you; the event will be flagged.';
      warn = !ids.length;
    } else if (!chosen.length) {
      msg = 'Choose at least one person; an empty selection is flagged.'; warn = true;
    } else {
      var forced = chosen.filter(function (x) { return x === v || VS.viewer.reach[x]; }), pend = chosen.filter(function (x) { return forced.indexOf(x) < 0; });
      msg = (forced.length ? 'Forced (live now): ' + forced.map(function (i) { return i === v ? 'yourself' : nm(i); }).join(', ') + '. ' : '') +
        (pend.length ? 'Pending until accepted: ' + pend.map(nm).join(', ') + ' (not below you).' : '');
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
  function tick(name, minutes) {
    ui.clock[name] = new Date(Date.parse(ui.clock[name]) + minutes * 60000).toISOString().slice(0, 19) + 'Z';
  }

  function applyRaw() {
    var st = stores[ui.store], ta = $('raw-roles');
    if (!ta) return true;
    var raw = { roles: null, remote: {}, clones: null };
    try { raw.roles = JSON.parse(ta.value); } catch (e) { toast('JSON error in roles file: ' + esc(e.message)); return false; }
    var tas = document.querySelectorAll('textarea[data-owner]');
    for (var i = 0; i < tas.length; i++) {
      var o = tas[i].getAttribute('data-owner'), a;
      try { a = JSON.parse(tas[i].value); } catch (e2) { toast('JSON error in ' + esc(o) + ': ' + esc(e2.message)); return false; }
      if (!Array.isArray(a)) { toast(esc(o) + ' must be a JSON list'); return false; }
      raw.remote[o] = a;
    }
    try { raw.clones = JSON.parse($('raw-clones').value); } catch (e3) { toast('JSON error in clone state: ' + esc(e3.message)); return false; }
    try { st.setRaw(raw); } catch (e4) { toast(esc(e4.message)); return false; }
    return true;
  }

  // One write: events go to the viewer clone's journal (open batch). Events are not
  // pre-validated, so a form can deliberately produce a flagged event.
  function commit(events) {
    var name = ui.store, s = viewerOf(name), ts = ui.clock[name];
    var res = stores[name].dispatch(s, events.map(function (e) { return { type: e[0], payload: e[1], origin: e[2] || 'human' }; }), ts);
    tick(name, 1);
    ui.last = res;
    closeDialog();
    render();
    toast(res.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ': ' + (n.status === 'applied' ? 'applied' : '<span class="tf">' + esc(n.status.toUpperCase()) + '</span> ' + esc(n.reason));
    }).join('<br>') + '<br><span class="muted">in the open batch; pushed at Sync, panel collapse, or viewer switch</span>');
    return res;
  }
  function nextId() { return stores[ui.store].nextEventId(viewerOf(ui.store)); }

  function submit(form) {
    if (!applyRaw()) return;
    var kind = form.getAttribute('data-kind'), item = form.getAttribute('data-fitem'), el = form.elements, v = memberOf(viewerOf(ui.store));
    function val(n) { return el[n] ? el[n].value : ''; }
    function people() { return Array.prototype.map.call(form.querySelectorAll('input[name="people"]:checked'), function (e) { return e.value; }); }
    var evs = [], open = null;
    switch (kind) {
      case 'create': {
        var pl = { kind: val('kind'), title: val('title') };
        if (val('body')) pl.body = val('body');
        if (val('project')) pl.project = val('project');
        if (pl.kind !== 'topic') { if (val('due')) pl.due = val('due'); pl.priority = Number(val('priority')); }
        open = nextId();
        evs.push(['item.create', pl]);
        if (val('assignee') && pl.kind !== 'topic') evs.push(['item.assign', { item: open, mode: 'chosen', people: [val('assignee')], completion: val('completion') }]);
        break;
      }
      case 'edit': {
        var it = VS.items[item], ep = { item: item };
        ['title', 'body', 'project'].forEach(function (f) { if (val(f) !== it[f]) ep[f] = val(f); });
        if ('body' in ep) {
          var km = keptMentions(it.mentions, ep.body);
          if (km.length) ep.mentions = km;
          if (it.attachments.length) ep.attachments = it.attachments;
        }
        if ((val('due') || null) !== it.due) ep.due = val('due') || null;
        if (Number(val('priority')) !== it.priority) ep.priority = Number(val('priority'));
        evs.push(['item.edit', ep]);
        break;
      }
      case 'assign': {
        var ap = { item: item, mode: val('mode'), completion: val('completion') };
        if (ap.mode === 'chosen') ap.people = people(); else ap.levels = Number(val('levels'));
        evs.push(['item.assign', ap]);
        break;
      }
      case 'split': {
        open = nextId();
        var sp0 = { kind: 'task', title: val('title'), parent: item, priority: Number(val('priority')) };
        if (val('due')) sp0.due = val('due');
        evs.push(['item.create', sp0]);
        evs.push(['item.assign', { item: open, mode: 'chosen', people: people(), completion: val('completion') }]);
        break;
      }
      case 'handoff': {
        var hp = { item: item, to: val('to') };
        if (val('who') && val('who') !== viewerOf(ui.store).split('.')[0]) hp.assignee = val('who');
        if (val('note')) hp.note = val('note');
        evs.push(['item.handoff', hp]);
        break;
      }
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
        if (val('who') && val('op') !== 'item.decline') np.assignee = val('who');
        if (val('note')) np.note = val('note');
        evs.push([val('op'), np]);
        break;
      }
      case 'cedit': {
        var ocm = VS.comments[form.getAttribute('data-fcid')], cep = { comment: form.getAttribute('data-fcid'), body: val('body') };
        var ckm = keptMentions(ocm.mentions, cep.body);
        if (ckm.length) cep.mentions = ckm;
        if (ocm.attachments.length) cep.attachments = ocm.attachments;
        evs.push(['comment.edit', cep]);
        break;
      }
      case 'admin': {
        var t = val('rtype'), rp = {};
        if (t === 'roles.add_member') rp.member = { id: val('nid'), name: val('nname') || val('nid'), keys: val('key') ? [val('key')] : [], leads: val('lead') ? [val('lead')] : [] };
        else if (t === 'roles.set_priority_levels') { rp.names = val('pnames').split(',').map(function (x) { return x.trim(); }).filter(Boolean); rp.default = Number(val('pdefault')); }
        else if (t === 'roles.set_attachment_limit') rp.max_bytes = Number(val('maxbytes'));
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
      case 'accept': commit([['item.accept', { item: item }]]); break;
      case 'ihide': commit([['item.hide', { item: item }]]); break;
      case 'chide': commit([['comment.hide', { comment: cid }]]); break;
      case 'comment': {
        if (!box || !box.value.trim()) return toast('Write a comment first.');
        var cp = { item: item, body: box.value.trim() };
        if (ui.replyTo) cp.reply_to = ui.replyTo;
        var ms = keptMentions(ui.mdraft, cp.body);
        if (ms.length) cp.mentions = ms;
        if (ui.adraft.length) cp.attachments = ui.adraft;
        ui.replyTo = null; ui.mdraft = []; ui.adraft = [];
        commit([['comment.add', cp, origin]]);
        break;
      }
    }
  }

  // Kanban drop: only the viewer's own live assignment moves.
  function dropOn(item, col) {
    var v = memberOf(viewerOf(ui.store)), r = VS.items[item] && VS.items[item].assignments.map(A).filter(function (x) { return x.to === v && x.state === 'live'; })[0];
    if (!r) return toast('Only your own assignment moves on the board.');
    if (col === r.status) return;
    if (col === 'todo' || col === 'doing' || col === 'blocked') return commit([['item.status', { item: item, status: col }]]);
    if (col === 'done') return commit([['item.close', { item: item }]]);
    toast('"' + esc(ST_LABEL[col] || col) + '" is not a status you set (todo, doing, blocked; done closes).');
  }

  // --------------------------------------------------------------- sync actions

  function syncNow(name, quiet) {
    var r = stores[name].sync(viewerOf(name), ui.clock[name]);
    if (!quiet) {
      if (r.noRemote) toast('No remote: the open batch is closed into a local commit; writes stay local, exactly like a long offline.');
      else if (r.offline) toast('Offline: the open batch is closed into a local commit; the sync is retried when you come back online (not an error).');
      else toast('Synced: pushed ' + r.pushed + ', fetched ' + r.fetched + ' commit' + (r.fetched === 1 ? '' : 's') + (r.fetched ? '. Late arrivals are checked against what their authors had seen; no earlier verdict changes.' : '.'));
    }
    return r;
  }
  function fetchNow(name) { return stores[name].fetch(viewerOf(name), ui.clock[name]); }
  // Switching viewer = the previous clone collapses its panel (push, 241) and the
  // new one opens the drawer (immediate check: fetch). Offline clones do neither.
  function switchViewer(stream) {
    var name = ui.store, prev = viewerOf(name);
    if (prev && prev !== stream) stores[name].sync(prev, ui.clock[name]);
    ui.viewers[name] = stream; ui.last = null;
    if (ui.open) fetchNow(name);
    vss[name] = stores[name].view(stream, ui.clock[name]);
    if (ui.open) markSeen(name);
    render();
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
      var b = $('handle-' + name), n = newKeys(name).length, badge = b.querySelector('.dr-badge'), vs = vss[name];
      b.className = 'dr-handle' + (ui.store === name ? ' on' : '') + (ui.open === name ? ' open' : '');
      b.setAttribute('aria-expanded', String(ui.open === name));
      badge.hidden = !n; badge.textContent = n;
      b.querySelector('.hoff').hidden = !!(vs.store.remote && vs.sync.online);
      b.title = vs.store.label + (n ? ': ' + n + ' new (assignments, pending assignments, notices)' : '') + (vs.store.remote ? (vs.sync.online ? '' : ' - offline') : ' - no remote');
    });
  }

  function render() {
    order.forEach(function (n) { vss[n] = stores[n].view(ui.viewers[n], ui.clock[n]); });
    var name = ui.store, s = viewerOf(name), v = memberOf(s);
    VS = vss[name]; EV = {};
    VS.events.forEach(function (e) { EV[e.id] = e; });
    if (ui.item && !VS.items[ui.item]) { ui.item = null; ui.sheet = false; }

    $('viewer').innerHTML = Object.keys(VS.members).map(function (k) {
      var m = VS.members[k];
      return '<option value="' + esc(k) + '"' + (k === v ? ' selected' : '') + '>' + esc(m.name + ' (' + k + ', ' + (m.removed ? 'removed' : m.role) + ')') + '</option>';
    }).join('');
    $('vclone').innerHTML = (VS.clones[v] || []).map(function (c) {
      return '<option value="' + esc(c.stream) + '"' + (c.stream === s ? ' selected' : '') + '>' + esc(E.cloneOf(c.stream)) + (c.online ? '' : ' (offline)') + (c.unpushed ? ' *' : '') + '</option>';
    }).join('');
    $('vstore').textContent = name;
    $('vclock').textContent = fmtTs(ui.clock[name]);
    renderHandles();
    $('drawer').classList.toggle('open', !!ui.open);

    var body = $('drbody'), sb = $('sheet').firstElementChild, top1 = body.scrollTop, top2 = sb.scrollTop;
    var draft = $('cbox') ? $('cbox').value : '';
    body.innerHTML = drawerHtml(name, v);
    body.scrollTop = top1;
    var chat = body.querySelector('.chat-log');
    if (chat) chat.scrollTop = chat.scrollHeight; // newest message at the bottom
    var open = ui.sheet && ui.item;
    $('sheet').classList.toggle('open', !!open);
    $('sheet').setAttribute('aria-hidden', String(!open));
    if (open) {
      sb.innerHTML = sheetHtml(v, VS.items[ui.item]);
      if (!ui.itemChanged && !ui.clearDraft && $('cbox')) $('cbox').value = draft;
      sb.scrollTop = ui.itemChanged ? 0 : top2;
    }
    ui.itemChanged = false; ui.clearDraft = false;
    measure();
  }

  function resetStore(name) {
    var d = defs[name];
    stores[name].reset();
    ui.clock[name] = d.now || new Date().toISOString().slice(0, 19) + 'Z';
  }

  function openStore(name) {
    if (ui.open === name) { ui.open = null; syncNow(name, true); render(); return; } // panel collapse pushes (241)
    if (ui.store !== name) { ui.item = null; ui.sheet = false; ui.last = null; ui.itemChanged = true; $('drbody').scrollTop = 0; }
    ui.store = name; ui.open = name;
    fetchNow(name); // immediate check when the drawer opens
    vss[name] = stores[name].view(viewerOf(name), ui.clock[name]);
    markSeen(name);
    render();
  }
  function showItem(id) { ui.item = id; ui.sheet = true; ui.itemChanged = true; ui.replyTo = null; ui.mdraft = []; ui.adraft = []; render(); }

  var QUICK = { 'q-status': 1, 'q-close': 1, confirm: 1, unassign: 1, accept: 1, ihide: 1, chide: 1, comment: 1 };

  function init() {
    window.TRACKER_FIXTURES.stores.forEach(function (d) {
      defs[d.name] = d;
      stores[d.name] = E.openStore(d); order.push(d.name); ui.viewers[d.name] = d.defaultViewer; resetStore(d.name);
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
        else if (k === 'cmode') { (ui.cmodes[ui.store] = ui.cmodes[ui.store] || {})[a.getAttribute('data-card')] = a.getAttribute('data-mode'); render(); }
        else if (k === 'cal') { ui.calMonth[a.getAttribute('data-card')] = a.getAttribute('data-month'); render(); }
        else if (k === 'clock') { tick(ui.store, Number(a.getAttribute('data-min'))); render(); }
        else if (k === 'sync') { syncNow(ui.store); render(); }
        else if (k === 'go-offline') { stores[ui.store].setOnline(viewerOf(ui.store), false); render(); toast('This clone is offline: new writes stay in its journal, and it fetches nothing.'); }
        else if (k === 'go-online') { stores[ui.store].setOnline(viewerOf(ui.store), true); syncNow(ui.store); render(); }
        else if (k === 'reply') { ui.replyTo = d.cid; render(); if ($('cbox')) $('cbox').focus(); }
        else if (k === 'reply-x') { ui.replyTo = null; render(); }
        else if (k === 'nread-all') { var rm = readMap(); feedEntries(memberOf(viewerOf(ui.store))).forEach(function (x) { rm[x.key] = true; }); render(); }
        else if (k === 'att-load') { var lm = (ui.loaded[seenKey(ui.store)] = ui.loaded[seenKey(ui.store)] || {}), oid = a.getAttribute('data-oid'); lm[oid] = !lm[oid]; render(); }
        else if (k === 'att-add') {
          var an = $('catt-name').value.trim(), asz = $('catt-size').value;
          if (!an || asz === '') return toast('Attachment needs a file name and a size in bytes.');
          ui.adraft.push({ oid: mockOid(an + '|' + asz), name: an, size: Number(asz), mime: mimeOf(an) }); render();
        }
        else if (k === 'att-x') { ui.adraft.splice(Number(a.getAttribute('data-idx')), 1); render(); }
        else if (QUICK[k]) { if (k === 'comment' || a.getAttribute('data-note')) ui.clearDraft = true; quick(k, a); }
        else openDialog(k, d);
        return;
      }
      var nk = t.closest('[data-nkey]');
      if (nk) readMap()[nk.getAttribute('data-nkey')] = true; // a feed entry is read once clicked
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
      if (t.id === 'viewer') switchViewer(stores[ui.store].clonesOf(t.value)[0]);
      else if (t.id === 'vclone') switchViewer(t.value);
      else if (t.id === 'f-kind') { ui.kind = t.value; render(); }
      else if (t.id === 'f-status') { ui.status = t.value; render(); }
      else if (t.id === 'a-family') { ui.act.family = t.value; render(); }
      else if (t.id === 'a-member') { ui.act.member = t.value; render(); }
      else if (t.id === 'flagged-only') { ui.act.flagged = t.checked; render(); }
      else if (t.id === 'conc-only') { ui.act.conc = t.checked; render(); }
      else if (t.id === 'cmention') {
        var box = $('cbox'), id = t.value;
        if (!id || !box) return;
        box.value = box.value + (box.value && !/\s$/.test(box.value) ? ' ' : '') + '@' + id + ' ';
        if (ui.mdraft.indexOf(id) < 0) ui.mdraft.push(id);
        render();
      }
      else if (t.closest && t.closest('#dlg form')) updateHint(t.closest('form'));
    });
    document.addEventListener('dragstart', function (e) {
      var c = e.target.closest && e.target.closest('[data-drag]');
      if (!c) return;
      e.dataTransfer.setData('text/plain', c.getAttribute('data-drag'));
      e.dataTransfer.effectAllowed = 'move';
    });
    document.addEventListener('dragover', function (e) { if (e.target.closest && e.target.closest('[data-kcol]')) e.preventDefault(); });
    document.addEventListener('drop', function (e) {
      var col = e.target.closest && e.target.closest('[data-kcol]');
      if (!col) return;
      e.preventDefault();
      var item = e.dataTransfer.getData('text/plain');
      if (item) dropOn(item, col.getAttribute('data-kcol'));
    });
    document.addEventListener('toggle', function (e) {
      var t = e.target;
      if (!t.tagName || t.tagName !== 'DETAILS') return;
      if (t.id === 'data-details') ui.dataOpen = t.open;
      else if (t.getAttribute('data-owner')) ui.openLogs[t.getAttribute('data-owner')] = t.open;
    }, true);
    document.addEventListener('keydown', function (e) {
      if (e.key !== 'Escape' || $('dlg').open) return;
      if (ui.sheet) { ui.sheet = false; render(); } else if (ui.open) { var n = ui.open; ui.open = null; syncNow(n, true); render(); }
    });
    window.addEventListener('resize', measure);
    render();
  }

  window.addEventListener('error', function (e) { var t = $('toast'); if (t) { t.hidden = false; t.textContent = 'script error: ' + e.message; } });
  init();
  // read-only hook for scripted checks (headless verification); not part of the model
  window.__tracker = { ui: ui, vs: function () { return VS; } };
})();
