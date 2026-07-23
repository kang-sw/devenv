---
title: "dashboard terminal: detached helper processes leak indefinitely when a caller never explicitly closes the terminal"
related:
  260723-feat-dashboard-terminal-lifetime-daemon-decouple: introduced-by
---

## Symptom

Surfaced while dogfooding Phase 1 of
`260723-feat-dashboard-terminal-lifetime-daemon-decouple` (terminal PTY
lifetime moved out of the daemon into a detached per-terminal helper
process). Running `cargo test -p ws-dashboard-daemon` (all three suites:
`--lib`, `--test routes`, `--test server`) leaves several real
`ws-dashboard terminal-helper` OS processes (plus their spawned shell
children) running indefinitely after the test binary exits, confirmed via
`ps aux | grep terminal-helper` immediately after a full green test run —
e.g. four survivors after one `cargo test -p ws-dashboard-daemon` pass, each
traced back to a `routes.rs` test that creates a terminal (via
`create_terminal_for_test` or an inline create call) and never calls the
`DELETE /api/dashboard/terminals/{id}` route before the test function
returns (e.g. websocket-attach tests that only `server.abort()` the
in-process HTTP server, never close the terminal itself).

This is a direct, intended architectural consequence of Phase 1: before this
change, dropping the in-process `TerminalRegistry`/`TerminalSession` at test
teardown implicitly closed the PTY master and killed the shell (`Arc` drop
semantics). After this change, the PTY lives in a detached, `setsid` +
double-forked helper process that by design does *not* die when the daemon
(or, in tests, the whole test process) goes away — that survival is the
entire point of the ticket. So this is not a regression in the "worktree/
workspace removal kills its own shells" contract (risk signal 1, verified
separately: those three removal routes now `.await` `session.terminate()`
directly instead of a `tokio::spawn`-detached fire-and-forget, see the
sibling fix in the same phase's commit), but a distinct gap: any caller
(test or otherwise) that creates a terminal and never explicitly closes it
now leaks a real OS process forever instead of being implicitly cleaned up
by process/scope exit.

## Finding

- Confirmed leak sources are exclusively tests that create a terminal via
  `crates/daemon/tests/routes.rs`'s `test_terminal_registry()` /
  `create_terminal_for_test()` helpers and never reach a
  `DELETE /api/dashboard/terminals/{id}` call before the test function ends
  (most WS-attach and cross-linked-server-forwarding tests fit this shape).
- The dedicated E2E test added in this phase
  (`crates/daemon/tests/terminal_lifetime.rs`) does *not* leak, because it
  explicitly closes its terminal and hard-kills both daemon processes before
  returning — proving the leak is caller discipline, not a helper-side bug.
- Helper processes have no self-imposed idle/orphan timeout independent of
  the daemon-driven grace-reattach window (`GRACE_WINDOW` only starts once
  the *shell* has exited, not while it's sitting idle with no attached
  daemon at all) — a genuinely idle-forever shell with no daemon ever
  reconnecting again will never self-terminate.
- In a real production deployment this is arguably fine or even desired
  (that is the feature), but it is a real hazard for CI/dev-machine hygiene:
  repeated local `cargo test -p ws-dashboard-daemon` runs accumulate
  orphaned shell processes over time with no automatic reaper.

## Fix direction (not decided)

- Test-side: add a small test-only teardown fixture (e.g. a `Drop` guard
  registered per `test_terminal_registry()` call, or a process-wide registry
  of created socket/registry paths swept at test-binary exit) that issues
  `GracefulShutdown`/verified-kill for any terminal a test forgot to close.
  Keeps the leak from accumulating in CI without touching the ~10+
  individual test bodies that would otherwise each need an explicit
  `DELETE` added.
- Helper-side (production hygiene, separate from tests): consider a
  much-longer idle-with-no-daemon-ever-reconnected ceiling (e.g. hours, not
  the 30s post-exit grace window) so a helper whose daemon is permanently
  gone (uninstalled, reconfigured to a different state dir, etc.) does not
  run forever. Needs care not to conflict with the legitimate
  "daemon restarts much later" resume story that is this ticket's whole
  point.
- Lowest-effort stopgap: document the leak risk and rely on OS-level
  process/session cleanup (e.g. logout, reboot) — likely insufficient for
  long-running dev boxes and CI runners that never reboot between runs.

None of the above is decided; this ticket captures the finding for triage.
