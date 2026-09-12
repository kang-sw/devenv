---
title: "Workflow-cost comparison cannot handle disjoint unrepresentative windows"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-bug-workflow-cost-measurement-manual-round-three-findings: corrected before baseline and measurement contract
dropped: 2026-09-12
---

# Workflow-cost comparison cannot handle disjoint unrepresentative windows

## Background

The refoundation after-removal measurement was attempted at
`develop@cb0458151aecf4979dd05b6c7e702ebbcbf755a2` against the corrected
before baseline at `develop@84b1f825`. Both fixed-size windows were
unrepresentative because the selector skipped more tickets without
`completed:` than `SIZE=20`: 48 before and 51 after.

The manual requires an honest comparison in this state to use the calendar
period shared by the two windows. The before closures span 2026-08-30 through
2026-09-09; the after closures span 2026-09-11 through 2026-09-12. There is no
shared period, and all 20 after stems are new. The run therefore stopped before
indicators 1-6 instead of substituting an undocumented proxy. The epic's
after-measurement Completion Criterion and Dropped criterion remain
unestablished.

## Phases

### Phase 1: Define a comparable disjoint-window contract

Reconcile the measurement procedure with the case where both fixed-shape
windows are unrepresentative and have no shared calendar period. The resulting
contract must preserve explicit limitations, keep both halves on the same
declared window shape, and either produce the comparison needed by the epic or
state a revised completion condition that does not imply unmeasured parity.


## Resolution (2026-09-12)

No measurement-contract change was needed. The existing manual permits changing SIZE when both halves use the same value. Recomputing both pinned records at SIZE=51 produced full representative windows with a shared closure period and a complete comparison, recorded in ai-docs/ref/refoundation-workflow-cost-measurement-260912.md.
