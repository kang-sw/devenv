# Brief: 260514-feat-ws-web-daemon-foundation

## Intent

Implement Phase 1 of the dashboard daemon foundation: replace the placeholder
daemon with an auth-gated Rust/Axum server shell that starts through
`ws-dashboard serve`, binds to loopback by default, exposes a one-time pairing
URL, gates health and placeholder UI behind an owner session cookie, and leaves
later dashboard panels out of scope.

## Approach

- Implement the recorded Phase 1 skeleton rather than changing the public shape.
- Keep the binary thin: CLI parsing and runtime setup delegate into the daemon
  library.
- Use in-memory owner auth for this phase: startup-generated token, one-time
  pairing, owner session cookie, and request auth gate.
- Build an Axum router where `/pair` is the only unauthenticated browser route;
  `/healthz` and `/` require owner auth.
- Keep the health response minimal and static.
- Start the server on `127.0.0.1` by default and expose startup pairing URL
  after binding.
- Convert skeleton route/server tests from ignored targets into executable
  assertions where Phase 1 behavior is implemented.

## Constraints

- Scope is Phase 1 only. Do not implement Phase 2 token TTL/persistence,
  complete Host/Origin policy, WebSocket endpoints, Phase 3 public bind modes,
  PTY terminals, workspace discovery, wsstate view-model APIs, or frontend
  package setup.
- Localhost is not authorization. Loopback binding is only a reachability
  default.
- `/pair` is the only unauthenticated browser route.
- Browser authentication must use an HTTP-only session cookie. A narrow bearer
  path may be left for later CLI/test use only if it does not replace cookie
  auth.
- The daemon must not become the canonical ws MCP root, harness selector, model
  backend, or named-agent session owner.
- Request logging must not leak pairing tokens from query strings.

## Out of scope

- Durable session storage, pairing token expiry, full origin/host policy,
  public bind support, WebSocket protocol behavior, PTY process lifecycle,
  workspace/resource APIs, named-agent panels, and real frontend assets.

## Details

- Existing skeleton commit: `882ded7`.
- Implement `ServeConfig::default_loopback` and `ServeConfig::from_args` while
  rejecting unsupported non-loopback public bind behavior for Phase 1.
- Implement `OwnerAuthState` with safe shared mutable state suitable for Axum
  cloning.
- Implement pairing so a valid `/pair?token=<token>` consumes the startup token
  and returns a response with an owner session cookie.
- Implement middleware or route guards so unauthenticated `/healthz` and `/`
  return `401`.
- Implement `startup_info` so tests can validate pairing URL construction
  without opening sockets.
- Implement `server::run` with listener bind, startup output, router serving,
  and graceful shutdown hook.
- Replace ignored scaffold tests with passing tests for the Phase 1 behavior.
- Run at least `cargo test -p ws-dashboard-daemon` and `cargo test --workspace`.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - planned caller-visible daemon foundation behavior.
- [Must] `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md` - Phase 1 scope, constraints, and recorded skeleton hash.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - daemon must stay separate from ws MCP authority and session-root semantics.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - plugin/runtime ownership boundaries around the dashboard shell.
- [Maybe] `ai-docs/mental-model/named-agent-runtime.md` - boundary guardrail if named-agent ownership questions arise.
