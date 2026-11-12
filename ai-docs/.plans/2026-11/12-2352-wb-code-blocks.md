# Plan: whiteboard code blocks get line numbers and a copy button

## Intent

User request (2026-11-12, whiteboard topic `code-blocks`): code blocks in a
topic body and inside items should carry line numbers and a copy button.
The board assets live in `agents-plugin-tool/internal/wswhiteboard/assets`;
the dashboard embeds the same assets, so both pages get the change.

## Change

- **Copy button**: at the top right of a code block, shown on hover. On a
  device without hover (`@media (hover: none)`) it is always shown (619).
  After a click it briefly reads "복사됨" (Korean UI string, as the user
  asked), then returns. It copies the code text only, never the line
  numbers.
- **Line numbers**: a muted left gutter, `user-select: none`, so a drag
  selection never picks the numbers up. Only on blocks of two or more
  lines; a one-line block gets no gutter (618).
- Light and dark themes both; follow the existing `page.css` tokens.
- Works on the file page and the dashboard page, including after a
  dashboard board swap (the dashboard replaces board DOM live; check how the
  existing scripts re-bind after a swap, e.g. the check-items scripts that
  just landed, and do the same).
- Compatibility tier T0: asset content only, no store, command, or MCP
  change.

## Verification

- Go tests in `internal/wswhiteboard` (and `internal/wsdashboard` if the
  embed changes) pass, including the embed size budget test.
- A node mini-DOM test in the style of
  `internal/wswhiteboard/check_form_test.go` pins: a multi-line block gets a
  gutter with one number per line, a one-line block gets none, and the copy
  action copies the code without numbers.
- Headless Chromium screenshots (cached Playwright, see the repo note) of a
  board with a one-line and a multi-line block, light and dark, file page
  and dashboard embed; no page errors. Report the paths.
- Report the exact commands and results.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing.
- Never run `git stash`.
- Commit with `## AI Context` citing whiteboard (618) and (619).
