# Plan: 260714-refactor-dashboard-active-root-atomic-select-pure-derivation — Phase 2: Single atomic selectRoot action; route all entry points; shrink the selection effect

## Relevant Ticket Contract

- D4: add `selectRoot(serverId, entityId)`, a single atomic action that sets the
  "selectedServerId triple" (`selectedServerIdRef.current`, `setSelectedServerId`,
  `setSelectedId`) in one commit. Every current caller routes through it.
  `selectRoot` does NOT hand-seed `openWorkRootKeys`/`openWorkRootRefs` — Phase 1's
  mount-by-construction (`deriveWorkbenchView`) already makes mounting a pure
  consequence of committed selection, so the old sync-mount hack is deleted, not
  moved. `selectRoot` accepts a non-work-root `entityId` (server row, workspace
  row) and just commits selection; resolving a concrete root stays in
  `resolveWorkbenchSelection`.
- Phase 2 bullet list (ticket lines 321-336): route `resource.select`
  (deleting the sync-mount hack), `handleWorkRootOpened` (fixes failure mode #4),
  `handleServerSelected`, `applyServerConnection`, and the `resourcesByServer`
  normalize effect through `selectRoot`. Demote the `[workbenchSelection]` effect
  to (a) lazy keep-alive persistence via existing `withOpenWorkRootKey`/
  `withOpenWorkRootRef`, (b) untouched layout-restore seeding. Add a pure reducer
  unit `selectRoot(state, input) -> nextState` asserting the triple advances
  atomically, plus a non-work-root `entityId` case. Extend `commands.test.ts`
  only if the `resource.select` dispatch shape shifts (it should not — optional
  `serverId` field stays).
- Constraints: SCOPE-BOUNDED to `App.tsx` selection handlers + `WorkbenchShell`
  active-root derivation, `workbench/openRootLookup.ts`, and `resourceModel.ts`
  pure helpers. No backend/command-bus/spec contract change required.
- Verification boundary: `npm run build`, `test:workbench`, `test:resource-model`,
  `test:commands`, `test:open-work-root` all pass, plus manual confirmation that
  label-click / picker-open / linked-server-open all select in one gesture with
  no watermark flash.

## Out of Scope

- Phase 3 (dead-code sweep beyond this phase's direct edits, optional
  `lastNonNullResourcesByServer` state promotion, spec/mental-model doc touch).
- Any rewrite of workbench rendering, Dockview integration, pane lifecycle, or
  the resource-fetch layer.
- `WorkbenchShell`'s active-root consumption (`openWorkRootInstances`,
  `effectiveActiveRootKey` at `App.tsx:4434-4445`, `6054-6060`) — Phase 1 already
  landed the pure union/derivation; Phase 2 does not touch it.
- The `260714-bug-linked-terminal-ws-relay-502` Phase-2-Prong-1 sticky-selection
  machinery (`driveStickyWorkbenchSelection`, `stickyWorkbenchSelectionRef`,
  `resolveStickyWorkbenchSelection`) — already landed on this branch, orthogonal
  to this ticket, and must be left intact and consumed as-is (see Codebase
  Findings).

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L1046-L1110` — `executeCommand`'s
  `"resource.select"` branch is the current sync-mount hack (ticket's
  `App.tsx:1017-1041` reference, shifted by Phase 1 + the sticky-selection
  commits landed since). When `serverId` is present and differs from
  `selectedServerIdRef.current`, it manually resolves `targetSelection` via
  `resolveActiveResources` + `resolveWorkbenchSelection` and seeds
  `openWorkRootKeys`/`openWorkRootRefs` synchronously via
  `withOpenWorkRootKey`/`withOpenWorkRootRef` (lines 1083-1107). This whole block
  is now redundant: Phase 1's `deriveWorkbenchView` already folds the
  freshly-resolved selected root into the union at render time regardless of
  whether `openWorkRootKeys`/`openWorkRootRefs` state has caught up, so manually
  seeding those refs here no longer changes what's mounted. Delete the block,
  replacing it with a single `selectRoot(serverId ?? selectedServerIdRef.current, entityId)`
  call (collapsing the branch entirely — no need to test `serverId !==
  selectedServerIdRef.current` since setting state to its current value is an
  inert no-op).
- `ws-dashboard/frontend/src/App.tsx#L1373-L1380` — `executeCommand`'s
  `useCallback` dependency array includes `resourcesByServer` solely for the
  deleted block's `resolveActiveResources` call (confirmed via grep: no other
  use of `resourcesByServer` inside `executeCommand`, `L1046-1382`). Remove
  `resourcesByServer` from this dependency array once the block is deleted.
  `openWorkRootRefs` stays — it is read elsewhere in `executeCommand` (a
  `workRoot.close`-style branch around `L1198`).
