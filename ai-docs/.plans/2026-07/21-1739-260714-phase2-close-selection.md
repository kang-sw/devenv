# Plan: 260714-bug-dashboard-workroot-close-button-hidden-when-selected — Phase 2: close-selected selects next sibling else empty placeholder; restore close-button reachability

## Lead Decision (resolved 2026-07-21)

The prior "Escalate to research" disposition is **superseded**; the design is
decided and verified against ground truth (see the new Implementation Plan).

- **Post-close selection scoping = TAB SEMANTICS.** Closing the selected work
  root selects the adjacent remaining **OPEN** work root (next in tree-walk
  order, else previous). The empty placeholder appears **only when no OPEN work
  roots remain**. "Open" = present in `openWorkRootKeys` (the mounted/tab set),
  **not** "any entity in the resources tree." This resolves the open scoping
  question the prior Escalations left dangling: the closed root's own *entity*
  never leaves the tree, so "zero entities" almost never occurs — the correct,
  tab-like criterion is "zero open roots."
- **Empty-state mechanism = OPTION (b): a close-scoped explicit-empty flag.** An
  App-level boolean (`closeEmptyWorkbench`) that (i) defaults `false`, (ii) is
  set `true` only by the `workRoot.close` branch when it closes the currently-
  selected root and no open sibling remains, (iii) is cleared to `false` by any
  subsequent selection to a non-null id (`selectRoot`, hence the `resource.select`
  handler), and (iv) is consulted once at the App-level selection fold-in
  (gating `stickyWorkbenchSelection → null`), which forces `workbenchModel`
  null and skips `deriveWorkbenchView`'s `withOpenWorkRootKey` fold-in. This is
  applied to the resolver's **output**, never to `driveStickyWorkbenchSelection`'s
  input, so the shared resolver / sticky-bridge behavior is unchanged for every
  other caller.

Verification against the code (file:line in Codebase Findings / below) confirms
option (b) is byte-identical on initial load, does not regress D5/Prong-1 sticky
bridging, and needs only additive wiring. The prior null-based idea was rejected
precisely because the reconcile effect (`App.tsx:812-831`) re-selects a still-
present entity after a close, so `selectedId === null` cannot durably encode
"empty" — the explicit flag can.

## Relevant Ticket Contract

- Ticket background: `canCloseWorkRoot`'s trailing `!selected` hides the X on
  the row the user is currently viewing
  (`ai-docs/tickets/ready/260714-bug-dashboard-workroot-close-button-hidden-when-selected.md:16-21`).
  Line numbers in the ticket text (`App.tsx:9112-9115`, later corrected to
  `~9793-9798` in the Phase 1 Result note) have drifted further; current
  location confirmed at `App.tsx:9831-9836`.
- Ticket Constraints (`...md:44-67`): do not change `workRoot.close`'s
  existing "unmount workbench, keep daemon terminal session alive" semantics
  for the **non-selected** case; `resolveWorkbenchSelection`
  (`resourceModel.ts:608-613`, thin wrapper over
  `resolveWorkbenchSelectionWithMatchInternal:547-599`) has a fallback that
  returns the first root walked when `selectedId` doesn't match anything,
  rather than `null` — the ticket explicitly flags this as the obstacle to a
  real "nothing selected" empty state and says investigation of the
  resolver's other callers is needed before changing it.
- Orchestrator-relayed lead decision (supersedes the ticket's Phase 2 phase
  text's "deselect only" framing): the selected row's X must work (not just
  render); closing the selected work root selects the next sibling in order,
  else the previous, else shows the Phase 1 empty placeholder **only when no
  work roots remain**. This is the authoritative behavior for this survey.
- Phase 1 Result (`...md:117-147`) confirms `EmptyWorkbenchPlaceholder` is
  already wired to `!resources || !workbenchModel` and explicitly leaves
  Phase 2 — the close-button gate plus the selection-fallback interaction —
  as the only remaining work for this ticket.

## Out of Scope

- Phase 1's placeholder rendering/CSS (`EmptyWorkbenchPlaceholder`,
  `App.tsx:10164-10201` and its scoped styles) — already landed, not touched.
