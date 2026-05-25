# Survey: 25-260525-feat-ws-dashboard-sqlite-agent-activity-source-phase-3

## Reusable Components
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L51-L99` — `read_activity_agent_records`: short-lived read-only SQLite adapter for current `agent_defs`; already selects most Phase 3 current metadata and soft-degrades through callers.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L101-L158` — `read_activity_agent_instance_records`: read-only SQLite adapter for retained `agent_instances`; carries current Phase 2 cleanup filtering inputs but not all retention timestamp fields yet.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L532-L570` — `activity_item_versions`: central daemon-private item-version map consumed by SSE diffing; currently payload-mtime-only for current and historical rows.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L665-L690` — `agent_record_modified_at`: existing payload mtime aggregation for `output.md`, current call files, runtime/stdout/stderr, and native Codex transcript files.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L754-L817` — `registry_historical_agent_items` and `historical_agent_instance_projection`: retained-instance projection/filter path used by feed, transcript resolution, and versions.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4402-L4607` — Activity SQLite fixture helpers: `write_agent_metadata`, `upsert_agent_def`, and `upsert_agent_instance` seed `state.sqlite` plus payload dirs for route/SSE tests.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4722-L4769` — SSE test helpers: `read_activity_sse_events` and `fetch_work_root_activity_events` parse named `event: activity` frames through route auth.

## Existing Patterns
- SSE diff path: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L362-L407` — compares `ActivityItem`, `item_versions`, and transcript cursors to enqueue `itemUpserted`, `itemRemoved`, and `transcriptUpdated`.
- Blocking projection boundary: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L99-L180` — route-facing async calls clone config and run filesystem/SQLite projection work inside `tokio::task::spawn_blocking`.
- Recent-limit selection: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L601-L649` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L754-L793` — current roles and historical items both pre-sort/truncate before final feed ordering.
- Retained-instance filtering: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L819-L844` — hides current/protected/deleted/tombstone/internal rows and requires useful payload, diagnostic, last-output, or pinned signal.
- Public Activity shape stability: see `ws-dashboard/crates/core/src/activity.rs#L8-L58` and `ws-dashboard/crates/core/src/activity.rs#L88-L170` — event and feed structs define the browser JSON vocabulary that Phase 3 should not extend.
- Existing payload-only SSE coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L4020-L4153` — mutates current-call and `output.md` payload files, then expects item, transcript, removal, and recreate events.
- Existing native transcript mutation coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L4155-L4245` — mutates a Codex session JSONL file and expects `transcriptUpdated` without leaking session/private paths.
- Existing retained-instance route/privacy coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L5107-L5515` — covers current counts vs historical items, protected cleanup filtering, transcript dispatch, and private marker deny-list.
- Existing locked/incompatible registry soft-degrade coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L6411-L6518` — confirms unreadable registry returns healthy empty Activity without leaking `state.sqlite` or cache markers.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L6-L49` — `ActivityRegistryAgentRecord` and `ActivityRegistryAgentInstanceRecord`: structs to extend with registry-only version/recency fields while keeping `state_path` private.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L462-L500` — `project_blocking`: builds current-role projections, summary, historical items, final ordering, and feed cursor.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L698-L752` — `registry_named_agent_projection`: converts registry rows plus payload readers into `NamedAgentProjection`; visible timing comes from `last_call_at`/`last_seen_at`/`updated_at` fallback.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L952-L984` — `activity_item_ordering` and `recent_value`: final feed ordering uses visible item timing after recent-limit preselection.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1085-L1109` — `resolve_transcript_record`: dispatches current `agent:` ids through `agent_defs` and historical `agent-instance:` ids through filtered `agent_instances`.
- `agents-plugin-tool/internal/wsstore/store.go#L65-L95` — `AgentDefinition`: authoritative runtime metadata inventory includes timestamps, last output, cleanup/retention fields, cleanup error, and pinned.
- `agents-plugin-tool/internal/wsstore/store.go#L1210-L1265` — SQLite schema: `agent_defs` and `agent_instances` timestamp/cleanup/retention columns available to dashboard read-only queries.
- `agents-plugin-tool/internal/wsstore/store.go#L510-L575` — `UpsertAgentDefinition`: writes `updated_at` on role/instance updates and retires old instances with retention timestamps, so registry-only changes are real freshness signals.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L549-L576` — SQLite registry is named-agent metadata authority; browser route/event shapes stay stable; missing/locked/unavailable/incompatible registry degrades instead of failing or leaking cache paths.
- `ai-docs/spec/ws-web-dashboard/index.md#L635-L665` — Activity SSE uses named `event: activity`, source-neutral event vocabulary, scoped workRoot subscriptions, and no backend/cache/session/path leaks.
- `ai-docs/mental-model/ws-web-dashboard.md#L149-L153` — Activity changes must preserve read-only projection, compatibility `agents`, source-neutral `items`, and privacy boundaries.
- `ai-docs/mental-model/named-agent-runtime.md#L108-L112` — SQLite is authoritative for role/instance metadata and path indexes, but current call, events, diagnostics, and output bytes remain file-backed payloads.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L160-L173` — payload `state_path` is accepted only when relative and normal; invalid paths produce no payload dir.
- `ws-dashboard/crates/daemon/tests/routes.rs#L5460-L5511` — retained-instance tests deny leaking raw instance ids, state paths, sessions, payload names, cleanup errors, output paths, and `state.sqlite`.

## Risk Signals
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L543-L568` — Possible contract risk: `activity_item_versions` ignores registry fields, so a status/timestamp/cleanup-only SQLite update may not emit SSE diffs when visible item timing/status also does not change.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L612-L639` — Possible contract risk: current `recentLimit` preselection sorts only by payload/file mtimes, so registry-recent rows with absent or old payload dirs can be omitted.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L777-L792` — Possible contract risk: retained historical recent-limit selection also uses only payload mtime, so registry-recent retained rows can lose to older payload mtimes.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4463-L4607` — Possible test risk: fixture helpers currently cannot set `created_at`, `updated_at`, `last_seen_at`, retention timestamps, or `cleanup_attempted_at`; Phase 3 tests may need helper expansion to avoid SQL one-offs.
- `agents-plugin-tool/internal/wsstore/store.go#L400-L407` — Possible compatibility risk: retention/cleanup columns are migration-added; dashboard queries that unconditionally select newly added columns preserve current behavior only if incompatible older schemas continue soft-degrading as intended.

## Opinion
- The brief matches current code reality: Phase 1/2 code already centralizes registry reads and SSE version comparison, so the main survey uncertainty is exact private version-key representation, not public API shape.
- No research escalation needed; implementation can stay inside the two daemon source files plus route tests named by the brief.
