---
title: ws MCP agent result and readiness API
related:
  260503-epic-ws-agent-workflow-stability: stabilization parent for named-agent lifecycle behavior
  260505-feat-ws-mcp-async-subquery: introduced key-returning async subquery fan-out that needs clearer result retrieval and cleanup semantics
parent: 260503-epic-ws-agent-workflow-stability
---

# ws MCP agent result and readiness API

## Background

`ws/subquery` now starts async named-agent work and immediately returns a
generated key. The current retrieval surface still inherits the older split
between `agents.wait` and `agents.print`: `wait` blocks and returns output,
while `print` reads the last output immediately. That split works mechanically
but blurs two separate concepts:

- waiting for one or more async calls to become ready;
- consuming the final result of one async call.

The split also leaves temporary helper agents hard to clean up consistently.
If both `wait` and `print` can return final output, both become potential
cleanup points.

## Direction

Introduce a clearer API model:

- `agents.result(name: "...", timeout_seconds?: <seconds>)` is the single
  result-consumption surface.
- `agents.wait(names: ["..."], timeout_seconds?: <seconds>)` waits for
  readiness across one or more agents and returns status metadata, not final
  output.
- `agents.print` is replaced by `agents.result` or kept only as a compatibility
  alias during migration.

`agents.result` should support both immediate and bounded retrieval:

- without a timeout, return already-available final output or an actionable
  non-ready status;
- with a timeout, wait for completion up to the bound and then return the final
  output when available.

`agents.wait` should support fan-out orchestration:

- accept multiple agent names in one call;
- behave like a readiness primitive, returning completed/failed/cancelled names
  and enough pending metadata for follow-up;
- avoid returning large final outputs.

## Temporary Agent Cleanup

Temporary helper agents should be marked by metadata, not by name parsing. Names
may carry a human-readable hint such as `subquery-tmp10491`, but cleanup policy
should depend on explicit agent metadata such as an ephemeral flag.

Result consumption is the only automatic cleanup point:

- successful `agents.result` on an ephemeral agent may erase the agent after
  reading the output;
- failed, cancelled, timed out, or still-running agents should remain available
  for `status`, `tail`, `cancel`, or explicit `erase`;
- follow-up text should not emphasize automatic deletion as a user-facing
  behavior.

## Open Questions

- Whether `agents.print` should be removed before first release or retained as
  a hidden/deprecated alias for `agents.result`.
- Exact structured text format for multi-name `agents.wait` so LLM callers can
  reliably distinguish ready and pending entries.
- Whether `agents.result(timeout_seconds: 0)` and omitted timeout should be
  identical, or whether omitted timeout should use the default bounded wait.
- Whether a later TTL/GC cleanup ticket is needed for abandoned ephemeral agents
  that are never consumed.
