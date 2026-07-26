---
title: Sage ready-gate enforcement is duplicated across mutation primitives and the commit gate
parent: 260723-epic-ticket-write-reshape
related:
  260622-feat-sage-review-ticket-gate: introduced the ready-landing sage gate and the tickets_mutate.go posture checks this ticket relocates
  260721-bug-lead-write-ticket-sage-ready-ordering: absorbed and dropped; supplied the move-before-gate observation and the partial-mutation evidence
  260713-bug-tickets-move-error-mutates-frontmatter: closed done; added the loud partial-mutation notice that de-blocking makes largely moot
  260626-bug-sage-review-config-setter-missing: the `sage_review` config knob this ticket names as the only legitimate escape still has no setter
sage-review-design: required
---

# Sage ready-gate enforcement is duplicated across mutation primitives and the commit gate

## Background

A downstream field report (wsflow 0.36.1, project InspectTGV) hit a hard stop
running `lead-write-ticket` end-to-end on a new `feat` ticket targeting `ready/`.
The findings were re-verified against source at 0.36.12; none were already fixed.

Sage posture for a `ready/` landing is enforced in two places:

- **Mutation primitives.** `tickets.move` and `tickets.create_empty` reject the
  landing outright via `sageReviewStageError` (`tickets_mutate.go:314-329`).
- **Commit gate.** `tickets.verify` carries the `ready-sage-posture` guardrail as
  a hard finding (`tickets_verify.go:112-121`), which `ws/git.commit` enforces.
  `tickets_verify.go:175` states this explicitly: the guardrail "stays HARD".

The duplication produces three concrete defects:

1. **The `ready/` path is unsatisfiable as written.** `lead-write-ticket` orders
   `5. Commit` before `6. Sage Review Gate`. Under `sage_review: ask|auto` the
   step-5 commit hits the hard `ready-sage-posture` guardrail before the step-6
   gate ever runs. Reordering is required regardless of anything else.
2. **`create_empty(initial_state: "ready")` is unreachable.** `judge:
   initial-status` offers `ready/` as a valid initial status and step 3.1 passes
   it straight to `create_empty`, but the tool rejects it whenever sage posture
   resolves to anything but `skipped`. The default (`skipped`) hides the dead
   branch; a project that opts in finds it immediately.
