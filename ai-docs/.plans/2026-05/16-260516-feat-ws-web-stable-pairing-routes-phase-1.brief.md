# Brief: 260516-feat-ws-web-stable-pairing-routes Phase 1

## Intent

Make the dashboard pairing URL a one-time entrypoint only. A successful
browser pairing request should consume the token, install the existing owner
session cookie, and redirect to a stable token-free app URL so refresh and
normal navigation do not keep the token in the address bar.

## Approach

- Update the daemon pairing route behavior for successful browser pairing.
- Preserve existing one-time token consumption, expiry handling, owner cookie
  installation, and protected-route auth.
- Preserve the bearer-auth smoke path and Host/Origin guardrails.
- Add route tests that prove success redirects to a token-free target and
  failure cases do not redirect into an authenticated-looking app URL or
  install an owner cookie.

## Constraints

- Scope is Phase 1 only.
- Do not implement `/servers/:serverId/...` frontend route identity; that is
  Phase 2.
- Do not broaden unauthenticated routes beyond `/pair`.
- Do not change bearer auth semantics.
- Do not change bind-mode, Host, or Origin policy.
- Do not expose the pairing token through logs or stable app URLs.

## Out of scope

- Browser route hierarchy beyond the token-free landing URL.
- New frontend router implementation.
- Persistent auth storage.
- Public deployment hardening.

## Details

Acceptance behavior:

- Valid `/pair?token=...` browser request:
  - consumes the token;
  - installs the existing owner session cookie;
  - returns a redirect to a token-free stable app URL, expected to be `/` unless
    the implementation has an existing safer app landing target;
  - does not include the token in the redirect target.
- Missing, invalid, reused, or expired token:
  - fails without installing an owner cookie;
  - does not redirect to `/` or any authenticated-looking app route.
- Bearer-auth protected HTTP calls continue to work without browser cookie
  pairing.
- Protected routes still reject unauthenticated browser requests.

Verification should run the focused daemon route/server tests affected by the
change, plus a broader Rust workspace test if practical.

## References

- [Must] `ws-dashboard/crates/daemon/src/auth.rs` - one-time pairing token,
  expiry, cookie install, and rejection behavior.
- [Must] `ws-dashboard/crates/daemon/src/router.rs` - `/pair`, protected-route
  auth, Host/Origin guardrails, and static/fallback route placement.
- [Must] `ws-dashboard/crates/daemon/src/server.rs` - startup auth-state wiring
  and pairing URL emission.
- [Must] `ws-dashboard/crates/daemon/tests/routes.rs` - route behavior tests.
- [Must] `ws-dashboard/crates/daemon/tests/server.rs` - startup/server tests.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260515-ws-web-daemon-foundation`,
  `260516-ws-web-dashboard-token-free-pairing-landing`, and
  `260516-ws-web-dashboard-protected-frontend-shell`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard auth/router
  ownership and pairing-change recipe.
- [Maybe] `ws-dashboard/crates/daemon/src/cli.rs`,
  `ws-dashboard/crates/daemon/src/config.rs`, and
  `ws-dashboard/crates/daemon/src/main.rs` - only if startup plumbing requires
  a redirect-target configuration change.
