---
domain: plugin-runtime
description: "Codex plugin packaging, launcher repair, and runtime compatibility contracts."
sources:
  - agents-plugin/
  - agents-plugin-wsflow/
  - agents-plugin/bin/
related:
  mcp-runtime: "Launcher compatibility uses runtime.capabilities first, with runtime.info, tools/list, and CLI probes as fallback checks."
  prompt-bundle: "runtime.json prompt bundle metadata must match embedded prompt content."
---

# Plugin Runtime

## Entry Points

- `agents-plugin/.codex-plugin/plugin.json` is the Codex-visible plugin manifest and points at the skill bundle and MCP config. {#260505-codex-plugin-manifest-skill-bundle}
- `agents-plugin/.mcp.json` is the plugin-managed MCP startup contract; `cwd: "."` makes `./bin/ws-mcp-launcher.py` resolve relative to the installed plugin cache. {#260505-plugin-local-mcp-server-config}
- `agents-plugin/.claude-plugin/plugin.json` is a Claude-facing compatibility manifest for the Codex-first candidate and should start the same Python launcher path, not the POSIX shell wrapper, so native Windows does not require `/bin/sh`. {#260505-plugin-local-mcp-server-config}
- `agents-plugin-wsflow/` is the scaffolded agentless derivative package; its `.mcp.json` selects `WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`, and `WS_MCP_SETUP_TOOL=setup`. {#260513-wsflow-agentless-plugin-package}
- `agents-plugin/bin/ws-mcp-launcher.py` owns runtime lookup, compatibility checks, release download, checksum verification, local dev runtime repair, and final handoff. {#260505-runtime-launcher-repair-project-root}
- `agents-plugin/runtime.json` is active compatibility data, not descriptive metadata. {#260505-runtime-contract-metadata}

## Module Contracts

- The launcher writes diagnostics to stderr only; any stdout before `exec "$binary" "$@"` corrupts stdio MCP JSON-RPC.
- `.mcp.json` cwd is the plugin cache, so repo root defaults must flow through `WS_MCP_PROJECT_ROOT`, not process cwd.
- `runtime.json` tool names, command names, and prompt bundle metadata are compared against the binary by the launcher; stale names or prompt metadata can make a working binary get replaced or rejected.
- The launcher tries `ws-mcp runtime capabilities` as the single-process fast path and accepts only a complete JSON payload matching version, MCP protocol, prompt bundle hash, required lead tools, and required CLI commands. Contracts can opt into `runtime_capabilities.match: exact`; exact contracts reject extra tools or commands and do not use weaker fallback validation after a capability mismatch. Missing, invalid, or partial capability output falls through to bounded legacy validation only for non-exact contracts. {#260506-runtime-capabilities-single-probe}
- Runtime capability output is mode-sensitive only for explicit product modes such as wsflow no-agent; tool-profile and allowed-tool filters remain ignored for the full ws launcher contract. {#260513-wsflow-runtime-contract-mode}
- The launcher's `.compatibility.json` stamp is a hot-path optimization, not a new trust boundary: only an exact stamp keyed to `runtime.json` content plus resolved binary path, size, and mtime can skip full validation; unreadable, missing, or mismatched stamps fail closed into `runtime_fully_compatible`. {#260506-launcher-hot-path-compatibility-cache}
- Release repair requires matching binary asset names and `SHA256SUMS`; local dev repair is gated to the local installed plugin cache, `.local-devenv-runtime`, and non-Windows platforms. {#260505-release-asset-build-checksum-pipeline}
- Windows release smoke should run the built executable once through `smoke --root <repo>` so local AV tools and CI both exercise one runtime process. {#260505-release-asset-build-checksum-pipeline}
- Install and repair paths still clear stale compatibility stamps and run full validation before handoff, then write a fresh stamp only after success. {#260506-launcher-hot-path-compatibility-cache}
- Version changes are multi-file and should use the bump helper rather than editing manifests manually. {#260505-runtime-version-bump-helper}
- Runtime install/repair has explicit override env vars: `WS_MCP_RUNTIME_DIR`, release repository/tag/base URL overrides, and bootstrap binary/URL/checksum overrides.
- The launcher exports `WS_MCP_RUNTIME_BINARY`; named-agent async workers use it to avoid spawning from stale plugin-cache executables after a plugin refresh.

## Coupling

- Add or rename an MCP tool: update `internal/mcp/server.go` dispatch, `tools()` schema, tests, `agents-plugin/runtime.json`, and any skill guidance that names the tool; the capabilities fast path will compare `runtime.json` against the lead tool registry.
- Add or rename a CLI command: update `cmd/ws-mcp/main.go`, `runtimeCapabilityCommandNames`, command tests, `runtime.json.commands`, launcher command probing assumptions, and docs. {#260505-runtime-cli-entrypoints}
- Edit embedded prompts: update `agents-plugin/runtime.json` prompt bundle hash/list or build release assets so the script rewrites it.
- Change `.mcp.json` timeouts or command path: verify installed plugin cache startup, not only source-tree execution.

## Extension Points & Change Recipes

- **Add a plugin skill**: add `agents-plugin/skills/<name>/SKILL.md`; keep the manifest pointing at `./skills` unless the bundle layout changes.
- **Add a runtime requirement**: extend `runtime.json`, then add requirement-specific launcher checks; use `runtime.info` only for runtime metadata the binary can report.
- **Change wsflow packaging**: keep `.mcp.json` env, exact `runtime.json`, and the package contract test aligned with no-agent `runtime.capabilities`.
- **Change runtime version**: run `agents-plugin-tool/scripts/bump-ws-version.sh`, then verify manifests, launcher compatibility glob, build script, workflow, and docs changed together.

## Common Mistakes

- Removing `.mcp.json` `cwd: "."` makes Codex resolve `./bin/ws-mcp-launcher.py` from the workspace instead of the plugin cache.
- Treating `runtime.json` as release notes leaves launcher repair with stale tool, command, or prompt-bundle expectations.
- Treating the compatibility stamp as sufficient after changing validation logic, `runtime.json`, or the binary identity; the stamp should force a miss unless the validated contract/binary pair is unchanged.
- Adding a launcher-required CLI command only to `runtime.json.commands`; the capabilities fast path has a separate manually maintained command list.
- Printing debug output to stdout from the launcher breaks MCP startup.
- Assuming Windows plugin-managed startup works without `python3`; the shared launcher needs an installed Python 3 interpreter on native Windows. {#260505-windows-plugin-managed-startup}

## Technical Debt

- Launcher JSON parsing is Python stdlib based; changing `runtime.json` structure can still break launcher compatibility checks without Go compiler coverage.
- Version compatibility is declared in `runtime.json.required_mcp` and enforced by the launcher against the plugin major/minor version.
