---
title: "Playwright acceptance suite has scattered unscoped locators after multi-root-mount landed, plus an unrelated TOML/text language-detection mismatch"
related:
  260707-bug-dashboard-e2e-panel-header-dead-code-drift: prerequisite
sage-review: completed
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

## Spec Impact

No existing spec stem addresses this behavior. Phase 1 is a test-only fix
(locator scoping in `e2e/dashboard-acceptance.spec.ts`); it introduces no
caller-visible product contract change. Phase 2 (TOML/text
language-detection mismatch) may turn out to be a real product bug fix
(if the app's language detection is actually wrong) or a stale-fixture fix
(if the test's expectation is wrong) — the diagnosis in Phase 2 itself
will determine which, and if it turns out to be a genuine product-visible
behavior change, that phase should add its own `## Spec Impact` addendum
before landing. Spec area: none yet identified. Contract-first spec: no.

## Phases

### Phase 1: Audit and fix multi-root locator scoping across the full spec file

Systematically review every Playwright locator in
`e2e/dashboard-acceptance.spec.ts` that queries within the workbench/dockview
DOM and confirm each is scoped to the active work root
(`[data-workbench-root-active="true"]`) where multiple work roots may be
open simultaneously during the suite's later "git workspace overflow /
linked worktree" test steps. Fix each unscoped locator found. Verification:
since Phase 2's TOML/text failure is a distinct, still-live issue in the
same suite, a fully green run is not attainable until both phases land —
re-run `npm run test:browser` twice consecutively and confirm the suite
reaches at least as far as the TOML assertion (i.e., every multi-root
locator-scoping failure is gone) with no new failures introduced before
that point; a fully green two-run confirmation is deferred to after Phase
2 also lands.

### Result (026b1b8c) - 2026-07-07

Audited the full spec file and scoped ~15 previously-bare locators (tab,
pane, activity, and document-viewer selectors inside the per-work-root
Dockview-mounted DOM) to `[data-workbench-root-active="true"] ` prefixes,
matching the pattern established by the prerequisite ticket's
`expectDockviewWorkbench` fix. Traced each work root's (`workRoot`,
`gitWorkRoot`, `secondWorkRoot`) full open/select/close lifecycle to judge
genuine collision risk rather than blanket-scoping every dockview-adjacent
selector; deliberately left toolbar/file-explorer/git-toolbar/terminal-
surface/close-popover locators unscoped after confirming they live outside
the per-root map or no other root ever produces a colliding element for
them. `expectDurableDockviewSplitDrop()`'s internals were left unscoped —
it has zero call sites (dead code), out of scope.

Review (single, full) found 2 Important misses on the first pass — a
`.readonly-text-pane` `toHaveCount(0)` assertion (spec:1897) and a
`confirmSessionClose` tab locator filtered `{hasText:"Terminal"}`
(spec:2663, the exact locator class this ticket's Background names as the
trigger bug — the `{hasText:"Agent"}` sibling had already been scoped but
this variant was missed). Fixed both in a follow-up commit; re-review
confirmed clean.

Verification: `npm run test:browser` run twice after each commit (4 runs
total across both cycles). Every run reached the identical, single,
already-known Phase 2 TOML failure point (spec:1954 after the final fix)
with no locator-scoping/strict-mode-violation failures before it, matching
the redefined verification boundary above. Per the implementer's own
honesty note: this was a manual audit of a ~3000-line file, not exhaustive
by construction — a future edit that changes root-open ordering or gives
an inactive root a terminal/document pane it doesn't have today could still
surface a new instance of this class.

### Phase 2: Diagnose and fix the TOML/text language-detection mismatch

Root-cause why `.document-source-viewer[data-editor-read-only="true"]`
reports `data-editor-language="text"` instead of the expected `"toml"` for
the relevant fixture file, and fix the underlying cause (or the test fixture
if the test's expectation is itself stale). Likely independent of Phase 1;
order between phases does not matter, but Phase 1 should land first since
it's what currently blocks the suite from running far enough to reliably
observe this one. Verification: re-run `npm run test:browser` twice
consecutively; once both phases have landed, confirm the full suite passes
green on both runs. If landing this phase alone (before Phase 1), confirm
at minimum that the TOML assertion itself now passes, even though the
suite as a whole may still fail later on Phase 1's unfixed locators.
