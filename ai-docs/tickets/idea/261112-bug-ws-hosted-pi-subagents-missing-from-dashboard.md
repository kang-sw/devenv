---
title: "Hosted Pi subagents run but are absent from the dashboard agents panel"
related:
  261110-epic-ws-dashboard-agent-host: hosted Pi lead dogfood
  261111-research-ws-hosted-session-subagent-output-rate-source: adjacent hosted child telemetry research
---

# Hosted Pi subagents are absent from the dashboard

## Background

During dashboard-hosted Pi dogfood on 2026-11-12, the user reported no
visible Pi subagents while two native `ws-agent-spawn` children were active.
The lead immediately called `ws-agent-list`; both `pi-load-audit`
(`634a5bb7-fc75-4333-8cde-358512b377a2`) and `runtime-audit`
(`e9571464-c2f3-4170-a002-e3c2623953d6`) reported `running`.
An intermediate report from `pi-load-audit` had already reached the lead.
These were Pi RPC children, not the earlier standalone CLI fallback probe.
Their OpenAI Codex model provider does not make them Codex CLI agents.

This is separate from the initial missing Pi plugin installation: after
installation, ws tool calls and native child execution work. Actual worker
execution and dashboard visibility must be checked independently.

## Diagnosis

Code read 2026-11-12, consistent with the Pi lead's live evidence (matching
registration/agent rows, snapshot with no `agents` member):

- `agents-plugin-pi/src/index.ts:935` creates the dashboard feed only when
  `hosted.mode !== "hosted"`, so in hosted mode the native registry -> agent
  event path (`dashboard-feed.ts:325-347`, `466-493`) never runs and
  `agentDashboardRefreshRef` stays unset. The feed is off there on purpose: a
  second intake writer would open a competing hosted slot and hide the
  runner's lead snapshot.
- `wsagent/pi.go` `piMapper.mapLine` (561-662) produces no event carrying an
  `AgentID`; the runner (`runner.go:1747`, `record.go:171-207`) only projects
  received Live events. No session-key mismatch.
- Pi RPC carries extension output to stdout only through
  `extension_ui_request`; `setWidget` passes string arrays through
  (`rpc-mode.js:123-136`).

## Decisions

- Hosted mode gets a child-only channel: the Pi extension emits child
  `agent` and `permission` events as JSON strings through
  `ctx.ui.setWidget("ws-dashboard-agents", [...])`, and the daemon's Pi mapper
  decodes only that widget key, accepting only `agent`/`permission` events
  with a non-empty agent id and ignoring everything else (no reset, turn, or
  lead batch events). User board (31).
  Rejected: deriving rows from `ws-agent-spawn` results and `ws-agent-settled`
  pushes in the RPC stream (pushes are batched, late, or absent for
  owner-held children; no batches, model, or rate); sending child events
  through the existing `dashboard.intake` (competing hosted slot, or new
  intake-to-runner routing).
- Field parity with the CLI feed: reuse `dashboardAgentEntries`, the same
  change detection, 100 ms throttle and 1 s poll while children run, and
  include child `sessionKey` and changed `batches`. User board (32).
- While any child exists, every 30 s the channel resends the full child set
  without batches, so rows lost to a holder replay gap recover. User board
  (33).
- Compatibility tier T0: the reserved widget key is the only new contract;
  an old daemon ignores the widget and an old extension never sends it. No
  MCP tool/argument, store shape, tracker event, holder protocol, or snapshot
  shape change.

## Investigation boundary

Capture the user-visible discrepancy now; no implementation mechanism is
settled. Inspect the hosted Pi adapter's feed suppression and the hosted
runner's child telemetry ingestion/projection, and compare actual session
identity and child registry data with what the dashboard receives. A missing
telemetry path and an identity mismatch are hypotheses, not established
causes. No source, configuration, protocol, or event change is authorized.

## Phases

### Phase 1: Diagnose and settle hosted Pi child visibility

Identify the missing handoff with source and runtime evidence, then settle
any repair contract before implementation. Verification must distinguish
running child execution, intermediate/final delivery to the parent, and
agents-panel visibility. Preserve CLI-created Pi behavior and avoid duplicate
feeds in hosted mode.
