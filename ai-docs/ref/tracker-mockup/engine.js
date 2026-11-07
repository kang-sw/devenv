// Tracker mockup data layer: replay the merged per-member event logs on top of the
// roles file and derive assignment/item state. No DOM here; app.js is the view.
// Exposes window.TrackerEngine. The replay rules are the proposal being settled
// (see README.md), kept as in v0.
(function () {
  'use strict';

  var KINDS = ['task', 'note', 'todo'];

  function clone(x) { return JSON.parse(JSON.stringify(x)); }
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

  function itemList(g) { return Object.keys(g.items).map(function (k) { return g.items[k]; }); }
  function assignsOf(g, itemId) { return g.assigns.filter(function (a) { return a.item === itemId; }); }
  function subtasksOf(g, itemId) { return itemList(g).filter(function (i) { return i.parent === itemId; }); }

  window.TrackerEngine = {
    KINDS: KINDS, clone: clone, mean: mean,
    replay: replay, rolesProblems: rolesProblems, reach: reach,
    childItems: childItems, progAssign: progAssign, progItem: progItem, itemStatus: itemStatus, chain: chain,
    itemList: itemList, assignsOf: assignsOf, subtasksOf: subtasksOf
  };
})();
