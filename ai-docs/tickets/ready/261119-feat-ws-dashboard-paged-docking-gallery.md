---
title: "대시보드 고정 패널을 Paged Docking Gallery(2D 스플릿·바운스-스냅)로 전환"
related:
  261112-feat-ws-dashboard-pin-column: the pinned column and vertical pin stack this ticket replaces with a gallery
  261115-bug-ws-dashboard-pinned-deeplink-under-topic-header: single docked-topic sticky-header occlusion resolved by construction in this ticket per (12)=a; closed upon Phase 1 verification, not at final ticket completion
  261115-feat-ws-dashboard-conversation-rendering-consolidation: conversation update-card clicks navigate to topics or cards that may now live on an inactive docked page; the navigation hook must activate that page first (see Decisions, page activation on navigation)
  261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility: prohibits using content-visibility to hide inactive gallery pages (see Decisions, mount invariant)
  261117-feat-ws-dashboard-wide-drawer-tickets-notes: wide-drawer sheets for tickets, notes, and git; kept separate per (08)=a, out of scope for this ticket
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7a76a0e23555c53f
sage-review-completeness-reviewed: 7a76a0e23555c53f
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
`(06)=ok`, approved the interactive mockup
`ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`,
and settled the remaining design choices `(07)=b`, `(08)=a`, `(09)=a`, `(10)=b`,
`(11)=a`, and `(12)=a` on the whiteboard topic `paged-docking-gallery`.
Review of this ticket then settled `(13)=a` (bounce-snap semantics with split
panes and at edge pages) and `(14)=a` (the non-drag, touch, and keyboard docking
path).

The mockup is a prototype: its bounce-snap, spring-hover, and rail constants are
read from the v4 source (`paged-docking-gallery-v4.html`, the wheel handler near
`#L282-L343` and `scheduleSpring` near `#L351-L360`) and are restated exactly in
Decisions and Phase 3. The mockup does not implement the damped edge micro-bounce
at pages with no neighbor (`(13)=a`); that behavior is new to this ticket and
reuses the mockup's inertia-guard micro-bounce styling.

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
  - Rail interactions: mouse wheel (one tick = one page, debounced; the mockup ignores rail wheel
    events within `140ms` of the last accepted one); direct dot
    click; click on the empty rail above or below the dots (`±1` page); vertical
    click-drag panning; dragging a dot to reorder pages.
  - Spring-loaded hover: hovering a dragged pane over any rail dot (`⌂`, `1..N`,
    `+`) for `300ms` (mockup `scheduleSpring`) switches the view to that page mid-drag,
    so the user can drop into an exact 2D edge zone or an empty page. Dropping on
    `+` before the `300ms` elapses creates a new page. Dropping on another dot appends the
    pane to that page, or unpins it when the target is `⌂`.
- **Boundary overscroll rubber-band bounce-snap.** Inside a docked page (or
  narrow Page 0), pushing past the top or bottom scroll limit flips to the
  adjacent page through a two-phase gate. Constants below are the mockup v4
  values; the tension threshold and the idle timer are different quantities.
  - Inertia guard: a wheel event arriving within `170ms` of the previous wheel
    event while an inner scroll was still consuming deltas (so momentum may still
    be running) hits a firm micro-bounce (`5px` translate, damped pill at `0%`,
    cleared after `220ms` without events) and charges no flip tension. This stops
    trackpad or wheel momentum from flipping pages by accident. Inner scrolling
    uses a `2px` boundary epsilon.
  - Tension, rubber band, and progress pill: continued scrolling at rest on the
    boundary accumulates flip tension, `min(125, |deltaY|)` per wheel event. The
    tension is shown as a percentage of the snap threshold, where `100%` equals
    `320` accumulated tension units (the mockup's standard sensitivity; its
    selectable presets are `200` / `320` / `480`). Rubber-band translation is
    `26 * (1 - exp(-2.3 * pct))` px, so at most `26px`, and diminishes toward
    the limit. A top or bottom edge pill fills `0%` to `100%`, names the target
    page, and highlights that page's rail dot; the pill marks "ready" from `85%`.
  - Spring-back or snap: when no wheel event arrives for `240ms` while tension is
    below `100%`, the view springs back cleanly and tension resets (the `240ms`
    is an idle spring-back timer, not the snap threshold). Reaching `100%`
    tension flips the page with a slide animation (`28px` overshoot settle), then
    holds a refractory lock of `420ms` during which wheel events are ignored. A
    wheel event in the opposite direction resets tension.
  - Widgets that consume their own wheel events (terminals, zoomable canvases,
    iframes) are not affected by the gate.
  - Wheel-only (`(14)=a`): the gate listens to `wheel` events only. Touch
    overscroll never charges or triggers bounce-snap, so native mobile scrolling
    and pull-to-refresh are unaffected.
