---
title: Restore single workspace/workRoot navigation collapse
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-workroot-registry-activation: durable registry changes make single known workRoot states common
spec:
  - 260523-ws-dashboard-single-workroot-nav-collapse
related-mental-model:
  - ws-web-dashboard
---

# Restore single workspace/workRoot navigation collapse

## Background

Dogfood feedback found that a dashboard resource tree with one workspace and
one workRoot still renders separate workspace and workRoot rows, even though
the browser information architecture should keep the left navigation focused on
workspace/workRoot location selection. The current frontend compact path
appears to require exactly one `mainInstance` under the single workRoot before
it collapses the navigation rows. That condition is stale: main instances were
later moved out of the default left-nav hierarchy and into durable workbench
surfaces, with sub instances as secondary workbench projections.

This is separate from linked-worktree discovery and workRoot registry
activation. It is a browser presentation issue: the daemon should continue to
preserve the full `server -> workspace -> workRoot -> mainInstance ->
subInstance` hierarchy as data, while the frontend decides whether the left
navigation can compact singleton workspace/workRoot rows without depending on
main/sub instance presence.

## Expected Behavior

- A single workspace with a single workRoot should render as one compact
  navigation row when there are no sibling workRoots that need comparison.
- The compact row should remain selectable by the workRoot id so file,
  Activity, terminal, workbench, and browser-route behavior keep using the
  concrete workRoot identity.
- Main/sub instance rows should not reappear as default recursive left-nav rows,
  and left-nav compaction should not require any main/sub instance.
- Multi-workRoot workspaces should keep separate workspace and workRoot rows.
- Offline or unavailable single workRoots should still display availability and
  activation state clearly in the compact row.
- Stale spec or mental-model wording that still implies main/sub instances are
  part of default left-nav compaction should be corrected with the UI fix.

## Notes

- Current frontend code has a `compactMainInstance` path that only compacts when
  the workspace is compactable, has exactly one workRoot, the root is
  compactable, and the root has exactly one main instance.
- The fix should replace that premise with a `workspace + workRoot` singleton
  compaction rule. Any main instance, when present, belongs to the right-side
  workbench surface model rather than the left navigation collapse condition.

## Phases

### Phase 1: Restore workspace/workRoot singleton compaction

Update the browser left navigation so a single workspace with a single workRoot
renders as one compact workRoot-selected row without requiring any
main/sub-instance data. The compact row should display the workspace/workRoot
identity and the workRoot kind, availability, and activation metadata. It
should keep selecting the concrete workRoot id so file explorer, Activity,
terminal, workbench, and server-scoped browser routes keep using workRoot
identity.

Keep multi-workRoot workspaces expanded as separate workspace and workRoot
rows. Do not reintroduce main/sub-instance rows into the default left
navigation; those remain durable workbench surfaces or secondary projections.
Correct stale spec or mental-model wording only where it still implies
main/sub instances participate in default left-nav compaction.

Verification should include route-independent frontend coverage for
single-workRoot compaction without main instances, multi-root non-compaction,
offline/unavailable metadata visibility, and browser-level evidence if the
visible left-nav rendering changes.
