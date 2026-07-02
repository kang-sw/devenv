---
title: wsflow v0.31.1 dogfood-feedback follow-ups
sage-review: required
---

# wsflow v0.31.1 dogfood-feedback follow-ups

## Context

This workset groups the 8 dogfood-feedback tickets captured after a full
wsflow v0.31.1 dogfood pass, for coordinated sequencing. This is a
non-hierarchical operating-context collection, not decomposition — none of
the 8 tickets receive a `parent:` link because of this workset's inclusion.

## Tickets

Suggested implementation order, each with a one-line rationale:

1. `260702-bug-config-unset-asymmetry` (ready) - foundational config semantic
   fix; no dependents block on it, but touches a primitive several other
   config-adjacent tickets reference conceptually.
2. `260702-bug-lead-manual-sections-thin` (ready) - doc fix; establishes the
   ferrule-style discipline pattern that ticket 7
   (`260702-feat-lead-revive-session-key-candidates`) explicitly
   depends on/cross-references.
3. `260702-feat-agenda-enumerate-and-clear-all` (ready) - small, independent.
4. `260702-feat-tickets-move-ready-gate-warning` (ready) - small, independent.
5. `260702-feat-enter-implement-policy-feedback` (ready) - small, independent.
6. `260702-feat-workflow-manual-state-only-view` (ready) - new lead-only tool
   surface, moderate scope, no hard dependency but naturally follows the
   smaller items; re-authored with a phase breakdown and verification
   criteria, sage review completed.
7. `260702-feat-lead-revive-session-key-candidates` - dropped — superseded by
   existing `workflow_manual` session-key-preservation reminder (see ticket
   `.dropped/` resolution); no longer part of this workset's implementation
   sequence.
8. `260702-research-tool-sprawl-session-key-overhead` (todo) - open
   investigation, `todo/` only, can run in parallel with any of the above;
   not implementation-ready.

## Focus

This is the active follow-up set from the v0.31.1 dogfood pass. Sequencing
here is only a suggestion, not a hard gate — any ticket in `ready/` can be
proceeded independently.

## Exit Criteria

- Done: all 7 actionable tickets are shipped or explicitly dropped, and the
  research ticket has either produced a follow-up actionable ticket or been
  closed with findings.
- Deferred: none currently identified.
