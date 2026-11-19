---
title: "감사 정책과 하네스 공통 이벤트 봉투(envelope)"
parent: 261119-epic-ws-conversation-audit
related:
  261119-feat-ws-audit-capture-harness-adapters: "consumer: maps each harness onto this envelope"
  261119-feat-ws-audit-sqlite-store: "consumer: persists this envelope"
  261114-feat-ws-conversation-audit-event-store: "superseded source of (545), (546), (595), (598) and the envelope-level parts of (599)"
---

# Audit policy and common event envelope

## Background

First child of `261119-epic-ws-conversation-audit` ((1331)=a, 2026-11-19).
It fixes what the audit captures, for which sessions, and the single
harness-neutral record every later child produces, stores, serves and shows.
It carries the policy and envelope decisions of the superseded
`261114-feat-ws-conversation-audit-event-store`.

Verified findings (codex.large read-only research, 2026-11-19):

- All four harnesses (Claude, Codex, agy, Pi) already flow through the same
  `wsagent` event path (`agents-plugin-tool/internal/wsagent/stream.go:35-38,169-199`),
  so one envelope fed from that path covers every hosted harness.
- Tool call IDs and full args/results exist on that path but are reduced to
  previews for `wslive` display (`stream.go:436-463,535-577`;
  `wslive/toolsummary.go:9-15`). The envelope must be built before that
  reduction (child 2 owns where).

## Decisions

1. **The daemon owns the audit record ((545)=a, 2026-11-14).** The daemon
   keeps tool inputs/outputs and hidden harness events in its own store, not
   in the harness's own transcript file. The capture breadth is re-asked as
   (1730), given the 2026-11-19 findings. (545)=a stands until then.
2. **Capture is what the harness JSON stream delivers, through one
   harness-neutral schema ((546)=a).** No API proxy to capture the system
   prompt. Every harness maps onto the same kinds. Rejected: per-harness
   schemas.
3. **Compatibility tier T0.** The envelope is new data. It carries its own
   schema version (`v`) from day one, so later changes have a tier to raise.
4. **Envelope ((595), carried; `session` added).**
   `{v, id, seq, session, at, harness, kind, subkind?, turn?, call_id?,
   parent_id?, agent_id?, model_seen, in?, out?, meta?, raw?, redactions?}`.
   - `session` is new relative to (595). The old design implied the session
     from a per-session directory. The store layout is now open (1735), so
     the record names its session explicitly (the session's reply ID).
   - `kind` is one of `user`, `assistant_text`, `thinking`, `tool_call`,
     `tool_result`, `hook`, `system_reminder`, `system`, `compaction`
     (summary text in `out`), `subagent`, `other`.
   - An unknown harness line becomes `other`, with the line kept verbatim in
     `raw` (after redaction, child 3).
   - `call_id` is the harness tool-call ID. It links `tool_call`,
     `tool_result`, attached hooks/reminders and subagent events (`parent_id`).
     It is the same ID the ring carries under (601) (child 2).
   - `at` uses the same clock and precision as ring entry stamps (UTC,
     nanoseconds), so the UI can place hidden events between ring rows
     ((599) additive field).
   - `seen: {in?: <bytes>, out?: <bytes>}` on the full envelope when the
     harness delivered less to the model than was stored (for example,
     Claude's `tool_result` content vs the full `tool_use_result`). An absent
     `seen` means fully seen or unknown. This is per body, separate from
     `model_seen` ((599) additive field).
   - A thinking event delivered only as a signature has
     `meta.signature_only: true` and an empty `out` ((599) additive field).
   - `redactions: [{field, offset, rule, length}]` ((596); child 3 fills it).
5. **Model-seen cut ((598)).** Each event records `model_seen`: true while it
   is in the model's current context. A compaction event marks every earlier
   event false and records itself as the cut. Without a boundary, the cut is
   the conversation start. Rejected: a token-count estimate, which the stream
   does not give reliably. The per-harness boundary sources and the agy case
   (1736) belong to child 2.

## Open Decision Queue

Last ID: (1731)

(1730) [open] [policy] What the audit captures: tool calls only, tool calls plus hidden harness events, or every raw stream line.
- Context: (545)=a (2026-11-14) asked for every tool input/output and every hidden event (hooks, reminders, thinking, the compaction summary, system events, subagent traffic), and the approved mockup v2 shows hidden-event rows with kind chips. The 2026-11-19 research found Claude tool-input/thinking deltas and agy thought chunks are dropped by the holder before the daemon (`wsagent/holder.go:139-141,228-246`), so hidden events need the pre-filter capture point (1732) and add volume.
- Alternative a: tool calls only (`tool_call`, `tool_result`, with linkage). Smallest store, no pre-filter tap needed for content the mapper already sees; the mockup's hidden-event rows and chips (child 5 Phase 2) lose their data.
- Alternative b: tool calls plus hidden events as assembled envelopes (thinking, hooks, reminders, compaction summary, system, subagent), `raw` kept only for `other`. Matches (545)=a and the mockup.
- Alternative c: every raw stream line verbatim, plus the mapped envelopes. Most complete; stores partial delta fragments whose content an assembled event may already carry (per-harness, unverified), at the highest volume and redaction surface.
> Recommend b: it is what (545)=a and the approved mockup already assume, and c's extra bytes are mostly stream fragments with the largest secret surface; a would strand child 5's Phase 2.

(1731) [open] [policy] Which sessions are audited.
- Context: all four hosted harnesses share the `wsagent` path. The parent epic is hosted-first ((01)=a) and freezes the Claude CLI plugin ((04)=a: keep current features, add none). Subagent traffic was previously in scope as the `subagent` kind, and daemon-owned `ws/agent.*` children are hosted sessions of their own.
- Alternative a: hosted lead sessions only.
- Alternative b: every daemon-hosted session (leads and `ws/agent.*` children).
- Alternative c: b plus non-hosted CLI-tier sessions (Claude CLI plugin, Pi extension).
- Alternative d: b, but off by default and enabled per session or workspace by the owner.
> Recommend b: one capture path covers it without per-tier work, children are where delegated tool calls actually run, and c adds a feature to a tier (04)=a froze.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`.
- The envelope type is harness-neutral: no harness-specific field outside
  `meta` and `raw`.
- Point at code by search, not coordinates, in the implementation; the
  coordinates above are research evidence.

## Phases

### Phase 1: Envelope type, kinds and capture policy gate

After (1730) and (1731) are answered: define the envelope (Decision 4) with
its schema version and kind set, the model-seen cut rule (Decision 5) as a
pure function over an event sequence, and one policy gate that answers
"capture this event for this session?" from the settled scope and session
set. Provide the sink interface the capture child writes to and the store
child implements, so children 2 and 3 can proceed in parallel.

Verification: unit tests for envelope JSON round-trip with the schema
version present; an unknown line becomes `other` with `raw`; the cut marks
earlier events false and falls back to the conversation start; the policy
gate admits exactly the settled kinds and session classes.
