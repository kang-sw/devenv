# Survey: 06-runtime-capabilities-probe

## Reusable Components
- `agents-plugin/bin/ws-mcp-launcher.py#L64-L69` — `version_compatible`: compares runtime and plugin major/minor versions; the capabilities fast path should preserve the same version gate.
- `agents-plugin/bin/ws-mcp-launcher.py#L96-L101` — `runtime_tools` / `runtime_commands`: sorted contract-surface extractors for `runtime.json`; use these instead of duplicating key parsing.
- `agents-plugin/bin/ws-mcp-launcher.py#L104-L112` — `run_binary`: existing subprocess wrapper with captured stdout/stderr and timeout; fits the single `runtime capabilities` invocation.
- `agents-plugin/bin/ws-mcp-launcher.py#L115-L192` — `tools_compatible`, `commands_compatible`, `prompt_bundle_compatible`: legacy fallback validators and expected semantics for tool, command, and prompt-bundle checks.
- `agents-plugin-tool/internal/mcp/server.go#L68-L84` — `ProtocolVersion` / `NewServer`: canonical MCP protocol constant and environment-derived role setup; capabilities should use the constant, not duplicate the string.
- `agents-plugin-tool/internal/mcp/server.go#L1390-L1400` — `LeadToolNames`: returns sorted names directly from `tools()` and ignores profile filters; already matches the full-lead-surface requirement.
- `agents-plugin-tool/internal/wsprompt/prompts.go#L78-L91` — `Bundle`: canonical prompt bundle metadata used by `runtime.info`; `runtime capabilities` already calls it.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L606-L614` — `printJSONOrFatal`: standard CLI JSON output helper that keeps success output as one JSON line.

## Existing Patterns
- Runtime subcommand dispatch: see `agents-plugin-tool/cmd/ws-mcp/main.go#L99-L117` — `runtime info` and `runtime capabilities` share a nested switch and stderr usage errors.
- Runtime metadata output: see `agents-plugin-tool/cmd/ws-mcp/main.go#L119-L135` — `runtime info` gathers `wsprompt.Bundle(sourceCommit)` and prints compatibility metadata.
- CLI command surface declaration: see `agents-plugin/runtime.json#L77-L114` — command keys use dotted names, with hyphenated CLI subcommands preserved (`agents.debug.runtime-log`, `git.merge-base`, `config.agents-tier`).
- Legacy command probing: see `agents-plugin/bin/ws-mcp-launcher.py#L155-L175` — dotted command names are converted into parent command invocations plus expected usage text, not necessarily full operational calls.
- Compatibility stamp flow: see `agents-plugin/bin/ws-mcp-launcher.py#L449-L461` — stamps are written only after `runtime_fully_compatible` succeeds, and repair clears stale stamps before revalidation.
- Tool profile filtering tests: see `agents-plugin-tool/internal/mcp/server_test.go#L496-L564` — profile and allowlist behavior is intentionally narrower than lead, while `runtime.info` stays visible.
- Capability skeleton tests: see `agents-plugin-tool/cmd/ws-mcp/main_test.go#L38-L90` and `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py#L18-L62` — acceptance already covers full lead surface under leaf env and fast-path-vs-fallback launcher behavior.

## Relevant Interfaces
- `agents-plugin-tool/cmd/ws-mcp/main.go#L137-L163` — `runtimeCapabilitiesPayload` / `runtimeCapabilities`: required JSON shape and current command-list TODO.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L165-L169` — `runtimeCapabilityCommandNames`: stub responsible for enumerating public CLI commands expected by `runtime.json.commands`.
- `agents-plugin/bin/ws-mcp-launcher.py#L353-L370` — `runtime_capabilities_compatible` / `runtime_fully_compatible`: fast-path stub and fallback orchestration point.
- `agents-plugin/runtime.json#L1-L10` — runtime contract fields for plugin version, required MCP range, MCP protocol, release tag, and prompt bundle hash.
- `agents-plugin/runtime.json#L30-L75` — required MCP tool surface to compare against `tools` in the capabilities payload.
- `agents-plugin/runtime.json#L77-L114` — required CLI command surface to compare against `commands` in the capabilities payload.
- `agents-plugin-tool/internal/mcp/server.go#L863-L1387` — `tools()` registry: canonical MCP tool list that backs `LeadToolNames` and `tools/list`.
- `agents-plugin-tool/internal/mcp/server.go#L1402-L1422` — `filteredTools` / `toolAllowed`: profile filtering path that capabilities must bypass by using `LeadToolNames`.

## Constraints
- `ai-docs/spec/plugin-runtime.md#L118-L133` — new runtimes report all launcher compatibility surfaces in one JSON response; old/missing capability commands must fall back to full validation or repair before stamping.
- `ai-docs/spec/mcp-tools.md#L13-L21` — MCP protocol is `2025-03-26`; tool-level errors and unknown/profile-rejected tools have distinct response conventions.
- `ai-docs/spec/mcp-tools.md#L172-L191` — `WS_MCP_TOOL_PROFILE` and `WS_MCP_ALLOWED_TOOLS` are optional containment filters and must not reduce the capability payload.
- `ai-docs/mental-model/plugin-runtime.md#L23-L28` — launcher diagnostics must stay on stderr, and compatibility stamps are fail-closed hot-path optimizations only.
- `ai-docs/mental-model/mcp-runtime.md#L33-L44` — tool and CLI additions require registry, dispatch, profile, runtime metadata, and tests to stay in sync.
- `ai-docs/mental-model/prompt-bundle.md#L32-L37` — prompt edits change the bundle hash and `runtime.json` is the launcher-facing authority.
- `ai-docs/ref/ws-mcp.md#L94-L122` — plugin launcher stdout is reserved for MCP JSON-RPC handoff and all fallback paths must preserve the same runtime contract.

## Opinion
- The Go-side capability command is mostly scaffolded; the main codebase gap is avoiding a second command-list source of truth because `runtime.json.commands` is declarative while `cmd/ws-mcp/main.go` has no exported command registry.
- The launcher validator should be small but strict: invalid JSON, wrong protocol, missing prompt hash, or any missing required tool/command should return `False` so the existing fallback path remains the compatibility transition mechanism.
- `commands_compatible` currently verifies command availability through usage text for grouped commands; if the capability list is manual, it must match `runtime.json` exactly or it can pass Go tests while drifting from actual dispatch behavior.
