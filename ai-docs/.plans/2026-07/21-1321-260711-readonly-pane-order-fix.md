# Plan: 260711-idea-dashboard-readonly-file-pane-order-split-registry-bug — Phase 1: diagnose latent-vs-regression (from d8e71b51) then fix document/readonly pane ordering not persisting

## Relevant Ticket Contract
- Determine whether the confirmed misbehavior is (i) the pre-existing latent
  issue this ticket hypothesized (`readOnlyFilePaneOrderByGroup` never
  mirrored by `movePane`, same shape `terminalPaneOrderByGroup` had before
  `bc566a78`), or (ii) a regression introduced by `d8e71b51`.
- If latent, apply the same scoped-key/registry-mirroring pattern used by
  `bc566a78` (and refined by `125d68e1`) to the read-only/document pane path:
  filter the move result's per-group pane order down to ids present in the
  read-only-file pane set, write into `readOnlyFilePaneOrderByGroup`.
- Success: dragging/splitting grouped document panes persists correctly;
  `test:workbench` and `test:resource-model` stay green, plus manual dogfood
  confirmation.

## Out of Scope
- Re-investigating the already-fixed terminal pane case (`bc566a78`,
  `125d68e1`, `d8e71b51`).
- Broader refactor unifying `paneOrderByRoot`, `terminalPaneOrderByGroup`,
  and `readOnlyFilePaneOrderByGroup` into one model.
- The pre-existing, unrelated `260713` acceptance-suite defect and the
  documented Dockview native-group-id-reuse behavior ("mechanism 2") noted in
  `d8e71b51`'s AI Context — neither is implicated here.

## Codebase Findings

**Confirmed hypothesis: H-LATENT (pre-existing gap, not a `d8e71b51` regression).**

- `ws-dashboard/frontend/src/App.tsx#L5818-L5890` (current `movePane`) — the
  only per-pane-kind mirror write inside `movePane` is
  `setTerminalPaneOrderByGroup` (L5871-L5887). There is no call to
  `onReadOnlyFilePaneOrderByGroupChange`/`setReadOnlyFilePaneOrderByGroup`
  anywhere in this function. Confirmed by exhaustive grep: all four existing
  `setReadOnlyFilePaneOrderByGroup` call sites in the file are at
  `App.tsx#L961`, `L1006`, `L1293`, `L1471` — file-open/close/placement paths,
  none inside `movePane`.
- `git show d8e71b51 -- ws-dashboard/frontend/src/App.tsx` — the entire diff
  only rekeys the two existing `movePane` writes
  (`onWorkbenchGroupsByRootChange`/`onPaneOrderByRootChange`) from
  `workbenchModel.root.id` to `moveRootKey = serverScopedIdentity(...)`. It
  does not touch, add, or remove any `readOnlyFilePaneOrderByGroup` code path.
  Its own commit message states the terminal mirror
  (`setTerminalPaneOrderByGroup`) was "left untouched per the plan: it is
  keyed by plain `groupId`, not `rootKey`, and is already correct" — i.e.
  `d8e71b51`'s scope was exclusively the `paneOrderByRoot`/
  `workbenchGroupsByRoot` root-key mismatch, orthogonal to the flat
  groupId-keyed `*PaneOrderByGroup` registries.
- `git show 125d68e1` (2026-07-14, six days before `d8e71b51`) — commit
  message explicitly states: "This is distinct from
  260711-idea-dashboard-readonly-file-pane-order-split-registry-bug: that
  idea ticket covers `readOnlyFilePaneOrderByGroup` (no mirror write at all
  yet)." This is a direct, dated, pre-`d8e71b51` confirmation that the gap
  already existed before the regression window opened.
- `ws-dashboard/frontend/src/App.tsx#L8900-L8947` (`readOnlyWorkbenchPanesByGroup`)
  — the read-only-pane reader has the identical "registry miss falls back to
  a fixed group" shape as the pre-`bc566a78` terminal reader: a pane id
  absent from `readOnlyFilePaneOrderByGroup[groupId]` lands in
  `byGroup[groups[1]?.id ?? groups[0]?.id ?? "group-2"]` (L8940-L8944), so
  every drag/drop move (which never writes this registry) gets reconciled
  back to that fallback group on the next render — the reported snap-back.
- Conclusion: the gap predates `d8e71b51` by at least a week and `d8e71b51`'s
  diff never touches the read-only path; this is the pre-existing latent
  issue `bc566a78` flagged, not a regression from the terminal-split fix.

**Fix-shape evidence (key-space pitfall to avoid):**

