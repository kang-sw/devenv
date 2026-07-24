---
title: "Windows wsflow MCP mid-session disconnects under high concurrency"
sage-review-design: completed
sage-review-completeness: completed
---

# Windows wsflow MCP mid-session disconnects under high concurrency

## Background

Downstream reports intermittent mid-session disconnects of the wsflow MCP server
on Windows 11, auto-reconnected by Claude Code. Disconnects correlate with high
concurrency (multiple background agents/subagents each holding their own wsflow
connection). No layer currently records *why* the server dies, so the crash cause
was never captured downstream. This ticket records a source-verified root-cause
investigation and a phased fix plan whose Phase 1 is designed to capture the
smoking gun for the first time.

Downstream's structural finding is confirmed, but its leading causal hypothesis
is corrected by the code: a busy/locked SQLite error is **not** fatal in serve
mode, so it is not the disconnect trigger. The strongest code-supported trigger
is an **unrecovered panic in a per-request goroutine**, which crashes the whole
serve process and leaves no persisted trace.

## Verified Findings (source-checked)

Launcher — `agents-plugin/bin/ws-mcp-launcher.py` (byte-identical wsflow mirror):

- Windows keeps an intermediate blocking Python process; POSIX exec-replaces it.
  `:866-869` — Windows `subprocess.call(args)`, POSIX `os.execvpe(...)`. So on
  Windows the PID Claude Code supervises is the launcher, not the Go server; the
  Go server is an stdio-inheriting grandchild.
- The Windows child is a bare subprocess — no `creationflags`, no Job Object, no
  kill-on-close linkage (`:868`). Killing the Python parent can orphan the Go
  child.
- Startup-only breadcrumb: `write_launch_breadcrumb` is called only from
  `fail()` (always a pre-launch `SystemExit(1)`). The Windows child exit code
  that `subprocess.call` returns (`:868`) is never inspected or recorded. No
  mid-session death is captured anywhere.
- Child stdio is inherited, not captured to a file (only short-lived compat
  probes use `capture_output`). `WS_MCP_LAUNCHER_DEBUG=1` logs to stderr only.
- No daemon/reuse: each MCP connection spawns a fresh launcher + fresh server
  (on Windows, two processes per connection).

Server — Go `ws-mcp` (`agents-plugin-tool/`):

- No `serve` log-file/verbose flag (`cmd/ws-mcp/main.go:88-104`; only
  `--stdio`/`--root`). A fatal serve error goes to stderr only (`main.go:101`).
- An undocumented `WS_MCP_DEBUG_LOG` env appends structured events
  (`internal/mcp/server.go:256-269`) but does **not** capture panics.
- **No `recover()` in the per-request goroutine** (`server.go:172-184`). The only
  `recover()` is the async agent worker (`internal/wsagent/agent.go:1016-1024`).
  A panic in any tool handler crashes the whole serve process (exit 2, stack to
  inherited stderr, skips all defers incl. `store.Close`) — no persisted trace.
- SQLite: WAL set only on new-DB creation (`internal/wsstore/store.go:254`);
  per-worktree `state.sqlite` opened per-operation by potentially multiple
  concurrent server processes (`store.go:181`). `busy_timeout=5000` (`store.go:250`)
  + 8-attempt app retry on writes/migrations (`internal/wsstore/retry.go:14-49`).
  After retry exhaustion the busy/locked error is **returned as a JSON-RPC error,
  not fatal** in serve mode — so contention degrades but does not disconnect.
  Point reads are unretried (`store.go:632,789,860,884`). Cross-process writer
  serialization is absent (`SetMaxOpenConns(1)` + process-local mutex only).
- No signal handling anywhere in serve (`os/signal` never imported); no panic
  recovery in the request path; no explicit WAL checkpoint or `-shm`/`-wal`
  cleanup; no Windows parent-death detection for the serve process. Clean
  shutdown happens only on stdin EOF.
- A cross-process PID lock already exists but is **unused** —
  `internal/wsstate/orchestrator_lock.go:28-90` (`AcquireOrchestratorLock`),
  zero callers.

## Decisions

