# Dashboard drawers share one height; the git handle drags

## Goal

User feedback 2026-11-12 (board): the pull-down drawers behind the
top-right handle buttons each open at a different height, which is
distracting; they should all use one height. Also the git (branch) handle
cannot be dragged to resize, unlike the panels handle.

Today (`agents-plugin-tool/internal/wsdashboard/assets/`):

- `dashboard.js` (~1979-2300): the session panels drawer and the git sheet
  share one frame and one height `drH` (stored `ws.dashboard.drawer.h`,
  default `DR_DEF` 320, clamp `drClamp`/`drMax`, which also stays short of
  the composer). Only the panels handle (`#drhandle`) drags; the git handle
  (`#ghandle`, replaced on each git region swap) only toggles on click.
- `tracker.js` (~4924-4975): each tracker store's drawer keeps its own
  height per store (`ui.drh[store]`, stored as `drawer.h` under the
  renderer's `storagePrefix`), and a store never dragged follows its content
  (`fitDrawer`, `tk-measure`).

## Decisions

- One height for every drawer: the panels drawer, the git sheet, and every
  tracker store drawer open at the same stored height, and dragging any of
  their handles sets that one height for all.
- A drawer whose height was never set uses the shared default (`DR_DEF`,
  320px) instead of following its content, so a fresh browser also shows
  one height. Rejected: keeping content-fit for never-dragged tracker
  stores (it is exactly the per-drawer height difference the user dislikes).
- Each drawer still clamps the shared value at display time with its own
  limits (the dashboard drawer's composer bound, the 60% window cap, the
  DR_MIN floor); the stored value is the user's drag result.
- The git handle drags the same way as the panels handle: a press that
  moves past the 5px threshold opens the git sheet (in place of the panels
  if those are open) and resizes; release under `DR_SHUT` closes; a press
  that never moves stays a click (toggle). The handle is looked up per
  event since a git region swap replaces it (use delegated listeners or
  rebind on swap).
- Keep `tracker.js` host-neutral: the shared height reaches the tracker
  renderer through a host option (for example an opts callback pair the
  dashboard passes, or a documented storage key the host names), not by the
  tracker reading dashboard globals. Where no host option is given (another
  host of the renderer), the tracker keeps one height for all its stores
  under its own `storagePrefix`, still not per store. Migrating existing
  stored values is not needed (display state; the first drag after the
  change sets the shared one); if trivial, seed the shared value from the
  dashboard's existing `ws.dashboard.drawer.h`.
- Dashboard asset change only: compatibility tier T0.

## Scope

- `assets/dashboard.js` drawer section, `assets/tracker.js` drawer height
  and drag, `assets/dashboard.css` / `assets/tracker.css` only if the git
  handle needs `cursor: grab` / `touch-action: none` like `.dr-handle`.
- Update the comments at both sites (the dashboard.js block comment says
  "only the panels handle drags"; tracker.js says heights are per store and
  follow content).
- Tests: the existing Go-driven node tests for the drawer and tracker
  (search `*_test.go` in `internal/wsdashboard` for `drClamp`, `fitDrawer`,
  `drawer.h`, `ghandle`) updated, plus cases: dragging the git handle sets
  the shared height and opens the git sheet; a tracker store drag changes
  the height the panels drawer and other stores open at; a never-set height
  is DR_DEF everywhere.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  a page with the real CSS/JS showing the panels drawer, the git sheet, and
  two tracker store drawers open in turn at the same height, and a git
  handle drag; screenshots into scratchpad/drawer-height/.

## Constraints

- Preserve: the session drawer and tracker drawer never show at once
  (ws:drawer-open / tk:drawer-open), open-state persistence of the panels
  drawer only, the `.anim` click animation (never on drag/resize), the
  tracker store page (`tk-paged`) which has no drawer.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/drawer-one-height` in this worktree; do not
  merge or push.
