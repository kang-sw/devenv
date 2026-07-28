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

A second, distinct entrypoint class also sits outside the owner-session
boundary: the per-terminal turn-state callback route
(`POST /api/dashboard/terminals/{terminal_id}/turn-state`) is authorized per
request by an opaque, daemon-generated callback token instead of the owner
session cookie. It is never reachable from a browser context, never issues or
consumes the owner session cookie, and is the one route in this surface a
spawned agent terminal's own hook process calls directly. The pairing route
remains the only unauthenticated *browser* entrypoint; this callback route is
a non-browser, token-authed exception to that browser-facing rule, not a
second hole in it.
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
- `POST .../terminals/output/batch` — batched output poll (260723 Phase 1);
  request `{"cursors": [{"terminalId": "<id>", "after": <u64>}, ...]}`,
  response `{"results": {"<terminalId>": <TerminalOutputView>, ...}}`.
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
  JSON body (create, input, resize, and the batch output route) enforce the
  same `application/json` content-type boundary and classify malformed bodies
  the same way the unscoped `Json` extractor does (`415` for a missing/non-JSON
  content type, `422` for a data error, `400` for a syntax error), staying
  byte-for-byte equivalent to the legacy route.
- **Owner-auth + bearer gating.** Terminal input, resize, and close are mutating
  host control; they preserve owner auth at the local gateway (router placement)
  and bearer auth to the linked daemon, and are never reachable without both.
- **Batch omits, never errors.** The batch output route never fails as a whole
  for a bad cursor: an unknown `terminalId` or one whose work root is currently
  offline/unavailable (same `resolve_online_available_work_root` gate as the
  single-ID route, applied per cursor) is silently omitted from `results` -
  still `200`, possibly with a partial or empty map. Callers must treat a
  missing key as "no update this tick", not as an error signal.
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
targets the workspace Git root's
`.ws-dashboard/worktrees/<branch-compatible-name>` convention. Custom path
selection may reuse the folder picker in target-path or parent-directory mode
without adding broad file-manager operations.

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
selected visible WorkRoot. That poll uses only lock-free git reads: the
`git status` summary passes `--no-optional-locks`
(`260714-bug-git-status-poll-index-lock-staleness`) and the change-line
summary uses the plumbing `git diff-index --numstat` form
(`260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge`) instead of
porcelain `git diff --numstat`, which did opportunistically rewrite the index
and take `.git/index.lock`; so the poll never takes the repository's
`.git/index.lock`. Mutating routes (switch/fetch/push/pull) are unaffected and
still take locks as needed. All
Git toolbar routes remain owner-authenticated,
address workRoots by opaque `workRootId`, keep Git work off async workers, and
avoid exposing host paths in command logs or bounded browser-visible errors.

