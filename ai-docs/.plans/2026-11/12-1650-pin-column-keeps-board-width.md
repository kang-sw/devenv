# Pinned column widens the page instead of shrinking the board

## Outcome

User feedback (2026-11-12, two screenshots) on the just-merged pin column
(261112-feat-ws-dashboard-pin-column, merge a1743525e): when a topic is
pinned, the pinned column takes its width out of the board column, so the
board becomes much narrower than it is with no pins. The user wants the board
to keep, as far as the window allows, the width it has as a single board, and
the page to grow wider by the pinned column instead.

Cause: the page wrapper `.dwrap` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css` (line ~58) caps
at `max-width: 1680px`; the pinned-column grids (lines ~1395-1415,
`:root[data-pincol] .dbody` with `var(--pinw, 480px)`) split that same
1680px among rail, board and column.

Change: while the column shows (`:root[data-pincol]`), raise the wrapper's
cap by the column's width plus the grid gap, so the board column keeps its
no-pin width and only shrinks once the window itself is narrower than the
widened cap. The column's own width (`--pinw`, user-resizable) drives the
extra width, so resizing the column widens or narrows the page, not the
board. The narrow wrapper variant (`.dwrap.narrow`) is unrelated; leave it.

## Boundaries

- CSS only (dashboard.css), plus JS only if `--pinw` is not already set on an
  element the wrapper rule can read (check where cards.js sets it; the
  wrapper is an ancestor of `.dbody`, so a variable set on `.dbody` or
  `.bpcol` is not visible to it; set it on `:root` or the wrapper if needed).
- Do not change when the column shows (the width breakpoints from the
  ticket's decisions (442)-(457)) or the rail behaviour. T0.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test ./internal/wsdashboard/ -count=1`
  (never run heavy go build/test locally).
- Headless Chromium screenshots (see `rail_hotkey_test.go` / the pin-column
  ticket's Result for how the worker built a fixture page; chrome-headless-shell
  is at `~/.cache/ms-playwright/chromium_headless_shell-1223/*/chrome-headless-shell`)
  at 2400px and 2800px wide, with and without a pin: the board column's width
  with a pin equals its width without one when the window is wide enough, and
  shrinks only when it is not. Report the measured board widths.
- Add or extend a test if the existing CSS/layout tests cover the grid rule.

## Return

Commit on `delegate/pincol-width` in worktree
`/home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone` with this plan in
the same commit. Do not merge or push. Report commit, approach, measured
widths, screenshot paths, and test results.
