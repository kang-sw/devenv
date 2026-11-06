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
completed: 2026-11-06
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

### Result (5b359f73c) - 2026-11-06

Landed: `internal/wsdashboard` (lock, endpoint file, loopback HTTP server with
`/health` and `/shutdown`, Host guard, `Ensure`, detached spawn, Windows binary
staging), the `ws-mcp dashboard [--port N] [--lock-wait D]` subcommand (exit 3
when another daemon holds the lock), `go serveDashboard(server)` beside the
stdio loop in `serve`, `Server.DashboardSettings` in `internal/mcp`, the three
`dashboard-config-manifest.json` files, and every launcher listing them.

Settled:
- **Keys and port.** `dashboard.enabled` (boolean; ws true, wsflow false, Pi
  true) and `dashboard.port` (integer 0-65535, 0 = OS-assigned; 29417 in all
  three). 29417 is IANA-unassigned (block 29170-29998, checked against the IANA
  CSV) and below the Linux (32768+) and Windows (49152+) ephemeral ranges. Both
  are read once at serve startup with no session key: project, global, and
  built-in values only (the descriptions say so). Phase 2's per-session opt-out
  needs its own read.
- **Manifest wiring.** Each package root ships `dashboard-config-manifest.json`
  (namespace `dashboard.`). `.mcp.json` and `.claude-plugin/plugin.json` list
  `<own>;<dashboard>`; the Pi bridge joins the two with `path.delimiter`.
  `WS_MCP_CONFIG_MANIFESTS` now accepts `;` on every platform in addition to the
  platform separator, because a plugin manifest's env block is one file for
  every platform and cannot write the platform separator. An undeclared
  `dashboard.enabled` means off, so a missing, unlisted, or rejected dashboard
  manifest leaves the dashboard off while `serve` runs.
- **Liveness.** Only a daemon process takes the lock (gofrs/flock: flock or
  LockFileEx on `<cache root>/dashboard/dashboard.lock`); ws-mcp never probes
  it, so a probe can never make a starting daemon think the lock is held.
  `/health` counts only when its service name and pid match the endpoint file.
  A pid is never signalled.
- **Hung daemon.** An unreachable recorded daemon (refused, unanswered,
  mismatched) leads to a spawn, and the lock decides. A live but hung holder
  keeps the lock; the new daemon waits its 5 s lock wait and exits; Ensure
  reports unavailable and nothing is terminated. The holder stays until the user
  stops it or the machine restarts.
- **Handoff.** A healthy older daemon gets `POST /shutdown` with
  `Authorization: Bearer <token>`. It removes its endpoint file, then releases
  the lock, and the new daemon's lock wait covers "wait for the lock to free".
  Ensure starts at most two daemons; the second only replaces an older daemon
  that a concurrent older ws-mcp started and that won the lock race.
- **Versions.** `CompareVersions` compares MAJOR.MINOR.PATCH (optional `v`)
  first. With equal cores a pre-release is older than the release, and two
  pre-releases compare as strings. An unparsable version is older than any
  parsable one. The order is total, so replacement cannot flap. Consequence for
  dogfood: unstamped local builds all report `0.48.10-dev`, so a rebuilt dev
  ws-mcp does not replace a running dev daemon built from older code. Stop it by
  hand (POST `/shutdown` with the endpoint token) when testing daemon changes.
- **Idle exit.** None. Spawning happens only at serve startup, so an idle exit
  would leave already-running sessions without a dashboard until their ws-mcp
  restarts. An idle daemon costs a few MB and no CPU.
- **Windows cache removal.** On Windows the daemon runs from a copy at
  `<cache root>/dashboard/bin/ws-mcp-dashboard-<sha16>.exe`, with that directory
  as its cwd, so it never locks a versioned plugin cache. Other copies are
  pruned on the next stage; a running copy survives until a later stage.
  Spawn tries `CREATE_BREAKAWAY_FROM_JOB` first, then plain `DETACHED_PROCESS`
  when the host's job forbids breakaway. On POSIX the daemon runs in a new
  session (`Setsid`).
- **Endpoint file.** JSON `{schema_version, port, pid, version, token,
  started_at}` at `<cache root>/dashboard/endpoint.json`, mode 0600, written via
  temp file and rename. Readers ignore unknown fields and treat absent ones as
  empty.

