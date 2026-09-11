---
title: "Batch promotion has no built-in cross-ticket coherence review: sibling tickets landing together are reviewed in isolation"
related:
  260911-research-golden-fixture-verification-gap: sibling; both are gaps in the ready-promotion verification path surfaced by the same dogfood arc
  260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming: trigger; deriving three children that all edit lead-run exposed the gap
---

# Batch-promotion cross-ticket coherence gap

## Background

`lead-ticket`'s **Promote to ready** step promotes a batch of related tickets
(prerequisites first), but the Sage machinery is per-ticket: `tickets.sage_gate`
takes one stem, spawns design/completeness reviewers over one ticket path plus the
populator's `relations:` table, and `tickets.sage_stamp` records one ticket's
verdicts. Nothing in the path reviews the batch *as a set* for mutual
contradiction or overlap.

Surfaced concretely while deriving three children from the impl-lifecycle research
ticket: all three edit the same flagship playbook (`lead-run.md`) — one adds a
merge step to report handling, one replaces the Select block, one restores goal
staging prose. Per-ticket parallel design review cannot, by construction, catch a
same-file collision or a contradictory decision between two siblings that have not
both landed yet.

## The gap

A batch of tickets promoted together can encode decisions that conflict, duplicate,
or edit the same region of the same shipped file, and the promotion pipeline has no
step that reviews the batch as a whole. Each reviewer sees one ticket; the relations
table names siblings but does not hand over their bodies or force a coherence check.

## Directions to weigh

- **Design review once over the batch.** Run a single design reviewer over all
  tickets in a promotion batch, returning a per-ticket verdict plus a cross-cutting
  coherence section; keep completeness per-ticket (it is inherently single-ticket).
  The lead stamps each ticket's design verdict from the batch reviewer's per-ticket
  output. (This is the ad-hoc approach taken for the trigger batch.)
- **Batch-aware per-ticket reviewers.** Hand each design reviewer the sibling
  ticket paths and instruct it to flag contradiction/overlap. Cheaper to wire into
  the existing one-reviewer-per-ticket spawn, but yields N partial views the lead
  must reconcile rather than one coherence authority.
- **A dedicated coherence pass.** A distinct lightweight reviewer (or a lead
  self-reconciliation step before the batch commit) scoped only to cross-ticket
  conflict, leaving per-ticket design/completeness unchanged.
- **Scope check.** Is this specific to a multi-ticket batch that shares an edit
  surface, or should any promotion of >1 ticket trigger it? Weigh the cost of an
  extra reviewer pass against how often batches share a surface.

Land with a concrete recommendation the follow-up implementation ticket can
execute, including whether the fix belongs in `lead-ticket`'s promote orchestration
(lead-time), the Sage gate/stamp machinery (tool-time), or a new reviewer surface.
