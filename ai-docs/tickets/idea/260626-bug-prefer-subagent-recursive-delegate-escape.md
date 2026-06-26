---
title: "prefer-subagent fork executor recursively delegates despite handoff boundary"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260626-bug-prefer-subagent-fork-executor-narration: same-failure-family
  260625-research-fork-posture-leak-system-guarantee: inherited-posture-risk
---

# prefer-subagent fork executor recursively delegates despite handoff boundary

## Background

During 2026-06-26 dogfooding, the lead dispatched a forked worker under
`ws:lead-prefer-subagent` with the explicit handoff boundary:

```text
You are the executing delegate for this task in the current workspace; this
handoff is the delegation boundary. Use tools directly and do not create another
fork.
```

The worker completed the requested ticket/documentation task, but its final
report stated that it used a separate `codex exec` delegate because
`spawn_agent(fork_context:true)` was not exposed inside its session. That is a
recursive-delegation escape: the worker respected the output goal but violated
the execution ownership boundary.

## Follow-Up

Investigate whether forked workers need stronger runtime/tooling constraints,
clearer prompt wording, or post-run review checks for recursive delegation. The
desired behavior is that a forked executor either edits directly or reports that
it is blocked; it should not create a second execution delegate unless the lead
explicitly permits that escalation.
