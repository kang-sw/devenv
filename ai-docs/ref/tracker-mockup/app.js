// Tracker mockup v1: the view. A dashboard-like frame (header, conversation
// placeholder, pull-down drawer with one handle per store) around the tracker
// cards, an item side sheet, and small action dialogs. All state is computed in
// the browser by TrackerEngine (engine.js) from fixtures.js; no build, no fetch.
(function () {
  'use strict';

  var E = window.TrackerEngine;
  var replay = E.replay, reach = E.reach, itemList = E.itemList, assignsOf = E.assignsOf, subtasksOf = E.subtasksOf;
  var progItem = E.progItem, progAssign = E.progAssign, itemStatus = E.itemStatus, chain = E.chain, childItems = E.childItems;

  var stores = {}, order = [], gs = {};
  var ui = {
    store: null,          // the store the header control and the sheet refer to
    open: null,           // the store whose drawer is open, or null
    viewers: {},          // store -> member id the page is viewed as
    seen: {},             // "store|viewer" -> { assignment key: true } already shown to that viewer
    fresh: {},            // assignment keys that were new when the drawer last opened
    item: null, sheet: false, last: null,
    flaggedOnly: false, kind: 'all', status: 'all', dataOpen: false, openLogs: {}
  };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }
  function $(id) { return document.getElementById(id); }
  function clone(x) { return JSON.parse(JSON.stringify(x)); }

  // ------------------------------------------------------------- small views

  function nm(g, id) { return g.members[id] ? g.members[id].name : id; }
  function fmtTs(ts) { return String(ts).slice(5, 16).replace('T', ' '); }
  function itemRef(id) { return '<a href="#" data-item="' + esc(id) + '">' + esc(id) + '</a>'; }
  function bar(v) {
    if (v == null) return '<span class="muted">-</span>';
    var r = Math.round(v);
    return '<span class="bar' + (r >= 100 ? ' full' : '') + '"><i style="width:' + r + '%"></i></span> <span class="pct">' + r + '%</span>';
  }
  var STATE_LABEL = { open: 'open', awaiting_confirm: 'awaiting confirm', 'awaiting confirm': 'awaiting confirm', done: 'done' };
  var STATE_CLS = { open: 'open', awaiting_confirm: 'awaiting', 'awaiting confirm': 'awaiting', done: 'done' };
  function stateChip(s) {
    if (!STATE_LABEL[s]) return '';
    return '<span class="chip ' + STATE_CLS[s] + '">' + STATE_LABEL[s] + '</span>';
  }
  function kindChip(k) { return '<span class="chip kind">' + esc(k) + '</span>'; }
  function viaChips(ev) {
    return ev.via === 'agent' ? '<span class="chip agent">agent</span>' + (ev.auto ? '<span class="chip auto">auto</span>' : '') : '<span class="chip">human</span>';
  }
  function complText(c) { return c === 'assigner' ? 'assigner confirms' : 'assignee closes'; }
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
    function walk(parent) {
      itemList(g).forEach(function (it) {
        if ((it.parent && g.items[it.parent] ? it.parent : null) !== parent) return;
        out.push(it); walk(it.id);
      });
    }
    walk(null);
    return out;
  }

  function describe(g, r, inTimeline) {
    var e = r.ev, p = e.payload || {}, ref = function (id) { return inTimeline ? '' : ' ' + itemRef(id); };
    switch (e.type) {
      case 'item.create':
        return 'created ' + esc(p.kind) + ' <b>' + esc(p.title) + '</b>' + (inTimeline ? '' : ' (' + itemRef(r.id) + ')') +
          (p.parent ? ' under ' + itemRef(p.parent) : '');
      case 'item.assign':
        return 'assigned' + ref(p.item) + ' to ' + (p.mode === 'all' ? 'everyone within L' + esc(p.levels) : esc((p.people || []).join(', ')) + ' (within L' + esc(p.levels) + ')') +
          ', ' + complText(p.completion);
      case 'progress.report':
        return 'reported ' + esc(p.pct) + '%' + ref(p.item) + (p.note ? ': ' + esc(p.note) : '');
      case 'item.close': return 'closed' + ref(p.item);
      case 'item.confirm': return 'confirmed ' + esc(p.assignee) + "'s work" + ref(p.item);
      case 'roles.add_member': return 'added member <b>' + esc((p.member || {}).id) + '</b>';
      case 'roles.set_lead': return 'set lead of <b>' + esc(p.member) + '</b> to ' + esc((p.leads || []).join(', ') || 'nobody');
      default: return esc(e.type);
    }
  }
  function evLine(g, r, inTimeline) {
    var e = r.ev;
    return '<li class="ev' + (r.status === 'flagged' ? ' flagged' : '') + (e.via === 'agent' ? ' agent' : '') + '">' +
      '<span class="when">' + esc(fmtTs(e.ts)) + '</span><b>' + esc(nm(g, e.author)) + '</b> ' + viaChips(e) +
      ' ' + describe(g, r, inTimeline) + (r.status === 'flagged' ? ' <span class="chip flag">flagged</span><span class="reason">' + esc(r.reason) + '</span>' : '') + '</li>';
  }

  // ------------------------------------------------------- viewer / badge state

  function viewerOf(name) { return ui.viewers[name]; }
  function seenKey(name) { return name + '|' + viewerOf(name); }
  function aKey(a) { return a.item + '>' + a.to; }
  function newAssigns(name) {
    var g = gs[name], v = viewerOf(name), seen = ui.seen[seenKey(name)] || {};
    return g.assigns.filter(function (a) { return a.to === v && a.state === 'open' && !seen[aKey(a)]; });
  }
  // Opening a store (or switching viewer inside an open drawer) shows what is new
  // once, then clears the handle badge, like the mailbox badge.
  function markSeen(name) {
    var seen = (ui.seen[seenKey(name)] = ui.seen[seenKey(name)] || {});
    ui.fresh = {};
    newAssigns(name).forEach(function (a) { ui.fresh[aKey(a)] = true; });
    gs[name].assigns.forEach(function (a) { if (a.to === viewerOf(name)) seen[aKey(a)] = true; });
  }

  // ------------------------------------------------------------------ cards

  function cardMine(g, v) {
    var rank = { open: 0, awaiting_confirm: 1, done: 2 };
    var mine = g.assigns.filter(function (a) { return a.to === v; }).sort(function (a, b) { return rank[a.state] - rank[b.state]; });
    var own = itemList(g).filter(function (i) { return i.author === v && i.kind !== 'note' && !i.closed && !assignsOf(g, i.id).length; });
    var nOpen = mine.filter(function (a) { return a.state === 'open'; }).length;
    var h = '<div class="sub">Assigned to me</div>';
    h += mine.length ? mine.map(function (a) {
      var it = g.items[a.item], ch = chain(g, it);
      var from = 'from <b>' + esc(nm(g, a.by)) + '</b>' + (ch.length > 2 || (ch.length === 2 && ch[1] !== a.by) ? ' (chain ' + esc(ch.slice().reverse().join(' > ')) + ')' : '');
      var next = a.state === 'open' ? 'Next: report progress, then close' :
        a.state === 'awaiting_confirm' ? 'Waiting for ' + esc(nm(g, a.by)) + ' to confirm' : 'Done';
      return '<div class="row click" data-item="' + esc(a.item) + '">' +
        '<div class="r1">' + stateChip(a.state) + (ui.fresh[aKey(a)] ? '<span class="chip new">new</span>' : '') +
        '<a class="title" href="#" data-item="' + esc(a.item) + '">' + esc(it.title) + '</a><span class="grow"></span>' + bar(progAssign(g, a)) + '</div>' +
        '<div class="r2">' + from + ' &middot; L' + a.depth + ' &middot; ' + complText(a.completion) + (it.project ? ' &middot; ' + esc(it.project) : '') + '</div>' +
        '<div class="r3"><span class="next">' + next + '</span>' + (a.state === 'open' ?
          '<button class="btn mini" data-act="report" data-item="' + esc(a.item) + '">Report</button>' +
          '<button class="btn mini" data-act="close" data-item="' + esc(a.item) + '">Close</button>' +
          '<button class="btn mini" data-act="split" data-item="' + esc(a.item) + '">Split</button>' : '') + '</div></div>';
    }).join('') : '<p class="muted">Nothing is assigned to you.</p>';
    h += '<div class="sub">My own items (not assigned)</div>';
    h += own.length ? own.map(function (it) {
      return '<div class="row click" data-item="' + esc(it.id) + '"><div class="r1">' + kindChip(it.kind) +
        '<a class="title" href="#" data-item="' + esc(it.id) + '">' + esc(it.title) + '</a><span class="grow"></span>' + bar(progItem(g, it)) + '</div>' +
        '<div class="r3"><span class="next">Open</span>' +
        '<button class="btn mini" data-act="report" data-item="' + esc(it.id) + '">Report</button>' +
        '<button class="btn mini" data-act="close" data-item="' + esc(it.id) + '">Close</button>' +
        '<button class="btn mini" data-act="assign" data-item="' + esc(it.id) + '">Assign</button></div></div>';
    }).join('') : '<p class="muted">No open items of your own.</p>';
    return card('My assignments', nOpen + ' open', h);
  }

  function cardGiven(g, v) {
    var items = itemList(g).filter(function (i) { return assignsOf(g, i.id).some(function (a) { return a.by === v; }); });
    var h = items.length ? items.map(function (it) {
      var as = assignsOf(g, it.id).filter(function (a) { return a.by === v; });
      return '<div class="row click" data-item="' + esc(it.id) + '"><div class="r1">' + kindChip(it.kind) +
        '<a class="title" href="#" data-item="' + esc(it.id) + '">' + esc(it.title) + '</a><span class="grow"></span>' +
        bar(progItem(g, it)) + stateChip(itemStatus(g, it)) + '</div>' +
        as.map(function (a) {
          return '<div class="assignee-line"><b>' + esc(nm(g, a.to)) + '</b><span class="muted">L' + a.depth + '</span>' + stateChip(a.state) + bar(progAssign(g, a)) +
            '<span class="muted">' + complText(a.completion) + '</span>' +
            (a.state === 'awaiting_confirm' && a.completion === 'assigner' ?
              '<button class="btn mini primary" data-act="confirm" data-key="' + esc(a.item + '|' + a.to) + '">Confirm</button>' : '') + '</div>';
        }).join('') + '</div>';
    }).join('') : '<div class="empty">You have not assigned anything yet. Create an item, then assign it.</div>';
    var waiting = g.assigns.filter(function (a) { return a.by === v && a.state === 'awaiting_confirm'; }).length;
    return card('Assigned by me / team progress', items.length + (items.length === 1 ? ' item' : ' items') + (waiting ? ', ' + waiting + ' to confirm' : ''), h);
  }

  function cardTeam(st, g, v) {
    var probs = E.rolesProblems(st.roles), r = reach(g.members, v, 99), seen = {};
    var lead = (g.members[v].leads || [])[0];
    var levels = {};
    Object.keys(r).forEach(function (k) { levels[r[k]] = (levels[r[k]] || 0) + 1; });
    var summary = 'Admins: ' + esc(Object.keys(g.admins).join(', ')) + '. You: ' + (lead ? 'lead <b>' + esc(lead) + '</b>' : 'no lead (root)') + ', ' +
      (Object.keys(r).length ? 'reach ' + Object.keys(levels).map(function (d) { return levels[d] + ' at L' + d; }).join(', ') : 'no followers') + '.';
    function node(id) {
      if (seen[id]) return '<li>' + esc(id) + ' (cycle guard)</li>';
      seen[id] = true;
      var m = g.members[id], kids = g.children[id] || [];
      var chips = (id === v ? '<span class="chip you">you</span>' : '') + (g.admins[id] ? '<span class="chip admin">admin</span>' : '') +
        (id === lead ? '<span class="chip">your lead</span>' : '') + (r[id] ? '<span class="chip" title="levels below you">L' + r[id] + '</span>' : '');
      return '<li><div class="node' + (id === v ? ' me' : '') + '"><span class="nm">' + esc(m.name) + '</span><span class="nid">' + esc(id) + '</span>' + chips + '</div>' +
        (kids.length ? '<ul>' + kids.map(node).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(g.members).filter(function (id) { var l = g.members[id].leads || []; return !l.length || !g.members[l[0]]; });
    var h = '<p class="r2" style="margin:0 0 6px">' + summary + '</p>';
    if (probs.length) h += '<p class="err">roles file problems: ' + esc(probs.join('; ')) + '</p>';
    h += '<ul class="tree">' + roots.map(node).join('') + '</ul>';
    var lost = Object.keys(g.members).filter(function (id) { return !seen[id]; });
    if (lost.length) h += '<p class="err">unreachable from any root (cycle in the roles file): ' + esc(lost.join(', ')) + '</p>';
    var nMem = Object.keys(g.members).length;
    return card('Team', nMem + (nMem === 1 ? ' member' : ' members'), h);
  }

  function cardItems(g) {
    var rows = itemsInTreeOrder(g).filter(function (it) {
      if (ui.kind !== 'all' && it.kind !== ui.kind) return false;
      var s = itemStatus(g, it);
      if (ui.status === 'open') return s === 'open' || s === 'awaiting confirm';
      if (ui.status === 'done') return s === 'done';
      return true;
    });
    function sel(id, val, list) {
      return '<select id="' + id + '">' + list.map(function (x) { return '<option' + (x === val ? ' selected' : '') + '>' + x + '</option>'; }).join('') + '</select>';
    }
    var h = '<div class="toolbar">kind ' + sel('f-kind', ui.kind, ['all', 'task', 'note', 'todo']) + ' status ' + sel('f-status', ui.status, ['all', 'open', 'done']) +
      '</div>';
    h += rows.length ? rows.map(function (it) {
      var as = assignsOf(g, it.id), d = itemDepth(g, it);
      var who = as.length ? '<span class="muted">&rarr; ' + esc(as.map(function (a) { return a.to; }).join(', ')) + '</span>' : '';
      return '<div class="row click" data-item="' + esc(it.id) + '" style="padding-left:' + d * 14 + 'px"><div class="r1">' + (d ? '<span class="muted">&#8627;</span>' : '') + kindChip(it.kind) +
        '<a class="title" href="#" data-item="' + esc(it.id) + '">' + esc(it.title) + '</a>' + who + '<span class="grow"></span>' +
        (it.kind === 'note' ? '' : bar(progItem(g, it)) + stateChip(itemStatus(g, it))) + '</div>' +
        '<div class="r2">' + esc(it.id) + ' by ' + esc(nm(g, it.author)) + (it.project ? ' &middot; ' + esc(it.project) : '') + '</div></div>';
    }).join('') : '<div class="empty">No items match.</div>';
    return card('Items', rows.length + ' of ' + Object.keys(g.items).length, h);
  }

  function cardActivity(g) {
    var rows = g.log.filter(function (r) { return !ui.flaggedOnly || r.status === 'flagged'; }), cap = 40;
    var shown = rows.slice(-cap).reverse();
    var flagged = g.log.filter(function (r) { return r.status === 'flagged'; }).length;
    var h = '';
    if (ui.last) {
      h += '<div class="lastact">Last action: ' + ui.last.map(function (r) {
        return esc(r.ev.type) + ' ' + (r.status === 'flagged' ? '<span class="chip flag">flagged</span> ' + esc(r.reason) : '<span class="chip done">applied</span>');
      }).join('; ') + '</div>';
    }
    h += '<div class="toolbar"><label><input type="checkbox" id="flagged-only"' + (ui.flaggedOnly ? ' checked' : '') + '> flagged only</label>' +
      '<span class="grow"></span><span class="muted">newest first, ' + shown.length + ' of ' + rows.length + '</span></div>';
    h += shown.length ? '<ul style="list-style:none;margin:0;padding:0">' + shown.map(function (r) { return evLine(g, r, false); }).join('') + '</ul>' : '<div class="empty">Nothing to show.</div>';
    return card('Activity', g.log.length + ' events, ' + flagged + ' flagged', h);
  }

  function fmtLog(a) { return '[\n' + a.map(function (e) { return '  ' + JSON.stringify(e); }).join(',\n') + '\n]'; }
  function fmtRoles(r) {
    return '{\n  "admins": ' + JSON.stringify(r.admins || []) + ',\n  "members": [\n' +
      r.members.map(function (m) { return '    ' + JSON.stringify(m); }).join(',\n') + '\n  ]\n}';
  }
  function cardData(st) {
    var owners = st.roles.members.map(function (m) { return m.id; });
    Object.keys(st.logs).forEach(function (k) { if (owners.indexOf(k) < 0) owners.push(k); });
    var h = '<details class="data" id="data-details"' + (ui.dataOpen ? ' open' : '') + '><summary>data (raw JSON, editable)</summary>' +
      '<p class="r2">The model being settled: roles file plus one event log per member. Edit, then apply; edits live in memory until reload.</p>' +
      '<div class="toolbar"><button class="btn mini primary" data-act="apply">apply</button><button class="btn mini" data-act="reset">reset store to fixtures</button></div>' +
      '<div class="sub">roles file</div><textarea id="raw-roles" rows="12" spellcheck="false">' + esc(fmtRoles(st.roles)) + '</textarea>';
    owners.forEach(function (o) {
      var log = st.logs[o] || [];
      h += '<details data-owner="' + esc(o) + '"' + (ui.openLogs[o] ? ' open' : '') + '><summary>log of ' + esc(o) + ' (' + log.length + ' events)</summary>' +
        '<textarea data-owner="' + esc(o) + '" rows="' + Math.min(18, log.length + 3) + '" spellcheck="false">' + esc(fmtLog(log)) + '</textarea></details>';
    });
    return card('Data', 'debug', h + '</details>');
  }

  function drawerHtml(st, g, v) {
    var flagged = g.log.filter(function (r) { return r.status === 'flagged'; }).length;
    return '<div class="dr-title"><b>' + esc(st.def.label) + '</b><span>' + Object.keys(g.members).length + ' members, ' + g.log.length + ' events merged from ' +
      Object.keys(st.logs).length + ' logs, ' + flagged + ' flagged</span><span class="grow"></span>' +
      '<button class="btn primary" data-act="create"><svg class="i"><use href="#i-plus"/></svg>New item</button></div>' +
      '<div class="dr-cols"><div class="dr-col">' + cardMine(g, v) + cardGiven(g, v) + '</div>' +
      '<div class="dr-col">' + cardItems(g) + '</div>' +
      '<div class="dr-col">' + cardTeam(st, g, v) + cardActivity(g) + cardData(st) + '</div></div>';
  }

  // ------------------------------------------------------------ item sheet

  function sheetHtml(g, v, it) {
    var as = assignsOf(g, it.id), mineA = as.filter(function (a) { return a.to === v; })[0];
    var ownOpen = it.author === v && !as.length && !it.closed && it.kind !== 'note';
    var acts = '';
    if (mineA && mineA.state === 'open') {
      acts += '<button class="btn" data-act="report" data-item="' + esc(it.id) + '">Report progress</button>' +
        '<button class="btn" data-act="close" data-item="' + esc(it.id) + '">Close</button>' +
        '<button class="btn" data-act="split" data-item="' + esc(it.id) + '">Split into subtasks</button>';
    }
    if (ownOpen) acts += '<button class="btn" data-act="report" data-item="' + esc(it.id) + '">Report progress</button><button class="btn" data-act="close" data-item="' + esc(it.id) + '">Close</button>';
    if (it.author === v && it.kind !== 'note') acts += '<button class="btn" data-act="assign" data-item="' + esc(it.id) + '">Assign</button>';
    as.forEach(function (a) {
      if (a.by === v && a.state === 'awaiting_confirm' && a.completion === 'assigner')
        acts += '<button class="btn primary" data-act="confirm" data-key="' + esc(a.item + '|' + a.to) + '">Confirm ' + esc(nm(g, a.to)) + '</button>';
    });
    var h = '<div class="sh-head"><div>' + kindChip(it.kind) + '</div><h2>' + esc(it.title) + '</h2><button class="btn mini" data-act="sheet-close" title="close (Esc)"><svg class="i"><use href="#i-x"/></svg></button></div>';
    h += '<dl class="sh-meta"><dt>id</dt><dd><code>' + esc(it.id) + '</code></dd>' +
      '<dt>status</dt><dd>' + (it.kind === 'note' ? '<span class="muted">note (no progress)</span>' : stateChip(itemStatus(g, it)) + ' ' + bar(progItem(g, it))) + '</dd>' +
      '<dt>creator</dt><dd>' + esc(nm(g, it.author)) + (g.admins[it.author] ? ' <span class="chip admin">admin</span>' : '') + '</dd>' +
      '<dt>project</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>parent</dt><dd>' + (it.parent ? itemRef(it.parent) + ' ' + esc((g.items[it.parent] || {}).title || '') : '<span class="muted">-</span>') + '</dd>' +
      '<dt>created</dt><dd>' + esc(it.ts) + '</dd></dl>';
    h += it.body ? '<div class="sh-body">' + esc(it.body) + '</div>' : '';
    h += '<div class="sh-actions">' + (acts || '<span class="muted">No actions available to ' + esc(nm(g, v)) + ' on this item.</span>') + '</div>';

    h += '<h3>Assignees</h3>';
    if (!as.length) {
      h += '<div class="empty">Not assigned' + (it.kind !== 'note' ? '. Own progress ' + (it.pct == null ? '0' : Math.round(it.pct)) + '%' + (it.closed ? ', closed' : '') : '') + '.</div>';
    } else {
      h += '<table class="atable"><tr><th>assignee</th><th>state</th><th>completion</th><th>progress</th></tr>' + as.map(function (a) {
        var ks = childItems(g, it.id, a.to);
        return '<tr><td><b>' + esc(nm(g, a.to)) + '</b><br><span class="muted">by ' + esc(a.by) + ', L' + a.depth + '</span></td><td>' + stateChip(a.state) + '</td><td>' + complText(a.completion) + '</td><td>' +
          bar(progAssign(g, a)) + '<br><span class="muted">' + (a.pct == null ? 'no report' : 'reported ' + Math.round(a.pct) + '%') + (a.note ? ': ' + esc(a.note) : '') +
          (ks.length && a.state !== 'done' ? ' (ignored, ' + ks.length + ' subtask' + (ks.length > 1 ? 's' : '') + ' drive the roll-up)' : '') + '</span></td></tr>';
      }).join('') + '</table>';
    }

    h += '<h3>Subtasks (roll-up)</h3>';
    function sub(parent) {
      var ks = subtasksOf(g, parent);
      return ks.length ? '<ul>' + ks.map(function (k) {
        return '<li><div>' + kindChip(k.kind) + itemRef(k.id) + ' ' + esc(k.title) + ' <span class="muted">' + esc(k.author) + (assignsOf(g, k.id).length ? ' &rarr; ' + esc(assignsOf(g, k.id).map(function (a) { return a.to; }).join(', ')) : '') + '</span><span class="grow"></span>' +
          bar(progItem(g, k)) + stateChip(itemStatus(g, k)) + '</div>' + sub(k.id) + '</li>';
      }).join('') + '</ul>' : '';
    }
    var tree = sub(it.id);
    h += tree ? '<div class="stree">' + tree + '</div>' : '<div class="empty">No subtasks.</div>';

    var evs = it.events.concat(it.flagged);
    evs.sort(function (a, b) { return g.log.indexOf(a) - g.log.indexOf(b); });
    h += '<h3>History (' + evs.length + ' events)</h3><ul class="timeline">' + evs.map(function (r) { return evLine(g, r, true); }).join('') + '</ul>';
    return h;
  }

  // --------------------------------------------------------- dialog forms

  function curr() { var st = stores[ui.store]; return { st: st, g: replay(st), v: viewerOf(ui.store) }; }
  function optsHtml(list, empty, sel) {
    if (!list.length) return '<option value="">(' + empty + ')</option>';
    return list.map(function (x) {
      var on = sel && (x[0] === sel || x[0].split('|')[0] === sel);
      return '<option value="' + esc(x[0]) + '"' + (on ? ' selected' : '') + '>' + esc(x[1]) + '</option>';
    }).join('');
  }
  function peopleHtml(g, v) {
    var r = reach(g.members, v, 99);
    var ids = Object.keys(r).sort(function (a, b) { return r[a] - r[b] || (a < b ? -1 : 1); });
    return ids.length ? ids.map(function (id) {
      return '<label><input type="checkbox" name="people" value="' + esc(id) + '" data-level="' + r[id] + '"> ' + esc(nm(g, id)) + ' <span class="muted">L' + r[id] + '</span></label>';
    }).join('') : '<span class="muted">(you have no followers; the event would be flagged)</span>';
  }
  function levelSel() { return '<select name="levels"><option>1</option><option>2</option><option>3</option></select>'; }
  function complRadios() {
    return '<div class="opts"><label><input type="radio" name="completion" value="assignee" checked> assignee closes</label>' +
      '<label><input type="radio" name="completion" value="assigner"> assigner confirms</label></div>';
  }
  function fld(label, inner) { return '<div class="fld"><span>' + label + '</span>' + inner + '</div>'; }

  function dialogHtml(kind, sel) {
    var c = curr(), g = c.g, v = c.v, items = itemList(g);
    var mineCreated = items.filter(function (i) { return i.author === v && i.kind !== 'note'; }).map(function (i) { return [i.id, i.id + ' ' + i.title]; });
    var received = g.assigns.filter(function (a) { return a.to === v && a.state === 'open'; }).map(function (a) { return [a.item, a.item + ' ' + g.items[a.item].title]; });
    var own = items.filter(function (i) { return i.author === v && i.kind !== 'note' && !i.closed && !assignsOf(g, i.id).length; }).map(function (i) { return [i.id, i.id + ' ' + i.title + ' (own)']; });
    var confirm = g.assigns.filter(function (a) { return a.by === v && a.completion === 'assigner' && a.state === 'awaiting_confirm'; })
      .map(function (a) { return [a.item + '|' + a.to, a.item + ' ' + g.items[a.item].title + ' by ' + nm(g, a.to)]; });
    var agent = '<label class="fld inline"><input type="checkbox" name="agent"> record as agent (auto, unprompted)</label>';
    var T = {
      create: ['New item', 'Appends item.create to ' + nm(g, v) + "'s log.",
        fld('kind', '<select name="kind"><option>task</option><option>note</option><option>todo</option></select>') +
        fld('title', '<input type="text" name="title" required>') + fld('body', '<textarea name="body" rows="3"></textarea>') +
        fld('project reference (free text, optional)', '<input type="text" name="project">'), 'Create'],
      assign: ['Assign', 'Appends item.assign. Only the item creator assigns; targets must be within N levels below you.',
        fld('item', '<select name="target">' + optsHtml(mineCreated, 'no items created by you', sel) + '</select>') +
        fld('who', '<div class="opts"><label><input type="radio" name="mode" value="all" checked> everyone within N levels</label><label><input type="radio" name="mode" value="chosen"> chosen people within N levels</label></div>') +
        fld('N levels below me', levelSel()) + fld('completion', complRadios()) +
        fld('chosen people', '<div class="people" id="people">' + peopleHtml(g, v) + '</div>') + '<div class="hint" id="hint"></div>', 'Assign'],
      split: ['Split into subtasks', 'Appends item.create (parent = the received item) and item.assign for your followers; progress rolls up to the parent.',
        fld('received item', '<select name="target">' + optsHtml(received, 'nothing assigned to you', sel) + '</select>') +
        fld('subtask title', '<input type="text" name="title" required>') + fld('N levels below me', levelSel()) + fld('completion', complRadios()) +
        fld('assign to', '<div class="people" id="people">' + peopleHtml(g, v) + '</div>') + '<div class="hint" id="hint"></div>', 'Split'],
      report: ['Report progress', 'Appends progress.report.',
        fld('item', '<select name="target">' + optsHtml(received.concat(own), 'nothing to report on', sel) + '</select>') +
        fld('progress', '<div class="fld inline"><input type="range" name="pct" min="0" max="100" value="50" style="flex:1"><output id="pct-out">50%</output></div>') +
        fld('note (optional)', '<input type="text" name="note">') + agent, 'Report'],
      close: ['Close', 'Appends item.close. With "assigner confirms" this only moves the assignment to awaiting confirmation.',
        fld('item', '<select name="target">' + optsHtml(received.concat(own), 'nothing to close', sel) + '</select>') + agent, 'Close'],
      confirm: ['Confirm', 'Appends item.confirm; the assignment becomes done.',
        fld('awaiting confirmation', '<select name="sel">' + optsHtml(confirm, 'nothing awaiting your confirmation', sel) + '</select>'), 'Confirm']
    }[kind];
    return '<form data-kind="' + kind + '"><h2>' + T[0] + '</h2><p>' + T[1] + '</p>' + T[2] +
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
      msg = ids.length ? 'Assigns to ' + ids.length + ' people within L' + n + ': ' + ids.map(function (i) { return nm(g, i); }).join(', ') : 'Nobody is within L' + n + ' below you; the event will be flagged.';
      warn = !ids.length;
    } else if (!chosen.length) {
      msg = 'Choose at least one person; an empty selection is flagged.'; warn = true;
    } else {
      var beyond = chosen.filter(function (x) { return !(x in r); });
      msg = 'Assigns to ' + chosen.map(function (i) { return nm(g, i); }).join(', ') + '.';
      if (beyond.length) { msg += ' ' + beyond.join(', ') + ' is beyond L' + n + '; the event will be flagged.'; warn = true; }
    }
    hint.textContent = msg; hint.classList.toggle('warn', warn);
  }

  function openDialog(kind, sel) {
    var d = $('dlg');
    d.innerHTML = dialogHtml(kind, sel);
    var f = d.querySelector('form');
    updateHint(f);
    if (!d.open) d.showModal();
    var first = f.querySelector('input[type="text"], select');
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
    var roles, logs = {};
    try { roles = JSON.parse(ta.value); } catch (e) { toast('JSON error in roles file: ' + esc(e.message)); return false; }
    if (!roles || !Array.isArray(roles.members)) { toast('roles file needs a "members" list'); return false; }
    var tas = document.querySelectorAll('textarea[data-owner]');
    for (var i = 0; i < tas.length; i++) {
      var o = tas[i].getAttribute('data-owner'), a;
      try { a = JSON.parse(tas[i].value); } catch (e2) { toast('JSON error in log of ' + esc(o) + ': ' + esc(e2.message)); return false; }
      if (!Array.isArray(a)) { toast('log of ' + esc(o) + ' must be a JSON list'); return false; }
      logs[o] = a;
    }
    st.roles = roles; st.logs = logs;
    return true;
  }

  // Append events for the current viewer from a dialog form. Events are not
  // pre-validated beyond required fields, so a form can deliberately produce a
  // flagged event (for example assign beyond N levels).
  function act(kind, form) {
    if (!applyRaw()) return;
    var st = stores[ui.store], g = replay(st), v = viewerOf(ui.store), evs = [], el = form.elements;
    function val(n) { return el[n] ? el[n].value : ''; }
    function maxTs() {
      var m = '';
      Object.keys(st.logs).forEach(function (k) { st.logs[k].forEach(function (e) { if (String(e.ts) > m) m = String(e.ts); }); });
      return m;
    }
    function mk(type, payload, agent) {
      var log = (st.logs[v] = st.logs[v] || []);
      var seq = log.reduce(function (s, e) { return Math.max(s, e.seq || 0); }, 0) + 1;
      var now = new Date().toISOString(), mt = maxTs();
      var ts = now > mt ? now : new Date(Date.parse(mt) + 1).toISOString();
      var e = { author: v, seq: seq, ts: ts, sig: (g.members[v].keys || ['unsigned'])[0], type: type };
      if (agent) { e.via = 'agent'; e.auto = true; }
      e.payload = payload;
      log.push(e); evs.push(e);
      return e;
    }
    function id(e) { return e.author + '#' + e.seq; }
    function people() { return Array.prototype.map.call(form.querySelectorAll('input[name="people"]:checked'), function (e) { return e.value; }); }
    var item;
    switch (kind) {
      case 'create':
        var pl = { kind: val('kind'), title: val('title') };
        if (val('body')) pl.body = val('body');
        if (val('project')) pl.project = val('project');
        item = id(mk('item.create', pl));
        break;
      case 'assign':
        item = val('target'); if (!item) return toast('No item to assign.');
        var ap = { item: item, mode: val('mode'), levels: Number(val('levels')), completion: val('completion') };
        if (ap.mode === 'chosen') ap.people = people();
        mk('item.assign', ap);
        break;
      case 'split':
        var parent = val('target'); if (!parent) return toast('Nothing to split.');
        item = id(mk('item.create', { kind: 'task', title: val('title'), parent: parent }));
        mk('item.assign', { item: item, mode: 'chosen', levels: Number(val('levels')), people: people(), completion: val('completion') });
        break;
      case 'report':
        item = val('target'); if (!item) return toast('Nothing to report on.');
        var rp = { item: item, pct: Number(val('pct')) };
        if (val('note')) rp.note = val('note');
        mk('progress.report', rp, el.agent.checked);
        break;
      case 'close':
        item = val('target'); if (!item) return toast('Nothing to close.');
        mk('item.close', { item: item }, el.agent.checked);
        break;
      case 'confirm':
        var sel = val('sel'); if (!sel) return toast('Nothing to confirm.');
        item = sel.split('|')[0];
        mk('item.confirm', { item: item, assignee: sel.split('|')[1] });
        break;
    }
    var g2 = replay(st);
    ui.last = evs.map(function (e) { return g2.log.filter(function (r) { return r.ev === e; })[0]; });
    if (ui.sheet || kind === 'create' || kind === 'split') { ui.item = item; ui.sheet = true; }
    closeDialog();
    render();
    toast(ui.last.map(function (r) {
      return esc(r.ev.type) + ' appended to ' + esc(v) + ': ' + (r.status === 'flagged' ? '<span class="tf">FLAGGED</span> ' + esc(r.reason) : 'applied');
    }).join('<br>'));
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
      var b = $('handle-' + name), n = newAssigns(name).length, badge = b.querySelector('.dr-badge');
      b.className = 'dr-handle' + (ui.store === name ? ' on' : '') + (ui.open === name ? ' open' : '');
      b.setAttribute('aria-expanded', String(ui.open === name));
      badge.hidden = !n; badge.textContent = n;
      b.title = stores[name].def.label + (n ? ': ' + n + ' new assignment' + (n > 1 ? 's' : '') : '');
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
      return '<option value="' + esc(k) + '"' + (k === v ? ' selected' : '') + '>' + esc(g.members[k].name + ' (' + k + ')') + '</option>';
    }).join('');
    $('vstore').textContent = ui.store;
    renderHandles();
    $('drawer').classList.toggle('open', !!ui.open);

    var body = $('drbody'), sb = $('sheet').firstElementChild, top1 = body.scrollTop, top2 = sb.scrollTop;
    body.innerHTML = drawerHtml(st, g, v);
    body.scrollTop = top1;
    var open = ui.sheet && ui.item;
    $('sheet').classList.toggle('open', !!open);
    $('sheet').setAttribute('aria-hidden', String(!open));
    if (open) { sb.innerHTML = sheetHtml(g, v, g.items[ui.item]); sb.scrollTop = ui.itemChanged ? 0 : top2; }
    ui.itemChanged = false;
    measure();
  }

  function resetStore(name) {
    var d = stores[name].def;
    stores[name].roles = clone(d.roles); stores[name].logs = clone(d.logs);
  }

  function openStore(name) {
    if (ui.open === name) { ui.open = null; render(); return; }
    if (ui.store !== name) { ui.item = null; ui.sheet = false; ui.last = null; ui.itemChanged = true; $('drbody').scrollTop = 0; }
    ui.store = name; ui.open = name;
    markSeen(name);
    render();
  }
  function showItem(id) { ui.item = id; ui.sheet = true; ui.itemChanged = true; render(); }

  function init() {
    window.TRACKER_FIXTURES.stores.forEach(function (s) {
      stores[s.name] = { def: s }; order.push(s.name); resetStore(s.name); ui.viewers[s.name] = s.defaultViewer;
    });
    ui.store = order[0];

    // handles: the panels handle is a static stub; each store gets its own, stacking leftward
    var hh = '<button class="dr-handle stub" data-handle="panels" title="side panels (a stub; not part of this mockup)"><svg class="i"><use href="#i-panels"/></svg><span class="hl">panels</span><svg class="i"><use href="#i-chev"/></svg></button>';
    order.forEach(function (n) {
      hh += '<button class="dr-handle" id="handle-' + esc(n) + '" data-handle="' + esc(n) + '" aria-controls="drbody"><svg class="i"><use href="#i-tasks"/></svg><span class="hl">' + esc(n) +
        '</span><span class="dr-badge" hidden></span><svg class="i"><use href="#i-chev"/></svg></button>';
    });
    $('drhandles').innerHTML = hh;

    document.addEventListener('click', function (e) {
      var t = e.target, h = t.closest('[data-handle]');
      if (h) { if (h.getAttribute('data-handle') === 'panels') toast('The side panels handle is a stub in this mockup.'); else openStore(h.getAttribute('data-handle')); return; }
      var os = t.closest('[data-open-store]');
      if (os) { if (ui.open !== os.getAttribute('data-open-store')) openStore(os.getAttribute('data-open-store')); return; }
      var a = t.closest('[data-act]');
      if (a) {
        var k = a.getAttribute('data-act'), sel = a.getAttribute('data-item') || a.getAttribute('data-key');
        if (k === 'apply') { if (applyRaw()) { render(); toast('Raw data applied.'); } }
        else if (k === 'reset') { resetStore(ui.store); ui.last = null; render(); toast('Store reset to fixtures.'); }
        else if (k === 'sheet-close') { ui.sheet = false; render(); }
        else if (k === 'dlg-cancel') closeDialog();
        else openDialog(k, sel);
        return;
      }
      var it = t.closest('[data-item]');
      if (it) { e.preventDefault(); showItem(it.getAttribute('data-item')); return; }
      if (t.closest('a[href="#"]')) e.preventDefault();
    });
    document.addEventListener('submit', function (e) {
      var f = e.target;
      if (f.closest && f.closest('#dlg')) { e.preventDefault(); act(f.getAttribute('data-kind'), f); }
    });
    document.addEventListener('input', function (e) {
      if (e.target.name === 'pct') $('pct-out').textContent = e.target.value + '%';
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
})();
