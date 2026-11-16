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

  // ------------------------------------------------------------ the wiki
  // The store's wiki branches (adapter.wiki) in the Items panel under the header's Items |
  // Wiki switch, or as the Wiki tab of the narrow layout: a picker of the store's wikis, the
  // page tree from the page paths, the page as the daemon rendered it, the shared editor in
  // its place, and the refused-save view (both texts, the lines that overlap marked) when a
  // save could not merge. Everything is read on demand and kept per store in ui.wiki; a save
  // re-reads the list, the pages, and the page.
  var WK_NAME = /^[a-z0-9][a-z0-9-]*$/, WK_NAME_MAX = 64;
  function wk() {
    return (ui.wiki[ui.store] = ui.wiki[ui.store] || {
      mode: 'items',        // 'items' | 'wiki': what the Items panel shows (the wide layouts)
      name: null,           // the wiki shown
      list: null, listErr: null, listBusy: false, // the store's wikis [{name, tip, unpushed}], or why they could not be read
      pages: null, pagesErr: null, pagesBusy: false, // {paths, commit} of the wiki shown
      page: null,           // the page path shown
      doc: null, docErr: null, docBusy: false, // {path, kind, content, commit, html} of the page shown; kind 'html' is an HTML page (wkFrameSync)
      edit: null,           // { path, isNew, newWiki, base, text, orig, newPath, note } while the editor is open; orig is the text the edit started from
      conflict: null,       // { c: the adapter's conflict, mine: the refused text } while the refused-save view shows
      saving: false,
      creating: false, wname: '', wfirst: 'index', // the "New wiki" form
      q: '', tree: true, closed: {}, ntree: false, // search text, tree shown (wide), folded dirs "wiki:dir", narrow tree open
      pick: false, more: false, // the picker and the page menu
      flash: false, top: false, full: false // a page just opened flashes its head and starts at the top; the refused view shows the whole text
    });
  }
  function wkIc(id, cls) { return '<svg class="i' + (cls ? ' ' + cls : '') + '"><use href="#tki-' + id + '"/></svg>'; }
  function wkSha(c) { return String(c || '').slice(0, 7); }
  function wkSwitch() {
    var m = wk().mode;
    return '<span class="wk-sw" role="group" aria-label="' + esc(wsT("js.tk.wk.sw.aria")) + '">' +
      '<button type="button"' + attr('act', 'wk-mode') + attr('mode', 'items') + ' aria-pressed="' + (m === 'items') + '">' + wkIc('tasks') + esc(wsT("js.tk.tab.items")) + '</button>' +
      '<button type="button"' + attr('act', 'wk-mode') + attr('mode', 'wiki') + ' aria-pressed="' + (m === 'wiki') + '">' + wkIc('book') + esc(wsT("js.tk.wk.tab")) + '</button></span>';
  }
  // the wiki an editor's preview renders in: the one shown, once it exists
  function wkRenderName() { var st = wk(); return st.edit && !st.edit.newWiki && st.name ? st.name : undefined; }
  function wkHas(st, path) { return !!(st.pages && st.pages.paths.indexOf(wkCanon(path)) >= 0); }
  // A page path as the daemon answers it (CanonicalWikiPath): name.md is the Markdown page
  // name, so a wiki:<name>/x.md link and a typed x.md find page x; any other path is left as
  // it is, the daemon refusing what it does not accept (a.html.md).
  function wkCanon(path) {
    var p = String(path || ''), m = /^(.*?)([^/]+)\.md$/.exec(p);
    return m && !/\.(md|html)$/i.test(m[2]) ? m[1] + m[2] : p;
  }
  // An HTML page's path ends in exactly .html after a non-empty name; the extension decides the kind.
  function wkIsHtml(path) { return /[^/]\.html$/.test(String(path || '')); }
  // Reads land on the state object they started from, for the wiki (and page) still shown; a
  // read for one no longer shown clears its busy flag and reads what is shown now instead.
  // wkDone wraps a read's answer: it clears the busy flag of read k ('list' | 'pages' |
  // 'doc'), drops an answer for a state no longer kept, and runs read k once more when a
  // change signal arrived while it was in flight (wkReread marked it), since the answer
  // predates that change. A store no longer shown is not re-read: its re-read is a soft one,
  // whose 404 for a page not yet read would set docErr and keep wkPull from opening the new
  // page when the store shows again; what it is missing then is read by wkPull.
  function wkDone(st, store, k, fn) {
    return function (res) {
      st[k + 'Busy'] = false;
      if (ui.wiki[store] !== st) return;
      fn(res);
      if (st[k + 'Again']) { st[k + 'Again'] = false; if (ui.store === store) wkReread(st, store, k); }
    };
  }
  // A read the daemon answers 503 found the store busy (a fetch or push holds it) and runs
  // again after the daemon's Retry-After (WK_RETRY_MS: the adapter passes on the status, not
  // the header) instead of drawing the error, up to WK_RETRIES times in a row. Read k stays
  // busy meanwhile, so a change signal marks it to run again (wkReread) rather than starting
  // a second one; again() decides what still to read when the wait ends.
  var WK_RETRY_MS = 1000, WK_RETRIES = 5;
  function wkRetry(st, store, k, res, again) {
    if (res.ok || res.status !== 503 || (st[k + 'Tries'] || 0) >= WK_RETRIES) { st[k + 'Tries'] = 0; return false; }
    st[k + 'Tries'] = (st[k + 'Tries'] || 0) + 1;
    st[k + 'Busy'] = true;
    setTimeout(function () {
      st[k + 'Busy'] = false;
      if (ui.wiki[store] === st && ui.store === store) again();
    }, WK_RETRY_MS);
    return true;
  }
  function wkLoadList(st, store) {
    if (st.listBusy) return;
    st.listBusy = true; st.listAgain = false;
    AD.wiki.list(store, wkDone(st, store, 'list', function (res) {
      if (wkRetry(st, store, 'list', res, function () { wkLoadList(st, store); })) return;
      if (!res.ok) { st.listErr = res.error; if (!st.list) st.list = []; render(); return; }
      st.list = res.wikis; st.listErr = null;
      if (st.name && !st.list.some(function (w) { return w.name === st.name; }) && !(st.edit && st.edit.newWiki)) { st.name = null; st.pages = null; st.page = null; st.doc = null; }
      if (!st.name && st.list.length) st.name = st.list[0].name;
      render();
    }));
  }
  function wkLoadPages(st, store, name) {
    if (st.pagesBusy) return;
    st.pagesBusy = true; st.pagesAgain = false;
    AD.wiki.pages(store, name, wkDone(st, store, 'pages', function (res) {
      if (st.name !== name) { if (ui.store === store) wkPull(st); return; }
      if (wkRetry(st, store, 'pages', res, function () { if (st.name === name) wkLoadPages(st, store, name); else wkPull(st); })) return;
      if (!res.ok) { st.pagesErr = res.error; if (!st.pages) st.pages = { paths: [], commit: '' }; render(); return; }
      st.pages = { paths: res.pages.slice(), commit: res.commit }; st.pagesErr = null;
      if (!st.page && st.pages.paths.length) st.page = wkHas(st, 'index') ? 'index' : st.pages.paths[0];
      render();
    }));
  }
  // soft: a re-read of the page shown; one that is gone shows as an error, not as a new page
  function wkLoadDoc(st, store, name, path, soft) {
    if (st.docBusy) return;
    st.docBusy = true; st.docAgain = false;
    AD.wiki.read(store, name, path, wkDone(st, store, 'doc', function (res) {
      if (st.name !== name || st.page !== path) { if (ui.store === store) wkPull(st); return; }
      if (wkRetry(st, store, 'doc', res, function () { if (st.name === name && st.page === path && !st.edit) wkLoadDoc(st, store, name, path, soft); else wkPull(st); })) return;
      if (!res.ok) {
        // a page that is not there (a wiki: link to it) opens as a new page at that path; the
        // editor opens on the store shown, so a store switched away from meanwhile reads the
        // page again when it shows
        if (res.status === 404 && !st.edit && !soft) { st.doc = null; st.docErr = null; if (ui.store === store) wkStartEdit(path, true, false); return; }
        st.docErr = res.error; render(); return;
      }
      st.doc = { path: res.path, kind: res.kind === 'html' ? 'html' : 'markdown', content: res.content, commit: res.commit, html: res.html }; st.docErr = null;
      render();
    }));
  }
  // What the panel shows but has not read yet is read now; the draw shows what is there.
  function wkPull(st) {
    var store = ui.store;
    if (st.list === null) wkLoadList(st, store);
    if (st.name && st.pages === null) wkLoadPages(st, store, st.name);
    if (st.name && st.page && st.doc === null && !st.docErr && !st.edit) wkLoadDoc(st, store, st.name, st.page);
  }
  function wkReload(st) {
    st.list = null; st.pages = null; st.doc = null; st.docErr = null;
    wkPull(st);
  }
  // A tracker change (a save by an agent, by another browser, or one the host's poll round
  // brought in) re-reads the wiki shown; what arrives replaces what is drawn, the page
  // excepted while it is edited or its refused save shows.
  function wkRefresh() {
    if (!AD.wiki || !ui.store) return;
    var st = ui.wiki[ui.store], store = ui.store;
    if (!st || st.list === null || !(st.mode === 'wiki' || (ui.layout === 'narrow' && tabState().narrow === 'wiki'))) return;
    wkReread(st, store, 'list');
    wkReread(st, store, 'pages');
    wkReread(st, store, 'doc');
  }
  // One read of a change signal; a read k already in flight started before the change, so it
  // is marked to run again when it lands (wkDone) instead of being dropped.
  function wkReread(st, store, k) {
    if (st[k + 'Busy']) { st[k + 'Again'] = true; return; }
    if (k === 'list') wkLoadList(st, store);
    else if (k === 'pages') { if (st.name && !(st.edit && st.edit.newWiki)) wkLoadPages(st, store, st.name); }
    else if (st.name && st.page && !st.edit && !st.conflict) wkLoadDoc(st, store, st.name, st.page, true);
  }

  function wkPicker() {
    var st = wk(), pop = '';
    if (st.pick) {
      pop = '<div class="wk-pop" role="menu">' + (st.list || []).map(function (w) {
        return '<button type="button" class="wk-opt' + (w.name === st.name ? ' on' : '') + '" role="menuitem"' + attr('act', 'wk-pick') + attr('wiki', w.name) + '>' + wkIc('book') +
          '<span class="nm">' + esc(w.name) + '</span><span class="n">' + (w.name === st.name && st.pages ? esc(wsTn("js.tk.wk.pages", st.pages.paths.length)) : '') + '</span>' +
          '<span class="sub">' + esc(wsT("js.tk.wk.pick.tip", wkSha(w.tip))) + (w.unpushed ? ' &middot; ' + esc(wsT("js.tk.wk.pick.unpushed")) : '') + '</span></button>';
      }).join('') + '<div class="wk-popsep"></div><button type="button" class="wk-opt add" role="menuitem"' + attr('act', 'wk-newwiki') + '>' + wkIc('plus') + '<span>' + esc(wsT("js.tk.wk.pick.new")) + '</span></button>' +
        '<div class="wk-pophint">' + wsT("js.tk.wk.pick.hint") + '</div></div>';
    }
    return '<span class="wk-pickw"><button type="button" class="wk-pick"' + attr('act', 'wk-pickopen') + ' aria-haspopup="menu" aria-expanded="' + st.pick + '"' + tipAttr(wsT("js.tk.wk.pick.switch")) + '>' + wkIc('book') +
      '<span class="pre">wiki/</span><span class="nm">' + esc(st.name || '') + '</span>' + wkIc('chev') + '</button>' + pop + '</span>';
  }
  function wkSearchHtml() {
    var st = wk();
    return '<span class="psearch">' + wkIc('search') + '<input id="wksearch" type="text" role="searchbox" autocomplete="off" spellcheck="false" value="' + esc(st.q) + '" placeholder="' + esc(wsT("js.tk.wk.search", st.name || '')) + '" aria-label="' + esc(wsT("js.tk.wk.search.aria")) + '">' +
      (st.q ? '<button type="button" class="psx"' + attr('act', 'wk-qclear') + ' aria-label="' + esc(wsT("js.tk.panel.clearsearch")) + '">' + wkIc('x') + '</button>' : '') + '</span>';
  }
  // the header's tools: the picker, the page count, the tree toggle (wide), "new page", and the search box
  function wkTools(narrow) {
    var st = wk();
    if (st.list && !st.list.length && !(st.edit && st.edit.newWiki)) return '<span class="chip">' + esc(wsT("js.tk.wk.nowiki")) + '</span>';
    if (!st.name) return '<span class="chip">' + esc(wsT("js.tk.loading")) + '</span>';
    return wkPicker() + (st.pages && !narrow ? '<span class="chip">' + esc(wsTn("js.tk.wk.pages", st.pages.paths.length)) + '</span>' : '') + '<span class="grow"></span>' +
      (narrow ? '' : '<button type="button" class="ia"' + attr('act', 'wk-tree') + ' aria-pressed="' + st.tree + '" aria-label="' + esc(wsT("js.tk.wk.tree.aria")) + '"' + tipAttr(wsT(st.tree ? "js.tk.wk.tree.hide" : "js.tk.wk.tree.show")) + '>' + wkIc('sidebar') + '</button>') +
      '<button type="button" class="btn ib"' + attr('act', 'wk-newpage') + ' aria-label="' + esc(wsT("js.tk.wk.newpage")) + '"' + tipAttr(wsT("js.tk.wk.newpage")) + '>' + wkIc('plus') + '</button>' +
      '<div class="prow2">' + wkSearchHtml() + '</div>';
  }
  // The tree from the page paths: index first, then folders before pages, each folder foldable;
  // the search box filters by path (the texts are not here to search).
  function wkTreeHtml() {
    var st = wk(), q = st.q.trim().toLowerCase(), all = st.pages ? st.pages.paths : [], seen = {}, h = '';
    var paths = all.filter(function (p) { return !q || p.toLowerCase().indexOf(q) >= 0; }).sort(function (a, b) {
      if (a === 'index') return -1; if (b === 'index') return 1;
      var da = a.indexOf('/') >= 0, db = b.indexOf('/') >= 0;
      if (da !== db) return da ? -1 : 1;
      return a < b ? -1 : a > b ? 1 : 0;
    });
    function hl(t) { if (!q) return esc(t); var k = t.toLowerCase().indexOf(q); return k < 0 ? esc(t) : esc(t.slice(0, k)) + '<mark>' + esc(t.slice(k, k + q.length)) + '</mark>' + esc(t.slice(k + q.length)); }
    function folded(segs, upto) { for (var k = 0; k < upto; k++) if (st.closed[st.name + ':' + segs.slice(0, k + 1).join('/')]) return true; return false; }
    if (q) h += '<div class="wk-tcount">' + esc(wsT("js.tk.wk.match", paths.length, all.length)) + '</div>';
    paths.forEach(function (p) {
      var segs = p.split('/');
      for (var d = 0; d < segs.length - 1; d++) {
        var dir = segs.slice(0, d + 1).join('/');
        if (seen[dir]) continue;
        seen[dir] = 1;
        if (folded(segs, d)) continue;
        var open = !st.closed[st.name + ':' + dir];
        h += '<button type="button" class="wk-tn dir" style="--d:' + d + '"' + attr('act', 'wk-fold') + attr('dir', dir) + ' aria-expanded="' + open + '">' + wkIc('chev', 'cv') + wkIc('folder') + '<span class="t">' + esc(segs[d]) + '</span></button>';
      }
      if (folded(segs, segs.length - 1)) return;
      var on = p === st.page && !(st.edit && st.edit.isNew), ed = on && !!(st.edit || st.conflict);
      h += '<button type="button" class="wk-tn' + (on ? ' on' : '') + (ed ? ' ed' : '') + '" style="--d:' + (segs.length - 1) + '"' + attr('act', 'wk-open') + attr('path', p) + tipAttr(st.name + '/' + p) + (on ? ' aria-current="page"' : '') + '>' +
        wkIc('doc') + '<span class="t">' + hl(segs[segs.length - 1]) + '</span></button>';
    });
    if (q && !paths.length) h += '<div class="wk-tcount">' + esc(wsT("js.tk.wk.nomatch")) + '</div>';
    if (st.pagesErr) h += '<div class="wk-tcount err">' + esc(st.pagesErr) + '</div>';
    return h + '<button type="button" class="wk-tadd"' + attr('act', 'wk-newpage') + '>' + wkIc('plus') + esc(wsT("js.tk.wk.newpage")) + '</button>';
  }
  function wkCrumb(path) {
    var st = wk(), segs = String(path).split('/');
    return '<nav class="wk-path" aria-label="' + esc(wsT("js.tk.wk.path.aria")) + '"><a href="#"' + attr('act', 'wk-open') + attr('path', 'index') + '>' + esc(st.name) + '</a>' + segs.map(function (s, k) {
      return '<span class="sep">/</span>' + (k === segs.length - 1 ? '<b>' + esc(s) + '</b>' : '<span>' + esc(s) + '</span>');
    }).join('') + '</nav>';
  }
  function wkMoreMenu() {
    var st = wk();
    if (!st.more) return '';
    var link = 'wiki:' + st.name + '/' + st.page;
    return '<div class="wk-menu" role="menu">' +
      '<button type="button" role="menuitem"' + attr('act', 'wk-copy') + attr('text', link) + '>' + wkIc('link') + '<span>' + esc(wsT("js.tk.wk.copylink")) + '<br><code>' + esc(link) + '</code></span></button>' +
      '<button type="button" role="menuitem"' + attr('act', 'wk-copy') + attr('text', st.page) + '>' + wkIc('copy') + '<span>' + esc(wsT("js.tk.wk.copypath")) + '</span></button>' +
      '<button type="button" role="menuitem"' + attr('act', 'wk-agent') + '>' + wkIc('history') + '<span>' + esc(wsT(XTAB.chat ? "js.tk.wk.history.agent" : "js.tk.wk.history.tools")) + '</span></button></div>';
  }
  function wkPageRead() {
    var st = wk(), d = st.doc;
    var head = '<div class="wk-phead' + (st.flash ? ' wk-flash' : '') + '">' + wkCrumb(st.page) + '<span class="grow"></span>';
    if (d) head += '<span class="wk-meta"' + tipAttr(wsT("js.tk.wk.commit.tip", wkSha(d.commit))) + '><span class="wk-sha">' + esc(wkSha(d.commit)) + '</span></span>' +
      '<button type="button" class="ia"' + attr('act', 'wk-edit') + ' aria-label="' + esc(wsT("js.tk.wk.edit")) + '"' + tipAttr(wsT("js.tk.wk.edit")) + '>' + wkIc('pen') + '</button>' +
      '<span class="wk-more"><button type="button" class="ia"' + attr('act', 'wk-more') + ' aria-label="' + esc(wsT("js.tk.wk.more")) + '" aria-haspopup="menu" aria-expanded="' + st.more + '"' + tipAttr(wsT("js.tk.wk.more")) + '>' + wkIc('more') + '</button>' + wkMoreMenu() + '</span>';
    head += '</div>';
    if (st.docErr) return head + '<div class="wk-empty"><p class="err">' + esc(wsT("js.tk.wk.fail", st.docErr)) + '</p></div>';
    if (!d) return head + '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>';
    // an HTML page's frame is not drawn here: the slot marks where the frame layer shows it
    if (d.kind === 'html') return head + '<div class="wk-fslot"' + attr('fkey', wkFrameKey()) + '></div>';
    return head + '<article class="wk-doc md">' + refLinksHtml(d.html) + '</article>';
  }
  function wkPageEdit() {
    var st = wk(), ed = st.edit, chord = MAC ? '⌘Enter' : 'Ctrl+Enter';
    var head = '<div class="wk-phead">' + (ed.isNew ? '<nav class="wk-path"><span>' + esc(st.name) + '</span><span class="sep">/</span><b>' + esc(wsT("js.tk.wk.new.title")) + '</b></nav>' : wkCrumb(ed.path)) +
      '<span class="grow"></span><span class="chip st-doing">' + esc(wsT("js.tk.wk.editing")) + '</span></div>';
    var path = ed.isNew ? '<label class="wk-epath">' + esc(wsT("js.tk.wk.epath")) + ' <input id="wk-epath" type="text" data-keep value="' + esc(ed.newPath) + '" spellcheck="false" autocomplete="off" aria-label="' + esc(wsT("js.tk.wk.epath.aria")) + '"></label>' : '';
    var base = ed.newWiki ? wsT("js.tk.wk.base.new") : wsT("js.tk.wk.base", wkSha(ed.base));
    var bar = '<div class="tb">' + btn('wk-save', wsT("js.tk.dlg.save"), {}, 'primary' + (st.saving ? ' busy' : '')) + btn('wk-cancel', wsT("js.tk.dlg.cancel"), {}) +
      '<span class="muted">' + esc(wsT("js.tk.wk.hint", chord)) + '</span><span class="wk-base"' + tipAttr(wsT("js.tk.wk.base.tip", wkSha(ed.base))) + '>' + esc(base) + '</span></div>';
    return head + '<div class="wk-edit">' + (ed.note ? '<div class="wk-note">' + wsT("js.tk.wk.note", '<code>' + esc(wkSha(ed.base)) + '</code>') + '</div>' : '') +
      (path ? '<div class="wk-etop">' + path + '</div>' : '') + editorHtml('wk-ebody', { value: ed.text, rows: 14, label: esc(wsT("js.tk.wk.ed.aria")), pv: 'wk-doc', src: wkEditHtml(ed) }) + bar + '</div>';
  }
  // An edit of an HTML page (its path, or a new page's typed path, ends in .html) is source
  // only: no preview (the saved page is its preview) and no images, which it cannot show.
  function wkEditHtml(ed) { return !!ed && wkIsHtml(ed.isNew ? wkNewPath(ed.newPath) : ed.path); }
  function wkNewPath(raw) { return wkCanon(String(raw || '').trim().replace(/^\/+|\/+$/g, '').replace(/\/{2,}/g, '/')); }
  // A new page's seed: a Markdown heading, or an HTML page's document skeleton (the caret goes into its body).
  function wkSeed(path) {
    var name = String(path).split('/').pop();
    if (!wkIsHtml(path)) return '# ' + name.replace(/-/g, ' ') + '\n\n';
    return '<!DOCTYPE html>\n<html>\n<head>\n<meta charset="utf-8">\n<title>' + esc(name.slice(0, -5).replace(/-/g, ' ')) + '</title>\n</head>\n<body>\n\n</body>\n</html>\n';
  }
  // A new page's path typed into another kind changes the editor: an untouched seed becomes the
  // other kind's, and the draw adds or drops the preview.
  function wkPathKind(ed, raw) {
    var was = wkEditHtml(ed);
    ed.newPath = raw;
    if (wkEditHtml(ed) === was) return;
    if (!wkDirty(ed) && ed.seed !== undefined) ed.seed = ed.text = wkSeed(wkNewPath(raw));
    delete ui.edpv['wk-ebody'];
    render();
  }
  // Line regions of side that differ from base (an LCS alignment), as [{b0, b1: the base lines
  // they replace, lines: side's line indexes}]; wkMarks classes each side's lines 'conf' when
  // its region overlaps one of the other side (the lines that refused the merge) or 'chg'.
  function wkRegions(base, side) {
    // the lines both start and end with are aligned as they are; the table covers the rest
    var pre = 0, suf = 0, N = base.length, M = side.length;
    while (pre < N && pre < M && base[pre] === side[pre]) pre++;
    while (suf < N - pre && suf < M - pre && base[N - 1 - suf] === side[M - 1 - suf]) suf++;
    base = base.slice(pre, N - suf); side = side.slice(pre, M - suf);
    var n = base.length, m = side.length, T = [], i, j;
    for (i = 0; i <= n; i++) T.push(new Array(m + 1).fill(0));
    for (i = n - 1; i >= 0; i--) for (j = m - 1; j >= 0; j--) T[i][j] = base[i] === side[j] ? T[i + 1][j + 1] + 1 : Math.max(T[i + 1][j], T[i][j + 1]);
    var regs = [], cur = null;
    i = 0; j = 0;
    function close() { if (cur) { cur.b1 = i + pre; regs.push(cur); cur = null; } }
    while (i < n || j < m) {
      if (i < n && j < m && base[i] === side[j]) { close(); i++; j++; }
      else { if (!cur) cur = { b0: i + pre, lines: [] }; if (j < m && (i >= n || T[i][j + 1] >= T[i + 1][j])) { cur.lines.push(j + pre); j++; } else i++; }
    }
    close();
    return regs;
  }
  function wkMarks(base, a, b) {
    var ra = wkRegions(base, a), rb = wkRegions(base, b), ma = {}, mb = {}, conf = 0, other = 0;
    function hit(r, o) { return (r.b0 < o.b1 && o.b0 < r.b1) || r.b0 === o.b0; }
    ra.forEach(function (r) { var c = rb.some(function (o) { return hit(r, o); }); if (c) conf++; else other++; r.lines.forEach(function (k) { ma[k] = c ? 'conf' : 'chg'; }); });
    rb.forEach(function (r) { var c = ra.some(function (o) { return hit(r, o); }); if (!c) other++; r.lines.forEach(function (k) { mb[k] = c ? 'conf' : 'chg'; }); });
    return { a: ma, b: mb, conf: conf, other: other };
  }
  function wkRefused() {
    var st = wk(), c = st.conflict.c, base = String(st.conflict.orig || '').split('\n'), a = String(st.conflict.mine).split('\n'), b = String(c.theirsText || '').split('\n'), mk = wkMarks(base, a, b);
    // unchanged runs fold to a gap row (two lines of context around a change); a click shows the whole text
    function lines(arr, m) {
      var keep = arr.map(function (t, k) { if (st.full) return true; for (var d = -2; d <= 2; d++) if (m[k + d]) return true; return false; }), h = '', k = 0;
      while (k < arr.length) {
        if (keep[k]) { h += '<span class="wk-ln' + (m[k] ? ' ' + m[k] : '') + '" data-n="' + (k + 1) + '">' + (esc(arr[k]) || ' ') + '</span>'; k++; continue; }
        var j = k;
        while (j < arr.length && !keep[j]) j++;
        h += '<span class="wk-ln gap" data-n=""' + attr('act', 'wk-full') + tipAttr(wsT("js.tk.wk.gap.tip")) + '>&#8943; ' + esc(wsTn("js.tk.wk.gap", j - k)) + '</span>'; k = j;
      }
      return '<pre class="wk-lines">' + h + '</pre>';
    }
    var head = '<div class="wk-phead">' + wkCrumb(c.path || st.page) + '<span class="grow"></span><span class="chip wk-nosave">' + esc(wsT("js.tk.wk.notsaved")) + '</span></div>';
    return head + '<div class="wk-ref">' +
      '<div class="wk-refwhy">' + wkIc('warn') + '<div><b>' + esc(wsT("js.tk.wk.ref.title")) + '</b> ' + esc(c.reason ? String(c.reason).replace(/[\s.]*$/, '.') : wsT("js.tk.wk.ref.moved", wkSha(c.theirs), wkSha(c.base))) + ' ' +
      esc(wsTn("js.tk.wk.ref.conf", mk.conf)) + ', ' + esc(wsTn("js.tk.wk.ref.other", mk.other)) + '. ' + esc(wsT("js.tk.wk.ref.kept")) + '</div></div>' +
      '<div class="wk-refacts">' + btn('wk-copymine', wkIc('copy') + esc(wsT("js.tk.wk.copymine")), {}) + btn('wk-theirs', wkIc('reload') + esc(wsT("js.tk.wk.theirs")), {}) + btn('wk-again', wkIc('pen') + esc(wsT("js.tk.wk.again")), {}, 'primary') +
      '<span class="wk-key"><span><i class="k-conf"></i>' + esc(wsT("js.tk.wk.key.conf")) + '</span><span><i class="k-chg"></i>' + esc(wsT("js.tk.wk.key.chg")) + '</span><a href="#"' + attr('act', 'wk-full') + '>' + esc(wsT(st.full ? "js.tk.wk.changes" : "js.tk.wk.full")) + '</a></span></div>' +
      '<div class="wk-vs"><section class="wk-side"><header>' + esc(wsT("js.tk.wk.yours")) + '<span class="wk-sha">' + esc(wsT("js.tk.wk.from", wkSha(c.base))) + '</span><span class="grow"></span><span>' + esc(wsT("js.tk.wk.unsaved")) + '</span></header>' + lines(a, mk.a) + '</section>' +
      '<section class="wk-side"><header>' + esc(wsT("js.tk.wk.theirs.side")) + '<span class="wk-sha">' + esc(wkSha(c.theirs)) + '</span><span class="grow"></span></header>' + lines(b, mk.b) + '</section></div></div>';
  }
  // The empty store ("no wiki yet") and the "New wiki" form: a wiki is created by its first page.
  function wkEmptyHtml(create) {
    var st = wk(), ok = WK_NAME.test(st.wname) && st.wname.length <= WK_NAME_MAX;
    return '<div class="wk-empty">' + (create ? '' : wkIc('book') + '<h3>' + esc(wsT("js.tk.wk.empty.title")) + '</h3><p>' + wsT("js.tk.wk.empty.text") + '</p>') +
      (st.listErr ? '<p class="err">' + esc(wsT("js.tk.wk.fail", st.listErr)) + '</p>' : '') +
      '<form class="wk-form">' + (create ? '<b>' + esc(wsT("js.tk.wk.newwiki")) + '</b>' : '') +
      '<label class="wk-fld">' + esc(wsT("js.tk.wk.name")) + '<span class="wk-in"><span>wiki/</span><input id="wk-wname" type="text" data-keep value="' + esc(st.wname) + '" spellcheck="false" autocomplete="off"></span><span class="h' + (ok ? '' : ' bad') + '" id="wk-wnameh">' + esc(wsT(ok ? "js.tk.wk.name.ok" : "js.tk.wk.name.bad")) + '</span></label>' +
      '<label class="wk-fld">' + esc(wsT("js.tk.wk.first")) + '<span class="wk-in"><input id="wk-wfirst" type="text" data-keep value="' + esc(st.wfirst) + '" spellcheck="false" autocomplete="off" style="padding-left:8px"></span><span class="h">' + esc(wsT("js.tk.wk.first.hint")) + '</span></label>' +
      '<div class="tb">' + (create ? btn('wk-cancelnew', wsT("js.tk.dlg.cancel"), {}) : '') + btn('wk-create', wkIc('plus') + esc(wsT("js.tk.wk.create")), {}, 'primary') + '</div></form></div>';
  }
  function wkNameInput(el) {
    var st = wk(), h = $('wk-wnameh');
    st.wname = el.value;
    var ok = WK_NAME.test(st.wname) && st.wname.length <= WK_NAME_MAX;
    if (h) { h.className = 'h' + (ok ? '' : ' bad'); h.textContent = wsT(ok ? "js.tk.wk.name.ok" : "js.tk.wk.name.bad"); }
  }
  function wkPageHtml() {
    var st = wk();
    if (st.creating) return wkEmptyHtml(true);
    if (st.conflict) return wkRefused();
    if (st.edit) return wkPageEdit();
    if (!st.page) return st.pages && !st.pagesBusy ? wkEmptyPages() : '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>';
    return wkPageRead();
  }
  function wkEmptyPages() {
    return '<div class="wk-empty">' + wkIc('book') + '<h3>' + esc(wsT("js.tk.wk.nopages")) + '</h3><div class="tb">' + btn('wk-newpage', wkIc('plus') + esc(wsT("js.tk.wk.newpage")), {}, 'primary') + '</div></div>';
  }
  // The wiki as the Items panel's card spec (panelHtml), or the narrow layout's Wiki tab (paneHtml).
  function cardWiki(v, narrow) {
    var st = wk();
    wkPull(st);
    var none = st.list !== null && !st.list.length && !(st.edit && st.edit.newWiki), body;
    if (narrow) {
      body = none ? wkEmptyHtml(false) : !st.name ? '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>' :
        '<div class="wk-nartree"><button type="button" class="wk-tsel"' + attr('act', 'wk-ntree') + ' aria-expanded="' + st.ntree + '">' + wkIc('tree') + '<span>' + esc(wsT("js.tk.wk.tree.aria")) + '</span><code>' + esc(st.edit && st.edit.isNew ? wsT("js.tk.wk.new.title") : st.page || '') + '</code>' + wkIc('chev', 'cv') + '</button>' +
        (st.ntree ? '<nav class="wk-tree" id="wktree" aria-label="' + esc(wsT("js.tk.wk.tree.aria")) + '">' + wkTreeHtml() + '</nav>' : '') + '</div><div class="wk-page" id="wkpage">' + wkPageHtml() + '</div>';
      return { tools: wkTools(true), body: body, id: 'card-wiki', cls: '', bodyCls: 'wk-nbody' };
    }
    if (none) return { tools: wkTools(false), body: wkEmptyHtml(false), id: 'card-items', cls: 'panel', bodyCls: 'wk-body notree' };
    if (!st.name) return { tools: wkTools(false), body: '<div class="wk-empty"><p class="muted">' + esc(wsT("js.tk.loading")) + '</p></div>', id: 'card-items', cls: 'panel', bodyCls: 'wk-body notree' };
    body = '<nav class="wk-tree" id="wktree" aria-label="' + esc(wsT("js.tk.wk.tree.aria")) + '">' + wkTreeHtml() + '</nav><div class="wk-page" id="wkpage">' + wkPageHtml() + '</div>';
    return { tools: wkTools(false), body: body, id: 'card-items', cls: 'panel', bodyCls: 'wk-body' + (st.tree ? '' : ' notree') + (ui.page ? ' wide' : '') };
  }
  // After a draw: the page and tree keep their scroll (a page just opened starts at the top), a
  // wiki: link in any rendered text gets its mark, and the flash runs once.
  function wkAfterDraw(body, scroll) {
    var st = ui.store ? wk() : null;
    ['wkpage', 'wktree'].forEach(function (k) { if ($(k) && scroll[k] && !(st && st.top && k === 'wkpage')) $(k).scrollTop = scroll[k]; });
    wkMarkLinks(body);
    if (st) { st.flash = false; st.top = false; }
  }
  function wkMarkLinks(root) {
    if (!AD.wiki || !root) return;
    var st = ui.store ? wk() : null;
    Array.prototype.forEach.call(root.querySelectorAll('a[data-wiki][data-page]'), function (a) {
      var name = a.getAttribute('data-wiki'), path = a.getAttribute('data-page'), miss = !!(st && st.name === name && st.pages && !wkHas(st, path));
      a.classList.add('wlink'); a.classList.toggle('miss', miss);
      if (!a.querySelector('svg.i')) a.insertAdjacentHTML('afterbegin', wkIc('book'));
      a.setAttribute('title', 'wiki:' + name + '/' + path + (miss ? ' (' + wsT("js.tk.wk.missing") + ')' : ''));
    });
  }
  // An HTML page (kind html) shows in the daemon's sandboxed iframe markup (doc.html, inserted
  // as given: the sandbox attribute is the host's), kept in the layer #tkframe outside the
  // subtree render() replaces. An iframe reloads whenever it is detached, so a frame drawn into
  // the page would restart the page's scripts on every draw. The read view leaves an empty
  // slot (.wk-fslot) in the page area instead, and after every draw wkFrameSync puts the layer
  // over it. The frame belongs to one key (store, wiki, path, commit): a draw that keeps the
  // key keeps the same iframe element; a draw without the slot (the item window, the editor or
  // refused-save view, the drawer closed, another panel, or the page's own menus open over it)
  // hides the layer and keeps the frame, so coming back does not reload it; a key change
  // empties the layer, and the new frame goes in once a slot for it shows. Nothing listens for
  // the frame's messages: the host takes none from a wiki page.
  var WKF = { key: '', slot: null, ro: null };
  function wkFrameKey() {
    var st = ui.store && ui.wiki[ui.store], d = st && st.doc;
    return d && d.kind === 'html' && d.html ? JSON.stringify([ui.store, st.name, d.path, d.commit]) : '';
  }
  // root: the subtree the draw filled, or null when nothing of the store shows
  function wkFrameSync(root) {
    var L = $('tkframe'), key = wkFrameKey(), st = key ? wk() : null, slot = null;
    if (key && root && !ui.win && !st.more && !st.pick) slot = root.querySelector('.wk-fslot');
    if (slot && slot.getAttribute('data-fkey') !== key) slot = null;
    if (WKF.key !== key && WKF.key) { L.innerHTML = ''; WKF.key = ''; }
    if (slot && !WKF.key) { L.innerHTML = st.doc.html; WKF.key = key; }
    WKF.slot = slot;
    L.hidden = !slot;
    // the slot moves and resizes with the drawer's drag and opening, the layout, and the window
    // (an outer frame resizing this page fires its resize), not only with a draw
    if (WKF.ro) WKF.ro.disconnect();
    if (slot && window.ResizeObserver) {
      WKF.ro = WKF.ro || new ResizeObserver(wkFramePlace);
      for (var n = slot; n && n !== document.body; n = n.parentElement) if (n === slot || wkClips(n)) WKF.ro.observe(n);
    }
    wkFramePlace();
  }
  function wkClips(n) { var cs = getComputedStyle(n); return cs.overflowX !== 'visible' || cs.overflowY !== 'visible'; }
  // The layer covers the slot's box, clipped to what its scrolling and clipping ancestors show
  // (the drawer's clip while it opens).
  function wkFramePlace() {
    var L = $('tkframe'), s = WKF.slot;
    if (!s || L.hidden) return;
    var r = s.getBoundingClientRect(), t = r.top, l = r.left, b = r.bottom, rt = r.right;
    for (var n = s.parentElement; n && n !== document.body; n = n.parentElement) {
      if (!wkClips(n)) continue;
      var q = n.getBoundingClientRect();
      t = Math.max(t, q.top); l = Math.max(l, q.left); b = Math.min(b, q.bottom); rt = Math.min(rt, q.right);
    }
    var S = L.style;
    S.left = r.left + 'px'; S.top = r.top + 'px'; S.width = r.width + 'px'; S.height = r.height + 'px';
    S.clipPath = b > t && rt > l ? 'inset(' + (t - r.top) + 'px ' + (r.right - rt) + 'px ' + (r.bottom - b) + 'px ' + (l - r.left) + 'px)' : 'inset(50%)';
  }
  // The panel shows the wiki: the Items | Wiki switch in the wide layouts, the Wiki tab when narrow.
  function wkShow() { if (ui.layout === 'narrow') { tabState().narrow = 'wiki'; } else wk().mode = 'wiki'; }
  // An edit is dirty when its text left where it started: the page's text, the seeded heading of
  // a new page (seed; orig stays '' as the conflict view's merge base), or the refused-save
  // "theirs" text after Edit again.
  function wkDirty(ed) { return !!ed && ed.text !== (ed.seed !== undefined ? ed.seed : ed.orig); }
  // Asked before a navigation that drops the wiki edit; false keeps the editor, draft, and page.
  // Cancel, Esc, and taking theirs drop the draft on purpose and do not ask.
  function wkLeaveOk() { return !wkDirty(wk().edit) || window.confirm(wsT("js.tk.wk.leave")); }
  function wkOpenPage(name, path) {
    if (wkLeaveOk()) wkGoPage(name, path);
  }
  function wkGoPage(name, path) {
    var st = wk();
    path = wkCanon(path); // the page list and the read answer the canonical path
    st.creating = false; st.edit = null; st.conflict = null; st.more = false; st.pick = false; st.ntree = false;
    if (st.name !== name) { st.name = name; st.pages = null; st.q = ''; }
    st.page = path; st.doc = null; st.docErr = null; st.flash = true; st.top = true;
    // a link into a wiki the store does not have yet starts that wiki with this page
    if (st.list && !st.list.some(function (w) { return w.name === name; })) { st.pages = { paths: [], commit: '' }; wkStartEdit(path, true, true); return; }
    if (st.pages && !wkHas(st, path)) { wkStartEdit(path, true, false); return; }
    wkShow();
    render();
  }
  function wkOpenLink(name, path) {
    if (!wkLeaveOk()) return; // declining leaves the item window open too
    if (ui.win) closeItemWindow();
    wkGoPage(name, path);
  }
  function wkStartEdit(path, isNew, newWiki) {
    var st = wk();
    st.creating = false; st.conflict = null; st.more = false; st.pick = false; st.ntree = false;
    delete ui.edpv['wk-ebody']; delete ui.eatt['wk-ebody'];
    var seed = wkSeed(path);
    if (isNew) st.edit = { path: path, isNew: true, newWiki: !!newWiki, base: st.pages ? st.pages.commit : '', text: seed, orig: '', seed: seed, newPath: path, note: false };
    else st.edit = { path: path, isNew: false, newWiki: false, base: st.doc.commit, text: st.doc.content, orig: st.doc.content, newPath: '', note: false };
    wkShow();
    render();
    var f = $(isNew && (!path || path.slice(-1) === '/') ? 'wk-epath' : 'wk-ebody');
    if (f) {
      f.focus({ preventScroll: true });
      var at = f.value.length, body = isNew && wkIsHtml(path) ? f.value.indexOf('\n</body>') : -1;
      if (body >= 0) at = body;
      if (f.id === 'wk-ebody') { try { f.setSelectionRange(at, at); } catch (x) { /* not a text field */ } }
    }
  }
  function wkCancel() {
    var st = wk();
    if (!st.edit) return;
    var ed = st.edit;
    st.edit = null; st.conflict = null;
    if (ed.newWiki) { st.name = null; st.page = null; st.pages = null; st.doc = null; st.list = null; }
    else if (ed.isNew) { st.page = wkHas(st, 'index') ? 'index' : (st.pages && st.pages.paths[0]) || null; st.doc = null; }
    render();
  }
  function wkCreate() {
    var st = wk(), name = ($('wk-wname') ? $('wk-wname').value : st.wname).trim(), first = ($('wk-wfirst') ? $('wk-wfirst').value : st.wfirst).trim().replace(/^\/+|\/+$/g, '') || 'index';
    st.wname = name; st.wfirst = first;
    if (!WK_NAME.test(name) || name.length > WK_NAME_MAX) return toast(esc(wsT("js.tk.wk.toast.name")));
    if (st.list && st.list.some(function (w) { return w.name === name; })) { st.creating = false; wkOpenPage(name, first); return; }
    st.name = name; st.pages = { paths: [], commit: '' }; st.page = first; st.doc = null; st.docErr = null;
    wkStartEdit(first, true, true);
  }
  function wkSave() {
    var st = wk(), ed = st.edit, store = ui.store, name = st.name;
    if (!ed || st.saving || !name) return;
    var box = $('wk-ebody');
    if (box) ed.text = box.value;
    var path = ed.isNew ? wkNewPath($('wk-epath') ? $('wk-epath').value : ed.newPath) : ed.path;
    if (!path) return toast(esc(wsT("js.tk.wk.toast.path")));
    if (ed.isNew && wkHas(st, path)) return toast(esc(wsT("js.tk.wk.toast.exists", path)));
    st.saving = true; render();
    AD.wiki.save(store, name, { base: ed.newWiki ? '' : ed.base, newWiki: !!ed.newWiki, op: 'write', path: path, content: ed.text, attachments: wkIsHtml(path) ? [] : savedAtts('wk-ebody', ed.text) }, function (res) {
      st.saving = false;
      var live = ui.wiki[store] === st && st.edit === ed; // the editor this save left from still shows
      if (res.ok) {
        toast(esc(wsT(res.merged ? "js.tk.wk.saved.merged" : "js.tk.wk.saved", wkSha(res.commit))) + (res.warnings || []).map(function (w) { return '<br>' + esc(w); }).join(''));
        if (live) { st.edit = null; st.conflict = null; st.page = path; st.flash = true; st.top = true; }
        // what the save changed is re-read on the next draw of this store's wiki
        st.list = null; st.pages = null; st.doc = null; st.docErr = null;
        render(); return;
      }
      if (!live) { if (!res.conflict) toast(esc(wsT("js.tk.wk.toast.fail", res.error))); return; }
      if (res.conflict) { st.conflict = { c: res.conflict, mine: ed.text, orig: ed.orig }; st.full = false; render(); return; }
      toast(esc(wsT("js.tk.wk.toast.fail", res.error))); render();
    });
  }
  function wkAct(k, a) {
    var st = wk();
    if (k === 'wk-mode') { st.mode = a.getAttribute('data-mode'); st.pick = st.more = false; render(); }
    else if (k === 'wk-pickopen') { st.pick = !st.pick; st.more = false; render(); }
    else if (k === 'wk-pick') { st.pick = false; var nm2 = a.getAttribute('data-wiki'); if (nm2 !== st.name && wkLeaveOk()) { st.name = nm2; st.pages = null; st.page = null; st.doc = null; st.docErr = null; st.edit = null; st.conflict = null; st.q = ''; st.creating = false; } render(); }
    else if (k === 'wk-newwiki') { st.pick = false; if (!wkLeaveOk()) { render(); return; } st.creating = true; st.edit = null; st.conflict = null; st.wname = ''; render(); if ($('wk-wname')) $('wk-wname').focus(); }
    else if (k === 'wk-cancelnew') { st.creating = false; render(); }
    else if (k === 'wk-create') wkCreate();
    else if (k === 'wk-open') wkOpenPage(st.name, a.getAttribute('data-path'));
    else if (k === 'wk-fold') { var fk = st.name + ':' + a.getAttribute('data-dir'); st.closed[fk] = !st.closed[fk]; render(); }
    else if (k === 'wk-tree') { st.tree = !st.tree; render(); }
    else if (k === 'wk-ntree') { st.ntree = !st.ntree; render(); }
    else if (k === 'wk-qclear') { st.q = ''; render(); if ($('wksearch')) $('wksearch').focus(); }
    else if (k === 'wk-more') { st.more = !st.more; st.pick = false; render(); }
    else if (k === 'wk-edit') { if (st.doc) wkStartEdit(st.page, false, false); }
    else if (k === 'wk-newpage') { if (st.name) wkStartEdit(st.page && st.page.indexOf('/') >= 0 ? st.page.slice(0, st.page.lastIndexOf('/') + 1) : '', true, false); }
    else if (k === 'wk-save') wkSave();
    else if (k === 'wk-cancel') wkCancel();
    else if (k === 'wk-copy' || k === 'wk-copymine') {
      var text = k === 'wk-copymine' ? st.conflict.mine : a.getAttribute('data-text');
      st.more = false;
      copyText(text, a.parentNode, function (ok) { toast(esc(ok ? wsT("js.toast.copied", k === 'wk-copymine' ? wsT("js.tk.wk.yours") : text) : wsT("js.tk.win.copyfail"))); if (k === 'wk-copy') render(); });
    }
    else if (k === 'wk-agent') {
      st.more = false;
      if (XTAB.chat) { if (ui.layout === 'narrow') tabState().narrow = 'chat'; else tabState().side = 'chat'; }
      else toast(esc(wsT("js.tk.wk.history.tools")));
      render();
    }
    else if (k === 'wk-theirs') { var c = st.conflict.c; st.conflict = null; st.edit = null; st.doc = null; st.docErr = null; toast(esc(wsT("js.tk.wk.reloaded", wkSha(c.theirs)))); wkReload(st); render(); }
    else if (k === 'wk-again') {
      var cf = st.conflict;
      st.conflict = null;
      st.edit = { path: cf.c.path || st.page, isNew: !cf.c.theirs, newWiki: !cf.c.theirs, base: cf.c.theirs || '', text: cf.mine, orig: cf.c.theirsText || '', newPath: cf.c.path || st.page, note: !!cf.c.theirs };
      delete ui.edpv['wk-ebody'];
      render();
      if ($('wk-ebody')) $('wk-ebody').focus();
    }
    else if (k === 'wk-full') { st.full = !st.full; render(); }
  }

  // The store settings: the store-wide values that are not roles, apart from the roles-event
  // dialog so they can be found. Every member reads them here; an admin's Edit opens the
  // setting's own dialog.
  function settingsHtml(admin) {
    function row(label, value, act) {
      return '<div class="srow"><span class="slabel">' + label + '</span><span class="sval">' + value + '</span>' + (admin ? btn(act, wsT("js.tk.act.edit"), {}) : '') + '</div>';
    }
    var prio = VS.priority.names.map(function (x, i) { return i === VS.priority.default ? wsT("js.tk.team.default", esc(x)) : esc(x); }).join(' &lt; ');
    var ls = VS.labels || [];
    var labels = '<div class="srow"><span class="slabel">' + wsT("js.tk.set.labels") + '</span><span class="sval">' + (ls.length ? '' : '<span class="muted">' + wsT("js.tk.set.labels.none") + '</span>') + '</span>' +
      (admin ? btn('slabel', wsT("js.tk.set.labels.add"), {}) : '') + '</div>' + (ls.length ? '<ul class="slabels">' + ls.map(function (l) {
        return '<li>' + labelChip(l) + (admin ? '<span class="grow"></span>' + btn('slabel', wsT("js.tk.act.edit"), { label: l.id }) + btn('slabel-rm', wsT("js.tk.set.labels.remove"), { label: l.id }) : '') + '</li>';
      }).join('') + '</ul>' : '');
    return '<div class="tkset"><h3>' + wsT("js.tk.set.title") + '</h3><p class="muted">' + (admin ? wsT("js.tk.set.intro.admin") : wsT("js.tk.set.intro.member")) + '</p>' +
      row(wsT("js.tk.set.prio"), prio, 'sprio') + row(wsT("js.tk.set.att"), esc(fmtSize(VS.attachments.max_bytes)), 'satt') + labels + '</div>';
  }

  function cardTeam(v) {
    var me = VS.members[v] || {}, R = VS.viewer.reach, levels = {}, seen = {};
    Object.keys(R).forEach(function (k) { levels[R[k]] = (levels[R[k]] || 0) + 1; });
    var summary = wsT("js.tk.team.summary", esc(VS.admins.join(', ')), VS.viewer.lead ? wsT("js.tk.team.lead", esc(VS.viewer.lead)) : wsT("js.tk.team.root"),
      Object.keys(R).length ? wsT("js.tk.team.reach", Object.keys(levels).map(function (d) { return wsT("js.tk.team.reachat", levels[d], d); }).join(', ')) : wsT("js.tk.team.nofollowers"));
    function shortKey(k) {
      var w = String(k).split(/\s+/), b = w[1] || w[0];
      if (String(k).length <= 32) return String(k);
      return (w[1] ? w[0] + ' ' : '') + b.slice(0, 8) + '\u2026' + b.slice(-8);
    }
    function keyLine(k, rev) {
      return '<div class="kline' + (rev ? ' rev' : '') + '"><code' + tipAttr(k) + '>' + esc(shortKey(k)) + '</code>' + (rev ? '<span class="chip flag">' + wsT("js.tk.team.key.revoked") + '</span>' : '') +
        '<button type="button" class="idcopy"' + attr('act', 'idcopy') + attr('id', k) + ' aria-label="' + esc(wsT("js.tk.team.key.copy.aria")) + '"' + tipAttr(k) + '><svg class="i"><use href="#tki-copy"/></svg><span>' + wsT("js.tk.team.key.copy") + '</span></button></div>';
    }
    function node(id) {
      if (seen[id]) return '<li>' + wsT("js.tk.team.cycle", esc(id)) + '</li>';
      seen[id] = true;
      var m = VS.members[id], open = !!ui.mdet[ui.store + '|' + id];
      var chips = (id === v ? '<span class="chip you">' + wsT("js.tk.team.you") + '</span>' : '') + (m.admin ? '<span class="chip admin">' + wsT("js.tk.team.admin") + '</span>' : '') +
        (id === VS.viewer.lead ? '<span class="chip">' + wsT("js.tk.team.yourlead") + '</span>' : '') + (R[id] ? '<span class="chip" title="' + wsT("js.tk.team.levels.tip") + '">L' + R[id] + '</span>' : '') +
        (m.joined ? '<span class="chip" title="' + wsT("js.tk.team.joined.tip", esc(alias(m.joined))) + '">' + wsT("js.tk.team.joined") + '</span>' : '') +
        (m.revoked.length ? '<span class="chip flag" title="' + wsT("js.tk.team.revoked.tip") + '">' + wsT("js.tk.team.revoked", m.revoked.length) + '</span>' : '') +
        ((VS.clones[id] || []).length > 1 ? '<span class="chip" title="' + wsT("js.tk.team.clones.tip") + '">' + wsT("js.tk.team.clones", VS.clones[id].length) + '</span>' : '');
      return '<li><div class="node' + (id === v ? ' me' : '') + '">' + avHtml(id) + (m.profile.org ? '<span class="org">' + esc(m.profile.org) + '</span>' : '') +
        '<span class="nm">' + esc(m.name) + '</span><span class="nid">' + esc(id) + '</span>' + chips +
        '<span class="grow"></span><button type="button" class="ia mdet"' + attr('act', 'mdet') + attr('who', id) + ' aria-expanded="' + (open ? 'true' : 'false') + '" aria-label="' + esc(wsT("js.tk.team.details")) + '"' + tipAttr(wsT("js.tk.team.details")) + '><svg class="i"><use href="#tki-more"/></svg></button></div>' +
        (open ? '<div class="mdetails">' + m.keys.map(function (k) { return keyLine(k, false); }).join('') + m.revoked.map(function (x) { return keyLine(x.key, true); }).join('') +
          (id === v || me.admin ? '<div>' + btn('profile', wsT("js.tk.team.editprofile"), { who: id }) + '</div>' : '') + '</div>' : '') +
        (m.children.length ? '<ul>' + m.children.map(node).join('') + '</ul>' : '') + '</li>';
    }
    var roots = Object.keys(VS.members).filter(function (id) { var m = VS.members[id], l = m.leads; return !m.removed && (!l.length || !VS.members[l[0]] || VS.members[l[0]].removed); });
    var h = '<p class="r2" style="margin:0 0 6px">' + summary + '</p>';
    if (VS.rolesProblems.length) h += '<p class="err">' + wsT("js.tk.team.problems", esc(VS.rolesProblems.join('; '))) + '</p>';
    h += '<ul class="tree">' + roots.map(node).join('') + '</ul>';
    var gone = Object.keys(VS.members).filter(function (id) { return VS.members[id].removed; });
    if (gone.length) h += '<p class="r2">' + wsT("js.tk.team.removed", esc(gone.join(', '))) + '</p>';
    var lost = Object.keys(VS.members).filter(function (id) { return !seen[id] && !VS.members[id].removed; });
    if (lost.length) h += '<p class="err">' + wsT("js.tk.team.unreachable", esc(lost.join(', '))) + '</p>';
    h += '<div class="toolbar" style="margin-top:8px">' + btn('admin', wsT("js.tk.team.adminbtn"), {}) + (X.teamTools ? X.teamTools(ui.store, VS, H) : '') +
      '<span class="muted">' + (me.admin ? wsT("js.tk.team.isadmin") : wsT("js.tk.team.notadmin")) + '</span></div>';
    h += settingsHtml(!!me.admin);
    var nMem = Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).length;
    return card(wsT("js.tk.tab.org"), wsTn("js.tk.team.members", nMem), h, '', 'card-org');
  }

  function kindFilter(it) { return ui.kind === 'request' ? !!it.pending.length : ui.kind === 'claimable' ? !!it.claimable : ui.kind === 'all' || it.kind === ui.kind; }
  function itemFilter(it) {
    if (!kindFilter(it)) return false;
    var s = it.status;
    if (ui.status === 'hidden') return it.hidden;
    if (it.hidden) return false;
    if (ui.status === 'open') return !wrapped(s);
    if (ui.status === 'done' || ui.status === 'in-review' || ui.status === 'dropped') return s === ui.status;
    if (ui.status === 'blocked') return s === 'blocked' || it.rollup.blocked.length;
    if (ui.status === 'idea') return s === 'idea';
    return true;
  }
  function sel(id, val, list) {
    return '<select id="' + id + '">' + list.map(function (x) {
      var k = Array.isArray(x) ? x[0] : x, l = Array.isArray(x) ? x[1] : x;
      return '<option value="' + esc(k) + '"' + (k === val ? ' selected' : '') + '>' + esc(l) + '</option>';
    }).join('') + '</select>';
  }
  function itemFilterBar(kindOnly) {
    return wsT("js.tk.filter.kind") + ' ' + sel('f-kind', ui.kind, [['all', wsT("js.tk.filter.all")], ['task', kindName('task')], ['topic', kindName('topic')], ['request', wsT("js.tk.filter.request")], ['claimable', wsT("js.tk.filter.claimable")]]) +
      (kindOnly ? '' : ' ' + wsT("js.tk.filter.status") + ' ' + sel('f-status', ui.status, [['all', wsT("js.tk.filter.all")], ['open', wsT("js.tk.filter.open")], ['idea', stLabel('idea')], ['blocked', stLabel('blocked')], ['in-review', stLabel('in-review')], ['done', stLabel('done')], ['dropped', stLabel('dropped')], ['hidden', wsT("js.tk.chip.hidden")]]));
  }
  function cardActivity() {
    var chip = wsT("js.tk.activity.chip", VS.stats.events, VS.stats.flagged) + (VS.stats.concurrent ? ', ' + wsT("js.tk.row.nconcurrent", VS.stats.concurrent) : ''), m = modeOf('activity');
    if (m !== 'list') return card(wsT("js.tk.tab.activity"), chip, modeBody('activity', m), picker('activity'), 'card-activity', m);
    var rows = VS.events.filter(function (n) { return (!ui.act.flagged || n.status !== 'applied') && (!ui.act.conc || n.markers.length); }), cap = 50, shown = rows.slice(-cap).reverse();
    var h = lastWrite();
    h += '<div class="toolbar">' + actChips() + '<span class="grow"></span><span class="muted">' + wsT("js.tk.activity.shown", shown.length, rows.length) + '</span></div>';
    h += shown.length ? '<ul class="plain">' + shown.map(function (n) { return evLine(n, false); }).join('') + '</ul>' : '<div class="empty">' + wsT("js.tk.activity.empty") + '</div>';
    return card(wsT("js.tk.tab.activity"), chip, h, picker('activity'), 'card-activity', m);
  }
  // The Activity filters "flagged only" and "concurrent only": toggle chips with the look of the
  // card's mode chips (list / activity timeline), the accent when on.
  function actChips() {
    return [['flagged', "js.tk.activity.flagged"], ['conc', "js.tk.activity.conc"]].map(function (x) {
      var on = !!ui.act[x[0]];
      return '<button type="button" class="btn mini' + (on ? ' primary' : '') + '" aria-pressed="' + on + '"' + attr('act', 'actf') + attr('f', x[0]) + attr('fk', 'actf:' + x[0]) + '>' + wsT(x[1]) + '</button>';
    }).join('');
  }
  function lastWrite() {
    if (!ui.last) return '';
    return '<div class="lastact">' + wsT("js.tk.activity.last", ui.last.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ' ' + (n.status === 'applied' ? '<span class="chip st-done">' + wsT("js.tk.activity.applied") + '</span>' : '<span class="chip flag">' + esc(n.status) + '</span> ' + esc(n.reason));
    }).join('; ')) + '</div>';
  }

  // ------------------------------------------------------------- view modes

  // The kanban, activity timeline, due timeline, and calendar renderers draw the
  // body of the Items panel for one scope (its own rows; "all" keeps its filters), or of
  // the Activity card. The panel is the wide main zone, so no mode spans rows.
  function modeBody(key, m, v) {
    if (m === 'kanban') return kanbanBody(key, v);
    if (m === 'gantt') return ganttBody(key, dueRows(key, v));
    if (m === 'calendar') return calendarBody(key, dueRows(key, v));
    if (m === 'activity') return activityBody();
    return '';
  }
  // The assignment records a scope shows: mine (live plus pending to me), given (what I
  // assigned or handed to others, live or pending), team (followers' open and unaccepted work).
  // A Today filter keeps only its records. A topic record (its holder) has no status, so no scope shows it here.
  function assignRecs(key, v) {
    var recs;
    if (key === 'mine') recs = VS.my.assignments.concat(VS.my.received.pending).map(A);
    else if (key === 'given') recs = [].concat.apply([], VS.my.given.map(function (id) {
      return VS.items[id].assignments.map(A).filter(function (r) { return (r.by === v || r.sender === v) && r.to !== v && (r.state === 'live' || r.state === 'pending'); });
    }));
    else recs = [].concat.apply([], VS.my.team.map(function (p) { return p.open.concat(p.pending); })).map(A);
    return recs.filter(function (r) { return keepRec(r) && VS.items[r.item].kind !== 'topic'; });
  }

  // Kanban: columns by status. Assignment scopes: cards are assignments (with the
  // pending column); "all": cards are items by item status, whose idea column's header "+"
  // opens an input row atop it (knewRowHtml). Dragging moves only the viewer's own
  // live assignment (item.status, or item.close on the done column).
  var KCOLS = { assign: ['pending', 'idea', 'todo', 'doing', 'blocked', 'in-review', 'done', 'dropped'],
    items: ['pending', 'idea', 'todo', 'doing', 'blocked', 'in-review', 'done', 'dropped'] };
  function kanbanBody(key, v) {
    var cols = KCOLS[key === 'all' ? 'items' : 'assign'], buckets = {};
    cols.forEach(function (c) { buckets[c] = []; });
    if (key === 'all') {
      VS.itemOrder.forEach(function (id) {
        var it = VS.items[id];
        // the columns already are the statuses, so the status filter would only empty them: kind filter only
        if (it.hidden || it.kind === 'topic' || !buckets[it.status] || !kindFilter(it) || !keepItem(id)) return;
        var mineR = it.assignments.map(A).filter(function (r) { return r.to === v && r.state === 'live'; })[0];
        buckets[it.status].push({ item: id, drag: mineR && !wrapped(mineR.status) ? id : null,
          sub: (it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return r.to + (r.state === 'pending' ? '?' : ''); }).join(', ') || wsT("js.tk.common.nobody")) });
      });
    } else {
      assignRecs(key, v).forEach(function (r) {
        var col = r.state === 'pending' ? 'pending' : r.status;
        if (!buckets[col]) return;
        buckets[col].push({ item: r.item, rec: r, drag: r.to === v && r.state === 'live' && !wrapped(r.status) ? r.item : null,
          who: key !== 'mine' ? r.to : null, sub: (key !== 'mine' ? esc(nm(r.to)) + ' &middot; ' : '') + source(r) });
      });
    }
    Object.keys(buckets).forEach(function (c) {
      buckets[c].sort(function (x, y) { var a = VS.items[x.item], b = VS.items[y.item]; return b.priorityLevel - a.priorityLevel || ((a.due || '9') < (b.due || '9') ? -1 : 1); });
    });
    var h = '<div class="toolbar">' + (key === 'all' ? itemFilterBar(true) : '') +
      '<span class="muted">' + wsT("js.tk.kanban.hint") + '</span></div><div class="kanban">';
    h += cols.map(function (c) {
      var rv = key === 'all' && c === 'idea', kn = rv && ui.knew[ui.store];
      return '<div class="kcol"' + attr('kcol', c) + '><div class="khead">' + stIcon(c) + '<b>' + esc(SI.info(c).label) + '</b> <span class="muted">' + buckets[c].length + '</span>' +
        (rv ? '<span class="grow"></span>' + ia('knew', 'plus', wsT("js.tk.kanban.new"), {}) : '') + '</div>' + (kn ? knewRowHtml(kn) : '') +
        buckets[c].map(function (x) {
          // quiet cards: title, then initials (the record's assignee off "mine"), priority, due; the source on hover
          var it = VS.items[x.item];
          return '<div class="kcard' + (x.drag ? ' drag' : '') + '"' + (x.drag ? ' draggable="true"' + attr('drag', x.drag) : '') + attr('item', x.item) + attr('rk', 'k:' + c + ':' + (x.rec ? x.rec.id : x.item)) + tipHtml(x.sub) + '>' +
            '<div class="kc-t">' + titleRef(x.item) + '</div>' + moreBtn(x.item, 'kmore', true) + '<div class="kc-m">' + warnHtml(it) + dotHtml(it) + (x.who ? '<span class="who">' + avHtml(x.who) + '</span>' : whoHtml(it)) +
            '<span class="grow"></span>' + priHtml(it) + dueHtml(it, x.rec ? stKey(it, x.rec) : c) + '</div></div>';
        }).join('') + '</div>';
    }).join('');
    return h + '</div>' + lastWrite();
  }

  var FAMILIES = [['all', "js.tk.family.all"], ['item', "js.tk.family.item"], ['assign', "js.tk.family.assign"], ['comment', "js.tk.family.comment"], ['roles', "js.tk.family.roles"]];
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
    var people = [['all', wsT("js.tk.activity.everyone")]].concat(Object.keys(VS.members).map(function (id) { return [id, nmo(id)]; }));
    var h = '<div class="toolbar">' + sel('a-family', f.family, FAMILIES.map(function (x) { return [x[0], wsT(x[1])]; })) + sel('a-member', f.member, people) +
      actChips() + '<span class="grow"></span><span class="muted">' + wsT("js.tk.activity.count", rows.length) + '</span></div>';
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
      .map(function (it) { return { item: it.id, status: it.status, rec: null, who: '' }; });
    return assignRecs(key, v).filter(function (r) { return dated(VS.items[r.item]); }).map(function (r) {
      return { item: r.item, status: stKey(null, r), rec: r, who: key === 'mine' ? '' : nm(r.to) };
    });
  }
  function whoTag(x) { return x.who ? '<span class="muted">' + esc(x.who) + '</span>' : ''; }
  // [first day, day after the last] of an item's own gantt bar, or null without a due date: from
  // its start date, else its first doing, else its creation, to its due day. A start after the due
  // (concurrent edits can fold one) gives a one-day bar at the due.
  function ganttSpan(it) {
    if (!it.due || it.kind === 'topic') return null;
    var e = dayNum(it.due) + 1;
    return [Math.min(dayNum(it.start || it.started || it.ts), e - 1), e];
  }
  // Due-date timeline: the scope's dated rows as the same context tree the list draws (the
  // fold state is shared with it). A row's bar runs from the item's start (ganttSpan) to its due day. A node with children also draws a thin summary bar spanning the
  // range of every dated item below it, whether they show or are folded away; a parent without
  // a due date of its own (a context row, say) is placed by that summary bar alone.
  function ganttBody(key, rows) {
    var bar = key === 'all' ? itemFilterBar() : '';
    if (!rows.length) return '<div class="toolbar">' + bar + '</div><div class="empty">' + wsT("js.tk.gantt.empty") + '</div>';
    var ents = ctxTree(rows, key === 'all' ? { keep: keepKind } : null), below = {};
    // [first day, day after the last] of an item's own bar, or null without a due date
    function own(id) { return ganttSpan(VS.items[id]); }
    function join(a, b) { return !a ? b : !b ? a : [Math.min(a[0], b[0]), Math.max(a[1], b[1])]; }
    function sub(id) { // the range of every dated item below id
      if (!(id in below)) below[id] = kidsShown(id).reduce(function (r, k) { return join(join(r, own(k)), sub(k)); }, null);
      return below[id];
    }
    var t0 = dayNum(today()), lo = t0, hi = t0 + 1;
    ents.forEach(function (e) { var r = e.k === 'node' && join(own(e.id), sub(e.id)); if (r) { lo = Math.min(lo, r[0]); hi = Math.max(hi, r[1]); } });
    var span = hi - lo, pct = function (d) { return ((d - lo) / span * 100).toFixed(2) + '%'; }, wid = function (r) { return (Math.max(1, r[1] - r[0]) / span * 100).toFixed(2) + '%'; };
    var axis = '';
    for (var d = lo; d < hi; d++) axis += '<span class="gtick' + (d === t0 ? ' now' : '') + '" style="left:' + pct(d) + '">' + (new Date(d * 86400000).getUTCDay() === 1 || d === lo ? esc(dayStr(d).slice(5)) : '') + '</span>';
    var h = '<div class="toolbar">' + bar + '<span class="muted">' + wsT("js.tk.gantt.hint", rows.length, esc(today())) + '</span></div>' +
      '<div class="gantt"><div class="grow-axis"><div class="glabel"></div><div class="gtrack axis">' + axis + '</div></div>';
    // x: the dueRows entry of a match, null for a context row; c: the fold control; sm: draw the summary bar
    function row(e, x, c, sm) {
      var it = VS.items[e.id], st = x ? x.status : stKey(it), r = own(e.id), s = sm && e.n ? sub(e.id) : null, over = it.due && it.due < today() && !wrapped(st);
      return '<div class="grow-row click' + (ENDED[st] ? ' ended' : '') + (x ? '' : ' ctx') + '"' + attr('item', it.id) + attr('rk', 'g:' + it.id) + '><div class="glabel" style="--ind:' + e.depth + '">' + c.tw + stBtn(st, it, x && x.rec) + titleRef(it.id) + c.sub + (x ? whoTag(x) : '') + priHtml(it) + moreBtn(it.id, 'gmore', true) + '</div>' +
        '<div class="gtrack"><span class="gnow" style="left:' + pct(t0) + '"></span>' +
        (r ? '<span class="gbar st-' + esc(st) + (over ? ' over' : '') + '" style="left:' + pct(r[0]) + ';width:' + wid(r) + '" title="' +
          esc(it.start ? wsT("js.tk.gantt.bar.start", it.start, it.due) : wsT(it.started ? "js.tk.gantt.bar.started" : "js.tk.gantt.bar.created", String(it.started || it.ts).slice(0, 10), it.due)) + '">' + esc(it.due.slice(5)) + '</span>' : '') +
        (s ? '<span class="gsum" style="left:' + pct(s[0]) + ';width:' + wid(s) + '" title="' + esc(wsT("js.tk.gantt.sum", dayStr(s[0]), dayStr(s[1] - 1))) + '"></span>' : '') + '</div></div>';
    }
    h += ents.map(function (e) {
      if (e.k === 'more') return '<div class="grow-row"><div class="glabel" style="--ind:' + e.depth + '">' + moreRow({ id: e.id, n: e.n, depth: 0 }, 'g:') + '</div><div class="gtrack"></div></div>';
      var c = foldCtl(e, 'tfold', 'g:');
      if (!e.rows.length) return row(e, null, c, true);
      return e.rows.map(function (x, i) { return row(e, x, i ? { tw: '<span class="tw"></span>', sub: '' } : c, !i); }).join('');
    }).join('');
    return h + '</div>';
  }

  // the weekday heads, Sunday first: the calendar view and the range picker share them
  function calDow() {
    return [wsT("js.tk.cal.sun"), wsT("js.tk.cal.mon"), wsT("js.tk.cal.tue"), wsT("js.tk.cal.wed"), wsT("js.tk.cal.thu"), wsT("js.tk.cal.fri"), wsT("js.tk.cal.sat")];
  }
  function calendarBody(key, rows) {
    var m = ui.calMonth[key] || today().slice(0, 7), first = dayNum(m + '-01'), dow = new Date(first * 86400000).getUTCDay();
    var y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7)), days = new Date(Date.UTC(y, mo, 0)).getUTCDate();
    var byDay = {};
    rows.forEach(function (x) { var due = VS.items[x.item].due; (byDay[due] = byDay[due] || []).push(x); });
    var prev = new Date(Date.UTC(y, mo - 2, 1)).toISOString().slice(0, 7), next = new Date(Date.UTC(y, mo, 1)).toISOString().slice(0, 7);
    var n = Object.keys(byDay).filter(function (k) { return k.slice(0, 7) === m; }).reduce(function (s, k) { return s + byDay[k].length; }, 0);
    var h = '<div class="toolbar">' + btn('cal', '&lsaquo;', { month: prev, card: key }) + '<b>' + esc(m) + '</b>' + btn('cal', '&rsaquo;', { month: next, card: key }) +
      btn('cal', wsT("js.tk.cal.today"), { month: today().slice(0, 7), card: key }) + (key === 'all' ? itemFilterBar() : '') + '<span class="muted">' + wsT("js.tk.cal.count", n, esc(m)) + '</span></div>' +
      '<div class="cal">' + calDow().map(function (d) { return '<div class="cdow">' + d + '</div>'; }).join('');
    for (var i = 0; i < dow; i++) h += '<div class="cday off"></div>';
    for (var d = 1; d <= days; d++) {
      var ds = m + '-' + (d < 10 ? '0' : '') + d, list = byDay[ds] || [];
      h += '<div class="cday' + (ds === today() ? ' now' : '') + '"' + attr('day', ds) + '><div class="cnum">' + d + '</div>' + list.map(function (x) {
        return '<div class="citem click' + (ENDED[x.status] ? ' ended' : '') + '"' + attr('item', x.item) + attr('rk', 'c:' + ds + ':' + x.item) + tipHtml(SI.info(x.status).label + ' · ' + VS.items[x.item].priorityName + (x.who ? ' · ' + esc(x.who) : '')) + '>' +
          stBtn(x.status, VS.items[x.item], x.rec) + titleRef(x.item) + moreBtn(x.item, 'cmore', true) + '</div>';
      }).join('') + '</div>';
    }
    return h + '</div>';
  }

  // ------------------------------------------------------- item reference card

  // A tracker card reference: an item rendered from the view state in a few lines,
  // opening its item window (a chat entry can carry one).
  function crefHtml(id) {
    var it = VS.items[id];
    var who = it.assignments.map(A).filter(function (r) { return r.state === 'live' || r.state === 'pending'; }).map(function (r) { return nm(r.to) + (r.state === 'pending' ? '?' : ''); });
    return '<div class="cref click"' + attr('item', id) + attr('cref', id) + ' title="' + wsT("js.tk.cref.tip") + '"><div class="r1">' + stChip(it.status) + prChip(it) + dueChip(it) +
      '<span class="grow"></span><span class="al">' + esc(it.alias) + '</span></div><div class="ct">' + esc(it.title) + '</div>' +
      '<div class="r2">&rarr; ' + esc(who.join(', ') || wsT("js.tk.common.nobody")) + '</div></div>';
  }

  // --------------------------------------------------------------- sync state

  // The store's sync state is one slim ribbon at the bottom of the Items card: a status dot,
  // a short text, and the apply button only while writes are pending; with nothing to do it
  // dims to a quiet line. A click opens the status popover (syncPopHtml) with the diagnostics.
  function unpushedN(vs) { return vs.sync.unpushed.reduce(function (a, c) { return a + c.events.length; }, 0); }
  // "synced N min ago" against the view state's clock
  function syncedAgo(ts) {
    if (!ts) return wsT("js.tk.rib.never");
    var m = Math.floor((Date.parse(VS.now) - Date.parse(ts)) / 60000);
    if (!(m >= 1)) return wsT("js.tk.rib.now");
    return m < 60 ? wsTn("js.tk.rib.min", m) : m < 1440 ? wsTn("js.tk.rib.hour", Math.floor(m / 60)) : wsTn("js.tk.rib.day", Math.floor(m / 1440));
  }
  // A wiki sync failure (a wiki fetch or push the ledger's sync went on without) leaves the
  // store online; it marks the line and keeps it from dimming while it stands.
  function wikiWarns(vs) { return vs.store.remote && vs.sync.wikiWarnings ? vs.sync.wikiWarnings : []; }
  function ribbonHtml() {
    var s = VS.sync, remote = VS.store.remote, n = unpushedN(VS), ww = wikiWarns(VS), tone, txt;
    if (!remote) { tone = n ? 'pend' : 'local'; txt = wsT("js.tk.rib.local") + (n ? ' &middot; ' + wsTn("js.tk.rib.pending", n) : ''); }
    else if (!s.online) { tone = 'off'; txt = wsT("js.tk.sync.offline") + (n ? ' &middot; ' + wsTn("js.tk.rib.topush", n) : ''); }
    else if (n) { tone = 'pend'; txt = wsTn("js.tk.rib.topush", n); }
    else { tone = 'ok'; txt = esc(syncedAgo(s.syncedAt)); }
    if (ww.length && s.online) { tone = 'pend'; txt += ' &middot; ' + wsT("js.tk.sync.wikiwarn"); }
    var open = !!(MN && MN.kind === 'sync');
    return '<div class="ribbon r-' + tone + (n || ww.length ? '' : ' idle') + '"><button type="button" class="rib-main"' + attr('act', 'rib') + attr('fk', 'rib') + ' aria-haspopup="dialog" aria-expanded="' + open + '"' +
      tipAttr(wsT("js.tk.rib.tip") + (!s.online && remote && s.error ? ' (' + s.error + ')' : '') + (ww.length ? ' (' + ww.join('; ') + ')' : '')) + '><span class="sdot"></span><span class="rt">' + txt + '</span></button>' +
      (n ? btn('sync', remote ? wsT("js.tk.sync.now") : wsT("js.tk.sync.closebatch"), {}, 'primary') : '') + '</div>';
  }
  // The status popover: the clone ref, the remote state, the store's counts, the unpushed
  // batch contents, the host's syncTools, and the apply button.
  function syncPopHtml() {
    var vs = VS, s = vs.sync, remote = vs.store.remote, events = unpushedN(vs), tools = X.syncTools ? X.syncTools(ui.store, vs, H) || '' : '';
    var nMem = Object.keys(vs.members).filter(function (id) { return !vs.members[id].removed; }).length;
    var h = '<div class="mh"><b>' + esc(vs.store.label) + '</b></div><div class="spop">';
    h += '<div>' + wsT("js.tk.sync.clone", '<code>' + esc(refOf(vs.viewer.stream)) + '</code>') + '</div>';
    h += '<div>' + (remote ? '<b' + (!s.online && s.error ? tipAttr(s.error) : '') + '>' + (s.online ? wsT("js.tk.sync.online") : wsT("js.tk.sync.offline")) + '</b> &middot; ' +
      wsT("js.tk.sync.synced", esc(fmtTs(s.syncedAt))) + (s.online ? '' : ' &middot; ' + wsT("js.tk.sync.retry")) : wsT("js.tk.sync.noremote")) + '</div>';
    if (!s.online && remote && s.error) h += '<div class="err">' + esc(s.error) + '</div>';
    wikiWarns(vs).forEach(function (w) { h += '<div class="err">' + esc(w) + '</div>'; });
    h += '<div class="muted">' + wsT("js.tk.drawer.stats", nMem, vs.stats.events, vs.stats.commits, vs.stats.flagged) + '</div>';
    h += '<div' + (events ? ' class="unp"' : '') + '>' + wsT("js.tk.sync.unpushed", events) + '</div>';
    if (remote && s.upstream) h += '<div class="muted"' + tipAttr(wsT("js.tk.sync.upstream.tip")) + '>' + wsT("js.tk.sync.upstream", s.upstream) + '</div>';
    if (s.unpushed.length) {
      h += '<div class="sbatch"><b>' + wsT("js.tk.sync.list", s.unpushed.map(function (c) { return c.open ? wsT("js.tk.sync.openn", c.events.length) : wsT("js.tk.sync.commitn", c.events.length); }).join(', ')) + '</b><ul class="plain">' +
        s.unpushed.map(function (c) {
          return '<li><b>' + (c.open ? wsT("js.tk.flag.open") : wsT("js.tk.sync.closed")) + '</b> <span class="muted">' + esc(fmtTs(c.ts)) + '</span>: ' + c.events.map(function (id) {
            var n = EV[id]; return esc(alias(id)) + (n ? ' ' + esc(n.type) : '');
          }).join(', ') + '</li>';
        }).join('') + '</ul><p class="muted">' + wsT("js.tk.sync.batchnote") + '</p></div>';
    }
    return h + '</div><div class="mtools">' + btn('sync', remote ? wsT("js.tk.sync.now") : wsT("js.tk.sync.closebatch"), {}, events ? 'primary' : '') + tools + '</div>';
  }

  // The drawer layout, switched by the window width (layoutOf):
  //   'a'      the Items panel (about two thirds) and one tab pane on the right holding
  //            Notifications, Support agent, Team, Activity, Data, one tab at a time;
  //   'd'      from about 1600px, three columns: Items | Notifications (always open) |
  //            the tab pane holding the remaining tabs;
  //   'narrow' below about 1000px, everything is one tab pane, Items first, with the
  //            Today chips at the top whatever tab is shown.
  // Only the shown tab is rendered. A hidden tab whose signature (tabSig) moved since it
  // was last shown carries a dot, like the dashboard's tab bar (`tdot`).
  // A tab is one icon button: the name is the tooltip and the aria-label, a dot rides its corner.
  // The host's extra tabs (ext.tabs) join the side tabs at mount (mergeTabs).
  // Built-in tab names are table keys (looked up in tabName); a host tab brings its own text.
  var TABS = { items: "js.tk.tab.items", wiki: "js.tk.wk.tab", notify: "js.tk.tab.notify", org: "js.tk.tab.org", activity: "js.tk.tab.activity" };
  var TAB_TIP = {};
  var TAB_ICON = { items: '#tki-tasks', wiki: '#tki-book', notify: '#tki-bell', org: '#tki-team', activity: '#tki-activity' };
  var SIDE = ['notify', 'org', 'activity'], XTAB = {};
  function mergeTabs() {
    (X.tabs || []).forEach(function (t) {
      XTAB[t.key] = t; TABS[t.key] = t.title; TAB_ICON[t.key] = t.icon;
      if (t.tip) TAB_TIP[t.key] = t.tip;
      var at = t.after ? SIDE.indexOf(t.after) : -1;
      if (at < 0) SIDE.push(t.key); else SIDE.splice(at + 1, 0, t.key);
    });
  }
  function tabName(k) { return XTAB[k] ? TABS[k] : wsT(TABS[k]); }
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
  // the store's content is on screen: its drawer is open, or it is the store page
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
    var nav = '<nav class="ptabs" role="tablist" aria-label="' + wsT("js.tk.tab.aria") + '">' + tabs.map(function (k) {
      var on = k === active, dot = !on && tabDot(k, v);
      return '<button type="button" class="ptab" role="tab" aria-selected="' + on + '" aria-label="' + esc(dot ? wsT("js.tk.tab.new", tabName(k)) : tabName(k)) + '"' + attr('act', 'tab') + attr('tab', k) + tipAttr(TAB_TIP[k] || tabName(k)) + '>' +
        '<svg class="i"><use href="' + esc(TAB_ICON[k]) + '"/></svg><span class="tdot' + (dot ? ' on' : '') + '" aria-hidden="true"></span></button>';
    }).join('') + '</nav>';
    var tools = c.tools != null ? c.tools : (c.chip ? '<span class="chip">' + c.chip + '</span>' : '') + '<span class="grow"></span>' + c.extra;
    tabShown(active, v);
    return '<section class="card pane' + (c.cls ? ' ' + c.cls : '') + '" id="' + c.id + '"' + attr('tab', active) + (c.attrs || '') + (c.mode ? attr('view', c.mode) : '') + '><header>' + nav + '</header>' +
      (narrow && active === 'items' ? todayBar() : '') + '<div class="ptools">' + tools + '</div><div class="cbody' + (c.bodyCls ? ' ' + c.bodyCls : '') + '">' + c.body + '</div>' + (active === 'items' || active === 'wiki' ? ribbonHtml() : '') + '</section>';
  }
  // storeTitle is a store's display label with its id, "Label (id)", or
  // the id alone when the store has no label (StoreInfo.label then
  // repeats the name); storeTitleHtml mutes the id.
  function storeTitle(st) { return st.label && st.label !== st.name ? st.label + ' (' + st.name + ')' : st.name; }
  function storeTitleHtml(st) { return st.label && st.label !== st.name ? '<b>' + esc(st.label) + '</b> <span class="muted">(' + esc(st.name) + ')</span>' : '<b>' + esc(st.name) + '</b>'; }
  function drawerHtml(name, v) {
    var vs = VS;
    // the Items panel holds the items or, under the header's Items | Wiki switch, the wiki
    var MAKE = { items: function (v) { return AD.wiki && wk().mode === 'wiki' ? cardWiki(v, false) : cardPanel(v); }, wiki: function (v) { return cardWiki(v, true); }, notify: cardNotify, org: cardTeam, activity: cardActivity };
    var h = '<div class="tk-title">' + storeTitleHtml(vs.store) + storeMenuBtn(name) + '<span class="grow"></span>' +
      // the drawer links to the store page; the page links back to the session view
      (ui.page ? '<button class="btn" data-act="page-back"' + tipAttr(wsT("js.tk.drawer.back.tip")) + '><svg class="i"><use href="#tki-back"/></svg>' + wsT("js.tk.drawer.back") + '</button>' :
        '<a class="btn ib" href="#/' + esc(encodeURIComponent(name)) + '" aria-label="' + wsT("js.tk.drawer.page") + '"' + tipAttr(wsT("js.tk.drawer.page.tip", name)) + '><svg class="i"><use href="#tki-page"/></svg></a>') + '</div>';
    if (!vs.viewer.isMember) h += '<p class="err">' + wsT("js.tk.drawer.notmember", esc(v)) + '</p>';
    var L = ui.layout = layoutOf(), ts = tabState(), specs = {};
    function make(k) { return (specs[k] = specs[k] || (MAKE[k] ? MAKE[k](v) : XTAB[k].make(name, VS, H))); }
    if (L === 'narrow') {
      var all = ['items'].concat(AD.wiki ? ['wiki'] : []).concat(SIDE), act = all.indexOf(ts.narrow) >= 0 ? ts.narrow : 'items';
      make(act);
      return h + '<div class="tk-zones" data-layout="narrow"><div class="tk-col">' + paneHtml(all, act, v, specs, true) + '</div></div>';
    }
    var tabs = L === 'd' ? SIDE.slice(1) : SIDE, active = tabs.indexOf(ts.side) >= 0 ? ts.side : tabs[0];
    make(active);
    h += '<div class="tk-zones" data-layout="' + L + '"><div class="tk-col tk-main">' + panelHtml(make('items'), true) + '</div>';
    if (L === 'd') { h += '<div class="tk-col">' + cardHtml(make('notify')) + '</div>'; tabShown('notify', v); }
    tabShown('items', v);
    return h + '<div class="tk-col">' + paneHtml(tabs, active, v, specs, false) + '</div></div>';
  }

  // ------------------------------------------------------------ item window

  function commentHtml(n, v, it) {
    var p = n.payload, c = VS.comments[n.id] || {}, body = c.body != null ? c.body : p.body, reply = '';
    if (p.reply_to) {
      var q = VS.comments[p.reply_to] || (EV[p.reply_to] && { author: EV[p.reply_to].member, body: EV[p.reply_to].payload.body });
      reply = '<div class="quote"' + tipAttr(alias(p.reply_to)) + '>&#8617; ' + (q ? '<b>' + whoName(q.author) + '</b>: ' + (q.hidden ? '<i>' + wsT("js.tk.cmt.hiddenq") + '</i>' : esc(String(q.body).slice(0, 90))) : '') + '</div>';
    }
    var acts = '';
    if (n.status === 'applied' && !c.hidden && !it.hidden) {
      acts += btn('reply', wsT("js.tk.cmt.reply"), { cid: n.id });
      if (n.member === v) acts += btn('cedit', wsT("js.tk.cmt.edit"), { cid: n.id }) + btn('chide', wsT("js.tk.cmt.hide"), { cid: n.id });
    }
    var live = n.status === 'applied' && !!VS.comments[n.id], ed = ui.cedit && ui.cedit.cid === n.id && live && !c.hidden;
    if (ed) acts = '';
    // an own comment being edited in place (ui.cedit): Cmd/Ctrl+Enter saves, Esc cancels
    var txt = ed ? '<div class="cedit">' + editorHtml('tkcedit', { value: ui.cedit.body, rows: 3, label: wsT("js.tk.cmt.edit"), pv: 'ctext' }) + '<div class="tb">' +
      btn('cedit-save', wsT("js.tk.dlg.save"), {}, 'primary') + btn('cedit-x', wsT("js.tk.dlg.cancel"), {}) + '<span class="muted">' + wsT("js.tk.cedit.hint", MAC ? '⌘Enter' : 'Ctrl+Enter') + '</span></div></div>' + mentionChips(c.mentions) + attChips(c.attachments, true) : c.hidden ? '<div class="hiddenc">' + wsT("js.tk.cmt.hidden", esc(alias(c.hidden))) + '<details><summary>' + wsT("js.tk.cmt.history") + '</summary>' + esc(body) + '</details></div>' :
      textBlock('ctext', 'comments', n.id, body) + mentionChips(live ? c.mentions : p.mentions) + attChips(live ? c.attachments : p.attachments, live);
    return '<li class="cmt' + (n.status !== 'applied' ? ' ' + n.status : '') + (n.markers.length ? ' concm' : '') + (n.member === v ? ' mine' : '') + '"' + attr('ev', n.id) + tipAttr(n.alias) + '>' +
      '<div class="ch"><b>' + whoName(n.member) + '</b>' + originChip(n.origin) + '<span class="when">' + esc(fmtTs(n.ts)) + '</span>' +
      (c.edited ? '<span class="chip">' + wsT("js.tk.cmt.edited") + '</span>' : '') + flags(n) + '<span class="grow"></span>' + acts + '</div>' + reply + txt +
      (n.reason ? '<span class="reason">' + esc(n.reason) + '</span>' : '') + markerHtml(n) + supHtml(n) + '</li>';
  }

  // The item window: what the thread sheet and the row detail overlay held, in a
  // centred window over the page. Its actions are the header's context menu (the "⋯"
  // button, a right-click on the header, the status icon); no button row stands between
  // the header and the body. The assignees table keeps its per-assignee actions.
  // What an event refers to inside the open item window, found by the event's type:
  // a comment (added, edited, or hidden) -> that comment; a status, close, confirm, reject,
  // or reopen -> that assignee's row in Assignees; an assignment or handoff -> the holders
  // line of the property card; an accept -> the accepter's row; a decline, return, or unassign ->
  // the assignees row that ended (the holders line no longer lists it), or that member's row when
  // the list hid the ended record (visibleRecs); a CC add or removal -> that member's CC row, else
  // the CC section; a body edit or the
  // item's creation -> the body; an edit that only adds attachments -> the first new chip.
  // Anything else falls back to its own line in the thread, and null when the window has none.
  function focusTarget(root, evId) {
    var e = EV[evId];
    if (!e) return null;
    var p = e.payload || {}, by = function (sel, id) { return root.querySelector('[' + sel + '="' + CSS.escape(id) + '"]'); }, holders = root.querySelector('[data-focus="holders"]'), el = null;
    if (e.type === 'comment.add') el = by('data-ev', e.id);
    else if (e.type === 'comment.edit' || e.type === 'comment.hide') el = by('data-ev', p.comment);
    else if (STATUS_EV[e.type]) el = by('data-hmember', assigneeOf(e)) || holders;
    else if (e.type === 'item.assign' || e.type === 'item.handoff' || e.type === 'item.claim' || e.type === 'item.approve') el = holders;
    else if (e.type === 'item.request' || e.type === 'item.deny' || e.type === 'item.unrequest') el = by('data-req', e.type === 'item.request' ? e.id : p.request);
    else if (e.type === 'item.open' || e.type === 'item.unopen') el = root.querySelector('[data-focus="opening"]');
    else if (e.type === 'item.cc' || e.type === 'item.uncc') el = by('data-ccmember', p.member) || root.querySelector('[data-focus="cc"]');
    else if (e.type === 'item.accept') el = by('data-hmember', e.member) || holders;
    else if (e.type === 'item.decline' || e.type === 'item.return' || e.type === 'item.unassign') {
      var gone = vals(VS.assignments).filter(function (r) { return r.ended && r.ended.id === e.id; })[0];
      el = gone && (by('data-rec', gone.id) || by('data-hmember', gone.to));
    }
    else if (e.type === 'item.create' || e.type === 'item.edit') {
      var atts = listOf(p.attachments).filter(function (x) { return x && x.oid; });
      el = !('body' in p) && atts.length ? by('data-oid', atts[0].oid) : root.querySelector('[data-focus="body"]');
    }
    el = el || by('data-ev', e.id);
    if (el && el.closest('.arow.more') && ui.aopen !== ui.item) { // the target sits in the folded rows: expand them
      ui.aopen = ui.item;
      var lst = root.querySelector('.alist'), tg = root.querySelector('[data-act="afold"]');
      if (lst) lst.classList.remove('folded');
      if (tg) tg.textContent = wsT("js.tk.asg.fewer");
    }
    return el;
  }
  // The item window's parts. The window is a header (title, chips, actions) over two
  // columns: the main one (body, subtasks, thread, composer) and a narrow side one (the property
  // card, the assignees). Below 820px of window width it is one column, in the order header ->
  // property card -> body -> subtasks -> assignees -> thread -> composer (CSS `order`).
  // How far up the lead chain a member is: 0 for a root, 1 for a root's follower, and so on.
  function leadDepth(id) {
    var d = 0, m = VS.members[id];
    while (m && (m.leads || [])[0] && d < 50) { d++; m = VS.members[m.leads[0]]; }
    return d;
  }
  var A_SHOWN = 4; // assignee rows shown before the "N more" toggle
  // The holders in one line: the count, then the live statuses other than todo, then pending.
  function holdersSummary(it) {
    var hs = holders(it), c = {}, parts = [];
    if (!hs.length) return '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>';
    hs.forEach(function (r) { var k = r.state === 'pending' ? 'pending' : r.status; c[k] = (c[k] || 0) + 1; });
    ['blocked', 'doing', 'in-review', 'done', 'dropped', 'pending'].forEach(function (k) { if (c[k]) parts.push(esc(SI.info(k).label) + ' ' + c[k]); });
    return '<b>' + hs.length + '</b>' + parts.map(function (x) { return ' <span class="muted">&middot;</span> ' + x; }).join('');
  }
  function ancestorsOf(it) {
    var out = [], cur = it.parent;
    while (cur && VS.items[cur] && out.length < 50) { out.unshift(cur); cur = VS.items[cur].parent; }
    return out;
  }
  // The property card's labels: an editor's are a button that opens the item menu's labels step
  // (labelStepHtml), where a toggle writes at once; anyone else's only show.
  function labelsProp(it) {
    var cs = labelChips(it);
    if (!canEdit(it)) return cs || '<span class="muted">-</span>';
    var names = (it.labels || []).map(labelOf).filter(Boolean).map(function (l) { return l.name; }).join(', '), t = wsT("js.tk.prop.labels.edit");
    return '<button type="button" class="plab" id="iw-plabels"' + attr('act', 'labels') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(names ? t + ': ' + names : t) + '"' + tipAttr(t) + '>' +
      (cs || '<span class="muted">' + wsT("js.tk.prop.labels.add") + '</span>') + '</button>';
  }
  function propertyCard(it) {
    var hs = holders(it), topic = it.kind === 'topic';
    return '<section class="pcard" aria-label="' + wsT("js.tk.prop.aria") + '"><h3>' + wsT("js.tk.prop.title") + '</h3><dl>' +
      '<dt>' + wsT("js.tk.prop.status") + '</dt><dd>' + (topic ? '<span class="muted">' + wsT("js.tk.prop.topic") + '</span>' : esc(SI.info(it.status).label) + ' <span class="muted">' + esc(SI.info(it.status).desc) + '</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.kind") + '</dt><dd>' + esc(kindName(it.kind)) + (it.pending.length ? ' <span class="muted">' + wsT("js.tk.prop.pending") + '</span>' : '') + (it.hidden ? ' <span class="muted">' + wsT("js.tk.prop.hidden") + '</span>' : '') + '</dd>' +
      (topic ? '<dt>' + wsT("js.tk.prop.holder") + '</dt><dd' + attr('focus', 'holders') + '>' + (holderNames(it) || '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>') + '</dd>' :
        '<dt>' + wsT("js.tk.prop.holders") + '</dt><dd' + attr('focus', 'holders') + '>' + holdersSummary(it) + '</dd>') +
      (it.chain.length > 1 ? '<dt>' + wsT("js.tk.prop.path") + '</dt><dd>' + esc(it.chain.join(' > ')) + '</dd>' : '') +
      (topic ? '' : '<dt>' + wsT("js.tk.prop.due") + '</dt><dd>' + (it.due || it.start ? esc(it.start && it.due ? it.start + ' – ' + it.due : it.due || wsT("js.tk.due.from", it.start)) : '<span class="muted">-</span>') + '</dd>') +
      '<dt>' + wsT("js.tk.prop.creator") + '</dt><dd>' + whoName(it.author) + (VS.members[it.author] && VS.members[it.author].admin ? ' <span class="chip admin">' + wsT("js.tk.team.admin") + '</span>' : '') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.project") + '</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.labels") + '</dt><dd>' + labelsProp(it) + '</dd>' +
      '<dt>' + wsT("js.tk.prop.parent") + '</dt><dd>' + (it.parent ? itemRef(it.parent) + ' ' + esc((VS.items[it.parent] || {}).title || '') : '<span class="muted">-</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.id") + '</dt><dd><code>' + esc(it.id) + '</code></dd>' +
      '<dt>' + wsT("js.tk.prop.dates") + '</dt><dd>' + wsT("js.tk.row.created", esc(fmtTs(it.ts))) + (it.edited ? '<br>' + wsT("js.tk.row.edited", esc(fmtTs(it.edited.ts))) : '') + '<br>' + wsT("js.tk.row.activity", esc(fmtTs(it.lastActivity))) + '</dd></dl></section>';
  }
  // Assignees: at most A_SHOWN rows, the highest in the lead chain first (shallowest lead depth;
  // then live before ended, then display name, then assignment order). What every row shares (the
  // provenance text and the close rule) is written once in the header; a row adds only what
  // differs. The rows past A_SHOWN are in the markup, hidden until the "N more" toggle (ui.aopen).
  // The assignees card's "+": who may assign sees it (the menu's rule: the creator, and on a topic
  // only while nobody holds it, as the store refuses a second holder). It opens the assign
  // popover (asgHtml, the item menu's assign step) anchored at itself.
  function canAssign(it, v) { return it.author === v && !it.hidden && (it.kind !== 'topic' || !holders(it).length); }
  function assignAddBtn(it, v) {
    if (!canAssign(it, v)) return '';
    return '<button type="button" class="ia a-add"' + attr('act', 'asg-add') + attr('item', it.id) + ' aria-haspopup="dialog" aria-label="' + esc(wsT("js.tk.act.assign")) + '"' + tipAttr(wsT("js.tk.act.assign")) + '><svg class="i"><use href="#tki-plus"/></svg></button>';
  }
  function openAssign(id, anchor) { itemMenu(id, anchor, null, { step: 'assign' }); }
  // A remove-a-person action on an assignee row (unassign, withdraw): a small red minus badge.
  function rmBadge(label, r) {
    return '<button type="button" class="rmb"' + attr('act', 'unassign') + attr('item', r.item) + attr('who', r.to) + ' aria-label="' + esc(label) + '"' + tipAttr(label) + '><svg class="i"><use href="#tki-minus"/></svg></button>';
  }
  // The records the assignees list shows: every live or pending record, and for a member who
  // holds neither only that member's latest ended record (latest by the parsed end time, ties to
  // the later record), so unassigning and reassigning never stacks rows. The member is the
  // record's `to`. The fold keeps every ended record; only this list hides them.
  function visibleRecs(recs) {
    var on = {}, last = {};
    recs.forEach(function (r) { if (r.state === 'live' || r.state === 'pending') on[r.to] = true; });
    recs.forEach(function (r, i) {
      if (r.state === 'live' || r.state === 'pending' || on[r.to]) return;
      var t = Date.parse(r.ended && r.ended.ts), b = last[r.to];
      if (isNaN(t)) t = -Infinity;
      if (!b || t >= b.t) last[r.to] = { r: r, t: t };
    });
    return recs.filter(function (r) { return r.state === 'live' || r.state === 'pending' || (last[r.to] && last[r.to].r === r); });
  }
  function assigneesHtml(v, it, recs) {
    recs = visibleRecs(recs);
    if (!recs.length) return '<section class="iw-asg"><h3>' + wsT("js.tk.asg.title") + '<span class="grow"></span>' + assignAddBtn(it, v) + '</h3><div class="empty">' + wsT("js.tk.asg.none") + '</div></section>';
    var topic = it.kind === 'topic'; // a topic's record is its holder: no completion, no status
    var rows = recs.map(function (r, i) { return { r: r, i: i, d: leadDepth(r.to), on: r.state === 'live' || r.state === 'pending', src: source(r), cm: topic ? null : complText(r.completion) }; });
    rows.sort(function (a, b) { return (b.on - a.on) || (a.d - b.d) || nm(a.r.to).localeCompare(nm(b.r.to)) || (a.i - b.i); });
    function common(key) { // the most frequent value, when at least two rows hold it
      var n = {}, best = null;
      rows.forEach(function (x) { n[x[key]] = (n[x[key]] || 0) + 1; if (n[x[key]] > 1 && (best === null || n[x[key]] > n[best])) best = x[key]; });
      return best;
    }
    var cs = common('src'), cc = common('cm'), open = ui.aopen === it.id, hid = rows.length - A_SHOWN, reqT = {};
    reqTargets(it).forEach(function (r) { reqT[r.id] = true; });
    var head = cs !== null || cc !== null ? '<div class="a-common muted">' + [cs, cc].filter(function (x) { return x !== null; }).join(' &middot; ') + '</div>' : '';
    var list = rows.map(function (x, k) {
      var r = x.r, b = '', rm = '';
      if (r.state === 'live' && topic && r.to === v) {
        // the holder releases the topic (unassigns their own record) or hands it off
        if (!it.hidden) b += btn('unassign', wsT("js.tk.act.release"), { item: r.item, who: v }) + btn('handoff', esc(wsT("js.tk.act.handoff")), { item: r.item });
      } else if (r.state === 'live') {
        if ((r.by === v || r.to === v) && wrapped(r.status)) b += btn('note', wsT("js.tk.asg.reopen"), { item: r.item, who: r.to, op: 'item.reopen' });
        if (!it.hidden && canReview(r)) b += btn('confirm', esc(wsT("js.tk.act.confirm", nm(r.to))), { item: r.item, who: r.to }, 'primary') + btn('note', esc(wsT("js.tk.act.reject", nm(r.to))), { item: r.item, who: r.to, op: 'item.reject' });
        var wa = wrapAuth(it, r);
        if (wa) b += btn('wrap', esc(wsT(wa === 'sup' ? "js.tk.act.wrap" : "js.tk.act.drop", nm(r.to))), { item: r.item, who: r.to });
        var departed = (VS.members[r.to] || {}).removed || (VS.members[r.by] || {}).removed;
        // a fallback holding (nobody else holds the task) is not unassigned or handed off: assign instead
        if (!r.fallback && (r.by === v || (departed && VS.viewer.admin))) rm = rmBadge(r.by === v ? wsT("js.tk.asg.unassign") : wsT("js.tk.asg.unassign.admin"), r);
        if (VS.viewer.admin && r.to !== v && !wrapped(r.status) && !r.fallback && !it.hidden) b += btn('handoff', wsT("js.tk.asg.handoff.admin"), { item: r.item, who: r.to });
        if (reqT[r.id]) b += btn('request', esc(wsT("js.tk.asg.request")), { item: r.item, who: r.to });
      } else if (r.state === 'pending' && r.to === v && (r.by === v || r.sender === v)) rm = rmBadge(wsT("js.tk.asg.withdraw"), r);
      var note = r.note || r.answerNote, diff = [];
      if (x.src !== cs) diff.push(cs !== null && x.src.indexOf(cs) === 0 ? x.src.slice(cs.length).trim() : x.src);
      if (x.cm !== cc && x.cm !== null) diff.push(x.cm);
      return '<li class="arow' + (!x.on ? ' gone' : '') + (k >= A_SHOWN ? ' more' : '') + '"' + attr('rec', r.id) + attr('hmember', r.to) + '>' +
        '<div class="ar-main">' + avHtml(r.to, r.state === 'pending') + '<span class="ar-name">' + whoName(r.to) + '</span>' +
        '<span class="ar-st">' + (topic && r.state === 'live' && !r.ended ? '<span class="chip">' + wsT("js.tk.asg.holder") + '</span>' : recChip(r)) + (r.ended ? ' <span class="muted">' + esc(alias(r.ended.id)) + endedBy(r) + '</span>' : '') + rm + '</span></div>' +
        (note ? '<div class="ar-note">&ldquo;' + esc(note) + '&rdquo;</div>' : '') +
        (diff.length ? '<div class="ar-diff muted">' + diff.join(' &middot; ') + '</div>' : '') +
        (b ? '<div class="tb">' + b + '</div>' : '') + '</li>';
    }).join('');
    return '<section class="iw-asg"><h3>' + wsT("js.tk.asg.title") + ' <span class="cnt">' + rows.length + '</span><span class="grow"></span>' + assignAddBtn(it, v) + '</h3>' + head +
      '<ul class="alist' + (open ? '' : ' folded') + '">' + list + '</ul>' +
      (hid > 0 ? '<div class="a-fold">' + btn('afold', open ? wsT("js.tk.asg.fewer") : wsT("js.tk.asg.more", hid), {}) + '</div>' : '') + '</section>';
  }
  // CC in the item window: the item's CC'd members (it.cc, in CC order), each with a remove
  // badge, and a "+" that opens the item menu's people step (item.cc). Any member may add or
  // remove anyone; a hidden item takes neither, and shows its list only when it has one.
  function ccHtml(it) {
    var cc = listOf(it.cc), live = !it.hidden;
    if (!cc.length && !live) return '';
    var add = live ? '<button type="button" class="ia a-add"' + attr('act', 'cc') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.cc.add")) + '"' + tipAttr(wsT("js.tk.cc.add")) + '><svg class="i"><use href="#tki-plus"/></svg></button>' : '';
    return '<section class="iw-asg iw-cc"' + attr('focus', 'cc') + '><h3>' + wsT("js.tk.cc.title") + (cc.length ? ' <span class="cnt">' + cc.length + '</span>' : '') + '<span class="grow"></span>' + add + '</h3>' +
      (cc.length ? '<ul class="alist">' + cc.map(function (id) {
        var rm = live ? '<button type="button" class="rmb"' + attr('act', 'uncc') + attr('item', it.id) + attr('who', id) + ' aria-label="' + esc(wsT("js.tk.cc.remove", nm(id))) + '"' + tipAttr(wsT("js.tk.cc.remove", nm(id))) + '><svg class="i"><use href="#tki-minus"/></svg></button>' : '';
        return '<li class="arow"' + attr('ccmember', id) + '><div class="ar-main">' + avHtml(id) + '<span class="ar-name">' + whoName(id) + '</span><span class="ar-st">' + rm + '</span></div></li>';
      }).join('') + '</ul>' : '<div class="empty">' + wsT("js.tk.cc.none") + '</div>') + '</section>';
  }
  // Taking work in the item window: the opening (scope, opener, and Claim / Withdraw when the
  // viewer may) and the take requests on the item, pending first, each with the answers the
  // viewer may give. Drawn only when the item is open or has requests.
  var REQ_ST = { pending: "js.tk.req.st.pending", approved: "js.tk.req.st.approved", denied: "js.tk.req.st.denied", withdrawn: "js.tk.req.st.withdrawn", lapsed: "js.tk.req.st.lapsed" };
  function takeHtml(v, it) {
    var o = it.open, qs = reqsOf(it).slice().sort(function (a, b) { return (b.state === 'pending') - (a.state === 'pending') || (a.ts < b.ts ? 1 : a.ts > b.ts ? -1 : 0); });
    if (!o && !qs.length) return '';
    var h = '<section class="iw-asg iw-take"><h3>' + wsT("js.tk.take.title") + (qs.length ? ' <span class="cnt">' + qs.length + '</span>' : '') + '</h3><ul class="alist">';
    if (o) {
      var b = (it.claimable ? btn('claim', esc(wsT("js.tk.act.claim")), { item: it.id }, 'primary') : '') + (canUnopen(it) ? btn('unopen', esc(wsT("js.tk.act.unopen")), { item: it.id }) : '');
      h += '<li class="arow"' + attr('focus', 'opening') + '><div class="ar-main">' + openChip(it) + '<span class="ar-name">' + (o.scope === 'store' ? wsT("js.tk.take.open.store", whoName(o.by)) : wsT("js.tk.take.open.team", whoName(o.by))) + '</span>' +
        '<span class="ar-st muted">' + esc(fmtTs(o.ts)) + '</span></div>' + (!it.claimable && o.by !== v ? '<div class="ar-diff muted">' + wsT("js.tk.open.tip.notmine") + '</div>' : '') + (b ? '<div class="tb">' + b + '</div>' : '') + '</li>';
    }
    h += qs.map(function (q) {
      var b = '';
      if (canApprove(it, q)) b += btn('approve', esc(wsT("js.tk.req.approve")), { item: it.id, req: q.id }, 'primary');
      if (canAnswer(it, q)) b += btn('note', esc(wsT("js.tk.req.deny")), { item: it.id, who: q.by, req: q.id, op: 'item.deny' });
      if (q.state === 'pending' && q.by === v) b += btn('unrequest', esc(wsT("js.tk.req.withdraw")), { item: it.id, req: q.id });
      return '<li class="arow' + (q.state === 'pending' ? '' : ' gone') + '"' + attr('req', q.id) + '><div class="ar-main">' + avHtml(q.by, q.state === 'pending') +
        '<span class="ar-name">' + wsT("js.tk.req.line", whoName(q.by), whoName(q.assignee)) + '</span><span class="ar-st"><span class="chip' + (q.state === 'pending' ? ' req' : ' ended') + '">' + esc(wsT(REQ_ST[q.state] || "js.tk.req.st.pending")) + '</span>' +
        (q.ended ? ' <span class="muted">' + esc(alias(q.ended.id)) + '</span>' : '') + '</span></div>' +
        (q.note ? '<div class="ar-note">&ldquo;' + esc(q.note) + '&rdquo;</div>' : '') + (q.answerNote ? '<div class="ar-note muted">&ldquo;' + esc(q.answerNote) + '&rdquo; (' + esc(nm(q.ended ? q.ended.by : '')) + ')</div>' : '') +
        (b ? '<div class="tb">' + b + '</div>' : '') + '</li>';
    }).join('');
    return h + '</ul></section>';
  }
  // Subtasks: the item's whole subtree, each row opening that item. Every node with children folds
  // with ▸/▾: the first level (the item's children) starts open, so their children show, and deeper
  // levels start folded. The fold state is the page's (ui.wfold), apart from the lists' stored one.
  // A node with children also carries its roll-up.
  // The section is drawn even without children when the viewer may add one (its "+" opens the
  // inline row here); a topic without children and without that right draws nothing.
  function subtasksHtml(it) {
    var add = ui.sub && ui.sub.rk === 'iw:' + it.id ? subRowHtml(0) : ''; // "add subtask" (header, menu, or the section's "+"): the input row tops the list
    var can = canSplit(it);
    if (!it.children.length && !add && !can && it.kind === 'topic') return '';
    function rows(id, depth) {
      return VS.items[id].children.map(function (k) {
        var ki = VS.items[k], key = ui.store + '|' + k, open = !!ki.children.length && (key in ui.wfold ? ui.wfold[key] : depth === 0);
        var c = foldCtl({ id: k, n: ki.children.length, open: open, to: '' }, 'wfold', 'w:');
        return '<div class="srow' + (wrapped(ki.status) ? ' gone' : '') + '"' + attr('item', k) + attr('rk', 'w:' + k) + ' style="--ind:' + depth + '">' + c.tw + stBtn(stKey(ki), ki) + titleRef(k) + c.sub + (ki.hidden ? '<span class="chip hid">' + wsT("js.tk.chip.hidden") + '</span>' : '') +
          '<span class="grow"></span>' + rollupHtml(ki) + whoHtml(ki) + '</div>' + (open ? rows(k, depth + 1) : '');
      }).join('');
    }
    var plus = can ? '<span class="grow"></span><button type="button" class="ia"' + attr('act', 'hsub') + attr('item', it.id) + ' aria-label="' + esc(wsT("js.tk.menu.sub")) + '"' + tipAttr(wsT("js.tk.menu.sub")) + '><svg class="i"><use href="#tki-plus"/></svg></button>' : '';
    return '<section class="iw-sub"><h3 class="prom">' + wsT("js.tk.win.subtasks") + ' <span class="cnt">' + it.children.length + '</span>' + rollupHtml(it) + plus + '</h3>' +
      (add || it.children.length ? '<div class="srows">' + add + rows(it.id, 0) + '</div>' : '') + '</section>';
  }
  // "Referenced by" (531): the items, then the comments, whose text references this one with a
  // #<alias> token (view backrefs), drawn only when there are any. An item row opens that item;
  // a comment row opens its own item's window at the comment.
  function backrefsHtml(it) {
    var l = listOf(it.backrefs).filter(function (id) { return VS.items[id] || VS.comments[id]; });
    if (!l.length) return '';
    return '<section class="iw-brefs"><h3 class="prom">' + wsT("js.tk.win.backrefs") + ' <span class="cnt">' + l.length + '</span></h3><div class="brows">' + l.map(function (id) {
      var x = VS.items[id], c = !x && VS.comments[id], host = c && VS.items[c.item];
      if (x) return '<a href="#" class="brow"' + attr('item', id) + '>' + stIcon(stKey(x)) + '<span class="bt">' + esc(x.title) + '</span><code class="al">' + esc(x.alias) + '</code></a>';
      return '<a href="#" class="brow"' + attr('act', 'bref') + attr('item', c.item) + attr('cid', id) + '><span class="sti"><svg class="i"><use href="#tki-chat"/></svg></span>' +
        '<span class="bt">' + esc(host ? host.title : alias(c.item)) + '</span><span class="bs">' + wsT("js.tk.bref.cmt", esc(nm(c.author))) + ' ' + snippet(c.body) + '</span></a>';
    }).join('') + '</div></section>';
  }
  // The edit mode (ui.iedit) turns the title and the body into fields in place: Enter in the
  // title or Cmd/Ctrl+Enter in the body saves, Esc cancels.
  function windowHtml(v, it) {
    var recs = it.assignments.map(A), up = ancestorsOf(it), ed = ui.iedit && ui.iedit.item === it.id ? ui.iedit : null;
    var h = '<div class="iw-top">' + (up.length ? '<nav class="iw-crumb" aria-label="' + wsT("js.tk.win.parents") + '">' + up.map(function (k) { return '<a href="#"' + attr('item', k) + '>' + esc(VS.items[k].title) + '</a>'; }).join('<span aria-hidden="true">&rsaquo;</span>') + '</nav>' : '') +
      '<div class="iw-head"' + attr('mitem', it.id) + attr('rk', 'iw:' + it.id) + '>' + stBtn(stKey(it), it) +
      (ed ? '<h2 id="iw-title" class="iw-edt"><input type="text" id="iw-etitle" data-keep autocomplete="off" value="' + esc(ed.title) + '" aria-label="' + wsT("js.tk.form.title") + '"></h2>' : '<h2 id="iw-title">' + esc(it.title) + '</h2>') +
      headActs(it) + moreBtn(it.id, 'iw-more', true) +
      '<button type="button" class="ia iw-x" data-act="win-close" aria-label="' + wsT("js.tk.win.close") + '"' + tipAttr(wsT("js.tk.win.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>' +
      '<div class="iw-chips">' + kindChip(it) + idCopyBtn(it) + (it.kind === 'topic' ? '' : stChip(it.status) + (canEdit(it) ? '' : prChip(it)) + dueChip(it) + rollupHtml(it)) + openChip(it) + labelChips(it, 'lsearch') + warnHtml(it) + '</div></div>';
    h += '<div class="iw-grid"><div class="iw-main">';
    if (ed) h += '<div class="iw-body iw-edit">' + editorHtml('iw-ebody', { value: ed.body, rows: 5, label: wsT("js.tk.form.body"), pv: 'sh-body' }) +
      '<input type="text" id="iw-eproj" data-keep autocomplete="off" value="' + esc(ed.project) + '" placeholder="' + wsT("js.tk.form.project") + '" aria-label="' + wsT("js.tk.form.project") + '">' + labelPickHtml(ed) +
      (ed.restored ? '<div class="muted" role="status">Draft restored ' + btn('iedit-discard', 'Discard draft', {}) + '</div>' : '') +
      '<div class="tb">' + btn('iedit-save', wsT("js.tk.dlg.save"), {}, 'primary') + btn('iedit-x', wsT("js.tk.dlg.cancel"), {}) + '<span class="muted">' + wsT("js.tk.edit.hint") + '</span></div></div>';
    else if (it.body || it.mentions.length || it.attachments.length)
      h += '<div class="iw-body">' + (it.body ? textBlock('sh-body', 'items', it.id, it.body, attr('focus', 'body')) : '') + mentionChips(it.mentions) + attChips(it.attachments, true) + '</div>';
    h += subtasksHtml(it) + backrefsHtml(it);
    var evs = it.thread.map(function (id) { return EV[id]; });
    h += '<section class="iw-thread"><h3 class="prom">' + wsT("js.tk.win.thread") + ' <span class="cnt">' + evs.length + '</span></h3><ul class="timeline">' + evs.map(function (n) {
      if (n.type === 'comment.add' && n.thread === it.id) return commentHtml(n, v, it);
      return evLine(n, n.parentThread === it.id ? 'parent' : true);
    }).join('') + '</ul></section>';

    // composer: a comment (optionally a quote reply); the status moved to the header's menu.
    // The box carries the @ button (bottom left) and the send button (bottom right); Cmd/Ctrl+Enter sends.
    // Typing @ (or the button) opens the member suggestion popup at the caret (see mentionSync).
    // The box is the shared editor (editorHtml) with the bar below its text.
    var rt = ui.replyTo && VS.comments[ui.replyTo] && VS.comments[ui.replyTo].item === it.id ? VS.comments[ui.replyTo] : null;
    var send = wsT("js.tk.composer.send", MAC ? '⌘Enter' : 'Ctrl+Enter');
    h += '<div class="composer2">' + (it.hidden ? '<span class="muted">' + wsT("js.tk.composer.hidden") + '</span>' : '') +
      (rt ? '<div class="replying">' + wsT("js.tk.composer.replying", esc(alias(rt.id)), esc(nm(rt.author)), esc(String(rt.body).slice(0, 70))) + ' ' + btn('reply-x', 'x', {}) + '</div>' : '') +
      draftHtml() +
      editorHtml('cbox', { cls: 'cbox', rows: 2, placeholder: wsT("js.tk.composer.placeholder", esc(nm(v))), pv: 'ctext', foot: '<div class="cbar">' +
      '<button type="button" id="cmention" class="cat"' + attr('act', 'mention') + ' aria-label="' + wsT("js.tk.composer.mention") + '"' + tipAttr(wsT("js.tk.composer.mention.tip")) + '>@</button>' +
      (CAPS.origin ? '<select id="corigin" title="' + wsT("js.tk.origin") + '"><option value="human">' + wsT("js.tk.origin.human") + '</option><option value="agent">' + wsT("js.tk.origin.agent") + '</option><option value="agent-auto">' + wsT("js.tk.origin.auto") + '</option></select>' : '') +
      '<span class="grow"></span><button type="button" class="csend" id="tkcsend"' + attr('act', 'comment') + attr('item', it.id) + ' aria-label="' + esc(send) + '"' + tipAttr(send) + '><svg class="i"><use href="#tki-send"/></svg></button></div>' }) +
      // with caps.attach, attachment metadata typed in by hand (a stand-in oid is computed)
      (CAPS.attach ? '<div class="cextra"><input id="catt-name" type="text" placeholder="' + wsT("js.tk.composer.attname") + '"><input id="catt-size" type="number" min="0" placeholder="' + wsT("js.tk.composer.attsize") + '">' +
      btn('att-add', wsT("js.tk.composer.attach"), {}) + '<span class="muted">' + wsT("js.tk.composer.limit", fmtSize(VS.attachments.max_bytes)) + '</span></div>' : '') + '</div>';
    h += '</div><div class="iw-side">' + propertyCard(it) + assigneesHtml(v, it, recs) + ccHtml(it) + takeHtml(v, it) + '</div></div>';
    return h;
  }
  // The header's common actions, left of the "⋯" menu: the viewer's status as a segmented control
  // (applies on click, no note; the menu keeps the status-with-note part), the priority chip (it
  // opens the menu's priority step; the chip row's read-only priority chip gives way to it), then
  // "add subtask" and "hand off" as icon buttons. Each shows under the same condition the menu uses.
  function headActs(it) {
    var mine = !it.hidden && mineLive(it), r = canStatus(it) && mine, h = '';
    if (r) {
      h += '<div class="iw-seg" role="group" aria-label="' + wsT("js.tk.prop.status") + '">' + ownStatusOpts(r).map(function (o) {
        // a disabled button shows no tooltip of its own, so the reason rides on a wrapper
        var b = '<button type="button" class="sg"' + attr('act', 'hstatus') + attr('item', it.id) + attr('status', o.s) + ' aria-pressed="' + (o.s === r.status) + '" aria-label="' + esc(o.label) + '"' + (o.dis ? ' disabled' : tipAttr(o.label)) + '>' +
          SI.svg(o.s) + '<span class="sl">' + esc(o.label) + '</span></button>';
        return o.dis ? '<span class="sgw"' + tipAttr(o.label + ': ' + o.why) + '>' + b + '</span>' : b;
      }).join('') + '</div>';
    }
    if (canEdit(it) && it.kind !== 'topic') {
      var lv = it.priorityLevel, d = VS.priority.default, pt = wsT("js.tk.win.pri", it.priorityName) + priStored(it);
      h += '<button type="button" class="hpri' + (lv > d ? ' hi' : lv < d ? ' lo' : '') + '"' + attr('act', 'pri') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(pt) + '"' + tipAttr(pt) + '>' +
        '<svg class="i"><use href="#tki-flag"/></svg><span>' + esc(it.priorityName) + '</span></button>';
    }
    if (canSplit(it)) h += '<button type="button" class="ia"' + attr('act', 'hsub') + attr('item', it.id) + ' aria-label="' + esc(wsT("js.tk.menu.sub")) + '"' + tipAttr(wsT("js.tk.menu.sub")) + '><svg class="i"><use href="#tki-plus"/></svg></button>';
    if (mine && !wrapped(mine.status) && !mine.fallback) h += '<button type="button" class="ia"' + attr('act', 'handoff') + attr('item', it.id) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.act.handoff")) + '"' + tipAttr(wsT("js.tk.act.handoff")) + '><svg class="i"><use href="#tki-handoff"/></svg></button>';
    return h ? '<div class="iw-acts">' + h + '</div>' : '';
  }
  // The item window's id button: "<kind> ID" copies the item's id (its tooltip) and says so for a moment.
  function idCopyBtn(it) {
    return '<button type="button" class="idcopy"' + attr('act', 'idcopy') + attr('id', it.alias) + ' aria-label="' + wsT("js.tk.win.idcopy.aria", esc(kindName(it.kind)), esc(it.alias)) + '"' + tipAttr(it.alias) + '>' +
      '<svg class="i"><use href="#tki-copy"/></svg><span>' + wsT("js.tk.win.idcopy", esc(kindName(it.kind))) + '</span></button>';
  }
  // Clipboard write; a page served over plain http (LAN) has no navigator.clipboard, so a
  // selected textarea and execCommand stand in. near is where the textarea goes: the rest
  // of the page is inert while the item window is open.
  function copyText(text, near, done) {
    function legacy() {
      var ta = document.createElement('textarea');
      ta.value = text; ta.setAttribute('readonly', ''); ta.style.position = 'fixed'; ta.style.opacity = '0';
      near.appendChild(ta); ta.select();
      var ok = false;
      try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
      ta.remove(); near.focus();
      done(ok);
    }
    if (navigator.clipboard && window.isSecureContext) navigator.clipboard.writeText(text).then(function () { done(true); }, legacy);
    else legacy();
  }
  function copyId(a) {
    copyText(a.getAttribute('data-id'), a.parentNode, function (ok) {
      if (!ok) return toast(esc(wsT("js.tk.win.copyfail")));
      var span = a.querySelector('span'), was = span.innerHTML;
      span.textContent = wsT("js.tk.win.copied"); a.classList.add('done');
      setTimeout(function () { if (a.isConnected) { span.innerHTML = was; a.classList.remove('done'); } }, 1200);
    });
  }
  // the attachments drafted for the next post; a picked mention shows as its @id in the text itself
  function draftHtml() {
    if (!ui.adraft.length) return '';
    return '<div class="cdraft">' +
      ui.adraft.map(function (x, i) { return '<span class="chip att">' + wsT("js.tk.att.file", esc(x.name), fmtSize(x.size)) + '</span>' + btn('att-x', 'x', { idx: i }); }).join('') +
      '<span class="muted">' + wsT("js.tk.composer.draft") + '</span></div>';
  }
  // A stand-in for the sha256 of the content: the drafted attachment has no content, so it hashes
  // the name and size into 64 hex digits. The real client hashes the file.
  function mockOid(s) {
    var h = '', x = 2166136261;
    for (var r = 0; r < 8; r++) { for (var i = 0; i < s.length; i++) x = Math.imul(x ^ s.charCodeAt(i) ^ r, 16777619) >>> 0; h += ('0000000' + x.toString(16)).slice(-8); }
    return h;
  }
  var MIME = { png: 'image/png', jpg: 'image/jpeg', pdf: 'application/pdf', txt: 'text/plain', zip: 'application/zip', zst: 'application/zstd', md: 'text/markdown' };
  function mimeOf(name) { return MIME[String(name).split('.').pop().toLowerCase()] || 'application/octet-stream'; }
  // A token in a text: tok with no word character, dot, hyphen, @, or # joined before it, and no word
  // character, hyphen, or # (or a dot that goes on into a word) after it, so @maya is not kept
  // by @mayan and #maya.pc-5 is not #maya.pc-50, while a sentence's closing dot is not part of it.
  function hasToken(body, tok) {
    var s = String(body), i = -1;
    while ((i = s.indexOf(tok, i + 1)) >= 0) {
      var b = s.charAt(i - 1), a = s.charAt(i + tok.length);
      if (!/[\w.@#-]/.test(b) && !/[\w#-]/.test(a) && !(a === '.' && /\w/.test(s.charAt(i + tok.length + 1)))) return true;
    }
    return false;
  }
  // An edit of a body keeps the mentions whose @id is still in the text and every attachment.
  function keptMentions(ids, body) { return listOf(ids).filter(function (id) { return hasToken(body, '@' + id); }); }
  // the stored mentions plus the ones an edit picked with @, each once
  function withPicks(ids, picks) { var out = listOf(ids).slice(); listOf(picks).forEach(function (id) { if (out.indexOf(id) < 0) out.push(id); }); return out; }
  // (531) The items a text references: each #<alias> (or #<item id>) token that names an item of the
  // store, in text order. A write carries them as refs, so a ref lasts while its token stays in
  // the text; the composer's # popup only types the token. A token ending in dots or hyphens is
  // also tried without them (the sentence's own punctuation).
  var REF_TOK = /(^|[^\w.@#-])#(\w[\w.-]*(?:#\d+)?)/g;
  function refItem(k, map) {
    var id;
    while (!(id = k.indexOf('#') > 0 ? (VS.items[k] ? k : null) : map[k] || null) && /[.-]$/.test(k)) k = k.slice(0, -1);
    return id ? { id: id, tok: k } : null;
  }
  function aliasMap() { var m = {}; Object.keys(VS.items).forEach(function (id) { m[VS.items[id].alias] = id; }); return m; }
  function refsIn(text) {
    var out = [], map = aliasMap(), m;
    REF_TOK.lastIndex = 0;
    while ((m = REF_TOK.exec(String(text)))) { var r = refItem(m[2], map); if (r && out.indexOf(r.id) < 0) out.push(r.id); }
    return out;
  }
  // The same tokens in escaped text as links that open the item, its title as the tooltip.
  function refLinks(h) {
    var map = null;
    return h.replace(REF_TOK, function (m, pre, key) {
      var r = refItem(key, map = map || aliasMap());
      if (!r) return m;
      return pre + '<a href="#" class="iref"' + attr('item', r.id) + tipAttr(VS.items[r.id].title) + '>#' + esc(r.tok) + '</a>' + key.slice(r.tok.length);
    });
  }
  // In the daemon's rendered markdown: the text nodes outside links and code.
  function refLinksHtml(h) {
    if (h.indexOf('#') < 0) return h;
    var t = document.createElement('template'), w, n, ns = [];
    t.innerHTML = h;
    w = document.createTreeWalker(t.content, NodeFilter.SHOW_TEXT);
    while ((n = w.nextNode())) if (n.nodeValue.indexOf('#') >= 0 && !(n.parentElement && n.parentElement.closest('a, code, pre'))) ns.push(n);
    ns.forEach(function (x) {
      var e = esc(x.nodeValue), r = refLinks(e);
      if (r === e) return;
      var sp = document.createElement('span'), f = document.createDocumentFragment();
      sp.innerHTML = r;
      while (sp.firstChild) f.appendChild(sp.firstChild);
      x.parentNode.replaceChild(f, x);
    });
    return t.innerHTML;
  }

  // ------------------------------------------------------- the markdown editor

  // One editor serves the item body edit (iw-ebody), the comment box (cbox), and a comment's
  // edit in place (tkcedit): a Write / Preview toggle over the textarea, which carries the @ / #
  // suggestion popup (data-mention), and a preview pane in its place while previewing. Each
  // place keeps its own save and cancel keys (the keydown handler goes by the textarea id).
  // The preview is the adapter's render of the text, the renderer the saved view uses, drawn
  // like the saved block (o.pv: its class); without render, the plain text. The textarea stays
  // in the page while hidden, so a save reads it as before.
  // o: { value, rows, label, placeholder, cls (the wrapper's extra class), pv, foot (HTML after
  // the text, inside the wrapper), src (source only: no Write / Preview toggle, no preview pane,
  // no @/# suggestions; edImages refuses images for it) }
  function editorHtml(id, o) {
    var on = !o.src && !!ui.edpv[id];
    if (o.src) {
      return '<div class="tked' + (o.cls ? ' ' + o.cls : '') + '"' + attr('ed', id) + ' data-src>' +
        '<textarea id="' + id + '" data-keep rows="' + (o.rows || 3) + '" spellcheck="false"' + (o.label ? ' aria-label="' + o.label + '"' : '') + (o.placeholder ? ' placeholder="' + o.placeholder + '"' : '') + '>' + esc(o.value || '') + '</textarea>' +
        (o.foot || '') + '</div>';
    }
    return '<div class="tked' + (o.cls ? ' ' + o.cls : '') + '"' + attr('ed', id) + '>' +
      '<div class="tked-tabs" role="group" aria-label="' + wsT("js.tk.ed.mode") + '">' +
      ['write', 'preview'].map(function (m) {
        return '<button type="button" class="tked-tab"' + attr('act', 'ed-mode') + attr('ed', id) + attr('mode', m) + ' aria-pressed="' + ((m === 'preview') === on) + '">' + (m === 'write' ? wsT("js.tk.ed.write") : wsT("js.tk.ed.preview")) + '</button>';
      }).join('') + '</div>' +
      '<textarea id="' + id + '" data-keep data-mention rows="' + (o.rows || 3) + '" aria-autocomplete="list" aria-controls="tkmpop" aria-expanded="false"' +
      (o.label ? ' aria-label="' + o.label + '"' : '') + (o.placeholder ? ' placeholder="' + o.placeholder + '"' : '') + (on ? ' hidden' : '') + '>' + esc(o.value || '') + '</textarea>' +
      '<div id="' + id + '-pv" class="' + pvClass(id, o.pv) + '"' + attr('pv', o.pv || '') + ' aria-live="polite"' + (on ? '' : ' hidden') + '>' + pvHtml(id) + '</div>' +
      (o.foot || '') + '</div>';
  }
  function pvClass(id, cls) { var st = ui.edpv[id]; return 'tked-pv' + (cls ? ' ' + cls : '') + (st && st.html != null && !st.err ? ' md' : ''); }
  function pvHtml(id) {
    var st = ui.edpv[id];
    if (!st) return '';
    if (!st.text.trim()) return '<span class="muted">' + wsT("js.tk.ed.empty") + '</span>';
    if (st.err != null) return '<span class="muted">' + wsT("js.tk.ed.failed", esc(st.err)) + '</span>';
    if (st.busy) return '<span class="muted">' + wsT("js.tk.ed.loading") + '</span>';
    return st.html != null ? refLinksHtml(st.html) : refLinks(esc(st.text));
  }
  // Switches editor id to mode ('write' or 'preview') in place, so the toggle keeps focus and a
  // re-render is not needed; a render that lands meanwhile draws the same state from ui.edpv.
  function edMode(id, mode) {
    var box = $(id), wrap = box && box.closest('.tked');
    if (!wrap || (mode === 'preview') === !!ui.edpv[id]) return;
    if (mode === 'preview') {
      if (MP) mentionClose();
      var st = ui.edpv[id] = { text: box.value, html: null, err: null, busy: false };
      if (AD.render && st.text.trim()) {
        st.busy = true;
        AD.render(ui.store, { text: st.text, attachments: edAtts(id), wiki: id === 'wk-ebody' ? wkRenderName() : undefined }, function (res) {
          if (ui.edpv[id] !== st) return; // back to write, or the editor closed
          st.busy = false;
          if (res && res.ok) st.html = String(res.html || ''); else st.err = String((res && res.error) || '');
          pvPaint(id);
        });
      }
    } else delete ui.edpv[id];
    var on = !!ui.edpv[id];
    box.hidden = on;
    $(id + '-pv').hidden = !on;
    Array.prototype.forEach.call(wrap.querySelectorAll('.tked-tab'), function (b) { b.setAttribute('aria-pressed', String((b.getAttribute('data-mode') === 'preview') === on)); });
    pvPaint(id);
    if (!on) box.focus();
  }
  function pvPaint(id) {
    var p = $(id + '-pv');
    if (!p) return;
    p.className = pvClass(id, p.getAttribute('data-pv'));
    p.innerHTML = pvHtml(id);
  }
  // An editor textarea's text kept in ui, so a re-render draws it back (the comment box keeps
  // its own draft across a render instead).
  function edKeep(t) {
    if (t.id === 'tkcedit' && ui.cedit) ui.cedit.body = t.value;
    else if (/^iw-e(title|body|proj)$/.test(t.id) && ui.iedit) { ui.iedit[{ 'iw-etitle': 'title', 'iw-ebody': 'body', 'iw-eproj': 'project' }[t.id]] = t.value; keepItemDraft(); }
    else if (t.id === 'wk-ebody' && wk().edit) wk().edit.text = t.value;
    else if (t.id === 'wk-epath' && wk().edit) wkPathKind(wk().edit, t.value);
  }

  // An image pasted or dropped into an editor (png, jpeg, gif, or webp, within the view's
  // attachments.max_bytes, checked here before any request) is uploaded at once through the
  // adapter's upload, which stages it until a write names it, then written into the text at the
  // caret as ![name](attachment:<oid>) and kept in ui.eatt[editor id]. A save sends the entries
  // whose attachment:<oid> is still in the text (savedAtts), and the preview's render request
  // carries them, so a pasted image shows before it is saved. Any other file, an oversized
  // image, and every image without upload (the mockup) get a notice and nothing else. An upload
  // that lands after its editor ended is dropped; its staged copy expires on the server.
  var IMG_TYPES = { 'image/png': 1, 'image/jpeg': 1, 'image/gif': 1, 'image/webp': 1 };
  function edImages(id, files, named) {
    var max = VS && VS.attachments ? VS.attachments.max_bytes : 0, store = ui.store;
    if (id === 'wk-ebody' && wkEditHtml(wk().edit)) return toast(wsT("js.tk.ed.img.off")); // an HTML page cannot show images
    files.forEach(function (f) {
      var label = esc((named && f.name) || wsT("js.tk.ed.img.this"));
      if (!AD.upload) return toast(wsT("js.tk.ed.img.off"));
      if (!IMG_TYPES[f.type]) return toast(wsT("js.tk.ed.img.type", label));
      if (max && f.size > max) return toast(wsT("js.tk.ed.img.big", label, fmtSize(max)));
      var list = ui.eatt[id] || (ui.eatt[id] = []);
      toast(wsT("js.tk.ed.img.busy", label));
      AD.upload(store, f, named ? f.name || '' : '', function (res) {
        var box = $(id);
        if (ui.store !== store || ui.eatt[id] !== list || !box) return; // the editor ended meanwhile
        if (!res || !res.ok || !res.attachment || !res.attachment.oid) return toast(wsT("js.tk.ed.img.fail", label, esc((res && res.error) || '')));
        var a = res.attachment;
        if (!list.some(function (x) { return x.oid === a.oid; })) list.push({ oid: a.oid, name: a.name, size: a.size, mime: a.mime });
        if (ui.edpv[id]) edMode(id, 'write');
        var alt = String(a.name || 'image').replace(/[\r\n]+/g, ' ').replace(/[\\\[\]]/g, '\\$&');
        edInsert(box, '![' + alt + '](attachment:' + a.oid + ')');
      });
    });
  }
  // Puts s at box's caret (its selection while it is not focused), spaced from the words around it.
  function edInsert(box, s) {
    var v = box.value, a = box.selectionStart, b = box.selectionEnd;
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = ' ' + s;
    if (b < v.length && !/\s/.test(v.charAt(b))) s += ' ';
    box.setRangeText(s, a, b, 'end');
    edKeep(box); // setRangeText fires no input event
    if (box.id === 'cbox' || document.activeElement !== box) box.focus();
  }
  // The attachments editor id's text already had: an edit's stored ones, the composer's
  // hand-drafted ones (caps.attach, the mockup).
  function edBase(id) {
    var o = id === 'iw-ebody' ? ui.iedit && VS.items[ui.iedit.item] : id === 'tkcedit' ? ui.cedit && VS.comments[ui.cedit.cid] : null;
    return id === 'cbox' ? ui.adraft : o ? listOf(o.attachments) : [];
  }
  // edBase plus the editor's uploads, each oid once: the list a preview renders against.
  function edAtts(id) {
    var out = edBase(id).slice();
    listOf(ui.eatt[id]).forEach(function (a) { if (!out.some(function (x) { return x.oid === a.oid; })) out.push(a); });
    return out;
  }
  // The attachments a save of editor id with text sends: those whose attachment:<oid> is still in
  // the text, as a mention lasts while its @id does. Under caps.attach the base entries are
  // metadata drafted by hand, which no text names, so they stay as before.
  function savedAtts(id, text) {
    var base = edBase(id);
    return edAtts(id).filter(function (a) { return String(text).indexOf('attachment:' + a.oid) >= 0 || (CAPS.attach && base.indexOf(a) >= 0); });
  }

  // ------------------------------------------ @-mention and #-reference suggestion popup

  // An @ or a # at the start or after whitespace in an editor's textarea opens a list anchored at it,
  // filtered by the text after it: members for @, items (by alias and title) for #. It is driven
  // by input events, which fire during an IME composition as well, so a Hangul name or title
  // filters while it is still being composed.
  var MP = null; // the open popup: { box: the textarea id, ch: '@' or '#', start: index of ch, q: the text after it, idx: highlighted row, ids }
  // the focused editor textarea t, when it is one
  function mentionBox(t) { return t && t.hasAttribute && t.hasAttribute('data-mention') && own(t) ? t : null; }
  function mentionMatches(q) {
    q = q.toLowerCase();
    var pre = [], sub = [];
    Object.keys(VS.members).forEach(function (id) {
      var m = VS.members[id], pr = m.profile || {};
      if (m.removed) return;
      var keys = [id, m.name, pr.given, pr.family].filter(Boolean).map(function (x) { return String(x).toLowerCase(); });
      if (keys.some(function (x) { return x.indexOf(q) === 0; })) pre.push(id);
      else if (keys.some(function (x) { return x.indexOf(q) >= 0; })) sub.push(id);
    });
    return pre.concat(sub);
  }
  // items for a # token: alias prefix, then title prefix, then either containing it; hidden items
  // and the open item itself left out
  function refMatches(q) {
    q = q.toLowerCase();
    var a = [], b = [], c = [];
    VS.itemOrder.forEach(function (id) {
      var it = VS.items[id], al = it.alias.toLowerCase(), ti = it.title.toLowerCase();
      if (it.hidden || id === ui.item) return;
      if (al.indexOf(q) === 0) a.push(id); else if (ti.indexOf(q) === 0) b.push(id); else if (ti.indexOf(q) >= 0 || al.indexOf(q) >= 0) c.push(id);
    });
    return a.concat(b, c).slice(0, 30);
  }
  function mentionToken(box) {
    if (box.selectionStart !== box.selectionEnd) return null;
    var m = /(^|\s)([@#])([^\s@#]*)$/.exec(box.value.slice(0, box.selectionStart));
    return m ? { ch: m[2], start: box.selectionStart - m[3].length - 1, q: m[3] } : null;
  }
  // Opens, refilters, re-places, or closes the popup after the box's text or caret changed.
  function mentionSync() {
    var box = mentionBox(document.activeElement), tok = box && mentionToken(box);
    if (!tok || (MP && MP.box !== box.id)) mentionClose();
    if (!tok) return;
    var same = MP && MP.ch === tok.ch && MP.start === tok.start && MP.q === tok.q;
    MP = { box: box.id, ch: tok.ch, start: tok.start, q: tok.q, idx: same ? MP.idx : 0, ids: tok.ch === '#' ? refMatches(tok.q) : mentionMatches(tok.q) };
    if (MP.idx >= MP.ids.length) MP.idx = 0;
    mentionDraw(box); mentionPlace(box);
  }
  function mentionDraw(box) {
    var p = $('tkmpop');
    var ref = MP.ch === '#';
    p.innerHTML = MP.ids.map(function (id, i) {
      var it = ref && VS.items[id];
      return '<li class="mopt' + (i === MP.idx ? ' on' : '') + '" role="option" id="tkmopt-' + i + '" aria-selected="' + (i === MP.idx) + '"' + attr('mp', i) + '>' +
        (ref ? '<span class="sti">' + SI.svg(stKey(it)) + '</span><span class="mn">' + esc(it.title) + '</span><span class="muted">' + esc(it.alias) + '</span>' :
          avHtml(id) + '<span class="mn">' + whoName(id) + '</span><span class="muted">' + esc(id) + '</span>') + '</li>';
    }).join('') || '<li class="qnone" role="presentation">' + (ref ? wsT("js.tk.ref.none") : wsT("js.tk.mention.none")) + '</li>';
    p.setAttribute('aria-label', ref ? wsT("js.tk.ref.tip") : wsT("js.tk.composer.mention.tip"));
    p.hidden = false;
    box.setAttribute('aria-expanded', 'true');
    box.setAttribute('aria-activedescendant', MP.ids.length ? 'tkmopt-' + MP.idx : '');
    var on = p.querySelector('.mopt.on');
    if (on) on.scrollIntoView({ block: 'nearest' });
  }
  // The @'s (or #'s) position in the viewport, measured on a hidden copy of the box laid out alike.
  function mentionAnchor(box) {
    var cs = getComputedStyle(box), d = document.createElement('div'), mark = document.createElement('span');
    ['boxSizing', 'width', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
      'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'letterSpacing', 'lineHeight', 'textTransform', 'wordSpacing', 'tabSize'].forEach(function (k) { d.style[k] = cs[k]; });
    d.style.cssText += ';position:absolute;visibility:hidden;top:0;left:-9999px;white-space:pre-wrap;overflow-wrap:break-word;border-style:solid';
    d.textContent = box.value.slice(0, MP.start);
    mark.textContent = MP.ch; d.appendChild(mark);
    box.parentNode.appendChild(d);
    var r = box.getBoundingClientRect(), x = r.left + mark.offsetLeft - box.scrollLeft, y = r.top + mark.offsetTop - box.scrollTop, lh = mark.offsetHeight;
    d.remove();
    return { x: x, top: y, bottom: y + lh };
  }
  // above the @ when it fits, so the box's text and send button stay visible; else below it
  function mentionPlace(box) {
    var p = $('tkmpop'), a = mentionAnchor(box), w = p.offsetWidth, h = p.offsetHeight;
    p.style.left = Math.max(8, Math.min(a.x - 4, innerWidth - w - 8)) + 'px';
    p.style.top = (a.top - h - 4 >= 8 ? a.top - h - 4 : Math.min(a.bottom + 4, innerHeight - h - 8)) + 'px';
  }
  function mentionClose() {
    if (!MP) return;
    var box = $(MP.box);
    MP = null; $('tkmpop').hidden = true;
    if (box) { box.setAttribute('aria-expanded', 'false'); box.removeAttribute('aria-activedescendant'); }
  }
  // Picking replaces the @query token with "@<id> " at the caret and records the id for the
  // editor's next write (the comment box's draft, or the edit's mpick); a #query token becomes
  // "#<alias> ", which the write reads back as a ref (refsIn).
  function mentionPick(i) {
    var box = MP && $(MP.box), id = MP && MP.ids[i];
    if (!box || !id) return;
    // The pick replaces the whole token, the part after the caret too (`@ma|ry`).
    var end = box.selectionStart + /^[^\s@#]*/.exec(box.value.slice(box.selectionStart))[0].length;
    var spaced = /\s/.test(box.value.charAt(end)), ref = MP.ch === '#';
    box.setRangeText((ref ? '#' + VS.items[id].alias : '@' + id) + (spaced ? '' : ' '), MP.start, end, 'end');
    if (spaced) box.selectionStart = box.selectionEnd = box.selectionEnd + 1;
    var picks = box.id === 'iw-ebody' ? ui.iedit && ui.iedit.mpick : box.id === 'tkcedit' ? ui.cedit && ui.cedit.mpick : ui.mdraft;
    if (!ref && picks && picks.indexOf(id) < 0) picks.push(id);
    edKeep(box); // setRangeText fires no input event
    mentionClose(); box.focus();
  }
  // The @ button: an @ at the caret (after a space when it would touch a word), then the popup.
  function mentionInsert() {
    var box = $('cbox');
    if (!box) return;
    edMode('cbox', 'write');
    box.focus();
    var s = box.selectionStart;
    box.setRangeText((s && !/\s/.test(box.value.charAt(s - 1)) ? ' ' : '') + '@', s, box.selectionEnd, 'end');
    mentionSync();
  }

  // --------------------------------------------------------- dialog forms

  function optsHtml(list, empty, cur) {
    if (!list.length) return '<option value="">(' + empty + ')</option>';
    return list.map(function (x) { return '<option value="' + esc(x[0]) + '"' + (String(x[0]) === String(cur) ? ' selected' : '') + '>' + esc(x[1]) + '</option>'; }).join('');
  }
  function memberOpts(cur, filter) {
    return optsHtml(Object.keys(VS.members).filter(function (id) { return !filter || filter(VS.members[id]); }).map(function (id) { return [id, nmo(id) + ' (' + id + ')']; }), wsT("js.tk.common.nobody"), cur);
  }
  function reachTag(v, id) { return id === v ? wsT("js.tk.ev.self") : VS.viewer.reach[id] ? wsT("js.tk.dlg.forced", VS.viewer.reach[id]) : wsT("js.tk.st.pending"); }
  function fld(label, inner) { return '<div class="fld"><span>' + label + '</span>' + inner + '</div>'; }
  // A person's earlier profiles are machine-local, never in the ledger: the adapter keeps
  // them (profiles / saveProfile). Editing your own profile saves the result as one.
  function saveProf(pr) {
    var one = { org: String(pr.org || '').trim(), family: String(pr.family || '').trim(), given: String(pr.given || '').trim() };
    if (one.given && AD.saveProfile) AD.saveProfile(one);
  }
  function profileText(pr) { return (pr.org ? pr.org + ' ' : '') + displayName({ org: pr.org, family: pr.family, given: pr.given }); }
  function profileFields(pr) {
    return '<div class="fld inline">' + fld(wsT("js.tk.profile.org"), '<input type="text" name="org" value="' + esc(pr.org) + '">') +
      fld(wsT("js.tk.profile.family"), '<input type="text" name="family" value="' + esc(pr.family) + '">') +
      fld(wsT("js.tk.profile.given"), '<input type="text" name="given" value="' + esc(pr.given) + '">') + '</div>' +
      '<div class="hint" id="pview"></div>';
  }
  function profileView(form) {
    var el = form.elements, pv = form.querySelector('#pview');
    if (!pv || !el.given) return;
    var pr = { org: el.org.value.trim(), family: el.family.value.trim(), given: el.given.value.trim() };
    pv.classList.toggle('warn', !pr.given);
    pv.textContent = pr.given ? (pr.org ? wsT("js.tk.profile.view.org", displayName(pr), pr.org, avatar(pr)) : wsT("js.tk.profile.view", displayName(pr), avatar(pr))) : wsT("js.tk.profile.required");
  }
  // The roles-event dialog's types, each under its readable name. The store settings
  // (priority levels, attachment limit) have their own dialogs (settingsHtml).
  var ROLE_TYPES = ['roles.add_member', 'roles.set_lead', 'roles.add_admin', 'roles.remove_admin', 'roles.add_key', 'roles.revoke_key', 'roles.remove_member'];
  var ROLE_NAME = { 'roles.add_member': "js.tk.rtype.addmember", 'roles.set_lead': "js.tk.rtype.setlead", 'roles.add_admin': "js.tk.rtype.addadmin", 'roles.remove_admin': "js.tk.rtype.removeadmin",
    'roles.add_key': "js.tk.rtype.addkey", 'roles.revoke_key': "js.tk.rtype.revokekey", 'roles.remove_member': "js.tk.rtype.removemember" };
  // a roles-event field, shown only while the type select names one of its types (rtypeShow)
  function rfld(types, label, inner) { return '<div class="fld"' + attr('rt', types) + '><span>' + label + '</span>' + inner + '</div>'; }
  function rtypeShow(form) {
    var t = form.elements.rtype && form.elements.rtype.value;
    if (!t) return;
    Array.prototype.forEach.call(form.querySelectorAll('[data-rt]'), function (el) { el.style.display = el.getAttribute('data-rt').split(' ').indexOf(t) >= 0 ? '' : 'none'; });
  }

  // the dialog kinds a data-act opens (a host adds its own through ext.dialogs); every item
  // action is the context menu's or inline, so only the store-wide forms stay modal
  // one priority-level row; labels count from 1, while the radio value and the field name stay 0-based
  var PRIO_MAX = 9;
  function prioRow(pi, name, checked) {
    return '<div class="fld inline plevel"><input type="radio" name="pdef" value="' + pi + '"' + (checked ? ' checked' : '') + ' aria-label="' + esc(wsT("js.tk.form.pdefault", pi + 1)) + '"' + tipAttr(wsT("js.tk.form.pdefault", pi + 1)) + '>' +
      '<input type="text" name="pname' + pi + '" value="' + esc(name) + '" placeholder="' + esc(wsT("js.tk.form.plevel", pi + 1)) + '" aria-label="' + esc(wsT("js.tk.form.plevel", pi + 1)) + '"></div>';
  }
  var DIALOGS = { profile: 1, admin: 1, sprio: 1, satt: 1, slabel: 1 };
  function dialogHtml(kind, d) {
    if (kind === 'register') return formHtml(kind, d, registerT(d)); // these need no store view
    if (kind === 'relabel') return formHtml(kind, d, relabelT(d));
    if (kind === 'unregister') return formHtml(kind, d, unregT(d));
    var v = memberOf(viewerOf(ui.store));
    var T;
    switch (kind) {
      case 'profile': {
        var pw = d.who || v, pm = VS.members[pw];
        T = [pw === v ? wsT("js.tk.dlg.profile.title") : wsT("js.tk.dlg.profile.title.admin", esc(nm(pw))),
          pw === v ? wsT("js.tk.dlg.profile.intro") : wsT("js.tk.dlg.profile.intro.admin"),
          profileFields(pm.profile) + '<input type="hidden" name="who" value="' + esc(pw) + '">', wsT("js.tk.dlg.save")];
        break;
      }
      case 'admin': {
        var mt = 'roles.set_lead roles.add_admin roles.remove_admin roles.add_key roles.revoke_key roles.remove_member';
        T = [wsT("js.tk.dlg.admin.title"), wsT("js.tk.dlg.admin.intro", esc(viewerOf(ui.store))),
          fld(wsT("js.tk.form.type"), '<select name="rtype">' + ROLE_TYPES.filter(function (t) { return CAPS.addMember || t !== 'roles.add_member'; }).map(function (t) { return '<option value="' + esc(t) + '">' + wsT(ROLE_NAME[t]) + '</option>'; }).join('') + '</select>') +
          rfld(mt, wsT("js.tk.form.member"), '<select name="member">' + memberOpts('') + '</select>') +
          rfld('roles.add_member roles.set_lead', wsT("js.tk.form.lead"), '<select name="lead"><option value="">' + wsT("js.tk.form.lead.none") + '</option>' + memberOpts(v) + '</select>') +
          rfld('roles.add_member roles.add_key roles.revoke_key', wsT("js.tk.form.key"), '<input type="text" name="key" placeholder="' + wsT("js.tk.form.key.ph") + '">') +
          (CAPS.addMember ? rfld('roles.add_member', wsT("js.tk.form.newmember"), '<div class="fld inline"><input type="text" name="nid" placeholder="' + wsT("js.tk.prop.id") + '"><input type="text" name="norg" placeholder="' + wsT("js.tk.field.org") + '"><input type="text" name="nfamily" placeholder="' + wsT("js.tk.field.family") + '"><input type="text" name="ngiven" placeholder="' + wsT("js.tk.field.given") + '"></div>') : ''),
          wsT("js.tk.dlg.admin.submit")];
        break;
      }
      case 'sprio': {
        // one row per level, lowest first, plus blank rows up to the 9-level cap; blank rows
        // are dropped on save, and the radio picks the default
        var pn = VS.priority.names, rows = '';
        for (var pi = 0; pi < Math.min(PRIO_MAX, pn.length + 2); pi++) rows += prioRow(pi, pn[pi] || '', pi === VS.priority.default);
        T = [wsT("js.tk.dlg.prio.title"), wsT("js.tk.dlg.prio.intro"), rows, wsT("js.tk.dlg.save")];
        break;
      }
      case 'slabel': {
        // add (no label) or rename / recolour one; the colour input gives #rrggbb in lowercase
        var el0 = d.label ? labelOf(d.label) : null;
        T = [wsT(el0 ? "js.tk.dlg.label.edit" : "js.tk.dlg.label.add"), wsT("js.tk.dlg.label.intro"),
          (el0 ? '<input type="hidden" name="label" value="' + esc(el0.id) + '">' : '') +
          fld(wsT("js.tk.form.lname"), '<input type="text" name="lname" autocomplete="off" value="' + esc(el0 ? el0.name : '') + '">') +
          fld(wsT("js.tk.form.lcolor"), '<input type="color" name="lcolor" value="' + esc(el0 ? el0.color : '#6b7280') + '">'), wsT("js.tk.dlg.save")];
        break;
      }
      case 'satt':
        T = [wsT("js.tk.dlg.att.title"), wsT("js.tk.dlg.att.intro", esc(fmtSize(VS.attachments.max_bytes))),
          fld(wsT("js.tk.form.maxbytes"), '<input type="number" name="maxbytes" min="1" step="1" value="' + esc(VS.attachments.max_bytes) + '">'), wsT("js.tk.dlg.save")];
        break;
      default: {
        // a host dialog (ext.dialogs)
        var xd = X.dialogs[kind].html(ui.store, VS, d, H);
        T = [xd.title, xd.intro, xd.body || '', xd.submit];
      }
    }
    return formHtml(kind, d, T);
  }
  // T = [title, intro, body, submit label]; no submit label leaves only a Close button.
  function formHtml(kind, d, T) {
    var btns = T[3] ? '<button type="button" class="btn" data-act="dlg-cancel">' + wsT("js.tk.dlg.cancel") + '</button><button type="submit" class="btn primary">' + T[3] + '</button>'
      : '<button type="button" class="btn primary" data-act="dlg-cancel">' + wsT("js.tk.dlg.close") + '</button>';
    return '<form' + attr('kind', kind) + attr('fitem', d.item || '') + attr('fcid', d.cid || '') + (d.store ? attr('fstore', d.store) : '') + '><h2>' + T[0] + '</h2><p>' + T[1] + '</p>' + T[2] +
      '<div class="btns">' + btns + '</div></form>';
  }

  // The register dialog ("+ store"): create a store (no remote, or an empty repository) or
  // join the store a remote holds. d = {profiles: {last, profiles}} as adapter.profiles
  // reads them, or {hostErr} when the host is unavailable (then it only says why). The
  // member profile defaults to the saved one used last; the host saves what is submitted
  // as the one used last (an edited profile becomes a new entry), so the dialog saves
  // nothing itself. No lead field: a self-join cannot set relations.
  var ID_RE = /^[a-z0-9][a-z0-9_-]{0,63}$/;
  function registerT(d) {
    if (d.hostErr) return [wsT("js.tk.dlg.reg.title"), wsT("js.tk.dlg.reg.unavailable"), '<div class="hint warn">' + esc(d.hostErr) + '</div>', null];
    var ps = (d.profiles && d.profiles.profiles) || [], last = d.profiles && ps[d.profiles.last] ? d.profiles.last : -1;
    var p0 = last >= 0 ? ps[last] : { org: '', family: '', given: '' };
    var saved = '<select name="saved">' + ps.map(function (x, i) {
      var t = profileText(x);
      return '<option value="' + i + '" data-p="' + esc(JSON.stringify({ org: x.org || '', family: x.family || '', given: x.given || '' })) + '"' + (i === last ? ' selected' : '') + '>' + esc(i === last ? wsT("js.tk.dlg.reg.saved.last", t) : t) + '</option>';
    }).join('') + '<option value=""' + (last < 0 ? ' selected' : '') + '>' + (ps.length ? wsT("js.tk.dlg.reg.saved.edited") : wsT("js.tk.dlg.reg.saved.none")) + '</option></select>';
    return [wsT("js.tk.dlg.reg.title"), wsT("js.tk.dlg.reg.intro"),
      '<div class="fld inline">' + fld(wsT("js.tk.dlg.reg.id"), '<input type="text" name="sid" autocomplete="off" spellcheck="false">') +
        fld(wsT("js.tk.dlg.reg.label"), '<input type="text" name="label" autocomplete="off">') + '</div>' +
      fld(wsT("js.tk.dlg.reg.remote"), '<input type="text" name="remote" autocomplete="off" spellcheck="false" placeholder="' + esc(wsT("js.tk.dlg.reg.remote.ph")) + '">') +
      fld(wsT("js.tk.dlg.reg.saved"), saved) + profileFields(p0) +
      fld(wsT("js.tk.dlg.reg.member"), '<input type="text" name="nid" data-auto="1" autocomplete="off" spellcheck="false" value="' + esc(asciiId(p0.given)) + '">') +
      '<div class="hint" id="regmsg" role="status" hidden></div>' + regHiddenHtml(),
      wsT("js.tk.dlg.reg.submit")];
  }
  function openRegister() {
    var he = AD.hostError && AD.hostError();
    if (he) { openDialog('register', { hostErr: he }); return; }
    if (!AD.profiles) { openDialog('register', {}); return; }
    AD.profiles(function (p) { openDialog('register', { profiles: p }); });
  }
  function regMsg(form, text, warn) {
    var m = form.querySelector('#regmsg');
    m.textContent = text; m.hidden = !text; m.classList.toggle('warn', !!warn);
  }
  function submitRegister(form) {
    var el = form.elements, bt = form.querySelector('button[type="submit"]');
    if (!bt || bt.disabled) return;
    function val(n) { return el[n] ? el[n].value.trim() : ''; }
    var req = { id: val('sid'), label: val('label'), remote: val('remote'), member: val('nid'), profile: { org: val('org'), family: val('family'), given: val('given') } };
    var bad = !ID_RE.test(req.id) ? ['sid', wsT("js.tk.dlg.reg.bad.id")] : req.id === 'keys' ? ['sid', wsT("js.tk.dlg.reg.bad.keys")]
      : !req.profile.given ? ['given', wsT("js.tk.dlg.reg.bad.given")] : !ID_RE.test(req.member) ? ['nid', wsT("js.tk.dlg.reg.bad.member")] : null;
    if (bad) { regMsg(form, bad[1], true); el[bad[0]].focus(); return; }
    // a join clones the remote, which can take a while: the button says so until the answer
    var label = bt.textContent;
    bt.disabled = true; bt.textContent = wsT("js.tk.dlg.reg.busy");
    regMsg(form, wsT("js.tk.dlg.reg.wait"), false);
    AD.register(req, function (res) {
      var here = form.isConnected && $('tkdlg').open; // the dialog may have been closed (Escape) meanwhile
      if (!res || !res.ok) {
        if (!here) { toast(esc((res && res.error) || wsT("js.tk.toast.refused"))); return; }
        bt.disabled = false; bt.textContent = label;
        regMsg(form, (res && res.error) || wsT("js.tk.toast.refused"), true);
        return;
      }
      if (here) closeDialog();
      var id = res.store || req.id;
      toast(res.creator ? wsT("js.tk.toast.reg.created", esc(id)) : wsT("js.tk.toast.reg.joined", esc(id), esc(req.member)));
      // the adapter answers after its store list holds the new store
      if (!ui.page && ui.open !== id && (AD.stores() || []).indexOf(id) >= 0) openStore(id);
    });
  }

  // The store registry on this machine (adapter.unregister, relabel, setHidden, hidden, info).
  // The store menu (the "⋯" in a store's title row) relabels, hides, or unregisters the store
  // shown; the "+ store" dialog lists the hidden stores, each with show and unregister. None of
  // these writes to the store itself: relabel and hide change this machine's registry entry, and
  // unregister deletes the local copy (the remote keeps the store). Unregister never syncs
  // first: a sync in flight would make the host refuse it as busy. A store that leaves the list
  // (hidden or removed, here or by another process) is dropped from every state naming it on the
  // next render (dropGone), with no sync, toast, or retry.
  function regInfo(id) { return (AD.info && AD.info(id)) || { id: id, label: '', remote: '', hidden: false }; }
  function regTitle(id) { var s = regInfo(id); return s.label && s.label !== id ? s.label + ' (' + id + ')' : id; }
  function regKnown(id) { return order.indexOf(id) >= 0 || (AD.hidden ? AD.hidden() || [] : []).indexOf(id) >= 0; }
  function resError(res) { return (res && res.error) || wsT("js.tk.toast.refused"); }
  function storeMenuBtn(name) {
    if (!AD.unregister && !AD.relabel && !AD.setHidden) return '';
    return '<button type="button" class="ia"' + attr('act', 'store-menu') + attr('store', name) + ' aria-haspopup="menu" aria-label="' + esc(wsT("js.tk.store.menu")) + '"' +
      tipAttr(wsT("js.tk.store.menu")) + '><svg class="i"><use href="#tki-more"/></svg></button>';
  }
  function storeMenuHtml() {
    var h = '<div class="mh"><b>' + esc(regTitle(MN.store)) + '</b></div>';
    if (AD.relabel) h += mi('m-s-label', esc(wsT("js.tk.store.relabel")), {}, { icon: 'pen' });
    if (AD.setHidden) h += mi('m-s-hide', esc(wsT("js.tk.store.hide")), {}, { icon: 'minus' });
    if (AD.unregister) h += msep() + mi('m-s-unreg', esc(wsT("js.tk.store.unreg")), {}, { icon: 'x', cls: 'danger' });
    return h;
  }
  function storeAct(k) {
    var id = MN.store;
    closeMenu(false);
    if (k === 'm-s-hide') hideStore(id);
    else if (k === 'm-s-label') openDialog('relabel', { store: id });
    else if (k === 'm-s-unreg') openDialog('unregister', { store: id });
  }
  // Hiding the store on screen leaves it the way a collapse or leaving its page does (the
  // apply path), then the reload drops its handle.
  function hideStore(id) {
    if (ui.page === id) leavePage();
    else if (ui.open === id) collapse();
    AD.setHidden(id, true, function (res) {
      if (!res || !res.ok) { toast(esc(resError(res))); return; }
      toast(wsT("js.tk.toast.hidden", esc(id)));
    });
  }
  // "show" in the register dialog's hidden list: the handle comes back and the list redraws.
  function showStore(id) {
    AD.setHidden(id, false, function (res) {
      if (!res || !res.ok) { toast(esc(resError(res))); return; }
      var box = $('tkdlg').querySelector('.tkhid');
      if (box) box.outerHTML = regHiddenHtml();
    });
  }
  function regHiddenHtml() {
    var hs = AD.hidden ? AD.hidden() || [] : [];
    if (!hs.length) return '<div class="tkhid" hidden></div>';
    function b(act, label, id) { return '<button type="button" class="btn mini"' + attr('act', act) + attr('store', id) + '>' + label + '</button>'; }
    return '<div class="tkhid"><div class="tkhid-h">' + wsT("js.tk.dlg.reg.hidden") + '</div>' + hs.map(function (id) {
      return '<div class="tkhid-row"><span class="grow">' + esc(regTitle(id)) + '</span>' +
        (AD.setHidden ? b('store-show', wsT("js.tk.dlg.reg.show"), id) : '') + (AD.unregister ? b('store-unreg', wsT("js.tk.dlg.unreg.submit"), id) : '') + '</div>';
    }).join('') + '</div>';
  }
  // d.store, as the dialog's data-fstore; the label field starts at the registry label (empty: the id shows).
  function relabelT(d) {
    var s = regInfo(d.store);
    return [wsT("js.tk.dlg.relabel.title", esc(d.store)), wsT("js.tk.dlg.relabel.intro"),
      fld(wsT("js.tk.dlg.reg.label"), '<input type="text" name="label" maxlength="80" autocomplete="off" value="' + esc(s.label || '') + '">') +
      '<div class="hint" id="regmsg" role="status" hidden></div>', wsT("js.tk.dlg.save")];
  }
  function submitRelabel(form) {
    var id = form.getAttribute('data-fstore'), bt = form.querySelector('button[type="submit"]');
    if (!bt || bt.disabled) return;
    var label = form.elements.label.value.trim();
    if (label === (regInfo(id).label || '')) { closeDialog(); return; }
    bt.disabled = true;
    AD.relabel(id, label, function (res) {
      var here = form.isConnected && $('tkdlg').open;
      if (res && res.ok) { if (here) closeDialog(); return; }
      if (!here) { toast(esc(resError(res))); return; }
      bt.disabled = false;
      regMsg(form, resError(res), true);
    });
  }
  // A store with no remote holds its only copy here: its data is lost for good, so the store id
  // must be typed before the button enables, and goes to the host as the confirmation.
  function unregT(d) {
    var id = d.store, s = regInfo(id), t = esc(regTitle(id));
    var body = s.remote ? '' : '<div class="hint warn">' + wsT("js.tk.dlg.unreg.lost", t) + '</div>' +
      fld(wsT("js.tk.dlg.unreg.confirm", esc(id)), '<input type="text" name="confirm" autocomplete="off" spellcheck="false">');
    return [wsT("js.tk.dlg.unreg.title", t), s.remote ? wsT("js.tk.dlg.unreg.intro", t) : wsT("js.tk.dlg.unreg.intro.local", t),
      body + '<div class="hint" id="regmsg" role="status" hidden></div>', wsT("js.tk.dlg.unreg.submit")];
  }
  function unregReady(form) {
    var bt = form.querySelector('button[type="submit"]'), c = form.elements.confirm;
    if (bt && !form.hasAttribute('data-busy')) bt.disabled = !!c && c.value.trim() !== form.getAttribute('data-fstore');
  }
  // A refusal (busy, unpushed events, a stuck store) shows the host's sentence in the dialog.
  function submitUnregister(form) {
    var id = form.getAttribute('data-fstore'), bt = form.querySelector('button[type="submit"]'), c = form.elements.confirm;
    unregReady(form);
    if (!bt || bt.disabled) return;
    var label = bt.textContent;
    form.setAttribute('data-busy', '1');
    bt.disabled = true; bt.textContent = wsT("js.tk.dlg.unreg.busy");
    regMsg(form, '', false);
    AD.unregister(id, c ? c.value.trim() : '', function (res) {
      var here = form.isConnected && $('tkdlg').open;
      if (res && res.ok) { if (here) closeDialog(); toast(wsT("js.tk.toast.unreg", esc(id))); return; }
      if (!here) { toast(esc(resError(res))); return; }
      form.removeAttribute('data-busy');
      bt.textContent = label;
      unregReady(form);
      regMsg(form, resError(res), true);
    });
  }
  // dropGone runs first in every render, once order is the store list: a store no longer in it
  // stops being the one shown, open, or routed (the #/ route is cleared without a history entry,
  // so no "unknown store" notice follows), its menu and dialogs close, and its page-lifetime state
  // goes, so its wiki and search answers are dropped instead of retried. Nothing syncs: a hidden
  // store was applied by its hide, an unregistered one has nothing left to sync.
  var GONE_BY_STORE = ['wiki', 'psr', 'nrow', 'knew', 'tab', 'panel'], GONE_BY_KEY = ['tabSeen', 'loaded', 'mdet', 'wfold'];
  function dropGone() {
    function gone(n) { return !!n && order.indexOf(n) < 0; }
    var lost = gone(ui.store);
    if (lost) {
      keepItemDraft();
      ui.store = order[0] || null;
      ui.item = null; ui.win = false; ui.last = null; ui.sel = null; ui.sub = null; ui.iedit = null; ui.cedit = null; ui.replyTo = null; ui.itemChanged = true;
    } else if (!ui.store) ui.store = order[0] || null;
    if (gone(ui.open)) ui.open = null;
    if (gone(ui.page)) {
      ui.page = null;
      if (/^#\/./.test(location.hash)) history.replaceState(null, '', location.pathname + location.search);
    }
    if (MN && (MN.kind === 'store' ? gone(MN.store) : lost)) closeMenu(false, true);
    var dl = $('tkdlg'), f = dl && dl.open && dl.querySelector('form');
    if (f) {
      var fs = f.getAttribute('data-fstore');
      if (fs ? !regKnown(fs) : lost && f.getAttribute('data-kind') !== 'register') closeDialog();
    }
    GONE_BY_STORE.forEach(function (k) {
      Object.keys(ui[k]).forEach(function (n) {
        if (!gone(n)) return;
        if (k === 'psr' && ui.psr[n]) clearTimeout(ui.psr[n].timer);
        delete ui[k][n];
      });
    });
    GONE_BY_KEY.forEach(function (k) {
      Object.keys(ui[k]).forEach(function (key) { if (gone(key.slice(0, key.indexOf('|')))) delete ui[k][key]; });
    });
  }

  function asciiId(given) { return String(given || '').toLowerCase().replace(/[^a-z0-9]+/g, '') || 'newcomer'; }
  function openDialog(kind, d) {
    var dl = $('tkdlg');
    dl.innerHTML = dialogHtml(kind, d || {});
    var f = dl.querySelector('form');
    profileView(f);
    rtypeShow(f);
    if (kind === 'unregister') unregReady(f);
    if (!dl.open) dl.showModal();
    var first = f.querySelector('input[type="text"], textarea, select');
    if (first) first.focus();
  }
  function closeDialog() { var d = $('tkdlg'); if (d.open) d.close(); }

  // ---------------------------------------------------------------- actions

  var toastTimer = null;
  function toast(html) {
    if (window.WsToast) { window.WsToast.show(html, { html: true }); return; }
    var t = $('tktoast');
    if (!t) document.body.appendChild(t = el('div', { class: 'tk tk-toast', id: 'tktoast', hidden: '' }));
    t.innerHTML = html; t.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.hidden = true; }, 6000);
  }
  // One write: events go to the viewer clone's journal (open batch). Events are not
  // pre-validated, so a form can deliberately produce a flagged event. An event of
  // the batch names an earlier one's id as "$N" (its 1-based position); the adapter
  // resolves it. `then(results)` runs after a written batch; a refused one keeps the
  // dialog, the menu, or the inline input open and toasts why.
  // `prep(results)` runs on success before the re-render (the composer drafts clear only then).
  function commit(events, then, prep, fail) {
    AD.dispatch(ui.store, events.map(function (e) { return { type: e[0], payload: e[1], origin: e[2] || 'human' }; }), function (res) {
      if (!res || !res.ok) { toast(esc((res && res.error) || wsT("js.tk.toast.refused"))); if (fail) fail(); return; }
      if (prep) prep(res.results || []);
      wrote(res.results || []);
      if (then) then(res.results || []);
    });
  }
  // A written batch: the last-write line, the dialog or the item menu closed, a re-render, and
  // one toast line per event (a host write, such as a simulated join, reports through this too).
  function wrote(results) {
    ui.last = results;
    closeDialog();
    // the labels step stays open across toggles: it draws again over the written item; the range
    // picker stays open across a late write (an earlier close's), unless the write is its own back row's
    var keep = !!(MN && MN.kind === 'item' && (MN.step === 'labels' || (MN.step === 'due' && !MN.rsent)));
    var mb = MN && MN.kind === 'item' && !keep ? MN.back : null; // an item menu closes; focus goes back to its row once redrawn
    if (MN && MN.kind === 'item' && !keep) closeMenu(false);
    render();
    if (keep && MN && MN.step === 'labels') labelStepRedraw();
    var me = mb && document.querySelector(mb);
    if (me && !me.matches('a[href], button, input, [tabindex]')) me = me.querySelector('button, a[href], [tabindex]');
    if (me && !(document.activeElement && document.activeElement.hasAttribute('data-keep'))) me.focus({ preventScroll: true });
    toast(results.map(function (n) {
      return esc(n.alias) + ' ' + esc(n.type) + ': ' + (n.status === 'applied' ? wsT("js.tk.activity.applied") : '<span class="tf">' + esc(String(n.status).toUpperCase()) + '</span> ' + esc(n.reason));
    }).join('<br>') + '<br><span class="muted">' + wsT("js.tk.toast.batch") + '</span>');
  }

  function submit(form) {
    if (form.getAttribute('data-kind') === 'register') return submitRegister(form);
    if (form.getAttribute('data-kind') === 'relabel') return submitRelabel(form);
    if (form.getAttribute('data-kind') === 'unregister') return submitUnregister(form);
    var kind = form.getAttribute('data-kind'), el = form.elements, v = memberOf(viewerOf(ui.store));
    function val(n) { return el[n] ? el[n].value : ''; }
    var evs = [];
    switch (kind) {
      case 'profile': {
        var who = val('who') || v, pm = VS.members[who].profile, pp = {};
        PROFILE_FIELDS.forEach(function (f) { if (val(f).trim() !== pm[f]) pp[f] = val(f).trim(); });
        if (!Object.keys(pp).length) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        var pe = { profile: pp };
        if (who !== v) pe.member = who;
        if (who === v) saveProf({ org: val('org'), family: val('family'), given: val('given') });
        evs.push(['member.profile', pe]);
        break;
      }
      case 'admin': {
        var t = val('rtype'), rp = {};
        if (t === 'roles.add_member' && !CAPS.addMember) return;
        if (t === 'roles.add_member') rp.member = { id: val('nid'), profile: { org: val('norg'), family: val('nfamily'), given: val('ngiven') }, keys: val('key') ? [val('key')] : [], leads: val('lead') ? [val('lead')] : [] };
        else {
          rp.member = val('member');
          if (t === 'roles.set_lead') rp.leads = val('lead') ? [val('lead')] : [];
          if (t === 'roles.add_key' || t === 'roles.revoke_key') rp.key = val('key');
        }
        evs.push([t, rp]);
        break;
      }
      case 'sprio': {
        var names = [], def = -1, picked = form.querySelector('input[name="pdef"]:checked');
        for (var i = 0; el['pname' + i]; i++) {
          var pname = val('pname' + i).trim();
          if (!pname) continue;
          if (picked && picked.value === String(i)) def = names.length;
          names.push(pname);
        }
        if (!names.length) return toast(wsT("js.tk.toast.prio.none"));
        if (def < 0) return toast(wsT("js.tk.toast.prio.default"));
        if (names.join('\n') === VS.priority.names.join('\n') && def === VS.priority.default) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        evs.push(['roles.set_priority_levels', { names: names, default: def }]);
        break;
      }
      case 'slabel': {
        var lid = val('label'), ln = val('lname').trim(), lc = val('lcolor').toLowerCase(), cl = lid ? labelOf(lid) : null;
        if (!ln) return toast(wsT("js.tk.toast.label.name"));
        if (!lid) { evs.push(['roles.add_label', { name: ln, color: lc }]); break; }
        if (!cl) { closeDialog(); return; }
        var lp = { label: lid };
        if (ln !== cl.name) lp.name = ln;
        if (lc !== cl.color) lp.color = lc;
        if (!('name' in lp) && !('color' in lp)) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        evs.push(['roles.edit_label', lp]);
        break;
      }
      case 'satt': {
        var mb = Number(val('maxbytes'));
        if (mb === VS.attachments.max_bytes) { closeDialog(); return toast(wsT("js.tk.toast.unchanged")); }
        evs.push(['roles.set_attachment_limit', { max_bytes: mb }]);
        break;
      }
      default: if (X.dialogs && X.dialogs[kind]) X.dialogs[kind].submit(ui.store, form, H); return;
    }
    commit(evs);
  }

  function quick(k, a) {
    var item = a.getAttribute('data-item'), who = a.getAttribute('data-who'), cid = a.getAttribute('data-cid');
    var box = $('cbox'), origin = CAPS.origin && $('corigin') ? $('corigin').value : 'human';
    // a comment clears the composer once written
    function put(evs, extra) { commit(evs, null, extra); }
    switch (k) {
      case 'confirm': put([['item.confirm', { item: item, assignee: who }]]); break;
      case 'unassign': put([['item.unassign', a.hasAttribute('data-rule') ? { item: item, root: a.getAttribute('data-root'), rule: a.getAttribute('data-rule') } : { item: item, assignee: who }]]); break;
      case 'accept': put([['item.accept', { item: item }]]); break;
      case 'claim': put([['item.claim', { item: item }]]); break;
      case 'unopen': put([['item.unopen', { item: item }]]); break;
      case 'approve': put([['item.approve', { item: item, request: a.getAttribute('data-req') }]]); break;
      case 'unrequest': put([['item.unrequest', { item: item, request: a.getAttribute('data-req') }]]); break;
      case 'uncc': put([['item.uncc', { item: item, member: who }]]); break;
      case 'ihide': put([['item.hide', { item: item }]]); break;
      case 'chide': put([['comment.hide', { comment: cid }]]); break;
      case 'comment': {
        if (!box || !box.value.trim()) return toast(wsT("js.tk.toast.nocomment"));
        var cp = { item: item, body: box.value.trim() };
        if (ui.replyTo) cp.reply_to = ui.replyTo;
        var ms = keptMentions(ui.mdraft, cp.body), crs = refsIn(cp.body);
        if (ms.length) cp.mentions = ms;
        if (crs.length) cp.refs = crs;
        var cas = savedAtts('cbox', cp.body);
        if (cas.length) cp.attachments = cas;
        put([['comment.add', cp, origin]], function () { ui.clearDraft = true; ui.replyTo = null; ui.mdraft = []; ui.adraft = []; delete ui.eatt.cbox; });
        break;
      }
    }
  }

  // ------------------------------------------------------------ context menu

  // One context menu serves every item: list rows, kanban cards, calendar entries, gantt rows,
  // and the item window header open it alike, by right-click, the "⋯" button, or Shift+F10 /
  // the menu key on a focused row; a click on a status icon opens its status part alone. It is
  // one fixed panel (#tkmenu) over everything, the item window included. MN is its state:
  //   { kind: 'item' | 'sync', item, step, op, who, from (the step it came from, for Back),
  //     statusOnly, back (selector of the opener) and backEl (the opener), rk (the opener's row key,
  //     where "add subtask" puts its input row), pt (the pointer, for a right-click) or rect, pick }
  // The item menu's steps: 'main' (status radios, note, and the actions the viewer may take),
  // 'pri' and 'due' (priority radios, a date), 'handoff' (a people list), 'note' (decline,
  // reject, reopen, return, deny a take request with a note), 'opening' (open for claiming: the
  // scope), 'request' (a take request: whose assignment, with a note), 'assign' (the assign form: the anchored assign
  // popover). 'sync' is the Items card ribbon's status popover; 'store' is the store menu of a
  // store's title row (MN.store: its id; see storeMenuHtml). Arrow keys move, Enter or
  // Space applies, ArrowRight opens a step, ArrowLeft or Esc goes back, Esc on the first step
  // closes it, and focus returns to the opener; Tab cycles inside it.
  var MN = null;
  function openMenu(o, anchor, pt) {
    if (MN && !menuApplyClose(false)) return; // a menu opened over another closes it first, applying its pick
    MN = o; MN.step = MN.step || 'main';
    MN.backEl = anchor || null; MN.back = anchor ? returnSel(anchor) : null;
    var rk = anchor && anchor.closest('[data-rk]');
    MN.rk = rk ? rk.getAttribute('data-rk') : null;
    MN.pt = pt || null; MN.rect = anchor ? anchor.getBoundingClientRect() : null;
    hideTip();
    if (anchor && anchor.hasAttribute('aria-haspopup')) anchor.setAttribute('aria-expanded', 'true');
    document.body.classList.add('tk-menuon');
    drawMenu();
  }
  // opts: { step, op, who, statusOnly }; a step opened from inside the menu keeps its place
  function itemMenu(id, anchor, pt, opts) {
    if (!VS || !VS.items[id]) return;
    opts = opts || {};
    if (MN && MN.kind === 'item' && anchor && $('tkmenu').contains(anchor)) {
      MN.from = MN.step; MN.step = opts.step || 'main'; MN.op = opts.op || null; MN.who = opts.who || null; MN.req = opts.req || null; MN.q = ''; MN.as = null; MN.mv = null;
      drawMenu(); return;
    }
    openMenu({ kind: 'item', item: id, step: opts.step || 'main', op: opts.op || null, who: opts.who || null, req: opts.req || null, statusOnly: !!opts.statusOnly, from: null, q: '' }, anchor, pt);
  }
  // Whether the item menu's opener left the place the menu was put at: gone, or moved. A render's
  // scroll restore fires a scroll with the redrawn opener where it was, which keeps the menu (the
  // labels step, open across toggles, redraws its list row under it).
  function openerMoved() {
    var el = MN.backEl, r = el && el.isConnected ? el.getBoundingClientRect() : null;
    return !r || !MN.rect || Math.abs(r.top - MN.rect.top) > 1 || Math.abs(r.left - MN.rect.left) > 1;
  }
  // Every close applies the open picker's pick once: the labels step's toggles, the range picker's
  // ends (an item's as one item.edit, a row's into its draft, which then draws again unless quiet:
  // a close from inside a render). This close cannot stay open, so an inverted range drops unwritten
  // (menuApplyClose refuses it where the menu can stay).
  function closeMenu(refocus, quiet) {
    if (!MN) return;
    var m = MN, el = m.backEl && m.backEl.isConnected ? m.backEl : m.back && document.querySelector(m.back);
    var lw = labelPending(m), rw = rangePending(m); // the picks, written once as the menu closes
    if (rw && rw.bad) rw = null;
    if (m.rg && m.rg.drag) clearInterval(m.rg.drag.tm); // a drag cut short: its edge timer stops too
    MN = null;
    $('tkmenu').hidden = true; $('tkmenu').innerHTML = '';
    $('tkcap').hidden = true;
    document.body.classList.remove('tk-menuon');
    if (m.backEl && m.backEl.hasAttribute('aria-haspopup')) m.backEl.setAttribute('aria-expanded', 'false');
    if (rw && rw.draft) {
      rw.draft.start = m.rg.s; rw.draft.due = m.rg.d;
      if (!quiet) { render(); el = m.back && document.querySelector(m.back); }
    }
    if (refocus && el) {
      if (!el.matches('a[href], button, input, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]');
      if (el) el.focus({ preventScroll: true });
    }
    if (lw) labelWrite(lw);
    if (rw && rw.ev) commit(rw.ev);
  }
  // The back row (and ArrowLeft) of a step opened from the main menu. On the range picker it applies
  // the pick: an inverted one is refused, a changed one written (the written batch closes the item
  // menu), and with nothing changed it steps back.
  function menuBack() {
    if (!MN.from) { closeMenu(true); return; }
    if (MN.step === 'due' && MN.rg) {
      var p = rangePending(MN);
      if (p && p.bad) { rangeRefused(); return; }
      if (p && p.ev) { var m = MN; m.rsent = true; commit(p.ev, null, null, function () { m.rsent = false; }); return; }
    }
    MN.step = MN.from; MN.from = null; MN.op = null; MN.who = null; MN.req = null; MN.q = ''; MN.as = null; MN.mv = null; MN.rg = null;
    drawMenu();
  }
  function drawMenu() {
    var m = $('tkmenu');
    m.innerHTML = MN.kind === 'sync' ? syncPopHtml() : MN.kind === 'new' ? newMenuHtml() : MN.kind === 'store' ? storeMenuHtml() : menuHtml();
    m.className = 'tk tk-menu' + (MN.kind === 'sync' ? ' spopw' : MN.step === 'assign' ? ' wide' : MN.step === 'due' && MN.rg && MN.rg.rm ? ' rwide' : '');
    m.setAttribute('role', MN.kind === 'sync' || MN.step === 'assign' || MN.step === 'due' ? 'dialog' : 'menu');
    m.setAttribute('aria-label', MN.kind === 'sync' ? wsT("js.tk.rib.tip") : MN.kind === 'store' ? wsT("js.tk.store.menu") : wsT("js.tk.menu.aria", mnItem() ? mnItem().title : ''));
    m.hidden = false;
    placeMenu();
    var first = m.querySelector('[data-autofocus]') || m.querySelector('[data-mi]:not([disabled]), button:not([disabled]), input');
    if (first) first.focus({ preventScroll: true });
  }
  // At the pointer for a right-click, else under the opener (above it when below does not fit),
  // kept inside the window.
  function placeMenu() {
    var m = $('tkmenu'), w = m.offsetWidth, h = m.offsetHeight, W = window.innerWidth, Hh = window.innerHeight, x, y;
    if (MN.pt) { x = MN.pt.x; y = MN.pt.y; if (y + h > Hh - 6) y = Math.max(6, MN.pt.y - h); }
    else if (MN.rect) {
      var r = MN.rect;
      x = r.left; y = r.bottom + 4;
      if (y + h > Hh - 6) y = r.top - h - 4 >= 6 ? r.top - h - 4 : Math.max(6, Hh - h - 6);
    } else { x = (W - w) / 2; y = (Hh - h) / 2; }
    x = Math.max(6, Math.min(x, W - w - 6));
    m.style.left = Math.round(x) + 'px'; m.style.top = Math.round(Math.max(6, y)) + 'px';
    placeCap();
  }
  function mi(act, label, data, o) {
    o = o || {};
    return '<button type="button" class="mi' + (o.cls ? ' ' + o.cls : '') + '" role="menuitem" data-mi' + attr('act', act) + dataAttrs(data) + (o.sub ? ' data-sub aria-haspopup="menu"' : '') + '>' +
      (o.icon ? '<svg class="i"><use href="#tki-' + o.icon + '"/></svg>' : '<span class="i"></span>') + '<span class="ml">' + label + '</span>' +
      (o.val ? '<span class="mv">' + o.val + '</span>' : '') + (o.sub ? '<svg class="i mch"><use href="#tki-chev"/></svg>' : '') + '</button>';
  }
  function msep() { return '<div class="msep" role="separator"></div>'; }
  // the priority step's radios (an item's, or the quick-add row's draft), cur the checked level
  function priGroup(cur) {
    return '<div role="group" aria-label="' + wsT("js.tk.menu.pri") + '">' + VS.priority.names.map(function (x, i) {
      var on = i === cur;
      return '<button type="button" class="mi mr" role="menuitemradio" aria-checked="' + on + '" data-mi' + attr('act', 'm-pri') + attr('p', i) + (on ? ' data-autofocus' : '') + '><span class="rdo"></span><span class="ml">' + esc(x) + '</span>' +
        (i === VS.priority.default ? '<span class="mv">' + wsT("js.tk.menu.default") + '</span>' : '') + '</button>';
    }).join('') + '</div>';
  }
  function noteField(ph) {
    return '<label class="mnote"><input type="text" id="tkmnote" data-mi autocomplete="off" placeholder="' + esc(ph) + '" aria-label="' + esc(wsT("js.tk.form.note.opt")) + '"><span class="mk" aria-hidden="true">&#9166;</span></label>';
  }
  function menuHtml() {
    var it = VS.items[MN.item], v = VS.viewer.member, h = '';
    if (!it) return '';
    h += '<div class="mh">' + idCopyBtn(it) + '<b>' + esc(it.title) + '</b></div>';
    var back = MN.from ? mi('m-back', wsT("js.tk.menu.back"), {}, { icon: 'back', cls: 'mback' }) + msep() : '';
    var mineR = mineLive(it), step = MN.step;
    if (step === 'pri') return h + back + priGroup(it.priority);
    if (step === 'labels') return h + back + labelStepHtml(it);
    if (step === 'due') return h + back + rangeHtml(rangeOf(MN));
    if (step === 'handoff') {
      var hfrom = MN.who || v, hrec = it.assignments.map(A).filter(function (r) { return r.to === hfrom && r.state === 'live'; })[0] || {};
      return h + back + '<div class="mtitle">' + (hfrom === v ? wsT("js.tk.dlg.handoff.title", esc(it.alias)) : wsT("js.tk.dlg.handoff.title.admin", esc(nm(hfrom)), esc(it.alias))) + '</div>' +
        '<p class="mintro">' + (hfrom === v ? wsT("js.tk.dlg.handoff.intro", esc(nm(hrec.by))) : wsT("js.tk.dlg.handoff.intro.admin", esc(nm(hfrom)), esc(nm(hrec.by)))) + '</p>' +
        '<div class="mrow"><input type="text" id="tkmq" data-mi data-autofocus autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.menu.search") + '" aria-label="' + wsT("js.tk.menu.search") + '" value="' + esc(MN.q || '') + '"></div>' +
        '<div class="mlist" role="group">' + handoffList(it, hfrom) + '</div>' + noteField(wsT("js.tk.form.note.opt"));
    }
    if (step === 'cc') {
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.cc.title", esc(it.alias)) + '</div><p class="mintro">' + wsT("js.tk.dlg.cc.intro") + '</p>' +
        '<div class="mrow"><input type="text" id="tkmq" data-mi data-autofocus autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.menu.search") + '" aria-label="' + wsT("js.tk.menu.search") + '" value="' + esc(MN.q || '') + '"></div>' +
        '<div class="mlist" role="group">' + ccPickList(it) + '</div>';
    }
    if (step === 'opening') {
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.open.title", esc(it.alias)) + '</div><p class="mintro">' + wsT("js.tk.dlg.open.intro") + '</p>' +
        '<div role="group">' + mi('m-opening', esc(wsT("js.tk.open.scope.team")), { scope: 'team' }, { icon: 'team', val: esc(wsT("js.tk.open.scope.team.sub")) }) +
        mi('m-opening', esc(wsT("js.tk.open.scope.store")), { scope: 'store' }, { icon: 'tasks', val: esc(wsT("js.tk.open.scope.store.sub")) }) + '</div>';
    }
    if (step === 'request') {
      var tg = reqTargets(it).filter(function (r) { return !MN.who || r.to === MN.who; });
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.request.title", esc(it.alias)) + '</div><p class="mintro">' + wsT("js.tk.dlg.request.intro") + '</p>' +
        '<div class="mlist" role="group">' + (tg.length ? tg.map(function (r) {
          return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-request') + attr('to', r.to) + '>' + avHtml(r.to) + '<span class="ml">' + whoName(r.to) + '</span><span class="mv">' + esc(stLabel(r.status)) + '</span></button>';
        }).join('') : '<p class="mempty">' + wsT("js.tk.menu.nobody") + '</p>') + '</div>' + noteField(wsT("js.tk.form.note.opt"));
    }
    if (step === 'wrap') {
      // another member's assignment: an org superior's wrap-up set, or the assigner's drop (wrapAuth)
      var wrec = it.assignments.map(A).filter(function (r) { return r.to === MN.who && r.state === 'live'; })[0], wa = wrapAuth(it, wrec);
      if (!wa) return h + back + '<p class="mempty">' + wsT("js.tk.win.noacts", esc(nm(v))) + '</p>';
      return h + back + '<div class="mtitle">' + wsT("js.tk.dlg.wrap.title", esc(nm(MN.who)), esc(it.alias)) + '</div><p class="mintro">' + wsT(wa === 'sup' ? "js.tk.dlg.wrap.intro" : "js.tk.dlg.wrap.intro.drop", esc(nm(MN.who))) + '</p>' +
        noteField(wsT("js.tk.form.note.opt")) + '<div role="group">' + WRAP_SET[wa].map(function (s, i) {
          return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-wrap') + attr('status', s) + (i ? '' : ' data-autofocus') + '>' + SI.svg(s) + '<span class="ml">' + esc(stLabel(s)) + '</span></button>';
        }).join('') + '</div>';
    }
    if (step === 'note') {
      var L = { 'item.reject': ["js.tk.dlg.reject.title", "js.tk.dlg.reject.intro"], 'item.reopen': ["js.tk.dlg.reopen.title", "js.tk.dlg.reopen.intro"],
        'item.decline': ["js.tk.dlg.decline.title", "js.tk.dlg.decline.intro"], 'item.return': ["js.tk.dlg.return.title", "js.tk.dlg.return.intro"],
        'item.deny': ["js.tk.dlg.deny.title", "js.tk.dlg.deny.intro"] }[MN.op];
      return h + back + '<div class="mtitle">' + wsT(L[0], esc(nm(MN.who || v)), esc(it.alias)) + '</div><p class="mintro">' + wsT(L[1]) + '</p>' +
        '<div class="mrow">' + noteField(wsT("js.tk.form.note")) + btn('m-note-send', wsT("js.tk.dlg.note.submit"), {}, 'primary') + '</div>';
    }
    if (step === 'assign') return h + back + asgHtml(it);
    if (step === 'move') return h + back + moveStepHtml(it);
    // the main step: the status part (radios that apply on click, a note that rides along), then the actions
    var parts = [];
    if (canStatus(it)) {
      var cur = mineR.status, pick = MN.pick || cur, whys = [];
      parts.push('<div class="mst" role="group" aria-label="' + wsT("js.tk.prop.status") + '">' + ownStatusOpts(mineR).map(function (o) {
        var on = o.s === cur;
        if (o.dis) whys.push(o.why);
        return '<button type="button" class="mr' + (o.s === pick && !on ? ' pick' : '') + '" role="menuitemradio" aria-checked="' + on + '" data-mi' + attr('act', 'm-status') + attr('status', o.s) + (o.dis ? ' disabled' : '') + (on ? ' data-autofocus' : '') + '>' +
          SI.svg(o.s) + '<span>' + esc(o.label) + '</span></button>';
      }).join('') + '</div>' + (whys.length ? '<p class="mintro mwhy">' + whys.map(esc).join('<br>') + '</p>' : '') + noteField(wsT("js.tk.menu.note")));
    }
    if (MN.statusOnly) return h + (parts.join('') || '<p class="mempty">' + wsT("js.tk.win.noacts", esc(nm(v))) + '</p>');
    var acts = actList(it).map(function (x) {
      return mi(x[0], esc(x[2]), x[3], { icon: x[1], cls: x[4] ? 'go' : '' });
    });
    if (mineR && wrapped(mineR.status) && !it.hidden) acts.push(mi('note', esc(wsT("js.tk.asg.reopen")), { item: it.id, who: v, op: 'item.reopen' }, { icon: 'return' }));
    if (acts.length) parts.push(acts.join(''));
    // the item window's own header menu: "add subtask" and "hand off" sit in the header beside it
    var inWin = ui.win && ui.item === it.id && MN.backEl && MN.backEl.closest && !!MN.backEl.closest('.iw-head');
    var edit = canEdit(it), task = it.kind !== 'topic', more = [];
    if (canSplit(it) && !inWin) more.push(mi('m-sub', wsT("js.tk.menu.sub"), { item: it.id }, { icon: 'plus' }));
    if (edit && task) {
      more.push(mi('pri', wsT("js.tk.menu.pri"), { item: it.id }, { icon: 'flag', sub: 1, val: esc(it.priorityName) }));
      more.push(mi('due', wsT("js.tk.menu.due"), { item: it.id }, { icon: 'calendar', sub: 1, val: esc(rangeText(it.start, it.due)) }));
    }
    if (mineR && !wrapped(mineR.status) && !mineR.fallback && !it.hidden && !inWin) more.push(mi('handoff', esc(wsT("js.tk.act.handoff")), { item: it.id }, { icon: 'handoff', sub: 1 }));
    // a topic's holder releases it: item.unassign of their own record
    if (mineR && !task && !it.hidden) more.push(mi('unassign', esc(wsT("js.tk.act.release")), { item: it.id, who: v }, { icon: 'withdraw' }));
    if (canAssign(it, v)) more.push(mi('assign', esc(wsT("js.tk.act.assign")), { item: it.id }, { icon: 'assign', sub: 1 }));
    if (canOpen(it)) more.push(mi('opening', esc(wsT("js.tk.act.open")), { item: it.id }, { icon: 'team', sub: 1 }));
    if (canUnopen(it)) more.push(mi('unopen', esc(wsT("js.tk.act.unopen")), { item: it.id }, { icon: 'withdraw' }));
    if (reqTargets(it).length) more.push(mi('request', esc(wsT("js.tk.act.request")), { item: it.id }, { icon: 'handoff', sub: 1 }));
    var mvr = mvMenuRoots(it.id);
    if (mvr.some(function (r) { return canMove(VS.items[r]); })) more.push(mi('move', esc(mvr.length > 1 ? wsTn("js.tk.mv.menu.sel", mvr.length) : wsT("js.tk.mv.menu")), { item: it.id }, { icon: 'tree', sub: 1 }));
    if (more.length) parts.push(more.join(''));
    var last = [];
    // the admin wording only where the creator-gone fallback is the sole reason the viewer may act
    if (edit) last.push(mi('edit', esc(it.author === v || mineLive(it) ? wsT("js.tk.act.edit") : wsT("js.tk.act.edit.admin")), { item: it.id }, { icon: 'pen' }));
    if (!(ui.win && ui.item === it.id)) last.push(mi('m-open', esc(wsT("js.tk.menu.open")), { item: it.id }, { icon: 'page' }));
    if (canHide(it)) last.push(mi('ihide', esc(it.author === v ? wsT("js.tk.act.hide") : wsT("js.tk.act.hide.admin")), { item: it.id }, { icon: 'x', cls: 'danger' }));
    if (last.length) parts.push(last.join(''));
    return h + (parts.length ? parts.join(msep()) : '<p class="mempty">' + wsT("js.tk.win.noacts", esc(nm(v))) + '</p>');
  }
  // The handoff step's people: anyone but the holder and removed members, filtered by the search.
  function handoffList(it, from) {
    var q = String(MN.q || '').trim().toLowerCase();
    var ids = Object.keys(VS.members).filter(function (id) { var m = VS.members[id]; return id !== from && !m.removed && (!q || (id + ' ' + nmo(id)).toLowerCase().indexOf(q) >= 0); });
    if (!ids.length) return '<p class="mempty">' + wsT("js.tk.menu.nobody") + '</p>';
    return ids.map(function (id) {
      return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-handoff') + attr('to', id) + '>' + avHtml(id) + '<span class="ml">' + whoName(id) + '</span><span class="mv">' + esc(id) + '</span></button>';
    }).join('');
  }
  // The CC step's people: every member not removed and not CC'd yet, the viewer included, filtered by the search.
  function ccPickList(it) {
    var q = String(MN.q || '').trim().toLowerCase(), cc = listOf(it.cc);
    var ids = Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed && cc.indexOf(id) < 0 && (!q || (id + ' ' + nmo(id)).toLowerCase().indexOf(q) >= 0); });
    if (!ids.length) return '<p class="mempty">' + wsT("js.tk.menu.nobody") + '</p>';
    return ids.map(function (id) {
      return '<button type="button" class="mi" role="menuitem" data-mi' + attr('act', 'm-cc') + attr('to', id) + '>' + avHtml(id) + '<span class="ml">' + whoName(id) + '</span><span class="mv">' + esc(id) + '</span></button>';
    }).join('');
  }
  // the people list a search box (#tkmq) filters: the CC step's or the handoff step's
  function mqList(it) { return MN.step === 'cc' ? ccPickList(it) : handoffList(it, MN.who || VS.viewer.member); }
  // "Move to..." (ticket decision 8): the item menu's move step, the keyboard and touch path of the
  // move drag. A searchable tree of the destinations the store takes (moveWhy) plus the top level;
  // an item that takes none shows only as the context line of one that does, so the item's own
  // subtree stays out. A pick puts what moves last under the choice. Opened from a selected list
  // row it moves the selection's roots: a destination shows when it takes one of them, with "N of
  // M" and the reasons when not all; the roots no destination takes are named up front.
  function mvMenuRoots(id) {
    var row = MN && MN.backEl && MN.backEl.closest && MN.backEl.closest('#card-items .irow');
    return row && mselHas(id) ? moveRoots(mselIds()) : [id];
  }
  function moveStepHtml(it) {
    var roots = MN.mv || (MN.mv = mvMenuRoots(it.id)), n = roots.length;
    var stuck = roots.map(function (r) { return [r, moveWhy(r, VS.items[r] ? VS.items[r].parent || null : null)]; }).filter(function (x) { return x[1]; });
    return '<div class="mtitle">' + (n > 1 ? esc(wsTn("js.tk.mv.title.sel", n)) : esc(wsT("js.tk.mv.title", it.alias))) + '</div><p class="mintro">' + wsT("js.tk.mv.intro") + '</p>' +
      (stuck.length ? '<p class="mintro mvstuck">' + stuck.map(function (x) { return wsT("js.tk.mv.stays", esc(VS.items[x[0]] ? VS.items[x[0]].alias : alias(x[0])), esc(x[1])); }).join('<br>') + '</p>' : '') +
      '<div class="mrow"><input type="text" id="tkmvq" data-mi data-autofocus autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.mv.search") + '" aria-label="' + wsT("js.tk.mv.search") + '" value="' + esc(MN.q || '') + '"></div>' +
      '<div class="mlist" role="group">' + moveList(roots) + '</div>';
  }
  function moveList(roots) {
    var q = String(MN.q || '').trim().toLowerCase(), kids = {}, h = '';
    function takes(dest) { return roots.filter(function (r) { return !moveWhy(r, dest); }); }
    function hit(it) { return !q || [it.title, it.alias, it.id, it.project].some(function (x) { return String(x == null ? '' : x).toLowerCase().indexOf(q) >= 0; }); }
    function opt(id, depth, label, ok) {
      var cur = roots.every(function (r) { return (VS.items[r].parent || '') === id; }), part = ok.length < roots.length;
      var tip = part ? roots.filter(function (r) { return ok.indexOf(r) < 0; }).map(function (r) { return wsT("js.tk.mv.stays", VS.items[r].alias, moveWhy(r, id || null)); }).join('; ') : '';
      return '<button type="button" class="mi mvo" role="menuitem" data-mi' + attr('act', 'm-move') + attr('to', id) + ' style="--ind:' + depth + '"' + (tip ? tipAttr(tip) : '') + '><span class="ml">' + esc(label) + '</span>' +
        '<span class="mv">' + [cur ? esc(wsT("js.tk.mv.cur")) : '', part ? esc(wsT("js.tk.mv.part", ok.length, roots.length)) : '', id ? esc(VS.items[id].alias) : ''].filter(Boolean).join(' &middot; ') + '</span></button>';
    }
    var top = takes(null);
    if (top.length && (!q || wsT("js.tk.mv.top").toLowerCase().indexOf(q) >= 0)) h += opt('', 0, wsT("js.tk.mv.top"), top);
    // hidden items, and so their subtrees, are left out, as the lists leave them
    VS.itemOrder.forEach(function (id) { var it = VS.items[id]; if (it && !it.hidden) (kids[it.parent || ''] = kids[it.parent || ''] || []).push(id); });
    function walk(id, depth) {
      var it = VS.items[id], ok = takes(id), on = ok.length > 0 && hit(it), sub = (kids[id] || []).map(function (k) { return walk(k, depth + 1); }).join('');
      if (!on && !sub) return '';
      return (on ? opt(id, depth, it.title, ok) : '<div class="mvctx" style="--ind:' + depth + '">' + esc(it.title) + ' <span class="mv">' + esc(it.alias) + '</span></div>') + sub;
    }
    h += (kids[''] || []).map(function (id) { return walk(id, 0); }).join('');
    return h || '<p class="mempty">' + wsT("js.tk.mv.none") + '</p>';
  }
  // The assign popover (530): the item menu's assign step, opened from the menu or anchored at
  // the Assignees "+" (openAssign). A search field over people and subtree rules, the picked
  // targets as chips, then the completion mode and the apply button, disabled while nothing is
  // picked. Rules: "My team" (everyone within L1 / L2 below me, or my whole subtree) while the
  // search is empty, and, for a member the search finds who leads anyone, the whole subtree below
  // them, pending when they are outside my reach (a rule request they accept). People: avatar,
  // name, id, and whether the assignment is live at once or pending. One rule per root: picking
  // another drops the first. A person already holding the item, or a rule one of my live rules
  // there covers, shows as assigned and is not picked. Focus stays in the search field: arrows
  // move, Enter toggles (Cmd/Ctrl+Enter applies), Esc goes back or closes (menuKey).
  // compl stays null until the completion radio is touched; until then asgCompl derives it (58).
  // A topic takes one holder: people only, picking one replaces the last, and no completion
  // mode (the store keeps none for a topic; the payload still carries the derived one).
  function asgState() { return MN.as || (MN.as = { q: '', sel: [], compl: null, idx: 0, opts: [] }); }
  // The default completion of one target (the writers' rule, as the MCP writer's completionClasses):
  // a member below the viewer in the org tree, or a rule rooted at the viewer or below them, takes
  // assigner, so a close goes through review; the viewer themself or a peer takes assignee.
  function complClass(o) {
    var id = o.t === 'r' ? o.root : o.id;
    return (o.t === 'r' && id === VS.viewer.member) || VS.viewer.reach[id] ? 'assigner' : 'assignee';
  }
  // The completion one target is written with: assigner for a target below the viewer whatever the
  // pick, since the store refuses assignee there (check.go checkWrite: the viewer confirms that
  // wrap-up); for anyone else the user's pick, else the default class.
  function asgClass(o, compl) { return complClass(o) === 'assigner' ? 'assigner' : compl || 'assignee'; }
  // whether a pick is below the viewer: the completion radio then offers no assignee
  function asgBelow(S) { return S.sel.some(function (o) { return complClass(o) === 'assigner'; }); }
  // The picked targets as the item.assign events they write, one per completion class (asgClass),
  // the first target's class first, each with its completion stored explicitly. Within an event,
  // people come first, then rules ({root, levels?}; the whole subtree without levels).
  function asgEvents(item, sel, compl) {
    var groups = [], by = {};
    sel.forEach(function (o) {
      var c = asgClass(o, compl);
      if (!by[c]) groups.push(by[c] = { c: c, sel: [] });
      by[c].sel.push(o);
    });
    return groups.map(function (g) {
      var people = g.sel.filter(function (o) { return o.t === 'p'; }).map(function (o) { return o.id; });
      var rules = g.sel.filter(function (o) { return o.t === 'r'; }).map(function (o) { return o.levels == null ? { root: o.root } : { root: o.root, levels: o.levels }; });
      return ['item.assign', { item: item, targets: people.concat(rules), completion: g.c }];
    });
  }
  // The completion mode the radio shows: the picks' classes (asgClass); 'mixed' when they fall in
  // both (written as one assign per class); the user's pick, or null, while nothing is picked.
  function asgCompl(S) {
    var cs = {};
    S.sel.forEach(function (o) { cs[asgClass(o, S.compl)] = true; });
    var k = Object.keys(cs);
    return k.length > 1 ? 'mixed' : k[0] || S.compl || null;
  }
  // everyone below root (removed members left out), by depth
  function subtreeOf(root) {
    if (root === VS.viewer.member) return VS.viewer.reach;
    var out = {}, q = [[root, 0]];
    while (q.length) {
      var c = q.shift(), m = VS.members[c[0]];
      (m ? m.children : []).forEach(function (k) {
        if (k in out || k === root || !VS.members[k] || VS.members[k].removed) return;
        out[k] = c[1] + 1; q.push([k, c[1] + 1]);
      });
    }
    return out;
  }
  function inRange(sub, levels) { return Object.keys(sub).filter(function (k) { return levels == null || sub[k] <= levels; }).length; }
  // the store's own cover test (engine ruleCovers) for a rule without include_root
  function ruleCovered(it, root, levels) {
    var v = VS.viewer.member;
    return vals(VS.rules || {}).some(function (y) {
      if (y.item !== it.id || y.by !== v || y.state !== 'live') return false;
      var d = y.root === root ? 0 : subtreeOf(y.root)[root];
      if (d == null) return false;
      return y.levels == null || (levels != null && d + levels <= y.levels);
    });
  }
  function asgKey(o) { return o.t === 'p' ? 'p:' + o.id : 'r:' + o.root + ':' + (o.levels == null ? '*' : o.levels); }
  function asgLabel(o) {
    var v = VS.viewer.member;
    if (o.t === 'p') return esc(nm(o.id));
    if (o.root !== v) return wsT("js.tk.ap.rule.of", esc(nm(o.root)));
    return o.levels == null ? wsT("js.tk.ap.rule.all") : wsT("js.tk.ap.rule.lv", o.levels);
  }
  // the options in list order, with the group headings in between
  function asgOpts(it) {
    var S = asgState(), v = VS.viewer.member, R = VS.viewer.reach, q = S.q.trim(), out = [];
    var held = {};
    it.assignments.map(A).forEach(function (r) { if (r.state === 'live' || r.state === 'pending') held[r.to] = true; });
    function rule(root, levels) {
      var sub = subtreeOf(root);
      return { t: 'r', root: root, levels: levels, n: inRange(sub, levels), pend: root !== v && !R[root], off: ruleCovered(it, root, levels) };
    }
    var topic = it.kind === 'topic', norule = topic || it.draft; // the quick-add row assigns people only
    if (!q && !norule) {
      var depth = Math.max.apply(null, [0].concat(vals(R)));
      if (depth) {
        out.push({ g: wsT("js.tk.ap.myteam") }, rule(v, 1));
        if (depth >= 2) out.push(rule(v, 2));
        out.push(rule(v, null));
      }
    }
    var ids = q ? mentionMatches(q) : Object.keys(VS.members).filter(function (id) { return !VS.members[id].removed; }).sort(function (a, b) {
      if (a === v || b === v) return a === v ? -1 : 1;
      return (R[a] || 99) - (R[b] || 99) || (a < b ? -1 : 1);
    });
    if (q && !norule) {
      var leads = ids.filter(function (id) { return id !== v && inRange(subtreeOf(id), null); });
      if (leads.length) out.push({ g: wsT("js.tk.ap.teams") });
      leads.forEach(function (id) { out.push(rule(id, null)); });
    }
    if (ids.length) out.push({ g: wsT("js.tk.ap.people") });
    ids.forEach(function (id) { out.push({ t: 'p', id: id, pend: id !== v && !R[id], off: !!held[id] }); });
    return out;
  }
  function asgListHtml(it) {
    var S = asgState(), sel = {};
    S.sel.forEach(function (o) { sel[asgKey(o)] = true; });
    S.opts = asgOpts(it);
    var pickable = S.opts.filter(function (o) { return !o.g && !o.off; });
    if (S.idx >= pickable.length) S.idx = Math.max(0, pickable.length - 1);
    var n = -1;
    var h = S.opts.map(function (o) {
      if (o.g) return '<li class="agrp" role="presentation">' + esc(o.g) + '</li>';
      var on = !!sel[asgKey(o)], i = o.off ? -1 : ++n, mark = o.off ? '<span class="amk">' + wsT("js.tk.ap.has") + '</span>' :
        o.t === 'p' ? '<span class="amk' + (o.pend ? ' pend' : '') + '">' + reachTag(VS.viewer.member, o.id) + '</span>' : o.pend ? '<span class="amk pend">' + wsT("js.tk.st.pending") + '</span>' : '';
      var lead = o.t === 'p' ? avHtml(o.id) + '<span class="an">' + whoName(o.id) + '</span><code class="aid">' + esc(o.id) + '</code>' :
        '<svg class="i"><use href="#tki-team"/></svg><span class="an">' + asgLabel(o) + '</span><span class="aid">' + esc(wsTn("js.tk.ap.n", o.n)) + '</span>';
      return '<li class="aopt' + (i === S.idx ? ' on' : '') + (o.off ? ' off' : '') + '" role="option"' + (i >= 0 ? ' id="tkaopt-' + i + '"' + attr('act', 'm-atog') + attr('ai', i) : ' aria-disabled="true"') +
        ' aria-selected="' + on + '"><span class="ack" aria-hidden="true"></span>' + lead + mark + '</li>';
    }).join('');
    return h || '<li class="qnone" role="presentation">' + wsT("js.tk.ap.none") + '</li>';
  }
  function asgChipsHtml() {
    return asgState().sel.map(function (o) {
      return '<span class="achip">' + (o.t === 'p' ? avHtml(o.id) : '<svg class="i"><use href="#tki-team"/></svg>') + '<span>' + asgLabel(o) + '</span>' +
        '<button type="button" class="ax"' + attr('act', 'm-ax') + attr('k', asgKey(o)) + ' aria-label="' + esc(wsT("js.tk.ap.rm", o.t === 'p' ? nm(o.id) : asgLabel(o).replace(/<[^>]*>/g, ''))) + '"><svg class="i"><use href="#tki-x"/></svg></button></span>';
    }).join('');
  }
  function asgHtml(it) {
    var S = asgState(), list = asgListHtml(it), any = S.opts.some(function (o) { return !o.g && !o.off; }), topic = it.kind === 'topic';
    return '<div class="ap"><div class="ap-chips" id="tkachips">' + asgChipsHtml() + '</div>' +
      '<label class="ap-q"><svg class="i"><use href="#tki-search"/></svg><input type="text" id="tkaq" data-autofocus autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="tkalist" aria-autocomplete="list"' +
      (any ? ' aria-activedescendant="tkaopt-' + S.idx + '"' : '') + ' placeholder="' + wsT("js.tk.ap.search") + '" aria-label="' + wsT("js.tk.ap.search") + '" value="' + esc(S.q) + '"></label>' +
      '<ul class="ap-list" id="tkalist" role="listbox" aria-multiselectable="' + !topic + '" aria-label="' + esc(wsT("js.tk.act.assign")) + '">' + list + '</ul>' +
      '<div class="ap-foot">' + (topic ? '<span class="muted">' + wsT("js.tk.ap.topic") + '</span>' : '<span class="ap-cm" role="radiogroup" aria-label="' + wsT("js.tk.form.completion") + '">' + ['assignee', 'assigner'].map(function (c) {
        return '<button type="button" role="radio" aria-checked="' + (asgCompl(S) === c) + '"' + attr('act', 'm-acm') + attr('c', c) + (c === 'assignee' && asgBelow(S) ? ' hidden' : '') + '>' + esc(complText(c)) + '</button>';
      }).join('') + '</span><span class="ap-mix muted" id="tkamix"' + tipAttr(wsT("js.tk.ap.compl.mixed.tip")) + (asgCompl(S) === 'mixed' ? '' : ' hidden') + '>' + wsT("js.tk.ap.compl.mixed") + '</span>') + '<button type="button" class="btn primary" id="tkaapply"' + attr('act', 'm-aapply') + (S.sel.length ? '' : ' disabled') + '>' + wsT("js.tk.dlg.assign.submit") + '</button></div></div>';
  }
  // the list, the chips, and the apply button again; the search field (and an IME composition in it) stays
  function asgDraw() {
    var it = mnItem(), S = asgState();
    if (!it || !$('tkalist')) return;
    $('tkalist').innerHTML = asgListHtml(it);
    $('tkachips').innerHTML = asgChipsHtml();
    $('tkaapply').disabled = !S.sel.length;
    $('tkaq').setAttribute('aria-activedescendant', $('tkaopt-' + S.idx) ? 'tkaopt-' + S.idx : '');
    var on = $('tkalist').querySelector('.aopt.on');
    if (on) on.scrollIntoView({ block: 'nearest' });
    Array.prototype.forEach.call(document.querySelectorAll('#tkmenu .ap-cm [role="radio"]'), function (b) {
      b.setAttribute('aria-checked', String(b.getAttribute('data-c') === asgCompl(S)));
      b.hidden = b.getAttribute('data-c') === 'assignee' && asgBelow(S); // a pick below the viewer: assigner only
    });
    if ($('tkamix')) $('tkamix').hidden = asgCompl(S) !== 'mixed';
  }
  function asgToggle(i) {
    var S = asgState(), o = S.opts.filter(function (x) { return !x.g && !x.off; })[i];
    if (!o) return;
    var k = asgKey(o), had = S.sel.some(function (x) { return asgKey(x) === k; });
    var one = mnItem() && mnItem().kind === 'topic';
    S.sel = one ? [] : S.sel.filter(function (x) { return asgKey(x) !== k && !(o.t === 'r' && x.t === 'r' && x.root === o.root); });
    if (!had) S.sel.push({ t: o.t, id: o.id, root: o.root, levels: o.levels });
    S.idx = i;
    asgDraw();
  }
  // the picks as item.assign events (asgEvents): one, or one per completion class
  function asgApply() {
    var S = asgState(), it = mnItem();
    if (!it || !S.sel.length) return;
    // a chip's picker: the picks go to the row, sent with its create
    if (it.draft) { var n = nd(); n.who = S.sel.map(function (o) { return o.id; }); n.compl = S.compl; return newPicked(); }
    commit(asgEvents(it.id, S.sel, S.compl));
  }
  function asgMenuKey(e) {
    var t = e.target, S = asgState(), k = e.key;
    if (t.id === 'tkaq') {
      if (e.isComposing || e.keyCode === 229) return true;
      var n = S.opts.filter(function (x) { return !x.g && !x.off; }).length;
      if (k === 'ArrowDown' || k === 'ArrowUp') { e.preventDefault(); if (n) { S.idx = (S.idx + (k === 'ArrowDown' ? 1 : n - 1)) % n; asgDraw(); } return true; }
      if (k === 'Enter') { e.preventDefault(); if (e.metaKey || e.ctrlKey) asgApply(); else asgToggle(S.idx); return true; }
      return false;
    }
    if ((k === 'ArrowLeft' || k === 'ArrowRight') && t.closest('.ap-cm')) {
      e.preventDefault();
      var nx = t.parentNode.querySelector('[data-c]:not([hidden]):not([data-c="' + t.getAttribute('data-c') + '"])');
      if (nx) { S.compl = nx.getAttribute('data-c'); asgDraw(); nx.focus(); }
      return true;
    }
    return false;
  }
  function menuNote() { var n = $('tkmnote'); return n ? n.value.trim() : ''; }
  // A status from the menu: done closes (it waits for the assigner when the completion says so).
  function applyStatus(s) {
    if (!setStatus(VS.items[MN.item], s, menuNote())) closeMenu(true);
  }
  // Writes the viewer's status on it with an optional note; false when there is nothing to write.
  function setStatus(it, s, note) {
    var origin = CAPS.origin && $('corigin') ? $('corigin').value : 'human';
    var r = it && mineLive(it);
    if (!r) return false;
    if (s === r.status && !note) return false;
    var p = { item: it.id };
    if (note) p.note = note;
    if (s === 'done') commit([['item.close', p, origin]]);
    else { p.status = s; commit([['item.status', p, origin]]); }
    return true;
  }
  // The menu's own acts (m-*); the rest of a menu item's data-act goes through the shared handlers.
  function menuAct(k, a) {
    if (MN.kind === 'store') return storeAct(k);
    var id = MN.item, it = mnItem();
    if (k === 'm-back') return menuBack();
    if (!it) return closeMenu(true);
    if (k === 'm-rpage' || k === 'm-rclr') return rangeAct(k, a);
    if (MN.kind === 'new' && !/^m-(atog|ax|acm|aapply)$/.test(k)) return newAct(k, a);
    switch (k) {
      case 'm-atog': asgToggle(Number(a.getAttribute('data-ai'))); return $('tkaq').focus();
      case 'm-ax': {
        var S = asgState(), ak = a.getAttribute('data-k');
        S.sel = S.sel.filter(function (x) { return asgKey(x) !== ak; });
        asgDraw(); return $('tkaq').focus();
      }
      case 'm-acm': asgState().compl = a.getAttribute('data-c'); return asgDraw();
      case 'm-aapply': return asgApply();
      case 'm-status': return applyStatus(a.getAttribute('data-status'));
      case 'm-pri': {
        var pv = Number(a.getAttribute('data-p'));
        if (pv === it.priority) return menuBack();
        return commit([['item.edit', { item: id, priority: pv }]]);
      }
      case 'm-ltog': MN.lab = a.getAttribute('data-label'); MN.lset = labelFlip(MN.lset, labelBase(it), MN.lab); return labelStepRedraw();
      case 'm-ledit': return labelDialog({ label: a.getAttribute('data-label') });
      case 'm-lrm': return labelRemove(a.getAttribute('data-label'), function () { if (MN) MN.lab = null; });
      case 'm-ladd': return labelDialog({});
      case 'm-cc': return commit([['item.cc', { item: id, member: a.getAttribute('data-to') }]]);
      case 'm-handoff': {
        var hp = { item: id, to: a.getAttribute('data-to') }, note = menuNote();
        if (MN.who && MN.who !== VS.viewer.member) hp.assignee = MN.who;
        if (note) hp.note = note;
        return commit([['item.handoff', hp]]);
      }
      case 'm-note-send': {
        var np = { item: id }, nt = menuNote();
        if (MN.op === 'item.deny') np.request = MN.req;
        else if (MN.who && MN.op !== 'item.decline') np.assignee = MN.who;
        if (nt) np.note = nt;
        return commit([[MN.op, np]]);
      }
      case 'm-opening': return commit([['item.open', { item: id, scope: a.getAttribute('data-scope') }]]);
      case 'm-wrap': {
        var wp = { item: id, assignee: MN.who, status: a.getAttribute('data-status') }, wn = menuNote();
        if (wn) wp.note = wn;
        return commit([['item.status', wp]]);
      }
      case 'm-request': {
        var rp = { item: id, assignee: a.getAttribute('data-to') }, rn = menuNote();
        if (rn) rp.note = rn;
        return commit([['item.request', rp]]);
      }
      case 'm-sub': {
        // under a list, tree, gantt, or subtask row; a kanban card or a calendar entry has no room
        // for an input row under it, so those (and the window header) use the item window's Subtasks
        var rk = MN.rk && !/^(iw|k|c):/.test(MN.rk) && document.querySelector('.tk [data-rk="' + CSS.escape(MN.rk) + '"]') ? MN.rk : 'iw:' + id;
        closeMenu(false);
        return startSub(id, rk);
      }
      case 'm-open': closeMenu(false); return showItem(id);
      case 'm-move': {
        var mroots = MN.mv || mvMenuRoots(id), slot = { parent: a.getAttribute('data-to') || null, after: undefined };
        if (mroots.length === 1 && moveNoop(mroots[0], slot)) return closeMenu(true);
        return moveApply(mroots, slot);
      }
    }
  }
  // The inline "add subtask" row under rk ('iw:<id>': the item window's Subtasks section).
  function startSub(id, rk) {
    if (rk === 'iw:' + id && !(ui.win && ui.item === id)) showItem(id);
    ui.sub = { item: id, rk: rk, val: '' };
    render();
    if ($('tksub')) $('tksub').focus();
  }
  // Keys inside the menu (see MN).
  function menuKey(e) {
    var m = $('tkmenu'), t = e.target, k = e.key, field = t.matches('input[type="text"], input[type="date"], input:not([type]), textarea, select');
    if (MN.step === 'assign' && asgMenuKey(e)) return;
    if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); menuDiscard(); return; }
    if (k === 'Tab') {
      var fs = Array.prototype.filter.call(m.querySelectorAll('button:not([disabled]), input:not([disabled]), select'), function (x) { return x.offsetParent !== null; });
      if (!$('tkcap').hidden) fs.push($('tkcap')); // the discard capsule, a fixed element beside the menu, comes last
      if (!fs.length) return;
      var i = fs.indexOf(t);
      e.preventDefault();
      fs[(i + (e.shiftKey ? fs.length - 1 : 1)) % fs.length].focus();
      return;
    }
    if (k === 'Enter' && !e.isComposing) {
      if (t.id === 'tkmnote' && MN.step === 'main') { e.preventDefault(); applyStatus(MN.pick || (VS.items[MN.item] && (function (r) { return r ? r.status : ''; })(mineLive(VS.items[MN.item])))); return; }
      if (t.id === 'tkmnote' && MN.step === 'note') { e.preventDefault(); menuAct('m-note-send', t); return; }
      if (t.id === 'tkmnote' && MN.step === 'request') { e.preventDefault(); var p0 = m.querySelector('[data-act="m-request"]'); if (p0) p0.click(); return; }
      if (t.hasAttribute('data-rin')) { e.preventDefault(); menuApplyClose(true); return; }
      if (t.id === 'tkmq') { e.preventDefault(); var p1 = m.querySelector('[data-act="m-handoff"], [data-act="m-cc"]'); if (p1) p1.click(); return; }
      if (t.id === 'tkmvq') { e.preventDefault(); var p2 = m.querySelector('[data-act="m-move"]'); if (p2) p2.click(); return; }
      return; // a button: its click; a form field: the form's submit
    }
    var items = Array.prototype.filter.call(m.querySelectorAll('[data-mi]'), function (x) { return !x.disabled; }), at = items.indexOf(t);
    if ((k === 'ArrowDown' || k === 'ArrowUp') && t.type !== 'date') {
      e.preventDefault();
      if (!items.length) return;
      var n = at < 0 ? (k === 'ArrowDown' ? 0 : items.length - 1) : (at + (k === 'ArrowDown' ? 1 : items.length - 1)) % items.length;
      items[n].focus();
      return;
    }
    if (field) return;
    if (k === 'ArrowRight' || k === 'ArrowLeft') {
      if (t.classList.contains('mr') && t.parentNode.classList.contains('mst')) {
        e.preventDefault();
        // arrowing over the radios picks without applying: the note's Enter then applies the picked status
        var rs = Array.prototype.filter.call(t.parentNode.children, function (x) { return !x.disabled; }), j = rs.indexOf(t), nx = rs[(j + (k === 'ArrowRight' ? 1 : rs.length - 1)) % rs.length];
        MN.pick = nx.getAttribute('data-status');
        rs.forEach(function (x) { x.classList.toggle('pick', x === nx && x.getAttribute('aria-checked') !== 'true'); });
        nx.focus();
      } else if (k === 'ArrowRight' && t.hasAttribute('data-sub')) { e.preventDefault(); t.click(); }
      else if (k === 'ArrowLeft' && MN.from) { e.preventDefault(); menuBack(); }
      return;
    }
    if (k === 'Home' || k === 'End') { e.preventDefault(); if (items.length) items[k === 'Home' ? 0 : items.length - 1].focus(); }
  }

  // ------------------------------------------------------- the date range picker

  // The item menu's 'due' step and the due chip of the new row and the edit row: one picker over
  // an item's start and due dates (MN.rg = { s, d: the picked ends, '' for none; s0, d0: what it
  // opened on; lm, rm: the left and right panel months, rm null with one panel; err: the inline
  // error shows; last: the end last changed, 's' or 'd'; drag: a handle drag in progress }).
  // The span runs from the start's left half-circle handle to the due's right one; dragging a
  // handle moves that end, clamped at the other end. One end alone shows its handle and a dashed
  // ghost for the other, which a drag pulls out into that end. Two panels show when the ends fall
  // in different months, each paged on its own; the panels refit only on an open, a typed date, a
  // clear, or a drag end. Under the grid, a date input and a clear button per end.
  // A plain click on a day moves an end too (rangeClick), so an end whose day no panel shows can
  // still be set; it never inverts the pair. Two panels on different months: the left sets the
  // start, the right the due. Else (one panel, or both on one month) the end nearer the day moves,
  // the due on a tie; with one end set, a day on or after the start sets the due, a day before it
  // moves the start, and mirrored for a lone due; with none set, the click sets the due.
  // Any close applies the pick once (closeMenu): an item's changed ends as one item.edit, a row's
  // draft fields; Esc and the discard capsule drop it (menuDiscard). An inverted pair is refused
  // by every close that can stay open (menuApplyClose).
  function monAdd(m, n) { var y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7)) - 1 + n; return new Date(Date.UTC(y, mo, 1)).toISOString().slice(0, 7); }
  function rangeBad(r) { return !!(r.s && r.d && r.s > r.d); }
  // the picker's state for menu m, set up on first use from what it edits: the item, or the row's draft
  function rangeOf(m) {
    if (m.rg) return m.rg;
    var src = m.kind === 'new' ? nd() : VS.items[m.item], s = (src && src.start) || '', d = (src && src.due) || '';
    var r = m.rg = { s: s, d: d, s0: s, d0: d, lm: null, rm: null, err: false, last: 's', drag: null };
    rangeFit(r);
    r.err = rangeBad(r);
    return r;
  }
  // the panels for the ends as they are: the earlier end's month left and the later one's right
  // when they differ, else one panel on the month of an end that is set (else as it was, else today's)
  function rangeFit(r) {
    var ms = r.s && r.s.slice(0, 7), md = r.d && r.d.slice(0, 7);
    if (ms && md && ms !== md) { r.lm = ms < md ? ms : md; r.rm = ms < md ? md : ms; return; }
    r.lm = ms || md || r.lm || today().slice(0, 7); r.rm = null;
  }
  function rangeHtml(r) {
    return '<div class="rgp' + (r.rm ? ' two' : '') + (r.err ? ' tk-rbad' : '') + '"><div class="rgcal" id="tkrcal">' + rangeCal(r) + '</div>' +
      '<div class="rgin">' + rangeField(r, 's') + rangeField(r, 'd') + '</div>' +
      '<p class="rgerr" id="tkrerr" role="alert"' + (r.err ? '' : ' hidden') + '>' + wsT("js.tk.range.err") + '</p></div>';
  }
  function rangeField(r, side) {
    var lab = wsT(side === 's' ? "js.tk.range.start" : "js.tk.range.due"), clr = wsT(side === 's' ? "js.tk.range.clear.start" : "js.tk.menu.due.clear");
    return '<div class="rgf"><label for="tkr' + side + '">' + lab + '</label><input type="date" id="tkr' + side + '"' + attr('rin', side) + (side === 'd' ? ' data-autofocus' : '') +
      ' value="' + esc(side === 's' ? r.s : r.d) + '"' + (r.err ? ' aria-invalid="true" aria-describedby="tkrerr"' : '') + '>' +
      '<button type="button" class="ia rgx"' + attr('act', 'm-rclr') + attr('side', side) + ' aria-label="' + esc(clr) + '"' + tipAttr(clr) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  function rangeCal(r) { return rangeMonth(r, r.lm, 'l') + (r.rm ? rangeMonth(r, r.rm, 'r') : ''); }
  // One month panel (side 'l' or 'r'): its arrows page it alone. Day cells are not buttons, so
  // they stay out of the menu's Tab ring; the keyboard path is the inputs.
  function rangeMonth(r, m, side) {
    var first = dayNum(m + '-01'), dow = new Date(first * 86400000).getUTCDay(), y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7));
    var days = new Date(Date.UTC(y, mo, 0)).getUTCDate(), lo = r.s && r.d && r.s <= r.d ? r.s : '', t = today();
    function nav(dir) {
      var l = wsT(dir < 0 ? "js.tk.range.prev" : "js.tk.range.next");
      return '<button type="button" class="ia rgnav"' + attr('act', 'm-rpage') + attr('side', side) + attr('dir', dir) + ' aria-label="' + esc(l) + '"' + tipAttr(l) + '>' + (dir < 0 ? '&lsaquo;' : '&rsaquo;') + '</button>';
    }
    var h = '<div class="rgm"><div class="rghd">' + nav(-1) + '<b>' + esc(m) + '</b>' + nav(1) + '</div><div class="rgg"' + attr('rgg', side) + '>' +
      calDow().map(function (d) { return '<div class="rgw">' + d + '</div>'; }).join('');
    for (var i = 0; i < dow; i++) h += '<div class="rgd off"></div>';
    for (var d = 1; d <= days; d++) {
      var ds = m + '-' + (d < 10 ? '0' : '') + d, k = '';
      if (ds === r.s) k += '<span class="rgk ks" data-rh="s"></span>';
      if (ds === r.d) k += '<span class="rgk kd" data-rh="d"></span>';
      if (ds === r.d && !r.s) k += '<span class="rgk ks ghost" data-rh="s"></span>';
      if (ds === r.s && !r.d) k += '<span class="rgk kd ghost" data-rh="d"></span>';
      h += '<div class="rgd' + (lo && ds >= lo && ds <= r.d ? ' in' : '') + (ds === r.s ? ' s' : '') + (ds === r.d ? ' d' : '') + (ds === t ? ' now' : '') + '"' + attr('day', ds) + '>' + k + '<span class="rgn">' + d + '</span></div>';
    }
    return h + '</div></div>';
  }
  // The open picker after a change: the grid draws again (a focused arrow keeps its focus), the
  // inputs other than src (the one being typed in, if any) take the ends, and the error, the panel
  // count, and the menu's width follow. A drag or a clear leaves no src: a focused input still
  // follows it.
  function rangeDraw(src) {
    var r = MN && MN.rg, c = $('tkrcal');
    if (!r || !c) return;
    var f = document.activeElement, fa = f && c.contains(f) && f.getAttribute('data-act') === 'm-rpage' ? [f.getAttribute('data-side'), f.getAttribute('data-dir')] : null;
    c.innerHTML = rangeCal(r);
    c.parentNode.classList.toggle('two', !!r.rm);
    c.parentNode.classList.toggle('tk-rbad', !!r.err);
    $('tkmenu').classList.toggle('rwide', !!r.rm);
    ['s', 'd'].forEach(function (side) {
      var i = $('tkr' + side);
      if (!i) return;
      if (i !== src && i.value !== r[side]) i.value = r[side];
      if (r.err) { i.setAttribute('aria-invalid', 'true'); i.setAttribute('aria-describedby', 'tkrerr'); } else { i.removeAttribute('aria-invalid'); i.removeAttribute('aria-describedby'); }
    });
    $('tkrerr').hidden = !r.err;
    placeMenu();
    if (fa) { var b = c.querySelector('[data-act="m-rpage"][data-side="' + fa[0] + '"][data-dir="' + fa[1] + '"]'); if (b) b.focus({ preventScroll: true }); }
  }
  // a typed date or an emptied input: that end in the pick
  function rangeInput(t) {
    var r = MN && MN.step === 'due' && MN.rg, side = t.getAttribute('data-rin');
    if (!r) return;
    r[side] = t.value || ''; r.last = side; r.err = rangeBad(r);
    rangeFit(r); rangeDraw(t);
  }
  // the month arrows (view only) and the clear buttons (that end in the pick; the picker stays open)
  function rangeAct(k, a) {
    var r = rangeOf(MN), side = a.getAttribute('data-side');
    if (k === 'm-rclr') { r[side] = ''; r.last = side; r.err = rangeBad(r); rangeFit(r); }
    else {
      var dir = Number(a.getAttribute('data-dir'));
      if (side === 'r' && r.rm) r.rm = monAdd(r.rm, dir); else r.lm = monAdd(r.lm, dir);
    }
    rangeDraw();
  }
  // an end moved by a drag, clamped at the other end (the ends never swap or push each other)
  function rangeClamp(r, side, day) {
    if (side === 's' && r.d && day > r.d) return r.d;
    if (side === 'd' && r.s && day < r.s) return r.s;
    return day;
  }
  function rangeSet(r, side, day) {
    day = rangeClamp(r, side, day);
    if (r[side] === day) return;
    r[side] = day; r.last = side; r.err = rangeBad(r);
    rangeDraw();
  }
  // A drag past the dragged end's panel edge: with one panel, the neighbouring month of that end
  // opens beside it (start: before, due: after) while the other end is set, else the panel pages;
  // with two panels, that end's own panel pages, never past the other panel.
  function rangeEdge(r, side, dir) {
    if (!r.rm) {
      if (!(side === 's' ? r.d : r.s)) r.lm = monAdd(r.lm, dir);
      else if (side === 's' && dir < 0) { r.rm = r.lm; r.lm = monAdd(r.lm, -1); }
      else if (side === 'd' && dir > 0) r.rm = monAdd(r.lm, 1);
      else return;
    } else if (side === 's') { var l = monAdd(r.lm, dir); if (l > r.rm) return; r.lm = l; }
    else { var m = monAdd(r.rm, dir); if (m < r.lm) return; r.rm = m; }
    rangeDraw();
  }
  // Pointer drags (mouse and touch) on a handle, captured by the menu so a redrawn grid keeps the
  // drag. A day under the pointer moves the end there; outside the end's panel the edge pages
  // (rangeEdge, then every RG_EDGE_MS while it stays out). A plain click on a day moves an end (rangeClick).
  var RG_EDGE_MS = 450;
  // the end a click on day moves, per the rules above the picker's functions; pane is the clicked panel, 'l' or 'r'
  function rangeClickEnd(r, day, pane) {
    if (!r.s && !r.d) return 'd';
    if (r.rm && r.rm !== r.lm) return pane === 'r' ? 'd' : 's';
    if (!r.d) return day >= r.s ? 'd' : 's';
    if (!r.s) return day <= r.d ? 's' : 'd';
    return Math.abs(dayNum(day) - dayNum(r.s)) < Math.abs(dayNum(day) - dayNum(r.d)) ? 's' : 'd';
  }
  function rangeClick(r, day, pane) {
    var side = rangeClickEnd(r, day, pane);
    day = rangeClamp(r, side, day);
    if (r[side] === day) return;
    r[side] = day; r.last = side; r.err = rangeBad(r);
    rangeFit(r); rangeDraw();
  }
  function rangeDown(e) {
    // On a picker with the capsule, a press on anything but a control (a day, a header, the panel)
    // keeps focus where it was: focus left on the body would take Esc and Tab away from menuKey.
    if (capOn() && !e.target.closest('input, button, select, textarea, label, [tabindex]')) e.preventDefault();
    var r = MN && MN.step === 'due' && MN.rg, k = r && e.target.closest('[data-rh]');
    if (!r || e.button > 0) return;
    if (!k) {
      var cell = e.target.closest('[data-day]');
      var pn = cell && cell.closest('[data-rgg]');
      if (cell) rangeClick(r, cell.getAttribute('data-day'), pn && pn.getAttribute('data-rgg'));
      return;
    }
    e.preventDefault();
    r.drag = { side: k.getAttribute('data-rh'), id: e.pointerId, x: e.clientX, y: e.clientY, at: 0, tm: setInterval(rangeTick, RG_EDGE_MS) };
    $('tkmenu').setPointerCapture(e.pointerId);
  }
  function rangeMove(e) {
    var r = MN && MN.rg, g = r && r.drag;
    if (!g || e.pointerId !== g.id) return;
    g.x = e.clientX; g.y = e.clientY;
    rangeTick();
  }
  function rangeTick() {
    var r = MN && MN.rg, g = r && r.drag;
    if (!g) return;
    var at = document.elementFromPoint(g.x, g.y), cell = at && at.closest && at.closest('#tkmenu [data-day]');
    if (cell) { rangeSet(r, g.side, cell.getAttribute('data-day')); return; }
    var grid = $('tkmenu').querySelector('[data-rgg="' + (r.rm && g.side === 'd' ? 'r' : 'l') + '"]'), b = grid && grid.getBoundingClientRect();
    if (!b) return;
    var dir = g.x < b.left || g.y < b.top ? -1 : g.x > b.right || g.y > b.bottom ? 1 : 0, now = Date.now();
    if (!dir || now - g.at < RG_EDGE_MS) return;
    g.at = now;
    rangeEdge(r, g.side, dir);
  }
  function rangeUp(e) {
    var r = MN && MN.rg, g = r && r.drag;
    if (!g || (e.pointerId != null && e.pointerId !== g.id)) return;
    clearInterval(g.tm); r.drag = null;
    if ($('tkmenu').hasPointerCapture && $('tkmenu').hasPointerCapture(g.id)) $('tkmenu').releasePointerCapture(g.id);
    rangeFit(r); r.err = rangeBad(r); rangeDraw();
  }
  // What closing menu m owes from its range picker: { bad } for an inverted pair; for an item,
  // { ev } with one item.edit of the ends the user moved off what the picker opened on that still
  // differ from the item (so an end another viewer wrote meanwhile and the user left alone is kept,
  // and a changed start never carries an unchanged due); for a row's draft, { draft } when an end
  // moved; null for nothing owed, or when the item or the viewer's edit right is gone.
  function rangePending(m) {
    var r = m && m.step === 'due' && m.rg;
    // a back-row write in flight (rsent) already carries this pick: its own close must not resend it
    if (!r || m.rsent) return null;
    if (m.kind === 'new') {
      var n = nd();
      if (!n) return null;
      if (rangeBad(r)) return { bad: true };
      return r.s === r.s0 && r.d === r.d0 ? null : { draft: n };
    }
    var it = m.kind === 'item' && VS && VS.items[m.item];
    if (!it || !canEdit(it)) return null;
    if (rangeBad(r)) return { bad: true };
    var ep = { item: it.id };
    if (r.s !== r.s0 && (r.s || null) !== (it.start || null)) ep.start = r.s || null;
    if (r.d !== r.d0 && (r.d || null) !== (it.due || null)) ep.due = r.d || null;
    return Object.keys(ep).length > 1 ? { ev: [['item.edit', ep]] } : null;
  }
  // An inverted pick refuses a close that can stay open: the error shows and focus goes to the
  // input of the end last changed. Returns whether it refused.
  function rangeRefused() {
    var p = MN && rangePending(MN);
    if (!p || !p.bad) return false;
    MN.rg.err = true;
    rangeDraw();
    var f = $('tkr' + MN.rg.last);
    if (f) f.focus({ preventScroll: true });
    return true;
  }
  // A close that applies and could keep the menu open (an outside click, focus loss, Enter in a
  // range input, a menu opened over this one): refused while the range picker holds an inverted
  // pair. Returns whether the menu closed.
  function menuApplyClose(refocus) {
    if (rangeRefused()) return false;
    closeMenu(refocus);
    return true;
  }
  // Esc and the discard capsule: the range or label pick drops and nothing is written (a row's
  // label toggles, which edit the draft live, go back to what the picker opened on); a step opened
  // from the main menu goes back to it, a picker without one closes and focus goes to its opener.
  function menuDiscard() {
    if (!MN) return;
    MN.rg = null; MN.lset = null;
    if (MN.lsnap) { var n = nd(); if (n) n.labels = MN.lsnap; MN.lsnap = null; render(); }
    menuBack();
  }
  // the menu and its discard capsule, one place for every "inside the menu" test
  function menuHas(t) { return !!t && ($('tkmenu').contains(t) || $('tkcap').contains(t)); }
  // the pickers that show the discard capsule
  function capOn() { return !!MN && (MN.kind === 'item' || MN.kind === 'new') && (MN.step === 'due' || MN.step === 'labels'); }
  // The capsule floats just below the menu, above it when below has no room, else over the menu's
  // bottom edge, kept inside the window.
  function placeCap() {
    var c = $('tkcap');
    c.hidden = !capOn();
    if (c.hidden) return;
    var mr = $('tkmenu').getBoundingClientRect(), cw = c.offsetWidth, ch = c.offsetHeight, W = window.innerWidth, Hh = window.innerHeight, y = mr.bottom + 6;
    if (y + ch > Hh - 6) y = mr.top - ch - 6 >= 6 ? mr.top - ch - 6 : Math.max(6, Math.min(Hh - ch - 6, mr.bottom - ch / 2));
    c.style.left = Math.round(Math.max(6, Math.min(mr.left + (mr.width - cw) / 2, W - cw - 6))) + 'px';
    c.style.top = Math.round(y) + 'px';
  }
  // A click outside the menu and its capsule closes the menu, applying its pick; a click on its
  // opener or the item window's backdrop is spent on that. A refused close keeps the menu and
  // spends the click, so another item's chip cannot open over the kept pick. Returns whether the
  // click is spent.
  function menuOutside(e, t) {
    if (!MN || (t.closest && t.closest('#tkmenu, #tkcap'))) return false;
    var spent = !!(MN.backEl && MN.backEl.contains(t)) || t === $('tkwin');
    if (!menuApplyClose(spent)) spent = true;
    if (spent) e.preventDefault();
    return spent;
  }
  // Focus leaving the labels step or the range picker for somewhere outside the menu, its capsule,
  // and its opener (whose click toggles it shut) applies the pick. Focus a redraw dropped (a month
  // arrow, a panel opening) has no new target and is not a focus loss.
  function menuFocusOut(e) {
    var to = e.relatedTarget;
    if (!MN || !to || menuHas(to) || (MN.backEl && MN.backEl.contains(to))) return;
    if ((MN.kind === 'item' && MN.step === 'labels') || ((MN.kind === 'item' || MN.kind === 'new') && MN.step === 'due')) menuApplyClose(false);
  }

