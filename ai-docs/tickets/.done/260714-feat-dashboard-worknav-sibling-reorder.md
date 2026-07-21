---
title: Left-nav sibling reorder for workspaces and worktrees (drag-and-drop, per-level only)
related:
  260714-feat-dashboard-multi-server-workbench-keepalive: sibling-scope carve-out - that ticket's Non-Goals explicitly defers this as a separate ticket
related-mental-model:
  - ws-web-dashboard
---

# Left-nav sibling reorder for workspaces and worktrees (drag-and-drop, per-level only)

## Background

Owners want to reorder items in the left WorkRoot navigation
(`ResourceNavigation`, `ws-dashboard/frontend/src/App.tsx:2679-2948`) by
drag-and-drop, but **only within their own hierarchy level**:

- Sibling **workspaces** can be reordered among themselves
  (`ServerRows`/`WorkspaceRows`, `App.tsx:2758-2854` and `App.tsx:8951-9068`
  render `resources.workspaces` and each workspace's `childWorkRoots` in
  server-supplied order today).
- Sibling **worktrees** (linked Git worktrees under one workspace, filtered by
  `isWorkspaceNavChildWorkRoot`, `App.tsx:9070-9072`) can be reordered among
  themselves.

**This is explicitly NOT re-parenting.** A worktree cannot be dragged under a
different workspace, and a workspace cannot be dragged above/below a different
server's workspace list. Only sibling order within the same parent/level may
change. The owner emphasized this constraint directly - do not build a
general-purpose tree-reparenting drag surface.

## Constraints

- Frontend only. No daemon/API changes are expected: the resource list already
  arrives fully formed per server; this ticket only needs to let the client
  override display order and persist that override, not change what the
  server reports.
- Scoped to `ResourceNavigation` and its child renderers
  (`ServerRows`, `WorkspaceRows`, `ResourceRow`) around `App.tsx:2679-2948` and
  `App.tsx:8951-9068`.
- Order must persist **per server** (a server's saved sibling order should
  survive reconnect/reload of that server's resource tree; do not conflate
  orders across different linked servers, mirroring the `serverScopedIdentity`
  keying already used for open-work-root/pane state elsewhere in this file).

## Prior Art

`paneOrderByRoot` (`App.tsx:459`, type `Record<string, WorkbenchPaneOrder>`
from `workbench/editorGroupModel.ts`) is the existing sibling-reorder
precedent in this codebase: dockview's built-in tab drag reordering feeds pane
moves back into a plain ordering map keyed by work-root (`rootKey =
serverScopedIdentity(serverRoute, workRootId)`), which `applyWorkbenchPaneOrder`
then applies when rendering pane tabs (see `App.tsx:4004-4020`,
`workbench/editorGroupModel.ts`, and its test coverage in
`workbench/workbenchModel.test.ts`). That pattern - a persisted, per-scope-key
order map applied at render time, independent of the underlying data's native
order - is the shape to follow here: a per-server sibling-order map for
workspace ids and one for worktree ids (scoped by parent workspace), applied
when `ServerRows`/`WorkspaceRows` render their children, rather than mutating
`resources.workspaces` itself.

Unlike `paneOrderByRoot`, the left-nav rows are not dockview panels, so there is
no built-in drag-reorder mechanism to hook into here - this phase needs actual
drag/drop interaction code (e.g. HTML5 DnD or a small pointer-based reorder
handler) on the nav rows, gated so a drag can only complete against a sibling
in the same list.

## Phases

### Phase 1: Sibling drag-and-drop reorder for workspaces and worktrees

- Add drag handles/affordances to workspace rows (`WorkspaceRows`,
  `App.tsx:8951-9068`) and worktree rows (the `childWorkRoots.map(...)` block
  within the same component) that only accept drops from - and onto - the same
  sibling list (same parent workspace for worktrees; same server for
  workspaces). Reject/ignore drops that would cross a parent boundary.
- Introduce a persisted per-server ordering map (naming and shape to follow
  the `paneOrderByRoot` precedent above) and apply it when rendering
  `resources.workspaces` and each workspace's `childWorkRoots`, without
  mutating the server-reported resource view itself.
- Persist the order per server (survives a resource refresh/reconnect for that
  server); decide storage location (browser-local vs. a small daemon-side
  registry entry) during implementation, consistent with how nearby persisted
  UI state (e.g. dockview layout restore, `workbench/layoutRestore.ts`) is
  kept.
- Do not add cross-level drag targets, drop-zone affordances between
  different parents, or any reparenting UI.

Verification should include a resource-model-level test for the ordering map
application (workspace list order, worktree list order, order surviving a
resource refresh) plus browser/e2e coverage for the actual drag gesture if the
project's existing Playwright harness supports simulated drag events; manual
verification is acceptable to note as a fallback if not.

## Promotion (2026-07-21)

Promoted per user curation.

## Result (2026-07-21)

- Feature: drag to reorder sibling work-root nav entries, with browser-local
  persistence. New pure module `workNavOrder.ts` (`WorkNavSiblingOrder`,
  `applySiblingOrder`, `reorderSiblingIds`, `isAcceptableSiblingDrop`,
  versioned localStorage) plus `App.tsx` HTML5 DnD glue. Daemon-side
  `OpenedWorkRoots` is untouched — frontend-only, as the ticket's Constraints
  required. Sibling scope: workspace rows are scoped to `server.id`, worktree
  rows to `serverScopedIdentity(serverId, workspaceId)`; cross-scope drops are
  rejected.
- Reused the `layoutRestore.ts` persistence pattern and the `paneOrderByRoot`
  render-ordering shape called out in Prior Art. Back-compat: an absent stored
  order falls back to natural (server-reported) order.
- Notable correctness point found during review: the compact-root render path
  (single-work-root workspace) initially did not participate in reorder at
  all. Fixed by threading drag props through that branch AND adding a new
  `dragEntityId` prop so the compact branch keys drag identity on
  `workspace.id` (not the workRoot display `id` it renders) — the two id
  spaces would otherwise silently no-op the reorder for compact rows.
- Verification: `npm run build` plus `test:resource-model` (including the new
  `workNavOrder.test.ts`) and `test:workbench` all pass.
- Review: 3-partition review (correctness/opus, fit, test). Fit found 1
  Critical (compact-root rows excluded from reorder) — fixed. Test found 1
  Important (the cross-scope drop-acceptance predicate was inline-duplicated
  and untested) — fixed by extracting `isAcceptableSiblingDrop` into
  `workNavOrder.ts` with 4 new unit cases covering its branches. The delta
  re-review (`ea032f52..d7c5647f`) verified both fixes directly against the
  identity-space and predicate-equivalence concerns and found no new issues.
- Residual accepted MINOR item (both the original review and the delta
  re-review converge on this single item; not two distinct items): a DOM drop
  always supplies a `beforeId` (the target row's id), so a drag can only
  insert before a sibling, never explicitly append after the last one — an
  item can still be moved to the end indirectly by moving every other sibling
  ahead of it. `reorderSiblingIds` already supports and unit-tests the
  `beforeId === undefined` end-append case, so this is a UX reachability
  limit only, with no data loss/duplication risk; the ticket does not require
  an end-of-list drop zone. Accepted as-is, not re-opened.
- Commits: feat `ea032f52`, relay fix `d7c5647f` (plan `e3fd3780`).
- Open item: manual dogfood confirmation (drag-reorder a sibling, reload the
  browser, confirm the order persists) remains the user's step; no live
  dashboard instance was available in the implementing session to capture
  that evidence directly.
