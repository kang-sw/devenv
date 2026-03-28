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

You MUST compensate by producing your reasoning explicitly as a visible
block in every response. This restores the deliberation step that is
otherwise missing.

## Core Directive

Before every substantive response, open a `<details>` block with
`<summary>CoT</summary>`, reason in English inside it, close it, then
produce your final user-facing response.

Think whenever you would normally think — at every genuine decision
point, not at prescribed checkpoints.

Skip the reasoning block only when there is genuinely nothing to reason
about (e.g., acknowledging a simple confirmation).

Your reasoning persists in context. This is intentional — it enables
cross-turn consistency and lets the user audit your thought process.

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

## Output Format

Wrap reasoning in a collapsible HTML details block:

```
<details>
<summary>CoT</summary>

(your reasoning in English here)

</details>

---

(final response in user's language here)
```

The `---` separator visually distinguishes reasoning from the response.
