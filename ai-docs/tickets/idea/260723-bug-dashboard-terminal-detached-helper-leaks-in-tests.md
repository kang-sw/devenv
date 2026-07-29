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

## Field evidence (2026-07-27)

The accumulation this ticket predicted was measured on a dev box after three
days of unswept runs, and it is larger than "several" survivors per pass:

- 489 orphaned `terminal-helper` processes, all reparented to PID 1, plus
  roughly 490 shell children — about 980 of the machine's 1941 processes.
- Accrual rate by start day: 102 (Jul 25), 192 (Jul 26), 195 (Jul 27). Oldest
  survivor had been up 2d 9h 34m.
- 1550 stale `/private/tmp/ws-dash*` registry directories, 1510 of them
  `ws-dashboard-terminal-registry-*`, totalling 2.1M.
- Zero of them were real instances: every registry path was a test temp dir,
  and no surviving process used the real state home.

Two facts worth not re-deriving. First, the cost is **not** load: all 489 sat
at 0.0% CPU and 0.0% memory, with the machine at load 4.67 on 16 cores, 92%
memory free and zero swap. The hazard is process-table and `/tmp` occupancy —
stale-pid detection and registry scans can see all of it — so flaky or slow
runs must not be attributed to this leak, and this leak must not be inferred
from a slow machine. Second, SIGTERM is sufficient: 489/489 exited on TERM
with no SIGKILL needed, so the helper's graceful path is intact in the leaked
state and a sweeper does not need force.

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

The stopgap is now half-taken, and only half: `_index.md` gained a
`## Dashboard Test Hygiene` section carrying the sweep commands and the
not-load caveat, so the risk is written down where a session sees it before
running the suite. That is documentation plus a manual sweep, not a fix — it
depends on every session remembering, which is the discipline that already
failed for three days. The three fix directions above stay undecided.
