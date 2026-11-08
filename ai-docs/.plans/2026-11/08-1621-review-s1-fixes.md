# Plan: review sweep S1 fixes

Branch `delegate/review-s1-fixes` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here; do not merge or push.

Source: review sweep segment S1 (`938f55828..fbb6d0a90`, board (446) b).
Full findings: `/tmp/review-sweep/S1-correctness.md`, `S1-fit.md`,
`S1-test.md`. Read `ai-docs/manuals/shipped-surface-boundary.md` before
editing under `agents-plugin-tool/`, and `ai-docs/manuals/ws-mcp.md` for
`internal/mcp/`.

## Fix (each with a test that fails before the fix)

1. **Critical - parser never returns on a truncated closing tag.** Inputs
   such as `<wb:add class="policy" topic="T">\n<wb:title>x</wb:title>\n</wb:add`
   (closing tag missing `>`) or ending in `</wb:` loop forever and grow the
   problem list. Confirmed by the lead on develop HEAD: the parser now lives
   in `internal/wsxmlcmd` (moved by the shared-syntax merge 4b402e9dd), so
   both `whiteboard.update` and `tracker.update` are affected; the reviewer
   located the loop at the old `strayClosing` / `readFields` pair (no advance
   past the bad text, no fatal check). Fix it in `wsxmlcmd` so a malformed
   closing tag is reported once and parsing terminates, with a refusal that
   names the position and line, consistent with the existing refusals.
   Tests: in `wsxmlcmd` for both inputs, plus one through the whiteboard
   command parser.
2. **Important - topic body written in the same batch as a `wb:answer` ranks
   as read.** `internal/wswhiteboard/view.go` ~L394 compares
   `docChanged > answered`; every command in one batch shares the sequence
   number, so the normal one-update-per-turn pattern (answer + new body) never
   marks the body unread. Intended meaning: "changed since the user last
   answered" - use `>=` (or equivalent) and add the same-batch test.
3. **Important - manual drift.** `ai-docs/manuals/ws-mcp.md` ~L194 still
   justifies the public mermaid script by the removed Now frame; the reason
   is now the topic-body doc frames. The same dashboard section omits the
   `ws_dashboard_lang` cookie and the per-language tables
   (`internal/wsi18n/tables/*.json`). Update it. Also fix the stale comments
   `internal/mcp/dashboard_page.go:19` ("Now frame") and
   `internal/wsdashboard/assets/dashboard.tmpl:32` ("language buttons", now
   a `<select>`).
4. **Re-graded Minor (wrong output) - tool summary with zero start.**
   `internal/wslive/toolsummary.go` ~L93/137/153: `startedAt: 0` yields a
   ~56-year duration that is persisted in the ring. Treat a zero start as
   unknown: skip it for the group start and leave `DurationMs` 0. Test it.
5. **Re-graded Minor (wrong behavior) - stale `drIcon`.**
   `internal/wsdashboard/assets/dashboard.js`: `drIcon` set at pointerdown
   survives a drag, so a later keyboard activation of the handle reveals a
   stale entry instead of toggling. Clear it in `drEnd` (or when the gesture
   was a drag). No JS harness exists; verify by reading and, if cheap, a
   headless Chromium check (cached Playwright under
   `~/.npm/_npx/*/node_modules/playwright`).

## Out of scope

Everything else in the three reports (the lead records the rest as
follow-ups), including the JS test gaps for `form.js` and the drawer rework,
the claude-mod tools-row ordering edge case, client `wsT` substitution, CSS
variable names, and `store.go` size.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wslive/ ./internal/mcp/ && go test -count=1 ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wslive/ ./internal/wsdashboard/ ./internal/wstrackerhost/`
plus `go test -count=1 -run 'Whiteboard|Tracker|Dashboard' ./internal/mcp/`.

## Return

Commit hashes, changed files, test output, and anything left open.
