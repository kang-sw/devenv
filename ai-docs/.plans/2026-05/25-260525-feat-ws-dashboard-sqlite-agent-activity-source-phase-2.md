# Survey: 25-260525-feat-ws-dashboard-sqlite-agent-activity-source-phase-2

## Reusable Components
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L31-L79` — `read_activity_agent_records`: existing read-only SQLite `agent_defs` adapter with short busy timeout and missing-db empty behavior; the retained-instance adapter can mirror this access style.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L24-L29` — `ActivityRegistryAgentRecord::payload_dir`: maps registry `state_path` to `<state_dir>/agents/<safe-relative>`; relevant for instance `state_path` payload resolution.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L81-L94` — `safe_relative_payload_path`: rejects empty, absolute, parent/current-dir, and other non-normal components before joining payload paths.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L437-L461` — `resolve_work_root_agents_dir` / `resolve_work_root_state_dir`: wsstate-compatible Git workRoot-to-state-dir derivation used by both runtime projection and route fixtures.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L675-L729` — `registry_named_agent_projection`: projects one registry row plus payload dir into the current named-agent view, including status mapping, current call reads, detail hints, diagnostics, output availability, and Codex native availability.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L731-L792` — `named_agent_activity_item`: maps a named-agent projection to source-neutral `ActivityItem` with transcript availability and private metadata exclusion.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L915-L1071` — `named_agent_transcript_blocking`: existing transcript reader that resolves metadata to Codex native JSONL or `output.md`, returns bounded empty/unavailable/degraded states, and avoids exposing file paths.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1727-L1757` — `read_current_call`: file-backed `current/state.json` adapter producing bounded active/terminal/error fields.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1823-L1842` — `resolve_codex_session_file`: daemon-private native Codex transcript lookup gated by Codex backend/harness and safe session-id shape.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1953-L1969` — `short_hash` / local SHA-256: existing dependency-free deterministic hashing helper suitable for opaque historical item tokens.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4462-L4528` — `upsert_agent_def`: existing SQLite fixture helper for `agent_defs`; a sibling helper can seed `agent_instances` in the same state.sqlite fixture.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4543-L4569` — payload fixture helpers: write `current/state.json` and `output.md` under an arbitrary registry `state_path` without `agent.json` discovery.

## Existing Patterns
- Read-only soft-degrade projection: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L578-L587` — registry read failures collapse to empty healthy projection rather than route failure.
- Current role counts stay compatibility-only: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L475-L499` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L559-L574` — `agents` and `summary` derive only from current projections, while `items` is independently assembled and sorted.
- Watch/event diffing is item-id keyed: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L502-L546` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L363-L400` — new historical items need stable ids plus item-version/transcript-cursor participation if they should not regress polling updates.
- Activity id validation currently accepts only `agent:<key>`: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L207-L237` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L856-L867` — historical ids need a separate parser/resolver rather than overloading role keys.
- Route privacy assertions are explicit deny-lists: see `ws-dashboard/crates/daemon/tests/routes.rs#L4909-L4926`, `ws-dashboard/crates/daemon/tests/routes.rs#L5008-L5021`, and `ws-dashboard/crates/daemon/tests/routes.rs#L5208-L5221` — add instance-id/state-path/session/database/output-path strings to response/transcript checks.
- Locked/incompatible registry coverage exists for current rows: see `ws-dashboard/crates/daemon/tests/routes.rs#L4933-L5025` for state-path payload resolution and existing activity tests around empty/missing registry in the same file.

