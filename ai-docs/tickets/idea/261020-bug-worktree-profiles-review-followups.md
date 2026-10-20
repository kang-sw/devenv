---
title: Worktree profiles lite-review follow-ups
related:
  261020-feat-worktree-profiles: origin; the lite review recorded these as Minor in its Phase 4 Result
---

# Worktree profiles lite-review follow-ups

## Background

The lite review of `261020-feat-worktree-profiles` left seven Minor findings,
recorded in that ticket's Phase 4 Result. The worker flagged three as worth a
follow-up outside the ticket's scope. They are captured here unverified, as
the worker reported them.

## Findings

- **Release and remove read the whole profile family.** One invalid profile in
  any scope blocks `worktree.release` and `worktree.remove`, even when it is
  unrelated to the worktree. A field added by a newer ws that an older ws
  rejects does the same.
- **A failed fresh sparse acquire leaves a dirty worktree.** When the sparse or
  branch step fails, the worktree keeps an empty index, and only
  `worktree.release` recovers it. The former `sparse_paths` path had the same
  leak.
- **A housekeeping worker key misses repo-scope config.** The cone checkout
  omits `.ws-workflow/`, so repo-defined profiles are invisible to that key.

- **The strict profile decoder also gates `workflow_manual` and acquire.**
  Raised by the v0.48.0 release-gate correctness review:
  `worktree_profile.go:139` decodes with `DisallowUnknownFields`, and
  `workflow_manual.go:340/:391` resolves the whole family. A profile field added
  by a later ws would fail an older teammate's session-bootstrap manual and
  acquire, not only release and remove.

## Questions

- Should release and remove resolve only the profile the worktree is marked
  with, and tolerate unknown fields from a newer ws?
- Should a failed fresh acquire roll the worktree back to a clean, reusable
  state itself?
- Should the housekeeping cone always include `.ws-workflow/`?
