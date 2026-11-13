---
title: "Dashboard board: a toolbar card (overview, new terminal, web, search, files) and a folded single-column overview mode"
blocked-by: 261113-feat-ws-dashboard-board-reorder-hold#phase1
related:
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite for Phases 2-3 (blocked-by #phase1): its wbHistory.keep is the one scroll writer per scroller (its decision 8) that the overview's anchor and landing go through, and its scripts/board-headless.mjs takes this ticket's scenarios. The toolbar sits outside the hold; overview freezes order and, once its Phase 2 lands, defers hold releases"
  261113-feat-ws-whiteboard-topic-history-load-more: "sibling consumer of the same keep helper (also blocked-by reorder-hold#phase1); a history collapse while a topic is folded in overview needs no rule beyond the one-session-per-scroller re-anchor"
  261113-feat-ws-whiteboard-board-frontend-performance: "concurrent board asset edits; lazy srcdoc and viewport mount/unmount mean folded bodies may unmount frames and remount them on exit (the exit landing absorbs late heights through keep); overview never clones or force-mounts a frame. Its content-visibility: auto on section.topic is overridden on stack items while overview is open (Decision 32). Either may land first"
  261113-feat-ws-dashboard-browser-pane: "its \"+ Web\" entry button sits next to the new-terminal button in the composer slot this ticket removes; it becomes the toolbar's fifth button, right after 새 터미널 (Decision 41). Whichever lands second moves it"
  261113-feat-ws-dashboard-iframe-shell: "landed (0586a4289): session pages are shell frames. The darkroom reaches the shell's rail and header through the existing layout.dim field, which also passes rail clicks through to the frame; the Esc forwarding and search message shape the key rules here"
  261114-feat-ws-dashboard-unified-search-palette: "the toolbar's search button opens the palette through the same entry the shell's search message uses (#tkqs); the palette rework must keep that entry. Overview stays open under the palette. Either may land first"
  261114-refactor-ws-split-oversized-source-files: "dashboard.js is on its list; overview logic goes to a new assets/overview.js instead of growing it (Decision 18)"
  261112-feat-ws-dashboard-pin-column: "built the pinned column and pin list this ticket moves into the stack in overview; (447) pins never reorder still holds"
  261110-feat-ws-dashboard-terminal: "built the new-terminal button and folder popover this ticket moves into the toolbar"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 47904150870a43e9
sage-review-completeness-reviewed: 47904150870a43e9
---

# Dashboard board: a toolbar card (overview, new terminal, web, search, files) and a folded single-column overview mode

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
    (cards.js:863).
  - `relayout` (cards.js:270) flips `colOn` and calls the page's `layout`,
    which toggles `data-pincol` and so the board's width.
  - `persist` (cards.js:332) stores cards, pinned and hand-moved topics, and
    the board width in localStorage.
- **The page mounts the framework** on the board region of session pages
  only: `cardBoard = mode !== "index" && ...` (dashboard.js:624), with the
  pinned column `#bpcol` created beside the board and passed as
  `column` (dashboard.js:644). The rest of the board path:
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
    dashboard.css:1496). Otherwise it is atop the board, with a "Pinned"
    label and a dashed separator (`.r-board > .bpins`, `::before`,
    dashboard.css:1494-1495).
  - The column itself has no label. 6b38ce4a4 dropped its count header.
- **Empty board.** The placeholder rule assumes the pin list is the board's
  first child: `.r-board > .bpins:empty:first-child + .empty:last-child`
  (dashboard.css:1489-1490, from 9b2ab99bb).
  - A toolbar first child broke it.
  - UX's empty-board render fix landed after this snapshot (aa3c2634f,
    2026-11-14). It keys on "no topic and no card" instead of child
    position: `.r-board:not(:has(> .topic, > .bcard)) > .empty`, with a
    comment that names a future toolbar card. In the live layout the empty
    board is pinned to its column's padding box and the placeholder
    flex-fills it (dashboard.css, the empty-board block after `.bpcol`).
