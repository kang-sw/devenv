---
title: "Terminal pane split reverts at drop for 3-way, vertical, and other non-horizontal Dockview arrangements"
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260714-bug-dashboard-terminal-pane-split-mirror-key-mismatch: closed sibling fix for the "move into another existing group" terminal snap-back; this ticket's repro is the same registry-split mechanism but for topologies (3-way, vertical, free-form) that fix was never proven against
  260711-idea-dashboard-readonly-file-pane-order-split-registry-bug: parallel open investigation into the same registry-split shape for read-only file panes (still unresolved)
completed: 2026-07-20
---

# Terminal pane split reverts at drop for 3-way, vertical, and other non-horizontal Dockview arrangements

## Background

Live dogfooding report (2026-07-20, manual testing, translated from Korean):
dragging a terminal pane in the Dockview workbench to create a horizontal
2-way split (left-right) works and the arrangement survives the next render.
Dragging to produce a 3-way split, a vertical (top-bottom) split, or other
free-form Dockview split arrangements does not — the layout visibly snaps
back to its pre-drop arrangement right at drop time.

User's suspicion: a prior fix (commit `bc566a78`, "mirror terminal pane
moves into terminalPaneOrderByGroup", 2026-07-11) may only have narrowly
special-cased the 2-way horizontal scenario it was built and tested
against, rather than fixing drag-and-drop split handling in general.

## Investigation (this capture pass — no product code touched)

- Read `bc566a78`: it added a mirror write in `movePane` (`App.tsx`) so a
  drag/drop result also updates `terminalPaneOrderByGroup` — a separate flat
  pane-order registry that `terminalWorkbenchPanesByGroup`
  (`App.tsx:8044-8078`) falls a pane back to `groups[0]` for when the pane's
  id is missing from it. That mirror's filter compared `paneId`-space ids
  against a `logicalKey`-keyed map (`id in terminalPanes`), which was always
  `false` — so the mirror emptied every group's terminal order on *every*
  move and never actually worked. This exact defect was root-caused and
  fixed under `260714-bug-dashboard-terminal-pane-split-mirror-key-mismatch`
  (commit `125d68e1`, 2026-07-14): the filter now builds `livePaneIds` from
  `Object.values(terminalPanes).map(pane => pane.paneId)`.
- Read the current (HEAD `e967b4d2`) state of the full path:
  `movePane` (`App.tsx:5817-5873`) ->
  `commitWorkbenchPaneMoveIntoDynamicGroup`
  (`workbench/editorGroupModel.ts:242-287`) -> Dockview's `onDidMovePanel`
  handler (`workbench/dockviewLayout.tsx:251-289`), which allocates a new
  dashboard group id via `nextDynamicWorkbenchGroupId`
  (`workbench/dockviewLayout.tsx:543-553`) whenever a move lands in a native
  Dockview group id not yet present in `dockGroupToWorkbenchGroupRef`.
