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
//   search(id, q, done)  optional, for the Items panel's search box: the ids of the
//                        store's items that pass q, parsed by the tracker's search
//                        grammar (the daemon parses it, so the box and the agents'
//                        tracker.query q agree). done({ok: true, ids} | {ok: false,
//                        error}); an error is a malformed query, shown under the box
//                        with the list left unfiltered. Without it the box matches a
//                        plain substring of the title, body, project, and id (the mockup)
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
//   qsSlot           an element in the header the quick-switcher button and popup
//                    go into
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

  var EDIT_FIELDS = ['title', 'body', 'project', 'labels', 'due', 'priority'];
  function listOf(x) { return Array.isArray(x) ? x : []; }
  // Mentions and attachments a write carries, for one-line descriptions.
  // A field name the event sentences print: the table's, else the field itself.
  var FIELD_KEY = { title: "js.tk.field.title", body: "js.tk.field.body", project: "js.tk.field.project", labels: "js.tk.field.labels", due: "js.tk.field.due", priority: "js.tk.field.priority",
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
  function whoHtml(it) {
    var as = holders(it);
    if (!as.length) return '<span class="who"></span>';
    var shown = as.slice(0, 3), more = as.length - shown.length;
    // the avatars fill the group, so the group tooltip names each relation instead of one per avatar
    var tip = wsT("js.tk.row.assignees", as.map(function (r) { var n = wsT("js.tk.av.who", nmo(r.to), avRel(r.to).t); return r.state === 'pending' ? wsT("js.tk.row.assignee.pending", n) : n; }).join(', '));
    return '<span class="who"' + tipAttr(tip) + '>' + shown.map(function (r) {
      return avHtml(r.to, r.state === 'pending', false);
    }).join('') + (more ? '<i class="av more">+' + more + '</i>' : '') + '</span>';
  }
  function priHtml(it) {
    if (!it || it.kind === 'topic') return '<span class="pri"></span>';
    var lv = it.priorityLevel, d = VS.priority.default;
    return '<span class="pri' + (lv > d ? ' hi' : lv < d ? ' lo' : '') + '"' + tipAttr(wsT("js.tk.pri.tip.name", lv, VS.priority.names.length - 1, it.priorityName) +
      (it.priority !== lv ? ' ' + wsT("js.tk.pri.stored", it.priority) : '')) + '>' + esc(it.priorityName) + '</span>';
  }
  // Due: D-N within 7 days, D-day today, D+N past due (danger colour), else a short date.
  var ENDED = { done: 1, dropped: 1, declined: 1, returned: 1, handed: 1, withdrawn: 1 };
  function dueHtml(it, st) {
    if (!it || !it.due || it.kind === 'topic') return '<span class="due2"></span>';
    var n = dayNum(it.due) - dayNum(today()), t, c, tip;
    if (ENDED[st]) { t = it.due.slice(5); c = 'ended'; tip = wsT("js.tk.due.tip.ended", it.due, SI.info(st).label); }
    else if (n < 0) { t = wsT("js.tk.due.dplus", -n); c = 'over'; tip = wsTn("js.tk.due.tip.overby", -n, it.due); }
    else if (n === 0) { t = wsT("js.tk.due.dday"); c = 'now'; tip = wsT("js.tk.due.tip.today", it.due); }
    else if (n <= 7) { t = wsT("js.tk.due.dminus", n); c = 'soon'; tip = wsTn("js.tk.due.tip.in", n, it.due); }
    else { t = it.due.slice(5); c = ''; tip = wsT("js.tk.due.on", it.due); }
    return '<span class="due2' + (c ? ' ' + c : '') + '"' + tipAttr(tip) + '>' + esc(t) + '</span>';
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
    if (recs.some(function (r) { return r.to === v && r.state === 'pending'; }))
      out.push(['accept', 'accept', wsT("js.tk.act.accept"), { item: id }, 1], ['note', 'decline', wsT("js.tk.act.decline"), { item: id, op: 'item.decline' }]);
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
  // A row drags (a list drop moves it with its subtree, see mvDrop) when the viewer may move it, or
  // when it is selected: then the selection moves.
  function irow(o) {
    var it = VS.items[o.item], r = o.rec, st = stKey(it, r), ms = mselHas(it.id);
    if (MSEEN) MSEEN[it.id] = true;
    return '<div class="irow' + (ui.sel === o.rk ? ' sel' : '') + (ms ? ' msel' : '') + (o.closed ? ' closed' : '') + (o.ctx ? ' ctx' : '') + '" tabindex="0"' + (ms || canMove(it) ? ' draggable="true"' : '') +
      attr('item', it.id) + attr('rk', o.rk) + (r ? attr('rec', r.id) : '') +
      (o.depth ? ' style="--ind:' + o.depth + '"' : '') + '><div class="ir-main">' + mcbHtml(it, ms) + (o.tw != null ? o.tw : o.depth ? '<span class="ind">&#8627;</span>' : '') + stBtn(st, it, r) +
      '<a class="title" href="#" tabindex="-1"' + attr('item', it.id) + '>' + esc(it.title) + '</a>' + (o.tsub || '') + openChip(it) + labelChips(it, 'lsearch') + warnHtml(it) + dotHtml(it) + '<span class="grow"></span>' +
      '<span class="ir-acts">' + moreBtn(it.id, '', true) + '</span>' + whoHtml(it) + priHtml(it) + dueHtml(it, st) + '</div>' +
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
      doc: null, docErr: null, docBusy: false, // {path, content, commit, html} of the page shown
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
  function wkHas(st, path) { return !!(st.pages && st.pages.paths.indexOf(path) >= 0); }
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
      st.doc = { path: res.path, content: res.content, commit: res.commit, html: res.html }; st.docErr = null;
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
      (path ? '<div class="wk-etop">' + path + '</div>' : '') + editorHtml('wk-ebody', { value: ed.text, rows: 14, label: esc(wsT("js.tk.wk.ed.aria")), pv: 'wk-doc' }) + bar + '</div>';
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
    var seed = '# ' + String(path).split('/').pop().replace(/-/g, ' ') + '\n\n';
    if (isNew) st.edit = { path: path, isNew: true, newWiki: !!newWiki, base: st.pages ? st.pages.commit : '', text: seed, orig: '', seed: seed, newPath: path, note: false };
    else st.edit = { path: path, isNew: false, newWiki: false, base: st.doc.commit, text: st.doc.content, orig: st.doc.content, newPath: '', note: false };
    wkShow();
    render();
    var f = $(isNew && (!path || path.slice(-1) === '/') ? 'wk-epath' : 'wk-ebody');
    if (f) { f.focus({ preventScroll: true }); if (f.id === 'wk-ebody') { try { f.setSelectionRange(f.value.length, f.value.length); } catch (x) { /* not a text field */ } } }
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
    var path = ed.isNew ? ($('wk-epath') ? $('wk-epath').value : ed.newPath).trim().replace(/^\/+|\/+$/g, '').replace(/\/{2,}/g, '/') : ed.path;
    if (!path) return toast(esc(wsT("js.tk.wk.toast.path")));
    if (ed.isNew && wkHas(st, path)) return toast(esc(wsT("js.tk.wk.toast.exists", path)));
    st.saving = true; render();
    AD.wiki.save(store, name, { base: ed.newWiki ? '' : ed.base, newWiki: !!ed.newWiki, op: 'write', path: path, content: ed.text, attachments: savedAtts('wk-ebody', ed.text) }, function (res) {
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
  // Due-date timeline: the scope's dated rows as the same context tree the list draws (the
  // fold state is shared with it). A row's bar runs from the item's start (first doing, else
  // creation) to its due day. A node with children also draws a thin summary bar spanning the
  // range of every dated item below it, whether they show or are folded away; a parent without
  // a due date of its own (a context row, say) is placed by that summary bar alone.
  function ganttBody(key, rows) {
    var bar = key === 'all' ? itemFilterBar() : '';
    if (!rows.length) return '<div class="toolbar">' + bar + '</div><div class="empty">' + wsT("js.tk.gantt.empty") + '</div>';
    var ents = ctxTree(rows, key === 'all' ? { keep: keepKind } : null), below = {};
    // [first day, day after the last] of an item's own bar, or null without a due date
    function own(id) {
      var it = VS.items[id];
      if (!it.due || it.kind === 'topic') return null;
      var e = dayNum(it.due) + 1;
      return [Math.min(dayNum(it.started || it.ts), e - 1), e];
    }
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
          esc(wsT(it.started ? "js.tk.gantt.bar.started" : "js.tk.gantt.bar.created", String(it.started || it.ts).slice(0, 10), it.due)) + '">' + esc(it.due.slice(5)) + '</span>' : '') +
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

  function calendarBody(key, rows) {
    var m = ui.calMonth[key] || today().slice(0, 7), first = dayNum(m + '-01'), dow = (new Date(first * 86400000).getUTCDay() + 6) % 7;
    var y = Number(m.slice(0, 4)), mo = Number(m.slice(5, 7)), days = new Date(Date.UTC(y, mo, 0)).getUTCDate();
    var byDay = {};
    rows.forEach(function (x) { var due = VS.items[x.item].due; (byDay[due] = byDay[due] || []).push(x); });
    var prev = new Date(Date.UTC(y, mo - 2, 1)).toISOString().slice(0, 7), next = new Date(Date.UTC(y, mo, 1)).toISOString().slice(0, 7);
    var n = Object.keys(byDay).filter(function (k) { return k.slice(0, 7) === m; }).reduce(function (s, k) { return s + byDay[k].length; }, 0);
    var h = '<div class="toolbar">' + btn('cal', '&lsaquo;', { month: prev, card: key }) + '<b>' + esc(m) + '</b>' + btn('cal', '&rsaquo;', { month: next, card: key }) +
      btn('cal', wsT("js.tk.cal.today"), { month: today().slice(0, 7), card: key }) + (key === 'all' ? itemFilterBar() : '') + '<span class="muted">' + wsT("js.tk.cal.count", n, esc(m)) + '</span></div>' +
      '<div class="cal">' + [wsT("js.tk.cal.mon"), wsT("js.tk.cal.tue"), wsT("js.tk.cal.wed"), wsT("js.tk.cal.thu"), wsT("js.tk.cal.fri"), wsT("js.tk.cal.sat"), wsT("js.tk.cal.sun")].map(function (d) { return '<div class="cdow">' + d + '</div>'; }).join('');
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
  function propertyCard(it) {
    var hs = holders(it), topic = it.kind === 'topic';
    return '<section class="pcard" aria-label="' + wsT("js.tk.prop.aria") + '"><h3>' + wsT("js.tk.prop.title") + '</h3><dl>' +
      '<dt>' + wsT("js.tk.prop.status") + '</dt><dd>' + (topic ? '<span class="muted">' + wsT("js.tk.prop.topic") + '</span>' : esc(SI.info(it.status).label) + ' <span class="muted">' + esc(SI.info(it.status).desc) + '</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.kind") + '</dt><dd>' + esc(kindName(it.kind)) + (it.pending.length ? ' <span class="muted">' + wsT("js.tk.prop.pending") + '</span>' : '') + (it.hidden ? ' <span class="muted">' + wsT("js.tk.prop.hidden") + '</span>' : '') + '</dd>' +
      (topic ? '<dt>' + wsT("js.tk.prop.holder") + '</dt><dd' + attr('focus', 'holders') + '>' + (holderNames(it) || '<span class="muted">' + wsT("js.tk.row.unassigned") + '</span>') + '</dd>' :
        '<dt>' + wsT("js.tk.prop.holders") + '</dt><dd' + attr('focus', 'holders') + '>' + holdersSummary(it) + '</dd>') +
      (it.chain.length > 1 ? '<dt>' + wsT("js.tk.prop.path") + '</dt><dd>' + esc(it.chain.join(' > ')) + '</dd>' : '') +
      (topic ? '' : '<dt>' + wsT("js.tk.prop.due") + '</dt><dd>' + (it.due ? esc(it.due) : '<span class="muted">-</span>') + '</dd>') +
      '<dt>' + wsT("js.tk.prop.creator") + '</dt><dd>' + whoName(it.author) + (VS.members[it.author] && VS.members[it.author].admin ? ' <span class="chip admin">' + wsT("js.tk.team.admin") + '</span>' : '') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.project") + '</dt><dd>' + (it.project ? esc(it.project) : '<span class="muted">-</span>') + '</dd>' +
      '<dt>' + wsT("js.tk.prop.labels") + '</dt><dd>' + (labelChips(it) || '<span class="muted">-</span>') + '</dd>' +
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
  // the text, inside the wrapper) }
  function editorHtml(id, o) {
    var on = !!ui.edpv[id];
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
    else if (/^iw-e(title|body|proj)$/.test(t.id) && ui.iedit) ui.iedit[{ 'iw-etitle': 'title', 'iw-ebody': 'body', 'iw-eproj': 'project' }[t.id]] = t.value;
    else if (t.id === 'wk-ebody' && wk().edit) wk().edit.text = t.value;
    else if (t.id === 'wk-epath' && wk().edit) wk().edit.newPath = t.value;
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
        for (var pi = 0; pi < Math.min(9, pn.length + 2); pi++) {
          rows += '<div class="fld inline plevel"><input type="radio" name="pdef" value="' + pi + '"' + (pi === VS.priority.default ? ' checked' : '') + ' aria-label="' + esc(wsT("js.tk.form.pdefault", pi)) + '"' + tipAttr(wsT("js.tk.form.pdefault", pi)) + '>' +
            '<input type="text" name="pname' + pi + '" value="' + esc(pn[pi] || '') + '" placeholder="' + esc(wsT("js.tk.form.plevel", pi)) + '" aria-label="' + esc(wsT("js.tk.form.plevel", pi)) + '"></div>';
        }
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
      ui.store = order[0] || null;
      ui.item = null; ui.win = false; ui.last = null; ui.sel = null; ui.sub = null; ui.iedit = null; ui.cedit = null; ui.replyTo = null; ui.itemChanged = true;
    } else if (!ui.store) ui.store = order[0] || null;
    if (gone(ui.open)) ui.open = null;
    if (gone(ui.page)) {
      ui.page = null;
      if (/^#\/./.test(location.hash)) history.replaceState(null, '', location.pathname + location.search);
    }
    if (MN && (MN.kind === 'store' ? gone(MN.store) : lost)) closeMenu(false);
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
  function commit(events, then, prep) {
    AD.dispatch(ui.store, events.map(function (e) { return { type: e[0], payload: e[1], origin: e[2] || 'human' }; }), function (res) {
      if (!res || !res.ok) { toast(esc((res && res.error) || wsT("js.tk.toast.refused"))); return; }
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
    var mb = MN && MN.kind === 'item' ? MN.back : null; // an item menu closes; focus goes back to its row once redrawn
    if (MN && MN.kind === 'item') closeMenu(false);
    render();
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
  function closeMenu(refocus) {
    if (!MN) return;
    var m = MN, el = m.backEl && m.backEl.isConnected ? m.backEl : m.back && document.querySelector(m.back);
    MN = null;
    $('tkmenu').hidden = true; $('tkmenu').innerHTML = '';
    document.body.classList.remove('tk-menuon');
    if (m.backEl && m.backEl.hasAttribute('aria-haspopup')) m.backEl.setAttribute('aria-expanded', 'false');
    if (refocus && el) {
      if (!el.matches('a[href], button, input, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]');
      if (el) el.focus({ preventScroll: true });
    }
  }
  function menuBack() {
    if (!MN.from) { closeMenu(true); return; }
    MN.step = MN.from; MN.from = null; MN.op = null; MN.who = null; MN.req = null; MN.q = ''; MN.as = null; MN.mv = null;
    drawMenu();
  }
  function drawMenu() {
    var m = $('tkmenu');
    m.innerHTML = MN.kind === 'sync' ? syncPopHtml() : MN.kind === 'new' ? newMenuHtml() : MN.kind === 'store' ? storeMenuHtml() : menuHtml();
    m.className = 'tk tk-menu' + (MN.kind === 'sync' ? ' spopw' : MN.step === 'assign' ? ' wide' : '');
    m.setAttribute('role', MN.kind === 'sync' || MN.step === 'assign' ? 'dialog' : 'menu');
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
    if (step === 'due') {
      return h + back + '<div class="mrow"><input type="date" id="tkmdue" data-mi data-autofocus value="' + esc(it.due || '') + '" aria-label="' + wsT("js.tk.menu.due") + '">' +
        btn('m-due-set', wsT("js.tk.menu.due.set"), {}, 'primary') + '</div>' + (it.due ? mi('m-due-clear', wsT("js.tk.menu.due.clear"), {}, { icon: 'x' }) : '');
    }
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
      more.push(mi('due', wsT("js.tk.menu.due"), { item: it.id }, { icon: 'calendar', sub: 1, val: it.due ? esc(it.due) : '' }));
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
  // The picked targets as the item.assign events they write: one with compl when the user picked a
  // completion, else one per default class, the first target's class first, each with its
  // completion stored explicitly. Within an event, people come first, then rules ({root, levels?};
  // the whole subtree without levels).
  function asgEvents(item, sel, compl) {
    var groups = [], by = {};
    sel.forEach(function (o) {
      var c = compl || complClass(o);
      if (!by[c]) groups.push(by[c] = { c: c, sel: [] });
      by[c].sel.push(o);
    });
    return groups.map(function (g) {
      var people = g.sel.filter(function (o) { return o.t === 'p'; }).map(function (o) { return o.id; });
      var rules = g.sel.filter(function (o) { return o.t === 'r'; }).map(function (o) { return o.levels == null ? { root: o.root } : { root: o.root, levels: o.levels }; });
      return ['item.assign', { item: item, targets: people.concat(rules), completion: g.c }];
    });
  }
  // The completion mode the radio shows: the user's pick once they touched it, else the picks'
  // default class (complClass); 'mixed' when they fall in both (written as one assign per class),
  // null while nothing is picked.
  function asgCompl(S) {
    if (S.compl) return S.compl;
    var cs = {};
    S.sel.forEach(function (o) { cs[complClass(o)] = true; });
    var k = Object.keys(cs);
    return k.length > 1 ? 'mixed' : k[0] || null;
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
        return '<button type="button" role="radio" aria-checked="' + (asgCompl(S) === c) + '"' + attr('act', 'm-acm') + attr('c', c) + '>' + esc(complText(c)) + '</button>';
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
    Array.prototype.forEach.call(document.querySelectorAll('#tkmenu .ap-cm [role="radio"]'), function (b) { b.setAttribute('aria-checked', String(b.getAttribute('data-c') === asgCompl(S))); });
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
    if (it.draft) { var n = ui.nrow[ui.store]; n.who = S.sel.map(function (o) { return o.id; }); n.compl = S.compl; return newPicked(); }
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
      var nx = t.parentNode.querySelector('[data-c]:not([data-c="' + t.getAttribute('data-c') + '"])');
      S.compl = nx.getAttribute('data-c'); asgDraw(); nx.focus();
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
      case 'm-due-set': {
        var dv = $('tkmdue').value || null;
        if (dv === it.due) return menuBack();
        return commit([['item.edit', { item: id, due: dv }]]);
      }
      case 'm-due-clear': return commit([['item.edit', { item: id, due: null }]]);
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
    if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); if (MN.from) menuBack(); else closeMenu(true); return; }
    if (k === 'Tab') {
      var fs = Array.prototype.filter.call(m.querySelectorAll('button:not([disabled]), input:not([disabled]), select'), function (x) { return x.offsetParent !== null; });
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
      if (t.id === 'tkmdue') { e.preventDefault(); menuAct('m-due-set', t); return; }
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

  // ------------------------------------------------------------ inline writes

  // A new item: an input row in the Items list (ui.nrow[store] = { val, kind, st, who, compl, due,
  // parent?, after?, at?, depth? } while it shows). The "+" that ends the panel opens it there (no
  // `at`: it appends a root item); the hover insert opens it in a gap between rows (`at` names the
  // row it sits against, and `parent` / `after` place the item where the row shows). Title + Enter
  // creates the item; the row stays for the next one, which then goes after the one just made;
  // Esc closes it. The icon toggle at its left edge switches the kind (task <-> topic).
  // The row's chips set status, assignee, priority, due, and labels before Enter (nrowChips,
  // newMenu); a topic has no status, priority, or due. Left at their defaults (idea, the viewer,
  // the store's default level, none, none) the row sends the create plus the creator's status
  // idea (asIdea), or for a topic the create plus the creator's assign (heldTopic). The chips keep
  // their values across creates while the row stays open; closing it restores the defaults.
  // n.pri is a level index (null: the store's default, sent as no priority); n.labels label ids.
  function nrowFill(n) {
    if (!n.who) { n.st = 'idea'; n.who = [VS.viewer.member]; n.compl = null; n.due = ''; n.pri = null; n.labels = []; }
    return n;
  }
  // a row opened from another (the hover insert moving it) keeps its text, kind, and chips
  function nrowNew(o, place) {
    var n = { val: o ? o.val : '', kind: o ? o.kind : 'task' };
    if (o && o.who) { n.st = o.st; n.who = o.who; n.compl = o.compl; n.due = o.due; n.pri = o.pri; n.labels = o.labels; }
    Object.keys(place || {}).forEach(function (k) { n[k] = place[k]; });
    return nrowFill(n);
  }
  function nrowMine(n) { return n.who.length === 1 && n.who[0] === VS.viewer.member; }
  // The status chip's choices: the creator's own statuses while the viewer alone is the assignee
  // (sent as item.status on the store's fallback holding); the wrap-up set when the viewer is an
  // org superior of every picked assignee, none of them the viewer (one item.status per assignee
  // after the assign, which is live at once for a subordinate), todo standing for no status write
  // (the live assignment's own start); none otherwise, where anyone else starts in the store's own
  // state (todo within reach, pending outside it). A topic has no status or due: only its one
  // holder shows.
  var NROW_ST = { own: ['idea', 'todo', 'doing', 'blocked'], wrap: ['todo', 'blocked', 'done', 'dropped'] };
  function nrowStMode(n) {
    var v = VS.viewer.member;
    if (n.kind === 'topic') return '';
    if (nrowMine(n)) return 'own';
    return n.who.length && n.who.every(function (id) { return id !== v && !!VS.viewer.reach[id]; }) ? 'wrap' : '';
  }
  // the chip's status as the mode reads it: a pick the mode does not offer falls back to its first
  function nrowSt(n) { var m = nrowStMode(n); return !m ? '' : NROW_ST[m].indexOf(n.st) >= 0 ? n.st : NROW_ST[m][0]; }
  // the chip's level: the pick while the store's levels still hold it, else the store's default
  function nrowPri(n) { return n.pri != null && n.pri >= 0 && n.pri < VS.priority.names.length ? n.pri : VS.priority.default; }
  // the picked labels the store's list still holds, in the list's order
  function nrowLabels(n) { return (VS.labels || []).filter(function (l) { return (n.labels || []).indexOf(l.id) >= 0; }); }
  function nrowChips(n) {
    var topic = n.kind === 'topic', h = '', st = nrowSt(n);
    function chip(id, step, label, inner, dflt) {
      return '<button type="button" class="nch' + (dflt ? ' dflt' : '') + '" id="' + id + '"' + attr('act', 'nchip') + attr('step', step) +
        ' aria-haspopup="' + (step === 'assign' ? 'dialog' : 'menu') + '" aria-expanded="false" aria-label="' + esc(label) + '"' + tipAttr(label) + '>' + inner + '</button>';
    }
    if (st) h += chip('tknst', 'status', wsT("js.tk.new.st", stLabel(st)), SI.svg(st) + '<span>' + esc(stLabel(st)) + '</span>', st === NROW_ST[nrowStMode(n)][0]);
    var one = n.who.length === 1, wl = one ? nm(n.who[0]) : wsTn("js.tk.ap.n", n.who.length);
    h += chip('tknwho', 'assign', wsT("js.tk.new.who", wl), one ? avHtml(n.who[0]) + '<span>' + esc(wl) + '</span>' :
      '<svg class="i"><use href="#tki-assign"/></svg><span>' + esc(wl) + '</span>', nrowMine(n));
    if (!topic) {
      var pv = nrowPri(n), pn = VS.priority.names[pv];
      h += chip('tknpri', 'pri', wsT("js.tk.new.pri", pn), '<svg class="i"><use href="#tki-flag"/></svg><span>' + esc(pn) + '</span>', pv === VS.priority.default);
      h += chip('tkndue', 'due', wsT("js.tk.new.due", n.due || wsT("js.tk.new.due.none")), '<svg class="i"><use href="#tki-calendar"/></svg>' + (n.due ? '<span>' + esc(n.due) + '</span>' : ''), !n.due);
    }
    // labels (any kind: a label is orthogonal to the kind); with an empty list only an admin, who can add one, sees the chip
    var ls = nrowLabels(n);
    if ((VS.labels || []).length || VS.viewer.admin) {
      var ln = ls.map(function (l) { return l.name; }).join(', ') || wsT("js.tk.new.labels.none");
      h += chip('tknlab', 'labels', wsT("js.tk.new.labels", ln), '<svg class="i"><use href="#tki-tag"/></svg>' + ls.slice(0, 3).map(function (l) { return '<span class="ldot" style="background:' + esc(l.color) + '"></span>'; }).join('') +
        (ls.length ? '<span>' + esc(ls.length === 1 ? ls[0].name : wsTn("js.tk.new.labels.n", ls.length)) + '</span>' : ''), !ls.length);
    }
    return h;
  }
  function newRowHtml() {
    var n = ui.nrow[ui.store];
    if (!n) return '';
    nrowFill(n);
    var kl = wsT(n.kind === 'topic' ? "js.tk.new.kind.topic" : "js.tk.new.kind.task");
    return '<div class="nrow"' + (n.depth ? ' style="--ind:' + n.depth + '"' : '') + '><button type="button" class="nkind" id="tknkind"' + attr('act', 'nkind') + ' aria-label="' + esc(kl) + '"' + tipAttr(kl) + '>' + SI.svg(n.kind === 'topic' ? 'topic' : 'todo') + '</button>' +
      '<input type="text" id="tknew" data-keep autocomplete="off" value="' + esc(n.val) + '" placeholder="' + wsT("js.tk.new.ph") + '" aria-label="' + wsT("js.tk.new.aria") + '">' +
      nrowChips(n) +
      '<button type="button" class="ia"' + attr('act', 'new-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.new.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  // The kind toggle (task <-> topic): a topic takes one holder, so several picked people fall back
  // to the viewer; its status, priority, and due chips hide (createNew leaves them out).
  function nrowKind(kind) {
    var n = ui.nrow[ui.store];
    if (!n) return;
    nrowFill(n).kind = kind;
    if (kind === 'topic' && n.who.length !== 1) n.who = [VS.viewer.member];
    render();
    if ($('tknkind')) $('tknkind').focus({ preventScroll: true });
  }
  // A chip's picker: the item menu's own step (MN.kind 'new') over the row's draft instead of an item.
  function newMenu(step, anchor) {
    var n = ui.nrow[ui.store];
    if (!n) return;
    nrowFill(n);
    var as = step === 'assign' ? { q: '', sel: n.who.map(function (id) { return { t: 'p', id: id }; }), compl: n.compl, idx: 0, opts: [] } : null;
    openMenu({ kind: 'new', step: step, from: null, q: '', as: as }, anchor);
  }
  // the item the asg* steps read: the row's draft (no records, no id) for a chip's picker
  function draftItem() {
    var n = ui.nrow[ui.store];
    return n ? { id: '', kind: n.kind, title: n.val.trim() || wsT("js.tk.new.item"), assignments: [], draft: true } : null;
  }
  function mnItem() { return MN.kind === 'new' ? draftItem() : VS.items[MN.item]; }
  // a menu whose subject left closes on the next render (a store menu's store, see dropGone)
  function mnStale() { return MN.kind === 'store' ? false : MN.kind === 'new' ? !ui.nrow[ui.store] : !VS.items[MN.item]; }
  function newMenuHtml() {
    var n = ui.nrow[ui.store], d = draftItem();
    if (!n) return '';
    var h = '<div class="mh"><b>' + esc(d.title) + '</b></div>';
    if (MN.step === 'status') {
      var nst = nrowSt(n);
      return h + '<div class="mst" role="group" aria-label="' + wsT("js.tk.prop.status") + '">' + (NROW_ST[nrowStMode(n)] || []).map(function (s) {
        var on = s === nst;
        return '<button type="button" class="mr" role="menuitemradio" aria-checked="' + on + '" data-mi' + attr('act', 'm-status') + attr('status', s) + (on ? ' data-autofocus' : '') + '>' + SI.svg(s) + '<span>' + esc(stLabel(s)) + '</span></button>';
      }).join('') + '</div>';
    }
    if (MN.step === 'due') {
      return h + '<div class="mrow"><input type="date" id="tkmdue" data-mi data-autofocus value="' + esc(n.due) + '" aria-label="' + wsT("js.tk.menu.due") + '">' +
        btn('m-due-set', wsT("js.tk.menu.due.set"), {}, 'primary') + '</div>' + (n.due ? mi('m-due-clear', wsT("js.tk.menu.due.clear"), {}, { icon: 'x' }) : '');
    }
    if (MN.step === 'pri') return h + priGroup(nrowPri(n));
    if (MN.step === 'labels') return h + newLabelsHtml(n);
    return h + asgHtml(d);
  }
  // The label chip's picker: the store's labels as toggles (the menu stays open across picks).
  // An admin also adds, renames, and removes labels here, through the store settings' own label
  // dialog and remove path (labelRemove), which change the list for every item.
  function newLabelsHtml(n) {
    var ls = VS.labels || [], admin = !!VS.viewer.admin;
    var rows = ls.map(function (l) {
      var on = (n.labels || []).indexOf(l.id) >= 0;
      return '<div class="mlrow"><button type="button" class="mi mr" role="menuitemcheckbox" aria-checked="' + on + '" data-mi' + attr('act', 'm-ltog') + attr('label', l.id) + '><span class="cbx"></span>' +
        '<span class="ldot" style="background:' + esc(l.color) + '"></span><span class="ml">' + esc(l.name) + '</span></button>' +
        (admin ? '<button type="button" class="ia" data-mi' + attr('act', 'm-ledit') + attr('label', l.id) + ' aria-label="' + esc(wsT("js.tk.new.labels.edit", l.name)) + '"' + tipAttr(wsT("js.tk.new.labels.edit", l.name)) + '><svg class="i"><use href="#tki-pen"/></svg></button>' +
          '<button type="button" class="ia danger" data-mi' + attr('act', 'm-lrm') + attr('label', l.id) + ' aria-label="' + esc(wsT("js.tk.new.labels.rm", l.name)) + '"' + tipAttr(wsT("js.tk.new.labels.rm", l.name)) + '><svg class="i"><use href="#tki-x"/></svg></button>' : '') + '</div>';
    }).join('');
    return '<div role="group" aria-label="' + esc(wsT("js.tk.set.labels")) + '">' + (rows || '<p class="mempty">' + wsT("js.tk.set.labels.none") + '</p>') + '</div>' +
      (admin ? msep() + mi('m-ladd', esc(wsT("js.tk.new.labels.add")), {}, { icon: 'plus' }) : '');
  }
  function newAct(k, a) {
    var n = ui.nrow[ui.store];
    if (!n) return closeMenu(false);
    if (k === 'm-status') n.st = a.getAttribute('data-status');
    else if (k === 'm-pri') n.pri = Number(a.getAttribute('data-p'));
    else if (k === 'm-due-set') n.due = $('tkmdue').value || '';
    else if (k === 'm-due-clear') n.due = '';
    else if (k === 'm-ltog') {
      var lid = a.getAttribute('data-label'), ls = n.labels = (n.labels || []).slice(), li = ls.indexOf(lid);
      if (li >= 0) ls.splice(li, 1); else ls.push(lid);
      return newLabelsRedraw(lid);
    } else if (k === 'm-ladd' || k === 'm-ledit') {
      closeMenu(false);
      return openDialog('slabel', k === 'm-ledit' ? { label: a.getAttribute('data-label') } : {});
    } else if (k === 'm-lrm') return labelRemove(a.getAttribute('data-label'), function () { newLabelsRedraw(null); });
    else return;
    newPicked();
  }
  // the label picker after a toggle or a removal: the row's chip and the open picker redraw, and
  // focus stays on the toggled label (the first item when gone)
  function newLabelsRedraw(lid) {
    render();
    if (!MN || MN.kind !== 'new' || MN.step !== 'labels') return;
    var bk = MN.backEl && MN.backEl.id && $(MN.backEl.id);
    if (bk) { MN.backEl = bk; bk.setAttribute('aria-expanded', 'true'); }
    $('tkmenu').innerHTML = newMenuHtml();
    placeMenu();
    var f = (lid && $('tkmenu').querySelector('[data-act="m-ltog"][data-label="' + CSS.escape(lid) + '"]')) || $('tkmenu').querySelector('[data-mi]');
    if (f) f.focus({ preventScroll: true });
  }
  // a chip's pick lands: the row draws again and focus goes back to the chip (the title when gone)
  function newPicked() {
    var back = MN && MN.backEl ? MN.backEl.id : '';
    closeMenu(false);
    render();
    var el = (back && $(back)) || $('tknew');
    if (el) el.focus({ preventScroll: true });
  }
  function newBtn() {
    return '<button type="button" class="nadd"' + attr('act', 'create') + ' aria-label="' + wsT("js.tk.drawer.new") + '"' + tipAttr(wsT("js.tk.drawer.new.tip")) + '><svg class="i"><use href="#tki-plus"/></svg></button>';
  }
  function openNewRow() {
    if (!VS) return;
    ui.nrow[ui.store] = nrowNew(ui.nrow[ui.store]);
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    render();
    if ($('tknew')) $('tknew').focus();
  }
  // after a render: a row opened by the hover insert goes back against its row; with that row
  // gone from the list it ends the list, still placed by its parent and after
  function placeNew() {
    var n = ui.nrow[ui.store], list = n && n.at && document.querySelector('#card-items .plist');
    if (!list) return;
    var at = list.querySelector(n.at.item ? '.irow[data-item="' + CSS.escape(n.at.item) + '"]' : '[data-rk="' + CSS.escape(n.at.rk) + '"]');
    if (at) at.insertAdjacentHTML(n.at.before ? 'beforebegin' : 'afterend', newRowHtml());
    else list.querySelector('.nadd').insertAdjacentHTML('beforebegin', newRowHtml());
  }
  function closeNewRow() { delete ui.nrow[ui.store]; render(); var b = document.querySelector('.tk [data-act="create"]'); if (b) b.focus(); }
  function createNew() {
    var n = ui.nrow[ui.store], title = $('tknew').value.trim(), v = VS.viewer.member;
    if (!n || !title) return;
    nrowFill(n);
    var cp = { kind: n.kind, title: title }, b, ls = nrowLabels(n);
    if (n.parent) cp.parent = n.parent;
    if (n.after != null) cp.after = n.after;
    if (ls.length) cp.labels = ls.map(function (l) { return l.id; });
    if (cp.kind === 'topic') b = heldTopic(cp, n.who.length === 1 ? n.who[0] : v);
    else {
      if (n.due) cp.due = n.due;
      if (nrowPri(n) !== VS.priority.default) cp.priority = nrowPri(n);
      // anyone but the viewer alone: the assign (one per completion class unless the picker set
      // one, asgEvents), then for a superior's wrap-up pick one status per assignee
      var st = nrowSt(n);
      if (nrowMine(n)) b = asIdea(cp, st);
      else {
        b = [['item.create', cp]].concat(asgEvents('$1', n.who.map(function (id) { return { t: 'p', id: id }; }), n.compl));
        if (nrowStMode(n) === 'wrap' && st !== 'todo') n.who.forEach(function (id) { b.push(['item.status', { item: '$1', assignee: id, status: st }]); });
      }
    }
    commit(b, null, function (res) {
      n.val = '';
      if (n.at && res[0] && res[0].status === 'applied') { n.after = res[0].id; n.at = { item: res[0].id }; }
    });
  }
  // (560) a quick-added task lands before todo: the create, then the creator's status idea (or the
  // status chip's pick) in the same write. No assign: the store holds a task nobody else holds for
  // its creator.
  function asIdea(cp, s) {
    return [['item.create', cp], ['item.status', { item: '$1', status: s || 'idea' }]];
  }
  // (529) a topic made here is its holder's at once (the creator unless the assignee chip picked
  // another): the create, then the assign that makes them its one holder (a topic has no fallback
  // holder; the store ignores its completion)
  function heldTopic(cp, holder) {
    return [['item.create', cp], ['item.assign', { item: '$1', mode: 'chosen', people: [holder], completion: 'assignee' }]];
  }
  // The "all" kanban's idea column "+": an input row atop that column (ui.knew[store] = { val }).
  // Title + Enter creates a creator-held task at idea (asIdea). The row stays for the next one;
  // Esc closes it.
  function knewRowHtml(n) {
    return '<div class="nrow"><svg class="i"><use href="#tki-plus"/></svg><input type="text" id="tkknew" data-keep autocomplete="off" value="' + esc(n.val) + '" placeholder="' + wsT("js.tk.kanban.new.ph") + '" aria-label="' + wsT("js.tk.kanban.new") + '">' +
      '<button type="button" class="ia"' + attr('act', 'knew-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.new.close")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  function openKnew() {
    if (!VS) return;
    ui.knew[ui.store] = ui.knew[ui.store] || { val: '' };
    render();
    if ($('tkknew')) $('tkknew').focus();
  }
  function closeKnew() { delete ui.knew[ui.store]; render(); var b = document.querySelector('.tk [data-act="knew"]'); if (b) b.focus(); }
  function createKnew() {
    var n = ui.knew[ui.store], title = $('tkknew').value.trim();
    if (!n || !title) return;
    commit(asIdea({ kind: 'task', title: title }), null, function () { n.val = ''; });
  }

  // The hover insert: in a list view, the pointer near the line between two rows (or under the
  // last one) shows a "+" badge at the left edge with a guide line. The places a new item can
  // take there (insOpts) run from b's depth (the row below must stay below) to one level under
  // a (its first child); the pointer's x over the left gutter picks the one whose badge is
  // nearest, left to right = shallow to deep. A marker at every other level slot from the root to
  // one under a (.hdep, insMarks) shows where the badge can move: a dim tick where a place is, a
  // small x where none is (above b's depth, or under a parent that cannot take the item); the slots
  // deeper than the badge are drawn too, and the guide line starts past the deepest. The row that
  // would parent the badge's place carries an outline (.hpar, insPar). A click opens the input row
  // in the gap.
  var IND = 16, INS_EDGE = 5; // a level's indent (tracker.css .ir-main, .hins), the gap's half height
  var HV = null; // the shown insert: { list, a, b, y, opts, pick }
  var HP = null; // the row outlined as the shown insert's parent (.hpar)
  function rowDepth(el) { return el ? Number(el.style.getPropertyValue('--ind')) || 0 : 0; }
  function rowLike(el) { return el && el.matches && el.matches('.irow, .csum, .tmore') ? el : null; }
  // ok(parent id, or null for the top level): may the place's parent take the item; a create's
  // rule (canSplit) by default, the moved items' (mvOk) for a drop
  function insOk(pid) { return !pid || (!!VS.items[pid] && canSplit(VS.items[pid])); }
  function insOpts(a, b, ok) {
    var db = b ? rowDepth(b) : 0, out = [];
    ok = ok || insOk;
    if (!a) return [{ depth: 0, parent: null, after: '' }]; // above the list's first row: the first root item
    var it = VS.items[a.getAttribute('data-item')], da = rowDepth(a);
    if (!it || !a.matches('.irow')) return out;
    if (ok(it.id)) out.push({ depth: da + 1, parent: it.id, after: '' });
    // after a, or after one of its ancestors: the item lands below that ancestor's subtree, which ends at a
    for (var x = it, d = da; x && d >= db; d--) {
      var p = x.parent ? VS.items[x.parent] : null;
      if (ok(x.parent || null)) out.push({ depth: d, parent: x.parent || null, after: x.id });
      x = p;
    }
    return out.sort(function (m, k) { return m.depth - k.depth; });
  }
  function insPick(opts, rel) {
    var best = 0;
    opts.forEach(function (o, i) { if (Math.abs(rel - (12 + o.depth * IND)) < Math.abs(rel - (12 + opts[best].depth * IND))) best = i; });
    return best;
  }
  // the depth markers' markup for the shown insert: one per level slot 0..max but the badge's
  var INS_X = '<svg viewBox="0 0 8 8"><path d="M1.5 1.5l5 5M6.5 1.5l-5 5"/></svg>';
  function insMarks(opts, pick, max) {
    var h = '';
    for (var d = 0; d <= max; d++) {
      if (d === pick) continue;
      var ok = opts.some(function (o) { return o.depth === d; });
      h += '<span class="' + (ok ? 'hk' : 'hx') + '" style="--d:' + d + '">' + (ok ? '' : INS_X) + '</span>';
    }
    return h;
  }
  // the row that would parent place o in the gap below row a: the nearest row above the gap one
  // level shallower than the place, when it shows the place's parent; none for a root place
  function insParRow(a, o) {
    if (!a || !o.parent) return null;
    for (var r = a; r; r = r.previousElementSibling) {
      if (!r.matches('.irow')) continue;
      var d = rowDepth(r);
      if (d < o.depth - 1) return null;
      if (d === o.depth - 1) return r.getAttribute('data-item') === o.parent ? r : null;
    }
    return null;
  }
  function insPar(el) {
    if (HP === el) return;
    if (HP) HP.classList.remove('hpar');
    if (el) el.classList.add('hpar');
    HP = el;
  }
  function insWidth(opts) { return 4 + opts[opts.length - 1].depth * IND + 40; } // the gutter: past the deepest badge by a little
  // pointer at (x, y) over row el in list: the gap it is near, or null
  function insGap(list, el, y) {
    var r = el.getBoundingClientRect(), a, b;
    if (y >= r.bottom - INS_EDGE) { a = el; b = rowLike(el.nextElementSibling); }
    else if (y <= r.top + INS_EDGE) {
      a = rowLike(el.previousElementSibling); b = el;
      if (!a && list.querySelector('.irow') !== el) return null; // the top of a later section: no place a create can name
    } else return null;
    return { a: a, b: b, y: (a ? a.getBoundingClientRect().bottom : r.top) - list.getBoundingClientRect().top };
  }
  function insShow(list, g, x) {
    var opts = insOpts(g.a, g.b), rel = x - list.getBoundingClientRect().left;
    if (!opts.length || rel > insWidth(opts)) return insHide();
    HV = { list: list, a: g.a, b: g.b, y: g.y, opts: opts, pick: insPick(opts, rel) };
    var s = list.querySelector(':scope > .hins');
    if (!s) {
      list.insertAdjacentHTML('beforeend', '<div class="hins" aria-hidden="true"><span class="hdep"></span><span class="hl"></span><span class="hb"><svg class="i"><use href="#tki-plus"/></svg></span>' +
        '<button type="button" class="hhit" tabindex="-1"' + attr('act', 'ins') + ' aria-label="' + wsT("js.tk.ins.aria") + '"></button></div>');
      s = list.querySelector(':scope > .hins');
    }
    var o = opts[HV.pick], max = g.a ? rowDepth(g.a) + 1 : 0, marks = insMarks(opts, o.depth, max), dep = s.querySelector('.hdep');
    s.style.top = Math.round(g.y - INS_EDGE) + 'px';
    s.style.setProperty('--ind', o.depth);
    s.style.setProperty('--hmax', max);
    s.classList.toggle('hdeep', max > o.depth);
    if (dep.innerHTML !== marks) dep.innerHTML = marks;
    s.querySelector('.hhit').style.width = insWidth(opts) + 'px';
    s.hidden = false;
    list.classList.add('hins-on');
    insPar(insParRow(g.a, o));
  }
  function insHide() {
    if (!HV) return;
    var s = HV.list.querySelector(':scope > .hins');
    if (s) s.hidden = true;
    HV.list.classList.remove('hins-on');
    insPar(null);
    HV = null;
  }
  function insMove(e) {
    var t = e.target, list = own(t) && t.closest('#card-items[data-view="list"] .plist');
    if (!list || lastPointer === 'touch') return insHide();
    if (t.closest('.hins')) { if (HV) insShow(list, HV, e.clientX); return; }
    var row = t.closest('.irow, .csum, .tmore'), g = row && list.contains(row) && !t.closest('.ir-more') && insGap(list, row, e.clientY);
    if (g) insShow(list, g, e.clientX); else insHide();
  }
  function insOpen() {
    if (!HV) return;
    var o = HV.opts[HV.pick], at = HV.a ? { rk: HV.a.getAttribute('data-rk') } : { rk: HV.b.getAttribute('data-rk'), before: true }, n = ui.nrow[ui.store];
    insHide();
    ui.nrow[ui.store] = nrowNew(n, { parent: o.parent, after: o.after, at: at, depth: o.depth });
    render();
    if ($('tknew')) $('tknew').focus();
  }
  // The move drag: a list row (or the selection, when a selected row is dragged) dropped on a row of
  // the Items list moves there with its subtree (ticket decisions 4, 5, 7). A row's middle band makes
  // it the row's last child, after the last child the list shows; the top and bottom bands make it
  // the sibling before or after the row: the hover insert's places (insOpts) at the row's depth, or,
  // on the bottom band of a row whose children show, its first child. In a filtered list `after`
  // names the shown row above, so siblings the list leaves out keep their places. Several roots land
  // one after another in tree order; the roots the drop refuses stay and are named (the indicator
  // counts them before the drop), the others move. A drop the store would refuse for every root, or
  // that would change nothing, is not offered. The kanban drag is apart: its cards carry data-drag
  // and text/plain, the rows draggable and MV_TYPE.
  var MV_TYPE = 'application/x-ws-tracker-move';
  var MV = null;  // while a row drags: { roots }
  var MVS = null; // the drop shown: { list, d (mvDrop's answer) }
  function hasMv(e) { return !!MV && !!e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], MV_TYPE) >= 0; }
  // does the item sit in root's subtree (root itself included)
  function underRoot(id, root) {
    for (var cur = id, n = 0; cur && VS.items[cur] && n < 1000; cur = VS.items[cur].parent, n++) if (cur === root) return true;
    return false;
  }
  function mvOk(pid) { return MV.roots.some(function (r) { return !moveWhy(r, pid || null); }); }
  // the input rows a list may hold between rows (an anchor scan steps over them)
  function rowGap(x) { return x && x.matches && x.matches('.nrow, .subrow'); }
  // the shown sibling above row r (same depth, same parent) whose item is not moving, '' when none shows
  function mvPrevSib(r, moving) {
    var d = rowDepth(r), par = VS.items[r.getAttribute('data-item')].parent || null;
    for (var x = r.previousElementSibling; x; x = x.previousElementSibling) {
      if (rowGap(x)) continue;
      if (!rowLike(x) || rowDepth(x) < d) return '';
      var xi = rowDepth(x) === d && x.matches('.irow') && VS.items[x.getAttribute('data-item')];
      if (xi && (xi.parent || null) === par && !moving[xi.id]) return xi.id;
    }
    return '';
  }
  // the last child row shown under row whose item is not moving; undefined when none shows (then the item goes last)
  function mvLastKid(row, moving) {
    var d = rowDepth(row), last;
    for (var x = row.nextElementSibling; x; x = x.nextElementSibling) {
      if (rowGap(x)) continue;
      if (!rowLike(x) || rowDepth(x) <= d) break;
      if (rowDepth(x) === d + 1 && x.matches('.irow') && !moving[x.getAttribute('data-item')]) last = x.getAttribute('data-item');
    }
    return last;
  }
  // An anchor the drop's own roots hold moves with them: step up to the shown sibling above it.
  // from: a row at or below the anchor's row (the nearest row of the item is the one meant).
  function mvAnchor(from, after, moving) {
    for (var r = from; after && moving[after];) {
      while (r && !(r.matches && r.matches('.irow') && r.getAttribute('data-item') === after)) r = r.previousElementSibling;
      after = r ? mvPrevSib(r, moving) : '';
    }
    return after;
  }
  // The place a band of row takes: { parent, after, depth (the indicator's indent), from }, or null.
  function mvSlot(list, row, band) {
    var d = rowDepth(row), opts, o, from = row;
    if (band === 'mid') return { parent: row.getAttribute('data-item'), after: undefined, depth: d + 1, from: row };
    if (band === 'top') {
      var a = rowLike(row.previousElementSibling);
      if (!a && list.querySelector('.irow') !== row) return null; // the top of a later section: no place to name (insGap's rule)
      while (a && !a.matches('.irow')) a = rowLike(a.previousElementSibling); // past a "… N more" or "closed N" row
      opts = insOpts(a, row, mvOk); from = a;
      o = opts.filter(function (x) { return x.depth === d; })[0];
    } else {
      var b = row.nextElementSibling;
      while (rowGap(b)) b = b.nextElementSibling;
      opts = insOpts(row, rowLike(b), mvOk);
      o = opts.filter(function (x) { return x.depth === d; })[0] || opts.filter(function (x) { return x.depth === d + 1; })[0];
    }
    return o ? { parent: o.parent, after: o.after, depth: o.depth, from: from } : null;
  }
  // The drop the pointer at clientY over row of list would make: { el, band, slot, n (roots it moves),
  // total }, or null when it moves nothing.
  function mvDrop(list, row, y) {
    var id = row.getAttribute('data-item'), roots = MV.roots;
    if (!VS.items[id] || roots.some(function (r) { return underRoot(id, r); })) return null; // onto what moves
    var rc = row.getBoundingClientRect(), f = (y - rc.top) / (rc.height || 1), mid = mvOk(id);
    var band = f < (mid ? 0.25 : 0.5) ? 'top' : f >= (mid ? 0.75 : 0.5) ? 'bot' : 'mid';
    var slot = mvSlot(list, row, band);
    if (!slot) return null;
    var moving = {}, n = 0, stuck = [];
    roots.forEach(function (r) {
      var why = moveWhy(r, slot.parent || null);
      if (!why) { moving[r] = true; n++; } else stuck.push(wsT("js.tk.mv.stays", VS.items[r] ? VS.items[r].alias : alias(r), why));
    });
    if (!n) return null;
    slot.after = band === 'mid' ? mvLastKid(row, moving) : mvAnchor(slot.from, slot.after, moving);
    if (roots.length === 1 && moveNoop(roots[0], slot)) return null;
    return { el: row, band: band, slot: { parent: slot.parent || null, after: slot.after, depth: slot.depth }, n: n, total: roots.length, stuck: stuck };
  }
  // The indicator: a line at the gap, indented to the place's depth, or the target row outlined
  // for the middle band; "N of M move" when some roots stay, naming each with its reason before
  // the drop (Decision 7).
  function mvShow(list, d) {
    if (MVS && MVS.list !== list) mvHide();
    var s = list.querySelector(':scope > .mvdrop');
    if (!s) {
      list.insertAdjacentHTML('beforeend', '<div class="mvdrop" aria-hidden="true"><span class="hl"></span><span class="mvn"></span></div>');
      s = list.querySelector(':scope > .mvdrop');
    }
    var lt = list.getBoundingClientRect().top, r = d.el.getBoundingClientRect(), mid = d.band === 'mid';
    s.classList.toggle('in', mid);
    s.style.top = Math.round((mid || d.band === 'top' ? r.top : r.bottom) - lt - (mid ? 0 : 1)) + 'px';
    s.style.height = mid ? Math.round(r.height) + 'px' : '';
    s.style.setProperty('--ind', mid ? rowDepth(d.el) : d.slot.depth);
    s.querySelector('.mvn').textContent = d.n < d.total ? wsT("js.tk.mv.part", d.n, d.total) + ': ' + d.stuck.join('; ') : '';
    s.hidden = false;
    MVS = { list: list, d: d };
  }
  function mvHide() {
    if (!MVS) return;
    var s = MVS.list.querySelector(':scope > .mvdrop');
    if (s) s.hidden = true;
    MVS = null;
  }
  function mvEnd() {
    MV = null;
    mvHide();
    Array.prototype.forEach.call(document.querySelectorAll('.tk .mvsrc, .tk .mv-on'), function (x) { x.classList.remove('mvsrc', 'mv-on'); });
  }
  // dragstart on a row: the selection when the row is selected, else the row's item
  function mvStart(e, row) {
    var id = row.getAttribute('data-item'), roots = mselHas(id) ? moveRoots(mselIds()) : [id];
    var stuck = roots.filter(function (r) { return !VS.items[r] || !canMove(VS.items[r]); });
    if (!VS.items[id] || stuck.length === roots.length) {
      e.preventDefault();
      toast(stuck.map(function (r) { return wsT("js.tk.mv.stays", esc(VS.items[r] ? VS.items[r].alias : alias(r)), esc(moveWhy(r, null))); }).join('<br>'));
      return;
    }
    MV = { roots: roots };
    e.dataTransfer.setData(MV_TYPE, roots.join(' '));
    e.dataTransfer.effectAllowed = 'move';
    // the row is the drag image, also when the drag began on its title link
    var rc = row.getBoundingClientRect();
    if (e.dataTransfer.setDragImage) e.dataTransfer.setDragImage(row, Math.max(0, e.clientX - rc.left), Math.max(0, e.clientY - rc.top));
    insHide(); hideTip();
    var list = row.closest('#card-items');
    // after the drag image is taken: the moving rows (subtrees included) dim, and the row previews stay shut
    setTimeout(function () {
      if (!MV || !list) return;
      list.classList.add('mv-on');
      Array.prototype.forEach.call(list.querySelectorAll('.irow'), function (x) { var xi = x.getAttribute('data-item'); if (roots.some(function (r) { return underRoot(xi, r); })) x.classList.add('mvsrc'); });
    }, 0);
  }
  function mvOver(e) {
    if (!hasMv(e)) return;
    var t = e.target, list = own(t) && t.closest && t.closest('#card-items[data-view="list"] .plist'), row = list && t.closest('.irow');
    var d = row && list.contains(row) && VS ? mvDrop(list, row, e.clientY) : null;
    if (!d) { mvHide(); return; }
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    mvShow(list, d);
  }
  function mvDropped(e) {
    if (!hasMv(e)) return;
    e.preventDefault();
    var t = e.target, list = own(t) && t.closest && t.closest('#card-items[data-view="list"] .plist'), row = list && t.closest('.irow');
    var d = row && list.contains(row) && VS ? mvDrop(list, row, e.clientY) : null, roots = MV.roots;
    mvEnd();
    if (d) moveApply(roots, d.slot);
  }

  // "Add subtask": an input row one level in, right under the row the menu opened from (or atop
  // the item window's Subtasks for the window header); ui.sub = { item: parent, rk: that row's
  // key, val }. Title + Enter creates a task under the parent, its creator's like any task made here
  // (529), and keeps the row for the next sibling; Esc closes it and returns to the row.
  function subRowHtml(ind) {
    var it = VS.items[ui.sub.item];
    return '<div class="subrow" style="--ind:' + ind + '"><span class="ind">&#8627;</span><input type="text" id="tksub" data-keep autocomplete="off" value="' + esc(ui.sub.val) + '" placeholder="' + wsT("js.tk.sub.ph") + '" aria-label="' + esc(wsT("js.tk.sub.aria", it ? it.title : '')) + '">' +
      '<button type="button" class="ia"' + attr('act', 'sub-x') + ' aria-label="' + wsT("js.tk.dlg.cancel") + '"' + tipAttr(wsT("js.tk.dlg.cancel")) + '><svg class="i"><use href="#tki-x"/></svg></button></div>';
  }
  // after a render: the subtask row goes under its anchor (the item window draws its own)
  function placeSub() {
    if (!ui.sub) return;
    if (!VS.items[ui.sub.item]) { ui.sub = null; return; }
    if (ui.sub.rk === 'iw:' + ui.sub.item) return;
    var at = document.querySelector('.tk:not(#tkmenu) [data-rk="' + CSS.escape(ui.sub.rk) + '"]');
    if (!at) return;
    var ie = at.style.getPropertyValue('--ind') ? at : at.querySelector('[style*="--ind"]');
    at.insertAdjacentHTML('afterend', subRowHtml((ie ? Number(ie.style.getPropertyValue('--ind')) || 0 : 0) + 1));
  }
  function closeSub() {
    var rk = ui.sub && ui.sub.rk;
    ui.sub = null; render();
    var el = rk && document.querySelector('.tk [data-rk="' + CSS.escape(rk) + '"]');
    if (el && !el.matches('[tabindex], button, a[href]')) el = el.querySelector('[tabindex], button, a[href]');
    if (el) el.focus();
  }
  function createSub() {
    var s = ui.sub, title = $('tksub').value.trim();
    if (!s || !title) return;
    commit([['item.create', { kind: 'task', title: title, parent: s.item }]], null, function () { s.val = ''; });
  }
  // The edit mode's label picker: every label of the store's list, each a toggle; the picked
  // ids live in ui.iedit.labels so a re-render keeps them.
  function labelPickHtml(ed) {
    var ls = VS.labels || [];
    if (!ls.length) return '';
    return '<div class="lpick" role="group" aria-label="' + esc(wsT("js.tk.form.labels")) + '"><span class="muted">' + wsT("js.tk.form.labels") + '</span>' + ls.map(function (l) {
      var on = ed.labels.indexOf(l.id) >= 0;
      return '<button type="button" class="chip lbl' + (on ? ' on' : '') + '"' + attr('act', 'ilabel') + attr('label', l.id) + ' aria-pressed="' + on + '"><span class="ldot" style="background:' + esc(l.color) + '"></span>' + esc(l.name) + '</button>';
    }).join('') + '</div>';
  }
  // The item window's edit mode and a comment's edit in place.
  function startEdit(id) {
    var it = VS.items[id];
    if (!it) return;
    if (!(ui.win && ui.item === id)) showItem(id);
    // seen and base pin the body revision the editor started from; body follows the textarea
    ui.iedit = { item: id, title: it.title, body: it.body || '', seen: it.body || '', base: (it.bodyRev || []).slice(), project: it.project || '', labels: (it.labels || []).slice(), mpick: [] };
    delete ui.edpv['iw-ebody']; delete ui.eatt['iw-ebody'];
    render();
    if ($('iw-etitle')) $('iw-etitle').focus();
  }
  function endEdit() { ui.iedit = null; render(); var b = $('tkwinbody').querySelector('.iw-more'); if (b) b.focus(); }
  function saveEdit() {
    var e = ui.iedit, it = e && VS.items[e.item];
    if (!it) return;
    var ep = { item: it.id }, cur = { title: $('iw-etitle').value, body: $('iw-ebody').value, project: $('iw-eproj').value };
    ['title', 'project'].forEach(function (f) { if (cur[f] !== (it[f] || '')) ep[f] = cur[f]; });
    // the body is judged against the revision loaded at startEdit, not the live one a concurrent
    // edit may have moved; sent with base, the store merges it three-way with edits since
    if (cur.body !== e.seen) { ep.body = cur.body; if (e.base.length) ep.base = e.base; }
    // labels the list no longer holds are dropped from the pick, so an edit never writes one
    var picked = e.labels.filter(function (x) { return !!labelOf(x); });
    if (picked.slice().sort().join(' ') !== (it.labels || []).slice().sort().join(' ')) ep.labels = picked;
    if (!('title' in ep) && !('body' in ep) && !('project' in ep) && !('labels' in ep)) { toast(wsT("js.tk.toast.unchanged")); return endEdit(); }
    if ('body' in ep) {
      var km = keptMentions(withPicks(it.mentions, e.mpick), ep.body), ers = refsIn(ep.body);
      if (km.length) ep.mentions = km;
      if (ers.length) ep.refs = ers;
      var eas = savedAtts('iw-ebody', ep.body);
      if (eas.length) ep.attachments = eas;
    }
    commit([['item.edit', ep]], null, function () { ui.iedit = null; });
  }
  function startCedit(cid) {
    var c = VS.comments[cid];
    if (!c) return;
    ui.cedit = { cid: cid, body: c.body, mpick: [] };
    delete ui.edpv.tkcedit; delete ui.eatt.tkcedit;
    render();
    var t = $('tkcedit');
    if (t) { t.focus(); t.setSelectionRange(t.value.length, t.value.length); }
  }
  function endCedit() {
    var cid = ui.cedit && ui.cedit.cid;
    ui.cedit = null; render();
    var b = cid && $('tkwinbody').querySelector('[data-act="cedit"][data-cid="' + CSS.escape(cid) + '"]');
    if (b) b.focus();
  }
  function saveCedit() {
    var e = ui.cedit, ocm = e && VS.comments[e.cid];
    if (!ocm) return;
    var cep = { comment: e.cid, body: $('tkcedit').value };
    if (cep.body === ocm.body) { toast(wsT("js.tk.toast.unchanged")); return endCedit(); }
    var ckm = keptMentions(withPicks(ocm.mentions, e.mpick), cep.body), cers = refsIn(cep.body);
    if (ckm.length) cep.mentions = ckm;
    if (cers.length) cep.refs = cers;
    var ceas = savedAtts('tkcedit', cep.body);
    if (ceas.length) cep.attachments = ceas;
    commit([['comment.edit', cep]], null, function () { ui.cedit = null; });
  }

  // Kanban drop: only the viewer's own live assignment moves, by the own status menu's rules
  // (ownStatusOpts: done closes, a refused choice says why); an org superior's wrap-up of someone
  // else's card goes through that card's menu.
  function dropOn(item, col) {
    var v = memberOf(viewerOf(ui.store)), r = VS.items[item] && VS.items[item].assignments.map(A).filter(function (x) { return x.to === v && x.state === 'live'; })[0];
    if (!r) return toast(wsT("js.tk.toast.notmine"));
    if (col === r.status || VS.items[item].kind === 'topic') return; // a topic has no status to move
    var o = ownStatusOpts(r).filter(function (x) { return x.s === col; })[0];
    if (!o) return toast(wsT("js.tk.toast.notstatus", esc(stLabel(col))));
    if (o.dis) return toast(esc(o.why));
    if (col === 'done') return commit([['item.close', { item: item }]]);
    commit([['item.status', { item: item, status: col }]]);
  }

  // --------------------------------------------------------------- sync actions

  // The apply path (panel collapse, leaving the page, switching stores, the Sync button):
  // the adapter closes the open batch, pushes, and fetches; `quiet` skips the toast.
  function syncNow(name, quiet, then) {
    AD.sync(name, function (r) {
      r = r || {};
      if (!quiet) {
        var got = r.received || 0;
        if (r.error) toast(wsT("js.tk.toast.syncfail", esc(r.error)));
        else if (r.noRemote) toast(wsT("js.tk.toast.noremote"));
        else if (r.offline) toast(wsT("js.tk.toast.offline"));
        else toast(got ? wsTn("js.tk.toast.synced.late", got, r.pushed || 0) : wsT("js.tk.toast.synced", r.pushed || 0));
      }
      if (then) then(r);
    });
  }
  // The immediate check when a store comes on screen (its drawer opens, its page is entered,
  // its viewer changes); what it brings arrives through onChange, and done() runs once it
  // landed.
  function checkNow(name, done) { AD.check(name, done); }

  // ------------------------------------------------------------------ render

  // The handle row's width (plus the 6px gap to the host's handle) and the header's bottom
  // edge, where the drawer hangs. The width first: the host header pads clear of it, which
  // can change how the header wraps and so where it ends.
  function measure() {
    var w = Math.ceil($('tkhandles').getBoundingClientRect().width);
    document.documentElement.style.setProperty('--tkhw', (w ? w + 6 : 0) + 'px');
    $('tkdrawer').style.setProperty('--tktop', Math.max(0, Math.round(O.header.getBoundingClientRect().bottom)) + 'px');
  }
  // The drawer's height is the user's, and one for every store: dragging any store's
  // handle sets it, clamped between DR_MIN and 80% of the window like the dashboard drawer
  // when it shows. It is the host's drawer height when the host gives opts.drawerHeight
  // (so the host's drawers and every store open alike), else this renderer's own, kept as
  // display state; never set, it is DR_DEF, not the content's height, so a fresh browser
  // shows one height too. Panels scroll inside whatever the height is.
  var DR_MIN = 160, DR_DEF = 320, DR_SHUT = 80;
  function drMax() { return Math.round(window.innerHeight * 0.8); }
  function drClamp(h) { var hi = drMax(); return Math.max(Math.min(DR_MIN, hi), Math.min(hi, h)); }
  function drHeight() { var h = O.drawerHeight ? +O.drawerHeight.get() : ui.drh; return h > 0 ? h : DR_DEF; }
  function drSetHeight(h) {
    if (O.drawerHeight) { O.drawerHeight.set(h); return; }
    ui.drh = h; dsSave('drawer.height', { h: h });
  }
  function fitDrawer() { $('tkdrawer').style.setProperty('--tkh', drClamp(drHeight()) + 'px'); }
  // The drag: the handle follows the pointer; the clip and the handle track the pointer's
  // extent (--tkext) while the sheet keeps at least DR_MIN, so a short drag reveals the
  // sheet rather than squashing it. The release sets the height, or closes short of
  // DR_SHUT. A press that never moves is a click.
  var drDrag = null, drDragged = false;
  function drDragStart(e) {
    var b = e.target.closest && e.target.closest('.tk-handle[data-handle]');
    if (!b || (e.button !== undefined && e.button !== 0)) return;
    drDrag = { name: b.getAttribute('data-handle'), y0: e.clientY, off: e.clientY - b.getBoundingClientRect().top, moved: false, ext: 0 };
    try { b.setPointerCapture(e.pointerId); } catch (x) { /* no capture: the moves still land */ }
  }
  function drDragMove(e) {
    var d = drDrag;
    if (!d || (!d.moved && Math.abs(e.clientY - d.y0) < 5)) return;
    if (!d.moved) { d.moved = true; if (ui.open !== d.name) openStore(d.name); } // dragging a closed handle opens its store
    var max = drMax(), top = Math.max(0, Math.round(O.header.getBoundingClientRect().bottom)), dr = $('tkdrawer');
    d.ext = Math.max(0, Math.min(max, e.clientY - d.off - top));
    dr.classList.add('drag');
    dr.style.setProperty('--tkh', Math.max(Math.min(DR_MIN, max), d.ext) + 'px');
    dr.style.setProperty('--tkext', d.ext + 'px');
  }
  function drDragEnd() {
    var d = drDrag;
    drDrag = null;
    if (!d || !d.moved) return;
    drDragged = true;
    setTimeout(function () { drDragged = false; }, 0);
    $('tkdrawer').classList.remove('drag');
    $('tkdrawer').style.removeProperty('--tkext');
    if (d.ext < DR_SHUT) { fitDrawer(); if (ui.open) collapse(); return; }
    drSetHeight(drClamp(d.ext));
    fitDrawer();
  }

  // One handle per store, stacking leftward from the host's handle in stores() order, then
  // (with adapter.register) the "+ store" handle, also with no store at all; the row is
  // rebuilt when the store list changes. Each store's handle sits in a slot with its fold
  // button (tkf-<store>), which fills the slot while that store's drawer is open (the
  // handle rides the drawer's bottom edge then) and closes it as a click on the handle does.
  var handleSig = null;
  function syncHandles() {
    order = (AD.stores() || []).slice();
    var sig = order.join('\n');
    if (sig === handleSig) return;
    handleSig = sig;
    $('tkhandles').innerHTML = order.map(function (n) {
      return '<span class="tk-slot"><button type="button" class="tk-fold" id="tkf-' + esc(n) + '"' + attr('fold', n) + ' aria-controls="tkbody" title="' + esc(wsT("js.tk.handle.fold")) +
        '" aria-label="' + esc(wsT("js.tk.handle.fold")) + '" hidden><svg class="i"><use href="#tki-up"/></svg></button>' +
        '<button class="tk-handle" id="tkh-' + esc(n) + '"' + attr('handle', n) + ' aria-controls="tkbody"><svg class="i"><use href="#tki-tasks"/></svg><span class="hl">' + esc(n) +
        '</span><span class="hoff" title="' + wsT("js.tk.handle.off") + '" hidden>&#8856;</span><span class="tk-badge" hidden></span></button></span>';
    }).join('') + (AD.register ? '<button class="tk-handle tk-add" id="tkadd" data-act="register" aria-haspopup="dialog" title="' + esc(wsT("js.tk.reg.handle.tip")) + '" aria-label="' + esc(wsT("js.tk.reg.handle.tip")) + '"><svg class="i"><use href="#tki-plus"/></svg></button>' : '');
  }
  function renderHandles() {
    order.forEach(function (name) {
      var b = $('tkh-' + name), n = newKeys(name).length, badge = b.querySelector('.tk-badge'), vs = vss[name];
      b.className = 'tk-handle' + (ui.store === name ? ' on' : '') + (ui.open === name ? ' open' : '');
      b.setAttribute('aria-expanded', String(ui.open === name));
      $('tkf-' + name).hidden = !(ui.open === name && !ui.page);
      badge.hidden = !n; badge.textContent = n;
      if (!vs) { b.querySelector('.hoff').hidden = true; b.title = name + ' - ' + (AD.error(name) || wsT("js.tk.loading")); return; }
      b.querySelector('.hoff').hidden = !!(vs.store.remote && vs.sync.online);
      // A store with a display label shows the label, its id in the tooltip.
      b.querySelector('.hl').textContent = vs.store.label || name;
      b.title = storeTitle(vs.store) + (n ? wsT("js.tk.handle.new", n) : '') + (vs.store.remote ? (vs.sync.online ? '' : ' - ' + wsT("js.tk.sync.offline")) : ' - ' + wsT("js.tk.handle.noremote"));
    });
  }

  // The item window makes the rest of the page inert; the renderer remembers which of the
  // body's children it marked, so it releases only those.
  var inerted = [];
  function setInert(on) {
    if (!on) { inerted.forEach(function (el) { el.inert = false; }); inerted = []; return; }
    if (inerted.length) return;
    var keep = [$('tkwin'), $('tkdlg'), $('tktoast'), $('wstoasts'), $('tktip'), $('tkmenu')];
    Array.prototype.forEach.call(document.body.children, function (el) {
      if (keep.indexOf(el) < 0 && !el.inert) { el.inert = true; inerted.push(el); }
    });
  }

  // dropEditorState forgets the previews and uploads of editors that are gone; open says
  // whether the item window shows.
  function dropEditorState(open) {
    // an editor's preview ends with the editor: an edit saved or cancelled, a comment posted, another item
    if (!ui.iedit) { delete ui.edpv['iw-ebody']; delete ui.eatt['iw-ebody']; }
    if (!ui.cedit) { delete ui.edpv.tkcedit; delete ui.eatt.tkcedit; }
    // the wiki editor's preview and uploads go when no store's wiki edit is open (another store's edit keeps them)
    if (!Object.keys(ui.wiki).some(function (k) { return !!ui.wiki[k].edit; })) { delete ui.edpv['wk-ebody']; delete ui.eatt['wk-ebody']; }
    if (!open || ui.itemChanged || ui.clearDraft) { delete ui.edpv.cbox; delete ui.eatt.cbox; }
  }
  // renderDue: a change notification asked for a render that has not run yet (see mount); any
  // render meets it, so a synchronous render() takes the pending one's place.
  var renderDue = false, changeTimer = 0;
  // UNREADS: the rendered store's unread notifications as api.unread() returns them,
  // rebuilt by every render. afterRender runs the host's hooks once a render is done.
  var UNREADS = { store: null, ready: false, entries: [] };
  function afterRender(name) {
    if (X.rendered) X.rendered(name, VS, H);
    if (O.onRender) O.onRender(UNREADS);
  }
  function render() {
    renderDue = false;
    syncHandles();
    order.forEach(function (n) { vss[n] = AD.view(n); });
    Object.keys(vss).forEach(function (n) { if (order.indexOf(n) < 0) delete vss[n]; });
    dropGone();
    var name = ui.store;
    VS = name ? vss[name] : null; EV = {};
    UNREADS = { store: name, ready: false, entries: [] };
    renderHandles();
    $('tkdrawer').classList.toggle('open', !!ui.open);

    // the store renders into the store page or the drawer, never both (the card ids are unique)
    var body = ui.page ? $('tkpage') : $('tkbody'), sb = $('tkwinbody'), top1 = body.scrollTop, top2 = sb.scrollTop;
    (ui.page ? $('tkbody') : $('tkpage')).innerHTML = '';
    $('tkpage').hidden = !ui.page; document.body.classList.toggle('tk-paged', !!ui.page);
    if (!VS) { // nothing to show yet: the store is loading, or its view could not be read
      hideTip();
      body.innerHTML = '<div class="tk-wait">' + esc(name ? AD.error(name) || wsT("js.tk.loading") : wsT("js.tk.nostore")) + '</div>';
      ui.win = false; $('tkwin').hidden = true; sb.innerHTML = ''; setInert(false);
      if (!ui.page) fitDrawer();
      afterRender(name);
      measure();
      return;
    }
    var s = viewerOf(name), v = memberOf(s);
    VS.events.forEach(function (e) { EV[e.id] = e; });
    if (ui.item && !VS.items[ui.item]) { ui.item = null; ui.win = false; }
    // per-render row marks: flagged writes in an item's own thread, unread notification entries per item
    FLAGGED = {}; UNREAD = {};
    VS.events.forEach(function (e) { if (e.status === 'flagged' && e.thread && VS.items[e.thread]) FLAGGED[e.thread] = (FLAGGED[e.thread] || 0) + 1; });
    var rm = readMap();
    UNREADS.ready = true;
    feedEntries(v).forEach(function (x) {
      if (rm[x.key]) return;
      UNREAD[x.item] = (UNREAD[x.item] || 0) + 1;
      var src = x.ev && (VS.comments[x.ev] || EV[x.ev]), who = src && (src.author || src.member);
      UNREADS.entries.push({ store: name, item: x.item, ev: x.ev, key: x.key, kind: x.kind, label: wsT(NK[x.kind]), title: VS.items[x.item].title || alias(x.item), author: who ? nm(who) : '' });
    });
    // With no drawer, store page, or item window open, nothing of the store shows but its
    // handle: the marks above and the host's rendered hook stay current, the drawer is left as
    // it is (the render that opens it draws it), and nothing is measured but the handle row.
    if (!ui.open && !ui.page && !ui.win) {
      hideTip();
      insPar(null); HV = null;
      $('tkwin').hidden = true; setInert(false);
      if (sb.firstChild) sb.innerHTML = '';
      dropEditorState(false);
      if (MN && MN.kind === 'sync') { $('tkmenu').innerHTML = syncPopHtml(); placeMenu(); }
      else if (MN && mnStale()) closeMenu(false);
      ui.itemChanged = false; ui.clearDraft = false;
      afterRender(name);
      measure();
      return;
    }

    var draft = $('cbox') ? $('cbox').value : '';
    // cards scroll inside; keep each card's scroll and the focused row across the re-render
    var scrolls = {}, fa = document.activeElement, focusRk = fa && fa.closest && body.contains(fa) && fa.closest('[data-rk]');
    focusRk = focusRk && focusRk.getAttribute('data-rk');
    var focusSearch = fa && (fa.id === 'psearch' || fa.id === 'wksearch') ? [fa.id, fa.selectionStart, fa.selectionEnd] : null;
    // the wiki's page and tree scroll inside the card body; a page just opened starts at the top
    var wkScroll = {}; ['wkpage', 'wktree'].forEach(function (k) { if ($(k)) wkScroll[k] = $(k).scrollTop; });
    // an inline input (new item, subtask, in-place edit, the composer) keeps focus and its caret
    var focusKeep = fa && fa.id && fa.hasAttribute('data-keep') ? [fa.id, fa.selectionStart, fa.selectionEnd] : null;
    // an editor's Write / Preview toggle keeps focus too (it has focus while the editor previews)
    var focusTab = fa && fa.classList && fa.classList.contains('tked-tab') ? '.tked-tab[data-ed="' + CSS.escape(fa.getAttribute('data-ed')) + '"][data-mode="' + fa.getAttribute('data-mode') + '"]' : null;
    var focusFk = fa && fa.closest && (body.contains(fa) || sb.contains(fa)) && fa.closest('[data-fk]'); // the fold buttons keep keyboard focus across their re-render
    focusFk = focusFk && focusFk.getAttribute('data-fk');
    Array.prototype.forEach.call(body.querySelectorAll('.card[id] > .cbody'), function (c) { scrolls[c.parentNode.id] = c.scrollTop; });
    hideTip();
    insPar(null); HV = null;
    body.innerHTML = drawerHtml(name, v);
    placeSub(); placeNew();
    if (!ui.page) fitDrawer();
    Array.prototype.forEach.call(body.querySelectorAll('.card[id] > .cbody'), function (c) { if (scrolls[c.parentNode.id]) c.scrollTop = scrolls[c.parentNode.id]; });
    body.scrollTop = top1;
    wkAfterDraw(body, wkScroll);
    if (focusSearch && $(focusSearch[0])) { $(focusSearch[0]).focus({ preventScroll: true }); $(focusSearch[0]).setSelectionRange(focusSearch[1], focusSearch[2]); }
    var ff = focusFk && lastInput === 'key' && body.querySelector('[data-fk="' + CSS.escape(focusFk) + '"]');
    if (ff) ff.focus({ preventScroll: true });
    var fr = focusRk && lastInput === 'key' && body.querySelector('[data-rk="' + CSS.escape(focusRk) + '"]');
    if (fr) fr.focus({ preventScroll: true });
    var sr = ui.sel && body.querySelector('[data-rk="' + CSS.escape(ui.sel) + '"]');
    if (sr) placeMore(sr);
    // the item window: one at a time; the rest of the page is inert while it is open
    var open = !!(ui.win && ui.item);
    $('tkwin').hidden = !open;
    $('tkwin').firstElementChild.setAttribute('aria-labelledby', 'iw-title');
    setInert(open);
    dropEditorState(open);
    if (open) {
      sb.innerHTML = windowHtml(v, VS.items[ui.item]);
      wkMarkLinks(sb);
      if (!ui.itemChanged && !ui.clearDraft && $('cbox')) $('cbox').value = draft;
      sb.scrollTop = ui.itemChanged ? 0 : top2;
      // the notification's target: a thin mark stays while the window is open; the flash and the scroll happen once
      var fx = ui.focus && focusTarget(sb, ui.focus.ev);
      if (fx) {
        fx.classList.add('fx-mark');
        if (ui.focus.flash) { fx.classList.add('fx-flash'); fx.scrollIntoView({ block: 'center' }); }
      }
      if (ui.focus) ui.focus.flash = false;
      var wf = focusFk && lastInput === 'key' && sb.querySelector('[data-fk="' + CSS.escape(focusFk) + '"]');
      if (wf) wf.focus({ preventScroll: true });
    } else sb.innerHTML = '';
    if (open) placeSub();
    var fk2 = focusKeep && $(focusKeep[0]);
    if (fk2) { fk2.focus({ preventScroll: true }); try { fk2.setSelectionRange(focusKeep[1], focusKeep[2]); } catch (x) { /* not a text field */ } }
    var ft = focusTab && document.querySelector(focusTab);
    if (ft) ft.focus({ preventScroll: true });
    if (MP) mentionSync(); // the editors were redrawn: the popup follows the new box, or closes with it
    // the menu stays over the redraw: the status popover follows the store, an item menu closes with its item
    if (MN && MN.kind === 'sync') {
      var mIn = $('tkmenu').contains(document.activeElement);
      $('tkmenu').innerHTML = syncPopHtml(); placeMenu();
      if (mIn) { var mf = $('tkmenu').querySelector('button'); if (mf) mf.focus({ preventScroll: true }); }
    } else if (MN && mnStale()) closeMenu(false);
    ui.itemChanged = false; ui.clearDraft = false;
    afterRender(name);
    measure();
  }

  // A store comes on screen: the immediate check, then what is new is shown once. What
  // the check brings in is new on this showing too: when the check lands after this
  // returns (a network round trip), those keys join the fresh set and are marked seen
  // then, so they do not come back as a badge on the next open. A check that lands
  // synchronously is already in the view read below.
  function bringUp(name) {
    var now = true, shown;
    checkNow(name, function () {
      if (now || ui.store !== name || ui.fresh !== shown || !viewing()) return;
      vss[name] = AD.view(name);
      markSeen(name, true);
      render();
    });
    now = false;
    vss[name] = AD.view(name);
    markSeen(name);
    shown = ui.fresh;
  }
  // Collapsing the open drawer pushes (the apply path).
  function collapse() { var n = ui.open; ui.open = null; syncNow(n, true); render(); }
  // foldStore is a click on store name's fold button: its open drawer closes as a click on
  // its handle closes it, and keyboard focus goes to that handle, since the button hides.
  function foldStore(name) {
    if (ui.open !== name) return;
    var kb = document.activeElement === $('tkf-' + name);
    openStore(name);
    if (kb && $('tkh-' + name)) $('tkh-' + name).focus();
  }
  function openStore(name) {
    if (ui.open === name) { collapse(); return; }
    if (ui.open) syncNow(ui.open, true); // switching to another store while one is open applies it
    if (ui.store !== name) { ui.item = null; ui.win = false; ui.last = null; ui.itemChanged = true; $('tkbody').scrollTop = 0; }
    ui.store = name; ui.open = name;
    bringUp(name);
    render();
    document.dispatchEvent(new CustomEvent('tk:drawer-open', { detail: { store: name } }));
  }
  // openItemWindow(storeId, itemId): the one entry point that opens an item, from a row, a
  // notification, a chat reference, a subtask link, or (later) a URL route. It switches to
  // the store when needed (the drawer stays as it is, or closes when it shows another
  // store), replaces any open window, and moves focus into it. Returns false when the
  // store or the item is unknown to the current viewer.
  // `opts` is { focus: eventId, opener: element } (a bare element still means the opener): the
  // window then scrolls to and marks what the event refers to, and just opens when the
  // window has nothing for it. An event URL route would reuse exactly this.
  function openItemWindow(storeId, itemId, opts) {
    var opener = opts && opts.nodeType ? opts : opts && opts.opener, focus = opts && !opts.nodeType && opts.focus;
    if ((AD.stores() || []).indexOf(storeId) < 0) return false;
    var sv = AD.view(storeId);
    if (!sv || !sv.items[itemId]) return false;
    if (ui.page && ui.page !== storeId) { history.pushState(null, '', '#/' + encodeURIComponent(storeId)); applyRoute(); } // on a store page: go to the item's store page
    if (ui.store !== storeId) {
      if (ui.open) { syncNow(ui.open, true); ui.open = null; }
      ui.store = storeId; ui.last = null; ui.sel = null;
    }
    if (!ui.win) ui.winReturn = returnSel(opener || document.activeElement);
    if (ui.item !== itemId) { ui.iedit = null; ui.cedit = null; if (ui.sub && ui.sub.rk.indexOf('iw:') === 0) ui.sub = null; }
    if (MN) closeMenu(false);
    ui.item = itemId; ui.win = true; ui.itemChanged = true; ui.aopen = null; ui.replyTo = null; ui.mdraft = []; ui.adraft = [];
    ui.focus = focus ? { ev: focus, flash: true } : null;
    render();
    $('tkwin').firstElementChild.focus({ preventScroll: true });
    return true;
  }
  // openNotify(storeId) brings a store's Notify tab up: on its store page when a store page
  // shows, else in its drawer, opened when it is not. The wide layout shows notifications
  // in a column of their own, which the side-tab choice leaves alone.
  function openNotify(storeId) {
    if ((AD.stores() || []).indexOf(storeId) < 0) return false;
    var ts = (ui.tab[storeId] = ui.tab[storeId] || { side: 'notify', narrow: 'items' });
    ts.side = 'notify'; ts.narrow = 'notify';
    if (ui.page) {
      if (ui.page !== storeId) { history.pushState(null, '', '#/' + encodeURIComponent(storeId)); applyRoute(); }
      else render();
    } else if (ui.open !== storeId) openStore(storeId);
    else render();
    return true;
  }
  function showItem(id, opener, focus) { openItemWindow(ui.store, id, { opener: opener, focus: focus }); }
  // uiCommand performs a store support chat's UI command (h.uiCommand) on store id's view,
  // as the user's own operation would: search puts cmd.arg in the Items search box and
  // sends it at once (no typing debounce to wait out; searchFor), focus opens the item whose alias is cmd.arg
  // (at the thread comment whose alias is cmd.comment, when given), scope and view switch
  // the Items scope and view mode. Search, scope, and view bring the Items panel up: the
  // Items tab when narrow (where it replaces the chat's tab), the items side of the
  // Items | Wiki switch when wide. A command for a store other than the one shown, or one
  // the view cannot follow (an unknown item or comment, a scope or mode the panel does not
  // offer now), does nothing. Returns whether it acted.
  function uiCommand(id, cmd) {
    var sv = id === ui.store && AD.view(id);
    if (!sv || !cmd) return false;
    var findAlias = function (ids, a) { var hit = null; ids.some(function (k) { if (alias(k) === a) { hit = k; return true; } return false; }); return hit; };
    if (cmd.op === 'focus') {
      var iid = findAlias(Object.keys(sv.items), cmd.arg);
      if (!iid) return false;
      var cid = cmd.comment ? findAlias(listOf(sv.items[iid].thread).filter(function (k) { return sv.comments[k]; }), cmd.comment) : null;
      return openItemWindow(id, iid, cid ? { focus: cid } : null);
    }
    VS = sv; // scopeList reads the viewer's team from the shown view
    var ps = panelState();
    if (cmd.op === 'search') { ps.q = String(cmd.arg || ''); searchState(id).now = true; }
    else if (cmd.op === 'scope') {
      if (!scopeList().some(function (x) { return x[0] === cmd.arg; })) return false;
      ps.scope = cmd.arg;
    } else if (cmd.op === 'view') {
      var sc = scopeOf();
      if (!SCOPE_MODES[sc].some(function (x) { return x[0] === cmd.arg; })) return false;
      ps.modes[sc] = cmd.arg; dsSave('panel.modes', ui.pmodes);
    } else return false;
    if (ui.layout === 'narrow') tabState().narrow = 'items';
    else if (AD.wiki) wk().mode = 'items';
    ui.sel = null;
    render();
    return true;
  }
  // A selector for the element focus returns to: the row, feed entry, or reference that
  // opened the window survives a re-render under the same data-rk / data-nkey / data-cref.
  function returnSel(el) {
    if (!el || el === document.body) return null;
    var k = ['rk', 'nkey', 'cref', 'handle'];
    for (var i = 0; i < k.length; i++) { var c = el.closest('[data-' + k[i] + ']'); if (c) return '[data-' + k[i] + '="' + CSS.escape(c.getAttribute('data-' + k[i])) + '"]'; }
    return el.id ? '#' + CSS.escape(el.id) : null;
  }
  function closeItemWindow() {
    var back = ui.winReturn;
    ui.win = false; ui.winReturn = null; ui.focus = null; ui.replyTo = null; ui.mdraft = []; ui.adraft = []; ui.iedit = null; ui.cedit = null;
    if (ui.sub && ui.sub.rk.indexOf('iw:') === 0) ui.sub = null;
    if (MN) closeMenu(false);
    render();
    var el = back && document.querySelector(back);
    if (el && !el.matches('a[href], button, [tabindex]')) el = el.querySelector('a[href], button, [tabindex]'); // a feed entry: its title link
    if (el) el.focus();
  }

  // The store page: the route #/<storeId> shows that store full-page, with the same Items
  // panel and tab pane layout, in place of the conversation and the drawer. It is the only
  // route the renderer keeps. Entering a page checks and shows what is new, like opening
  // the drawer; leaving it pushes, like a collapse. Clearing the hash, the browser back
  // button, or "Back" returns to the host's own view. A hash that is not a #/ route is the
  // host page's and is left alone.
  // The link #/<storeId>/<itemId> (a tracker event hook's url, which points at this
  // machine's own daemon) enters that store page and opens the item once the store's view
  // holds it: applyRoute replaces it with the store page's route at once, so the item is not
  // a route of its own, and back leaves the page.
  function routeStore() {
    var m = /^#\/([^/]+)$/.exec(location.hash), id = m && decodeURIComponent(m[1]);
    return id && (AD.stores() || []).indexOf(id) >= 0 ? id : null;
  }
  function routeItem() {
    var m = /^#\/([^/]+)\/([^/]+)$/.exec(location.hash);
    if (!m) return null;
    try { return { store: decodeURIComponent(m[1]), item: decodeURIComponent(m[2]) }; } catch (e) { return null; }
  }
  // openRouteItem opens the item a link named (ui.routeItem) once its store's view has
  // arrived; an item the view does not hold is dropped.
  function openRouteItem() {
    var r = ui.routeItem, sv = r && AD.view(r.store);
    if (!sv) return;
    ui.routeItem = null;
    if (sv.items[r.item] && ui.page === r.store) openItemWindow(r.store, r.item);
  }
  function applyRoute() {
    var ri = routeItem();
    if (ri && (AD.stores() || []).indexOf(ri.store) >= 0) {
      ui.routeItem = ri;
      history.replaceState(null, '', location.pathname + location.search + '#/' + encodeURIComponent(ri.store));
    }
    var s = routeStore();
    if (/^#\/./.test(location.hash) && !s) toast(wsT("js.tk.toast.nostore", esc(location.hash.slice(2))));
    if (s !== ui.page) {
      if (ui.page) syncNow(ui.page, true);
      if (s) {
        if (ui.open) { syncNow(ui.open, true); ui.open = null; }
        if (ui.store !== s) { ui.item = null; ui.win = false; ui.last = null; ui.sel = null; ui.itemChanged = true; }
        ui.store = s;
        bringUp(s);
      }
      ui.page = s;
    }
    render();
    openRouteItem();
  }
  function leavePage() { history.pushState(null, '', location.pathname + location.search); applyRoute(); }

  // The quick switcher: the magnifier in the header opens one search over the stores (by
  // name and label) and the items of every store (by title; items hidden by a tombstone
  // are left out, everything else is visible to every member), read from each store's
  // current view state. A store opens its page; an item opens its window; a host action
  // (ext.qsActions) opens its dialog. Arrow keys move, Enter picks, Esc closes, and focus
  // returns to where it was.
  var qs = { open: false, items: [], idx: 0, back: null };
  function qResults(q0) {
    var q = q0.trim().toLowerCase();
    var out = [];
    order.forEach(function (n) {
      var st = vss[n] ? vss[n].store : { label: '' };
      if (!q || n.toLowerCase().indexOf(q) >= 0 || String(st.label).toLowerCase().indexOf(q) >= 0) out.push({ kind: 'store', store: n, label: st.label });
    });
    if (X.qsActions && VS) (X.qsActions(ui.store, VS, q) || []).forEach(function (x) { out.push({ kind: 'action', key: x.key, icon: x.icon, title: x.title, hint: x.hint, tag: x.tag }); });
    if (q) order.forEach(function (n) {
      var V = vss[n];
      if (!V) return;
      V.itemOrder.forEach(function (id) {
        var it = V.items[id];
        if (!it.hidden && it.title.toLowerCase().indexOf(q) >= 0) out.push({ kind: 'item', store: n, item: id, title: it.title, alias: it.alias, st: it.kind === 'topic' ? 'topic' : it.status });
      });
    });
    return out.slice(0, 30);
  }
  function qRender() {
    var list = qs.items = qResults($('tkqin').value), last = null, h = '';
    if (qs.idx >= list.length) qs.idx = Math.max(0, list.length - 1);
    list.forEach(function (x, i) {
      if (x.kind !== last) { last = x.kind; h += '<li class="qgrp" role="presentation">' + (x.kind === 'store' ? wsT("js.tk.qs.stores") : x.kind === 'action' ? wsT("js.tk.qs.actions") : wsT("js.tk.tab.items")) + '</li>'; }
      h += '<li class="qopt' + (i === qs.idx ? ' on' : '') + '" role="option" id="tkqopt-' + i + '" aria-selected="' + (i === qs.idx) + '"' + attr('qi', i) + '>' +
        (x.kind === 'action' ? '<svg class="i"><use href="' + esc(x.icon || '#tki-plus') + '"/></svg><b>' + esc(x.title) + '</b><span class="muted">' + esc(x.hint || '') + '</span><span class="grow"></span><span class="qk">' + esc(x.tag || '') + '</span>' :
          x.kind === 'store' ? '<svg class="i"><use href="#tki-tasks"/></svg><b>' + esc(x.store) + '</b><span class="muted">' + esc(x.label) + '</span><span class="grow"></span><span class="qk">' + wsT("js.tk.qs.openpage") + '</span>' :
          '<span class="sti">' + SI.svg(x.st) + '</span><span class="qt">' + esc(x.title) + '</span><span class="grow"></span><code class="al">' + esc(x.alias) + '</code><span class="qk">' + esc(x.store) + '</span>') + '</li>';
    });
    $('tkqlist').innerHTML = h || '<li class="qnone" role="presentation">' + wsT("js.tk.qs.none") + '</li>';
    $('tkqin').setAttribute('aria-activedescendant', list.length ? 'tkqopt-' + qs.idx : '');
    var on = $('tkqlist').querySelector('.qopt.on');
    if (on) on.scrollIntoView({ block: 'nearest' });
  }
  function qOpen() {
    if (qs.open) return;
    qs.open = true; qs.idx = 0; qs.back = document.activeElement;
    $('tkqpop').hidden = false; $('tkqs').setAttribute('aria-expanded', 'true');
    $('tkqin').value = ''; qRender(); $('tkqin').focus();
  }
  function qClose(keepFocus) {
    if (!qs.open) return;
    qs.open = false; $('tkqpop').hidden = true; $('tkqs').setAttribute('aria-expanded', 'false');
    var b = qs.back; qs.back = null;
    if (!keepFocus && b && b.isConnected) b.focus(); else if (!keepFocus) $('tkqs').focus();
  }
  function qPick(i) {
    var x = qs.items[i];
    if (!x) return;
    qClose(true);
    if (x.kind === 'action') { if (X.dialogs && X.dialogs[x.key]) openDialog(x.key, {}); else if (X.action) X.action(x.key, null, H); return; }
    if (x.kind === 'store') { if (ui.page !== x.store) location.hash = '#/' + encodeURIComponent(x.store); else $('tkqs').focus(); }
    else if (!openItemWindow(x.store, x.item, { opener: $('tkqs') })) toast(wsT("js.tk.toast.hiddenitem"));
  }

  // The immediate custom tooltip: one fixed element shown at once for whatever [data-tip]
  // the pointer or keyboard focus is on (the browser's title waits ~1s).
  var tipFor = null;
  function showTip(el) {
    var t = $('tktip'), up = el.matches('.kcard, .citem') ? parentTip(el) : '';
    tipFor = el; t.textContent = el.getAttribute('data-tip') + (up ? '\n' + up : ''); t.hidden = false;
    var r = el.getBoundingClientRect(), w = t.offsetWidth, h = t.offsetHeight;
    var x = Math.max(6, Math.min(r.left + r.width / 2 - w / 2, window.innerWidth - w - 6)), y = r.bottom + 6;
    if (y + h > window.innerHeight - 6) y = r.top - h - 6;
    t.style.left = Math.round(x) + 'px'; t.style.top = Math.round(y) + 'px';
  }
  function hideTip() { tipFor = null; $('tktip').hidden = true; }
  // Kanban and calendar cards stay independent tasks. Hovering or focusing one marks its direct
  // parent's and direct children's cards in the same view (lk-par, lk-kid); the card's tooltip
  // names the parent when its card is not on screen (another month, or scrolled out of view).
  var LK = null; // the card whose links are marked
  function linkCards(box, id) { return id ? box.querySelectorAll('.kcard[data-item="' + CSS.escape(id) + '"], .citem[data-item="' + CSS.escape(id) + '"]') : []; }
  function linkHi(card) {
    if (card === LK) return;
    if (LK && LK.isConnected) Array.prototype.forEach.call(LK.closest('.kanban, .cal').querySelectorAll('.lk-on, .lk-par, .lk-kid'), function (n) { n.classList.remove('lk-on', 'lk-par', 'lk-kid'); });
    LK = card || null;
    var box = card && card.closest('.kanban, .cal'), it = box && VS && VS.items[card.getAttribute('data-item')];
    if (!it) return;
    card.classList.add('lk-on');
    Array.prototype.forEach.call(linkCards(box, it.parent), function (n) { n.classList.add('lk-par'); });
    it.children.forEach(function (k) { Array.prototype.forEach.call(linkCards(box, k), function (n) { if (n !== card) n.classList.add('lk-kid'); }); });
  }
  function onScreen(n) {
    var r = n.getBoundingClientRect(), v = { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight };
    for (var p = n.parentElement; p && !p.classList.contains('card'); p = p.parentElement) {
      if (getComputedStyle(p).overflow === 'visible') continue;
      var q = p.getBoundingClientRect();
      v = { left: Math.max(v.left, q.left), top: Math.max(v.top, q.top), right: Math.min(v.right, q.right), bottom: Math.min(v.bottom, q.bottom) };
    }
    return r.right > v.left && r.left < v.right && r.bottom > v.top && r.top < v.bottom;
  }
  function parentTip(card) {
    var box = card.closest('.kanban, .cal'), it = box && VS && VS.items[card.getAttribute('data-item')], par = it && VS.items[it.parent];
    if (!par || Array.prototype.some.call(linkCards(box, par.id), onScreen)) return '';
    return wsT("js.tk.link.parent", par.title);
  }
  // A row's detail overlay (the quick preview; the item window holds the full view) is
  // fixed-positioned so a short, content-height card does not clip it: below the row, or
  // above when below would leave the open drawer and above has more room.
  function placeMore(row) {
    var m = row.querySelector(':scope > .ir-more');
    if (!m) return;
    var r = row.getBoundingClientRect(), c = $(ui.page ? 'tkpage' : 'tkclip').getBoundingClientRect(), ind = Number(row.style.getPropertyValue('--ind')) || 0;
    var left = r.left + 28 + ind * 16;
    m.style.left = Math.round(left) + 'px'; m.style.width = Math.round(Math.max(220, r.right - 6 - left)) + 'px';
    var h = m.offsetHeight, lo = Math.min(c.bottom, window.innerHeight) - 4, hi = Math.max(c.top, 0) + 4;
    var down = r.bottom - 3 + h <= lo || lo - r.bottom >= r.top - hi;
    m.style.top = Math.round(down ? r.bottom - 3 : r.top + 3 - h) + 'px';
  }
  function placeShownMore() { Array.prototype.forEach.call(document.querySelectorAll('.tk .irow:hover, .tk .irow:focus-within, .tk .irow.sel'), placeMore); }
  var lastPointer = 'mouse', lastInput = 'pointer'; // the focused row is restored after a re-render only for keyboard use

  var QUICK = { confirm: 1, unassign: 1, accept: 1, ihide: 1, chide: 1, comment: 1, claim: 1, unopen: 1, approve: 1, unrequest: 1, uncc: 1 };
  // the item menu's steps an action button opens (in the menu, or a menu anchored to the button)
  var STEPS = { note: 1, handoff: 1, assign: 1, pri: 1, due: 1, move: 1, opening: 1, request: 1, wrap: 1, cc: 1 };

  // ------------------------------------------------------------- display state

  // Per-card view modes, the drawer height, the recently-closed fold, and the tree folds persist under
  // opts.storagePrefix; everything else lasts the page. Storage may throw (a locked-down
  // browser): then the values last the page.
  function dsLoad(k) {
    try { var m = JSON.parse(localStorage.getItem(PFX + k)); return m && typeof m === 'object' && !Array.isArray(m) ? m : {}; } catch (e) { return {}; }
  }
  function dsSave(k, m) { try { localStorage.setItem(PFX + k, JSON.stringify(m)); } catch (e) { /* storage off: the value lasts this page */ } }

  // ---------------------------------------------------------------- mount

  // The icons the renderer draws, under its own ids (tki-*) so a host sprite never collides.
  var SPRITE = '<svg width="0" height="0" style="position:absolute" aria-hidden="true">' +
    '<symbol id="tki-back" viewBox="0 0 16 16"><path d="M10 3.5L5.5 8l4.5 4.5"/></symbol>' +
    '<symbol id="tki-chev" viewBox="0 0 16 16"><path d="M4 6l4 4 4-4"/></symbol>' +
    '<symbol id="tki-up" viewBox="0 0 16 16"><path d="M4 10l4-4 4 4"/></symbol>' +
    '<symbol id="tki-tasks" viewBox="0 0 16 16"><path d="M2.5 4.2l1.4 1.4 2.4-2.6M2.5 10.2l1.4 1.4 2.4-2.6M8.5 4.6h5M8.5 10.6h5"/></symbol>' +
    '<symbol id="tki-plus" viewBox="0 0 16 16"><path d="M8 3v10M3 8h10"/></symbol>' +
    '<symbol id="tki-minus" viewBox="0 0 16 16"><path d="M4 8h8"/></symbol>' +
    '<symbol id="tki-x" viewBox="0 0 16 16"><path d="M4 4l8 8M12 4l-8 8"/></symbol>' +
    '<symbol id="tki-accept" viewBox="0 0 16 16"><path d="M3.2 8.4l3 3 6.6-7"/></symbol>' +
    '<symbol id="tki-decline" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.6"/><path d="M5.9 5.9l4.2 4.2M10.1 5.9l-4.2 4.2"/></symbol>' +
    '<symbol id="tki-chat" viewBox="0 0 16 16"><path d="M2.8 3.8h10.4v6.8H7.4l-2.8 2.4v-2.4H2.8z"/></symbol>' +
    '<symbol id="tki-confirm" viewBox="0 0 16 16"><path d="M1.8 8.6l2.8 2.8 5.6-6M7.6 11.2l.4.2 6.2-6.6"/></symbol>' +
    '<symbol id="tki-reject" viewBox="0 0 16 16"><path d="M5.6 4L2.8 6.8l2.8 2.8"/><path d="M2.8 6.8h6.6a3.6 3.6 0 0 1 0 7.2H6.6"/></symbol>' +
    '<symbol id="tki-return" viewBox="0 0 16 16"><path d="M13 13V9.4a2.8 2.8 0 0 0-2.8-2.8H3.4M6.4 3.6l-3 3 3 3"/></symbol>' +
    '<symbol id="tki-close" viewBox="0 0 16 16"><rect x="2.4" y="2.4" width="11.2" height="11.2" rx="2.4"/><path d="M5.2 8.2l2 2 3.6-3.8"/></symbol>' +
    '<symbol id="tki-split" viewBox="0 0 16 16"><circle cx="4" cy="3.6" r="1.6"/><circle cx="4" cy="12.4" r="1.6"/><circle cx="12" cy="8" r="1.6"/><path d="M4 5.2v5.6M4 7.2c0 1 1 1.6 2.4 1.2l4-.5"/></symbol>' +
    '<symbol id="tki-handoff" viewBox="0 0 16 16"><circle cx="5.6" cy="5" r="2.2"/><path d="M1.8 13.4c.4-2.6 2-3.8 3.8-3.8 1 0 1.9.3 2.6 1M9.6 9.8h4.8M12.4 7.8l2 2-2 2"/></symbol>' +
    '<symbol id="tki-status" viewBox="0 0 16 16"><circle cx="8" cy="8" r="5.6"/><path d="M8 2.4a5.6 5.6 0 0 1 0 11.2z" fill="currentColor"/></symbol>' +
    '<symbol id="tki-assign" viewBox="0 0 16 16"><circle cx="6.4" cy="5" r="2.4"/><path d="M2 13.6c.4-2.8 2.2-4 4.4-4s4 1.2 4.4 4M12.6 4.6v4M10.6 6.6h4"/></symbol>' +
    '<symbol id="tki-withdraw" viewBox="0 0 16 16"><path d="M3.2 8h9.6M6.4 4.8L3.2 8l3.2 3.2"/></symbol>' +
    '<symbol id="tki-warn" viewBox="0 0 16 16"><path d="M8 2.4l6 10.8H2z"/><path d="M8 6.6v3M8 11.4v.05"/></symbol>' +
    '<symbol id="tki-search" viewBox="0 0 16 16"><circle cx="7" cy="7" r="4.4"/><path d="M10.3 10.3l3.4 3.4"/></symbol>' +
    '<symbol id="tki-list" viewBox="0 0 16 16"><path d="M5.6 4h8M5.6 8h8M5.6 12h8M2.3 4h.1M2.3 8h.1M2.3 12h.1"/></symbol>' +
    '<symbol id="tki-tree" viewBox="0 0 16 16"><path d="M2.4 3.4h6M4.6 3.4v9.2M4.6 8h2.6M4.6 12.6h2.6M9 8h4.6M9 12.6h4.6"/></symbol>' +
    '<symbol id="tki-kanban" viewBox="0 0 16 16"><rect x="2" y="3" width="3.2" height="10" rx=".8"/><rect x="6.4" y="3" width="3.2" height="6.4" rx=".8"/><rect x="10.8" y="3" width="3.2" height="8.2" rx=".8"/></symbol>' +
    '<symbol id="tki-timeline" viewBox="0 0 16 16"><path d="M2 2.4v11.2M4.6 4.6h5M7.2 8h6.4M4.6 11.4h4"/></symbol>' +
    '<symbol id="tki-calendar" viewBox="0 0 16 16"><rect x="2.2" y="3.4" width="11.6" height="10.2" rx="1.6"/><path d="M2.2 6.8h11.6M5.4 2v2.8M10.6 2v2.8"/></symbol>' +
    '<symbol id="tki-bell" viewBox="0 0 16 16"><path d="M8 2.6a3.6 3.6 0 0 0-3.6 3.6v2.4L3 10.8h10l-1.4-2.2V6.2A3.6 3.6 0 0 0 8 2.6zM6.6 12.8a1.5 1.5 0 0 0 2.8 0"/></symbol>' +
    '<symbol id="tki-team" viewBox="0 0 16 16"><circle cx="5.6" cy="5.4" r="2.1"/><path d="M1.8 13c.3-2.4 1.8-3.6 3.8-3.6s3.5 1.2 3.8 3.6"/><circle cx="11.2" cy="6" r="1.7"/><path d="M10.6 9.6c1.8-.1 3.2.9 3.6 3"/></symbol>' +
    '<symbol id="tki-activity" viewBox="0 0 16 16"><path d="M1.8 8h3l1.6-4.6 3 9 1.6-4.4h3.2"/></symbol>' +
    '<symbol id="tki-more" viewBox="0 0 16 16"><circle cx="3.4" cy="8" r="1.3" fill="currentColor" stroke="none"/><circle cx="8" cy="8" r="1.3" fill="currentColor" stroke="none"/><circle cx="12.6" cy="8" r="1.3" fill="currentColor" stroke="none"/></symbol>' +
    '<symbol id="tki-send" viewBox="0 0 16 16"><path d="M2.5 8L13.5 2.8 10.6 13.4 7.6 9.1z"/><path d="M7.6 9.1l5.9-6.3"/></symbol>' +
    '<symbol id="tki-flag" viewBox="0 0 16 16"><path d="M3.6 14V2.6M3.6 3h8.4l-1.8 3 1.8 3H3.6"/></symbol>' +
    '<symbol id="tki-tag" viewBox="0 0 16 16"><path d="M2.6 2.6h5.2l5.8 5.8-5.2 5.2-5.8-5.8z"/><circle cx="5.6" cy="5.6" r="1.1"/></symbol>' +
    '<symbol id="tki-pen" viewBox="0 0 16 16"><path d="M10.6 2.8l2.6 2.6-7.6 7.6-3.2.6.6-3.2z"/></symbol>' +
    '<symbol id="tki-copy" viewBox="0 0 16 16"><rect x="5.5" y="5.5" width="8" height="8" rx="1.2"/><path d="M10.5 3.5v-.3a.7.7 0 0 0-.7-.7H3.2a.7.7 0 0 0-.7.7v6.6a.7.7 0 0 0 .7.7h.3"/></symbol>' +
    '<symbol id="tki-page" viewBox="0 0 16 16"><path d="M9.5 2.5h4v4M13.5 2.5L8 8M11.5 9.5v3.2a.8.8 0 0 1-.8.8H3.3a.8.8 0 0 1-.8-.8V5.3a.8.8 0 0 1 .8-.8h3.2"/></symbol>' +
    // the wiki (cardWiki)
    '<symbol id="tki-book" viewBox="0 0 16 16"><path d="M8 4.2C6.6 3 4.6 2.6 2.4 2.8v9.6c2.2-.2 4.2.2 5.6 1.4 1.4-1.2 3.4-1.6 5.6-1.4V2.8C11.4 2.6 9.4 3 8 4.2zM8 4.2v9.6"/></symbol>' +
    '<symbol id="tki-doc" viewBox="0 0 16 16"><path d="M4 2.4h5.2L12 5.2v8.4H4z"/><path d="M9 2.6v2.8h2.8M6 8.4h4M6 10.8h4"/></symbol>' +
    '<symbol id="tki-folder" viewBox="0 0 16 16"><path d="M2 4.2h4l1.4 1.6H14v6.7H2z"/></symbol>' +
    '<symbol id="tki-sidebar" viewBox="0 0 16 16"><rect x="2.2" y="2.8" width="11.6" height="10.4" rx="1.4"/><path d="M6.2 2.8v10.4"/></symbol>' +
    '<symbol id="tki-history" viewBox="0 0 16 16"><path d="M2.8 8a5.2 5.2 0 1 0 1.6-3.8"/><path d="M2.6 2.6v2.6h2.6M8 5.2V8l2 1.4"/></symbol>' +
    '<symbol id="tki-link" viewBox="0 0 16 16"><path d="M6.8 9.2a2.6 2.6 0 0 0 3.7 0l2-2a2.6 2.6 0 0 0-3.7-3.7l-.8.8M9.2 6.8a2.6 2.6 0 0 0-3.7 0l-2 2a2.6 2.6 0 0 0 3.7 3.7l.8-.8"/></symbol>' +
    '<symbol id="tki-reload" viewBox="0 0 16 16"><path d="M13 8a5 5 0 1 1-1.5-3.6"/><path d="M12 2v2.8H9.2"/></symbol>' +
    '</svg>';

  function el(tag, props, html) {
    var n = document.createElement(tag);
    Object.keys(props).forEach(function (k) { n.setAttribute(k, props[k]); });
    if (html) n.innerHTML = html;
    return n;
  }
  function buildDom() {
    var b = document.body;
    b.appendChild(el('div', { class: 'tk-sprite', 'aria-hidden': 'true' }, SPRITE));
    b.appendChild(el('div', { class: 'tk tk-drawer', id: 'tkdrawer' },
      '<div class="tk-dr-in"><div class="tk-dr-clip" id="tkclip"><div class="tk-dr-sheet" role="region" aria-label="' + wsT("js.tk.aria.store") + '"><div class="tk-dr-body" id="tkbody"></div></div></div>' +
      '<div class="tk-handles" id="tkhandles"></div></div>'));
    O.pageParent.appendChild(el('main', { class: 'tk tk-spage', id: 'tkpage', role: 'region', 'aria-label': wsT("js.tk.aria.page"), hidden: '' }));
    b.appendChild(el('div', { class: 'tk tk-iwin-back', id: 'tkwin', hidden: '' }, '<div class="iwin" role="dialog" aria-modal="true" tabindex="-1"><div class="iwin-in" id="tkwinbody"></div></div>' +
      '<ul class="tk-mpop" id="tkmpop" role="listbox" aria-label="' + wsT("js.tk.composer.mention.tip") + '" hidden></ul>'));
    b.appendChild(el('dialog', { class: 'tk tk-dlg', id: 'tkdlg' }));
    // a page without the shared toast stack (the tracker mockup) gets the renderer's own
    if (!window.WsToast) b.appendChild(el('div', { class: 'tk tk-toast', id: 'tktoast', hidden: '' }));
    b.appendChild(el('div', { class: 'tk tk-tip', id: 'tktip', role: 'tooltip', hidden: '' }));
    b.appendChild(el('div', { class: 'tk tk-menu', id: 'tkmenu', role: 'menu', hidden: '' }));
    O.qsSlot.appendChild(el('span', { class: 'tk tk-qswrap' },
      '<button class="btn tk-qsbtn" id="tkqs" title="' + wsT("js.tk.qs.tip") + '" aria-haspopup="dialog" aria-expanded="false"><svg class="i"><use href="#tki-search"/></svg></button>' +
      '<span class="tk-qpop" id="tkqpop" role="dialog" aria-label="' + wsT("js.tk.qs.tip") + '" hidden>' +
      '<input id="tkqin" type="text" role="combobox" aria-controls="tkqlist" aria-expanded="true" aria-autocomplete="list" autocomplete="off" spellcheck="false" placeholder="' + wsT("js.tk.qs.placeholder") + '">' +
      '<ul id="tkqlist" role="listbox" aria-label="' + wsT("js.tk.qs.results") + '"></ul>' +
      '<span class="tk-qhint">' + wsT("js.tk.qs.hint") + '</span></span>'));
  }
  // Delegated handlers act only on the renderer's own nodes, so the host page's clicks,
  // changes, and keys stay the host's.
  function own(t) { return !!(t && t.closest && t.closest('.tk')); }
  // the editor (editorHtml's wrapper) a drag is over, when it is one
  function edDrop(e) { return e.target.closest ? e.target.closest('.tked[data-ed]') : null; }
  function hasFiles(e) { return !!e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], 'Files') >= 0; }
  // the files a clipboard or a drop carries
  function filesOf(dt) {
    if (!dt) return [];
    if (dt.items && dt.items.length) return Array.prototype.map.call(dt.items, function (it) { return it.kind === 'file' ? it.getAsFile() : null; }).filter(Boolean);
    return Array.prototype.slice.call(dt.files || []);
  }

  // A store image (an attachment, or a wiki's image) fails while its store is busy: the daemon
  // answers 503 while a fetch or push holds the store, and a script never sees an image's
  // status. A failed one loads again after IMG_RETRY_MS, with a query the daemon ignores so
  // the browser does not reuse the failure, up to IMG_RETRIES times per element; an element a
  // redraw replaced meanwhile is left alone.
  var IMG_RETRY_MS = 1500, IMG_RETRIES = 3;
  var IMG_ROUTE = /^\/tracker\/stores\/[^/]+\/(wiki\/[^/]+\/)?attachments\/[^/]+$/;
  function imgRetry(e) {
    var img = e.target;
    if (!img || img.tagName !== 'IMG') return;
    var u;
    try { u = new URL(img.src, location.href); } catch (x) { return; }
    if (u.origin !== location.origin || !IMG_ROUTE.test(u.pathname)) return;
    var n = +(img.getAttribute('data-tk-retry') || 0);
    if (n >= IMG_RETRIES) return;
    img.setAttribute('data-tk-retry', String(n + 1));
    setTimeout(function () {
      if (!img.isConnected) return;
      u.searchParams.set('retry', String(n + 1));
      img.src = u.pathname + u.search;
    }, IMG_RETRY_MS);
  }

  function listen() {
    // dragging a handle sets the drawer height; a drag's trailing click is swallowed
    var hs = $('tkhandles');
    hs.addEventListener('pointerdown', drDragStart);
    hs.addEventListener('pointermove', drDragMove);
    hs.addEventListener('pointerup', drDragEnd);
    hs.addEventListener('pointercancel', drDragEnd);
    hs.addEventListener('click', function (e) { if (drDragged) e.stopPropagation(); }, true);

    document.addEventListener('click', function (e) {
      var t = e.target;
      if (qs.open && !(t.closest && t.closest('#tkqpop, #tkqs'))) qClose(true);
      if (mentionBox(t)) mentionSync(); // a click into an @token opens it, elsewhere in the box closes it
      else if (MP && !(t.closest && t.closest('#tkmpop, #cmention'))) mentionClose();
      // a click outside the menu closes it; a click on its own opener toggles it shut
      if (MN && !(t.closest && t.closest('#tkmenu'))) {
        var mOpener = MN.backEl && MN.backEl.contains(t);
        closeMenu(mOpener || t === $('tkwin'));
        if (mOpener || t === $('tkwin')) { e.preventDefault(); return; }
      }
      if (!own(t)) return;
      if (t === $('tkwin')) { closeItemWindow(); return; } // the backdrop
      if (t.closest('#tkqs')) { if (qs.open) qClose(); else qOpen(); return; }
      var qo = t.closest('[data-qi]');
      if (qo) { qPick(Number(qo.getAttribute('data-qi'))); return; }
      var h = t.closest('.tk-handle[data-handle]');
      if (h) { openStore(h.getAttribute('data-handle')); return; }
      var fb = t.closest('.tk-fold[data-fold]');
      if (fb) { foldStore(fb.getAttribute('data-fold')); return; }
      // a wiki: link in rendered text (the daemon marks it data-wiki / data-page) opens that page in the panel
      var wl = t.closest('a[data-wiki][data-page]');
      if (wl && AD.wiki) { e.preventDefault(); wkOpenLink(wl.getAttribute('data-wiki'), wl.getAttribute('data-page')); return; }
      // the wiki's menus close on a click elsewhere in the tracker
      if (AD.wiki && ui.store && wk().pick && !t.closest('.wk-pickw')) { wk().pick = false; render(); }
      if (AD.wiki && ui.store && wk().more && !t.closest('.wk-more')) { wk().more = false; render(); }
      var a = t.closest('[data-act]');
      if (a) {
        e.preventDefault(); e.stopPropagation();
        var k = a.getAttribute('data-act'), d = { item: a.getAttribute('data-item'), who: a.getAttribute('data-who'), op: a.getAttribute('data-op'), req: a.getAttribute('data-req'), cid: a.getAttribute('data-cid'), label: a.getAttribute('data-label') };
        if (k.indexOf('wk-') === 0) wkAct(k, a);
        else if (k === 'win-close') closeItemWindow();
        else if (k === 'page-back') leavePage();
        else if (k === 'afold') { ui.aopen = ui.aopen === ui.item ? null : ui.item; render(); }
        else if (k === 'menu') itemMenu(d.item, a);
        else if (k === 'mstatus') itemMenu(d.item, a, null, { statusOnly: true });
        else if (k.indexOf('m-') === 0) { if (MN) menuAct(k, a); }
        else if (STEPS[k]) itemMenu(d.item, a, null, { step: k, op: d.op, who: d.who, req: d.req });
        else if (k === 'edit') { closeMenu(false); startEdit(d.item); }
        else if (k === 'iedit-save') saveEdit();
        else if (k === 'ilabel' && ui.iedit) { var il = ui.iedit.labels, ix = il.indexOf(d.label); if (ix >= 0) il.splice(ix, 1); else il.push(d.label); render(); }
        else if (k === 'lsearch') labelSearch(d.label);
        else if (k === 'claimf') claimableFilter();
        else if (k === 'slabel-rm') labelRemove(d.label);
        else if (k === 'iedit-x') endEdit();
        else if (k === 'cedit') startCedit(d.cid);
        else if (k === 'cedit-save') saveCedit();
        else if (k === 'cedit-x') endCedit();
        else if (k === 'create') openNewRow();
        else if (k === 'ins') insOpen();
        else if (k === 'msel') { var mr = a.closest('.irow'); if (mr) mselClick(mr, e); }
        else if (k === 'msel-clear') { mselClear(); render(); }
        else if (k === 'mdet') { var mk = ui.store + '|' + d.who; ui.mdet[mk] = !ui.mdet[mk]; render(); }
        else if (k === 'idcopy') copyId(a);
        else if (k === 'new-x') closeNewRow();
        else if (k === 'nchip') newMenu(a.getAttribute('data-step'), a);
        else if (k === 'nkind') { var nk0 = ui.nrow[ui.store]; if (nk0) nrowKind(nk0.kind === 'topic' ? 'task' : 'topic'); }
        else if (k === 'knew') openKnew();
        else if (k === 'knew-x') closeKnew();
        else if (k === 'sub-x') closeSub();
        else if (k === 'rib') openMenu({ kind: 'sync' }, a);
        else if (k === 'actf') { var af = a.getAttribute('data-f'); ui.act[af] = !ui.act[af]; render(); }
        else if (k === 'tab') { tabState()[ui.layout === 'narrow' ? 'narrow' : 'side'] = a.getAttribute('data-tab'); ui.sel = null; render(); }
        else if (k === 'today') {
          var ps = panelState(), set = a.getAttribute('data-set');
          ps.today = ps.today === set ? null : set;
          if (ps.today && !scopeHasToday(set, scopeOf(), panelMode(scopeOf()), memberOf(viewerOf(ui.store)))) ps.scope = TODAY_HOME[set];
          if (ui.layout === 'narrow') tabState().narrow = 'items';
          ui.sel = null; render();
        }
        else if (k === 'dlg-cancel') closeDialog();
        else if (k === 'open') showItem(d.item);
        else if (k === 'bref') showItem(d.item, a, d.cid);
        else if (k === 'tdrill') { // a team head's count: drill to that person's rows of it; the same count again, or the clear control, clears it
          var dps = panelState(), dset = a.getAttribute('data-set');
          dps.drill = !dset || (dps.drill && dps.drill.who === d.who && dps.drill.key === dset) ? null : { who: d.who, key: dset };
          ui.sel = null; render();
        }
        else if (k === 'scope') { panelState().scope = a.getAttribute('data-scope'); ui.sel = null; render(); }
        else if (k === 'pmode') { panelState().modes[scopeOf()] = a.getAttribute('data-mode'); dsSave('panel.modes', ui.pmodes); render(); }
        else if (k === 'rel-fold') { var rfs = panelState(), rk0 = a.getAttribute('data-set'); rfs.relFold = rfs.relFold || {}; rfs.relFold[rk0] = !rfs.relFold[rk0]; render(); }
        else if (k === 'closed-toggle') { var cps = panelState(); cps.closedOpen = !cps.closedOpen; ui.closedOpen[ui.store] = cps.closedOpen; dsSave('closed', ui.closedOpen); render(); }
        else if (k === 'tree-closed') { var to = panelState().treeOpen, pid = a.getAttribute('data-parent'); to[pid] = !to[pid]; render(); }
        else if (k === 'tfold') { var fk = ui.store + '|' + a.getAttribute('data-node'), fto = a.getAttribute('data-to'); if (fto) ui.tfold[fk] = fto; else delete ui.tfold[fk]; dsSave('tree.fold', ui.tfold); render(); }
        else if (k === 'wfold') { ui.wfold[ui.store + '|' + a.getAttribute('data-node')] = a.getAttribute('aria-expanded') !== 'true'; render(); }
        else if (k === 'psearch-clear') { panelState().q = ''; render(); if ($('psearch')) $('psearch').focus(); }
        else if (k === 'cmode') { (ui.cmodes[ui.store] = ui.cmodes[ui.store] || {})[a.getAttribute('data-card')] = a.getAttribute('data-mode'); dsSave('card.modes', ui.cmodes); render(); }
        else if (k === 'cal') { ui.calMonth[a.getAttribute('data-card')] = a.getAttribute('data-month'); render(); }
        else if (k === 'sync') { syncNow(ui.store, false, function () { render(); }); }
        else if (k === 'reply') { ui.replyTo = d.cid; delete ui.edpv.cbox; render(); if ($('cbox')) $('cbox').focus(); }
        else if (k === 'reply-x') { ui.replyTo = null; render(); }
        else if (k === 'mention') mentionInsert();
        else if (k === 'ed-mode') edMode(a.getAttribute('data-ed'), a.getAttribute('data-mode'));
        else if (k === 'hstatus') setStatus(VS.items[d.item], a.getAttribute('data-status'), '');
        else if (k === 'hsub') startSub(d.item, 'iw:' + d.item);
        else if (k === 'asg-add') openAssign(d.item, a);
        else if (k === 'nread-all') { markRead(feedEntries(memberOf(viewerOf(ui.store))).map(function (x) { return x.key; })); render(); }
        else if (k === 'att-load') { if (CAPS.attach) { var lm = (ui.loaded[seenKey(ui.store)] = ui.loaded[seenKey(ui.store)] || {}), oid = a.getAttribute('data-oid'); lm[oid] = !lm[oid]; render(); } }
        else if (k === 'att-add') {
          var an = $('catt-name').value.trim(), asz = $('catt-size').value;
          if (!an || asz === '') return toast(wsT("js.tk.toast.attneeds"));
          ui.adraft.push({ oid: mockOid(an + '|' + asz), name: an, size: Number(asz), mime: mimeOf(an) }); render();
        }
        else if (k === 'att-x') { ui.adraft.splice(Number(a.getAttribute('data-idx')), 1); render(); }
        else if (k === 'register') openRegister();
        else if (k === 'store-menu') openMenu({ kind: 'store', store: a.getAttribute('data-store') }, a);
        else if (k === 'store-show') showStore(a.getAttribute('data-store'));
        else if (k === 'store-unreg') openDialog('unregister', { store: a.getAttribute('data-store') });
        else if (QUICK[k]) quick(k, a);
        else if (DIALOGS[k] || (X.dialogs && X.dialogs[k])) openDialog(k, d);
        else if (X.action) X.action(k, a, H);
        return;
      }
      // selection mode: a click on a list row toggles it, Shift adds a range (mselClick), rather than opening the item window
      var mrow = mselOn() && t.closest('#card-items .irow');
      if (mrow) { e.preventDefault(); mselClick(mrow, e); return; }
      // touch: a tap selects a row, which shows its actions and details as a hover would;
      // a tap on the selected row (or on its title) opens the item window
      var row = t.closest('.irow');
      if (row && lastPointer === 'touch' && !t.closest('a.title') && ui.sel !== row.getAttribute('data-rk')) {
        e.preventDefault(); ui.sel = row.getAttribute('data-rk'); render(); return;
      }
      var nk = t.closest('[data-nkey]');
      if (nk) markRead([nk.getAttribute('data-nkey')]); // a feed entry is read once clicked
      var it = t.closest('[data-item]');
      if (it && !t.closest('select, textarea, input, #tkdlg')) { e.preventDefault(); showItem(it.getAttribute('data-item'), t, nk && nk.getAttribute('data-nev')); return; }
      if (t.closest('a[href="#"]')) e.preventDefault();
      // a footnote link in rendered text scrolls to its note; following it would set the
      // hash, which is the store-page route (applyRoute)
      var fn = t.closest('.md a[href^="#"]');
      if (fn) { e.preventDefault(); var to = document.getElementById(decodeURIComponent(fn.getAttribute('href').slice(1))); if (to) to.scrollIntoView({ block: 'nearest' }); }
    });
    document.addEventListener('submit', function (e) {
      var f = e.target;
      if (f.closest && f.closest('#tkdlg, #tkmenu')) { e.preventDefault(); submit(f); }
      else if (f.closest && f.closest('.wk-form')) { e.preventDefault(); wkCreate(); }
    });
    // the search box filters as you type; the render rebuilds the panel, so focus and the caret are put back (see render),
    // and an IME composition (Korean, Japanese) is left alone until it commits
    function searchInput(e) {
      if (e.isComposing || !own(e.target)) return;
      if (e.target.id === 'psearch') { panelState().q = e.target.value; render(); }
      else if (e.target.id === 'wksearch') { wk().q = e.target.value; render(); }
      else if (e.target.id === 'wk-wname') wkNameInput(e.target);
    }
    document.addEventListener('input', searchInput);
    document.addEventListener('compositionend', searchInput);
    document.addEventListener('change', function (e) {
      var t = e.target;
      if (!own(t)) return;
      if (t.id === 'f-kind') { ui.kind = t.value; render(); }
      else if (t.id === 'f-status') { ui.status = t.value; render(); }
      else if (t.id === 'a-family') { ui.act.family = t.value; render(); }
      else if (t.id === 'a-member') { ui.act.member = t.value; render(); }
      else if (t.closest('#tkdlg form')) {
        var df = t.closest('form');
        if (t.name === 'saved') { // picking a saved profile fills the fields
          var o = t.options[t.selectedIndex], sp = o && o.getAttribute('data-p');
          if (sp) { var pr = JSON.parse(sp); df.elements.org.value = pr.org; df.elements.family.value = pr.family; df.elements.given.value = pr.given; if (df.elements.nid) df.elements.nid.value = asciiId(pr.given); }
        }
        if (t.name === 'rtype') rtypeShow(df);
        profileView(df);
      }
    });
    // the inline inputs keep their text in ui, so a re-render draws it back
    document.addEventListener('input', function (e) {
      var t = e.target;
      if (!own(t)) return;
      if (t.id === 'tknew' && ui.nrow[ui.store]) ui.nrow[ui.store].val = t.value;
      else if (t.id === 'tkknew' && ui.knew[ui.store]) ui.knew[ui.store].val = t.value;
      else if (t.id === 'tksub' && ui.sub) ui.sub.val = t.value;
      else if (t.id === 'tkaq' && MN && MN.step === 'assign') { var as = asgState(); as.q = t.value; as.idx = 0; asgDraw(); }
      else if (t.id === 'tkmq' && MN) { MN.q = t.value; var ml = $('tkmenu').querySelector('.mlist'); if (ml) ml.innerHTML = mqList(VS.items[MN.item]); }
      else if (t.id === 'tkmvq' && MN && MN.mv) { MN.q = t.value; var mvl = $('tkmenu').querySelector('.mlist'); if (mvl) mvl.innerHTML = moveList(MN.mv); }
      else edKeep(t);
      if (mentionBox(t)) mentionSync(); // also mid-composition: the popup filters on the composing text
    });
    document.addEventListener('input', function (e) {
      var t = e.target, df = t.closest && t.closest('#tkdlg form');
      if (!df || !df.elements.given) return;
      if (df.elements.saved && PROFILE_FIELDS.indexOf(t.name) >= 0) df.elements.saved.value = '';
      // the register dialog's member id follows the given name until it is typed in
      if (df.getAttribute('data-kind') === 'register' && df.elements.nid) {
        if (t.name === 'nid') df.elements.nid.removeAttribute('data-auto');
        else if (t.name === 'given' && df.elements.nid.hasAttribute('data-auto')) df.elements.nid.value = asciiId(t.value);
      }
      profileView(df);
    });
    document.addEventListener('input', function (e) {
      var f = e.target.closest && e.target.closest('#tkdlg form[data-kind="unregister"]');
      if (f) unregReady(f);
    });
    document.addEventListener('dragstart', function (e) {
      var c = own(e.target) && e.target.closest('[data-drag]');
      if (!c) return;
      e.dataTransfer.setData('text/plain', c.getAttribute('data-drag'));
      e.dataTransfer.effectAllowed = 'move';
    });
    document.addEventListener('dragover', function (e) {
      if (own(e.target) && (e.target.closest('[data-kcol]') || (edDrop(e) && hasFiles(e)))) e.preventDefault();
    });
    // the list's move drag (mvStart): its own listeners, apart from the kanban's above
    document.addEventListener('dragstart', function (e) {
      var row = own(e.target) && e.target.closest && e.target.closest('#card-items .irow[draggable="true"]');
      if (row && VS) mvStart(e, row);
    });
    document.addEventListener('dragover', mvOver);
    document.addEventListener('dragleave', function (e) { if (MVS && !(e.relatedTarget && MVS.list.contains(e.relatedTarget))) mvHide(); });
    document.addEventListener('drop', mvDropped);
    document.addEventListener('dragend', function () { if (MV) mvEnd(); });
    // an image pasted into an editor's textarea, or dropped on an editor (edImages); a paste that
    // also carries text (a copied web page) still pastes its text
    document.addEventListener('paste', function (e) {
      var t = mentionBox(e.target), files = t ? filesOf(e.clipboardData) : [];
      if (!files.length) return;
      if (Array.prototype.indexOf.call(e.clipboardData.types || [], 'text/plain') < 0) e.preventDefault();
      edImages(t.id, files, false);
    });
    document.addEventListener('drop', function (e) {
      var wrap = own(e.target) && edDrop(e);
      if (!wrap || !hasFiles(e)) return;
      e.preventDefault();
      edImages(wrap.getAttribute('data-ed'), filesOf(e.dataTransfer), true);
    });
    document.addEventListener('drop', function (e) {
      var col = own(e.target) && e.target.closest('[data-kcol]');
      if (!col) return;
      e.preventDefault();
      var item = e.dataTransfer.getData('text/plain');
      if (item) dropOn(item, col.getAttribute('data-kcol'));
    });
    document.addEventListener('error', imgRetry, true); // an image's error does not bubble
    document.addEventListener('pointerdown', function (e) { lastPointer = e.pointerType || 'mouse'; lastInput = 'pointer'; }, true);
    document.addEventListener('mouseover', function (e) {
      var tt = own(e.target) && e.target.closest('[data-tip]');
      if (tt && tt !== tipFor) showTip(tt); else if (!tt && tipFor) hideTip();
      var row = own(e.target) && e.target.closest('.irow');
      if (row && !row.contains(e.relatedTarget)) placeMore(row);
      linkHi(own(e.target) && e.target.closest('.kcard, .citem'));
    });
    document.addEventListener('focusin', function (e) {
      if (MP && e.target.id !== MP.box) mentionClose();
      var row = own(e.target) && e.target.closest('.irow'), card = own(e.target) && e.target.closest('.kcard, .citem');
      if (row) placeMore(row);
      linkHi(card);
      if (own(e.target) && e.target.matches('button[data-tip]')) showTip(e.target); else if (card) showTip(card); else if (tipFor) hideTip();
    });
    document.addEventListener('mousemove', insMove);
    document.addEventListener('scroll', function (e) {
      if (tipFor) hideTip(); placeShownMore(); insHide();
      if (MP) mentionSync();
      if (MN && MN.kind === 'item' && !(e.target.nodeType === 1 && $('tkmenu').contains(e.target))) closeMenu(false); // an item menu does not follow a scrolled row
    }, true);
    // a right-click on a row, card, entry, or the item window header opens its menu at the pointer
    document.addEventListener('contextmenu', function (e) {
      var t = e.target;
      if (!own(t) || t.closest('#tkmenu, #tkdlg, input, textarea, select, a[href]:not([href="#"])')) return;
      var m = t.closest('[data-mitem], [data-item]');
      if (!m) return;
      e.preventDefault();
      itemMenu(m.getAttribute('data-mitem') || m.getAttribute('data-item'), m, { x: e.clientX, y: e.clientY });
    });
    document.addEventListener('mouseout', function (e) { if (!e.relatedTarget && tipFor) hideTip(); });
    $('tkqin').addEventListener('input', function () { qs.idx = 0; qRender(); });
    $('tkqin').addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); var n = qs.items.length; if (n) { qs.idx = (qs.idx + (e.key === 'ArrowDown' ? 1 : n - 1)) % n; qRender(); } }
      else if (e.key === 'Enter') { e.preventDefault(); qPick(qs.idx); }
      else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); qClose(); }
    });
    // the caret moved without an edit (arrow keys, Home/End): the popup follows the token under it
    document.addEventListener('keyup', function (e) {
      if (mentionBox(e.target) && /^(ArrowLeft|ArrowRight|Home|End)$/.test(e.key)) mentionSync();
    });
    // the assign popover's rows and chip buttons act on click without taking focus from its search field
    $('tkmenu').addEventListener('mousedown', function (e) { if (e.target.closest('.ap-list .aopt, .ap-chips .ax')) e.preventDefault(); });
    $('tkmenu').addEventListener('mousemove', function (e) {
      var o = e.target.closest('.aopt[data-ai]'), i = o && Number(o.getAttribute('data-ai'));
      if (!o || !MN || !MN.as || i === MN.as.idx) return;
      MN.as.idx = i;
      Array.prototype.forEach.call($('tkalist').querySelectorAll('.aopt'), function (x) { x.classList.toggle('on', x === o); });
      $('tkaq').setAttribute('aria-activedescendant', o.id);
    });
    // a row is picked on mousedown, so the box keeps focus and its caret
    $('tkmpop').addEventListener('mousedown', function (e) {
      e.preventDefault();
      var o = e.target.closest('[data-mp]');
      if (o) mentionPick(Number(o.getAttribute('data-mp')));
    });
    $('tkmpop').addEventListener('mousemove', function (e) {
      var o = e.target.closest('[data-mp]'), i = o && Number(o.getAttribute('data-mp'));
      if (o && MP && i !== MP.idx) { MP.idx = i; mentionDraw($(MP.box)); }
    });
    $('tkqpop').addEventListener('mousemove', function (e) {
      var o = e.target.closest('[data-qi]'), i = o && Number(o.getAttribute('data-qi'));
      if (o && i !== qs.idx) { qs.idx = i; Array.prototype.forEach.call($('tkqlist').querySelectorAll('.qopt'), function (x) { var on = Number(x.getAttribute('data-qi')) === i; x.classList.toggle('on', on); x.setAttribute('aria-selected', String(on)); }); $('tkqin').setAttribute('aria-activedescendant', 'tkqopt-' + i); }
    });
    document.addEventListener('keydown', function (e) {
      lastInput = 'key';
      var t = e.target, id = t.id, chord = e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !e.isComposing, plain = e.key === 'Enter' && !e.isComposing && !e.shiftKey && !e.metaKey && !e.ctrlKey;
      if (MN && $('tkmenu').contains(t)) { menuKey(e); return; }
      // Shift+F10 or the menu key opens the focused row's menu under it
      if ((e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey)) && own(t) && !t.closest('#tkdlg, input, textarea, select')) {
        var mk = t.closest('[data-mitem], [data-item]');
        if (mk) { e.preventDefault(); itemMenu(mk.getAttribute('data-mitem') || mk.getAttribute('data-item'), t.closest('[data-rk]') && !t.matches('button') ? t.closest('[data-rk]') : t); }
        return;
      }
      // an editor showing its preview (focus on its toggle or the pane): Cmd/Ctrl+Enter saves or
      // sends as in its textarea, and Esc goes back to writing rather than closing the window
      var edw = own(t) && t.closest && t.closest('.tked-tabs, .tked-pv') && t.closest('.tked'), eid = edw && edw.getAttribute('data-ed');
      if (eid && ui.edpv[eid] && (chord || e.key === 'Escape')) {
        e.preventDefault();
        if (e.key === 'Escape') edMode(eid, 'write');
        else if (eid === 'iw-ebody') saveEdit();
        else if (eid === 'tkcedit') saveCedit();
        else if (eid === 'wk-ebody') wkSave();
        else if (eid === 'cbox' && $('tkcsend')) quick('comment', $('tkcsend'));
        return;
      }
      // the inline inputs: Enter (or Cmd/Ctrl+Enter in a multi-line field) writes, Esc leaves
      if (own(t) && t.hasAttribute && t.hasAttribute('data-keep')) {
        var esc1 = e.key === 'Escape';
        if (id === 'tknew') { if (plain) { e.preventDefault(); createNew(); } else if (esc1) { e.preventDefault(); closeNewRow(); } return; }
        if (id === 'tkknew') { if (plain) { e.preventDefault(); createKnew(); } else if (esc1) { e.preventDefault(); closeKnew(); } return; }
        if (id === 'tksub') { if (plain) { e.preventDefault(); createSub(); } else if (esc1) { e.preventDefault(); closeSub(); } return; }
        if (id === 'iw-etitle' || id === 'iw-eproj') { if (plain || chord) { e.preventDefault(); saveEdit(); } else if (esc1) { e.preventDefault(); endEdit(); } return; }
        if (t.hasAttribute('data-mention')) {
          // an editor: the Enter that commits an IME composition neither picks, saves, nor sends;
          // while the popup is open, its keys come first and Esc closes it, not the editor
          if (e.isComposing || e.keyCode === 229) return;
          var mp = MP && MP.box === id;
          if (mp && MP.ids.length) {
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); MP.idx = (MP.idx + (e.key === 'ArrowDown' ? 1 : MP.ids.length - 1)) % MP.ids.length; mentionDraw(t); return; }
            if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey) { e.preventDefault(); mentionPick(MP.idx); return; }
          }
          if (mp && esc1) { e.preventDefault(); mentionClose(); return; }
        }
        if (id === 'iw-ebody') { if (chord) { e.preventDefault(); saveEdit(); } else if (esc1) { e.preventDefault(); endEdit(); } return; }
        if (id === 'tkcedit') { if (chord) { e.preventDefault(); saveCedit(); } else if (esc1) { e.preventDefault(); endCedit(); } return; }
        if (id === 'wk-ebody') { if (chord) { e.preventDefault(); wkSave(); } else if (esc1) { e.preventDefault(); wkCancel(); } return; }
        if (id === 'wk-epath') { if (plain || chord) { e.preventDefault(); wkSave(); } else if (esc1) { e.preventDefault(); wkCancel(); } return; }
        if (id === 'wk-wname' || id === 'wk-wfirst') { if (plain) { e.preventDefault(); wkCreate(); } else if (esc1 && wk().creating) { e.preventDefault(); wk().creating = false; render(); } return; }
        if (id === 'cbox' && chord) { e.preventDefault(); if ($('tkcsend')) quick('comment', $('tkcsend')); return; }
      }
      // Esc in the search box clears it first; with it empty, Esc goes on to close the window or the drawer
      if (e.key === 'Escape' && e.target.id === 'psearch' && e.target.value && own(e.target)) { e.preventDefault(); panelState().q = ''; render(); if ($('psearch')) $('psearch').focus(); return; }
      if (e.key === 'Escape' && e.target.id === 'wksearch' && e.target.value && own(e.target)) { e.preventDefault(); wk().q = ''; render(); if ($('wksearch')) $('wksearch').focus(); return; }
      // Esc closes an open wiki menu before anything else
      if (e.key === 'Escape' && own(t) && AD.wiki && ui.store && (wk().pick || wk().more)) { e.preventDefault(); wk().pick = wk().more = false; render(); return; }
      // keyboard: a focused row shows its actions and details; Enter opens its item window
      if (e.key === 'Enter' && e.target.classList && e.target.classList.contains('irow') && own(e.target)) { e.preventDefault(); showItem(e.target.getAttribute('data-item')); return; }
      // ArrowRight unfolds a focused tree row, ArrowLeft folds it
      if ((e.key === 'ArrowRight' || e.key === 'ArrowLeft') && e.target.classList && e.target.classList.contains('irow') && own(e.target)) {
        var tw = e.target.querySelector(':scope > .ir-main > .tw[data-act]');
        if (tw && (tw.getAttribute('aria-expanded') === 'true') === (e.key === 'ArrowLeft')) { e.preventDefault(); tw.click(); }
        return;
      }
      // a host that handled this Esc itself (its own menu, say) marks it defaultPrevented
      if (e.key !== 'Escape' || e.defaultPrevented || $('tkdlg').open) return;
      if (ui.win) closeItemWindow();
      else if ((ui.open || ui.page) && mselOn()) { e.preventDefault(); mselClear(); render(); }
      else if (ui.open) collapse();
    });
    // the layout follows the width: re-render when it crosses a breakpoint, else refit
    window.addEventListener('resize', function () { if (MN) closeMenu(false); if (MP) mentionSync(); if (layoutOf() !== ui.layout) render(); else { if (!ui.page) fitDrawer(); measure(); } });
    // the store page route: reload on #/<storeId> restores the page; back and forward follow the hash
    window.addEventListener('hashchange', applyRoute);
    window.addEventListener('popstate', function () { if (routeStore() !== ui.page) applyRoute(); });
    // the host opened its own drawer: an open store collapses (and so syncs)
    document.addEventListener('ws:drawer-open', function () { if (ui.open) collapse(); });
  }

  function mount(adapter, opts) {
    if (api) return api;
    AD = adapter; CAPS = adapter.caps || {}; X = adapter.ext || {}; O = opts || {};
    PFX = O.storagePrefix || 'ws.tracker.';
    ui.drh = +dsLoad('drawer.height').h || 0; ui.cmodes = dsLoad('card.modes'); ui.pmodes = dsLoad('panel.modes'); ui.closedOpen = dsLoad('closed'); ui.tfold = dsLoad('tree.fold');
    mergeTabs();
    H = { esc: esc, attr: attr, btn: btn, card: card, fld: fld, fmtTs: fmtTs, fmtSize: fmtSize, nm: nm, nmo: nmo, memberOpts: memberOpts,
      profileFields: profileFields, profileText: profileText, asciiId: asciiId, crefHtml: crefHtml, uiCommand: uiCommand,
      openDialog: openDialog, closeDialog: closeDialog, wrote: wrote, toast: toast,
      refresh: function (forget) { if (forget) ui.last = null; render(); } };
    buildDom();
    listen();
    api = {
      refresh: function () { render(); },
      openStore: openStore,
      closeStore: function () { if (ui.open) collapse(); },
      openItemWindow: openItemWindow,
      viewerChanged: function (id) {
        ui.last = null;
        if (id === ui.store && viewing()) bringUp(id);
        render();
      },
      toast: toast,
      unread: function () { return UNREADS; },
      openNotice: function (x) { if (x && openItemWindow(x.store, x.item, x.ev ? { focus: x.ev } : null)) markRead([x.key]); },
      openNotify: openNotify
    };
    syncHandles();
    ui.store = order[0] || null;
    // A change notification renders (and re-reads the wiki shown) on the next timer tick, so
    // the notifications that arrive meanwhile land as one redraw. The views of a read of
    // several stores land in separate tasks, a tick apart or more, so a change with more to
    // come waits up to MORE_HOLD for the rest; the last one (or any other change) brings the
    // redraw forward to the next tick, and a store slower than the hold still draws the others
    // first. A timer, not an animation frame: a hidden tab pauses frames, and the handle badges
    // should still follow there.
    var MORE_HOLD = 100, changeAt = 0;
    AD.onChange(function (more) {
      renderDue = true;
      var at = Date.now() + (more ? MORE_HOLD : 0);
      if (changeTimer && changeAt <= at) return;
      clearTimeout(changeTimer);
      changeAt = at;
      changeTimer = setTimeout(function () { changeTimer = 0; wkRefresh(); if (renderDue) render(); openRouteItem(); }, at - Date.now());
    });
    applyRoute();
    // read-only hook for scripted checks (headless verification); not part of the model
    window.__tracker = { ui: ui, vs: function () { return VS; }, openItemWindow: openItemWindow };
    // the entry point a later URL route uses to open an item from a link
    window.openItemWindow = openItemWindow;
    return api;
  }

  window.WsTracker = { mount: mount };
})();
