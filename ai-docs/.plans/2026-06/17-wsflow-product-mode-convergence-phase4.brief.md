# Brief: 260616-refactor-wsflow-product-mode-convergence Phase 4

## Intent

Finish wsflow product-mode convergence by deleting the wsflow-only
`prompt.render` MCP surface after its legacy context-materialization behavior
has been absorbed into wsflow-mode `playbook.render`.

## Scope Boundary

Selected scope: `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md`
Phase 4, "remove prompt.render and stale migration doctrine".

Included:

- Remove `prompt.render` from MCP schema advertisement, dispatch, hidden-tool
  gates, runtime capability metadata, and wsflow package runtime contract.
- Keep the narrow wsflow/no-agent `playbook.render` bridge for the five legacy
  prompt stems: `reference-discovery`, `plan-populator-survey`,
  `plan-populator-research`, `code-reviewer`, and `mental-model-updater`.
- Update tests so full ws and wsflow both verify that `prompt.render` is no
  longer advertised or callable, while wsflow `playbook.render` still preserves
  the legacy context behavior.
- Update specs, mental models, and wsflow mirroring docs so `prompt.render` is
  described only as retired history, not a normal dispatch path.

Excluded:

- Do not remove the legacy five-stem context bridge from wsflow
  `playbook.render`.
- Do not remove or rename the five legacy rsrc stems.
- Do not change wsflow thin skill shims or the wsflow rsrc mirror except where
  verification proves runtime contract metadata needs adjustment.
- Do not redesign future `api.*` documentation/memory tooling.

## Caller-Visible Contract

After the change, wsflow users and launchers must see `playbook.render`, not
`prompt.render`, as the delegate prompt materialization surface. The
`prompt.render` tool name must be absent from `tools/list`, rejected by
`tools/call`, absent from `runtime.capabilities`, and absent from
`agents-plugin-wsflow/runtime.json`. Full ws must remain unchanged except that
there is no longer a wsflow-only hidden-tool exception for `prompt.render`.

## Contract Instructions

- `agents-plugin-tool/internal/mcp/server.go` owns tool schema, dispatch,
  product-mode visibility, and runtime capability name lists. Delete the
  `prompt.render` surface rather than leaving it hidden in wsflow.
- `agents-plugin-tool/internal/mcp/playbook_tools.go` owns the retained wsflow
  `playbook.render` legacy context bridge. Keep that behavior intact.
- `agents-plugin-wsflow/runtime.json` must no longer require `prompt.render`.
- `agents-plugin-wsflow/tests/` must assert the wsflow no-agent contract without
  `prompt.render`.
- Runtime tests under `agents-plugin-tool/internal/mcp/` must cover both the
  removed tool and the retained bridge.
- Do not add a compatibility alias or fallback tool for `prompt.render`.

## Integration Test Instructions

Run at minimum:

- `cd agents-plugin-tool && go test -count=1 ./internal/mcp`
- `cd agents-plugin-tool && go test -count=1 ./internal/wsrsrc`
- `python3 -m unittest discover agents-plugin-wsflow/tests`

Run broader package tests if local changes touch shared runtime metadata or
launcher contracts beyond the listed files.

## Implementation Strategy Decisions

- Treat `prompt.render` deletion as a public MCP surface removal scoped to
  wsflow product mode, already authorized by the ticket's `spec-remove` entries.
- Preserve the Phase 2 bridge in `playbook.render`; users migrate by changing
  tool name, not by losing context injection.
- Prefer deleting prompt-render-specific hidden-tool code over making
  `prompt.render` hidden in both products; explicit calls should become unknown
  tool errors.
- Update documentation after implementation evidence, not before, so stale
  doctrine cleanup reflects the actual final surface.

## Rejected Alternatives

- Keeping `prompt.render` as a deprecated hidden alias: rejected because Phase 4
  explicitly removes the MCP/runtime surface and wsflow runtime capabilities
  must omit it.
- Removing legacy context append from `playbook.render`: rejected because Phase
  2 made that the compatibility bridge for existing prompt.render use cases.
- Leaving docs to describe both paths during migration: rejected because Phase 4
  is the migration endpoint for this surface.

## Approach

- Remove server registry/schema/dispatch/capability references for
  `prompt.render`.
- Adjust or replace tests that currently expect wsflow to advertise and serve
  `prompt.render`.
- Verify wsflow `playbook.render` legacy context tests still pass.
- Update package runtime metadata and wsflow runtime contract tests.
- Update specs, mental models, reference docs, ticket Result, and `_index.md`.

## Constraints

- `playbook.render` declared-variable semantics must remain strict outside the
  narrow wsflow/no-agent legacy stem set.
- Product-mode gates for mercenary and exec surfaces must not regress.
- Full ws must not gain wsflow-only behavior while removing the old hidden-tool
  exception.
- `prompt.render` references in historical tickets may remain when clearly
  archival; active docs and mental models must not teach it as live workflow.

## Out of scope

- New playbooks, new native subagent flows, and fresh-reader audit playbook work.
- `api.*` future namespace redesign.
- wsflow marketplace/install behavior unless runtime contract checks require a
  metadata-only adjustment.

## Details

Expected code hotspots:

- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/server_test.go`
- `agents-plugin-tool/internal/mcp/playbook_tools.go`
- `agents-plugin-wsflow/runtime.json`
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py`

Expected documentation hotspots:

- `ai-docs/spec/mcp-tools.md`
- `ai-docs/spec/plugin-runtime.md`
- `ai-docs/spec/workflow-skills.md`
- `ai-docs/mental-model/prompt-bundle.md`
- `ai-docs/mental-model/mcp-runtime.md`
- `ai-docs/mental-model/plugin-runtime.md`
- `ai-docs/mental-model/workflow-skills.md`
- `ai-docs/ref/wsflow-mirroring.md`
- `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md`
- `ai-docs/_index.md`

## Verification Contract

Acceptance requires:

- `prompt.render` absent from wsflow `runtime.capabilities` and package
  `runtime.json`.
- `tools/list` does not advertise `prompt.render` in full ws or wsflow mode.
- Explicit `tools/call` for `prompt.render` returns an unknown-tool JSON-RPC
  error in both modes.
- wsflow `playbook.render` still accepts legacy stems with free-form context and
  still rejects undeclared context for non-legacy stems.
- wsflow skill-shim tests still pass.
- Docs no longer describe `wsflow/prompt.render` as the retained normal dispatch
  path.

## References

- [Must] `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md` - selected Phase 4 scope and prior phase results.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - binding pivot direction: prompt.render was a migration bridge toward playbook rendering.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow package and runtime contract rules.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc and playbook.render contracts.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP registry, product-mode gates, and runtime.capabilities rules.
- [Must] `ai-docs/mental-model/plugin-runtime.md` - runtime.json and wsflow package contract coupling.
- [Must] `ai-docs/mental-model/workflow-skills.md` - wsflow skill and delegate dispatch contracts.
