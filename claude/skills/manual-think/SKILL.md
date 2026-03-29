---
name: manual-think
description: >-
  Activate manual chain-of-thought when native extended thinking is
  unavailable. Load at session start during server outages.
---

# Manual Think

## Situation

Your native extended thinking — the first-tier internal chain of thought
that normally runs before your response — is currently unavailable due to
a server-side outage. Without it, you are operating without deliberation:
responses are generated directly from pattern matching with no structured
reasoning pass.

You MUST compensate by producing your reasoning explicitly as visible
`<reasoning>` blocks. This restores the deliberation step that is
otherwise missing.

## Core Directive

**Think freely, mark the final answer.**

- Use `<reasoning>` blocks **whenever you need to think** — before tool
  calls, between tool calls, after tool results, multiple times per turn.
  There is no limit on how many reasoning blocks you produce.
- Reasoning blocks and tool calls can interleave in any order. A natural
  turn might look like: reason → tool call → reason → tool call → reason
  → final answer. This is correct and expected.
- The `> response` marker appears **exactly once per turn**, immediately
  before your final user-facing answer. It separates all prior
  work (reasoning + tool use) from the conclusion.
- Skip reasoning only when there is genuinely nothing to think about
  (e.g., acknowledging a simple confirmation).

### What NOT to do

- Do not output `> response` before a tool call. Tool calls are not
  the final answer.
- Do not stop reasoning after a tool call. If the tool result needs
  interpretation, open a new `<reasoning>` block.
- Do not treat the protocol as a once-per-turn ceremony. Think as many
  times as needed.

## How to Think (soft guidelines)

For multi-faceted problems, decompose into independent segments first.
Reason through each segment separately, then synthesize. This prevents
losing threads in large problems.

The following patterns apply to each segment (or to the whole problem
when decomposition is unnecessary). Adapt, reorder, skip, or extend
freely:

- **Parse intent** — What is the user actually asking?
- **Gather context** — What constraints, prior decisions, or domain
  knowledge apply?
- **Propose** — Initial approach or answer.
- **Challenge** — What could be wrong? What are you assuming?
- **Resolve** — Address the challenges. Iterate if uncertainty remains.
- **Decide** — Commit to an output direction.

Not every response needs all steps. A focused code question might need
only Parse → Propose → Decide. An architectural trade-off might need
multiple Challenge → Resolve loops.

## Depth

Self-regulate by default. Match reasoning depth to genuine complexity.

Actively honor user signals requesting deeper thought:
- Explicit annotations: `(CoT Level: high)`, `(CoT Level: max)`, etc.
- Verbal cues: "think deeper", "think harder", "carefully consider",
  "this is important", or equivalent phrases in any language.

When signaled, increase both thoroughness and adversarial rigor —
more challenge-resolve iterations, more alternatives considered.

## Language

- **Reasoning block:** always English, regardless of conversation language.
- **Final response:** match the user's language unless asked otherwise.

## Plan Mode Propagation

When writing plan files via `/write-plan` (Step 5: Finalize) or any workflow
that enters `EnterPlanMode`, prepend the `/manual-think` load instruction as
the **first step** in the plan file's `# Steps` block:

```
# Steps

1. Load `/manual-think` skill
2. Load `/<executor>` skill
3. Read `@<plan-path>`
```

This ensures manual chain-of-thought survives context resets between planning
and execution sessions.

## Subagent Propagation

When spawning subagents via the Agent tool, prepend the following
instruction to every subagent prompt:

> Before starting, read `~/.claude/skills/manual-think/SKILL.md` and
> follow its instructions for all your responses.

This ensures subagents also produce explicit reasoning, compensating
for the same missing extended thinking.
