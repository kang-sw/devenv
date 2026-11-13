---
title: "Hosted sessions show no token usage on Pi rows or tool-batch lines"
completed: 2026-11-13
---

# Hosted sessions show no token usage on Pi rows or tool-batch lines

## Background

Dogfood 2026-11-13: in a dashboard-hosted Claude lead, the agents pane shows
Pi rows with only model and effort (no context size, no tokens/s), and the
conversation pane's tool-summary lines ("Grep 2 · Read 1  9s") carry no token
chips. A read-only investigation found the hosted lead's own context band
renders correctly (`snapshot.usage` -> `bandView`, `pageview_band.go:95`) and
Claude subagent rows show `actx`. Neither gap is a regression; the hosted path
never implemented either.

- Pi rows: `SpawnAgentEvent` (`internal/wsagent/spawn_live.go:23-26`) sets
  label, status, model, effort and parent only, never `ContextTokens`.
  `mergeSpawnAgents` (`internal/wslive/spawn_agents.go:51`) copies tokens only
  from a separate harness-reported row, which a hosted lead does not have. The
  Pi child's own snapshot does hold usage, but the lead's row never reads it,
  and `piMapper` requests `get_session_stats` only on `agent_settled`
  (`internal/wsagent/pi.go:742`), so a running Pi has no usage at all.
- Batch chips: `batchNums` (`internal/wsdashboard/pages.go:504`) reads
  `ToolBatch.Usage`, which only the harness-side
  `agents-plugin/hooks/claude-mod/dashboard.ts` fills. The hosted
  `internal/wsagent/stream.go` batch tracker / `wireBatch` has no usage field;
  `claudeMessage.Usage` (`internal/wsagent/claude.go:1174`) feeds only the
  lead context sum.

## Idea

- Pi rows: where the runner-owned snapshot writer calls `SpawnAgentEvent`,
  put the child conversation's latest `Context.Tokens` into
  `Event.ContextTokens`; also request stats mid-turn (e.g. `turn_end` /
  `message_end`) so running rows show a figure.
- Batch chips: add `Usage *wslive.TokenUsage` to `wireBatch` and the batch
  tracker, filled from `msg.Usage` in `claudeMapper.assistant`; the Pi mapper
  needs its own usage source.
- Compatibility: optional snapshot/event fields, T0.

## Decisions

- 2026-11-13 owner decision, board item (53) = a: request Pi session stats
  mid-turn too (assistant `message_end` / `turn_end`), throttled to at most
  about once per second, so running rows show their child's usage.
- Rejected: stats only at turn end.
- T0: all added wire/event/snapshot fields are optional; no protocol bump.
- Keep the existing row and batch rendering; populate their existing figures.

## Constraints

- Follow `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` for tooling internals.
- Run Go builds/tests only on remote w5090.

## Phases

### Phase 1: Populate hosted child rows and batch usage

Project each child's own context and available output rate onto hosted Pi
rows, request throttled mid-turn stats, and attach assistant-message usage to
hosted batches. Use Pi message usage too if directly available. Leave the
composer context bar outside this change. Verify mapper counts, throttling,
row projection/recovery, and existing token-chip rendering with remote Go tests.

### Result (d567abd01) - 2026-11-13

- Pi child status usage now feeds optional `State.ContextTokens` and
  `State.OutputTps` through `SpawnAgentEvent` into the hosted lead's row.
  Context survives daemon recovery and closure; an explicitly unknown context
  clears the previous figure, and settled rows omit the streaming rate.
  Activity-only native aliases no longer erase the child's own telemetry.
- Assistant `message_end` and `turn_end` request Pi stats while work continues,
  with mid-turn requests coalesced to one per second. `agent_settled` keeps its
  unconditional final refresh. Rate estimates use the child's output tokens
  over its received streaming span, excluding request/tool latency; absent or
  burst-flushed streams do not fabricate a rate.
- Hosted batches carry optional `Usage`, populated from both Claude and Pi
  assistant messages. Claude split-line updates replace rather than accumulate
  the same message's usage. Finished batches reach the existing ring summaries
  and token-chip renderer without a presentation change. Pi usage was directly
  available, so no Pi batch-source follow-up was necessary.
- Verification on remote w5090: `go test ./internal/wsagent/...
  ./internal/wslive/... ./internal/wsdashboard/...` passed twice (final wsagent
  19.947s; the other final results cached). Final `go test ./...` passed every
  package except the owner-excluded
  `cmd/ws-mcp.TestHostedConnectionSeeingALANChangeReplacesTheDaemon`
  predecessor-exit timeout. `internal/wsholder` passed (15.165s); no known
  holder-shutdown failures occurred. No excluded failure was investigated.
- Regression coverage exercises both mappers through wire decoding and finished
  ring usage, split-message replacement, missing usage, throttled running stats,
  own-child rates, native aliases, unknown context, and running/closed recovery.
- One self-review pass completed. Fixed explicit unknown-context projection and
  removed unrelated formatting churn. `git diff --check` passed.
- Forward: Hosted Pi's rate is an approximate first-update-to-message-end
  figure. Align it with native Pi's visible-block/reasoning-excluded estimator
  if exact cross-surface rate parity is needed.
- Forward: The possible composer context-bar / `ctx` SSE gap remains unverified
  and outside this ticket; the reported screenshot did not include that region.

## Resolution

Implemented both Idea parts on `impl/develop/hosted-token-usage`, including the
owner's mid-turn stats decision. Closed using the `git mv` fallback because
`ws tickets.close` is unavailable in this worker's tool set. All additions are
optional (T0); no protocol or schema version changed.
