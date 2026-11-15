# Dev-asset render latency and real-handler coverage

## Assignment

Resolve review follow-up `yoyo-numeric-untreated` within `agents-plugin-tool/internal/wsdashboard` dev-asset tests/code only. The review finding targets `devassets_test.go:111-141`, commit `00d676339`: replacing a real loopback request with `Handler().ServeHTTP` may avoid a timeout without explaining slow dev rendering and loses request-chain coverage.

Measure and name the actual cause of slow dev-asset rendering, distinguishing verified mechanism from original-CI inference. Restore routing/trust-gate/middleware coverage for that dev-asset path, either with a cause-fixed real loopback request or a recorder plus focused real-loopback test, following evidence. No blind timeout bump, skipped/deleted assertions, unrelated fixes, or new externally observable contract. A small T0 product slowness fix is allowed; stop and report if larger or not T0.

Read applicable AGENTS.md and manuals, especially shipped-surface-boundary.md and release-compatibility.md before relevant edits. Do not touch badge popover files or stream.js. Keep evidence outside checkout; commit implementation plus this unchanged plan with AI Context and `resolves: ["yoyo-numeric-untreated"]` through ws/git.commit.

## Verification and safety

Only `-run` devasset tests, `-count=20`, plus `GOOS=windows` vet are authorized; never full wsdashboard or mcp tests, `./...`, native Windows, downloads/providers, shared daemons or port 29417, CI dispatch, push, merge, or runtime swap. Every Go shell invocation including nested calls must explicitly cd to this absolute owned agents-plugin-tool directory and set GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local, with isolated HOME/config/runtime/temp and warm caches. Measure with bounded owned fixtures; do not signal foreign processes or kill outer Go.

## Deliverable

Report head SHA, exact changed files, measured attribution/before-after evidence, source citations, coverage restored, commands/results, material gaps, and evidence path. Keep checkout clean and held until MAIN integration report; lead owns release. No additional review is required for test-only repair; any product-logic change gets at most one bounded independent logic review. Reports are short complete English sentences with VERIFIED/INFERRED labels; no ordinary acknowledgement.
