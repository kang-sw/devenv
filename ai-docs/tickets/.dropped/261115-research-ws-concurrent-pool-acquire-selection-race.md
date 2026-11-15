---
title: Investigate concurrent worktree pool acquisition selection
---

# Investigate concurrent worktree pool acquisition selection

## Background

Two independent baseline/candidate verification checkouts were requested concurrently through `worktree.acquire`, both using the full profile and different target branches. One failed with a Git index-lock collision at the same pooled path the other returned successfully. Investigate whether pool selection and reservation are atomic across concurrent calls. This capture authorizes no tooling or workflow behavior change.

## Outcome Ledger

### Verified Findings

- In session `useable-curvature-immovably`, the parent submitted two `worktree.acquire` calls in one parallel tool batch on 2026-11-15. The bases were `d0ad8a9f1863e15b2bcc2d5f2d22eb062c920c14` and `e124d76e932a015470a20e6f6f5339459beca700`; the target branches were distinct.
- The baseline call returned a Git failure creating `index.lock` in a pooled worktree's Git admin directory. The candidate call returned that same pooled worktree successfully with a worker key.
- After the candidate acquisition completed, a serial baseline acquisition succeeded at a different pooled path. No lock was removed or unknown process signaled. The verification worker was instructed to verify exact clean pins before execution.
- These are observed tool responses, not proof of the internal race mechanism or a claim that the resulting candidates were corrupted. The parent session transcript contains the parallel batch and subsequent serial acquisition.

### Confirmed Decisions

None. The existing acquisition contract remains unchanged.

### Proposals

Inspect pool selection, lease reservation and cross-call synchronization before selecting a fix or documenting a concurrency restriction.

### Open Questions

- Can two concurrent acquisitions select the same idle worktree before either reserves it?
- Does an index-lock refusal leave any branch or pool state requiring reconciliation?
- Which deterministic concurrency regression would distinguish safe independent provisioning from same-path selection?

### Rejected Alternatives

No product alternatives were settled. Deleting the lock or assuming pins from a pooled path was not used as recovery.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 같은 병렬 acquire 경쟁 문제의 중복 조사 티켓; 261115-bug-ws-worktree-acquire-concurrent-pool-claim.