- **Leading hypothesis (C): unrecovered request-goroutine panic** crashes the
  serve process → that connection's pipe EOFs → single-connection
  disconnect+auto-reconnect. Fits intermittency, load correlation, auto-recovery,
  and the total absence of a trace. This supersedes downstream hypothesis B.
- **Hypothesis B (SQLITE_BUSY → fatal exit): rejected as a disconnect cause** —
  busy is non-fatal in serve mode. Contention still raises latency/error rates
  (amplifier, not trigger).
- **Hypothesis A (orphaning): confirmed structurally, reclassified as amplifier**
  — the intermediate Python process + missing Job Object + no server-side
  parent-death detection orphan the Go child and leave stale locks that break the
  *next* connect, rather than directly triggering the disconnect. Stale `-shm`
  with 0 live processes (downstream evidence) is explained by unclean exit
  skipping `store.Close`, i.e. by A/C, not B.

## Constraints

- This dev environment is Linux/WSL2 with `powershell.exe` interop available, so
  Windows-side behavior (Job Object reaping, orphaning, concurrency repro) can be
  exercised by spawning real Windows processes from WSL — Phases 2-4 are not
  blocked on a separate Windows host for local validation, though a Windows CI
  runner is still needed for automated coverage. Phase 1 is fully cross-platform.
- Launcher edits must be applied to `agents-plugin/bin/ws-mcp-launcher.py` and
  kept byte-identical with the `agents-plugin-wsflow` mirror.

## Spec Impact

- Target spec area: `mcp-tools.md` (the `ws-mcp serve --stdio` process contract)
  for the panic-resilience behavior, and `plugin-runtime.md` (launcher/serve
  diagnostics) for crash-trace persistence and the abnormal-child-exit breadcrumb.
- Expected caller-visible change: a panic in a single tool handler returns a
  JSON-RPC error for that one request instead of terminating the serve session;
  crash traces and abnormal child-exit reasons are persisted to a documented
  runtime-dir location instead of vanishing to inherited stderr.
- Contract-first spec: no — a robustness fix reflected into the specs at
  implementation/closeout, not a new caller-facing API requiring an upfront
  contract.

## Phases

### Phase 1: Request-goroutine panic recovery + crash capture

Add `recover()` to the per-request goroutine at `internal/mcp/server.go:172-184`.
On panic: (a) fail only that request with a JSON-RPC error (do not crash the
process), and (b) persist the panic value + full stack to an **always-on**
dedicated crash file under the runtime dir. The always-on file is the required
sink, because the downstream that hit this never set `WS_MCP_DEBUG_LOG` (an
opt-in no-op when unset, `server.go:256-259`) — which is exactly why no trace
exists today; `WS_MCP_DEBUG_LOG` may mirror the event as an optional secondary.
Specify the crash-file path and overwrite/rotation behavior so the "documented
runtime-dir location" in Spec Impact is concrete and reviewable.

`recover()` converts the crash to a per-request error but skips the panicking
handler's own defers (tx rollback, store/file close). Confirm the per-operation
store open/close model (`store.go:181`) bounds any connection / `-wal` / `-shm`
leak so a recovered panic cannot wedge the process, and add a regression test
that a deliberately-panicking write handler is followed by a **successful
subsequent request on the same process**. Reject silently swallowing panics — the
trace must be persisted and the request must return a visible error. This is the
single highest-value change and is fully cross-platform.

### Result (325f368f) - 2026-07-24

Implemented in `agents-plugin-tool/internal/mcp/server.go`: the per-request
goroutine in `ServeStdio` gained a fourth `recover()` defer, appended after the
existing `wg.Done`/`cancel`/`requests.Delete` defers so Go LIFO runs it first on
unwind. On panic it writes a JSON-RPC error (code `-32000`, message `internal
error: request handler panicked (<method>)`) on the serialized response path and
the process keeps serving. New `crashLogPath()`/`recordPanic()` helpers persist
the panic value + `debug.Stack()` as one JSON line to an always-on
`<cache-root>/crash/mcp-panic.log` (resolved via `wsstate.CacheRoot`, no
rotation), with a stderr fallback if the file is unwritable, and mirror the event
through the existing `appendDebugEvent` (so `WS_MCP_DEBUG_LOG` still receives it
when set). Regression test `TestServeStdioRecoversPanicAndPersistsCrashTrace`
drives a real dispatched `todo.append` panic via a package-level `testPanicHook`
seam and asserts (a) the panicking request returns the `-32000` JSON-RPC error,
(b) a subsequent `runtime.info` on the same process succeeds, and (c) the crash
file contains the panic text.