- `ws-dashboard/frontend/src/workRootFiles.ts#L45-L64` — `ReadOnlyFilePane`
  has distinct `id` and `logicalKey` fields (disjoint string shapes), and
  `readOnlyFilePaneOrderByGroup`/`ReadOnlyFilePaneOrder` is `Record<string,
  readonly string[]>` — structurally the same as `TerminalPaneState`'s
  `paneId`/`logicalKey` split that caused the `125d68e1` key-mismatch bug.
- `ws-dashboard/frontend/src/App.tsx#L3637-L3670` (`WorkbenchShell` props) —
  inside `movePane`'s scope, `readOnlyFilePanes` is already the prop
  `ReadOnlyFilePane[]` (an array of pane objects, each with `.id`), and
  `onReadOnlyFilePaneOrderByGroupChange` (`Dispatch<SetStateAction<WorkbenchPaneOrder>>`)
  is the setter to call — both directly available in `movePane`'s closure,
  same as `terminalPanes`/`setTerminalPaneOrderByGroup` are for the terminal
  mirror.
- `ws-dashboard/frontend/src/App.tsx#L5871-L5887` (current, correct terminal
  mirror post-`125d68e1`) — filters `result.paneOrderByGroup` id lists
  against a `livePaneIds` set built from `pane.paneId` (the `WorkbenchPane.id`
  space), not against Record-key/`logicalKey` membership. The read-only mirror
  must copy this corrected shape directly — using `readOnlyFilePanes.map(pane
  => pane.id)` — rather than re-deriving the original `bc566a78`-era
  `id in <record>` pattern, which would silently drop every pane again.
- `git show bc566a78 -- ws-dashboard/frontend/src/App.tsx` — confirms the
  precise insertion shape to mirror (added directly after the existing
  `paneOrderByRoot` write, before the function's final
  `setActivePaneByGroupForSelected` call).

## Implementation Plan
1. In `ws-dashboard/frontend/src/App.tsx`, inside `movePane` (currently
   `L5818-L5890`), immediately after the existing `setTerminalPaneOrderByGroup`
   block (ends `L5887`) and before `setActivePaneByGroupForSelected` (`L5889`),
   add a mirror write to `onReadOnlyFilePaneOrderByGroupChange`:
   - Build `livePaneIds` from `readOnlyFilePanes.map((pane) => pane.id)` (the
     prop array already in scope — no `Object.values` needed since it is not
     a Record here, unlike `terminalPanes`).
   - For each `[groupId, paneIds]` in `result.paneOrderByGroup`, filter
     `paneIds` down to `livePaneIds.has(id)` and write into a new object,
     spread from `current`, exactly mirroring the `setTerminalPaneOrderByGroup`
     block's structure.
   - Add a short contract comment (matching the existing style at
     `L5866-L5870`) noting `readOnlyFilePaneOrderByGroup` is a separate flat
     registry from `paneOrderByRoot`/`terminalPaneOrderByGroup`, keyed by
     plain `groupId`, and that filtering must use `pane.id` (`WorkbenchPane.id`
     space), not `logicalKey`, per the `125d68e1` lesson.
   - This write is unconditional inside the existing `if (workbenchModel)`
     block (same gating as the terminal mirror); no new gating needed since
     read-only file panes are workRoot-scoped identically to terminal panes.
2. No other files require changes: `readOnlyFilePaneOrderByGroup`'s reader
   (`readOnlyWorkbenchPanesByGroup`, `App.tsx#L8900-L8947`) and its
   save/restore snapshot path (`workRootFiles.ts` `loadReadOnlyFilePaneRestoreSnapshot`/
   `saveReadOnlyFilePaneRestoreSnapshot`) are already correct and orthogonal
   to this gap — they only need the mirror write upstream to receive correct
   data.
3. Record the diagnosis (H-LATENT, not H-regression, with the `125d68e1`
   commit-message citation as the deciding evidence) and the fix in the
   ticket's Phase 1 `### Result` section and `## Ticket Updates` per repo
   ticket convention, once implemented.

## Verification Plan
- `cd ws-dashboard/frontend && npm run build` (tsc + vite build) — must stay
  green.
- `cd ws-dashboard/frontend && npm run test:workbench` — must stay green
  (covers workbench/dockview layout model tests; no existing test currently
  exercises `readOnlyFilePaneOrderByGroup` mirroring directly, so this is a
  regression-guard run, not new coverage).
- `cd ws-dashboard/frontend && npm run test:resource-model` — must stay green
  (unaffected area, but named in the ticket's success criteria).
- Manual dogfood: open two or more read-only/document panes in the same
  Dockview group, drag one into a new non-horizontal split (mirroring the
  terminal repro in `d8e71b51`'s linked plan
  `ai-docs/.plans/2026-07/20-1752-260720-bug-dashboard-terminal-split-nonhorizontal-snap-back.md`),
  and confirm the split persists across the next render instead of snapping
  back to the fallback group.

## Escalations
- None.
