---
title: ws dashboard daemon core - lifecycle, session discovery, and the session page
parent: 261106-epic-ws-dashboard-serve-daemon
related:
  261106-feat-ws-dashboard-claude-adapter: prerequisite for Phase 2 onward only; its Phase 1 event probe runs before this ticket's Phase 2, while this ticket's Phase 1 lifecycle reads no probe result and may run alongside the probe (266)
  261106-feat-ws-dashboard-live-snapshot-pi: prerequisite for Phase 3; Phase 3 renders the live snapshot schema and the conversation ring (with its mail entries) that its Phase 1 defines; the snapshot and ring writes call this ticket's Phase 2 nudge
  261106-feat-ws-dashboard-browser-interaction: builds on this ticket; its message and cancel routes, composer chips, and held bubble extend the HTTP server and session page of Phase 3
  261106-feat-whiteboard-rich-items: context; Phase 3 serves the shared mermaid asset its page loads by relative path (293)
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: ac72942546448dbd
sage-review-design-reviewed: ac72942546448dbd
---

# ws dashboard daemon core - lifecycle, session discovery, and the session page

## Background

The parent epic `261106-epic-ws-dashboard-serve-daemon` adds a `ws-mcp
dashboard` subcommand: one daemon per machine environment that serves every
ws session on it as a live web page. This child is the daemon itself: its
process lifecycle, how it finds sessions and learns that they changed, and
the read-only HTTP render of the index and session pages. Writes from the
browser, LAN access, and the image route are the sibling
`261106-feat-ws-dashboard-browser-interaction`; the live snapshot and
conversation ring this child renders are defined by
`261106-feat-ws-dashboard-live-snapshot-pi`. Read the epic's Scope,
Non-Scope, and Cross-Child Decisions for the shared picture; the decisions
below are the ones that bind this child, condensed with their item numbers.

Lifecycle is isolated as Phase 1 because the dropped web-dashboard MVP's
bugs came from it (201): a helper killed while healthily serving another
daemon because "connected but unanswered" was read as dead (bff07cafb), probe
faults authorizing reclaim (f55de2d62), and a registry entry verified by
`(pid, start_time)` that matched an unrelated process after a reboot
(e6caac0dc).

The static whiteboard page and dashboard panels this daemon reuses
(`261106-feat-whiteboard-dashboard-panels`) and the mailbox history ring
(`261106-feat-mailbox-history-ring`) are done.

## Decisions

**Process and transport**
- **stdio MCP unchanged; the daemon is optional.** MCP behavior never
  depends on the daemon. When the daemon is down, every whiteboard and
  dashboard feature works through the static page, and the pointer line
  prints the file path. (168), (180)
- **Daemon entry point.** The daemon is a new `ws-mcp dashboard`
  subcommand. Rejected: a non-stdio flag on `ws-mcp serve`, which is the
  stdio MCP entry point every launcher calls (`cmd/ws-mcp/main.go`); a
  separate subcommand keeps that path untouched.
- **The static page and the embed stay.** The daemon renders the same
  `View` and templates as the static page and pushes updates to the browser
  over SSE. (170) It reuses the panel providers of
  `261106-feat-whiteboard-dashboard-panels` rather than defining its own.
  (171)

**Lifecycle and discovery**
- **One daemon per OS environment.** One daemon serves the machine however
  many ws-mcp processes run. A WSL distro and native Windows each count as
  their own environment. (173), (179)
- **Single instance by OS file lock.** The daemon holds an OS file lock
  (flock / LockFileEx) on a lock file in the machine cache directory. It
  writes an endpoint file with port, pid, version, and token. ws-mcp reads
  the endpoint file and checks `/health`. When the daemon is absent or
  older, ws-mcp spawns it detached; an older daemon is replaced through the
  handoff below. (174), (178) Rejected: pid files and port-as-lock, which go
  stale or race.
