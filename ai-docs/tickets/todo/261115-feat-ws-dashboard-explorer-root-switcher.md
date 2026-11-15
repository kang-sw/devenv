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

Pending the Open Decision Queue below.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Read-only; no filesystem mutation, no new symlink permissions, normal Files
  root guards unchanged (epic Cross-Child Decisions).
- Implementation waits for the card ticket's Phase 1 and for MAIN's start
  signal after `261114-refactor-ws-dashboard-whiteboard-asset-split`.

## Open Decision Queue

Last ID: (10) (working labels; owner-visible questions go to MAIN unnumbered)

(08) [open] [announced] The switcher is offered to local viewers only, exactly where `root=` already works; remote (LAN) viewers keep seeing the session root alone - the epic authorizes worktree/root switching "without widening guarded roots", and `browseRoot` refuses `root=` to non-local viewers today.
(09) [open] [announced] Keep the existing free-text change-root popover beside the new select - removing it would delete functionality (house rule: no deleting functionality); the select is the quick path for the allowed roots, the popover stays the local viewer's arbitrary-folder path.
(10) [open] [technical] Feed the select from an additive session-scoped JSON route that returns the session root plus the repository's worktrees (path, branch, detached) from the existing `RepoWorktreeSource`, over reading the rail's server-rendered DOM - the rail lives in another frame and its cache refreshes only while a rail stream is open, while a route reuses the same source with no new dependency (T0 additive endpoint).

## Phases

### Phase 1: Allowed-root select in the explorer navigation row

Drafted after the queue settles: a select listing the session's project root
and its repository's worktrees (labeled project/worktree and branch), shown
only to local viewers, switching the active inner tab's root through the
existing `root=` mechanism and per-card state; the free-text popover remains.
Verification will include refusal for remote viewers, a stale worktree that
no longer exists falling back to the session root, per-card independence, and
a side-by-side against the navigation row of frame 06 (dual pane and column
view excluded).
