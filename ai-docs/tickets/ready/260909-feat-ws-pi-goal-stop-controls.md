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

## Blocked (2026-09-10)

Phase 1 is blocked before source implementation by a verified Pi host admission
gap. The installed user-message path can accept an automatic `followUp` after
an asynchronous input/preflight boundary, even when the goal was stopped in the
intervening microtask. The public extension API provides neither a selective
queued-message cancellation handle nor a final cancellation check at dequeue.
Clearing the whole queue would also remove unrelated owner input and child
reports, violating this ticket's preservation contract. Timer cancellation and
goal-generation checks alone do not cover an already accepted host message.

The alternative idle `sendMessage` custom-message path starts synchronously, but
it bypasses the normal `before_agent_start` prompt composition. The installed
session's prompt-reset/next-turn-refresh behavior can remove the ws system prompt
on a subsequent tool/continuation turn; active-tool refresh can also remove it
before the first custom-message response. This transport is therefore rejected
as a substitute for the existing user-message path. Do not narrow the stop
contract to adapter timers, abort unrelated work, clear all queues, or bypass
canonical lead prompt assembly merely to implement the aliases.

Prerequisite: a supported Pi user-prompt admission cancellation contract covering
asynchronous preflight and queued consumption, or atomic idle-only admission with
cancellation through preflight, while retaining the normal extension hook and
system-prompt lifecycle. An upstream capability change (or an explicit owner
revision of the required behavior) is needed before this Pi-adapter-only phase
can advance. No additional approval for the already accepted feature scope is
being requested, and no upstream or shared-runtime implementation was attempted.

Evidence and source-backed alternatives are retained in
`ai-docs/.plans/2026-09/10-0237-260909-feat-ws-pi-goal-stop-controls.md`.
Research exercised the installed AgentSession, extension input handling, and
Agent lifecycle with a local throwing provider stub; no network or model request
ran. No source/dependency changed and no Phase Result or owner-live acceptance
is claimed. Keep the separate live child-report acceptance requirement for the
eventual implementation.
