# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 5: Persist and restore per-work-root dockview layout across reload

## Relevant Ticket Contract

- Serialize each work root's dockview arrangement (groups, tab order, split
  proportions, active panel) on change (debounced) into browser-local
  storage, keyed by `serverRoute` + `workRootId`. On mount, before creating
  any panes for a work root, read the persisted layout if present and use it
  to drive `buildWorkbenchEditorGroups`/pane creation order instead of the
  default single-group arrangement.
- Every pane reference must be revalidated on restore: a file pane restores
  only if the file remains previewable; a terminal pane restores only if it
  maps to a listed daemon-alive terminal or a valid restore intent.
  Unavailable references are dropped, not shown as errors.
- Persisted keys must include `serverRoute` + `workRootId` + pane/terminal id
  to avoid collisions across linked servers and work roots (ticket
  Constraints).
- Restore must never treat persisted browser state as authoritative over live
  daemon/resource state; every restored reference is revalidated on load
  (ticket Constraints).
- Contract-first: yes — this phase revises the `WorkRoot IO Restore Model`
  spec anchor's claim that "exact browser workbench arrangement remain
  outside the restore model." The spec update is binding for this phase's
  completion (see Implementation Plan step 7 / spec diff below).
- Verification should cover layout serialization round-trip tests, restore
  behavior when a referenced file/terminal is no longer available, and
  collision tests for the same `workRootId` on two different `serverRoute`
  values.

## Out of Scope

- Phase 6 (terminal visual buffer/scrollback capture-restore) and Phase 7
  (reusing this primitive for in-session close/reopen) — later phases, not
  touched here.
- Any change to how terminal reattach-by-id or file-pane content restore
  decide reattach-vs-fresh-spawn (Phase 4's `listTerminals` reattach and the
  existing `readOnlyFilePanes` restore mechanism stay as-is); this phase only
  restores *layout shape* (which groups exist, which pane ids sit in which
  group/order, which pane is active per group), not pane content.
- Dockview's native `toJSON()`/`fromJSON()` full-layout serialization is not
  usable as the persistence mechanism (see Codebase Findings) and is out of
  scope as an approach.
- Terminal WebSocket lifecycle, cursor/truncation behavior (Phases 3-4) —
  unrelated to this phase, do not touch `App.tsx:6235-6306`'s socket effect.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L384-L400` — `workbenchGroupsByRoot`
  (`Record<rootKey, {id,label}[]>`) and `paneOrderByRoot`
  (`Record<rootKey, WorkbenchPaneOrder>`) are already lifted to `App()` and
  keyed by `rootKey = serverScopedIdentity(serverId, rootId)` (Phase 2). This
  is the primary state this phase must persist/restore.
- `ws-dashboard/frontend/src/App.tsx#L3356-3358` — `activePaneByRoot`
  (`Record<rootKey, Record<groupId, paneId>>`) is local to `WorkbenchShell`,
  not lifted to `App()`. `WorkbenchShell` already receives
  `workbenchGroupsByRoot`/`paneOrderByRoot`/`onWorkbenchGroupsByRootChange`/
  `onPaneOrderByRootChange` as props (`App.tsx#L3302-3336`), so the combined
  persistence effect (grouping all three pieces) is most naturally placed
  inside `WorkbenchShell`, where all three are in scope together.
- `ws-dashboard/frontend/src/App.tsx#L656-674` — the effect that adds a
  newly-visited root to `openWorkRootKeys`/`openWorkRootRefs` (Phase 1) is the
  natural seed point: when a `rootKey` is first added here, this phase should
  also seed `workbenchGroupsByRoot[rootKey]`/`paneOrderByRoot[rootKey]` from a
  persisted snapshot (if present) instead of leaving them absent (which falls
  back to `initialWorkbenchGroups`/`{}` at `App.tsx#L3611-3612` and
  `App.tsx#L731,819-822`).
