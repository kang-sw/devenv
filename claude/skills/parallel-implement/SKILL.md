---
name: parallel-implement
description: >
  Coordinate multiple implementer+reviewer pairs for parallel work
  across disjoint scopes. Handles worktree isolation, scope partitioning,
  and sequential merge coordination.
argument-hint: "<plan-path or ticket-path with multiple phases>"
---

# Parallel Implementation

Target: $ARGUMENTS

<!-- TODO: Design and implement this skill. Key concerns:
  - Scope partitioning: how to divide work across pairs
  - Isolation strategy: worktree per pair vs. branch per pair
  - Merge coordination: sequential merge ordering, conflict resolution
  - Review strategy: per-pair review, aggregate review, or both
  - Relationship to /implement: orchestration wrapper or independent flow
-->

## Invariants

- Each parallel unit is an independent `/implement` invocation.
- Scopes must be disjoint — no two units may modify the same file.
- Merge order is deterministic — units merge sequentially, not concurrently.
- User approves the aggregate result before final merge to target branch.

## Doctrine

Parallel implementation optimizes for **throughput with isolation** —
maximize concurrent work while preventing interference between units.
When a rule is ambiguous, apply whichever interpretation better preserves
isolation between parallel units.
