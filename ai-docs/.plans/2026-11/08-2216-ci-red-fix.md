# Plan: make ws-mcp CI green on Linux and Windows

Branch `delegate/ci-red-fix` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/foe-breath-retying`), based on develop.
Commit here; do not merge or push. Do not run git stash.

Source: the lead is preparing a release tonight (user pre-approved shipping,
2026-11-08). The release gate needs a green `ws-mcp CI` run
(`.github/workflows/ws-mcp-ci.yml`: ubuntu `all` plus Windows shards `rest`,
`mcp 0..3 of 4`, via `agents-plugin-tool/scripts/ci-test-shard.sh`). Recent
develop runs failed; read the logs with
`gh run view 42483195091 --log-failed` (and earlier runs from
`gh run list --workflow ws-mcp-ci.yml -L 10`).

Known failures in run 42483195091:

- ubuntu: `internal/wsgit` TestWorkingTreeCountsUnmerged.
- windows mcp shards: TestWhiteboardMarkdownFileLinks,
  TestWhiteboardTicketPanel, TestDashboardInboxDrainClaimsEachEntryOnce.
- windows rest: TestClaudeArgsFromProfile, TestClaudeBackendStart,
  TestClaudeProcessControls, TestPiArgsAllOffByDefault, TestPiArgsFromProfile,
  TestPiBridgeRegistersOnlyTheProfileServers, TestPiBackendStart,
  TestCreateWritesRecordRegistrationAndSnapshot, TestRunnerSuiteOnPi,
  TestNonZeroExitIsFatalWithStderrTail, TestFatalKillsTheChild, and any
  others the log lists.

## Task

1. Diagnose each failure from the log. Decide per failure whether the test
   or the code is wrong. A real product bug on Windows (path separators,
   process handling, line endings) is fixed in the code; a test that assumes
   POSIX (shell scripts as fake binaries, `/` paths, signals) is fixed in
   the test, using a Windows-capable fake or a `runtime.GOOS` skip only when
   the behaviour genuinely does not exist on Windows (say why in a comment).
2. Linux failures reproduce locally: fix and run the package.
3. Windows: the smoke host `ssh ki608@192.168.33.6` may be used for real
   Windows runs (see the clone note infra.windows-smoke-host). Otherwise use
   `GOOS=windows go vet ./...` and careful reading. You may also verify on
   GitHub Actions by pushing your branch `delegate/ci-red-fix` to origin and
   running `gh workflow run ws-mcp-ci.yml --ref delegate/ci-red-fix`; the
   branch push is allowed, never push develop or main.
4. Keep changes surgical; one commit per logical fix group with
   `## AI Context`.

## Verification

- Locally: `cd agents-plugin-tool && gofmt -l . && go vet ./... && go test -count=1 ./...`
  (read the full output; flaky tests TestPiBackendStart and
  TestStickyFallbackPort under load are known — fix them too if the cause is
  clear).
- A green `ws-mcp CI` dispatch on your branch is the target; report its run
  id and every job's conclusion.

## Return

Commit hashes, per-failure cause and fix, local test output summary, the CI
run id and conclusions, and anything left red with the reason.
