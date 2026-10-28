---
title: Pi agent widget expanded mode with live per-agent tool batches
related:
  260905-feat-ws-pi-live-agent-widget: extends this widget
  260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets: tree gutter and state bullets reused
  260908-epic-ws-pi-subagent-conversation-view: neighbouring per-child view (/audit); separate surface
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: bb2b583fd91051a7
sage-review-completeness-reviewed: bb2b583fd91051a7
---
# Pi agent widget expanded mode with live per-agent tool batches

## Background

The user now runs Pi mostly as a mailbox partner lead whose main job is
watching its child agents work. The live agent widget (`src/agent-widget.ts`,
key `ws-agents`, placed below the editor) shows one status row per live
agent, capped at five rows, and nothing about what each agent is doing. The
user wants a monitoring mode: the conversation keeps a few rows at the top,
and a large bottom region shows each agent's recent work live.

Evidence gathered on 2026-10-28 (pi-coding-agent / pi-tui 0.84.4):

- Child RPC events already reach the lead in process: the spawner's
  `client.onEvent` handler sees `tool_execution_start` / `tool_execution_end`
  (tool name, args, result, error), message start / end and token deltas.
  `RpcAgentRecord` keeps only `lastText`, timing and telemetry, so no recent
  tool history exists today. Token deltas deliberately avoid per-token
  fan-out.
- A string-array widget is capped at 10 lines by the host; a component
  factory widget is not, and its factory receives the `tui`, whose
  `terminal.rows` gives the height.
- Regular `tuiMode`: the main-screen renderer draws the transcript and the
  dock as one line list and shows the bottom `terminal.rows` lines, so a tall
  widget pushes the transcript into scrollback and leaves its last few lines
  visible. When the host's `clearOnShrink` setting is on, a shrink of the
  rendered content takes the `clearOnShrink` path, a full redraw that clears
  screen and scrollback; the setting defaults off (`terminal.clearOnShrink` in
  settings, else `PI_CLEAR_ON_SHRINK=1`), and with it off a shrink is a
  differential redraw (pi-coding-agent `dist/core/settings-manager.js#L832-L838`,
  pi-tui `dist/tui-main-screen.js#L237-L243`, `#L314-L318`).
- Fullscreen `tuiMode` (experimental): the transcript is a `ScrollView`
  (grow/shrink, `minSize: 1`) and the dock children each have `shrink: 1`, so
  an oversized widget may be shrunk rather than the transcript.
- There is no extension API that resizes the transcript region directly.
- The display summary stack (`src/display-summary.ts`) already resolves
  `pi.display_summary_model`, `pi.display_summary_effort` and `workflow.lang`,
  and batches row summaries into one flush request.

## Decisions

- **A slash command toggles the agent widget between its normal mode and an
  expanded mode.** Rejected: an always-on large panel, and an automatic switch
  when children run.
- **The expanded mode works in both regular and fullscreen `tuiMode`.**
- **Row unit is one tool batch: one assistant message and the tool calls it
  made.** Rejected: one row per agent run (lead prompt to settle), since a
  worker's run spans tens of minutes and every row would read as current.
- **Each agent shows its latest N rows, N configurable, default 3, the running
  row included.** Child agents nest under their parent with tree marks.
- **Every agent gets a fixed allotment of N rows**, blank rows padding an
  agent with fewer batches, so the widget height changes only when an agent
  enters or leaves the widget. An agent enters or leaves by the existing
  widget's row rules (live agents and open owner threads). The height
  stays put while batches come and go, and for a user who turns on the host's
  opt-in `clearOnShrink` a shrink would also clear scrollback. Rejected: a
  content-sized height that moves with every batch; a high-water-mark height
  that only grows until toggle-off.
