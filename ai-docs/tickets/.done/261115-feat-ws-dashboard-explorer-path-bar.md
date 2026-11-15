---
title: "Explorer path bar with back, forward and up"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "prerequisite: the path bar lives in the card core's navigation row and keeps history per inner tab (Phase 1)"
  261115-feat-ws-dashboard-explorer-root-switcher: "sibling: same navigation row; the root switcher picks the root the path bar addresses"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 0b17123e5fe73613
sage-review-design-reviewed: 0b17123e5fe73613
completed: 2026-11-15
---

# Explorer path bar with back, forward and up

## Background

The approved explorer epic includes a path bar. The approved mockup
([mockup.html](../assets/261114-epic-ws-dashboard-file-explorer/mockup.html),
navigation row in frames 01 and 06) shows back, forward and up buttons and an
editable input labeled "path inside the root", focused by clicking it or with
Ctrl/Cmd+L.

Today (evidence gathered 2026-11-15, baseline `1ecde93d4`): files.js has no
navigation history and no editable directory-path input; it has a name/content
search text input (`ui.q`) as well as the change-root text input (`ui.rIn`)
(`agents-plugin-tool/internal/wsdashboard/assets/files.js#L945-L946`,
`#L1001-L1002` at `1ecde93d4`).
`/fs/list` already accepts `dir` relative to the root or absolute inside it
(`containedDir` with `relativeInside`), answers with the absolute directory in
`Abs`, and refuses a path outside the root, a symlink, a missing path or a
non-directory with an indistinguishable 404. `dir=""` addresses the root.

The compact layout (Variant A, owner choice (908)=a, branch
`exec5/explorer-compact-a`) since put a 30px navigation row in place: inner
tabs, an up button, and a breadcrumb trail (`ui.crumbs`) after the
`<root>▾` root segment, each path segment clickable. It has no back/forward
buttons and no editable input.

## Decisions

- **Inside the current root only.** The path input addresses folders inside
  the current root; a path outside it shows the existing refusal and never
  switches roots. Source: the mockup labels the input "path inside the root",
  and the epic forbids widening roots; root changes stay with the root
  switcher and the change-root popover.
- **History lives in memory per inner tab** and is not persisted. Rejected:
  persisting it in per-card browser state - the persisted location already
  restores where the tab was, and a stored history adds a store field and
  stale entries for no visible gain.
- **Ctrl/Cmd+L is scoped to the focused explorer instance.** Rejected: a
  page-wide shortcut - the browser's own Ctrl/Cmd+L must keep working
  elsewhere, and several explorer instances may be on the page.
- **Reconciled with Variant A: the input edits the breadcrumb in place.**
  Clicking the trail's empty space or Ctrl/Cmd+L swaps the trail for an
  input holding the path relative to the root (absolute paths inside the
  root are accepted too); Enter navigates, Esc or blur restores the
  breadcrumb. Back and forward are compact icon buttons placed before
  Variant A's existing up button. Rejected: a permanent input beside the
  breadcrumb - it repeats the path and spends the row space the owner cut in
  (905). Source: owner (908)=a; reconciliation granted by MAIN 2026-11-15.
- Settled 2026-11-15 by MAIN acknowledgement of the announced and technical
  calls (no owner policy question).
- **Compatibility: T0.** Dashboard assets and localization only; no route,
  protocol, shared-store, or persisted-history change.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Read-only and client-side: no route change, no new symlink permission; the
  existing `/fs/list` guards decide what may be listed.
- Builds on the Variant A compact layout (`exec5/explorer-compact-a`); MAIN
  granted the start on 2026-11-15.
- UI follows the owner UI principle: implement, run the relevant tests, and
  hand the head to the owner's own visual check; no screenshot rounds.

## Prior Decisions

