---
title: "Remote linked server's child worktree flashes then hides: activeResources/selection can transiently collapse to null, hiding every mounted work-root instance"
related:
  260714-feat-dashboard-multi-server-workbench-keepalive: root cause is a state MADE REACHABLE by this ticket's Phase 1 mount-gating restructure (folding the loading/error/no-workRoot early-returns into `activeHeader`, decoupling `openWorkRootInstances.map(...)` from `workbenchModel`) - not a pre-existing bug, and not the server.id/serverRoute key-mismatch this ticket's Phase 1 investigation already refuted
completed: 2026-07-14
---

# Remote linked server's child worktree flashes then hides: activeResources/selection can transiently collapse to null

## Background

Live dogfooding regression, reachable only after
`260714-feat-dashboard-multi-server-workbench-keepalive` Phase 1 landed
(`32fc74ad`).

**Symptom** (a REMOTE linked server's CHILD worktree workroot only - not
local, not a remote server's root workroot): opening a terminal makes it
flash visible then hide; the active Dockview group shows only
`dv-watermark` (zero panels); revisiting the child workroot flashes each
previously-opened terminal, then hides them again.

## Root Cause

Phase 1 folded three former full-subtree early-returns in `WorkbenchShell`
(loading-with-no-cached-resources, error-with-no-cached-resources,
no-resolved-workRoot) into the `activeHeader` ternary
(`App.tsx:5660-5706` pre-fix line numbers), specifically so that
`openWorkRootInstances.map(...)` (`App.tsx:5711-5753`) would keep rendering
unconditionally, decoupled from `workbenchModel`. That restructure is
correct and intentional for its own goal (a focus switch must not tear down
other On servers' mounted panes) - but it made a new state reachable that
did not exist before: when the *active-selection* derivation transiently
fails to resolve for any reason, `selectedWorkRootStateKey` ends up `null`
and matches NO mounted instance, so `isActiveRoot` is false for every
`openWorkRootInstances` entry at once - every instance gets `display:none`
in the same render (flash-then-hide), while the header shows the "No
workRoot" watermark branch.

Confirmed chain (frontend `ws-dashboard/frontend/src/App.tsx`, pre-fix line
numbers):

- `activeResources = resourcesByServer[selectedServerId] ?? null`
  (`App.tsx:541`).
- `workbenchSelection = resolveWorkbenchSelection(activeResources,
  selectedId)` (`App.tsx:760-762`); `resolveWorkbenchSelection`
  (`App.tsx:9488-9537`) returns `null` if and only if its `resources`
  argument (i.e. `activeResources`) is `null` (or has zero workspaces) -
  otherwise it always falls back to *some* workRoot, never `null`. This is
  the crux: the header/selection can only fully collapse when
  `activeResources` itself goes missing for the active server.
- `selectedWorkRootStateKey` is derived from `selection` inside
  `WorkbenchShell` (`App.tsx:3544-3549`).
- `isActiveRoot = rootKey === selectedWorkRootStateKey` gates
  `display:none` per instance (`App.tsx:5712`, `5733`).
- Meanwhile `openWorkRootInstances` (`App.tsx:4127-4153`) resolves each open
  root against its OWN server's slot in the full `resourcesByServer` map
  directly (`resourcesByServer[ref.serverRoute]`) - not through
  `activeResources`/`selection`. This is why the instances themselves keep
  mounting/resolving fine (the "flash") even while the single-slot
  `activeResources`/`selection` path collapses: the two derivations read
  from genuinely different data paths, one per-instance keyed off the full
  map, the other single-slot keyed off `selectedServerId` alone.
- A workRoot (root or a git-linked child worktree) is opened/pinned via
  `handleWorkRootOpened` (`App.tsx:574-600`), which sets
  `selectedServerId`, applies the aggregated "open" response into
  `resourcesByServer`, sets `selectedId`, then kicks off an async canonical
  re-fetch (`loadResources("open")`) plus `normalizeServerRoute` and the 5s
  poll. Across that open-to-canonical-refresh window, `activeResources`
  is derived from a single `resourcesByServer[selectedServerId]` slot with
  no fallback, so any transient absence or incompleteness of that slot
  during the window collapses `activeResources` (and therefore
  `selection`/`workbenchModel`) to `null`.

There is NO `server.id`/`serverRoute` key mismatch - the backend rewrites
both consistently to the same linked-server slug for root and child
worktrees alike; that theory was raised and refuted during this
investigation and is not the mechanism.

**Local** (`selectedServerId` is always `"server-local"`, populated
synchronously at mount) and **remote root** (the server's own top-level
workRoot, whose resources have already been resolved once the server is
selectable at all) do not exercise this window in the same way an
already-open remote child worktree does, since only the single-slot
`activeResources` derivation lacks any fallback for a momentarily-missing
entry - `openWorkRootInstances` never had this gap, because it was already
reading from the full per-server map.

**Epistemic note:** this mechanism is confirmed by direct, careful reading
of the four code points above (data paths, gating conditions, and the
collapse-iff-`resources`-is-`null` behavior of `resolveWorkbenchSelection`
were all traced and verified in the source, not assumed). The exact
runtime event sequence that produces the moment-by-moment transient (vs.
one of several structurally-equivalent async windows in this data flow) was
not reproduced live - remote is only intermittently monitored during this
work, so interactive confirmation in a running dashboard is deferred. The
fix below is written to close the general collapse condition
(`activeResources` transiently null while previously-open instances remain
mounted) rather than one specific hypothesized trigger, so it holds
regardless of exactly which async window produces the gap in practice.

## Phases

### Phase 1: Cache last non-null per-server resources; keep-last-active-root render safety net

### Result (this commit) - 2026-07-14

Two minimal, additive fixes, both scoped to `ws-dashboard/frontend/src/App.tsx`
plus a small pure-function extraction into `resourceModel.ts`:

1. **Primary** (`App.tsx:541` derivation site): `activeResources` now falls
   back to the last non-null resources cached for `selectedServerId`
   instead of collapsing straight to `null` when
   `resourcesByServer[selectedServerId]` has no entry on a given render.
   Extracted two pure helpers into `resourceModel.ts` -
   `withLastNonNullResourcesByServer` (updates the per-server cache,
   no-op if the fresh value is `null` or already cached) and
   `resolveActiveResources` (prefers a fresh `resourcesByServer` entry,
   falls back to the cache, otherwise `null`) - mirroring the existing
   `mergeResourcesByServer` extraction pattern from Phase 1. Because
   `resourcesByServer` itself only ever accumulates (never deletes an
   entry, per `mergeResourcesByServer`'s existing contract), the fallback
   cache can only ever help a server that has resolved at least once; a
   server with no prior successful fetch still resolves to `null`, so this
   cannot resurrect a server ahead of its first real fetch or after an
   explicit teardown. Local and remote-root behavior is unchanged - both
   already populate `resourcesByServer[selectedServerId]` on essentially
   every render once selected, so the fallback path is never exercised for
   them in practice.
2. **Safety net** (`WorkbenchShell`, the `openWorkRootInstances.map(...)`
   render loop): added `lastActiveRootKeyRef`, a ref remembering the last
   `rootKey` that genuinely matched a mounted `openWorkRootInstances` entry.
   If `selectedWorkRootStateKey` matches none of the currently mounted
   instances (this collapse, or any future equivalent gap), `isActiveRoot`
   falls back to the last genuinely-active root key instead of `false` for
   every instance - so the just-opened/just-revisited root stays visible
   through the transient window instead of every mounted instance going
   `display:none` at once. Once `selectedWorkRootStateKey` matches a
   mounted instance again, it is adopted as the new last-active key. No
   other use of `selectedWorkRootStateKey` in `WorkbenchShell` (pane-order
   keys, the activity-pane close handler, etc.) was touched - only the
   render-time `isActiveRoot` gate.

Neither change touches the backend, git toolbar, activity console, or the
Phase 2 left-nav On/Off controls.

Unit tests added in `resourceModel.test.ts` for the new pure helpers
(`withLastNonNullResourcesByServer`, `resolveActiveResources`): a
never-resolved server still resolves to `null`; a cached fallback resolves
correctly when the live map has no entry; a fresh `resourcesByServer` entry
is always preferred over the cache; a cached fallback for one server never
leaks into a different server's gap; and re-caching an unchanged value (or
a `null` fresh value) is a no-op that returns the same object reference (so
it is safe to call on every render without extra allocation on the common
path). No seam exists for the `WorkbenchShell` render-loop safety net (it
reads component state/refs inline in `App()`'s render), matching the
`260714-bug-dashboard-terminal-pane-split-mirror-key-mismatch` precedent of
not forcing a test seam that doesn't exist.

Verification: `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite
build`) passes clean. `npm run test:resource-model` (includes the new
tests), `npm run test:workbench`, `npm run test:commands`, and
`npm run test:open-work-root` all pass unchanged/extended.

Forward note: live/manual verification in a running dashboard (opening a
remote linked server's child worktree and confirming no flash-then-hide) was
not performed as part of this fix - the task explicitly prohibited touching
the live dogfooding daemon/gateway process, and headless-Playwright
verification is deferred per the same constraint. This fix was verified by
static/code-level confirmation of the collapse mechanism and its correction,
plus the existing automated suites, not by an interactive repro-then-fix
cycle. Live confirmation remains an open follow-up the next time the affected
remote server is being monitored.

#### Edition (670412eb) - 2026-07-14

An opus correctness review of the Phase 1 safety net found a reachable
cross-server content leak: `lastActiveRootKeyRef`'s fallback in
`WorkbenchShell` was not scoped to the currently selected server and never
reset on a server switch. Concrete traced scenario: on server A with root
A1 active (`lastActiveRootKeyRef = A1`), the user selects a remote server B
that has never resolved and is not `connected`, so `handleServerSelected`
sets `selectedServerId=B` but skips `loadResources`. `activeResources`,
`selection`, and `selectedWorkRootStateKey` all collapse to `null`, but the
fallback still resolved to `lastActiveRootKeyRef.current = A1` (still
mounted via keep-alive) - so server A's live terminals stayed visible under
server B's header instead of the empty watermark.

Fix: added `lastActiveRootServerIdRef`, tracking the server route that
`lastActiveRootKeyRef`'s root belongs to, and gated the fallback so it only
applies when that stored server route matches the currently selected server
(`selectedServerId`, newly threaded down as a `WorkbenchShell` prop -
`resources` alone cannot substitute, since it is `null` in exactly this
scenario). Chose server-scoped gating over unconditionally clearing
`lastActiveRootKeyRef` on every server switch: a blanket reset would also
defeat this ticket's original purpose (a same-server transient
selection/mount collapse must still fall back to that server's last-active
root). The decision is extracted as a pure `resolveEffectiveActiveRootKey`
helper in `workbench/openRootLookup.ts`.

This corrects the Result claim above that "No seam exists for the
`WorkbenchShell` render-loop safety net" - a seam now exists specifically
for the fallback-selection decision (not the full render loop), with unit
coverage in `workbench/openRootLookup.test.ts` for: a genuine mount match
winning outright, a same-server transient collapse still falling back
correctly, a cross-server switch to an unresolved server resolving to
`null` instead of pinning the previous server's root, and the no-history
case.

Verification: `cd ws-dashboard/frontend && npm run build` (`tsc -b && vite
build`), `npm run test:resource-model`, and `npm run test:workbench` (now
including the new `resolveEffectiveActiveRootKey` cases) all pass.

## Spec Impact

No spec stem addresses this defect specifically. This is a regression fix
restoring the intended keep-alive behavior already documented under
`{#260714-ws-dashboard-cross-server-workbench-keepalive}` in
`ai-docs/spec/ws-web-dashboard/index.md` (added by the Phase 1 result on
this same ticket family) - no new caller-visible contract is introduced,
only a state made reachable by that change being closed off. Contract-first
spec: no.
