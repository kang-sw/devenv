---
title: Add dashboard workspace forget/remove UI
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-workroot-registry-activation: current durable registry spine that needs workspace-level owner controls
  260523-feat-ws-dashboard-linked-worktree-discovery: derived child workRoots should follow Git/filesystem state rather than receive direct owner cleanup controls
  260524-feat-ws-dashboard-workspace-root-prune-policy: automatic no-active-workRoot pruning remains separate from deliberate owner removal
related-mental-model:
  - ws-web-dashboard
---

# Add dashboard workspace forget/remove UI

## Background

Dashboard resource ownership is settling around a workspace-level user model:
the owner adds or removes workspace roots, while child workRoots under that
workspace are derived from filesystem and Git state. This replaces the older
workRoot-level forget/remove idea, which treated child workRoots as independent
owner-managed rows.

The dashboard still needs an explicit high-friction owner cleanup path for a
workspace the owner no longer wants visible. That path is separate from
automatic pruning: pruning removes a workspace when no active workRoots remain,
while owner removal deliberately removes a workspace root and its dependent
browser-only state.

## Direction

- Add explicit forget/remove controls at the workspace level, not at the child
  workRoot level.
- Keep child workRoots, including linked Git worktrees, derived from the
  workspace root's filesystem/Git projection. Do not add a direct child
  workRoot forget action.
- Keep filesystem deletion and Git worktree deletion out of scope. Removing a
  workspace from the dashboard removes daemon-local ownership/membership and
  dependent browser-only panes or selections, not files on disk.
- Use high-friction confirmation copy that states the action removes the
  workspace from the dashboard only and does not delete files or Git worktrees.
- Allow unavailable workspaces to be forgotten. Confirmation copy should still
  avoid implying that the dashboard inspected or deleted the underlying path.
- If a workspace has online or active child workRoots, the remove action should
  perform a bounded deactivate-and-remove sequence instead of forcing the owner
  through a separate manual deactivation step.
- A removed workspace may reappear only after a later explicit open action.
  Suppression lists for automatic rediscovery are out of scope for this ticket.
- Route visible controls through stable dashboard command ids, not direct-only
  click handlers.
- Do not expose host paths, Git internals, pairing tokens, or daemon-private ids
  in browser-visible text or logs.

## Phases

### Phase 1: Add workspace-level owner removal

Add an explicit, confirmed workspace forget/remove action that removes
daemon-local workspace ownership/membership and dependent browser-only state.
The action must not delete files, remove Git worktrees, or expose host paths.

Removing a workspace with active child workRoots should run a bounded
deactivate-and-remove sequence. Unavailable workspaces remain removable with
copy that describes dashboard-only removal rather than filesystem deletion.
Child workRoots do not receive direct forget/remove controls.

Verification should cover confirmation flow, unavailable workspace removal,
active-child deactivation/removal, dependent pane/selection cleanup,
rediscovery only through explicit open, command-id dispatch, and no host-path
leakage.

### Result (pending) - 2026-05-24

Implemented Phase 1 workspace-level owner removal:

- Added an owner-authenticated `DELETE /api/dashboard/workspaces/{workspaceId}`
  route that removes daemon-local workspace membership and persists the updated
  registry without deleting files or Git worktrees.
- Added the workspace-level `workspace.remove` dashboard command and exposed it
  only on workspace/compact-workspace rows, not on child workRoot rows.
- The frontend requires explicit confirmation copy that states the action is
  dashboard-only and does not delete files or Git worktrees.
- Successful removal reconciles the live resource view and clears read-only
  panes/order plus workbench browser-only group/order state for removed roots.
- Live terminal sessions for removed workRoots are dropped from the daemon
  terminal registry after registry persistence succeeds.

Verification:

- `cargo test -p ws-dashboard-daemon --test routes workspace_remove_route_forgets_workspace_without_deleting_files_or_paths --manifest-path ws-dashboard/Cargo.toml`
- `cargo test -p ws-dashboard-daemon --test routes root_picker_routes_are_owner_authenticated --manifest-path ws-dashboard/Cargo.toml`
- `npm run test:commands`
- `npm run build`
- `npm run test:browser -- dashboard-acceptance.spec.ts`
