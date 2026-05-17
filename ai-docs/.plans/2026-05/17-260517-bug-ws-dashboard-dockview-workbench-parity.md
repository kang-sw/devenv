# Survey: 260517-bug-ws-dashboard-dockview-workbench-parity

## Reusable Components
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L54-L143` — `DockviewWorkbenchLayout`: existing skeleton mounts `DockviewReact` under `data-workbench-layout-owner="dockview"` and routes active-panel/move events back as dashboard pane/group ids.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L145-L186` — `DockviewWorkbenchPanel` / `DockviewWorkbenchTab`: current Dockview panel and tab renderers already preserve workbench pane markers, surface-kind markers, and custom tab text.
- `ws-dashboard/frontend/src/workbench/dockviewBridge.ts#L10-L57` — bridge types/options: preserves the dashboard-owned handle boundary and disables floating groups without disabling Dockview tab/group movement.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L43-L88` — order helpers: apply/derive stable pane order while appending newly discovered panes, useful for keeping dashboard state authoritative across Dockview sync.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L90-L206` — movement/active helpers: `commitWorkbenchPaneMove`, `selectWorkbenchPane`, and active reconciliation retain cross-split membership and omit stale empty-group active ids.
- `ws-dashboard/frontend/src/workbench/policy.ts#L98-L124` — `decideSurfaceOpen`: duplicate logical-key focus and group placement policy for opened vs pinned surfaces.
- `ws-dashboard/frontend/src/App.tsx#L1701-L1769` — read-only file pane adapters: filter panes by owning workRoot, keep support-group placement, and provide existing `ReadOnlyTextPane` body nodes.
- `ws-dashboard/frontend/src/App.tsx#L1429-L1481` — terminal pane adapters: filter terminal panes by owning workRoot, preserve terminal placement/order, and provide existing keyed `TerminalPaneBody` nodes.
- `ws-dashboard/frontend/src/App.tsx#L1484-L1688` — `TerminalPaneBody`: xterm/WebSocket/input/resize/close body implementation that should be preserved and only adapted for Dockview sizing if needed.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L146-L153` — `expectDockviewWorkbench`: existing browser helper asserts the Dockview owner marker, Dockview DOM, and absence of retired custom split shell.

## Existing Patterns
- Workbench model assembly: see `ws-dashboard/frontend/src/App.tsx#L846-L873` and `ws-dashboard/frontend/src/App.tsx#L1283-L1376` — `WorkbenchShell` builds two logical groups from resources, file panes, and terminal panes before passing them to Dockview.
- Fresh file/terminal focus uses one-shot request sequence guards: see `ws-dashboard/frontend/src/App.tsx#L980-L1022` — active pane updates must not reassert on every terminal poll/resource rebuild.
- Terminal session restore is daemon-list driven with stale-list protection: see `ws-dashboard/frontend/src/App.tsx#L877-L890` and `ws-dashboard/frontend/src/terminals.ts#L204-L235` — live daemon sessions are merged without pruning locally new sessions from older list responses.
- Browser gate is daemon-served production UI, not unit-only: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L179-L541` — existing steps already cover owner pairing, workRoot open, file preview, terminal WebSocket/input/sizing, refresh restore, and narrow viewport.
- Workbench route-test style is executable TypeScript assertions: see `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L397-L467` — bridge tests use a fake Dockview port and assert dashboard handles do not expose raw Dockview objects.
- Dockview public API supports incremental operations: see `ws-dashboard/frontend/node_modules/dockview-core/dist/esm/api/component.api.d.ts#L463-L532` — API exposes `panels`, `groups`, `getPanel`, `addPanel`, `removePanel`, `addGroup`, `removeGroup`, `fromJSON`, `toJSON`, and `clear`.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L16-L37` — `DockviewWorkbenchPane`, `DockviewWorkbenchGroup`, `DockviewWorkbenchLayoutProps`: adapter contract between dashboard policy/model state and Dockview.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L188-L241` — `syncDockviewWorkbench` / params mapper: current rebuild-only sync clears all Dockview panels, causing the main implementation hotspot for preserving mounted terminal bodies during ordinary selection.
- `ws-dashboard/frontend/src/workbench/dockviewBridge.ts#L18-L57` — `DockviewBridgePort`, `WorkbenchDockviewBridge`: existing bridge test surface if adapter responsibilities or fake-port coverage need expansion.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L1-L32` — surface kind, row policy, lifecycle owner, and close policy vocabulary referenced by tabs, placement, and close behavior.
- `ws-dashboard/frontend/src/workbench/policy.ts#L24-L49` — `WorkbenchPlacementState` and open decisions: source for preserving duplicate focus and primary/support group choice.
- `ws-dashboard/frontend/src/workRootFiles.ts#L33-L46` and `ws-dashboard/frontend/src/workRootFiles.ts#L108-L147` — `ReadOnlyFilePane` identity/content helpers: file logical key and pane id are workRoot-relative and should remain the duplicate-open boundary.
- `ws-dashboard/frontend/src/terminals.ts#L44-L53`, `ws-dashboard/frontend/src/terminals.ts#L183-L201` — `TerminalPaneState`, logical key, and pane id helpers: terminal pane ids are keyed by daemon terminal id and used by xterm remounting.
- `ws-dashboard/frontend/package.json#L6-L18` — required verification scripts: `test:workbench`, `test:work-root-files`, `test:terminals`, `build`, and `test:browser`.

