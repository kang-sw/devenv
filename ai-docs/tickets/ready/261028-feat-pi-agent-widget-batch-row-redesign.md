---
title: Pi agent widget batch row redesign with generating phase and failure marks
related:
  261028-feat-pi-agent-widget-expanded-mode: redesigns the expanded mode this ticket shipped; its decisions constrain
  260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets: the depth gutter and state bullets the layout moves
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 892a99aa6f8a720f
sage-review-design-reviewed: 892a99aa6f8a720f
---
# Pi agent widget batch row redesign with generating phase and failure marks

## Background

`261028-feat-pi-agent-widget-expanded-mode` shipped the `/agents` expanded mode
of the live agent widget. In use, the user finds its batch rows hard to read:

- A finished row starts its text in the column where the running row puts its
  spinner, so finished and running rows do not line up.
- Prose and metrics interleave (`called read, grep · total 4 tool calls · 1m ago`).
- The depth gutter `│ ` sits in front of child agents' header rows, not in front
  of the batch rows it groups.
- Nothing shows while an agent is thinking or streaming a tool call's
  arguments (for example a long file `write`). A batch opens only at the
  assistant `message_end` (`agent-batches.ts`, `observeBatchEvent`), so that
  time, often long, reads as idle.

Evidence (2026-10-28, pi-ai / pi-coding-agent 0.84.4):

- The spawner's `client.onEvent` handler already receives `message_update`
  events whose `assistantMessageEvent.type` is one of `thinking_start` /
  `thinking_delta` / `thinking_end`, `text_start` / `text_delta` / `text_end`,
  `toolcall_start` / `toolcall_delta` / `toolcall_end`
  (`node_modules/@earendil-works/pi-ai/dist/types.d.ts`, `AssistantMessageEvent`).
- Over RPC the child's events reach the spawner without the partial message:
  `toJsonEvent` (`pi-coding-agent/dist/modes/json-event.js`) drops `message`
  from `message_update` and `partial` from every assistant event, and
  `toolcall_start` carries `id` and `toolName` instead. A `toolcall_delta`
  therefore holds only `contentIndex` and its `delta` text; the full tool call
  arrives at `toolcall_end` (design review, 2026-10-28).
- Per-token fan-out (confirmed by code in the design review): `isStreamedAssistantOutput`
  (`spawner.ts`) counts only `text_delta` and `thinking_delta` as streaming
  deltas, so each `toolcall_delta` falls through to `publishSubtree`
  and `refresh` in the `client.onEvent` handler.

## Decisions

- **The tree mark moves to the batch rows.** Each agent's batch rows begin with
  a dim `│`, and a child agent's header is indented by plain spaces, two per
  depth, instead of the `│ ` gutter. The child's bullet and its own batch
  rows' `│` sit two columns right of its parent's bullet and `│`, as in the
  sketch.
- **The left column holds prose only; every metric is right-aligned at the
  row's right edge.** Prose is the display summary when one exists, else the
  finished batch's tool names, else the running call (its args preview once
  it executes).
  Prose truncates first; the metric column truncates only when it alone does
  not fit. Rejected: metrics inline after the prose (the current
  `· N tool calls · Xs ago`); the `called` / `total` wording. Supersedes
  261028-feat-pi-agent-widget-expanded-mode's running-row wording
  `<elapsed> · <done>/<n> done · +<k> running`; the running row's metrics are
  `<done>/<n> · +<k> · <elapsed>` as in the sketch.
- **A finished batch row carries a `✓` in the spinner's column.**
- **A finished batch with a failed call shows `✗` in that column and `N failed`
  among its metrics** (option D). A running batch keeps its spinner even after
  a call failed; its metrics carry `N failed` until it finishes. This needs a
  failure count in the batch view and in the `agent-view` relay message.
