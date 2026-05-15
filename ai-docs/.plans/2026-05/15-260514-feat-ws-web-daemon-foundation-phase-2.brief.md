# Brief: 260514-feat-ws-web-daemon-foundation Phase 2

## Intent

Complete owner session authentication for the dashboard daemon foundation. The
Phase 1 shell already gates non-pair routes behind owner auth; this phase
hardens the auth path with pairing-token expiry, clear failed pairing behavior,
cookie reuse across browser requests, narrow bearer auth for CLI/smoke callers,
future WebSocket pre-upgrade auth gating, and conservative Host/Origin checks.

## Approach

- Implement the Phase 2 skeleton contracts from commits `b0a846b` and
  `3964c61`.
- Keep `ws-dashboard serve` and router construction shape intact.
- Preserve `/pair` as the only unauthenticated browser route.
- Normalize `OwnerAuthState::new_ephemeral` through policy-backed construction
  with high-entropy startup secrets and deterministic zero-TTL test behavior.
- Enforce expired, invalid, reused, and missing pairing failures without setting
  a session cookie.
- Keep browser navigation cookie-based while allowing a narrow bearer token path
  for smoke/CLI HTTP callers.
- Apply Host/Origin checks before browser route handlers and before future
  WebSocket upgrade acceptance.
- Convert Phase 2 ignored skeleton tests into active passing tests when the
  corresponding behavior is implemented.

## Constraints

- Scope is Phase 2 only. Do not implement Phase 3 local/tunnel/public bind-mode
  guards.
- Do not add PTY terminals, workspace/resource APIs, named-agent panels,
  dashboard feature panels, or frontend package setup.
- Loopback binding is not authorization; every browser route except `/pair`
  still requires owner auth.
- Browser auth must continue to use an HTTP-only session cookie with
  `SameSite=Lax`; bearer auth supplements it only for CLI/smoke callers.
- Health output must remain exactly minimal and secret-free.
- Request handling and logs must not expose pairing tokens, session identifiers,
  host paths, cache paths, Git roots, wsstate internals, or diagnostics.
- Host/Origin checks should reject clearly invalid browser entrypoints without
  breaking ordinary loopback developer usage.

## Out of scope

Public bind modes, durable session storage, RBAC, public-internet hardening
beyond this auth boundary, WebSocket endpoint payload behavior, PTY process
lifecycle, workspace discovery, wsstate view-model APIs, and real frontend
assets.

## Details

- Existing Phase 2 skeleton final commit: `b0a846b`.
- Ticket skeleton frontmatter update: `3964c61`.
- Implement in `ws-dashboard/crates/daemon/src/auth.rs` and
  `ws-dashboard/crates/daemon/src/router.rs`.
- Integration targets live in `ws-dashboard/crates/daemon/tests/routes.rs`.
- The key tests to activate and pass are:
  - `expired_pairing_tokens_do_not_install_sessions`
  - `browser_auth_rejects_invalid_host_and_origin_with_owner_cookie`
  - `bearer_auth_can_access_http_smoke_routes_without_cookie`
- Keep `websocket_upgrade_requests_are_auth_gated_before_upgrade_acceptance`
  passing as the pre-upgrade boundary; do not add a real WebSocket endpoint.
- Run at least `cargo test -p ws-dashboard-daemon` and `cargo test --workspace`.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - daemon foundation owner-auth
  and pairing/session contract.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - daemon entrypoints,
  owner-auth boundary, protected routes, and server/router coupling.
- [Must] `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md` -
  Phase 2 scope only; later unresolved phases are excluded.
