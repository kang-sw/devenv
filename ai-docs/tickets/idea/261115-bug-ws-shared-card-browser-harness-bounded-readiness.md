---
title: Shared-card browser harness lacks bounded reusable fixture readiness
related:
  261114-bug-ws-shared-actions-terminal-fixture-reconciled-away: Existing terminal-authority residual; do not duplicate its diagnosis or waive assertions
  261114-refactor-ws-dashboard-whiteboard-asset-split: Preserve harness command behavior and carry this gap through the move map
---

# Shared-card browser harness lacks bounded reusable fixture readiness

## Background

MAIN requested this idea capture after UI-T5 browser preparation could not supply a credible five-minute recipe. Static inspection at pinned repair `7e3abb6e0d84f71da292a83f2d89aec7fe9a3c90` found that `scripts/board-headless.mjs serve --shared-actions` always builds Go and starts/stops its own isolated daemon, has no total deadline or prebuilt/reuse entry option, and does not seed the exited terminal required by `shared-controls`. Browser action timeouts can accumulate. These are source-read findings, not a newly executed failure or measured runtime cost.

UI-T5's obsolete badge predicates are repaired separately; that source change does not make browser fixtures ready. Its browser coverage remains unallocated. Full static evidence and exact existing entry points: `52f38f1b-01-review794-c2-t5-pinned-commit-browser-readiness.md` in the project review cache. Existing terminal reconciliation and narrow-drag failures retain their original classifications.

## Constraints

- This is an idea, not implementation, fixture startup, runtime permission or a new acceptance gate. Settle any command/interface or fixture design before promotion.
- Preserve shared-controls/shared-actions predicates, renderer-owned fixtures, terminal authority and unrelated sessions. Do not manufacture acceptance with removed assertions, browser-local terminal substitutes or relabeled historical failures.
- Keep fixture lifecycle, build/cache preparation, total deadline and actual owned-process quiescence explicit; do not use the live owner dashboard or infer daemon-control authority from a test assignment.

## Phases

### Phase 1: Establish bounded shared-card fixture readiness

Define a minimal, explicitly authorized path to run the existing shared-controls/shared-actions coverage at 1800px and 420px with coherent renderer fixtures and exited-terminal prerequisites. Account for preparation/build reuse, fixture ownership, cancellation and final process evidence within a cumulative budget. Coordinate the terminal-specific work with the related existing ticket; this ticket owns orchestration/readiness rather than duplicating that diagnosis. Preserve pending coverage and distinguish diagnostic partial runs from completed acceptance. No runtime work is authorized by this capture.
