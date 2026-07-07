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

### Result (67fabd77) - 2026-07-07

Root cause differed from the ticket's own working hypothesis and from a
first, disproven fix attempt. Two rounds of live-repro research were needed:

1. First plan hypothesized the vendor `@xterm/addon-fit` `MINIMUM_ROWS=1`
   clamp firing while a dockview tab is hidden/detached. Live-repro
   disproved this: a hidden/detached container's `getComputedStyle` returns
   `""` (not `0`), so `proposeDimensions()` yields `NaN`, and the vendor
   `fit()` already has an `isNaN` guard that safely no-ops — this path was
   never actually broken.
2. Research live-reproduced the real trigger: a **visible** pane whose
   container is momentarily too short (e.g. during dockview relayout on a
   tab/session switch, or a genuinely short window/split) causes
   `fitAddon.fit()` to propose the degenerate `rows=1` floor and clear the
   rendered screen. Neither of the ticket's two literally-named repro modes
   (bare tab switch, session/workroot round-trip) reproduces this at a
   normal tall viewport — the load-bearing trigger is short *visible* pane
   height, confirmed via `page.setViewportSize` to a short height.

Fixed in `App.tsx`'s `TerminalPaneBody`: `fitNow()` now checks
`fitAddon.proposeDimensions()` before applying a fit and skips the
resize/shrink loop entirely when the result is unmeasurable or floors at 1
row, preserving the last-good emulator size instead of collapsing. The
`paneVisible` effect now explicitly re-runs `fitNow()`/`forwardSize()` (via
new `fitNowRef`/`forwardSizeRef`) so a pane that was skipped-while-short or
briefly hidden is deterministically re-fitted once genuinely visible/sized,
rather than relying solely on the next incidental `ResizeObserver` callback.
No changes to `terminalSizeBounds.minRows` or the Phase 6/7 restore write
path (`workbench/terminalVisualRestore.ts`), confirmed out of scope and
untouched.

Added `terminalRows`/`emulatorRowCount` e2e helpers and regression coverage
in `dashboard-acceptance.spec.ts`: the short-viewport transition (the
confirmed, load-bearing trigger) asserts exact preservation of the
tall-viewport row count (not merely "not floored to 1", tightened during
review), plus the two ticket-named repro modes retained as non-regression
stability assertions.

Review (partitioned correctness/fit/test, one fix-relay cycle): correctness
clean with 2 accepted minors, fit clean, test non-clean 1 important
(assertion under-asserted the fix's contract) fixed and re-reviewed clean.

Verification: pre-fix repro captured the exact `<cols>x1` collapse
(screenshot); post-fix, the regression test passed twice independently
(~30-32s each, re-confirmed by reviewers); `npm run test:terminals` (PTY
bounds + restore-write-path unit tests) stayed green.

Forward note: the suite's second test
("linked server root picker uses server-scoped local gateway routes") fails
when run after the main gate test in the same file, due to a pre-existing,
unrelated single-use-pairing-token/isolated-browser-context issue in the
e2e harness — confirmed present before this fix on a clean branch tip.
Worth a follow-up `idea/` ticket on the e2e harness if a second
cookie-based test is ever added to this file.

## Spec Impact

No existing spec stem addresses terminal pane resize/visibility-lifecycle
behavior. This is a regression fix restoring already-shipped Phase 6/7
restore behavior (terminal content persists and renders at the correct
size across tab/session switches) — no new caller-visible contract is being
introduced, only a defect in already-committed behavior being corrected.
Spec area: none yet identified. Contract-first spec: no.
