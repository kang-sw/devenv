---
title: "Dashboard terminal content clears and collapses to 1 row on tab/session switch"
related:
  260703-feat-dashboard-workroot-session-keepalive: related-area
sage-review: completed
---

# Dashboard terminal content clears and collapses to 1 row on tab/session switch

## Background

Dogfooding report while manually verifying `260703`'s Phase 6/7 restore work
(terminal visual buffer restore + close/reopen restore reuse) via the
`--no-auth` local dashboard daemon.

Observed behavior, not yet root-caused:

- A terminal pane that is open in a workroot gets cleared out after
  navigating away to another session/workroot and coming back.
- The same clearing happens on a plain dockview tab switch (not just a full
  session round-trip).
- The terminal's row size appears to collapse to `1` row (columns stay at
  whatever the pane width implies, i.e. `<cols> x 1`) after the switch.

This looks like it could be a resize/fit regression (e.g. a `fit()`/resize
observer not re-firing correctly when a dockview pane is hidden then shown
again, collapsing the terminal to a 1-row viewport) rather than a pure
data-loss bug in the visual-restore snapshot itself — the "cleared" look may
be a side effect of xterm re-rendering into a 1-row viewport rather than an
actual loss of the stored `serialized` buffer. Needs investigation before
assuming which layer (dockview visibility lifecycle, xterm `fit()` call
site, or the Phase 6/7 restore/reattach path) owns the regression.

## Phases

### Phase 1: Root-cause and fix terminal clear/1-row-collapse on tab or session switch

Reproduce reliably (tab switch within one session vs. full session
round-trip), identify where the resize/visibility lifecycle interacts with
xterm's `fit()`/`resize()` and with the Phase 6/7 restore write path, and fix
without regressing the just-landed visual-buffer-restore behavior. Add or
extend a Playwright e2e regression test in `e2e/dashboard-acceptance.spec.ts`
covering both reproduction modes (in-session dockview tab switch, and full
session/workroot round-trip) so this class of regression is caught
automatically going forward. Verification: reproduce first (screenshot/trace
before the fix), then confirm both reproduction modes are fixed via the new
or extended e2e coverage, run at least twice to rule out flake.

## Spec Impact

No existing spec stem addresses terminal pane resize/visibility-lifecycle
behavior. This is a regression fix restoring already-shipped Phase 6/7
restore behavior (terminal content persists and renders at the correct
size across tab/session switches) — no new caller-visible contract is being
introduced, only a defect in already-committed behavior being corrected.
Spec area: none yet identified. Contract-first spec: no.
