# Plan: 260720-bug-dashboard-terminal-split-nonhorizontal-snap-back — Phase 1: Reproduce and root-cause non-horizontal split snap-back

> Verdict: concrete-plan. Root cause was CONFIRMED by an instrumented
> Playwright run against the daemon-served production frontend in this
> environment, and a candidate fix was applied temporarily and observed to
> eliminate the snap-back before being fully reverted. All instrumentation and
> the trial fix were reverted; the working tree is clean except this plan file.

## Relevant Ticket Contract
- Reproduce (a) a 3-way terminal split, (b) a vertical (top/bottom) 2-way
  split, and (c) a drop onto an existing inner group's edge; for each, capture
  whether the drop reverts at the same render tick.
- Instrument `onDidMovePanel` (`workbench/dockviewLayout.tsx:251`), `movePane`
  (`App.tsx:5817`), and `syncDockviewWorkbench`'s reconciliation
  (`workbench/dockviewLayout.tsx:403`) to see which step diverges from the
  working horizontal 2-way case. (Done — see Codebase Findings.)
- Confirm or rule out the user's "narrow prior fix" theory with runtime
  evidence, not assertion. (Done — see below.)
- Fix the root cause once identified; decide whether `260711` (read-only file
  pane analog) is folded in or stays separate, only after the terminal-pane
  root cause is confirmed.
- Verification: extend `dashboard-acceptance.spec.ts` with an e2e case that
  drops at a different position/orientation than the existing right-edge
  gesture in `expectDurableDockviewSplitDrop`, using a **terminal** pane.

## Out of Scope
- Fully resolving `260711` (read-only file pane registry-split). The fix below
  is pane-kind-agnostic (it lives in `movePane`, shared by terminal / readonly
  / agentChat panes) and is expected to also cover the readonly analog, but
  proving that and closing `260711` is a follow-up decision, not part of this
  slice.
- Reworking Dockview's own single-pane-source group-id reuse (mechanism 2
  below). Runtime evidence shows it preserves pane membership and does not
  produce the reported tab snap-back for the realistic ticket gestures, so it
  is documented but not fixed here.
- Any free-form arrangement beyond (a)/(b)/(c).

## Codebase Findings

### Environment / harness (confirmed runnable here)
- The browser gate runs in this WSL environment: `frontend/dist` build exists,
  the debug daemon binary `ws-dashboard/target/debug/ws-dashboard` exists,
  Playwright chromium is installed, and `cargo` is available. A temporary spec
  reusing `e2e/daemonHarness.ts#startDaemon` booted the daemon, paired, opened
  a workRoot, created terminals, and drove drag gestures headlessly.
- New terminals from `[data-command-id="terminal.create"]`
  (`App.tsx:createTerminalPane` ~5201) all land in the current group
  (`placeTerminalSessions`), so seeding a multi-pane group is just repeated
  clicks. Terminal tabs carry `data-workbench-pane-id^="terminal:"` and
  `data-workbench-group-id`.
- Dockview drop-zone threshold is 20% (`dockview-core/dist/cjs/dnd/droptarget.js`
  `DEFAULT_ACTIVATION_SIZE = {value:20,type:'percentage'}`): an edge split
  requires dropping in the outer 20% of a **specific group's** `.dv-groupview`
  rect (e.g. `(0.5, 0.95)` = bottom split, `(0.95, 0.5)` = right split).
  Dropping over the owner container at a boundary between two side-by-side
  groups hits the sash and fires no move — coordinates must be resolved from
  the target group's `.dv-groupview` bounding box, not the workbench owner.

### CONFIRMED root cause — mechanism 1 (PRIMARY; this is the reported bug)
Runtime instrumentation of `onDidMovePanel` + `movePane` +
`syncDockviewWorkbench` (temporary `console.debug`, captured via
`page.on("console")`) produced this decisive contrast for a **multi-pane
source group** (the realistic way every ticket scenario creates a new group):

- Horizontal right-edge drop (works): `onDidMovePanel` fires with a brand-new
  native Dockview group id, `mappedTargetGroupId` MISS → a new dashboard group
  is allocated; the arrangement persists.
- Vertical bottom-edge drop (snaps back): `onDidMovePanel` ALSO fires
  correctly with a new native id, `mappedTargetGroupId` MISS, and `movePane`
  correctly produces `result.groups = [group-1, group-2, group-3]` and mirrors
  the moved terminal into `terminalPaneOrderByGroup["group-3"]`. **But then the
  reconciliation reverts it**: `syncDockviewWorkbench` issues
  `existingPanel.api.moveTo({group: dock-group-1, ...})`, moving the panel from
  the new native group back into the original group — the visible snap-back.

