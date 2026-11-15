---
title: "Explorer allowed-root switcher: project and its worktrees"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "prerequisite: the switcher lives in the card core's navigation row (Phase 1)"
  261115-feat-ws-dashboard-explorer-path-bar: "sibling: same navigation row; the path bar addresses paths inside the root this switcher selects"
  261114-feat-ws-dashboard-worktree-view: "consumer context: the worktree page also roots an explorer at one worktree"
---

# Explorer allowed-root switcher: project and its worktrees

## Background

The approved explorer epic includes worktree/root switching without widening
guarded roots. The approved mockup
([mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html),
frame 06 and the shared navigation row) shows an "allowed root" select that
offers the session's project root and its worktrees, with a lock badge, ahead
of the back/forward/up buttons and the path input.

Today (evidence gathered 2026-11-15):

- An explorer instance browses `s.Project` for any admitted viewer. A
  `root=<abs>` parameter on the session-scoped `/fs/*` routes is accepted only
  for a local viewer (`localViewer`: loopback peer and loopback Host) that is
  not a cross-site fetch, and `browseRootParam` accepts any absolute
  plain-volume directory with no allow-list (`fs_routes.go`, `browseRoot` and
  `browseRootParam`; `access.go`, `localViewer`).
- files.js already has a local-viewer-only free-text "change root" popover
  (`applyRoot`, `ui.rIn`); the changed root persists per session as
  `ws.dashboard.files.root.<rid>`.
- The daemon already enumerates a repository's worktrees for the rail through
  `RepoWorktreeSource.ReadRepoWorktrees` (`railworktrees.go`, implemented in
  `mcp/dashboard_worktrees.go` via `git worktree list`; primary worktree
  first, cached by `railLister` and refreshed while a rail stream is open).
  The rail renders it server-side into HTML; no JSON route gives the explorer
  this list. A session record carries only `Project` and `GitCommonDir`.

## Decisions

- **Local viewers only.** The switcher is offered exactly where `root=`
  already works (a local viewer, not a cross-site fetch); remote (LAN) viewers
  keep seeing the session root alone. Source: the epic authorizes
  worktree/root switching "without widening guarded roots", and `browseRoot`
  refuses `root=` to non-local viewers today.
- **The free-text change-root popover stays** beside the new select: the
  select is the quick path for the allowed roots, the popover remains the
  local viewer's arbitrary-folder path. Rejected: replacing the popover -
  that would delete functionality.
- **The select is fed by an additive session-scoped JSON route** returning the
  session root plus the repository's worktrees (path, branch, detached) from
  the existing `RepoWorktreeSource`, refused like `root=` to non-local
  viewers. Compatibility: T0 (additive endpoint). Rejected: reading the
  rail's server-rendered DOM - the rail lives in another frame and its cache
  refreshes only while a rail stream is open.
- Settled 2026-11-15 by MAIN acknowledgement of the announced and technical
  calls (no owner policy question).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Read-only; no filesystem mutation, no new symlink permissions, normal Files
  root guards unchanged (epic Cross-Child Decisions).
- Implementation waits for the card ticket's Phase 1 and for MAIN's start
  signal after `261114-refactor-ws-dashboard-whiteboard-asset-split`.

## Phases

### Phase 1: Allowed-root select in the explorer navigation row

Depends on the card ticket's Phase 1 (navigation row, inner tabs, per-card
state).

- Add the session-scoped worktree route described in `## Decisions`; it
  answers the session root first, then the other worktrees of the session's
  repository, each with path, branch and detached flag, and refuses non-local
  viewers and cross-site fetches with the same 404 as `root=`.
- In the navigation row, a select (labeled project/worktree and branch, with
  the mockup's lock badge) switches the active inner tab's root through the
  existing `root=` mechanism; the choice persists in per-card state. A stored
  root the server now refuses (a removed worktree) falls back to the session
  root with the existing "root dropped" notice.
- Remote viewers see no select; the existing change-root popover stays.

Verification: Go tests for the route (session root first, worktree fields,
refusal for remote viewers and cross-site fetches, unknown session 404);
browser checks that switching roots in one card leaves another card and the
Files tab unchanged, that a removed worktree falls back to the session root,
and that a remote viewer sees no select; `TestFS|TestFiles` neighbours and
`./internal/wsi18n` for new strings; side-by-side against the navigation row
of frame 06 (dual pane and column view excluded).
