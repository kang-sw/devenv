---
title: "agy 서브에이전트 출력 속도(TPS) 표시"
related:
  261119-feat-ws-agy-plan-usage-display: "sibling agy dashboard work; plan quota display is separate from per-response output rate"
  261118-refactor-ws-split-agy-adapter: "moves agy.go code; locate this ticket's agy.go anchors by symbol"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: db2200bf23d1af37
sage-review-completeness-reviewed: db2200bf23d1af37
completed: 2026-11-20
---

# agy subagent output rate (TPS)

## Background

The owner asked whether agy token streaming and counts can support a TPS value
in the subagent view. The ACP stream supplies message deltas and a completed
prompt's exact output- and thought-token counts. Expose a completed-response
visible-text rate in the parent's subagent row and that session's hover card.

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
  `thoughtTokens` is a subset of `outputTokens`; do not add it again. Thought
  tokens dominate: `agy-1.5.9-subagent-removed.jsonl:14` has 1023 output with
  988 thought tokens over two message chunks (lines 11-12), line 18 has 175
  with 169, and the compaction probe has 60 of 69 thought tokens with no
  thought chunk streamed. `agent_thought_chunk` updates look like occasional
  summaries, not a token stream, so their receipt times do not measure
  thinking time.
- Line anchors into `agy.go` will go stale:
  `261118-refactor-ws-split-agy-adapter` moves its code and Phase 3 of
  `261118-feat-ws-agy-tool-names-and-worker-role` edits it. Find code by
  symbol (`endTurn`, `agyMapper.update`) rather than by line.
- Existing `usage_update {used,size}` maps to `wslive.UsageContext`, not
  response output tokens (`agy.go:1221-1235`, `runner_conv.go:935-951`).
- Prior art: Pi measures from first `message_update` to `message_end`, requiring
  at least 250 ms and 8 output tokens before emitting a
  `KindStatus.OutputTps` rate (`wsagent/pi.go:924-938,1203-1208,1286-1306`).
  Claude uses a token-weighted window of up to eight eligible requests
  (`agents-plugin/hooks/claude-mod/dashboard.ts:528-619`). The Pi extension's
  tracker, when the provider reports a reasoning breakdown, excludes both the
  reasoning tokens and the thinking spans and applies the 8-token threshold to
  the counted tokens (`agents-plugin-pi/src/output-rate.ts:1-22,56-75`).
- The subagent row shows a rate only while the child runs:
  `conv.state` copies `c.outputTps` into `State.OutputTps` only while
  `c.turn && !c.closed` (`runner_conv.go:689-691`), and `AgentNode.Rate()`
  returns 0 for a row that is not running (`wsdashboard/pageview.go:310-315`).
  An agy rate arrives with the prompt result that `endTurn` handles as the
  turn's end, so the row shows the previous response's rate during a later
  turn and none during a subagent's first or only turn.
- `endTurn` ends the turn only at the last prompt's answer
  (`agy.go:1084ff`); with steering, earlier `session/prompt` results arrive
  mid-turn.
- `sessionHover` shows a rate while working only from `Observed.Current` with
  `At >= TurnStartedAt`, and while idle only from `LastTurnOutput`
  (`pageview_hover.go:39-49`); both are `ModelObservation` values. The
  `ObservedDirName` comment in `wslive/observed.go` records that older typed
  writers replace `snapshot.json` and drop unknown fields.
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

- Completed-response TPS comes from the exact ACP `session/prompt` result usage
  (token formula as re-confirmed in `(1410)` below) and the local receipt span of that response's message deltas, excluding
  tool-execution intervals. Use a small rolling window over the last few
  eligible responses. During generation, retain the previous value. Display it
  in both the parent's subagent row and the session hover card.