- **Multi-pane and edge-page bounce-snap semantics (`(13)=a`, 2026-11-19).**
  - Hovered pane: in a docked page with one or more split panes, the gate
    evaluates scroll boundaries on the scrollable pane under the pointer (the
    nearest scrolling ancestor of the wheel event target, `.gal-pbody` or
    equivalent), not on all panes together, and it is not restricted to
    single-pane pages. The mockup already resolves its scroller with
    `e.target.closest('.gal-pbody, .gal-board')`.
  - Narrow Page 0 (Main Board): bounce-snap triggers at the top
    (`scrollTop <= 0`) and at the bottom of the rendered board
    (`scrollTop + clientHeight >= scrollHeight - eps`). The board has no
    automatic bottom load (closed topics load only when the user clicks
    `button.closed-more`), so the rendered bottom is the flip boundary with no
    new auto-paging added. When zero docked pages exist in narrow view (so the
    gallery rail is hidden), the Page 0 bottom gate stays inert and does not
    flip to `+`.
  - Edges with no adjacent page (the top of the first page: Page 0 in narrow
    view, Page 1 in wide view; and the bottom of the trailing `+` page) show a
    brief damped micro-bounce only. They never charge flip tension and never wrap
    around.
- **Non-drag docking path and touch, keyboard, and mobile behavior
  (`(14)=a`, 2026-11-19).**
  - Keep the existing pin and unpin button controls on topics and on `WsCards`
    (`explorer`, `terminal`, `web`, `preview`).
  - Pin docks the item onto a new docked page (appended before `+`) and activates
    that page; pinning a `preview` card ends its transient preview state. Unpin
    returns the item to the Main Board in its unpinned state and drops its page if
    it was the last pane on that page.
  - On touch, mobile, and narrow PWA views users dock and unpin with the
    pin/unpin button and switch pages by tapping the vertical rail dots. The pin
    button and the rail dots are standard buttons, so keyboard focus and Enter
    work.
  - Wheel bounce-snap is wheel-event only (see the bounce-snap decision above).
- **Persistence and compatibility (`(10)=b`, 2026-11-19).** Keep browser-local
  state in the single `localStorage` key `ws.dashboard.cards.<rid>` (and
  `ws.dashboard.cards.wt:<worktree>` on worktree pages; `dashboard-board.js#L401`),
  with nothing stored server-side.
  - The existing `{ cards, topics, bb, bw, cw }` record (cards with `pinned`,
    topics with `p`, plus the older bare-array `cards` list;
    `agents-plugin-tool/internal/wsdashboard/assets/cards.js#L691-L728`) migrates
    cleanly into gallery entries, one pinned item per page.
  - Record schema: the `ws.dashboard.cards.<rid>` record (and the `.wt:<worktree>`
    record) gains one field,
    `gallery: { v: 1, active: <pageId|null>, pages: [{ id, root: <leaf | split node> }], rev?: <timestamp or counter> }`.
    A leaf references a card (by its card key) or a topic (by its name). A split
    node holds an axis, a ratio, and two children. Dual-write `cards[].pinned` and
    `topics[].p` alongside it so a previous asset build still renders docked
    items as flat pins. The ordinals are derived deterministically: the 1-based
    depth-first (first child before second child) traversal order of leaves across
    `gallery.pages`, pages in array order; unpinned items keep their existing
    unpinned values.
  - Load precedence when `gallery` and the flat pin flags disagree (for example
    an older asset build opened and re-saved the same key, which strips the
    unknown `gallery` field or changes flat `pinned`/`p`):
    if `gallery` is absent, migrate the flat `pinned`/`p` ordinals one pin per
    page in ascending ordinal order; if `gallery` is present, reconcile it against
    the current `cards[]` and topics: drop leaves whose item no longer has its pin
    flag set or no longer exists (and remove a page left empty, normalizing a
    split whose child is gone), and append any flat-pinned item that no leaf
    references as a new page in ascending ordinal order. The reconciled result is
    written back on the next `persist()`.
  - Because the previous build's `persist()` (`cards.js#L700-L728`,
    `#L1528`) reconstructs `{ cards, topics, bb, bw, cw }` and drops unknown
    top-level fields on load, reopening an older asset build resets 2D
    split-tree layout back to flat 1-pin-per-page. Owner explicitly approved
    this as a client-only ephemeral UI layout exception permitting a patch
    release (treated at most as T2-equivalent patch scope; no minor bump).
  - *Rejected:* `a` (companion `ws.dashboard.gallery.<rid>` key to preserve
    strict T0 write preservation — unnecessarily strict for browser-local UI
    layout state).
