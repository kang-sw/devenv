---
title: "Dashboard board: whiteboard topics pin like board cards, drag reorder, and a pinned column in the wide view"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-terminal: the board card framework (WsCards, cards.js) whose pin and placement rule topics join
  261110-feat-ws-dashboard-session-rail: the session rail that docks at >= 1600px and overlays below it; this ticket hides it in the wide view while a pin exists
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b5c8db9acb7eea6e
sage-review-completeness-reviewed: b5c8db9acb7eea6e
completed: 2026-11-12
---

# Dashboard board: whiteboard topics pin like board cards, drag reorder, and a pinned column in the wide view

## Background

Only board cards (the WsCards framework in
`agents-plugin-tool/internal/wsdashboard/assets/cards.js`, today the
terminal and file cards) can be pinned. A pinned card stands first in the
board column, in pin order; the rest move among the topics by recency.
Whiteboard topics cannot be pinned, and nothing on the board can be moved by
hand.

The user asked (whiteboard, 2026-11-12) for:

1. Whiteboard topics pinnable with the same frame as the board cards.
2. Drag to reorder: pinned among pinned, unpinned among unpinned. An unpinned
   move is one-shot, since board ordering is a bump concept anyway.
3. In the wide three-column view (>= 1600px), dragging a card or topic to the
   right pins it; while any pin exists the left session list hides (it opens
   with Ctrl+`) and the right column becomes the pinned list.
4. An extra-wide view: when four columns fit, the session list shows
   permanently again.

A raw HTML mockup over the real dashboard CSS
(`ai-docs/paged/pin-column-mockup/index.html`, git-ignored; screenshots of the
three layouts beside it) was shown on the board and approved by the user
("좋아 보입니다", 2026-11-12). The decisions below record what it showed.

## Decisions

- (442) Pin state and hand-moved order are stored per session in this
  browser, extending the existing card record under
  `ws.dashboard.cards.<rid>`. Nothing is stored server-side, so pins are not
  shared across devices or viewers.
- (443) Below the wide view (< 1600px) there is no pinned column: pinned
  topics and cards stand first in the board column, in pin order, as pinned
  cards do today.
- (444) Compatibility tier T0: dashboard asset content only; no store,
  protocol, MCP, or config change.
- (445) Topic pinning and dragging are implemented by letting topics take
  part in the existing WsCards placement engine (pinned first in pin order,
  the rest by recency), with no separate topic placement scheme. The topic
  HTML that wswhiteboard renders stays unchanged; the dashboard attaches the
  pin button and the drag handle on its side, so the whiteboard file page and
  the embed snapshot are unaffected. Rejected: a topic-only placement scheme
  (cards and topics mixed in one pinned column would order inconsistently).
- (446) The extra-wide view starts at 2200px (session list 340 + conversation
  min 320 + board about 700 + pinned column about 480, plus gaps); the number
  may be tuned after use. The pinned column has a width handle on its left
  edge, and its width is remembered in the browser with the pins.
- (447) The pinned column scrolls separately from the board column. When a
  pinned topic or card is updated it keeps its place and shows the update
  mark the board already uses for a changed topic; it does not move.
  Rejected: bumping an updated pin (a pin means "stays here").
- (448) A pinned topic that the lead closes (`wb:close`) keeps its pin: it
  stays in the pinned list in its closed (folded) shape until the user
  unpins it. Rejected: unpinning automatically on close (a topic the user was
  watching would vanish).
- (449) Mockup first; the approved mockup fixes these behaviours:
  - Layouts: < 1600px one or two columns with pins atop the board column;
    1600-2199px with no pin, today's three columns (rail | conversation |
    board); 1600-2199px with a pin, the rail becomes the overlay
    (Ctrl+` and the top-left arrow open it) and the columns are
    conversation | board | pinned; >= 2200px with a pin, four columns
    rail | conversation | board | pinned; >= 2200px with no pin, three
    columns as today.
  - The header of a topic (its title bar) and of a card is the drag handle;
    buttons, links, and inputs inside it do not start a drag.
  - Dropping within the pinned list reorders pins; dropping a pinned item
    into the board column unpins it at the drop position; dropping an
    unpinned item into the pinned list pins it at the drop position.
  - In the wide view with no pin yet, a drop zone appears along the right
    edge while dragging; dropping there pins the item and opens the pinned
    column.
  - Below 1600px, while dragging with no pin, the empty pinned group at the
    top of the board column shows as a drop zone.
  - An unpinned item moved by hand stands where it was dropped until a later
    board update bumps a topic above it under the existing recency rule.
  - Every topic's title bar gets a pin button (lucide `pin`, the card's
    icon), pressed while pinned.
