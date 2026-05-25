# Brief: 260525-feat-ws-dashboard-sqlite-agent-activity-source Phase 1

## Intent

Replace the dashboard WorkRoot Activity current named-agent projection with a read-only SQLite registry source while preserving the existing browser route and JSON shapes.

## Scope Boundary

Implement only Phase 1 from `260525-feat-ws-dashboard-sqlite-agent-activity-source`: current named-agent role rows come from wsstore `agent_defs`, and file-backed payload readers use the registry `state_path`. Do not add retained `agent_instances` history items and do not make Activity SSE/versioning fully SQLite-aware beyond keeping existing behavior working.

## Caller-Visible Contract

`GET /api/dashboard/work-roots/{workRootId}/activity`, `/activity/events`, and `/activity/items/{activityId}/transcript` keep their response shapes. `ActivityFeed.agents`, `ActivityFeed.items` current named-agent entries, and `ActivityFeed.summary.total` are based on current role rows in `agent_defs`. Missing, locked, incompatible, or absent SQLite state degrades to an empty or partial projection without route failure or cache path exposure.

## Contract Instructions

Work in `ws-dashboard/crates/daemon/src/work_root_activity.rs` and route tests. Isolate wsstore schema reads in a small adapter module or contained section instead of spreading SQL throughout projection logic.

Resolve the workRoot wsstate key with the existing Git identity and cache layout logic. Read `<cache>/proj/<worktreeKey>/state.sqlite` read-only. Do not write, migrate, repair, or import from legacy `agent.json`.

Query current role rows from `agent_defs`; normalize only dashboard fields: opaque role id, public name, backend, harness, tier, model, effort, status, session presence, last call/seen/update timestamps, current `state_path`, and bounded diagnostics. Continue reading `current/state.json`, `output.md`, and Codex native transcript files from the payload directory resolved by `state_path`.

Do not scan `agents/*/agent.json` to discover current agents. Do not use `agent_instances` for visible history in Phase 1. Do not edit frontend source.

## Integration Test Instructions

Extend `ws-dashboard/crates/daemon/tests/routes.rs` Activity route coverage. Add a SQLite-backed fixture where `agent_defs` contains current rows and payload directories have no `agent.json`; assert current count, status counts, privacy-preserving session presence, transcript availability, and transcript route output. Add soft-degrade coverage for missing or incompatible registry state. Keep existing no-private-field assertions.

Run:

```sh
cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon work_root_activity -- --nocapture
```

Also run `cargo fmt --manifest-path ws-dashboard/Cargo.toml --check` or equivalent formatting verification after edits.

## Implementation Strategy Decisions

- Use SQLite as the named-agent metadata authority for current roles.
- Keep payload bytes file-backed; SQLite supplies only metadata and `state_path` indexes.
- Use short-lived read-only SQLite connections with a busy timeout.
- Preserve opaque activity ids for current role rows as `agent:<agent_key>`.
- Treat historical instance display as out of scope for Phase 1.

## Rejected Alternatives

- Legacy `agents/*/agent.json` scan or fallback: explicitly removed from dashboard Phase 1 discovery.
- Dashboard-side SQLite migration/import/repair: ws-mcp/wsagent owns compatibility and migration.
- Frontend shape changes: the ticket is backend-only.
- `agent_instances` history projection: Phase 2.

## Approach

- Add a registry adapter that resolves `state.sqlite`, opens it read-only, validates the needed schema softly, and returns current role metadata rows.
- Convert registry rows into the existing `NamedAgentProjection` path.
- Replace directory scan discovery with registry rows plus payload dirs from `state_path`.
- Update transcript lookup to resolve the activity role id through the registry before reading file-backed payloads.
- Adjust tests to seed SQLite `agent_defs` plus payload dirs without `agent.json`.

## Constraints

- No browser response may expose host paths, cache paths, SQLite paths, session ids, pids, stdout/stderr paths, `agent.json`, `state.sqlite`, or `current/state.json`.
- A missing database, missing table, unknown schema, busy/locked read, or absent rows must not crash the route.
- SQLite reads must be bounded and read-only.
- Keep `ActivityFeed.agents` and `summary.total` based on current `agent_defs` only.

## Out of scope

- Retained historical `agent_instances` Activity items.
- Historical instance transcript ids.
- Registry-aware SSE/recent versioning for metadata-only changes beyond preserving current polling behavior.
- Frontend code changes.

## Details

The wsstore schema defines `agent_defs(agent_key primary key, actor_id, public_name, state_path, schema_version, backend, harness, tier, model, effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, ...)`. Use `public_name` for the display name, `session_id` only to set `sessionPresent`, and `state_path` to locate `<cache>/proj/<worktreeKey>/agents/<state_path>`.

Status mapping should continue through the existing dashboard `agent_status` helper. Current call state still comes from `<payloadDir>/current/state.json`. Output availability still checks `<payloadDir>/output.md`. Codex native transcript resolution still uses daemon-private session id and configured `CODEX_HOME`/`codex_home`.

## Verification Contract

- New/updated route test proves SQLite `agent_defs` rows project without `agent.json` files.
- Transcript route resolves an activity id through SQLite `state_path`.
- Missing registry state returns an empty healthy projection.
- Incompatible registry state degrades without route failure.
- Activity responses keep existing JSON shape and privacy checks.

## References

- [Must] `ai-docs/tickets/ready/260525-feat-ws-dashboard-sqlite-agent-activity-source.md` - selected Phase 1 scope.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - `260525-ws-dashboard-sqlite-agent-activity-source`, Activity projection, read model, and transcript contracts.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard Activity ownership and privacy constraints.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - wsstore role/current-instance and file-backed payload split.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - SQLite metadata authority and lock behavior.
- [Must] `ws-dashboard/crates/daemon/src/work_root_activity.rs` - existing projector and transcript readers.
- [Must] `agents-plugin-tool/internal/wsstore/store.go` - authoritative `agent_defs` schema.
