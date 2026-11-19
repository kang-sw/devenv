---
title: "표시 트랜스크립트 메모리 우선: 라이브 세션은 RAM에서 제공하고, 파일은 복구와 종료된 대화에만 사용"
related:
  261117-research-ws-daemon-file-touch-map: "its Outcome Ledger proposals L3 (heartbeat in RAM) and 4.7 (counter-based ring trim, ring as segments) are the open items this ticket would execute; none is implemented"
  261114-feat-ws-conversation-audit-event-store: "would add a second per-conversation store; its design should not repeat the per-entry file pattern"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f486f77711d9a18c
sage-review-completeness-reviewed: f486f77711d9a18c
---

# Display transcript memory-first: live sessions served from RAM, files only for recovery and closed conversations

## Background

The daemon keeps two transcript layers per session: the harness's native
transcript and ws's display transcript (the live snapshot plus the entry
ring) that the dashboard renders. The display layer is fed from the live
stream, not derived from the native transcript.

The performance golden rules (`ai-docs/manuals/performance.md`, AGENTS.md
`## Code Standards` item 6, merged in `965b7a9f1`) say memory is the source of
truth and files are for recovery. The owner's direction (whiteboard (131),
2026-11-18): live transcript traffic should stay in RAM, and file reads should
serve only loading old conversations (and, per the rules, recovery).

A static read of origin/develop `81831d39e` on 2026-11-18 (the display code is
identical on `track/agent-v2`) found the layer still violates the rules:

- **Rule 2, heartbeat rewrite.** Every open conversation rewrites
  `snapshot.json` whole every 30 s: `wslive/snapshot.go:89`
  `HeartbeatInterval`, `wsagent/runner_conv.go:329` and `:400-401`,
  `wsagent/record.go:492-494` ("Heartbeats always reach disk: legacy readers
  do not have the hosted slot"); the dedupe at `record.go:507` is bypassed for
  heartbeats. Every state change also rewrites the whole file
  (`record.go:388-390`) instead of appending a delta. No Rule 8 exception
  covers it. (Those wsagent line numbers are from `81831d39e` and have
  drifted: on `track/agent-v2` the heartbeat tick is
  `runner_conv.go#L526`, the "Heartbeats always reach disk" comment
  `record.go#L530-L531`, the heartbeat bypass of the dedupe `record.go#L544`,
  and the whole-file write `record.go#L553-L554`.)
- **Rule 3, per-record files.** The ring is one file per entry, written by
  temp plus rename (`wslive/ring.go:223-245`), and every append lists the
  directory and removes the surplus (`AppendRing` L177-193, `trimRing`
  L251-263); `RingHistoryCap` 2000, `RingCap` 50 (`wslive/snapshot.go:80-83`).
- **Rules 1 and 5, reads from files.** Each ring-append wake makes the page
  relist the ring directory (`wsdashboard/entry_files.go` `readRing` L36-80,
  `pages.go` `readSessionWith` L1068-1140), and a cold page open reads up to
  50 entry files. A non-hosted `ws-mcp` session's `snapshot.json` is reread
  repeatedly.
- **Already memory-first:** a hosted session's snapshot reaches pages from the
  daemon's RAM slot (`wsdashboard/live_hosted.go` L13-23, `Publish`
  L170-191), and a heartbeat wakes no page. The ring is not held daemon-side.
- **Landed perf work that does not change this layer:** `eed79fc4c` (lead
  close cascades, orphan close at start), `7225f1a83` (one registration read
  per lock), the dedupe of non-heartbeat snapshot writes, `687acb176`
  (dashboard liveness from memory).

Files remain needed for daemon-restart recovery, non-hosted `ws-mcp`
processes, and opening a closed conversation.

No measurement was taken; the read was static.

### Re-verification, 2026-11-19 (track/agent-v2 `930efc597`, static)

- The two hosted paths differ. The hosted `ws-mcp` path already writes no
  file and wakes no page on an unchanged heartbeat, and debounces snapshots
  (`mcp/live_snapshot.go:33-47`, `:84-96`). The hosted **agent-lead** path
  (`wsagent`, leads running inside the daemon) does neither: the heartbeat
  tick (`runner_conv.go:454`, `:526`) reaches `flushSnapshot(true)`
  (`record.go:530-564`), whose dedupe (`:544`) skips heartbeats, so every
  open agent lead rewrites `snapshot.json` every 30 s. Every changed apply
  rewrites the whole file with no debounce; streaming deltas are changes
  (`wslive/state.go:369-386`), about 10 writes/s for Claude
  (`claude.go:740` `claudePartialEvery`) and one per delta for Codex
  (`codex_mapper.go:205-212`).
