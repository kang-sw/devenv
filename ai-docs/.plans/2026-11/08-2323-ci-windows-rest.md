# Plan: ws-mcp CI green on the Windows `rest` shard

Branch `delegate/ci-windows-rest` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`), based on
develop `eb5b241d7`. Commit here; do not merge or push. Do not run git stash.

## Source

CI run 42488770488 (`gh run view 42488770488 --log-failed`) on develop
`eb5b241d7`: every job passed except `go test (windows-latest, rest)`:

- `internal/wsdashboard` `TestStickyFallbackPort`:
  `stickyport_test.go:59: no free preferred port held steady across 10 attempts`
  (the scenario returned false on all 10 attempts, so some step misses
  deterministically on Windows - not a random port steal).
- `internal/wsindex` `TestDiscoveryClearsAbsenceWhenFetchFails`:
  `storage_test.go:809: Read after the failed fetch = unknown, <nil>; want
  fresh from a new discovery`.

## Task

1. Find the root cause of each on Windows. Real Windows is reachable at
   `ssh ki608@192.168.33.6`; you may copy the worktree there (a scratch
   directory) and run the two tests with
   `go test -count=1 -run <name> ./internal/<pkg>/`. Candidate causes to check,
   not conclusions: a port left in TIME_WAIT by the daemon's own connections
   blocking a rebind on Windows (Go does not set SO_REUSEADDR there); the
   `unreachableURL` (`/nonexistent/...`) or the fetch failure path behaving
   differently on Windows; a state file write/rename racing on Windows.
2. Fix the product when the product is wrong on Windows (for the sticky port:
   a daemon that cannot rebind its recorded port after its own restart is a
   product bug worth fixing if that is the cause); fix the test when only the
   test's assumption is POSIX-specific. Do not skip a test on Windows unless
   the behavior is genuinely POSIX-only, and say why in a comment.
3. Keep every other test green.

## Verification

From `agents-plugin-tool`: `gofmt -l .`,
`go vet ./internal/wsdashboard/ ./internal/wsindex/`,
`go test -count=1 ./internal/wsdashboard/ ./internal/wsindex/` on macOS, the
two tests (plus their packages if time allows) on the Windows host, and
`GOOS=windows go vet` on the touched packages.

## Return

Root cause per test (evidence: command output from the Windows host), commit
hashes, changed files, test output (macOS and Windows), anything left open.
