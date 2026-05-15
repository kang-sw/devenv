---
title: ws web daemon foundation
parent: 260514-epic-ws-web-dashboard-mvp
spec:
  - 260515-ws-web-daemon-foundation
skeletons:
  phase-1: 882ded7
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# ws web daemon foundation

## Background

The web dashboard needs a local-first daemon before feature panels can attach to
host state. The daemon is the host-control boundary for HTTP, WebSocket, static
UI serving, pairing, sessions, and bind-mode safety.

The initial `ws-dashboard/` scaffold already exists with `core`,
`harness-core`, `harness-cli`, `daemon`, and `frontend` slots. That scaffold
preserves the planned source layout but does not complete the daemon
foundation. The current daemon binary is only a placeholder, so this ticket
creates the first real caller-visible dashboard server contract.

## Decisions

- Treat loopback binding as a reachability limit, not an authorization boundary.
  Localhost access still requires owner authentication.
- Build the initial server as the final daemon skeleton rather than a temporary
  mock server. Router construction, configuration parsing, logging, shutdown,
  auth middleware shape, and smoke-test harnesses should remain useful for
  later dashboard panels.
- Use a one-time startup pairing token to bootstrap browser access. The pairing
  route is the only unauthenticated browser entrypoint; successful pairing
  installs a normal owner session cookie.
- Keep browser authentication cookie-based. Bearer-style credentials may be
  accepted for CLI or smoke-test callers, but they do not replace the browser
  session cookie because normal browser navigation and static asset requests do
  not reliably carry custom authorization headers.
- Keep the daemon separate from ws MCP authority. It may later expose ws runtime
  view models, but it must not become the canonical MCP session root, harness
  selector, model backend, or named-agent session owner.

## Constraints

- Default bind is `127.0.0.1`. Public interface binding such as `0.0.0.0`
  requires explicit opt-in and must fail closed unless owner authentication is
  enabled.
- `/pair` is the only unauthenticated browser route. Health, static UI, future
  API routes, and WebSocket upgrades require an authenticated owner session.
- Health responses stay minimal. They must not expose host paths, cache paths,
  Git roots, wsstate internals, pairing tokens, session identifiers, or process
  diagnostics.
- Host and Origin validation are part of the owner-auth boundary for browser
  entrypoints.
- Static UI serving can be minimal while the frontend package is still a
  placeholder, but it must exercise the authenticated route path.
- Multi-user accounts, RBAC, public internet hardening beyond fail-closed bind
  behavior, PTY terminals, workspace discovery, and wsstate view-model APIs are
  out of scope for this foundation ticket.

## Prior Art

- Reuse the existing `ws-dashboard/crates/daemon` binary slot for the Axum
  entrypoint.
- Keep resource naming aligned with the existing `ws-dashboard/crates/core`
  opaque id and resource primitives, but do not expose workspace or instance
  APIs in this ticket.
- Preserve the future harness split in `ws-dashboard/crates/harness-core`; owner
  authentication should not depend on harness backend behavior.

## Phases

### Phase 1: Add auth-gated daemon shell

Create the Rust/Axum daemon entrypoint, CLI serving configuration, loopback
default bind, router construction, structured logging, graceful shutdown, and
minimal health/static routes. Add the auth middleware shape during this phase:
all browser routes except pairing must pass through an owner-auth check, even if
some session internals remain in-memory for the foundation.

Success criteria:

- `ws-dashboard serve` or the chosen equivalent starts an Axum server on
  `127.0.0.1` by default.
- Startup output gives the local owner a one-time pairing URL.
- `/pair` is reachable without an existing session and consumes the startup
  token when valid.
- `/healthz` and static UI serving reject unauthenticated requests and succeed
  after pairing.
- Server startup, shutdown, and request logging are covered by focused tests or
  smoke checks.

### Phase 2: Complete owner session authentication

Finish the conservative owner-auth path around the Phase 1 skeleton. Pairing
tokens must be high entropy, single-use, and time-limited. Successful pairing
installs an HTTP-only owner session cookie suitable for normal browser
navigation. HTTP request auth should also support a narrow bearer-style path
only where useful for CLI or smoke-test callers.

Success criteria:

- Invalid, reused, missing, or expired pairing tokens fail without installing a
  session.
- Authenticated owner sessions can reuse the cookie across ordinary browser
  requests.
- Unauthenticated HTTP requests receive a clear `401` response.
- Future WebSocket routes have an auth gate in place before upgrade acceptance,
  even if no PTY or agent WebSocket endpoint is implemented in this ticket.
- Host and Origin checks reject clearly invalid browser entrypoints without
  weakening local developer usage.

### Phase 3: Add bind-mode guards

Support local, tunnel, and public bind-mode configuration without changing the
auth contract. Local and tunnel modes prefer loopback binding. Public binding
requires explicit opt-in and must fail closed if owner authentication is absent
or disabled.

Success criteria:

- Loopback bind remains the default.
- Public bind attempts require an explicit mode or flag; accidental `0.0.0.0`
  exposure fails with an actionable error.
- Public bind cannot start without owner authentication enabled.
- Bind-mode decisions are covered by tests that do not require public network
  exposure.

### Phase 4: Verify daemon security smoke

Add end-to-end smoke coverage around the foundation boundary.

Success criteria:

- Unauthenticated HTTP and WebSocket paths are rejected.
- Pairing succeeds once, installs a session cookie, and rejects reuse.
- Session reuse works for health and static UI routes.
- Local bind startup succeeds on loopback.
- Public-mode guard failures are tested.
- Health output remains minimal and does not leak host-control internals.
