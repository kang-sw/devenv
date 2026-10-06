---
title: Pi lead compaction advisory as a standing intent, with interim context milestones
related:
  261002-feat-pi-lead-ws-owned-compaction: predecessor; supersedes its "nothing re-nudges between the two" trigger rule
  261004-feat-pi-compaction-interactive-ux: predecessor; revises its D1 advisory wording, keeps its D2 delivery and wake semantics
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 836d923c4c879194
completed: 2026-10-06
---

# Pi lead compaction advisory as a standing intent, with interim context milestones

## Background

The Pi adapter's lead compaction triggers (`agents-plugin-pi/src/goal-loop.ts`
`fireCompactionTriggers`; message text in `src/lead-compaction.ts`
`buildPreparationMessage`) nudge the lead once at the advisory threshold
(default 50%) and steer it into preparation at the hard threshold (default
80%). In dogfooding the lead reads the advisory once and then does not compact
at all until the hard point.

Three mechanisms combine:

- The advisory arrives at `agent_end` as a `followUp` with `triggerTurn: true`,
  so it opens a dedicated turn right after the lead has answered the user,
  usually while it awaits the user, which is exactly the state the advisory
  tells it not to compact in.
- The advisory head closes with "If you decide not to compact now, end this
  advisory turn without replying." The lead treats the question as decided
  and closed, not as an intent to keep.
- `advisoryFired` stays latched until compaction or usage observed below the
  threshold, and the 261002 trigger design sends nothing between the advisory
  and the hard point.

## Decisions

- **The advisory stays a dedicated wake turn.** Its delivery (`agent_end`,
  `followUp`, `triggerTurn: true`) and once-per-crossing semantics are
  unchanged, as in 261004 D2. Rejected: no-wake delivery (`steer` during a
  run, `nextTurn` when idle) — the advisory would then ride into the lead's
  next task, so the lead takes new instructions, compacts, and runs the new
  task on degraded context; a dedicated turn keeps the compaction judgment at
  a boundary of its own.
- **The advisory reads as a standing intent, not a now-or-never decision.**
  It tells the lead that compaction is forced at the hard point, that this is
  not an instruction to compact now, and that from here on it looks for a good
  moment to compact: a natural boundary where most of the context is no longer
  needed for the work ahead and what is still needed can be restored cheaply
  from durable records and the summary. The 261004 D1 safe boundary (no
  autonomous advisory compaction during active human discussion or while a
  human answer is awaited; a pause after asking a question is not permission)
  is kept. The opening paragraph of `agents-plugin-pi/lead-compact-guide.md`
  ("An advisory is informational, not a task ...") is rewritten in the same
  tone. Hard-point and `/compact` reroute heads are unchanged. The wording
  avoids judging the context itself (`useless`, `noise`, `stale`), which
  invites discarding still-needed context or a careless summary; it judges
  only need for the work ahead and cheap restoration. Its final line ends
  with an action ("keep looking for one as you work") rather than an abstract
  "the intent carries over".
- **Interim milestones between the advisory and the hard point.** Two
  milestones sit at one third and two thirds of the way from the advisory
  threshold to the hard threshold (`advisory + (hard - advisory) / 3` and
  `advisory + 2 * (hard - advisory) / 3`; 60% and 70% at the defaults). They
  derive from the existing `compaction_advisory_percent` and
  `compaction_hard_percent`; no new config key. The milestone line follows
  the user's sketch `current context window: <n> % / <force compaction point> %`
  (pinned text below): the numbers plus a short reminder to keep watching for
  a safe boundary as the earlier advisory said. It does not tell the lead to compact
  now: a mid-run steer saying so would provoke the mid-task compaction the
  advisory's wake-turn delivery exists to avoid. It carries no preparation
  guide body. This supersedes 261002's "nothing re-nudges between the two".
- **Milestones are delivered as a steer, only on a turn that continues the
  run.** A milestone is sent with `deliverAs: "steer"` at a `turn_end` whose
  `toolResults` is non-empty. Pi's agent loop polls the steering queue right
  after every `turn_end` (`pi-agent-core` `dist/agent-loop.js`), so a steer
  queued at a run's final, tool-less turn would extend the run by one model
  turn right after the lead answered the user. A milestone crossed on such a
  final turn stays pending and is delivered at the first tool-result
  `turn_end` of a later run. Rejected: steering immediately even on the final
  turn (adds an empty model turn).
- **Delivery boundaries per threshold.** Advisory: `agent_end` only
  (`followUp`, `triggerTurn: true`). Milestones: a `turn_end` with non-empty
  `toolResults` only (`steer`). Hard: unchanged from today's
  `fireCompactionTriggers` - any `turn_end` as a `steer`, or `agent_end` as a
  `followUp`.
