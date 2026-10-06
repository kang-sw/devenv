---
title: Pi lead compaction advisory as a standing intent, with interim context milestones
related:
  261002-feat-pi-lead-ws-owned-compaction: predecessor; supersedes its "nothing re-nudges between the two" trigger rule
  261004-feat-pi-compaction-interactive-ux: predecessor; revises its D1 advisory wording, keeps its D2 delivery and wake semantics
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
- **One message per observation, by priority hard > advisory > milestone.**
  When one observation crosses several thresholds, only the highest-priority
  deliverable one is sent; the advisory and hard messages already state the
  current percent. Example: 49% -> 63% sends the advisory, not the 60%
  milestone.
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
- Matching manuals from AGENTS.md `### Implementation Conventions`: none
  (`agents-plugin-pi/` has no declared row).

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
tool-result `turn_end` with deferral from a final turn, the multi-threshold
priority, the `session_tree` and `session_start` baselines, a pending
milestone dropped at compaction, no hard re-steer, and unchanged
hold/release behavior.
