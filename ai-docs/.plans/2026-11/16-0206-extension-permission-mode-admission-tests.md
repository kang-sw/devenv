# Registered extension permission-mode admission tests

## Assignment

MAIN assigned EXEC2 one test-only task at 2026-11-15T17:06:21Z from review sweep `4be663726..994ea16f5`, item 1 of `/tmp/review-sweep-0501-test.md`. `CreateLead` and `SetPermissionMode` use registered backend native `PermissionModes`, with Pi legacy handling, but lack extension-backend regression coverage.

Add table tests using existing fixture patterns (discovery hints: `registration_choices_test.go:201-238`, `registration_test.go:81-115`, `lead_runner_test.go:592`; verify current source before assuming coordinates):

1. `CreateLead` on a registered fixture backend with a nonempty modes list excluding `plan`, requesting `plan`, is refused.
2. A backend with nil `PermissionModes` rejects a nonempty requested mode at creation.
3. `SetPermissionMode` on the fixture lead accepts listed `review` and refuses unlisted `plan`.
4. `SetPermissionMode` on a Pi lead follows the `ErrUnsupported` branch.

## Scope and safety

- New test code only, preferably a new focused `internal/wsagent/*_test.go` file that reuses existing fixtures. No production edits, fixture rewrites, contract changes, deletions or weakening of assertions. The original immutable plan is also committed.
- EXEC4 is concurrently editing runner code for CSI Phase 2; do not modify that spine or proactively integrate it. Record the acquired develop base.
- If a test exposes a real product defect, STOP with evidence before changing production or hiding the failure. Do not amend permission policy or resume permission-profile runtime phases.
- Read all matching AGENTS.md manuals before edits. Comments, plan and commits are English. Reports use short complete English sentences, normal spacing, VERIFIED/INFERRED labels and file:line citations.
- No full `internal/wsagent` package: MAIN reports baseline-red with 128 stale tests. Never full mcp, dashboard, repository/module suites. No providers, live harness launches, installations/downloads, SSH, network, shared daemon swap, push, merge, CI dispatch, polling, timers or monitors.

## Verification and delivery

On macOS from `agents-plugin-tool/`, run only the new named tests with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsagent -run <exact nonempty selector> -count=1`. Read the full output and verify tests actually ran. Fake registered backends/process fixtures only; prove admission through actual Runner owners, not a substituted validation implementation. Relevant narrow existing fixture selectors may run only when directly needed, never the full package.

Commit the new tests plus this original plan, with AI Context describing the source behavior under test, base, assertions, exact commands/results and limitations. Return the full head/base, changed paths, clean status, source/test citations, test outcomes and any unresolved issue. MAIN alone integrates. Retain the pooled lane until MAIN reports the merge; do not start another task.