Verification:
- `go test ./... -count=1` in `agents-plugin-tool`: all packages ok.
- `go test ./internal/wsdashboard -race`: ok.
- `go vet` for linux and windows on the new packages, plus a darwin build: ok.
- `python3 -m unittest discover agents-plugin-wsflow/tests` (20) and
  `agents-plugin/tests` (81): ok.
- Pi `npm test`: 35 failures and 1 cancelled out of 2514. The base commit
  5b359f73c fails the identical set (diffed), and the updated bridge manifest
  test passes.

Tests:
- In-process in `internal/wsdashboard`:
  - the lock refusal;
  - the endpoint round trip with unknown fields;
  - `/health`, including the pid mismatch;
  - shutdown without, with a wrong, and with a bare token;
  - the Host guard on both routes;
  - the port fallback;
  - the handoff;
  - an older client leaving a newer daemon;
  - a concurrent older winner being replaced;
  - the hung holder left alone;
  - a stale endpoint.
- In `cmd/ws-mcp`:
  - the key off spawning nothing;
  - the port key reaching `--port`;
  - the real subcommand refused by the lock (exit 3) and stopped by token;
  - a real end-to-end handoff in which an older-stamped binary's daemon is
    replaced by `serve` and the new detached daemon still answers after `serve`
    exits;
  - `serve` answering `tools/list` with the key off, the manifest missing, and
    the daemon unable to start.
- In `internal/mcp`: the three manifests loading beside their packages'
  manifests with per-package defaults and differing only in the enabled
  default.

Platforms exercised live: Linux (WSL2) only, through the tests above. Not yet
done:
- the lead's hand checks on this machine (a detached daemon surviving its
  spawner in a real host session; two concurrent ws-mcp starts leaving one
  daemon);
- macOS;
- native Windows (LockFileEx, breakaway, cache-directory removal).

Lite review: no Critical or Important findings. Three Minors were left unfixed:
- Windows staging prunes another spawner's in-flight `.tmp` or not-yet-started
  copy, so a simultaneous two-version start can fail one spawn until restart.
- After two spawns, Ensure reports started or replaced even when the winner is
  still older.
- Unstamped dev builds do not replace each other (noted above).

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

### Result (ee0520fa7) - 2026-11-06

Landed:
- `internal/wsdashboard`: `registry.go` (the session registry, the liveness
  rule, and pruning), `index.go` (`Index` built from the registry through a
  `SessionSource`, with `Refresh`, `Nudge`, `Sessions`, `Session`, and an
  `OnChange` hook), the daemon's `POST /nudge` route and poll loop
  (`DaemonOptions.Sessions`, `PollInterval`), and the `SendNudge` client.
- `internal/mcp/dashboard_sessions.go`: registration, heartbeat, the
  ended mark, the nudge queue, and `DashboardSessionSource`. The `serve` loop
  runs the heartbeat. `ws-mcp dashboard` passes the source.
- The `dashboard.enabled` description in all three manifests (8af00f482),
  and a "Dashboard daemon" section in the ws-mcp manual.

Settled:
- **Registry.** One directory per session:
  `<cache root>/dashboard/sessions/<reply-id>/registration.json`. The file is
  JSON `{schema_version, reply_id, project, harness, registered_at,
  heartbeat_at, ended_at}`, written via temp file and rename. Readers ignore
  unknown fields, and absent fields read as empty.
  - It registers sessions, not roots. A stdio ws-mcp registers a top-level
    lead session at that session's first keyed call, but only while
    `dashboard.enabled` resolves on for that session (session scope
    included).
  - A key-off session is never written. If a stale entry for it exists, it is
    removed. A session-scope tune is re-read at once after `config.tune`,
    and otherwise on a keyed call at least a minute after the last check.
  - This is how ws beside wsflow on one root, or a session tuned off, stays
    off the dashboard.
  - Child sessions are not registered. They have no board and appear in
    their lead's tree.
  - No session key and no pid is recorded.
- **Stale entries.** Pruning is done by ws-mcp only, at its first
  registration after a process start. It deletes entries last seen more than
  7 days ago, and directories without a registration whose mtime is that old.
  The daemon only reads. It drops from its index any session whose
  registration file is gone, whose project root is gone, or whose session
  record no longer maps to a top-level lead rooted at that project.