- `workRoot.close`'s existing behavior for the **non-selected** case
  (`App.tsx:1140-1178`) — unchanged per ticket Constraints.
- The separate per-server On/Off keep-alive lifecycle
  (`260714-feat-dashboard-multi-server-workbench-keepalive`) — ticket says
  this is a distinct, smaller ticket.
- Honoring `workNavOrder` (drag-reordered sibling display order,
  `workNavOrder.ts:71-94`, `applySiblingOrder`) when picking the "next
  sibling" after close. Decision: use the natural resources-tree walk order
  (the same order `flattenEntities`/`resolveWorkbenchSelectionWithMatchInternal`
  already use for their own fallback-to-first-root behavior), not the
  browser-local drag-reorder overlay. Rationale: `resourceModel.ts` currently
  has **zero imports** (confirmed via `grep '^import' resourceModel.ts` —
  empty) and is the file the ticket/task directs the new pure function into;
  `workNavOrder.ts` is a separate browser-storage-adjacent module
  (`import { browserStorage } from "./workRootFiles.js"`). Pulling
  drag-order into the pure close-selection function would require threading
  `WorkNavSiblingOrder` through the pure function's signature and the
  App-level close handler for a cosmetic ordering nicety the phase text does
  not ask for. Flagged as a possible follow-up, not required here.
- Changing `resolveWorkbenchSelectionWithMatchInternal`'s fallback behavior
  for its **other** callers (initial load via `reconcileSelectedId`, D5
  sticky-selection bridging via `resolveStickyWorkbenchSelection`) — must not
  regress; see Escalations, this is exactly the blocked part.

## Codebase Findings

- `App.tsx:9831-9836` — `canCloseWorkRoot` gate:
  ```
  const canCloseWorkRoot =
    (presentation === "workRoot" ||
      presentation === "compactWorkRoot" ||
      presentation === "workspace") &&
    isOpenWorkRoot &&
    !selected;
  ```
  Dropping `&& !selected` is the entire gate-reachability fix. No CSS/layout
  risk signal: `hasWorkspaceRemove`'s "more actions" button already renders
  unconditionally on selected rows today (`App.tsx:9943-9971`), so a selected
  row already supports a second `resource-row-action` button visually.

- `App.tsx:9945-9957` — the X's `onClick` unconditionally dispatches
  `onCommand(buildWorkRootCloseCommand(closeWorkRootId, actionServerId))`
  regardless of `selected`. No payload carries a "was selected" flag
  (`commands.ts:359-366`: payload is `{type:"workRoot.close", serverRoute,
  workRootId}`). Any "was this the active selection" check must happen in the
  command handler using its own closed-over state, not the payload.

- `App.tsx:1140-1178` — the `workRoot.close` command handler. Confirmed: it
  only removes `rootKey` from `openWorkRootKeys`/`openWorkRootRefs`/
  `workbenchGroupsByRoot`/`paneOrderByRoot`. It **never touches `selectedId`**
  today. This is why the X is gated off today for the selected row at all —
  closing the selected root without moving `selectedId` away from it would be
  immediately neutralized (see next finding), not just visually odd.

- `workbench/openRootLookup.ts:260-295` (`deriveWorkbenchView`) — `openInstanceKeys`
  is `withOpenWorkRootKey(state.openWorkRootKeys, selectedRootKey)`
  (line 286-288): the currently-resolved **selected** root's key is
  unconditionally folded into the mounted-instance set every render,
  independent of `openWorkRootKeys`. Consequence: closing the selected root
  has **no visible effect** unless the same action also moves `selectedId`
  away from it in the same commit — confirming the close handler must own
  the selection update, not just the unmount side effects.

