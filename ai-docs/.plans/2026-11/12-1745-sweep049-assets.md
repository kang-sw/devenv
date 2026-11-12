# 0.49 sweep repairs, batch B: dashboard assets (pinned column, drawers, tests)

## Outcome

Fix the batch B findings of the 0.49 pre-ship review sweep (range
62304312b..32b702129). The findings file is
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/05aaa35c-01-findings.md`;
read its batch B section first. All files are under
`agents-plugin-tool/internal/wsdashboard/`.

1. **B1:** the drawers (`assets/dashboard.css` `.dr-in` max-width 1680px ~862,
   `assets/tracker.css` `.tk-dr-in` ~156) must follow the page width when the
   pinned column widens the page (`:root[data-pincol] .dwrap:not(.narrow)`
   rules from 85835c4d4, plan `ai-docs/.plans/2026-11/12-1650-pin-column-keeps-board-width.md`),
   so their handles and the fold button sit at the page's right edge as
   without a pinned column; check the `.top.dr-room` padding too.
2. **B3:** `assets/cards.js` ~810 `fetchMissing` must not delete stored pins
   when the served board is absent or empty (board placeholder, board not yet
   loaded); drop a missing pin only when the closed route answered without it,
   as the pin-column ticket's decision intended.
3. **B4:** `cards.js` ~639 / `dashboard.css` ~1471: the edge-mode pin zone
   (`.bpzone`) must not swallow in-board reorder drops in the 1600-2199px
   layout; keep it a narrow zone at the page's right edge that does not
   overlap the board column (or otherwise make a drop over the board
   reorder), whichever is simplest.
4. **B5 (test):** add a node case in `cards_test.go` for the pinned column
   width: pointerdown/move/up on `.bp-edge` changes `--pinw` in the right
   direction, respects the 50% innerWidth cap and PINW_MIN, saves `w` with
   the pins, double-click resets.
5. **B6 (test):** make `tracker_settings_test.go` `TestTrackerPriorityDialogRows`
   run the behaviour with `runTrackerCases` (labels count from 1 while values
   stay 0-based, typing in the last row appends a row, rows stop at 9)
   instead of matching code text.
6. **M3:** fix the stale comment at `dashboard.css` ~1419 ("handle on its
   left edge").
7. **B2 is pending a user decision** (where the pinned column's width handle
   goes; the right-edge handle cannot widen once the page fills the window).
   Do not change handle placement now; the lead will send the decision as a
   follow-up message to you. Write B5's test so the placement change is easy
   to adapt.

## Boundaries

- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`. Asset-only, tier T0.
- No refactors beyond each item. Never run go build/test/vet locally.
- Headless Chromium is available for layout checks if useful (screenshots of
  the drawer at 1800px and 2400px with a pinned column).

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 10 -- test ./internal/wsdashboard/ -count=1`
  from the worktree root: green.

## Return

Commit(s) on `impl/develop/sweep049-assets` in
`/home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone`, this plan file
in the first commit. Do not merge or push. Report commits, what changed per
item, and test output, then wait for the B2 follow-up.
