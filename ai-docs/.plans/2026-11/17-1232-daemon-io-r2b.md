# Daemon file I/O round 2b: incremental rail and live-store-only rereads

## Context

Owner, 2026-11-17 11:56: several daemon "fixes" have shown no visible
improvement. The shared dashboard daemon on the owner's Windows PC still
delivers messages about 10 s late.

An opus audit of the profiles at 11:18 and 11:43 found the following.
- The source for those profiles is 892d59c23. File:line references below
  are to that source; develop has moved since, so re-locate them.
- The profile extracts are in `D:/Temp/daemon-io-audit/`, and that build's
  source tree is in `D:/Temp/daemon-io-audit/src/`.

Findings:
- Real CPU is about 0.5 core.
- The cost is blocked Windows file syscalls, about 5,000-7,000 per second.
  Each call costs 10-100x normal: stat about 0.5 ms, ReadDir 1.5-3 ms.
- Every top path is an O(N) re-read driven by an event, not by a timer.
- Rendering runs back to back while wakes keep arriving (about 9 wakes/s).
  Making one pass cheaper only raises the pass rate. The number of reads per
  trigger has to fall, and the trigger rate needs a cap.

7cb54027f (merged 12:33) added:
- 1 s TTLs;
- per-session wake coalescing at 250 ms;
- per-view render single-flight.
It did not change what the rail and the Index re-read.

A parallel worker (round 2a, branch impl/develop/daemon-io-r2a) owns:
- a syscall counter seam exposed in `/debug/pprof/streams`;
- the answer-replay lock;
- wsagent `List`/spawn rows;
- the wslive evidence cache;
- nudge discovery;
- the whiteboard cache cap and MCP query.
Do not touch those. If you need a counter for your tests, add a local
test-only counting hook. The 2a seam lands separately, and the merge
reconciles the two.

## Outcome

1. **Incremental rail (M3).**
   - Where it reads now:
     - `wsdashboard/rail.go:67` (`railBaseView`) re-reads every listed
       session (63) on any wake, because the rail page source has nil deps
       (`rail.go:112-131`, kept on purpose by 632db1e7a);
     - `sse.go:106-121`;
     - `stream_cache.go:197-251`.
   - Keep each session's rail input in memory, keyed by what changes it: the
     Index revision, the live ring generation and the snapshot generation.
   - Re-read only the sessions named by wakes since the last pass, and
     recompute clock-derived liveness in memory.
   - Cap rail recompute at 1 per second, with a trailing pass so the last
     change always shows.
   - Workspace/worktree lists and layout keep coming from their existing
     in-memory sources.
2. **Live-store-only rereads (M6).**
   - Today:
     - `daemon.go:623-629` (`agentChanged`) triggers `Index.Reread`
       (`index.go:403-481`) on every runner write, about 4.8/s;
     - for a lead, `DashboardSessionSource.ReadSession`
       (`mcp/dashboard_sessions.go:898-913`) then re-reads the record, the
       board, ticket listings and the mailbox history every time;
     - 39% of lead rereads produce an identical fingerprint.
   - A runner write refreshes only the live-store part of the fingerprint:
     snapshot, ring and inbox.
   - Split the lead fingerprint into that live part and a
     record/board/tickets/mailbox part. Re-read the second part only on:
     - a ws-mcp `Nudge`;
     - the periodic `Refresh`;
     - a board poke for that session.
   - Coalesce per-session rereads to at most once per second, with a trailing
     reread.
   - `HashLiveStoresClock` (`wsdashboard/agent_sessions.go:221-247`), which
     does 4 directory listings per read, should list only the store the write
     named, where the caller knows it.
   - `RefreshClock` (`index.go:328-345`) rereads stale non-ended sessions
     every 30 s. Make it recompute liveness from the in-memory snapshot time
     instead of re-reading files, if that is straightforward.

## Constraints

- Read before editing:
  - `ai-docs/manuals/shipped-surface-boundary.md`;
  - `ai-docs/manuals/release-compatibility.md`;
  - `ai-docs/manuals/ws-mcp.md`, for `internal/mcp/`.
- In-memory only (T0). If anything persisted or cross-version changes, stop
  and report it with the tier and source lines.
- Simplest complete change: no new frameworks or configuration knobs.
- Keep these behaviours:
  - the rail and session pages reflect a change within about 1 s;
  - session start/stop, ended/hidden/label changes and worktree changes
    still show;
  - stale/live badges still age correctly without wakes.
- NO local `go test`, Go test executables, ws/ws-mcp executables or the
  launcher on this PC (owner AV rule). Do not start the daemon or Electron.
  - Go tests run on w5090 only:
    `D:/sys_lock.sh remote w5090 run io-r2b <min> --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/guiding-dragonish-consumer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard ./internal/mcp ./internal/wslive ./internal/wswhiteboard ./internal/wsi18n'`.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
  - Failing tests are your work to fix. If a test also fails at base 7cb54027f,
    record it as one Forward.
- Temp files only under `D:/Temp/daemon-io-r2b/`, deleted afterwards.
- Commits carry no Co-Authored-By trailer. Do not push.

## Verification

- Counter-based tests:
  - **M3:** with 60 sessions, a burst of 100 wakes naming one session
    re-reads only that session's inputs, and renders the rail at most about
    once per second plus a trailing pass.
  - **M6:** a runner write for a lead does not re-read the record, board,
    tickets or mailbox; a Nudge does.
- One w5090 run of the list above; R1, then a fixes-only R2 for substantive
  findings.
- Report:
  - commits and files;
  - per-item before/after counts;
  - the tier;
  - anything skipped and why.
