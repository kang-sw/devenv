---
title: "Numbers 화면: 하네스·모델별 한 달 통계"
parent: 261119-epic-ws-daemon-sqlite-storage
related:
  261119-refactor-ws-whiteboard-sqlite-board-store: "prerequisite: SQLite board store must land on develop before this ticket is promoted to ready"
---

# Numbers screen for monthly harness and model statistics

## Background

Extend the dashboard's plan-usage area into a Numbers screen that gives the
owner evidence of when AI load and failures clustered across work harnesses.
The owner requested usage and performance trends useful for diagnosing periods
when AI tools were unavailable at work (2026-11-20, 20:22 KST), and placed this
work after the SQLite storage epic (20:24 KST).

## Decisions

- **Sequencing (`(1860)=a`):** Instrumentation, storage, and the screen land on
  top of the SQLite storage epic, after the SQLite board store reaches
  `develop`. Rejected: enabling instrumentation now with an interim JSONL
  store and migrating it to SQLite later; that doubles store work. Accepted
  cost: data starts accumulating later.
- **Owner scope:** Approximately 30 days of statistics by harness (Claude,
  Codex, agy, Pi) and model/settings: output TPS, error rate, response latency,
  input/output token totals and growth trend, and periods when load or outages
  clustered. Daemon-side metering is an acceptable first source.

## Constraints

- Follow `ai-docs/manuals/performance.md` for `agents-plugin-tool/` changes,
  including file-operation counts and interactive-path I/O.
- Follow `ai-docs/manuals/release-compatibility.md` for any cross-version store
  or API surface; settle a source-backed compatibility tier before ready
  promotion.
- Follow `ai-docs/manuals/shipped-surface-boundary.md` for any text emitted to
  downstream projects.
- The approximately 30-day telemetry retention must survive the existing
  seven-day session-directory prune.
- The SQLite board store must land on `develop` before this ticket is promoted
  to `ready/`.

## Evidence (2026-11-20 audit)

### Bottom line
- [VERIFIED] A 30-day per-harness/per-model view cannot be reconstructed from today's stores. Ended session dirs are deleted after 7 days, rings are capped by entry count, plan-usage history is 14 days; durable telemetry is not consistently keyed by model or turn outcome. wsdashboard/registry.go:55-57,369-424; wslive/snapshot.go:78-83; wsdashboard/plan_usage_history.go:14-18

### Session identity/settings
- [VERIFIED] agent.json persists Backend, Model, Effort, created/closed time, and Tier for spawned conversations: session-level, not per turn. wsagent/record.go:27-45,51-53,116-119
- [VERIFIED] Model/effort can change at a turn boundary by replacing current values, so joining old ring turns to the current record misattributes earlier turns. wsagent/runner_events.go:914-953
- [VERIFIED] Closed sessions' directories age out after 7 days; pruning removes record, snapshot, ring, model evidence, output-rate sidecar. wsdashboard/registry.go:55-57,369-424,460-465

### Turn ring
- [VERIFIED] Common runner path (claude/codex/agy/pi) records each RingTurn with tool Done/Failed counts, DurationMs, origin/wake flag, optional Reason. wsagent/runner_events.go:108-135; wslive/state.go:307-345; wslive/ring.go:58-87
- [VERIFIED] Ring retains at most 2,000 entries per session (count-based, not time). wslive/ring_store.go:24-39,90-103,669-693; wslive/snapshot.go:78-83
- [VERIFIED] RingTurn stores no success/failed status, failure class, harness, model, tier, tokens, or first-output time. wslive/ring.go:58-110; wslive/state.go:338-345
- [VERIFIED] Spawned children persist TurnResult{status,message,error} and cumulative ActiveMs as recovery state, not history; leads get none. wsagent/spawn.go:27-55,257-285; wsagent/record.go:129-170,241-247

### Output rate
- [VERIFIED] Only agy produces a durable OutputRate (rolling window of 8 eligible responses); stored per session in output-rate.json with only outputTps + timestamp; reader returns latest only. wsagent/agy_output_rate.go:11-24,65-69,141-179; wslive/output_rate.go:16-38,102-150,179-193,225-259
- [VERIFIED] Pi computes TPS in memory, exposed only while a turn is live. wsagent/pi.go:924-932,1203-1208; wsagent/runner_conv.go:747-756,1041-1058
- [INFERRED] Claude and Codex have no TPS producer.

