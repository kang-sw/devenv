# Brief: 260615-refactor-native-explore-dispatch-skill-guidance

## Intent

Move shipped scoped-exploration guidance away from rendering the generic `explore`
playbook as the normal path. Lead workflow text should dispatch host-native
Explore/search subagents directly with a scoped question or purpose-specific
query block.

## Scope Boundary

Implement Phase 1 only. Update shared rsrc guidance, generated wsflow rsrc copy,
`workflow-skills` spec text, and related mental models. Keep the `explore`
playbook and its tests as fallback/compatibility surface. Do not add
`explore.claude.md`.

## Caller-Visible Contract

Shipped workflow skill text no longer tells callers to call
`ws/playbook.render(name: "explore")` or `ws/playbook.print(name: "explore")`
for ordinary scoped exploration. The normal path is: spawn a host-native
Explore/search subagent, provide a scoped English task prompt or existing
purpose-specific query block, require read-only evidence with citations, and
collect gaps/follow-up needs.

## Contract Instructions

Update canonical files under `agents-plugin/rsrc/` first. Regenerate
`agents-plugin/rsrc/manifest.json` and the byte-identical
`agents-plugin-wsflow/rsrc/` mirror after rsrc edits. Keep `explore.md` present.
Do not reframe implementer/reviewer delegate rendering; bundled delegates still
use `playbook.render`.

## Integration Test Instructions

Run the rsrc manifest/mirror regeneration tests, relevant Go tests for
`internal/wsrsrc` and `internal/mcp`, and wsflow package tests. Also run escaped
searches proving remaining `explore` render references are ticket/test/fallback
only, not shipped normal-path guidance.

## Implementation Strategy Decisions

- Update `lead-workflow-manual` first because other playbooks refer to it.
- Rewrite generic call sites to say "native Explore/search subagent" directly.
- Rewrite purpose-specific call sites to pass their own query/check block as the
  native subagent task prompt.
- Reconcile docs after source guidance so spec text describes implemented
  wording, not planned wording.

## Rejected Alternatives

- Do not solve this by adding `explore.codex.md`; that only changes the worker
  prompt after the old render step.
- Do not add an empty `explore.claude.md`; the loader would select it and
  replace the base playbook.
- Do not delete `explore.md` or tests in this phase.

## Approach

- Replace canonical rsrc references to rendering the `explore` playbook.
- Regenerate manifest and wsflow rsrc mirror.
- Update `workflow-skills`, `workflow-skills` mental model, and `prompt-bundle`
  mental model to mark `explore` as fallback/compatibility.
- Verify remaining references and run focused tests.

## Constraints

- Preserve host-neutral text in shared guidance.
- Preserve wsflow generated-sameness for `rsrc/`.
- Preserve English written artifacts.

## Out of scope

Phase 2 decisions about deleting, renaming, or repurposing the generic
`explore` playbook.

## Verification Contract

- Escaped search finds no shipped normal-path guidance using
  `playbook.render(name: "explore")` or `playbook.print(name: "explore")`.
- `go test -count=1 ./internal/wsrsrc ./internal/mcp` passes or failures are
  identified as pre-existing.
- `python3 -m unittest discover agents-plugin-wsflow/tests` is run or an
  explicit pre-existing failure is reported.

## References

- [Must] `ai-docs/tickets/ready/260615-refactor-native-explore-dispatch-skill-guidance.md` - Phase 1 scope and guardrails.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow skill contracts and call-site expectations.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc manifest and `explore` compatibility constraints.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow rsrc generated-sameness contract.
