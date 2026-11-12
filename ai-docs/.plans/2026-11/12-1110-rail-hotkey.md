# Plan: Ctrl+` toggles the dashboard session rail

## Goal

User request (2026-11-12, board): when the left session list (the session
rail, `#rail`) is collapsed, Ctrl+` (backtick) opens it, and pressing it again
closes it. It is the control the user presses most, so a keyboard toggle
replaces reaching for the top-left arrow (`#railbtn`).

## Scope

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`, the session
  rail block (~L4440-4500): `railWide()`, `railShown()`, `railOpen(open)`, and
  the existing Escape handler. Add one keydown listener next to them:
  - Match `e.ctrlKey && e.code === "Backquote"` (physical key, so it works on
    a Korean layout too), with no Alt/Meta/Shift, and not while `e.isComposing`.
  - Below the 1600px breakpoint (`!railWide()`), toggle with
    `railOpen(!railShown())` and `preventDefault()`. At 1600px and up the rail
    is a permanent column, so the key does nothing there (do not
    preventDefault).
  - It must work while focus is in the composer textarea or another input.
- Add the hint to the arrow's tooltip/aria label (the `#railbtn` title in
  `dashboard.tmpl` or wherever its label is set), e.g. "... (Ctrl+`)". Keep
  any i18n catalog in step if the label comes from one.
- Out of scope: same-origin topic body frames forwarding the key, focus
  management on open, a wide-mode collapse.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 6 -- test -count=1 ./internal/wsdashboard/...`
  (never run Go build/test locally).
- A headless Chromium check of the real page assets at a width below 1600px:
  Ctrl+` opens the rail, Ctrl+` again closes it, it works with the composer
  focused, and nothing happens at 1700px. Node with
  `require('/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/pw/node_modules/playwright-core')`
  chromium at
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`. Put any
  screenshots in
  `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/rail-hotkey/`.
- If an existing page/markup test asserts the arrow's label, update it.

## Branch

Commit on `delegate/rail-hotkey` in this worktree. Do not merge or push.
