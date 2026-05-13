---
title: Streamable HTTP MCP transport for ws runtime
related:
  260429-research-host-neutral-ws-plugin: host-neutral plugin architecture anchor
related-mental-model:
  - mcp-runtime
  - plugin-runtime
  - claude-compatibility
---

# Streamable HTTP MCP transport for ws runtime

## Background

The current ws MCP runtime is stdio-centered. Claude Code plugin manifests start
`ws-mcp serve --stdio` through the package launcher, so one spawned MCP process
implicitly maps to one host session. That model keeps several stateful runtime
points simple because process lifetime and session lifetime are effectively the
same boundary.

Discussion on 2026-05-13 found that Claude Code documentation does not expose a
stdio-specific automatic reconnect or restart setting. Current documented
automatic reconnection behavior applies to HTTP/SSE-style MCP connections, while
stdio servers are local child processes whose failed pipe is not automatically
reconnected. This makes long-running or remote plugin-managed stdio sessions a
known reliability concern.

MCP Streamable HTTP provides a standard session mechanism through
`Mcp-Session-Id`. A server may return that header on initialize, and a client
that receives it must include it on later HTTP requests. The transport also
supports stream resume with event ids and `Last-Event-ID`. This makes HTTP a
plausible long-term direction, but not a small transport-only swap.

## Research Questions

- Confirm which HTTP transport shape Claude Code plugin manifests support today:
  legacy HTTP+SSE, current Streamable HTTP, or both.
- Confirm whether plugin-provided MCP servers can be HTTP endpoints managed by a
  plugin-local launcher, or whether HTTP endpoints must be started and managed
  outside plugin installation.
- Decide whether ws should initially run HTTP as a per-project daemon, a
  per-plugin-version daemon, or a user-level daemon that multiplexes projects.
- Classify existing runtime state into process-scoped, project-scoped,
  session-scoped, and job-scoped ownership.
- Decide how `ws.setup(root)`, harness detection, cancellation maps, debug
  events, API async jobs, and named-agent runtime state should behave after a
  client reconnects with the same `Mcp-Session-Id`.
- Decide what state, if any, must survive daemon restart versus only client
  reconnect.

## Current Direction

Treat Streamable HTTP as the likely long-term transport for reliable remote and
long-running MCP sessions. Prefer designing against the standard
`Mcp-Session-Id` boundary instead of inventing a ws-specific session protocol.

The conservative migration shape is probably:

1. Add a runtime research spike that maps current stateful code paths.
2. Add `ws-mcp serve --http` with local health and smoke coverage.
3. Add daemon lifecycle management around version compatibility, port or socket
   discovery, stale daemon replacement, and project root binding.
4. Only then change plugin manifests or installer behavior.

## Risks

- A naive HTTP daemon could leak volatile root or harness state across projects
  or clients.
- A per-user daemon reduces process churn but makes project isolation harder.
- A per-project daemon preserves current assumptions better but still needs
  session-scoped maps for reconnect correctness.
- Plugin-managed HTTP may require lifecycle behavior that Claude Code or Codex
  plugin manifests do not standardize.
- Implementing legacy SSE first could create migration debt if Streamable HTTP
  is the real long-term target.
