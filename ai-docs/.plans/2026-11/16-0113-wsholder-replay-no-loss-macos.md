# macOS holder replay no-loss fixture repair

## Assignment

MAIN assigned EXEC2 at 2026-11-15T16:12:50Z to diagnose and repair only `TestAConsumersReplayIsNoLoss` in `agents-plugin-tool/internal/wsholder`. It fails on macOS with "timed out waiting for the ticker to outrun its ring", observed at develop 8aebef706 and b0f459708 and on a solo rerun. Relevant merge-agent logs are `/tmp/b7*.log`; inspect only relevant evidence, not broad logs. Earlier Ubuntu/Windows CI was green; a platform timing assumption is a hypothesis, not an established cause.

## Scope and boundaries

- Work from the acquired develop-based branch `fix/wsholder-replay-no-loss-macos` in this full pooled worktree. Start by recording the exact base and inspecting the named test, its holder/ticker implementation and applicable repository manuals. Existing source and tests are ground truth.
- Establish a source-cited root cause and distinguish a test fixture defect from a product defect. Fix only the bounded test/fixture cause with assertions and no-loss coverage preserved. No deleting/skipping, timeout-only raise, retries or sleeps that conceal the failure.
- If correctness requires a product behavior/protocol/API change, STOP before editing it and report evidence. Do not expand to unrelated tests or modules.
- No push, merge, shared daemon swap, CI dispatch, provider or harness launches, installations/downloads, Windows SSH, polling, timers, monitors, or old task queues.
- Read every matching manual declared by AGENTS.md before editing. Keep AI-authored artifacts and code comments in English.

## Required verification

On macOS, with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, run from `agents-plugin-tool`:

1. `go test ./internal/wsholder -run TestAConsumersReplayIsNoLoss -count=20`.
2. `go test ./internal/wsholder`.

Only this named test and the whole wsholder package are authorized; never full wsdashboard/mcp or repository suites. Capture baseline reproduction if practical and exact post-change results. Any added focused check must directly establish the cause or preserved semantics, not broaden scope.

## Delivery

Commit the surgical repair and this immutable original plan with required AI Context explaining cause, approach, rejected timeout-only alternatives, test-vs-product verdict, exact commands/results and material gaps. Return full head SHA, exact base, clean status, changed paths, source file:line citations and remaining limitations. Use concise complete English, normal spacing, short bullets, and VERIFIED/INFERRED labels. MAIN alone integrates; retain lane until its merge report. ETA is 40 minutes from assignment. Do not start another task.
