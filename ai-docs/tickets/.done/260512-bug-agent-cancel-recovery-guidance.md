---
title: Agent cancel recovery guidance
spec:
  - 260512-agent-cancel-resume-guidance
  - 260512-agent-recall-hidden-surface
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
completed: 2026-05-12
---

# Agent cancel recovery guidance

## Background

Named-agent callers tend to cancel work after a single result timeout because
the current cancelled-state guidance looks terminal. `agents.cancel` returns
status text whose follow-up points to diagnostics and erase, while `agents.recall`
is visible as a specialized recovery tool despite being unreliable in practice.

The runtime should keep cancellation as an urgent stop path, but cancellation
should not imply that an unresponsive agent session is unrecoverable when no
result has been produced.

## Decisions

- Prefer retrying `agents.call` on the same registered agent with a recovery
  prompt over advertising `agents.recall`.
- Keep the recall implementation and CLI mirror available for now to avoid
  breaking compatibility; hide it from normal MCP discovery and workflow docs.
- Make the guidance visible in tool output, not only in repository docs, because
  timeout-prone model behavior is driven by immediate follow-up text.

## Phases

### Phase 1: Cancel output and recall visibility

Update named-agent cancellation/status guidance so cancelled calls without
cleanup-needed state tell callers that, when cancellation followed a no-result
timeout, they can retry `agents.call` on the same registered agent to attempt
resume.

Remove `agents.recall` from model-visible MCP tool discovery and workflow
guidance while preserving compatibility implementation paths. Update tests and
docs that assert the named-agent tool surface or follow-up text.

### Result (ca26dcc) - 2026-05-12

Implemented cancellation recovery guidance in status output and follow-up text.
Cancelled calls without cleanup-needed state now include a `cancel_recovery_tip`
that tells callers to retry `agents.call` on the same registered agent with a
recovery prompt when cancellation followed a no-result timeout.

Removed `agents.recall` from advertised MCP tool discovery, runtime capability
metadata, and workflow guidance while keeping the compatibility implementation
and CLI command path available for manual use.
