---
title: "감사 수집 지점(필터 이전)과 하네스별 어댑터 (Claude → Codex → agy → Pi)"
parent: 261119-epic-ws-conversation-audit
blocked-by: 261119-feat-ws-audit-policy-event-envelope
related:
  261119-feat-ws-audit-sqlite-store: "sink: persists what this child captures; may proceed in parallel"
  261119-feat-ws-dashboard-audit-view: "consumer of the ring call ID (601), Phase 1"
  261117-feat-ws-harness-capability-catalog: "per-harness@version fixture policy; compaction capability sources"
  261114-feat-ws-conversation-audit-event-store: "superseded source of (598) boundary sources, (601), and the non-blocking writer constraint"
---

# Audit capture point and per-harness adapters

## Background

Second child of `261119-epic-ws-conversation-audit` ((1331)=a, 2026-11-19).
It turns each hosted harness's stream into the envelope of
`261119-feat-ws-audit-policy-event-envelope`, from a point that still sees
what the display path filters out, and it carries the ring call ID that
joins history rows to audit events.

Verified findings (codex.large read-only research, 2026-11-19):

- All four harnesses (Claude, Codex, agy, Pi) flow through the same `wsagent`
  event path (`agents-plugin-tool/internal/wsagent/stream.go:35-38,169-199`):
  one line mapper per backend, fed line by line.
- Tool call IDs and full args/results exist briefly and are dropped when
  converted for `wslive` display; only previews survive
  (`stream.go:436-463,535-577`; `wslive/toolsummary.go:9-15`). The batch
  tracker holds each call's `id`, but its wire form carries name, clocks,
  failure and an argument preview only.
- The holder filters Claude tool-input/thinking deltas and agy thought chunks
  before they reach the daemon (`wsagent/holder.go:139-141,228-246`). The
  launch passes a per-backend `RingSkip` pattern. The holder keeps matching
  stdout lines out of its ring, so they take no sequence and reach no client.
  Full capture must tap before that filter.
- The superseded ticket's adapter order (Claude -> Pi -> Codex, "Codex is not
  parsed") is stale: all four adapters exist now.

## Decisions

1. **Adapter order: Claude -> Codex -> agy -> Pi** (parent epic harness order
   (25)). It replaces the stale Claude -> Pi -> Codex order. Each adapter maps
   onto the common envelope ((546)=a). Nothing harness-specific leaks outside
   `meta`/`raw`.
2. **Capture taps before the display reduction and before the holder
   ring-skip filter** (2026-11-19 finding). Where exactly is (1342).
3. **Ring call ID ((601), carried).** The ring's per-call record gains an
   optional call ID, so a history row can open its audit events.
   - Today no ring type carries one (`wslive` `ToolCallArgs` holds `Name` and
     `ArgsPreview` only), and the history DOM only has `data-entry`.
   - `ToolCallArgs` gains `id` (omitempty), the same harness tool-call ID the
     audit store records as `call_id`. A batch whose calls carry IDs writes
     `CallArgs` even when no call had an argument preview. The `BatchCallCap`
     cap is unchanged.
   - Every adapter fills it where its events carry a call ID. Ring entries
     written before the change have no ID. The UI keeps today's pill for
     those ((602), child 5), with no time-window matching.
   - Additive optional field (T0). Ring readers ignore unknown fields. The
     ring's caps and `/history` behaviour do not change. Emitting the ID as
     `data-call` in the template belongs to child 5.
   - Rejected: a time-window join between ring rows and audit events, which
     is ambiguous for parallel calls and calls that start in the same
     millisecond.
4. **Model-seen cut boundary sources ((598), carried).** Claude's
   `compact_boundary` and Pi's `compaction_start`/`compaction_end` provide the
   boundary. Codex and agy sources are confirmed per adapter phase. agy's
   case is (1346).

## Open Decision Queue

Last ID: (1346)

