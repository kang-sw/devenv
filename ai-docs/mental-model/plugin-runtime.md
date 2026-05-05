---
domain: plugin-runtime
description: "Codex plugin packaging, launcher repair, and runtime compatibility contracts."
sources:
  - agents-plugin/
  - agents-plugin/bin/
related:
  mcp-runtime: "Launcher compatibility probes depend on ws-mcp runtime.info, tools/list, and CLI command behavior."
  prompt-bundle: "runtime.json prompt bundle metadata must match embedded prompt content."
---

# Plugin Runtime

## Entry Points

- `agents-plugin/.codex-plugin/plugin.json` is the Codex-visible plugin manifest and points at the skill bundle and MCP config. {#260505-codex-plugin-manifest-skill-bundle}
- `agents-plugin/.mcp.json` is the plugin-managed MCP startup contract; `cwd: "."` makes `./bin/ws-mcp-launcher` resolve relative to the installed plugin cache. {#260505-plugin-local-mcp-server-config}
- `agents-plugin/bin/ws-mcp-launcher` owns runtime lookup, compatibility checks, release download, checksum verification, local dev runtime repair, and final `exec`. {#260505-runtime-launcher-repair-project-root}
- `agents-plugin/runtime.json` is active compatibility data, not descriptive metadata. {#260505-runtime-contract-metadata}

## Module Contracts

- The launcher writes diagnostics to stderr only; any stdout before `exec "$binary" "$@"` corrupts stdio MCP JSON-RPC.
- `.mcp.json` cwd is the plugin cache, so repo root defaults must flow through `WS_MCP_PROJECT_ROOT`, not process cwd.
- `runtime.json` tool names, command names, and prompt bundle metadata are compared against the binary by the launcher; stale names or prompt metadata can make a working binary get replaced or rejected.
- Release repair requires matching binary asset names and `SHA256SUMS`; local dev repair is gated to the local installed plugin cache, `.local-devenv-runtime`, and non-Windows platforms. {#260505-release-asset-build-checksum-pipeline}
- Version changes are multi-file and should use the bump helper rather than editing manifests manually. {#260505-runtime-version-bump-helper}
- Runtime install/repair has explicit override env vars: `WS_MCP_RUNTIME_DIR`, release repository/tag/base URL overrides, and bootstrap binary/URL/checksum overrides.

## Coupling

- Add or rename an MCP tool: update `internal/mcp/server.go` dispatch, `tools()` schema, tests, `agents-plugin/runtime.json`, and any skill guidance that names the tool.
- Add or rename a CLI command: update `cmd/ws-mcp/main.go`, command tests, `runtime.json.commands`, launcher command probing assumptions, and docs. {#260505-runtime-cli-entrypoints}
- Edit embedded prompts: update `agents-plugin/runtime.json` prompt bundle hash/list or build release assets so the script rewrites it.
- Change `.mcp.json` timeouts or command path: verify installed plugin cache startup, not only source-tree execution.

## Extension Points & Change Recipes

- **Add a plugin skill**: add `agents-plugin/skills/<name>/SKILL.md`; keep the manifest pointing at `./skills` unless the bundle layout changes.
- **Add a runtime requirement**: extend `runtime.json`, then add requirement-specific launcher checks; use `runtime.info` only for runtime metadata the binary can report.
- **Change runtime version**: run `agents-plugin-tool/scripts/bump-ws-version.sh`, then verify manifests, launcher compatibility glob, build script, workflow, and docs changed together.

## Common Mistakes

- Removing `.mcp.json` `cwd: "."` makes Codex resolve `./bin/ws-mcp-launcher` from the workspace instead of the plugin cache.
- Treating `runtime.json` as release notes leaves launcher repair with stale tool, command, or prompt-bundle expectations.
- Printing debug output to stdout from the launcher breaks MCP startup.
- Assuming Windows plugin-managed startup is complete because Windows `ws-mcp.exe` assets exist; native plugin-managed startup remains planned. {#260505-windows-plugin-managed-startup}

## Technical Debt

- Launcher JSON parsing is shell/sed based and shape-sensitive; changing `runtime.json` structure can break compatibility checks without compiler coverage.
- Version compatibility is declared in `runtime.json.required_mcp` but enforced by a launcher glob; the bump helper keeps that declaration, launcher logic, and release/version defaults aligned.
