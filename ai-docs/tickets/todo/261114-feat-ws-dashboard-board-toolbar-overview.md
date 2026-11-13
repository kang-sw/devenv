---
title: "Dashboard board: a toolbar card (overview, new terminal, search, files) and a folded single-column overview mode"
blocked-by: 261113-feat-ws-dashboard-board-reorder-hold#phase1
related:
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite for Phases 2-3 (blocked-by #phase1): its wbHistory.keep is the one scroll writer per scroller (its decision 8) that the overview's anchor and landing go through, and its scripts/board-headless.mjs takes this ticket's scenarios. The toolbar sits outside the hold; overview freezes order and, once its Phase 2 lands, defers hold releases"
  261113-feat-ws-whiteboard-topic-history-load-more: "sibling consumer of the same keep helper (also blocked-by reorder-hold#phase1); a history collapse while a topic is folded in overview needs no rule beyond the one-session-per-scroller re-anchor"
  261113-feat-ws-whiteboard-board-frontend-performance: "concurrent board asset edits; lazy srcdoc and viewport mount/unmount mean folded bodies may unmount frames and remount them on exit (the exit landing absorbs late heights through keep); overview never clones or force-mounts a frame. Either may land first"
  261113-feat-ws-dashboard-iframe-shell: "landed (0586a4289): session pages are shell frames. The darkroom reaches the shell's rail and header through the existing layout.dim field, which also passes rail clicks through to the frame; the Esc forwarding and search message shape the key rules here"
  261114-feat-ws-dashboard-unified-search-palette: "the toolbar's search button opens the palette through the same entry the shell's search message uses (#tkqs); the palette rework must keep that entry. Overview stays open under the palette. Either may land first"
  261114-refactor-ws-split-oversized-source-files: "dashboard.js is on its list; overview logic goes to a new assets/overview.js instead of growing it (ODQ (577))"
  261112-feat-ws-dashboard-pin-column: "built the pinned column and pin list this ticket moves into the stack in overview; (447) pins never reorder still holds"
  261110-feat-ws-dashboard-terminal: "built the new-terminal button and folder popover this ticket moves into the toolbar"
---

# Dashboard board: a toolbar card (overview, new terminal, search, files) and a folded single-column overview mode

## Background

Owner ideas and answers relayed by MAIN (2026-11-13), with UX mockups the
owner approved, and UX technical calls (UX lead, 2026-11-13). The board needs
two things:

- A toolbar card at the head of the board: one row of board-level actions.
- An overview mode that folds every card at once, so the whole board can be
  scanned in one column and a card picked.

Approved visual references, archived in this ticket's tracked assets folder:

- **Final overview (v3):**
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-board-toolbar-overview/board-overview-v3-mockup.html`.
  Its toolbar row matches the toolbar mockup's.
- **Toolbar card:**
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-board-toolbar-overview/board-toolbar-overview-mockup.html`.
  Only its toolbar card is current. Its overview (a tile grid) is superseded
  by v3.
- `board-overview-v2-mockup.html` was a comparison step and is not archived.
  Its difference from v3 is recorded under (566) below.

Where a mockup and the Decisions differ, the Decisions win. The mockups model
neither the real topic markup nor the iframe shell nor board swaps.

Owner's sign-off on v3, (565)=ok, verbatim:

> 565=ok^네, 이 정도면 만족합니다. 고정 패널들이 지금 목업에서는 약간 부정확한
> 위치로 날아오는데, 이거는 구현에서 고치기로 하고, 이 목업으로 확정하고 구현
> 들어가기로 해요

Gist: satisfied; the pinned panels fly to slightly wrong positions in the
mockup, which the implementation fixes; v3 is final and implementation starts.

### What exists today (develop 37bcfb5fb)

- **Card framework.** `assets/cards.js` places cards and topics in the board
  region.
  - `fixedNode` (cards.js:91) names what a board update never moves: a card
    and the pin list.
  - `split` (cards.js:209-213) returns the board's node order. When pins
    stand atop the board, the pin list comes first:
    `board: colOn ? out : [pins].concat(out)`.
  - `mount` inserts the pin list as the board's first child
    (cards.js:864).
  - `relayout` (cards.js:270) flips `colOn` and calls the page's `layout`,
    which toggles `data-pincol` and so the board's width.
  - `persist` (cards.js:332) stores cards, pinned and hand-moved topics, and
    the board width in localStorage.
- **The page mounts the framework** on the board region of session pages
  only: `cardBoard = mode !== "index" && ...` (dashboard.js:624), with the
  pinned column `#bpcol` created beside the board and passed as
  `column` (dashboard.js:640). The rest of the board path:
  - `swap` holds a board update while a card is dragged (`boardDragHeld`,
    dashboard.js:509) and lands it at the drag's end.
  - `restoreScroll` (dashboard.js:194) writes absolute scroll positions back.
  - `reducedMotion()` (dashboard.js:917) reads `prefers-reduced-motion`.
- **New-terminal entry point.** `WsTerm.mount($("bcol"))`
  (dashboard.js:669) builds an icon button and its folder popover:
  - The popover offers the session worktree, the repository root, or a typed
    path.
  - The button goes into `#tslot` at the end of the composer's status line
    (dashboard.tmpl:22, `{{if .Terminal}}<span class="tslot" id="tslot">`).
  - On a page without a composer (board mode, no adapter), it goes into a
    fixed corner slot `.tslot.tfix` (terminal.js:554-556,
    dashboard.css:712-713).
  - The `.cfoot` hide rule special-cases the slot (dashboard.css:554).
  - The page test pins all of this (terminal_page_test.go:81-108).
  - The button moved there from a board header row in 4bced7409 (see
    Prior Decisions).
- **Recap row.** A topic is `section.topic > .tophead > .tbar` plus an
  optional `.tsub` that holds the latest lead log as `.trecap .rtext` and the
  check pill (wswhiteboard page.tmpl:42; recap template page.tmpl:34).
  - A stuck head already clamps that recap to two lines
    (wswhiteboard page.css:711), so the clamp rule exists.
  - A closed topic renders without `.tsub`.
  - The closed-topic control is `button.closed-more[data-closed-next]`
    (page.tmpl:23). Its "N more" count exists only in localized text.
- **Pinned column and pin list.**
  - `#bpins` sits in `#bpcol` while the column shows (`.bpcol > .bpins`,
    dashboard.css:1494). Otherwise it is atop the board, with a "Pinned"
    label and a dashed separator (`.r-board > .bpins`, `::before`,
    dashboard.css:1492-1493).
  - The column itself has no label. 6b38ce4a4 dropped its count header.
- **Empty board.** The placeholder rule assumes the pin list is the board's
  first child: `.r-board > .bpins:empty:first-child + .empty:last-child`
  (dashboard.css:1489-1490, from 9b2ab99bb).
  - A toolbar first child breaks it.
  - UX's empty-board render fix keys on "no topic" instead:
    `.r-board:not(:has(> .topic)) > .empty`, with a comment that names a
    future toolbar card. That fix exists only as uncommitted work on
    `fix/ux-empty-board-render` (no commits past develop), so it has not
    landed.
- **Iframe shell** (landed in 0586a4289). The shell owns the rail and the
  header. A framed page reports `layout {pincol, dim}`. `dim()` sums the
  backdrop alpha of open modals and overlays (dashboard.js:7555-7575).
  - While `dim > 0`, the shell's header and rail take
    `filter: brightness(var(--fdim))` and `pointer-events: none`, so clicks
    fall through to the frame (dashboard.css:1891-1892).
  - The frame forwards an Escape it did not prevent to the shell as
    `key esc` (dashboard.js:7607).
  - The shell's `search` message clicks `#tkqs` (dashboard.js:7621). The
    header's search button runs `shSearch` (dashboard.js:7518).
- **Files.** The file browser (files.js) is the conversation column's Files
  tab, mounted only on live (adapter) pages. `setCtab("files")`
  (dashboard.js:341) shows it.
- **Flash.** A card's `show()` flashes `.bcard.flash .bc-in`
  (cards.js:460-471, dashboard.css:646-647, off under reduced motion at
  dashboard.css:655). Topics have no flash.
- **Reorder-hold Phase 1** (`wbHistory.watch/seen/keep/keeping` in
  wswhiteboard stream.js, and `scripts/board-headless.mjs` with an isolated
  `serve` daemon) is implemented on `impl/develop/perm-unlit-icon`
  (14bb9a454, 7bb5b42f8) and not merged yet.

## Decisions

1. **(552)=ok, owner 2026-11-13: concept.** A board toolbar card plus an
   overview mode.
2. **Toolbar card placement (UX lead, 2026-11-13).**
   - The toolbar is a `fixedNode` in cards.js and always the board region's
     first child: before the pin list when pins stand atop the board (narrow),
     and before everything else otherwise. `split` returns it first in both
     layouts.
   - It is not sticky and scrolls away with the board. It is not a Card: it
     has no pin, fold, maximize, close, or drag, it is never a drop peer or a
     drop target, it carries no `data-rail` (the topic rail ignores it), and
     it is never persisted.
   - It sits outside reorder-hold. It is never a hold candidate, a moving
     node, or a compensation anchor (that ticket anchors on topics and cards
     only).
   - Session pages only: it mounts where cards.js mounts (`cardBoard`, live
     and board modes). It does not appear on the index or the standalone
     whiteboard page.
3. **(555)=a, owner 2026-11-13: buttons.**
   - The buttons, in order: 한 눈에 보기 (Overview) / 새 터미널▾ (New
     terminal, with its folder popover) / 검색 (Search, with a `Ctrl K` hint)
     / 파일 (Files).
   - A right-aligned count follows the buttons, as both mockups show (see
     ODQ (585)).
   - More buttons are added on demand later, not in this ticket.
   - On a narrow board, the labels collapse to icons except Overview's, and
     the key hint hides (v3 `.page.n` rules). The collapse keys on the
     toolbar's own width (a container query), because the board column's
     width depends on the rail and the pinned column, not the viewport.
4. **(556)=a, owner 2026-11-13: one new-terminal entry point.**
   - 새 터미널 moves into the toolbar. The composer slot (`#tslot` in
     dashboard.tmpl:22) and the fixed corner fallback (`.tslot.tfix`) are
     removed, along with their CSS and the `.cfoot` special case.
   - The popover keeps its options (session worktree, repository root, typed
     path), its error handling, and its keyboard behaviour. It opens below
     the button.
   - The drawer's holder list keeps its "attach" for existing terminals. That
     is not a new-terminal entry point.
   - Rejected: keeping both entry points.
5. **(553)=a, owner 2026-11-13: a folded topic shows its existing recap
   row** (the latest lead log in `.tsub > .trecap`), clamped to two lines by
   the stuck-head clamp rule. A topic without a recap row folds to its head
   row alone. Rejected alternatives were not relayed.
6. **Overview view (owner direction, 2026-11-13; v3).**
   - The board stays one column, with no grid. Rejected: the toolbar
     mockup's tile grid (auto-fill columns on wide boards, a 3-line recap,
     and cards and closed topics hidden).
   - A topic folds to its head row plus the recap clamped to two lines. A
     terminal or file card folds to its header (title row) only. The closed
     section keeps its closed topics' head rows and its show-more control.
   - The topic you were viewing on entry stays at the same screen height and
     gets a left accent bar (v3 `.here`), unless the toolbar was in view.
   - Darkroom:
     - The page background, the rail, the header, the composer, the
       conversation column and tab bar, and the pinned label dim.
     - The toolbar and the card stack stay bright.
     - The board column's width does not change, so no line reflows.
   - Head actions (pin button, note pen) fade out. A pinned item shows a pin
     mark (see ODQ (581)).
