---
title: Sage ready-gate enforcement is duplicated across mutation primitives and the commit gate
parent: 260723-epic-ticket-write-reshape
related:
  260622-feat-sage-review-ticket-gate: introduced the ready-landing sage gate and the tickets_mutate.go posture checks this ticket relocates
  260721-bug-lead-write-ticket-sage-ready-ordering: absorbed and dropped; supplied the move-before-gate observation and the partial-mutation evidence
  260713-bug-tickets-move-error-mutates-frontmatter: closed done; added the loud partial-mutation notice that de-blocking makes largely moot
  260626-bug-sage-review-config-setter-missing: the `sage_review` config knob this ticket names as the only legitimate escape still has no setter
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-27
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
2. **`create_empty(initial_state: "ready")` is unreachable by default.** `judge:
   initial-status` offers `ready/` as a valid initial status and step 3.1 passes
   it straight to `create_empty`, but the tool rejects it whenever sage posture
   resolves to anything but `skipped`. This is not an opt-in edge case:
   `builtinConfigDefaults()` ships `sage_review: "auto"` (`server.go:466`) and
   `ResolvedSageReviewPosture` maps `auto` → `required`
   (`tickets_mutate.go:241-250`), so **every project hits the dead branch out of
   the box**. (`ResolvedSageReviewPosture`'s `default: "skipped"` branch is the
   fallback for an unrecognized config *string*, not the shipped default — an
   easy misread, and this repository's own tickets carry
   `sage-review-design: required` as confirmation.) Phase 1 test fixtures must
   use the real default or they will never exercise this path.
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
- **The warning must state the consequence and a *reachable* escape.** Shouting
  alone reproduces the dead end. So does naming an escape no tool can perform:
  there is no `sage_review` setter today (`260626-bug-sage-review-config-setter-missing`),
  so the message must name the resolving call and the concrete config surface,
  not an abstract "change the config". Target shape:

  ```text
  sage-review-design: required — moved anyway.
  ws/git.commit will FAIL on guardrail `ready-sage-posture` until design
  review completes. Resolve it with:
    ws/tickets.sage_gate(stem, landing: "ready")
  This posture is non-waivable per ticket by design. Changing it means
  changing the `sage_review` config item; ws/config.show reports the
  resolved config path.
  What design review checks: design coherence, whether this is the right
  problem, and whether an implementer can execute without filling in
  design gaps — not whether the underlying research is settled.
  ```

- **The message must ride the path an agent actually takes.** Putting it only on
  a `sage_gate(answer: "no")` refusal is dead text: at posture `required` the
  gate never asks, and `answer` is schema-documented as "Optional follow-up
  answer to a prior ask action" (`server.go:4130`), so no agent has a reason to
  send it. The non-waivable statement and the review-scope line belong on the
  ordinary `required` → `run` gate result and on the move/create warning.
- **`blocked` de-blocks at mutation time too, with its own message.**
  `prepareSageReviewForUpwardMove` rejects through two paths —
  `sageReviewStageError` and `sageReviewBlockedError` (`tickets_mutate.go:328`)
  — and `readyPostureProblems` treats `blocked` as a distinct case. Both stop
  rejecting; verify still hard-fails on `blocked`. Its warning says a prior
  review found unresolved issues, which is a different instruction from "review
  has not run".

- **The last line is load-bearing.** Downstream, the owner waived review because
  "we've already verified this enough", meaning the *research*. That is not what
  either reviewer assesses: design checks coherence, right-problem, and
  implementer-executability against the specs and mental models the ticket links;
  completeness checks structure, fields, and fresh-reader clarity from the ticket
  file alone. Both are disjoint from "is the research settled". Once that was
  named, the owner reversed the waiver. Surfacing review scope is the cheapest
  high-value change in this ticket.
- **`ready/`'s guarantee narrows, and that is accepted.** Today "in `ready/`"
  is filesystem-enforced to mean "passed sage". After this change it means
  "passed sage **if the landing was committed through `ws/git.commit`**". The
  owner was asked directly and accepted. Note the window is wider than a
  `git.commit` bypass: `tickets.list` and `project_tree` scan the working tree,
  so the ordinary interval between a successful `tickets.move` and its commit is
  itself an unreviewed-ticket-visible-in-`ready/` state, and `ready/` is
  `lead-goal-step`'s sole progress gate. The consumption-side mitigation is
  `260726-feat-proceed-sage-posture-consumption-guard`; land it with or before
  this ticket.
