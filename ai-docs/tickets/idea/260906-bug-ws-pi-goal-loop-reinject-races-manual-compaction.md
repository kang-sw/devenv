---
title: goal-compact-and-continue re-injects the goal reminder before compaction finishes, and the late compaction overwrites the turn it raced
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: owns the lever and the agent_settled reinject path
spec:
  - pi-adapter-runtime
---

# goal-compact-and-continue re-injects the goal reminder before compaction finishes, and the late compaction overwrites the turn it raced

## Background

Owner dogfood, 2026-09-06, two runs against the shipped `goal-loop.ts`.

1. Trivial goal (`Reply with the single word pong, then finish this goal.`)
   on a near-empty session: the model called `goal-compact-and-continue`
   instead of `goal-achieved`. The TUI showed `Error: This operation was
   aborted`, the goal reminder re-fired, then `Error: Compaction failed:
   Nothing to compact (session too small)` twice.
2. Same lever on a filled session: the reminder re-fired at once, the model
   kept working for several turns, and when the compaction summary landed
   late it replaced everything said after the lever call.

Both are one ordering defect. The lever calls `ctx.compact(...)` without
awaiting it (Pi's extension wrapper is fire-and-forget by design). Pi's
`AgentSession.compact()` begins with `await this.abort()` and only then
sets `_compactionAbortController`, the flag its prompt guard (`Cannot submit
a prompt while compaction is in progress`) checks. The abort settles the
in-flight turn, so `agent_settled` fires while the flag is still unset; the
goal loop's armed handler calls `pi.sendUserMessage(reminder)`, the prompt
passes the guard, and a new turn starts while the summarization request is
still running. When compaction completes, Pi assigns
`agent.state.messages = sessionContext.messages`, so the racing turn's
messages are discarded in favour of the summary plus kept entries. When
compaction fails instead (run 1), the racing turn survives but the lever's
`onError` notify arrives after the model has already moved on, and the
reminder text gives the model no reason not to call the lever again.

Secondary observation from run 1: the reminder advertises the compaction
lever prominently enough that the model picked it for a one-word goal at 13%
context. The advisory percent already exists; the wording may need to make
"below the advisory point, do not compact" explicit.

## Proposed direction

- Track an in-flight compaction in goal-loop state, set when the lever fires
  (before `ctx.compact`) and, defensively, on `session_before_compact` with
  `reason: "manual"`.
- `agent_settled` while that flag is set neither re-injects nor advances the
  runaway streak; it sets a status line (`Goal loop: waiting for
  compaction`), mirroring the existing yield branch.
- Re-arm from the completion side: on the lever's `onComplete` (or
  `session_compact`), clear the flag and send the reminder then, since the
  abort-triggered settle has already passed and no further `agent_settled`
  will come. On `onError`/`session_compact_failed`, clear the flag and send
  the reminder with the failure reason included so the model chooses a
  terminal lever rather than retrying compaction (covers `Nothing to compact
  (session too small)` and `Already compacted`).
- Keep the observe-only posture for Pi's threshold/overflow auto-compaction;
  this ticket touches only the manual lever path.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change.
- The lever remains non-terminal and never disarms the goal.
- Pure reducer shape for the settle decision is preserved so the new
  "waiting" branch is unit-testable without a live Pi session.

## Phases

### Phase 1: Gate reinject on the in-flight compaction and re-arm on completion

Implement the direction above in `goal-loop.ts`; tests for settle-while-
compacting (no reminder, streak unchanged), reminder sent exactly once on
completion, reminder-with-reason on failure, and the trivial-goal case
(reminder below the advisory point tells the model not to compact). Live
check (owner-run): repeat both dogfood runs and confirm the post-lever
conversation is not replaced.