- `ws-dashboard/frontend/src/App.tsx#L271` — `resolveWorkbenchSelection` is
  imported from `resourceModel.js` and, after the deletion above, has no
  remaining call site in `App.tsx` (only comment references remain). Remove the
  import. `tsconfig.app.json` has no `noUnusedLocals`, so this won't fail
  `npm run build`, but leaving a dead import contradicts the phase's "shrink"
  intent.
- `ws-dashboard/frontend/src/App.tsx#L669-L698` — `handleWorkRootOpened`: sets
  the triple across two statements (`selectedServerIdRef.current =
  openedView.server.id; setSelectedServerId(openedView.server.id);`
  unconditionally, then `setSelectedId(openedWorkRootId)` only `if
  (openedWorkRootId)`). This is failure mode #4's entry point. Route through
  `selectRoot`: when `openedWorkRootId` resolves, call
  `selectRoot(openedView.server.id, openedWorkRootId)`; when it does not
  resolve (no newly-opened workRoot identified), current behavior intentionally
  leaves `selectedId` untouched while still switching the server — preserve that
  by falling back to the current `selectedId` as the entity argument (e.g.
  `selectRoot(openedView.server.id, openedWorkRootId ?? selectedId)`), which
  reproduces today's "no-op on `selectedId`" behavior since setting state to its
  existing value is inert. This requires `selectedId` in scope inside this
  `useCallback` (add to its dependency array, currently `[loadResources,
  loadServers, activeResources]`).
- `ws-dashboard/frontend/src/App.tsx#L713-L723` — `handleServerSelected`: triple
  is `selectedServerIdRef.current = server.id; setSelectedServerId(server.id);
  setSelectedId(server.id);` then conditional `loadResources("explicit")` if
  connected. Replace the triple with `selectRoot(server.id, server.id)`; keep
  the `loadResources` conditional untouched.
- `ws-dashboard/frontend/src/App.tsx#L725-L747` — `applyServerConnection`: same
  triple shape as `handleServerSelected`, interleaved with `setServersView` (line
  727-737, unrelated — keep) and `loadServers()`/conditional `loadResources`
  (keep). Replace the triple with `selectRoot(server.id, server.id)`.
- `ws-dashboard/frontend/src/App.tsx#L700-L711` — the `resourcesByServer`
  normalize effect is a **partial** duplication, not a full triple: it only sets
  `selectedServerIdRef.current`/`setSelectedServerId` when
  `resources.server.id !== selectedServerIdRef.current`; it never calls
  `setSelectedId`. Routing this through `selectRoot(serverId, entityId)`
  (which always commits both fields) requires passing the CURRENT `selectedId`
  as `entityId` to reproduce a no-op on that field — same trick as
  `handleWorkRootOpened` above. That means adding `selectedId` to this effect's
  dependency array (currently `[resourcesByServer]`) and calling
  `selectRoot(resources.server.id, selectedId)` unconditionally (the inner
  `if` guard can be dropped since `normalizeServerRoute(resources.server.id)`
  already runs unconditionally today whenever `resources` exists — the `if`
  only gated the state-setting, and idempotent re-sets are harmless). Flag this
  in the implementation as a deliberate behavior-preserving generalization, not
  a silent scope expansion.