### Tokens/context
- [VERIFIED] Token shape input/output/cache-read/cache-write is attached to tool batches and summed into RingTools summaries. wslive/snapshot.go:372-400; wslive/toolsummary.go:9-24,60-103,120-180
- [VERIFIED] Claude and Pi parse full usage but persist it only for messages with a tool-call batch; pure-text responses are lost. wsagent/claude.go:1421-1475; wsagent/stream.go:417-477; wsagent/pi.go:803-817,1203-1247
- [VERIFIED] Codex mapper keeps only latest context total/window and cache-hit %, discarding output/cache-write/reasoning counts. wsagent/codex_mapper.go:203-224,569-593
- [VERIFIED] Agy prompt results carry usage, but only output-minus-thought is parsed for TPS; usage_update keeps latest context only. wsagent/agy_output_rate.go:141-185; wsagent/agy.go:1350-1365
- [VERIFIED] Context/cost are snapshot state, not history. wslive/snapshot.go:198-242; wsagent/record.go:603-658

### Model evidence
- [VERIFIED] Claude and Pi persist observed answering-model evidence, pruned to first/changes/latest/recent eight. wsagent/claude_alerts.go:70-95; wsagent/pi.go:1252-1283; wslive/observed.go:19-63,180-223; wslive/observed_prune.go:12-27,42-75
- [INFERRED] Codex and agy lack per-response model evidence.

### Failures/retries
- [VERIFIED] Structured ring notices: login, model, usage_limit only (Claude all three; Codex login + usage_limit). wslive/host.go:9-49; wsagent/claude_alerts.go:98-123; wsagent/codex_mapper.go:517-531
- [VERIFIED] Retry state lives only in live/recovery snapshot, cleared at settlement. wslive/host.go:52-82; wsagent/runner_recovery.go:20-53
- [VERIFIED] Agy watchdog builds failure classes (silence, capacity/503, EOF, stream/TLS timeout, interrupted, API/model failure) but only as in-memory list and diagnostic text. wsagent/runner_conv.go:92-117; wsagent/agy_watchdog.go:477-527
- [VERIFIED] No durable, queryable cross-harness failure-class series exists. wsagent/runner_events.go:208-211,309-358

### Plan usage / tracker
- [VERIFIED] Claude 5h/7d % from live snapshots; Codex weekly via Codex or Pi credentials, both stored as harness codex. wsdashboard/plan_usage.go:193-274; wsdashboard/plan_usage_probe.go:69-139,215-256
- [VERIFIED] Agy % by two model groups, probed on explicit refresh only. wsdashboard/plan_usage_agy.go:22-63,189-263
- [VERIFIED] Plan history: harness/group/window/source/reset/as-of/percent only, 14 days, whole-file rewrite, exact schema v1 reader. Failed probes append nothing. wsdashboard/plan_usage_history.go:14-38,153-180,199-316; plan_usage_probe.go:94-112; plan_usage_agy.go:253-271
- [VERIFIED] Tracker is an append-only work-item event log without harness/model/token/latency fields. wsledger/doc.go:1-35; wstracker/activity.go:37-62

### Missing
- [VERIFIED] Time-to-first-output: known transiently (turn start + first reply StartedAt) but not retained. wslive/state.go:307-320,380-397; wslive/snapshot.go:292-301; wslive/ring.go:74-87
- [VERIFIED] Complete per-turn token totals missing for codex/agy and pure-text claude/pi.
- [VERIFIED] Durable canonical turn outcome/failure class missing for all harnesses.
- [VERIFIED] Immutable model/tier-at-turn missing.
- [VERIFIED] Retention (7-day session prune, 2,000-entry ring cap, 14-day plan history) blocks 30 days.

### Non-authoritative suggestions (not decided)
- [INFERRED] Per-turn in-memory measurement snapshotting harness/model/effort/tier at turn start, collecting first-output, full tokens, normalized outcome class; settle once at turn end. Parse values streams already expose; no new probes.
- [INFERRED] Daemon-global store outside sessions/ so the 7-day prune does not delete it; memory-first rollup so the screen reads RAM only. Do not extend plan-usage-history.json (schema-2 would be T3 for its exact-v1 reader).
- [INFERRED] Keep plan quota trends as a separately joined dataset.

## Phases

### Phase 1: Design and settle at promotion

Settle the phase plan when this ticket is promoted to `ready/`, after the
SQLite board store lands on `develop`.
