---
title: Concurrent worktree acquisition selects the same pooled checkout
---

# Concurrent worktree acquisition selects the same pooled checkout

## Background

During 2026-11-15 dogfooding, two independent `worktree.acquire` calls ran concurrently from session `boxer-nest-joyous`, both using base `develop`, profile `full`, and different target branches. Both selected pooled checkout `dense-hardship-outclass`. The permission-profile acquisition failed to create its index lock, while the worker-profile acquisition succeeded and returned key `carve-unwritten-ravage`. A sequential retry of the failed acquisition succeeded in a different checkout (`fidelity-precision-simplify`).

Expected caller behavior is that independent parallel acquisitions reserve distinct available pool entries, or serialize safely without surfacing an incidental Git index-lock failure. This report does not establish the locking mechanism or whether other pool operations are affected.

## Constraints

- Preserve unrelated leases, checkout state, and user artifacts.
- No lock-file deletion or repair was attempted during this incident.
- Investigate source and reproduce with an isolated pool; this ticket does not choose an acquisition-locking design.

## Phases

### Phase 1: Investigate and prevent concurrent pool-selection collision

Trace pool selection, reservation, and checkout boundaries for concurrent acquisitions. Reproduce the observed same-checkout selection with independent target branches in an isolated fixture. Implement the smallest justified fix after the concurrency contract is settled, and verify concurrent acquisitions either return distinct safe leases or serialize without an incidental Git lock error. Cover cleanup after a failed acquisition without affecting a successful sibling.

## Resolution
Duplicate of 261115-bug-ws-worktree-acquire-concurrent-pool-claim (owner-approved FIX tier (1060), assigned to EXEC3); evidence kept here for that ticket.
