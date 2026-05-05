---
title: ws-mcp async subquery
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-feat-ws-mcp-nonblocking-orchestration: provides async agent call/wait primitives reused by subquery
  260504-research-durable-leaf-role-assignment: deferred recursive-helper containment research
related-mental-model:
  - executor-wrapup
completed: 2026-05-05
---

# ws-mcp async subquery

## Background

`ws/subquery` is used for read-only codebase surveys that frequently fan out
across independent domains, especially forge-spec and forge-mental-model
workflows. The current MCP tool waits for the delegated query to finish and
returns answer text directly. That synchronous surface is sensitive to host MCP
request timeouts and makes broad survey fan-out brittle.

`ws/api.ask` has a different shape: it performs one user-facing API documentation
question, runs any resolved domain managers in parallel internally, and returns
one aggregated answer. Keeping `api.ask` synchronous is natural because the
runtime owns routing, domain fan-out, and result aggregation.

## Decisions

- Make `ws/subquery` always return immediately with a subquery key and follow-up
  instructions.
- Reuse the named-agent async path instead of one-shot sync calls, so status,
  wait, tail, cancel, interrupt hooks, stdout/stderr capture, and final output
  are available through existing `ws/agents.*` tools.
- Do not keep a sync compatibility mode. The plugin is not yet downstream
  published, and all affected references are internal skills, prompts, tests,
  and runtime docs.
- Keep `ws/api.ask` synchronous. It already fans out to per-domain managers in
  parallel and aggregates the final response inside the tool.
- Treat named subquery workers as read-only leaf work. If the old sync
  `delegate` tool profile and async worker `leaf` profile differ, prefer leaf
  containment for the new subquery behavior.

## Phases

### Phase 1: Runtime async subquery surface

Replace the sync `oneShot` subquery path with named-agent async startup.

Success criteria:

- `ws/subquery` registers or resets a generated subquery agent name using the
  embedded subquery system prompt.
- The tool starts the query through the same async path as `ws/agents.call`.
- The immediate response includes the generated key/name, status, pid, and
  follow-up commands for `ws/agents.wait`, `ws/agents.status`,
  `ws/agents.tail`, and `ws/agents.cancel`.
- Completed results are retrieved through `ws/agents.wait` or
  `ws/agents.print`; `ws/subquery` no longer returns answer text directly.
- Tests cover the immediate return contract and the generated agent prompt.

### Phase 2: Internal reference migration

Update workflow skill text, runtime docs, and prompt guidance so every internal
caller treats `ws/subquery` as an async start operation.

Success criteria:

- Forge-spec and forge-mental-model instructions tell the lead to capture
  subquery keys and wait for all returned names before synthesizing.
- Workflow primitive docs describe the async contract and `agents.wait` result
  retrieval.
- Implementer and discussion guidance no longer assume `ws/subquery` returns
  answer text directly.
- `api.ask` docs explicitly remain synchronous and explain that domain fan-out is
  internal to the tool.

### Result (pending) - 2026-05-05

Implemented in the next source commit. `ws/subquery` now registers a generated
`subquery-<timestamp>-<seq>` named agent, starts it through the async
`agents.call` path, and immediately returns `subquery_key`, status, pid, and
follow-up `agents.wait/status/tail/cancel` calls. Results are retrieved through
`agents.wait` or `agents.print`; subquery no longer returns answer text
directly.

The generated key includes an atomic sequence suffix so parallel subquery starts
from the same process do not collide when the clock value matches. Delegate MCP
profiles may use `agents.wait/status/tail/cancel/print` only for generated
`subquery-*` agent names, preserving the broader lead-owned orchestration
boundary. Internal skill/prompt/reference text now treats subquery as an async
start operation, and `api.ask` documentation explicitly remains synchronous
while its per-domain managers fan out internally.
