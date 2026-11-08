# Plan: tracker drawer UI spine - renderer seam, daemon endpoints, product wiring

Ticket `261108-feat-ws-tracker-drawer-ui`
(`ai-docs/tickets/ready/261108-feat-ws-tracker-drawer-ui.md`; its Decisions
and Constraints govern; this brief is the worker's spine for every leaf).
Branch `impl/develop/bribe-diner-candy`, worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`. Commit on this
branch; never merge, push, rebase, or amend.

Read before editing: `ai-docs/manuals/shipped-surface-boundary.md` (all of
`agents-plugin-tool/`), `ai-docs/manuals/ws-mcp.md` only if you touch
`internal/mcp/`. The mockup's UI contract is `ai-docs/ref/tracker-mockup/`
(`README.md` layout section, `SCENARIOS.md`).

## 1. Files

| file | role |
|---|---|
| `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` | the shared renderer: everything `app.js` renders, plus `status-icons.js` and the pure helpers it took from `engine.js` |
| `agents-plugin-tool/internal/wsdashboard/assets/tracker.css` | the renderer's styles: the tracker rules of `style.css`, without the mockup frame |
| `ai-docs/ref/tracker-mockup/mock-adapter.js` | the engine-and-fixtures adapter and all mockup-only chrome |
| `ai-docs/ref/tracker-mockup/i18n-en.js` | generated English client table for the mockup (Phase 1b) |
| `ai-docs/ref/tracker-mockup/index.html`, `style.css` | the mockup frame; loads the shipped renderer by relative path |

`app.js` and `status-icons.js` are removed once their content has moved.
`engine.js` and `fixtures.js` stay as they are: `engine.js` is the reference
implementation the Go port's golden test reads
(`internal/wstracker/testdata/gen-golden.js`), so it is not edited.

The mockup loads, in order: `fixtures.js`, `engine.js`, `i18n-en.js`,
`../../../agents-plugin-tool/internal/wsdashboard/assets/tracker.js`,
`mock-adapter.js`; styles `style.css` then
`../../../agents-plugin-tool/internal/wsdashboard/assets/tracker.css`. Classic
`<script src>` and `<link>` with relative paths load from `file://`; `fetch`
does not, so nothing is fetched.

## 2. The renderer seam (Decision 3)

`tracker.js` defines `window.WsTracker` and renders nothing until the host page
calls `WsTracker.mount(adapter, opts)`, which returns an `api` object. The
renderer never names `TrackerEngine`, `TRACKER_FIXTURES`, `fetch`, or a daemon
URL; the adapter is its only data source. Its header comment documents the
final adapter, opts, and api contract: that comment is the contract Phase 2
builds on, so keep it exact.

### 2.1 Adapter (data)

All store-keyed calls take the registry store id.

- `stores()` -> ordered array of store ids (handle order).
- `view(id)` -> the latest view-state object for the viewer (the README "View
  state" shape, which `wstrackerhost.StoreView` emits), or `null` while it is
  not loaded or could not be read; `error(id)` -> the reason string or `null`.
  Synchronous: the daemon adapter serves a cache it refreshes, the mockup
  adapter derives on demand. The viewer is `view.viewer` (`stream`,
  `member`); the renderer keeps no viewer of its own. "Today" is
  `view.now.slice(0, 10)`.
- `dispatch(id, events, done)`: `events` = `[{type, payload, origin}]`. A
  payload's `item`, `parent`, `comment`, or `reply_to` field whose value is
  the string `"$N"` (N >= 1, naming an earlier event of the same batch by
  1-based position) is the id that event gets; the adapter resolves it. This
  replaces the mockup's `nextId()`: the renderer never computes an event id.
  `done(res)` exactly once, possibly synchronously:
  `{ok: true, results: [{id, alias, type, status, reason}]}` or
  `{ok: false, error: <string>, refusal: {index, type, reason} | null}`.
  A create-then-open flow opens `res.results[0].id`.
- `check(id)`: the immediate check when a store's drawer opens or its page is
  entered (Decision 7). Fire and forget; the adapter reports any change
  through `onChange`.
- `sync(id, done)`: the apply path at panel collapse, leaving the page,
  switching to another store while one is open, and the Sync button.
  `done(report)` with `{noRemote, offline, closed, pushed, received,
  unpushed, error}` (absent fields read as false/0/empty).
- `seen(id)` -> `{badge: {key: true}, read: {key: true}}`: what this viewer
  has seen (handle-badge seen marks, notification read marks).
  `markSeen(id, {badge: [keys], read: [keys]})` adds marks (a union; nothing
  is ever unmarked); the adapter updates its own copy at once and persists.
  The renderer keeps no seen state of its own besides the per-open "fresh"
  set and the tab dots' signatures (page lifetime, in memory).
- `profiles(done)` -> `done({last: <index or -1>, profiles: [{org, family,
  given}]})`; `saveProfile(profile)`: the machine-local saved profiles
  (Decision 6). The renderer's profile dialog and the register dialog use
  these instead of `localStorage`.
- `register(req, done)` (Phase 2): `req` = `{id, label, remote, member,
  profile: {org, family, given}}` (remote may be empty; no lead field);
  `done({ok: true, store: <id>} | {ok: false, error})`. An adapter without
  `register` gets no "+ store" handle.
- `onChange(fn)`: the renderer subscribes once; the adapter calls `fn()`
  whenever `stores()`, a `view()`, or `seen()` changed, and the renderer
  re-renders.
- `caps`: `{origin, attach, addMember}` booleans, all false when absent.
  `origin`: show the origin pickers (human / agent); product writes are
  human only. `attach`: the composer's attachment drafting and the
  simulated attachment "load" toggle (attachment blobs are out of scope).
  `addMember`: the admin dialog offers `roles.add_member`, which the Go
  schema does not have (a real member arrives by `member.join`; `describe`
  must render `member.join` readably).
- `ext` (optional, mockup chrome): `tabs` (extra side tabs, each `{key,
  icon, title, tip, make()}` returning a card spec like the built-in ones;
  the mockup's Support agent placeholder and Data tabs move here, so the
  product shows neither), `syncTools(id, view)` -> extra HTML in the sync
  line (Go offline / Come back online), `action(k, el)` -> true when it
  handled a `data-act` the renderer does not know, and whatever the join
  simulation needs (its dialog and quick-switcher action). The viewer and
  clone selects, the clock, Go offline / online, the raw JSON editor and
  reset, the theme gear, and the localStorage profile list live in
  `mock-adapter.js` and the mockup's `index.html` only.

### 2.2 Mount options and DOM

The renderer creates its own nodes (drawer, item window, dialog, toast,
tooltip, store page, quick-switcher popup) so a host provides only:

- `opts.header`: the page header element; the tracker drawer hangs from its
  bottom edge (the mockup measures `#top`, the product `.dwrap > .top`).
- `opts.qsSlot`: an element in the header the quick-switcher button and
  popup go into.
- `opts.pageParent`: where the store page node is placed; while a store
  page shows, the body carries the class `tk-paged`, and the host's own CSS
  hides its conversation column and session drawer under it.
- `opts.storagePrefix`: browser-storage key prefix for display state:
  `"tracker-mockup."` in the mockup, `"ws.tracker."` in the product.

Ids the renderer creates carry a `tk` prefix (`tkdrawer`, `tkclip`, `tkbody`,
`tkhandles`, `tkpage`, `tkwin`, `tkdlg`, `tktoast`, `tktip`, `tkqs`, ...), so
they never collide with the dashboard's `drawer`, `drbody`, `gear`, `convo`.
Content ids inside cards may stay as they are when they do not collide.

The store handles (and Phase 2's "+ store" handle) sit in the renderer's own
handle row, absolutely positioned at the top right and offset left by the
host's handle width: `right: calc(var(--dpad) + var(--drhw, 0px) + Npx)`,
where `--drhw` is what the host sets for its own handle (the product's
session-panel handle already sets it; the mockup sets it for its panels
stub). The renderer sets `--tkhw` to its row's width; the host header pads
clear of both. Order, right to left: the host's handle, the store handles in
`stores()` order, then "+ store".

The tracker drawer and the host's drawer never show at once: when a store's
drawer opens, the renderer dispatches `document` event `tk:drawer-open`; the
host closes its own. When the host opens its drawer it dispatches
`ws:drawer-open`; the renderer collapses an open store (a collapse, so it
syncs).

### 2.3 Display state (Decision 5)

Per-card view modes (the Items panel's mode per scope and the side cards'
modes), the store drawer height per store, and the recently-closed fold per
store persist in browser storage under `opts.storagePrefix`; everything else
in `ui` stays page-lifetime. Storage access is guarded (a locked-down browser
throws). The mockup's old `tracker-mockup-drawer-h` key may be dropped.

### 2.4 api

`refresh()`, `openStore(id)` (toggle, as a handle click), `closeStore()`,
`openItemWindow(storeId, itemId, opts)` (also kept as
`window.openItemWindow`), `viewerChanged(id)` (the mockup's viewer switch:
marks seen when the store is on screen, clears the last-write line,
re-renders), `toast(html)`. `window.__tracker` (the read-only hook scripted
checks use) stays.

## 3. Strings (Phase 1b)

Every user-visible string the renderer emits goes through `wsT("js.tk.<area>.<name>", args...)`
or `wsTn(...)` (count forms `.one`/`.other`), with `{0}`-style placeholders.
The key is a double-quoted literal at the call site, lowercase letters,
digits, and dots only (`internal/wsi18n/keys_test.go` finds keys with
`"(js\.[a-z0-9.]+)"`; a key built at run time is invisible to it and is not
allowed). The English text goes into `internal/wsi18n/tables/en.json`; other
tables fall back to English and need not be filled. A placeholder may carry
already-escaped HTML; table text is ours and inserted as HTML. Board-reference
numbers such as "(354)" in comments are not strings. Item, comment, and
person text never passes through `wsT`.

The dashboard page already defines `wsT`/`wsTn` (`wsi18n.Script`). The
mockup's `i18n-en.js` defines the same two functions over the English
`js.tk.*` subset, generated from `en.json`; a Go test in
`internal/wsdashboard` regenerates it in memory and fails on drift (with an
environment switch that rewrites the file), and skips when the mockup
directory is absent.

## 4. Daemon side (Phase 2)

### 4.1 wstrackerhost

- Seen state: `<root>/<store id>/ui-seen.json`, beside `host-sync.json`,
  outside git: `{"version": 1, "badge": [...], "read": [...]}`, sorted and
  unique. `Host.Seen(id)` and `Host.MarkSeen(id, add)` (a union under a lock,
  written atomically with the existing `writeJSON`). An unregistered id is an
  error.
- `Host.DispatchBatch(ctx, id, events)` over `DispatchWith`: resolves `"$N"`
  in the `item`, `parent`, `comment`, and `reply_to` fields inside the build
  (N must name an earlier event of the batch; otherwise an error and nothing
  written), so the ids come from the store's next id under the write lock,
  never from the browser.
- `StartPoll(interval, onRound func())`: `onRound` (may be nil) runs after
  each `PollOnce` round the process ran.
- `RedactCredentials(s)` and `(*StoreView).Redact()` move here from
  `internal/mcp/tracker_tools.go` (`redactRemoteCredentials`,
  `redactStoreView`); `internal/mcp` calls them.

### 4.2 wsdashboard (stays tracker-free: no wstrackerhost, wstracker, or wsledger import)

`DaemonOptions.Tracker TrackerHost`, an interface `wsdashboard` defines with
its own JSON-level types (`Stores`, `View` returning the view JSON plus the
seen marks, `Dispatch`, `Check`, `Sync`, `MarkSeen`, `Profiles`,
`SaveProfile`, `Register`); every string it returns is already redacted.
`(*Daemon).TrackerChanged()` bumps a revision counter and wakes the broker.

Routes, mounted when `Tracker` is non-nil (all behind the existing access
guard; every POST also requires `sameOrigin`, as the inbox routes do; JSON in
and out; bodies size-limited; errors `{"error": ...}`):

| route | answer |
|---|---|
| `GET /tracker/stores` | `{"stores": [{"id", "label", "member", "remote"}], "rev"}`; a host that could not be created answers 503 with its error |
| `GET /tracker/stores/<id>/view` | `{"view": <StoreView>, "seen": {"badge", "read"}}`; unknown store 404 |
| `POST /tracker/stores/<id>/dispatch` | body `{"events": [{"type", "payload", "origin"}]}`; origin is forced to `human`; 200 `{"ids": [...]}`, a refusal 409 `{"error", "refusal": {"index", "type", "reason"}}` |
| `POST /tracker/stores/<id>/check`, `.../sync` | 200 with the report (an unreachable remote is a report with `error`, the offline path, not an HTTP error) |
| `POST /tracker/stores/<id>/seen` | body `{"badge", "read"}`; 204 |
| `GET /tracker/profiles`, `POST /tracker/profiles` | the saved list; save one |
| `POST /tracker/register` | body `{"id", "label", "remote", "member", "profile"}`; 200 `{"store", "creator", "profile"}` |

The SSE nudge rides the session page's existing stream (a browser holds six
connections per origin across tabs, so no second stream): an
`event: tracker` with data `{"rev": N}` whenever the revision differs from
what that stream last sent, including once on connect. The revision bumps on
the daemon's poll round, after every check, sync, dispatch, register,
mark-seen, and profile save the endpoints ran.

### 4.3 cmd/ws-mcp

`dashboardCommand` creates the one `wstrackerhost.Host` before `StartDaemon`
and passes an adapter implementing `wsdashboard.TrackerHost` (in package
`main`, beside `dashboard.go`), then starts the poll with
`daemon.TrackerChanged` as `onRound`. When `wstrackerhost.New` fails the
adapter still goes in and every method returns that error, so the page's
"+ store" dialog shows it. Register through the endpoint uses origin
`human` and a timeout like `trackerRegisterTimeout`.

### 4.4 Product page

`pages.go` embeds `tracker.js`/`tracker.css` and the session template inlines
them; a section of `dashboard.js` is the HTTP adapter (fetch, the `tracker`
SSE event, the view cache) and mounts the renderer with `storagePrefix`
`"ws.tracker."`. The session-panel handle's `ws.dashboard.drawer` /
`ws.dashboard.drawer.h` keys are untouched. The "+ store" handle shows on
every session page, with zero stores and when the host failed (its dialog
shows the error).
