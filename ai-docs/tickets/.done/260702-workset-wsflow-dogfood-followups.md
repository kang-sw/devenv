---
title: wsflow v0.31.1 dogfood-feedback follow-ups
sage-review: required
completed: 2026-07-02
---

# wsflow v0.31.1 dogfood-feedback follow-ups

## Context

This workset groups the 8 dogfood-feedback tickets captured after a full
wsflow v0.31.1 dogfood pass, for coordinated sequencing. This is a
non-hierarchical operating-context collection, not decomposition — none of
the 8 tickets receive a `parent:` link because of this workset's inclusion.

## Tickets

Suggested implementation order, each with a one-line rationale. All entries
resolved on integration branch `workset/wsflow-dogfood-followups` (base
`main` @ `c9d65eca`); tests green after every merge.

1. `260702-bug-config-unset-asymmetry` (done, `21408323`/`0fb0c4e7`) -
   foundational config semantic fix; no dependents block on it, but touches a
   primitive several other config-adjacent tickets reference conceptually.
2. `260702-bug-lead-manual-sections-thin` (done, `5fad1751`/`951a1cc5`) - doc
   fix; establishes the ferrule-style discipline pattern that ticket 7
   (`260702-feat-lead-revive-session-key-candidates`) had cross-referenced
   before it was dropped.
3. `260702-feat-agenda-enumerate-and-clear-all` (done, `f765a723`/`d3cf2781`)
   - small, independent.
4. `260702-feat-tickets-move-ready-gate-warning` (done, `72344ee6`) - small,
   independent.
5. `260702-feat-enter-implement-policy-feedback` (done, `dfa78a3b`) - small,
   independent.
6. `260702-feat-workflow-manual-state-only-view` (done, `73e83c5e`) - new
   lead-only tool surface, moderate scope, no hard dependency but naturally
   follows the smaller items; re-authored with a phase breakdown and
   verification criteria, sage review completed.
7. `260702-feat-lead-revive-session-key-candidates` - dropped — superseded by
   existing `workflow_manual` session-key-preservation reminder (see ticket
   `.dropped/` resolution); no longer part of this workset's implementation
   sequence.
8. `260702-research-tool-sprawl-session-key-overhead` (done, `1561ca5c`) -
   closed with findings: both investigated risks (tool-deferral bundling,
   session-scoped default key) are inherent to the MCP protocol/harness or
   would revert a deliberate correctness fix; no follow-up ticket created.

## Focus

This is the active follow-up set from the v0.31.1 dogfood pass. Sequencing
here is only a suggestion, not a hard gate — any ticket in `ready/` can be
proceeded independently.

## Exit Criteria

- Done: all 7 actionable tickets are shipped or explicitly dropped, and the
  research ticket has either produced a follow-up actionable ticket or been
  closed with findings. **Met** — 6 shipped, 1 dropped, research closed with
  findings, all on integration branch `workset/wsflow-dogfood-followups`
  pending merge approval into `main`.
- Deferred: none currently identified.