- **Served-session liveness rule (Phase 3 peer group).**
  `Registration.Live(now)`: live means `ended_at` is empty and `heartbeat_at`
  is within 5 minutes.
  - ws-mcp heartbeats every 2 minutes, and stops for a session that has made
    no keyed call for 12 hours. Such a track, abandoned inside a long-lived
    host process, therefore reads as ended, and its next call revives it.
  - A clean serve exit, or a parent-death exit, writes `ended_at`. A crash
    reads as ended within 5 minutes.
  - `Index.Session(replyID).Live` exposes the rule.
- **Nudge body.** JSON `{"reply_id", "project"}`, where `project` is the
  session root exactly as the registration records it. The daemon answers:
  - 204: the session was re-read; only that session is touched.
  - 400: malformed body.
  - 404: unknown session, or a project that does not match.

  `/nudge` passes the loopback Host guard and needs no token. The sender
  ignores the answer.
- **Which writes nudge.** A successful `whiteboard.update`, and every tool in
  `whiteboardProviders`' dirtied-by lists (todo, agenda, route, ticket
  moves, mailbox send and recv), via `Server.dashboardNudge(sessionKey)`.
  - A new registration also nudges.
  - Nudges go through a bounded queue of 64 that never blocks the caller. One
    goroutine drains it: it folds duplicates and reads the endpoint file for
    each batch. A full queue drops the nudge, and the poll covers it.
  - Nothing is sent for an unregistered (key-off) session.
- **Session set and reply-id.** The registry alone defines the session set,
  and an entry is keyed by its reply-id (`wsmailbox.ReplyID`). Listing does
  not depend on any store, so a session with a snapshot but no board is
  listed.
  - The daemon maps a reply-id to its session key by scanning the key
    store's file names. It rescans when the directory changes, or after 5
    seconds on a miss.
  - The key stays inside the daemon process. No `Session` field carries it.
- **For the live-snapshot sibling.** Place the per-session snapshot and ring
  under `wsdashboard.SessionDir(dir, replyID)`, beside `registration.json`.
  `dir` is `wsdashboard.Dir("")`. After each write, call
  `s.dashboardNudge(sessionKey)`; it is a no-op for a key-off session.
  - Name the snapshot's updated-at field so the core can age snapshots by it.
  - `DashboardSessionSource.ReadSession` currently fingerprints only the
    session record. Phase 3, or whichever of the two lands second, adds the
    snapshot and ring to `SessionData`.

Verification:
- `go test ./... -count=1` in `agents-plugin-tool`: all packages ok.
- `go test ./internal/wsdashboard -race -count=3`: ok.
- `go test ./internal/mcp -race -run TestDashboard`: ok.
- `go vet` on the changed packages for linux and windows, and a darwin
  `go build ./...`: ok.
- Mutation checks: queueing nudges for unregistered sessions fails the key-off
  tests, and removing the idle cutoff fails the heartbeat test.

Tests:
- `internal/wsdashboard/index_test.go`:
  - a registration round trip with unknown fields ignored, and a path-like or
    mismatched reply-id refused;
  - the live rule;
  - pruning;
  - Refresh adding sessions, listing one with no board, ageing liveness with
    the clock, and dropping a removed registration, a removed root, and a
    gone record;
  - a nudge re-reading exactly one session, with malformed, unknown, and
    project-mismatched nudges reading and changing nothing;
  - the polling backstop picking up a write that sent no nudge, using a 20 ms
    interval;
  - the `/nudge` route: 204, 404, 400, unknown fields, the Host guard, 405,
    and no index.
- `internal/mcp/dashboard_sessions_test.go`:
  - key-on registration with no session key in the file;
  - a child session not registered;
  - one key-on and one key-off server on one root, where only the key-on
    session is registered, nudged, and indexed (through the real source,
    with no board), and its board write is seen on the next nudge;
  - `todo.add` returning the same result with no daemon, a 500 daemon, and a
    hung daemon, without waiting on the nudge;
  - a session-scope tune off unregistering the session and stopping nudges;
  - heartbeat, idle cutoff, resume, and the ended mark;
  - the first registration pruning expired entries.
- Also fixed: a Phase 1 test flake. The "wrong token" case equalled the real
  token whenever its last hex digit was 0.

