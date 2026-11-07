// Tracker mockup v2 data layer: validate and fold the per-member commit lists on
// top of the roles file. No DOM here; app.js is the view. Exposes
// window.TrackerEngine. The rules are the ones settled on the board (README.md).
//
// Shape: store = { roles, commits: { owner: [commit] } }, commit = { author, ts,
// signer, seen: { member: lastSeq }, events: [ { seq, type, origin?, payload } ] }.
// Event id = author#seq (displayed as author-seq). Fields starting with "_" on a
// commit are mock-only bookkeeping of app.js (local / arrived late), not model.
//
// Replay: (1) every event gets a causal past = its own earlier events plus what
// its commit's `seen` names, closed transitively; (2) the events are put in one
// order that respects that past (ties: ts, author, seq); (3) each event is checked
// against the state folded from its causal past only, so a late arrival can never
// change an earlier verdict; (4) the revocation rule may then ignore events signed
// with a revoked key that the revoker had not seen; (5) a final fold builds the
// shown state and marks concurrent changes.
(function () {
  'use strict';

  var KINDS = ['task', 'todo', 'topic'];
  var SETTABLE = ['todo', 'doing', 'blocked'];
  var ORIGINS = ['human', 'agent', 'agent-auto'];

  function clone(x) { return JSON.parse(JSON.stringify(x)); }
  function alias(id) { return String(id).replace('#', '-'); }
  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function vals(o) { return Object.keys(o).map(function (k) { return o[k]; }); }
  function Rej(m) { this.reason = m; }
  function rej(m) { throw new Rej(m); }

  // ---------------------------------------------------------------- org tree

  function childrenMap(members) {
    var c = {};
    Object.keys(members).forEach(function (id) {
      if (members[id].removed) return;
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

  // ------------------------------------------------------ flatten + causality

  function flatten(st) {
    var nodes = [], byId = {};
    Object.keys(st.commits || {}).forEach(function (owner) {
      var last = 0;
      (st.commits[owner] || []).forEach(function (c, ci) {
        (c.events || []).forEach(function (ev, ei) {
          var n = { author: c.author, owner: owner, seq: ev.seq, ts: c.ts, commit: c, ci: ci, ei: ei, ev: ev,
            type: ev.type, p: ev.payload || {}, origin: ev.origin || 'human',
            status: 'applied', reason: null, markers: [], causal: true, preds: [] };
          n.id = c.author + '#' + ev.seq;
          n.alias = (c.author !== owner ? owner + ':' : '') + alias(n.id);
          var pre = null;
          if (c.author !== owner) pre = 'commit author "' + c.author + '" does not own the ref "' + owner + '" (an event sits only in its author\'s ref)';
          else if (ev.seq !== last + 1) pre = 'seq ' + ev.seq + ' after ' + last + ' (expected ' + (last + 1) + ')';
          else last = ev.seq;
          // A broken event consumes neither a seq nor an id: it stays visible, flagged, outside causality.
          if (pre) { n.causal = false; n.status = 'flagged'; n.reason = pre; n.id = '!' + nodes.length + ':' + owner + ':' + n.id; }
          byId[n.id] = n; nodes.push(n);
        });
      });
    });
    return { nodes: nodes, byId: byId };
  }

  // Closed vector clock: member -> highest seq in the event's causal past.
  function causalize(nodes, byId) {
    function cv(n) {
      if (n.cv) return n.cv;
      if (n.busy) { n.cycle = true; return {}; }
      n.busy = true;
      var v = {}, seen = n.commit.seen || {}, ids = [];
      if (n.seq > 1) ids.push(n.author + '#' + (n.seq - 1));
      Object.keys(seen).forEach(function (m) { if (m !== n.author && seen[m] > 0) ids.push(m + '#' + seen[m]); });
      ids.forEach(function (pid) {
        var x = byId[pid];
        if (!x || !x.causal) { n.waitOn = n.waitOn || pid; return; }
        var xv = cv(x);
        if (x.cycle) n.cycle = true;
        if (x.waitOn) n.waitOn = n.waitOn || x.waitOn;
        Object.keys(xv).forEach(function (m) { if ((v[m] || 0) < xv[m]) v[m] = xv[m]; });
        if ((v[x.author] || 0) < x.seq) v[x.author] = x.seq;
        n.preds.push(x);
      });
      n.busy = false; n.cv = v;
      return v;
    }
    nodes.forEach(function (n) { if (n.causal) cv(n); });
  }

  // x happened before y: x is in y's causal past.
  function hb(x, y) { return x !== y && x.causal && !!y.cv && (y.cv[x.author] || 0) >= x.seq; }

  function later(x, y) { // display-winner order for concurrent writes: later ts, then author id, then seq
    if (x.ts !== y.ts) return String(x.ts) > String(y.ts);
    if (x.author !== y.author) return x.author > y.author;
    return x.seq > y.seq;
  }

  function linearize(nodes) {
    var pend = nodes.filter(function (n) { return n.causal && !n.waitOn && !n.cycle; }), placed = {}, out = [];
    while (pend.length) {
      var best = -1;
      for (var i = 0; i < pend.length; i++) {
        var n = pend[i];
        if (!n.preds.every(function (x) { return placed[x.id]; })) continue;
        if (best < 0 || later(pend[best], n)) best = i;
      }
      if (best < 0) break;
      placed[pend[best].id] = true;
      out.push(pend.splice(best, 1)[0]);
    }
    return out;
  }

  // ------------------------------------------------------------------ state

  function genesis(roles) {
    var S = { members: {}, admins: {}, items: {}, asg: {}, rules: {}, comments: {}, reg: {} };
    (roles.members || []).forEach(function (m) {
      S.members[m.id] = { id: m.id, name: m.name || m.id, keys: (m.keys || []).slice(), revoked: {}, leads: (m.leads || []).slice(), removed: null, joined: null };
    });
    (roles.admins || []).forEach(function (a) { S.admins[a] = true; });
    return S;
  }

  function activeRecs(S, item) { return vals(S.asg).filter(function (r) { return r.item === item && !r.removed; }); }
  function activeRec(S, item, who) { return activeRecs(S, item).filter(function (r) { return r.to === who; })[0] || null; }
  function lastRemovedRec(S, item, who) {
    var rs = vals(S.asg).filter(function (r) { return r.item === item && r.to === who && r.removed; });
    return rs[rs.length - 1] || null;
  }
  function liveRules(S, item) { return vals(S.rules).filter(function (r) { return r.item === item && !r.ended; }); }

  // ------------------------------------------------------------------ checks

  function needItem(S, id, what) {
    var it = S.items[id];
    if (!it) rej((what || 'item') + ' ' + alias(id) + ' is not in the state its author had seen');
    return it;
  }
  function needMember(S, id) {
    var m = S.members[id];
    if (!m) rej('unknown member "' + id + '"');
    if (m.removed) rej(id + ' was removed (' + alias(m.removed) + ')');
    return m;
  }
  function needAdmin(S, a) { if (!S.admins[a]) rej('roles events are admin-only (' + a + ' is not an admin)'); }
  function keyInUse(S, key) {
    return vals(S.members).some(function (m) { return m.keys.indexOf(key) >= 0 || m.revoked[key]; });
  }
  function checkLeads(S, id, leads) {
    if (!Array.isArray(leads)) rej('leads must be a list');
    if (leads.length > 1) rej('leads must have length at most 1 (got ' + leads.length + ')');
    leads.forEach(function (l) {
      if (!S.members[l] || S.members[l].removed) rej('lead "' + l + '" is not a member');
      if (l === id || wouldCycle(S.members, id, l)) rej('cycle: ' + l + ' is already below ' + id);
    });
  }
  function notAssignee(S, it, who) {
    var gone = lastRemovedRec(S, it.id, who);
    if (gone) return who + ' was unassigned from ' + it.alias + ' (' + alias(gone.removed.id) + ')';
    if (it.author === who) return who + ' created ' + it.alias + ' but is not assigned to it (self-assign first)';
    return who + ' is not an assignee of ' + it.alias;
  }
  function needRec(S, item, who) {
    var r = activeRec(S, item, who);
    if (!r) rej(notAssignee(S, S.items[item], who));
    return r;
  }
  function needRequest(S, p) {
    var it = needItem(S, p.item);
    if (!it.request) rej(it.alias + ' is not a request');
    return it;
  }

  function check(S, n) {
    var p = n.p, a = n.author, c = n.commit, m = S.members[a], it, r, q;
    if (!m) rej('author "' + a + '" is not a member in the state its author had seen');
    if (m.removed) rej(a + ' was removed from the store (' + alias(m.removed) + ')');
    if (!c.signer) rej('commit is unsigned');
    if (m.revoked[c.signer]) rej('signed with ' + c.signer + ', already revoked in its own causal past (' + alias(m.revoked[c.signer]) + ')');
    if (m.keys.indexOf(c.signer) < 0) rej('signer key "' + c.signer + '" is not a key of ' + a);
    if (ORIGINS.indexOf(n.origin) < 0) rej('origin must be human, agent or agent-auto');
    if (n.origin === 'agent-auto' && /^request\./.test(n.type)) rej('an agent never accepts, declines or returns a request on its own (agent-auto is invalid here)');
    if (n.origin === 'agent-auto' && n.type !== 'item.status' && n.type !== 'item.close')
      rej('agent-auto is limited to item.status and item.close on the author\'s own assignment');

    switch (n.type) {
      case 'item.create':
        if (!p.title) rej('title required');
        if (KINDS.indexOf(p.kind) < 0) rej('kind must be task, todo or topic');
        if (p.project != null && typeof p.project !== 'string') rej('project is one optional free-text string');
        if (p.parent) {
          var par = needItem(S, p.parent, 'parent');
          if (par.hidden) rej('parent ' + par.alias + ' is hidden');
          if (par.author !== a && !activeRec(S, p.parent, a)) rej('parent ' + par.alias + ' is neither assigned to nor created by ' + a);
        }
        if (p.request != null) {
          if (p.kind === 'topic') rej('a topic cannot be a request');
          if (p.request === a) rej('a request names someone else (self-assign instead)');
          needMember(S, p.request);
        }
        break;

      case 'item.edit':
        it = needItem(S, p.item);
        if (it.author !== a) rej('only the creator (' + it.author + ') edits ' + it.alias);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (!['title', 'body', 'project'].some(function (f) { return has(p, f); })) rej('nothing to edit (title, body or project)');
        if (has(p, 'title') && !p.title) rej('title cannot be empty');
        break;

      case 'item.hide': {
        it = needItem(S, p.item);
        if (it.author !== a) rej('only the creator (' + it.author + ') hides ' + it.alias);
        if (it.hidden) rej(it.alias + ' is already hidden');
        var open = activeRecs(S, it.id).filter(function (x) { return x.to !== a && x.status !== 'done'; });
        if (open.length) rej('unassign the open assignments first (' + open.map(function (x) { return x.to; }).join(', ') + ')');
        break;
      }

      case 'item.assign': {
        it = needItem(S, p.item);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (it.kind === 'topic') rej('topics have no assignment');
        if (it.request) rej('a request becomes an assignment only when its recipient accepts it');
        var cr = S.members[it.author];
        if (it.author !== a && !(S.admins[a] && cr && cr.removed)) rej('only the creator (' + it.author + ') assigns ' + it.alias);
        var completion = p.completion || 'assignee';
        if (completion !== 'assignee' && completion !== 'assigner') rej('completion must be assignee or assigner');
        var levels = Number(p.levels || 0), R, all = reach(S.members, a, 99);
        if (p.mode === 'all') {
          if (!(levels >= 1)) rej('levels must be >= 1');
          R = reach(S.members, a, levels);
          if (!Object.keys(R).length) rej('nobody is within ' + levels + ' levels below ' + a);
          var cov = liveRules(S, it.id).filter(function (x) { return x.by === a && x.levels >= levels; })[0];
          if (cov) rej('already assigned to everyone within ' + cov.levels + ' levels (' + alias(cov.id) + ')');
        } else if (p.mode === 'chosen') {
          var ppl = p.people || [];
          if (!ppl.length) rej('choose at least one person');
          R = levels >= 1 ? reach(S.members, a, levels) : {};
          ppl.forEach(function (x) {
            if (x === a) return; // self-assignment
            if (!S.members[x] || S.members[x].removed) rej(x + ' is not a member');
            if (!(x in R)) rej('outside reach: ' + x + (x in all ? ' is ' + all[x] + ' levels below ' + a + ', limit ' + levels : ' is not below ' + a));
          });
          if (!ppl.some(function (x) { return !activeRec(S, it.id, x); })) rej('everyone chosen is already assigned');
        } else rej('mode must be all or chosen');
        break;
      }

      case 'item.unassign':
        it = needItem(S, p.item);
        if (p.assignee) {
          r = activeRec(S, it.id, p.assignee);
          if (!r) rej(p.assignee + ' has no active assignment on ' + it.alias);
          var departed = (S.members[r.to] || {}).removed || (S.members[r.by] || {}).removed;
          if (r.by !== a && !(S.admins[a] && departed)) rej('only the assigner (' + r.by + ') unassigns');
        } else if (!activeRecs(S, it.id).some(function (x) { return x.by === a; }) && !liveRules(S, it.id).some(function (x) { return x.by === a; }))
          rej(a + ' has no assignments to withdraw on ' + it.alias);
        break;

      case 'item.status':
        it = needItem(S, p.item);
        if (SETTABLE.indexOf(p.status) < 0) rej('status must be todo, doing or blocked (done goes through item.close)');
        r = needRec(S, it.id, a);
        if (r.status === 'done') rej('assignment is done; reopen it first');
        break;

      case 'item.close':
        it = needItem(S, p.item);
        r = needRec(S, it.id, a);
        if (r.status === 'done') rej('assignment is already done');
        if (r.status === 'awaiting') rej('already awaiting confirmation by ' + r.by);
        break;

      case 'item.confirm':
      case 'item.reject':
        it = needItem(S, p.item);
        r = activeRec(S, it.id, p.assignee);
        if (!r) rej('no active assignment of ' + it.alias + ' to ' + p.assignee);
        if (r.by !== a) rej('only the assigner (' + r.by + ') ' + (n.type === 'item.confirm' ? 'confirms' : 'rejects'));
        if (r.completion !== 'assigner') rej('this assignment does not require confirmation');
        if (r.status !== 'awaiting') rej('assignment is ' + r.status + ', not awaiting confirmation');
        break;

      case 'item.reopen':
        it = needItem(S, p.item);
        r = activeRec(S, it.id, p.assignee || a);
        if (!r) rej(notAssignee(S, it, p.assignee || a));
        if (r.by !== a && r.to !== a) rej('only the assigner (' + r.by + ') or the assignee (' + r.to + ') reopens');
        if (r.status !== 'done') rej('assignment is ' + r.status + ', not done');
        break;

      case 'request.accept':
      case 'request.decline':
        it = needRequest(S, p);
        if (it.request.to !== a) rej('only the recipient (' + it.request.to + ') ' + (n.type === 'request.accept' ? 'accepts' : 'declines'));
        if (it.request.state !== 'pending') rej('request is already ' + it.request.state);
        break;

      case 'request.return': {
        it = needRequest(S, p);
        var to = it.request.to, lead = ((S.members[to] || {}).leads || [])[0];
        if (lead !== a) rej('only ' + to + '\'s direct lead (' + (lead || 'none: ' + to + ' is a root') + ') returns an accepted request');
        if (it.request.state !== 'accepted') rej('request is ' + it.request.state + ', not accepted');
        break;
      }

      case 'comment.add':
        it = needItem(S, p.item);
        if (it.hidden) rej('thread ' + it.alias + ' is hidden');
        if (!p.body) rej('body required');
        if (p.reply_to) {
          q = S.comments[p.reply_to];
          if (!q) rej('reply_to ' + alias(p.reply_to) + ' is not a comment its author had seen');
          if (q.item !== p.item) rej('reply_to must be in the same thread');
          if (q.hidden) rej('cannot reply to a hidden comment');
        }
        break;

      case 'comment.edit':
      case 'comment.hide':
        q = S.comments[p.comment];
        if (!q) rej('comment ' + alias(p.comment) + ' is not in the state its author had seen');
        if (q.author !== a) rej('only the comment author (' + q.author + ') ' + (n.type === 'comment.edit' ? 'edits' : 'hides') + ' it');
        if (q.hidden) rej('comment is already hidden');
        if (n.type === 'comment.edit' && !p.body) rej('body required');
        break;

      case 'roles.add_member': {
        needAdmin(S, a);
        var nm = p.member || {};
        if (!nm.id) rej('member.id required');
        if (S.members[nm.id]) rej('member "' + nm.id + '" already exists' + (S.members[nm.id].removed ? ' (removed; ids are not reused)' : ''));
        if (!nm.keys || !nm.keys.length) rej('member.keys required');
        nm.keys.forEach(function (k) { if (keyInUse(S, k)) rej('key "' + k + '" is already in use'); });
        checkLeads(S, nm.id, nm.leads || []);
        break;
      }
      case 'roles.set_lead':
        needAdmin(S, a); needMember(S, p.member); checkLeads(S, p.member, p.leads || []);
        break;
      case 'roles.add_admin':
        needAdmin(S, a); needMember(S, p.member);
        if (S.admins[p.member]) rej(p.member + ' is already an admin');
        break;
      case 'roles.remove_admin':
        needAdmin(S, a);
        if (!S.admins[p.member]) rej(p.member + ' is not an admin');
        if (Object.keys(S.admins).length <= 1) rej('cannot remove the last admin');
        break;
      case 'roles.add_key':
        needAdmin(S, a); needMember(S, p.member);
        if (!p.key) rej('key required');
        if (keyInUse(S, p.key)) rej('key "' + p.key + '" is already in use');
        break;
      case 'roles.revoke_key':
        needAdmin(S, a);
        if (!S.members[p.member]) rej('unknown member "' + p.member + '"');
        if (S.members[p.member].revoked[p.key]) rej('key "' + p.key + '" is already revoked');
        if (S.members[p.member].keys.indexOf(p.key) < 0) rej('"' + p.key + '" is not a key of ' + p.member);
        break;
      case 'roles.remove_member': {
        needAdmin(S, a); needMember(S, p.member);
        var kids = childrenMap(S.members)[p.member] || [];
        if (kids.length) rej('reassign ' + p.member + '\'s followers first (' + kids.join(', ') + ')');
        if (S.admins[p.member]) rej('remove the admin role first');
        break;
      }

      default: rej('unknown event type "' + n.type + '"');
    }
  }

  // ------------------------------------------------------------------ apply
  // Lenient: an event that passed its own check is folded even when a concurrent
  // event has since changed the state under it. C (final fold only) turns on the
  // concurrent-change markers.

  function show(v) {
    if (v && typeof v === 'object' && !Array.isArray(v)) return v.status + (v.note ? ' ("' + v.note + '")' : '');
    if (Array.isArray(v)) return v.join(', ') || 'nobody';
    if (v === true) return 'hidden';
    var s = String(v);
    return s.length > 60 ? s.slice(0, 57) + '...' : s;
  }
  function mark(n, other, kind, field, info) {
    if (n.markers.some(function (x) { return x.other === other.id && x.field === field; })) return;
    n.markers.push({ kind: kind, field: field, other: other.id, winner: info.winner, mine: info.mine, theirs: info.theirs });
  }

  // Multi-value register: heads are the writes no later write has seen. One head
  // = no conflict. Several = concurrent writes; the shown value is the display
  // winner (later ts, then author id) and every head stays visible in the thread.
  function write(S, key, label, n, v, C) {
    var heads = (S.reg[key] || []).filter(function (h) { return !hb(h.n, n); }), conc = heads.slice();
    heads.push({ n: n, v: v });
    S.reg[key] = heads;
    var w = heads.reduce(function (b, h) { return !b || later(h.n, b.n) ? h : b; }, null);
    if (C) conc.forEach(function (h) {
      mark(h.n, n, 'field', label, { winner: w.n.id, mine: show(h.v), theirs: show(v) });
      mark(n, h.n, 'field', label, { winner: w.n.id, mine: show(v), theirs: show(h.v) });
    });
    return w.v;
  }

  function addRec(S, via, item, to, by, completion, extra) {
    var id = via.id + '>' + to;
    if (S.asg[id]) return;
    var r = { id: id, item: item, to: to, by: by, completion: completion, via: via.id, ts: via.ts,
      depth: to === by ? 0 : (reach(S.members, by, 99)[to] || null),
      status: 'todo', note: null, noteBy: null, last: via, removed: null, writes: [], inherited: null, request: false };
    if (extra) Object.keys(extra).forEach(function (k) { r[k] = extra[k]; });
    S.asg[id] = r;
  }

  // Live "everyone within N": whoever enters the reach later inherits the assignment.
  function refreshLive(S, n) {
    vals(S.rules).forEach(function (rule) {
      if (rule.ended) return;
      Object.keys(reach(S.members, rule.by, rule.levels)).forEach(function (t) {
        if (rule.excluded[t] || activeRec(S, rule.item, t) || S.asg[rule.id + '>' + t]) return;
        addRec(S, { id: rule.id, ts: n.ts }, rule.item, t, rule.by, rule.completion, { inherited: n.id, last: n });
      });
    });
  }

  function removeRec(S, r, n, C) {
    r.removed = n;
    if (!C) return;
    var label = r.to + '\'s assignment on ' + alias(r.item);
    r.writes.forEach(function (w) {
      if (hb(w, n)) return;
      mark(w, n, 'removed', label, { winner: n.id });
      mark(n, w, 'removed', label, { winner: n.id });
    });
    var parent = S.items[r.item];
    vals(S.items).forEach(function (it) {
      if (it.parent !== r.item || it.author !== r.to || !parent || parent.author === r.to || hb(it.node, n)) return;
      mark(it.node, n, 'subtask', 'subtask under ' + alias(r.item), { winner: n.id });
      mark(n, it.node, 'subtask', 'subtask under ' + alias(r.item), { winner: n.id });
    });
  }

  function apply(S, n, C) {
    var p = n.p, a = n.author, it, q, m, r;
    switch (n.type) {
      case 'item.create':
        S.items[n.id] = { id: n.id, alias: n.alias, kind: p.kind, title: p.title, body: p.body || '', project: p.project || '',
          parent: p.parent || null, author: a, ts: n.ts, hidden: false, edited: null, node: n,
          request: p.request ? { to: p.request, by: a, state: 'pending', node: n, done: null, returned: null, note: null } : null };
        break;
      case 'item.edit':
        it = S.items[p.item]; if (!it) break;
        ['title', 'body', 'project'].forEach(function (f) {
          if (has(p, f)) it[f] = write(S, 'it:' + it.id + ':' + f, f + ' of ' + it.alias, n, p[f], C);
        });
        it.edited = n;
        break;
      case 'item.hide':
        it = S.items[p.item]; if (!it) break;
        it.hidden = write(S, 'it:' + it.id + ':hidden', it.alias + ' hidden', n, true, C);
        break;
      case 'item.assign': {
        var completion = p.completion || 'assignee', targets;
        if (!S.items[p.item]) break;
        if (p.mode === 'all') {
          targets = Object.keys(reach(S.members, a, Number(p.levels)));
          S.rules[n.id] = { id: n.id, item: p.item, by: a, levels: Number(p.levels), completion: completion, excluded: {}, ended: null };
        } else targets = p.people || [];
        targets.forEach(function (t) {
          if (S.members[t] && !activeRec(S, p.item, t)) addRec(S, n, p.item, t, a, completion);
        });
        break;
      }
      case 'item.unassign':
        if (p.assignee) {
          r = activeRec(S, p.item, p.assignee);
          liveRules(S, p.item).forEach(function (rule) { if (rule.by === (r ? r.by : a)) rule.excluded[p.assignee] = true; });
          if (r) removeRec(S, r, n, C);
        } else {
          liveRules(S, p.item).forEach(function (rule) { if (rule.by === a) rule.ended = n.id; });
          activeRecs(S, p.item).filter(function (x) { return x.by === a; }).forEach(function (x) { removeRec(S, x, n, C); });
        }
        break;
      case 'item.status': case 'item.close': case 'item.confirm': case 'item.reject': case 'item.reopen': {
        var who = (n.type === 'item.status' || n.type === 'item.close') ? a : (p.assignee || a);
        r = activeRec(S, p.item, who);
        if (!r) { // folded onto an assignment that a concurrent event removed
          var gone = lastRemovedRec(S, p.item, who);
          if (gone) {
            gone.writes.push(n);
            if (C && !hb(gone.removed, n)) {
              var lb = who + '\'s assignment on ' + alias(p.item);
              mark(n, gone.removed, 'removed', lb, { winner: gone.removed.id });
              mark(gone.removed, n, 'removed', lb, { winner: gone.removed.id });
            }
          }
          break;
        }
        r.writes.push(n);
        var v = n.type === 'item.status' ? p.status : n.type === 'item.close' ? (r.completion === 'assigner' ? 'awaiting' : 'done') :
          n.type === 'item.confirm' ? 'done' : 'doing';
        var w = write(S, 'st:' + r.id, 'status of ' + who + ' on ' + alias(p.item), n, { status: v, note: p.note || null, by: a, n: n }, C);
        r.status = w.status; r.note = w.note; r.noteBy = w.by; r.last = w.n;
        break;
      }
      case 'request.accept':
        it = S.items[p.item]; if (!it || !it.request) break;
        it.request.state = 'accepted'; it.request.done = n;
        addRec(S, n, it.id, a, a, 'assignee', { request: true });
        break;
      case 'request.decline':
        it = S.items[p.item]; if (!it || !it.request) break;
        it.request.state = 'declined'; it.request.done = n; it.request.note = p.note || null;
        break;
      case 'request.return':
        it = S.items[p.item]; if (!it || !it.request) break;
        it.request.state = 'returned'; it.request.returned = n; it.request.note = p.note || null;
        r = activeRec(S, it.id, it.request.to);
        if (r) removeRec(S, r, n, C);
        break;
      case 'comment.add':
        S.comments[n.id] = { id: n.id, alias: n.alias, item: p.item, author: a, body: p.body, reply_to: p.reply_to || null,
          hidden: false, edited: null, ts: n.ts, node: n };
        break;
      case 'comment.edit':
        q = S.comments[p.comment]; if (!q) break;
        q.body = write(S, 'cm:' + q.id, 'text of comment ' + q.alias, n, p.body, C); q.edited = n;
        break;
      case 'comment.hide':
        q = S.comments[p.comment]; if (q) { q.hidden = true; q.hiddenBy = n; }
        break;
      case 'roles.add_member': {
        var nm = p.member || {};
        S.members[nm.id] = { id: nm.id, name: nm.name || nm.id, keys: (nm.keys || []).slice(), revoked: {}, leads: (nm.leads || []).slice(), removed: null, joined: n.id };
        break;
      }
      case 'roles.set_lead': {
        m = S.members[p.member]; if (!m) break;
        var leads = p.leads || [];
        if (leads.some(function (l) { return l === m.id || wouldCycle(S.members, m.id, l); })) break; // concurrent edits closing a cycle
        m.leads = write(S, 'lead:' + m.id, 'lead of ' + m.id, n, leads, C);
        break;
      }
      case 'roles.add_admin': S.admins[p.member] = true; break;
      case 'roles.remove_admin': delete S.admins[p.member]; break;
      case 'roles.add_key': m = S.members[p.member]; if (m) m.keys.push(p.key); break;
      case 'roles.revoke_key':
        m = S.members[p.member]; if (!m) break;
        m.keys = m.keys.filter(function (k) { return k !== p.key; }); m.revoked[p.key] = n.id;
        break;
      case 'roles.remove_member':
        m = S.members[p.member]; if (m) { m.removed = n.id; delete S.admins[p.member]; }
        break;
    }
    if (/^roles\./.test(n.type) || n.type === 'item.assign') refreshLive(S, n);
  }

  // ------------------------------------------------------------------ replay

  function replay(st) {
    var f = flatten(st), nodes = f.nodes, byId = f.byId;
    causalize(nodes, byId);
    nodes.forEach(function (n) {
      if (!n.causal) return;
      if (n.cycle) { n.status = 'flagged'; n.reason = 'causal cycle: its seen names an event that (transitively) saw it'; }
      else if (n.waitOn) { n.status = 'waiting'; n.reason = 'waiting for ' + alias(n.waitOn) + ', which its seen names but this client has not received'; }
    });
    var order = linearize(nodes), ignored = {};

    // Verdicts. A pass repeats only when the revocation rule ignores a new event.
    for (var pass = 0; pass < 4; pass++) {
      order.forEach(function (n) {
        n.status = 'applied'; n.reason = null; n.revokedLate = false;
        if (ignored[n.id]) { n.status = 'flagged'; n.reason = ignored[n.id]; n.revokedLate = true; return; }
        var S = genesis(st.roles);
        for (var i = 0; order[i] !== n; i++) if (order[i].status === 'applied' && hb(order[i], n)) apply(S, order[i], null);
        try { check(S, n); } catch (e) {
          if (!(e instanceof Rej)) throw e;
          n.status = 'flagged'; n.reason = e.reason;
        }
      });
      var more = false;
      order.forEach(function (r) {
        if (r.status !== 'applied' || r.type !== 'roles.revoke_key') return;
        var who = r.p.member, key = r.p.key, cut = r.cv[who] || 0;
        order.forEach(function (n) {
          if (n.author !== who || n.commit.signer !== key || n.seq <= cut || ignored[n.id] || n.status !== 'applied') return;
          ignored[n.id] = 'signed with ' + key + ', which ' + r.alias + ' revoked having seen ' + who + ' only up to seq ' + cut +
            '; events from a revoked key that the revoker had not seen are ignored, whatever their seen claims';
          more = true;
        });
      });
      if (!more) break;
    }

    var S = genesis(st.roles), C = {};
    order.forEach(function (n) { if (n.status === 'applied') apply(S, n, C); });

    // threads: an event belongs to the item it names; a child create is also a line in the parent thread
    var cmItem = {};
    nodes.forEach(function (n) { if (n.type === 'comment.add' && n.p.item) cmItem[n.id] = n.p.item; });
    nodes.forEach(function (n) {
      n.thread = n.type === 'item.create' ? n.id : (n.p.item || (n.p.comment && cmItem[n.p.comment]) || null);
      n.parentThread = n.type === 'item.create' && n.p.parent ? n.p.parent : null;
    });

    // display order: the causal order, with events outside it (envelope-flagged, waiting) placed by ts
    var log = order.slice();
    nodes.forEach(function (n) {
      if (order.indexOf(n) >= 0) return;
      var i = 0;
      while (i < log.length && String(log[i].ts) <= String(n.ts)) i++;
      log.splice(i, 0, n);
    });

    return { members: S.members, admins: S.admins, items: S.items, asg: S.asg, rules: S.rules, comments: S.comments,
      log: log, byId: byId, children: childrenMap(S.members), heads: headSeqs(nodes) };
  }

  // Highest valid seq received per member: the `seen` of a commit written now, and
  // (for the writer) the seq its next event continues from.
  function headSeqs(nodes) {
    var out = {};
    nodes.forEach(function (n) { if (n.causal && n.seq > (out[n.author] || 0)) out[n.author] = n.seq; });
    return out;
  }

  // Problems in the roles file itself (the genesis the events replay onto).
  function rolesProblems(roles) {
    var out = [], ids = {};
    (roles.members || []).forEach(function (m) { ids[m.id] = m; });
    (roles.admins || []).forEach(function (a) { if (!ids[a]) out.push('admin "' + a + '" is not a member'); });
    if (!(roles.admins || []).length) out.push('no admins');
    (roles.members || []).forEach(function (m) {
      var l = m.leads || [];
      if (l.length > 1) out.push(m.id + ': leads must have length at most 1 (got ' + l.length + ')');
      l.forEach(function (x) { if (!ids[x]) out.push(m.id + ': lead "' + x + '" is not a member'); });
      if (!m.keys || !m.keys.length) out.push(m.id + ': no keys');
    });
    return out;
  }

  // ----------------------------------------------------------- derived data

  function itemList(g) { return vals(g.items); }
  function recsOf(g, item, withRemoved) {
    return vals(g.asg).filter(function (r) { return r.item === item && (withRemoved || !r.removed); });
  }
  function subtasksOf(g, id, withHidden) {
    return itemList(g).filter(function (i) { return i.parent === id && (withHidden || !i.hidden); });
  }
  // The item's own status from its assignments (manual by its assignees); requests
  // that are not accepted show their request state instead; topics have none.
  function itemStatus(g, it) {
    if (it.kind === 'topic') return null;
    if (it.request && it.request.state !== 'accepted') return it.request.state;
    var s = recsOf(g, it.id).map(function (r) { return r.status; });
    if (!s.length) return 'unassigned';
    if (s.every(function (x) { return x === 'done'; })) return 'done';
    if (s.indexOf('blocked') >= 0) return 'blocked';
    if (s.indexOf('awaiting') >= 0) return 'awaiting';
    if (s.some(function (x) { return x === 'doing' || x === 'done'; })) return 'doing';
    return 'todo';
  }
  function counted(it) { return it.kind !== 'topic' && !(it.request && it.request.state !== 'accepted'); }
  // Children by status (topics and requests not accepted are not counted) and
  // every blocked descendant down the tree.
  function rollup(g, it) {
    var kids = subtasksOf(g, it.id).filter(counted), counts = {}, blocked = [];
    kids.forEach(function (k) { var s = itemStatus(g, k); counts[s] = (counts[s] || 0) + 1; });
    (function walk(id, depth) {
      if (depth > 30) return;
      subtasksOf(g, id).forEach(function (k) {
        if (counted(k) && itemStatus(g, k) === 'blocked') blocked.push(k.id);
        walk(k.id, depth + 1);
      });
    })(it.id, 0);
    return { total: kids.length, counts: counts, blocked: blocked };
  }
  function chain(g, it) { // authors from the item up to the root item
    var out = [], cur = it, n = 0;
    while (cur && n++ < 50) { out.push(cur.author); cur = cur.parent && g.items[cur.parent]; }
    return out;
  }
  function threadOf(g, id) {
    return g.log.filter(function (n) { return n.thread === id || n.parentThread === id; });
  }
  function lastActivity(g, id) { // ts of the newest applied event in the thread
    var t = '';
    g.log.forEach(function (n) { if (n.status === 'applied' && n.thread === id && String(n.ts) > t) t = String(n.ts); });
    return t;
  }

  window.TrackerEngine = {
    KINDS: KINDS, SETTABLE: SETTABLE, ORIGINS: ORIGINS,
    clone: clone, alias: alias, replay: replay, rolesProblems: rolesProblems, reach: reach, hb: hb,
    itemList: itemList, recsOf: recsOf, subtasksOf: subtasksOf, itemStatus: itemStatus, rollup: rollup,
    chain: chain, threadOf: threadOf, lastActivity: lastActivity
  };
})();
