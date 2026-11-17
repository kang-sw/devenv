---
title: "Hosted compaction lifecycle: daemon compaction nudge, session-state restore bundle and ws/compact (summarize, new harness session, inherited ws key)"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-harness-capability-catalog: "prerequisite; its compaction item grade (native / ws-substitute) selects the fallback; its Decision 6 defers the ws-substitute build to this ticket"
  261117-feat-ws-single-session-holder: "reattach after daemon restart targets the newest harness-session segment"
  261117-feat-ws-exec-tools: "daemon-owned exec jobs survive compaction; the restore bundle lists them"
  261117-feat-ws-agent-tools-mailbox-routing: "children and lineage keyed by the ws key; compaction applies to children too"
  261117-feat-ws-hosted-prompt-assembly: "sibling stage-2 child; shares the daemon injection path"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
---

# Hosted compaction lifecycle

## Background

Part of owner 18:35's "application layer" on top of the common harness
interface, placed in stage 2 by (28)=a: (1) a daemon-owned compaction nudge, and
lead addition A, a daemon session-state restore without the lead-revive ritual.
This ticket also builds `ws/compact`, the common compaction command. Per owner
(31)=a the catalog ticket (Decision 6) names this ticket the owner of
`ws/compact`, the compaction nudge and the `ws-substitute` (summarize and
continue in a new session); `ws/compact` is the default compaction method for
hosted sessions, and the catalog's native compaction grades (Claude and Pi
through the existing `/compact` path) remain factual inputs for the fallback.

Owner 18:39 goal: the nudge and the state restore apply to child agents too, not
only leads. Subagent compaction timing, notably for Pi children, was hard; the
expected gains are subagent quality and token efficiency.

Evidence of today's state:

- Pi compaction (`agents-plugin-pi/src/lead-compaction.ts`, `goal-loop.ts`):
  the session writes prose through the `ws-compact` lever under seven `###`
  sections (`LEAD_PROSE_SECTIONS`: User preferences and style; Agreed working
  practices; Decisions not yet recorded; Current work; Immediate next step; Mood
  and rapport; Residual details). The adapter adds fixed sections: `## Session`
  (ws key, ticket, playbook), `## Carried forward by the lead`, child agents in
  flight and finished, background tasks, `## Dialog` near-raw under a byte
  budget, required re-reads inlined, on-demand references, and `## Resume`.
  `session_before_compact` returns the extension's own compaction, bypassing
  Pi's summarizer; without lever prose a separate tool-less model call is the
  fallback (`FALLBACK_SYSTEM_PROMPT`). Compaction is in place (`ctx.compact()`);
  no new-session path exists. Agenda and todos are not in the summary
  (`lead-revive` restores them).
- The daemon has no mechanism to start a new hosted harness session that
  inherits an existing ws session key. Nearest: Claude `--resume spec.SessionID`
  (`wsagent/claude.go`) and handoff TakeOver/HandOff (`wsagent/handoff.go`). The
  daemon only observes compaction levers and `compact_boundary`
  (`wsagent/backend.go`, `claude.go`).

## Decisions

Confirmed. Decisions 4-8 were queued and acknowledged by the owner on
2026-11-17 ("32..37=ok").

**Scope boundary.** The catalog ticket grades compaction per harness and routes
the existing `/compact` command through the catalog; this ticket owns
`ws/compact`, the default-method policy (31)=a, and the nudge/restore.

1. **Scope placement (28)=a.** The nudge (1) and state restore (A) are stage-2
   children of the epic.
2. **Nudge and restore apply to child agents too (owner 18:39).** Not only
   leads.
3. **Default compaction method (31)=a.** For hosted sessions on every harness
   the default is `ws/compact`: summarize in our format (today's Pi extension
   compaction format), open a NEW harness session, inherit the ws session key,
   and inject the summary as its first prompt. Native harness compaction is only
   the fallback when the session cannot write the summary itself.
4. **(33), owner ack 2026-11-17: (28)=a's single "hosted prompt assembly" child
   is split into two tickets:** prompt assembly (2, 3, B, C;
   `261117-feat-ws-hosted-prompt-assembly`) and compaction lifecycle (1, A,
   ws/compact; this ticket). Rejected: one ticket, because ws/compact is a
   separate runtime mechanism (session replacement, segment chain, record-shape
   compatibility tier) with different verification.
