# Plan: 260703-feat-dashboard-workroot-session-keepalive — Phase 7: Reuse layout/terminal-visual restore for in-session close/reopen

## Relevant Ticket Contract

- Phase 7 text: "Wire Phase 2's explicit 'close work root' teardown and its
  later reopen path through the same capture-and-restore primitive built in
  Phases 5-6, instead of starting a reopened root from a blank workbench.
  Closing a work root should snapshot its layout and terminal visual state at
  close time (or reuse the last debounced snapshot); reopening within the
  same browser session restores from that snapshot the same way a page reload
  would."
- Spec Impact classification: "Phases 1-4, 7: Contract-first: no ... an
  implementation-quality fix, not a new persisted contract." No spec edit is
  required for this phase.
- Verification boundary implied by sibling phases (5/6): unit/route tests for
  pure logic, `npm run build`, `npm run test:workbench`/`test:terminals`;
  Playwright e2e is not runnable in this sandbox (missing `libasound.so.2`/no
  Chromium), same pre-existing gap as every prior phase — structural/code
  verification is the accepted fallback here too.

## Out of Scope

- Phases 1-6 behavior itself (already merged); only the close/reopen reuse
  wiring is in scope.
- Any change to the `WorkRoot IO Restore Model` spec anchor — this phase is
  classified Contract-first: no.
- Daemon-side terminal lifecycle, `listTerminals`/reattach protocol changes —
  Phase 7 only touches which *in-memory* restore snapshot the existing
  reattach/seed code paths read from.
- Buffer retention policy, truncation signaling — Phase 4's explicit non-goal,
  unrelated here.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L409-L420` — `initialWorkbenchLayoutRestore`
  and `initialTerminalVisualRestore` are `useState` values with **no setter
  ever called** — loaded once via `loadWorkbenchLayoutRestoreSnapshot()` /
  `loadTerminalVisualRestoreSnapshot()` at mount and frozen for the rest of the
  browser session. This is the root cause of the phase's gap: every same-session
  reopen consumer below reads this frozen mount-time value instead of the
  continuously-updated live state.
- `ws-dashboard/frontend/src/App.tsx#L687-L723` — the workbench-selection seed
  effect. On (re)selecting a root, if `rootKey` is newly added to
  `openWorkRootKeys`, it seeds `workbenchGroupsByRoot`/`paneOrderByRoot` from
  `initialWorkbenchLayoutRestore[rootKey]` (frozen). A same-session close
  deletes both maps' entries for that rootKey (`App.tsx#L935-L950`), so a
  reopen re-seeds from this same frozen snapshot — which reflects state at
  page load, not state as of the close.
- `ws-dashboard/frontend/src/App.tsx#L913-L951` — the `workRoot.close` command
  handler: removes `rootKey` from `openWorkRootKeys`/`openWorkRootRefs`/
  `workbenchGroupsByRoot`/`paneOrderByRoot` in one commit. Does not call
  `closeTerminal()` — daemon terminal session stays alive (existing, correct,
  unchanged).
- `ws-dashboard/frontend/src/App.tsx#L3636-L3670` — **risk signal (real bug,
  not just staleness).** The layout save effect computes `liveEntries` for
  currently-open roots plus `untouchedEntries` as
  `Object.entries(initialWorkbenchLayoutRestore).filter(([rootKey]) => !openWorkRootKeysSet.has(rootKey))`.
  The moment a root closes, this same effect re-fires (its deps include
  `openWorkRootKeys`/`workbenchGroupsByRoot`, both just changed): the closed
  root now falls into `untouchedEntries` and is written back from the **stale
  mount-time** `initialWorkbenchLayoutRestore` entry, clobbering whatever
  live/current layout `localStorage` held for that root just before the close.
  This means even a full page reload after an in-session close/edit sequence
  would currently restore the pre-session layout, not the last live one —
  Phase 7 must fix this, not just the in-memory reopen seed.
- `ws-dashboard/frontend/src/App.tsx#L5076-L5081` — `DockviewWorkbenchLayout`'s
  `initialGroupSizeById={initialWorkbenchLayoutRestore[rootKey]?.groupSizeById}`
  is read once per mount of that root's dockview instance. Since close fully
  unmounts the instance (Phase 2 result) and reopen remounts a fresh one, this
  is a second same-bug read site that needs the live value at remount time.
