# Brief: 260525-feat-ws-dashboard-sqlite-agent-activity-source Phase 2

## Intent

Add retained named-agent instance history to the WorkRoot Activity feed by reading useful `agent_instances` rows from the ws SQLite registry. The Activity Console should show historical instance items when retained payloads or diagnostics are still useful, while preserving the current-role compatibility contract for agent rows and summary counts.

## Scope Boundary

Implement only Phase 2 of `260525-feat-ws-dashboard-sqlite-agent-activity-source`: retained `agent_instances` rows may add historical `ActivityFeed.items`, and transcript reads for those historical items must resolve through the instance row `state_path`. Leave Phase 3 SQLite-aware refresh/versioning unimplemented except for any minimal item-version plumbing required so existing watch/event code does not regress.

## Caller-Visible Contract

`ActivityFeed.items` may include both current role items and retained historical instance items. `ActivityFeed.agents` and `ActivityFeed.summary.total` remain based only on current `agent_defs` role rows, so adding retained historical items must not increase current agent counts. Current role item ids stay `agent:<agentKey>`. Historical instance item ids must be opaque, stable for the instance row, and non-colliding with current role ids. Transcript routes must resolve both current role ids and historical instance ids using the same file-backed readers, without leaking cache paths, state paths, session ids, raw instance ids, database names, or private payload paths.

## Contract Instructions

Extend `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs` with a read-only adapter for retained `agent_instances` rows. Keep the existing `agent_defs` adapter as the authority for current roles, `ActivityFeed.agents`, and summary totals.

Extend `ws-dashboard/crates/daemon/src/work_root_activity.rs` to merge historical instance projections into `ActivityFeed.items` only. Do not add historical instances to `agents`, do not include them in `summarize`, and do not change the frontend response shape.

Add a distinct historical id parser and prefix, for example `agent-instance:<opaque-token>`. Keep `agent:<agentKey>` unchanged. The opaque token must be deterministic from stable registry identity and must not expose `state_path`, host paths, session ids, public names, or raw payload directory names. The daemon already has local SHA-256 helpers in `work_root_activity.rs`; reuse existing helpers before adding dependencies.

Extend transcript routing in `work_root_activity.rs` so `agent:` ids resolve through current `agent_defs` rows and historical `agent-instance:` ids resolve through `agent_instances.state_path`, then reuse the existing Codex native transcript and `output.md` file readers. Unknown, filtered, cleanup-deleted, payload-missing, or otherwise unavailable historical ids should return the existing bounded unavailable/empty transcript behavior instead of panicking or exposing lookup internals.

Filter historical rows whose cleanup state means deleted payloads, internal retention tombstones, or rows with missing/unsafe payload paths and no useful metadata. Keep failed, cancelled, completed, retired, or otherwise non-current instances when they have at least one useful signal: output availability, current-call state, native transcript availability, `last_output_path`, meaningful status/diagnostic state, or cleanup diagnostics.

Do not reintroduce `agent.json` discovery, temporary mock data, frontend-side synthesis, route-specific hardcoding, or registry writes from the dashboard daemon.

## Integration Test Instructions

Extend daemon Activity route tests in `ws-dashboard/crates/daemon/tests/routes.rs`; add fixture helpers for `agent_instances` alongside existing `agent_defs` helpers. Cover a retained historical instance without `agent.json` that appears in `items` without increasing `agents.length` or `summary.total`.

Add transcript route coverage proving a current role item still resolves by role `state_path` and a retained historical item resolves by instance `state_path`. Add assertions that current `agent:` ids and historical `agent-instance:` ids cannot collide. Add filtering coverage for cleanup-deleted/tombstone or payload-useless historical rows, and retention coverage for failed/cancelled/completed/retired rows that still have useful payload, call state, transcript availability, or diagnostics.

## Implementation Strategy Decisions

Use SQLite as the metadata authority for both current role rows and retained instance rows, but keep their semantics separate: `agent_defs` is current-role state, `agent_instances` is item history. Historical items are Activity items only; they are not compatibility agent rows.