- **Over the height budget, every agent's allotment shrinks uniformly**, down
  to one batch row under its header, and agents that still do not fit collapse
  into a `+N more` tail. An agent awaiting the owner or an approval never
  folds into the tail: it keeps at least its header row, together with its
  ancestors' header rows so its tree marks stay intact, and only running
  agents collapse. When the awaiting headers alone exceed the budget, the
  budget is exceeded rather than an awaiting row hidden (the existing widget's invariant, ae03bcc2, and
  260909-feat-ws-pi-agent-row-model-and-usage's "preserve waiting-row
  visibility"). The allotment is recomputed only when an agent enters
  or leaves and on terminal resize. Rejected: dropping the oldest dimmed rows
  and folding settled agents first, which changes the height at settle.
- **A finished batch row is dimmed; the running batch row is bright.**
- **No tool-verb prefix column** (`read | ran | edit`); the user found it
  awkward in the preview.
- **A finished batch is summarized once, when it ends; there is no in-progress
  summary.** Rejected: summarizing at batch start and again at its end, which
  doubles the model calls.
  - With a display summary model: `<summary> · N tool calls · <relative time>`.
  - Without one: `called <tool names> · total N tool calls · <relative time>`.
- **Batch summaries stay off until a dedicated config flag is raised (through
  `lead-tune` / `config.tune`), even when a display summary model is set.**
  Summarizing every child batch is expected to cost a lot of quota, so a
  configured model alone must not turn it on.
- **The running batch row has a spinner prefix and shows the oldest call
  still in flight, its elapsed time, the batch's finished-call count and how
  many other calls are still running:**
  `⠋ <tool> (<args preview>) · <elapsed> · <done>/<n> done · +<k> running`
  (the `+<k> running` part only when k > 0). Pi runs a message's tool calls in
  parallel by default: pi-agent-core's `Agent` defaults `toolExecution` to
  `"parallel"` and runs them one at a time only when that option is
  `"sequential"` or a called tool declares `executionMode: "sequential"`, and
  pi-coding-agent's `dist/core` sets no `toolExecution` (pi-coding-agent
  `node_modules/@earendil-works/pi-agent-core/dist/agent.js#L134`,
  `dist/agent-loop.js#L287-L288`). Rejected: an `<i>/<n>` position, which
  assumed sequential execution; one running row per in-flight call, whose row
  count varies inside a batch against the fixed allotment; the most recently
  started call, since the oldest is the one holding the batch up.
- **The expanded mode extends the existing `ws-agents` widget in place**, same
  key and placement; no second widget.
- **Height budget is computed from `tui.terminal.rows`**: terminal rows minus
  the editor, the footer and at least 5 transcript rows. The implementation
  measures the editor and footer heights where the host exposes them and
  otherwise estimates them conservatively. Rejected: relying on
  the fullscreen dock shrink, which may shrink the widget rather than the
  transcript, and which regular mode does not have.
- **A bounded per-agent batch buffer is fed by the spawner's existing
  `client.onEvent` handler.** Rejected: reading each child's session JSONL,
  which adds file I/O and parse lag for events already in process.
- **While expanded and a batch is running, the widget repaints every 1 s**
  for the elapsed field, a descendant's running batch from its `agent-view`
  start time included; normal mode keeps its 10 s tick.
- **The args preview is the call's arguments flattened to one line and
  truncated to the row width.** Rejected for now: per-tool formatters; one
  code path serves every tool.
- **An assistant message with no tool calls makes no row.** Rejected: a
  `replied · <relative time>` row; the agent header already shows its state.
- **The slash command is `/agents`, a toggle.** Rejected: `/monitor`,
  `/ws-agents`. A clash with a host built-in is checked at implementation.
- **The expanded state is session-only**: every launch starts in normal mode.
  Rejected for now: remembering the last state, or a start-mode config key.
- **Row count N and the batch summary flag are
  `agents-plugin-pi/config-manifest.json` keys set through `config.tune`**
  (`pi.agent_widget_rows`, default 3, and
  `pi.agent_widget_batch_summary_enabled`, default false),
  like the existing `pi.display_summary_*` keys.
- **Batch summaries reuse `pi.display_summary_model`,
  `pi.display_summary_effort` and `workflow.lang`.**
- **Finished batches across agents coalesce into one summary flush request
  per short window.** Rejected: one model call per batch.
