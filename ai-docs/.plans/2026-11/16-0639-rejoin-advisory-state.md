# E1: separate rejoin advisory from terminal errors

## Authority and target

MAIN assigned range-review E1 [Important] under the owner's autonomy grant `(1170)=a`. Read E1 first in `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/c639c0e9-01-range-e314-findings.md`; raw correctness report is `/tmp/range-review-e314/correctness.md`. Base `619237f2331c3dcd95a76d34e5da102dc8612ba5`, branch `impl/develop/rejoin-advisory-state`.

## Bounded product fix

The earlier `8aa2aaed7` change placed EnsureLead rejoin advisories into `conv.lastErr`, suppressing the terminal process-exit reason and losing persistent advisory text at turn start. Permission reconciliation also appends distinct errors indefinitely using substring deduplication.

Within `agents-plugin-tool/internal/wsagent/runner.go`, `permission.go`, and their focused recovery/runner tests, keep current advisories separate from terminal error state. Hold or recompute the EnsureLead advisory at State construction, as permission advisories are projected. State.Error must expose a currently failing cause, survive a turn start, and clear when EnsureLead succeeds. Preserve pre-existing terminal errors. Successive distinct advisory diagnostics must remain bounded rather than concatenate into history. Use the simplest complete private implementation; no new protocol/store fields or cross-module interface. T0 is expected and must be supported by unchanged source-backed writers/readers. Do not silently change admission, attachment, lifecycle, successful error clearing, or terminal reason semantics beyond the requested fix.

## Acceptance

1. Open a turn, apply a rejoin advisory, then exit the process: settlement reason is `the backend process exited`, not the advisory.
2. While EnsureLead fails, advisory survives a turn start; after success it disappears.
3. Successive distinct diagnostics do not accumulate without bound.
4. Attach with a pre-existing `lastErr` preserves it.
5. Full `./internal/wsagent`, `go vet`, and `gofmt` checks pass. Run focused regression tests before and after the repair; an offline focused race check is appropriate if supported by the existing fixture.
6. State the compatibility tier, expected T0 with no protocol change.

## Boundaries and delivery

Pi worker only. Read matching manuals and repository context before edits. Never touch `whiteboard.md`, other owners' work, SSH, shared daemon/runtime/live stores, push, CI, providers/downloads/network, or full dashboard/MCP packages. Go runs offline (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`). Keep complete raw logs privately under `/tmp`; do not add public runtime inventories or payloads. Commit this immutable plan with the logical product fix using AI Context and the exact expected branch. Root will inspect outputs and obtain an independent correctness/test/fit-aware review before HEAD READY. Return exact head/base/files, source-backed root cause and tier, exact commands/private logs, and any remaining uncertainty. Hold worktree lease until MAIN's merge report; MAIN sends product heads through its merge agent. No autonomous follow-on.
