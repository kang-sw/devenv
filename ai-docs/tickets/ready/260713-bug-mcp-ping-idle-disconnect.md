---
title: Restore ws MCP liveness across Claude Code idle periods
related:
  260513-research-streamable-http-mcp-transport: longer-term reconnect-capable transport direction; this ticket keeps the current stdio boundary
related-mental-model:
  - mcp-runtime
  - plugin-runtime
sage-review-design: completed
sage-review-completeness: completed
---

# Restore ws MCP liveness across Claude Code idle periods

## Background

On native Windows with Claude Code 2.1.207, long-lived Claude sessions were
still running while their wsflow MCP children had disconnected and required
manual restart. The ws/wsflow MCP logs contain 461 explicit
`Terminating MCP server process tree` events. Among them, 130 no-call sessions
terminated approximately 15 minutes after connection, and 12 active sessions
terminated approximately 15 minutes after their last MCP activity.

The ws runtime itself has no idle timer: `ServeStdio` blocks on stdin until EOF
or an error, and the Windows launcher waits for the runtime child without an
idle deadline. Windows anonymous pipes likewise do not expire merely because
they are inactive. These findings make host-side liveness handling more likely
than an OS pipe timeout.

ws advertises MCP protocol version `2025-03-26` but currently handles only
`initialize`, `tools/list`, and `tools/call`; a `ping` request receives method
not found. That protocol version requires a ping receiver to promptly return an
empty result and permits the sender to terminate a connection that fails its
liveness check. Claude's logs do not expose raw ping traffic or a termination
reason, so missing ping support is a strong candidate rather than a proven
cause.

## Decisions

- Restore protocol-compliant `ping` handling as the first bounded remedy and
  use a Windows idle A/B check to test the causal hypothesis.
- Do not emit blank lines or other unsolicited stdout traffic as a keepalive;
  stdout remains newline-delimited JSON-RPC only.
- Keep this fix on the current stdio transport. Streamable HTTP reconnect and
  daemon lifecycle remain with `260513-research-streamable-http-mcp-transport`.
- If a compliant ping response does not prevent the disconnect, record a
  separate follow-up from the observed lifecycle evidence instead of expanding
  this ticket into speculative launcher or transport work.

## Constraints

- `ping` is a base-protocol method, not an MCP tool; it must not change
  `tools/list`, runtime tool inventories, or ws/wsflow product-mode filtering.
- Preserve existing concurrent request handling and serialized response writes.
- Keep the response protocol-exact: the request id is preserved and `result` is
  an empty object.
- Verification must distinguish process survival from session-state recovery;
  persisted ws session keys may survive a restarted process and therefore do
  not prove that the original stdio connection stayed alive.

## Spec Impact

- Target: `ai-docs/spec/mcp-tools.md` — MCP Server Protocol Surface.
- Expected change: include protocol-compliant `ping` request handling in the
  advertised stdio base-protocol behavior.
- Contract-first spec: no — the external MCP 2025-03-26 contract already fixes
  the response shape; implementation and its A/B result should update the local
  conformance description during closeout.

## Phases

### Phase 1: Add ping handling and validate idle liveness

Add base-protocol `ping` dispatch that returns an empty JSON-RPC result with the
original request id. Add focused stdio tests covering the exact response and
confirm that no tool schema or runtime inventory changes. Run the existing Go,
plugin, and wsflow package test suites.

On native Windows Claude Code, connect wsflow, leave the MCP connection without
tool activity for at least 20 minutes, then verify that the original Python and
ws-mcp process ids remain alive and that a subsequent tool call succeeds
without manual MCP restart. Record the Claude Code version, timestamps, process
ids, and relevant MCP log tail. If the process is still terminated, close this
phase only for protocol conformance and capture the remaining host-lifecycle
failure as a separate evidence-backed follow-up.
