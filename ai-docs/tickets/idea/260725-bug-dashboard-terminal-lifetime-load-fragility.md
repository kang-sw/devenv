---
title: two pre-existing terminal_lifetime tests fail reproducibly under CPU-saturation load
related:
  260725-bug-dashboard-terminal-platform-macos-unsupported: found-during
---

# two pre-existing terminal_lifetime tests fail reproducibly under CPU-saturation load

## Background

Two pre-existing tests in `ws-dashboard/crates/daemon/tests/terminal_lifetime.rs`
fail reproducibly under CPU saturation while the rest of the file's tests
pass:

- `terminal_live_pty_eof_exit_flips_status_to_exited` (`:825`)
- `terminal_boot_reconcile_adopts_grace_row_and_delivers_final_output_on_reattach`
  (`:497`, fails with `left: String("running")`, `right: "exited"`)

Measured at 20-way spin load on a 16-core host:

- Full 4-test target: 7 runs / 7 FAILED.
- Same target with the new close-kill test `--skip`ped (restoring
  pre-Phase-2 parallelism): 3 runs / 3 FAILED.
- New close-kill test alone, `--exact`: 8 runs / 8 PASSED.
- Full target unloaded: 7 runs / 7 passed.

Attribution is solid two ways:

1. Phase 2's test commit `23321137` is purely additive to the file — a
   single hunk `@@ -853,3 +853,125 @@` appended after the last pre-existing
   test, editing neither failing test.
2. The indirect-starvation hypothesis (new test crowding out the other three
   under `cargo test`'s default parallelism) was tested and refuted by the
   `--skip` experiment above: the failures persist even with the new test
   removed from the run entirely.

So this is a pre-existing load-fragility defect in the two named tests, not
a regression introduced by Phase 2, and not caused by resource contention
with the new close-kill test.

Consequence to record: this target is not load-robust and should not be
wired into CI that runs alongside other jobs until fixed.

## Phases

### Phase 1: Diagnose and fix load sensitivity in the two named tests

Root-cause why each test's timing assumptions break under CPU saturation
(likely a too-tight poll deadline or an assumption about scheduling latency
that does not hold when the host is contended) and either widen the
deadline/retry margin or restructure the assertion so it is robust to
scheduler delay, following this file's existing "err generous" margin
philosophy for other polls.
