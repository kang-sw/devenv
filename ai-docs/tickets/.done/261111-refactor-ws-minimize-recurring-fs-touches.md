---
title: "Minimize recurring filesystem touches in the dashboard daemon, ws-mcp, and claude-mod"
related:
  261106-feat-ws-dashboard-live-snapshot-pi: revises its on-disk-only live path for the daemon-hosted MCP
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3d7c63f65794e38c
sage-review-completeness-reviewed: 3d7c63f65794e38c
completed: 2026-11-11
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
   store, and the inbox directory on every check. It runs inside the daemon
   in CLI mode (`cmd/ws-mcp/proxy.go` ~89).
4. **Tracker watch every 2 s** (`wstrackerhost/watch.go` ~16, started at
   `cmd/ws-mcp/dashboard.go` ~126): registry read, globs, journal stats,
   `packed-refs`, `tables.list`, and two ref-tree walks per store per tick.
5. **Live snapshot heartbeat every 30 s** (`wslive/snapshot.go` ~90,
   re-armed in `live_snapshot.go` ~446): a snapshot write, a nudge, a full
   `ReadSession`, and up to five git processes including a full `git status`
   (`live_snapshot.go` ~453-490; `wsgit.BranchSync` runs up to four and
   `WorkingTreeCounts` one, `internal/wsgit/git.go#L145-L197`), forever while
   the session is active.

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
  - Wake cadence: the existing debounce (100 ms / 250 ms / 1 s by change
    kind, including the 1 s agent debounce of `b0e680e23`) still decides
    when a change is handed over; the debounced fire that today writes the
    file instead updates the daemon's in-memory snapshot copy and wakes the
    page streams. Pages never render more often than today.
  - Wake scope: a live-state change runs no full `ReadSession`. The daemon
    updates its snapshot copy in both snapshot readers (the daemon's
    `d.snapshots` used by `pages.go`, and the session source's
    `src.snapshots` behind the index fingerprint and `SnapshotUpdatedAt`),
    patches the index entry's `SnapshotUpdatedAt` in place without raising
    `Rev` (as a heartbeat-only change already does), then wakes the
    streams with a live-only wake kind. A stream woken that way reuses its
    last inbox (held, drained) and board-panel reads and reads the ring
    only when an entry was appended since its last read; on the hosted
    path the ring append (`appendLiveRing`) signals that in process instead
    of an HTTP self-nudge. When a full wake and a live-only wake merge
    before a stream renders, the full wake wins. Registration, the reply registry, mailbox
    history, inbox, and tickets are not re-read on a live wake; they keep
    their own triggers (their writers' nudges and the 30 s backstop poll).
  - Flush clock: the flush keeps its own clock, independent of the change
    debounce, so a long busy turn still writes at least once per heartbeat
    interval: at a debounced fire or heartbeat tick, write when a heartbeat
    interval has passed since the last flush and the state changed since
    it. On the hosted path a heartbeat with nothing changed writes
    nothing, sends no nudge, and runs no `ReadSession`, but refreshes the
    in-memory copy's `UpdatedAt` (memory only, and not a change for the
    flush clock), so an idle hosted session never reads as stale
    (`Snapshot.Stale`, 3 x 30 s). The direct path's heartbeat keeps
    rewriting the file as today, since the file is the daemon's only
    liveness signal there.
  - Flush points: turn end is the turn-complete intake event; session end
    is the hosting per-connection `mcp.Server` closing (`proxy.go`), which
    flushes once more and drops the in-memory copy, so a later direct-path
    writer's file for the same session (for example after a launcher
    fallback) is never hidden; daemon shutdown flushes every hosted session.
  - Wiring: the proxy host is built before the daemon starts
    (`cmd/ws-mcp/dashboard.go` ~78-79), so the hook binds late, as the
    existing `lateFunc` pattern there does.
  Rejected: keeping the per-change file write and removing only the
  in-process HTTP nudge and re-read (half the cost stays, and a file write
  per change is still scanned). Rejected: a new adapter-to-daemon REST
  stream; the adapters' MCP connection already reaches the daemon, and the
  live-snapshot ticket rejected adapter-side discovery and token handling.
  Rejected: a full `ReadSession` per wake (the agentChanged model taken
  literally), which keeps most of the per-intake file opens.
  This revises, for the daemon-hosted path only, the on-disk-only rule
  (169) and the related rejections (174) (the daemon holding state in
  memory and losing it when replaced) and (233 b) (state carried outside
  the file with the file as a backup) of
  `261106-feat-ws-dashboard-live-snapshot-pi`, and the matching rule of
  `261106-feat-ws-dashboard-daemon-core` and
  `261106-epic-ws-dashboard-serve-daemon` ("no protocol that pushes data
  into the daemon"). Those rules were set when ws-mcp was always a separate
  process. Their substance holds: in process the hub is the single source
  of the state, every flush point and the shutdown flush put it on disk,
  and the adapters' resync covers a replaced daemon.
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
  The cache serves only `dashboard.intake` and the live writer's active
  check. Intake's own pass through the `server.go` keyed gate (~692) is
  served from the cache too, so an intake call reads no `key.json`; the
  gate's lookup for every other tool, which gates capability profiles,
  keeps reading `key.json` as today. A removed key therefore stays usable
  for up to 10 s for intake only, which writes nothing but that key's own session display state.
  Rejected: an mtime-validated cache, which still stats the file on every
  call, and the DLP scans stats too. Rejected: caching the general
  tool-call lookup too, which would delay revocation for every capability
  gate.
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
  existing fire-and-forget `/nudge` after its write, naming its own session
  as today's body does, and sends it whether or not the sender is
  registered on the dashboard (it is a no-op only when no daemon runs).
  The daemon's `/nudge` handler wakes every hosted mailbox waiter before it
  validates the named registration, and each woken waiter rechecks its own
  resolved target; a lost nudge falls to the 10 s backstop. Because the
  unchanged body cannot tell a mail nudge from the snapshot, ring,
  whiteboard, and inbox nudges that arrive up to about 10/s, the
  nudge-driven waiter wake is coalesced: at most one recheck per waiter
  per second, with a trailing recheck so a mail nudge still lands within
  about 1 s. In-process mailbox and inbox writes wake waiters without that
  coalescing. This event-driven 1 s recheck, which runs only while nudges
  keep arriving, is an accepted exception to the 10 s floor of (141).
  Technical: wake every waiter over resolving the recipient's reply-id -
  the sender does not know the recipient's project, a named-store address
  has no reply-id, and the nudge body must not change; the coalescing below
  bounds the cost. Today no mailbox send path calls
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "The daemon reads on-disk stores only. It reads each session's stores from disk. ... There is no protocol that pushes data into the daemon." — bearing: constrains
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "The daemon reads on-disk stores only. A stdio ws-mcp registers its project root in a machine registry so the daemon can find each session's stores. There is no protocol that pushes data into the daemon. (169)" — bearing: constrains
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, Result 14605071e): "Nudge. Each successful snapshot or ring write calls `s.dashboardNudge(sessionKey)`. It does not block, and it is a no-op for an unregistered session." — bearing: constrains
- 261110-bug-ws-dashboard-daemon-read-amplification (2026-11-10, Result ceaeeb66f): "`wsdashboard.FileCache[T]` is a stat-checked parse cache ... Added beyond the ticket: a file modified within 2 s of the ..." — bearing: supports
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Result b0e680e23): "an intake batch whose every change is a `KindAgent` event with an `AgentID` waits up to `debounceAgent` (1 s); permission prompts, lead batches, and status keep 100/250 ms." — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "Refresh is event-driven: usage on `session.measure`, ... git on `tool.call` (2 s throttle) and `turn.complete`, ... and a 10 s tick for the budget deltas." — bearing: supports
- c5e3f055 (2026-11-07, commit): "the band shows modified/staged/untracked counts from `git --no-optional-locks status --porcelain` inside the same once-per-heartbeat cached read and 5s timeout." — bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, Decisions): "The adapters' existing `mailbox wait` child gains a flag to also watch that inbox and to report which kind arrived" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/live_snapshot.go, session_auth.go, dashboard_sessions.go, server.go; internal/wsdashboard/daemon.go, index.go; internal/wsmailbox/wait.go; internal/wstrackerhost/watch.go; cmd/ws-mcp/proxy.go; agents-plugin/hooks/claude-mod/band.tsx plus the wsflow mirror |
| scope.surface | cross-module | spans mcp, wsdashboard, wsmailbox, wstrackerhost, cmd/ws-mcp, and the claude-mod hooks in two packages |
| scope.new_public_symbol | yes | an in-process wake hook field on exported mcp.ServerOptions and a mailbox wake signal reachable from wsmailbox.Wait, per Decisions 140 and 141 |
| scope.new_type_contract | yes | the ServerOptions wake hook signature and the in-memory snapshot hand-off from intake to the daemon renderer |
| scope.test_surface | existing | internal/mcp/live_snapshot_test.go, internal/mcp/session_auth_test.go, internal/wsmailbox/wait_test.go, internal/wstrackerhost/watch_test.go, internal/wsdashboard/daemon_test.go, agents-plugin/hooks/claude-mod/band-engine.test.tsx |
| complexity.reuse_points | confirmed | Daemon.agentChanged index.Reread plus broker.wake at internal/wsdashboard/daemon.go#L510-L516; existing dashboardNudge sender in internal/mcp/dashboard_sessions.go |
| complexity.side_effect_risk | high | changes when the snapshot file reaches disk and how the daemon learns of changes, affecting restart recovery and every live page |
| risk.correctness | high | cache staleness of session records, lost wakes, and flush points at turn end and shutdown must all hold or pages and recovery go stale |
| risk.fit | moderate | revises the on-disk-only rule 169 for the daemon-hosted path while the direct path keeps it, so two live paths coexist |
| risk.test | moderate | needs injected clocks and file-op counting across daemon-hosted and direct paths plus claude-mod suites in both packages |
| risk.security_or_contract | moderate | wire formats are declared unchanged but the key-existence cache delays revocation of a removed key by up to 10 s |

