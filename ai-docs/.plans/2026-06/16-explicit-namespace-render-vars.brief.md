# Brief: 260616-refactor-explicit-namespace-render-vars

## Intent

Replace implicit `ws` -> `wsflow` playbook namespace string substitution with explicit reserved render variables so shared rsrc text marks display namespace output deliberately.

## Scope Boundary

Implement Phase 1 of `260616-refactor-explicit-namespace-render-vars`. Keep Phase 1 product-mode full-only/wsflow-only marker selection intact. Do not absorb or remove `prompt.render`; that remains a later convergence phase.

## Caller-Visible Contract

`playbook.print` and `playbook.render` render display namespace notation through reserved variables such as `McpNamespace` and `SkillNamespace`. Full ws output uses `ws/` and `ws:`. wsflow output uses `wsflow/` and `wsflow:`. Actual MCP tool identifiers such as `ws.lead.login` remain literal unless intentionally modeled by a separate semantic variable.

## Contract Instructions

Update the MCP playbook rendering layer, not `wsrsrc.Load`, so generic rsrc loading keeps its declared-variable contract. Add a small allowlist of reserved namespace variables that playbook rendering may inject even when a playbook frontmatter does not declare them. Runtime-injected reserved variables must win over caller `context`.

Update shared rsrc playbooks that use display `ws/` or `ws:` notation to use explicit namespace variables. Preserve actual `ws.*` tool identifiers and product-mode marker comments in source. Regenerate `agents-plugin/rsrc/manifest.json` and the byte-identical `agents-plugin-wsflow/rsrc/` mirror.

## Integration Test Instructions

Extend `agents-plugin-tool/internal/mcp/playbook_tools_test.go` around existing product-mode playbook tests. Tests must prove wsflow output gets namespace notation from explicit variables, full ws output stays `ws`, caller context cannot override reserved namespace variables, literal `ws.*` tool names are not rewritten, and shipped rsrc output no longer depends on broad namespace substitution.

## Implementation Strategy Decisions

- Add reserved namespace vars in the playbook tool layer before calling `wsrsrc.Load`.
- Do not require every rsrc frontmatter block to declare common namespace vars.
- Remove broad namespace substitution from `playbook.print` / `playbook.render` after rsrc text is migrated.
- Keep `prompt.render`'s legacy substitution path until the later prompt-render absorption/removal phases.

## Rejected Alternatives

- Keep token-safe substitution as the main contract: rejected because new prose patterns can still produce surprising failures.
- Add `{WS_NAMESPACE}` as raw ad hoc text outside the existing variable system: rejected because rsrc already has a variable mechanism.
- Move the exception into `wsrsrc.Load`: rejected because `wsrsrc` is a generic loader and should not learn product-mode semantics.

## Approach

- Teach `buildPlaybookVars` about reserved namespace variables and make them available alongside declared vars.
- Convert rsrc display namespace notation to `{{.McpNamespace}}/...` and `{{.SkillNamespace}}:...`.
- Keep literal `ws.*` identifiers as source literals.
- Replace broad playbook namespace substitution tests with explicit-var assertions.
- Regenerate manifests and mirrors.

## Constraints

- Preserve wsflow byte-identical rsrc mirror policy.
- Preserve product-mode full-only/wsflow-only section selection.
- Preserve `prompt.render` until later ticket phases.
- Do not stage unrelated `ai-docs/presentation/260616-wsflow-seminar-v3.js`.

## Verification Contract

- `cd agents-plugin-tool && go test -count=1 ./internal/mcp ./internal/wsrsrc`
- `cd agents-plugin-tool && go test -count=1 ./...`
- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check`

## References

- [Must] `ai-docs/tickets/ready/260616-refactor-explicit-namespace-render-vars.md` - selected implementation contract.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - pivot direction and wsflow convergence premise.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc loading, playbook rendering, and product-mode context.
- [Must] `ai-docs/mental-model/workflow-skills.md` - rsrc workflow skill authoring contracts.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - wsflow product-mode and namespace override runtime contracts.
