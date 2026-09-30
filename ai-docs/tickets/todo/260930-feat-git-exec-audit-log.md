---
title: Git execution audit log to attribute stale index.lock files
---

# Git execution audit log to attribute stale index.lock files

## Background

A downstream project (Windows, Claude Code / Codex hosts using ws-mcp) reports
that `.git/index.lock` is left behind very often. The lock is almost always
stale: no git process is alive, and the lock is hours old. We cannot tell
whether ws tools cause it, and nothing on disk today can answer that
afterwards.

Static analysis shows one plausible ws path:

- Every mutating ws git call goes through `wsgit.Runner.RunGit`
  (`agents-plugin-tool/internal/wsgit/git.go:26`). That covers `git.commit`
  (`add` / `commit`), `git.merge` (`switch` / `merge`),
  `worktree.acquire` / `release` (`reset --hard` / `switch` / `checkout`), and
  `tickets.move` (`add` / `mv`). The `wsindex` `ExecRunner`
  (`internal/wsindex/git.go`) is the second runner.
- On `notifications/cancelled` (`internal/mcp/server.go:251`), the request ctx
  is cancelled.
- `exec.CommandContext` then calls its default `Process.Kill`: SIGKILL on
  Unix, `TerminateProcess` on Windows. `wsgit` sets no custom `Cancel`.
- A git killed that way cannot remove the lock it holds, so the lock stays
  stale for good.

A second, non-ws candidate is the host's own shell tool. If it kills a process
tree that includes a git holding the lock, the result is the same.

The existing debug events (`appendDebugEvent`, `server.go:306`) cannot tell
these apart:

- They live in memory unless `WS_MCP_DEBUG_LOG` is set.
- They do not record the tool name.

The goal of this ticket is attribution: when a stale lock appears, compare
its mtime with an always-on record of ws git executions.

## Decisions

### Logging

- **Always-on git execution log at both runners.** Every git process launched
  through `wsgit.ExecRunner` and the `wsindex` `ExecRunner` gets:
  - a start record, written right after the process starts (so its pid is
    known), and
  - an end record, written after it exits.

  A start with no matching end means the git process or the server died
  mid-run. The log has no env gate, because attribution after the fact
  needs it to already be on.
  Rejected: extending the env-gated `WS_MCP_DEBUG_LOG` debug events. They are
  off by default and lack the tool name.
- **Cancellation receipts are logged too.** Every `notifications/cancelled` is
  recorded with the request id and the tool name of the request it targets.
- **Scope of logged git calls.** The five direct `exec.Command("git", ...)`
  sites are not logged:
  - `wsstate/paths.go`
  - `wsdoc/project_tree.go`
  - `wsdoc/tickets_sage_freshness.go`
  - `wsdoc/tickets_scope.go`
  - `mcp/server.go:3236`

  They are read-only and run without a request ctx, so ws cancellation can
  never kill them while they hold a lock.
- **Carrying tool identity.** The MCP tool name and request id reach the
  runner through a context value set at `tools/call` dispatch.
  - `Runner.RunGit(ctx, root, args...)` stays unchanged.
  - No `ExecRunner{}` construction site (31 of them) has to change.
- **Record shape.** One JSON object per line. `exec_id` pairs a start with its
  end.

  ```text
  {"ts","event":"git.start","exec_id","server_pid","git_pid","root","args","tool","request_id"}
  {"ts","event":"git.end","exec_id","outcome":"ok|failed|killed_by_cancel","exit_code","duration_ms"}
  {"ts","event":"request.cancelled","server_pid","request_id","tool"}
  ```

  - `killed_by_cancel` means the request ctx was cancelled while git ran.
- **Only `root` is logged, not the git-dir.** `root` is the `-C` directory.
  - The lookup maps a lock path to its worktree root at lookup time:
    - `.git/index.lock` maps to the main worktree.
    - `.git/worktrees/<n>/index.lock` maps to the worktree path recorded in
      `<n>/gitdir`.
  - Rejected: resolving the git-dir on every call. That needs an extra
    `git rev-parse` process per git call, which is costly on Windows.
- **Commit messages are redacted.** The value after `-m` is replaced by a
  length marker; every other arg is logged verbatim.
  - Downstream may attach these logs to upstream bug reports, and the message
    plays no part in lock attribution.
  - Rejected: logging args verbatim.

### Storage and retention

- **Location.** Log files live under
  `<dir of wsconfig.GlobalPath>/logs/git-exec/`. The default is
  `~/.ws/logs/git-exec/`, and `WS_CONFIG_HOME` is honored.
  - This follows `wsnote.MachinePath`, which puts the machine-layer store next
    to `GlobalPath`.