- **Replacement handoff and version tolerance.** A newer ws-mcp asks the
  older daemon to shut down through a request authenticated with the
  endpoint-file token, waits for the lock to free, then spawns the new
  daemon. Phase 1 states whether an idle daemon exits, how a daemon running
  from a versioned plugin cache avoids blocking that directory's removal on
  Windows, and how a hung daemon that holds the lock but fails `/health` is
  handled. Readers of the snapshot, the machine registry, and the nudge body
  ignore unknown fields and treat absent ones as empty, since a newer daemon
  reads what older ws-mcp processes still write (the e6caac0dc registry
  history). Rejected: terminating the older daemon by pid, which races pid
  reuse.
- **Cross-platform.** Discovery and the single-instance lock work the same
  on Linux, macOS, and Windows. (173)
- **Spawn at `serve` startup, behind a config key.** A stdio `ws-mcp serve`
  checks for the daemon at startup and spawns it when absent, gated by a
  config key whose default is on in ws, off in wsflow, and on in Pi (Pi
  ships one distribution, the (291) reading). The key off means fully opted
  out: that ws-mcp neither registers its root, spawns the daemon, nor sends
  nudges, so its sessions never appear on the dashboard, and the pointer
  line prints the file path. (285) Rejected: spawning on the first
  whiteboard update only, which leaves sessions without a board (lead bar,
  tree, conversation) unserved; no automatic spawn, only an explicit
  command.

**Data flow**
- **The daemon reads on-disk stores only.** A stdio ws-mcp registers its
  project root in a machine registry so the daemon can find each session's
  stores. There is no protocol that pushes data into the daemon. (169) The
  daemon's one write target, the board-answer inbox, belongs to the
  browser-interaction sibling.
- **Nudge plus polling.** After a store write, ws-mcp sends a
  fire-and-forget `POST /nudge {reply-id, project}`; errors are ignored. The
  daemon re-reads that session. A polling backstop of about 30 seconds
  catches missed nudges. (181) Rejected: fsnotify (platform variance).
- **Nudge wiring for snapshot and ring writes.** The nudge sender lands in
  Phase 2 here; the snapshot and conversation-ring writes of
  `261106-feat-ws-dashboard-live-snapshot-pi` Phase 1 call it. Whichever of
  the two lands second wires that call. (295) Rejected: serializing the two,
  since both wait only on the Claude probe.

**Network and URLs**
- **Localhost guards on the routes this child adds.** In localhost mode
  every route checks the Host header against loopback names, which stops
  DNS rebinding. Machine routes carry no Origin: `/nudge` needs no
  authentication, since it only makes the daemon re-read disk, and the
  handoff's shutdown request requires the endpoint-file token. The daemon
  sends no CORS headers. (284) LAN mode, where a request with a loopback
  peer address and a loopback Host header still passes by this Host check
  and every other request needs the access token (299), and the Origin
  check on browser write routes belong to the browser-interaction sibling. Rejected: a token on localhost too, which
  puts the credential in the pointer-line URI and so in agent transcripts.
- **Routes.** `/` lists projects and sessions. `/s/<reply-id>` shows one
  session's page. The page is keyed by reply-id, never by session key,
  because a session key is tool authority. (180)
- **Port.** A fixed default port, configurable. When it is taken, the
  daemon falls back to an ephemeral port and records it in the endpoint
  file. (180) The port is a config key, its default a fixed unassigned high
  port, which the spawning ws-mcp passes to `ws-mcp dashboard` as a flag.
  (286) Rejected: an environment variable, since config keys already carry
  per-package defaults and show in `config.list`.
- **Config key home.** The ws and wsflow manifests are single-namespace
  `claude.` (`internal/mcp/config_manifest.go`), and Pi's ws-mcp loads only
  the `pi.` manifest (`agents-plugin-pi/src/bridge.ts`). So each package
  (ws, wsflow, Pi) ships a `dashboard.`-namespace config manifest, listed in
  its launcher's `WS_MCP_CONFIG_MANIFESTS` beside its existing manifest,
  that carries the dashboard and port keys with that package's defaults.
  (design review) Rejected: declaring the keys in the existing `claude.`
  manifests, which puts a host-neutral key under the `claude.` namespace and
  which Pi's ws-mcp does not load.
