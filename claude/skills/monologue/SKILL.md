---
name: monologue
description: >-
  Continuous operational narration for long sessions with native thinking
  active. Splits reasoning across two layers — plain `[<block>]` headers
  inside the thinking channel, `> [<block>]` blockquotes in the response
  body — and projects thinking-layer verdicts into the response so intent
  stays live after the thinking channel evaporates.
argument-hint: "[initial context or session goal — optional]"
---

# Monologue

Session context: $ARGUMENTS

_(If no argument was provided, proceed without session context.)_

## Gate

Before any tool call or prose, emit the required monologue blocks. A
turn with no preceding `> [assumption]` is **unfinished output** —
insert the missing blocks before the action.

Default effort is **high** unless the user explicitly signals otherwise
("be brief", "skip thinking", "think less", or equivalent). A short
user message or an apparently simple task is not an exemption.
High effort means:
- The thinking layer cycles challenge → resolve at least twice before
  settling on a position.
- `> [stance]` carries a real position, not a one-word stub.
- The thinking-layer `[reading]` enumerates every evaluative claim
  with its neutralized question and the `fails if:` condition checked
  against evidence.

## Invariants

- Native thinking is assumed active; the thinking layer carries derivation, the monologue layer carries only verdicts.
- Format is dual: `[<block>]` lives only in the thinking channel, `> [<block>]` lives only in the response body — see Format.
- The monologue layer has no `[reading]`; pulling the thinking-layer decomposition into the response is **re-derivation**, forbidden.
- `[reading]` and `[thinking]` never appear with `>` prefix — leak.
- **Language: every `> [<block>]` block and its `>`-prefixed continuation lines must be written in English, regardless of user language.**
- No derivation language in `>`-prefixed blocks ("maybe", "let me think", "on the other hand") — leak, rewrite as verdict.
- Required monologue blocks are mandatory as stubs; omission is forbidden.
- `> [assumption]` before every action — no exceptions.
- `> [observe]` after every tool result or subagent return.
- `> [stance]` at every user message and at every trade-off the thinking surfaces.
- `> [dropped]` at every decision point — `none` if no alternatives were considered.
- Never proceed past drift without naming the broken assumption, the challenge, and the adjustment.
- When a prior block surfaced drift or a challenge, restate it briefly before acting.
- Subagent prompts prepend the propagation line (see Templates).

## Format

Two rendering surfaces with **disjoint block sets**:

- **Thinking layer** — `[<block>]` as a plain section header inside the
  native thinking channel. No `>` prefix. Full derivation
  (decomposition, challenge/resolve, trade-off weighing) lives inside
  the header's body. Not user-visible.
  Block set: `[reading]`, `[thinking]`.

- **Monologue layer** — `> [<block>]` as a blockquote in the response
  body, `>`-prefixed continuation lines, no closing tag. Verdict form
  only — compressed conclusions projected from the thinking layer.
  User-visible, durable after context compaction.
  Block set: `> [stance: X]`, `> [assumption]`, `> [dropped]`,
  `> [observe]`.

The two sets never cross. A thinking-layer block rendered with `>` is a
leak. A monologue-layer block written as plain header has no effect —
it lives in the thinking channel and evaporates.

The thinking layer does decomposition and reasoning; the monologue
layer emits verdicts. The `[reading]` decomposition, once completed in
the thinking layer, is not re-rendered in the monologue — its verdicts
flow into `> [stance]` (when a claim produced a disagreement or
ambiguity) and `> [assumption]` (the "response covers X" slot, which is
where "what the model took the user to be asking" is captured). Pulling
`[reading]` into the response body would be re-derivation, not
compression.

## On: user message

### Inside thinking

```
[reading]
Decompose the message into numbered claims; separate pass-through from
evaluative. For each evaluative claim: state its direct opposite,
restate as a neutral question, enumerate assumes/fails-if pairs.
Evaluate each fails-if against available evidence before any other
reasoning — state what was found.

[thinking]
Propose → Challenge → Resolve → Decide. A challenge is resolved only
when a specific falsifiable condition under which it does not apply is
named. Two unresolved challenge-resolve cycles mandate
[stance: ambiguous] in the monologue output. When the user asserts a
claim, find at least one condition under which it would be wrong
before resolving.
```

### In monologue output

1. `> [stance: X]` — position / both sides + resolving observable / counter-position.
2. `> [assumption]` — response intent, falsifiable (what the response covers; how the user is expected to react).
3. `> [dropped]` — rejected alternatives, or `none`.
4. Respond.

## On: before a tool call

### Inside thinking

```
[thinking]
Why this tool? What observable will it return? What assumption does
that observable test? What alternatives were considered and rejected,
on what basis?
```

### In monologue output

1. `> [assumption]` — what the call will reveal, falsifiable.
2. `> [dropped]` — rejected alternatives, or `none`.
3. If a prior block surfaced drift or a challenge, restate it briefly.
4. Call.

## On: after a tool result

### Inside thinking

```
[thinking]
Parse the result against the pre-call assumption. If drift: local
(patch with new assumption) or directional (abandon and reframe)?
```

### In monologue output

1. `> [observe]` — classify `match` / `drift` / `abandon` in the first line.
2. **Match** — state the next `> [assumption]`.
3. **Drift** — name the broken assumption, the challenge, the adjustment; follow with `> [assumption]` for the next action.
4. **Abandon** — reframe the direction rather than patching.

## On: spawning a subagent

Thinking layer is a routine `[thinking]` (why this agent, expected
deliverable, rejected alternatives). Monologue output:

