---
title: "대시보드 고정 패널을 Paged Docking Gallery(2D 스플릿·바운스-스냅)로 전환"
related:
  261112-feat-ws-dashboard-pin-column: the pinned column and vertical pin stack this ticket replaces with a gallery
  261115-bug-ws-dashboard-pinned-deeplink-under-topic-header: sticky-header problem in the pin column; the full-bleed default may remove it, but this ticket does not close 261115 (see Constraints)
  261117-feat-ws-dashboard-wide-drawer-tickets-notes: wide-drawer sheets for tickets, notes, and git; kept separate from dockable gallery pages per settled decision (08)=a
  261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility: constrains the mount invariant (no content-visibility hiding of inactive pages)
---

# Replace the Dashboard Pinned Panel with a Paged Docking Gallery (2D Splits and Bounce-Snap)

## Background

Pinned dashboard content stacks vertically today. In the wide view, pinned
board cards and topics form the pin stack (`#bpins` inside `#bpcol`, from
`261112-feat-ws-dashboard-pin-column`). Below 1600px the pinned items sit at
the top of the board column. The stack has three known costs:

- A pinned topic keeps its sticky header, so stacked pins occlude each other's
  content (`261115-bug-ws-dashboard-pinned-deeplink-under-topic-header`; the
  sticky header is real, `dashboard-rail.css#L71`, but the 261115 ticket itself
  records only a deep-linked control left behind `.tophead .tbar` after the
  sticky stack changes at 1800×900, not pin-to-pin occlusion,
  `ai-docs/tickets/idea/261115-bug-ws-dashboard-pinned-deeplink-under-topic-header.md#L11-L22`).
- Each pin gets only a slice of the screen height, so a pinned terminal or
  canvas is cramped.
- Pins cannot be arranged in two dimensions; every pin takes the full width of
  the column.

The concept was worked through on the whiteboard topic
`right-pinned-panel-concept`. The owner confirmed decisions `(05)=ok` and
`(06)=ok`, approved the interactive mockup
`ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`,
and settled the remaining scope choices `(07)=c`, `(08)=a`, and `(09)=a` on
whiteboard topic `pdg-promote`.

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
  - Spring-back or snap: tension is the accumulated wheel delta at the
    boundary (each event clamped to 125px, mockup v4 default threshold 320, tunable
    in the mockup). The pill percent is tension divided by that threshold.
    Pausing for about 240ms idle before the threshold springs back cleanly.
    Reaching the threshold flips the page with a slide animation, then holds a
    refractory lock of about 420ms.
  - Widgets that consume their own wheel events (terminals, zoomable canvases,
    iframes) are not affected by the gate.