- **Mount invariant.** Inactive docking pages stay mounted. Switching pages
  never reloads an iframe or a terminal. This continues the `moveBefore` /
  no-remount invariant in `cards.js`. The gallery does not use
  `content-visibility` to hide inactive pages, per
  `261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility`.
  - Offscreen hiding must keep `IntersectionObserver` correct. `visibility: hidden`
    and `opacity: 0` are also ruled out, not only `content-visibility: hidden`:
    the viewed-topic `IntersectionObserver` in `dashboard-board.js` and the
    `IntersectionObserver` in `terminal.js` report an element that is invisible
    but still inside the viewport as intersecting, so hidden pages would count as
    viewed and terminals would keep drawing. Instead, inactive pages sit off the
    viewport inside a clipped viewport container (for example
    `transform: translateY(±100%)` in a clipped slide stack, with `overflow:
    hidden` on the container), so the observers report them non-intersecting while
    the terminal and iframe DOM stays mounted.
- **Center drop zone (`(07)=b`, 2026-11-19).** Dropping a dragged pane onto the
  center drop zone of an existing docked pane swaps positions with that target
  pane in the 2D split tree.
  - *Rejected:* `a` (tab stack inside the cell — adds a second navigation
    chrome layer inside paged cells), `c` (four-edge split only with no center
    action).
- **Dockable target scope (`(08)=a`, `(11)=a`, 2026-11-19).** Keep existing
  topics and all four registered `WsCards` board card kinds (`explorer`,
  `terminal`, `web`, and `preview` per `preview.js#L300`, ending transient
  preview state on dock matching `#L316`) as the dockable targets. Individual
  whiteboard panels (`entry-panel`) and
  `261117-feat-ws-dashboard-wide-drawer-tickets-notes` sheets (`tickets`,
  `notes`, `git`) remain separate and out of scope for this ticket.
  - *Rejected:* `(08)=b` (also docking individual whiteboard `entry-panel`
    items), `(08)=c` (unifying `wide-drawer` sheets as dockable gallery pages
    in this ticket), `(11)=b` (excluding `preview` cards from docking).
- **Rail dot indicator style (`(09)=a`, 2026-11-19).** Show the pane count
  inside each rail dot (`1`, `2`, ...), with `⌂` for the narrow Main Board and
  `+` for the trailing empty page, matching mockup v4.
  - *Rejected:* `b` (plain dots with count and layout preview shown only on
    hover).
- **Docked-topic header isolation and deep-link exposure (`(12)=a`,
  2026-11-19).** Docked topic panes separate a non-scrolling flex header from
  the scrolling body (matching `.gal-phead`/`.gal-pbody` in
  `paged-docking-gallery-v4.html#L91-L101`, avoiding `.tophead`
  `position: sticky` occlusion in `dashboard-rail.css#L71`) and update
  `topicScroller` (`dashboard-frames.js#L250-L256`) and `navigate`
  (`wswhiteboard/assets/stream.js#L476`) so deep links into a docked topic
  never land occluded behind the header. Keep
  `261115-bug-ws-dashboard-pinned-deeplink-under-topic-header` open until
  Phase 1 lands and passes deep-link verification, then close `261115`.
  - *Rejected:* `b` (closing `261115` now without verification), `c` (keeping
    `.tophead` sticky and leaving `261115` as a separate later fix).