The reason the reconciliation reverts is pinned exactly. Instrumenting the
fallback loop in `terminalWorkbenchPanesByGroup` (`App.tsx:8084-8087`) showed,
for the vertical case:
`{ paneId: <moved terminal>, fallbackGroup: "group-1",
   groupsList: ["group-1","group-2"],           // group-3 MISSING from the list
   orderKeys: ["group-1","group-2","group-3"],   // registry DOES have group-3
   orderForPane: ["group-3"] }`
i.e. `terminalPaneOrderByGroup` correctly records the moved terminal under
`group-3`, but the **group list** that `terminalWorkbenchPanesByGroup` iterates
(`dashboardGroups`, from `buildEditorGroupsForRoot` →
`resolveRootLayout(rootKey, workbenchGroupsByRoot, …)`) never contains
`group-3`. Because the iteration loop (`App.tsx:8075`) only visits groups in
that list, the moved terminal is never consumed under its true group and the
final fallback (`App.tsx:8084-8087`) dumps it into `groups[0]` = `group-1`.
`syncDockviewWorkbench` then reconciles Dockview to match and moves the panel
back.

Why the group list lacks the dynamically created group: **a state-key
mismatch unique to the drag/drop path.** `movePane` persists the post-move
group list and pane order keyed by the **bare** `workbenchModel.root.id`:
- `App.tsx:5858-5868` `onWorkbenchGroupsByRootChange(... [workbenchModel.root.id]: …)`
- `App.tsx:5869-5872` `onPaneOrderByRootChange(... [workbenchModel.root.id]: …)`

But every **reader** resolves layout under the **server-scoped** key
`serverScopedIdentity(root.resourcePath.serverId, root.id)` (=
`"<serverRoute>/<rootId>"`, `resourceModel.ts:81`):
- `App.tsx:4388-4401` computes `rootKey = serverScopedIdentity(...)` and passes
  it to `buildEditorGroupsForRoot` → `resolveRootLayout(rootKey, …)`
  (`workbench/layoutRestore.ts:332-354`, `groups = workbenchGroupsByRoot[rootKey]
  ?? restored?.groups ?? null`).

Every other **writer** already uses the scoped key — this is the clean,
existing convention the fix must follow, not a new mechanism:
- `openWorkRootActivityPane` (`App.tsx:5922`, `5935-5941`) writes
  `onWorkbenchGroupsByRootChange` under `rootKey = serverScopedIdentity(...)`.
- File-open placement (`App.tsx:988`) writes under `workRootStateKey =
  serverScopedIdentity(...)`.
- Restore-seed (`App.tsx:~901`) and cleanup (`App.tsx:~1135`) both key by the
  scoped `rootKey`.

