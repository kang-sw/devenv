# session-rail: reliable reorder, hover rename/copy, depth-1 tree, context menu (spec draft, UX session)

Source: the owner's request (quoted in MAIN's mail, 2026-11-13 14:29Z) and the
screenshot `dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/335914850f447e78-1.png`.
Owner answers: (534)=c tree and order server-side; (535)=a a right-click
menu on every row; (536)=a children only within the same workspace; (537)
ok on MAIN's technical direction; (538) ok; (539)=a; (540)=a. Code read
against develop 14aabe4b5. Decision 13 (2026-11-14 owner feedback): (554)=a, reading A.

Suggested stem: `261113-feat-ws-dashboard-session-rail-tree`.

## Background

- The rail region is server-rendered (`rail.go`, `pageview_rail.go`,
  `RailRow`). It is replaced whole on every rail swap. Every row's second
  line (`Message`, the session's last message) changes, so swaps are
  frequent.
- Client order is local. `WsRail.pin` orders starred rows first, then the
  drag order (`sessionOrder`, localStorage); `arrange` orders workspaces
  (`wsOrder`, localStorage). `drop()` refuses: another list, the other side
  of the stars, ended rows, and the item itself.
- Drag lives in dashboard.js (`rdStart`/`rdBegin`/`rdMove`/`rdEnd`, about
  lines 5720-5865). The whole row (a link) is the press target. The touch
  path is a 380 ms hold.
- Rename exists only for the viewed session's head (`labelStart`, POST
  `/s/<rid>/label`, `label_route.go`), and the per-rid route is already
  there. The address copy exists only on the head name (`data-copy` from
  `SessionView.MailboxOpen`, pageview.go:111).
- The hosted session menu (`rpSessMake`, dashboard.js ~6532) is bound to
  the viewed session: its `rid` and `lead.st`.

### Likely reorder failures (UX read; reconcile with MAIN's session-rail log)

1. **A swap during the press kills the drag.** `swap()` holds a rail swap
   only while `rdrag.on`. A press still inside `RD_SLOP`, or a touch still in
   its hold, keeps a node that the swap replaces. `rdBegin` then sees
   `!d.item.isConnected` and drops the drag silently. With per-message
   rail updates this is frequent.
2. **Silent refusals.** Peers are only the same parent node, the same star
   state, and not ended. A worktree group with one live session has
   `rdPeers < 2`, so no drag starts. Dropping across worktree groups or
   across the star boundary is refused with no feedback.
3. **The row is a link.** The press competes with link navigation, text
   selection and the native drag (cancelled). Small slop under a trackpad
   triggers navigation instead. `.rs` has no `user-select` or
   `touch-action` rule (MAIN).
4. **Drag feedback and reach (MAIN).**
   - The drop line is hidden whenever its y falls outside the list bounds
     (dashboard.js ~5771). Near the edges the drop target exists but is
     invisible.
   - Edge auto-scroll only engages within 28 px of the list edge.
   - `WsRail.pin` re-sorts the stars on every swap (rail.js:164). A swap
     that lands right after a drop can re-order what was just dropped.

Phase 1 addresses all four:
- (1) Decision 2.
- (2) The refusal hint.
- (3) The grip, with `user-select: none` and `touch-action: none` on it.
- (4) The drop line is clamped to the list edge instead of hidden; the
  edge zone grows to 48 px; and the order is applied once after the drop
  is saved, so `pin` re-runs on the new order (in Phase 2, the server
  order).

## Decisions

1. **Drag handle, separate from the link (537).**
   - Each live row gets a grip in a reserved left gutter (14 px; the row
     already sits indented under the worktree guide line).
   - Mouse: the grip shows on row hover or focus-within. The press starts
     the drag at once past `RD_SLOP`. A press on the rest of the row is the
     link, never a drag.
   - Coarse pointers (`@media (pointer: coarse)`): the grip is always
     visible and starts the drag on press (`touch-action: none` on the
     grip), with no hold.
   - Workspace bars keep their bar-as-handle behaviour.
