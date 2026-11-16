# CI: three failures exposed once `internal/wsagent` compiled again

The previous CI runs could not build the `internal/wsagent` test binary because of a `t.Chdir` call; that is fixed. The next CI run (develop, Go 1.23, `GOTOOLCHAIN=local`) shows these failures:

```
--- FAIL: TestPiHostedEventWidget (0.00s)        # ubuntu AND windows: deterministic
    wake_test.go:162: a non-lead must not import hosted wake/count events: [{... BackgroundTasks:0x... Background:0x... Monitors:0x... ...}]
FAIL	internal/wsagent

--- FAIL: TestDaemonRestartReattachesToARunningHolder (0.34s)   # ubuntu only
    holders_live_test.go:102: the closed daemon still holds the holder's attach
FAIL	internal/wsdashboard

--- FAIL: TestLiteralContextNearLinearGrowth/rejected-group-tilde/ordered=false (0.10s)   # windows only
    literal_context_performance_test.go:183: 50020 bytes best-of-3=0s
    literal_context_performance_test.go:183: 100024 bytes best-of-3=5.804ms
    literal_context_performance_test.go:183: 200032 bytes best-of-3=8.9857ms
FAIL	internal/wsxmlcmd
```

## 1. `TestPiHostedEventWidget` (deterministic; likely a real behavior bug)

A non-lead Pi session imports hosted background-task/monitor count events that the test says it must not. Check `git log` on `internal/wsagent` for the recent review repair `repair-idle-pi` (plan `ai-docs/.plans/2026-11/16-2119-repair-idle-pi.md`) and anything else in the range that touched Pi hosted events. Decide from the plans and the `## AI Context` which side is right:
- If the product is wrong, fix the product so a non-lead does not import those events.
- If the repair intentionally changed the contract, update the test to the new contract and quote the plan line that authorizes it.

Do not weaken the test just to make it pass. Reproduce locally first: `go test ./internal/wsagent -run TestPiHostedEventWidget -count=1`.

## 2. `TestDaemonRestartReattachesToARunningHolder` (ubuntu only)

After the daemon closes, the holder still reports the old daemon's attach. Find out whether this is a race in the test, where it does not wait for the holder to observe the disconnect, or a product bug, where close does not drop the attach promptly. Run it with `-count=30` locally. A test that polls for an asynchronous state must wait with a bounded deadline, not assert immediately. A product bug gets a product fix.

## 3. `TestLiteralContextNearLinearGrowth` (timing flake)

This is the same flake pattern as `TestLiteralCodeNearLinearGrowth`, already fixed in `ed342893f`: Windows' timer granularity even gives `0s` for the smallest size. Apply the same amortization: batches of at least 20ms, best of 7, normalized per parse, with GC before each batch. Keep the absolute bounds. Factor a shared helper if both tests now use the same loop. Check the rest of `internal/wsxmlcmd` and the repo for other ratio-based timing tests with sub-millisecond samples, and treat them the same way.

## Verification

- `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:
  - `go test ./internal/wsagent -run 'TestPiHosted' -count=3`
  - `go test ./internal/wsdashboard -run 'TestDaemonRestartReattaches|TestHolder' -count=30`
  - `go test ./internal/wsxmlcmd -count=3`
- Also run the full `./internal/wsagent` package once (allowed), and `GOOS=windows go vet` on the touched packages.

## Common rules

- The module is `go 1.23`: no newer standard-library APIs.
- Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`.
- Do not ssh, do not launch GUI, do not restart the shared daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in commits.
- One commit per fix with `## AI Context`. Commit on your branch only; do not merge. Stop and report if a fix needs a product decision.
- Return: commits, root cause for each with evidence, tests run with results, anything left unfixed.
