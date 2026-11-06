---
title: ws dashboard live snapshot and Pi adapter - adapter event intake, snapshot file, conversation ring, Pi producer and submission
parent: 261106-epic-ws-dashboard-serve-daemon
related:
  261106-feat-ws-dashboard-claude-adapter: prerequisite through its Phase 1 event probe only, which runs before any work here (266); its later phases feed this ticket's Phase 1 intake and produce the same snapshot schema
  261106-feat-ws-dashboard-daemon-core: its Phase 3 renders this ticket's snapshot and ring; Phase 1 here sends the nudge its Phase 2 adds, and whichever of the two lands second wires that call
  261106-feat-ws-dashboard-browser-interaction: prerequisite for Phase 3; the Pi drain and inject uses its board-answer inbox, drain tool, and `mailbox wait` inbox flag
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1e76933f5ae0cadd
sage-review-completeness-reviewed: 1e76933f5ae0cadd
---

# ws dashboard live snapshot and Pi adapter - adapter event intake, snapshot file, conversation ring, Pi producer and submission

## Background

The parent epic `261106-epic-ws-dashboard-serve-daemon` shows each session's
lead and subagent activity and its conversation live in the browser. The
daemon reads disk only, so live state reaches it through files ws-mcp
writes: a per-session live snapshot (current state only) and a per-session
conversation ring (recent finished messages). This child defines both, the
ws-mcp intake that host adapters feed, and the Pi adapter that feeds it,
plus the Pi side of browser message submission (213). The Claude adapter
feeds the same intake in `261106-feat-ws-dashboard-claude-adapter`; the
daemon core renders both files. Read the epic's Cross-Child Decisions for
shared context; the decisions below bind this child.

Phase 1 is the Go side alone so the Claude adapter's later phases can build
on it without waiting for the Pi producer.

## Decisions

**Snapshot**
- **Live activity goes through a per-session snapshot file.** Host adapters
  (the Claude mod and the Pi extension) hand live events to their session's
  ws-mcp. ws-mcp overwrites one per-session live snapshot file, debounced,
  and sends the nudge. Lead phase changes and streamed reply text use a short
  debounce (about 100 ms) so the browser follows them live. The file holds
  the current state only, not an event log, so its size is bounded. (188),
  (191), (233), (250) Rejected: forwarding events to the daemon over HTTP.
  The daemon would hold state in memory and lose it when it is replaced
  (174), and it would break the on-disk-only rule (169). Rejected: adapters
  posting to the daemon directly. Discovery, token, and restart handling
  would then be written twice in TypeScript instead of once in ws-mcp.
  Rejected: carrying the state in the nudge body with the file as a backup
  (233 b), which keeps the same state on two paths.
- **One snapshot schema.** The snapshot carries agent activity in the shape
  of Pi's batch model (`BatchView`, `agents-plugin-pi/src/agent-batches.ts`,
  caps `AGENT_BATCH_CAP` 5, `BATCH_TOOL_NAME_CAP` 5, `BATCH_ARGS_PREVIEW_CAP`
  200). The Claude adapter produces the same shape. (195)
- **Lead in the snapshot.** The lead is the root entry of the same
  snapshot, not a separate file or path. It carries the same recent-batch
  view as a subagent, plus the current turn's start time, its done and
  failed totals, and the previous turn's duration, which the 5-batch cap
  does not let the browser compute. (216), (217), (247) Rejected: a lead
  line with the current step only (216 a).
- **Live content matches the Pi expanded agent widget.** Per agent: the last
  5 batches (a batch is one assistant message and its tool calls); the
  generating phase (thinking, replying, the tool call being streamed, or
  waiting for permission); tool names, up to 5 per batch; argument previews,
  up to 200 characters, on the running call only, while a finished batch
  shows tool names and call and failure counts, and a call whose arguments
  are still streaming shows its tool name and character count (245); done
  and failed counts, and elapsed time; an optional one-line caption. (193)
  Rejected: session-level state only (193 a); a full activity stream with
  tool results (193 c). Thinking text and tool result bodies stay out
  (epic Non-Scope).