- `App.tsx:685-690` (`selectRoot`) — the single atomic
  `selectedServerIdRef.current` / `setSelectedServerId` / `setSelectedId`
  commit helper; accepts `entityId: string | null`. This is the correct call
  for applying the post-close selection (mirrors `App.tsx:1273`'s existing
  use of `selectRoot` inside `server.off`'s handler for an analogous
  same-handler reselect).

- `App.tsx:1372-1379` — `executeCommand`'s `useCallback` deps:
  `[activeResources, loadResources, loadServers, openWorkRootRefs,
  readOnlyFilePanes, selectRoot]`. `activeResources` and `selectRoot` are
  already present; `selectedId` is **not** currently a dep (not currently
  read inside the callback body) and must be added once the `workRoot.close`
  branch reads it. `selectedServerIdRef` (`App.tsx:669-671`, kept in sync by
  both the ref-sync effect and `selectRoot` itself) already gives ref-based
  server-match without a new dep, mirroring `server.off`'s existing
  `serverId === selectedServerIdRef.current` check at `App.tsx:1262`.

- `App.tsx:9632-9756` (`WorkspaceRows`) — non-obvious base-root/child-worktree
  semantics: a non-compacted workspace's own row selects `workspace.id`
  (`selected={selectedWorkspace}` computed from `selectedId === workspace.id`
  **or** a non-child root of this workspace matching `selectedId`), while its
  X closes `closeWorkRootId = baseRoot?.id` (`App.tsx:9705`,
  `workspaceBaseWorkRoot` in `resourceModel.ts:530-538`). So `selectedId` at
  close time can legitimately be the **workspace id**, not the closing
  root's own id, even though the row renders as selected. A naive
  `workRootId === selectedId` check in the close handler would silently miss
  this case. Correct check: compare the **resolved** selection's root id
  (`resolveWorkbenchSelection(activeResources, selectedId)?.root.id`) against
  the closing `workRootId` — this already collapses the workspace-id
  indirection via `resolveWorkbenchSelectionWithMatchInternal`'s existing
  `selectedId === workspace.id` branch (`resourceModel.ts:559-570`), with no
  new resolver logic needed for this part.

- `resourceModel.ts:425-472` (`flattenEntities`) — produces one `"workRoot"`
  entity per `workspace.workRoots[]` entry, walked in natural
  workspace-then-workRoot order (server-supplied, unreordered). This is the
  **same** order `resolveWorkbenchSelectionWithMatchInternal`'s
  `fallback ??= rootSelection` (line 580) already treats as canonical "first
  root" — reusing it for "next/previous sibling" keeps one consistent
  ordering convention in the file rather than introducing a second one.

- `resourceModel.ts` has zero existing imports (self-contained pure module,
  confirmed via `grep '^import' resourceModel.ts`, no output) and is already
  covered by `npm run test:resource-model`
  (`ws-dashboard/frontend/package.json:12`, compiles+runs
  `resourceModel.test.ts`). This is the correct home for the new pure
  "pick next selection after close" function per the task's own instruction.

- `resourceModel.test.ts:87-122` — existing fixture helpers to reuse:
  `readyState`, `workRoot(id, workspaceId, label, mainInstances?)`. Existing
  `multiRootWorkspace` fixture (`resourceModel.test.ts:292-307`, two roots
  `root-multi-a`/`root-multi-b` in one workspace) is directly reusable for
  the within-workspace next/previous cases; a cross-workspace case needs one
  new small fixture (two workspaces, one root each) following the same
  `workRoot()`/`readyState` pattern already used throughout the file.

