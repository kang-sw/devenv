---
title: "Dashboard: pull-down drawer from the top for Todo, Agenda, Tickets, and Mailbox"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-epic-ws-personal-common-store: the drawer is the frame that store's page tabs will later register into; no dependency either way
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ff04e6dac9d790ef
sage-review-completeness-reviewed: ff04e6dac9d790ef
completed: 2026-11-07
---

# Dashboard: pull-down drawer from the top for Todo, Agenda, Tickets, and Mailbox

## Background

On the daemon-served pages the session's side panels (todos, agendas, the
ticket kanban, the mailbox) render below the board, so on the session page
they sit at the bottom of the board column, which the user found
inconvenient (2026-11-07: "아젠다랑 티켓, 메일박스 같은 탭들인데 이게 맨 아래로
가니까 생각보다 불편해서요"). The user asked for a drawer pulled down from the
top of the page, built as a frame that the planned common TODO platform
(`261107-epic-ws-personal-common-store`) can later add tabs to: "페이지
위쪽에서 잡아당겨서 내릴 수 있는 ... 프레임워크를 만들고(향후 아까 말한 공통 TODO
관리 플랫폼의 기반이 되게) 거기서 잡아당겨 내리는 탭으로".

A whiteboard mockup (2026-11-07, Now slot) drew the closed and open states;
the user confirmed it ("네 그런 느낌이 맞아요").

## Decisions

- **Handle**: the top-right button group (live indicator, settings gear) moves
  left, and a wide rectangular tab shaped like a sticky note hangs from the
  top edge at the far right of the header. Clicking it or dragging it down
  opens the drawer; the tab rides the drawer's bottom edge as it opens.
  Clicking it again or pushing it up closes the drawer, and so do Escape and a
  click outside the drawer. The tab carries small counts (open todos, unread
  mail).
- **Theme, not literal sticky-note colours** (user, 2026-11-07: "포스트잇이라는
  건 모양만 말한거고 테마 자체는 당연히 dim 해야 할 거고요 .. ? 지금 다크
  테마에서 밝게 보여서 이야기했어요"): "sticky note" is the shape only. The
  tab and the drawer use the page's theme variables, dim in the dark theme.
  Rejected: the mockup's bright yellow fill.
- **Overlay** (user, board (105) a): the drawer slides down over the page.
  The conversation, the board, the composer text, and scroll positions do not
  move. This matches the epic's rule that content changes never move another
  panel. Rejected: pushing the page body down by the drawer's height.
- **Contents** (user, (106) a): the four panels (Todo, Agenda, Tickets,
  Mailbox) move into the drawer as its tabs and leave the bottom of the page.
  The board's question items and the Now slot stay in the board column.
  Rejected: keeping the bottom panels as well.
- **Pages** (user, (107) a): the session page and the daemon-served board
  page only. The disk board file (`ai-docs/whiteboards/*.html`, opened outside
  the daemon) keeps its panels as today. Rejected: changing the disk page too.
- **Browser-local state** ((108), announced default from the epic's
  cross-child decision): the open state, the drawer height, and the last
  selected tab persist in localStorage under `ws.dashboard.`. No server-side
  preference state.
- **A frame with registered tabs** ((109), technical): the drawer is built as
  a generic frame. Each tab is a registered entry (id, title, icon, count,
  content region), and the four panels are its first entries, so a later
  ticket adds a tab without reworking the drawer. Rejected: hard-coding four
  tabs into the header markup.

- **Tabs follow the present panels** (announced, from
  `261106-feat-whiteboard-dashboard-panels`: empty panels are omitted): the
  drawer shows a tab only for a panel the render includes. A persisted last
  tab that is no longer present falls back to the first present tab. With no
  tab present, the handle is hidden (technical: there is nothing to open).
  Rejected: a fixed set of four tabs with empty states.
- **Handle counts are read on the page** (technical): after each panels
  region swap the page script reads the counts from the panel markup it
  already received. Rejected: a new head-region field, because it changes the
  server render for figures the page already has.
- **Board-first page** (technical): that page has no gear, so the handle sits
  at the right end after the live indicator, with the same behaviour.

- **Bounded height and an approval cue** (technical, from design review
  2026-11-07): the drawer spans the page width under the header, as the
  mockup drew. Its drag-set height is clamped between about 160px and 60% of
  the viewport, and it never covers the composer. While an approval is
  waiting, the handle shows a waiting cue, open or closed, because an open
  drawer can cover the agents tree's approval row. This keeps the epic's
  approval-visibility rule. Rejected: confining the drawer to the board
  column (it differs from the confirmed mockup, and the tabs layout has no
  board column), and an unbounded height.
- **Tabs are views onto the one panels region** (technical, from design
  review): all four panels still arrive in the single `panels` region. Each
  tab entry selects its panel's `#p-<id>` element in that region, and tab
  presence is read from that markup after each swap. Inside a tab the panel's
  own summary or fold is hidden and its body shown. Rejected: one server
  region per tab, which changes `pages.go`, the SSE stream, and
  `wswhiteboard` for no visible gain.

## Constraints

- The drawer's tab contents keep updating live as the panels do today (the
  `panels` region swap on the session page and the served board page's live
  refresh). A swap must not close the drawer, change its tab, or reset its
  scroll.
- The epic requires the page to keep showing a waiting approval (conversation
  and agents tree) after any layout change; the drawer is a layout change, so
  this holds with the drawer closed and must be checked with it open.
- Implementation notes from design review:
  - The todo chip reads `done/total`, so the open count is computed as
    total minus done. The mailbox chip reads `N unread`.
  - In `dashboard.js` the narrow-layout `TAB` map sends panels to the Board
    tab's dot, and `PANES` / `scrollState()` cover `bcol`. Point them at the
    drawer, so a swap restores the drawer's scroll.
  - Escape closes open menus first (link menu, gear menu). It does not close
    the drawer while the composer has focus, which keeps the composer's
    Escape-then-Tab path.
  - Outside-click detection treats the link menu as inside the drawer.
  - On the board-first page the document scrolls, so the drawer is
    fixed-positioned there.
- Light and dark themes; narrow screens (the tabs layout); no layout shift of
  the header when counts change.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261106-feat-whiteboard-dashboard-panels (2026-11-06, ticket Decisions): "Empty panels are omitted. A ticket panel with no ready or todo tickets, and a mailbox panel for a session with no mailbox address and no history, are left out, as empty todo and agenda panels already are" — bearing: constrains
- 75ff0c2b (2026-11-06, commit): "Region diffing by content hash: each region element carries data-h; the client names held hashes in the stream URL, so a fresh stream or a reconnect sends exactly what changed." — bearing: constrains
- 75ff0c2b (2026-11-06, commit): "A mode pseudo-region (live/board) tells the page its layout no longer fits; the page reloads unless the composer holds text, then shows a banner." — bearing: constrains
- f594e798 (2026-11-07, commit): "Split is a grid with the tree row, a 10px splitter row, and the conversation row sized by --sa/--sb fr shares the page script sets on the grid ... swaps only rewrite region innerHTML, so grid-level state survives them" — bearing: constrains
- f594e798 (2026-11-07, commit): "Saved share is the tree's fraction (ws.dashboard.split); dblclick resets, arrow keys nudge. Input mode key: ws.dashboard.input (enter|mod)." — bearing: supports
- 073fd387 (2026-11-07, commit): "The link menu keys on a renderer-emitted marker, not every a[href], so raw Now-slot anchors and disk-board file: links stay untouched (ticket constraint)." — bearing: supports
- 0336011f (2026-11-07, commit): "internal/mcp/dashboard_page.go is touched to wire FileActions into the served board; the ticket named only wsdashboard and wswhiteboard but the served board is built there." — bearing: constrains
- 783d58c5 (2026-11-07, commit): "the board-first page assertion [in pages_test.go] still guarantees [csend] is absent without an adapter" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl, dashboard.css, dashboard.js, internal/wsdashboard/pages.go, pinned markup tests (pages_test.go, pageview_test.go); possibly internal/wswhiteboard/ and internal/mcp/dashboard_page.go |
| scope.surface | internal | no exported Go symbol named; page markup, CSS, and page script only |
| scope.new_public_symbol | no | none; the registered-tab entry (id, title, icon, count, content region) is page-script internal |
| scope.new_type_contract | no | none in Go; the tab-entry shape is a page-script convention, not a typed signature |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/pages_test.go, pageview_test.go, internal/wswhiteboard/regions_test.go, render_test.go; the drawer's JS behaviour has no automated check and relies on the headless Chromium list |
| complexity.reuse_points | confirmed | the panels region swap (pages.go#L49-L62, RenderRegions in wswhiteboard/render.go), the TAB/layout code and ws.dashboard. localStorage keys in dashboard.js |
| complexity.side_effect_risk | moderate | moves #r-panels out of #bcol, touching the split grid, the narrow tabs layout, and region swaps that must not close the drawer or reset its scroll |
| risk.correctness | moderate | open state, tab, and scroll must survive data-h region swaps and SSE reconnects; counts must not shift the header |
| risk.fit | moderate | overlay drawer interacts with the 1:3 split grid, the narrow tabs layout, and the board-first page that has no gear or adapter |
| risk.test | high | no automated browser test for the page script; verification is a manual headless Chromium matrix across two pages, two themes, two widths |
| risk.security_or_contract | low | no new route, input, or wire shape; only localStorage keys under ws.dashboard. |

## Phases

### Phase 1: Drawer frame and the four panels as its tabs

Build the drawer frame on the session page and the served board page, move
the four panels into it as registered tabs, and remove them from the bottom
of those pages, keeping the disk board file unchanged.

Verification: `go build ./... && go vet ./internal/... ./cmd/...` and
`go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/`
in agents-plugin-tool, with pinned markup expectations updated. Headless
Chromium (cached Playwright under ~/.npm/_npx/*/node_modules/playwright) on
both pages, light and dark, wide and narrow:

- open by click and by drag;
- close by click, Escape, and outside click;
- tab switching;
- persistence across reload;
- a live panel swap while open;
- the page content does not move when the drawer opens;
- counts on the tab;
- the disk board file still shows its panels.

Report what was not checked.

### Result (e12879e10) - 2026-11-07

Landed in `agents-plugin-tool/internal/wsdashboard/assets/` (`dashboard.tmpl`,
`dashboard.css`, `dashboard.js`) and `pages_test.go`. No Go, SSE, or
`wswhiteboard` change: the served board page is the `session` template in
board mode, so both pages share the drawer.

- **Frame:** `drawerTab({id, title, icon, count, badge, content})` in
  `dashboard.js`. The four panels register through `panelTab(id)` as views
  onto the one `panels` region (`#p-<id>`). Presence, title, icon, and chip
  are re-read after every panels swap. A tab whose panel is absent drops
  out, a persisted tab that is no longer present falls back to the first
  present tab, and with no tab present the handle hides.
- **Layout:** the drawer is `position: fixed` on both pages, spans `.dwrap`'s
  width under the header, and is clipped (visibility, never `display:none`).
  The handle hangs from the page's top edge at the header's right end while
  closed and rides the sheet's bottom edge while open. The header reserves
  the handle's room as right padding (`.top.dr-room`). The height is clamped
  to [min(160, room), room], where room is 60% of the viewport cut short of
  the composer with the handle below it.
- **Behaviour:** the drawer opens by click or drag. It closes by click,
  push-up, Escape, or outside click. Escape yields to the link and gear
  menus and to a focused composer, and the link menu counts as inside.
  Persistence uses `ws.dashboard.drawer`, `ws.dashboard.drawer.h`, and
  `ws.dashboard.drawer.tab`. Handle counts are open todos (total minus done)
  and unread mail. A lock cue shows while an approval waits (lead permission
  in convlive, or a permission phase/row in the tree). A panels swap while
  the drawer is closed lights the handle's dot, which replaces the
  board-tab dot in `TAB`. `drbody` joined `PANES`.

Verification:
- `go build ./... && go vet ./internal/... ./cmd/...`: pass.
- `go test -count=1 ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/mcp/`: pass.
  - `pages_test.go` now pins the drawer hooks on both pages and the panels
    region in the drawer, out of `#bcol`.
- Headless Chromium (cached Playwright): a throwaway harness over the page
  fixture with four panels and live bump/approval toggles. It covered both
  pages, light and dark, and 1400px and 420px widths. 178/178 checks
  passed, plus the empty-panels session (handle and header room hidden) and
  the disk board file (`RenderPage`: the 4 panels still in `aside.side`).
  The checks covered:
  - closed handle at top 0, aligned to the header's right edge;
  - open by click and by drag;
  - close by click, Escape, outside click, and push-up;
  - Escape in the composer keeps the drawer open;
  - tab switching;
  - reload persistence of open state, tab, and height;
  - a live panels swap while open keeps it open, keeps the tab, and keeps
    `drbody` scroll;
  - grid, board, convo, and composer rects and scroll offsets, and the
    composer text, unchanged on open;
  - handle counts;
  - the dot on a closed swap;
  - the approval cue closed and open;
  - no page errors.

Not checked: real touch input (only mouse pointer events), Safari and
Firefox, reduced-motion, and a window short enough that the composer room
falls under 160px.

Decisions:
- Header room is padding, not a trailing flex slot. A slot wraps alone on a
  narrow screen and leaves the handle over the first line. On a 420px
  screen the header items do not fit beside the handle, so narrow headers
  now wrap to two lines.
- Narrow screens (<=640px) drop the handle chevron and the tab icons, and
  the tab strip scrolls horizontally.
- Handle counts show only when non-zero. Peer leads' approvals in the tree
  also light the approval cue.
