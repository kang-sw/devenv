---
title: "Daemon file-touch map: what each file does, how it is coupled, and where reduction pays"
related:
  261117-research-ws-dashboard-data-only-client-assembly: sibling research; page-load cost, discussed after daemon round 2
  261116-feat-ws-daemon-hub-first-polling: in-progress daemon polling work whose wakes drive rereads
---

# Daemon file-touch map: what each file does, how it is coupled, and where reduction pays

## Background

On the owner's Windows PC the shared dashboard daemon spends most of its CPU in
file syscalls. AhnLab V3 makes each one 10-100x slower, and its scanner
(`ASDSvc`) uses more than a full core while the daemon churns files. The owner
gated the release on this (clone note `release-gate-daemon-file-io-20261117`).

Owner, 2026-11-17 13:30-13:31, verbatim:

> 애초에 ASDSvc라던지 이런 검사 프로세스의 CPU 점유 자체가 파일 터치할 때 일어나는거라 근본적으론 우리가 파일 터치를 최소화해야 합니다. 백업 목적의 flush만 하기, 필요한 delta만 쓰기, etc etc ... 그리고 대화가 228개나 떠있다는 것도 납득은 안 가고요. 서브에이전트가 하나도 정리가 안 되고 있거나 그런 거 같은데. 제가 손으로 띄운 세션도 200개가 안 될거라

> 아뇨 이건 근데 일단 그냥 고치라고 하지 말고, 티켓부터 잡아서 설계부터 합시다

> 어느 부분이 어떻게 엮여있고 어떻게 줄여야 효과적인지, 이런 부분을 먼저 파악해야 상태 유실이라던지 품질 문제가 발생하지 않습니다.

Translation:
- The scanner's CPU use is caused by file touches, so the root fix is to
  minimize file touches: flush only for backup, write only the needed deltas,
  and so on.
- 228 live conversations make no sense. Subagents seem never to be cleaned up;
  the owner launched fewer than 200 sessions by hand.
- Do not just order fixes. Open a ticket and design first.
- First understand which parts are coupled and how, and which reductions are
  effective, so that the reduction causes no state loss or quality problems.

This ticket is that understanding step. It maps the daemon's file touches
before any write-strategy change is designed. Implementation tickets derive
from its Outcome Ledger.

## Scope

In scope:
- Every file the shared daemon process (`ws-mcp-dashboard-*.exe`) reads,
  writes, stats or lists in steady state and on interactive paths, grouped by
  store.
  - wsagent conversation records: `snapshot.json`, the ring and the record
    file.
  - The registration / lock rows.
  - The wsinbox held, drained, answer-pending, attachment and journal
    directories.
  - The wslive observed evidence (`model-evidence`), the snapshot cache and
    the Index.
  - The whiteboard folders and their flock.
  - Mailbox, tracker and plan-usage history.
  - The localhost secret and endpoint files.
  - Ticket listings and record files read for lead pages.
- For each store:
  - **Writers and readers:** who writes it and who reads it, in-process and
    cross-process, including older same-minor clients, the launcher,
    standalone `ws-mcp` commands, hosted processes and Electron.
  - **Trigger:** what triggers each touch (event, timer, heartbeat, request).
  - **Rate and cost:** measured or estimated rate, and amplified cost.
  - **Restoration role:** what the file restores after a daemon restart or
    crash, and what would be lost if it were written later or less often.
  - **Ordering and atomicity:** the guarantees a reader relies on.
  - **Compatibility:** the AGENTS.md compatibility tier of changing its timing,
    its granularity (delta vs rewrite), or its existence.
- **Coupling:** where one store's write triggers rereads of others, such as:
  - runner write → `agentChanged` → `Index.Reread` → record, board and ticket
    rereads;
  - snapshot flush → fence → registration reads;
  - `seedSpawnRows` → `Runner.List` → every inbox.

  Include the wake fan-out that turns one write into N reads.
- **Conversation lifecycle:** why 228 `conv.loop` goroutines are live (owner:
  fewer than 200 sessions ever launched by hand). Establish:
  - when conversations are created, including restore at daemon start;
  - when they are dropped;
  - whether ended, stopped or idle-timed-out conversations keep a loop,
    heartbeat and snapshot writes.

Out of scope:
- Implementing write-strategy changes. Each one gets its own actionable
  ticket, derived from the Outcome Ledger.
- Page payload and client rendering (sibling research
  `261117-research-ws-dashboard-data-only-client-assembly`).

Already in flight (bounded fixes that keep on-disk shape and write timing; this
research takes their results as the baseline):
- `impl/develop/daemon-io-r2a`: M8 answer-replay lock, M2 evidence-cache sweep
  locking, M1 in-memory held counts, M5, M4, syscall counter seam.
- `impl/develop/hosted-send-path`: F2, `PageInput` off the message POST path.
- `impl/develop/composer-instant-bubble`: F1, optimistic client bubble.
- r2b, merged at af93c65b3: incremental rail and live-store-only rereads.

## Questions

1. **The touch inventory.** Which stores account for the touches, in
   touches/s on the owner's PC? Ranked, with the code path for each.
2. **Restoration contract per store.** Which data must be on disk:
   - before acknowledging the user (for example, an accepted message);
   - within a bounded window (for example, a live snapshot);
   - only at clean stop?

   Which files are pure caches that could be dropped or rebuilt?
3. **Coupling graph.** Which writes cause rereads elsewhere, and which reads
   exist only because the daemon rereads its own writes?
