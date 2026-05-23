---
title: Surface linked Git worktrees in dashboard resource discovery
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: persisted roots and discovered sibling worktrees should share a clear source model
related-mental-model:
  - ws-web-dashboard
---

# Surface linked Git worktrees in dashboard resource discovery

## Background

Dogfood feedback found that a repository with linked Git worktrees does not
show those worktrees in the dashboard even though the original resource model
intended Git primary roots and linked Git worktrees to be additive workRoot
kinds under one workspace. The current local discovery provider can classify a
specific opened path as `gitPrimaryRoot` or `gitLinkedWorktree`, but the live
resource route only passes already-opened candidate paths into that provider.
Opening the primary repository root therefore does not automatically expand the
repository's linked worktree list.

The current `devenv` checkout demonstrates the mismatch: `git worktree list`
reports the main worktree plus linked/prunable worktrees, while dashboard live
resources remain bounded to daemon-opened paths.

## Discussion

Likely first behavior:

- When an opened workRoot is a Git repository, discover sibling linked worktrees
  through Git metadata such as `git worktree list --porcelain`.
- Add reachable linked worktrees as additional workRoot rows in the same
  workspace, preserving the existing opaque `workRootId` and
  `gitLinkedWorktree` kind vocabulary.
- Degrade prunable, missing, moved, or inaccessible worktrees as stale rows
  instead of dropping them silently.
- Keep host paths daemon-private; the browser sees labels, opaque ids, kind,
  status, and actions, not raw Git metadata paths.
- Avoid broad filesystem crawling. This should be Git-worktree expansion from an
  already opened repository/worktree, not an arbitrary disk scan.

Open questions:

- Should linked worktrees become immediately opened workRoots for file and
  Activity APIs, or should selecting/opening them register them explicitly?
- Should prunable worktrees appear by default, behind an unavailable/degraded
  state, or be hidden until a diagnostic view exists?
- How should persistence interact with auto-discovered linked worktrees: store
  only user-opened roots, or store the expanded set with provenance?
