# Survey: 260518-feat-ws-dashboard-activity-transcript-api

## Reusable Components
- `ws-dashboard/crates/core/src/activity.rs#L125-L150` — `ActivityTranscript`/`TranscriptBlock`: stable public transcript contract with cursor, timestamp, render kind, title, text, data, and degraded fields.
- `ws-dashboard/crates/core/src/activity.rs#L194-L325` — core serde/redaction test: verifies camelCase transcript fields and absence of private snake_case/session/pid/stdout fields.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L124-L148` — `WorkRootActivityProjector::named_agent_transcript`: existing async boundary that already pushes transcript work into `spawn_blocking`.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L184-L212` — `work_root_activity_transcript`: existing owner-auth protected selected transcript route handler; it should stay the single browser-visible API.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L856-L962` — `named_agent_transcript_blocking`: current selected named-agent transcript implementation, source metadata construction, output fallback, cursor/limit slicing, and degraded/unavailable status handling.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L999-L1019` — `transcript_blocks_from_output`: current `output.md` to bounded markdown `TranscriptBlock` conversion.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L631-L704` — named-agent projection metadata reader: has backend/harness/model/session presence and diagnostics that native transcript resolution can reuse without leaking fields.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L843-L854` — transcript limit/cursor normalization helpers: existing finite backfill controls for selected transcript reads.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L813-L818` — `bounded`: reusable text cap used for backend error and output text; native block text/details should preserve this bound or a similarly explicit one.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1682-L1698` — fixture helpers for agent metadata, current call state, and `output.md` content.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1738-L1758` — transcript route test helper for selected activity transcript requests.
- `ai-docs/ref/codex-integration.md#L139-L157` — documented Codex native session JSONL envelope and relevant `event_msg`/`response_item` payload types.

## Existing Patterns
- Read-only route placement: see `ws-dashboard/crates/daemon/src/router.rs#L84-L96` — activity snapshot, transcript, and event routes are all protected dashboard routes.
- Private-field redaction assertions: see `ws-dashboard/crates/daemon/tests/routes.rs#L2026-L2043` and `ws-dashboard/crates/daemon/tests/routes.rs#L2194-L2207` — route tests explicitly search response text for root/cache/session/output/private path leaks.
- Output fallback route coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L2100-L2210` — current tests cover auth, unknown workRoot/activity, `output.md` cursor/limit, `hasMore`, and response redaction.
- Empty/unavailable degradation: see `ws-dashboard/crates/daemon/tests/routes.rs#L2292-L2350` — missing output and malformed `agent.json` produce explicit transcript states instead of route failure.
- Agent cache layout fixture seeding: see `ws-dashboard/crates/daemon/tests/routes.rs#L1811-L1846` — tests derive wsstate agent directories through `resolve_work_root_agents_dir` and seed realistic named-agent files.
- Daemon-private wsagent layout: see `agents-plugin-tool/internal/wsagent/agent.go#L323-L340` and `agents-plugin-tool/internal/wsagent/agent.go#L2041-L2069` — runtime stores `agent.json`, `current/*`, `output.md`, and `events.jsonl` under wsstate-derived agent directories.
- Codex stdout JSONL parser precedent: see `agents-plugin-tool/internal/wsagent/codex.go#L200-L249` — existing runner consumes one JSON event per line, accepts large lines, and tolerates trailing non-JSON only after session id and final text.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1282-L1307` — `AgentMetadata`: deserializes backend, harness, model, session id, status, last call time, and last output path from `agent.json`; public projection collapses session id to a presence flag.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1309-L1325` — `CurrentCallState`: current-call deserialization intentionally omits pid/stdout/stderr/session paths and keeps bounded public status/timing/error fields.
- `agents-plugin-tool/internal/wsagent/agent.go#L610-L629` — Codex session id is persisted into `agent.json` and current-call state when the runner observes it.
- `agents-plugin-tool/internal/wsagent/agent.go#L671-L707` — completed calls write final plain-text result to `output.md`, preserving the existing fallback source.
- `ai-docs/ref/codex-integration.md#L143-L154` — native Codex session JSONL uses `{timestamp,type,payload}`; payload variants include task boundaries, assistant messages, function calls, and function outputs.
- `ai-docs/ref/codex-integration.md#L156-L162` — Codex turn grouping and session-file discovery are thread-id based under `~/.codex/sessions/.../rollout-*<thread_id>.jsonl`.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L607-L622` — modification time detection already includes `current/runtime.jsonl`, `current/stdout`, and `current/stderr`, but not external `~/.codex/sessions` files.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1329-L1445` — daemon unit-test module exists for private helpers; parser/resolver unit tests can live near private parsing code without route setup.

## Constraints
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L102-L121` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L124-L148` — activity projection and transcript reads must stay off Axum async workers via blocking tasks.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L893-L961` — missing/unreadable `output.md` currently returns explicit `empty`, `unavailable`, or `degraded` transcript states; native parsing must not regress this fallback path.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L936-L960` — cursor semantics are currently line-offset based over the selected source's block vector; native blocks need finite slicing semantics compatible with existing route callers.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L751-L762` — feed transcript availability currently depends only on local `output.md` presence; native-only availability would require keeping feed item transcript metadata coherent.
- `ai-docs/ref/codex-integration.md#L139-L162` — native Codex session files are outside wsstate cache under the user's Codex home; locating them from a thread/session id is daemon-private and must not surface paths.
- `agents-plugin-tool/internal/wsagent/agent.go#L323-L340` — `agent.json` has only `session_id`, not a native transcript path; a resolver cannot expect a stored direct session-file path today.

## Risk Signals
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1282-L1307` — Possible doc/comment risk: the comment says raw session id is not deserialized, but `session_id` is currently deserialized privately; implementers should treat it as daemon-private resolver input, not public response data.
- `ai-docs/ref/codex-integration.md#L143-L154` — Possible fixture risk: the reference documents field paths but does not provide full concrete `payload` JSON examples for every supported native record; tests may need synthetic fixtures derived from the table.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L751-L762` — Possible feed/route mismatch risk: an agent with no `output.md` but an available native Codex session would still advertise transcript `empty` unless feed availability logic is extended.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L856-L962` — Possible reuse risk: current function interleaves source resolution, projection, output read, block conversion, slicing, and status assembly; adding adapters inside it could duplicate glue instead of creating a reusable source boundary.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L999-L1019` — Possible parser-quality risk: existing output parser is line-to-markdown only; native Codex events need richer render kinds and malformed-line degradation without leaking raw JSON.
- `ai-docs/ref/codex-integration.md#L161-L162` — Possible privacy/performance risk: thread-id search under `~/.codex/sessions` can touch host-private paths and many files; responses and diagnostics must stay bounded and path-free.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2194-L2207` — Possible redaction-test risk: existing forbidden checks cover `output.md` and cache paths, but native transcript path, raw JSONL, thread id/session id, stdout/stderr, and malformed raw records need additional assertions.
- `CHANGELOG.md#L703-L729` — Possible format-history risk: prior Codex tail code was wrong when it used `--json` stdout shape instead of native session-file `{timestamp,type,payload}` envelopes; avoid reusing stdout parser assumptions for session files.

## Opinion
- The public transcript contract is already suitable for native records; the implementation pressure is internal source resolution, parser degradation, and keeping feed availability consistent.
- No research escalation is needed for Claude/Gemini because the brief explicitly defers them without fixtures.
