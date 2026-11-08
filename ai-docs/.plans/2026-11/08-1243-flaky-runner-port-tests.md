# Plan: de-flake TestPiBackendStart and TestStickyFallbackPort

Branch `delegate/flaky-runner-port-tests` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch; do not merge or push.

## Problem (lead, 2026-11-08)

Under full-suite load (`cd agents-plugin-tool && go test ./...`), two tests
failed once each and pass in isolation (`-count=5` / `-count=3`):

- `internal/wsagent/pi_test.go` `TestPiBackendStart` (line ~466): the fake pi
  process's args file was missing when the test read it after a ~3s start
  wait. Likely a fixed timeout or a read before the child wrote the file.
- `internal/wsdashboard/stickyport_test.go` `TestStickyFallbackPort` (line
  ~50): "port in use". Likely the test picks a free port, closes it, and
  expects it to stay free (a TOCTOU race with other parallel packages).

## Decisions (lead, technical)

1. Fix the tests' synchronisation, not production behaviour, unless the
   failure exposes a real product race; if it does, stop and report with
   evidence before changing production code.
2. Wait on a condition (poll until the file exists with a generous deadline,
   or wait for a ready signal the code already provides), not a longer fixed
   sleep. For the port test, keep the listener the test reserved where
   possible, or retry with a fresh port on bind collision, whichever keeps
   the test's assertion intact.
3. Keep each test's asserted behaviour unchanged.

## Verification

- `go test -count=20 -run 'TestPiBackendStart' ./internal/wsagent/` and
  `go test -count=20 -run 'TestStickyFallbackPort' ./internal/wsdashboard/`
  pass.
- Under load: run `go test -count=1 ./...` from `agents-plugin-tool` twice
  (or run the two targets with `-count=10` while `go test ./...` runs in
  parallel); report results honestly, including any other flake seen.
- `go vet` on the touched packages.

## Return

Commit, changed files, root cause of each flake with evidence, test output.
