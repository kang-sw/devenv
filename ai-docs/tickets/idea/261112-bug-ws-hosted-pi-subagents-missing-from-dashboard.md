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