## Constraints
- `ai-docs/tickets/ready/260517-bug-ws-dashboard-dockview-workbench-parity.md#L65-L74` — Dockview must own visible groups/tabs/panes, while dashboard still owns surface identity, duplicate focus, placement, close policy, and restore sanitization.
- `ai-docs/mental-model/ws-web-dashboard.md#L144-L147` — browser-visible dashboard changes require browser-level verification against the daemon-served production frontend.
- `ai-docs/mental-model/ws-web-dashboard.md#L88-L89` — read-only file panes must route through daemon-authorized file reads, focus duplicates, and render only under the owning workRoot.
- `ai-docs/mental-model/ws-web-dashboard.md#L93-L104` — terminal changes must preserve WebSocket-first I/O, suppress HTTP polling while connected, keep xterm keyed per pane, and retain resize/input fidelity.
- `ai-docs/mental-model/ws-web-dashboard.md#L110-L114` — workbench serialized layout must not persist daemon ids, surface kind metadata, or registry-derived row policy.
- `ws-dashboard/frontend/src/App.tsx#L1801-L2021` and `ws-dashboard/frontend/src/styles.css#L866-L986` — old `WorkbenchEditorGroup` / `WorkbenchTabLane` and `.workbench-splits`/`.workbench-group` CSS still exist as dead custom layout authority candidates; browser acceptance is meant to fail if this path returns visibly.
- `ws-dashboard/frontend/src/App.tsx#L1538-L1581` and `ws-dashboard/frontend/src/styles.css#L1257-L1277` — terminal fit measures `.terminal-surface` and `.xterm-screen`; any Dockview pane padding/overflow changes can clip the active bottom row.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L443-L462` — browser terminal fill assertion currently locates `.workbench-pane[data-surface-kind="persistentTerminal"] .workbench-pane-body`; marker/class changes must keep this assertion aligned with the Dockview panel wrapper.

## Opinion
- Main risk is remount churn: the skeleton's `api.clear()` rebuild in `syncDockviewWorkbench` is acceptable for a compile-clean owner marker but conflicts with the brief's requirement not to remount terminal xterm surfaces during ordinary tab selection.
- Dockview move events currently only pass `params.groupId` from stale panel params (`ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L107-L115`); implementer should confirm how to map the current `panel.group.id` back to dashboard group ids after user Dockview moves.
- The old custom workbench functions are no longer referenced by the render path, but leaving them in `App.tsx` plus their CSS increases regression risk and import clutter (`partitionWorkbenchPanesByCategory`, `resolveWorkbenchPaneDrop`, `workbenchPaneDragMimeType`, `defaultSurfaceRegistry`, `draggedPaneId`).
- The existing browser test already carries substrate assertions, but it should be strengthened around file + terminal tab switching under the Dockview owner so a custom-shell fallback or Dockview-less pane body cannot pass by coincidental DOM text.