- The ring and page-read findings above still hold (`wslive/ring.go:177-275`,
  `wsdashboard/entry_files.go:39-80`, `pages.go:1068-1140`); a parsed-entry
  cache (`entry_cache.go`) means a wake reads only new entry files.
- Missing above, on the agent-lead path:
  - every ring append and snapshot write takes the registry flock and reads
    the registration file (`runner_conv.go:780-797` via
    `runner_summary.go:13-62` `withOwner`);
  - every write notifies the daemon (`wsdashboard/daemon.go:668`
    `agentChanged`), which calls `RereadLive` (throttled to once a second per
    session, `index.go:320-345`) listing four directories
    (`agent_sessions.go:355-383`), and the page does a full render that
    rereads the ring;
  - `record.json` is rewritten whole (`record.go:322-364`) on send, kick
    (`runner_conv.go:945-1075`), turn end, and `persistSeq`
    (`runner_process.go:717-722`): about 3-5 times per turn. Review correction:
    `ringDirty` is set on every changed `live.apply`, so `lineEnd`
    -> `persistSeq` also rewrites it per changed line (streaming deltas
    included), the dominant `record.json` write.
- Files are still needed for daemon-restart recovery (`record.go:394-430`,
  `runner.go:912`, `:1513`, `codex.go:90`), opening a closed conversation
  (`live_hosted.go:211-232`), and non-hosted `ws-mcp serve --stdio`. The disk
  heartbeat exists for readers without the hosted slot (`fcf912cd6`); every
  0.50.x daemon has the slot (`2e282425c`, v0.50.0), and an agent lead lives
  inside its daemon.

## Decisions

- **(153), 2026-11-19: three phases, Phase 1 runs now.** Owner: "153=a".
  Phase 1 changes the agent-lead hot path without changing any stored shape;
  Phase 2 holds the ring in daemon RAM with append-only segment files;
  Phase 3 turns `record.json` into a delta append log. Each later phase is
  planned when it is promoted.
  Rejected: Phases 1 and 2 together (Phase 2 needs its own compatibility
  verdict); Phase 1 only with the rest decided after measurement.
- **(153) condition, 2026-11-19: no loss on a graceful shutdown.** Owner: "적어도
  강제 종료가 아닌 종료인 한(종료 신호를 받고 종료하는 한) flush하는 로직이
  있거나 해서 유실되는 메시지가 없게 … 적어도 holder에서 consume한 버퍼는
  파일에 best-effort로 들어가게." Applies to every phase: whatever the daemon
  has consumed from a holder and holds only in memory is flushed to file
  best-effort on a graceful shutdown (`--replace`, close, a termination
  signal). A hard kill is not covered. Tests pin zero loss on the graceful
  path.

- **(155), 2026-11-19: Phase 1 keeps the per-write registry ownership check.**
  Owner: "155=c". `lockSummaryWriter` keeps its registry flock plus
  registration read per write in Phase 1; the in-memory ownership cache moves
  to Phase 2, where the ring-append path (the remaining per-write fence
  caller) is rebuilt. This keeps `7225f1a83`'s rejection of a time-bounded row
  cache intact.
  Rejected: a stat-gated fence (still one file touch per write); ownership in
  RAM revalidated once per heartbeat interval (weakens the terminal-takeover
  fence).
- **(156), 2026-11-19: Phase 1 compatibility tier T1 (display-degraded).**
  Owner: "156=ok". On-disk shapes are unchanged; a `snapshot.json` reader
  without the daemon's RAM slot sees an idle agent lead's file age past
  `wslive.StaleAfterBeats` (3 x 30 s, `wslive/snapshot.go:435-444`) once disk
  heartbeats stop and may show it stale. Nothing is lost, and the daemon
  keeps the live state through the slot, as hosted `ws-mcp` already does
  (`mcp/live_snapshot.go:33-47`). Every 0.50.x daemon has the slot
  (`2e282425c`) and an agent lead lives inside its daemon, so the change is
  patch-releasable.
  Rejected: T0 (the stale display is observable).
