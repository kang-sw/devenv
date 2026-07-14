# Plan: 260714-feat-dashboard-multi-server-workbench-keepalive — Phase 1: Per-server resources cache + mount-gating / lookup fix (hide, not unmount)

## Relevant Ticket Contract

- Decision 1: replace the single `resources: DashboardResourcesView | null`
  slot with a per-server cache (`Record<serverId, DashboardResourcesView>`);
  keep `activeResources` as the selected server's entry for the single-active
  workbench; change `findOpenWorkRoot` and `WorkbenchShell`'s mount-gating
  guard so each open work root resolves against ITS OWN server's cached tree.
- Constraint: reuse the existing `serverScopedIdentity`-keyed pane state and
  `display:none` hiding — no parallel keep-alive mechanism.
- Constraint: a plain focus switch must never deallocate (only explicit Off,
  which is Phase 2, deallocates).
- Non-goal (this phase and generally): no backend changes; polling stays
  single-target (only the visible/active root polls); right workbench stays
  single-active, no side-by-side rendering.
- Verification boundary (ticket text): with two servers On and a terminal
  started in each, switching focus back and forth preserves and restores both
  terminals without re-init; no visible regression to intra-server work-root
  switching. Verify via
  `ai-docs/ref/dashboard-headless-browser-verification.md`.

## Out of Scope

- Phase 2: left-nav per-server On/Off control, the deallocation gesture, and
  `server-local`'s disabled-Off pinning. Phase 1 only makes non-focused On
  servers survive a focus switch; there is no On/Off UI yet, so "On but not
  focused" is simply "any server whose cache entry hasn't been dropped."
- Backend changes (none needed — already per-`serverId` stateless/multi-upstream).
- git toolbar / activity console multiplexing (stay single-instance for the
  active root).
- Left-nav drag-reorder, work-root X-deselect, watched-work-root push channel
  (separate tickets).
