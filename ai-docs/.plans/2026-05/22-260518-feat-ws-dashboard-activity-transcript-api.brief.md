# Brief: 260518-feat-ws-dashboard-activity-transcript-api

## Intent

Expand Activity Console transcript parsing behind daemon-owned adapters so
backend-native transcript records can become bounded `TranscriptBlock` values
without exposing cache/session paths or native record formats to the browser.
Start with fixture-backed Codex native session JSONL evidence only.

## Scope Boundary

Selected scope: `Phase 1: Expand Activity Console transcript sources and live
transcript behavior`.

In scope:

- Add a transcript source adapter/resolver boundary behind the existing
  `ActivityTranscript` and `TranscriptBlock` contracts.
- Add fixture-backed Codex native session JSONL parsing using the session file
  format documented in `ai-docs/ref/codex-integration.md`.
- Normalize supported Codex session events into bounded transcript blocks.
- Degrade malformed or unsupported native lines/records without failing the
  entire selected activity transcript when possible.
- Preserve private-field redaction and existing named-agent `output.md`
  fallback behavior.
- Decide from implementation evidence whether block-level transcript events add
  clear UX value; default to feed-level `transcriptUpdated` plus bounded
  backfill unless a material win is proven.

Out of scope:

- Claude/Gemini native transcript parsing unless fixture-backed stable formats
  already exist in the repo during implementation.
- Exec transcript source integration, blocked on
  `260513-feat-async-exec-output-reader`.
- Browser transcript UI redesign or Activity Console controls.
- Live backend invocation to generate fixtures.

## Caller-Visible Contract

Browser callers still request selected activity transcripts by opaque workRoot
and activity ids through the existing Activity Console transcript route. The
response remains an `ActivityTranscript` with bounded `TranscriptBlock` values.

Supported native Codex session records may produce blocks for assistant
messages, task/status boundaries, function/tool calls, function/tool outputs,
and degraded unsupported records. The route must not expose backend session
paths, cache paths, host paths, pids, session ids, stdout/stderr paths, stream
paths, native transcript paths, raw JSONL lines, or backend-native record
objects.

Malformed native records degrade individual blocks or source status where
possible. Missing or unreadable native session files fall back to existing
available sources or return explicit unavailable/degraded transcript state
without breaking the whole Activity Feed.

## Contract Instructions

- Extend the daemon Activity transcript implementation in
  `ws-dashboard/crates/daemon/src/work_root_activity.rs` without creating a
  second browser-visible transcript API.
- Keep public shapes in `ws-dashboard/crates/core/src/activity.rs` stable unless
  tests prove an additive field is necessary. Prefer existing `TranscriptBlock`
  `renderKind`, `title`, `text`, `data`, and `degraded` fields.
- Introduce an internal adapter/resolver boundary that can support Codex now and
  Claude/Gemini later without hardcoding browser-visible backend paths.
- Use `ai-docs/ref/codex-integration.md` as the stable fixture source for Codex
  native session file JSONL. The relevant session-file line shape is
  `{\"timestamp\":\"...\",\"type\":\"...\",\"payload\":{...}}`; supported
  records include `event_msg` task/agent message payloads and `response_item`
  function call/output payloads.
- If live code needs to locate a native Codex session file, do so only from
  daemon-private metadata already available in named-agent state. Do not expose
  resolved paths or require the browser to provide them.
- Preserve the existing named-agent `output.md` backfill path. Codex-native
  transcript parsing should improve transcript detail when a verified native
  session file is available, not regress current output-only transcripts.
- Do not parse Claude, Gemini, or exec transcript sources without fixture-backed
  evidence in this implementation.
- Do not add selected transcript event streams unless implementation shows a
  concrete visible benefit beyond feed-level `transcriptUpdated` plus bounded
  backfill.

## Integration Test Instructions

Required boundary type: daemon transcript parser/resolver tests plus route tests
for selected Activity Console transcript backfill.

Coverage must prove:

- Codex native session JSONL fixture lines from `ai-docs/ref/codex-integration.md`
  parse into bounded `TranscriptBlock` values.
- Assistant messages, function/tool calls, function/tool outputs, and status or
  task boundary records get source-neutral render kinds/titles/text/data.
- Missing or unreadable native session files degrade or fall back without route
  failure.
- Malformed native JSONL lines produce degraded block/source status behavior
  without exposing raw private content.
- The transcript route still redacts host paths, cache paths, native transcript
  paths, pids, session ids, stdout/stderr paths, stream paths, and backend-native
  records.
- Existing `output.md` transcript backfill and cursor/limit bounds still work.
- If block-level transcript events are added, cursor behavior and redaction are
  tested; if not added, tests should show feed-level invalidation/backfill is
  sufficient for this slice.

