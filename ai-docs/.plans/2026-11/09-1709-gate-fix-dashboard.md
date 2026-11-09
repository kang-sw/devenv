# Release-gate fix, unit B: wsdashboard

The ws 0.48.13 release gate (lead-review of `11ef849000956f9e0282def355e3298b51f9faf9..889db46fb`) returned NEEDS FIX. User decisions (board, 2026-11-09): (146) fix locally; (147) three parallel worktree units, then re-review; (148) a - JS tests only for the B1/B2 fixes, the other JS coverage gaps become follow-ups; (149) ok + (150) a - the Windows scrollbar restyle is included in this round. This unit owns `internal/wsdashboard`. Unit A (wsledger/host/mcp) and unit C (whiteboard/wsagent/shipped text, including the whiteboard page's scrollbar) run in parallel on other branches; do not edit their files.

Full findings file (read-only reference): `/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/e20afafa-01-findings.md`, section "Unit B".

## Fixes

- **B1** `internal/wsdashboard/assets/tracker.js:1176`: on a 404 `wkLoadDoc` calls `wkStartEdit(path, true, false)`, which acts on the store showing now; if the user switched stores while the read waited, a "new page" editor for store A's path opens in store B's wiki and saving writes it there. Return unless `ui.store === store` (or pass the store through).
- **B2** `tracker.js:1197` `wkRefresh` with busy guards at 1141/1154/1168: a change signal arriving while a read is in flight is dropped and the in-flight read returns pre-write state. Mark dirty when a refresh is skipped and re-run the loader when the read completes.
- **B1/B2 tests** (148 a): node tests for these two fixes using the existing node harness pattern (`relay_test.go`, `tracker_assignees_test.go`). Keep the test focused; extract a small pure helper if that is what makes the logic testable. No other JS test additions in this round.
- **B3** `internal/wsdashboard/attachments_test.go:187` with `daemon_test.go:22`: `TestMessageRouteRefusesBadImages` times out under `-race` (500 ms test client vs a ~13 MB body). Large-body cases use a longer-timeout client (`postJSONWith`, as gitpush_test does).

## Scrollbar restyle (149 / 150)

On Windows the dashboard shows the OS default scrollbars: wide, opaque grey track, out of place on the dark and light themes; on macOS they are overlay and look fine. There is no scrollbar CSS today in `assets/dashboard.css` or `assets/tracker.css`.

- Thin scrollbar, transparent track, semi-transparent thumb derived from the existing theme tokens (light and dark), a little stronger on hover.
- Standard `scrollbar-width: thin` / `scrollbar-color` plus `::-webkit-scrollbar*` rules. Note Chromium 121+ ignores `::-webkit-scrollbar` on an element whose `scrollbar-width`/`scrollbar-color` is set, so pick a combination that gives the intended look on current Chrome/Edge on Windows and on Firefox, and does not regress macOS overlay scrollbars.
- Apply it to every scroller of the dashboard page and the tracker (put it where both pages' styles pick it up once; avoid per-element copies).
- Unit C applies the same look to the whiteboard page (`internal/wswhiteboard/assets/page.css`) independently; match the spec above so the two read alike.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Do not touch the user's dashboard daemons on ports 29417 / 45743; a headless check uses a dev daemon with isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`. Never run `git stash`. Do not push. Do not edit `ai-docs/tickets/`.
- Minors on contact may be fixed (e.g. the `ai-docs/ref/wiki-mockup` path comments in `tracker.css:822` / `tracker.js:1108`, which name a repo-only path in shipped assets); do not go looking for more.

## Verification

`cd agents-plugin-tool && go test ./internal/wsdashboard/...` and `go test -race ./internal/wsdashboard/...` (B3 must pass under -race), then full `go test ./...` once. Headless Chromium screenshot of the dashboard and the tracker with a scrolled panel, light and dark, with the scrollbar visible; screenshots to your scratchpad, paths in the report. One or more commits on branch `fix/gate-0.48.13-dashboard`, this plan committed with the first.
