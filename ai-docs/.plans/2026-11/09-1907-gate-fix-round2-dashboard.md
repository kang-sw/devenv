# Release-gate fix, round 2 (dashboard): wiki sync warnings visible, B1 recovery hole

The ws 0.48.13 release gate re-review (range `11ef849000956f9e0282def355e3298b51f9faf9..f7de37da9`, after fix units A/B/C) found two defects a user can hit, graded Important by the gate's re-grade rule. The user authorized autonomous fix rounds until the gate clears (board, 2026-11-09). A parallel round-2 delegate on branch `fix/gate-0.48.13-round2` adds tests in `internal/mcp`, `internal/wsledger` (test files: restore/wiki push), and `internal/wstrackerhost` (wiki save timeout test); avoid editing those test files.

## D1: wiki sync failures are invisible on the dashboard

Round 1 (A3/A4, commits ed75b8d3a, ad11abc68) turned wiki push and fetch failures into warnings so they no longer abort the ledger sync. The dashboard used to show them only because they aborted the sync (as the store's sync error). Now:

- `cmd/ws-mcp/tracker_host.go:250-268` (`syncReport`) and `internal/wsdashboard/tracker_routes.go:299` (`TrackerSyncReport` has no warnings field): the dashboard's check and sync routes drop every warning.
- `internal/wsledger/remote.go:282-285` with `internal/wstrackerhost/poll.go:86-89`: the wiki fetch-failure warning is queued in the one-shot pending-warnings queue, and the poller's next Fetch, whose report it discards, usually drains it. So a wiki namespace that cannot be fetched looks stale on the dashboard with no signal; a refused local-only wiki push shows only as a warning on saves to other wikis.

Outcome: while a wiki fetch fails or a wiki's push is refused, the dashboard shows that as a visible warning for the store (redacted with `RedactCredentials`), and it clears once the wiki syncs again. Suggested shape (pick the simplest that meets the outcome): keep the wiki fetch failure as a standing warning like `wikiWarns` (re-reported on each sync until a fetch succeeds) rather than one-shot, and forward wiki warnings through `syncReport` into a new `TrackerSyncReport` warnings field that the tracker UI shows in the store's sync status area, using existing styles and i18n keys pattern (add the en key and every other table's key, as `TestOtherTablesHoldEveryEnglishKey` requires). MCP `tracker.sync` behavior must stay as it is (it already reports these). Tests: Go test that a failing wiki fetch surfaces in the dashboard sync report across polls and clears after success; a refused wiki push surfaces too.

## D2: B1's recovery defeated by the wkDone re-run

`internal/wsdashboard/assets/tracker.js:1149` (wkDone Again path) with `:1184`: a non-soft read for a missing page (a `wiki:` link) is in flight; a change signal sets `docAgain`; the user switches stores before the 404 arrives; B1 correctly skips the editor, but `wkDone` then calls `wkReread(..., 'doc')`, a soft read of the hidden store's page that 404s and sets `docErr`; when that store shows again `wkPull` skips the read because `docErr` is set, so the user sees "not found" where a new-page editor should open. Fix: do not re-run when `ui.store !== store` (or leave `docErr` unset for a soft 404 while `st.doc === null`). Extend `TestTrackerWikiReadsFollowStoreAndChanges` (`tracker_wiki_test.go`) with this sequence; confirm it fails before the fix.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing; `ai-docs/manuals/ws-mcp.md` before editing `cmd/ws-mcp`.
- Do not touch the user's dashboard daemons on ports 29417 / 45743; a headless check, if you make one, uses a dev daemon with isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`. Never run `git stash`. Do not push. Do not edit `ai-docs/tickets/`.

## Verification

`cd agents-plugin-tool && go test -count=1 ./internal/wsdashboard/... ./internal/wsledger/... ./internal/wstrackerhost/... ./cmd/ws-mcp/...` (read the output, do not hide the exit code behind a pipe); `go test -race` on changed packages. One or more commits on branch `fix/gate-0.48.13-round2-dashboard`, this plan committed with the first.
