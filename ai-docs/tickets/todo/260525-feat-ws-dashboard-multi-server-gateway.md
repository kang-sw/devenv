---
title: ws dashboard multi-server gateway
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260513-research-streamable-http-mcp-transport: adjacent long-running daemon transport research
  260514-research-ws-web-dashboard-direction: longer-range dashboard server federation and remote hardening direction
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard multi-server gateway

## Background

The dashboard should promote servers to first-tier product objects while keeping
the browser attached only to the local dashboard daemon. The intended product
shape is a local dashboard daemon acting as a gateway to local, WSL, and linked
remote dashboard daemons.

The first concrete dogfood target is an owner-provided remote Windows SSH host.
Tracked docs should keep that target generic; machine-local details belong in
`ai-docs/_index.local.md`.

## Decisions

- The browser always talks to the local dashboard daemon.
- The local daemon is a gateway, launcher, reconnect manager, and request
  forwarder; it is not the lifecycle owner of linked remote daemons.
- A remote daemon may be started by the local daemon over SSH, but it should
  continue running if the local daemon exits.
- On local daemon restart, persisted endpoint and SSH metadata may be used to
  reconnect or recreate tunnels, but credentials are not restored automatically.
- Credential persistence is explicitly deferred. Users keep remote passphrases
  themselves and re-enter them in the UI when reconnecting.
- SSH is a deploy/start/tunnel transport. Dashboard owner authentication remains
  a separate daemon-level pairing or link-auth concern.
- Remote daemons should prefer remote-loopback bind plus SSH forwarding for the
  MVP. Public remote bind, TLS, and broader hardening belong to later scope.

## Constraints

- Persisted linked-server state may include display name, server id, kind, SSH
  target metadata, endpoint hints, tunnel configuration, last-seen timestamps,
  and bounded capabilities.
- Passphrases, remote session tokens, and active tunnel process details are
  memory-only in the MVP.
- Forwarded APIs must preserve the existing server-scoped resource shape rather
  than inventing separate remote-only routes.
- Server ids remain opaque dashboard ids; host paths, SSH secrets, passphrases,
  and cache paths must not leak into browser routes.

## Phases

### Phase 1: Linked server registry and gateway skeleton

Add daemon-owned linked-server registry support and gateway routing skeletons.
The local server should be one first-tier server entry, and linked servers should
have explicit states such as connected, auth-required, unreachable, starting,
stale-endpoint, or tunnel-required.

Requests under `/servers/:serverId/...` should continue to use the same frontend
resource model. Local requests remain in-process, while linked-server requests
can be forwarded through a bounded gateway adapter once a linked endpoint is
connected.

Deferred scope: SSH deployment, tunnel lifecycle, credential persistence, public
remote bind, and rich UI polish.

Verification should cover registry persistence, local-server compatibility, and
gateway refusal behavior for unauthenticated or unreachable linked servers.

### Phase 2: Remote link authentication and reconnect handshake

Define and implement the remote daemon link-auth handshake. A remote daemon may
print a daemon-lifetime passphrase at startup. The user enters that passphrase
into the local dashboard UI, and the local daemon exchanges it for a memory-only
remote link session token or equivalent authenticated transport state.

The handshake should let the local daemon distinguish auth-required,
wrong-passphrase, stale-endpoint, incompatible-capability, and reachable-but-not-
dashboard-daemon states without exposing raw secrets or daemon internals to the
browser.

Deferred scope: persisted credentials and multi-owner identity.

Verification should include daemon API tests for success and failure states plus
browser coverage for entering a passphrase and observing the resulting server
state transition.

### Phase 3: SSH remote start and tunnel reconnect

Add SSH-backed remote start/reconnect support for owner-provided remote hosts.
The local daemon may use SSH to deploy or locate the dashboard binary, start a
remote daemon, capture bounded stdout startup metadata, and create a local
forward to the remote daemon's loopback endpoint.

The remote daemon must not be terminated merely because the local daemon exits.
On local restart, persisted endpoint and SSH metadata should allow best-effort
tunnel recreation and reconnect attempts. If authentication is missing, the
server should become auth-required rather than disappearing.

Deferred scope: service installation, automatic credential persistence, public
remote exposure, and full remote hardening.

Verification should include at least one remote-dogfood path against an owner-
provided SSH host, with local-daemon restart behavior observed.

### Phase 4: Server-first left navigation

Refactor the left navigation so servers become the top hierarchy. Remove the
current fixed local-server display and separate open-workRoot subsection in
favor of one server/workspace/workRoot tree.

The nav should expose a thin top status row such as `Servers | +`, and each
server group should expose compact server heading actions such as refresh and
open workRoot. Workspace and WorkRoot rows stay nested under their owning
server.

Deferred scope: broad visual-system redesign and remote administration panels.

Verification should include browser coverage for local server parity, linked
server display states, open-workRoot targeting under the selected server, and
workspace/workRoot navigation continuity.
