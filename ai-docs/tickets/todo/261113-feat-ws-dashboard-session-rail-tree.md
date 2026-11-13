---
title: "Dashboard session rail: reliable reorder, hover rename/copy, depth-1 session tree, context menu"
related:
  261113-feat-ws-dashboard-iframe-shell: "ready, in flight; the rail moves into the shell document - whichever ticket lands second rebases"
  261113-feat-ws-dashboard-board-reorder-hold: "todo; separate board topic DnD, no shared code"
---

# Dashboard session rail: reliable reorder, hover rename/copy, depth-1 session tree, context menu

## Background

Owner request (MAIN's mail, 2026-11-13 14:29Z) with a rail screenshot; UX spec
read against develop 14aabe4b5. The spec is archived verbatim at
`ai-docs/tickets/assets/261113-feat-ws-dashboard-session-rail-tree/ux-spec.md`.
Owner answers (relayed by MAIN, 2026-11-13) are folded into the Decisions
below.

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
  dashboard.js:5726-5862). The whole row (a link) is the press target. The
  touch path is a 380 ms hold (`RD_HOLD`).
- Rename exists only for the viewed session's head (`labelStart`,
  dashboard.js:2518; POST `/s/<rid>/label`, `handleSessionLabel` in
  `label_route.go:110`); the per-rid route is already there. The address
  copy exists only on the head name (`data-copy` from
  `SessionView.MailboxOpen`/`MailboxRef`, pageview.go:111-134).
- The hosted session menu (`rpSessMake`, dashboard.js:6532-6627) is bound to
  the viewed session: its `rid` and `lead.st`.

### Likely reorder failures (UX read; reconcile with MAIN's session-rail log)

1. **A swap during the press kills the drag.** `swap()` (dashboard.js:496)
   holds a rail swap only while `rdrag.on`. A press still inside `RD_SLOP`,
   or a touch still in its hold, keeps a node that the swap replaces.
   `rdBegin` then sees `!d.item.isConnected` (dashboard.js:5795) and drops the
   drag silently. With per-message rail updates this is frequent.
2. **Silent refusals.** Peers are only the same parent node, the same star
   state, and not ended. A worktree group with one live session has
   `rdPeers < 2`, so no drag starts. Dropping across worktree groups or across
   the star boundary is refused with no feedback.
3. **The row is a link.** The press competes with link navigation, text
   selection and the native drag (cancelled). Small slop under a trackpad
   triggers navigation instead. `.rs` has no `user-select` or `touch-action`
   rule (MAIN).
4. **Drag feedback and reach (MAIN).**
   - The drop line is hidden whenever its y falls outside the list bounds
     (dashboard.js:5770). Near the edges the drop target exists but is
     invisible.
   - Edge auto-scroll only engages within 28 px of the list edge
     (`RD_EDGE`, dashboard.js:5726).
   - `WsRail.pin` re-sorts the stars on every swap (rail.js:164). A swap that
     lands right after a drop can re-order what was just dropped.

Phase 1 addresses all four: (1) Decision 2; (2) the refusal hint; (3) the
grip, with `user-select: none` and `touch-action: none` on it; (4) the drop
line is clamped to the list edge instead of hidden, the edge zone grows to
48 px, and the order is applied once after the drop is saved, so `pin`
re-runs on the new order (in Phase 2, the server order).

## Decisions

1. **Drag handle, separate from the link (537 ok, owner 2026-11-13).**
   - Each live row gets a grip in a reserved left gutter (14 px; the row
     already sits indented under the worktree guide line). The grip is a
     separate element from the row's `<a>`.
   - Mouse: the grip shows on row hover or focus-within. The press starts the
     drag at once past `RD_SLOP`. A press on the rest of the row is the link,
     never a drag.
   - `user-select: none` applies on the grip and on the rail rows during a
     drag, so the press never selects text.
   - Coarse pointers (`@media (pointer: coarse)`): the grip is always visible
     and starts the drag on press (`touch-action: none` on the grip), with no
     hold.
   - Workspace bars keep their bar-as-handle behaviour.
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
     local order posts it once (import), then clears the local key.
   - Compatibility tier: T0. It is a new store file with `schema_version` and
     a new route; no existing store, tool, or event changes.
