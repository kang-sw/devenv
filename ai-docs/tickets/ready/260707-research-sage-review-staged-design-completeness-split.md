---
title: "Split sage review into a staged design/completeness gate across todo and ready"
sage-review: completed
spec:
  - mcp-tools
related:
  260707-feat-forge-autonomy-bootstrap-chaining: prior sage-review-gate precedent this design references
---

# Split sage review into a staged design/completeness gate across todo and ready

## Background

Raised mid-session while draining the ready queue (`260703-chore-bootstrap-staleness-alarm`
implementation dispatch), as a workflow-process agenda item, not yet discussed
or designed. Captured verbatim from the user's framing so the next session
can pick it up without re-deriving intent:

The current Sage Review Gate (spec: `ai-docs/spec/mcp-tools.md#260624-sage-review-gate`,
prior ticket precedent e.g. `260707-feat-forge-autonomy-bootstrap-chaining`)
already dispatches two distinct reviewer roles — `ticket-reviewer-design` and
`ticket-reviewer-completeness` — aggregated into one `pass`/`concern`/`block`
verdict under a single shared `sage-review:` posture field. Both reviewers
already run together at both `todo`-write and `ready`-promotion time today;
the actual gap this ticket targets is not introducing a design/completeness
split (it already exists as two reviewer roles), but adding **per-stage
gating** — running only the design reviewer at `todo` and only the
completeness reviewer at `ready` promotion, instead of both together at
every write. The user wants to explore splitting the *gating*, not the
reviewer roles, into two stages keyed to ticket status:

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

Also motivated by the user's `idea`/`todo`/`ready` lifecycle intent: `idea`
is the long-term backlog, `todo` is "a realizable idea" — design fit matters
there, implementation-readiness does not yet. The user observed a real
symptom of today's single-pass design: completeness-style pressure leaks
into the `todo` stage, creating a tendency to prematurely fill in details a
sketch-level ticket shouldn't need yet.

## Decisions

- **Two sequential single-pass gates, not a stateful loop.** No reset
  mechanism, no cyclic design/completeness re-triggering. The two reviews
  map directly to ticket lifecycle transitions:
  - At `todo`: a design-sketch review only, tolerant of missing detail —
    checks direction/fit, not implementation readiness. Meant to catch a
    wrong direction early without demanding ready-level polish.
  - At `ready` promotion: a completeness review checking whether the
    ticket is actually fit to hand to `lead-implement` — missing
    verification criteria, undecided user-policy points, capture gaps.
  An earlier revision of this ticket proposed a reset loop (completeness
  fix judged architecture-shaped -> design status resets -> re-review) plus
  a second frontmatter track and a shared re-review judge in
  `lead-write-ticket`. Dropped mid-discussion: the loop's cycle-breaking
  gate ("is this completeness fix architecture-shaped?") would either
  need a loose bias (which tends to never fire, defeating the loop's
  purpose) or fire unpredictably; and the shared judge conflated two
  different trigger contexts (a human ticket edit vs. a reviewer-driven
  fix).
- **Completeness reviewer reuses the `lead-check-blockers` framing as its
  rubric**, rather than a new reviewer layer. `lead-check-blockers`
  (`agents-plugin/rsrc/lead-check-blockers/lead-check-blockers.md`)
  already separates user-blocking design questions from autonomous
  hygiene/capture gaps in exactly the shape completeness review needs —
  no new concept required, only wiring that framing into the existing
  completeness-reviewer prompt. A dedicated "readiness reviewer" role was
  considered and rejected: no evidence yet that completeness reviewer's
  existing scope can't carry this, and a new reviewer type adds a
  standing per-ticket review cost (mental-model load + dispatch) for an
  unproven need.
