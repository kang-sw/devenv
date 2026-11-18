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

## Decisions

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
- The mechanism (heartbeat removal, where the ring lives in RAM, recovery file
  shape, compatibility tier) is settled at promotion through the Open
  Decision Queue; nothing in this ticket chooses it yet.
- `track/agent-v2` edits `record.go` and `runner_conv.go` heavily; the
  executor coordinates with that track before starting.

## Phases

### Phase 1: Plan at promotion

The phase plan is written when the ticket is promoted to `ready/`.
