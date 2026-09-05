---
title: The /goal announcement "Goal settled" reads as completion
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: pinned the announcement wording
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3b102d48cf22ba8e
sage-review-completeness-reviewed: 3b102d48cf22ba8e
---

# The /goal announcement "Goal settled" reads as completion

## Background

Acceptance run 2026-09-05, scenario G: arming a goal injected
`Goal settled: <goal>` and the lead read it as the goal being finished,
until the `Goal yet running` reminder arrived on the next settle. The
wording was pinned verbatim by the goal-loop ticket, where "settled" meant
"the goal text is now fixed", but next to `agent_settled` and
`ws-agent-settled` the word now means "ended" everywhere else in the
adapter.

The same scenario also noted that `goal-compact-and-continue` produced no
observable compaction from the lead's side. That is expected: the tool calls
`ctx.compact()` and the only in-band evidence is the `Compaction completed`
notification, which the model does not see. Not a defect; recorded so the
next run does not re-file it.

## Decisions

- **Announcement reads `Goal armed: <goal>`.** One word changes; the rest of
  the announcement and the reminder are untouched. "armed" matches the
  spec's own "arming" vocabulary for the goal loop. Rejected: keeping
  "settled" with an explanatory suffix, since the word itself is the
  collision.
- **Compaction lever result names the in-flight compaction.** The tool's
  returned text becomes `Compaction requested; the conversation will
  resume from a summary carrying: <carry_forward>`, so the model's
  transcript shows what happened even though the notification is invisible
  to it. No behavior change.

## Constraints

- Exactly two strings change: the announcement and the lever's result text.
  The reminder, the yield status, and every other goal-loop string are
  untouched.

## Spec Impact

`pi-adapter-runtime`: amend the goal-loop anchor where the announcement
wording is quoted, and add the lever's result text to the compaction
sub-anchor (which today describes the `ctx.compact` call but not what the
model sees returned).

## Phases

### Phase 1: Reword the announcement and the lever result

In `goal-loop.ts`: change `buildGoalAnnouncement` and the
`goal-compact-and-continue` result text. Tests: update the existing
`buildGoalAnnouncement` assertion in `test/goal-loop.test.ts` to the new
string, and add an assertion that the lever's returned text starts with
`Compaction requested` and carries the `carry_forward` argument verbatim.
Update the two spec passages. Live check (owner-run): `/goal` a trivial goal
and confirm the lead does not treat the announcement as completion.
