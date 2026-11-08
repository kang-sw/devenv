# Plan: dashboard gear menu theme setting (auto / light / dark)

Branch `delegate/dashboard-theme-toggle` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/renewed-paving-daffodil`). Commit on this
branch; do not merge or push.

Read `ai-docs/manuals/shipped-surface-boundary.md` before editing (everything
here ships under `agents-plugin-tool/`). UI strings go through the dashboard
i18n tables (`internal/wsdashboard/lang.go` and its tables; `i18n_test.go`
checks coverage).

## Request (user, 2026-11-08)

"톱니바퀴에 다크모드 / 화이트모드 전환 버튼도 있으면 좋겠네요." Today the
dashboard pages follow the OS only: the colour variables switch under
`@media (prefers-color-scheme: dark)` in
`internal/wswhiteboard/assets/page.css` (inlined into dashboard pages) and
`internal/wsdashboard/assets/dashboard.css`.

## Decisions (lead, technical)

1. A three-way theme setting in the gear menu (`{{define "gear"}}` in
   `internal/wsdashboard/assets/dashboard.tmpl`): auto (follow the OS, the
   default and today's behaviour), light, dark. It is shown on every page that
   has the gear, including the list and board-first pages where the gear holds
   only the language item, in the style of the existing gear sections.
2. Applied as a `data-theme="light|dark"` attribute on the root element
   (absent for auto). The CSS keeps the `prefers-color-scheme` rules for auto
   and adds overrides so `light` beats an OS dark preference and `dark` applies
   on an OS light preference, for every variable block that has a dark variant
   in both stylesheets. Set `color-scheme` accordingly so form controls and
   scrollbars follow.
3. Persisted in a cookie the server reads, like the language choice
   (`ws_dashboard_lang` in `dashboard.js` and its server-side reading), so the
   server renders `data-theme` from the first byte and there is no flash of the
   wrong theme on load or on region swaps. Changing it applies immediately
   without a reload (just set the attribute and the cookie).
4. The standalone whiteboard HTML file written outside the daemon has no gear
   and keeps following the OS; leave it unchanged.
5. A parallel tracker-mockup styling change uses the same `data-theme`
   attribute name and values; keep them exactly `light` and `dark`.

## Verification

- `cd agents-plugin-tool && go build ./... && go vet ./internal/wsdashboard/ ./internal/wswhiteboard/ && go test -count=1 ./internal/wsdashboard/ ./internal/wswhiteboard/`.
- Add tests in the existing page-render test style: the cookie yields the root
  attribute for light and dark, none for auto or an invalid value; the gear
  renders the theme section; i18n keys exist in every table.
- Headless Chromium (cached Playwright under
  `~/.npm/_npx/*/node_modules/playwright`) against a rendered page or a test
  server: with the browser emulating dark, `light` shows the light palette and
  with the browser emulating light, `dark` shows the dark palette; auto
  follows the emulation; the choice survives a reload. Screenshots to
  `/tmp/dashboard-theme-toggle/`.

## Return

Commits, changed files, test output, screenshot paths, and anything unsettled.