- **Subagent tree.** The tree is keyed by the host's native agent id. A
  delegate that holds a ws session key is labelled with that session's
  reply-id; the session key itself never reaches the browser.
  `session.children` sees only keyed delegates, so it cannot be the source of
  the tree. (192), (243) Rejected: showing the delegate's session key, which
  exposes tool authority to the browser.
- **Tree retention.** The tree keeps every running agent and the last 5
  finished ones; a failed agent keeps its failed marker. (244) Rejected:
  dropping finished agents at once; keeping every agent for the session,
  which grows the snapshot without bound (191).
- **Permission waits.** When the lead waits on a tool permission prompt,
  the snapshot carries that state with the tool and its argument preview;
  the approval itself stays in the terminal. (241)
- **Version tolerance.** Readers of the snapshot and the nudge body ignore
  unknown fields and treat absent ones as empty, since a newer daemon reads
  what older ws-mcp processes still write.
- **The intake is an adapter-only MCP tool.** Adapters hand events to
  ws-mcp through an MCP tool whose short description marks it adapter-only,
  with compact text output. (289) Rejected: a channel hidden from the model,
  which needs its own discovery while both adapters already call ws MCP
  tools (Pi through its MCP client, the Claude mod through `$.tool.call`).
  Cost accepted: one tool entry in `tools/list` on every host.
- **Dashboard key off writes nothing.** With the session's dashboard key
  off (the core's `dashboard.` key, added by its Phase 1, which lands first
  in wave W0), ws-mcp writes no snapshot and no conversation-ring entries.
  (design review) Rejected: accepting unread ring writes in wsflow and for
  users who turned the dashboard off.
- **Nudge wiring.** Snapshot and ring writes call the nudge sender of
  `261106-feat-ws-dashboard-daemon-core` Phase 2; whichever of the two
  phases lands second wires the call. (295) Rejected: serializing them.

**Phasing**
- **Three phases: Go intake, Pi producer, Pi submission.** (294) The Claude
  adapter's feed phase waits only on the Go phase. Rejected: one Pi phase
  covering both the Go side and the Pi side, which would hold the Claude
  feed behind Pi work.

**Adapter knob**
- **A Pi adapter knob gates the dashboard feed and submission.** It is a
  `pi.` knob in the Pi package's `config-manifest.json`, read through
  `adapter-config.ts`, default on. The Pi package ships in one distribution,
  so there is no wsflow default to set. (291) Rejected: no knob, which
  breaks the convention that every adapter feature is tunable.

**Conversation ring**
- **Conversation history is a per-session ring.** ws-mcp writes each
  finished conversation message (a user message, a lead reply, a turn
  summary) from adapter events as one file in a per-session ring that keeps
  the last N entries (N a child constant, about 50), the per-entry-file
  structure of the mailbox history ring. The message in progress lives only
  in the snapshot. A page reload or a daemon replacement keeps the recent
  conversation. (258) Rejected: no history, only messages received since the
  page opened (lost on reload); an unbounded append log for the session.
- **Origin tags.** User messages are tagged by origin, whiteboard or
  terminal, and a turn that incoming mail starts carries a `mail` origin
  tag. (260)