- **Descendant agents get batch rows through a separate control-channel
  message** (working name `t: "agent-view"`), not through the subtree
  snapshot. Each process computes a width-independent batch model for its own
  direct children; each hop merges its direct children's models with the ones
  forwarded from below, keyed by agent id within `MAX_SUBTREE_DESCENDANTS`,
  and relays the result to its parent. The root widget alone renders, since
  only it knows the width, the per-agent allotment and the theme. The user:
  grandchildren are often the actual executors.
  - Why a new path: the spawner's `client.onEvent` sees only direct
    children; descendants reach the widget as `record.subtreeDescendants`
    (`agent-widget.ts:221`, `subtree-lifecycle.ts:26-34`), which carries no
    tool events.
  - Every RPC child loads the parent's exact extension entry
    (`spawner.ts:2783`, `--no-extensions --extension <extensionPath>`), so a
    tree runs one extension version and the new message type needs no
    compatibility fallback. The control channel is parent-child only (the
    grandparent endpoint is blanked per hop, `spawner.ts:2771`), hence the
    hop-by-hop relay. Pi's RPC stdio protocol is not ws-owned, so the message
    rides `agent-channel.ts`.
  - Shape: per batch the tool names (deduplicated, capped), the call count
    and the end time; for the running batch the oldest in-flight tool, its
    args preview (truncated), its start time, done / n and the other
    in-flight count. Each batch carries a per-agent sequence number, the
    stable key the root uses to summarize a relayed batch once. Tool names
    cap at 5 per batch and the args preview at 200 characters. A fixed cap
    of 5 batches per agent, since the child does
    not know the lead's N; the lead shows its latest N. The receiver computes
    elapsed from the start time, so no per-second publishes.
  - Delivery: always sent, unacknowledged, latest value wins, throttled at
    `SUBTREE_TELEMETRY_INTERVAL_MS` (1 s), skipped when unchanged, dropped on
    disconnect, and never an input to wait or settle. A hop drops a child's
    forwarded entries when that child disconnects or exits; a child resets
    its unchanged-key and republishes on reconnect, so idle descendants'
    rows come back without waiting for a new tool call. Past
    `MAX_SUBTREE_DESCENDANTS`, a hop keeps the same entries the subtree
    snapshot keeps (its parent-before-child selection).
  - Rejected: a `batches` field on the snapshot's per-descendant `display`.
    The snapshot is acknowledged lifecycle state behind the busy fence and
    the hello resume, `promptPublicationKey` already strips display so
    telemetry cannot delay lifecycle edges, and batches change on every tool
    call, so the field would bloat every lifecycle publish and resume payload.
  - Rejected: demand-driven delivery, where the root's `/agents` toggle
    propagates a subscribe down the tree. Always-on traffic is about 1 KB per
    agent per second on loopback, while subscribe state per hop, delivery to
    children spawned while subscribed, and restoration on reconnect are the
    lifecycle edges most likely to fail silently. Revisit only if the traffic
    measures as a problem.
  - Rejected: header rows only for descendants; the lead reading each
    descendant's session JSONL (file I/O and parse lag).
- **Batch summaries cover descendant batches too, from the message's tool
  names alone**, so a descendant's summary is coarser than a direct child's,
  whose summary request can carry each call's arguments and results.
  Rejected: descendant rows keeping the no-model fallback text even with
  summaries on, which leaves the executors without summaries; widening the
  message with per-call args previews, which multiplies its size across up
  to 128 descendants.
- **Three phases**: Phase 1 the expanded mode for direct children with
  descendants as header rows, Phase 2 the descendant `agent-view` message,
  Phase 3 the summaries. The message builds on Phase 1's batch buffer and a
  channel change deserves its own verification; the summaries consume the
  rows of both and add the model-cost path behind their own flag. Rejected:
  folding the channel change into Phase 1.

## Constraints

- Runtime pi-tui classes come from the host through `loadHostPiTui()`
  (`src/pi-tui.ts`); a static import of `@earendil-works/pi-tui` is a second
  module instance (repo note `gotcha.pi-tui-dual-package`).
- UI wiring stays gated on `ctx.mode === "tui"`, as the existing widget arming
  is.
- Agent-driven tmux probes use an isolated server socket
  (`tmux -L ws-probe-<pid>`), never the default one (machine note
  `tmux-probe-isolation`).

## Prior Decisions

