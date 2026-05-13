---
title: Dual MCP startup ordering and reconnect behavior
related:
  260513-research-streamable-http-mcp-transport: prerequisite validation for doctor-plus-http transport migration
related-mental-model:
  - mcp-runtime
  - plugin-runtime
  - claude-compatibility
---

# Dual MCP startup ordering and reconnect behavior

## Background

The current plausible path toward reliable HTTP MCP is a dual-server shape:

- `ws-doctor` remains a lightweight stdio MCP server that the host can spawn
  automatically.
- `ws` becomes, or can optionally become, a fixed-port HTTP MCP server that gets
  the host's HTTP reconnect behavior.
- `ws-doctor` ensures, repairs, or reports the fixed-port HTTP daemon state.

This depends on host behavior that is not yet verified. If Claude Code or Codex
starts configured MCP servers concurrently, an HTTP `ws` entry may attempt to
connect before `ws-doctor` has ensured the daemon. The design only works if the
host either starts MCP servers in a usable order or retries the failed HTTP
connection after the doctor-side daemon becomes available.

## Questions

- When multiple MCP servers are configured, does the host initialize them
  sequentially, concurrently, or in an unspecified order?
- If a stdio MCP server and an HTTP MCP server are both configured, can the
  stdio server run early enough to start or repair the HTTP server before the
  host gives up on the HTTP entry?
- If the HTTP server is initially unavailable but becomes available shortly
  afterward, does the host retry automatically?
- Does retry behavior differ between initial startup failure and mid-session
  disconnection?
- Does behavior differ between Claude Code project/user/plugin-provided MCP
  configuration and Codex CLI/plugin-managed MCP configuration?
- Can plugin hooks, monitors, or other host startup surfaces run before MCP HTTP
  connection attempts, or are MCP startup attempts earlier than those surfaces?

## Validation Sketch

Create a minimal disposable plugin or project-local MCP configuration with:

- `doctor-test`: a stdio MCP server that records timestamps and starts a simple
  local HTTP MCP test server after a configurable delay.
- `main-test`: an HTTP MCP server entry pointing at the fixed local test port.

Run the matrix against Claude Code and Codex where possible:

- HTTP server already running before host startup.
- HTTP server starts immediately from the stdio doctor process.
- HTTP server starts after a short delay that should fall within documented
  initial HTTP retry windows.
- HTTP server starts after the expected retry window to confirm failure mode.
- HTTP server dies mid-session and restarts to verify reconnect behavior.

Record:

- observed startup order,
- retry timing,
- whether tools become available without manual `/mcp` retry,
- whether plugin-provided MCP behaves differently from project or user config,
- logs needed to diagnose failures.

## Current Working Assumption

This validation is a prerequisite before committing to a `ws-doctor` stdio MCP
plus fixed-port HTTP `ws` architecture. If hosts do not retry HTTP startup after
doctor-side repair, the architecture still helps manual repair but cannot
provide automatic HTTP startup from plugin installation alone.