- `ws-dashboard/frontend/src/App.tsx#L1265-L1275` — **risk signal, not in the
  ticket's enumerated caller list**: the `server.off` command handler has its
  own full triple (`selectedServerIdRef.current = LOCAL_DASHBOARD_SERVER_ROUTE;
  setSelectedServerId(...); setSelectedId(...)`) guarded by `if (serverId ===
  selectedServerIdRef.current)`, with a comment explicitly stating it "Mirrors
  `handleServerSelected`'s refocus triplet." This is the same duplicated shape
  D4 targets, but neither the ticket's Phase 2 bullet list nor its Constraints
  section names it. It predates the active-root refactor ticket's authoring
  (`ce6c7350`, the On/Off lifecycle feature, landed before `a6fc08e2`, the
  fragility idea ticket). Routing it through
  `selectRoot(LOCAL_DASHBOARD_SERVER_ROUTE, LOCAL_DASHBOARD_SERVER_ROUTE)` is a
  pure consolidation (identical resulting state), but since the ticket text
  doesn't call it out, treat it as optional/confirm-first rather than mandatory
  — see the implementation step below.
- `ws-dashboard/frontend/src/App.tsx#L862-L894` — the `[workbenchSelection]`
  effect is **already** in the demoted shape Phase 2 asks for: it only calls
  `setOpenWorkRootKeys(withOpenWorkRootKey(...))` /
  `setOpenWorkRootRefs(withOpenWorkRootRef(...))` (keep-alive persistence) plus
  untouched layout-restore seeding (lines 876-893). This was already true after
  Phase 1 landed (`ddd353fe`) — Phase 2's second bullet is effectively a no-op
  here; no further edit needed to this effect itself.
- `ws-dashboard/frontend/src/workbench/openRootLookup.ts` — the file already
  hosts the pure helpers this phase should reuse and pattern-match:
  `withOpenWorkRootKey`/`withOpenWorkRootRef` (append-if-absent,
  position-preserving primitives, `L84-105`), and the
  `driveStickyWorkbenchSelection` idempotent-driver pattern (`L167-199`) — a
  precedent for "pure resolver function + thin driver wrapper" that the new
  `selectRoot` pure core should follow: put a trivial pure function (e.g.
  `applySelectRoot(input: {serverId: string; entityId: string}) ->
  {selectedServerId: string; selectedId: string}`) in `openRootLookup.ts`
  alongside these, and make the `App()`-level `selectRoot` a `useCallback` that
  calls the pure function and applies its result via the three setters/ref.
  This keeps the pure core unit-testable per D6 without a React harness.
- `ws-dashboard/frontend/src/workbench/openRootLookup.test.ts` — existing test
  file already imports `withOpenWorkRootKey`/`withOpenWorkRootRef` and uses a
  hand-rolled `assertEqual`/`assertDeepEqual` harness (no test framework). Add
  the new `selectRoot`/`applySelectRoot` pure-reducer unit tests here rather
  than a new file, so no `package.json` `test:workbench` script edit is needed
  (that script hardcodes each compiled test file path; Phase 1 added
  `deriveWorkbenchView.test.js`/`layoutRestore.test.js` there as new files — this
  phase can avoid that edit by reusing the existing file).
- `ws-dashboard/frontend/src/commands.test.ts#L633-L692` — confirms the
  `resource.select` command payload shape (`{type: "select", entityId,
  serverId?}`) and its handler-dispatch tests are independent of the internal
  `App.tsx` handler logic; no edit needed here since the payload shape is
  unchanged (per ticket, confirmed).
- `ws-dashboard/frontend/src/App.tsx#L519` — `selectedServerIdRef` declaration;
  the new `selectRoot` `useCallback` should be declared after this and the
  `[selectedServerId]` sync effect (`L646-648`), before `handleWorkRootOpened`
  (`L669`), since all four routed call sites plus the `resource.select` branch
  need it in scope.

## Implementation Plan

1. In `ws-dashboard/frontend/src/workbench/openRootLookup.ts`: add a small pure
   function (e.g. `applySelectRoot`) taking `{serverId, entityId}` and returning
   `{selectedServerId, selectedId}` — trivial construction, but gives the atomic
   commit a testable, named unit per D6/D4. Export it alongside
   `withOpenWorkRootKey`/`withOpenWorkRootRef`.