- `(1410)` reopened and re-confirmed `a`, owner 2026-11-19 23:25 KST: the rate
  is a visible-text rate, `(outputTokens - thoughtTokens)` over the
  message-delta receipt span. The `(1412)` 8-token eligibility applies to these
  visible tokens, so a mostly-thinking response is ineligible. Precedent: the
  reasoning-breakdown branch of `agents-plugin-pi/src/output-rate.ts`.
  - Rejected: `outputTokens` over a first-update-to-result span; its start is
    unreliable and thinking time is still undercounted.
  - Rejected: keeping all `outputTokens` and excluding responses with
    `thoughtTokens > 0`; the rate would almost never appear.
  - Rejected: a live estimate from text-delta length while generating; the
    token/character ratio varies by model and language and can err widely.
  - Rejected: bundling Codex subagents; Codex token events need separate
    research and a later ticket.

Owner-confirmed board `(1411)`, `(1412)`, 2026-11-19:

- `(1411)` The hover card shows agy TPS through a rate record independent of
  `ModelObservation`; it carries no observed model and no observation source.
  Compatibility target is T1: an old daemon keeps showing "—" for the hover
  rate, and a new daemon reads the new rate data, including on reopen. The
  implementation traces and records the old-reader, new-writer, and reopen
  paths as evidence.
  - Rejected: fabricating an observed model, or admitting an agy observation
    source without answering-model evidence.
- `(1412)` Reuse the existing thresholds: a completed response is eligible
  only when its measured span is at least 250 ms and its visible tokens
  (`(1410)`) are at least 8. The displayed rate is the token-weighted average
  (total counted tokens over total span) of at most the last 8 eligible responses, following the Pi
  250 ms/8-token and Claude up-to-8 window precedents.
  - Rejected: inventing new threshold or window values without data.

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
  `wsdashboard/pageview.go:308-315`). The separate, additive hover rate record
  of `(1411)` targets T1: an old daemon displays "—" without changing the new
  rate data, and a new daemon reads that data on reopen. Existing observation
  intake and pruning reject unknown evidence sources without rewriting them
  (`wslive/observed.go:113-126`, `observed_prune.go:42-48,104-114`); do not
  force agy rate through `ModelObservation`. Claim T1 only after the actual
  writer, old reader, and reopen path are traced, and record that trace in the
  phase `### Result`. Preserve a patch-safe tier.
- `ai-docs/manuals/shipped-surface-boundary.md`: any UI label or status text
  stays downstream-neutral.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)

## Prior Decisions

- 14468d0e0 (2026-11-15, commit): "Existing Lead has no observed model/effort/rate; show unknown output identity and unavailable rate rather than invent aliases, inherit a child's model, or read requested hosted settings." — bearing: constrains
- 15c0409b1 (2026-11-17, commit): "State.Apply now hands durable storage ... just the records that change an agent's persisted fold: the first record, a model change point, the new latest ... Repeats stay in memory only." — bearing: constrains
- aa9da6ac2 (2026-11-17, commit): "A hosted copy whose writer folded the whole evidence directory is served with no evidence I/O; any other copy, a shared one, and the file still overlay the directory." — bearing: constrains
- d567abd01 (2026-11-13, commit): "fix(wsagent): populate hosted child and tool-batch token usage" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/agy.go, wsagent/runner_conv.go, wslive/ (hover rate record), wsdashboard/pageview_hover.go |
| scope.surface | cross-module | wsagent -> wslive -> wsdashboard; new hover rate record crosses the wslive snapshot boundary (wslive/snapshot.go) |
| scope.new_public_symbol | unknown | the (1411) hover rate record is unnamed in the ticket; any new wslive snapshot field or type would be exported |
| scope.new_type_contract | yes | new additive hover rate record, T1 old-reader/new-writer/reopen contract per (1411) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/agy_fixture_test.go, agy_test.go; wslive/observed_test.go; wsdashboard/pageview_hover_test.go |
| complexity.reuse_points | confirmed | KindStatus.OutputTps row path (runner_conv.go:925-951, wslive/state.go:705-719, wsdashboard/pageview.go:308-315) and Pi tracker (wsagent/pi.go:1203-1208) read |
| complexity.side_effect_risk | moderate | touches the shared conv.apply/State telemetry path and a new persisted-or-live hover record read by old and new daemons |
| risk.correctness | moderate | tool-interval exclusion, steered multi-prompt turns, and the 250 ms / 8-token boundaries are new span logic over local receipt times |
| risk.fit | moderate | hover record must stay independent of ModelObservation (wslive/observed.go:113-126 rejects unknown sources); agy.go split ticket may move anchors |
| risk.test | moderate | recorded fixtures carry no receipt timestamps, so span tests rely on synthetic times; a dashboard view capture is also required |
| risk.security_or_contract | high | T1 compatibility claim needs a traced old reader, new writer and reopen path; wrong tier breaks patch-safe release |

