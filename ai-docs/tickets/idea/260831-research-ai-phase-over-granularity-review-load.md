---
title: AI over-granular phase splitting amplifies per-phase review load
related:
  260831-refactor-severity-graded-per-slice-review-relay: sibling lever — that ticket lightens per-phase review *weight*; this one investigates reducing the *count* of phases that each draw review
  260824-epic-review-watermark-model: the review-altitude epic whose sweep/gate is the coarser-boundary alternative to per-phase review
related-mental-model:
  - workflow-skills
---

# AI over-granular phase splitting amplifies per-phase review load

## Background

The dogfood pain that motivated `260831-refactor-severity-graded-per-slice-review-relay`
is a *product* of two factors: (1) each ticket phase draws per-phase review, and
(2) the AI tends to split a ticket into more, finer phases than a human would.
`260831` attacks factor (1) — the per-phase review *weight*. This ticket
captures factor (2): the phase *count* amplifier. Even after per-phase weight is
severity-graded, an over-split ticket multiplies whatever residual weight
remains across many slices.

This is a separate subsystem from the review loop: phase granularity is decided
in ticket authoring / planning (`lead-write-ticket`'s `judge: ticket-shape`,
and whatever planning heuristics drive phase decomposition), not in the
`enter.implement` review path. Kept as a distinct follow-up rather than folded
into `260831` to avoid perturbing two subsystems in one change.

## Problem Statement

- Observed tendency: AI decomposes a single reviewable unit into several
  `### Phase N` sections where a coarser split (often one phase) would be a
  cleaner reviewable slice.
- `judge: ticket-shape` already states "Phase default: actionable tickets use
  one `Phase 1`" and "Phase split: add phases only for sequentially dependent
  units; differing review, verification, or rollback boundaries alone do not
  justify a phase split." So the *convention* is already conservative — the open
  question is why authoring drifts more granular than the convention, and
  whether the lever is stronger authoring-time enforcement or a coarser review
  boundary.

## Candidate Levers (unsettled)

- **Tighten phase-shape enforcement at authoring time.** Make
  `judge: ticket-shape` (or a verify-phase check in `lead-write-ticket`)
  actively push back on phases that are not sequentially dependent, collapsing
  them toward one slice. Risk: over-collapsing genuinely separable work.
- **Batch review at a coarser boundary instead of per-phase.** This is
  effectively what epic `260824`'s sweep/gate already does at integration time —
  so the question is whether per-phase review should shrink further (relying more
  on the integration net) rather than trying to reduce phase count. Interacts
  directly with `260824`'s "keep per-phase light" thesis and its under-review
  risk framing.
- **Do nothing structural; rely on `260831` alone.** If severity-graded weight
  makes typical phases cheap enough, phase count may stop mattering in practice.
  Test this first before investing in a phase-count lever.

## Open Questions

- Is the drift a planning-model behavior (how implementation plans propose
  phases) or a ticket-authoring behavior (how `lead-write-ticket` records them),
  or both?
- After `260831` lands, does phase count still produce felt load, or is the
  amplifier neutralized by cheaper per-phase weight? (Measure before acting.)
- If a lever is warranted, is it authoring-time collapse or deferring more to
  the `260824` integration net — and are those mutually exclusive?
