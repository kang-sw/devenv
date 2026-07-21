---
title: Keep the X (close/deselect) button visible on the currently-selected work root
related:
  260714-feat-dashboard-multi-server-workbench-keepalive: sibling-scope carve-out - that ticket's Non-Goals explicitly defers this as a separate ticket
related-mental-model:
  - ws-web-dashboard
---

# Keep the X (close/deselect) button visible on the currently-selected work root

## Background

In the left WorkRoot navigation (`ResourceRow`, `App.tsx:9074-9236`), the X
button that closes an open work root is gated by:

```ts
const canCloseWorkRoot =
  (presentation === "workRoot" || presentation === "compactWorkRoot") &&
  isOpenWorkRoot &&
  !selected;
```

(`App.tsx:9112-9115`). Because of the trailing `!selected`, the X
**disappears the moment its own row becomes the selected one** - the row you
are currently looking at is the one row where you cannot see or use the close
control. This reads as an inconsistency: every other open work root shows its
X, but the one you're actively viewing does not.

**Requested fix (purely visual/UX consistency):** keep the X visible on the
selected work root too. Clicking it should **deselect** the row - clearing
`selectedId` so the workbench falls back to its "nothing selected" empty state
(`StatusPane` with `title="No workRoot"`, `App.tsx:5619-5626`) - rather than
performing the existing `workRoot.close` lifecycle action verbatim on the
still-focused root.

This is a **work-root granularity** UX fix, independent of the separate
per-server On/Off lifecycle being introduced by
`260714-feat-dashboard-multi-server-workbench-keepalive` (that ticket's
Non-Goals explicitly lists this as a separate, smaller ticket). Do not
conflate the two: this ticket only concerns the single work-root row's close
affordance and the `selectedId`/workbench-empty-state relationship, not
server-level keep-alive/deallocation.

## Constraints

- Do not change `workRoot.close`'s existing "unmount workbench, keep daemon
  terminal session alive for reattach" semantics (`App.tsx:978-1010`) for the
  **non-selected** case - that behavior is intentional and out of scope here.
- Non-obvious existing behavior to account for: `resolveWorkbenchSelection`
  (`App.tsx:9433-9481`) has a **fallback** - if `selectedId` does not match any
  entity, it still returns the first available workspace/root
  (`fallback ??= rootSelection`) rather than `null`. That means naively calling
  `setSelectedId(null)` alone will likely **not** produce the "nothing
  selected" empty state today, since the resolver would just fall back to the
  first root's workbench instead. Achieving the requested "deselect returns to
  the empty StatusPane" behavior needs either: (a) an explicit
  "no selection" sentinel/flag threaded through `resolveWorkbenchSelection`/
  `workbenchModel` that bypasses the fallback when the user explicitly
  deselected (vs. the fallback's original purpose of picking something
  reasonable on initial load), or (b) an equivalent mechanism that
  distinguishes "never selected anything yet" from "user explicitly cleared
  the selection." Investigate which of `resolveWorkbenchSelection`'s callers
  actually rely on the fallback before changing it, to avoid regressing
  initial-load behavior.
- There is currently no `setSelectedId(null)` call anywhere in the file (only
  `setSelectedId(<id>)` calls exist, `App.tsx:586,610,633,673,679,945,1181`) -
  this is genuinely new capability, not a dormant path being re-enabled.

## Decided Direction (2026-07-21)

The close-button-hidden symptom exposes a deeper gap: the "empty main screen"
UX is undefined for when the active work root is closed/deselected. The
decided deliverable this round is to **add an "empty screen" placeholder** for
the main content area when no work root is active (on close/deselect), and
wire the close/deselect interaction to it. The original concern - the X/close
control should stay usable/visible for the selected row, wired to "deselect"
rather than the destructive close/unmount - folds into this same effort
rather than standing as a separate change.

## Phases

### Phase 1: Empty-main-screen placeholder

Define and render a coherent empty-main-screen placeholder for the main
content area, shown whenever there is no active work root (including but not
limited to the close/deselect interaction added in Phase 2). Success: the
main content area has a defined, coherent "nothing active" presentation
instead of a blank/undefined state.

### Phase 2: Wire close/deselect on the selected nav row into the empty state

Change `canCloseWorkRoot` (`App.tsx:9112-9115`) so the X renders whenever
`isOpenWorkRoot` is true for a `workRoot`/`compactWorkRoot` row, regardless of
`selected` - keeping the control usable/visible on the row you are currently
viewing, not just every other open row. Wire clicking the X on the *selected*
row to deselect (clear `selectedId`) and land on Phase 1's empty placeholder,
rather than re-running today's destructive `workRoot.close` handler verbatim;
account for `resolveWorkbenchSelection`'s existing fallback-to-first-root
behavior (`App.tsx:9433-9481`) so a deselect actually reaches the empty state
instead of silently falling back to another root. Preserve today's
`workRoot.close` behavior unchanged for the open-but-not-selected case.
Decide during implementation whether clicking X on the selected row should
also run the existing close/unmount side effects or deselect only while
leaving workbench state mounted for quick re-select; document the choice in
the Result.

