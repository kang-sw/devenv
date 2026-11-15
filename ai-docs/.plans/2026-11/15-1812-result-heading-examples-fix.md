# Result heading examples: bounded guard fix

## Request and scope

MAIN promoted this settled small dogfood fix under owner 1058: ticket Result-heading guard must ignore fenced code blocks and quoted lines; include a regression with a fenced `### Result` example. No new ticket ceremony or policy change. Agent-card head has been sent, so this is next before the remaining Pi FIX group.

Likely implementation scope: `agents-plugin-tool/internal/wsdoc/tickets_verify.go`, `tickets_verify_test.go`, and an existing Markdown heading helper in `tickets.go` only if needed for minimal reuse. Do not add a new exported API, parser framework, store shape or workflow rule.

## Source evidence and semantic boundary

Lead read `ticketPhaseHeadingProblems` in tickets_verify.go:288–315. It trims each line and validates Phase/Result/Edition prefixes, even inside fences. Its comment explicitly says append-only phase/Result ordering is a diff-level property outside this function. Therefore the observed target is heading well-formedness, not a request to introduce or redesign phase-text immutability. Preserve all real heading validation and line-number diagnostics. Report this naming distinction in the final result.

The adjacent `ticketUnresolvedPhaseWarning` uses another lenient line scan; avoid unrelated semantics changes. Reuse a suitable existing fenced/quoted heading scanner if it preserves diagnostic line numbers and compatibility; otherwise keep the smallest local scanner consistent with established code. `tickets.go` around 788–853 already handles fenced/quoted headings for decision queues and Blocked markers.

## Verification

Add focused regressions proving fenced backtick and tilde examples and quoted lines do not become Result headings. Include a fenced malformed `### Result` example, a quoted example, real malformed headings still rejected, real valid Result behavior retained, and line numbers after skipped blocks remain correct. Exercise reasonable opening/closing fence indentation and length behavior consistent with the existing Markdown helper rather than inventing broad parsing semantics. Do not weaken validation by suppressing whole documents containing code.

Run the affected wsdoc heading/verify tests and narrow applicable package/build checks; read full output and reject empty selectors. No full suite, long/Windows checks, screenshots, network/install/download, live Claude/Pi/provider launches, owner data, shared daemon or 29417 operations. Follow applicable manuals before code edits. T0 transparent bug fix; no wire/store surface changes.

Commit the scoped fix, tests and this unchanged plan with AI Context on `fix/result-heading-quoted-examples`. One logical unit. No merge/push, cleanup or worktree release. Return source paths, exact checks, limitations, commit and quiescence. MAIN alone merges develop; EXEC2 lead owns final fresh-develop sync and affected integration verification.
