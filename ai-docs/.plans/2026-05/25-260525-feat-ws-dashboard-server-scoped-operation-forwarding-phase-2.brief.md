# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 2

## Intent

Add the backend foundation that makes canonical server-scoped one-shot
dashboard routes safe to register before later phases attach every operation.
The local gateway must be able to resolve `server-local` in-process and resolve
linked servers into bounded refusal or forwarding decisions without exposing a
generic daemon proxy.

## Scope Boundary

Selected scope: Phase 2, "Backend local aliases and one-shot forwarding
skeleton."

In scope:

- Protected server-scoped daemon route aliases for representative one-shot
  dashboard operations.
- `server-local` in-process dispatch through existing local route handlers or
  equivalent shared handler functions.
- Linked-server resolver and allowlisted ordinary HTTP/JSON forwarding helper.
- Bounded refusal states for unknown, auth-required, tunnel-required, or
  unreachable linked servers.
- Upstream status/error preservation where practical.
- Rewriting returned `DashboardResourcesView` and nested `ResourcePath.serverId`
  values to the selected linked-server id.
- Tests proving protected-route auth, local alias equivalence, refusal states,
  bearer forwarding on at least one remote route, upstream error preservation,
  and resource-view rewriting.

Deferred:

- Full operation route coverage.
- SSE forwarding for document or Activity event streams.
- Terminal WebSocket gatewaying.
- Remote root picker/open WorkRoot browser dogfood; that is Phase 3.
- Credential persistence, deployment automation, and public endpoint hardening.

## Caller-Visible Contract

Authenticated callers may use canonical `/api/dashboard/servers/{serverId}/...`
one-shot routes for the skeleton operations. Calls for `server-local` behave
like the existing local routes. Calls for linked servers either forward to the
remembered daemon endpoint using the daemon-lifetime bearer token or return a
bounded gateway refusal/error when the linked server cannot be used.

The gateway must not become a generic proxy. Only explicitly allowlisted
dashboard operation paths may forward. Forwarded `DashboardResourcesView`
responses must be rewritten so the returned server and nested resource paths
identify the selected linked server, not the upstream daemon's local identity.

## Contract Instructions

Reuse existing protected router/auth wiring in
`ws-dashboard/crates/daemon/src/router.rs`; do not add routes outside the
protected dashboard API boundary.

Reuse existing linked-server registry, selected-server resource forwarding,
link-auth token memory, and refusal patterns before adding new state.

Prefer extracting shared local handler functions only where needed to avoid
duplicating behavior between legacy local routes and new `server-local` aliases.

Keep the forwarding helper allowlisted. It may support ordinary HTTP/JSON
requests and response bodies needed by the skeleton, but it must reject or
avoid SSE and WebSocket paths.

## Integration Test Instructions

Add or extend Rust daemon route tests around server-scoped route behavior.
Focused filters should include one or more of:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml linked_server
cargo test --manifest-path ws-dashboard/Cargo.toml forwarding
```

Final verification should include:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:root-picker
npm --prefix ws-dashboard/frontend run test:open-work-root
```

## Implementation Strategy Decisions

- Implement the skeleton before broad coverage.
- Route browser-to-remote traffic through the local gateway only.
- Preserve old local routes as compatibility aliases.
- Keep owner-auth at the local gateway and bearer auth only for upstream linked
  daemon calls.
- Treat SSE and WebSocket forwarding as separate future mechanisms.

## Rejected Alternatives

- Generic unrestricted proxy: rejected because it can expose private or future
  daemon paths outside the ticket contract.
- Full API inventory coverage in Phase 2: rejected because later phases own
  operation-specific behavior and verification.
- Browser direct calls to linked endpoints: rejected by product model and
  existing linked-server auth constraints.

## Approach

- Survey existing linked-server resource forwarding and refusal code.
- Introduce a small resolver/forwarding abstraction for allowlisted one-shot
  dashboard routes.
- Add representative server-scoped route aliases and local dispatch for
  `server-local`.
- Add linked-server route tests with fake upstream behavior for bearer
  forwarding, upstream error preservation, and resource-view rewrite.
- Run focused tests first, then the full dashboard cargo suite and frontend
  route/build smoke commands.

## Constraints

- All written code comments and docs must be English.
- Do not touch Phase 3+ remote browser behavior unless a minimal compile/test
  adjustment is required by the Phase 2 skeleton.
- Do not claim remote operation transparency from this phase; it creates the
  backend skeleton that later phases attach to.

## Survey References

### Must

- `ai-docs/tickets/ready/260525-feat-ws-dashboard-server-scoped-operation-forwarding.md`
- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ws-dashboard/crates/daemon/src/router.rs`
- `ws-dashboard/crates/daemon/src/resources.rs`
- `ws-dashboard/crates/daemon/src/root_picker.rs`
- `ws-dashboard/crates/core/src/view_model.rs`
- `ws-dashboard/crates/core/src/resources.rs`

### Maybe

- `ws-dashboard/crates/daemon/src/work_root_files.rs`
- `ws-dashboard/crates/daemon/src/work_root_activity.rs`
- `ws-dashboard/crates/daemon/src/terminal.rs`
- `ai-docs/tickets/idea/260514-research-ws-web-dashboard-direction.md`
