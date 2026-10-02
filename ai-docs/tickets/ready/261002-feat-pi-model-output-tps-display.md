---
title: "Pi model output TPS in the lead footer and subagent gutter"
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 1c0ea303f9bf65a1
---

# Pi model output TPS in the lead footer and subagent gutter

## Background

The Pi lead footer (`agents-plugin-pi/src/agent-footer.ts`) and the subagent
gutter widget (`agents-plugin-pi/src/agent-widget.ts`) show token counts,
context, and cost, but nothing shows how fast the model is producing output.
The user wants an approximate tokens-per-second figure per agent. A naive
`output tokens / wall time` is badly low: wall time includes tool execution,
request latency and prefill before the first block, and opaque reasoning
time. The figure must count only the time the model is visibly streaming.

Nothing in the repo measures timing or token rate today (no `tps`, rate, or
TTFT code). Evidence gathered from the installed Pi packages
(`@earendil-works/pi-ai` / `pi-coding-agent` 0.84.4 in `node_modules`, global
0.85.1 with an identical event union):

- `message_update` carries an `assistantMessageEvent` from the union in
  `pi-ai/dist/types.d.ts` (`text_start/_delta/_end`,
  `thinking_start/_delta/_end`, `toolcall_start/_delta/_end`). No delta
  carries a timestamp; the receiver must take `Date.now()` on receipt.
- `Usage.output` already includes reasoning tokens; `Usage.reasoning`
  (optional) is that subset when the provider reports a breakdown. For
  Anthropic and OpenAI, final output usage arrives only at message end.
- Opaque reasoning surfaces as a `thinking_start` ... `thinking_end` bracket
  with no deltas (OpenAI encrypted reasoning) or summary-only deltas
  (Anthropic summarized/redacted thinking), so reasoning time cannot be paired
  with reasoning tokens reliably.
- Tool-call arguments are output tokens and stream as `toolcall_delta`.
- Lead: extensions observe their own session's deltas through
  `pi.on("message_update", ...)`; handlers are awaited per delta.
- Direct children: each is a `pi --mode rpc` process whose full event stream
  (including every `message_update`) already reaches the lead's
  `client.onEvent` listener in `spawner.ts`. Today that listener uses
  `text_delta`/`thinking_delta` only to stamp `lastOutputAt` and ignores
  `toolcall_*`. Grandchildren are not visible per delta.
- Footer usage today is fed only by `pi.on("message_end", ...)` in
  `src/index.ts`; the gutter row is formatted by `formatRow` in
  `src/agent-widget.ts`, whose `model (effort) · ctx · $` telemetry group is
  appended all-or-nothing by width.

## Decisions

- **D1 - Metric: visible-output decode rate.** Per completed assistant
  message:
  - when `usage.reasoning` is defined: tokens = `usage.output -
    usage.reasoning`; time = sum of the message's `text_*` and `toolcall_*`
    block spans.
  - when `usage.reasoning` is undefined: tokens = `usage.output`; time = sum of
    all block spans, `thinking_*` included.
  - A block span runs from its `*_start` receipt to its matching `*_end`
    receipt (`Date.now()` on receipt), keyed by `contentIndex`. Spans are
    summed per block, never first-delta-to-last-delta, so gaps between blocks,
    pre-first-block latency, and tool execution between messages never count.
  - A message whose counted block has a `*_start` with no matching `*_end`
    by `message_end` (e.g. an event lost through the RPC pipe) is
    ineligible.
  - Rejected: including reasoning tokens and opaque thinking brackets in the
    rate (whole-generation rate). Whether a no-delta thinking bracket tracks
    real reasoning generation time is unverified per provider, and the user
    only needs an approximate figure.
- **D2 - Aggregation: token-weighted window of the last 8 eligible messages
  per agent.** Rate = sum of tokens / sum of time over that agent's last 8
  eligible completed assistant messages; never an average of per-message
  rates.
  - The window resets when the measured message's model differs from the
    previous eligible message's model (the lead can switch models mid-session).
  - Messages ending in error or abort are ineligible, read from the
    `message_end` message's `stopReason` (`"error"` / `"aborted"`). The model
    identity compared for the reset is that message's `provider` + `model`,
    on both the lead and the RPC child path.
  - A message is also ineligible when its counted span is under 250 ms or its
    counted tokens are under 8; an ineligible message neither enters nor
    resets the window. The span guard catches burst-flushed tool-call
    arguments (e.g. OpenAI `function_call_arguments.done`, SDK read batching)
    whose near-zero span would inflate the rate; the token guard drops empty or
    near-empty messages. Rejected: no threshold (token weighting alone does not
    protect a window of short tool-call messages); a span-only threshold.
  - Rate state is in memory only and starts empty after a lead restart or a
    child revive: spans exist only as receipt times of live deltas, and stored
    session entries carry no per-block timing.
  - The displayed value changes only at `message_end`; there is no live
    character-based estimate while a message streams.
  - Rejected: whole-session cumulative average (slow to reflect model or load
    changes); live char-based estimate (not needed for a rough figure).
- **D3 - Measurement sites.**
  - The lead measures its own session in-process from `pi.on("message_update")`
    and `pi.on("message_end")`.
  - The lead measures each direct child from the existing RPC event stream
    listener in `spawner.ts`, which must start handling `toolcall_*` events.
    Receive-time skew through the RPC pipe is accepted.
  - Excluded: grandchildren and other liveness-only rows, and synthetic
    thread rows with no backing RPC record.
  - Per-delta handlers stay O(1): Pi awaits extension handlers per delta, so a
    slow handler stalls streaming.
