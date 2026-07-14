# Plan: 260714-feat-dashboard-multi-server-workbench-keepalive — Phase 2: Left-nav per-server On/Off lifecycle UX

## Relevant Ticket Contract

- On = workspaces expanded + server kept warm (opened roots' panes stay mounted,
  hidden via `display:none` when not focused).
- Clicking a server label auto-turns it On (expand + focus) — IDENTICAL to
  today's activation; do NOT introduce a new activation gesture. The left nav
  already passes a `null` tree for unselected servers (`App.tsx:2770`,
  re-verified against current head `6726ded5`; was `:2742` when this plan was
  first drafted); that path now maps to "On but not focused" backed by the
  Phase 1 `resourcesByServer` cache.
- Off is the ONLY new gesture: an On/Off button on the server label. Clicking
  Off collapses that server's workspaces AND deallocates its tabs/panes AND its
  cached resources entry — the one place that tears down per-server workbench
  state.
- `server-local` is always On; render its On/Off button disabled.
- Right workbench stays single-active; no change to workbench rendering, git
  toolbar, or activity console (Constraint, Non-Goal).
- Constraint: reuse the existing `serverScopedIdentity`-keyed pane state and
  `display:none` hiding, not a parallel mechanism.
- Constraint: explicit Off is the only path that deallocates; a plain focus
  switch must never deallocate.
- No backend changes (Non-Goal — backend is already per-`serverId` stateless).
- Polling is unchanged: only the visible/active root polls (Decision 4).

## Out of Scope

- Phase 1 (`resourcesByServer` cache, `findOpenWorkRoot` / mount-gating fix) —
  already implemented and merged (`bd4702b0`, remediated in `32fc74ad`). Reuse
  as-is; do not modify `findOpenWorkRoot`, the `WorkbenchShell` mount-gating
  guard, or `mergeResourcesByServer`.
- The remote-child-workroot regression fix that landed on top of Phase 1
  (merge `6726ded5`, after this plan was first drafted): `resolveActiveResources`
  / `withLastNonNullResourcesByServer` (`resourceModel.ts:220-245`) and
  `resolveEffectiveActiveRootKey` (`workbench/openRootLookup.ts:73-87`) plus
  their `App.tsx` call sites. Already implemented and merged; reuse as-is. The
  ONLY touch this phase makes to that logic is the narrow ref-reset addition
  inside `WorkbenchShell`'s existing teardown effect specified in
  Implementation Plan step 3 (resetting `lastActiveRootKeyRef` /
  `lastActiveRootServerIdRef` when they point at a rootKey Off just closed).
  Do not otherwise modify `resolveActiveResources`,
  `withLastNonNullResourcesByServer`, or `resolveEffectiveActiveRootKey`.
- Backend changes of any kind (Non-Goal).
- Workbench rendering, git toolbar, activity console multiplexing across
  servers (Non-Goal / Constraint — stays single-instance for the active root).
- Side-by-side / multi-root workbench rendering (Non-Goal).
- Left-nav workspace drag-reorder and work-root X-deselect (separate tickets).
  The existing single-root `workRoot.close` command handler is reused/batched,
  not redesigned.
- Backend "watched work-root" push channel (separate idea ticket).

## Codebase Findings

- **Anchors below were re-verified against the current goal head
  (`6726ded5`) after this plan was first drafted; `App.tsx` line numbers
  shifted by the remote-child-workroot regression fix that landed in the
  interim (net +72 lines by the tail of the file, +3/+27/+28/+30/+48 in
  earlier sections — see the new bullets after this list for what that fix
  added and why Off must now also account for it).**
- `ws-dashboard/frontend/src/App.tsx:2764-2778` — `ResourceNavigation` passes
  `resources={server.id === selectedServerId ? resources : null}` (ternary at
  `:2770`) into `ServerRows`, where `resources` is the single active-server
  tree (`activeResources`, not the Phase 1 per-server cache). This is the
  exact line the ticket calls out as needing to map onto "On but not focused";
  today it always collapses/nulls out any non-selected server regardless of
  On state.
