# Diagnose and fix the two CI failures

## Authority and target

MAIN assigned this single bounded task from develop `f7fa9b5aae0786891b77eef42d7334b70b192922`, CI run `43759101172`. Work on `impl/develop/ci-rejoin-devassets`. CI excerpt: `/tmp/ci-43759101172-failed.log` (private evidence, do not commit it).

## Required investigation

1. Windows rest: `TestLeadRejoinsItsProcessAfterADaemonRestart` fails at `agents-plugin-tool/internal/wsagent/lead_runner_test.go:912`: row has HostPID 4001, HarnessPID 0, MCPOwnerPID 0. Ubuntu and local macOS passed. Prior Windows run `43754236679` at `9001d3456` passed before the rejoin EnsureLead diagnostic patch merged at `8aa2aaed7`. Treat that patch as a prime suspect, not an established cause. Trace Windows process identity/PID creation time, persistence/rejoin ordering, and advisory State.Error writes before process assignment; compare test fixtures and actual platform paths.
2. Ubuntu all: `TestDevAssetsShowEditsWithoutARestart` fails at `agents-plugin-tool/internal/wsdashboard/devassets_test.go:101` awaiting `/s/livesession` headers. Inspect client timeout, page work after entries P4 `b27e2ce71` and tree P3, and rationale/history `8ba64f1ce`, `00d676339`. Fix only a deterministic cause or demonstrably too-tight timeout.
3. Prior Windows rest run `43756112418` at `e7908adc1` was cancelled at 25 minutes without logs. Report any source-backed hang candidate revealed by this investigation; do not assert it caused that cancellation.

## Scope and safety

Default to test-only fixes in the two affected tests/fixtures. Preserve every assertion and its meaning. Product edits require a proven real defect, must remain minimal and T0, and must be explicitly reported to root before expanding beyond the test-only fix. Likely source investigation is within wsagent rejoin/permission and wsdashboard session rendering paths; do not introduce new interfaces or structure. Read matching manuals before edits. No SSH, push, CI, daemon/shared-runtime/live-store operations, providers/downloads/network or unrelated files. Keep full raw logs privately under `/tmp`; public artifacts remain concise and aggregate-only.

## Verification and delivery

Run offline (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`): full wsagent; each of the two named tests with `-count=5`; `-race` on the wsagent rejoin test; `GOOS=windows go vet` and `GOOS=windows go test -c` for wsagent, with the cross-built binary privately under `/tmp`. Never run full wsdashboard or MCP packages. Preserve initial failures and distinguish observed facts from inferred load/platform causes; cross-compilation is not Windows runtime verification. Commit logical fixes and this immutable plan with AI Context on the expected branch. Root will inspect evidence and commission one-pass independent Lite review before HEAD READY. Return exact head, each root cause and file:line evidence, exact commands and private logs, compatibility verdict if relevant, and limitations. Hold the lease until MAIN's merge report; no autonomous follow-on.
