---
title: Fix ws dashboard Dockview split scroll resets
parent: 260514-epic-ws-web-dashboard-mvp
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
  - 260517-ws-dashboard-readonly-text-scroll-containment
  - 260521-ws-dashboard-activity-console-ui-shell
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-23
---

# Fix ws dashboard Dockview split scroll resets

## Background

Dogfood feedback on 2026-05-23 showed that enabling Dockview split groups can
periodically force scroll position back to the top across WorkRoot Activity,
read-only editor panes, and other widgets. The symptom is broader than Activity
Console transcript tail-follow behavior and appears tied to the common Dockview
workbench adapter.

Local investigation points at `syncDockviewWorkbench()` in
`ws-dashboard/frontend/src/workbench/dockviewLayout.tsx`: it stores active pane
state per dashboard group, but checks `existingPanel.api.isActive` before
calling `existingPanel.api.setActive()`. Dockview's panel active state includes
the active group, so in a split only one group's visible panel is globally
active. Re-running sync can therefore activate each group-active pane in turn,
causing repeated Dockview focus/content updates.

The same area also compares `params.body` by ReactNode identity before
`updateParameters()`. Pane bodies are recreated on App renders, so unrelated
refreshes can churn Dockview parameters even when the pane's visible data has
not changed.

## Phases

### Phase 1: Stabilize split active-pane sync

Make Dockview sync distinguish "visible active tab within this group" from
"globally focused active panel". Avoid calling `setActive()` for an already
visible active pane in an inactive split group. Add regression coverage that a
two-group workbench sync does not repeatedly activate both groups on ordinary
rerender.

### Phase 2: Bound Dockview parameter churn

Replace ReactNode identity comparison for pane body updates with a stable
revision/fingerprint or equivalent adapter boundary so resource/activity polling
does not call `updateParameters()` for unchanged editor/activity panes. Preserve
real updates for transcript refresh, file content changes, terminal status, and
tab metadata.

### Phase 3: Browser acceptance for split scroll stability

Add a browser-level regression that opens split workbench panes, scrolls an
editor and WorkRoot Activity transcript away from top, triggers the relevant
resource/activity refresh path, and asserts scroll positions are not reset.

### Result (5adaad3) - 2026-05-23

Fixed the shared Dockview workbench adapter path. Split sync now distinguishes a
group's selected tab from Dockview's globally active focused panel, so ordinary
sync no longer calls `setActive()` for already selected panes in inactive split
groups. Pane parameter updates now use stable `contentRevision` values for
editor, terminal, and WorkRoot Activity surfaces instead of comparing recreated
React body identity.

Verification covered workbench model tests for group-local active-tab detection
and parameter churn, frontend production build, and browser acceptance that a
scrolled read-only pane in a split workbench survives refresh without resetting
to the top.
