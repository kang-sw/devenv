---
title: Per-slice review — one repair relay with Critical-only re-review
parent: 260824-epic-review-watermark-model
related:
  260824-feat-per-phase-review-floor: completed predecessor that made a single delegated reviewer the normal per-phase floor; this ticket changes its relay behavior
  260824-feat-lead-review-range-scenario: adjacent stronger range-review layer; this ticket must not assume a fixed integration boundary
  260726-bug-lead-implement-lost-review-relay-cycle-cap: prior multi-cycle contract that this ticket deliberately replaces rather than reopens
related-mental-model:
  - workflow-skills
  - mcp-runtime
sage-review-design: completed
---

# Per-slice review — one repair relay with Critical-only re-review

## Background

The current per-slice review runbook counts the initial review as cycle 1 and
then permits one relay for `single` allocation or two for `partitioned`
allocation. That made sense while the slice loop itself was the only practical
place to reach a settled review result, but it repeatedly consumes lead and
reviewer context on work that will later receive a broader integration review.

The desired local shape is deliberately smaller:

```text
implement -> review #1 -> one disposition/fix relay -> closeout
```

The single relay still records a response for every actionable finding, rather
than silently discarding it. In particular, `[won't fix: <reason>]` remains a
supported disposition. A default second review is removed. The only exception
is a Critical finding: this ticket uses the existing `Critical` severity rather
than introducing a new `fatal` label, so it receives one Critical-scoped
re-review after the repair relay. A remaining Critical finding stops the slice
from merging and is escalated with its durable evidence.

This ticket intentionally does **not** define a universal integration-review
boundary. Existing downstream review boundaries are too stale and varied to
make that prerequisite a truthful local rule. Stronger range review and its
durable slice artifacts remain adjacent work, not an assumed substitute for a
missing local disposition.

## Decisions

- Count review rounds consistently with the existing contract: initial review is
  review #1; the ordinary path has no review #2. The normal loop is therefore
  one repair relay, not one review round.
- Every non-clean Critical/Important finding from review #1 receives exactly
  one recorded relay disposition: `[fixed]`, `[won't fix: <reason>]`,
  `[deferred: <reason>]`, or `[escalate: <reason>]`. Preserve the existing
  admissibility and lead-owned decision rules for those markers.
- Use the initial review's `Critical` severity as the sole default trigger for
  review #2. Do not wait for the old `fixed -> re-review unresolved`, repeated
  root-cause, or reviewer-churn signals: those signals are observable only
  after the re-review this policy removes and are therefore stale as a default
  routing condition.
- Review #2 is scoped to the Critical findings and their fixes. If it remains
  Critical, stop rather than opening another relay; report and ticketize or
  otherwise explicitly escalate the unresolved finding.
- Do not invoke `review-adjudicator` or the elevated-implementer path in the
  normal one-relay flow. Retain or reshape them only where the Critical
  exception demonstrably needs them; do not preserve multi-cycle machinery by
  default merely because it existed under the replaced budget.
- Keep reviewer findings and disposition records file-first. The lead receives
  paths and compact verdicts, not copied report bodies, so a later review or
  compaction can recover the evidence.

## Constraints

- The generated review todo Instruction is the binding execution surface.
  `lead-implement` directs the lead to follow that installed instruction rather
  than supplementing it from remembered playbook prose; changing shared prose
  alone is insufficient.
- This replaces the policy restored by
  `260726-bug-lead-implement-lost-review-relay-cycle-cap`; do not edit that
  closed ticket or describe the new behavior as repairing its old defect.
- `Critical`, `Important`, and `Minor` remain the per-slice severity vocabulary.
  Do not introduce an additional `fatal` severity label. Range/release review
  may use its own blocking-verdict vocabulary.
- Preserve the existing risk-based reviewer allocation. This ticket changes
  loop behavior after allocation, not the `lead-only`/`single`/`partitioned`
  resolver.

## Phases

### Phase 1: Atomically replace the multi-cycle slice loop

- Change the generated `lead-implement` review instruction and its tests so all
  relaying allocations run review #1 followed by at most one recorded repair
  relay, then closeout without a default re-review.
- Keep the reviewer artifact and implementer disposition inputs self-contained.
  Require the normal relay output to account for every non-clean
  Critical/Important finding with the settled marker vocabulary.
- Remove default-path budget, adjudication, and capacity/root-cause wording
  that implies a second review is expected. `lead-only` remains a no-delegate
  path and must not gain relay vocabulary.
- Add the Critical-specific branch: one repair relay followed by a
  Critical-scoped review #2 when review #1 contains any Critical finding.
- Make a remaining Critical finding a hard stop/escalation outcome, not a third
  relay. Preserve the findings and disposition paths in that outcome.
- Do not use the prior `fixed -> unresolved`, repeated-root-cause, or churn
  detectors as generic review #2 triggers. Evaluate whether any existing
  elevated/adjudication helper has a narrowly justified role inside this
  Critical-only branch; remove unreachable default behavior rather than leaving
  contradictory instructions.

This phase is atomic: do not land the default one-relay path before the
Critical-only branch exists, because that would remove the current Critical
verification protection in an intermediate release.

Verification: generated instructions and focused tests prove `single` and
`partitioned` issue exactly one ordinary relay after review #1, preserve the
`[won't fix]` marker, and do not expose re-review/adjudication instructions on
the ordinary path; `lead-only` remains free of relay terms. Tests also cover
no-Critical closeout after one relay; Critical causes exactly one scoped
re-review; an unresolved Critical does not schedule another relay; and the
generated instruction gives the lead an explicit hard-stop handoff with durable
evidence paths.

## Spec Impact

Target: the implementation-review contract in
`ai-docs/spec/workflow-skills.md` (`{#260612-reviewer-allocation-tier-default}`)
and the `lead-implement` behavior area. Expected caller-visible change:
per-slice review uses one repair relay by default, retains explicit disposition
markers including `[won't fix]`, and reserves a second review for an initial
Critical finding; it no longer advertises a generic two- or three-review-cycle
budget.
