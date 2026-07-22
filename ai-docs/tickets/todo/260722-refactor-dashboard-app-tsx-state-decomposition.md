---
title: Decompose App.tsx - untangle the WorkbenchShell/App() state core (design-gated)
related:
  260722-refactor-dashboard-app-tsx-leaf-extraction: prerequisite that shrinks the leaf surface first
related-mental-model:
  - ws-web-dashboard
sage-review-design: completed
sage-review-completeness: required
---

# Decompose App.tsx - untangle the WorkbenchShell/App() state core (design-gated)

## Background

After the leaf-extraction prerequisite shrinks App.tsx's surface, the residual
mass is two deeply state-entangled giants - `WorkbenchShell` (~2,562 lines) and
`App()` (~1,535 lines) - which together hold ~45% of the file's ~252 hook
calls. Decomposing these is the real technical-debt reducer but also the
highest-risk work in the file, so it is split out here and left deliberately
loose. This ticket is a standing agenda ("the entangled core of App.tsx still
needs decomposing"), not a fixed plan - it will go stale, and that is expected.

Entanglement points (point-in-time snapshot, 2026-07-22):
- 23-prop `WorkbenchShell` call site: 9 `onXChange` setState callbacks + 2
  RefObjects (`workbenchLayoutRestoreRef`, `terminalVisualRestoreRef`).
- State lifted higher than its sole consumer: openWorkRootKeys / openWorkRootRefs
  / workbenchGroupsByRoot / paneOrderByRoot (WorkbenchShell is the real consumer
  of 3 of the 4, yet App() owns them).
- One giant reconciliation `useEffect` fanning out to 6+ setState calls.
- Several "mirror state into a ref for effect access" patterns.
- Two context providers mounted inline in App()'s JSX (TerminalPrefsContext,
  SettingsTerminalContext).

## Phases

### Phase 1: State-strategy design gate

Decide the state-management strategy (context/store seam) to replace the prop
drill and lifted refs BEFORE any extraction. The structural survey warned that
mechanically hoisting `WorkbenchShell` just relocates the 23-prop drill across a
file boundary without reducing debt. This phase produces the target seam design
(e.g. a dedicated workbench store/context), weighs alternatives, and gets owner
sign-off. No code extraction until this design lands.

### Phase 2: Execute the decomposition (loose)

Execute per the Phase 1 design: introduce the store/context, cut the prop drill,
split `WorkbenchShell`/`App()` into cohesive units. High regression risk
(terminal restore/reattach, dockview layout); heavy review expected. Left
intentionally underspecified until Phase 1 fixes the approach.

Verification: settled by the Phase 1 design, but at minimum the
ws-web-dashboard domain-rule browser gate (`npm run test:browser`, Playwright
acceptance) plus `npm run build` and `test:*` green, since this reshapes
visible UI (terminal restore/reattach, dockview layout).

## Spec Impact

Deferred to Phase 1. If the decomposition introduces an observable seam it gets
spec'd at design time; a pure internal reshape needs no spec. Contract-first
spec: to be decided at the design gate.