- **Confirmed blocker (see Escalations)**: `resourceModel.ts:547-599`
  (`resolveWorkbenchSelectionWithMatchInternal`)'s `fallback ??= rootSelection`
  is **unconditional** — it fires for every root walked regardless of
  `selectedId`'s value, including `null`. Since `workRoot.close` never
  removes the closed root's *entity* from the resources tree (it only
  unmounts the panel — `App.tsx:1151-1177` only touches
  `openWorkRootKeys`/`openWorkRootRefs`/`workbenchGroupsByRoot`/
  `paneOrderByRoot`, never `resourcesByServer`), the closed root's own entity
  is still walked and still becomes `fallback` whenever it was the *only*
  work root in the tree. Setting `selectedId = null` in that exact case does
  **not** reach a true empty state — the resolver silently re-resolves back
  to the very root just closed (`matched: false`, but `selection` non-null).
  This exactly matches the mechanism the ticket's own Constraints section
  (`...md:49-64`) already anticipated needing "an explicit no-selection
  sentinel... or an equivalent mechanism," and traced further:
  `resourceModel.ts:289-346` (`resolveStickyWorkbenchSelection`) consumes the
  resolver's `matched` flag internally for D5/Prong-1 sticky-bridging but
  does **not** expose it to its caller;
  `workbench/openRootLookup.ts:191-197` (`driveStickyWorkbenchSelection`)
  and `workbench/openRootLookup.ts:260-295` (`deriveWorkbenchView`) likewise
  only see the resolved `selection`, never `matched`; `App.tsx:4559-4581`
  (`workbenchModel`) derives purely from `resources && selection`. There is
  currently no way anywhere in this chain to distinguish "never selected yet
  / transient miss" (must keep the existing fallback-to-first-root behavior
  for initial load and D5 bridging) from "user explicitly closed their last
  open root" (must reach `workbenchModel === null`) — both currently collapse
  to the identical `selectedId === null`, unmatched-selection state.

## Implementation Plan

Ordered, minimal, additive. Two files: `resourceModel.ts` (one new pure
function) and `App.tsx` (flag + gate + handler wiring + the gate-reachability
edit). No change to `resolveWorkbenchSelectionWithMatchInternal`,
`resolveStickyWorkbenchSelection`, `driveStickyWorkbenchSelection`, or
`deriveWorkbenchView` bodies — the empty state is imposed entirely by nulling
the resolver's already-driven **output** before it is consumed.

1. **Gate-reachability fix (the actual bug).** `App.tsx:9831-9836` — drop the
   trailing `&& !selected` from `canCloseWorkRoot`. Single-token edit; the X
   already coexists with the "more actions" button on selected rows
   (`App.tsx:9943-9971`), so no layout risk. The X's `onClick`
   (`App.tsx:9945-9957`) already dispatches `buildWorkRootCloseCommand`
   unconditionally — no change there.

2. **New pure function** in `resourceModel.ts`, added right after
   `reconcileSelectedId` (`resourceModel.ts:490-503`) so it sits with the other
   selection-derivation helpers and above the `WorkbenchSelection` type. It uses
   the in-module `serverScopedIdentity` (already defined in this file; no new
   import — the module stays import-free) to map open **keys** → entity ids
   internally, so the caller passes only the raw open-key set:

   ```ts
   // Post-close selection scoped to currently-OPEN roots (tab semantics).
   // Walks the resource tree in natural order, keeping only roots whose
   // serverScopedIdentity key is in `openRootKeys`; returns the next still-open
   // root after `closingRootId`, else the previous, else null when no open root
   // remains. Pure: the open set is passed in because this module has no access
   // to React `openWorkRootKeys` state.
   export function pickWorkRootSelectionAfterClose(
     resources: DashboardResourcesView | null,
     closingRootId: string,
     openRootKeys: ReadonlySet<string>,
   ): string | null {
     if (!resources) return null;
     const orderedOpenRootIds: string[] = [];
     for (const workspace of resources.workspaces) {
       for (const root of workspace.workRoots) {
         const key = serverScopedIdentity(root.resourcePath.serverId, root.id);
         if (openRootKeys.has(key)) orderedOpenRootIds.push(root.id);
       }
     }
     const idx = orderedOpenRootIds.indexOf(closingRootId);
     if (idx === -1) {
       // Defensive/race: closing root not in the open set — first remaining.
       return orderedOpenRootIds.find((id) => id !== closingRootId) ?? null;
     }
     return orderedOpenRootIds[idx + 1] ?? orderedOpenRootIds[idx - 1] ?? null;
   }
   ```

   (`orderedOpenRootIds` still contains `closingRootId` at `idx`, so `idx+1` /
   `idx-1` are the adjacent *remaining* open roots; the only-open-root case
   yields `undefined ?? undefined ?? null`.)

