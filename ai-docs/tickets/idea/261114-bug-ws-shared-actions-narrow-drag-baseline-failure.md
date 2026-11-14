---
title: Shared-actions narrow drag assertion also fails on plain develop
---

# Shared-actions narrow drag assertion also fails on plain develop

## Background

The authorized 649 candidate/plain comparison reproduced the same board
420x900 drag failure on plain develop
`9854ff350fa1b0eccfb5c71649c5190c405925b9`, without 649 or 652.
This is a separate follow-up, not evidence that 649 introduced the covered
board-drag failure or that the product drag implementation is necessarily wrong.

Both original gestures start at (58.671875, 268), end at (30, 351), and move
in 12 steps. Both enter group drag, then end unpinned without `data-bdrag`.
The preview timestamp remains equal to the original `sharedDragAt`; relative
order remains unchanged. The original conjunction is
`changedAt=false`, `unpinned=true`, `noDragFlag=true` on both sources.
Whether the gesture, fixture expectation or actual ordering policy is wrong
remains unproved. No assertion was weakened and no fix was attempted.

Evidence: `/tmp/compact-panel-649-700-followup/` contains
`narrow-{candidate,plain}-board/diagnostics.json`, `comparison.json`, raw logs
and screenshots. The terminal report is
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@08d59332/review-paths/1de23d1e-01-compact-header-649-followup-terminal.md`.

## Constraints

- Preserve actual pointer gestures, independent pin/drag-end checks and
  native error collection; do not replace interaction with a synthetic reorder.
- The 649 adapter menu failure is candidate-only at an earlier checkpoint and
  remains a different blocker. A later plain drag failure does not clear it.
- Further runtime diagnosis needs the shared heavy slot and bounded approval.

## Phases

### Phase 1: Attribute the baseline narrow drag mismatch

Trace the retained pointer/order/timestamp evidence against the current drag
policy and harness predicate. Determine whether the expected reorder is valid
for that actual gesture before proposing a product or fixture correction.
Define a focused regression check with raw before/after order and native hit
ownership; do not infer root cause from the failing conjunction alone.