- `ws-dashboard/frontend/src/workRootFiles.ts#L436-L534,699-711` — **direct
  reusable pattern**: `readOnlyFilePaneRestoreSnapshot`/
  `loadReadOnlyFilePaneRestoreSnapshot`/`saveReadOnlyFilePaneRestoreSnapshot`
  plus the `pruneReadOnlyFilePaneOrder` degrade helper is exactly the
  save/load/prune shape this phase needs: versioned JSON in
  `window.localStorage` under a fixed key (`readOnlyFilePaneRestoreStorageKey
  = "ws-dashboard.readOnlyFilePanes.v1"`, `workRootFiles.ts#L784`), guarded
  `try/catch`, a `browserStorage()` null-check for non-browser/test
  environments, and pruning stale pane-id references out of an
  order-by-group map by filtering against a live pane-id set. Model the new
  module on this exact shape rather than inventing a new one.
- `ws-dashboard/frontend/src/App.tsx#L587-592` — the existing save effect for
  read-only file panes is a plain `useEffect` that re-saves on every relevant
  state change (no explicit debounce timer; React's batching plus a cheap
  JSON.stringify of a small object is treated as "debounced enough" by the
  existing pattern). Follow the same style unless profiling proves it
  insufficient; do not add a bespoke debounce utility if none is used
  elsewhere for this kind of save.
- `ws-dashboard/frontend/src/terminals.ts#L303-308` (`terminalPaneId`) and
  `ws-dashboard/frontend/src/workRootFiles.ts#L362-374`
  (`readOnlyFilePaneId`) — both pane-id families are **deterministic**,
  derived from stable identifiers (`serverRoute`+`terminalId`,
  `serverRoute`+`workRootId`+`path`+`mode`), not randomly generated. This
  means a persisted `paneOrderByRoot`/`activePaneByRoot` value referencing a
  pane id by string remains valid/matchable across reload as long as the
  underlying terminal/file still exists — no id-remapping table is needed,
  only a "does this pane id currently exist" prune check exactly like
  `pruneReadOnlyFilePaneOrder`.
- `ws-dashboard/frontend/src/resourceModel.ts#L81-86`
  (`serverScopedIdentity`) — existing collision-safe key builder
  (`[dashboardServerRoute(serverRoute), ...parts].join("/")`), already used
  for `rootKey`. Reuse this, do not invent a new key format; it already
  satisfies the ticket's `serverRoute`+`workRootId` collision constraint when
  used as the map key, matching the read-only-file-pane restore snapshot's
  approach of storing full identifying fields inline.
- `ws-dashboard/frontend/src/workbench/layoutSerialization.ts` (whole file) —
  **risk signal / dead code with an incompatible model**: this file defines
  `WorkbenchLayoutState`/`WorkbenchArrangementNode` (a recursive
  attachment/split tree) and `serializeWorkbenchLayout`, but it is **not
  imported anywhere** except its own test and `workbench/index.ts`'s
  re-export barrel — `App.tsx` does not import it (confirmed: `App.tsx`'s
  only import from `./workbench` is the barrel at `App.tsx#L106-125`, which
  does not include any `layoutSerialization` symbol). Its tree-shaped
  `WorkbenchArrangementNode` model does not match the app's actual flat
  `groups[]` + `paneOrderByGroup` + `activePaneByGroup` shape, and it has no
  concept of a work-root scope at all. Do not retrofit this file for Phase 5;
  write a new module (e.g. `workbench/layoutRestore.ts`) shaped like
  `workRootFiles.ts`'s restore functions instead. Flag this dead code in the
  PR/report; deleting or repurposing it is a separate decision outside this
  phase's scope.
