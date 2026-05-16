# Brief: 260516-feat-ws-web-stable-pairing-routes Phase 2

## Intent

Reserve explicit server-scoped browser route identity for the dashboard by
making `/servers/:serverId/...` the intended refresh-safe route shape for
frontend navigation, without making browser routes the authority for resource
identity.

## Approach

- Introduce the smallest frontend route basis needed for
  `/servers/:serverId/...`.
- Keep daemon-owned opaque ids from the resource view model as the source of
  truth.
- Preserve Phase 1 token-free pairing behavior and all daemon auth/static
  serving guardrails.
- Add focused frontend and/or daemon tests that prove the route basis is
  refresh-safe and does not hide server identity inside workspace, workRoot, or
  instance ids.

## Constraints

- Scope is Phase 2 only.
- Do not change `/pair` success or failure behavior.
- Do not implement workbench split groups.
- Do not implement complete deep-link coverage for every dashboard resource.
- Do not expose host paths as route identity.
- Do not make browser route params authoritative over daemon resource ids.

## Out of scope

- Stable workRoot/mainInstance deep links beyond the initial route basis.
- Dockview/FlexLayout workbench layout.
- Live terminal, agent, editor, viewer, or diagnostics routing.
- Linked daemon/server forwarding.

## Details

Acceptance behavior:

- The frontend recognizes or normalizes server-scoped dashboard routes beginning
  with `/servers/:serverId`.
- The current selected server can be represented in the browser URL with an
  explicit `serverId`.
- Refreshing a server-scoped app URL continues to load the protected frontend
  shell through the existing authenticated static/fallback route boundary.
- Resource ids continue to come from `/api/dashboard/resources`; the browser
  must not infer workspace, workRoot, or instance identity from host paths.
- Existing resource navigation, loading/error/stale states, and command ids
  remain intact.

Verification should run frontend build checks and daemon route/server tests if
static fallback behavior changes.

## References

- [Must] `ws-dashboard/frontend/src/App.tsx` - current browser shell and
  resource navigation behavior.
- [Must] `ws-dashboard/crates/daemon/src/router.rs`,
  `ws-dashboard/crates/daemon/src/auth.rs`, and
  `ws-dashboard/crates/daemon/src/server.rs` - protected shell routing, pairing
  redirect, and auth boundary.
- [Must] `ws-dashboard/crates/core/src/view_model.rs` and
  `ws-dashboard/crates/core/src/resources.rs` - authoritative resource id and
  hierarchy vocabulary.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-server-scoped-browser-routes`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260516-ws-web-dashboard-core-resource-vocabulary`,
  `260516-ws-web-dashboard-token-free-pairing-landing`, and
  `260516-ws-web-dashboard-protected-frontend-shell`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - route boundary,
  opaque-id contract, frontend/App coupling, and static-serving rules.
- [Maybe] `ws-dashboard/crates/daemon/tests/routes.rs` and
  `ws-dashboard/crates/daemon/tests/server.rs` - update only if daemon static
  fallback behavior changes.
- [Maybe] `ws-dashboard/crates/daemon/tests/fixtures/dashboard_resources.json`
  - use when route assertions need fixture resource ids.