- `ws-dashboard/frontend/src/App.tsx#L3885-L3896` — `onVisualRestoreEntryFor:
  (pane) => initialTerminalVisualRestore[pane.logicalKey]` and
  `onVisualCapture: (pane, capture) => upsertTerminalVisualRestoreEntry({...})`.
  Capture already does a correct localStorage read-merge-write per pane
  (`workbench/terminalVisualRestore.ts#L114-L128`, `upsertTerminalVisualRestoreEntry`
  — no clobber bug here, unlike layout), but the read side
  (`onVisualRestoreEntryFor`) still only consults the frozen mount-time value,
  so a capture made mid-session is invisible to a same-session reattach.
- `ws-dashboard/frontend/src/App.tsx#L3977-L4014` — the `listTerminals` reattach
  effect calls `reconcileListedTerminalSessions(current, rootId, sessions,
  listStartedAtMs, serverRoute, initialTerminalVisualRestore)` (also frozen).
  This runs whenever a root's `workbenchModel` is (re)built, including on
  reopen (a new `TerminalPaneState` is created via reattach-by-id since Phase
  2's close deletes the pane entirely — `removeTerminalPanesForWorkRoot`). This
  is the second terminal-visual read site needing the live value.
- `ws-dashboard/frontend/src/workbench/layoutRestore.ts#L41-L45` —
  `workbenchLayoutRestoreRootKey(entry)` already exported; reuse it to key a
  merged-back snapshot rather than re-deriving `serverScopedIdentity` inline.
- `ws-dashboard/frontend/src/workbench/terminalVisualRestore.ts#L114-L128` —
  `upsertTerminalVisualRestoreEntry` (already fresh-read-merge-write per
  entry, confirms no save-side clobber bug for terminal-visual, only a
  read-side staleness bug).
- `ws-dashboard/frontend/src/App.tsx#L1134-L1151` and `#L3357-L3390` — the
  `WorkbenchShell` component receives `initialWorkbenchLayoutRestore` /
  `initialTerminalVisualRestore` as plain-value props from `App()`, typed
  `WorkbenchLayoutRestoreSnapshot` / `TerminalVisualRestoreSnapshot`. All read
  sites above live inside `WorkbenchShell`, while `workbenchGroupsByRoot`/
  `paneOrderByRoot`/`openWorkRootKeys`/`openWorkRootRefs` are lifted,
  App()-level state (Phase 2). Converting the two snapshot values to
  `useRef`s in `App()` and passing the ref objects themselves down (instead of
  dereferenced values) lets every read site above switch to `.current` with no
  further prop-plumbing, since mutation of a shared ref object is visible
  without a re-render.
- `ws-dashboard/frontend/src/workbench/openRootLookup.ts#L30-L57` —
  `resolveClosedWorkRootRefs` (existing, Phase 2) is the pure diff already
  used to detect a just-closed rootKey; not modified by this phase, just
  confirms the close-cleanup effect timing this plan relies on.
- Test locations: `ws-dashboard/frontend/src/workbench/layoutRestore.test.ts`,
  `ws-dashboard/frontend/src/workbench/terminalVisualRestore.test.ts`, run via
  `npm run test:workbench` / `npm run test:terminals` respectively
  (`ws-dashboard/frontend/package.json#L13-L18`).

## Implementation Plan

1. `App.tsx#L409-L420`: replace
   `const [initialWorkbenchLayoutRestore] = useState(() => loadWorkbenchLayoutRestoreSnapshot())`
   with `const workbenchLayoutRestoreRef = useRef<WorkbenchLayoutRestoreSnapshot>(loadWorkbenchLayoutRestoreSnapshot())`,
   and similarly `initialTerminalVisualRestore` →
   `const terminalVisualRestoreRef = useRef<TerminalVisualRestoreSnapshot>(loadTerminalVisualRestoreSnapshot())`.
   Update the comments above them to describe a session-lifetime, continuously
   refreshed snapshot rather than a mount-only one.
2. `App.tsx#L687-L723` (selection seed effect): read
   `workbenchLayoutRestoreRef.current[rootKey]` instead of
   `initialWorkbenchLayoutRestore[rootKey]`; drop `initialWorkbenchLayoutRestore`
   from the effect's dependency array (a ref is not a reactive dependency —
   `workbenchSelection` alone still drives the effect correctly).
