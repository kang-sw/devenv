# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 1: Per-work-root workbench instances (no destroy on root switch)

## Relevant Ticket Contract

- Replace the single shared `DockviewWorkbenchLayout` mount in
  `WorkbenchShell` with one instance per open work root; each holds its own
  dockview panel set computed from that root's panes only. Inactive roots'
  instances stay mounted in the DOM (`display:none` or equivalent), not torn
  down.
- Constraints: `terminalPanes`, `workbenchGroupsByRoot`, `paneOrderByRoot` are
  already root-scoped or filterable by root id — reuse them per-instance,
  introduce no new state shapes for those three.
- Verify dockview does not require a single global instance for cross-root
  drag/drop; if it does, scope that interaction out this phase rather than
  redesigning dockview usage.
- No socket-visibility changes this phase — a pane in a hidden root instance
  keeps its live socket for now (Phase 3 territory).

## Out of Scope

- Phase 2 "close work root" affordance/teardown (only reuses today's
  destructive path when explicitly triggered later).
- Phase 3 visibility-gated socket lifecycle.
- Phases 4-7 (cursor accuracy, layout persistence, terminal-visual persistence,
  close/reopen restore reuse).
- Any change to `activityPaneOpenByRoot`/activity-stream fetching behavior for
  backgrounded roots — this phase does not need live activity data outside the
  selected root.

## Codebase Findings

- `frontend/src/App.tsx#L3201-L3213` — `WorkbenchShell` is one large function
  component; all per-root state (`terminalPanes`, `activePaneByRoot`,
  `paneOrderByRoot` via props, `closedAgentPaneByRoot`, `activityPaneOpenByRoot`)
  already lives here, keyed by root id or `serverScopedIdentity(serverId,
  rootId)`.
- `frontend/src/App.tsx#L3319-L3337` — `selectedWorkRootId` /
  `selectedWorkRootStateKey` derive the *single* active root's slices
  (`workbenchGroups`, `paneOrderByGroup`, `activePaneByGroup`,
  `activityPaneOpenForSelected`) from the `*ByRoot` maps. This pattern
  (`map[rootKey] ?? default`) is exactly what a per-root loop needs to repeat
  for every open root, not just the selected one.
- `frontend/src/App.tsx#L3437-L3494` — the `workbenchModel` block is the single
  place that calls `buildWorkbenchEditorGroups` + `applyWorkbenchPaneOrder` to
  produce `editorGroups`, but only for `selection`'s root (the active one).
  This must become a per-open-root computation (a plain function call inside a
  `.map()`, not a new hook — `buildWorkbenchEditorGroups` is a pure function,
  so calling it N times per render is safe and avoids hook-in-loop issues).
- `frontend/src/App.tsx#L5669-L5759` (`buildWorkbenchEditorGroups`) — accepts
  `selectedInstance` and `supportEntity` params but immediately
  `void`s them (`L5695-L5696`); they are dead for this function's output. A
  per-root call only needs `root` and `mainInstance` (e.g.
  `root.mainInstances[0] ?? null`), no real selection-resolution needed for
  inactive roots.
- `frontend/src/App.tsx#L5839-L5873` (`terminalWorkbenchPanesByGroup`) and
  `frontend/src/App.tsx#L6469-L6493` (`readOnlyWorkbenchPanesByGroup`) —
  both already filter their pane list by `pane.session.workRootId === root.id`
  / `pane.workRootId === root.id` (+ `serverRoute`) before building group
  contents, and both consume the shared flat `terminalPaneOrderByGroup` /
  `readOnlyFilePaneOrderByGroup` maps defensively (`paneById.get(paneId)`,
  skip if absent). Confirms the ticket's claim: these two flat (not
  root-scoped) order maps are safe to keep passing to every root's
  `buildWorkbenchEditorGroups` call unchanged — stale ids from another root are
  silently ignored, not double-rendered. No new state shape needed here.
