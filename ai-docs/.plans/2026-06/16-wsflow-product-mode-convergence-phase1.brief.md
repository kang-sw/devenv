# Brief: 260616-refactor-wsflow-product-mode-convergence Phase 1

## Intent

Make `playbook.print` and `playbook.render` safe to use from wsflow product mode before the later phases remove `prompt.render` and collapse wsflow skills.

## Scope Boundary

Implement Phase 1 only: product-mode-aware playbook output for existing playbook tools. Do not remove `prompt.render`, do not replace wsflow skill bodies, and do not change the wsflow skill inventory in this phase.

## Caller-Visible Contract

In `WS_MCP_NO_AGENT=1` with `WS_MCP_NAMESPACE=wsflow`, playbook tool output uses wsflow-facing notation and omits guidance for hidden full-ws surfaces such as `ws.mercenary.*`, `exec.*`, and mercenary preference. Full ws behavior remains unchanged.

## Contract Instructions

- Update `agents-plugin-tool/internal/mcp/playbook_tools.go` and `server.go` rather than editing canonical rsrc playbook bodies for wsflow-only behavior.
- Reuse the existing `RuntimeNamespace`, `NoAgentMode`, `wsNamespaceRef`, and product-mode gate predicates.
- Keep `playbook.print` and `playbook.render` visible in no-agent mode.
- Keep `prompt.render` unchanged in Phase 1; it is removed by a later phase.
- Do not introduce a divergent `agents-plugin-wsflow/rsrc` body; the rsrc mirror remains byte-identical.

## Integration Test Instructions

- Extend Go tests under `agents-plugin-tool/internal/mcp` for wsflow-mode playbook output.
- Run `cd agents-plugin-tool && go test -count=1 ./internal/mcp ./internal/wsrsrc`.
- Run `python3 -m unittest discover agents-plugin-wsflow/tests`.

## Implementation Strategy Decisions

- Apply namespace substitution and no-agent guidance filtering at render time, after normal harness rendering and context substitution.
- Suppress the appended mercenary tip in wsflow no-agent mode rather than relying only on line filtering.
- Update public playbook tool descriptions so wsflow tools/list no longer says the tools are "Full ws" only.

## Rejected Alternatives

- Do not hand-edit generated wsflow rsrc copies; generated sameness is the rsrc contract.
- Do not remove `prompt.render` in this phase.
- Do not hide `playbook.print` or `playbook.render` in wsflow mode; Phase 1 makes them safe instead.

## Approach

- Add a small product-mode output filter in the playbook render path.
- Make delegation tips product-mode-aware.
- Add wsflow-mode tests for representative lead and delegate playbooks.
- Keep existing full ws golden tests passing.

## Constraints

- `agents-plugin-wsflow/rsrc` must remain byte-identical to `agents-plugin/rsrc`.
- Product-mode filtering must not affect full ws output.
- Caller-injected `prompt.render` context remains out of scope for playbook filtering.

## Out of scope

- `prompt.render` retirement.
- wsflow thin-skill shim conversion.
- spec and mental-model closeout beyond Phase 1 Result capture.

## Verification Contract

- Go unit tests prove no-agent playbook output has wsflow notation and no hidden full-ws tool names.
- wsflow package tests prove runtime contract and distributed skill bundle remain valid.

## References

- [Must] `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md` - selected phase and deferred phases.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - migration anchor and wsflow convergence direction.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc loading and wsflow prompt rendering contracts.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - product-mode gate and runtime surface contracts.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow skill and wsflow surface contracts.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - generated rsrc mirror and wsflow package verification rules.
