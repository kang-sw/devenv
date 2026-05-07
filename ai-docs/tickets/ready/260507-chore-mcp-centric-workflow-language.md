---
title: MCP-centric workflow language cleanup
related:
  260429-research-host-neutral-ws-plugin: host-neutral migration anchor
spec:
  - 260507-mcp-centric-workflow-language
related-mental-model:
  - workflow-skills
  - mcp-runtime
  - named-agent-runtime
---

# MCP-centric workflow language cleanup

## Background

Shared Codex-facing workflow guidance should present ws orchestration through MCP
primitives first. Legacy CLI adapter phrasing is now mostly useful for tests and
compatibility references, so skill text that still teaches CLI-era surfaces can
make the host-neutral contract look less authoritative.

The immediate discussion centered on general-purpose named-agent registration:
`ws/agents.register(name: "<agent-name>")` should be documented as the primary
MCP form when no role prompt is needed, while `prompts` and optional tier/model
arguments cover role-specific or workload-specific variants.

## Decisions

- Use MCP notation as the shared workflow-language default.
- Keep CLI adapter syntax out of Codex-facing skill bodies unless the skill is
  explicitly about compatibility or testing.
- Treat `prompts` as canonical and `prompt_refs` as a migration alias in shared
  guidance.
- Document promptless named-agent registration as general-purpose delegation
  with delegate orientation and the default `core` tier.

## Phases

### Phase 1: Audit Codex-facing workflow skill language

Audit `agents-plugin/skills/lead-*` for CLI-era wording, prompt-reference
phrasing, and examples that imply CLI adapters are the primary execution
surface. Preserve compatibility references in Claude-specific documentation.

Success criteria:

- Codex-facing workflow skills consistently teach MCP primitives for ws runtime
  actions.
- `lead-workflow-manual` documents promptless `ws/agents.register` as the
  general-purpose named-agent form.
- Role-specific agent examples use `prompts: ["<prompt-stem>"]`.
- Any retained CLI mention is explicitly scoped to compatibility or testing.
