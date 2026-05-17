# Survey: 260517-feat-ws-dashboard-workroot-activity Phase 3

## Reusable Components
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L1-L39` — `SurfaceKind`/registry policy types: central enum and lifecycle/close policy schema for adding a distinct WorkRoot Activity surface kind.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L41-L126` — `defaultSurfaceRegistryEntries`: existing daemon-projection reversible surfaces (`viewer`, `diagnostics`, `eventsLog`, `taskView`) already use `rowPolicy: "opened"`, `lifecycleOwner: "daemonProjection"`, `closePolicy: "releaseProjection"`, and `closeConfirmationPolicy: "none"`.
- `ws-dashboard/frontend/src/workbench/policy.ts#L126-L167` — `surfaceLogicalKey` and `decideSurfaceOpen`: duplicate-open focus behavior is already keyed by logical surface key and returns the existing attachment/group.
- `ws-dashboard/frontend/src/workbench/policy.ts#L169-L225` — `decideSurfaceOpenWithDynamicGroups`: dynamic placement/focus helper used by App; it already returns `nextState` and created-group metadata for caller state updates.
- `ws-dashboard/frontend/src/workbench/policy.ts#L249-L293` — `decideSurfaceCloseConfirmation`/`decideWorkbenchTabClosePresentation`: reversible surfaces close immediately with no cursor-near popover when registry confirmation policy is `none`.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L163-L196` — `reconcileActiveWorkbenchPanes` and `selectWorkbenchPane`: reusable active-pane state updates for focusing the Activity pane after open or duplicate-open.
- `ws-dashboard/frontend/src/workbench/placementGroups.ts#L3-L25` — `reconcileDashboardGroupsForPlacement`: persists policy-created dynamic groups into App-owned group state while preserving existing labels.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L19-L58` — `DockviewWorkbenchPane`/`DockviewWorkbenchGroup`: pane body metadata shape already supports custom React bodies and close callbacks.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L225-L247` — `DockviewWorkbenchPanel`: renders `data-surface-kind`, `data-workbench-group-id`, and `data-workbench-pane-id` markers used by browser tests.
- `ws-dashboard/frontend/src/workRootActivity.ts#L5-L46` — `WorkRootActivityView` and agent/call types: frontend mirror of daemon projection fields available for the detail pane.
- `ws-dashboard/frontend/src/workRootActivity.ts#L48-L62` — `fetchWorkRootActivity`: existing protected daemon API helper to reuse; it encodes opaque workRoot ids and surfaces bounded API errors.
- `ws-dashboard/frontend/src/workRootActivity.ts#L66-L160` — `workRootActivityBadge`: existing badge formatter must remain compact summary-only while the badge becomes clickable.