Project historical rows through the existing named-agent projection/readers as much as possible by introducing an internal record/projection path that can carry instance identity and payload `state_path` without changing browser-visible agent row types.

Treat unreadable, locked, missing, or incompatible registry state as an empty healthy projection, consistent with Phase 1. Do not degrade the whole feed because historical rows are unavailable.

## Rejected Alternatives

Do not count historical instances in `summary.total`; the ticket requires totals to remain current-role counts.

Do not use `state_path`, public name, session id, or host path as the historical activity id; those values may leak private layout or collide with current role ids.

Do not resurrect filesystem `agent.json` scanning as a fallback; Phase 1 intentionally moved current metadata authority to SQLite.

Do not implement Phase 3 registry change versioning in this slice.

## Approach

- Add an `ActivityRegistryAgentInstanceRecord` or equivalent adapter record for `agent_instances`.
- Build current role projections from `agent_defs` exactly as today for `agents`, summary, and `agent:` items.
- Build retained historical projections from useful `agent_instances` rows and append them only to `items`.
- Route transcript ids through a source enum or equivalent resolver so current and historical ids resolve to the correct registry row.
- Extend route fixtures and tests around retained instance rows, id opacity, filtering, and transcript resolution.

## Constraints

- Preserve existing JSON field names and response shapes.
- Keep all registry reads read-only and off the async runtime thread.
- Keep path handling safe-relative under the wsstate `agents` directory.
- Preserve privacy assertions for cache roots, state paths, session ids, database filenames, raw instance ids, and output paths.
- Avoid broad formatting of unrelated daemon files; existing package-level formatting drift is out of scope.

## Out of scope

- Phase 3 SQLite-aware refresh/versioning beyond minimal no-regression item-version handling.
- Frontend UI changes.
- Registry writes or wsstore schema migrations.
- Changes to named-agent retention cleanup policy.
- Broad router/server formatting cleanup.

## Details

The wsstore schema defines `agent_instances(instance_id, agent_key, public_name, state_path, backend, harness, tier, model, effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, ... cleanup_state, cleanup_error, pinned)`. Historical projection should use these fields and should ignore rows with unsafe or empty `state_path` unless another useful, non-private diagnostic-only representation is explicitly needed.

Suggested cleanup filtering: hide `cleanup_deleted` rows and tombstone/internal cleanup rows; keep `current`, `retired`, `cleanup_failed`, empty cleanup state, and other non-deleted states only when useful signals exist. If code discovers exact cleanup states in wsstore that conflict with this guidance, escalate before changing the caller-visible contract.

Historical labels and source display may reuse public name when present, otherwise the role key or a bounded generic role label. Browser-visible metadata may include the role `agentId` and a public indication that the item is historical, but must not expose raw instance ids, state paths, session ids, or payload locations.

## Verification Contract

Required:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon work_root_activity -- --nocapture`
- `cargo check --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`
- `rustfmt --edition 2021 --check ws-dashboard/crates/daemon/src/work_root_activity.rs ws-dashboard/crates/daemon/src/work_root_activity_registry.rs ws-dashboard/crates/daemon/tests/routes.rs`

If `rustfmt --check` on `routes.rs` fails only due pre-existing unrelated formatting drift, report the precise failure and also run a narrower check on changed daemon source files.

## References

- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - Activity Console ownership, privacy, route, and test constraints.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - role/current-instance split, retention cleanup, and SQLite/payload ownership.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - SQLite Activity Source and Activity Console read-model contracts.
- [Must] `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs` - current SQLite read adapter.
- [Must] `ws-dashboard/crates/daemon/src/work_root_activity.rs` - current feed projection, id parsing, transcript resolution, and unit tests.
- [Must] `ws-dashboard/crates/daemon/tests/routes.rs` - Activity route fixtures and integration tests.
- [Maybe] `agents-plugin-tool/internal/wsstore/store.go` - authoritative `agent_instances` schema and cleanup state behavior.