- `frontend/src/App.tsx#L4402-L4436` (`movePane`) and `L4438-L4448`
  (`selectPane`) close over `workbenchModel`/`editorGroups`/`activePaneByGroup`,
  i.e. always the *currently selected* root. These do not need to become
  root-parameterized: only the visible (non-`display:none`) dockview instance
  can ever receive real pointer/keyboard interaction, so wiring the same
  `movePane`/`selectPane`/`requestWorkbenchPaneClose` callbacks into every
  per-root `DockviewWorkbenchLayout` mount is safe — hidden instances'
  internal `DockviewApi` events are also suppressed during programmatic
  `syncPanels()` via the existing `syncingRef` guard
  (`frontend/src/workbench/dockviewLayout.tsx#L119-L133`).
- `frontend/src/App.tsx#L4175-L4306` — all terminal action handlers
  (`sendTerminalData`, `closeTerminalPane`, `forwardTerminalResize`,
  `updateTerminalSocketStatus`, `applyTerminalSocketMessage`,
  `acceptTerminalSocketResize`) take a specific `pane: TerminalPaneState` as
  their first argument, not an implicit "selected root" — safe to reuse
  verbatim across every root's `terminalActions` object.
- `frontend/src/App.tsx#L4517-L4567` — current render: early-returns
  `StatusPane` if `!workbenchModel`, otherwise renders `WorkbenchToolbar` (once,
  selected root only) + a single `DockviewWorkbenchLayout`. The toolbar/error/
  loading banner/close-popover stay single (Phase 2 territory for close
  affordance); only the `DockviewWorkbenchLayout` line needs to become a loop.
