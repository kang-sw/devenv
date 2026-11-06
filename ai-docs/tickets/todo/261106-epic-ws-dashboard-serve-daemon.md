---
title: ws dashboard serve daemon - live multi-session dashboard and subagent monitoring
sage-review-design: completed
related:
  261106-feat-whiteboard-dashboard-panels: prerequisite; the daemon reuses its panel providers and View/templates (171)
  261106-feat-mailbox-history-ring: indirect prerequisite through the panels ticket's mailbox panel
  260513-research-streamable-http-mcp-transport: context; that idea replaces the MCP transport, which this epic leaves on stdio (168)
  260514-epic-ws-web-dashboard-mvp: history; dropped IDE-scale web dashboard (terminal, file explorer) whose daemon lifecycle bugs (f55de2d62, bff07cafb, e6caac0dc) inform this epic's lifecycle decisions
sage-review-design-reviewed: b251eaef33967e28
---

# ws dashboard serve daemon - live multi-session dashboard and subagent monitoring

## Scope

A `dashboard` subcommand of the ws-mcp binary runs one dashboard daemon per
machine environment. It serves every ws session on that machine as a live web page:
the whiteboard board and dashboard panels of each session, a live view of
each session's lead and subagent activity, the session's conversation, and a
composer that sends user messages back to the session. The goal is that a
user on Claude or Pi can follow and drive a session from the dashboard alone,
without looking at the agent window; tool permission prompts stay in the
terminal. (240), (241)

The browser page is an interactive alternative to the static whiteboard page;
every ws-mcp keeps working without it.

Decided in the 2026-11-06 whiteboard discussion, items (168)-(181),
(187)-(197), (200), (201), (208)-(214), (216)-(220), (233), (238)-(245),
and (247)-(258). An uncommitted local HTML mockup informed (243)-(258); it is
not a contract.

## Non-Scope

- Changing the MCP transport. Every ws-mcp stays a stdio MCP server; the
  daemon is a side channel. (168)
- Direct operations from the browser: no interrupting, killing, or steering a
  session or subagent, and no approving or denying tool permission prompts.
  The browser's writes are a user message sent to the session and the
  cancellation of a sent message the session has not received yet. (189),
  (240), (241), (249)
  Rejected: permission approval and turn interrupt from the browser (241 b);
  they would let the LAN token authorize tool execution, a security boundary
  that needs its own design.
- Thinking text and tool result bodies in the live view. (193)
- Browser input through the mailbox. The mailbox stays agent-to-agent; user
  messages reach a session through the host adapters. (208), (214)
- Filesystem watching (fsnotify or equivalent). (181)
- The dropped MVP's IDE scope: browser terminal, file explorer, editor.
- Multi-user access and public deployment hardening beyond the LAN token.

## Child Tickets

Each child is captured later as its own ticket. Parts that fail and verify
differently are phases inside a child rather than children of their own;
the lifecycle work stays isolated as the core child's first phase, because
the dropped MVP's lifecycle bugs came from it. (201)
Rejected: nine single-concern children (199), too granular; three children
with browser interaction folded into the core.

- Planned: daemon core - Phase 1 lifecycle (file lock, endpoint file,
  `/health`, detached auto-spawn, version replacement) (174), (178), (179);
  then the machine root registry, session index, nudge, and polling (169),
  (181); then the HTTP render of `/` and `/s/<reply-id>` with partial SSE
  region updates, the session page layouts, the pointer-line URI, and the
  peer lead group with mail lines in the conversation (170), (180), (239),
  (248), (257), (259)-(263).
- Planned: browser interaction and access - the message endpoint, the
  per-session board-answer inbox with held-message merge and cancel (208),
  (209), (240), (249), (251), LAN opt-in and access token (173), (197), and
  the image route limited to the project root (238). All are where the
  browser gains write or network access, so they are reviewed together.
- Planned: live snapshot and Pi adapter - the ws-mcp intake for adapter
  events, the per-session snapshot file (188), (191), (195), (217), (247),
  the conversation history ring with its `mail` origin tag (258), (260),
  and the Pi adapter producing them; a
  phase for the Pi side of message submission (drain and inject) (213).
- Planned: Claude adapter - Phase 1 is the event probe (196), (218), (241)
  and runs before any other child's work except the core child's Phase 1
  lifecycle, which may run alongside it (266); later phases build the adapter on
  its result, including the Claude side of message submission (drain and
  inject) (213).

