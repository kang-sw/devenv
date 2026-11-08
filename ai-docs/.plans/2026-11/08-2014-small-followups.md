# Plan: small follow-ups bundle

Branch `delegate/small-followups` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop. Commit here, one commit per item; do not merge or push. Do not run
git stash. Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
under `agents-plugin-tool/` (every string the MCP tooling emits ships), and
`ai-docs/manuals/ws-mcp.md` for `internal/mcp/`.

## Items

1. **Whiteboard stale-topic nudge omits the required `result`.** The info
   line ends "if finished, close them with wb:close." (source in
   `internal/wswhiteboard`, see `nudge_test.go` ~L43/L64 and
   `internal/mcp/whiteboard_tools_test.go` ~L655), but `wb:close` now
   refuses a close without `result`. Reword so the agent reading it writes a
   valid close the first time, e.g. `close them with wb:close and a one-line
   result.` Keep it one sentence; update the tests.
2. **Stale package comment.** `internal/wsxmlcmd/wsxmlcmd.go` line 2 says
   the syntax is used by "(whiteboard.update today)"; tracker.update uses it
   too. Fix the comment to not name a dated caller list (or name both).
3. **Ticket path text.**
   `ai-docs/tickets/todo/261007-bug-session-record-cross-process-write-race.md`
   names the old `keys/<key>.json` record. The record is now split into
   `<cache>/sessions/<key>/{key.json, agenda.json, todos.json,
   whiteboard.json}` (see `internal/mcp/session_layout.go`, merge
   `c9f642e17`). Update the ticket's description of the race to the current
   layout without changing its decisions; note that `enterMode` now writes
   agenda and todos as two files, so cross-process atomicity across those
   two parts is weaker than before. Ticket text is English; follow
   `ws/convention.read(name: "ticket-conventions")`.
4. **Default priority names in English** (user, board (489) a).
   `internal/wstracker/state.go` ~L24 `defaultPriority` names are Korean
   (`알림`, `업무`, `긴급`). Change them to English: `Notice`, `Task`,
   `Urgent` (same order, same default index 1). Keep parity with the mockup
   engine `ai-docs/ref/tracker-mockup/engine.js` (and its fixtures / README
   rows if they state the default names) and update every golden or test
   that pins the names. Stored stores that wrote explicit priority names in
   genesis are unaffected; only the no-priority default changes.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wstracker/ ./internal/mcp/ && go test -count=1 ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wstracker/ ./internal/wstrackerhost/ ./internal/mcp/`
plus `node --check` on any changed mockup JS. Note: two wswhiteboard tests
(`TestFileTextRendersFileTargetsAsPathText`,
`TestRenderPageEmbedSizeBudget`) may fail only on the unmerged drawer branch,
not on develop; on develop they should pass.

## Return

Commit hashes, changed files, test output, anything left open.
