# Tree Phase 3 CI contract repair

## Authority and outcome

MAIN assigned a priority unblock at 2026-11-15 19:55:23Z, pausing Phase 4. Produce one small separate head from exact develop base `0b406215ea5fd3b8cb00b721d0cd3e3ee6103104` on `fix/tree-phase3-ci-contracts`. MAIN will inspect and merge it. Do not resume Phase 4 or alter its retained lane.

## Scope

Inspect and reproduce the three reported wsdashboard failures: `TestBoardKeepWiring` (`board_keep_test.go`), `TestHiddenFrameHoldsItsUpdatesUntilShown` (`shell_frame_test.go`), and `TestBoardHistoryHooksSurroundSSESwap` (`whiteboard_bridge_test.go`). Read the actual current line locations, relevant production JavaScript and tests. For each failure decide, with source-backed evidence, whether the old expectation is stale after the intended additive tree-row transport or a real contract regressed. Repair tests without weakening assertions, or make a small production JavaScript repair if needed. In particular verify actual history capture/restore ordering around tree row and full-region swaps, reader anchoring, hidden updates, and disclosure lifecycle rather than assuming the third failure is cosmetic.

Permitted source edits stay under `agents-plugin-tool/internal/wsdashboard/`, principally the three tests and directly implicated JavaScript. No template/CSS/design change, new API semantics, Phase 4 entries implementation, other packages, tooling changes or ticket edits. If more than a small JS/test repair is required, stop with evidence before expanding scope. Directional T0: preserve additive opt-in and full-region compatibility; no stores, MCP, hello or manifest changes.

## Verification

Use cached dependencies only: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, `-count=1`. Reproduce the three tests before edits, then pass them. Run focused `./internal/wsdashboard -run 'TreeRow|Board|Shell|Whiteboard'` and the production Playwright tree-rows test if cached tooling is present. Never run the full dashboard/MCP/module suite. Preserve exact commands, output and failure/root-cause diagnosis in a generated review/evidence artifact. No network, installs/downloads, real provider/harness launch, Windows SSH, shared daemon/board mutation, process signals, push, merge, rebase or CI dispatch.

Read every matching AGENTS/manual before editing. Commit one logical repair with AI Context stating the root cause and why assertions remain strong. Include this write-once plan. Report full head SHA, exact base, clean status, per-test root cause and stale-vs-real verdict, source citations, exact verification outcomes and omissions. MAIN alone merges develop and swaps the daemon. Retain this worktree until MAIN reports integration; no next task.
