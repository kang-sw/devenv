// Tracker adapter contract, profile display, and org tree helpers.
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
