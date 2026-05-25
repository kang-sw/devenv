---
title: Emit an implementation verdict before lead-implement starts work
spec:
  - 260505-implementation-workflow-skills
related:
  260525-bug-actor-gate-verbose-setup-guidance: adjacent dogfood turn that exposed workflow output boundaries
completed: 2026-05-25
---

# Emit an implementation verdict before lead-implement starts work

## Background

`lead-proceed` already emits a `Routing Verdict` before invoking the next
workflow stage. `lead-implement` should similarly make its own execution route
visible before it starts implementation work, so the user can see whether the
skill chose direct edit or delegation, branch behavior, plan depth, and review
allocation before source edits begin.

This is not a replacement for `lead-proceed`'s verdict. `lead-proceed` decides
which skill runs next; `lead-implement` should report only its internal
implementation route.

## Phases

### Phase 1: Add lead-implement implementation verdict

Update `lead-implement` so after it applies its route judgments and before Prep
or source inspection, it emits an implementation verdict that names the selected
delegation mode, branch mode, plan depth, review allocation, target, selected
scope, and decisive reason.

The verdict should be concise and mechanically shaped, similar to
`lead-proceed`'s `Routing Verdict`, but should not reuse `NEXT:` because
`lead-implement` is not routing to a sibling workflow skill at that point.

Mirror the behavior in the shipped `wsflow:lead-implement` skill using wsflow
names and no full-ws tool notation. Run the skill-authoring fresh-reader audit
and wsflow package tests.

### Result (a66ba52) - 2026-05-25

`lead-implement` now emits a non-blocking `## Implementation Verdict` after
route judgments and before Prep. The verdict reports target, mode, branch mode,
plan depth, review allocation, scope, and decisive route facts, then continues
immediately without using `NEXT:`.

`wsflow:lead-implement` mirrors the checkpoint with target, branch mode, scope,
and reason only; edit mode, plan depth, and review allocation remain owned by
`wsflow:lead-edit` and later outcome reporting.

Review-driven fixes made both skills record `<current-branch>` before applying
branch-mode judgment and reuse that value during Prep/Prepare. Tests now lock
the exact ordered verdict fields for full ws and wsflow and reject `NEXT:` lines
inside the verdict template.

Verification:

- `python3 -m unittest agents-plugin/tests/test_skill_dispatch_contracts.py`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- Fresh-reader audit: clean after three cycles
- Consistency sweep: clean after review-driven fixes