- **`judge: initial-status` keeps `ready/`.** De-blocking `create_empty` makes
  the branch reachable, so defect 2 dissolves rather than needing the status
  removed from the judge.
- **This revises a recorded epic decision, already recorded.** `260723`'s
  `## Cross-Child Decisions` said "Existing hard/soft choices are the seed
  classification. ready→sage is already hard (`tickets_mutate.go`)". This ticket
  removes exactly that hardness. The hard/soft *axis* survives untouched; only
  the enforcement *location* moves, which is what the epic's own "verify =
  mechanical floor, sage = semantic ceiling" decision already implies. The epic
  bullet was revised in commit `ae1b7dff` alongside this ticket's creation, so
  implementation only needs to confirm the recorded revision still matches
  shipped behavior at close.

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
  notice describes. Phase 1 owns the disposition: delete the notice if it becomes
  unreachable, or keep it and state which remaining rejection path still reaches
  it. This is an acceptance item, not an open question.

## Spec Impact

- Target spec area: `ai-docs/spec/mcp-tools.md`, the `tickets.move` /
  `tickets.create_empty` sections.
- Expected caller-visible change: both tools stop failing on unresolved ready
  sage posture; they succeed and return a warning naming the commit-time
  consequence and the resolving `sage_gate` call. `ws/git.commit` /
  `tickets.verify` behavior is unchanged and becomes the documented single
  enforcement point. `tickets.sage_gate` gains the non-waivable statement and the
  review-scope line on its `required` → `run` result.
- Contract-first spec: **no**. Owner decision, reversing this ticket's initial
  `yes`: the exact warning strings, the fact/return shape, and whether the
  review-scope line is one field or prose all settle during implementation, and
  planned spec text would mostly restate the phase. The behavioral direction —
  mutation permissive, commit gate blocking — is already fully stated here, so
  `## Spec Impact` is sufficient addressing. Post-implementation closeout updates
  `mcp-tools.md` with the shipped contract.

## Phases

### Phase 1: Relocate ready-sage enforcement to the commit gate

Make the `ready/` landing path satisfiable by moving to a single enforcement
point, in one reviewable slice:

- Remove both ready sage-posture rejections from `tickets.move` and
  `tickets.create_empty` — the `sageReviewStageError` path *and* the
  `sageReviewBlockedError` path; return the decided warning, with distinct text
  for unreviewed vs `blocked`.