Run at minimum:

```text
cd ws-dashboard && cargo test -p ws-dashboard-core activity
cd ws-dashboard && cargo test -p ws-dashboard-daemon work_root_activity
```

Add targeted daemon parser/route test filters to the completion report.

## Implementation Strategy Decisions

- Fixture-backed Codex native session JSONL is the only native transcript source
  candidate for this ticket.
- Claude and Gemini remain deferred without fixture-backed stable formats.
- Exec transcript source integration remains deferred until async exec output
  reader exists.
- Block-level selected transcript SSE is optional and should be skipped unless
  it clearly improves the visible transcript UX over existing
  `transcriptUpdated` invalidation plus selected backfill.
- Redaction and graceful degradation are more important than preserving every
  backend-native field.

## Rejected Alternatives

- Browser-visible native session paths or raw JSONL records: rejected because
  the daemon owns all source resolution and normalization.
- Live Codex invocation to create fixtures: rejected because tests must be
  stable and runnable without backend credentials or network state.
- Codex-only public contracts: rejected because the resolver boundary must allow
  later Claude/Gemini adapters.
- Implementing exec transcript integration now: rejected because that source is
  blocked by `260513-feat-async-exec-output-reader`.

## Approach

- Read the existing transcript route and named-agent projection code.
- Add fixture-backed Codex session JSONL parser tests before or with the parser.
- Add a private transcript resolver/adapter layer that tries native Codex
  session parsing only when safe evidence exists, then falls back to current
  `output.md` backfill behavior.
- Keep cursor/limit and byte bounds explicit when native records produce blocks.
- Extend route tests for redaction, malformed records, missing native files, and
  fallback preservation.

## Constraints

- Do not expose private paths, session ids, pids, raw backend records, or native
  transcript file paths in API responses, diagnostics, DOM-visible fixtures, or
  logs.
- Do not block Axum async workers while reading native transcript files; follow
  existing blocking-read patterns.
- Preserve source-neutral Activity Console vocabulary.
- Keep frontend behavior unchanged unless an additive route type change makes a
  frontend type/test update necessary.

## Out of scope

- Claude/Gemini transcript parsing without fixtures.
- Exec transcript source support.
- Backend-native transcript event stream unless clearly justified.
- Activity Console UI redesign.
- Agent controls.

## Details

Codex native session fixture basis from `ai-docs/ref/codex-integration.md`:

```json
{"timestamp":"<ISO>","type":"<event_type>","payload":{...}}
```

Relevant examples:

- `event_msg` / `task_started`
- `event_msg` / `task_complete`
- `event_msg` / `agent_message`
- `response_item` / `function_call`
- `response_item` / `function_call_output`

Map these to existing `TranscriptBlock` render kinds where possible:

- assistant/message text -> assistant-like block
- function/tool call -> tool-call summary block with bounded detail
- function/tool output -> tool-result/output block with bounded detail
- task/status records -> status block
- malformed/unsupported records -> degraded status/error block

## Verification Contract

Implementation is complete only when:

- Codex native fixture parsing is covered by deterministic tests.
- Transcript route tests prove redaction, degraded malformed records, fallback,
  and existing backfill behavior.
- Required Rust test commands pass.
- Review relay is clean across correctness, fit, and test partitions.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md`:
  `{#260522-ws-dashboard-activity-console-transcript-expansion}`,
  `{#260521-ws-dashboard-activity-console-read-model}`,
  `{#260521-ws-dashboard-activity-console-watch-stream}`,
  `{#260521-ws-dashboard-activity-console-ui-shell}`.
- [Must] `ai-docs/spec/named-agent-runtime.md`:
  `{#260505-codex-agent-session-jsonl-handling}`,
  `{#260505-codex-jsonl-trailing-noise-tolerance}`,
  `{#260505-named-agent-registry-state-layout}`,
  `{#260505-agent-readiness-result-split}`.
- [Must] `ai-docs/ref/codex-integration.md` - fixture source for Codex native
  session file JSONL shape and event types.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard activity
  transcript route, redaction, blocking-read, and test coupling rules.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - wsagent cache/session
  layout and Codex JSONL parsing pitfalls.
- [Must] `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-transcript-api.md`
  - selected phase scope and verification checklist.
- [Must] `ai-docs/tickets/todo/260518-epic-ws-dashboard-activity-console.md`
  - cross-child read-only/source-neutral boundaries.
- [Maybe] `ai-docs/tickets/todo/260513-feat-async-exec-output-reader.md` -
  future exec source, blocked for this implementation.
