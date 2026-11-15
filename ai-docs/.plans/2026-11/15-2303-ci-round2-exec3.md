# CI round 2: EXEC3 ship blockers

## Authority and scope

MAIN assigned this single bounded v0.50.0 ship-blocker task at 2026-11-15 14:03:23Z. Read /tmp/ci-red-round2-brief.md in full and execute only EXEC3. Read /tmp/ci-43721279531-failed.log for the named failures. Base is develop, at least 159d1c3e3; establish exact base before edits. Primary files are agents-plugin-tool/internal/wsagent/lead_handoff_test.go, lead_runner_test.go and internal/mcp/process_env_test.go, with narrowly required fixture helpers. Read applicable repository instructions/manuals before editing. No other EXEC sections.

## Outcome

1. Check whether already-landed b96654037 fixes TestLeadHandedToATerminalIsLetGo on Windows; do not duplicate its change.
2. Diagnose and minimally fix TestLeadRejoinDoesNotStallBehindALongReplay on Windows if it is a fixture portability/timing issue.
3. Run TestHostedReconnectAdoptsOnlyLiveSamePIDRecords locally with -count=20, identify its cause and minimally fix a fixture issue if warranted.
4. For every named test, provide cause file:line, introducing commit since afe7f1f16e75bd2f86187568ae89a9f2298e1aec, fixture-versus-product verdict, and fix or existing coverage. Product behavior/protocol changes are STOP/report boundaries, not authorized implementation.

## Verification and safety

Use focused -run only, never full internal/mcp, full dashboard, or full Go suites. Offline Go: GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local. Preserve every initial failure and exact command/output; no blind reruns, test deletion, skips (including GOOS skips), or timeout increases. No providers, downloads, installations, credential access, push, CI dispatch, shared daemon/runtime operations, stash mutation, unrelated files or process signals. An existing cached Linux image may be used only network-none/pull-never with isolated HOME/provider state.

Explicit exception to the prior no-network rule: SSH to ki608@192.168.33.6 is authorized only for the required Windows focused tests. Use a unique remote folder per run; copy this head or fixtures without modifying C:\Users\ki608\devenv or %APPDATA%\Werkschaft. No dependency downloads. Never signal PID 2640. Any termination must be PID-specific with CIM creation-tick identity verification; never kill by image name. Never launch Electron on the Mac. Read relevant existing Windows procedure before remote execution. Windows verification needs actual ok lines for both wsagent names; macOS focused verification needs all three names on the final head. Keep any unavailable platform/toolchain evidence as a gap, not a pass.

## Delivery

Target ETA 14:33Z / 23:33 KST. Return a clean commit with this plan and AI Context explaining v0.50.0 ship-blocker rationale and T0 for test-only work. Include full report and command/log paths plus the required per-test table, exact HEAD/base/changed files and verification limits. MAIN alone merges develop, operates shared runtime, pushes or dispatches CI. Hold the worktree lease until MAIN reports final merge. Stop before scope expansion; no follow-on tasks. Reports use short, complete plain English sentences with VERIFIED/INFERRED labels and useful file:line citations. Report blockers or ETA slips over 30 minutes immediately.
