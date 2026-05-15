# Survey: 15-260514-feat-ws-web-daemon-foundation-phase-2

## Reusable Components
- `ws-dashboard/crates/daemon/src/auth.rs#L7-L10` — auth constants: cookie name, 32-byte secret size, and default 5-minute pairing TTL live beside the auth state.
- `ws-dashboard/crates/daemon/src/auth.rs#L11-L23` — `OwnerAuthState`/`AuthInner`: cloned Axum state shares one pairing token, a consumed flag, and the owner session secret through `Arc<Mutex<_>>`.
- `ws-dashboard/crates/daemon/src/auth.rs#L34-L39` and `ws-dashboard/crates/daemon/src/auth.rs#L156-L168` — `PairingTokenPolicy`: explicit TTL policy with `default()` and `new(Duration)`; current constructor accepts it but does not yet store/enforce it.
- `ws-dashboard/crates/daemon/src/auth.rs#L41-L55` — `PairingOutcome`/`AuthRejection`: status vocabulary already includes `Expired` and `Forbidden` for Phase 2 route behavior.
- `ws-dashboard/crates/daemon/src/auth.rs#L57-L92` — `OwnerAuthState::new_ephemeral*` and `consume_pairing_token`: central startup/test construction and one-time token consumption path using `random_secret()`.
- `ws-dashboard/crates/daemon/src/auth.rs#L99-L124` — `authenticate_headers`: existing cookie auth parser rejects before pairing succeeds and returns `401` for missing/invalid owner cookie.
- `ws-dashboard/crates/daemon/src/auth.rs#L126-L153` — bearer and entrypoint auth seams: `issue_bearer_token`, `authenticate_browser_entrypoint`, and `authenticate_websocket_upgrade` are the skeleton extension points for bearer, Host/Origin, and pre-upgrade gating.
- `ws-dashboard/crates/daemon/src/auth.rs#L176-L193` — request/set-cookie and bearer header formatters: tests can reuse `OwnerSessionCookie::as_request_cookie_header`, `as_set_cookie_header`, and `BearerAuthToken::as_authorization_header`.
- `ws-dashboard/crates/daemon/src/router.rs#L19-L33` — `build_router`: `/pair` is the only unprotected route; `/healthz`, `/`, and fallback are merged behind `require_owner_auth`.
- `ws-dashboard/crates/daemon/src/router.rs#L60-L81` — `require_owner_auth`: middleware already branches WebSocket `Upgrade` requests to `authenticate_websocket_upgrade` before handler execution.
- `ws-dashboard/crates/daemon/tests/routes.rs#L36-L73` — route test helpers: `pair_and_cookie` and `pair_response` centralize pairing requests and Set-Cookie extraction.
- `ws-dashboard/crates/daemon/src/server.rs#L31-L39` — startup path: daemon creates `OwnerAuthState::new_ephemeral()`, emits one pairing URL, and injects auth into router state.