- **Iframe shell** (landed in 0586a4289). The shell owns the rail and the
  header. A framed page reports `layout {pincol, dim}`. `dim()` takes the
  maximum backdrop alpha of open modals and overlays (dashboard.js:7555-7568).
  The frame posts `layout` from a MutationObserver on the root that watches
  only `style` and `data-pincol` (dashboard.js:7582), or from an explicit
  `queue()`.
  - While `dim > 0`, the shell's header and rail take
    `filter: brightness(var(--fdim))` and `pointer-events: none`, so clicks
    fall through to the frame (dashboard.css:1893-1894).
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
     Decision 26).
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
     mark (see Decision 22).
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
      (Decision 19).
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
    - Board updates follow Decision 17.
14. **Overview state lives in sessionStorage (UX lead, 2026-11-13).** No
    overview state goes into localStorage or cards.js `persist()`. Key shape
    and reload behaviour: Decision 27.
15. **Empty board tolerates the toolbar (UX lead, 2026-11-13).**
    - A board with no topic still renders the full dashed placeholder (full
      column height in the live layout) when the toolbar precedes it.
    - The empty state is decided by the absence of topics and cards, never by
      child position.
    - UX's rule for this landed in aa3c2634f
      (`.r-board:not(:has(> .topic, > .bcard)) > .empty`). Phase 1 keeps it
      and verifies it with the toolbar present: the toolbar sits at the top
      of the filled board and the placeholder fills the rest of the column,
      with no column overflow.
16. **Compatibility tier: T0** (checked against AGENTS.md Compatibility
    Tiers). Evidence:
    - The touched surfaces are dashboard assets (cards.js, terminal.js, a new
      overview.js, dashboard.js, dashboard.css, dashboard.tmpl), the asset
      embed list in pages.go, new `js.board.*` keys in the wsi18n tables,
      and, under Decision 26, one additive HTML attribute in wswhiteboard
      page.tmpl.
    - The shell-frame protocol is unchanged: the existing `layout.dim` field
      carries the darkroom (Decision 19), with no new message kind or field.
      Shell and frame are served together from one asset set in any case.
    - Browser storage: one new per-session sessionStorage key. The cards.js
      localStorage record is unchanged.
    - No MCP tool or argument, `/mcp` hello, shared store on-disk shape,
      tracker event type, or config/rsrc manifest field changes.

17. **(576), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): board updates are held during overview.** While
    overview is open, board updates are held and land after the exit's
    motion ends, as `boardDragHeld` holds them during a drag
    (dashboard.js:509). On exit, the landed swap gets reorder-hold's
    compensation like any other swap.
    - Rejected: applying swaps with a frozen order. A morph during overview
      inserts unfolded new topics and changes rows under the user's eye
      mid-scan, and a frozen-order morph needs the placement override that
      reorder-hold Phase 2 owns.
18. **(577), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): a new asset `assets/overview.js`** (`window.WsOverview`)
    holds the overview's state, interaction, and motion. It is embedded and
    served like shell.js. cards.js gains only the toolbar node and the
    overview placement.
    - Rejected: growing dashboard.js. It is on the oversized-file list
      (261114-refactor-ws-split-oversized-source-files, about 7.6k lines),
      and overview is one bounded module with a narrow page interface
      (board, scroller, pin list, keep helper, toolbar).
