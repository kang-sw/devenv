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

## Phases

### Phase 1: Show X on the selected work root and wire it to deselect

- Change `canCloseWorkRoot` (`App.tsx:9112-9115`) so the X renders whenever
  `isOpenWorkRoot` is true for a `workRoot`/`compactWorkRoot` row, regardless
  of `selected`.
- When the row is selected, clicking the X should deselect (clear
  `selectedId`) and result in the workbench showing its "nothing selected"
  `StatusPane` (`App.tsx:5619-5626`) - not simply re-run today's
  `workRoot.close` handler as-is, since that handler doesn't touch
  `selectedId` at all today and (per the Constraints note above) clearing
  `selectedId` alone may not surface the empty state without also addressing
  `resolveWorkbenchSelection`'s fallback behavior.
- When the row is open-but-not-selected (today's existing case), preserve
  current `workRoot.close` behavior unchanged.
- Decide during implementation whether clicking X on the *selected* row should
  also run the existing close/unmount side effects (i.e., both deselect and
  close), or deselect only while leaving the root's workbench state mounted
  for a quick re-select - the ticket only mandates the visible/clickable X and
  the resulting empty-state landing; pick whichever keeps the two lifecycles
  (this ticket's deselect vs. `workRoot.close`'s unmount-keep-daemon-alive)
  coherent and document the choice in the Result.

Verification should include a resource-model/render-level test asserting the X
is present and clickable on a selected, open work root, plus a test/manual
check that clicking it lands on the "nothing selected" `StatusPane` rather than
silently falling back to another root via `resolveWorkbenchSelection`'s
fallback.
