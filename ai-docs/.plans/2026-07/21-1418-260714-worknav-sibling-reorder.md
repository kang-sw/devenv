# Plan: 260714-feat-dashboard-worknav-sibling-reorder — Phase 1: drag-based sibling reordering of work-root nav entries with persistence

## Relevant Ticket Contract

- Add drag handles/affordances to workspace rows and worktree rows that only
  accept drops from/onto the same sibling list (same server for workspaces;
  same parent workspace for worktrees). Reject/ignore cross-parent drops. This
  is explicitly NOT re-parenting.
- Introduce a persisted per-server ordering map (naming/shape to follow the
  `paneOrderByRoot` precedent) applied at render time over
  `resources.workspaces` and each workspace's `childWorkRoots`, without
  mutating the server-reported resource view.
- Persist the order per server; survive a resource refresh/reconnect for that
  server. Storage location (browser-local vs. daemon-side) is an
  implementation decision, "consistent with how nearby persisted UI state
  (e.g. dockview layout restore, `workbench/layoutRestore.ts`) is kept."
- Frontend only — no daemon/API changes.
- Do not add cross-level drag targets or reparenting UI.
- Verification: resource-model-level test for order application (workspace
  list order, worktree list order, order surviving refresh) plus
  browser/e2e drag coverage if the Playwright harness supports it, manual
  fallback acceptable otherwise.

## Out of Scope

- Any other phase of this ticket (only "Phase 1" is authorized; the ticket
  currently has no further phases listed, so this is its only phase).
- Reordering a workspace's own base/primary/compact row relative to its
  worktree children (parent/child, not siblings).
- Cross-server workspace reordering, or dragging a worktree to a different
  workspace (explicit non-goal — "NOT re-parenting").
- Daemon-side (`OpenedWorkRoots` / `opened-workroots.json`) changes — that
  store is a Rust-side daemon registry for opened/pinned roots (see
  `ai-docs/tickets/.done/260523-feat-ws-dashboard-persist-open-workroots.md`)
  and is unrelated to nav display order; this ticket's persistence is
  frontend browser-local, mirroring `workbench/layoutRestore.ts`.
- Any change to how `WorkRootId`/workspace id are derived server-side (ids are
  re-derived from canonicalized paths on every daemon load, not stored —
  confirmed in `ai-docs/tickets/.done/260721-bug-dashboard-worktree-create-duplicate-add.md`
  "Result" section — so a persisted order keyed by these ids is stable across
  reloads as long as the underlying path/workspace doesn't change, and simply
  goes stale/ignored if it does).

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:2975-2994` — `ResourceNavigation` renders
  `servers.map((server) => <ServerRows resources={resourcesByServer[server.id] ?? null} .../>)`.
  This is the natural grouping boundary for workspace siblings: one
  `resources.workspaces` array per `server.id`.
- `ws-dashboard/frontend/src/App.tsx:3112-3124` — `ServerRows` renders
  `resources.workspaces.map((workspace) => <WorkspaceRows key={workspace.id} .../>)`.
  Current ordering source is exactly server-supplied array order; this is the
  workspace-sibling render site to wrap with an order-application step.
- `ws-dashboard/frontend/src/App.tsx:9429-9552` — `WorkspaceRows`. Two render
  shapes:
  - `compactRoot` branch (App.tsx:9457-9490): workspace compacts to a single
    row, no worktree children rendered — no worktree-sibling reorder surface
    exists in this shape (nothing to do here).
  - Non-compact branch: `childWorkRoots = workspace.workRoots.filter(isWorkspaceNavChildWorkRoot)`
    (App.tsx:9444-9446) then `childWorkRoots.map((root) => ...)` at
    App.tsx:9516-9549. This is the worktree-sibling render site; the grouping
    boundary is one workspace (`workspace.id`, scoped further by `serverId`).
- `ws-dashboard/frontend/src/resourceModel.ts:522-524` —
  `isWorkspaceNavChildWorkRoot(root) => root.kind === "gitLinkedWorktree"`.
  Pins the exact "worktree sibling" definition: only linked-git-worktree kind
  work roots under a workspace are worktree siblings; the workspace's own
  base/primary root (`workspaceBaseWorkRoot`, resourceModel.ts:530-538) or
  compact root is never part of that sibling list.
- `ws-dashboard/frontend/src/resourceModel.ts:139-160` — `WorkspaceView`
  (`{ id, label, ..., workRoots }`) and `WorkRootView` (`{ id, resourcePath, ...}`)
  — the two id spaces an order map needs to key on (`workspace.id` for
  workspace siblings, `root.id` for worktree siblings).
- `ws-dashboard/frontend/src/resourceModel.ts:81-87` — `serverScopedIdentity(serverId, id)`
  — existing per-server compound key helper already used throughout App.tsx
  for exactly this "don't conflate state across servers" concern (e.g.
  `openWorkRootKeys`, `paneOrderByRoot`). Reuse this for the worktree-order
  map key (`serverScopedIdentity(serverId, workspace.id)` -> worktree id
  order) so worktree order also can't leak across servers even though it is
  additionally scoped by workspace.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts:20,62-115` —
  `WorkbenchPaneOrder = Record<string, readonly string[]>`, plus
  `applyWorkbenchPaneOrder`/`deriveWorkbenchPaneOrder`: the "ordered ids
  first, then any remaining live items appended in original order" shape to
  copy for sibling order application. This is the `paneOrderByRoot` precedent
  the ticket names directly; it operates over dockview pane *groups*, so it
  is not directly importable for a flat nav-row sibling list, but its
  ordering-application shape (persisted id-order map, applied at render time,
  never mutating the source data, unlisted ids fall back to natural/end
  order) is exactly what to reproduce for workspace/worktree lists.
