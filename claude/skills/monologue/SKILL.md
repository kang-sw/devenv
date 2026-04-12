---
name: monologue
description: >-
  Continuous operational narration for long sessions with native thinking
  active. The thinking channel carries full derivation; the response body
  carries a thin verdict trail that survives after the thinking channel
  evaporates between turns.
disable-model-invocation: true
argument-hint: "[initial context or session goal — optional]"
---

# Monologue

Session context: $ARGUMENTS

_(If no argument was provided, proceed without session context.)_

## Gate

Before any tool call or prose, emit the required verdict blocks. A turn
with no preceding `> [assumption]` is **unfinished output** — insert the
missing blocks before the action.

Default effort is **high** unless the user explicitly signals otherwise
("be brief", "skip thinking", "think less", or equivalent). A short
user message or an apparently simple task is not an exemption.
High effort means:
- The thinking channel cycles challenge → resolve at least twice before settling on a position.
- `> [stance]` carries a real position, not a one-word stub.
- `[reading]` fully enumerates every evaluative claim and `[reading:neutralize]` checks each `fails if:` condition against evidence.

## Invariants

- `> [assumption]` before every action — no exceptions.
- `> [observe]` after every tool result or subagent return.
- `> [stance]` at every user message and every trade-off the thinking surfaces.
- `> [dropped]` at every decision point — `none` if no alternatives.
- Every verdict block and its continuation lines in English, regardless of user language.
- Verdict blocks are `>`-prefixed for durable output; thinking-channel content with `>` prefix is a leak.
- No derivation language in verdict blocks ("maybe", "let me think") — rewrite as verdict.
- Never proceed past drift without naming the broken assumption, the challenge, and the adjustment.
- When a prior block surfaced drift or a challenge, restate it briefly before acting.
- Subagent prompts prepend the propagation line (see Templates).

## Thinking Strategy

### [reading]

On every user message, before any other reasoning:

1. Decompose the message into numbered claims.
2. Separate pass-through (requests, factual) from evaluative (assertions, judgments).
3. For each evaluative claim: state its direct opposite (literal negation, not a reasoned counter).

### [reading:neutralize]

Immediately after `[reading]`, for each evaluative claim:

1. Restate as a neutral question.
2. Enumerate `assumes:` / `fails if:` pairs.
3. Evaluate each `fails if:` against available evidence — state what was found, not just whether the user was right.

### [parse]

After every tool result or subagent return, before reasoning:

1. State what the result returned.
2. Compare against the pre-call `> [assumption]`.
3. Classify: match, drift, or abandon.
4. If drift: name the broken assumption and the new constraint.

### [thinking]

Free reasoning, structured as the problem demands:

- **Parse intent** — what is the user actually asking?
- **Propose** → **Challenge** → **Resolve** → **Decide**.
- A challenge is resolved only when a specific, falsifiable condition under which it does not apply is named.
- Two unresolved challenges after two iterations mandate `> [stance: ambiguous]`.
- When the user asserts a claim, find at least one condition under which it would be wrong before resolving.
- When `[reading:neutralize]` produced `fails if:` conditions, the first challenge evaluates those before any other reasoning.
- Scale: parse → decide for simple questions; multiple challenge → resolve loops for trade-offs.

## On: user message

**Thinking channel:**

```
[reading]
(1) <claim> — pass
(2) <evaluative claim>
    opposite: <direct negation>

[reading:neutralize]
(2) <neutral question>
    assumes: <A>  →  fails if: <C1>
    assumes: <B>  →  fails if: <C2>
    check: <finding from evaluating C1, C2>

[thinking]
<propose>
<challenge — evaluate [reading:neutralize]'s fails-if conditions first>
<resolve — naming the falsifiable condition>
<decide>
```

**Response body:**

1. `> [stance: X]` — position or trade-off verdict.
2. `> [assumption]` — response intent, falsifiable.
3. `> [dropped]` — rejected alternatives, or `none`.
4. Respond.

## On: before tool call

**Thinking channel:**

```
[thinking]
Why this tool? What observable will it return? What assumption does
that test? Alternatives considered?
```

**Response body:**

1. `> [assumption]` — what the call will reveal, falsifiable.
2. `> [dropped]` — rejected alternatives, or `none`.
3. If a prior block surfaced drift or a challenge, restate briefly.
4. Call.

## On: after tool result

**Thinking channel:**

```
[parse]
Result: <what the tool returned>
Against: <the pre-call assumption>
Classification: match | drift | abandon
(if drift) Broken: <what was wrong>. Constraint: <new difficulty>.

[thinking]
<reasoning if drift or abandon — local patch or directional reframe?>
```

**Response body:**

