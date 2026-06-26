# Survey: 17-wsflow-product-mode-convergence-phase2

## Reusable Components

- `agents-plugin-tool/internal/mcp/server.go` - `wsflowRenderEligibleStems` holds the exact five legacy `prompt.render` stems; `implementer` is not eligible.
- `agents-plugin-tool/internal/mcp/server.go` - `Server.renderPrompt` is the current legacy behavior: rsrc-backed playbook rendering, wsflow namespace substitution, and sorted free-text `## Render Context`.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` - `renderPlaybookBody` is the shared core for rsrc load, MCP-layer var substitution, delegation tip, child-key splice, and product-mode selection.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` - `buildPlaybookVars` owns declared-variable validation plus reserved terminology/model/namespace variable layering.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` - product-marker selection strips full-only/wsflow-only markers according to `WS_MCP_NO_AGENT`.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` - `renderPlaybook` writes rendered playbook text to a worktree-scoped `prompt` path and returns the path plus tier.
- `agents-plugin-tool/internal/wsrsrc/loader.go` - `wsrsrc.Load` is the single rsrc loader for manifest validation, harness overlays, includes, frontmatter, and optional substitution.
- `agents-plugin-tool/internal/wsrsrc/loader.go` - flat-playbook fallback lets `agents-plugin/rsrc/code-reviewer.md` load as the legacy `code-reviewer` stem.

## Existing Patterns

- `agents-plugin-tool/internal/mcp/server_test.go` - legacy `prompt.render` wsflow coverage verifies advertisement, context append, namespace output, and `implementer` rejection.
- `agents-plugin-tool/internal/mcp/server_test.go` - legacy stem coverage renders all five eligible stems from shipped rsrc and checks non-empty, fully substituted output.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go` - playbook wsflow output coverage checks hidden full-ws guidance, marker stripping, namespace vars, and literal `ws.lead.login` preservation.
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go` - playbook render wsflow delegate coverage renders a shipped delegate in wsflow mode and rejects mercenary/exec guidance.
- `agents-plugin-tool/internal/mcp/server_test.go` - product-mode tool visibility coverage checks no-agent `tools/list` hides mercenary/config surfaces while keeping `playbook.print` and `playbook.render`.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py` - runtime-contract coverage compares wsflow `runtime.json` tools to live no-agent capabilities and currently requires `prompt.render`.

## Relevant Interfaces

- `agents-plugin-tool/internal/mcp/server.go` - MCP dispatch cases for `prompt.render`, `playbook.print`, and `playbook.render`; `playbook.render` resolves root, rsrc root, optional child-key mint, then calls `renderPlaybook`.
- `agents-plugin-tool/internal/mcp/server.go` - public schemas: `prompt.render(stem, context)` and `playbook.render(session_key, name, context, root_override)`.
- `agents-plugin-tool/internal/mcp/server.go` - root-aware schema injection includes both `prompt.render` and `playbook.render`; the current special case does not mark `session_key` required for `playbook.render`.
- `agents-plugin-tool/internal/mcp/server.go` - `WS_MCP_NO_AGENT`, namespace selection, and explicit-call gates for no-agent hidden tools and wsflow-only tools.
- `agents-plugin-tool/internal/mcp/server.go` - product-mode gates hide exec and `ws.mercenary.*` in no-agent mode; `prompt.render` is wsflow-only.
- `agents-plugin-wsflow/runtime.json` - wsflow exact runtime contract includes both `playbook.render` and retained `prompt.render`.
- `agents-plugin/rsrc/reference-discovery/reference-discovery.md`, `plan-populator-survey`, `plan-populator-research`, and `mental-model-updater` - four legacy stems are subdir render playbooks with delegate role and model vars.
- `agents-plugin/rsrc/code-reviewer.md` - `code-reviewer` is a flat var-free prompt body with `{{.McpNamespace}}` usage.

## Constraints

- `ai-docs/spec/mcp-tools.md` - `prompt.render` contract: rsrc source, namespace substitution, free-text context append, temp file output, five-stem allowlist, no `implementer`.
- `ai-docs/spec/mcp-tools.md` - `playbook.render` contract: materializes a named playbook to a tmp file, returns `recommended-tier`, handles root override and child-key minting, and uses explicit namespace vars.
- `ai-docs/mental-model/prompt-bundle.md` - rsrc is the single prompt source; playbook render layers caller context, terminology, model aliases, and namespace vars with namespace vars winning.
- `ai-docs/mental-model/prompt-bundle.md` - `prompt.render` still owns legacy regex substitution and allowlist until absorption/removal; playbook render uses markers plus explicit namespace vars.
- `ai-docs/mental-model/mcp-runtime.md` - no-agent product gates must affect tools/list, tools/call, and CLI capabilities; root-aware public schemas should use `session_key`, with `ws.lead.login` as the only advertised root-accepting schema.
- `ai-docs/ref/wsflow-mirroring.md` - wsflow rsrc must remain a generated byte-identical copy of canonical rsrc; render-time product behavior must not be stored as divergent files.

## Risk Signals

- Current `playbook.render` treats `context` as declared template variables and errors on undeclared keys, while legacy `prompt.render` appends arbitrary context as free text. Bridging these without changing all playbook context semantics needs care.
- Legacy `prompt.render` substitutes `ws/` and `ws:` before appending context, so context values are intentionally not namespace-substituted. New playbook behavior should preserve that observable materialization shape for legacy stems.
- `playbook.render` is root-aware but schema injection currently does not require `session_key`; the brief says no source edit may weaken session-key requirements.
- Existing `prompt.render` wsflow tests still pass `root` in arguments, while current docs say public root-aware tools should use `session_key`.
- Curated wsflow skill text still dispatches through `wsflow/prompt.render`, while generated rsrc text already names `playbook.render`; Phase 2 should keep old skill behavior working and leave shim collapse to Phase 3.

## Opinion

- No research escalation is needed: Phase 2 direction is settled as compatibility absorption, with `prompt.render` retained and skill shim collapse/removal deferred.
- The implementation points are localized: legacy materialization exists in `server.go`, while reusable playbook rendering behavior is already factored in `playbook_tools.go`.
- If rsrc files do change, mirror regeneration is already guarded; if only MCP rendering code changes, the mirror should stay byte-identical without regeneration.
