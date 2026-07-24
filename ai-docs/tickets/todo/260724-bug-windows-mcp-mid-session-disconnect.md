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

### Phase 2: Launcher-side abnormal-exit diagnostics

On the Windows branch, record the Go child's exit code/reason on abnormal
(non-zero/signal) termination — the value `subprocess.call` already returns and
currently discards (`ws-mcp-launcher.py:868`) — via a breadcrumb complementing the
startup-only `last-launch-error`. Optionally redirect the Windows child's stderr
to a timestamped runtime-dir file so a Go-side crash stack survives even before
Phase 1 ships. Keep the wsflow mirror byte-identical.

### Phase 3: Windows process-lifecycle hardening

Assign the Go child to a Windows Job Object with kill-on-close so terminating the
launcher deterministically reaps the server (eliminates orphans/stale locks —
blocks hypothesis A), and/or add server-side parent-death detection so an orphaned
serve process self-terminates. Full removal of the intermediate process is likely
infeasible (no true `exec` on Windows), so Job Object is the pragmatic path.
Repro-dependent; validate locally via `powershell.exe` interop from WSL (real
Windows process spawning), with a Windows CI runner for automated regression.

### Phase 4: SQLite multi-process discipline

Extend bounded busy-retry to the unretried point-read paths
(`store.go:632,789,860,884`); re-assert `journal_mode=WAL` on existing-DB opens;
evaluate wiring the already-present but unused `orchestrator_lock.go` (or
`busy_timeout`/`wal_autocheckpoint` tuning) to coordinate concurrent server
processes over the shared `state.sqlite`. Hardening/robustness, not a confirmed
disconnect cause — sequence after Phase 1 confirms or rules out contention's role.
