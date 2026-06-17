# Brief: 260616-refactor-wsflow-product-mode-convergence Phase 3

## Intent

Collapse the wsflow distributed lead skill files from curated procedure bodies to
thin user-entry shims that execute the shared product-mode playbook surface. This
removes the duplicated wsflow procedure corpus while preserving the same shipped
skill inventory and leaving runtime `prompt.render` removal to Phase 4.

## Scope Boundary

Selected scope: `Phase 3: collapse wsflow skills to thin shims` in
`260616-refactor-wsflow-product-mode-convergence`.

In scope:

- Replace maintained procedure text in shipped `agents-plugin-wsflow/skills/lead-*`
  `SKILL.md` files with thin entry shims.
- Keep each shipped wsflow skill directory and `name: wsflow:lead-*`
  invocation stable.
- Update wsflow package tests so they assert shim shape, inventory, forbidden
  full-ws references, and product-mode rendered output where appropriate.
- Preserve the existing product-mode `playbook.print` / `playbook.render`
  behavior implemented by earlier phases.

Out of scope:

- Removing `prompt.render`.
- Removing the wsflow rsrc mirror.
- Changing full ws skill shims or shared lead playbook semantics unless a test
  reveals a strict dependency needed for wsflow shims.
- Changing the wsflow shipped skill inventory.

## Caller-Visible Contract

Every shipped wsflow skill remains invocable under the same package-qualified
`wsflow:lead-*` skill name, while its `SKILL.md` frontmatter keeps the existing
bare `name: lead-*` package-local convention. The `SKILL.md` body is only a shim that calls
`wsflow/playbook.print(name: "<lead-name>")` and executes the returned procedure
inline. Workflow behavior comes from shared rsrc playbooks rendered in wsflow
product mode.

## Contract Instructions

- Use the existing full ws thin-shim pattern as the canonical shape, adapted to
  wsflow notation.
- Keep frontmatter `name:` and `description:` intact unless tests prove a stale
  description is tied to the curated body.
- Use `wsflow/playbook.print(name: "<lead-name>")` in each wsflow skill body.
- Do not use `ws/`, `ws:`, `ws.`, `ws.mercenary.*`, `subquery`, excluded skill
  names, or full-ws-only wording in distributed wsflow skill text.
- Do not hand-copy or synthesize shared procedure bodies into wsflow skills.
- Keep `agents-plugin-wsflow/rsrc/` byte-identical to `agents-plugin/rsrc/`; this
  phase should not require rsrc regeneration unless shared playbook text changes.
- If a shipped wsflow skill has no corresponding shared rsrc playbook, escalate
  instead of leaving a curated body behind.

## Integration Test Instructions

Primary verification:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `go test -count=1 ./...` from `agents-plugin-tool`

Focused expectations:

- wsflow package tests must fail if a shipped wsflow skill carries a curated
  procedure body instead of the thin `playbook.print` shim.
- Existing forbidden-reference and inventory tests must still pass.
- Product-mode playbook rendering tests must continue proving wsflow output does
  not expose hidden full-ws-only guidance.

## Implementation Strategy Decisions

- Prefer mechanical generation of shim bodies from the existing wsflow skill
  inventory to avoid hand divergence.
- Treat `SKILL.md` files as entry points only; shared rsrc playbooks remain the
  behavior source of truth.
- Keep Phase 4 doctrine cleanup separate: references may say `prompt.render`
  remains live until later removal.

## Rejected Alternatives

- Keeping curated wsflow skill bodies and only updating docs is rejected because
  the Phase 3 goal is to remove the duplicate procedure corpus.
- Making wsflow skills byte-identical to full ws skills is rejected because
  wsflow must retain `wsflow:` skill names and `wsflow/` MCP notation in
  distributed text.
- Removing `prompt.render` during this phase is rejected because Phase 4 owns
  tool-surface deletion and stale doctrine cleanup.

## Approach

- Survey current wsflow skill files, full ws shim shape, shared rsrc playbook
  coverage, and wsflow tests.
- Convert every shipped wsflow `SKILL.md` to the same minimal shim structure.
- Update tests to enforce the new shim contract and rendered-output safety.
- Run wsflow package tests and full tooling tests.

## Constraints

- AI-authored docs, plans, commits, tickets, and code comments stay English.
- Do not change phase plan text after existing Result sections; append Result
  only during closeout.
- Keep `.codex` and unrelated untracked files out of commits.
- Preserve user or generated changes not introduced by this implementation.

## Out of scope

- Phase 4 `prompt.render` removal and `spec-remove` closure.
- Any new wsflow-only skill or tool.
- Any new root-level project layout.

## Details

The expected wsflow shim shape is:

```markdown
---
name: lead-<name>
description: <existing description>
---

# <Title>

Call `wsflow/playbook.print(name: "lead-<name>")` and execute the returned procedure
inline against the user request.
```

Title text may preserve the existing local title when it is already concise and
not tied to the curated body.

## Verification Contract

Required before implementation commit:

- `python3 -m unittest discover agents-plugin-wsflow/tests`
- focused tests updated or added for thin-shim enforcement
- `go test -count=1 ./...` in `agents-plugin-tool`
- `git diff --check`

Required before final gate after docs:

- `ws/spec_index.verify`
- repeat package tests affected by doc/test changes if needed

## References

- [Must] `ai-docs/tickets/ready/260616-refactor-wsflow-product-mode-convergence.md` - selected Phase 3 and deferred Phase 4 boundary.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - binding convergence direction and wsflow endpoint.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow skill inventory, forbidden-reference rules, and rsrc mirror contract.
- [Must] `ai-docs/mental-model/workflow-skills.md` - entry-shim and wsflow skill surface contracts.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc as single source of truth and wsflow product-mode rendering boundary.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - product-mode gates and playbook rendering/tool visibility separation.
- [Must] `ai-docs/spec/workflow-skills.md` - wsflow skill surface and converged implementation spine contracts.
- [Must] `ai-docs/spec/mcp-tools.md` - playbook tools, wsflow agentless mode, and prompt.render bridge contracts.
- [Must] `ai-docs/spec/plugin-runtime.md` - wsflow package/runtime capability contract.
