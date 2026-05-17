# Survey: 260517-bug-ws-dashboard-dockview-dynamic-groups

## Reusable Components
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L58-L99` — `applyWorkbenchPaneOrder`: reapplies serialized pane order while appending newly discovered original panes; dynamic group state can reuse this once groups are no longer fixed to `primary`/`support`.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L105-L146` — `moveWorkbenchPane`: existing pure tab reorder/cross-group movement primitive; it currently no-ops if the target group is unknown.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L148-L181` — active-pane helpers: reconcile empty groups by omitting stale active ids and select panes per group.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L206-L237` — `commitWorkbenchPaneMove` plus `commitWorkbenchPaneMoveIntoDynamicGroup` skeleton: contract point for allocating/mapping a dashboard group before committing a Dockview split move.
- `ws-dashboard/frontend/src/workbench/policy.ts#L86-L101` — branded id constructors: use `workbenchGroupId` and `surfaceLogicalKey` for dashboard ids/keys, not Dockview handles.
- `ws-dashboard/frontend/src/workbench/policy.ts#L103-L145` — `decideSurfaceOpen` and dynamic skeleton: duplicate-key focus is already implemented; dynamic placement must preserve that while changing target groups/next state.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L36-L100` — default registry: `persistentTerminal` is pinned and `editor`/viewer/support kinds are opened; dynamic policy decisions should still derive row policy here.
- `ws-dashboard/frontend/src/workbench/dockviewBridge.ts#L10-L31` — Dockview bridge options/events seam: floating groups are disabled while drag/move/drop events remain available for adapter mapping.

## Existing Patterns
- Dockview visible owner marker and no custom split shell: see `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L164-L181` and browser assertion `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L146-L153`.
- Dockview sync rebuilds panels from dashboard groups and returns Dockview-group-to-dashboard-group mapping: see `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L237-L347`.
- Current snap-back cause: `onDidMovePanel` ignores moves when `move.panel.group.id` is not in `dockGroupToWorkbenchGroupRef`; see `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L128-L156`.
- App currently maintains three separate group/order states (`activePaneByGroup`, generic `paneOrderByGroup`, terminal/file order): see `ws-dashboard/frontend/src/App.tsx#L832-L843`.
- Workbench model currently hard-codes two fixed groups and labels (`primary`/`support`): see `ws-dashboard/frontend/src/App.tsx#L1292-L1385`.
- Terminal placement/list reconciliation currently routes through fixed `primary`/`support` state: see `ws-dashboard/frontend/src/App.tsx#L1390-L1436` and grouping fallback `ws-dashboard/frontend/src/App.tsx#L1438-L1463`.
- Read-only file placement currently routes through fixed `primary`/`support` state: see `ws-dashboard/frontend/src/App.tsx#L202-L230`, `ws-dashboard/frontend/src/App.tsx#L1794-L1806`, and grouping fallback `ws-dashboard/frontend/src/App.tsx#L1808-L1837`.
- Browser acceptance already opens a real daemon-served workRoot, opens a read-only file, creates terminals, uses platform-aware terminal commands, and writes evidence artifacts: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L192-L303` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L304-L555`.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L13-L31` — group/order/active types: use ordered `WorkbenchEditorGroupRef[]` plus `WorkbenchPaneOrder`/`WorkbenchActivePaneState` as the pure model surface.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L33-L46` — `WorkbenchDynamicGroupRequest` / `WorkbenchDynamicPaneMove`: adapter-facing contract for unknown Dockview split target mapping without storing raw handles.
- `ws-dashboard/frontend/src/workbench/policy.ts#L13-L54` — `WorkbenchPlacementState` / `WorkbenchDynamicPlacementDecision`: placement function should return a `nextState` and optional `createdGroupId`.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L35-L44` — `DockviewWorkbenchLayoutProps`: current `onMovePane` accepts only `(paneId, targetGroupId, beforePaneId?)`; it must carry dynamic target metadata somehow for newly created Dockview groups.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L186-L207` and `#L210-L235` — pane/tab DOM exposes `data-workbench-group-id` and `data-workbench-pane-id`; Playwright split-drop verification can assert these after synchronization.
- `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L399-L412` — reserved dynamic move test currently expects a throw; replace with success assertions for unknown target group creation and existing-group parity.
- `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L486-L725` — placement tests cover duplicate focus, existing opened/pinned policy, and reserved dynamic policy throws for group-2 creation, group-3 exclusion, and terminal group-1 preference.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L155-L166` — `expectDurableDockviewSplitDrop` placeholder documents the required browser evidence and suggested selectors/timing.
- `ws-dashboard/frontend/package.json#L4-L17` — acceptance scripts: `test:workbench`, `test:work-root-files`, `test:terminals`, `build`, and `test:browser` are already wired.

## Constraints
- `ai-docs/tickets/ready/260517-bug-ws-dashboard-dockview-dynamic-groups.md#L24-L41` — initial groups are dynamic `group 1`/`group 2`; terminals prefer group 1; files prefer/create group 2; groups 3+ are user-created and not automatic targets.
- `ai-docs/.plans/2026-05/17-260517-bug-ws-dashboard-dockview-dynamic-groups.brief.md#L17-L36` — Dockview remains visible owner; dashboard owns ids, pane order, active panes, placement, duplicate focus, close, and restore sanitization.
- `ai-docs/spec/ws-web-dashboard/index.md#L155-L200` — planned spec requires durable Dockview-created groups while preserving duplicate-open focus and dashboard-owned placement policy.
- `ai-docs/spec/ws-web-dashboard/index.md#L220-L233` — browser gate must prove split-drop durability after React synchronization, not just Dockview DOM presence.
- `ai-docs/mental-model/ws-web-dashboard.md#L7-L10` — visible browser UI changes require Playwright/browser-level verification; terminal work must avoid POSIX-only assumptions.
- `ai-docs/mental-model/ws-web-dashboard.md#L70-L82` — raw Dockview handles must stay inside the adapter; serialized layout must omit surface kind, row policy, daemon ids, and raw Dockview group handles.
- `ws-dashboard/frontend/src/App.tsx#L1001-L1023` — fresh terminal/read-only focus effects are sequence-guarded to avoid stealing tab focus during poll-driven rebuilds; preserve this behavior while changing groups.
- `ws-dashboard/frontend/src/App.tsx#L1495-L1792` — terminal pane owns WebSocket input/output, focus fallback, and resize forwarding; dynamic groups should not remount/churn terminals unnecessarily during ordinary output.

## Opinion
- `ws-dashboard/frontend/src/App.tsx#L1292-L1385` — the largest migration risk is App-level fixed group construction, not the pure helpers; a small browser-state group list shared by build/placement/order paths should reduce drift between terminal and file placement.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L128-L156` — adapter event mapping likely needs the narrowest API expansion: map unknown Dockview groups to generated dashboard ids before invoking product callbacks, while still passing only serializable dashboard metadata upward.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L155-L166` — Playwright drag/drop against Dockview overlays may be the flakiest part; keep assertions selector-based (`data-workbench-*`, Dockview owner marker) and settle past a poll cycle as the skeleton suggests.