So `movePane` is the sole path writing the wrong key; its dynamic-group and
pane-order writes land in a slot no reader consults, so drag/drop-created
groups are not reliably reflected in the derived group list (matching the
ticket's report that some topologies survive while others snap back).

Validation: temporarily changing only `movePane`'s two writes to key by
`serverScopedIdentity(workbenchModel.root.resourcePath.serverId,
workbenchModel.root.id)` and rebuilding produced, on re-run:
- vertical `group-3` persists in the DOM (`data-workbench-group-id="group-3"`),
- `terminalFallbackToGroup0` fired **0** times for the moved pane,
- `syncDockviewWorkbench` revert `moveTo` fired **0** times,
- the group list gained `group-3` (36 render logs).
The trial fix and all instrumentation were then reverted.

### CONFIRMED — mechanism 2 (single-pane-source id reuse; NOT the reported bug)
When the dragged pane is the **sole** pane of its source group, Dockview
empties+reuses that group's native id via `doRemoveGroup({skipDispose:true})` +
`doAddGroup` (`dockview-core/dist/cjs/dockview/dockviewComponent.js`
~`2311-2326` / `2042-2067`), so `move.panel.group.id` is still an
already-mapped id. `onDidMovePanel` (`dockviewLayout.tsx:258-267`) sees
`mappedTargetGroupId` HIT, sets `dynamicTargetGroup = undefined`, and calls
`movePane` with the pane's own current group → a no-op. Runtime evidence:
`onDidMovePanel {moveGroupId:"2", mappedTargetGroupId:"group-2",
dynamicTargetGroup:null}`. Pane **membership is preserved** (no group
collapse, no data loss) and Dockview retains the reposition, so this does NOT
manifest as the reported tab snap-back for the ticket's realistic gestures —
all three ticket scenarios (3-way, vertical, inner-edge) create a new group by
dragging out of a group that still holds ≥1 other pane, which is mechanism 1.
This mechanism is the survey's "id-reuse" hypothesis, now confirmed as real but
distinct and secondary.

### Pitfalls the executor should not re-derive
- The pure model `commitWorkbenchPaneMoveIntoDynamicGroup`
  (`workbench/editorGroupModel.ts`) and its unit test
  (`workbench/workbenchModel.test.ts`) are already correct and
  direction-agnostic; the bug is the **key used at the `movePane` call site**,
  not the model. A model unit test will not catch this — the e2e gate is the
  real regression guard.
- `terminalPaneOrderByGroup` is keyed by `groupId` (not rootKey); the mirror
  write at `App.tsx:5878-5894` is correct and must NOT be changed.
- The existing `expectDurableDockviewSplitDrop`
  (`e2e/dashboard-acceptance.spec.ts:396-462`) drags a **readonly** pane
  (`data-workbench-pane-id^="readonly"`) and hardcodes a single rightward
  outer-container gesture; it never exercised the terminal registry-split path.

## Implementation Plan

1. **Fix the drag/drop layout-persistence key mismatch** — `movePane`
   (`ws-dashboard/frontend/src/App.tsx:5857-5872`). Compute the scoped key once
   inside the `if (workbenchModel)` block:
   `const moveRootKey = serverScopedIdentity(
   workbenchModel.root.resourcePath.serverId, workbenchModel.root.id);`
   (`serverScopedIdentity` is already imported, `resourceModel.ts:81`) and key
   BOTH `onWorkbenchGroupsByRootChange` and `onPaneOrderByRootChange` by
   `moveRootKey` instead of `workbenchModel.root.id`. Behavioral change: a
   drag/drop-created dynamic group and its pane order are now persisted under
   the same key every reader and every other writer use
   (`openWorkRootActivityPane`, `App.tsx:5935`; file-open, `App.tsx:988`), so
   the derived `dashboardGroups` list includes the new group, the moved
   terminal is consumed under its true group instead of falling back to
   `groups[0]`, and `syncDockviewWorkbench` no longer reverts the panel. This
   is the validated fix. Do NOT touch the `setTerminalPaneOrderByGroup` mirror.

2. **Regression e2e — terminal-pane non-horizontal split durability**
   (`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`). Add a new
   `test.step` (or a terminal-specific helper alongside
   `expectDurableDockviewSplitDrop`) in the existing
   `"dashboard workRoot UI browser acceptance"` serial test, after terminals
   exist. It must drag **terminal** tabs
   (`.dockview-workbench-tab[data-workbench-pane-id^="terminal"]`), not the
   readonly pane, and cover at least these gestures, asserting durability after
   `settlePastPollCycle(page)`:
   - **Vertical 2-way**: seed a group with ≥2 terminals
     (`[data-command-id="terminal.create"]` twice), drag one terminal tab to
     the **bottom** edge of that group's `.dv-groupview` rect; assert the moved
     terminal ends in a new `data-workbench-group-id` (distinct from origin)
     that survives the poll settle, and the terminal pane is still visible.
   - **3-way**: from the resulting 2-group layout, seed another terminal into a
     multi-pane group and drag it to a fresh edge; assert ≥3 distinct
     `data-workbench-group-id`s persist through settle.
   - **Inner-group edge**: drag a terminal tab onto an **existing inner
     group's** edge (resolve the target group's `.dv-groupview` rect, not the
     outer owner) and assert the resulting split persists.
   - Drag mechanics: resolve `(fx,fy)` fractions against the target group's
     `.dv-groupview` bounding box (walk up from the tab/pane element to the
     `.dv-groupview` ancestor), use `page.mouse.move → down → move(center) →
     move(edge, {steps}) → up` (same pointer technique as
     `expectDurableDockviewSplitDrop`), and drop in the outer ~5% of the rect
     to clear Dockview's 20% edge threshold. This test FAILS pre-fix (moved
     terminal reverts to the origin group) and PASSES post-fix — confirmed by
     the instrumented trial run.

3. **`260711` decision (note, non-binding).** Because the fix is at the shared
   `movePane` call site, the readonly-file-pane registry-split snap-back
   (`readOnlyFilePaneOrderByGroup`, same `groups[0]` fallback shape) is expected
   to be resolved by the same change. Recommend adding a readonly-pane vertical
   drop assertion and, if green, folding `260711` closed with this ticket;
   otherwise keep it separate. Leave the final call to the lead.

## Verification Plan
- TDD-ish for step 2: add the terminal vertical-drop e2e assertion first and
  confirm it FAILS on unfixed `movePane` (terminal reverts to origin group),
  then apply step 1 and confirm it PASSES. (The instrumented trial already
  demonstrated both directions.)
- `cd ws-dashboard/frontend && npm run build && (cd .. && cargo build -p
  ws-dashboard-daemon) && npx playwright test dashboard-acceptance` — the full
  browser gate must stay green (the new terminal-split step plus all existing
  steps). `npm run test:browser` runs the same sequence with the build baked
  in.
- Manual smoke (optional, matches the dogfooding report): in the running
  dashboard, create ≥2 terminals in one group, drag one to the bottom/right
  edge and to an inner group's edge, and confirm the split stays after the next
  render tick.

## Escalations
None. Root cause confirmed with runtime evidence, fix validated and reverted,
harness proven runnable in this environment.
