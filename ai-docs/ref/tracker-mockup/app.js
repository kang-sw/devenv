// Tracker mockup v0: every view is computed in the browser by replaying the
// merged per-member event logs on top of the roles file. No build, no deps.
(function () {
  'use strict';

  var KINDS = ['task', 'note', 'todo'];
  var stores = {};
  var ui = { store: null, viewer: null, item: null, last: null, flaggedOnly: false, open: {} };

  function clone(x) { return JSON.parse(JSON.stringify(x)); }
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }
  function $(id) { return document.getElementById(id); }
  function mean(a) { return a.reduce(function (s, x) { return s + x; }, 0) / a.length; }

  // ---------------------------------------------------------------- replay

  function Rej(m) { this.reason = m; }
  function rej(m) { throw new Rej(m); }

  function childrenMap(members) {
    var c = {};
    Object.keys(members).forEach(function (id) {
      (members[id].leads || []).forEach(function (l) { (c[l] = c[l] || []).push(id); });
    });
    return c;
  }

  // member id -> depth (1 = direct follower) for everyone within `levels` below `from`
  function reach(members, from, levels) {
    var c = childrenMap(members), out = {}, frontier = [from];
    for (var d = 1; d <= levels; d++) {
      var next = [];
      frontier.forEach(function (f) {
        (c[f] || []).forEach(function (k) { if (!(k in out) && k !== from) { out[k] = d; next.push(k); } });
      });
      frontier = next;
    }
    return out;
  }

  function wouldCycle(members, id, lead) {
    var cur = lead, n = 0;
    while (cur && n++ <= Object.keys(members).length + 1) {
      if (cur === id) return true;
      cur = members[cur] && (members[cur].leads || [])[0];
    }
    return false;
  }

  function replay(st) {
    var members = {};
    st.roles.members.forEach(function (m) { members[m.id] = clone(m); });
    var admins = {};
    (st.roles.admins || []).forEach(function (a) { admins[a] = true; });
    var items = {}, assigns = [], log = [], queue = [];

    Object.keys(st.logs).forEach(function (owner) {
      var last = 0;
      st.logs[owner].forEach(function (ev) {
        var seqIssue = ev.seq === last + 1 ? null : 'seq ' + ev.seq + ' after ' + last + ' (expected ' + (last + 1) + ')';
        if (typeof ev.seq === 'number' && ev.seq > last) last = ev.seq;
        queue.push({ ev: ev, owner: owner, seqIssue: seqIssue, id: ev.author + '#' + ev.seq });
      });
    });
    // Merge order: timestamp, then author id, then per-author seq.
    queue.sort(function (a, b) {
      var x = String(a.ev.ts), y = String(b.ev.ts);
      if (x !== y) return x < y ? -1 : 1;
      if (a.ev.author !== b.ev.author) return a.ev.author < b.ev.author ? -1 : 1;
      return (a.ev.seq || 0) - (b.ev.seq || 0);
    });

    function assignOf(item, who) {
      for (var i = 0; i < assigns.length; i++) if (assigns[i].item === item && assigns[i].to === who) return assigns[i];
      return null;
    }
    function hasAssigns(item) { return assigns.some(function (a) { return a.item === item; }); }
    function needItem(p) { var it = items[p.item]; if (!it) rej('unknown item ' + p.item); return it; }
    function needAdmin(ev) { if (!admins[ev.author]) rej('relation edits are admin-only (' + ev.author + ' is not an admin)'); }
    function checkLeads(id, leads) {
      if (!Array.isArray(leads)) rej('leads must be a list');
      if (leads.length > 1) rej('leads must have length 1 (got ' + leads.length + ')');
      leads.forEach(function (l) {
        if (!members[l]) rej('lead "' + l + '" is not a member');
        if (l === id || wouldCycle(members, id, l)) rej('cycle: ' + l + ' is already below ' + id);
      });
    }

    queue.forEach(function (q) {
      var ev = q.ev, p = ev.payload || {}, rec = { id: q.id, ev: ev, status: 'applied', reason: null, itemId: p.item };
      try {
        if (ev.author !== q.owner) rej('author "' + ev.author + '" does not match the ref owner "' + q.owner + '"');
        if (!ev.sig) rej('unsigned');
        var m = members[ev.author];
        if (!m) rej('author "' + ev.author + '" is not in the roles file');
        if ((m.keys || []).indexOf(ev.sig) < 0) rej('signature key "' + ev.sig + '" is not a key of ' + ev.author);
        if (q.seqIssue) rej(q.seqIssue);
        if (ev.auto && (ev.via !== 'agent' || ['progress.report', 'item.close'].indexOf(ev.type) < 0))
          rej('auto recording needs via "agent" and is limited to progress.report and item.close');

        switch (ev.type) {
          case 'item.create':
            if (!p.title) rej('title required');
            if (KINDS.indexOf(p.kind) < 0) rej('kind must be task, note or todo');
            if (p.parent) {
              var par = items[p.parent];
              if (!par) rej('parent ' + p.parent + ' does not exist');
              if (!assignOf(p.parent, ev.author) && par.author !== ev.author) rej('parent is neither assigned to nor created by the author');
            }
            items[q.id] = { id: q.id, kind: p.kind, title: p.title, body: p.body || '', project: p.project || '', parent: p.parent || null,
              author: ev.author, ts: ev.ts, closed: null, pct: null, note: null, events: [], flagged: [] };
            rec.itemId = q.id;
            break;

          case 'item.assign': {
            var it = needItem(p);
            if (it.author !== ev.author) rej('only the item creator assigns it');
            if (it.kind === 'note') rej('notes are not assignable');
            var levels = Number(p.levels);
            if (!(levels >= 1)) rej('levels must be >= 1');
            var completion = p.completion || 'assignee';
            if (completion !== 'assignee' && completion !== 'assigner') rej('completion must be assignee or assigner');
            var r = reach(members, ev.author, levels), all = reach(members, ev.author, 99), targets;
            if (p.mode === 'all') targets = Object.keys(r);
            else if (p.mode === 'chosen') {
              targets = p.people || [];
              targets.forEach(function (x) {
                if (!(x in r)) rej('assignment outside reach: ' + x + (x in all ? ' is ' + all[x] + ' levels below ' + ev.author + ', limit ' + levels : ' is not below ' + ev.author));
              });
            } else rej('mode must be all or chosen');
            targets = targets.filter(function (t) { return !assignOf(p.item, t); });
            if (!targets.length) rej('no new targets (empty, or already assigned)');
            targets.forEach(function (t) {
              assigns.push({ id: q.id, item: p.item, by: ev.author, to: t, depth: r[t], completion: completion, ts: ev.ts,
                state: 'open', pct: null, note: null, confirmedBy: null });
            });
            break;
          }

          case 'progress.report': {
            var it2 = needItem(p), a = assignOf(p.item, ev.author);
            var pct = Number(p.pct);
            if (!(pct >= 0 && pct <= 100)) rej('pct must be 0..100');
            if (a) {
              if (a.state !== 'open') rej('assignment is already ' + a.state);
              a.pct = pct; a.note = p.note || null;
            } else if (it2.author === ev.author && !hasAssigns(p.item)) {
              if (it2.closed) rej('item is already closed');
              it2.pct = pct; it2.note = p.note || null;
            } else rej(ev.author + ' is not an assignee of ' + p.item);
            break;
          }

          case 'item.close': {
            var it3 = needItem(p), a3 = assignOf(p.item, ev.author);
            if (a3) {
              if (a3.state !== 'open') rej('assignment is already ' + a3.state);
              a3.state = a3.completion === 'assigner' ? 'awaiting_confirm' : 'done';
            } else if (it3.author === ev.author && !hasAssigns(p.item)) {
              if (it3.closed) rej('item is already closed');
              it3.closed = q.id;
            } else rej(ev.author + ' is not an assignee of ' + p.item);
            break;
          }

          case 'item.confirm': {
            needItem(p);
            var a4 = assignOf(p.item, p.assignee);
            if (!a4) rej('no assignment of ' + p.item + ' to ' + p.assignee);
            if (a4.by !== ev.author) rej('only the assigner (' + a4.by + ') confirms');
            if (a4.completion !== 'assigner') rej('this assignment does not require confirmation');
            if (a4.state !== 'awaiting_confirm') rej('assignment is ' + a4.state + ', not awaiting confirmation');
            a4.state = 'done'; a4.confirmedBy = ev.author;
            break;
          }

          case 'roles.add_member': {
            needAdmin(ev);
            var nm = p.member || {};
            if (!nm.id) rej('member.id required');
            if (members[nm.id]) rej('member "' + nm.id + '" already exists');
            if (!nm.keys || !nm.keys.length) rej('member.keys required');
            checkLeads(nm.id, nm.leads || []);
            members[nm.id] = { id: nm.id, name: nm.name || nm.id, keys: nm.keys, leads: nm.leads || [] };
            break;
          }

          case 'roles.set_lead': {
            needAdmin(ev);
            if (!members[p.member]) rej('unknown member "' + p.member + '"');
            checkLeads(p.member, p.leads || []);
            members[p.member].leads = p.leads || [];
            break;
          }

          default: rej('unknown event type "' + ev.type + '"');
        }
      } catch (e) {
        if (!(e instanceof Rej)) throw e;
        rec.status = 'flagged'; rec.reason = e.reason;
      }
      log.push(rec);
      if (rec.itemId && items[rec.itemId]) items[rec.itemId][rec.status === 'applied' ? 'events' : 'flagged'].push(rec);
    });

    return { members: members, admins: admins, items: items, assigns: assigns, log: log, children: childrenMap(members) };
  }

  // Problems in the roles file itself (the base the events replay onto).
  function rolesProblems(roles) {
    var out = [], ids = {};
    roles.members.forEach(function (m) { ids[m.id] = m; });
    (roles.admins || []).forEach(function (a) { if (!ids[a]) out.push('admin "' + a + '" is not a member'); });
    roles.members.forEach(function (m) {
      var l = m.leads || [];
      if (l.length > 1) out.push(m.id + ': leads must have length 1 (got ' + l.length + ')');
      l.forEach(function (x) { if (!ids[x]) out.push(m.id + ': lead "' + x + '" is not a member'); });
      if (!m.keys || !m.keys.length) out.push(m.id + ': no keys');
    });
    return out;
  }

  // ----------------------------------------------------------- derived data

  function childItems(g, parentId, author) {
    return Object.keys(g.items).map(function (k) { return g.items[k]; })
      .filter(function (i) { return i.parent === parentId && i.author === author; });
  }
  function progAssign(g, a) {
    if (a.state === 'done') return 100;
    var vals = childItems(g, a.item, a.to).map(function (k) { return progItem(g, k); }).filter(function (v) { return v != null; });
    if (vals.length) return mean(vals);
    return a.pct == null ? 0 : a.pct;
  }
  function progItem(g, it) {
    var as = g.assigns.filter(function (a) { return a.item === it.id; });
    if (as.length) return mean(as.map(function (a) { return progAssign(g, a); }));
    if (it.kind === 'note') return null;
    return it.closed ? 100 : (it.pct == null ? 0 : it.pct);
  }
  function itemStatus(g, it) {
    var as = g.assigns.filter(function (a) { return a.item === it.id; });
    if (as.length) {
      if (as.every(function (a) { return a.state === 'done'; })) return 'done';
      if (as.some(function (a) { return a.state === 'awaiting_confirm'; })) return 'awaiting confirm';
      return 'open';
    }
    return it.kind === 'note' ? '-' : (it.closed ? 'done' : 'open');
  }
  function chain(g, it) { // authors from the item up to the root item
    var out = [], cur = it, n = 0;
    while (cur && n++ < 50) { out.push(cur.author); cur = cur.parent && g.items[cur.parent]; }
    return out;
  }
  function pctText(v) { return v == null ? '-' : Math.round(v) + '%'; }
  function nm(g, id) { return g.members[id] ? g.members[id].name : id; }
  function link(id) { return '<a href="#" data-item="' + esc(id) + '">' + esc(id) + '</a>'; }
  function who(g, id) { return esc(id) + (g.admins[id] ? ' (admin)' : ''); }

  // ------------------------------------------------------------------ views

  function viewSummary(st, g) {
    var flagged = g.log.filter(function (r) { return r.status === 'flagged'; }).length;
    return '<p>' + Object.keys(g.members).length + ' members, ' + g.log.length + ' events merged from ' +
      Object.keys(st.logs).length + ' member logs, ' + flagged + ' flagged, ' + Object.keys(g.items).length + ' items.</p>';
  }

  function viewMine(g) {
    var mine = g.assigns.filter(function (a) { return a.to === ui.viewer; });
    var h = '<h2>My assignments (' + esc(ui.viewer) + ')</h2>';
    if (!mine.length) return h + '<p>none</p>';
    h += '<table border="1"><tr><th>item</th><th>title</th><th>assigned by</th><th>from ancestor (root item author)</th><th>levels</th><th>completion</th><th>state</th><th>progress</th></tr>';
    mine.forEach(function (a) {
      var it = g.items[a.item], ch = chain(g, it), root = ch[ch.length - 1];
      h += '<tr><td>' + link(a.item) + '</td><td>' + esc(it.title) + '</td><td>' + esc(a.by) + '</td><td>' +
        esc(root) + (ch.length > 1 ? ' (via ' + esc(ch.slice().reverse().join(' > ')) + ')' : ' (direct)') + '</td><td>' + a.depth +
        '</td><td>' + esc(a.completion) + '</td><td>' + esc(a.state) + '</td><td>' + pctText(progAssign(g, a)) + '</td></tr>';
    });
    return h + '</table>';
  }

  function viewTree(st, g) {
    var probs = rolesProblems(st.roles), h = '<h2>Org tree (roles file + admin events)</h2>';
    if (probs.length) h += '<p>roles file problems:</p><ul>' + probs.map(function (x) { return '<li>' + esc(x) + '</li>'; }).join('') + '</ul>';
    var seen = {};
    function node(id, depth) {
      if (seen[id]) return '<li>' + esc(id) + ' (cycle guard)</li>';
      seen[id] = true;
      var m = g.members[id], kids = g.children[id] || [];
      var label = esc(id) + ' - ' + esc(m.name) + (g.admins[id] ? ' [admin]' : '') + ' <small>keys: ' + esc((m.keys || []).join(', ')) + '</small>';
      if (id === ui.viewer) label = '<b>' + label + ' &lt;- viewing</b>';
      return '<li>' + label + (kids.length ? '<ul>' + kids.map(function (k) { return node(k, depth + 1); }).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(g.members).filter(function (id) {
      var l = g.members[id].leads || [];
      return !l.length || !g.members[l[0]];
    });
    h += '<ul>' + roots.map(function (r) { return node(r, 0); }).join('') + '</ul>';
    var unreached = Object.keys(g.members).filter(function (id) { return !seen[id]; });
    if (unreached.length) h += '<p>unreachable from any root (cycle in the roles file): ' + esc(unreached.join(', ')) + '</p>';
    return h;
  }

  function viewItems(g) {
    var h = '<h2>Items</h2><table border="1"><tr><th>id</th><th>kind</th><th>title</th><th>creator</th><th>assignees</th><th>progress</th><th>status</th></tr>';
    function rows(parent, depth) {
      Object.keys(g.items).forEach(function (k) {
        var it = g.items[k];
        if ((it.parent || null) !== parent) return;
        var as = g.assigns.filter(function (a) { return a.item === it.id; }).map(function (a) { return a.to; });
        h += '<tr><td>' + link(it.id) + '</td><td>' + esc(it.kind) + '</td><td>' + new Array(depth + 1).join('&nbsp;&nbsp;&nbsp;') + (depth ? '- ' : '') +
          esc(it.title) + '</td><td>' + esc(it.author) + '</td><td>' + esc(as.join(', ')) + '</td><td>' +
          (progItem(g, it) == null ? '-' : '<progress max="100" value="' + Math.round(progItem(g, it)) + '"></progress> ' + pctText(progItem(g, it))) +
          '</td><td>' + esc(itemStatus(g, it)) + '</td></tr>';
        rows(it.id, depth + 1);
      });
    }
    rows(null, 0);
    return h + '</table>';
  }

  function evRow(r) {
    var e = r.ev;
    return '<tr><td>' + esc(r.id) + '</td><td>' + esc(e.ts) + '</td><td>' + esc(e.author) + (e.via ? ' (' + esc(e.via) + (e.auto ? ', auto' : '') + ')' : '') +
      '</td><td>' + esc(e.type) + '</td><td><code>' + esc(JSON.stringify(e.payload)) + '</code></td><td>' +
      (r.status === 'flagged' ? 'FLAGGED: ' + esc(r.reason) : 'applied') + '</td></tr>';
  }
  var EV_HEAD = '<tr><th>event</th><th>ts</th><th>author</th><th>type</th><th>payload</th><th>result</th></tr>';

  function viewDetail(g) {
    var it = ui.item && g.items[ui.item];
    var h = '<h2>Item detail</h2>';
    if (!it) return h + '<p>click an item id</p>';
    var ch = chain(g, it);
    h += '<table border="1">' +
      '<tr><th>id</th><td>' + esc(it.id) + '</td></tr><tr><th>kind</th><td>' + esc(it.kind) + '</td></tr>' +
      '<tr><th>title</th><td>' + esc(it.title) + '</td></tr><tr><th>body</th><td><pre>' + esc(it.body) + '</pre></td></tr>' +
      '<tr><th>project ref</th><td>' + esc(it.project || '-') + '</td></tr><tr><th>creator</th><td>' + esc(it.author) + '</td></tr>' +
      '<tr><th>parent</th><td>' + (it.parent ? link(it.parent) : '-') + '</td></tr>' +
      '<tr><th>ancestor chain</th><td>' + esc(ch.join(' < ')) + '</td></tr>' +
      '<tr><th>created</th><td>' + esc(it.ts) + '</td></tr>' +
      '<tr><th>rolled-up progress</th><td>' + pctText(progItem(g, it)) + ' (' + esc(itemStatus(g, it)) + ')</td></tr></table>';
    var as = g.assigns.filter(function (a) { return a.item === it.id; });
    h += '<h3>Assignments</h3>';
    if (!as.length) h += '<p>none' + (it.kind !== 'note' ? '; own progress ' + pctText(it.pct) + (it.closed ? ', closed by ' + esc(it.closed) : '') : '') + '</p>';
    else {
      h += '<table border="1"><tr><th>assignee</th><th>by</th><th>levels</th><th>completion</th><th>state</th><th>own report</th><th>rolled-up</th><th>subtasks</th></tr>';
      as.forEach(function (a) {
        var ks = childItems(g, it.id, a.to);
        h += '<tr><td>' + esc(a.to) + '</td><td>' + esc(a.by) + '</td><td>' + a.depth + '</td><td>' + esc(a.completion) + '</td><td>' + esc(a.state) +
          '</td><td>' + pctText(a.pct) + (a.note ? ' ' + esc(a.note) : '') + (ks.length && a.state !== 'done' ? ' (ignored: subtasks drive roll-up)' : '') +
          '</td><td>' + pctText(progAssign(g, a)) + '</td><td>' + (ks.map(function (k) { return link(k.id); }).join(', ') || '-') + '</td></tr>';
      });
      h += '</table>';
    }
    var kids = Object.keys(g.items).map(function (k) { return g.items[k]; }).filter(function (i) { return i.parent === it.id; });
    h += '<h3>Subtasks</h3>';
    h += kids.length ? '<ul>' + kids.map(function (k) { return '<li>' + link(k.id) + ' ' + esc(k.title) + ' (' + esc(k.author) + ') ' + pctText(progItem(g, k)) + '</li>'; }).join('') + '</ul>' : '<p>none</p>';
    var evs = it.events.map(function (r) { return r; }).concat(it.flagged);
    evs.sort(function (a, b) { return g.log.indexOf(a) - g.log.indexOf(b); });
    h += '<h3>Event history</h3><table border="1">' + EV_HEAD + evs.map(evRow).join('') + '</table>';
    return h;
  }

  function viewLog(g) {
    var rows = g.log.filter(function (r) { return !ui.flaggedOnly || r.status === 'flagged'; });
    return '<h2>Merged event log</h2><label><input type="checkbox" id="flagged-only"' + (ui.flaggedOnly ? ' checked' : '') +
      '> flagged only</label> (' + rows.length + ' shown)<table border="1">' + EV_HEAD + rows.map(evRow).join('') + '</table>';
  }

  // ---------------------------------------------------------------- actions

  function opts(list, empty) {
    if (!list.length) return '<option value="">(' + empty + ')</option>';
    // the item last touched or opened is preselected, so an action follows what the user is looking at
    return list.map(function (x) {
      return '<option value="' + esc(x[0]) + '"' + (x[0].split('|')[0] === ui.item ? ' selected' : '') + '>' + esc(x[1]) + '</option>';
    }).join('');
  }
  function peopleBoxes(g, name) {
    var r = reach(g.members, ui.viewer, 99);
    var ids = Object.keys(r).sort(function (a, b) { return r[a] - r[b]; });
    return ids.length ? ids.map(function (id) {
      return '<label><input type="checkbox" name="' + name + '" value="' + esc(id) + '"> ' + esc(id) + ' (L' + r[id] + ')</label> ';
    }).join('') : '(no followers)';
  }
  function levelSel(id) { return '<select id="' + id + '"><option>1</option><option>2</option><option>3</option></select>'; }
  var COMPL = '<option value="assignee">assignee closes</option><option value="assigner">assigner confirms</option>';

  function viewActions(g) {
    var v = ui.viewer;
    var mineCreated = Object.keys(g.items).map(function (k) { return g.items[k]; })
      .filter(function (i) { return i.author === v && i.kind !== 'note'; }).map(function (i) { return [i.id, i.id + ' ' + i.title]; });
    var received = g.assigns.filter(function (a) { return a.to === v && a.state === 'open'; })
      .map(function (a) { return [a.item, a.item + ' ' + g.items[a.item].title]; });
    var own = Object.keys(g.items).map(function (k) { return g.items[k]; }).filter(function (i) {
      return i.author === v && i.kind !== 'note' && !i.closed && !g.assigns.some(function (a) { return a.item === i.id; });
    }).map(function (i) { return [i.id, i.id + ' ' + i.title + ' (own)']; });
    var confirm = g.assigns.filter(function (a) { return a.by === v && a.completion === 'assigner' && a.state === 'awaiting_confirm'; })
      .map(function (a) { return [a.item + '|' + a.to, a.item + ' ' + g.items[a.item].title + ' by ' + a.to]; });
    var h = '<h2>Actions (append events to ' + esc(v) + "'s log)</h2>";
    h += '<p><b>Create</b> <select id="c-kind"><option>task</option><option>note</option><option>todo</option></select> ' +
      '<input id="c-title" placeholder="title"> <input id="c-body" placeholder="body"> <input id="c-project" placeholder="project ref (free text)"> ' +
      '<button data-act="create">create</button></p>';
    h += '<p><b>Assign</b> <select id="a-item">' + opts(mineCreated, 'no items created by you') + '</select> ' +
      '<select id="a-mode"><option value="all">all within N levels</option><option value="chosen">chosen within N levels</option></select> N=' + levelSel('a-levels') +
      ' <select id="a-completion">' + COMPL + '</select> chosen: ' + peopleBoxes(g, 'a-people') + ' <button data-act="assign">assign</button></p>';
    h += '<p><b>Split</b> received <select id="s-item">' + opts(received, 'nothing assigned to you') + '</select> ' +
      '<input id="s-title" placeholder="subtask title"> N=' + levelSel('s-levels') + ' <select id="s-completion">' + COMPL + '</select> to: ' +
      peopleBoxes(g, 's-people') + ' <button data-act="split">split</button></p>';
    h += '<p><b>Report</b> <select id="r-item">' + opts(received.concat(own), 'nothing to report on') + '</select> ' +
      '<input id="r-pct" type="number" min="0" max="100" value="50" size="3">% <input id="r-note" placeholder="note"> ' +
      '<label><input type="checkbox" id="r-agent"> as agent (auto)</label> <button data-act="report">report</button></p>';
    h += '<p><b>Close</b> <select id="x-item">' + opts(received.concat(own), 'nothing to close') + '</select> ' +
      '<label><input type="checkbox" id="x-agent"> as agent (auto)</label> <button data-act="close">close</button></p>';
    h += '<p><b>Confirm</b> <select id="f-sel">' + opts(confirm, 'nothing awaiting your confirmation') + '</select> <button data-act="confirm">confirm</button></p>';
    if (ui.last) {
      h += '<p>Last action produced:</p><pre>' + ui.last.map(function (x) {
        return esc(JSON.stringify(x.ev)) + '\n  => ' + (x.status === 'flagged' ? 'FLAGGED: ' + esc(x.reason) : 'applied');
      }).join('\n') + '</pre>';
    }
    return h;
  }

  function val(id) { var e = $(id); return e ? e.value : ''; }
  function checked(name) {
    return Array.prototype.map.call(document.querySelectorAll('input[name="' + name + '"]:checked'), function (e) { return e.value; });
  }
  function setMsg(m) { $('msg').textContent = m || ''; }

  function act(kind) {
    if (!applyRaw()) return;
    var st = stores[ui.store], g = replay(st), v = ui.viewer, evs = [];
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
    var item;
    switch (kind) {
      case 'create':
        if (!val('c-title')) return setMsg('title required');
        var pl = { kind: val('c-kind'), title: val('c-title') };
        if (val('c-body')) pl.body = val('c-body');
        if (val('c-project')) pl.project = val('c-project');
        item = id(mk('item.create', pl));
        break;
      case 'assign':
        item = val('a-item'); if (!item) return setMsg('no item to assign');
        var ap = { item: item, mode: val('a-mode'), levels: Number(val('a-levels')), completion: val('a-completion') };
        if (ap.mode === 'chosen') ap.people = checked('a-people');
        mk('item.assign', ap);
        break;
      case 'split':
        var parent = val('s-item'); if (!parent) return setMsg('nothing to split');
        if (!val('s-title')) return setMsg('subtask title required');
        item = id(mk('item.create', { kind: 'task', title: val('s-title'), parent: parent }));
        mk('item.assign', { item: item, mode: 'chosen', levels: Number(val('s-levels')), people: checked('s-people'), completion: val('s-completion') });
        break;
      case 'report':
        item = val('r-item'); if (!item) return setMsg('nothing to report on');
        var rp = { item: item, pct: Number(val('r-pct')) };
        if (val('r-note')) rp.note = val('r-note');
        mk('progress.report', rp, $('r-agent').checked);
        break;
      case 'close':
        item = val('x-item'); if (!item) return setMsg('nothing to close');
        mk('item.close', { item: item }, $('x-agent').checked);
        break;
      case 'confirm':
        var sel = val('f-sel'); if (!sel) return setMsg('nothing to confirm');
        item = sel.split('|')[0];
        mk('item.confirm', { item: item, assignee: sel.split('|')[1] });
        break;
    }
    var g2 = replay(st);
    ui.last = evs.map(function (e) { return g2.log.filter(function (r) { return r.ev === e; })[0]; });
    ui.item = item;
    render();
    setMsg('appended ' + evs.length + ' event(s) to ' + v);
  }

  // --------------------------------------------------------------- raw pane

  function fmtLog(a) { return '[\n' + a.map(function (e) { return '  ' + JSON.stringify(e); }).join(',\n') + '\n]'; }
  function fmtRoles(r) {
    var o = {}; Object.keys(r).forEach(function (k) { if (k !== 'members' && k !== 'admins') o[k] = r[k]; });
    return '{\n  "admins": ' + JSON.stringify(r.admins || []) + ',\n  "members": [\n' +
      r.members.map(function (m) { return '    ' + JSON.stringify(m); }).join(',\n') + '\n  ]\n}';
  }
  function viewRaw(st) {
    var owners = st.roles.members.map(function (m) { return m.id; });
    Object.keys(st.logs).forEach(function (k) { if (owners.indexOf(k) < 0) owners.push(k); });
    var h = '<h2>Raw data (editable)</h2><button data-act="apply">apply</button> <button data-act="reset">reset store to fixtures</button>' +
      '<h3>roles file</h3><textarea id="raw-roles" rows="14" cols="80" spellcheck="false">' + esc(fmtRoles(st.roles)) + '</textarea>';
    owners.forEach(function (o) {
      var log = st.logs[o] || [];
      h += '<details data-owner="' + esc(o) + '"' + (ui.open[o] ? ' open' : '') + '><summary>log of ' + esc(o) + ' (' + log.length + ' events)</summary>' +
        '<textarea data-owner="' + esc(o) + '" rows="' + Math.min(24, log.length + 3) + '" cols="80" spellcheck="false">' + esc(fmtLog(log)) + '</textarea></details>';
    });
    return h;
  }

  function applyRaw() {
    var st = stores[ui.store], ta = $('raw-roles');
    if (!ta) return true;
    var roles, logs = {};
    try { roles = JSON.parse(ta.value); } catch (e) { setMsg('JSON error in roles file: ' + e.message); return false; }
    if (!roles || !Array.isArray(roles.members)) { setMsg('roles file needs a "members" list'); return false; }
    var tas = document.querySelectorAll('textarea[data-owner]');
    for (var i = 0; i < tas.length; i++) {
      var o = tas[i].getAttribute('data-owner'), a;
      try { a = JSON.parse(tas[i].value); } catch (e2) { setMsg('JSON error in log of ' + o + ': ' + e2.message); return false; }
      if (!Array.isArray(a)) { setMsg('log of ' + o + ' must be a JSON list'); return false; }
      logs[o] = a;
    }
    st.roles = roles; st.logs = logs;
    return true;
  }

  // ----------------------------------------------------------------- render

  function render() {
    var st = stores[ui.store], g = replay(st);
    if (!g.members[ui.viewer]) ui.viewer = Object.keys(g.members)[0];
    $('store').innerHTML = Object.keys(stores).map(function (k) {
      return '<option value="' + esc(k) + '"' + (k === ui.store ? ' selected' : '') + '>' + esc(stores[k].def.label) + '</option>';
    }).join('');
    $('viewer').innerHTML = Object.keys(g.members).map(function (k) {
      return '<option value="' + esc(k) + '"' + (k === ui.viewer ? ' selected' : '') + '>' + esc(k + ' - ' + g.members[k].name) + '</option>';
    }).join('');
    $('views').innerHTML = viewSummary(st, g) + viewActions(g) + viewMine(g) + viewTree(st, g) + viewItems(g) + viewDetail(g) + viewLog(g);
    $('raw').innerHTML = viewRaw(st);
  }

  function resetStore(name) {
    var d = stores[name].def;
    stores[name].roles = clone(d.roles); stores[name].logs = clone(d.logs);
  }

  function init() {
    window.TRACKER_FIXTURES.stores.forEach(function (s) { stores[s.name] = { def: s }; resetStore(s.name); });
    var first = window.TRACKER_FIXTURES.stores[0];
    ui.store = first.name; ui.viewer = first.defaultViewer;

    document.addEventListener('click', function (e) {
      var t = e.target;
      if (t.getAttribute && t.getAttribute('data-item')) { e.preventDefault(); ui.item = t.getAttribute('data-item'); render(); return; }
      var a = t.getAttribute && t.getAttribute('data-act');
      if (!a) return;
      if (a === 'apply') { if (applyRaw()) { render(); setMsg('applied'); } }
      else if (a === 'reset') { resetStore(ui.store); ui.last = null; render(); setMsg('reset'); }
      else act(a);
    });
    document.addEventListener('change', function (e) {
      if (e.target.id === 'store') {
        ui.store = e.target.value; ui.viewer = stores[ui.store].def.defaultViewer; ui.item = null; ui.last = null; ui.open = {};
        render(); setMsg('');
      } else if (e.target.id === 'viewer') { ui.viewer = e.target.value; ui.last = null; render(); }
      else if (e.target.id === 'flagged-only') { ui.flaggedOnly = e.target.checked; render(); }
    });
    document.addEventListener('toggle', function (e) {
      var o = e.target.getAttribute && e.target.getAttribute('data-owner');
      if (o && e.target.tagName === 'DETAILS') ui.open[o] = e.target.open;
    }, true);
    render();
  }

  window.addEventListener('error', function (e) { var m = $('msg'); if (m) m.textContent = 'script error: ' + e.message; });
  init();
})();
