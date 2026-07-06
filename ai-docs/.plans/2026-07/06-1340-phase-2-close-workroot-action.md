# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 2: Explicit "close work root" action

## Relevant Ticket Contract

- "Add a close affordance for open work roots in the left panel. Triggering it
  runs the current (pre-Phase-1) destructive teardown for that root's panes
  only: dockview panel removal, xterm dispose, socket close, and any
  `terminalPanes`/`workbenchGroupsByRoot`/`paneOrderByRoot` entries scoped to
  that root. Switching among still-open roots must not trigger this path."
- Phase 1 Result (already landed on this branch): per-visited-root state is
  `openWorkRootKeys`/`openWorkRootRefs` (append-only membership list keyed by
  `serverScopedIdentity(serverRoute, rootId)`) plus a `findOpenWorkRoot`
  resolver, currently all local to `WorkbenchShell`
  (`ws-dashboard/frontend/src/App.tsx:3284-3287`). Removing a root's key from
  `openWorkRootKeys` is what stops its `DockviewWorkbenchLayout` instance from
  rendering (`ws-dashboard/frontend/src/App.tsx:4643-4661`), which is the
  actual mechanism that unmounts dockview + all `TerminalPaneBody` instances
  for that root and fires their existing dispose/socket-close cleanup — no new
  dispose/close code is needed, only wiring to stop rendering the instance.
- Spec Impact for this phase: "Contract-first: no" — internal lifecycle
  change only, no browser-visible persisted contract, no spec update in this
  phase.

## Out of Scope

- Phase 3 (socket visibility-gating for still-open roots), Phase 4 (cursor
  accuracy/gap signaling), Phases 5-7 (layout/terminal-visual persistence and
  reuse on reopen). Do not implement snapshot-and-restore-on-reopen; Phase 2
  reopening a closed root today starts from a fresh/blank workbench, per
  ticket sequencing (Phase 7 is what wires the shared restore primitive back
  into this close path).
- Do not call the daemon `closeTerminal()` API (`closeTerminal` in
  `ws-dashboard/frontend/src/terminals.ts`, used by `closeTerminalPane` at
  `ws-dashboard/frontend/src/App.tsx:4365-4384`). That kills the PTY session
  server-side. The ticket's "socket close" for this phase is the browser-side
  WebSocket close in `TerminalPaneBody`'s cleanup effect
  (`ws-dashboard/frontend/src/App.tsx:6398`), not a daemon terminal
  termination — the daemon terminal must stay alive so a future reopen can
  reattach by id (matches the ticket's `WorkRoot IO Restore Model` framing:
  browser state is not authoritative over daemon state).
