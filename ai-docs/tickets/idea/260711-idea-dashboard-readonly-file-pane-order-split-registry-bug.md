---
title: "Check readOnlyFilePaneOrderByGroup for the same split-registry snap-back bug"
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
---

# Check readOnlyFilePaneOrderByGroup for the same split-registry snap-back bug

## Background

A dogfooded bug was confirmed and fixed (commit `bc566a78`,
2026-07-11): dragging a terminal panel into a different Dockview group
appeared to work for a moment, then snapped back to its original group on
the next render. Root cause: `movePane` (`App.tsx` ~4973) only wrote
drag-move results into `paneOrderByRoot`, while terminal panes are ordered
by a separate flat registry, `terminalPaneOrderByGroup`. Any pane id
missing from that registry falls back to `groups[0]`
(`workbench/editorGroupModel.ts` `applyWorkbenchPaneOrder`,
`originalGroupByPaneId` fallback), so the untouched registry silently
reverted the move.

While fixing this, the implementing agent flagged a parallel finding
in-commit but explicitly left it untouched:

> `readOnlyFilePaneOrderByGroup` has the same registry-split shape and is
> a plausible parallel latent issue, but `movePane`'s read-only file
> handling was left untouched — out of scope for this surgical fix.

## Questions To Resolve

- Does `readOnlyFilePaneOrderByGroup` have the same "separate flat
  registry, not mirrored by `movePane`" shape as `terminalPaneOrderByGroup`
  did before the fix?
- If so, does it manifest the same symptom: dragging a read-only file
  viewer pane into a newly created split (not just an existing group)
  snaps back to its original group?
- If confirmed, apply the same mirroring fix pattern used in `bc566a78`
  (filter the move result's per-group pane order down to ids present in
  the read-only-file pane set, write into
  `readOnlyFilePaneOrderByGroup` alongside `paneOrderByRoot`).

## Non-Goals

- Re-investigating the already-fixed terminal pane case.
- Broader refactor of the pane-order registry split itself (e.g.
  unifying `paneOrderByRoot`, `terminalPaneOrderByGroup`, and
  `readOnlyFilePaneOrderByGroup` into one model) — worth a separate idea
  if this pattern turns out to repeat a third time, but not forced here.
