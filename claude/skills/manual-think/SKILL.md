---
name: manual-think
description: >-
  Activate manual chain-of-thought when native extended thinking is
  unavailable. Load at session start during server outages.
disable-model-invocation: true
---

# Manual Think

## Gate

Before writing any prose or calling any tool, open a `>` block. Do not
let a tool call appear in output without a preceding `> [assumption]` —
a turn with a tool call and no prior `> [assumption]` is **unfinished
output**; insert the missing block before the call.

Default effort is **high** unless the user explicitly signals otherwise
("be brief", "skip thinking", "think less", or equivalent). A short
user message or an apparently simple task is not an exemption.
High effort means:
- `> [thinking]` cycles through challenge → resolve at least twice before
  settling on a position.
- `> [stance]` is not omitted even when the answer feels obvious.
- `> [reading:neutralize]` fully enumerates `assumes:` / `→ fails if:`
  pairs; no claim is waved through as trivially true.

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

1. `> [reading]` — decompose the message into numbered claims. For each evaluative claim: `- opposite:` is its direct negation (not a reasoned counter). No evaluative framing → "pass".
2. `> [reading:neutralize]` — for each evaluative claim: restate as neutral question, then `- assumes:` / `→ fails if:` pairs. No synthesis line. No evaluative framing → "pass".
3. `> [thinking]` — free reasoning, challenge/resolve as needed.
4. `> [stance: X]` — when thinking produced a trade-off or an unresolved challenge.
5. `> [assumption]` — what the response covers, expected reaction.
6. `> [dropped]` — if alternatives were weighed.
7. Respond.

```
> [reading]
> (1) User requests X — pass
> (2) User says Y is problematic
>     - opposite: Y is not problematic
> (3) User wants Z done — pass

> [reading:neutralize]
> (2) Is Y actually problematic?
>     - assumes: Y is directly causing the problem  →  fails if: removing Y doesn't fix — misidentified cause
>     - assumes: "problematic" means blocking  →  fails if: Y is minor friction — disproportionate response

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
- **Resolution standard**: a challenge is resolved only when the response names a specific, falsifiable condition under which the challenge does not apply. "This is probably fine" without naming that condition = unresolved. An unresolved challenge after two iterations mandates `[stance: ambiguous]` — do not proceed past it.
- When the user asserts a claim, Challenge is mandatory — find at least one condition under which it would be wrong before resolving.
- When `[reading:neutralize]` produced `fails if:` conditions, the first Challenge must evaluate those conditions against available evidence before any other reasoning. State what was found, not just whether the user was right.
- Scale: Parse → Decide for simple questions; multiple Challenge → Resolve loops for trade-offs.

## Reference

### Block Types

| Block | Role |
|---|---|
| `> [reading]` | Literal decomposition of user's message into numbered claims. For each evaluative claim: `- opposite:` is its direct negation, not a reasoned counter. No evaluative framing → "pass". |
| `> [reading:neutralize]` | Analytical decomposition of each evaluative claim. Restate as neutral question, then `- assumes:` / `→ fails if:` pairs. No synthesis line. Mandatory after `[reading]`. |
| `> [thinking]` | Free-form reasoning chain. Parse, challenge, resolve, decide — whatever the problem demands. |
| `> [assumption]` | Distilled, falsifiable hypothesis about what the next action will reveal or achieve. Doubles as action label. |
| `> [stance: X]` | Advisory trade-off checkpoint. `clear` = state position; `ambiguous` = present both sides, then state what specific observable evidence would resolve the ambiguity — do not pick; `disagree` = present counter-position, do not soften. **Mandatory when any challenge in `[thinking]` remains unresolved after two iterations.** Not a last resort — fire proactively. |
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
