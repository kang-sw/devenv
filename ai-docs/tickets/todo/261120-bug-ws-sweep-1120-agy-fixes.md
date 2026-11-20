---
title: "리뷰 스윕 1120: agy watchdog·TPS·사용량 결함 수정"
---

# Review sweep 1120: agy watchdog, output-rate, and plan-usage fixes

Block ticket for the review range `770ab243d..3d380a90b`. The findings below
come from the range review (codex reviewers, correctness / fit / test
partitions; fit was clean).

## Findings

### Critical

1. `agents-plugin-tool/internal/wsagent/agy_watchdog.go:227` - the first
   model-wait timeout clears its timer, sets `modelWaitCancelling`, interrupts
   the process, then waits only for `TurnEnd` before `retryModelWait`. If the
   stalled agy stream emits no `TurnEnd` after the interrupt, no timer remains
   to force-stop the process or fail the turn; the parent waits forever in the
   exact failure mode the watchdog exists to recover.

### Important

2. `agents-plugin-tool/internal/wsagent/record.go:611` - `flushSnapshot`
   returns when the first pending output-rate sample arrives on a hosted slot
   before `lastDiskAt` is set, suppressing both the snapshot write and
   `outputRates.Append`. A first steered completion not followed at once by
   `TurnEnd` stays RAM-only, breaking the merge's immediate-persistence and T2
   reopen-recovery claim.
3. `agents-plugin-tool/internal/wsdashboard/plan_usage_agy.go:419` - numeric
   `reset_time` accepts any positive finite `float64` and converts it to
   milliseconds without an `int64` / JavaScript `Date` range check; a huge
   value reaches `new Date(...)` as an invalid date. Drop the malformed bucket.
4. `agents-plugin-tool/internal/wsagent/agy_watchdog_test.go:372` - the
   `/compact` exclusion test toggles `compactCommand` directly and never runs
   the real compact transition; add a test over the caller-visible path.
5. `agents-plugin-tool/internal/wsdashboard/plan_usage_agy_test.go:24` - the
   fake-agy harness skips on Windows while the range adds Windows
   process-group configuration and shutdown draining; add Windows-runnable
   coverage of the probe cancellation path.

## Constraints

- Manuals: `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Compatibility tier T0 for every fix (internal behavior and tests only).
- Verification: `go test ./internal/wsagent ./internal/wsdashboard` green;
  re-run the range review over the original base with the fix commits in the
  head.
