# Brief: Remove duplicate left magnifying glass icon (`#tkqs`) on dashboard session pages

## Outcome
On dashboard `session` pages (live session, worktree view, and saved conversation history — both standalone and framed inside `shell`), remove the duplicate left magnifying glass search button (`#tkqs` in `#qsslot`) in the top-right header while keeping the right search-drawer handle (`#shandle`) and all search keyboard shortcuts (`Ctrl+K`, `Ctrl+Space`, `Cmd+K`, `/`) working cleanly.

Preserve `#tkqs` on the `index` page (and isolated tracker fixtures without a drawer/searchHost), because `index` has no `#drawer` or `#shandle` and `#tkqs` is its only search button.

## Input Paths
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard-tracker.js` (`tkMount`, `qsSlot`, `searchHost`)
- `agents-plugin-tool/internal/wsdashboard/assets/tracker-mount.js` (`WsTracker.mount`, `#tkqs`, `#tki-search`)
- `agents-plugin-tool/internal/wsdashboard/assets/tracker-render.js` (`qOpen`, `qClose`, `$('tkqs')` attribute/focus handling)
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js` (`shHead`, `shFrameInit`, `shSearch`, `#qsslot` mirroring)
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard-drawer.js` (`#shandle`, `drRoom`)
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` (`session`, `shell`, `index`, `qsslot`)
- `agents-plugin-tool/internal/wsdashboard/assets/overview.js`
- `agents-plugin-tool/internal/wsdashboard/assets/tracker.css` and `dashboard-menus.css`
- Tests to inspect and update as appropriate:
  - `agents-plugin-tool/internal/wsdashboard/pages_test.go`
  - `agents-plugin-tool/internal/wsdashboard/shell_test.go`
  - `agents-plugin-tool/internal/wsdashboard/shell_search_test.go`
  - `agents-plugin-tool/internal/wsdashboard/frontend-geometry.test.js`
  - `agents-plugin-tool/internal/wsdashboard/testdata/worktree-page.browser.cjs`
  - `agents-plugin-tool/internal/wsdashboard/testdata/file-card-state.browser.cjs`
  - `agents-plugin-tool/internal/wsdashboard/testdata/tracker-projection.browser.cjs`

## Settled Constraints
- Keep `#tki-search` symbol definition in `tracker-mount.js` (still referenced by `tracker-panel.js` and `tracker-menu.js`).
- Keep `#tkqs` working on the `index` page where `#drawer` / `#shandle` / `#sbody` do not exist.
- On `session` pages where `#shandle` / `searchHost` exists, do NOT mount `#tkqs` into `#qsslot` (so neither standalone session pages nor `shell` header mirroring show the left magnifying glass icon).
- Make sure `tkMount()` still mounts `WsTracker` when `qsSlot` is omitted/null (do not early-return on `!slot` when `searchHost` is present).
- Guard every `$('tkqs')` lookup in `tracker-render.js` (`qOpen`, `qClose`, `aria-expanded`, focus restore — fallback to `#shandle` or safe no-op when `#tkqs` is absent) and `O.qsSlot` in `tracker-mount.js` so no `TypeError` occurs.
- Keep `Ctrl+K`, `Ctrl+Space`, `Cmd+K`, `/` shortcuts and `shSearch()` working in standalone session, shell-framed session, and index pages.
- Compatibility tier: T0 (dashboard UI asset change only; no MCP tool, store, or wire protocol change).
- Surgical change only; do not refactor unrelated dashboard/tracker/drawer code.

## Permitted Actions
- Read and edit files under `agents-plugin-tool/internal/wsdashboard/` inside worktree `/Users/kang-sw/.ws-worktrees/devenv/blah-rinsing-macaroni`.
- Run Go tests (`go test ./internal/wsdashboard/...`) and JS/browser tests for `wsdashboard`.
- Commit the completed change on branch `impl/develop/remove-dup-search-icon` via `ws/git.commit` with `expected_branch: "impl/develop/remove-dup-search-icon"` and `## AI Context`.
- Do NOT merge into `develop` or push.

## Verification
1. Run `go test ./internal/wsdashboard/...` in `agents-plugin-tool` and confirm all tests pass.
2. Run JS unit test (`node --test agents-plugin-tool/internal/wsdashboard/frontend-geometry.test.js` or equivalent) and confirm it passes.
3. Run browser tests if the Playwright test runner works in this environment (check how `testdata/*.browser.cjs` are executed in `wsdashboard` tests).
4. Verify via tests that:
   - Session pages (and shell-framed session pages) do not render `#tkqs`, while `#shandle` and keyboard search shortcuts open/close search cleanly without JS errors.
   - Index page still renders `#tkqs` and opens search cleanly.

## Boundary to Return
Return a `delegate-report` once the change is verified and committed on `impl/develop/remove-dup-search-icon`, or immediately with `blocked` / `ask-user` if you find an unexpected contract conflict (e.g., if `shell` header or any page relies on `#tkqs` in a way that conflicts with keeping `#shandle` as the sole session search button).