- **Persistence and compatibility (T0).** Browser-local state in `localStorage`
  under `ws.dashboard.cards.<rid>`, as today. Nothing is stored server-side.
  - A legacy flat `pinned` list migrates cleanly into gallery entries, one
    pinned item per page (the stored record has no separate `pinned` list: it
    is `{cards, topics, bw, bb, cw}`, and pin order is the per-card `pinned`
    number plus the per-topic `p` number, `cards.js#L690-L728`).
  - Compatibility tier T0: dashboard asset content only. No store, protocol,
    MCP, or config change.
  - **Dual-write of the flat pin order `(11)`.** To keep T0, the new build
    writes the gallery record next to the existing flat pin order: every
    docked pane keeps a nonzero `cards[].pinned` (or `topics[].p`) in the order
    page by page, pane by pane. The previous build then still renders those
    pins as a stack, and accepts that its own `persist()` drops the gallery
    key whenever it saves. *Rejected:* declaring T1 or higher (it would turn
    this dashboard-asset change into a store-compatibility change that the
    owner's T0 statement excludes).
- **Mount invariant.** Inactive docking pages stay mounted. Switching pages
  never reloads an iframe or a terminal. This continues the `moveBefore` /
  no-remount invariant in `cards.js`. The gallery does not use
  `content-visibility` to hide inactive pages, per
  `261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility`.
- **Four-edge split only; no center drop action (`(07)=c`).** Dropping a pane
  onto a docked pane only exposes top, bottom, left, and right edge split
  zones; there is no center drop action. *Rejected:* tab stack inside the cell
  (`a`) and swapping positions with the target pane (`b`), to keep the Phase 2
  2D split layout tree simple and bounded.
- **Dockable target scope (`(08)=a`).** Only existing whiteboard topics
  (`.topic`) and board cards (explorer, terminal, web) are dockable.
  *Rejected:* docking individual whiteboard panels (`.entry-panel`, `b`) and
  unifying `261117-feat-ws-dashboard-wide-drawer-tickets-notes` sheets
  (tickets, notes, git) as dockable pages (`c`), both kept out of scope so
  `wide-drawer` stays independent.
- **Rail dot indicator style (`(09)=a`).** Each docked-page dot shows its pane
  count (`1`, `2`, ...), with `⌂` for the narrow Main Board (Page 0) and `+`
  for the trailing empty page, matching mockup v4. *Rejected:* plain dots with
  count and layout preview shown only on hover (`b`).

## Constraints

- Decision IDs `(01)`-`(10)` were settled in whiteboard conversations
  (`right-pinned-panel-concept`, `pdg-promote`); only the IDs shown in this
  ticket are recorded here.

- Tier T0 holds only if pins saved by this build still read as the legacy flat
  `pinned` list in the previous asset build. If the new record cannot keep that,
  the tier becomes T1 or higher, and the ticket must say so before `ready/`.
  (Checkable from the tree: the previous build's `stored()` reads only `cards[]`
  entries with a known `kind`, `topics[]` entries with `+p || +a || f`, and
  `bw`/`bb`/`cw`, and ignores any other key, `cards.js#L715-L728`; its
  `persist()` rewrites the record from only `cards`, `topics`, `bb`, `bw`, `cw`,
  so a gallery key added by the new build is dropped whenever the previous build
  saves, `cards.js#L700-L714`. T0 therefore needs the new build to keep writing
  `cards[].pinned` and `topics[].p` as a nonzero pin order for every docked
  pane; settled by `(11)` above.)
- Planning must settle, without changing a recorded decision: the gallery
  record's key name, shape (it must hold Phase 2's split tree), version marker,
  read precedence when the gallery record and the flat pin order disagree
  (two tabs, a partly dropped key), and the handling of stored ids that no
  longer resolve.
- Planning must enumerate every consumer of the replaced pin stack: the
  `.bpins` overview-motion rules (`dashboard-board.css#L159-L160`), `pinColumnFits`
  and `opt.column`/`opt.shows` (`dashboard-board.js#L373-L418`), and
  `pincol_width_test.go`; and decide what overview mode
  (`:root[data-overview]`) does with docked pages. Gallery rail class names stay
  distinct from the session rail, wswhiteboard `rail.js`, and the session rail
  cues; rerun the CSS collision test.
- Inactive pages are hidden without `content-visibility`, and the existing
  `.bcard ... > .bc-body { content-visibility: auto }` rule
  (`dashboard-board.css#L99`) and topic-part skipping must not skip docked
  bodies on inactive pages. The no-remount test covers a hidden-page switch and a
  `moveBefore` across pages, and terminal fit, canvas size and iframe state
  survive the hiding method.
- The update mark of `261112` (447) must stay visible for a pin on a hidden
  page; propagating it to the rail dot is the expected route.
- The boundary gate resolves the scrolling pane from the pointer target, as
  mockup v4 does (`e.target.closest('.gal-pbody, .gal-board')`). Touch swipe is
  out of scope for the bounce-snap flip; rail dots and drag cover touch.
- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (declared for `agents-plugin-tool/`)
- Convention: `ai-docs/manuals/release-compatibility.md` (declared for `agents-plugin-tool/internal/`)
- Convention: `ai-docs/manuals/performance.md` (declared for `agents-plugin-tool/`)
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (what counts as shipped
  dashboard surface), `ai-docs/manuals/release-compatibility.md` (tier
  statement), and `ai-docs/manuals/performance.md` (mounted inactive pages and
  wheel-handling cost).
