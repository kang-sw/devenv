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

## Decisions

- **Independent resolution tracks, gated in sequence.** Design and
  completeness keep their own `resolution:` aggregation semantics
  (`missing` → block-review vs. `autonomous` → pass-with-lead-fix)
  independently — completeness does not inherit unresolved design
  concerns. But sequencing is a hard gate: completeness review only runs
  once design status is `completed`; a ticket cannot enter completeness
  review while design is still open.
- **Reset loop, not a one-shot gate.** If a completeness-driven fix
  touches architecture or cross-module interaction, design status resets
  to required and a new design pass must complete before completeness can
  pass again. This can loop (design → completeness → reset → design →
  completeness → ...). To keep the loop converging in practice, the gate
  deciding "does this completeness fix count as architecture-changing"
  should be biased loose (default: no reset) — completeness-stage fixes
  are expected to be narrow by nature, so only a clearly cross-module or
  contract-shape change should trigger a reset.
- **One shared judge, cheap, lives in `lead-write-ticket`.** Both "does a
  post-design-review ticket edit need a design re-review" and "does a
  completeness-driven fix count as architecture-changing" collapse into
  the same judge table — a new 1-2 line judge in `lead-write-ticket`, not
  a full design-reviewer re-dispatch on every edit. Rationale: a full
  reviewer re-run costs a mental-model load and reviewer dispatch even
  when the answer is "no impact"; a cheap lead-owned judge avoids that
  cost and avoids growing `lead-write-ticket` into reviewer-dispatch
  logic. If the judge says yes, only the edited region needs to be
  relayed to the design reviewer as a delta re-check, not a full re-run.
- **Frontmatter splits per stage.** Replace the single `sage-review:`
  field with two stage-scoped fields (exact key names/values are an
  implementation-time detail, not settled here) — e.g.
  `sage-review-design: completed` / `sage-review-completeness:
  waiting-design` — so a ticket can be mid-loop (design done, completeness
  pending or reset-pending) without conflating the two states in one
  value.
- **Category exemptions.** `epic` tickets require the design-review stage
  (architecture direction still needs an early sanity check) but are
  exempt from completeness (epics don't reach `lead-implement` directly).
  `research` and `workset` tickets stay exempt from both stages, matching
  today's exemption from the contract-first spec gate.

## Deferred to Implementation

- Exact `sage-review-design:`/`sage-review-completeness:` value vocabulary
  (state names for pending/waiting-design/completed/reset, etc.).
- Exact wording and placement of the shared judge table in
  `lead-write-ticket`.
- Whether the "architecture-changing" judge needs example cases in its
  table to keep the loose bias consistent across sessions.

## Status

Design direction converged with the user in discussion (2026-07-07); not
yet phased into an actionable ticket. Sage review intentionally left at
`recommended` — still pending, since this ticket may be split into an
actionable follow-up before it needs a design/completeness pass itself.
