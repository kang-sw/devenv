---
title: MCP child actor bootstrap
parent: 260524-epic-mcp-actor-setup-state
related:
  260524-feat-mcp-actor-setup-bootstrap: supplies the setup and actor persistence foundation
  260524-feat-exec-output-ask: model-backed readers should use scoped child actors
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# MCP child actor bootstrap

## Background

Named agents and subqueries currently receive root context from the parent
runtime through command arguments and stored agent state. If nested MCP
processes lose environment or process-local setup state, child calls need an
explicit actor id recovery path that does not rely on launch metadata.

## Decisions

- `agents.call` and `subquery` should mint or reuse a child actor id with
  delegate or reader authority before launching the child call.
- The parent runtime should inject a `ws.setup(id: "<child-actor-id>")`
  instruction into the child system prompt or call bootstrap context on every
  call.
- Persistent named agents may keep a stable child actor per registered agent,
  while one-shot subqueries may use ephemeral child actors that are erased or
  marked inactive with the ephemeral agent.
- Child setup should recover root and authority from actor persistence rather
  than from environment variables.
- Child actor authority remains cooperative unless a later host-level launch
  boundary supplies stronger isolation.

## Constraints

- Do not expose the lead bootstrap method in delegate orientation or subquery
  prompts.
- Do not rely on `WS_MCP_TOOL_PROFILE`, `WS_MCP_PROJECT_ROOT`, or other
  environment propagation for child correctness.
- Preserve existing named-agent prompt composition rules and avoid duplicating
  setup instructions across repeated prompt fragments.
- Child actor setup should be compatible with later `exec.ask` reader sessions
  and other model-backed reader tools.

## Phases

### Phase 1: Inject child actor setup instructions

Implement child actor creation and setup prompt injection for named agents and
subqueries after `260524-feat-mcp-actor-setup-bootstrap` lands.

Verification should cover persistent named-agent calls, subquery calls,
restarted nested MCP behavior where the child calls `ws.setup(id: ...)`,
absence of lead bootstrap instructions from child prompts, and actor cleanup or
inactive marking for ephemeral subqueries.
