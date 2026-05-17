# Brief: 260517-feat-ws-dashboard-workroot-activity Phase 1

## Intent

Implement the read-only daemon projection behind WorkRoot Activity so dashboard
callers can ask an opened workRoot for named-agent activity without reading
`~/.cache/ws@kang-sw-devenv/` directly.

## Scope Boundary

Implement Phase 1 only: daemon-owned read-only WorkRoot Activity projection.
Use the existing skeleton contracts from `43049cb`. Do not implement Phase 2
top-bar badges or Phase 3 WorkRoot Activity pane UI.

## Caller-Visible Contract

Authenticated callers can request
`GET /api/dashboard/work-roots/{workRootId}/activity` for an opened workRoot.
The route returns a camelCase JSON `WorkRootActivityView` with the requested
`workRootId`, overall status, named-agent summary counts, and bounded agent
rows. Unknown workRoots return `404 { "error": "unknown workRoot" }`.

The response must not expose host root paths, ws cache paths, session ids, pids,
stdout/stderr paths, `agent.json`, or `current/state.json`.

## Implementation Strategy Decisions

- Keep cache parsing daemon-owned. Browser helpers consume only the protected
  route.
- Derive the ws cache layout from the same path rules documented by
  `wsstate`: `$WS_CACHE_HOME` or `~/.cache/ws@kang-sw-devenv`, project hash for
  canonical workRoot path, and linked worktree key when detectable.
- Parse `agents/*/agent.json` and optional `current/state.json`; do not shell out
  to MCP tools or call model backends.
- Treat malformed agent/current-call records as row-level degraded or
  unavailable diagnostics instead of failing the whole route.
- Keep command activity absent until `260513-feat-async-exec-output-reader`.

## Rejected Alternatives

- Do not let the browser read ws cache files directly.
- Do not add dashboard-specific agent cache state.
- Do not add top-bar badge or activity-pane UI in this phase.
- Do not expose control actions such as start, interrupt, cancel, erase, or
  retry.

## Approach

- Complete `work_root_activity.rs` by resolving the opened workRoot path,
  deriving the wsstate worktree agent directory, reading agent metadata/current
  call state, and mapping records into `WorkRootActivityView`.
- Preserve and extend the skeleton route tests with fixture cache directories
  for idle/running/failed/malformed records and no-leak assertions.
- Keep Rust public shapes in `ws-dashboard-core` and TypeScript mirror shapes in
  `workRootActivity.ts` aligned.

## Constraints

- Phase 1 must remain read-only.
- Synchronous filesystem/cache scanning must not block Axum async workers.
- Diagnostic strings must be bounded and must not include private paths or raw
  file names from the ws cache layout.
- Existing workRoot file, terminal, and resource routes must remain unchanged.

## Out of scope

- Top-bar `agents: N active` badge.
- WorkRoot Activity workbench pane.
- Running command rows.
- Agent lifecycle control.
- Native Windows terminal control-key follow-up.

## Details

Skeleton commit: `43049cb`.

Required source contracts:
- `ws-dashboard/crates/core/src/activity.rs`
- `ws-dashboard/crates/daemon/src/work_root_activity.rs`
- `ws-dashboard/frontend/src/workRootActivity.ts`

Required route:
- `GET /api/dashboard/work-roots/{workRootId}/activity`

## Verification Contract

Required verification:
- `cargo check -p ws-dashboard-daemon`
- `cargo test -p ws-dashboard-core activity::tests::work_root_activity_view_serializes_camel_case_without_host_internals`
- `cargo test -p ws-dashboard-daemon work_root_activity_route`
- `npm run test:work-root-activity`
- `git diff --check`

No Playwright/browser gate is required for Phase 1 because this phase adds the
daemon projection and frontend route helper only. Browser UI starts in Phase 2.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260517-ws-dashboard-workroot-activity-projection` - Phase 1 public behavior.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260516-ws-web-dashboard-resource-view-model-contract` - opaque workRoot identity and daemon-owned resource boundaries.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260516-ws-web-dashboard-protected-frontend-shell` - protected dashboard route seam.
- [Must] `ai-docs/spec/named-agent-runtime.md` / `260505-named-agent-registry-state-layout` - named-agent disk layout and state contract.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - protected route and dashboard-owned projection guidance.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - wsstate/wsagent path and current-call semantics.
