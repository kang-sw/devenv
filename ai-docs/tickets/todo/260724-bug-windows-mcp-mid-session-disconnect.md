---
title: "Windows wsflow MCP mid-session disconnects under high concurrency"
sage-review-design: required
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

- This dev environment is Linux/WSL2; Windows-specific confirmation and testing
  (Job Object, orphaning, repro under concurrency) require a Windows host and
  cannot be fully validated here. Phase 1 is cross-platform and lands/verifies
  independently of a Windows host.
- Launcher edits must be applied to `agents-plugin/bin/ws-mcp-launcher.py` and
  kept byte-identical with the `agents-plugin-wsflow` mirror.

## Phases

### Phase 1: Request-goroutine panic recovery + crash capture

Add `recover()` to the per-request goroutine at `internal/mcp/server.go:172-184`.
On panic: (a) fail only that request with a JSON-RPC error (do not crash the
process), and (b) persist the panic value + stack to disk — reuse the
`WS_MCP_DEBUG_LOG` sink and/or a dedicated crash file under the runtime dir.
This is the single highest-value change: it both prevents session-wide death from
one bad handler and captures the smoking gun the investigation lacked. Cross-
platform; verifiable with a deliberately-panicking test handler. Reject silently
swallowing panics — the trace must be persisted and the request must return a
visible error.

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
Repro-dependent; requires a Windows host to validate.

### Phase 4: SQLite multi-process discipline

Extend bounded busy-retry to the unretried point-read paths
(`store.go:632,789,860,884`); re-assert `journal_mode=WAL` on existing-DB opens;
evaluate wiring the already-present but unused `orchestrator_lock.go` (or
`busy_timeout`/`wal_autocheckpoint` tuning) to coordinate concurrent server
processes over the shared `state.sqlite`. Hardening/robustness, not a confirmed
disconnect cause — sequence after Phase 1 confirms or rules out contention's role.