- `ws-dashboard/frontend/src/App.tsx:2799-2895` — `ServerRows` renders the
  server label button, `.server-row-actions` (existing `ServerActionButton`s +
  `OpenWorkRootControl`, `:2856-2877`), and gates the expanded
  `.server-workspaces` block on `{selected && resources ? ... : null}`
  (line `:2879`). This guard is what must change to `{isOn && resources ? ...}`
  so an On-but-unfocused server also renders expanded, using its own cached
  tree.
- `ws-dashboard/frontend/src/App.tsx:1192-1210` — `ResourceNavigation` is
  invoked with `resources={activeResources}` only; it has no
  `resourcesByServer` prop today. Compare to `WorkbenchShell`, invoked a few
  lines below (`App.tsx:1238-1259`) with BOTH `resources={activeResources}`
  AND `resourcesByServer={resourcesByServer}` (`App.tsx:1243`) — that is the
  exact sibling pattern to mirror when threading the per-server cache down to
  `ResourceNavigation`/`ServerRows`. (The regression fix also added a new
  `selectedServerId` prop to that same `WorkbenchShell` call, immediately
  after, at `:1244` — unrelated to this step, noted only so the `:1243` anchor
  isn't mistaken for further drift.)
- `ws-dashboard/frontend/src/App.tsx:437-439` — `resourcesByServer` state
  (`Record<serverId, DashboardResourcesView>`), the Phase 1 cache. Its
  presence/absence is the natural source of truth for "On" — no new boolean
  "on" map is needed. `isOn` can be derived per row as: `server.id ===
  LOCAL_DASHBOARD_SERVER_ROUTE || server.id === selectedServerId ||
  Boolean(resourcesByServer[server.id])`. This matches the ticket's framing
  that Off is the only new *client-tracked* gesture.
- **New since this plan was drafted — the remote-child-workroot regression
  fix (merge `6726ded5`) added two more persistent per-server caches that Off
  must also clear, or the deallocated server's stale state resurfaces:**
  - `ws-dashboard/frontend/src/App.tsx:558-568` — `lastNonNullResourcesByServerRef`,
    a per-server last-non-null-resources cache (same `ResourcesByServer`
    shape) backing `resolveActiveResources` (`resourceModel.ts:235-245`),
    which falls back to it whenever `resourcesByServer[selectedServerId]` is
    momentarily absent. If Off deletes `resourcesByServer[serverId]` (see the
    Implementation Plan step 3 bullet already covering this) without also
    deleting `lastNonNullResourcesByServerRef.current[serverId]`, re-Turning
    the SAME server On later (before its first fresh fetch resolves)
    resurrects the stale pre-Off tree through this fallback — a real
    deallocation-correctness gap, not a cosmetic flash. Fix: reuse the new
    `removeResourcesByServer` (step 1) against this ref's `.current` too,
    since it is the same `ResourcesByServer` type.
  - `ws-dashboard/frontend/src/App.tsx:3596-3597` (declared inside
    `WorkbenchShell`) and `App.tsx:5765-5778` (updated/consumed via
    `resolveEffectiveActiveRootKey`, `workbench/openRootLookup.ts:73-87`) —
    `lastActiveRootKeyRef` / `lastActiveRootServerIdRef`, a server-scoped
    "last genuinely-mounted active root" fallback used when
    `selectedWorkRootStateKey` doesn't match any currently mounted
    `openWorkRootInstances` entry for one render. These live INSIDE
    `WorkbenchShell`, not `App()`, so Off's `App()`-level command handler
    cannot reach them directly — they must be reset from inside
    `WorkbenchShell`'s own existing teardown effect (`App.tsx:3623-3714`)
    instead, per the Implementation Plan step 3 addendum below.
- `ws-dashboard/frontend/src/resourceModel.ts:20` —
  `LOCAL_DASHBOARD_SERVER_ROUTE = "server-local"`, the constant to test against
  for the always-On / disabled-Off-button server, already imported by
  `commands.ts`.
- `ws-dashboard/frontend/src/resourceModel.ts:192-209` — `ResourcesByServer`
  type and `mergeResourcesByServer` (accumulate-only merge, unit-tested in
  `resourceModel.test.ts`). No removal counterpart exists yet; Off needs a new
  companion pure function (e.g. `removeResourcesByServer(current, serverId)`)
  next to it, same file, same test-coverage convention.
- `ws-dashboard/frontend/src/App.tsx:496-506` — `openWorkRootKeys` (ordered,
  de-duped `serverScopedIdentity(serverId, rootId)` keys) and
  `openWorkRootRefs` (`Record<rootKey, {rootId, serverRoute}>`), both lifted to
  `App()` so the left panel can read membership and trigger removal. Off must
  filter these by `ref.serverRoute === targetServerId` to find every rootKey
  belonging to the server being turned off.
- `ws-dashboard/frontend/src/App.tsx:1017-1055` — the existing `workRoot.close`
  command handler: the exact single-root teardown recipe (removes the rootKey
  from `openWorkRootKeys`, deletes the entry from `openWorkRootRefs`,
  `workbenchGroupsByRoot`, `paneOrderByRoot`). The comment at
  `App.tsx:1022-1027` confirms unmounting the root's
  `DockviewWorkbenchLayout` instance (by dropping it from `openWorkRootKeys`)
  is what fires the existing xterm dispose/socket-close cleanup — Off should
  batch exactly this over every rootKey belonging to the target server, not
  invent a new teardown mechanism.
- `ws-dashboard/frontend/src/App.tsx:3623-3714` — `WorkbenchShell`'s own
  effect, keyed off diffing `openWorkRootKeys`/`openWorkRootRefs` against a
  snapshot ref via `resolveClosedWorkRootRefs`
  (`ws-dashboard/frontend/src/workbench/openRootLookup.ts:39`), already loops
  over an ARRAY of closed refs (`for (const {rootKey, rootId, serverRoute} of
  closedRefs)`) and clears `terminalPanes`, `agentChatPanes`,
  `activityPaneOpenByRoot`, `activePaneByRoot`, `groupSizeByRoot`,
  `terminalsReadyRootKeys`, `closedAgentPaneByRoot` per rootKey. Batching
  multiple simultaneous rootKey removals (all of one server's open roots at
  once) is already handled by this existing loop — no new teardown code is
  needed in `WorkbenchShell` for the pane-state part of Off. It DOES now need
  one small addition for the `lastActiveRootKeyRef` /
  `lastActiveRootServerIdRef` reset (see the childroot-fix bullets above and
  Implementation Plan step 3).
- `ws-dashboard/frontend/src/App.tsx:512-527,588-599` — the resource-refresh
  coordinator's poll interval calls `fetchResources: () =>
  requestDashboardResources(selectedServerIdRef.current)` every tick,
  regardless of which server that is. **Risk**: if Off targets the
  currently-*selected* server, the next poll tick will refetch that same
  server id and re-merge it into `resourcesByServer` via
  `mergeResourcesByServer`, silently reviving the entry Off just deleted —
  violating the Constraint that "explicit Off is the only path that
  deallocates". Mitigation (mechanical, not a strategy question): when Off
  targets `serverId === selectedServerId`, also refocus to
  `LOCAL_DASHBOARD_SERVER_ROUTE` in the same handler, mirroring
  `handleServerSelected`'s `selectedServerIdRef.current` /
  `setSelectedServerId` / `setSelectedId` triplet (`App.tsx:645-655`).
- `ws-dashboard/frontend/src/App.tsx:5956-5979` — existing "power button"
  visual/interaction precedent: `ChromeIconButton` with `CirclePower` icon and
  a `workbench-power-button-${root.activation}` class suffix, used for
  workRoot online/offline (a different, backend-driven activation concept —
  do not reuse its command, only its icon/class naming convention for the new
  per-server On/Off button).
- `ws-dashboard/frontend/src/App.tsx:9285-9297` — the `workRoot.close` button
  render pattern (`ChromeIconButton` + `onClick={() =>
  onCommand(buildWorkRootCloseCommand(id, actionServerId))}`) — the wiring
  convention to mirror for the new server Off button (a new
  `buildServerOffCommand`-style builder dispatched through the same
  `onCommand`/`executeCommand` pipeline).
- `ws-dashboard/frontend/src/commands.ts:2-46,48-109,357-365,590-619` —
  `DashboardCommandId`/`DashboardCommandPayload` unions, the
  `buildWorkRootCloseCommand` builder (`:357-365`), and the label switch
  (`:614-619`). A new command (e.g. `"server.off"` /
  `{ type: "server.off"; serverId: string }`) belongs in all three spots,
  plus a builder function next to `buildWorkRootCloseCommand`.
- `ws-dashboard/frontend/src/styles.css:2755-2767` — `.server-row-actions` and
  `.server-row-action.icon-button` sizing rules already exist; the new Off
  button belongs in this action group and can reuse the sizing class,
  following the existing `.server-row-action` convention rather than a
  parallel one.

## Implementation Plan

1. `ws-dashboard/frontend/src/resourceModel.ts` (near `mergeResourcesByServer`,
   `:204-209`): add a companion pure function `removeResourcesByServer(current,
   serverId)` that returns a new `ResourcesByServer` with `serverId`'s entry
   deleted (no-op / same-reference return if absent, matching the existing
   style). Add a unit test in `resourceModel.test.ts` (same file that already
   tests `mergeResourcesByServer`) asserting removal only affects the target
   key and leaves other cached servers' entries untouched.
2. `ws-dashboard/frontend/src/commands.ts`: add a new command
   (`DashboardCommandId` union `:2-46`, `DashboardCommandPayload` union
   `:48-109`, e.g. `{ type: "server.off"; serverId: string }`), a
   `buildServerOffCommand(serverId)` builder next to
   `buildWorkRootCloseCommand` (`:357-365`), and a label-switch case (`:614`
   area, e.g. `"Turn server off"`). Extend `commands.test.ts` alongside the
   existing `workRoot.close` coverage (`:130,174,252,503-515`) with equivalent
   builder/label assertions for the new command.
3. `ws-dashboard/frontend/src/App.tsx`: add an `executableHandlers[...]`
   branch for the new command, alongside `workRoot.close` (`:1017-1055`):
   - Read `serverId` from the payload.
   - Guard: no-op if `serverId === LOCAL_DASHBOARD_SERVER_ROUTE` (defense in
     depth; the button itself will also be `disabled`).
   - Compute `keysToRemove` = the set of rootKeys from `openWorkRootRefs` whose
     `ref.serverRoute === serverId`.
   - Batch-filter `openWorkRootKeys` and batch-delete matching entries from
     `openWorkRootRefs`, `workbenchGroupsByRoot`, `paneOrderByRoot` — the same
     four setters `workRoot.close` already uses (`:1028-1054`), just filtering
     by membership in `keysToRemove` instead of equality to one `rootKey`.
   - `setResourcesByServer((current) => removeResourcesByServer(current,
     serverId))` to drop the cached tree (this is what makes `ServerRows`
     collapse the server and stops `findOpenWorkRoot` from resolving anything
     for it, per Phase 1's `resourcesByServer[ref.serverRoute]` lookup at
     `App.tsx:4182`).
   - **Correctness prerequisite (post-childroot-fix `6726ded5`) — Off must
     clear ALL THREE persistent per-server caches that now exist, not just
     `resourcesByServer`, or Off will fail to actually deallocate:**
     1. `resourcesByServer[serverId]` — the bullet immediately above; already
        planned, unchanged.
     2. `lastNonNullResourcesByServerRef.current[serverId]`
        (declared/updated `App.tsx:558-568`) — in this same handler, also run
        `lastNonNullResourcesByServerRef.current =
        removeResourcesByServer(lastNonNullResourcesByServerRef.current,
        serverId)` (reusing the identical step-1 function against the
        identical `ResourcesByServer` shape). Without this,
        `resolveActiveResources` (`resourceModel.ts:235-245`) can resurrect
        the stale pre-Off tree if the same server is re-Turned On before its
        first fresh fetch resolves.
     3. `lastActiveRootKeyRef` / `lastActiveRootServerIdRef`
        (`App.tsx:3596-3597`, consumed at `App.tsx:5765-5778` via
        `resolveEffectiveActiveRootKey`) — these are local to
        `WorkbenchShell`, so this `App()`-level handler cannot reach them
        directly; clear them from inside `WorkbenchShell`'s own teardown
        effect instead (next bullet).
   - If `serverId === selectedServerId`, also refocus to
     `LOCAL_DASHBOARD_SERVER_ROUTE` (mirror `handleServerSelected`,
     `App.tsx:645-655`) to prevent the poll interval from reviving the entry
     just deleted (see Codebase Findings risk above).
   - `WorkbenchShell`'s existing teardown effect (`:3623-3714`) already loops
     over however many rootKeys disappeared from `openWorkRootKeys` in one
     commit (`closedRefs`, from `resolveClosedWorkRootRefs`) and clears
     `terminalPanes`/`agentChatPanes`/etc per rootKey — no new mechanism is
     needed there for that part of Off. It DOES need one small addition: after
     that existing per-`closedRefs` loop, if `lastActiveRootKeyRef.current`
     matches any `closedRefs[].rootKey`, reset both
     `lastActiveRootKeyRef.current` and `lastActiveRootServerIdRef.current` to
     `null`. This is a one-arm extension of an already-batching effect (fires
     identically for a single `workRoot.close` or a batched server Off), not
     new plumbing, and is what makes prerequisite (3) above actually happen.
4. `ws-dashboard/frontend/src/App.tsx` (`ResourceNavigation` call site,
   `:1192-1210`): add a `resourcesByServer={resourcesByServer}` prop, mirroring
   the identical prop already passed to `WorkbenchShell` a few lines below
   (`:1243`).
5. `ResourceNavigation` (`:2680-2797`) and `ServerRows` (`:2799-2895`):
   - Accept the new `resourcesByServer` prop through `ResourceNavigation` and
     pass each server's OWN entry into `ServerRows` (replacing the
     `server.id === selectedServerId ? resources : null` ternary at `:2770`
     with a lookup into `resourcesByServer[server.id] ?? null`).
   - Add an `isOn` prop/derivation on `ServerRows`: `server.id ===
     LOCAL_DASHBOARD_SERVER_ROUTE || server.id === selectedServerId ||
     Boolean(resourcesByServer[server.id])` (compute in `ResourceNavigation`
     or inline in `ServerRows`, whichever keeps the diff smaller).
   - Change the expansion guard at `:2879` from `{selected && resources ? ...}`
     to `{isOn && resources ? ...}`.
6. `ServerRows`' `.server-row-actions` (`:2856-2877`): add a new
   `ChromeIconButton` (reusing the `.server-row-action` sizing class,
   `styles.css:2761-2767`) for Off:
   - `disabled={server.id === LOCAL_DASHBOARD_SERVER_ROUTE}`.
   - Icon/class naming mirrors the existing workRoot power-button convention
     (`CirclePower` + a `-${state}` class suffix, `App.tsx:5956-5979`), e.g.
     `server-row-power-button-${isOn ? "on" : "off"}`.
   - `onClick`: dispatch `onCommand(buildServerOffCommand(server.id))`. Only
     meaningful while On (the button is the Off-only gesture per the ticket —
     turning On is still exclusively the label-click path, so the button need
     not handle an On-click case).
7. `ws-dashboard/frontend/src/styles.css` (near `:2755-2767`): add whatever
   minimal `.server-row-power-button-on` / `-off` styling is needed (or reuse
   the existing `workbench-power-button` rule if it generalizes cleanly)
   consistent with the rest of `.server-row-action`.

## Verification Plan

- `npm run build` (typecheck across the new command/prop plumbing).
- `npm run test:resource-model` (covers the new `removeResourcesByServer` unit
  test alongside existing `mergeResourcesByServer`/refresh/linked-server
  tests).
- `npm run test:commands` (covers the new command builder + label).
- `npm run test:workbench` (regression guard: `findOpenWorkRoot`/mount-gating
  from Phase 1 must stay unaffected by this phase).
- Manual / deferred: the ticket's own verification boundary — open two
  servers, start a terminal in each, switch focus confirming both survive,
  then Off the second confirming only its surfaces release, and confirm
  `server-local`'s Off button is disabled — via the headless-Playwright method
  in `ai-docs/ref/dashboard-headless-browser-verification.md`. Defer to live
  dogfooding, consistent with the Phase 1 precedent (its own
  headless-Playwright verification boundary was likewise deferred, per the
  Phase 1 Result note in the ticket).
- Manual / deferred, additional case surfaced by the childroot-fix
  prerequisite above: Off a server, then re-Turn it On (label click) again
  before its fresh fetch would plausibly resolve, and confirm the tree shown
  is either empty/loading or the fresh tree — never the stale pre-Off tree.
  This exercises `lastNonNullResourcesByServerRef` actually being cleared, not
  just `resourcesByServer`.

## Escalations

- None.
