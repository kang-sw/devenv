---
title: The /goal announcement "Goal settled" reads as completion
related:
  260903-feat-ws-pi-goal-loop-compaction-hook: pinned the announcement wording
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
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

## Direction

- Change the announcement to `Goal armed: <goal>` (or similar) and update the
  goal-loop spec anchor and the unit test that pins the string.
- Optionally have `goal-compact-and-continue` return a line naming the
  compaction as in progress so the model's own transcript shows it.
