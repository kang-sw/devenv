# Plan: 260720-bug-dashboard-workroot-close-button-missing-worktree-parent — whole target

## Relevant Ticket Contract
- A base work root (`kind: "gitPrimaryRoot"`) that has one or more linked
  worktrees renders as a `presentation="workspace"` `ResourceRow`, not
  `"workRoot"`/`"compactWorkRoot"`, so it never gets a close (X) button in any
  selection state.
- Fix shape (Phase 1): extend the close-gate so the `"workspace"`-presentation
  row gets an X when its underlying base root is open, wired to the same
  `workRoot.close` semantics as `"compactWorkRoot"`/`"workRoot"` rows; thread
  `isOpenWorkRoot` and the base root's own id into that row the same way it is
  already computed for the compact/child rows.
- Constraint: do not conflate with sibling ticket
  `260714-bug-dashboard-workroot-close-button-hidden-when-selected` (the
  `!selected` gate) — that ticket is still `todo/` and unfixed; this plan keeps
  `!selected` behavior as-is for every presentation, including the new one.
- Constraint: `hasWorkspaceRemove` (registry-remove, via the "..." menu) and
  the new close/unmount affordance must stay two distinguishable UI elements;
  do not merge them.
- Verification boundary: "manually and/or with a render-level test" that a
  workspace with base + >=1 linked worktree shows the X on the base row when
  open, in both selected and non-selected states.

## Out of Scope
- Fixing `260714`'s `!selected` gate itself (sibling ticket, still unfixed in
  current source — verified `App.tsx:9564-9567` still has the plain
  `!selected` check).
- Any change to `hasWorkspaceRemove` / the overflow "Remove workspace..."
  action.
- Adding a browser-rendering test harness (jsdom/React Testing Library) to
  this frontend — none exists today (see Codebase Findings); introducing one
  is a test-infra decision beyond this bug's scope.
- The full Playwright e2e/acceptance suite — currently blocked by unrelated
  pre-existing ticket `260713`; do not rely on it turning green as part of
  this fix's verification.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx:9407-9524` (`WorkspaceRows`) — confirmed
  current structure matches the ticket's trace (line numbers have drifted
  slightly from the ticket's citation but the logic is unchanged):
  - `compactWorkspaceWorkRoot(workspace)` (`resourceModel.ts:400-408`) returns
    `null` whenever `workspace.workRoots.length !== 1`.
  - When `null`, the second branch (`App.tsx:9469-9523`) renders one depth-0
    `presentation="workspace"` row keyed on `workspace.id` with no
    `isOpenWorkRoot` prop passed (defaults `false`, `App.tsx:9540`), plus one
    depth-1 `presentation="workRoot"` row per `childWorkRoots` — i.e. per
    `root.kind === "gitLinkedWorktree"` entry
    (`isWorkspaceNavChildWorkRoot`, `resourceModel.ts:522-524`). The base
    `gitPrimaryRoot` entry is filtered out of `childWorkRoots` and is
    represented only by the `"workspace"` row.
- `ws-dashboard/frontend/src/App.tsx:9564-9567` — `canCloseWorkRoot` gate,
  confirmed as cited:
  ```ts
  const canCloseWorkRoot =
    (presentation === "workRoot" || presentation === "compactWorkRoot") &&
    isOpenWorkRoot &&
    !selected;
  ```
  `"workspace"` is not in the presentation check, so this is unconditionally
  `false` for the has-worktrees base-root row.
- `ws-dashboard/frontend/src/App.tsx:9619` — the close button's `onClick`
  calls `buildWorkRootCloseCommand(id, actionServerId)`, where `id` is the
  `ResourceRow`'s `id` prop. For the `"workspace"` row `id={workspace.id}`
  (`App.tsx:9472`), which is **not** a work-root id — confirms the ticket's
  constraint that a separate id must be resolved and threaded in, distinct
  from the row's identity/selection `id`.
- `ws-dashboard/frontend/src/resourceModel.ts:541-547`
  (`resolveWorkbenchSelectionWithMatchInternal`) already contains the exact
  lookup needed for "the root a workspace's own row represents when it does
  not compact":
  ```ts
  const workspaceRoot =
    workspace.workRoots.find((root) => !isWorkspaceNavChildWorkRoot(root)) ??
    workspace.workRoots[0] ??
    null;
  ```
  This is a directly reusable existing mechanism — extract it into a small
  named helper and use it both here and in `WorkspaceRows`, instead of
  inventing new lookup logic or hardcoding `kind === "gitPrimaryRoot"`.
- `ws-dashboard/frontend/src/App.tsx:9454-9456` and `9503-9505` — the existing
  pattern for computing `isOpenWorkRoot` on a row:
  `openWorkRootKeys.has(serverScopedIdentity(serverId, <root>.id))`. Reuse
  verbatim for the resolved base root.
