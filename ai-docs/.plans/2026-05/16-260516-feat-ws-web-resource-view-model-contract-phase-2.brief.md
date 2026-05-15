# Brief: 260516-feat-ws-web-resource-view-model-contract Phase 2

## Intent

Complete Phase 2 of the dashboard resource view-model contract by exposing an
owner-authenticated resource API backed by deterministic mock data. The result
should let later frontend work consume one stable hierarchy shape before live
workspace discovery, PTY, named-agent, or event-stream integrations exist.

## Approach

- Treat skeleton commit `ae44dad` as the public contract baseline.
- Keep view-model structs in `ws-dashboard-core` and daemon provider/route
  wiring in `ws-dashboard-daemon`.
- Serve `GET /api/dashboard/resources` from the existing protected router so
  owner cookie and bearer auth paths cover it automatically.
- Keep the mock provider deterministic and broad enough to exercise singleton
  chains, multi-root workspaces, all workRoot kinds, offline/inaccessible
  states, main/sub instances, loading/stale/error state, compactable hints, and
  actions.
- Verify both the core JSON contract tests and the protected route tests.

## Constraints

- Do not add live filesystem discovery, wsstate integration, PTY, named-agent
  live calls, event streams, frontend package code, or root picker behavior.
- Do not make the dashboard daemon ws MCP session authority.
- Keep `/pair` the only unauthenticated browser route.
- Preserve the full hierarchy in the API; UI compaction remains a frontend
  presentation policy.

## Out of scope

- Frontend shell rendering.
- Local workspace discovery and root picker behavior.
- Event stream/reconnect/backfill behavior.
- Terminal, editor, viewer, translation, bookmark, or remote daemon features.

## Details

Existing skeleton contracts:

- `ws-dashboard/crates/core/src/view_model.rs` defines
  `DashboardResourcesView`, `ServerView`, `WorkspaceView`, `WorkRootView`,
  `InstanceView`, `ViewState`, and `ActionHint`.
- `ws-dashboard/crates/daemon/src/resources.rs` defines the provider seam and
  `dashboard_resources` route handler.
- `ws-dashboard/crates/daemon/src/mock.rs` defines
  `MockDashboardResourcesProvider`.
- `ws-dashboard/crates/daemon/src/router.rs` nests
  `/api/dashboard/resources` inside the protected router.
- `ws-dashboard/crates/daemon/tests/routes.rs` covers unauthenticated rejection
  and authenticated mock hierarchy response shape.

Acceptance checks:

- `cargo test -p ws-dashboard-core --lib`
- `cargo test -p ws-dashboard-daemon --test routes`
- `cargo test --workspace`

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-core-resource-vocabulary`,
  `260516-ws-web-dashboard-resource-view-model-contract`, and
  `260516-ws-web-dashboard-mock-view-model-fixtures`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - protected router, owner
  auth, daemon-owned view-models, and core workRoot vocabulary.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-resource-view-model-contract.md`
  - Phase 2 scope and success criteria.