- **Topic markup stays unchanged; the dashboard wraps the body (carries over
  `261112` `(445)`).** The `wswhiteboard` `page.tmpl` topic HTML does not change,
  so the whiteboard file page and the embed snapshot are unaffected. When a topic
  docks, the dashboard wraps the `section.topic`'s post-`.tophead` sibling
  elements in a dashboard-managed scroll container (`.gal-pbody`; the header
  becomes `.gal-phead`-style non-scrolling). On board updates, `morphBoard` /
  `frameGraft` (`dashboard-board.js#L570-L600`) must treat `.gal-pbody` as the
  graft target (keeping wrapping idempotent, falling back to `moveBefore`
  re-wrapping only when absent) so updating a docked `has-doc` topic does not
  move and reload its doc-frame iframes on browsers without `Element.moveBefore`;
  the morph's `plan.pins` / `WsCards.pinList()` handling expands to cover all
  gallery pages. Unpinning unwraps cleanly back to stock markup.
- **Closed and missing docked topics (carries over `261112` `(448)`, `(452)`,
  `(453)` unchanged).**
  - A docked topic that the lead closes keeps its docked slot in its closed
    (folded) shape until the user unpins it.
  - A docked topic missing from the initial open board is fetched through the
    existing paged closed route (`/s/<rid>/closed?from=N`), paging until found or
    the route ends; the `closed-slot` hidden-slot staging (`71c26a5ab`) is kept,
    so a topic that moves between a fetched closed page and the gallery leaves a
    hidden slot rather than a duplicate.
  - A docked topic confirmed absent (the closed route ends without it) is dropped
    quietly from the stored record, and when it was the last pane of its page that
    page is removed (normalizing any parent split).
  - Appearance: a closed folded topic in a full-height docked pane keeps its
    header and closed styling; its folded body lives inside `.gal-pbody`, which
    stays scrollable and fills the pane through the pane's flex column (header
    `flex: none`, body `flex: 1 1 auto; min-height: 0; overflow: auto`), so the
    flex layout does not break whether it is folded or expanded.
- **Page activation on navigation.** Anything that reveals or navigates to a
  topic or card checks first whether the target lives on an inactive docked page
  (in narrow view, anywhere off Page 0), activates that page, and only then
  scrolls the item or pane into view. This covers the `reveal` hook
  (`dashboard-board.js`), `navigate` in `wswhiteboard/assets/stream.js` (its
  `#bpins` host lookup migrates to the gallery host), and the update-card clicks
  of `261115-feat-ws-dashboard-conversation-rendering-consolidation`.
- **Layout mode mapping.**
  - `tabs` layout always uses the narrow model at any viewport width (carries over
    `261112` `(455)`): the Main Board is Page 0 and there is no gallery column.
  - `1600px` to `2199px`: when at least one docked page exists, the session rail
    becomes the overlay (carries over `261112` `(449)`, `(457)`); with no docked
    page the rail docks as today.
  - `>= 2200px` with zero docked pages: no empty gallery column is reserved,
    and board-width (`bw`) resizing via `.bp-edge` with zero docked pages goes
    away with that column. The Main Board takes the full width; the gallery rail
    appears only on drag start or once a docked page exists. This supersedes the
    empty pin-column reservation introduced by commit `f1f1e8f44` (`PIN_SLOT` in
    `pinColumnFits`, idle drop placeholder, and zero-pin `.bp-edge`), while
    preserving `261112` `(446)`'s 2200px breakpoint and `(449)`. With one or
    more docked pages, the layout is rail | conversation | board | gallery as
    before.

## Constraints