- **Shared page assets.** The daemon serves the shared static assets the
  whiteboard page loads by relative path, including the mermaid asset of
  `261106-feat-whiteboard-rich-items`, so a served page renders the same as
  the static one. (293) Rejected: leaving the asset unowned; no other child
  serves static assets.
- **Pointer line.** When the daemon is up, the whiteboard pointer line
  prints the session's URI; otherwise it prints the file path. (180)
- **Links in served pages.** Link resolution is an injected callback of
  `RenderPage` (the rich-items ticket; panels use `whiteboardFileLink`), and
  the daemon injects its own resolver. In a served page a non-image file
  link renders as its repo-relative path text, not a link: browsers block
  `file:` links from http pages, and serving arbitrary files is out of
  scope. The browser-interaction sibling's Phase 2 extends this resolver so
  a board image maps to its image route URL. (design review) Rejected:
  reusing the static page's `file:` and page-relative links, which break or
  resolve under `/s/` in a served page.

**Session page**
- **Partial updates.** The page never reloads. The daemon sends each changed
  region (lead bar, subagent tree, conversation, topics, panels) as an HTML
  fragment over SSE, rendered with the same server templates as the static
  page, and the page swaps only that region. The composer is never swapped,
  so typed text and attached chips survive every update. (239) Rejected:
  full reloads, which lose a message being typed and the scroll position;
  JSON plus client-side rendering, which duplicates the templates in
  JavaScript.
- **View state survives swaps.** UI state inside a swapped region (open
  `<details>`, expanded tree nodes, scroll positions) is remembered by the
  page by element key and re-applied after the swap. (255) Rejected:
  rendering open state on the server, which would make the server track each
  browser's view.
- **Layout.** The default layout is side by side: the left column holds the
  subagent tree, the conversation, and the composer; the right column holds
  the board with the panels under it. The page itself does not scroll; each
  column scrolls on its own, and the conversation stays pinned to the bottom
  unless the user scrolls up. On a narrow screen the page switches to tabs: a
  conversation / board switch with the composer pinned to the bottom, the
  tree and panels in a side column, and a marker on the hidden tab when its
  region updates. (248) Rejected: tabs as the default, which never shows a
  question and the conversation at once; conversation first with the board
  in a drawer, which hides the board, the primary channel, by default.
- **Layout switch.** A settings control in the top-right corner lets the
  user force either layout. Both layouts arrange the same region elements
  with CSS only (one grid-template-areas rule per layout), and the choice is
  stored in the browser's localStorage. (257) A host without an adapter has
  only the board-first layout, so its page hides the control. (278)
  Rejected: offering the drawer layout as a third mode, which adds its own
  open state, overlay, and update alert to maintain.
- **Lead bar and tree height.** The lead bar stays a one-line status in
  every layout; the lead's recent batches show as the root node of the
  subagent tree. (254) In the side-by-side layout the tree is capped at about
  38% of the column height and scrolls by itself; a running agent shows only
  its live row and a finished one its header and caption, and a click
  expands its last 5 batches. (256) Rejected: an uncapped tree, which pushes
  the composer off screen when many subagents run.
- **Lead phase on the index.** `/` shows each session's lead phase as a
  small badge (thinking, replying, tool, waiting for permission, idle) from
  the same snapshot field. (220), (241)
- **Hosts without an adapter.** A host with no adapter, such as Codex, shows
  only the state in its on-disk stores: no lead bar, no conversation, and no
  browser submission; its page keeps only the copy-answers button and uses a
  board-first layout with the board and panels only. (190), (211), (219),
  (252) Rejected: falling back to mail for those hosts (211 b), which would
  put user input back into the mailbox.