- ae03bcc2 (2026-09-05, commit): "`buildAgentRows` is pure and returns the full uncapped/sorted row set; `buildWidgetLines` applies the display cap (5, "+N more" tail, never folding an awaiting-state row) and width truncation separately" — bearing: constrains
- ae03bcc2 (2026-09-05, commit): "`createAgentWidgetController`'s 10-second elapsed timer arms/disarms only while there is at least one row; `index.ts` stops any prior controller before creating a new one in `session_start` to guard against a `/reload` timer leak." — bearing: constrains
- ae03bcc2 (2026-09-05, commit): "Wiring uses a new `agentWidgetRefreshRef` mutable ref in spawner.ts ... so spawner.ts's registry-transition points ... can trigger a re-render without either module importing agent-widget.ts" — bearing: constrains
- cbcc14cc (2026-09-10, commit): "The existing widget factory remains the render-time width authority, so the heading shares the established truncation path at 40/80/120 columns." — bearing: supports
- 260909-feat-ws-pi-agent-row-model-and-usage (2026-09-09, Decisions): "Preserve row order, waiting-row visibility, cap semantics, question hints, and terminal-width bounds. This is human-facing display; no new model tool, billing integration, or change to delegation/settlement behavior." — bearing: constrains
- 038d4224 (2026-09-10, commit): "Shared compact duration and latest-input formatters prevent the audit surface from drifting from the existing agent widget." — bearing: constrains
- 260905-feat-ws-pi-live-agent-widget (2026-09-05, commit 07d24cdc): "shouldArmAgentWidget lives in agent-widget.ts (not index.ts) since the widget owns its own arming contract" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/spawner.ts RpcAgentRecord and client.onEvent handler, agents-plugin-pi/src/index.ts command registration, agents-plugin-pi/config-manifest.json, agents-plugin-pi/src/agent-channel.ts and agents-plugin-pi/src/subtree-lifecycle.ts for Phase 2, agents-plugin-pi/src/display-summary.ts for Phase 3 |
| scope.surface | public-interface | new /agents slash command and new config-manifest.json keys ship downstream in the Pi package |
| scope.new_public_symbol | yes | /agents command, row-count N config key, batch-summary flag config key |
| scope.new_type_contract | yes | per-agent bounded tool-batch buffer on RpcAgentRecord; expanded-mode widget state; agent-view control-channel message |
| scope.test_surface | existing | agents-plugin-pi/test/agent-widget.test.ts, agents-plugin-pi/test/spawner.test.ts, agents-plugin-pi/test/display-summary.test.ts, the subtree-lifecycle channel tests |
| complexity.reuse_points | confirmed | buildWidgetLines and controller in agent-widget.ts, spawner client.onEvent at spawner.ts#L3172, display-summary flush and buildFlushRequest, loadHostPiTui in pi-tui.ts |
| complexity.side_effect_risk | moderate | adds buffering on the spawner's hot RPC event path and a 1 s repaint timer beside the existing 10 s tick |
| risk.correctness | moderate | height budget, uniform allotment shrink and +N more tail, and the running row must handle the default parallel tool execution |
| risk.fit | moderate | tall widget interacts with regular-mode scrollback and fullscreen dock shrink, both host-owned layout paths |
| risk.test | moderate | layout correctness needs isolated tmux probes in both tuiModes beyond unit tests |
| risk.security_or_contract | moderate | new config keys and a Phase 3 model-cost path that must stay off unless its flag is raised; child tool args flow into a summary prompt |

## Phases

### Phase 1: Expanded mode with per-agent tool batch rows

Add the expanded mode to the live agent widget per `## Decisions` for direct
children, with the no-model row text for every finished batch; descendants
show their existing header rows. Verify the layout in both `tuiMode`s with an
isolated tmux probe, and pin row formatting, the batch buffer, the height
budget and the awaiting-agent fold rule with unit tests.

### Phase 2: Descendant batch rows over the control channel

Add the `agent-view` control-channel message per `## Decisions`: each
process publishes its direct children's batch model, each hop merges and
relays, and the root widget renders descendant batch rows from it. Pin the
message parse, the hop merge and its bound, the throttle, and the
drop-on-disconnect with unit tests, and check a three-level tree live with an
isolated tmux probe.

### Phase 3: Display summary for finished batches

When the batch summary flag is raised and a display summary model is
configured, summarize each finished batch once, descendants' from their tool
names, and replace its fallback text with
`<summary> · N tool calls · <relative time>`. Phase 1's and Phase 2's rows
are the input. Pin with unit tests: flag off makes zero model calls even with
a summary model set, each batch (descendants' by sequence number) is
summarized once, finished batches across agents coalesce into one flush, and a
descendant's request carries tool names only.