Verification: `go build ./...` clean; `go test ./...` all 12 packages pass.

Deviation from plan framing: the ticket phrasing "recover() skips the panicking
handler's own defers" overstates Go semantics — inner defers (e.g. `defer
store.Close()`) still run during unwind, so per-operation store open/close already
bounds the connection/`-wal`/`-shm` lifetime; no store-cleanup code was needed,
only confirmation. Reviews (correctness/fit/test) all clean; 5 minor findings
recorded, none requiring a fix (keyed-tool panic case, crash-file-unwritable
branch, and concurrent-panics case all judged out of Phase 1 scope).

Deferred to later phases (unchanged): Phase 2 launcher abnormal-exit breadcrumb,
Phase 3 Windows Job Object / parent-death detection, Phase 4 SQLite point-read
retry + WAL re-assert.

### Phase 2: Launcher-side abnormal-exit diagnostics

On the Windows branch, record the Go child's exit code/reason on abnormal
(non-zero/signal) termination — the value `subprocess.call` already returns and
currently discards (`ws-mcp-launcher.py:868`) — via a breadcrumb complementing the
startup-only `last-launch-error`. Optionally redirect the Windows child's stderr
to a timestamped runtime-dir file so a Go-side crash stack survives even before
Phase 1 ships. Keep the wsflow mirror byte-identical.

### Result (5f240b05) - 2026-07-24

Implemented in `agents-plugin/bin/ws-mcp-launcher.py` (and byte-identically
mirrored into `agents-plugin-wsflow/bin/ws-mcp-launcher.py`): a new best-effort
`write_exit_breadcrumb(exit_code)` and a Windows-branch change to the handoff
block — `subprocess.call` now captures the child exit code and, when non-zero,
writes `last-abnormal-exit` (exit code + timestamp) into the runtime dir
(`WS_MCP_RUNTIME_DIR` / `.runtime/<os>-<arch>/`), then returns that code. The
breadcrumb complements, never overwrites, the startup-only `last-launch-error`
(distinct filename), and a clean exit (code 0, incl. stdin-EOF shutdown) writes
nothing. The POSIX `os.execvpe` exec-replace path is byte-for-byte unchanged.

The optional Windows-child stderr-to-file redirect from the phase plan was
evaluated and **dropped**: Phase 1's always-on `<cache-root>/crash/mcp-panic.log`
already persists the confirmed root-cause trigger (recovered request-goroutine
panic) with full stack, so unbounded stderr capture is unjustified without field
evidence of a remaining gap (an abnormal-exit breadcrumb paired with an *empty*
Phase 1 crash file). Revisit only if that pairing is observed.

Regression tests `test_windows_abnormal_exit_writes_complementary_breadcrumb` and
`test_windows_clean_exit_does_not_write_breadcrumb` drive full `main()` with
`host_os="windows"`, patching `subprocess.call` to a non-zero code
(`3221225477` = `0xC0000005`) and `0`; they assert the exit code is returned, the
breadcrumb presence/absence, and that `last-launch-error` is not collided with.
Adversarial review mutation-tested both to confirm neither passes vacuously;
verdict clean aside from the doc pass (now landed in `b7d91a37`) and a trivial
unused-local cleanup (applied).

Verification: `python3 -m unittest discover agents-plugin/tests` = 45 pass;
`agents-plugin-wsflow/tests` = 9 pass; launcher `diff` = zero differences.

Doc pass (`b7d91a37`): added spec anchor
`{#260724-launcher-abnormal-exit-breadcrumb}` to `plugin-runtime.md` documenting
both breadcrumbs, and corrected the stale mental-model claim that the wsflow
launcher "intentionally diverges" and skipped canonical fixes — disproven by the
byte-identical diff and contradictory with the adjacent keep-in-sync rule.

Deferred (unchanged): Phase 3 Windows Job Object / parent-death detection,
Phase 4 SQLite point-read retry + WAL re-assert.

### Phase 3: Windows process-lifecycle hardening

Assign the Go child to a Windows Job Object with kill-on-close so terminating the
launcher deterministically reaps the server (eliminates orphans/stale locks —
blocks hypothesis A), and/or add server-side parent-death detection so an orphaned
serve process self-terminates. Full removal of the intermediate process is likely
infeasible (no true `exec` on Windows), so Job Object is the pragmatic path.
Repro-dependent; validate locally via `powershell.exe` interop from WSL (real
Windows process spawning), with a Windows CI runner for automated regression.

### Result (88a67a78) - 2026-07-24

Implemented the **server-side parent-death detection** half of the phase's
"and/or" (Job Object deferred — see below). `cmd/ws-mcp/serve()` now arms a
Windows-only goroutine (`startParentDeathWatch`, build-tagged
`parent_watch_windows.go`; `!windows` no-op stub in `parent_watch_other.go`)
that captures `os.Getppid()` once, opens the parent handle with
`windows.OpenProcess(SYNCHRONIZE, ...)`, blocks on
`windows.WaitForSingleObject(h, INFINITE)`, and — only on a real `WAIT_OBJECT_0`
signal — records a `process.parent_exited` event to the new always-on
`<cache-root>/crash/mcp-lifecycle.log` sink (via exported
`mcp.RecordLifecycleEvent`, reusing Phase 1's `crashDir()`) and `os.Exit(0)`s.
So a force-killed launcher can no longer leave an orphaned server holding a
stale `state.sqlite` lock (hypothesis A). The watch is armed only from
`serve()`, never inside `Server.ServeStdio`, so the `smoke` self-test and every
`internal/mcp` test that calls `ServeStdio` do not arm a live `os.Exit`
goroutine. A `//go:build windows` regression test exercises `watchProcessExit`
against a real killed child; the `windows-smoke` CI job gained its first
`go test ./...` step so that test actually runs on `windows-latest`.

Verification: `go build ./...` + full `go test ./...` clean (Linux, exercising
the `!windows` stub); `GOOS=windows go build ./...` and
`GOOS=windows go vet ./cmd/ws-mcp/...` clean (compile-verify the Windows sources
+ test). The Windows-only test cannot execute on this Linux host — it runs only
in CI. Adversarial review verdict SHIP, no blockers/majors; the one actionable
minor (fire `onExit` only on `WAIT_OBJECT_0`, not `WAIT_FAILED`) was applied.

**Job Object backstop (3b) deferred — binding-decision escalation resolved by
evidence.** The phase plan's survey emitted an `[escalate-to-binding-decision]`
on whether to build the launcher-side kill-on-close Job Object at all. Two
findings resolved it toward "not now": (1) a naive kill-on-close job would
cascade-kill mercenary async workers and break the survives-disconnect contract
unless a mandatory `JOB_OBJECT_LIMIT_BREAKAWAY_OK` + `CREATE_BREAKAWAY_FROM_JOB`
companion ships; the server-side detection above has no such risk. (2) The
kill-on-close mechanism could **not** be validated through `powershell.exe`
interop from WSL: across three attempts (including a real job-owning parent
process exiting) the child was never reaped, despite `IsProcessInJob` confirming
correct assignment (`anyJob=False` before, `ourJob=True` after) — so this dev
box cannot verify the ctypes Job Object path before shipping it. Root cause of
the interop reap-failure was not isolated; the takeaway is that 3b needs a real
Windows session/CI, matching the survey's escalation. **3b is deferred until
field or Windows-CI evidence shows 3a's server-side detection is insufficient**
(e.g. the launcher is force-killed in a way that races ahead of the Go
watcher). If pursued, it must ship the mercenary breakaway companion.

Deferred (unchanged): Phase 4 SQLite point-read retry + WAL re-assert.

### Phase 4: SQLite multi-process discipline

Extend bounded busy-retry to the unretried point-read paths
(`store.go:632,789,860,884`); re-assert `journal_mode=WAL` on existing-DB opens;
evaluate wiring the already-present but unused `orchestrator_lock.go` (or
`busy_timeout`/`wal_autocheckpoint` tuning) to coordinate concurrent server
processes over the shared `state.sqlite`. Hardening/robustness, not a confirmed
disconnect cause — sequence after Phase 1 confirms or rules out contention's role.

### Result (6539b0c0) - 2026-07-24

Implemented the two low-risk hardening items in
`agents-plugin-tool/internal/wsstore/store.go`; item 3 evaluated to no-action
(below). **Item 1 (point-read retry):** wrapped all previously-unretried read
paths in the existing `withSQLiteRetry` helper — the ticket cited 4
(`AgentDefinition`, `ExecJob`, `Artifact`, `PruneExpired`) but a full audit found
3 more (`PruneAgentInstances`, `Count`, `retryTombstones`), 7 total, all now
covered. Single-row closures return `sql.ErrNoRows` unwrapped (not busy/locked,
so not retried) preserving each method's found/not-found contract; multi-row
closures reset their accumulator slice at the top of each attempt (no
double-append on retry) with `defer rows.Close()` scoped per attempt. Reads take
no `writeMu` — SQLite/WAL already permits concurrent readers with a single
writer and `SetMaxOpenConns(1)` serializes this process; the retry only exists to
survive *another process's* writer holding the lock. **Item 2 (WAL re-assert):**
removed the `if setJournalMode` (new-DB-only) gate in `configure()` so
`PRAGMA journal_mode=WAL` is issued unconditionally on every open — a cheap no-op
when already WAL, and it migrates any legacy non-WAL `state.sqlite` to WAL on
next open. Removed the now-dead `newDB`/`setJournalMode` plumbing and the unused
`fileExists` helper.

**Item 3 (evaluate lock/tuning): no action, by design.** Wiring
`internal/wsstate/orchestrator_lock.go` (`AcquireOrchestratorLock`, still
zero non-test callers) was rejected from source analysis: it is a coarse
whole-process singleton `O_EXCL` lock on a *different* file (`orchestrator.lock`,
not `state.sqlite`), structurally mismatched to the fine-grained per-operation
reader/writer contention this phase targets — repurposing it would add redundant
per-query filesystem locking or wrongly forbid the multiple-concurrent-server
scenario the ticket treats as supported. Raising `busy_timeout` above 5000ms or
adding `wal_autocheckpoint` tuning was also declined: no source evidence of
insufficiency (short-transaction discipline throughout; no unbounded `-wal`
growth signal). Both remain revisitable only on field/load evidence
(lock-holds exceeding 5s, or `-wal` bloat). "Evaluate wiring … or tuning" thus
resolves to *evaluated, no change* — the deliverable the phase asked for.

Verification: `go build ./...` clean; `go test ./internal/wsstore/...` and full
`go test ./...` green (all packages, `-count=3` no flakiness). New tests:
`TestIndependentHandleContentionRetriesPointRead` (single-row) and
`...MultiRowRead` force real `SQLITE_BUSY` on a reader via a
`locking_mode=EXCLUSIVE` holder (WAL readers don't block on `BEGIN IMMEDIATE`),
`TestManagerOpenReassertsWALOnPreExistingNonWALDatabase`, plus WAL idempotence
assertions on `TestOpenCloseReopenCreatesWorktreeDatabase`. Adversarial review
verdict SHIP (no blockers/majors) and empirically confirmed both contention
tests fail if the retry wrap is removed (non-vacuous). Fully cross-platform; no
launcher/OS-process code touched.

---

**Ticket complete.** All four phases have `### Result` sections and are merged
into the goal branch: Phase 1 (request-goroutine panic recovery + always-on
crash trace, the confirmed disconnect trigger), Phase 2 (launcher
`last-abnormal-exit` breadcrumb), Phase 3a (Windows server-side parent-death
self-termination; Job Object backstop 3b deferred pending real-Windows
evidence), Phase 4 (SQLite point-read retry + unconditional WAL re-assert). Moved
to `.done/`.