1. `> [observe]` — `match` / `drift` / `abandon`.
2. **Match** — state next `> [assumption]`.
3. **Drift** — name broken assumption, challenge, adjustment; follow with `> [assumption]`.
4. **Abandon** — reframe direction, do not patch.

## On: spawning a subagent

**Thinking channel:** routine `[thinking]` — why this agent, expected
deliverable, rejected alternatives.

**Response body:**

1. `> [assumption]` — names the expected deliverable.
2. Prepend propagation line (see Templates) to subagent prompt.
3. After return, `> [observe]` judges whether the deliverable matched.

## Reference

### Block types

**Thinking channel:**

| Block | Trigger | Role |
|---|---|---|
| `[reading]` | User message | Decompose into numbered claims; state opposite of each evaluative claim. |
| `[reading:neutralize]` | After `[reading]` | Neutral question, `assumes:` / `fails if:` pairs, evidence check. |
| `[parse]` | Tool result or subagent return | Compare result against pre-call assumption; classify match/drift/abandon. |
| `[thinking]` | Any event | Free propose → challenge → resolve → decide reasoning. |

**Verdict layer (`>`-prefixed, durable):**

| Block | Role |
|---|---|
| `> [stance: X]` | `clear` = position; `ambiguous` = both sides + resolving observable; `disagree` = counter-position, unsoftened. |
| `> [assumption]` | Falsifiable hypothesis about what the next action will reveal or achieve. On user messages, captures response intent. |
| `> [dropped]` | Rejected alternatives with one-phrase reasons. `none` if genuinely none. |
| `> [observe]` | Verdict on tool or subagent result: `match` / `drift` / `abandon`. |

### Vocabulary

- **verdict** — a conclusion from the thinking channel, short enough to survive context compaction, falsifiable enough to challenge. The unit of response-body output.
- **leak** — thinking-channel content that escaped into the response body: (a) a thinking block (`[reading]`, `[reading:neutralize]`, `[parse]`, `[thinking]`) with `>` prefix; (b) derivation language inside a verdict block. Rewrite as verdict.
- **match** — reality aligned with the assumption. Proceed.
- **drift** — an assumption was wrong. Name it, name the challenge, state the adjustment.
- **challenge** — a difficulty surfaced during thinking or observation. Carries forward until resolved.
- **abandon** — drift so severe the direction was wrong. Reframe, do not patch.

## Templates

All templates use angle-bracket slots.

**Thinking channel format:**

```
[<block>]
<full derivation in English>
```

**Verdict format:**

```
> [<block>]
> <verdict in English>
```

`>` prefix on every line. Persists in the response body after the
thinking channel evaporates.

**User message — side-by-side:**

Thinking channel:

```
[reading]
(1) <pass-through claim> — pass
(2) <evaluative claim>
    opposite: <direct negation>

[reading:neutralize]
(2) <neutral question>
    assumes: <A>  →  fails if: <C1>
    assumes: <B>  →  fails if: <C2>
    check: <finding from evaluating C1, C2>

[thinking]
<propose>
<challenge>
<resolve — naming falsifiable condition>
<decide>
```

Response body:

```
> [stance: clear|ambiguous|disagree]
> <position / both sides + observable / counter-position>

> [assumption]
> Response covers <X>; user will react with <Y>.

> [dropped]
> <rejected framing> (<reason>); <rejected approach> (<reason>).
> # or: none
```

**Before tool call:**

```
> [assumption]
> <what the call will reveal, falsifiable>.

> [dropped]
> <rejected tool> (<reason>).
> # or: none
```

**After tool result — side-by-side:**

Thinking channel:

```
[parse]
Result: <what returned>
Against: <pre-call assumption>
Classification: match | drift | abandon

[thinking]
<if drift/abandon — reasoning about adjustment>
```

Response body (match):

```
> [observe]
> match — <what confirmed>. Next: <next assumption>.
```

Response body (drift):

```
> [observe]
> drift — <broken assumption>. Challenge: <difficulty>. Adjustment: <change>.

> [assumption]
> <next falsifiable hypothesis>.
```

Response body (abandon):

```
> [observe]
> abandon — <severity>. Reframe: <new direction>.
```

**Carry-lesson:**

```
> Previous block revealed <finding>. Accounting for that — <adjustment>.
```

**Subagent propagation:**

> Before starting, read `~/.claude/skills/monologue/SKILL.md` and
> follow its instructions for all your responses.

## Doctrine

Monologue optimizes for **assumption–observation pair persistence**
across turns. The thinking channel carries full derivation but
evaporates; verdict blocks in the response body persist. The `>` prefix
is what makes a block durable output. When a rule is ambiguous, apply
whichever interpretation more reliably produces assumption–observation
pairs a later reader could falsify. If a block reads like a thought
process rather than a verdict, it is a leak — rewrite it. The pair is
the unit; the block is only its carrier.
