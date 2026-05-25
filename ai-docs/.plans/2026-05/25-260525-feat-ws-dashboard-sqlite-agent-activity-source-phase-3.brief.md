# Brief: 260525-feat-ws-dashboard-sqlite-agent-activity-source Phase 3

## Intent

Make WorkRoot Activity freshness SQLite-aware so registry-only metadata changes are visible to Activity snapshots, SSE polling diffs, and recent refresh limits even when payload file mtimes do not change. This completes the SQLite-backed Activity source ticket without changing the browser route shapes or frontend behavior.

## Scope Boundary

Implement only Phase 3 of `260525-feat-ws-dashboard-sqlite-agent-activity-source`: registry-aware versioning, registry-aware recent ordering, and corresponding route/SSE tests. Phase 1 current-role SQLite projection and Phase 2 retained-instance items are already implemented; preserve them. Do not add new frontend features, new event types, main-session Activity sources, or remote-server forwarding behavior.

## Caller-Visible Contract

Activity snapshots and SSE polling must observe changes caused only by SQLite registry metadata. Current role rows from `agent_defs` and retained historical rows from useful `agent_instances` should produce changed Activity item versions when relevant registry fields change, even if `output.md`, `current/state.json`, native transcript files, and payload directories are unchanged. Existing Activity event streams must continue to emit the current event vocabulary (`itemUpserted`, `itemRemoved`, `transcriptUpdated`, `snapshotInvalidated`, `modeChanged`, `heartbeat`) through named `event: activity` SSE frames. `recentLimit` should select rows by registry-aware recency rather than only payload filesystem mtime.

## Contract Instructions

Extend `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs` so the read-only registry records expose the SQLite metadata needed for versioning and recency. For current `agent_defs`, include registry timestamps such as `created_at`, `updated_at`, `last_seen_at`, `last_call_at`, and `last_output_path`. For retained `agent_instances`, include the same activity timestamps plus relevant cleanup/retention metadata such as `cleanup_state`, `cleanup_attempted_at`, `cleanup_error`, `retention_eligible_at`, `retention_checked_at`, `retention_next_check_at`, and `pinned` when present in the schema.

Extend `ws-dashboard/crates/daemon/src/work_root_activity.rs` so item versions combine registry metadata and payload mtimes. Registry-only status/timestamp/cleanup/retention changes must change the relevant item version. Payload-only changes to output/current-call/runtime/stdout/stderr/native Codex transcript files must still change item versions and transcript update behavior.

Update recent ordering for current role projections and retained historical items to use a registry-aware recency key that considers registry timestamps and payload mtimes. Do not order or filter recent rows only by payload directory mtimes. Preserve compatibility semantics: `ActivityFeed.agents` and summary counts remain based on current `agent_defs`; retained instances affect only `items`.

Keep missing, locked, unavailable, or incompatible registry state soft-degraded as today. Avoid holding SQLite connections while reading payload files. Do not write, migrate, repair, or import registry state from the dashboard daemon.

Do not change frontend code or public JSON/event field names unless a test fixture requires unchanged API semantics.

## Integration Test Instructions

Extend `ws-dashboard/crates/daemon/tests/routes.rs`.

Add SSE coverage proving a registry-only current-role metadata change, such as `agent_defs.status` plus `updated_at` or `last_seen_at`, emits an `itemUpserted` for the current `agent:<agentKey>` item without touching payload files.

Add SSE coverage proving payload-only transcript changes still emit `transcriptUpdated` for current role items. Existing `output.md` and Codex native transcript tests may be extended if they continue to prove this after the versioning change.

Add retained-instance coverage proving registry-only appearance/removal or cleanup transition of a useful retained `agent_instances` row results in the expected historical item `itemUpserted` or `itemRemoved`, while current agent counts remain unchanged.

Add `recentLimit` coverage proving registry-recent rows are selected even when payload mtimes are absent or older than less-recent registry rows. Cover current `agent_defs`; cover retained `agent_instances` if the existing API/test shape makes that practical without adding frontend-visible behavior.