- `ws-dashboard/frontend/src/App.tsx` — no `draggable`/`onDragStart`/
  `onDragOver`/`onDrop`/`DragEvent`/`dataTransfer` usage anywhere in this
  file (checked via grep, zero hits). Dockview's own tab drag-reorder is
  internal to the dockview library and only surfaces through
  `commitWorkbenchPaneMoveIntoDynamicGroup`/pane-move commands — there is no
  existing plain-DOM HTML5 drag idiom in this frontend to reuse for the nav
  rows. The ticket's own "Prior Art" section already acknowledges this
  ("this phase needs actual drag/drop interaction code ... on the nav rows").
  Risk signal: do not go looking for a reusable DnD helper that isn't there;
  new (small) HTML5 DnD glue on `ResourceRow`/`WorkspaceRows` is in scope and
  expected, per the ticket text itself.
- `ws-dashboard/frontend/src/workbench/layoutRestore.ts:39,47-112` — the
  persistence pattern to mirror: a single `browserStorage()` (from
  `workRootFiles.ts:786-792`, `window.localStorage` wrapped in a try/catch
  returning `null`) read/write, a versioned JSON blob
  (`{ version: 1, entries: [...] }`), and a defensive parser that silently
  drops anything malformed/mismatched-version rather than throwing. Directly
  reusable *pattern*, not directly reusable *code* (different shape: this
  ticket's persisted value is two id-order maps, not per-root layout
  entries).
- `ws-dashboard/frontend/src/App.tsx:476-478,488-490` — `paneOrderByRoot`
  state and `workbenchLayoutRestoreRef` seeded via
  `useState(...)`/`useRef(loadWorkbenchLayoutRestoreSnapshot())` at mount,
  the wiring precedent for a new `workNavOrder` piece of state seeded the
  same way (load once at mount, save on every change via an effect).
