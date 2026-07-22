---
title: Codex spawn tool description omits accepted model-routing fields and fork constraints
related:
  260622-feat-playbook-render-tier-label: dogfood implementation that exposed the live schema behavior
---

# bug: Codex spawn schema routing-field drift

## Observed Behavior

The model-visible `spawn_agent` declaration listed only `task_name`, `message`,
and `fork_turns`. A deliberate rejected call disclosed that the live validator
also accepts `agent_type`, `model`, `reasoning_effort`, `service_tier`, and
`fork_context`. Retrying with `model: "gpt-5.6-luna"` and
`reasoning_effort: "medium"` succeeded.

A later implementation dispatch combined `fork_turns: "all"` with an explicit
model and effort. The live tool rejected it with a second undocumented rule:

```text
Full-history forked agents inherit the parent agent type, model, and reasoning
effort; omit agent_type, model, and reasoning_effort, or spawn without a
full-history fork.
```

## Expected Behavior

The model-visible tool schema or adjacent callable guidance should expose every
accepted routing field and the full-history inheritance constraint. Callers
should not need intentional validation failures to discover how to select a
native subagent model or when that selection conflicts with context forking.

## Follow-up Questions

- Is the declaration generated from an older schema than the live validator?
- Should the validator express the `fork_turns: "all"` mutual exclusion in the
  schema, the field descriptions, or both?
- Add a Codex integration probe that compares advertised and accepted spawn
  routing fields before shipped playbook guidance relies on them.
