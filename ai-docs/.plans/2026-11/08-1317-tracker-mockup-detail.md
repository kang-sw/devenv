# Plan: tracker mockup detail pass - icon tabs, search, closed items, full-height panels, notification focus

Branch `delegate/tracker-mockup-detail` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

Context: the reference mockup `ai-docs/ref/tracker-mockup/` (README is the
data-structure contract; `SCENARIOS.md` is the walk). Previous passes: merge
`5d41b4201` (tab pane layout, item window, store page, quick switcher; plan
`ai-docs/.plans/2026-11/08-1232-tracker-mockup-tabs.md`) and merge
`eb9087915` (drag-set drawer height; plan `08-1301-tracker-mockup-drawer-drag.md`).
Presentation only: leave `engine.js`, the fixtures, and the view-state shape
untouched (the Go port `agents-plugin-tool/internal/wstracker` reads
`engine.js` through `testdata/gen-golden.js`). If a change below seems to need
a data-structure change, stop and report instead.

Out of scope here: member names and avatar initials. A separate later pass
changes the roster to a structured profile; keep `initials()` as it is.

## Decisions (user, board 2026-11-08)

User: "레이아웃은 이 정도면 잘 닫혔다고 봅니다. 이제 슬슬 디테일 목업을."
Current header (screenshot): row 1 is `Items` + scope pills (mine / received
requests / assigned by me / my team / all) + an "N open" chip; row 2 is text
pills list / kanban / due timeline / calendar. The right pane's tab bar
(Notifications, Support agent, Team, Activity, Data) wraps onto two lines.

1. **Icon tabs and view icons** ((384) ok). The right pane's tabs become icon
   buttons (one icon each, inline SVG, no external library), with the name in
   a tooltip and `aria-label`; the active tab stays highlighted and the "new"
   dot stays. The same icons serve the narrow (<1000px) all-tabs layout and
   the D (>=1600px) layout. The Items panel's view switch (list or tree /
   kanban / due timeline / calendar) becomes icon buttons the same way, to
   free width in row 2.
2. **Search in row 2** ((385) ok). Keep the two-row header. Row 2: the view
   icons, then a search box taking the remaining width. It filters the current
   scope client-side as you type, matching title, body, project, and item id;
   it combines with the Today chip filter; Esc clears it. It also searches the
   closed-items section below.
3. **Closed items at the bottom** ((386) b). Done items leave the main list
   and appear in a "recently closed N" section at the bottom of the Items list,
   greyed, expanded by default, covering the last 14 days (by close time, or
   the closest field the view state already carries; note what you used);
   older ones are reached through search. Its expanded/collapsed state is local
   UI state per store, like the existing per-store scope memory. Kanban keeps
   its done column; timeline and calendar keep showing ended items muted.
4. **Tree view and closed items** ((387) a). The tree stays a tree. Closed
   children among open siblings collapse into one "closed N" summary row under
   their parent, expandable on click. A closed parent with open descendants
   stays in the tree, greyed; a closed parent whose whole subtree is closed
   moves as one unit to the "recently closed" section. The parent's roll-up
   ("done 3/5") stays.
5. **Panels span the drawer height** ((389) ok; user: "카드가 그냥 쭉
   늘어나게요. 비워놓으니 약간 미완성처럼 보여서"). The Items panel and the
   right pane (and the Notifications column in D) always fill the drawer's
   height, whatever their content; long content scrolls inside each panel as
   today. The "recently closed" section sits at the bottom of the Items list
   inside that height. The drag-set drawer height behaviour ((382) a) stays.
6. **Notification click focuses its target** ((390) ok). Clicking a
   notification opens the item window and scrolls to and highlights what the
   notification refers to, found through the event id the notification
   already carries: a comment or mention -> that comment (brief background
   flash, then a thin left mark stays); a status, close, or confirm request
   -> that assignee's status line in the header; an assignment or handoff ->
   the holders line; a body edit -> the body; an attachment -> its chip.
   Extend the existing opener to `openItemWindow(storeId, itemId, {focus:
   eventId})` so a later URL route can reuse it. If the target is not found,
   just open the window.
7. Keep everything else: hover actions and their keyboard / touch
   equivalents, status icons, theme setting, light and dark, store page,
   quick switcher, item window, every behaviour `SCENARIOS.md` walks.

## Verification

- `node --check` on every mockup script; no console errors in headless
  Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`)
  for maya and ada.
- Screenshots to `/tmp/tracker-mockup-detail/`: the header at 1280 and 1760
  in light and dark (icon tabs, view icons, search box); a search applied; the
  recently-closed section; a tree with a "closed N" row and a greyed closed
  parent with open children; a short list showing both panels filling the
  drawer; a notification click with its comment highlighted in the item window
  (light and dark); 900px narrow layout.
- Rerun SCENARIOS.md steps touching notifications, the Items views, and the
  item window; update step text that names the text tabs or view pills.
- `cd agents-plugin-tool && go test ./internal/wstracker/` passes and
  `git diff develop -- ai-docs/ref/tracker-mockup/engine.js ai-docs/ref/tracker-mockup/fixtures.js` is empty.
- README: update the layout section (icon tabs, search, recently closed,
  tree closed rows, full-height panels, notification focus) and add Settled
  rows for (384)-(390).

## Return

Commit, changed files, screenshot paths, test output, the close-time field
used for "last 14 days", SCENARIOS steps changed, and anything left open.
