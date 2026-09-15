---
title: "Serial lead-run worker can acquire an unrequested second worktree"
related:
  260910-feat-lead-run-worktree-parallel-route: defines worktree provisioning as an explicitly user-gated parallel-route behavior
---

# Serial lead-run worker can acquire an unrequested second worktree

## Background

Dogfood observation on 2026-09-15: the lead acquired one isolated worktree and explicitly selected the serial `lead-run` route with no additional worktree. The spawned ticket worker nevertheless called `worktree.acquire`, created a second pooled worktree, and implemented on that checkout. The lead's acquired checkout remained on the parent goal branch while the worker reported an impl branch in the second worktree.

The serial lead-run contract says the worker checkout is shared and outlives the turn; worktree provisioning belongs to the separately approved parallel route. The ticket records the deviation and the resulting branch-location surprise. The enforcement location is not yet settled.

## Phases

### Phase 1: Keep serial ticket workers on the assigned checkout

Reproduce a serial ticket-worker run with `worktree.acquire` visible in its lead-capability tool profile, establish a mechanical or procedural guard that prevents unapproved provisioning, and verify the parallel route can still acquire one worktree per approved ticket. The lead and worker must report the same checkout and branch location in serial mode.
