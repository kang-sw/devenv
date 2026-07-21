---
title: Dashboard workbench "active work root" derivation is structurally fragile — three regressions in one locus in one session
related:
  260714-feat-dashboard-multi-server-workbench-keepalive: origin of the cluster - generalized intra-server hide-not-unmount to cross-server, decoupling mount-gating from selection and making the active-root transient-collapse state reachable
  260714-bug-dashboard-childroot-workbench-flash-hide: regression #1 - flash-then-hide + dv-watermark on remote CHILD work roots; fixed via resolveActiveResources last-non-null fallback plus a lastActiveRootKeyRef safety net (an Edition on this ticket also folded in a cross-server pin-leak fix found in review)
  260714-bug-dashboard-worktree-label-click-requires-server-focus: regression #2's fix - the one-gesture focus+select fix (ddb0e220) that itself introduced regression #3
  260714-bug-dashboard-select-server-switch-mount-gap-flash-hide: regression #3 - flash-then-dv-watermark on remote ROOT and child work roots from the one-gesture select handler's server-switch mount gap; may still be landing, referenced by name only if not yet filed
---

# Dashboard workbench "active work root" derivation is structurally fragile

## Background / Evidence

Three regressions landed in the SAME code cluster within one work session —
the dashboard workbench's "which work-root is currently visible" derivation
across servers:

1. childroot flash-then-hide + `dv-watermark` on remote CHILD work roots
   (introduced by keepalive Phase 1's mount-gating decoupling; fixed via
   `resolveActiveResources` last-non-null fallback + a `lastActiveRootKeyRef`
   safety net).
2. cross-server content-leak: the safety net re-pinned the previous server's
   live panes under a newly-selected unresolved server's header (found in
   review; fixed by server-scoping the fallback in
   `resolveEffectiveActiveRootKey`).
3. flash-then-`dv-watermark` on remote ROOT and child work roots when
   clicking a work-root label (introduced by the one-gesture focus+select
   fix `ddb0e220`; the select handler switches server + selects root in one
   commit but the root isn't mounted until an async effect runs one render
   later, so the server-scoped guard sees a stale server and collapses to
   watermark for a frame).

Three regressions in one locus in one session is the signal that motivates
this ticket.

## Structural root cause

The "currently-visible work root" is decided by the loosely-coupled
agreement of many sources that update at DIFFERENT times, with no atomic
transaction:

- `selectedServerId`, `selectedId` (state), `resourcesByServer`
  (async-populated state), `openWorkRootKeys`/`openWorkRootRefs` (the
  mounted set — updated by an async `useEffect`, NOT synchronously with
  selection), plus render-time refs `lastNonNullResourcesByServerRef`,
  `lastActiveRootKeyRef`/`lastActiveRootServerIdRef` used as fallbacks.
- Because mounting is decoupled from selection via the async effect
  (`App.tsx:794` region), EVERY selection entry point has a one-render
  "selected-but-not-mounted" gap, and each new entry point (server-label
  click, picker open, work-root label click) must independently avoid
  tripping it — the mount-gap fix literally replicates the effect's mount
  logic a third time.
- The fallback refs are written during render only when the root is already
  mounted, so a gap leaves them stale → fallback either over-fires (content
  leak) or under-fires (watermark). The fallbacks themselves exist only
  because the primary derivation transiently collapses — a patch-on-patch
  smell.
- keep-alive generalized hide-not-unmount to cross-server, but active-root
  derivation still funnels through a single `selectedServerId` slot with
  per-server caches + fallbacks bolted on, exposing a transient at every
  entry point.

## Refactor direction (candidates, not decided)

- Atomic selection action `selectRoot(serverId, rootId)` that in ONE
  transaction sets `selectedServerId` + `selectedId` AND ensures the root is
  in `openWorkRootKeys`/`openWorkRootRefs` — removing the
  "selected-but-not-mounted" gap so no entry point can forget it; the async
  effect shrinks to reconciling external opens only.
- Derive visible-root as a PURE function of committed state
  `(selectedServerId, selectedId, resourcesByServer, openWorkRootKeys)`,
  holding "last-good" memory as explicit STATE rather than render-time refs,
  so the transient-collapse fallbacks become unnecessary or at least
  unit-testable.
- Consolidate the three selection entry points through the single action.
- Testability is the meta-point: these regressions all reach dogfooding
  because there is NO component/render test harness — every fix defers
  verification to manual Playwright/dogfooding. Pushing the active-root
  decision into pure functions (as `openRootLookup.ts` already partially is)
  would let the exact transient render sequences be unit-tested.

## Not actionable yet (TBD)

Architecture change — needs a design pass and prioritization against the
chat-UI milestone before promotion. No implementation direction committed.
Owner decision required.

## Disposition

Answered by `260714-refactor-dashboard-active-root-atomic-select-pure-derivation`
(Phases 1-3, all landed). See that ticket's "Traceability - each failure mode ->
the structural property that prevents it" table for the full failure-mode ->
structural-property mapping (D1-D5 close all three regressions recorded above
plus the fourth failure mode root-caused during that ticket's own design pass);
not restated here. Phase 1 replaced the ref-backed safety net
(`lastActiveRootKeyRef`/`lastActiveRootServerIdRef`) with a pure render-time
derivation (`deriveWorkbenchView`) that mounts the selected root by
construction instead of via a lagging effect. Phase 2 consolidated every
selection entry point through one atomic `selectRoot` action, eliminating the
per-entry-point mount-gap discipline this ticket's "Structural root cause"
section identified as unable to scale. Phase 3 was a cleanup pass confirming
no dead code, comments, or spec contract were left behind, and closed with
Phase 1/2/3 Results recorded on the actionable ticket.

This idea ticket intentionally stays open in `idea/` as the standing evidence
log for the three-regressions-in-one-locus signal that motivated the design -
its structural diagnosis is resolved, not the ticket itself moved or closed.
