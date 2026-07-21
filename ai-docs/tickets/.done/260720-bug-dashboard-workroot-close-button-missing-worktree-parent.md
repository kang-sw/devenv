---
title: Close ("X") button never renders on a base work root's label when it has associated worktrees
related:
  260714-bug-dashboard-workroot-close-button-hidden-when-selected: sibling case - that ticket covers the X disappearing on the *selected* workRoot/compactWorkRoot row (`!selected` gate); this ticket covers a base root with worktrees never getting a close X at all, in any selection state, because its row never reaches "workRoot"/"compactWorkRoot" presentation in the first place
related-mental-model:
  - ws-web-dashboard
completed: 2026-07-20
---

# Close ("X") button never renders on a base work root's label when it has associated worktrees

## Background

Dogfooding report (manual testing): the X (close) button is missing on the
label of a "root" work-root - the base repo directory that has one or more
associated git worktrees - as opposed to a plain worktree or a worktree-less
root, which do show their X. This is a **separate** case from the
already-tracked `260714-bug-dashboard-workroot-close-button-hidden-when-selected`
(which is about the X disappearing only once a row becomes selected): here the
X is absent unconditionally, selected or not.

Root cause, traced in `ws-dashboard/frontend/src/App.tsx` and
`ws-dashboard/frontend/src/resourceModel.ts`:

- `WorkspaceRows` (`App.tsx:9391-9508`) decides how a workspace renders based
  on `compactWorkspaceWorkRoot(workspace)` (`resourceModel.ts:400-408`), which
  returns `null` whenever `workspace.workRoots.length !== 1` - i.e. whenever
  the workspace's primary/base root has at least one linked worktree.
- When compact returns `null` (the has-worktrees case), `WorkspaceRows` falls
  into its second branch (`App.tsx:9453-9507`): it renders one depth-0 row with
  `presentation="workspace"` for the group/base root itself
  (`id={workspace.id}`, `actions={workspace.actions}`, no `isOpenWorkRoot`
  prop passed at all - defaults to `false`), and then a depth-1 `"workRoot"`
  row **only** for each child that passes `isWorkspaceNavChildWorkRoot`
  (`resourceModel.ts:522-524`), which keeps only `kind === "gitLinkedWorktree"`
  entries. The base root itself (`kind: "gitPrimaryRoot"`) is filtered out of
  `childWorkRoots` and never gets its own `"workRoot"`/`"compactWorkRoot"` row
  in this branch - it is represented solely by the `"workspace"`-presentation
  row.
- `ResourceRow`'s close-button gate is presentation-scoped and does not
  include `"workspace"`:

  ```ts
  const canCloseWorkRoot =
    (presentation === "workRoot" || presentation === "compactWorkRoot") &&
    isOpenWorkRoot &&
    !selected;
  ```

  (`App.tsx:9548-9551`). Since the base-root-with-worktrees row's
  `presentation` is `"workspace"`, `canCloseWorkRoot` is unconditionally
  `false` for it - independent of `selected`, and `isOpenWorkRoot` isn't even
  wired up for this row to begin with (see above).
- The row's only close-adjacent affordance is `hasWorkspaceRemove`
  (`App.tsx:9545-9547`), gated on an `action.id === "workspace.remove"` entry
  in `workspace.actions`, rendered via the "..." overflow menu
  (`App.tsx:9607-9649`) alongside "Add worktree...". `workspace.remove` is a
  destructive "remove workspace from registry" action, not the ordinary
  `workRoot.close` "unmount workbench, keep daemon session alive for
  reattach" affordance every other open work root gets via its X button.

This reads as an unhandled case rather than intentional suppression: nothing
in the code or `ai-docs/mental-model/ws-web-dashboard.md` documents an intent
to withhold the close/deselect affordance specifically from a workspace's
grouping row. `canCloseWorkRoot`'s presentation gate appears to have been
written with only the single-root (`"compactWorkRoot"`) and per-worktree-child
(`"workRoot"`) rows in mind, without accounting for the has-worktrees group
row (`"workspace"` presentation) that stands in for the base root in that
configuration.

## Constraints

- Do not conflate with `260714-bug-dashboard-workroot-close-button-hidden-when-selected`:
  that ticket's fix (making `canCloseWorkRoot` ignore `selected`) would not by
  itself fix this case, since this row's `presentation` never satisfies the
  `"workRoot" || "compactWorkRoot"` check regardless of `selected`.
