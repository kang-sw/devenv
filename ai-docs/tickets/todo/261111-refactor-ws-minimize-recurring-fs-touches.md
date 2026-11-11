---
title: "Minimize recurring filesystem touches in the dashboard daemon, ws-mcp, and claude-mod"
related:
  261106-feat-ws-dashboard-live-snapshot-pi: revises its on-disk-only live path for the daemon-hosted MCP
---

# Minimize recurring filesystem touches in the dashboard daemon, ws-mcp, and claude-mod

## Background

The user's machine (Windows + WSL) runs DLP software that scans every file
open, so each recurring open, stat, read, or write costs CPU. With a Claude
session streaming and one dashboard tab open, the ws runtime touches the
filesystem thousands of times a minute and makes the whole PC slow. The user
wants the runtime to be event-driven: what changes inside one process is
signalled in memory, and any poll that remains runs no more often than every
10 seconds. The daemon keeps its statelessness for recovery: state still
reaches disk, only less often, and pointless disk round-trips go away.

An inventory (2026-11-11, one lead session, one dashboard tab) found these
offenders, with rough file operations per minute:

1. **Intake chain, ~3,000-3,600/min while streaming text, up to ~10x with
   tool-heavy turns.** The MCP server runs inside the dashboard daemon: the
   launcher proxies stdio to the daemon's `/mcp`, and `cmd/ws-mcp/proxy.go`
   (about lines 78-86 and 177) builds and serves one `mcp.Server` per
   connection. Yet each `dashboard.intake` call (claude-mod sends up to 10/s,
   `dashboard.ts` FLUSH_INTERVAL_MS / TEXT_FLUSH_INTERVAL_MS):
   - reads `key.json` three times (`server.go` ~692,
     `live_snapshot.go` ~221 and ~298, `session_auth.go` lookup ~219);
   - schedules a snapshot write (100 ms / 250 ms / 1 s debounce,
     `live_snapshot.go` liveSeams), which re-reads `key.json` for the active
     check (~240) and writes through a temp file and rename;
   - sends an HTTP `POST /nudge` to its own daemon, re-reading
     `endpoint.json` per batch (`dashboard_sessions.go` runDashboardNudges);
   - makes the daemon re-read the registration, snapshot, ring, inbox,
     tickets, and history from disk (`index.go` Nudge,
     `dashboard_sessions.go` ReadSession ~585-623, `pages.go` ~681-694).
     `FileCache` refuses files modified less than 2 s ago (`file_cache.go`
     ~48, ~76), so every read while streaming is a miss.
2. **Status band git.** `band.tsx` `toolCalled` (~940-955) runs
   `readGit` (`git status --porcelain -b` and `git diff --numstat`, a full
   worktree stat each) on every `tool.call`, including the mod's own MCP
   calls such as intake (`register.ts` ~1311 passes `byModel` but `readGit`
   ignores it), throttled to once per 2 s (`band.tsx` ~30).
3. **`mailbox wait` polls every 500 ms** (`wsmailbox/wait.go`
   DefaultWaitPoll), idle or not, reading the reply registry, the named
   store, and the inbox directory twice per check. It runs inside the daemon
   in CLI mode (`cmd/ws-mcp/proxy.go` ~89).
4. **Tracker watch every 2 s** (`wstrackerhost/watch.go` ~16, started at
   `cmd/ws-mcp/dashboard.go` ~126): registry read, globs, journal stats,
   `packed-refs`, `tables.list`, and two ref-tree walks per store per tick.
5. **Live snapshot heartbeat every 30 s** (`wslive/snapshot.go` ~90,
   re-armed in `live_snapshot.go` ~446): a snapshot write, a nudge, a full
   `ReadSession`, and about four git processes including a full `git status`
   (`live_snapshot.go` ~453-490), forever while the session is active.

An in-process route already exists for agent sessions: `Daemon.agentChanged`
calls `index.Reread` and `broker.wake` directly (`daemon.go` ~510-516).
The direct (non-daemon) path, where ws-mcp is a separate process
(`ws-mcp-launcher.py` direct fallback), and daemon restart recovery
(`stream_cache.go` ~22-24) still need the files.

## Decisions

