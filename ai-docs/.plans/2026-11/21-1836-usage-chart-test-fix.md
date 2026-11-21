# Fix the pre-existing usage chart node test failures

## Problem
`node --test usage.test.js usage-axis.test.js usage-trend.test.js` in
`agents-plugin-tool/internal/wsdashboard/` fails on develop (already failing
before the 2026-11-21 Claude usage refresh change; 6 failures were seen in
usage.test.js then). Examples on develop 872a7833b:
- usage.test.js:203 "unknown resets have isolated real dots without
  baseline, gradients or connected lines": expected 2, got 1.
- usage.test.js:222 "stale cards neutralize only the current marker and
  freeze pace without recoloring history": TypeError reading `attributes`
  of undefined at :230.
- at least one more strictEqual 1 vs 0 failure above those.

## Task
1. Run the three files, list every failing test with its assertion.
2. For each, use `git log -S`/`git log -L` on the test and on
   `assets/usage.js` (and any chart helper it uses) to find which commit
   made test and code diverge, and decide: the test is stale (code changed
   on purpose: cite the commit and its AI Context) -> update the test; or the
   code regressed (no intentional change explains it) -> fix the code
   minimally.
3. If a case is ambiguous (an intentional-looking product change you cannot
   confirm), do not guess: leave it failing and report it with evidence.

## Constraints
- Surgical: only the failing tests and the code they show to be wrong.
- Read ai-docs/manuals/shipped-surface-boundary.md before editing shipped
  assets.
- Verify: the three node test files pass, plus
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsdashboard -run 'PlanUsage|Usage|Asset' -timeout 20m`
  from agents-plugin-tool if assets changed (never the whole package).
  GOCACHE under /tmp if the default is not writable.
- One commit with `## AI Context` naming each failure, its verdict (stale
  test / regression) and the evidence commit.
- No pushes, no daemon, no main checkout.
