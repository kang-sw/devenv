---
title: "Use a safe low-ceremony preference for current-branch completion"
sage-review-design: completed
sage-review-completeness: completed
related:
  260711-feat-current-branch-low-ceremony: predecessor that implemented raw-fact safety but left low-ceremony preference implicit in documentation policy
spec:
  - 260505-implementation-workflow-skills
  - 260625-session-state-tools
related-mental-model:
  - workflow-skills
  - mcp-runtime
completed: 2026-07-11
---

# Use a safe low-ceremony preference for current-branch completion

## Background

The implemented low-ceremony path correctly recomputes automatic direct-edit
and lead-only-review safety from raw facts, so caller overrides cannot authorize
risky work. Its selection contract is still incomplete: `Branch Action:
current` has no explicit low-ceremony preference input. An inline low-risk
request with `policy.docs.mode=skip-with-reason` currently retains the branch
even when the caller requested only documentation omission.

Using `explicit_direct_edit_request` as the missing signal is unsafe because
that field intentionally forces direct editing independently of scope and risk.
Requiring callers to name a branch strategy would also expose workflow
mechanics that should remain automatic for users who only know `discuss` and
`proceed`. Documentation skip is a distinct policy and must not remain a proxy
for low-ceremony preference.

## Decisions

- Add `policy.low_ceremony_if_safe: yes|no|unknown` to `enter.implement`.
- Treat `yes` only as a preference to use the lightest safe implementation
  path. It grants no safety authority, requires no branch terminology from the
  caller, and changes no delegation, review, or documentation verdict by itself.
- The lead sets `yes` only when the caller clearly asks to omit normal ceremony
  and proceed directly, such as "skip the procedure and fix it directly."
  Words such as `hotfix`, `tweak`, or `small fix` alone remain scope or urgency
  hints rather than a low-ceremony preference signal.
- `no` or `unknown` cannot produce `Branch Action: current`; the standard
  create/rename/continue/stop path remains authoritative.
- With `yes`, retain the existing raw-fact conjunction unchanged: inline target,
  valid named non-implementation branch, automatic direct-edit eligibility,
  automatic lead-only review with `review.override=auto`, no explicit
  delegation request, and documentation skipped with a non-empty reason.
- If `yes` is rejected by any eligibility predicate, fall back to the standard
  branch/delegation/review/docs verdict and emit a concise warning that the
  low-ceremony preference was not applicable. Do not stop otherwise valid work.
- Do not reuse or reinterpret `explicit_direct_edit_request`, documentation
  policy, `allow_rename`, or another existing field as the intent signal.
- Keep shared playbook edits compact and pre-call only: modify existing policy
  wording rather than adding a mode section, eligibility matrix, or resolver
  predicate copy.
- Do not establish a user-facing keyword or require callers to learn branch,
  review, or documentation vocabulary; the field is an internal normalized
  preference supplied by the lead.

## Constraints

- The new policy field is an opt-in gate in addition to existing safety checks,
  never an override of them.
- A risky request may still proceed through the normal isolated, delegated, and
  reviewed path; only current-branch completion is denied.
- `policy.docs.skip-with-reason` remains independently usable on standard
  implementation branches.
- Existing `Branch Action: current` completion todos and no-push boundary remain
  unchanged after eligibility succeeds.
- wsflow product mode must expose the same schema, verdict, playbook behavior,
  and tests as the canonical package.

## Phases

### Phase 1: Add a low-ceremony preference gate without weakening raw-fact safety

- Extend the public `enter.implement` policy schema, normalization,
  conditions, agenda/verdict output, and tests with
  `policy.low_ceremony_if_safe`.
- Require `low_ceremony_if_safe=yes` in current-branch eligibility and add positive,
  `no`, `unknown`, missing, rejected-request, and standard-branch preservation
  cases through the public resolver/enter path.
- Ensure a rejected request preserves independently derived delegation, review,
  documentation, branch, final-action, and merge todos.
- Compactly update existing `lead-implement` policy wording so clear caller
  preference for reduced ceremony maps to `low_ceremony_if_safe=yes` without
  requiring branch language, while `hotfix`/`tweak` wording alone remains
  insufficient.
- Regenerate canonical and wsflow manifests/mirrors and update the existing
  workflow/session-state specs and mental models after implementation.

### Result (bd855ea3) - 2026-07-11

Added `policy.low_ceremony_if_safe: yes|no|unknown` as a preference-only
`enter.implement` input. Only explicit `yes` may attempt the existing
current-branch exception, while every raw direct-edit, review, risk,
documentation, target, and branch predicate remains independently mandatory.
Missing, `no`, `unknown`, and rejected `yes` requests preserve the standard
derived branch, delegation, review, documentation, final-action, and merge
path; rejected `yes` also emits an observable not-applicable warning.

The lead playbook now maps clear requests for a streamlined implementation flow
without requiring branch vocabulary, keeps the preference independent from
direct-edit and documentation policy, and treats `hotfix`, `tweak`, or `small
fix` alone as insufficient. Canonical and wsflow resources remain generated and
byte-identical. Full Go and wsflow tests passed, and partitioned correctness,
fit, and test review finished clean after one Important Layer-boundary finding
was fixed and re-reviewed.
