# Plan: tracker mockup styling - dashboard tone and masonry cards

Branch `delegate/tracker-mockup-styling` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`). Commit on this
branch; do not merge or push.

Context: the reference mockup `ai-docs/ref/tracker-mockup/` (`index.html`,
`style.css`, `app.js`, `engine.js`, `fixtures.js`; README is the contract)
of epic `261107-epic-ws-personal-common-store`. It was deliberately unstyled
(board (139)) while the data structures settled; they are settled now. The
tracker will live in the dashboard's pull-down drawer, beside the dashboard
pages, so it should look like part of the dashboard.

A parallel delegate changes `engine.js`, README attachment rules, and the Go
port on another branch. Keep this change presentational: `style.css`,
`index.html`, and markup/class changes in `app.js`. Do not change engine
semantics, fixtures' meaning, or the view-state shape.

## Decisions (user, board (347) a, (348) ok, 2026-11-08)

1. **Dashboard tone.** Reuse the dashboard's look: the shared variables and
   component styles of `agents-plugin-tool/internal/wswhiteboard/assets/page.css`
   (variables, panels, chips, buttons) and
   `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css` (top bar,
   gear menu, conversation entries, composer). The mockup is standalone, so
   copy the variables and the rules it needs into `style.css` (with a comment
   naming the source files); do not import from the Go tree at runtime.
   Light and dark both work.
2. **Theme toggle.** Light/dark follows the OS by default, with a mockup
   setting `auto / light / dark` (in the mockup's existing settings or top
   bar area, local UI state in localStorage) applied through a `data-theme`
   attribute on the root, so `auto` keeps the `prefers-color-scheme` rules and
   `light`/`dark` override them. A parallel dashboard change adds the same
   three-way setting to the dashboard gear menu with a `data-theme` root
   attribute; use that attribute name and values so the two stay aligned.
3. **Masonry card grid.** The cards layout becomes a Pinterest-style masonry
   grid: cards keep their natural height and pack into columns with no
   awkward gaps (user screenshot: a short "Assigned by me" card left empty
   space beside the tall Items card). Column count follows the width. Prefer
   CSS columns or a small JS packer over waiting on CSS `masonry`; keep card
   order readable (left to right, top to bottom as far as the technique
   allows) and note the choice in a comment. Fixed-height cards (Support
   agent chat, internal scroll) stay fixed.
4. **Everything else styled, nothing restructured.** Thread sheet, dialogs,
   chips (mentions, attachments, agent provenance, flags), notification card
   unread highlighting, kanban/list/table modes, the handle badge, the drawer
   frame. Layout and card composition stay as they are: the user will discuss
   layout (card set, order, size, density) next, looking at screenshots of
   this styled version.

## Verification

- `node --check` on every mockup script; the page loads with no console
  errors in headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`) for at least three viewers.
- Screenshots to `/tmp/tracker-mockup-styling/`: for viewers maya, ada (admin)
  and one follower, the cards layout in light and dark at about 1280px and
  1680px wide, one thread sheet with mention and attachment chips, one dialog,
  and the theme setting in each of its three states. Check visually that the
  masonry leaves no large gaps.
- Existing behaviour unchanged: notification click and mark-all, the `@`
  picker, attachment load toggle, and a rejected post still work (rerun the
  relevant parts of `SCENARIOS.md` scenario 10).
- `cd agents-plugin-tool && go test ./internal/wstracker/` still passes (the
  golden generator reads `engine.js`; this change must not affect it).

## Return

Commits, changed files, the masonry technique chosen and why, screenshot
paths, test output, and anything left for the layout discussion.