- **D4 - Footer.** Append the lead's rate right after the `↓output` token
  part, as in `↓12.3k ~42t/s`. Omitted until the first eligible message.
- **D5 - Gutter.** A separate segment right after `model (effort)`:

  ```text
  … · gpt-5.5 (high) · 42t/s · 45.2k · $0.123
  ```

  - No `~` in the gutter.
  - Omitted until the agent's first eligible message.
  - When the row is too narrow, the TPS segment is dropped first and the rest
    of the telemetry group is kept; only then does the existing all-or-nothing
    rule for the remaining group apply.
  - Rejected: TPS inside the effort parentheses (`(high, 42t/s)`, mixes two
    meanings); TPS at the row end after cost (less visible).

- **D6 - Number format.** Rates display as integers rounded to nearest. The footer form is
  `~42t/s` (no space, same unit as the gutter); the gutter form is `42t/s`.

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` applies to the
  shipped Pi package text this ticket changes.
- The TPS segment's theme color is the implementer's choice among existing
  theme tokens; styling is applied after width truncation, like the rest of
  `formatRow`.

## Verification

- Unit tests on the pure rate aggregator: `usage.reasoning` defined and
  undefined; a no-delta thinking bracket; interleaved thinking/text/toolcall
  blocks; error and abort skip; model-change reset; the 8-message window;
  both eligibility guards.
- `formatRow` width tests: at a width that fits everything but the TPS
  segment, the row keeps `model (effort) · ctx · $` and drops only TPS; the
  segment is absent before the first eligible message.
- A footer presentation test showing `↓<output> ~Nt/s` placement and
  omission before the first eligible message.

## Prior Decisions

- 260921-feat-pi-gutter-two-clock-semantics (2026-09-21, Result): "Stream deltas update only that field and defer subtree observation, publication, and telemetry refresh, preventing local and parent-watcher gutter fan-out per token." — bearing: constrains
- 260913-bug-ws-pi-cost-footer-cpu-saturation (2026-09-13, Result): "The synchronous 250 ms descendant scan and render-time history traversal were removed because both ran on Pi's main thread and reproduced sustained CPU saturation." — bearing: constrains
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20, commit ab8b02f6): "omit clocks and telemetry for propagated rows because the Phase-1 transport does not carry those facts" — bearing: supports
- 260915-bug-ws-pi-widget-context-value-removed (2026-09-15, Phase 1): "Kept a dedicated unlabeled live-row formatter rather than changing formatContextTokens so /audit's existing ctx Nk and ctx ? contract remains unchanged." — bearing: constrains
- 260912-feat-ws-pi-custom-footer-cost-telemetry (2026-09-13, Result): "The built-in footer cannot be extended field-by-field, so the adapter owns a complete replacement while leaving the independent belowEditor agent widget untouched." — bearing: supports
- 1d10d694 (2026-09-10, commit): "The owner requested compact live gutter telemetry after dogfood exposed raw token counts and floating-point cost tails." — bearing: constrains
- d1eeaf98 (2026-09-10, commit): "The owner requested visually distinct model and estimated-cost telemetry in the live agent gutter." — bearing: supports
- 7d70ff13 (2026-03-24, commit): "tok/s uses total_output_tokens (includes thinking) / total_api_duration_ms" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-footer.ts, agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/index.ts, plus a new pure aggregator module |
| scope.surface | cross-module | AgentRow in agent-widget.ts gains a rate field fed from spawner.ts records; footer presentation in agent-footer.ts fed from index.ts lifecycle; package-internal, not ws-mcp surface |
| scope.new_public_symbol | yes | exported pure rate aggregator for unit testing; name not fixed by the ticket |
| scope.new_type_contract | yes | AgentRow gains an optional TPS field; aggregator state and input types are new |
| scope.test_surface | new-files | aggregator unit tests need a new test file; agents-plugin-pi/test/agent-widget.test.ts and agents-plugin-pi/test/agent-footer.test.ts extended |
| complexity.reuse_points | confirmed | existing client.onEvent listener and isStreamedAssistantOutput in spawner.ts#L2598-L2604 and #L2900-L2925; formatRow telemetry group agent-widget.ts#L517-L556; footer tokenParts agent-footer.ts#L161-L167; pi.on message_end index.ts#L561 |
| complexity.side_effect_risk | moderate | per-delta handlers on the lead and the RPC listener run on Pi's main thread; a non-O(1) handler stalls streaming, per prior CPU-saturation and per-token fan-out decisions |
| risk.correctness | moderate | span pairing by contentIndex across interleaved blocks, reasoning-subtraction branch, eligibility guards, and model-change reset all affect the displayed figure |
| risk.fit | moderate | formatRow width logic must drop TPS first before the existing all-or-nothing telemetry rule, and styling must stay after truncation |
| risk.test | moderate | timing depends on Date.now receipt; tests need an injectable clock to cover spans deterministically |
| risk.security_or_contract | low | display-only in-memory state; no persisted format, MCP schema, or cross-process protocol change |

## Phases

### Phase 1: Measure and display output TPS

Implement D1-D6 in `agents-plugin-pi`: a pure aggregator fed by the lead's
own `message_update`/`message_end` events and by each direct child's RPC
events, surfaced in the footer presentation and in `AgentRow`/`formatRow`.
Done when the `## Verification` tests pass.
