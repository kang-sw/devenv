# Plan: dashboard - header fixed items to the left, drawer handle icon array, drawer panels as cards

## Outcome

User feedback on the live session page (2026-11-07, screenshot of the top-right
header: count chips, live dot, gear, then the pull-down drawer handle showing
only a chevron). Confirmed on the whiteboard:

1. (131) a - Header layout: the fixed items (the head region's count chips,
   the `live` indicator, the gear menu) sit on the left, right after the
   `Whiteboard` title (no `margin-left: auto` push to the right). The drawer
   handle stays anchored at the top-right corner; future additional handles
   (per-store tabs, not in this change) would stack leftward from the right,
   so do not let the fixed items depend on the handle's width beyond keeping
   them from overlapping it.
2. (132) ok - The handle always shows one icon per present drawer panel
   (todo, agenda, tickets, mailbox; same icons as today's panel title bars),
   in drawer order, with the existing badge counts (open todos, unread mail)
   as a small numeric badge over the icon instead of today's badges-only
   row. The handle width follows its content instead of the fixed
   `--drhw: 116px` (update `.dwrap > .top.dr-room` padding or replace it so
   the header never sits under the handle). Clicking an icon opens the
   drawer and briefly highlights that panel's card (scroll it into view
   inside the sheet if needed); clicking elsewhere on the handle and
   dragging keep today's behavior. Narrow screens (the existing
   max-width media block): keep only icons that carry a badge, plus the
   chevron. Keep the approval-waiting lock and the hidden-tab dot.
3. (133, user: "티켓 | 아젠다 | 메일박스 이 카드들은 탭 방식보단 카드 방식으로
   한 번에 보이는 게 좋겠어요") - The drawer sheet shows every present panel
   at once as cards instead of tabs: on a wide screen side by side in equal
   columns (equal split, not content-sized, so cards do not shift as content
   changes), each card scrolling on its own within the sheet height; on a
   narrow screen stacked vertically in one scrolling column. Remove the tab
   strip and the `ws.dashboard.drawer.tab` preference (ignore a stale stored
   value). An absent panel makes no card (same rule as today's tab
   presence). Drawer open state, height drag, and the stored height keep
   working. Each card keeps its panel title bar (title, icon, count chip).

## Settled constraints

- Files: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`,
  `dashboard.css`, `dashboard.js`, plus Go tests in `internal/wsdashboard/`
  that pin markup (update expectations that change). The served
  whiteboard-only page shares the drawer; check it still renders.
- Keep the `drawerTab({...})` registration frame (later tabs register into
  it); it now drives cards and handle icons rather than tab buttons. Rename
  only if it clarifies; keep the change surgical.
- Use existing CSS variables; light and dark themes.
- No server-side or protocol change. Shipped-surface boundary applies
  (`ai-docs/manuals/shipped-surface-boundary.md`); UI strings stay English
  as today.

## Boundary

Commit on branch `delegate/drawer-cards-header` only; do not merge or push.
Return to the lead if this needs a server or protocol change.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./internal/wsdashboard/ ./internal/wswhiteboard/ ./internal/mcp/`;
`node --check` on dashboard.js. Headless Chromium (cached Playwright under
~/.npm/_npx/*/node_modules/playwright): live session page and board-first
page with all four panels present, light and dark, at 1400px and 420px:
header items sit left after the title and never overlap the handle; handle
shows four icons (badges on todo/mailbox when counts exist; only badged icons
at 420px); clicking an icon opens the drawer and highlights that card; the
drawer shows all cards at once (side by side at 1400, stacked at 420), each
scrolling on its own; height drag and reopen keep the stored height. Report
what was not checked.
