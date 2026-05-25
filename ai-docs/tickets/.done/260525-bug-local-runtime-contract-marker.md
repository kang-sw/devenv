---
title: Local runtime marker should carry an explicit build contract
spec:
  - 260505-runtime-launcher-repair-project-root
related-mental-model:
  - plugin-runtime
---

# Local runtime marker should carry an explicit build contract

## Background

`agents-plugin/.local-devenv-runtime` intentionally enables local dogfood so a
developer can run the latest local `ws-mcp` runtime from an installed Codex
plugin cache without shipping first. After `v0.29.2`, the marker remained a
boolean file, so plugin-managed MCP startup depended on the launcher inferring
`~/devenv/agents-plugin-tool` and finding `go` through the MCP process
environment. When that inference failed, the forced local path could not install
a compatible runtime and the MCP server stayed down.

The marker should become an explicit contract copied into the plugin cache. A
valid contract activates forced local runtime repair. An invalid, stale, or
machine-inapplicable contract should disable local repair and fall back to the
ordinary release/cache path.

## Phases

### Phase 1: Local runtime contract file

Implement `.local-devenv-runtime` as a JSON contract with:

- `schema_version`: current value `1`.
- `source_root`: absolute path to the local checkout.
- `tool_dir`: absolute path to the `agents-plugin-tool` module.
- `go`: absolute path to the Go executable.

The launcher must:

- Treat a missing marker as the normal release/cache path.
- Treat an invalid contract, missing required path, absent Go executable, absent
  tool directory, or non-absolute path as inactive local repair and use the
  normal release/cache path.
- Force local source build only when the contract is valid.
- Fail rather than fall back to release when a valid active contract exists but
  the local build runs and produces no compatible runtime.
- Continue to avoid release download fallback for explicit
  `WS_MCP_BOOTSTRAP_BINARY` / `WS_MCP_BOOTSTRAP_URL` forced paths.
- Mirror launcher behavior to `agents-plugin-wsflow/`.

Rejected alternatives:

- Do not infer `go` or the checkout path from the MCP process `PATH`; Codex
  startup environments can differ from the user's interactive shell.
- Do not use file mtime to decide whether the marker is valid; validity should
  come from the contract content.

Verification:

- Unit tests cover valid contract activation, invalid contract release/cache
  fallback, valid-contract build failure with no release fallback, and source
  build precedence over local distribution assets.
- Run `python3 -m unittest discover agents-plugin/tests`.
- Run `python3 -m unittest discover agents-plugin-wsflow/tests`.

### Result

Implemented. `.local-devenv-runtime` is now a JSON contract that activates local
dogfood only when it names absolute `source_root`, `tool_dir`, and executable
`go` paths. Invalid or stale contracts are inactive and fall back to the normal
cache/release path. Valid active contracts force a local source build and fail
startup rather than silently using release assets when the build cannot produce
a compatible runtime.

Verification run:

- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `python3 -m py_compile agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
- `cmp -s agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
- Installed-cache launcher smoke with `WS_MCP_LAUNCHER_DEBUG=1 ... runtime info`