- **Mail entries.** While the session's dashboard key is on, mail the lead
  sends or receives is a ring entry written by ws-mcp's mailbox send and
  receive path rather than by an adapter event.
  It records the key of its mailbox history entry (the entry's file name),
  so a mail line finds its body while the history ring still holds it.
  (283) The history ring keeps 10 messages, so the line must still show its
  direction, peer, and first line after the body has left it. (260), (283)
  Rejected: copying mail bodies into the conversation ring so every line
  expands (two stores of the same body, and unbounded entry size).

**Pi submission**
- **Injection with user authority.** The adapter injects a drained message
  as the user's own message (`pi.sendUserMessage`) with a short label naming
  the whiteboard, so it carries user authority. (210) Rejected: an adapter
  notice without user authority (210 b), which makes the user confirm their
  own answer twice; injecting into a running turn, since Pi steering is
  one-at-a-time and a lead mid-turn has usually not asked anything yet.
- **Hold until agent end.** A message that arrives while the turn runs is
  left undrained in the inbox and submitted at agent end, so merge and
  cancel act on it until then. Because `mailbox wait` only peeks, the
  adapter re-arms it without the inbox flag while an entry is held and adds
  the flag back after the turn-end drain. (212) Rejected: draining on
  arrival and queueing with Pi `followUp`, which empties the inbox before
  merge or cancel can act.

**Ordering**
- **Claude probe first.** This child's work starts after the Claude adapter
  child's Phase 1 probe, so the schema is fixed knowing which Claude events
  exist. (266)

## Constraints

- The new intake tool is listed in the `runtime.json` of `agents-plugin/`, `agents-plugin-wsflow/`, and `agents-plugin-pi/`, as `mailbox.history` is; the wsflow capability test requires an exact match.
- Shipped surface: ws-mcp strings and the Pi package (`agents-plugin-pi/`)
  ship downstream; none may name this repository's tickets, commits, or
  layout.
- MCP behavior never depends on the daemon: snapshot and ring writes and
  their nudge never fail or delay an adapter's call or a mailbox tool. (168)
- The snapshot file stays bounded by the caps above; the ring stays bounded
  by N. (191), (258)
- Out of scope: rendering (core child); the inbox, drain tool, and wait flag
  themselves (browser-interaction child); Claude event production (Claude
  adapter child).
- Reuse points: `agents-plugin-pi/src/agent-batches.ts` (`BatchView`, caps,
  generating state outside the capped batch list),
  `agents-plugin-pi/src/adapter-label.ts`, `agents-plugin-pi/src/mailbox-waiter.ts`,
  `agents-plugin-pi/src/mcp-stdio-client.ts`, the mailbox history ring in
  `agents-plugin-tool/internal/mcp/mailbox_history.go`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "Live activity goes through a per-session snapshot file. Host adapters (the Claude mod and the Pi extension) hand live events to their session's ws-mcp. ws-mcp overwrites one per-session live snapshot file, debounced" — bearing: supports
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "Nudge plus polling. After a store write, ws-mcp sends a fire-and-forget `POST /nudge {reply-id, project}`; errors are ignored." — bearing: constrains
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "stdio MCP unchanged; the daemon is optional. MCP behavior never depends on the daemon." — bearing: constrains
- 7a70197d (2026-11-06, commit): "Dependency order for promotion and waves: Claude probe || core P1; core P2 || live-snapshot P1; core P3 || Claude P2 || Pi P2; browser interaction; Pi P3 || Claude P3." — bearing: constrains
- 7a70197d (2026-11-06, commit): "(291) reading for Pi: the Pi package ships in one distribution, so its pi.* knob defaults on with no wsflow default." — bearing: supports
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Nudge wiring for snapshot and ring writes. The nudge sender lands in Phase 2 here; the snapshot and conversation-ring writes of `261106-feat-ws-dashboard-live-snapshot-pi` Phase 1 call it." — bearing: supports
- 261106-feat-ws-dashboard-claude-adapter (2026-11-06, Decisions): "One snapshot schema. The Claude adapter produces the same snapshot shape as Pi's batch model (`BatchView`, caps 5 batches, 5 tool names, 200 preview characters)." — bearing: constrains
- 144028b0 (2026-11-06, commit): "(258) a adds a per-session conversation history ring (last N, about 50) beside the bounded snapshot (191), so reload and daemon replacement (174) keep recent conversation" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/ new intake tool, snapshot and ring writers, mailbox_history.go send and receive path; agents-plugin-pi/src/ agent-batches.ts, mailbox-waiter.ts, adapter-label.ts, adapter-config.ts, config-manifest.json |
| scope.surface | cross-module | new adapter-only MCP tool in tools/list, on-disk snapshot and ring files read by the daemon child, Pi extension and ws-mcp both change |
| scope.new_public_symbol | yes | adapter-only intake MCP tool, name settled by the worker; new pi. dashboard knob in agents-plugin-pi/config-manifest.json |
| scope.new_type_contract | yes | snapshot file schema in BatchView shape plus lead root fields, conversation ring entry schema, intake tool argument schema |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/mailbox_history_test.go, agents-plugin-pi/test/agent-batches.test.ts, mailbox-waiter.test.ts, mailbox-rearm.integration.test.ts, adapter-label.test.ts, adapter-config.test.ts; new Go test files likely for intake and snapshot |
| complexity.reuse_points | confirmed | agents-plugin-pi/src/agent-batches.ts BatchView and caps at L34-L40, mailboxHistoryCap ring in agents-plugin-tool/internal/mcp/mailbox_history.go#L35-L37, mailbox-waiter.ts, adapter-label.ts sendUserMessage note |
| complexity.side_effect_risk | high | new disk writes on every adapter event and in the mailbox send and receive path, debounce timers, and Pi user-message injection with user authority |
| risk.correctness | high | debounce coalescing, retention of running plus 5 finished agents, hold-until-agent-end rearm sequencing, and merge or cancel races in the inbox drain |
| risk.fit | moderate | must match the Claude adapter's schema and the daemon core's reader across three sibling children landing in parallel waves |
| risk.test | moderate | timing-dependent debounce and rearm behavior plus live hand checks with a Pi session and daemon |
| risk.security_or_contract | high | session key must never reach browser-bound fields, injected messages carry user authority, and the snapshot schema is a cross-version on-disk contract |

## Phases

### Phase 1: ws-mcp event intake, live snapshot, and conversation ring

Add the ws-mcp intake that adapters call with live events (an adapter-only
MCP tool, (289)), the debounced
per-session snapshot writer (lead root, subagent tree, retention, phases,
permission wait), the per-session conversation ring with origin tags, mail
entries written from ws-mcp's mailbox send and receive path carrying their
history key, and the nudge after each snapshot and ring write (core Phase
2's sender; if that has not landed, the worker leaves the call for whichever
lands second and says so in the Result). With the session's dashboard key
off (the core's `dashboard.` key from its Phase 1, which lands first), ws-mcp
writes no snapshot and no ring entries.

The worker settles and records in the Result:

- The snapshot and ring locations. Constraint: the daemon finds them from a
  registered project root and a reply-id. (169)
- The snapshot's field names and encoding. Constraint: the `BatchView` shape
  and caps above; readers ignore unknown fields.
- The ring constant N. Constraint: about 50. (258)
- The debounce for changes other than lead phase and streamed reply text.
  Constraint: lead phase and reply text stay at about 100 ms. (233), (250)
- How a reader tells a session that has an adapter from one that has none,
  which the core's board-first page reads. (190), (252)
- Where a delegate's session key becomes its reply-id label. Constraint:
  the session key never reaches the browser. (192)
- Which side marks a mail-started turn's `mail` origin. (260)
- What a turn summary contains and where it comes from. Suggested: the
  turn's totals (done, failed) and duration from adapter events, with no
  model-written text.
- How a stale or restarted snapshot reads, for example an updated-at field
  the core ages out. Constraint: a ws-mcp restart mid-turn loses its
  in-memory state, and a crashed session leaves a frozen snapshot; neither
  may show as live indefinitely. The settled field is the one the daemon
  core's reader ages snapshots by, so the core's session index uses it; the
  Result names it for the core worker.
- The intake call rate for streamed reply text. Constraint: adapters
  coalesce deltas on their side, and the intake tool's contract says so.

Verification: Go tests in `agents-plugin-tool/internal/mcp/` cover the
intake producing a snapshot with the lead root fields (turn start, done and
failed totals, previous turn duration), the batch, tool-name, and preview
caps, a streaming call shown by name and character count, retention of
running plus 5 finished agents with a failed marker kept, the permission-wait
state, debounce coalescing a burst into one write, unknown snapshot fields
ignored by the reader, no session key in the snapshot's browser-bound
fields, the ring keeping N entries and dropping the oldest, origin tags, and
mail entries from send and receive carrying their history key, direction,
peer, and first line (extending
`agents-plugin-tool/internal/mcp/mailbox_history_test.go`), the stale-snapshot
reading settled above, and a snapshot write error, a ring write error, a
nudge error, and an unreachable daemon each never failing or delaying an
intake call or a mailbox send or receive, and the dashboard key off leaving
no snapshot and no ring entries after intake calls and a mailbox send and
receive. The intake tool's `runtime.json`
entries pass `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`
and `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`
(`agents-plugin-tool/cmd/ws-mcp/main_test.go`) and the Pi package's byte
mirror check `TestPiMirrorUpToDate`
(`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`). Then `go test
./...` in `agents-plugin-tool`.

### Result (14605071e) - 2026-11-06

Landed:
- `internal/wslive` holds the shared schema, the reducer, the ring, and
  the readers:
  - `snapshot.go`: types, caps, `ReadSnapshot`, `Snapshot.Stale`,
    `AdapterPresent`.
  - `state.go`: the `State` reducer from adapter events.
  - `text.go`: line cleaning, argument flattening, and the session-key
    scrubber.
  - `ring.go`: `AppendRing`, `ReadRing`, `RingNames`.
- `internal/mcp/live_snapshot.go` adds:
  - the adapter-only MCP tool `dashboard.intake`, with args `{session_key,
    adapter, events[]}` and compact text output (`ok: X of Y events
    applied`, one `skipped event i: ...` line per rejected event, or
    `dashboard off: nothing recorded`);
  - the debounced snapshot writer with its heartbeat;
  - ring writes, and the mail lines that mailbox send and recv add.
- The tool is listed in the `runtime.json` of ws, wsflow, and Pi.
- The ws-mcp manual's "Dashboard daemon" section lists both stores.
- f49150edc moved the stores and wired the nudge after daemon core Phase 2
  landed (merged in fe7f4d4a2).
- 395dfb33a (a review fix) stops the heartbeat of a session that went
  inactive, so a session tuned off writes nothing more.

Settled:
- **Locations.** Each session's stores sit in its dashboard registry
  directory, `wsdashboard.SessionDir(wsdashboard.Dir(""), replyID)`, beside
  `registration.json`. That is `<cache root>/dashboard/sessions/<reply-id>/`:
  - `snapshot.json`, overwritten via temp file and rename;
  - `ring/<UTC ns stamp>-<hex>.json`, one immutable file per entry.

  Only a registered top-level lead session whose `dashboard.enabled`
  resolves on is written. Child sessions write nothing: they appear in their
  lead's subagent tree, and their mail gets no ring line.
  `PruneRegistrations` removes the stores together with the session
  directory.
- **Snapshot encoding.** JSON with camelCase fields: `{schemaVersion: 1,
  replyId, adapter, updatedAt, heartbeatMs, lead, agents[]}`.
  - `lead` and each agent carry `phase`, `phaseStartedAt`, `caption`,
    `done`, `failed`, `batches[]`, and `permission {toolName, argsPreview,
    startedAt}`.
  - `lead` adds `turnStartedAt`, `turnOrigin`, `previousTurnMs`, and `reply
    {text, startedAt, truncated}`.
  - Each agent adds `id`, `parentId`, `label`, `replyId`, `status`
    (running, done, or failed), `startedAt`, and `endedAt`.
  - `batches` is Pi's `BatchView` verbatim: `{epoch, seq, toolNames,
    callCount, failed, startedAt, endedAt | running {toolName, argsPreview,
    startedAt, done, othersRunning} | generating {kind, phaseStartedAt,
    toolName, chars}}`.
  - Caps: 5 batches, 5 tool names, a 200-character preview, 64-rune tool
    names. Text is capped at 64 KiB, keeping the head and setting
    `truncated`. The tree keeps every running agent (a defensive cap of 64)
    plus the 5 most recent finished ones, and a failed agent keeps
    `status: failed`.
  - Readers ignore unknown fields, and absent fields read as empty.
- **Phase.** The first match wins:
  - `permission` while a permission wait is set;
  - otherwise the generating view's kind (`thinking`, `replying`, `tool`);
  - otherwise `tool` while a batch is running;
  - otherwise `replying` while the lead streams a reply;
  - otherwise `thinking` inside a turn or for a running agent;
  - otherwise `idle`.
- **Ring.** N = 50 (`wslive.RingCap`). An entry is `{kind, origin, text,
  truncated, done, failed, durationMs, direction, peer, title,
  historyKey}`, depending on its kind (user, reply, turn, or mail). The
  entry's time is its file-name stamp.
- **Debounce.** About 100 ms for a change to the lead's phase or streamed
  reply, 250 ms for anything else. A burst inside one window becomes one
  write.
- **Staleness, for the core worker.** The field is `updatedAt` (epoch ms).
  ws-mcp also rewrites the snapshot every 30 s as a heartbeat
  (`heartbeatMs`), and `Snapshot.Stale(now)` is true once `now - updatedAt
  > 3 * heartbeatMs`. A restarted ws-mcp starts from empty state; a crashed
  or abandoned writer leaves a snapshot that goes stale.
  `DashboardSessionSource` now:
  - reports `SessionData.SnapshotUpdatedAt`, which the index carries as
    `Session.SnapshotUpdatedAt`;
  - folds the snapshot (without `updatedAt`, but with its stale flag) and
    the ring entry names into the fingerprint.

  A heartbeat therefore does not raise `Rev`, while a writer that stopped
  does, at the next 30 s poll.
- **Adapter presence.** `wslive.AdapterPresent(path, now)`: a readable,
  non-stale snapshot with a non-empty `adapter`. A ring alone does not
  count.
- **Delegate labels.** An adapter sends a delegate's session key as the
  event's `sessionKey`. ws-mcp turns it into `replyId` (`wsmailbox.ReplyID`)
  and never stores the key. Before capping, a scrubber replaces every key
  seen so far, plus `session_key: <value>` argument forms, with
  `id:<reply-id>` in previews, labels, captions, and text.
- **Mail origin.** The adapter marks it through `turn_start {origin:
  "mail"}`. User messages are tagged `terminal` or `whiteboard`.
- **Turn summary.** ws-mcp writes it at `turn_end` as `{kind: "turn",
  origin, done, failed, durationMs}`. The counts are the turn's tool-call
  totals: finished batches deduplicated by epoch and seq, plus the running
  batch's finished calls. It contains no model text. A turn_end with no
  turn_start (ws-mcp restarted mid-turn) writes no summary. turn_end, or an
  agent ending, closes running batches (an unfinished call counts as
  failed).
- **Reply rate.** The adapter sends the full text so far, replacing the
  previous text, at most about 10 calls a second, coalescing on its own
  side; the tool description says so. `final: true` moves the text to the
  ring.
- **Nudge.** Each successful snapshot or ring write calls
  `s.dashboardNudge(sessionKey)`. It does not block, and it is a no-op for
  an unregistered session.
- **Failure isolation.** Write errors are logged only, as debug events
  `dashboard.snapshot_write_error` and `dashboard.ring_write_error`. Intake
  and mailbox calls never fail or wait on the dashboard. The intake tool
  gets no mailbox piggyback badge.

Verification:
- `go test ./... -count=1` in `agents-plugin-tool`: all packages ok. That
  includes the runtime capability tests and `TestPiMirrorUpToDate`.
- `go test -race` on the new dashboard, live, and mail tests (`-count=3`),
  and on `internal/wslive`: ok.
- The new tests are `internal/wslive/state_test.go`, `ring_test.go`,
  `internal/mcp/live_snapshot_test.go`, the mail-line test in
  `mailbox_history_test.go`, and `TestIndexSnapshotAgeIsNotAChange`.
- Lite review: one Important finding (the heartbeat kept writing for a
  session tuned off), fixed in 395dfb33a with
  `TestDashboardTurnedOffStopsTheHeartbeat`. A minor finding, that a
  rejected permission event created an agent, is fixed there too.

Open:
- Minor review findings not fixed:
  - a status-less event for an agent that retention already dropped
    recreates it as running;
  - tool names, epoch, agent ids, and mail titles are not scrubbed. They
    are validated identifiers or mailbox text, and mail bodies already sit
    unscrubbed in the mailbox history.

### Phase 2: Pi adapter feeds the intake

The Pi extension hands its lead and subagent events to its session's
ws-mcp intake from Phase 1: batches, generating phase, tool calls, captions,
turn boundaries, finished user messages with their origin, lead replies
(streamed text in progress, the finished reply into the ring), and turn
summaries. The `pi.` dashboard knob (291) gates the feed; when it is off the
extension sends no intake events.

The worker settles and records in the Result:

- The knob's name. Constraint: default on; the manifest default and the
  built-in default stay pinned equal by the existing test.
- How a user message's origin (whiteboard or terminal) is known on the Pi
  side. Constraint: a message injected from the inbox carries the whiteboard
  label (210).

Verification: Node tests in `agents-plugin-pi/test/` (extending
`agent-batches.test.ts` or a new test file) cover the events the extension
sends for a lead turn with tool calls, a subagent with a session key
labelled by reply-id, streamed reply text, a user message carrying its
origin tag, a finished reply and a turn summary reaching the ring, the
permission-wait state, a failed agent, streamed reply deltas coalesced on
the extension side rather than one intake call per token, an intake failure
leaving the session unaffected, and the `pi.` knob off sending no intake
events; run `npm test` in `agents-plugin-pi`. The lead checks by hand with a
live Pi session and a running daemon that the lead bar, the subagent tree,
and the conversation follow the session, including streamed reply text. If
the core's Phase 3 has not landed, the lead inspects the snapshot and ring
files directly instead, and the live-page check happens once it lands.

### Result (2fb9b65f8) - 2026-11-06

Landed:
- `agents-plugin-pi/src/dashboard-feed.ts` (new) feeds `dashboard.intake`
  through the bridge's ws-mcp client. It is wired in `index.ts`, with small
  hooks in `spawner.ts`.
- `agents-plugin-pi/src/adapter-label.ts` gains the whiteboard label.
- `agents-plugin-pi/src/bridge.ts` hides adapter-only tools from the model.
- The `pi.dashboard_enabled` knob is declared in `config-manifest.json` and
  `goal-loop.ts`.
- New tests are in `test/dashboard-feed.test.ts`; `adapter-config.test.ts`
  and `bridge.test.ts` are extended.
- Review fixes landed in 9926e0b5f.

Settled:
- **Knob.** `pi.dashboard_enabled`, boolean, default true, global default
  scope. Its manifest default is pinned to the built-in default by the
  existing resolver test.
  - It is read through `readAdapterConfig`, and one read holds for 2 s
    whether it was on or off.
  - With the knob off, no intake call is made. Without a bridge or a session
    key, events are held and retried.
  - Its description covers only the feed. Phase 3 should widen it to
    submission.
- **User-message origin.** `WHITEBOARD_MESSAGE_LABEL = "[user message from
  the ws whiteboard]"` lives in `adapter-label.ts`, with
  `labelWhiteboardText` and `whiteboardLabeledBody`. Phase 3 should inject
  through `pi.sendUserMessage(labelWhiteboardText(text))`.
  - A finished user message that opens with that label is sent as
    `whiteboard`, with the label stripped.
  - Adapter-labelled messages are not sent.
  - Everything else is `terminal`.
  - A turn's origin comes from the `input` event that started it.
  - A mailbox wake of an idle lead marks the next turn `mail`, and an
    adapter-labelled wake input does not overwrite that mark.
