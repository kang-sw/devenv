# Survey: 17-wsflow-product-mode-convergence-phase4

## Reusable Components
- `agents-plugin-tool/internal/mcp/playbook_tools.go#L499-L522` — `renderPlaybook`: retained wsflow/no-agent bridge already diverts caller `context` into a free-text `## Render Context` block for the five legacy stems and writes the rendered prompt path; Phase 4 can leave this path intact.
- `agents-plugin-tool/internal/mcp/server.go#L2569-L2578` — `wsflowRenderEligibleStems`: single allowlist shared by old `prompt.render` and the retained `playbook.render` bridge; keep this map unless the bridge is intentionally removed later.
- `agents-plugin-tool/internal/mcp/server.go#L2616-L2635` — `appendRenderContext`: sorted context-block builder used by both the retired `prompt.render` path and the retained bridge; reusable for bridge tests after dispatch deletion.
- `agents-plugin-tool/internal/mcp/server.go#L2372-L2388` — `LeadToolNames`: runtime.capabilities derives tool names from `tools()` and product-mode gates, so deleting schema/dispatch/gate entries affects capability output without a separate runtime list.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L183-L194` — `runtimeCapabilities`: CLI fast path reports `mcp.LeadToolNames()` plus command names; wsflow runtime contract tests compare this output exactly.

## Existing Patterns
- Product-mode explicit-call gating: see `agents-plugin-tool/internal/mcp/server.go#L298-L306` — `callTool` rejects no-agent-hidden and full-ws-hidden tools before dispatch; after deleting `prompt.render` from `tools()` and `wsflowOnlyTool`, explicit calls should fall through to the existing unknown-tool path.
- Product-mode tools/list filtering: see `agents-plugin-tool/internal/mcp/server.go#L2391-L2401` and `agents-plugin-tool/internal/mcp/server.go#L2441-L2452` — `filteredTools` delegates visibility to `toolAllowed`, so removed or hidden tools need both schema and predicate review.
- Root-aware schema injection: see `agents-plugin-tool/internal/mcp/server.go#L2320-L2356` — root-aware tools are centralized in `rootAwareToolSchemaRequiresSessionKey`; remove `prompt.render` from this list with its public schema.
- No-agent contract test pattern: see `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L90-L115` — package test shells `go run ./cmd/ws-mcp runtime capabilities`, compares exact tool/command sets against `agents-plugin-wsflow/runtime.json`, and checks hidden tools.
- Full-vs-wsflow surface tests: see `agents-plugin-tool/internal/mcp/server_test.go#L257-L300` and `agents-plugin-tool/internal/mcp/server_test.go#L1229-L1304` — existing tests prove full ws hides `prompt.render` while wsflow advertises and serves it; Phase 4 should invert/replace wsflow expectations.
- Retained bridge regression test: see `agents-plugin-tool/internal/mcp/server_test.go#L1306-L1369` — current wsflow `playbook.render` test verifies context materialization for `code-reviewer` and `plan-populator-survey`, plus no hidden full-ws guidance.
- Declared-var strictness test: see `agents-plugin-tool/internal/mcp/playbook_tools_test.go#L800-L810` — non-legacy wsflow playbook render rejects undeclared context with `ErrUndeclaredVar`, protecting the narrow bridge boundary.

## Relevant Interfaces
- `agents-plugin-tool/internal/mcp/server.go#L725-L732` — `prompt.render` dispatch case: resolves root, reads `stem`, calls `s.renderPrompt`, and returns the prompt path; this is the primary call path to remove.
- `agents-plugin-tool/internal/mcp/server.go#L2135-L2146` — `prompt.render` tool schema: public advertisement block for `tools/list`; deleting this should make the tool absent in both product modes.
- `agents-plugin-tool/internal/mcp/server.go#L2535-L2562` — product-mode predicates: `noAgentHiddenTool` hides agent-backed surfaces in wsflow, while `wsflowOnlyTool` currently identifies only `prompt.render`; Phase 4 removes the latter special case unless another wsflow-only tool remains.
- `agents-plugin-tool/internal/mcp/server.go#L2580-L2614` — `renderPrompt`: old helper that duplicates bridge behavior by loading a playbook body, namespace-substituting, appending context, and writing a prompt file; likely dead after dispatch deletion except for tests or comments.
- `agents-plugin-wsflow/runtime.json#L11-L40` — exact wsflow tool contract includes `prompt.render`; removing it must stay aligned with wsflow `runtime.capabilities`.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L109-L115` and `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L116-L134` — assertions currently require `prompt.render` in wsflow and absent from full ws; update to assert absence in both surfaces.
- `agents-plugin-tool/internal/wsrsrc/loader.go#L135-L143` and `agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L307-L310` — flat playbook fallback comments still name wsflow `prompt.render` as the reason `code-reviewer` loads as a playbook; behavior remains relevant for `playbook.render`, wording is stale.
- `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L55` — rsrc mirror drift-guard comment says namespace substitution happens in the `prompt.render` tool layer; update wording to product-mode render/playbook layer if touched.