- `ws-dashboard/frontend/node_modules/dockview-core/dist/cjs/api/component.api.d.ts#L34-35`
  and `dockviewComponent.d.ts` (`SerializedDockview`) — dockview's
  `DockviewApi` does expose `toJSON(): SerializedDockview` /
  `fromJSON(data)`. **This is not usable for this phase's restore path**:
  `fromJSON` reconstructs panels purely from serialized `params`, but this
  app's panel params (`DockviewWorkbenchPanelParams.body`, see
  `dockviewLayout.tsx#L65-77`) carry live `ReactNode`s and closures
  (`onRequestClosePane`, terminal callbacks) that are not JSON-serializable
  and are only known at render time. The app already owns a declarative
  reconciliation loop (`syncDockviewWorkbench`,
  `dockviewLayout.tsx#L315-432`) that creates/moves dockview panels to match
  an app-owned `groups`/`activePaneByGroup` model every render; `fromJSON`
  would fight this loop by trying to create panels a second, incompatible
  way. Conclusion: persist the app's own group/order/active model (as this
  plan does), not dockview's native serialization.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L426-432` —
  `syncDockviewWorkbench` already returns a `workbenchGroupByDockGroup: Map<
  dockGroupId, workbenchGroupId>`, i.e. the app already tracks which live
  dockview group backs each dashboard group id after every sync. This is the
  hook point for the one piece of the ticket's "split proportions" requirement
  that dockview *can* give us without `fromJSON`: each dockview group's `api`
  exposes `width`/`height` getters and a `setSize({width|height})` setter
  (`node_modules/dockview-core/.../api/panelApi.d.ts#L47-53`,
  `gridviewPanelApi.d.ts#L16-23`). Capturing `{width,height}` per dashboard
  group id after sync (on `api.onDidLayoutChange`, debounced) and reapplying
  it via `group.api.setSize(...)` immediately after `syncPanels()` creates/
  finds that group on restore is a tractable, additive mechanism — it does
  not require `fromJSON` and does not conflict with the existing
  reconciliation loop. Treat this as best-effort: if a persisted group id from
  a prior session's layout doesn't exist in the freshly built groups, drop
  the size entry (same prune-by-liveness rule as pane ids); no error path.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L20-21`
  (`WorkbenchPaneOrder`, `WorkbenchActivePaneState`) — existing shared types
  for pane-order-per-group and active-pane-per-group; reuse these types for
  the restore snapshot instead of declaring new shapes.
- `ws-dashboard/frontend/package.json#L15` (`test:workbench` script) — runs
  `workbench/workbenchModel.test.ts` and `workbench/openRootLookup.test.ts`.
  A new `workbench/layoutRestore.test.ts` should be added to this script's
  command chain (same `tsc -p tsconfig.route-tests.json && node
  ./node_modules/.tmp/route-tests/workbench/<name>.test.js` pattern as the
  existing two).
- `ws-dashboard/frontend/src/workRootFiles.test.ts` — existing test file for
  the pattern being mirrored; read it for the exact assertion style
  (round-trip save/load, corrupt-JSON fallback, prune-on-missing-pane) before
  writing `layoutRestore.test.ts`.

## Implementation Plan

