// Tracker mockup v3 data layer: validate and fold the per-clone commit lists on top
// of the roles file, and simulate the per-clone journal and sync. No DOM here;
// the view is the shared renderer tracker.js, fed through mock-adapter.js.
// Exposes window.TrackerEngine. The rules are the ones settled on the board
// (README.md), and this file is the reference the Go port follows.
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
  var SETTABLE = ['idea', 'todo', 'doing', 'blocked'];
  var ORIGINS = ['human', 'agent', 'agent-auto'];
  // stage is retired: only older events carry it (see apply), so EDIT_FIELDS, the
  // list a refusal names, leaves it out.
  var ITEM_FIELDS = ['title', 'body', 'project', 'due', 'priority', 'labels', 'stage'], EDIT_FIELDS = ITEM_FIELDS.slice(0, 6);
  // The store's label list (admin-only roles events); a label's id is its add
  // event's id, and an item's labels field is a list of those ids.
  var LABEL_LIST = ['roles.add_label', 'roles.edit_label', 'roles.remove_label'], COLOR_RE = /^#[0-9a-f]{6}$/;
  var STAGE_SHAPE = 'stage must be review (null or absent: todo)';
  // Priority levels are store configuration (roles file, roles.set_priority_levels).
  // Codes are integers 0..n-1, larger = more urgent; names are display names only.
  var DEFAULT_PRIORITY = { names: ['Notice', 'Task', 'Urgent'], default: 1 };
  // Stale (284): a live assignment in doing or blocked with no status write for this
  // many days by the viewer's clock. A client-side constant, not shared data; a
  // view may pass its own as deriveViewState(input, {staleDays}).
  var STALE_DAYS = 3;
  var DUE_RE = /^\d{4}-\d{2}-\d{2}$/;
  // Attachments (337)-(339): only metadata is modelled. `oid` is the sha256 of the
  // content; the store limit is the roles setting attachments.max_bytes (default
  // 10 MiB), changed by roles.set_attachment_limit.
  var DEFAULT_ATTACHMENT_MAX = 10 * 1024 * 1024;
  var OID_RE = /^[0-9a-f]{64}$/;

  // Member profile ((391)-(393)): `org` (one free-text string, may be empty), `family`
  // (may be empty: a single-name person) and `given` (required). The display name and
  // the avatar are derived from it; neither is stored. A name with Hangul, a CJK
  // ideograph or kana reads family + given with no space; any other reads
  // `given family`. These two functions and the Go port's must agree.
  var PROFILE_FIELDS = ['org', 'family', 'given'];
  var PROFILE_LABEL = { org: 'org', family: 'family name', given: 'given name' };
  var CJK_RE = /[\p{Script=Han}\p{Script=Hangul}\p{Script=Hiragana}\p{Script=Katakana}]/u;
  function pstr(x) { return typeof x === 'string' ? x.trim() : ''; }
  // The stored profile of a roles-file member or a join payload. A member written
  // before profiles (a bare `name`, or nothing) keeps one given name: the name, else the id.
  function normProfile(m, id) {
    var pr = m && m.profile && typeof m.profile === 'object' && !Array.isArray(m.profile) ? m.profile : null;
    if (pr && pstr(pr.given)) return { org: pstr(pr.org), family: pstr(pr.family), given: pstr(pr.given) };
    return { org: pr ? pstr(pr.org) : '', family: pr ? pstr(pr.family) : '', given: pstr(m && m.name) || id };
  }
  function isCJKName(pr) { return CJK_RE.test(pr.family + pr.given); }
  function displayName(pr) {
    if (!pr.family) return pr.given;
    return isCJKName(pr) ? pr.family + pr.given : pr.given + ' ' + pr.family;
  }
  // CJK: the given name, up to two characters; else first letter of given + first
  // letter of family, upper-cased; a single name: its first two letters.
  function avatar(pr) {
    var g = Array.from(pr.given), f = Array.from(pr.family);
    if (isCJKName(pr)) return g.slice(0, 2).join('');
    return (f.length ? g.slice(0, 1).concat(f.slice(0, 1)) : g.slice(0, 2)).join('').toUpperCase();
  }

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
  function normAttachments(a) {
    var m = a && a.max_bytes;
    return { max_bytes: Number.isInteger(m) && m > 0 ? m : DEFAULT_ATTACHMENT_MAX };
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
    var S = { members: {}, admins: {}, items: {}, asg: {}, rules: {}, comments: {}, reg: {}, prio: normPriority(roles.priority),
      att: normAttachments(roles.attachments), labels: {}, lorder: [], reqs: {}, rorder: [] };
    (roles.members || []).forEach(function (m) {
      S.members[m.id] = { id: m.id, profile: normProfile(m, m.id), keys: (m.keys || []).slice(), revoked: {}, leads: (m.leads || []).slice(), removed: null, joined: null };
    });
    (roles.admins || []).forEach(function (a) { S.admins[a] = true; });
    return S;
  }

  // Assignment records. state: pending -> live (accept) | declined | withdrawn;
  // live -> unassigned | returned | handed (a handoff was accepted).
  // A rule request (ruleRequest, id = the rule id) is the pending record a rule
  // rooted outside its author's reach sends to the root: pending -> accepted (the
  // rule goes live) | declined | withdrawn; accepted -> returned | unassigned. It
  // never holds the item, so holder lookups (openRec) skip it.
  // A fallback (fallback set) is the holding the fold derives for a task nobody
  // else holds (see settle): live -> displaced (someone else came to hold it) |
  // unassigned (its holder was removed). openRecs and openRec skip it, so it never
  // blocks an assign; liveRec finds it, so its holder sets its status and closes it.
  function recsOn(S, item) { return vals(S.asg).filter(function (r) { return r.item === item; }); }
  function liveRecs(S, item) { return recsOn(S, item).filter(function (r) { return r.state === 'live'; }); }
  function openRecs(S, item) { return recsOn(S, item).filter(function (r) { return (r.state === 'live' || r.state === 'pending') && !r.fallback; }); }
  function liveRec(S, item, who) { return liveRecs(S, item).filter(function (r) { return r.to === who; })[0] || null; }
  function pendingRec(S, item, who) { return recsOn(S, item).filter(function (r) { return r.to === who && r.state === 'pending'; })[0] || null; }
  function openRec(S, item, who) { return openRecs(S, item).filter(function (r) { return r.to === who && !r.ruleRequest; })[0] || null; }
  function lastEndedRec(S, item, who) {
    var rs = recsOn(S, item).filter(function (r) { return r.to === who && r.ended; });
    return rs[rs.length - 1] || null;
  }
  // An accepted rule request to `who` whose rule is still live: what item.return ends.
  function acceptedReq(S, item, who) {
    return recsOn(S, item).filter(function (r) { return r.ruleRequest && r.to === who && r.state === 'accepted' && S.rules[r.rule] && !S.rules[r.rule].ended; })[0] || null;
  }

  // Assignment rules (490)-(492): a rule {root, includeRoot, levels} covers the root
  // (with includeRoot) and everyone up to `levels` below it; levels null = the whole
  // subtree. The older "everyone within N" is a rule rooted at its author. state:
  // live (its author reaches the root, or the root accepted its request) | pending
  // (the request is open); ended = the event id that ended it.
  var WHOLE = 99;
  function ruleDepth(rule) { return rule.levels == null ? WHOLE : rule.levels; }
  function openRules(S, item) { return vals(S.rules).filter(function (r) { return r.item === item && !r.ended; }); }
  function liveRules(S, item) { return openRules(S, item).filter(function (r) { return r.state === 'live'; }); }
  function coveredBy(S, rule) {
    var out = [], root = S.members[rule.root];
    if (rule.includeRoot && root && !root.removed) out.push(rule.root);
    return out.concat(Object.keys(reach(S.members, rule.root, ruleDepth(rule))));
  }
  // Rule y covers rule x by shape: everything x reaches, y reaches.
  function ruleCovers(S, y, x) {
    var d = y.root === x.root ? 0 : reach(S.members, y.root, WHOLE)[x.root];
    if (d == null) return false;
    if (x.includeRoot && !(d === 0 ? y.includeRoot : y.levels == null || d <= y.levels)) return false;
    if (y.levels == null) return true;
    return x.levels != null && d + x.levels <= y.levels;
  }

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
  // (391) A profile write: an object of strings among org / family / given. `full`
  // (a join) needs `given`; a later write carries only the fields it changes, and a
  // `given` it carries cannot be empty.
  function checkProfile(pr, full) {
    if (!pr || typeof pr !== 'object' || Array.isArray(pr)) rej('profile must be an object {org, family, given}');
    Object.keys(pr).forEach(function (k) {
      if (PROFILE_FIELDS.indexOf(k) < 0) rej('profile has no field "' + k + '" (org, family, given)');
      if (typeof pr[k] !== 'string') rej('profile.' + k + ' must be a string');
    });
    if (full && !has(pr, 'given')) rej('profile.given required');
    if (has(pr, 'given') && !pr.given.trim()) rej('profile.given cannot be empty');
    if (!PROFILE_FIELDS.some(function (f) { return has(pr, f); })) rej('profile has none of org, family, given');
  }
  function checkFields(S, p) {
    if (has(p, 'title') && !p.title) rej('title cannot be empty');
    if (p.project != null && typeof p.project !== 'string') rej('project is one optional free-text string');
    if (p.due != null && !(typeof p.due === 'string' && DUE_RE.test(p.due) && !isNaN(Date.parse(p.due)))) rej('due is an optional YYYY-MM-DD date');
    if (p.priority != null && !(Number.isInteger(p.priority) && p.priority >= 0 && p.priority < S.prio.names.length))
      rej('priority must be a level 0..' + (S.prio.names.length - 1) + ' (' + S.prio.names.join(' / ') + ')');
    if (p.labels != null) {
      if (!Array.isArray(p.labels)) rej('labels must be a list of label ids');
      var seen = {};
      p.labels.forEach(function (x) {
        if (typeof x !== 'string' || !has(S.labels, x)) rej('label "' + String(x) + '" is not in the store\'s label list');
        if (S.labels[x].removed) rej('label ' + alias(x) + ' (' + S.labels[x].name + ') was removed');
        if (seen[x]) rej('label ' + alias(x) + ' is listed twice');
        seen[x] = true;
      });
    }
  }
  // The live label other than `except` named `name`, ignoring case (label:<name> is
  // matched lower-cased).
  function liveLabel(S, id) { return has(S.labels, id) && !S.labels[id].removed ? S.labels[id] : null; }
  function labelNamed(S, name, except) {
    var k = String(name).toLowerCase();
    for (var i = 0; i < S.lorder.length; i++) {
      var l = liveLabel(S, S.lorder[i]);
      if (l && l.id !== except && l.name.toLowerCase() === k) return l;
    }
    return null;
  }
  // A label name: a non-blank string without surrounding spaces, unique among the
  // live labels other than `except`, ignoring case.
  function checkLabelName(S, p, except) {
    if (typeof p.name !== 'string' || !p.name.trim()) rej('name must be a non-blank string');
    if (p.name.trim() !== p.name) rej('name has leading or trailing spaces');
    var l = labelNamed(S, p.name, except);
    if (l) rej('label "' + p.name + '" already exists (' + alias(l.id) + ')');
  }
  function needLabel(S, id) {
    if (!has(S.labels, String(id))) rej('label "' + String(id) + '" is not in the store\'s label list');
    var l = S.labels[id];
    if (l.removed) rej('label ' + alias(l.id) + ' (' + l.name + ') was removed');
    return l;
  }
  // A label add or rename whose name a label already holds in L, the fold-order
  // label list: the causal check passed, so the holder was written concurrently,
  // and the first in fold order keeps the name.
  function labelClash(L, n) {
    if (n.type === 'roles.remove_label' || !has(n.p, 'name')) return null;
    var l = labelNamed(L, n.p.name, n.type === 'roles.edit_label' ? n.p.label : '');
    return l ? 'label "' + n.p.name + '" already exists (' + alias(l.id) + ', written concurrently; the first in fold order keeps the name)' : null;
  }
  // (335) `mentions` is an explicit list of member ids (the text keeps @name for
  // display); (338) `attachments` is metadata only; (531) `refs` is the item ids the
  // text references with #ID. All three ride with a body: item.create, item.edit with
  // a body, comment.add, comment.edit. `prior` is the target's shown
  // attachments in the author's seen state (none on create and comment.add): an oid
  // already there is exempt from the size limit, so only an attachment the write
  // adds is held to a limit an admin may since have lowered.
  function checkExtras(S, p, prior) {
    if (p.mentions != null) {
      if (!Array.isArray(p.mentions)) rej('mentions must be a list of member ids');
      p.mentions.forEach(function (x) {
        if (typeof x !== 'string') rej('mentions must be a list of member ids');
        if (!S.members[x]) rej('mentioned "' + x + '" is not a member');
        if (S.members[x].removed) rej('mentioned ' + x + ' was removed (' + alias(S.members[x].removed) + ')');
      });
    }
    if (p.attachments != null) {
      if (!Array.isArray(p.attachments)) rej('attachments must be a list of {oid, name, size, mime}');
      p.attachments.forEach(function (x) {
        if (!x || typeof x !== 'object' || Array.isArray(x)) rej('attachments must be a list of {oid, name, size, mime}');
        if (typeof x.oid !== 'string' || !OID_RE.test(x.oid)) rej('attachment oid must be the sha256 of the content (64 lowercase hex digits)');
        if (typeof x.name !== 'string' || !x.name) rej('attachment name required');
        if (!Number.isInteger(x.size) || x.size < 0) rej('attachment size must be a byte count');
        if (typeof x.mime !== 'string' || !x.mime) rej('attachment mime type required');
        var kept = (prior || []).some(function (o) { return o.oid === x.oid; });
        if (x.size > S.att.max_bytes && !kept) rej('attachment "' + x.name + '" is ' + x.size + ' bytes, above the store limit of ' + S.att.max_bytes + ' bytes (attachments.max_bytes)');
      });
    }
    if (p.refs != null) {
      if (!Array.isArray(p.refs)) rej('refs must be a list of item ids');
      p.refs.forEach(function (x) {
        if (typeof x !== 'string') rej('refs must be a list of item ids');
        if (!S.items[x]) rej('referenced ' + alias(x) + ' is not an item its author had seen');
      });
    }
  }
  // item.assign targets (490)-(492): member ids and rules {root, include_root?,
  // levels?}, mixed, one rule per root. A rule is checked like the older
  // "everyone within N": someone must be in it, and the author's live rules must
  // not already cover it.
  var TARGET_SHAPE = 'a target is a member id or a rule {root, include_root?, levels?}';
  function checkTargets(S, it, a, targets) {
    if (!Array.isArray(targets) || !targets.length) rej('choose at least one person or rule');
    var roots = {}, rules = 0;
    targets.forEach(function (x) {
      if (typeof x === 'string') {
        if (x !== a && (!S.members[x] || S.members[x].removed)) rej(x + ' is not a member');
        return;
      }
      if (!x || typeof x !== 'object' || Array.isArray(x) || typeof x.root !== 'string' || !x.root) rej(TARGET_SHAPE);
      if (x.include_root != null && typeof x.include_root !== 'boolean') rej(TARGET_SHAPE);
      if (x.levels != null && !(Number.isInteger(x.levels) && x.levels >= 1)) rej('rule levels must be a whole number >= 1');
      if (!S.members[x.root] || S.members[x.root].removed) rej(x.root + ' is not a member');
      if (roots[x.root]) rej('one rule per root in an assign (' + x.root + ')');
      roots[x.root] = true; rules++;
      var rule = { root: x.root, includeRoot: !!x.include_root, levels: x.levels == null ? null : x.levels };
      if (!coveredBy(S, rule).length) rej('nobody is ' + (rule.levels == null ? '' : 'within ' + rule.levels + ' levels ') + 'below ' + x.root);
      var cov = liveRules(S, it.id).filter(function (y) { return y.by === a && ruleCovers(S, y, rule); })[0];
      if (cov) rej('the rule rooted at ' + x.root + ' is already covered by ' + alias(cov.id));
    });
    if (!rules && !targets.some(function (x) { return !openRec(S, it.id, x); }))
      rej('everyone chosen already holds a live or pending assignment');
  }
  // (153 b) A topic is held by one person, for ownership only: an assign names
  // exactly one member, as targets: [id] or mode chosen with one person, and only
  // while the topic has no live or pending record (a pending handoff included).
  // Its completion is accepted and not stored.
  var TOPIC_ONE = 'a topic has one holder: assign it to exactly one person';
  function checkTopicAssign(S, it, p) {
    var one = p.targets != null ? p.mode == null && Array.isArray(p.targets) && p.targets.length === 1 && typeof p.targets[0] === 'string'
      : p.mode === 'chosen' && Array.isArray(p.people) && p.people.length === 1 && typeof p.people[0] === 'string';
    if (!one) rej(TOPIC_ONE);
    var held = [];
    openRecs(S, it.id).forEach(function (x) { if (held.indexOf(x.to) < 0) held.push(x.to); });
    if (held.length) rej('topic ' + it.alias + ' is already held by ' + held.join(', ') + '; unassign ' + held[0] + ' first');
  }
  function notAssignee(S, it, who) {
    var pr = pendingRec(S, it.id, who);
    if (pr && !openRec(S, it.id, who)) { // who holds only a rule request, which never holds the item itself
      var ru = S.rules[pr.rule];
      if (ru && !ru.includeRoot) return who + ' holds only a pending rule request on ' + it.alias + '; accepting it assigns the members below ' + who + ', not ' + who;
      return who + ' has not accepted the pending rule request on ' + it.alias;
    }
    if (pr) return who + ' has not accepted the pending assignment on ' + it.alias;
    var gone = recsOn(S, it.id).filter(function (r) { return r.to === who && r.ended && !r.fallback; }).pop(); // a fallback left out
    if (gone) return who + '\'s assignment on ' + it.alias + ' ended (' + gone.state + ', ' + alias(gone.ended.id) + ')';
    if (it.author === who) return who + ' created ' + it.alias + ' but is not assigned to it (self-assign first)';
    return who + ' is not an assignee of ' + it.alias;
  }
  function needRec(S, item, who) {
    var r = liveRec(S, item, who);
    if (!r) rej(notAssignee(S, S.items[item], who));
    return r;
  }

  // Taking work. An opening (it.open, the latest item.open: node, by, scope, ended)
  // lets one member in its scope (team: the opener's org subtree; store: every
  // member) claim a task nobody else holds. It ends when the item is claimed or
  // assigned, when anyone else comes to hold it, or when it is withdrawn
  // (item.unopen); ended is that event. A take request (S.reqs, its id the
  // item.request event's) asks for the live assignment `of` that `holder` holds:
  // pending -> approved | denied | withdrawn (item.unrequest, or the requester
  // removed) | lapsed (the assignment ended first).
  function activeOpen(S, it) { return it.open && !it.open.ended && liveFallback(S, it.id) ? it.open : null; }
  function inScope(S, o, who) { return liveMember(S, who) && (o.scope === 'store' || below(S.members, o.by, who)); }
  function holders(S, it) {
    var held = [];
    openRecs(S, it.id).forEach(function (x) { if (!x.ruleRequest && held.indexOf(x.to) < 0) held.push(x.to); });
    return held;
  }
  // Why it cannot be claimed: never opened, or the event that ended its opening.
  function notOpen(S, it) {
    var o = it.open;
    if (!o || (!o.ended && !liveFallback(S, it.id))) return it.alias + ' is not open for claiming';
    var e = o.ended;
    if (e.type === 'item.claim') return e.author + ' already claimed ' + it.alias + ' (' + alias(e.id) + ')';
    if (e.type === 'item.unopen') return e.author + ' withdrew the opening of ' + it.alias + ' (' + alias(e.id) + ')';
    if (e.type === 'item.assign') return e.author + ' assigned ' + it.alias + ' (' + alias(e.id) + '), which ended its opening';
    return 'the opening of ' + it.alias + ' ended (' + alias(e.id) + ')';
  }
  function needReq(S, it, id) {
    var q = has(S.reqs, String(id)) ? S.reqs[id] : null;
    if (!q || q.item !== it.id) rej(alias(String(id)) + ' is not a take request on ' + it.alias);
    return q;
  }
  // Why q is no longer pending.
  function reqEnded(q) {
    var e = q.ended, rq = alias(q.id);
    if (q.state === 'approved') return e.author + ' already approved ' + rq + ' (' + alias(e.id) + ')';
    if (q.state === 'denied') return e.author + ' denied ' + rq + ' (' + alias(e.id) + ')';
    if (q.state === 'withdrawn') return rq + ' was withdrawn (' + alias(e.id) + ')';
    return rq + ' lapsed: ' + q.holder + '\'s assignment on ' + alias(q.item) + ' ended (' + alias(e.id) + ')';
  }
  // The suffix of a fold-time flag (see foldFlag).
  var CONCURRENTLY = '; written concurrently, the first in fold order wins';

  // An older item.edit stage change, checked as it was when stages were written:
  // tasks and todos only, review or null (todo). Moving to review needs an item with
  // no live or pending assignment (the fallback is neither). A new write carrying
  // stage is refused by the writers, not here: the mock store appends what it is given.
  function checkStage(S, it, p) {
    if (it.kind === 'topic') rej('topics have no stage');
    if (p.stage == null) return;
    if (p.stage !== 'review') rej(STAGE_SHAPE);
    var held = [];
    openRecs(S, it.id).forEach(function (x) { if (held.indexOf(x.to) < 0) held.push(x.to); });
    if (held.length) rej(it.alias + ' has live or pending assignments (' + held.join(', ') + '); unassign them before moving it to review');
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
        if (p.stage != null) {
          if (p.kind === 'topic') rej('topics have no stage');
          if (p.stage !== 'review') rej(STAGE_SHAPE);
        }
        checkExtras(S, p, null);
        if (p.parent) {
          var par = needItem(S, p.parent, 'parent');
          if (par.hidden) rej('parent ' + par.alias + ' is hidden');
          if (par.author !== a && !liveRec(S, p.parent, a)) rej('parent ' + par.alias + ' is neither assigned to nor created by ' + a);
        }
        // (528) the sibling it follows; null or "" = first. An anchor that is not a
        // sibling when the view is built falls back to the end (deriveViewState).
        if (p.after != null && typeof p.after !== 'string') rej('after must be a sibling item id');
        break;

      case 'item.edit':
        it = needItem(S, p.item);
        if (it.author !== a && !creatorGone(S, it, a)) rej('only the creator (' + it.author + ') edits ' + it.alias);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (!ITEM_FIELDS.some(function (f) { return has(p, f); })) rej('nothing to edit (' + EDIT_FIELDS.join(', ') + ')');
        if (has(p, 'priority') && p.priority == null) rej('priority cannot be cleared; set a level');
        checkFields(S, p);
        if (has(p, 'stage')) checkStage(S, it, p);
        if ((p.mentions != null || p.attachments != null) && !has(p, 'body')) rej('mentions and attachments go with a body edit');
        if (p.refs != null && !has(p, 'body')) rej('refs go with a body edit');
        checkExtras(S, p, it.attachments);
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
        if (it.author !== a && !creatorGone(S, it, a))
          rej('only the creator (' + it.author + ') assigns ' + it.alias + '; an assignee hands off their own assignment with item.handoff');
        var completion = p.completion || 'assignee';
        if (completion !== 'assignee' && completion !== 'assigner') rej('completion must be assignee or assigner');
        if (it.kind === 'topic') checkTopicAssign(S, it, p);
        if (p.targets != null) {
          if (p.mode != null) rej('item.assign takes targets or mode, not both');
          checkTargets(S, it, a, p.targets);
        } else if (p.mode === 'all') { // the older form of a rule rooted at the author
          var levels = Number(p.levels || 0);
          if (!(levels >= 1)) rej('levels must be >= 1');
          if (!Object.keys(reach(S.members, a, levels)).length) rej('nobody is within ' + levels + ' levels below ' + a);
          var cov = liveRules(S, it.id).filter(function (x) { return x.by === a && ruleCovers(S, x, { root: a, includeRoot: false, levels: levels }); })[0];
          if (cov && cov.root === a && cov.levels != null) rej('already assigned to everyone within ' + cov.levels + ' levels (' + alias(cov.id) + ')');
          if (cov) rej('the rule rooted at ' + a + ' is already covered by ' + alias(cov.id));
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
        if (r.fallback) rej(from + ' holds ' + it.alias + ' only because nobody else does; assign it instead');
        if (r.status === 'done') rej('assignment is done; nothing to hand off');
        if (!p.to) rej('to required');
        if (p.to === from) rej('hand off to someone else');
        needMember(S, p.to);
        if (openRec(S, it.id, p.to)) rej(p.to + ' already holds a live or pending assignment on ' + it.alias);
        var pend = vals(S.asg).filter(function (x) { return x.handoffOf === r.id && x.state === 'pending'; })[0];
        if (pend) rej('a handoff of this assignment to ' + pend.to + ' is already pending (' + alias(pend.via) + ')');
        break;
      }

      case 'item.open': {
        it = needItem(S, p.item);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (it.author !== a && !creatorGone(S, it, a)) rej('only the creator (' + it.author + ') opens ' + it.alias + ' for claiming');
        if (it.kind === 'topic') rej('a topic is not opened for claiming; its creator assigns its one holder');
        if (p.scope !== 'team' && p.scope !== 'store') rej('scope must be team (the opener\'s org subtree) or store (every member)');
        var ao = activeOpen(S, it);
        if (ao) rej(it.alias + ' is already open (' + alias(ao.node.id) + ')');
        var hd = holders(S, it);
        if (hd.length) rej(it.alias + ' has live or pending assignments (' + hd.join(', ') + '); only a task nobody else holds is opened');
        var fbo = liveFallback(S, it.id);
        if (fbo && fbo.status === 'done') rej(it.alias + ' is done; nothing to open');
        break;
      }

      case 'item.unopen':
        it = needItem(S, p.item);
        if (it.author !== a && !creatorGone(S, it, a)) rej('only the creator (' + it.author + ') withdraws the opening of ' + it.alias);
        if (!activeOpen(S, it)) rej(notOpen(S, it));
        break;

      case 'item.claim': {
        it = needItem(S, p.item);
        if (it.hidden) rej(it.alias + ' is hidden');
        var o = activeOpen(S, it);
        if (!o) rej(notOpen(S, it));
        if (liveFallback(S, it.id).to === a) rej(a + ' already holds ' + it.alias);
        if (!inScope(S, o, a)) rej(a + ' is not in ' + o.by + '\'s org subtree; ' + it.alias + ' is open to that team only');
        break;
      }

      case 'item.request': {
        it = needItem(S, p.item);
        if (it.hidden) rej(it.alias + ' is hidden');
        if (!p.assignee) rej('assignee required (the holder of the assignment to take)');
        if (p.assignee === a) rej(a + ' cannot request their own assignment');
        r = needRec(S, it.id, p.assignee);
        if (r.fallback) rej(p.assignee + ' holds ' + it.alias + ' only because nobody else does; it is taken by a claim once its creator opens it');
        if (r.status === 'done') rej('assignment is done; nothing to take');
        if (openRec(S, it.id, a)) rej(a + ' already holds a live or pending assignment on ' + it.alias);
        S.rorder.forEach(function (id) {
          q = S.reqs[id];
          if (q.by === a && q.of === r.id && q.state === 'pending')
            rej(a + '\'s request for ' + p.assignee + '\'s assignment on ' + it.alias + ' is already pending (' + alias(q.id) + ')');
        });
        break;
      }

      case 'item.approve':
      case 'item.deny':
        it = needItem(S, p.item);
        q = needReq(S, it, p.request);
        if (q.state !== 'pending') rej(reqEnded(q));
        if (a !== q.holder && it.author !== a && !creatorGone(S, it, a))
          rej('only ' + q.holder + ' (the assignment\'s holder) or the creator (' + it.author + ') ' + (n.type === 'item.approve' ? 'approves' : 'denies') + ' ' + alias(q.id));
        break;

      case 'item.unrequest':
        it = needItem(S, p.item);
        q = needReq(S, it, p.request);
        if (q.state !== 'pending') rej(reqEnded(q));
        if (q.by !== a) rej('only the requester (' + q.by + ') withdraws ' + alias(q.id));
        break;

      case 'item.accept':
      case 'item.decline':
        it = needItem(S, p.item);
        r = pendingRec(S, it.id, a);
        if (!r) rej(liveRec(S, it.id, a) ? a + '\'s assignment on ' + it.alias + ' is already live' : a + ' has no pending assignment on ' + it.alias);
        break;

      case 'item.return': { // an accepted assignment, and an accepted rule rooted at the assignee
        it = needItem(S, p.item);
        r = liveRec(S, it.id, p.assignee);
        if (r && r.fallback) r = null; // nobody else holds it: nothing to return
        q = acceptedReq(S, it.id, p.assignee);
        if (!r && !q) rej((p.assignee || '?') + ' has no live assignment on ' + it.alias);
        var to = (r || q).to, lead = ((S.members[to] || {}).leads || [])[0];
        if (lead !== a) rej('only ' + to + '\'s direct lead (' + (lead || 'none: ' + to + ' is a root') + ') returns an accepted assignment');
        if (!q && !r.accepted) rej(r.to + '\'s assignment was forced by ' + r.by + ' (their lead chain); only an accepted one is returnable');
        break;
      }

      case 'item.unassign':
        it = needItem(S, p.item);
        if (p.rule != null) { // one rule of the author's, with its request and records
          if (p.assignee) rej('item.unassign takes assignee or rule, not both');
          if (typeof p.rule !== 'string') rej('rule must be a rule id');
          q = S.rules[p.rule];
          if (!q || q.item !== it.id || q.ended) rej(alias(p.rule) + ' is not an open rule on ' + it.alias);
          if (q.by !== a) rej('only the rule\'s author (' + q.by + ') ends ' + alias(p.rule));
          if (p.root != null && p.root !== q.root) rej(alias(p.rule) + ' is rooted at ' + q.root + ', not ' + p.root);
        } else if (p.root != null) { // the author's rules rooted at `root`, with their requests and records
          if (p.assignee) rej('item.unassign takes assignee or root, not both');
          if (!openRules(S, it.id).some(function (x) { return x.by === a && x.root === p.root; }))
            rej(a + ' has no rule rooted at ' + p.root + ' on ' + it.alias);
        } else if (p.assignee) {
          r = openRec(S, it.id, p.assignee);
          if (!r) rej(p.assignee + ' has no live or pending assignment on ' + it.alias);
          var departed = (S.members[r.to] || {}).removed || (S.members[r.by] || {}).removed;
          var holder = it.kind === 'topic' && r.to === a && r.state === 'live'; // (153 b) the holder releases a topic
          if (r.by !== a && !(r.state === 'pending' && r.sender === a) && !(S.admins[a] && departed) && !holder)
            rej('only the assigner (' + r.by + ') unassigns' + (r.state === 'pending' && r.sender !== r.by ? ' (or ' + r.sender + ' withdraws)' : ''));
        } else if (!openRecs(S, it.id).some(function (x) { return x.by === a || (x.state === 'pending' && x.sender === a); }) &&
          !openRules(S, it.id).some(function (x) { return x.by === a; }))
          rej(a + ' has no assignments to withdraw on ' + it.alias);
        break;

      case 'item.status':
        it = needItem(S, p.item);
        if (it.kind === 'topic') rej('topics have no status');
        if (SETTABLE.indexOf(p.status) < 0) rej('status must be idea, todo, doing or blocked (done goes through item.close)');
        r = needRec(S, it.id, a);
        if (r.status === 'done') rej('assignment is done; reopen it first');
        break;

      case 'item.close':
        it = needItem(S, p.item);
        if (it.kind === 'topic') rej('topics have no status');
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
        checkExtras(S, p, null);
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
        if (n.type === 'comment.edit') checkExtras(S, p, q.attachments);
        break;

      case 'roles.add_member': {
        needAdmin(S, a);
        var nm = p.member || {};
        if (!nm.id) rej('member.id required');
        if (String(nm.id).indexOf('.') >= 0) rej('member ids cannot contain "." (it separates member and clone in a stream id)');
        if (S.members[nm.id]) rej('member "' + nm.id + '" already exists' + (S.members[nm.id].removed ? ' (removed; ids are not reused)' : ''));
        checkProfile(nm.profile, true);
        if (!nm.keys || !nm.keys.length) rej('member.keys required');
        nm.keys.forEach(function (k) { if (keyInUse(S, k)) rej('key "' + k + '" is already in use'); });
        checkLeads(S, nm.id, nm.leads || []);
        break;
      }
      case 'member.profile': {
        var who = p.member == null ? a : p.member;
        checkProfile(p.profile, false);
        needMember(S, who);
        if (who !== a && !S.admins[a]) rej('only an admin changes another member\'s profile (' + a + ' is not an admin)');
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
      case 'roles.set_attachment_limit':
        needAdmin(S, a);
        if (!Number.isInteger(p.max_bytes) || p.max_bytes <= 0) rej('max_bytes must be a positive integer (bytes)');
        break;
      case 'roles.add_label':
        needAdmin(S, a);
        checkLabelName(S, p, '');
        if (!(typeof p.color === 'string' && COLOR_RE.test(p.color))) rej('color must be #rrggbb in lowercase hex');
        break;
      case 'roles.edit_label': {
        needAdmin(S, a);
        var lb = needLabel(S, has(p, 'label') ? p.label : undefined);
        if (!has(p, 'name') && !has(p, 'color')) rej('nothing to edit (name, color)');
        if (has(p, 'name')) checkLabelName(S, p, lb.id);
        if (has(p, 'color') && !(typeof p.color === 'string' && COLOR_RE.test(p.color))) rej('color must be #rrggbb in lowercase hex');
        break;
      }
      case 'roles.remove_label':
        needAdmin(S, a);
        needLabel(S, has(p, 'label') ? p.label : undefined);
        break;

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
      if (v.labels) return v.labels.length ? v.labels.map(alias).join(', ') : '(none)';
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

  function mentionsOf(p) { return Array.isArray(p.mentions) ? p.mentions.slice() : []; }
  function refsOf(p) { return Array.isArray(p.refs) ? p.refs.slice() : []; }
  function labelsOf(p) { return Array.isArray(p.labels) ? p.labels.map(String) : []; }
  function attachmentsOf(p) {
    return Array.isArray(p.attachments) ? p.attachments.map(function (x) { return { oid: x.oid, name: x.name, size: x.size, mime: x.mime }; }) : [];
  }
  // The register's shown write: mentions, attachments and refs follow the body write that won.
  function regWinner(S, key) { return winner(S.reg[key]).n; }

  // A record's id is "<via>><member>" unless extra.id names it (a rule's records
  // and its request).
  function addRec(S, via, item, to, by, completion, extra) {
    var id = (extra && extra.id) || via.id + '>' + to;
    if (S.asg[id]) return null;
    var r = { id: id, item: item, to: to, by: by, sender: by, completion: completion, via: via.id, ts: via.ts,
      state: 'live', forced: true, accepted: null, leadAtAccept: null, ended: null, handoffOf: null, answerNote: null,
      status: 'todo', note: null, noteBy: null, last: via, writes: [], inherited: null, rule: null, ruleRequest: false };
    if (extra) Object.keys(extra).forEach(function (k) { r[k] = extra[k]; });
    S.asg[id] = r;
    return r;
  }

  // A rule's records: "<rule id>><member>", via the assign event, for everyone it
  // covers who holds nothing on the item and was not unassigned from it.
  function extendRule(S, rule, n, inherited) {
    coveredBy(S, rule).forEach(function (t) {
      if (rule.excluded[t] || openRec(S, rule.item, t) || S.asg[rule.id + '>' + t]) return;
      addRec(S, { id: rule.via, ts: n.ts }, rule.item, t, rule.by, rule.completion,
        { id: rule.id + '>' + t, rule: rule.id, inherited: inherited ? n.id : null, last: n });
    });
  }
  // Live rules: whoever enters the coverage later inherits the assignment.
  function refreshLive(S, n) {
    vals(S.rules).forEach(function (rule) { if (!rule.ended && rule.state === 'live') extendRule(S, rule, n, true); });
  }
  // A rule ends with the live records it made; a handoff its holder accepted stays.
  function endRule(S, rule, n, C) {
    if (!rule) return;
    if (!rule.ended) rule.ended = n.id;
    recsOn(S, rule.item).forEach(function (x) {
      if (x.rule === rule.id && !x.ruleRequest && x.state === 'live') endRec(S, x, n, 'unassigned', C);
    });
  }
  // item.unassign {root} or {rule}: a pending request is withdrawn, an accepted one
  // unassigned, then the rule ends with its records.
  function unassignRule(S, rule, n, C) {
    var q = rule.request && S.asg[rule.request];
    if (q && q.state === 'pending') endRec(S, q, n, 'withdrawn', C);
    else if (q && q.state === 'accepted') { q.state = 'unassigned'; q.ended = n; }
    endRule(S, rule, n, C);
  }
  // Who holds a task nobody else holds: its creator, or when the creator was removed
  // the first admin in the roles admin order.
  function liveMember(S, id) { return !!S.members[id] && !S.members[id].removed; }
  function fallbackHolder(S, it) {
    if (liveMember(S, it.author)) return it.author;
    return Object.keys(S.admins).filter(function (a) { return liveMember(S, a); })[0] || null;
  }
  function liveFallback(S, item) { return recsOn(S, item).filter(function (r) { return r.fallback && r.state === 'live'; })[0] || null; }
  // Keep every task held after n: a live or pending record other than a rule request
  // displaces the fallback, and with none the fallback holder gets a fresh record
  // "<n>@<item>><holder>" at todo, so each fallback period has its own status
  // register. A topic may be held by nobody.
  function settle(S, id, n, C) {
    var it = S.items[id];
    if (!it || it.kind === 'topic') return;
    var fb = null, held = false;
    recsOn(S, id).forEach(function (r) {
      if (r.fallback) { if (r.state === 'live') fb = r; }
      else if ((r.state === 'live' || r.state === 'pending') && !r.ruleRequest) held = true;
    });
    if (held) {
      if (fb) endRec(S, fb, n, 'displaced', C);
      if (it.open && !it.open.ended) it.open.ended = n; // an opening never outlives the fallback hold
      return;
    }
    var h = fallbackHolder(S, it);
    if (fb || !h) return;
    addRec(S, n, id, h, h, 'assignee', { id: n.id + '@' + id + '>' + h, fallback: true, start: n,
      status: n.type === 'item.create' && n.p.stage === 'review' ? 'idea' : 'todo' }); // an older create in the review stage
  }
  function settleAll(S, n, C) { Object.keys(S.items).forEach(function (id) { settle(S, id, n, C); }); }
  function assignPerson(S, n, item, t, a, completion) { // (273) below the author = forced (live), anyone else = pending
    if (!S.members[t] || openRec(S, item, t)) return;
    var forced = t === a || below(S.members, a, t);
    addRec(S, n, item, t, a, completion, { forced: forced, state: forced ? 'live' : 'pending' });
  }
  // (491) A rule the author reaches the root of is live at once; one rooted outside
  // that reach sends the root a request and waits for its accept.
  function addRule(S, n, item, x, a, completion) {
    var id = n.id + '~' + x.root;
    if (S.rules[id] || !S.members[x.root]) return;
    var live = x.root === a || below(S.members, a, x.root);
    var rule = S.rules[id] = { id: id, via: n.id, item: item, by: a, root: x.root, includeRoot: !!x.include_root,
      levels: x.levels == null ? null : Number(x.levels), completion: completion, excluded: {}, ended: null,
      state: live ? 'live' : 'pending', request: null, ts: n.ts };
    if (live) { extendRule(S, rule, n, false); return; }
    rule.request = addRec(S, n, item, x.root, a, completion, { id: id, rule: id, ruleRequest: true, forced: false, state: 'pending' }).id;
  }

  // End a record. Unassign, return, withdraw and an accepted handoff beat any
  // concurrent status write on it: the writes are marked, valid but not in effect.
  // (295) A pending handoff of the record ends with it, as withdrawn, pointing at
  // the same event: there is nothing left to hand on.
  function endRec(S, r, n, state, C) {
    r.state = state; r.ended = n;
    if (r.ruleRequest && S.rules[r.rule] && !S.rules[r.rule].ended) S.rules[r.rule].ended = n.id; // a withdrawn request ends its rule
    vals(S.asg).forEach(function (x) {
      if (x.handoffOf === r.id && x.state === 'pending') { x.state = 'withdrawn'; x.ended = n; }
    });
    S.rorder.forEach(function (id) { // a take request names one assignment
      var q = S.reqs[id];
      if (q.of === r.id && q.state === 'pending') { q.state = 'lapsed'; q.ended = n; }
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

  // A write whose causal-past check passed but that an event earlier in fold
  // order, concurrent with it, has already overtaken (a second claim of one
  // opening, an approval of a request already answered) is flagged in the final
  // fold (C) and takes no effect. A check-time fold skips it by the same test
  // without flagging, so later verdicts see what a full replay shows.
  function foldFlag(n, reason, C) { if (C) { n.status = 'flagged'; n.reason = reason; } }
  // item.open; a creator's assign concurrent with it and earlier in fold order
  // already holds the item, so the opening is flagged.
  function applyOpen(S, n, C) {
    var it = S.items[n.p.item];
    if (!it) return;
    var held = holders(S, it);
    if (held.length) { foldFlag(n, it.alias + ' has live or pending assignments (' + held.join(', ') + '); only a task nobody else holds is opened' + CONCURRENTLY, C); return; }
    it.open = { node: n, by: n.author, scope: n.p.scope, ended: null };
  }
  // item.claim: a live record by the opener, which displaces the fallback and ends
  // the opening; a claim whose opening an earlier event ended is flagged.
  function applyClaim(S, n, C) {
    var it = S.items[n.p.item];
    if (!it || !it.open) return;
    var o = activeOpen(S, it);
    if (!o) { foldFlag(n, notOpen(S, it) + CONCURRENTLY, C); return; }
    var a = n.author;
    if (!liveMember(S, a)) return;
    addRec(S, n, it.id, a, o.by, 'assignee', { forced: false, sender: a, accepted: n }); // no accepted notice: nobody asked
    o.ended = n;
  }
  // item.request: for the live assignment its assignee holds; when that ended
  // earlier in fold order, the request lapses at once.
  function applyRequest(S, n) {
    var p = n.p, it = S.items[p.item];
    if (!it || has(S.reqs, n.id)) return;
    var q = { node: n, id: n.id, item: it.id, by: n.author, holder: p.assignee, of: null, note: p.note || null, answerNote: null, state: 'pending', ended: null };
    var r = liveRec(S, it.id, p.assignee);
    if (r && !r.fallback) q.of = r.id;
    else {
      q.state = 'lapsed'; q.ended = n;
      r = lastEndedRec(S, it.id, p.assignee);
      if (r && !r.fallback) { q.of = r.id; q.ended = r.ended; }
    }
    S.reqs[n.id] = q; S.rorder.push(n.id);
  }
  // item.approve: the named assignment ends handed and the requester's new record
  // starts live with its assigner, completion, status, and note, as an accepted
  // handoff does (290); a requester holding a record by then gets no second one.
  // An approval of a request an earlier event answered, withdrew, or let lapse is
  // flagged.
  function applyApprove(S, n, C) {
    var p = n.p, q = has(S.reqs, String(p.request)) ? S.reqs[p.request] : null;
    if (!q || q.item !== p.item) return;
    if (q.state !== 'pending') { foldFlag(n, reqEnded(q) + CONCURRENTLY, C); return; }
    q.state = 'approved'; q.ended = n; q.answerNote = p.note || null;
    var old = S.asg[q.of];
    if (!old || old.state !== 'live') return;
    endRec(S, old, n, 'handed', C);
    if (openRec(S, q.item, q.by) || !liveMember(S, q.by)) return;
    addRec(S, n, q.item, q.by, old.by, old.completion, { forced: false, sender: n.author, handoffOf: old.id, accepted: n,
      status: old.status, note: old.note, noteBy: old.noteBy });
  }

  function apply(S, n, C) {
    var p = n.p, a = n.author, it, q, m, r;
    switch (n.type) {
      case 'item.create':
        // (154 a) task and todo are one kind: a todo create folds as a task.
        S.items[n.id] = { id: n.id, alias: n.alias, kind: p.kind === 'todo' ? 'task' : p.kind, title: p.title, body: p.body || '', project: p.project || '',
          due: p.due || null, priority: p.priority != null ? p.priority : S.prio.default,
          parent: p.parent || null, after: has(p, 'after') ? (p.after || null) : undefined, author: a, ts: n.ts, hidden: false, edited: null, node: n,
          mentions: mentionsOf(p), attachments: attachmentsOf(p), refs: refsOf(p), labels: labelsOf(p) };
        break;
      case 'item.edit':
        it = S.items[p.item]; if (!it) break;
        ITEM_FIELDS.forEach(function (f) {
          if (!has(p, f)) return;
          if (f === 'stage') {
            // An older event: review is the fallback holder's idea status, null (todo)
            // its todo. Held by anyone else, the item is not in review, and the
            // assignment wins.
            var fb = liveFallback(S, it.id);
            if (!fb) return;
            fb.writes.push(n);
            var sw = write(S, 'st:' + fb.id, 'status of ' + fb.to + ' on ' + alias(it.id), n, { status: p.stage === 'review' ? 'idea' : 'todo', note: null, by: a, n: n }, C);
            fb.status = sw.status; fb.note = sw.note; fb.noteBy = sw.by; fb.last = sw.n;
            return;
          }
          if (f === 'labels') { it.labels = write(S, 'it:' + it.id + ':labels', 'labels of ' + it.alias, n, { labels: labelsOf(p) }, C).labels; return; }
          it[f] = write(S, 'it:' + it.id + ':' + f, f + ' of ' + it.alias, n, p[f] == null ? (f === 'due' ? null : '') : p[f], C);
        });
        if (has(p, 'body')) { var bw = regWinner(S, 'it:' + it.id + ':body'); it.mentions = mentionsOf(bw.p); it.attachments = attachmentsOf(bw.p); it.refs = refsOf(bw.p); }
        it.edited = n;
        break;
      case 'item.hide':
        it = S.items[p.item]; if (!it) break;
        it.hidden = write(S, 'it:' + it.id + ':hidden', it.alias + ' hidden', n, true, C);
        break;
      case 'item.assign': {
        if (!S.items[p.item]) break;
        if (S.items[p.item].open && !S.items[p.item].open.ended) S.items[p.item].open.ended = n; // delegated another way
        // a topic record carries no status, so no completion mode either
        var completion = S.items[p.item].kind === 'topic' ? '' : p.completion || 'assignee';
        if (p.targets != null) (Array.isArray(p.targets) ? p.targets : []).forEach(function (x) {
          if (typeof x === 'string') assignPerson(S, n, p.item, x, a, completion);
          else if (x && typeof x.root === 'string') addRule(S, n, p.item, x, a, completion);
        });
        else if (p.mode === 'all') { // the older form: a rule rooted at the author, its id the event's
          var rule = S.rules[n.id] = { id: n.id, via: n.id, item: p.item, by: a, root: a, includeRoot: false, levels: Number(p.levels),
            completion: completion, excluded: {}, ended: null, state: 'live', request: null, ts: n.ts };
          extendRule(S, rule, n, false);
        } else (p.people || []).forEach(function (t) { assignPerson(S, n, p.item, t, a, completion); });
        break;
      }
      case 'item.handoff':
        r = liveRec(S, p.item, p.assignee || a);
        if (!r || r.fallback || !S.members[p.to] || openRec(S, p.item, p.to)) break;
        addRec(S, n, p.item, p.to, r.by, r.completion, { forced: false, state: 'pending', sender: a, handoffOf: r.id, answerNote: null, handoffNote: p.note || null });
        break;
      case 'item.accept': { // answers every pending assignment of the author on the item
        var acc = recsOn(S, p.item).filter(function (x) { return x.to === a && x.state === 'pending'; });
        if (!acc.length) { onEnded(S, p.item, a, n, C); break; }
        acc.forEach(function (r) {
          r.accepted = n; r.answerNote = p.note || null; r.last = n;
          r.leadAtAccept = ((S.members[a] || {}).leads || [])[0] || null;
          if (r.ruleRequest) { // (491) the rule goes live, forced from here on
            r.state = 'accepted';
            var rule = S.rules[r.rule];
            if (rule && !rule.ended) { rule.state = 'live'; extendRule(S, rule, n, false); }
            return;
          }
          r.state = 'live';
          var o = r.handoffOf && S.asg[r.handoffOf];
          if (o && o.state === 'live') { // (290) the new assignment starts with the original's status and note
            endRec(S, o, n, 'handed', C);
            r.status = o.status; r.note = o.note; r.noteBy = o.noteBy;
          }
        });
        break;
      }
      case 'item.decline':
        recsOn(S, p.item).forEach(function (r) {
          if (r.to !== a || r.state !== 'pending') return;
          r.state = 'declined'; r.ended = n; r.answerNote = p.note || null;
          if (r.ruleRequest && S.rules[r.rule] && !S.rules[r.rule].ended) S.rules[r.rule].ended = n.id;
        });
        break;
      case 'item.return': {
        var rq = acceptedReq(S, p.item, p.assignee);
        r = liveRec(S, p.item, p.assignee);
        if (r && r.fallback) r = null; // a fallback is not returned; with no other holding nothing happens
        if (!r) { q = handedRec(S, p.item, p.assignee, n); r = q && reverseHanded(S, q, n, C); }
        if (r && (r.accepted || !rq)) {
          endRec(S, r, n, 'returned', C); r.answerNote = p.note || null;
          // (291) returning an accepted handoff restores the original assignment with
          // its prior status, unless its holder was removed or holds another one now
          q = r.handoffOf && S.asg[r.handoffOf];
          if (q && q.state === 'handed' && q.ended === r.accepted && S.members[q.to] && !S.members[q.to].removed && !openRec(S, q.item, q.to)) {
            q.state = 'live'; q.ended = null; q.restored = n; r.restores = q.id;
          }
        }
        if (rq) { rq.state = 'returned'; rq.ended = n; rq.answerNote = p.note || null; endRule(S, S.rules[rq.rule], n, C); }
        break;
      }
      case 'item.unassign':
        if (p.rule != null) {
          q = S.rules[p.rule];
          if (q && q.item === p.item && !q.ended && q.by === a) unassignRule(S, q, n, C);
        } else if (p.root != null) {
          openRules(S, p.item).forEach(function (rule) { if (rule.by === a && rule.root === p.root) unassignRule(S, rule, n, C); });
        } else if (p.assignee) {
          r = openRec(S, p.item, p.assignee);
          if (!r) { q = handedRec(S, p.item, p.assignee, n); r = q && reverseHanded(S, q, n, C); }
          openRules(S, p.item).forEach(function (rule) { if (rule.by === (r ? r.by : a)) rule.excluded[p.assignee] = true; });
          if (r) endRec(S, r, n, r.state === 'pending' ? 'withdrawn' : 'unassigned', C);
        } else {
          openRules(S, p.item).forEach(function (rule) {
            if (rule.by !== a) return;
            rule.ended = n.id;
            q = rule.request && S.asg[rule.request];
            if (q && q.state === 'accepted') { q.state = 'unassigned'; q.ended = n; }
          });
          recsOn(S, p.item).filter(function (x) { return x.by === a && handedConc(x, n); }).forEach(function (x) { reverseHanded(S, x, n, C); });
          openRecs(S, p.item).filter(function (x) { return x.by === a || (x.state === 'pending' && x.sender === a); })
            .forEach(function (x) { endRec(S, x, n, x.state === 'pending' ? 'withdrawn' : 'unassigned', C); });
        }
        break;
      case 'item.status': case 'item.close': case 'item.confirm': case 'item.reject': case 'item.reopen': {
        var who = (n.type === 'item.status' || n.type === 'item.close') ? a : (p.assignee || a);
        r = liveRec(S, p.item, who);
        if (r && r.fallback && r.start !== n && !hb(r.start, n)) r = null; // n had not seen the fallback start: it was meant for the record that ended
        if (!r) { onEnded(S, p.item, who, n, C); break; } // folded onto an assignment a concurrent event ended
        r.writes.push(n);
        var v = n.type === 'item.status' ? p.status : n.type === 'item.close' ? (r.completion === 'assigner' ? 'awaiting' : 'done') :
          n.type === 'item.confirm' ? 'done' : 'doing';
        var w = write(S, 'st:' + r.id, 'status of ' + who + ' on ' + alias(p.item), n, { status: v, note: p.note || null, by: a, n: n }, C);
        r.status = w.status; r.note = w.note; r.noteBy = w.by; r.last = w.n;
        break;
      }
      case 'item.open': applyOpen(S, n, C); break;
      case 'item.unopen': // after a claim or an assign folded first, a no-op
        it = S.items[p.item];
        if (it && it.open && !it.open.ended) it.open.ended = n;
        break;
      case 'item.claim': applyClaim(S, n, C); break;
      case 'item.request': applyRequest(S, n); break;
      case 'item.approve': applyApprove(S, n, C); break;
      case 'item.deny': case 'item.unrequest': // after the request was answered or lapsed, a no-op
        q = has(S.reqs, String(p.request)) ? S.reqs[p.request] : null;
        if (q && q.item === p.item && q.state === 'pending') {
          q.state = n.type === 'item.deny' ? 'denied' : 'withdrawn'; q.ended = n;
          if (n.type === 'item.deny') q.answerNote = p.note || null;
        }
        break;
      case 'comment.add':
        S.comments[n.id] = { id: n.id, alias: n.alias, item: p.item, author: a, body: p.body, reply_to: p.reply_to || null,
          hidden: false, edited: null, ts: n.ts, node: n, mentions: mentionsOf(p), attachments: attachmentsOf(p), refs: refsOf(p) };
        break;
      case 'comment.edit':
        q = S.comments[p.comment]; if (!q) break;
        q.body = write(S, 'cm:' + q.id, 'text of comment ' + q.alias, n, p.body, C); q.edited = n;
        var cw = regWinner(S, 'cm:' + q.id); q.mentions = mentionsOf(cw.p); q.attachments = attachmentsOf(cw.p); q.refs = refsOf(cw.p);
        break;
      case 'comment.hide':
        q = S.comments[p.comment]; if (q) { q.hidden = true; q.hiddenBy = n; }
        break;
      case 'roles.add_member': {
        var nm = p.member || {};
        S.members[nm.id] = { id: nm.id, profile: normProfile(nm, nm.id), keys: (nm.keys || []).slice(), revoked: {}, leads: (nm.leads || []).slice(), removed: null, joined: n.id };
        break;
      }
      case 'member.profile': // (392) each field is its own register: concurrent changes merge by (203)
        m = S.members[p.member == null ? a : p.member]; if (!m) break;
        PROFILE_FIELDS.forEach(function (f) {
          if (has(p.profile, f)) m.profile[f] = pstr(write(S, 'prof:' + m.id + ':' + f, PROFILE_LABEL[f] + ' of ' + m.id, n, pstr(p.profile[f]), C));
        });
        break;
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
        S.rorder.forEach(function (id) {
          q = S.reqs[id];
          if (q.by === p.member && q.state === 'pending') { q.state = 'withdrawn'; q.ended = n; }
        });
        break;
      case 'roles.set_priority_levels':
        S.prio = write(S, 'prio', 'priority levels', n, { names: (p.names || []).slice(), default: p.default }, C);
        break;
      case 'roles.set_attachment_limit':
        S.att = { max_bytes: write(S, 'att', 'attachment limit', n, p.max_bytes, C) };
        break;
      case 'roles.add_label':
        if (!has(S.labels, n.id)) S.lorder.push(n.id);
        S.labels[n.id] = { id: n.id, name: p.name, color: p.color, removed: null };
        break;
      case 'roles.edit_label': { // name and color are field writes, merged like priority levels
        var l = has(S.labels, String(p.label)) ? S.labels[p.label] : null; if (!l) break;
        if (has(p, 'name')) l.name = write(S, 'lb:' + l.id + ':name', 'name of label ' + alias(l.id), n, p.name, C);
        if (has(p, 'color')) l.color = write(S, 'lb:' + l.id + ':color', 'color of label ' + alias(l.id), n, p.color, C);
        break;
      }
      case 'roles.remove_label':
        if (has(S.labels, String(p.label)) && !S.labels[p.label].removed) S.labels[p.label].removed = n.id;
        break;
    }
    if (/^roles\./.test(n.type) || n.type === 'item.assign') { refreshLive(S, n); settleAll(S, n, C); }
    else if (n.type === 'item.create') settle(S, n.id, n, C);
    else if (/^item\./.test(n.type)) settle(S, p.item, n, C);
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
        n.status = 'applied'; n.reason = null; n.revokedLate = false; n.clash = false;
        if (ignored[n.id]) { n.status = 'flagged'; n.reason = ignored[n.id]; n.revokedLate = true; return; }
        var S = genesis(st.roles);
        // an add or rename flagged only by the label-name clash still counts for later checks: their writers saw it
        for (var i = 0; order[i] !== n; i++) if ((order[i].status === 'applied' || order[i].clash) && hb(order[i], n)) apply(S, order[i], null);
        try { check(S, n); } catch (e) {
          if (!(e instanceof Rej)) throw e;
          n.status = 'flagged'; n.reason = e.reason;
        }
        if (n.status === 'applied' && LABEL_LIST.indexOf(n.type) >= 0) {
          // a name two admins add or rename to concurrently passes both causal checks
          var L = genesis(st.roles);
          for (var j = 0; order[j] !== n; j++) if (order[j].status === 'applied' && LABEL_LIST.indexOf(order[j].type) >= 0) apply(L, order[j], null);
          var clash = labelClash(L, n);
          if (clash) { n.status = 'flagged'; n.reason = clash; n.clash = true; }
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

    return { S: S, members: S.members, admins: S.admins, items: S.items, asg: S.asg, rules: S.rules, comments: S.comments, prio: S.prio, att: S.att,
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
      if (m.profile && !pstr(m.profile.given)) out.push(m.id + ': profile.given required');
    });
    if (roles.priority && (!Array.isArray(roles.priority.names) || !roles.priority.names.length)) out.push('priority.names must be a non-empty list');
    if (roles.attachments && !(Number.isInteger(roles.attachments.max_bytes) && roles.attachments.max_bytes > 0)) out.push('attachments.max_bytes must be a positive integer');
    return out;
  }

  // ----------------------------------------------------------- derived state

  // An item's own status from its live assignments, the fallback included, so a task
  // always has one (todo in a store with no holder to fall back on). idea only when
  // every live holder is at idea: it ranks below todo.
  function itemStatus(g, it) {
    if (it.kind === 'topic') return null;
    var recs = recsOn(g.S, it.id), live = recs.filter(function (r) { return r.state === 'live'; });
    if (!live.length) {
      // a pending rule request holds nothing, so it does not make the item pending
      return recs.some(function (r) { return r.state === 'pending' && !r.ruleRequest; }) ? 'pending' : 'todo';
    }
    var s = live.map(function (r) { return r.status; });
    if (s.every(function (x) { return x === 'done'; })) return 'done';
    if (s.indexOf('blocked') >= 0) return 'blocked';
    if (s.indexOf('awaiting') >= 0) return 'awaiting';
    if (s.some(function (x) { return x === 'doing' || x === 'done'; })) return 'doing';
    if (s.every(function (x) { return x === 'idea'; })) return 'idea';
    return 'todo';
  }
  // Pending assignments are excluded from status and roll-up: an item whose only
  // assignments are pending is not counted; nor is an item at idea.
  function counted(g, it) {
    var s = itemStatus(g, it);
    return it.kind !== 'topic' && s !== 'pending' && s !== 'idea';
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
  // The statuses a team row's load counts (613 a): the work in the member's hands.
  var TEAM_LOAD = ['todo', 'doing', 'blocked'];
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
      if (r.fallback && r.state !== 'live') return; // an ended fallback period holds nothing worth showing
      var last = r.last ? r.last.ts : r.ts, live = r.state === 'live';
      assignments[r.id] = { id: r.id, item: r.item, to: r.to, by: r.by, sender: r.sender, completion: r.completion, state: r.state,
        forced: r.forced, status: r.status, note: r.note, noteBy: r.noteBy, via: r.via, ts: r.ts,
        inherited: !!r.inherited, ruleLevels: g.rules[r.rule] ? g.rules[r.rule].levels : null,
        rule: r.rule, ruleRequest: !!r.ruleRequest, fallback: !!r.fallback,
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
    // (528) Sibling order: each item, in fold order, goes right after its `after`
    // anchor, first for null, last when the field is absent (every item created
    // before it existed) or the anchor is not a sibling placed so far. A hidden
    // anchor keeps its place, so hiding a row moves no other row. Concurrent
    // inserts after one anchor: the later in fold order lands nearer the anchor.
    var sib = {};
    vals(S.items).forEach(function (it) {
      var key = it.parent && S.items[it.parent] ? it.parent : '', l = sib[key] = sib[key] || [];
      var at = it.after == null ? -1 : l.indexOf(it.after);
      if (it.after === null) l.unshift(it.id);
      else if (at < 0) l.push(it.id);
      else l.splice(at + 1, 0, it.id);
    });
    // (531) Backlinks: the shown items, then the shown comments on shown items, that
    // reference an item, each once, in fold order; an item's own thread excluded.
    var backrefs = {};
    function backref(t, from) { var l = backrefs[t] = backrefs[t] || []; if (l.indexOf(from) < 0) l.push(from); }
    vals(S.items).forEach(function (it) {
      if (!it.hidden) it.refs.forEach(function (t) { if (t !== it.id && S.items[t]) backref(t, it.id); });
    });
    vals(S.comments).forEach(function (q) {
      var host = S.items[q.item];
      if (!q.hidden && host && !host.hidden) q.refs.forEach(function (t) { if (t !== q.item && S.items[t]) backref(t, q.id); });
    });
    var items = {};
    vals(S.items).forEach(function (it) {
      var recs = recsOn(S, it.id), d = 0, cur = it, chain = [];
      while (cur && d < 50) { chain.push(cur.author); cur = cur.parent && S.items[cur.parent]; d++; }
      var started = null;
      recs.forEach(function (r) { r.writes.forEach(function (w) { if (w.type === 'item.status' && w.p.status === 'doing' && (!started || String(w.ts) < started)) started = String(w.ts); }); });
      items[it.id] = { id: it.id, alias: it.alias, kind: it.kind, title: it.title, body: it.body, project: it.project,
        labels: it.labels.filter(function (id) { return !!liveLabel(S, id); }),
        due: it.due, priority: it.priority, priorityLevel: priorityLevel(S.prio, it.priority), priorityName: S.prio.names[priorityLevel(S.prio, it.priority)],
        parent: it.parent && S.items[it.parent] ? it.parent : null, author: it.author, ts: it.ts, hidden: !!it.hidden,
        edited: it.edited ? { id: it.edited.id, ts: it.edited.ts } : null, status: itemStatus(g, it), counted: counted(g, it),
        rollup: rollup(g, it), lastActivity: lastAct[it.id] || it.ts, started: started,
        children: (sib[it.id] || []).slice(),
        assignments: recs.filter(function (r) { return !r.fallback || r.state === 'live'; }).map(function (r) { return r.id; }),
        pending: recs.filter(function (r) { return r.state === 'pending'; }).map(function (r) { return r.id; }),
        thread: threads[it.id] || [], concurrent: (threads[it.id] || []).filter(function (id) { return g.byId[id].markers.length; }).length,
        depth: chain.length - 1, chain: chain.slice().reverse(), mentions: it.mentions.slice(), attachments: clone(it.attachments),
        refs: it.refs.slice(), backrefs: backrefs[it.id] || [], open: null, claimable: false,
        requests: S.rorder.filter(function (id) { return S.reqs[id].item === it.id; }) };
      var o = activeOpen(S, it);
      if (o) {
        items[it.id].open = { id: o.node.id, by: o.by, scope: o.scope, ts: o.node.ts };
        items[it.id].claimable = !it.hidden && liveFallback(S, it.id).to !== v && inScope(S, o, v);
      }
    });
    var requests = {};
    S.rorder.forEach(function (id) {
      var q = S.reqs[id];
      requests[id] = { id: id, item: q.item, by: q.by, assignee: q.holder, assignment: q.of, state: q.state, note: q.note,
        answerNote: q.answerNote, ts: q.node.ts, ended: q.ended ? { id: q.ended.id, type: q.ended.type, ts: q.ended.ts, by: q.ended.author } : null };
    });
    var comments = {};
    vals(S.comments).forEach(function (q) {
      comments[q.id] = { id: q.id, item: q.item, author: q.author, body: q.body, reply_to: q.reply_to, ts: q.ts,
        edited: q.edited ? { id: q.edited.id, ts: q.edited.ts } : null, hidden: q.hiddenBy ? q.hiddenBy.id : null,
        mentions: q.mentions.slice(), attachments: clone(q.attachments), refs: q.refs.slice() };
    });
    var itemOrder = [];
    (function walk(parent) { (sib[parent] || []).forEach(function (id) { itemOrder.push(id); walk(id); }); })('');
    var rules = {};
    vals(S.rules).forEach(function (u) {
      rules[u.id] = { id: u.id, item: u.item, via: u.via, by: u.by, root: u.root, includeRoot: u.includeRoot, levels: u.levels,
        completion: u.completion, state: u.ended ? 'ended' : u.state, request: u.request, ended: u.ended, ts: u.ts };
    });

    var members = {};
    Object.keys(S.members).forEach(function (id) {
      var m = S.members[id];
      members[id] = { id: id, name: displayName(m.profile), profile: clone(m.profile), avatar: avatar(m.profile), leads: m.leads.slice(), keys: m.keys.slice(),
        revoked: Object.keys(m.revoked).map(function (k) { return { key: k, by: m.revoked[k] }; }),
        removed: m.removed, joined: m.joined, admin: !!S.admins[id], children: (g.children[id] || []).slice(), depth: R[id] || null,
        role: (m.leads || []).length ? ((g.children[id] || []).length ? 'lead' : 'member') : 'root' };
    });

    // ---- viewer-specific lists
    var A = vals(assignments), RANK = { blocked: 0, doing: 1, todo: 2, idea: 3, awaiting: 4, done: 5 };
    function visible(r) { return items[r.item] && !items[r.item].hidden; }
    // (153 b) a held topic is ownership only: in my.assignments, received and the
    // pending badge, never in a status-based list or count
    function notTopic(r) { return items[r.item].kind !== 'topic'; }
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
    // time; handoff: to the original assigner when a handoff is accepted; done /
    // blocked: to the assigner when someone else set that status on a live assignment
    // (an awaiting one is in the confirm list), keyed by status and time so a later
    // write of the same status is a new notice; declined / returned (285): to the
    // sender, and for a returned handoff also to the original assigner (291).
    var notices = [];
    A.forEach(function (r) {
      var base = { assignment: r.id, item: r.item, to: r.to, by: r.by, sender: r.sender, state: r.state };
      if (r.accepted) {
        var b1 = Object.assign({ ts: r.accepted.ts }, base);
        if (r.leadAtAccept === v) notices.push(Object.assign({ key: 'acc:' + r.id, kind: 'accepted' }, b1));
        else if (r.handoffOf && assignments[r.handoffOf] && assignments[r.handoffOf].by === v) notices.push(Object.assign({ key: 'ho:' + r.id, kind: 'handoff', from: r.handoffFrom }, b1));
      }
      if (r.by === v && r.to !== v && r.state === 'live' && (r.status === 'done' || r.status === 'blocked') && r.statusBy && r.statusBy !== v)
        notices.push(Object.assign({ key: 'w:' + r.id + ':' + r.status + ':' + r.lastUpdate, kind: r.status, ts: r.lastUpdate }, base));
      if (r.to === v || !r.ended) return;
      var b2 = Object.assign({ ts: r.ended.ts, endedBy: r.ended.by, note: r.answerNote, from: r.handoffFrom, restores: r.restores }, base);
      if (r.state === 'declined' && r.sender === v) notices.push(Object.assign({ key: 'dec:' + r.id, kind: 'declined' }, b2));
      if (r.state === 'returned' && (r.sender === v || (r.handoffOf && r.by === v)) && r.ended.by !== v) notices.push(Object.assign({ key: 'ret:' + r.id, kind: 'returned' }, b2));
    });

    // (613 a) The team roll-up: a lead sees their whole subtree, in tree order with
    // depth from the viewer; an admin also sees every other live member, after it, as
    // the outside group (outside: true, depth from the org-tree root: a root is 0).
    // Per member: the open, pending, and waiting lists; counts, the member's live
    // assignments per status, a creator-fallback holding left out ((262 a): nobody took
    // that work); load, those at todo, doing, or blocked (idea, awaiting, and done are
    // not the member's work in hand); stale and overdue (the item's due
    // date passed), subsets of load; fallback, the live fallback holdings not done.
    var today = String(now).slice(0, 10), team = [], inTeam = {};
    inTeam[v] = true;
    function teamRow(id, depth, outside) {
      inTeam[id] = true;
      var recs = A.filter(function (r) { return r.to === id && visible(r) && notTopic(r); });
      var open = recs.filter(function (r) { return r.state === 'live' && r.status !== 'done'; }).sort(byRank);
      var pend = recs.filter(function (r) { return r.state === 'pending'; });
      var waiting = A.filter(function (r) { return r.sender === id && r.to !== id && r.state === 'pending' && visible(r) && notTopic(r); });
      var row = { id: id, depth: depth, outside: outside, open: ids(open), pending: ids(pend), waiting: ids(waiting),
        counts: {}, load: 0, stale: 0, overdue: 0, fallback: 0, last: recs.map(function (r) { return r.lastUpdate; }).sort().pop() || null };
      recs.forEach(function (r) {
        if (r.state !== 'live') return;
        if (r.fallback) { if (r.status !== 'done') row.fallback++; return; }
        row.counts[r.status] = (row.counts[r.status] || 0) + 1;
        if (TEAM_LOAD.indexOf(r.status) < 0) return;
        row.load++;
        if (r.stale) row.stale++;
        var due = items[r.item].due;
        if (today && due && due < today) row.overdue++;
      });
      team.push(row);
    }
    (function walk(lead) {
      (g.children[lead] || []).forEach(function (id) {
        if (inTeam[id] || !(id in R)) return;
        teamRow(id, R[id], false);
        walk(id);
      });
    })(v);
    if (S.admins[v]) {
      var outside = function (list, depth) {
        list.forEach(function (id) {
          if (inTeam[id]) return;
          inTeam[id] = true; // a removed root is passed through, not shown
          if (!S.members[id].removed) teamRow(id, depth, true);
          outside(g.children[id] || [], depth + 1);
        });
      };
      outside(Object.keys(S.members).filter(function (id) { return !(S.members[id].leads || []).length; }), 0);
      // a member no root reaches (a lead chain through a removed lead) roots its own branch
      Object.keys(S.members).forEach(function (id) { if (!S.members[id].removed) outside([id], 0); });
    }

    // Pending take requests on shown items: incoming, those the viewer may answer
    // (the named assignment's holder or the item's creator) and did not file;
    // outgoing, those the viewer filed.
    var reqsMine = { incoming: [], outgoing: [] };
    S.rorder.forEach(function (id) {
      var q = S.reqs[id], it = S.items[q.item];
      if (q.state !== 'pending' || !it || it.hidden) return;
      if (q.by === v) reqsMine.outgoing.push(id);
      else if (q.holder === v || it.author === v || creatorGone(S, it, v)) reqsMine.incoming.push(id);
    });

    var badge = [];
    mine.forEach(function (r) { if (r.by !== v && r.forced && r.status === 'todo' && notTopic(r)) badge.push('a:' + r.id); });
    pendingToMe.forEach(function (r) { badge.push('p:' + r.id); });
    reqsMine.incoming.forEach(function (id) { badge.push('q:' + id); });
    notices.forEach(function (x) { badge.push(x.key); });

    var commits = 0;
    Object.keys(input.commits || {}).forEach(function (k) { commits += input.commits[k].length; });

    return {
      viewer: { member: v, stream: stream, name: me ? displayName(me.profile) : v, profile: me ? clone(me.profile) : null, admin: !!S.admins[v], lead: me ? (me.leads || [])[0] || null : null,
        followers: (g.children[v] || []).slice(), reach: R, isMember: !!(me && !me.removed) },
      priority: clone(S.prio), attachments: clone(S.att),
      labels: S.lorder.filter(function (id) { return !!liveLabel(S, id); }).map(function (id) { var l = S.labels[id]; return { id: id, name: l.name, color: l.color }; }),
      members: members, admins: Object.keys(S.admins), rolesProblems: rolesProblems(input.roles),
      items: items, itemOrder: itemOrder, assignments: assignments, requests: requests, rules: rules, comments: comments, events: events, notices: notices,
      my: {
        assignments: ids(mine.sort(byRank)),
        today: { work: ids(mine.filter(function (r) { return r.status !== 'done' && r.status !== 'awaiting' && notTopic(r); }).sort(byUrgency)), pending: ids(pendingToMe.sort(byUrgency)), confirms: ids(confirms) },
        received: { pending: ids(pendingToMe), answered: ids(answered), sent: ids(sent) },
        given: given, created: created, team: team, requests: reqsMine, badge: badge
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
      // A new clone (the join dialog's stand-in for a fresh checkout): online, empty, and
      // it fetches what the remote holds.
      addClone: function (s, now) {
        if (state.clones[s]) throw new Error('clone stream "' + s + '" already exists');
        state.clones[s] = { online: true, syncedAt: null, have: {}, late: {}, local: [] };
        if (hasRemote) { receive(s); state.clones[s].syncedAt = now || null; }
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
    DEFAULT_ATTACHMENT_MAX: DEFAULT_ATTACHMENT_MAX,
    displayName: displayName, avatar: avatar, normProfile: normProfile,
    alias: alias, memberOf: memberOf, cloneOf: cloneOf, refOf: refOf, reach: reach,
    openStore: openStore, deriveViewState: deriveViewState
  };
})();
