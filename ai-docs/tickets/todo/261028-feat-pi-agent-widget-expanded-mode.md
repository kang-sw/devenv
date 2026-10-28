---
title: Pi agent widget expanded mode with live per-agent tool batches
related:
  260905-feat-ws-pi-live-agent-widget: extends this widget
  260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets: tree gutter and state bullets reused
  260908-epic-ws-pi-subagent-conversation-view: neighbouring per-child view (/audit); separate surface
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
  visible. A shrink of the rendered content takes the `clearOnShrink` path, a
  full redraw that clears screen and scrollback.
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
  widget's row rules (live agents and open owner threads). Rejected: a
  content-sized height, whose frequent shrinks each take the regular-mode
  `clearOnShrink` full redraw that clears scrollback; a high-water-mark height
  that only grows until toggle-off.
- **Over the height budget, every agent's allotment shrinks uniformly**, down
  to one batch row under its header, and agents that still do not fit collapse
  into a `+N more` tail. The allotment is recomputed only when an agent enters
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
- **The running batch row has a spinner prefix and shows the call in flight
  with its elapsed time and its position in the batch:**
  `⠋ <tool> (<args preview>) · <elapsed> · <i>/<n>`. The user recalls Pi
  executing a message's tool calls sequentially.
- **The expanded mode extends the existing `ws-agents` widget in place**, same
  key and placement; no second widget.
- **Height budget is computed from `tui.terminal.rows`**: terminal rows minus
  the editor, the footer and at least 5 transcript rows. Rejected: relying on
  the fullscreen dock shrink, which may shrink the widget rather than the
  transcript, and which regular mode does not have.
- **A bounded per-agent batch buffer is fed by the spawner's existing
  `client.onEvent` handler.** Rejected: reading each child's session JSONL,
  which adds file I/O and parse lag for events already in process.
- **While expanded and a batch is running, the widget repaints every 1 s**
  for the elapsed field; normal mode keeps its 10 s tick.
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
  `agents-plugin-pi/config-manifest.json` keys set through `config.tune`**,
  like the existing `pi.display_summary_*` keys.
- **Batch summaries reuse `pi.display_summary_model`,
  `pi.display_summary_effort` and `workflow.lang`.**
- **Finished batches across agents coalesce into one summary flush request
  per short window.** Rejected: one model call per batch.
- **Summaries are Phase 2**, since they consume Phase 1's rows and add the
  model-cost path behind their own flag.

## Constraints

- Runtime pi-tui classes come from the host through `loadHostPiTui()`
  (`src/pi-tui.ts`); a static import of `@earendil-works/pi-tui` is a second
  module instance (repo note `gotcha.pi-tui-dual-package`).
- UI wiring stays gated on `ctx.mode === "tui"`, as the existing widget arming
  is.
- Agent-driven tmux probes use an isolated server socket
  (`tmux -L ws-probe-<pid>`), never the default one (machine note
  `tmux-probe-isolation`).

## Phases

### Phase 1: Expanded mode with per-agent tool batch rows

Add the expanded mode to the live agent widget per `## Decisions`, with the
no-model row text for every finished batch. Verify the layout in both
`tuiMode`s with an isolated tmux probe, and pin row formatting, the batch
buffer and the height budget with unit tests.

### Phase 2: Display summary for finished batches

When the batch summary flag is raised and a display summary model is
configured, summarize each finished batch once and replace its fallback text
with `<summary> · N tool calls · <relative time>`. Phase 1's rows are the
input.
