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
