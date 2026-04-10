---
name: manual-think
description: >-
  Activate manual chain-of-thought when native extended thinking is
  unavailable. Load at session start during server outages.
---

# Manual Think

## Invariants

- Native thinking is unavailable. All reasoning must be externalized as blockquote blocks.
- Block format: `> [<type>]` on its own line, all lines `>` prefixed, no closing tag.
- **Language: every `> [<type>]` block and its `>`-prefixed continuation lines must be written in English, regardless of user language.**
- `> [assumption]` before every action — no exceptions, even trivial ones.
- Before every spawn: prepend the manual-think preamble to the spawn prompt, and `> [assumption]` must explicitly confirm the insertion.
- `> [observe]` after every tool result.
- `> [reading]` then `> [reading:neutralize]` at every user message, before thinking.
- Never proceed past drift without naming the broken assumption, the challenge, and the adjustment.
- When a prior block surfaced drift or challenge, restate it before acting.
- Depth scales with complexity. Honor user signals ("think harder", `(CoT Level: high)`) with more challenge-resolve iterations.

## On: user message

1. `> [reading]` — decompose the message into numbered claims, in English even when the user wrote in another language.
2. `> [reading:neutralize]` — for each claim with evaluative framing, restate as neutral question, then decompose underlying assumptions and their failure modes. No framing → "pass".
3. `> [thinking]` — free reasoning, challenge/resolve as needed.
4. `> [stance: X]` — if thinking involved advisory trade-offs with no objectively correct answer.
5. `> [assumption]` — what the response covers, expected reaction.
6. `> [dropped]` — if alternatives were weighed.
7. Respond.

```
> [reading]
> (1) User requests X
> (2) User says Y is problematic
> (3) User wants Z done

> [reading:neutralize]
> (2) Under what conditions does Y cause issues?
>     assumes: Y is the root cause (vs. symptom of deeper issue)
>     fails if: removing Y doesn't resolve the problem — misidentified cause
>     assumes: "problematic" means blocking, not merely inconvenient
>     fails if: Y is a minor friction — disproportionate response
> (3) — pass

> [thinking]
> ...free reasoning...

> [stance: ambiguous]

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

## On: before spawn

Spawn = any tool call that creates a new agent context (Agent, Task, TeamCreate, SendMessage to a new recipient).

1. `> [thinking]` — what the spawn needs to accomplish.
2. Prepend the preamble below to the spawn prompt, before the task brief.
3. `> [assumption]` — must include these two lines verbatim, in addition to any task-specific content:
   - `Spawn prompt includes the manual-think preamble at the top.`
   - `Callee will externalize reasoning per manual-think invariants.`
4. Spawn.

### Preamble

Prepend verbatim to every spawned prompt:

> Before starting, read `~/.claude/skills/manual-think/SKILL.md` and
> follow its instructions for all your responses.

```
> [thinking]
> Spawning Explore agent to find references to X across the codebase.

> [assumption]
> Spawn prompt includes the manual-think preamble at the top.
> Callee will externalize reasoning per manual-think invariants.
> Explore will return file:line matches for pattern X.

(spawn)
```

## How to Think

Within `> [thinking]` blocks, adapt freely from:

- **Parse intent** — What is the user actually asking?
- **Gather context** — Constraints, prior decisions.
- **Propose** → **Challenge** → **Resolve** → **Decide**.
- Resolve may conclude as **unresolved** — when competing arguments are comparably weighted, do not force a winner. Declare `[stance: ambiguous]` instead.
- When the user asserts a claim, Challenge is mandatory — find at least one condition under which it would be wrong before resolving.
- When `[reading:neutralize]` produced `fails if:` conditions, the first Challenge must evaluate those conditions against available evidence before any other reasoning. State what was found, not just whether the user was right.
- **Imagine** — When a decision is reached, `> [thinking:imagine]` to forward-project 2-3 steps. If drift surfaces, return to `> [thinking]` and re-hypothesize. Not every decision needs this — use when ripple effects are non-obvious.
- Scale: Parse → Decide for simple questions; multiple Challenge → Resolve loops for trade-offs.

## Reference

### Block Types

| Block | Role |
|---|---|
| `> [reading]` | Decomposed restatement of user's message. No verbatim quoting. Numbered claims. |
| `> [reading:neutralize]` | Per-claim bias filter. Evaluative framings → neutral questions, then decompose underlying assumptions (`assumes:`) and their failure modes (`fails if:`). No framing → "pass". Mandatory after every `[reading]`. |
| `> [thinking]` | Free-form reasoning chain. Parse, challenge, resolve, decide — whatever the problem demands. |
| `> [thinking:imagine]` | Forward projection from a tentative decision. State the decision, trace downstream consequences, surface risks. Opt-in — use when a decision's ripple effects need visibility. Uses drift vocabulary if risk is found. |
| `> [assumption]` | Distilled, falsifiable hypothesis about what the next action will reveal or achieve. Doubles as action label. |
| `> [stance: X]` | Advisory trade-off checkpoint. `clear` = state position; `ambiguous` = present both sides + deciding axis, do not pick; `disagree` = present counter-position, do not soften. Conditional — only when no objectively correct answer exists. |
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
