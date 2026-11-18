# Plan: drawer-wait-fold

Close the one Important finding (I1) and the five Minors of the v0.50.7
release-gate delta review of develop 5bab46f66..47d2d0c87 (drawer-fullspan,
0db1f2b0e: drawers span to the composer, riding resize handle hidden while
open; owner request 2026-11-18 13:29).

Findings (authoritative detail, file:line):
`C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/e15b208e-01-drawer-delta-findings.md`

## Scope (agents-plugin-tool/internal/wsdashboard/ and internal/wsi18n/tables/)

- I1: while the panels sheet is open, the approval-wait signal stays visible.
  `drawerWait` (assets/dashboard-drawer.js ~:314-323) also toggles `wait` on
  `#drfold` (the fold button that takes the hidden handle's slot); add a
  `.dr-fold.wait` style matching the handle's existing wait tint and lock
  icon. Keep the `.ag-source` exclusion exactly as is. Pin it with one
  assertion in `frontend-drawer.test.js` (or `testdata/drawer-height.test.cjs`).
- Minors:
  - i18n: replace the stale "click or drag" wording for the drawer handle and
    sheet labels in every `internal/wsi18n/tables/*.json` that carries those
    keys (en.json/ko.json :272,:290 and siblings); click only.
  - `assets/tracker-wiki.js:407`: drop the stale "resizes with the drawer's
    drag" comment wording.
  - `drawer_height_test.go:177`: remove the dead `function drMeasure() {}` stub.
  - Keyboard open: when the drawer (`#drhandle`) or tracker (`.tk-handle`)
    opener hides itself on open, move focus to its fold button (`#drfold` /
    `.tk-fold`) so focus does not fall to body.
  - `assets/dashboard-drawer.js:314-315`: fix the lone "// The tree" comment
    reflow artifact.

## Constraints

- Keep the owner's drawer behavior from 0db1f2b0e: the sheet spans to the
  composer at max size and no resize handle exists while open.
- No production behavior change beyond I1 and the focus handoff.
- Antivirus rule: no local `go test`, Go tools, gofmt, ws/ws-mcp executables,
  launchers, or Electron. Go runs on w5090 only via
  `D:/sys_lock.sh remote w5090 run ...` (w5090 has no Node). Local `node`
  runs of the touched dashboard Node test files are allowed.
- Temp files only under `D:/Temp/drawer-wait-fold/`.
- Embedded assets compile into the binary; no generated bundle changes are
  expected.

## Done when

One commit on `impl/develop/drawer-wait-fold`; `go vet` and
`go test ./internal/wsdashboard/... ./internal/wsi18n/...` pass on w5090
(accepted pre-existing reds only: TestTopicStateBadgeResearchingMetadata,
TestTopicStateBadgeClockAndReconciliation); the touched Node test files pass
locally.
