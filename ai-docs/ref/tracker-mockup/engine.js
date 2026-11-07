// Tracker mockup v3 data layer: validate and fold the per-clone commit lists on top
// of the roles file, and simulate the per-clone journal and sync. No DOM here;
// app.js is the view. Exposes window.TrackerEngine. The rules are the ones settled
// on the board (README.md), and this file is the reference the Go port follows.
//
// Shape: input = { roles, commits: { stream: [commit] } }, stream = "<member>.<clone>"
// (ref refs/heads/tracker/m/<member>/<clone>), commit = { stream, ts, signer,
// seen: { stream: lastSeq }, events: [ { seq, ts, type, origin?, payload } ] }.
// Event id = <member>.<clone>#<seq> (displayed as member.clone-seq). Fields starting
// with "_" on a commit are mock bookkeeping (_local, _open, _late), not model.
//
// Replay: (1) every event gets a causal past = its stream's earlier events plus what
// its commit's `seen` names, closed transitively; (2) the events are put in one
// order that respects that past (ties: ts, member, clone, seq); (3) each event is
// checked against the state folded from its causal past only, so a late arrival can
// never change an earlier verdict; (4) the revocation rule may then ignore events
// signed with a revoked key that the revoker had not seen; (5) a final fold builds
// the shown state and marks concurrent changes.
//
// Public surface (the future backend API shape, see README "View state"):
//   openStore(def)                  -> store handle (journal, clones, sync)
//   store.view(stream, now)         -> deriveViewState(...) plus sync info, plain JSON
//   store.dispatch(stream, events, ts) -> per-event verdicts; the only write path
//   store.sync / fetch / setOnline / raw / setRaw / reset / nextEventId
//   deriveViewState(input, { stream, now }) -> plain JSON, no engine objects
(function () {
  'use strict';

  var KINDS = ['task', 'todo', 'topic'];
  var SETTABLE = ['todo', 'doing', 'blocked'];
  var ORIGINS = ['human', 'agent', 'agent-auto'];
  var ITEM_FIELDS = ['title', 'body', 'project', 'due', 'priority'];
  // Priority levels are store configuration (roles file, roles.set_priority_levels).
  // Codes are integers 0..n-1, larger = more urgent; names are display names only.
  var DEFAULT_PRIORITY = { names: ['알림', '업무', '긴급'], default: 1 };
  // Stale (284): a live assignment in doing or blocked with no status write for this
  // many days by the viewer's clock. A client-side constant, not shared data; a
  // view may pass its own as deriveViewState(input, {staleDays}).
  var STALE_DAYS = 3;
  var DUE_RE = /^\d{4}-\d{2}-\d{2}$/;

  function clone(x) { return x === undefined ? undefined : JSON.parse(JSON.stringify(x)); }
  function alias(id) { return String(id).replace('#', '-'); }
  function has(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function vals(o) { return Object.keys(o).map(function (k) { return o[k]; }); }
  function memberOf(stream) { var s = String(stream), i = s.indexOf('.'); return i < 0 ? s : s.slice(0, i); }
  function cloneOf(stream) { var s = String(stream), i = s.indexOf('.'); return i < 0 ? '' : s.slice(i + 1); }
  function refOf(stream) { return 'refs/heads/tracker/m/' + memberOf(stream) + '/' + cloneOf(stream); }
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
  // The forced-assignment rule: `by` reaches `to` when `by` is in `to`'s lead chain.
  function below(members, by, to) { return to in reach(members, by, 99); }

  function wouldCycle(members, id, lead) {
    var cur = lead, n = 0;
    while (cur && n++ <= Object.keys(members).length + 1) {
      if (cur === id) return true;
      cur = members[cur] && (members[cur].leads || [])[0];
    }
    return false;
  }

  function normPriority(p) {
    if (!p || !Array.isArray(p.names) || !p.names.length) return clone(DEFAULT_PRIORITY);
    var d = Number.isInteger(p.default) && p.default >= 0 && p.default < p.names.length ? p.default : Math.min(1, p.names.length - 1);
    return { names: p.names.slice(), default: d };
  }
  // A stored value outside the current range displays as the top level.
  function priorityLevel(prio, v) {
    var top = prio.names.length - 1;
    return Number.isInteger(v) && v >= 0 && v <= top ? v : top;
  }

  // ------------------------------------------------------ flatten + causality

  function flatten(st) {
    var nodes = [], byId = {};
    Object.keys(st.commits || {}).forEach(function (ref) {
      var last = 0;
      (st.commits[ref] || []).forEach(function (c, ci) {
        (c.events || []).forEach(function (ev, ei) {
          var stream = String(c.stream || '');
          var n = { stream: stream, author: memberOf(stream), clone: cloneOf(stream), ref: ref, seq: ev.seq, ts: ev.ts || c.ts,
            commit: c, ci: ci, ei: ei, ev: ev, type: ev.type, p: ev.payload || {}, origin: ev.origin || 'human',
            status: 'applied', reason: null, markers: [], causal: true, preds: [] };
          n.id = stream + '#' + ev.seq;
          n.alias = (stream !== ref ? ref + ':' : '') + alias(n.id);
          var pre = null;
          if (stream !== ref) pre = 'commit stream "' + stream + '" does not match the ref ' + refOf(ref) + ' (a clone writes only its own ref)';
          else if (ev.seq !== last + 1) pre = 'seq ' + ev.seq + ' after ' + last + ' (expected ' + (last + 1) + ')';
          else last = ev.seq;
          // A broken event consumes neither a seq nor an id: it stays visible, flagged, outside causality.
          if (pre) { n.causal = false; n.status = 'flagged'; n.reason = pre; n.id = '!' + nodes.length + ':' + ref + ':' + n.id; }
          byId[n.id] = n; nodes.push(n);
        });
      });
    });
    return { nodes: nodes, byId: byId };
  }

  // Closed vector clock: stream -> highest seq in the event's causal past.
  function causalize(nodes, byId) {
    function cv(n) {
      if (n.cv) return n.cv;
      if (n.busy) { n.cycle = true; return {}; }
      n.busy = true;
      var v = {}, seen = n.commit.seen || {}, ids = [];
      if (n.seq > 1) ids.push(n.stream + '#' + (n.seq - 1));
      Object.keys(seen).forEach(function (s) { if (s !== n.stream && seen[s] > 0) ids.push(s + '#' + seen[s]); });
      ids.forEach(function (pid) {
        var x = byId[pid];
        if (!x || !x.causal) { n.waitOn = n.waitOn || pid; return; }
        var xv = cv(x);
        if (x.cycle) n.cycle = true;
        if (x.waitOn) n.waitOn = n.waitOn || x.waitOn;
        Object.keys(xv).forEach(function (s) { if ((v[s] || 0) < xv[s]) v[s] = xv[s]; });
        if ((v[x.stream] || 0) < x.seq) v[x.stream] = x.seq;
        n.preds.push(x);
      });
      n.busy = false; n.cv = v;
      return v;
    }
    nodes.forEach(function (n) { if (n.causal) cv(n); });
  }

  // x happened before y: x is in y's causal past.
  function hb(x, y) { return x !== y && x.causal && !!y.cv && (y.cv[x.stream] || 0) >= x.seq; }

  // Display-winner order for concurrent writes (and the fold tie-break): later ts,
  // then member id, then clone id, then seq.
  function later(x, y) {
    if (x.ts !== y.ts) return String(x.ts) > String(y.ts);
    if (x.author !== y.author) return x.author > y.author;
    if (x.clone !== y.clone) return x.clone > y.clone;
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
    var S = { members: {}, admins: {}, items: {}, asg: {}, rules: {}, comments: {}, reg: {}, prio: normPriority(roles.priority) };
    (roles.members || []).forEach(function (m) {
      S.members[m.id] = { id: m.id, name: m.name || m.id, keys: (m.keys || []).slice(), revoked: {}, leads: (m.leads || []).slice(), removed: null, joined: null };
    });
    (roles.admins || []).forEach(function (a) { S.admins[a] = true; });
    return S;
  }

  // Assignment records. state: pending -> live (accept) | declined | withdrawn;
  // live -> unassigned | returned | handed (a handoff was accepted).
  function recsOn(S, item) { return vals(S.asg).filter(function (r) { return r.item === item; }); }
  function liveRecs(S, item) { return recsOn(S, item).filter(function (r) { return r.state === 'live'; }); }
  function openRecs(S, item) { return recsOn(S, item).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }); }
  function liveRec(S, item, who) { return liveRecs(S, item).filter(function (r) { return r.to === who; })[0] || null; }
  function pendingRec(S, item, who) { return recsOn(S, item).filter(function (r) { return r.to === who && r.state === 'pending'; })[0] || null; }
  function openRec(S, item, who) { return openRecs(S, item).filter(function (r) { return r.to === who; })[0] || null; }
  function lastEndedRec(S, item, who) {
    var rs = recsOn(S, item).filter(function (r) { return r.to === who && r.ended; });
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
  // (208, 292) An admin stands in for a removed creator: assign, unassign, edit, hide.
  function creatorGone(S, it, a) { var cr = S.members[it.author]; return !!(S.admins[a] && cr && cr.removed); }
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
  function checkFields(S, p) {
    if (has(p, 'title') && !p.title) rej('title cannot be empty');
    if (p.project != null && typeof p.project !== 'string') rej('project is one optional free-text string');
    if (p.due != null && !(typeof p.due === 'string' && DUE_RE.test(p.due) && !isNaN(Date.parse(p.due)))) rej('due is an optional YYYY-MM-DD date');
    if (p.priority != null && !(Number.isInteger(p.priority) && p.priority >= 0 && p.priority < S.prio.names.length))
      rej('priority must be a level 0..' + (S.prio.names.length - 1) + ' (' + S.prio.names.join(' / ') + ')');
  }
  function notAssignee(S, it, who) {
    if (pendingRec(S, it.id, who)) return who + ' has not accepted the pending assignment on ' + it.alias;
    var gone = lastEndedRec(S, it.id, who);
    if (gone) return who + '\'s assignment on ' + it.alias + ' ended (' + gone.state + ', ' + alias(gone.ended.id) + ')';
    if (it.author === who) return who + ' created ' + it.alias + ' but is not assigned to it (self-assign first)';
    return who + ' is not an assignee of ' + it.alias;
  }
  function needRec(S, item, who) {
    var r = liveRec(S, item, who);
    if (!r) rej(notAssignee(S, S.items[item], who));
    return r;
  }

  function check(S, n) {
    var p = n.p, a = n.author, c = n.commit, m = S.members[a], it, r, q;
    if (!m) rej('author "' + a + '" is not a member in the state its author had seen');
    if (m.removed) rej(a + ' was removed from the store (' + alias(m.removed) + ')');
    if (!c.signer) rej('commit is unsigned');
    if (m.revoked[c.signer]) rej('signed with ' + c.signer + ', already revoked in its own causal past (' + alias(m.revoked[c.signer]) + ')');
    if (m.keys.indexOf(c.signer) < 0) rej('signer key "' + c.signer + '" is not a key of ' + a);
    if (ORIGINS.indexOf(n.origin) < 0) rej('origin must be human, agent or agent-auto');
    if (n.origin === 'agent-auto' && /^item\.(accept|decline|return|handoff)$/.test(n.type))
      rej('an agent never accepts, declines, returns or hands off an assignment on its own (agent-auto is invalid here)');
    if (n.origin === 'agent-auto' && n.type !== 'item.status' && n.type !== 'item.close')
      rej('agent-auto is limited to item.status and item.close on the author\'s own assignment');

    switch (n.type) {
      case 'item.create':
        if (!p.title) rej('title required');
        if (KINDS.indexOf(p.kind) < 0) rej('kind must be task, todo or topic');
        checkFields(S, p);
        if (p.parent) {
          var par = needItem(S, p.parent, 'parent');
          if (par.hidden) rej('parent ' + par.alias + ' is hidden');
          if (par.author !== a && !liveRec(S, p.parent, a)) rej('parent ' + par.alias + ' is neither assigned to nor created by ' + a);
        }
        break;

      case 'item.edit':
        it = needItem(S, p.item);
        if (it.author !== a && !creatorGone(S, it, a)) rej('only the creator (' + it.author + ') edits ' + it.alias);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (!ITEM_FIELDS.some(function (f) { return has(p, f); })) rej('nothing to edit (' + ITEM_FIELDS.join(', ') + ')');
        if (has(p, 'priority') && p.priority == null) rej('priority cannot be cleared; set a level');
        checkFields(S, p);
        break;

      case 'item.hide': {
        it = needItem(S, p.item);
        if (it.author !== a && !creatorGone(S, it, a)) rej('only the creator (' + it.author + ') hides ' + it.alias);
        if (it.hidden) rej(it.alias + ' is already hidden');
        var open = openRecs(S, it.id).filter(function (x) { return x.to !== a && x.status !== 'done'; });
        if (open.length) rej('unassign the open assignments first (' + open.map(function (x) { return x.to; }).join(', ') + ')');
        break;
      }

      case 'item.assign': {
        it = needItem(S, p.item);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (it.kind === 'topic') rej('topics have no assignment');
        if (it.author !== a && !creatorGone(S, it, a))
          rej('only the creator (' + it.author + ') assigns ' + it.alias + '; an assignee hands off their own assignment with item.handoff');
        var completion = p.completion || 'assignee';
        if (completion !== 'assignee' && completion !== 'assigner') rej('completion must be assignee or assigner');
        if (p.mode === 'all') { // live and forced-only: everyone within N levels below the author
          var levels = Number(p.levels || 0);
          if (!(levels >= 1)) rej('levels must be >= 1');
          if (!Object.keys(reach(S.members, a, levels)).length) rej('nobody is within ' + levels + ' levels below ' + a);
          var cov = liveRules(S, it.id).filter(function (x) { return x.by === a && x.levels >= levels; })[0];
          if (cov) rej('already assigned to everyone within ' + cov.levels + ' levels (' + alias(cov.id) + ')');
        } else if (p.mode === 'chosen') { // anyone: below the author = forced (live), anyone else = pending
          var ppl = p.people || [];
          if (!ppl.length) rej('choose at least one person');
          ppl.forEach(function (x) { if (x !== a && (!S.members[x] || S.members[x].removed)) rej(x + ' is not a member'); });
          if (!ppl.some(function (x) { return !openRec(S, it.id, x); })) rej('everyone chosen already holds a live or pending assignment');
        } else rej('mode must be all or chosen');
        break;
      }

      case 'item.handoff': {
        it = needItem(S, p.item);
        if (it.hidden) rej(it.alias + ' is hidden');
        var from = p.assignee || a; // (286) an admin may hand off any member's live assignment
        if (from !== a && !S.admins[a]) rej('only the assignee (' + from + ') hands off their assignment; an admin may hand off any member\'s');
        r = needRec(S, it.id, from);
        if (r.status === 'done') rej('assignment is done; nothing to hand off');
        if (!p.to) rej('to required');
        if (p.to === from) rej('hand off to someone else');
        needMember(S, p.to);
        if (openRec(S, it.id, p.to)) rej(p.to + ' already holds a live or pending assignment on ' + it.alias);
        var pend = vals(S.asg).filter(function (x) { return x.handoffOf === r.id && x.state === 'pending'; })[0];
        if (pend) rej('a handoff of this assignment to ' + pend.to + ' is already pending (' + alias(pend.via) + ')');
        break;
      }

      case 'item.accept':
      case 'item.decline':
        it = needItem(S, p.item);
        r = pendingRec(S, it.id, a);
        if (!r) rej(liveRec(S, it.id, a) ? a + '\'s assignment on ' + it.alias + ' is already live' : a + ' has no pending assignment on ' + it.alias);
        break;

      case 'item.return': {
        it = needItem(S, p.item);
        r = liveRec(S, it.id, p.assignee);
        if (!r) rej((p.assignee || '?') + ' has no live assignment on ' + it.alias);
        var lead = ((S.members[r.to] || {}).leads || [])[0];
        if (lead !== a) rej('only ' + r.to + '\'s direct lead (' + (lead || 'none: ' + r.to + ' is a root') + ') returns an accepted assignment');
        if (!r.accepted) rej(r.to + '\'s assignment was forced by ' + r.by + ' (their lead chain); only an accepted one is returnable');
        break;
      }

      case 'item.unassign':
        it = needItem(S, p.item);
        if (p.assignee) {
          r = openRec(S, it.id, p.assignee);
          if (!r) rej(p.assignee + ' has no live or pending assignment on ' + it.alias);
          var departed = (S.members[r.to] || {}).removed || (S.members[r.by] || {}).removed;
          if (r.by !== a && !(r.state === 'pending' && r.sender === a) && !(S.admins[a] && departed))
            rej('only the assigner (' + r.by + ') unassigns' + (r.state === 'pending' && r.sender !== r.by ? ' (or ' + r.sender + ' withdraws)' : ''));
        } else if (!openRecs(S, it.id).some(function (x) { return x.by === a || (x.state === 'pending' && x.sender === a); }) &&
          !liveRules(S, it.id).some(function (x) { return x.by === a; }))
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
        r = liveRec(S, it.id, p.assignee);
        if (!r) rej('no live assignment of ' + it.alias + ' to ' + p.assignee);
        if (r.by !== a) rej('only the assigner (' + r.by + ') ' + (n.type === 'item.confirm' ? 'confirms' : 'rejects'));
        if (r.completion !== 'assigner') rej('this assignment does not require confirmation');
        if (r.status !== 'awaiting') rej('assignment is ' + r.status + ', not awaiting confirmation');
        break;

      case 'item.reopen':
        it = needItem(S, p.item);
        r = liveRec(S, it.id, p.assignee || a);
        if (!r) rej(notAssignee(S, it, p.assignee || a));
        if (r.by !== a && r.to !== a) rej('only the assigner (' + r.by + ') or the assignee (' + r.to + ') reopens');
        if (r.status !== 'done') rej('assignment is ' + r.status + ', not done');
        break;

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
        if (String(nm.id).indexOf('.') >= 0) rej('member ids cannot contain "." (it separates member and clone in a stream id)');
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
      case 'roles.set_priority_levels': {
        needAdmin(S, a);
        var names = p.names;
        if (!Array.isArray(names) || !names.length) rej('names must be a non-empty list (lowest to highest)');
        if (names.length > 9) rej('at most 9 priority levels');
        names.forEach(function (x) { if (typeof x !== 'string' || !x.trim()) rej('every level needs a display name'); });
        if (p.default == null || !Number.isInteger(p.default) || p.default < 0 || p.default >= names.length) rej('default must be a level 0..' + (names.length - 1));
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
    if (v === null || v === undefined) return '(none)';
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      if (v.names) return v.names.join(' / ') + ' (default ' + v.names[v.default] + ')';
      return v.status + (v.note ? ' ("' + v.note + '")' : '');
    }
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
  // winner (later ts, then member, clone, seq) and every head stays in history.
  // Concurrent writes by one member from two of their clones get no marker (271):
  // the later ts wins, except that a human-origin write beats a concurrent agent or
  // agent-auto write of the same member (288). The losing write stays in the thread
  // with a quiet "superseded" note (289), never a conflict box.
  function winner(heads) { // heads are pairwise concurrent
    var cand = heads.filter(function (h) {
      return h.n.origin === 'human' || !heads.some(function (o) { return o.n.author === h.n.author && o.n.origin === 'human'; });
    });
    return cand.reduce(function (b, h) { return !b || later(h.n, b.n) ? h : b; }, null);
  }
  function supersede(n, label, by, why) {
    n.superseded = (n.superseded || []).filter(function (x) { return x.field !== label; });
    if (by) n.superseded.push({ field: label, by: by.id, why: why });
  }
  function write(S, key, label, n, v, C) {
    var heads = (S.reg[key] || []).filter(function (h) { return !hb(h.n, n); }), conc = heads.slice();
    heads.push({ n: n, v: v });
    S.reg[key] = heads;
    var w = winner(heads);
    if (C) {
      conc.forEach(function (h) {
        if (h.n.author === n.author) return;
        mark(h.n, n, 'field', label, { winner: w.n.id, mine: show(h.v), theirs: show(v) });
        mark(n, h.n, 'field', label, { winner: w.n.id, mine: show(v), theirs: show(h.v) });
      });
      heads.forEach(function (h) {
        if (h === w || h.n.author !== w.n.author) supersede(h.n, label, null);
        else supersede(h.n, label, w.n, later(h.n, w.n) ? 'human' : 'later');
      });
    }
    return w.v;
  }

  function addRec(S, via, item, to, by, completion, extra) {
    var id = via.id + '>' + to;
    if (S.asg[id]) return null;
    var r = { id: id, item: item, to: to, by: by, sender: by, completion: completion, via: via.id, ts: via.ts,
      state: 'live', forced: true, accepted: null, leadAtAccept: null, ended: null, handoffOf: null, answerNote: null,
      status: 'todo', note: null, noteBy: null, last: via, writes: [], inherited: null };
    if (extra) Object.keys(extra).forEach(function (k) { r[k] = extra[k]; });
    S.asg[id] = r;
    return r;
  }

  // Live "everyone within N": whoever enters the reach later inherits the assignment.
  function refreshLive(S, n) {
    vals(S.rules).forEach(function (rule) {
      if (rule.ended) return;
      Object.keys(reach(S.members, rule.by, rule.levels)).forEach(function (t) {
        if (rule.excluded[t] || openRec(S, rule.item, t) || S.asg[rule.id + '>' + t]) return;
        addRec(S, { id: rule.id, ts: n.ts }, rule.item, t, rule.by, rule.completion, { inherited: n.id, last: n });
      });
    });
  }

  // End a record. Unassign, return, withdraw and an accepted handoff beat any
  // concurrent status write on it: the writes are marked, valid but not in effect.
  // (295) A pending handoff of the record ends with it, as withdrawn, pointing at
  // the same event: there is nothing left to hand on.
  function endRec(S, r, n, state, C) {
    r.state = state; r.ended = n;
    vals(S.asg).forEach(function (x) {
      if (x.handoffOf === r.id && x.state === 'pending') { x.state = 'withdrawn'; x.ended = n; }
    });
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
  // A write folded onto a record a concurrent event already ended.
  function onEnded(S, item, who, n, C) {
    var gone = lastEndedRec(S, item, who);
    if (!gone) return;
    gone.writes.push(n);
    if (C && !hb(gone.ended, n)) {
      var lb = who + '\'s assignment on ' + alias(item);
      mark(n, gone.ended, 'removed', lb, { winner: gone.ended.id });
      mark(gone.ended, n, 'removed', lb, { winner: gone.ended.id });
    }
  }

  // (295) An unassign, return, or removal of a record that is concurrent with the
  // accept that handed it on wins, as removal beats writes (213): the accepted
  // handoff ends withdrawn (the accept and its holder's writes are marked, valid
  // but not in effect), and the record comes back live for the ending event to end.
  function handedConc(r, n) { return r.state === 'handed' && !!r.ended && r.ended.type === 'item.accept' && !hb(r.ended, n); }
  function handedRec(S, item, who, n) {
    var rs = recsOn(S, item).filter(function (r) { return r.to === who && handedConc(r, n); });
    return rs[rs.length - 1] || null;
  }
  function reverseHanded(S, q, n, C) {
    var acc = q.ended;
    var h = vals(S.asg).filter(function (x) { return x.handoffOf === q.id && x.accepted === acc; })[0];
    if (h && h.state === 'live') { h.writes.push(acc); endRec(S, h, n, 'withdrawn', C); }
    q.state = 'live'; q.ended = null;
    return q;
  }

  function apply(S, n, C) {
    var p = n.p, a = n.author, it, q, m, r;
    switch (n.type) {
      case 'item.create':
        S.items[n.id] = { id: n.id, alias: n.alias, kind: p.kind, title: p.title, body: p.body || '', project: p.project || '',
          due: p.due || null, priority: p.priority != null ? p.priority : S.prio.default,
          parent: p.parent || null, author: a, ts: n.ts, hidden: false, edited: null, node: n };
        break;
      case 'item.edit':
        it = S.items[p.item]; if (!it) break;
        ITEM_FIELDS.forEach(function (f) {
          if (has(p, f)) it[f] = write(S, 'it:' + it.id + ':' + f, f + ' of ' + it.alias, n, p[f] == null ? (f === 'due' ? null : '') : p[f], C);
        });
        it.edited = n;
        break;
      case 'item.hide':
        it = S.items[p.item]; if (!it) break;
        it.hidden = write(S, 'it:' + it.id + ':hidden', it.alias + ' hidden', n, true, C);
        break;
      case 'item.assign': {
        var completion = p.completion || 'assignee';
        if (!S.items[p.item]) break;
        if (p.mode === 'all') {
          Object.keys(reach(S.members, a, Number(p.levels))).forEach(function (t) {
            if (!openRec(S, p.item, t)) addRec(S, n, p.item, t, a, completion);
          });
          S.rules[n.id] = { id: n.id, item: p.item, by: a, levels: Number(p.levels), completion: completion, excluded: {}, ended: null };
        } else (p.people || []).forEach(function (t) {
          if (!S.members[t] || openRec(S, p.item, t)) return;
          var forced = t === a || below(S.members, a, t);
          addRec(S, n, p.item, t, a, completion, { forced: forced, state: forced ? 'live' : 'pending' });
        });
        break;
      }
      case 'item.handoff':
        r = liveRec(S, p.item, p.assignee || a);
        if (!r || !S.members[p.to] || openRec(S, p.item, p.to)) break;
        addRec(S, n, p.item, p.to, r.by, r.completion, { forced: false, state: 'pending', sender: a, handoffOf: r.id, answerNote: null, handoffNote: p.note || null });
        break;
      case 'item.accept':
        r = pendingRec(S, p.item, a);
        if (!r) { onEnded(S, p.item, a, n, C); break; }
        r.state = 'live'; r.accepted = n; r.answerNote = p.note || null; r.last = n;
        r.leadAtAccept = ((S.members[a] || {}).leads || [])[0] || null;
        q = r.handoffOf && S.asg[r.handoffOf];
        if (q && q.state === 'live') { // (290) the new assignment starts with the original's status and note
          endRec(S, q, n, 'handed', C);
          r.status = q.status; r.note = q.note; r.noteBy = q.noteBy;
        }
        break;
      case 'item.decline':
        r = pendingRec(S, p.item, a);
        if (r) { r.state = 'declined'; r.ended = n; r.answerNote = p.note || null; }
        break;
      case 'item.return':
        r = liveRec(S, p.item, p.assignee);
        if (!r) { q = handedRec(S, p.item, p.assignee, n); r = q && reverseHanded(S, q, n, C); }
        if (!r) break;
        endRec(S, r, n, 'returned', C); r.answerNote = p.note || null;
        // (291) returning an accepted handoff restores the original assignment with
        // its prior status, unless its holder was removed or holds another one now
        q = r.handoffOf && S.asg[r.handoffOf];
        if (q && q.state === 'handed' && q.ended === r.accepted && S.members[q.to] && !S.members[q.to].removed && !openRec(S, q.item, q.to)) {
          q.state = 'live'; q.ended = null; q.restored = n; r.restores = q.id;
        }
        break;
      case 'item.unassign':
        if (p.assignee) {
          r = openRec(S, p.item, p.assignee);
          if (!r) { q = handedRec(S, p.item, p.assignee, n); r = q && reverseHanded(S, q, n, C); }
          liveRules(S, p.item).forEach(function (rule) { if (rule.by === (r ? r.by : a)) rule.excluded[p.assignee] = true; });
          if (r) endRec(S, r, n, r.state === 'pending' ? 'withdrawn' : 'unassigned', C);
        } else {
          liveRules(S, p.item).forEach(function (rule) { if (rule.by === a) rule.ended = n.id; });
          recsOn(S, p.item).filter(function (x) { return x.by === a && handedConc(x, n); }).forEach(function (x) { reverseHanded(S, x, n, C); });
          openRecs(S, p.item).filter(function (x) { return x.by === a || (x.state === 'pending' && x.sender === a); })
            .forEach(function (x) { endRec(S, x, n, x.state === 'pending' ? 'withdrawn' : 'unassigned', C); });
        }
        break;
      case 'item.status': case 'item.close': case 'item.confirm': case 'item.reject': case 'item.reopen': {
        var who = (n.type === 'item.status' || n.type === 'item.close') ? a : (p.assignee || a);
        r = liveRec(S, p.item, who);
        if (!r) { onEnded(S, p.item, who, n, C); break; } // folded onto an assignment a concurrent event ended
        r.writes.push(n);
        var v = n.type === 'item.status' ? p.status : n.type === 'item.close' ? (r.completion === 'assigner' ? 'awaiting' : 'done') :
          n.type === 'item.confirm' ? 'done' : 'doing';
        var w = write(S, 'st:' + r.id, 'status of ' + who + ' on ' + alias(p.item), n, { status: v, note: p.note || null, by: a, n: n }, C);
        r.status = w.status; r.note = w.note; r.noteBy = w.by; r.last = w.n;
        break;
      }
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
      case 'roles.remove_member': // (287) the member's live assignments end as unassigned, pending ones as withdrawn
        m = S.members[p.member]; if (!m) break;
        m.removed = n.id; delete S.admins[p.member];
        vals(S.asg).forEach(function (x) { if (x.to === p.member && handedConc(x, n)) reverseHanded(S, x, n, C); });
        vals(S.asg).forEach(function (x) {
          if (x.to === p.member && (x.state === 'live' || x.state === 'pending')) endRec(S, x, n, x.state === 'pending' ? 'withdrawn' : 'unassigned', C);
        });
        break;
      case 'roles.set_priority_levels':
        S.prio = write(S, 'prio', 'priority levels', n, { names: (p.names || []).slice(), default: p.default }, C);
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
        var who = r.p.member, key = r.p.key;
        order.forEach(function (n) { // the cut is per stream: what the revoker had seen of each of that member's clones
          var cut = r.cv[n.stream] || 0;
          if (n.author !== who || n.commit.signer !== key || n.seq <= cut || ignored[n.id] || n.status !== 'applied') return;
          ignored[n.id] = 'signed with ' + key + ', which ' + r.alias + ' revoked having seen ' + n.stream + ' only up to seq ' + cut +
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

    return { S: S, members: S.members, admins: S.admins, items: S.items, asg: S.asg, rules: S.rules, comments: S.comments, prio: S.prio,
      log: log, byId: byId, children: childrenMap(S.members), heads: headSeqs(nodes) };
  }

  // Highest valid seq received per stream: the `seen` of a commit written now, and
  // (for the writer) the seq its next event continues from.
  function headSeqs(nodes) {
    var out = {};
    nodes.forEach(function (n) { if (n.causal && n.seq > (out[n.stream] || 0)) out[n.stream] = n.seq; });
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
      if (String(m.id).indexOf('.') >= 0) out.push(m.id + ': member ids cannot contain "."');
      if (l.length > 1) out.push(m.id + ': leads must have length at most 1 (got ' + l.length + ')');
      l.forEach(function (x) { if (!ids[x]) out.push(m.id + ': lead "' + x + '" is not a member'); });
      if (!m.keys || !m.keys.length) out.push(m.id + ': no keys');
    });
    if (roles.priority && (!Array.isArray(roles.priority.names) || !roles.priority.names.length)) out.push('priority.names must be a non-empty list');
    return out;
  }

  // ----------------------------------------------------------- derived state

  function itemStatus(g, it) {
    if (it.kind === 'topic') return null;
    var recs = recsOn(g.S, it.id), live = recs.filter(function (r) { return r.state === 'live'; });
    if (!live.length) {
      if (recs.some(function (r) { return r.state === 'pending'; })) return 'pending';
      var last = recs.filter(function (r) { return r.ended; }).sort(function (x, y) { return later(x.ended, y.ended) ? 1 : -1; }).pop();
      return last && (last.state === 'declined' || last.state === 'returned') ? last.state : 'unassigned';
    }
    var s = live.map(function (r) { return r.status; });
    if (s.every(function (x) { return x === 'done'; })) return 'done';
    if (s.indexOf('blocked') >= 0) return 'blocked';
    if (s.indexOf('awaiting') >= 0) return 'awaiting';
    if (s.some(function (x) { return x === 'doing' || x === 'done'; })) return 'doing';
    return 'todo';
  }
  // Pending assignments are excluded from status and roll-up: an item whose only
  // assignments are pending (or declined / returned) is not counted.
  function counted(g, it) {
    var s = itemStatus(g, it);
    return it.kind !== 'topic' && s !== 'pending' && s !== 'declined' && s !== 'returned';
  }
  function subtasksOf(g, id) { return vals(g.items).filter(function (i) { return i.parent === id && !i.hidden; }); }
  function rollup(g, it) {
    var kids = subtasksOf(g, it.id).filter(function (k) { return counted(g, k); }), counts = {}, blocked = [];
    kids.forEach(function (k) { var s = itemStatus(g, k); counts[s] = (counts[s] || 0) + 1; });
    (function walk(id, depth) {
      if (depth > 30) return;
      subtasksOf(g, id).forEach(function (k) {
        if (counted(g, k) && itemStatus(g, k) === 'blocked') blocked.push(k.id);
        walk(k.id, depth + 1);
      });
    })(it.id, 0);
    return { total: kids.length, counts: counts, blocked: blocked };
  }
  function daysBetween(a, b) { return (Date.parse(b) - Date.parse(a)) / 86400000; }

  // The one read path for a view: everything a viewer's page shows, as plain JSON
  // (no engine objects, no functions). `input` is what the viewer's clone holds.
  function deriveViewState(input, opts) {
    var g = replay(input), stream = opts.stream, v = memberOf(stream), now = opts.now || '', S = g.S;
    var staleDays = opts.staleDays || STALE_DAYS;
    var me = S.members[v] || null, R = reach(S.members, v, 99);

    var events = g.log.map(function (n) {
      return { id: n.id, alias: n.alias, stream: n.stream, member: n.author, seq: n.seq, ts: n.ts, type: n.type, origin: n.origin,
        payload: clone(n.p), status: n.status, reason: n.reason, ignored: !!n.revokedLate,
        markers: n.markers.map(function (m) {
          var o = g.byId[m.other];
          return { kind: m.kind, field: m.field, other: m.other, otherMember: o ? o.author : null, winner: m.winner, mine: m.mine || null, theirs: m.theirs || null };
        }),
        superseded: clone(n.superseded || []),
        thread: n.thread, parentThread: n.parentThread, local: !!n.commit._local, open: !!n.commit._open, late: !!n.commit._late };
    });

    var assignments = {};
    vals(S.asg).forEach(function (r) {
      var last = r.last ? r.last.ts : r.ts, live = r.state === 'live';
      assignments[r.id] = { id: r.id, item: r.item, to: r.to, by: r.by, sender: r.sender, completion: r.completion, state: r.state,
        forced: r.forced, status: r.status, note: r.note, noteBy: r.noteBy, via: r.via, ts: r.ts,
        inherited: !!r.inherited, ruleLevels: g.rules[r.via] ? g.rules[r.via].levels : null,
        handoffOf: r.handoffOf, handoffFrom: r.handoffOf && S.asg[r.handoffOf] ? S.asg[r.handoffOf].to : null,
        handoffNote: r.handoffNote || null, answerNote: r.answerNote,
        restored: r.restored ? { id: r.restored.id, ts: r.restored.ts } : null, restores: r.restores || null,
        accepted: r.accepted ? { id: r.accepted.id, ts: r.accepted.ts } : null, leadAtAccept: r.leadAtAccept,
        ended: r.ended ? { id: r.ended.id, type: r.ended.type, ts: r.ended.ts, by: r.ended.author } : null,
        depth: r.to === r.by ? 0 : (reach(S.members, r.by, 99)[r.to] || null),
        statusOrigin: r.last && r.writes.indexOf(r.last) >= 0 ? r.last.origin : null, statusBy: r.noteBy,
        lastUpdate: last,
        stale: !!(live && now && (r.status === 'doing' || r.status === 'blocked') && daysBetween(last, now) >= staleDays) };
    });

    var lastAct = {}, threads = {};
    g.log.forEach(function (n) {
      if (n.status === 'applied' && n.thread && String(n.ts) > (lastAct[n.thread] || '')) lastAct[n.thread] = String(n.ts);
      [n.thread, n.parentThread].forEach(function (t) { if (t) (threads[t] = threads[t] || []).push(n.id); });
    });
    var items = {};
    vals(S.items).forEach(function (it) {
      var recs = recsOn(S, it.id), d = 0, cur = it, chain = [];
      while (cur && d < 50) { chain.push(cur.author); cur = cur.parent && S.items[cur.parent]; d++; }
      var started = null;
      recs.forEach(function (r) { r.writes.forEach(function (w) { if (w.type === 'item.status' && w.p.status === 'doing' && (!started || String(w.ts) < started)) started = String(w.ts); }); });
      items[it.id] = { id: it.id, alias: it.alias, kind: it.kind, title: it.title, body: it.body, project: it.project,
        due: it.due, priority: it.priority, priorityLevel: priorityLevel(S.prio, it.priority), priorityName: S.prio.names[priorityLevel(S.prio, it.priority)],
        parent: it.parent && S.items[it.parent] ? it.parent : null, author: it.author, ts: it.ts, hidden: !!it.hidden,
        edited: it.edited ? { id: it.edited.id, ts: it.edited.ts } : null, status: itemStatus(g, it), counted: counted(g, it),
        rollup: rollup(g, it), lastActivity: lastAct[it.id] || it.ts, started: started,
        children: vals(S.items).filter(function (k) { return k.parent === it.id; }).map(function (k) { return k.id; }),
        assignments: recs.map(function (r) { return r.id; }),
        pending: recs.filter(function (r) { return r.state === 'pending'; }).map(function (r) { return r.id; }),
        thread: threads[it.id] || [], concurrent: (threads[it.id] || []).filter(function (id) { return g.byId[id].markers.length; }).length,
        depth: chain.length - 1, chain: chain.slice().reverse() };
    });
    var comments = {};
    vals(S.comments).forEach(function (q) {
      comments[q.id] = { id: q.id, item: q.item, author: q.author, body: q.body, reply_to: q.reply_to, ts: q.ts,
        edited: q.edited ? { id: q.edited.id, ts: q.edited.ts } : null, hidden: q.hiddenBy ? q.hiddenBy.id : null };
    });
    var itemOrder = [];
    (function walk(parent) {
      Object.keys(items).forEach(function (id) { if (items[id].parent === parent) { itemOrder.push(id); walk(id); } });
    })(null);

    var members = {};
    Object.keys(S.members).forEach(function (id) {
      var m = S.members[id];
      members[id] = { id: id, name: m.name, leads: m.leads.slice(), keys: m.keys.slice(),
        revoked: Object.keys(m.revoked).map(function (k) { return { key: k, by: m.revoked[k] }; }),
        removed: m.removed, joined: m.joined, admin: !!S.admins[id], children: (g.children[id] || []).slice(), depth: R[id] || null,
        role: (m.leads || []).length ? ((g.children[id] || []).length ? 'lead' : 'member') : 'root' };
    });

    // ---- viewer-specific lists
    var A = vals(assignments), RANK = { blocked: 0, doing: 1, todo: 2, awaiting: 3, done: 4 };
    function visible(r) { return items[r.item] && !items[r.item].hidden; }
    function byRank(x, y) { return RANK[x.status] - RANK[y.status]; }
    function byUrgency(x, y) { // priority desc, then due asc (none last), then status
      var ix = items[x.item], iy = items[y.item];
      if (ix.priorityLevel !== iy.priorityLevel) return iy.priorityLevel - ix.priorityLevel;
      var dx = ix.due || '9999', dy = iy.due || '9999';
      return dx < dy ? -1 : dx > dy ? 1 : byRank(x, y);
    }
    var ids = function (l) { return l.map(function (r) { return r.id; }); };
    var mine = A.filter(function (r) { return r.to === v && r.state === 'live' && visible(r); });
    var pendingToMe = A.filter(function (r) { return r.to === v && r.state === 'pending' && visible(r); });
    var answered = A.filter(function (r) { return r.to === v && !r.forced && r.state !== 'pending' && (r.accepted || r.state === 'declined') && visible(r); });
    var sent = A.filter(function (r) { return r.sender === v && r.to !== v && !r.forced && visible(r); });
    var confirms = A.filter(function (r) { return r.by === v && r.to !== v && r.state === 'live' && r.status === 'awaiting'; });
    var given = Object.keys(items).filter(function (id) {
      return !items[id].hidden && A.some(function (r) { return r.item === id && (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); });
    });
    var created = Object.keys(items).filter(function (id) {
      var it = items[id];
      return it.author === v && !it.hidden && it.kind !== 'topic' && !A.some(function (r) { return r.item === id && (r.state === 'live' || r.state === 'pending'); });
    });

    // Notices are derived, never events. accepted: to the accepter's lead at accept
    // time; handoff: to the original assigner when a handoff is accepted; declined /
    // returned (285): to the sender, and for a returned handoff also to the original
    // assigner (291).
    var notices = [];
    A.forEach(function (r) {
      var base = { assignment: r.id, item: r.item, to: r.to, by: r.by, sender: r.sender, state: r.state };
      if (r.accepted) {
        var b1 = Object.assign({ ts: r.accepted.ts }, base);
        if (r.leadAtAccept === v) notices.push(Object.assign({ key: 'acc:' + r.id, kind: 'accepted' }, b1));
        else if (r.handoffOf && assignments[r.handoffOf] && assignments[r.handoffOf].by === v) notices.push(Object.assign({ key: 'ho:' + r.id, kind: 'handoff', from: r.handoffFrom }, b1));
      }
      if (r.to === v || !r.ended) return;
      var b2 = Object.assign({ ts: r.ended.ts, endedBy: r.ended.by, note: r.answerNote, from: r.handoffFrom, restores: r.restores }, base);
      if (r.state === 'declined' && r.sender === v) notices.push(Object.assign({ key: 'dec:' + r.id, kind: 'declined' }, b2));
      if (r.state === 'returned' && (r.sender === v || (r.handoffOf && r.by === v)) && r.ended.by !== v) notices.push(Object.assign({ key: 'ret:' + r.id, kind: 'returned' }, b2));
    });

    var R3 = reach(S.members, v, 3), team = [];
    (function walk(lead) {
      (g.children[lead] || []).forEach(function (id) {
        if (!(id in R3)) return;
        var recs = A.filter(function (r) { return r.to === id && visible(r); });
        var open = recs.filter(function (r) { return r.state === 'live' && r.status !== 'done'; }).sort(byRank);
        var pend = recs.filter(function (r) { return r.state === 'pending'; });
        var waiting = A.filter(function (r) { return r.sender === id && r.to !== id && r.state === 'pending' && visible(r); });
        team.push({ id: id, depth: R3[id], open: ids(open), pending: ids(pend), waiting: ids(waiting),
          done: recs.filter(function (r) { return r.state === 'live' && r.status === 'done'; }).length,
          blocked: open.filter(function (r) { return r.status === 'blocked'; }).length,
          stale: open.filter(function (r) { return r.stale; }).length,
          last: recs.map(function (r) { return r.lastUpdate; }).sort().pop() || null });
        if (R3[id] < 3) walk(id);
      });
    })(v);

    var badge = [];
    mine.forEach(function (r) { if (r.by !== v && r.forced && r.status === 'todo') badge.push('a:' + r.id); });
    pendingToMe.forEach(function (r) { badge.push('p:' + r.id); });
    notices.forEach(function (x) { badge.push(x.key); });

    var commits = 0;
    Object.keys(input.commits || {}).forEach(function (k) { commits += input.commits[k].length; });

    return {
      viewer: { member: v, stream: stream, name: me ? me.name : v, admin: !!S.admins[v], lead: me ? (me.leads || [])[0] || null : null,
        followers: (g.children[v] || []).slice(), reach: R, isMember: !!(me && !me.removed) },
      priority: clone(S.prio),
      members: members, admins: Object.keys(S.admins), rolesProblems: rolesProblems(input.roles),
      items: items, itemOrder: itemOrder, assignments: assignments, comments: comments, events: events, notices: notices,
      my: {
        assignments: ids(mine.sort(byRank)),
        today: { work: ids(mine.filter(function (r) { return r.status !== 'done' && r.status !== 'awaiting'; }).sort(byUrgency)), pending: ids(pendingToMe.sort(byUrgency)), confirms: ids(confirms) },
        received: { pending: ids(pendingToMe), answered: ids(answered), sent: ids(sent) },
        given: given, created: created, team: team, badge: badge
      },
      stats: { events: events.length, commits: commits,
        flagged: events.filter(function (e) { return e.status !== 'applied'; }).length,
        concurrent: events.filter(function (e) { return e.markers.length; }).length },
      heads: clone(g.heads), now: now
    };
  }

  // ------------------------------------------------------- store (protocol mock)
  // Remote = pushed commits per stream. Each clone holds what it fetched (`have`,
  // a commit count per stream), its own unpushed commits (`local`), and the open
  // batch (the last local commit with _open). Writes batch in the open batch; it
  // closes into one commit when the clone's `seen` would change (it receives new
  // events) or at push (227). The view of a clone is a replay of what it holds.

  function openStore(def) {
    var state;
    var hasRemote = !!(def.sync && def.sync.remote);

    function reset() {
      state = { roles: clone(def.roles), remote: {}, clones: {} };
      var streams = {};
      [def.commits, def.late, def.local].forEach(function (o) { Object.keys(o || {}).forEach(function (s) { streams[s] = true; }); });
      (def.clones || []).forEach(function (s) { streams[s] = true; });
      (def.roles.members || []).forEach(function (m) {
        if (!Object.keys(streams).some(function (s) { return memberOf(s) === m.id; })) streams[m.id + '.pc'] = true;
      });
      Object.keys(streams).forEach(function (s) {
        state.clones[s] = { online: (def.offline || []).indexOf(s) < 0, syncedAt: (def.sync && def.sync.syncedAt) || null, have: {}, late: {}, local: [] };
      });
      if (hasRemote) {
        Object.keys(def.commits || {}).forEach(function (s) { state.remote[s] = clone(def.commits[s]); });
        Object.keys(state.clones).forEach(function (c) {
          Object.keys(state.remote).forEach(function (s) { state.clones[c].have[s] = state.remote[s].length; });
        });
        // late: pushed by their writer, fetched by no other clone yet
        Object.keys(def.late || {}).forEach(function (s) {
          state.remote[s] = (state.remote[s] || []).concat(clone(def.late[s]));
          state.clones[s].have[s] = state.remote[s].length;
        });
      } else {
        Object.keys(def.commits || {}).forEach(function (s) { state.clones[s].local = clone(def.commits[s]); });
      }
      Object.keys(def.local || {}).forEach(function (s) { state.clones[s].local = state.clones[s].local.concat(clone(def.local[s])); });
    }

    function input(s) {
      var c = state.clones[s], commits = {};
      Object.keys(state.remote).forEach(function (ref) {
        var n = ref === s ? state.remote[ref].length : (c.have[ref] || 0);
        commits[ref] = state.remote[ref].slice(0, n).map(function (cm, i) { return c.late[ref + '/' + i] ? Object.assign({}, cm, { _late: true }) : cm; });
      });
      if (c.local.length) commits[s] = (commits[s] || []).concat(c.local.map(function (cm) { return Object.assign({}, cm, { _local: true }); }));
      return { roles: state.roles, commits: commits };
    }
    function seenFor(g, s) {
      var seen = {};
      Object.keys(g.heads).sort().forEach(function (k) { if (k !== s) seen[k] = g.heads[k]; });
      return seen;
    }
    function closeBatch(c, ts) {
      var last = c.local[c.local.length - 1];
      if (last && last._open) { delete last._open; last.ts = ts || last.ts; }
    }
    function upstream(s) {
      var c = state.clones[s], n = 0;
      Object.keys(state.remote).forEach(function (ref) { if (ref !== s) n += Math.max(0, state.remote[ref].length - (c.have[ref] || 0)); });
      return n;
    }
    function receive(s) {
      var c = state.clones[s], got = 0;
      Object.keys(state.remote).forEach(function (ref) {
        if (ref === s) return;
        var all = state.remote[ref];
        for (var i = c.have[ref] || 0; i < all.length; i++) {
          // "arrived late": written before this clone's previous sync, received only now
          if (c.syncedAt && String(all[i].ts) < String(c.syncedAt)) c.late[ref + '/' + i] = true;
          got++;
        }
        c.have[ref] = all.length;
      });
      return got;
    }
    function need(s) { if (!state.clones[s]) throw new Error('unknown clone stream "' + s + '"'); return state.clones[s]; }

    var api = {
      name: def.name, label: def.label, remote: hasRemote ? def.sync.remote : null,
      reset: reset,
      streams: function () { return Object.keys(state.clones); },
      clonesOf: function (member) { return Object.keys(state.clones).filter(function (s) { return memberOf(s) === member; }); },
      nextEventId: function (s) { need(s); return s + '#' + ((replay(input(s)).heads[s] || 0) + 1); },
      // The only write path: append events to the clone's open batch. events =
      // [{ type, payload, origin? }]; ts = the write time (every event keeps its own).
      dispatch: function (s, evs, ts) {
        var c = need(s), g = replay(input(s)), seen = seenFor(g, s), seq = g.heads[s] || 0;
        var last = c.local[c.local.length - 1];
        if (last && last._open && JSON.stringify(last.seen) !== JSON.stringify(seen)) { closeBatch(c, ts); last = null; }
        if (!last || !last._open) {
          var m = g.members[memberOf(s)];
          last = { stream: s, ts: ts, signer: (m && m.keys[0]) || (m && Object.keys(m.revoked)[0]) || 'unsigned', seen: seen, events: [], _open: true };
          c.local.push(last);
        }
        var out = evs.map(function (e) {
          var ev = { seq: ++seq, ts: ts, type: e.type };
          if (e.origin && e.origin !== 'human') ev.origin = e.origin;
          ev.payload = clone(e.payload || {});
          last.events.push(ev);
          return s + '#' + ev.seq;
        });
        var g2 = replay(input(s));
        return out.map(function (id) { var n = g2.byId[id]; return { id: id, alias: alias(id), type: n.type, status: n.status, reason: n.reason }; });
      },
      // Apply path: close the batch, push, fetch. No remote = close only (same path as a long offline).
      sync: function (s, now) {
        var c = need(s);
        closeBatch(c, now);
        if (!hasRemote) return { noRemote: true, pushed: 0, fetched: 0, unpushed: c.local.length };
        if (!c.online) return { offline: true, pushed: 0, fetched: 0, unpushed: c.local.length };
        var pushed = c.local.length;
        c.local.forEach(function (cm) { delete cm._open; (state.remote[s] = state.remote[s] || []).push(cm); });
        c.local = [];
        var got = receive(s);
        c.syncedAt = now;
        return { pushed: pushed, fetched: got, unpushed: 0 };
      },
      // Receive only (drawer open, poll): closes the open batch first when new events arrive.
      fetch: function (s, now) {
        var c = need(s);
        if (!hasRemote || !c.online) return { fetched: 0 };
        if (upstream(s)) closeBatch(c, now);
        var got = receive(s);
        c.syncedAt = now;
        return { fetched: got };
      },
      setOnline: function (s, on) { need(s).online = !!on; },
      view: function (s, now) {
        var c = need(s), vs = deriveViewState(input(s), { stream: s, now: now });
        vs.store = { name: def.name, label: def.label, remote: hasRemote ? def.sync.remote : null };
        vs.sync = { online: c.online, syncedAt: c.syncedAt, upstream: hasRemote ? upstream(s) : 0,
          unpushed: c.local.map(function (cm) { return { open: !!cm._open, ts: cm.ts, events: cm.events.map(function (e) { return s + '#' + e.seq; }) }; }) };
        vs.clones = {};
        Object.keys(state.clones).forEach(function (k) { (vs.clones[memberOf(k)] = vs.clones[memberOf(k)] || []).push({ stream: k, online: state.clones[k].online, unpushed: state.clones[k].local.length }); });
        return vs;
      },
      raw: function () { return clone(state); },
      setRaw: function (r) {
        if (!r || !r.roles || !Array.isArray(r.roles.members)) throw new Error('roles file needs a "members" list');
        state = { roles: r.roles, remote: r.remote || {}, clones: r.clones || {} };
        Object.keys(state.remote).concat(Object.keys(state.clones)).forEach(function (s) {
          if (!state.clones[s]) state.clones[s] = { online: true, syncedAt: null, have: {}, late: {}, local: [] };
        });
      }
    };
    reset();
    return api;
  }

  window.TrackerEngine = {
    KINDS: KINDS, SETTABLE: SETTABLE, ORIGINS: ORIGINS, DEFAULT_PRIORITY: DEFAULT_PRIORITY, STALE_DAYS: STALE_DAYS,
    alias: alias, memberOf: memberOf, cloneOf: cloneOf, refOf: refOf, reach: reach,
    openStore: openStore, deriveViewState: deriveViewState
  };
})();
