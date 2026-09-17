---
title: Exec MCP Windows test-timing margins recur as release smoke flakes
related:
  260916-bug-exec-mcp-windows-tempdir-cleanup-race: prior recurrence of the same class (test-only, no production bug)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d71808cf038e2818
sage-review-completeness-reviewed: d71808cf038e2818
completed: 2026-09-17
---

# Exec MCP Windows test-timing margins recur as release smoke flakes

## Background

The `Windows ws-mcp smoke` release job fails on nearly every release and is
hand-patched each time, yet the failure keeps coming back on the next release
(v0.46.9 run 35195663913 failed on `Run Go tests (Windows)`). A git-history
investigation established the cause: it is **not one test and not a production
bug**, but a rotating set of individually-patched timing margins in the exec
test suite, all sharing one fragile shape.

Every affected test:

1. launches a subprocess whose completion time is a **fixed nominal duration**
   (`mcpLongShellArgs` ≈ `sleep 6` / `ping -n 7`, `mcpAbortShellArgs` sleep-30,
   busy-loops writing a fixed byte count), then
2. waits for it via a **fixed wall-clock budget** — `execjob.ForegroundWindow`
   (5s synchronous launch window) plus an explicit `exec.result`
   `timeout_seconds` — rather than an open-ended poll-until-done, and
3. the margin between (1) and (2) was hand-tuned against a "normal"
   `windows-latest` runner with only ~2s of slack.

`windows-latest` is meaningfully slower and more variable than the tuning
assumed (this repo's own history shows 46-minute hangs that forced job-timeout
caps, and repeated antivirus / cold-start `ForegroundWindow` overruns). Under
load, whichever test has the thinnest margin that day fails; the fix each
release widens *that one test's* budget and stops there without auditing the
siblings using the identical pattern, so the next release a different test
fails. Genuine production races were found and fixed twice
(active-map/mutex ordering; `waitProcessExit` replacing a probabilistic sleep)
and do **not** recur — the undying part is exactly this test-timing-margin
class, which the engineers' own commit/ticket text has three times called
"test-only, no production exec bug".

The v0.46.9 instance: `TestExecMCPResultReadableJSONStdoutAndTimeout`
(unchanged since it was written) launches the ~6s `mcpLongShellArgs` job, then
has only 5s (`ForegroundWindow`) + 3s (`timeout_seconds`) = 8s of budget and
asserts `succeeded` unconditionally with no "still running" fallback. Its
`stdout_bytes: 0` at assertion time means even process spawn was slow under CI
load. The commit that most directly touched this coupling explicitly recorded
(under "Rejected") that this test's sleep-6-inside-timeout-3 margin was
load-bearing and fragile, worked around it for a sibling test by giving that
sibling its own helper, and left this test's thin margin in place — a risk
written into a commit message but never enforced where the next editor would
see it.

## Decisions

- **Fix approach: replace fixed-sleep-vs-fixed-budget races with
  poll-until-terminal.** For each affected test, call `exec.result` with a
  budget that is a generous multiple of (`ForegroundWindow` + the job's nominal
  duration) and assert on the *terminal* result, instead of asserting success
  off a tight margin with no running-state fallback. This is exactly the
  pattern already applied to the sibling `TestExecMCPRunningLargeAndAbort` in a
  prior release, so it is precedent-consistent and the lowest-risk path; it
  trades a few seconds of suite time for removing the load-dependent race
  entirely.
- **Scope is the class, not just the failing test.** Fixing only
  `TestExecMCPResultReadableJSONStdoutAndTimeout` reproduces the exact
  per-test-patch behavior that makes this recur. Audit every exec test in
  `agents-plugin-tool/internal/mcp/` and `agents-plugin-tool/internal/execjob/`
  that launches a fixed-duration subprocess and waits on a fixed budget
  (the `mcpLongShellArgs` / `mcpAbortShellArgs` / `mcpLargeShellArgs` helper
  users are the entry set), and convert each to poll-until-terminal in this
  pass.