- `ai-docs/tickets/.done/260523-feat-ws-dashboard-persist-open-workroots.md:21,53`
  and `ai-docs/tickets/.done/260721-bug-dashboard-worktree-create-duplicate-add.md`
  ("Result" section) — confirms `OpenedWorkRoots`/`opened-workroots.json` is
  a daemon-side (Rust) registry of *which* roots are opened, and that
  `WorkRootId`s are deterministically re-derived from canonicalized paths on
  every daemon load rather than stored — i.e. stable enough across reloads to
  key a persisted client-side order map on, but this ticket's persistence
  target is a different concern/store entirely (browser-local nav display
  order, not the daemon's opened-root registry).
- `ws-dashboard/frontend/package.json:15` — `test:workbench` script chains
  `workbench/layoutRestore.test.js` after `workbenchModel.test.js`/
  `openRootLookup.test.js`/`deriveWorkbenchView.test.js`; `test:resource-model`
  (package.json:12) chains `resourceModel.test.js`/`resourceRefresh.test.js`/
  `linkedServers.test.js`. A new pure-logic module + test file must be wired
  into one of these `tsc -p tsconfig.route-tests.json && node ...` chains (or
  get its own new `test:*` script) to actually run in CI/local verification.

## Implementation Plan

1. **New pure ordering module** — add
   `ws-dashboard/frontend/src/workNavOrder.ts` (sibling to `resourceModel.ts`,
   not under `workbench/`, since this is a resource-nav concept, not a
   dockview-pane concept):
   - `export type WorkNavSiblingOrder = { workspaceOrderByServer: Record<string, readonly string[]>; worktreeOrderByWorkspace: Record<string, readonly string[]> }`.
   - `applySiblingOrder<T extends { id: string }>(items: readonly T[], order: readonly string[] | undefined): T[]` —
     ids in `order` first (skipping ids no longer present, mirroring
     `applyWorkbenchPaneOrder`'s dangling-id handling), then any remaining
     `items` not in `order` appended in their original (server-supplied)
     order. When `order` is `undefined`/empty, returns `items` unchanged —
     this is the "don't break existing ordering when no custom order is set"
     guarantee.
   - `reorderSiblingIds(effectiveOrder: readonly string[], sourceId: string, beforeId: string | undefined): string[]` —
     pure move-within-flat-list helper (remove `sourceId`, splice back in
     before `beforeId`, or at the end if `beforeId` is undefined/not found).
     Caller passes the *effective* (already-`applySiblingOrder`'d) id list so
     the returned array is always the full current sibling set in the new
     order, including any ids that weren't yet in the persisted map.
   - Persistence: `loadWorkNavOrderSnapshot(storage = browserStorage())` /
     `saveWorkNavOrderSnapshot(order, storage = browserStorage())`, mirroring
     `workbench/layoutRestore.ts`'s versioned-blob + defensive-parse pattern
     (storage key e.g. `"ws-dashboard.workNavOrder.v1"`; malformed/absent/
     wrong-version data resolves to
     `{ workspaceOrderByServer: {}, worktreeOrderByWorkspace: {} }`, which
     `applySiblingOrder` then treats as "no custom order" — this is the
     back-compat path for an older store/browser without this feature).
2. **Test file** — `ws-dashboard/frontend/src/workNavOrder.test.ts` covering:
   `applySiblingOrder` with no order (identity), partial order, order
   containing a stale/removed id, order missing a newly-added id (appended at
   end); `reorderSiblingIds` moving forward/backward/to-end; load/save
   round-trip through a fake `Storage`; load returning empty maps for
   malformed/absent/wrong-version JSON (back-compat case). Wire into
   `test:resource-model` in `ws-dashboard/frontend/package.json:12` (append
   `&& node ./node_modules/.tmp/route-tests/workNavOrder.test.js`), since this
   is nav/resource-model-level logic, not workbench/dockview logic.
3. **App-level state + persistence wiring** — in `App.tsx`, near the existing
   `paneOrderByRoot`/`workbenchLayoutRestoreRef` state (App.tsx:476-490):
   - `const [workNavOrder, setWorkNavOrder] = useState<WorkNavSiblingOrder>(() => loadWorkNavOrderSnapshot());`
   - An effect that calls `saveWorkNavOrderSnapshot(workNavOrder)` whenever
     `workNavOrder` changes (mirror the existing layout-save effect's
     shape/CONTRACT comment referencing `mergeWorkbenchLayoutRestoreEntries`
     if a merge-on-write is needed; here a plain overwrite is sufficient since
     `workNavOrder` is the single source of truth in memory, not merged from
     multiple concurrently-open roots).
   - A drop handler, e.g. `handleWorkspaceReorder(serverId, sourceId, beforeId)`
     and `handleWorktreeReorder(serverId, workspaceId, sourceId, beforeId)`,
     that recompute the effective order via `applySiblingOrder` +
     `reorderSiblingIds` and call `setWorkNavOrder`.
4. **Thread order + handlers down to render sites**:
   - `ResourceNavigation` (App.tsx:2889-2930 props, body at 2975-2994) gains
     `workNavOrder` and the two reorder callbacks as new props, passed
     through to each `ServerRows`.
   - `ServerRows` (App.tsx:3015-3128): apply
     `applySiblingOrder(resources.workspaces, workNavOrder.workspaceOrderByServer[server.id])`
     before the `.map((workspace) => <WorkspaceRows .../>)` at
     App.tsx:3114-3123; pass the worktree order map (or just
     `workNavOrder.worktreeOrderByWorkspace`) and both reorder callbacks down
     into each `WorkspaceRows`.
   - `WorkspaceRows` (App.tsx:9429-9552): apply
     `applySiblingOrder(childWorkRoots, worktreeOrderByWorkspace[serverScopedIdentity(serverId, workspace.id)])`
     before the `childWorkRoots.map(...)` at App.tsx:9516-9549. Only the
     non-compact branch needs this — the `compactRoot` branch (App.tsx:9457-
     9490) has no sibling list to reorder.
5. **Drag/drop affordance on rows** — add HTML5 DnD to the workspace row
   (`ResourceRow` with `presentation="workspace"`, rendered at
   App.tsx:9492-9515) and each worktree row (App.tsx:9516-9541):
   - `draggable` on the row's drag-handle element (or the whole row-select
     button, whichever reads better with existing `data-command-id`
     click-handling — verify `onDragStart` doesn't fight the existing
     `onClick`).
   - `onDragStart`: stash `{ sourceId, scopeKey }` in `event.dataTransfer`
     (JSON string via a dedicated MIME type constant, mirroring
     `workbenchPaneDragMimeType` in `editorGroupModel.ts:1` as the naming
     precedent — e.g. `workNavSiblingDragMimeType`). `scopeKey` is
     `server.id` for a workspace row, `serverScopedIdentity(serverId, workspace.id)`
     for a worktree row.
   - `onDragOver`/`onDrop` on sibling rows within the same list only:
     `event.preventDefault()` in `onDragOver` only when the dragged payload's
     `scopeKey` matches this row's own scope key (reject/ignore otherwise —
     this is the cross-parent-boundary guard the ticket requires). `onDrop`
     parses the payload, verifies scope match again, and calls the
     appropriate `handleWorkspaceReorder`/`handleWorktreeReorder` with this
     row's id as `beforeId`.
   - No visual drop-indicator polish is required by the ticket text; keep the
     affordance minimal (e.g. a small grip icon, reusing an existing
     `lucide-react` icon already imported in App.tsx:1-43 if a suitable one
     exists, else adding one import) unless CSS work is trivial.

## Verification Plan

- `cd ws-dashboard/frontend && npm run build` (typecheck + bundle the new
  module and App.tsx changes).
- `npm run test:resource-model` (after wiring `workNavOrder.test.js` into it
  per step 2) — covers the new pure ordering/persistence logic without a DOM
  harness.
- `npm run test:workbench` — regression guard; this phase should not touch
  `workbench/editorGroupModel.ts`/`layoutRestore.ts` behavior, so this should
  stay green unchanged.
- Manual/browser fallback: if the project's Playwright harness
  (`test:browser`) supports simulated `dragstart`/`dragover`/`drop` events,
  add one e2e case dragging a worktree row within one workspace and
  confirming order survives a reload; otherwise note manual verification
  (drag two workspaces under the same server, drag two worktrees under the
  same workspace, reload the page, confirm order persisted; confirm a drag
  attempt from one server's workspace list onto another server's list is a
  no-op) as the accepted fallback per the ticket's own verification text.

## Escalations

- None.