5. **(34), owner ack 2026-11-17: ws/compact runs only at an idle turn
   boundary.** Rejected: running it mid-turn, because in-flight harness tool
   calls and harness-side background shells die with the old session
   (daemon-owned ws/exec jobs survive).
6. **(35), owner ack 2026-11-17: one ws session owns a chain of harness-session
   segments** (resume and holder reattach target the newest segment; the
   dashboard stitches transcripts). Rejected: minting a new ws session per
   compaction, because agenda, whiteboard and child lineage are keyed by the ws
   key, and inheriting it is the point.
7. **(36), owner ack 2026-11-17: when the session cannot write the lever prose
   (context overflow), fall back to native compaction where the catalog grades
   it native, otherwise a cheap headless summarizer session over the
   transcript.** Rejected: a daemon-side model API call, because the daemon
   holds no model credentials.
8. **(37), owner ack 2026-11-17: the summary format is the Pi lever prose
   sections plus daemon-assembled fixed sections.** Rejected: a new format,
   because the existing Pi format is proven on Pi.

## Constraints

- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If playbooks, skills, `agents-plugin/rsrc/`
  or `agents-plugin-wsflow/` text is touched, also read
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; no per-event O(N) rereads; interactive paths never wait on
  file I/O; judged by measured syscalls/s on the owner's PC. The nudge's turn
  and token observation must not add per-event file writes; segment-chain
  records are appended, not rewritten. Related, not edited from here:
  `261117-research-ws-daemon-file-touch-map`.
- Shipped playbooks stay harness-neutral and must not name this repository's
  specifics; the Claude CLI tier is frozen (its existing lever and `lead-revive`
  path are untouched).
- Identity, authority, lineage and mailbox delivery ownership are preserved
  across compaction: the new segment inherits the ws key and never gains
  authority.

## Compatibility

The segment chain changes the session record shape (one ws key now maps to an
ordered list of harness-session segments). The compatibility tier must be
settled before this ticket reaches `ready/`, and preferably at T0-T2, with the
verdict citing source lines for which version writes and reads the session
record and whether reopening with the new version recovers it, per
`ai-docs/manuals/release-compatibility.md`. A T3 boundary needs the owner's
explicit approval; T4 is never shipped. The verdict is not decided here.

## Phases

Plausible breakdown for phase planning; phase text is refined at `ready/`
promotion. Phase 2 and Phase 3 need the compatibility tier settled first.

### Phase 1: Daemon compaction nudge from turn and token observation

The daemon observes turn and token usage (it already reads usage figures) and
nudges a session, lead or child, to compact at a safe point (idle turn
boundary, see (34)). The nudge replaces the plugin-side advisory for hosted
sessions only.

Deferred: ws/compact itself; state restore.

Verification: Go tests for threshold crossing, safe-point-only
delivery, child sessions nudged, and no nudge mid-turn; no per-event file write
(measured syscalls/s).

### Phase 2: Session segment chain and ws/compact

Build `ws/compact` per (31)=a: the session writes the summary in the Pi format
(37), the daemon opens a new harness session as a new segment under
the same ws key, injects the summary as its first prompt, and retargets
resume/holder reattach to the newest segment. Native compaction is the fallback
when the session cannot write the summary (36). Applies to leads and
children. Records the compatibility verdict.

Verification: fixture-driven tests per harness available at that
time (Claude first): new segment created, ws key inherited, agenda/whiteboard/
lineage intact, children still reachable, resume after daemon restart lands on
the newest segment, fallback path on overflow; a recorded end-to-end hosted
compaction of a lead and of a child.

### Phase 3: State restore bundle

After compaction and after a daemon restart, the daemon injects a restore
bundle: ws key, agenda, ticket/plan, children, exec jobs. This replaces the
`lead-revive` ritual for hosted sessions and applies to children as well as
leads. Depends on Phase 2.

Verification: tests that a compacted session and a
restart-recovered session each receive the bundle with every listed field, with
no `lead-revive` call; CLI-tier `lead-revive` unchanged.