- 0be5997a (2026-11-15, commit): "Owner choice (908)=a relayed by MAIN; implement the approved Variant A mockup rather than introduce another layout." — bearing: supports
- 86042116 (2026-11-15, commit): "path bar inside root only, in-memory per-tab history, scoped Ctrl/Cmd+L" — bearing: supports
- 261114-feat-ws-dashboard-reusable-file-explorer-card (2026-11-15, Result (52673cf6d)): "Board persistence restores explorer IDs; each ID owns a separate browser record, removed on close after destroying requests and handlers." — bearing: constrains
- 1f7363b4 (2026-11-15, commit): "incrementing the listing generation before candidate authorization discarded valid pending initial loads/expansions even when that candidate was refused." — bearing: constrains
- 261115-feat-ws-dashboard-local-browse-anchor (2026-11-15, commit): "Real-session lookup, root/capability rules, terminal ownership trust, and even the legacy empty worktree JSON field remain unchanged" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/files.js, assets/dashboard.css (Variant A navigation row), files_test.go and testdata/files-explorer.test.cjs; agents-plugin-tool/internal/wsi18n/tables for new labels. |
| scope.surface | internal | Existing WsFiles.create instance controls and guarded /s/<rid>/fs/list; no new endpoint or exported interface is requested. |
| scope.new_public_symbol | no | Back/forward controls, the editable trail and per-tab history are internal to the existing explorer closure. |
| scope.new_type_contract | no | No route, shared-store or persisted record change; history is live per-tab state only. |
| scope.test_surface | existing | TestFilesExplorerInstances in agents-plugin-tool/internal/wsdashboard/files_test.go runs testdata/files-explorer.test.cjs; TestFSListStaysInsideTheRoot in fs_routes_test.go covers containment; internal/wsi18n validates strings. |
| complexity.reuse_points | confirmed | Read snapshot, switchTab, cleanLocation, navigate, listDir and navAttempt in files.js; Variant A paintHead/build supply ui.rootBtn, ui.crumbs and ui.up; fs_routes.go containedDir/handleFSList supply guarded validation and canonical Dir/Abs. |
| complexity.side_effect_risk | moderate | The same navigation state serves cards, the Files tab and toolbar sheet; tab/root switches and stale asynchronous responses must not record or restore another tab's history. |
| risk.correctness | moderate | cleanLocation currently maps absolute paths and parent segments to the root; typed paths need guarded server validation and canonical relative Dir before accepting history, while refusals keep the listing. |
| risk.fit | moderate | The input replaces Variant A's breadcrumb rather than adding a row; root-switcher sibling shares the same navigation row, and Ctrl/Cmd+L must be instance-scoped. |
| risk.test | moderate | Existing controlled-promise fixture supports instance/tab isolation, but editable-path commit/cancel/blur, history traversal and browser shortcut scoping need coverage and the specified browser checks. |
| risk.security_or_contract | moderate | New user-typed absolute and relative paths must preserve containedDir and root isolation; final symlinks and root escapes remain refused, with no root switch or permission change. |

## Phases

### Phase 1: Path bar, history and up

Depends on the card ticket's Phase 1 (navigation row and inner tabs).

- The breadcrumb trail turns into an editable input (click on the trail's
  empty space, or Ctrl/Cmd+L) holding the active inner tab's directory
  relative to the root; Enter navigates through `/fs/list` with that path,
  Esc or blur restores the breadcrumb. A refused path (outside the root, a symlink, a
  missing folder) shows the existing refusal text and keeps the current
  listing.
- Back and forward buttons before the existing up button follow an in-memory
  history per inner tab; up goes to the parent and is disabled at the root.
- Ctrl/Cmd+L focuses the input while focus is inside that instance.

Verification: browser checks for navigation inside the root, refusal outside
it and through a symlink without losing the listing, independent history in
two inner tabs, up disabled at the root, and Ctrl/Cmd+L scoping with two
instances (the browser shortcut untouched outside them); `TestFiles`
neighbours and `./internal/wsi18n` for new strings. Look and placement go to
the owner's visual check (owner UI principle) instead of a side-by-side
capture.

### Result (e8b1b1107) - 2026-11-15

- Implemented the in-place breadcrumb editor, Enter/Esc/blur handling,
  instance-scoped Ctrl/Cmd+L, and compact back/forward controls before up.
  Reused existing button/input classes with three small layout rules.
- History belongs to each live inner tab, truncates the forward branch after
  new navigation, omits duplicate canonical destinations, resets on accepted
  root changes, and is never serialized. Typed paths and history destinations
  go through the guarded listing route before changing the current directory;
  stale tab/root/navigation responses are ignored.
- Preserved the existing root popover and route permissions. Absolute child
  paths inside the root work; the exact absolute root remains refused by the
  existing strict `relativeInside` guard (use an empty path or `.`). Added a
  route regression test for this boundary rather than changing route behavior.
- Verification passed with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:
  `go test ./internal/wsdashboard -run 'TestFS|TestFiles|TestExplorer' -count=1`
  and `go test ./internal/wsi18n -count=1` from `agents-plugin-tool/`.
  JavaScript syntax checks for `assets/files.js` and
  `testdata/files-explorer.test.cjs`, plus `git diff --check`, passed.
  Controlled-promise tests cover editing/cancel/blur, raw typed paths,
  refusals preserving listing/selection/scroll, cache bypass, canonical
  history, per-tab/instance/storage isolation, shortcut scoping, root reset,
  stale responses, and teardown; Go tests retain real route access checks.
- Brief lite self-review covered correctness, security boundaries, test
  integrity, and cleanup with no remaining findings. Independent review was
  omitted under the lead's explicit no-subagent rule. Browser/screenshot
  rounds were omitted under the owner UI principle; the owner's visual
  acceptance is pending. No daemon rebuild/restart, process kill, push, or
  merge was performed.