19. **(578), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): the darkroom reaches the shell through `layout.dim`.**
    While overview is open, the framed page's `dim()` counts an overview
    backdrop alpha of 0.72 among the open backdrops it takes the maximum of
    (shell brightness 0.28, v3's factor). Wiring: Decision 36. The shell's `[data-fdim]` filter
    gets a 240 ms transition on the overview curve.
    - Rejected: a new shell message kind. The shell already dims its rail and
      header by `--fdim` and lets clicks through to the frame
      (dashboard.css:1893-1894), which is also what makes Decision 11 hold
      for rail and header clicks. No protocol change.
20. **(579), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): the pin list moves as one node.** On a board showing the
    pinned column, overview moves the whole pin list `#bpins` into the board
    right after the toolbar (one move, `moveBefore` where available) and
    moves it back on exit. `data-pincol` and the column stay as they are.
    Without `moveBefore`, pinned body frames reload while folded, and their
    heights are restored through the `setFrameHeights` path.
    - Rejected: moving each pinned node, or flipping `colOn`. Flipping
      `colOn` runs the page's `layout` hook, which toggles `data-pincol` and
      widens the board, so every line would reflow.
21. **(580), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): flying pins are partial clones that track the live
    rect.** Each flying pin clones only its folded parts (a topic's
    `.tophead`, a card's `.bc-hd`) over a plain body box that collapses to 0
    in the same motion. Every frame, it interpolates from its start rect
    toward its real node's live rect with the eased progress.
    - Rejected: v3's whole-node clone flying to a rect computed at the start.
      A whole clone re-creates srcdoc frames and cannot clone an xterm or a
      running preview. A target computed at the start misses the slot's
      growth and the anchor's scroll compensation, which is the off landing
      the owner saw in (565).
22. **(581), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): in overview, a click anywhere on a stack item is the
    item's click** (unfold and land), including its head controls (chips,
    ticket links, pin, note pen, bulk buttons). Head actions fade out, and a
    pinned item's pressed `.tpin` shows as a non-interactive pin mark.
    - Rejected: letting head controls fire. In overview the stack is a
      picker; v3 fades the head actions and shows a pin mark.
23. **(582), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): the pin list's label.** Its "Pinned" label and dashed
    separator stay and dim with the darkroom on a narrow board. On a wide
    board, the flown-in list shows neither, so the pins land directly under
    the toolbar as in v3.
    - Rejected: the label in both widths. The real pinned column has no label
      (6b38ce4a4 dropped its header), so the owner's "pinned label" maps to
      the narrow pin group's label, and v3's wide stack has no label above
      the pins.
24. **(583), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): Files and New terminal are absent where they cannot
    act.** 파일 shows the conversation column's Files tab
    (`setCtab("files")`, switching to the conversation tab first where one
    column shows at a time) and focuses its filter. It is absent on a page
    without the Files pane (board mode, no adapter). 새 터미널 is absent on a
    page without `data-term`.
    - Rejected: disabled buttons. files.js is the only file browser and
      mounts only on live pages, and terminal.js registers nothing without
      `data-term`, so a disabled button would promise an action the page
      never has.
25. **(584), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): the reduced-motion landing cue.** Under reduced motion,
    the landing flash becomes a static accent outline removed after 1.2 s.
    Topics get the same flash as cards.
    - Rejected: no cue. The flash is the only cue for where the card landed,
      reduced motion concerns movement rather than a colour change, and the
      card precedent exists (dashboard.css:655).
26. **(585), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): the toolbar count reads "open N · closed M".** N counts
    the open topics on the board and in the pin list. M is the closed topics
    shown plus the show-more remainder, read from a new additive
    `data-closed-more` attribute on the closed-more control (wswhiteboard
    page.tmpl:23). The count updates with each landed board swap.
    - Rejected: parsing the localized button text, or dropping the closed
      count. Both mockups show the counts, and the remainder exists today
      only inside localized text.
27. **(586), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): a per-session sessionStorage key**,
    `ws.dashboard.overview.<rid>`, holds the on state and the entry anchor
    (a topic name or card id, and its offset). A reload in overview opens
    folded with no motion, and exit restores the stored anchor (the board's
    top if the anchor is gone).
    - Rejected: a page-global key. sessionStorage is shared by every
      same-origin frame in the tab, and the shell keeps up to five session
      frames loaded.
