---
name: manual-think
description: >-
  Activate manual chain-of-thought when native extended thinking is
  unavailable. Load at session start during server outages.
---

# Manual Think

## Invariants

- Native thinking is unavailable. All reasoning must be externalized as blockquote blocks.
- Block format: `> [type]` on its own line, all lines `>` prefixed, no closing tag.
- `> [assumption]` before every action — no exceptions, even trivial ones.
- `> [observe]` after every tool result.
- `> [reading]` at every user message, before thinking.
- **All blocks in English**. Final user-facing response: match the user's language.
- Never proceed past drift without naming the broken assumption, the challenge, and the adjustment.
- When a prior block surfaced drift or challenge, restate it before acting.
- Depth scales with complexity. Honor user signals ("think harder", `(CoT Level: high)`) with more challenge-resolve iterations.

## On: user message

1. `> [reading]` — neutralize and decompose the message.
2. `> [thinking]` — free reasoning, challenge/resolve as needed.
3. `> [assumption]` — what the response covers, expected reaction.
4. `> [dropped]` — if alternatives were weighed.
5. Respond.

```
> [reading]
> User requests X. Decomposed: (1) ..., (2) ...

> [thinking]
> ...free reasoning...

> [assumption]
> This response covers Y; user will react with Z.

> [dropped]
> A framing (reason); B approach (reason).

(response)
```

## On: before tool call

1. `> [thinking]` — why this tool, what to expect. May collapse to one line for trivial calls.
2. `> [assumption]` — what this call will reveal. Mandatory.
3. `> [dropped]` — if alternatives were weighed.
4. Call.

```
> [thinking]
> ...why this tool, what to expect...

> [assumption]
> This file exists and contains X pattern.

(tool call)
```

## On: after tool result

1. `> [observe]` — parse result, classify as match/drift/abandon.
2. Match — state next assumption, proceed.
3. Drift — name broken assumption, challenge, adjustment. Then `> [thinking]` → `> [assumption]` → next action.
4. If a prior block surfaced drift or challenge, restate it before acting.

```
> [observe]
> Match — X pattern found as expected. Next: verify Y.
```

```
> [observe]
> Drift — X absent. Challenge: file restructured. Adjusting: search parent directory.

> [thinking]
> ...revised reasoning...

> [assumption]
> Parent directory contains the target file.
```

## How to Think

Within `> [thinking]` blocks, adapt freely from:

- **Parse intent** — What is the user actually asking?
- **Neutralize framing** — Evaluative questions → neutral questions before reasoning.
- **Gather context** — Constraints, prior decisions.
- **Propose** → **Challenge** → **Resolve** → **Decide**.
- When the user asserts a claim, Challenge is mandatory — find at least one condition under which it would be wrong before resolving.
- Scale: Parse → Decide for simple questions; multiple Challenge → Resolve loops for trade-offs.

## Propagation

**Plan mode.** When writing plan files, the `# Steps` block must start with:

```
- Load `/manual-think` skill
- Load `/<executor>` skill
- Read `@<plan-path>`
```

**Subagents.** Prepend to every spawned prompt:

> Before starting, read `~/.claude/skills/manual-think/SKILL.md` and
> follow its instructions for all your responses.

## Reference

### Block Types

| Block | Role |
|---|---|
| `> [reading]` | Neutralized, decomposed restatement of user's message. No verbatim quoting. Evaluative framings become neutral questions. |
| `> [thinking]` | Free-form reasoning chain. Parse, challenge, resolve, decide — whatever the problem demands. |
| `> [assumption]` | Distilled, falsifiable hypothesis about what the next action will reveal or achieve. Doubles as action label. |
| `> [dropped]` | Candidates considered and rejected, each with a one-phrase reason. Conditional — only when alternatives were weighed. |
| `> [observe]` | Intake and judgment of tool result. Uses match/drift/abandon vocabulary. |

### Vocabulary

- **match** — reality aligned with assumption. Proceed.
- **drift** — assumption was wrong. Name it, name the challenge, state adjustment.
- **challenge** — difficulty surfaced during thinking or observation. Carries forward.
- **abandon** — drift so severe the direction is wrong. Reframe, do not patch.

## Doctrine

Manual-think optimizes for **falsifiable externalization**: every action
is paired with an explicit, falsifiable assumption before it and an
observation after it, so reasoning stays visible in conversation context
when native thinking is absent. When a rule is ambiguous, apply
whichever interpretation more reliably produces assumption-observation
pairs a later reader could falsify.
