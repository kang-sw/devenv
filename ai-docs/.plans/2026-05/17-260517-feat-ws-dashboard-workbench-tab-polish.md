# Survey: 260517-feat-ws-dashboard-workbench-tab-polish

## Reusable Components
- `ws-dashboard/frontend/src/workbench/policy.ts#L249-L293` — `decideSurfaceCloseConfirmation` / `decideWorkbenchTabClosePresentation`: skeleton close-confirmation decision helpers already encode terminal/agent cursor-near `Yes`/`No` versus immediate reversible closes.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L41-L114` — `defaultSurfaceRegistry`: source of surface kind labels, pinned/opened row policy, close policy, and confirmation policy for tab rendering and close routing.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L35-L57` — `DockviewTabCloseRequest` / `DockviewWorkbenchLayoutProps.onRequestClosePane`: adapter seam for tab close requests with dashboard group/pane ids and pointer coordinates.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L246-L279` — `DockviewWorkbenchTab`: only visible Dockview tab renderer; already emits stable category, close-affordance, confirmation-policy, group, and pane data attributes.
- `ws-dashboard/frontend/src/workbench/policy.ts#L169-L225` — `decideSurfaceOpenWithDynamicGroups`: dynamic placement helper for duplicate focus, group-2 editor/file placement, group-1 terminal placement, and preserving group 3+ as user-created.
- `ws-dashboard/frontend/src/workRootFiles.ts#L33-L50` — `ReadOnlyFilePane` mode fields: skeleton preview/pinned mode contract is already represented on pane state.
- `ws-dashboard/frontend/src/workRootFiles.ts#L112-L166` — read-only pane identity helpers: single-click maps to `preview`, double-click to `pinned`, preview keys are one-per-workRoot, and pinned keys include path.
- `ws-dashboard/frontend/src/App.tsx#L220-L283` — `openReadOnlyFile`: current file-open pipeline places, focuses, fetches content, and records read-only pane state; this is the main place to thread preview versus pinned gesture.
- `ws-dashboard/frontend/src/App.tsx#L953-L968` — `setActivePaneByGroupForSelected`: scoped active-pane update helper for deterministic focus handoff under the selected workRoot.
- `ws-dashboard/frontend/src/App.tsx#L1316-L1332` — `closeTerminalPane`: existing daemon-backed terminal close/terminate route and error marking, reusable as the terminal tab-close confirm action.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L163-L195` — `reconcileActiveWorkbenchPanes` / `selectWorkbenchPane`: existing active-pane fallback and selection helpers for close-after-focus and cancel focus coherence.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L221-L287` — `commitWorkbenchPaneMoveIntoDynamicGroup`: existing Dockview move reconciliation preserves dynamic groups, order, and active pane state.
- `ws-dashboard/frontend/src/styles.css#L1-L56` — dashboard semantic token layer for tab accents, badges, hover-only controls, and popover styling.

