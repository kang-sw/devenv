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
each session's turn and subagent activity, and answer submission back to the
session. The browser page is an interactive alternative to the static
whiteboard page; every ws-mcp keeps working without it.

Decided in the 2026-11-06 whiteboard discussion, items (168)-(181) and
(187)-(197).

## Non-Scope

- Changing the MCP transport. Every ws-mcp stays a stdio MCP server; the
  daemon is a side channel. (168)
- Direct operations from the browser: no interrupting, killing, or steering a
  session or subagent. The browser's only write is answer submission. (172),
  (189)
- Thinking text and tool result bodies in the live view. (193)
- Filesystem watching (fsnotify or equivalent). (181)
- The dropped MVP's IDE scope: browser terminal, file explorer, editor.
- Multi-user access and public deployment hardening beyond the LAN token.

## Child Tickets

Each child is captured later as its own ticket, in this order. The split
keeps lifecycle, rendering, network access, and each adapter separate,
because they fail and verify separately; the lifecycle child is isolated
because the dropped MVP's lifecycle bugs came from it. (199)
Rejected: one large child.

- Planned: Claude mod event probe - whether subagent tool-call and
  generating-phase events reach the mod (196).
- Planned: daemon foundation - file lock, endpoint file, `/health`, detached
  auto-spawn, version replacement (174), (178), (179).
- Planned: machine root registry and the daemon's session index, nudge, and
  polling (169), (181).
- Planned: HTTP render of `/` and `/s/<reply-id>` with SSE, and the
  pointer-line URI (170), (180).
- Planned: browser answer submission by mail (172).
- Planned: LAN opt-in and access token (173), (197).
- Planned: live snapshot - the ws-mcp intake for adapter events and the
  per-session snapshot file (188), (191), (195).
- Planned: Pi adapter.
- Planned: Claude adapter.

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
  into the daemon. (169)
- **Nudge plus polling.** After a store write, ws-mcp sends a fire-and-forget
  `POST /nudge {reply-id, project}`; errors are ignored. The daemon re-reads
  that session. A polling backstop of about 30 seconds catches missed
  nudges. (181)
  Rejected: fsnotify (platform variance).
- **Live activity goes through a per-session snapshot file.** Host adapters
  (the Claude mod and the Pi extension) hand live events to their session's
  ws-mcp. ws-mcp overwrites one per-session live snapshot file, debounced,
  and sends the nudge. The file holds the current state only, not an event
  log, so its size is bounded. (188), (191)
  Rejected: forwarding events to the daemon over HTTP. The daemon would hold
  state in memory and lose it when it is replaced (174), and it would break
  the on-disk-only rule (169). Rejected: adapters posting to the daemon
  directly. Discovery, token, and restart handling would then be written
  twice in TypeScript instead of once in ws-mcp.

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
- **The LAN view matches the localhost view.** Argument previews show in LAN
  mode too; the access token protects them. (197)
  Rejected: hiding argument previews in LAN mode. The same token already
  allows answer submission, which can move a session, so hiding read content
  adds little protection.

**URLs**
- **Routes.** `/` lists projects and sessions. `/s/<reply-id>` shows one
  session's board. The page is keyed by reply-id, never by session key,
  because a session key is tool authority. (180)
- **Port.** A fixed default port, configurable. When it is taken, the daemon
  falls back to an ephemeral port and records it in the endpoint file. (180)
- **Pointer line.** When the daemon is up, the whiteboard pointer line prints
  the session's URI; otherwise it prints the file path. (180)

**Browser interaction**
- **The browser views all sessions live.** (172)
- **Answer submission by mail.** The browser submits whiteboard answers as
  mail to the session's own address `id:<reply-id>`. The board reply-id and
  the mailbox reply-id are the same value. Delivery wakes the session's turn
  through the existing mailbox push. (172)
- **Read-only monitoring.** The live view offers no control over sessions or
  subagents. (189)

**Live monitoring**
- **Hosts without an adapter.** A host with no adapter, such as Codex, shows
  only the state in its on-disk stores. The dashboard works without the live
  layer. (190)
- **Subagent tree.** The tree is keyed by the host's native agent id. A
  delegate that holds a ws session key gets that key attached. `session.children`
  sees only keyed delegates, so it cannot be the source of the tree. (192)
- **Live content matches the Pi expanded agent widget.** (193)
  - Per subagent, the last 5 batches. A batch is one assistant message and
    its tool calls.
  - The generating phase: thinking, replying, or the tool call being
    streamed.
  - Tool names, up to 5 per batch.
  - Argument previews, up to 200 characters.
  - Done and failed counts, and elapsed time.
  - An optional one-line caption.

  Rejected: session-level state only (193 a); a full activity stream with
  tool results (193 c).
- **One snapshot schema.** The live snapshot carries subagent activity in
  the shape of Pi's batch model (`BatchView`, `agents-plugin-pi/src/agent-batches.ts`,
  caps `AGENT_BATCH_CAP` 5, `BATCH_TOOL_NAME_CAP` 5, `BATCH_ARGS_PREVIEW_CAP`
  200). The Claude adapter produces the same shape. (195)
- **Claude adapter probe first.** The first child probes whether the Claude
  mod receives subagent tool-call and generating-phase events. Verified so
  far: subagent `turn.start` events carry an `agentId`. If the events are
  unavailable, the Claude live view falls back to session-level state
  (193 a). (196)

**Audience**
- **Host-independent.** The dashboard targets every ws host user: Claude,
  Pi, and Codex in a terminal. It goes ahead whether or not another host UI,
  such as Orca, shows similar views. (194)

## Completion Criteria

- Done: every child is done; on Claude and Pi, a browser at `/` shows each
  running session live with its subagent activity, and a submitted answer
  reaches the session. (200)
- Dropped: the user drops the direction. A failed adapter probe is not a
  drop trigger, because (190) and (196) define the fallback. (200)
- Deferred: whatever a child's result defers.
