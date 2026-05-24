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
- Route visible controls through stable dashboard command ids, not direct-only
  click handlers.
- Do not expose host paths, Git internals, pairing tokens, or daemon-private ids
  in browser-visible text or logs.

## Open Questions

- Should removing a workspace first deactivate every active child workRoot, or
  can the remove operation perform one bounded deactivate-and-remove sequence?
- Should a removed workspace root reappear if it is later rediscovered through
  another explicit open action, or should there be a suppression list?
- What confirmation copy is appropriate for unavailable roots where the
  dashboard cannot currently inspect the filesystem state?
