# Plan: review-0507-fixes

Repair the six Important findings of the v0.50.7 release-gate review of
develop 94de8f53f..3171e500c, so the range can be re-reviewed and shipped
(owner (1371)=a 2026-11-18: ship develop now as 0.50.7).

Findings (authoritative detail, file:line):
`C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/00c5c98d-01-findings.md`

## Scope

- I1: update `agpop.hint` / `agpop.pin` in every `internal/wsi18n/tables/*.json`
  to the current Esc/pin behavior (Esc closes even when pinned; outside click
  respects the pin). Keep the behavior from 862a15443.
- I2: update `testdata/agent-popover.browser.cjs` (and `tree-rows.browser.cjs`
  if affected) plus `agent_popover_browser_test.go` comments to the island
  contract (top-right in the convo cell, Esc closes pinned, outside click
  keeps pin).
- I3: Go wrapper for `testdata/agents-island.test.cjs` in the sibling style
  (skip when node is absent); add a pinned-Esc case.
- I4: make `TestViewWithinReadersRedactApart` fail without `-race`:
  credentialed remote + a wiki warning; assert the cached state stays
  unredacted and each returned view is redacted.
- I5: `agProject` compact island labels permission / compacting / idle phases
  correctly (permission visibly distinct), reusing the full panel's labels and
  classes; cover it in agents-island.test.cjs.
- I6: drawer wait signal (`dashboard-drawer.js:328`) no longer fires for linked
  delegate agents in the hidden `.ag-source` block; restore the previous scope
  and pin it with a test.
- Minors only when trivially adjacent (stale "popover" comments in touched
  files, the `<summary>` without `<details>` if touching that template block).

## Constraints

- No production behavior change beyond I5 and I6.
- Antivirus rule: no local `go test`, Go tools, gofmt, ws/ws-mcp executables,
  launchers, or Electron. Go tests run on w5090 only via
  `D:/sys_lock.sh remote w5090 run ...` (w5090 has no Node, so node-gated
  wrappers skip there). Local `node <file>.test.cjs` for the touched dashboard
  Node contract files is allowed. The Chromium acceptance needs
  `WS_PLAYWRIGHT_MODULE`; run it locally only if a Playwright module is already
  installed, never install one.
- Temp files only under `D:/Temp/review-0507-fix/`.
- Embedded dashboard assets are compiled into the binary; no generated bundle
  changes are expected. If any rsrc file changes, regenerate per repo rules and
  keep LF line endings.

## Done when

One or more commits on `impl/develop/review-0507-fixes` close I1-I6;
`go test ./internal/wsdashboard/... ./internal/wstrackerhost/... ./internal/wsi18n/...`
passes on w5090 and `go vet` is clean for touched packages; touched `.test.cjs`
files pass locally under node.
