# Plan: board-pending-node-tests

The v0.50.7 publish CI gate (ws-mcp CI run 44257680749 on release/0.50.7 @
a63e9ef59, cut 55ba27c0f) failed in `internal/wsdashboard` on both
`go test (ubuntu-latest, all)` and `go test (windows-latest, rest)`. Every
failure is a Node-driven test, which w5090 (no Node) skips, so pre-flight and
all prior w5090 runs never exercised them. Fix forward on develop so the
release can be re-cut.

Failed log: `D:/Temp/ship-0507/ci-failed.log`.

## Failing tests (both legs)

- TestBoardExposureNoHiddenLayoutUnderNode (board_exposure_client_test.go:8):
  `pill.apply = boardApplyPending;` ReferenceError-style failure.
- TestHiddenFrameHoldsItsUpdatesUntilShown (shell_frame_test.go:39) and
  TestPageStreamsRideWebSocket (stream_page_test.go:91):
  `if (ev.name === "board" && boardReturnTimer) boardReturnFresh = true;`.
- TestBoardHistoryHooksSurroundSSESwap, TestBoardTopicsClientHiddenAndRecovery,
  TestBoardViewedCrossTopicInsertion, TestBoardViewedPlacementAndQueuedCounts
  (read their messages in the log).

## Likely cause (verify)

The UI peer's board-pending merge 5bab46f66 (478597365 auto-apply held board
updates on session return; 54c6caa96 pull-in-place pill button) added symbols
(`boardApplyPending`, `boardReturnTimer`, `boardReturnFresh`, ...) that the
Node test harnesses, which evaluate extracted slices of the dashboard assets,
do not define. Determine for each failure whether it is a harness gap (the
harness must define or load the new symbols) or a real production defect
(e.g. a symbol used in a script that can run before or without the file that
declares it). Fix production code only for a real defect; otherwise fix the
harnesses. Keep the board-pending behavior intact.

## Constraints

- No local `go test`, Go tools, gofmt, ws/ws-mcp executables, launchers, or
  Electron (antivirus). w5090 has no Node, so these tests skip there: reproduce
  each failing test's Node script locally with `node` (extract what the Go
  test feeds node, under `D:/Temp/board-pending-node-tests/`), fix, and rerun
  it locally until it passes. Then run `go vet` + `go test
  ./internal/wsdashboard/...` on w5090 for compile/regression safety.
- Temp files only under `D:/Temp/board-pending-node-tests/`.
- Scan the rest of `internal/wsdashboard` for other Node-gated tests the same
  range broke (any test that shells out to node) and run them locally too.

## Done when

One commit on `impl/develop/board-pending-node-tests`; every listed test's
Node script passes locally; w5090 go vet + go test wsdashboard green
(accepted pre-existing reds only: TestTopicStateBadgeResearchingMetadata,
TestTopicStateBadgeClockAndReconciliation).
