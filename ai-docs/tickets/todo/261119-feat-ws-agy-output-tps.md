---
title: "agy 서브에이전트 출력 속도(TPS) 표시"
related:
  261119-feat-ws-agy-plan-usage-display: "sibling agy dashboard work; plan quota display is separate from per-response output rate"
---

# agy subagent output rate (TPS)

## Background

The owner asked whether agy token streaming and counts can support a TPS value
in the subagent view. The ACP stream supplies message deltas and a completed
prompt's exact output-token count. Expose a completed-response rate in the
parent's subagent row and that session's hover card.

## Verified Findings

- ws launches `agy acp --process-per-session`, JSON-RPC 2.0 over stdio
  (`agents-plugin-tool/internal/wsagent/agy.go:21-24,45-47,131-164`).
- `agyMapper` receives `agent_message_chunk`, `agent_thought_chunk`, tool-call
  updates, and `usage_update`; each line gets local receipt time from
  `time.Now().UnixMilli()` (`agy.go:797-804,1132-1247`). The recorded ACP
  notifications have neither provider timestamps nor per-delta token counts.
- Prompt results carry `usage.outputTokens` alongside `inputTokens`,
  `cachedReadTokens`, `cachedWriteTokens`, `thoughtTokens`, and `totalTokens`,
  but `endTurn` currently unmarshals only `stopReason`
  (`agy.go:1084-1095`;
  `agents-plugin-tool/internal/wsagent/testdata/agy-1.5.9-subagent-removed.jsonl:14,18`;
  `agy-1.5.12-compaction-probe-1.jsonl:11,14,18`). In these fixtures,
  `thoughtTokens` is a subset of `outputTokens`; do not add it again.
- Existing `usage_update {used,size}` maps to `wslive.UsageContext`, not
  response output tokens (`agy.go:1221-1235`, `runner_conv.go:935-951`).
- Prior art: Pi measures from first `message_update` to `message_end`, requiring
  at least 250 ms and 8 output tokens before emitting a
  `KindStatus.OutputTps` rate (`wsagent/pi.go:924-938,1203-1208,1286-1306`).
  Claude uses a token-weighted window of up to eight eligible requests
  (`agents-plugin/hooks/claude-mod/dashboard.ts:528-619`). These are precedents,
  not approvals for agy's exact thresholds or window size.
- The existing row path is `KindStatus.OutputTps` -> `conv.apply` ->
  `State.OutputTps` -> `SpawnAgentEvent.OutputTps` -> `wslive` agent state ->
  `Snapshot.Agent.OutputTps` -> `AgentNode.Rate()`
  (`runner_conv.go:925-951`, `runner.go:1455-1458`, `spawn_live.go:30-34`,
  `wslive/state.go:705-719,1052-1073`, `wsdashboard/pageview.go:308-315`).
- The hover rate currently comes from `Lead.Observed.Current.OutputTps` or
  `Lead.LastTurnOutput.OutputTps` (`wsdashboard/pageview_hover.go:34-48`).
  `ModelObservation` means answering-model evidence and admits only
  `claude.step`, `claude.assistant`, and `pi.assistant`; it also requires a
  nonempty model (`wslive/observed.go:19-27,113-146`). Agy has no observed
  answering-model field in the recorded response stream. Its configured model
  must not be presented as observed.

## Decisions

Owner-confirmed board `(1410)=a`, 2026-11-19:

- Completed-response TPS is exact ACP `session/prompt` result
  `usage.outputTokens` divided by the local receipt span of that response's
  message deltas, excluding tool-execution intervals. Use a small rolling
  window over the last few eligible responses. During generation, retain the
  previous value. Display it in both the parent's subagent row and the session
  hover card.
  - Rejected: a live estimate from text-delta length while generating; the
    token/character ratio varies by model and language and can err widely.
  - Rejected: bundling Codex subagents; Codex token events need separate
    research and a later ticket.

## Constraints

- The hover's "model observed in output" stays unknown for agy. Its response
  stream does not identify the answering model, and configured model is not
  observation evidence.
- `ai-docs/manuals/performance.md`: compute receipt spans and the rolling
  rate in memory; add no file I/O to interactive paths. Measure file operations
  per affected action if the implementation changes any such path.
- `ai-docs/manuals/release-compatibility.md`: verify the directional tier per
  changed surface. The existing `KindStatus.OutputTps` to subagent row path is
  T0: current readers already accept this optional rate and old data stays
  readable (`runner_conv.go:948-951`, `wslive/state.go:717-719`,
  `wsdashboard/pageview.go:308-315`). For a separate, additive hover rate
  record, T1 is an *inference*, pending the representation in `(1411)`: the
  intended direction is that an old daemon displays "—" without changing the
  new rate data, and a new daemon reads that data on reopen. Existing
  observation intake and
  pruning reject unknown evidence sources without rewriting them
  (`wslive/observed.go:113-126`, `observed_prune.go:42-48,104-114`); do not
  force agy rate through `ModelObservation` or claim T1 until the actual
  writer, old reader, and reopen path are traced. Preserve a patch-safe tier.
- `ai-docs/manuals/shipped-surface-boundary.md`: any UI label or status text
  stays downstream-neutral.

## Open Decision Queue

Last ID used: (1412)

(1411) [open] [technical] Represent agy hover TPS in a rate record independent of `ModelObservation` and verify the inferred T1 tier from its old/new/reopen paths, over fabricating an observed model or source - the ACP result has no answering-model evidence and existing observation intake rejects it.

(1412) [open] [technical] Choose the exact small rolling-window size and sample eligibility thresholds for agy, using Pi/Claude's 250 ms, 8-token, up-to-eight precedents as candidates, over hard-coding those precedents as owner decisions - the owner confirmed only a small window of eligible completed responses.

## Phases

### Phase 1: Capture completed agy output rate and render both views

- Parse `usage.outputTokens` from the final ACP prompt result. Track the local
  receipt times of message deltas for the corresponding response and exclude
  intervals while tool calls execute. Treat responses without a measurable
  eligible span or valid output count as ineligible. A thought-only response
  has no message-delta span. Do not infer token counts from text length or
  count `thoughtTokens` twice.
- Hold a rolling in-memory window of eligible completed responses and emit
  `KindStatus.OutputTps` on eligible completion. Keep the previous rate during
  generation; ineligible completions do not publish a new rate.
- Carry the rate through the existing parent subagent row path. Add a hover
  rate path that does not assert an observed model; settle `(1411)` and `(1412)`
  before promoting this ticket to `ready/` and record their answers and
  compatibility evidence.
- Verify with fixture-driven Go tests over the recorded agy ACP fixtures and
  synthetic local receipt times: exact result-token rate, eligibility
  thresholds, tool-interval exclusion, rolling window, no new `OutputTps` on
  ineligible completion, and retention of the prior value during generation.
  Cover row and hover rendering, including an unknown observed model. Use
  focused commands such as `go test -run 'TestAgy.*(Output|Rate)' ./internal/wsagent`,
  `go test -run 'Test.*(OutputTps|Rate)' ./internal/wslive`, and
  `go test -run 'Test.*(Hover|AgentRate)' ./internal/wsdashboard` from
  `agents-plugin-tool/`; adjust patterns to the actual test names. Capture a
  dashboard view with a nonempty `출력 속도` for an agy subagent.
