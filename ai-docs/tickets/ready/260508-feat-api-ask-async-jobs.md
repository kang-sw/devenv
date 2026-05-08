---
title: api.ask async jobs
related:
  260429-feat-api-deps: original API documentation cache implementation ticket
spec:
  - 260508-api-docs-async-jobs
  - 260508-api-documentation-async-mcp-tools
skeletons:
  phase-2: 082141a
related-mental-model:
  - api-documentation-cache
  - mcp-runtime
  - named-agent-runtime
---

# api.ask async jobs

## Background

`ws/api.ask` currently returns a synchronous tool response. Internally it can
route domains and fan out per-domain manager calls, but the caller cannot keep
control of API documentation work that exceeds the host tool-call timeout
window. That makes long cache bootstrap, slow upstream fetches, or broad
multi-domain questions physically hard to recover from when the maximum wait is
shorter than the job.

The existing synchronous `ws/api.ask` behavior should remain available for
ordinary lookups. Long-running API documentation work needs a separate async
job surface rather than changing the meaning of the existing tool.

## Decisions

- Preserve `ws/api.ask` as the synchronous quick-path API.
- Add a separate async API documentation job surface instead of overloading
  `ws/api.ask`.
- Store async job state durably enough that callers can poll, inspect, cancel,
  and recover after a client timeout.
- Reuse current API domain routing, per-domain manager sessions, cache
  ownership, and partial-success aggregation semantics.
- Do not use `ws/subquery` as the only answer; it is a useful wrapper pattern
  but does not expose API-job-specific status or cancellation.

## Phases

### Phase 1: Spec async API documentation job surface

Define caller-visible behavior for async API docs lookup. Cover start, result,
status, cancellation, timeout recovery, partial failures, sync/async
compatibility, and any CLI mirror expectations.

The likely MCP surface is:

- `ws/api.ask_async` returns an `api_job_key` immediately.
- `ws/api.result` consumes or reads the final answer.
- `ws/api.status` reports routing, domain progress, failures, and final-output
  availability.
- `ws/api.cancel` stops active work on a best-effort basis.

### Result (05778dc) - 2026-05-08

Planned spec entries were added for the async API docs job lifecycle and MCP
tool surface. `ws/api.ask` remains documented as the synchronous quick path, and
the async surface is tracked by spec stems `260508-api-docs-async-jobs` and
`260508-api-documentation-async-mcp-tools`.

### Phase 2: Implement durable API docs jobs

Implement async jobs using the existing API docs runtime pieces without
duplicating manager behavior. Job state should record the prompt, optional
domain hint, resolved domains when known, per-domain progress, final text,
errors, timestamps, and cancellation state.

The implementation must account for long-running manager fetch/bootstrap work
that outlives the caller's initial MCP request.

### Phase 3: Guidance, tests, and compatibility

Update workflow guidance so ordinary API lookups still use `ws/api.ask`, while
potentially long bootstrap or broad multi-domain work uses the async surface.

Add tests for immediate return, result polling, partial domain failure, all
domain failure, cancellation, and compatibility with the existing synchronous
tool.
