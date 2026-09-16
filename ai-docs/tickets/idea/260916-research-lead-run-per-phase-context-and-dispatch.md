---
title: lead-run per-phase context savings and dispatch discipline
related:
  260831-research-ai-phase-over-granularity-review-load: same axis — phase granularity and review/dispatch load
  260611-bug-agent-context-exhaustion-opaque-failure: the size pressure that forces coupled phases to split into phases
  260915-feat-ws-rationale-query-and-prior-decisions: dogfood session that surfaced this; its Phase 2 populator writes a bounded prior-decisions section into the ticket, a candidate durable home for per-phase state
---

# lead-run per-phase context savings and dispatch discipline

## Status of this ticket

Direction-only. The concrete prose edits to any playbook are **TBD and require
owner agreement** before implementation — see "Concrete edits" below. Do not
promote to ready or implement without that agreement; this surface is delicate.

## Observation

In a goal run, lead-run dispatches one worker per phase by default and, at each
dispatch, re-reads the whole ticket body to grade risk (Spawn step 2). For a
large multi-phase ticket this means the lead reads the full body once per phase.
The cost is largest exactly where it is unavoidable: phases are frequently split
at ticket-authoring time because the implementation is too big for one worker
context, NOT because the work is logically separable — so "batch the coupled
phases into one dispatch" does not apply to the tickets that hurt most.

Surfaced 2026-09-16 while running the rationale-query ticket, a single coupled
feature split into three phases for size, each dispatched and full-read
separately.

## Primary direction — lead-side context savings

Replace per-cycle "full re-read + re-grade" with:

1. Grade every phase's risk once, at the first read of the ticket, producing a
   per-phase tier plan.
2. Dispatch each subsequent phase at its planned tier without re-reading the
   whole body; re-grade a phase only when a prior phase's Result signals a
   material change (a contract break / Edition, an "Outstanding" note). This
   generalizes the existing retry-escalation pattern (signal-driven tier bump).
3. The per-phase tier plan must be durable across compaction, so it lives in the
   ticket (a natural home is alongside the route facts, emitted by the fact
   populator), never in lead conversation state — consistent with "keep it in
   the ticket, the branch, and git, not conversation".

Volatile facts (which phases have Results, the current route facts — a worker
may edit route facts) must be re-fetched authoritatively via the tickets query,
not trusted from stale context. A fresh / post-compaction lead with no context
still full-reads.

## Secondary threads

- **Task-block batch authorization.** For genuinely tightly-coupled AND small
  phases, the lead may authorize multi-phase execution in the worker task block
  (the "unless the task block says otherwise" escape already exists in the
  ticket-worker Inputs) — grade once, dispatch once. Bounded by worker context
  capacity.
- **Worker unilateral-batching discipline.** The ticket-worker Inputs say
  "later phases are out of scope unless the task block says otherwise", but a
  worker was observed batching three phases on its own judgment (worktree-pool
  ticket, 2026-09-16). Decide the rule: either the worker must stop for lead
  authorization before batching, or the lead must always authorize via the task
  block — the current text allows an unsanctioned widening.

## Concrete edits — TBD (owner agreement required)

The exact edits to the lead-run, ticket-worker, and ticket-fact-populator
surfaces are deliberately deferred. This touches the grading contract, the
durable-state-in-ticket invariant, and the fact-populator output shape at once,
and it interacts with the phase-over-granularity research. The form must be
agreed with the owner before any shipped prose changes; this ticket captures
direction only.

## Open questions

- Where exactly the per-phase tier plan lives, and whether the fact populator or
  a separate step emits it.
- Whether Result-gated re-grade needs an explicit "material change" marker in
  the Result, or the lead infers it.
- How this composes with the phase-over-granularity work (they may share one
  fix).
