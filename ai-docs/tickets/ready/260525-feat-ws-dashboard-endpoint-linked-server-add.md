---
title: ws dashboard endpoint-first linked server add
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260525-feat-ws-dashboard-multi-server-gateway: completed gateway substrate this ticket builds on
  260514-research-ws-web-dashboard-direction: longer-range remote hardening and server federation direction
spec:
  - 260525-ws-dashboard-linked-server-registry-gateway-skeleton
  - 260525-ws-dashboard-remote-link-auth-handshake
  - 260525-ws-dashboard-endpoint-linked-server-add
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard endpoint-first linked server add

## Background

The multi-server gateway substrate can list linked servers, establish SSH-
managed tunnels, exchange daemon link passphrases for memory-only bearer
tokens, and forward selected-server resources. The browser now has a server-
first left navigation, but it does not yet have a product-level "Add server"
flow.

The primary user model should not require the dashboard to manage SSH. A user or
owner-directed agent may start a remote `ws-dashboard` daemon directly and may
create any needed tunnel, VPN route, port forward, or local loopback forward
outside the dashboard. From the dashboard gateway perspective, a loopback SSH
tunnel such as `http://127.0.0.1:<port>` is just an endpoint.

## Decisions

- Make endpoint-only linking the primary add-server path.
- Treat a user-managed loopback SSH tunnel as an ordinary endpoint from the
  dashboard's point of view. `http://127.0.0.1:<port>` should not get a
  special browser model merely because SSH created the port forward.
- Treat SSH as an optional advanced or agent-operated transport, not as the
  default user-facing add-server modal.
- The local dashboard daemon remains the browser gateway. The browser submits an
  endpoint and optional daemon-lifetime link passphrase to the local daemon; it
  does not call the remote endpoint directly.
- Endpoint-only linked servers persist non-secret metadata such as server id,
  display label, kind, and endpoint hint. Passphrases and bearer tokens remain
  memory-only.
- The existing SSH start/reconnect API may remain available for advanced
  automation and dogfood, but the basic modal must not require SSH target,
  startup command, or remote deployment details.
- Remote daemon startup, tunnel creation, VPN routing, reverse proxying, and
  public bind hardening are operator responsibilities for this slice. The
  dashboard may document them through `--remote-guide`, but the browser add flow
  should not become a remote deployment wizard.

## Constraints

- Endpoint input may point to a user-managed local loopback tunnel, VPN/private
  network endpoint, or other owner-provided reachable dashboard endpoint.
- The endpoint-only route must work equally for a direct reachable endpoint and
  for a local loopback endpoint produced by an external tunnel.
- The endpoint-only flow must verify that the target behaves like a compatible
  `ws-dashboard` daemon before treating it as connected.
- If a passphrase is provided, the local daemon should immediately exchange it
  through remote `/api/dashboard/link-auth` and store the resulting bearer token
  only in memory.
- If no passphrase is provided or link-auth fails, the linked server should
  remain visible in a bounded `authRequired` or unreachable state with a
  re-enter-passphrase path.
- The UI should communicate that public endpoints are the owner's
  responsibility; TLS, reverse-proxy setup, public bind hardening, and
  credential persistence remain out of scope.
- Server ids remain opaque dashboard ids. Do not expose passphrases, bearer
  tokens, SSH targets, host paths, or cache paths in browser routes.
- The primary add-server modal must not ask for SSH target, remote startup
  command, remote path, tunnel port, or deployment strategy. Those details may
  exist outside the dashboard or in advanced automation, but they are not part
  of the ordinary endpoint registration contract.

## Rejected Alternatives

- Do not make the primary add-server modal manage SSH sessions or expose SSH
  command authoring. That would mix transport setup, remote daemon lifecycle,
  and endpoint registration into one fragile user model.
- Do not require the remote daemon lifetime to be owned by the local dashboard
  daemon. The local daemon may reconnect to remembered endpoint metadata after
  restart, but remote process survival is outside this ticket.
- Do not persist passphrases or bearer tokens for convenience. Re-entry after a
  local daemon restart is expected until a separate credential-storage design is
  accepted.

## Phases

### Phase 1: Endpoint-only linked server API

Add an owner-authenticated daemon route for registering or updating a linked
server from an endpoint without SSH metadata.

Suggested route shape:

```text
POST /api/dashboard/servers/link
```

Suggested request shape:

```json
{
  "serverId": "server-remote-dev",
  "label": "Remote dev",
  "endpoint": "http://127.0.0.1:49170",
  "passphrase": "optional daemon-lifetime passphrase"
}
```

The daemon should normalize and validate the endpoint, reject empty identifiers
or labels, reject unsupported endpoint schemes, probe the target enough to
distinguish unreachable or incompatible targets, persist non-secret linked-
server metadata with `kind: manual`, and attempt link-auth when a passphrase is
provided. Connected servers should immediately support the existing
`/api/dashboard/servers/{serverId}/resources` forwarding path.

Deferred scope: SSH startup, tunnel creation, service installation, persisted
credentials, public exposure hardening, and forwarding of every non-resource
sub-API.

Verification should cover endpoint metadata persistence, wrong passphrase,
unreachable endpoint, successful link-auth, connected resource forwarding, and
restart behavior where the server remains listed but requires passphrase entry
again.

### Phase 2: Add server modal and row actions

Wire the left-nav `Add server` icon to a primary endpoint-first modal. The modal
should default to a compact form with display name, endpoint, and passphrase
fields plus a clear connect action. The modal may include a short non-intrusive
hint that the endpoint can be a user-managed loopback tunnel.

On success, refresh the server list, select the newly linked server, and request
its resources. On auth failure or unreachable endpoint, keep the form open with
bounded feedback that does not expose daemon internals. Existing server row
actions should use the server action hints where practical: connected servers
can refresh, auth-required servers can re-enter a passphrase, and tunnel-
required SSH-managed servers can reconnect when that route is available.

Deferred scope: a full remote deployment wizard, SSH command authoring UI,
credential storage, and broad visual-system redesign.

Verification should include frontend API wrapper tests, modal submit/error
state tests where the existing frontend test substrate supports them, and a
manual dogfood path against an owner-managed local tunnel endpoint.

### Phase 3: SSH advanced-path containment

Audit the existing SSH start/reconnect affordances after endpoint-first linking
lands. Keep SSH as an advanced or agent-operated path, surfaced only where it
does not confuse the primary user model.

If exposed in the browser, SSH controls should be clearly separated from the
basic endpoint modal and should not require ordinary users to understand startup
commands. If not exposed, keep the daemon API and `--remote-guide` as the
documented automation path for owner-directed agents.

Deferred scope: automatic binary deployment, Windows service or Scheduled Task
installation, persisted SSH credentials, and public remote hardening.

Verification should confirm the basic endpoint modal remains the default path
and that any SSH affordance is hidden, advanced, or documentation-backed rather
than mixed into the primary add-server form.
