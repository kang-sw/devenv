// Tracker renderer: the shared view of tracker stores - a pull-down drawer with
// one handle per store, the store page (route #/<storeId>), the item window, the
// quick switcher, the item context menu with its inline inputs, and the store-wide
// dialogs (profile, admin, "+ store"). It renders nothing until the host page
// calls WsTracker.mount(adapter, opts). The adapter is its only data source: the
// renderer reads the plain view-state object a store serves and writes only by
// dispatching events, so the same file runs over any data layer serving that JSON.
//
// adapter (data; every store-keyed call takes the registry store id):
//   stores()          -> ordered array of store ids (handle order)
//   view(id)          -> the viewer's latest view-state object, or null while it is
//                        not loaded or could not be read. Synchronous. The viewer
//                        is view.viewer (stream, member); "today" is
//                        view.now.slice(0, 10). The renderer keeps no viewer.
//   detail(id, ids, done?) -> optional: request visible/expanded bundles; returns
//                        whether all ids have detail matching their summary row.
//                        done runs when matching detail is installed. view(id)
//                        exposes no mismatched body/comments/events/HTML.
//   detailReady(id, item), detailError(id), retryDetail(id, item)
//                        optional matching-detail state and explicit retry.
//   history(id)       -> optional: request global event history for Activity/sync;
//                        true when view.historyEvents is current. Does not install
//                        unversioned history as item detail.
//   historyError(id), retryHistory(id) -> optional history error/retry hooks.
//   error(id)         -> the reason string when view(id) is null, else null
//   dispatch(id, events, done)
//                        events = [{type, payload, origin}]. A payload's item,
//                        parent, comment, or reply_to field holding the string "$N"
//                        (N >= 1, an earlier event of the same batch by 1-based
//                        position) is the id that event gets; the adapter resolves
//                        it, the renderer never computes an event id. done(res)
//                        exactly once, possibly synchronously: {ok: true, results:
//                        [{id, alias, type, status, reason}]} or {ok: false, error,
//                        refusal: {index, type, reason} | null}
//   check(id, done)      the immediate check when a store's drawer opens or its
//                        page is entered; a change arrives through onChange.
//                        done() exactly once, possibly synchronously, after the
//                        check landed and view(id) shows what it brought
//   sync(id, done)       the apply path: panel collapse, leaving the page, switching
//                        to another store while one is open, the status ribbon's
//                        apply button.
//                        done(report), report = {noRemote, offline, closed, pushed,
//                        received, unpushed, error}; absent fields read as
//                        false / 0 / empty
//   seen(id)          -> {badge: {key: true}, read: {key: true}}: what this viewer
//                        has seen (handle-badge seen marks, notification read marks)
//   markSeen(id, {badge: [keys], read: [keys]})
//                        adds marks (a union; nothing is unmarked); the adapter
//                        updates its own copy at once and persists
//   profiles(done)       done({last: <index or -1>, profiles: [{org, family,
//                        given}]}): the machine-local saved profiles
//   saveProfile(profile) saves one {org, family, given}
//   register(req, done)  optional, for the "+ store" handle and its dialog, which
//                        an adapter without it does not get: req = {id, label,
//                        remote, member, profile: {org, family, given}} (remote may
//                        be empty: a store on this machine only). It creates the
//                        store (no remote, or an empty repository) or joins the one
//                        the remote holds, and saves the profile as the one used
//                        last. done({ok: true, store: <id>, creator: <bool>} | {ok:
//                        false, error}); creator: this registration created the
//                        store (false: it joined). done comes after stores() holds
//                        the new store, which the renderer then opens
//   render(id, req, done)
//                        optional, for the editors' preview: renders markdown req.text
//                        the way the view's html renders an item body or comment text,
//                        with req.attachments ([{oid, name, size, mime}]: the ones the
//                        editor's save would carry, uploads included) as the text's own,
//                        so an ![name](attachment:<oid>) image of one shows.
//                        done({ok: true, html} | {ok: false, error}). Without it the
//                        preview shows the plain text (the mockup)
//   upload(id, file, name, done)
//                        optional, for an image pasted or dropped into an editor: stages
//                        the file's bytes for a later write; name is the dropped file's
//                        name, '' for a clipboard paste. done({ok: true, attachment:
//                        {oid, name, size, mime}} | {ok: false, error}). Without it a
//                        pasted or dropped image is refused with a notice (the mockup)
//                        render's req.wiki names the wiki an editor's text belongs to
//                        (the wiki editor), so the wiki tree's images show as well
//   wiki                 optional: the store's wiki branches; without it the Items
//                        panel shows no Items | Wiki switch and no Wiki tab.
//     list(id, done)       done({ok: true, wikis: [{name, tip, unpushed}]} | err)
//     pages(id, wiki, done) done({ok: true, pages: [path], commit} | err)
//     read(id, wiki, path, done)
//                          done({ok: true, path, content, commit, html} | err);
//                          html is the page rendered as the view renders a body
//     save(id, wiki, req, done)
//                          req = {base, newWiki, op, path, to, content, commit,
//                          message, attachments}; done({ok: true, commit, merged,
//                          created, warnings} | err); a save that could not merge is
//                          err with status 409 and conflict = {path, kind, reason,
//                          base, theirs, oursText, theirsText}
//                        err is {ok: false, error, status} (status 0: not reached)
//   search(id, q, done, o)  optional, for the Items panel's search box: the ids of the
//                        store's items that pass q, parsed by the tracker's search
//                        grammar (the daemon parses it, so the box and the agents'
//                        tracker.query q agree). done({ok: true, ids, hits} | {ok: false,
//                        error, status}); an error is a malformed query (status 400),
//                        shown under the box with the list left unfiltered. o.excerpts
//                        also asks for hits, [{id, title, alias, kind, status, hidden,
//                        field (body, comment, or item), comment, text}] (the palette's
//                        tracker content slot); o.signal aborts the request. Without it
//                        the box matches a plain substring of the title, body, project,
//                        and id (the mockup)
//   hidden()          -> optional: the ids of the stores hidden on this machine, which
//                        stores() leaves out; the "+ store" dialog lists them
//   info(id)          -> optional: {id, label, remote, hidden}, the registry entry of
//                        a store in stores() or hidden() (label: '' when the store has
//                        none), or null; the store menu and its dialogs read it
//   relabel(id, label, done), setHidden(id, hidden, done)
//                        optional, for the store menu and the hidden list: set this
//                        machine's label of the store ('' clears it) or its hidden
//                        flag. done({ok: true} | {ok: false, error}), after stores(),
//                        hidden(), and info() show the change
//   unregister(id, confirm, done)
//                        optional, for the store menu and the hidden list: delete the
//                        store's local copy from this machine (the remote keeps it).
//                        confirm is the typed store id, which a store with no remote
//                        requires. done({ok: true} | {ok: false, error, kind}), after
//                        stores() no longer holds it; kind is the refusal's (confirm,
//                        busy, unpushed, stuck). A store that leaves stores() (hidden or
//                        unregistered) drops from every state that names it, unsynced
//   hostError()       -> optional: the reason the store list could not be read (the
//                        host is unavailable), or null; the "+ store" dialog shows it
//                        and offers nothing else
//   onChange(fn)         the renderer subscribes once; the adapter calls fn()
//                        whenever stores(), a view(), or seen() changed; fn(true)
//                        marks a change more of which follows shortly (one store's
//                        view of a read of several), which the renderer holds back
//                        briefly to draw with the rest
//   caps                 {origin, attach, addMember} booleans, false when absent:
//                        origin shows the origin pickers (human / agent); attach
//                        the composer's attachment drafting and the simulated
//                        attachment load toggle; addMember offers roles.add_member
//                        in the admin dialog
//   ext                  optional host chrome:
//     tabs               [{key, icon, title, tip, after, make(id, view, h)}]: extra
//                        side tabs. icon is an SVG <use> href resolving in the host
//                        page; after is the tab key it follows (absent: last); make
//                        returns a card spec (h.card)
//     syncTools(id, view, h)  -> extra HTML at the end of the store status popover
//                        (opened from the Items card's status ribbon)
//     teamTools(id, view, h)  -> extra HTML in the Team tab's toolbar
//     qsActions(id, view, q)  -> [{key, icon, title, hint, tag}]: quick-switcher
//                        actions for query q; picking one opens dialogs[key], else
//                        calls action(key, null, h)
//     dialogs            {kind: {html(id, view, d, h) -> {title, intro, body,
//                        submit}, submit(id, form, h)}}: dialogs a data-act="<kind>"
//                        opens
//     action(k, el, h)   -> true when it handled a data-act the renderer does not know
//     rendered(id, view, h)   called after every render, before the drawer is
//                        sized (view is null while the store is not loaded)
//   h, the helper bag ext functions get: esc, attr, btn, card, fld, fmtTs, fmtSize,
//     nm, nmo, memberOpts, profileFields, profileText, asciiId, crefHtml,
//     uiCommand(id, {op, arg, comment}) (a support chat's UI command on store id's view:
//     search, focus, scope, or view, as the user would; see uiCommand),
//     openDialog(kind, d), closeDialog(), wrote(results) (a write the host made itself:
//     the last-write line and the per-event toast, as after a dialog), toast(html),
//     refresh(forget) (re-render; forget also clears the last-write line)
//
// opts:
//   header           the page header element; the drawer hangs from its bottom edge
//   qsSlot           optional element in the header for the search-palette button
//                    (omitted when searchHost owns the trigger)
//   searchHost       optional {parent, setOpen(bool), contains(target)} sheet host;
//                    the returned API's openSearch/closeSearch follows host switches
//   searchState      optional {load(), save({query, slot})} browser persistence
//   searchRows(update) optional host rows: {group (1..5), kind, title, text?,
//                    hint?, id?, topic?, entry?, at?}: topics, bodies, items,
//                    answers and entries (group 5, kind board-entry, entry id).
//                    Legacy group-0 session rows are ignored.
//                    Read on open; update(rows) may replace them as older board
//                    pages load.
//   searchPick(row, o) opens a selected host row; o.keep (Ctrl+Enter) leaves the
//                    focus in the open palette. Tracker rows keep their actions
//   The palette's file slots, each optional (without them a slot stays idle):
//   fileFind(q, signal)   -> Promise of {items: [{path, dir}]}, a name search
//   fileSearch(q, signal) -> Promise of {files: [{path, matches: [{line, pre, hit,
//                    post}]}], more}, a literal content search
//   fileList(dir, signal) -> Promise of {entries: [{name, dir}]}, the absolute
//                    directory dir's entries; rejects with {status, text}
//   filePreview(path, {root?, line?})  shows a file (relative to root, else to the
//                    session root) at an optional line
//   fileOpen(name, dir)   opens file name of absolute directory dir in the host's
//                    default app
//   fileLocal()      -> whether the browser is on the host's machine (path mode)
//   pageParent       where the store page node goes; while a store page shows, the
//                    body carries the class tk-paged and the host's CSS hides its
//                    own conversation column and session drawer
//   storagePrefix    browser-storage key prefix for display state (view modes,
//                    recently-closed fold, tree folds); default "ws.tracker."
//   drawerHeight()   optional read-only maximum height in px, including the host's
//                    composer clearance; without it drawers fill the viewport
//                    below the header. Heights are never persisted.
//   drawerBoundary   optional host geometry element (e.g. the composer) observed
//                    alongside header to refit drawers on resize or display changes
//   onRender(u)      called after every render with the rendered store's unread
//                    notifications, u = api.unread()
//
// The renderer creates its own nodes (tkdrawer, tkclip, tkbody, tkhandles, tkpage,
// tkwin, tkdlg, tkmenu, tktip, tkqs, ...); each root carries the class tk, which
// scopes tracker.css. Its handle row sits at the top right, left of the host's own
// handle: right = --dpad + --drhw (the host's handle width) + 6px. The renderer sets
// --tkhw on the root element to the row's width plus that gap (0 when the row is
// empty), and the host header pads clear of --drhw and --tkhw. Opening a store's
// drawer dispatches the document event tk:drawer-open, on which the host closes its
// own drawer; the host dispatching ws:drawer-open collapses an open store (a
// collapse, so it syncs).
//
// api = WsTracker.mount(adapter, opts):
//   refresh()                 re-render
//   openStore(id)             toggle a store's drawer, as a handle click
//   closeStore()              collapse the open drawer (syncs)
//   openItemWindow(storeId, itemId, opts)
//                             opts {focus: eventId, opener: element}; false when the
//                             store or the item is unknown to the viewer; also
//                             window.openItemWindow
//   viewerChanged(id)         the adapter's viewer of a store changed: checks it
//                             and marks seen when the store is on screen, clears
//                             the last-write line, re-renders
//   toast(html)               a transient message: the page's shared toast stack
//                             (window.WsToast) when it has one, else the renderer's
//                             own #tktoast
//   unread()                  {store, ready, entries}: the rendered store's unread
//                             notifications, newest first, each {store, item, ev, key,
//                             kind, label, title, author} in plain text (never the
//                             feed's HTML); ready is false while the store is not
//                             loaded, when entries is empty
//   openNotice(entry)         open an unread() entry's item at what it refers to and
//                             mark it read, as a click on its Notify row does
//   openNotify(id)            bring store id's Notify tab up (its drawer, or its store
//                             page when one shows)
// window.__tracker = {ui, vs(), openItemWindow} is a read-only hook for scripted checks.
(function () {
  'use strict';

  // Pure helpers of the view-state model: ids, clone streams, and the profile
  // display rules (a name with Hangul, a CJK ideograph, or kana is family + given
  // with no space, else given + family; the avatar is up to two characters).
  function alias(id) { return String(id).replace('#', '-'); }
  function memberOf(stream) { var s = String(stream), i = s.indexOf('.'); return i < 0 ? s : s.slice(0, i); }
  function cloneOf(stream) { var s = String(stream), i = s.indexOf('.'); return i < 0 ? '' : s.slice(i + 1); }
  function refOf(stream) { return 'refs/heads/tracker/m/' + memberOf(stream) + '/' + cloneOf(stream); }
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
  // Derive the org forest from a members map (and an optional id -> leadId|'' override map):
  // returns nested tree roots ({ id, depth, count, children }), unplaced live members (no live
  // lead and no live followers), removed members, unreachable live members (e.g. cycles),
  // plus lookup maps (parentOf, depthOf, countOf). Shared by cardTeam, the org editor, and pickers.
  function orgTree(members, leadsOverride) {
    var M = members || {}, ids = Object.keys(M), hasOv = !!leadsOverride;
    function live(id) { return !!(id && M[id] && !M[id].removed); }
    var parentOf = {}, kids = {}, removed = [], liveIds = [];
    ids.forEach(function (id) {
      if (!live(id)) { removed.push(id); return; }
      liveIds.push(id);
      kids[id] = [];
    });
    liveIds.forEach(function (id) {
      var m = M[id], raw;
      if (hasOv && Object.prototype.hasOwnProperty.call(leadsOverride, id)) raw = leadsOverride[id] || '';
      else raw = m.leads && m.leads.length ? m.leads[0] : '';
      parentOf[id] = live(raw) ? raw : '';
    });
    liveIds.forEach(function (id) {
      var m = M[id];
      if (!hasOv && Array.isArray(m.children)) {
        m.children.forEach(function (cid) {
          if (live(cid) && parentOf[cid] === id && kids[id].indexOf(cid) < 0) kids[id].push(cid);
        });
      }
    });
    liveIds.forEach(function (id) {
      var p = parentOf[id];
      if (p && kids[p].indexOf(id) < 0) kids[p].push(id);
    });
    var seen = {}, depthOf = {}, countOf = {};
    function build(id, depth) {
      seen[id] = true;
      depthOf[id] = depth;
      var ch = [], total = 0;
      kids[id].forEach(function (cid) {
        if (seen[cid]) { ch.push({ id: cid, depth: depth + 1, count: 0, children: [], cycle: true }); return; }
        var sub = build(cid, depth + 1);
        ch.push(sub);
        total += 1 + sub.count;
      });
      countOf[id] = total;
      return { id: id, depth: depth, count: total, children: ch, cycle: false };
    }
    var roots = [], unplaced = [];
    liveIds.forEach(function (id) {
      if (parentOf[id]) return;
      if (kids[id].length) roots.push(build(id, 0));
      else { seen[id] = true; depthOf[id] = 0; countOf[id] = 0; unplaced.push(id); }
    });
    var unreachable = liveIds.filter(function (id) { return !seen[id]; });
    return { roots: roots, unplaced: unplaced, removed: removed, unreachable: unreachable, parentOf: parentOf, depthOf: depthOf, countOf: countOf };
  }
  // Indexes org entities alongside the org tree: live/removed partitions, head lookup,
  // subtree size, nearest enclosing org for any member, and subtree membership.
  function orgIndex(orgs, members, leadsOverride) {
    var M = members || {}, ot = orgTree(M, leadsOverride);
    var list = Array.isArray(orgs) ? orgs : (orgs ? Object.keys(orgs).map(function (k) { return orgs[k]; }) : []);
    var byId = {}, live = [], removed = [], headToOrg = {};
    function isLiveMember(id) { return !!(id && M[id] && !M[id].removed); }
    list.forEach(function (o) {
      if (!o || !o.id) return;
      var rec = { id: o.id, name: o.name || '', head: o.head || '', removed: o.removed || null };
      byId[rec.id] = rec;
      if (rec.removed) removed.push(rec);
      else {
        live.push(rec);
        if (rec.head && isLiveMember(rec.head)) headToOrg[rec.head] = rec;
      }
    });
    function sizeOf(id) {
      var o = byId[id];
      if (!o || o.removed || !o.head || !isLiveMember(o.head)) return 0;
      return (ot.countOf[o.head] || 0) + 1;
    }
    function nearestOrg(memberId) {
      if (!isLiveMember(memberId)) return null;
      var seen = {}, x = memberId;
      while (x && !seen[x]) {
        if (headToOrg[x]) return headToOrg[x];
        seen[x] = true;
        x = ot.parentOf[x] || '';
      }
      return null;
    }
    function orgSubtree(id) {
      var o = byId[id];
      if (!o || o.removed || !o.head || !isLiveMember(o.head)) return [];
      var out = [];
      function walk(node, inside) {
        if (!node || node.cycle) return;
        var hit = inside || node.id === o.head;
        if (hit) out.push(node.id);
        (node.children || []).forEach(function (c) { walk(c, hit); });
      }
      ot.roots.forEach(function (r) { walk(r, false); });
      if (!out.length) out.push(o.head);
      return out;
    }
    return { tree: ot, byId: byId, live: live, removed: removed, headToOrg: headToOrg, sizeOf: sizeOf, nearestOrg: nearestOrg, orgSubtree: orgSubtree };
  }
  // Mints a client-side org id (`o-<4-char base36>`) avoiding every id in existingIds.
  function orgMintId(existingIds, rng) {
    var used = {};
    if (Array.isArray(existingIds)) existingIds.forEach(function (id) { used[id] = true; });
    else if (existingIds) Object.keys(existingIds).forEach(function (id) { used[id] = true; });
    var fn = typeof rng === 'function' ? rng : Math.random;
    var chars = '0123456789abcdefghijklmnopqrstuvwxyz';
    for (var tries = 0; tries < 256; tries++) {
      var s = 'o-';
      for (var i = 0; i < 4; i++) s += chars[Math.floor(fn() * 36) % 36];
      if (!used[s]) return s;
    }
    for (var n = 0; ; n++) {
      var fb = 'o-' + n.toString(36).padStart(4, '0');
      if (!used[fb]) return fb;
    }
  }
  // Maps each org id to non-hidden items referencing it via open assign rules or org CC registrations.
  function orgItemRefs(orgs, items, rules, itemOrder) {
    var I = items || {}, R = rules || {}, out = {};
    var ruleList = Array.isArray(R) ? R : Object.keys(R).map(function (k) { return R[k]; });
    var order = Array.isArray(itemOrder) && itemOrder.length ? itemOrder : Object.keys(I);
    var rank = {};
    order.forEach(function (id, i) { rank[id] = i; });
    function ensure(oid) {
      if (!out[oid]) out[oid] = { byItem: {}, items: [], assignCount: 0, ccCount: 0, count: 0 };
      return out[oid];
    }
    function touch(oid, itemId, field) {
      var it = I[itemId];
      if (!it || it.hidden) return;
      var slot = ensure(oid), entry = slot.byItem[itemId];
      if (!entry) {
        entry = { id: itemId, alias: it.alias || alias(itemId), title: it.title || '', status: it.status || '', assign: false, cc: false };
        slot.byItem[itemId] = entry;
        slot.items.push(entry);
      }
      if (field === 'assign' && !entry.assign) { entry.assign = true; slot.assignCount++; }
      if (field === 'cc' && !entry.cc) { entry.cc = true; slot.ccCount++; }
    }
    ruleList.forEach(function (ru) {
      if (!ru || !ru.org || ru.ended) return;
      touch(ru.org, ru.item, 'assign');
    });
    order.forEach(function (id) {
      var it = I[id];
      if (!it || it.hidden || !Array.isArray(it.orgCC)) return;
      it.orgCC.forEach(function (reg) {
        if (reg && reg.org) touch(reg.org, id, 'cc');
      });
    });
    Object.keys(out).forEach(function (oid) {
      out[oid].items.sort(function (a, b) {
        var ra = Object.prototype.hasOwnProperty.call(rank, a.id) ? rank[a.id] : 1e9;
        var rb = Object.prototype.hasOwnProperty.call(rank, b.id) ? rank[b.id] : 1e9;
        return ra !== rb ? ra - rb : (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
      });
      out[oid].count = out[oid].items.length;
      delete out[oid].byItem;
    });
    return out;
  }
  // Pure state and validation helpers for the admin org-chart editor modal (Decisions 4-12, 18).
  function orgEdInit(members, viewer, orgs) {
    var ot = orgTree(members), baseLeads = {}, baseAdmins = {}, M = members || {};
    Object.keys(ot.parentOf).forEach(function (id) {
      baseLeads[id] = ot.parentOf[id] || '';
      baseAdmins[id] = !!(M[id] && M[id].admin);
    });
    var baseOrgs = {}, baseOrgOrder = [];
    var olist = Array.isArray(orgs) ? orgs : (orgs ? Object.keys(orgs).map(function (k) { return orgs[k]; }) : []);
    olist.forEach(function (o) {
      if (!o || !o.id) return;
      baseOrgs[o.id] = { id: o.id, name: o.name || '', head: o.head || '', removed: o.removed || null };
      baseOrgOrder.push(o.id);
    });
    return {
      viewer: viewer || '', baseLeads: baseLeads, baseAdmins: baseAdmins,
      baseOrgs: baseOrgs, baseOrgOrder: baseOrgOrder,
      stagedLeads: {}, stagedAdmins: {}, stagedOrgs: {},
      fold: {}, goneOpen: {}, editingOrg: null, q: '',
      drag: null, dragKind: null, over: null, refuse: null, lastUndoErr: null, errors: {}, saving: false
    };
  }
  function orgEdProjectedOrgs(ed) {
    var out = {}, bo = ed.baseOrgs || {}, so = ed.stagedOrgs || {};
    Object.keys(bo).forEach(function (id) {
      var o = bo[id], st = so[id];
      if (o.removed) return;
      if (st && st.op === 'delete') return;
      if (st && st.op === 'edit') {
        out[id] = { id: id, name: st.name, head: st.head || '', removed: null, staged: 'edit' };
      } else {
        out[id] = { id: id, name: o.name, head: o.head || '', removed: null, staged: null };
      }
    });
    Object.keys(so).forEach(function (id) {
      var st = so[id];
      if (st && st.op === 'create') {
        out[id] = { id: id, name: st.name, head: st.head || '', removed: null, staged: 'create' };
      }
    });
    return out;
  }
  function orgEdAllDisplayOrgs(ed) {
    var out = [], bo = ed.baseOrgs || {}, so = ed.stagedOrgs || {}, seen = {};
    (ed.baseOrgOrder || Object.keys(bo)).forEach(function (id) {
      var o = bo[id];
      if (!o || o.removed) return;
      seen[id] = true;
      var st = so[id];
      if (st && st.op === 'delete') {
        out.push({ id: id, name: o.name, head: o.head || '', removed: null, staged: 'delete' });
      } else if (st && st.op === 'edit') {
        out.push({ id: id, name: st.name, head: st.head || '', removed: null, staged: 'edit' });
      } else {
        out.push({ id: id, name: o.name, head: o.head || '', removed: null, staged: null });
      }
    });
    Object.keys(so).forEach(function (id) {
      if (seen[id]) return;
      var st = so[id];
      if (st && st.op === 'create') {
        out.push({ id: id, name: st.name, head: st.head || '', removed: null, staged: 'create' });
      }
    });
    return out;
  }
  function orgEdHeadOf(ed, memberId) {
    if (!memberId) return null;
    var proj = orgEdProjectedOrgs(ed), ids = Object.keys(proj);
    for (var i = 0; i < ids.length; i++) {
      if (proj[ids[i]].head === memberId) return proj[ids[i]];
    }
    return null;
  }
  function orgEdNameClash(ed, name, excludeId) {
    var low = String(name || '').trim().toLowerCase();
    if (!low) return null;
    var proj = orgEdProjectedOrgs(ed), ids = Object.keys(proj);
    for (var i = 0; i < ids.length; i++) {
      var o = proj[ids[i]];
      if (o.id !== excludeId && String(o.name || '').trim().toLowerCase() === low) return o;
    }
    return null;
  }
  function orgEdCreateOrg(ed, name, head, rng) {
    var trimmed = String(name || '').trim();
    if (!trimmed) return { ok: false, reason: 'blank_name' };
    var clash = orgEdNameClash(ed, trimmed, '');
    if (clash) return { ok: false, reason: 'dup_name', other: clash };
    var target = head || '';
    if (target) {
      if (!Object.prototype.hasOwnProperty.call(ed.baseLeads, target)) return { ok: false, reason: 'unknown_member' };
      var held = orgEdHeadOf(ed, target);
      if (held) return { ok: false, reason: 'already_head', other: held };
    }
    var used = {};
    Object.keys(ed.baseOrgs || {}).forEach(function (id) { used[id] = true; });
    Object.keys(ed.stagedOrgs || {}).forEach(function (id) { used[id] = true; });
    var id = orgMintId(used, rng);
    ed.stagedOrgs[id] = { op: 'create', name: trimmed, head: target };
    delete ed.errors['org:' + id];
    ed.refuse = null;
    return { ok: true, id: id };
  }
  function orgEdRenameOrg(ed, orgId, newName) {
    var proj = orgEdProjectedOrgs(ed), cur = proj[orgId];
    if (!cur) return { ok: false, reason: 'unknown_org' };
    var trimmed = String(newName || '').trim();
    if (!trimmed) return { ok: false, reason: 'blank_name' };
    var clash = orgEdNameClash(ed, trimmed, orgId);
    if (clash) return { ok: false, reason: 'dup_name', other: clash };
    var st = ed.stagedOrgs[orgId];
    if (st && st.op === 'create') {
      st.name = trimmed;
    } else {
      var base = ed.baseOrgs[orgId];
      if (trimmed === base.name && cur.head === base.head) delete ed.stagedOrgs[orgId];
      else ed.stagedOrgs[orgId] = { op: 'edit', name: trimmed, head: cur.head };
    }
    delete ed.errors['org:' + orgId];
    return { ok: true };
  }
  function orgEdStageOrgHead(ed, orgId, newHead) {
    var proj = orgEdProjectedOrgs(ed), cur = proj[orgId];
    if (!cur) return { ok: false, reason: 'unknown_org' };
    var target = newHead || '';
    if (target) {
      if (!Object.prototype.hasOwnProperty.call(ed.baseLeads, target)) return { ok: false, reason: 'unknown_member' };
      var held = orgEdHeadOf(ed, target);
      if (held && held.id !== orgId) return { ok: false, reason: 'already_head', other: held };
    }
    var st = ed.stagedOrgs[orgId];
    if (st && st.op === 'create') {
      st.head = target;
    } else {
      var base = ed.baseOrgs[orgId];
      if (cur.name === base.name && target === base.head) delete ed.stagedOrgs[orgId];
      else ed.stagedOrgs[orgId] = { op: 'edit', name: cur.name, head: target };
    }
    delete ed.errors['org:' + orgId];
    ed.refuse = null;
    return { ok: true };
  }
  function orgEdDeleteOrg(ed, orgId) {
    var st = ed.stagedOrgs[orgId];
    if (st && st.op === 'create') {
      delete ed.stagedOrgs[orgId];
      delete ed.errors['org:' + orgId];
      if (ed.editingOrg === orgId) ed.editingOrg = null;
      return { ok: true };
    }
    var base = ed.baseOrgs && ed.baseOrgs[orgId];
    if (!base || base.removed) return { ok: false, reason: 'unknown_org' };
    ed.stagedOrgs[orgId] = { op: 'delete' };
    delete ed.errors['org:' + orgId];
    if (ed.editingOrg === orgId) ed.editingOrg = null;
    ed.refuse = null;
    return { ok: true };
  }
  function orgEdProjectedLeads(ed) {
    var out = {};
    Object.keys(ed.baseLeads).forEach(function (id) { out[id] = ed.baseLeads[id]; });
    Object.keys(ed.stagedLeads).forEach(function (id) { out[id] = ed.stagedLeads[id]; });
    return out;
  }
  function orgEdProjectedAdmins(ed) {
    var out = {};
    Object.keys(ed.baseAdmins).forEach(function (id) { out[id] = !!ed.baseAdmins[id]; });
    Object.keys(ed.stagedAdmins).forEach(function (id) { out[id] = !!ed.stagedAdmins[id]; });
    return out;
  }
  function orgEdAdminCount(ed) {
    var pa = orgEdProjectedAdmins(ed), n = 0;
    Object.keys(pa).forEach(function (id) { if (pa[id]) n++; });
    return n;
  }
  function orgEdIsInSubtree(leadsMap, candidate, memberId) {
    var seen = {};
    for (var x = candidate; x && !seen[x]; x = leadsMap[x]) {
      if (x === memberId) return true;
      seen[x] = true;
    }
    return false;
  }
  function orgEdCanRemoveAdmin(ed, id) {
    var pa = orgEdProjectedAdmins(ed);
    return !pa[id] || orgEdAdminCount(ed) > 1;
  }
  function orgEdStageLead(ed, memberId, newLead) {
    var target = newLead || '';
    if (!Object.prototype.hasOwnProperty.call(ed.baseLeads, memberId)) return { ok: false, reason: 'unknown_member' };
    var leads = orgEdProjectedLeads(ed);
    if (target && orgEdIsInSubtree(leads, target, memberId)) return { ok: false, reason: 'cycle' };
    if (target === (ed.baseLeads[memberId] || '')) delete ed.stagedLeads[memberId];
    else ed.stagedLeads[memberId] = target;
    delete ed.errors['lead:' + memberId];
    ed.refuse = null;
    return { ok: true };
  }
  function orgEdToggleAdmin(ed, id) {
    if (!Object.prototype.hasOwnProperty.call(ed.baseAdmins, id)) return { ok: false, reason: 'unknown_member' };
    var pa = orgEdProjectedAdmins(ed), cur = !!pa[id];
    if (cur && !orgEdCanRemoveAdmin(ed, id)) return { ok: false, reason: 'last_admin' };
    var next = !cur;
    if (next === !!ed.baseAdmins[id]) delete ed.stagedAdmins[id];
    else ed.stagedAdmins[id] = next;
    delete ed.errors['admin:' + id];
    return { ok: true };
  }
  function orgEdUndo(ed, key) {
    var sep = String(key || '').indexOf(':');
    if (sep < 0) return false;
    var kind = key.slice(0, sep), id = key.slice(sep + 1);
    if (kind === 'lead') {
      if (!Object.prototype.hasOwnProperty.call(ed.stagedLeads, id)) return false;
      var nextLeads = orgEdProjectedLeads(ed);
      var orig = ed.baseLeads[id] || '';
      delete nextLeads[id];
      if (orig && orgEdIsInSubtree(nextLeads, orig, id)) return false;
      delete ed.stagedLeads[id];
      delete ed.errors[key];
      return true;
    }
    if (kind === 'admin') {
      if (!Object.prototype.hasOwnProperty.call(ed.stagedAdmins, id)) return false;
      var wasAdd = ed.stagedAdmins[id] === true;
      delete ed.stagedAdmins[id];
      delete ed.errors[key];
      // Undoing a staged add_admin could leave 0 admins if an existing admin's removal was also staged;
      // restore staged removals so the editor never holds a state the server refuses.
      if (wasAdd && orgEdAdminCount(ed) < 1) {
        Object.keys(ed.stagedAdmins).forEach(function (aid) {
          if (ed.stagedAdmins[aid] === false) { delete ed.stagedAdmins[aid]; delete ed.errors['admin:' + aid]; }
        });
      }
      return true;
    }
    if (kind === 'org') {
      var st = ed.stagedOrgs && ed.stagedOrgs[id];
      if (!st) return false;
      ed.lastUndoErr = null;
      if (st.op === 'create') {
        delete ed.stagedOrgs[id];
        delete ed.errors[key];
        if (ed.editingOrg === id) ed.editingOrg = null;
        return true;
      }
      var base = ed.baseOrgs && ed.baseOrgs[id];
      if (!base) return false;
      if (base.head) {
        var held = orgEdHeadOf(ed, base.head);
        if (held && held.id !== id) {
          ed.lastUndoErr = { key: key, reason: 'already_head', other: held, head: base.head };
          return false;
        }
      }
      var clash = orgEdNameClash(ed, base.name, id);
      if (clash) {
        ed.lastUndoErr = { key: key, reason: 'dup_name', other: clash };
        return false;
      }
      delete ed.stagedOrgs[id];
      delete ed.errors[key];
      return true;
    }
    return false;
  }
  function orgEdUndoAll(ed) {
    ed.stagedLeads = {};
    ed.stagedAdmins = {};
    ed.stagedOrgs = {};
    ed.editingOrg = null;
    ed.errors = {};
    ed.refuse = null;
    ed.lastUndoErr = null;
  }
  function orgEdChanges(ed) {
    var list = [];
    Object.keys(ed.stagedOrgs || {}).sort().forEach(function (id) {
      var st = ed.stagedOrgs[id], base = ed.baseOrgs && ed.baseOrgs[id];
      if (st.op === 'create') {
        list.push({ key: 'org:' + id, kind: 'org_create', org: id, name: st.name, head: st.head || '' });
      } else if (st.op === 'delete') {
        list.push({ key: 'org:' + id, kind: 'org_delete', org: id, name: base ? base.name : id, head: base ? (base.head || '') : '' });
      } else if (st.op === 'edit') {
        list.push({
          key: 'org:' + id, kind: 'org_edit', org: id,
          fromName: base ? base.name : '', toName: st.name,
          fromHead: base ? (base.head || '') : '', toHead: st.head || ''
        });
      }
    });
    Object.keys(ed.stagedLeads).sort().forEach(function (id) {
      list.push({ key: 'lead:' + id, kind: 'lead', member: id, from: ed.baseLeads[id] || '', to: ed.stagedLeads[id] || '' });
    });
    Object.keys(ed.stagedAdmins).sort().forEach(function (id) {
      list.push({ key: 'admin:' + id, kind: ed.stagedAdmins[id] ? 'add_admin' : 'remove_admin', member: id });
    });
    return list;
  }
  function orgEdCount(ed) {
    return Object.keys(ed.stagedLeads).length + Object.keys(ed.stagedAdmins).length + Object.keys(ed.stagedOrgs || {}).length;
  }
  function orgEdCloseNeedsConfirm(ed) {
    return !!ed && orgEdCount(ed) > 0;
  }
  // Save-time pre-flight check and deterministic single-batch event builder (Decisions 7, 8 & 12).
  // Order: add_admin -> org_remove -> org head releases -> topological org_set (with vacate on
  // head-swap cycles) -> set_lead (ascending final depth) -> remove_admin (viewer very last).
  function orgEdPreflightAndBuild(ed, currentMembers, viewer, currentOrgs) {
    var M = currentMembers || {}, who = viewer || ed.viewer || '';
    var me = M[who];
    if (!me || me.removed || !me.admin) {
      return { ok: false, errors: { _viewer: { code: 'not_admin' } }, dropped: [], events: [], indexMap: [] };
    }
    var curTree = orgTree(M), curParent = curTree.parentOf;
    var curOrgs = {};
    var osrc = currentOrgs !== undefined ? currentOrgs : (ed.baseOrgs || {});
    var olist = Array.isArray(osrc) ? osrc : Object.keys(osrc || {}).map(function (k) { return osrc[k]; });
    olist.forEach(function (o) {
      if (o && o.id) curOrgs[o.id] = { id: o.id, name: o.name || '', head: o.head || '', removed: o.removed || null };
    });
    var errors = {}, dropped = [], activeLeads = {}, activeAdmins = {}, activeDeletes = [], activeSets = {};
    Object.keys(ed.stagedLeads).sort().forEach(function (id) {
      var key = 'lead:' + id, target = ed.stagedLeads[id] || '', curM = M[id];
      if (!curM || curM.removed) { errors[key] = { code: 'removed_member' }; return; }
      if (target && (!M[target] || M[target].removed)) { errors[key] = { code: 'removed_target', target: target }; return; }
      var curLead = curParent[id] || '', baseLead = ed.baseLeads[id] || '';
      if (curLead === target) { dropped.push(key); return; }
      if (curLead !== baseLead) { errors[key] = { code: 'stale_base', actual: curLead }; return; }
      activeLeads[id] = target;
    });
    Object.keys(ed.stagedAdmins).sort().forEach(function (id) {
      var key = 'admin:' + id, want = !!ed.stagedAdmins[id], curM = M[id];
      if (!curM || curM.removed) { errors[key] = { code: 'removed_member' }; return; }
      if (want === !!curM.admin) { dropped.push(key); return; }
      activeAdmins[id] = want;
    });
    Object.keys(ed.stagedOrgs || {}).sort().forEach(function (id) {
      var key = 'org:' + id, st = ed.stagedOrgs[id], cur = curOrgs[id], base = ed.baseOrgs && ed.baseOrgs[id];
      if (st.op === 'delete') {
        if (!cur || cur.removed) { dropped.push(key); return; }
        if (base && (cur.name !== base.name || (cur.head || '') !== (base.head || ''))) {
          errors[key] = { code: 'stale_base' }; return;
        }
        activeDeletes.push(id);
        return;
      }
      var wantName = String(st.name || '').trim(), wantHead = st.head || '';
      if (st.op === 'create') {
        if (cur) { errors[key] = { code: 'org_exists' }; return; }
        if (!wantName) { errors[key] = { code: 'blank_name' }; return; }
        if (wantHead && (!M[wantHead] || M[wantHead].removed)) { errors[key] = { code: 'removed_head', target: wantHead }; return; }
        activeSets[id] = { op: 'create', name: wantName, head: wantHead };
        return;
      }
      if (!cur || cur.removed) { errors[key] = { code: 'removed_org' }; return; }
      if (!wantName) { errors[key] = { code: 'blank_name' }; return; }
      if (wantHead && (!M[wantHead] || M[wantHead].removed)) { errors[key] = { code: 'removed_head', target: wantHead }; return; }
      if (cur.name === wantName && (cur.head || '') === wantHead) { dropped.push(key); return; }
      if (base && (cur.name !== base.name || (cur.head || '') !== (base.head || ''))) {
        errors[key] = { code: 'stale_base' }; return;
      }
      activeSets[id] = { op: 'edit', name: wantName, head: wantHead };
    });
    var finalLeads = {};
    Object.keys(curParent).forEach(function (id) { finalLeads[id] = curParent[id]; });
    Object.keys(activeLeads).forEach(function (id) { finalLeads[id] = activeLeads[id]; });
    var finalTree = orgTree(M, finalLeads);
    var unreach = {};
    finalTree.unreachable.forEach(function (id) { unreach[id] = true; });
    Object.keys(activeLeads).forEach(function (id) {
      if (unreach[id] || (activeLeads[id] && orgEdIsInSubtree(finalLeads, activeLeads[id], id))) {
        errors['lead:' + id] = { code: 'cycle' };
      }
    });
    var finalAdminCount = 0;
    Object.keys(curParent).forEach(function (id) {
      var isAdm = Object.prototype.hasOwnProperty.call(activeAdmins, id) ? activeAdmins[id] : !!(M[id] && M[id].admin);
      if (isAdm) finalAdminCount++;
    });
    if (finalAdminCount < 1) {
      Object.keys(activeAdmins).forEach(function (id) {
        if (activeAdmins[id] === false) errors['admin:' + id] = { code: 'last_admin' };
      });
    }
    var finalOrgs = {}, delSet = {};
    activeDeletes.forEach(function (id) { delSet[id] = true; });
    Object.keys(curOrgs).forEach(function (id) {
      var o = curOrgs[id];
      if (!o.removed && !delSet[id]) finalOrgs[id] = { id: id, name: o.name, head: o.head || '' };
    });
    Object.keys(activeSets).forEach(function (id) {
      finalOrgs[id] = { id: id, name: activeSets[id].name, head: activeSets[id].head || '' };
    });
    var nameSeen = {}, headSeen = {};
    Object.keys(finalOrgs).sort().forEach(function (id) {
      var o = finalOrgs[id], low = o.name.toLowerCase();
      if (nameSeen[low]) {
        if (activeSets[id]) errors['org:' + id] = { code: 'dup_name' };
        if (activeSets[nameSeen[low]]) errors['org:' + nameSeen[low]] = { code: 'dup_name' };
      } else nameSeen[low] = id;
      if (o.head) {
        if (headSeen[o.head]) {
          if (activeSets[id]) errors['org:' + id] = { code: 'already_head', target: o.head };
          if (activeSets[headSeen[o.head]]) errors['org:' + headSeen[o.head]] = { code: 'already_head', target: o.head };
        } else headSeen[o.head] = id;
      }
    });
    if (Object.keys(errors).length) {
      return { ok: false, errors: errors, dropped: dropped, events: [], indexMap: [] };
    }
    var events = [], indexMap = [];
    // 1. roles.add_admin first
    Object.keys(activeAdmins).sort().forEach(function (id) {
      if (activeAdmins[id] === true) {
        events.push({ type: 'roles.add_admin', payload: { member: id }, origin: 'human' });
        indexMap.push('admin:' + id);
      }
    });
    // 2. roles.org_remove
    var simOrgs = {};
    Object.keys(curOrgs).forEach(function (id) {
      if (!curOrgs[id].removed) simOrgs[id] = { name: curOrgs[id].name, head: curOrgs[id].head || '' };
    });
    activeDeletes.sort().forEach(function (id) {
      events.push({ type: 'roles.org_remove', payload: { org: id }, origin: 'human' });
      indexMap.push('org:' + id);
      delete simOrgs[id];
    });
    function headOwner(h) {
      if (!h) return '';
      var ids = Object.keys(simOrgs);
      for (var i = 0; i < ids.length; i++) if (simOrgs[ids[i]].head === h) return ids[i];
      return '';
    }
    function nameOwner(n) {
      var low = String(n || '').toLowerCase(), ids = Object.keys(simOrgs);
      for (var i = 0; i < ids.length; i++) if (simOrgs[ids[i]].name.toLowerCase() === low) return ids[i];
      return '';
    }
    // 3. Pure head releases first (frees heads before any take-up)
    var remaining = Object.keys(activeSets).sort();
    var nextRem = [];
    remaining.forEach(function (id) {
      var want = activeSets[id], sim = simOrgs[id];
      if (sim && sim.head && !want.head) {
        var no = nameOwner(want.name);
        if (!no || no === id) {
          events.push({ type: 'roles.org_set', payload: { org: id, name: want.name, head: '' }, origin: 'human' });
          indexMap.push('org:' + id);
          simOrgs[id] = { name: want.name, head: '' };
          return;
        }
        events.push({ type: 'roles.org_set', payload: { org: id, name: sim.name, head: '' }, origin: 'human' });
        indexMap.push('org:' + id);
        simOrgs[id].head = '';
      }
      nextRem.push(id);
    });
    remaining = nextRem;
    // 4. Topological org_set (head take-ups, renames, creates), breaking head/name cycles with a vacate event
    var guard = remaining.length * 3 + 4;
    while (remaining.length && guard-- > 0) {
      var ready = [];
      remaining.forEach(function (id) {
        var want = activeSets[id];
        var ho = headOwner(want.head), no = nameOwner(want.name);
        if ((!ho || ho === id) && (!no || no === id)) ready.push(id);
      });
      if (ready.length) {
        var remSet = {};
        ready.forEach(function (id) {
          var want = activeSets[id];
          events.push({ type: 'roles.org_set', payload: { org: id, name: want.name, head: want.head }, origin: 'human' });
          indexMap.push('org:' + id);
          simOrgs[id] = { name: want.name, head: want.head };
          remSet[id] = true;
        });
        remaining = remaining.filter(function (id) { return !remSet[id]; });
        continue;
      }
      var vacId = '';
      for (var i = 0; i < remaining.length; i++) {
        var cid = remaining[i];
        if (simOrgs[cid] && simOrgs[cid].head) { vacId = cid; break; }
      }
      if (vacId) {
        events.push({ type: 'roles.org_set', payload: { org: vacId, name: simOrgs[vacId].name, head: '' }, origin: 'human' });
        indexMap.push('org:' + vacId);
        simOrgs[vacId].head = '';
        continue;
      }
      var renId = remaining[0];
      if (simOrgs[renId]) {
        var tmp = simOrgs[renId].name + ' #' + renId;
        while (nameOwner(tmp)) tmp += '_';
        events.push({ type: 'roles.org_set', payload: { org: renId, name: tmp, head: simOrgs[renId].head }, origin: 'human' });
        indexMap.push('org:' + renId);
        simOrgs[renId].name = tmp;
      }
    }
    // 5. roles.set_lead in ascending depth of the final tree
    Object.keys(activeLeads).sort(function (a, b) {
      var da = finalTree.depthOf[a] || 0, db = finalTree.depthOf[b] || 0;
      return da !== db ? da - db : (a < b ? -1 : a > b ? 1 : 0);
    }).forEach(function (id) {
      var target = activeLeads[id];
      events.push({ type: 'roles.set_lead', payload: { member: id, leads: target ? [target] : [] }, origin: 'human' });
      indexMap.push('lead:' + id);
    });
    // 6. roles.remove_admin last, saving viewer's own removal very last
    var rms = Object.keys(activeAdmins).filter(function (id) { return activeAdmins[id] === false; }).sort(function (a, b) {
      if (a === who && b !== who) return 1;
      if (b === who && a !== who) return -1;
      return a < b ? -1 : a > b ? 1 : 0;
    });
    rms.forEach(function (id) {
      events.push({ type: 'roles.remove_admin', payload: { member: id }, origin: 'human' });
      indexMap.push('admin:' + id);
    });
    return { ok: true, errors: {}, dropped: dropped, events: events, indexMap: indexMap };
  }
  function orgEdMapRefusal(indexMap, refusal) {
    if (!refusal) return { key: '_batch', reason: '' };
    var idx = typeof refusal.index === 'number' ? refusal.index : -1;
    var key = idx >= 0 && idx < indexMap.length ? indexMap[idx] : '_batch';
    return { key: key, reason: refusal.reason || '' };
  }
  // Pure item-window org helpers (Decisions 10, 11, 19 & 20).
  function itemOrgRules(it, rules) {
    if (!it || !rules) return [];
    var list = Array.isArray(rules) ? rules : Object.keys(rules).map(function (k) { return rules[k]; });
    return list.filter(function (ru) {
      return ru && !ru.ended && ru.org && ru.item === it.id;
    }).sort(function (a, b) {
      var ta = a.ts || '', tb = b.ts || '';
      return ta !== tb ? (ta < tb ? -1 : 1) : (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
    });
  }
  function ruleCoversMember(ru, memberId, ot) {
    if (!ru || ru.ended || ru.state !== 'live' || !ru.root || !memberId) return false;
    if (memberId === ru.root) return !!ru.includeRoot;
    var parentOf = (ot && ot.parentOf) || {};
    var cur = parentOf[memberId] || '', steps = 1, seen = {};
    seen[memberId] = true;
    while (cur && !seen[cur]) {
      if (cur === ru.root) return ru.levels == null || steps <= ru.levels;
      seen[cur] = true;
      cur = parentOf[cur] || '';
      steps++;
    }
    return false;
  }
  function itemOrgAssignInfo(it, rules, assignments, orgs, members, viewer, canAssignItem) {
    if (!it) return [];
    var M = members || {}, oi = orgIndex(orgs, M), ot = oi.tree;
    var rlist = Array.isArray(rules) ? rules : Object.keys(rules || {}).map(function (k) { return rules[k]; });
    var openRules = rlist.filter(function (ru) { return ru && !ru.ended && ru.item === it.id; });
    var orgRules = itemOrgRules(it, openRules);
    if (!orgRules.length) return [];
    var amap = assignments || {};
    var activeRecs = [];
    (it.assignments || []).concat(it.pending || []).forEach(function (aid) {
      var r = typeof aid === 'string' ? amap[aid] : aid;
      if (r && (r.state === 'live' || r.state === 'pending') && !r.fallback) activeRecs.push(r);
    });
    var canAsg = canAssignItem !== undefined ? !!canAssignItem : (!it.hidden && it.author === viewer);
    return orgRules.map(function (ru) {
      var org = oi.byId[ru.org] || { id: ru.org, name: ru.org, head: '', removed: 'deleted' };
      var deleted = !!org.removed;
      var vacant = !deleted && (!org.head || !(M[org.head] && !M[org.head].removed));
      var recs = activeRecs.filter(function (r) { return r.rule === ru.id; });
      var onlyThrough = [], stayed = [];
      recs.forEach(function (r) {
        if (r.ruleRequest) { onlyThrough.push(r); return; }
        var coveredElsewhere = openRules.some(function (o) {
          return o.id !== ru.id && ruleCoversMember(o, r.to, ot);
        });
        if (coveredElsewhere) stayed.push(r);
        else onlyThrough.push(r);
      });
      var canRemove = !it.hidden && ru.by === viewer;
      return {
        rule: ru, org: org, orgId: ru.org, deleted: deleted, vacant: vacant,
        pending: ru.state === 'pending', recs: recs, onlyThrough: onlyThrough, stayed: stayed,
        canRemove: canRemove, canRetarget: canRemove && canAsg
      };
    });
  }
  function itemOrgCCInfo(it, orgs, members) {
    if (!it || !Array.isArray(it.orgCC) || !it.orgCC.length) return [];
    var M = members || {}, oi = orgIndex(orgs, M), srcs = it.ccSources || {};
    var ccSet = {};
    (it.cc || []).forEach(function (m) { ccSet[m] = true; });
    return it.orgCC.map(function (reg) {
      var org = oi.byId[reg.org] || { id: reg.org, name: reg.org, head: '', removed: 'deleted' };
      var deleted = !!org.removed;
      var vacant = !deleted && (!org.head || !(M[org.head] && !M[org.head].removed));
      var added = (reg.added || []).filter(function (m) {
        return M[m] ? !M[m].removed && (!it.cc || ccSet[m]) : true;
      });
      var onlyThrough = [], stayed = [];
      added.forEach(function (m) {
        var s = srcs[m];
        var otherOrgs = s && Array.isArray(s.orgs) ? s.orgs.filter(function (oid) { return oid !== reg.org; }) : [];
        if (s && (s.personal || otherOrgs.length > 0)) stayed.push(m);
        else onlyThrough.push(m);
      });
      return {
        org: org, orgId: reg.org, deleted: deleted, vacant: vacant,
        added: added, onlyThrough: onlyThrough, stayed: stayed,
        canRemove: !it.hidden, canRetarget: !it.hidden
      };
    });
  }
  function itemDeletedOrgSummary(it, rules, orgs, viewer) {
    if (!it) return { assignDeletedCount: 0, ccDeletedCount: 0, needsCleanup: false };
    var oi = orgIndex(orgs, {});
    function isDel(oid) { var o = oi.byId[oid]; return !o || !!o.removed; }
    var assignDeletedCount = 0, authorHasDeletedAssign = false;
    itemOrgRules(it, rules).forEach(function (ru) {
      if (isDel(ru.org)) {
        assignDeletedCount++;
        if (ru.by === viewer) authorHasDeletedAssign = true;
      }
    });
    var ccDeletedCount = 0;
    (it.orgCC || []).forEach(function (reg) {
      if (reg && reg.org && isDel(reg.org)) ccDeletedCount++;
    });
    var needsCleanup = !it.hidden && (ccDeletedCount > 0 || authorHasDeletedAssign);
    return { assignDeletedCount: assignDeletedCount, ccDeletedCount: ccDeletedCount, needsCleanup: needsCleanup };
  }
  function orgAssignUnassignEvent(itemId, ru) {
    var p = { item: itemId, rule: ru.id };
    var root = ru.assignRoot || ru.root;
    if (root) p.root = root;
    return ['item.unassign', p];
  }
  function orgCCUnccEvent(itemId, orgId) {
    return ['item.uncc_org', { item: itemId, org: orgId }];
  }
  // Live, doing or blocked, and no status write for this many days: "stale" (the
  // view state carries the flag; the number is for the text).
  var STALE_DAYS = 3;
  // the send / save chord's name: Cmd+Enter on Apple platforms, else Ctrl+Enter
  var MAC = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || '');

  // Status icons: every status differs by shape as well as colour, so it reads
  // without colour too, and carries one short description for the tooltip. Shapes
  // are 16x16 inline SVG drawn with currentColor; `tone` names the CSS custom
  // property that colours the icon.
  var SI = (function () {
    var RING = '<circle cx="8" cy="8" r="5.6"/>';
    var SET = {
      idea:       { label: "js.tk.st.idea", desc: "js.tk.st.idea.desc", tone: 'wb-muted-fg',
        svg: '<path d="M8 2.2a4 4 0 0 0-2.4 7.2c.5.4.8 1 .8 1.6v.4h3.2V11c0-.6.3-1.2.8-1.6A4 4 0 0 0 8 2.2zM6.2 13.8h3.6"/>' },
      todo:       { label: "js.tk.st.todo", desc: "js.tk.st.todo.desc", tone: 'wb-primary', svg: RING },
      doing:      { label: "js.tk.st.doing", desc: "js.tk.st.doing.desc", tone: 'wb-primary', svg: RING + '<path d="M8 2.4a5.6 5.6 0 0 1 0 11.2z" fill="currentColor"/>' },
      blocked:    { label: "js.tk.st.blocked", desc: "js.tk.st.blocked.desc", tone: 'tk-fail', svg: RING + '<path d="M4.1 4.1l7.8 7.8"/>' },
      'in-review': { label: "js.tk.st.inreview", desc: "js.tk.st.inreview.desc", tone: 'tk-warn',
        svg: '<path d="M4.5 2.4h7M4.5 13.6h7M5.4 2.4c0 3.2 5.2 3.4 5.2 5.6s-5.2 2.4-5.2 5.6M10.6 2.4c0 3.2-5.2 3.4-5.2 5.6s5.2 2.4 5.2 5.6"/>' },
      done:       { label: "js.tk.st.done", desc: "js.tk.st.done.desc", tone: 'tk-ok', svg: '<circle cx="8" cy="8" r="6" fill="currentColor" stroke="none"/><path d="M5.3 8.2l1.9 1.9 3.6-3.8" stroke="var(--wb-card)"/>' },
      // wrapped up like done (the filled disc) without being finished (a dash, not a check)
      dropped:    { label: "js.tk.st.dropped", desc: "js.tk.st.dropped.desc", tone: 'wb-muted-fg', svg: '<circle cx="8" cy="8" r="6" fill="currentColor" stroke="none"/><path d="M5.2 8h5.6" stroke="var(--wb-card)"/>' },
      pending:    { label: "js.tk.st.pending", desc: "js.tk.st.pending.desc", tone: 'tk-teal',
        svg: '<circle cx="8" cy="8" r="5.6" stroke-dasharray="2.2 1.9"/><path d="M6.5 6.6a1.55 1.55 0 1 1 2.2 1.4c-.5.25-.7.6-.7 1.1M8 10.9v.05"/>' },
      unassigned: { label: "js.tk.st.unassigned", desc: "js.tk.st.unassigned.desc", tone: 'wb-muted-fg', svg: '<circle cx="8" cy="8" r="5.6" stroke-dasharray="0.8 2.2"/>' },
      declined:   { label: "js.tk.st.declined", desc: "js.tk.st.declined.desc", tone: 'wb-muted-fg', svg: RING + '<path d="M5.9 5.9l4.2 4.2M10.1 5.9l-4.2 4.2"/>' },
      returned:   { label: "js.tk.st.returned", desc: "js.tk.st.returned.desc", tone: 'wb-muted-fg', svg: RING + '<path d="M10.4 10.4V8.9a1.9 1.9 0 0 0-1.9-1.9H5.6M7.1 5.4L5.5 7l1.6 1.6"/>' },
      handed:     { label: "js.tk.st.handed", desc: "js.tk.st.handed.desc", tone: 'wb-muted-fg', svg: RING + '<path d="M5.2 8h5.2M8.6 6l2 2-2 2"/>' },
      withdrawn:  { label: "js.tk.st.withdrawn", desc: "js.tk.st.withdrawn.desc", tone: 'wb-muted-fg', svg: RING + '<path d="M5.4 8h5.2"/>' },
      topic:      { label: "js.tk.st.topic", desc: "js.tk.st.topic.desc", tone: 'wb-muted-fg', svg: '<path d="M2.8 4h10.4v6.6H7.4l-2.8 2.4v-2.4H2.8z"/>' }
    };
    // label and desc hold table keys; info looks them up at call time.
    function info(key) { var s = SET[key] || SET.unassigned; return { label: wsT(s.label), desc: wsT(s.desc), tone: s.tone, svg: s.svg }; }
    // The icon as inline SVG; the caller wraps it with its own tooltip trigger.
    function svg(key) {
      var s = SET[key] || SET.unassigned;
      return '<svg class="sti-svg" viewBox="0 0 16 16" aria-hidden="true" style="color:var(--' + s.tone + ')">' + s.svg + '</svg>';
    }
    return { set: SET, info: info, svg: svg };
  })();

  var PROFILE_FIELDS = ['org', 'family', 'given'];

  // Set by mount: the adapter, its caps and ext, the options, the display-state
  // storage prefix, and the helper bag ext functions get.
  var AD = null, CAPS = {}, X = {}, O = {}, PFX = 'ws.tracker.', H = null, api = null;
  var order = [], vss = {}, EV = {};
  var ui = {
    store: null,          // the store the item window and the dialogs refer to
    open: null,           // the store whose drawer is open, or null
    page: null,           // the store shown full-page by the route #/<storeId>, or null
    routeItem: null,      // { store, item } a #/<storeId>/<itemId> link opens once the view holds it
    fresh: {},            // badge keys that were new when the drawer last opened
    loaded: {},           // "store|stream" -> { attachment oid: true } "loaded" by that viewer's clone (simulated, page lifetime)
    mdraft: [], adraft: [], // composer drafts: member ids picked with @, attachment metadata to post
    item: null, win: false, last: null, replyTo: null,
    winReturn: null,      // selector of the element that opened the item window; focus returns there on close
    aopen: null,          // id of the item whose assignee fold is expanded; the fold resets when another item opens
    tab: {},              // store -> { side: tab key of the tab pane, narrow: tab key of the narrow layout }
    tabSeen: {},          // "store|stream" -> { tab key -> signature last shown }: a hidden tab whose signature moved shows a dot
    layout: null,         // 'a' | 'd' | 'narrow', from the window width
    cmodes: {}, calMonth: {}, // store -> { card key -> view mode } (display state, stored); card or scope key -> calendar month
    pmodes: {}, closedOpen: {}, // store -> { scope -> view mode } and store -> "recently closed" expanded (display state, stored)
    panel: {},            // store -> { scope, modes: { scope -> view mode }, today: Today chip filter or null, q: search text, closedOpen: "recently closed" expanded, treeOpen: { parent id -> its "closed N" row expanded } } of the Items panel
    psr: {},              // store -> the search box's adapter.search state (searchState, page lifetime)
    tfold: {},            // "store|item id" -> 'o' | 'f': a context-tree node unfolded in full or folded (display state, stored; absent: the default, see ctxTree)
    wfold: {},            // "store|item id" -> true | false: a node of the item window's subtree open or folded (page lifetime; absent: the default)
    focus: null,          // { ev: event id, flash: true until its first paint } the item window scrolls to and marks; null: nothing
    mdet: {},             // "store|member id" -> true: the member's key and profile details are expanded in the org card (page lifetime)
    ofold: {},            // "store|member id" -> true: the member's subtree is collapsed in the org card (page lifetime)
    iwOrgFold: {},        // "store|item|key" -> true|false: item window org group expand/collapse override
    orged: null,          // open admin org-chart editor state (orgEdInit), or null when closed
    sel: null,            // the row selected by a tap: it shows the same actions and details a hover shows
    msel: {},             // store -> { ids: { item id: true }, at: anchor row key } while the Items list is in selection mode (see mselClick)
    nrow: {},             // store -> { val, kind } while the Items list shows its "+ New" input row
    knew: {},             // store -> { val } while the "all" kanban's idea column shows its "+" input row
    sub: null,            // { item: parent, rk: the row it sits under, val } while an "add subtask" input row shows
    iedit: null,          // { item, title, body, project, mpick } while the item window edits the item in place
    itemDrafts: Object.create(null), // store -> item -> cached draft, or null after discard
    cedit: null,          // { cid, body, mpick } while an own comment is edited in place; mpick: member ids picked with @ meanwhile
    edpv: {},             // editor textarea id -> { text, html, err, busy } while that editor shows its preview (editorHtml)
    eatt: {},             // editor textarea id -> [{oid, name, size, mime}] images uploaded into that editor since it opened (edImages)
    wiki: {},             // store -> the Items panel's wiki state (wk(), see cardWiki)
    act: { family: 'all', member: 'all', flagged: false, conc: false },
    kind: 'all', status: 'open'
  };
  // The Items panel: scope presets crossed with the view modes each scope offers.
  // A side card offers only the modes that fit it; a card missing from CARD_MODES has one layout.
  // Each entry is [key, name key(, tooltip key)]: the names are table keys, looked up where drawn.
  var ASSIGN_MODES = [['list', "js.tk.mode.list"], ['kanban', "js.tk.mode.kanban"], ['gantt', "js.tk.mode.gantt"], ['calendar', "js.tk.mode.calendar"]];
  // The scope keys and the mode keys SCOPE_MODES offers are copied in Go (wslive
  // UIScopes and UIViewModes, which a support agent's UI commands are checked against);
  // a wsdashboard test fails when the two sites differ.
  var SCOPES = [['mine', "js.tk.scope.mine", "js.tk.scope.mine.tip"],
    ['related', "js.tk.scope.related", "js.tk.scope.related.tip"],
    ['received', "js.tk.scope.received", "js.tk.scope.received.tip"],
    ['given', "js.tk.scope.given", "js.tk.scope.given.tip"],
    ['team', "js.tk.scope.team", "js.tk.scope.team.tip"],
    ['all', "js.tk.scope.all", "js.tk.scope.all.tip"]];
  var SCOPE_MODES = { mine: ASSIGN_MODES, related: [['list', "js.tk.mode.list"]], received: [['list', "js.tk.mode.list"]], given: ASSIGN_MODES, team: ASSIGN_MODES,
    all: [['list', "js.tk.mode.tree"], ['kanban', "js.tk.mode.kanban"], ['gantt', "js.tk.mode.gantt"], ['calendar', "js.tk.mode.calendar"]] };
  var CARD_MODES = { activity: [['list', "js.tk.mode.list"], ['activity', "js.tk.mode.activity"]] };

  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c];
    });
  }
  // An item body or comment text as a block of class cls: the markdown the daemon rendered
  // for it in safe mode (view html.<kind>[id]), marked md for the shared markdown styles,
  // else the escaped plain text (a view without html, as the mockup's). Either way a #<alias>
  // token is a link to that item (refLinks).
  function textBlock(cls, kind, id, text, attrs) {
    var h = VS.html && VS.html[kind] && VS.html[kind][id];
    return '<div class="' + cls + (h != null ? ' md' : '') + '"' + (attrs || '') + '>' + (h != null ? refLinksHtml(h) : refLinks(esc(text))) + '</div>';
  }
  function $(id) { return document.getElementById(id); }
  function attr(k, v) { return ' data-' + k + '="' + esc(v) + '"'; }
  function vals(o) { return Object.keys(o).map(function (k) { return o[k]; }); }

  // ------------------------------------------------------------- small views

  var VS = null; // the view state being rendered
  function nm(id) { return VS.members[id] ? VS.members[id].name : id; }
  // The org is not part of the display name: it shows as a muted prefix where a
  // person is named in markup (whoName), and as a plain prefix in tooltips and option
  // texts (nmo).
  function orgOf(id) { var m = VS.members[id]; return m && m.profile ? m.profile.org : ''; }
  function nmo(id) { var o = orgOf(id); return (o ? o + ' ' : '') + nm(id); }
  function whoName(id) { var o = orgOf(id); return (o ? '<span class="org">' + esc(o) + '</span> ' : '') + esc(nm(id)); }
  function fmtTs(ts) { return ts ? String(ts).slice(5, 16).replace('T', ' ') : '-'; }
  function today() { return String(VS.now || '').slice(0, 10); }
  function itemRef(id) { return '<a href="#"' + attr('item', id) + '>' + esc(alias(id)) + '</a>'; }
  function titleRef(id) {
    var it = VS.items[id];
    return '<a class="title" href="#"' + attr('item', id) + '>' + esc(it ? it.title : alias(id)) + '</a>';
  }
  // A status's or a kind's display name: the table's, else the value itself.
  function stLabel(s) { return SI.set[s] ? wsT(SI.set[s].label) : s; }
  // todo is the retired name of task: an old create payload in the event history reads as a task.
  var KIND_KEY = { task: "js.tk.kind.task", todo: "js.tk.kind.task", topic: "js.tk.kind.topic" };
  function kindName(k) { return KIND_KEY[k] ? wsT(KIND_KEY[k]) : k; }
  function stChip(s) { return s ? '<span class="chip st-' + esc(s) + '">' + esc(stLabel(s)) + '</span>' : ''; }
  // An ended record's pill is its end-state text (endText), not a status chip: the item-status
  // table's "unassigned" names an item without assignees and misreads on a member row.
  function recChip(r) {
    if (r.ended) return '<span class="chip ended">' + esc(endText(r.state) || r.state) + '</span>';
    if (r.state !== 'live') return stChip(r.state);
    return stChip(r.status) + (r.statusOrigin && r.statusOrigin !== 'human' ? '<span class="chip agent" title="' + wsT("js.tk.chip.byagent.tip", esc(r.statusBy), esc(r.statusOrigin)) + '">' + wsT("js.tk.chip.byagent") + '</span>' : '');
  }
  // " (stored N, outside the current range: shown as the top level)" when the stored priority is out of range
  function priStored(it) { return it.priority !== it.priorityLevel ? ' ' + wsT("js.tk.pri.stored", esc(it.priority)) : ''; }
  function prChip(it) {
    if (!it || it.kind === 'topic') return '';
    return '<span class="chip pr pr-' + it.priorityLevel + '" title="' + wsT("js.tk.pri.tip", it.priorityLevel, VS.priority.names.length - 1) +
      priStored(it) + '">' + esc(it.priorityName) + '</span>';
  }
  function itemDueState(it) {
    var time = (VS.itemTime || {})[it.id];
    return time ? time.dueState : !it.due ? '' : it.due < today() ? 'over' : it.due === today() ? 'today' : 'future';
  }
  function dueChip(it) {
    if (!it || !it.due) return '';
    var state = itemDueState(it), over = state === 'over' && !wrapped(it.status), now = state === 'today' && !wrapped(it.status);
    return '<span class="chip due' + (over ? ' over' : now ? ' now' : '') + '" title="' + (over ? wsT("js.tk.due.tip.over", esc(it.due)) : now ? wsT("js.tk.due.tip.now", esc(it.due)) : wsT("js.tk.due.on", esc(it.due))) + '">' +
      (now ? wsT("js.tk.due.today") : wsT("js.tk.due.on", esc(it.due.slice(5)))) + '</span>';
  }
  // The store's label list (view.labels, live labels in add order); an item carries label ids.
  function labelOf(id) { return (VS.labels || []).filter(function (l) { return l.id === id; })[0] || null; }
  // A removed label is gone from view.labels; the activity log still names it, by its add
  // event and its applied renames (EV, the events by id).
  function labelName(id) {
    var l = labelOf(id), n = VS.labelNames && VS.labelNames[id];
    if (n == null) n = EV[id] && EV[id].type === 'roles.add_label' ? EV[id].payload.name : null;
    if (l) return l.name;
    VS.events.forEach(function (e) { if (e.type === 'roles.edit_label' && e.status === 'applied' && e.payload.label === id && typeof e.payload.name === 'string') n = e.payload.name; });
    return typeof n === 'string' ? n : alias(id);
  }
  // One label chip, its colour a dot; with act, a button (a row's chip searches label:"name").
  function labelChip(l, act, extra) {
    var inner = '<span class="ldot" style="background:' + esc(l.color) + '"></span>' + esc(l.name);
    if (!act) return '<span class="chip lbl">' + inner + '</span>';
    return '<button type="button" class="chip lbl"' + attr('act', act) + attr('label', l.id) + (extra || '') + ' tabindex="-1"' + tipAttr(wsT("js.tk.label.search", l.name)) + '>' + inner + '</button>';
  }
  // An admin removes a label from the store's list (the store settings and the quick-add picker):
  // it leaves every item, so the viewer confirms first. then runs after the write.
  function labelRemove(id, then) {
    var l = labelOf(id);
    if (l && window.confirm(wsT("js.tk.set.labels.confirm", l.name))) commit([['roles.remove_label', { label: l.id }]], then);
  }
  function labelChips(it, act) {
    return (it.labels || []).map(labelOf).filter(Boolean).map(function (l) { return labelChip(l, act); }).join('');
  }
  // An opened item's "open" chip (items[].open): who opened it to which scope, and whether the
  // viewer may claim it (the accent). A click shows the claimable items (the Items filter).
  function openChip(it) {
    var o = it.open;
    if (!o) return '';
    var tip = (o.scope === 'store' ? wsT("js.tk.open.tip.store", nm(o.by)) : wsT("js.tk.open.tip.team", nm(o.by))) + ' ' + (it.claimable ? wsT("js.tk.open.tip.mine") : wsT("js.tk.open.tip.notmine"));
    return '<button type="button" class="chip opn' + (it.claimable ? ' mine' : '') + '"' + attr('act', 'claimf') + ' tabindex="-1"' + tipAttr(tip) + '>' + esc(wsT(it.claimable ? "js.tk.chip.claimable" : "js.tk.chip.open")) + '</button>';
  }
  function kindChip(it) {
    return '<span class="chip kind">' + esc(kindName(it.kind)) + '</span>' + (it.pending.length ? '<span class="chip req">' + wsT("js.tk.chip.pendingassign") + '</span>' : '') +
      (it.hidden ? '<span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '');
  }
  function originChip(o) {
    return o === 'agent-auto' ? '<span class="chip agent">' + wsT("js.tk.chip.agent") + '</span><span class="chip auto">' + wsT("js.tk.chip.auto") + '</span>' :
      o === 'agent' ? '<span class="chip agent">' + wsT("js.tk.chip.agent") + '</span>' : '';
  }
  function orgNameOf(id) {
    if (!id) return '';
    var list = VS && Array.isArray(VS.orgs) ? VS.orgs : [];
    for (var i = 0; i < list.length; i++) if (list[i] && list[i].id === id) return list[i].name || id;
    return id;
  }
  function orgEventInfo(evId) {
    if (!evId || !VS) return null;
    var list = VS.historyEvents || VS.events || [];
    for (var i = list.length - 1; i >= 0; i--) {
      if (list[i] && list[i].id === evId) return { ts: list[i].ts || '', by: list[i].member || '' };
    }
    return null;
  }
  function complText(c) { return c === 'assigner' ? wsT("js.tk.compl.assigner") : wsT("js.tk.compl.assignee"); }
  // an item.assign target list as HTML: people (by is "self"), then an org or subtree rule
  function targetsText(ts, by) {
    return ts.map(function (t) {
      if (typeof t === 'string') return esc(t === by ? wsT("js.tk.ev.self") : t);
      if (t.org) return wsT("js.tk.ev.rule.org", esc(orgNameOf(t.org)));
      var r = t.levels == null ? wsT("js.tk.ev.rule.all", esc(t.root)) : wsT("js.tk.ev.rule.lv", esc(t.root), esc(t.levels));
      return t.include_root ? esc(t.root) + ', ' + r : r;
    }).join(', ');
  }
  function rollupHtml(it) {
    var r = it.rollup, h = '';
    if (r.total) h += '<span class="chip roll" title="' + wsT("js.tk.roll.tip") + '">' + wsT("js.tk.roll.done", r.counts.done || 0, r.total) +
      (r.counts.blocked ? ' &middot; ' + wsT("js.tk.roll.blocked", r.counts.blocked) : '') + '</span>';
    if (r.blocked.length) h += '<span class="chip warn" title="' + esc(wsT("js.tk.roll.below.tip", r.blocked.map(function (id) { return alias(id) + ' ' + VS.items[id].title; }).join('; '))) + '">&#9888; ' + wsT("js.tk.roll.below") + '</span>';
    return h;
  }
  var END_KEY = { declined: "js.tk.end.declined", returned: "js.tk.end.returned", handed: "js.tk.end.handed", unassigned: "js.tk.end.unassigned", withdrawn: "js.tk.end.withdrawn" };
  function endText(s) { return END_KEY[s] ? wsT(END_KEY[s]) : ''; }
  function endedBy(r) {
    if (r.ended && r.ended.type === 'roles.remove_member') return ' ' + wsT('js.tk.end.removed');
    return r.ended && VS.events.filter(function (e) { return e.id === r.ended.id; }).map(function (e) { return e.type === 'roles.remove_member' ? ' ' + wsT("js.tk.end.removed") : ''; })[0] || '';
  }
  // Where an assignment stands: not accepted yet, accepted (then how it ended), or how it ended; '' when none applies.
  function standing(r) {
    if (r.state === 'pending') return wsT("js.tk.src.pending");
    if (r.accepted) return r.state !== 'live' ? wsT("js.tk.src.acceptedthen", endText(r.state)) : wsT("js.tk.src.accepted");
    return endText(r.state);
  }
  function source(r) {
    if (r.handoffOf) {
      var st = standing(r);
      return (r.sender === r.handoffFrom ? wsT("js.tk.src.handoff", esc(nm(r.sender)), esc(nm(r.by))) : wsT("js.tk.src.handoff.admin", esc(nm(r.handoffFrom)), esc(nm(r.sender)), esc(nm(r.by)))) + (st ? ', ' + st : '');
    }
    if (r.restored) return wsT("js.tk.src.restored", esc(nm(r.by)), esc(alias(r.restored.id)));
    if (r.to === r.by) return wsT("js.tk.src.self");
    if (!r.forced) return wsT("js.tk.src.sent", esc(nm(r.by)), standing(r) || r.state);
    return wsT("js.tk.src.forced", esc(nm(r.by))) + (r.inherited ? ' ' + (r.ruleLevels == null ? wsT("js.tk.src.inherited.all") : wsT("js.tk.src.inherited", r.ruleLevels)) : '');
  }
  function btn(act, label, data, cls) {
    var d = '';
    Object.keys(data || {}).forEach(function (k) { d += attr(k, data[k]); });
    return '<button class="btn mini' + (cls ? ' ' + cls : '') + '"' + attr('act', act) + d + '>' + label + '</button>';
  }
  // A side card is a spec: shown as one tab of the tab pane, or (Notifications on a
  // wide screen) as a standalone card. cardHtml draws the standalone form.
  function card(title, chip, body, extra, id, mode, cls) {
    return { title: title, chip: chip, body: body, extra: extra || '', id: id, mode: mode || '', cls: cls || '' };
  }
  function cardHtml(c) {
    return '<section class="card' + (c.cls ? ' ' + c.cls : '') + '"' + (c.id ? ' id="' + c.id + '"' : '') + (c.mode ? attr('view', c.mode) : '') + '><header><h2>' + c.title + '</h2>' + (c.chip ? '<span class="chip">' + c.chip + '</span>' : '') +
      '<span class="grow"></span>' + c.extra + '</header><div class="cbody">' + c.body + '</div></section>';
  }
  // the chosen mode is remembered per store, so it survives a viewer switch within the store
  function modeOf(key) { var m = (ui.cmodes[ui.store] || {})[key]; return CARD_MODES[key] && m ? m : 'list'; }
  function picker(key) {
    return '<span class="cmodes"' + attr('card', key) + '>' + CARD_MODES[key].map(function (m) {
      return '<button class="btn mini' + (modeOf(key) === m[0] ? ' primary' : '') + '"' + attr('act', 'cmode') + attr('card', key) + attr('mode', m[0]) + '>' + wsT(m[1]) + '</button>';
    }).join('') + '</span>';
  }
  // the view's rank (view.go rank, engine.js RANK)
  var RANK = { blocked: 0, doing: 1, todo: 2, idea: 3, 'in-review': 4, done: 5, dropped: 6 };
  function byRank(a, b) { return RANK[a.status] - RANK[b.status]; }
  // A done or dropped assignment is wrapped up: no status write, handoff, or take request until
  // it is reopened (check.go wrapped).
  function wrapped(s) { return s === 'done' || s === 'dropped'; }
  function A(id) { return VS.assignments[id]; }

  var EDIT_FIELDS = ['title', 'body', 'project', 'labels', 'due', 'start', 'priority'];
  function listOf(x) { return Array.isArray(x) ? x : []; }
  // Mentions and attachments a write carries, for one-line descriptions.
  // A field name the event sentences print: the table's, else the field itself.
  var FIELD_KEY = { title: "js.tk.field.title", body: "js.tk.field.body", project: "js.tk.field.project", labels: "js.tk.field.labels", due: "js.tk.field.due", start: "js.tk.field.start", priority: "js.tk.field.priority",
    org: "js.tk.field.org", family: "js.tk.field.family", given: "js.tk.field.given" };
  function fieldName(f) { return FIELD_KEY[f] ? wsT(FIELD_KEY[f]) : f; }
  function extrasTxt(p) {
    var m = listOf(p.mentions), a = listOf(p.attachments);
    return (m.length ? ' ' + wsT("js.tk.ev.mentions", esc(m.map(function (x) { return '@' + x; }).join(', '))) : '') +
      (a.length ? ' ' + wsTn("js.tk.ev.attachments", a.length) : '');
  }
  // ": <note>" after a sentence, '' without one
  function tail(s) { return s ? wsT("js.tk.ev.tail", esc(s)) : ''; }
  // In the sentences, the item reference is a placeholder holding ' ' + the link, or '' inside the item's own thread.
  function describe(n, inThread) {
    var p = n.payload, ref = function (id) { return inThread ? '' : ' ' + itemRef(id); };
    switch (n.type) {
      case 'item.create':
        if (inThread === 'parent') return wsT("js.tk.ev.create.sub", itemRef(n.id), esc(p.title), VS.items[n.id] ? stChip(VS.items[n.id].status) : '');
        return wsT("js.tk.ev.create", esc(kindName(p.kind)), esc(p.title)) + (inThread ? '' : ' (' + itemRef(n.id) + ')') + (p.parent && !inThread ? ' ' + wsT("js.tk.ev.create.under", itemRef(p.parent)) : '') +
          (p.due ? wsT("js.tk.ev.create.due", esc(p.due)) : '') + (p.priority != null ? wsT("js.tk.ev.create.priority", esc(VS.priority.names[p.priority] || p.priority)) : '') +
          (listOf(p.labels).length ? wsT("js.tk.ev.create.labels", esc(listOf(p.labels).map(labelName).join(', '))) : '') + extrasTxt(p);
      case 'item.edit':
        return wsT("js.tk.ev.edit", ref(p.item), EDIT_FIELDS.filter(function (f) { return f in p; }).map(function (f) {
          var v = f === 'priority' ? (VS.priority.names[p[f]] || p[f]) : f === 'labels' ? (listOf(p[f]).map(labelName).join(', ') || null) : p[f];
          return wsT("js.tk.ev.field", fieldName(f), esc(v == null ? wsT("js.tk.ev.none") : String(v).slice(0, 80)));
        }).join(', ')) + extrasTxt(p);
      case 'item.hide': return wsT("js.tk.ev.hide", ref(p.item));
      case 'item.move': return p.parent ? wsT("js.tk.ev.move", ref(p.item), itemRef(p.parent)) : wsT("js.tk.ev.move.top", ref(p.item));
      case 'item.assign':
        // a topic's assignment names its holder; the store keeps no completion for it
        return wsT(VS.items[p.item] && VS.items[p.item].kind === 'topic' ? "js.tk.ev.assign.topic" : "js.tk.ev.assign", ref(p.item), p.targets ? '<b>' + targetsText(p.targets, n.member) + '</b>' : p.mode === 'all' ? wsT("js.tk.ev.assign.all", esc(p.levels)) :
          '<b>' + esc((p.people || []).map(function (x) { return x === n.member ? wsT("js.tk.ev.self") : x; }).join(', ')) + '</b>', complText(p.completion));
      case 'item.handoff': return (p.assignee && p.assignee !== n.member ? wsT("js.tk.ev.handoff.admin", esc(p.assignee), ref(p.item), esc(p.to)) : wsT("js.tk.ev.handoff", ref(p.item), esc(p.to))) + tail(p.note);
      case 'item.accept': return wsT("js.tk.ev.accept", ref(p.item)) + tail(p.note);
      case 'item.decline': return wsT("js.tk.ev.decline", ref(p.item)) + tail(p.note);
      case 'item.return': return wsT("js.tk.ev.return", esc(p.assignee), ref(p.item)) + tail(p.note);
      case 'item.unassign': {
        var uru = p.rule && VS.rules && VS.rules[p.rule];
        var ulbl = uru && uru.org ? wsT("js.tk.ev.unassign.org", esc(orgNameOf(uru.org)))
          : (p.root ? wsT("js.tk.ev.unassign.rule", esc(p.root)) : esc(p.assignee || wsT("js.tk.ev.unassign.all")));
        return wsT("js.tk.ev.unassign", ulbl, ref(p.item));
      }
      // with assignee: an org superior's wrap-up (or the assigner's drop) on that member's assignment
      case 'item.status': return (p.assignee && p.assignee !== n.member ? wsT("js.tk.ev.status.of", esc(p.assignee), stChip(p.status), ref(p.item)) : wsT("js.tk.ev.status", stChip(p.status), ref(p.item))) + tail(p.note);
      case 'item.close': return wsT("js.tk.ev.close", ref(p.item)) + tail(p.note);
      case 'item.confirm': return wsT("js.tk.ev.confirm", esc(p.assignee), ref(p.item)) + tail(p.note);
      case 'item.reject': return wsT("js.tk.ev.reject", esc(p.assignee), ref(p.item)) + tail(p.note);
      case 'item.reopen': return wsT("js.tk.ev.reopen", esc(p.assignee || n.member), ref(p.item)) + tail(p.note);
      case 'item.open': return wsT(p.scope === 'store' ? "js.tk.ev.open.store" : "js.tk.ev.open.team", ref(p.item));
      case 'item.unopen': return wsT("js.tk.ev.unopen", ref(p.item));
      case 'item.claim': return wsT("js.tk.ev.claim", ref(p.item));
      case 'item.request': return wsT("js.tk.ev.request", esc(p.assignee), ref(p.item)) + tail(p.note);
      case 'item.approve': case 'item.deny': {
        var rq = (VS.requests || {})[p.request];
        return wsT(n.type === 'item.approve' ? "js.tk.ev.approve" : "js.tk.ev.deny", esc(rq ? rq.by : alias(p.request)), ref(p.item)) + tail(p.note);
      }
      case 'item.unrequest': return wsT("js.tk.ev.unrequest", esc(alias(p.request)), ref(p.item));
      case 'item.cc': return wsT("js.tk.ev.cc", esc(p.member), ref(p.item));
      case 'item.uncc': return wsT("js.tk.ev.uncc", esc(p.member), ref(p.item));
      case 'item.cc_org': return wsT("js.tk.ev.ccorg", esc(orgNameOf(p.org)), ref(p.item));
      case 'item.uncc_org': return wsT("js.tk.ev.unccorg", esc(orgNameOf(p.org)), ref(p.item));
      case 'comment.add': return wsT("js.tk.ev.comment", ref(p.item)) + (inThread ? '' : tail(String(p.body || '').slice(0, 90))) + extrasTxt(p);
      case 'comment.edit': return wsT("js.tk.ev.cedit", esc(alias(p.comment)), inThread ? '' : ref(n.thread || '')) + extrasTxt(p);
      case 'comment.hide': return wsT("js.tk.ev.chide", esc(alias(p.comment)));
      case 'member.profile': {
        var pr = p.profile || {}, ch = PROFILE_FIELDS.filter(function (f) { return f in pr; }).map(function (f) { return wsT("js.tk.ev.field", fieldName(f), esc(pr[f])); }).join(', ');
        return p.member && p.member !== n.member ? wsT("js.tk.ev.profile.admin", esc(p.member), ch) : wsT("js.tk.ev.profile", ch);
      }
      case 'member.join': {
        var jn = p.profile ? displayName(normProfile(p, p.member)) : p.name;
        return jn && jn !== p.member ? wsT("js.tk.ev.join.name", esc(p.member), esc(jn)) : wsT("js.tk.ev.join", esc(p.member));
      }
      case 'roles.add_member': return wsT("js.tk.ev.addmember", esc((p.member || {}).id), esc(displayName(normProfile(p.member, (p.member || {}).id))), esc(((p.member || {}).leads || []).join(', ') || wsT("js.tk.common.nobody")));
      case 'roles.set_lead': return wsT("js.tk.ev.setlead", esc(p.member), esc((p.leads || []).join(', ') || wsT("js.tk.common.nobody")));
      case 'roles.add_admin': return wsT("js.tk.ev.addadmin", esc(p.member));
      case 'roles.remove_admin': return wsT("js.tk.ev.removeadmin", esc(p.member));
      case 'roles.org_set': return p.head ? wsT("js.tk.ev.orgset", esc(p.name || p.org), esc(p.head)) : wsT("js.tk.ev.orgset.vacant", esc(p.name || p.org));
      case 'roles.org_remove': return wsT("js.tk.ev.orgremove", esc(orgNameOf(p.org)));
      case 'roles.add_key': return wsT("js.tk.ev.addkey", esc(p.key), esc(p.member));
      case 'roles.revoke_key': return wsT("js.tk.ev.revokekey", esc(p.key), esc(p.member));
      case 'roles.remove_member': return wsT("js.tk.ev.removemember", esc(p.member));
      case 'roles.set_priority_levels': return wsT("js.tk.ev.prioritylevels", esc((p.names || []).join(' / ')), esc((p.names || [])[p.default]));
      case 'roles.set_attachment_limit': return Number.isInteger(p.max_bytes) ? wsT("js.tk.ev.attlimit.size", esc(p.max_bytes), fmtSize(p.max_bytes)) : wsT("js.tk.ev.attlimit", esc(p.max_bytes));
      case 'roles.add_label': return wsT("js.tk.ev.addlabel", esc(p.name));
      case 'roles.edit_label':
        return wsT("js.tk.ev.editlabel", esc(labelName(p.label)), ['name', 'color'].filter(function (f) { return f in p; }).map(function (f) {
          return wsT("js.tk.ev.field", wsT(f === 'name' ? "js.tk.field.name" : "js.tk.field.color"), esc(p[f]));
        }).join(', '));
      case 'roles.remove_label': return wsT("js.tk.ev.removelabel", esc(labelName(p.label)));
      default: return esc(n.type);
    }
  }
  function markerHtml(n) {
    return n.markers.map(function (m) {
      var oa = esc(alias(m.other)) + ' (' + esc(m.otherMember || '?') + ')';
      if (m.kind === 'field') return '<div class="conc">' + wsT("js.tk.conc.field", esc(m.field), esc(m.mine), oa, esc(m.theirs), esc(alias(m.winner))) + '</div>';
      if (m.kind === 'removed') return '<div class="conc">' + (n.id === m.winner ? wsT("js.tk.conc.lost", oa, esc(m.field)) : wsT("js.tk.conc.ended", oa, esc(m.field))) + '</div>';
      return '<div class="conc">' + wsT("js.tk.conc.parent", esc(m.field), oa) + '</div>';
    }).join('');
  }
  function flags(n) {
    var h = '';
    if (n.late) h += '<span class="chip late" title="' + wsT("js.tk.flag.late.tip") + '">' + wsT("js.tk.flag.late") + '</span>';
    if (n.open) h += '<span class="chip local" title="' + wsT("js.tk.flag.open.tip") + '">' + wsT("js.tk.flag.open") + '</span>';
    else if (n.local) h += '<span class="chip local" title="' + wsT("js.tk.flag.local.tip") + '">' + wsT("js.tk.flag.local") + '</span>';
    if (n.status === 'flagged') h += '<span class="chip flag">' + (n.ignored ? wsT("js.tk.flag.ignored") : wsT("js.tk.flag.flagged")) + '</span>';
    if (n.status === 'waiting') h += '<span class="chip">' + wsT("js.tk.flag.waiting") + '</span>';
    if (n.markers.length) h += '<span class="chip conc-c">' + wsT("js.tk.flag.concurrent") + '</span>';
    return h;
  }
  // a same-member write that lost to another clone's concurrent write: a quiet note, no conflict box
  function supHtml(n) {
    return (n.superseded || []).map(function (x) {
      return '<div class="sup muted">' + (x.why === 'human' ? wsT("js.tk.sup.human", esc(alias(x.by)), esc(x.field)) : wsT("js.tk.sup.later", esc(alias(x.by)), esc(x.field))) + '</div>';
    }).join('');
  }
  function evLine(n, inThread) {
    // an item window's thread names the person; its event alias is the row's tooltip
    return '<li class="ev' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (n.origin !== 'human' ? ' agent' : '') + '"' + attr('ev', n.id) + (inThread ? tipAttr(n.alias) : '') + '>' +
      '<span class="when">' + esc(fmtTs(n.ts)) + '</span>' + (inThread ? '' : '<span class="al">' + esc(n.alias) + '</span>') + ' <b>' + whoName(n.member) + '</b> ' + originChip(n.origin) +
      ' ' + describe(n, inThread) + ' ' + flags(n) + (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(n) + supHtml(n) + '</li>';
  }

  // ------------------------------------------------------- viewer / badge state

  // The viewer is the view state's (the adapter's); the seen marks are the adapter's too.
  function viewerOf(name) { return vss[name] ? vss[name].viewer.stream : ''; }
  function seenKey(name) { return name + '|' + viewerOf(name); }
  function seenOf(name) { var s = AD.seen(name) || {}; return { badge: s.badge || {}, read: s.read || {} }; }
  function newKeys(name) {
    if (!vss[name]) return [];
    var seen = seenOf(name).badge;
    return vss[name].my.badge.filter(function (k) { return !seen[k]; });
  }
  // Opening a store (or switching viewer inside an open drawer) shows what is new
  // once, then clears the handle badge, like the mailbox badge. `add` keeps the keys
  // already fresh on this showing (a check that landed after it began).
  function markSeen(name, add) {
    var keys = newKeys(name);
    if (!add) ui.fresh = {};
    keys.forEach(function (k) { ui.fresh[k] = true; });
    if (keys.length) AD.markSeen(name, { badge: keys, read: [] });
  }
  function isNew(key) { return ui.fresh[key] ? '<span class="chip new">' + wsT("js.tk.chip.new") + '</span>' : ''; }

  // ---------------------------------------------------------------- item rows
 
  // An item row shows by default only what must be seen: status icon, title,
  // assignee initials, priority, due, a warning icon for flagged or concurrent writes,
  // and an unread dot. Everything else (kind, source clone and event id, dates, roll-up,
  // assignment path, notes) is the row's detail overlay, and the actions are icons; both
  // appear on hover. Keyboard focus and a tap-selection (ui.sel) show the same:
  // one CSS state, `.irow:is(:hover, :focus-within, .sel)`. (:focus-within, because
  // Chromium does not re-match :has(:focus-visible) in time when Tab moves from the row
  // into its buttons, so they hide mid-move and focus falls to the body.)
  // The overlay ignores the pointer, so moving down a list hovers the next row.

  // A topic has no status: its live record (its holder) shows the topic, a pending one stays pending.
  function stKey(it, r) { var topic = !!it && it.kind === 'topic'; return r ? (r.state !== 'live' ? r.state : topic ? 'topic' : r.status) : topic ? 'topic' : it.status; }
  function tipAttr(text) { return ' data-tip="' + esc(text) + '"'; }
  // data-tip from already-escaped HTML: drop the tags, keep the entities (the attribute decodes them)
  function tipHtml(h) { return ' data-tip="' + String(h).replace(/<[^>]*>/g, '').replace(/"/g, '&quot;') + '"'; }
  function stDesc(s) { var x = SI.info(s); return wsT("js.tk.st.tip", x.label, x.desc); }
  function stIcon(s) { return '<span class="sti"' + tipAttr(stDesc(s)) + '>' + SI.svg(s) + '</span>'; }
  // The avatar text comes from the view state (the profile rules live in the data layer).
  function initials(id) { var m = VS.members[id]; return m ? m.avatar : String(id).slice(0, 2).toUpperCase(); }
  // A person's relation to the viewer in the org tree, for the avatar color: {c: class suffix,
  // t: tooltip}. Up the viewer's lead chain is warm (r-u1 the direct lead .. r-u3 three or more
  // levels up), down the viewer's reach green (r-d1 direct reports .. r-d3), anyone else gray.
  function avRel(id) {
    var v = VS.viewer.member, m = VS.members[id], d = 0, me = VS.members[v], R = VS.viewer.reach || {};
    if (id === v) return { c: ' me', t: wsT("js.tk.av.self") };
    if (!m || m.removed) return { c: ' r-n', t: wsT("js.tk.av.none") };
    while (me && (me.leads || [])[0] && d < 50) {
      d++;
      if (me.leads[0] === id) return { c: ' r-u' + Math.min(d, 3), t: d === 1 ? wsT("js.tk.av.up1") : wsT("js.tk.av.up", d) };
      me = VS.members[me.leads[0]];
    }
    if (R[id]) return { c: ' r-d' + Math.min(R[id], 3), t: R[id] === 1 ? wsT("js.tk.av.dn1") : wsT("js.tk.av.dn", R[id]) };
    return { c: ' r-n', t: wsT("js.tk.av.none") };
  }
  // A person's avatar; every person avatar goes through here so the relation colors stay one rule.
  // tip: false where an enclosing tooltip already names the relation.
  function avHtml(id, pend, tip) {
    var r = avRel(id);
    return '<i class="av' + (pend ? ' pend' : '') + r.c + '"' + (tip === false ? '' : tipAttr(r.t)) + '>' + esc(initials(id)) + '</i>';
  }
  function holders(it) { return it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }); }
  // A topic's holder in markup (a pending handoff names its receiver as pending), '' when nobody holds it.
  function holderNames(it) { return holders(it).map(function (r) { return r.state === 'pending' ? wsT("js.tk.row.assignee.pending", whoName(r.to)) : whoName(r.to); }).join(', '); }
  // A list row's assignee, priority, or due part: a plain span (tooltip only), or, when step names
  // the item menu step the viewer may open from it (irow), a button that opens that step through
  // the click handler's STEPS dispatch, the pickers the quick-add row's chips and the item menu use.
  function partHtml(cls, tip, inner, it, step) {
    if (!step) return '<span class="' + cls + '"' + (tip ? tipAttr(tip) : '') + '>' + inner + '</span>';
    return '<button type="button" class="' + cls + ' rp"' + attr('act', step) + attr('item', it.id) + ' tabindex="-1" aria-haspopup="' + (step === 'assign' ? 'dialog' : 'menu') + '" aria-label="' + esc(tip) + '"' + tipAttr(tip) + '>' + inner + '</button>';
  }
  // an empty part's dim icon (.rpe), shown on the row's hover, focus, or tap-selection
  function partIcon(icon) { return '<svg class="i"><use href="#tki-' + icon + '"/></svg>'; }
  // ed: the viewer may assign (canAssign); the group then opens the assign step, an empty one as the dim icon
  function whoHtml(it, ed) {
    var as = holders(it), step = ed ? 'assign' : null;
    if (!as.length) return ed ? partHtml('who rpe', wsT("js.tk.act.assign"), partIcon('assign'), it, step) : '<span class="who"></span>';
    var shown = as.slice(0, 3), more = as.length - shown.length;
    // the avatars fill the group, so the group tooltip names each relation instead of one per avatar
    var tip = wsT("js.tk.row.assignees", as.map(function (r) { var n = wsT("js.tk.av.who", nmo(r.to), avRel(r.to).t); return r.state === 'pending' ? wsT("js.tk.row.assignee.pending", n) : n; }).join(', '));
    return partHtml('who', tip, shown.map(function (r) {
      return avHtml(r.to, r.state === 'pending', false);
    }).join('') + (more ? '<i class="av more">+' + more + '</i>' : ''), it, step);
  }
  // ed: the viewer may edit the task (canEdit); the level then opens the priority step
  function priHtml(it, ed) {
    if (!it || it.kind === 'topic') return '<span class="pri"></span>';
    var lv = it.priorityLevel, d = VS.priority.default;
    return partHtml('pri' + (lv > d ? ' hi' : lv < d ? ' lo' : ''), wsT("js.tk.pri.tip.name", lv, VS.priority.names.length - 1, it.priorityName) +
      (it.priority !== lv ? ' ' + wsT("js.tk.pri.stored", it.priority) : ''), esc(it.priorityName), it, ed ? 'pri' : null);
  }
  // Due: D-N within 7 days, D-day today, D+N past due (danger colour), else a short date.
  var ENDED = { done: 1, dropped: 1, declined: 1, returned: 1, handed: 1, withdrawn: 1 };
  // ed: the viewer may edit the task (canEdit); the date then opens the due step, an empty one as the dim icon.
  // A start date alone shows "from MM-DD" in the neutral badge; with a due as well the due's form
  // stays, marked as a range (rng), the start leading its tooltip.
  function dueHtml(it, st, ed) {
    if (!it || it.kind === 'topic') return '<span class="due2"></span>';
    if (!it.due && it.start) return partHtml('due2 from', wsT("js.tk.due.tip.from", it.start), esc(wsT("js.tk.due.from", it.start.slice(5))), it, ed ? 'due' : null);
    if (!it.due) return ed ? partHtml('due2 rpe', wsT("js.tk.menu.due"), partIcon('calendar'), it, 'due') : '<span class="due2"></span>';
    var state = itemDueState(it), n = dayNum(it.due) - dayNum(today()), t, c, tip;
    // The projection owns the boundary; the hourly global clock still supplies
    // the day distance (its UTC date is unchanged by hourly truncation).
    if (state === 'today') n = 0;
    else if (state === 'over') n = Math.min(-1, n);
    else if (state === 'future') n = Math.max(1, n);
    if (ENDED[st]) { t = it.due.slice(5); c = 'ended'; tip = wsT("js.tk.due.tip.ended", it.due, SI.info(st).label); }
    else if (n < 0) { t = wsT("js.tk.due.dplus", -n); c = 'over'; tip = wsTn("js.tk.due.tip.overby", -n, it.due); }
    else if (n === 0) { t = wsT("js.tk.due.dday"); c = 'now'; tip = wsT("js.tk.due.tip.today", it.due); }
    else if (n <= 7) { t = wsT("js.tk.due.dminus", n); c = 'soon'; tip = wsTn("js.tk.due.tip.in", n, it.due); }
    else { t = it.due.slice(5); c = ''; tip = wsT("js.tk.due.on", it.due); }
    if (it.start) { c += (c ? ' ' : '') + 'rng'; tip = wsT("js.tk.due.tip.from", it.start) + ' · ' + tip; }
    return partHtml('due2' + (c ? ' ' + c : ''), tip, esc(t), it, ed ? 'due' : null);
  }
  // a short start..due text for a menu value or a chip: "11-10–11-19", the due alone, or "from 11-10"
  function rangeText(s, d) { return s && d ? s.slice(5) + '–' + d.slice(5) : d ? d : s ? wsT("js.tk.due.from", s.slice(5)) : ''; }
  // A list row's label icon: an editor's way to the labels step (the chips themselves search,
  // lsearch); with the store's list empty only an admin, who can add a label there, gets it.
  function labelEdHtml(it) {
    if (!canEdit(it) || !((VS.labels || []).length || VS.viewer.admin)) return '';
    return partHtml('rlab rpe', wsT("js.tk.prop.labels.edit"), partIcon('tag'), it, 'labels');
  }
  // A list row's pencil: an editor's way into the row's edit (startRowEdit); it takes no room
  // until the row is hovered or focused, like the label icon.
  function titleEdHtml(it) {
    if (!canEdit(it)) return '';
    var tip = wsT("js.tk.act.edit");
    return '<button type="button" class="rp rpe rtit" tabindex="-1"' + attr('act', 'tedit') + attr('item', it.id) + ' aria-label="' + esc(tip) + '"' + tipAttr(tip) + '>' + partIcon('pen') + '</button>';
  }
  // A list row's edit (ui.ned = the new row's draft fields plus { item, rk, ed, canAsg, orig }): the
  // pencil turns the row into the new row's own markup (nrowChips, newMenu) prefilled from the item.
  // The chips only change the draft; Enter saves it, the x or Esc drops it. Like the new row it has
  // no blur or outside-click rule: it stays open until one of those. Saving sends what changed
  // (editRowEvents): one item.edit of the title, priority, due, and labels; the viewer's own status
  // as item.status (item.close for done), as the item menu does; and the assignees as item.assign
  // and item.unassign, only where the viewer may assign (canAssign: the creator).
  // editHeld: the people really assigned (live or pending), the fallback and rule requests left out.
  function editHeld(it) {
    var w = holders(it).filter(function (r) { return !r.fallback && !r.ruleRequest; }).map(function (r) { return r.to; });
    return w.filter(function (x, i) { return w.indexOf(x) === i; });
  }
  // the assignee chip's prefill: the people assigned; a task nobody else holds shows its creator
  // (the store's fallback holding), an unheld topic nobody (a topic has no fallback)
  function editWho(it) {
    var w = editHeld(it);
    return w.length || it.kind === 'topic' ? w : [it.author];
  }
  function startRowEdit(id, rk) {
    var it = VS.items[id];
    if (!it || !canEdit(it)) return;
    var r = mineLive(it), who = editWho(it), n = ui.ned = { item: id, rk: rk, ed: id, val: it.title, kind: it.kind, st: r ? r.status : it.status, who: who, compl: null,
      start: it.start || '', due: it.due || '', pri: it.priority != null ? it.priority : null, labels: (it.labels || []).slice(), canAsg: canAssign(it, VS.viewer.member) };
    // what the editor opened on: a save sends only the fields the user moved off these (editRowEvents)
    n.orig = { title: n.val, st: n.st, start: n.start, due: n.due, pri: n.pri, who: who.slice(), held: editHeld(it), labels: n.labels.slice() };
    render();
    var el = $('tkned');
    if (el) { el.focus(); el.select(); }
  }
  // The kind icon is plain: the store does not change an item's kind after creation (item.edit
  // has no kind field), so it names the kind rather than offering the new row's toggle. A click
  // anywhere in the row opens no item window (clickItem).
  function editRowHtml(o) {
    var n = ui.ned;
    return '<div class="nrow ned"' + (o.depth ? ' style="--ind:' + o.depth + '"' : '') + attr('item', n.item) + attr('rk', n.rk) + '><span class="nkind"' + tipAttr(kindName(n.kind)) + '>' + SI.svg(n.kind === 'topic' ? 'topic' : 'todo') + '</span>' +
      '<input type="text" id="tkned" data-keep autocomplete="off" value="' + esc(n.val) + '" aria-label="' + esc(wsT("js.tk.form.title")) + '">' +
      nrowChips(n) +
      '<button type="button" class="ia"' + attr('act', 'ned-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.new.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  // The events a save of draft n sends for item it ([] for none): only the fields the user changed
  // since the editor opened (n.orig) that still differ from the item, so a field another viewer
  // wrote meanwhile and the user left alone is kept.
  function editRowEvents(n, it, title) {
    var ev = [], ep = { item: it.id }, topic = it.kind === 'topic', r = mineLive(it), st = nrowSt(n), o = n.orig;
    if (title && title !== o.title.trim() && title !== it.title) ep.title = title;
    if (!topic) {
      var pri = nrowPri(n), due = n.due || null, start = n.start || null;
      if (n.pri !== o.pri && pri !== (it.priority != null ? it.priority : VS.priority.default)) ep.priority = pri;
      if (start !== (o.start || null) && start !== (it.start || null)) ep.start = start;
      if (due !== (o.due || null) && due !== (it.due || null)) ep.due = due;
    }
    // the draft's label picks (what it added and dropped since the prefill) over the item's labels
    // as they are now, so a label another viewer wrote meanwhile is kept; labels the list no longer
    // holds drop, as the edit form's save does
    var picked = labelsOver(it.labels, labelPicks(n.orig.labels, n.labels));
    if (!sameLabels(picked, it.labels)) ep.labels = picked;
    if (Object.keys(ep).length > 1) ev.push(['item.edit', ep]);
    if (st && r && st !== o.st && st !== r.status) ev.push(st === 'done' ? ['item.close', { item: it.id }] : ['item.status', { item: it.id, status: st }]);
    // the assignees as a delta over who is assigned now: item.assign for new picks since open who
    // hold nothing, item.unassign for the ones dropped from the prefill who still hold it. The
    // completion pick rides on the added people only: the store keeps a holding's completion, and
    // re-assigning a kept person to change it would end their record (status, acceptance).
    // A kept fallback prefill needs a real assignment when a new pick would displace the fallback.
    if (n.canAsg && n.who.slice().sort().join(' ') !== o.who.slice().sort().join(' ')) {
      var cur = editHeld(it), add = n.who.filter(function (x) { return o.held.indexOf(x) < 0 && cur.indexOf(x) < 0; });
      if (add.length) ev = ev.concat(asgEvents(it.id, add.map(function (id) { return { t: 'p', id: id }; }), n.compl));
      o.who.forEach(function (x) { if (n.who.indexOf(x) < 0 && cur.indexOf(x) >= 0) ev.push(['item.unassign', { item: it.id, assignee: x }]); });
    }
    return ev;
  }
  function endRowEdit() {
    var n = ui.ned;
    ui.ned = null;
    render();
    var a = n && document.querySelector('.irow[data-item="' + CSS.escape(n.item) + '"]');
    if (a) a.focus({ preventScroll: true });
  }
  function saveRowEdit() {
    var n = ui.ned, it = n && VS.items[n.item], el = $('tkned');
    if (!it) return;
    var ev = editRowEvents(n, it, (el ? el.value : n.val).trim()); // a blank title keeps the old one
    if (!ev.length) { toast(wsT("js.tk.toast.unchanged")); return endRowEdit(); }
    // only this save's draft closes: another row's edit opened while this one was in flight stays
    commit(ev, null, function () { if (ui.ned === n) ui.ned = null; });
  }
  var FLAGGED = {}, UNREAD = {}; // per render: item -> flagged writes in its own thread; item -> unread notification entries
  function warnHtml(it) {
    var f = FLAGGED[it.id] || 0, c = it.concurrent;
    if (!f && !c) return '';
    var tip = wsT("js.tk.row.warn", [f ? wsTn("js.tk.row.flagged", f) : '', c ? wsTn("js.tk.row.concurrent", c) : ''].filter(Boolean).join(', '));
    return '<span class="wicon' + (f ? ' flag' : '') + '"' + tipAttr(tip) + '><svg class="i"><use href="#tki-warn"/></svg></span>';
  }
  function dotHtml(it) {
    var n = UNREAD[it.id];
    return n ? '<span class="udot"' + tipAttr(wsTn("js.tk.row.unread", n)) + '></span>' : '';
  }

  // Row actions: every action the viewer may take on the item, from the view state, as
  // descriptors [act, icon, label, data, go]. The context menu (menuHtml) draws them; each
  // dispatches through the same data-act handlers.
  function dataAttrs(data) {
    var d = '';
    Object.keys(data || {}).forEach(function (k) { d += attr(k, data[k]); });
    return d;
  }
  function ia(act, icon, label, data, cls) {
    return '<button type="button" class="ia' + (cls ? ' ' + cls : '') + '"' + attr('act', act) + dataAttrs(data) + ' aria-label="' + esc(label) + '"' + tipAttr(label) + '><svg class="i"><use href="#tki-' + icon + '"/></svg></button>';
  }
  function actList(it) {
    var out = [];
    if (!it || it.hidden) return out;
    var v = VS.viewer.member, recs = it.assignments.map(A), id = it.id;
    if (recs.some(function (r) { return r.to === v && r.state === 'pending'; })) {
      if (canAccept(it)) out.push(['accept', 'accept', wsT("js.tk.act.accept"), { item: id }, 1]);
      out.push(['note', 'decline', wsT("js.tk.act.decline"), { item: id, op: 'item.decline' }]);
    }
    recs.forEach(function (r) {
      var lead = VS.members[r.to] && VS.members[r.to].leads[0], wr = wrapAuth(it, r);
      if (canReview(r))
        out.push(['confirm', 'confirm', wsT("js.tk.act.confirm", nm(r.to)), { item: id, who: r.to }, 1], ['note', 'reject', wsT("js.tk.act.reject", nm(r.to)), { item: id, who: r.to, op: 'item.reject' }]);
      if (wr) out.push(['wrap', wr === 'sup' ? 'status' : 'x', wsT(wr === 'sup' ? "js.tk.act.wrap" : "js.tk.act.drop", nm(r.to)), { item: id, who: r.to }]);
      if (r.state === 'live' && r.accepted && lead === v) out.push(['note', 'return', wsT("js.tk.act.return", nm(r.to)), { item: id, who: r.to, op: 'item.return' }]);
      // a rule request is withdrawn as its one rule, not as a person (the store refuses that)
      if (r.state === 'pending' && (r.by === v || r.sender === v) && r.to !== v) out.push(['unassign', 'withdraw', wsT("js.tk.act.withdraw", nm(r.to)), r.ruleRequest ? { item: id, root: r.to, rule: r.rule } : { item: id, who: r.to }]);
    });
    if (it.claimable) out.push(['claim', 'accept', wsT("js.tk.act.claim"), { item: id }, 1]);
    reqsOf(it).forEach(function (q) {
      if (canApprove(it, q)) out.push(['approve', 'accept', wsT("js.tk.act.approve", nm(q.by)), { item: id, req: q.id }, 1]);
      if (canAnswer(it, q)) out.push(['note', 'decline', wsT("js.tk.act.deny", nm(q.by)), { item: id, who: q.by, req: q.id, op: 'item.deny' }]);
      if (q.state === 'pending' && q.by === v) out.push(['unrequest', 'withdraw', wsT("js.tk.act.unrequest", nm(q.assignee)), { item: id, req: q.id }]);
    });
    return out;
  }
  // Resolving an in-review record (check.go item.confirm / item.reject): its assigner or any org
  // superior of its assignee, on any completion mode; a self-assigned one is its holder's.
  function canReview(r) {
    var v = VS.viewer.member;
    return r.state === 'live' && r.status === 'in-review' && (r.by === v || !!VS.viewer.reach[r.to]);
  }
  // A status write on another member's assignment (check.go item.status with assignee): 'sup' when
  // the viewer is an org superior of the assignee (blocked, done with no review, or dropped), 'drop'
  // when the viewer is only its assigner (dropped alone), else ''. Only on a live, not wrapped-up
  // task record (a creator-fallback holding included), never a pending one.
  var WRAP_SET = { sup: ['blocked', 'done', 'dropped'], drop: ['dropped'] };
  function wrapAuth(it, r) {
    var v = VS.viewer.member;
    if (!it || it.hidden || it.kind === 'topic' || !r || r.to === v || r.state !== 'live' || wrapped(r.status)) return '';
    return VS.viewer.reach[r.to] ? 'sup' : r.by === v ? 'drop' : '';
  }
  // The status choices on the viewer's own live assignment r (check.go item.status / item.close):
  // [{ s, label, dis, why }]. done is item.close, "request done" when the assigner confirms, and
  // refused while such a close is in review; dropped is the assignee's own only when they close it
  // themselves (completion assignee) or assigned it to themselves.
  var OWN_SET = ['idea', 'todo', 'doing', 'blocked', 'in-review', 'done', 'dropped'];
  function ownStatusOpts(r) {
    var v = VS.viewer.member, gate = r.completion === 'assigner';
    return OWN_SET.map(function (s) {
      var o = { s: s, label: s === 'done' && gate ? wsT("js.tk.menu.reqdone") : stLabel(s), dis: false, why: '' };
      if (s === 'done' && gate && r.status === 'in-review') { o.dis = true; o.why = wsT("js.tk.menu.inreview.why", nm(r.by)); }
      if (s === 'dropped' && gate && r.by !== v) { o.dis = true; o.why = wsT("js.tk.menu.drop.why", nm(r.by)); }
      return o;
    });
  }
  // The viewer's own live assignment on the item, if any.
  function mineLive(it) { var v = VS.viewer.member; return it.assignments.map(A).filter(function (r) { return r.to === v && r.state === 'live'; })[0]; }
  // item.edit, as the store judges it (check.go needCreatorOrAssignee): the creator or a live
  // assignee (an admin too once the creator was removed). Hiding stays the creator's (canHide).
  function canEdit(it) { return (isCreator(it) || !!mineLive(it)) && !it.hidden; }
  function canHide(it) { return isCreator(it) && !it.hidden; }
  // The creator, or an admin once the creator was removed (check.go creatorGone).
  function isCreator(it) {
    var gone = VS.viewer.admin && VS.members[it.author] && VS.members[it.author].removed;
    return it.author === VS.viewer.member || !!gone;
  }
  // Taking work, as the store judges it (check.go): the creator opens a task nobody but the
  // fallback holds (not a topic, not hidden, not already open, the fallback not done) to a scope,
  // and withdraws an open opening; claiming is items[].claimable, computed per viewer. A take
  // request names one live, explicit, not-done assignment of someone else, and is refused while
  // the viewer holds a live or pending record on the item or already asked for that assignment;
  // the named assignment's holder or the creator answers it (approves it only while that
  // assignment is not done or dropped), and its requester withdraws it.
  function fallbackRec(it) { return it.assignments.map(A).filter(function (r) { return r.fallback && r.state === 'live'; })[0] || null; }
  function holdsOn(it, who) { return it.assignments.map(A).some(function (r) { return r.to === who && (r.state === 'live' || r.state === 'pending') && !r.fallback && !r.ruleRequest; }); }
  function canOpen(it) {
    if (!isCreator(it) || it.hidden || it.kind === 'topic' || it.open) return false;
    var fb = fallbackRec(it);
    return !!fb && !wrapped(fb.status) && !it.assignments.map(A).some(function (r) { return (r.state === 'live' || r.state === 'pending') && !r.fallback && !r.ruleRequest; });
  }
  function canUnopen(it) { return !!it.open && isCreator(it); }
  function reqsOf(it) { var all = VS.requests || {}; return listOf(it.requests).map(function (id) { return all[id]; }).filter(Boolean); }
  function reqTargets(it) {
    var v = VS.viewer.member, asked = {};
    if (it.hidden || !VS.viewer.isMember || holdsOn(it, v)) return [];
    reqsOf(it).forEach(function (q) { if (q.by === v && q.state === 'pending') asked[q.assignment] = true; });
    return it.assignments.map(A).filter(function (r) { return r.state === 'live' && !r.fallback && !r.ruleRequest && !wrapped(r.status) && r.to !== v && !asked[r.id]; });
  }
  function canAnswer(it, q) { return q.state === 'pending' && (q.assignee === VS.viewer.member || isCreator(it)); }
  // An approval also needs the named assignment not wrapped up: a done or dropped record stays
  // live, so its requests never lapse, and the store refuses to hand it over (check.go
  // takeWrapped). A denial stays open.
  function canApprove(it, q) { var r = A(q.assignment); return canAnswer(it, q) && !(r && r.state === 'live' && wrapped(r.status)); }
  // Accepting answers every pending record of the viewer on the item; it also needs none of them
  // to be a handoff of a done or dropped assignment: a wrapped-up record stays live, so its handoff
  // never lapses, and the store refuses to pass it on (check.go handoffWrapped). A decline stays open.
  function canAccept(it) {
    var v = VS.viewer.member, mine = it.assignments.map(A).filter(function (r) { return r && r.to === v && r.state === 'pending'; });
    return mine.length > 0 && !mine.some(function (r) { var o = r.handoffOf && A(r.handoffOf); return !!o && o.state === 'live' && wrapped(o.status); });
  }
  // A subtask's parent is assigned to or created by its author.
  function canSplit(it) { return (!!mineLive(it) || it.author === VS.viewer.member) && !it.hidden; }
  function canStatus(it) { var r = !it.hidden && it.kind !== 'topic' && mineLive(it); return !!r && !wrapped(r.status); }
  // item.move, as the store judges it (check.go checkMove): the item rule is the creator or a live
  // assignee (an admin too once the creator was removed); only when the parent changes, the
  // destination rule: not hidden, the viewer's (created or live-assigned; a topic too), and not the
  // item or inside its own subtree. A same-parent reorder and a move to the top level take the item
  // rule alone. moveWhy(id, dest) is '' when the store takes the move, else the reason; dest null is
  // the top level. The dashboard offers no move the store would refuse.
  function canMove(it) {
    var gone = VS.viewer.admin && VS.members[it.author] && VS.members[it.author].removed;
    return !it.hidden && (it.author === VS.viewer.member || !!mineLive(it) || !!gone);
  }
  function moveWhy(id, dest) {
    var it = VS.items[id], d = dest ? VS.items[dest] : null;
    if (!it || it.hidden) return wsT("js.tk.mv.why.hidden", it ? it.alias : alias(id));
    if (!canMove(it)) return wsT("js.tk.mv.why.mine", it.alias);
    if (!dest || dest === (it.parent || null)) return '';
    if (!d || d.hidden) return wsT("js.tk.mv.why.hidden", d ? d.alias : alias(dest));
    if (d.author !== VS.viewer.member && !mineLive(d)) return wsT("js.tk.mv.why.mine", d.alias);
    for (var cur = d, n = 0; cur && n < 1000; cur = cur.parent ? VS.items[cur.parent] : null, n++) if (cur.id === id) return wsT("js.tk.mv.why.sub", it.alias);
    return '';
  }
  // a parent's children in item order (hidden ones included); null: the top level
  function sibsOf(parent) { return parent ? (VS.items[parent] ? VS.items[parent].children : []) : VS.itemOrder.filter(function (id) { return !VS.items[id].parent; }); }
  // What moves of a set of items: the ones with no ancestor in the set (the rest travel with it), in tree order.
  function moveRoots(ids) {
    var set = {}, pos = {};
    ids.forEach(function (id) { if (VS.items[id]) set[id] = true; });
    VS.itemOrder.forEach(function (id, i) { pos[id] = i; });
    return Object.keys(set).filter(function (id) {
      for (var p = VS.items[id].parent, n = 0; p && VS.items[p] && n < 1000; p = VS.items[p].parent, n++) if (set[p]) return false;
      return true;
    }).sort(function (a, b) { return (a in pos ? pos[a] : 1e9) - (b in pos ? pos[b] : 1e9); });
  }
  // Roots placed at slot { parent, after } (after: an id, '' first, undefined last): one item.move per
  // root the slot takes, each anchored after the one before, so they land together in tree order.
  // refused: [[id, reason]] for the others, which stay.
  function moveBatch(roots, slot) {
    var evs = [], refused = [], prev = slot.after;
    roots.forEach(function (id) {
      var why = moveWhy(id, slot.parent || null);
      if (why) { refused.push([id, why]); return; }
      var p = { item: id, parent: slot.parent || null };
      if (prev !== undefined) p.after = prev;
      evs.push(['item.move', p]);
      prev = id;
    });
    return { evs: evs, refused: refused };
  }
  // the move would leave the item where it is
  function moveNoop(id, slot) {
    var it = VS.items[id];
    if (!it || (it.parent || null) !== (slot.parent || null)) return false;
    var s = sibsOf(slot.parent || null), i = s.indexOf(id);
    return slot.after === undefined ? i === s.length - 1 : slot.after === '' ? i === 0 : i > 0 && s[i - 1] === slot.after;
  }
  // The write: the refused roots are named (after the result toast, so they stay readable), the rest move.
  function moveApply(roots, slot) {
    var b = moveBatch(roots, slot), why = b.refused.map(function (x) { return wsT("js.tk.mv.stays", esc(VS.items[x[0]] ? VS.items[x[0]].alias : alias(x[0])), esc(x[1])); }).join('<br>');
    if (!b.evs.length) { if (why) toast(why); return; }
    commit(b.evs, why ? function () { toast(why); } : null);
  }
  // Checkbox selection in the Items list (ui.msel[store] = { ids: { item id: true }, at: the anchor
  // row's key }). A row's checkbox shows on hover; clicking it enters selection mode, which lasts
  // while anything is selected (Esc, or the x of the badge atop the list, clears it). Selection
  // mode is multi-select: a plain row click toggles that row (and makes it the anchor), and
  // Shift-click adds the range from the anchor, keeping the rest; Ctrl/Cmd change nothing. The
  // item window opens from the menu or Enter. Ranges run over the rows shown, in their order, so a
  // folded subtree's rows are not in one. A selection is by item: an item shown twice is selected
  // in both places. Each panel draw keeps only what it drew (MSEEN), so a row a filter, a fold, or
  // a scope change took away leaves the selection.
  var MSEEN = null; // per panel draw: { item id: true } of the rows drawn
  function mselOf() { return ui.msel[ui.store] || null; }
  function mselHas(id) { var s = mselOf(); return !!s && !!s.ids[id]; }
  function mselIds() { var s = mselOf(); return s ? Object.keys(s.ids) : []; }
  function mselOn() { return mselIds().length > 0; }
  function mselClear() { delete ui.msel[ui.store]; }
  function mcbHtml(it, on) {
    return '<button type="button" class="mcb" role="checkbox" aria-checked="' + on + '"' + attr('act', 'msel') + attr('item', it.id) + ' tabindex="-1" aria-label="' + esc(wsT("js.tk.sel.aria", it.title)) + '"' +
      tipAttr(wsT("js.tk.sel.tip")) + '></button>';
  }
  // The pick on selection s ({ ids, at }) over the rows shown (keys: [item id, row key] in order)
  // for a click on row i: Shift with an anchor shown adds the range between them, anything else
  // toggles row i and makes it the anchor.
  function mselPick(s, keys, i, shift) {
    var j = -1;
    if (shift && s.at) keys.some(function (x, k) { if (x[1] === s.at) { j = k; return true; } return false; });
    if (j >= 0 && i >= 0) {
      keys.slice(Math.min(i, j), Math.max(i, j) + 1).forEach(function (x) { s.ids[x[0]] = true; });
      return s;
    }
    var id = keys[i] ? keys[i][0] : null;
    if (!id) return s;
    if (s.ids[id]) delete s.ids[id]; else s.ids[id] = true;
    s.at = keys[i][1];
    return s;
  }
  // A click on a list row in selection mode, or on a row's checkbox (which enters the mode); both
  // act alike (mselPick).
  function mselClick(row, e) {
    var area = row.closest('#card-items .cbody'), rows = area ? Array.prototype.slice.call(area.querySelectorAll('.irow')) : [row];
    var rk = row.getAttribute('data-rk'), s = ui.msel[ui.store] || (ui.msel[ui.store] = { ids: {}, at: null });
    mselPick(s, rows.map(function (x) { return [x.getAttribute('data-item'), x.getAttribute('data-rk')]; }), rows.indexOf(row), e.shiftKey);
    if (!mselOn()) mselClear();
    render();
    // the clicked row keeps focus, so Enter opens its item window
    var back = document.querySelector('#card-items [data-rk="' + CSS.escape(rk) + '"]');
    if (back) back.focus({ preventScroll: true });
  }
  // A click on a list row that no control of it took (a part's picker, the status icon, the "⋯"
  // menu dispatch by data-act first): the title opens the item window (true: the caller goes on to
  // open it), anywhere else only focuses the row, where Enter opens it.
  function rowClick(row, t) {
    if (t.closest('a.title')) return true;
    if (t.closest('input')) return false; // the title's in-place input keeps focus
    row.focus({ preventScroll: true });
    return false;
  }
  // The element whose item window a click no control took opens, or null: none from a form field
  // or the dialog, nor from a row's in-place edit (editRowHtml, .ned), which stays open until saved
  // or dropped, though it carries the row's data-item.
  function clickItem(t) {
    return t.closest('select, textarea, input, #tkdlg, #tkpm, .ned') ? null : t.closest('[data-item]');
  }
  // The "⋯" button every menu-capable row, card, entry, and the item window header carries.
  function moreBtn(id, cls, tab) {
    return '<button type="button" class="ia mbtn' + (cls ? ' ' + cls : '') + '"' + attr('act', 'menu') + attr('item', id) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.menu.more")) + '"' +
      tipAttr(wsT("js.tk.menu.more")) + (tab ? '' : ' tabindex="-1"') + '><svg class="i"><use href="#tki-more"/></svg></button>';
  }
  // A status icon: a button opening the menu's status part when the viewer can set the
  // item's status, else the plain icon.
  // rec: the assignment record a row shows; another member's record keeps the plain icon.
  function stBtn(s, it, rec) {
    if (!it || !canStatus(it) || (rec && rec.to !== VS.viewer.member)) return stIcon(s);
    return '<button type="button" class="sti stb"' + attr('act', 'mstatus') + attr('item', it.id) + ' tabindex="-1" aria-haspopup="menu" aria-label="' + esc(stDesc(s)) + '"' + tipAttr(stDesc(s)) + '>' + SI.svg(s) + '</button>';
  }

  // The detail overlay: what the row leaves out by default.
  function detailsHtml(it, r, extra) {
    var d = [], st = stKey(it, r), ist = stKey(it);
    d.push(kindChip(it) + ' ' + wsT("js.tk.row.by", '<span class="al">' + esc(it.alias) + '</span>', esc(nm(it.author))) + (it.project ? ' &middot; ' + esc(it.project) : ''));
    d.push('<b>' + esc(SI.info(st).label) + '</b> <span class="muted">' + esc(SI.info(st).desc) + '</span>' +
      (r && r.state === 'live' && r.statusOrigin && r.statusOrigin !== 'human' ? ' &middot; ' + wsT("js.tk.row.setby", esc(nm(r.statusBy)), esc(r.statusOrigin)) : '') +
      (r && ist !== st && it.kind !== 'topic' ? ' &middot; ' + wsT("js.tk.row.itemst", esc(SI.info(ist).label)) : '') +
      (r && r.stale ? ' &middot; ' + wsT("js.tk.row.stale", STALE_DAYS) : ''));
    var roll = rollupHtml(it);
    if (roll) d.push(roll);
    if (r) d.push(source(r) + (it.kind === 'topic' ? '' : ' &middot; ' + complText(r.completion)) + (it.chain.length > 1 ? ' &middot; ' + wsT("js.tk.row.chain", esc(it.chain.join(' > '))) : ''));
    else if (it.kind === 'topic') { if (holders(it).length) d.push(wsT("js.tk.row.heldby", holderNames(it))); }
    else d.push(holders(it).length ? '&rarr; ' + holders(it).map(function (x) { return esc(nm(x.to)) + ' <span class="muted">' + esc(SI.info(stKey(it, x)).label) + '</span>'; }).join(', ') : '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>');
    var note = r && (r.note ? [r.note, r.noteBy] : r.handoffNote ? [r.handoffNote, r.sender] : r.answerNote ? [r.answerNote, r.to] : null);
    if (note) d.push('&ldquo;' + esc(note[0]) + '&rdquo; <span class="muted">(' + esc(note[1]) + ')</span>');
    if (extra) d.push(extra);
    var f = FLAGGED[it.id] || 0;
    d.push('<span class="muted">' + wsT("js.tk.row.created", esc(fmtTs(it.ts))) + (it.edited ? ' &middot; ' + wsT("js.tk.row.edited", esc(fmtTs(it.edited.ts))) : '') + ' &middot; ' + wsT("js.tk.row.activity", esc(fmtTs(it.lastActivity))) +
      (r ? ' &middot; ' + wsT("js.tk.row.updated", esc(fmtTs(r.lastUpdate || r.ts))) : '') + (f ? ' &middot; ' + wsT("js.tk.row.nflagged", f) : '') + (it.concurrent ? ' &middot; ' + wsT("js.tk.row.nconcurrent", it.concurrent) : '') + '</span>');
    return d.map(function (x) { return '<div>' + x + '</div>'; }).join('');
  }
  // o: { rk (unique row key in the drawer), item, rec?, more?, depth?, closed? (greyed: done, in the "recently closed" section or kept in the tree),
  //      ctx? (a context row of a tree: an ancestor of a match, not a match itself), tw? (the tree's fold control, in place of the depth arrow), tsub? (the folded "N subtasks" chip) }
  // The row is the one row component every list and tree draws; data-item carries the item id.
  // Only its title opens the item window (rowClick); each part the viewer may change opens its own
  // picker (partHtml, labelEdHtml), the status icon the status menu (stBtn).
  // A row drags (a list drop moves it with its subtree, see mvDrop) when the viewer may move it, or
  // when it is selected: then the selection moves.
  function irow(o) {
    var it = VS.items[o.item], r = o.rec, st = stKey(it, r), ms = mselHas(it.id), ed = canEdit(it) && it.kind !== 'topic';
    if (MSEEN) MSEEN[it.id] = true;
    if (ui.ned && ui.ned.item === it.id && ui.ned.rk === o.rk) return editRowHtml(o); // the row is being edited in place
    return '<div class="irow' + (ui.sel === o.rk ? ' sel' : '') + (ms ? ' msel' : '') + (o.closed ? ' closed' : '') + (o.ctx ? ' ctx' : '') + '" tabindex="0"' + (ms || canMove(it) ? ' draggable="true"' : '') +
      attr('item', it.id) + attr('rk', o.rk) + (r ? attr('rec', r.id) : '') +
      (o.depth ? ' style="--ind:' + o.depth + '"' : '') + '><div class="ir-main">' + mcbHtml(it, ms) + (o.tw != null ? o.tw : o.depth ? '<span class="ind">&#8627;</span>' : '') + stBtn(st, it, r) +
      '<a class="title" href="#" tabindex="-1"' + attr('item', it.id) + '>' + esc(it.title) + '</a>' + titleEdHtml(it) + (o.tsub || '') + openChip(it) + labelChips(it, 'lsearch') + warnHtml(it) + dotHtml(it) + labelEdHtml(it) + '<span class="grow"></span>' +
      '<span class="ir-acts">' + moreBtn(it.id, '', true) + '</span>' + whoHtml(it, canAssign(it, VS.viewer.member)) + priHtml(it, ed) + dueHtml(it, st, ed) + '</div>' +
      '<div class="ir-more">' + detailsHtml(it, r, o.more) + '</div></div>';
  }
  function sub(label, n) { return '<div class="sub">' + label + (n != null ? ' <span class="subn">' + n + '</span>' : '') + '</div>'; }

