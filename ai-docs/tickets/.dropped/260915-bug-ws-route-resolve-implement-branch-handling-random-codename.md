---
title: "route.resolve_implement branch handling is inconsistent under lead-provisioned random-codename impl branches"
dropped: 2026-09-21
---

# `route.resolve_implement` branch handling under pre-provisioned impl branches

## Observed

In a parallel `ws:lead-run` batch (2026-09-15) the lead pre-created each
ticket's impl branch via `worktree.acquire` with the canonical
`impl/<goal-branch>/<random-slug>` shape, then spawned a worker per
worktree. The lead-run parallel route expects the worker's
`route.resolve_implement` to detect the already-checked-out branch and
return a **continue** verdict. Across three sibling workers the behavior
diverged three different ways:

- **Ticket 4** (`plum-tidal-wren`): continued on the provisioned branch as
  expected.
- **Ticket 5** (`flint-cove-ridge`): the verdict's first todo **renamed**
  the provisioned branch to a freshly derived slug `slurp-had-affix`. The
  worker followed the rename per protocol. The lead's tracking (worktree
  key, assignment note) still resolved, but the branch name the lead
  pre-committed a route-facts fix onto no longer matched.
- **Ticket 3** (`brisk-cedar-nook`): the verdict was a deterministic
  **"Branch Action: stop"** because the branch's target scope "differs
  from suspected prior work". The worker reasoned that this repo's impl
  branches are *always* random codenames that never lexically match a
  ticket stem, so the branch-identity heuristic will **perpetually
  false-positive** here; it verified the sole pre-existing commit was the
  legitimate route-facts fix and proceeded (recording the ambiguity, not
  escalating).

Separately, `merge_confirm` resolved to `skip` for tickets 4 and 5 but
`ask` for ticket 3 on the *same* goal run — the goal-run auto-merge
signal was not uniform across siblings.

## Why it matters

- The lead-run parallel route documents a **continue** contract for
  pre-provisioned impl branches; two of three workers deviated (rename,
  stop). A rename desyncs any lead state keyed on the pre-chosen branch
  name; a false-positive stop forces every worker in this repo to reason
  past a deterministic guard on each run.
- The branch-identity heuristic assumes impl branch slugs carry ticket
  identity. This repo's convention is *random word-word-word codenames*
  that never match a stem, so the heuristic has no signal here and is a
  standing false-positive source.
- Non-uniform `merge_confirm` across a single goal-run batch makes the
  lead's auto-merge vs. ask decision inconsistent for equivalent work.

## Candidate follow-ups (triage)

- Teach `route.resolve_implement` to treat an already-checked-out
  `impl/<goal-branch>/<slug>` at the expected worktree as an authoritative
  continue, without re-deriving or renaming, when the lead provisioned it.
- Drop or gate the branch-stem-identity heuristic when the project's
  branch convention is random codenames (or key continue-detection on the
  worktree/agenda binding rather than the slug text).
- Make goal-run `merge_confirm` derivation uniform across a batch.

## Notes

Captured under the "Dogfood surprises get captured" discipline; no design
commitment implied. Sibling same-session dogfood tickets:
`260915-bug-ws-ticket-facts-new-public-symbol-likely-enum`,
`260915-bug-ws-tickets-close-operates-on-server-cwd-not-worktree`.