- **Renumber `lead-write-ticket` unconditionally**: Sage Review Gate becomes step
  5 and Commit step 6. Not a conditional ordering inside the numbered list —
  the gate is a no-op for non-`ready/` landings anyway (`sage_gate` returns
  `skip` for `idea`, and for exempt categories), and step 6.3's existing wording
  ("carrying the posture change together with any other uncommitted edits already
  held on the ticket") already reads as if the gate preceded the commit.
- Put the non-waivable statement and the review-scope line on the ordinary
  `required` → `run` gate result and on the move/create warning — not only on an
  `answer: "no"` refusal, which no agent has reason to send at `required`. Keep
  them on the `recommended` ask prompt as well.
- Resolve the `260713` partial-mutation notice per `## Prior Art`.
- Confirm `260723`'s already-revised seed-classification bullet still matches
  shipped behavior; no fresh epic edit is needed.

Single phase on purpose. The deliverables are heterogeneous (Go mutation change,
gate message, playbook renumber) but not sequentially dependent, and shipping any
subset leaves the `ready/` path still unsatisfiable — a partial landing has no
reviewable value. Rejected: splitting per surface.

Other rejected alternatives: adding a `waive` action or `waived_by_owner` verdict
(collapses `required` into `recommended`); keeping the block and only improving
the error text (leaves the unsatisfiable ordering and the partial-mutation state
intact); removing `ready/` from `judge: initial-status` (treats the symptom).

Verification boundary — all of the following, at the shipped default
`sage_review: auto` (fixtures must not use `skipped`):

1. `tickets.create_empty(initial_state: "ready")` succeeds and returns the
   warning instead of failing.
2. `tickets.move(to: "ready")` succeeds from both an unreviewed posture and a
   `blocked` posture, each returning its own warning text.
3. `ws/git.commit` on either resulting ticket still fails on guardrail
   `ready-sage-posture`.
4. `tickets.sage_gate` at posture `required` returns the non-waivable statement,
   the review-scope line, and the resolving call on its ordinary `run` result.
5. A `todo/` → `ready/` promotion completes end-to-end through the renumbered
   rendered playbook with no hand edit.

### Result (e4df433c) - 2026-07-27

`tickets.move` and `tickets.create_empty` no longer reject a `ready/` landing on
sage posture; they persist the resolved posture, succeed, and return a warning on
the existing `Tip` channel. `tickets.verify` / `ws/git.commit` is untouched and is
now the single hard enforcement point. `tickets.sage_gate` gained an `Advisory`
carrying the non-waivable statement and the review-scope line on every `run`/`ask`
result. `lead-write-ticket` runs the Sage Review Gate at step 5 and Commit at
step 6.

Five things the phase decided that the ticket did not anticipate:

- **`tickets.sage_gate` lost its commit behavior entirely.** The reorder put the
  gate before the ticket was committed, so the ask-decline path's canonical
  `chore(sage): skip design review` commit began sweeping the whole authored
  ticket — through a nil-`Verifier` client that also bypassed the guardrail this
  ticket designates as the chokepoint. The first fix moved the caller but kept the
  payload and re-emitted it as `pending_commit_*` lines with a ready-to-paste
  `ws/git.commit(...)`, which reproduced both defects one hop out. The second fix
  deleted `CommitTitle`/`CommitPaths`/`AIContext` from `SageGateResult` and
  `mergeGateCommit` outright, following `260725`'s `tickets.sage_stamp` precedent
  literally. This is a caller-visible contract change, recorded in
  `{#260720-sage-gate-record-tools}`.
- **One rejection survives on purpose.** De-blocking removed the blocked check for
  *all* upward moves, but `tickets_verify.go` runs the guardrail only for
  `status == "ready"`, so outside a ready landing the removal moved enforcement to
  nowhere rather than to the chokepoint. A non-`ready` upward move with a blocked
  required stage hard-rejects again through `blockedUpwardMoveError`.
- **The two warning variants name different tools.** `sage_gate` returns
  `stop_blocked` before any resolution logic and cannot clear a blocked posture;
  only `sage_stamp` with a non-`block` verdict can. Sharing one instruction clause
  would have reproduced the "escape no tool can perform" the `## Decisions` forbid.
- **`create_empty(ready)` now stamps both required fields**, so the two ready
  landing entry points produce the same posture shape. Before, create stamped only
  design while `git.commit` failed on `sage-review-completeness: unset`.
- **A `block` verdict at a `ready/` landing became newly reachable** once the move
  stopped rejecting, and dead-ended with the ticket half-promoted. Step 5.3 now
  demotes and reports instead of falling through to step 6.

`260713` partial-mutation notice, disposed per `## Prior Art`: **deleted**. Its
sole producer was the posture-rejection return in `TicketsMove`, which is gone.
The underlying condition still exists — `prepareSageReviewForUpwardMove` persists
the self-healing write and `atomicGitMove` can then still fail, leaving the file
mutated — but that path returned an empty notice before this change too, so
deletion is not a regression. The `blockedUpwardMoveError` rejection also mutates
the file before returning; that write ordering predates this phase.

`260723`'s revised seed-classification bullet was confirmed read-only against
shipped behavior; it already matches and needed no edit.

Three partitioned review cycles, the full budget. Cycle 1 returned 1 Critical
(the renumber made steps 5 and 6 both commit, so step 6 failed with `no staged
changes` — the ticket's own verification item 5) and 5 Important. Cycle 2 returned
a new Critical because the cycle-1 `sage_gate` fix relocated its defect rather than
removing it; that cycle was relayed with an elevated root-cause posture. Cycle 3
was clean on fit and traced the two priority paths live against a real MCP server
build rather than by inference.

Carried forward unresolved, budget spent rather than dismissed:

- The regression guards for the deleted commit directive match exact strings, not
  the behavior class, so a reworded commit instruction would pass. Reported by the
  test partition and mutation-demonstrated. Captured as
  `260727-bug-string-match-guards-miss-reworded-directives`.
- `blocked`-at-`ready` has `wsdoc`-level coverage but no MCP-dispatch-level test,
  unlike its unreviewed sibling.
