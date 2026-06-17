# Brief: wsflow product-mode convergence Phase 2

## Intent

Absorb the live wsflow `prompt.render` materialization behavior into
product-mode-aware `playbook.render` so current wsflow delegate-prompt use cases
can move to the shared playbook surface before the old tool is removed.

## Scope Boundary

Implement only Phase 2 of
`260616-refactor-wsflow-product-mode-convergence`: make `playbook.render` cover
the existing `prompt.render` allowlist, context injection, and wsflow namespace
behavior in no-agent/wsflow mode. Leave wsflow skill shim collapse and
`prompt.render` removal to later phases.

## Caller-Visible Contract

In wsflow product mode, callers can use `playbook.render` for the five legacy
`prompt.render` stems: `reference-discovery`, `plan-populator-survey`,
`plan-populator-research`, `code-reviewer`, and `mental-model-updater`.
The rendered prompt file must preserve the old free-text Render Context
append behavior and wsflow-facing namespace output. Full ws behavior remains
unchanged except for any shared helper refactor with identical output.

## Contract Instructions

- Reuse the existing rsrc/playbook loading path; do not add a second prompt
  source, mock prompt source, or fallback embedded prompt source.
- Preserve the five-stem render eligibility in wsflow mode for this phase.
- Do not make `implementer` render-eligible for wsflow as part of this phase.
- Treat actual MCP tool identifiers as stable implementation names; only
  user-facing namespace notation should render through product-mode behavior.
- Preserve `prompt.render` as a callable wsflow-only tool until Phase 4.
- Keep wsflow package rsrc bodies generated-identical to canonical rsrc bodies.

## Integration Test Instructions

Add or update MCP/runtime tests covering wsflow-mode `playbook.render` for at
least one free-response legacy stem and one file-writing legacy stem. Verify
rendered files contain the caller context block, use wsflow namespace notation
where applicable, and do not expose hidden full-ws mercenary/exec guidance.
Keep existing `prompt.render` tests passing.

## Implementation Strategy Decisions

- Phase 2 is a compatibility bridge: move behavior into `playbook.render`
  without deleting the old `prompt.render` surface.
- Product-mode selection and namespace behavior belong in the MCP playbook
  layer, not in stored wsflow-only rsrc file divergence.
- Caller context for legacy prompt-style use cases should be passed as task
  input/context materialization behavior, not as undeclared template variables.

## Rejected Alternatives

- Deleting `prompt.render` now is rejected; that is Phase 4.
- Reintroducing broad playbook namespace string rewriting is rejected; Phase 1.5
  replaced it with explicit namespace variables and product markers.
- Adding wsflow-specific divergent rsrc prompt bodies is rejected; the generated
  rsrc mirror must stay byte-identical.

## Approach

- Locate the current `prompt.render` implementation and tests.
- Locate current `playbook.render` context handling and product-mode rendering.
- Add the minimum compatibility path that lets wsflow callers render the five
  legacy stems through `playbook.render` with free-text context appended.
- Extend tests for both the retained old tool and the new playbook path.
- Regenerate rsrc manifests or wsflow mirror only if rsrc files change.

## Constraints

- `WS_MCP_NO_AGENT=1` remains the wsflow product-mode selector.
- Product-mode tool visibility stays symmetric until Phase 4.
- `runtime.capabilities`, tools/list, and explicit-call gates must remain
  consistent for any changed surface.
- No source edit may weaken session-key requirements for root-aware tools.

## Out of scope

- Collapsing `agents-plugin-wsflow/skills/lead-*` curated bodies to shims.
- Removing `prompt.render` from MCP schemas, dispatch, runtime metadata, or
  package tests.
- Future pure-tooling `api.*` namespace work.
- Adding new renderable delegate stems beyond the existing five-stem legacy set.

## Details

The old `prompt.render(stem, context)` appends caller-supplied context as a
free-text Render Context block after rendering a prompt stem. The new
`playbook.render` support should preserve that observable materialization
shape for the legacy stem set in wsflow mode without turning arbitrary context
keys into rsrc template variables.

## Verification Contract

- Run the focused Go tests that cover MCP playbook and prompt rendering.
- Run `go test -count=1 ./...` in `agents-plugin-tool` unless an unrelated known
  flake repeats; if the exec abort timing flake appears, record exact output and
  run the focused affected suites.
- Run `python3 -m unittest discover agents-plugin-wsflow/tests`.
- Run `git diff --check`.

## References

- [Must] `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md` - selected Phase 2 scope and later-phase exclusions.
- [Must] `ai-docs/tickets/todo/260605-epic-ws-playbook-factory-pivot.md` - parent endpoint: wsflow differs by namespace and capability gates, not separate workflow text.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - original pivot direction for playbook rendering and wsflow convergence.
- [Must] `ai-docs/spec/mcp-tools.md` - `prompt.render`, `playbook.render`, product-mode, and rsrc contracts.
- [Must] `ai-docs/spec/plugin-runtime.md` - wsflow package/runtime contract coupling.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - single rsrc source of truth and current prompt/render boundary.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - product-mode gates and namespace behavior.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow package and rsrc mirror rules.
- [Maybe] `ai-docs/mental-model/workflow-skills.md` - wsflow skill consumers that still mention `prompt.render`.
- [Maybe] `ai-docs/mental-model/api-documentation-cache.md` - guardrail against restoring agent-backed API prompt stems.