3. **Import additions.** `App.tsx:260-291` resourceModel import block — add
   `pickWorkRootSelectionAfterClose` and `resolveWorkbenchSelection` (the latter
   is exported at `resourceModel.ts:608-613` but not yet imported here;
   `serverScopedIdentity`/`flattenEntities`/`workspaceBaseWorkRoot` already are).

4. **Flag declaration.** `App.tsx` immediately after the `openWorkRootRefs`
   state (`App.tsx:532-534`):
   `const [closeEmptyWorkbench, setCloseEmptyWorkbench] = useState(false);`

5. **Consume/gate site.** `App.tsx` right after
   `stickyWorkbenchSelectionRef.current = nextDriverState;` (`App.tsx:634`),
   define the single gated selection value:
   `const workbenchSelection = closeEmptyWorkbench ? null : stickyWorkbenchSelection;`
   Then:
   - `App.tsx:658` — change `deriveWorkbenchView`'s `selection: stickyWorkbenchSelection`
     to `selection: workbenchSelection`. (Gated null ⇒ `selectedRootKey` null ⇒
     `openInstanceKeys = [...openWorkRootKeys]`, i.e. the `withOpenWorkRootKey`
     fold-in is skipped, per option (b)(iv).)
   - `App.tsx:904` — delete the now-redundant
     `const workbenchSelection = stickyWorkbenchSelection;` (its comment can be
     folded into the gate site); the downstream seed effect (`App.tsx:905-918`)
     and the `<WorkbenchShell selection={workbenchSelection}>` prop
     (`App.tsx:1552`) then see the gated value, so `workbenchModel`
     (`App.tsx:4559-4581`, `resources && selection ? … : null`) collapses to
     null ⇒ `EmptyWorkbenchPlaceholder`.
   Gating the OUTPUT (not the `driveStickyWorkbenchSelection` input at
   `App.tsx:629-634`) leaves the sticky driver's read-and-advance state machine
   untouched — the driver still runs on the real `{activeResources, selectedId,
   selectedServerId}` render key and commits `nextDriverState` normally.

6. **Clear site.** `App.tsx:685-690` `selectRoot` — add, after the three commit
   statements:
   `if (entityId !== null) setCloseEmptyWorkbench(false);`
   The `useCallback` dep array stays `[]` (`setCloseEmptyWorkbench` is a stable
   setter). The `entityId !== null` guard is required so the no-sibling close's
   own `selectRoot(server, null)` does not immediately re-clear the flag it is
   about to set. The reconcile effect (`App.tsx:812-831`) uses `setSelectedId`
   directly, **not** `selectRoot`, so its automatic re-selection of a still-
   present entity deliberately does **not** clear the flag — exactly what keeps
   the empty state stable until a real user selection.