- Existing code to coordinate with (extend, do not replace wholesale):
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css`,
  `dashboard-board.js`, `cards.js`, `dashboard-layout.css`. Existing tests:
  `pincol_width_test.go`, `cards_test.go`, `whiteboard_bridge_test.go`.
- Dockable scope is limited to existing whiteboard topics (`.topic`) and board
  cards (explorer, terminal, web). Individual whiteboard `.entry-panel` cards
  and `261117-feat-ws-dashboard-wide-drawer-tickets-notes` sheets (tickets,
  notes, git) are out of scope per `(08)=a`.
- `261115-bug-ws-dashboard-pinned-deeplink-under-topic-header` stays in `idea/`
  as is; this ticket does not close it. Its Phase 1 demands its own strict
  center-ownership probe evidence. After this ticket lands, the owner may re-run
  that probe and close 261115 separately; that is out of scope here `(12)`.

## Open Decision Queue

Last ID used: (14)

- (13) [policy] What does a Phase 1 drop onto an occupied page's dot do, and where does an appended pane land once Phase 2 exists?
  - Context: Decisions say a drop on another dot appends the pane to that page and the dot shows a pane count (`(09)=a`), but Phase 1 has one pane per page and no split zones.
  - Alternatives: (a) Phase 1 creates a new page at the end for the dropped pane; from Phase 2 on, an appended pane splits the page's root, default direction vertical with the new pane at the right/bottom, ratio 50:50. (b) Phase 1 rejects the drop on an occupied page (only `+` and `⌂` accept); Phase 2 then appends as in (a). (c) Phase 1 replaces the page's pane.
  - Recommendation: (a) - keeps every drop productive in Phase 1 and gives Phase 2 one default rule; check the exact Phase 2 default direction against mockup v4.
- (14) [policy] Record that this ticket supersedes `261112-feat-ws-dashboard-pin-column` Decisions (443) and the layout and drop-zone parts of (449)?
  - Context: (443) says below 1600px there is no pinned column and pins stand first in the board column; (449) fixes the right-edge drop zone, the empty pinned group at the top of the board column below 1600px, and the pinned-column conditions. This ticket's narrow-view gallery and rail replace them. The reversal is owner-approved but not named.
  - Alternatives: (a) Add a `## Decisions` line "supersedes `261112` (443) and the layout and drop-zone parts of (449): the gallery replaces the pinned column and the top-of-board pin group at every width". (b) Name only (443).
  - Recommendation: (a). (444), (445), (447), and (448) stay in force.

## Prior Decisions

- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions (442)): "Pin state and hand-moved order are stored per session in this browser, extending the existing card record under `ws.dashboard.cards.<rid>`. Nothing is stored server-side" — bearing: supports
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions (444)): "Compatibility tier T0: dashboard asset content only; no store, protocol, MCP, or config change." — bearing: constrains
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions (445)): "Topic pinning and dragging are implemented by letting topics take part in the existing WsCards placement engine ... with no separate topic placement scheme." — bearing: constrains
- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions (443)): "Below the wide view (< 1600px) there is no pinned column: pinned topics and cards stand first in the board column, in pin order" — bearing: contradicts (replaced by the narrow-view gallery; see `(14)`)
- 261117-bug-ws-dashboard-pwa-renderer-crash-content-visibility (2026-11-17, Decisions): "Keep visible-only rendering. Owner 2026-11-17 15:56: do not blind-revert content-visibility skipping." — bearing: constrains
- f1b4e1866 (2026-11-17, commit): "Merge PWA crash provisional fix V1: drop the focus guard on board topic-part content-visibility skipping" — bearing: constrains
- 261115-bug-ws-dashboard-pinned-deeplink-under-topic-header (idea, Phase 1): "Retain the strict assertion and compare the same isolated setup on both production trees. Do not infer a complete cause beyond the retained measurements" — bearing: constrains
- 261117-feat-ws-dashboard-wide-drawer-tickets-notes (done 2026-11-17, commit): wide-drawer sheets landed as a separate drawer surface — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, dashboard-board.js, dashboard-rail.css, dashboard-layout.css, plus pincol_width_test.go and cards_test.go |
| scope.surface | internal | dashboard asset content only (T0); no exported Go symbol named, the persisted ws.dashboard.cards record is asset-internal |
| scope.new_public_symbol | no | none named; any new WsCards method is an asset-internal global, not confirmed by the ticket |
| scope.new_type_contract | yes | a gallery record shape in the ws.dashboard.cards.<rid> localStorage record that the previous asset build must still read (cards.js#L700-L728) |
| scope.test_surface | new-files | Phase 2 layout-tree and Phase 3 wheel-timing tests have no existing file; cards_test.go and pincol_width_test.go exist and need updates |
| complexity.reuse_points | confirmed | cards.js pin and drag engine, moveBefore moves (cards.js#L391-L401, L518, L531), .bp-edge grab (cards.js#L1361) |
| complexity.side_effect_risk | high | replaces the pin stack and its drag, width and persistence paths shared by board, pinned column and narrow layout, and adds wheel gating |
| risk.correctness | high | 2D split tree, spring-loaded drag and rubber-band timing over the existing pin engine, with a no-remount invariant for iframes and terminals |
| risk.fit | moderate | must extend cards.js (1597 lines of pin, drag and width logic) rather than replace it, per the ticket constraint |
| risk.test | high | drag, hover timing and wheel gating have no browser test coverage in the named test files; the ticket plans scripted timing tests that do not exist yet |
| risk.security_or_contract | moderate | no security surface, but T0 depends on the previous build still reading new records, and its persist() drops unknown keys (cards.js#L700-L714) |

## Phases

### Phase 1: Gallery shell, rail, and single-pane pages

Replace the pin stack with the gallery: the wide and narrow rail variants, the
Main Board as Page 0 in narrow view, one card or topic per docked page, the
trailing `+` page, and wheel, dot, empty-rail, and panning navigation. Dock and
unpin by dragging to a page or to the Main Board, without split zones. The
existing pin buttons (`.bc-pin` on cards, `.tpin` on topics, `cards.js`) stay
the click path to dock; which page a button-pinned item lands on is settled in
planning with mockup v4. The gallery record's key name and shape, its version
marker, and whether the active page survives a reload are in scope for this
phase's plan. Migrate
the `localStorage` record from the legacy flat `pinned` list, and keep
dual-writing the flat pin order for the previous build (`(11)`).

Verification:

- Wide and narrow captures compared side by side with
  `ai-docs/assets/mockups/261119/right-pinned-panel-concept/paged-docking-gallery-v4.html`.
- Live interaction checks, each observed once in a running dashboard: wheel,
  dot click, empty-rail click, and rail panning change the page; dragging a
  pane to a page, `+`, or the Main Board docks or unpins it; each outcome
  matches mockup v4.
- Asset tests in the wsdashboard package, run via `~/.local/bin/ws-rgo`:
  `pincol_width_test.go` updated for the 1600px switch; `cards_test.go` for the
  record migration and the no-remount invariant.
- Stale-asset check: pins saved by this build still render in the previous
  asset build (T0 constraint above).

### Phase 2: 2D split layout tree and edge docking

Each page holds a split tree. Edge drop zones show a preview overlay. Draggable
dividers set split ratios. A header drag unpins a pane or moves it to another
page. The last pane's removal removes its page. Only four-edge split zones are
supported; there is no center drop action (`(07)=c`).

Verification: unit coverage for layout-tree split, remove, and normalize
operations (normalize: collapse a single-child split and clamp ratios; exact
minimum sizes are planning details), run via `~/.local/bin/ws-rgo`; live
captures of two-pane and four-pane layouts compared with mockup v4; live checks
that divider drag, the edge preview overlay, and a cross-page move behave as in
mockup v4.

### Phase 3: Spring-loaded hover and boundary bounce-snap

Spring-loaded hover over rail dots (about 300ms). The inertia guard (about
170ms), the rubber band (at most 26px), the progress pill, the snap threshold
(accumulated clamped wheel delta, see the Decisions bullet), the idle
spring-back (about 240ms), and the refractory lock (about 420ms). Widgets that consume
their own wheel events are excluded.

Verification: scripted wheel-event timing tests for each threshold, plus a check
that terminals and iframes do not flip pages. Performance check per
`ai-docs/manuals/performance.md`.
