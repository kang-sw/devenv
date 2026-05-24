---
title: Add dashboard workRoot forget/remove UI
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-workroot-registry-activation: durable known workRoot membership needs an explicit removal path
  260523-feat-ws-dashboard-linked-worktree-discovery: discovered linked workRoots may become stale, prunable, or intentionally unwanted
  260524-feat-ws-dashboard-workspace-root-prune-policy: automatic empty-workspace pruning is separate from explicit owner cleanup
related-mental-model:
  - ws-web-dashboard
---

# Add dashboard workRoot forget/remove UI

## Background

Dogfood feedback after durable workRoot registry activation found that the
dashboard can keep known workRoots visible but does not expose a browser control
to forget or remove an unwanted known workRoot. This is visible when temporary
or prunable worktrees such as `ws-dash-gate-*` or stale linked worktrees remain
known after their useful lifetime.

The linked-worktree discovery ticket explicitly excludes delete/remove behavior.
This ticket captures that separate follow-up so discovery can keep degraded
workRoots visible without also needing to solve user-driven cleanup.

The workspace root prune policy separates automatic empty-workspace pruning from
explicit owner cleanup. A workspace with no active workRoots may disappear from
the visible resource tree automatically, while a workspace that still has an
active child workRoot remains visible even if its root workRoot is unavailable.
This ticket remains about deliberate owner forget/remove controls for visible
resources that policy does not automatically prune.

## Direction

- Add an explicit high-friction forget/remove action for known dashboard
  workRoots.
- Do not make forget/remove responsible for automatic empty-workspace pruning;
  that belongs to the workspace root policy.
- Keep filesystem deletion out of scope unless a later ticket deliberately adds
  host file-manager behavior.
- Forgetting a workRoot should remove daemon-local durable membership and any
  browser-only panes or selections that depend on that workRoot.
- Unknown, offline, unavailable, prunable, or missing workRoots should be
  eligible for the same safe forget path when policy allows it.
- Visible controls must dispatch through stable dashboard command ids, not
  direct-only click handlers.
- The action must not expose host paths, Git internals, pairing tokens, or
  daemon-private ids in browser-visible text or logs.

## Open Questions

- Should online workRoots require deactivation before forget is offered, or can
  forget perform a bounded deactivate-and-remove sequence?
- Should linked workTrees discovered from Git metadata reappear automatically on
  the next refresh after being forgotten, or should a suppression list exist?
- Should prunable/missing roots get a different label or confirmation copy than
  available roots?