Preserve privacy assertions: registry timestamps, state paths, raw instance ids, sessions, cache paths, output paths, cleanup errors, and database names must not leak into route or event bodies.

## Implementation Strategy Decisions

Use SQLite metadata as part of the daemon-private version/recency calculation, not as new browser-visible fields. Keep browser-visible Activity item timing fields derived from the existing normalized model.

Prefer a shared internal helper for registry-aware versions/recency so current role items, retained historical items, and SSE snapshots use the same logic. If the implementation needs a string version, it may compose or hash a bounded set of registry metadata plus payload mtime data; the exact representation is daemon-private.

Recent-limit selection should happen before projection as today, but the sort key must include registry metadata. Watch snapshots should use the same item id set and filtering semantics as the feed so hidden current/protected/deleted/payload-useless retained instances do not create phantom versions.

## Rejected Alternatives

Do not implement native filesystem watchers or durable event logs; the current stream remains a bounded polling fallback.

Do not make `recentLimit` frontend-owned or add frontend filtering to compensate for daemon recency.

Do not add new Activity event types or change public response shapes.

Do not scan legacy `agent.json` or use payload directory discovery as metadata authority.

Do not broaden this slice into main-session Activity, async exec Activity, or remote Activity forwarding.

## Approach

- Extend registry adapter record structs with registry timestamp and cleanup/retention fields needed for recency/version keys.
- Add a registry-aware version/recency helper for current `agent_defs` rows and retained `agent_instances` rows.
- Use the helper in `registry_named_agents`, `registry_historical_agent_items`, and `activity_item_versions`.
- Keep payload mtime checks for output/current-call/runtime/stdout/stderr/native Codex transcripts in the version/recency key.
- Add route/SSE tests for registry-only current updates, payload-only transcript updates, retained instance item removal/appearance, and registry-aware `recentLimit`.

## Constraints

- Keep all registry access read-only and short-lived.
- Keep all Git/registry/file/transcript reads off Axum async workers.
- Preserve current id semantics: current `agent:<agentKey>` ids and historical `agent-instance:<token>` ids.
- Preserve retained-instance filtering from Phase 2 before versions or recent ordering are emitted.
- Preserve privacy redaction and bounded diagnostics.
- Avoid broad formatting of unrelated daemon files.

## Out of scope

- Frontend changes.
- New watcher implementation or durable replay cursors.
- Registry schema migrations or writes.
- Main-session, exec-job, or remote-server Activity sources.
- Changes to named-agent retention cleanup policy.

## Details

Current code paths to update include `activity_item_versions`, `registry_named_agents`, `registry_historical_agent_items`, and the registry adapter structs in `work_root_activity_registry.rs`. Existing SSE diffing already compares `ActivityItem` plus `item_versions`; after Phase 3, registry-only changes should flow through that existing diff path rather than adding a parallel event mechanism.

If parsing registry timestamp strings is needed, treat malformed or empty timestamps as oldest/absent rather than degrading the whole feed. The version key may use raw registry timestamp strings as daemon-private version components if that is simpler and stable enough for equality comparisons.

## Verification Contract

Required:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon work_root_activity -- --nocapture`
- `cargo check --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`
- `rustfmt --edition 2021 --check ws-dashboard/crates/daemon/src/work_root_activity.rs ws-dashboard/crates/daemon/src/work_root_activity_registry.rs ws-dashboard/crates/daemon/tests/routes.rs`

Before final handoff, also run:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - Activity projection, stream, transcript, privacy, and test rules.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - SQLite registry and retained instance runtime model.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - SQLite Activity source, read model, watch stream, live UX, and transcript contracts.
- [Must] `ws-dashboard/crates/daemon/src/work_root_activity.rs` - Activity projection, versioning, recent-limit, transcript, and SSE diff code.
- [Must] `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs` - read-only SQLite adapter.
- [Must] `ws-dashboard/crates/daemon/tests/routes.rs` - Activity route, SSE, transcript, and fixture helpers.
- [Maybe] `agents-plugin-tool/internal/wsstore/store.go` - authoritative wsstore schema and timestamp/cleanup fields.