2. In `ws-dashboard/frontend/src/App.tsx`, add `selectRoot` as a `useCallback`
   near `L648` (after the `selectedServerIdRef` sync effect, before
   `handleWorkRootOpened`): calls `applySelectRoot`, then commits
   `selectedServerIdRef.current`, `setSelectedServerId`, `setSelectedId` from
   its result in one function body (one React commit). Empty dependency array
   (only stable setters/ref involved).
3. Route the four full-triple call sites through `selectRoot`:
   - `handleServerSelected` (`L713-723`) → `selectRoot(server.id, server.id)`.
   - `applyServerConnection` (`L725-747`) → `selectRoot(server.id, server.id)`.
   - `handleWorkRootOpened` (`L669-698`) → `selectRoot(openedView.server.id,
     openedWorkRootId ?? selectedId)`; add `selectedId` to this callback's
     dependency array.
   - `resource.select` branch inside `executeCommand` (`L1046-1110`) → delete
     the whole `if (serverId && serverId !== selectedServerIdRef.current) {...}`
     block (including the manual `resolveActiveResources`/
     `resolveWorkbenchSelection`/`withOpenWorkRootKey`/`withOpenWorkRootRef`
     seeding) and replace with a single unconditional
     `selectRoot(serverId ?? selectedServerIdRef.current, entityId)` call
     (drop the plain `setSelectedId(entityId)` it currently falls through to).
     Remove `resourcesByServer` from `executeCommand`'s dependency array
     (`L1373-1380`) since it becomes unused there. Remove the now-unused
     `resolveWorkbenchSelection` import (`L271`); keep `resolveActiveResources`
     (still used at the top-level `activeResources` computation).
4. Update the `resourcesByServer` normalize effect (`L700-711`): add `selectedId`
   to its dependency array, drop the inner `if (resources.server.id !==
   selectedServerIdRef.current)` guard, and call `selectRoot(resources.server.id,
   selectedId)` unconditionally (idempotent when nothing changed).
   `normalizeServerRoute(resources.server.id)` stays unconditional as today.
5. Leave the `[workbenchSelection]` effect (`L862-894`) untouched — it is
   already in the demoted (keep-alive-only) shape Phase 2 asks for.
6. Flag-but-do-not-silently-fold: the `server.off` handler's refocus triplet
   (`L1265-1275`) is the same pattern but not named by the ticket. Either (a)
   route it through `selectRoot(LOCAL_DASHBOARD_SERVER_ROUTE,
   LOCAL_DASHBOARD_SERVER_ROUTE)` as a same-behavior consolidation bonus, or (b)
   leave it as-is and note the omission in the phase's commit message / ticket
   Result for a reviewer's call — pick one explicitly rather than silently
   expanding or silently ignoring it.
7. Add pure reducer unit tests to
   `ws-dashboard/frontend/src/workbench/openRootLookup.test.ts`: assert
   `applySelectRoot` advances `selectedServerId`+`selectedId` together
   (atomically) for a work-root-shaped `entityId`, and separately for a
   non-work-root `entityId` (server row / workspace row id) — asserting it just
   sets selection without inventing a mount (i.e., it returns the same shape
   regardless of what kind of id `entityId` is; `selectRoot` never resolves or
   validates the id itself).
8. Run the verification suite (below); then manually dogfood label-click,
   picker-open, and linked-server-open against a running daemon to confirm no
   watermark flash on any of the three gestures (D6 LIMIT: this property is not
   automatable).

## Verification Plan

- `cd ws-dashboard/frontend && npm run build`
- `npm run test:workbench`
- `npm run test:resource-model`
- `npm run test:commands` (confirms `resource.select` payload shape is
  unaffected)
- `npm run test:open-work-root`
- Manual/dogfood (no automated render harness exists, per D6 LIMIT): confirm
  work-root label click, root-picker open, and linked-server open each select
  in one gesture with no `dv-watermark` flash, across a server switch and a
  same-server switch.

## Escalations

- None.
