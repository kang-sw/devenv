---
title: "Hosted sessions show no token usage on Pi rows or tool-batch lines"
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

## Open

- Whether the owner also saw the composer context bar missing (the screenshot
  crop excluded it); if so, check the SSE live-update path for the `ctx`
  region, which the investigation did not verify.
- Cost of mid-turn `get_session_stats` calls on a busy Pi.
