---
title: "Pi accepts goal-compact-and-continue without an active goal, then aborts compaction"
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: introduced the goal-mode lever and active-goal lifecycle
  260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction: established held-push and carry-forward behavior during valid active-goal compaction
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 72a2c0a4b6bd76d7
sage-review-completeness-reviewed: 72a2c0a4b6bd76d7
---

# Pi accepts goal-compact-and-continue without an active goal, then aborts compaction

## Background

After the prior goal had been declared achieved, an owner-requested behavior probe called `goal-compact-and-continue`. The tool returned `Compaction requested; ...`, but Pi subsequently displayed `Error: This operation was aborted`.

The lever is meaningful only while a goal is active. Accepting it after goal teardown schedules an invalid host compaction transition and produces a misleading success result before the later abort.

## Decisions

- Reject `goal-compact-and-continue` synchronously when no goal is active.
- The rejection must clearly identify the inactive-goal precondition and must not report that compaction was requested.
- An inactive-goal call must not schedule host compaction, persist carry-forward prose, re-arm goal injection, or otherwise mutate goal-loop state.
- Preserve the existing valid active-goal compaction, held-push, and verbatim carry-forward behavior unchanged.
- Keep the guard in the authoritative lever path so tool calls and any equivalent adapter entry point cannot diverge.

## Constraints

- This is a narrow Pi-extension hotfix; do not change shared ws-mcp or shared playbook contracts.
- Preserve the unrelated zero-byte untracked plan at `ai-docs/.plans/2026-09/06-1203-260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches.md` exactly and exclude it from commits.
- Do not introduce a new public command or compatibility alias.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts#L1087-L1142, agents-plugin-pi/test/goal-loop.test.ts |
| scope.surface | public-interface | existing model-invoked goal-compact-and-continue tool is registered in agents-plugin-pi/src/goal-loop.ts#L1087-L1142 |
| scope.new_public_symbol | no | guard changes the existing goal-compact-and-continue tool |
| scope.new_type_contract | no | existing carry_forward parameter shape remains unchanged |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts already covers goal-compact-and-continue; no inactive tool-execution case was found |
| complexity.reuse_points | confirmed | existing GoalLoopState.active guard in agents-plugin-pi/src/goal-loop.ts |
| complexity.side_effect_risk | moderate | the guard must precede compaction state, carry-forward, and callback mutation |
| risk.correctness | high | an inactive call currently reaches ctx.compact in agents-plugin-pi/src/goal-loop.ts#L1117-L1140 |
| risk.fit | moderate | active-goal compaction and its carry-forward lifecycle must remain unchanged |
| risk.test | moderate | focused existing tests can assert both rejection and unchanged active behavior |
| risk.security_or_contract | moderate | rejection changes the existing model-invoked tool's caller-visible contract |

## Phases

### Phase 1: Fail fast before inactive-goal compaction

Add the inactive-goal precondition at the authoritative `goal-compact-and-continue` execution boundary. Cover both the rejected inactive call and the unchanged active-goal path with focused regression tests. Verify that rejection occurs before any compaction callback, carry-forward persistence, or re-injection scheduling, and run the relevant Pi extension suite.
