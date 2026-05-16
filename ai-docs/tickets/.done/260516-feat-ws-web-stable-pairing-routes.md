---
title: ws web dashboard stable pairing routes
completed: 2026-05-16
parent: 260516-epic-ws-web-dashboard-workbench-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: containing workbench substrate epic
  260514-research-ws-web-dashboard-direction: route identity and pairing direction
spec:
  - 260516-ws-web-dashboard-token-free-pairing-landing
  - 260516-ws-web-dashboard-server-scoped-browser-routes
plans:
  phase-1: 2026-05/16-260516-feat-ws-web-stable-pairing-routes-phase-1.brief
  phase-2: 2026-05/16-260516-feat-ws-web-stable-pairing-routes-phase-2.brief
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard stable pairing routes

## Background

The daemon currently prints a one-time pairing URL as the owner entrypoint.
That URL should remain the startup entrypoint, but successful pairing should
move the browser to a stable token-free app URL backed by the owner session
cookie. The frontend route shape should also reserve explicit server identity
through `/servers/:serverId/...` paths instead of encoding server context
inside workspace, workRoot, or instance ids.

## Decisions

- Keep `/pair?token=...` as a one-time entrypoint and never make the token part
  of normal refresh-safe navigation.
- Successful pairing redirects to a token-free app URL after installing the
  owner cookie.
- Missing, invalid, reused, or expired pairing tokens fail without redirecting
  into an authenticated-looking app route.
- Browser routes should explicitly include `/servers/:serverId/...` for
  server-scoped dashboard resources.

## Phases

### Phase 1: Token-Free Pairing Landing

Update the pairing flow so a successful token exchange installs the existing
owner session cookie and redirects the browser to a token-free stable app URL.
Preserve one-time token consumption, expiry handling, bearer-auth behavior for
smoke callers, and protected-route rejection before authentication.

### Result (679f3fe) - 2026-05-16

Updated the browser pairing route so a valid `/pair?token=...` request consumes
the token, installs the owner cookie, and returns `303 See Other` to `/`
without carrying the token forward. Failure paths for missing, invalid, reused,
and expired tokens remain non-redirecting and cookie-free.

Verified with `cargo test --manifest-path ws-dashboard/crates/daemon/Cargo.toml
--test routes --test server` and `cargo test --workspace` from
`ws-dashboard/`. Correctness and test review reported clean. Phase 2
`/servers/:serverId/...` route identity was not touched.

### Phase 2: Server-Scoped Browser Route Basis

Introduce the frontend route identity basis for `/servers/:serverId/...` while
keeping daemon-owned opaque resource ids as the source of truth. The first pass
does not need complete deep-link coverage, but it should make refresh-safe,
server-scoped navigation the intended route shape for later workbench surfaces.

### Result (9461754) - 2026-05-16

Added the first server-scoped browser route basis. The frontend now normalizes
`/`, `/servers`, and `/servers/:serverId...` against the daemon-reported
`resources.server.id`, preserving daemon resource data as the authority rather
than treating route params as workspace, workRoot, or instance identity.

The daemon serves the protected frontend shell for `/servers` and
`/servers/{*app_path}` refreshes through the existing owner-auth static route
boundary. Follow-up test coverage in `6ed9528` added exact `/servers` fallback
coverage and frontend route-normalization assertions for `/`, `/servers`,
wrong server ids, already-scoped routes, and query/hash preservation.

Verified with `cargo fmt --manifest-path crates/daemon/Cargo.toml`, frontend
build plus `npm run test:routes`, daemon route tests, and daemon server tests.
Correctness, fit, and test review reported clean after the test coverage
follow-up.
