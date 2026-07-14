---
title: "Server-switch work-root select flashes to dv-watermark: openWorkRootKeys/Refs mount one render too late for resolveEffectiveActiveRootKey's first branch"
related:
  260714-bug-dashboard-worktree-label-click-requires-server-focus: this ticket's regression is a state made reachable by that ticket's one-gesture focus+select fix (`ddb0e220`)
  260714-bug-dashboard-childroot-workbench-flash-hide: this fix mounts the selected root synchronously instead of loosening or resetting that ticket's server-scoped `resolveEffectiveActiveRootKey` guard, so the cross-server content leak it closed stays closed
completed: 2026-07-14
---

# Server-switch work-root select flashes to dv-watermark: openWorkRootKeys/Refs mount one render too late

## Background

Live dogfooding regression on a REMOTE server, introduced by `ddb0e220`
(`260714-bug-dashboard-worktree-label-click-requires-server-focus`'s
one-gesture focus+select fix). **Symptom**: clicking a work-root/worktree
label under an On-but-unfocused remote server now focuses that server and
selects the entity in one gesture (the intended fix), but the terminal pane
flashes then shows an empty `dv-watermark` workbench for one frame before
the correct pane appears. Affects both a remote server's own root work root
and its child worktrees.

## Root Cause

`ddb0e220` made the `resource.select` handler (`App.tsx`, in
`executeCommand`) switch the focused server AND select the entity in a
SINGLE React commit:

```
selectedServerIdRef.current = serverId;
setSelectedServerId(serverId);
setSelectedId(entityId);
```

On that switch-render (Render N):

- `activeResources`/`workbenchSelection` resolve correctly (the remote
  server's tree is already cached in `resourcesByServer`, which is why its
  rows were clickable at all), so `selectedWorkRootStateKey` (`App.tsx`,
  inside `WorkbenchShell`) is set to the target root's key.
- BUT the selected root is not yet in `openWorkRootKeys`. The only path that
  adds a newly-selected root to `openWorkRootKeys`/`openWorkRootRefs` was
  the `workbenchSelection` effect (`App.tsx`, keyed on
  `[workbenchSelection]`), which by React semantics runs AFTER Render N
  commits - one render too late for this render's decision.
- So `openWorkRootInstances` (derived straight from
  `openWorkRootKeys`/`openWorkRootRefs`) lacks the key on Render N, making
  `selectedRootIsMounted` false.
- The `WorkbenchShell` ref-update block that advances
  `lastActiveRootKeyRef`/`lastActiveRootServerIdRef` only writes when
  `selectedRootIsMounted` is true, so it is skipped this render -
  `lastActiveRootServerIdRef` still holds the PREVIOUS server.
- `resolveEffectiveActiveRootKey` (`workbench/openRootLookup.ts`): its first
  branch (`selectedRootKey && selectedRootIsMounted`) fails (not mounted
  yet); its second branch, the `260714-bug-dashboard-childroot-workbench-flash-hide`
  server-scoped fallback guard (`lastActiveRootServerId ===
  selectedServerId`), also fails (stale previous server) - so it returns
  `null`, every mounted `openWorkRootInstances` entry gets `display:none`,
  and the workbench shows `dv-watermark` for that one frame. The next render
  (N+1), the `workbenchSelection` effect has mounted the key and the correct
  pane appears - hence "flash then hide, then reappear".

This is a state made reachable by `ddb0e220`'s one-gesture composition
(switching server and selection in the same commit); it is not a defect in
the `260714-bug-dashboard-childroot-workbench-flash-hide` server-scoped guard
itself, which must stay intact to keep that ticket's cross-server content
leak closed.

## Phases

### Phase 1: Synchronously mount the selected root on server-switch select

### Result (this commit) - 2026-07-14

