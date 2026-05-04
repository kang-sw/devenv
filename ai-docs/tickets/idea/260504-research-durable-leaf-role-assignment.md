---
title: durable leaf role assignment
related:
  260503-feat-ws-mcp-worktree-orchestrator-lock: deferred leaf-level follow-up from worktree-local orchestrator lock work
  260503-epic-ws-agent-workflow-stability: stabilization parent that exposed delegate containment limits
  260503-epic-agents-plugin-skill-porting: orchestration skill migration should proceed without blocking on this research
---

# durable leaf role assignment

## Background

The worktree-local orchestrator lock now prevents delegated Codex agents from
using lead-level `agents.*` and `config.*` orchestration tools in the observed
plugin-managed failure mode. That closes the high-ROI blocker for returning to
`write-code` and the remaining orchestration skill migration.

Leaf-level containment remains unsettled. The async worker can set
`WS_MCP_TOOL_PROFILE=leaf`, but plugin-managed Codex sessions may reuse or
attach to an MCP server whose tool surface is not governed by the child
process's environment. As a result, a worker that should be leaf-level may still
receive delegate-level tools such as `subquery`.

The concrete concern is recursive bounded-helper use: for example, a delegated
worker or a `subquery`-like helper recursively calling `subquery`. Solving that
durably may require role assignment state outside process environment,
per-call/run identity, recursion budgets, or MCP-session-bound authorization.
Those designs are more complex than their current return on investment.

## Deferred Questions

- Should ws assign durable roles to named agent records, current calls, or MCP
  server/session identities?
- Can recursive `subquery` calls be bounded with a simple depth or budget model
  without introducing brittle global state?
- Should leaf-level workers hide `subquery`, or should `subquery` remain
  delegate-level only while leaf is reserved for future stricter workers?
- What evidence would justify implementing this instead of relying on
  delegate-level containment plus prompt orientation?

## Current Decision

Defer durable leaf-level role assignment. Treat the current worktree-lock
boundary as sufficient for the next migration slice: lead-owned orchestration is
protected, delegated agents cannot recursively spawn/manage named agents, and
remaining recursive helper policy can be revisited only if it becomes an
observed workflow failure.