4. **Reduction options and their failure modes.** For each high-rate store,
   evaluate:
   - RAM-only plus periodic or clean-stop flush;
   - append-only delta instead of whole-file rewrite;
   - dropping fsync where atomic rename suffices;
   - coalescing;
   - dropping heartbeat-only writes;
   - in-memory ownership fences.

   For each option, state:
   - the state-loss window on crash;
   - the effect on restart recovery, staleness badges and cross-process
     readers;
   - its compatibility tier.
5. **Conversation leak.** What is the lifecycle gap behind 228 loops? What is
   the safe cleanup rule, and what restore behaviour does it imply?
6. **Ordering.** Given the coupling, which changes must land together, and
   which first, to reduce touches without state loss?

## Method

- **Read-only.**
  - Source reading and evidence of the running daemon only.
  - No local `go test`, ws/ws-mcp executables, launcher, daemon or Electron on
    the owner's PC.
  - Existing pprof captures may be analysed: `D:/Temp/daemon-live-1314/`,
    `D:/Temp/send-latency/dump/`, `D:/Temp/daemon-io-audit/`.
- **Measurement.** Use the r2a syscall counter seam (`/debug/pprof/streams`)
  once it lands on the running daemon, for per-store touch rates on the owner's
  PC.
- **Evidence.** Every claim in Verified Findings cites file:line or a
  measurement file.

## Outcome Ledger

### Verified Findings

- **Live CPU profile, 2026-11-17 13:17-13:27** (`D:/Temp/daemon-live-1314/`):
  - 72% of daemon CPU samples are Windows file syscalls (CreateFile, CloseHandle,
    GetFileAttributesEx, GetFileInformationByHandleEx, MoveFileEx).
  - The goroutine dump had no lock waiters at capture time.
  - 228 `wsagent.(*conv).loop` goroutines were live.
  - `ASDSvc` was at about 105% CPU.
- **Top paths in the 20 s CPU window:**
  - `conv.loop` → `liveWriter.flushSnapshot` / `writeFileAtomic` (record.go ~486-512), including `heartbeatSnapshot`: about 48% cumulative, together with `conv.state` → `seedSpawnRows` → `Runner.List` → `wsinbox.Held` ReadDir.
  - The live stream `pageCache.getSince` → snapshot/Index reads: about 28%.
- **Earlier send-latency trace** (develop 28f523038, 13:03 dumps):
  - one render held the wslive observed-evidence slot inside a single `ObservedCache.Load` for at least 33 s;
  - the session involved has about 2,100 `model-evidence` files;
  - a full resweep is forced every 15 s (`observed_cache.go:16`).
- **Hosted-lead live writes** take a registration flock plus two registration
  reads per write (`runner_summary.go:13-30`, `runner_conv.go:590-605`).
- **Cross-process readers.** For current-minor clients, the inbox, live, ring
  and snapshot files are read cross-process by effectively no one. The MCP
  servers, `mailbox wait`, `dashboard.inbox_drain` and the hosted runners all
  run inside the daemon (`wsdashboard/proxy.go:17-27`,
  `cmd/ws-mcp/hosted_cli.go:136-144`, `mcp/dashboard_inbox.go:60-123`).

  The remaining cross-process readers are:
  - a manually run standalone `ws-mcp mailbox wait`;
  - older minors, already behind the T3 boundary;
  - Electron, which uses HTTP.

  This is unverified for every store, so Question 1 must confirm it per store.
- **Conversation leak** (read-only check, 2026-11-17 ~13:35): nothing closes
  finished conversations automatically.
  - **Restore at start.** `Runner.Start` (runner.go:262-291) restores every
    `sessions/<id>/agent.json` with no `ClosedAt` or `HandedOffAt` through
    `startConv`. Each one gets a permanent `conv.loop` with a heartbeat ticker
    (runner.go:518-523, runner_conv.go:318).
  - **Heartbeat writes.** Each beat calls `heartbeatSnapshot` (runner_conv.go:390), which always writes (record.go:488-489), then `persistSeq` and `writeRegistration` or `checkRow`.
  - **Idle timeout.** It only stops the process (runner_conv.go ~379-383, profile.go:46). The conversation, its loop and its heartbeat stay.
  - **Closing.** Only `CloseConversation` / `closeOne` (runner.go:1103-1138) closes a conversation. Its callers are `pi.stop`, hire, the HTTP close, a failed spawn and the parent-close cascade.
  - **Disk.** Under `C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\` there are 476 records:
    - 246 are closed;
    - 230 are open: 211 Pi children, 18 leads and 1 root hosted Claude.
    - Of the open records, 200 were last modified before today, and 225 have `turn_open=false`.
    - All 230 open snapshots were rewritten today by heartbeats.
    - This matches the 228 live loops.

### Confirmed Decisions

- Owner 2026-11-17 13:30-13:31: do not improvise write-strategy fixes. First
  map the coupling and effective reductions so that reduction causes no state
  loss or quality problems, then design. Files serve backup and restoration;
  the target direction is backup-only flushes and delta writes.
- Owner 2026-11-17 12:54: no release until daemon file-system load is fixed.
  Judge by measured syscalls/s and latency on the owner's PC.

### Proposals

- The investigator's in-memory accept with write-behind queue (F6), and its
  background `snapshot.json` writer with heartbeat in RAM (F4c). Both are
  candidates, not decisions.

### Open Questions

- All of `## Questions` above.

### Rejected Alternatives