## Phases

### Phase 1: Event-driven live path and 10 s poll floor

Implement (140) through (145) as decided above.

Verification:

- Go tests (remote through ws-rgo) showing, on the daemon-hosted path:
  a burst of intake calls renders through the in-process wake with no
  snapshot-file write per call, no HTTP self-nudge, and no `key.json` read
  per call; the snapshot is flushed at heartbeat, turn end, and shutdown;
  a restarted daemon renders the last flushed snapshot.
- A daemon-side file-operation count during an intake burst (an injected
  or counting filesystem seam): no `ReadSession`, no registration, reply
  registry, mailbox history, inbox, or ticket read per wake, and a ring
  read only after an append; wakes follow the existing debounce cadence;
  after a live wake the index entry's `SnapshotUpdatedAt` matches the
  rendered snapshot and `Rev` is unchanged.
- An idle hosted session and an idle direct-path session both stay
  non-stale past 90 s (injected clock); the idle hosted one writes no
  snapshot file meanwhile.
- A continuous busy turn (injected clock) flushes at least once per
  heartbeat interval; an unchanged heartbeat writes nothing; closing the
  hosting `mcp.Server` flushes once and drops the in-memory copy, after
  which a direct-path snapshot file for that session is what renders.
- The direct path still writes the snapshot per change and nudges.
- `mailbox wait` wakes on an in-process write and on a `/nudge` without a
  poll tick (injected clock), and its backstop recheck is 10 s.