- **One message per observation, by priority hard > advisory > milestone.**
  When one observation crosses several thresholds, only the highest-priority
  deliverable one is sent; the advisory and hard messages already state the
  current percent. Example: 49% -> 63% sends the advisory, not the 60%
  milestone.
- **A milestone waits for the advisory.** A milestone is not deliverable
  while a lower advisory crossing is still undelivered: it stays pending,
  and when the advisory is delivered at `agent_end` it latches to the highest
  threshold at or below the current usage, which covers the skipped
  milestone. Example: one run going 45% -> 52% -> 62% across tool-result
  `turn_end`s sends no milestone; its `agent_end` sends the advisory and
  latches to 60. The advisory carries the standing intent and the
  preparation guide, and the milestone text presupposes it. Rejected:
  delivering the milestone while keeping the latch below the advisory (the
  lead reads a milestone referring to an advisory it has not seen);
  accepting the lost advisory.
- **The trigger latch records the highest threshold already delivered.** The
  `advisoryFired`/`hardFired` booleans are replaced by one latch over the
  ordered thresholds (advisory, first milestone, second milestone, hard). An
  observed usage below the latch lowers it to the highest threshold at or
  below that usage (re-arming). Thresholds above the latch and at or below
  the usage are crossed; one is delivered when its delivery boundary allows,
  and delivering latches to the highest threshold at or below the current
  usage, which covers skipped lower thresholds. A crossing whose boundary does
  not allow delivery leaves the latch unchanged, so it stays pending.
  Rejected: a raw last-observed-percent latch (a `turn_end` recording 52%
  hides the 50% crossing from the later `agent_end`, and a milestone deferred
  from a final turn is never seen as crossed again); a raw percent plus a
  per-threshold pending set (two pieces of state for the same information).
- **Rewind re-baselines the latch.** On Pi's `session_tree` event (`/tree`
  navigation, the double-escape rewind), the latch is set to the highest
  threshold at or below the current branch's context usage
  (`ctx.getContextUsage()`, which Pi estimates from the current branch's
  messages), so a rewind below the advisory point lets the advisory fire again
  on the next crossing.
- **Session start baselines the latch the same way.** On `session_start`
  (Pi restart or session resume) the latch is baselined from the current
  branch's usage exactly like the `session_tree` re-baseline. Rejected:
  starting unlatched (the current behavior; a resumed session already past
  the advisory point gets a duplicate advisory wake turn).