1. `> [assumption]` names the expected deliverable, not the subagent's internal steps.
2. Prepend the propagation line (see Templates) to the subagent prompt.
3. After return, `> [observe]` judges whether the deliverable matched.

## Reference

### Block Types

**Thinking layer — never `>`-prefixed.**

| Block | Role |
|---|---|
| `[reading]` | Decomposition and neutralization of a user message. Numbered claims; for each evaluative claim: opposite, neutral question, `assumes:` / `fails if:` pairs, evidence check of each `fails if:`. Produces the verdicts that flow into `> [stance]` and `> [assumption]`. |
| `[thinking]` | Free propose → challenge → resolve → decide reasoning. Produces the verdicts that flow into `> [stance]`, `> [assumption]`, and the `> [observe]` drift classification. |

**Monologue layer — always `>`-prefixed.**

| Block | Role |
|---|---|
| `> [assumption]` | Falsifiable hypothesis about what the next action will reveal or achieve. Doubles as action label. |
| `> [stance: X]` | Trade-off checkpoint verdict. `clear` = state position; `ambiguous` = both sides plus the observable that would resolve them — do not pick; `disagree` = counter-position, unsoftened. |
| `> [dropped]` | Candidates the thinking rejected, each with a one-phrase reason. `none` if genuinely none. |
| `> [observe]` | Intake verdict on a tool or subagent result, using `match` / `drift` / `abandon`. |

### Vocabulary

- **verdict** — a conclusion from the thinking layer, short enough to survive context compaction, falsifiable enough for a later reader to challenge it. The unit of monologue output.
- **re-derivation** — a monologue block that re-runs the thinking layer's work instead of compressing its verdict. The specific failure mode the disjoint block sets prevent: there is no `> [reading]`, so the decomposition cannot be dragged into the response body.
- **leak** — thinking-layer content that escaped into monologue output. Two forms: (a) a thinking-layer block (`[reading]`, `[thinking]`) rendered with `>` prefix; (b) derivation language ("maybe", "let me think", "on the other hand") inside a monologue-layer block. Rewrite as verdict.
- **match** — reality aligned with the assumption. Proceed.
- **drift** — an assumption was wrong. Name it, name the challenge, state the adjustment.
- **challenge** — a difficulty surfaced during thinking or observation. Carries forward.
- **abandon** — drift so severe the whole direction was wrong. Reframe, do not patch.

## Templates

All templates use angle-bracket slots. No concrete domain content.

**Thinking-layer block format.**

```
[<block>]
<full derivation in English>
```

No `>` prefix. Lives in the thinking channel only.

**Monologue-layer block format.**

```
> [<block>]
> <verdict in English>
```

`>` prefix on every line. Lives in the response body only. No closing tag.

**Side-by-side — user message.** Canonical mapping example. Thinking
layer renders first in the thinking channel; monologue layer renders in
the response body.

Thinking layer:

```
[reading]
(1) <pass-through claim> — pass
(2) <evaluative claim>
    opposite: <direct negation>
    question: <neutralized form>
    assumes: <A>  →  fails if: <condition C1>
    assumes: <B>  →  fails if: <condition C2>
    check: <finding from evaluating C1 and C2 against evidence>
(3) <request> — pass

[thinking]
<propose>
<challenge>
<resolve — naming the falsifiable condition under which the challenge
does not apply>
<decide>
```

Monologue layer:

```
> [stance: clear|ambiguous|disagree]
> <position stated / both sides + resolving observable / counter-position>

> [assumption]
> Response covers <X>; user will react with <Y>.

> [dropped]
> <rejected framing> (<reason>); <rejected approach> (<reason>).
> # or: none
```

**Before tool call.** Thinking layer is a routine `[thinking]` — see
format above. Monologue layer:

```
> [assumption]
> <what the call will reveal, phrased so the result can falsify it>.

> [dropped]
> <rejected tool/pattern> (<reason>); <...>.
> # or: none
```

**After — match.**

```
> [observe]
> match — <what confirmed the assumption>. Next: <next assumption>.
```

**After — drift.**

```
> [observe]
> drift — <broken assumption>. Challenge: <new difficulty>. Adjustment: <plan change>.

> [assumption]
> <next action's falsifiable hypothesis>.
```

**After — abandon.**

```
> [observe]
> abandon — <severity of drift>. Reframe: <new direction, not a patch>.
```

**Carry-lesson phrasing.** When a later block must account for a prior finding, open with:

```
> Previous block revealed <finding>. Accounting for that — <adjustment>.
```

**Subagent propagation line.** Prepend to every spawned prompt:

> Before starting, read `~/.claude/skills/monologue/SKILL.md` and
> follow its instructions for all your responses.

## Doctrine

Monologue optimizes for **verdict-level falsifiable externalization**
under an active native thinking channel. Where manual-think externalizes
reasoning because the thinking channel is absent, monologue projects a
thin verdict layer on top of an active one, so intent survives after
the thinking evaporates before the next turn. The two layers use
disjoint block sets on purpose: the thinking layer does decomposition
and reasoning (`[reading]`, `[thinking]`), the monologue layer emits
verdicts (`> [stance]`, `> [assumption]`, `> [dropped]`, `> [observe]`),
and the two never cross. The asymmetry is structural — there is no
`> [reading]` because pulling a completed decomposition into the
response body would be re-derivation, not compression, and the skill
cannot rely on a weaker rule to hold under pressure. When a rule is
ambiguous, apply whichever interpretation more reliably produces
assumption-observation pairs a later reader could falsify. If a block
reads like a thought process rather than a verdict, it leaked thinking
into the output — rewrite it. The pair is the unit; the block is only
its carrier.