7. **Close-handler wiring.** `App.tsx:1140-1178` `workRoot.close` branch — after
   the existing four unmount `setState` calls, append (all reads use closure
   values captured pre-commit, so `openWorkRootRefs` still contains the closing
   key):
   ```ts
   const selectedRootId =
     resolveWorkbenchSelection(activeResources, selectedId)?.root.id ?? null;
   if (selectedRootId === workRootId) {
     const nextId = pickWorkRootSelectionAfterClose(
       activeResources,
       workRootId,
       new Set(Object.keys(openWorkRootRefs)),
     );
     if (nextId) {
       selectRoot(selectedServerIdRef.current, nextId);
     } else {
       selectRoot(selectedServerIdRef.current, null);
       setCloseEmptyWorkbench(true);
     }
   }
   ```
   - `Object.keys(openWorkRootRefs)` is the open-key set: `openWorkRootRefs` is
     seeded/removed in lockstep with `openWorkRootKeys`
     (`App.tsx:915-918`, and this same close branch), so its keys mirror
     `openWorkRootKeys`; it is already a dep of `executeCommand`, so no new
     dep is needed for the open set. (`openWorkRootKeysSet`, `App.tsx:938-940`,
     is the equivalent memo if the array form is preferred, but would add a dep.)
   - `resolveWorkbenchSelection(activeResources, selectedId)?.root.id` collapses
     the workspace-id-vs-root-id indirection (a `workspace`-presentation row's X
     closes `workspaceBaseWorkRoot`, `App.tsx:9705`,
     `resourceModel.ts:530-538`), matching the Codebase-Findings analysis; a
     naive `workRootId === selectedId` would miss it.
   - Both `setState`/`selectRoot`/`setCloseEmptyWorkbench` calls run inside the
     same synchronous command handler ⇒ one React 18 batched commit, mirroring
     `server.off`'s in-handler unmount-then-`selectRoot` (`App.tsx:1198-1273`).

8. **Dep-array addition.** `App.tsx:1372-1379` `executeCommand` deps — add
   `selectedId` (now read via `resolveWorkbenchSelection(activeResources,
   selectedId)`). `activeResources`, `openWorkRootRefs`, and `selectRoot` are
   already present; `setCloseEmptyWorkbench` and the module import
   `resolveWorkbenchSelection`/`pickWorkRootSelectionAfterClose` need no dep.

## Verification Plan

- Commands: `npm run build` (frontend); `npm run test:resource-model` extended
  with cases for `pickWorkRootSelectionAfterClose`. Because the function is
  scoped to OPEN roots, **every case passes an explicit open-key set**
  (`new Set([...])` of `serverScopedIdentity(serverId, rootId)` keys), not just
  a tree — the open set is the discriminating input. Reuse
  `readyState`/`workRoot(...)` (`resourceModel.test.ts:87-122`) and
  `multiRootWorkspace` (`resourceModel.test.ts:292-307`); add one small
  two-workspace fixture for the cross-workspace case.
  - (1) multi-root workspace, ALL roots open, close a middle root ⇒ next-sibling
    id (open-key set = every root's key).
  - (2) multi-root workspace, all open, close the last root ⇒ previous-sibling id.
  - (3) two-workspace fixture, all roots open, close a workspace's last root ⇒
    first root of the next workspace (cross-workspace natural-order boundary).
  - (4) **OPEN-scoping discriminator:** tree has siblings but only the closing
    root is in the open-key set ⇒ `null` (a sibling *exists* in the tree but no
    *open* sibling remains — this is the tab-semantics case that produces the
    empty placeholder, and the reason the open set is a parameter).
  - (5) two open roots that are non-adjacent in the tree (intervening roots NOT
    in the open set), close one ⇒ the other open id (order derives from the tree
    walk, membership from the open set).
  - (6) closing root absent from the open-key set (defensive/race): other open
    roots present ⇒ first open id; none present ⇒ `null`.
  - (7) `null` resources ⇒ `null`; empty open-key set ⇒ `null`.
- Render-level check called for by the ticket phase text (`...md:111-115`), now
  writable because the mechanism is decided: X present/clickable on a **selected
  open** row; clicking it with a remaining open sibling lands the selection on
  that sibling (not the placeholder); clicking it when it is the **last open**
  root lands on `EmptyWorkbenchPlaceholder` (via `closeEmptyWorkbench`) and does
  **not** fall back to another root; and a subsequent explicit selection clears
  the flag and restores a normal workbench.

## Escalations

- Confidence: **high (resolved 2026-07-21)** — the escalation below is retained
  as the record of *why* the decision was needed; it is now answered by the Lead
  Decision + Implementation Plan above and no longer blocks execution. Ground-
  truth verification of option (b):
  - **Initial-load default selection is byte-identical.** `reconcileSelectedId`
    (`resourceModel.ts:490-503`) and the reconcile effect (`App.tsx:812-831`,
    `setSelectedId` at 823/829) are untouched; `closeEmptyWorkbench` defaults
    `false` (`App.tsx` new state after 534) and is set only in the
    `workRoot.close` branch (`App.tsx:1140-1178`), never on the mount →
    `loadResources` → reconcile path. The gate
    (`closeEmptyWorkbench ? null : stickyWorkbenchSelection`, `App.tsx:634`) is
    a no-op while false.
  - **D5 / Prong-1 sticky bridging is unaffected.**
    `resolveStickyWorkbenchSelection` (`resourceModel.ts:289-346`) bridges only
    when `selectedId` is non-null (guard at `resourceModel.ts:321`,
    `cached && selectedId && …`); `driveStickyWorkbenchSelection`
    (`workbench/openRootLookup.ts:191-223`) advances its state machine from the
    real render key. The flag gates the resolver's **output** (after
    `App.tsx:634`), never the driver's input, so no bridge transition is
    altered. No path outside the close branch sets the flag, and the flag is
    only true when the user just closed their last open root (selection then
    null or auto-reconciled) — a state in which no legitimate bridge fires,
    because a bridge requires a non-null user `selectedId` whose selection
    clears the flag via `selectRoot`. No D5 case would wrongly show empty. No
    residual blocker.
  - Minor residual (low risk, accepted): the selected-root check uses
    `resolveWorkbenchSelection(activeResources, selectedId)?.root.id`, which can
    return the fallback (first) root during a transient tree miss; closing that
    root would then reselect. This matches the resolver's existing
    fallback-to-first semantics used everywhere and is normally masked by the
    sticky bridge — not worth extra logic.
