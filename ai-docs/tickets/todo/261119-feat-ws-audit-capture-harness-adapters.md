---
title: "감사 수집 지점(필터 이전)과 하네스별 어댑터 (Claude → Codex → agy → Pi)"
parent: 261119-epic-ws-conversation-audit
blocked-by: 261119-feat-ws-audit-policy-event-envelope
related:
  261119-feat-ws-audit-ring-store: "sink: persists what this child captures; may proceed in parallel"
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
   ring-skip filter** (2026-11-19 finding). Where exactly is Decision 5
   ((1732)=a).
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
   case is Decision 6 ((1736)=a).
5. **Capture point: a holder-side tee on a separate audit channel ((1732)=a,
   owner 2026-11-19).** The holder forwards each line its `RingSkip` pattern
   skips to the daemon on a separate, unsequenced, versioned audit channel,
   and still keeps those lines out of its own ring. The daemon's capture
   adapter reads them there, ahead of the display reduction.
   - Lines the holder skips while no daemon is attached are lost to the
     audit. This is accepted: it matches the audit's drop-on-error rule, and
     the holder ring stays small for replay. The audit channel has no replay
     and takes no sequence number.
   - Rejected: b, disabling `RingSkip` for audited sessions. Partial deltas
     would consume the holder ring's replay budget.
   - Rejected: c, a daemon-side tap after the filter only. Content that
     exists only in skipped lines (agy thought chunks, possibly Claude
     thinking) would never be captured, so (1730)=b could not be met for agy.
   - Compatibility statement (parent epic holder-version rule: the holder
     protocol is versioned and the new daemon speaks the old holder
     protocol). The audit channel is an additive, version-negotiated
     capability, T0. It must never refuse an old peer, because a hello or
     protocol bump that refuses old connections is T3
     (`ai-docs/manuals/release-compatibility.md`), which needs a minor bump
     and the owner's explicit approval, and neither is requested here.
     - New daemon, old holder: the daemon negotiates down and receives no
       audit channel. Sessions on that holder are captured from the
       post-filter stream only, so skipped-only content (Claude thinking and
       tool-input deltas, agy thought chunks) is missing from the audit.
       Nothing else changes. The old holder keeps running unmodified.
     - Old daemon, new holder: the holder sends audit lines only after the
       daemon negotiated the capability, so the old daemon receives nothing
       it cannot read.
     - The holder is PC-wide and outlives a daemon restart, so a holder older
       than the daemon is an expected, ordinary state, not an error.
   - Ordering: the two channels interleave by arrival, not by a shared
     sequence. The holder stamps each tee'd line with a UTC nanosecond time at
     read, and the daemon assigns the envelope `seq` at capture. Phase 1
     verifies that assembled events (a thinking block, a tool-input) keep
     their order against the sequenced lines of the same turn.
   - Capture still never blocks the holder reader or the stream mapper (see
     Constraints). A slow or absent daemon drops audit lines, never delays
     the session.
6. **agy compaction is recorded as the cut, labelled as an estimate
   ((1736)=a, owner 2026-11-19).** The daemon infers agy compaction (the
   capability catalog records it as `session/prompt /compact` at idle
   boundaries, detected by a descriptor usage drop and prompt-response
   completion), so the inferred compaction is recorded as the model-seen cut,
   with `meta.estimated: true` on the compaction event so the UI can label it.
   - Rejected: b, no cut for agy (every event stays `model_seen: true` and the
     compaction is a plain `system` event). It shows provably stale context
     as seen.
   - Rejected: c, record the compaction but omit `model_seen` on earlier agy
     events. It leaves the cut unusable for agy without gaining accuracy over
     a, which already labels the estimate.
   - The rule observes ws's own `/compact` completion and a usage drop, not a
     token-count estimate, so it does not reopen (598)'s rejection. Whether
     agy reports its own automatic compaction at all stays unverified and is
     confirmed from a recorded fixture in Phase 3. If it does not, only the
     compaction ws itself triggered gets a cut.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`.
- Capture must not block the stream mapper or the holder reader. Hand off
  off the hot path. On a sink error, log and drop the event rather than
  stall the session (carried).
- The holder protocol stays versioned; the new daemon speaks the old holder
  protocol (parent epic Compatibility). Decision 5's audit channel is
  negotiated, additive and T0.
- (601) preserves old ring entry readability, the ring's caps and `/history`
  behaviour.
- Release-gate file-touch rule (parent epic): no per-event O(N) rereads, no
  rewrites; interactive paths never wait on file I/O.
- Per-version recorded stream fixtures follow the capability catalog's
  `harness@version` policy.

## Phases

### Phase 1: Capture tap, Claude adapter and ring call ID

After the envelope child has landed: implement the holder-side tee and its
versioned audit channel (Decision 5), the daemon-side capture on that
channel, the Claude stream-json adapter for every kind the policy
admits (thinking, hooks, reminders, the compaction summary and subagent lines
included), the Claude `compact_boundary` cut, and the ring call ID (601) for
Claude. Write into the envelope child's sink interface. An in-memory sink is
enough to verify; persistence is child 3.

Verification: unit tests over recorded Claude stream-json fixtures, one per
kind, including a line the holder's `RingSkip` matches (it arrives over the audit
channel and is assembled, per (1732)=a, not stored as a raw line); a holder
protocol test that an old holder (no audit channel) still works against the
new daemon and a new holder sends no audit lines to an old daemon; a test
that lines skipped while no daemon is attached are dropped without stalling
the holder; ordering of assembled events against the sequenced lines of one
turn; a ring test that a Claude tools row's calls carry the same IDs as
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
thought chunks per (1730)=b and the Decision 5 tee, the inferred compaction as
the cut with `meta.estimated: true` (Decision 6),
and the ring call ID where agy carries a tool-call ID.

Verification: per-kind fixtures, including a thought chunk and a
`/compact` completion that yields the estimated cut (`meta.estimated: true`,
earlier events `model_seen: false`); ring call ID parity.

### Phase 4: Pi adapter

Depends on Phase 3. Map Pi's events onto the envelope, including
`compaction_start`/`compaction_end` as the cut, and fill the ring call ID
where Pi's events carry one.

Verification: per-kind fixtures, including a compaction pair; ring call ID
parity.
