---
title: ws dashboard resource continuity sprint
related:
  260514-epic-ws-web-dashboard-mvp: containing dashboard MVP board that remains the long-lived parent context
  260524-feat-ws-dashboard-workspace-root-prune-policy: first stabilizing implementation slice for workspace/workRoot lifecycle
  260523-feat-ws-dashboard-readonly-file-pane-restore: restore file pane continuity after browser refresh or daemon restart
  260523-feat-ws-dashboard-linked-worktree-discovery: discover linked worktrees as child workRoots under owner-managed workspace roots
  260523-feat-ws-dashboard-tool-output-safe-summary: show bounded Activity Console tool output snippets for authenticated owners
  260524-feat-ws-dashboard-workspace-forget-remove-ui: add explicit workspace owner cleanup separate from automatic pruning
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard resource continuity sprint

## Scope

Coordinate a focused dashboard pass that makes the current dogfood workspace
model feel durable and inspectable across refreshes, daemon restarts, linked
worktrees, stale resources, and Activity Console transcript review.

This epic is a one-way board over existing child tickets. Child ticket
frontmatter does not need to point back to this epic, and their current
`parent:` links to the long-lived dashboard MVP board remain unchanged unless a
separate ticket edit deliberately changes that structure.

The sprint scope includes:

- Settling workspace root ownership, disabled/recovery-needed state, and
  automatic no-active-workRoot pruning.
- Restoring read-only file pane descriptors through normal file-open behavior.
- Discovering linked Git worktrees as child workRoots under an owner-managed
  workspace root.
- Making Activity Console tool-output transcript blocks bounded but useful for
  an authenticated owner viewer.
- Adding explicit workspace owner forget/remove controls for dashboard roots
  that automatic pruning should not remove.

## Non-Scope

- Terminal focus stability and native-Windows terminal control-key follow-ups.
- Main-session Activity freshness or named-agent history retention work.
- Broad persistable UI-state mapping beyond the file-pane restore child.
- Filesystem deletion, Git worktree deletion, or host file-manager behavior.
- Replacing the existing dashboard MVP epic as the durable parent board.

## Child Tickets

- `260524-feat-ws-dashboard-workspace-root-prune-policy` - done; implement the
  root ownership and automatic pruning policy first so later discovery and
  cleanup work has stable lifecycle semantics.
- `260523-feat-ws-dashboard-readonly-file-pane-restore` - done; restore
  browser-visible read-only file panes after refresh or daemon restart without
  persisting file contents or absolute host paths.
- `260523-feat-ws-dashboard-linked-worktree-discovery` - done; add Git linked
  worktrees as child workRoots instead of independent workspaces.
- `260523-feat-ws-dashboard-tool-output-safe-summary` - done; show bounded
  head/tail tool-output snippets for authenticated owners.
- `260524-feat-ws-dashboard-workspace-forget-remove-ui` - done; add explicit
  workspace cleanup UX on top of the root-prune policy and linked-worktree
  discovery behavior.

## Cross-Child Decisions

- Preserve the public resource vocabulary as `workspace` and `workRoot`; do not
  introduce a Git-specific public identity layer for linked worktrees.
- Keep host paths, Git metadata paths, and daemon-private ids out of
  browser-visible route identities. Activity Console may show bounded tool
  output snippets to authenticated owners, but must not expose internal storage
  paths or unbounded payloads.
- Treat automatic workspace pruning and explicit workspace owner removal as
  separate mechanisms. Pruning handles no-active-workRoot cleanup; owner removal
  handles deliberate cleanup of workspace roots.
- Discovery and cleanup must not silently promote child workRoots into new
  workspaces or expose direct child workRoot forget/remove controls. A future
  derive/promote operation may create a workspace explicitly.
- Persist descriptors and bounded display snippets, not source file contents,
  terminal buffers, or stale API response bodies.
- Route visible controls through stable dashboard command ids where the current
  dashboard command spine already applies.

## Completion Criteria

- Done: all listed child tickets are either completed or deliberately removed
  from this sprint scope with their final state recorded.
- Dropped: the sprint grouping stops being useful because the child tickets no
  longer form one dashboard continuity pass.
- Deferred: terminal stability, broader UI persistence, main-session Activity
  freshness, and filesystem/Git deletion features remain outside this epic
  unless new child tickets are explicitly added.