3. `App.tsx#L1134-L1151` and the `WorkbenchShell` props type at
   `App.tsx#L3357-L3390`: change the prop type/shape for
   `initialWorkbenchLayoutRestore`/`initialTerminalVisualRestore` to accept the
   ref objects (e.g. `workbenchLayoutRestoreRef: MutableRefObject<WorkbenchLayoutRestoreSnapshot>`),
   pass `workbenchLayoutRestoreRef` / `terminalVisualRestoreRef` at the JSX
   call site instead of the current dereferenced values. Rename the prop/local
   variable if useful for clarity (e.g. drop the `initial` prefix), but keep
   the rename mechanical/local to this file — no external contract change.
4. `App.tsx#L3636-L3670` (layout save effect) — fix the clobber bug and keep
   the ref fresh:
   - Compute `untouchedEntries` from `workbenchLayoutRestoreRef.current`
     (via the prop) instead of the frozen value prop.
   - After computing `[...liveEntries, ...untouchedEntries]`, also write the
     merged result back into `workbenchLayoutRestoreRef.current`, keyed via
     `workbenchLayoutRestoreRootKey(entry)` (reuse the existing export from
     `layoutRestore.ts`), so the ref and `localStorage` stay in sync on every
     effect run — this is what makes step 2's reopen-seed and step 1's ref
     both see the closed root's true last-live entry instead of the mount-time
     one.
5. `App.tsx#L5076-L5081` (`initialGroupSizeById`): switch to
   `workbenchLayoutRestoreRef.current[rootKey]?.groupSizeById`.
6. `App.tsx#L3885-L3896` (`onVisualRestoreEntryFor` / `onVisualCapture`):
   - `onVisualRestoreEntryFor: (pane) => terminalVisualRestoreRef.current[pane.logicalKey]`.
   - In `onVisualCapture`, alongside the existing `upsertTerminalVisualRestoreEntry(...)`
     call, also update the ref:
     `terminalVisualRestoreRef.current = { ...terminalVisualRestoreRef.current, [entry.logicalKey]: entry }`
     (build the entry object once, reuse for both calls).
7. `App.tsx#L3977-L4014` (`listTerminals` reattach effect): pass
   `terminalVisualRestoreRef.current` instead of `initialTerminalVisualRestore`
   into `reconcileListedTerminalSessions(...)`.
8. Grep for any remaining reference to the old
   `initialWorkbenchLayoutRestore`/`initialTerminalVisualRestore` identifiers
   after the rename (`grep -n "initialWorkbenchLayoutRestore\|initialTerminalVisualRestore" App.tsx`)
   to confirm no stale read site was missed — the survey above found the
   complete set (2 layout read sites, 1 layout save/clobber site, 2
   terminal-visual read sites, 1 terminal-visual write site), but re-grep after
   the rename as a mechanical completeness check since a missed site would
   silently keep reading the frozen value.
9. No change needed to `workbench/layoutRestore.ts` or
   `workbench/terminalVisualRestore.ts` themselves — both modules' pure
   load/save/upsert functions already have the correct contract; only App.tsx's
   in-memory consumption of them needs to stop freezing at mount.

## Verification Plan

- `npm run build` (type-correctness of the `useRef`/prop-type changes).
- `npm run test:workbench` and `npm run test:terminals` (existing
  `layoutRestore.test.ts` / `terminalVisualRestore.test.ts` unit suites; no
  new pure-function surface is introduced by this phase, so no new test file
  is expected, but re-run to confirm no regression from the prop-type change).
- Manual/structural verification (Playwright e2e not runnable in this sandbox,
  same pre-existing gap as every prior phase): trace through the code path for
  a close→reopen sequence in the same session where the root's layout was
  rearranged (or a terminal pane produced new output) *after* mount and
  *before* close, confirming: (a) the save effect's post-close run no longer
  overwrites `localStorage` with the mount-time entry for that root, (b) the
  reopen selection-seed effect reads the just-updated ref and restores the
  rearranged layout, and (c) the terminal reattach path's
  `onVisualRestoreEntryFor`/`reconcileListedTerminalSessions` calls see the
  mid-session capture instead of the mount-time snapshot.

## Escalations

- None.