- The base root in this configuration has no per-root id of its own surfaced
  to `ResourceRow` here - the row is keyed by `workspace.id`, and
  `workRoot.close`'s existing command builder
  (`buildWorkRootCloseCommand(id, actionServerId)`) expects a work-root id.
  Implementation will need to resolve which underlying `WorkRootView` (the
  `gitPrimaryRoot` entry in `workspace.workRoots`) a close/deselect on the
  `"workspace"` row should target, and whether `isOpenWorkRoot` needs to be
  computed and threaded into this row the same way it already is for
  `"compactWorkRoot"`/`"workRoot"` (`App.tsx:9438-9440`, `9487-9489`).
- `hasWorkspaceRemove` and any future `canCloseWorkRoot`-equivalent for the
  `"workspace"` row are two different actions (registry removal vs.
  close/unmount) and must remain distinguishable in the UI - do not merge them
  into one button.

## Phases

### Phase 1: Give the has-worktrees base-root row a close affordance

- Extend (or add a parallel) close-gate so the `"workspace"`-presentation row
  gets an X when its underlying base (`gitPrimaryRoot`) work root is open,
  wired to the same `workRoot.close` semantics used for `"compactWorkRoot"`/
  `"workRoot"` rows (or to whatever `selectedId`/deselect behavior
  `260714-bug-dashboard-workroot-close-button-hidden-when-selected` lands,
  if that ticket ships first - keep the two behaviors consistent).
- Thread `isOpenWorkRoot` (and the base root's own id) into the `"workspace"`
  `ResourceRow` call (`App.tsx:9453-9471`) the same way it's already computed
  for the compact and per-worktree-child rows.
- Verify manually and/or with a render-level test: a workspace with 2+
  `workRoots` (base + at least one linked worktree) shows an X on the base
  row when that root is open, in both selected and non-selected states.

### Result (commit `2717ac1b`) - 2026-07-20

Confirmed root cause: `WorkspaceRows` (`App.tsx` ~9407-9524) renders a base
work root that has one or more linked worktrees as a `presentation="workspace"`
row (the base `gitPrimaryRoot` entry is filtered out of `childWorkRoots` and
represented only by this row). `ResourceRow`'s `canCloseWorkRoot` gate only
accepted `presentation === "workRoot" || presentation === "compactWorkRoot"`,
so it was unconditionally `false` for the workspace row regardless of
`selected` - matching the ticket's trace exactly.

Fix (commit `2717ac1b`): extended `canCloseWorkRoot`'s presentation check to
also accept `"workspace"`; extracted a pure helper
`workspaceBaseWorkRoot(workspace)` in `resourceModel.ts` (deduplicating an
identical pre-existing inline lookup in
`resolveWorkbenchSelectionWithMatchInternal`); threaded `isOpenWorkRoot` and a
new `closeWorkRootId` prop (default `= id`) through `WorkspaceRows` into
`ResourceRow`, so the base-root workspace row's close button dispatches
`workRoot.close` for the resolved base-root id instead of `workspace.id`. `id`
itself was left untouched as the row's selection identity per the plan's
constraint (selection semantics were out of scope). `hasWorkspaceRemove` and
the "..." overflow menu are unchanged and remain a distinct affordance from
the new X.

Verification: `npm run build` clean; `npm run test:resource-model` clean,
including the new `workspaceBaseWorkRoot` unit test (covering both root
orderings) - reviewer independently confirmed the test is non-tautological by
reverting the helper to a naive lookup and observing the reversed-order
assertion fail; `npm run test:workbench` clean, no regression.

**Verification boundary** (recorded honestly): this frontend has no
render/DOM test harness (no jsdom/RTL/vitest), so the ticket's DOM acceptance
criterion - the X button actually appearing on the base row with linked
worktrees, in selected and non-selected states - is verified only at the
build + pure-logic-unit + code-review level. Live DOM confirmation is
deferred to manual dogfood. The full Playwright e2e suite was not used
(blocked by unrelated pre-existing ticket `260713`).

Review: partitioned fit=clean, test=clean with 1 accepted informational
minor - the inline `canCloseWorkRoot` boolean gate plus the
`isOpenWorkRoot`/`closeWorkRootId` wiring in `App.tsx` have no extracted
pure-logic test seam. This is a pre-existing coverage boundary (shared by
every other presentation's close-gate wiring), not introduced by this fix;
recorded, no action taken.

## Spec Impact

None. This restores the expected close affordance on a base work-root row
that has linked worktrees; it does not add or change any caller-visible
behavioral contract. `ai-docs/spec/ws-web-dashboard/index.md` does not
describe left-nav close-button ("X") presence/gating at the contract level
for any presentation (only the unrelated workbench-tab hover-close
affordance, `index.md:807`) - there is nothing to correct or extend.
Verified, no spec edit needed.
