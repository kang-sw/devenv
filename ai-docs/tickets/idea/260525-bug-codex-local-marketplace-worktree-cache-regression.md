---
title: Codex local marketplace refresh can regress ws plugin cache from a sibling worktree
related:
  260524-bug-codex-plugin-cache-refresh-mcp-startup-race: adjacent cache materialization race, but not the same version-source regression
related-mental-model:
  - plugin-runtime
---

# Codex local marketplace refresh can regress ws plugin cache from a sibling worktree

## Background

Local dogfooding showed the installed Codex `ws@kang-sw-devenv` plugin cache
moving from `ws/0.29.2` back to `ws/0.29.1` while the user expected the global
marketplace source in `~/.codex/config.toml` to remain authoritative:

```text
[marketplaces.kang-sw-devenv]
source_type = "local"
source = "/Users/kang-sw/devenv"
```

The main worktree source and installed cache had been observed at `0.29.2`, but
the sibling dashboard worktree still carried `agents-plugin/runtime.json` and
`.codex-plugin/plugin.json` at `0.29.1`. After worktree-local Codex activity, the
global cache path contained only:

```text
/Users/kang-sw/.codex/plugins/cache/kang-sw-devenv/ws/0.29.1
```

Running processes also showed mixed MCP runtimes: newer Codex sessions launched
`.../ws/0.29.2/.../ws-mcp-0.29.2...`, while an older worktree session launched
`.../ws/0.29.1/.../ws-mcp-0.29.1...`.

This suggests Codex local marketplace refresh may consult a worktree-local
`.agents/plugins/marketplace.json` with the same marketplace id and relative
`./agents-plugin` path, then materialize that worktree's plugin version into the
user-global cache. If confirmed, sibling worktrees can downgrade the enabled
plugin cache despite a global marketplace source pointing at the main checkout.

## Reproduction Notes

- Main source: `/Users/kang-sw/devenv/agents-plugin` at `0.29.2`.
- Sibling worktree source:
  `/Users/kang-sw/devenv/.git/wt/ws-dashboard-dev/agents-plugin` at `0.29.1`.
- Both `.agents/plugins/marketplace.json` files use marketplace name
  `kang-sw-devenv` and relative plugin path `./agents-plugin`.
- `codex mcp get ws` reported `cwd:
  /Users/kang-sw/.codex/plugins/cache/kang-sw-devenv/ws/0.29.1/.` after the
  regression.

## Research Questions

- Does Codex local marketplace refresh prefer the nearest worktree
  `.agents/plugins/marketplace.json` over the configured
  `~/.codex/config.toml` source, or did another explicit refresh path install
  from the sibling worktree?
- Is the cache key only `<marketplace-name>/<plugin-name>/<plugin-version>`,
  making same-named local marketplaces from sibling worktrees overwrite each
  other?
- Should ws dogfood use a unique marketplace id per worktree, an absolute source
  path in marketplace metadata, or a post-refresh guard that verifies cache
  source/version before trusting the installed MCP server?