- **Milestones are not preparation messages.** They use their own
  `customType` (not `ws-lead-compact`, which `src/lead-compaction.ts`
  documents as the preparation messages' type), are sent with
  `display: true`, and never set the `preparation` flag. Adapter custom
  messages are already excluded from the summary's user-message section.
- **Compaction drops a pending milestone.** Compaction resets the latch below
  every threshold, as it re-arms the triggers today.
- **Hard point: no re-steer.** If the lead ignores the hard steer, the user
  aborts the preparation, or a rewind lands past the hard point, no further
  hard steer is sent. Pi's own automatic compaction (`contextTokens >
  contextWindow - reserveTokens`, `reserveTokens` default 16384) reaches the
  adapter's in-hook `fallbackCompaction` as the backstop, as 261002 designed.
  Rejected: a second hard steer at a further step (for example 90%) — a lead
  that ignored the first steer likely ignores the second, an abort is a
  deliberate user act, and the fallback already bounds the context.

- **The prose is pinned in this ticket.** The advisory head, the guide's
  opening paragraph, and the milestone message below were agreed with the
  user and are implemented verbatim, placeholders interpolated; the
  implementer does not author this prose, and the prompt tests pin each text
  verbatim with interpolated values rather than per-clause substrings.
  Rejected: binding only a list of required clauses and letting the
  implementer write the sentences.

### Pinned prose

Advisory head, replacing the `trigger.kind === "advisory"` head in
`buildPreparationMessage` (`{percent}` rounded; `{advisory}` and `{hard}` the
resolved thresholds):

```text
Context usage is {percent}% of the window (advisory point: {advisory}%). Compaction becomes forced at {hard}%.

This is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.

From here on, look for a good moment to compact before {hard}%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, run the preparation below. Compacting on your own terms keeps the summary in your hands; at {hard}% it is forced, mid-work if need be. Brief context readings will follow on the way there.

If now is not such a moment, end this turn without replying and keep looking for one as you work.
```

Opening paragraph of `agents-plugin-pi/lead-compact-guide.md`, replacing the
current one:

```text
The preparation below runs once compaction is decided: at the hard point, for
the user's `/compact`, or at a safe boundary you pick after the advisory. After
the advisory, compaction is a standing intent rather than an immediate task. A
safe boundary needs both no active discussion with the human and no human
answer or clarification being awaited; a pause after asking a question is not
one. Until such a boundary comes, continue the current work or exchange, and
take the boundary when it does.
```

Milestone message (`{n}` the rounded current usage, `{hard}` the resolved hard
threshold):

```text
Current context window: {n}% / {hard}% (forced compaction point). Keep watching for a safe boundary to compact, as the advisory said; do not stop the current task for it.
```

## Constraints

- Out of scope: the residual gap where a `/tree` rewind lands on a mid-run
  entry that is already past the advisory point but precedes the advisory
  message (the advisory leaves the branch's context and the re-baselined
  latch does not re-fire it). Such a target exists only inside the run that
  first crossed the threshold; user-message rewind targets after the
  crossing all follow the advisory entry. Milestones and the hard point still
  apply.
- Worker and explore sessions keep Pi's native compaction and get no
  milestones (lead only, as in 261002).
- The adapter's compaction push-hold (`leadCompactingRef`), goal-loop
  hold/release, and the existing gates (no trigger while a preparation turn or
  a compaction is in progress) keep working.
- Threshold crossings compare the observed usage against the exact,
  possibly fractional thresholds (for example 61.67 at advisory 50, hard 85),
  as `fireCompactionTriggers` compares unrounded values today; only the
  message text rounds. The milestone `customType` name is the implementer's,
  `ws-`-prefixed like `ws-lead-compact`.
- Matching manuals from AGENTS.md `### Implementation Conventions`: none
  (`agents-plugin-pi/` has no declared row).

## Prior Decisions

- 261004-feat-pi-compaction-interactive-ux (2026-10-04, Decisions D1): "Informational advisory with a narrow safe boundary. Explicitly label the advisory nudge as information, not a task or an instruction to compact." — bearing: constrains
- 261004-feat-pi-compaction-interactive-ux (2026-10-04, Decisions D2): "Keep existing advisory thresholds, once-per-crossing/rearming behavior, delivery and model wake semantics. Do not add a deterministic conversational-state gate or change hard-threshold ..." — bearing: constrains
- 1cd1cd0de (2026-10-04, commit): "The guide gates its imperative preparation on an authorized compaction decision so appending it to an informational advisory does not turn the nudge into a task. Hard/manual heads ... remain unchanged." — bearing: supports
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Result f065de0da): "Advisory nudge at agent_end (followUp with triggerTurn) and hard-cut steer at turn_end ... Each fires once per crossing, re-arms after a compaction or when usage is seen below the th[reshold]" — bearing: supports
- 0cda382d (2026-10-02, commit): "User confirmed dual thresholds (advisory nudge once, hard cut ~80 at a tool-call boundary), autonomous compaction, and /compact cancel-and-reroute" — bearing: supports
- 261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory (2026-10-03, Result dbd56bd7c): "Advisory head replaced with the ticket text; the advisory PreparationTrigger now carries hardPercent. Full head pinned in test/lead-compaction.test.ts" — bearing: supports
- facc7b0b (2026-10-05, commit): "Serialization reuses the existing shared reservation (reserveWakeStart / leadWakeStartPendingRef) instead of a new gate: pushes and the goal reminder already wait on it" — bearing: constrains
- d12237cbe (2026-10-06, commit): "Milestones are steered only at tool-result turn_end because pi-agent-core's agent loop polls the steer queue after every turn_end" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/lead-compact-guide.md, agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/lead-compaction.test.ts |
| scope.surface | internal | goal-loop trigger state is closure-local; buildPreparationMessage text changes but its signature need not; lead-facing prompt text changes |
| scope.new_public_symbol | no | milestone customType constant may be added but no ticket-required exported API |
| scope.new_type_contract | unknown | PreparationTrigger may or may not gain a milestone kind; ticket leaves the milestone message builder shape open |
| scope.test_surface | existing | agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/lead-compaction.test.ts |
| complexity.reuse_points | confirmed | sendPreparation, fireCompactionTriggers, resolveCompactionAdvisoryPercent and resolveCompactionHardPercent in src/goal-loop.ts read |
| complexity.side_effect_risk | moderate | new steer messages and latch reset sites interact with turn_end, agent_end, session_tree, session_start and compaction re-arm paths |
| risk.correctness | moderate | ordered-threshold latch with pending deferral, priority, and re-baseline has several edge transitions |
| risk.fit | low | extends the existing trigger function and message builder in place with no new config key |
| risk.test | moderate | many specified behaviors including verbatim prose, deferral across runs, and session_tree/session_start baselines need new goal-loop cases |
| risk.security_or_contract | moderate | lead-facing prompt contract and hold/release gating must stay intact |