- **(157), 2026-11-19: `record.json` coalescing keeps ring-tied `HolderSeq`
  writes immediate.** Owner: "157=ok". In Phase 1 the send, kick, and turn-end
  `record.json` rewrites coalesce to turn boundaries and a graceful shutdown;
  `lineEnd`/`persistSeq` (`wsagent/runner_process.go:703-722`) still writes
  `HolderSeq` at once when the line wrote a conversation entry. That sequence
  is the replay-dedupe cursor: stale after a hard kill, the replay after a
  restart would write ring entries a second time. Phase 2 revisits it.
  Rejected: coalescing `HolderSeq` too.
- **Executor, 2026-11-19 10:52: a Pi worker runs Phase 1.** Owner: Claude
  usage is tight, so Phase 1 runs on a Pi worker rather than a Claude worker;
  briefs stay tight and the lead reviews strictly.

- **(163), 2026-11-19: both kick writes of `agent.json` stay immediate.**
  Owner: "163=ok". Kick is the turn-start boundary and makes two writes: the
  `Queued` write before `proc.Send` (until a turn sent them, the record is the
  only durable copy of drained user messages) and the post-send write that
  clears `Queued`/`Mail`/`Reports` and sets `Drained`/`TurnMark` (deferred, a
  restart after a mid-turn hard kill would re-send the prompt and redeliver
  mail - the duplicate-on-replay class (157) refused for `HolderSeq`). Phase 1
  coalesces only the `Sends` write of `Runner.send` and a child's
  `LastActiveAt` stamp, and flushes every deferred write in `letGo` (`runner_events.go`,
  reached from `Runner.Close` on SIGTERM, `/shutdown`, and `--replace`). This
  narrows the kick part of (157)'s "send, kick, and turn-end" coalescing.
  Rejected: deferring the kick writes (message loss and replay duplicates on a
  hard kill).

- **(134), 2026-11-18: capture only.** Owner: "todo에만 잡아두죠" - Claude
  usage is scarce, so the work is captured in `todo/` and executed later.
- **(142), 2026-11-18: placement.** A standalone `todo/` ticket related to
  `261117-research-ws-daemon-file-touch-map`, not under epic
  `261114-epic-ws-common-session-foundation`; the executor and phase plan are
  chosen at promotion. The daemon file-touch work is tracked by that research
  ticket, not by the session-foundation epic.

## Constraints