- `ResourceNavigation` / `ServerRows` left-nav rendering — Phase 1 does not
  change what tree the left nav shows per server; it still shows `null` for
  unselected servers today (`App.tsx:2729`), and that stays as-is until Phase 2
  attaches the On/Off control.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:427-429` — `resources` is a single
  `useState<DashboardResourcesView | null>`. This is the slot Decision 1
  replaces with `Record<serverId, DashboardResourcesView>` (e.g.
  `resourcesByServer`).
- `ws-dashboard/frontend/src/App.tsx:502-514` —
  `resourceRefreshCoordinatorRef.current` is constructed once with
  `applyResources: setResources`. The coordinator (`resourceRefresh.ts:66-151`)
  always fetches for `selectedServerIdRef.current` (single-target fetch,
  serialized: a second `refresh()` call while one is in-flight is queued via
  `pendingForegroundRefresh`, not run concurrently — confirmed in
  `resourceRefresh.ts:85-90,122-127`). `applyResources` must become "merge
  `resources` into the cache at key `resources.server.id`" instead of a plain
  replace — the fetched payload always carries its own server id, so the cache
  key must come from the payload, not from `selectedServerIdRef.current` at
  apply time.
- `ws-dashboard/frontend/src/App.tsx:537-538` — `activeResources =
  resources?.server.id === selectedServerId ? resources : null`. Becomes
  `resourcesByServer[selectedServerId] ?? null`. All existing consumers of
  `activeResources` (`ResourceNavigation` at `App.tsx:1154`, `WorkbenchShell`
  at `App.tsx:1203`, `entities`/`workbenchModel` derivation) keep receiving
  this same single-active-server value unchanged.
- `ws-dashboard/frontend/src/App.tsx:531-536` — `serverConnections` fallback
  reads bare `resources.server` when `serversView` hasn't loaded yet. This
  bare `resources` reference (and the ones at `App.tsx:568`
  `flattenEntities(resources)` in `handleWorkRootOpened`, and `App.tsx:595-604`
  the selection-sync effect) are NOT cited in the ticket's root-cause list but
  are additional single-slot reads that need to move to either
  `activeResources` or an explicit `resourcesByServer[someId]` lookup once the
  slot becomes a map. Grep confirms these are the only remaining bare
  `resources` reads in `App.tsx` outside the two structural fix points below.
- `ws-dashboard/frontend/src/App.tsx:594-604` — effect that force-syncs
  `selectedServerId` to `resources.server.id` whenever they differ. Given the
  fetch coordinator is single-target and serialized (see above), this
  mismatch branch is effectively unreachable/defensive under Phase 1's
  "polling unchanged" constraint; safe to keep as a no-op fallback keyed off
  `resourcesByServer[selectedServerIdRef.current]` rather than a special
  strategy question. Flagged so the executor doesn't need to re-derive this.
- `ws-dashboard/frontend/src/App.tsx:3353-3378` — `WorkbenchShell`'s prop
  list. It currently receives only `resources: DashboardResourcesView | null`
  (bound to `activeResources` at the call site, `App.tsx:1203`). The mount-
  gating fix needs the FULL per-server cache inside `WorkbenchShell`, so a new
  prop (e.g. `resourcesByServer: Record<string, DashboardResourcesView>`) must
  be added and threaded from `App()`; `resources` (bound to `activeResources`)
  stays as-is for the active-root header/toolbar/`workbenchModel` derivation
  (`App.tsx:4083-4105`, `WorkbenchToolbar` `server={resources.server}` at
  `App.tsx:5643`), which must remain scoped to the selected server only.
- `ws-dashboard/frontend/src/workbench/openRootLookup.ts:11-29` —
  `findOpenWorkRoot(resources, ref)` already filters candidates by
  `root.resourcePath.serverId === ref.serverRoute` internally; it takes a
  single `DashboardResourcesView` and a `{rootId, serverRoute}` ref. **It does
  not need to change.** The fix belongs entirely at the call site
  (`App.tsx:4117`): index the new `resourcesByServer` map by `ref.serverRoute`
  and pass that single resolved tree in, e.g.
  `findOpenWorkRoot(resourcesByServer[ref.serverRoute] ?? null, ref)`. This is
  simpler than the ticket text's literal "change findOpenWorkRoot" and avoids
  touching `workbench/openRootLookup.test.ts:93-150`'s existing unit tests
  (they pass a single resources tree already and remain valid unmodified).
- `ws-dashboard/frontend/src/App.tsx:4107-4130` — `openWorkRootInstances`
  (built from `openWorkRootKeys`/`openWorkRootRefs` + `findOpenWorkRoot`) is
  computed unconditionally, before any of `WorkbenchShell`'s three early
  returns (`App.tsx:5611,5615,5619`). No hook ordering hazard: no hooks appear
  between this computation and the early returns (verified — the only
  `return` statements in `WorkbenchShell`'s body between line 3353 and 5700
  are the three `StatusPane` guards plus the final JSX). This means once the
  call site is fixed to resolve per-own-server, `openWorkRootInstances` will
  already be correct for background/non-selected servers before any
  render-gating change is made.
- `ws-dashboard/frontend/src/App.tsx:5611-5626` — the actual mount-gating
  bug: `if (!resources || !workbenchModel) return <StatusPane .../>` (where
  `resources` here is the `activeResources`-bound prop) unmounts the ENTIRE
  component tree, including `openWorkRootInstances.map(...)` at
  `App.tsx:5656-5698` (the `display:none`-hidden mounted-root subtree at
  `App.tsx:5674-5679`). This early return fires whenever the active server has
  no resolved tree (e.g. mid-switch, or a linked server that's connected but
  has no active work root selected) — tearing down every other On server's
  mounted panes as a side effect of the active server alone being empty. This
  is the crux structural change: the "no active work root" status message and
  the always-render mounted-instance subtree need to stop being coupled to one
  `return`.
- `ws-dashboard/frontend/src/App.tsx:493-496,3422-3470` — confirmed:
  `openWorkRootKeys`/`openWorkRootRefs` and all per-root pane state
  (`activePaneByRoot`, `terminalPanes`, `agentChatPanes`, etc.) are already
  keyed by `serverScopedIdentity(serverRoute, rootId)` inside `WorkbenchShell`
  and independent of which server is currently selected — reuse as-is, no new
  keep-alive mechanism needed.
- `ws-dashboard/frontend/src/App.tsx:2723-2737` — `ResourceNavigation` /
  `ServerRows` already pass `resources={server.id === selectedServerId ?
  resources : null}` (i.e. `null` for unselected servers) — this is pre-
  existing Phase-2-relevant behavior, not something Phase 1 touches.
- `ws-dashboard/frontend/src/resourceModel.ts:182-185` — confirmed
  `DashboardResourcesView = { server: ServerView; workspaces: WorkspaceView[] }`.
- `ws-dashboard/frontend/src/resourceRefresh.ts:45-64` — coordinator options
  type; `applyResources` signature is `(resources: DashboardResourcesView) =>
  void`, unchanged — only the closure passed at `App.tsx:510` needs to change
  from `setResources` to a merge-into-map callback.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` and
  `ai-docs/ref/dashboard-headless-browser-verification.md` — existing
  acceptance/manual-probe infra; no dedicated `App.tsx` unit-test file exists
  (App-level logic is exercised via the Playwright suite plus extracted pure
  helpers' unit tests). `workbench/openRootLookup.test.ts` and
  `resourceRefresh.test.ts` are the closest unit coverage and both remain
  valid without edits under the call-site-only fix above.

## Implementation Plan

1. In `App.tsx`, replace the `resources` state (`App.tsx:427-429`) with
   `resourcesByServer` (`Record<string, DashboardResourcesView>`), initialized
   to `{}`. Derive `activeResources = resourcesByServer[selectedServerId] ??
   null` (replaces `App.tsx:537-538`).
2. Update the `applyResources` callback passed into
   `createResourceRefreshCoordinator` (`App.tsx:506-514`) to merge into the
   map: `applyResources: (resources) => setResourcesByServer((current) => ({
   ...current, [resources.server.id]: resources }))`. Same treatment for any
   other place resources gets set directly if one is found during
   implementation (grep for `setResources(` to confirm none remain outside
   this coordinator plus `applyExternalResources`'s internal call, which
   already funnels through the same `applyResources` closure per
   `resourceRefresh.ts:133-141`).
3. Fix up the remaining bare single-slot reads found during survey:
   - `serverConnections` fallback (`App.tsx:531-536`) → use `activeResources`.
   - `handleWorkRootOpened`'s `flattenEntities(resources)` (`App.tsx:568`) →
     use `activeResources`. Confirmed via `onOpenWorkRoot`'s prop chain
     (`App.tsx:1162` → `ResourceNavigation` → `ServerRows`, which only
     receives a non-null `resources` tree for the currently selected server,
     `App.tsx:2729`): `onOpenWorkRoot`/`handleWorkRootOpened` can only fire for
     the already-selected server today, so `openedView.server.id ===
     selectedServerId` always holds at call time and `activeResources` is
     equivalent to the old bare `resources` read.
   - Selection-sync effect (`App.tsx:594-604`) → key off
     `resourcesByServer[selectedServerIdRef.current]` per the finding above;
     keep the effect's existing defensive shape, just re-based off the map.
4. Add a new prop to `WorkbenchShell` (`App.tsx:3353-3378`):
   `resourcesByServer: Record<string, DashboardResourcesView>` (do not remove
   the existing `resources` prop — it stays bound to `activeResources` and
   continues to gate the active-root header/toolbar/`workbenchModel`).
   Thread it from the call site at `App.tsx:1199-1210`
   (`resourcesByServer={resourcesByServer}`).
5. Fix the lookup: at `App.tsx:4117`, change
   `findOpenWorkRoot(resources, ref)` to
   `findOpenWorkRoot(resourcesByServer[ref.serverRoute] ?? null, ref)`. Leave
   `workbench/openRootLookup.ts` untouched (its signature already supports
   this).
6. Fix the mount-gating guard (`App.tsx:5611-5698`): restructure so
   `openWorkRootInstances.map(...)` (and its `display:none` hidden-root
   subtree) always renders regardless of whether the active server currently
   resolves, and only the header/toolbar/status-message area is conditional
   on `resources`/`workbenchModel`. Concretely: keep the `loading`/`error`
   full-bleed `StatusPane` returns for the "nothing has ever loaded yet"
   cases, but change the `!resources || !workbenchModel` branch
   (`App.tsx:5619-5626`) so it renders a `StatusPane` INSIDE the same
   `<div className="workbench-shell">` wrapper alongside (not instead of) the
   `openWorkRootInstances.map(...)` block — i.e. conditionally render either
   the toolbar+active area or the "No workRoot" `StatusPane` for the header
   region, while the mounted-root map always renders below/alongside it.
7. Typecheck (`tsc -b` via `npm run build`) to catch any other bare
   `resources`-as-single-slot usage the survey missed (the App.tsx grep found
   no other call sites, but the build is a cheap independent check for
   exhaustiveness given prop-shape changes).

## Verification Plan

- `cd ws-dashboard/frontend && npm run test:workbench` (covers
  `openRootLookup.test.ts`, `layoutRestore.test.ts`, `workbenchModel.test.ts`
  — confirms `findOpenWorkRoot` unit behavior is untouched).
- `cd ws-dashboard/frontend && npm run test:resource-model` (covers
  `resourceRefresh.test.ts`, `resourceModel.test.ts`, `linkedServers.test.ts`
  — confirms the coordinator's merge-into-map `applyResources` still behaves
  correctly for stale/skipped/failed refresh reasons).
- `cd ws-dashboard/frontend && npm run build` (tsc typecheck across the
  `App.tsx` prop-shape change and all call sites).
- Manual/headless verification per the ticket's stated boundary using
  `ai-docs/ref/dashboard-headless-browser-verification.md`: with two linked
  servers and a terminal started in each, switch focus back and forth and
  confirm both terminals survive and restore without re-init, and confirm no
  regression to intra-server work-root switching (still hide-not-unmount).

## Escalations

- None.
