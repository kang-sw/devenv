---
title: "implement route reuses a prior phase branch despite a create verdict"
related:
  260911-feat-ws-git-merge-lead-owned-merge-authority: surfaced while routing its second phase
---

# implement route reuses a prior phase branch despite a create verdict

## Background

While starting Phase 2 of an in-progress ticket, the implementation route
returned a create verdict but selected the still-existing impl branch used for
Phase 1. Reusing that branch would mix a completed phase's retained history with
the new phase and make the route's create instruction impossible to follow
literally.

The worker preserved isolation by creating a fresh random impl branch from the
assigned goal branch and continued successfully. The route output itself still
needs investigation because a later worker should not have to override its
resolved branch target to obtain the isolation promised by the verdict.

## Evidence

- Phase 2 routing selected the prior `marry-hurt-list` branch at `70117c27`
  while reporting that a branch should be created.
- The worker instead used the fresh `cedar-river-lantern` branch and recorded
  that deviation in the Phase 2 result.
- The fresh branch passed the full Go suite, MCP smoke, both Python package
  suites, and independent correctness, fit, and test review.
