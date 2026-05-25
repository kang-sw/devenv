---
title: Discover linked Git workRoots through the durable registry
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: persisted roots and discovered sibling worktrees should share a clear source model
  260523-feat-ws-dashboard-workroot-registry-activation: prerequisite durable membership, availability, and activation model
  260524-feat-ws-dashboard-workspace-root-prune-policy: workspace root ownership and automatic empty-workspace pruning policy
  260524-feat-ws-dashboard-workspace-forget-remove-ui: explicit owner cleanup is workspace-level rather than child-workRoot-level
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
activation spine. Linked worktree discovery should discover child workRoots
from Git metadata, update their filesystem/Git-derived projection on refresh,
and avoid creating a parallel hidden-discovery model.

## Discussion

Agreed direction:

- When an opened workRoot is a Git repository, discover sibling linked worktrees
  through Git metadata such as `git worktree list --porcelain`.
- Add discovered linked worktrees as child workRoot rows under the same
  owner-managed workspace root, preserving the existing opaque `workRootId` and
  `gitLinkedWorktree` kind vocabulary.
- Treat linked worktrees as child workRoots derived from the workspace root
  workRoot. They should not become independent workspaces unless a later
  explicit derive/promote operation creates a new owner-managed workspace.
- Do not expose Git `prunable` as a first-class public availability value.
  Surface it, if needed, as bounded degraded detail under an unavailable or
  missing state.
- Recompute current availability and classification from filesystem/Git on
  explicit refresh and bounded polling. Git discovery determines the sibling
  worktree set; filesystem access determines whether each child workRoot is
  currently usable.
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
- Do not add dashboard-side delete/remove worktree functionality or direct
  child workRoot forget controls as part of this ticket. Explicit owner cleanup
  belongs to workspace-level removal, while child workRoots mirror Git and
  filesystem state.
- Do not introduce an invisible discovered-worktree state. Known child
  workRoots are visible while their owning workspace remains visible, but the
  workspace root policy may automatically prune a workspace when it has no
  active workRoots.
- Externally added linked worktrees appear on explicit refresh or bounded
  polling. Externally removed worktrees disappear from the child projection when
  Git no longer reports them and the filesystem is not usable. Workspace pruning
  still depends only on the workspace's overall active workRoot count.

## Phases

### Phase 1: Discover linked worktrees as child workRoots

Extend the live resource refresh path so Git repositories can discover linked
worktrees through `git worktree list --porcelain`, then present those entries
as child workRoots under the owner-managed workspace root.

Discovery should avoid broad filesystem crawling and should not create
independent workspaces. Public state should keep the existing `workRoot`
vocabulary, derive availability from current filesystem/Git access, and keep
Git-specific details such as `prunable` bounded behind broader unavailable or
missing states.

Verification should cover primary-root discovery, linked-worktree discovery,
external addition, external removal, no direct child forget/remove action,
workspace prune interaction through active workRoot count, and no host-path or
Git-metadata path leakage.

### Result (pending) - 2026-05-24

Implemented Phase 1 linked Git worktree discovery:

- The live dashboard resource projection expands an owner-opened Git workRoot
  through `git worktree list --porcelain` without broad filesystem crawling.
- Discovered linked worktrees render as `gitLinkedWorktree` child workRoots
  under the same workspace rather than independent workspaces.
- Discovered child workRoots are synchronized into the in-memory registry with
  `Discovered` provenance so file/terminal/activity routes can target them,
  while persisted owner state continues to store only explicitly opened roots.
- Removed linked worktrees fall out of the discovered projection on refresh;
  unavailable-only workspaces then follow the no-active-workRoot prune policy.
- Git metadata paths and host paths remain absent from browser-visible resource
  responses.

Verification:

- `cargo test -p ws-dashboard-daemon --manifest-path ws-dashboard/Cargo.toml`