## Cross-Child Decisions

**Process and transport**
- **stdio MCP unchanged; the daemon is optional.** MCP behavior never depends
  on the daemon. When the daemon is down, every whiteboard and dashboard
  feature works through the static page, and the pointer line prints the file
  path. (168), (180)
- **The static page and the embed stay.** The daemon renders the same `View`
  and templates as the static page and pushes updates to the browser over
  SSE. (170)
- **The static dashboard ticket goes first.** The daemon reuses the panel
  providers of `261106-feat-whiteboard-dashboard-panels` rather than
  defining its own. (171)

**Data flow**
- **The daemon reads on-disk stores only.** It reads each session's stores
  from disk. A stdio ws-mcp registers its project root in a machine registry
  so the daemon can find the stores. There is no protocol that pushes data
  into the daemon. The daemon's one write target is the board-answer inbox:
  it writes sent messages there, merges into or deletes entries not yet
  drained. (169), (209), (249), (251)
- **Nudge plus polling.** After a store write, ws-mcp sends a fire-and-forget
  `POST /nudge {reply-id, project}`; errors are ignored. The daemon re-reads
  that session. A polling backstop of about 30 seconds catches missed
  nudges. (181)
  Rejected: fsnotify (platform variance).
- **Live activity goes through a per-session snapshot file.** Host adapters
  (the Claude mod and the Pi extension) hand live events to their session's
  ws-mcp. ws-mcp overwrites one per-session live snapshot file, debounced,
  and sends the nudge. Lead phase changes and streamed reply text use a
  short debounce (about 100 ms) so the browser follows them live. The file
  holds the current state only, not an event log, so its size is bounded.
  (188), (191), (233), (250)
  Rejected: forwarding events to the daemon over HTTP. The daemon would hold
  state in memory and lose it when it is replaced (174), and it would break
  the on-disk-only rule (169). Rejected: adapters posting to the daemon
  directly. Discovery, token, and restart handling would then be written
  twice in TypeScript instead of once in ws-mcp. Rejected: carrying the
  state in the nudge body with the file as a backup (233 b), which keeps
  the same state on two paths.