1. Add `ws-dashboard/frontend/src/workbench/layoutRestore.ts`, modeled
   directly on `workRootFiles.ts#L436-534,699-711`:
   - Type `WorkbenchLayoutRestoreEntry = { serverRoute: string; workRootId:
     string; groups: ReadonlyArray<{id:string; label:string}>;
     paneOrderByGroup: WorkbenchPaneOrder; activePaneByGroup:
     WorkbenchActivePaneState; groupSizeById?: Record<string, {width?:number;
     height?:number}> }`.
   - Type `WorkbenchLayoutRestoreSnapshot = Record<rootKey, WorkbenchLayoutRestoreEntry>`
     (rootKey = `serverScopedIdentity(serverRoute, workRootId)`, matching the
     existing `workbenchGroupsByRoot`/`paneOrderByRoot` key convention).
   - `loadWorkbenchLayoutRestoreSnapshot(storage = browserStorage-equivalent)`
     — versioned (`version: 1`) JSON parse from a new fixed key
     `"ws-dashboard.workbenchLayout.v1"`, defensive `try/catch` returning
     `{}` on any parse/shape failure, matching
     `loadReadOnlyFilePaneRestoreSnapshot`'s defensiveness.
   - `saveWorkbenchLayoutRestoreSnapshot(entries, storage)` — same
     `removeItem`-when-empty / `setItem`-otherwise shape as
     `saveReadOnlyFilePaneRestoreSnapshot`.
   - `pruneWorkbenchLayoutOrder(orderByGroup, livePaneIds)` — reuse/port
     `pruneReadOnlyFilePaneOrder`'s exact filter logic (drop pane ids not in
     the live set, drop groups left empty) generalized for this module (do
     not import the file-panes-specific private function; duplicate the
     ~10-line filter here since it is currently unexported/file-private in
     `workRootFiles.ts`).
   - A `browserStorage()`-equivalent local helper (or export the existing one
     from `workRootFiles.ts` if that's cheaper than duplicating — check
     whether it's already exported before duplicating).
2. In `App.tsx`, add a `useEffect` (in `WorkbenchShell`, since it has
   `workbenchGroupsByRoot`, `paneOrderByRoot`, and its local
   `activePaneByRoot` all in scope) that calls
   `saveWorkbenchLayoutRestoreSnapshot` whenever
   `workbenchGroupsByRoot`/`paneOrderByRoot`/`activePaneByRoot` change,
   building one `WorkbenchLayoutRestoreEntry` per `openWorkRootKeys` entry
   (using `openWorkRootRefs[rootKey]` for `serverRoute`/`workRootId`).
   Mirror the existing save effect's style (`App.tsx#L587-592`): no bespoke
   debounce, since the existing precedent doesn't use one either.
3. In `App.tsx`'s root-open effect (`App.tsx#L656-674`), when a `rootKey` is
   newly added to `openWorkRootKeys` (i.e. this is the first time this
   session sees this root), read
   `loadWorkbenchLayoutRestoreSnapshot()[rootKey]` once and, if present, seed
   `workbenchGroupsByRoot`/`paneOrderByRoot` (via
   `setWorkbenchGroupsByRoot`/`setPaneOrderByRoot`) with the persisted
   `groups`/`paneOrderByGroup`, pruned against the pane ids currently known
   to exist (`Object.values(terminalPanes)` pane ids + `readOnlyFilePanes`
   pane ids at that moment — if those haven't loaded yet for this root,
   revalidate again in a follow-up effect once they do, since terminal
   listing is async per the ticket's Phase 4 background). Only seed if
   `workbenchGroupsByRoot[rootKey]` is not already present, so this never
   clobbers an in-session live layout.
4. Seed `activePaneByRoot[rootKey]` the same way inside `WorkbenchShell`
   (it already receives `openWorkRootKeys`/`openWorkRootRefs` as props per
   `App.tsx#L3304-3305`), reusing the same loaded snapshot entry.
5. Add the revalidation/prune pass: once `terminalPanes`/`readOnlyFilePanes`
   are populated for a restored root (i.e. on every render/effect where they
   change, not just at seed time), run `pruneWorkbenchLayoutOrder` against
   `paneOrderByRoot[rootKey]` and drop any `activePaneByRoot[rootKey][groupId]`
   entry whose pane id is no longer live — this reuses
   `reconcileActiveWorkbenchPanes` (`editorGroupModel.ts#L163-185`), which
   already falls back to `group.panes[0]?.id` when the preferred/current
   active pane id isn't in the live set, so no new active-pane-repair logic
   should be needed beyond calling it with the restored preference.
6. Best-effort split-size restore: extend `DockviewWorkbenchLayoutProps`
   (`dockviewLayout.tsx#L49-63`) with an optional `initialGroupSizeById` prop
   and an `onLayoutSnapshot(sizeByWorkbenchGroupId)` callback fired
   (debounced, e.g. via the existing `queueMicrotask`/effect idiom already in
   this file) from a new `api.onDidLayoutChange` subscription in
   `handleReady`. On `syncPanels()` after initial panel creation, for each
   dashboard group id present in `initialGroupSizeById`, resolve its live
   dockview group via the existing `workbenchGroupByDockGroup`-equivalent
   mapping and call `group.api.setSize({width, height})`. Wire the
   size-by-group map through the same per-root
   `WorkbenchLayoutRestoreEntry.groupSizeById` persisted by step 1-2. If this
   step proves awkward to plumb through the per-root instance loop cleanly,
   it is the one part of this phase safe to land as a documented partial gap
   (group/tab-order/active-pane restore working, split-size restore
   following in the same phase's fix-up commit) — do not let it block landing
   the rest, but do not silently drop it from the spec update without saying
   so.
7. Update the spec anchor per the diff below (binding for this phase, per
   ticket's Contract-first: yes classification).

### Spec diff proposal (`ai-docs/spec/ws-web-dashboard/index.md`, `WorkRoot IO Restore Model` anchor, `#260516-ws-web-dashboard-workroot-io-restore-model`)

Current (approx. lines 1530-1548, will have shifted since Phase 1):

> The dashboard combines daemon-owned live terminal state, read-only file pane
> state, and browser workbench arrangement into one restore model for
> selected workRoots. Daemon state is authoritative for live terminal
> existence, while browser arrangement remains presentation state. File panes
> restore only when the file remains previewable; otherwise the pane shows an
> honest unavailable state. The daemon persists the owner's opened workRoot
> paths in local dashboard state and seeds the live resource view from that
> remembered list on startup. Remembered roots re-run normal discovery
> instead of bypassing moved, offline, inaccessible, primary-root, or
> linked-worktree classification. Auth sessions, live terminal process
> survival, Activity acknowledgement state, and **exact browser workbench
> arrangement** remain outside the restore model.

Proposed replacement of the last sentence plus a new paragraph:

> Auth sessions, live terminal process survival, and Activity acknowledgement
> state remain outside the restore model. Per-work-root browser workbench
> arrangement — dockview group membership, tab order, active pane per group,
> and split proportions on a best-effort basis — is persisted to browser-local
> storage keyed by server route and workRoot id, and restored on reload (and
> on reopening a work root closed via the explicit close action, per a later
> phase). Restore never treats persisted layout as authoritative over live
> daemon/resource state: every pane reference in a persisted layout is
> revalidated against currently-available resources, and an unavailable
> reference (a file pane whose file is no longer previewable, or a terminal
> pane with no matching daemon-alive terminal or restore intent) is silently
> dropped from the restored layout rather than shown as an error, consistent
> with this anchor's existing file-pane restore rule.

(Exact final wording is the implementing agent's call; the binding content is:
scope narrows from "arrangement stays outside" to "arrangement is restored,
keyed by serverRoute+workRootId, revalidated, degrade-by-drop on missing
references, split proportions best-effort" — keep Auth/terminal-process/
Activity-ack exclusions unchanged since those are untouched by this phase.)

## Verification Plan

- `npm run test:workbench` (extend to include the new
  `layoutRestore.test.ts`, per `package.json#L15`) — unit-test
  save/load round-trip, corrupt-JSON fallback (mirroring
  `workRootFiles.test.ts`'s coverage style), and the prune-on-missing-pane
  degrade path.
- New collision test: two entries with the same `workRootId` but different
  `serverRoute` values save/load independently without clobbering each other
  (ticket's explicit verification-boundary requirement).
- New restore test: a persisted layout referencing a pane id that does not
  exist in a supplied live pane-id set is pruned from `paneOrderByGroup` and
  from `activePaneByGroup` (falls back via `reconcileActiveWorkbenchPanes`).
- `npm run build` (tsc -b + vite build) — required by this repo's existing
  phase verification convention (Phases 1-4 all ran this).
- `npm run test:terminals` — regression check, since step 5's revalidation
  pass touches `terminalPanes`-derived pane id sets.
- Manual/structural verification for the actual dockview-visible behavior
  (reload a work root with a multi-group layout, confirm groups/tabs/active
  pane restore) — Playwright e2e remains not runnable in this sandbox
  (`libasound.so.2` missing, no Chromium binary), the same pre-existing gap
  as Phases 1-4; note this explicitly rather than skipping the verification
  boundary silently.

## Escalations

- None.