- None of this path branches on split direction/orientation or on group
  count — grepping App.tsx and workbench/*.ts(x) for
  `direction ===`/`vertical`/`horizontal`/`groups.length` turns up nothing
  scoped to "exactly 2 groups" or "horizontal only" in the move/mirror/
  dynamic-group-creation code itself.
- `commitWorkbenchPaneMoveIntoDynamicGroup` is unit-tested past the 2-way
  case: `workbench/workbenchModel.test.ts` (~505-536) asserts a move into an
  unmapped target creates `group-3` with `createdGroupId: "group-3"`, and
  that test passes. The pure state-transition model looks direction-agnostic
  and is exercised beyond 2-way at the unit level.
- However, the **only** browser/e2e verification of a live Dockview
  split-drop, `expectDurableDockviewSplitDrop`
  (`e2e/dashboard-acceptance.spec.ts:396-462`), always performs the exact
  same single gesture: drag to
  `(ownerBox.x + width * 0.95, ownerBox.y + height * 0.5)` — a rightward
  drop against the *outer container's* right edge. There is no e2e coverage
  of a vertical (top/bottom edge) drop, a drop onto an *existing inner
  group's* edge (as opposed to the outer container edge), or a genuine
  T-shaped/3-way arrangement produced by a different gesture. So even
  though the state model looks direction-agnostic in isolation, nothing has
  ever proven other topologies survive the full mount -> native Dockview
  drop -> `onDidMovePanel` -> React state update -> `syncPanels`
  reconciliation cycle end-to-end.
- This exact code area has a track record of "fix the tested/observed case,
  flag-but-defer the rest": `bc566a78`'s own commit message flagged
  `readOnlyFilePaneOrderByGroup` as having "the same registry-split shape
  ... left untouched — out of scope for this surgical fix", which produced
  `260711-idea-dashboard-readonly-file-pane-order-split-registry-bug` — that
  ticket is still open/unresolved as of this session.

**Honest assessment**: static reading alone did not turn up a second
concrete state-level bug (parallel to the paneId/logicalKey mismatch
already fixed in `260714`) that would explain specifically
vertical/3-way terminal drops reverting while horizontal 2-way persists —
the mirror-write and dynamic-group-creation logic do not appear to branch
on topology anywhere in the current code. The strongest corroborated lead
is the one-directional e2e coverage above: it is fully consistent with (and
would explain) a bug that only manifests for topologies the fix was never
proven against, even without an explicit code-level guard. Whether the
narrow-fix suspicion is literally true (an undiscovered topology-specific
branch/bug) or the gap is really "only ever verified in one direction, and
a related but distinct defect lurks for the others" needs live/interactive
reproduction to settle — out of scope for this capture pass per
instruction not to fix or further debug live.

## Phases

### Phase 1: Reproduce and root-cause non-horizontal split snap-back

Reproduce in the running dashboard:

- (a) a 3-way terminal split (add a third split beyond an existing 2-way
  arrangement),
- (b) a vertical (top/bottom) 2-way terminal split,
- (c) at least one other free-form arrangement (e.g., drop onto an
  *existing inner group's* edge rather than the outer container edge).

For each, capture whether the drop reverts at the same render tick, and
instrument (temporary logging is fine) `onDidMovePanel`
(`workbench/dockviewLayout.tsx:251`), `movePane` (`App.tsx:5817`), and
`syncDockviewWorkbench`'s reconciliation (`workbench/dockviewLayout.tsx:403`)
to see exactly which step diverges from the working horizontal 2-way case
— e.g., does the new dashboard group get created and then get thrown away
by the next `syncPanels`, does `onDidMovePanel` even fire with the expected
new group id for these gestures, or does something else entirely happen.
Confirm or rule out the user's "narrow prior fix" theory concretely.

Fix the root cause once identified. If the fix generalizes to other pane
kinds (read-only file, agent chat) sharing the same registry-split shape,
decide whether `260711` should be resolved together with this ticket or
stay separate.

**Verification**: extend `dashboard-acceptance.spec.ts` with an e2e case
that drops at a different position/orientation than the existing
right-edge gesture in `expectDurableDockviewSplitDrop`, so this topology
gets real end-to-end coverage going forward instead of relying on the one
direction currently exercised.

### Result (commits `1a6a734b`, `d8e71b51`) - 2026-07-20

Confirmed root cause (runtime-instrumented, not speculative): `movePane`
(`App.tsx`) persisted the drag/drop-created dynamic group and pane order
under the bare `workbenchModel.root.id`, while every reader
(`resolveRootLayout` via `buildEditorGroupsForRoot`) and every other writer
(`openWorkRootActivityPane`, file-open placement, restore-seed, cleanup)
keyed by `serverScopedIdentity(root.resourcePath.serverId, root.id)`. Because
the moved terminal's true dynamic group was written under the wrong key, it
never appeared in the group list `terminalWorkbenchPanesByGroup` iterates
(`App.tsx:8075`); the moved pane fell into that loop's `groups[0]` fallback
(`App.tsx:8084-8087`), and `syncDockviewWorkbench` then reconciled Dockview's
panel back to match the fallback - the visible snap-back. This reproduced
only for a drag out of a *multi-pane* source group (all three ticket
scenarios); dragging a group's sole pane hits a separate, unrelated
Dockview native-group-id-reuse behavior, documented in the plan as
"mechanism 2" and confirmed not to cause the reported symptom.

Fix: `movePane`'s two writes (`onWorkbenchGroupsByRootChange`,
`onPaneOrderByRootChange`) now key by `serverScopedIdentity(...)` instead of
the bare root id (commit `d8e71b51`), matching every other writer/reader.
`setTerminalPaneOrderByGroup`'s mirror write was left untouched - it is
correctly keyed by plain `groupId`, not `rootKey`. A terminal-pane
non-horizontal e2e regression step (vertical 2-way, 3-way, and
existing-inner-group-edge splits) was added to
`dashboard-acceptance.spec.ts` ahead of the fix, TDD-style (commit
`1a6a734b`).

Validation: the full `dashboard-acceptance.spec.ts` suite is blocked before
reaching the new step by the pre-existing, unrelated
`260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` defect,
confirmed present identically on the pre-change base commit (not a
regression from this work). Validation therefore used an isolated, deleted
probe spec reusing `e2e/daemonHarness.ts#startDaemon`: pre-fix FAIL (3-way
drop reverts to the origin group), post-fix PASS (vertical/3-way/inner-edge
all persist through settle). `npm run build`, `cargo build -p
ws-dashboard-daemon`, and `npm run test:workbench` all passed.

Review: partitioned correctness/fit/test review (opus) came back clean on
all three axes, with 2 accepted non-blocking minors: (a) the inner-edge case
uses a `>=3` group-count threshold where `>=4` would be marginally sharper,
but the pairwise `not.toBe` group-id checks already make the weaker
threshold safe; (b) one `not.toBe` assertion in the new step redundantly
duplicates a check `expectDurableTerminalSplitDrop` already performs
internally. Both accepted as-is, no follow-up needed.

Coverage-activation dependency (cross-ticket, not a gap in this fix): the
new e2e regression guard will not execute in the real
`dashboard-acceptance` suite / CI until
`260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` is fixed,
since that defect aborts the suite before reaching the new step today. The
guard is committed and proven correct via the isolated probe above; it is
inert in CI only until 260713 is separately resolved.

Per lead decision: readonly-pane e2e coverage was intentionally NOT added
in this ticket, and `260711` (read-only file pane registry-split analog) is
NOT moved or closed here. The `movePane` fix is at the shared call site
used by terminal/readonly/agentChat panes alike and is expected to also
resolve the readonly analog, but confirming and closing that is left to
`260711` as a separate follow-up.

## Spec Impact

None yet identified. This is a gap in already-intended behavior — per the
`260517-bug-ws-dashboard-dockview-dynamic-groups` decisions, a Dockview
split-drop preview should always become durable dashboard arrangement
state regardless of topology. No existing spec stem covers pane
drag-move/split-registry behavior at the contract level (already noted in
`260714`). Contract-first spec: no.

**Post-fix verification**: `ai-docs/spec/ws-web-dashboard/index.md:796-798`
already states "Dockview-created split drops become durable dashboard
workbench groups instead of snapping back" as the intended contract — this
fix corrects an internal persistence-key bug that violated that
already-documented behavior for non-horizontal topologies; it introduces no
new caller-visible contract. Verified, no spec edit needed.