- **Subagent tree labels.** A delegate that holds a ws session key is
  labelled with that session's reply-id, linked to its `/s/<reply-id>` page
  when it has a board; the session key itself never reaches the browser.
  (192), (243) Rejected: showing the delegate's session key, which exposes
  tool authority to the browser.

**Conversation**
- **Conversation stream.** The page shows the conversation in time order:
  user messages tagged by origin (whiteboard or terminal), the lead's reply
  text, streaming while the lead is replying (250), and turn summary lines.
  When the lead waits on a tool permission prompt, the conversation and the
  lead bar show that it is waiting for approval in the terminal, with the
  tool and its argument preview. (241) Rejected: showing the reply text only
  once each message ends (250 b); keeping the reply text in the terminal
  only (241 c).
- **Mail lines.** Mail the lead sends or receives shows as a line,
  `→ peer: first line` or `← peer: first line`, linked to the peer's page
  and expanding to the full body from the mailbox history ring; a turn that
  incoming mail starts carries a `mail` origin tag. (260) The history ring
  keeps 10 messages per session, so a mail line whose message has left it
  shows its first line with no expansion. (283) Rejected: showing mail in
  the mailbox panel only (260); copying mail bodies into the conversation
  ring so every line expands (two stores of the same body, and unbounded
  entry size) (283).

**Peer leads**
- **Peer lead group.** The tree's lead root holds one "peer lead" group: the
  sessions this lead exchanges mail with. Each row shows the peer's lead
  phase line from the peer's own snapshot, the last exchange time, a
  `/s/<reply-id>` link, and the peer's subagent count. (259) A peer row is
  collapsed by default; a peer the user expands shows that peer's lead
  batches and subagent tree, one level deep: the peer's own peer group never
  appears. (279) The daemon renders every listed peer's tree inside a
  collapsed `<details>`, and the browser keeps the expanded set in
  localStorage keyed by peer reply-id. (280) A peer without an adapter, such
  as Codex, shows no phase. (263) Rejected: recursive peer trees (A and B
  mailing each other form a cycle, and each page would fan in every peer's
  updates); peers that never show a tree; the browser reporting its expanded
  set so the daemon renders only those trees (adds browser-to-daemon state
  for a list capped at live peers plus 5); a badge on the mailbox panel only.
- **Peer resolution.** An `id:<reply-id>` address resolves directly; a named
  address goes through the mailbox presence record's owner session key and
  `ReplyID()`. Only the reply-id reaches the browser. A peer this daemon
  does not serve shows its address and last exchange time with no live
  state. (261) Contacts that resolve to the same reply-id merge into one row
  with the latest exchange time. A named contact resolves through the
  presence owner at render time, so after a rebind the row follows the
  address's current holder.