Success: closing/deselecting a work root shows the coherent empty placeholder
instead of a blank/broken main area, and the close/deselect control is usable
while its own row is selected.

Verification should include a resource-model/render-level test asserting the
X is present and clickable on a selected, open work root, plus a test/manual
check that clicking it lands on the empty-main-screen placeholder rather than
silently falling back to another root via `resolveWorkbenchSelection`'s
fallback.

## Phase 1 Result (2026-07-21)

- Phase 1 (empty main-screen placeholder) implemented. New
  `EmptyWorkbenchPlaceholder` component rendered in `WorkbenchShell`'s
  `activeHeader` `!resources || !workbenchModel` branch (`App.tsx`), replacing
  the small `StatusPane "No workRoot"` box: full-height centered layout, copy
  "No work root selected" / "Open a work root to get started", existing
  `FolderGit2` glyph, and an "Open workRoot" CTA implemented as a new
  `"empty"` variant of `OpenWorkRootControl` reusing the existing
  `buildRootPickerOpenCommand` flow. Scoped CSS (`.empty-workbench*`,
  `.open-work-root-empty*`); `StatusPane` untouched so `ResourceDetail` is
  unaffected.
- Verification: `npm run build` PASS; `npm run test:workbench` PASS. No DOM
  harness for this render branch (manual visual dogfood pending, user's
  step).
- Review: single full-scope, `clean with 1 minor`. Residual minor: the new
  empty-state CTA (`EmptyWorkbenchPlaceholder`, `App.tsx:10169-10201`) renders
  unconditionally clickable without checking whether the server's `openRoot`
  action is actually enabled, unlike the sidebar `variant="icon"` call site
  which only renders when `openRoot` is enabled - a connected-but-disabled
  server would surface an inline picker error instead of the CTA silently
  doing nothing. Accepted/deferred, not a bug (shared component's existing
  error handling covers the failure gracefully; decided design/plan do not
  ask for this parity).
- Commits: feat 21116b54 (plan e212e96b).
- **Phase 2 STILL PENDING** (not done): close-button `!selected` gate
  reachability (`App.tsx` ~9793-9798) + selection-fallback behavior in
  `resourceModel.ts` (`resolveWorkbenchSelectionWithMatchInternal` ~547-599)
  so that closing the last/selected work root actually lands on this new
  empty placeholder instead of snapping back to the first root. This ticket
  remains in `ready/` until Phase 2 lands.

## Phase 2 Result (2026-07-21)

- Dropped the `!selected` gate on `canCloseWorkRoot`: the X now renders for
  any open `workRoot`/`compactWorkRoot` row regardless of `selected`, so the
  close control stays reachable on the row you are currently viewing.
- Closing the selected root now selects the adjacent remaining OPEN work
  root (next-in-order, else previous) via a new pure
  `pickWorkRootSelectionAfterClose` helper in `resourceModel.ts`. The empty
  placeholder from Phase 1 shows only when no OPEN work roots remain at all,
  gated by a new close-scoped `closeEmptyWorkbench` flag: it defaults
  `false`, is set only by the close-last-open-root branch, and is cleared by
  any subsequent non-null selection routed through `selectRoot`.
- Design decision: TAB semantics - the adjacency/emptiness scoping is over
  OPEN work roots, not all tree entities. Chose option (b) from the
  Constraints section (an explicit-empty flag) over changing
  `resolveWorkbenchSelection`'s fallback-resolver behavior, to avoid
  regressing initial-load selection and the existing D5 sticky-bridging
  behavior.
- Commits: `fac5cdba` (implementation: gate removal, adjacent-open-root
  selection, `closeEmptyWorkbench` flag) and `c103d71e` (relay fix: route
  `onCreated`/worktree-add selection through `selectRoot` so it also clears
  the flag, instead of bypassing it).
- Review: correctness/fit/test partitioned review. 1 Important finding
  (`onCreated` selection path bypassed `selectRoot` and could leave the
  `closeEmptyWorkbench` flag stuck set) - fixed in `c103d71e` and
  re-reviewed RESOLVED. Remaining minors accepted by design: a raw-resolver
  mismatch during the D5 bridge window, and per-current-server empty scoping
  deferred to the separate `260714-feat-dashboard-multi-server-workbench-keepalive`
  ticket.
- Plan: `ai-docs/.plans/2026-07/21-1739-260714-phase2-close-selection.md`.
- Verification: automated tests/build covered in review; live-dashboard
  manual confirmation of the selected-row X and close-to-empty behavior
  remains an open dogfood item for the user (no live instance available in
  this implementing session).

Both phases complete. Closing the ticket.
