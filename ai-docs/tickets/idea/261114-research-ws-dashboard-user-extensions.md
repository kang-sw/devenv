---
title: "Dashboard user extensions: local sandboxed drawer panels with a permissioned data bridge, and an optional daemon-side backend"
related:
  261113-feat-ws-dashboard-iframe-shell: "owns the drawer and the shell-side postMessage bridge an extension frame talks to"
  261112-research-ws-daemon-extension-protocol-chat-adapter: "the other daemon extension surface; candidate for a shared message envelope"
  261110-feat-ws-dashboard-file-preview: "origin of the capability-URL + sandboxed-frame pattern an extension reuses"
---

# Dashboard user extensions

## Background

Owner, 2026-11-13 (whiteboard topic `user-extensions`): let the machine's
user install local extensions to the dashboard. An extension shows UI in a
drawer at the bottom right of the dashboard and can read dashboard data.
The design panels live in that board topic.

This ticket is research only ((564)): it records the agreed shape and the
open backend question; it carries no implementation plan. An actionable
ticket derives from the Confirmed Decisions below once (569) is answered.

## Frontend extension shape

An extension is a folder holding a manifest plus HTML/JS. The daemon serves
the folder on a capability URL whose responses carry the CSP
`sandbox allow-scripts`, so the document runs in an opaque origin, never in
the daemon's. The shell (iframe-shell ticket) owns a bottom-right drawer
that hosts the extension in a sandboxed iframe. The extension reaches data
only through a shell-side `postMessage` bridge: messages are
`{v: 1, req | event}`, and the shell checks each request against the
permissions the manifest declares before answering it. A direct fetch to the
daemon fails by construction (opaque origin sends `Origin: null`; every write
route's `sameOrigin` gate rejects it, and it carries no cookie or LAN token).

Outbound network is limited by the extension document's CSP `connect-src`,
set to the hosts the manifest declares; the install step shows those hosts
to the user.

v1 is read-only: sessions (list and status), board topics and items, and
tracker data, plus the drawer UI. Write actions are a later, additive
extension of the same bridge.

## Backend extension shape (open)

Owner, answering (563), asked whether an extension can also have a backend
attached to the daemon, tied into the daemon's hooks:

> 563=a^이거 daemon hook이랑 연동할 수 있으면 좋겠는데요. 지금 이 확장은 frontend만 고려한 거 같은데, 혹시 daemon에 붙는 backend 확장까지 고려해볼 수 있을까요? lua? python? 아니면 daemon에 `/ext/events` 같은 websocket endpoint 노출로 별도 프로세스 띄우기 + IPC?

(Gist: the owner wants the extension to connect with daemon hooks and asks
whether a daemon-attached backend could be considered — embedded Lua or
Python, or a daemon WebSocket endpoint such as `/ext/events` that a
separately started process connects to over IPC.)

Candidates (question (569)):

- (a) The daemon spawns a child process the manifest names and talks to it
  over stdio JSON lines with the same `v: 1` schema as the frontend bridge.
  Language-free; no port, no token. MAIN recommends (a).
- (b) The daemon exposes an `/ext/events` WebSocket; a process the user
  starts separately connects with a token.
- (c) The daemon embeds a Lua or JS engine and runs the extension's script
  in-process.

Whichever shape wins, hooks mean: the daemon forwards to the backend the
daemon events the manifest subscribes to (session status, board, tracker).
That generalizes today's `tracker-hooks.json` runner from "tracker events
to a one-shot command" to "subscribed daemon events to an extension
backend".

## Outcome Ledger

### Verified Findings

- **Capability-URL sandbox pattern exists.** The HTML preview's capability
  route `GET /s/<reply-id>/fs/cap/<capability>/<path>` (and the file view's
  `/fs/view/` page route) serves a document under a path-segment
  capability (expiry + MAC under a per-start daemon key) and sends a CSP
  that sandboxes the document itself (`sandbox allow-scripts`, no
  `allow-same-origin`), limits loads to the capability's own path prefix,
  and sets `connect-src 'none'`. See the file comment in
  `agents-plugin-tool/internal/wsdashboard/fs_capability.go` and
  `fsCapabilityCSP` there. The access guard admits the route by shape
  without cookie or LAN token because the opaque-origin frame sends none
  (`previewCapability`, comment in `wsdashboard/access.go`). Relative
  assets resolve under the capability prefix; module scripts, styles, and
  fonts get `Access-Control-Allow-Origin: null` (`capCORS`), and any fetch
  (`Sec-Fetch-Dest: empty`) is refused (`capRefused`).
- **Differences an extension route must handle.** The preview capability
  is scoped to a session's reply-id and project root and expires
  (`fsCapabilityTTL`, invalidated by daemon restart); an extension
  capability scopes to an extension folder instead. The preview CSP's
  `connect-src 'none'` becomes the manifest's declared hosts for (568).
  `capRefused` refusing every fetch also refuses an extension fetching its
  own JSON data files; an extension would ship data as script or receive it
  over the bridge.
- **The sameOrigin gate rejects opaque origins.** `sameOrigin` in
  `agents-plugin-tool/internal/wsdashboard/inbox_routes.go` returns false
  for a missing or `"null"` Origin; the write routes (inbox, rail, holders,
  workspace, gitpush, sysopen, fs act) call it, so a sandboxed extension
  frame cannot call them directly.
- **Iframe-shell data rule.** `261113-feat-ws-dashboard-iframe-shell`
  decides "all data goes through the daemon ... `postMessage` is only for UI
  control" for the shell's own same-origin frames. An extension frame is
  opaque-origin and cannot open the daemon WebSocket, so the extension
  bridge is a deliberate, scoped exception to that rule: the shell (which
  holds the data via its own connection) answers permissioned reads. The
  shell's message shapes are left to its implementer and recorded in its
  Result; the extension bridge's `{v:1, ...}` envelope is a separate
  versioned surface.
- **The tracker hooks runner is the existing outward hook.**
  `agents-plugin-tool/internal/wstrackerhook`: the user-only file
  `tracker-hooks.json` in the daemon state directory lists argv commands
  (no shell) with a scope (`mine`/`all`); each event runs the command once
  with one `Event` JSON (`EventVersion = 1`) on stdin, per-hook serial
  queue, bounded by a run timeout, failures logged and not retried
  (`doc.go`, `config.go`, `event.go`, `RunCommand` in `exec.go`). Only the
  machine's user writes the file, so nothing a store holds chooses what
  runs. Candidate (a) differs in being a long-lived child with a
  bidirectional stream rather than one process per event.
- **Overlap with the daemon extension protocol research.**
  `261112-research-ws-daemon-extension-protocol-chat-adapter` (todo/, no
  decisions on shape yet) covers outward tracker events, session status,
  and notifications, plus inward chat messages into sessions or the
  tracker, over a WebSocket where the daemon dials an adapter server; it
  already names the tracker hook's `v: 1` events as the existing outward
  point and wants a protocol version from the first release. The outward
  event set equals this ticket's backend-hook subscription set (session
  status, board, tracker), and both need versioning and capability or
  permission negotiation. They differ in transport and trust: the chat
  adapter is a remote network peer the daemon dials; an extension backend
  under (a) is a local child process over stdio, and under (b) a local
  client dialing the daemon.