- **Peer retention.** The group shows every live peer and the last 5 ended
  peers, ordered by last exchange. (262) An unserved peer counts as live
  only while its mailbox presence record is alive (276); for an
  `id:<reply-id>` peer that record is the reply-id registry entry, read the
  way `mailboxPresenceLive` reads named presence: live while its `LastSeen`
  is within 10 minutes (`mailboxPresenceLive` also requires
  `wsstate.ProcessAlive(p.PID)` for named presence, and `ReplyEntry` carries
  no PID, so only the `LastSeen` half applies:
  agents-plugin-tool/internal/mcp/mailbox_runtime.go#L164-L173,
  agents-plugin-tool/internal/wsmailbox/replyid.go#L41-L46). A served peer's liveness comes from its own session
  state in this child's session index. (282) Rejected: always listing
  unserved peers as live, which grows without bound from stale contacts;
  counting a reply-id peer live while its registry entry exists, which the
  30-day lazy reap (`mailboxReplyIDRetention`) turns into 30 days of a
  finished peer shown live; counting every unserved reply-id peer as ended
  (282); a ticker refresh of `LastSeen`, which writes under the machine-wide
  reply lock on every tick.

**Ordering**
- **Claude probe first, except Phase 1.** Phase 2 onward waits for the Claude
  adapter child's Phase 1 probe; Phase 1 lifecycle depends on no probe result
  and may run alongside it. (266) Rejected: holding the core lifecycle phase
  until the probe lands (266 b), which serializes a phase that reads no
  snapshot.

## Constraints

- Shipped surface: the `ws-mcp dashboard` subcommand, its HTML, CSS, and
  scripts, the pointer line, the three `dashboard.` manifests (the Pi one in
  `agents-plugin-pi/`, which has no Implementation Conventions row but ships
  downstream all the same), and every string ws-mcp emits ship downstream;
  none may name this repository's tickets, commits, or layout.
- The `ws-mcp serve` stdio path and every MCP tool's behavior stay unchanged
  when the daemon is absent, down, hung, or older; nudge and health errors
  never surface as tool errors. (168)
- The daemon holds no session state that only it knows; a replacement
  daemon rebuilds everything from disk. (169), (174)
- Out of scope: the board-answer inbox, message and cancel routes, composer
  chips, LAN mode, the access token, and the image route (browser-interaction
  sibling); the snapshot and ring writers and their schema (live-snapshot
  sibling); adapter code; the MCP transport.
- Reuse points: `wswhiteboard.View` and templates in
  `agents-plugin-tool/internal/wswhiteboard/`, panel providers in
  `agents-plugin-tool/internal/mcp/whiteboard_panels.go`, the mailbox
  history ring in `internal/mcp/mailbox_history.go`, `Presence.Owner` in
  `internal/wsmailbox/store.go`, `ReplyID` and reply entries in
  `internal/wsmailbox/replyid.go`, `mailboxPresenceLive` in
  `internal/mcp/mailbox_runtime.go`, the machine cache root
  `wsstate.CacheRoot`, the pointer line written in
  `internal/mcp/whiteboard_tools.go`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "stdio MCP unchanged; the daemon is optional. MCP behavior never depends on the daemon. When the daemon is down, every whiteboard and dashboard feature works through the static page, and the pointer line prints the file path." — bearing: constrains
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "Routes. `/` lists projects and sessions. `/s/<reply-id>` shows one session's page. The page is keyed by reply-id, never by session key, because a session key is tool authority." — bearing: constrains
- 261106-feat-whiteboard-dashboard-panels (2026-11-06, Decisions): "Mailbox panel. The session's own address. Unread count... Whether the session is waiting for mail, from the listening marker... Correspondents with their last exchange time" — bearing: supports
- 261106-feat-mailbox-history-ring (2026-11-06, Result b1771b9df): "one file per entry under `<cache root>/mailbox-history/<session-key>/`, written temp+rename...; best-effort trim to the newest 10; read newest first, skipping unreadable/unparsable/unstamped entries" — bearing: supports
- 261106-feat-ws-dashboard-claude-adapter (2026-11-06, Decisions): "Adapters hand events to their session's ws-mcp. The mod does not post to the daemon. Rejected: adapters posting to the daemon directly." — bearing: constrains
- bff07cafb (2026-07-29, commit): "fix(dashboard): kill helpers only on positive evidence of unreachability" — bearing: constrains
- e6caac0dc (2026-07-29, commit): "fix(dashboard): gate terminal-registry kills on a recorded boot identity" — bearing: constrains
- 260904-bug-windows-parent-watch-pid-reuse-flake (2026-09-04, Open Questions): "Assert on process identity, not just handle-openability — capture the helper's creation time (`GetProcessTimes`) before kill... (true PID-reuse detection)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/main.go, new daemon package, internal/mcp/whiteboard_tools.go, internal/mcp/whiteboard_panels.go, internal/wswhiteboard/, the ws, wsflow, and Pi `dashboard.`-namespace manifests |
| scope.surface | public-interface | new ws-mcp dashboard subcommand, HTTP routes, two config keys, pointer line text |
| scope.new_public_symbol | yes | ws-mcp dashboard subcommand, /health, /nudge, /, /s/<reply-id>, dashboard and port config keys |
| scope.new_type_contract | yes | endpoint file, machine root registry entry, nudge body, served-session liveness rule |
| scope.test_surface | new-files | new tests beside daemon code plus existing cmd/ws-mcp/main_test.go, internal/mcp/whiteboard_tools_test.go, whiteboard_panels_test.go, mailbox_correspondents_test.go |
| complexity.reuse_points | confirmed | wswhiteboard.View and RenderPage in internal/wswhiteboard/view.go and render.go, mailboxPresenceLive, wsmailbox.ReplyID, Presence.Owner, wsstate.CacheRoot, mailbox_history.go all present |
| complexity.side_effect_risk | high | every serve startup may spawn a detached machine-wide process, hold an OS lock, and write cache files |
| risk.correctness | high | single-instance lock, handoff, hung-daemon handling, and version comparison repeat the failure class of bff07cafb, f55de2d62, e6caac0dc |
| risk.fit | moderate | new daemon package must reuse static-page templates and panel providers without forking them |
| risk.test | high | cross-platform lock and Windows cache removal untestable here; page behavior relies on manual browser checks with no headless browser |
| risk.security_or_contract | high | new localhost HTTP server with Host-header guard, token-authenticated shutdown, and a no-session-key-in-browser invariant |

## Phases

### Phase 1: Daemon lifecycle

Add the `ws-mcp dashboard` subcommand with the single-instance OS file lock
in the machine cache directory, the endpoint file (port, pid, version,
token), `/health`, the token-authenticated shutdown request, the default
port with ephemeral fallback, the Host check for localhost mode on every
route, and the ws-mcp side: at `serve` startup, unless the dashboard config
key is off (285), read the endpoint file, check `/health`, spawn the daemon
detached with the configured port (286) when it is absent, and replace an
older one through the handoff (shutdown request, wait for the lock to free,
spawn). Both config keys live in a `dashboard.`-namespace config manifest
that each package (ws, wsflow, Pi) ships and lists in its launcher's
`WS_MCP_CONFIG_MANIFESTS` beside its existing manifest.

The worker settles each of these and records the choice and its reason in
the Result:

- The two config key names and the default port number. Constraint: the
  dashboard key defaults on in ws, off in wsflow, and on in Pi (285), (291);
  the port is an unassigned high port (286).
- The exact manifest wiring per package: file names, and how each launcher
  lists two manifests (the Claude manifests' `env`, Codex `.mcp.json`, and
  the Pi bridge, which today sets `WS_MCP_CONFIG_MANIFESTS` to one path).
  Constraint: the platform path-list separator; a manifest that fails to
  load leaves the dashboard off rather than failing `serve`.
- Whether an idle daemon exits, and if so on what condition. Constraint: a
  ws-mcp that finds no daemon spawns one, so an exit never strands a running
  session without a way back.
- How a daemon running from a versioned plugin cache avoids blocking that
  directory's removal on Windows.
- How a hung daemon (holds the lock, fails `/health`) is handled.
  Constraint: never terminate a daemon by pid (pid reuse); "no answer" is
  not proof the holder is gone (bff07cafb).
- How versions compare for "older", including `-dev` builds.
- The endpoint file's encoding. Constraint: readers ignore unknown fields
  and treat absent ones as empty.

Verification: Go tests beside the new daemon code and in
`agents-plugin-tool/cmd/ws-mcp/main_test.go` cover a second daemon refused by
the lock, the endpoint file written and read back with unknown fields
ignored, `/health`, the shutdown request rejected without the endpoint-file
token and accepted with it, the replacement handoff end to end (older daemon
exits, lock frees, newer daemon writes its endpoint file), an older ws-mcp
leaving a newer daemon running (no replacement flapping), each package's
`dashboard.` manifest loading beside its existing one with the per-package
defaults, a missing or unparsable `dashboard.` manifest leaving the
dashboard off while `serve` still runs, port fallback when the default is
taken, a non-loopback Host header refused, the hung-daemon path, the
dashboard key off spawning no daemon and the port key reaching the spawned
daemon's flag, and a ws-mcp `serve` whose MCP tools answer normally with no
daemon, then `go test ./...` in `agents-plugin-tool`. The Pi launcher change
(the bridge's `WS_MCP_CONFIG_MANIFESTS` listing both manifests with the
platform separator) is covered by a test in `agents-plugin-pi/test/` (for
example extending `bridge.test.ts`), run with `npm test` in
`agents-plugin-pi`. The lead checks by hand on this
machine (WSL) that a detached daemon survives the ws-mcp that spawned it and
that two concurrent ws-mcp starts leave one daemon. macOS and native Windows
(LockFileEx, cache-directory removal) need a lead or user run on those hosts;
the Result names which platforms were exercised live.

### Phase 2: Root registry, session index, nudge, and polling

Each stdio ws-mcp registers its project root in a machine registry the
daemon reads. The daemon builds a session index from the registered roots'
on-disk stores, re-reads one session on `POST /nudge {reply-id, project}`,
and polls every registered root about every 30 seconds. ws-mcp sends the
nudge fire-and-forget after each store write.

The worker settles and records in the Result:

- The registry's location and entry shape, and how stale entries (a
  crashed ws-mcp, a removed root) are recognized. Constraint: readers ignore
  unknown fields; no liveness verdict rests on `(pid, start_time)` alone
  (e6caac0dc). Constraint: a session whose dashboard key is off never
  appears on the dashboard, even when another key-on ws-mcp registered the
  same project root (ws and wsflow in one project, or a session-scope
  tune); for example, register per session or reply-id rather than per
  root, or keep a per-session opt-out marker the index honors. (285)
