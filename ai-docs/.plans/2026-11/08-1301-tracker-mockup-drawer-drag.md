# Plan: tracker mockup drawer height set by dragging its handle

Branch `delegate/tracker-mockup-drawer-drag` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

Context: the reference mockup `ai-docs/ref/tracker-mockup/` (README is the
data-structure contract). The previous change (merge `5d41b4201`, plan
`ai-docs/.plans/2026-11/08-1232-tracker-mockup-tabs.md`) made the drawer
height follow its content (`fitDrawer()` in `app.js`, up to
`min(66vh, 700px)`). That contradicts the shipped dashboard drawer
(`ai-docs/tickets/.done/261107-feat-ws-dashboard-pull-down-drawer.md`:
drag-set height clamped between about 160px and 60% of the viewport,
persisted across reload; implementation around the `drag` class in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`). Presentation
only: leave `engine.js`, the fixtures, and the view-state shape untouched.

## Decision (user, board (382) a, 2026-11-08)

The user: "드로어는 애초에 사람이 손으로 끌어내려서 수동 조정하는 높이 패널
아니었나요?" Then (382) a:

1. Dragging the drawer handle sets the drawer height, like the dashboard:
   same clamp (about 160px to 60% of the viewport; match the dashboard's
   numbers if they differ), saved per store as local UI state like the
   existing per-store scope memory, restored on reload. Panels scroll inside
   that height (the Items list and the tab pane each scroll internally).
2. Only while the user has never dragged that store's drawer, the first open
   uses the content-fit height (the existing `fitDrawer()` behaviour, capped at
   the same maximum). Once dragged, the user's height always wins, whatever
   the content or the active tab.
3. Click-to-open/close and the existing handle behaviour stay. The store page
   (`#/<store>`) is full-page and is not affected. Light and dark.

## Verification

- `node --check` on every mockup script; no console errors in headless
  Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`).
- Screenshots to `/tmp/tracker-mockup-drawer-drag/`: first open (content-fit),
  after a drag to a taller and to a shorter height (Items list scrolling),
  after reload (height restored), a tab switch not changing the height.
- `cd agents-plugin-tool && go test ./internal/wstracker/` passes and
  `git diff develop -- ai-docs/ref/tracker-mockup/engine.js ai-docs/ref/tracker-mockup/fixtures.js` is empty.
- README: update the drawer-height line of the layout section and add a
  Settled row for (382) a, noting it supersedes the content-height drawer of
  (366)'s plan.

## Return

Commit, changed files, screenshot paths, test output.
