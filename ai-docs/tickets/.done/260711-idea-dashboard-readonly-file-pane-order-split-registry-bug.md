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

## Result (2026-07-21)

- **Diagnosis:** H-LATENT confirmed, not a regression from the terminal-split
  fix `d8e71b51`. `readOnlyFilePaneOrderByGroup` was never mirror-written
  inside `movePane`; the gap predates `d8e71b51` and is the exact parallel
  latent issue this ticket hypothesized. Commit `125d68e1` (2026-07-14)
  already documents the gap in the terminal mirror path (comparing against
  `paneId`, not `logicalKey`), confirming the pre-existing shape.
- **Fix:** added the read-only pane-order mirror write inside `movePane`,
  filtered by `pane.id` (the `WorkbenchPane`-id space) to match the terminal
  mirror's fix pattern; the registry itself is left flat/group-id-keyed by
  design (no `rootKey` layer added). The filtering logic was then extracted
  into a pure exported helper, `filterPaneOrderByPaneIds`, in
  `ws-dashboard/frontend/src/workbench/layoutRestore.ts`, and both the
  terminal and read-only mirror closures in `App.tsx` were routed through it.
  A `layoutRestore.test.ts` case was added and mutation-verified to catch the
  wrong-id-space silent-no-op class seen in `bc566a78`/`125d68e1`.
- **Verification:** `npm run build`, `test:workbench`, and
  `test:resource-model` all pass. Review passes: correctness review (opus)
  clean, fit review clean, one test-coverage finding raised and fixed, delta
  re-review clean.
- **Commits:** `ba9d858a` (fix: mirror `movePane` result into
  `readOnlyFilePaneOrderByGroup`), `f93b7da8` (test: extract `movePane`
  pane-order filter into tested helper `filterPaneOrderByPaneIds`); plan
  recorded at `8a42982e`.
- **Doc check:** grepped `ai-docs/spec/` and `ai-docs/mental-model/` for
  `paneOrder`/`movePane`/`readOnlyFilePaneOrderByGroup`/
  `terminalPaneOrderByGroup`/pane-order/workbench surfaces. No doc describes
  `movePane`'s per-registry mirroring behavior at this level of concreteness;
  the existing mental-model workbench statements remain accurate at their
  altitude. No doc edit made.
- **Open item:** manual dogfood confirmation in the running dashboard remains
  the user's step — no live dashboard instance was available in the
  implementing session to re-verify beyond the automated test suites.
