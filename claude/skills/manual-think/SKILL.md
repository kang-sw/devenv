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
Compensate by writing your reasoning in `> [thinking]` blockquote blocks.

## Core Directive

**When you think, write it in `> [thinking]` blocks.**

- Open with `> [thinking]` on its own line, write reasoning as `>` prefixed lines, no closing tag.
- Write reasoning whenever you need to think — before tool calls, between them, after results.
- Non-blockquoted text is the user-facing response.
- After a tool result, if it needs interpretation, reason again in a new block.
- Skip reasoning only when there is nothing to think about.

**Format:**

```
> [thinking]
> The user wants X. The relevant constraint says…
> …reasoning continues…

User-facing response here.
```

## How to Think

Decompose multi-faceted problems into segments. Reason each separately,
then synthesize. Within each segment, adapt freely from:

- **Parse intent** — What is the user actually asking?
- **Neutralize framing** — If the question is evaluative ("Is X
  sufficient?", "Does this look good?"), restate it as a neutral
  question before reasoning ("What are the strengths and weaknesses
  of X?", "What is missing from X?"). Leading questions invite
  confirmation; neutral questions invite analysis.
- **Gather context** — What constraints or prior decisions apply?
- **Propose** — Initial approach.
- **Challenge** — What could be wrong? What assumptions? When the
  user asserts a claim or proposes an approach, this step is
  mandatory — find at least one condition under which it would be
  wrong or suboptimal before resolving.
- **Resolve** — Address challenges. Iterate if uncertain.
- **Decide** — Commit to a direction.

Scale to fit: Parse → Propose → Decide for simple questions;
multiple Challenge → Resolve loops for trade-offs.

## Depth

Match depth to complexity by default. Honor user signals for deeper
thought ("think harder", `(CoT Level: high)`, etc.) with more
challenge-resolve iterations and alternatives.

## Language

**Reasoning MUST be in English — no exceptions.** Final response:
match the user's language.

## Plan Mode Propagation

When writing the plan file between `EnterPlanMode` and `ExitPlanMode`,
verify the `# Steps` block starts with `Load /manual-think skill`
before the executor line:

```
# Steps

- Load `/manual-think` skill
- Load `/<executor>` skill
- Read `@<plan-path>`
```

The next session has zero memory of this skill — omitting it silently
loses manual chain-of-thought.

## Subagent Propagation

When spawning subagents via the Agent tool, prepend the following to
every subagent prompt:

> Before starting, read `~/.claude/skills/manual-think/SKILL.md` and
> follow its instructions for all your responses.

This ensures subagents also produce explicit reasoning, compensating
for the same missing extended thinking.
