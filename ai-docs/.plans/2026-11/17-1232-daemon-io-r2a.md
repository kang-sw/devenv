# Daemon file I/O round 2a: syscall counter, answer-replay lock, per-event O(N) reads

## Context

Owner, 2026-11-17 11:56: several daemon "fixes" have shown no visible
improvement, and the regressions go back and forth. The shared dashboard
daemon on the owner's Windows PC still delivers messages about 10 s late.

An opus audit of the profiles at 11:18 and 11:43 found the following.
- The source for those profiles is 892d59c23. File:line references below
  are to that source; develop has moved since, so re-locate them.
- The profile extracts are in `D:/Temp/daemon-io-audit/`, and that build's
  source tree is in `D:/Temp/daemon-io-audit/src/`.

Findings:
- Real CPU is about 0.5 core.
- The cost is blocked Windows file syscalls, about 5,000-7,000 per second.
  - Each call is 10-100x normal NTFS cost: stat about 0.4-0.7 ms, a small
    ReadDir 1.5-3 ms, an atomic replace about 24 ms. This is likely
    antivirus amplification. The owner declined an AV exclusion: reduce the
    syscalls.
- Every top path is an O(N) re-read driven by an event (wake, board-write
  poke, child event), not by a timer.
- Earlier fixes mostly slowed timers. They were validated on fixture CPU or
  on Linux, never on syscalls/s on the owner's machine.

7cb54027f (merged 12:33, the stat-storm round) already adds:
- an O(1) board cache check on the dashboard path;
- 1 s TTLs and rate limits on several paths;
- per-view render single-flight.
The audit expects that round to cut some traffic, but not 10x. A parallel
worker (round 2b, branch impl/develop/daemon-io-r2b) owns the rail
incremental view and the Index/fingerprint split. Do not touch `rail.go` or
the Index reread logic.

## Outcome

0. **Syscall counter first.**
   - Add a counting seam for `os.Stat`/`Lstat`, `os.ReadDir`, `os.Open`/
     `OpenFile`/`ReadFile` and atomic writes in the hot daemon packages:
     wsagent, wslive, wsinbox, wsdashboard, mcp and wswhiteboard. Use the
     narrowest seam that covers the paths below; a package-level counter
     helper is enough.
   - Expose the totals and per-site counts in `/debug/pprof/streams`.
   - This is how the owner's machine will judge the change, so it must count
     the real calls, not estimates.
1. **Answer replay (M8)**, `wsdashboard/inbox_routes.go` about :262, :284-312;
   `mcp/dashboard_answer_revision.go:12-21`; `daemon.go:598`.
   - Never hold the daemon-wide `inboxMu` across `RecordDashboardAnswer` or
     any board lock. Lock only the journal scan and unlink.
   - Stop retrying permanent errors such as an identity mismatch: log once
     and drop or park the entry.
   - Schedule a replay from `sessionChanged` only when the journal is known
     to be non-empty, tracked in memory.
2. **Spawn-row reseed (M1)**, `wsagent/runner_conv.go:419` (`conv.state` →
   `wsinbox.Held`), `runner.go:859-871` (`List`), `spawn_live.go:114-163`
   (`seedSpawnRows`, `wakeSpawnParents`), `permission_store.go:14`.
   - Keep each conversation's held-message count in memory. Update it where
     the conversation already sees its inbox change (`inboxChanged`, Send,
     drain).
   - Cache the lead permission selection and invalidate it on write.
   - `seedSpawnRows` walks a parent→children index instead of `List()`.
   - `List()` must do no per-conversation file I/O on this path.
     `Held()` may stay on explicit API/hire paths.
3. **Evidence cache (M2)**, `wslive/observed_cache.go:16,79,114,133,168,175`.
   - Drop the 15 s forced per-record re-sweep: evidence files are write-once
     (see :24-26). Trust directory-listing diffs, with at most a rare sweep
     (5 min or more, or only when the directory changes).
   - Replace `os.SameFile` at :79 and :175 with `wsstate.SameFileMetadata`,
     as fcf912cd6 did elsewhere.
4. **Nudge discovery (M5)**, `mcp/whiteboard_nudges.go:166-232,354,366`.
   - A Poke checks only the poked (project root, session key): one identity
     stat plus that board.
   - The full `discover()` over all session keys runs only on the 5-minute
     recovery timer.
5. **Whiteboard cache remainder (M4)**:
   - Replace the 64-board cap that resets the whole map
     (`wswhiteboard/folders_cache.go` about :109) with an LRU, or raise it to
     512 or more.
   - Route MCP `whiteboard.query` (`mcp/whiteboard_store.go:44-46`) and
     nudge `Notify` loads (`board_notice.go:238-252`) through the shared
     folder cache, with read-your-writes kept.

## Constraints

- Read before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`;
  - `ai-docs/manuals/ws-mcp.md`, for `internal/mcp/`.
- In-memory only (T0). If anything persisted or cross-version changes, stop
  and report it with the tier and source lines.
- Simplest complete change: no new frameworks or configuration knobs.
- Keep these behaviours:
  - board writes visible within about 1 s;
  - MCP read-your-writes;
  - dashboard sends never blocked by board locks;
  - the hire/answer API still sees the true held counts.
- NO local `go test`, Go test executables, ws/ws-mcp executables or the
  launcher on this PC (owner AV rule). Do not start the daemon or Electron.
  - Go tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run io-r2a <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/flyaway-fifteen-judgingly' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsagent ./internal/wslive ./internal/wsinbox ./internal/wsdashboard ./internal/mcp ./internal/wswhiteboard ./internal/wsi18n'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
  - Failing tests are your work to fix. If a test also fails at base 7cb54027f,
    record it as one Forward.
- Temp files only under `D:/Temp/daemon-io-r2a/`, deleted afterwards.
- Commits carry no Co-Authored-By trailer. Do not push.

## Verification

- Counter-based tests, through the new seam:
  - **M1:** a child event with N=200 conversations does O(1) file syscalls,
    not O(N).
  - **M2:** loads over 60 s of fake time cause no per-record re-stat.
  - **M5:** a poke does O(1) syscalls, not O(keys).
  - **M8:** a replay blocked on a board lock does not block a concurrent
    dashboard send. A permanent error is not retried forever.
  - **M4:** a `whiteboard.query` cache hit does O(1) stats.
- One w5090 run of the list above; R1, then a fixes-only R2 for substantive
  findings.
- Report:
  - commits and files;
  - per-item before/after syscall counts;
  - the tier;
  - anything skipped and why.
