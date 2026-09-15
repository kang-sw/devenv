---
title: "Restore the prefixless context-token value in Pi live agent rows"
related:
  260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets: source ticket captured the intended token-label change incorrectly
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 21ec7d0d598da603
sage-review-completeness-reviewed: 21ec7d0d598da603
completed: 2026-09-15
---

# Restore the prefixless context-token value in Pi live agent rows

## Background

The source ticket recorded removal of the live-row `ctx Xk` text, and the implementation followed that wording by dropping the complete context-token value. The owner's actual instruction was narrower: remove only the literal `ctx ` prefix and retain the compact token value. For example, a row that previously showed `ctx 132.4k` must show `132.4k`, not omit the field.

The current row model still carries `contextTokens`, and `/audit` still consumes the labeled formatter. This correction is limited to the live agent widget presentation and the tests that encoded the mistaken omission.

## Decisions

- Keep the compact `Nk` context-token value in every live agent row and remove only its `ctx ` prefix.
- Preserve the activity field added by the source ticket. The prefixless token value occupies its prior telemetry position, between model/effort and activity.
- Keep `/audit` unchanged; its existing labeled `ctx Nk` presentation is not part of this correction.

## Constraints

- Do not change `AgentRow` population, context-token accounting, model/effort, activity, estimated-cost, state-bullet, or owner-attention behavior.
- Preserve the current unknown-token fallback while removing only the `ctx ` label from the live row.
- Add positive assertions for the retained compact value, including a representative value such as `132.4k`, and negative assertions that the live row contains no `ctx ` prefix. A test that checks only the prefix absence is insufficient.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts#L84-L86, agents-plugin-pi/test/agent-widget.test.ts#L306-L330 |
| scope.surface | internal | no exported symbol or type-signature change; only live-row rendering changes in agents-plugin-pi/src/agent-widget.ts#L371-L386 |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-pi/test/agent-widget.test.ts#L306-L330; `npm test` is defined in agents-plugin-pi/package.json#L45 |
| complexity.reuse_points | confirmed | AgentRow.contextTokens is populated at agents-plugin-pi/src/agent-widget.ts#L264-L283; formatContextTokens is retained for /audit at agents-plugin-pi/src/audit.ts#L28-L311 |
| complexity.side_effect_risk | low | pure formatRow presentation change in agents-plugin-pi/src/agent-widget.ts#L371-L414 |
| risk.correctness | moderate | the live row must retain the numeric value while excluding only the ctx prefix |
| risk.fit | low | confined to the existing live agent-widget presentation |
| risk.test | moderate | plain and themed rendering plus truncation require discriminating existing tests |
| risk.security_or_contract | low | local TUI presentation only; no security or external contract change |

## Phases

### Phase 1: Restore the prefixless live-row token value

Update both plain and themed live-row rendering to include the existing compact context-token value without the `ctx ` prefix. Replace the tests that currently permit the value to disappear with discriminating assertions that require the value and reject the prefix. Run the focused agent-widget tests and the full Pi adapter test suite.

### Result (682a2718) - 2026-09-15

- Restored the live-row context value with an internal unlabeled formatter: populated values render as `132.4k`, unknown values as `?`, and both plain and themed telemetry retain the field between model/effort and activity. `/audit` continues to use the unchanged labeled `formatContextTokens` formatter.
- Added discriminating plain and themed assertions for `0.0k` and `132.4k`, the missing-token fallback, the absence of only the `ctx ` prefix, and (review follow-up `520c26c7`) exact-fit 65-column versus 64-column all-or-nothing telemetry behavior.
- Verification: focused `npm test -- --test-name-pattern='telemetry retains prefixless context values'` passed (58 files, 58 pass); the earlier broader focused set passed (59 files, 59 pass). Full `npm test` ran 1,569 passing tests with one unrelated failure in `test/bridge.test.ts` because its fixed 54-tool assertion observes 56 bundled tools; captured as `260915-bug-pi-bridge-bundled-tool-count` rather than changing unrelated bridge behavior.
- Review: round 1 correctness clean; round 1 test review found the fit-boundary coverage gap, fixed in `520c26c7`; round 2 verified that fix cleanly with no unresolved observations.


## Resolution (2026-09-15)

Phase 1 completed: live Pi agent rows retain the prefixless compact context-token value; independent review completed with its fit-boundary finding fixed. The full suite has one unrelated stale bundled-tool count assertion tracked in 260915-bug-pi-bridge-bundled-tool-count.
