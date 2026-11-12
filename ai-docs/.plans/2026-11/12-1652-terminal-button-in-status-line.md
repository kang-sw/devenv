# "+ Terminal" button moves from its own header row to the composer's status line

## Outcome

User feedback (2026-11-12, two screenshots): the "+ 터미널" button at the top
right of the board column takes a whole row of vertical space. Remove that
row, and put the button as a small button at the bottom right: the right end
of the composer's status line (the row under the input that shows the folder,
usage `47%/일`, and cost, rendered from `{{region . "sline"}}` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` line 21).

Today `assets/terminal.js` `header(col)` (around line 538) builds a
`div.bhead` holding `button.btn.bnew` (plus icon and the `js.term.new`
label) and the `div.tpop` popover (folder options, a path input, an open
button, an error line), and inserts it as the column's first child.

Change:

- No `div.bhead` row at the top of the column any more.
- The button becomes a small icon button (terminal or plus icon, no text
  label, keep `title`/`aria-label` from `js.term.new.title` / `js.term.new`)
  at the right end of the composer status line; it must not shift the
  status line's existing content.
- The popover opens from the button's new position (above it, anchored to the
  bottom-right, kept inside the viewport), with the same contents, keyboard
  focus, outside-click/Escape close, and open behaviour as today.
- The status line region is re-rendered by the page's live updates (morph);
  make sure the button and an open popover survive those re-renders (mount
  outside the morphed node, or re-attach, whichever matches how the file
  already handles nodes inside morphed regions).
- Pages where the terminal feature is offered but no composer is shown: find
  out whether that case exists. If it does, keep a small fixed button at the
  bottom right of the page for it and say so in the report; if not, say that.

## Boundaries

- Asset-only (terminal.js, dashboard.css, dashboard.tmpl if a mount point is
  needed). T0. No change to terminal open/list routes or the card itself.
- Another delegate is editing dashboard.css concurrently (pinned column
  width, `.dwrap` / `:root[data-pincol]` rules); keep your CSS edits away
  from those rules to avoid conflicts.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test ./internal/wsdashboard/ -count=1`
  (never run heavy go build/test locally); update tests that assert the old
  `.bhead`/`.bnew` structure.
- Headless Chromium screenshot (chrome-headless-shell at
  `~/.cache/ms-playwright/chromium_headless_shell-1223/*/chrome-headless-shell`)
  of a fixture page showing the status line with the button, and one with the
  popover open, if a fixture can be built the way earlier dashboard tests do.

## Return

Commit on `delegate/term-button-sline` in worktree
`/home/swkang/.ws-worktrees/devenv/lapped-override-viewing` with this plan in
the same commit. Do not merge or push. Report commit, approach, screenshot
paths, and test results.
