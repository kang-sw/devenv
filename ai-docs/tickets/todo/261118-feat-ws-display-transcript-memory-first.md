---
title: "Display transcript memory-first: live sessions served from RAM, files only for recovery and closed conversations"
related:
  261117-research-ws-daemon-file-touch-map: "its Outcome Ledger proposals L3 (heartbeat in RAM) and 4.7 (counter-based ring trim, ring as segments) are the open items this ticket would execute; none is implemented"
  261114-feat-ws-conversation-audit-event-store: "would add a second per-conversation store; its design should not repeat the per-entry file pattern"
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
  covers it.
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
    (`runner_process.go:717-722`): about 3-5 times per turn.
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
- Phase 1's scope is set by (153); Phase 2 and Phase 3 mechanisms (where the
  ring lives in RAM, the recovery file shape, the compatibility tier) are
  settled at their own promotion.
- The work runs on `track/agent-v2`, which edits `record.go` and
  `runner_conv.go` heavily, by a Claude worker (store and compatibility
  sensitive).

## Phases

### Phase 1: Agent-lead hot path without a stored-shape change

Scope, on the hosted agent-lead path:
- The heartbeat stays in RAM: no unconditional `snapshot.json` write per tick.
- Snapshot file writes follow the hosted `ws-mcp` rule
  (`mcp/live_snapshot.go:33-47`): a file write at turn end and close, and
  otherwise at most once per heartbeat interval when the state changed. The
  live state keeps reaching pages through the RAM slot.
- Registry ownership is held in memory instead of a flock plus registration
  read per write.
- The daemon is woken through the live slot instead of `RereadLive`'s
  directory listings per write.
- `record.json` rewrites within one turn are coalesced.
- The graceful-shutdown flush of the (153) condition covers everything this
  phase holds only in memory.

Verification: tests pin the reduced write counts and zero loss on a graceful
shutdown; per-turn file-operation counts are measured before and after on the
owner's PC with the method of `261117-research-ws-daemon-file-touch-map`. Go
builds and tests run on the remote w5090 only.

### Phase 2: Ring in daemon RAM, append-only segment files

Planned at its promotion, including its compatibility tier with source lines.

### Phase 3: `record.json` as a delta append log

Planned at its promotion.
