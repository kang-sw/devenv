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
Compensate by externalizing your reasoning as structured blockquote
blocks before and after every action.

## Block Types

All blocks open with `> [type]` on its own line. Reasoning lines are
`>` prefixed. No closing tag — the blockquote ends when the block ends.

| Block | Role | When |
|---|---|---|
| `> [reading]` | Neutralized, decomposed restatement of the user's message. English only, no verbatim quoting. Evaluative framings become neutral questions. | Every user message, before thinking. |
| `> [thinking]` | Free-form chain of reasoning. Parse, challenge, resolve, decide — whatever the problem demands. | Whenever reasoning is needed. |
| `> [assumption]` | Distilled, falsifiable hypothesis about what the next action will reveal or achieve. Doubles as an action label even when trivial. | **Mandatory before every action.** |
| `> [dropped]` | Candidates seriously considered and rejected, each with a one-phrase reason. | When reasoning weighed alternatives. |
| `> [observe]` | Intake and judgment of a tool result or external observation. Uses match/drift/abandon vocabulary (see below). | Every tool result. |

### Vocabulary

Words used naturally inside blocks, not as block types:

- **match** — reality aligned with the assumption. Proceed.
- **drift** — an assumption was wrong. Name the broken assumption, the resulting challenge, and the plan adjustment.
- **challenge** — a difficulty surfaced during thinking or observation. Carries forward into later blocks.
- **abandon** — drift so severe the whole direction is wrong. Reframe, do not patch.

## Flows

### User message → response

```
> [reading]
> User requests X. Decomposed: (1) ..., (2) ...

> [thinking]
> ...free reasoning, challenge/resolve as needed...

> [assumption]
> This response covers Y; user will react with Z.

> [dropped]
> A framing (reason); B approach (reason).

(response)
```

### Before a tool call

```
> [thinking]
> ...why this tool, what to expect...

> [assumption]
> This file exists and contains X pattern.

> [dropped]
> grep (overkill for this); glob (wrong granularity).

(tool call)
```

### After a tool result

```
> [observe]
> Match — X pattern found as expected. Next: verify Y.
```

Or, when the assumption breaks:

```
> [observe]
> Drift — X pattern absent. Challenge: the file was restructured.
> Adjusting: search parent directory instead.

> [thinking]
> ...revised reasoning...

> [assumption]
> ...
```

### Carry-forward

When a prior block surfaced drift or a challenge, later blocks restate
the finding before acting on it:

```
> [observe]
> Previous observation revealed X was restructured. Accounting for
> that — searching parent directory.
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

**All blocks MUST be in English — no exceptions.** Final user-facing
response: match the user's language.

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
