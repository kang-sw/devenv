---
title: Stop Pi goal reinjection without interrupting current work
sage-review-design: completed
sage-review-completeness: completed
spec:
  - pi-adapter-runtime
related:
  260906-workset-ws-pi-dogfood-ux: inclusion in the owner UX collection
sage-review-design-reviewed: 19cac2472e6cb051
sage-review-completeness-reviewed: 19cac2472e6cb051
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
- Cancel any pending automatic goal reminder/rearm that has not begun executing.
  In-flight async callbacks must recheck that their goal is still armed before
  delivering another automatic turn. Existing child-report delivery continues.
- Clear the active goal status consistently and report that automatic goal
  continuation has stopped. Repeated stop with no active goal is harmless.
- A later explicit new goal can arm normal continuation again. Session recovery
  or compaction must not resurrect a stopped goal or stale queued callback.
- Preserve other existing goal commands and ordinary goal text entry. Document
  the reserved stop/clear/reset command words in help/completion.

## Spec Impact

Update `pi-adapter-runtime` goal-loop command/lifecycle coverage for stop aliases,
non-interruption of current work, suppression of queued reminders, persistence
through recovery, idempotency and explicit rearming.

## Phases

### Phase 1: Add explicit goal stop aliases with race-safe disarming

Route the three exact stop commands to the existing goal state lifecycle and
ensure queued timers/async work cannot rearm a stopped goal. Keep the current
response and child report lifecycle intact; align command help and status.

Verification: command parsing for all aliases and ordinary goal text; repeated
stop; stop during active lead response, running child, pending reminder and
compaction; stale callback settlement after stop/new goal; no automatic turn
after session recovery; successful explicit new-goal rearm. Owner live check
stops a goal while a child runs and confirms its report still arrives without
automatic goal continuation. No source implementation is part of preparation.
