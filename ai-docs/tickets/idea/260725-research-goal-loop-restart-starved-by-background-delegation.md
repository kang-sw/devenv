---
title: goal-loop Stop-hook re-injection is starved when a cycle drives work through background subagents
---

# goal-loop Stop-hook re-injection is starved when a cycle drives work through background subagents

## Background

The `/goal` directive (here: `/lead-goal-step until drain all ready/ tickets`)
re-injects the goal skill each turn via a Stop-hook that fires on a **clean
terminal turn-stop**. `lead-goal-step` in turn mandates delegating everything
beyond selection to subagents to conserve lead context. During a live goal run
on 2026-07-24/25 these two mechanisms were observed to conflict.

## Observed behavior

- **Phase 1 cycle** dispatched its lead-proceed worker **synchronously**
  (`Agent(run_in_background:false)`, worker returned its full report in one
  call). The lead turn ended with **no pending background tasks** → the goal
  Stop-hook fired and re-injected the next cycle. Loop advanced normally.
- **Phase 2 cycle** dispatched a worker that **yielded control mid-flight**
  (returned a partial "waiting for my sub-agent" message instead of blocking on
  its own child). The lead then kept the turn alive with `SendMessage` resumes
  and background `Bash` polls. Every lead turn in this phase ended in a
  **"background work pending"** state and was re-entered via
  `<task-notification>`, **not** via a Stop-hook re-injection.
- Net effect: across the entire Phase 2 cycle the Stop-hook **never re-fired**.
  The loop appeared stalled; the human intervened to ask why.
- Confirmation: the moment the lead ended a turn **cleanly** (a plain answer
  with zero pending background tasks), the Stop-hook immediately re-injected the
  next cycle. This isolates the cause to *pending-background-task turn-ends
  suppress / bypass the goal Stop-hook re-injection path*.

## Why this matters

The starvation is triggered by the **exact posture `lead-goal-step` prescribes**
("delegate everything, conserve lead context"). The more faithfully a cycle
delegates via backgrounded subagents, the more reliably it starves the
Stop-hook-driven loop that is supposed to drive the next cycle. A caller who
follows the skill's own guidance can silently stall the goal run.

## Possible directions

- Characterize the harness rule precisely: does a turn ending with pending
  background tasks suppress the Stop-hook entirely, or merely defer it until the
  last background task drains — and if the latter, why did draining the last
  Phase 2 task not re-fire it? (The task-notification re-entry path may consume
  the "stop" event that the hook needs.)
- Skill-side mitigation: have `lead-goal-step` / fan-out variants prefer
  **synchronous** worker dispatch (block to completion, then return), or
  explicitly instruct dispatched workers to **not yield control** until fully
  done, so each cycle ends on a clean terminal stop.
- Harness-side option: let the goal Stop-hook re-fire on the terminal drain of
  the last background task, not only on a user-input-shaped clean stop.
- Relevant to the `lead-goal-fan-out-step` design (`260724-feat-lead-fan-out-worktree`):
  its fan-out path dispatches N background mini-leads and juggles them, which is
  the worst case for this starvation. The fan-out doctrine may need an explicit
  "keep the driving turn alive until the batch settles" rule, or a synchronous
  settle barrier, so the outer goal loop is not starved between cycles.

## Notes

Dogfood surprise surfaced while draining `ready/` via `/lead-goal-step` on
2026-07-24/25 (implementing `260724-feat-lead-fan-out-worktree`). Mechanism is
inferred from observed re-entry patterns, not yet confirmed against the harness
Stop-hook implementation; captured as research rather than a hard bug. Not
reduced to a minimal repro.
