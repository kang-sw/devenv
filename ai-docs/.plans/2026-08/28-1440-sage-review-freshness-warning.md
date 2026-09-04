# Plan: sage-review freshness warning

## Relevant Ticket Contract
- Implement a surgical Git-inferred freshness detector: when a ticket has a completed design or completeness Sage stamp and the ticket content changed afterwards, `tickets.sage_gate` returns `check_review_required` instead of `skip`.
- Report the affected stage(s), a review baseline, and an instruction to inspect the diff and decide whether to rerun that Sage stage.
- Any content diff triggers; do not infer meaning from the body. Mechanically exclude only Sage-owned posture fields and pure ticket path/status moves.
- Add non-blocking equivalent warnings to `tickets.verify` and `git.commit`.
- Do not add a ticket, durable ledger, new review snapshot field, automatic rerun, automatic reset, or merge to `develop`.
- Test committed post-stamp edits, uncommitted edits, no-ticket-touch commits, status-only move, and per-stage behavior.

## Out of Scope
- No semantic review of ticket prose or body meaning.
- No new persisted review-baseline field, durable ledger, snapshot field, automatic Sage rerun, or automatic posture reset.
- No change to `tickets.sage_stamp`'s current posture-writing/blocked-section behavior except where tests need fixtures.
- No hard commit block for the new freshness warning; `tickets.verify` and `git.commit` warnings remain non-blocking.
- No source, test, or doc edits outside the implementation task; this survey writes only this plan.

## Codebase Findings
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L118-L187` — `SageGate` already resolves the ticket path, category/stage requirements, and terminal `completed`/`skipped` skip branches; insert the freshness check before returning `skip` for completed terminal stages.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L42-L60` — `SageGateResult` currently exposes `Action` values `skip|stop_blocked|ask|run` plus reviewer/mode/advisory fields; add fields for `check_review_required`, affected stages, baseline, and instruction.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L407-L430` — `effectiveSageReviewPostures` maps `sage-review-design`, `sage-review-completeness`, and legacy `sage-review` into effective stage values; reuse it to decide which completed stage(s) are freshness candidates.
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L62-L99` and `#L123-L187` — `TicketVerify` is the non-mutating guardrail aggregator with soft warnings in `VerifyResult.Warnings`; add freshness warnings here so standalone `tickets.verify` sees the same advisory condition.
- `agents-plugin-tool/internal/mcp/server.go#L2604-L2651` — `verifyAdapter` converts `TicketVerify` warnings to `git.commit` text advisories without blocking; adding a `VerifyResult.Warnings` entry is enough for MCP `git.commit` and the CLI mirror to surface the warning.
- `agents-plugin-tool/internal/wsgit/git.go#L495-L530` — commit verification runs after staging and before commit, using delete-side-filtered paths. This naturally catches staged ticket content changes and skips pure delete-side/status transitions when nothing remains to verify.
- `ai-docs/mental-model/mcp-runtime.md#L104-L105` — `wsdoc` must not import `wsgit`; if it needs Git read state it should shell out to `git` directly, with a filesystem-first gate where possible. A small wsdoc-local Git helper is consistent with this rule.
- `agents-plugin-tool/internal/wsdoc/project_tree.go#L99-L118` — existing `wsdoc` precedent shells out to Git without importing `wsgit`, supporting a local freshness helper that uses `git -C <root> ...`.
- `agents-plugin-tool/internal/mcp/server.go#L2653-L2687` — `formatTicketVerify` already renders warnings and emits a PASS-with-warnings next instruction; freshness warning text should fit this existing shape.
- `agents-plugin-tool/internal/mcp/server.go#L2689-L2695` and `agents-plugin-tool/internal/mcp/tickets_sage_test.go#L48-L110` — `formatSageGate` and its tests pin the result text contract; add rendering/tests for `action: check_review_required` and the required stage/baseline/instruction lines.
- `agents-plugin-tool/internal/mcp/tickets_verify_test.go#L102-L145` and `agents-plugin-tool/internal/mcp/server_test.go#L1896-L1968` — existing parity tests prove soft warnings surface through both `tickets.verify` and `git.commit`; extend or mirror this pattern for freshness warnings.
- `ai-docs/spec/mcp-tools.md#L1353-L1380` and `#L1767-L1793` — current spec says `tickets.verify` warnings are non-blocking and `git.commit` surfaces them as text-mode advisories; implementation should preserve this behavior.

## Implementation Plan
1. In `agents-plugin-tool/internal/wsdoc/tickets_sage.go`, extend `SageGateResult` with fields such as `FreshnessStages []string`, `ReviewBaseline string`, and `ReviewInstruction string`, and allow `Action == "check_review_required"`.
2. Add a wsdoc-local freshness helper, likely in a new `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go`, that:
   - resolves the ticket's current Git-tracked path;
   - finds a review baseline from Git history for the last commit that made the relevant Sage posture become `completed`;
   - compares current content against that baseline, including staged and unstaged changes;
   - ignores diffs that only change Sage-owned posture fields and ignores pure path/status moves;
   - returns affected completed stage(s) without semantic body inspection.
3. Keep the helper additive and failure-tolerant where appropriate: no ticket path, no Git history, or no applicable completed stage should produce no warning rather than changing existing gate outcomes. Propagate only unexpected post-gate Git errors if tests show current wsdoc Git-reader precedent expects that.
4. In `SageGate`, call the helper before every completed-stage skip path: standalone todo design, ready epic design-only, ready completeness-only after terminal design, and ready combined/legacy-completed cases. If freshness is detected, return `check_review_required` with affected stage(s), baseline, and an instruction to inspect the diff and decide whether to rerun those Sage stage(s).
5. In `agents-plugin-tool/internal/wsdoc/tickets_verify.go`, call the same helper from `verifyTicketFile` for ticket-shaped paths after reading frontmatter. Append a soft warning such as guardrail `sage-review-freshness` for each affected stage set; do not change `OK`.
6. In `agents-plugin-tool/internal/mcp/server.go`, update `formatSageGate` for `check_review_required`. Prefer no special commit-path formatting for freshness warnings: the `VerifyResult.Warnings` path should already reach `git.commit` through `verifyAdapter` and `formatGitCommit`.
7. Add focused tests in `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go` for committed post-stamp edits, uncommitted edits, stage-specific design/completeness detection, no warning when only Sage posture fields changed, and no warning for pure path/status moves.
8. Add MCP/text tests in `agents-plugin-tool/internal/mcp/tickets_sage_test.go`, `agents-plugin-tool/internal/mcp/tickets_verify_test.go`, and/or `agents-plugin-tool/internal/mcp/server_test.go` proving `tickets.sage_gate` returns `check_review_required`, `tickets.verify` emits a non-blocking warning, `git.commit` surfaces the warning as an advisory, and no-ticket-touch commits are unaffected.

## Verification Plan
- `cd agents-plugin-tool && go test ./internal/wsdoc/... ./internal/mcp/... ./internal/wsgit/...`
- Focused during iteration: `cd agents-plugin-tool && go test ./internal/wsdoc -run 'Sage|TicketVerify'`
- Focused MCP checks: `cd agents-plugin-tool && go test ./internal/mcp -run 'Sage|TicketsVerify|GitCommitSurfaces'`
- Confirm no plan-time source/test/doc edits besides this file before handing to the executor.

## Escalations
- None.
