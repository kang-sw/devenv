---
title: Restore single workspace/workRoot navigation collapse
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-workroot-registry-activation: durable registry changes make single known workRoot states common
related-mental-model:
  - ws-web-dashboard
---

# Restore single workspace/workRoot navigation collapse

## Background

Dogfood feedback found that a dashboard resource tree with one workspace and
one workRoot still renders separate workspace and workRoot rows, even though
the resource view-model contract says singleton chains may render as compact
rows. The current frontend compact path appears to require exactly one
`mainInstance` under the single workRoot before it collapses the navigation
rows. Real opened workRoots commonly have no main instance, so the compact
presentation does not appear in the default dashboard path.

This is separate from linked-worktree discovery and workRoot registry
activation. It is a browser presentation issue: the daemon should continue to
preserve the full `server -> workspace -> workRoot -> mainInstance ->
subInstance` hierarchy as data, while the frontend decides whether the left
navigation can compact singleton rows.

## Expected Behavior

- A single workspace with a single workRoot should render as one compact
  navigation row when there are no sibling workRoots that need comparison.
- The compact row should remain selectable by the workRoot id so file,
  Activity, terminal, workbench, and browser-route behavior keep using the
  concrete workRoot identity.
- Main/sub instance rows should not reappear as default recursive left-nav rows.
- Multi-workRoot workspaces should keep separate workspace and workRoot rows.
- Offline or unavailable single workRoots should still display availability and
  activation state clearly in the compact row.

## Notes

- Current frontend code has a `compactMainInstance` path that only compacts when
  the workspace is compactable, has exactly one workRoot, the root is
  compactable, and the root has exactly one main instance.
- The fix should clarify whether compacting means
  `workspace + workRoot` only, or `workspace + workRoot + mainInstance` when a
  main instance exists. The default no-main-instance dashboard path should not
  be excluded unintentionally.
