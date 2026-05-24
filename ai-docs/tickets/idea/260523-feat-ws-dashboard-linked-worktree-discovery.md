---
title: Discover linked Git workRoots through the durable registry
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: persisted roots and discovered sibling worktrees should share a clear source model
  260523-feat-ws-dashboard-workroot-registry-activation: prerequisite durable membership, availability, and activation model
  260524-feat-ws-dashboard-workspace-root-prune-policy: workspace root ownership and automatic empty-workspace pruning policy
related-mental-model:
  - ws-web-dashboard
---

# Discover linked Git workRoots through the durable registry

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

This ticket remains behind the durable workspace/workRoot registry and
activation spine. Linked worktree discovery should add and update known
workRoot membership; it should not create a parallel hidden-discovery model.

## Discussion

Agreed direction:

- When an opened workRoot is a Git repository, discover sibling linked worktrees
  through Git metadata such as `git worktree list --porcelain`.
- Add discovered linked worktrees as additional known workRoot rows in the same
  durable workspace registry, preserving the existing opaque `workRootId` and
  `gitLinkedWorktree` kind vocabulary.
- Treat linked worktrees as child workRoots derived from the workspace root
  workRoot. They should not become independent workspaces unless a later
  explicit derive/promote operation creates a new owner-managed workspace.
- Default discovered sibling workRoots to offline activation. Users explicitly
  bring a workRoot online before file, Activity, or terminal APIs target it.
- Recompute current status from filesystem/Git on explicit refresh and bounded
  polling. Missing, prunable, moved, or inaccessible worktrees remain visible
  as degraded rows instead of dropping silently.
- Use the durable registry's separated availability/activation model. Linked
  discovery should never overload `status: online/offline` to mean both
  reachability and activation.
- Detect externally added and externally removed worktrees on explicit refresh
  and bounded polling. Later filesystem watchers may only act as refresh-needed
  hints, not as the source of truth.
- Keep host paths daemon-private; the browser sees labels, opaque ids, kind,
  status, and actions, not raw Git metadata paths.
- Avoid broad filesystem crawling. This should be Git-worktree expansion from an
  already opened repository/worktree, not an arbitrary disk scan.
- Do not add dashboard-side delete/remove worktree functionality as part of this
  ticket. Future forget/delete UX must be separate from discovery and must not
  be required to keep externally deleted worktrees visible as degraded rows.
- Do not introduce an invisible discovered-worktree state. Known child
  workRoots are visible while their owning workspace remains visible, but the
  workspace root policy may automatically prune a workspace when it has no
  active workRoots.

Open questions:

- Which Git metadata states should map to which public availability labels:
  reachable, missing, inaccessible, prunable, moved, or unknown?
- Should `prunable` be a first-class public availability value or a degraded
  Git-specific detail under a broader unavailable/missing availability?
- How frequently should bounded polling refresh selected, online, offline, and
  large workRoot sets?
- How should externally removed child workRoots interact with the new root
  policy when the workspace still has other active workRoots?
