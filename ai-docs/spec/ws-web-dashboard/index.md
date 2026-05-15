---
title: ws Web Dashboard
summary: Personal ws-aware web dashboard daemon, browser UI, and host-control behavior.
---

# ws Web Dashboard

The ws web dashboard provides a personal browser-accessible control plane for a
host machine. It serves dashboard UI, gates host-control actions behind owner
authentication, and consumes ws runtime state through daemon-owned view models.

## Daemon Foundation {#260515-ws-web-daemon-foundation}

The dashboard daemon starts through the `ws-dashboard serve` command as a
Rust/Axum HTTP server with explicit serving configuration, structured startup
logging, graceful shutdown, and a minimal health surface. The default bind
target is `127.0.0.1`. The daemon does not treat loopback access as
authorization.

On startup, the daemon creates an in-memory high-entropy one-time pairing token
and exposes the corresponding pairing URL to the local owner through startup
output. The pairing route is the only unauthenticated browser entrypoint. A
successful pairing exchange consumes the token, installs an HTTP-only owner
session cookie with `SameSite=Lax`, and lets the browser access authenticated
routes.

Authenticated owner sessions have broad host-control authority for dashboard
features, but the daemon remains separate from ws MCP stdio session authority.
The daemon must not make itself the canonical ws MCP root, harness, model
backend, or named-agent session owner.

HTTP routes other than pairing reject unauthenticated requests before handler
execution, including the health route, the placeholder UI route, and fallback
paths. Browser-facing authentication uses a normal HTTP-only session cookie.

The daemon rejects non-loopback serving hosts in this foundation shell rather
than partially enabling public bind behavior. Binding to a public interface such
as `0.0.0.0` remains unavailable until explicit public-bind guardrails exist.

The initial UI route serves a minimal placeholder surface behind owner
authentication. Health output is the exact minimal body `ok\n`; host paths,
cache paths, Git roots, pairing tokens, session values, diagnostics, and wsstate
internals are not URL identity and are not exposed by the health surface.

> [!note] Planned 🚧
> Future daemon foundation phases will add durable owner-auth hardening such as
> token expiry, complete Host/Origin checks, WebSocket upgrade authentication
> surfaces, and explicit local/tunnel/public bind-mode guardrails.
