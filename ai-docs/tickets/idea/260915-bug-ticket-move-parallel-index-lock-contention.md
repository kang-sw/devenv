---
title: "Concurrent ticket moves in one worktree contend on the Git index lock"
---

# Concurrent ticket moves in one worktree contend on the Git index lock

## Background

Dogfood observation on 2026-09-15: three `tickets.move(..., to: "ready")` calls were dispatched concurrently against the same worktree. Two failed at `git add` with `index.lock: File exists`, while the third completed and moved its ticket. The caller then had to move the successful ticket back to its original status before retrying the batch serially.

This makes a seemingly independent multi-ticket promotion non-atomic at the caller level and exposes a raw Git lock failure. The ticket records the concurrency surprise only; serialization, lock-aware retry, and explicit same-worktree concurrency rejection remain implementation choices for later settlement.

## Phases

### Phase 1: Make same-worktree ticket mutations fail coherently under overlap

Reproduce concurrent ticket status mutations against one worktree, choose a bounded concurrency contract, and ensure a batch caller cannot be left with one successful status move plus sibling raw `index.lock` failures without actionable recovery guidance. Preserve independent operations across separate worktrees.
