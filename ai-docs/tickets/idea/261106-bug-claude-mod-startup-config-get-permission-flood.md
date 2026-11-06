---
title: "Claude mod: startup config_get calls raise a flood of permission dialogs"
---

# Claude mod: startup config_get calls raise a flood of permission dialogs

## Observation (2026-11-06, dogfood)

During the Phase 1 event probe of 261106-feat-ws-dashboard-claude-adapter, the
probe worker started an interactive Claude Code 2.1.319 session in default
permission mode. The session ran in a fresh directory that is not a ws project,
and `~/.claude/settings.json` had no `mcp__plugin_ws_ws__*` allow rule. The ws
Claude mod's own startup `config_get` calls raised about 11 person-approved
permission dialogs at session start (`tool.check` ask with `approver: "person"`).
The log is kept at
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/shared/claude-mod-event-probe/probe-log-interactive-permission.jsonl`.

A downstream user in default mode without an allow rule would likely see the
same flood at every session start, even in a project that never uses ws.

## Possible direction

- Read the mod's knobs through one call (or a non-tool path such as a file or
  the MCP server's runtime data) instead of one `config_get` per knob.
- Or have the mod's plugin-raised tool calls bypass the permission check: the
  mod is trusted code, and a `tool.check` hook it owns could allow its own
  read-only calls.
- Or have install guidance add a read-only allow rule.
- Confirm first whether the count scales with the number of `claude.*` knobs.