## Existing Patterns
- Protected router pattern: see `ws-dashboard/crates/daemon/src/router.rs#L23-L32` — add or keep browser surfaces inside the protected router rather than beside `/pair`.
- Pairing failure responses: see `ws-dashboard/crates/daemon/src/router.rs#L39-L57` and `ws-dashboard/crates/daemon/tests/routes.rs#L151-L173` — missing/invalid/reused failures return explicit non-OK statuses and must not set `Set-Cookie`.
- Axum integration tests: see `ws-dashboard/crates/daemon/tests/routes.rs#L75-L149` — tests call `build_router` with `tower::ServiceExt::oneshot`, not sockets.
- Phase 2 skeleton activation targets: see `ws-dashboard/crates/daemon/tests/routes.rs#L175-L264` — three ignored tests cover zero-TTL expiry, invalid Host/Origin rejection, and bearer access without cookies.
- WebSocket pre-upgrade boundary: see `ws-dashboard/crates/daemon/tests/routes.rs#L267-L285` — `/ws` has no endpoint behavior, but Upgrade requests are expected to fail auth before any upgrade acceptance.
- Minimal health contract: see `ws-dashboard/crates/daemon/src/router.rs#L83-L85` and `ws-dashboard/crates/daemon/tests/routes.rs#L287-L329` — authenticated `/healthz` must remain exactly `ok\n` and secret-free.
- Startup logging split: see `ws-dashboard/crates/daemon/src/server.rs#L36-L37` — the pairing URL is printed for the owner while structured tracing logs only the bound address.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/src/router.rs#L13-L17` — `AppState`: router state carries both `ServeConfig` and `OwnerAuthState`; route tests construct this directly.
- `ws-dashboard/crates/daemon/src/auth.rs#L76-L97` — public pairing/session methods: `pairing_token`, `consume_pairing_token`, and `issue_session_cookie` are used by router and tests.
- `ws-dashboard/crates/daemon/src/auth.rs#L134-L153` — browser/WebSocket auth methods: middleware expects `Result<(), AuthRejection>` and maps rejections through `AuthRejection::status_code`.
- `ws-dashboard/crates/daemon/src/auth.rs#L195-L202` — `AuthRejection::status_code`: only `401 Unauthorized` and `403 Forbidden` are currently modeled.
- `ws-dashboard/crates/daemon/src/config.rs#L8-L35` — `ServeConfig`: default loopback and Phase 1 non-loopback fail-closed behavior remain the serving-config boundary for this phase.
- `ws-dashboard/crates/daemon/src/cli.rs#L20-L33` — `ServeArgs`: public CLI surface already exposes host, port, and static-dir only; no Phase 2 CLI knobs exist.
- `ws-dashboard/crates/daemon/tests/server.rs#L23-L34` — startup-info test asserts pairing URL includes the exposed token; changing token exposure affects this test.

## Constraints
- `ws-dashboard/crates/daemon/src/auth.rs#L62-L73` — `new_ephemeral_with_policy` currently ignores its `PairingTokenPolicy`; zero-TTL test behavior depends on making policy-backed construction real without adding sleeps.
- `ws-dashboard/crates/daemon/src/auth.rs#L80-L92` — token consumption checks `pairing_consumed` before candidate validity, so any post-consumption request currently reports `AlreadyUsed` even for a different candidate.
- `ws-dashboard/crates/daemon/src/auth.rs#L99-L106` — cookie auth is disabled until a pairing succeeds; bearer auth must account for that if it is meant to work before browser pairing.
- `ws-dashboard/crates/daemon/src/auth.rs#L126-L132` — `issue_bearer_token` is an `unimplemented!` panic and no `Authorization` parsing exists in `authenticate_headers` yet.
- `ws-dashboard/crates/daemon/src/auth.rs#L138-L141` — skeleton explicitly flags the exact local Host/Origin allowance as ambiguous; the docs only require rejecting clearly invalid browser entrypoints without breaking loopback usage.
- `ws-dashboard/crates/daemon/src/router.rs#L66-L74` — WebSocket routing is detected only by an `Upgrade: websocket` header and then falls back to existing auth methods; there is no WebSocket route or payload scope in this phase.
- `ai-docs/mental-model/ws-web-dashboard.md#L21-L24` — loopback is not authorization; cloned auth state must share one-time consumption; public/tunnel binding must stay fail-closed until later bind-mode work.
- `ai-docs/tickets/ready/260514-feat-ws-web-daemon-foundation.md#L123-L145` — Phase 3 bind-mode guards and Phase 4 security smoke remain unresolved but excluded from the Phase 2 brief.

## Opinion
- The implementation surface is compact and intentionally centered on `auth.rs`, `router.rs`, and `tests/routes.rs`; most risk is in choosing conservative Host/Origin acceptance without accidentally making loopback presence an authorization substitute.
- The skeleton already exposes the right seams, but bearer auth semantics are underspecified relative to the current `pairing_consumed` gate, so tests should reveal whether the bearer path is startup-issued and usable before any browser pairing.
- No wrong-assumption doc was found during the survey; the only ambiguity is already called out in the skeleton comment for Host/Origin allowance.