- **Conversation history is a per-session ring.** ws-mcp writes each
  finished conversation message (a user message, a lead reply, a turn
  summary) from adapter events as one file in a per-session ring that keeps
  the last N entries (N a child constant, about 50), the per-entry-file
  structure of the mailbox history ring. The message in progress lives only
  in the snapshot. A page reload or a daemon replacement keeps the recent
  conversation. (258) A mail entry records the key of its mailbox history
  entry (the entry's file name), written by ws-mcp's mailbox send and
  receive path rather than by an adapter event, so a mail line finds its
  body while the history ring still holds it. (283)
  Rejected: no history, only messages received since the page opened (lost
  on reload); an unbounded append log for the session.

**Lifecycle and discovery**
- **One daemon per OS environment.** One daemon serves the machine however
  many ws-mcp processes run. A WSL distro and native Windows each count as
  their own environment. (173), (179)
- **Single instance by OS file lock.** The daemon holds an OS file lock
  (flock / LockFileEx) on a lock file in the machine cache directory. It
  writes an endpoint file with port, pid, version, and token. ws-mcp reads the
  endpoint file and checks `/health`. When the daemon is absent or older,
  ws-mcp spawns it detached; an older daemon is replaced through the
  handoff below.
  (174), (178)
  Rejected: pid files and port-as-lock, which go stale or race.
- **Replacement handoff and version tolerance.** A newer ws-mcp asks the
  older daemon to shut down through a request authenticated with the
  endpoint-file token, waits for the lock to free, then spawns the new
  daemon. The core child's Phase 1 states whether an idle daemon exits and
  how a daemon running from a versioned plugin cache avoids blocking that
  directory's removal on Windows. Readers of the snapshot, the machine
  registry, and the nudge body ignore unknown fields and treat absent ones
  as empty, since a newer daemon reads what older ws-mcp processes still
  write (the e6caac0dc registry history). Phase 1 also covers a hung
  daemon that holds the lock but fails `/health`. Rejected: terminating
  the older daemon by pid, which races pid reuse.
- **Cross-platform.** Discovery and the single-instance lock work the same on
  Linux, macOS, and Windows. (173)

**Network and access**
- **Localhost by default; LAN opt-in.** LAN access is opt-in and requires an
  access token. (173)
- **Localhost guards instead of a token.** Localhost needs no token. In
  localhost mode every route checks the Host header against loopback
  names, which stops DNS rebinding; in LAN mode the access token takes the
  Host check's place. The browser write routes (message, cancel) require
  an Origin equal to the daemon's own origin, which stops cross-site form
  posts; the daemon sends no CORS headers. Machine routes carry no Origin:
  `/nudge` needs no authentication, since it only makes the daemon re-read
  disk, and the handoff's shutdown request requires the endpoint-file
  token. A local process can write the inbox files directly, so a
  localhost credential would guard nothing the browser guards leave open.
  (284) Rejected: a token on localhost too, which puts the credential in the
  pointer-line URI and so in agent transcripts; one-time pairing with an
  owner cookie as 260515 did, a pairing step for the same protection.
- **The LAN view matches the localhost view.** Argument previews and the
  conversation show in LAN mode too; the access token protects them. (197)
  Rejected: hiding argument previews in LAN mode. The same token already
  allows sending user messages (210), which can move a session, so hiding
  read content adds little protection.
- **Images only under the project root.** The daemon serves an image a board
  item references only when the file is inside that session's project root
  and refuses any other path. (238)
  Rejected: serving any referenced path behind the token, which exposes
  local files in LAN mode.

**URLs**
- **Routes.** `/` lists projects and sessions. `/s/<reply-id>` shows one
  session's page. The page is keyed by reply-id, never by session key,
  because a session key is tool authority. (180)
- **Port.** A fixed default port, configurable. When it is taken, the daemon
  falls back to an ephemeral port and records it in the endpoint file. (180)
- **Pointer line.** When the daemon is up, the whiteboard pointer line prints
  the session's URI; otherwise it prints the file path. (180)

**Browser interaction**
- **The browser views all sessions live.** (172)
- **The composer sends a user message.** The input is a general message
  composer, not an answer form. Clicking an option, `ok`/`no`, or an
  acknowledgement on a board card attaches an answer chip for that item to
  the composer (one chip per item ID, a later click replaces it; a chip can
  carry a note). Sending delivers one user message: the typed text plus the
  answer block `(nn) a; (mm) ok`. After sending, cards show a sent mark and
  stay editable; a later answer to the same ID replaces the earlier one.
  (240), (253)
  Rejected: locking cards after sending, which blocks correcting a misclick.
- **Message submission through the host adapters.** (208)
  - The daemon writes each sent message as one file in a per-session
    board-answer inbox on disk, per-entry files like the mailbox history
    ring, outside the mailbox store. (209)
  - The adapters' existing `mailbox wait` child gains a flag to also watch
    that inbox and to report which kind arrived; a new ws-mcp tool drains
    the inbox. (209)
  - The adapter injects a drained message as the user's own message (Claude
    `$.prompt.submit`, Pi `pi.sendUserMessage`) with a short label naming the
    whiteboard, so it carries user authority. (210)
  - A message that arrives while the session's turn runs is held and
    submitted when the turn ends (Claude: the watcher's turn-end flush; Pi:
    at agent end). (212) Held means left undrained in the inbox: the
    adapter drains only at turn end, so merge (251) and cancel (249) act on
    the entry until then. Draining claims an entry atomically (for example
    by rename), so a cancelled entry is never also injected and a claimed
    entry can no longer be merged into or cancelled; the daemon's merge and
    cancel claim the entry the same way, so a rewrite racing the drain
    cannot bring back an injected entry. Because `mailbox wait` only peeks,
    the adapter re-arms it without the inbox flag while an entry is held
    and adds the flag back after the turn-end drain. Rejected: draining on
    arrival and queueing with Pi `followUp`, which empties the inbox before
    merge or cancel can act; reusing the Claude watcher's tool-boundary mail
    flush, which injects mid-turn.
  - A second send while a message is held merges into the held message, so
    one inbox entry starts one turn; for a chip on the same item ID the
    later one wins. (251) The merged text keeps each send as its own
    paragraph, separated by a blank line, and the held bubble draws a thin
    divider between them. (277) Rejected: a plain join, which runs separate
    sends into one paragraph; numbered parts, which are noise to the agent.
    Rejected: queueing each send as its own message; after the first is
    injected the next lands in a running turn and is held again (212).
  - The conversation shows a held message as held, with a cancel control.
    Cancel removes the entry from the inbox and returns its text and chips
    to the composer. It touches only a message the session has not
    received, so it is not session control. (249)

  Rejected: mail to the session's `id:<reply-id>` (172), because the mailbox
  stays agent-to-agent. Rejected: a second wait child per session, which
  doubles the long-running processes each adapter manages; a daemon-to-adapter
  socket, which would need daemon discovery and token handling in TypeScript
  (rejected in (191)). Rejected: an adapter notice without user authority
  (210 b), which makes the user confirm their own answer twice. Rejected:
  injecting into a running turn; Pi steering is one-at-a-time, and a lead
  mid-turn has usually not asked anything yet.
