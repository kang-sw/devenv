---
title: "dashboard daemon integration tests leak detached terminal helper processes: routes.rs has no reaper"
related:
  260725-bug-dashboard-terminal-platform-macos-unsupported: found-during
  260723-bug-dashboard-terminal-detached-helper-leaks-in-tests: prior-art
---

## Symptom

`ws-dashboard/crates/daemon/tests/routes.rs` spawns terminals through the
detached-helper path but has no reaper. `tests/terminal_lifetime.rs` DOES
have a `HelperReaper` (`Drop`-based) that cleans up any helper the test
forgot to close — `routes.rs` has no equivalent, so every `routes.rs` run
leaks roughly 8 `ws-dashboard terminal-helper` processes.

The leaked processes do not self-expire: `serve_connections` loops on
`IDLE_ACCEPT_POLL` indefinitely for as long as its shell lives, and each
leaked helper still holds a PTY and an interactive shell. Observed
accumulation: 81 orphaned `ws-dashboard terminal-helper` processes were live
on the dogfood host at review time, accumulated across repeated runs.

This is platform-independent — Linux leaks identically. It is NOT a
macOS-port defect.

## Finding

- Discovered during the cycle-2 correctness review of ticket
  `260725-bug-dashboard-terminal-platform-macos-unsupported` Phase 1 (commit
  range `c17a982b..11a02e46`), while doing a process census to verify a
  different finding. It did not gate that phase.
- This overlaps with the general leak pattern already captured in
  `260723-bug-dashboard-terminal-detached-helper-leaks-in-tests` (any caller
  that creates a terminal and never explicitly closes it leaks a real OS
  process). That ticket's "Fix direction" section proposed a test-only
  `Drop`-based teardown guard; `terminal_lifetime.rs`'s `HelperReaper` is
  that guard, implemented for one test file only. `routes.rs` has ~8
  terminal-creating tests and none of them route through a reaper, so the
  leak observed here is the un-backported remainder of that earlier finding,
  now with a much larger observed accumulation (81 vs. the earlier ~4).
- Likely fix shape (not decided): `terminal_lifetime.rs`'s `HelperReaper` is
  the existing in-repo precedent, and as of the same phase it routes through
  the cfg-independent `terminal_platform::process_start_time` re-export
  rather than a local `/proc` mirror — so a shared, extracted reaper helper
  usable from both `routes.rs` and `terminal_lifetime.rs` is plausible.
  Leave the actual design (shared helper module vs. per-file duplication vs.
  a process-wide sweep at test-binary exit) open for triage.

None of the above is decided; this ticket captures the finding for triage.
