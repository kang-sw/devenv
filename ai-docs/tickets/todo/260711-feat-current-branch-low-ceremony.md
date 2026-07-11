---
title: "Allow low-ceremony direct edits to stay on the current branch"
sage-review-design: required
related:
  260630-epic-skill-playbook-diet: preserve the dieted playbook boundary by keeping deterministic routing in enter.implement
  260703-chore-implement-branch-rename-default-allow: adjacent branch-plan policy work whose isolation behavior remains the standard path
spec:
  - 260505-implementation-workflow-skills
  - 260625-session-state-tools
related-mental-model:
  - workflow-skills
  - mcp-runtime
---

# Allow low-ceremony direct edits to stay on the current branch

## Background

`lead-proceed` and `lead-implement` already compose most of a lightweight
implementation route: a narrow inline target may need no ticket, direct edits
may stay lead-owned, low-risk work receives lead-only review, and documentation
may be skipped with a recorded reason. The remaining implementation verdict
still creates an `impl/*` branch and installs merge work even when all of those
conditions hold. For a caller who treats push as the external publication
boundary, that local branch/merge ceremony adds cost without a proportional
risk reduction.

The lightweight behavior must emerge from existing verdict facts rather than
introducing another user-facing mode, configuration hierarchy, or large
playbook rule set.

## Decisions

- Keep `lead-proceed -> lead-implement -> enter.implement` as the only
  implementation spine; do not add a hotfix/tweak skill or executor.
- Do not add a named requested/resolved profile. Explicit hotfix, tweak, or
  simple-fix wording may help the lead supply existing caller-policy facts, but
  it does not override risk classification.
- Resolve current-branch execution only when the target is inline, the observed
  current branch is neither `impl/*` nor legacy `implement/*`, and the raw facts
  independently satisfy the automatic low-risk predicates: automatic
  direct-edit eligibility, automatic lead-only review eligibility with no
  review override, and skipped documentation with a non-empty reason. Explicit
  direct-edit or lead-only overrides do not satisfy these eligibility checks.
- Under that conjunction, keep the current branch, omit merge work, run focused
  verification and lead-owned review, and create one logical explicit-path
  commit with `## AI Context`. Never push as part of this path.
- Any failed or unknown predicate retains the standard isolated-branch path.
- An invocation already on `impl/*` or legacy `implement/*` always retains the
  standard continue/rename, final-action, and merge behavior even when every
  low-risk predicate otherwise matches.
- Preserve the dieted playbooks: prefer edits to existing judgment/policy
  wording, with no new eligibility matrix or duplicated resolver rules.
- Sprint removal is separate scope; this change must not depend on
  `lead-sprint` or its episode markers.

## Constraints

- Do not reinterpret `explicit_direct_edit_request=yes` or
  `policy.review.override=lead-only` as current-branch authorization. Eligibility
  must be recomputed from the unoverridden scope and risk facts.
- File count alone is not an eligibility predicate. Semantic blast radius,
  review allocation, documentation reachability, and rollback clarity remain
  decisive through existing facts.
- Public contracts, schemas, protocols, security-sensitive changes,
  cross-module patterns, unknown risk, or insufficient verification must not
  reach current-branch execution because they cannot resolve the required
  verdict conjunction.
- Keep `enter.implement` and its installed todo instructions authoritative.
  Playbook prose must not bypass or contradict the branch verdict.
- Standard implementation-branch creation, merge approval, and cleanup behavior
  must remain unchanged for every non-matching verdict.

## Phases

### Phase 1: Derive current-branch completion from the low-risk verdict conjunction

- Extend the implementation resolver and session todo derivation so the exact
  inline, non-implementation-current-branch, automatically-safe direct-edit,
  automatically-derived lead-only, and docs-skipped conjunction stays on the
  current branch and terminates without a merge todo.
- Keep verification, commit, lead-only review rationale, final reporting, and
  the no-push boundary explicit in generated instructions.
- Make only compact edits to existing `lead-proceed` and `lead-implement`
  wording needed to pass explicit caller intent into existing facts; do not add
  a new mode section or duplicate runtime predicates.
- Update the existing implementation-workflow spec and affected mental models
  after behavior is implemented; update the public `enter.implement` branch
  action and todo-derivation contract under `260625-session-state-tools` as well.
- Verify matching and near-miss resolver cases, todo ordering/omission, standard
  branch behavior preservation, rendered canonical/wsflow playbooks, manifests,
  and the wsflow package test suite.