- `frontend/src/workbench/dockviewLayout.tsx#L94-L152` — `DockviewWorkbenchLayout`
  is already a self-contained component: its own `apiRef`/`syncingRef` live in
  its own hook instance, so mounting it N times via `.map()` gives N
  independent `DockviewReact` instances automatically — no shared-global-state
  redesign needed. Confirms cross-root drag/drop was never wired across
  instances (each mount's `DockviewApi` is private), so "scope out
  cross-root drag/drop" is a no-op default, not new work.
- `frontend/src/workbench/dockviewLayout.tsx#L315-L328` (`syncDockviewWorkbench`)
  — the `api.removePanel` destructive loop this ticket is about; it stays
  unchanged, it just now only ever sees one root's `desiredPaneIds` per
  mounted instance instead of being fed a filtered-then-swapped set from a
  single shared instance.
- **Risk signal — no existing "open work roots" list.** Selection is a single
  tree-walk id (`selectedId` in `App`, resolved via `resolveWorkbenchSelection`,
  `frontend/src/App.tsx#L7428-L7477`) with no history of previously-visited
  roots. Phase 1 needs new state to know which roots to keep mounted — an
  ordered list/set of root keys (`serverScopedIdentity(serverId, rootId)`)
  appended whenever `selection`'s root changes, is a new state shape, but it is
  about *set membership*, not a new shape for panes/layout/order (which the
  ticket's reuse instruction is actually about) — treat this as in-scope
  addition, not a contract violation.
- **Risk signal — e2e locator may become ambiguous.** Playwright tests query
  `page.locator('[data-workbench-layout-owner="dockview"]')`
  (`frontend/e2e/dashboard-acceptance.spec.ts#L318,L408`) expecting exactly one
  match. That attribute is owned by `DockviewWorkbenchLayout`'s own wrapper div
  (`frontend/src/workbench/dockviewLayout.tsx#L136-L142`, CONTRACT-marked, do
  not remove/rename it). Once >1 work root is open, N such divs exist
  simultaneously. Current test flows never open a second work root mid-test,
  so this is latent, not a current failure — but the new per-root wrapper
  (the `display:none` container) should carry its own marker (e.g.
  `data-workbench-root-active="true"|"false"`) so future/updated test
  selectors can scope to the active instance without touching the existing
  CONTRACT marker.
- `frontend/src/styles.css#L1463,L1541-L1580` — existing `.workbench-shell` /
  `.dockview-workbench-layout` rules; add the new per-root wrapper's
  `display:none`-when-inactive rule near these, or use inline styles instead
  (see Implementation Plan step 6).

## Implementation Plan

1. `frontend/src/App.tsx` (top-level `App`, near `L378-L383`): no change needed
   here — `workbenchGroupsByRoot`/`paneOrderByRoot` already live at this level
   and are passed down; reuse as-is.
2. `frontend/src/App.tsx` (`WorkbenchShell`, near `L3256-L3286` state block):
   add `const [openWorkRootKeys, setOpenWorkRootKeys] = useState<string[]>([])`
   (ordered, de-duplicated) plus a `useEffect` keyed on
   `selectedWorkRootStateKey` that appends the key if not already present.
   Also keep a small `Record<string, { rootId: string; serverRoute: string | null }>`
   alongside it (or derive both from one map) so each open root can be
   re-resolved without depending on `selectedId`/tree-walk state.
3. `frontend/src/App.tsx` (near `L3437-L3494`): factor the
   `buildWorkbenchEditorGroups` + `applyWorkbenchPaneOrder` call into a helper
   (e.g. `buildEditorGroupsForRoot(root, mainInstance, rootKey)`) that pulls
   `workbenchGroupsByRoot[rootKey] ?? initialWorkbenchGroups`,
   `paneOrderByRoot[rootKey] ?? {}`, `activePaneByRoot[rootKey] ?? {}`,
   `closedAgentPaneByRoot[root.id] ?? []`, and — only for the selected
   root — the live `activityPaneOpenByRoot`/`workRootActivityState`/
   `activityTranscriptRefresh`; pass `false`/`{phase:"loading"}`/`null` for
   non-selected roots (matches `buildWorkbenchEditorGroups`'s existing
   defaults, `L5682-L5684`). Keep using the existing selected-root
   `workbenchModel` for the toolbar/activity badge (`L4534-L4540`) unchanged.
4. Resolve each open root's `WorkRootView` + `mainInstance` by walking
   `resources.workspaces` for the stored `rootId` (a small local lookup is
   enough — `buildWorkbenchEditorGroups` ignores `selectedInstance`/
   `supportEntity`, so `resolveWorkbenchSelection` is not required, just a
   root-id search across `workspace.workRoots`).
5. `frontend/src/App.tsx` (render, `L4561-L4567`): replace the single
   `<DockviewWorkbenchLayout .../>` with
   `openWorkRootKeys.map((rootKey) => <div key={rootKey} className="workbench-root-instance" data-workbench-root-active={rootKey === selectedWorkRootStateKey} style={{ display: rootKey === selectedWorkRootStateKey ? undefined : "none" }}><DockviewWorkbenchLayout groups={editorGroupsForRoot(rootKey)} activePaneByGroup={activePaneByRoot[rootKey] ?? {}} onMovePane={movePane} onRequestClosePane={requestWorkbenchPaneClose} onSelectPane={selectPane} /></div>)`.
   Reuse `movePane`/`selectPane`/`requestWorkbenchPaneClose` unchanged (they
   already operate on the selected root only, and only the visible instance
   can dispatch real events — see Codebase Findings).
6. Prefer the inline `style={{ display: ... }}` on the wrapper (step 5) over a
   new CSS rule, matching the existing `WorkbenchClosePopover` inline-style
   precedent (`L4603-L4606`); no `styles.css` change is required unless a
   transition/animation is added later.
7. Skip the resources-not-loaded / no-selection guard (`L4525-L4532`) as today;
   `openWorkRootKeys` will be empty until a first selection exists, so no
   extra guard is needed there.

## Verification Plan

- No unit test target exists for `WorkbenchShell`/`App.tsx` (it is exercised
  only via `frontend/e2e/dashboard-acceptance.spec.ts`, Playwright). Manual/e2e
  verification: open a work root with a terminal pane, produce distinguishing
  output, switch to a second work root, switch back, and confirm the terminal
  DOM node (and its xterm scrollback) was never unmounted — e.g. assert no
  `TerminalPaneBody`/xterm remount by checking the pane's DOM node identity or
  scroll position survives the round trip, instead of the buffered-replay
  flicker described in the ticket's Background section.
- Run `frontend/e2e/dashboard-acceptance.spec.ts` (or at minimum the terminal
  and dockview-owner sections, `L318`, `L408`, `L2582-L2652`) to confirm no
  regression from the loop-based render; single-root flows must be unaffected.
- If a work-root-switch scenario is added to the acceptance spec, verify
  `page.locator('[data-workbench-layout-owner="dockview"]')` usages are updated
  to scope by the new `data-workbench-root-active="true"` marker to avoid
  Playwright strict-mode multi-match failures once two roots are open
  simultaneously.

## Escalations

- None.
