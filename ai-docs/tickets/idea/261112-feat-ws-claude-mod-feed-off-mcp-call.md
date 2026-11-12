---
title: "Claude mod: send the dashboard feed and drains to the daemon's local HTTP instead of $.mcp.call"
---

# Claude mod feed off `$.mcp.call`

## Background

Observed 2026-11-12 (whiteboard topic `hook-alarm`): Orca shows idle/working
alarm banners for a ws lead pane on every tool call. Orca's trace
(`~/Library/Application Support/orca/logs/main.trace.ndjson`) logged 3,375
`agent_finish_dropped` (`superseded-snapshot`) breadcrumbs for that pane at
about 0.2 s cadence, matching the Claude mod's dashboard intake cadence
(`agents-plugin/hooks/claude-mod/dashboard.ts`, `FLUSH_INTERVAL_MS`).

- Orca registers command hooks on PreToolUse, PostToolUse, Stop, and others
  (`~/.orca/agent-hooks/claude-hook.sh`), forwarding every event to its app.
- A probe (`ai-docs/ref/claude-mcp-call-probe-mod/runs/6-auto-mode-debug-excerpt.txt`)
  shows a mod `$.mcp.call` skips the auto-mode classifier ("called by
  plugin") but runs the `tool.call` / `tool.check` pipeline.
- Hypothesis, unconfirmed: each mod `$.mcp.call` (feed intake, inbox drain,
  lookups in `register.ts`) fires the session's PreToolUse/PostToolUse
  command hooks, so every hook-consuming tool (Orca here) sees a tool call
  per feed flush. Confirming experiment: whiteboard item (615), alarms in a
  Claude session without ws.
- Mitigation landed separately: the feed flush gap was raised (616 b).

## Decisions

1. **Move the mod's high-frequency calls off `$.mcp.call`** once (615)
   confirms the hypothesis: the dashboard feed intake and the inbox drains
   go to the ws daemon's local HTTP API directly, so they never enter Claude
   Code's tool pipeline or its hooks. (User, (616) c.) Rejected: only
   lowering the cadence (still fires hooks, just less often).

## Constraints

- Mechanism (endpoint, how the mod learns the daemon endpoint and token,
  how the call carries the session identity the MCP connection carries
  today, compatibility tier) is not settled; settle it through the Open
  Decision Queue before `todo`/`ready`.
- If (615) shows alarms without ws, this ticket is dropped.
