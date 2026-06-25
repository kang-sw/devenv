# Survey: 25-260625-refactor-workflow-delegation-config.brief

## Selected Slice

Phase 1: Config namespace and API refactor.

This survey maps likely files and tests only. It must not be treated as approval
to implement Phase 2 manual insertion, XML wrapping, or lead-tune rewrites.

## Reusable Components

- `agents-plugin-tool/internal/wsconfig/scope.go` - registered config item keys
  and default write scopes. Add workflow-prefixed constants here and set both
  new keys to `ScopeGlobal`.
- `agents-plugin-tool/internal/wsconfig/resolver.go` - layered
  session/project/global/builtin resolver. Use it for global writes and
  keyless reads; do not add a parallel config path.
- `agents-plugin-tool/internal/wsconfig/scoped_show.go` - `config.show`
  registered-key enumeration. Adding workflow keys to `scopeRegistry` should
  naturally make them visible even when unset.
- `agents-plugin-tool/internal/mcp/server.go` - MCP schema registry, dispatch,
  `config.tuning` construction, product-mode gates, `LeadToolNames`, and
  mercenary visibility. This is the main Phase 1 source file.
- `agents-plugin-tool/internal/mcp/session_config_adapter.go` - session adapter
  for resolver reads/writes. New global-only writers should not need session
  writes, but existing resolver construction can be reused.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` - render-time mercenary
  guidance helpers. Phase 1 may need comment/text updates or a helper call-site
  change if render guidance now reads global state before calling render helpers.
- `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` - runtime
  tool manifests. Remove `ws.lead.prefer_mercenary`; add the new config writer
  tools in the product modes where live capabilities expose them.

## Existing Behavior To Replace

- `wsconfig.ItemPreferMercenary = "prefer_mercenary"` currently defaults to
  `ScopeSession`.
- `server.go` currently dispatches `ws.lead.prefer_mercenary`, accepts legacy
  `enabled`, and writes `ItemPreferMercenary` with a session key.
- `playbook.render` currently resolves mercenary guidance from
  `resolver.GetBool(capturedKey, wsconfig.ItemPreferMercenary)` inside the lead
  session-key branch.
- `mercenaryHiddenFromConfig` currently reads `ItemPreferMercenary` with empty
  session key and treats empty/`hide` as hidden.
- `config.tuning` currently exposes `delegation.prefer_mercenary` with writer
  `ws.lead.prefer_mercenary`.
- Tests in `prefer_mercenary_phase2_test.go` and `mercenary_surface_test.go`
  assert the old session-scoped tool behavior and must be rewritten.

## Likely Source Edits For Phase 1

- `agents-plugin-tool/internal/wsconfig/scope.go`
  - Add `ItemWorkflowPreferSubagent = "workflow.prefer_subagent"`.
  - Add `ItemWorkflowPreferMercenary = "workflow.prefer_mercenary"`.
  - Register both as `ScopeGlobal`.
  - Keep or remove `ItemPreferMercenary` depending on compile fallout. If kept,
    mark legacy/orphan and do not use it in new behavior.

- `agents-plugin-tool/internal/mcp/server.go`
  - Add schemas for `config.workflow_prefer_subagent` and
    `config.workflow_prefer_mercenary`.
  - Add dispatch cases that validate canonical enums and write global config.
  - Remove `ws.lead.prefer_mercenary` schema and dispatch.
  - Update `buildTuningCatalog` to register workflow knobs using the new writer
    tools.
  - Update `currentPreferMercenary` to read `"workflow.prefer_mercenary"` and
    report canonical `on|off|hide`.
  - Add current-value helper for `"workflow.prefer_subagent"`.
  - Update `noAgentHiddenTool` and any `ws.lead.prefer_mercenary` visibility
    comments.
  - Update `mercenaryHiddenFromConfig` to use the new workflow key and builtin
    default `hide`.
  - Update `LeadToolNames`/runtime-capability expectations through `tools()`.

- `agents-plugin-tool/internal/mcp/playbook_tools.go`
  - If user-facing guidance still names `ws.lead.prefer_mercenary`, update it to
    `config.workflow_prefer_mercenary`.
  - Do not implement XML wrapping or subagent manual append here in Phase 1.

- `agents-plugin/runtime.json`
  - Replace the removed old tool with new writer tools.

- `agents-plugin-wsflow/runtime.json`
  - Reflect no-agent live capability expectations. Do not expose mercenary-only
    writer if no-agent gates hide it.

## Likely Test Edits

- `agents-plugin-tool/internal/wsconfig/scope_test.go`
  - Add default-scope tests for the two workflow keys.
  - Add builtin/global resolution tests if implementation adds a shared builtin
    defaults map for workflow keys.

- `agents-plugin-tool/internal/mcp/server_test.go`
  - Tools/list and runtime capability expectations for new/removed tools.
  - Mercenary visibility under builtin `hide`, explicit `on`, and explicit
    `off` if the existing explicit-availability contract keeps `off` visible.
  - No-agent hidden/visible expectations.

- `agents-plugin-tool/internal/mcp/mercenary_surface_test.go`
  - Replace `ws.lead.prefer_mercenary` handler tests with
    `config.workflow_prefer_mercenary`.
  - Replace session-scope resolver tests with global-only behavior.
  - Keep tests that prove mercenary availability semantics remain unchanged.

- `agents-plugin-tool/internal/mcp/prefer_mercenary_phase2_test.go`
  - Either rewrite as global workflow preference tests or remove if fully
    superseded.

- `agents-plugin-tool/internal/mcp/prompt_override_test.go`
  - Update `config.tuning` expectations from `delegation.prefer_mercenary` /
    `ws.lead.prefer_mercenary` to workflow knob ids/writers.
  - Add or adjust current-value assertions for builtin `off`/`hide`.

- `agents-plugin-tool/internal/mcp/session_auth_test.go`
  - Update delegate/leaf denial coverage to target `config.workflow_*` through
    the existing `config.*` gate.
  - Legacy session record tests can remain only if they assert old typed fields
    are ignored/orphaned.

- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py`
  - Update expected runtime manifest/tool contract if no-agent capabilities
    change.

