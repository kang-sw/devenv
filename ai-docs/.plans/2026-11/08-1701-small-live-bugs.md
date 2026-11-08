# Plan: small live bug bundle (board (460) a)

Branch `delegate/small-live-bugs` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/snowy-limpness-smartly`), based on
develop. Commit here, one commit per fix; do not merge or push.

Approved on board (460) a (2026-11-08). Evidence per item:
`/tmp/idea-triage/2605-2608.md` (260814) and `/tmp/idea-triage/2609.md`.
Read `ai-docs/manuals/shipped-surface-boundary.md` before editing shipped
files and `ai-docs/manuals/ws-mcp.md` for `internal/mcp/`. Each source
ticket's confirmed decisions are the contract; do not widen scope.

## Fix

1. `260814-bug-release-workflow-duplicate-runs-race-asset-upload`: add a
   `concurrency:` group to `.github/workflows/ws-mcp-release.yml` so duplicate
   runs for one release do not race the asset upload, per the ticket.
2. `260913-bug-plugin-json-migration-vocab-uncovered`: remove the
   "Codex-first" wording from `agents-plugin/.codex-plugin/plugin.json` (and
   the wsflow copy if it carries it) and widen `TEXT_TREES` in
   `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py` as the
   ticket's confirmed decision says; fix any other hit the widened check finds
   only if it is the same vocabulary, else report it.
3. `260913-bug-mailbox-tests-read-ambient-ws-mailbox-env`:
   `setupMailboxTestEnv` (`internal/mcp/mailbox_tools_test.go`) must clear
   `WS_MAILBOX` and `WS_MAILBOX_AUTO` (via `t.Setenv`) so tests do not depend
   on the caller's environment. Show a test failing with them set before the
   fix.
4. Pi bridge tool count (part of
   `260916-research-pi-ts-suite-outside-release-ci-contract-rot`, absorbing
   the dropped `260915-bug-pi-bridge-bundled-tool-count`):
   `agents-plugin-pi/test/bridge.test.ts` asserts 65 tools while
   `agents-plugin-pi/runtime.json` has 72 (`LIVE_TOOL_NAMES` lacks the tracker
   tools). Bring the test in line with `runtime.json`, preferring a derivation
   from `runtime.json` over a new hard-coded number if the test structure
   allows it. Do not add CI wiring (the research ticket stays open for that).

## Tickets

Close 260814, 260913-plugin-json and 260913-mailbox as done with a one-line
reason. Leave 260916 open and append a dated status line that the count
drift is fixed and CI wiring remains. Use `ws/tickets.close` with session key
`clumsy-dinner-fax` (this worktree's worker key) and confirm with
`git status` it acted in this worktree; otherwise `git mv` there after a
`## Closed (2026-11-08)` line.

## Verification

- `cd agents-plugin-tool && gofmt -l . && go test -count=1 -run Mailbox ./internal/mcp/`, also with `WS_MAILBOX=x WS_MAILBOX_AUTO=1` set.
- `python3 -m pytest agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`.
- In `agents-plugin-pi`: the bridge test (`npx vitest run test/bridge.test.ts` or the package's test runner).
- YAML parse check of the workflow file.

## Return

Commit hashes, changed files, test output, and anything left open.
