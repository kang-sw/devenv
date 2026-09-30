---
title: lead-run parallel route merges before release, orphaning impl branches
---

# lead-run parallel route merges before release, orphaning impl branches

## Background

Dogfood observation (2026-09-30, two-ticket parallel run on `develop`): the
`lead-run` Parallel route says to collect reports, merge serially through
`ws/git.merge`, and release every acquired worktree. Following that order,
each `git.merge` succeeded but returned `cleanup_failed [advisory]`: deleting
the merged `impl/*` source failed because the branch was still checked out in
its pooled worktree. The lead had to release both worktrees and then run
`git branch -d` by hand.

Every parallel run hits this, since the worker's worktree always holds the
impl branch at merge time.

Candidate directions (unsettled):

- Reorder the playbook: release each worktree before merging its branch
  (release detaches HEAD and cleans the tree; the branch ref survives).
- Or have `git.merge` / `worktree.release` handle the leftover (e.g. release
  deletes a fully-merged impl branch it detaches from).
