---
title: ws dashboard multi-server gateway
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260513-research-streamable-http-mcp-transport: adjacent long-running daemon transport research
  260514-research-ws-web-dashboard-direction: longer-range dashboard server federation and remote hardening direction
spec:
  - 260525-ws-dashboard-remote-deployment-guide
  - 260525-ws-dashboard-linked-server-registry-gateway-skeleton
  - 260525-ws-dashboard-remote-link-auth-handshake
  - 260525-ws-dashboard-ssh-tunnel-reconnect
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-25
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
- The `ws-dashboard` CLI should expose an AI-agent-readable remote deployment
  guide, tentatively through a `--remote-guide` flag or equivalent help-surface
  command, so downstream users and agents can inspect the expected SSH tunneling
  bootstrap flow without reverse-engineering implementation details.

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

## Implementation Order

Implement this ticket in reviewable slices rather than as one broad feature.
Although the phase numbers describe the final dependency shape, the recommended
implementation order is:

1. Phase 3 first: add the CLI remote deployment guide so later human or AI-agent
   dogfood has a stable reference for SSH tunneling, passphrase boundaries, and
   reconnect expectations.
2. Phase 1 next: add the linked-server registry and gateway skeleton after
   deciding whether the frontend consumes a selected-server resource view plus a
   separate server list or a full multi-server tree. Prefer the smaller
   selected-server resource view plus server-list approach unless implementation
   evidence points elsewhere.
3. Phase 2 next: add the memory-only remote link-auth handshake and state
   transitions.
4. Phase 4 next: add SSH start/reconnect and tunnel lifecycle using an owner-
   provided remote Windows SSH host as the concrete dogfood target.
5. Phase 5 last: refactor the left nav to server-first hierarchy once backend
   state, auth, and remote reachability are real enough to render.

Before moving implementation slices to `ready/`, add or update spec anchors for
the API shape, resource view choice, remote link-auth handshake, and tunnel
lifecycle being implemented by that slice.

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

### Result (e276809) - 2026-05-25

Implemented the first backend multi-server skeleton. The daemon now exposes a
separate authenticated server list with `server-local` plus persisted linked
servers, while preserving the existing single-server `DashboardResourcesView`
for selected-server resources.

The state store can persist linked-server metadata without dropping WorkRoot
registry or root-picker pin state. Persisted linked servers currently expose
bounded `authRequired` or `tunnelRequired` states, not credentials, SSH targets,
endpoint hints, passphrases, or host paths. A new server-scoped resources route
dispatches `server-local` to the existing local resource view and returns
bounded refusal errors for known linked servers until link-auth and transport
forwarding are implemented.

Verification:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-core`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon linked -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon dashboard_servers_api_lists_local_and_persisted_linked_servers -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon server_scoped_resources_route_dispatches_local_and_refuses_linked_servers -- --nocapture`
- `ws/spec_index.verify`

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

### Result (6f4299a) - 2026-05-25

Implemented daemon-to-daemon link authentication. Remote dashboard daemons now
own a daemon-lifetime link passphrase separate from the one-time browser
pairing URL. `POST /api/dashboard/link-auth` exchanges the passphrase for a
bearer token without requiring browser pairing, and wrong passphrases fail
without installing browser cookies.

The local gateway now accepts owner-authenticated passphrase submissions for
remembered linked servers, forwards them to the linked endpoint, stores the
returned bearer token only in memory, and updates the linked server view to
`connected` for the local daemon lifetime. Connected linked servers can forward
the existing resources view through bearer auth, with server ids rewritten to
the local linked-server id so browser routes remain server-scoped.

Verification:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon remote_link_auth_exchanges_passphrase_for_bearer_without_browser_pairing -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon local_link_auth_connects_and_forwards_remote_resources -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-core`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`
- `ws/spec_index.verify`

#### Edition (348a435) - 2026-05-25

Exposed the daemon-lifetime link passphrase through startup info/output so a
remote daemon launched by SSH can be linked by the owner or an owner-directed
AI agent. Browser pairing remains a separate one-time URL, and SSH deployment,
tunnel lifecycle, and remote Windows dogfood remain Phase 4 scope.