Not exercised: the lead's hand check that two sessions in two projects both
appear in one running daemon's index. Look for `dashboard: session <reply-id>
added (project <path>)` lines in `<cache root>/dashboard/dashboard.log`.
Because dev builds share one version, stop a running dev daemon by token
first, so that the rebuilt daemon is the one serving.

Lite review: no Critical or Important findings. One Minor was fixed: the
manifest description had overstated how fast a per-session change applies
(8af00f482). Two Minors were left:
- The fingerprint covers only the session record, so a panel-store nudge (for
  example from todo or mailbox) does not bump `Rev`. Phase 3 needs to widen
  it.
- One session key used against two ws-mcp processes whose
  `dashboard.enabled` resolves differently would make the registration
  flap. A lead's key normally stays on one package's server.

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

### Result (2f1ea3cc3) - 2026-11-06

Landed:
- `internal/wswhiteboard` (47ad75d42): `RenderRegions` renders the board and
  panels fragments from the static page's own templates. `PageAssets` exposes
  the shared CSS, sprite, form script, and mermaid init. `View.FileText` and
  `PanelRow.Path` render file targets as path text.
- `internal/mcp` (0426fcd3d): `DashboardSessionSource.ReadPage`
  (`wsdashboard.PageSource`) returns the served board and panels, the
  resolved mail contacts, and the mailbox history bodies. The
  `ReadSession` fingerprint is widened. The `full:` pointer line is added,
  and the whiteboard playbook is updated in the ws, wsflow, and Pi mirrors.
- `internal/wsdashboard` (75ff0c2bc, 3cd05ff28, e4eef7e71):
  - `pageview.go`: pure view-model builders.
  - `pages.go`: routes and region rendering.
  - `sse.go`: the broker and streams.
  - `assets/`: page and region templates, CSS, the view-state script, and
    sprite icons.
  - The routes are `/`, `/events`, `/s/<reply-id>`, `/s/<reply-id>/events`,
    and `/s/<mermaid asset name>`. All are GET/HEAD only (streams GET only)
    and sit behind the loopback Host guard, with no CORS.
- The ws-mcp manual's "Dashboard daemon" section now names the pages and the
  pointer line (be5c346de).

Settled:
- **Pointer line ("daemon is up").**
  - ws-mcp records each session's last nudge outcome: a 204 stores the
    endpoint port, and any error or a missing endpoint clears it.
    Unregistering also clears it.
  - `whiteboard.update` prints `full: http://127.0.0.1:<port>/s/<reply-id>`
    only when the session is registered and the port is set. Otherwise it
    prints the page path.
  - The update path makes no network call, so it never blocks or fails.
  - Accepted lag: the first update after the daemon dies may still name the
    URL once, and an update before any nudge has returned names the path.
- **SSE region mapping.**
  - Every region element carries `data-region` and `data-h`, a short sha256
    of its HTML. The page opens `/s/<id>/events?h=name.hash,...` with the
    hashes it holds. The stream sends `event: region {name, hash, html}` only
    for regions whose hash differs, then remembers what it sent.
  - One broker wakes every stream on any index `OnChange`, because a peer's
    or a delegate's change shows in another session's tree.
  - Each stream re-renders at most every 250 ms, and every 15 s even without
    a wake, because liveness windows follow the clock. A keep-alive comment
    goes out every 25 s.
  - Times render as fixed UTC text plus epoch ms, and the page script formats
    them, so an unchanged state hashes the same.
  - A `mode` pseudo-region (live/board) tells the page that its layout no
    longer fits. The page reloads unless the composer or the board answers
    hold input; then it shows a banner.
  - `event: gone` fires when the session leaves the index.
  - The composer sits outside every region and is never swapped.
- **`/` ended sessions.** Live sessions (the Phase 2 registration rule) are
  listed. Ended sessions are listed under "ended" for 24 h after their last
  heartbeat or end mark. Older ones are only counted. A project with nothing
  listed is skipped.
- **Board images.** A served board renders an image as its alt text followed
  by `<code class="fpath">` holding the repo-relative path. A non-image file
  link renders as its label plus the path, or the path alone when the label
  equals it. There is no `file:` URL and no relative href or src.
- **Board-first page.** It is chosen when `wslive` adapter presence fails (no
  snapshot, no adapter, or a stale one). The page has a head, the board, and
  the panels only: no tree, conversation, composer, or layout control.
- **Peers.**
  - Contacts sharing a reply-id merge, keep the latest exchange, and are
    labelled by the named address.
  - A served peer's liveness comes from its registration. An unserved named
    peer follows presence, and its reply-id resolves through the presence
    Owner at read time, so a rebind follows the new holder. An unserved `id:`
    peer is live while its reply entry is seen within 10 minutes.
  - All live peers are shown, plus the last 5 ended; older ones are counted.
  - A served peer with an adapter shows its lead batches and agents one level
    deep inside a collapsed `<details data-peer>`, whose open state is saved
    in localStorage.
