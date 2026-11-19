---
title: "대시보드 고정 패널을 Paged Docking Gallery(2D 스플릿·바운스-스냅)로 전환"
related:
  261112-feat-ws-dashboard-pin-column: the pinned column and vertical pin stack this ticket replaces with a gallery
  261115-bug-ws-dashboard-pinned-deeplink-under-topic-header: sticky-header occlusion in stacked pins; the full-bleed default removes that case by construction, and whether 261115 then closes is settled at ready promotion
  261117-feat-ws-dashboard-wide-drawer-tickets-notes: wide-drawer sheets for tickets, notes, and git; open question (08) decides whether they become dockable pages
---

# 대시보드 고정 패널을 Paged Docking Gallery(2D 스플릿·바운스-스냅)로 전환

## Background

Pinned dashboard content stacks vertically today. In the wide view, pinned
board cards and topics form the pin stack (`#bpins` inside `#bpcol`, from
`261112-feat-ws-dashboard-pin-column`). Below 1600px the pinned items sit at
the top of the board column. The stack has three known costs:

- A pinned topic keeps its sticky header, so stacked pins occlude each other's
  content (`261115-bug-ws-dashboard-pinned-deeplink-under-topic-header`).
- Each pin gets only a slice of the screen height, so a pinned terminal or
  canvas is cramped.
- Pins cannot be arranged in two dimensions; every pin takes the full width of
  the column.

The concept was worked through on the whiteboard topic
`right-pinned-panel-concept`. The owner confirmed decisions `(05)=ok` and
`(06)=ok`, and approved the interactive mockup
`ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`.
This ticket records the confirmed decisions and the three questions still open.
The ticket is not ready for implementation. The open items must settle before
promotion to `ready/`.

## Decisions

Owner-confirmed (whiteboard `right-pinned-panel-concept`, `(05)=ok`, `(06)=ok`,
mockup `ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`):

- **Unified wide and narrow model.** The vertical pin stack is replaced by a
  Paged Docking Gallery: a sequence of full-bleed docking pages backed by one
  data model at every viewport width.
  - Wide view (`>= 1600px`): the Main Board stays fixed on the left. A vertical
    dot rail sits immediately to the right of the `.bp-edge` width resize
    handle and lists `Docked Page 1..N`, followed by a trailing empty page (`+`).
  - Narrow view (`< 1600px`): the Main Board, with its infinite scroll, is
    Page 0 (`⌂`) at the top of the same rail: `[⌂ Main] · [1] · [2] · [+]`.
    Docked pages get full height. There is no top pin banner and no extra tab
    chrome.
  - Crossing the 1600px boundary changes only the layout. Both views read the
    same `localStorage` record.
- **Full-bleed 2D docking.**
  - A docked card fills 100% of its page. Scrolling stays inside the card or
    topic.
  - Each page holds a 2D split layout tree with vertical and horizontal splits.
    Dropping a pane on the top, bottom, left, or right edge zone of another pane
    shows a preview overlay and creates a split. Split ratios are set by
    draggable dividers.
  - Dragging a pane header back to the Main Board unpins it. In narrow view,
    dropping it on `⌂` does the same. Dropping it on another page's dot or on
    `+` moves it across pages.
  - Closing or unpinning the last pane of a page removes that page.
  - A trailing empty page (`+`) always exists at the end of the gallery, for
    docking new content full-bleed.
- **Vertical gallery rail and spring-loaded docking.**
  - The rail appears when at least one docking page exists, and also when a drag
    starts on the Main Board. A user can therefore dock into `+` with zero
    docked pages.
  - Rail interactions: mouse wheel (one tick = one page, debounced); direct dot
    click; click on the empty rail above or below the dots (`±1` page); vertical
    click-drag panning; dragging a dot to reorder pages.
  - Spring-loaded hover: hovering a dragged pane over any rail dot (`⌂`, `1..N`,
    `+`) for about 300ms switches the view to that page mid-drag, so the user
    can drop into an exact 2D edge zone or an empty page. Dropping on `+` before
    the 300ms elapses creates a new page. Dropping on another dot appends the
    pane to that page, or unpins it when the target is `⌂`.
- **Boundary overscroll rubber-band bounce-snap.** Inside a docked page (or
  narrow Page 0), pushing past the top or bottom scroll limit flips to the
  adjacent page through a two-phase gate:
  - Inertia guard: a wheel event arriving within about 170ms of active inner
    scrolling hits a firm micro-bounce and charges no flip tension. This stops
    trackpad or wheel momentum from flipping pages by accident.
  - Rubber band and progress pill: continued scrolling at rest on the boundary
    applies diminishing rubber-band translation (at most 26px). It fills a top
    or bottom edge pill (`0%` to `100%`) that names the target page and
    highlights that page's rail dot.
  - Spring-back or snap: pausing before the threshold (about 240ms idle) springs
    back cleanly. Crossing the threshold flips the page with a slide animation,
    then holds a refractory lock of about 420ms.
  - Widgets that consume their own wheel events (terminals, zoomable canvases,
    iframes) are not affected by the gate.