- The nudge body's encoding of `project`. Constraint: readers ignore unknown
  fields; a nudge naming an unknown session is ignored.
- The served-session liveness rule the session index exposes, which the peer
  group's retention (Phase 3) reads for a served peer. (282)
- Which store writes send the nudge. Constraint: a nudge failure never
  delays or fails the write.
- Which on-disk stores define the session set under a registered root, and
  how a session maps to its reply-id. Constraint: a session with a live
  snapshot but no board is listed (285). Record it so the live-snapshot
  sibling's Phase 1 places the snapshot where this index enumerates.

Verification: Go tests cover registration and stale-entry handling, a nudge
that re-reads exactly one session, a malformed or unknown nudge ignored, the
polling backstop picking up a write that sent no nudge (with an injected
clock or short interval), unknown registry fields ignored, and ws-mcp store
writes sending a nudge whose failure leaves the tool result unchanged, a
ws-mcp with the dashboard key off registering no root and sending no nudge
so its sessions never enter the index, two ws-mcp processes on one root
(one key-on, one key-off) leaving only the key-on session in the index, and
a session with a snapshot but no board listed (extending
`agents-plugin-tool/internal/mcp/whiteboard_tools_test.go` or a new test
file in `internal/mcp`). If this phase lands after the live-snapshot
sibling's Phase 1, the tests also cover its snapshot and ring writes sending
a nudge. Then `go test ./...` in `agents-plugin-tool`. The lead checks by
hand that two sessions in two projects both appear in the index of one
running daemon.