- **Conversation stream.** The session page shows the conversation in time
  order: user messages tagged by origin (whiteboard or terminal), the lead's
  reply text, and turn summary lines. The reply text streams while the lead
  is replying (250). When the lead waits on a tool permission prompt, the
  conversation and the lead bar show that it is waiting for approval in the
  terminal, with the tool and its argument preview; the approval itself
  stays in the terminal. (241) Mail the lead sends or receives shows as a
  line, `→ peer: first line` or `← peer: first line`, linked to the peer's
  page and expanding to the full body from the mailbox history ring; a turn
  that incoming mail starts carries a `mail` origin tag. (260) The history
  ring keeps 10 messages per session, so a mail line whose message has left
  it shows its first line with no expansion. (283)
  Rejected: showing mail in the mailbox panel only (260); copying mail
  bodies into the conversation ring so every line expands (two stores of
  the same body, and unbounded entry size) (283).
  Rejected: showing the reply text only once each message ends (250 b);
  keeping the reply text in the terminal only (241 c).
- **No session control.** The live view offers no control over sessions or
  subagents beyond sending and cancelling the user's own messages. (189),
  (249)

**Live monitoring**
- **Hosts without an adapter.** A host with no adapter, such as Codex, shows
  only the state in its on-disk stores: no lead bar, no conversation, and no
  browser submission; its page keeps only the copy-answers button and uses a
  board-first layout with the board and panels only. The dashboard works
  without the live layer. (190), (211), (219), (252)
  Rejected: falling back to mail for those hosts (211 b), which would put
  user input back into the mailbox.
- **Lead in the snapshot.** The lead is the root entry of the same
  per-session snapshot, not a separate file or path. It carries the same
  recent-batch view as a subagent, plus the current turn's start time, its
  done and failed totals, and the previous turn's duration, which the
  5-batch cap does not let the browser compute. (216), (217), (247)
  Rejected: a lead line with the current step only (216 a).
- **Subagent tree.** The tree is keyed by the host's native agent id. A
  delegate that holds a ws session key is labelled with that session's
  reply-id, linked to its `/s/<reply-id>` page when it has a board; the
  session key itself never reaches the browser. `session.children` sees
  only keyed delegates, so it cannot be the source of the tree. (192), (243)
  Rejected: showing the delegate's session key (192 as first written),
  which exposes tool authority to the browser.
- **Tree retention.** The tree keeps every running agent and the last 5
  finished ones; a failed agent keeps its failed marker. (244)
  Rejected: dropping finished agents at once; keeping every agent for the
  session, which grows the snapshot without bound (191).
- **Live content matches the Pi expanded agent widget.** (193)
  - Per agent, the last 5 batches. A batch is one assistant message and
    its tool calls.
  - The generating phase: thinking, replying, the tool call being
    streamed, or waiting for permission.
  - Tool names, up to 5 per batch.
  - Argument previews, up to 200 characters, on the running call only; a
    finished batch shows tool names and call and failure counts, and a
    call whose arguments are still streaming shows its tool name and
    character count. (245)
  - Done and failed counts, and elapsed time.
  - An optional one-line caption.

  Rejected: session-level state only (193 a); a full activity stream with
  tool results (193 c).