### Confirmed Decisions

- **(562)=a, extension packaging and isolation.** An extension is a folder
  with a manifest plus HTML/JS, served by the daemon on a capability URL
  with CSP `sandbox allow-scripts` (opaque origin, the file-preview
  pattern), rendered in a sandboxed iframe inside a drawer the iframe shell
  owns. Data flows only through the shell's `postMessage` bridge
  `{v: 1, req | event}`, which checks every request against the manifest's
  permissions; direct daemon fetches fail (Origin null, `sameOrigin`
  rejects).
- **(563)=a, v1 scope.** v1 is read-only (sessions list/status, board
  topics/items, tracker) plus the drawer UI; write actions arrive later as
  additive bridge requests (T0). The owner attached the backend question
  to this answer; it is open as (569).
- **(568)=a, outbound network.** The extension document's CSP `connect-src`
  allows only the hosts the manifest declares, and installation shows that
  list to the user.
- **(564)=ok, ticket form.** This is a research ticket in `idea/` with no
  implementation plan.
- **Compatibility tier.** The bridge message schema and the manifest schema
  are a new cross-version surface; T0 when introduced (purely additive), with
  the `v` field versioning the envelope.

### Proposals

- **Share one envelope with the chat-adapter protocol.** Define a single
  versioned envelope (`v`, request/response correlation, `event` with a
  kind) and one event vocabulary (session status, board, tracker; tracker
  events reuse the `wstrackerhook.Event` fields) used by three transports:
  the frontend `postMessage` bridge, an extension backend's stdio (a), and
  the chat adapter's WebSocket. Permission checks stay per transport
  (manifest permissions for extensions; adapter capability negotiation for
  the chat adapter). Non-authoritative: the chat-adapter research has not
  settled its shape.
- **Backend (a), as MAIN recommends.** Daemon-spawned child over stdio JSON
  lines with the same `v: 1` schema; lifecycle tied to the extension being
  enabled; the manifest names the argv like `tracker-hooks.json` does (no
  shell), and only the machine's user installs it.
- **Generalize the hooks file into extension subscriptions.** The
  manifest's subscription list replaces a per-hook scope; `tracker-hooks.json`
  stays as-is (T0) and could later be expressed as a backend-only
  extension.

### Open Questions

- **(569) Backend extension shape** — pending owner answer: (a) spawned
  child over stdio JSON lines (MAIN's recommendation), (b) `/ext/events`
  WebSocket with a token for a separately started process, or (c) an
  embedded Lua/JS engine. Includes how manifest-subscribed daemon events
  (session status, board, tracker) are forwarded to the backend.
- **Schema sharing with the chat adapter.** Whether this bridge's `v: 1`
  envelope and event vocabulary should be the same schema as
  `261112-research-ws-daemon-extension-protocol-chat-adapter`'s protocol
  (see Proposals).
- Manifest format details, the install location and install UX, the
  capability's lifetime for a long-open drawer, and how a backend and its
  frontend panel talk to each other (through the daemon, or not at all in
  v1).

### Rejected Alternatives

- Not recorded here: the options that lost in (562), (563), and (568) are
  in the board topic `user-extensions` design panels; copy them in if a
  derived ticket needs the reasons.