Behind that poll, Git status/branch freshness is change-triggered with a TTL
ceiling rather than recomputed on every tick: a per-repo `notify` filesystem
watcher, armed when the repo's mount supports it, invalidates the cached
answer as soon as a relevant file changes, so the poll only re-reads when
something actually moved. The TTL is the ceiling on how stale a read can be
when no watcher event arrived: 120 s while the watcher is armed, 2 s when it
is degraded (unsupported mount, over the per-process directory-watch budget)
or unarmed. Watcher unavailability degrades freshness, never correctness — a
degraded repo still answers from the same lock-free poll path, just on the
tighter TTL. User-initiated mutations (branch switch/create, fetch, push,
pull, worktree add/remove) are never TTL-delayed: they invalidate the cached
answer directly, independent of whether a watcher event has arrived. Watcher
health, and each repo's current worktree/refs invalidation counters, are
visible at [`GET /api/dashboard/diag/git`](#260726-dashboard-git-invocation-budget-and-spawn-diagnostics).

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
The per-repo filesystem watchers described above act only as an invalidation
hint for the Git status/branch cache; they do not replace this canonical
resource refresh as the source of workRoot availability, and a watcher outage
degrades cache freshness, not availability correctness.

An *unavailable* Git workRoot (known to the registry, but its Git probe
currently fails) answers Git toolbar routes with `409 workRoot unavailable`.
An *unknown* workRoot id (not present in the registry at all — for example
after the canonical resource refresh has pruned it) answers with
`404 unknown workRoot`. The two are deliberately distinct: unavailable is a
transient, retryable read against a workRoot the caller may still recover,
while unknown means the id no longer resolves to anything and the caller must
re-open the workRoot by path.

## Git Invocation Budget And Spawn Diagnostics {#260726-dashboard-git-invocation-budget-and-spawn-diagnostics}

Every Git invocation the daemon makes for toolbar state, resource discovery, and
Activity projection runs under a wall-clock budget. On expiry the child is
terminated and the call reports failure, so a wedged or slow Git invocation
surfaces as a bounded error instead of an indefinitely pending request.
`WS_DASHBOARD_GIT_TIMEOUT_MS` sets the budget (default `10000`); `0` disables
bounding entirely and restores unbounded waiting. The sibling
`WS_DASHBOARD_GIT_PROBE_TTL_MS` (default `30000`, `0` disables) memoizes
discovery probes and is unrelated to the budget.

The bound covers waiting, not termination itself: a child wedged in
uninterruptible I/O — for example against a disconnected network mount — cannot
be terminated or reaped, so the budget bounds every case except an unkillable
child.

A Git invocation that finishes while its output remains incomplete still reports
its real exit status, because the command's success or failure is independent of
whether the daemon could read all of its output. Output can remain incomplete
when a descendant process the command started keeps the inherited output
channels open. Callers that only need the exit status — branch switch, branch
create, fetch, push, pull — succeed normally in that case. Callers that parse
output instead treat incomplete output as a failure, so a partial read is never
consumed as a complete answer.

Routinely non-zero Git exits are not reported as daemon faults: a branch with no
configured upstream, an unborn `HEAD`, a directory that is not a repository, and
a branch-existence check for a branch that does not exist are all expected
answers rather than errors. Unexpected failures, spawn failures, and budget
expiries are logged with the subcommand, exit code, bounded stderr, and elapsed
time; host paths are not exposed.

`GET /api/dashboard/diag/git` is owner-authenticated and reports cumulative
counters for the current daemon process, plus one entry per repo the
filesystem watcher currently knows about:

```json
{
  "totalSpawns": 0, "timeouts": 0, "failures": 0, "bySubcommand": {}, "uptimeMs": 0,
  "repos": [
    {
      "key": "/abs/path/to/repo",
      "health": "armed",
      "reason": null,
      "worktreeEpoch": 0,
      "refsEpoch": 0,
      "lastEventMs": null,
      "registeredWatches": 0
    }
  ]
}
```

`health` is one of `armed`, `degraded`, or `unarmed`; `reason` is set only for
`degraded` (for example an over-budget directory count or an unsupported
mount) and is otherwise `null`. `worktreeEpoch`/`refsEpoch` are the cache
invalidation counters described above — a caller comparing two reads a known
interval apart can tell whether either axis changed, and `registeredWatches`
is a rough sizing signal, not a correctness guarantee. This array reports
watcher state, not Git spawn activity, so a pane that only re-renders from
already-armed watcher events (no new poll ticks) does not grow `totalSpawns`.

`failures` already includes `timeouts`, so a consumer must not add them. The
counters cover Git invocations that go through the shared execution path — the
toolbar, discovery, and Activity projection paths above — and do not include the
worktree add and remove flows, which invoke Git directly. Two reads taken a
known interval apart yield the daemon's Git invocation rate, which is the
intended use.

## Shared Git Probe Memo And Per-WorkRoot Git Context {#260726-dashboard-shared-git-probe-memo-and-per-root-git-context}

One memoized Git probe per work root answers three questions for every consumer:
whether the directory is a Git work tree, whether it is a primary root or a
linked worktree, and which canonical worktree and common root the WorkRoot
Activity projection reads its per-project state under. Resource discovery, the
Git toolbar routes, and the Activity projection share that one answer, so
whichever of them runs first pays for it and the others read it for free until
it expires. `WS_DASHBOARD_GIT_PROBE_TTL_MS` (default `30000`) bounds how long the
answer is reused.

Two consequences follow from sharing, and both are intended:

- A directory that becomes a repository — or a repository whose topology changes
  — is reflected in the toolbar, the sidebar classification, and the Activity
  pane **together**, within the memo's lifetime rather than immediately. The
  three surfaces cannot disagree about whether a work root is a repository.
- A probe that fails to answer at all, such as one that exceeds the Git
  invocation budget, is remembered as "not a repository" for the same lifetime.
  A work root whose Git probe times out therefore reads as non-Git, and its
  Activity pane as empty, until the memo expires. This is bounded and
  self-healing, and it replaces re-running a full-budget Git invocation on every
  poll tick.

Resolving a single work root's Git context reads the registry and the
filesystem directly rather than enumerating every known work root, so the cost
of answering a Git toolbar request does not grow with the number of open work
roots. The Git toolbar routes are pure reads with respect to the registry: they
no longer register newly discovered linked worktrees as a side effect. Newly
created linked worktrees still appear through the canonical resource endpoint's
polling refresh, and immediately after a dashboard-initiated worktree add.

An online work root whose directory has become unreadable answers the Git
toolbar routes with the bounded unavailable response described under
[Git-Aware WorkRoot Toolbar](#260524-ws-dashboard-git-aware-workroot-toolbar).
That response is what the caller observes only until the next canonical resource
refresh: that refresh removes work roots it can no longer see from the registry,
after which the same id is no longer known and the routes answer not-found
instead. Both answers are bounded and path-free; a caller must not treat either
one as a durable classification of the same work root.

> [!note] Implementation Gap · 2026-07-26
> Missing behavior: the registry removal above discards a work root the user
> explicitly opened, on the strength of one failed availability read, with no
> affordance to recover it. Recovering the work root currently requires opening
> it again by path.

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

## Which-Key Leader Hint Overlay {#260722-ws-dashboard-which-key-hint-overlay}

While a `Ctrl+Space` leader sequence is pending, the dashboard shows a
transient which-key/lazyvim-style hint popup anchored to the bottom-right of
the viewport, listing the currently-reachable next keys from the hotkey
binding registry's leader tree. The popup does not appear immediately: it
appears only after the sequence has been pending for 250ms, so a fast,
already-memorized leader chord never flashes the popup. That 250ms delay is
anchored to the initial idle-to-pending transition (the moment `Ctrl+Space`
is first pressed) — narrowing the sequence with a further key press does not
restart the delay or re-hide an already-visible popup.

Each reachable next key renders as one row: `key → +group` when that key
leads to further sub-keys (a branch), or `key → <label>` when that key
resolves directly to a bound command (a leaf). As the user types further
keys in the sequence, the popup's row list updates to the new current node's
children, narrowing along with the sequence. The popup dismisses on every
path that returns leader mode to idle: successful resolution of a binding,
an unmatched-key cancel, `Escape`, or a second `Ctrl+Space` leader press.

A leaf row's label is sourced from the resolved `HotkeyBinding`'s
`description` field, not from a label derived from the bound command. This
is a fixed contract, not an incidental shortcut: resolving a command-derived
label requires invoking the binding's payload builder against the live
dispatch context, and every context-dependent default binding's payload
builder returns no result when no work root is selected — a common,
unremarkable state the popup must still render correctly in. Every default
leaf binding's `description` is authored as the same human-readable action
label a command-derived label would otherwise produce, so reading
`description` directly satisfies the popup's labeling intent without that
gap.

The popup is a read-only presentation layer over the live hotkey binding
registry: it defines no bindings and no dispatch path of its own, and it
reflects the registry's current contents — including a user's own
rebindings — automatically. It does not capture or consume keyboard input;
the existing leader-press capture path is unchanged and remains the sole
input-handling surface. That path's terminal-focus/IME guard applies to
leader-*continuation* keys and standalone bindings, but the leader-*entry*
trigger (`Ctrl+Space` from idle) is checked before that guard and is never
blocked by terminal or editable-target focus, so the popup can always be
opened regardless of where focus currently sits.

> [!note] Implementation Gap · 2026-07-22
> Browser-level (Playwright) verification of the popup's appear/narrow/
> dismiss lifecycle across all four dismissal paths has not been run yet.
> The behavior above is verified by unit tests over the pure row-derivation
> logic only.

## Dashboard Settings Panel {#260722-ws-dashboard-settings-panel}

The dashboard exposes a single global Settings modal for dashboard-wide
preferences, distinct from any per-workRoot or per-worktree UI. It is
opened by a `settings.open` command (mirrored by `settings.close` on
dismissal), issued from a keyboard-reachable topbar button; it follows the
same react-aria `ModalOverlay`/`Modal`/`Dialog` composition as other
dashboard confirmation modals (`260722-ws-dashboard-git-worktree-removal`),
and is keyboard-only operable: open, section navigation, and dismissal
(`Escape` or backdrop click) all work without a mouse. There is
deliberately no default leader-hotkey binding for `settings.open` — the
shipped keymap table is exhaustiveness-tested (`hotkeys.test.ts`) against
every command, and no settings leaf was added to it. A hotkey binding can
be introduced once the planned hotkey-rebind section below lands.

The panel's body is not a hard-coded screen; it renders an ordered list of
independently registered setting sections, backed by a module-scope
`SETTINGS_SECTIONS` registry of `{ id, title, Component }` descriptors with
stable `Component` identity (never an inline function rebuilt on
re-render, which would otherwise remount — and drop focus out of — a
section's inputs on every keystroke). The `SettingsModal` shell receives
this descriptor list as an injected `sections` prop and iterates it
generically: it consumes only `id`/`title`/`Component` and threads no
section-specific typed props. A section owns reading and writing its own
preferences through its own context/state; the shell has no knowledge of a
section's internal fields or storage shape. This registry contract exists
so that later sections (starting with a planned hotkey-rebind editor) can
be added by appending a descriptor to `SETTINGS_SECTIONS`, without
modifying the shell or any other section's code.

Section preferences persist through a shared, namespaced preferences-store
helper built over the dashboard's existing low-level `browserStorage()`
accessor, generalizing the versioned `"ws-dashboard.<feature>.v<N>"`
JSON-blob convention already used ad hoc per feature (e.g. hotkey
bindings). The helper is the single read/write seam for section prefs:
each section loads and saves through it under its own namespaced key, with
defensive parsing that falls back to that section's defaults on missing or
malformed stored data, rather than each section reimplementing its own
storage logic.

The first registered section is Terminal style: font family, font size, and
background theme color, replacing the values previously hardcoded at the
xterm construction site (`260516-ws-web-dashboard-browser-terminal-emulator-behavior`
documents the resulting rendering behavior). When no preference is stored,
the section's defaults reproduce the prior hardcoded look exactly — an
empty font-family override leaves the existing Nerd Font fallback stack
unchanged (a non-empty override is prepended onto that stack rather than
replacing it), font size defaults to `12`, and background defaults to
`#0b0d10` — so an unconfigured dashboard is visually unchanged.

Live values reach open terminal panes through a dedicated
`SettingsTerminalContext` (prefs plus the section's single write path),
separate from the read-only `TerminalPrefsContext` that every open
terminal pane consumes to live-restyle its emulator. A change applies to
already-open panes immediately, without remounting or interrupting the
underlying session; new panes read the persisted preference at
construction time.

> [!note] Protected zone
> The terminal mount effect that constructs the xterm `Terminal` instance
> and performs session restore/reattach is a protected zone
> (`260516-ws-web-dashboard-protected-frontend-shell`). Its only
> settings-related change is substituting the 3 constructor values
> (`fontFamily`/`fontSize`/`theme.background`) for prefs-derived reads at
> construction time; it does not otherwise change. Live restyling of an
> already-open pane must be implemented as a separate effect, declared
> outside the mount effect, that subscribes to prefs changes post-mount and
> mutates `terminal.options.*` directly, then re-triggers the existing
> FitAddon refit and size-forward so a font-metric change reflows the
> emulator's cell geometry (guarded to no-op against an already-disposed
> terminal). The mount effect itself must never be made to depend on live
> prefs, since that would remount the emulator and interrupt the session.

> [!note] Implementation Gap · 2026-07-22
> Browser-level (Playwright) verification that a live Terminal-style change
> fans out to an open pane, and that the protected-zone mount effect is
> otherwise unaffected, has not been run yet. The behavior above is
> verified by unit tests over the pure prefs helpers
> (`terminalPrefs.test.ts`) and the section-registry contract
> (`settingsSections.test.ts`, `settingsStore.test.ts`) only.

The second registered section, Notifications
(`260725-feat-dashboard-pty-agent-attention-notification` Phase 8), persists a
single opt-in boolean (`ws-dashboard.settings.notifications.v1`, `{ enabled:
boolean }`) through the same namespaced preferences-store helper — no other
notification state is cached client-side. Enabling the toggle also requests
the browser's `Notification` permission, but ONLY from the checkbox's own
`onChange` handler — a real user gesture — never from a mount-time effect,
mirroring the pattern the codebase already avoids for `sw.js` registration
(`main.tsx`). The section's copy states plainly that OS-level notification
requires a secure context (`localhost` or a TLS origin): a plain-http LAN
page's `Notification` API is un-permissioned and ungrantable there, not
absent — the global itself may still be defined — so the section reads
`window.isSecureContext` before `typeof Notification`, and (both readable with
no permission prompt of their own) shows the current state, including an
explicit "unavailable - this page is not a secure context" message, rather
than only surprising the user after an unresponsive click. On an insecure
context the checkbox itself is also disabled, since no click there can ever
change the permission.
If the owner grants the permission, the section reflects that as soon as the
prompt settles rather than at the next unrelated repaint. If the owner denies
it, the toggle does not stay on: an enabled preference guarding a tier the
browser will never allow is a control that lies about its own effect, so the
denial turns the preference back off and the section shows the denied state.

This preference is the sole gate the
[Browser-Level Attention Cue](#260726-dashboard-browser-level-attention-cue)
checks before raising an OS notification; it is not itself responsible for
deciding *when* to notify.

> [!note] Planned 🚧
> A future hotkey-rebind editor section will register into this panel to
> edit bindings from the hotkey binding registry directly, and is expected
> to introduce the first default leader-hotkey binding for
> `settings.open`. It is a planned extension of the section registry, not
> specified further here.

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

Work-root left-nav rows carry a second, smaller information line beneath the
row title reporting how many surfaces that root currently has open, counted
separately for terminals and documents and phrased in place of a count when
nothing is open. The counts describe live workbench state — surfaces mounted
for that root right now — so they change as terminals and documents are opened
and closed, and a root with no open surfaces says so rather than showing
nothing. The counting scope is per root: a row never reports another root's
surfaces. The same line also reports the root's agent terminals: how many the
root has, and how many of those are signalling working or waiting states the
user has not yet acknowledged — so a state the user has already looked at
stops being reported even while that agent is still mid-turn. The agent report
appears whenever the root has any agent terminal at all, including one just
spawned that has not yet reported a turn, and it leads the line ahead of the
terminal and document counts so it stays readable when the line is too long
for the sidebar. A root whose only open surfaces are agent terminals reports
those agents rather than describing itself as having nothing open. An agent
terminal is reported by the agent counts only and is never also included in
the terminal count, so no open surface is counted twice. Every work-root row
reserves the vertical space for the second line whether or not it currently
has counts to show, so opening or closing a root changes what a row says
without changing how tall it is and without reflowing the rows around it.

A work-root row also encodes open-versus-closed state visually, not only
through the presence of its close affordance: a root that is not currently
open is drawn with reduced emphasis while staying listed, selectable, and
hoverable with the same hover feedback as any other row. A row that is
reporting an error keeps its error appearance regardless of open state. Both
the second line and the open-state emphasis apply to work-root rows —
including the compact single-work-root form — and not to workspace rows, which
carry neither.

A row whose agents are working or waiting also carries an attention level,
with waiting outranking working. Every open work root the nav shows is
reported by exactly one row for this purpose: its own row where it has one,
and otherwise the row that stands for it — a workspace's base root has no row
of its own, so its agents raise the level on the workspace row, which takes
the level without taking the second line or any counts. A hidden worktree is
the deliberate exception: it is reported by no row at all, neither its own nor
any row standing above it, because the user asked not to see that root and
silence about its agents is part of what was asked for. A server row carries
the highest such level among the roots reported beneath it, while carrying no
counts of its own, so no shown root's agents go unreported and no level
appears that no row below would account for. That level is presented as an animated overlay layered over the row rather than as
a change to the row's own background, so it never competes with the row's
open-state, hover, selection, or error appearance, and it is suppressed to a
static tint for viewers who ask for reduced motion. The level is derived, not
separately dismissed: it is raised and cleared entirely by the acknowledgement
state of the root's own agent terminals, so acknowledging the last still-
pending terminal clears the row and its server row with no separate row-level
action, and a row keeps reporting its agents while another root is selected.
{#260725-nav-row-open-surface-counts-and-open-state}

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

Tab lifecycle affordances stay live independently of Dockview parameter
refresh timing and independently of whether the tab has ever been the active
pane. A tab restored by a page refresh and never activated must respond to its
close and acknowledge affordances on the first interaction, with no preceding
tab-body click required. A tab's attention indicator must not change the tab's
close-affordance geometry when it appears or clears, because the indicator can
clear during the same pointer gesture that presses the close affordance.

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

The badge's activity fetch is bounded by a client-side timeout: a daemon
response that stalls without ever returning transitions the badge to its
existing error state and lets the next poll retry, rather than leaving it stuck
in its loading state indefinitely
(`260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge`).

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

The frontend package exposes this gate through `npm run test:browser`. Building
the production frontend is a property of the gate run itself, not of that one
script: every Playwright invocation path builds the frontend before the daemon
is started - except where the gate does not construct the served directory
itself, described below - so a bare `npx playwright test`, a single-spec run, or
an IDE runner cannot serve a bundle older than the current frontend source when
the harness builds that directory. The gate serves
that build through the dashboard daemon, pairs as owner through the startup
pairing URL, and records textual evidence plus regenerable screenshot artifacts
outside tracked source. A build failure ends the run instead of falling back to
the previous bundle.

The gate skips that build, announcing which condition fired on the run's own
output, only where it does not construct the served static directory itself: a
caller-supplied `WS_DASHBOARD_STATIC_DIR`, or external daemon mode
(`WS_DASHBOARD_DAEMON_MODE=external`, `WS_DASHBOARD_DAEMON_BASE_URL`, or
`WS_DASHBOARD_DAEMON_PAIRING_URL`). Neither condition proves the built output is
unused, so on those two paths keeping the served bundle current stays the
caller's responsibility.

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
file behavior, and default spawned-daemon agent close coverage. The covered
close paths include a refresh-restored tab that has never been activated: the
gate closes such a tab on its first close-affordance click, both while it
carries a pending attention indicator and while it carries none, and asserts
that the close affordance does not move between the press and the release of
that click. Evidence that clicks the tab body before the close affordance does
not satisfy this clause. The
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

Native macOS evidence to date (260725 Phase 1) covers a native
`cargo build -p ws-dashboard-daemon --all-targets` pass on aarch64-apple-darwin
plus per-target `cargo test -p ws-dashboard-daemon` results, run and read
individually rather than trusted from a single fail-fast invocation:

- `--lib`: 124 passed, 0 failed, 2 ignored (includes the four
  `terminal_platform::platform_identity_tests`, covering `process_start_time`,
  `verify_process_identity`, and both `kill_verified` outcomes on the macOS
  leg).
- `--test server`: 15 passed, 0 failed.
- `--test terminal_lifetime`: 3 passed, 0 failed (real two-daemon-process
  restart/reattach/dead-shell-detection lifecycle, run against real OS
  processes and a real Unix-domain-socket IPC channel).
- `--test terminal_windows_reaper_acceptance`: 0 tests collected — this
  target's content is entirely `#[cfg(windows)]`-gated, so it compiles clean
  on macOS but contributes no macOS coverage.
- `--test routes`: 164 passed, 2 failed. Both failures
  (`dashboard_resources_refresh_prunes_workspace_without_available_work_roots`,
  `online_missing_work_root_returns_bounded_unavailable_without_path_leak`)
  are attributed to the pre-existing, diff-untouched
  `discovery.rs::canonical_or_normalized` work-root-id instability captured in
  ticket `260725-bug-dashboard-workroot-id-unstable-when-path-canonicalize-fails`,
  not to this port; `discovery.rs` and `work_root_files.rs` are unmodified by
  this phase.

Linux non-regression evidence: native `cargo check --target
x86_64-unknown-linux-gnu -p ws-dashboard-daemon` cannot run in this
environment — `ring` and `libsqlite3-sys` both invoke `cc-rs` at build-script
time even for `cargo check`, and the environment has no
`x86_64-linux-gnu-gcc` cross C toolchain. Verified instead with a native
x86_64 Linux container:

```
docker run --rm --platform linux/amd64 \
  -v <repo>/ws-dashboard:/workspace:ro \
  -e CARGO_TARGET_DIR=/tmp/target -w /workspace rust:latest \
  cargo check --locked -p ws-dashboard-daemon --all-targets
...
Checking ws-dashboard-daemon v0.1.0 (/workspace/crates/daemon)
Finished `dev` profile [unoptimized + debuginfo] target(s) in 44.07s
(exit code 0)
```

Run against a clean working tree at each Phase 1 implementation tip in turn —
including the final one, `1aca7993` — so it type-checked the reviewed source
with test targets included. Documentation-only commits after that point do not
affect the result, because the Linux leg compiles no macOS-gated code. This is
produced Linux evidence, not a deferral.

Native macOS evidence (260725 Phase 2) closes the process/socket-level half of
the live-lifecycle gap: all four lifecycle legs verified on native
aarch64-apple-darwin via `cargo test -p ws-dashboard-daemon --test
terminal_lifetime` (4 passed, 0 failed), each with a non-vacuity mutation
proving its assertion can actually fail — mutated, run to a confirmed FAIL,
reverted via `git checkout --`, re-run to a confirmed PASS, with `git status`/
`git diff` on `crates/daemon/src/` clean after every revert:

- Spawn (`unix::spawn_detached`, `terminal_platform.rs`): forcing the
  `pre_exec` closure to `return Err(...)` immediately failed
  `create_terminal`'s `assert_eq!(response.status(), OK, "create terminal")`
  with `left: 400, right: 200`.
- Daemon-restart re-adopt (`reconcile_entry`'s adopt arm, `terminal.rs`):
  commenting out `self.insert_unchecked(session);` failed both restart-adopt
  tests at their "adopted terminal missing from list: []" panics.
- Dead-shell detection (PTY-EOF reader path, `terminal_helper_process.rs`):
  commenting out `shared.transition(TerminalHelperStatus::Exited);` in
  `spawn_reader_thread`'s EOF branch failed
  `terminal_live_pty_eof_exit_flips_status_to_exited`'s `saw_exited` assertion
  after its 5s drain deadline. This is a **confirmed observation**, not an
  assumption carried over from prior phases' prose: macOS PTYs deliver EOF on
  shell exit the same way Linux does, so the reader-thread path (not a
  Windows-style process-handle reaper, which stays `#[cfg(windows)]`-gated and
  contributes no macOS coverage) is what flips status to `exited` on this
  platform too.
- Identity-verified close (`TerminalSession::terminate`'s fallback
  `kill_verified` call, `terminal.rs`): the new
  `terminal_close_kills_verified_process_via_fallback_kill` test `SIGSTOP`s
  the helper before issuing `DELETE` so it cannot service the graceful
  `GracefulShutdown` IPC path within `terminate()`'s 200ms window (`SIGKILL`
  is not maskable by `SIGSTOP`), forcing the fallback `kill_verified` SIGKILL
  to be the mechanism that actually terminates it. Inserting an early
  `return Ok(false);` in `macos::kill_verified` right after its `pid == 0`
  guard reproduced the leg's non-vacuity failure: the test's process-death
  poll timed out with the `SIGSTOP`'d helper still alive. This mutation
  leaves a frozen process if the test fails between the `SIGSTOP` and the
  daemon's kill; the test's own `HelperReaper` drop guard (identity-verified
  `kill -KILL`, independent of `kill_verified`) was confirmed to reap it
  automatically during the panic unwind, since `SIGKILL` still terminates a
  stopped (`T`-state) process.

Pid-mismatch/pid-reuse negative coverage is deliberately **not** added at the
integration level — reproducing genuine OS pid reuse deterministically would
require racing OS pid allocation and would be flaky by construction. This case
stays covered only at the unit level:
`terminal_platform::platform_identity_tests::kill_verified_refuses_to_kill_on_start_time_mismatch`,
part of the `--lib` 124/0/2 result re-confirmed in this phase.

`--lib` (124 passed, 0 failed, 2 ignored) and `--test server` (15 passed, 0
failed) were re-run alongside `--test terminal_lifetime` to confirm no
regression; both match the Phase 1 baseline exactly. This phase's own tests
were confirmed to leak nothing at the process level: live `terminal-helper`
process counts (`pgrep -f terminal-helper`) were compared before and after
every run, including each mutation round-trip, and returned to the same
baseline every time, and no `T`-state strays remained, independent of the
already-tracked `tests/routes.rs` detached-helper leak
(`260725-bug-dashboard-routes-test-terminal-helper-leak-no-reaper`).

The browser-facing UI gate remains an explicit gap on macOS, deferred to a
later phase, and must not be read as covered by this process/socket-level
result — `terminal_lifetime` exercises the real lifecycle through real OS
processes and a real Unix-domain-socket IPC channel, including a real
`tokio_tungstenite` client attached to the daemon's terminal WebSocket route
in three of its four lifecycle legs (restart reattach, boot-reconcile
grace-row adoption, and dead-shell detection — only the close-kill leg has
no socket), with bidirectional traffic (terminal input written back over the
socket, not merely output read), asserted `101` upgrade responses on every
attach, and the restart leg's client attaching with a real `?after=` resume
cursor, so the WS protocol surface itself is exercised thoroughly — but not
through the browser-facing UI.

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

The dashboard daemon manages shell terminal sessions scoped to opened
workRoots through an authenticated registry, but the daemon itself does not
own the PTY. Creating a terminal spawns a detached per-terminal helper
process that owns the underlying PTY for the shell's lifetime; the daemon
holds a proxy session that talks to the helper over a native IPC channel (a
Unix domain socket on Unix, a named pipe on Windows) and mirrors the
helper's output into a local ring cache so ordinary reads stay synchronous
calls instead of a per-request IPC round trip. Authenticated owners can
create and list live terminal sessions by opaque terminal ids. Spawns run in
the selected workRoot directory and terminal ids are not process ids or host
paths.

Terminal creation optionally names a vendor profile id, resolved against a
small daemon-side profile registry (spawn argv, env scrub list, hook config
shape). An absent profile id keeps the default interactive-shell spawn
unchanged, byte for byte, from a request that names no profile at all. The
resolved profile — if any — is recorded read-only on the session for
provenance, and that provenance survives a daemon restart: the daemon records
the resolved profile id in daemon-owned per-terminal state at spawn time and
restores it when boot reconciliation reattaches the session, so a reattached
agent terminal keeps reporting the profile it was spawned with. The on-disk
terminal registry still never carries a profile id — it is written by the
helper process, not the daemon. A session reattaches without provenance
whenever no readable per-terminal record survives to reattach time — cases
include a terminal spawned before this behavior existed (there is no backfill),
a daemon with no resolvable state directory, which records nothing at spawn, a
record whose write failed at spawn, and a record missing or unreadable at
restore time. Every such case reports no profile, which is the behavior before
this provenance existed; none of them reports a different profile.
{#260725-ws-web-dashboard-terminal-spawn-profile}

Each helper records its identity — process id and process start-time — in a
per-terminal registry file, so the daemon can distinguish a still-live helper
from a stale entry whose pid has since been reused by an unrelated process
rather than trusting a bare pid. The start-time value's *source* is
platform-specific (`/proc/<pid>/stat` on Linux, `GetProcessTimes` on Windows,
`proc_pidinfo`/`PROC_PIDTBSDINFO` on macOS); the recorded registry value
itself stays an opaque number end-to-end and is never interpreted outside the
platform module that produced it.

Live terminal sessions persist across browser refresh, and across a daemon
restart, because the PTY's lifetime belongs to the detached helper process,
not the daemon. Browser arrangement state controls where sessions are shown,
not whether the underlying live session exists. See
[WorkRoot IO Restore Model](#260523-ws-dashboard-terminal-tab-restore) for
daemon-restart reattachment and helper-termination behavior.

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

A terminal whose shell has just exited remains listed and WebSocket-
attachable for a bounded grace window after the exit before it is dropped, so
a browser that reconnects slightly late — including immediately after a
daemon restart — still receives the final output and exit status instead of
finding the terminal already gone. Once a terminal id is unknown or has
passed out of that grace window with no live process behind it, the WebSocket
route rejects the upgrade with a bounded not-found response before accepting
the socket, the same as any other unknown terminal id.
{#260723-terminal-attach-grace-window}

## Terminal Pane {#260516-ws-web-dashboard-terminal-pane}

The dashboard workbench renders daemon-owned terminal sessions in terminal panes
for the selected workRoot. Creating a terminal opens or focuses a terminal pane,
and refresh can reconstruct visible terminal panes from daemon live session
state plus browser arrangement where available.

A terminal session that has exited, terminated, or errored is visually retired
in place — the pane grays out and its control relabels to an explicit clear
affordance — rather than being auto-removed, so the final scrollback stays
readable until the owner clears it. Retirement follows the session's reported
status even while the browser is on the HTTP-polling fallback path, and a
coarse reconciliation poll additionally retires panes the daemon no longer
lists. Clearing or closing an already-retired or already-gone pane is
idempotent: it resolves as success without surfacing a terminal-close error.
{#260724-terminal-pane-dead-session-retire}

The terminal pane substrate is shell-neutral: it does not hardcode a
specific agent preset, but MAY be spawned with a resolved vendor profile
(command argv, env scrub, provenance) over the same single-sourced PTY
plumbing — no second helper kind, no parallel PTY implementation. See
[Terminal Registry And PTY Spawn](#260516-ws-web-dashboard-terminal-registry-pty-spawn)
for the profile registry.

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

## Terminal Attention Event Stream {#260726-dashboard-terminal-attention-event-stream}

The daemon exposes a server-wide, work-root-independent SSE stream of
per-terminal turn-state ("attention") transitions, so the browser can learn
that a terminal has gone from `working` to `ready` (or `idle`) even when no
Activity Console pane for that terminal's workRoot is open. The local route is
`GET /api/dashboard/terminals/attention/events`; the server-scoped sibling is
`GET /api/dashboard/servers/{serverRoute}/terminals/attention/events`, forwarded
transparently to the linked daemon the same way the Activity Console watch
stream and document-content-changed stream are. Unlike those two streams, this
one is not scoped to a `{workRootId}` path segment: attention state is keyed by
terminal id across the whole daemon, independent of which workRoot (or
Activity Console pane) is currently selected in the browser.

On connect, the stream first emits one `event: attentionSnapshot` frame
carrying every currently-pending attention state (`{ items: [...] }`), so a
browser refresh or a fresh reconnect after a network interruption does not
lose a state transition that happened while no stream was open. Subsequent
transitions arrive as `event: attention` frames, one per terminal-id/state
change, wire-shaped as
`{ type: "terminal.attentionChanged", terminalId, workRootId, state, updatedAtMs }`
where `state` is one of `working` / `ready` / `idle` — the same three-value
vocabulary the daemon's per-terminal turn-state callback route accepts, not a
parallel enum. A terminal's attention entry is removed from the snapshot the
moment its underlying terminal session closes (explicit close or owning
workRoot/workspace removal), so a reconnect never reports state for a
terminal that no longer exists.

If the stream falls behind its buffered event backlog, the daemon ends the
SSE response rather than silently skipping forward: attention state is not
safely re-derivable from a later event the way a document's content is from a
re-read, so the browser's native reconnect (which re-enters this route and
receives a fresh, complete snapshot) is the resync path, not an in-place skip.

This stream is independent of the Activity Console read model and watch
stream ([Activity Console Read Model](#260521-ws-dashboard-activity-console-read-model),
[Activity Console Watch Stream](#260521-ws-dashboard-activity-console-watch-stream)):
it carries no Activity Console item data and does not affect that projection.

## Turn-State Hook Delivery Failure Visibility {#260727-dashboard-terminal-notify-failure-visibility}

The turn-state transitions the
[Terminal Attention Event Stream](#260726-dashboard-terminal-attention-event-stream)
carries originate outside the daemon: the agent CLI's own hook runner fires the
`ws-dashboard terminal-notify` command at every turn boundary, and that command
POSTs the new state back to the daemon. That hook process is **permanently
silent**: whatever happens — the callback file is missing, unreadable, or
carries no credential; the daemon's port has moved; the token is stale; the
POST is refused or rejected — it prints nothing to stdout or stderr and always
exits `0`. The silence is deliberate and load-bearing, because a non-zero exit
or any stderr output makes the agent CLI surface a hook-error line and a
persistent error indicator inside the owner's live session on *every* turn
boundary, for as long as the breakage lasts. This spec entry describes what an
operator can observe instead.

Two artifacts carry that observability, both written by the hook process itself:

- **A rotated failure log.** Every failed delivery appends one line naming the
  turn state, the callback path, and the error text to
  `logs/terminal-notify.log.<date>` under the daemon's state directory, subject
  to the same daily rotation and retention policy as the daemon's own log.
- **A per-terminal failure record.** Beside the terminal's `callback.json`, the
  hook process maintains `notify-failures.json` carrying the consecutive
  failure count, the timestamp of the most recent failure, and that failure's
  error text (the same text the log line carries, capped in length). A
  successful delivery deletes the record, so its presence always means "the
  most recent delivery attempt failed". The record is keyed by the callback
  file's own location rather than by a terminal id parsed out of that file,
  because an unparseable callback file is itself one of the failure modes it
  must report. The hook process never creates the profile directory to write
  it: once a terminal's directory has been reclaimed, a late hook fire records
  nothing rather than resurrecting it.

Failing to write either artifact is itself swallowed silently. Observability
never comes at the cost of the stdio silence above.

The daemon reads the record for every live terminal on the same periodic sweep
that reclaims orphaned agent profile directories, and emits **one** warning to
its own log per terminal when a failure looks genuinely stuck rather than
transient — that is, when a record exists, its most recent failure is at least
one full sweep period old, and the terminal's `callback.json` has not been
rewritten since that failure (a rewrite means the target may have just been
re-pointed, so the next hook fire settles it; an unreadable or absent
`callback.json` is *not* treated as a repair). The warning names the terminal,
the failure count, and the last error. It does not repeat on later sweeps while
the same failure persists; the terminal becomes eligible to warn again once its
record is observed cleared (a delivery succeeded) or once its id leaves and
re-enters the live terminal set. Reading these records never influences which
directories that sweep reclaims.

Two non-goals are explicit. Attention state has no wall-clock expiry — a
`ready` badge is not aged out because deliveries stopped arriving, since
"stopped arriving" is indistinguishable from "the agent is idle". And there is
no user-facing "hook delivery is broken" affordance in the browser; the
audience for this signal is the operator reading `daemon.log`, not the owner
watching a terminal tab.

## Terminal Tab Attention Indicator {#260726-dashboard-terminal-tab-attention-indicator}

A workbench terminal tab label carries a state affordance driven by the
[Terminal Attention Event Stream](#260726-dashboard-terminal-attention-event-stream),
so an agent that has finished its turn is visible on the tab itself without
opening the pane. The affordance reuses the stream's three-value vocabulary
rather than introducing a parallel one: `working` and `ready` each render a
distinct badge, and `idle` — like the absence of an entry — renders nothing.

The indicator is suppressed unless the terminal session's daemon-reported
status is live. The daemon reclaims a dead terminal's attention entry lazily,
so an entry can outlive the session it describes; gating the render on live
status is what keeps a retired or exited pane from showing a badge for a turn
that ended with the process. This is a presentation gate, not a daemon
guarantee, and it remains the only defense the badge itself has: nothing else
ever clears a stale badge, and this gate only applies once the session is dead,
so a badge stranded on a **live** session by a failed turn-state delivery stays
stranded indefinitely. What
[Turn-State Hook Delivery Failure Visibility](#260727-dashboard-terminal-notify-failure-visibility)
adds is a signal for the *operator* in the daemon's log, not a defense for the
badge — see [Terminal Pane](#260516-ws-web-dashboard-terminal-pane) for
retirement.

Acknowledgement clears the badge and is **revision-keyed, not sticky**:
acknowledging records the acknowledged transition's own revision against the
same server-scoped terminal identity the stream is keyed by, so a *later* turn
boundary on the same terminal raises the badge again. Acknowledgement state is
browser-local presentation state — it is not persisted and stays outside the
[WorkRoot IO Restore Model](#260516-ws-web-dashboard-workroot-io-restore-model),
the same way Activity acknowledgement state does.

Acknowledgement has two triggers, and the second is load-bearing rather than
redundant: activating the terminal's pane, **and** clicking the tab that is
already active. The feature's primary flow is an agent left running in the
active tab while the owner is away from the browser, so the badge routinely
appears on a tab that never changes activation; a trigger that fires only on
activation *change* can never clear it, and `ready` is terminal. Both triggers
are idempotent.

For the same reason, a state transition repaints a terminal tab that is
already mounted, active, and connected. Terminal tabs otherwise suppress
presentation refreshes while their session is connected, to keep the emulator
from being disturbed mid-keystroke; the attention state is an explicit
exception to that suppression, and one that only shows up on the *second*
transition of a session.

## Browser-Level Attention Cue {#260726-dashboard-browser-level-attention-cue}

The [Terminal Tab Attention Indicator](#260726-dashboard-terminal-tab-attention-indicator)
is only visible to an owner already looking at the dashboard. A second cue
carries the same state out to the browser chrome, for the case the whole
feature exists for: the owner has switched to another tab or another window
while an agent works.

The cue has two tiers, and the zero-permission one is the default rather than
a fallback. The dashboard is routinely reached over plain http on a LAN, where
the page is not a secure context. The browser's `Notification` API is
un-permissioned and ungrantable there, not absent entirely: on Chromium the
global itself is still defined, and `window.isSecureContext` is the property
that distinguishes this case from a granted or denied secure context, and a
browser that genuinely omits the global on that same insecure origin (Safari
and Firefox may) reaches the identical ungrantable outcome for the identical
reason: the origin, not the missing global. Tier 2 cannot work on this class
of origin either way. Tier 1 therefore uses only what any page may do unasked:
it alternates `document.title` between the page's own
title and an attention-labelled variant, and swaps the favicon for a badged
one. Tier 2 is a real OS notification, and it is opt-in through the
[Dashboard Settings Panel](#260722-ws-dashboard-settings-panel).

Both tiers read one document-level attention level, aggregated over every work
root the navigation tree currently shows, across every connected server, using
the same `ready` outranks `working` outranks none priority a server row uses.
Two consequences follow from *shows*, and both are deliberate. A hidden
worktree contributes nothing here, exactly as it contributes nothing to any
navigation row — a root the owner asked not to see stays silent in the browser
chrome too. And the cue cannot disagree with the tab or navigation badges,
because it is derived from the same per-terminal pending state rather than
tracking its own.

That derivation is also why the cue needs no acknowledgement of its own.
Acknowledging the last pending terminal — by the ordinary tab triggers — drops
the aggregate level to none, and the title and favicon return to the values
the page loaded with. There is no separate dismiss action, and no second
acknowledgement watermark that could disagree with the first.

Tier 1 is level-driven: it is present while the level is non-none and absent
otherwise. Tier 2 is edge-driven instead, and fires only on entry into
`ready` — `working` is ordinary background progress, not something worth
interrupting an owner for. Two observable consequences of an aggregate edge:
reloading the page while an agent is already waiting notifies again, since the
level rises from none on load; and a second agent reaching `ready` while
another already is fires nothing, since the aggregate never left `ready`. The
per-tab badges still distinguish both cases.

A browser that exposes `Notification` may still refuse to construct one — some
mobile browsers require notifications to go through a service worker, which
this dashboard does not use — so a failure to raise the OS notification is
contained to that tier and never disturbs the page.

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

A live terminal survives a daemon restart: because the PTY lives in the
detached per-terminal helper process rather than the daemon, daemon exit does
not run any terminal teardown, and daemon exit alone never kills a terminal.
On daemon (re)start, the daemon runs `boot_reconcile`: an identity-gated
reconcile pass over the on-disk per-terminal registry that classifies each
entry against a 6-row decision table, collapsing to four caller-observable
outcomes — adopt a still-live helper as an attached live session, adopt a
helper whose shell already exited but remains within a bounded post-exit
grace window (so an exit that happened just before or during the restart is
still visible and attachable rather than silently dropped, per
[Terminal Attach Grace Window](#260723-terminal-attach-grace-window)), kill a
helper whose identity cannot be verified against the recorded pid and
process-start-time (for example after pid reuse), or drop a stale registry
entry that has no live process behind it, without touching any unrelated
process. Because reconciliation re-adopts the still-running helper under the
same terminal id, the frontend's existing resume-by-id path reattaches to the
same live shell — not a newly spawned one — with cursor continuity, the same
as any other stale-cursor reconnect surfacing a truncation gap marker if the
daemon's retained output no longer covers the gap. The restore-descriptor
fallback (a newly created daemon terminal session attached to the remembered
workRoot, carrying title plus a workRoot-relative cwd hint) still applies
only when boot reconcile could not adopt any live or in-grace helper for that
terminal id — it no longer describes the ordinary daemon-restart case.

Only two events terminate a helper process: an explicit terminal-close
request, or removal of the owning workRoot/workspace root. Termination is
graceful-then-verified: the daemon first sends the helper a graceful-shutdown
request over the IPC channel, then falls back to an identity-verified kill of
the helper's recorded pid. On Linux and Windows this guarantee is
structurally closed and never a bare-pid re-resolve: the fallback kill
captures a stable OS handle (Linux pidfd / Windows process handle) at
verification time and signals through that handle, so a pid reused by an
unrelated process after the helper already exited is never mistakenly
killed. On macOS the fallback kill instead verifies identity immediately
before signalling and then signals through the bare recorded pid (`kill(2)`;
macOS has no pidfd-equivalent stable handle to signal through instead),
followed by a best-effort, non-guaranteed post-kill re-check — this narrows,
but does not close, the same verify-to-kill race; it must not be read as a
reliable mis-kill detector. On Windows, the helper additionally places its
spawned shell into a kill-on-close job object so the fallback kill tears down
the whole shell subtree; on Unix, the helper detaches from the daemon at
spawn time so it keeps running independent of the daemon process.

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