28. **(587), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): Esc closes only the topmost layer**: the palette, then
    the terminal popover, then overview. The overview's Esc handler calls
    `preventDefault`, so the frame does not forward `esc` to the shell. The
    rail hotkey (Ctrl+`) exits overview first and then acts, as 새 터미널 and
    파일 do.
    - Rejected: closing everything at once. An unprevented Escape is
      forwarded to the shell (dashboard.js:7607) and would also close the
      rail overlay. The rail is unusable while dimmed with
      `pointer-events: none`, so its hotkey exits first.
29. **(588), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): a palette pick that targets a board item while overview
    is open acts as a card click**: exit, land, flash.
    - Rejected: jumping inside the folded stack. The target's content is
      folded, and landing it unfolded is the card-click contract
      (Decision 10).
30. **(589), UX technical call, owner-acknowledged 2026-11-14 ((590)=ok): the light theme uses the dark theme's darkroom factors**
    (brightness 0.28, saturate 0.4) and the same darkened page background
    mix. The light captures in Phase 3 go to the owner with the comparison.
    - Rejected: a light-specific variant. No light mockup exists, and the
      shell's modal dim already uses one factor in both themes.
31. **(575)=b, owner 2026-11-14: the entry hint toast shows on the first
    entry per browser session only.** The toast reads "바깥 클릭·Esc: 원래
    자리로 · 카드 클릭: 그 카드로" (outside click or Esc: back to where you
    were; card click: go to that card). One tab-wide sessionStorage flag,
    not per dashboard session, records that it was shown.
    - Rejected: (a) never, with the Overview button's title carrying the
      hint; (c) on every entry, as v3 does, which repeats for a user who
      already knows the mode.

Decisions 32-40 settle the design review's autonomous issues (UX technical
calls, 2026-11-14, auto-confirmed under the owner's overnight delegation).

32. **Stack items render while off-screen.** While overview is open, every
    stack item (topics, cards, the moved pin list's items) takes
    `content-visibility: visible`, overriding any `content-visibility: auto`
    that 261113-feat-ws-whiteboard-board-frontend-performance puts on
    `section.topic`. Folded bodies are hidden, so this is cheap.
    - Rejected: relying on `contain-intrinsic-size`. A skipped topic keeps its
      last remembered (unfolded) or 600 px size, so the off-screen stack would
      not be compact and scanning would jump as each topic renders.
33. **A bottom spacer keeps the anchor reachable.** On entry, overview adds an
    overview-only spacer at the end of the board, sized so the entry anchor's
    offset is reachable after the fold (the folded content below the anchor
    plus the spacer fills the viewport space below the offset). On exit, the
    spacer is removed inside the same keep session, after the unfold, so the
    restore is not clamped either.
    - Rejected: letting the anchor drop near the board's end. Folding shrinks
      the content below the anchor, the needed `scrollTop` exceeds the maximum
      and is clamped, and the anchor slides for entries from roughly the last
      dozen topics.
34. **The pinned column's scroll position survives overview.** On entry,
    overview records `#bpins`'s `scrollTop` in the column. On exit, after the
    list is back in `.bpcol`, it restores that value through a keep session
    on `#bpins`. The pins' fly-back targets (Decision 8) are the rects at that
    restored scroll position. A pinned-item click (Decision 10) scrolls the
    column to the item instead.
35. **The pin list stays in the board while overview is open.** cards.js
    `place()` keeps `#bpins` in the board, right after the toolbar, while
    overview is open, whatever `colOn` says. A `relayout` during overview (the
    1600 px PIN_WIDE crossing, a layout switch) still applies `colOn` and
    `data-pincol`, and an `arrange()` from card events still runs. Only the
    pin list's parent is held. On exit, the list goes where `place()` puts
    it for the current `colOn`.
    - Rejected: exiting overview on every relayout or arrange. A card event
      or a window resize would end the user's scan unasked.
36. **`layout.dim` wiring.** Entering and leaving overview calls the frame's
    `queue()` to post `layout` (the root observer does not watch the overview
    attribute). The shell's filter applies brightness only, so the shell's
    rail and header get brightness 0.28 without Decision 30's saturate 0.4.
    That is an expected LOOK difference, listed in the Result.
    - Rejected: adding a saturate field to `layout`. That is a shell protocol
      change, for a small difference on a dimmed region.
37. **A palette board pick in overview always exits first.** Both board picks
    of 261114-feat-ws-dashboard-unified-search-palette exit overview and land
    the target as a card click does (Decision 29). That covers Enter (the
    palette closes) and Ctrl+Enter (the palette stays open, and the target
    lands and flashes behind it). The board-pick path routes through
    `WsOverview` when overview is open, and a test fails if it stops doing so,
    whichever ticket lands second.
38. **Exit with no anchor keeps the toolbar's position.** When the toolbar
    was in view on entry (no anchor, Decision 6), exit keeps the toolbar at
    its screen y through overview's own keep session anchored on the toolbar,
    so the board top stays where it was. This is not a reorder-hold
    compensation anchor; Decision 2 still holds for that ticket.
39. **Every card kind folds to its header.** Terminal, file, preview, and
    web (261113-feat-ws-dashboard-browser-pane) cards all fold to `.bc-hd` in
    overview. Decision 6's "terminal or file card" is the cards that existed
    when it was written.
40. **The corner slot's leftovers go with it.** Removing `.tslot.tfix`
    (Decision 4) also removes the right margin that 4bced7409 gave the board's
    sticky copy-answers bar to clear the corner button
    (`body:has(> .tfix) .r-board .reply`).
41. **(592)=ok, owner 2026-11-14: the web entry is the toolbar's fifth
    button.** Browser-pane's "+ Web" entry
    (261113-feat-ws-dashboard-browser-pane, its Decision 9, "next to + Terminal")
    becomes the toolbar button 웹 (Web), placed right after 새 터미널 ("fifth"
    counts buttons, it is not a position): 한 눈에 보기 / 새 터미널 / 웹 / 검색 /
    파일. It keeps its own
    URL or workspace-path input popover, which opens below the button. It is
    absent on a page where a web card cannot open, by Decision 24's rule. On a
    narrow board it collapses to an icon with the others (Decision 3). This is
    the on-demand addition that Decision 3's "more buttons are added on demand
    later" anticipates, for an entry point the owner already approved.
    - Rejected: (a) folding it into the 새 터미널 popover, which mixes two card
      kinds under a terminal label; (b) keeping it in the composer slot, which
      Decision 4 removes.

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
  (Decision 21).
- Overview writes no `scrollTop` outside `wbHistory.keep` sessions on the
  board scroller (Decision 10). `restoreScroll`'s owned-scroller skip
  (reorder-hold decision 4) covers it.
- A maximized card exits overview before it maximizes, and overview cannot
  be entered while a card is maximized (the sheet's scrim already dims the
  page).

## Prior Decisions

- 4bced740 (2026-11-12, commit): "The board column's own header row (.bhead) is gone; the button is a small square-terminal icon button in a slot at the right end of the composer's status line ... the header row cost a whole row of vertical space" — bearing: constrains (the ticket reverses it on purpose through (552)=ok, (555)=a and (556)=a; the no-composer fixed slot and popover placement rules it set go with the slot)
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "The board column gains a one-row header, mockup-sized, holding \"+ Terminal\". Its popover picks the working folder ... Rejected: a button in the session top bar ((120) b)." — bearing: supports (the toolbar returns the button to the board's head)
- 261112-feat-ws-dashboard-pin-column ((447), 2026-11-12, Decisions): "The pinned column scrolls separately from the board column. When a pinned topic or card is updated it keeps its place ... it does not move." — bearing: constrains (pin order is untouched in overview)
- 3e922446 (2026-11-12, commit): "One movable pin list element (#bpins) instead of two reconcilers: a layout switch moves that one element, and a pin or unpin moves one node across, so frames reload as little as the browser allows." — bearing: supports (Decision 20 moves the one pin list node)
- 6b38ce4a4 (2026-11-13, commit): "fill the empty board state, drop the pinned-count header" — bearing: constrains (Decision 23: the column has no label to dim)
- 9b2ab99bb (2026-11-14, commit): "#bpins stays mounted (cards.js pins logic needs it), so added the `.bpins:empty:first-child + .empty:last-child` form" — bearing: constrains (superseded by Decision 15: the empty state keys on no topic, not child position)
- aa3c2634f (2026-11-14, commit): "The guard keys on 'no topic and no card' rather than child position, so a future board-toolbar first child and an open card do not trip the fill." — bearing: supports (Phase 1 keeps and verifies this rule)
- 261113-feat-ws-dashboard-board-reorder-hold (decision 8): "Rejected: separate globals per consumer ... and handing out the raw IntersectionObserver" — bearing: constrains (overview uses the shared keep helper and adds no scroll writer of its own)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | assets/cards.js, assets/terminal.js, assets/dashboard.js, assets/dashboard.css, assets/dashboard.tmpl, new assets/overview.js, pages.go embed list, wswhiteboard assets/page.tmpl, internal/wsi18n/tables/*.json (11 tables) |
| scope.surface | cross-module | wsdashboard assets plus one additive attribute in wswhiteboard page.tmpl and new wsi18n keys; no exported Go symbol, shell protocol unchanged (Decision 16) |
| scope.new_public_symbol | yes | window.WsOverview (assets/overview.js), data-closed-more attribute, js.board.toolbar.* and js.board.overview.* i18n keys |
| scope.new_type_contract | no | none; Decision 19 reuses the existing layout.dim field |
| scope.test_surface | new-files | existing cards_test.go and terminal_page_test.go are updated, but overview node unit tests and the toolbar, overview and overview-motion headless scenarios are new; scripts/board-headless.mjs absent from scripts/ on develop (pending 261113-feat-ws-dashboard-board-reorder-hold) |
| complexity.reuse_points | unconfirmed | wbHistory.keep and board-headless.mjs are not on develop yet (grep of wswhiteboard/assets/stream.js finds no keep; scripts/ holds only dev.py); fixedNode, moveBefore and layout.dim reuse points were read |
| complexity.side_effect_risk | high | moves the iframe-bearing pin list, writes board scroll, holds board swaps, and removes the composer terminal slot in the shared dashboard.js and cards.js paths |
| risk.correctness | high | scroll anchoring, per-frame pin flight with exact landing, and held swaps interact with the unlanded reorder-hold keep sessions |
| risk.fit | moderate | follows the cards.js fixedNode and WsTerm.mount patterns, but adds a new module and removes an existing entry point |
| risk.test | high | motion and landing criteria are verifiable only in headless captures, and the headless harness it depends on has not landed |
| risk.security_or_contract | low | T0 per Decision 16: no route, tool, store or protocol change; one additive HTML attribute and one per-session sessionStorage key |

## Phases

### Phase 1: the toolbar card and the single new-terminal entry point

Decisions 2-4, 15, 24, 26, 40, 41. No overview behaviour yet: the
Overview button renders but does nothing until Phase 2 wires it (or it is
hidden behind a flag Phase 2 removes; worker's choice).

- cards.js: the toolbar node, built at `mount` before the pin list,
  becomes a `fixedNode`, and `split` returns it first in both pin layouts.
  It is excluded from `over()`'s flow peers, from `flowNext`, from
  `headTime`, and from drop targets. It has no `data-rail`.
- terminal.js: `button()` mounts into the toolbar's slot. The `#tslot`
  composer slot, the `.tslot.tfix` fallback, their CSS, and the `.cfoot`
  special case go, and so does the copy-answers bar's corner margin
  (Decision 40). The popover opens below the button and still fits the
  viewport (`placePop`).
- Web: Decision 41, when browser-pane's "+ Web" button is on the base
  branch. Otherwise browser-pane lands second and moves it into the toolbar
  (its runner was told on 2026-11-14), and this phase adds nothing for it.
- Search: the toolbar button calls the same entry as the shell's search
  message (`$("tkqs")` click / `shSearch`).
- Files: Decision 24.
- The count: Decision 26.
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
  fallback and its CSS (including the `.reply` corner margin) are gone. The
  toolbar is built by cards.js at mount, so the new-terminal button's
  presence on a `data-term` page and absence without it is checked in the
  node harness (or as an asset-string check), not in the server render.
- The count (Decision 26), node harness: "open N · closed M" for a board with
  open topics, pinned topics, shown closed topics and a `data-closed-more`
  remainder, and it updates after a landed morph.
- wswhiteboard: the closed-more control carries `data-closed-more` with the
  remainder count.
- `go test ./internal/wsi18n/...` passes.
- `go test ./internal/wsdashboard/... ./internal/wswhiteboard/...` passes.
- Headless, in `scripts/board-headless.mjs` (landed by reorder-hold Phase 1,
  this ticket's prerequisite; absent from `scripts/` on develop today, pending
  261113-feat-ws-dashboard-board-reorder-hold; a new scenario `toolbar`, against
  `board-headless.mjs serve` pages):
  - On an adapter page and on a board page, the toolbar is first, above the
    pins at 420 px, and scrolls away with the board (not sticky).
  - The new-terminal popover opens from the toolbar, and picking the session
    worktree opens a terminal card at the head of the unpinned flow (on a
    page with `data-term`).
  - The composer shows no terminal button.
  - Where browser-pane's web button exists, it sits right after 새 터미널
    (order 한 눈에 보기 / 새 터미널 / 웹 / 검색 / 파일) and opens its input popover; it is absent where Decision 41 says.
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

Decisions 5-7, 10-14, 17-20, 22-25, 27-29, 31 and 32-39.
This phase builds every end state and the reduced-motion path (Decision 9's
instant switch). Phase 3 adds the motion between those states. This phase
owns the topic flash and its reduced-motion cue (Decision 25), since card
and pin landings flash here.

- `assets/overview.js` (Decision 18), embedded and served like shell.js.
- Enter and exit from the Overview button.
- The root view state: the fold, the 2-line recap clamp, head actions faded,
  and the pin mark.
- The darkroom in the frame, and in the shell through `layout.dim`.
- The `.here` accent bar on the entry anchor.
- On a wide board, the pin list moves into the stack under the toolbar and
  back (Decision 20). The label follows Decision 23.
- Order freeze: no drag, held board swaps (Decision 17), and deferred
  reorder-hold releases if its Phase 2 has landed.
- Outside click capture (Decision 11), card click landing, and a pinned
  item's return with a flash. ↑/↓/Enter/Esc (Decision 28).
- Toolbar actions per Decision 12, and the palette pick per Decision 29.
- sessionStorage per Decision 27. Landing scroll writes go through
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
    lands (Decision 17).
  - Search keeps overview open. Picking a terminal folder exits first and
    opens the card. 파일 exits first and shows the Files tab.
  - A reload in overview opens folded, and exit restores the stored anchor.
  - A second session frame in the same tab keeps its own overview state.
  - With reduced motion emulated, enter and exit switch in one frame, and a
    landed topic and card show the static outline for 1.2 s (Decision 25).
    Without it, a landed topic carries the flash class like a card.
  - Enter from the board's last topics: the anchor's screen y holds within
    1 px, and exit restores the entry scroll within 1 px (Decision 33).
  - Entry with the toolbar in view: no `.here` anchor, and exit leaves the
    toolbar at its screen y (Decision 38).
  - Off-screen topics measure folded while in overview, with
    `content-visibility: auto` set on `section.topic` by the scenario
    (Decision 32).
  - With the pinned column scrolled, enter and exit restore `#bpins`'s
    `scrollTop` (Decision 34). Crossing 1600 px wide during overview leaves the
    pin list in the board, and exit puts it where the new width says
    (Decision 35).
  - The shell receives a `layout` message with dim 0.72 on entry and the
    prior dim on exit (Decision 36).
  - No drag starts in overview (Decision 13). If reorder-hold Phase 2 has
    landed, a hold release due during overview is deferred until exit;
    otherwise the Result says the check was skipped and why.
  - Esc layers (Decision 28): with the palette open over overview, Esc closes
    only the palette; with the terminal popover open, only the popover; the
    shell receives no `esc` key message for any of them. The rail hotkey
    exits overview, then acts.
  - Palette picks (Decisions 29, 37): Enter on a board item exits, lands, and
    flashes it. Ctrl+Enter exits and lands it with the palette still open.
    A node or asset test fails if the board-pick path stops routing through
    `WsOverview`. If the palette ticket has not landed, the current palette's
    pick path is checked and the Result says so.
  - The hint toast (Decision 31): it shows on the first entry in a tab and
    not on a second entry or in a second session frame in the same tab.
  - Maximize (Constraints): the Overview button does nothing while a card is
    maximized, and a cards.js `maximize()` call made while overview is open
    exits overview first.
- `go test ./internal/wsdashboard/... ./internal/wsi18n/...` passes.

### Phase 3: the one concurrent motion

Decisions 8-9, 21 and 30, keeping Phase 2's flash (Decision 25) as is.

- Enter: within one ~240 ms motion on one curve, near-viewport topics and
  cards fold (height transitions), the slot under the toolbar opens from 0
  to the pin list's height, and each pin's clone flies from its column rect
  to its live slot rect while its body box collapses (Decision 21).
- The anchor is kept through the motion by re-anchoring the keep session
  each frame (a `wbHistory.keep` call on a running session re-anchors it).
- Exit: the exact reverse, with the landing offset eased toward its target
  on the same curve.
- A pin whose start or landing rect is off-screen fades instead of flying.
- Reduced motion keeps Phase 2's instant switch.

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
  - The anchored topic's screen y holds within 1 px through enter, both from
    mid-board and from the board's last topics (Decision 33).
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
  captures against the dark mockup per Decision 30).
  - The stack, the fold depth, the 2-line recap, the darkroom levels, the
    toolbar staying bright, the pin landing slot, and the `.here` bar match.
  - The pin landing is exact where the mockup's is off ((565)).
  - Differences that the Decisions require (real topic markup, the shell's
    header dim, absent buttons) are listed in the Result.
- `go test ./internal/wsdashboard/...` passes.

## Sage Review Round 1 (2026-11-14)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | browser-pane's "+ Web" button loses its home and has no defined place in the toolbar | important | missing |
| 2 | content-visibility from frontend-performance Phase 2 defeats the folded stack | important | autonomous |
| 3 | the entry anchor cannot hold its screen y near the bottom of the board | important | autonomous |
| 4 | blocked-by prerequisite not landed | minor | autonomous |
| 5 | the pinned column's scroll position is lost across overview | minor | autonomous |
| 6 | other cards.js placement paths can pull the pin list back mid-overview | minor | autonomous |
| 7 | layout.dim wiring details are misstated or missing | minor | autonomous |
| 8 | Decision 29 depends on a palette pick path that the ready palette ticket rewrites | minor | autonomous |
| 9 | small spec gaps | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Decisions assigned to Phase 2 with no matching verification (13, 25, 28, 29, 31, maximize constraint) | minor |
| 2 | Toolbar count (Decision 26) has no verification | minor |
| 3 | Topic flash ownership split between Phases 2 and 3 | minor |
| 4 | Deferred reorder-hold release has no check | minor |
