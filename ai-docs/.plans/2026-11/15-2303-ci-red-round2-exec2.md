# CI red round 2: EXEC2 ship blockers

## Outcome and inputs

Repair only the EXEC2 section of `/tmp/ci-red-round2-brief.md` for the v0.50.0 ship blocker. Baseline is develop159d1c3e34d92e392af968f072ac8133fffad35e; branch `fix/ci-red-round2-exec2`. Read only relevant portions of `/tmp/ci-43721279531-failed.log` and the affected source/test files. Deadline target is 2026-11-15T14:33Z (30 minutes from assignment).

## Exact scope

- `internal/wsgit/commit_merge_test.go`: TestCommitMergeAdmitsExactIncomingContent, whose newline filename is invalid on Windows.
- `internal/wsinbox/selections_test.go` and directly implicated selections normalization helpers: TestSelectionOrdinaryPrefixDiscovery and TestNormalizeSelectionsUsesOnlyExistingPartsAndLimits, empty normalized selections on Windows.
- `internal/wslive/summary_compat_test.go` and directly implicated historical-source fixture machinery: TestSummaryHistoricalReaderWriterPreservesOriginalBytes, pinned source git read exit128 on Ubuntu/Windows. Trace checkout depth, pinned SHA reachability and path spelling.

For every named test report root cause file:line, introducing commit since release/0.49.1 afe7f1f16, fixture portability versus product-bug verdict, and exact fix. Preserve substantive assertions. Never delete/skip tests, add a runtime.GOOS skip without MAIN approval, or merely raise timeouts. A necessary product behavior/protocol change is a STOP before that expanded change; return evidence, do not implement it autonomously. Fix test portability and self-contained fixture inputs without invented historical provenance.

## Verification and safety

Read matching declared manuals before edits: release-compatibility.md for all internal changes; shipped-surface-boundary.md; ws-mcp.md only if MCP paths become necessary (they are outside the assigned scope). Read windows-dogfood.md for live-host safety. Do not edit other EXEC sections.

Run only named focused tests on macOS with GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local. Verify the Windows names on ssh ki608@192.168.33.6 using a unique disposable folder per run, cloning/copying the exact candidate head. Do not modify C:\Users\ki608\devenv or %APPDATA%\Werkschaft. Never signal PID2640. Kill only exact owned PIDs with CIM creation ticks, never image-name kills. No Electron, provider/harness launch, install/download, shared-daemon swap, push or CI dispatch. Native Windows/SSH focused checks are explicitly authorized for this assignment; broad/full suites are not.

## Delivery

Commit the scoped repair plus this plan, with AI Context stating v0.50.0 ship blocker and T0 fixture-only compatibility unless a different verified tier is raised. Return exact head, clean status, macOS and Windows command/result lines, and the per-test cause/introduction/verdict/fix table. MAIN alone merges develop and changes shared runtime. Retain the worktree until MAIN reports merge; do not start another task.