7. **(557)=a, owner 2026-11-13: the pinned column empties in overview.** On a
   board that shows the pinned column, the pins fly out of it into the stack,
   directly under the toolbar.
   - The column keeps its place, so the board's width is unchanged.
   - On a narrow board, the pins already stand atop the stack. They fold in
     place and nothing flies.
   - Rejected: keeping the pinned column visible in overview.
8. **(566)=a, with the owner's motion note: one concurrent motion.**
   - The fold, the slot opening under the toolbar, and the pins' fly-in are
     ONE motion: about 240 ms, one easing curve (v3:
     `cubic-bezier(.2,.8,.2,1)`), started together and ended together, with
     no stagger.
   - The exit is the exact reverse: unfold, the slot closing, and the pins
     flying back to the column while they unfold.
   - Where a pin's start or landing rect is off-screen, that pin fades over
     the same 240 ms instead of flying (v3).
   - The pins land exactly in their stack slot ((565)): at the motion's end
     each flying pin's rect equals its real node's rect within 1 px, and the
     real node shows with no jump.
   - Rejected: v2's two-step motion (fold first, then the pins fly in and
     push the rest down), and fade-only pins (the v3 toggle).
9. **Motion scope (UX lead, 2026-11-13).**
   - Only near-viewport cards animate: those within one viewport height of
     the visible area. The rest switch at once, and the anchor keeps the view
     still.
   - Under `prefers-reduced-motion`, enter and exit switch instantly, with
     the same end states.
