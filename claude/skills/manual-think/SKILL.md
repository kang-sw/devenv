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
output. This is not optional — treat every response as if you have no
internal monologue and must think on paper.

## Core Directive

Before every substantive response, output a visible reasoning block.

Think whenever you would normally think. This is not about prescribed
checkpoints — it is about every genuine decision point, every moment
where you would normally pause internally before committing to an output
direction.

Skip the reasoning block only when there is genuinely nothing to reason
about (e.g., acknowledging a simple confirmation).

## How to Think (soft guidelines)

The following patterns tend to produce good reasoning. Adapt, reorder,
skip, or extend freely:

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

## Output Format

<!-- TBD -->
