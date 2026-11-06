---
title: ws dashboard serve daemon - live multi-session dashboard and subagent monitoring
sage-review-design: required
related:
  261106-feat-whiteboard-dashboard-panels: prerequisite; the daemon reuses its panel providers and View/templates (171)
  261106-feat-mailbox-history-ring: indirect prerequisite through the panels ticket's mailbox panel
  260513-research-streamable-http-mcp-transport: context; that idea replaces the MCP transport, which this epic leaves on stdio (168)
  260514-epic-ws-web-dashboard-mvp: history; dropped IDE-scale web dashboard (terminal, file explorer) whose daemon lifecycle bugs (f55de2d62, bff07cafb, e6caac0dc) inform this epic's lifecycle decisions
---

# ws dashboard serve daemon - live multi-session dashboard and subagent monitoring

## Scope

A `--serve` mode of the ws-mcp binary runs one dashboard daemon per machine
environment. It serves every ws session on that machine as a live web page:
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
  region updates, the session page layouts, and the pointer-line URI (170),
  (180), (239), (248), (257).
- Planned: browser interaction and access - the message endpoint, the
  per-session board-answer inbox with held-message merge and cancel (208),
  (209), (240), (249), (251), LAN opt-in and access token (173), (197), and
  the image route limited to the project root (238). All are where the
  browser gains write or network access, so they are reviewed together.
- Planned: live snapshot and Pi adapter - the ws-mcp intake for adapter
  events, the per-session snapshot file (188), (191), (195), (217), (247),
  the conversation history ring (258), and the Pi adapter producing them; a
  phase for the Pi side of message submission (drain and inject) (213).
- Planned: Claude adapter - Phase 1 is the event probe (196), (218), (241)
  and runs before any other child's work; later phases build the adapter on
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
  conversation. (258)
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
  ws-mcp spawns it detached, and the new daemon replaces the older one.
  (174), (178)
  Rejected: pid files and port-as-lock, which go stale or race.
- **Cross-platform.** Discovery and the single-instance lock work the same on
  Linux, macOS, and Windows. (173)

**Network and access**
- **Localhost by default; LAN opt-in.** LAN access is opt-in and requires an
  access token. (173)
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
    `followUp`). (212)
  - A second send while a message is held merges into the held message, so
    one inbox entry starts one turn; for a chip on the same item ID the
    later one wins. (251)
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
  stays in the terminal. (241)
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
  child's work: subagent tool-call and generating-phase events (196), the
  main thread's generating-phase events (218), and the lead's reply text
  and tool-permission waits (241). Verified so far: subagent `turn.start`
  events carry an `agentId`, and the main thread already delivers
  `turn.start`, `turn.step`, `tool.call`, and `turn.complete`. If subagent
  events are unavailable, the Claude live view falls back to session-level
  state (193 a); if main-thread phase events are unavailable, the lead shows
  tool-call granularity only (218). The fallback for missing reply-text or
  permission-wait events is settled with the probe result. (196), (201)

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
  stored in the browser's localStorage. (257)
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

## Completion Criteria

- Done: every child is done; on Claude and Pi, a browser at `/` shows each
  running session live with its lead and subagent activity and its
  conversation, and a message sent from the composer reaches the session as
  user input. (200), (210), (240), (241)
- Dropped: the user drops the direction. A failed adapter probe is not a
  drop trigger, because (190) and (196) define the fallback. (200)
- Deferred: whatever a child's result defers.