10. **Exit and landing (owner direction, 2026-11-13; v3).**
    - Esc, an outside click, or the Overview button again exits back to where
      you were. The topic that was anchored on entry returns to its entry
      offset, which restores the scroll position.
    - A card click unfolds the board and lands that card at the top of the
      board scroller, with a flash.
    - Clicking a pinned item (wide board) sends it back to the pinned column
      with the others. The column scrolls to it, and it flashes there.
    - ↑/↓ move focus between stack items (roving focus; the toolbar is not a
      stack item), Enter acts as a click on the focused item, and Esc exits.
      Focus starts on the anchored item.
    - Clicks during the motion are swallowed.
    - All landing scroll writes go through `wbHistory.keep` (reorder-hold
      decision 8), not raw `scrollTop` or `scrollIntoView` writes, so a
      running keep session never reads them as a user scroll. The keep
      session then keeps the landed card in place while unfolded frames
      report late heights.
11. **(567)=a, owner 2026-11-13: an outside click only exits.**
    - The click is captured and swallowed, so the control under it (a rail
      row, a composer button, a tab) does not fire.
    - Under the shell, a click on the shell's rail or header reaches the
      frame, because the darkroom dim sets `pointer-events: none` there
      (ODQ (578)).
    - Clicks inside the toolbar, an open popover, or the palette are not
      outside clicks.
    - Rejected: letting the click both exit and reach the control under it.
