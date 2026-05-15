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
with an explicit expiry policy and exposes the corresponding pairing URL to the
local owner through startup output. The pairing route is the only
unauthenticated browser entrypoint. A successful pairing exchange consumes the
token, installs an HTTP-only owner session cookie with `SameSite=Lax`, and lets
the browser access authenticated routes. Missing, invalid, reused, or expired
pairing tokens fail without installing a session cookie.

Authenticated owner sessions have broad host-control authority for dashboard
features, but the daemon remains separate from ws MCP stdio session authority.
The daemon must not make itself the canonical ws MCP root, harness, model
backend, or named-agent session owner.

HTTP routes other than pairing reject unauthenticated requests before handler
execution, including the health route, the placeholder UI route, and fallback
paths. Browser-facing authentication uses a normal HTTP-only session cookie.
The daemon also accepts a narrow bearer authentication path for CLI and smoke
callers against protected HTTP routes; bearer auth supplements browser cookie
navigation and does not replace it.

Browser entrypoints reject clearly invalid Host and Origin values while
preserving ordinary loopback development usage. Future WebSocket upgrade
requests enter the owner-auth gate before any upgrade acceptance; the foundation
does not yet expose WebSocket endpoint behavior.

Serving configuration supports explicit `local`, `tunnel`, and `public` bind
modes. Local mode remains the default and binds to loopback unless the caller
changes the host. Tunnel mode preserves loopback-oriented serving intent for
external tunnel frontends. Non-loopback hosts such as `0.0.0.0` are rejected
unless the caller explicitly selects public mode. Public mode can accept a
non-loopback host only while owner authentication is enabled; bind-mode
acceptance does not relax browser cookie auth, bearer auth, Host/Origin checks,
or WebSocket pre-upgrade auth.

The initial UI route serves a minimal placeholder surface behind owner
authentication. Health output is the exact minimal body `ok\n`; host paths,
cache paths, Git roots, pairing tokens, session values, diagnostics, and wsstate
internals are not URL identity and are not exposed by the health surface.

## Core Resource Vocabulary {#260516-ws-web-dashboard-core-resource-vocabulary}

The dashboard core crate exposes opaque ids and resource path vocabulary for
the first visible hierarchy without exposing host paths as identity. Core
resource paths carry `serverId`, `workspaceId`, `workRootId`, and optional
`instanceId` fields when serialized for dashboard consumers.

The physical directory target is named `workRoot` in the public dashboard core
vocabulary. WorkRoot metadata distinguishes `plainDirectory`, `gitPrimaryRoot`,
and `gitLinkedWorktree`; status values describe whether the remembered root is
online, offline, moved, or inaccessible. Main/sub instance role, instance kind,
and interaction mode values serialize with the same dashboard camelCase naming
contract.

## 🚧 Resource View-Model Contract {#260516-ws-web-dashboard-resource-view-model-contract}

The dashboard will expose authenticated HTTP view-model APIs for the first
visible resource hierarchy:

```text
server -> workspace -> workRoot -> mainInstance -> subInstance
```

Callers will address API resources through opaque ids rather than host paths,
Git roots, wsstate paths, workRoot keys, or runtime session identifiers. The
daemon will own those private identifiers and expose only authenticated
view-model fields that the browser needs to render navigation, selection,
status, stale/error/loading state, and available actions.

`workspace` will mean a daemon-discovered project group, not a user-created
category. `workRoot` will mean the physical directory used as an open, spawn,
and run target. WorkRoots will report additive kind metadata for
`plainDirectory`, `gitPrimaryRoot`, or `gitLinkedWorktree`, plus status such as
online, offline, moved, or inaccessible. Primary roots and linked Git worktrees
will share the same core workRoot API shape while preserving enough metadata
for the UI to distinguish their repository role and lifecycle affordances.

The API shape will preserve the full hierarchy even when the browser later
renders singleton `workspace -> workRoot -> mainInstance` chains as compact
rows. Authenticated callers may observe compactability hints, but compaction is
a presentation policy and not URL identity.

## 🚧 Mock View-Model Fixtures {#260516-ws-web-dashboard-mock-view-model-fixtures}

The dashboard daemon will provide deterministic fixture-backed resource data
that uses the same view-model API contract as live providers. Frontend and
contract tests will be able to render the first visible shell without live
wsstate, PTY, named-agent, harness, or filesystem discovery dependencies.

Fixtures will cover singleton chains, multi-root workspaces, plain directories,
Git primary roots, linked Git worktrees, offline or inaccessible workRoots,
main instances, sub instances, stale/error/loading states, and visible action
hints. Protected API route tests will verify that fixture-backed dashboard data
remains behind the owner-auth boundary.
