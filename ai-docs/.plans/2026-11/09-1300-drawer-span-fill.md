# Dashboard drawer: stretch cards down into the empty last-row cells

## Request

The user (2026-11-09, screenshot): the dashboard drawer (the "hamburger"
panel, `#drawer` / `#drbody`) lays its cards out in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css` (~L459:
`#drbody { display: grid; grid-template-columns: repeat(auto-fit,
minmax(min(300px, 100%), 1fr)); grid-auto-rows: minmax(280px, 1fr); ... }`).
When the cards wrap, the last row is partly empty (e.g. 5 cards in row 1,
"설정" alone in row 2, the rest of row 2 blank). They want the cards that can
span to extend down so the lower area is not empty.

## Outcome (whiteboard (67), (68), (69), all ok)

- (67) JS computes the grid's current column count and the empty cells of
  the last row, and sets `grid-row: span 2` on the card directly above each
  empty cell, so that card fills the hole. `grid-auto-flow: dense` is
  rejected because it reorders cards. Recompute on a ResizeObserver of
  `#drbody` (or the drawer), on region swaps that re-render drawer cards,
  and when the drawer opens; clear previously applied spans before each
  recompute.
- (68) Any card qualifies for the span.
- (69) The <=759px layout (flex column) and the git view are unchanged:
  no span is applied there (clear spans when the grid is not active).
- Read the column count from the computed `grid-template-columns` (number
  of tracks), not from guessed widths. Cards are in DOM order with no
  explicit placement today; if any card already spans, account for it or
  skip the fill rather than produce overlap.

## Boundaries

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.
- Drawer JS lives around `dashboard.js` ~L1272 (`drawer`, `drclip`,
  `drsheet`, `drbody`). Keep the change local to the drawer.
- Verification: `node --check` on edited JS; `go test ./internal/wsdashboard/`.
  No JS harness: reason through 6 cards in 5 columns, 6 in 4, 6 in 3
  (no empty cells), 1 column, and the <=759px mode in the commit's AI
  Context; state that the browser check was not done.
- Commit on branch `drawer-span-fill` in this worktree; do not merge or push.
  Never run `git stash`.
