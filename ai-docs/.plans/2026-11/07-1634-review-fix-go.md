# Plan: review-sweep repair, Go side (9768d4607..f1c811464 NEEDS FIX)

## Outcome

The range review sweep `9768d4607..develop` returned NEEDS FIX. Fix the
following findings, recorded in full (file:line, scenario, suggested fix) in
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/57c9501f-01-findings.md`.
Read that file first; the IDs below refer to it.

- I1 clickjacking: add `Content-Security-Policy: frame-ancestors 'none'` and
  `X-Frame-Options: DENY` to every daemon-served HTML response (index `/`,
  session page `/s/<id>`, served board page, file-view pages). Test on `/` and
  `/s/<id>` at least. Do not add a broader CSP (M1 stays a follow-up).
- I2 + M5 + m2a + M6 doc drift: `ai-docs/manuals/ws-mcp.md` (ring keeps 2000
  on disk, live view 50, older paged; drained/ keeps the newest 50 per
  `wsinbox.DrainedCap`; correct the live-snapshot git-wait statement);
  `dashboard.port` description in `agents-plugin/`, `agents-plugin-wsflow/`,
  `agents-plugin-pi/` `dashboard-config-manifest.json` gains the sticky
  last-port step (keep the three in sync; wsflow differs only in defaults);
  fix the `live_snapshot.go` header comment (M6).
- M7: cache the `dashboard.enabled` resolution in `live_snapshot.go` so a
  transient resolve failure does not stop the session and rebuild empty
  `wslive.State` (keep last known value on failure).
- M8: scrub session keys from mail titles in the ring and from `MailBodies`
  before they reach the browser, using the existing scrubber in
  `wslive/text.go`.
- M9: ring entry files and dirs owner-only (0600 / 0700), matching mail history.
- M10: a `permission{clear:true}` or status-less agent event for an evicted
  subagent must not recreate it as running (`wslive/state.go`).
- m2b: `sendMessage` must not sweep attachments its own `req.Refs` reference
  (sweep after resolving refs and exclude them, or age by last reference).
  Test: cancel an old held message, re-send, images survive.
- m2e: `fullDocumentPattern` in `wswhiteboard/store.go` must not refuse a Now
  that mentions `<html>`/`<!DOCTYPE>` inside a markdown code span or fence.
- m2d: `wsgit.BranchSync` branch name via `git symbolic-ref --short -q HEAD`
  with the existing detached fallback; test a branch shadowed by a same-name tag.

## Settled constraints

- Files: `agents-plugin-tool/` (wsdashboard, cmd/ws-mcp, internal/mcp,
  wslive, wsinbox, wswhiteboard, wsgit and tests), `ai-docs/manuals/ws-mcp.md`,
  the three `dashboard-config-manifest.json`. Do not touch
  `agents-plugin/hooks/`, `agents-plugin-pi/src/`, or `config-manifest.json`
  files (another delegate is editing them in parallel).
- Out of scope, held for a user decision: `wswhiteboard/assets/pill.js`
  label (I5) and Now HTML isolation on served pages (m2f). Do not change them.
- Accepted as follow-ups, do not fix: M1, M2, M3, M4, m2c.
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`);
  read `ai-docs/manuals/ws-mcp.md` before editing `internal/mcp/`.

## Boundary

Commit on branch `delegate/review-fix-go` only, one commit per logical unit
is fine; do not merge or push. Return to the lead if a fix needs a protocol
or product decision beyond the above.

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`;
`go test -count=1 ./...` (the known `TestStickyFallbackPort` port-race flake
and the pre-existing `-race` timeout of `TestMessageRouteRefusesBadImages`
may be ignored if they are the only failures; say so). `-race` on wslive,
wsinbox and the wsdashboard message tests. A regression test per finding where
it is testable. Report what was not checked.
