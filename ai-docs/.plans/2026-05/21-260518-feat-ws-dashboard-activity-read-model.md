# Survey: 260518-feat-ws-dashboard-activity-read-model

## Reusable Components
- `ws-dashboard/crates/core/src/activity.rs#L5-L63` — Current WorkRoot Activity public serde surface: camelCase, `WorkRootId`-scoped, and already redacts paths/session/process fields; the new `ActivityFeed`, `ActivityItem`, `ActivityTranscript`, and `TranscriptBlock` types can replace or coexist with this shape.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L43-L99` — `WorkRootActivityProjector`: daemon-owned projector with explicit `cache_home` override and `spawn_blocking` around Git/cache scanning and JSON parsing.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L137-L157` — `resolve_work_root_agents_dir`: wsstate-compatible primary/linked Git workRoot agents-directory derivation for named-agent source lookup.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L198-L249` — named-agent scanner: discovers `agents/*`, supports recent-limit sorting before final rows, and maps malformed/missing records into row-level degradation.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L281-L346` — named-agent row projection: consumes only bounded public metadata from `agent.json`, collapses `session_id` to presence, and converts output availability into a path-free hint.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L349-L379` — current-call projection: extracts active/terminal/timing/error state from `current/state.json` while omitting pid/session/stream paths.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L18-L37` — `OpenedWorkRoots`: daemon-owned `workRootId` to host-path resolver already used by protected workRoot routes.
- `ws-dashboard/frontend/src/workRootActivity.ts#L52-L78` — existing Activity route builder and fetch wrapper: encoded opaque `workRootId`, JSON `Accept`, and `apiErrorDetail` error propagation.
- `ws-dashboard/frontend/src/workRootFiles.ts#L80-L109` — route-helper pattern for path/query construction plus fetch error handling; useful precedent for adding a transcript endpoint helper without browser route authority.
- `ws-dashboard/crates/core/src/events.rs#L19-L47` and `ws-dashboard/crates/daemon/src/events.rs#L39-L64` — cursor/backfill precedent: event blocks use cursor/sequence/timestamp/category and `events_after` returns bounded slices after a cursor.

## Existing Patterns
- Protected route wiring: see `ws-dashboard/crates/daemon/src/router.rs#L37-L91` — dashboard API routes are added to `protected` before the owner-auth layer; Activity already sits at `/api/dashboard/work-roots/{work_root_id}/activity`.
- Unknown workRoot handling: see `ws-dashboard/crates/daemon/src/work_root_activity.rs#L102-L119` and `ws-dashboard/crates/daemon/src/work_root_files.rs#L111-L151` — route handlers resolve opened roots from state and return JSON `{ error }` with 404 on unknown ids.
- Backend route tests: see `ws-dashboard/crates/daemon/tests/routes.rs#L1031-L1084` — helpers build `AppState`, pair owner cookies, assert unauthenticated rejection, and assert unknown workRoot JSON.
- Named-agent fixture route tests: see `ws-dashboard/crates/daemon/tests/routes.rs#L1197-L1217` and `ws-dashboard/crates/daemon/tests/routes.rs#L1257-L1467` — helpers seed `agent.json` and `current/state.json`, then assert summary/status/private-field redaction.
- Malformed-record degradation tests: see `ws-dashboard/crates/daemon/tests/routes.rs#L1519-L1635` — one bad metadata row and one bad current-call row degrade without failing the route.
- Primary/linked Git workRoot coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L1678-L1759` — verifies the primary key and linked `project@worktree` key shape.
- Frontend helper tests: see `ws-dashboard/frontend/src/workRootActivity.test.ts#L39-L93` — tests encoded route construction, recent-limit query, fetch headers, returned shape consumption, and JSON error propagation.
- Existing visible Activity pane consumers: see `ws-dashboard/frontend/src/App.tsx#L1206-L1285` and `ws-dashboard/frontend/src/App.tsx#L2080-L2245` — current UI expects `agents` and named-agent rows; response migration needs either compatibility or synchronized helper/UI update even if no new UI is added.

## Relevant Interfaces
- `ai-docs/spec/ws-web-dashboard/index.md#L276-L303` — `{#260521-ws-dashboard-activity-console-read-model}`: source-neutral feed plus per-item transcript backfill, ordering, cursor/bounds, and redaction requirements.
- `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-read-model.md#L25-L52` — ticket decisions and constraints: route may evolve, public concepts names, named agents as first source, bounded degraded states.
- `ai-docs/tickets/todo/260518-epic-ws-dashboard-activity-console.md#L37-L67` — cross-child vocabulary and read-only non-scope for controls, exec, and broader transcript sources.
- `ai-docs/mental-model/ws-web-dashboard.md#L63-L66` — WorkRoot Activity projection ownership and redaction summary.
- `ai-docs/mental-model/ws-web-dashboard.md#L118-L122` — Activity route derives wsstate layout from canonical Git roots and keeps cache/session/process/stream paths private.
- `ai-docs/mental-model/ws-web-dashboard.md#L151-L153` — frontend command payloads must avoid private paths and visible controls must stay on command dispatch; relevant if existing Activity pane controls are touched.
- `ai-docs/mental-model/ws-web-dashboard.md#L239-L245` — WorkRoot Activity coupling requires projection, core structs, protected route tests, and wsstate cache layout to change together.
- `ai-docs/mental-model/named-agent-runtime.md#L13-L25` — named-agent status/current-call semantics: only queued/running are active; result requires terminal completion and `output.md`.
- `agents-plugin-tool/internal/wsagent/agent.go#L323-L370` — source record schema for `agent.json` and `current/state.json`, including private fields that must not cross the dashboard API boundary.
- `agents-plugin-tool/internal/wsagent/agent.go#L690-L708` — completed calls write final result text to `output.md` and persist `LastOutputPath` as `output.md`; useful for minimal named-agent transcript backfill.
- `ws-dashboard/crates/core/src/lib.rs#L7-L10` — current core activity type re-exports; new public core types likely need matching exports for daemon/tests/frontend contract alignment.
- `ws-dashboard/frontend/package.json#L6-L15` — relevant frontend verification scripts: `test:work-root-activity` and `build` already exist.

## Constraints
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L79-L98` — Git/cache scanning is deliberately inside `spawn_blocking`; transcript lookup that reads `output.md` or cache records should preserve that async-worker boundary.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L610-L655` — current Rust deserializers intentionally omit pid/session/stdout/stderr paths; adding transcript logic must not deserialize or echo them into public diagnostics.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1136-L1149`, `ws-dashboard/crates/daemon/tests/routes.rs#L1446-L1462`, and `ws-dashboard/crates/daemon/tests/routes.rs#L1620-L1630` — existing redaction assertions already cover host paths, cache paths, session ids, pid, stream names, `agent.json`, and `state.json`; transcript tests need equivalent body-level checks.
- `ws-dashboard/frontend/src/App.tsx#L1235-L1271` — current recent refresh merges named-agent lists by `agentId`; feed ordering and item ids may invalidate this helper unless it is retained as compatibility-only.
- `ws-dashboard/frontend/src/App.tsx#L2121-L2123` — existing Activity pane is explicitly read-only and path/session/process-free; helper response changes should not force visible control or UI-scope changes in this slice.
- `ai-docs/mental-model/ws-web-dashboard.md#L63-L66` and `ai-docs/mental-model/ws-web-dashboard.md#L390-L393` — WorkRoot Activity remains a daemon-owned read-only projection and must not become ws MCP/named-agent session authority.

## Risk Signals
- `ws-dashboard/frontend/src/App.tsx#L2153-L2245` — Possible contract risk: existing visible Activity pane directly renders `WorkRootActivityView.agents`; replacing `/activity` with a feed snapshot can break current UI unless a compatibility projection or synchronized mapping remains.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L243-L248` — Possible test/product risk: final named-agent rows sort alphabetically by `agent_id`, while the new contract requires live/attention/blocked/failed/recent priority before A-Z tie-breaking.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L325-L329` and `agents-plugin-tool/internal/wsagent/agent.go#L690-L708` — Possible reuse/risk: the current projection only exposes output presence; transcript backfill needs private daemon-side file reads from `output.md` without exposing the path or raw cache record identity.
- `ws-dashboard/crates/daemon/src/router.rs#L82-L85` — Possible route migration risk: the existing `/activity` endpoint is already consumed by frontend helpers; adding `/activity/items/{activityId}/transcript` is straightforward, but changing `/activity` response shape needs compatibility tests.
- `ws-dashboard/crates/daemon/src/events.rs#L23-L64` — Possible shortcut risk: instance-event fixtures are explicitly mock-backed; the brief forbids mock-only data as the canonical Activity Console route source, so transcript logic should not copy the fixture provider as production source.
- `ws-dashboard/frontend/src/workRootActivity.ts#L80-L128` — Possible frontend risk: merge/summarize logic assumes named-agent statuses and `agents`; a source-neutral `items` feed needs updated pure helpers/tests or isolation from existing badge refresh behavior.

## Opinion
- The survey found enough concrete source and tests to implement without broader research; no strategy escalation is needed.
- The safest code touch map appears contained to public core activity types/re-exports, `work_root_activity.rs` projection plus route handler(s), protected route tests, and `workRootActivity.ts`/`workRootActivity.test.ts`; visible UI files are only a compatibility risk unless the response migration forces them to compile.
- Caller requested sequencing and test strategy, but this delegated survey is limited to discovery; the file above records concrete implementation anchors and risk controls rather than prescribing a step-by-step implementation plan.