- (450) Placement: a child of 261110-epic-ws-dashboard-agent-host, beside
  its board-card siblings 261110-feat-ws-dashboard-terminal and
  261110-feat-ws-dashboard-session-rail.
- (451) The topic rail (wswhiteboard `rail.js` capsules, which rail
  `section.topic[data-topic], [data-rail]`) keeps railing the board column
  only; the pinned column has no rail. Rejected: a second rail instance for
  the pinned column (the pinned list is short and hand-picked, so a rail adds
  a second scroll-sync target for little navigation gain).
- (452) (448) holds for a pinned closed topic that the board no longer
  serves: the served board carries only the 30 most recently closed topics
  (wswhiteboard `ClosedCap`), so the page keeps the last received node of a
  pinned topic for its lifetime when a swap drops it, and on load fetches a
  pinned name missing from the board through the existing paged closed
  route (`/s/<rid>/closed?from=N`), paging until it is found or the route
  ends. Rejected: a server-side change to serve pinned topics (breaks
  (444)'s asset-only scope).
- (453) A pin whose topic the store no longer holds (the closed route ends
  without it) is dropped from the stored record silently.
- (454) Hand moves of unpinned items land only in the open flow (between
  open topics and cards), never among closed topics or below the closed
  control or the reply bar. An unpinned closed topic returns to the closed
  section. A hand-moved unpinned topic stores a placement time that
  overrides its bar time until the topic itself next updates, since topics
  otherwise keep server order among themselves.
- (455) The layout menu's "tabs" choice counts as the narrow view at any
  width: no pinned column, pins atop the board column.
- (456) Dragging topics and cards follows the rail's drag thresholds (a7b53d6b):
  pointer drag past 5px, touch long-press 380ms.
- (457) supersedes 261110-feat-ws-dashboard-session-rail ("from a 1600px
  viewport ... the rail is always shown"): between 1600px and 2199px the rail
  docks only while no pin exists, because the user asked for the pinned
  column to take its place (Background item 3). `railWide()` and the Ctrl+`
  handler gate on the pin state as well as the width.

## Constraints

- A card's node is never re-created by a board update today (cards.js weave
  and settle; `Element.moveBefore` where available keeps a frame running).
  Topics joining the engine keep that property for their body frames
  (commits 94fd68458, 92fba2eb7: a moving open topic keeps its body frame).
- The `.tk-spage` store-page placement (21432949: grid-column 2 / row 1 of
  `.dbody` at >= 1600px) follows every grid the pin state produces.
- The mockup under `ai-docs/paged/pin-column-mockup/` is git-ignored and
  optional reference; `## Decisions` is the authoritative record of what it
  showed.
- Peer lead `autisticcavalierharpistdairy` owns the hosted wsdashboard
  surface; this ticket touches only the shared board assets, and the merge
  is announced to that lead by mail first.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261110-feat-ws-dashboard-session-rail (2026-11-10, Decisions): "From a 1600px viewport the page has three columns, rail | conversation | board, and the rail is always shown. Below 1600px the top-left arrow toggles the rail" — bearing: constrains
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "Terminals sit among the whiteboard's topics in the right column. Pinning a terminal card fixes it at the top. An unpinned card keeps its place in the topic flow" — bearing: supports
- 261110-feat-ws-dashboard-file-preview (2026-11-10, Decisions): "The framework owns what every card kind shares: fold, base height, drag handle, the maximize sheet with Esc, pin, close, the persistence of that view state, and placement in the board flow" — bearing: supports
- a7b53d6b (2026-11-12, commit): "Pointer drag with 5px threshold, touch long-press 380ms; rail updates mid-drag wait until drop. WsRail stays free of dashboard globals." — bearing: supports
- 939a2c59 (2026-11-12, commit): "A rail swap arriving mid-drag is held (like the head label edit) and lands at the drag's end, instead of aborting the drag" — bearing: supports
- 9034387e (2026-11-11, commit): "Host-neutral hook rather than naming dashboard cards in the shared rail: rail.js rails `section.topic[data-topic], [data-rail]` ... cards.js marks every card, so a new kind joins" — bearing: constrains
- 21432949 (2026-11-12, commit): "pageParent is .dbody when the page has one (session pages), else .dwrap ... CSS places .tk-spage in .dbody's grid-column 2 / row 1" — bearing: constrains
- 0cd850f5 (2026-11-12, commit): "Not fixed (pre-existing, separate): an updated topic that stays open reloads its body frame, which shows blank for ~2 frames" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard.js, assets/dashboard.css, cards_test.go |
| scope.surface | cross-module | WsCards framework API in cards.js consumed by terminal.js and preview.js; dashboard.js board morph; dashboard.css rail layout |
| scope.new_public_symbol | yes | a topic participant on window.WsCards; no Go or MCP symbol |
| scope.new_type_contract | yes | ws.dashboard.cards.<rid> browser record extended with topic pins, hand order, and pinned column width |
| scope.test_surface | existing | cards_test.go, board_motion_test.go, board_frame_graft_test.go, rail_test.go, rail_hotkey_test.go; headless width check is new |
| complexity.reuse_points | confirmed | WsCards order/weave/settle in assets/cards.js#L76-L138; rail overlay and Ctrl+Backquote hotkey in assets/dashboard.js#L5014-L5060 |
| complexity.side_effect_risk | high | topics joining the placement engine touch the board morph and frame-graft path that keeps body frames alive |
| risk.correctness | high | placement merges recency, pin order, one-shot hand moves, and closed pinned topics across reloads and board swaps |
| risk.fit | moderate | must keep wswhiteboard topic HTML and the shared topic rail unchanged while the dashboard decorates topics |
| risk.test | moderate | layout and drag behavior need a headless browser check; node harness covers only placement |
| risk.security_or_contract | low | T0 dashboard asset change, browser-local storage only, no route, store, or MCP change |

## Phases

### Phase 1: Topics in the card engine, drag reorder, pinned column

Goals: (442)-(457) as above ((450) is placement only).

Suggested approach: give `WsCards` a topic participant (a topic is a unit
with a key `topic:<stem-or-name>`, the board's own node, no framework header)
so pin order and recency placement cover both kinds; add the pin button and
drag handle in `dashboard.js` where the board morph builds topic sections;
add the layout rules in `dashboard.css` keyed by a root attribute the page
sets from the pin count (the existing >= 1600px rail docking becomes
conditional on no pin below 2200px).

Implementation notes from design review:

- The pinned column is a second container outside `region("board")` and
  outside the rail box. morphBoard's `live` map, `WsCards.weave`/`settle`,
  `settleMoves`/`frameGrafts`, and `boardSpots` assume one parent today, so
  the morph and the engine reconcile across both containers; a CSS-only
  change does not suffice. Moving between parents (pin, unpin, crossing a
  width boundary) uses `Element.moveBefore` where available.
- The pin button and drag wiring are live-DOM decoration: a changed topic
  arrives as a new section, and frameGraft replaces the old `.tbar`
  children, so decoration is reapplied to every inserted or grafted section
  after each swap.
- The update mark (447) needs no new work: it is the server-rendered
  `.chip.fresh` plus the change bars (wswhiteboard `chg.js`).

Verification:

- `cards_test.go` (node harness) cases: a pinned topic stands among pinned
  cards in pin order; an unpinned hand move is undone by a later bump above
  it; an updated pinned topic keeps its index and gains the update mark; a
  closed pinned topic stays pinned; a pinned closed topic that falls past
  the closed cap stays in the pinned list, and after a reload is fetched
  through the closed route; a pin whose topic is gone is dropped; a pinned
  topic updated through the graft path keeps its pin button pressed; pin
  and order survive a reload from storage.
- A headless check at 1500, 1800, and 2400px widths, with and without pins:
  the rail docked or overlaid as (449) lists, the pinned column present only
  in the wide views with a pin, Ctrl+` opening the rail overlay at 1800px
  with a pin, the "tabs" layout showing no pinned column, and the store page
  in its cell in each grid. Drag behaviour in the same pass: a header drag
  moves the item, a button inside the header does not start one, the drop
  semantics of (449), the right-edge drop zone at 1800px with no pin, the
  empty-group drop zone at 1500px, and the pinned column width surviving a
  reload. The package has no automated headless-browser harness, so this is
  an ad-hoc check (for example a local headless Chromium against a
  `file://` or daemon page) whose outcome the Result records.
- Existing board tests (`board_motion_test.go`, `board_frame_graft_test.go`,
  `rail_test.go`, `rail_hotkey_test.go`) stay green.

### Result (71c26a5ab) - 2026-11-12

Landed (3e922446b, 71c26a5ab):

- `cards.js`: topics are units of the placement engine beside cards. A
  topic bar gets a lucide `pin` button before its note pen; pinned topics
  and cards share one pin list (`#bpins`) in pin order; an unpinned topic or
  card dragged by its header lands in the open flow and stays until a topic
  updated later rises above it (its own update or close ends the move); a
  closed unpinned topic dropped in the flow returns to the closed section.
  Drag thresholds 5px / touch hold 380ms; buttons, links, and fields in a
  header never start a drag. Storage per session is now
  `{cards, topics, w}`; the old bare card array is still read. A pinned
  topic the board lacks after a reload is fetched page by page through the
  closed route; when the route ends without it, the pin is dropped; a failed
  fetch keeps it. New API: `weave` returns `{pins, board}`, `settle(el,
  plan)`, `pinList`, `pinnedTopic`, `relayout`, `dragging`, `move`,
  `decorate`.
- `dashboard.js`: mounts the pinned column `aside#bpcol` in `.dbody`;
  `pinColumnFits` (>= 1600px, a pin, layout not "tabs") drives
  `data-pincol` on the root; `railWide` docks the rail below 2200px only
  without `data-pincol`, so Ctrl+` and the arrow open the overlay there.
  morphBoard reads and grafts pinned topics in the pin list; board-scoped
  reads (answers, chips, topic notes, mentions, held asks, drafts) go
  through `boardBoxes`; a board swap waits while a drag runs; closed pages
  keep a hidden `closed-slot` where a pinned topic stands, so the show-more
  index stays the server's.
- `dashboard.css`: the 1600-2199 and >= 2200 pinned-column grids, the rail
  overlay under `data-pincol` below 2200, the column's own scroll, the
  left-edge width handle (`--pinw`, kept in storage, double-click resets),
  drag line, right-edge zone, empty-group zone, pin button states.
- i18n: `js.pins.title`, `js.pins.drop`, `js.pins.width` in all 11 tables.

Verification:

- Node harness: `TestBoardTopicPins` runs the whole of cards.js over a
  minimal DOM and covers every case the plan lists (pin order among cards,
  hand move undone by a later bump, updated pinned topic keeps index and
  update mark, graft keeps the pressed pin, closed pinned topic stays,
  past-cap pinned topic kept and refetched after reload, gone pin dropped,
  reload restores pins/order/moves) plus the fetched-page decoration and
  the pending-pin order. `TestBoardPinnedClosedPagesCount` runs morphBoard
  with cards.js and fails on 3e922446b (index 2 instead of 3; 3 instead
  of 4). `rail_hotkey_test.go` gains the 1800px-with-pin and 2200px cases.
- `go test ./...` (remote, ws-rgo): all green except the known
  pre-existing `internal/mcp TestDashboardReadPageServesTheBoard`;
  `go vet` clean on `wsdashboard` and `wsi18n`.
- Headless Chromium against a fixture page (6 topics), driven over CDP:
  1500px: no column, pins atop the board in the labeled group, rail
  overlaid; 1800px no pin: rail docked; 1800px with a pin: column at the
  right (480px), rail overlaid, Ctrl+` opens and closes it; 2400px with a
  pin: rail, board, and column all docked, Ctrl+` left alone; "tabs" layout
  at 1800px: no column. Width drag to 600px survived a reload, as did pins
  and a hand move. Drag into the column pinned at the drop index; a header
  drag to the top of the flow moved the topic there; a press on the note
  pen dragged nothing; at 1800px with no pin the right-edge zone showed and
  a drop there opened the column; at 1200px and 1500px the empty group zone
  showed and a drop there pinned. The store page took the grid's cell in
  all six grids (with and without a pin at 1500/1800/2400). No page errors.

Decisions:

- The pinned column sits at the `.dbody` level, as in the mockup, so the
  topic rail (bound to `#bcol`) never sees it; one movable pin list is the
  only node that changes parents on a layout switch.
- Pin button and header drag live in `cards.js`, not `dashboard.js` as the
  plan suggested: cards.js already owns placement and card headers, so
  both kinds share one drag path.
- The pinned column also applies to an ended session page (the same width
  rule; only a live page has the "tabs" menu).
- An unpinned closed topic seats at the head of the closed section; the
  next board swap puts it where the board keeps it.

Review (lite): two Important findings (pages index undercount, fetched
pages without pin buttons) fixed in 71c26a5ab. Unfixed minor: a drop
between two neighbours with equal times lands one slot above, since the
placement rule orders by time.