- **Lead turn.** It runs from Pi `agent_start` to `agent_settled`.
  - A lead `AgentBatchBuffer` is fed by the same `observeBatchEvent` the
    widget uses, and goes out as `agent` events with no agentId.
  - The streamed reply is the full text so far, thinking excluded. Every
    assistant message with text ends in `reply {final: true}`.
- **Subagents.** `dashboardAgentEntries(registry)` uses the widget relay's
  agents, order and batch views, including `parentId`.
  - Label: alias, then title, then short id. Caption: role plus a state
    phrase.
  - Status is running while the agent is launching, running, streaming,
    awaiting approval, or waiting on its own agents. It is failed when
    `lastRunFailed` is set, which happens on a spawn failure or an exit
    during in-flight work. Otherwise it is done.
  - A fork's own session key comes from its readiness payload. It is held
    as live-only `ownSessionKey` and sent only as `sessionKey`. Worker and
    explore children carry no key the parent can see, so they show by label
    only.
- **Permission waits.** Pi has no tool permission prompt for the lead. An
  execute-worker waiting on approval goes out as `permission {agentId,
  tool: "ws-worker-exec", args: {command}}` and is cleared when the wait
  ends.
- **Coalescing and isolation.**
  - Handlers record state synchronously and never await or throw.
  - One flush runs at a time, at most one intake call per 100 ms.
  - Discrete events wait in a list capped at 100 that drops its oldest
    entry. Each is queued after the lead and stream state held before it.
  - A call counts as delivered only if it resolves without `isError` within
    5 s. A rejection, a timeout, or a missing bridge triggers a resync after
    2 s. The resync sends `reset`, then the remembered `turn_start`, then
    the lost discrete events and the full lead and tree state.