- **Persistence and compatibility (T0).** Browser-local state in `localStorage`
  under `ws.dashboard.cards.<rid>`, as today. Nothing is stored server-side.
  - A legacy flat `pinned` list migrates cleanly into gallery entries, one
    pinned item per page.
  - Compatibility tier T0: dashboard asset content only. No store, protocol,
    MCP, or config change.
- **Mount invariant.** Inactive docking pages stay mounted. Switching pages
  never reloads an iframe or a terminal. This continues the `moveBefore` /
  no-remount invariant in `cards.js`. The gallery does not use
  `content-visibility` to hide inactive pages, per
  `261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility`.

## Constraints

- Tier T0 holds only if pins saved by this build still read as the legacy flat
  `pinned` list in the previous asset build. If the new record cannot keep that,
  the tier becomes T1 or higher, and the ticket must say so before `ready/`.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (what counts as shipped
  dashboard surface), `ai-docs/manuals/release-compatibility.md` (tier
  statement), and `ai-docs/manuals/performance.md` (mounted inactive pages and
  wheel-handling cost).
- Existing code to coordinate with (extend, do not replace wholesale):
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css`,
  `dashboard-board.js`, `cards.js`, `dashboard-layout.css`. Existing tests:
  `pincol_width_test.go`, `cards_test.go`, `whiteboard_bridge_test.go`.
- Dockable scope in Phase 1 is the current board cards and topics only (explorer,
  terminal, web). Options (08) change that scope and are not decided here.

## Phases

Phase boundaries are provisional. Ready promotion may merge or reorder them
after the queue settles.

### Phase 1: Gallery shell, rail, and single-pane pages

Replace the pin stack with the gallery: the wide and narrow rail variants, the
Main Board as Page 0 in narrow view, one card or topic per docked page, the
trailing `+` page, and wheel, dot, empty-rail, and panning navigation. Dock and
unpin by dragging to a page or to the Main Board, without split zones. Migrate
the `localStorage` record from the legacy flat `pinned` list.

Verification:

- Wide and narrow captures compared side by side with
  `ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`.
- Asset tests in the wsdashboard package, run via `~/.local/bin/ws-rgo`:
  `pincol_width_test.go` updated for the 1600px switch; `cards_test.go` for the
  record migration and the no-remount invariant.
- Stale-asset check: pins saved by this build still render in the previous
  asset build (T0 constraint above).

### Phase 2: 2D split layout tree and edge docking

Each page holds a split tree. Edge drop zones show a preview overlay. Draggable
dividers set split ratios. A header drag unpins a pane or moves it to another
page. The last pane's removal removes its page. Center drop-zone behavior waits
on `(07)`.

Verification: unit coverage for layout-tree split, remove, and normalize
operations, run via `~/.local/bin/ws-rgo`; live captures of two-pane and
four-pane layouts compared with mockup v4.

### Phase 3: Spring-loaded hover and boundary bounce-snap

Spring-loaded hover over rail dots (about 300ms). The inertia guard (about
170ms), the rubber band (at most 26px), the progress pill, the snap threshold
(about 240ms idle), and the refractory lock (about 420ms). Widgets that consume
their own wheel events are excluded.

Verification: scripted wheel-event timing tests for each threshold, plus a check
that terminals and iframes do not flip pages. Performance check per
`ai-docs/manuals/performance.md`.

## Open Decision Queue

Last ID: (09)

(07) [open] [policy] Center drop zone inside a docked pane. Options:
- a: tab stack inside the cell
- b: swap positions with the target pane
- c: four-edge split only, with no center action

(08) [open] [policy] Dockable target scope and coexistence with `wide-drawer`. Options:
- a: keep existing topics and board cards (explorer, terminal, web) only
- b: also allow docking individual whiteboard panels (`entry-panel`)
- c: also unify with the `261117-feat-ws-dashboard-wide-drawer-tickets-notes` sheets (tickets, notes, git) as dockable pages

(09) [open] [policy] Rail dot indicator style. Options:
- a: show the pane count inside each dot (`1`, `2`, ...), with `⌂` for the narrow Main Board and `+` for the trailing empty page, as in mockup v4
- b: plain dots, with count and layout preview shown only on hover