- `ws-dashboard/frontend/src/App.tsx:9526-9560` — `ResourceRow` props/type.
  `id` is reused for three purposes today (DOM `data-resource-id`, the
  `resource.select` command's `entityId`, and the close command's
  `workRootId` argument). For `"workRoot"`/`"compactWorkRoot"` these three
  purposes coincide (the row's `id` already *is* a work-root id), so no prop
  split was previously needed. For `"workspace"` they diverge: `id` must stay
  `workspace.id` (selection semantics — `selectedWorkspace`,
  `App.tsx:9427-9432`, already treats `selectedId === workspace.id` as one hit
  case; changing `id` itself would risk altering selection behavior, which is
  out of scope). A new optional `closeWorkRootId` prop (defaulting to `id`) is
  the minimal, additive way to let the close action target a different id
  than the row's selection identity, without touching any other presentation.
- `ws-dashboard/frontend/src/commands.ts:359-367` —
  `buildWorkRootCloseCommand(workRootId, serverRoute?)` confirms the command
  shape; no change needed there.
- No render/component test harness exists in this frontend at all — verified
  via `ws-dashboard/frontend/package.json` (no `vitest`, `jsdom`,
  `@testing-library/*` dependency) and `ws-dashboard/frontend/vite.config.ts`
  (no test config). Every `*.test.ts` under `ws-dashboard/frontend/src/`
  (e.g. `resourceModel.test.ts`) is a plain top-level `assertEqual`/
  `assertTrue` script compiled by `tsc` and run directly with `node` — pure
  logic only, no JSX/DOM rendering, no `describe`/`it`. `App.tsx`'s React
  components (`WorkspaceRows`, `ResourceRow`) have zero existing test
  coverage of this kind. A true DOM "render-level" test of the X button
  appearing is therefore not feasible without adding new test infrastructure,
  which is out of scope for this fix; see Verification Plan for the
  achievable substitute.
- `ws-dashboard/frontend/src/resourceModel.test.ts:291-311` — `multiRootWorkspace`
  fixture (two `plainDirectory` roots) is the nearest existing has-multiple-
  workRoots fixture pattern for `compactWorkspaceWorkRoot`; the `workRoot(id,
  workspaceId, label, mainInstances?)` factory (`resourceModel.test.ts:102-121`)
  defaults `kind: "plainDirectory"` and existing tests mutate `.kind`/other
  fields after construction (e.g. `offlineUnavailableRoot`,
  `resourceModel.test.ts:313-316`) — same pattern to use for a
  `gitPrimaryRoot` + `gitLinkedWorktree` fixture pair.
- `ws-dashboard/crates/daemon/src/git_worktree.rs:446` —
  `.find(|root| root.kind == WorkRootKind::GitPrimaryRoot)` is the backend's
  own way of locating the primary root among a workspace's roots, confirming
  "find the base root among `workRoots`" is an established, expected
  operation on this data shape (further support for extracting a shared
  frontend helper rather than inlining an ad hoc lookup).
- CSS (`ws-dashboard/frontend/src/styles.css`): `.resource-row-action`,
  `.resource-row-actions`, and the one `[data-resource-presentation="workRoot"]`
  rule (`styles.css:2588`, indentation-only) do not gate the close button's
  visibility — visibility is purely the `canCloseWorkRoot` JS condition
  (`App.tsx:9610-9622`). No CSS changes needed.

## Implementation Plan

1. `ws-dashboard/frontend/src/resourceModel.ts` — add a small exported helper
   near `isWorkspaceNavChildWorkRoot` (after line 524), reusing the existing
   lookup verbatim:
   ```ts
   // The root a workspace's own row represents when the workspace does not
   // compact to a single row: the primary/base root if present, otherwise the
   // first workRoot. Shared by workbench-selection resolution and the
   // "workspace"-presentation resource row's close affordance.
   export function workspaceBaseWorkRoot(
     workspace: WorkspaceView,
   ): WorkRootView | null {
     return (
       workspace.workRoots.find((root) => !isWorkspaceNavChildWorkRoot(root)) ??
       workspace.workRoots[0] ??
       null
     );
   }
   ```
   Then replace the inline duplicate at `resourceModel.ts:544-547`
   (`resolveWorkbenchSelectionWithMatchInternal`) with
   `const workspaceRoot = workspaceBaseWorkRoot(workspace);` to remove the
   duplication instead of leaving two copies of the same lookup.

2. `ws-dashboard/frontend/src/App.tsx` — add `workspaceBaseWorkRoot` to the
   existing `resourceModel` named-import block (~line 259-274), alphabetically
   next to `workRootActivationEndpoint`/`workspaceEndpoint`.

3. `ws-dashboard/frontend/src/App.tsx:9469-9487` (`WorkspaceRows`, second
   branch, the `"workspace"`-presentation `ResourceRow` call) — resolve the
   base root and thread it in:
   ```tsx
   const baseRoot = workspaceBaseWorkRoot(workspace);
   ```
   (place this near `compactRoot`/`childWorkRoots`, before the `if
   (compactRoot)` early return, so it's available in the second branch), then
   on the `"workspace"` `ResourceRow` add:
   ```tsx
   closeWorkRootId={baseRoot?.id}
   isOpenWorkRoot={
     baseRoot != null &&
     openWorkRootKeys.has(serverScopedIdentity(serverId, baseRoot.id))
   }
   ```
   mirroring the existing pattern at `App.tsx:9454-9456`/`9503-9505`. Leave
   every other prop on this row unchanged.

4. `ws-dashboard/frontend/src/App.tsx:9526-9567` (`ResourceRow`) —
   - Add `closeWorkRootId?: string` to the props type and destructure with
     default `closeWorkRootId = id` (so `"workRoot"`/`"compactWorkRoot"`
     callers, which never pass it, keep today's behavior unchanged).
   - Extend the presentation check in `canCloseWorkRoot`:
     ```ts
     const canCloseWorkRoot =
       (presentation === "workRoot" ||
         presentation === "compactWorkRoot" ||
         presentation === "workspace") &&
       isOpenWorkRoot &&
       !selected;
     ```
     No extra "did we resolve a base root" guard is needed here: when
     `WorkspaceRows` cannot resolve a `baseRoot` it already passes
     `isOpenWorkRoot={false}` for that row (step 3), which alone keeps
     `canCloseWorkRoot` `false` — so the close button can never render (and
     `buildWorkRootCloseCommand` can never be called) with a missing/wrong
     target id.
   - In the close button's `onClick` (`App.tsx:9618-9620`), call
     `onCommand(buildWorkRootCloseCommand(closeWorkRootId, actionServerId))`
     instead of `buildWorkRootCloseCommand(id, actionServerId)`.
   - Leave `hasWorkspaceRemove` and the overflow menu entirely untouched —
     they remain a separate action rendered independently
     (`App.tsx:9610-9622` still shows both via `hasWorkspaceRemove ||
     canCloseWorkRoot` and independent `canCloseWorkRoot ? ... : null` /
     `hasWorkspaceRemove ? ... : null` blocks).

5. `ws-dashboard/frontend/src/resourceModel.test.ts` — add a unit test for the
   new `workspaceBaseWorkRoot` helper, mirroring the existing
   `multiRootWorkspace` fixture style (lines 291-311) and the
   mutate-after-construction pattern (lines 313-316):
   - Build a workspace with two `workRoot(...)` entries; set the first's
     `.kind = "gitPrimaryRoot"` and the second's `.kind = "gitLinkedWorktree"`.
   - Assert `workspaceBaseWorkRoot(workspace)?.id` equals the primary root's
     id (covers the exact has-worktrees shape this bug is about).
   - Add a second assertion with the roots in reversed order (linked worktree
     first, primary second) to confirm the helper finds the primary
     regardless of array position, since `workRoots` ordering is not
     guaranteed by any invariant found in this survey.
   - Import `workspaceBaseWorkRoot` in the test file's existing
     `resourceModel.js` import block (alphabetically, near
     `resolveStickyWorkbenchSelection`/`withLastNonNullResourcesByServer`).

## Verification Plan
- `cd ws-dashboard/frontend && npm run test:resource-model` — runs
  `resourceModel.test.ts` (and its siblings) through the project's existing
  `tsc`-then-`node` test pipeline; covers the new `workspaceBaseWorkRoot`
  helper and the refactored call site in
  `resolveWorkbenchSelectionWithMatchInternal`.
- `cd ws-dashboard/frontend && tsc -b` (or `npm run build`) to confirm the
  `App.tsx` prop/type additions (`closeWorkRootId`) type-check cleanly.
- No automated render-level test is added for the actual X-button DOM
  appearance: this frontend has no React rendering/DOM test harness at all
  (no jsdom, no `@testing-library/react`, no vitest — see Codebase Findings),
  so a genuine component-render assertion is not feasible without adding new
  test infrastructure, which is out of scope for this bug fix. Verification
  of the visible fix is manual/dogfood only:
  - Open a workspace whose base root has >=1 linked worktree (has-worktrees
    shape, i.e. `compactWorkspaceWorkRoot` returns `null`).
  - Open the base root (so it appears in `openWorkRootKeys`).
  - Confirm the X renders on the base row (a) while some other row is
    selected, and (b) confirm it is present/absent consistently with the
    pre-existing (unfixed) `!selected` behavior that `"workRoot"`/
    `"compactWorkRoot"` rows already show today when the base row itself is
    selected — i.e. this fix must not make the `"workspace"` row's X behave
    differently from its sibling presentations' current `!selected` quirk.
  - Confirm clicking the X dispatches `workRoot.close` for the base root's own
    id (not `workspace.id`) — e.g. via the existing debug/devtools command log
    if present, or by confirming the base root unmounts/keeps its daemon
    session alive for reattach rather than the workspace being removed from
    the registry.
  - Confirm the "..." overflow menu's "Remove workspace..." action is still
    present and unchanged, distinct from the new X.
- Do not rely on the Playwright `test:browser` e2e suite as a gate for this
  change — it is currently blocked by unrelated pre-existing ticket `260713`.

## Escalations
- None.
