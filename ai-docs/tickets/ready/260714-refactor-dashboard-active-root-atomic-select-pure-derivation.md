---
title: "Refactor dashboard active-work-root selection into one atomic selectRoot action + a pure, mount-by-construction derivation (kill the 4-regression bug class)"
related:
  260714-idea-dashboard-workbench-active-root-derivation-fragility: fragility record this design answers - keep that idea ticket as the standing evidence log; this is its actionable follow-up
  260714-bug-dashboard-childroot-workbench-flash-hide: failure mode #1 (activeResources transient collapse) - must stay eliminated
  260714-bug-dashboard-select-server-switch-mount-gap-flash-hide: failure mode #3 (server-switch mount gap) - the synchronous-mount hack this design replaces structurally
  260714-bug-dashboard-worktree-label-click-requires-server-focus: failure mode #2 fix that introduced #3; its serverId-threading is subsumed by the atomic selectRoot entry point
  260714-feat-dashboard-multi-server-workbench-keepalive: origin of the cluster (cross-server hide-not-unmount); this refactor must preserve its keep-alive behavior
---

# Refactor dashboard active-work-root selection into one atomic action + pure derivation

## Background

The "which work root is currently visible" cluster in
`ws-dashboard/frontend/src` produced FOUR regressions in one cycle, each
individually reviewed-and-approved yet collectively fragile. The failure is at
the design level, not in any single fix. See
`260714-idea-dashboard-workbench-active-root-derivation-fragility` for the
standing evidence log; this ticket is the actionable design that replaces the
cluster.

### The four failure modes (acceptance oracle)

1. **childroot collapse** (`260714-bug-dashboard-childroot-workbench-flash-hide`):
   `activeResources = resourcesByServer[selectedServerId]` transiently goes
   `null` for one render (open-to-canonical-refetch window), collapsing
   `resolveWorkbenchSelection` to `null`, zeroing `selectedWorkRootStateKey`,
   so `isActiveRoot` is false for every mounted instance at once ->
   `display:none` on all + `dv-watermark`. Patched with
   `lastNonNullResourcesByServerRef` (a per-server last-good cache) plus a
   `lastActiveRootKeyRef` render-time safety net.
2. **cross-server content leak** (Edition on the childroot ticket): the
   `lastActiveRootKeyRef` safety net re-pinned server A's live keep-alive panes
   under a freshly-selected unresolved server B's header. Patched by
   server-scoping the fallback in `resolveEffectiveActiveRootKey`
   (`lastActiveRootServerId === selectedServerId`).
3. **server-switch mount gap**
   (`260714-bug-dashboard-select-server-switch-mount-gap-flash-hide`): the
   `resource.select` handler flips `selectedServerId`+`selectedId` in one
   commit, but the selected root enters `openWorkRootKeys`/`openWorkRootRefs`
   only via the `[workbenchSelection]` effect one render later. On the
   intervening render `selectedRootIsMounted` is false and the server-scoped
   guard sees the stale previous server -> `resolveEffectiveActiveRootKey`
   returns `null` -> full `dv-watermark` flash. Patched by replicating the
   effect's mount logic synchronously inside the handler
   (`App.tsx:1017-1041`).
4. **linked-server open mount gap** (freshly root-caused, currently
   UNPATCHED): `handleWorkRootOpened` (`App.tsx:606-635`) sets
   `selectedServerIdRef`/`setSelectedServerId`/`applyExternalResources`/
   `setSelectedId` in one commit but never calls
   `withOpenWorkRootKey`/`withOpenWorkRootRef`. On the intervening render the
   root is unmounted and `lastActiveRootServerId` is the previous server, so
   `resolveEffectiveActiveRootKey`'s server-scoped guard fails and every pane
   renders `display:none` behind `dv-watermark`. Same class as #3, different
   entry point - direct evidence that per-entry-point mount seeding does not
   scale.

### Structural root cause (confirmed against the real code)

The visible root is decided by loosely-coupled state that updates at different
times with no transaction, and the two derivations that must agree read from
GENUINELY DIFFERENT data paths:

- The single-slot path (`activeResources` -> `resolveWorkbenchSelection` ->
  `selectedWorkRootStateKey`, `App.tsx:569-573,792-795,3759-3764`) reads
  `resolveActiveResources(...)` WITH the `lastNonNullResourcesByServerRef`
  fallback.