12. **Toolbar actions in overview (UX lead, 2026-11-13; v3).**
    - Search opens the palette and keeps overview open.
    - New terminal opens its popover in place. Picking a folder exits
      overview first, then opens the terminal card. The card enters at the
      head of the unpinned flow, under the toolbar and pins.
    - Files exits overview first, then shows the Files tab.
13. **Card order is frozen while in overview (UX lead, 2026-11-13).**
    - No drag starts in overview.
    - No reorder-hold release moves a topic in overview (once that ticket's
      Phase 2 has landed, overview defers releases as `pageHidden()` does).
    - Board updates follow ODQ (576).
14. **Overview state lives in sessionStorage (UX lead, 2026-11-13).** No
    overview state goes into localStorage or cards.js `persist()`. Key shape
    and reload behaviour: ODQ (586).
15. **Empty board tolerates the toolbar (UX lead, 2026-11-13).**
    - A board with no topic still renders the full dashed placeholder (full
      column height in the live layout) when the toolbar precedes it.
    - The empty state is decided by the absence of topics, never by child
      position.
    - If UX's `fix/ux-empty-board-render` rule
      (`.r-board:not(:has(> .topic)) > .empty`) has landed when Phase 1
      starts, Phase 1 verifies it with the toolbar present. If it has not,
      Phase 1 replaces the `:first-child` form at dashboard.css:1489-1490
      with that "no topic" form.
16. **Compatibility tier: T0** (checked against AGENTS.md Compatibility
    Tiers). Evidence:
    - The touched surfaces are dashboard assets (cards.js, terminal.js, a new
      overview.js, dashboard.js, dashboard.css, dashboard.tmpl), the asset
      embed list in pages.go, new `js.board.*` keys in the wsi18n tables,
      and, under ODQ (585), one additive HTML attribute in wswhiteboard
      page.tmpl.
    - The shell-frame protocol is unchanged: the existing `layout.dim` field
      carries the darkroom (ODQ (578)), with no new message kind or field.
      Shell and frame are served together from one asset set in any case.
    - Browser storage: one new per-session sessionStorage key. The cards.js
      localStorage record is unchanged.
    - No MCP tool or argument, `/mcp` hello, shared store on-disk shape,
      tracker event type, or config/rsrc manifest field changes.

## Open Decision Queue

Last ID: (589)

(576) [open] [technical] While overview is open, board updates are held and land after the exit's motion ends, as `boardDragHeld` holds them during a drag.
Held swaps over applying swaps with a frozen order. A morph during overview would insert unfolded new topics and change rows under the user's eye mid-scan, and a frozen-order morph needs the placement override that reorder-hold Phase 2 owns. The hold path already exists (dashboard.js:509). On exit, the landed swap gets reorder-hold's compensation like any other swap.

(577) [open] [technical] The overview's state, interaction, and motion live in a new asset `assets/overview.js` (window.WsOverview). It is embedded and served like shell.js. cards.js gains only the toolbar node and the overview placement.
A new module over growing dashboard.js. dashboard.js is on the oversized-file list (261114-refactor-ws-split-oversized-source-files, about 7.6k lines), and overview is one bounded module with a narrow page interface (board, scroller, pin list, keep helper, toolbar).

