---
title: "Reduce implement-branch friction: reuse+rename policy, auto-delete cleanup, and a shorter naming convention"
sage-review: recommended
---

# Reduce implement-branch friction: reuse+rename policy, auto-delete cleanup, and a shorter naming convention

## Background

Raised mid-session while running `ws:lead-drain-ready-queue` end-to-end for
`260703-chore-bootstrap-staleness-alarm`, then again while running
`lead-proceed`/`lead-implement` on `260707-research-sage-review-staged-design-completeness-split`.
Captured verbatim from the user's framing so a later session can pick it up
without re-deriving intent. Three related friction points, all about
implement-branch ceremony feeling heavier than the user wants:

### 1. Default branch policy: reuse+rename during goal-driven drains

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

### 2. Auto-delete implement branches after merge, without asking

Raised later in the same session, after `lead-implement`'s Branch Cleanup
step asked for confirmation before `git branch -d` on a just-merged
implement branch (per its current "ask the user" step). The user's framing:
"이것도 자꾸 안 물어보고 ... 자동 삭제하게" (this too — stop asking, just
auto-delete). Same underlying complaint as item 1: too many confirmation
prompts around routine, low-risk branch lifecycle steps once a merge has
already happened. Not yet decided whether this should be unconditional or
still respect the existing skip conditions (branch checked out, linked
worktree, ambiguous merge target, unreachable commits) that already exist in
`lead-implement`'s Branch Cleanup step — presumably those skip conditions
stay as-is and only the *confirmation ask* on the safe/no-skip-condition path
is removed.

### 3. Shorter implement-branch naming convention

Raised in the same message as item 2. The user wants the branch naming
convention changed from the current `implement/<slug>` to a shorter form:
`impl/<stem>`, with `<stem>` capped at a maximum of 15 characters. Exact
truncation/collision-avoidance mechanics not yet specified by the user (e.g.
what happens when the natural scope slug is longer than 15 characters —
truncate, abbreviate, hash-suffix, or something else).

Not yet discussed or designed beyond this framing. Open questions for a
future session include (non-exhaustive, not yet decided):

- What exactly "reuse+rename" means in item 1 — reusing the current branch
  if it is already an `implement/*`/`impl/*` branch and renaming it to match
  the new ticket's scope slug, versus some other reuse semantics.
- Whether skipping the merge-confirmation prompt (item 1) applies only when
  the user supplied an explicit up-front goal that already implies "drive to
  merge", or is a blanket branch-policy default regardless of how the ticket
  was entered.
- How item 1 interacts with `lead-implement`'s existing invariant "Wait for
  user approval before merge or another implementation slice" — whether that
  invariant is being asked to change, or whether the fix is narrower (e.g.
  skip the *branch-creation/rename* confirmation only, while merge approval
  itself stays intact).
- Scope of item 1: does it apply only to `ws:lead-drain-ready-queue`-originated
  work, or to any `lead-proceed`/`lead-implement` run where the user stated
  an explicit up-front goal?
- Whether item 2 (auto-delete without asking) is scoped the same way as item
  1 (only goal-driven/drain-queue runs) or applies unconditionally to every
  `lead-implement` Branch Cleanup step regardless of how the ticket entered
  the pipeline. The user's phrasing ("이것도") suggests it may be intended as
  the same blanket default as item 1, but this wasn't explicitly confirmed.
- Item 3's exact truncation/collision mechanics for stems over 15 characters,
  and whether the `impl/` rename applies retroactively to in-flight branches
  or only to newly created ones going forward.
- Whether items 1-3 should ship as one ticket or be split (branch-naming is a
  purely mechanical convention change independent of the confirmation-prompt
  behavior in items 1-2, and could land separately).

## Status

Not yet discussed or designed. Sage review intentionally left at
`recommended` — still pending.