## Existing Patterns
- Dockview adapter boundary: see `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L132-L198` — Dockview events are reduced to dashboard pane/group ids; raw Dockview handles stay inside the adapter.
- Dockview sync/update pattern: see `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L282-L392` — panel add/move/remove and active-panel sync happen from dashboard groups, with `syncingRef` suppressing feedback loops.
- Terminal focus request guard: see `ws-dashboard/frontend/src/App.tsx#L1158-L1184` — one-shot sequence refs prevent repeated focus steals after terminal output or model rebuilds.
- Read-only file focus request guard: see `ws-dashboard/frontend/src/App.tsx#L1131-L1156` — same sequence pattern already focuses file panes exactly once per open request.
- Workbench model assembly still creates default panes: see `ws-dashboard/frontend/src/App.tsx#L1587-L1673` — current opened workRoot always gets `main-agent`, `selected-viewer`, `editor-detail`, `task-view`, `diagnostics-events`, and `inspector`, which is the likely source of fake/default tab behavior.
- File explorer row command pattern: see `ws-dashboard/frontend/src/App.tsx#L777-L839` — each row is a single button with a `data-command-id`; adding double-click pinning should preserve row command identity and conventional tree behavior.
- Terminal controls currently bypass tab-close policy: see `ws-dashboard/frontend/src/App.tsx#L2171-L2192` — pane-local `Terminate` button calls `actions.onClose` immediately and is separate from the requested tab close affordance.
- Browser evidence notes already reserve this scope: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L20-L24` — acceptance gate comments list hover close, popover cancel/confirm, immediate close, tab presentation, and preview-to-pinned coverage.
- Existing Playwright Dockview selectors: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L193-L201` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L204-L270` — stable owner marker and `.dockview-workbench-tab` selectors are used for Dockview assertions.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L19-L33` — `DockviewWorkbenchPane` / `DockviewWorkbenchGroup`: tab metadata fields available to render category badges, labels, detail, meta, and pane bodies.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L74-L84` — `dockviewTabCategoryPresentation`: skeleton fallback/category-presentation helper; currently returns pinned-left fallback for pinned tabs and chip for opened tabs.
- `ws-dashboard/frontend/node_modules/dockview-core/dist/esm/api/component.api.d.ts#L585-L614` — Dockview tab-group API: `createTabGroup`, `addPanelToTabGroup`, `removePanelFromTabGroup`, `getTabGroups`, `getTabGroupForPanel`, and `moveTabGroup` exist if native chips/groups are attempted.
- `ws-dashboard/frontend/node_modules/dockview-core/dist/esm/dockview/framework.d.ts#L17-L21` — `IDockviewPanelHeaderProps`: custom tab renderer receives `api`, `containerApi`, `params`, and `tabLocation`.
- `ws-dashboard/frontend/src/App.tsx#L1545-L1560` — `WorkbenchPane` / `WorkbenchEditorGroupModel`: app-level pane shape lacks close callbacks today; close routing can be keyed by pane id/kind from built groups.
- `ws-dashboard/frontend/src/App.tsx#L2207-L2230` — `readOnlyFilePlacementState`: maps read-only panes into placement-state attachments; it currently rebuilds logical keys as pinned `editor/workRoot/path`, not mode-aware preview keys.
- `ws-dashboard/frontend/src/App.tsx#L2250-L2282` — `readOnlyWorkbenchPanesByGroup`: groups read-only panes by recorded order, with group-2 fallback for unconsumed panes.
- `ws-dashboard/frontend/src/App.tsx#L2284-L2310` — `readOnlyWorkbenchPane`: maps read-only pane state into workbench `editor` opened pane metadata and body.
- `ws-dashboard/frontend/src/App.tsx#L1691-L1735` — `placeTerminalSessions`: terminal sessions route through dynamic placement and append pane ids into terminal order.
- `ws-dashboard/frontend/src/App.tsx#L1769-L1798` — `terminalWorkbenchPanesByGroup`: maps daemon terminal panes into pinned workbench panes by terminal order and group fallback.
- `ws-dashboard/frontend/package.json#L6-L17` — scripts: required gates are already exposed as `test:workbench`, `test:work-root-files`, `test:terminals`, `build`, and `test:browser`.

## Constraints
- `ai-docs/mental-model/ws-web-dashboard.md#L154-L160` and `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L132-L138` require raw Dockview handles to remain adapter-private.
- `ai-docs/spec/ws-web-dashboard/index.md#L201-L207` and `ws-dashboard/frontend/src/workbench/policy.ts#L174-L181` require files/editors to prefer/create group 2, terminals to prefer group 1, and group 3+ to remain user-created.
- `ai-docs/mental-model/ws-web-dashboard.md#L200-L207` warns against global group/pane/active state; App already scopes `workbenchGroupsByRoot`, `paneOrderByRoot`, and `activePaneByRoot` by workRoot.
- `ws-dashboard/frontend/src/workbench/dockviewLayout.tsx#L413-L423` avoids Dockview parameter churn for connected terminals; tab polish should not re-render terminals on every output/socket metadata tick.
- `ai-docs/spec/ws-web-dashboard/index.md#L531-L542` requires terminal tab close confirmation to preserve close-as-terminate after confirm.
- `ws-dashboard/frontend/DESIGN.md#L15-L22` and `ws-dashboard/frontend/DESIGN.md#L84-L95` require dark, square, dense, semantic-token UI and low-height badges/chips.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L89-L95` requires external browser gates to declare terminal shell profile/platform so local Playwright does not send POSIX commands to a remote Windows daemon.

## Opinion
- Highest-risk mismatch is that `buildWorkbenchEditorGroups` still manufactures multiple default opened panes for every workRoot; removing fake/default tabs will likely require changing model assembly and tests before close UI can be meaningfully verified.
- The skeleton close helpers are pure and tested, but no production close path is wired from `DockviewWorkbenchTab` to `App` yet; implementer should expect to add state for pending close confirmation and actual pane removal for reversible/read-only panes.
- Preview identity exists in `workRootFiles.ts`, but App currently always creates pinned/default read-only panes and computes placement logical keys without mode, so Phase 3 is mostly an App wiring/state-replacement task.
- Dockview native tab groups are available in installed types, but there is no existing project pattern using them; pinned-left badge fallback is lower-risk unless native chips can be proven quickly inside `dockviewLayout.tsx` without leaking Dockview handles.
- A doc wording trap exists: `ai-docs/spec/ws-web-dashboard/index.md#L177-L182` says panel close detaches daemon resources by default, while this brief and terminal close spec require terminal tab confirm to proceed with terminate behavior.