- **One file per day per server process:**
  `git-exec-YYYYMMDD-<server-pid>.jsonl`.
  - Several ws-mcp processes on one machine append at the same time. Separate
    files mean no cross-process lock is needed on each git call.
- **Retention is about one month.** At server start, files older than 30 days
  are deleted, so the log cycles instead of growing without bound.
- **Appends within one process are serialized.** The server runs each request
  in its own goroutine (`ServeStdio`, `server.go:219`), so several runner
  calls can append to the same file at once.
  - Every append goes through one in-process writer that holds a mutex across
    choosing the day file, opening or rolling it over, and writing one
    complete line.
  - Records must never interleave, tear, or get lost.
  - This is the same lesson as `260929-bug-config-tune-agents-tier-lost-update`:
    concurrent requests in one process race on a shared file.
- **Log writes are best-effort.** A failure to write or prune never fails or
  retries the git call. The only waiting a log adds is the serialized append
  itself.

### Lookup and discoverability

- **Manual lookup is a CLI subcommand, not an MCP tool.** The command is
  `ws-mcp git lock-audit <lock-path>`, which downstream runs as
  `ws-cli git lock-audit ...`.
  - It fits the existing domain-grouped CLI
    (`ws-mcp git <status|diff|log|merge-base|commit>`,
    `cmd/ws-mcp/main.go:355`).
  - It reads the lock file's mtime and reports one of:
    - matching ws git executions for that worktree whose run window covers
      the mtime, with their tool, request id, and outcome (an unmatched start
      is reported as such);
    - otherwise, that no ws git execution matches that time.
  - Rejected: a new MCP tool. The user judged that adding MCP surface for a
    debugging aid is inappropriate.
- **Automatic attribution on lock errors.** When a ws git call fails because
  `index.lock` exists, the error response includes the lock-audit verdict for
  that lock, from the same matcher the CLI uses.
  - This changes existing error text; it adds no tool.
  - Rejected: CLI-only lookup. The on-site agent would then need to know the
    CLI exists.
- **The help text is the manual.**
  - `ws-mcp git lock-audit --help` prints the full procedure: where the log
    lives, how the lock mtime is used, and how to read each verdict.
  - Top-level `--help`, `-h`, and `help` print extended usage that names the
    lookup command and the log directory.
  - Rejected: a separate shipped document. The binary is the only surface
    guaranteed to be present downstream, and help text cannot drift from it.
    `ai-docs/manuals/` does not ship.

## Constraints

- **Out of scope: changing cancellation behavior.** That includes letting
  lock-holding git commands finish instead of being killed. Changing the kill
  path while measuring it would confound the attribution; revisit once the log
  names the cause.
- **Out of scope: `GIT_OPTIONAL_LOCKS=0`** for the read-only status and diff
  calls. It is deferred for the same reason.
- **Out of scope: pi harness git paths.**
- **Shipped-surface rules apply.** Help text and error text ship downstream.
  Follow `ai-docs/manuals/shipped-surface-boundary.md`: nothing may depend on
  this repository alone.
- **MCP runbook.** Edits under `agents-plugin-tool/internal/mcp/` follow
  `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: Log git executions and add lock-audit lookup

Implement every decision above:

- the serialized start/end logging at both runners;
- cancellation receipt records;
- redaction;
- day-file rollover and pruning at server start;
- the `git lock-audit` CLI subcommand and its shared matcher;
- automatic attribution on lock errors;
- the extended help output.

Verification expectations:

- A normal git call writes a start and an end record with an `ok` outcome.
- A failed call is recorded as `failed`.
- A git call whose request ctx is cancelled mid-run is recorded as
  `killed_by_cancel`.
- A `notifications/cancelled` receipt is recorded with the target tool name.
- The `-m` value is redacted in logged args.
- A concurrency test runs many goroutines appending records at once. Every
  line must parse as JSON and no record may be lost. This mirrors the
  `TestConcurrent*NoLostWrites` tests in `internal/wsconfig/scope_test.go`.
- Files older than 30 days are pruned at server start, and newer files are
  kept.
- Lookup tests use a synthetic log plus a lock file with a controlled mtime.
  They cover four cases:
  - an overlapping killed execution is reported;
  - an unmatched start is reported;
  - a `.git/worktrees/<n>/index.lock` path maps to its worktree;
  - no overlapping execution yields the "no ws git execution" answer.
- A ws git call that fails on an existing `index.lock` returns the verdict in
  its error response.
- `ws-mcp --help` names the lookup command and the log directory.
  `ws-mcp git lock-audit --help` prints the procedure.