- Manuals for the touched paths: `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
- Phase 1's scope is set by (153), (155), (156), (157), and (163); Phase 2 and Phase 3
  mechanisms (where the ring lives in RAM, the in-memory registry ownership
  that (155) moved out of Phase 1, the recovery file shape, the compatibility
  tier) are settled at their own promotion.
- The work runs on `track/agent-v2`, which edits `record.go` and
  `runner_conv.go` heavily, by a Pi worker (Executor decision above; store and
  compatibility sensitive, so the lead reviews strictly).

## Prior Decisions

- 7225f1a83 (2026-11-18, commit): "Rejected: A time-bounded row cache would let a live write slip past a terminal takeover inside the window. That weakens the approved fence: the terminal login (hosted_login.go) and the holder manager write the row from other processes." — bearing: constrains (Phase 1 keeps the per-write fence by (155))
- fcf912cd6 (2026-11-16, commit): "Keep legacy disk heartbeats and their original intervals. Elide only redundant extra hosted snapshots and lead registrations proven equal under the existing ownership lock" — bearing: constrains (Phase 1 supersedes its legacy-disk-heartbeat rule for slotted agent-lead snapshots, per (156))
- 7bc9be16 (2026-11-17, commit): "261117-research-ws-daemon-file-touch-map > Forward: land lifecycle cascade/orphan close (L1) and in-memory hygiene (L2) first; heartbeat-in-RAM (L3) must ship slot + index liveness + no heartbeat writes together." — bearing: constrains
- 2e282425c (2026-11-15, commit): "Reuse the daemon's existing HostedSlot via an optional in-process AgentLiveHost binding before runner Start, open only after owning the registration, publish complete snapshots under the registration/conversation fence" — bearing: constrains
- fc1967ea (2026-11-17, commit): "Approved extension adds timestamp-normalized identical snapshot dedup (21->1 file-only writes expected), forced heartbeats, per-conversation spawn refresh rate limiting with a trailing timer" — bearing: constrains
- 78ff8838 (2026-11-17, commit): "Named-store fingerprints retain unchanged directory prints. ... Unknown runner writes still list all four live directories." — bearing: supports
- edb6f8a2 (2026-11-11, commit): "The live-only wake uses distinct broker tokens (live:<id>, ring:<id>) so the stream cache can tell a snapshot hand-off from a store change without a new wake channel" — bearing: supports
- 261117-feat-ws-mailbox-memory-first-store (2026-11-17, Result e973b72b4): "Reply-id registry and queues: these are part of the snapshot plus the deltas, rebuilt at open." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsagent/record.go, wsagent/runner_conv.go, wsagent/runner_summary.go, wsagent/runner_process.go, wsdashboard/daemon.go, wsdashboard/live_hosted.go |
| scope.surface | cross-module | wsagent writer path, wsdashboard live slot and daemon wake, wslive heartbeat constant; no stored-shape change |
| scope.new_public_symbol | no | the slot wake reuses `Publish` and `RingAppended` (`wsdashboard/live_hosted.go`), as hosted ws-mcp does |
| scope.new_type_contract | no | existing slot interfaces suffice |
| scope.test_surface | existing | wsagent/snapshot_io_test.go, wsagent/record_test.go, wsagent/runner_test.go, wsdashboard/live_hosted_test.go |
| complexity.reuse_points | confirmed | mcp/live_snapshot.go#L33-L47 write rule and wsdashboard/live_hosted.go daemonLiveSlot Publish were read |
| complexity.side_effect_risk | high | changes crash and restart recovery cadence (snapshot and record.json write timing); the ownership fence is unchanged |
| risk.correctness | high | graceful-shutdown flush must lose nothing and the ownership fence must stay untouched (7225f1a83) |
| risk.fit | moderate | the hosted ws-mcp path already implements the target rule; the agent-lead path is a different writer in wsagent |
| risk.test | moderate | write-count tests exist via writeSnapshotFile seam; zero-loss shutdown tests and a before-after measurement are new |
| risk.security_or_contract | moderate | on-disk shapes stay but a legacy reader sees a less frequent snapshot heartbeat and may mark the session stale |

## Phases

### Phase 1: Agent-lead hot path without a stored-shape change

Scope, on the hosted agent-lead path:
- The heartbeat stays in RAM: no unconditional `snapshot.json` write per tick.
  This applies only to a conversation that holds a daemon live slot (a lead
  whose `OpenHostedSlot` succeeded); children, Pi workers, and a lead without
  a slot are file-only writers and keep the disk heartbeat. The tick still
  publishes the snapshot so the daemon's copy stays fresh.
- Snapshot file writes follow the hosted `ws-mcp` rule
  (`mcp/live_snapshot.go:33-47`): a file write at turn end and close, and
  otherwise at most once per heartbeat interval when the state changed. The
  live state keeps reaching pages through the RAM slot.
- Registry ownership keeps its per-write flock plus registration read
  ((155)); it is not changed in this phase.
- The daemon is woken through the live slot instead of `RereadLive`'s
  directory listings per write: a snapshot change calls the slot's `Publish`
  and a ring append its `RingAppended`, as hosted `ws-mcp` does. Only the
  per-apply `notify` of a changed line and the summary-store wake switch;
  `notify` for registration writes, mail, start failures, and observed
  evidence stays. No new public symbol is expected.
- `ringDirty` narrows to real ring appends (`len(res.Ring) > 0`), not every
  changed `live.apply`; today every changed line, streaming deltas included,
  makes `lineEnd`/`persistSeq` rewrite `agent.json`. A snapshot-only line's
  `HolderSeq` waits for the next ring line, the heartbeat, or the shutdown
  flush.
- `record.json` (the file is named `agent.json`; `wsdashboard.AgentRecordFileName`)
  rewrites coalesce ((157), (163)): both kick writes (`Queued` before the send,
  the clear and `Drained`/`TurnMark` after it) stay immediate, as does the
  `HolderSeq` write tied to a ring entry; only `Runner.send`'s `Sends` write
  and a child's `LastActiveAt` stamp are deferred to turn boundaries, and every
  deferred write is flushed in `letGo` on a graceful shutdown (SIGTERM,
  `/shutdown`, `--replace`). Every write that is the only durable copy of
  already-consumed input stays immediate too: the mailbox claim's `Mail`
  append (`runner_conv.go` mail claim), `requeueSteered`'s `Reports`,
  `saveSteered`'s `Steered`/`SteerSeq`/`TurnMark`, and the `PendingReports`
  take/remove writes (`children.go`).
- The graceful-shutdown flush of the (153) condition covers everything this
  phase holds only in memory.

Every new deferred or shutdown snapshot write goes through the same
`snap.ForDisk()` projection as today's writes (the live reasoning summary
never reaches disk).

Verification: tests pin the reduced write counts (an idle slotted lead: no
snapshot file write per heartbeat; a changing one: at most one per heartbeat
interval plus turn end and close; `agent.json` writes during a streamed reply
no longer scale with deltas) and zero loss on a graceful shutdown; per-turn file-operation counts are measured before and after on the
owner's PC with the method of `261117-research-ws-daemon-file-touch-map`. The
tests gate the phase; the measurement is taken by the lead or owner on the
owner's PC after the merge and recorded in the Result. Go builds and tests run
on the remote w5090 only.

### Result (c47ac38d6) - 2026-11-19

- Slotted agent leads publish every change/heartbeat to RAM; unchanged
  heartbeats write no snapshot file. Changed recovery projections write at
  most once per heartbeat interval, plus forced turn-end and close writes.
  Every disk projection, including shutdown, uses `ForDisk`. File-only
  writers retain disk heartbeats. Stored shapes and the per-write ownership
  fence are unchanged (accepted compatibility tier T1).
- Transcript changes and summary-store appends use the existing slot's
  `Publish`/`RingAppended`, with the file-only notify fallback. Registration,
  mail, start-failure, and observed-evidence notifications remain in place.
- `ringDirty` now belongs to the live writer, which knows actual
  ring-generating results, rather than treating every changed snapshot as a
  ring append. An owned append batch is marked even on partial failure, to
  fence any entries it already wrote. Ring-tied `HolderSeq` stays immediate;
  snapshot-only cursors wait for a ring line, heartbeat, or shutdown.
- Only `Runner.send`'s coverage/activity metadata is deferred. Both kick
  writes, claimed Mail, Reports, Steered/SteerSeq/TurnMark, and PendingReports
  remain immediate. Immediate writes also flush the deferred metadata;
  turn-end and `letGo` flush it explicitly, including with no live process.
- Deterministic tests: 200 changed reply projections make 1 slotted snapshot
  replacement versus 202 on the unchanged file-only control (including
  initialization/turn start). Idle hosted heartbeats leave the file unchanged
  while refreshing RAM. 200 snapshot-only holder lines make 0 record
  replacements; a finished reply's ring line makes 1 immediate cursor write.
  Tests also pin both kick writes, failed-flush retry, Claude/Pi graceful
  shutdown's consumed prefix and held input, child Send metadata with no
  process, and reasoning-summary exclusion from the shutdown snapshot.
- Verification, **w5090 only**: after
  `python3 scripts/generate-package-assets.py`, `go build ./...`,
  `go vet ./internal/wsagent ./internal/wsdashboard ./internal/mcp ./cmd/ws-mcp`,
  and `go test -count=1 -timeout=10m` of those four packages passed
  (134.805s / 279.766s / 484.355s / 296.586s respectively). The first check
  failed on three new test fixtures whose directory basename did not match
  the record's reply-id, and on the missing embedded-assets prerequisite;
  corrected the fixture directories and generated assets remotely before
  the passing full check. No implementation failure was hidden or test
  expectation weakened. `git diff --check` passed; no local Go build/vet/test.
- Review: reviewer-child tools are unavailable, so R1 was the lead-authorized
  self-review fallback, not independent review. No Critical/Important/Minor
  findings; no R2 needed. The lead's independent Claude diff review remains
  before merge. No merge, push, or daemon replacement was performed.
- Forward: the lead/owner takes the ticket's before/after per-turn syscall
  counts and latency measurement on the owner's Windows PC after merge and
  records it in an Edition. The fixture counts above are not that measurement.
  Phase 2 and Phase 3 remain unimplemented; this ticket stays in `ready/`.

### Phase 2: Ring in daemon RAM, append-only segment files

Planned at its promotion, including its compatibility tier with source lines.
Carries the in-memory registry ownership that (155) moved out of Phase 1.

### Phase 3: `record.json` as a delta append log

Planned at its promotion.
