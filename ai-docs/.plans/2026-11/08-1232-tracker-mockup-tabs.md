# Plan: tracker mockup - Items panel + tab pane layout, item window overlay

Branch `delegate/tracker-mockup-tabs` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

Context: the reference mockup `ai-docs/ref/tracker-mockup/` (README is the
data-structure contract; `SCENARIOS.md` is the walk) of epic
`261107-epic-ws-personal-common-store`. The previous layout change (merge
`8838d1b68`, plan `ai-docs/.plans/2026-11/08-1052-tracker-mockup-layout.md`)
gave one Items panel with scopes, quiet rows, hover actions, and a two-zone
drawer whose side column stacks cards. Screenshots of that state:
`/tmp/tracker-mockup-layout/`. Presentation only: leave `engine.js`, the
fixtures' meaning, and the view-state shape untouched (the Go port
`agents-plugin-tool/internal/wstracker` reads `engine.js` through
`testdata/gen-golden.js`).

## Problem (user, 2026-11-08)

"레이아웃이 빈 공간이 좀 너무 많긴 하네요." Card heights follow the drawer,
not their content, so a 3-row Items panel fills the drawer while the side
column's six stacked cards overflow (Notifications alone fills it and pushes
Today below the fold).

## Decisions (user, board (366) b and its note, (362) ok, 2026-11-08)

1. **Layout A + D** ((366) b; wireframe
   `ai-docs/whiteboards/shots/tracker-layout-options.png`, gitignored, view it
   locally):
   - **A (default):** the Items panel on the left (about 2/3), and on the
     right one tab pane instead of stacked cards: tabs Notifications,
     Support agent, Team, Activity, Data (debug) in that order, one visible at
     a time and using the pane's full height. Notifications is the default
     tab. A hidden tab with something new shows a dot, like the dashboard's
     tab bar (`tdot` in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`).
   - **Today becomes summary chips in the Items panel header** (my open work
     N / waiting for my answer N / waiting for my confirm N), always visible
     whatever scope is shown ((358) intent). Clicking a chip narrows the panel
     to those items (a filter within the current presentation; clicking again
     clears it). The Today card is removed.
   - **D (wide screens, about 1600px and up):** three columns, Items |
     Notifications (always open) | the tab pane holding the remaining tabs.
     Switch automatically by width.
   - **Narrow (below about 1000px):** everything becomes tabs, Items first,
     like the dashboard's tabs layout; the Today chips stay at the top.
   - **Drawer height follows content** up to the existing maximum, so a short
     list gives a short drawer; the Items list scrolls inside when longer.
     Remove the per-card 70vh rules that no longer apply; the support agent
     chat keeps its fixed-height behaviour inside its tab.
2. **Item window overlay** ((366) note: "각 태스크를 클릭했을 때에도 창이
   열리는 방식이 좋겠네요"). Clicking an item (row click, Enter, second tap)
   opens it in a window overlay over the page (dimmed backdrop, centred
   window, Esc / backdrop / close button closes, focus moves in and returns),
   instead of today's thread sheet. The window holds what the thread sheet and
   the row detail overlay hold today: title, status, kind, holders,
   assignment path, dates, the body with mentions and attachments, the
   comment thread and composer, and the row's actions as labelled buttons.
   Keep one window at a time; opening another item replaces it. Expose one
   function that opens the window by item id (for example
   `openItemWindow(storeId, itemId)`), because a follow-up will add URL routes
   that open an item from a link; do not build routing now.
3. Keep: the hover actions and their keyboard / touch equivalents, status
   icons (`status-icons.js`), the theme setting, light and dark, and every
   behaviour `SCENARIOS.md` walks (notifications, mentions `@` picker,
   attachments, flagged writes, kanban, timeline, calendar, dialogs).
   The hover detail overlay may stay as a quick preview or be dropped in
   favour of the window; choose and note it.

## Verification

- `node --check` on every mockup script; no console errors in headless
  Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`)
  for maya, ada, lena, and ivy.
- Screenshots to `/tmp/tracker-mockup-tabs/`: maya and ada at 1280 and 1760
  (D) in light and dark; 900px (narrow tabs); a hidden tab with its dot; a
  Today chip filter applied; the item window open (with comments, mention and
  attachment chips) in light and dark; the drawer with a short list showing
  the content-height drawer.
- Rerun SCENARIOS.md 1, 3, 4, 8 and 10 through the new layout and window;
  update step text that names the Today card, the side column, or the thread
  sheet.
- `cd agents-plugin-tool && go test ./internal/wstracker/` passes and
  `git diff develop -- ai-docs/ref/tracker-mockup/engine.js ai-docs/ref/tracker-mockup/fixtures.js` is empty.
- README: update the layout section (tab pane, Today chips, D and narrow
  modes, item window) and add Settled rows for (366) b with its note and
  (362) (the earlier delegate choices: unread dot = unread notification,
  pending holders dashed, my-team scope only with followers, detail overlay
  over rows, store chip removed), noting (366) supersedes (359)'s stacked side
  column and (358)'s Today card.

## Return

Commits, changed files, screenshot paths, test output, SCENARIOS steps
changed, and anything left open (stop and report rather than inventing a
data-structure change).
