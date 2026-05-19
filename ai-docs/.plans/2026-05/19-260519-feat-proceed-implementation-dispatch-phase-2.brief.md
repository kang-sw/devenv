# Brief: 260519-feat-proceed-implementation-dispatch Phase 2

## Intent

Teach proceed to decide implementation dispatch from conversation and workflow
artifacts before the implementation handoff, then carry that decision as a
downstream lower bound.

## Scope Boundary

Implement only Phase 2. Phase 1 skeleton wording cleanup is complete and must
not be revisited except to preserve consistency.

## Caller-Visible Contract

Full ws `lead-proceed` announcements include:

- `Implementation Dispatch`: `direct-edit` or `write-code`.
- `Dispatch Reason`: the predicate that selected the dispatch.
- `Branch Mode`: direct current branch, create `implement/<scope>`, continue
  `implement/*`, or sprint blocked.

Full ws `lead-implement` honors a proceed-selected dispatch as a lower bound:
it may escalate `direct-edit` to `write-code`, but it must not downgrade
`write-code` to direct edit.

wsflow `lead-proceed` announcements include:

- `Execution Path`: `wsflow:lead-implement -> wsflow:lead-edit`.
- `Complexity Flag`: narrow, broad, caller-visible, or cross-module.
- `Branch Mode`: continue current branch, create branch when explicitly
  requested or repository rules require it, or sprint blocked.

wsflow skill text must not mention `write-code` or excluded full ws skills.

## Implementation Strategy Decisions

- `lead-proceed` remains source-free: use only conversation state, ticket
  frontmatter/body, phase text, status, category, spec links, and plans.
- Direct edit is selected only when all predicates are explicitly known true.
- Unknown predicates select `write-code` in full ws and at least `broad` or
  `caller-visible` complexity in wsflow when applicable.
- Ready tickets and spec-linked changes default to `write-code` unless direct
  edit is unambiguously proven from artifacts.
- `lead-proceed` still hands work to `lead-implement`; it does not invoke edit
  or write-code directly.

## Rejected Alternatives

- Do not copy full ws `write-code` language into wsflow.
- Do not require source inspection or implementation plan reads in proceed.
- Do not let `lead-implement` downgrade a proceed-selected `write-code`.

## Approach

- Add full ws route-context fields for implementation dispatch, dispatch
  reason, and branch mode.
- Add full ws dispatch and branch-mode judgments.
- Update full ws announce and carried context templates.
- Update full ws `lead-implement` assessment to read and honor carried
  implementation dispatch.
- Add wsflow route-context fields for execution path, complexity flag, and
  branch mode.
- Add wsflow complexity and branch-mode judgments without mentioning excluded
  full ws skills.

## Constraints

- Follow skill-authoring compression and invariant checklist.
- Keep wsflow distributed skill text free of full ws-only names.
- Keep Phase 2 behavior text self-contained; no references to this ticket.

## Out of scope

- Runtime enforcement outside skill instructions.
- Changes to MCP tools, source code, or tests beyond skill bundle verification.
- New skeleton routing.

## Details

Target files:

- `agents-plugin/skills/lead-proceed/SKILL.md`
- `agents-plugin/skills/lead-implement/SKILL.md`
- `agents-plugin-wsflow/skills/lead-proceed/SKILL.md`
- `agents-plugin-wsflow/skills/lead-implement/SKILL.md`
- `ai-docs/spec/workflow-skills.md`
- `ai-docs/mental-model/workflow-skills.md`

## Verification Contract

- Inspect full ws `lead-proceed` text for `Implementation Dispatch`,
  `Dispatch Reason`, `Branch Mode`, and direct-edit predicate rules.
- Inspect full ws `lead-implement` text for lower-bound handling.
- Inspect wsflow `lead-proceed` text for `Execution Path`, `Complexity Flag`,
  and `Branch Mode`.
- `rg -n "lead-write-code|ws:|ws/" agents-plugin-wsflow/skills`
  should only match allowed test/reference contexts outside distributed skill
  text; package tests are authoritative.
- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `ws/spec_index.verify`

## References

- `ai-docs/spec/workflow-skills.md` - planned proceed dispatch behavior.
- `ai-docs/mental-model/workflow-skills.md` - implementation routing and
  documentation ownership model.
- `ai-docs/ref/wsflow-mirroring.md` - wsflow mirror and forbidden-reference
  rules.