## Phases

### Phase 1: Capture completed agy output rate and render both views

- Each `session/prompt` result, matched by its JSON-RPC id, closes exactly one
  response, including the earlier prompt results of a steered turn; each
  opens its own receipt-span window and publishes on its own completion. When
  a steered prompt is sent before the earlier prompt's result, a message chunk
  belongs to the oldest prompt still without a result.
- Parse `usage.outputTokens` and `usage.thoughtTokens` from the ACP prompt
  result; the counted tokens are `outputTokens - thoughtTokens` (`(1410)`), an
  absent `thoughtTokens` counting as 0. A missing or non-numeric
  `outputTokens`, or a negative difference, makes the response ineligible.
- The span is the sum of message-delta runs. A run is a maximal sequence of
  consecutive `agent_message_chunk` updates for the response with no other
  session update (thought chunk, tool-call update, or any other kind) between
  them; its length is its last chunk's receipt time minus its first. So
  latency before the first message delta, thought-chunk time, and every gap
  around a tool call, from the last message delta before a tool-call update to
  the first message delta after it, stay out of the span. A run of one chunk
  spans 0; this reads somewhat high, as the Claude hooks tracker notes for the
  same estimate (`agents-plugin/hooks/claude-mod/dashboard.ts`).
- Treat responses without a measurable eligible span or valid token count as
  ineligible. A thought-only response has no message-delta span. A response
  is eligible only with a span of at least 250 ms and at least 8 counted
  tokens (`(1412)`). Do not infer token counts from text length.
- Hold an in-memory window of at most the last 8 eligible completed responses
  and emit `KindStatus.OutputTps` as their token-weighted rate on eligible
  completion. Keep the previous rate during generation; ineligible
  completions do not publish a new rate. Clear the window when the session's
  model config option changes (`config_option_update`, for example
  `agy-1.5.10-hosted-lead.jsonl:77,85`), as the Pi tracker clears on a model
  change (`agents-plugin-pi/src/output-rate.ts`, `finish`). Clearing the
  window publishes nothing: the row and hover keep the last published rate
  until the next eligible response.
- Carry the rate through the existing parent subagent row path unchanged: the
  row shows a rate only while the child runs, so it shows the previous
  response's rate during a later turn and none during a first or only turn.
- Add the `(1411)` hover rate record, independent of `ModelObservation`, that
  asserts no observed model, and show it on the agy session's own hover card.
  `sessionHover` needs its own selection branch for it that keeps the previous
  value during generation. The record must survive reopen; note that older
  typed writers replace `snapshot.json` and drop unknown fields, so a
  `snapshot.json` field would not hold under an old daemon. Trace the
  old-daemon reader, the new writer, and the reopen path, and record that T1
  evidence in the `### Result`.
- Verify with fixture-driven Go tests over the recorded agy ACP fixtures and
  synthetic local receipt times: the exact visible-token rate (fixture
  `outputTokens - thoughtTokens`), the 250 ms and 8-token thresholds at
  their boundaries, a mostly-thinking response (for example 175 output with
  169 thought tokens) staying ineligible, run splitting at thought and
  tool-call updates, tool-interval exclusion, the
  8-response token-weighted window and its reset on a model change, no new
  `OutputTps` on
  ineligible completion, retention of the prior value during generation and
  across a model-change reset, and one window per prompt result in a steered
  turn, with chunk attribution to the oldest unanswered prompt.
  Cover row and hover rendering, including an unknown observed model. Use
  focused commands such as `go test -run 'TestAgy.*(Output|Rate)' ./internal/wsagent`,
  `go test -run 'Test.*(OutputTps|Rate)' ./internal/wslive`, and
  `go test -run 'Test.*(Hover|AgentRate)' ./internal/wsdashboard` from
  `agents-plugin-tool/`; adjust patterns to the actual test names. Capture a
  dashboard view with a nonempty `출력 속도` for an agy subagent during its
  second or later turn. Fixture responses often carry only a few visible
  tokens (`agy-1.5.10-hosted-lead.jsonl:52`: 27 output, 25 thought), so prompt
  the subagent for a deliberately long visible reply if needed and say so in
  the `### Result`; do not loosen the thresholds.

