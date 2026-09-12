---
title: "A reviewer delegate can leave the shared worktree on another branch"
---

# A reviewer delegate can leave the shared worktree on another branch

## Observed

While executing `260909-refactor-drain-ready-queue-worker-spawner` Phase 2, a
worker spawned a fresh reviewer with the rendered `code-reviewer` playbook,
the ticket path, and the branch name, per the worker protocol. The reviewer
verified the diff by running `git checkout` inside the *shared* worktree —
switching to `develop`, re-running the build and both test suites there, and
reporting that it had "restored branch state to `develop`". The review itself
was correct and read-only with respect to file content, but the worker's next
shell call landed on `develop`: the working tree carried the pre-change text
of the files it had just edited, and the work branch had to be restored by
hand before the round-1 fixes could be committed.

The worker noticed only because a doc it had edited read as unedited. A
slower failure is available: an edit, a regen, or a commit made while the
worktree sits on the wrong branch.

## Why it is a surprise

A review of a named commit range never needs to move `HEAD` — `git diff
<base>..<head>`, `git show`, and `git log` read any commit from any checkout.
The delegate is told which branch to review, so "check out the branch" is a
plausible reading of its task, and nothing in the rendered prompt says the
worktree is shared with a live caller or that `HEAD` must not move.

The hazard grows with the worker-interpreter direction: every worker spawns
its reviewers into the same worktree it is editing in, and the epic opens
parallelism inside the drain loop later.

## Possible directions

- State the read-only boundary in `HEAD` terms in the reviewer playbook: read
  the range with `git diff`/`git show`; never checkout, stash, reset, or
  otherwise move `HEAD` or the index.
- Or give reviewers their own checkout, so the boundary is structural rather
  than prose (`git worktree add` per review, or a host-provided isolated
  worktree).
- Either way, decide whether the same rule belongs on every delegate a worker
  spawns, not just reviewers.

## Evidence

- Round-1 review of commit `f2294816` on branch
  `impl/epic/refound/acid-fried-exile`; the worker's following `git branch
  --show-current` returned `develop`.

## Result

Took the prose direction, not the per-review-checkout one. `code-reviewer.md`
gained one paragraph under `## Constraints`: the checkout is shared with a
live caller still editing in it; read the range with `git diff <base>..<head>`,
`git show`, and `git log`, which reach any commit from any checkout; never
`checkout`, `switch`, `stash`, `reset`, `rebase`, or otherwise move `HEAD` or
the index; never build or test on a checkout other than the one handed over.
It names the failure the reviewer cannot observe itself - the caller's next
edit or commit lands on the wrong branch over pre-change contents - because
the delegate has no other way to know moving `HEAD` costs anything.

The paragraph is downstream-neutral: no repository-specific names, paths, or
vocabulary. Mirror regenerated (`WSRSRC_REGEN=1` manifest, then
`WS_REGEN_WSFLOW_RSRC=1`); `diff -r agents-plugin/rsrc
agents-plugin-wsflow/rsrc` is empty and the drift tests pass with no env var
set.

Not settled here: whether the same boundary belongs on every delegate a worker
spawns rather than reviewers only, and whether reviewers should eventually get
a structural per-review checkout instead of a prose rule. Prose was chosen now
because it costs nothing at spawn time and covers the observed failure; a
structural boundary is a separate change with its own cost.
