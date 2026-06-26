---
title: Add `hide` value to prefer_mercenary config — suppress mercenary surface in full ws mode
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260620-bug-mercenary-path-visible-when-prefer-off: related surface visibility issue
---

# Add `hide` value to prefer_mercenary config — suppress mercenary surface in full ws mode

## Background

`prefer_mercenary` currently accepts `on`/`off`, controlling whether the lead agent
prefers mercenary dispatch over native subagents. The mercenary MCP surface
(`ws.mercenary.*` tools) is always visible in full ws mode regardless of this setting.

In wsflow/no-agent mode (`WS_MCP_NO_AGENT=1`), mercenary tools are already hidden via
`noAgentHiddenTool`. But a user running full ws who wants to operate native-subagent-only
has no way to suppress the mercenary surface without opting into the entire no-agent mode.

## Proposal

Extend `prefer_mercenary` to accept three values: `on | off | hide`.

- `on` — current behavior: prefer mercenary dispatch; tools visible.
- `off` — current behavior: prefer native; tools visible (lead can still use them).
- `hide` — suppress mercenary surface entirely: `ws.mercenary.*` tools removed from
  `tools/list` and blocked in `toolAllowed()`. Lead agent never sees or routes to
  mercenary.

This mirrors the pattern just established for exec tools (`permanentlyHiddenTool`):
tools hidden from discovery AND from call-gate.

## Motivation

- Users running fresh-agent-centric workflows have no reason to see mercenary tools
  in the tool list; their presence adds noise and can mislead the lead agent.
- `hide` gives a clean "native-only" mode without the full wsflow/no-agent surface
  reduction (which also hides exec and other tools the user may still want).
- Config-driven: follows the existing layered config resolution
  (`session > project > global > builtin`), so a project or user can opt in.

## Constraints

- `hide` must gate both `filteredTools()` (discovery) and `toolAllowed()` (call-gate),
  consistent with the exec permanent-hide pattern.
- `LeadToolNames()` must also filter mercenary tools when `hide` is active.
- Default remains `off` (no behavior change for existing users).
- wsflow no-agent mode (`WS_MCP_NO_AGENT=1`) continues to hide mercenary unconditionally
  regardless of this config value.

## Notes

- `prefer_mercenary` is currently `ScopeSession`; `hide` may warrant `ScopeProject` or
  `ScopeGlobal` as the natural scope (a deployment-level policy). Evaluate at
  implementation time.
- This is an idea ticket; promote to ready after design review.
