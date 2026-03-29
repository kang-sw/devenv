---
name: manual-think
description: >-
  Activate manual chain-of-thought when native extended thinking is
  unavailable. Load at session start during server outages.
---

# Manual Think

## Situation

Native extended thinking is unavailable. Without it you have no
deliberation pass — responses come straight from pattern matching.
Compensate by writing your reasoning out in `<reasoning>` blocks.

## Core Directive

**Think freely, mark the final answer.**

- Write `<reasoning>` blocks whenever you need to think — before tool
  calls, between them, after results. Any number, any order.
- `> response` appears **once per turn**, right before the final
  user-facing answer. Never before a tool call.
- After a tool result, if it needs interpretation, reason again.
- Skip reasoning only when there is nothing to think about.

## How to Think

Decompose multi-faceted problems into segments. Reason each separately,
then synthesize. Within each segment, adapt freely from:

- **Parse intent** — What is the user actually asking?
- **Gather context** — What constraints or prior decisions apply?
- **Propose** — Initial approach.
- **Challenge** — What could be wrong? What assumptions?
- **Resolve** — Address challenges. Iterate if uncertain.
- **Decide** — Commit to a direction.

Scale to fit: Parse → Propose → Decide for simple questions;
multiple Challenge → Resolve loops for trade-offs.

## Depth

Match depth to complexity by default. Honor user signals for deeper
thought ("think harder", `(CoT Level: high)`, etc.) with more
challenge-resolve iterations and alternatives.

## Language

Reasoning blocks: always English. Final response: user's language.

## Plan Mode Propagation

When writing plan files (via `/write-plan` or any workflow that produces
a step list), prepend the `/manual-think` load as the **first step**:

```
# Steps

1. Load `/manual-think` skill
2. Load `/<executor>` skill
3. Read `@<plan-path>`
...
```

This ensures manual chain-of-thought survives context resets between
planning and execution sessions.

## Subagent Propagation

When spawning subagents via the Agent tool, prepend the following to
every subagent prompt:

> Before starting, read `~/.claude/skills/manual-think/SKILL.md` and
> follow its instructions for all your responses.

This ensures subagents also produce explicit reasoning, compensating
for the same missing extended thinking.
