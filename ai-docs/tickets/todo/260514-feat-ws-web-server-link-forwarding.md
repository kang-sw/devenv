---
title: ws web server link and forwarding
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260514-feat-ws-web-remote-wsl-hardening: validates linked-server behavior across WSL and remote deployments
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# ws web server link and forwarding

## Background

The dashboard may need to show multiple host environments as one seamless
control plane: for example, a native Windows daemon viewing a WSL daemon, or a
local daemon viewing a remote server daemon through an authenticated link.

Native Windows code can invoke WSL-exposed tools to inspect or control Linux
state, but treating that as the primary integration path would be brittle. The
preferred direction is to run a ws web daemon in each environment and link
daemons through an authenticated forwarding relationship.

The forwarding model should preserve the same resource shape used by local
state: `/api/servers/:serverId/workspaces/:workspaceId/...` resolves locally
for the local server and forwards to a linked daemon for linked server ids.
Running terminal, editor, agent, and task resources under a workspace are
instances, not sessions.

This ticket needs further design discussion before promotion to `ready`,
especially around trust, routing, and which actions are forwarded versus kept
local.

## Phases

### Phase 1: Define linked-server identity and pairing

Define how one daemon registers another daemon, how owner authentication applies
across the link, how linked servers get stable opaque ids, and how stale or
revoked links are handled.

### Phase 2: Add forwarding transport

Add a transport for forwarding selected API calls, event streams, and terminal
or agent views from a child daemon to a parent dashboard without exposing the
child daemon publicly. The parent should route by `serverId` rather than
requiring feature panels to know whether a resource is local or linked.

### Phase 3: Add multi-server frontend model

Represent linked servers in the frontend workspace model so panels can show
local, WSL, and remote resources with clear provenance while preserving a
seamless dashboard experience, shared resource keys, and server-grouped flat
workspace navigation.

### Phase 4: Verify WSL and remote link behavior

Verify the linked-daemon model for Windows-to-WSL, local-to-remote tunnel, link
loss, reconnect, authentication failure, and action routing.
