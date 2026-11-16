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
//   qsSlot           an element in the header for the search-palette button
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
//   storagePrefix    browser-storage key prefix for display state (view modes, drawer
//                    height, recently-closed fold, tree folds); default "ws.tracker."
//   drawerHeight     {get() -> px, set(px)}: the host's own drawer height, so every
//                    store drawer opens at the height the host's drawers do and a
//                    store handle's drag sets it for all (get answering 0 or less
//                    means the default, 320px); without it the renderer keeps one
//                    height for all its stores under storagePrefix
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
    drh: 0,               // the one drawer height in px of every store, set by dragging a handle (display state, stored; 0: DR_DEF); unused with opts.drawerHeight
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
  function dueChip(it) {
    if (!it || !it.due) return '';
    var over = it.due < today() && !wrapped(it.status), now = it.due === today() && !wrapped(it.status);
    return '<span class="chip due' + (over ? ' over' : now ? ' now' : '') + '" title="' + (over ? wsT("js.tk.due.tip.over", esc(it.due)) : now ? wsT("js.tk.due.tip.now", esc(it.due)) : wsT("js.tk.due.on", esc(it.due))) + '">' +
      (now ? wsT("js.tk.due.today") : wsT("js.tk.due.on", esc(it.due.slice(5)))) + '</span>';
  }
  // The store's label list (view.labels, live labels in add order); an item carries label ids.
  function labelOf(id) { return (VS.labels || []).filter(function (l) { return l.id === id; })[0] || null; }
  // A removed label is gone from view.labels; the activity log still names it, by its add
  // event and its applied renames (EV, the events by id).
  function labelName(id) {
    var l = labelOf(id), n = EV[id] && EV[id].type === 'roles.add_label' ? EV[id].payload.name : null;
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
  function complText(c) { return c === 'assigner' ? wsT("js.tk.compl.assigner") : wsT("js.tk.compl.assignee"); }
  // an item.assign target list as HTML: people (by is "self"), then a subtree rule as its root's
  // reach, the root itself listed first when the rule includes it
  function targetsText(ts, by) {
    return ts.map(function (t) {
      if (typeof t === 'string') return esc(t === by ? wsT("js.tk.ev.self") : t);
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
  function endedBy(r) { return r.ended && VS.events.filter(function (e) { return e.id === r.ended.id; }).map(function (e) { return e.type === 'roles.remove_member' ? ' ' + wsT("js.tk.end.removed") : ''; })[0] || ''; }
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
      case 'item.unassign': return wsT("js.tk.ev.unassign", p.root ? wsT("js.tk.ev.unassign.rule", esc(p.root)) : esc(p.assignee || wsT("js.tk.ev.unassign.all")), ref(p.item));
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
    var n = dayNum(it.due) - dayNum(today()), t, c, tip;
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
    return t.closest('select, textarea, input, #tkdlg, .ned') ? null : t.closest('[data-item]');
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

  // ------------------------------------------------------------------ cards

  // The notification feed: one tab, list only, newest first. Every entry
  // is derived from the view state; an entry only opens the item window, and the
  // actions live on the item's row in the Items panel. Read state is local
  // UI state per viewer clone, like the handle badge's `seen`; no event is written
  // (in the product the daemon keeps it per clone on the local filesystem).
  var NK = { assigned: "js.tk.nk.assigned", pending: "js.tk.nk.pending", answer: "js.tk.nk.answer", reference: "js.tk.nk.reference", done: "js.tk.nk.done", blocked: "js.tk.nk.blocked", dropped: "js.tk.nk.dropped",
    confirm: "js.tk.nk.confirm", reply: "js.tk.nk.reply", due: "js.tk.nk.due", mention: "js.tk.nk.mention", request: "js.tk.nk.request", cc: "js.tk.nk.cc" };
  function snippet(s) { s = String(s || ''); return '&ldquo;' + esc(s.length > 80 ? s.slice(0, 77) + '...' : s) + '&rdquo;'; }
  // Every entry also carries `ev`, the event it is about (none for a due date, which has
  // none), so a click can open the item window at what the entry refers to (focusTarget).
  var STATUS_EV = { 'item.status': 1, 'item.close': 1, 'item.confirm': 1, 'item.reject': 1, 'item.reopen': 1 };
  // item.status carries assignee when an org superior (or the assigner) wrote another member's assignment
  function assigneeOf(e) { return e.type === 'item.close' ? e.member : e.payload.assignee || e.member; }
  // the assignment record's latest status-side event: the one written at its lastUpdate, else the last one
  function statusEv(r) {
    var l = VS.events.filter(function (e) { return STATUS_EV[e.type] && e.status === 'applied' && e.payload.item === r.item && assigneeOf(e) === r.to; });
    var at = l.filter(function (e) { return e.ts === r.lastUpdate; });
    return (at.length ? at : l).slice(-1).map(function (e) { return e.id; })[0] || null;
  }
  // A CC notice's sentence (my.ccNotices: status writes and comments on an item the viewer is
  // CC'd on); a status write's note and a comment's text ride along as its tail.
  var CC_KEY = { 'item.status': "js.tk.feed.cc.status", 'item.close': "js.tk.feed.cc.close", 'item.confirm': "js.tk.feed.cc.confirm",
    'item.reject': "js.tk.feed.cc.reject", 'item.reopen': "js.tk.feed.cc.reopen", 'comment.add': "js.tk.feed.cc.comment" };
  function ccNoticeTxt(x) {
    var e = EV[x.event], p = (e && e.payload) || {}, by = esc(nm(x.by)), to = esc(nm(x.to));
    if (x.type === 'comment.add') { var c = VS.comments[x.event]; return wsT("js.tk.feed.cc.comment", by, snippet(c ? c.body : p.body)); }
    if (!CC_KEY[x.type]) return null;
    var txt = x.type === 'item.status' ? (x.by === x.to ? wsT("js.tk.feed.cc.status.own", by, esc(stLabel(x.status))) : wsT("js.tk.feed.cc.status", by, to, esc(stLabel(x.status)))) : wsT(CC_KEY[x.type], by, to);
    return txt + (p.note ? wsT("js.tk.ev.tail", snippet(p.note)) : '');
  }
  function feedEntries(v) {
    var out = [], t = today();
    function add(key, kind, item, ts, html, ev) {
      if (VS.items[item] && !VS.items[item].hidden) out.push({ key: key, kind: kind, item: item, ts: String(ts || ''), html: html, ev: ev || null });
    }
    VS.my.assignments.map(A).forEach(function (r) {
      var it = VS.items[r.item];
      if (it.kind === 'topic') return; // a held topic carries no work to start or finish: the store's "a:" badge leaves it out too
      // new forced assignments: the same rule as the handle badge's "a:" keys
      if (r.by !== v && r.forced && r.status === 'todo') add('a:' + r.id, 'assigned', r.item, r.ts, r.inherited && r.ruleLevels != null ? wsT("js.tk.feed.assigned.all", esc(nm(r.by)), esc(r.ruleLevels)) : wsT("js.tk.feed.assigned", esc(nm(r.by))), r.via);
      // near the due date, as the due chip computes it
      if (it.due && it.due <= t && !wrapped(r.status) && !wrapped(it.status))
        add('d:' + r.id + ':' + (it.due < t ? 'over' : 'today'), 'due', r.item, it.due + 'T00:00:00Z', it.due < t ? wsT("js.tk.feed.overdue", esc(it.due)) : wsT("js.tk.due.today"));
    });
    VS.my.received.pending.map(A).forEach(function (r) {
      add('p:' + r.id, 'pending', r.item, r.ts, r.handoffOf ? wsT("js.tk.feed.pending.handoff", esc(nm(r.sender))) : wsT("js.tk.feed.pending", esc(nm(r.sender))), r.via);
    });
    // take requests I may answer: the handle badge's "q:" keys
    listOf((VS.my.requests || {}).incoming).forEach(function (id) {
      var q = (VS.requests || {})[id];
      if (q) add('q:' + id, 'request', q.item, q.ts, (q.assignee === v ? wsT("js.tk.feed.request", esc(nm(q.by))) : wsT("js.tk.feed.request.of", esc(nm(q.by)), esc(nm(q.assignee)))) + (q.note ? wsT("js.tk.ev.tail", snippet(q.note)) : ''), id);
    });
    // the model's derived notices; done / blocked / dropped: work I assigned, set by someone else
    // (the assignee, or an org superior of theirs)
    VS.notices.forEach(function (x) {
      var r = A(x.assignment), txt;
      if (x.kind === 'done' || x.kind === 'blocked' || x.kind === 'dropped') {
        txt = x.kind === 'done' ? wsT("js.tk.feed.finished", esc(nm(x.to))) : x.kind === 'blocked' ? wsT("js.tk.feed.blocked", esc(nm(x.to))) : wsT("js.tk.feed.dropped", esc(nm(x.to)), esc(nm(r.statusBy || x.to)));
        add(x.key, x.kind, x.item, x.ts, txt + (r.note ? wsT("js.tk.ev.tail", snippet(r.note)) : ''), statusEv(r));
        return;
      }
      if (x.kind === 'declined') txt = wsT("js.tk.feed.declined", esc(nm(x.to)));
      else if (x.kind === 'returned') txt = r.handoffOf ? wsT("js.tk.feed.returned.handoff", esc(nm(x.endedBy)), esc(nm(x.to))) : wsT("js.tk.feed.returned", esc(nm(x.endedBy)), esc(nm(x.to)));
      else if (x.kind === 'accepted') txt = r.handoffOf ? wsT("js.tk.feed.accepted.handoff", esc(nm(x.to))) : wsT("js.tk.feed.accepted", esc(nm(x.to)), esc(nm(x.by)));
      else if (x.kind === 'handoff') txt = wsT("js.tk.feed.tookover", esc(nm(x.to)), esc(nm(x.from)));
      else return;
      add(x.key, x.kind === 'declined' || x.kind === 'returned' ? 'answer' : 'reference', x.item, x.ts, txt + (x.note ? wsT("js.tk.ev.tail", snippet(x.note)) : ''), (x.kind === 'declined' || x.kind === 'returned' ? r.ended : r.accepted).id);
    });
    VS.my.today.confirms.map(A).forEach(function (r) {
      add('c:' + r.id + ':' + r.lastUpdate, 'confirm', r.item, r.lastUpdate, wsT("js.tk.feed.confirm", esc(nm(r.to))), statusEv(r));
    });
    vals(VS.comments).forEach(function (c) {
      if (c.hidden || c.author === v) return;
      var q = c.reply_to && VS.comments[c.reply_to];
      if (q && q.author === v) add('r:' + c.id, 'reply', c.item, c.ts, wsT("js.tk.feed.reply", esc(nm(c.author)), snippet(c.body)), c.id);
      var ts = c.edited ? c.edited.ts : c.ts;
      if (c.mentions.indexOf(v) >= 0) add('m:' + c.id + ':' + ts, 'mention', c.item, ts, wsT("js.tk.feed.mention", esc(nm(c.author)), snippet(c.body)), c.id);
    });
    vals(VS.items).forEach(function (it) {
      var ts = it.edited ? it.edited.ts : it.ts;
      if (it.author !== v && it.mentions.indexOf(v) >= 0) add('m:' + it.id + ':' + ts, 'mention', it.id, ts, wsT("js.tk.feed.mention.body", esc(nm(it.author))), it.edited ? it.edited.id : it.id);
    });
    // CC notices: the view already leaves out what notifies on its own; they are not in the badge
    listOf(VS.my.ccNotices).forEach(function (x) {
      var txt = ccNoticeTxt(x);
      if (txt) add(x.key, 'cc', x.item, x.ts, txt, x.event);
    });
    return out.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : a.key < b.key ? -1 : 1; });
  }
  function readMap() { return seenOf(ui.store).read; }
  function markRead(keys) {
    var rm = readMap();
    keys = keys.filter(function (k) { return !rm[k]; });
    if (keys.length) AD.markSeen(ui.store, { badge: [], read: keys });
  }
  function cardNotify(v) {
    var list = feedEntries(v), read = readMap(), unread = list.filter(function (x) { return !read[x.key]; }).length;
    var h = list.length ? list.map(function (x) {
      return '<div class="row click nfeed' + (read[x.key] ? '' : ' unread') + '"' + attr('item', x.item) + attr('nkey', x.key) + attr('nkind', x.kind) + (x.ev ? attr('nev', x.ev) : '') + '><div class="r1">' +
        '<span class="chip nk">' + esc(wsT(NK[x.kind])) + '</span>' + isNew(x.key) + titleRef(x.item) + '<span class="grow"></span><span class="muted ts">' + esc(fmtTs(x.ts)) + '</span></div>' +
        '<div class="r2">' + x.html + '</div></div>';
    }).join('') : '<p class="muted">' + wsT("js.tk.feed.empty") + '</p>';
    return card(wsT("js.tk.tab.notify"), wsT("js.tk.feed.chip", unread, list.length), h, unread ? btn('nread-all', wsT("js.tk.feed.readall"), {}) : '', 'card-notify');
  }

  // Mention chips and attachment chips. Loading is simulated (caps.attach): a real
  // client fetches the blob on demand.
  function fmtSize(n) { return n >= 1048576 ? wsT("js.tk.size.mib", (n / 1048576).toFixed(1)) : n >= 1024 ? wsT("js.tk.size.kib", Math.round(n / 1024)) : wsT("js.tk.size.b", n); }
  function mentionChips(ids) {
    ids = listOf(ids).filter(function (x) { return typeof x === 'string'; });
    return ids.length ? '<div class="mentions">' + ids.map(function (id) {
      return '<span class="chip mention" title="' + wsT("js.tk.att.mentions", esc(id)) + '">@' + esc(VS.members[id] ? VS.members[id].name : id) + '</span>';
    }).join('') + '</div>' : '';
  }
  function attChips(list, live) {
    list = listOf(list).filter(function (x) { return x && typeof x === 'object'; });
    if (!list.length) return '';
    var got = ui.loaded[seenKey(ui.store)] || {};
    return '<div class="atts">' + list.map(function (x) {
      var label = wsT("js.tk.att.file", esc(x.name), Number.isInteger(x.size) ? fmtSize(x.size) : '?');
      var meta = wsT("js.tk.att.meta", x.mime, x.size, x.oid);
      if (!live) return '<span class="chip att rejected" title="' + wsT("js.tk.att.rejected") + '">' + label + '</span>';
      if (!CAPS.attach) return '<span class="chip att fixed" title="' + esc(meta) + '">' + label + '</span>';
      var on = !!got[x.oid];
      return '<button type="button" class="chip att' + (on ? ' loaded' : '') + '"' + attr('act', 'att-load') + attr('oid', x.oid) +
        ' title="' + esc(on ? meta : wsT("js.tk.att.fetch", meta)) + '">' + label + ' &middot; ' + (on ? wsT("js.tk.att.loaded") : wsT("js.tk.att.notloaded")) + '</button>';
    }).join('') + '</div>';
  }

  // ------------------------------------------------------------- Items panel
 
  // One panel replaces My assignments, Received requests, Assigned by me, My team,
  // and Items: scope presets (SCOPES) crossed with the view modes each scope offers
  // (SCOPE_MODES). It opens on "mine"; the chosen scope and each scope's mode are
  // remembered per store in memory, like the side cards' modes, and never written.

  // The modes per scope and the "recently closed" fold are display state: they start from the stored values.
  // today and drill (a team head's count) are display state too, never stored.
  function panelState() {
    var n = ui.store;
    return (ui.panel[n] = ui.panel[n] || { scope: 'mine', modes: (ui.pmodes[n] = ui.pmodes[n] || {}), today: null, drill: null, q: '', closedOpen: ui.closedOpen[n] !== false, treeOpen: {} });
  }
  function scopeList() { return SCOPES.filter(function (x) { return x[0] !== 'team' || VS.my.team.length; }); }
  function scopeOf() { var s = panelState().scope; return scopeList().some(function (x) { return x[0] === s; }) ? s : 'mine'; }
  function panelMode(s) { var m = panelState().modes[s]; return m && SCOPE_MODES[s].some(function (x) { return x[0] === m; }) ? m : 'list'; }

  // Today as summary chips in the Items panel header (the Today card is gone):
  // my open work / waiting for my answer / waiting for my confirm, always visible
  // whatever scope is shown. A chip narrows the panel to its rows, as a filter on
  // the current scope and view mode; the same chip again clears it. When the current
  // scope shows none of them, the click moves the panel to the scope that holds them
  // (TODAY_HOME), so a chip never opens on an empty panel.
  // [key, chip name key, tooltip key, the tooltip's lead phrase key (the filter's name in sentences)]
  var TODAY = [['work', "js.tk.today.work", "js.tk.today.work.tip", "js.tk.today.work.what"],
    ['pending', "js.tk.today.pending", "js.tk.today.pending.tip", "js.tk.today.pending.what"],
    ['confirms', "js.tk.today.confirms", "js.tk.today.confirms.tip", "js.tk.today.confirms.what"]];
  var TODAY_HOME = { work: 'mine', pending: 'received', confirms: 'given' };
  var TF = null; // per render: the active Today filter { key, recs: {assignment id}, items: {item id} }, or null
  function todayFilter() {
    var k = panelState().today;
    if (!k || !VS.my.today[k]) return null;
    var f = { key: k, recs: {}, items: {} };
    VS.my.today[k].forEach(function (id) { f.recs[id] = true; f.items[A(id).item] = true; });
    return f;
  }
  // The search box (row 2 of the panel header) filters the current scope and combines with
  // the Today filter. With adapter.search the query is the tracker's search grammar, which
  // the adapter's host parses and answers with the matching ids (SIDS): the request goes out
  // SEARCH_DEBOUNCE ms after the last keystroke, and again at once when the store's view
  // changes (a write or a refresh can change what matches), so the ids never go stale. Until
  // the first answer the list stays unfiltered, and while a newer request runs it keeps the
  // last answer. A malformed query leaves the list unfiltered and shows the host's message
  // under the box (SERR). Without adapter.search the box matches a plain substring (the
  // mockup). SQ is the lower-cased query of the render in progress, '' for none or while no
  // answer filters the list.
  var SQ = '', SIDS = null, SERR = '', FILT = false; // per render: the query; the matching ids, null when not filtering by ids; the malformed-query message; whether the Today filter or a search is on
  var SEARCH_DEBOUNCE = 250;
  function matchItem(it) {
    if (!SQ) return true;
    if (SIDS) return !!SIDS[it.id];
    // the plain-substring fallback still honours one label:<name> token, the query a label chip sets
    var lq = /^label:"?([^"]*)"?$/.exec(SQ);
    if (lq) return (it.labels || []).some(function (x) { var l = labelOf(x); return !!l && l.name.toLowerCase() === lq[1]; });
    if (SQ === 'is:claimable') return !!it.claimable; // and the is:claimable token, the query of the search grammar
    return [it.title, it.body, it.project, it.id, it.alias].concat((it.labels || []).map(labelName)).some(function (x) { return String(x == null ? '' : x).toLowerCase().indexOf(SQ) >= 0; });
  }
  // A label chip's click: the Items search becomes label:"<name>", set whole (no debounce).
  function labelSearch(id) {
    var l = labelOf(id);
    if (!l) return;
    if (ui.win) closeItemWindow();
    panelState().q = 'label:"' + l.name + '"';
    searchState(ui.store).now = true;
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    ui.sel = null;
    render();
  }
  // An "open" chip's click: the Items panel shows every item, filtered to the claimable ones.
  function claimableFilter() {
    if (ui.win) closeItemWindow();
    panelState().scope = 'all'; ui.kind = 'claimable';
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    ui.sel = null;
    render();
  }
  // a store's search state: want / wantView, the query and view object the last request was
  // made for; seq, the request counter (an answer to an older request is dropped); q, ids, err,
  // the last answer; timer, the pending debounced request; now, the next changed query was set
  // whole (a support chat's search command), not typed, so it skips the debounce
  function searchState(n) { return (ui.psr[n] = ui.psr[n] || { seq: 0, want: null, wantView: null, q: null, ids: null, err: '', timer: null, now: false }); }
  // Called by each render of the panel with the trimmed query: (re)sends the request when the
  // query or the view object changed since the last one, debounced when the query was typed.
  function searchFor(n, q) {
    var st = searchState(n), now = st.now;
    st.now = false;
    if (!q) { clearTimeout(st.timer); ui.psr[n] = null; return; }
    if (st.want === q && st.wantView === VS) return;
    var typed = st.want !== q && !now;
    st.want = q; st.wantView = VS;
    clearTimeout(st.timer);
    st.timer = setTimeout(function () {
      st.timer = null;
      var seq = ++st.seq;
      AD.search(n, q, function (res) {
        if (ui.psr[n] !== st || seq !== st.seq) return;
        st.q = q;
        if (res && res.ok) { st.err = ''; st.ids = {}; (res.ids || []).forEach(function (id) { st.ids[id] = true; }); }
        else { st.err = String((res && res.error) || ''); st.ids = null; }
        if (ui.store === n) render();
      });
    }, typed ? SEARCH_DEBOUNCE : 0);
  }
  // a row with an assignment record passes on the record; an item row passes on the item
  function keepRec(r) { return (!TF || !!TF.recs[r.id]) && matchItem(VS.items[r.item]); }
  function keepItem(id) { return (!TF || !!TF.items[id]) && matchItem(VS.items[id]); }
  function todayBar() {
    var t = VS.my.today, on = panelState().today;
    return '<div class="todaybar" role="group" aria-label="' + wsT("js.tk.today") + '"><span class="tl">' + wsT("js.tk.today") + '</span>' + TODAY.map(function (x, i) {
      var n = t[x[0]].length;
      return (i ? '<span class="tsep">&middot;</span>' : '') + '<button type="button" class="tch' + (on === x[0] ? ' on' : '') + (n ? '' : ' zero') + '" aria-pressed="' + (on === x[0]) + '"' +
        attr('act', 'today') + attr('set', x[0]) + tipAttr(on === x[0] ? wsT("js.tk.today.tip.on", wsT(x[2]), n) : wsT("js.tk.today.tip.off", wsT(x[2]), n)) + '>' + esc(wsT(x[1])) + ' <b>' + n + '</b></button>';
    }).join('') + (on ? '<button type="button" class="tch tclr"' + attr('act', 'today') + attr('set', on) + ' aria-label="' + wsT("js.tk.today.clear") + '"' + tipAttr(wsT("js.tk.today.clear")) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') + '</div>';
  }
  function filterEmpty() {
    var x = TF && TODAY.filter(function (y) { return y[0] === TF.key; })[0], what = x ? esc(wsT(x[3])) : '', q = esc(panelState().q.trim());
    return '<div class="empty">' + (SQ && TF ? wsT("js.tk.panel.empty.both", q, what) : SQ ? wsT("js.tk.panel.empty.search", q) : wsT("js.tk.panel.empty.today", what)) + ' ' +
      (SQ ? btn('psearch-clear', wsT("js.tk.panel.clearsearch"), {}) + ' ' : '') + (TF ? btn('today', wsT("js.tk.panel.clearfilter"), { set: TF.key }) : '') + '</div>';
  }

  // Recently closed: done items leave the main list for a section at the bottom of the
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
    var t = it.assignments.map(A).filter(function (r) { return r.state === 'live' && wrapped(r.status); }).map(function (r) { return String(r.lastUpdate); }).sort().pop();
    return t || String(it.lastActivity);
  }
  function addClosed(ts, n, html) {
    if (!CLOSED) return;
    if (SQ || Date.parse(VS.now) - Date.parse(ts) <= CLOSED_DAYS * 86400000) CLOSED.entries.push({ ts: ts, n: n, html: html });
    else CLOSED.older += n;
  }
  function closedHtml() {
    if (!CLOSED || (!CLOSED.entries.length && !CLOSED.older)) return '';
    var on = panelState().closedOpen, n = CLOSED.entries.reduce(function (a, x) { return a + x.n; }, 0);
    var rows = CLOSED.entries.sort(function (a, b) { return a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : 0; }).map(function (x) { return x.html; }).join('');
    return '<section class="pclosed' + (on ? ' open' : '') + '"><button type="button" class="pc-head" aria-expanded="' + on + '"' + attr('act', 'closed-toggle') + attr('fk', 'closed') + '><svg class="i"><use href="#tki-chev"/></svg>' +
      (SQ ? wsT("js.tk.closed") : wsT("js.tk.closed.recent")) + ' <span class="subn">' + n + '</span><span class="muted">' + (SQ ? wsT("js.tk.closed.allages") : CLOSED.older ? wsT("js.tk.closed.lastolder", CLOSED_DAYS, CLOSED.older) : wsT("js.tk.closed.last", CLOSED_DAYS)) + '</span></button>' +
      (on ? '<div class="pc-rows">' + (rows || '<p class="muted">' + wsT("js.tk.closed.none") + '</p>') + '</div>' : '') + '</section>';
  }

  // Context trees: every scope list (and each sub-section of one) draws its matching rows as
  // a tree. Each match's ancestors stand above it as greyed context rows (not matches: the
  // scope counts leave them out, a click opens the item); a context node's children that are
  // in no match's chain fold into one "… N more" row; a match's own children show folded
  // ("N subtasks"). The tree is the matches plus their ancestors (the spine). A node is in one
  // of three fold states, kept per device in browser storage by store + item id (ui.tfold):
  //   default  the spine children show; the others are the "… N more" row, or, when no
  //            spine child shows, the folded "N subtasks" chip;
  //   'o'      every child shows;
  //   'f'      no child shows.
  // The "all" tree passes opt.rest, which draws a node's non-spine children (its closed units)
  // as the "closed N" row instead; there a node toggles between the default and 'f' only.
  // The kids of a node are its non-hidden children (opt.kids replaces that), in item order.
  // A match's chain of ancestors stops below the first hidden one, or the first opt.keep(id) refuses
  // (the "all" scope's kind filter), so neither shows as a context row; the match then roots there.
  function kidsShown(id) { var it = VS.items[id]; return it ? it.children.filter(function (k) { return VS.items[k] && !VS.items[k].hidden; }) : []; }
  function foldOf(id) { return ui.tfold[ui.store + '|' + id] || ''; }
  // rows: the scope's matches in its own order, each an irow option object ({ rk, item, rec?, more?, closed? }).
  // Returns the entries in display order: { k: 'node', id, depth, rows (the node's matches; none for a context
  // row), n (children it can show), open (they show: the fold control reads ▾), to (the fold state a click on
  // the control sets) }, { k: 'more', id, depth, n } (the "… N more" row), { k: 'html', html } (opt.rest's).
  // Roots are ordered by their first match in `rows`, children by item order.
  function ctxTree(rows, opt) {
    opt = opt || {};
    var kidsOf = opt.kids || kidsShown, byItem = {}, rank = {}, pos = {}, spine = {}, out = [];
    VS.itemOrder.forEach(function (id, i) { pos[id] = i; });
    rows.forEach(function (r, i) {
      (byItem[r.item] = byItem[r.item] || []).push(r);
      for (var cur = r.item, n = 0; cur && VS.items[cur] && n < 50; cur = VS.items[cur].parent, n++) {
        if (cur !== r.item && (VS.items[cur].hidden || (opt.keep && !opt.keep(cur)))) break;
        if (!(cur in rank)) rank[cur] = i;
      }
    });
    function inItemOrder(a, b) { return (pos[a] || 0) - (pos[b] || 0); }
    Object.keys(rank).forEach(function (id) { var p = VS.items[id].parent; if (p && p in rank) (spine[p] = spine[p] || []).push(id); });
    function walk(id, depth) {
      var S = (spine[id] || []).sort(inItemOrder), U = kidsOf(id).filter(function (k) { return !(k in rank); }), f = foldOf(id);
      var n = S.length + U.length, open = n > 0 && f !== 'f' && (f === 'o' || !!S.length || !!opt.rest);
      out.push({ k: 'node', id: id, depth: depth, rows: byItem[id] || [], n: n, open: open, to: open ? 'f' : opt.rest ? '' : 'o' });
      if (!open) return;
      (f === 'o' && !opt.rest ? S.concat(U).sort(inItemOrder) : S).forEach(function (k) { walk(k, depth + 1); });
      if (U.length && (opt.rest || f !== 'o')) out.push(opt.rest ? { k: 'html', html: opt.rest(id, U, depth + 1) } : { k: 'more', id: id, depth: depth + 1, n: U.length });
    }
    Object.keys(rank).filter(function (id) { var p = VS.items[id].parent; return !(p && p in rank); })
      .sort(function (a, b) { return rank[a] - rank[b] || inItemOrder(a, b); }).forEach(function (id) { walk(id, 0); });
    return out;
  }
  // A node's fold control (`tw`, left of the status icon; a spacer on a node without children) and, while
  // folded, its "N subtasks" chip after the title; both carry the same toggle. act: the data-act
  // (tfold in the lists and the timeline, wfold in the item window); fk: the focus key prefix.
  function foldCtl(e, act, fk) {
    if (!e.n) return { tw: '<span class="tw"></span>', sub: '' };
    var t = VS.items[e.id].title;
    function b(cls, inner, tab) {
      return '<button type="button" class="' + cls + '"' + attr('act', act) + attr('node', e.id) + attr('to', e.to) + attr('fk', cls + ':' + fk + e.id) +
        ' aria-expanded="' + e.open + '" aria-label="' + esc(wsT(e.open ? "js.tk.tree.fold" : "js.tk.tree.unfold", t)) + '"' + tipAttr(wsT(e.open ? "js.tk.tree.fold" : "js.tk.tree.unfold", t)) + (tab ? '' : ' tabindex="-1"') + '>' + inner + '</button>';
    }
    return { tw: b('tw', '<svg class="i"><use href="#tki-chev"/></svg>', true), sub: e.open ? '' : b('tsub', esc(wsTn("js.tk.tree.subtasks", e.n))) };
  }
  function moreRow(e, fk) {
    return '<button type="button" class="tmore" style="--ind:' + e.depth + '"' + attr('act', 'tfold') + attr('node', e.id) + attr('to', 'o') + attr('fk', 'tm:' + fk + e.id) +
      tipAttr(wsT("js.tk.tree.more.tip", e.n, VS.items[e.id].title)) + '>' + esc(wsT("js.tk.tree.more", e.n)) + '</button>';
  }
  // A context tree as item rows. pre: the section's row-key prefix (context rows get pre + 'ctx:' + id).
  // A tree with no nesting at all draws no fold column, so a flat list keeps its plain rows.
  function treeHtml(ents, pre) {
    var flat = ents.every(function (e) { return e.k === 'node' && !e.n && !e.depth; });
    return ents.map(function (e) {
      if (e.k === 'html') return e.html;
      if (e.k === 'more') return moreRow(e, pre);
      var c = flat ? { tw: null, sub: '' } : foldCtl(e, 'tfold', pre);
      if (!e.rows.length) return irow({ rk: pre + 'ctx:' + e.id, item: e.id, depth: e.depth, ctx: true, tw: c.tw, tsub: c.sub, more: wsT("js.tk.tree.ctx") });
      return e.rows.map(function (r, i) {
        return irow({ rk: r.rk, item: r.item, rec: r.rec, more: r.more, closed: r.closed, depth: e.depth, tw: flat ? null : i ? '<span class="tw"></span>' : c.tw, tsub: i ? '' : c.sub });
      }).join('');
    }).join('');
  }
  function treeRows(rows, pre, opt) { return treeHtml(ctxTree(rows, opt), pre); }
  function keepKind(id) { return kindFilter(VS.items[id]); }

  function listMine() {
    var mine = VS.my.assignments.map(A).filter(keepRec), open = mine.filter(function (r) { return !wrapped(r.status); }), done = mine.filter(function (r) { return wrapped(r.status); });
    var created = VS.my.created.filter(keepItem);
    done.forEach(function (r) { addClosed(String(r.lastUpdate), 1, irow({ rk: 'mine:' + r.id, item: r.item, rec: r, closed: true })); });
    if (FILT && !mine.length && !created.length) return filterEmpty();
    var h = open.length ? treeRows(open.map(function (r) { return { rk: 'mine:' + r.id, item: r.item, rec: r }; }), 'mine:') : TF ? '' : SQ ? (created.length ? '' : '<p class="muted">' + wsT("js.tk.list.nomatch") + '</p>') : '<p class="muted">' + wsT("js.tk.list.mine.none") + '</p>';
    if (created.length) h += sub(wsT("js.tk.list.mine.created"), created.length) + treeRows(created.map(function (id) { return { rk: 'mine:c:' + id, item: id }; }), 'mine:c:');
    return h;
  }
  function answerTxt(x) {
    var r = A(x.assignment);
    return (x.kind === 'declined' ? wsT("js.tk.feed.declined", esc(nm(x.to))) :
      (r.handoffOf ? wsT("js.tk.feed.returned.handoff", esc(nm(x.endedBy)), esc(nm(x.to))) : wsT("js.tk.feed.returned", esc(nm(x.endedBy)), esc(nm(x.to)))) +
      (x.restores ? wsT("js.tk.list.restored", esc(nm(x.from))) : r.handoffOf ? wsT("js.tk.list.nobodyholds") : '')) +
      (x.note ? wsT("js.tk.ev.tail", '&ldquo;' + esc(x.note) + '&rdquo;') : '') + ', ' + esc(fmtTs(x.ts));
  }
  function listReceived() {
    var R = VS.my.received, h = '';
    var ans = VS.notices.filter(function (x) { return (x.kind === 'declined' || x.kind === 'returned') && keepRec(A(x.assignment)); });
    function only(ids) { return ids.filter(function (id) { return keepRec(A(id)); }); }
    var pend = only(R.pending), answered = only(R.answered), sent = only(R.sent);
    if (FILT && !ans.length && !pend.length && !answered.length && !sent.length) return filterEmpty();
    // each sub-section draws its own tree
    function rows(ids, pre, more) { return treeRows(ids.map(function (id) { var r = A(id); return { rk: pre + id, item: r.item, rec: r, more: more && more(r) }; }), pre); }
    if (ans.length) h += sub(wsT("js.tk.list.rcv.answers"), ans.length) + treeRows(ans.map(function (x) { return { rk: 'rcv:a:' + x.key, item: x.item, rec: A(x.assignment), more: answerTxt(x) }; }), 'rcv:a:');
    if (pend.length || !FILT) h += sub(wsT("js.tk.today.pending.what"), pend.length) + (pend.length ? rows(pend, 'rcv:p:') : '<p class="muted">' + wsT("js.tk.list.rcv.none") + '</p>');
    if (answered.length) h += sub(wsT("js.tk.list.rcv.answered"), answered.length) + rows(answered, 'rcv:d:');
    if (sent.length) h += sub(wsT("js.tk.list.rcv.sent"), sent.length) + rows(sent, 'rcv:s:', function (r) { return wsT("js.tk.list.rcv.sentto", esc(nm(r.to))); });
    return h;
  }
  // Related to me: the view's related lists (my.related), the CC section above the mentions
  // section, each in the view's order and foldable (relSecHtml); the done items either section
  // would hold (related.closed) go to the recently-closed section like any scope's closed rows.
  function listRelated() {
    var R = VS.my.related || {}, cc = listOf(R.cc).filter(keepItem), mn = listOf(R.mentioned).filter(keepItem);
    listOf(R.closed).filter(keepItem).forEach(function (id) { addClosed(closeTs(VS.items[id]), 1, irow({ rk: 'rel:x:' + id, item: id, closed: true })); });
    if (FILT && !cc.length && !mn.length) return filterEmpty();
    return relSecHtml('cc', wsT("js.tk.list.rel.cc"), cc, wsT("js.tk.list.rel.cc.none")) + relSecHtml('mentioned', wsT("js.tk.list.rel.mentioned"), mn, wsT("js.tk.list.rel.mentioned.none"));
  }
  // One related section: a fold header with its count, open unless folded (panelState().relFold,
  // display state, never stored), then its rows as a context tree. Under a filter an empty section is left out.
  function relSecHtml(key, label, ids, none) {
    if (FILT && !ids.length) return '';
    var rf = panelState().relFold = panelState().relFold || {}, on = !rf[key], pre = 'rel:' + key + ':';
    return '<section class="rsec' + (on ? ' open' : '') + '"' + attr('rsec', key) + '><button type="button" class="rs-head" aria-expanded="' + on + '"' + attr('act', 'rel-fold') + attr('set', key) + attr('fk', 'rsec:' + key) + '>' +
      '<svg class="i"><use href="#tki-chev"/></svg>' + label + ' <span class="subn">' + ids.length + '</span></button>' +
      (on ? (ids.length ? treeRows(ids.map(function (id) { return { rk: pre + id, item: id }; }), pre) : '<p class="muted">' + none + '</p>') : '') + '</section>';
  }
  function listGiven() {
    var v = VS.viewer.member, given = VS.my.given.filter(keepItem), open = [];
    if (!VS.my.given.length) return '<div class="empty">' + wsT("js.tk.list.given.never") + '</div>';
    if (!given.length) return filterEmpty();
    given.forEach(function (id) {
      var lines = VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); }).sort(byRank).map(function (r) {
        return '<b>' + esc(nm(r.to)) + '</b>' + (r.depth ? ' <span class="muted">L' + r.depth + '</span>' : '') + ' ' + esc(SI.info(stKey(VS.items[id], r)).label) + (r.forced ? '' : ' <span class="muted">' + (r.handoffOf ? wsT("js.tk.list.given.handoff") : wsT("js.tk.list.given.sent")) + '</span>') +
          (r.note ? ' &ldquo;' + esc(r.note) + '&rdquo;' : '');
      });
      if (wrapped(VS.items[id].status)) return addClosed(closeTs(VS.items[id]), 1, irow({ rk: 'given:' + id, item: id, more: lines.join('<br>'), closed: true }));
      open.push({ rk: 'given:' + id, item: id, more: lines.join('<br>') });
    });
    return open.length ? treeRows(open, 'given:') : '<p class="muted">' + wsT("js.tk.list.given.none") + '</p>';
  }
  // My team as a group-by-person view: each group header carries that person's roll-up counts:
  // the load (live work at todo, doing, or blocked, a creator-fallback holding left out) with its
  // per-status breakdown, stale and overdue (both inside the load), in review, idea, fallback-held,
  // done, dropped, unaccepted, and waiting on others. The viewer's subtree comes first; an admin's members
  // outside it follow under their own heading, indented by depth from the org-tree root.
  // Under a Today filter or a search a group with no matching row is left out.
  // A count is a button: it narrows the list to that person's rows of that count (the drill,
  // panelState().drill = { who, key }, list mode only), and the same count again clears it.
  // The statuses a team row's load counts, in its breakdown's order (the view's roll-up counts
  // the same ones); every other status but idea, done, and dropped (in-review) shows as its own
  // count after stale and overdue, sorted.
  var TEAM_LOAD = ['todo', 'doing', 'blocked'];
  // The records a team head's count stands for, by the rules the view's counts use, so a count
  // and its rows agree. key: a status, 'load', 'stale', 'overdue', 'fallback', 'unaccepted', or
  // 'waiting'; recs: the assignment records by id; items: the items by id; day: today's date.
  function teamDrillRecs(p, key, recs, items, day) {
    if (key === 'unaccepted' || key === 'waiting') return (key === 'unaccepted' ? p.pending : p.waiting).map(function (id) { return recs[id]; }).filter(Boolean);
    return Object.keys(recs).map(function (id) { return recs[id]; }).filter(function (r) {
      var it = items[r.item];
      if (r.to !== p.id || r.state !== 'live' || !it || it.hidden || it.kind === 'topic') return false;
      if (key === 'fallback') return !!r.fallback && !wrapped(r.status);
      if (r.fallback) return false;
      if (key !== 'load' && key !== 'stale' && key !== 'overdue') return r.status === key;
      if (TEAM_LOAD.indexOf(r.status) < 0) return false;
      return key === 'load' || (key === 'stale' ? !!r.stale : !!(day && it.due && it.due < day));
    });
  }
  // The drill in effect: { p, key }, or null; a drill on a person no longer listed is dropped.
  function teamDrill() {
    var ps = panelState(), d = ps.drill, p = d && VS.my.team.filter(function (x) { return x.id === d.who; })[0];
    if (d && !p) ps.drill = null;
    return p ? { p: p, key: d.key } : null;
  }
  function teamCounts(p, on) {
    function cnt(key, txt, tip, err) {
      var sel = !!on && on.key === key;
      return '<button type="button" class="tcn' + (err ? ' err' : '') + (sel ? ' on' : '') + '" aria-pressed="' + sel + '"' + attr('act', 'tdrill') + attr('who', p.id) + attr('set', key) + attr('fk', 'td:' + p.id + ':' + key) +
        tipAttr((tip ? tip + '; ' : '') + wsT(sel ? "js.tk.team.drill.off" : "js.tk.team.drill.tip")) + '>' + txt + '</button>';
    }
    function st(s, n) { return cnt(s, wsT("js.tk.team.st", n, esc(SI.info(s).label)), '', s === 'blocked'); }
    var c = [], seen = { idea: true, done: true, dropped: true }, br = [], other = [];
    TEAM_LOAD.forEach(function (s) { if (p.counts[s]) br.push(st(s, p.counts[s])); seen[s] = true; });
    Object.keys(p.counts).sort().forEach(function (s) { if (!seen[s] && p.counts[s]) other.push(st(s, p.counts[s])); });
    if (p.load) c.push(cnt('load', wsT("js.tk.team.load", p.load), wsT("js.tk.team.load.tip")) + (br.length ? ' (' + br.join(' &middot; ') + ')' : ''));
    if (p.stale) c.push(cnt('stale', wsT("js.tk.team.stale", p.stale), wsT("js.tk.team.stale.tip", STALE_DAYS), true));
    if (p.overdue) c.push(cnt('overdue', wsT("js.tk.team.overdue", p.overdue), wsT("js.tk.team.overdue.tip"), true));
    c = c.concat(other);
    if (p.counts.idea) c.push(st('idea', p.counts.idea));
    if (p.fallback) c.push(cnt('fallback', wsT("js.tk.team.fallback", p.fallback), wsT("js.tk.team.fallback.tip")));
    if (p.counts.done) c.push(st('done', p.counts.done));
    if (p.counts.dropped) c.push(st('dropped', p.counts.dropped));
    if (p.pending.length) c.push(cnt('unaccepted', wsT("js.tk.team.unaccepted", p.pending.length), ''));
    if (p.waiting.length) c.push(cnt('waiting', wsT("js.tk.team.waiting", p.waiting.length), wsT("js.tk.team.waiting.tip")));
    return c.join(' &middot; ');
  }
  function listTeam() {
    var dr = teamDrill(), outside = false;
    var h = VS.my.team.map(function (p) {
      if (dr && dr.p !== p) return '';
      var m = VS.members[p.id], on = dr && dr.key, work, wait;
      if (dr) {
        var pos = {}, got = teamDrillRecs(p, on, VS.assignments, VS.items, today()).filter(keepRec);
        p.open.forEach(function (id, i) { pos[id] = i + 1; });
        got = got.map(function (r, i) { return { r: r, i: i }; }).sort(function (a, b) { return (pos[a.r.id] || 1e9) - (pos[b.r.id] || 1e9) || a.i - b.i; }).map(function (x) { return x.r.id; });
        work = on === 'waiting' ? [] : got; wait = on === 'waiting' ? got : [];
      } else {
        work = p.open.concat(p.pending).filter(function (id) { return keepRec(A(id)); }); wait = p.waiting.filter(function (id) { return keepRec(A(id)); });
      }
      if (FILT && !dr && !work.length && !wait.length) return ''; // a drilled group stays, with its clear control
      var g = '';
      if (p.outside && !outside) { outside = true; g += sub('<span' + tipAttr(wsT("js.tk.team.outside.tip")) + '>' + wsT("js.tk.team.outside") + '</span>', VS.my.team.filter(function (x) { return x.outside; }).length); }
      g += '<div class="tg' + (p.outside ? ' out' : '') + '" style="--d:' + (p.outside ? p.depth : p.depth - 1) + '"' + attr('person', p.id) + '><div class="tg-head">' + avHtml(p.id) + '<b>' + whoName(p.id) + '</b>' +
        '<span class="nid">' + esc(p.id) + (p.outside ? '' : ' L' + p.depth) + (m.children.length ? wsT("js.tk.team.islead") : '') + '</span><span class="cnt">' + teamCounts(p, dr) + '</span>' +
        (dr ? '<button type="button" class="ia"' + attr('act', 'tdrill') + attr('who', p.id) + attr('set', '') + attr('fk', 'td:x') + ' aria-label="' + esc(wsT("js.tk.team.drill.clear")) + '"' + tipAttr(wsT("js.tk.team.drill.clear")) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') +
        '<span class="grow"></span><span class="muted ts"' + tipAttr(wsT("js.tk.team.last.tip")) + '>' + esc(fmtTs(p.last)) + '</span></div>';
      if (dr && !work.length && !wait.length) return g + '<p class="muted">' + wsT("js.tk.team.drill.none") + '</p></div>';
      // one tree per person
      g += treeRows(work.map(function (id) { var r = A(id); return { rk: 'team:' + id, item: r.item, rec: r, closed: r.state === 'live' && wrapped(r.status) }; }).concat(wait.map(function (id) {
        var r = A(id);
        return { rk: 'team:w:' + id, item: r.item, rec: r, more: r.handoffOf ? wsT("js.tk.team.sent.handoff", esc(nm(p.id)), esc(nm(r.to))) : wsT("js.tk.team.sent", esc(nm(p.id)), esc(nm(r.to))) };
      })), 'team:' + p.id + ':');
      return g + '</div>';
    }).join('');
    return h || (FILT && VS.my.team.length ? filterEmpty() : '<div class="empty">' + wsT("js.tk.team.nobody") + '</div>');
  }
  function allRows(kanban) { return VS.itemOrder.map(function (id) { return VS.items[id]; }).filter(kanban ? function (it) { return !it.hidden && kindFilter(it); } : itemFilter); }
  // The tree and closed items. Under the default "open" status, a done item never
  // leaves its place silently: a closed child among open siblings folds into one "closed N"
  // row under its parent (a click expands it); a closed parent with an open descendant stays in
  // the tree, greyed; a closed parent whose whole subtree is closed moves, as one unit, to the
  // "recently closed" section (the parent's own roll-up, done 3/5, still counts the children).
  // While a search or the Today filter is on, the rows are the matches instead: the open and greyed
  // ones as a context tree, the closed ones in the section, every age. Another status filter draws
  // its matches as a context tree too.
  function listAll() {
    var bar = '<div class="toolbar">' + itemFilterBar() + '</div>';
    if (ui.status !== 'open') {
      var rows = allRows().filter(function (it) { return keepItem(it.id); });
      return bar + (rows.length ? treeRows(rows.map(function (it) { return { rk: 'all:' + it.id, item: it.id }; }), 'all:', { keep: keepKind }) : FILT ? filterEmpty() : '<div class="empty">' + wsT("js.tk.list.all.nomatch") + '</div>');
    }
    var base = {}, order = [], kids = {}, openBelow = {};
    VS.itemOrder.forEach(function (id) { var it = VS.items[id]; if (!it.hidden && kindFilter(it)) { base[id] = it; order.push(id); } });
    order.forEach(function (id) { var par = base[id].parent; if (par && base[par]) (kids[par] = kids[par] || []).push(id); });
    function kidsOf(id) { return kids[id] || []; }
    function isOpen(id) { return itemFilter(base[id]); }
    function isClosed(id) { return base[id].kind !== 'topic' && wrapped(base[id].status); }
    function hasOpen(id) { if (!(id in openBelow)) openBelow[id] = kidsOf(id).some(function (k) { return isOpen(k) || hasOpen(k); }); return openBelow[id]; }
    function inTree(id) { return isOpen(id) || (isClosed(id) && hasOpen(id)); }   // open, or closed with something open below
    function isUnit(id) { return isClosed(id) && !hasOpen(id); }                  // closed, and so is everything below
    function parentOf(id) { var par = base[id].parent; return par && base[par] ? par : null; }
    function size(id) { return 1 + kidsOf(id).reduce(function (a, k) { return a + size(k); }, 0); }
    function latest(id) { return kidsOf(id).reduce(function (a, k) { var t = latest(k); return t > a ? t : a; }, isClosed(id) ? closeTs(base[id]) : ''); }
    function unitRows(id, depth) {
      return irow({ rk: 'all:' + id, item: id, depth: depth, closed: true }) + kidsOf(id).map(function (k) { return unitRows(k, depth + 1); }).join('');
    }
    if (FILT) {
      var main = order.filter(function (id) { return inTree(id) && keepItem(id); }), shut = order.filter(function (id) { return isUnit(id) && keepItem(id); });
      shut.forEach(function (id) { addClosed(closeTs(base[id]), 1, irow({ rk: 'all:' + id, item: id, closed: true })); });
      return bar + (main.length ? treeRows(main.map(function (id) { return { rk: 'all:' + id, item: id, closed: isClosed(id) }; }), 'all:', { keep: keepKind }) : shut.length ? '<p class="muted">' + wsT("js.tk.list.nomatch") + '</p>' : filterEmpty());
    }
    // the tree's matches are the open items and the greyed closed parents; a node's closed units are its "closed N" row
    function closedRow(id, cu, depth) {
      var it = base[id], on = !!panelState().treeOpen[id], n = cu.reduce(function (a, k) { return a + size(k); }, 0), r = it.rollup;
      return '<button type="button" class="csum' + (on ? ' open' : '') + '" style="--ind:' + depth + '" aria-expanded="' + on + '"' + attr('act', 'tree-closed') + attr('parent', id) + attr('fk', 'csum:' + id) +
        tipAttr(wsT(on ? "js.tk.list.csum.fold" : "js.tk.list.csum.show", it.title + (r.total ? ' ' + wsT("js.tk.list.csum.done", r.counts.done || 0, r.total) : ''))) + '><svg class="i"><use href="#tki-chev"/></svg>' + wsT("js.tk.closed") + ' <span class="subn">' + n + '</span></button>' +
        (on ? cu.map(function (k) { return unitRows(k, depth); }).join('') : '');
    }
    var tree = order.filter(inTree).map(function (id) { return { rk: 'all:' + id, item: id, closed: isClosed(id) }; });
    order.forEach(function (id) {
      if (isUnit(id) && !(parentOf(id) && (isUnit(parentOf(id)) || inTree(parentOf(id))))) addClosed(latest(id), size(id), unitRows(id, 0));
    });
    return bar + (tree.length ? treeHtml(ctxTree(tree, { kids: function (id) { return base[id] ? kidsOf(id).filter(function (k) { return inTree(k) || isUnit(k); }) : []; }, rest: closedRow, keep: function (id) { return !!base[id]; } }), 'all:') :
      '<div class="empty">' + wsT("js.tk.list.all.none") + '</div>');
  }
  function scopeChip(s) {
    if (s === 'mine') return wsT("js.tk.team.open", VS.my.assignments.map(A).filter(function (r) { return !wrapped(r.status) && VS.items[r.item].kind !== 'topic'; }).length);
    if (s === 'related') { var rl = VS.my.related || {}; return wsTn("js.tk.scope.chip.items", listOf(rl.cc).length + listOf(rl.mentioned).length); }
    if (s === 'received') return wsT("js.tk.scope.chip.pending", VS.my.received.pending.length);
    if (s === 'given') return wsTn("js.tk.scope.chip.items", VS.my.given.length) + (VS.my.today.confirms.length ? ', ' + wsT("js.tk.scope.chip.confirm", VS.my.today.confirms.length) : '');
    if (s === 'team') {
      var t = { below: 0, outside: 0, load: 0, stale: 0, overdue: 0, pending: 0 };
      VS.my.team.forEach(function (p) { t[p.outside ? 'outside' : 'below']++; t.load += p.load; t.stale += p.stale; t.overdue += p.overdue; t.pending += p.pending.length; });
      return [t.below || !t.outside ? wsT("js.tk.scope.chip.below", t.below) : '', t.outside ? wsT("js.tk.scope.chip.outside", t.outside) : '', wsT("js.tk.team.load", t.load),
        t.stale ? wsT("js.tk.team.stale", t.stale) : '', t.overdue ? wsT("js.tk.team.overdue", t.overdue) : '', t.pending ? wsT("js.tk.team.unaccepted", t.pending) : ''].filter(Boolean).join(', ');
    }
    return wsT("js.tk.scope.chip.of", allRows(panelMode('all') === 'kanban').length, Object.keys(VS.items).length);
  }
  var LISTS = { mine: listMine, related: listRelated, received: listReceived, given: listGiven, team: listTeam, all: listAll };
  function panelBody(s, m, v) { return m === 'list' ? LISTS[s](v) : modeBody(s, m, v); }
  // the panel as a spec: the header is two rows: scopes and the scope chip,
  // then the view icons and the search box. It is the card header when the panel stands alone,
  // or the tool area under the tab bar in the narrow layout.
  var VIEW_ICON = { "js.tk.mode.list": 'list', "js.tk.mode.tree": 'tree', "js.tk.mode.kanban": 'kanban', "js.tk.mode.gantt": 'timeline', "js.tk.mode.calendar": 'calendar' };
  function cardPanel(v) {
    var s = scopeOf(), m = panelMode(s), q = panelState().q;
    var qt = q.trim();
    TF = todayFilter(); SQ = ''; SIDS = null; SERR = '';
    if (qt && !AD.search) SQ = qt.toLowerCase();
    else if (qt) {
      searchFor(ui.store, qt);
      var sr = searchState(ui.store);
      if (sr.err) { if (sr.q === qt) SERR = sr.err; } // a newer query's answer is pending: no stale message
      else if (sr.ids) { SQ = qt.toLowerCase(); SIDS = sr.ids; }
    } else searchFor(ui.store, '');
    FILT = !!(TF || SQ);
    CLOSED = m === 'list' ? { entries: [], older: 0 } : null;
    MSEEN = {};
    var scopes = '<span class="scopes" role="tablist" aria-label="' + wsT("js.tk.panel.scope") + '">' + scopeList().map(function (x) {
      return '<button type="button" class="scp' + (x[0] === s ? ' on' : '') + '" role="tab" aria-selected="' + (x[0] === s) + '"' + attr('act', 'scope') + attr('scope', x[0]) + tipAttr(wsT(x[2])) + '>' + esc(wsT(x[1])) + '</button>';
    }).join('') + '</span>';
    var modes = SCOPE_MODES[s].length > 1 ? '<span class="vmodes" role="group" aria-label="' + wsT("js.tk.panel.view") + '">' + SCOPE_MODES[s].map(function (x) {
      return '<button type="button" class="vbtn' + (m === x[0] ? ' on' : '') + '" aria-pressed="' + (m === x[0]) + '" aria-label="' + esc(wsT(x[1])) + '"' + attr('act', 'pmode') + attr('mode', x[0]) + tipAttr(wsT(x[1])) +
        '><svg class="i"><use href="#tki-' + VIEW_ICON[x[1]] + '"/></svg></button>';
    }).join('') + '</span>' : '';
    var search = '<span class="psearch"><svg class="i"><use href="#tki-search"/></svg><input id="psearch" type="text" role="searchbox" autocomplete="off" spellcheck="false" value="' + esc(q) + '" placeholder="' + wsT("js.tk.panel.search") + '" aria-label="' + wsT("js.tk.panel.search.aria") + '">' +
      (q ? '<button type="button" class="psx"' + attr('act', 'psearch-clear') + ' aria-label="' + wsT("js.tk.panel.clearsearch") + '"' + tipAttr(wsT("js.tk.panel.clearsearch.tip")) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') + '</span>';
    // a new item: the hover insert between list rows (insShow), or the "+" that ends the panel, which
    // opens the input row in its place (the keyboard path; an empty list keeps only this one)
    var n = ui.nrow[ui.store], body = panelBody(s, m, v) + (n && !n.at ? newRowHtml() : newBtn()), closed = closedHtml();
    var serr = SERR ? '<div class="psearch-err" role="alert">' + esc(SERR) + '</div>' : '';
    // the selection keeps what this draw showed; a badge pinned atop the list (sticky in the
    // scrolling body, over a strip of its own so it never sits on the first row) counts it and clears it
    var ms = mselOf();
    if (ms) Object.keys(ms.ids).forEach(function (id) { if (!MSEEN[id]) delete ms.ids[id]; });
    if (!mselOn()) mselClear();
    var nsel = mselIds().length, mbar = nsel ? '<div class="mselbar" role="status"><span class="chip mselc">' + esc(wsTn("js.tk.sel.n", nsel)) + '<button type="button" class="psx"' + attr('act', 'msel-clear') + ' aria-label="' + wsT("js.tk.sel.clear") + '"' +
      tipAttr(wsT("js.tk.sel.clear")) + '><svg class="i"><use href="#tki-x"/></svg></button></span></div>' : '';
    TF = null; SQ = ''; SIDS = null; SERR = ''; FILT = false; CLOSED = null; MSEEN = null;
    return { title: wsT("js.tk.tab.items"), tools: scopes + '<span class="chip">' + esc(scopeChip(s)) + '</span><div class="prow2">' + modes + search + '</div>' + serr, body: mbar + '<div class="plist">' + body + '</div>' + closed, id: 'card-items', cls: 'panel',
      attrs: attr('scope', s) + attr('view', m) + (nsel ? attr('msel', nsel) : '') };
  }
  // With the adapter's wiki, the title is the Items | Wiki switch (wkSwitch); the Today bar belongs to the items
  function panelHtml(p, withToday) {
    return '<section class="card panel" id="card-items"' + (p.attrs || '') + '><header>' + (AD.wiki ? wkSwitch() : '<h2>' + wsT("js.tk.tab.items") + '</h2>') + p.tools + '</header>' + (withToday && !p.bodyCls ? todayBar() : '') +
      '<div class="cbody' + (p.bodyCls ? ' ' + p.bodyCls : '') + '">' + p.body + '</div>' + ribbonHtml() + '</section>';
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