- Reason (historical): the gate-reachability fix and the "a sibling exists" selection
  path are fully traced and low-risk (single boolean edit + one new pure
  function + a handler wire-up, all evidenced above). But the phase's
  defining success criterion — "empty placeholder ONLY when no work roots
  remain," verified against "not silently falling back to another root" —
  is blocked by a confirmed, multi-file entanglement: `selectedId === null`
  is currently ambiguous between "never selected" (must keep today's
  fallback-to-first-root for initial load / D5 sticky bridging) and
  "explicitly closed the last open root" (must reach true `null`
  `workbenchModel`), and no existing field distinguishes them across
  `resolveWorkbenchSelectionWithMatchInternal` →
  `resolveStickyWorkbenchSelection` → `driveStickyWorkbenchSelection` →
  `deriveWorkbenchView`/`workbenchModel`. The ticket's own Constraints
  section already flagged this exact ambiguity and asked for investigation
  of the resolver's callers before changing it — this is a strategy/design
  decision, not a wiring gap a light survey plan should guess at, per the
  orchestrator's own caution against regressing "the normal (non-close)
  selection-resolution path."
- Research should decide: the mechanism to distinguish an explicit
  "no selection / all siblings closed" state from the ordinary
  never-selected-yet fallback, safely threaded through
  `resolveWorkbenchSelectionWithMatchInternal`/`resolveStickyWorkbenchSelection`/
  `driveStickyWorkbenchSelection`/`deriveWorkbenchView`/`workbenchModel`
  without regressing initial-load default selection or D5/Prong-1 sticky
  bridging. Concretely: (a) expose the resolver's existing `matched` flag
  further up this chain and gate `workbenchModel`/`deriveWorkbenchView`'s
  fold-in on it instead of on `selection` alone, vs. (b) a separate
  App-level "explicitly deselected" boolean (set by the close handler when
  no sibling exists, cleared by any subsequent `resource.select`/`selectRoot`
  call) consulted before computing `workbenchModel`/`deriveWorkbenchView`'s
  input selection, vs. (c) some other approach — and confirm whether "no work
  roots remain" should scope to "zero work-root entities anywhere in the
  tree" or "zero *open* work roots for the current server" (these differ:
  the closed root's own entity never leaves the tree, so the former can only
  occur in already-degenerate trees; the "standard tab-like" framing in the
  lead's decision reads more like the latter).
