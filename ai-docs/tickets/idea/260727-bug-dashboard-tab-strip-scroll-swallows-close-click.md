---
title: Tab-strip scroll on activation may swallow the close click on an overflowing workbench tab strip
related:
  260726-bug-dashboard-restored-tab-close-inert-until-activated: Phase 1 fixed the badge-driven instance of this lost-click family and explicitly left this one open
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
---

# Tab-strip scroll on activation may swallow the close click on an overflowing workbench tab strip

## Background

`260726-bug-dashboard-restored-tab-close-inert-until-activated` Phase 1 fixed
one member of a lost-`click` family: pressing `×` on a never-activated,
attention-badged terminal tab activated the panel through dockview's native
`pointerdown`, which acknowledged attention, which unmounted the in-flow badge,
which slid the close button 11px left between the user's press and release — so
the `click` landed on `.dv-tab` instead of the button and was silently
swallowed. The fix (F1) took the badge out of the tab's layout flow, and the
measured shift went from `-11.0px` to `0.0px`.

That fix is badge-specific by construction. The originating ticket names a
second chain for the same symptom and records that F1 does **not** close it.

## What Is Verified, And What Is Not

**Verified in dependency source, not observed in a browser.**
`TabsContainer.setActivePanel` scrolls the tab strip to reveal the newly active
tab — it assigns `parentElement.scrollLeft`
(`node_modules/dockview-core/dist/esm/dockview/components/titlebar/tabs.js`,
the `setActivePanel` path). Activation happens synchronously from the same
native `pointerdown` on `.dv-tab` that Phase 1 traced
(`.../components/tab/tab.js`). So whenever the strip overflows and the pressed
tab is only partially visible, pressing `×` displaces the tab horizontally
between `pointerdown` and `mouseup`, with no attention indicator involved.

**Not verified:** that this actually reproduces as an inert close in a real
browser. Nobody has run it. Phase 1's browser assertions use a terminal count
small enough that the strip never overflows, so its green result says nothing
about this path. The displacement could also be small enough, or the button
wide enough, that the release still lands inside the button.

This distinction is the whole point of this ticket: a plausible source-traced
mechanism is not a defect until it is reproduced.

## First Step

Reproduce or falsify, before designing anything. Extend the Phase 1 harness in
`ws-dashboard/frontend/e2e/agent-attention-indicator.spec.ts`: spawn enough
terminals in one work root to overflow the tab strip, reload, do not click any
tab body, then run the same hand-orchestrated gesture Phase 1 added
(`page.mouse.move` → `down` → two `requestAnimationFrame` waits →
`boundingBox()` → `up`) against a partially-visible tab. Phase 1's own
measurement helper already reports exactly the numbers this needs: the
before/during bounding boxes, whether the first click opened the popover, and
whether a second one did.

If it does not reproduce, close this as dropped and record the measured
displacement — that number is the useful residue either way.

## Why Not Just Land F3 Now

The originating ticket's F3 (a native `pointerdown` listener on the close
button that `stopPropagation()`s dockview's activation) is immune to every
reflow chain including this one, and was a live candidate there. It was not
selected because the evidence pointed at case 1, and it carries a stated cost:
clicking `×` on an inactive tab would no longer activate or acknowledge it,
**for every surface kind**. Paying a cross-surface behavior change for a defect
nobody has observed is the wrong order of operations. Reproduce first.

## Notes

- Discovered during the Phase 1 review of the originating ticket. The
  correctness reviewer re-derived the `scrollLeft` assignment from dockview
  source rather than taking the originating ticket's word for it; the fit
  reviewer independently flagged that the phase's new spec sentence would read
  as covering this case.
- The substrate spec's tab-affordance sentence is deliberately normative ("must
  respond ... on the first interaction"), so this path is a gap against a
  stated invariant rather than undocumented territory.
