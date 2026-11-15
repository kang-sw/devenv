---
title: "Explorer allowed-root switcher: project and its worktrees"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "prerequisite: the switcher lives in the card core's navigation row (Phase 1)"
  261115-feat-ws-dashboard-explorer-path-bar: "sibling: same navigation row; the path bar addresses paths inside the root this switcher selects"
  261114-feat-ws-dashboard-worktree-view: "consumer context: the worktree page also roots an explorer at one worktree"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a15b8d8ffc7bbb47
sage-review-completeness-reviewed: a15b8d8ffc7bbb47
completed: 2026-11-15
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
  (`applyRoot`, `ui.rIn`); the legacy root record is
  `ws.dashboard.files.root.<rid>`, while inner-tab roots persist in
  `ws.dashboard.files.tabs.<rid>` through each owner's storage adapter;
  explorer cards wrap those records in `ws.dashboard.explorer.<rid>.<card-id>`
  (`agents-plugin-tool/internal/wsdashboard/assets/files.js#L138-L145`,
  `#L1182-L1189`, `#L1240-L1249` at `1ecde93d4`).
- The daemon already enumerates a repository's worktrees for the rail through
  `RepoWorktreeSource.ReadRepoWorktrees` (`railworktrees.go`, implemented in
  `mcp/dashboard_worktrees.go` via `git worktree list`; primary worktree
  first, cached by `railLister` and refreshed while a rail stream is open).
  The rail renders it server-side into HTML; no JSON route gives the explorer
  this list. A session's registration carries `Project` and `GitCommonDir`
  for repository identity, not an enumerated worktree list
  (`agents-plugin-tool/internal/wsdashboard/registry.go#L76-L90`).
- The compact layout (Variant A, owner choice (908)=a, branch
  `exec5/explorer-compact-a`) replaces the navigation row relative to the
  pinned base `1ecde93d4`: the breadcrumb starts with a `<root>▾` root segment
  (`ui.rootBtn`), enabled only for a local viewer, that opens the change-root
  popover; there is no separate select slot or lock badge in the row
  (`git show exec5/explorer-compact-a:agents-plugin-tool/internal/wsdashboard/assets/files.js`,
  lines 298-315, 945-960 and 1034-1067). The up button precedes that root
  segment; `ui.crumbs` holds the directory trail. Variant A is absent from
  the pinned base; source inspection also observed its subsequent develop
  merge `6d3cc2c96`.

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
- **Reconciled with Variant A: the allowed roots are listed inside the
  popover the `<root>▾` root segment opens**, as one-click rows above the
  free-text input, the current root marked. Rejected: a separate select with
  a lock badge in the navigation row - the approved Variant A row has no slot
  for it and its root segment already is the root control. Source: owner
  (908)=a; reconciliation granted by MAIN 2026-11-15.
- Settled 2026-11-15 by MAIN acknowledgement of the announced and technical
  calls (no owner policy question).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Read-only; no filesystem mutation, no new symlink permissions, normal Files
  root guards unchanged (epic Cross-Child Decisions).
- Builds on the Variant A compact layout (`exec5/explorer-compact-a`); MAIN
  granted the start on 2026-11-15.
- UI follows the owner UI principle: implement, run the relevant tests, and
  hand the head to the owner's own visual check; no screenshot rounds. The
  route's access rules keep their Go tests.

## Prior Decisions

- 0be5997a (2026-11-15, commit): "Keep the shared explorer core responsible for all three surfaces; preserve existing action routing, tab/search isolation, and attachment-slot behavior." — bearing: constrains
- 1f7363b4 (2026-11-15, commit): "A separate rootAttempt sequence now supersedes only candidate responses" — bearing: constrains
- 261115-feat-ws-dashboard-local-browse-anchor (2026-11-15, commit 02b75478): "Real-session lookup, root/capability rules, terminal ownership trust, and even the legacy empty worktree JSON field remain unchanged" — bearing: constrains
- 261115-feat-ws-dashboard-explorer-path-bar (2026-11-15, Decisions): "The path input addresses folders inside the current root; a path outside it shows the existing refusal and never switches roots." — bearing: constrains
- 86042116 (2026-11-15, commit): "keep free-text root popover, additive session-scoped worktree JSON route (T0), path bar inside root only, in-memory per-tab history, scoped Ctrl/Cmd+L" — bearing: supports
- 02a2e572 (2026-11-15, commit): "Trimmed acceptance per (899)=a and the owner UI-fast principle: scoped tests, wsi18n, one lite (clean); owner on-screen check is acceptance." — bearing: supports
- 7f890500 (2026-11-10, commit): "Change root is a `root` query parameter gated by localViewer and crossSiteFetch under the files route's abs-mode path rules; no capability is minted there" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/files.js, fs_routes.go, railworktrees.go, internal/mcp/dashboard_worktrees.go, route/JS tests and internal/wsi18n strings |
| scope.surface | public-interface | Additive session-scoped HTTP JSON endpoint consumed by the shared explorer; existing RepoWorktreeSource interface reused |
| scope.new_public_symbol | yes | New HTTP route; its exact path is an implementation choice, no new exported Go symbol required by the plan |
| scope.new_type_contract | yes | New JSON answer carrying ordered roots with path, branch and detached fields; existing RepoWorktree fields have no JSON tags |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/fs_routes_test.go TestFSChangeRootIsLoopbackOnly; files_test.go TestFilesExplorerInstances runs testdata/files-explorer.test.cjs; internal/wsi18n exists |
| complexity.reuse_points | confirmed | Read browseRoot/browseRootParam, RepoWorktreeSource and railLister, mcp readRepoWorktrees, files.js applyRoot/useRoot/loadRoot and card storage adapters; inspected Variant A ui.rootBtn/ui.crumbs/ui.up via git show |
| complexity.side_effect_risk | moderate | Shared root popover and asynchronous root changes affect cards, Files tab and toolbar; enumeration must not depend on a rail stream being open |
| risk.correctness | moderate | Session-first ordering and duplicate roots, stale worktree directories, candidate-response races and per-tab/card isolation require coverage |
| risk.fit | moderate | Reuses the existing source and root control, but must compose the pinned base with Variant A and the sibling path-bar changes in the same navigation area |
| risk.test | moderate | Existing Go and production-JS fixtures support guard/isolation checks; new route answers and popover interactions need assertions, while visual acceptance remains owner-owned |
| risk.security_or_contract | high | New endpoint discloses absolute worktree paths and branches; localViewer and crossSiteFetch refusal must precede enumeration and preserve existing root/capability semantics (T0) |

