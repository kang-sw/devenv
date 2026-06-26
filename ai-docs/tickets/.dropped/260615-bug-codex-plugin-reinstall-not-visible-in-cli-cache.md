---
title: Codex plugin reinstall leaves ws absent from CLI installed state and cache
related:
  260525-bug-codex-local-marketplace-worktree-cache-regression: adjacent local marketplace cache fidelity risk
  260524-bug-codex-plugin-cache-refresh-mcp-startup-race: adjacent partial materialization failure class
related-mental-model:
  - plugin-runtime
---

# Codex plugin reinstall leaves ws absent from CLI installed state and cache

## Background

Dogfood surprise on 2026-06-15 after the user reported reinstalling the `ws`
plugin. The active Codex session did not expose any `ws:*` skills or `ws` MCP
tools, and the CLI/cache state also showed no installed `ws` plugin:

```text
$ codex mcp list
No MCP servers configured yet. Try `codex mcp add my-tool -- my-command`.

$ codex mcp get ws
Error: No MCP server named 'ws' found.

$ codex plugin list
ws@kang-sw-devenv      not installed           /home/swkang/devenv/agents-plugin
wsflow@kang-sw-devenv  not installed           /home/swkang/devenv/agents-plugin-wsflow
```

The local marketplace source was still configured:

```text
[marketplaces.kang-sw-devenv]
source_type = "local"
source = "/home/swkang/devenv"
```

But `~/.codex/plugins/cache/kang-sw-devenv/ws` did not exist. The only
`kang-sw-devenv` cache artifact present was an old backup path:

```text
~/.codex/plugins/cache/kang-sw-devenv/plugin-backup-iwl5JA/ws/0.22.3/.runtime/linux-amd64/ws-mcp
```

## Why it matters

The user-facing reinstall action appears to have completed from the user's
point of view, but the CLI installed-state, plugin cache, current session skill
inventory, and MCP registry all agree that `ws` is not installed. This is a
different failure shape from a stale installed version or a partially
materialized runtime package: the installed package is absent.

## Research Questions

- Did the UI reinstall action fail silently, or does the CLI read a different
  plugin installed-state store than the UI/app-server path?
- Does `codex plugin add ws@kang-sw-devenv --json` reproduce a successful
  install from the same marketplace source, and if so why did the reported UI
  reinstall not persist?
- Should ws dogfood add a post-reinstall acceptance check that verifies
  `codex plugin list`, `codex mcp list`, the installed cache directory, and the
  current session skill inventory agree before trusting a reload?

## Notes

This ticket is intentionally separate from
`260525-bug-codex-local-marketplace-worktree-cache-regression`, which concerns
wrong-source or sibling-worktree cache selection. This observation is about the
plugin being absent from installed state after an expected reinstall.

## Dropped - 2026-06-15

User clarified that the plugin had not actually been installed before the check.
The observed absent CLI/cache/MCP state was therefore expected, not a Codex
reinstall or cache failure.