- The per-instance path (`openWorkRootInstances`, `App.tsx:4378-4404`) resolves
  each open root against raw `resourcesByServer[ref.serverRoute]` via
  `findOpenWorkRoot`, WITHOUT that fallback.
- Membership in `openWorkRootInstances` is seeded by an async effect keyed on
  `[workbenchSelection]` (`App.tsx:796-828`), so it lags selection by one
  render at EVERY entry point.
- The gap is papered over by render-time mutable refs
  (`lastActiveRootKeyRef`/`lastActiveRootServerIdRef`, `App.tsx:3781-3782`,
  `5968-5981`) that only advance when the root already happens to be mounted,
  so they are stale in exactly the gap they are meant to cover -> the fallback
  under-fires (watermark) or over-fires (leak).

Every new entry point (label click, picker open, linked-server open) must
independently avoid tripping this gap. Four instances in one cycle is the
signal that a per-entry-point discipline cannot hold; the fix must be
structural.

## Decisions

**D1 - Mount the selected root BY CONSTRUCTION, not by effect.** Derive
`openWorkRootInstances` from the UNION of persisted `openWorkRootKeys` and the
currently-selected root's key, computed at render time. The selected root is
therefore always present as a mounted instance in the SAME render as the
selection change, with no dependency on any effect having run. `openWorkRootKeys`
reverts to its true job (keep-alive membership of previously-visited roots); the
selection effect demotes to a lazy persister of the selected key into that set
(needed only so the root stays mounted after the user switches AWAY), which is
no longer on the visible-collapse critical path.

The union MUST use `withOpenWorkRootKey`'s semantics exactly
(`openRootLookup.ts:79-86`, already unit-tested): append-if-absent,
POSITION-PRESERVING - an already-present selected key keeps its existing slot,
and only a genuinely new key is appended. Do NOT filter-then-append (that would
move an already-open selected root to the end of the list and remount its live
Dockview instance, losing terminal/editor state - the exact class of harm this
refactor exists to avoid). Bind the union to that primitive by name.
`deriveWorkbenchView` calls `withOpenWorkRootKey(openWorkRootKeys,
selectedRootKey)` to produce `openInstanceKeys`. The selected entry's
`{rootId, serverRoute}` is taken from `selection.root` (the freshly resolved
`resolveWorkbenchSelection` result), NOT from the one-render-lagging
`openWorkRootRefs` map - so the selected instance can resolve on the mount
render even before its ref is persisted.

**D2 - Resolve the selected instance through the SAME fallback path as
`activeResources`.** The union's selected-root entry resolves against
`resolveActiveResources(resourcesByServer, selectedServerId, lastNonNull...)`
(the fallback-bearing path), not raw `resourcesByServer[serverRoute]`. This
closes the "two derivations, two data paths" split that made failure mode #1
reachable: the selected root cannot be filtered out of the mounted set by a
transient raw-map gap. Keep-alive (non-selected) instances continue resolving
against the raw per-server map as today - they were never the collapse source.

