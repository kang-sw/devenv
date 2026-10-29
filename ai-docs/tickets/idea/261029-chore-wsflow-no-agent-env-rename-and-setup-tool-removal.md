---
title: Rename WS_MCP_NO_AGENT and remove the unread WS_MCP_SETUP_TOOL env
related:
  261029-feat-claude-mod-package-parity-knob-defaults: origin; deferred from its scope
---

# Rename WS_MCP_NO_AGENT and remove the unread WS_MCP_SETUP_TOOL env

## Background

After `261029-feat-claude-mod-package-parity-knob-defaults` unblocks CLI
`config tune` in wsflow, `WS_MCP_NO_AGENT` gates only the `playbook.render`
legacy context bridge (`wsflowRenderEligibleStems`), so its name no longer
describes what it does, and wsflow is no longer framed as agentless.
`WS_MCP_SETUP_TOOL` is set in wsflow's `.mcp.json` and
`.claude-plugin/plugin.json`, but no source under `agents-plugin-tool/`
reads it (grep of `internal/` and `cmd/`, 2026-10-29).

Open: whether the render bridge itself should be retired (which would remove
the last reason for the env), or the env renamed to name the bridge.