- **Hidden tool.** `filterOutAdapterOnlyTools` removes `dashboard.intake`
  from the registered Pi tools and from `wsToolNames`.

Verification:
- `npm test` in `agents-plugin-pi`, with `node_modules` linked from the
  main checkout for the run: 2544 tests, 35 fail, 1 cancelled.
  - The failing names match the pre-change baseline exactly.
  - One baseline failure, the bundled 65-tool contract check in
    `bridge.test.ts`, already failed before this ticket because
    `runtime.json` lists 69 tools.
- `test/dashboard-feed.test.ts`: 29 tests pass.
- Lite review: findings fixed in 9926e0b5f.
  - Critical: the mail origin was wiped by the wake input.
  - Important: a failed call was treated as delivered, and a resync did
    not resend state.
  - Important: an idle exit after finished work flipped an agent to
    failed.
  - Two minor items.

Open:
- The lead's hand check with a live Pi session and the daemon (the core's
  Phase 3 page has not landed, so inspect `snapshot.json` and `ring/` under
  the session's registry directory).
- A grandchild whose process fails shows as done; only direct children
  carry the failure flag.
- A leftover mail mark tags the next turn that has no origin of its own as
  mail.
- A call that times out but still lands has its conversation entries
  resent after the resync, so they appear twice.

### Phase 3: Pi drain and inject

The Pi extension arms its existing `mailbox wait` child with the inbox flag.
On an inbox wake while no turn runs, it drains through the drain tool and
injects the message with `pi.sendUserMessage` and the whiteboard label; while
a turn runs, it leaves the entry held, re-arms without the inbox flag, and
at agent end drains, injects, and re-arms with the flag. The same `pi.`
dashboard knob (291) gates submission: when it is off, the wait is armed
without the inbox flag.

Verification: Node tests in `agents-plugin-pi/test/` (extending
`mailbox-waiter.test.ts` and `mailbox-rearm.integration.test.ts`, and
`adapter-label.test.ts` for the label) cover an idle wake injecting at once,
a mid-turn wake re-arming without the flag and injecting only at agent end,
the flag restored after the drain, a cancelled entry never injected, a
merged entry injected once as one message, an injected message entering the
ring tagged `whiteboard`, and the `pi.` knob off arming the wait without the
inbox flag; run `npm test` in `agents-plugin-pi`. The lead checks by hand
with a live Pi session: a message sent from the composer while idle starts a
turn as user input with the whiteboard label and shows in the conversation
tagged whiteboard; one sent mid-turn shows as held, can be cancelled or
merged, and is submitted when the turn ends.