## Phases

### Phase 1: Allowed-root select in the explorer navigation row

Depends on the card ticket's Phase 1 (navigation row, inner tabs, per-card
state).

- Add the session-scoped worktree route described in `## Decisions`; it
  answers the session root first, then the other worktrees of the session's
  repository, each with path, branch and detached flag, and refuses non-local
  viewers and cross-site fetches with the same 404 as `root=`.
- The popover the `<root>▾` root segment opens lists the route's roots
  (folder name and branch, detached marked, current root marked) above the
  existing free-text input; choosing one switches the active inner tab's root
  through the existing `root=` mechanism, and the choice persists in per-card
  state. A stored
  root the server now refuses (for example, a removed worktree directory)
  falls back to the session root with the existing "root dropped" notice;
  merely unregistering a still-existing directory as a worktree does not
  revoke `root=` access (`agents-plugin-tool/internal/wsdashboard/fs_routes.go#L157-L178`,
  `assets/files.js#L351-L370` at `1ecde93d4`).
- Remote viewers keep the disabled root segment and see no list; the
  free-text input stays.

Verification: Go tests for the route (session root first, worktree fields,
refusal for remote viewers and cross-site fetches, unknown session 404);
node/browser-harness checks that switching roots in one card leaves another
card and the Files tab unchanged, that a removed worktree directory falls
back to the session root (the existing directory-validity guard, not Git
membership, decides refusal), and that a remote viewer gets no list; `TestFS|TestFiles`
neighbours and `./internal/wsi18n` for new strings. Look and placement go to
the owner's visual check (owner UI principle) instead of a side-by-side
capture.

### Result (cd250bbca) - 2026-11-15

Implemented the additive T0 `GET /s/<reply-id>/fs/roots` route and one-click
root suggestions above the existing free-text input. The session root comes
first with its branch metadata when available, followed by deduplicated,
valid, non-prunable worktree directories. Reads use `RepoWorktreeSource`
directly, independent of an open rail stream; an absent or failed source
still returns the session root. Non-local and cross-site viewers are refused
before enumeration. The host0 discovery answer and existing root/capability
guards are unchanged.

The shared popover shows folder, branch/detached state, and the current root;
selection uses `applyRoot` and existing per-tab/card persistence. Request
tokens reject stale popover answers, and current marking follows initial
loads and saved-root fallback. Merely unlisted directories remain browsable;
a removed saved directory falls back with the existing notice. Existing
classes and localized labels were reused, with no CSS or breadcrumb/up/back/
forward edits, per the lead's scope boundary and owner (918).

Verification (all passed with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`):

- `go test ./internal/wsdashboard -run 'TestFS|TestFiles|TestExplorer' -count=1`
- `go test -race ./internal/wsdashboard -run 'TestFSRoots|TestFSChangeRootIsLoopbackOnly|TestFilesExplorerInstances|TestFilesExplorerCards' -count=1`
- `go test ./internal/wsi18n -count=1`
- `node --check agents-plugin-tool/internal/wsdashboard/assets/files.js` and
  `git diff --check` (from the repository root).

Go tests cover ordering/fields, duplicates and invalid/prunable directories,
source fallback, refusal before enumeration, unknown sessions, and removed
versus unlisted directory validity. Production-JS fixtures cover late
responses, current-root races, remote suppression, free-text retention,
saved-root fallback, per-card persistence, and isolation from other cards,
inner tabs, and the Files tab.

A brief correctness/security/test self-review replaced independent review
because the lead disabled spawning; no independent-review pass is claimed.
The owner's on-screen check remains visual acceptance. No browser/screenshot
rounds, daemon rebuild/restart, process kills, push, or merge were performed.
