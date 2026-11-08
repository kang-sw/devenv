---
title: Tracker drawer UI - per-store handles, shared renderer, daemon endpoints, local UI state, join dialog
parent: 261107-epic-ws-personal-common-store
related:
  261108-feat-ws-tracker-caller: prerequisite (registry, sync entry points, wrappers)
  261108-feat-ws-dashboard-disk-assets-dev-mode: prerequisite (disk-assets dev mode, Decision 4)
  261107-feat-ws-dashboard-pull-down-drawer: the drawer frame this extends (done)
  261108-feat-ws-tracker-support-agent-chat: blocked by this ticket (its card lives in the drawer)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a7394381ca3a5da1
sage-review-completeness-reviewed: a7394381ca3a5da1
---

# Tracker drawer UI - per-store handles, shared renderer, daemon endpoints, local UI state, join dialog

## Background

The dashboard side of the tracker caller, split from
`261108-feat-ws-tracker-caller` by board (408) b ("the UI port is large and
reviewable on its own, and needs the core first"). The mockup
(`ai-docs/ref/tracker-mockup/`; README layout section and `SCENARIOS.md`;
merges `2db82f6c2` member profile, `23b2eb7ff` detail pass, and `5b60f20e7` item window pass, the newest) is the
UI contract: it already renders the view-state JSON `wstracker.View` emits.

Evidence gathered 2026-11-08:

- The shipped drawer (`internal/wsdashboard/assets/dashboard.js`
  `drawerTab`, `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L1137`) is one handle whose entries are the four session
  panels.
- Dashboard and whiteboard assets are compiled in through `embed.FS`
  (`wsdashboard/pages.go#L33-L34`, `wswhiteboard/render.go#L29-L30`): by
  default an asset change needs `./install.sh update` and a daemon restart. The
  disk-assets dev mode has since landed (`WS_MCP_DASHBOARD_DEV_SOURCE`,
  `agents-plugin-tool/internal/wsdashboard/devassets.go#L15`; merge
  `393650fea`), so Decision 4's prerequisite is met.
- The mockup keeps local UI state (read marks, seen marks, view modes, drawer
  height, recently-closed fold, saved profiles) in localStorage as a
  simulation only; Decision 5 splits it for the product.

## Decisions

All confirmed on the session board, 2026-11-08.

1. **Per-store handles** ((398)): each store gets its own drawer handle,
   stacked leftward from the top-right corner, extending the shipped
   single-handle drawer frame.
2. **Port, not rewrite** ((405)): the dashboard UI ports the mockup's `app.js`
   rendering into dashboard assets, driven by daemon JSON view and write
   endpoints and an SSE change nudge. Rejected: rewriting it as Go templates -
   the mockup already renders the exact view-state JSON.
3. **One renderer, two data adapters** ((412) c, part a): the drawer renderer
   lives once in the shipped assets; the mockup page loads that same file and
   feeds it from `engine.js` and the fixtures, the daemon feeds it over HTTP.
   The mockup and the product cannot drift, and the fixture loop stays.
4. **Disk-assets dev mode** ((412) c, part b): provided by
   `261108-feat-ws-dashboard-disk-assets-dev-mode`, split out by board (429) a
   to run ahead of this ticket and to cover the whiteboard assets too. This
   ticket's UI tuning uses it; it builds no dev mode of its own.
5. **Local UI state split by kind** ((397), revised by (451) a; (449) a):
   what the viewer has seen - notification read marks, handle-badge seen
   marks - is stored per clone on the local filesystem by the daemon, never
   in browser storage, so the same notification is not new again on another
   device opening the dashboard over LAN; the daemon owns it
   (`261108-feat-ws-tracker-caller` Decision 7). Saved join profiles stay
   the core's machine-local `profiles.json` (Decision 6). How things are
   displayed - per-card view modes, the store panel height, the
   recently-closed fold - stays in browser storage beside the shipped layout
   keys, because display settings naturally differ per device and screen
   size. The shipped session-panel handle's open state and height
   (`DR_OPEN` / `DR_H`) stay browser-local and unchanged. Rejected: keeping
   every tracker UI state on disk as first drafted (the (340) reason - one
   person, several devices - fits seen-state, not screen-size-dependent
   display); moving the session-panel handle's state to disk too (outside
   this ticket's tracker scope; can follow if the split feels odd).
6. **Register / join dialog** ((411) c, (393), (400), (452) b): every
   dashboard shows a small "+ store" handle at the end of the tracker handle
   stack, also when no store is registered, so the dashboard is a first-run
   surface as well as the way to add another store. It opens one dialog that
   creates a store (blank remote, or an empty repository) or joins the store
   a remote holds, with the `wstrackerhost.RegisterRequest` fields (local
   store id, label, remote - may be blank, member id, member profile) and no
   lead field (a self-join cannot set relations). The
   member profile comes from the core's machine-local saved profiles
   (default: last used; pick or edit, an edit saves a new entry). The
   mockup's join dialog, which simulates a member joining the store already
   open, is the visual starting point only. Showing the handle needs no
   tracker knob: `workflow.tracker_enabled` gates only manual text, and the
   epic's Exposure decision governs the MCP and skill surface, not this
   dashboard control. Rejected: the handle only while a store exists, with
   the first store registered through `tk:register` from an agent (no
   first-run path in the dashboard); showing it only when the knob resolves
   on (the daemon would read the knob, against `12db25a1`).
7. **Sync entry points** ((396)): opening a store's drawer triggers the
   core's immediate check; collapsing the panel triggers its push.
8. **Core entry points** ((447)): drawer writes go through `Host.Dispatch` /
   `Host.DispatchWith` so the store write lock covers them; drawer open calls
   `Host.Check`, panel collapse calls `Host.Sync`; `StoreView` is the JSON
   the shared renderer reads (caller Forward notes `a6f97847`, `5b9ee61ea`).
9. **One Host per daemon** ((448)): one `wstrackerhost.Host`, created in
   `cmd/ws-mcp` and handed to `wsdashboard` through a small interface
   `wsdashboard` defines, shared by the poll and the endpoints. Rejected:
   `wsdashboard` importing `wstrackerhost` (it stays tracker-free,
   `12db25a1`); a second Host in the daemon (two write-lock holders in one
   process).
10. **Browser acceptance is ad hoc** ((450)): the headless Chromium checks
    run as an ad hoc Playwright script on the cached Playwright install
    (`~/.npm/_npx/*/node_modules/playwright`), screenshots under `/tmp`,
    steps recorded in the phase Result. Rejected: a committed browser
    harness - the Go test path has no Node toolchain, and earlier dashboard
    tickets verified the same way.

## Constraints

- Manuals that bind edits here: `ai-docs/manuals/shipped-surface-boundary.md`
  (all of `agents-plugin-tool/`); `ai-docs/manuals/ws-mcp.md` when touching
  `internal/mcp/`. UI strings go through the i18n tables
  (`internal/wsi18n/tables/*.json`, key coverage checked by
  `internal/wsi18n/keys_test.go`; `internal/wsdashboard/i18n_test.go` covers
  page language behavior).
- The mockup stays runnable from `file://` with the fixtures after the
  renderer moves.
- Open known nit: two-character Hangul avatars (`.av`, 9px in a 20px circle)
  are tight; tune with the dev mode.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Out of scope: the support-agent chat card
  (`261108-feat-ws-tracker-support-agent-chat`); attachment blob loading
  (`261108-feat-ws-tracker-ledger-attachments`).

## Prior Decisions

- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions; view modes revised by (451) a, epic updated in the same commit): "Local UI state (user, board (332) a, (340)): per-viewer UI state such as notification read state, the handle badge's seen marks, and each card's chosen view mode lives per clone on the local filesystem outside git" — bearing: constrains
- 261108-feat-ws-tracker-caller (2026-11-08, Decisions): "Timer ownership ((404)): when the dashboard daemon runs it owns the poll timer (and, in the drawer ticket, the local UI state); MCP processes open stores on demand under wsledger's lock" — bearing: constrains
- a6f97847 (2026-11-08, commit): "Forward: the drawer ticket's daemon endpoints call Host.Check on drawer open and Host.Sync on panel collapse; StoreView is the JSON the shared renderer reads." — bearing: constrains
- 5b9ee61ea (2026-11-08, commit): "Forward: the drawer ticket's writes should go through Host.DispatchWith/Dispatch so the store write lock covers them." — bearing: constrains
- 12db25a1 (2026-11-08, commit): "The poll lives in cmd/ws-mcp next to StartDaemon, so wsdashboard stays tracker-free; the daemon never reads the knob." — bearing: constrains
- 261107-feat-ws-dashboard-pull-down-drawer (2026-11-07, Phase 1 commit e12879e1): "Forward: the personal common store's page tabs register through drawerTab({id, title, icon, count, badge, content}) in dashboard.js." — bearing: supports
- 261108-feat-ws-dashboard-disk-assets-dev-mode (2026-11-08, Decisions): "drawer-ui follows ((431)): `261108-feat-ws-tracker-drawer-ui` Decision 4 now points here, its Phase 1 no longer carries the dev mode" — bearing: supports
- f37503d1 (2026-11-08, commit): "User decision (board (382) a): the drawer is a hand-adjusted height panel ... fitDrawer now yields to a saved height." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | dashboard.js, dashboard.css, dashboard.tmpl, new shared renderer asset, daemon endpoint Go files in internal/wsdashboard, cmd/ws-mcp/dashboard.go, ai-docs/ref/tracker-mockup/*, internal/wsi18n/tables/*.json |
| scope.surface | cross-module | wsdashboard endpoints over wstrackerhost.Host (Check, Sync, View, Dispatch, Profiles) plus the mockup page loading a shipped asset |
| scope.new_public_symbol | yes | new daemon HTTP endpoints and SSE event, and likely an exported wsdashboard hook for the tracker host; names not fixed by the ticket |
| scope.new_type_contract | yes | adapter seam between the shared renderer and its two data sources; local UI state file format; none named yet |
| scope.test_surface | new-files | existing internal/wsdashboard/*_test.go and wstrackerhost/host_test.go; endpoint, SSE, and local-state tests are new, plus a headless Chromium run |
| complexity.reuse_points | confirmed | drawerTab in dashboard.js#L1137, wstrackerhost.Host (Dispatch, DispatchWith, Check, Sync, View, Profiles, SaveProfile), mockup app.js and engine.js |
| complexity.side_effect_risk | high | moves the mockup renderer to a shipped asset while it must stay runnable from file://, and adds drawer writes to stores that MCP processes also write |
| risk.correctness | high | cross-process store writes must go through Dispatch or DispatchWith for the write lock; sync triggers on open and collapse; state surviving daemon restart |
| risk.fit | high | the 2299-line mockup app.js ports into a 1532-line dashboard.js frame; the daemon host is created in cmd/ws-mcp/dashboard.go, outside wsdashboard, which is meant to stay tracker-free |
| risk.test | high | headless Chromium acceptance with two stores and SSE has no existing harness in the tree; unit tests cover only endpoints |
| risk.security_or_contract | moderate | new daemon write endpoints reach a git-backed store and join writes a member profile; credential redaction and the daemon's existing local access gating apply |

## Phases

### Phase 1: Shared renderer

- Move the mockup's UI rendering into a dashboard asset with a data adapter
  seam (Decision 3). The contract is the whole mockup UI, not only the
  drawer cards: the `#/<storeId>` store page route (README (371)), the item
  window, and the quick switcher come along; the hash route must coexist with
  the dashboard page (`dashboard.js` uses no `location.hash` today). The
  mockup loads the shipped asset and keeps its engine-and-fixtures adapter.
- The seam carries what the renderer reads from outside itself: the pure
  `TrackerEngine` helpers it calls (`displayName`, `avatar`, `normProfile`,
  `alias`, `memberOf`, `cloneOf`, `refOf`) either move into the shipped asset
  or are supplied by the adapter; a string lookup for UI text (the mockup
  adapter feeds English, the daemon adapter feeds the `wsi18n` tables), so
  both the i18n constraint and the `file://` constraint hold. Mockup-only
  chrome - viewer switcher, clock advance, raw JSON editor, go offline /
  online, reset, the localStorage profile list - stays on the mockup-adapter
  side.
- Verification: `node --check` on the mockup scripts; the mockup's SCENARIOS
  walk and a headless Chromium error sweep (900, 1280, 1760px; every viewer,
  scope, view mode) unchanged; `go test` for `wsdashboard`.

### Phase 2: Daemon wiring, per-store handles, local UI state, join dialog

- Daemon JSON view and write endpoints over the core's stores and an SSE
  change nudge (Decision 2), over the one daemon Host and the core entry
  points (Decisions 8, 9); the HTTP data adapter for the shared renderer.
  A write whose events reference ids minted in the same batch goes through
  `Host.DispatchWith` (a server-side builder, or a placeholder-for-next-id
  payload the endpoint resolves inside the build), never browser-computed
  next ids, which would race MCP-process writers on the same clone.
- The daemon creates the Host before `StartDaemon` and passes it in; when
  creation fails, the endpoints report the error and the "+ store" dialog
  shows it rather than the handle vanishing. The poll exposes no callback
  today, so the poll-tick nudge needs a small observer on the poll or a
  `cmd/ws-mcp` loop around `PollOnce`; either fits.
- The SSE nudge fires on the daemon's poll tick and Check / Sync results and
  after daemon-side writes; a write by an MCP process on the same clone
  reaches an open drawer at the next poll or Check.
- Per-store drawer handles (Decision 1); sync entry points (Decision 7).
- Local UI state split by kind (Decision 5): seen-state (read marks, badge
  seen marks) through daemon endpoints to a per-clone file under the tracker
  root beside the store's host state, outside git, owned by
  `wstrackerhost` and reached through Decision 9's interface, replacing
  localStorage in the daemon adapter; display state (view modes, store panel
  height, recently-closed fold) in browser storage under tracker-specific
  keys. The session-panel handle's `DR_OPEN` / `DR_H` are untouched.
- The "+ store" handle and the register / join dialog (Decision 6).
- Verification: endpoint and SSE tests; headless Chromium (Decision 10) against a daemon
  with two local-bare-remote stores (two handles, a write from the drawer
  visible after sync in the other clone, an MCP-process write reaching an
  open drawer after the next poll, drawer open calling Check and panel
  collapse calling Sync, seen-state surviving a reload, a daemon restart,
  and a second browser context, display state surviving a reload in the
  same browser context, the "+ store" handle present with zero stores and
  a store created from it getting its own handle, the join dialog
  defaulting to the last profile and
  an edited profile saving as a new entry); endpoint tests also cover
  credential redaction in responses and the daemon's existing local access
  gating on the new write endpoints; light and dark screenshots.

## Sage Review Round 1 (2026-11-08)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Join dialog has no product entry point or create flow; the mockup dialog is the wrong shape | important | missing |
| 2 | Write endpoint payload and intra-batch event references | minor | autonomous |
| 3 | Renderer depends on engine.js helpers and simulation chrome | minor | autonomous |
| 4 | Store page route, item window, quick switcher not scoped explicitly | minor | autonomous |
| 5 | i18n path for a renderer shared with a file:// mockup | minor | autonomous |
| 6 | SSE nudge trigger sources and seen-state file placement unnamed | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Phase 2 verification omits some committed behaviors | minor |
| 2 | Security note has no matching acceptance check | minor |
| 3 | Board references and duplicated Convention lines add friction | minor |