3. **The dead end pushes agents toward frontmatter tampering.** With posture
   `required`, `tickets.sage_gate` returns a bare `action: run` and ignores
   `answer`. The reporting agent correctly refused to fabricate verdicts through
   `sage_stamp` (its schema says "Record sage-review verdicts **after** reviewers
   ran"), leaving only: run the review the owner declined, hand-edit frontmatter,
   or mutate project config. It reported this as "no owner-waiver action exists".

That last framing is **inaccurate and must not be implemented as stated**. An
auditable waiver already exists: at posture `recommended`, `sage_gate(answer:
"no")` writes `sage-review-<stage>: skipped` and returns commit metadata
(`chore(sage): skip design review` / "user declined design review in ask mode",
`tickets_sage.go:189-201`). The gap is only that `required` ignores `answer`.

## Decisions

- **Single chokepoint at the commit gate.** `tickets.verify` /`ws/git.commit`
  remains the one HARD enforcement point. `tickets.move` and
  `create_empty` stop rejecting on ready sage posture and emit a loud warning
  instead. This is the owner's explicit direction: do not block at tool level.
- **No `waive` action, no `waived_by_owner` verdict.** `required` meaning
  "non-waivable per ticket" is the only thing distinguishing it from
  `recommended`; a per-ticket override collapses the two postures. The defect is
  that the gate never says so.
- **The warning must state the consequence and the legitimate escape.** Shouting
  alone reproduces the dead end. Target shape:

  ```text
  sage-review-design: required — moved anyway.
  ws/git.commit will FAIL on guardrail `ready-sage-posture` until design
  review completes. This posture is non-waivable per ticket by design;
  the only legitimate change is project config `sage_review`.
  What design review checks: ticket document recoverability, not whether
  the underlying research is settled.
  ```

- **The last line is load-bearing.** Downstream, the owner waived review because
  "we've already verified this enough", meaning the *research*. Sage reviewers
  assess *ticket document recoverability* — a disjoint concern. Once that was
  named, the owner reversed the waiver. Surfacing review scope is the
  cheapest high-value change in this ticket.
- **`ready/`'s guarantee narrows, and that is accepted.** Today "in `ready/`"
  is filesystem-enforced to mean "passed sage". After this change it means
  "passed sage **if the landing was committed through `ws/git.commit`**". The
  owner was asked directly and accepted: opening that window requires bypassing
  `ws/git.commit`, which is the already-known hole `260723` exists to close.
- **`judge: initial-status` keeps `ready/`.** De-blocking `create_empty` makes
  the branch reachable, so defect 2 dissolves rather than needing the status
  removed from the judge.
- **This revises a recorded epic decision.** `260723`'s `## Cross-Child
  Decisions` says "Existing hard/soft choices are the seed classification.
  ready→sage is already hard (`tickets_mutate.go`)". This ticket removes exactly
  that hardness. The hard/soft *axis* survives untouched; only the enforcement
  *location* moves, which is what the epic's own "verify = mechanical floor,
  sage = semantic ceiling" decision already implies. The epic body is revised in
  the same logical commit as the implementation.

## Constraints

- Do not weaken `tickets.verify`'s `ready-sage-posture` guardrail. It becomes the
  sole enforcement point and must stay a hard finding.
- Do not change reviewer criteria or the semantic sage judgment — out of scope
  per the parent epic's `## Non-Scope`.
- `260723-feat-ticket-write-verify-commit-gate` is already in `.done/`, so the
  commit-gate chokepoint this ticket relies on is live. No prerequisite work.

## Prior Art

- `260713-bug-tickets-move-error-mutates-frontmatter` (done) added a loud
  partial-mutation notice for the block path. De-blocking removes the state that
  notice describes; check whether the notice becomes dead code.

## Spec Impact

- Target spec area: `ai-docs/spec/mcp-tools.md`.
- Expected caller-visible change: `tickets.move` and `tickets.create_empty` no
  longer fail on unresolved ready sage posture; they succeed and return a warning
  naming the commit-time consequence. `ws/git.commit` / `tickets.verify` behavior
  is unchanged and becomes the documented single enforcement point.
- Contract-first spec: yes. Deferred to promotion time — this ticket lands in
  `todo/`, so the ready spec-address gate has not fired. The `🚧` entry and the
  `ready/` move must land in the same logical commit; see
  `260726-bug-spec-planned-marker-ready-ticket-cycle` for why that ordering is
  currently undefined.

## Phases

### Phase 1: Relocate ready-sage enforcement to the commit gate

Make the `ready/` landing path satisfiable by moving to a single enforcement
point, in one reviewable slice:

- Remove the ready sage-posture rejection from `tickets.move` and
  `tickets.create_empty`; return a warning carrying the decided message shape.
- Reorder `lead-write-ticket` so the Sage Review Gate precedes Commit for any
  `ready/` landing (including a requested `todo/` → `ready/` promotion).
- Make `tickets.sage_gate` fail loud at posture `required` when `answer: "no"`
  is supplied: state that the posture is non-waivable per ticket and name
  `sage_review` config as the only legitimate change.
- Add the review-scope line ("what this review checks") to the gate's ask prompt
  and to the `required` refusal.
- Revise `260723`'s seed-classification `## Cross-Child Decisions` bullet in the
  same commit: the axis stands, the enforcement location moves to verify.

Rejected alternatives: adding a `waive` action or `waived_by_owner` verdict
(collapses `required` into `recommended`); keeping the block and only improving
the error text (leaves the unsatisfiable ordering and the partial-mutation state
intact); removing `ready/` from `judge: initial-status` (treats the symptom).

Verification boundary: with `sage_review: auto`, a `todo/` → `ready/` promotion
completes end-to-end through the rendered playbook without a hand edit, and a
commit attempted before sage review still fails on `ready-sage-posture`.
