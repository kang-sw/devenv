# Survey: 260521-feat-ws-dashboard-command-dispatch-spine

## Reusable Components
- `ws-dashboard/frontend/src/App.tsx#L93-L102` — current `CommandPayload`/`CommandEntry`: command-log payload shape is local to `App.tsx`; extraction can preserve the existing recent-command log contract.
- `ws-dashboard/frontend/src/App.tsx#L372-L399` — `executeCommand`: current shared callback logs all command ids and already executes `select` and `refresh` payloads, but most migrated commands still execute through adjacent callbacks.
- `ws-dashboard/frontend/src/openWorkRoot.ts#L4-L24` — `requestOpenWorkRoot`: existing open-workRoot API helper posts the user-entered host path and returns the aggregated resource view; keep host path input local to this UI/API boundary rather than command payload identity.
- `ws-dashboard/frontend/src/workRootFiles.ts#L80-L110` — file API helpers: listing/read endpoints encode opaque `workRootId` plus relative paths and provide bounded fetch wrappers for explorer refresh/open behavior.
- `ws-dashboard/frontend/src/workRootFiles.ts#L112-L165` — read-only pane identity helpers: existing preview/pinned pane constructors and logical keys are the workbench-facing surface identities for `fileExplorer.openFile`.
- `ws-dashboard/frontend/src/workRootFiles.ts#L208-L240` — explorer state helpers: existing initial-load, expand-load, refresh-path, and toggle helpers support command parity tests without rendering the full app.
- `ws-dashboard/frontend/src/terminals.ts#L116-L130` — `createTerminal`: existing terminal-create helper posts default PTY dimensions; command dispatch can call this without touching terminal raw input.
- `ws-dashboard/frontend/src/terminals.ts#L183-L201` — terminal pane identity helpers: `terminalPaneLogicalKey`, `terminalPaneId`, and `terminalPaneFromSession` convert daemon sessions into workbench pane state using logical ids.
- `ws-dashboard/frontend/src/workbench/policy.ts#L169-L235` — `decideSurfaceOpenWithDynamicGroups`: shared placement/focus policy for terminals, read-only files, and WorkRoot Activity; useful for preserving duplicate-focus and group placement when command handlers invoke workbench actions.
- `ws-dashboard/frontend/src/workbench/editorGroupModel.ts#L187-L195` — `selectWorkbenchPane`: small reusable active-pane update helper already used by activity/terminal/workbench selection paths.

## Existing Patterns
- API wrapper modules live beside App and route tests: see `ws-dashboard/frontend/src/openWorkRoot.ts#L4-L24`, `ws-dashboard/frontend/src/workRootFiles.ts#L80-L110`, and `ws-dashboard/frontend/src/terminals.ts#L78-L130` — a command module can follow this small exported-type/helper pattern.
- Pure route/model tests are TypeScript files with inline assertions executed through package scripts: see `ws-dashboard/frontend/src/workRootFiles.test.ts#L20-L46` and `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts#L43-L77`.
- Command identities are already rendered as `data-command-id` selectors for browser coverage: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L318-L326`, `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L749-L778`, and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L962-L985`.
- WorkRoot Activity opener is currently a contained top-bar click plus placement-policy path: see `ws-dashboard/frontend/src/App.tsx#L1904-L1914`, `ws-dashboard/frontend/src/App.tsx#L1705-L1731`, and `ws-dashboard/frontend/src/App.tsx#L2311-L2325`.
- Workbench close/select/move paths are local functions inside `WorkbenchShell`: see `ws-dashboard/frontend/src/App.tsx#L1593-L1655`, `ws-dashboard/frontend/src/App.tsx#L1657-L1703`, and `ws-dashboard/frontend/src/App.tsx#L1835-L1863`.

## Relevant Interfaces
- `ai-docs/spec/ws-web-dashboard/index.md#L150-L164` — inspectable shell command contract: stable ids, logical dashboard payload targets, terminal raw-byte exception, and planned shared dispatch spine.
- `ai-docs/mental-model/ws-web-dashboard.md#L13-L16` — domain rules: visible UI changes need browser verification; controls must route clicks through command dispatch; terminal raw byte input is exempt.
- `ws-dashboard/frontend/src/App.tsx#L238-L360` — `openReadOnlyFile`: performs file pane placement, preview/pinned replacement, focus request, stale-read guard, and content/error fetch side effects.
- `ws-dashboard/frontend/src/App.tsx#L652-L817` — `WorkRootFileExplorer`: owns explorer snapshots, directory loading, selection, toggle/open/refresh side effects, and `onCommand` logging calls.
- `ws-dashboard/frontend/src/App.tsx#L890-L940` — `FileExplorerRow`: maps row kind/status to `fileExplorer.toggleDirectory`, `fileExplorer.openFile`, or `fileExplorer.selectEntry`; double-click pins previewable files.
- `ws-dashboard/frontend/src/App.tsx#L1422-L1448` — `createTerminalPane`: creates a daemon terminal, inserts pane state, updates terminal order via placement policy, and focuses the new pane.
- `ws-dashboard/frontend/src/App.tsx#L1693-L1731` — `selectPane` and `openWorkRootActivityPane`: select/focus behavior and activity duplicate-open/focus behavior to preserve if command-routed.
- `ws-dashboard/frontend/src/App.tsx#L3330-L3358` — `ViewerReserve` command log renderer: current observable log shows recent command id plus label.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L99-L108` — WorkRoot Activity registry entry: reversible daemon projection, opened row, release-only close, no confirmation.
- `ws-dashboard/frontend/package.json#L6-L17` — available frontend verification commands and existing targeted test script pattern.