- **Rejected alternatives.** Widening only the failing test's budget (the
  status quo that recurs); `t.Skip` on Windows CI (drops coverage on the exact
  surface most likely to hold Windows-specific bugs); job-level retry-on-failure
  (masks the margin bug instead of fixing it). A shared per-runner time-scale
  env knob and splitting the suite for `t.Parallel()` were considered but are
  broader than this bug needs and risk new cross-test process/PID interaction;
  the suite-duration/`t.Parallel()` angle is a possible separate follow-up, not
  part of this fix.
- **No production code change is expected.** `execjob.Abort` and
  `execjob.ResultWithTimeout` are already deterministic/poll-based; the defect
  is entirely in the test harness's fixed-margin assumption. If the audit
  surfaces a real production timing gap, that is a separate ticket.

## Constraints

- Edits under `agents-plugin-tool/internal/mcp/` read
  `ai-docs/manuals/ws-mcp.md`; `internal/execjob/` timing constants
  (`ForegroundWindow`) are the shared budget input any converted test must size
  against.
- Verification must include the Windows path, which is where the margin bites;
  a green local Linux run is necessary but not sufficient. Confirm the
  `windows-smoke` release job (or an equivalent Windows `go test ./...` run) is
  green after the change.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server_test.go (multiple test funcs: TestExecMCPResultReadableJSONStdoutAndTimeout at L2818, TestServeStdioDoesNotBlockToolsListBehindLongCall at L1803, and any other mcpLongShellArgs/mcpAbortShellArgs/mcpLargeShellArgs user found by the audit); agents-plugin-tool/internal/execjob/execjob.go read-only for ForegroundWindow (L27) |
