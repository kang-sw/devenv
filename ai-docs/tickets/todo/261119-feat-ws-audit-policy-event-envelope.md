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
   in the harness's own transcript file. How much it captures is settled by
   (1730)=b (Decision 6) and for which sessions by (1731)=b (Decision 7).
2. **Capture is what the harness JSON stream delivers, through one
   harness-neutral schema ((546)=a).** No API proxy to capture the system
   prompt. Every harness maps onto the same kinds. Rejected: per-harness
   schemas.
3. **Compatibility tier T0.** The envelope is new data. It carries its own
   schema version (`v`) from day one, so later changes have a tier to raise.
4. **Envelope ((595), carried; `session` added).**
   `{v, id, seq, session, at, harness, kind, subkind?, turn?, call_id?,
   parent_id?, agent_id?, model_seen, in?, out?, meta?, raw?, redactions?,
   seen?, stored_trunc?}`.
   - `session` is new relative to (595). The old design implied the session
     from a per-session directory. The record names its session explicitly
     (the session's reply ID), which keeps it self-describing for export and
     for a ws/agent.* child's own file, even though (1735)=a gives each
     session its own database file.
   - `kind` is one of `user`, `assistant_text`, `thinking`, `tool_call`,
     `tool_result`, `hook`, `system_reminder`, `system`, `compaction`
     (summary text in `out`), `subagent`, `other`.
   - An unknown harness line becomes `other`, with the line kept verbatim in
     `raw` (after redaction, child 3). `raw` is set only for `other`
     ((1730)=b); every mapped kind is an assembled event without `raw`.
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
   - `stored_trunc: {in?, out?}` marks a body the store kept only as head
     plus tail ((1737)=b, child 3). Per body it records the original byte
     length and the kept head and tail lengths. It is separate from `seen`
     (what the model received) and from the read API's response-level
     `truncated` (a body too large for one response). Absent means the body
     is stored whole. The final field shape is fixed in Phase 1.
   - A thinking event delivered only as a signature has
     `meta.signature_only: true` and an empty `out` ((599) additive field).
   - `redactions: [{field, offset, rule, length}]` ((596); child 3 fills it).
5. **Model-seen cut ((598)).** Each event records `model_seen`: true while it
   is in the model's current context. A compaction event marks every earlier
   event false and records itself as the cut. Without a boundary, the cut is
   the conversation start. Rejected: a token-count estimate, which the stream
   does not give reliably. The per-harness boundary sources and the agy case
   (1736) belong to child 2. For agy the cut is the inferred compaction with
   `meta.estimated: true` ((1736)=a, child 2).

6. **Capture scope: tool calls plus hidden events ((1730)=b, owner
   2026-11-19).** The audit captures tool calls (`tool_call`, `tool_result`,
   with linkage) and the hidden harness events (thinking, hooks, system
   reminders, the compaction summary, system events, subagent traffic) as
   assembled envelopes. `raw` is kept only for `other`. This is what (545)=a
   and the approved mockup v2 assume.
   - Rejected: a, tool calls only. It strands the dashboard child's
     hidden-event rows and kind chips (its Phase 2) and contradicts (545)=a.
   - Rejected: c, every raw stream line verbatim plus the mapped envelopes.
     It stores partial delta fragments whose content an assembled event may
     already carry (per harness, unverified), at the highest volume and the
     widest redaction surface.
   - Knock-ons: hidden events need the pre-filter tap, which (1732)=a settles
     in child 2. The tapped partial-delta lines (Claude tool-input and
     thinking deltas, agy thought chunks) are input to the adapter's
     assembly, not stored as lines. The dashboard child keeps its hidden-event
     rows and chips.
7. **Audited sessions: every daemon-hosted session ((1731)=b, owner
   2026-11-19).** Leads and `ws/agent.*` children. A child's tool calls land
   in that child's own audit record under its own session ID, not in its
   lead's. The `subagent` kind stays for traffic a harness reports about its
   own subagents. There is no per-session or per-workspace switch.
   - Rejected: a, hosted lead sessions only. Delegated tool calls run in the
     children, so the audit would miss most of them.
   - Rejected: c, b plus non-hosted CLI-tier sessions (Claude CLI plugin, Pi
     extension). It adds a feature to a tier the parent epic froze ((04)=a:
     keep current features, add none).
   - Rejected: d, b but off by default and enabled per session or workspace.
     It makes the audit absent exactly when it is needed afterwards, and adds
     a settings surface.
   - Knock-ons: the policy gate keys on "daemon-hosted" and nothing else.
     Capture, the per-session store file ((1735)=a) and the per-session read
     routes all follow the session ID, so a child's audit is read on the
     child's own session.

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

Define the envelope (Decision 4) with its schema version and kind set, the
model-seen cut rule (Decision 5) as a pure function over an event sequence,
and one policy gate that answers "capture this event for this session?":
every Decision 4 kind (Decision 6) for every daemon-hosted session
(Decision 7), with no per-session switch. Provide the sink interface the capture child writes to and the store
child implements, so children 2 and 3 can proceed in parallel.

Verification: unit tests for envelope JSON round-trip with the schema
version present; an unknown line becomes `other` with `raw`; the cut marks
earlier events false and falls back to the conversation start; a mapped kind
never carries `raw`; the policy gate admits a lead and a `ws/agent.*` child
and refuses a non-hosted (CLI-tier) session, for every kind.