**D3 - `effectiveActiveRootKey` becomes pure and ref-free.** Because the
selected root is mounted by construction (D1) and resolvable through the
fallback (D2), `effectiveActiveRootKey` is simply the selected root's key when
the selection resolves, else `null`. Delete `lastActiveRootKeyRef`,
`lastActiveRootServerIdRef`, their advance block (`App.tsx:5971-5974`), their
close-time reset (`App.tsx:3899-3916`), and the fallback branches of
`resolveEffectiveActiveRootKey`. With no cross-render mutable memory there is
nothing to leak (mode #2) and nothing to go stale (modes #3/#4). `null` on an
unresolved server correctly shows the empty-state watermark (mode #2's intended
behavior) without a guard.

**D4 - One atomic `selectRoot(serverId, entityId)` action = the single
selection entry point.** It sets `selectedServerIdRef.current`,
`setSelectedServerId`, and `setSelectedId` together in one commit (the
"selectedServerId triple" currently duplicated across `handleServerSelected`,
`applyServerConnection`, the `resource.select` handler, `handleWorkRootOpened`,
and the `resourcesByServer` normalize effect). Every current caller routes
through it. Because D1/D2/D3 make mounting a pure consequence of committed
selection state, `selectRoot` does NOT need to hand-seed
`openWorkRootKeys`/`openWorkRootRefs` (the mount hack added by mode #3's fix at
`App.tsx:1017-1041` is deleted, not moved). `selectRoot` accepts a
non-work-root `entityId` (server row, workspace row) and simply sets selection
- resolution to a concrete root stays in `resolveWorkbenchSelection`.

**D5 - `lastNonNullResourcesByServer` stays.** It addresses a genuinely
different concern (resource-fetch gaps, mode #1's upstream cause) and is already
pure + unit-tested. It is the ONE piece of last-good memory the design keeps; it
is keyed by server and cannot pin one server's content under another. Whether it
remains a render-time write-through ref or is promoted to explicit state is left
to Phase 3 (behaviorally identical; promotion only removes the render-mutation
smell). Do not conflate it with the `lastActiveRootKey` refs being deleted.

**D6 - Test harness = pure single-render derivation seam.** No jsdom / RTL /
Playwright. The project's test rig is `tsc` + node over pure-logic modules
(`test:workbench`, `test:resource-model`). Extract one pure function
`deriveWorkbenchView(committedState)` -> `{ activeResources, selectedRootKey,
openWorkRootInstances (keys), effectiveActiveRootKey }`, taking the exact
committed state slice of a single render. The four regressions become
committed-state fixtures asserting the view never collapses in the intervening
render. Add a second pure reducer-style unit for `selectRoot(state, input) ->
nextState` asserting the triple advances atomically. This is the cheapest seam
that would have caught all four.

Oracle LIMIT (state it, do not overclaim): `deriveWorkbenchView` fixtures cover
the four watermark/collapse modes as single-render committed-state assertions,
but a pure function CANNOT observe (a) whether React actually remounts vs reuses
a Dockview instance across renders (a reconciliation property of keyed children,
not of the derived data) nor (b) the D7 restore flash (a paint-timing property).
Both are manual-dogfooding-only. The closest automated PROXY for (a) is a
cross-render order-stability unit: assert `deriveWorkbenchView(stateN).openInstanceKeys`
is a POSITION-PRESERVING PREFIX of `deriveWorkbenchView(stateN+1).openInstanceKeys`
across a selection change (append-only, no reorder) - a violation is the signal
that a live instance would remount. Add that unit in Phase 1.

**D8 - Left-nav `isOpenWorkRoot` badge one-render lag is a known-accepted
cosmetic transient.** The badge reads `openWorkRootKeys.has(...)` directly
(`App.tsx:9328`, `9377`), and under D1 the selected root is mounted by the union
one render before it is persisted into `openWorkRootKeys` by the demoted effect.
So on the mount render the workbench is correctly visible while the left-nav
"open" badge is not yet lit, for exactly one frame. This is cosmetic (a nav
indicator, not the workbench content), self-corrects the next render, and does
NOT belong to the watermark bug class. Accepted as-is; not worth threading the
union result into the badge. (If a reviewer wants it gone, the badge could read
`openWorkRootKeys.has(key) || key === selectedRootKey` - noted, not planned.)

## Constraints

- SCOPE-BOUNDED to the derivation/selection cluster: `App.tsx` selection
  handlers + `WorkbenchShell` active-root derivation, `workbench/openRootLookup.ts`,
  and the pure helpers in `resourceModel.ts`. Do NOT rewrite workbench
  rendering, Dockview integration, pane lifecycle, or the resource-fetch layer.
- Dockview instance identity is keyed by `rootKey`; the union MUST be
  deduplicated and order-stable so no mounted instance remounts (would lose
  terminal/editor state). The selected root, when already in `openWorkRootKeys`,
  keeps its existing position; when absent it is appended.
- Preserve `260714-feat-dashboard-multi-server-workbench-keepalive`: switching
  focus must not unmount other On servers' panes. The union only ADDS the
  selected root; it never drops keep-alive members.
- Preserve the `server.off` teardown contract (`removeResourcesByServer` +
  `resolveClosedWorkRootRefs`); deleting the `lastActiveRootKey` refs removes
  their reset block but must not touch the close/off cleanup of
  `openWorkRootKeys`/`openWorkRootRefs`/`resourcesByServer`.
- Out-of-cluster change to call out: `handleWorkRootOpened`,
  `handleServerSelected`, and `applyServerConnection` currently own their own
  copy of the selectedServerId triple; routing them through `selectRoot` is a
  minimal, in-cluster edit to those callbacks (no signature change to their own
  callers). No backend, command-bus wire-shape, or spec contract change is
  required - the optional `serverId` field added by failure mode #2's fix stays.

## Traceability - each failure mode -> the structural property that prevents it

| # | Failure mode | Structural property that now prevents it |
|---|---|---|
| 1 | childroot `activeResources` transient collapse hides all panes | D2: selected instance resolves through the fallback-bearing `resolveActiveResources` path, the SAME source as `activeResources`; the raw-map/single-slot data-path split that let it be filtered out is closed. D5 keeps the per-server last-good cache that keeps `activeResources` non-null across the fetch gap. |
| 2 | cross-server keep-alive content leak under a new server's header | D3: `lastActiveRootKeyRef`/`lastActiveRootServerIdRef` are DELETED. With no cross-render mutable active-root memory, there is no previous-server key to re-pin; an unresolved server resolves `effectiveActiveRootKey = null` -> watermark, by construction, no guard. |
| 3 | server-switch select mount-gap watermark flash | D1: `openWorkRootInstances` includes the selected root by render-time union, so it is mounted in the SAME commit as the server+selection flip - there is no "selected-but-not-mounted" render. The synchronous-mount hack (`App.tsx:1017-1041`) is deleted as redundant. |
| 4 | linked-server `handleWorkRootOpened` open mount-gap watermark | D1 (same mechanism as #3, independent of entry point) + D4: routing `handleWorkRootOpened` through the single `selectRoot` action means no entry point can omit mounting, because mounting is no longer a per-entry-point step at all - it is a pure consequence of committed selection state. |

All four map to a structural property; none is left to per-call discipline. No
failure mode is unprevented. Residual risk is limited to correct
implementation of the union dedupe/order (Constraint above) and to the one
gap the design deliberately does NOT invent new memory for: selecting a server
whose tree has never resolved still shows the watermark until its first fetch
lands - which is the correct, intended empty state, not a regression.

## Phases

### Phase 1: Pure `deriveWorkbenchView` seam + mount-by-construction + delete active-root refs

Introduce the pure derivation and the render-time union together (they are
coupled: the union is what lets the refs be removed safely; removing the refs
before the union would reopen modes #1-#4).

- Add `deriveWorkbenchView(committedState)` to `workbench/openRootLookup.ts`
  (or a sibling `workbench/workbenchView.ts` if `openRootLookup.ts` grows too
  broad), taking `{ resourcesByServer, lastNonNullResourcesByServer,
  selectedServerId, selectedId, openWorkRootKeys, openWorkRootRefs }` and
  returning `{ activeResources, selectedRootKey, openInstanceKeys (union,
  deduped, order-stable), effectiveActiveRootKey }`. Reuse
  `resolveActiveResources`, `resolveWorkbenchSelection`/`findOpenWorkRoot`, and
  `serverScopedIdentity`. Build `openInstanceKeys` as
  `withOpenWorkRootKey(openWorkRootKeys, selectedRootKey)` (D1: append-if-absent,
  position-preserving, bound to the tested primitive - never filter-then-append),
  and resolve the selected entry's `{rootId, serverRoute}` from `selection.root`,
  not the lagging `openWorkRootRefs`.
- Rewire `WorkbenchShell` to build `openWorkRootInstances` from
  `openInstanceKeys` (union incl. the selected root; selected-root entry
  resolved via `activeResources` per D2) and set
  `effectiveActiveRootKey = selectedRootKey` (D3).
- Add the D7 restore-layout fallback: extract pure `resolveRootLayout(rootKey,
  workbenchGroupsByRoot, paneOrderByRoot, activePaneByRoot, restoreSnapshot)`
  and route `buildEditorGroupsForRoot`'s `groupsForRoot`/`paneOrderForRoot`
  reads (`App.tsx:4269-4270`) and the per-instance `effectiveActivePaneByGroup`
  read (`App.tsx:5988`) through it, so a restored root shows its restored layout
  on the SAME render the union mounts it. Leave the stateful seeding effects
  (`App.tsx:815-826`, `3826-3835`) in place.
- Delete `lastActiveRootKeyRef`, `lastActiveRootServerIdRef`, the advance block
  (`App.tsx:5971-5974`), the close-time reset (`App.tsx:3899-3916`), and the
  fallback branches of `resolveEffectiveActiveRootKey` (reduce it to a pure
  identity or remove it if `deriveWorkbenchView` subsumes it).
- Tests (the oracle): new `workbench/*.test.ts` wired into `test:workbench`:
  (a) one committed-state fixture per failure mode #1-#4 asserting the derived
  view does not collapse (selected instance present, `effectiveActiveRootKey`
  non-null) on the intervening render; (b) the mode-#2 unresolved-server case
  asserting `effectiveActiveRootKey === null` (watermark, no leak); (c) the
  keep-alive case asserting non-selected members survive and do not reorder;
  (d) the cross-render order-stability proxy (D6): `openInstanceKeys(stateN)`
  is a position-preserving prefix of `openInstanceKeys(stateN+1)` across a
  selection change; (e) `resolveRootLayout` fixture asserting a RESTORED root's
  derived `groups`/`paneOrderByGroup`/`activePaneByGroup` are the restored
  (non-default) values even when the three layout state maps are still empty
  for that key (the D7 mount-render fixture).

Verification boundary: `npm run build`, `npm run test:workbench`,
`test:resource-model`, `test:commands`, `test:open-work-root` all pass. Live
dogfooding remains the final gate for the properties the pure oracle cannot
observe (D6 LIMIT): actual Dockview remount-vs-reuse across renders and the D7
restore paint-timing. No automated render harness exists - a known, accepted
limitation carried from the four prior fixes.

### Result (ddd353fe) - 2026-07-14

Replaced the ref-backed active-root safety net with a pure render-time
derivation. `deriveWorkbenchView` (workbench/openRootLookup.ts) resolves
`activeResources` via the fallback-bearing `resolveActiveResources` (D2), folds
the freshly-resolved selected root into `openWorkRootKeys` via
`withOpenWorkRootKey` (append-if-absent, position-preserving, D1), and returns a
pure `effectiveActiveRootKey` (selected key or null, D3). `WorkbenchShell` maps
`openWorkRootInstances` over that union and resolves the selected entry from
`selection` (not the lagging `openWorkRootRefs`); keep-alive members keep the
raw `findOpenWorkRoot` path. `lastActiveRootKeyRef`/`lastActiveRootServerIdRef`
(with their advance and close-time reset blocks) and `resolveEffectiveActiveRootKey`
are deleted. `resolveRootLayout` (workbench/layoutRestore.ts, D7) gives a
freshly-mounted restored root its restored layout on the same render
(precedence state -> restore -> default). `WorkbenchSelection`,
`resolveWorkbenchSelection`, `isWorkspaceNavChildWorkRoot`, and `findInstanceById`
were relocated App.tsx -> resourceModel.ts (verbatim, no logic drift) to break
the circular import. The render-time union and the ref-deletion landed in one
commit per the load-bearing ordering constraint.

All four failure modes are structurally prevented (partitioned review confirmed
against the committed code, incl. mode #4 by construction since the seam reads
committed `selectedServerId`/`selectedId`).

Verification: `npm run build` clean; `test:workbench`, `test:resource-model`,
`test:commands`, `test:open-work-root` all exit 0. New fixtures a-e added and
wired into the explicit `test:workbench` script (confirmed executing).

Review: partitioned correctness (opus) / fit (opus) / test (sonnet) - all clean,
no Critical/Important. Minors accepted: stale comments inside the Phase-2-deferred
sync-mount hack and a Phase-3-deferred comment (left in place per plan).

Deferred: Phase 2 (atomic `selectRoot` + route all entry points + delete the
sync-mount hack), Phase 3 (cleanup, optional `lastNonNullResourcesByServer` state
promotion, doc/spec + mental-model invariant note). D6 LIMIT: live dogfooding
remains the final gate for actual Dockview remount-vs-reuse and D7 restore
paint-timing (outside the pure oracle's scope).

### Phase 2: Single atomic `selectRoot` action; route all entry points; shrink the selection effect

- Add `selectRoot(serverId, entityId)` (a `useCallback` in `App()`) that sets
  the selectedServerId triple in one commit. Route through it: the
  `resource.select` handler (`App.tsx:983-1044`, deleting the synchronous-mount
  hack `1017-1041`), `handleWorkRootOpened` (`606-635` - fixes failure mode #4),
  `handleServerSelected` (`650-660`), `applyServerConnection` (`662-684`), and
  the `resourcesByServer` normalize effect (`637-648`) where it re-derives the
  server id.
- Demote the `[workbenchSelection]` effect (`796-828`) to: (a) lazily persist
  the selected root's key into `openWorkRootKeys`/`openWorkRootRefs` for
  keep-alive after switch-away (via existing `withOpenWorkRootKey`/
  `withOpenWorkRootRef`), and (b) keep the layout-restore seeding untouched.
  This is no longer on the visible-collapse path.
- Tests: pure reducer unit `selectRoot(state, input) -> nextState` asserting the
  triple advances atomically and a non-work-root `entityId` sets selection
  without inventing a mount; extend `commands.test.ts` if the `resource.select`
  dispatch shape shifts (it should not - the optional `serverId` field stays).

Verification boundary: same suites as Phase 1, plus manual confirmation that
label-click / picker-open / linked-server-open all select in one gesture with
no watermark flash.

### Phase 3: Cleanup + optional state promotion + doc/spec touch

- Remove now-dead code: the duplicated mount logic, any unused branch of
  `resolveEffectiveActiveRootKey`, and stale comments referencing the deleted
  refs across `App.tsx` / `openRootLookup.ts`.
- OPTIONAL (reviewer's call): promote `lastNonNullResourcesByServer` from a
  render-time write-through ref to explicit state, removing the last
  render-time mutation smell (behaviorally identical; D5).
- Update the keep-alive spec note under
  `{#260714-ws-dashboard-cross-server-workbench-keepalive}` in
  `ai-docs/spec/ws-web-dashboard/index.md` only if the derivation's
  caller-visible contract changed (expected: no new contract - this restores
  intended behavior more robustly). Fold the fragility record's disposition
  back into `260714-idea-...-fragility` (leave that idea ticket as the standing
  evidence log; do not delete it).

## Migration risk, ordering, rollback

- **Ordering is load-bearing.** Phase 1 must land the union AND the ref-deletion
  in one reviewable unit; splitting them (refs removed before the union exists)
  transiently reopens modes #1-#4. Phase 2 (selectRoot consolidation) depends on
  Phase 1's mount-by-construction so it can delete rather than relocate the
  mount hack. Phase 3 is pure cleanup.
- **Top risk: instance remount / order churn.** If the union reorders or
  duplicates `rootKey`s, mounted Dockview instances remount and lose live
  terminal/editor state - worse than the flash it replaces. Mitigated by binding
  the union to `withOpenWorkRootKey` (D1: append-if-absent, position-preserving),
  the cross-render prefix-stability proxy unit (D6), and live dogfooding for the
  reconciliation property the pure oracle cannot see.
- **Second risk (new defect this design would otherwise add): restore-layout
  flash** - D7. Under D1 the union mounts a restored root one render before the
  layout-seed effects run, so without the render-time restore fallback a restored
  root flashes the default layout for one frame. Mitigated by `resolveRootLayout`
  reading `workbenchLayoutRestoreRef` at render (D7) with a mount-render fixture;
  the paint-timing itself is dogfood-verified.
- **Third risk: keep-alive regression** - if the demoted effect fails to
  persist the selected key, switching away unmounts the root. Covered by a
  switch-away fixture in Phase 1/2 tests.
- **Accepted cosmetic transient:** the left-nav `isOpenWorkRoot` badge lags the
  workbench by one render (D8); not a bug-class member, self-corrects next
  render.
- **Rollback:** each phase is one commit. Phase 1 is the only correctness-
  bearing change; reverting its single commit restores the current
  four-fix state exactly (the prior refs/guards are deleted in that same
  commit, so a revert brings them back atomically). Phases 2-3 are refactors
  that revert independently without reopening any failure mode.
- **Known verification limitation (D6 LIMIT):** no render/DOM harness exists;
  the pure `deriveWorkbenchView` + `resolveRootLayout` fixtures are the automated
  oracle for the collapse modes and layout resolution, but two properties are
  manual-dogfooding-only: (a) actual Dockview remount-vs-reuse across renders (a
  React reconciliation property), for which the prefix-stability unit is the
  closest automated proxy, and (b) the D7 restore paint-timing. Live remote
  dogfooding remains the final gate, consistent with all four prior fixes.