## Constraints
- `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md#L190-L197` — selected Phase 4 removes `prompt.render` and stale migration doctrine while requiring wsflow runtime capabilities to omit it and wsflow skills to remain thin playbook shims.
- `ai-docs/mental-model/prompt-bundle.md#L35-L41` — retained bridge is explicitly narrow: no-agent mode plus the five legacy stems only; full ws and arbitrary playbooks still reject undeclared context.
- `ai-docs/mental-model/mcp-runtime.md#L44-L50` — runtime.capabilities, tools/list, and explicit-call product-mode visibility must stay aligned; the current text still describes `prompt.render` as live.
- `ai-docs/mental-model/plugin-runtime.md#L30-L32` — launcher compatibility accepts exact `runtime.capabilities` matches, so `agents-plugin-wsflow/runtime.json` must change with runtime output.
- `ai-docs/ref/wsflow-mirroring.md#L58-L76` — wsflow skills must stay thin `wsflow/playbook.print` shims and delegate prompts must be handed to native subagents through the retained bridge path; this section currently names `prompt.render`.
- `ai-docs/ref/wsflow-mirroring.md#L148-L158` — package tests are the static drift guard for wsflow skills and runtime surface; the doc currently says the only wsflow-only runtime surface is `prompt.render`.
- `ai-docs/spec/mcp-tools.md#L335-L375` and `ai-docs/spec/mcp-tools.md#L377-L413` — spec anchors `260529-prompt-render-tool`, `260529-wsflow-only-tool-surface`, and `260609-playbook-tools` currently teach both the live tool and migration bridge; Phase 4 docs need to retire the former while preserving the latter.
- `ai-docs/spec/plugin-runtime.md#L92-L108` — wsflow package spec says `prompt.render` remains callable until removal and `playbook.render` covers the five stems; this is a direct Phase 4 hotspot.
- `ai-docs/spec/workflow-skills.md#L200-L214` — wsflow implementation spine still instructs delegate prompt dispatch through `prompt.render` or migration `playbook.render`; update to the final `playbook.render` path.

## Risk Signals
- `agents-plugin-tool/internal/mcp/server.go#L2580-L2614` — Possible reuse/dead-code risk: `renderPrompt` carries old behavior that the bridge now owns; leaving it after dispatch deletion may preserve an untested compatibility path and stale comments.
- `agents-plugin-tool/internal/mcp/server_test.go#L1229-L1304` — Possible test risk: an entire wsflow test currently asserts advertisement, successful calls, context injection, and ineligible-stem `isError` for `prompt.render`; replacement should assert unknown-tool JSON-RPC errors while keeping context coverage in `playbook.render` tests.
- `agents-plugin-tool/internal/mcp/server_test.go#L1327-L1329` — Possible contract risk: retained bridge test currently requires both `playbook.render` and `prompt.render` in wsflow tools/list; this will fail after correct removal and should become a `playbook.render`-only assertion.
- `agents-plugin-wsflow/runtime.json#L8-L10` and `agents-plugin-wsflow/runtime.json#L39-L40` — Possible launcher risk: wsflow uses exact runtime capability matching, so leaving `prompt.render` in `runtime.json` after runtime deletion will make launcher/package tests reject the binary.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py#L109-L115` — Possible assertion drift risk: the contract test still asserts `prompt.render` is present in the wsflow contract; this is the package-level failure that should flip for Phase 4.
- `ai-docs/ref/wsflow-mirroring.md#L74-L105` — Possible documentation risk: the mirroring reference treats `prompt.render` as normal dispatch and as a symmetric wsflow-only gate; stale doctrine here would contradict the new public surface even if tests pass.
- `ai-docs/mental-model/prompt-bundle.md#L60-L68` — Possible documentation risk: the mental model says `prompt.render` remains live until later removal; Phase 4 should convert this to retired-history language while preserving the five-stem bridge.
- `/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/prompt-paths/45cbe759-01-plan-populator-survey.md#L1-L3` — Possible dogfood/tooling risk: the rendered role prompt supplied session key `ebay-theft-moonlight-reliant-74`, but the MCP registry rejected it as `unknown_session`; survey used direct files instead of `ws/mental_models.find`.

## Opinion
- Survey evidence is sufficient for implementation; no `[escalate-to-research]` planner decision is needed because the ticket and brief already authorize deletion and the retained bridge has isolated code and tests.
- The highest-risk implementation detail is deleting every `prompt.render` public surface while not deleting `wsflowRenderEligibleStems` or `appendRenderContext`, because those now support `playbook.render`.
- Active docs/specs contain several known-wrong migration assumptions after Phase 4; update docs after code/test evidence so they describe `prompt.render` only as retired history and `playbook.render` as the live materialization path.
