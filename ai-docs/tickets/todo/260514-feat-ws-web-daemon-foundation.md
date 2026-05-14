---
title: ws web daemon foundation
parent: 260514-epic-ws-web-dashboard-mvp
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# ws web daemon foundation

## Background

The web dashboard needs a local-first daemon before feature panels can attach to
host state. The daemon is the host-control boundary for HTTP, WebSocket, static
UI serving, pairing, sessions, and bind-mode safety.

This ticket is intentionally a substrate ticket. Detailed endpoint names,
storage paths, and packaging choices need follow-up discussion before promotion
to `ready/`.

The initial `ws-dashboard/` scaffold already exists with `core`,
`harness-core`, `harness-cli`, `daemon`, and `frontend` slots. That scaffold
preserves the planned source layout but does not complete Phase 1; the daemon
still needs a real Axum entrypoint, serving modes, health surface, logging, and
lifecycle behavior.

## Phases

### Phase 1: Add daemon shell and serving modes

Create the Rust/Axum daemon entrypoint, dev/static UI serving path, health
surface, structured logging, and basic process lifecycle behavior.

### Phase 2: Add owner authentication

Add a conservative owner-auth path with a one-time pairing URL, session cookie,
HTTP request auth, WebSocket auth, and Origin/Host checks. Authenticated owner
sessions should retain broad host-control authority.

### Phase 3: Add bind-mode guards

Support local, tunnel, and public bind modes. Prefer `127.0.0.1`; require
explicit opt-in for `0.0.0.0`; fail closed if public mode lacks authentication.

### Phase 4: Verify daemon security smoke

Add smoke coverage for unauthenticated HTTP/WebSocket rejection, pairing flow,
session reuse, local bind startup, and public-mode guard failures.
