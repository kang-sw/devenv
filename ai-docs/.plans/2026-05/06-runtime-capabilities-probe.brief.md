# Brief: runtime capabilities probe

## Intent

Complete Phase 3 of `260506-bug-ws-mcp-launcher-startup-delay` by adding a
single-process `ws-mcp runtime capabilities` compatibility probe and wiring the
Python launcher to prefer it before the legacy full validation fanout.

## Approach

- Implement the skeleton contracts for `ws-mcp runtime capabilities`.
- Reuse existing runtime metadata and MCP tool registration sources instead of
  maintaining a separate tool list.
- Enumerate the public CLI command surface so the capability response matches
  `agents-plugin/runtime.json.commands`.
- Update the Python launcher to validate version range, MCP protocol, prompt
  bundle, required MCP tools, and required CLI commands from one JSON response.
- Preserve fallback validation when the capability command is missing, invalid,
  or incomplete.

## Constraints

- Read this brief and any plan only; do not read the ticket directly.
- Preserve Phase 1/2 compatibility stamp semantics: stamps are written only
  after successful validation and stale/missing data fails closed.
- Launcher stdout must stay reserved for MCP JSON-RPC handoff; launcher
  diagnostics go to stderr only.
- The capability payload must report the full lead MCP tool surface independent
  of `WS_MCP_TOOL_PROFILE` and `WS_MCP_ALLOWED_TOOLS`.
- Do not bump release versions or publish release assets in this implementation.

## Out of scope

- Raising `.mcp.json` startup timeout.
- Shipping, tagging, or changing release workflow defaults.
- Reworking the POSIX shell launcher unless a test or contract requires parity.

## Details

Skeleton acceptance criteria already exist:

- `agents-plugin-tool/cmd/ws-mcp/main_test.go`
  `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` must pass.
- `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py` must pass and
  should be extended if needed to cover successful single-probe validation.

The capability command must output one JSON object with these top-level fields:

- `version`
- `source_commit`
- `mcp_protocol`
- `prompt_bundle`
- `tools`
- `commands`

The launcher should first attempt `runtime capabilities`. If it succeeds and
the payload satisfies `runtime.json`, `runtime_fully_compatible()` returns true
without invoking the legacy `version`, `tools/list`, per-command subprocess
fanout, or `runtime info` checks. If the command is absent, exits non-zero,
prints invalid JSON, or omits required surfaces, the launcher falls back to the
existing full validation path.

## References

- `ai-docs/spec/plugin-runtime.md` — [Must] runtime contract metadata,
  launcher repair, hot-path cache, CLI entrypoints, and
  `260506-runtime-capabilities-single-probe`.
- `ai-docs/spec/mcp-tools.md` — [Must] MCP protocol surface and tool profile
  behavior.
- `ai-docs/mental-model/plugin-runtime.md` — [Must] launcher, runtime contract,
  compatibility stamp, and release coupling.
- `ai-docs/mental-model/mcp-runtime.md` — [Must] command and tool registry
  change recipe.
- `ai-docs/mental-model/prompt-bundle.md` — [Must] prompt bundle hash/list
  compatibility behavior.
- `ai-docs/ref/ws-mcp.md` — [Must] canonical runtime reference to update during
  doc pipeline.
