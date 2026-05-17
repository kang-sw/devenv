# Brief: 260517-bug-ws-dashboard-dockview-dynamic-groups

## Intent

Make Dockview split-drop behavior real in the dashboard workbench. When the
user drops a pane into a Dockview-created split group, dashboard state must
create or map a durable group id, retain the pane there across React
synchronization, and keep file/terminal workflows usable afterward.

## Scope Boundary

Implement the whole ready ticket: dynamic group modeling, automatic placement
policy, and browser evidence for split-drop durability. Keep pinned/opened tab
visual polish, icon badges, preview tabs, and close-button polish deferred to
`260517-feat-ws-dashboard-workbench-tab-polish`.

Do not reintroduce a custom tab/split layout. Dockview remains the visible
owner of groups, tabs, split sizing, and pane attachment. Dashboard policy owns
dashboard group ids, pane order, active panes, placement, duplicate focus, close
behavior, and restore sanitization.

## Caller-Visible Contract

- Opened workRoots start with two dynamic dashboard groups, group 1 and group
  2. These initial groups are not permanent `primary`/`support` special cases.
- Terminal creation prefers group 1.
- Editor/read-only file opens prefer group 2 when at least two groups exist.
- If only group 1 exists and an editor/read-only file opens, create group 2 and
  open the pane there.
- Groups 3+ are user-created groups. Automatic terminal/file placement must not
  target groups 3+ unless later explicit focused/open-into-group policy is
  added.
- A Dockview split-drop preview that accepts a tab drop creates or maps a
  dashboard group and the moved pane remains in that group after React
  synchronization.
- Duplicate logical keys still focus the existing pane instead of creating
  duplicates.
- Raw Dockview handles do not escape the workbench adapter.

## References

- [Must] `ai-docs/tickets/ready/260517-bug-ws-dashboard-dockview-dynamic-groups.md`
- [Must] `ai-docs/spec/ws-web-dashboard/index.md`
  - `260516-ws-web-dashboard-workroot-workbench-substrate`
  - `260516-ws-web-dashboard-file-open-placement-policy`
  - `260516-ws-web-dashboard-terminal-tab-selection-and-empty-initial-state`
  - `260516-ws-web-dashboard-browser-ui-acceptance-gate`
  - `260516-ws-web-dashboard-workroot-io-command-placement-polish`
- [Must] `ai-docs/mental-model/ws-web-dashboard.md`
- [Must] `ws-dashboard/frontend/src/workbench/editorGroupModel.ts`
- [Must] `ws-dashboard/frontend/src/workbench/policy.ts`
- [Must] `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx`
- [Must] `ws-dashboard/frontend/src/App.tsx`
- [Must] `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts`
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`
- [Maybe] `260517-feat-ws-dashboard-workbench-tab-polish` for deferred
  pinned/opened visual semantics only.

## Skeleton Contracts

Skeleton commit `9c6e642` established placeholder contracts for:

- `commitWorkbenchPaneMoveIntoDynamicGroup`
- `decideSurfaceOpenWithDynamicGroups`
- reserved workbench model assertions for dynamic movement and placement
- `expectDurableDockviewSplitDrop` browser evidence scaffold

Replace the placeholder throws with implementation. Preserve the contract
comments unless they become stale after equivalent code and tests make the
behavior obvious.

## Implementation Notes

- Model dashboard workbench groups as ordered dynamic state rather than fixed
  `primary`/`support` keys. Existing test names may still mention old labels;
  migrate where necessary without losing behavior.
- `dockviewLayout.tsx` currently ignores `onDidMovePanel` targets whose Dockview
  group id is not mapped to a dashboard group. This is the snap-back path to
  fix: map or allocate a dashboard group before calling product callbacks.
- Keep app-level terminal/file ordering state compatible with dynamic group ids.
- Do not persist surface kind, row policy, daemon ids, or raw Dockview group
  handles in layout JSON.
- Browser split-drop tests should use real daemon-served frontend evidence.
  Continue using platform-aware terminal command helpers; do not add POSIX-only
  shell assertions.

## Acceptance

- `npm run test:workbench`
- `npm run test:work-root-files`
- `npm run test:terminals`
- `npm run build`
- `npm run test:browser`

Browser evidence must prove Dockview ownership, split-drop durability after
React synchronization, and ordinary file/terminal interactions in the resulting
layout.
