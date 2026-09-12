---
title: Stop Pi goal reinjection without interrupting current work
sage-review-design: completed
sage-review-completeness: completed
spec:
  - pi-adapter-runtime
related:
  260906-workset-ws-pi-dogfood-ux: inclusion in the owner UX collection
sage-review-design-reviewed: aea47f4d50abc9ca
sage-review-completeness-reviewed: aea47f4d50abc9ca
---

# Stop Pi goal reinjection without interrupting current work

## Background

The owner needs an explicit goal-loop stop control. On 2026-09-09 the owner
confirmed that stopping disarms automatic follow-up execution while preserving
the current response, child agents and conversation history. The current
`/goal <goal>` handler arms/replaces a goal; `goal-achieved` and `goal-blocked`
already disarm goal state without interrupting other work.

## Decisions

- `/goal stop` disables automatic goal reinjection/rearm. `/goal clear` and
  `/goal reset` are aliases of that same operation, not replacement goal text.
- Do not abort the response currently executing, stop child agents, delete
  history or erase their results. Already-executing work may finish normally.
- Cancel any automatic goal reminder or rearm that the adapter has not yet
  handed to Pi. In-flight timers and compaction callbacks must compare a goal
  generation token and recheck that their goal is still armed before submission.
  Track at most one outstanding reminder handoff per lead session and include an
  adapter-owned correlation marker in the submitted reminder. Wake timeout
  recovery must not resubmit while that handoff remains unconfirmed. Clear it
  only when the public `message_start` event exposes a user payload containing
  the matching marker; unrelated owner messages and child-report wakes do not
  clear it. A reminder already handed to Pi may execute once after stop; the adapter does not clear
  host queues or promise selective cancellation after handoff. Existing
  child-report delivery and its independent wake recovery continue.
- Clear the active goal status consistently and report that automatic goal
  continuation has stopped. Repeated stop with no active goal is harmless.
- A later explicit new goal can arm normal continuation again. Session recovery
  or compaction must not resurrect a stopped goal or stale queued callback.
- Preserve other existing goal commands and ordinary goal text entry. Document
  the reserved stop/clear/reset command words in help/completion.

## Spec Impact

Update `pi-adapter-runtime` goal-loop command/lifecycle coverage for stop aliases,
non-interruption of current work, suppression before host handoff, the permitted
single already-handed-off reminder, persistence through recovery, idempotency
and explicit rearming.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/push-wake.test.ts, ai-docs/spec/pi-adapter-runtime.md |
| scope.surface | public-interface | user-visible /goal stop, /goal clear, and /goal reset aliases |
| scope.new_public_symbol | no | existing goal command gains aliases only |
| scope.new_type_contract | no | no new type or signature is required by the phase text |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts and agents-plugin-pi/test/push-wake.test.ts |
| complexity.reuse_points | confirmed | registerGoalLoop lifecycle, cancelSettleTimer, and the shared wake reservation |
| complexity.side_effect_risk | high | reminder scheduling shares wake and compaction lifecycle state with child-report delivery |
| risk.correctness | high | stale timer and compaction callbacks must not rearm a stopped or replacement goal |
| risk.fit | high | stop must preserve the shared push-recovery and child-report lifecycle |
| risk.test | high | aliases, timers, active work, compaction, recovery, and stale callbacks need coverage |
| risk.security_or_contract | high | user-visible goal-continuation and host-handoff behavior must retain its stated boundary |

## Phases

### Phase 1: Add explicit goal stop aliases with race-safe disarming

Route the three exact stop commands to the existing goal state lifecycle, cancel
local timers, and increment a generation token so stale timer and compaction
callbacks cannot submit or rearm a stopped goal. Add a session-level outstanding
handoff ID before submission, carry that ID in an adapter-owned reminder marker,
and clear it only when a public user `message_start` payload contains the matching
marker. This covers both an idle admission and a queued follow-up consumed within
an existing run, where no additional `agent_start` occurs. Make goal wake-timeout
recovery observe it rather than issuing a duplicate. Keep the current response and child report lifecycle intact; align command help
and status. Do not clear Pi's host queues: a reminder already handed to Pi before
stop may execute once.

Verification: command parsing for all aliases and ordinary goal text; repeated
stop; stop during active lead response, running child, pending reminder and
compaction; slow asynchronous admission across wake-timeout expiry produces only
one reminder handoff; matching user `message_start` clears the marker for both
idle admission and queued consumption while unrelated messages do not;
child-report wake recovery remains independent; stale callback settlement after stop/new goal; one
already-handed-off reminder permitted without subsequent rearm; no new automatic
turn after session recovery; successful explicit new-goal rearm. Owner live check stops a goal while
a child runs and confirms its report still arrives and no reminder is submitted
after stop. No source implementation is part of preparation.

## Scope revision (2026-09-12)

The owner explicitly narrowed the contract to adapter-owned best-effort stopping.
The earlier blocked analysis correctly established that Pi exposes no selective
cancellation after asynchronous host admission, but that stronger guarantee is
no longer required. Phase 1 now prevents reminders that have not been handed to
Pi and permits one already-handed-off reminder to execute, so it is implementable
without an upstream Pi queue-cancellation API. The adapter still must not clear
unrelated owner input or child reports or bypass canonical prompt composition.

The prior admission-race evidence and rejected alternatives remain in
`ai-docs/.plans/2026-09/10-0237-260909-feat-ws-pi-goal-stop-controls.md` as the
reason for the explicit best-effort boundary. No source implementation or owner-live
acceptance is claimed by this scope revision.
