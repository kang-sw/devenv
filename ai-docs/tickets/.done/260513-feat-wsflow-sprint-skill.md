---
title: wsflow sprint skill
parent: 260513-epic-wsflow-agentless-plugin
related:
  260513-feat-wsflow-agentless-plugin-scaffold: established initial wsflow skill bundle and drift guards
spec:
  - 260513-wsflow-sprint-skill
related-mental-model:
  - workflow-skills
  - plugin-runtime
completed: 2026-05-13
---

# wsflow sprint skill

## Background

The initial wsflow scaffold intentionally excluded `lead-sprint` with the other
persistent multi-turn orchestration skills. The preferred scope changed after
that scaffold was completed: wsflow should include a sprint workflow, but only
as an agentless sprint-branch session container.

The wsflow sprint skill should preserve the useful part of the full ws sprint
workflow: work continues on a `sprint/` branch, individual tasks avoid repeated
documentation passes, and wrap-up performs one consolidated documentation and
ticket/index closeout. It must not reintroduce wsflow-managed named agents,
subquery result collection, delegated write-code, skeleton routing, or
managed mental-model updater dispatch.

## Constraints

- Keep wsflow distributed skill text under `wsflow:lead-*` and
  `wsflow/<tool>` notation.
- Keep mutations lead-owned. Native host subagents may be mentioned only for
  bounded read-only investigation or review.
- Do not add `lead-write-code`, `lead-write-skeleton`, `lead-salvage`, or
  `lead-skill-authoring` to the wsflow skill set.
- Update wsflow drift tests and mirroring guidance with the new shipped skill
  inventory.

## Phases

### Phase 1: Add wsflow sprint session container

Add `agents-plugin-wsflow/skills/lead-sprint/SKILL.md` as a curated wsflow
rewrite of the full sprint workflow. The skill should create or continue
`sprint/` branches, loop over user tasks, route source changes through
`wsflow:lead-edit`, defer documentation during the task loop, and run a
lead-owned wrap-up through `wsflow:lead-update-spec`, mental-model review,
executor wrap-up, ticket/index updates, verification, and merge or branch
cleanup.

Update wsflow package tests, specs, mental models, project memory, and
`ai-docs/ref/wsflow-mirroring.md` so `lead-sprint` is treated as included in
wsflow while the other excluded orchestration or authoring skills remain
excluded.

Acceptance criteria:

- `agents-plugin-wsflow/skills/lead-sprint/SKILL.md` exists and uses wsflow
  notation only.
- The wsflow sprint skill does not mention `ws/subquery`, `ws/agents.*`,
  `mental-model-updater`, `lead-write-code`, `lead-write-skeleton`,
  `lead-salvage`, or `lead-skill-authoring`.
- The wsflow sprint skill routes source changes through `wsflow:lead-edit`.
- The wsflow sprint wrap-up keeps documentation updates lead-owned and does not
  dispatch managed updater agents.
- wsflow skill inventory tests expect `lead-sprint` as an included skill and
  continue to reject forbidden managed-agent surfaces.
- The workflow-skills spec and workflow-skills mental model describe wsflow
  sprint as included behavior.

### Result (792595f) - 2026-05-13

Added `agents-plugin-wsflow/skills/lead-sprint/SKILL.md` as an agentless
sprint-branch session container. The skill creates or continues `sprint/`
branches, routes source changes through `wsflow:lead-edit`, defers
documentation until wrap-up, and keeps spec, mental-model, ticket, index,
merge, and cleanup mutations lead-owned.

Updated the wsflow skill-bundle test inventory, workflow-skills spec,
workflow-skills mental model, and `ai-docs/ref/wsflow-mirroring.md` so
`lead-sprint` is treated as an included wsflow skill while `lead-write-code`,
`lead-write-skeleton`, `lead-salvage`, and `lead-skill-authoring` remain
excluded.

Verified `python3 -m unittest discover agents-plugin-wsflow/tests`,
`claude plugin validate agents-plugin-wsflow`, forbidden-pattern scanning over
`agents-plugin-wsflow/skills`, and `ws/spec_index.verify`.
