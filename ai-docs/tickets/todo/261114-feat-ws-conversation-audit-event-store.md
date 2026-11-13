---
title: "Conversation audit: the daemon stores every harness stream event of a conversation and serves a read API for the audit view"
---

# Conversation audit event store

## Background

The owner wants to audit what a hosted conversation actually did and saw:
every tool call with its full input and output, and the events a harness
streams but the dashboard hides today (hooks, system reminders, thinking, the
compaction summary, system events, subagent traffic). The UI is an audit
panel, mockup v2 in the board topic `conversation-audit`
(`shared/docs/conversation-audit-mockup.html` under the project's ws cache).
The owner approved it with "550=ok", adding that in the wide view a click dims
everything except the conversation pane and the floating window.

Work split (EXEC and UI/UX EXEC, 2026-11-14): this ticket is the backend,
meaning the daemon-side store and the read API. UI/UX EXEC drafts the UI
ticket, blocked-by this ticket's API phase.

What exists today (Explore evidence, 2026-11-14, paths under
`agents-plugin-tool/internal/`):

- The Claude stream-json mapper (`wsagent/claude.go`, `mapLine` /
  `mapEvents`) keeps only assistant `text` and `tool_use` blocks.
  - `thinking` and other block types fall through the block switch.
  - Tool results keep only id, error flag and timing (`toolResults`, ~:1255).
  - Hooks, system reminders and the compaction summary are not recognized;
    `compact_boundary` becomes a bare `KindCompact` with its trigger.
  - Subagent lines (`parent_tool_use_id`) go to `subagentReply`.
- The Pi mapper is `wsagent/pi.go` (event switch ~:726-850). Codex is not
  parsed.
- Persistence is the per-session ring (`wslive/ring.go`):
  `<daemon dir>/sessions/<replyID>/ring/`, one JSON file per entry, named by
  stamp.
  - Caps: 2000 entries on disk, 64 KiB per text.
  - `ToolSummary` keeps no result content; argument previews are capped at
    200 runes.
  - GC: `trimRing` past 2000, and `PruneRegistrations` removes a session dir
    7 days after last seen (`wsdashboard/registry.go:332-370`).
- `wsredact.Secrets` masks to `***`, keeping no rule name or length, and runs
  only on the 200-rune argument preview (`wslive/state.go:1201-1214`).
- Read surface: `GET /s/<reply-id>/history?before=<ring name>&n=<count>`
  serves rendered HTML, cursor-paged (`wsdashboard/pages.go:1261-1300`).
  There is no route serving tool input or output, and no search over
  conversation content.
- The ring has no schema version, and its readers ignore unknown fields
  (`ring.go` ReadRingPage).

## Decisions

1. **The daemon keeps every tool input/output and every hidden harness event
   in its own store ((545)=a).** The store is the daemon's, not the harness's
   own transcript file.
2. **Capture is what the harness JSON stream delivers, through a
   harness-neutral event schema ((546)=a).** No API proxy to capture the
   system prompt. Future harnesses map onto the same schema.
3. **Compatibility tier: T0.** It adds a new store beside the ring, new
   routes and no change to existing routes, ring entries, MCP tools or
   tracker events. The new store carries its own `schema_version` from day
   one, so later changes have a tier to raise.

## Open Decision Queue

Last ID: (601). MAIN's board IDs are (a) = (594), (b) = (595), (c) = (596),
(d) = (597), (e) = (598), (f) = (599), (g) = (601), and (600) is the
owner's group-ack check, covering (594)-(599) and (601).

- (a) [open] [technical] The audit store is a separate append-only
  per-session log, `sessions/<replyID>/audit/`: JSONL segment files with a
  size cap per segment, plus a small sidecar index (event id -> segment,
  offset). This is chosen over widening ring entries: the ring is a capped
  UI view (2000 entries, 64 KiB text) whose trimming would discard audit
  data, and full tool outputs would bloat every history page read.
- (b) [open] [technical] Envelope: `{v, id, seq, at, harness, kind,
  subkind?, turn?, call_id?, parent_id?, agent_id?, model_seen, in?, out?,
  meta?, raw?, redactions?}`.
  - `kind` is one of `user`, `assistant_text`, `thinking`, `tool_call`,
    `tool_result`, `hook`, `system_reminder`, `system`, `compaction`
    (summary text in `out`), `subagent`, `other`.
  - An unknown harness line becomes `other`, with the line kept verbatim in
    `raw`.
  - Claude stream-json is mapped first. Pi follows, from `pi.go`'s existing
    events. Codex maps to the same kinds when a Codex mapper exists.
  - This is chosen over per-harness schemas, which (546)=a rules out.
- (c) [open] [technical] Redaction: before writing, `wsredact` runs over
  `in`, `out` and `raw`. A redaction keeps the rule name and the original
  length, never the original
  (`redactions: [{field, offset, rule, length}]`). This extends `wsredact`
  with a match-reporting entry point and keeps the `Secrets(s) string`
  signature for existing callers. It is chosen over storing unredacted
  data with a separate secure store, which is a new key-management
  surface.
- (d) [open] [technical] GC: the audit dir lives inside the session dir, so
  the existing 7-day `PruneRegistrations` removes it. A per-session byte cap
  (config knob, default 256 MiB) drops the oldest whole segments first. It
  is chosen over a time-only rule, because one runaway tool output could
  fill the disk within 7 days.
- (e) [open] [technical] "Model-seen cut point": each event records
  `model_seen` (true while it is in the model's current context). A
  compaction event marks every earlier event false and records itself as
  the cut. Claude's `compact_boundary` and Pi's
  `compaction_start`/`compaction_end` provide the boundary. Without a
  boundary, the cut is the conversation start. This is chosen over a
  token-count estimate, which the stream does not give reliably.
- (f) [open] [technical] Read API contract (the UI ticket's dependency). All
  routes are under the existing session prefix, return JSON, and require the
  same auth as `/s/<id>/history`:
  - `GET /s/<id>/audit/events?before=<event id>&after=<event id>&n=<count>&kinds=<k1,k2>`
    pages events oldest-first within the page. `n` defaults to 100, max 500.
    It returns `{events: [summary], before, after, end, cut}`, where
    `summary` is `{id, seq, at, kind, subkind, call_id, parent_id, agent_id,
    model_seen, title, in_bytes, out_bytes, attached: [event id]}` and
    `attached` lists the events tied to a call (its result, its hooks, its
    reminders, its subagent events).
  - `GET /s/<id>/audit/event/<event id>` returns the full envelope (`in`,
    `out`, `meta`, `raw`, `redactions`). A body past 8 MiB is served as a
    truncated head with `truncated: true` and a `range` follow-up
    (`?part=out&offset=&len=`).
  - `GET /s/<id>/audit/search?q=<text>&regex=0|1&case=0|1&kinds=&n=` searches
    the stored `in` and `out` of the whole conversation, newest first. It
    returns `{hits: [{event_id, kind, field, line_no, line}], next}`. The
    search is a streaming scan of the segment files, so no index is built.
  - `GET /s/<id>/audit/events?call=<call_id>` returns that call's summary
    plus its attached events, so a click on a ring history row (which holds
    the ring's tool id) finds the audit events.
  - Fields the audit mockup v2 needs (UI/UX EXEC, 2026-11-14, all additive):
    - A `tool_call` summary carries `duration_ms`, which is null while the
      call is running.
    - A `tool_call` summary carries `ctx_after`: the context size of the
      request that follows the call (input + cache_read + cache_creation,
      from the next assistant message's usage). Parallel calls answered in
      one request each get the same shared value.
    - The full envelope carries `seen: {in?: <bytes>, out?: <bytes>}` when
      the harness delivered less to the model than was stored (for example,
      Claude's tool_result content vs the full `tool_use_result`). An absent
      `seen` means fully seen or unknown. This is per body, separate from
      the compaction-level `model_seen`.
    - Each search hit also carries `call_id` and `parent_id`.
    - A thinking event whose content was not delivered, only its signature,
      has `meta.signature_only: true` and an empty `out`.
    - `at` uses the same clock and precision as ring entry stamps (UTC,
      nanoseconds), so the UI can place hidden events between ring rows.
  - This is chosen over an MCP tool surface, which can come later as an
    additive T0 tool, because the UI is the consumer.
- (g) [open] [technical] The ring's per-call record gains an optional call
  id, so a ring history row can open its audit events. Today no ring type
  carries one (`wslive/toolsummary.go` `ToolBatch`, `ToolCallArgs`;
  `wslive/ring.go` `RingEntry`), and the history DOM only has `data-entry`.
  - `ToolCallArgs` gains `id` (omitempty), the same harness tool-call id the
    audit store records as `call_id`. A batch whose calls carry ids writes
    `CallArgs` even when no call had an argument preview; the
    `BatchCallCap` cap is unchanged.
  - The Claude mapping fills it in Phase 1; Pi fills it where its events
    carry a call id. Ring entries written before the change have no id, and
    the UI keeps today's pill for those, with no time-window matching.
  - Decision 3 still holds: this is an additive optional field (T0), and
    ring readers ignore unknown fields. The constraint that the ring's caps
    and `/history` behaviour do not change still holds; emitting the id as
    `data-call` in the template belongs to the UI ticket.
  - This is chosen over a time-window join between ring rows and audit
    events, which is ambiguous for parallel calls and for calls that start
    in the same millisecond.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`; `internal/mcp/` ->
  `ai-docs/manuals/ws-mcp.md`.
- The existing ring, its caps and `/history` behaviour do not change.
- Writing must not block the stream mapper. Append off the hot path, and on a
  write error log it and drop the event rather than stall the session.
- Store files use mode 0600 and dirs 0700, like the ring.

## Phases

### Phase 1: Envelope, Claude mapping, store, redaction, GC

The store and writer (queue items (a), (c), (d)), the envelope (b), Claude
stream-json mapping of every line kind including thinking, hooks,
reminders, the compaction summary and subagent lines, and the model-seen
cut (e), and the ring call id (g). Pi mapping too, if it fits the same
pass.

Verification: unit tests over recorded stream-json fixtures, one per kind;
redaction tests (rule name and length kept, original absent); GC tests (the
byte cap drops whole oldest segments; the session prune removes the dir); a
ring test that a Claude tools row's calls carry the same ids as the audit
store's `tool_call` events, and that an older ring entry without ids still
reads.

### Phase 2: Read API

The three routes of (f), with paging, kinds filter, attached links, cut, and
search. Verification: route tests on a fixture store, including paging
boundaries, a truncated large body, and a search hit's line number.