## Commands

Focused command set:

```bash
cd agents-plugin-tool
go test ./internal/wsconfig -count=1
go test ./internal/mcp -count=1 -run 'Test.*WorkflowPrefer|Test.*PreferMercenary|TestConfigTuning|TestNoAgent|TestRuntimeCapabilities|TestLeadToolNames'
go test ./internal/mcp ./internal/wsconfig ./cmd/ws-mcp -count=1
python3 -m unittest discover ../agents-plugin-wsflow/tests
git diff --check
```

If the focused regexp misses renamed tests, run the full package tests:

```bash
cd agents-plugin-tool
go test ./internal/mcp ./internal/wsconfig ./cmd/ws-mcp -count=1
python3 -m unittest discover ../agents-plugin-wsflow/tests
git diff --check
```

## Risks

- Stale planned spec text still says the workflow keys are session-default
  items. The ready ticket supersedes that text.
- Removing `ws.lead.prefer_mercenary` will break many old tests; do not restore
  the old tool as an alias to make tests pass.
- `hide` is both a render/content and keyless visibility state. Keep the new
  mercenary key global-only so keyless visibility and later render guidance do
  not diverge.
- `off` versus `hide` semantics must remain distinct: `off` should mean native
  default guidance, while `hide` suppresses the mercenary surface/content.
- No-agent/wsflow must be checked explicitly because `config.tuning` is visible
  there but full-ws-only knobs are omitted.
- Runtime manifests compare against live capabilities; forgetting them will fail
  package contract tests even when Go unit tests pass.

## Prep Verification Notes

- `ws/specs.find(ticket_stem: ...)` returned no direct results even though the
  ticket frontmatter lists spec anchors. Relevant specs were read via explicit
  `specs.status` anchors and file excerpts.
- `ws/mental_models.status` requires paths under `ai-docs/mental-model/...`;
  status metadata was retrieved for `mcp-runtime`, `prompt-bundle`, and
  `workflow-skills` after correcting the path shape.
- `ws/infra.read(name: "impl-playbook")` was read; its implementation invariants
  are included in the brief references.