2. **Swaps never kill a press.**
   - A rail swap is held from `pointerdown` on a grip (not only once the
     drag is on) until `rdEnd`.
   - If anything replaces the node anyway, the drag re-finds its item by
     `data-rid` / `data-ws` after the swap instead of aborting.
   - A refused drop shows a "not allowed" cursor and a short inline hint
     ("다른 워크스페이스로는 옮길 수 없음", etc.), not a silent no-op.
3. **Layout lives server-side (534=c).**
   - A new daemon store `rail-layout.json` with `schema_version: 1`, holding
     per workspace path: `order` (the top-level reply-ids) and `children`
     (parent reply-id -> ordered child reply-ids).
   - Every change re-reads the file first (the `workspaceStore` pattern), so
     two browsers or processes do not lose each other's edits.
   - The server orders the rail rows from it, and every browser sees the
     same tree through the normal rail swap. `sessionOrder` in localStorage
     is retired.
   - The first page load that finds an empty server layout and a local
     order posts it once (import), then clears the local key.
   - Compatibility tier: T0. It is a new store file and a new route; no
     existing store, tool, or event changes.
4. **One route with validated ops.**
   - `POST /rail/layout {op, rid, parent?, before?, after?}`. `op` is one of
     `move` (reorder at top level or among siblings), `attach` (become a
     child of `parent` at a position), `detach` (become top-level next to
     the old parent).
   - The server validates:
     - depth 1: a parent is never a child, and a session with children
       cannot be attached;
     - the same workspace (536=a);
     - no self-attach, no ended target parent.
   - Ops, not whole-array writes, so concurrent edits merge. A refusal
     returns a reason string that the client toasts. The route takes the
     same gate as the other rail write routes (local viewer + same origin).
5. **Tree rendering.**
   - A child renders under its parent, indented one step (16 px; 12 px in
     the overlay rail), with a guide line.
   - A child row is one line: its second line (`Message`) is dropped, as
     the owner asked. The full message stays reachable as the row's
     `title` tooltip.
   - Row-one signals stay on child rows: the approval badge, working,
     failure, the subagent count, and the time.
   - A child whose worktree differs from its parent's shows a small branch
     tag on its line, because 536=a allows any worktree of the same
     workspace.
6. **Collapse/expand.**
   - A parent with children shows a chevron and a child count. Default:
     expanded. The collapsed state is a view preference, kept per browser
     like the workspace `collapsed` set.
   - A collapsed parent rolls up its children's attention onto its own
     row: any child approval wait shows as the parent's "?" badge (with the
     child count in the tooltip), and any working child shows the working
     mark. A collapse must never hide an approval request.
   - Keyboard: Left/Right on a focused parent row collapses/expands it.
7. **Ended or forgotten parent; ended child.**
   - (539)=a: when a parent ends, its live children are shown at
     the top level in the parent's slot. The stored link is kept, and they
     re-nest when the parent is revived.
   - Forgetting the parent drops the link for good.
   - A child that ends goes to the ended fold like any row, keeping its
     link, and re-nests on revive.
8. **Stars vs tree.**
   - Stars order top-level units: a parent moves with its children as one
     unit, so starred units come first, then the layout order.
   - (540)=a: a child cannot be starred. Its star button is hidden
     while it is a child; a star it had stays stored but inert, and is
     active again on detach.
   - Dropping into the middle of a parent (attach) is allowed across the
     star boundary. Reorder stays within its star section, as today.
   - Stars and the workspace order move into the server layout too ((538),
     announced). The layout store holds per workspace the starred rids and,
     at its top level, the workspace order. The localStorage `stars` and
     `wsOrder` keys are imported once with the session order, then
     retired.
