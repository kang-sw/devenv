---
title: Open Decision Queue splits announced defaults from policy questions
related:
  260726-feat-doc-organization-autonomy-odq-admission-filter: absorbed and dropped; its placement-autonomy concern is a subset of the announced-default class here
  260730-feat-odq-batch-interview: prerequisite, done; the batch interview this ticket reshapes
  260924-feat-open-decision-queue-response-format: done; the current Response format this ticket amends
---

# Open Decision Queue splits announced defaults from policy questions

## Background

The `lead-ticket` Open Decision Queue admits every unconfirmed item that could
change ticket text and presents each one the same way: context, alternatives,
a recommendation, then an explicit answer. In practice most items have one
answer that follows from something citable, and the owner ends up answering
"as recommended" to all of them to reach the one or two that actually need a
judgment.

Owner observation (2026-09-25 session): two ticket settlements in a row ran
this way. For `260924-chore-review-sweep-test-and-naming-minors`, seven queued
items had six whose answer was forced by a Go language limit (`_test.go`
helpers cannot be shared across packages), a prior commit decision
(ca734c1d), or a plain consolidation; only one reversed a prior decision
(7c16252a) and needed the owner. The owner asked to leave only the policy
questions, then asked to make that the procedure. Both settlements also hit
the Final confirmation step, which re-asked for approval of items already
answered.

`260726-feat-doc-organization-autonomy-odq-admission-filter` raised the same
problem for documentation-placement decisions and deferred its boundary until
the batch interview landed (done, 766281e65). Its counter-evidence still
applies: in a downstream field report, seven items an agent had internalized
as settled were queued, and two were materially revised once actually asked.
Whatever the agent treats as determined must still pass in front of the owner.

## Decisions

- **The queue has two classes, presented in two groups.**
  - **Announced defaults** (upper group): one line each, "will do X", with
    the citation that makes the alternatives lose. No per-item answer is
    requested.
  - **Policy questions** (lower group, visibly marked as such): the current
    block format - context, alternatives, recommendation.
- **Admission to the announced-default class requires a citable reason**, one
  of: a language or platform constraint; a prior commit or ticket decision; an
  established project convention; or a direct consequence of an
  already-confirmed decision. Documentation placement (parent/related,
  epic-child vs standalone, absorb vs rewrite, initial status, stem naming,
  which commit carries the edit) is announced-default by default. Anything
  without such a citation is a policy question.
- **An item that reverses a prior decision is always a policy question**, even
  when the lead can cite a reason for the reversal.
- **Announced defaults are confirmed by one explicit owner acknowledgement**
  covering the whole group, which may arrive in the same turn as the policy
  answers. The owner may object to any announced item; that item becomes a
  policy question under its existing ID.
  - Rejected: silence as consent (settling announced items when the owner
    answers only the policy questions). It breaks the rule that only
    explicitly confirmed decisions are persisted.
- **The Final confirmation step is removed.** Once announced defaults are
  acknowledged and every policy question is settled, the lead persists
  without re-showing the confirmed set for another approval. The
  trace-before-write step and the Sage reviewers remain the guard against
  interacting decisions.
  - Rejected: keeping Final confirmation. Under the two-class queue, the
    acknowledgement already shows every item at once, so the step re-asks the
    same content.
- **The queue mechanism itself stays.** Items still get stable IDs, live in the
  ticket's temporary `## Open Decision Queue` section, and block persistence
  until settled.

## Constraints

- `lead-ticket` is a shipped playbook: the change follows
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/skill-authoring.md`, and its wsflow mirror follows
  `ai-docs/manuals/wsflow-mirroring.md`.
- No text may cite this repository's own tickets or commits as the rule's
  authority (Architecture Rule 4); the examples above are motivation, not
  shipped content.

## Phases

### Phase 1: Two-class queue and Final confirmation removal

Amend the `lead-ticket` playbook's Open Decision Queue admission, Queue state,
Response format, and Final confirmation sections to the Decisions above, and
mirror the change to wsflow. Check whether `lead-discuss`'s capture handoff
and any contract test that pins the ODQ text (for example
`agents-plugin/tests/test_skill_dispatch_contracts.py`) need the matching
change.

Verification:

- The rendered `lead-ticket` playbook presents the two groups with the policy
  group visibly marked, states the citation requirement and the
  reversal-is-policy rule, requires one explicit acknowledgement for announced
  defaults, and no longer contains a Final confirmation step.
- The full and wsflow packages' tests pass, including skill-shim and mirror
  drift tests.
