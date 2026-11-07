---
title: "Dashboard: pull-down drawer from the top for Todo, Agenda, Tickets, and Mailbox"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-epic-ws-personal-common-store: the drawer is the frame that store's page tabs will later register into; no dependency either way
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

## Constraints

- The drawer's tab contents keep updating live as the panels do today (the
  `panels` region swap on the session page and the served board page's live
  refresh). A swap must not close the drawer, change its tab, or reset its
  scroll.
- The epic requires the page to keep showing a waiting approval (conversation
  and agents tree) after any layout change; the drawer is a layout change, so
  this holds with the drawer closed and must be checked with it open.
- Light and dark themes; narrow screens (the tabs layout); no layout shift of
  the header when counts change.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`);
  `internal/mcp/` edits follow `ai-docs/manuals/ws-mcp.md`.

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
