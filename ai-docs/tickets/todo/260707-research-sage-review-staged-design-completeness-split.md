---
title: "Split sage review into a staged design/completeness gate across todo and ready"
sage-review: recommended
---

# Split sage review into a staged design/completeness gate across todo and ready

## Background

Raised mid-session while draining the ready queue (`260703-chore-bootstrap-staleness-alarm`
implementation dispatch), as a workflow-process agenda item, not yet discussed
or designed. Captured verbatim from the user's framing so the next session
can pick it up without re-deriving intent:

The current Sage Review Gate (two-reviewer design + completeness verdict,
`pass`/`concern`/`block`, described in prior ticket precedent e.g.
`260707-feat-forge-autonomy-bootstrap-chaining`) runs as a single pass at
ticket-write time. The user wants to explore splitting it into two stages
gated by ticket status:

- At `todo`: only a design-level sketch review runs. This stage should
  tolerate a rougher, sketch-level design — it is meant to catch
  fundamentally wrong directions early, not demand ready-level polish.
- At `ready` promotion: a completeness-level review runs, checking things
  like whether a user policy decision is still needed, whether additional
  judgment/decisions are missing, and other before-implementation gaps —
  i.e. exactly the checks that matter once a ticket is about to be handed
  to `lead-implement`.
- When a ticket's design content is edited after its design review already
  passed, an agent should judge whether the edit is substantial enough to
  require re-running the design-level review, rather than always
  re-running it or never re-running it.

## Open Questions

- How does this interact with the existing `resolution: missing` /
  `resolution: autonomous` aggregation rule (concern → block-review vs.
  pass-with-lead-fix)? Does each stage keep its own resolution semantics,
  or does completeness inherit unresolved design concerns from todo?
- Where does the "does this edit need design re-review" judgment live —
  a new judge table in `lead-write-ticket`, or a check folded into the
  existing promotion-to-ready flow?
- Does this change the `sage-review:` frontmatter vocabulary (currently at
  least `completed`/`recommended`/`required`, per `tickets.create` tool
  output) to track per-stage status (e.g. `design: completed`,
  `completeness: pending`), or stay a single field?
- Should epic/research/workset tickets (which skip today's contract-first
  spec gate) be exempt from the design stage, the completeness stage, or
  neither?

## Status

Not yet discussed with the user beyond the initial framing above. Sage
review intentionally left at `recommended` (not run yet) — pick this up
together next session before deciding a design direction.
