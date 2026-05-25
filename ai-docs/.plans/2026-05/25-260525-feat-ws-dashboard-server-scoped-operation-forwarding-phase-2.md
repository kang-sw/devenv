# Implementation Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 2

## Scope

Implement only Phase 2 backend local aliases and a one-shot forwarding skeleton. Preserve deferred scope: full operation coverage, SSE forwarding, terminal WebSocket gatewaying, remote root picker/open WorkRoot browser dogfood, credential persistence, deployment automation, and public endpoint hardening.

## Concrete File Map

- `ws-dashboard/crates/daemon/src/router.rs#L67-L215` — Protected router boundary. Add server-scoped skeleton aliases here, inside `protected`, not beside `/pair` or `/api/dashboard/link-auth`.
- `ws-dashboard/crates/daemon/src/servers.rs#L427-L469` — Existing selected-server resource resolver/forwarding path for local, unknown, auth-required/tunnel-required, bearer-auth remote resources, and bounded gateway errors.
- `ws-dashboard/crates/daemon/src/servers.rs#L609-L631` — Existing `DashboardResourcesView` rewrite logic that updates top-level server identity and nested `ResourcePath.serverId` values.
- `ws-dashboard/crates/daemon/src/servers.rs#L682-L755` — Current remote-resource error enum and bearer `reqwest` call; extend or factor this into an allowlisted ordinary HTTP/JSON forwarding helper rather than adding a generic proxy.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L112-L277` — Representative one-shot local handlers for root picker list, create directory, pin/unpin, and open WorkRoot; likely first aliases for local equivalence.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L279-L307` — Activation local handler returns `DashboardResourcesView`, so it is a useful rewrite test target if included in the skeleton.
- `ws-dashboard/crates/core/src/view_model.rs#L11-L83` and `ws-dashboard/crates/core/src/resources.rs#L94-L104` — Public resource view and nested `ResourcePath` fields that must be rewritten for linked-server responses.
- `ws-dashboard/crates/daemon/tests/routes.rs#L128-L153` — Test `AppState` factory already installs `LinkedServerSessions` and record-only tunnels.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2181-L2268` — Existing server-scoped resources route test for local dispatch, linked refusal, and unknown server.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2333-L2445` and `#L2447-L2564` — Existing remote test-server patterns for link-auth, bearer forwarding, and resource-view rewrite assertions.
- `ws-dashboard/crates/daemon/tests/routes.rs#L7972-L7989` — Existing helper for spawning in-process remote Axum servers.

## Sequencing

1. **Define the skeleton route set.** Start with a small allowlisted set of one-shot routes: root picker list/create/pins and open WorkRoot; optionally include activation if needed to exercise resource-view rewrite through a mutating route. Do not include document/activity SSE or terminal socket paths.
2. **Register protected server-scoped aliases.** In `router.rs`, add `/api/dashboard/servers/{server_id}/...` aliases for the selected skeleton routes inside the protected router, preserving existing local routes as compatibility aliases.
3. **Factor server resolution.** In `servers.rs`, extract reusable linked-server resolution from `dashboard_server_resources`: `server-local` dispatch, unknown server, missing endpoint/tunnel-required, no token/auth-required, and connected linked server with endpoint/token.
4. **Add allowlisted one-shot forwarding.** Add a helper that accepts only explicit dashboard operation paths/methods from the skeleton allowlist, forwards ordinary HTTP/JSON with the linked-server bearer token, preserves upstream status/body where practical, and rejects stream/upgrade paths by construction.
5. **Wire local alias dispatch.** For `server-local`, call existing handlers or small shared handler functions so legacy local routes and server-scoped local aliases exercise the same behavior. Prefer extraction only where Axum handler signatures make direct reuse awkward.
6. **Wire linked dispatch.** For linked servers, build the upstream legacy local route path matching the selected operation, forward request method/query/body/content-type, add daemon-to-daemon bearer auth, and map resolver/refusal failures to bounded JSON errors.
7. **Rewrite resource responses.** Reuse or expose `rewrite_resources_for_linked_server` for forwarded routes returning `DashboardResourcesView`, including `workRoot.open` and any activation route included in the skeleton.
8. **Keep frontend untouched except compatibility.** Do not change remote root picker/open behavior, route helpers, or browser dogfood flows unless a compile/test adjustment is forced by backend route shape.