- A direct-path mailbox send posts `/nudge` (also from an unregistered
  sender), and the nudge wakes a hosted waiter on a named-store address
  within about 1 s; a burst of non-mail nudges causes at most one waiter
  recheck per second plus a trailing one.
- (142): a removed key stops intake within 10 s, while another tool's call
  through the keyed gate reads `key.json` on every call.
- Tracker watch interval is 10 s; the heartbeat runs no git when no tool
  call ended since the last refresh.
- claude-mod tests: no git refresh for a non-model call; model-call refresh
  at most once per 10 s plus turn end. Run the plugin suites locally with
  `nice -n 19 claude plugin test agents-plugin` and the wsflow package.
- Existing suites stay green: `go test ./...` remotely, plugin tests for ws
  and wsflow.

### Result (82ca8f9e0) - 2026-11-11

Implemented (140) through (145) in six commits (edb6f8a27, fdf968c02,
8d43e769d, 271b76c6c, 84650b458, 82ca8f9e0).

- (140) `mcp.ServerOptions.Live` (a `wsdashboard.LiveHost`, bound late in
  `cmd/ws-mcp` through an atomic pointer on the proxy host) makes a hosted
  server open a per-session `wsdashboard.HostedSlot`. Each debounced fire
  publishes the snapshot in memory. The daemon serves that copy through
  `SnapshotCache.SetLive` (and the session source's `LiveSnapshotHolder`).
  A real change wakes pages with a live-only broker token (`live:<id>`); a
  ring append uses `ring:<id>`. A live-only render reuses the previous
  render's reads (`sessionReads`: page content, held, drained, and the ring
  unless `ring:<id>` moved) and keeps the old refresh stamp. The index's
  `SnapshotUpdatedAt` is patched with no Rev raise. The file flushes when
  dirty once a heartbeat interval has passed, at turn end, and on
  `Server.Close`, which also releases the slot. A failed flush restores the
  flush clock. An idle heartbeat publishes a fresh `updatedAt` with no
  write and no wake. Slots support takeover: the newer slot owns the copy.
  A session directory the daemon does not serve stays on the direct path.
- (141)/(145) A new `internal/wswake` hub. Hosted `mailbox wait` waits on
  hard wakes (an in-process mailbox send, a daemon inbox write) and on
  nudge wakes (`/nudge`, fired before the registration check; coalesced to
  one recheck per second plus a trailing one), with a 10 s backstop. The
  standalone CLI keeps 500 ms. A direct-path mailbox send posts `/nudge`
  even when the sender is unregistered. The tracker watch is now 10 s
  (with a cmd test seam). The band's git throttle is 10 s.
- (142) `sessionStore` intake cache (root, scope, parent). The key's
  existence is re-checked by one stat at most every 10 s. The cache serves
  intake's handler, the live writer's active check, and intake's keyed-gate
  pass only. The touch keeps `touchGuardWindow`.
- (143) The band reads git only for model tool calls. The edit is mirrored
  to wsflow.
- (144) `wslive.State.BatchesEnded` gates the heartbeat repo refresh.

Verification:
- Remote `go test ./...` passes except `internal/wsrsrc`
  `TestPiMirrorUpToDate`. That failure is already on the base (launcher copy
  drift from 40da39aed) and this branch does not touch the file.
- Hosted, direct-path, intake-cache, mailbox-wake, and live-only-render
  tests are listed in the commits.
- `claude plugin test` passes for agents-plugin and agents-plugin-wsflow
  (698 each), and the wsflow python suite passes.

Coverage note: a daemon restart is not tested end to end. It is covered by
two tests together: the close-flush file check, and the slot-close test,
which shows the file renders again once the copy is gone.