### Phase 3: Index and session page render

Serve `/` (projects, sessions, lead phase badge) and `/s/<reply-id>`
(lead bar, subagent tree with the lead as root and the peer lead group,
conversation with mail lines, topics, board, panels, composer region) from
the session index and the on-disk stores, with partial SSE region updates,
view-state re-application after swaps, the side-by-side and tabs layouts
with the layout switch, the board-first page for a host without an adapter
(read from the adapter-presence signal the live-snapshot sibling's Phase 1
settles (297)), the daemon's own link resolver injected into `RenderPage`
(a non-image file link renders as repo-relative path text), the shared
static assets the page loads by relative path, including the
mermaid asset (293), and the pointer line printing the session URI when the
daemon is up. The
composer region renders and is never swapped; sending from it is the
browser-interaction sibling.

The worker settles and records in the Result:

- How ws-mcp decides "the daemon is up" for the pointer line. Constraint:
  the check never blocks or fails `whiteboard.update`, and a down daemon
  prints the file path. (168), (180)
- How the SSE stream maps a session re-read to changed regions, so an
  unchanged region is not resent.
- Whether `/` lists ended sessions, and for how long. Constraint: it uses
  the served-session liveness rule Phase 2 settles. (282)
- How a board image renders in a served page before the
  browser-interaction sibling's Phase 2 lands, for example as path text or
  alt text. Constraint: no `file:` URL and no path resolving under `/s/`.