- **(140) The daemon-hosted intake renders from memory and flushes the
  snapshot for recovery only.** When the MCP server runs inside the daemon,
  `dashboard.intake` applies events to the session's in-memory live state as
  today and hands the change to the daemon in process (the agentChanged-style
  wake: a hook passed to the server, for example through `ServerOptions`).
  The daemon renders the live snapshot from that in-memory state instead of
  re-reading the snapshot file, with no HTTP self-nudge and no
  `endpoint.json` read. The snapshot file is still written, but only at the
  heartbeat interval, at turn end, and at session end or daemon shutdown, so
  a restarted daemon starts from it; an adapter also resyncs its full state
  after a failed intake call (`dashboard.ts` header), which covers the gap
  between flushes. Conversation ring appends stay as today (one write per
  finished entry). The direct path keeps the per-change file write plus
  nudge.
  Rejected: keeping the per-change file write and removing only the
  in-process HTTP nudge and re-read (half the cost stays, and a file write
  per change is still scanned). Rejected: a new adapter-to-daemon REST
  stream; the adapters' MCP connection already reaches the daemon, and the
  live-snapshot ticket rejected adapter-side discovery and token handling.
  This revises the on-disk-only rule (169) of
  `261106-feat-ws-dashboard-live-snapshot-pi` for the daemon-hosted path
  only; that rule was set when ws-mcp was always a separate process.
- **(141) Event wake first, and no recurring poll faster than 10 s.**
  - `mailbox wait` inside the daemon wakes on an in-process signal from
    mailbox and inbox writes, with a 10 s backstop recheck in place of the
    500 ms poll.
  - Tracker watch goes from 2 s to 10 s.
  - The status band's git refresh on model tool calls is throttled to at
    most once per 10 s, plus the existing refresh at turn end.
  Rejected: keeping a short mailbox poll for mail written outside the
  daemon; (145) covers that case with a wake instead.
- **(142) Intake caches the session record in memory.** `root`, `scope`,
  and `parent` are written only when the key is minted (no writer changes
  them later), so intake and the live writer's active check keep them per
  key in memory and re-check that the key still exists at most every 10 s.
  The activity touch keeps its existing `touchGuardWindow`.
  Rejected: an mtime-validated cache, which still stats the file on every
  call, and the DLP scans stats too.
- **(143) The status band runs no git for the mod's own MCP calls.**
  `toolCalled` refreshes git only when `byModel` is true. The mod's own
  calls (intake, drains, lookups) never change the working tree.
- **(144) The heartbeat's repo refresh runs only when a tool call ended
  since the last refresh.** An idle session's heartbeat runs no git. The
  working tree changes through tool calls; a person's own edits show at the
  next tool call or turn.

- **(145) Mail written outside the daemon wakes waiters through the
  nudge.** In-process mailbox and inbox writes wake waiting `mailbox wait`
  calls directly. A direct-path (out-of-daemon) mailbox send posts the
  existing fire-and-forget `/nudge` after its write, and the daemon's
  `/nudge` handler also wakes that session's mailbox waiters; a lost nudge
  falls to the 10 s backstop. Today no mailbox send path calls
  `dashboardNudge` (only `live_snapshot.go`, `dashboard_inbox.go`, and
  `server.go` do), so this send-side nudge is new.
  Rejected: accepting up to 10 s latency for out-of-daemon mail, which
  would visibly delay mail on that path.

## Constraints

- Statelessness for recovery stays: every state the daemon renders from
  memory still reaches disk at the flush points in (140), and a daemon
  restart recovers from disk plus the adapters' resync.
- The direct (non-daemon) path keeps today's behavior apart from (145).
- Out of scope: the 30 s backstop poll, the 15 s SSE refresh tick, the
  presence and registration heartbeats, holder and git-badge rescans
  (all already at or above 10 s); the steered-message drain on each model
  tool call; the launcher's per-re-arm `python3` process for `mailbox wait`.
- Wire formats stay: the intake tool schema, the snapshot and ring file
  schemas, and the nudge body do not change.
- claude-mod edits mirror into `agents-plugin-wsflow/hooks/claude-mod/` per
  `ai-docs/manuals/wsflow-mirroring.md`.
- Manuals: `ai-docs/manuals/ws-mcp.md` (internal/mcp),
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`.
- Never run heavy go build, test, or vet locally; use
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 20 -- test ./...`.

## Phases

### Phase 1: Event-driven live path and 10 s poll floor

Implement (140) through (145) as decided above.

Verification:

- Go tests (remote through ws-rgo) showing, on the daemon-hosted path:
  a burst of intake calls renders through the in-process wake with no
  snapshot-file write per call, no HTTP self-nudge, and no `key.json` read
  per call; the snapshot is flushed at heartbeat, turn end, and shutdown;
  a restarted daemon renders the last flushed snapshot.
- The direct path still writes the snapshot per change and nudges.
- `mailbox wait` wakes on an in-process write and on a `/nudge` without a
  poll tick (injected clock), and its backstop recheck is 10 s.
- Tracker watch interval is 10 s; the heartbeat runs no git when no tool
  call ended since the last refresh.
- claude-mod tests: no git refresh for a non-model call; model-call refresh
  at most once per 10 s plus turn end. Run the plugin suites locally with
  `nice -n 19 claude plugin test agents-plugin` and the wsflow package.
- Existing suites stay green: `go test ./...` remotely, plugin tests for ws
  and wsflow.