### Result (0274c5f7) - 2026-11-20

- **What landed (`d0fe8445`, `0274c5f7`):**
  - `agents-plugin-tool/internal/wsagent/agy_output_rate.go`, `agy.go`: added in-memory per-prompt span tracking (`agyOutputRateTracker`) matching `session/prompt` results by JSON-RPC id, attributing `agent_message_chunk` updates to the oldest unanswered prompt, splitting runs on any non-`agent_message_chunk` update or permission request, computing visible tokens as `outputTokens - thoughtTokens` (absent/null `thoughtTokens` = 0; rejecting non-numeric, negative, or non-`end_turn` results), enforcing the 250 ms / 8 visible-token thresholds and `MaxOutputTps` outlier guard before mutating the 8-sample token-weighted rolling window, silently clearing the window on model `config_option_update` (not `thought_level`), dropping untracked spans on unsolicited-turn prompt takeover / `closeUnsolicited` / `endTurn`, ignoring replayed spans, and preserving `lastContext` on rate `KindStatus` events.
  - `agents-plugin-tool/internal/wslive/output_rate.go`, `snapshot.go`, `state.go`: added additive `OutputRate{OutputTps, At}` independent of `ModelObservation` (carrying no observed model or source), atomic `<sessionDir>/output-rate.json` persistence, `State.SeedOutputRate` / `State.setOutputRate` (ignoring older or unchanged values), `OverlayOutputRate`, and 1 s-validated negative-caching `OutputRateCache`.
  - `agents-plugin-tool/internal/wsagent/record.go`, `runner.go`, `internal/mcp/live_snapshot.go`, `internal/wsdashboard/snapshot_cache.go`, `pageview_hover.go`: seeded `OutputRate` once at startup/reopen from `output-rate.json` (with `runner.go` `startConv` restoring `c.outputTps`), persisted only on eligible changed completions, served hosted `foldedCopy` snapshots and sessions with `Lead.Observed != nil` with zero `output-rate.json` disk I/O, and added `sessionHover`'s `Lead.OutputRate` fallback branch retaining the rate across generation and idle while keeping `Observed` unknown (`nil`).