Narrowest correct fix (option iii' from investigation): when the
`resource.select` handler's `"select"` case switches server for a work-root
selection, it now ALSO mounts the selected root's `openWorkRootKeys`/
`openWorkRootRefs` entries synchronously in that same commit, mirroring
exactly what the `workbenchSelection` effect does when it opens a selected
root - so Render N already has `selectedRootIsMounted = true`,
`resolveEffectiveActiveRootKey`'s first branch wins outright, the pane is
visible immediately, and the stale server-scoped guard is never consulted
for this path.

1. **Extracted a shared "ensure open" pair** into
   `workbench/openRootLookup.ts` - `withOpenWorkRootKey(openWorkRootKeys,
   rootKey)` and `withOpenWorkRootRef(openWorkRootRefs, rootKey, ref)` -
   pure, idempotent (append/seed only if absent, return the same reference
   otherwise), with the identical key format (`serverScopedIdentity`) and
   ref shape (`{rootId, serverRoute}`) the effect already used inline. Both
   the `workbenchSelection` effect (`App.tsx`) and the new `resource.select`
   fast path now call these same two functions, so they cannot drift apart.
   The effect's layout-restore seeding (from `workbenchLayoutRestoreRef`)
   was left untouched and NOT duplicated into the handler - it is not
   needed to close the mount gap (that only requires
   `openWorkRootKeys`/`openWorkRootRefs`), and duplicating it would risk a
   second, harder-to-reason-about seeding path for the same ref.
2. **Verified `entityId` is not always a work-root id** at the
   `resource.select` dispatch site (`ResourceRow`'s label `onClick`, one
   dispatch site in the codebase, confirmed via grep). `ResourceRow` has
   three presentations dispatched from `WorkspaceRows`: `workRoot`
   (`id=root.id`, a genuine work-root id), `compactWorkRoot`
   (`id=compactRoot.id`, also a genuine work-root id), and `workspace`
   (`id=workspace.id` - NOT a work-root id; relies on
   `resolveWorkbenchSelection`'s first branch, which maps a workspace id to
   its primary work root). So the new fast path does NOT mount based on
   `entityId` directly - it resolves the target work root the same way the
   effect resolves `workbenchSelection`: `resolveActiveResources(
   resourcesByServer, serverId, lastNonNullResourcesByServerRef.current)`
   for the target server's tree, then `resolveWorkbenchSelection(
   targetResources, entityId)`, and mounts `.root` from that result. A
   `null` result (target server's tree not yet cached) is a no-op, same as
   the pre-existing behavior for that case - this fix closes the gap only
   for the traced regression (a server whose tree is already cached, which
   is why its rows were clickable in the first place).
3. Gated strictly to the server-switch branch (`serverId &&
   serverId !== selectedServerIdRef.current`) - same-server selection is
   untouched. Did NOT touch `resolveEffectiveActiveRootKey`, the
   `260714-bug-dashboard-childroot-workbench-flash-hide` server-scoped
   guard, `resolveActiveResources`, or `lastNonNullResourcesByServerRef` -
   loosening or resetting any of those would reopen the cross-server
   content-leak that ticket closed.

Unit tests added in `workbench/openRootLookup.test.ts` for
`withOpenWorkRootKey`/`withOpenWorkRootRef`: appending an absent key without
reordering existing keys, idempotent no-op (same array/object reference)
when already present/seeded, and never clobbering an already-open root's
ref with a different candidate ref for the same key.

Verification: `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite
build`) passes clean. `npm run test:workbench` (includes the new
`withOpenWorkRootKey`/`withOpenWorkRootRef` cases), `npm run test:commands`,
and `npm run test:resource-model` all pass unchanged/extended.

No component-render test infra exists in this project (no
React-Testing-Library/jsdom harness), so the interactive
click-a-remote-work-root-and-observe-no-flash behavior itself is not
covered by an automated test; live confirmation is deferred to dogfooding
the next time the affected remote server is being monitored, per the same
constraint noted in `260714-bug-dashboard-childroot-workbench-flash-hide`.

## Spec Impact

No spec stem describes this defect specifically. This is a regression fix
restoring the one-gesture click behavior `ddb0e220`
(`260714-bug-dashboard-worktree-label-click-requires-server-focus`) already
established as intended, without introducing a new caller-visible contract.
Contract-first spec: no.
