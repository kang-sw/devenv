---
title: ws dashboard Dockview dynamic groups
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260517-bug-ws-dashboard-dockview-workbench-parity: made Dockview the visible workbench layout owner but kept dashboard groups effectively fixed to primary/support
  260517-feat-ws-dashboard-workbench-tab-polish: should build on dynamic group behavior rather than fighting fixed-group assumptions
  260516-epic-ws-web-dashboard-workbench-substrate: owns the dashboard workbench substrate and layout policy boundary
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
  - 260516-ws-web-dashboard-file-open-placement-policy
  - 260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state
  - 260516-ws-web-dashboard-browser-ui-acceptance-gate
skeletons:
  phase-1: 9c6e642
  phase-2: 9c6e642
  phase-3: 9c6e642
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard Dockview dynamic groups

## Background

Dockview now owns the visible workbench layout, but dashboard state still treats
workbench groups as effectively fixed `primary` and `support` groups. Dockview
can show split-drop previews such as vertical split glyphs during drag/hover,
but dropping into a new split does not become durable dashboard arrangement
state. That mismatch makes the UI look capable while the operation appears to
fail or snap back.

The next corrective slice should make Dockview-created groups real dashboard
groups. This preserves the Dockview ownership promise and lets panel split
affordances work instead of being rejected after preview.

## Decisions

- Workbench groups become dynamic dashboard state, not a fixed two-key
  `primary`/`support` map.
- The initial opened-workRoot preset still starts with two dynamic groups.
  These are the initial group 1 and group 2, not permanent special cases.
- Terminal creation should prefer group 1 when a group exists.
- Editor/read-only file opens should prefer group 2 when at least two groups
  exist.
- If only one group exists and an editor/read-only file opens, create group 2
  and open the file there.
- Groups 3 and later are user-created layout groups. Automatic placement should
  not target them unless the user explicitly focuses, moves, or opens into that
  group through later policy.
- The dashboard must either persist or reconstruct dynamic group membership
  within the current browser state so Dockview split drops do not snap back on
  the next React sync.
- Raw Dockview handles still stay behind the workbench adapter. Dashboard state
  stores dashboard group ids, pane order, and active pane identity, not Dockview
  object handles.

## Phases

### Phase 1: Model dynamic workbench groups

Replace fixed `primary`/`support` assumptions in the frontend workbench state
with an ordered dynamic group model. Keep the initial two-group preset, but make
group ids generated/registered dashboard state that can grow when Dockview
creates a new split.

Success means moving a pane into a Dockview-created split creates or maps a
dashboard group id, records pane membership there, and survives the next
workbench synchronization without snapping back to the initial two groups.

### Phase 2: Apply automatic placement policy

Update terminal and editor/read-only file placement to use the new group model:
terminals prefer group 1, editor/read-only files prefer group 2, editor/file
opens create group 2 if only group 1 exists, and groups 3+ receive no automatic
placement unless later explicit focused-group policy says otherwise.

Success means default creation remains predictable after users create additional
groups: automatic terminal/file opens do not unexpectedly jump to group 3+.

### Phase 3: Verify split drag/drop behavior

Add browser-level evidence that Dockview split previews correspond to durable
behavior. The browser gate should drag a workbench tab into a vertical or
otherwise new split target, confirm a new dashboard group exists, confirm the
pane remains in that group after React synchronization, and confirm ordinary
terminal/file interactions still work afterward.

Success means the visible split-drop affordance no longer lies: a previewed and
accepted Dockview split drop produces a stable dashboard group instead of a
no-op or snap-back.
