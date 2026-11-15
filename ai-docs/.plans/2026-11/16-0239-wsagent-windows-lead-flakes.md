# Diagnose and fix two Windows wsagent lead flakes

## Authority and outcome

MAIN assigned one task on 2026-11-15 at 17:39:13 UTC. Diagnose two Windows CI failures in run 43740174813, commit cd477b5a8, first attempt (rerun passed):

1. `TestLeadHeldSendFlushesAtAnUnsolicitedTurnsEnd`: lead_runner_test.go:310, "the submitted message's turn should run"; snapshot Held:1, Status running, Batches:[] .
2. `TestLeadRejoinDoesNotStallBehindALongReplay`: lead_runner_test.go:1595, "1 holder calls waited out their timeout during the rejoin".

Use `gh run view 43740174813 --log-failed` and, if necessary, the explicit first-attempt log option/API to get the actual failed-attempt evidence. Do not dispatch/rerun CI or substitute a passing rerun log. Decide test defect versus product race with evidence, not blind timeout bumps. MAIN integrates; no push or swap.

## Scope and evidence discipline

- Read current tests, runner/holder synchronization and relevant commit rationale. Compare the CI source with the acquired current base so already-landed fixes are not rediscovered as changes.
- Reproduce deterministically in isolated macOS tests using controlled Windows-like scheduling/I/O delays or explicit synchronization, following established race-regression patterns (MAIN cited rail race fix 4f5749f86 as an example, not mandatory implementation).
- If product race: make the smallest correction to existing lifecycle behavior and add a deterministic regression test that fails on pre-fix source. If fixture race: repair its synchronization while retaining the intended assertions. Keep failure evidence, before/after outcomes and exact source rationale outside the checkout. Do not delete/skip tests, weaken guarantees or raise timeouts as a substitute for cause.
- EXEC2 is concurrently editing wsagent runner admission for permission Phase 2a. Keep changes minimal and avoid unrelated admission/profile/permission changes. Report the intended touched files to lead as soon as identified, then include actual paths in final output. Branch overlap is not a reason to broaden scope or integrate another worker's changes.
- Read all applicable conventions before edits, including shipped-surface-boundary and release-compatibility for tooling/internal paths. A public API/protocol semantic change, new version/schema boundary, major architectural change or unrelated failure requiring expansion stops to MAIN; do not settle it silently.

## Authorized verification and safety

- Full `go test ./internal/wsagent -count=1` is explicitly authorized for this task, plus filtered `-race -count=N` repetitions of the two named tests, and `GOOS=windows go vet` scoped to the affected package. Use bounded package timeouts; select repeat count technically and report it.
- Every Go shell invocation, including nested calls, explicitly changes to the absolute owned module cwd `/Users/kang-sw/.ws-worktrees/devenv/bountiful-dress-energy/agents-plugin-tool` and uses `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, isolated HOME/config/runtime/temp and existing warm caches. Read complete verification outputs before claiming success.
- No full MCP/dashboard/all-package suite, downloads, providers, shared daemon/port 29417, foreign keys/signals, outer-Go kills, Electron, Windows host actions, push, merge or swap. Native Windows execution is not requested; cross-vet and deterministic scheduling tests are the current acceptance. Ask lead if native evidence is genuinely necessary.
- For a product-logic correction, obtain at most one bounded independent lite correctness review after a clean checkpoint; fix Important/Critical findings within scope, do not add a broad review or proof matrix. A pure fixture synchronization repair needs only focused verification under current review-track policy. No unrelated follow-up implementation.
- Commit one logical fix with AI Context recording test-versus-product verdict, causal evidence, deterministic pre-fix failure and post-fix verification. Include this unchanged plan. Return full head SHA, root cause per test, touched paths, exact tests/counts/vet output and evidence directory. Keep clean checkout held for MAIN integration; the lead releases it after MAIN's merge report. Next task only MAIN.
