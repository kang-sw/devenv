---
title: implement branch squash gate
related-mental-model:
  - workflow-skills
  - git-workflow-tools
---

# implement branch squash gate

## Background

`ws:lead-implement` currently leaves implementation branch history in the same
granular shape used for recovery checkpoints: brief, plan, implementation,
review fixes, spec updates, mental-model updates, and ticket closeout are often
separate commits. That is useful while the branch is in flight, but it leaves
small bugfixes with excessive main-history noise after merge.

The desired policy is to keep ready-ticket promotion and shared queue state on
`main`, then limit any squash operation to commits created on the implementation
branch after implementation begins. Squash should run after the final doc gate
has completed and before the merge gate waits for user approval.

## Decisions

- Do not rewrite discussion, spec-planning, or ticket-promotion commits that
  occurred on `main` before the implementation branch was created.
- Implementation branches may keep granular recovery checkpoints while work is
  active.
- Before merge approval, `ws:lead-implement` should prepare a cleaner branch
  history by squashing branch-local commits into logical buckets.
- Default bucket shape should be conservative:
  - pre-implementation docs: brief, plan, and implementation-only preparation;
  - implementation: source, tests, and review-fix commits;
  - post-implementation docs: implemented spec updates, mental-model updates,
    ticket Result/closeout, and index queue updates.
- Small direct edits may squash to a single logical commit.
- Larger implementation slices may keep multiple implementation commits when
  independent source surfaces or reviewable behavior changes warrant it.

## Phases

### Phase 1: Add implementation-branch squash gate

Update `ws:lead-implement` so implementation-branch modes run a squash/cleanup
step after the final doc commit gate and before the final action gate reports
merge readiness.

The squash step should operate only on commits created on the implementation
branch, not on the main-branch ready-promotion commits that preceded branch
creation. It should preserve user intervention safety: if history rewriting is
ambiguous, conflicts, or cannot preserve required commit metadata, stop and
report the blocker instead of forcing a rewrite.

The expected final branch history should normally be two or three logical
commits:

- `docs(plan): prepare <scope>`
- `<type>(<scope>): implement <scope>`
- `docs(ticket): close <scope>`

Commit-message synthesis must preserve the important workflow metadata from the
granular commits, including AI Context rationale, Ticket Updates, Spec entries,
Mental Model Notes, review-fix rationale, and ticket closeout evidence.

Verification should cover:

- the skill text clearly places the squash step before the merge gate;
- the squash scope excludes ready-promotion commits on `main`;
- the fallback path is explicit when safe rewriting is not possible;
- downstream merge instructions refer to the squashed branch history rather than
  the pre-squash checkpoint count.