## Existing Patterns
- App-owned per-root workbench state: see `ws-dashboard/frontend/src/App.tsx#L1029-L1075` — `activePaneByRoot`, `workbenchGroupsByRoot`, and `paneOrderByRoot` are keyed by selected workRoot and should be mirrored by any Activity pane order/visibility state.
- Selected-root activity fetch state: see `ws-dashboard/frontend/src/App.tsx#L1059-L1192` — badge fetch state carries `rootId` to avoid rendering stale prior-root data during switches; the detail pane can reuse the same helper but must guard selected/root ownership.
- Read-only file open/focus placement: see `ws-dashboard/frontend/src/App.tsx#L2544-L2568` and `ws-dashboard/frontend/src/App.tsx#L1285-L1310` — creates placement state from existing panes/orders, places panes, then uses a sequence-guarded focus request.
- Terminal group-1 dynamic placement: see `ws-dashboard/frontend/src/App.tsx#L1956-L2032` — `placeTerminalSessions` uses `persistentTerminal` policy and explicit `group-1` fallback, useful precedent for Activity's required group-1 exception.
- Building visible groups from per-surface buckets: see `ws-dashboard/frontend/src/App.tsx#L1896-L1954` — `buildWorkbenchEditorGroups` merges agent, terminal, and read-only panes into each dashboard group; Activity panes will need another bucket in this merge.
- Pane close routing in App: see `ws-dashboard/frontend/src/App.tsx#L1488-L1571` — close requests are policy-decided by surface kind, then App removes the backing pane state and reconciles active panes.
- Toolbar badge location: see `ws-dashboard/frontend/src/App.tsx#L1733-L1858` — `WorkbenchToolbar` renders the badge inside `.workbench-toolbar-meta`; click handling should be added without breaking the current row structure.
- Read-only pane body style pattern: see `ws-dashboard/frontend/src/App.tsx#L2648-L2716` and `ws-dashboard/frontend/src/styles.css#L1333-L1370` — specialized pane bodies own their header/scroll region and suppress the shared detail paragraph via surface-specific CSS.
- Browser acceptance style: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L403-L496` — existing Phase 2 badge assertions already locate `.workbench-activity-badge` and measure top-bar height at 1440px/480px.
- Browser close/focus evidence: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L613-L703` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1147-L1178` — reversible file tabs close immediately; terminal/agent tabs show cursor-near confirmation.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L249-L307` — `DockviewWorkbenchTab`: tab attributes expose pane kind/category/group and the hover-only close button; Activity browser evidence can assert these markers.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L309-L423` — `syncDockviewWorkbench`: dashboard group order maps to Dockview groups; Activity group placement must flow through `groups`/`activePaneByGroup`, not raw Dockview handles.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L456-L473` — `toDockviewWorkbenchPanelParams`: all pane metadata/body updates enter Dockview through params.
- `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L635-L843` — existing placement tests cover duplicate focus, group-2 editor placement, and policy-created group persistence; add Activity cases near these.
- `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L948-L1042` — existing tests prove terminal group-1 dynamic placement and reversible immediate-close presentation.
- `ws-dashboard/crates/core/src/activity.rs#L5-L62` — daemon/core activity JSON contract: workRoot id, summary, agent metadata, current-call status/timing, detail hints, diagnostics; deliberately omits host internals.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L56-L94` — route handler/projector: browser calls resolve only opened workRoot ids; projection runs cache/Git work on blocking pool.
- `ws-dashboard/frontend/package.json#L15-L15` — `npm run test:work-root-activity` runs the frontend route/helper formatter test entrypoint.
- `ai-docs/spec/ws-web-dashboard/index.md#L119-L160` — workbench substrate spec: duplicate-open focus, dynamic groups, immediate close for reversible views, Dockview ownership, group-1 terminal and group-2 editor policies.
- `ai-docs/spec/ws-web-dashboard/index.md#L183-L214` — WorkRoot Activity projection, badge, and planned pane behavior including group-1 exception.
- `ai-docs/tickets/ready/260517-feat-ws-dashboard-workroot-activity.md#L111-L124` — Phase 3 ticket contract: reversible pane, group 1, duplicate focus, close behavior, no running-command rows.

## Constraints
- WorkRoot Activity remains daemon-owned and read-only; browser code must call `/api/dashboard/work-roots/{workRootId}/activity` and must not read wsstate/cache paths directly (`ai-docs/mental-model/ws-web-dashboard.md#L80-L84`, `ws-dashboard/frontend/src/workRootActivity.ts#L48-L62`).
- The API and UI must not expose host cache paths, stream paths, pids, session ids, or process/control actions; the core serializer test explicitly guards forbidden strings (`ws-dashboard/crates/core/src/activity.rs#L70-L134`).
- Generic opened/support surfaces currently auto-place into group 2; Activity needs an explicit exception without changing editor/read-only placement or group-3+ preservation (`ws-dashboard/frontend/src/workbench/policy.ts#L169-L181`, `ws-dashboard/frontend/src/workbench/policy.ts#L321-L349`).
- `SurfaceKind` is a closed TypeScript union, so adding Activity ripples through registry entries, icon CSS, tests that assert the stable kind list, and any exhaustive surface-kind usage (`ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L1-L10`, `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L81-L95`).
- Existing toolbar CSS depends on `.workbench-toolbar-meta` staying nowrap/hidden and the activity secondary text hiding at <=560px; clickability must not add a wrapper that breaks these measurements (`ws-dashboard/frontend/src/styles.css#L548-L605`, `ws-dashboard/frontend/src/styles.css#L869-L876`, `ws-dashboard/frontend/src/styles.css#L1168-L1173`).
- Browser verification must run against daemon-served production frontend for visible UI changes; pure TS/build tests are insufficient (`ai-docs/mental-model/ws-web-dashboard.md#L14-L16`, `ai-docs/spec/ws-web-dashboard/index.md#L346-L379`).
- Running Commands should remain absent or explicitly empty until the async exec job source lands (`ai-docs/tickets/todo/260513-feat-async-exec-output-reader.md#L18-L31`).

## Opinion
- Highest implementation risk is placement policy: treating Activity as ordinary `opened` would put it in group 2, while treating it as `pinned` would alter row/category semantics; a small explicit helper or registry policy extension is safer than overloading existing row policy.
- App.tsx is already dense and surface-specific state is split across terminal/read-only files; keeping the Activity pane model small and pure in `workRootActivity.ts` would reduce additional coupling.
- Browser evidence can reuse existing badge and tab close selectors, but deterministic non-empty agent rows may be harder than empty/no-agent detail; the brief only requires visible empty/no-agent detail, so avoid inventing daemon fixtures unless needed.
