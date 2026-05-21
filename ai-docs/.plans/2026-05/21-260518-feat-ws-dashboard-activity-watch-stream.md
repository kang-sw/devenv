# Survey: 260518-feat-ws-dashboard-activity-watch-stream

## Reusable Components
- `ws-dashboard/crates/core/src/activity.rs#L8-L23` — `ActivityFeed`/`WorkRootActivityView`: existing public feed snapshot shape with `workRootId`, `updateMode`, `feedCursor`, `items`, and compatibility `agents` projection to reuse for stream invalidations.
- `ws-dashboard/crates/core/src/activity.rs#L39-L74` — `ActivityItem`, `ActivitySourceDisplay`, `ActivityTranscriptAvailability`: public source-neutral item and transcript availability shapes suitable for `itemUpserted` payloads.
- `ws-dashboard/crates/core/src/activity.rs#L76-L101` — `ActivityTranscript`/`TranscriptBlock`: selected transcript public backfill contract and cursor fields referenced by `transcriptUpdated` events.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L56-L138` — `WorkRootActivityProjector`: daemon-owned projection entrypoint already moves Git/wsstate scanning and transcript reads to `spawn_blocking`.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L206-L226` — `resolve_work_root_agents_dir`: reusable wsstate cache layout resolver for mapping an opened Git workRoot to `<cacheHome>/proj/<key>/agents` without exposing paths.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L288-L336` — `scan_named_agents`: existing named-agent directory scan and recent-limit behavior; useful for fallback/coalesced refresh snapshots.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L449-L584` — named-agent row-to-`ActivityItem` helpers and activity id parser: existing normalization for source display, item status, timing, transcript availability, and `agent:` ids.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L18-L55` — `OpenedWorkRoots`: shared in-memory workRoot registry and opaque id to private path resolver used by current protected workRoot routes.

## Existing Patterns
- Protected route nesting: see `ws-dashboard/crates/daemon/src/router.rs#L35-L102` — all dashboard APIs, current activity routes, static UI, and future socket routes sit inside the `require_owner_auth` layer.
- Auth-before-handler boundary: see `ws-dashboard/crates/daemon/src/router.rs#L137-L158` and `ws-dashboard/crates/daemon/src/auth.rs#L144-L166` — HTTP/SSE routes inherit cookie/bearer auth plus Host/Origin checks before handler execution.
- Unknown workRoot handling: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L141-L158` and `ws-dashboard/crates/daemon/src/work_root_activity.rs#L160-L188` — current activity routes resolve `workRootId` first and return bounded JSON errors.
- Fixture instance stream precedent: see `ws-dashboard/crates/core/src/events.rs#L19-L48` and `ws-dashboard/crates/daemon/src/events.rs#L12-L83` — existing event types and cursor backfill scaffold are JSON response based, not live SSE.
- Route-level test style: see `ws-dashboard/crates/daemon/tests/routes.rs#L122-L130` and `ws-dashboard/crates/daemon/tests/routes.rs#L3093-L3198` — tests build `AppState`, pair for cookie auth, and exercise router endpoints with `tower::ServiceExt`.
- Activity route fixture style: see `ws-dashboard/crates/daemon/tests/routes.rs#L1773-L1987` — tests seed wsstate-like agent cache directories, fetch activity/transcript routes, and assert redaction of roots/cache files/session fields.
- Frontend fallback polling precedent: see `ws-dashboard/frontend/src/App.tsx#L1235-L1297` — current UI polls recent activity only while the pane is open; backend stream work should not assume this is already removed.

## Relevant Interfaces
- `ws-dashboard/crates/core/src/lib.rs#L1-L14` — public core re-export surface currently exports activity and instance-event types; new shared event types likely need exposure here if used across daemon/frontend contracts.
- `ws-dashboard/crates/daemon/src/router.rs#L21-L33` — `AppState` currently carries `WorkRootActivityProjector` by value; any watch service state must fit this cloned router state model.
- `ws-dashboard/crates/daemon/src/server.rs#L34-L48` — daemon startup constructs `AppState` with default `OpenedWorkRoots`, `TerminalRegistry`, and `WorkRootActivityProjector`.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L43-L54` — existing query structs use camelCase serde; the new `after` cursor query should follow this route-local pattern.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L599-L705` — named-agent transcript backfill reads `output.md`, applies numeric cursors, and returns public transcript state without exposing source paths.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L764-L818` — current-call parsing and text bounding omit PID/session/stream paths and cap backend errors.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L1025-L1070` — deserialized wsagent metadata/state intentionally excludes private fields such as `pid`, stream paths, and session id.
- `ws-dashboard/frontend/src/workRootActivity.ts#L93-L178` — frontend mirrors the current read model and route helper names; later live UX will likely add stream event types here.

## Constraints
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L91-L111` — Git layout discovery and cache scanning are synchronous today and explicitly moved to a blocking pool; event normalization should avoid blocking Axum workers.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L266-L269` — current feed cursors are snapshot-derived strings, not a durable event log cursor; reconnect behavior must account for this mismatch.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L288-L336` — missing or unreadable `agents/` directories intentionally produce an empty healthy projection rather than a route failure.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L350-L365` — current recent-change detection watches only portable mtimes for `agent.json`, `output.md`, and selected `current/*` files; nested/atomic watcher behavior is not yet modeled.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L577-L584` — accepted named-agent activity ids reject empty keys and path separators, which is useful when mapping delete/recreate events back to public ids.
- `ws-dashboard/Cargo.toml#L19-L31` and `ws-dashboard/crates/daemon/Cargo.toml#L10-L24` — no filesystem watcher or tokio-stream helper crate is currently present; adding live watch/SSE may require dependency updates.

## Risk Signals
- `ws-dashboard/crates/daemon/src/events.rs#L67-L83` — Possible contract risk: the existing “event stream” scaffold returns finite JSON, so copying it directly would miss the brief's SSE/heartbeat stream behavior.
- `ws-dashboard/crates/core/src/events.rs#L19-L48` — Possible public vocabulary risk: instance events carry `streamId`, `resourcePath`, and categories from the older instance scaffold; Activity Console events need feed/transcript vocabulary instead.
- `ws-dashboard/crates/daemon/src/router.rs#L21-L33` — Possible reuse/state risk: `AppState` has only a stateless `WorkRootActivityProjector`; a watcher registry or replay buffer must not be accidentally recreated per request/router clone.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L266-L269` — Possible cursor risk: `feed_cursor` is snapshot based and may not safely serve as event replay cursor without an explicit event cursor model.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L288-L336` — Possible fallback/delete risk: current scans cannot distinguish “agents dir missing,” “agent erased,” and “watch unavailable” except by rebuilding the snapshot.
- `ws-dashboard/frontend/src/App.tsx#L1235-L1297` — Possible scope-coupling risk: frontend still has a recent-poll hotfix; backend tests should prove stream/fallback independently without relying on frontend live UX changes.

## Opinion
- The backend read model is well factored for reuse, but live watch state/replay is not scaffolded yet; implementation will need careful separation between public event vocabulary and private wsstate/watch details.
- The mental model references Activity Console read model/UI shell but does not yet list the planned watch-stream anchor in its spec refs; avoid assuming prior dashboard docs already capture stream implementation details.