(578) [open] [technical] The darkroom reaches the shell's rail and header through the existing `layout.dim` field. The framed page's `dim()` adds an alpha of 0.72 while overview is open (shell brightness 0.28, v3's factor). The shell's `[data-fdim]` filter gets a 240 ms transition on the overview curve.
The existing field over a new shell message kind. The shell already dims its rail and header by `--fdim` and lets clicks through to the frame (dashboard.css:1891-1892), which is also what makes (567)=a hold for rail and header clicks. No protocol change.

(579) [open] [technical] On a board showing the pinned column, overview moves the whole pin list `#bpins` into the board right after the toolbar (one move, `moveBefore` where available) and moves it back on exit. `data-pincol` and the column stay as they are.
Moving the list over moving each pinned node or flipping `colOn`. Flipping `colOn` runs the page's `layout` hook, which toggles `data-pincol` and widens the board, so every line would reflow. Moving the list is one node move. Without `moveBefore`, pinned body frames reload while folded, and their heights are restored through the `setFrameHeights` path.

(580) [open] [technical] Each flying pin is a clone of only its folded parts (a topic's `.tophead`, a card's `.bc-hd`) over a plain body box that collapses to 0 in the same motion. Every frame, it interpolates from its start rect toward its real node's live rect with the eased progress.
Partial clones tracking the live rect over v3's whole-node clone flying to a rect computed at the start. A whole clone re-creates srcdoc frames and cannot clone an xterm or a running preview. A target computed at the start misses the slot's growth and the anchor's scroll compensation, which is the off landing the owner saw in (565). Tracking the live rect lands it exactly.

(581) [open] [technical] In overview, a click anywhere on a stack item, including its head controls (chips, ticket links, pin, note pen, bulk buttons), is the item's click (unfold and land). Head actions fade out, and a pinned item's pressed `.tpin` shows as a non-interactive pin mark.
One item click over letting head controls fire. In overview the stack is a picker; v3 fades the head actions and shows a pin mark.

(582) [open] [technical] The pin list's own label ("Pinned") and dashed separator: on a narrow board they stay and dim with the darkroom; on a wide board, the flown-in list shows neither, so the pins land directly under the toolbar as in v3.
This mapping over showing the label in both widths. The real pinned column has no label (6b38ce4a4 dropped its header), so the owner's "pinned label" maps to the narrow pin group's label. v3's wide stack has no label above the pins.

(583) [open] [technical] 파일 shows the conversation column's Files tab (`setCtab("files")`, switching to the conversation tab first where one column shows at a time) and focuses its filter. The button is absent on a page without the Files pane (board mode, no adapter). 새 터미널 is absent on a page without `data-term`.
Absent buttons over disabled ones. files.js is the only file browser and mounts only on live pages, and terminal.js registers nothing without `data-term`, so a disabled button would promise an action the page never has.

(584) [open] [technical] Under reduced motion, the landing flash becomes a static accent outline removed after 1.2 s. Topics get the same flash as cards.
A static outline over no cue, which is the existing card precedent (dashboard.css:655). The flash is the only cue for where the card landed, and reduced motion concerns movement, not a colour change.

(585) [open] [technical] The toolbar count reads "open N · closed M". N counts the open topics on the board and in the pin list. M is the closed topics shown plus the show-more remainder, read from a new additive `data-closed-more` attribute on the closed-more control (wswhiteboard page.tmpl:23). The count updates with each landed board swap.
An attribute over parsing the localized button text or dropping the closed count. Both mockups show the counts, and the remainder exists today only inside localized text.

(586) [open] [technical] The sessionStorage key is per session, `ws.dashboard.overview.<rid>`, holding the on state and the entry anchor (a topic name or card id, and its offset). A reload in overview opens folded with no motion, and exit restores the stored anchor (the board's top if the anchor is gone).
A per-session key over a page-global key. sessionStorage is shared by every same-origin frame in the tab, and the shell keeps up to five session frames loaded.

(587) [open] [technical] Esc closes only the topmost layer: the palette, then the terminal popover, then overview. The overview's Esc handler calls `preventDefault`, so the frame does not forward `esc` to the shell. The rail hotkey (Ctrl+`) exits overview first and then acts, as 새 터미널 and 파일 do.
Topmost-layer Esc over closing everything at once. An unprevented Escape is forwarded to the shell (dashboard.js:7607) and would also close the rail overlay. The rail is unusable while dimmed with `pointer-events: none`, so its hotkey exits first.

(588) [open] [technical] A palette pick that targets a board item while overview is open acts as a card click: exit, land, flash.
A card click over jumping inside the folded stack. The target's content is folded, and landing it unfolded is the card-click contract (Decision 10).

(589) [open] [technical] The light theme uses the same darkroom factors as dark (brightness 0.28, saturate 0.4) and the same darkened page background mix.
One set of factors over a light-specific variant. No light mockup exists, and the shell's modal dim already uses one factor in both themes. The light captures in Phase 3 go to the owner with the comparison.

(575) [open] [policy] Whether entering overview shows the hint toast "바깥 클릭·Esc: 원래 자리로 · 카드 클릭: 그 카드로" (outside click or Esc: back to where you were; card click: go to that card).
- Context: v3 shows this toast on every entry. v3's other toasts are stand-ins for real actions and say so ("(목업)", "mockup"). This one does not, so it reads as product behaviour. A toast on every entry repeats for a user who already knows the mode.
- (a) Never; the Overview button's title carries the hint.
- (b) On the first entry per browser session only (sessionStorage).
- (c) On every entry, as v3 does.
> Recommendation: (b). The hint teaches the two exits once, without repeating on every glance.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Never restart, rebuild, or swap the shared dashboard daemon (port 29417),
  and never run `scripts/dev.py rerun daemon` (it replaces the shared dev
  daemon). Live checks use a dev daemon instance with its own
  `WS_CACHE_HOME`, such as `node scripts/board-headless.mjs serve`, which
  builds into a temp directory with its own cache and config homes.
- UI strings go through `wsT` as `js.board.toolbar.*` and
  `js.board.overview.*` keys. Reorder-hold decision 7 opened the
  `js.board.` prefix.
  - Each key goes into all 11 tables in `internal/wsi18n/tables/*.json`.
    `keys_test.go` requires every English key that a script uses, present in
    every table, and used somewhere.
  - So removing the composer button must not orphan `js.term.new` /
    `js.term.new.title`: reuse them for the toolbar button or remove them
    from every table.
  - Non-English entries are worker translations, flagged as unchecked in the
    Result. The ko entries use the owner's labels (한 눈에 보기, 새 터미널,
    검색, 파일).
- The overview fold is a view state, never a card's own fold. It does not
  call `card.fold()` or `persist()` and changes no server markup for topics
  or cards. It is a root attribute plus CSS, with height transitions only on
  near-viewport nodes.
- A pinned item's pin order never changes in overview ((447)). The pin list
  leaves the column and returns with its order intact.
- A flying pin never contains an iframe, a terminal, or a preview body
  (ODQ (580)).
- Overview writes no `scrollTop` outside `wbHistory.keep` sessions on the
  board scroller (Decision 10). `restoreScroll`'s owned-scroller skip
  (reorder-hold decision 4) covers it.
- A maximized card exits overview before it maximizes, and overview cannot
  be entered while a card is maximized (the sheet's scrim already dims the
  page).

## Prior Decisions

- 4bced7409 (2026-11-12, commit): "The board column's own header row (.bhead) is gone; the button is a small square-terminal icon button in a slot at the right end of the composer's status line ... User feedback: the header row cost a whole row of vertical space at the top of the board column for one button." - bearing: superseded by (552)=ok, (555)=a and (556)=a. The new board-top row carries four actions and a count, and the composer slot goes.
- 261110-feat-ws-dashboard-terminal (Decisions 11): "The board column gains a one-row header, mockup-sized, holding \"+ Terminal\". Its popover picks the working folder ... Rejected: a button in the session top bar ((120) b)." - bearing: supports (the toolbar returns the button to the board's head).
- 261112-feat-ws-dashboard-pin-column ((447)): "The pinned column scrolls separately from the board column. When a pinned topic or card is updated it keeps its place ... it does not move." - bearing: constrains (pin order is untouched in overview).
- 6b38ce4a4 (2026-11-13, commit): "fill the empty board state, drop the pinned-count header" - bearing: constrains (ODQ (582): the column has no label to dim).
- 9b2ab99bb (2026-11-14, commit): "#bpins stays mounted (cards.js pins logic needs it), so added the `.bpins:empty:first-child + .empty:last-child` form" - bearing: superseded by Decision 15 (the empty state keys on no topic, not child position).
- 261113-feat-ws-dashboard-board-reorder-hold (decision 8): "Rejected: separate globals per consumer ... and handing out the raw IntersectionObserver" - bearing: constrains (overview uses the shared keep helper and adds no scroll writer of its own).

## Phases

### Phase 1: the toolbar card and the single new-terminal entry point

Decisions 2-4, 15; ODQ (583), (585). No overview behaviour yet: the
Overview button renders but does nothing until Phase 2 wires it (or it is
hidden behind a flag Phase 2 removes; worker's choice).

- cards.js: the toolbar node, built at `mount` before the pin list,
  becomes a `fixedNode`, and `split` returns it first in both pin layouts.
  It is excluded from `over()`'s flow peers, from `flowNext`, from
  `headTime`, and from drop targets. It has no `data-rail`.
- terminal.js: `button()` mounts into the toolbar's slot. The `#tslot`
  composer slot, the `.tslot.tfix` fallback, their CSS, and the `.cfoot`
  special case go. The popover opens below the button and still fits the
  viewport (`placePop`).
- Search: the toolbar button calls the same entry as the shell's search
  message (`$("tkqs")` click / `shSearch`).
- Files: ODQ (583).
- The count: ODQ (585).
- Empty board: Decision 15.
- The toolbar's look follows the toolbar mockup's card (v3's toolbar row is
  the same): the button row, the pressed state, and the narrow
  label collapse.
- i18n keys in all 11 tables.

Verification:
- Go tests (internal/wsdashboard), node harness via `runNode` in the
  cards_test.go style:
  - The toolbar is the board's first child with pins atop and with the
    pinned column shown.
  - A board morph (`weave` + `settle`), `relayout` across a width crossing, a
    pin, an unpin, and a drag drop never move it, and it is never a drop
    peer.
  - The empty-board placeholder selector matches with the toolbar present
    and an empty `#bpins` (CSS test in the terminal_page_test.go style).
- terminal_page_test.go updated: the composer has no `#tslot`, the fixed
  fallback is gone, and the toolbar slot is present on a `data-term` page
  and absent without it.
- `go test ./internal/wsi18n/...` passes.
- `go test ./internal/wsdashboard/... ./internal/wswhiteboard/...` passes.
- Headless, in `scripts/board-headless.mjs` (landed by reorder-hold Phase 1,
  this ticket's prerequisite; a new scenario `toolbar`, against
  `board-headless.mjs serve` pages):
  - On an adapter page and on a board page, the toolbar is first, above the
    pins at 420 px, and scrolls away with the board (not sticky).
  - The new-terminal popover opens from the toolbar, and picking the session
    worktree opens a terminal card at the head of the unpinned flow (on a
    page with `data-term`).
  - The composer shows no terminal button.
  - A topicless board shows the full-height placeholder.
  - Exits non-zero on any failure.
  - If reorder-hold Phase 1 has not landed (the blocked-by was dropped),
    create `scripts/board-toolbar-headless.mjs` on the same pattern instead
    (repo-local beside `scripts/dev.py`, not shipped, Playwright from the
    npx cache, no `package.json`, scenarios exit non-zero on failure, run
    against an isolated dev daemon). Later phases add scenarios to whichever
    file Phase 1 chose.
- LOOK: captures of the toolbar at 1400 and 420 px, light and dark, side by
  side with the toolbar row of
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-board-toolbar-overview/board-toolbar-overview-mockup.html`
  (and v3's identical row).

### Phase 2: overview mode without motion

Decisions 5-7 and 10-14; ODQ (576)-(579), (581)-(584), (586)-(588), (575).
This phase builds every end state and the reduced-motion path (Decision 9's
instant switch). Phase 3 adds the motion between those states.

- `assets/overview.js` (ODQ (577)), embedded and served like shell.js.
- Enter and exit from the Overview button.
- The root view state: the fold, the 2-line recap clamp, head actions faded,
  and the pin mark.
- The darkroom in the frame, and in the shell through `layout.dim`.
- The `.here` accent bar on the entry anchor.
- On a wide board, the pin list moves into the stack under the toolbar and
  back (ODQ (579)). The label follows ODQ (582).
- Order freeze: no drag, held board swaps (ODQ (576)), and deferred
  reorder-hold releases if its Phase 2 has landed.
- Outside click capture (Decision 11), card click landing, and a pinned
  item's return with a flash. ↑/↓/Enter/Esc (ODQ (587)).
- Toolbar actions per Decision 12, and the palette pick per ODQ (588).
- sessionStorage per ODQ (586). Landing scroll writes go through
  `wbHistory.keep`.

Verification:
- node unit tests (extracted from overview.js under `runNode`):
  - the anchor pick on entry (the first visible stack item; none when the
    toolbar is in view);
  - the exit target rules: no target returns to the entry anchor and offset;
    a card target lands at the top; a pinned target goes back to the column;
  - the outside-click classifier: toolbar, popover, palette, stack item, and
    outside, including a click on the frame's rail placeholder;
  - the Esc layer order;
  - the sessionStorage round trip, including a missing anchor.
- Headless scenarios `overview` (adapter page with `#bcol` scrolling, and a
  board page with the document scrolling; 1400 and 420 px):
  - Enter from mid-board: the anchored topic's screen y holds within 1 px.
    Every topic shows only its head row and at most two recap lines, and
    every card only its header.
  - On a wide board, the pins stand directly under the toolbar and
    `#bpcol` is empty, while the board column's width is unchanged
    (measured).
  - Exits by Esc, by an outside click on the composer, by an outside click
    on the shell's rail, and by the button: each restores the entry scroll
    within 1 px. The clicked composer control and rail row did not fire (no
    navigation, no focus change, no request).
  - Card click: that card's top lands at the scroller top and it carries the
    flash class. A pinned card click: it is back in `#bpcol`, in view, and
    flashed.
  - ↑/↓ move focus through the stack, and Enter on the third item lands it.
  - A board update injected during overview changes nothing until exit, then
    lands (ODQ (576)).
  - Search keeps overview open. Picking a terminal folder exits first and
    opens the card. 파일 exits first and shows the Files tab.
  - A reload in overview opens folded, and exit restores the stored anchor.
  - A second session frame in the same tab keeps its own overview state.
  - With reduced motion emulated, enter and exit switch in one frame.
- `go test ./internal/wsdashboard/... ./internal/wsi18n/...` passes.

### Phase 3: the one concurrent motion

Decisions 8-9; ODQ (580), (584), (589).

- Enter: within one ~240 ms motion on one curve, near-viewport topics and
  cards fold (height transitions), the slot under the toolbar opens from 0
  to the pin list's height, and each pin's clone flies from its column rect
  to its live slot rect while its body box collapses (ODQ (580)).
- The anchor is kept through the motion by re-anchoring the keep session
  each frame (a `wbHistory.keep` call on a running session re-anchors it).
- Exit: the exact reverse, with the landing offset eased toward its target
  on the same curve.
- A pin whose start or landing rect is off-screen fades instead of flying.
- Reduced motion keeps Phase 2's instant switch.
- The topic flash under reduced motion follows ODQ (584).

Verification:
- Headless scenario `overview-motion`, sampling every animation frame through
  enter and exit at 1400 px (pins in the column) and 420 px:
  - Every animated element starts within the same frame and ends within one
    frame of 240 ms (±1 frame), and no element starts late (no stagger).
  - The exit's per-frame positions mirror the enter's within 1 px at
    mirrored progress.
  - At the end of enter, each pin clone's rect equals its real node's rect
    within 1 px before the clone is removed, and the node's rect does not
    change in the frame after removal (exact landing, (565)).
  - The anchored topic's screen y holds within 1 px through enter.
  - Only near-viewport nodes carry a transition. Off-screen nodes switch in
    one frame.
  - With reduced motion emulated, no transition runs and the end states
    equal the animated run's.
  - The scenario exits non-zero on failure, with a per-frame trace.
- LOOK: captures at 1400 and 420 px, light and dark, of: normal view; overview
  at rest; mid-enter (about 120 ms); and after a card-click landing. They are
  placed side by side with the same states of
  `ai-docs/tickets/assets/261114-feat-ws-dashboard-board-toolbar-overview/board-overview-v3-mockup.html`
  (its "넓게 1240" (wide 1240) and "좁게 420" (narrow 420) views; the light
  captures against the dark mockup per ODQ (589)).
  - The stack, the fold depth, the 2-line recap, the darkroom levels, the
    toolbar staying bright, the pin landing slot, and the `.here` bar match.
  - The pin landing is exact where the mockup's is off ((565)).
  - Differences that the Decisions require (real topic markup, the shell's
    header dim, absent buttons) are listed in the Result.
- `go test ./internal/wsdashboard/...` passes.