## Tests

Focused Rust tests to add/extend in `ws-dashboard/crates/daemon/tests/routes.rs`:

- Protected-route auth: unauthenticated `/api/dashboard/servers/server-local/root-picker` (or another new alias) returns `401`, matching existing protected API behavior.
- Local alias equivalence: authenticated `/api/dashboard/servers/server-local/root-picker?...` and legacy `/api/dashboard/root-picker?...` return equivalent JSON for the same fixture directory; likewise cover one POST route such as pins/create/open if practical.
- Refusal states: unknown server returns `404`; remembered linked server without token returns bounded `409` auth-required or tunnel-required without leaking SSH target/endpoint/passphrase.
- Bearer forwarding: spawn a remote daemon with `spawn_test_server`, link it, call at least one server-scoped skeleton route through the local gateway, and assert the remote protected route accepts the forwarded bearer.
- Upstream error preservation: force a remote route error such as invalid root-picker path/create request and assert the local gateway preserves status and bounded error body where practical.
- Resource-view rewriting: forward an upstream `DashboardResourcesView` response, such as open WorkRoot if included, and assert top-level `server.id` and nested `workRoots[*].resourcePath.serverId` are rewritten to the linked server id.
- Allowlist rejection: prove a non-skeleton route, an SSE route, or terminal socket route is not forwarded by the one-shot helper.

Suggested focused commands:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml linked_server
cargo test --manifest-path ws-dashboard/Cargo.toml forwarding
```

Final verification:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:root-picker
npm --prefix ws-dashboard/frontend run test:open-work-root
```

## Risks

- `ws-dashboard/crates/daemon/src/router.rs#L217-L220` — Route-boundary risk: adding canonical aliases outside `protected` would bypass owner auth; only `/pair` and remote `/api/dashboard/link-auth` are intentionally unprotected from browser session auth.
- `ws-dashboard/crates/daemon/src/servers.rs#L757-L759` — Proxy risk: existing `remote_url` blindly joins endpoint and path; the new helper must be allowlist-first and must not accept arbitrary caller-provided upstream paths.
- `ws-dashboard/crates/daemon/src/servers.rs#L443-L459` — Error-shape risk: current resource forwarding collapses most upstream failures to gateway errors; Phase 2 asks for upstream status/error preservation where practical, so tests should pin the intended behavior for skeleton routes.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L208-L277` — Rewrite risk: open WorkRoot returns an aggregated local `DashboardResourcesView` plus opened-id header; forwarding must rewrite server ids without changing the daemon-owned opened-id header semantics.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L8-L11`, `ws-dashboard/crates/daemon/src/work_root_activity.rs#L7-L10`, and `ws-dashboard/crates/daemon/src/terminal.rs#L8-L13` — Deferred-scope risk: document/activity SSE and terminal WebSocket handlers use stream/upgrade mechanics and should stay out of the ordinary HTTP forwarding skeleton.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1-L20` — Test-file scale risk: `routes.rs` is already large; keep new tests grouped near existing linked-server/resource tests or extract helpers locally to avoid duplicating request boilerplate.

## Lead Notes

- The phase is implementable without lead/user decision if the skeleton route set stays representative and small.
- If implementation wants to include more than root picker/open WorkRoot plus one resource-returning mutation, pause for lead scope confirmation because that approaches Phase 3/full-operation coverage.
- Do not claim remote operation transparency after this phase; the deliverable is a protected backend alias and forwarding substrate for later operation coverage.