- Do not touch `hasWorkspaceRemove`/`workspace.remove` behavior
  (`ws-dashboard/frontend/src/App.tsx:825-902`); it is a different, more
  destructive action (removes the workspace from the dashboard entirely) and
  already has its own confirmation flow.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:3284-3287` — `openWorkRootKeys`
  (`string[]`) and `openWorkRootRefs` (`Record<string, {rootId, serverRoute}>`)
  are local `useState` inside `WorkbenchShell`, populated by an effect at
  `App.tsx:3348-3363` keyed on `selectedWorkRootStateKey`/`selection`.
- `ws-dashboard/frontend/src/App.tsx:3554-3577` (`openWorkRootInstances`) —
  derives the render list by mapping `openWorkRootKeys` through
  `findOpenWorkRoot`; a rootKey absent from `openWorkRootKeys` is silently
  skipped, i.e. its `.workbench-root-instance` div and
  `DockviewWorkbenchLayout` simply stop rendering.
- `ws-dashboard/frontend/src/App.tsx:4643-4661` — the actual render loop that
  mounts one `DockviewWorkbenchLayout` per open root; removing a key from
  `openWorkRootKeys` unmounts this subtree, which is the reuse target for
  "dockview panel removal, xterm dispose, socket close" (no new
  dispose/close call needed).
- `ws-dashboard/frontend/src/App.tsx:6058-6398` (`TerminalPaneBody`) — mount
  effect keyed `[terminalId]`; cleanup at `~6323-6398` disposes the xterm
  instance and closes the WebSocket (`socket.close()` at line 6398). This
  fires automatically on unmount regardless of cause, confirming reuse via
  unmount is sufficient and no explicit call is required.
- **Cross-boundary risk (non-obvious constraint):** the left panel (aside
  `.shell-panel-nav`, `App.tsx:957-978`, component `ResourceNavigation` at
  `App.tsx:2494`) is rendered by `App()`, a sibling of `WorkbenchShell`, not a
  descendant. `openWorkRootKeys`/`openWorkRootRefs` currently live only inside
  `WorkbenchShell`, so the left panel cannot today know which roots are open
  or trigger their close. `App()` already lifts the structurally analogous
  per-root state (`workbenchGroupsByRoot`, `paneOrderByRoot`,
  `readOnlyFilePanes`) to itself and passes it into `WorkbenchShell` as
  controlled props + `Dispatch<SetStateAction<...>>` setters
  (`App.tsx:379-383`, `369-378`, passed at `App.tsx:1006-1026`). The same
  lift-to-`App()` pattern is the established, minimal-risk way to give the
  left panel read/close access to `openWorkRootKeys`/`openWorkRootRefs`.
- `ws-dashboard/frontend/src/App.tsx:787-902` (`executeCommand`) — App-level
  command dispatcher already owns exactly this kind of destructive,
  cross-cutting-state cleanup for `workspace.remove`: it deletes scoped
  entries from `paneOrderByRoot`/`workbenchGroupsByRoot`/`readOnlyFilePanes`
  keyed by `serverScopedIdentity(serverRoute, root.id)`. `workRoot.close`
  should follow the exact same shape inside `executeCommand`.
- `ws-dashboard/frontend/src/commands.ts:2-105` — command catalog; add
  `"workRoot.close"` to `DashboardCommandId` and a
  `{ type: "workRoot.close"; workRootId: string }` variant to
  `DashboardCommandPayload` (serverRoute already carried generically via
  `DashboardCommand.payload.serverRoute`, `commands.ts:109`). Add
  `buildWorkRootCloseCommand(workRootId, serverRoute)` mirroring
  `buildWorkspaceRemoveCommand` (`commands.ts:343-350`). Add a
  `dashboardCommandLabel` case and extend `commands.test.ts` following the
  existing `workspace.remove` test block (`commands.test.ts:36,86,127,170,
  246,468-489`).
- `ws-dashboard/frontend/src/App.tsx:7071-7179` (`WorkspaceRows`) and
  `App.tsx:7185-7325` (`ResourceRow`) — the left-panel row components for
  workspaces/work roots. `ResourceRow` already has a proven pattern for a
  destructive per-row action: `hasWorkspaceRemove` renders a `ChromeIconButton`
  (`icon={MoreHorizontal}`) opening an overflow menu with a `Trash2`-iconed
  danger item (`App.tsx:7263-7322`). For work-root close, a direct `X`-iconed
  `ChromeIconButton` (no overflow menu, no confirmation — this action does not
  destroy daemon state) is a lighter, already-imported pattern (`X` icon used
  identically at `App.tsx:1594,2127,2409,5267`).
- `ws-dashboard/frontend/src/App.tsx:7147-7168` — `WorkspaceRows` renders
  child work-root rows with `actionServerId` **not** passed (defaults to
  `"server-local"` per `ResourceRow`'s prop default at `App.tsx:7194`), even
  though `WorkspaceRows` already receives the correct `serverId` prop. This
  must be fixed as part of this phase (pass `actionServerId={serverId}`) or
  the close command will carry the wrong `serverRoute` for non-local servers,
  and its `openWorkRootKeys` membership lookup (which is keyed by
  `serverScopedIdentity(serverRoute, rootId)`) will silently miss.
- `ws-dashboard/frontend/src/App.tsx:7096-7124` — `compactRoot` branch of
  `WorkspaceRows` already passes `actionServerId={serverId}` correctly; the
  compact-workspace-as-workroot row is also a legitimate close target and
  needs the same `isOpen`/`onClose` wiring as the non-compact work-root row.
- **Non-obvious constraint beyond the ticket's explicit list:**
  `buildEditorGroupsForRoot` (`App.tsx:3469-3524`) reads
  `activityPaneOpenByRoot[rootKey]` **unconditionally when `isSelectedRoot`
  is true** (line 3508: `isSelectedRoot ? activityPaneOpenByRoot[rootKey] ??
  false : false`) — the `false` override only applies while the root is in
  the background. If `activityPaneOpenByRoot[rootKey]` is left `true` after
  close, reselecting the same root later (a "reopen" within the session,
  since nothing currently prevents re-selecting a closed root's id from the
  resource tree) immediately resurfaces the WorkRoot Activity pane even
  though `terminalPanes`/`workbenchGroupsByRoot`/`paneOrderByRoot` were
  cleared. The ticket's explicit teardown list
  (`terminalPanes`/`workbenchGroupsByRoot`/`paneOrderByRoot`) does not
  mention `activityPaneOpenByRoot`, but skipping it reintroduces exactly the
  kind of stale-presentation bug this ticket is about. Recommend also
  clearing `activityPaneOpenByRoot[rootKey]` on close (cheap, same
  `WorkbenchShell`-local `useState`, `App.tsx:3326-3328`).
  `activePaneByRoot[rootKey]` and `closedAgentPaneByRoot[rootId]`
  (`App.tsx:3257-3259`, `3276-3278`) are lower-risk staleness (referenced
  pane ids simply won't exist in the freshly rebuilt empty groups) — clearing
  them is a cheap consistency improvement but not required for correctness.
- `ws-dashboard/frontend/src/terminals.ts:592-599`
  (`removeClosedTerminalPane`) — existing precedent for a pure, exported,
  unit-testable pane-map filter helper. Add a sibling
  `removeTerminalPanesForWorkRoot(current, rootId, serverRoute)` here (filter
  by `pane.session.workRootId`/`pane.session.serverRoute`) instead of an
  inline filter in `App.tsx`, matching the Phase 1 review finding that
  extracting pure logic out of `App.tsx` into a testable module was a real,
  missed opportunity (ticket Phase 1 Result: "extracted `findOpenWorkRoot`...
  added coverage").
- No unit test harness renders `App.tsx` JSX directly (Phase 1 Result: "the
  claimed `test:workbench`/`test:terminals` pass results provided zero
  regression coverage... neither suite touches `App.tsx`"). The left-panel
  button wiring and the `openWorkRootKeys` lift itself will only be covered
  by `npm run build` (type-check) plus manual/Playwright verification (same
  environment gap as Phase 1 — Playwright cannot run in this sandbox,
  `libasound.so.2` missing).

## Implementation Plan

1. **`ws-dashboard/frontend/src/terminals.ts`** — add
   `removeTerminalPanesForWorkRoot(current: Record<string, TerminalPaneState>, rootId: string, serverRoute: string | undefined): Record<string, TerminalPaneState>`,
   filtering out entries whose `pane.session.workRootId === rootId &&
   (pane.session.serverRoute ?? "server-local") === (serverRoute ??
   "server-local")`. Export it next to `removeClosedTerminalPane`
   (`terminals.ts:592-599`). Add a unit test in `terminals.test.ts` covering:
   removes only matching-root entries, leaves other roots/servers untouched,
   no-op on empty map.

2. **`ws-dashboard/frontend/src/commands.ts`** —
   - Add `"workRoot.close"` to `DashboardCommandId` (near `"workRoot.open"`,
     line 69).
   - Add `{ type: "workRoot.close"; workRootId: string }` to
     `DashboardCommandPayload` (near line 69).
   - Add `buildWorkRootCloseCommand(workRootId: string, serverRoute:
     string = LOCAL_DASHBOARD_SERVER_ROUTE): DashboardCommand`, mirroring
     `buildWorkspaceRemoveCommand` (`commands.ts:343-350`).
   - Add a `dashboardCommandLabel` case, e.g. `case "workRoot.close": return
     "Close work root";` (near the `workspace.remove` case,
     `commands.ts:562-563`).
   - Extend `commands.test.ts` with a block mirroring the existing
     `workspace.remove` coverage (label stability, default server route,
     remote server route, payload shape) — follow the pattern at
     `commands.test.ts:246,468-489`.

3. **`ws-dashboard/frontend/src/App.tsx` — lift `openWorkRootKeys`/
   `openWorkRootRefs` from `WorkbenchShell` to `App()`:**
   - Move the two `useState` declarations (currently `App.tsx:3284-3287`) up
     to `App()`, alongside `workbenchGroupsByRoot`/`paneOrderByRoot`
     (`App.tsx:379-383`).
   - Move the population effect (currently `App.tsx:3348-3363`, keyed on
     `selectedWorkRootStateKey`/`selection`) up to `App()` as well, using
     `App()`'s own `workbenchSelection` (`App.tsx:636-639`) in place of
     `selection`.
   - Change `WorkbenchShell`'s props to receive `openWorkRootKeys`,
     `openWorkRootRefs` as read props plus
     `onOpenWorkRootKeysChange: Dispatch<SetStateAction<string[]>>` and
     `onOpenWorkRootRefsChange: Dispatch<SetStateAction<Record<string,
     {rootId:string; serverRoute:string}>>>` setters — mirroring
     `workbenchGroupsByRoot`/`onWorkbenchGroupsByRootChange`
     (`App.tsx:3213-3241`). Replace the local `useState` calls and the local
     effect inside `WorkbenchShell` with the incoming props/setters; all
     downstream reads (`openWorkRootInstances` at `App.tsx:3554-3577`) stay
     unchanged since they only read `openWorkRootKeys`/`openWorkRootRefs` by
     name.
   - Wire the new props at the `<WorkbenchShell ...>` call site
     (`App.tsx:1006-1026`), passing `openWorkRootKeys={openWorkRootKeys}`,
     `openWorkRootRefs={openWorkRootRefs}`,
     `onOpenWorkRootKeysChange={setOpenWorkRootKeys}`,
     `onOpenWorkRootRefsChange={setOpenWorkRootRefs}`.

4. **`ws-dashboard/frontend/src/App.tsx` — `executeCommand`
   (`App.tsx:787-902`):** add a `workRoot.close` case (payload type
   `"workRoot.close"`) that, given `workRootId` and `serverRoute` (default
   `"server-local"`):
   - Computes `rootKey = serverScopedIdentity(serverRoute, workRootId)`.
   - `setOpenWorkRootKeys((current) => current.filter((key) => key !==
     rootKey))` — this is what stops rendering that root's
     `DockviewWorkbenchLayout` instance and triggers the unmount-driven
     dispose/socket-close.
   - `setOpenWorkRootRefs((current) => { const next = {...current}; delete
     next[rootKey]; return next; })`.
   - Delete `rootKey` from `workbenchGroupsByRoot` and `paneOrderByRoot`
     (mirror the filter shape already used in the `workspace.remove` case,
     `App.tsx:879-892`).
   - Do **not** call `closeTerminal()`/`closeTerminalPane` — the daemon
     terminal must stay alive.
   - This case cannot filter `terminalPanes`/`activityPaneOpenByRoot`/
     `activePaneByRoot`/`closedAgentPaneByRoot` directly because those remain
     `WorkbenchShell`-local state; instead, add a small `useEffect` inside
     `WorkbenchShell` keyed on the `openWorkRootKeys` prop that detects a
     previously-tracked rootKey no longer present (diff against a
     `useRef<string[]>` of the last-seen keys) and, for each newly-missing
     rootKey with a resolvable `{rootId, serverRoute}` from the *previous*
     `openWorkRootRefs` snapshot:
     - `setTerminalPanes((current) => removeTerminalPanesForWorkRoot(current,
       rootId, serverRoute))`.
     - `setActivityPaneOpenByRoot((current) => { const next = {...current};
       delete next[rootKey]; return next; })` (addresses the stale-reselect
       finding above).
     - Optionally also clear `activePaneByRoot[rootKey]` and
       `closedAgentPaneByRoot[rootId]` for full symmetry (low risk either
       way; include if it doesn't complicate the effect).

5. **`ws-dashboard/frontend/src/App.tsx` — left panel wiring:**
   - `ResourceNavigation` (`App.tsx:2494-2608`): accept a new prop
     `openWorkRootKeys: ReadonlySet<string>` (build a `Set` once, e.g. via
     `useMemo`, at the `App()` call site) and pass it down into `ServerRows`.
   - `ServerRows` (`App.tsx:2610-2703`): forward `openWorkRootKeys` into
     each `WorkspaceRows`.
   - `WorkspaceRows` (`App.tsx:7071-7179`): for the `compactRoot` branch
     (`App.tsx:7096-7126`) and the per-work-root branch
     (`App.tsx:7147-7168`), compute `isOpen = openWorkRootKeys.has(
     serverScopedIdentity(serverId, root.id))` and pass `isOpen` into
     `ResourceRow` as `isOpenWorkRoot`. Fix the missing
     `actionServerId={serverId}` on the non-compact work-root `ResourceRow`
     call (`App.tsx:7147-7168`) while doing this, since the close command
     needs the correct `serverRoute`.
   - `ResourceRow` (`App.tsx:7185-7325`): add an optional prop
     `isOpenWorkRoot?: boolean`. When `kind` is present (i.e. the row
     represents a work root, `presentation` is `"workRoot"` or
     `"compactWorkRoot"`) and `isOpenWorkRoot` is `true`, render a
     `ChromeIconButton` with `icon={X}`, `commandId="workRoot.close"`,
     `label={`Close ${title}`}`, calling
     `onCommand(buildWorkRootCloseCommand(actionEntityId, actionServerId))`
     — `executeCommand` (step 4) fully handles this command itself, so no
     extra `handlers` argument or callback prop chain is needed, consistent
     with how `resource.action.server.add` and `workRoot.activation.set`
     are dispatched directly by their buttons (`App.tsx:2543-2560`,
     `4795-4814`). No confirmation dialog (unlike `workspace.remove`) —
     this action does not delete anything on disk or terminate any daemon
     session.
   - `App()` call site (`App.tsx:961-978`): pass
     `openWorkRootKeys={openWorkRootKeysSet}` into `<ResourceNavigation>`,
     where `openWorkRootKeysSet` is a `useMemo(() => new
     Set(openWorkRootKeys), [openWorkRootKeys])` built from the lifted
     `openWorkRootKeys` state (step 3).

6. Confirm `.resource-row-actions` styling in
   `ws-dashboard/frontend/src/styles.css` accommodates an additional
   unconditional icon button (currently only rendered conditionally for
   `hasWorkspaceRemove`); add a small style tweak only if visually cramped —
   keep this a minimal, no-new-class change if the existing
   `.resource-row-actions` flex layout already handles two buttons.

## Verification Plan

- `npm run build` (tsc -b + vite build) inside `ws-dashboard/frontend` — must
  pass; this is the primary signal for the prop-lifting refactor's type
  correctness (mirrors Phase 1's verification).
- `npm run test:terminals` — must include the new
  `removeTerminalPanesForWorkRoot` unit test and continue passing.
- `npm run test:commands` — must include the new `workRoot.close` command
  builder/label tests and continue passing.
- `npm run test:workbench` — regression guard for `findOpenWorkRoot`/
  workbench model tests untouched by this phase.
- Playwright e2e (`dashboard-acceptance.spec.ts`) is the only test surface
  that could exercise the actual left-panel button + unmount behavior
  end-to-end; per Phase 1 Result this cannot run in this sandbox
  (`libasound.so.2` missing, no Chromium binary) — note this gap explicitly
  in the phase report rather than silently skipping it, matching the
  sibling ticket's precedent.
- Manual/structural verification: after wiring, confirm via code reading (or
  a local dev run if the sandbox gap doesn't apply there) that closing an
  open, non-selected work root removes its rootKey from `openWorkRootKeys`
  and that `TerminalPaneBody`'s cleanup effect actually fires (relying on
  the same React unmount semantics already proven by Phase 1's
  `display:none`-vs-unmount distinction).

## Escalations

- None.
