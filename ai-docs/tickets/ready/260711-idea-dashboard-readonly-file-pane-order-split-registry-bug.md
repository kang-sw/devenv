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

## Dogfood Confirmation (2026-07-21)

Confirmed via live dogfooding on 2026-07-21: grouped document/read-only-file
panes now misbehave (splits/arrangement not working properly). This upgrades
the ticket from a flagged parallel lead to a confirmed, reproducing symptom.

## Phases

### Phase 1: Determine latent-vs-regression, then apply the mirroring fix

First determine whether the confirmed misbehavior is (i) the pre-existing
latent issue this ticket hypothesized -
`readOnlyFilePaneOrderByGroup` being a separate flat registry not mirrored
by `movePane` (the same shape `terminalPaneOrderByGroup` had before
`bc566a78`) - or (ii) a **regression** introduced by the terminal-split fix
commit `d8e71b51` (which keyed `movePane`'s workbench-group/pane-order
persistence by the scoped `rootKey` for terminal panes). Compare behavior
against the pre-`d8e71b51` baseline to distinguish the two.

Then fix by applying the **same** scoped-key/registry-mirroring pattern used
by the terminal-split fix (`d8e71b51`, and the earlier `bc566a78` mirroring
fix it built on) to the read-only/document pane path: filter the move
result's per-group pane order down to ids present in the read-only-file pane
set, write into `readOnlyFilePaneOrderByGroup` keyed consistently with
`paneOrderByRoot`/`workbenchGroupsByRoot`, so grouped document panes' split/
order persists and does not snap back.

Success: dragging/splitting grouped document panes persists correctly;
existing frontend tests (`test:workbench`, `test:resource-model`) stay green,
plus manual dogfood confirmation.
