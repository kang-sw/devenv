---
title: Concurrent pooled worktree acquires can select the same checkout
---

# Concurrent pooled worktree acquires can select the same checkout

## Background

Observed during dogfooding on 2026-11-15: two independent `worktree.acquire`
requests ran in one parallel tool batch, both using profile `full` and base
`81382b093`, with different existing target branches. One returned a successful
lease for `decree-thinness-strum`; the other failed while switching branches in
that same checkout because its Git `index.lock` already existed. Independent
acquires should not provision or switch the same idle checkout concurrently.

The successful checkout was subsequently verified clean on its intended branch
and original head. A sequential retry of the second request selected another
checkout successfully. No lock was removed, no worker was dispatched into the
contended checkout, and no dirty work was discarded.

## Observed evidence

- First target: `impl/feat/unified-history-task3/flask-churn-crib`; successful
  lease returned `decree-thinness-strum`.
- Second target: `impl/develop/model813-label`; failure named
  `.git/worktrees/decree-thinness-strum/index.lock` and refused `git switch`.
- Read-only status after both calls: the first checkout remained clean at
  `45ed47acccf3d46bce68e66c6adebc611e5723c6` on its intended branch.
- Sequential retry returned `dynasty-gawk-emphatic` for the second branch.

These observations establish overlapping selection, not its locking root cause.

## Constraints

- Preserve existing leases, dirty work, profile matching and checkout safety.
- Do not remove an unknown Git lock or steal an occupied branch as recovery.
- Investigation and implementation remain unscheduled; this idea records the
  tool surprise, not a selected synchronization mechanism.

## Phases

### Phase 1: Diagnose and prevent overlapping checkout selection

Trace idle-pool selection and lease publication under concurrent acquires.
Ensure independent successful requests cannot provision or switch the same
checkout at once. Add a deterministic concurrent-acquire regression covering
same-profile requests with distinct target branches, plus failure recovery and
existing lease safety. The synchronization mechanism is not yet selected.
