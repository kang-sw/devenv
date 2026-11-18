---
title: "agy custom agent: replace Antigravity's default system prompt for hosted agy leads"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-feat-ws-hosted-antigravity-adapter: "its custom-agent removability verdict (08)=c rejected this direction; (127)=a reopens it as a probe"
  261117-feat-ws-hosted-prompt-assembly: "Phase 2 edits the agy lead row of the harness catalog; a custom agent would change how the agy lead receives the rendered manual"
  261115-research-ws-unified-agent-tool-surface: "dropped; holds the earlier agy probe of excludeDefaultComponents, tools: [] and enable_mcp_tools"
---

# agy custom agent: replace Antigravity's default system prompt for hosted agy leads

## Background

Users report, as general reputation rather than a concrete case, that
Antigravity's (agy) default system prompt yields poor results. An agy custom
agent replaces that default prompt. `261117-feat-ws-hosted-antigravity-adapter`
probed this and recorded verdict (08)=c
(`ai-docs/tickets/.done/261117-feat-ws-hosted-antigravity-adapter.md#L330-L375`):

- the custom agent body replaces agy's default prompt (measured 9135 to 3748
  tokens);
- a `tools` allowlist drops the session's MCP tools, and `mcpServers` restores
  them only through the lazy `call_mcp_tool` gateway;
- a malformed agent config silently falls back to the default agent;
- the agent file must live in the workspace or the agy home;
- the recommendation then was that the hosted lead keeps agy's default agent
  and its subagent tools.

The earlier unified-tool-surface probe of `excludeDefaultComponents`,
`tools: []` and `enable_mcp_tools` is at
`ai-docs/tickets/.dropped/261115-research-ws-unified-agent-tool-surface.md#L634-L669`.

Owner (127)=a, 2026-11-18, reopens the direction: start with a probe, then
settle the design from its results.

## Probe questions

1. Does a body-only custom agent, with no `tools` allowlist, keep the session's
   ws MCP tools as direct tools rather than only through `call_mcp_tool`?
2. What does `excludeDefaultComponents` remove, and does the hosted lead still
   work (file edits, shell, subagents, ws MCP calls) with it set?
3. Where must the agent file live for a hosted agy lead (workspace versus agy
   home), and how does the daemon detect the silent fallback to the default
   agent so a malformed file fails loudly instead?
4. Quality A/B: run the same task on a hosted agy lead with the default agent
   and with the custom agent, and compare ws procedure compliance (playbook
   reads, ticket and commit flow) and the coding result side by side.

## Outcome Ledger

### Verified Findings

- (08)=c probe facts listed in `## Background`, from
  `261117-feat-ws-hosted-antigravity-adapter`.

### Confirmed Decisions

- (127)=a, 2026-11-18: revive the agy custom-agent direction as a probe-first
  research; the reason is user reputation about agy's default prompt quality.
- "Quality" is judged as ws procedure compliance plus the coding result,
  default agent versus custom agent, side by side.

### Proposals

### Open Questions

- Whether a custom agent subsumes or changes the rendered-manual delivery of
  `261117-feat-ws-hosted-prompt-assembly` for the agy lead (system-prompt
  append versus agent body).
- Whether the result still matters once Pi becomes the canonical hosted
  connector (`261118-research-ws-pi-hosted-connector`); agy's native lead
  stays first-class support in the meantime.

### Rejected Alternatives
