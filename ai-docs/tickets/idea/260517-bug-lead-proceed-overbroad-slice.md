---
title: lead-proceed overbroad implementation slice selection
related:
  260517-feat-ws-dashboard-workroot-activity: dogfood case where a cohesive product surface hid separate implementation blast radii
related-mental-model:
  - workflow-skills
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