| scope.surface | internal | no exported symbol changes; edits confined to _test.go files |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none proposed; any shared poll helper stays an internal test utility |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/server_test.go (existing file, no new test files planned) |
| complexity.reuse_points | confirmed | poll-until-terminal pattern already landed in TestExecMCPRunningLargeAndAbort (server_test.go#L2662-2777, exec.result timeout_seconds:30 + reapExecKeys) |
| complexity.side_effect_risk | low | test-file-only edits; Decisions section states no production code change is expected |
| risk.correctness | moderate | audit scope is open-ended (every fixed-margin exec test) and the precedent test's t.Cleanup/reap ordering is subtly LIFO-dependent (server_test.go#L2670-2689 comment), so a converted test can still get budget or cleanup ordering wrong |
| risk.fit | low | converts to a pattern already used in the same file for the same helper family |
| risk.test | moderate | the failure mode under fix is Windows-CI-load timing itself; a green local Linux run is explicitly insufficient (this ticket's own Constraints), and real verification needs Windows CI or an equivalent loaded Windows runner |
| risk.security_or_contract | low | test-only change; no public API, security, or contract surface touched |

## Phases

### Phase 1: Convert exec tests to poll-until-terminal

Audit the fixed-margin exec tests (start from the `mcpLongShellArgs` /
`mcpAbortShellArgs` / `mcpLargeShellArgs` users in
`agents-plugin-tool/internal/mcp/`) and convert each — beginning with the
v0.46.9 failure `TestExecMCPResultReadableJSONStdoutAndTimeout` — to wait on a
generous poll budget and assert the terminal result, removing tight-margin
unconditional success assertions. Enumerate the target set concretely by
grepping the three helper names (the audit is bounded, not open-ended), and when
converting preserve the reap-before-`t.TempDir()`-cleanup ordering the precedent
relies on (the LIFO `t.Cleanup` caution at `server_test.go#L2670-2689`). Leave
production `execjob` behavior unchanged unless the audit proves a real timing
gap. Verify with the full `go test ./...` on both Linux and a Windows run, and
confirm the release `windows-smoke` job is green.

#### Result (1da0789d) - 2026-09-17

Landed in two commits on `impl/goal/develop/amber-quartz-lantern/suds-water-crowd`:

- `096ff2c5` widened `TestExecMCPResultReadableJSONStdoutAndTimeout`'s
  `exec.result` `timeout_seconds` from 3 to 30 (the v0.46.9 failure), matching
  the poll-until-terminal precedent in `TestExecMCPRunningLargeAndAbort`.
- `1da0789d` fixed two round-1 correctness-review findings the first commit's
  helper-name-bounded grep missed: `internal/execjob/execjob_test.go`'s
  `TestLongLargeAndAbort` has its own local `slow`/6s helper hitting the same
  pattern via `ResultWithTimeout(..., 2*time.Second)` (widened to 30s), and
  `TestServeStdioDoesNotBlockToolsListBehindLongCall`'s *passing* condition
  (not an assertion) depended on the same thin margin in the opposite
  direction — fixed by switching it to the longer-duration `mcpAbortShellArgs`
  helper plus a `t.Cleanup(reapExecKeys)` (also added to the primary test,
  matching the LIFO-ordering convention). Two independent review rounds
  (correctness + test partitions) both came back clean on round 2; no
  production `execjob` code changed.

Decisions taken during execution (none required escalation):
- Audit widened beyond the ticket's literal "grep the three helper names"
  instruction once round-1 review found the pattern recurring under a
  differently-named local helper (`slow`) in `internal/execjob/`, which the
  Decisions section's directory-level scope ("every exec test in ... and
  `agents-plugin-tool/internal/execjob/`") already covered even though the
  three-helper-name search didn't reach it.
- `TestServeStdioDoesNotBlockToolsListBehindLongCall` was judged not to need
  an assertion conversion (it never asserts on the spawned job's completion
  state), but did need a margin/cleanup fix for a different reason (its own
  pass condition, and the Windows tempdir-race class once the job is made to
  outlive the test).

Verification:
- `go test ./...` (agents-plugin-tool): all packages green except
  `TestMailboxInertByDefault` / `TestMailboxSelfAddressSurface`, confirmed
  pre-existing and unrelated (they read this machine's real
  `WS_MAILBOX_AUTO` identity; reproduce identically on the base commit before
  this ticket's changes).
- `go vet` / `gofmt -l` clean on both changed test files.
- `GOOS=windows go vet ./internal/mcp/...` clean (compile-only Windows check).
- **Windows execution could not be completed in this session.** The
  project's documented Windows smoke host (`ssh ki608@192.168.33.6`, per the
  `infra.windows-smoke-host` repo note) is reachable for read-only diagnostics,
  but every write action against it (creating a directory, `scp`) was denied
  by this session's own tool-permission classifier as a data-exfiltration
  risk, and the worker protocol forbids pushing this branch to trigger the
  `windows-smoke` release-CI job directly. This ticket's own Constraints treat
  a green Windows run as required, not optional ("necessary but not
  sufficient" alongside Linux) — **the lead should either grant this session's
  successor the SSH-write permission to complete a manual Windows `go test
  ./...` run, or push/PR this branch to trigger `windows-smoke` CI**, before
  treating this fix as fully verified and closing the ticket.

Observation carried forward (non-blocking, from round-2 review, not acted on
per the two-round cap): `server_test.go`'s
`TestServeStdioDoesNotBlockToolsListBehindLongCall` now has its thinnest
remaining margin at its own "ServeStdio did not exit after input close" 3s
budget (`server_test.go#L1868-1872`) against an `exec.result` call that now
always consumes its full `timeout_seconds:2` (previously it could return early
on the short job's completion). Not a regression and not part of this ticket's
target pattern, but flagged as the most likely next recurrence site if the
class resurfaces.

#### Edition (4c5c2d43) - 2026-09-17

Windows verification obtained and ticket closed. The lead merged the fix into
`develop` (merge `4c5c2d43`) for the ws/wsflow 0.46.10 release and ran the
release CI to get the Windows run the worker could not: `ws-mcp release` PR run
35224670350 passed **Windows ws-mcp smoke green (17m8s)** on `windows-latest`,
and the `v0.46.10` tag run re-confirmed it. This is the recurring flake's first
green release-smoke since v0.46.7 (v0.46.8 and v0.46.9 both failed on it). Fix
shipped in v0.46.10.