- **The running batch row exists from the assistant message's start.** It moves
  through a generating phase (thinking, reply text, tool-call argument streaming
  with the streaming call's tool name and a character count) and then the
  execution phase that exists today. It opens only on an assistant
  `message_start` (user and toolResult messages emit one too); what it shows
  before the first content block starts is the implementer's call, pinned in a
  test. It is driven by `message_update` events, costs O(1) per delta, and is
  repainted by the expanded mode's existing 1 s tick: a generating row counts
  as running for `hasRunningBatch`, so the fast tick runs while it shows, and
  it reaches `collectBatchViews` and the agent-view model like a running
  batch. Descendants get the phase over `agent-view`.
- **Colors**: finished rows stay dimmed, their `✓` included; the running row
  stays bright; only `✗` takes the error color (the expanded-mode ticket's
  "a finished batch row is dimmed; the running batch row is bright").
- **`N failed` sits in the right-aligned metric column**:
  `1 call · 1 failed · 20s ago`.
- **An agent's unfilled history slot shows a placeholder row**: the batch-row
  `│`, then a dim `-` in the mark column, nothing else. It replaces today's
  gutter-only blank row (`buildExpandedWidgetLines`, the per-agent padding
  loop), so an agent with fewer batches than its allotment reads as empty
  history, not as missing rows. The fold padding after the `+N more` tail
  stays blank; it belongs to no agent. User directive (2026-10-28).
- **A generating row whose assistant message ends with no tool calls is
  removed** (the expanded-mode ticket's "an assistant message with no tool
  calls makes no row"); the allotment refills from older batches, and a
  placeholder row appears only where history runs out. The generating state
  is held outside the `AGENT_BATCH_CAP` list until `message_end` makes it a
  batch, so a no-tool message never evicts a retained batch.
- **Generating labels**: `thinking`, `replying`, and `<tool>` for tool-call
  streaming. The metric column shows `<N> chars · <elapsed>` for
  tool-call streaming and `<elapsed>` otherwise, elapsed measured from the
  current phase's start, so thinking time and argument-writing time read
  separately. Rejected: localized labels (every widget UI string is English);
  elapsed from the message's start.
- **Several streamed tool calls**: the row shows the call currently streaming,
  and its character count covers that call alone. Rejected: a total across
  the message's calls; the streaming call is what holds the row up, as the
  execution row shows the oldest in-flight call.
- **No args preview while a tool call streams**: the row shows `<tool>` and the
  character count (the sum of `delta` lengths, O(1) per delta); the args
  preview appears once the batch opens at `message_end` and the call executes,
  as today. User decision (19), 2026-10-28. Rejected: buffering a bounded
  prefix of the `delta` text and parsing it with pi-ai's exported
  `parseStreamingJson` at the 1 s render; the user judged the tool name and
  count enough. The relay therefore carries no streaming preview either.
- **Relay shape**: `agent-view` carries the generating phase kind, tool name,
  character count and phase start time, republished under the existing 1 s
  throttle, and a `failed` integer per batch, also on `BatchView`. Rejected: a
  per-call error list, which multiplies message size across up to 128
  descendants. The character count changes every tick; the throttle bounds the
  traffic at the already-accepted rate of about 1 KB per agent per second.
  Supersedes 261028-feat-pi-agent-widget-expanded-mode's "no per-second
  publishes" while a tool call streams. The generating-state update and a
  `scheduleAgentViewPublish()` dirty mark run on the streaming-delta path,
  since a delta classified as streaming skips `observeBatchEvent` and the
  `batchChanged` schedule (`spawner.ts`, `client.onEvent`); the schedule
  returns early while its timer is pending, so the mark stays O(1).
- **No compatibility fallback for the `agent-view` change**: every RPC child
  loads the parent's exact extension entry (expanded-mode ticket).
- **Repaint stays on the existing 1 s expanded tick**, generating phase
  included; no faster spinner timer.
- **Every `message_update` delta** (`text_delta`,
  `thinking_delta`, `toolcall_delta`) is classified as a streaming delta and
  handled with O(1) state updates only. Rejected: leaving the fall-through and
  throttling `refresh` downstream, since the fall-through also republishes the
  subtree per token. The design review confirmed the fan-out by code
  (`isStreamedAssistantOutput`, `spawner.ts:2891-2896`; `message_update` in the
  refresh list, `spawner.ts:3237-3241`). The widened classifier also calls
  `markAgentOutput`, so the header's `(Ns)` activity clock resets while tool
  arguments stream; this matches 260921's "any sign the LLM is producing
  output" and is pinned in a test.
- **A batch closed by `turn_end` / `agent_end` / `agent_settled` with calls
  still unfinished** is the implementer's call (count them failed or not);
  pin the choice in a test.
- **The space indent and batch-row `│` apply to the expanded mode only**;
  normal mode keeps its `│ ` depth lanes on header rows, its only tree cue
  without batch rows. Rejected: the space indent in both modes.
- **Two phases.** Phase 1 is the layout and the `✓` mark, renderer only. Phase 2
  adds the generating phase, the `toolcall_delta` fan-out fix, and the `✗`
  failure marking, including the `agent-view` change.

Agreed sketch (no display summary model; colors in the decisions above):

```text
· worker-a · running · 3m (12s) · opus (high) · 45k · $0.12
│ ✓ read, grep                                    4 calls · 1m ago
│ ✗ edit                               1 call · 1 failed · 20s ago
│ ⠋ bash (cd x && go test ./...)                   1/3 · +1 · 12s
  · grandchild-1 · running · 40s (3s) · sonnet (high) · 12k
  │ ✓ read                                        2 calls · 5s ago
  │ ⠋ thinking                                                 4s
  │ -
```

(Allotment 3 in the sketch: grandchild-1 has two batches, so its third slot
is a placeholder.)

With a display summary, the prose is the summary:
`│ ✓ <summary>                                    4 calls · 1m ago`.

## Constraints

- Runtime pi-tui classes come from the host through `loadHostPiTui()`
  (`src/pi-tui.ts`); a static import of `@earendil-works/pi-tui` is a second
  module instance (repo note `gotcha.pi-tui-dual-package`).
- Agent-driven tmux probes use an isolated server socket
  (`tmux -L ws-probe-<pid>`), never the default one.
- The expanded mode's fixed-height contract stays: every agent keeps its
  allotment of N rows, padded with placeholder rows, and the height changes only when
  an agent enters or leaves, on resize, or when N changes.
- The awaiting-agent fold rule and the `+N more` tail stay as shipped.

## Prior Decisions

- 261028-feat-pi-agent-widget-expanded-mode (2026-10-28, Decisions): "A finished batch row is dimmed; the running batch row is bright." — bearing: constrains
- 260921-feat-pi-gutter-two-clock-semantics (2026-09-21, commit 93a39a73): "Treat streaming deltas as high-water assignments only; defer subtree and telemetry refresh to message boundaries or the periodic gutter cadence, preserving the shared activity clock without render fan-out." — bearing: supports
- 261002-feat-pi-model-output-tps-display (2026-10-02, commit 9d6266dd): "Tracker ignores deltas entirely so the per-delta path is O(1) (Pi awaits extension handlers per delta; prior CPU-saturation and per-token fan-out decisions constrain this)." — bearing: constrains
- 261004-feat-pi-descendant-gutter-detail-parity (2026-10-04, commit c834d5b4): "Coalesce telemetry-only snapshots with one event-driven trailing sampler; streaming deltas merely mark that sampler dirty, while identity, state, teardown and dispatch-fence edges publish immediately." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts, agent-batches.ts, agent-view.ts, spawner.ts, plus their tests |
| scope.surface | cross-module | agent-view relay message between parent and child processes; exported BatchView (agent-batches.ts#L74) |
| scope.new_public_symbol | unknown | ticket names no new exported symbol; generating-phase types are not named |
| scope.new_type_contract | yes | BatchView gains a failed count; agent-view carries phase kind, tool name, char count, phase start |
| scope.test_surface | existing | agents-plugin-pi/test/agent-batches.test.ts, agent-widget.test.ts, agent-view.test.ts |
| complexity.reuse_points | confirmed | observeBatchEvent, batchView, flattenArgsPreview (agent-batches.ts), AGENT_WIDGET_EXPANDED_TICK_MS (agent-widget.ts#L691), SUBTREE_TELEMETRY_INTERVAL_MS (subtree-lifecycle.ts#L257) read |
| complexity.side_effect_risk | high | Phase 2 changes the spawner's per-token client.onEvent hot path (spawner.ts#L3201-L3240) and the agent-view wire shape |
| risk.correctness | moderate | generating-phase state machine over RPC-shaped stream events plus failure counting; Phase 1 is renderer only |
| risk.fit | moderate | column alignment and truncation over host pi-tui in two tuiModes, verified only by tmux probe |
| risk.test | moderate | O(1)-per-delta and live long-write behavior need a probe beyond unit tests |
| risk.security_or_contract | moderate | agent-view message shape changes with no compatibility fallback (same-entry children assumed) |

## Phases

### Phase 1: Batch row layout and finished marks

Apply the layout decisions to the expanded mode's rendering per `## Decisions`:
the batch-row `│`, the space indent for child headers, prose on the left and
metrics right-aligned, `✓` on finished rows, and the `-` placeholder in
unfilled history slots. The batch model and the relay are unchanged. Pin with
unit tests: column alignment of `✓`, the spinner and the `-` placeholder, the
right-aligned metric column at several widths, prose truncating before metrics,
summary versus tool-name prose, nested indentation at depth 2 and 3, and the
unchanged fixed height and fold rule. Check the layout in both `tuiMode`s with
an isolated tmux probe.

### Result (eee08749b) - 2026-10-28

Expanded-mode batch rows now begin with a dim `│` under their agent's
bullet, then a mark column (`✓` finished, the spinner running, a dim `-`
placeholder in each unfilled history slot), the prose left (display summary,
else tool names, else the running call with its args preview) and the
metrics right-aligned at the row's edge (`N calls · Xs ago`;
`<done>/<n> · +<k> · <elapsed>`). Expanded headers indent two spaces per
depth instead of the `│ ` lanes; normal mode is unchanged. Fold padding after
the `+N more` tail stays blank.

Verification: `npm test -- test/agent-widget.test.ts` 103/103 pass (new
tests pin the mark column, right-aligned metrics at 40/60/80/120 columns,
prose truncating before metrics, summary versus tool-name prose, nested
indentation at depth 1-3, narrow-width clipping without split styling, and
the unchanged fixed height and fold rule). Full suite: the same 31
pre-existing failures as the base commit (socket and child-process tests in
this sandbox), no new ones. Isolated tmux probes (`tmux -L ws-probe-<pid>`,
Pi 1.0.4, `--offline --no-session --no-extensions` plus a synthetic-registry
probe extension driving the real controller) rendered the sketch's layout at
depth 0-2 in both regular (90 columns) and fullscreen (70 columns) `tuiMode`.

Decisions: an empty display summary falls back to the tool names; the
running row keeps truncating its args preview inside the parentheses within
the prose budget; when the metrics alone exceed the width they truncate and
the prose is dropped. The old formatter exports (test-only consumers) were
replaced by `formatFinishedBatchMetrics`, `formatRunningBatchMetrics` and
`layoutBatchRowBody`.

### Phase 2: Generating phase and failure marks

Open the running batch row at the assistant message's start and drive its
generating phase from `message_update` events; add the per-batch failure count
and the `✗` row; carry both over `agent-view` to descendants. Fix the
confirmed `toolcall_delta` fan-out. Pin with unit tests: the phase transitions (thinking, reply text,
tool-call streaming, execution, finished), a no-tool message leaving no row,
the tool name and character count from `toolcall_start` / `toolcall_delta`
events shaped as the RPC stream delivers them (no `partial`), the failure
count and `✗` row, the `agent-view` parse and hop merge of the new fields, and
O(1) handling per delta (feeding N `toolcall_delta` events triggers no
`publishSubtree` or `refresh` call, while the character count still reaches
the widget and the relay). Check a live child doing a long `write` with an
isolated tmux probe.
