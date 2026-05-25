---
title: wsflow lead-implement mirroring gap
related:
  260520-refactor-lead-skill-cascade: discovered while closing the lead skill cascade; full lead-implement changed shape while wsflow kept lead-edit delegation
related-mental-model:
  - workflow-skills
completed: 2026-05-23
---

# wsflow lead-implement mirroring gap

## Background

`ai-docs/ref/wsflow-mirroring.md` requires each included `agents-plugin/skills/lead-*`
behavior change to either update the matching `agents-plugin-wsflow/skills/lead-*`
skill in the same logical change or leave an explicit follow-up. The lead skill
cascade unified full `ws:lead-implement` into one implementation spine and
deleted full `ws:lead-edit` / `ws:lead-write-code`, but wsflow still ships
`wsflow:lead-edit` and has `wsflow:lead-implement` delegate execution to it.

This may be an intentional product difference because wsflow is curated rather
than generated. It still needs an explicit resolution so future mirroring audits
do not treat the divergence as an untracked accident.

## Phases

### Phase 1: Resolve wsflow implement shape

Decide whether wsflow should mirror the full unified `lead-implement` spine or
document `wsflow:lead-edit` delegation as an intentional package-specific
difference.

If mirroring is chosen, update `agents-plugin-wsflow/skills/lead-implement` and
dependent wsflow skill text to remove stale `lead-edit` ownership assumptions.
If divergence is chosen, update `ai-docs/ref/wsflow-mirroring.md`,
`ai-docs/spec/workflow-skills.md`, and the workflow-skills mental model with the
explicit rationale.

Verification:
- No stale full/wsflow drift remains undocumented for `lead-implement`.
- wsflow distributed skill tests pass, except for any separately documented
  unrelated inventory-drift failure.

### Result (e15b0451) - 2026-05-23

Resolved by choosing intentional wsflow divergence for now. `a3a2a332`,
`baccb851`, and `ce3633ea` documented that wsflow source execution remains
`lead-edit`-mediated while full ws uses the unified `lead-implement` spine.
`e15b0451` made the mirroring reference and wsflow bundle test allow only the
documented `lead-edit` exception, so future wsflow drift remains visible.