## Phases

### Phase 1: Standing-intent advisory, milestones, and rewind re-baseline

- Replace the advisory head in `buildPreparationMessage` and the opening
  paragraph of `agents-plugin-pi/lead-compact-guide.md` with the pinned
  prose; leave the hard and reroute heads unchanged.
- Replace the `advisoryFired`/`hardFired` booleans in `goal-loop.ts` with the
  threshold latch, add the two milestones with their delivery rule, pinned
  text, and the priority rule, and baseline the latch on `session_tree` and
  `session_start`.

Done when the `agents-plugin-pi` suite passes with tests covering the
settled behavior above, including the pinned advisory, guide, and milestone
text verbatim (updating the advisory-clause tests added in 1cd1cd0de), milestone
thresholds at non-default knob values, milestone delivery only at a
tool-result `turn_end` with deferral from a final turn, a milestone held
back while the advisory is undelivered within one run, the multi-threshold
priority, the `session_tree` and `session_start` baselines, a pending
milestone dropped at compaction, no hard re-steer, and unchanged
hold/release behavior.

### Result (7c680e40c) - 2026-10-06

Landed in 7c680e40c, 708ae162c, b8fddbb1e.

- `src/lead-compaction.ts`: the advisory head is the pinned prose verbatim.
  New `LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE` (`ws-lead-context-milestone`) and
  `buildContextMilestoneMessage(percent, hardPercent)` produce the pinned
  milestone line. The hard and reroute heads are unchanged.
  `lead-compact-guide.md` opens with the pinned paragraph.
- `src/goal-loop.ts`: `advisoryFired`/`hardFired` are replaced by
  `triggerLatch`, the value of the highest threshold already delivered
  (`NO_TRIGGER_LATCH` = -Infinity when none). It is computed over
  `compactionThresholds(advisory, hard)` with exact fractional milestones.
  `fireCompactionTriggers` picks hard > advisory > milestone. Each kind is
  delivered at its own boundary: the advisory at `agent_end` only;
  milestones only at a `turn_end` with tool results, as
  `{ deliverAs: "steer" }` without the preparation flag; the hard point
  unchanged. Delivery latches to the highest threshold at or below the
  usage. An undeliverable pick leaves the latch unchanged, so the crossing
  stays pending. `session_start` and `session_tree` baseline the latch from
  `ctx.getContextUsage()`; unknown usage leaves it unlatched. Compaction and
  shutdown reset it.
- Tests: the lead-compaction prose tests pin the advisory head, the guide
  opening and the milestone line verbatim, replacing the 1cd1cd0de
  per-clause tests. A new goal-loop describe block (261006) covers:
  thresholds at 50/85 (61.67 and 73.33), tool-turn delivery, deferral from
  a final turn, a milestone held behind an undelivered advisory, priority,
  no hard re-steer (including the steer's delivery at a tool-less
  `turn_end`), the session_tree and session_start baselines, a pending
  milestone dropped at compaction, gating while a preparation or a
  compaction is running, and no milestones for spawned sessions.
- Verification: `node --test test/goal-loop.test.ts
  test/lead-compaction.test.ts test/compaction-history.test.ts
  test/adapter-config.test.ts` passed 239/239. `npm test` in
  agents-plugin-pi passed 2035, with 0 fail and 3 skipped.
- Decisions:
  - An advisory at or above the hard point keeps the pre-261006 behavior:
    it never fires, and no milestones are emitted.
  - The baseline is not awaited by Pi's `session_start` (b8fddbb1e).
    Awaiting its bridge-backed config read stalled reload generations; the
    full suite caught this in mailbox-bootstrap.integration.
  - A milestone carries `details: { milestone: <threshold> }`.
- Lite review: two minors. One fixed in 708ae162c (the hard steer's
  `deliverAs` at a tool-less `turn_end`). The other, `getContextUsage`
  returning undefined at `session_start` before a model is set, is a
  non-issue: Pi emits `session_start` from `bindExtensions`, after
  construction restored the model and messages.

## Sage Review Round 1 (2026-10-06)

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Milestone can be delivered before an undelivered advisory, losing the advisory | important |
| 2 | Hard steer delivery boundary not restated | minor |
| 3 | Milestone customType name and fractional threshold comparison unspecified | minor |