(1342) [open] [policy] Where audit capture taps the stream, given the holder drops ring-skipped lines before the daemon.
- Context: Claude and agy launches pass a `RingSkip` pattern, so the holder keeps Claude tool-input/thinking/signature delta lines and agy `agent_thought_chunk` lines out of its ring; they never reach the daemon (`wsagent/holder.go:139-141,228-246`). The skip exists to keep partial-delta lines out of the holder's bounded output ring, which serves replay after a daemon restart. The parent epic requires the holder protocol to stay versioned, with the new daemon speaking the old holder protocol.
- Alternative a: holder-side tee. The holder forwards skipped lines to the daemon on a separate, unsequenced audit channel and still keeps them out of its ring. Holder protocol change (versioned). Skipped lines produced while no daemon is attached are lost to the audit.
- Alternative b: disable `RingSkip` for audited sessions. Every line reaches the daemon, which tees it to the audit adapter before mapping. No protocol change; partial deltas consume the holder ring's replay budget.
- Alternative c: daemon-side tap after the filter only. No holder change; content that exists only in skipped lines (agy thought chunks, possibly Claude thinking) is not captured.
> Recommend a: it captures what (1340) b/c need without shrinking the replay ring, and the daemon-downtime loss matches the audit's existing drop-on-error rule; c cannot satisfy hidden-event capture for agy.

(1346) [open] [policy] How the model-seen cut handles agy compaction, which is inferred rather than reported as a boundary event.
- Context: (598) takes the cut from an explicit harness boundary and rejects token-count estimates. The capability catalog records agy compaction as `session/prompt /compact` at idle boundaries "with descriptor usage-drop and prompt-response completion detection", so the daemon infers it from a usage drop instead of reading a boundary line. Whether agy reports its own automatic compaction at all is unverified.
- Alternative a: record the inferred compaction as the cut, with `meta.estimated: true` on the compaction event, so the UI can label it.
- Alternative b: record no cut for agy; every event stays `model_seen: true` (cut at conversation start), and the compaction is stored as a plain `system` event.
- Alternative c: record the inferred compaction event but mark later-than-cut certainty as unknown (`model_seen` omitted) for agy events before it.
> Recommend a: the inference is observed from ws's own `/compact` completion, not a token estimate, and labelling it keeps (598)'s rule honest while still showing what fell out of context; b shows provably stale context as seen.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`.
- Capture must not block the stream mapper or the holder reader. Hand off
  off the hot path. On a sink error, log and drop the event rather than
  stall the session (carried).
- The holder protocol stays versioned; the new daemon speaks the old holder
  protocol (parent epic Compatibility).
- (601) preserves old ring entry readability, the ring's caps and `/history`
  behaviour.
- Release-gate file-touch rule (parent epic): no per-event O(N) rereads, no
  rewrites; interactive paths never wait on file I/O.
- Per-version recorded stream fixtures follow the capability catalog's
  `harness@version` policy.

## Phases

### Phase 1: Capture tap, Claude adapter and ring call ID

After (1342) is answered and the envelope child has landed: implement the
settled capture tap, the Claude stream-json adapter for every kind the policy
admits (thinking, hooks, reminders, the compaction summary and subagent lines
included), the Claude `compact_boundary` cut, and the ring call ID (601) for
Claude. Write into the envelope child's sink interface. An in-memory sink is
enough to verify; persistence is child 3.

Verification: unit tests over recorded Claude stream-json fixtures, one per
kind, including a line the holder's `RingSkip` matches (it is captured per
(1342)); a ring test that a Claude tools row's calls carry the same IDs as
the captured `tool_call` events, and an older ring entry without IDs still
reads; a slow or failing sink never delays line delivery.

### Phase 2: Codex adapter

Depends on Phase 1. Map the Codex app-server stream onto the envelope,
including its compaction boundary if the stream carries one (confirm from a
recorded fixture), and fill the ring call ID.

Verification: per-kind fixtures for the supported Codex version range; ring
call ID parity as in Phase 1.

### Phase 3: agy adapter

Depends on Phase 2. Map the agy ACP stream onto the envelope, including
thought chunks per (1340)/(1342), the compaction handling settled by (1346),
and the ring call ID where agy carries a tool-call ID.

Verification: per-kind fixtures, including a thought chunk and a
`/compact` completion; ring call ID parity.

### Phase 4: Pi adapter

Depends on Phase 3. Map Pi's events onto the envelope, including
`compaction_start`/`compaction_end` as the cut, and fill the ring call ID
where Pi's events carry one.

Verification: per-kind fixtures, including a compaction pair; ring call ID
parity.