9. **Drop affordance: three zones per target row.**
   - The top 25% of the row means before, the bottom 25% after, and the
     middle 50% child (attach). The middle zone exists only on a valid
     parent: top-level, live, the same workspace, not the dragged item
     itself, and the dragged item has no children. Elsewhere the row splits
     50/50 into before/after.
   - Before/after: the existing `.rdrop` line, starting at the indent of
     the level the drop lands on, so a line between two children reads as
     "among the children" and a line at the top-level indent reads as "top
     level".
   - A drop line among P's children attaches to P at that position. A
     drop line at the top level detaches a dragged child.
   - Child zone: the target row gets an accent outline and tint, plus a
     trailing "↳" glyph at the row's right edge. There is no line.
   - Dragging a parent drags its whole unit (children follow). Its children
     are not drop targets.
   - The pure zone/validity calculation goes in rail.js (like `drop()`),
     so it is node-testable.
10. **Hover icons: pencil and clipboard.**
    - On row hover or focus-within, the time label on the row's right edge
      hides, and a pencil and a clipboard take that slot. The name never
      reflows, because the icons do not take width from it. The approval,
      count and failure badges stay.
    - Pencil: the name becomes a one-line input. It reuses the head's
      `labelStart`/`labelKey`/`labelSave` logic, generalized to a rid and
      posting `/s/<rid>/label`: Enter saves, Escape or blur cancels, and an
      empty value clears the label. A rail swap while the input is open
      waits, like `headHeld`.
    - Clipboard: copies the row's `data-copy`. The server builds that
      string per row with the same builder as the head (537), via a
      `RailRow` field.
    - Coarse pointers: there is no hover, so rename and copy live in the
      context menu (decision 11).
11. **Context menu (535=a).**
    - It opens on `contextmenu` (right click), on long-press of a row on
      coarse pointers (the hold now means "menu", since drag moved to the
      grip), and on Shift+F10 or the ContextMenu key on a focused row.
    - Every row gets: rename, copy address, move under ▸ (the eligible
      parents in the workspace), detach (children only), and reopen (ended
      rows with a revive).
    - Hosted rows also get the existing session menu content (model,
      permission, effort, stop, restart, hand to terminal, close). For
      that, `rpSessMake` is generalized to a target rid: it reads that
      rid's state from `/agents/<rid>` rather than the viewed page's
      `lead.st`, and its actions post to that rid (537).
    - It is placed with `popPlace`. Escape closes it and focus returns to
      the row.
12. **Keyboard path (outliner convention).**
    - On a focused row: Alt+↑/↓ reorders among its level; Alt+→ attaches to
      the previous top-level sibling (as its last child); Alt+← detaches.
    - Each op announces its result through an aria-live line ("EXEC → 메인
      아래로").
    - Invalid ops are refused with the same reason strings as the route.
13. **Branch row hierarchy and the star after the title (owner feedback,
    2026-11-14; screenshot
    `dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/c3dacd3e8aaf6f02-1.png`;
    mockup `shared/docs/session-rail-branch-indent-mockup.html`).**
    - Problem: the workspace icon (`.wsbar`, padding 12 px) and the
      worktree branch icon (`.wthd`, padding 12 px) share one column, so
      the branch row competes with the workspace header.
    - Reading A, (554)=a: "별도
      타이틀의 오른쪽에" reads as "별(star)도". The star moves from the
      row's left gutter to just after the name (after the hosted icon). It
      is still revealed on hover or focus, and a starred row always shows
      its filled star. The name ellipsizes before the star, so the star
      never leaves the row.
    - The branch row (`.wthd`) indents to the star's old column (28 px),
      and the worktree guide line moves under the branch icon (about 34
      px). The harness icon stays at 49 px, so the rail reads 12 / 28 / 49
      px: workspace, worktree, session.
    - The drag grip (decision 1) takes the 14 px gutter between the guide
      line and the harness icon, where the star used to be. The pencil and
      clipboard (decision 10) keep the time slot on the right edge.
    - Child rows (decision 5) indent from 49 px as before. Under (540)=a a
      child shows no star.
    - Alternative reading (B), not recommended: the branch moves next to
      the workspace title, revealed on hover, and the branch row goes away.
      A repository with several worktrees would then lose the per-worktree
      grouping of its sessions.
    - Phase 1, because it touches the same row markup as the grip and the
      hover icons. The `.star` button must stay outside the `<a class="rl">`
      link (no nested interactive content). The worker picks how to place
      it after the name, for example by splitting the link so that it
      wraps only the row's text parts.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`;
  `agents-plugin-tool/internal/` ->
  `ai-docs/manuals/release-compatibility.md`.
- T0 only: a new store file with `schema_version`, and a new route. No
  change to `Registration`, the label store, mailbox addresses, or the
  `/agents/<rid>/*` actions.
- Layout writes use the rail's existing write gate (local viewer +
  same-origin POST).