- **One snapshot schema.** The live snapshot carries agent activity in the
  shape of Pi's batch model (`BatchView`, `agents-plugin-pi/src/agent-batches.ts`,
  caps `AGENT_BATCH_CAP` 5, `BATCH_TOOL_NAME_CAP` 5, `BATCH_ARGS_PREVIEW_CAP`
  200). The Claude adapter produces the same shape. (195)
- **Claude adapter probe first.** The Claude adapter child's Phase 1
  probes which events the Claude mod receives, and it runs before any other
  child's work except the core child's Phase 1 lifecycle, which depends on
  no probe result and may run alongside it (266): subagent tool-call and generating-phase events (196), the
  main thread's generating-phase events (218), and the lead's reply text
  and tool-permission waits (241). Verified so far: subagent `turn.start`
  events carry an `agentId`, and the main thread already delivers
  `turn.start`, `turn.step`, `tool.call`, and `turn.complete`. If subagent
  events are unavailable, the Claude live view falls back to session-level
  state (193 a); if main-thread phase events are unavailable, the lead shows
  tool-call granularity only (218). The fallback for missing reply-text or
  permission-wait events is settled with the probe result. (196), (201)
  Rejected: holding the core lifecycle phase until the probe lands (266 b),
  which serializes a phase that reads no snapshot.
