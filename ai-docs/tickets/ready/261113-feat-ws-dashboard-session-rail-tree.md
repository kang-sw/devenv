---
title: "Dashboard session rail: reliable reorder, hover rename/copy, depth-1 session tree, context menu"
related:
  261113-feat-ws-dashboard-iframe-shell: "ready, in flight; the rail moves into the shell document - whichever ticket lands second rebases"
  261113-feat-ws-dashboard-board-reorder-hold: "ready; separate board topic DnD, no shared code; its scripts/board-headless.mjs is the headless-harness pattern this ticket reuses"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d027b49160890a53
sage-review-completeness-reviewed: d027b49160890a53
---

# Dashboard session rail: reliable reorder, hover rename/copy, depth-1 session tree, context menu

## Background

Owner request (MAIN's mail, 2026-11-13 14:29Z) with a rail screenshot; UX spec
read against develop 14aabe4b5. The spec is archived verbatim at
`ai-docs/tickets/assets/261113-feat-ws-dashboard-session-rail-tree/ux-spec.md`
(refreshed 2026-11-14 with its decision 13, owner answer (554)=a). The
branch-indent mockup that decision refers to is archived beside it at
`ai-docs/tickets/assets/261113-feat-ws-dashboard-session-rail-tree/session-rail-branch-indent-mockup.html`.
Owner answers (relayed by MAIN, 2026-11-13 and 2026-11-14) are folded into the
Decisions below.

- The rail region is server-rendered (`rail.go`, `pageview_rail.go`,
  `RailRow`) and replaced whole on every rail swap. Every row's second line
  (`Message`, the session's last message) changes, so swaps are frequent.
- Client order is local. `WsRail.pin` (rail.js:164) orders starred rows
  first, then the drag order (`sessionOrder`, localStorage `railorder`,
  rail.js:15 and the `orderList` helper at rail.js:113-136); `arrange` orders
  workspaces (`wsOrder`, localStorage `wsorder`). `drop()` (rail.js:202)
  refuses: another list, the other side of the stars, ended rows, and the
  item itself.
- Drag lives in dashboard.js (`rdPeers`/`rdStart`/`rdBegin`/`rdMove`/`rdEnd`,
  dashboard.js:5721-5869). The whole row (a link) is the press target. The
  touch path is a 380 ms hold (`RD_HOLD`).
- Rename exists only for the viewed session's head (`labelStart`,
  dashboard.js:2518; POST `/s/<rid>/label`, `handleSessionLabel` in
  `label_route.go:110`); the per-rid route is already there. The address
  copy exists only on the head name (`data-copy` from
  `SessionView.MailboxOpen`/`MailboxRef`, pageview.go:111-138).
- The hosted session menu (`rpSessMake`, dashboard.js:6742-6837) is bound to
  the viewed session: its `rid` and `lead.st`.

### Likely reorder failures (UX read; reconcile with MAIN's session-rail log)

1. **A swap during the press kills the drag.** `swap()` (dashboard.js:496)
   holds a rail swap only while `rdrag.on`. A press still inside `RD_SLOP`,
   or a touch still in its hold, keeps a node that the swap replaces.
   `rdBegin` then sees `!d.item.isConnected` (dashboard.js:5801) and drops the
   drag silently. With per-message rail updates this is frequent.
2. **Silent refusals.** Peers are only the same parent node, the same star
   state, and not ended. A worktree group with one live session has
   `rdPeers < 2`, so no drag starts. Dropping across worktree groups or across
   the star boundary is refused with no feedback.
3. **The row is a link.** The press competes with link navigation, text
   selection and the native drag (cancelled). Small slop under a trackpad
   triggers navigation instead. `.rs` has no `user-select` or `touch-action`
   rule (MAIN). (Corrected: `.rail .rs` already carries `user-select: none` and `-webkit-touch-callout: none`, dashboard.css:1607-1608; only `touch-action` is missing from `.rs`.)
4. **Drag feedback and reach (MAIN).**
   - The drop line is hidden whenever its y falls outside the list bounds
     (`rdOver`, dashboard.js:5776). Near the edges the drop target exists but
     is invisible.
   - Edge auto-scroll only engages within 28 px of the list edge
     (`RD_EDGE`, dashboard.js:5732).
   - `WsRail.pin` re-sorts the stars on every swap (rail.js:164). A swap that
     lands right after a drop can re-order what was just dropped.

Phase 1 addresses all four: (1) Decision 2; (2) the refusal hint (Decision 2);
(3) the grip, with `user-select: none` and `touch-action: none` on it
(Decision 1); (4) Decision 15: the drop line is clamped to the list edge
instead of hidden, the edge zone grows to 48 px, and the order is applied once
after the drop is saved, so `pin` re-runs on the new order (in Phase 2, the
server order).

## Decisions

1. **Drag handle, separate from the link (537 ok, owner 2026-11-13).**
   - Each live row gets a grip in a reserved left gutter (14 px; the row
     already sits indented under the worktree guide line). The grip is a
     separate element from the row's `<a>`. Per Decision 14 ((554)=a) that
     gutter is the column the star held: the star moves after the name, and
     the grip takes the 14 px between the guide line and the harness icon.
   - Mouse: the grip shows on row hover or focus-within. The press starts the
     drag at once past `RD_SLOP`. A press on the rest of the row is the link,
     never a drag.
   - `user-select: none` applies on the grip and on the rail rows during a
     drag, so the press never selects text.
   - Coarse pointers (`@media (pointer: coarse)`): the grip is always visible
     and starts the drag on press (`touch-action: none` on the grip), with no
     hold.
   - Workspace bars keep their bar-as-handle behaviour.
   - Supersedes 939a2c59 ("Touch drags after a 380ms still hold (a swipe
     keeps scrolling the list) ..."): the touch drag starts on the grip with
     no hold, under 537 ok; a swipe on the rest of the row still scrolls. The
     row long-press becomes the context menu in Phase 4 (Decision 11).
2. **Swaps never kill a press.**
   - A rail swap is held from `pointerdown` on a grip (not only once the drag
     is on) until `rdEnd`. There is no cancel on a swap before the slop
     threshold.
   - If anything replaces the node anyway, the drag re-finds its item by
     `data-rid` / `data-ws` after the swap instead of aborting.
   - A refused drop shows a "not allowed" cursor and a short inline hint
     ("다른 워크스페이스로는 옮길 수 없음", etc.), not a silent no-op.
3. **Layout lives server-side (534=c, owner 2026-11-13).** Tree AND order are
   stored server-side like `label.json`, replacing localStorage
   `railorder`/`wsorder`.
   - A new daemon store `rail-layout.json` with `schema_version: 1`, holding
     per workspace path: `order` (the top-level reply-ids) and `children`
     (parent reply-id -> ordered child reply-ids).
   - Every change re-reads the file first (the `workspaceStore` pattern,
     `workspaces.go:53`), so two browsers or processes do not lose each
     other's edits.
   - The server orders the rail rows from it, and every browser sees the same
     tree through the normal rail swap. `sessionOrder` in localStorage is
     retired.
   - Migration: the first page load that finds an empty server layout and a
     local order posts it once (the `import` op, Decision 4), then clears the
     local key.
   - Compatibility tier: T0. It is a new store file with `schema_version` and
     a new route; no existing store, tool, or event changes.
   - Edge rules (design review 2026-11-14, autonomous):
     - A browser that finds a non-empty server layout clears (or ignores)
       its own `railorder`, `stars`, and `wsorder` keys; it never imports
       over a server layout, and its local keys never re-order rows again.
     - Entries for reply-ids the index does not know are filtered on read.
       A write prunes them only against a loaded index (the registry has
       been read at least once); a write before that keeps every stored
       entry, so a daemon start never wipes the layout.
     - A layout write wakes the rail streams and changes the rail's render
       stamp, as label_route's `d.index.Reread` does for a label
       (label_route.go:149). The workspace routes already wake the rail with
       `d.broker.wake(railListRepoKey)` (workspace_routes.go:224,
       railworktrees.go:28); the rail stamp (`railPageSource`, rail.go:89,
       today `pages.indexStamp()`) must also cover the layout, so a woken
       render is not skipped as unchanged.
   - Pre-paint state with the iframe shell (design review 2026-11-14,
     autonomous): `261113-feat-ws-dashboard-iframe-shell` (501) leaves the
     shell free to apply localStorage collapse/pin/order before first paint.
     Phase 2 removes that pre-paint application for order and stars (they
     come server-rendered) and narrows it to collapse only (the workspace
     `railcol` set and, from Phase 3, the parent collapse set). If the shell
     lands after Phase 2, it rebases onto the narrowed set.
   - Supersedes a7b53d6b ("(408)=a order in localStorage beside stars
     (ws.dashboard.railorder / wsorder)"): order moves server-side under
     534=c; its (409)=b and (410) stand.
   - Supersedes 939a2c59 ("One flat JSON array per kind
     (ws.dashboard.railorder by reply-id, ws.dashboard.wsorder by path)"):
     the server store keeps per-workspace `order`/`children`/`stars` and a
     top-level workspace order (534=c, 538); the flat local arrays are read
     once for the import and then retired.
4. **One route with validated ops.**
   - `POST /rail/layout {op, rid?, parent?, before?, after?, path?, order?,
     stars?, wsorder?}`. `op` is one of:
     - `move` (rid, before/after): reorder at top level or among siblings.
       Valid when rid is live, before/after names one row at the same level
       (the same worktree group at top level, or the same parent's
       children), and both sit in the same star section.
     - `attach` (rid, parent, before/after): become a child of `parent` at a
       position. Valid when parent is live and top-level, rid has no
       children, rid is not parent, both are in the same workspace, and
       before/after (when given) names one of parent's children.
     - `detach` (rid, before/after): become top-level. Valid when rid is a
       stored child. It lands next to the old parent when both are in the
       same worktree group, else at the head of rid's own worktree group;
       before/after, when given, must name a top-level row of rid's own
       worktree group.
     - `star` / `unstar` (rid): add or remove rid in its workspace's starred
       set. Valid when the index knows rid; `star` is refused on a stored
       child ((540)=a); both are idempotent.
     - `wsmove` (path, before/after): reorder workspaces. Valid when path and
       before/after are distinct workspaces the rail lists.
     - `import` (order, stars, wsorder): the browser's local `railorder`,
       `stars`, and `wsorder` arrays. Applied only when the server layout is
       empty: the file is missing or holds no entry at all (no order,
       children, star, or workspace-order entry in any workspace). The
       server maps each rid to its workspace through the index, drops
       unknown or duplicate entries, and imports no children (local state
       has no tree). It needs a loaded index, and answers a retryable
       refusal before that (the browser keeps its keys and retries on a
       later load). On a non-empty layout it answers "not applied" and
       changes nothing; the browser clears its keys either way.
   - The server validates (every op):
     - depth 1: a parent is never a child, and a session with children cannot
       become a child;
     - the same workspace (536=a, owner 2026-11-13: same workspace only);
     - no self-attach, no ended target parent.
   - Orphans (design review 2026-11-14, autonomous): an orphan is a live
     child whose parent has ended while the stored link is kept (Decision 7).
     Any op on an orphan, and any `attach` under it, first drops the
     orphan's dormant link to its ended parent, then applies as for a
     top-level row. The node-tested zone function (Decision 9) and the
     server validator use this same rule.
   - Ops, not whole-array writes, so concurrent edits merge. A refusal returns
     a reason string that the client toasts. The route takes the same gate as
     the other rail write routes (local viewer + same origin).
5. **Tree rendering.**
   - A child renders under its parent, indented one step (16 px; 12 px in the
     overlay rail), with a guide line.
   - A top-level row always renders in its own worktree group (design review
     2026-11-14, autonomous). Only a child can render outside its own
     worktree group: under its parent.
   - A child row is one line: children show without the `.s2` output line
     (the `Message` line), as the owner asked. The full message stays
     reachable as the row's `title` tooltip.
   - Row-one signals stay on child rows: the approval badge, working, failure,
     the subagent count, and the time.
   - A child whose worktree differs from its parent's shows a small branch
     tag on its line, because 536=a allows any worktree of the same workspace.
6. **Collapse/expand.**
   - A parent with children shows a chevron and a child count. Default:
     expanded. The collapsed state is a view preference, kept per browser like
     the workspace `collapsed` set.
   - A collapsed parent rolls up its children's attention onto its own row:
     any child approval wait shows as the parent's "?" badge (with the child
     count in the tooltip), and any working child shows the working mark. A
     collapse must never hide an approval request.
   - Because collapse is per browser, the server always renders a parent's
     roll-up marks (the child approval count and the working mark), and CSS
     shows them only when the client marks the parent collapsed (design
     review 2026-11-14, autonomous).
   - Keyboard: Left/Right on a focused parent row collapses/expands it.
7. **Ended or forgotten parent; ended child.**
   - (539)=a, owner 2026-11-13: when a parent ends with live children, the
     children go top-level, in the parent's slot. The stored link is kept, and
     they re-nest when the parent is revived. Rejected: (b) the ended parent
     stays in the live list as a dimmed group head while it has live
     children.
   - Placement (design review 2026-11-14, autonomous): such a child (an
     orphan, Decision 4) goes top-level in its OWN worktree group. It takes
     the parent's slot only when it shares the parent's worktree; otherwise
     it shows at the head of its own group's live rows. It counts as
     top-level for stars and drops while it is an orphan; any op on it drops
     the dormant link first (Decision 4).
   - Forgetting the parent drops the link for good.
   - A child that ends goes to the ended fold like any row, keeping its link,
     and re-nests on revive.
8. **Stars vs tree.**
   - Stars order top-level units: a parent moves with its children as one
     unit, so starred units come first, then the layout order.
   - (540)=a, owner 2026-11-13: a child cannot be starred while it is a
     child. Its star button is hidden; a star it had stays stored but inert,
     and is active again on detach. Rejected: (b) starring allowed, with
     starred children first among their siblings.
   - Dropping into the middle of a parent (attach) is allowed across the star
     boundary. Reorder stays within its star section, as today.
   - (538) ok, owner 2026-11-13: stars and the workspace order also live in
     the server layout. The layout store holds per workspace the starred rids
     and, at its top level, the workspace order. The localStorage `stars` and
     `wsorder` keys are imported once with the session order, then retired.
     Rejected: keeping them in localStorage (a tree shared across browsers
     with local stars would order units differently per browser).
   - Supersedes 939a2c59 ("drop() is the refusal point (other group, other
     kind, across the stars, ended rows, itself)") for attach only: an attach
     crosses the star boundary (above). Reorder across the stars stays
     refused, now with the hint (Decision 2); the other refusals stand.
9. **Drop affordance: three zones per target row.**
   - Drop on row middle = child, edges = reorder: the top 25% of the row means
     before, the bottom 25% after, and the middle 50% child (attach). The
     middle zone exists only on a valid parent: top-level, live, the same
     workspace, not the dragged item itself, and the dragged item has no
     children (a session with children cannot become a child). Elsewhere the
     row splits 50/50 into before/after.
   - Before/after: the existing `.rdrop` line, starting at the indent of the
     level the drop lands on, so a line between two children reads as "among
     the children" and a line at the top-level indent reads as "top level".
   - A drop line among P's children attaches to P at that position. A drop
     line at the top level detaches a dragged child. That detach line is
     valid only in the child's own worktree group (Decision 5: a top-level
     row renders there); a top-level line in another group is refused with
     the hint. A detach with no line (Alt+←, the menu) lands as Decision 4
     states.
   - An orphan (Decision 4) is a valid parent and a valid reorder target as
     a top-level row; attaching under it drops its dormant link first.
   - Child zone: the target row gets an accent outline and tint, plus a
     trailing "↳" glyph at the row's right edge. There is no line.
   - Dragging a parent drags its whole unit (children follow). Its children
     are not drop targets.
   - The pure zone/validity calculation goes in rail.js (like `drop()`), so it
     is node-testable.
10. **Hover icons: pencil and clipboard.**
    - On row hover or focus-within, the time label on the row's right edge
      hides, and a pencil and a clipboard take that slot. The name never
      reflows, because the icons do not take width from it. The approval,
      count and failure badges stay.
    - Hover pencil to rename: the name becomes a one-line input. It reuses the
      head's `labelStart`/`labelKey`/`labelSave` logic (dashboard.js:2518-2552),
      generalized to a rid and posting `/s/<rid>/label` (reusing
      `handleSessionLabel`): Enter saves, Escape or blur cancels, and an empty
      value clears the label. A rail swap while the input is open waits, like
      `headHeld`.
    - Hover clipboard to copy the mailbox address: copies the row's
      `data-copy`. The server builds that string per rail row with
      `MailboxRef` (the same builder as the head, pageview.go:111-138) via a
      new `RailRow` field. The row needs the project path (`RailRow` has none
      today; the builder's `dir=` attribute needs it).
    - The builder's inputs come from the rail view, never a per-row git read
      (design review 2026-11-14, autonomous): `dir` is the registration's
      project; `root` is the row's `RailWorktree.Path` (the worktree's top
      level, as the head's `Git.TopLevel`), emitted only when it differs
      from the project; `branch` is `RailWorktree.Branch` unless `Detached`;
      `harness` and the label come from the row. A row whose worktree is a
      fallback entry (`RailWorkspace.Path` or the project itself, when no
      listed worktree contains it; pageview_rail.go:394-414) and a
      plain-folder workspace row take the same inputs from that entry. The
      head's builder is factored so both feed the same function.
    - Coarse pointers: there is no hover, so rename and copy live in the
      context menu (decision 11).
    - Accepted interim (design review 2026-11-14, autonomous): from Phase 1
      until Phase 4, coarse-pointer users have no rename or copy on rail
      rows (the context menu lands in Phase 4), and a row long-press does
      nothing. The viewed session's head rename and copy stay as today.
11. **Context menu on all rows (535=a, owner 2026-11-13).**
    - It opens on `contextmenu` (right click), on long-press of a row on
      coarse pointers (the hold now means "menu", since drag moved to the
      grip), and on Shift+F10 or the ContextMenu key on a focused row.
    - Right-click menu on ALL rows. Non-hosted rows get: rename, copy address,
      move under ▸ (the eligible parents in the workspace), detach from parent
      (children only), and reopen if ended (rows with a revive).
    - Hosted rows get the same entries plus the existing session menu content
      (model, permission, effort, stop, restart, hand to terminal, close). For
      that, `rpSessMake` is parameterized by the clicked rid: it reads that
      rid's state from `/agents/<rid>` rather than the viewed page's
      `lead.st`, and its actions post to that rid.
    - It is placed with `popPlace`. Escape closes it and focus returns to the
      row.
12. **Keyboard path (outliner convention).**
    - On a focused row: Alt+↑/↓ reorders among its level; Alt+→ attaches to
      the previous top-level sibling (as its last child); Alt+← detaches.
    - The row handler calls `preventDefault` on Alt+← and Alt+→: they are the
      browser's Back/Forward on Windows and Linux, and under the iframe
      shell a Back would pop a `pushState` entry (design review 2026-11-14,
      autonomous).
    - Each op announces its result through an aria-live line ("EXEC → 메인
      아래로").
    - Invalid ops are refused with the same reason strings as the route.
13. **Compatibility tier: T0.** A new `rail-layout.json` store with
    `schema_version`, and one op-based `POST /rail/layout` (ops `move`,
    `attach`, `detach`, `star`, `unstar`, `wsmove`, `import`); additive. No
    change to `Registration`, the label store, mailbox addresses, or the
    `/agents/<rid>/*` actions.
14. **Star after the name; the branch row indents ((554)=a, owner
    2026-11-14, relayed by MAIN).** UX spec decision 13 (archived
    `ux-spec.md`); mockup
    `ai-docs/tickets/assets/261113-feat-ws-dashboard-session-rail-tree/session-rail-branch-indent-mockup.html`
    (column "안 A").
    - Owner gloss, verbatim: "session-rail feedback reading A: star moves
      right after the session name, hover-revealed (starred rows always show
      it); branch row indents into the freed star column -> workspace /
      worktree / session = 12 / 28 / 49 px".
    - Today the workspace bar (`.rail .wsbar`, padding 12 px,
      dashboard.css:1564) and the branch row (`.rail .wthd`, padding 12 px,
      dashboard.css:1587) share one column, and the star button is the row's
      first child, before the `<a class="rl">` link (`rrow`,
      dashboard.tmpl:288; `.rail .star`, dashboard.css:1612-1619).
    - The star moves to just after the name (after the hosted icon when
      present). It shows on row hover or focus-within, and a starred row
      always shows its filled star. It keeps its width while hidden, so
      revealing it never reflows the name (as Decision 10 requires); the
      name ellipsizes before the star, so the star never leaves the row.
    - The `.star` button stays outside the `<a class="rl">` link (no nested
      interactive content). The worker picks how to place it after the name,
      for example by splitting the link so that it wraps only the row's text
      parts.
    - The branch row (`.wthd`) indents to the star's old column (28 px), and
      the worktree guide line (`.rail .rsessions` border, dashboard.css:1593)
      moves under the branch icon (about 34 px). The harness icon stays at
      49 px: workspace / worktree / session = 12 / 28 / 49 px.
    - The 14 px column between the guide line and the harness icon, where
      the star was, is the grip's (Decision 1). The pencil and clipboard
      keep the time slot on the right edge (Decision 10). Child rows
      (Decision 5) indent from 49 px; under (540)=a a child shows no star.
    - Rejected: reading B, "the branch moves next to the workspace title,
      revealed on hover, and the branch row goes away. A repository with
      several worktrees would then lose the per-worktree grouping of its
      sessions."
15. **Drag feedback and reach (Background failure 4; MAIN's read, settled
    in the spec's Phase 1).**
    - The drop line is clamped to the list's visible edge instead of hidden
      when its y falls outside the list (`rdOver`, dashboard.js:5776).
    - The edge auto-scroll zone grows from 28 px to 48 px (`RD_EDGE`,
      dashboard.js:5732).
    - The order is applied once after the drop is saved, so `pin` re-runs on
      the new order and a swap landing right after a drop cannot re-order
      what was just dropped. In Phase 2 the saved order is the server's.
    - The grip carries `touch-action: none` (Decision 1); `.rail .rs`
      already has `user-select: none` (dashboard.css:1607).

## Constraints

- T0 only: a new store file with `schema_version`, and a new route. No change
  to `Registration`, the label store, mailbox addresses, or the
  `/agents/<rid>/*` actions.
- Layout writes use the rail's existing write gate (local viewer + same-origin
  POST).
- No row node is moved while its rename input is open or a drag runs. Hold the
  swap, as `headHeld` and `railHeld` do.
- New UI strings (hints, menu entries, aria-live text) go through `wsT` in
  every `internal/wsi18n/tables/*.json`. Non-English entries are worker
  translations: flag them unchecked.
- The rail moves into the shell document with
  `261113-feat-ws-dashboard-iframe-shell`; whichever lands second rebases.
- Browser checks run in a repo-local headless harness on the pattern of
  `261113-feat-ws-dashboard-board-reorder-hold`'s `scripts/board-headless.mjs`:
  a developer command beside `scripts/dev.py`, not shipped, driving a dev
  daemon from `scripts/dev.py rerun daemon`, Playwright resolved from the npx
  cache with no `package.json` and no Playwright dependency, each scenario
  asserting and exiting non-zero on failure. If that file has landed when
  Phase 1 starts, Phase 1 adds rail scenarios to it; otherwise it creates
  `scripts/rail-headless.mjs` on the same pattern. Later phases add
  scenarios to whichever file Phase 1 chose. "The headless harness" in the
  phases below means that file.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 939a2c59 (2026-11-12, commit): "One flat JSON array per kind (ws.dashboard.railorder by reply-id, ws.dashboard.wsorder by path) instead of a per-group map: a session belongs to exactly one group" - bearing: superseded by Decision 3 (owner 534=c, 538) in Phase 2
- a7b53d6b (2026-11-12, commit): "(408)=a order in localStorage beside stars (ws.dashboard.railorder / wsorder); (409)=b sessions within their group plus workspace sections; (410) new sessions on top, stars first, ended fold excluded." - bearing: (408)=a superseded by Decisions 3 and 8 (owner 534=c, 538) in Phase 2; (409)=b and (410) constrain
- 939a2c59 (2026-11-12, commit): "A rail swap arriving mid-drag is held (like the head label edit) and lands at the drag's end, instead of aborting the drag; railHeld clears if the held hash turns out to be the current one." - bearing: constrains
- 939a2c59 (2026-11-12, commit): "drop() is the refusal point (other group, other kind, across the stars, ended rows, itself) and is node-tested; dashboard.js only offers peers from the same list and star part" - bearing: "across the stars" superseded for attach by Decision 8 (owner 534=c tree; Phase 3); the rest constrains
- 939a2c59 (2026-11-12, commit): "Touch drags after a 380ms still hold (a swipe keeps scrolling the list); a non-passive touchmove and contextmenu guard stop scroll/long-press menu only while a touch drag is held." - bearing: the 380 ms hold superseded by Decision 1 (owner 537 ok) in Phase 1; the touchmove guard during a held drag constrains
- 939a2c59 (2026-11-12, commit): "pin() takes the order as a third argument: sort key is star, then rank (an id not in the order ranks -1, so a new session lands on top), then data-ord (server order)." - bearing: constrains
- 261113-feat-ws-dashboard-iframe-shell (2026-11-13, Decisions): "rail order, stars, and collapse state are browser-storage only (`assets/rail.js#L1-L15`, \"nothing reaches the server\")" - bearing: order and stars superseded by Decisions 3 and 8 (owner 534=c, 538) in Phase 2, which narrows the shell's (501) pre-paint state to collapse; collapse constrains
- 0e96b4b0 (2026-11-13, commit): "Chose option 6b: Go renders the opening attributes and rename JavaScript only escapes and appends the label, avoiding duplicate attribute logic or a route change." - bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/ rail.go, pageview_rail.go, new rail-layout store and route, assets/rail.js, assets/dashboard.js, assets/dashboard.css, assets/dashboard.tmpl, internal/wsi18n/tables/*.json; repo-local scripts/ headless harness (not shipped) |
| scope.surface | cross-module | new POST /rail/layout route and rail-layout.json store, RailRow gains fields, browser localStorage keys retired; same wsdashboard package plus wsi18n tables |
| scope.new_public_symbol | yes | POST /rail/layout route, rail-layout.json store, WsRail zone/validity function, RailRow fields (project path, mailbox ref) |
| scope.new_type_contract | yes | rail-layout.json schema_version 1 (order, children, stars, workspace order) and the layout op request {op, rid, parent, before, after, path, order, stars, wsorder} with ops move/attach/detach/star/unstar/wsmove/import |
| scope.test_surface | existing | rail_test.go, railscript_test.go (node harness for rail.js), rail_hotkey_test.go, label_route_test.go; new Go layout-store tests; browser checks in the headless harness on the board-reorder-hold pattern (scripts/board-headless.mjs or a sibling scripts/rail-headless.mjs) |
| complexity.reuse_points | confirmed | workspaceStore re-read pattern (workspaces.go#L50-L77), labelStart/labelKey/labelSave (dashboard.js#L2518-L2552), MailboxOpen/MailboxRef (pageview.go#L111-L138), handleSessionLabel (label_route.go#L110), WsRail.drop/pin (rail.js#L164-L202), rpSessMake (dashboard.js#L6742), broker.wake(railListRepoKey) (workspace_routes.go#L224) |
| complexity.side_effect_risk | high | rail swap, drag, head label edit and star/order import all interact; rail moves into the shell document concurrently (iframe-shell); localStorage retirement affects every browser |
| risk.correctness | high | swap/press races, depth-1 validation, concurrent two-browser layout merges, ended/forgotten parent re-nest rules |
| risk.fit | high | four-phase UX rework over rail.js/dashboard.js hot paths and a pending rebase against the iframe shell, worktree-delete and usage-gutter row changes |
| risk.test | high | drag, hover, long-press and context-menu behaviors need browser-level verification; the repo has a node harness for rail.js only |
| risk.security_or_contract | moderate | new write route behind the existing local-viewer + same-origin gate; T0 additive store with schema_version |

## Phases

### Phase 1: reliable reorder + hover rename/copy (no tree, local order kept)

Decisions 1, 2, 10, 14, 15. This fixes the reported "drag does not take",
ships the pencil/clipboard, moves the star after the name with the 12 / 28 /
49 px branch indent, and sets up the headless harness (Constraints). Order is
still local in this phase. Touch users have no rail rename/copy until Phase 4
(Decision 10, accepted interim).

Verification:
- node tests for the press/swap rules (a swap during the press is held, and a
  replaced node is re-found).
- Headless harness: a grip drag reorders with a rail swap injected mid-press;
  a press on the row body navigates and never drags; a refused cross-group
  drop shows the hint. The pencil renames through `/s/<rid>/label` (the name
  updates in the rail and, for the viewed session, in the head), and a swap
  during the edit does not drop typing. The clipboard copies the same string
  the head name copies (for a session in a listed worktree).
- Headless harness, Decision 15 and the grip: with the pointer past the list
  edge, the drop line shows clamped at the edge (not hidden); a pointer 40 px
  inside the list edge auto-scrolls (inside 48 px, outside the old 28 px); a
  rail swap injected right after a drop keeps the dropped order; the grip's
  computed `touch-action` is `none`.
- Headless harness, Decision 14: the star sits after the name, hidden until
  row hover or focus and always shown on a starred row; clicking it stars
  without navigating; a long name ellipsizes before the star. Measured left
  offsets of the workspace icon, branch icon and harness icon are 12 / 28 /
  49 px.
- LOOK: captures of the rail at the default width and in the overlay rail,
  idle and with a row hovered, placed side by side with the mockup's "안 A"
  column
  (`ai-docs/tickets/assets/261113-feat-ws-dashboard-session-rail-tree/session-rail-branch-indent-mockup.html`);
  the hierarchy, star position and grip column match it.

### Result (898b56491) - 2026-11-14

Landed in 7ce5df3bb, f4122e7d4, 9c694d73b and 898b56491.

- **rail.js:** `dropRefusal`, `itemKey`/`refind` and `swapGate`, the pure parts of
  the press/swap rules, with node tests in `railscript_test.go`.
- **Head rename:** the head's rename runs from an edit object, so rail rows
  reuse `labelStart`/`labelKey`/`labelSave`.
- **Rows:**
  - A grip in the 14 px column. The star sits after the name, outside the link.
  - The link is split into `.rl` (the name, the only tab stop) and the tail and
    message links (`tabindex -1`), so the whole row still navigates.
  - The pencil and clipboard sit over a fixed 44 px time slot.
  - The workspace / worktree / session indent is 12 / 28 / 49 px.
- **Copy tag:** the head and each rail row build it with one Go builder. Its
  inputs come from the rail view (Decision 10).
- **Drag:** a rail swap is held from the press on a grip or a bar. A refused drop
  shows a hint and the not-allowed cursor. The drop line is clamped to the list
  edge, `RD_EDGE` is 48 px, and the order is applied once after the drop.
- **New i18n keys:** `rail.copy`, `js.rail.drag.ws`, `js.rail.drag.wt` and
  `js.rail.drag.stars` in every table. The non-English entries are unchecked
  worker translations, listed in 9c694d73b.
- **Headless harness:** `scripts/rail-headless.mjs`.
  - Overriding the Constraints text by the lead's instruction, it never runs
    `dev.py rerun daemon`.
  - It builds `ws-mcp` and starts its own `ws-mcp dashboard --port 0` with a temp
    `WS_CACHE_HOME`, `WS_CONFIG_HOME` and `HOME`, and seeds fake registrations.
  - Later phases append to its `SCENARIOS`.

Verification:
- `go build ./... && go vet && go test ./internal/wsdashboard/ ./internal/wsi18n/...`:
  ok.
- `node scripts/rail-headless.mjs`: all 12 scenarios passed (offsets, star, grip,
  body-press, grip-reorder with a swap mid-press, swap-after-drop, refused-hint,
  clamp, edge-scroll at 40 px, rename with a swap mid-edit, copy equal to the
  head's, look).
- LOOK: captures at the column and overlay widths match the mockup's "안 A" for
  the hierarchy, star position and grip column.

Decisions:
- A row's hover and current background starts at the guide line and includes the
  grip; the mockup starts it at 49 px.
- A group's only row still drags, so that the refusal hint explains why it
  cannot move.
- A window blur ends a press, so that a lost release cannot hold swaps forever.
- A plain-folder row copies a tag without `root` or `branch`, as the head does
  outside git.
- The pencil reuses the `head.rename` string.

### Phase 2: server-side layout store and route

Decisions 3, 4, 13. The server orders rows from the store, there is a
one-time import of the local order (and, per decision 8, stars and workspace
order), and `sessionOrder` is retired. All ops land in the route here; the
tree renders in Phase 3. The iframe shell's (501) pre-paint state narrows to
collapse (Decision 3).

Verification:
- Go tests: store round trip and `schema_version`; re-read before write (two
  writers both kept); op validation (depth, workspace, self, ended); per op:
  `move` (same level, same worktree group, same star section), `attach`,
  `detach` (next to the old parent in the same worktree group, else the head
  of its own group), `star`/`unstar` (unknown rid refused, `star` on a child
  refused, idempotent), `wsmove` (unknown or self path refused), `import`
  (applied on an empty file, "not applied" on a file with any entry, unknown
  rids dropped, retryable refusal before the index loads); an op on an
  orphan, and an attach under it, drops its dormant link first.
- Go tests, edge rules: an entry for an unknown rid is filtered on read and
  kept by a write made before the index loads, pruned by one made after; a
  layout write changes the rail stamp and wakes a rail stream.
- Headless harness, two browser contexts: a reorder in one shows in the other
  after its rail swap; a star toggle and a workspace reorder in one show in
  the other; stars persist across both.
- Headless harness, import: an empty server layout plus local `railorder`,
  `stars` and `wsorder` keys imports them and clears the keys; a browser
  that then loads with stale local keys clears them and shows the server
  order; two browsers that both try to import leave exactly one import
  applied and both clear their keys.

### Result (09d894cc0) - 2026-11-14

Landed in 51429395e, 845578455, and 09d894cc0.

- **Store.** `rail_layout.go` adds the `rail-layout.json` store in the daemon
  directory, at `schema_version` 1:
  `{schema_version, workspace_order, workspaces: {path: {order, children, stars}}}`.
  - Every op re-reads the file under a mutex before it writes.
  - A malformed file, or one with a newer schema, refuses the write.
  - Unknown fields are kept, both at the top level and per workspace.
  - Unknown rids are filtered on read. A write prunes them only after
    `index.Loaded()`, which is new.
- **Route.** `rail_layout_route.go` serves `POST /rail/layout` with all seven
  ops and Decision 4's validation, orphans included.
  - Validation uses the rail's own grouping: `railBaseView`.
  - Answers: 200 `{applied}`; 200 `not_applied` (import); 409 for a refusal; 400
    `invalid`; 503 `retry`; 500 `store`.
  - The reason codes are a stable set shared with the client: `unknown`,
    `ended`, `self`, `depth`, `workspace`, `group`, `level`, `stars`, `child`,
    `toplevel`, `invalid`, `retry`, `store`. Each maps to the i18n key
    `js.rail.layout.<code>`.
  - Phase 1's drag-hint keys were renamed onto `workspace`, `group`, and
    `stars`.
- **Ordering.** The server orders rows: starred units first, then the layout
  order (rows it does not hold go on top), then registration order. It orders
  workspaces by `workspace_order`.
  - The star state is rendered by the server.
  - A layout generation is added to the rail stamp (`data-lgen`), and every
    write wakes `railListRepoKey`.
  - In this phase a stored child renders flat, right after its parent.
- **Client.**
  - A drop moves the row at once, posts `move` or `wsmove`, and undoes the move
    on a refusal.
  - A star click posts `star` or `unstar`.
  - A swap is held while an op is in flight. A render whose `data-lgen` is
    older than the last applied one is dropped.
  - The one-time `import` follows Decision 3's edge rules. Local keys are kept
    on `retry`.
  - The only client layout state left is the `railcol` collapse set. No order or
    star pre-paint code remains, so the iframe shell rebases onto collapse
    only.

Verification:
- `gofmt`, `go build ./...`, `go vet`, and
  `go test ./internal/wsdashboard/ ./internal/wsi18n/...`: ok.
- `node scripts/rail-headless.mjs`: all 16 scenarios passed, in two runs. They
  include `layout-shared` (two browsers: reorder, star, and workspace order),
  `layout-import`, `layout-stale-keys`, and `layout-import-race` (exactly one
  import applied).
- The non-English entries for the ten new `js.rail.layout.*` keys are unchecked
  worker translations.

Decisions:
- **Write gate.** The route takes same-origin only, like the workspace routes.
  The Constraints line names "local viewer + same-origin", but the existing rail
  write routes (workspace add, forget, and worktree add) check same-origin
  only. `TrustedWrite`'s local-viewer check is kept for writes that carry
  execution authority, such as worktree delete. A layout edit carries none.
- **Empty layout.** The layout counts as empty only when it holds no entry at
  all, whether or not the rail still lists the stored rids.
- **`not_applied`.** It is answered with 200 and is never toasted.

### Phase 3: the tree

Decisions 5-9. Rendering, the three-zone drop, attach/detach, the collapse
roll-up, ended/forgotten rules, stars vs tree.

Verification:
- node tests for the zone/validity function (valid parent, a parent with
  children dragged, cross-workspace, ended target, child zone absent on a
  child row, a top-level detach line refused outside the child's own
  worktree group, an orphan as a valid parent).
- Go view tests: a child row has no message line and has its branch tag when
  its worktree differs; a parent with a child waiting on approval or working
  carries the roll-up marks in its markup whatever its collapse state; an
  ended parent's live child in another worktree renders top-level in its own
  worktree group, and one in the same worktree in the parent's slot.
- Headless harness: attach EXEC and UI under 메인 by middle drop, reorder
  the children, detach one by a top-level line, collapse 메인 while a child
  waits on approval (the roll-up badge is visible on 메인 only while
  collapsed). Screenshots at the default rail width and the overlay rail.

### Result (764f4c60c) - 2026-11-14

Landed in 6c7678eab, d0f1f5bb7, 5f3c6bb09, 1970aafeb, 92e2f5d35 and
764f4c60c.

What landed:
- `WsRail.dropZone(d, t, frac)` in rail.js is the pure zone and validity
  function.
  - `d` and `t` come from `WsRail.rowFacts(row)`: `{rid, ws, group, live,
    parent, kids, star, first}`.
  - It returns `{zone, ref, op}`, `{refusal: <code>}` or `null`.
  - The refusal codes are the same as the route's, and each refusal follows the
    route's check order for the op the drop would post.
  - `dropNoop` and `moveRow` sit beside it. Phase 4's keyboard path reuses
    them.
- The Go view nests children under their parent, inside a sibling
  `<div class="rkids" data-parent>`.
  - A child row is one line, carries its message as its `title`, shows no star,
    and shows a branch tag when its worktree group differs from its parent's.
  - The parent's roll-up marks are always rendered: the `.kap` approval badge
    with the child count, and the working orb. CSS shows them only while the
    parent is collapsed.
  - Orphans follow Decision 7: the parent's slot in the same worktree, else the
    head of their own group.
- The client drag uses three zones per row: a quarter line before, a quarter
  line after, and a middle child zone with an outline, a tint and "↳".
  - A drop line sits at the indent of the level it lands on.
  - A parent drags with its whole unit.
  - A drop posts `attach`, `detach` or `move`. A refusal toasts the mapped
    reason.
- Collapse:
  - The set is kept per browser in localStorage under `ws.dashboard.railkids`,
    and the parent shows a chevron with the child count.
  - Left and Right on a focused parent row collapse and expand it, only with no
    modifier key.
  - Children step in 16 px, or 12 px in the overlay rail.

Verification:
- `gofmt`, `go build ./...`, `go vet` and
  `go test ./internal/wsdashboard/ ./internal/wsi18n/...` pass. These include
  the node zone tests and the Go view tests listed above.
- `node scripts/rail-headless.mjs` passes all 17 scenarios, among them the
  new `tree` scenario:
  - attach EXEC and UI under 메인;
  - reorder the children;
  - detach one by a top-level line;
  - collapse 메인 while UI waits on approval (seeded through the snapshot's
    permission phase): the roll-up badge shows only while collapsed.
  - Screenshots were taken at the column and overlay widths.

Decisions:
- A session leaves the rail only by leaving the index, because the dashboard has
  no session-forget route. Its link is then dropped by the Phase 2 read filter
  and prune, so no forget hook was added.
- "After" an expanded parent lands before its first child, at the child indent.
- A parent dragged over another parent's child is refused as `depth`.
- The refused-hint scenario now reads the cross-worktree and cross-star
  refusals on a row's top edge, because the middle of the row is now a valid
  attach target.
- The non-English entries of the `rail.kids.*` keys are unchecked worker
  translations.

Known limits:
- A child whose own worktree group has no top-level row has no drop line for a
  detach. Phase 4's Alt+← and the context menu cover that case.
- The collapse set never prunes ids.

### Phase 4: context menu and keyboard path

Decisions 11, 12.

Verification:
- Headless harness: right-click a non-hosted row (rename/copy/move
  under/detach); right-click a hosted row other than the viewed one, change
  its effort, and check that `/agents/<that rid>/settings` was hit, not the
  viewed rid's. Long-press under touch emulation opens the menu, and the grip
  drags. Shift+F10 opens it from the keyboard; Alt+arrows reorder, attach
  and detach with an aria-live announcement, and Alt+← / Alt+→ leave the
  page URL and history length unchanged.
