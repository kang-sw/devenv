# Survey: 17-260517-feat-ws-dashboard-workroot-activity-phase-1

## Reusable Components
- `ws-dashboard/crates/core/src/activity.rs#L5-L62` — `WorkRootActivityView`, `WorkRootActivitySummary`, `NamedAgentActivityView`, `NamedAgentCallActivityView`: public camelCase Phase 1 response shape already skeletoned; keep host/cache/session/process/stream internals out of these fields.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L18-L60` — `WorkRootActivityProjectionConfig` / `WorkRootActivityProjector`: daemon-local cache-home seam and placeholder projection point for deriving wsstate/wsagent rows from an opened workRoot path.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L18-L55` — `OpenedWorkRoots`: shared opened-workRoot registry; `resolve` is the existing opaque `workRootId` to daemon-private host path lookup used by file, terminal, and activity routes.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L63-L89` — `work_root_activity` / `activity_error`: existing route handler reports `404 { "error": "unknown workRoot" }` before projecting state.
- `ws-dashboard/frontend/src/workRootActivity.ts#L3-L64` — `workRootActivityEndpoint` / `fetchWorkRootActivity`: TypeScript mirror shape and protected-route fetch helper already exist for route-helper tests only.
- `agents-plugin-tool/internal/wsstate/paths.go#L83-L116` — `CacheRoot` / `Resolve`: source-of-truth path derivation for `$WS_CACHE_HOME`, default `~/.cache/ws@kang-sw-devenv`, project key, and linked-worktree key.
- `agents-plugin-tool/internal/wsagent/agent.go#L323-L370` — `Agent` / `CurrentCall`: JSON fields present in `agents/*/agent.json` and optional `current/state.json`; includes private fields that dashboard output must summarize or omit.

## Existing Patterns
- Protected dashboard API routes: see `ws-dashboard/crates/daemon/src/router.rs#L37-L91` — route is already nested under owner auth alongside resources/files/terminals.
- Blocking live resource projection: see `ws-dashboard/crates/daemon/src/resources.rs#L23-L31` — existing route uses `tokio::task::spawn_blocking` around synchronous filesystem-backed view construction.
- Unknown opened workRoot handling: see `ws-dashboard/crates/daemon/src/work_root_files.rs#L111-L123` and `ws-dashboard/crates/daemon/src/terminal.rs#L354-L361` — handlers reject unresolved opaque ids without leaking paths.
- Route test setup with owner pairing and opened roots: see `ws-dashboard/crates/daemon/tests/routes.rs#L1030-L1096` and `ws-dashboard/crates/daemon/tests/routes.rs#L1219-L1244` — activity tests already have cache-home injection plus `open_work_root_for_test`.
- No-leak response assertions: see `ws-dashboard/crates/daemon/tests/routes.rs#L1112-L1148` and `ws-dashboard/crates/core/src/activity.rs#L117-L133` — existing tests assert camelCase and absence of host/cache/session/process/stream path details.
- Frontend route-test pattern: see `ws-dashboard/frontend/src/workRootActivity.test.ts#L27-L74` and `ws-dashboard/frontend/package.json#L10-L16` — pure TypeScript endpoint/fetch tests run without browser UI.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/src/server.rs#L42-L48` — `AppState` construction: daemon currently installs the default `WorkRootActivityProjector` with no test cache override.
- `ws-dashboard/crates/daemon/src/router.rs#L24-L31` — `AppState`: route state carries `opened_work_roots`, `terminals`, and `work_root_activity` projector.
- `agents-plugin-tool/internal/wsstate/paths.go#L174-L195` — `layoutFor`: agent directories live at `<cache>/proj/<worktreeKey>/agents`, with primary roots using the project key directly.
- `agents-plugin-tool/internal/wsstate/paths.go#L198-L219` — `gitIdentity`: worktree root and common root come from `git rev-parse`; non-Git/plain directories cannot use this exact Git path discovery.
- `agents-plugin-tool/internal/wsstate/paths.go#L253-L261` — `shortHash`: project/worktree keys are the first eight hex chars of SHA-256 over canonical paths.
- `agents-plugin-tool/internal/wsagent/agent.go#L2041-L2069` — `Manager.layout`: named-agent file locations are `agent.json`, `current/state.json`, stdout/stderr/runtime logs, output, events, and prompt files under each sanitized agent key.
- `agents-plugin-tool/internal/wsagent/agent.go#L2161-L2166` — `AgentKey`: agent directory keys are trimmed names with unsafe characters replaced by `-`; directory name can serve as opaque `agentId` when metadata is malformed.
- `agents-plugin-tool/internal/wsagent/agent.go#L28-L40` — agent and call status constants: idle/running/blocked/failed/erased plus queued/running/completed/failed/cancelled are the runtime vocabulary to summarize.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L75-L94` — dashboard APIs address workRoots by opaque ids; host paths, Git roots, wsstate paths, workRoot keys, and runtime session identifiers stay daemon-private.
- `ai-docs/spec/ws-web-dashboard/index.md#L219-L227` — WorkRoot Activity projection is read-only named-agent status only, with malformed/stale records represented as bounded unavailable/diagnostic states and no control actions.
- `ai-docs/spec/named-agent-runtime.md#L13-L22` — registry metadata and current-call state are file-backed under each worktree-local agent directory.
- `ai-docs/mental-model/ws-web-dashboard.md#L79-L84` — dashboard is not ws MCP authority; wsstate/named-agent views should be daemon-owned projections rather than adopting caller MCP sessions.
- `ai-docs/mental-model/named-agent-runtime.md#L13-L18` — all agent paths must come from wsstate layout plus `AgentKey`; active calls are only `queued` and `running`.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1017-L1028` — skeleton route-test checklist explicitly requires auth rejection, unknown id, empty projection, idle/running/failed fixtures, malformed-row degradation, and no leaks.
- `ws-dashboard/crates/daemon/Cargo.toml#L15-L27` — daemon already has `serde`, `serde_json`, `tokio`, and `anyhow`; no extra crate is obviously required for Phase 1 parsing.

## Verification Commands
- `cd ws-dashboard && cargo check -p ws-dashboard-daemon`
- `cd ws-dashboard && cargo test -p ws-dashboard-core activity::tests::work_root_activity_view_serializes_camel_case_without_host_internals`
- `cd ws-dashboard && cargo test -p ws-dashboard-daemon work_root_activity_route`
- `cd ws-dashboard/frontend && npm run test:work-root-activity`
- `git diff --check`

## Opinion
- The skeleton already contains the correct route, core/frontend shapes, protected auth placement, and test seam; the main uncertainty is faithfully reproducing wsstate's Git-aware layout derivation in Rust without mutating cache metadata or shelling out to MCP tools.
- Plain non-Git opened workRoots are possible in dashboard discovery, while wsstate `Resolve` currently depends on Git; Phase 1 should treat absent/unresolvable wsstate layout as an empty or unavailable projection without expanding scope beyond named-agent cache reads.
- Keep the survey scope backend-only: no top-bar badge, Activity pane, running-command rows, browser gate, or lifecycle controls are needed for Phase 1.
