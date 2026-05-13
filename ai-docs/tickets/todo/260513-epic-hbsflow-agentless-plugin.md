---
title: hbsflow agentless plugin distribution
related:
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture anchor
related-mental-model:
  - plugin-runtime
  - mcp-runtime
  - named-agent-runtime
  - workflow-skills
  - claude-compatibility
---

# hbsflow agentless plugin distribution

## Scope

Create and maintain `agents-plugin-hbsflow/` as an internal derivative plugin
distribution for agentless workflow use. The distribution should reuse the
shared `agents-plugin-tool/` runtime where possible, publish Codex and Claude
metadata, use the `hbsflow` namespace, and hide ws named-agent and subquery
surfaces from its runtime contract and workflow guidance.

The milestone includes the runtime gates, package scaffold, skill text
normalization, installer or marketplace touchpoints, and verification needed for
Codex and Claude users to install `hbsflow` separately from the full `ws`
plugin.

## Non-Scope

Do not fork a separate hbsflow spec corpus or independent workflow doctrine.
Behavioral meaning remains managed through the existing `agents-plugin/` and
`agents-plugin-tool/` specs unless a spec entry explicitly marks an hbsflow
variant constraint.

Do not reintroduce a live `claude-plugin/` tree. Claude support for hbsflow
should follow the current shared-package compatibility pattern.

Do not port ws named-agent or subquery workflow semantics into hbsflow skill
text. hbsflow should prefer host-native agent/subagent capabilities when a host
offers them, and direct local investigation when it does not.

Do not present hbsflow to end users as a ws variant, ws-lite package, or
ws-compatible mode. The derivative relationship is an internal maintenance
fact for this repository; distributed hbsflow manifests, skills, workflow
guidance, and ordinary runtime output should speak in hbsflow terms.

## Child Tickets

- `260513-feat-hbsflow-agentless-runtime-mode` - ready shared runtime slice for
  `WS_MCP_NO_AGENT`, namespace text, setup alias behavior, and default full ws
  regression coverage.
- `260513-feat-hbsflow-agentless-plugin-scaffold` - ready package scaffold and
  derivative distribution slice after the runtime mode is available.
- Planned: installer, marketplace, and release verification slice after the
  scaffold establishes the hbsflow package and runtime contract.

## Cross-Child Decisions

`agents-plugin/` remains the canonical full ws distribution.
`agents-plugin-hbsflow/` is an internal derivative distribution and must be
evaluated whenever a change touches plugin packaging, MCP tool surfaces,
runtime contracts, launcher behavior, bundled skills, prompt guidance, or
release validation.

The hbsflow distribution contract is:

- Folder: `agents-plugin-hbsflow/`.
- Plugin name: `hbsflow`.
- MCP server key: `hbsflow`.
- Skill namespace: `hbsflow:lead-*`.
- MCP notation in skill text: `hbsflow/<tool>`.
- Agentless runtime env: `WS_MCP_NO_AGENT=1`.
- User-facing runtime namespace env: `WS_MCP_NAMESPACE=hbsflow`.
- Setup tool alias env: `WS_MCP_SETUP_TOOL=setup`.

When full ws changes affect copied or caller-visible hbsflow surfaces, include
the hbsflow update in the same logical change. If hbsflow cannot follow in the
same patch, record a ticketed follow-up instead of silently leaving drift.

Internal repository specs, tickets, and mental models may mention that hbsflow
derives from the ws runtime to preserve maintenance traceability. The hbsflow
package itself should not require users or host agents to know about ws naming,
ws skill invocations, or ws-managed agents.

## Completion Criteria

- Done: hbsflow installs and validates as a separate Codex and Claude-compatible
  plugin, advertises no named-agent or subquery surfaces, uses `hbsflow` in
  user-facing workflow guidance, and has tests covering its runtime contract.
- Dropped: the team decides the full `ws` plugin should remain the only
  maintained distribution, and hbsflow-specific package work is no longer
  wanted.
- Deferred: additional host-specific polish, release automation, or internal
  rollout packaging that is not required for the initial agentless distribution.