Verification:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon startup_info_builds_local_pairing_url_and_remote_link_passphrase -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon remote_link_auth_exchanges_passphrase_for_bearer_without_browser_pairing -- --nocapture`
- `ws/spec_index.verify`

### Phase 3: CLI remote deployment guide

Add a CLI help surface that dumps a concise remote deployment guide for AI
agents and downstream users. The guide should describe the intended
local-dashboard-as-gateway model, remote loopback bind, SSH tunneling shape,
daemon-lifetime passphrase handling, credential non-persistence, reconnect
expectations, and safe troubleshooting checks.

The exact CLI shape can be `ws-dashboard --remote-guide` or another
discoverable command-line affordance that appears in help text. The output
should be stable enough for AI agents to follow, but it is documentation, not a
machine protocol.

Deferred scope: fully automated remote deployment, persisted credentials,
service installation, and public remote exposure.

Verification should include CLI help/output tests that assert the guide is
discoverable and contains the key SSH tunneling and passphrase boundaries.

### Result (19e6371) - 2026-05-25

Implemented the CLI remote deployment guide as `ws-dashboard --remote-guide`.
The flag is visible in top-level help, parses without a subcommand, prints a
human/AI-agent-readable SSH tunneling guide, and exits without starting the
daemon.

The guide covers local-dashboard-as-gateway topology, remote loopback binding,
SSH tunnel boundaries, daemon-lifetime passphrase handling, disabled credential
persistence, reconnect expectations, and troubleshooting checks. The spec now
records the guide as documentation rather than a machine protocol or remote
process launcher.

Verification:

- `rustfmt --edition 2024 --check ws-dashboard/crates/daemon/src/cli.rs ws-dashboard/crates/daemon/src/main.rs`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon cli::tests -- --nocapture`
- `cargo run --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon -- --help | rg -n -- "--remote-guide|SSH-tunneled"`
- `cargo run --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon -- --remote-guide | rg -n "local ws-dashboard daemon|SSH tunnel|remote loopback|daemon-lifetime passphrase|Credential persistence is disabled|not a stable"`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`
- `ws/spec_index.verify`

### Phase 4: SSH remote start and tunnel reconnect

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

### Result (be10fe9) - 2026-05-25

Implemented the SSH-backed remote start/reconnect path in reviewable backend
slices. The daemon now persists non-secret linked-server SSH metadata, keeps
active SSH tunnel handles and bearer tokens in memory only, and exposes owner-
authenticated routes to start a linked SSH server or reconnect its tunnel.
Remembered SSH servers return to `tunnelRequired` after local daemon restart
until a tunnel is recreated, then move to `authRequired` or `connected`
depending on memory-only link-auth state.

The start route can run an owner-provided SSH startup command, capture bounded
remote daemon startup output, derive the remote loopback endpoint from the
printed pairing URL, capture the daemon-lifetime link passphrase only in
memory, create a local SSH forward, and attempt immediate link-auth through the
forwarded endpoint. Responses remain bounded `ServerConnectionView` objects and
do not expose SSH targets, forwarded ports, remote endpoint hints, passphrases,
bearer tokens, host paths, or cache paths.

Remote Windows dogfood verified the path end-to-end after copying the current
dashboard daemon workspace to an owner-provided Windows SSH host and building it
there. The local daemon started a remote Windows dashboard daemon through an
owner-provided Scheduled Task script so the remote daemon outlived the SSH
startup command, captured startup metadata, created the SSH tunnel, linked
automatically, and forwarded resources under the linked server id. Automatic
binary deployment and service installation remain deferred; the MVP start path
expects the owner or owner-directed agent to provide the remote startup command
that locates or launches the dashboard binary.

Verification:

- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon ssh -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon linked -- --nocapture`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-daemon`
- `cargo test --manifest-path ws-dashboard/Cargo.toml -p ws-dashboard-core`
- Remote Windows dogfood: copied the current `ws-dashboard` Rust workspace to
  the owner-provided SSH host, built `ws-dashboard-daemon`, started the remote
  daemon through the local `/api/dashboard/servers/ssh/start` route with an
  owner-provided Scheduled Task startup script, observed `status:
  "connected"`, and verified `/api/dashboard/servers/{serverId}/resources`
  returned the linked server id.
- `ws/spec_index.verify`

### Phase 5: Server-first left navigation

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

### Result (cb3ee8c) - 2026-05-25

Implemented the server-first left navigation. The frontend now fetches the
separate dashboard server list, tracks the selected server independently from
workspace/workRoot selection, and requests the existing resource tree through
the selected server scope. Only the selected server's resource tree is treated
as active, so selecting a linked server without loaded resources no longer shows
stale workspaces from a different server.

The fixed local-server header and standalone open-workRoot subsection were
replaced by a thin `Servers` toolbar and server rows. Workspace and WorkRoot
rows now nest under the selected server group. The open-workRoot control is
kept as a compact toolbar action but remains local-only until a server-scoped
open route exists, so it is disabled for non-local selected servers instead of
silently targeting the local daemon.

Verification:

- `npm --prefix ws-dashboard/frontend run test:resource-model`
- `npm --prefix ws-dashboard/frontend run build`