- No row node is moved while its rename input is open or a drag runs.
  Hold the swap, as `headHeld` and `railHeld` do.
- New UI strings (hints, menu entries, aria-live text) go through `wsT` in
  every `internal/wsi18n/tables/*.json`. Non-English entries are worker
  translations: flag them unchecked.
- Do not touch the pinned-column / sticky-header files MAIN's
  `fix/wb-pin-empty-sticky` is editing until that branch merges.

## Phases

### Phase 1: reliable reorder + hover rename/copy (no tree, local order kept)

Decisions 1, 2, 10, 13. This fixes the reported "drag does not take" and ships
the pencil/clipboard. Order is still local in this phase.

Verification:
- node tests for the press/swap rules (a swap during the press is held, and
  a replaced node is re-found).
- Playwright: a grip drag reorders with a rail swap injected mid-press; a
  press on the row body navigates and never drags; a refused cross-group
  drop shows the hint. The pencil renames through `/s/<rid>/label` (the
  name updates in the rail and, for the viewed session, in the head), and
  a swap during the edit does not drop typing. The clipboard copies the
  same string the head name copies.

### Phase 2: server-side layout store and route

Decisions 3, 4. The server orders rows from the store, there is a one-time
import of the local order, and `sessionOrder` is retired.

Verification:
- Go tests: store round trip and `schema_version`; re-read before write
  (two writers both kept); op validation (depth, workspace, self, ended);
  a layout entry for a rid that no longer exists is dropped on read.
- Playwright with two browser contexts: a reorder in one shows in the other
  after its rail swap.

### Phase 3: the tree

Decisions 5-9. Rendering, the three-zone drop, attach/detach, the
collapse roll-up, ended/forgotten rules, stars vs tree.

Verification:
- node tests for the zone/validity function (valid parent, a parent with
  children dragged, cross-workspace, ended target, child zone absent on a
  child row).
- Go view tests: a child row has no message line and has its branch tag
  when its worktree differs; a collapsed parent carries a child's approval
  badge.
- Playwright: attach EXEC and UI under 메인 by middle drop, reorder the
  children, detach one by a top-level line, collapse 메인 while a child
  waits on approval (the badge shows on 메인). Screenshots at the default
  rail width and the overlay rail.

### Phase 4: context menu and keyboard path

Decisions 11, 12.

Verification:
- Playwright: right-click a non-hosted row (rename/copy/move under/detach);
  right-click a hosted row other than the viewed one, change its effort, and
  check that `/agents/<that rid>/settings` was hit, not the viewed rid's.
  Long-press under touch emulation opens the menu, and the grip drags.
  Shift+F10 opens it from the keyboard; Alt+arrows reorder, attach and
  detach with an aria-live announcement.

## Owner questions

All settled by the owner (2026-11-13 14:35Z): Q1 = (538) ok, Q2 = (539)=a,
Q3 = (540)=a. The questions are kept below for the record.


- [OWNER Q1] Stars and the workspace order: keep them in localStorage, or
  move them into the server layout with the tree? Rec: move both, so the
  rail looks the same in every browser. A tree shared across browsers with
  local stars would order units differently per browser.
- [OWNER Q2] A parent ends while children live: (a) the children show at
  the top level in its slot and re-nest on revive (rec); (b) the ended
  parent stays in the live list as a dimmed group head while it has live
  children.
- [OWNER Q3] Starring a child: (a) not allowed while it is a child; its
  star is kept inert and returns on detach (rec); (b) allowed, with
  starred children first among their siblings.
