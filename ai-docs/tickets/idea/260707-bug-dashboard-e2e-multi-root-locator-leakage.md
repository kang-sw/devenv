---
title: "Playwright acceptance suite has scattered unscoped locators after multi-root-mount landed, plus an unrelated TOML/text language-detection mismatch"
related:
  260707-bug-dashboard-e2e-panel-header-dead-code-drift: prerequisite
---

# Playwright acceptance suite has scattered unscoped locators after multi-root-mount landed, plus an unrelated TOML/text language-detection mismatch

## Background

Discovered while implementing `260707-bug-dashboard-e2e-panel-header-dead-code-drift`
(same session — this was the first time the Playwright acceptance suite
(`npm run test:browser`, `e2e/dashboard-acceptance.spec.ts`) could actually
run in this sandbox; Playwright system deps had never been installed
before). That ticket fixed the suite's *first* blocking failure
(`.panel-header` dead selector) and an *immediately following* one
(`expectDockviewWorkbench`'s bare `[data-workbench-layout-owner="dockview"]`
locator, unscoped since `c7a1f59c feat(ws-dashboard): mount one dockview
workbench instance per open work root` started persistently mounting one
Dockview instance per open work root, toggled via `display:none` rather
than unmounted).

Fixing those two exposed **at least two more, distinct** failures further
into the same test run, discovered but deliberately NOT fixed under that
ticket (to avoid unbounded scope creep in a "dead code + one stale
assertion" ticket):

1. **Locator-leakage class (same root cause as the two already-fixed
   issues):** on a second run of the identical suite, a
   `.dockview-workbench-tab[...="confirmSessionClose"]` locator filtered by
   `{hasText: "Agent"}` resolved to 2 elements instead of 1 — again because
   an inactive (but still DOM-mounted, `display:none`) second work-root
   instance contains a matching element. This strongly suggests the
   `c7a1f59c` multi-root-mount change left **multiple** locators throughout
   this 2700+ line spec file unscoped to the active work root, of which
   `expectDockviewWorkbench` was only the first one hit. A full audit is
   needed to find every locator in the file that assumes single-root DOM
   presence and scope each to `[data-workbench-root-active="true"]` (the
   active-root discriminator attribute, confirmed at `App.tsx`'s
   `openWorkRootInstances.map`, ~L5016-5024).

2. **Separate, apparently unrelated feature bug:** a
   `.document-source-viewer[data-editor-read-only="true"]` assertion
   expected `data-editor-language="toml"` but observed `"text"` — a TOML
   file's language-detection/syntax-highlighting mode not being applied as
   expected. Not yet diagnosed; may be a real regression in language
   detection, or a test fixture/timing issue. Needs investigation
   independent of the locator-leakage issue above.

Both failures currently make `npm run test:browser` non-green even after
`260707`'s fixes land. Whoever picks this up should re-run the suite fresh
against `260707`'s merged state to get the current, authoritative failure
list — the two issues above were observed across two runs during that
ticket's implementation and may not be the complete set (the suite failed
at the first-encountered issue each run and never reached the end).

## Phases

### Phase 1: Audit and fix multi-root locator scoping across the full spec file

Systematically review every Playwright locator in
`e2e/dashboard-acceptance.spec.ts` that queries within the workbench/dockview
DOM and confirm each is scoped to the active work root
(`[data-workbench-root-active="true"]`) where multiple work roots may be
open simultaneously during the suite's later "git workspace overflow /
linked worktree" test steps. Fix each unscoped locator found. Verification:
run `npm run test:browser` to a fully green state across at least two
consecutive runs.

### Phase 2: Diagnose and fix the TOML/text language-detection mismatch

Root-cause why `.document-source-viewer[data-editor-read-only="true"]`
reports `data-editor-language="text"` instead of the expected `"toml"` for
the relevant fixture file, and fix the underlying cause (or the test fixture
if the test's expectation is itself stale). Likely independent of Phase 1;
order between phases does not matter, but Phase 1 should land first since
it's what currently blocks the suite from running far enough to reliably
observe this one.