## Constraints
- `ws-dashboard/frontend/src/openWorkRoot.ts#L8-L17` — `workRoot.open` currently needs a host path for the daemon request, but the brief forbids host paths in command payloads; the command boundary needs a UI-local way to resolve/execute without logging that path as command identity.
- `ws-dashboard/frontend/src/App.tsx#L897-L914` — file explorer row semantics are compact and conventional: directory toggles, previewable file opens, all other entries select.
- `ws-dashboard/frontend/src/App.tsx#L938-L940` — double-click on previewable file rows uses the same row command id but changes gesture to pinned mode; parity coverage should not accidentally erase the single-click preview versus double-click pin distinction.
- `ws-dashboard/frontend/src/App.tsx#L267-L330` — pinned duplicate file opens focus existing panes and remove matching previews before returning; command execution should preserve this early-return/focus behavior.
- `ws-dashboard/frontend/src/App.tsx#L335-L360` — read-only file fetch completion is guarded against stale preview requests; command migration should not bypass the guard.
- `ws-dashboard/frontend/src/App.tsx#L1552-L1568` and `ws-dashboard/frontend/src/App.tsx#L2868-L2900` — terminal input/close remain direct terminal surfaces; raw `terminal.input` is data-id only for the xterm surface and should stay outside dashboard dispatch.
- `ws-dashboard/frontend/src/App.tsx#L1638-L1655` and `ws-dashboard/frontend/src/App.tsx#L1810-L1813` — workbench close uses a two-step confirmation path for session-backed panes; broad close migration may be larger than the command-spine slice.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L13-L24` — browser gate is the evidence home for visible UI/workbench changes; pure tests alone may not satisfy UI-facing contract if rendered behavior changes.

## Risk Signals
- `ws-dashboard/frontend/src/App.tsx#L512-L528` — Possible contract risk: `workRoot.open` calls `onCommand` before directly posting the host path; moving this into command dispatch must avoid placing the path in `CommandPayload` while still making programmatic dispatch executable.
- `ws-dashboard/frontend/src/App.tsx#L759-L817` — Possible reuse risk: file explorer actions interleave snapshot updates, command logging, loading, and file-open callbacks; a command spine that only wraps `onCommand` would leave side effects split beside the dispatch path.
- `ws-dashboard/frontend/src/App.tsx#L1904-L1914` and `ws-dashboard/frontend/src/App.tsx#L1943-L1954` — Possible shortcut risk: Activity and terminal toolbar controls currently call `onCommand` and then adjacent direct side-effect callbacks; these are explicit parity targets for shared dispatch.
- `ws-dashboard/frontend/src/App.tsx#L1593-L1703` — Possible scope risk: workbench close/select/move paths are stateful and confirmation-sensitive; migrating them wholesale could dominate Phase 1, so an implementation note may be safer if containment is poor.
- `ws-dashboard/frontend/src/App.tsx#L93-L96` — Possible contract risk: current payload union only carries select/action/refresh and cannot describe required logical targets like `workRootId`, `paneId`, `terminalId`, relative file path, activity ids, or later activity command ids.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L1129-L1143` — Possible test risk: the browser test intentionally focuses the `terminal.create` button after terminal input churn; changing terminal-create dispatch/focus behavior could break this focus-stealing guard.

## Opinion
- `ws-dashboard/frontend/src/App.tsx#L372-L399` — The current `executeCommand` is a useful seed but not a complete dispatcher; most implementation uncertainty is in keeping side-effect closures near the state they mutate while exporting a programmatic dispatch API.
- `ws-dashboard/frontend/src/App.tsx#L1835-L1863` — Workbench close controls already expose command ids but are confirmation/lifecycle paths, so the brief's audit-only allowance matches the codebase shape.
- `ws-dashboard/frontend/src/terminalCommandPlan.ts#L1-L1` — Search found no existing frontend command registry module; `terminalCommandPlan.ts` is unrelated shell command-profile logic despite its name.