- **Served mailbox panel.** The daemon holds no mailbox identity, so the
  served panel shows the `id:<reply-id>` address and its unread count, never
  a named inbox.
- **Fingerprint.** It adds the served panels (when a board exists), the
  history entry names, and each contact's address, reply-id, and last
  exchange. PresenceLive stays out because it changes with the clock alone.
  This resolves the Phase 2 minor.

Verification:
- `go test ./... -count=1` in `agents-plugin-tool`: all packages ok.
- `go test ./internal/wsdashboard -race`: ok.
- `go vet` for linux, windows, and darwin on wsdashboard, mcp, wswhiteboard,
  and cmd/ws-mcp: ok.
- The python unittest suites for `agents-plugin` and `agents-plugin-wsflow`
  were run by the mcp leaf: ok.
- `node --check` on the page script: ok.

Tests:
- `internal/wsdashboard/pages_test.go` covers:
  - the live page's regions, origin tags, the escaped streamed reply, mail
    expansion (and first line only once out of history), and the served peer
    link;
  - no history key, project path, or `file:` on the page;
  - a board-first page with no gear, composer, or tabs;
  - the `/` listing;
  - 404 for an unknown, invalid, or unreadable session;
  - the trailing-slash redirect;
  - HEAD refused on streams;
  - the mermaid asset served beside the page;
  - the Host guard on every page route;
  - an SSE client receiving only `head,board` after a nudge, then
    `event: gone`;
  - the mode event when an adapter appears;
  - region hashes that do not change with render time;
  - the rendered permission wait and the `/` ended section;
  - the script and CSS hooks.
- `internal/wsdashboard/pageview_test.go` covers:
  - the adapter-presence mode;
  - agent nesting and delegate links;
  - peer merge, rules, cap, and order;
  - one-level peer trees;
  - conversation mail bodies and live state;
  - no live state on the board-first page;
  - the index ended rule.
- `internal/wswhiteboard/regions_test.go` covers:
  - regions matching the full page;
  - the mermaid flag;
  - FileText path text, including images, absolute paths, and `file:`
    autolinks;
  - panel row paths.
- `internal/mcp/dashboard_page_test.go` covers:
  - served board and panels that are substrings of the static page;
  - path text for links, images, and ticket rows;
  - no session key in the page content;
  - contacts merging, liveness, and rebind;
  - mail bodies through a real `mailbox.send`;
  - the fingerprint covering the page stores;
  - the pointer line following the last nudge (URL / path / key-off).

Platforms: developed and tested on darwin. Linux and windows were checked by
`go vet` only, and no daemon was started there.

Hand checks for the lead (no headless browser here). Open `/s/<id>` for a
session with a live adapter, and for one without:
- A region update keeps typed composer text, open `<details>` (agents, mail
  lines, peers, panels), scroll positions, and in-progress board answers.
- The conversation stays pinned to the bottom unless scrolled up.
- Tabs and the hidden-tab dot: on narrow screens, and in "Tabs".
- Gear layout switch (Automatic, Side by side, Tabs): it persists across
  reload, and the tree is capped at about 38% of the height.
- An expanded peer stays expanded across reload.
- Copy (button or Ctrl/Cmd+Enter) yields the composer text, a blank line,
  then the board answer line.
- The live indicator goes "offline" when the daemon stops and recovers. The
  banner appears when a session leaves the index.
- The board-first page shows a single column with no gear.
- `/` looks like the mockup, in light and dark.
- `full:` prints the URL once a nudge succeeds.

Lite review: no Critical or Important findings.
- Fixed: the trailing-slash URL broke the relative mermaid asset (now a
  redirect); HEAD held a stream open (streams are now GET only); the
  permission wait and the `/` ended section were pinned only at the view
  level.
- Left as minors:
  - Every `/` render, and every index SSE render, does a full board read per
    session to get HasBoard and the counts. A counts-only read would scale
    better with many sessions and open tabs.
  - There is no CSP. The lead's Now-slot HTML passes through verbatim by
    design, and on the daemon it runs same-origin with the other sessions'
    pages. That reach is new compared with the static file page, so the
    browser-interaction sibling should weigh it when it adds write routes.
  - The no-session-key check sits on `PageContent` (the only input the
    daemon renders), not on rendered pages or SSE events.
  - Sending from the composer stays with the browser-interaction sibling.

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
