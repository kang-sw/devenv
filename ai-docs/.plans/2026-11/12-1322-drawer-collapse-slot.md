# An open drawer leaves a collapse button in its handle's slot

## Goal

User feedback 2026-11-12 (board, screenshot of the dashboard with a tracker
store drawer open: the store's handle `mylocaltest` rides the drawer's bottom
edge, and the top handle row `+ · · develop ≡` shows an empty gap where that
handle sat): the gap the handle leaves behind ("이빨 빠진 자리") should hold a
button that folds the drawer back up, which reads better than an empty slot.

Today (`agents-plugin-tool/internal/wsdashboard/assets/`):

- Tracker store drawers (`tracker.js` / `tracker.css`): `.tk-handles` is a
  row-reverse flex row; an open store's `.tk-handle.open` moves down with
  `transform: translateY(...)`, so its layout slot stays empty.
- The session panels drawer (`#drhandle`, `.dr-handle`) and the git sheet
  (`.r-git .gh`) are absolutely positioned and ride the sheet's bottom edge
  through `style.top` (`dashboard.js` ~2205-2215), emptying their slot at
  the top too.

## Decisions

- Every drawer whose handle rides down (tracker store, panels, git) shows,
  while open, a collapse button in that handle's original slot: the same tab
  shape as the handles (34px, `border-radius: 0 0 8px 8px`, card background,
  `--wb-shadow-hi`), a chevron-up icon (lucide, through the existing icon
  helper), muted colour and the handles' hover state; it keeps the slot's
  width so neighbouring handles do not shift. Quiet `--wb-*` tone, light and
  dark.
- Clicking it closes that drawer exactly like clicking the open handle (the
  same close path, with the `.anim` click animation). Keyboard reachable
  (a real `<button>`), with a tooltip / aria-label through the existing i18n
  tables ("접기" / "Collapse" style; every locale table updated like
  neighbouring keys). Drag on it is not required.
- It appears and disappears in step with the drawer opening/closing
  (including drag-open, release-under-`DR_SHUT` close, and the session vs
  tracker drawer mutual exclusion via ws:drawer-open / tk:drawer-open); it
  never shows on the tracker store page (`tk-paged`).
- Keep `tracker.js` host-neutral (no dashboard globals).
- Dashboard asset change only: compatibility tier T0.

## Scope and verification

- `dashboard.js` / `dashboard.css` drawer section, `tracker.js` /
  `tracker.css` handle row, the i18n table that holds the neighbouring
  dashboard/tracker strings.
- Tests: extend the existing Go-driven node tests for the drawers
  (`drawer_height_test.go` and the tracker drawer tests): the collapse
  button exists in the slot only while the drawer is open, clicking it
  closes the drawer, and it is absent on the tracker store page.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  a page with the real CSS/JS showing a tracker store drawer, the panels
  drawer, and the git sheet open in turn with the collapse button in each
  slot, light and dark, and the drawer closed after clicking it;
  screenshots into scratchpad/drawer-collapse/.

## Constraints

- Preserve: the shared drawer height and drag (31bee5440, 62304312b), the
  handle ride-down, the wait (approval) tint on `.dr-handle`, the git
  handle's per-swap replacement, the `.anim` animation only on clicks.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/drawer-collapse-slot` in this worktree; do not
  merge or push.
