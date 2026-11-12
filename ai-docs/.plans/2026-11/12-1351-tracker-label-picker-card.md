# Tracker item labels are assigned straight from the property card

## Goal

User feedback 2026-11-12 (board, screenshot of a tracker item window): the
user could not find where labels are assigned. Today labels are assigned only
in the item window's edit mode (`⋯` -> edit, the `labelPickHtml(ed)` chip row
in `agents-plugin-tool/internal/wsdashboard/assets/tracker.js` ~4814, then
save); the property card's label row (`tracker.js` ~2605) only displays them.
Board item (382) answered a: clicking the property card's label row opens the
label picker, and a pick saves immediately.

## Decisions

- The property card's label row becomes a control when the viewer may edit
  the item (the same permission the edit mode uses): click (and Enter/Space)
  opens a popover picker listing the store's labels as the same chips
  `labelPickHtml` renders (selected state shown), anchored to the row.
- Toggling a chip writes the change right away through the same write the
  edit-mode save uses for labels (no separate draft; the edit mode keeps
  working unchanged). Escape or a click outside closes the popover; focus
  returns to the row. With no edit permission the row stays display-only
  (no hover affordance). An empty label set shows a muted "add label" style
  placeholder for editors only.
- If the store has no labels defined, the popover says so (and, for an admin,
  points at where labels are managed if such a place exists; otherwise just
  the message).
- Rejected: menu/context-menu entries and bulk assignment (option b) - not
  requested now.
- Keep `tracker.js` host-neutral (no dashboard globals). Quiet `--wb-*`
  tone, light and dark. New strings through the existing i18n tables (all 11
  locale tables, like neighbouring `js.tk.*` keys; also
  `ai-docs/ref/tracker-mockup/i18n-en.js` if neighbouring keys live there).
- Dashboard asset change only: compatibility tier T0.

## Scope and verification

- `tracker.js` / `tracker.css` property card and picker, i18n tables.
- Tests: extend the node-driven tracker tests (`tracker_quickadd_test.go` or
  the test file covering the item window / property card): the row opens the
  picker only for editors, a chip toggle emits the label write, Escape closes.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  the item window with the picker open and after a pick, light and dark, into
  scratchpad/label-picker/.

## Constraints

- Preserve the edit mode's label chips and save, the label display colours,
  and the known open follow-up ("picker does not reopen after an admin adds or
  renames a label" - fix it only if it falls out naturally, and say so).
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/tracker-label-picker-card` in this worktree; do
  not merge or push.
