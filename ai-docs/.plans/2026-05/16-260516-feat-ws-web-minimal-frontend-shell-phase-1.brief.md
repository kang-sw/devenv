# Brief: 260516-feat-ws-web-minimal-frontend-shell Phase 1

## Intent

Complete Phase 1 by establishing a buildable React/TypeScript/Vite frontend
package and serving its production assets through the dashboard daemon's
existing owner-auth boundary.

## Approach

- Treat skeleton commit `83e6e23` as the contract baseline.
- Keep `/pair` as the only unauthenticated browser route.
- Serve `ServeConfig.static_dir/index.html` at `/` and
  `ServeConfig.static_dir/assets/*` at `/assets/*` only through the protected
  router.
- Keep the no-static-dir placeholder route available for daemon development.
- Keep the frontend package minimal: build entrypoints and a placeholder shell
  only, with navigation/detail/resource rendering deferred to Phase 2.

## Constraints

- Do not add Phase 2 navigation/detail UI or resource rendering.
- Do not add live discovery, event streams, PTY, editor, viewer, translation,
  named-agent controls, root picker, or bookmark behavior.
- Do not create another unauthenticated route beside `/pair`.
- Preserve existing daemon auth, Host/Origin, bearer, bind-mode, and resource
  API behavior.

## Out of scope

- Inspectable navigation tree and detail pane.
- Command registry beyond preserving room for later command ids.
- Frontend resource API fetching.
- Visual polish beyond a buildable protected shell.

## Details

Existing skeleton contracts:

- `ws-dashboard/frontend/package.json` provides `dev`, `build`, and `preview`
  scripts for the React/TypeScript/Vite package.
- `ws-dashboard/frontend/package-lock.json` pins the initial package graph.
- `ws-dashboard/crates/daemon/src/router.rs` serves static assets from
  `ServeConfig.static_dir` behind `require_owner_auth`.
- `ws-dashboard/crates/daemon/tests/routes.rs` covers unauthenticated rejection
  and paired-cookie success for `/` and `/assets/app.js`.

Acceptance checks:

- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard && cargo test -p ws-dashboard-daemon --test routes`
- `cd ws-dashboard && cargo test --workspace`

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260515-ws-web-daemon-foundation` and
  `260516-ws-web-dashboard-protected-frontend-shell`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - static_dir, protected
  router, and daemon auth boundaries.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-minimal-frontend-shell.md`
  - Phase 1 scope and success criteria.