- **Decisions:**
  - Persisted the T1 hover rate in `<sessionDir>/output-rate.json` rather than an existing file: no existing `SessionDir` file read by `wsdashboard` preserves unknown fields across old-daemon rewrites (`snapshot.json` is replaced wholesale by typed writers, `model-evidence/` is restricted to answering-model `ModelObservation`, and `wsagent`'s `record.json` is not read by `wsdashboard` or present for standalone MCP sessions). Live state stays in RAM; `output-rate.json` is written at most once per eligible changed completion and loaded once at startup/reopen.
  - Under `code-review-lite`, fixed 1 critical (dropping live unsolicited-turn untracked spans on `promptSent` takeover and `endTurn` so subsequent tracked prompts are not starved) and 1 minor (rejecting `> MaxOutputTps` outlier samples before inserting them into the 8-sample window) in `0274c5f7`.
- **Compatibility trace (T0 row / T1 hover):**
  - *Row path (T0):* `agyMapper.response` (`wsagent/agy.go`) emits `KindStatus.OutputTps` -> `conv.apply` (`wsagent/runner_conv.go`) stores `c.outputTps` -> `conv.state` exposes `State.OutputTps` while `c.turn && !c.closed` -> `SpawnAgentEvent.OutputTps` (`wsagent/spawn_live.go`) -> `wslive.State.Apply` (`wslive/state.go`, `Agent.OutputTps`) -> `AgentNode.Rate()` (`wsdashboard/pageview.go`). Existing optional field; unchanged wire contract.
  - *Hover path (T1) — new writer:* `agyMapper.response` (`wsagent/agy.go`) emits `KindStatus.OutputRate` -> `wslive.State.Apply` -> `State.setOutputRate` (`wslive/output_rate.go`) updates in-memory `State.outputRate` (published on `Snapshot.Lead.OutputRate` / `HostedSlot.Publish`) and emits `Result.OutputRate` only when changed and monotonic -> `liveWriter.writeOutputRate` (`wsagent/record.go`) and `mcp.liveSession` (`mcp/live_snapshot.go`) atomically write `<sessionDir>/output-rate.json` via `wslive.WriteOutputRate` (`outputTps` and `at` only; no `model`/`source`).
  - *Hover path (T1) — old reader / writer:* Old `wslive.ReadSnapshot` (`wslive/snapshot.go`) ignores unknown `lead.outputRate`; old `sessionHover` (`wsdashboard/pageview_hover.go`) inspects only `Observed`/`LastTurnOutput` (both `nil` for agy) and renders `—` with unknown observed model. Old snapshot writers (`wsagent/record.go`, `mcp/live_snapshot.go`) overwrite `snapshot.json` dropping `lead.outputRate`, and old `pruneObserved` (`wslive/observed_prune.go`) scans only `model-evidence/`, leaving `output-rate.json` intact.
  - *Hover path (T1) — reopen path:* New `newLiveWriter` (`wsagent/record.go`), `mcp.liveHub.session` (`mcp/live_snapshot.go`), and `SnapshotCache.snapshotResult` (`wsdashboard/snapshot_cache.go`, via `OutputRateCache` when not a `foldedCopy` and `Lead.Observed == nil`) read `output-rate.json` (`wslive.ReadOutputRate`) and restore `Lead.OutputRate` via `SeedOutputRate` / `OverlayOutputRate` (and `runner.go` `startConv` restores `c.outputTps`) while leaving `Lead.Observed == nil`.
- **Verification:**
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -v -run 'TestAgy.*(Output|Rate)' ./internal/wsagent` — PASS (8 tests including recorded-fixture visible-token counts, boundary/ineligible cases, tool/thought run splitting, 8-sample window + model-change reset, steered multi-prompt attribution, row/hover retention + reopen after legacy snapshot rewrite, unsolicited-turn prompt takeover, and outlier guard).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -run 'TestAgy' ./internal/wsagent` — PASS.
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -v -run 'Test.*(OutputTps|OutputRate|Rate)' ./internal/wslive` — PASS (5 tests including T1 sidecar survival across legacy snapshot rewrite and `pruneObserved`, and `OutputRateCache` validation/negative cache).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -v -run 'TestSessionHoverAgyOutputRateAndUnknownModel|TestAgentRowStats' ./internal/wsdashboard` — PASS.
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -v -run 'TestObservedIntakePersistsAcrossReconnectAndLegacySnapshotRewrite|TestDashboardIntakeWritesSnapshotAndRing' ./internal/mcp` — PASS.
  - Fixture-driven Playwright Chromium capture (`TestAgyOutputRateHoverAndRowBrowser` with `testdata/agy-output-rate.browser.cjs`) — PASS (`0.89s`). A live dashboard capture against the shared daemon is not possible without a daemon rebuild the lead does not own, so this fixture-driven render drove the dashboard against the recorded `agy-1.5.9-subagent-removed.jsonl` response (1023 output / 988 thought = 35 visible tokens) with synthetic 500 ms receipt times (70 t/s) without loosening the 250 ms / 8-token thresholds, verifying both the parent subagent row (`70 t/s`) and the child hover card (`출력 속도: 70`, `출력에서 확인된 모델: 알 수 없음`) during its second turn.
