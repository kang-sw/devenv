---
title: "Pi adapter: stopped reviewer can leave worker final gate outstanding"
related:
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: owns descendant completion accounting
  260908-feat-ws-pi-agent-session-disk-retention: observed during its fix-only review closure
dropped: 2026-09-13
---

# Pi adapter: stopped reviewer can leave worker final gate outstanding

## Background

Observed 2026-09-13 during the retention worker's closure at `045da319`.
The reviewer returned a normal report with a CLEARED verdict. The worker
consumed that report, materialized its artifact, and called `ws-agent-stop`.
`ws-agent-list` then showed the sole reviewer as dormant. Nevertheless,
`ws-report-to-lead(kind: "final")` rejected completion with:

> final rejected while child results are outstanding; consume their reports,
> follow up or explicitly stop them, then submit a fresh final

A second explicit stop followed by a fresh final produced the same rejection.
The worker then continued the same reviewer to request a terminal acknowledgement
rather than starting another review. The exact outstanding counter is now diagnosed: `publishSubtree` counts
`expectedReport` or `waitingOnChildren` independently of a child's dormant registry
status, and a non-silent `stopAgent` clears both before republishing
(agents-plugin-pi/src/subtree-lifecycle.ts#L40-L48; agents-plugin-pi/src/spawner.ts#L3492-L3503).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/subtree-lifecycle.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/test/recursive-worker.test.ts |
| scope.surface | public-interface | ws-agent-stop and ws-report-to-lead final-gate behavior is externally observable |
| scope.new_public_symbol | no | no new public symbol is required |
| scope.new_type_contract | no | the confirmed fix clears existing expectedReport and waitingOnChildren fields |
| scope.test_surface | existing | agents-plugin-pi/test/recursive-worker.test.ts#L295-L318 covers stopped-child quiescence and repeated dormant stops |
| complexity.reuse_points | confirmed | subtree publication and stop disposition are implemented in agents-plugin-pi/src/subtree-lifecycle.ts#L35-L62 and agents-plugin-pi/src/spawner.ts#L3484-L3575 |
| complexity.side_effect_risk | high | stop disposition changes parent completion and child resumption lifecycle |
| risk.correctness | high | an uncleared obligation rejects a worker final; clearing a live obligation could lose work |
| risk.fit | high | worker-local reviewer completion must compose with subtree final gating |
| risk.test | high | report delivery, stop, dormant retry, and fresh-final ordering are race-sensitive |
| risk.security_or_contract | moderate | ws-agent-stop and worker final semantics are a caller-visible lifecycle contract |

## Phases

### Phase 1: Reconcile stopped-child disposition with the final gate

Reproduce normal report -> consume -> explicit stop -> parent final with one
reviewer, including report delivery racing stop and repeated stop on a dormant
child. Inspect `publishSubtree`/`assertSubtreeFinal` in
`agents-plugin-pi/src/subtree-lifecycle.ts` and the stop/delivery transitions in
`agents-plugin-pi/src/spawner.ts`. Determine which obligation remains and either
settle it through the documented explicit-stop path or expose an actionable
reason. Preserve the final gate for genuinely outstanding descendants; do not
weaken it merely because an agent is dormant. Add a regression for the confirmed
cause. No additional runtime fix is authorized: the confirmed fix and regression
already landed under 260912-feat-ws-pi-recursive-worker-subtree-lifecycle
(agents-plugin-pi/src/spawner.ts#L3492-L3503; agents-plugin-pi/test/recursive-worker.test.ts#L295-L318).


## Resolution (2026-09-13)

No implementation remains: the confirmed stop-disposition fix and dormant-stop regression already landed under `260912-feat-ws-pi-recursive-worker-subtree-lifecycle`. This later capture is absorbed by that completed ticket.