## Relevant Interfaces
- `ws-dashboard/crates/core/src/activity.rs#L8-L23` — `ActivityFeed`: public response shape; `items` can grow while `agents` remains compatibility named-agent rows.
- `ws-dashboard/crates/core/src/activity.rs#L76-L86` — `WorkRootActivitySummary`: count fields currently describe current named-agent rows only.
- `ws-dashboard/crates/core/src/activity.rs#L88-L123` — `ActivityItem`, `ActivitySourceDisplay`, and `ActivityTranscriptAvailability`: historical rows must fit this source-neutral item shape without frontend schema changes.
- `ws-dashboard/crates/core/src/activity.rs#L125-L138` — `ActivityTranscript`: transcript routes return bounded normalized blocks and source metadata keyed by public `activity_id`.
- `ws-dashboard/crates/core/src/activity.rs#L152-L186` — `NamedAgentActivityView` / `NamedAgentCallActivityView`: compatibility current-agent row shape; historical instances should not be added here unless the public contract changes.
- `agents-plugin-tool/internal/wsstore/store.go#L1230-L1264` — `agent_instances` schema: `instance_id`, `agent_key`, `public_name`, `state_path`, backend/model/session/status/timing, `last_output_path`, cleanup metadata, and `pinned` are available in SQLite.
- `agents-plugin-tool/internal/wsstore/store.go#L552-L564` and `agents-plugin-tool/internal/wsstore/store.go#L624-L646` — current role replacement/deletion retires old instance rows while advancing/removing `agent_defs`; retained instances can share role keys with current rows.
- `agents-plugin-tool/internal/wsstore/store.go#L927-L938` — cleanup candidates exclude current/active/running/queued/recovery/deleted cleanup states, status `running`, pinned rows, and current role pointers.
- `ai-docs/spec/ws-web-dashboard/index.md#L549-L570` — SQLite Activity Source contract: `agent_defs` remains current metadata/count authority; retained instance rows may add historical `ActivityItem`s without increasing current counts.
- `ai-docs/mental-model/named-agent-runtime.md#L27-L36` — role/instance split: role pointer selects current `StatePath`; historical payload directories remain retention-owned and cleanup is SQLite-candidate based.
- `ai-docs/mental-model/ws-web-dashboard.md#L77-L80` — Activity Console ownership/privacy: read-only projection, no `agent.json` scans, payloads through registry `state_path`, and transcript updates via selected backfill.

## Constraints
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L123-L131` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L149-L160` — projection and transcript reads already run under `spawn_blocking`; new SQLite/file reads should stay inside these blocking paths.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L39-L45` — registry opens with `SQLITE_OPEN_READ_ONLY`, `NO_MUTEX`, and a 50ms busy timeout; retained-instance reads should preserve read-only/soft-degrade behavior.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L699-L704` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L719-L720` — browser-visible projection collapses private paths/session ids to presence/detail hints only.
- `ai-docs/spec/ws-web-dashboard/index.md#L531-L547` — transcript blocks must be normalized/bounded and never expose host/cache/backend session/process/stdout/stderr/native transcript paths.
- `ai-docs/mental-model/ws-web-dashboard.md#L149-L149` — Activity changes should test Git key derivation, missing/locked/incompatible registries, `state_path` resolution, unknown ids, transcript bounds, native degradation/fallback, feed-level updates, and private-field redaction.
- `agents-plugin-tool/internal/wsstore/store.go#L927-L938` — exact runtime cleanup states include `current`, `active`, `running`, `queued`, `recovery`, `cleanup_deleted`; this may refine the brief's suggested hide/keep filter list.

## Risk Signals
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L524-L546` — Possible test/refresh risk: `activity_item_versions` currently reads only `agent_defs`, so historical instance payload/native changes would lack item-version entries unless extended or consciously left Phase-3-limited.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L915-L950` — Possible reuse risk: transcript resolution currently requires `agent_key` from `agent:` ids and searches `agent_defs`; historical `agent-instance:` ids need a resolver that returns instance metadata plus `state_path` without falling back to current role rows.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L675-L729` — Possible contract risk: `registry_named_agent_projection` consumes `ActivityRegistryAgentRecord` whose `agent_key` becomes `NamedAgentActivityView.agent_id`; reusing it directly for historical instances could accidentally put opaque instance ids into compatibility row semantics or expose raw role/instance identity.
- `agents-plugin-tool/internal/wsstore/store.go#L927-L938` — Possible contract/filter risk: runtime cleanup candidate logic treats `recovery` as protected and `cleanup_deleted` as excluded, while the brief suggests hiding deleted/tombstone/internal cleanup rows; exact dashboard filter behavior may need lead/planner judgment if undocumented states appear.
- `agents-plugin-tool/internal/wsstore/store.go#L526-L532` — Possible privacy risk: default `instance_id` derives from `agent_key:state_path`; using raw or lightly encoded instance ids would expose role/path-ish registry identity and violate the opaque-token requirement.
- `ws-dashboard/crates/daemon/tests/routes.rs#L4430-L4459` — Possible fixture risk: legacy helper names still say `write_agent_metadata` but now seed SQLite rows; new tests should avoid accidentally creating `agent.json`-like assumptions or payload dirs named by role key.

## Opinion
- The codebase already has most projection/transcript primitives; the main unknown is not schema access but preserving the current-role vs historical-instance boundary while reusing named-agent projection logic.
- The brief is aligned with current specs, but the exact cleanup-state/tombstone vocabulary is only partially represented in source; if implementation discovers states beyond `current/active/running/queued/recovery/cleanup_deleted/retired/cleanup_failed`, escalate before inventing browser semantics.
