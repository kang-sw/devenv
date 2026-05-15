---
title: ws Web Dashboard
summary: Personal ws-aware web dashboard daemon, browser UI, and host-control behavior.
---

# ws Web Dashboard

The ws web dashboard provides a personal browser-accessible control plane for a
host machine. It serves dashboard UI, gates host-control actions behind owner
authentication, and consumes ws runtime state through daemon-owned view models.

## 🚧 Daemon Foundation {#260515-ws-web-daemon-foundation}

The dashboard daemon starts as a Rust/Axum HTTP server with explicit serving
configuration, structured logging, graceful shutdown, and a minimal health
surface. The default bind target is loopback. The daemon does not treat
loopback access as authorization.

On startup, the daemon creates a high-entropy one-time pairing token and exposes
the corresponding pairing URL to the local owner through startup output or a
future open command. The pairing route is the only unauthenticated browser
entrypoint. A successful pairing exchange consumes the token, installs an
owner session cookie, and lets the browser access authenticated routes.

Authenticated owner sessions have broad host-control authority for dashboard
features, but the daemon remains separate from ws MCP stdio session authority.
The daemon must not make itself the canonical ws MCP root, harness, model
backend, or named-agent session owner.

HTTP routes other than pairing reject unauthenticated requests. WebSocket
upgrades also require an authenticated owner session before the connection is
accepted. Browser-facing authentication uses a normal HTTP-only session cookie;
bearer-style credentials may exist for CLI or smoke-test callers but do not
replace the browser session cookie.

The daemon validates request host and origin information for browser entrypoints
before granting owner access. Local and tunnel modes prefer `127.0.0.1`.
Binding to a public interface such as `0.0.0.0` requires explicit opt-in and
fails closed unless owner authentication is enabled.

The initial static UI serving path may serve built frontend assets or a minimal
placeholder surface, but it remains behind owner authentication. Host paths,
cache paths, Git roots, and wsstate internals are not URL identity and are not
exposed by the health surface.
