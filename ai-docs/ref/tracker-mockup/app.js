// Tracker mockup v3: the view. A dashboard-like frame (header, conversation
// placeholder, pull-down drawer with one handle per store) around the tracker
// views, an item window overlay, and small action dialogs. The view reads only the
// plain view-state object a store handle returns (store.view) and writes only by
// dispatching events (store.dispatch); it never touches engine internals.
(function () {
  'use strict';

  var E = window.TrackerEngine, alias = E.alias, memberOf = E.memberOf;
  var PROFILE_FIELDS = ['org', 'family', 'given'];

  var stores = {}, defs = {}, order = [], vss = {}, EV = {};
  var ui = {
    store: null,          // the store the header control and the item window refer to
    open: null,           // the store whose drawer is open, or null
    page: null,           // the store shown full-page by the route #/<storeId> (371), or null
    viewers: {},          // store -> clone stream the page is viewed as ("<member>.<clone>")
    clock: {},            // store -> mock "now"; each write advances it by a minute
    seen: {},             // "store|stream" -> { badge key: true } already shown to that viewer
    fresh: {},            // badge keys that were new when the drawer last opened
    read: {},             // "store|stream" -> { notification entry key: true } read by that viewer (local UI state, (334), (340))
    loaded: {},           // "store|stream" -> { attachment oid: true } "loaded" by that viewer's clone (simulated, local UI state)
    mdraft: [], adraft: [], // composer drafts: member ids picked with @, attachment metadata to post
    item: null, win: false, last: null, replyTo: null,
    winReturn: null,      // selector of the element that opened the item window; focus returns there on close
    drh: loadDrh(),       // store -> drawer height in px, set by dragging its handle (382), local UI state; absent: follows the content
    tab: {},              // store -> { side: tab key of the tab pane, narrow: tab key of the narrow layout } (366), local UI state
    tabSeen: {},          // "store|stream" -> { tab key -> signature last shown }: a hidden tab whose signature moved shows a dot
    layout: null,         // 'a' | 'd' | 'narrow', from the window width (366)
    cmodes: {}, calMonth: {}, // store -> { card key -> view mode }; card or scope key -> calendar month (local UI state, never an event)
    panel: {},            // store -> { scope, modes: { scope -> view mode }, today: Today chip filter or null, q: search text, closedOpen: "recently closed" expanded, treeOpen: { parent id -> its "closed N" row expanded } } of the Items panel (360), (366), (385), (386), (387), local UI state like cmodes
    focus: null,          // { ev: event id, flash: true until its first paint } the item window scrolls to and marks (390); null: nothing
    sel: null,            // the row selected by a tap (354): it shows the same actions and details a hover shows
    act: { family: 'all', member: 'all', flagged: false, conc: false },
    kind: 'all', status: 'open', dataOpen: false, openLogs: {}
  };
  // (357) The Items panel: scope presets crossed with the view modes each scope offers.
  // (303) A side card offers only the modes that fit it; a card missing from CARD_MODES has one layout.
  var ASSIGN_MODES = [['list', 'list'], ['kanban', 'kanban'], ['gantt', 'due timeline'], ['calendar', 'calendar']];
  var SCOPES = [['mine', 'mine', 'my live assignments, then done, then what I created and did not assign'],
    ['received', 'received requests', 'pending assignments to me (someone outside my lead chain, or a handoff), the ones I answered, what I sent, and answers to it'],
    ['given', 'assigned by me', 'what I assigned or handed to others, live or pending'],
    ['team', 'my team', 'what each follower (down to 3 levels) works on, whoever assigned it, grouped by person'],
    ['all', 'all', 'every item as a tree, with kind and status filters']];
  var SCOPE_MODES = { mine: ASSIGN_MODES, received: [['list', 'list']], given: ASSIGN_MODES, team: ASSIGN_MODES,
    all: [['list', 'tree'], ['kanban', 'kanban'], ['gantt', 'due timeline'], ['calendar', 'calendar']] };
  var CARD_MODES = { activity: [['list', 'list'], ['activity', 'activity timeline']] };
  var SI = window.TrackerStatusIcons;

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
  // (391) The org is not part of the display name: it shows as a muted prefix where a
  // person is named in markup (whoName), and as a plain prefix in tooltips and option
  // texts (nmo).
  function orgOf(id) { var m = VS.members[id]; return m && m.profile ? m.profile.org : ''; }
  function nmo(id) { var o = orgOf(id); return (o ? o + ' ' : '') + nm(id); }
  function whoName(id) { var o = orgOf(id); return (o ? '<span class="org">' + esc(o) + '</span> ' : '') + esc(nm(id)); }
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
  // (366) A side card is a spec: shown as one tab of the tab pane, or (Notifications on a
  // wide screen) as a standalone card. cardHtml draws the standalone form.
  function card(title, chip, body, extra, id, mode, cls) {
    return { title: title, chip: chip, body: body, extra: extra || '', id: id, mode: mode || '', cls: cls || '' };
  }
  function cardHtml(c) {
    return '<section class="card' + (c.cls ? ' ' + c.cls : '') + '"' + (c.id ? ' id="' + c.id + '"' : '') + (c.mode ? attr('view', c.mode) : '') + '><header><h2>' + c.title + '</h2>' + (c.chip ? '<span class="chip">' + c.chip + '</span>' : '') +
      '<span class="grow"></span>' + c.extra + '</header><div class="cbody">' + c.body + '</div></section>';
  }
  // (305) the chosen mode is remembered per store, so it survives a viewer switch within the store
  function modeOf(key) { var m = (ui.cmodes[ui.store] || {})[key]; return CARD_MODES[key] && m ? m : 'list'; }
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
      case 'member.profile': {
        var pr = p.profile || {};
        return 'changed ' + (p.member && p.member !== n.member ? 'the profile of <b>' + esc(p.member) + '</b> (admin)' : 'their own profile') + ': ' +
          PROFILE_FIELDS.filter(function (f) { return f in pr; }).map(function (f) { return f + ' &rarr; "' + esc(pr[f]) + '"'; }).join(', ');
      }
      case 'roles.add_member': return 'added member <b>' + esc((p.member || {}).id) + '</b> (' + esc(E.displayName(E.normProfile(p.member, (p.member || {}).id))) + ') under ' + esc(((p.member || {}).leads || []).join(', ') || 'nobody');
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

  // ---------------------------------------------------------------- item rows
  //
  // (356) An item row shows by default only what must be seen: status icon, title,
  // assignee initials, priority, due, a warning icon for flagged or concurrent writes,
  // and an unread dot. Everything else (kind, source clone and event id, dates, roll-up,
  // assignment path, notes) is the row's detail overlay, and the actions are icons; both
  // appear on hover. (354) Keyboard focus and a tap-selection (ui.sel) show the same:
  // one CSS state, `.irow:is(:hover, :focus-within, .sel)`. (:focus-within, because
  // Chromium does not re-match :has(:focus-visible) in time when Tab moves from the row
  // into its buttons, so they hide mid-move and focus falls to the body.)
  // The overlay ignores the pointer, so moving down a list hovers the next row.

  function stKey(it, r) { return r ? (r.state === 'live' ? r.status : r.state) : it.kind === 'topic' ? 'topic' : it.status; }
  function tipAttr(text) { return ' data-tip="' + esc(text) + '"'; }
  // data-tip from already-escaped HTML: drop the tags, keep the entities (the attribute decodes them)
  function tipHtml(h) { return ' data-tip="' + String(h).replace(/<[^>]*>/g, '').replace(/"/g, '&quot;') + '"'; }
  function stDesc(s) { var x = SI.info(s); return x.label + ': ' + x.desc; }
  function stIcon(s) { return '<span class="sti"' + tipAttr(stDesc(s)) + '>' + SI.svg(s) + '</span>'; }
  // (383 b) The avatar text comes from the view state (the profile rules live in the engine).
  function initials(id) { var m = VS.members[id]; return m ? m.avatar : String(id).slice(0, 2).toUpperCase(); }
  function holders(it) { return it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }); }
  function whoHtml(it) {
    var as = holders(it), v = VS.viewer.member;
    if (!as.length) return '<span class="who"></span>';
    var shown = as.slice(0, 3), more = as.length - shown.length;
    var tip = 'assignees: ' + as.map(function (r) { return nmo(r.to) + (r.state === 'pending' ? ' (pending)' : ''); }).join(', ');
    return '<span class="who"' + tipAttr(tip) + '>' + shown.map(function (r) {
      return '<i class="av' + (r.state === 'pending' ? ' pend' : '') + (r.to === v ? ' me' : '') + '">' + esc(initials(r.to)) + '</i>';
    }).join('') + (more ? '<i class="av more">+' + more + '</i>' : '') + '</span>';
  }
  function priHtml(it) {
    if (!it || it.kind === 'topic') return '<span class="pri"></span>';
    var lv = it.priorityLevel, d = VS.priority.default;
    return '<span class="pri' + (lv > d ? ' hi' : lv < d ? ' lo' : '') + '"' + tipAttr('priority ' + lv + ' of ' + (VS.priority.names.length - 1) + ': ' + it.priorityName +
      (it.priority !== lv ? ' (stored ' + it.priority + ', outside the current range: shown as the top level)' : '')) + '>' + esc(it.priorityName) + '</span>';
  }
  // (356) Due: D-N within 7 days, D-day today, D+N past due (danger colour), else a short date.
  var ENDED = { done: 1, declined: 1, returned: 1, handed: 1, withdrawn: 1 };
  function dueHtml(it, st) {
    if (!it || !it.due || it.kind === 'topic') return '<span class="due2"></span>';
    var n = dayNum(it.due) - dayNum(today()), t, c, tip;
    if (ENDED[st]) { t = it.due.slice(5); c = 'ended'; tip = 'due ' + it.due + ' (' + SI.info(st).label + ')'; }
    else if (n < 0) { t = 'D+' + -n; c = 'over'; tip = 'due ' + it.due + ', overdue by ' + -n + (n === -1 ? ' day' : ' days'); }
    else if (n === 0) { t = 'D-day'; c = 'now'; tip = 'due today (' + it.due + ')'; }
    else if (n <= 7) { t = 'D-' + n; c = 'soon'; tip = 'due ' + it.due + ', in ' + n + (n === 1 ? ' day' : ' days'); }
    else { t = it.due.slice(5); c = ''; tip = 'due ' + it.due; }
    return '<span class="due2' + (c ? ' ' + c : '') + '"' + tipAttr(tip) + '>' + esc(t) + '</span>';
  }
  var FLAGGED = {}, UNREAD = {}; // per render: item -> flagged writes in its own thread; item -> unread notification entries
  function warnHtml(it) {
    var f = FLAGGED[it.id] || 0, c = it.concurrent;
    if (!f && !c) return '';
    var tip = [f ? f + ' flagged write' + (f === 1 ? '' : 's') : '', c ? c + ' concurrent change' + (c === 1 ? '' : 's') : ''].filter(Boolean).join(', ') + '; see the thread';
    return '<span class="wicon' + (f ? ' flag' : '') + '"' + tipAttr(tip) + '><svg class="i"><use href="#i-warn"/></svg></span>';
  }
  function dotHtml(it) {
    var n = UNREAD[it.id];
    return n ? '<span class="udot"' + tipAttr(n + ' unread notification' + (n === 1 ? '' : 's') + ' (Notifications)') + '></span>' : '';
  }

  // Row actions: every action the viewer may take on the item, from the view state, as
  // descriptors [act, icon, label, data, go]. A row draws them as icon buttons; the item
  // window draws the same list as labelled buttons (366). Each dispatches through the
  // same data-act handlers.
  function dataAttrs(data) {
    var d = '';
    Object.keys(data || {}).forEach(function (k) { d += attr(k, data[k]); });
    return d;
  }
  function ia(act, icon, label, data, cls) {
    return '<button type="button" class="ia' + (cls ? ' ' + cls : '') + '"' + attr('act', act) + dataAttrs(data) + ' aria-label="' + esc(label) + '"' + tipAttr(label) + '><svg class="i"><use href="#i-' + icon + '"/></svg></button>';
  }
  function actList(it) {
    var out = [];
    if (!it || it.hidden) return out;
    var v = VS.viewer.member, recs = it.assignments.map(A), id = it.id;
    var mineR = recs.filter(function (r) { return r.to === v && r.state === 'live'; })[0];
    if (recs.some(function (r) { return r.to === v && r.state === 'pending'; }))
      out.push(['accept', 'accept', 'Accept', { item: id }, 1], ['note', 'decline', 'Decline...', { item: id, op: 'item.decline' }], ['open', 'chat', 'Discuss (open the item)', { item: id }]);
    if (mineR && mineR.status !== 'done') {
      out.push(['status', 'status', 'Status...', { item: id }]);
      if (mineR.status !== 'awaiting') out.push(['close', 'close', 'Close...', { item: id }]);
      out.push(['split', 'split', 'Split into a subtask...', { item: id }], ['handoff', 'handoff', 'Hand off...', { item: id }]);
    }
    recs.forEach(function (r) {
      var lead = VS.members[r.to] && VS.members[r.to].leads[0];
      if (r.state === 'live' && r.by === v && r.status === 'awaiting')
        out.push(['confirm', 'confirm', 'Confirm ' + nm(r.to) + '\'s close', { item: id, who: r.to }, 1], ['note', 'reject', 'Reject ' + nm(r.to) + '\'s close...', { item: id, who: r.to, op: 'item.reject' }]);
      if (r.state === 'live' && r.accepted && lead === v) out.push(['note', 'return', 'Return ' + nm(r.to) + '\'s accepted assignment...', { item: id, who: r.to, op: 'item.return' }]);
      if (r.state === 'pending' && (r.by === v || r.sender === v) && r.to !== v) out.push(['unassign', 'withdraw', 'Withdraw what you sent to ' + nm(r.to), { item: id, who: r.to }]);
    });
    if (it.author === v && it.kind !== 'topic' && !holders(it).length) out.push(['assign', 'assign', 'Assign...', { item: id }]);
    return out;
  }
  function actsFor(it) { return actList(it).map(function (x) { return ia(x[0], x[1], x[2], x[3], x[4] ? 'go' : ''); }).join(''); }

  // The detail overlay: what the row leaves out by default.
  function detailsHtml(it, r, extra) {
    var d = [], st = stKey(it, r), ist = stKey(it);
    d.push(kindChip(it) + ' <span class="al">' + esc(it.alias) + '</span> by ' + esc(nm(it.author)) + (it.project ? ' &middot; ' + esc(it.project) : ''));
    d.push('<b>' + esc(SI.info(st).label) + '</b> <span class="muted">' + esc(SI.info(st).desc) + '</span>' +
      (r && r.state === 'live' && r.statusOrigin && r.statusOrigin !== 'human' ? ' &middot; set by ' + esc(nm(r.statusBy)) + '\'s agent (' + esc(r.statusOrigin) + ')' : '') +
      (r && ist !== st && it.kind !== 'topic' ? ' &middot; item ' + esc(SI.info(ist).label) : '') +
      (r && r.stale ? ' &middot; <span class="err">stale</span>: no status update for ' + E.STALE_DAYS + '+ days' : ''));
    var roll = rollupHtml(it);
    if (roll) d.push(roll);
    if (r) d.push(source(r) + ' &middot; ' + complText(r.completion) + (it.chain.length > 1 ? ' &middot; chain ' + esc(it.chain.join(' > ')) : ''));
    else if (it.kind !== 'topic') d.push(holders(it).length ? '&rarr; ' + holders(it).map(function (x) { return esc(nm(x.to)) + ' <span class="muted">' + esc(SI.info(stKey(it, x)).label) + '</span>'; }).join(', ') : '<span class="muted">not assigned</span>');
    var note = r && (r.note ? [r.note, r.noteBy] : r.handoffNote ? [r.handoffNote, r.sender] : r.answerNote ? [r.answerNote, r.to] : null);
    if (note) d.push('&ldquo;' + esc(note[0]) + '&rdquo; <span class="muted">(' + esc(note[1]) + ')</span>');
    if (extra) d.push(extra);
    var f = FLAGGED[it.id] || 0;
    d.push('<span class="muted">created ' + esc(fmtTs(it.ts)) + (it.edited ? ' &middot; edited ' + esc(fmtTs(it.edited.ts)) : '') + ' &middot; last activity ' + esc(fmtTs(it.lastActivity)) +
      (r ? ' &middot; assignment updated ' + esc(fmtTs(r.lastUpdate || r.ts)) : '') + (f ? ' &middot; ' + f + ' flagged' : '') + (it.concurrent ? ' &middot; ' + it.concurrent + ' concurrent' : '') + '</span>');
    return d.map(function (x) { return '<div>' + x + '</div>'; }).join('');
  }
  // o: { rk (unique row key in the drawer), item, rec?, more?, depth?, acts?, closed? (greyed: done, in the "recently closed" section or kept in the tree) }
  function irow(o) {
    var it = VS.items[o.item], r = o.rec, st = stKey(it, r), acts = o.acts != null ? o.acts : actsFor(it);
    return '<div class="irow' + (ui.sel === o.rk ? ' sel' : '') + (o.closed ? ' closed' : '') + '" tabindex="0"' + attr('item', it.id) + attr('rk', o.rk) + (r ? attr('rec', r.id) : '') +
      (o.depth ? ' style="--ind:' + o.depth + '"' : '') + '><div class="ir-main">' + (o.depth ? '<span class="ind">&#8627;</span>' : '') + stIcon(st) +
      '<a class="title" href="#" tabindex="-1"' + attr('item', it.id) + '>' + esc(it.title) + '</a>' + warnHtml(it) + dotHtml(it) + '<span class="grow"></span>' +
      (acts ? '<span class="ir-acts">' + acts + '</span>' : '') + whoHtml(it) + priHtml(it) + dueHtml(it, st) + '</div>' +
      '<div class="ir-more">' + detailsHtml(it, r, o.more) + '</div></div>';
  }
  function sub(label, n) { return '<div class="sub">' + label + (n != null ? ' <span class="subn">' + n + '</span>' : '') + '</div>'; }

  // ------------------------------------------------------------------ cards

  // (331)-(334) The notification feed: one tab (366), list only, newest first. Every entry
  // is derived from the view state; an entry only opens the item window, and the
  // actions live on the item's row in the Items panel (357). Read state is local
  // UI state per viewer clone, like the handle badge's `seen`; no event is written
  // (in the product the daemon keeps it per clone on the local filesystem, (340)).
  var NK = { assigned: 'assigned', pending: 'pending', answer: 'answer', reference: 'for reference', done: 'done', blocked: 'blocked',
    confirm: 'confirm', reply: 'reply', due: 'due', mention: 'mention' };
  function snippet(s) { s = String(s || ''); return '&ldquo;' + esc(s.length > 80 ? s.slice(0, 77) + '...' : s) + '&rdquo;'; }
  // (390) Every entry also carries `ev`, the event it is about (none for a due date, which has
  // none), so a click can open the item window at what the entry refers to (focusTarget).
  var STATUS_EV = { 'item.status': 1, 'item.close': 1, 'item.confirm': 1, 'item.reject': 1, 'item.reopen': 1 };
  function assigneeOf(e) { return e.type === 'item.status' || e.type === 'item.close' ? e.member : e.payload.assignee || e.member; }
  // the assignment record's latest status-side event: the one written at its lastUpdate, else the last one
  function statusEv(r) {
    var l = VS.events.filter(function (e) { return STATUS_EV[e.type] && e.status === 'applied' && e.payload.item === r.item && assigneeOf(e) === r.to; });
    var at = l.filter(function (e) { return e.ts === r.lastUpdate; });
    return (at.length ? at : l).slice(-1).map(function (e) { return e.id; })[0] || null;
  }
  function feedEntries(v) {
    var out = [], t = today();
    function add(key, kind, item, ts, html, ev) {
      if (VS.items[item] && !VS.items[item].hidden) out.push({ key: key, kind: kind, item: item, ts: String(ts || ''), html: html, ev: ev || null });
    }
    VS.my.assignments.map(A).forEach(function (r) {
      var it = VS.items[r.item];
      // new forced assignments: the same rule as the handle badge's "a:" keys
      if (r.by !== v && r.forced && r.status === 'todo') add('a:' + r.id, 'assigned', r.item, r.ts, '<b>' + esc(nm(r.by)) + '</b> assigned this to you' + (r.inherited ? ' (everyone within L' + esc(r.ruleLevels) + ')' : ''), r.via);
      // near the due date, as the due chip computes it
      if (it.due && it.due <= t && r.status !== 'done' && it.status !== 'done')
        add('d:' + r.id + ':' + (it.due < t ? 'over' : 'today'), 'due', r.item, it.due + 'T00:00:00Z', it.due < t ? 'overdue since ' + esc(it.due) : 'due today');
    });
    VS.my.received.pending.map(A).forEach(function (r) {
      add('p:' + r.id, 'pending', r.item, r.ts, '<b>' + esc(nm(r.sender)) + '</b> ' + (r.handoffOf ? 'hands you their assignment' : 'sent you a pending assignment') + '; answer it on its row (Items, received requests)', r.via);
    });
    VS.notices.forEach(function (x) {
      var r = A(x.assignment), txt;
      if (x.kind === 'declined') txt = '<b>' + esc(nm(x.to)) + '</b> declined what you sent';
      else if (x.kind === 'returned') txt = '<b>' + esc(nm(x.endedBy)) + '</b> (lead of ' + esc(nm(x.to)) + ') returned ' + esc(nm(x.to)) + '\'s accepted ' + (r.handoffOf ? 'handoff' : 'assignment');
      else if (x.kind === 'accepted') txt = '<b>' + esc(nm(x.to)) + '</b> accepted ' + (r.handoffOf ? 'a handoff' : 'an assignment from ' + esc(nm(x.by))) + ' (you are their lead)';
      else txt = '<b>' + esc(nm(x.to)) + '</b> took over ' + esc(nm(x.from)) + '\'s assignment you gave';
      add(x.key, x.kind === 'declined' || x.kind === 'returned' ? 'answer' : 'reference', x.item, x.ts, txt + (x.note ? ': ' + snippet(x.note) : ''), (x.kind === 'declined' || x.kind === 'returned' ? r.ended : r.accepted).id);
    });
    vals(VS.assignments).forEach(function (r) {
      if (r.by !== v || r.to === v || r.state !== 'live' || (r.status !== 'done' && r.status !== 'blocked')) return;
      add('w:' + r.id + ':' + r.status + ':' + r.lastUpdate, r.status, r.item, r.lastUpdate,
        '<b>' + esc(nm(r.to)) + '</b> ' + (r.status === 'done' ? 'finished work you assigned' : 'is blocked on work you assigned') + (r.note ? ': ' + snippet(r.note) : ''), statusEv(r));
    });
    VS.my.today.confirms.map(A).forEach(function (r) {
      add('c:' + r.id + ':' + r.lastUpdate, 'confirm', r.item, r.lastUpdate, '<b>' + esc(nm(r.to)) + '</b> closed this; it waits for your confirm', statusEv(r));
    });
    vals(VS.comments).forEach(function (c) {
      if (c.hidden || c.author === v) return;
      var q = c.reply_to && VS.comments[c.reply_to];
      if (q && q.author === v) add('r:' + c.id, 'reply', c.item, c.ts, '<b>' + esc(nm(c.author)) + '</b> replied to your comment: ' + snippet(c.body), c.id);
      var ts = c.edited ? c.edited.ts : c.ts;
      if (c.mentions.indexOf(v) >= 0) add('m:' + c.id + ':' + ts, 'mention', c.item, ts, '<b>' + esc(nm(c.author)) + '</b> mentioned you: ' + snippet(c.body), c.id);
    });
    vals(VS.items).forEach(function (it) {
      var ts = it.edited ? it.edited.ts : it.ts;
      if (it.author !== v && it.mentions.indexOf(v) >= 0) add('m:' + it.id + ':' + ts, 'mention', it.id, ts, '<b>' + esc(nm(it.author)) + '</b> mentioned you in the item body', it.edited ? it.edited.id : it.id);
    });
    return out.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : a.key < b.key ? -1 : 1; });
  }
  function readMap() { return (ui.read[seenKey(ui.store)] = ui.read[seenKey(ui.store)] || {}); }
  function cardNotify(v) {
    var list = feedEntries(v), read = readMap(), unread = list.filter(function (x) { return !read[x.key]; }).length;
    var h = list.length ? list.map(function (x) {
      return '<div class="row click nfeed' + (read[x.key] ? '' : ' unread') + '"' + attr('item', x.item) + attr('nkey', x.key) + attr('nkind', x.kind) + (x.ev ? attr('nev', x.ev) : '') + '><div class="r1">' +
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

  // ------------------------------------------------------------- Items panel
  //
  // (357) One panel replaces My assignments, Received requests, Assigned by me, My team,
  // and Items: scope presets (SCOPES) crossed with the view modes each scope offers
  // (SCOPE_MODES). (360) It opens on "mine"; the chosen scope and each scope's mode are
  // remembered per store in memory, like the side cards' modes, and never written.

  function panelState() { return (ui.panel[ui.store] = ui.panel[ui.store] || { scope: 'mine', modes: {}, today: null, q: '', closedOpen: true, treeOpen: {} }); }
  function scopeList() { return SCOPES.filter(function (x) { return x[0] !== 'team' || VS.my.team.length; }); }
  function scopeOf() { var s = panelState().scope; return scopeList().some(function (x) { return x[0] === s; }) ? s : 'mine'; }
  function panelMode(s) { var m = panelState().modes[s]; return m && SCOPE_MODES[s].some(function (x) { return x[0] === m; }) ? m : 'list'; }

  // (366) Today as summary chips in the Items panel header (the Today card is gone):
  // my open work / waiting for my answer / waiting for my confirm, always visible
  // whatever scope is shown (358). A chip narrows the panel to its rows, as a filter on
  // the current scope and view mode; the same chip again clears it. When the current
  // scope shows none of them, the click moves the panel to the scope that holds them
  // (TODAY_HOME), so a chip never opens on an empty panel.
  var TODAY = [['work', 'my work', 'my open work: my live assignments not done, by priority then due'],
    ['pending', 'answer', 'waiting for my answer: pending assignments to me'],
    ['confirms', 'confirm', 'waiting for my confirm: closes of work I assigned']];
  var TODAY_HOME = { work: 'mine', pending: 'received', confirms: 'given' };
  var TF = null; // per render: the active Today filter { key, recs: {assignment id}, items: {item id} }, or null
  function todayFilter() {
    var k = panelState().today;
    if (!k || !VS.my.today[k]) return null;
    var f = { key: k, recs: {}, items: {} };
    VS.my.today[k].forEach(function (id) { f.recs[id] = true; f.items[A(id).item] = true; });
    return f;
  }
  // (385) The search box (row 2 of the panel header) filters the current scope as you type, on
  // the item's title, body, project, and id (the full id and the short alias), and combines
  // with the Today filter. SQ is the lower-cased query of the render in progress, '' for none.
  var SQ = '', FILT = false; // per render: the search query; whether the Today filter or a search is on
  function matchItem(it) {
    if (!SQ) return true;
    return [it.title, it.body, it.project, it.id, it.alias].some(function (x) { return String(x == null ? '' : x).toLowerCase().indexOf(SQ) >= 0; });
  }
  // a row with an assignment record passes on the record; an item row passes on the item
  function keepRec(r) { return (!TF || !!TF.recs[r.id]) && matchItem(VS.items[r.item]); }
  function keepItem(id) { return (!TF || !!TF.items[id]) && matchItem(VS.items[id]); }
  function todayBar() {
    var t = VS.my.today, on = panelState().today;
    return '<div class="todaybar" role="group" aria-label="Today"><span class="tl">Today</span>' + TODAY.map(function (x, i) {
      var n = t[x[0]].length;
      return (i ? '<span class="tsep">&middot;</span>' : '') + '<button type="button" class="tch' + (on === x[0] ? ' on' : '') + (n ? '' : ' zero') + '" aria-pressed="' + (on === x[0]) + '"' +
        attr('act', 'today') + attr('set', x[0]) + tipAttr(x[2] + ' (' + n + '); ' + (on === x[0] ? 'click to clear the filter' : 'click to show only these in the panel')) + '>' + esc(x[1]) + ' <b>' + n + '</b></button>';
    }).join('') + (on ? '<button type="button" class="tch tclr"' + attr('act', 'today') + attr('set', on) + ' aria-label="clear the Today filter"' + tipAttr('clear the Today filter') + '><svg class="i"><use href="#i-x"/></svg></button>' : '') + '</div>';
  }
  function filterEmpty() {
    var x = TF && TODAY.filter(function (y) { return y[0] === TF.key; })[0];
    return '<div class="empty">Nothing in this scope ' + (SQ ? 'matches &ldquo;' + esc(panelState().q.trim()) + '&rdquo;' : 'is in &ldquo;' + esc(x[2].split(':')[0]) + '&rdquo;') +
      (SQ && TF ? ' under the &ldquo;' + esc(x[2].split(':')[0]) + '&rdquo; filter' : '') + '. ' + (SQ ? btn('psearch-clear', 'clear the search', {}) + ' ' : '') + (TF ? btn('today', 'clear the filter', { set: TF.key }) : '') + '</div>';
  }

  // (386) Recently closed: done items leave the main list for a section at the bottom of the
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
    var t = it.assignments.map(A).filter(function (r) { return r.state === 'live' && r.status === 'done'; }).map(function (r) { return String(r.lastUpdate); }).sort().pop();
    return t || String(it.lastActivity);
  }
  function addClosed(ts, n, html) {
    if (!CLOSED) return;
    if (SQ || Date.parse(ui.clock[ui.store]) - Date.parse(ts) <= CLOSED_DAYS * 86400000) CLOSED.entries.push({ ts: ts, n: n, html: html });
    else CLOSED.older += n;
  }
  function closedHtml() {
    if (!CLOSED || (!CLOSED.entries.length && !CLOSED.older)) return '';
    var on = panelState().closedOpen, n = CLOSED.entries.reduce(function (a, x) { return a + x.n; }, 0);
    var rows = CLOSED.entries.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : 0; }).map(function (x) { return x.html; }).join('');
    return '<section class="pclosed' + (on ? ' open' : '') + '"><button type="button" class="pc-head" aria-expanded="' + on + '"' + attr('act', 'closed-toggle') + attr('fk', 'closed') + '><svg class="i"><use href="#i-chev"/></svg>' +
      (SQ ? 'closed' : 'recently closed') + ' <span class="subn">' + n + '</span><span class="muted">' + (SQ ? 'matches of every age' : 'last ' + CLOSED_DAYS + ' days' + (CLOSED.older ? '; ' + CLOSED.older + ' older through search' : '')) + '</span></button>' +
      (on ? '<div class="pc-rows">' + (rows || '<p class="muted">Nothing closed in this window.</p>') + '</div>' : '') + '</section>';
  }

  function listMine() {
    var mine = VS.my.assignments.map(A).filter(keepRec), open = mine.filter(function (r) { return r.status !== 'done'; }), done = mine.filter(function (r) { return r.status === 'done'; });
    var created = VS.my.created.filter(keepItem);
    done.forEach(function (r) { addClosed(String(r.lastUpdate), 1, irow({ rk: 'mine:' + r.id, item: r.item, rec: r, closed: true })); });
    if (FILT && !mine.length && !created.length) return filterEmpty();
    var h = open.length ? open.map(function (r) { return irow({ rk: 'mine:' + r.id, item: r.item, rec: r }); }).join('') : TF ? '' : SQ ? (created.length ? '' : '<p class="muted">No open item matches.</p>') : '<p class="muted">Nothing open is assigned to you.</p>';
    if (created.length) h += sub('created by me, not assigned', created.length) + created.map(function (id) { return irow({ rk: 'mine:c:' + id, item: id }); }).join('');
    return h;
  }
  function answerTxt(x) {
    var r = A(x.assignment);
    return (x.kind === 'declined' ? '<b>' + esc(nm(x.to)) + '</b> declined what you sent' :
      '<b>' + esc(nm(x.endedBy)) + '</b> (lead of ' + esc(nm(x.to)) + ') returned ' + esc(nm(x.to)) + '\'s accepted ' + (r.handoffOf ? 'handoff' : 'assignment') +
      (x.restores ? '; ' + esc(nm(x.from)) + '\'s assignment is restored' : r.handoffOf ? '; nobody holds it now' : '')) +
      (x.note ? ': &ldquo;' + esc(x.note) + '&rdquo;' : '') + ', ' + esc(fmtTs(x.ts));
  }
  function listReceived() {
    var R = VS.my.received, h = '';
    var ans = VS.notices.filter(function (x) { return (x.kind === 'declined' || x.kind === 'returned') && keepRec(A(x.assignment)); });
    function only(ids) { return ids.filter(function (id) { return keepRec(A(id)); }); }
    var pend = only(R.pending), answered = only(R.answered), sent = only(R.sent);
    if (FILT && !ans.length && !pend.length && !answered.length && !sent.length) return filterEmpty();
    function rows(ids, pre, more) { return ids.map(function (id) { var r = A(id); return irow({ rk: pre + id, item: r.item, rec: r, more: more && more(r) }); }).join(''); }
    if (ans.length) h += sub('answers to what I sent', ans.length) + ans.map(function (x) { return irow({ rk: 'rcv:a:' + x.key, item: x.item, rec: A(x.assignment), more: answerTxt(x) }); }).join('');
    if (pend.length || !FILT) h += sub('waiting for my answer', pend.length) + (pend.length ? rows(pend, 'rcv:p:') : '<p class="muted">No pending assignments to you.</p>');
    if (answered.length) h += sub('answered', answered.length) + rows(answered, 'rcv:d:');
    if (sent.length) h += sub('sent by me', sent.length) + rows(sent, 'rcv:s:', function (r) { return 'sent to <b>' + esc(nm(r.to)) + '</b>'; });
    return h;
  }
  function listGiven() {
    var v = VS.viewer.member, given = VS.my.given.filter(keepItem), h = '';
    if (!VS.my.given.length) return '<div class="empty">You have not assigned anything to others.</div>';
    if (!given.length) return filterEmpty();
    given.forEach(function (id) {
      var lines = VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); }).sort(byRank).map(function (r) {
        return '<b>' + esc(nm(r.to)) + '</b>' + (r.depth ? ' <span class="muted">L' + r.depth + '</span>' : '') + ' ' + esc(SI.info(stKey(null, r)).label) + (r.forced ? '' : ' <span class="muted">(' + (r.handoffOf ? 'handoff' : 'sent') + ')</span>') +
          (r.note ? ' &ldquo;' + esc(r.note) + '&rdquo;' : '');
      });
      if (VS.items[id].status === 'done') return addClosed(closeTs(VS.items[id]), 1, irow({ rk: 'given:' + id, item: id, more: lines.join('<br>'), closed: true }));
      h += irow({ rk: 'given:' + id, item: id, more: lines.join('<br>') });
    });
    return h || '<p class="muted">Nothing assigned by you is open.</p>';
  }
  // (357) My team as a group-by-person view: each group header carries that person's roll-up counts.
  // Under a Today filter a group with no matching row is left out.
  function listTeam() {
    var h = VS.my.team.map(function (p) {
      var m = VS.members[p.id], c = [p.open.length + ' open'];
      var work = p.open.concat(p.pending).filter(function (id) { return keepRec(A(id)); }), wait = p.waiting.filter(function (id) { return keepRec(A(id)); });
      if (FILT && !work.length && !wait.length) return '';
      if (p.done) c.push(p.done + ' done');
      if (p.blocked) c.push('<span class="err">' + p.blocked + ' blocked</span>');
      if (p.stale) c.push('<span class="err">' + p.stale + ' stale</span>');
      if (p.pending.length) c.push(p.pending.length + ' unaccepted');
      if (p.waiting.length) c.push('<span' + tipAttr('pending assignments they sent that nobody accepted yet') + '>' + p.waiting.length + ' waiting on others</span>');
      var g = '<div class="tg" style="--d:' + (p.depth - 1) + '"' + attr('person', p.id) + '><div class="tg-head"><i class="av">' + esc(initials(p.id)) + '</i><b>' + whoName(p.id) + '</b>' +
        '<span class="nid">' + esc(p.id) + ' L' + p.depth + (m.children.length ? ', lead' : '') + '</span><span class="cnt">' + c.join(' &middot; ') + '</span><span class="grow"></span>' +
        '<span class="muted ts"' + tipAttr('last update on their work') + '>' + esc(fmtTs(p.last)) + '</span></div>';
      g += work.map(function (id) { var r = A(id); return irow({ rk: 'team:' + id, item: r.item, rec: r }); }).join('');
      g += wait.map(function (id) {
        var r = A(id);
        return irow({ rk: 'team:w:' + id, item: r.item, rec: r, more: esc(nm(p.id)) + ' sent this to <b>' + esc(nm(r.to)) + '</b>' + (r.handoffOf ? ' (handoff)' : '') + '; not accepted yet' });
      }).join('');
      return g + '</div>';
    }).join('');
    return h || (FILT && VS.my.team.length ? filterEmpty() : '<div class="empty">Nobody below you.</div>');
  }
  function allRows(kanban) { return VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(kanban ? function (it) { return !it.hidden && kindFilter(it); } : itemFilter); }
  // (387) The tree and closed items. Under the default "open" status, a done item never
  // leaves its place silently: a closed child among open siblings folds into one "closed N"
  // row under its parent (a click expands it); a closed parent with an open descendant stays in
  // the tree, greyed; a closed parent whose whole subtree is closed moves, as one unit, to the
  // "recently closed" section (the parent's own roll-up, done 3/5, still counts the children).
  // While a search or the Today filter is on, the rows are the plain matches instead: the open
  // and greyed ones in tree order, the closed ones in the section, every age.
  function listAll() {
    var bar = '<div class="toolbar">' + itemFilterBar() + '</div>';
    if (ui.status !== 'open') {
      var rows = allRows().filter(function (it) { return keepItem(it.id); });
      return bar + (rows.length ? rows.map(function (it) { return irow({ rk: 'all:' + it.id, item: it.id, depth: it.depth }); }).join('') : FILT ? filterEmpty() : '<div class="empty">No items match.</div>');
    }
    var base = {}, order = [], kids = {}, openBelow = {};
    VS.itemOrder.forEach(function (id) { var it = VS.items[id]; if (!it.hidden && kindFilter(it)) { base[id] = it; order.push(id); } });
    order.forEach(function (id) { var par = base[id].parent; if (par && base[par]) (kids[par] = kids[par] || []).push(id); });
    function kidsOf(id) { return kids[id] || []; }
    function isOpen(id) { return itemFilter(base[id]); }
    function isClosed(id) { return base[id].kind !== 'topic' && base[id].status === 'done'; }
    function hasOpen(id) { if (!(id in openBelow)) openBelow[id] = kidsOf(id).some(function (k) { return isOpen(k) || hasOpen(k); }); return openBelow[id]; }
    function inTree(id) { return isOpen(id) || (isClosed(id) && hasOpen(id)); }   // open, or closed with something open below
    function isUnit(id) { return isClosed(id) && !hasOpen(id); }                  // closed, and so is everything below
    function parentOf(id) { var par = base[id].parent; return par && base[par] ? par : null; }
    function size(id) { return 1 + kidsOf(id).reduce(function (a, k) { return a + size(k); }, 0); }
    function latest(id) { return kidsOf(id).reduce(function (a, k) { var t = latest(k); return t > a ? t : a; }, isClosed(id) ? closeTs(base[id]) : ''); }
    function unitRows(id, off) {
      return irow({ rk: 'all:' + id, item: id, depth: base[id].depth - off, closed: true }) + kidsOf(id).map(function (k) { return unitRows(k, off); }).join('');
    }
    if (FILT) {
      var main = order.filter(function (id) { return inTree(id) && keepItem(id); }), shut = order.filter(function (id) { return isUnit(id) && keepItem(id); });
      shut.forEach(function (id) { addClosed(closeTs(base[id]), 1, irow({ rk: 'all:' + id, item: id, closed: true })); });
      return bar + (main.length ? main.map(function (id) { return irow({ rk: 'all:' + id, item: id, depth: base[id].depth, closed: isClosed(id) }); }).join('') : shut.length ? '<p class="muted">No open item matches.</p>' : filterEmpty());
    }
    function node(id) {
      var it = base[id], h = irow({ rk: 'all:' + id, item: id, depth: it.depth, closed: isClosed(id) });
      kidsOf(id).forEach(function (k) { if (inTree(k)) h += node(k); });
      var cu = kidsOf(id).filter(isUnit);
      if (cu.length) {
        var on = !!panelState().treeOpen[id], n = cu.reduce(function (a, k) { return a + size(k); }, 0), r = it.rollup;
        h += '<button type="button" class="csum' + (on ? ' open' : '') + '" style="--ind:' + (it.depth + 1) + '" aria-expanded="' + on + '"' + attr('act', 'tree-closed') + attr('parent', id) + attr('fk', 'csum:' + id) +
          tipAttr('closed items under ' + it.title + (r.total ? ' (' + (r.counts.done || 0) + '/' + r.total + ' of its children are done)' : '') + '; click to ' + (on ? 'fold' : 'show') + ' them') + '><svg class="i"><use href="#i-chev"/></svg>closed <span class="subn">' + n + '</span></button>' +
          (on ? cu.map(function (k) { return unitRows(k, 0); }).join('') : '');
      }
      return h;
    }
    var roots = order.filter(function (id) { return inTree(id) && !(parentOf(id) && inTree(parentOf(id))); });
    order.forEach(function (id) {
      if (isUnit(id) && !(parentOf(id) && (isUnit(parentOf(id)) || inTree(parentOf(id))))) addClosed(latest(id), size(id), unitRows(id, base[id].depth));
    });
    return bar + (roots.length ? roots.map(node).join('') : '<div class="empty">No open items.</div>');
  }
  function scopeChip(s) {
    if (s === 'mine') return VS.my.assignments.map(A).filter(function (r) { return r.status !== 'done'; }).length + ' open';
    if (s === 'received') return VS.my.received.pending.length + ' pending';
    if (s === 'given') return VS.my.given.length + (VS.my.given.length === 1 ? ' item' : ' items') + (VS.my.today.confirms.length ? ', ' + VS.my.today.confirms.length + ' to confirm' : '');
    if (s === 'team') {
      var t = { open: 0, blocked: 0, stale: 0, pending: 0 };
      VS.my.team.forEach(function (p) { t.open += p.open.length; t.blocked += p.blocked; t.stale += p.stale; t.pending += p.pending.length; });
      return VS.my.team.length + ' below you, ' + t.open + ' open' + (t.blocked ? ', ' + t.blocked + ' blocked' : '') + (t.stale ? ', ' + t.stale + ' stale' : '') + (t.pending ? ', ' + t.pending + ' unaccepted' : '');
    }
    return allRows(panelMode('all') === 'kanban').length + ' of ' + Object.keys(VS.items).length;
  }
  var LISTS = { mine: listMine, received: listReceived, given: listGiven, team: listTeam, all: listAll };
  function panelBody(s, m, v) { return m === 'list' ? LISTS[s](v) : modeBody(s, m, v); }
  // (366) the panel as a spec: the header is two rows (384), (385): scopes and the scope chip,
  // then the view icons and the search box. It is the card header when the panel stands alone,
  // or the tool area under the tab bar in the narrow layout.
  var VIEW_ICON = { list: 'list', tree: 'tree', kanban: 'kanban', 'due timeline': 'timeline', calendar: 'calendar' };
  function cardPanel(v) {
    var s = scopeOf(), m = panelMode(s), q = panelState().q;
    TF = todayFilter(); SQ = q.trim().toLowerCase(); FILT = !!(TF || SQ);
    CLOSED = m === 'list' ? { entries: [], older: 0 } : null;
    var scopes = '<span class="scopes" role="tablist" aria-label="scope">' + scopeList().map(function (x) {
      return '<button type="button" class="scp' + (x[0] === s ? ' on' : '') + '" role="tab" aria-selected="' + (x[0] === s) + '"' + attr('act', 'scope') + attr('scope', x[0]) + tipAttr(x[2]) + '>' + esc(x[1]) + '</button>';
    }).join('') + '</span>';
    var modes = SCOPE_MODES[s].length > 1 ? '<span class="vmodes" role="group" aria-label="view">' + SCOPE_MODES[s].map(function (x) {
      return '<button type="button" class="vbtn' + (m === x[0] ? ' on' : '') + '" aria-pressed="' + (m === x[0]) + '" aria-label="' + esc(x[1]) + '"' + attr('act', 'pmode') + attr('mode', x[0]) + tipAttr(x[1]) +
        '><svg class="i"><use href="#i-' + VIEW_ICON[x[1]] + '"/></svg></button>';
    }).join('') + '</span>' : '';
    var search = '<span class="psearch"><svg class="i"><use href="#i-search"/></svg><input id="psearch" type="text" role="searchbox" autocomplete="off" spellcheck="false" value="' + esc(q) + '" placeholder="Search this scope (Esc clears)" aria-label="search items in this scope: title, body, project, id">' +
      (q ? '<button type="button" class="psx"' + attr('act', 'psearch-clear') + ' aria-label="clear the search"' + tipAttr('clear the search (Esc)') + '><svg class="i"><use href="#i-x"/></svg></button>' : '') + '</span>';
    var body = panelBody(s, m, v), closed = closedHtml();
    TF = null; SQ = ''; FILT = false; CLOSED = null;
    return { title: 'Items', tools: scopes + '<span class="chip">' + esc(scopeChip(s)) + '</span><div class="prow2">' + modes + search + '</div>', body: '<div class="plist">' + body + '</div>' + closed, id: 'card-items', cls: 'panel',
      attrs: attr('scope', s) + attr('view', m) };
  }
  function panelHtml(p, withToday) {
    return '<section class="card panel" id="card-items"' + p.attrs + '><header><h2>Items</h2>' + p.tools + '</header>' + (withToday ? todayBar() : '') + '<div class="cbody">' + p.body + '</div></section>';
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
      return '<li><div class="node' + (id === v ? ' me' : '') + '"><i class="av">' + esc(m.avatar) + '</i>' + (m.profile.org ? '<span class="org">' + esc(m.profile.org) + '</span>' : '') +
        '<span class="nm">' + esc(m.name) + '</span><span class="nid">' + esc(id) + '</span>' + chips +
        '<span class="nid">' + esc(m.keys.join(', ')) + '</span>' + (id === v || me.admin ? btn('profile', 'Edit profile', { who: id }) : '') + '</div>' + (m.children.length ? '<ul>' + m.children.map(node).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(VS.members).filter(function (id) { var m = VS.members[id], l = m.leads; return !m.removed && (!l.length || !VS.members[l[0]] || VS.members[l[0]].removed); });
    var h = '<p class="r2" style="margin:0 0 6px">' + summary + '</p>';
    if (VS.rolesProblems.length) h += '<p class="err">roles file problems: ' + esc(VS.rolesProblems.join('; ')) + '</p>';
    h += '<ul class="tree">' + roots.map(node).join('') + '</ul>';
    var gone = Object.keys(VS.members).filter(function (id) { return VS.members[id].removed; });
    if (gone.length) h += '<p class="r2">removed: ' + esc(gone.join(', ')) + '</p>';
    var lost = Object.keys(VS.members).filter(function (id) { return !seen[id] && !VS.members[id].removed; });
    if (lost.length) h += '<p class="err">unreachable from any root: ' + esc(lost.join(', ')) + '</p>';
    h += '<div class="toolbar" style="margin-top:8px">' + btn('admin', 'Admin: roles event', {}) + btn('join', 'Join a store...', {}) +
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

  // (303) The kanban, activity timeline, due timeline, and calendar renderers draw the
  // body of the Items panel for one scope (its own rows; "all" keeps its filters), or of
  // the Activity card. The panel is the wide main zone (359), so no mode spans rows.
  function modeBody(key, m, v) {
    if (m === 'kanban') return kanbanBody(key, v);
    if (m === 'gantt') return ganttBody(key, dueRows(key, v));
    if (m === 'calendar') return calendarBody(key, dueRows(key, v));
    if (m === 'activity') return activityBody();
    return '';
  }
  // The assignment records a scope shows: mine (live plus pending to me), given (what I
  // assigned or handed to others, live or pending), team (followers' open and unaccepted work).
  // A Today filter (366) keeps only its records.
  function assignRecs(key, v) {
    var recs;
    if (key === 'mine') recs = VS.my.assignments.concat(VS.my.received.pending).map(A);
    else if (key === 'given') recs = [].concat.apply([], VS.my.given.map(function (id) {
      return VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); });
    }));
    else recs = [].concat.apply([], VS.my.team.map(function (p) { return p.open.concat(p.pending); })).map(A);
    return recs.filter(keepRec);
  }

  // Kanban: columns by status. Assignment scopes: cards are assignments (with the
  // pending column); "all": cards are items. Dragging moves only the viewer's own
  // live assignment (item.status, or item.close on the done column).
  var KCOLS = { assign: ['pending', 'todo', 'doing', 'blocked', 'awaiting', 'done'],
    items: ['unassigned', 'pending', 'todo', 'doing', 'blocked', 'awaiting', 'done', 'declined', 'returned'] };
  function kanbanBody(key, v) {
    var cols = KCOLS[key === 'all' ? 'items' : 'assign'], buckets = {};
    cols.forEach(function (c) { buckets[c] = []; });
    if (key === 'all') {
      VS.itemOrder.forEach(function (id) {
        var it = VS.items[id];
        // the columns already are the statuses, so the status filter would only empty them: kind filter only
        if (it.hidden || it.kind === 'topic' || !buckets[it.status] || !kindFilter(it) || !keepItem(id)) return;
        var mineR = it.assignments.map(A).filter(function (r) { return r.to === v && r.state === 'live'; })[0];
        buckets[it.status].push({ item: id, drag: mineR && mineR.status !== 'done' ? id : null,
          sub: (it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return r.to + (r.state === 'pending' ? '?' : ''); }).join(', ') || 'nobody') });
      });
    } else {
      assignRecs(key, v).forEach(function (r) {
        var col = r.state === 'pending' ? 'pending' : r.status;
        if (!buckets[col]) return;
        buckets[col].push({ item: r.item, rec: r, drag: r.to === v && r.state === 'live' && r.status !== 'done' ? r.item : null,
          who: key !== 'mine' ? r.to : null, sub: (key !== 'mine' ? esc(nm(r.to)) + ' &middot; ' : '') + source(r) });
      });
    }
    Object.keys(buckets).forEach(function (c) {
      buckets[c].sort(function (x, y) { var a = VS.items[x.item], b = VS.items[y.item]; return b.priorityLevel - a.priorityLevel || ((a.due || '9') < (b.due || '9') ? -1 : 1); });
    });
    var h = '<div class="toolbar">' + (key === 'all' ? itemFilterBar(true) : '') +
      '<span class="muted">drag a card to change the status of your own assignment (done = close)</span></div><div class="kanban">';
    h += cols.map(function (c) {
      return '<div class="kcol"' + attr('kcol', c) + '><div class="khead">' + stIcon(c) + '<b>' + esc(SI.info(c).label) + '</b> <span class="muted">' + buckets[c].length + '</span></div>' +
        buckets[c].map(function (x) {
          // (356) quiet cards: title, then initials (the record's assignee off "mine"), priority, due; the source on hover
          var it = VS.items[x.item];
          return '<div class="kcard' + (x.drag ? ' drag' : '') + '"' + (x.drag ? ' draggable="true"' + attr('drag', x.drag) : '') + attr('item', x.item) + tipHtml(x.sub) + '>' +
            '<div class="kc-t">' + titleRef(x.item) + '</div><div class="kc-m">' + warnHtml(it) + dotHtml(it) + (x.who ? '<span class="who"><i class="av">' + esc(initials(x.who)) + '</i></span>' : whoHtml(it)) +
            '<span class="grow"></span>' + priHtml(it) + dueHtml(it, x.rec ? stKey(it, x.rec) : c) + '</div></div>';
        }).join('') + '</div>';
    }).join('');
    return h + '</div>' + lastWrite();
  }

  var FAMILIES = [['all', 'all events'], ['item', 'item fields and status'], ['assign', 'assignment (assign, accept, decline, return, handoff, unassign)'], ['comment', 'comments'], ['roles', 'roles and profiles']];
  function family(t) {
    if (/^(roles|member)\./.test(t)) return 'roles';
    if (/^comment\./.test(t)) return 'comment';
    if (/^item\.(assign|accept|decline|return|handoff|unassign)$/.test(t)) return 'assign';
    return 'item';
  }
  function activityBody() {
    var f = ui.act, rows = VS.events.filter(function (n) {
      return (f.family === 'all' || family(n.type) === f.family) && (f.member === 'all' || n.member === f.member) &&
        (!f.flagged || n.status !== 'applied') && (!f.conc || n.markers.length);
    }).slice().sort(function (a, b) { return String(b.ts) < String(a.ts) ? -1 : String(b.ts) > String(a.ts) ? 1 : 0; });
    var people = [['all', 'everyone']].concat(Object.keys(VS.members).map(function (id) { return [id, nmo(id)]; }));
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
  // The dated rows a due timeline or calendar draws for a scope: one per item on
  // "all" (its filters apply), one per assignment record on an assignment scope,
  // with the record's own state and, off "mine", the assignee.
  function dueRows(key, v) {
    function dated(it) { return it && it.due && !it.hidden && it.kind !== 'topic'; }
    if (key === 'all') return VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(function (it) { return dated(it) && itemFilter(it) && keepItem(it.id); })
      .map(function (it) { return { item: it.id, status: it.status, chip: stIcon(it.status), who: '' }; });
    return assignRecs(key, v).filter(function (r) { return dated(VS.items[r.item]); }).map(function (r) {
      return { item: r.item, status: stKey(null, r), chip: stIcon(stKey(null, r)), who: key === 'mine' ? '' : nm(r.to) };
    });
  }
  function whoTag(x) { return x.who ? '<span class="muted">' + esc(x.who) + '</span>' : ''; }
  // Due-date timeline: one bar per row, from the item's start (first doing, else
  // creation) to its due day.
  function ganttBody(key, rows) {
    var bar = key === 'all' ? itemFilterBar() : '';
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
      return '<div class="grow-row click' + (ENDED[x.status] ? ' ended' : '') + '"' + attr('item', it.id) + '><div class="glabel">' + x.chip + titleRef(it.id) + whoTag(x) + priHtml(it) + '</div>' +
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
      btn('cal', 'today', { month: today().slice(0, 7), card: key }) + (key === 'all' ? itemFilterBar() : '') + '<span class="muted">' + n + ' due in ' + esc(m) + '</span></div>' +
      '<div class="cal">' + ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map(function (d) { return '<div class="cdow">' + d + '</div>'; }).join('');
    for (var i = 0; i < dow; i++) h += '<div class="cday off"></div>';
    for (var d = 1; d <= days; d++) {
      var ds = m + '-' + (d < 10 ? '0' : '') + d, list = byDay[ds] || [];
      h += '<div class="cday' + (ds === today() ? ' now' : '') + '"' + attr('day', ds) + '><div class="cnum">' + d + '</div>' + list.map(function (x) {
        return '<div class="citem click' + (ENDED[x.status] ? ' ended' : '') + '"' + attr('item', x.item) + tipHtml(SI.info(x.status).label + ' · ' + VS.items[x.item].priorityName + (x.who ? ' · ' + esc(x.who) : '')) + '>' + x.chip + titleRef(x.item) + '</div>';
      }).join('') + '</div>';
    }
    return h + '</div>';
  }

  // ------------------------------------------------------- chat card (placeholder)

  // (300) (302) (305) A placeholder for the support-agent chat child (the future
  // daemon-side local support agent): placement and a static sample only, with no
  // responder and no events. Entry shapes follow the dashboard conversation view
  // (user text, assistant text, a collapsed tool line) plus a tracker card reference
  // that renders an item from the view state and opens its item window (366).
  function crefHtml(id) {
    var it = VS.items[id];
    var who = it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return nm(r.to) + (r.state === 'pending' ? '?' : ''); });
    return '<div class="cref click"' + attr('item', id) + attr('cref', id) + ' title="open the item"><div class="r1">' + stChip(it.status) + prChip(it) + dueChip(it) +
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

  // (366) The drawer layout, switched by the window width (layoutOf):
  //   'a'      the Items panel (about two thirds) and one tab pane on the right holding
  //            Notifications, Support agent, Team, Activity, Data, one tab at a time;
  //   'd'      from about 1600px, three columns: Items | Notifications (always open) |
  //            the tab pane holding the remaining tabs;
  //   'narrow' below about 1000px, everything is one tab pane, Items first, with the
  //            Today chips at the top whatever tab is shown.
  // Only the shown tab is rendered. A hidden tab whose signature (tabSig) moved since it
  // was last shown carries a dot, like the dashboard's tab bar (`tdot`).
  var TABS = { items: 'Items', notify: 'Notifications', chat: 'Support agent', org: 'Team', activity: 'Activity', data: 'Data' };
  // (384) a tab is one icon button: the name is the tooltip and the aria-label, a dot rides its corner
  var TAB_TIP = { data: 'Data (debug): the raw roles file, refs, and clone state' };
  var TAB_ICON = { items: 'tasks', notify: 'bell', chat: 'agent', org: 'team', activity: 'activity', data: 'data' };
  var SIDE = ['notify', 'chat', 'org', 'activity', 'data'];
  function layoutOf() { var w = window.innerWidth; return w >= 1600 ? 'd' : w < 1000 ? 'narrow' : 'a'; }
  function tabState() { return (ui.tab[ui.store] = ui.tab[ui.store] || { side: 'notify', narrow: 'items' }); }
  function tabSig(k, v) {
    if (k === 'items') return VS.my.badge.join(',');
    if (k === 'notify') { var rm = readMap(); return feedEntries(v).filter(function (x) { return !rm[x.key]; }).map(function (x) { return x.key; }).join(','); }
    if (k === 'activity') return String(VS.events.length);
    if (k === 'org') return VS.events.filter(function (e) { return /^roles\./.test(e.type); }).length + '|' + Object.keys(VS.members).length;
    return '';
  }
  function tabSeen() { return (ui.tabSeen[seenKey(ui.store)] = ui.tabSeen[seenKey(ui.store)] || {}); }
  // the store's content is on screen: its drawer is open, or it is the store page (371)
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
    var nav = '<nav class="ptabs" role="tablist" aria-label="tracker tabs">' + tabs.map(function (k) {
      var on = k === active, dot = !on && tabDot(k, v);
      return '<button type="button" class="ptab" role="tab" aria-selected="' + on + '" aria-label="' + esc(TABS[k] + (dot ? ' (new)' : '')) + '"' + attr('act', 'tab') + attr('tab', k) + tipAttr(TAB_TIP[k] || TABS[k]) + '>' +
        '<svg class="i"><use href="#i-' + TAB_ICON[k] + '"/></svg><span class="tdot' + (dot ? ' on' : '') + '" aria-hidden="true"></span></button>';
    }).join('') + '</nav>';
    var tools = c.tools != null ? c.tools : (c.chip ? '<span class="chip">' + c.chip + '</span>' : '') + '<span class="grow"></span>' + c.extra;
    tabShown(active, v);
    return '<section class="card pane' + (c.cls ? ' ' + c.cls : '') + '" id="' + c.id + '"' + attr('tab', active) + (c.attrs || '') + (c.mode ? attr('view', c.mode) : '') + '><header>' + nav + '</header>' +
      (narrow ? todayBar() : '') + '<div class="ptools">' + tools + '</div><div class="cbody">' + c.body + '</div></section>';
  }
  function drawerHtml(name, v) {
    var vs = VS, nMem = Object.keys(vs.members).filter(function (id) { return !vs.members[id].removed; }).length;
    var MAKE = { items: cardPanel, notify: cardNotify, chat: cardChat, org: cardTeam, activity: cardActivity, data: cardData };
    var h = '<div class="dr-title"><b>' + esc(vs.store.label) + '</b><span>' + nMem + ' members, ' + vs.stats.events + ' events from ' + vs.stats.commits + ' commits, ' +
      vs.stats.flagged + ' flagged</span><span class="grow"></span>' +
      // (371) the drawer links to the store page; the page links back to the session view
      (ui.page ? '<button class="btn" data-act="page-back"' + tipAttr('back to the session view (the browser back button works too)') + '><svg class="i"><use href="#i-back"/></svg>Back to session</button>' :
        '<a class="btn" href="#/' + esc(encodeURIComponent(name)) + '"' + tipAttr('open this store as its own page (#/' + name + ')') + '><svg class="i"><use href="#i-page"/></svg>Open as page</a>') +
      '<button class="btn primary" data-act="create"><svg class="i"><use href="#i-plus"/></svg>New</button></div>' + syncHtml(name);
    if (!vs.viewer.isMember) h += '<p class="err">' + esc(v) + ' is not a member of this store (removed or never joined): writes are flagged.</p>';
    var L = ui.layout = layoutOf(), ts = tabState(), specs = {};
    function make(k) { return (specs[k] = specs[k] || MAKE[k](v)); }
    if (L === 'narrow') {
      var all = ['items'].concat(SIDE), act = all.indexOf(ts.narrow) >= 0 ? ts.narrow : 'items';
      make(act);
      return h + '<div class="dr-zones" data-layout="narrow"><div class="dr-col">' + paneHtml(all, act, v, specs, true) + '</div></div>';
    }
    var tabs = L === 'd' ? SIDE.slice(1) : SIDE, active = tabs.indexOf(ts.side) >= 0 ? ts.side : tabs[0];
    make(active);
    h += '<div class="dr-zones" data-layout="' + L + '"><div class="dr-col dr-main">' + panelHtml(make('items'), true) + '</div>';
    if (L === 'd') { h += '<div class="dr-col">' + cardHtml(make('notify')) + '</div>'; tabShown('notify', v); }
    tabShown('items', v);
    return h + '<div class="dr-col">' + paneHtml(tabs, active, v, specs, false) + '</div></div>';
  }

  // ------------------------------------------------------------ item window

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

  // (366) The item window: what the thread sheet and the row detail overlay held, in a
  // centred window over the page. The row's actions come first as labelled buttons
  // (actList), then the item actions a row does not carry (edit, hide, and assign or
  // split where the row has none). The assignees table keeps only the per-assignee
  // actions the action bar does not already offer.
  // (390) What an event refers to inside the open item window, found by the event's type:
  // a comment (added, edited, or hidden) -> that comment; a status, close, confirm, reject,
  // or reopen -> that assignee's entry on the holders line; an assignment or handoff -> the
  // holders line; an accept -> the accepter's entry on it; a decline, return, or unassign ->
  // the assignees row that ended (the holders line no longer lists it); a body edit or the
  // item's creation -> the body; an edit that only adds attachments -> the first new chip.
  // Anything else falls back to its own line in the thread, and null when the window has none.
  function focusTarget(root, evId) {
    var e = EV[evId];
    if (!e) return null;
    var p = e.payload || {}, by = function (sel, id) { return root.querySelector('[' + sel + '="' + CSS.escape(id) + '"]'); }, holders = root.querySelector('[data-focus="holders"]'), el = null;
    if (e.type === 'comment.add') el = by('data-ev', e.id);
    else if (e.type === 'comment.edit' || e.type === 'comment.hide') el = by('data-ev', p.comment);
    else if (STATUS_EV[e.type]) el = by('data-hmember', assigneeOf(e)) || holders;
    else if (e.type === 'item.assign' || e.type === 'item.handoff') el = holders;
    else if (e.type === 'item.accept') el = by('data-hmember', e.member) || holders;
    else if (e.type === 'item.decline' || e.type === 'item.return' || e.type === 'item.unassign') {
      var gone = vals(VS.assignments).filter(function (r) { return r.ended && r.ended.id === e.id; })[0];
      el = gone && by('data-rec', gone.id);
    }
    else if (e.type === 'item.create' || e.type === 'item.edit') {
      var atts = listOf(p.attachments).filter(function (x) { return x && x.oid; });
      el = !('body' in p) && atts.length ? by('data-oid', atts[0].oid) : root.querySelector('[data-focus="body"]');
    }
    return el || by('data-ev', e.id);
  }
  function windowActs(v, it) {
    var recs = it.assignments.map(A), mineR = recs.filter(function (r) { return r.to === v && r.state === 'live'; })[0];
    var gone = VS.viewer.admin && VS.members[it.author] && VS.members[it.author].removed; // (292)
    var list = actList(it).map(function (x) { return x[0] === 'open' ? ['win-discuss', 'chat', 'Discuss (comment below)', {}] : x; }), have = {};
    list.forEach(function (x) { have[x[0]] = true; });
    if ((it.author === v || gone) && !it.hidden) list.push(['edit', 'tasks', it.author === v ? 'Edit' : 'Edit (admin: creator removed)', { item: it.id }]);
    if (!have.assign && it.author === v && it.kind !== 'topic' && !it.hidden) list.push(['assign', 'assign', 'Assign...', { item: it.id }]);
    if (!have.split && (mineR || it.author === v) && it.kind !== 'topic' && !it.hidden) list.push(['split', 'split', 'Split into a subtask...', { item: it.id }]);
    if ((it.author === v || gone) && !it.hidden) list.push(['ihide', 'x', it.author === v ? 'Hide item' : 'Hide item (admin)', { item: it.id }]);
    return list.map(function (x) {
      return '<button type="button" class="btn' + (x[4] ? ' primary' : '') + '"' + attr('act', x[0]) + dataAttrs(x[3]) + '><svg class="i"><use href="#i-' + x[1] + '"/></svg>' + esc(x[2]) + '</button>';
    }).join('');
  }
  function windowHtml(v, it) {
    var recs = it.assignments.map(A), mineR = recs.filter(function (r) { return r.to === v && r.state === 'live'; })[0];
    var acts = windowActs(v, it), hs = holders(it);
    var h = '<div class="iw-top"><div class="iw-head">' + stIcon(stKey(it)) + '<h2 id="iw-title">' + esc(it.title) + '</h2>' +
      '<button type="button" class="ia iw-x" data-act="win-close" aria-label="close (Esc)"' + tipAttr('close (Esc)') + '><svg class="i"><use href="#i-x"/></svg></button></div>' +
      '<div class="iw-chips">' + kindChip(it) + '<code class="al">' + esc(it.alias) + '</code>' + (it.kind === 'topic' ? '' : stChip(it.status) + prChip(it) + dueChip(it) + rollupHtml(it)) + warnHtml(it) + '</div></div>';
    h += '<dl class="iw-meta">' +
      '<dt>status</dt><dd>' + (it.kind === 'topic' ? '<span class="muted">topic: discussion only</span>' : esc(SI.info(it.status).label) + ' <span class="muted">' + esc(SI.info(it.status).desc) + '</span>') + '</dd>' +
      '<dt>kind</dt><dd>' + esc(it.kind) + (it.pending.length ? ' <span class="muted">(pending assignment)</span>' : '') + (it.hidden ? ' <span class="muted">(hidden)</span>' : '') + '</dd>' +
      (it.kind !== 'topic' ? '<dt>holders</dt><dd' + attr('focus', 'holders') + '>' + (hs.length ? hs.map(function (r) { return '<span class="hold"' + attr('hmember', r.to) + '><i class="av' + (r.state === 'pending' ? ' pend' : '') + (r.to === v ? ' me' : '') + '">' + esc(initials(r.to)) + '</i> ' + whoName(r.to) + ' <span class="muted">' + esc(SI.info(stKey(it, r)).label) + '</span></span>'; }).join(', ') : '<span class="muted">not assigned</span>') + '</dd>' : '') +
      (it.chain.length > 1 ? '<dt>path</dt><dd>' + esc(it.chain.join(' > ')) + '</dd>' : '') +
      (it.kind !== 'topic' ? '<dt>due</dt><dd>' + (it.due ? esc(it.due) : '<span class="muted">-</span>') + '</dd>' : '') +
      '<dt>creator</dt><dd>' + esc(nm(it.author)) + (VS.members[it.author] && VS.members[it.author].admin ? ' <span class="chip admin">admin</span>' : '') + '</dd>' +
      '<dt>project</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>parent</dt><dd>' + (it.parent ? itemRef(it.parent) + ' ' + esc((VS.items[it.parent] || {}).title || '') : '<span class="muted">-</span>') + '</dd>' +
      '<dt>id</dt><dd><code>' + esc(it.id) + '</code></dd>' +
      '<dt>dates</dt><dd>created ' + esc(fmtTs(it.ts)) + (it.edited ? ' &middot; edited ' + esc(fmtTs(it.edited.ts)) : '') + ' &middot; last activity ' + esc(fmtTs(it.lastActivity)) + '</dd></dl>';
    h += it.body ? '<div class="sh-body"' + attr('focus', 'body') + '>' + esc(it.body) + '</div>' : '';
    h += mentionChips(it.mentions) + attChips(it.attachments, true);
    h += '<div class="sh-actions">' + (acts || '<span class="muted">No item actions for ' + esc(nm(v)) + '.</span>') + '</div>';

    if (it.kind !== 'topic') {
      h += '<h3>Assignees</h3>';
      if (!recs.length) h += '<div class="empty">Not assigned.</div>';
      else h += '<table class="atable">' + recs.map(function (r) {
        var b = '';
        if (r.state === 'live') {
          if ((r.by === v || r.to === v) && r.status === 'done') b += btn('note', 'Reopen', { item: r.item, who: r.to, op: 'item.reopen' });
          var departed = (VS.members[r.to] || {}).removed || (VS.members[r.by] || {}).removed;
          if (r.by === v || (departed && VS.viewer.admin)) b += btn('unassign', r.by === v ? 'Unassign' : 'Unassign (admin, member removed)', { item: r.item, who: r.to });
          if (VS.viewer.admin && r.to !== v && r.status !== 'done' && !it.hidden) b += btn('handoff', 'Hand off (admin)', { item: r.item, who: r.to }); // (286)
        } else if (r.state === 'pending' && r.to === v && (r.by === v || r.sender === v)) b += btn('unassign', 'Withdraw', { item: r.item, who: r.to });
        return '<tr class="' + (r.state !== 'live' && r.state !== 'pending' ? 'gone' : '') + '"' + attr('rec', r.id) + '><td><b>' + esc(nm(r.to)) + '</b><br><span class="muted">' + source(r) + '</span></td><td>' +
          recChip(r) + (r.ended ? ' <span class="muted">' + esc(alias(r.ended.id)) + endedBy(r) + '</span>' : '') +
          '<br><span class="muted">' + complText(r.completion) + '</span></td><td>' + (r.note ? '&ldquo;' + esc(r.note) + '&rdquo;' : r.answerNote ? '&ldquo;' + esc(r.answerNote) + '&rdquo;' : '<span class="muted">no note</span>') +
          (b ? '<div class="tb">' + b + '</div>' : '') + '</td></tr>';
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
    h += '<h3>Thread (' + evs.length + ' events)</h3><ul class="timeline">' + evs.map(function (n) {
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
      Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).map(function (id) { return '<option value="' + esc(id) + '">@' + esc(id) + ' ' + esc(nmo(id)) + '</option>'; }).join('') +
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
    return optsHtml(Object.keys(VS.members).filter(function (id) { return !filter || filter(VS.members[id]); }).map(function (id) { return [id, nmo(id) + ' (' + id + ')']; }), 'nobody', cur);
  }
  function reachTag(v, id) { return id === v ? 'self' : VS.viewer.reach[id] ? 'forced, L' + VS.viewer.reach[id] : 'pending'; }
  // Anyone in the store: below me = forced (live at once); anyone else = pending until accepted.
  function peopleHtml(v) {
    var R = VS.viewer.reach, ids = Object.keys(VS.members).filter(function (id) { return id !== v && !VS.members[id].removed; });
    ids.sort(function (a, b) { return (R[a] || 99) - (R[b] || 99) || (a < b ? -1 : 1); });
    return '<label><input type="checkbox" name="people" value="' + esc(v) + '"> myself</label>' + ids.map(function (id) {
      return '<label><input type="checkbox" name="people" value="' + esc(id) + '"> ' + whoName(id) + ' <span class="muted">' + reachTag(v, id) + '</span></label>';
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
  // (393) A person's earlier profiles are machine-local, never in the ledger: simulated here
  // with localStorage (the real flow belongs to the later caller ticket). Most recent first;
  // the first entry is the last-used profile.
  var PROF_KEY = 'tracker-mockup-profiles';
  function loadProfiles() {
    try {
      var x = JSON.parse(localStorage.getItem(PROF_KEY));
      return Array.isArray(x) ? x.filter(function (e) { return e && typeof e.given === 'string' && e.given; }).map(function (e) { return { org: String(e.org || ''), family: String(e.family || ''), given: e.given }; }) : [];
    } catch (e) { return []; }
  }
  function rememberProfile(pr) {
    if (!pr.given.trim()) return;
    var one = { org: pr.org.trim(), family: pr.family.trim(), given: pr.given.trim() };
    var list = loadProfiles().filter(function (x) { return x.org !== one.org || x.family !== one.family || x.given !== one.given; });
    list.unshift(one);
    try { localStorage.setItem(PROF_KEY, JSON.stringify(list.slice(0, 12))); } catch (e) { /* storage off: not remembered */ }
  }
  function profileText(pr) { return (pr.org ? pr.org + ' ' : '') + E.displayName({ org: pr.org, family: pr.family, given: pr.given }); }
  function profileFields(pr) {
    return '<div class="fld inline">' + fld('org (optional)', '<input type="text" name="org" value="' + esc(pr.org) + '">') +
      fld('family name (optional)', '<input type="text" name="family" value="' + esc(pr.family) + '">') +
      fld('given name (required)', '<input type="text" name="given" value="' + esc(pr.given) + '">') + '</div>' +
      '<div class="hint" id="pview"></div>';
  }
  function profileView(form) {
    var el = form.elements, pv = form.querySelector('#pview');
    if (!pv || !el.given) return;
    var pr = { org: el.org.value.trim(), family: el.family.value.trim(), given: el.given.value.trim() };
    pv.classList.toggle('warn', !pr.given);
    pv.textContent = pr.given ? 'Shown as ' + E.displayName(pr) + (pr.org ? ' (org ' + pr.org + ')' : '') + ', avatar ' + E.avatar(pr) : 'The given name is required; an empty one is flagged.';
  }
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
            Object.keys(VS.members).filter(function (id) { return id !== v && !VS.members[id].removed; }).map(function (id) { return '<option value="' + esc(id) + '">' + esc(nmo(id)) + ' (' + reachTag(v, id) + ')</option>'; }).join('') + '</select>') +
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
      case 'join': {
        // (393) the chosen profile joins the current store. A join is a self-write (member.join, (231));
        // the mockup has the roles.add_member stand-in, which only an admin writes, so an admin's clone
        // writes it and the new member's clone is created and opened.
        if (!VS.store.remote) { T = ['Join ' + VS.store.name, 'This store has no remote, so nobody can join it. Open a team store first.', '', 'Close']; break; }
        var saved = loadProfiles(), p0 = saved[0] || { org: '', family: '', given: '' };
        T = ['Join ' + VS.store.name, 'Joins this store as a new member. The saved profiles are this machine\'s own list (localStorage), never in the ledger: pick one, or edit the fields, which saves a new entry. The profile goes into the join payload.',
          fld('saved profiles', '<select name="saved">' + saved.map(function (x, i) { return '<option value="' + i + '" data-p="' + esc(JSON.stringify(x)) + '"' + (i === 0 ? ' selected' : '') + '>' + esc(profileText(x)) + (i === 0 ? ' (last used)' : '') + '</option>'; }).join('') +
            '<option value="">' + (saved.length ? '(edited)' : '(none saved)') + '</option></select>') + profileFields(p0) +
          '<div class="fld inline">' + fld('member id (a handle, no ".")', '<input type="text" name="nid" value="' + esc(asciiId(p0.given)) + '">') +
          fld('lead', '<select name="lead"><option value="">(none: root)</option>' + memberOpts('', function (m) { return !m.removed; }) + '</select>') + '</div>', 'Join'];
        break;
      }
      case 'profile': {
        var pw = d.who || v, pm = VS.members[pw];
        T = [pw === v ? 'Edit your profile' : 'Edit the profile of ' + esc(nm(pw)),
          'member.profile: ' + (pw === v ? 'your own profile.' : 'an admin changing another member\'s profile; it shows in Activity with you as the author.') + ' Only the fields you change are written; each merges by the (203) rule.',
          profileFields(pm.profile) + '<input type="hidden" name="who" value="' + esc(pw) + '">', 'Save'];
        break;
      }
      case 'admin':
        T = ['Roles event', 'Admin-only. Written to ' + esc(viewerOf(ui.store)) + '\'s own journal; replay applies it to the roles file in causal order.',
          fld('type', '<select name="rtype">' + ROLE_TYPES.map(function (t) { return '<option>' + t + '</option>'; }).join('') + '</select>') +
          fld('member (existing)', '<select name="member">' + memberOpts('') + '</select>') +
          fld('lead (set_lead, add_member)', '<select name="lead"><option value="">(none: root)</option>' + memberOpts(v) + '</select>') +
          fld('key (add_key, revoke_key, add_member)', '<input type="text" name="key" placeholder="e.g. jon-k3">') +
          fld('new member id and profile (add_member)', '<div class="fld inline"><input type="text" name="nid" placeholder="id"><input type="text" name="norg" placeholder="org"><input type="text" name="nfamily" placeholder="family"><input type="text" name="ngiven" placeholder="given"></div>') +
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

  function asciiId(given) { return String(given || '').toLowerCase().replace(/[^a-z0-9]+/g, '') || 'newcomer'; }
  function openDialog(kind, d) {
    var dl = $('dlg');
    dl.innerHTML = dialogHtml(kind, d || {});
    var f = dl.querySelector('form');
    updateHint(f); profileView(f);
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
  function commit(events, asStream) {
    var name = ui.store, s = asStream || viewerOf(name), ts = ui.clock[name];
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
      case 'join': {
        if (!VS.store.remote) return closeDialog();
        var jp = { org: val('org').trim(), family: val('family').trim(), given: val('given').trim() }, jid = val('nid').trim();
        var adm = VS.admins.filter(function (id) { return VS.members[id] && !VS.members[id].removed; })[0];
        var adminStream = adm && stores[ui.store].clonesOf(adm)[0];
        if (!adminStream) return toast('No admin clone to write the stand-in join.');
        var jres = commit([['roles.add_member', { member: { id: jid, profile: jp, keys: [jid + '-k1'], leads: val('lead') ? [val('lead')] : [] } }]], adminStream);
        if (jres[0].status !== 'applied') return; // flagged: the toast says why; the write stays in the admin clone's journal
        rememberProfile(jp);
        var jst = stores[ui.store], jclone = jid + '.pc';
        jst.sync(adminStream, ui.clock[ui.store]);
        jst.addClone(jclone, ui.clock[ui.store]);
        switchViewer(jclone);
        toast('Joined ' + esc(ui.store) + ' as ' + esc(profileText(jp)) + ' (stand-in: ' + esc(jres[0].alias) + ' written by ' + esc(adm) + '). Now viewing as ' + esc(jclone) + '.');
        return;
      }
      case 'profile': {
        var who = val('who') || v, pm = VS.members[who].profile, pp = {};
        PROFILE_FIELDS.forEach(function (f) { if (val(f).trim() !== pm[f]) pp[f] = val(f).trim(); });
        if (!Object.keys(pp).length) { closeDialog(); return toast('Nothing changed.'); }
        var pe = { profile: pp };
        if (who !== v) pe.member = who;
        if (who === v) rememberProfile({ org: val('org'), family: val('family'), given: val('given') });
        evs.push(['member.profile', pe]);
        break;
      }
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
        if (t === 'roles.add_member') rp.member = { id: val('nid'), profile: { org: val('norg'), family: val('nfamily'), given: val('ngiven') }, keys: val('key') ? [val('key')] : [], leads: val('lead') ? [val('lead')] : [] };
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
    if (viewing()) fetchNow(name);
    vss[name] = stores[name].view(stream, ui.clock[name]);
    if (viewing()) markSeen(name);
    render();
  }

  // ------------------------------------------------------------------ render

  function measure() {
    // handle width first: it pads the header, which can change how the header wraps and so where it ends
    var d = document.documentElement.style;
    d.setProperty('--drhw', Math.ceil($('drhandles').getBoundingClientRect().width) + 'px');
    d.setProperty('--drtop', Math.max(0, Math.round($('top').getBoundingClientRect().bottom)) + 'px');
  }
  // (382) The drawer's height is the user's: dragging a store's handle sets it, clamped
  // between DR_MIN and 60% of the window like the dashboard drawer, kept per store as
  // local UI state in localStorage and restored on reload. Only while a store's drawer
  // was never dragged does it follow its content (366), up to the same maximum: the
  // .dr-measure rules in style.css lift every height cap for one synchronous layout (so
  // nothing paints), then --drh is set. Panels scroll inside whatever the height is.
  var DR_MIN = 160, DR_SHUT = 80;
  function drMax() { return Math.round(window.innerHeight * 0.6); }
  function drClamp(h) { var hi = drMax(); return Math.max(Math.min(DR_MIN, hi), Math.min(hi, h)); }
  function loadDrh() { // called while ui is built, so the key stays inline (a var below would still be undefined)
    try { var m = JSON.parse(localStorage.getItem('tracker-mockup-drawer-h')); return m && typeof m === 'object' ? m : {}; } catch (e) { return {}; }
  }
  function saveDrh() { try { localStorage.setItem('tracker-mockup-drawer-h', JSON.stringify(ui.drh)); } catch (e) { /* storage off: the height lasts this page */ } }
  function fitDrawer() {
    var root = document.documentElement, sheet = $('drclip').firstElementChild, set = ui.drh[ui.store];
    if (set > 0) { root.style.setProperty('--drh', drClamp(set) + 'px'); return; }
    root.classList.add('dr-measure');
    var nat = Math.ceil(sheet.getBoundingClientRect().height);
    root.classList.remove('dr-measure');
    root.style.setProperty('--drh', Math.max(120, Math.min(nat, drMax())) + 'px');
  }
  // The drag (382): the handle follows the pointer; the clip and the handle track the
  // pointer's extent (--drext) while the sheet keeps at least DR_MIN, so a short drag
  // reveals the sheet rather than squashing it. The release sets the height, or closes
  // short of DR_SHUT. A press that never moves is a click.
  var drDrag = null, drDragged = false;
  function drDragStart(e) {
    var b = e.target.closest && e.target.closest('.dr-handle[data-handle]');
    if (!b || b.getAttribute('data-handle') === 'panels' || (e.button !== undefined && e.button !== 0)) return;
    drDrag = { name: b.getAttribute('data-handle'), y0: e.clientY, off: e.clientY - b.getBoundingClientRect().top, moved: false, ext: 0 };
    try { b.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the moves still land */ }
  }
  function drDragMove(e) {
    var d = drDrag;
    if (!d || (!d.moved && Math.abs(e.clientY - d.y0) < 5)) return;
    if (!d.moved) { d.moved = true; if (ui.open !== d.name) openStore(d.name); } // dragging a closed handle opens its store
    var max = drMax(), top = Math.max(0, Math.round($('top').getBoundingClientRect().bottom)), r = document.documentElement.style;
    d.ext = Math.max(0, Math.min(max, e.clientY - d.off - top));
    $('drawer').classList.add('drag');
    r.setProperty('--drh', Math.max(Math.min(DR_MIN, max), d.ext) + 'px');
    r.setProperty('--drext', d.ext + 'px');
  }
  function drDragEnd() {
    var d = drDrag;
    drDrag = null;
    if (!d || !d.moved) return;
    drDragged = true;
    setTimeout(function () { drDragged = false; }, 0);
    $('drawer').classList.remove('drag');
    document.documentElement.style.removeProperty('--drext');
    if (d.ext < DR_SHUT) { fitDrawer(); if (ui.open) { var n = ui.open; ui.open = null; syncNow(n, true); render(); } return; }
    ui.drh[d.name] = drClamp(d.ext); saveDrh();
    fitDrawer();
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
    if (ui.item && !VS.items[ui.item]) { ui.item = null; ui.win = false; }
    // per-render row marks (356): flagged writes in an item's own thread, unread notification entries per item
    FLAGGED = {}; UNREAD = {};
    VS.events.forEach(function (e) { if (e.status === 'flagged' && e.thread && VS.items[e.thread]) FLAGGED[e.thread] = (FLAGGED[e.thread] || 0) + 1; });
    var rm = readMap();
    feedEntries(v).forEach(function (x) { if (!rm[x.key]) UNREAD[x.item] = (UNREAD[x.item] || 0) + 1; });

    $('viewer').innerHTML = Object.keys(VS.members).map(function (k) {
      var m = VS.members[k];
      return '<option value="' + esc(k) + '"' + (k === v ? ' selected' : '') + '>' + esc(nmo(k) + ' (' + k + ', ' + (m.removed ? 'removed' : m.role) + ')') + '</option>';
    }).join('');
    $('vclone').innerHTML = (VS.clones[v] || []).map(function (c) {
      return '<option value="' + esc(c.stream) + '"' + (c.stream === s ? ' selected' : '') + '>' + esc(E.cloneOf(c.stream)) + (c.online ? '' : ' (offline)') + (c.unpushed ? ' *' : '') + '</option>';
    }).join('');
    $('vclock').textContent = fmtTs(ui.clock[name]);
    renderHandles();
    $('drawer').classList.toggle('open', !!ui.open);

    // (371) the store renders into the store page or the drawer, never both (the card ids are unique)
    var body = ui.page ? $('spage') : $('drbody'), sb = $('iwinbody'), top1 = body.scrollTop, top2 = sb.scrollTop;
    (ui.page ? $('drbody') : $('spage')).innerHTML = '';
    $('spage').hidden = !ui.page; document.body.classList.toggle('paged', !!ui.page);
    var draft = $('cbox') ? $('cbox').value : '';
    // (350) cards scroll inside; keep each card's scroll and the focused row across the re-render
    var scrolls = {}, fa = document.activeElement, focusRk = fa && fa.closest && body.contains(fa) && fa.closest('[data-rk]');
    focusRk = focusRk && focusRk.getAttribute('data-rk');
    var focusSearch = fa && fa.id === 'psearch' ? [fa.selectionStart, fa.selectionEnd] : null; // (385)
    var focusFk = fa && fa.closest && body.contains(fa) && fa.closest('[data-fk]'); // (386), (387) the fold buttons keep keyboard focus across their re-render
    focusFk = focusFk && focusFk.getAttribute('data-fk');
    Array.prototype.forEach.call(body.querySelectorAll('.card[id] > .cbody'), function (c) { scrolls[c.parentNode.id] = c.scrollTop; });
    hideTip();
    body.innerHTML = drawerHtml(name, v);
    if (!ui.page) fitDrawer();
    Array.prototype.forEach.call(body.querySelectorAll('.card[id] > .cbody'), function (c) { if (scrolls[c.parentNode.id]) c.scrollTop = scrolls[c.parentNode.id]; });
    var chat = body.querySelector('.chat-log');
    if (chat) chat.scrollTop = chat.scrollHeight; // newest message at the bottom
    body.scrollTop = top1;
    if (focusSearch && $('psearch')) { $('psearch').focus({ preventScroll: true }); $('psearch').setSelectionRange(focusSearch[0], focusSearch[1]); }
    var ff = focusFk && lastInput === 'key' && body.querySelector('[data-fk="' + CSS.escape(focusFk) + '"]');
    if (ff) ff.focus({ preventScroll: true });
    var fr = focusRk && lastInput === 'key' && body.querySelector('[data-rk="' + CSS.escape(focusRk) + '"]');
    if (fr) fr.focus({ preventScroll: true });
    var sr = ui.sel && body.querySelector('[data-rk="' + CSS.escape(ui.sel) + '"]');
    if (sr) placeMore(sr);
    // (366) the item window: one at a time; the rest of the page is inert while it is open
    var open = !!(ui.win && ui.item);
    $('iwin').hidden = !open;
    $('iwin').firstElementChild.setAttribute('aria-labelledby', 'iw-title');
    document.querySelector('.dwrap').inert = open; $('drawer').inert = open;
    if (open) {
      sb.innerHTML = windowHtml(v, VS.items[ui.item]);
      if (!ui.itemChanged && !ui.clearDraft && $('cbox')) $('cbox').value = draft;
      sb.scrollTop = ui.itemChanged ? 0 : top2;
      // (390) the notification's target: a thin mark stays while the window is open; the flash and the scroll happen once
      var fx = ui.focus && focusTarget(sb, ui.focus.ev);
      if (fx) {
        fx.classList.add('fx-mark');
        if (ui.focus.flash) { fx.classList.add('fx-flash'); fx.scrollIntoView({ block: 'center' }); }
      }
      if (ui.focus) ui.focus.flash = false;
    } else sb.innerHTML = '';
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
    if (ui.store !== name) { ui.item = null; ui.win = false; ui.last = null; ui.itemChanged = true; $('drbody').scrollTop = 0; }
    ui.store = name; ui.open = name;
    fetchNow(name); // immediate check when the drawer opens
    vss[name] = stores[name].view(viewerOf(name), ui.clock[name]);
    markSeen(name);
    render();
  }
  // (366) openItemWindow(storeId, itemId): the one entry point that opens an item, from a
  // row, a notification, a chat reference, a subtask link, or (later) a URL route. It
  // switches to the store when needed (the drawer stays as it is, or closes when it shows
  // another store), replaces any open window, and moves focus into it. Returns false
  // when the store or the item is unknown to the current viewer.
  // `opts` is { focus: eventId, opener: element } (a bare element still means the opener): the
  // window then scrolls to and marks what the event refers to (390), and just opens when the
  // window has nothing for it. An event URL route would reuse exactly this.
  function openItemWindow(storeId, itemId, opts) {
    var opener = opts && opts.nodeType ? opts : opts && opts.opener, focus = opts && !opts.nodeType && opts.focus;
    if (!stores[storeId]) return false;
    if (!stores[storeId].view(viewerOf(storeId), ui.clock[storeId]).items[itemId]) return false;
    if (ui.page && ui.page !== storeId) { history.pushState(null, '', '#/' + encodeURIComponent(storeId)); applyRoute(); } // on a store page: go to the item's store page
    if (ui.store !== storeId) {
      if (ui.open) { syncNow(ui.open, true); ui.open = null; }
      ui.store = storeId; ui.last = null; ui.sel = null;
    }
    if (!ui.win) ui.winReturn = returnSel(opener || document.activeElement);
    ui.item = itemId; ui.win = true; ui.itemChanged = true; ui.replyTo = null; ui.mdraft = []; ui.adraft = [];
    ui.focus = focus ? { ev: focus, flash: true } : null;
    render();
    $('iwin').firstElementChild.focus({ preventScroll: true });
    return true;
  }
  function showItem(id, opener, focus) { openItemWindow(ui.store, id, { opener: opener, focus: focus }); }
  // A selector for the element focus returns to: the row, feed entry, or reference that
  // opened the window survives a re-render under the same data-rk / data-nkey / data-cref.
  function returnSel(el) {
    if (!el || el === document.body) return null;
    var k = ['rk', 'nkey', 'cref', 'handle', 'open-store'];
    for (var i = 0; i < k.length; i++) { var c = el.closest('[data-' + k[i] + ']'); if (c) return '[data-' + k[i] + '="' + CSS.escape(c.getAttribute('data-' + k[i])) + '"]'; }
    return el.id ? '#' + CSS.escape(el.id) : null;
  }
  function closeItemWindow() {
    var back = ui.winReturn;
    ui.win = false; ui.winReturn = null; ui.focus = null; ui.replyTo = null; ui.mdraft = []; ui.adraft = [];
    render();
    var el = back && document.querySelector(back);
    if (el && !el.matches('a[href], button, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]'); // a feed entry: its title link
    if (el) el.focus();
  }

  // (371) The store page: the route #/<storeId> shows that store full-page, with the same
  // Items panel and tab pane layout, in place of the conversation and the drawer. It is
  // the only route (no item routes: an item URL would point at the viewer's own daemon,
  // so sharing it across people was rejected, (369)). Entering a page fetches and shows
  // what is new, like opening the drawer; leaving it pushes, like a collapse. Clearing
  // the hash, the browser back button, or "Back to session" returns to the session view.
  function routeStore() {
    var m = /^#\/([^/]+)$/.exec(location.hash), id = m && decodeURIComponent(m[1]);
    return id && stores[id] ? id : null;
  }
  function applyRoute() {
    var s = routeStore();
    if (location.hash.length > 2 && !s) toast('No store named ' + esc(location.hash.slice(2)) + '; showing the session view.');
    if (s !== ui.page) {
      if (ui.page) syncNow(ui.page, true);
      if (s) {
        if (ui.open) { syncNow(ui.open, true); ui.open = null; }
        if (ui.store !== s) { ui.item = null; ui.win = false; ui.last = null; ui.sel = null; ui.itemChanged = true; }
        ui.store = s; fetchNow(s);
        vss[s] = stores[s].view(viewerOf(s), ui.clock[s]);
        markSeen(s);
      }
      ui.page = s;
    }
    render();
  }
  function leavePage() { history.pushState(null, '', location.pathname + location.search); applyRoute(); }

  // (372) b The quick switcher: the magnifier next to the gear opens one search over the
  // stores (by name and label) and the items of every store (by title; items hidden by a
  // tombstone are left out, everything else is visible to every member (148)), read from
  // each store's current view state. A store opens its page; an item opens its window.
  // Arrow keys move, Enter picks, Esc closes, and focus returns to where it was.
  var qs = { open: false, items: [], idx: 0, back: null };
  function qResults(q) {
    q = q.trim().toLowerCase();
    var out = [];
    order.forEach(function (n) {
      var st = vss[n].store;
      if (!q || n.toLowerCase().indexOf(q) >= 0 || String(st.label).toLowerCase().indexOf(q) >= 0) out.push({ kind: 'store', store: n, label: st.label });
    });
    // (393) the join action for the store the page is on (a store with no remote has nothing to join)
    if ((!q || 'join'.indexOf(q) >= 0 || q.indexOf('join') >= 0) && vss[ui.store].store.remote) out.push({ kind: 'join', store: ui.store, label: vss[ui.store].store.label });
    if (q) order.forEach(function (n) {
      var V = vss[n];
      V.itemOrder.forEach(function (id) {
        var it = V.items[id];
        if (!it.hidden && it.title.toLowerCase().indexOf(q) >= 0) out.push({ kind: 'item', store: n, item: id, title: it.title, alias: it.alias, st: it.kind === 'topic' ? 'topic' : it.status });
      });
    });
    return out.slice(0, 30);
  }
  function qRender() {
    var list = qs.items = qResults($('qin').value), last = null, h = '';
    if (qs.idx >= list.length) qs.idx = Math.max(0, list.length - 1);
    list.forEach(function (x, i) {
      if (x.kind !== last) { last = x.kind; h += '<li class="qgrp" role="presentation">' + (x.kind === 'store' ? 'Stores' : x.kind === 'join' ? 'Actions' : 'Items') + '</li>'; }
      h += '<li class="qopt' + (i === qs.idx ? ' on' : '') + '" role="option" id="qopt-' + i + '" aria-selected="' + (i === qs.idx) + '"' + attr('qi', i) + '>' +
        (x.kind === 'join' ? '<svg class="i"><use href="#i-plus"/></svg><b>Join ' + esc(x.store) + '</b><span class="muted">with a saved or new profile</span><span class="grow"></span><span class="qk">join</span>' :
          x.kind === 'store' ? '<svg class="i"><use href="#i-tasks"/></svg><b>' + esc(x.store) + '</b><span class="muted">' + esc(x.label) + '</span><span class="grow"></span><span class="qk">open page</span>' :
          '<span class="sti">' + SI.svg(x.st) + '</span><span class="qt">' + esc(x.title) + '</span><span class="grow"></span><code class="al">' + esc(x.alias) + '</code><span class="qk">' + esc(x.store) + '</span>') + '</li>';
    });
    $('qlist').innerHTML = h || '<li class="qnone" role="presentation">No store or item matches.</li>';
    $('qin').setAttribute('aria-activedescendant', list.length ? 'qopt-' + qs.idx : '');
    var on = $('qlist').querySelector('.qopt.on');
    if (on) on.scrollIntoView({ block: 'nearest' });
  }
  function qOpen() {
    if (qs.open) return;
    qs.open = true; qs.idx = 0; qs.back = document.activeElement;
    gearMenu(false);
    $('qpop').hidden = false; $('qs').setAttribute('aria-expanded', 'true');
    $('qin').value = ''; qRender(); $('qin').focus();
  }
  function qClose(keepFocus) {
    if (!qs.open) return;
    qs.open = false; $('qpop').hidden = true; $('qs').setAttribute('aria-expanded', 'false');
    var b = qs.back; qs.back = null;
    if (!keepFocus && b && b.isConnected) b.focus(); else if (!keepFocus) $('qs').focus();
  }
  function qPick(i) {
    var x = qs.items[i];
    if (!x) return;
    qClose(true);
    if (x.kind === 'join') { openDialog('join', {}); return; }
    if (x.kind === 'store') { if (ui.page !== x.store) location.hash = '#/' + encodeURIComponent(x.store); else $('qs').focus(); }
    else if (!openItemWindow(x.store, x.item, { opener: $('qs') })) toast('That item is not visible to this viewer.');
  }

  // Theme (board (347) a): auto / light / dark, local UI state in localStorage. Applied as
  // data-theme="light|dark" on the root, absent for auto, matching the dashboard's gear menu;
  // index.html applies the saved value before first paint.
  var THEME_KEY = 'tracker-mockup-theme';
  function themeOf() { try { var t = localStorage.getItem(THEME_KEY); return t === 'light' || t === 'dark' ? t : 'auto'; } catch (e) { return 'auto'; } }
  function setTheme(t) {
    try { if (t === 'auto') localStorage.removeItem(THEME_KEY); else localStorage.setItem(THEME_KEY, t); } catch (e) { /* storage off: the choice lasts this page */ }
    if (t === 'auto') document.documentElement.removeAttribute('data-theme'); else document.documentElement.setAttribute('data-theme', t);
    Array.prototype.forEach.call(document.querySelectorAll('#gmenu [data-theme-set]'), function (b) { b.setAttribute('aria-pressed', String(b.getAttribute('data-theme-set') === t)); });
  }
  function gearMenu(open) { $('gmenu').hidden = !open; $('gear').setAttribute('aria-expanded', String(open)); }

  // (355) The immediate custom tooltip: one fixed element shown at once for whatever
  // [data-tip] the pointer or keyboard focus is on (the browser's title waits ~1s).
  var tipFor = null;
  function showTip(el) {
    var t = $('tip');
    tipFor = el; t.textContent = el.getAttribute('data-tip'); t.hidden = false;
    var r = el.getBoundingClientRect(), w = t.offsetWidth, h = t.offsetHeight;
    var x = Math.max(6, Math.min(r.left + r.width / 2 - w / 2, window.innerWidth - w - 6)), y = r.bottom + 6;
    if (y + h > window.innerHeight - 6) y = r.top - h - 6;
    t.style.left = Math.round(x) + 'px'; t.style.top = Math.round(y) + 'px';
  }
  function hideTip() { tipFor = null; $('tip').hidden = true; }
  // A row's detail overlay (the quick preview; the item window holds the full view) is
  // fixed-positioned so a short, content-height card (366) does not clip it: below the
  // row, or above when below would leave the open drawer and above has more room.
  function placeMore(row) {
    var m = row.querySelector(':scope > .ir-more');
    if (!m) return;
    var r = row.getBoundingClientRect(), c = $(ui.page ? 'spage' : 'drclip').getBoundingClientRect(), ind = Number(row.style.getPropertyValue('--ind')) || 0;
    var left = r.left + 28 + ind * 16;
    m.style.left = Math.round(left) + 'px'; m.style.width = Math.round(Math.max(220, r.right - 6 - left)) + 'px';
    var h = m.offsetHeight, lo = Math.min(c.bottom, window.innerHeight) - 4, hi = Math.max(c.top, 0) + 4;
    var down = r.bottom - 3 + h <= lo || lo - r.bottom >= r.top - hi;
    m.style.top = Math.round(down ? r.bottom - 3 : r.top + 3 - h) + 'px';
  }
  function placeShownMore() { Array.prototype.forEach.call(document.querySelectorAll('.irow:hover, .irow:focus-within, .irow.sel'), placeMore); }
  var lastPointer = 'mouse', lastInput = 'pointer'; // the focused row is restored after a re-render only for keyboard use

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
    // (382) dragging a handle sets the drawer height; a drag's trailing click is swallowed
    $('drhandles').addEventListener('pointerdown', drDragStart);
    $('drhandles').addEventListener('pointermove', drDragMove);
    $('drhandles').addEventListener('pointerup', drDragEnd);
    $('drhandles').addEventListener('pointercancel', drDragEnd);
    $('drhandles').addEventListener('click', function (e) { if (drDragged) e.stopPropagation(); }, true);

    setTheme(themeOf());
    document.addEventListener('click', function (e) {
      var t = e.target, h = t.closest('[data-handle]');
      if (t === $('iwin')) { closeItemWindow(); return; } // the backdrop
      if (t.closest('#qs')) { if (qs.open) qClose(); else qOpen(); return; }
      var qo = t.closest('[data-qi]');
      if (qo) { qPick(Number(qo.getAttribute('data-qi'))); return; }
      if (qs.open && !t.closest('#qpop')) qClose(true);
      if (t.closest('#gear')) { gearMenu($('gmenu').hidden); return; }
      var th = t.closest('[data-theme-set]');
      if (th) { setTheme(th.getAttribute('data-theme-set')); return; }
      if (!$('gmenu').hidden && !t.closest('#gmenu')) gearMenu(false);
      if (h) { if (h.getAttribute('data-handle') === 'panels') toast('The side panels handle is a stub in this mockup.'); else openStore(h.getAttribute('data-handle')); return; }
      var os = t.closest('[data-open-store]');
      if (os) { if (ui.open !== os.getAttribute('data-open-store')) openStore(os.getAttribute('data-open-store')); return; }
      var a = t.closest('[data-act]');
      if (a) {
        e.preventDefault(); e.stopPropagation();
        var k = a.getAttribute('data-act'), d = { item: a.getAttribute('data-item'), who: a.getAttribute('data-who'), op: a.getAttribute('data-op'), cid: a.getAttribute('data-cid') };
        if (k === 'apply') { if (applyRaw()) { render(); toast('Raw data applied.'); } }
        else if (k === 'reset') { resetStore(ui.store); ui.last = null; render(); toast('Store reset to fixtures.'); }
        else if (k === 'win-close') closeItemWindow();
        else if (k === 'page-back') leavePage();
        else if (k === 'win-discuss') { if ($('cbox')) $('cbox').focus(); }
        else if (k === 'tab') { tabState()[ui.layout === 'narrow' ? 'narrow' : 'side'] = a.getAttribute('data-tab'); ui.sel = null; render(); }
        else if (k === 'today') {
          var ps = panelState(), set = a.getAttribute('data-set');
          ps.today = ps.today === set ? null : set;
          if (ps.today && !scopeHasToday(set, scopeOf(), panelMode(scopeOf()), memberOf(viewerOf(ui.store)))) ps.scope = TODAY_HOME[set];
          if (ui.layout === 'narrow') tabState().narrow = 'items';
          ui.sel = null; render();
        }
        else if (k === 'dlg-cancel') closeDialog();
        else if (k === 'open') showItem(d.item);
        else if (k === 'scope') { panelState().scope = a.getAttribute('data-scope'); ui.sel = null; render(); }
        else if (k === 'pmode') { panelState().modes[scopeOf()] = a.getAttribute('data-mode'); render(); }
        else if (k === 'closed-toggle') { panelState().closedOpen = !panelState().closedOpen; render(); }
        else if (k === 'tree-closed') { var to = panelState().treeOpen, pid = a.getAttribute('data-parent'); to[pid] = !to[pid]; render(); }
        else if (k === 'psearch-clear') { panelState().q = ''; render(); if ($('psearch')) $('psearch').focus(); }
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
      // (354) touch: a tap selects a row, which shows its actions and details as a hover
      // would; a tap on the selected row (or on its title) opens the item window (366)
      var row = t.closest('.irow');
      if (row && lastPointer === 'touch' && !t.closest('a.title') && ui.sel !== row.getAttribute('data-rk')) {
        e.preventDefault(); ui.sel = row.getAttribute('data-rk'); render(); return;
      }
      var nk = t.closest('[data-nkey]');
      if (nk) readMap()[nk.getAttribute('data-nkey')] = true; // a feed entry is read once clicked
      var it = t.closest('[data-item]');
      if (it && !t.closest('select, textarea, input, #dlg')) { e.preventDefault(); showItem(it.getAttribute('data-item'), t, nk && nk.getAttribute('data-nev')); return; }
      if (t.closest('a[href="#"]')) e.preventDefault();
    });
    document.addEventListener('submit', function (e) {
      var f = e.target;
      if (f.closest && f.closest('#dlg')) { e.preventDefault(); submit(f); }
    });
    // (385) the search box filters as you type; the render rebuilds the panel, so focus and the caret are put back (see render),
    // and an IME composition (Korean, Japanese) is left alone until it commits
    function searchInput(e) {
      if (e.target.id !== 'psearch' || e.isComposing) return;
      panelState().q = e.target.value; render();
    }
    document.addEventListener('input', searchInput);
    document.addEventListener('compositionend', searchInput);
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
      else if (t.closest && t.closest('#dlg form')) {
        var df = t.closest('form');
        if (t.name === 'saved') { // (393) picking a saved profile fills the fields
          var o = t.options[t.selectedIndex], sp = o && o.getAttribute('data-p');
          if (sp) { var pr = JSON.parse(sp); df.elements.org.value = pr.org; df.elements.family.value = pr.family; df.elements.given.value = pr.given; df.elements.nid.value = asciiId(pr.given); }
        }
        updateHint(df); profileView(df);
      }
    });
    document.addEventListener('input', function (e) {
      var t = e.target, df = t.closest && t.closest('#dlg form');
      if (!df || !df.elements.given) return;
      if (df.elements.saved && t.name !== 'saved' && t.name !== 'nid' && t.name !== 'lead') df.elements.saved.value = '';
      profileView(df);
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
      if ($('drbody').contains(t)) fitDrawer(); // an opened details block changes the content height
    }, true);
    document.addEventListener('pointerdown', function (e) { lastPointer = e.pointerType || 'mouse'; lastInput = 'pointer'; }, true);
    document.addEventListener('mouseover', function (e) {
      var tt = e.target.closest && e.target.closest('[data-tip]');
      if (tt && tt !== tipFor) showTip(tt); else if (!tt && tipFor) hideTip();
      var row = e.target.closest && e.target.closest('.irow');
      if (row && !row.contains(e.relatedTarget)) placeMore(row);
    });
    document.addEventListener('focusin', function (e) {
      var row = e.target.closest && e.target.closest('.irow');
      if (row) placeMore(row);
      if (e.target.matches && e.target.matches('button[data-tip]')) showTip(e.target); else if (tipFor) hideTip();
    });
    document.addEventListener('scroll', function () { if (tipFor) hideTip(); placeShownMore(); }, true);
    document.addEventListener('mouseout', function (e) { if (!e.relatedTarget && tipFor) hideTip(); });
    $('qin').addEventListener('input', function () { qs.idx = 0; qRender(); });
    $('qin').addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); var n = qs.items.length; if (n) { qs.idx = (qs.idx + (e.key === 'ArrowDown' ? 1 : n - 1)) % n; qRender(); } }
      else if (e.key === 'Enter') { e.preventDefault(); qPick(qs.idx); }
      else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); qClose(); }
    });
    $('qpop').addEventListener('mousemove', function (e) {
      var o = e.target.closest('[data-qi]'), i = o && Number(o.getAttribute('data-qi'));
      if (o && i !== qs.idx) { qs.idx = i; Array.prototype.forEach.call($('qlist').querySelectorAll('.qopt'), function (x) { var on = Number(x.getAttribute('data-qi')) === i; x.classList.toggle('on', on); x.setAttribute('aria-selected', String(on)); }); $('qin').setAttribute('aria-activedescendant', 'qopt-' + i); }
    });
    document.addEventListener('keydown', function (e) {
      lastInput = 'key';
      // (385) Esc in the search box clears it first; with it empty, Esc goes on to close the window or the drawer
      if (e.key === 'Escape' && e.target.id === 'psearch' && e.target.value) { e.preventDefault(); panelState().q = ''; render(); if ($('psearch')) $('psearch').focus(); return; }
      // (354) keyboard: a focused row shows its actions and details; Enter opens its item window
      if (e.key === 'Enter' && e.target.classList && e.target.classList.contains('irow')) { e.preventDefault(); showItem(e.target.getAttribute('data-item')); return; }
      if (e.key !== 'Escape' || $('dlg').open) return;
      if (!$('gmenu').hidden) { gearMenu(false); return; }
      if (ui.win) closeItemWindow(); else if (ui.open) { var n = ui.open; ui.open = null; syncNow(n, true); render(); }
    });
    // (366) the layout follows the width: re-render when it crosses a breakpoint, else refit
    window.addEventListener('resize', function () { if (layoutOf() !== ui.layout) render(); else { if (!ui.page) fitDrawer(); measure(); } });
    // (371) the store page route: reload on #/<storeId> restores the page; back and forward follow the hash
    window.addEventListener('hashchange', applyRoute);
    window.addEventListener('popstate', function () { if (routeStore() !== ui.page) applyRoute(); });
    applyRoute();
  }

  window.addEventListener('error', function (e) { var t = $('toast'); if (t) { t.hidden = false; t.textContent = 'script error: ' + e.message; } });
  init();
  // read-only hook for scripted checks (headless verification); not part of the model
  window.__tracker = { ui: ui, vs: function () { return VS; }, openItemWindow: openItemWindow };
  // (366) the entry point a later URL route uses to open an item from a link
  window.openItemWindow = openItemWindow;
})();