- **Scope-boundary check replaces the reset loop as the drift guard.**
  Filling a completeness gap and quietly drifting the design (e.g. an
  unrelated interface change slipped in as a "completeness fix") is a
  real residual risk under a one-shot model — there is no cyclic
  detection to catch it after the fact. Rather than tolerate that risk
  unmitigated, the completeness reviewer's rubric gets an explicit scope
  check: distinguish a genuine completeness/readiness gap (fill it,
  `resolution: autonomous`-eligible) from a design-shaped gap in disguise
  (new public interface, cross-module interaction change, architecture
  reshaping) — the latter must be raised as a blocking finding
  (`resolution: missing`) and left unfilled, not patched in under cover
  of a completeness fix. This is a preventive check, not a corrective
  loop: it stops drift at the point of temptation rather than detecting
  it afterward. The residual risk this doesn't cover — the reviewer
  misjudging a borderline case — is accepted as an irreducible risk any
  review design carries, human or agent.
- **Frontmatter splits per stage**, since a ticket can now be in
  "design done, completeness not yet run" as a distinct, meaningful state
  under the two-gate (non-looping) model: two stage-scoped fields replace
  the single `sage-review:` field — e.g. `sage-review-design:` /
  `sage-review-completeness:` (exact key names/values deferred to
  implementation).
- **Category exemptions.** `epic` tickets require the design-review stage
  (architecture direction still needs an early sanity check) but are
  exempt from completeness (epics don't reach `lead-implement` directly).
  `research` and `workset` tickets stay exempt from both stages, matching
  today's exemption from the contract-first spec gate.
- **Design review is never skippable by entry path — hard invariant.**
  The `sage-review-design:` -> `sage-review-completeness:` ordering is
  enforced regardless of how a ticket reaches `ready`: a ticket jumping
  directly from `idea` to `ready`, or authored directly at `ready` status
  without ever passing through `todo`, must still pass the design-review
  gate before (or as part of) completeness review — completeness review
  must refuse to run, or must trigger design review first, if
  `sage-review-design:` has never completed. No entry path is allowed to
  produce a `ready` ticket whose design was never reviewed.

## Deferred to Implementation

- Exact `sage-review-design:`/`sage-review-completeness:` value
  vocabulary.
- Exact wording of the completeness-reviewer scope-boundary check (what
  counts as "design-shaped" vs. a genuine completeness gap) — should be
  read against the existing completeness-reviewer prompt first, since it
  may already partially cover this distinction.
- Whether `lead-write-ticket` needs any change at all under this
  simplified model, or whether both gates are fully owned by existing
  sage-review dispatch points (todo-write time, ready-promotion time)
  with no new judge table.
- Exact mechanism for enforcing the never-skippable design-review
  invariant on non-`todo` entry paths (`idea` -> `ready` direct promotion,
  or a ticket authored directly at `ready`) — likely: the ready-promotion
  check reads `sage-review-design:`, and if absent/incomplete, runs design
  review first (or blocks) before completeness review is allowed to run
  at all, rather than assuming design review already happened because the
  ticket reached `ready`.
- Migration plan for existing tickets that already carry the single
  `sage-review:` field (live and load-bearing today via native
  `tickets.move`/`tickets.create` posture stamping, per
  `ai-docs/spec/mcp-tools.md#260624-sage-review-gate` — this ticket's own
  frontmatter is one such case). Options to weigh at implementation time:
  bulk-migrate existing values onto the new split fields (e.g. treat an
  existing `completed`/`skipped` single-field value as satisfying both new
  fields), leave old tickets on the legacy single field as a grandfathered
  posture, or force a fresh design review pass. Not addressed here because
  it depends on the exact value vocabulary chosen for the split fields.

## Status

Design direction converged with the user in discussion (2026-07-07),
including one mid-discussion revision (dropped an initial reset-loop
proposal for the simpler two-gate model above, after comparing it against
`lead-check-blockers`'s existing blocking/non-blocking framing). Not yet
phased into an actionable ticket, and no empirical validation done yet
against a real sage-review case.

Sage review ran 2026-07-07: design verdict `concern` (2 important,
`resolution: autonomous` — missing `spec:` link, missing migration plan for
the legacy single `sage-review:` field), completeness verdict `concern` (1
important, `resolution: autonomous` — missing `related:` link). No
`resolution: missing` issues on either side, so aggregation resolves to
`pass`; all three findings applied autonomously (frontmatter `spec:`/
`related:` added, Background corrected to state the reviewer roles already
exist and only per-stage gating is new, migration-plan gap added to
Deferred to Implementation).