- Compatibility (`(10)=b`): dashboard asset content only (no store, protocol,
  MCP, or config change). Dual-write `cards[].pinned` and `topics[].p` in
  `ws.dashboard.cards.<rid>` (and `.wt:<worktree>`) so a previous asset build
  renders docked items as flat pins; loss of 2D split-tree layout if an old
  asset build calls `persist()` (`cards.js#L700-L728`, `#L1528`) is explicitly
  accepted by the owner as a client-only UI layout exception permitting a patch
  release.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (what counts as shipped
  dashboard surface), `ai-docs/manuals/release-compatibility.md` (tier
  statement), and `ai-docs/manuals/performance.md` (mounted inactive pages and
  wheel-handling cost).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Existing code to coordinate with (extend, do not replace wholesale):
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css`,
  `dashboard-board.js`, `cards.js`, `dashboard-layout.css`,
  `dashboard-frames.js`, `preview.js`, and `wswhiteboard/assets/stream.js`.
  Existing tests: `pincol_width_test.go`, `cards_test.go`,
  `whiteboard_bridge_test.go`; the new Node and Playwright harnesses are named in
  Phases.
- Dockable scope is limited to existing topics and board cards (`explorer`,
  `terminal`, `web`, `preview`). Individual whiteboard `entry-panel` items and
  `wide-drawer` sheets (`tickets`, `notes`, `git`) are out of scope per
  `(08)=a` and `(11)=a`.

## Prior Decisions

Local ticket/history fallback on 2026-11-19: `rationale.query` is unavailable
in this session; these pointers are not an exhaustive rationale-index result.

- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions): "(442) Pin state and hand-moved order are stored per session in this browser, extending the existing card record under ws.dashboard.cards.<rid>." — bearing: constrains
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions): "(444) Compatibility tier T0: dashboard asset content only; no store, protocol, MCP, or config change." — bearing: supports
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions): "on load fetches a pinned name missing from the board through the existing paged closed route (/s/<rid>/closed?from=N), paging until it is found or the route ends." — bearing: constrains
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Result): "one movable pin list is the only node that changes parents on a layout switch." — bearing: supports
- 261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility (2026-11-17, Decisions): "Keep visible-only rendering. Owner 2026-11-17 15:56: do not blind-revert content-visibility skipping." — bearing: constrains
- 261117-feat-ws-dashboard-wide-drawer-tickets-notes (2026-11-17, Decisions): "In the wide drawer the Tickets panel and the Notes panel each span 2 grid columns; Agenda, Session holders and Worktrees take 1 column each." — bearing: constrains
- 24aa976cf (2026-11-17, commit): "file clicks reuse the memory-only reading pane across the board and pins." — bearing: constrains
- 71c26a5ab (2026-11-12, commit): "A topic that leaves a fetched closed page for the pin list, one that loadClosed finds already pinned, and a pinned closed topic pushed past the cap now leave a hidden slot (closed-slot)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard-board.js, assets/dashboard-rail.css, assets/dashboard-layout.css, pincol_width_test.go, cards_test.go, whiteboard_bridge_test.go |
| scope.surface | cross-module | window.WsCards in cards.js consumed by preview.js, files.js, webcard.js, terminal.js, dashboard-board.js, dashboard-conversation.js, dashboard-frames.js, dashboard-tracker.js, overview.js, plus dashboard-rail.css and dashboard-layout.css |
| scope.new_public_symbol | unknown | no Go or MCP symbol named; whether window.WsCards exposes new methods beyond weave/settle/pinList/relayout is unset |
| scope.new_type_contract | yes | ws.dashboard.cards.<rid> browser localStorage record extends {cards, topics, bb, bw, cw} with gallery pages and 2D split tree state |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/pincol_width_test.go, cards_test.go, whiteboard_bridge_test.go; new gallery-state/tree/wheel .test.cjs and gallery.browser.cjs files named in Phases |
| complexity.reuse_points | confirmed | WsCards weave/settle/moveBefore/grab in cards.js#L391-L545,L1350-L1437; pinColumnFits and #bpcol mount in dashboard-board.js#L364-L423; .bp-edge and 1600px/2200px rules in dashboard-rail.css#L59-L103 |
| complexity.side_effect_risk | high | replaces #bpcol/#bpins across board morph, Element.moveBefore frame preservation, overview, and wide-explorer preview visibility |
| risk.correctness | high | 2D split tree mutations, cross-page moves, no-remount invariant, and wheel inertia/snap timing gates |
| risk.fit | high | reconciling full-bleed 2D panes with non-scrolling docked-topic headers, wide-explorer file-preview-pane, and `preview` card docking |
| risk.test | high | browser drag/split/wheel timing tests, docked-topic deep-link check, and live captures against mockup v4 are required across viewports |
| risk.security_or_contract | low | browser-local `ws.dashboard.cards.<rid>` dual-write + owner-approved client-only patch exception settled per `(10)=b` |

## Phases

Test harnesses used by every phase (this repo's `internal/wsdashboard` suite, run
from `agents-plugin-tool/`; prefix with `~/.local/bin/ws-rgo` on remote-Go hosts):

- Node unit tests, `node --test internal/wsdashboard/testdata/*.test.cjs`
  (also run through Go wrappers such as `runNode` in
  `board_exposure_client_test.go`): pure logic with no DOM, namely the page
  and split-tree model, the load-time migration and reconcile, the layout-mode
  mapping, and the wheel tension arithmetic. New files:
  `gallery-state.test.cjs`, `gallery-tree.test.cjs`, `gallery-wheel.test.cjs`.
- Playwright browser tests, `testdata/*.browser.cjs` driven by a Go test, run as
  `go test ./internal/wsdashboard -run 'Gallery'`: DOM, drag, wheel, deep-link,
  and layout checks at real viewports. New files: `testdata/gallery.browser.cjs`
  with `gallery_browser_test.go`, following `board_units_browser_test.go`.
- Existing Go asset tests: `pincol_width_test.go`, `cards_test.go`,
  `whiteboard_bridge_test.go`.

### Phase 1: Gallery shell, rail, and single-pane pages

Phase 1 owns single-pane docking and unpinning (no split zones). Scope:

- Replace the pin stack with the gallery: the wide and narrow rail variants, the
  Main Board as Page 0 in narrow view, one card or topic per docked page
  (topics plus `explorer`, `terminal`, `web`, and `preview` cards per `(11)=a`),
  the trailing `+` page, rail dots with pane counts per `(09)=a`, and wheel, dot,
  empty-rail, and panning navigation.
- Docking by drag between the Main Board and docked pages: the rail appears when a
  drag starts on the Main Board, dropping on `+` creates a page (including with
  zero docked pages), dropping a docked pane on the Main Board (or `⌂`) unpins it,
  and dropping on another page's dot appends to it. Rail dot reordering: dragging
  a dot reorders pages.
- Non-drag path per `(14)=a`: the pin and unpin buttons dock to a new page before
  `+` (activating it, ending `preview` transient state) and unpin back to the Main
  Board (dropping the page when it was the last pane); touch and keyboard use the
  buttons and the rail dots.
- Layout modes per the layout mode mapping decision: `tabs` is always narrow,
  `1600px` to `2199px` rail overlay when a docked page exists, and no reserved
  empty column at `>= 2200px` with zero docked pages.
- Docked-topic body wrapper (`.gal-phead`/`.gal-pbody`) re-applied across board
  swaps and morphs and removed on unpin, with `page.tmpl` unchanged; separate the
  non-scrolling header from the scrolling body and update `topicScroller`
  (`dashboard-frames.js`) and `navigate` (`wswhiteboard/assets/stream.js`) per
  `(12)=a`.
- Page activation on `reveal`, `navigate`, and update-card clicks (see Decisions);
  offscreen hiding by moving inactive pages off the clipped viewport, never
  `visibility`, `opacity`, or `content-visibility`.
- Carry-over of `261112` `(448)`, `(452)`, `(453)`: closed docked topics keep their
  slot, missing docked topics are fetched through `/s/<rid>/closed?from=N` with
  `closed-slot` staging preserved, and confirmed-absent topics are dropped quietly
  (removing the page if it was the last pane); closed folded topics render in
  `.gal-pbody` as specified.
- `localStorage` migration from `{ cards, topics, bb, bw, cw }` (and bare-array
  legacy records; `agents-plugin-tool/internal/wsdashboard/assets/cards.js#L691-L728`)
  to the `gallery` schema, the load precedence rules, and the dual-write of
  `cards[].pinned` / `topics[].p`, all per `(10)=b`.
- Close `261115-bug-ws-dashboard-pinned-deeplink-under-topic-header` once Phase 1
  lands and the deep-link verification below passes.

Verification:

- Wide and narrow captures compared side by side with
  `ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`.
- Go asset tests: `pincol_width_test.go` updated for the 1600px switch and the
  `>= 2200px` zero-page behavior; `cards_test.go` for the record migration,
  `preview` docking, and the no-remount invariant.
- Node unit tests (`gallery-state.test.cjs`): flat-to-gallery migration, bare-array
  legacy records, `gallery` and flat-flag disagreement reconcile (cleared pin,
  deleted item, newly pinned flat item), deterministic `p` ordinals, and the
  layout-mode mapping (`tabs`, `1600`-`2199`, `>= 2200`).
- Playwright (`go test ./internal/wsdashboard -run 'Gallery'`): drag-dock from the
  Main Board, rail appearing on drag start, drop on `+` with zero pages, drop on
  `⌂` unpinning, rail dot reorder by dragging a dot, the pin/unpin button path
  (page appended and activated; unpin dropping an emptied page), keyboard Enter on
  pin button and dots, tapping dots in a touch-emulated narrow viewport, `reveal`
  / `navigate` / update-card activation of an inactive page, off-viewport
  inactive pages reported non-intersecting by `IntersectionObserver` with terminal
  and iframe DOM still mounted, and closed or missing docked topics (slot kept,
  fetched via the closed route, quietly dropped with the last-pane page removed).
- Stale-asset check: dual-written `cards[].pinned` and `topics[].p` render docked
  items as flat pins in the previous asset build (`(10)=b`).
- Docked-topic deep-link check (`(12)=a`): clicking an item reference into a
  docked topic scrolls the target into view unobstructed by the header. After it
  passes, close `261115`.

### Result (f7b0cb109) - 2026-11-20

Landed (`d8fec4dcf..f7b0cb109`):

- `assets/gallery.js` (window.WsGallery, shipped ahead of cards.js in `CardsScript`) is the only home of
  the page/split-tree model, migration and reconcile, depth-first ordinals, positions, the layout mode
  mapping, drop zones, and the Phase 3 timing gates (`d8fec4dcf`).
- `cards.js` builds and owns `#bgal` (`.bp-edge`, `.gal-rail` of `button.gal-dot`, clipped
  `.gal-viewport` with `.gal-page` slides and the trailing `.gal-plus`); `pinList()` is removed for
  `gallery()` / `docked()` / `isDocked()` / `activate()`; every caller (board morph, frames, conversation,
  overview, stream.js `navigate`, topicScroller) migrated (`0e8d810d1`, `862e67a40`).
- Narrow view: the board column's children move off-screen inside the column clip while a docked page
  shows, so the covered Main Board reports non-intersecting; inactive slides sit 100% + 24px off the
  viewport so a touching edge never counts as intersecting (`7dc18b475`, `9730b4de0`).

Verification:

- `node --test testdata/gallery-*.test.cjs`: 47/47.
- `go test ./internal/wsdashboard/ -count=1`: ok (321s); `./internal/wswhiteboard/...`: ok (199s).
- `go test ./internal/wsdashboard -run Gallery` with Playwright (`TestGalleryBrowser`, `8f3533d17`): every
  Phase 1 Playwright item passes, three consecutive runs. Other browser suites touching the board pass or
  fail identically on base `d10d68e64` (pre-existing).
- Stale-asset check (10)=b (`TestGalleryRecordInPreviousBuild`, `90c080681`): the frozen previous
  `cards.js` renders the new record as flat pins in depth-first order; its own save drops `gallery`, and
  the new build reloads one pin per page.
- Deep-link check (12)=a: pass — the target lands at top 123.3px, below the non-sticky header's bottom
  111.3px, and owns its `elementFromPoint`. `261115` closed.
- Captures (`ai-docs/assets/captures/261119-gallery-phase1/`, `c9ffea097`): compared live wide 2400x1000,
  narrow 1200x900 (page and home) and touch 420x860 vs mockup v4 wide/narrow: match; deliberate styling
  differences are listed in that folder's README.

Decisions:

- `active` also takes `"+"` (the trailing page) besides a page id or `null` (home), so a view on `+`
  survives re-derivation.
- A center drop is defined only between two docked panes; a board-sourced drag gets the four edges only.
- In narrow view with zero docked pages the whole bounce-snap gate is inert (rail hidden), not only Page
  0's bottom.
- Overview covers the Main Board only; docked units stay in the dimmed gallery (no re-parenting).
- `data-pincol` is kept as the "right column shows, session rail overlays below 2200px" signal its
  existing readers use; `data-gal=col|drag|narrow` is added. A wide drag with zero pages shows a fixed
  right-edge overlay so the grid does not reflow under the pointer.
- A docked topic reporting FAR no longer unmounts its frames, so inactive pages never reload doc frames.

### Phase 2: 2D split layout tree and edge docking

Phase 2 owns multi-pane recursive 2D layouts. Scope:

- Each page holds a recursive split tree (vertical and horizontal splits). The
  five-zone drop (four edge zones plus center) shows a preview overlay; an edge
  drop splits, and a drop on the center zone swaps positions with the target pane
  per `(07)=b`.
- Draggable split dividers set ratios.
- Pane-to-pane drags move or re-split a pane within a page and across pages; a
  header drag out to the Main Board unpins that pane.
- Unpinning or closing one pane of a multi-pane page collapses its parent split
  and normalizes the tree; the last pane's removal removes the page.

Verification:

- Node unit tests (`gallery-tree.test.cjs`): split, swap, move, remove, collapse,
  and normalize operations, ratio clamping, and depth-first ordinal derivation.
- Playwright (`go test ./internal/wsdashboard -run 'Gallery'`): five-zone drop
  overlay and result, divider drag, pane moves within and across pages, unpinning
  one pane out of a multi-pane page collapsing its parent split, and last-pane
  page removal; live captures of two-pane and four-pane layouts compared with
  mockup v4.

### Phase 3: Spring-loaded hover and boundary bounce-snap

Scope, with the exact mockup v4 constants:

- Spring-loaded hover over rail dots: `300ms`, switching the view mid-drag;
  dropping on `+` before `300ms` creates a new page.
- Bounce-snap gate: inertia guard `170ms` (micro-bounce `5px`); tension
  `min(125, |deltaY|)` per wheel event against a `100%` snap threshold of `320`
  units; rubber band at most `26px`; progress pill with the target dot highlight;
  `240ms` idle spring-back when below `100%`; refractory lock `420ms` after a
  flip. Widgets that consume their own wheel events are excluded.
- `(13)=a` semantics: hovered-pane boundaries in split pages, narrow Page 0 top
  and rendered-bottom boundary (inert with zero docked pages; no new auto-load),
  and a damped micro-bounce only at edges without an adjacent page (no tension,
  no wrap).
- `(14)=a`: wheel events only; touch overscroll does not charge tension.

Verification:

- Node unit tests (`gallery-wheel.test.cjs`): scripted wheel-event timing for each
  constant (tension accumulation and the `100%` snap at `320`; spring-back after
  `240ms` idle below the threshold; inertia guard at `170ms`; refractory `420ms`;
  direction reversal reset; `300ms` hover timer and the `+` drop before `300ms`).
- Playwright (`go test ./internal/wsdashboard -run 'Gallery'`): the `300ms`
  spring-loaded hover auto-switches pages during a drag; dropping on `+` before
  `300ms` creates a new page instead of waiting for the hover switch; hovered
  pane under the pointer in a split page drives the flip; narrow Page 0 bottom
  flips to Page 1 when docked pages exist and stays inert when zero docked pages
  exist; first-page top and trailing `+` bottom show the damped micro-bounce with
  no flip or wrap; terminals and iframes do not flip pages; touch scroll
  emulation never charges tension.
- Performance check per `ai-docs/manuals/performance.md` (mounted inactive pages,
  wheel handler cost).
