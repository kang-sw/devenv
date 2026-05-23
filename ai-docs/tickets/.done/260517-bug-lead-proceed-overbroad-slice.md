---
title: lead-proceed overbroad implementation slice selection
related:
  260517-feat-ws-dashboard-workroot-activity: dogfood case where a cohesive product surface hid separate implementation blast radii
related-mental-model:
  - workflow-skills
completed: 2026-05-23
---

# lead-proceed overbroad implementation slice selection

## Background

During `260517-feat-ws-dashboard-workroot-activity` routing, `lead-proceed`
initially selected the whole ticket because the phases formed one cohesive
product surface. The user pointed out that implementation blast radius should
also influence slice selection: the ticket's phases split into daemon
projection, top-bar UI, and workbench pane placement surfaces.

Investigate whether `lead-proceed` should choose narrower implementation slices
for `todo` or `ready` tickets when phase boundaries map to distinct code,
verification, or review surfaces, even if the product goal is cohesive.

The expected improvement is a conservative routing rule: when phase boundaries
separate backend/API, visible UI, workbench placement, browser-gate, or other
review surfaces, default to the first unfinished phase unless adjacent phases
are inseparable from ticket artifacts.

## Phases

### Phase 1: Enforce one proceed phase

Close the dogfood follow-up by confirming that `lead-proceed` now resolves one
ticket phase per invocation instead of grouping adjacent phases by product
cohesion.

### Result (a0df5510) - 2026-05-23

`a0df5510` changed full ws and wsflow `lead-proceed` so an explicit single phase
is honored exactly, no explicit phase selects the first unfinished phase, and
multiple named phases stop for slicing. The workflow spec and workflow-skills
mental model now carry the same rule, so the original overbroad-slice dogfood
issue is stale and closed.
