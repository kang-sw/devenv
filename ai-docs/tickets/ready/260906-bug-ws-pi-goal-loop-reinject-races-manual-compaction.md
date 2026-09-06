---
title: goal-compact-and-continue re-injects the goal reminder before compaction finishes, and the late compaction overwrites the turn it raced
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: owns the lever and the agent_settled reinject path
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: db933d4c79ded81c
sage-review-completeness-reviewed: db933d4c79ded81c
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

The goal reminder is not the only turn-starter that can race. Every
adapter push (`pushToLead`: child final reports, settle/exited signals,
approval and headless-question steers, fork-question advisories) goes
through `pi.sendMessage(..., { triggerTurn: true })`. Pi's
`sendCustomMessage` routes that to `agent.steer/followUp` only while the
agent is streaming; otherwise it calls `_runAgentPrompt` directly, which has
no compaction guard at all (the `Cannot submit a prompt while compaction is
in progress` check lives in `prompt()` only). During a compaction the agent
is not streaming, so a child report landing mid-compaction starts a turn
and is overwritten the same way. The adapter's own `heldPushQueue` already
holds `followUp` pushes while the lead is mid-turn, keyed on
`isOwningAgentIdle()` (Pi's `isIdle`, which ignores compaction); `steer`
pushes are never held. Owner-typed input is already safe once Pi's flag is
set: the interactive mode queues it via `queueCompactionMessage` (steer /
followUp) and flushes after compaction, with extension commands executing
immediately.

## Proposed direction

Owner's framing (2026-09-06): while a compaction is in flight, the adapter
treats the lead exactly as if the agent were mid-turn. Everything that would
be queued behind a running turn is queued behind the compaction, and is
released when the compaction ends.

- Track an in-flight compaction in adapter state (a `leadCompactingRef`
  beside `leadIdleRef`), set when the lever fires (before `ctx.compact`)
  and, defensively, on `session_before_compact` (any reason). It is cleared
  only by the single release routine below.
- `isOwningAgentIdle()` returns `false` while that flag is set, so
  `followUp` pushes fall into the existing `heldPushQueue`. `steer` pushes
  gain the same hold while compacting (they cannot interrupt a compaction
  usefully, and starting a turn is the defect); `HeldPush` therefore
  records the family's `deliverAs`, and `flushHeldPushes` re-sends each item
  with its recorded mode instead of the fixed `"followUp"`. The existing
  `agent_settled` release of the held queue is gated on the flag: the
  abort-triggered settle inside Pi's `compact()` fires deterministically
  before Pi sets its own flag, so an ungated flush there would start a turn
  through `_runAgentPrompt` and reproduce run 2 on the push path.
- One lead turn-starter lives outside `pushToLead`: `ask.ts`'s
  `injectDiscussionSummary` sends the `ws-thread-summary` custom message
  with `triggerTurn: true` directly, and its trigger (`/done` in a
  `lead-ask` thread) is an extension command, which interactive mode runs
  immediately even during a compaction. It is routed through the same
  hold: while the flag is set it enters the held queue with its own family
  and `deliverAs`, and is released like any other push. Background's
  "every adapter push goes through `pushToLead`" holds after this change,
  not before it.
- **One release routine, idempotent, deferred past Pi's flag.** A single
  `releaseAfterCompaction(outcome)` runs at most once per compaction
  (second calls no-op on a cleared flag). It clears the flag, then branches
  on the agent's state at that instant:
  - Agent idle: flush the held queue (status lines computed at release
    time as today), then send the goal reminder when one is pending (see
    the next bullet) with an explicit `deliverAs: "followUp"`, so it queues
    behind the turn a released push has just started and starts a turn
    itself when nothing did.
  - Agent not idle (streaming, or a prompt in flight that has not yet set
    `_isAgentRunActive`, which is where Pi's `compaction_end` flush of
    owner input queued during compaction can leave it): send nothing. The
    held queue and the pending reminder are left to that turn's
    `agent_settled`, whose flush is no longer gated once the flag is clear
    and whose settle reducer sends the reminder through the normal path.
    Starting a second run from the release would be the original defect.
  - **Reminder only for lever-originated compactions.** The reminder is
    "pending" only when the adapter itself started the compaction (the
    lever sets a `pendingRearm` payload beside the flag: failure reason if
    any, carry-forward per Phase 2). Compactions the adapter did not
    originate (owner `/compact`, Pi's threshold/overflow auto-compaction)
    clear the flag and flush held pushes on the idle branch but never
    synthesize a reminder: the goal loop stays observe-only for them, and
    on the auto-compaction path the run is still active so the settle
    reducer produces the next reminder with its streak, yield, and
    force-stop rules intact. Whichever path sends the next reminder
    consumes and clears `pendingRearm`.
  Who calls it:
  - `session_compact` and `session_compact_failed` handlers, via
    `setImmediate` (or equivalent macrotask deferral). Pi emits
    `session_compact` while `_compactionAbortController` is still set and
    clears it only after the awaited emit returns (`core/agent-session.js`,
    the `session_compact` emit precedes `_compactionAbortController =
    undefined`), and `prompt()` throws `Cannot submit a prompt while
    compaction is in progress` while it is set, with the throw swallowed
    into `runner.emitError`. Sending from inside that handler is therefore
    a silent failure; the deferral lands after `compact()` has cleared the
    flag. `session_compact_failed` is emitted from the catch block after
    the flag is already cleared, so its deferral is not required for
    correctness; it is kept so both handlers share one code path. These
    handlers cover compactions the adapter did not originate.
  - The lever's `onComplete` and `onError` callbacks, which Pi invokes after
    `compact()` has fully returned. They are the backstop for the lever
    path: `session_compact` is emitted under a guard (`savedCompactionEntry`
    lookup by summary), so a successful compaction can complete without it,
    and without a backstop the adapter would hold every push forever.
  - `agent_start` while the flag is set clears the flag without sending a
    reminder (the turn that started will settle normally and the existing
    release paths apply). It is the last-resort backstop for a non-lever
    compaction whose completion event never arrives; a turn can only start
    once Pi's compaction guard has lifted.
- Goal loop: `agent_settled` while compacting neither re-injects nor
  advances the runaway streak; it sets a status line (`Goal loop: waiting
  for compaction`), mirroring the yield branch. The reminder sent by the
  release routine on the failure path includes the failure reason so the
  model chooses a terminal lever rather than retrying compaction (covers
  `Nothing to compact (session too small)` and `Already compacted`).
- Reminder wording below the advisory point: when the computed context
  percent is under the configured advisory percent (the existing
  `compaction_advisory_percent` source), the reminder states `Below the
  compaction advisory point; do not call goal-compact-and-continue` in
  place of the current neutral lever mention. At or above the point the
  existing advisory sentence stays as is.
- Owner-typed input needs no adapter work beyond the flag: Pi already queues
  it while `isCompacting`. For lever-originated compactions the adapter's
  flag (set before `ctx.compact`) closes the gap between `abort()` resolving
  and Pi setting its flag for every adapter-originated turn starter.
- **Accepted window: owner-typed `/compact` while a goal is armed.** Pi's
  built-in `/compact` calls `session.compact()` directly; its `abort()`
  emits `agent_settled` before `compaction_start` and
  `session_before_compact`, so the adapter learns of that compaction only
  after the settle has passed. No extension hook precedes it. The goal
  reminder can therefore still re-inject on that settle and be overwritten,
  exactly as today. This ticket accepts that window rather than shadowing
  Pi's `/compact`: the lever is the documented way to compact under an
  armed goal, and the guide and spec say so. Pi's threshold/overflow
  auto-compaction has no such window (it runs mid-turn with the agent
  active, so pushes queue as `steer`/`followUp`).
- Pi's threshold/overflow auto-compaction keeps its observe-only posture for
  the goal loop, but the push hold applies to it as well, since the same
  race exists there for child reports.
- **Carry-forward is not verbatim today.** Pi appends `customInstructions`
  to its summarization prompt as `Additional focus: <text>`
  (`core/compaction/compaction.js`, `generateSummaryWithUsage`), so the
  lever's `carry_forward` only steers the summarizer; the model that resumes
  sees whatever the summary model made of it, not the string it wrote. The
  lever's own result text already echoes the string, but that result is
  part of the turn the compaction summarizes away. Owner's ask (2026-09-06):
  the exact string must reach the resumed model. The adapter keeps the
  `carry_forward` string in goal-loop state when the lever fires and folds
  it verbatim into the re-arm reminder sent on `session_compact`, under a
  fixed heading (`Carried forward verbatim from before compaction:`), and
  clears it once sent. The failure-path reminder carries it too, since the
  model's own turn was aborted either way. `customInstructions` still goes
  to Pi so the summary is also steered by it.

## Spec Impact

`pi-adapter-runtime`, Phase 1: amend
`{#260904-pi-goal-loop-model-driven-compaction}` (its "the goal then reaches
a fresh settle and the existing armed `agent_settled` reminder re-enters"
sentence becomes the release-routine re-arm, and the accepted `/compact`
window is stated); amend `{#260904-pi-report-to-lead-channel}` (the hold
predicate becomes "agent idle and no compaction in flight", and "`steer`
families are never held" gains the compacting exception); amend
`{#260904-pi-goal-loop-arming-settled-levers}` with the waiting settle
outcome and the below-advisory reminder wording. Phase 2: amend the lever
sub-anchor under the goal-loop anchor with the verbatim carry-forward
guarantee. `pi-lead-guide.md` gains one line naming the lever as the way
to compact under an armed goal.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change.
- The lever remains non-terminal and never disarms the goal.
- Every reminder or push sent after a compaction goes through the release
  routine; no handler sends a prompt from inside a `session_*compact*`
  event.
- Pure reducer shape for the settle decision is preserved so the new
  "waiting" branch is unit-testable without a live Pi session.

## Phases

### Phase 1: Hold every push while compacting and re-arm the goal on completion

Add the compacting flag beside `leadIdleRef` in `spawner.ts`/`index.ts`,
set by the lever and `session_before_compact`; make `isOwningAgentIdle()`
false while set; add `deliverAs` to `HeldPush` and hold `steer` pushes too;
route `injectDiscussionSummary` through the hold; gate the `agent_settled`
flush on the flag. Implement
`releaseAfterCompaction` and wire its three callers (deferred
`session_compact`/`session_compact_failed`, the lever's
`onComplete`/`onError`, and the `agent_start` flag reset). In
`goal-loop.ts`: the settle reducer gains a "waiting for compaction"
outcome; the reminder is sent only by the release routine, with
`deliverAs: "followUp"` and the failure reason when applicable; the
below-advisory wording lands. Tests: settle-while-compacting (no reminder,
streak unchanged, status set, held queue not flushed); `followUp` and
`steer` pushes held while compacting and re-sent with their recorded modes
by the release routine before the reminder; release runs once when both a
`session_compact` and an `onComplete` arrive; release from
`session_compact` is deferred (nothing sent synchronously inside the
handler); `onError` alone releases with the reason in the reminder;
`agent_start` clears the flag without a reminder; a non-lever
`session_compact` releases held pushes but sends no reminder; release
while the agent is not idle sends nothing and the following settle flushes
and re-arms; the thread-summary injection is held while compacting; the
trivial-goal case
(reminder below the advisory point tells the model not to compact). Amend
the three Phase 1 spec passages and the guide line. Live check (owner-run):
repeat both dogfood runs and confirm the post-lever conversation is not
replaced and a child report arriving during compaction is delivered
afterwards.

### Result (81463a7d) - 2026-09-06

Landed in `81463a7d` (source, tests), `45f7c2ef` (spec, guide), and the two
review-fix commits `2c8db8c9` and `60216eaa`.

- `leadCompactingRef` sits beside `leadIdleRef` in `spawner.ts`; it is set
  by the lever before `ctx.compact` and by `session_before_compact` for
  every compaction, and `isOwningAgentIdle()` (now exported) returns false
  while it is set. `HeldPush` records `deliverAs`; `steer` pushes are held
  only while compacting; `registerPushFlush`'s settle flush is gated on the
  flag. The held queue is a small `push | raw` union so `ask.ts`'s
  `ws-thread-summary` shares the same hold. `injectDiscussionSummary`
  holds on the general `!isOwningAgentIdle()` predicate, so an ordinary
  mid-turn `/done` is now delivered by the post-settle flush instead of
  Pi's in-run followUp queue; this deliberate widening is recorded in a code
  comment.
- `goal-loop.ts` owns `releaseAfterCompaction` (idempotent on the flag),
  called from deferred `session_compact` / `session_compact_failed`
  (`setImmediate`), the lever's `onComplete` / `onError`, and the
  `agent_start` backstop. `registerGoalLoop` returns a shutdown handle that
  `session_shutdown` uses to reset the flag and markers.
- Review found that the ticket's auto-compaction premise did not hold:
  Pi's threshold auto-compaction that ends the turn emits `session_compact`
  and then `agent_settled` with only microtask hops in between, so the
  settle is consumed as `waiting` and, with no lever `pendingRearm`, nothing
  re-armed the loop. The release routine now also replays a settle that was
  swallowed while compacting (`settleSwallowedWhileCompacting`), sent with
  `deliverAs: "followUp"` because the preceding flush may have started a
  turn synchronously; the lever branch consumes both markers so the lever
  case still sends exactly one reminder, and both markers are cleared on the
  not-idle branch and the `agent_start` backstop. A replayed force-stop
  clears the status footer. Accepted narrow gap, comment only: a non-lever
  compaction whose `session_compact` emit is skipped by Pi's
  `savedCompactionEntry` guard has no completion callback.
- Tests: 836 pass (811 before the ticket). Spec anchors
  `{#260904-pi-report-to-lead-channel}`,
  `{#260904-pi-goal-loop-arming-settled-levers}`,
  `{#260904-pi-goal-loop-model-driven-compaction}` and the guide verb-table
  line amended.
- Live check (owner-run) still pending: repeat both dogfood runs in a fresh
  Pi session.

### Phase 2: Carry the lever's string forward verbatim

Keep `carry_forward` in goal-loop state when the lever fires; fold it
verbatim under the fixed heading into the re-arm reminder on both the
success and failure paths; clear it once sent. Tests: the reminder after
`session_compact` contains the exact string once; a reminder with no
pending carry-forward has no heading; the string is cleared after one
reminder. Amend the goal-loop anchor in `pi-adapter-runtime` (lever
sub-anchor) to state the verbatim guarantee. Live check (owner-run):
`goal-compact-and-continue` with a distinctive sentence and confirm it
appears byte-for-byte in the first message after compaction.

### Result (bfcf850b) - 2026-09-06

Source/tests landed in `bfcf850b`; the existing
`{#260904-pi-goal-loop-model-driven-compaction}` spec passage was updated in
`f873268f`.

- The lever captures the raw string before calling Pi's compaction API while
  still passing it unchanged as `customInstructions`. The common reminder
  sender includes it once on success or failure, preserving empty strings,
  Unicode, tabs, leading/trailing whitespace, and mixed newlines. Consumption
  happens after dispatch returns; a synchronous send failure retains it.
- Carry lifetime is independent of compaction re-arm markers: busy release,
  turn-start backstop clearing, and idle/child yields preserve it for an
  eligible ordinary reminder. Goal replacement, terminal levers, runaway
  force-stop, and shutdown discard unsent carry. No durable persistence was
  added.
- The landed delayed settle orchestration, reducer decisions, push priority,
  and boundary guard remain unchanged. Delivery means the next eligible goal
  reminder, not an arbitrary first post-compaction message when held pushes or
  owner input take priority. The adjacent stale spec wording about sending
  directly on release was reconciled to that existing timing contract. No
  design deviation or source-scope expansion was needed.
- Verification: tests-first observed 11 missing-carry failures; focused
  goal-loop tests then passed 118/118 and the full Pi suite passed 894/894.
  Tests ran with `WS_PI_SPAWN_ROLE` unset in the test subprocess to exercise
  the lead-only loop rather than inherit the implementer's worker role. Full
  outputs were read. Spec index and scoped documentation checks passed.
- Independent review, reported by the lead: clean, 0 findings; focused
  118/118 and full Pi 894/894 independently passed. No unresolved review
  findings or mental-model update is needed: the non-obvious carry-lifetime
  invariant is now in the authoritative spec.
- Owner-run Phase 1 and Phase 2 live checks remain pending. Automated tests
  and review do not satisfy these live gates; implementation and spec work
  are recorded here without claiming the ticket is complete.

## Blocked

2026-09-06 — Waiting for owner-run live verification; not eligible for ticket
closure or another automated implementation selection. Both phase plans
explicitly require these live checks, and neither has recorded completion:

1. **Phase 1:** In fresh Pi sessions, repeat the two Background dogfood cases:
   the near-empty, one-word `pong` goal and the filled-session compaction
   lever run. Confirm post-lever conversation is not replaced by a late
   summary, and exercise a child report arriving during compaction to confirm
   it is delivered afterwards. Record the failure-path outcome for the
   near-empty session as well as the successful filled-session outcome.
2. **Phase 2:** In a fresh Pi session with enough context to compact, call
   `goal-compact-and-continue` with a distinctive sentence. In an otherwise
   quiet run, inspect the first post-compaction message (the goal reminder)
   and compare its carried payload byte-for-byte with the supplied string.
   If held pushes or owner input precede it, inspect the first eligible goal
   reminder instead, per the landed timing contract; neither model paraphrase
   nor a TUI notification proves delivery.

The owner must provide the live observations before this block can be cleared
and closure reassessed. These checks have not been performed or waived; keep
this ticket open and skip it in the automated ready-queue drain meanwhile.
