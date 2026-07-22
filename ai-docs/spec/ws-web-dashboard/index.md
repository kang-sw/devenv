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
logging, bounded graceful shutdown, and a minimal health surface. The default
bind target is `127.0.0.1`. The daemon does not treat loopback access as
authorization. After the outer server process receives its shutdown signal,
long-lived browser connections such as idle sockets, SSE streams, or WebSockets
may receive a short drain window, but they must not keep the local development
server alive indefinitely.

The daemon writes its structured logs to two sinks by default: stderr
(unchanged for foreground `dev.sh run` usage) and a durable rolling log file
under `<state dir>/logs/daemon.log`, so a detached or backgrounded daemon
keeps a diagnosable log history even when its stderr stream is discarded. The
file sink rotates daily to `daemon.log.YYYY-MM-DD` and retains roughly the
last 14 rotated files before pruning older ones. `ws-dashboard serve --log-file
<path>` overrides the default file-sink location; the existing `--log-filter`
flag (default `"info"`) continues to control verbosity for both sinks
unchanged. If the log directory or file cannot be opened, the file sink
degrades fail-soft: the daemon logs a warning to stderr and continues with
stderr-only output rather than failing to start.
{#260716-dashboard-daemon-persistent-log-file-sink}

On startup, the daemon creates an in-memory high-entropy one-time pairing token
with an explicit expiry policy and exposes the corresponding pairing URL to the
local owner through startup output. The pairing route is the only
unauthenticated browser entrypoint. A successful pairing exchange consumes the
token, installs an HTTP-only owner session cookie with `SameSite=Lax`, and
redirects browser callers to a token-free stable app URL. Missing, invalid,
reused, or expired pairing tokens fail without installing a session cookie and
without redirecting into an authenticated-looking app route.
{#260516-ws-web-dashboard-token-free-pairing-landing}

Authenticated owner sessions have broad host-control authority for dashboard
features, but the daemon remains separate from ws MCP stdio session authority.
The daemon must not make itself the canonical ws MCP root, harness, model
backend, or provider/native-agent session owner.

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

For explicit local debugging, `ws-dashboard serve --no-auth` disables the owner
auth middleware only for loopback, non-public serving. The daemon rejects
no-auth serving for public bind mode or non-loopback bind hosts before it
listens, and server startup revalidates the serving config so manually
constructed configs cannot bypass that guard. In no-auth mode the protected
router is still built as one route set, but the owner-auth layer is omitted for
that whole protected router; individual handlers do not select behavior by auth
mode. Startup output continues to expose the owner pairing URL, and additionally
prints a token-free direct dashboard URL after the listener address is known so
local debug sessions do not need to use `/pair`.

The `ws-dashboard --remote-guide` CLI surface prints an AI-agent-readable
remote deployment guide and exits without starting the daemon. The guide
describes the local-dashboard-as-gateway model, remote loopback serving,
SSH tunneling, daemon-lifetime passphrase handling, non-persisted credentials,
reconnect expectations, and safe troubleshooting checks. The guide is human and
agent documentation, not a machine protocol, and does not grant authentication
or start remote processes by itself. {#260525-ws-dashboard-remote-deployment-guide}

The initial UI route serves a minimal placeholder surface behind owner
authentication. Health output is the exact minimal body `ok\n`; host paths,
cache paths, Git roots, pairing tokens, session values, diagnostics, and wsstate
internals are not URL identity and are not exposed by the health surface.

The daemon resolves its persisted state file (opened work roots, root-picker
pins, linked servers) through an ordered fallback: an explicit
`WS_DASHBOARD_STATE_FILE` path wins outright; otherwise
`WS_DASHBOARD_STATE_HOME` (joined with `opened-workroots.json`), then
`XDG_STATE_HOME` (joined with `ws-dashboard/opened-workroots.json`), then
`HOME` (joined with `.local/state/ws-dashboard/opened-workroots.json`) are
tried in order. On a native Windows build, if none of the above resolve, the
daemon falls back to `%LOCALAPPDATA%\ws-dashboard\opened-workroots.json` via
`LOCALAPPDATA`; this Windows fallback does not change the Linux/macOS
resolution order or its env var names. If no candidate resolves, state
persistence is disabled for the run: every load returns empty and every
persist call reports success without writing, so a daemon started this way
loses all opened work roots, root-picker pins, and linked-server links across
requests, with only a startup log warning surfaced to the operator.
{#260708-dashboard-state-file-resolution-order}

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

## Resource View-Model Contract {#260516-ws-web-dashboard-resource-view-model-contract}

The dashboard exposes authenticated HTTP view-model APIs for the first visible
resource hierarchy:

```text
server -> workspace -> workRoot -> mainInstance -> subInstance
```

Callers address API resources through opaque ids rather than host paths, Git
roots, wsstate paths, workRoot keys, or runtime session identifiers. The daemon
owns those private identifiers and exposes only authenticated view-model fields
that the browser needs to render navigation, selection, status,
stale/error/loading state, and available actions.

`workspace` means a daemon-discovered project group, not a user-created
category. `workRoot` means the physical directory used as an open, spawn, and
run target. WorkRoots report additive kind metadata for `plainDirectory`,
`gitPrimaryRoot`, or `gitLinkedWorktree`, plus status such as online, offline,
moved, or inaccessible. Primary roots and linked Git worktrees share the same
core workRoot API shape while preserving enough metadata for the UI to
distinguish their repository role and lifecycle affordances.

The API shape preserves the full hierarchy even when the browser later renders
a singleton `workspace -> workRoot` resource tree as one compact workRoot row.
Authenticated callers may observe compactability hints, but compaction is a
presentation policy and not URL identity; the compact row selects the concrete
workRoot id and does not require a main instance.

## Linked Server Registry And Gateway Skeleton {#260525-ws-dashboard-linked-server-registry-gateway-skeleton}

The local dashboard daemon exposes servers as first-tier owner-visible objects
without requiring the browser to connect directly to remote hosts. The local
daemon always appears as `server-local`, and remembered linked servers appear
beside it with bounded state such as `connected`, `authRequired`,
`unreachable`, `starting`, `staleEndpoint`, or `tunnelRequired`.

The server list is separate from the selected server's resource tree. This lets
existing callers keep consuming the single-server `DashboardResourcesView`
shape while new callers can render a multi-server navigation shell from a
server list plus the selected server resources.

Focusing a different linked server preserves the previously-focused server's
mounted workbench surfaces instead of tearing them down. Terminals, agent
chats, and editors already opened under a server stay mounted and hidden - not
destroyed - while a different server holds focus, the same hide-not-unmount
behavior a work-root switch already applies within one server. Each open work
root's mount state resolves against its own server's resource tree, so a
non-focused server's open surfaces keep resolving and stay mounted instead of
unmounting merely because a different server is now selected. The right-side
workbench still renders exactly one work root at a time, and only the focused
root polls for updates; a non-focused server's cached resource tree is not
refreshed until that server is refocused.
{#260714-ws-dashboard-cross-server-workbench-keepalive}

The left nav exposes this keep-alive as a per-server On/Off control. A server
is On by default once selected or previously opened; clicking a server's label
turns it On (expands its workspaces and focuses it) - the same gesture that
already selects a server, not a new one. Off is a separate, explicit button on
the server row and is the only gesture that deallocates a server's workbench
state: turning a server Off collapses its workspaces, closes every work root
open under it, and discards its cached resource tree, so nothing about that
server survives in memory afterward. A focus switch alone - selecting a
different server without pressing Off - never deallocates anything. Turning a
server back On (re-selecting or re-adding it) always fetches a fresh resource
tree rather than reusing any prior state. `server-local` is always On and its
Off control is disabled, since the local daemon's own workbench cannot be
deallocated. Off targeting the currently-focused server refocuses selection to
`server-local`. The right-side workbench remains single-active, and the git
toolbar and activity console remain single-instance for the active root; Off
does not change that.

Linked-server identity stays dashboard-owned and local-gateway scoped. `serverId`
selects the target daemon for browser requests; it is not a ws MCP session key,
provider session id, endpoint, or remote host path. Any future ws MCP binding for
a linked server belongs to the selected target daemon and remains daemon-private
to that target.

Authenticated callers can request resources through
`/api/dashboard/servers/{serverId}/resources`. The local server id returns the
same live resource view as the existing local resources route. Linked server
ids are recognized from daemon-owned persisted metadata, but until link-auth
and transport forwarding are implemented they return bounded refusal errors
instead of leaking endpoint, SSH target, passphrase, host path, or cache path
details. Unknown server ids return a bounded not-found response.

## Remote Link Authentication Handshake {#260525-ws-dashboard-remote-link-auth-handshake}

Remote dashboard daemons expose a link-auth handshake for local gateway
daemons. The remote daemon owns a daemon-lifetime link passphrase that is
separate from the one-time browser pairing URL and exposes it through startup
info/output for the owner or an owner-directed AI agent to record. A caller
that knows the passphrase can exchange it for a bearer token suitable for
daemon-to-daemon gateway requests. Wrong passphrases fail without consuming
browser pairing state or installing browser cookies.

The local gateway accepts a passphrase for a remembered linked server through
an owner-authenticated local route. It forwards the passphrase to the linked
server endpoint, stores the returned bearer token only in memory, and updates
the linked server view to `connected` for the local daemon lifetime. Linked
server credentials are not persisted. If the local daemon restarts, the server
can remain remembered but returns to an auth-required or tunnel-required state
until the owner re-enters the passphrase.

Once a linked server has a memory-only token, the local gateway may forward
bounded read-only resource requests to the remote daemon using bearer auth while
preserving the same server-scoped route shape. Link-auth failures distinguish
unknown server, missing endpoint, wrong passphrase, upstream rejection, and
unreachable endpoint without exposing SSH targets, endpoint hints, passphrases,
bearer tokens, host paths, or cache paths.

## SSH Tunnel Reconnect For Linked Servers {#260525-ws-dashboard-ssh-tunnel-reconnect}

The local gateway may remember non-secret SSH remote metadata for a linked
dashboard server: a display label, opaque server id, SSH target, remote
loopback dashboard endpoint, and the most recent local forwarded endpoint.
Passphrases, bearer tokens, and active tunnel process handles remain
daemon-memory-only.

An authenticated owner can ask the local gateway to start or reconnect an SSH
tunnel for a linked server. The gateway creates a local loopback forward to the
remembered remote loopback endpoint, updates the linked server's local endpoint
hint, and returns only the bounded `ServerConnectionView` state. Responses do
not expose SSH targets, local forwarded endpoint ports, remote endpoint hints,
passphrases, bearer tokens, host paths, or cache paths.

For SSH start, the owner may provide a remote startup command that prints the
remote daemon startup output and then exits while the remote daemon remains
alive. The local gateway captures bounded startup output, derives the remote
loopback endpoint from the printed pairing URL, captures the daemon-lifetime
link passphrase only in memory, creates the SSH forward, and attempts immediate
link-auth through the forwarded endpoint. A failed automatic link-auth attempt
does not persist credentials; the linked server remains visible as
`authRequired`.

After local daemon restart, remembered linked servers with SSH and remote
endpoint metadata are visible as `tunnelRequired` until a tunnel is recreated.
Once the tunnel exists but no memory-only link token is present, the linked
server becomes `authRequired` and the owner must re-enter the remote
daemon-lifetime passphrase through the link-auth route.

## Endpoint-First Linked Server Add {#260525-ws-dashboard-endpoint-linked-server-add}

The local dashboard lets an authenticated owner register a linked dashboard
server from an owner-provided endpoint without asking the dashboard to manage
SSH. The endpoint may be a user-managed loopback tunnel, VPN/private network
route, port forward, or other reachable dashboard daemon URL. The browser
submits the endpoint and optional daemon-lifetime link passphrase to the local
daemon; the browser does not call the remote endpoint directly.

The local daemon normalizes and validates the endpoint, rejects unsupported
schemes, probes enough to distinguish unreachable or incompatible targets, and
persists only non-secret linked-server metadata such as opaque server id,
display label, kind, and endpoint hint. If a passphrase is supplied, the local
daemon exchanges it with the remote `/api/dashboard/link-auth` endpoint and
stores the returned bearer token only in memory. Without a memory-only token,
the server remains visible in a bounded auth-required or unreachable state.

The left navigation's add-server affordance opens a compact endpoint-first
modal. SSH-managed start or reconnect remains an advanced or agent-operated
path and is not required by the default add-server flow.

## 🚧 Server Route Identity And Scoped Operation Endpoints {#260703-ws-dashboard-server-route-scoped-operation-endpoints}

`serverRoute` is the canonical browser-visible term for the daemon routing
identity that selects which daemon a dashboard operation targets. Existing
serialized fields named `serverId` — notably the resource path's `serverId` and
the linked-server registry/link-auth/tunnel identity — are compatibility wire
field names that carry the selected Server Route. The canonical term is
`serverRoute`; the wire field name stays `serverId` so this contract does not
contradict the resource-path and linked-server sections that still serialize
`serverId`.

The local daemon's Server Route is always `server-local`. Frontend callers pass
a `serverRoute` (or a full resource path whose `serverId` carries it); a `null`,
empty, or `server-local` route resolves to the local, unscoped route shape, and
any other route resolves to the server-scoped shape. This keeps existing
local-only persisted UI records readable: an omitted route defaults to
`server-local`.

Server-scoped dashboard operations address a target daemon through canonical
route shapes rooted at `/api/dashboard/servers/{serverRoute}/...`:

```text
/api/dashboard/servers/{serverRoute}/resources
/api/dashboard/servers/{serverRoute}/root-picker[/directories|/pins]
/api/dashboard/servers/{serverRoute}/work-roots/open
/api/dashboard/servers/{serverRoute}/work-roots/{workRootId}/...
/api/dashboard/servers/{serverRoute}/workspaces/{workspaceId}/...
/api/dashboard/servers/{serverRoute}/terminals/{terminalId}/...
```

The equivalent local, unscoped routes (for example `/api/dashboard/root-picker`
or `/api/dashboard/work-roots/{workRootId}/...`) remain the observed shape for
the `server-local` route, so a route-agnostic caller sees no behavior change for
local operations.

A single Server Route segment must be dot-free: it matches `[A-Za-z0-9_-]+`. Dot
is reserved as a future hop separator for multi-hop routing. Generated
linked-server routes already satisfy this (a `server-<slug>` slug reduces to
that character set). The frontend rejects a route with a reserved character both
when constructing a canonical `servers/{serverRoute}/...` route and when adding
a linked server, surfacing a bounded client-side error rather than silently
rewriting the value. Existing persisted dotted routes are not rewritten.
Daemon-side rejection of dotted Server Route requests is authoritative: a
server-scoped route whose segment contains a dot is refused before any
linked-server lookup with a bounded `400` error, and this refusal is the sole
authority regardless of any client-side check. Existing persisted dotted
linked-server ids are refused the same way rather than being silently rewritten.

### One-Shot Operation Forwarding Envelope

Server-scoped one-shot HTTP operations resolve by Server Route:

- `server-local` (and the `null`/empty default) dispatches **in-process**
  through the equivalent local handler, so its response is byte-for-byte
  equivalent to the unscoped local route — including status, JSON body, and
  headers such as `x-ws-dashboard-opened-work-root-id`. The unscoped local
  routes remain live as `server-local` compatibility aliases.
- Any other Server Route forwards to the selected linked server through a single
  allowlisted JSON/HTTP helper that attaches the daemon-held memory-only bearer
  token and targets the linked server's endpoint hint. Only explicitly
  registered server-scoped one-shot routes reach the helper; unregistered
  server-scoped paths (the terminal WebSocket socket route and other unlisted
  operations) fall through the protected router as `404` rather than proxying
  arbitrary daemon paths. Document-event and Activity-event SSE are the
  streaming exceptions and are proxied explicitly — see
  [Document-Event SSE Proxying](#document-event-sse-proxying) and
  [Remote Activity, Git, And Workspace Operations](#remote-activity-git-workspace-operations).

A forwarded response preserves the upstream status and body as much as
practical. For operations that return a `DashboardResourcesView`, the daemon
rewrites the returned view — including every nested `ResourcePath.serverId` — to
the selected Server Route so a linked server's local identities never leak into
server-scoped resource paths, and it preserves the
`x-ws-dashboard-opened-work-root-id` header.

### Bounded Gateway Errors

Forwarding failures surface as bounded dashboard errors that never leak
endpoints, tokens, private paths, or daemon/session metadata:

- **Unknown Server Route** — `404`, the route matches no linked server.
- **Invalid Server Route** — `400`, the route segment contains a dot.
- **Auth required** — `409`, no in-memory bearer session for the linked server.
- **Tunnel required** — `409`, the linked server has no endpoint hint yet.
- **Unreachable upstream** — `502`, the forwarded request could not complete.
- **Upstream rejection** — the linked server's own status and error body are
  preserved as returned.

Collision-safe UI identity derives from the Server Route: the same
`workRootId`, `workspaceId`, `activityId`, or `terminalId` observed on two
different Server Routes produces distinct workbench panes, file-pane source
keys, document/Activity subscription keys, Git state keys, terminal
pane/restore/list records, command payloads, and persisted UI records. Records
that omit a route are treated as `server-local`.

### Remote File Operations

WorkRoot file listing, file read, and file write are server-scoped one-shot
operations that resolve through the same forwarding envelope above:
`/api/dashboard/servers/{serverRoute}/work-roots/{workRootId}/files`,
`.../files/read`, and `.../files/write`. `server-local` dispatches in-process to
the unscoped handlers; any other route forwards over the allowlisted bearer
helper. File write preserves optimistic-concurrency semantics end to end: the
`baseContentHash`/`contentHash` conflict contract is carried through unchanged,
so a stale base hash surfaces the upstream `409` content-hash mismatch and a
missing base hash surfaces `422`, identically for local and remote routes. The
`server-local` write alias enforces the same `application/json` request
content-type boundary as the unscoped route before dispatching, and classifies
malformed-JSON bodies the same way the unscoped `Json` extractor does (`422`
for a data error such as a missing field, `400` for a syntax error), so it
stays byte-for-byte equivalent rather than silently accepting bodies the
unscoped `Json` extractor would reject or diverging on status code.

### Document-Event SSE Proxying

Document-event SSE is the one streaming route this envelope proxies rather than
`404`-ing. `/api/dashboard/servers/{serverRoute}/work-roots/{workRootId}/documents/events`
dispatches in-process for `server-local`; for any other route the gateway
resolves the linked server through the same dot-free refusal and bounded-error
boundary, then opens a bearer-authenticated upstream `text/event-stream` GET and
re-streams its raw bytes to the browser. Key properties:

- **Opaque byte passthrough.** The gateway does not parse or rewrite SSE frames.
  Document-event payloads carry only `source.workRootId`/`path`/`contentHash`,
  and the browser scopes events by the *subscription* URL it connected to (the
  server-scoped endpoint), so no per-frame `serverRoute` rewrite is needed and
  same-`workRootId` documents on different routes stay isolated by subscription.
- **Content-type enforcement.** A successful upstream response whose content type
  is missing or is not `text/event-stream` is rejected as `502` rather than
  forwarded, so a non-stream upstream can never masquerade as an event stream.
- **Upstream error preservation.** A non-success upstream response is forwarded
  with its status and body preserved (like other one-shot forwards); an
  unreachable upstream surfaces `502`.
- **Implicit cleanup.** No explicit lifecycle frames are exchanged. When either
  side disconnects, the browser-facing stream drops, which drops the upstream
  `reqwest` response and releases the subscription.

Activity-event SSE is proxied by the same mechanism — see
[Remote Activity, Git, And Workspace Operations](#remote-activity-git-workspace-operations).
The terminal WebSocket socket route remains deferred and continues to `404`; it
is not proxied by this phase. Terminal HTTP lifecycle routes are forwarded — see
[Remote Terminal HTTP Lifecycle](#remote-terminal-http-lifecycle).

### Remote Activity, Git, And Workspace Operations {#remote-activity-git-workspace-operations}

WorkRoot Activity, Git toolbar, workspace removal, and Git worktree-add
operations are server-scoped through the same envelope. `server-local`
dispatches in-process to the unscoped handlers (byte-for-byte equivalent);
any other Server Route forwards over the allowlisted bearer helper. The
registered server-scoped routes are:

- Activity: `.../work-roots/{workRootId}/activity`,
  `.../activity/items/{activityId}/transcript`, and
  `.../activity/events` (SSE).
- Workspace removal: `DELETE .../workspaces/{workspaceId}`.
- Git worktree-add: `.../workspaces/{workspaceId}/git-worktree-add/options`,
  `.../git-worktree-add/preview`, and `.../git-worktree-add`.
- Git toolbar: `.../work-roots/{workRootId}/git/status`, `.../git/branches`
  (GET lists, POST creates), `.../git/switch-branch`, `.../git/fetch`,
  `.../git/push`, and `.../git/pull-ff-only`.

Key properties:

- **Activity-event SSE proxying.** `.../activity/events` reuses the exact
  document-event SSE mechanism: a bearer-authenticated upstream
  `text/event-stream` GET, content-type enforcement (`502` on a
  missing/non-stream content type), upstream error preservation, opaque byte
  passthrough, and implicit drop-based cleanup. It resolves through the same
  dot-free refusal and bounded-error boundary as one-shot routes, so the new
  stream branch never bypasses auth.
- **Worktree-add resource rewrite.** The `git-worktree-add` submit response
  carries a `DashboardResourcesView` nested under a wrapper object. The gateway
  rewrites that nested view — including every `ResourcePath.serverId` — to the
  selected Server Route, so a linked server's created worktree surfaces under
  the browser-visible route rather than the linked daemon's local identity.
  Workspace removal returns a bare `DashboardResourcesView` and is rewritten by
  the same resources path as `open work-root` and activation.
- **Owner-auth + bearer gating.** Mutating operations (branch create/switch,
  fetch/push/pull, workspace removal, worktree add) preserve owner auth at the
  local gateway (router placement) and bearer auth to the linked daemon; they
  are never reachable without both.
- **Body-parsing aliases match axum.** The `server-local` aliases that parse a
  JSON request body (worktree preview/submit, branch create, switch-branch)
  enforce the same `application/json` content-type boundary and classify
  malformed bodies the same way the unscoped `Json` extractor does (`415` for a
  missing/non-JSON content type, `422` for a data error, `400` for a syntax
  error), staying byte-for-byte equivalent to the legacy route.
- **Remote host paths.** Git worktree path previews and error messages returned
  from a linked server describe remote host paths; the UI presents them as
  belonging to the selected server, not the local host.

Agent-control actions (interrupt, cancel, erase, retry, terminate) remain out of
scope for this phase.

### Remote Terminal HTTP Lifecycle {#remote-terminal-http-lifecycle}

Terminal create, list, output poll, input, resize, and close are server-scoped
through the same one-shot envelope. `server-local` dispatches in-process to the
unscoped terminal handlers (byte-for-byte equivalent); any other Server Route
forwards over the allowlisted bearer helper. The registered server-scoped routes
are:

- `GET`/`POST .../work-roots/{workRootId}/terminals` — list / create.
- `GET .../terminals/{terminalId}/output` — output poll (carries `?after=`).
- `POST .../terminals/{terminalId}/input` — raw input.
- `POST .../terminals/{terminalId}/resize` — PTY resize.
- `DELETE .../terminals/{terminalId}` — close.

Key properties:

- **Opaque daemon-local terminal ids.** Terminal ids stay opaque ids owned by
  the target daemon; the gateway does not synthesize or rewrite them. Collision
  safety is achieved purely on the browser side by keying pane/session/restore
  state on `{serverRoute, terminalId}`, so the same bare `terminalId` on two
  servers stays distinct and an operation on one server's terminal never reaches
  another server's identically-named terminal. Closing a remote terminal
  forwards `DELETE` to that server only and leaves a local terminal sharing the
  same bare id untouched.
- **Close closes upstream.** A forwarded `DELETE` closes the terminal on the
  linked daemon, so a subsequent output poll for that id surfaces the upstream
  `404`.
- **Body-parsing aliases match axum.** The `server-local` aliases that parse a
  JSON body (create, input, resize) enforce the same `application/json`
  content-type boundary and classify malformed bodies the same way the unscoped
  `Json` extractor does (`415` for a missing/non-JSON content type, `422` for a
  data error, `400` for a syntax error), staying byte-for-byte equivalent to the
  legacy route.
- **Owner-auth + bearer gating.** Terminal input, resize, and close are mutating
  host control; they preserve owner auth at the local gateway (router placement)
  and bearer auth to the linked daemon, and are never reachable without both.
- **Live socket route.** `.../terminals/{terminalId}/socket` is the live
  WebSocket transport, described in
  [Remote Terminal WebSocket Gatewaying](#remote-terminal-websocket-gatewaying).

### Remote Terminal WebSocket Gatewaying {#remote-terminal-websocket-gatewaying}

The live terminal WebSocket route
`GET /api/dashboard/servers/{serverRoute}/terminals/{terminalId}/socket` carries
PTY output, input, resize, ping/pong, and close frames. The browser connects
only to the local gateway; for a linked Server Route the gateway opens its own
upstream WebSocket to the linked daemon with the memory-only bearer token and
relays frames. This is a distinct upgrade-and-relay mechanism, not the one-shot
HTTP envelope.

- **Server-local dispatch.** `server-local` dispatches in-process to the
  unscoped `terminal_websocket` handler unchanged (no relay).
- **Refusal before upgrade.** Every linked-server refusal resolves to a bounded
  HTTP error response *before* the browser-side upgrade is accepted, never a
  half-open or aborted `101` socket: dot-free rejection (`400`), unknown Server
  Route (`404`), auth required / tunnel required (`409`), and — when the upstream
  connect itself fails — unreachable (`502`). The upstream is contacted first;
  the browser upgrade is completed only after the upstream WebSocket connects.
- **Upstream URL construction.** The upstream socket URL appends the legacy
  `/api/dashboard/terminals/{terminalId}/socket?after=` path to the stored linked
  endpoint through the same helper used by one-shot forwarding *first*, then
  swaps the `http`/`https` scheme to `ws`/`wss`, so any base path baked into the
  linked endpoint is preserved.
- **Bearer on the upstream upgrade.** The upstream upgrade request carries the
  linked server's bearer token as an `Authorization` header. An upstream that
  rejects the upgrade with an HTTP status (e.g. `410`) propagates that status as
  a bounded refusal without completing the browser upgrade.
- **Bidirectional frame relay.** Text, binary, ping, pong, and close frames relay
  in both directions with explicit conversion between the browser and upstream
  message types. Tungstenite's raw frame variant has no browser equivalent and is
  dropped rather than treated as an error.
- **Bounded cleanup.** When either side closes or errors, the relay loop breaks,
  best-effort sends a close frame to the other side, and lets both connections
  drop — a browser-initiated close tears down the upstream connection and an
  upstream-initiated close propagates a close frame to the browser, with no
  lingering task.

## Durable WorkRoot Registry And Activation {#260523-dashboard-workroot-registry-activation}

The dashboard exposes known workspace and workRoot membership from a
daemon-local durable registry instead of treating only currently opened
workRoots as the visible resource set. A known workRoot remains visible until a
future explicit forget/remove policy removes it, even when it is currently
missing, inaccessible, moved, or inactive.

> [!note] Planned 🚧 {#260524-dashboard-workspace-root-prune-policy}
> The registry will distinguish owner-managed workspaces from automatically
> detected workRoots. A workspace has a root workRoot anchor, such as an
> owner-added directory or Git root, and may contain discovered child workRoots
> such as linked Git worktrees. If the root workRoot becomes unavailable, the
> workspace remains visible in a disabled or recovery-needed state while any
> child workRoot is still active, where active means activation permits
> targeting and availability is currently usable. This gives callers room to
> reconnect the root or derive a new workspace from a dangling child. Automatic
> pruning removes a workspace only when it has no active workRoots. Explicit
> forget/remove UI remains a separate owner-driven cleanup policy.

WorkRoot view-models separate live availability from user-controlled
activation. `availability` describes the daemon's current filesystem/Git
assessment of whether the workRoot can be used now, with initial public values
for available, missing, moved, inaccessible, and unknown states. `activation`
describes whether the dashboard is currently allowed to target that workRoot
for file, Activity, and terminal APIs, with `online` and `offline` values.
A reachable workRoot with `activation: offline` remains a visible row and is
not the same state as a missing or inaccessible workRoot.

Existing opened-workRoot persistence migrates into the registry as known
membership with `activation: online`, preserving current restart behavior.
Newly discovered sibling workRoots may enter the same registry with
`activation: offline` while remaining visible in the resource tree.

Resource views, activation actions, and protected route gates derive activation
from the same registered workRoot id regardless of whether the visible row is
reached as an explicitly opened root or as an automatically discovered linked
workRoot. If one physical workRoot is visible through both paths, callers see
one activation state and the same online/offline behavior across navigation,
Activity, file, and terminal surfaces.
{#260524-dashboard-workroot-registry-wide-activation-lookup}

## Git Worktree Creation {#260524-ws-dashboard-git-worktree-creation}

The dashboard lets an authenticated owner add a linked Git worktree from a
workspace-scoped overflow menu. The workspace remove affordance appears
behind the same overflow menu, preserving its dashboard-only confirmation and
registry behavior while making room for non-destructive workspace operations.

The add-worktree flow is a Git operation, not a generic filesystem picker. A
modal collects a worktree name, branch resolution, and target path resolution.
Automatic branch naming derives a branch-compatible candidate from the
worktree name, then the daemon previews whether submit will create a new branch,
check out an existing branch, or block the request. Automatic path naming
targets the workspace Git root's `.git/ws-worktree/<branch-compatible-name>`
convention. Custom path selection may reuse the folder picker in target-path
or parent-directory mode without adding broad file-manager operations.

Submit revalidates the preview, runs the corresponding `git worktree add`
operation, refreshes canonical dashboard resources, activates the created
workRoot by default, and selects or focuses the created linked workRoot when
the daemon can identify it. Checked-out branches, invalid names, unavailable
Git roots, and path conflicts produce bounded errors without exposing private
host paths in command payloads, logs, or browser-visible diagnostics.

## Git Worktree Removal {#260722-ws-dashboard-git-worktree-removal}

The dashboard lets an authenticated owner remove a linked Git worktree
through a two-step preview/submit flow mirroring worktree creation: a
non-mutating preview reports the target's dirty state (modified/untracked
file counts) and, when the worktree has a checked-out branch, whether that
branch is unmerged. Submit re-resolves and re-validates before mutating.

The op always resolves and runs `git worktree remove`, and any branch
deletion, from the workspace's PRIMARY Git root — never from a working
directory rooted inside the worktree being removed, since a worktree cannot
remove itself from its own context.

Force gate: submitting against a worktree with uncommitted or untracked
changes without an explicit `force` flag is rejected with a bounded 409
response; the caller must resubmit with `force` set after the owner has seen
the data-loss warning. Force is never implied or defaulted — it is a
deliberate per-submit caller choice, not something a client can trigger
accidentally.

Optional branch deletion is non-destructive. Whether deletion is requested is
the caller's choice. Before deleting, the daemon checks — using a
non-mutating merge-base ancestry check equivalent to the condition under
which plain `git branch -d` itself would refuse — whether the branch has
commits unreachable from its upstream, or, absent an upstream, from the
primary root's current HEAD. A branch found unmerged is left intact and
reported back as skipped; branch deletion only ever runs plain `git branch
-d`, never a force variant, so it can never discard dangling commits.

On successful disk removal, the daemon unregisters the workRoot entry and
persists the registry, then clears terminal, Codex, and Claude sessions
scoped to the removed workRoot id. If the registry persist fails after a
successful `git worktree remove`, disk is treated as the source of truth:
the entry is not re-registered — doing so would resurrect a registry row
pointing at a path that no longer exists — session cleanup still runs, and
the returned view reflects the completed removal. The stale persisted file
is logged for self-heal on a later successful persist.

## Git-Aware WorkRoot Toolbar {#260524-ws-dashboard-git-aware-workroot-toolbar}

The selected WorkRoot toolbar shows Git controls only for online, available
Git workRoots. Non-Git, offline, missing, moved, or inaccessible workRoots do
not render branch or Git status controls beyond bounded
unavailable diagnostics.

The toolbar includes a branch chip for the current branch or a bounded detached
`HEAD` label. Opening the chip shows a daemon-resolved branch list with
checked-out branches disabled when known, plus a `+ New branch...` action that
creates and switches to a new branch from a selected base branch. Branch switch
and create actions follow Git defaults and revalidate server-side before
mutation.

A compact Git status pill summarizes line/file and upstream state with the
segment grammar `+<added-lines> -<removed-lines> *<modified-files>
?<untracked-files> | ↑<ahead> ↓<behind>`. The pill always exposes a small
fetch/refresh action, and upstream push/pull segments are interactive only when
applicable. Push runs plain `git push`; pull runs
`git pull --ff-only` so dashboard-triggered pulls cannot leave the workRoot in
a merge or rebase conflict state.

Status refresh stays host-light: the dashboard refreshes immediately on
selected WorkRoot changes, visibility return, explicit fetch/push/pull,
branch switch, and branch create, then polls conservatively only for the
selected visible WorkRoot. That poll's `git status` call passes
`--no-optional-locks`, so it never takes the repository's `.git/index.lock`
(`260714-bug-git-status-poll-index-lock-staleness`); mutating routes
(switch/fetch/push/pull) are unaffected and still take locks as needed. All
Git toolbar routes remain owner-authenticated,
address workRoots by opaque `workRootId`, keep Git work off async workers, and
avoid exposing host paths in command logs or bounded browser-visible errors.

Authenticated route behavior distinguishes registry membership and current
operability. Unknown workRoot ids return not-found responses. Known workRoots
with offline activation return a bounded offline response. Online workRoots
whose availability has degraded return a bounded unavailable response without
exposing host paths. Terminal HTTP routes and already-open terminal WebSockets
re-check the owning workRoot's activation and availability before accepting
input, resize, close, or output/backfill access. Online/offline transitions are
dashboard commands with logical targets so mouse controls and later keybindings
share the same command path.

Explicit resource refresh recomputes availability from filesystem/Git without
changing activation. While the dashboard is open, bounded polling refreshes
known workRoot availability through the same canonical resource endpoint so
external filesystem or Git worktree changes can become visible. Polling is not
the sole correctness mechanism: explicit refresh remains deterministic, polling
does not become browser-side resource authority, overlapping refresh requests
are suppressed, stale poll results do not overwrite newer open or activation
resource views, and refresh failures keep the last known resource tree visible.
Filesystem watchers, if added later, act only as refresh hints.

## Worktree Removal Confirmation And Hide UX {#260722-ws-dashboard-worktree-removal-hide-ux}

Removing a linked worktree always opens a real confirmation modal — never a
bare browser `confirm()` dialog — regardless of whether the target is clean
or dirty; worktree add/remove is treated as a heavy operation independent of
whether there is uncommitted work to lose. When the preview reports
uncommitted or untracked changes, the modal additionally surfaces a distinct
red data-loss notice naming the file counts that will be deleted; submitting
from that state is what authorizes the daemon's force gate. A 409 force-gate
response — the target turned dirty after a clean preview — refreshes the
preview in place so the notice appears, rather than forcing the owner to
close and reopen the modal.

The same modal includes a "delete branch too" checkbox that defaults OFF, so
the branch is preserved unless the owner opts in. When the preview reports
the branch as unmerged, the checkbox shows a red inline warning; even if
checked in that state, the branch is kept rather than deleted.

Hiding a worktree is pure browser-local display state, fully decoupled from
the daemon registry: it only removes the row from the dashboard's visible
worktree list, never touches the worktree directory or its branch, and never
calls the removal op. The hide affordance is not gated on the daemon's
worktree-removal availability — an unavailable or inactive worktree can still
be hidden, which is the case where decluttering is most useful. Restore
path: the owning workspace's "..." overflow menu gains a hidden-worktrees
submenu listing currently-hidden entries; selecting one un-hides it. The
`g w h` chord opens/reveals this hidden-worktrees submenu for the currently
relevant workspace.

## Mock View-Model Fixtures {#260516-ws-web-dashboard-mock-view-model-fixtures}

The dashboard daemon provides deterministic fixture-backed resource data that
uses the same view-model API contract as live providers. Frontend and contract
tests can render the first visible shell without live wsstate, PTY, named-agent,
harness, or filesystem discovery dependencies.

Fixtures cover singleton chains, multi-root workspaces, plain directories, Git
primary roots, linked Git worktrees, offline or inaccessible workRoots, main
instances, sub instances, stale/error/loading states, and visible action hints.
Protected API route tests verify that fixture-backed dashboard data remains
behind the owner-auth boundary.

Normal daemon operation uses live opened workRoot state as the primary resource
authority for authenticated dashboard resource loads. Fixture-backed resources
remain available for deterministic tests and explicit fixture contexts, but the
production resources endpoint does not silently return the mock workspace when
no workRoot has been opened. {#260516-ws-web-dashboard-live-resource-authority}

## Protected Frontend Shell {#260516-ws-web-dashboard-protected-frontend-shell}

The dashboard daemon serves the first React/TypeScript/Vite browser shell
behind the same owner-auth boundary as other protected dashboard routes. Static
asset serving does not add another unauthenticated top-level route beside
`/pair`; unauthenticated browser requests for the dashboard shell are rejected
before assets or fallback UI are served.

The frontend package provides documented local development and production build
entrypoints that later dashboard slices can reuse. The first shell remains
narrow: it does not implement PTY, editor, document viewer, live workspace
discovery, event streams, named-agent controls, or root picker behavior.

Browser navigation reserves explicit server-scoped routes such as
`/servers/:serverId/...` for dashboard resources, while daemon-owned opaque ids
remain the source of truth and server identity is not hidden inside workspace,
workRoot, or instance ids. Refreshing `/servers` or server-scoped app paths
serves the protected frontend shell through the same owner-auth static route
boundary. {#260516-ws-web-dashboard-server-scoped-browser-routes}

## PWA Installability {#260721-ws-dashboard-pwa-installability}

The dashboard daemon serves a web app manifest, a service worker, and app
icons at fixed root paths (`/manifest.webmanifest`, `/sw.js`,
`/icon-192.png`, `/icon-512.png`) inside the same owner-auth protected
router as `/` and `/assets/{*asset_path}`; none of these add a new
unauthenticated top-level route beside `/pair`. The manifest declares
`display: "standalone"`, `start_url: "/"` (the daemon-served app root), and
`name`/`icons` fields so Chrome/Edge offer the "Install app" affordance from
the served origin. `theme_color` and `background_color` derive from the
dashboard's existing dark-theme CSS tokens rather than invented colors.

The service worker is intentionally a no-precache, network-passthrough
script: it exists only to satisfy the browser's installability heuristic
(a registered worker with a `fetch` listener), not to provide offline
caching. It must not gain `cache.addAll`/precache behavior, since the
dashboard's dev-iteration workflow depends on reloads always reaching the
current build rather than a stale cached one.

Because these resources sit behind owner auth, and browsers fetch
`rel="manifest"` (and its declared icons) with an "omit" credentials mode by
default, the served shell's `<link rel="manifest">` element sets
`crossorigin="use-credentials"` so the owner-auth session cookie
accompanies the manifest/icon fetches. This is only correct because the
protected router's owner-auth check accepts a session cookie (in addition
to a bearer token); a token-only auth boundary could not satisfy a
browser-initiated manifest fetch this way.

## Browser Shortcut Suppression {#260721-ws-dashboard-browser-shortcut-suppression}

The dashboard installs one capture-phase `document`-level `keydown`
listener at the top-level `App` component that calls `preventDefault()`
for a fixed set of Class-A browser shortcuts (Ctrl/Cmd+S/P/F/G/D/O/U/J/R,
zoom Ctrl/Cmd+Plus/Minus/Equals/Underscore/0, and Backspace-as-back-
navigation when the focused target is not an editable field). Ctrl/Cmd+R
is suppressed and reserved for in-app reverse-history-search (terminal and
agent chat inputs); reload remains available via F5 or the browser reload
button. Normal editing/clipboard combos (Ctrl+C/V/X/A/Z/Y) are never
suppressed. The block/allow decision is a pure,
DOM-free predicate (`keydownSuppression.ts`) so the exact set is
unit-tested without a browser DOM; the App effect only reads real event/
target state into the predicate's input shape. This suppressor targets
Class A shortcuts only (interceptable via page-level `keydown`) — Class B
browser-chrome-reserved shortcuts (Ctrl+W/T/N/Tab/1-9) are not addressed
here since they never reach page script in any served mode. It runs
identically in a plain browser tab and the Phase 1 installed PWA
(`260721-ws-dashboard-pwa-installability`).

## Inspectable Navigation Shell {#260516-ws-web-dashboard-inspectable-navigation-shell}

The first browser shell renders the resource view-model contract from the
daemon API. It shows server, workspace, and workRoot location state; loading,
empty, stale, and error states; compact singleton rows; and a reserved
right-side viewer region without implementing the deferred viewer feature. Each
workspace with one workRoot renders as one compact left-nav row selected by the
concrete workRoot id, without depending on main/sub instance presence.
When the workspace and workRoot labels are identical, the compact row displays
that label once; distinct labels remain visible as a workspace/workRoot pair.
Workspaces with multiple workRoots continue to show separate workspace and
workRoot rows. Main/sub instances remain workbench surfaces or projections
rather than default recursive left-nav rows. Sibling workspace rows within a
server and sibling worktree rows within a workspace are user-reorderable by
drag, with the resulting order persisted browser-locally per scope rather than
changing server-reported order.

User-visible dashboard controls expose stable command ids so later keyboard
bindings can target the same behaviors. Representative visible controls route
mouse or click behavior through a shared dashboard command dispatch path, with
a command observer preserving recent-command evidence and programmatic dispatch
using the same command ids as click handlers. Command payloads use logical
dashboard targets such as opaque resource ids, pane ids, logical surface keys,
activity ids, or terminal ids; host paths, cache paths, stream paths, pids, and
backend session paths are not command identity. Terminal raw byte input remains
the narrow exception because shell input fidelity must not be forked through
dashboard commands. The shell reserves `^b` to mean ctrl plus lowercase `b`;
full custom keybinding UI remains out of scope.
{#260523-ws-dashboard-single-workroot-nav-collapse}

## WorkRoot Workbench Substrate {#260516-ws-web-dashboard-workroot-workbench-substrate}

The dashboard frontend presents a `left nav | workRoot workbench` shell. The
left navigation selects server, workspace, and concrete workRoot locations,
while each opened workRoot owns a constrained Dockview-backed workbench area
behind a dashboard-owned adapter.

The workbench uses sibling split groups with compact editor-like tab strips and
dominant pane bodies. Pinned and opened concepts remain dashboard model
concepts: durable surfaces such as agent and persistent terminal views appear as
compact workbench tabs or chips, while transient or support surfaces such as
editor, viewer, diff, diagnostics, logs/events, task view, and inspector
surfaces appear as ordinary workbench tabs. The compact header preserves that
pinned/opened structure without returning to large explanatory rows. Main
instances are durable workRoot-local surfaces. Sub instances are view-only
projections attached to a main instance through badges, popovers, cards, or
drawers rather than independent top-level navigation rows.

Workbench panes do not add a second generic title/status chrome below the
pinned/opened tab rows. The tab rows provide visible surface identity and
selection, while pane-local content or controls provide any useful
surface-specific status.

Layout attachment identity stays separate from daemon resource identity. Layout
state records arrangement only; daemon APIs and `/servers/:serverId/...`
browser routes keep authoritative server, workspace, workRoot, and instance
identity. Panel close follows dashboard surface policy: reversible browser
views detach immediately, while live terminal or agent tab closes require
explicit confirmation before invoking their daemon-backed lifecycle behavior.
PTY/TUI logical columns do not continuously follow visual drag resizing.

Surface opening follows dashboard-owned placement policy: already-open logical
surface keys focus their existing attachment, opened/support surfaces prefer the
second or later split group, and durable agent or persistent terminal surfaces
prefer the focused group before falling back to the first group.

Visible tabs select the active pane and support frontend-only movement such as
reordering within a split group and moving to another split group. Tab movement
changes browser arrangement state only: floating/popout groups stay disabled,
daemon-backed lifecycle stays separate, and PTY/TUI logical dimensions do not
continuously follow visual drag resizing.

Dockview owns the visible workbench group, tab, split-sizing, and pane
attachment layout. Dashboard-owned policy still owns surface identity,
duplicate-open focus, placement, close behavior, restore sanitization, and the
choice to flatten pinned/opened row concepts into Dockview-compatible tab
metadata when a two-row custom tab shell would compete with Dockview ownership.
Synchronization back into Dockview must be group-local: an inactive split
group's selected tab is not treated as inactive merely because another split has
global focus, and pane parameter updates must be keyed by stable content
revisions instead of React node identity so unrelated refreshes do not remount
scrolling pane bodies.

Dockview-created split drops become durable dashboard workbench groups instead
of snapping back to a fixed `primary`/`support` pair. Each opened workRoot owns
its own dynamic group, pane-order, and active-pane state. An opened workRoot
starts from two dynamic groups: terminals prefer group 1, editor/read-only file
panes prefer group 2, editor/file opens create group 2 when only group 1
exists, and groups 3+ remain user-created groups without automatic placement
unless the user explicitly targets them through later policy.

Workbench tabs provide polished lifecycle affordances while keeping Dockview as
the visible tab owner. Pinned/opened hierarchy is visible through
Dockview-compatible tab metadata and pinned-left badge or chip presentation.
Tabs expose hover-only close buttons. Live terminal or agent closes use a
cursor-near `Yes`/`No` confirmation popover; reversible views such as read-only
editor previews, diagnostics, and resource views close immediately and use the
same deterministic focus handoff as ordinary tab close. Opened workRoots do
not show mock or default panes when no live or user-opened surface exists.

### WorkRoot Activity Projection {#260517-ws-dashboard-workroot-activity-projection}

The dashboard exposes a workRoot-owned runtime activity projection for opened
workRoots. Authenticated callers request it through
`GET /api/dashboard/work-roots/{workRootId}/activity`. The projection summarizes
source-neutral Activity for daemon-owned sources without making browser callers
read cache files, host paths, provider records, or ws runtime state directly.

The implemented source today is the legacy named-agent/mercenary compatibility
projection over daemon-owned wsstate and wsagent state. That source reports
bounded status for named agents, including identity, backend or model metadata
when available, current-call state, last-call timing, and unavailable or
diagnostic states for stale or malformed records. It is not the future authority
for provider-native Codex/OpenCode sessions or managed vendor CLI sessions, and
the projection does not expose agent control actions such as start, interrupt,
cancel, erase, or retry. Running command activity remains absent until the async
exec job model exists.

### WorkRoot Activity Top-Bar Badge {#260517-ws-dashboard-workroot-activity-topbar-badge}

Opened workRoot top bars show a compact activity badge in the existing badge
row. The badge summarizes source-neutral Activity state for the selected
workRoot and opens or focuses the detailed WorkRoot Activity pane. The current
implemented counts are derived from the named-agent compatibility projection.

Adding activity summary does not add a new top-bar row or increase the top-bar
height. Under constrained widths the badge compacts, truncates, or hides
secondary text rather than wrapping the toolbar and reducing workbench body
space. Switching workRoots must not briefly render the previous workRoot's
activity state.

### WorkRoot Activity Pane {#260517-ws-dashboard-workroot-activity-pane}

The WorkRoot Activity pane is a reversible workbench surface showing the
selected workRoot's detailed runtime activity projection. Closing it detaches
the browser view immediately without confirmation and without changing daemon
agent state.

Opening the activity detail from the top-bar badge focuses an existing activity
pane for the selected workRoot or creates one through the workbench support
split placement policy. New Activity panes prefer the second/support split when
available or creatable, while duplicate opens focus the existing Activity pane
in whatever split currently owns it. Activity pane close remains reversible and
has no daemon-side effect.

The pane displays source-neutral Activity rows plus any legacy named-agent
compatibility projection rows still required by current UI consumers. It also
shows an explicit empty Running Commands section. Real running-command rows
remain absent until the async exec job model exists.

While the Activity pane is open, the dashboard refreshes recently updated
Activity rows and merges them into the existing projection so newly observed
compatibility named-agent activity appears without a browser reload. The full
projection remains available for the initial selected-workRoot fetch.

## Activity Console Read Model {#260521-ws-dashboard-activity-console-read-model}

The dashboard exposes a workRoot-scoped Activity Console read model that
combines a live/latest Activity Feed snapshot with selected activity transcript
backfill. The existing workRoot Activity endpoint returns selectable Activity
Items for the opened workRoot while preserving a compatibility named-agent
projection for the current Activity pane. A per-item transcript endpoint
returns normalized Transcript Blocks for the selected item. Named agents are the
first supported source, but the public shape stays source-neutral so main-agent
sessions, exec jobs, diagnostics, and later readable activity can fit the same
console contract.

Activity Feed snapshots report enough item state for compact ribbon rendering
without requiring a transcript fetch: stable activity id, kind, label, status,
live/attention flags, timing fields, source display metadata, transcript
availability, bounded diagnostics, selected item hint, feed cursor, and update
mode. Ordering favors active, live, attention, blocked, failed, and recently
updated activity before using alphabetical order as a tie-breaker.

> [!note] Implementation Gap · 2026-07-11 (supersedes 2026-06-20 note)
> Missing behavior: the Activity Console read model is source-neutral, but the
> dashboard does not yet expose dashboard-owned Activity source adapters for
> host-owned agent-client surfaces such as Codex app-server or OpenCode ACP.
> `260620-feat-ws-dashboard-agent-client-activity-sources` Phase 1 formalizes
> the intended source split without implementing any adapter: legacy
> ws-mercenary/named-agent state stays a compatibility source (see
> `#260525-ws-dashboard-sqlite-agent-activity-source` below); Codex app-server
> and OpenCode ACP are the interactive provider sources; OpenCode serve is an
> optional observation-only source (it supplements discovery, but is not the
> primary interactive provider counterpart); and the dashboard Activity model
> — `ActivityFeed`/`ActivityItem`/`ActivityTranscript`/`TranscriptBlock` — stays
> the only shape the browser ever sees, regardless of source. New Codex/
> OpenCode activity must flow through `ActivityFeed.items`, never forced into
> the legacy `agents` compatibility projection. See
> `#260620-ws-dashboard-agent-client-provider-contract` below for the
> dashboard-owned `AgentClientProvider` contract module this split adapts to.
> Updated 2026-07-13: Phase 2 (Codex app-server, `crates/daemon/src/codex_app_server.rs`)
> and Phase 4 (Claude CLI stream-json duplex, `crates/daemon/src/claude_cli.rs`)
> adapters are now implemented, each with their own `AgentClientProvider` impl,
> a provider registry keyed by `(server_id, activity_id)`, a plugin-presence
> spawn gate, dual local/server-scoped `*-sessions` routes, and a merge into
> `ActivityFeed.items` (never the legacy `agents` projection). Phase 3
> (OpenCode ACP) remains unimplemented, blocked pending an OpenCode install for
> its own fixture-verification spike. Future adapters should normalize provider
> thread, turn, message, tool, and status events into Activity Items and
> Transcript Blocks without exposing provider session ids, ws session keys,
> cache paths, process ids, or raw provider event ids as browser authority —
> both implemented adapters were reviewed against exactly this boundary.

Transcript backfill returns bounded normalized blocks rather than backend-native
cache records, raw session JSON, stdout/stderr paths, or file contents. Each
block carries a cursor, timestamp when available, a render kind such as user,
assistant, tool call/result, status, error, or output, and degraded-state
markers. Cursor, block-count, and byte-count bounds keep transcript reads
finite and make unknown activity ids, unavailable sources, empty transcripts,
and malformed records explicit response states instead of whole-feed failures.
Transcript reads default to the latest bounded tail window for the selected
activity. Older transcript history pages backward from the current earliest
loaded cursor so the UI can prepend older blocks when the user scrolls upward,
without forcing an initial read from the beginning of a long transcript.

Browser callers continue to address the model by opaque `workRootId` and
activity id. Responses must not expose host paths, cache paths, backend session
ids, process ids, stdout/stderr paths, stream paths, or backend-native
transcript paths. The read model remains read-only and does not add agent start,
interrupt, cancel, erase, retry, or exec-job control actions.

The dashboard keeps three activity/session identity classes separate:
`activityId` is the browser-facing opaque Activity identifier;
`providerSessionId` is a daemon-private provider-native thread/session/rollout
identifier used only by source adapters; and `wsSessionKey` is a daemon-private
ws MCP credential returned by `ws.ferrule(root)` for future dashboard-launched
top-level harness sessions. Browser routes, command payloads, diagnostic
payloads, and Activity ids must not carry `providerSessionId` or `wsSessionKey`.

### SQLite-Backed Named-Agent Compatibility Source {#260525-ws-dashboard-sqlite-agent-activity-source}

The Activity Console read model currently uses the ws runtime SQLite registry as
the named-agent compatibility metadata source for opened workRoots. Current
named-agent role rows are the source for the compatibility agent projection and
current agent counts, and file-backed payload readers resolve current call
state, output, and transcripts through registry `state_path` metadata rather
than legacy `agent.json` discovery. This source is implemented compatibility
behavior for ws mercenary/named-agent history; it is not the long-term Activity
authority for provider-native Codex/OpenCode adapters or managed vendor CLI
sessions.

Browser-visible routes and payload shapes stay stable. Activity snapshots,
watch stream events, and transcript reads continue to use opaque workRoot and
activity ids, and transcript/output bytes remain normalized by daemon-owned
file-backed transcript readers. Missing, locked, unavailable, or incompatible
registry state degrades to an empty or partial read model rather than failing
the whole route or exposing cache paths, provider session ids, ws session keys,
or raw registry identifiers.

Retained named-agent instance rows add historical Activity Items when their
payloads or diagnostics remain useful. Historical instance items use stable
opaque activity ids distinct from current `agent:<agentKey>` role ids, resolve
transcripts through the instance `state_path`, and do not increase
`ActivityFeed.agents` or current named-agent summary counts. Current,
protected, cleanup-deleted, tombstone/internal, and payload-useless instance
rows stay hidden from historical item and transcript projection.

Activity freshness is registry-aware. Item versions and recent refresh ordering
consider SQLite registry timestamps and cleanup/retention metadata together
with payload mtimes for current-call state, output, runtime logs,
stdout/stderr, and native transcript files. Registry-only updates can produce
Activity item upserts, removals, transcript invalidations, and snapshot
invalidations through the existing event vocabulary, while payload-only
transcript changes continue to update transcript availability.

### Agent-Client Provider Contract And Interactive Source Split {#260620-ws-dashboard-agent-client-provider-contract}

`ws-dashboard-core`'s `agent_client_provider` module (`crates/core/src/agent_client_provider.rs`)
defines a dashboard-owned, ACP-shaped `AgentClientProvider` contract: inert
request/response types and a non-runtime trait covering initialization/
capabilities, session list/create/resume, prompt/send, assistant/user message
and tool-activity events, permission/blocked states, interruption/
cancellation, file-change summaries, transcript backfill, and provider
metadata. Phase 1 of `260620-feat-ws-dashboard-agent-client-activity-sources`
introduced this contract with no implementation; Phase 2 (Codex app-server)
and Phase 4 (Claude CLI stream-json duplex) now implement it, and Phase 3
(OpenCode ACP, blocked pending install) remains outstanding — the contract
exists so all three adapters conform to one reviewed shape instead of each
inventing its own. `AgentClientCapabilities` (`compact`, `steer`, `goal`,
`rewind`, `fork`, `skills`) lets a concrete adapter report which per-harness
capabilities it supports; see
`ai-docs/mental-model/ws-dashboard-agent-harness.md` for the Passthrough/
Overlay/Hack/Unavailable tiering behind each flag.

This module normalizes into the same `items`/`agents` split the read model
above already uses: legacy ws-mercenary/named-agent state remains the
`agents` compatibility projection
(`#260525-ws-dashboard-sqlite-agent-activity-source`); Codex app-server and
OpenCode ACP are interactive `AgentClientProvider` sources whose normalized
output must land in `ActivityFeed.items`; OpenCode serve is an optional
observation-only source, not an interactive provider; and the browser only
ever sees the existing source-neutral `ActivityItem`/`ActivityTranscript`/
`TranscriptBlock` shapes regardless of which source produced a row.

`ActivityItem.kind` / `ActivitySourceDisplay.kind` gain the additive string
values `agent.codex`, `agent.opencode`, and `agent.claude` alongside the
existing `namedAgent`/`exec` values, and `TranscriptBlock.render_kind` gains
`thinking` — extractable reasoning/thinking content (Claude `assistant`
stream thinking segments, Codex reasoning item stream) kept distinct from
ordinary `assistant` text blocks. All three fields stay plain strings, not
closed enums; parsers and tests must keep tolerating unrecognized future
values.

> [!note] {#260714-transcript-block-role-turn-id}
> `TranscriptBlock` gains two additive, optional fields: `role` (a plain
> string — `user`, `agent`, `tool`, or unset for blocks such as `thinking`
> where `render_kind` already disambiguates) and `turnId` (an opaque string
> grouping every block emitted by one logical provider turn, used only for
> browser-side bubble-merge equality checks). Exposing `turnId` does not
> reverse the browser-privacy cursor contract in
> `#260620-ws-dashboard-agent-client-provider-contract` — that contract
> keeps provider session/cache/process identifiers off the wire; a
> per-turn grouping key carries none of that. Both fields are absent on
> blocks a source adapter does not yet populate; existing consumers must
> keep tolerating their absence.
>
> Implemented by `260713-bug-dashboard-agent-chat-transcript-role-turnid-echo`
> Phase 2. Codex's projector (`codex_projection.rs`) derives `role` per
> item type (`user` for `userMessage`/`hookPrompt`, `agent` for
> `agentMessage`, `tool` for command/MCP/dynamic/collab tool items, unset
> for `reasoning`/`fileChange`/`plan`/`contextCompaction`/unsupported) and
> `turn_id` from its existing `current_turn_id`/`order_turn_ids` tracking
> (the provider's own turn id, reused verbatim as the grouping key — an
> explicit, ticket-approved exception to the "never copy provider ids"
> rule, scoped to this field only) — independent of `suppress_local_prompt`
> (fork/resume never calls `send_prompt`, so suppression state never exists
> for a seeded projector). `CodexProjector::seeded` (fork replay) carries
> `role` through but intentionally resets `turn_id` to `None` (fork-of-a-
> fork turn-id resolution stays out of scope). Claude's projector
> (`claude_projection.rs`) only ever emits `role` `agent`/`tool` — never
> `user`, since Claude's stream-json protocol never echoes the client's own
> prompt — and synthesizes `turn_id` as a daemon-local per-turn-boundary
> counter (`claude-turn-N`) at `ingest_assistant`'s existing `turn_started`
> transition, since Claude's protocol carries no native per-turn id.

The Phase-1 frontend interaction-API draft
(`ws-dashboard/frontend/src/activitySessionApi.ts`) records illustrative
method-shape names for the common interactive subset (`activity.history.list`,
`activity.session.start/create/send`, `activity.session.usage` read-only usage
display) and the per-harness-gated methods
(`activity.session.compact/steer/goal.set|get|clear/rewind/fork/skills`), which
a caller must hide/disable unless the active harness's adapter reports the
matching `AgentClientCapabilities` flag. These route under the same dual
server-scoped/local-gateway pattern
(`#remote-activity-git-workspace-operations`) the existing read routes already
use, keeping the same `workRootId`/`activityId` identity model rather than
introducing a new one.

`260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` Phase 1 wires the
frontend to these routes for Codex and Claude sessions: a real fetch client
(`ws-dashboard/frontend/src/activitySessionClient.ts`) replaces the prior
stub-only call sites for session create/resume/history/prompt-send and, where
the active harness's capabilities allow it, steer and fork. OpenCode sessions
remain stub-backed pending Phase 3 of `260620`. `ActivitySessionForkRequest`/
`ActivitySessionForkResponse` gained an optional `cutCursor` field identifying
a specific transcript cut point to fork from rather than only the session's
current end.
Phase 2 of the same ticket replaces the initial one-shot send-then-fetch with
incremental polling: the frontend re-fetches the transcript on a fixed
interval, diffing each poll against the previously-seen block count so only
new or still-in-progress tail content is handed to the UI rather than the
whole transcript every tick, and stops once the daemon-reported transcript
`live` flag goes `false`. This is deliberately a polling upgrade, not a new
SSE/websocket transport — the dashboard's existing `authenticate_websocket_upgrade`
seam stays reserved future work, and polling is an already-accepted transport
pattern in this codebase (mirroring the `workRootActivity` `pollFallback`
mode), not a stopgap unique to chat.

Phase 3 of the same ticket lands the daemon-side Codex fork route the
frontend already targeted: `CodexControlRequest` gains a `Fork { cutCursor }`
variant (`{"action":"fork","cutCursor":...}` on the wire), dispatched by
`codex_session_control` to a new `CodexAppServerProvider::fork` provider
method. {#260713-ws-dashboard-agent-chat-codex-fork-route} Forking spawns a
dedicated new Codex connection for the forked thread rather than sharing the
source session's connection — the daemon has no per-thread notification
demultiplexing, and the underlying `thread/fork` primitive loads the thread
from disk by id, so the source connection need not stay alive. The new
session is seeded with the forked thread's existing turns (via a pure
projection of `thread/fork`'s inline response) so its transcript shows
correct pre-fork history immediately, then continues ingesting live
notifications normally. `cutCursor` is an ordinal transcript-block index on
the wire; the daemon resolves it to the underlying provider turn id
internally (never exposed on `TranscriptBlock`) before calling `thread/fork`.
When resolution fails (a stale or out-of-range cursor), the fork proceeds as
a full-thread fork and the response's `cutCursor` comes back `null` rather
than echoing the request — this reflects what was actually applied, not what
was requested, so callers must not assume a non-null request `cutCursor`
implies a successful cut. Live Claude fork-from-here stays out of scope
(Hack-tier per the capability tiering in
`ai-docs/mental-model/ws-dashboard-agent-harness.md`); no `goal`/`rewind`
`CodexControlRequest` variant exists yet either, despite
`AgentClientCapabilities` already reporting `goal: true`.

## Activity Console UI Shell {#260521-ws-dashboard-activity-console-ui-shell}

The WorkRoot Activity pane renders a reusable read-only Activity Console
instead of a vertical named-agent card dump. The console combines a horizontal
Activity Ribbon for live/latest items with a selected Transcript Block viewer
below it, using the Activity Console read model as its route-backed source.

Ribbon items use a compact three-line shape: small source discriminator text, a
primary name/title line, and small status/recency text. Source discriminator
text identifies the activity channel such as `agent.codex`, `agent.claude`, or
`cmd.exec` rather than repeating the primary title. The status row includes the
current activity status plus relative update time when known; completed activity
may also show bounded elapsed duration when space allows. The text area stays
compact, truncates instead of wrapping, and the ribbon scrolls horizontally at
constrained desktop widths. Live, active, and attention-worthy items use
semantic active styling, and a small short-lived green breathing indicator may
mark newly updated or locally dirty items until the user selects or otherwise
acknowledges them. The Activity Console body does not render a separate summary
chip row above the ribbon; the ribbon is the primary item selector.

The browser may keep a local acknowledgement watermark per workRoot/activity
item. On initial feed load it compares that local watermark with daemon item
timestamps or cursors to mark newly updated items dirty. Selecting or explicitly
acknowledging an item clears only browser-local dirty state; the daemon does not
gain read-receipt authority.

Selecting a ribbon item renders normalized transcript blocks. Agent activity
renders as action-unit blocks where dialogue and assistant output are expanded
by default, while tool calls, MCP activity, and command runs default to one-line
summaries with inline detail expansion. Compact summaries prefer bounded
semantic content from normalized safe fields and first-line text over generic
category titles, so tool calls can show the tool name and argument-size hint,
tool results can show outcome/status/byte hints, and degraded records can show
their omission reason without exposing raw native payloads. Exec activity
renders as terminal-style output. Transcript views follow the tail by default
for newly selected or live updated activity. When the user scrolls away from the
tail, the browser preserves that scroll position across feed refreshes,
transcript refreshes, selected-transcript invalidations, and workbench split
rerenders until the user returns to the tail. Initial selected transcript loads
start from the latest tail window, not the oldest block. Older transcript
history is loaded when the user scrolls near the top and is prepended while
preserving the user's visible position. Explicit refresh or load-more controls
remain available for fallback and error states rather than being the primary
navigation path.

> [!note] Implementation Gap · 2026-05-23
> Missing behavior: expanded dialogue and assistant transcript text is rendered
> as plain preformatted text rather than markdown. The dashboard should define a
> shared markdown rendering component first, then apply that component
> consistently across Activity Console messages and other project surfaces that
> render trusted normalized markdown.

Visible Activity Console controls expose stable command ids and route their
clicked behavior through the dashboard command dispatch path so later keyboard
bindings can invoke the same behavior. The shell remains read-only, does not add
agent control buttons, and does not consume live SSE/watch streams until the
live UX child implements that behavior.

## Activity Console Watch Stream {#260521-ws-dashboard-activity-console-watch-stream}

The dashboard exposes a workRoot-scoped read-only Activity Console event
stream for feed and transcript invalidations:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}
```

The stream is owner-authenticated before any transport is accepted. It uses SSE
because Activity Console updates are read-only; a different transport requires
a recorded bidirectional need. Subscriptions are scoped to the requested opened
workRoot instead of every remembered or opened root. The current backend stream
announces `pollFallback` mode and uses bounded polling to produce event updates;
a later native watcher can switch to `watch` mode without changing the public
payload vocabulary.

Stream events carry source-neutral Activity Feed semantics rather than
filesystem or backend-native payloads. Expected event categories include item
upsert/removal, transcript invalidation with transcript cursor metadata,
snapshot invalidation for overflow or watch resets, mode changes between watch
and polling fallback, and heartbeats. Event cursors let reconnecting callers ask
for events after the last observed cursor, but the stream may intentionally
force a snapshot refetch when events were missed or coalesced.

The fallback stream normalizes observed agent changes, missing directories,
agent erasure, and recreated agent directories without leaking cache paths, raw
file paths, backend-native transcript records, process ids, session ids,
stdout/stderr paths, or file contents. The stream remains read-only and does
not make the frontend responsible for consuming live updates; frontend merge
and stale-root behavior belongs to the live UX feature.

## Activity Console Live UX {#260521-ws-dashboard-activity-console-live-ux}

The Activity Console frontend subscribes to the workRoot activity event
stream while the console is visible or otherwise actively used. Stream handling
merges source-neutral events into the route-backed Activity Console state
without making browser state authoritative over daemon activity.

`itemUpserted` and `itemRemoved` events update the current feed while preserving
selection when the selected item still exists. `snapshotInvalidated` causes a
bounded read-model refetch instead of browser-side reconstruction of missed
events. `transcriptUpdated` refreshes or backfills transcript state only when
the affected activity is the currently selected item. `modeChanged` transitions
the frontend between stream-driven updates and bounded fallback polling; always
on full-list polling is not the normal live mode.

The frontend ignores events for stale workRoots after the user switches
roots or closes the console, tears down subscriptions when the console is no
longer visible, and keep the static UI shell usable when the stream is
unavailable. Streamed or polled updates newer than the browser-local
acknowledgement watermark may turn on the ribbon dirty cue; selecting or
acknowledging an item clears only local dirty state and sends no daemon read
receipt.

Live UX adoption remains read-only. It does not expose raw SSE payloads, backend
paths, cache paths, source ids, or control actions in browser UI state.

## Activity Console Transcript Expansion {#260522-ws-dashboard-activity-console-transcript-expansion}

The Activity Console transcript backend supports additional daemon-owned
transcript source adapters behind the existing `ActivityTranscript` and
`TranscriptBlock` contracts. Browser callers continue to request selected
activity transcripts by opaque workRoot and activity ids; they never receive
backend session paths, cache paths, host paths, pids, session ids,
stdout/stderr paths, stream paths, native transcript paths, or backend-native
record formats.

`TranscriptBlock.render_kind` gained a `thinking` value in
`260620-feat-ws-dashboard-agent-client-activity-sources` Phase 1
(see `#260620-ws-dashboard-agent-client-provider-contract`) for extractable
reasoning/thinking content, distinct from ordinary `assistant` text blocks;
Phase 1 only adds this vocabulary value, it does not add a Claude transcript
parser.

Native backend transcript parsing starts only from fixture-backed formats whose
shape can be verified without invoking a live backend. Codex native session
JSONL is the first supported native source. Claude and Gemini native transcript
handling remain deferred unless their formats are similarly documented or
fixture-backed. Missing, unreadable, malformed, or unsupported native transcript
records degrade individual blocks or source status where possible instead of
failing the whole selected activity transcript, and the existing `output.md`
fallback remains available.

Source adapters normalize dialogue, assistant output, tool calls, tool results,
status/error entries, prompt/user messages, interruptions, handoff/status
records, MCP/tool activity, patch/apply outcomes, and command/output-like
records into bounded `TranscriptBlock` values when their fixture-backed native
shape is known. Raw backend JSON or markdown may be adapter input but is not the
browser contract. Low-value native telemetry may be skipped instead of rendered
as transcript noise. Remaining unsupported records degrade into bounded
structural summaries with omission reasons, never raw JSON, private record
strings, payload snippets, paths, session ids, or tool output. Exec transcript
source integration remains blocked until the async exec output reader model
exists.

The backend continues to use feed-level `transcriptUpdated` invalidations plus
bounded selected backfill for live transcript updates. It does not expose a
selected-activity block-level transcript event stream until block append/update
behavior has a clear UX win.

## Dark-First Visual System {#260516-ws-web-dashboard-dark-visual-system}

The dashboard frontend provides a dark-first visual baseline for the protected
browser shell. Callers see a dashboard-specific `DESIGN.md` guide under the
frontend package, semantic theme tokens instead of scattered literal colors,
and a shell reskin that preserves existing resource, loading, stale, error, and
command behavior while presenting a consistent dark operational interface.

The visual system uses `ai-docs/ref/design.md` as a Carbon-inspired density,
geometry, hairline, and component reference rather than as a default light
palette. Desktop and narrow viewport screenshot checks make the resulting
shell inspectable before larger workbench surfaces depend on it.

The frontend visual guide also defines a dashboard-local building-block
vocabulary for frames, panels, panes, toolbars, rows, chips, badges, state
surfaces, document surfaces, and code blocks. Current high-impact surfaces
consume that vocabulary while preserving their existing commands and data:
left navigation, open-workRoot chrome, workbench toolbar and Dockview tabs,
Activity Console ribbon/transcript blocks, read-only text panes, and common
empty/loading/error surfaces.

The dashboard chrome presents left navigation, file explorer, and workRoot
topbar controls with conventional icon-first affordances, accessible names, and
reduced visible metadata clutter. High-signal status remains visible,
secondary diagnostics move to low-weight surfaces or overflow menus, and all
visible or overflowed actions continue to route through the existing dashboard
command model. Ready navigation rows remain low-height without status chips,
compact resource glyphs do not overlap, power state reads through symmetric
icon color rather than filled action backgrounds, topbar chips and overflow
menu labels preserve readable semantic text, WorkRoot Activity ribbons reserve
bright text for the primary label, and icon buttons expose most border or
glass-like treatment only on hover/focus/active states while retaining stable
dimensions and accessible names. {#260524-dashboard-icon-first-chrome}

The dashboard distinguishes large working context regions through a surface
hierarchy rather than uniform hairlines. Left navigation, workRoot topbar
chrome, Dockview split groups, editor/document shells, pane-local ribbons, and
content bodies use separate semantic surface and divider roles so structural
boundaries, local dividers, selection states, and hover/focus control chrome do
not compete visually. Editor/document panes read as one context island whose
tabbar, internal ribbon, and body belong together, while Dockview split
boundaries use a stronger structural gutter or surface contrast than local
tab/body dividers. {#260524-dashboard-context-surface-hierarchy}

## Browser UI Acceptance Gate {#260516-ws-web-dashboard-browser-ui-acceptance-gate}

Dashboard frontend changes that affect visible browser behavior provide a
browser-level acceptance gate against the daemon-served production frontend
after owner pairing. The gate exercises the workRoot UI as a user sees it:
opening a real workRoot, browsing files, creating terminals, switching terminal
tabs, sending terminal input, observing terminal output, and checking pane
layout at recorded viewport sizes.

The frontend package exposes this gate through `npm run test:browser`. The gate
builds the production frontend, serves it through the dashboard daemon, pairs
as owner through the startup pairing URL, and records textual evidence plus
regenerable screenshot artifacts outside tracked source.

The gate includes viewport containment checks for long file explorer content:
expanding a large tree must not make the top-level document scroll or push the
dashboard footer out of view, and overflow must stay inside the explorer region.

The browser gate proves the live terminal path uses an owner-authenticated
WebSocket connection instead of periodic output polling while connected. It
covers owner pairing, WebSocket connection, input fidelity, ANSI/control
rendering, resize behavior, close-as-terminate, reconnect or reload
reconstruction, and timing evidence showing local keystroke echo is no longer
bounded by the former polling interval.
{#260516-ws-web-dashboard-terminal-websocket-browser-gate}

For workbench layout changes, the browser gate also proves that the visible
workbench is Dockview-backed rather than a parallel custom tab/split shell. The
assertion checks for the dashboard's stable Dockview owner marker and Dockview
DOM beneath it, and it rejects the retired `.workbench-splits > .workbench-group`
layout as the visible workbench authority.

Workbench split browser evidence verifies that a Dockview split-drop preview
corresponds to durable dashboard behavior: dragging a tab into a new split
target creates or maps a dashboard group, the pane remains there after React
synchronization, ordinary file/terminal interactions still work in the
resulting layout, and opening a second workRoot does not leak the first
workRoot's user-created groups or active panes. Split-scroll evidence also
keeps a scrolled pane away from the top across refresh-driven synchronization.

Workbench tab polish evidence is browser-level Playwright evidence against the
daemon-served frontend. It covers hover-only close affordances, terminal and
agent close confirmation popover cancel/confirm paths, immediate close for
reversible panes, pinned/opened badge or chip presentation, preview-to-pinned
file behavior, and default spawned-daemon agent close coverage. The
implementation workflow also runs a post-implementation frontend-design
verification and autonomous tweak pass before ordinary implementation review,
then reruns the relevant browser evidence.

Read-only text pane scroll containment and terminal input fidelity evidence are
browser-level Playwright evidence against the daemon-served frontend. The gate
covers long read-only file scrolling without top-level document scroll,
shell-visible `ctrl-u` and `ctrl-w` line-editing behavior, WebSocket input
frames for those controls, committed Hangul text input reaching the shell, and
a synthetic IME composition guard proving that composition-in-progress fallback
keystrokes are not forwarded as raw terminal input.

Pure TypeScript helper tests, Vite builds, route tests, curl evidence, and
fixture-only dogfood do not by themselves close UI-facing dashboard work. When
automated browser tooling cannot run, the verification artifact records exact
manual browser steps, viewports, screenshot or trace paths when generated, and
pass/fail observations.

## Deterministic Terminal Endpoint Harness {#260516-ws-web-dashboard-terminal-deterministic-endpoint-harness}

The daemon-served browser acceptance harness supports deterministic daemon
endpoints for terminal portability verification. Spawned mode can run the
dashboard daemon on an explicit host, bind mode, port, daemon binary path, and
static asset directory through `WS_DASHBOARD_DAEMON_HOST`,
`WS_DASHBOARD_DAEMON_BIND_MODE`, `WS_DASHBOARD_DAEMON_PORT`,
`WS_DASHBOARD_DAEMON_BIN`, and `WS_DASHBOARD_STATIC_DIR`. External mode can
attach the browser gate to an already-running base or pairing URL through
`WS_DASHBOARD_DAEMON_BASE_URL` or `WS_DASHBOARD_DAEMON_PAIRING_URL`. When the
daemon runs on a different host from Playwright, the gate can use
`WS_DASHBOARD_TEST_WORKROOT` to open a fixture path that exists on the daemon
host instead of creating a local Playwright-host temporary directory. Browser
checks that need a second opened workRoot can use
`WS_DASHBOARD_TEST_SECOND_WORKROOT`; when attaching to an external daemon
without that second reachable path, only the second-root isolation substep is
skipped while the rest of the browser gate still runs.

The same browser acceptance flow can target a native Windows daemon running on
remote loopback behind SSH local forwarding. The harness waits for an owner
pairing URL and a reachable HTTP readiness signal before starting browser
assertions. Failures identify the failing layer, such as daemon startup,
forwarding or endpoint reachability, pairing, readiness, or browser
assertions, while redacting private endpoints, hostnames, paths, and pairing
tokens from diagnostics.

## Terminal Cross-Platform Evidence {#260516-ws-web-dashboard-terminal-cross-platform-evidence}

Terminal portability runs record durable evidence for each supported
environment exercised during implementation. The evidence identifies the OS,
shell profile, daemon endpoint mode, forwarding path when used, readiness
signal, browser gate result, terminal commands or fixtures used, and any
explicit OS-scoped limitations. Machine-readable evidence stays in ignored
browser-test artifacts, and tracked dogfood summaries omit private endpoint,
user, host, path, pairing-token, and screenshot details.

Native Windows evidence may use a machine-local SSH host recorded outside
tracked source. If native Windows evidence cannot run, the evidence states the
exact blocker and records the result as an explicit gap instead of treating a
POSIX local gate as native-Windows coverage.

## Local WorkRoot Discovery Provider {#260516-ws-web-dashboard-local-workroot-discovery-provider}

The dashboard daemon provides a live local discovery provider that maps opened
physical directories into the resource view-model contract. The provider
classifies workRoots as `plainDirectory`, `gitPrimaryRoot`, or
`gitLinkedWorktree`, reports online, offline, moved, and inaccessible states,
and preserves stable daemon-owned identity when a workRoot's discovered kind
changes.

Discovery refreshes through explicit owner actions that invoke the provider,
including opening a workRoot through the root picker backend. Broad filesystem
watching remains out of scope for the first visible substrate.

## Root Picker Empty Directory Creation {#260516-ws-web-dashboard-root-picker-empty-directory-creation}

The dashboard exposes backend support for a cross-platform root picker that
lists filesystem locations as workRoot candidates without turning the browser
into a general file manager. Authenticated owners can open existing plain
directories or Git-backed directories into the dashboard model.

The picker includes only a narrow `Create empty folder` operation for creating
a new workRoot candidate. Generic delete, rename, move, copy, and recursive
folder deletion operations remain unavailable.

The browser shell opens workRoots through an owner-authenticated root picker
modal instead of an always-visible path input. The left navigation exposes an
`Open...` entrypoint; the modal is an explorer-style folder selection dialog
with local Back/Forward history, Up and Refresh actions, an address field,
platform-aware built-in places derived from daemon-owned data, a details-style
current-folder row list, and footer controls for opening the selected or typed
path. Directory rows support keyboard-friendly selection and row action, and
closing the modal restores focus to the opener. Opening a selected or exact
directory continues through the `workRoot.open` command path, open-workRoot API,
daemon-opened workRoot id reconciliation, and canonical resource refresh flow.
The modal may expose only the existing single-segment `Create empty folder`
action, not broad file-manager operations. Host paths remain authenticated
picker/open request data rather than loggable command payload fields.
{#260524-ws-dashboard-root-picker-modal}
{#260524-ws-dashboard-react-aria-root-picker-pilot}

Authenticated owners can pin and unpin root picker directories in the picker
sidebar. Pinned directories are stored in daemon-local dashboard persistence,
are visually distinguished from built-in places, and remain selection or
navigation affordances only. Unavailable pinned directories stay visible in a
degraded state so the owner can remove them without the picker exposing private
diagnostics or silently creating, opening, deleting, renaming, or moving
filesystem resources. Pin and unpin controls keep host paths as authenticated
request data rather than loggable command payload fields.
{#260524-ws-dashboard-root-picker-pins}

After an authenticated owner opens a workRoot, the browser-visible resource
tree refreshes from the canonical dashboard resources endpoint and selects the
real opened workRoot instead of continuing to present mock workspace state.
Open-workRoot responses may update the view immediately, and successful
responses include an `x-ws-dashboard-opened-work-root-id` header identifying
the daemon-owned id for the requested root. The resources endpoint remains the
canonical source for subsequent refreshes.
{#260516-ws-web-dashboard-open-workroot-resource-refresh}

## WorkRoot File Listing API {#260516-ws-web-dashboard-workroot-file-listing-api}

The dashboard exposes an authenticated API for listing directories below a
selected workRoot. Responses identify children by daemon-owned workRoot-relative
location data rather than raw host paths, distinguish file and directory
entries, expose basic readability or preview eligibility when cheap, and report
unreadable or inaccessible locations without mutating the filesystem.

Listing requests remain rooted below the selected workRoot. Traversal attempts,
missing paths, files requested as directories, and inaccessible locations return
bounded unavailable or error states without exposing host paths as browser route
identity.

## WorkRoot File Explorer {#260516-ws-web-dashboard-workroot-file-explorer}

The dashboard browser shell renders a selected-workRoot file explorer in the
lower portion of the left navigation area. The explorer supports directory
expansion, explicit refresh, loading, empty, and error states while keeping
server, workspace, and workRoot identity visible above it.

The first explorer surface is navigation-only. It does not offer delete,
rename, move, copy, chmod, recursive folder deletion, or broad file-manager
operations. Readable file open actions may hand off to read-only text pane
behavior when that later surface exists; until then, the explorer does not imply
write-back editing.

The explorer presents conventional tree/list affordances that visibly
distinguish files from directories, make expansion and refresh controls
recognizable, keep the selected workRoot identity visible, and avoid hidden or
nonstandard interactions while staying read-only. Long expanded trees scroll
inside the explorer region instead of growing the whole browser document.
{#260516-ws-web-dashboard-file-explorer-conventional-affordance}

## Read-Only File API {#260516-ws-web-dashboard-readonly-file-api}

The dashboard exposes an authenticated API for reading previewable text files
below an opened workRoot. Callers address files by opaque `workRootId` and
workRoot-relative location data from the file listing API. The route rejects
traversal, missing files, directories, unreadable paths, unsupported binary
content, and oversized files with bounded unavailable states.

Successful responses include read-only text content and enough metadata for the
browser to render a stable viewer title, language or extension hint when cheap,
size information, and read-only status without exposing absolute host paths.

## Read-Only Text Pane {#260516-ws-web-dashboard-readonly-text-pane}

The dashboard workbench can open a read-only text pane for a previewable file
under the selected workRoot. The pane renders file content as an inspectable
viewer/editor body and clearly indicates read-only status. Opening the same
file focuses the existing logical pane instead of duplicating it by default.
File explorer single-click opens or replaces one read-only preview tab for the
selected workRoot. Double-click pins that file as a stable opened tab that
later preview opens do not replace. Reopening an already pinned file focuses
that pinned tab.

The text pane does not provide save, dirty-state, formatting, rename, delete,
move, copy, conflict handling, or language-server behavior.

Long read-only file content scrolls inside the text pane without moving the
top-level browser document, displacing dashboard chrome, or requiring a future
editor replacement to prove containment.
{#260517-ws-dashboard-readonly-text-scroll-containment}

## Document Viewer Mode {#260524-ws-dashboard-document-viewer-mode}

The dashboard will present previewable documents through a reusable document
viewer mode instead of treating every file as raw preformatted text. A document
pane owns one workRoot-relative source attachment and a pane-local
`view | edit` mode control. View mode is format-aware and read-only; edit mode
is visibly reserved but disabled until the raw-text edit/save feature lands.
Switching document presentation mode does not create a second workbench tab for
the same document.

Markdown documents render through a real Markdown AST pipeline rather than a
hand-rolled parser. The initial Markdown viewer supports polished GFM table and
task-list rendering, Obsidian-style callouts such as `> [!note]`, and bounded
footnote or footer hover affordances while keeping raw HTML disabled or safely
ignored until a later sanitized or sandboxed HTML feature exists.

The viewer exposes a block model that callers can address independently from
rendered React nodes. Blocks include stable-in-content ids, ordinal position,
kind, original markdown, plain text, translatability, and line ranges when
available. Ordinary soft line breaks remain part of one prose block, list items
are separate blocks, and non-prose blocks such as fenced code may be marked
non-translatable. Block-level actions copy the visible text or a workRoot-
relative path reference such as `@path/to/file.md#L12-L18`; copied path
references never include absolute host paths.

The viewer accepts local translation overlay data keyed by the current content
hash and block id so later daemon translation results can reuse the same
rendering path. Without a real daemon translation result, translated-copy
actions remain pending or unavailable.

## Document Translation Overlay {#260524-ws-dashboard-document-translation-overlay}

Markdown view mode exposes a pane-local translation toggle next to the
view/edit control. When the toggle is enabled, opening or focusing the pane
requests whole-document translation for the current immutable content hash.
Translated blocks overlay the viewer by replacing each block's rendered
content as results become available. Hovering a translated block temporarily
shows the original block. Selecting one or more blocks exposes copy actions for
the currently visible text, translated text when available, and pathrefs.

Translation requests are daemon-owned operations. The frontend builds the
document block set and sends it with full document context; the daemon owns
provider configuration, model discovery, prompting, bounded output parsing, and
SHA256/content-hash cache behavior. The first provider shape is an
OpenAI-compatible LLM provider, suitable for a local Ollama endpoint, while the
provider union leaves room for future non-LLM translation APIs. Provider
configuration is daemon-side; the browser can observe bounded configured,
reachable, model, cache, and per-block status without receiving API keys,
prompts, raw model output, or daemon cache paths.

LLM translation roundtrips preserve block identity. Requests contain
`blockId + content` pairs, and successful responses return matching
`blockId + translatedContent` pairs. Missing, duplicate, unknown, or
unparseable block ids become bounded block-level failure states rather than raw
model output in the browser.

## Document Edit And Save Fan-Out {#260524-ws-dashboard-document-edit-save-fanout}

The dashboard provides a raw-text edit mode for editable workRoot files while
keeping formatted view mode read-only. Document reads return source identity,
content hash, media or renderer hints, edit capability, size, and content.
Document writes use optimistic concurrency through the read content hash and
return a fresh content hash after a successful save.

Open panes for the same `workRootId + path` receive save and external-change
updates by document source identity instead of pane identity. Clean panes can
re-read or update to the new content hash after another pane saves. Dirty edit
panes are marked stale or conflicted without silently overwriting user edits.
Per-workRoot document event streams publish content-change and watch-invalidated
events; filesystem watchers are freshness hints only, with focus and visibility
re-reads plus content-hash checks remaining the correctness fallback.

## File Open Placement Policy {#260516-ws-web-dashboard-file-open-placement-policy}

File-open commands from the workRoot file explorer use workbench placement
policy that prefers the second or later split group when available, so active
terminal or future agent work is not displaced. Placement remains browser
arrangement state; file content authorization and preview availability remain
daemon-owned.

## Terminal Registry And PTY Spawn {#260516-ws-web-dashboard-terminal-registry-pty-spawn}

The dashboard daemon owns shell terminal sessions scoped to opened workRoots.
Authenticated owners can create and list live terminal sessions by opaque
terminal ids. Spawns run in the selected workRoot directory and terminal ids are
not process ids or host paths.

Live terminal sessions persist across browser refresh because the daemon owns
their lifecycle. Browser arrangement state controls where sessions are shown,
not whether the daemon session exists.

## Terminal I/O Transport {#260516-ws-web-dashboard-terminal-io-transport}

The dashboard exposes authenticated terminal output, input, status, and resize
transport for daemon-owned PTY sessions. Unauthenticated callers are rejected
before stream or upgrade acceptance. Resize forwarding remains bounded and does
not continuously rewrite logical terminal dimensions during visual split drag.

Live browser terminal I/O uses an owner-authenticated WebSocket as the primary
transport for daemon-owned PTY sessions. The WebSocket attaches to existing
opaque terminal ids after owner auth, carries ordered PTY output, status, and
exit data to the browser, and carries raw input plus bounded resize requests
back to the daemon. If the owning workRoot goes offline or becomes unavailable,
the WebSocket stops accepting client input and stops sending buffered or live
PTY output. HTTP output transport remains available for initial replay, reload
reconstruction, deterministic tests, or fallback, but the normal connected
xterm path does not depend on periodic output polling.
{#260516-ws-web-dashboard-terminal-websocket-transport}

## Terminal Pane {#260516-ws-web-dashboard-terminal-pane}

The dashboard workbench renders daemon-owned terminal sessions in terminal panes
for the selected workRoot. Creating a terminal opens or focuses a terminal pane,
and refresh can reconstruct visible terminal panes from daemon live session
state plus browser arrangement where available.

The terminal pane is a shell terminal substrate only; it does not hardcode
Codex, Claude, or other agent presets.

Terminal tab labels behave as selectable workbench tabs for every visible
terminal session. Opening a real workRoot shows an explicit empty workbench
state or a live daemon terminal surface, never a mock or placeholder terminal.
Selecting a terminal focuses only that session, and terminal input and output
do not cross between sessions.
{#260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state}

## Browser Terminal Emulator Behavior {#260516-ws-web-dashboard-browser-terminal-emulator-behavior}

The browser terminal pane behaves as a real terminal emulator surface for a
daemon-owned PTY. PTY output is delivered into the terminal emulator so ANSI
color and control sequences render as terminal behavior rather than raw text.
Keyboard input originates from the focused emulator surface and reaches the
corresponding daemon terminal session.

The terminal fills the available workbench pane and fits or resizes from
measured container dimensions while staying within the daemon PTY size
contract. Resize forwarding remains bounded; visual split dragging does not
continuously rewrite logical PTY dimensions.

Terminal rendering prefers a Powerline/Nerd Font-capable monospace stack when
available, with ordinary monospace fallbacks. HTTP polling is suppressed while a
terminal WebSocket is connecting or connected; fallback polling avoids idle
terminal state churn, discards stale in-flight poll results after socket attach
or cursor advancement, and uses bounded per-terminal in-flight requests.

The browser terminal emulator preserves byte-stream input behavior for ordinary
shell editing and interactive control keys. Acceptance includes Backspace,
left/right cursor movement, command history navigation, Ctrl-C, Ctrl-D or EOF
where safe, Ctrl-L or clear-screen behavior, paste, and ordinary prompt editing
inside a real shell.
{#260516-ws-web-dashboard-terminal-websocket-input-fidelity}

Focused terminal panes preserve native terminal input fidelity for committed
Hangul text, IME fallback guarding, and shell line editing. Committed Hangul
text reaches the shell through the live terminal path,
composition-in-progress keystrokes are not forwarded as raw bytes by fallback
browser handlers, and shell editing controls such as `ctrl-u` and `ctrl-w`
produce their native shell-visible effects through the live terminal path.
Focused terminal panes keep browser focus on the xterm input target across
ordinary input, Enter, shell output, and committed text input unless the owner
interacts outside the terminal surface.
{#260517-ws-dashboard-terminal-ime-and-line-editing-fidelity}

## Terminal Shell Selection Portability {#260516-ws-web-dashboard-terminal-shell-selection-portability}

Dashboard terminal spawning has an explicit, testable shell-selection contract
across supported platforms. Unix-like platforms use `$SHELL` or the `/bin/sh`
fallback. Native Windows prefers `pwsh.exe`, then `powershell.exe`, then
`%COMSPEC%`, and finally the `cmd.exe` fallback. The selection contract is
testable independently from the compile-time host platform so Unix and Windows
fallback behavior can be verified on any developer machine.

Browser-backed PTY sessions do not blindly inherit an unusable daemon launch
terminal type. If the daemon process has no `TERM`, an empty `TERM`, or
`TERM=dumb`, new dashboard terminal sessions use `xterm-256color` so shell
programs see terminal capabilities compatible with the browser emulator.
Explicit non-dumb parent `TERM` values are preserved.

Shell spawn failures stay bounded to recoverable diagnostics. Authenticated
terminal creation may report that terminal spawning failed, but private
workRoot host paths are not exposed to unauthenticated callers.

## Platform-Aware Terminal Command Helpers {#260516-ws-web-dashboard-terminal-platform-command-helpers}

Terminal tests and browser acceptance gates express portable terminal intent
through platform-aware helpers rather than shared POSIX command strings.
Acceptance behaviors such as echo, line editing, paste, clear screen,
interrupt, resize, ANSI/control rendering, scroll output, and terminal
isolation map to shell-appropriate commands for Unix shells, `cmd.exe`, and
PowerShell where practical.

External daemon browser gates require an explicit remote command profile
through `WS_DASHBOARD_TERMINAL_SHELL_PROFILE` or target platform hint through
`WS_DASHBOARD_TERMINAL_PLATFORM`, so a locally running Playwright process does
not silently use POSIX commands against a remote native-Windows shell. Any
behavior that cannot be made equivalent on a supported platform carries an
explicit OS or shell limitation and is not presented as native-Windows evidence.

The terminal surface keeps scrolled output and alternate-screen/fullscreen TUI
content within the visible terminal box: the active bottom row must remain
fully visible, and fitted xterm rows are trimmed when the rendered screen would
otherwise exceed the available surface.

## Terminal Close Terminates Session {#260516-ws-web-dashboard-terminal-close-termination}

Closing a terminal panel explicitly terminates its daemon-owned terminal
session after inline `Yes`/`No` confirmation near the close action. Cancel
leaves the terminal open and focus coherent; confirm preserves the
close-as-terminate behavior. Hidden detached restore UX remains absent.

## WorkRoot IO Restore Model {#260516-ws-web-dashboard-workroot-io-restore-model}

The dashboard combines daemon-owned live terminal state, read-only file pane
state, and browser workbench arrangement into one restore model for selected
workRoots. Daemon state is authoritative for live terminal existence, while
browser arrangement remains presentation state. File panes restore only when
the file remains previewable; otherwise the pane shows an honest unavailable
state. The daemon persists the owner's opened workRoot paths in local dashboard
state and seeds the live resource view from that remembered list on startup.
Remembered roots re-run normal discovery instead of bypassing moved, offline,
inaccessible, primary-root, or linked-worktree classification. Auth sessions,
live terminal process survival, and Activity acknowledgement state remain
outside the restore model.

Per-work-root browser workbench arrangement — dockview group membership, tab
order, active pane per group, and split proportions on a best-effort basis —
is persisted to browser-local storage keyed by server route and workRoot id,
and restored on reload (and on reopening a work root closed via the explicit
close action, per a later phase). Restore never treats persisted layout as
authoritative over live daemon/resource state: every pane reference in a
persisted layout is revalidated against currently-available resources, and an
unavailable reference (a file pane whose file is no longer previewable, or a
terminal pane with no matching daemon-alive terminal or restore intent) is
silently dropped from the restored layout rather than shown as an error,
consistent with this anchor's existing file-pane restore rule.

Browser-visible terminal tab descriptors can restore after daemon restart as
newly created daemon terminal sessions attached to the remembered workRoot. The
restore descriptor carries title plus a workRoot-relative cwd hint, but it does
not treat old daemon terminal ids or PTY processes as resumable state.

When the frontend instead reattaches to a still-alive daemon terminal by id on
reload, it restores that pane's visual appearance rather than replaying only
plain text: each terminal pane's serialized scrollback buffer (text, cursor
position, and styles) and scroll viewport offset are captured into a bounded,
debounced browser-local snapshot keyed by serverRoute + workRootId + terminal
id. On reattach, the freshly created terminal surface writes the matching
snapshot's serialized buffer and restores its scroll offset, then the existing
delta-cursor mechanism resumes the live WebSocket from the sequence captured
alongside that snapshot to catch up on any output the daemon produced since -
surfacing the same truncation gap marker as any other stale-cursor reconnect
if the daemon's retained output no longer covers that gap. This visual
snapshot is never treated as authoritative over live daemon state: a
reattached terminal with no matching snapshot, and a new session spawned via
the restore-intent fallback above, both still start from an empty buffer.
{#260523-ws-dashboard-terminal-tab-restore}

## WorkRoot IO Command And Placement Polish {#260516-ws-web-dashboard-workroot-io-command-placement-polish}

WorkRoot IO commands use consistent command ids and placement behavior across
file open, create terminal, focus existing surface, close terminal, and refresh.
Logical targets that are already open focus existing surfaces rather than
duplicating panes.

## WorkRoot IO Dogfood Verification {#260516-ws-web-dashboard-workroot-io-dogfood-verification}

The dashboard verifies the workRoot IO workflow through the daemon-served
frontend: open/select a workRoot, browse files, open a read-only text pane,
create and use a terminal, refresh without losing the terminal, close the
terminal, and inspect desktop and narrow layouts. Verification records exact
tooling blockers when a check cannot run.

WorkRoot IO acceptance verification starts from the default dashboard resource
load, opens or selects a real workRoot, and proves the browser-visible resource
tree, file navigation, read-only text pane, and terminal session all operate
against that real workRoot rather than mock fixtures.
{#260516-ws-web-dashboard-live-resource-dogfood-verification}

WorkRoot IO dogfood includes browser-level evidence from the daemon-served
production frontend after owner pairing. The artifact records the daemon
command, browser automation or manual browser steps, viewport sizes, terminal
commands used to verify color/control handling, generated screenshot or trace
paths when present, and explicit pass/fail checks for the known UI failures.
{#260516-ws-web-dashboard-browser-workroot-io-dogfood-evidence}

## Instance Event Envelope Fixtures {#260516-ws-web-dashboard-instance-event-envelope-fixtures}

The dashboard defines a shared event envelope for instance-scoped streams.
Events reference opaque server, workspace, workRoot, and instance ids from the
resource view-model contract, carry ordered cursor and sequence data,
timestamps, event categories, payload values, and explicit error or end markers.

Deterministic transcript fixtures cover ordinary output, status transitions,
errors, reconnect/backfill, and empty streams so later PTY, named-agent, exec,
diagnostic, viewer, and translation features can reuse one stream shape.

## Authenticated Instance Event Stream Scaffold {#260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold}

The dashboard exposes an authenticated stream route scaffold that serves
fixture-backed instance events before live PTY, named-agent, exec, diagnostic,
viewer, or translation sources exist. Unauthenticated callers are rejected
before stream acceptance or WebSocket upgrade behavior.

Authenticated callers can request events after a cursor and receive
deterministic fixture events without making the dashboard daemon the ws MCP or
named-agent session authority.
