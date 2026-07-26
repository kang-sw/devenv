---
title: enter.proceed does not check sage posture, so an unreviewed ready ticket can be drained
related:
  260726-bug-sage-ready-enforcement-single-chokepoint: opens the window this guard closes; that ticket relocates write-path enforcement to the commit gate
  260722-feat-goal-run-autonomy-posture: an autonomous goal run meeting an unreviewed ready ticket is a posture question owned there, not here
  260723-epic-ticket-write-reshape: adjacent enforcement-placement reshape, but that epic owns the write path and this is the consumption path
sage-review-design: required
---

# enter.proceed does not check sage posture, so an unreviewed ready ticket can be drained

## Background

Raised by the owner while reviewing `260726-bug-sage-ready-enforcement-single-chokepoint`:
a ticket sitting in `ready/` without a completed sage review carries that fact in
its own frontmatter, so `enter.proceed` has everything it needs to catch the case
and route to review — but does not look.

Verified: `proceed_resolver.go` contains no sage handling at all. (Grepping the
file for "sage" matches only substrings of `message` and `warnings`.) The
resolver already carries a `Warnings []string` channel and a
`facts.ticket.{status, category, freshness}` model, so the hook point exists and
is simply unused.

**This is a prerequisite for the single-chokepoint change, not defence in depth.**
The initial risk framing — "the window only opens if someone bypasses
`ws/git.commit`" — understates it. `tickets.list` and `project_tree` scan the
working tree, not git. Once
`260726-bug-sage-ready-enforcement-single-chokepoint` lets `tickets.move`
succeed before review, the interval between a successful move and its commit *is*
a state where an unreviewed ticket is visible in `ready/`. That is not a bypass
artifact; it is the normal mid-procedure state of every promotion. `ready/` is
also `lead-goal-step`'s sole progress gate, so an autonomous run can select such a
ticket in that interval.

## Decisions

- **Consumption path, not write path — no conflict with the single-chokepoint
  decision.** That decision forbids enforcing the same invariant at both the
  mutation primitive and the commit gate, because reaching the commit
  *necessarily* passes through the mutation primitive, so double enforcement
  makes the ordering unsatisfiable. Proceeding is not a prerequisite for
  committing, so no cycle can form on this axis.
- **Route, do not block.** A hard stop here recreates exactly the dead end that
  drove the downstream agent toward frontmatter tampering. `enter.proceed`
  should report the posture and name the call that resolves it.
- **`blocked` is not the same as unreviewed.** A `blocked` posture means a prior
  review found unresolved issues; proceeding on it is a stronger error than
  proceeding on a ticket whose review simply has not run. The two deserve
  different output, and `blocked` is the candidate for an actual stop.

## Constraints

- Do not duplicate the ready-landing guardrail semantics. This guard answers "is
  this ticket safe to *start*", not "is this ticket allowed to *be* in ready".
- Autonomy behavior (does an autonomous goal run stop, ask, or run sage itself)
  is out of scope; it belongs to `260722-feat-goal-run-autonomy-posture`. This
  ticket only makes the fact available and the route explicit.

## Prior Art

- `proceed_resolver.go`'s existing `warnFactIfMeaningful` / `warnIfPresent`
  helpers and its `Warnings` output section are the established pattern to
  extend; do not invent a second warning channel.

## Spec Impact

- Target spec area: `ai-docs/spec/mcp-tools.md`, `enter.proceed` section.
- Expected caller-visible change: `enter.proceed` resolves the target ticket's
  sage posture and surfaces it — a routing warning naming the sage gate call when
  review has not run, and distinct output for `blocked`.
- Contract-first spec: no. Whether `blocked` stops or warns, and the exact fact
  shape, should be settled during implementation against the existing resolver
  fact/warning model.

## Phases

### Phase 1: Surface sage posture at proceed time

- Read the target ticket's `sage-review-design` / `sage-review-completeness`
  posture in `proceed_resolver.go` and expose it through the existing fact model.
- Emit a routing warning when a `ready/` target's posture is neither `completed`
  nor `skipped`, naming `ws/tickets.sage_gate(stem, landing: "ready")` as the
  resolving call.
- Give `blocked` its own output, and decide there whether it stops or warns.
- Confirm `lead-goal-step` and `lead-goal-fan-out-step` inherit the guard by
  routing through proceed rather than needing their own copy.

Rejected alternatives: hard-blocking at proceed (recreates the dead end);
re-checking the invariant in each goal-loop skill (duplicates the guard across
callers instead of hosting it where they already converge); leaving the window
open on the grounds that it requires a `git.commit` bypass (false — the
move-to-commit interval is a normal state).

Verification boundary: with a `ready/` ticket whose sage posture is `required`,
`enter.proceed` reports the posture and the resolving call rather than routing
straight to implementation, and a goal-loop step over the same ticket inherits
that behavior without its own check.
