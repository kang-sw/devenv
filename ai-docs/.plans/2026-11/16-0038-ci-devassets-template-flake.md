# Post-ship v0.50.1: dev-assets template recovery CI flake

## Outcome and scope

Do ONLY EXEC2 of `/tmp/ci-flakes-round4-brief.md`: `TestDevAssetsReportABrokenTemplateAndRecover` in `agents-plugin-tool/internal/wsdashboard/devassets_test.go`. Ubuntu CI run43729697102 timed out awaiting GET `/s/livesession?frame=1` headers at line127. Relevant failed log is `/tmp/ci-43729697102-failed.log`. This is post-ship v0.50.1 work, not a v0.50.0 ship blocker. Base develop4be663726eb2cc6fa9779b8f55b152f76c3a2c00; branch `fix/ci-devassets-template-flake`. Target 40 minutes from 2026-11-15T15:38:15Z, i.e.16:18:15Z.

Diagnose relevant dev-assets template rebuild/recovery and test-client synchronization using this test, its helpers, and directly implicated asset/template lock/rebuild implementation. Find the actual race or fixture defect, distinguishing test timing from product locking; never infer a root cause merely because the client has a short timeout. Previous runs and macOS count5 passed near-identical code. Preserve broken-template error and subsequent recovery assertions.

## Boundaries

Fix portably within this named test and directly implicated test helpers. A necessary product behavior/protocol change is a STOP: return cited cause and proposed resolution before modifying product code. Do not delete/skip assertions or tests, only increase timeouts, add blind sleeps, or change unrelated EXEC3 wsagent work. Prior round3 approval for a scoped inbox POST budget does not authorize a generic deadline change here.

Read declared manuals before edits: release-compatibility.md for internal code and shipped-surface-boundary.md for package changes. No new skills or workflow semantics. Worker is Pi; lead owns coordination. No pushes, daemon swaps, CI dispatch, installs/downloads, provider/harness launch or full-package tests. Never run full wsdashboard or mcp. Do not acquire or modify another lane.

## Verification and delivery

Use GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local and the exact test `-run '^TestDevAssetsReportABrokenTemplateAndRecover$' -count=20`; run `-race` on the same bounded test if cheap. Reproduce independently on local Linux only if an existing offline container/runtime is available without downloading or launching the shared product runtime; otherwise explicitly state Ubuntu was not run. Do not expand the package selector to chase unrelated failures.

Commit one logical repair plus this immutable plan with AI Context identifying post-ship v0.50.1 scope, cause and fixture/product verdict, rejected timeout-only approach, and verification. Return exact clean head, cause file:line, source-based explanation of the synchronization fix and verification commands/results/gaps. Preserve source/line evidence rather than treating a passing run as proof. MAIN alone merges and deploys. Retain the worktree until MAIN's merge report, then the lead releases it. No next task.