Verification: Go tests render `/` and `/s/<reply-id>` from fixture stores
and snapshot files: region fragments match the static page's templates for
the board and panels, an unknown reply-id is a 404, no session key appears
in any rendered page or fragment, a host without an adapter renders the
board-first page without the layout control, the conversation renders origin
tags, streamed reply text, and the permission-wait state, a mail line
expands when its history entry exists and shows only its first line when it
has left the ring, peer rows merge an `id:` contact and a named contact of
one session, follow a rebind, render one level deep, and apply the live and
ended rules (served peer from the session index, unserved named peer from
presence, unserved `id:` peer from `LastSeen` within 10 minutes, last 5
ended), `/` applying the ended-session rule settled above, no served page or
fragment carrying a `file:` URL or a page-relative path that resolves under
`/s/` (a non-image file link renders as its repo-relative path text, and a
board image renders per the rule settled above), the board-first page chosen
from the adapter-presence signal, an SSE client receives only the changed
region after a nudge, and the pointer line prints the URI with the daemon up
and the file path with it down (extending
`agents-plugin-tool/internal/mcp/whiteboard_tools_test.go`,
`whiteboard_panels_test.go`, and `mailbox_correspondents_test.go` where they
fit), then `go test ./...` in `agents-plugin-tool`. No headless browser is
available here, so the lead opens a live session page by hand and checks: a
region update keeps typed composer text, open `<details>`, and scroll
positions; the conversation stays pinned to the bottom unless scrolled up;
the narrow-screen tabs and the hidden-tab marker; the layout switch and its
persistence across reload; an expanded peer staying expanded across reload;
the tree cap at about 38% of the column.

## Sage Review Round 1 (2026-11-06)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Dashboard and port keys have no host-neutral home in the ws/wsflow manifests | important | autonomous |
| 2 | Registration and nudging for a session whose key is off | minor | autonomous |
| 3 | Session enumeration source unstated | minor | autonomous |
| 4 | Cross-ticket [browser-interaction, daemon-core]: LAN loopback contract versus the epic's token-replaces-Host-check | important | missing |
| 5 | Cross-ticket [daemon-core, browser-interaction]: no owner for link resolution in daemon-served pages | important | autonomous |
| 6 | Cross-ticket [claude-adapter, live-snapshot-pi, daemon-core]: adapter feed hand checks need core Phase 3 | minor | autonomous |
| 7 | Cross-ticket [all four]: phase-level ordering cannot be typed edges | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | How sessions are enumerated within a registered root is not stated | minor |
| 2 | Whether / lists ended sessions is not stated | minor |
| 3 | How the page detects a host without an adapter is not stated | minor |
| 4 | The cross-ticket nudge wiring has no verification owner | minor |
| 5 | The handoff direction for an older ws-mcp is implicit | minor |
