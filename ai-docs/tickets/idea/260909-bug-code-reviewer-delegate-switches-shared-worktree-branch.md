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
