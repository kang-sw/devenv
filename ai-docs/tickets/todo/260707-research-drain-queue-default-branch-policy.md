---
title: "Default branch policy to reuse+rename during goal-driven ready-queue drains"
sage-review: recommended
---

# Default branch policy to reuse+rename during goal-driven ready-queue drains

## Background

Raised mid-session while running `ws:lead-drain-ready-queue` end-to-end for
`260703-chore-bootstrap-staleness-alarm`, then again while running
`lead-proceed`/`lead-implement` on `260707-research-sage-review-staged-design-completeness-split`.
Captured verbatim from the user's framing so a later session can pick it up
without re-deriving intent:

When a ticket is picked up via `ws:lead-drain-ready-queue`, the user typically
pushes it through together with an explicit goal for the whole run (i.e. the
full survey -> implement -> review -> doc -> merge pipeline is meant to run
without pausing at each step). In this mode, the user wants the default
branch policy to be **reuse+rename** rather than the current default of
creating a fresh `implement/<slug>` branch per ticket and separately asking
for merge confirmation at the end. The user's stated complaint: today's flow
"자꾸 머지 물어봐서 귀찮네요" (it keeps asking about merge, which gets
tedious) when the intent to drive the ticket through to a merged state was
already given up front as the goal.

Not yet discussed or designed beyond this framing. Open questions for a
future session include (non-exhaustive, not yet decided):

- What exactly "reuse+rename" means here — reusing the current branch if it
  is already an `implement/*` branch and renaming it to match the new
  ticket's scope slug, versus some other reuse semantics.
- Whether skipping the merge-confirmation prompt applies only when the user
  supplied an explicit up-front goal that already implies "drive to merge",
  or is a blanket branch-policy default regardless of how the ticket was
  entered.
- How this interacts with `lead-implement`'s existing invariant "Wait for
  user approval before merge or another implementation slice" — whether that
  invariant is being asked to change, or whether the fix is narrower (e.g.
  skip the *branch-creation/rename* confirmation only, while merge approval
  itself stays intact).
- Scope: does this apply only to `ws:lead-drain-ready-queue`-originated work,
  or to any `lead-proceed`/`lead-implement` run where the user stated an
  explicit up-front goal?

## Status

Not yet discussed or designed. Sage review intentionally left at
`recommended` — still pending.
