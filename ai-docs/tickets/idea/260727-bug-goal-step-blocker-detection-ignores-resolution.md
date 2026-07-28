---
title: "goal-step blocker detection skips tickets whose blocker was already resolved"
related:
  260726-refactor-retire-spec-planned-marker-mechanism: the ticket this was observed on; it is unblocked and in ready/ but stays invisible to the goal loop
---

# goal-step blocker detection skips tickets whose blocker was already resolved

## Background

`lead-goal-step` instructs its selection subagent to "check its body for a
recorded blocker note (e.g. a `## Blocked (...)` entry) and skip blocked
candidates". The instruction matches on the *presence* of a blocker heading, not
on whether that blocker is still live.

Observed during a goal run on 2026-07-27. `260726-refactor-retire-spec-planned-marker-mechanism`
sits in `ready/` and is genuinely advanceable: its two blockers
(`## Blocked (2026-07-26, round 1)` and `## Blocked (2026-07-26, round 2)`) were
both cleared by a later `## Reviewed (round 3)` pass, and the ticket was promoted
to `ready/` on that basis (`df8da886 docs(tickets): clear marker-retirement sage
block and promote to ready`). The selection subagent skipped it anyway, reporting
the superseded headings, and correctly noted the supersession while still
following the instruction as written.

The consequence is not a one-off miss. Because a `## Blocked` heading is a
durable body record and nothing removes it on resolution, an unblocked ticket
that was ever blocked becomes **permanently unselectable** by the goal loop. It
never surfaces as a candidate, and it does not reach the "every remaining ticket
is blocked" conclusion either while other candidates remain, so the loop can
report progress while silently excluding real work.

Note the tension with the opposite failure the skill already guards against:
`lead-goal-step` requires recording a blocker before yielding precisely so the
next turn does not re-pick a stuck ticket. Blocker records must therefore stay
durable — the fix cannot be "delete the heading on resolution" without
re-opening that hole.

## Open questions

- Is the resolution signal a distinct heading (`## Reviewed`, `## Unblocked`), a
  date comparison against the newest blocker, or an explicit frontmatter field?
  The `260726` case resolved via `## Reviewed (round 3)`, but that heading is a
  sage-review convention, not a general unblock marker.
- Should `ready/` promotion itself count as the unblock signal? A ticket does not
  reach `ready/` while a live blocker stands, so status may already carry the
  answer and make body parsing unnecessary for `ready/` candidates.
- Does this belong in the skill's selection instruction, in ticket conventions
  (a defined resolution marker), or in a `tickets.*` MCP predicate so both the
  goal loop and any other consumer share one definition?

## Impact

Discovered by dogfooding, not by a reported failure. One live instance today.
Severity scales with how many tickets accumulate historical blocker records, and
it fails quietly — the skipped ticket appears only in the selection subagent's
skip list, which the lead does not surface to the user by default.
