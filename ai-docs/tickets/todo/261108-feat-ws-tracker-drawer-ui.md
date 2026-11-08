---
title: Tracker drawer UI - per-store handles, shared renderer, daemon endpoints, local UI state, join dialog
parent: 261107-epic-ws-personal-common-store
related:
  261108-feat-ws-tracker-caller: prerequisite (registry, sync entry points, wrappers)
  261108-feat-ws-dashboard-disk-assets-dev-mode: prerequisite (disk-assets dev mode, Decision 4)
  261107-feat-ws-dashboard-pull-down-drawer: the drawer frame this extends (done)
  261108-feat-ws-tracker-support-agent-chat: blocked by this ticket (its card lives in the drawer)
---

# Tracker drawer UI - per-store handles, shared renderer, daemon endpoints, local UI state, join dialog

## Background

The dashboard side of the tracker caller, split from
`261108-feat-ws-tracker-caller` by board (408) b ("the UI port is large and
reviewable on its own, and needs the core first"). The mockup
(`ai-docs/ref/tracker-mockup/`; README layout section and `SCENARIOS.md`;
latest merges `2db82f6c2` member profile and `23b2eb7ff` detail pass) is the
UI contract: it already renders the view-state JSON `wstracker.View` emits.

Evidence gathered 2026-11-08:

- The shipped drawer (`internal/wsdashboard/assets/dashboard.js`
  `drawerTab`, around L1137) is one handle whose entries are the four session
  panels.
- Dashboard and whiteboard assets are compiled in through `embed.FS`
  (`wsdashboard/pages.go:32`, `wswhiteboard/render.go:28`): any asset change
  needs `./install.sh update` and a daemon restart; there is no disk-assets
  mode.
- The mockup keeps local UI state (read marks, seen marks, view modes, drawer
  height, recently-closed fold, saved profiles) in localStorage as a
  simulation only.

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
5. **Local UI state on disk** ((397)): notification read state, handle-badge
   seen marks, view modes, drawer height, and the recently-closed fold are
   stored per clone on the local filesystem by the daemon, never in browser
   storage. The daemon owns it (`261108-feat-ws-tracker-caller` Decision 7).
6. **Join dialog** ((411) c, (393), (400)): the dashboard offers register and
   join through the mockup's dialog, backed by the core's machine-local saved
   profiles (default: last used; pick or edit, an edit saves a new entry).
7. **Sync entry points** ((396)): opening a store's drawer triggers the
   core's immediate check; collapsing the panel triggers its push.

## Constraints

- Manuals that bind edits here: `ai-docs/manuals/shipped-surface-boundary.md`
  (all of `agents-plugin-tool/`); `ai-docs/manuals/ws-mcp.md` when touching
  `internal/mcp/`. UI strings go through the i18n tables
  (`internal/wsi18n/tables/*.json`, coverage checked by
  `internal/wsdashboard/i18n_test.go`).
- The mockup stays runnable from `file://` with the fixtures after the
  renderer moves.
- Open known nit: two-character Hangul avatars (`.av`, 9px in a 20px circle)
  are tight; tune with the dev mode.
- Out of scope: the support-agent chat card
  (`261108-feat-ws-tracker-support-agent-chat`); attachment blob loading
  (`261108-feat-ws-tracker-ledger-attachments`).

## Phases

### Phase 1: Shared renderer

- Move the mockup's drawer rendering into a dashboard asset with a data
  adapter seam (Decision 3); the mockup loads it from the shipped asset path
  and keeps its engine-and-fixtures adapter.
- Verification: `node --check` on the mockup scripts; the mockup's SCENARIOS
  walk and a headless Chromium error sweep (900, 1280, 1760px; every viewer,
  scope, view mode) unchanged; `go test` for `wsdashboard`.

### Phase 2: Daemon wiring, per-store handles, local UI state, join dialog

- Daemon JSON view and write endpoints over the core's stores and an SSE
  change nudge (Decision 2); the HTTP data adapter for the shared renderer.
- Per-store drawer handles (Decision 1); sync entry points (Decision 7).
- Local UI state on disk (Decision 5), replacing localStorage in the daemon
  adapter.
- The register / join dialog (Decision 6).
- Verification: endpoint and SSE tests; headless Chromium against a daemon
  with two local-bare-remote stores (two handles, a write from the drawer
  visible after sync in the other clone, local UI state surviving a reload
  and a daemon restart, the join dialog defaulting to the last profile);
  light and dark screenshots.
