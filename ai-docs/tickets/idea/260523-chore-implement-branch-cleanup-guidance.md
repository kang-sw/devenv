---
title: Implement merge flow lacks branch cleanup guidance
related:
  260523-bug-implement-merge-target-discovery: merge-target safety and cleanup both affect implementation branch lifecycle
---

# Implement merge flow lacks branch cleanup guidance

## Background

Local dogfood exposed many stale implementation branches after completed work.
`lead-implement` documents the final merge step, but it does not say whether the
merged implementation branch should be deleted, reported as retained, or left to
manual cleanup. That omission makes branch accumulation the default outcome even
after the implementation branch is safely contained in its merge target.

Branch cleanup is destructive enough to require explicit approval or a clearly
bounded post-merge rule. The workflow should not silently delete active
worktree-checked-out branches, unmerged branches, or branches whose target was
ambiguous.

## Proposed Direction

Add a post-merge cleanup decision to the implementation branch lifecycle.
Possible behavior:

- after an approved merge, identify whether the implementation branch is an
  ancestor of the actual merge target;
- skip deletion for the current branch, linked-worktree branches, ambiguous
  merge targets, or branches with unpushed unique commits;
- ask before deleting unless repository rules explicitly allow automatic
  deletion after merge;
- report retained branches with the reason so cleanup debt remains visible.

This should be handled together with safer merge-target discovery so cleanup is
based on the same proven target, not a default branch assumption.
