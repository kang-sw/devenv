---
title: ws-mcp nonblocking orchestration
parent: 260503-epic-ws-agent-workflow-stability
related:
  260503-epic-ws-agent-workflow-stability: parent stabilization epic
completed: 2026-05-03
---

# ws-mcp nonblocking orchestration

## Background

The ws MCP server needed to remain responsive while agent work or bounded waits
were in progress. One long `agents.wait` must not monopolize stdio and block
unrelated status/debug calls. The temporary generic surfaces also needed cleanup
before release: `agents.call` should be the async start primitive, and generic
`agents.oneshot` should be removed in favor of explicit composition or
purpose-specific helpers such as `subquery`.

This child ticket preserves the completed Phase 4 scope split out of
`260503-epic-ws-agent-workflow-stability`.

## Result - 2026-05-03

Implemented nonblocking orchestration in `ws-mcp`. `ServeStdio` now dispatches
JSON-RPC requests concurrently while serializing stdout writes, and tracks
in-flight request IDs with cancellable contexts. `notifications/cancelled`
messages are logged and cancel the matching request context when the
notification is read.

The generic agent API surface was simplified before release. `agents.call` now
starts the async worker and returns promptly with running state, PID, and
follow-up commands. The temporary `agents.call_async` and generic
`agents.oneshot` surfaces were removed from MCP tools, CLI commands,
`runtime.json`, and shared skill text. Purpose-specific one-turn lookup remains
available as `subquery`; persistent delegates use `agents.register` +
`agents.call` + `agents.wait/status/print` + `agents.erase`.

`agents.wait` now short-polls by default. When a call is still active and no
timeout is supplied, it returns running state and follow-up guidance instead of
blocking indefinitely. Supplying `timeout_seconds` keeps bounded blocking wait
ergonomics, and the concurrent MCP server remains responsive to unrelated
status/debug/list requests while that wait is active.

MCP tool profiles were added through `WS_MCP_TOOL_PROFILE` and
`WS_MCP_ALLOWED_TOOLS`. `lead` exposes the full surface. `delegate` hides and
rejects durable `agents.*` orchestration while retaining helper tools such as
`subquery`. `leaf` also hides and rejects `subquery`. Filtering applies to both
`tools/list` and `tools/call`. Codex-backed async worker turns receive
`WS_MCP_TOOL_PROFILE=leaf`, while the internal sync path used by `subquery`
receives `delegate`.

Verification covered `cd agents-plugin-tool && go test ./...`, runtime JSON
parsing, rebuilding `agents-plugin/.runtime/darwin-arm64/ws-mcp`, direct MCP
tool-surface smoke, leaf-profile smoke, installed Codex cache launcher smoke,
`claude plugin validate agents-plugin`, and `git diff --check`.

Follow-up cancellation smoke after refreshing Codex confirmed the operational
contract: interrupting a long `agents.wait(timeout_seconds: 300)` no longer
blocks the MCP server, but the underlying async worker continues running until
the lead explicitly calls `agents.cancel`. A later `runtime.debug_events`
retrieval smoke showed no `notification.cancelled` event after Codex UI
interrupt, so ws must not rely on MCP cancellation notifications for
user-triggered task cancellation.