4. **One route with validated ops.**
   - `POST /rail/layout {op, rid, parent?, before?, after?}`. `op` is one of
     `move` (reorder at top level or among siblings), `attach` (become a child
     of `parent` at a position), `detach` (become top-level next to the old
     parent).
   - The server validates:
     - depth 1: a parent is never a child, and a session with children cannot
       become a child;
     - the same workspace (536=a, owner 2026-11-13: same workspace only);
     - no self-attach, no ended target parent.
   - Ops, not whole-array writes, so concurrent edits merge. A refusal returns
     a reason string that the client toasts. The route takes the same gate as
     the other rail write routes (local viewer + same origin).
5. **Tree rendering.**
   - A child renders under its parent, indented one step (16 px; 12 px in the
     overlay rail), with a guide line.
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
   - Keyboard: Left/Right on a focused parent row collapses/expands it.
7. **Ended or forgotten parent; ended child.**
   - (539)=a, owner 2026-11-13: when a parent ends with live children, the
     children go top-level, in the parent's slot. The stored link is kept, and
     they re-nest when the parent is revived. Rejected: (b) the ended parent
     stays in the live list as a dimmed group head while it has live
     children.
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
     line at the top level detaches a dragged child.
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
      `MailboxRef` (the same builder as the head, pageview.go:111-134) via a
      new `RailRow` field. The row needs the project path (`RailRow` has none
      today; the builder's `dir=` attribute needs it, and `root`/`branch`
      come from git state as for the head).
    - Coarse pointers: there is no hover, so rename and copy live in the
      context menu (decision 11).
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
    - Each op announces its result through an aria-live line ("EXEC → 메인
      아래로").
    - Invalid ops are refused with the same reason strings as the route.
13. **Compatibility tier: T0.** A new `rail-layout.json` store with
    `schema_version`, and one op-based `POST /rail/layout`; additive. No
    change to `Registration`, the label store, mailbox addresses, or the
    `/agents/<rid>/*` actions.

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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: reliable reorder + hover rename/copy (no tree, local order kept)

Decisions 1, 2, 10. This fixes the reported "drag does not take" and ships the
pencil/clipboard. Order is still local in this phase.

Verification:
- node tests for the press/swap rules (a swap during the press is held, and a
  replaced node is re-found).
- Playwright: a grip drag reorders with a rail swap injected mid-press; a
  press on the row body navigates and never drags; a refused cross-group drop
  shows the hint. The pencil renames through `/s/<rid>/label` (the name
  updates in the rail and, for the viewed session, in the head), and a swap
  during the edit does not drop typing. The clipboard copies the same string
  the head name copies.

### Phase 2: server-side layout store and route

Decisions 3, 4, 13. The server orders rows from the store, there is a
one-time import of the local order (and, per decision 8, stars and workspace
order), and `sessionOrder` is retired.

Verification:
- Go tests: store round trip and `schema_version`; re-read before write (two
  writers both kept); op validation (depth, workspace, self, ended); a layout
  entry for a rid that no longer exists is dropped on read.
- Playwright with two browser contexts: a reorder in one shows in the other
  after its rail swap.

### Phase 3: the tree

Decisions 5-9. Rendering, the three-zone drop, attach/detach, the collapse
roll-up, ended/forgotten rules, stars vs tree.

Verification:
- node tests for the zone/validity function (valid parent, a parent with
  children dragged, cross-workspace, ended target, child zone absent on a
  child row).
- Go view tests: a child row has no message line and has its branch tag when
  its worktree differs; a collapsed parent carries a child's approval badge.
- Playwright: attach EXEC and UI under 메인 by middle drop, reorder the
  children, detach one by a top-level line, collapse 메인 while a child waits
  on approval (the badge shows on 메인). Screenshots at the default rail width
  and the overlay rail.

### Phase 4: context menu and keyboard path

Decisions 11, 12.

Verification:
- Playwright: right-click a non-hosted row (rename/copy/move under/detach);
  right-click a hosted row other than the viewed one, change its effort, and
  check that `/agents/<that rid>/settings` was hit, not the viewed rid's.
  Long-press under touch emulation opens the menu, and the grip drags.
  Shift+F10 opens it from the keyboard; Alt+arrows reorder, attach and detach
  with an aria-live announcement.