- **Peer leads.** The tree's lead root holds one "peer lead" group: the
  sessions this lead exchanges mail with. Each row shows the peer's lead
  phase line from the peer's own snapshot, the last exchange time, a
  `/s/<reply-id>` link, and the peer's subagent count. (259) A peer row is
  collapsed by default; a peer the user expands shows that peer's lead
  batches and subagent tree, one level deep: the peer's own peer group
  never appears. (279) The daemon renders every listed peer's tree inside
  a collapsed `<details>`, and the browser keeps the expanded set in
  localStorage keyed by peer reply-id, so a co-lead stays expanded across
  reloads and days. (280)
  Rejected: recursive peer trees (A and B mailing each other form a cycle,
  and each page would fan in every peer's updates); peers that never show
  a tree (279 replaced it: the user follows a co-lead's subagents); the
  browser reporting its expanded set so the daemon renders only those trees
  (adds browser-to-daemon state for a list capped at live peers plus 5);
  a badge on the mailbox panel only.
- **Peer resolution.** The daemon resolves each mailbox contact to a
  session: an `id:<reply-id>` address resolves directly; a named address
  goes through the mailbox presence record's owner session key and
  `ReplyID()`. Only the reply-id reaches the browser. A peer this daemon
  does not serve shows its address and last exchange time with no live
  state. (261) Contacts that resolve to the same reply-id (an `id:` address
  and a named address of one session) merge into one row with the latest
  exchange time. A named contact resolves through the presence owner at
  render time, so after a rebind the row follows the address's current
  holder, the session that mail to it now reaches.
- **Peer retention.** The group shows every live peer and the last 5 ended
  peers, ordered by last exchange. (262) A peer this daemon does not serve
  counts as live only while its mailbox presence record is alive; otherwise
  it falls under the ended cap. (276) For an `id:<reply-id>` peer, which has
  no presence record, that record is the reply-id registry entry, read the
  way `mailboxPresenceLive` reads named presence: live while its `LastSeen`
  is within 10 minutes. No ticker refreshes that field and it has no pid,
  so for such a peer "live" means a mailbox exchange in the last 10
  minutes, and a quiet one shows as ended; accepted for this rare case
  over a ticker refresh that writes under the machine-wide reply lock on
  every tick. Every running ws-mcp in the daemon's environment
  registers its root (169), so an unserved live peer is the rare case (one
  outside this environment, or an older ws-mcp without the registry); a
  served peer's liveness comes from its own session state in the core
  child's session index. (282) Rejected: always listing unserved peers as
  live, which grows without bound from stale contacts; counting a reply-id
  peer live while its registry entry exists, which the 30-day lazy reap
  (`mailboxReplyIDRetention`) turns into 30 days of a finished peer shown
  live; counting every unserved reply-id peer as ended (282).
- **Peers on hosts without an adapter.** The group works for any host; a
  peer without an adapter, such as Codex, shows no phase. (263)

**Session page**
- **Partial updates.** The page never reloads. The daemon sends each
  changed region (lead bar, subagent tree, conversation, topics, panels) as
  an HTML fragment over SSE, rendered with the same server templates as the
  static page, and the page swaps only that region. The composer is never
  swapped, so typed text and attached chips survive every update. (239)
  Rejected: full reloads, which lose a message being typed and the scroll
  position; JSON plus client-side rendering, which duplicates the templates
  in JavaScript.
- **View state survives swaps.** UI state inside a swapped region (open
  `<details>`, expanded tree nodes, scroll positions) is remembered by the
  page by element key and re-applied after the swap. (255)
  Rejected: rendering open state on the server, which would make the server
  track each browser's view.
- **Layout.** The default layout is side by side: the left column holds the
  subagent tree, the conversation, and the composer; the right column holds
  the board with the panels under it. The page itself does not scroll; each
  column scrolls on its own, and the conversation stays pinned to the
  bottom unless the user scrolls up. On a narrow screen the page switches
  to tabs: a conversation / board switch with the composer pinned to the
  bottom, the tree and panels in a side column, and a marker on the hidden
  tab when its region updates. (248)
  Rejected: tabs as the default, which never shows a question and the
  conversation at once; conversation first with the board in a drawer,
  which hides the board, the primary channel, by default.
- **Layout switch.** A settings control in the top-right corner lets the
  user force either layout. Both layouts arrange the same region elements
  with CSS only (one grid-template-areas rule per layout), and the choice is
  stored in the browser's localStorage. (257) A host without an adapter has
  only the board-first layout, so its page hides the control. (278)
  Rejected: offering the drawer layout as a third mode, which adds its own
  open state, overlay, and update alert to maintain.
- **Lead bar and tree height.** The lead bar stays a one-line status in
  every layout; the lead's recent batches show as the root node of the
  subagent tree. (254) In the side-by-side layout the tree is capped at
  about 38% of the column height and scrolls by itself; a running agent
  shows only its live row and a finished one its header and caption, and a
  click expands its last 5 batches. (256)
  Rejected: an uncapped tree, which pushes the composer off screen when many
  subagents run.
- **Lead phase on the index.** `/` shows each session's lead phase as a
  small badge (thinking, replying, tool, waiting for permission, idle) from
  the same snapshot field. (220), (241)

**Audience**
- **Host-independent.** The dashboard targets every ws host user: Claude,
  Pi, and Codex in a terminal. It goes ahead whether or not another host UI,
  such as Orca, shows similar views. (194)
  This reopens the dashboard topic the user closed on 2026-08-01 (df8336ce,
  `260730-chore-ws-dashboard-drop-sweep`), which dropped an IDE-scale
  dashboard for host UIs such as Orca; (194) answers that reason, and the
  IDE scope stays out (Non-Scope).
- **Daemon entry point.** The daemon is a new `ws-mcp dashboard` subcommand.
  Rejected: a non-stdio flag on `ws-mcp serve`, which is the stdio MCP entry
  point every launcher calls (`cmd/ws-mcp/main.go`); a separate subcommand
  keeps that path untouched.

## Prior Decisions

- 261106-feat-mailbox-history-ring (2026-11-06, Decisions): "One ring per session, both directions, full bodies. Each session keeps the last 10 messages it exchanged, received and sent together." — bearing: constrains
- 261106-feat-whiteboard-dashboard-panels (2026-11-06, Decisions): "Mailbox panel. The session's own address. Unread count... Whether the session is waiting for mail, from the listening marker... Correspondents with their last exchange time" — bearing: supports
- 261022-feat-mailbox-recent-peers-and-correspondents (2026-10-22, Decisions): "Correspondents: reply-ids limited to the caller's own exchange history... only the addresses the calling session (its session key) has actually exchanged mail with" — bearing: supports
- df8336ce (2026-08-01, commit): "user closed the dashboard topic definitively" (recorded on 260802-research-ws-pi-native-framework: "Dashboard — closed topic, not absorbed"); the Audience section names this reopening and answers it with (194) — bearing: constrains
- 260730-chore-ws-dashboard-drop-sweep (2026-08-01, commit 41157331): "recovery remains archive/ws-dashboard; no dashboard artifact is retained in the live docs tree." — bearing: constrains
- 261028-feat-pi-agent-widget-batch-row-redesign (2026-10-28, commit 1b6079a5): "The generating state lives on AgentBatchBuffer.generating, outside the capped batches list, and batchViews appends its view last with the seq the batch would take" — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "Refresh is event-driven: usage on session.measure, model and effort on main-loop turn.step, git on tool.call (2 s throttle) and turn.complete" — bearing: supports
- 260515-ws-web-daemon-foundation (2026-05-15, commit 03442cdf): "made the dashboard daemon shell caller-visible: `ws-dashboard serve`, loopback default, one-time pairing, owner session cookie, gated health/UI/fallback routes" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-batches.ts is the only path named; the children also reach agents-plugin-tool/cmd/ws-mcp, agents-plugin-tool/internal/mcp, agents-plugin-tool/internal/wsmailbox, agents-plugin-tool/internal/wswhiteboard, agents-plugin/hooks/claude-mod, and the Pi adapter |
| scope.surface | cross-module | new ws-mcp dashboard subcommand, HTTP routes /, /s/reply-id, /health, /nudge, a new inbox-drain MCP tool, a new mailbox wait flag, adapter injection paths |
| scope.new_public_symbol | yes | ws-mcp dashboard subcommand, /health and /nudge routes, inbox-drain MCP tool, mailbox wait inbox flag |
| scope.new_type_contract | yes | endpoint file, machine root registry, per-session live snapshot with lead root and peer group, board-answer inbox entry with merged held text, conversation ring entry with mail origin tag, nudge body |
| scope.test_surface | new-files | no daemon code or tests exist; existing agents-plugin-tool/internal/mcp/whiteboard_panels_test.go, mailbox_history_test.go, mailbox_correspondents_test.go, agents-plugin-pi/test/agent-batches.test.ts would extend |
| complexity.reuse_points | confirmed | wswhiteboard.View at agents-plugin-tool/internal/wswhiteboard/view.go#L20, panel providers in agents-plugin-tool/internal/mcp/whiteboard_panels.go, mailbox history ring with direction and full body at agents-plugin-tool/internal/mcp/mailbox_history.go#L36-L59, Presence.Owner at agents-plugin-tool/internal/wsmailbox/store.go#L65-L76, ReplyID and ReplyEntry.Contacts at agents-plugin-tool/internal/wsmailbox/replyid.go#L26-L57, mailboxPresenceLive at agents-plugin-tool/internal/mcp/mailbox_runtime.go#L164-L173, BatchView caps at agents-plugin-pi/src/agent-batches.ts#L34-L40, ws-mcp mailbox wait in agents-plugin-tool/cmd/ws-mcp/mailbox.go |
| complexity.side_effect_risk | high | detached process spawn, OS file locks, a network listener, and injection of messages with user authority into live sessions |
| risk.correctness | high | daemon lifecycle and replacement caused the dropped MVP's bugs f55de2d62, bff07cafb, e6caac0dc; held-message merge and cancel race against the adapter drain; peer resolution and liveness read mailbox stores other sessions write |
| risk.fit | moderate | the dashboard topic was closed on 2026-08-01 per df8336ce; ws-mcp serve is the stdio MCP entry at agents-plugin-tool/cmd/ws-mcp/main.go#L80-L105; both prerequisite tickets are in .done |
| risk.test | high | cross-platform flock and LockFileEx, SSE region swaps and localStorage state in a browser, and the Claude adapter event probe need live hosts |
| risk.security_or_contract | high | LAN access token, user-authority message injection, project-root containment of the image route, and keeping session keys out of peer rows |

## Completion Criteria

- Done: every child is done; on Claude and Pi, a browser at `/` shows each
  running session live with its lead and subagent activity and its
  conversation, and a message sent from the composer reaches the session as
  user input. (200), (210), (240), (241)
- Dropped: the user drops the direction. A failed adapter probe is not a
  drop trigger, because (190) and (196) define the fallback. (200)
- Deferred: whatever a child's result defers.

## Sage Review Round 1 (2026-11-06)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Localhost authentication on the user-authority write path is undecided | important | missing |
| 2 | Unserved reply-id peers stay live for up to 30 days | important | missing |
| 3 | Pi followUp hold conflicts with merge and cancel on an inbox entry | important | autonomous |
| 4 | Daemon exit, replacement handoff, and cross-version tolerance are unstated | minor | autonomous |
| 5 | No join key from a conversation mail line to its mailbox history entry | minor | autonomous |
| 6 | Peer rows need dedupe and handling of rebinding | minor | autonomous |
