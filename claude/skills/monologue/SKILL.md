---
name: monologue
description: >-
  Continuous operational narration for long sessions with native thinking
  active. Shapes the internal thinking pipeline and projects its verdicts
  as a tight stream of falsifiable blocks, keeping intent live in the
  conversation context after the thinking channel evaporates.
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
- The thinking pipeline cycles challenge → resolve at least twice
  before settling on a position.
- `> [stance]` carries a real position, not a one-word stub.
- `> [reading]` enumerates every evaluative claim with its neutralized
  question and the `fails if:` condition checked against evidence.

## Invariants

- Native thinking is assumed active and carries derivation; monologue blocks carry only its verdicts.
- No derivation language in blocks ("maybe", "let me think", "on the other hand") — leak, rewrite as verdict.
- Block format: `> [<type>]` on its own line, `>`-prefixed continuation, no closing tag.
- **Language: every `> [<type>]` block and its `>`-prefixed continuation lines must be written in English, regardless of user language.**
- Required blocks are mandatory as stubs; omission is forbidden.
- `> [assumption]` before every action — no exceptions.
- `> [observe]` after every tool result or subagent return.
- `> [reading]` at every user message.
- `> [stance]` at every user message and at every trade-off the thinking surfaces.
- `> [dropped]` at every decision point — `none` if no alternatives were considered.
- Never proceed past drift without naming the broken assumption, the challenge, and the adjustment.
- When a prior block surfaced drift or a challenge, restate it briefly before acting.
- Subagent prompts prepend the propagation line (see Templates).

## Thinking Pipeline

Monologue assumes an active internal thinking channel. Use it for
derivation; reserve monologue output for verdicts. The stages below
describe **what runs inside the thinking channel** — do not render them
as monologue blocks. Their conclusions land in `[reading]`, `[stance]`,
`[assumption]`, `[dropped]`, and `[observe]`.

**On a user message, inside thinking:**

1. Decompose the message into numbered claims; separate pass-through from evaluative.
2. For each evaluative claim, state its direct opposite.
3. Restate as a neutral question; enumerate `assumes: … → fails if: …` pairs.
4. Evaluate each `fails if:` against available evidence before any other reasoning.
5. Propose → Challenge → Resolve → Decide. A challenge is resolved only when a specific falsifiable condition under which the challenge does not apply is named.
6. An unresolved challenge after two challenge-resolve cycles mandates `[stance: ambiguous]` in output.
7. When the user asserts a claim, find at least one condition under which it would be wrong before resolving.

**On a tool call, inside thinking:**

1. Why this tool? What observable will it return? What assumption does that observable test?
2. What alternatives were considered and rejected, on what basis?

**On a tool result, inside thinking:**

1. Parse the result against the pre-call assumption.
2. If drift: local (patch with new assumption) or directional (abandon and reframe)?

## On: user message

1. `> [reading]` — numbered decomposition; `fails if:` / `found:` per evaluative claim.
2. `> [stance: X]` — position, or both sides + resolving observable, or counter-position.
3. `> [assumption]` — response intent, falsifiable.
4. `> [dropped]` — rejected alternatives, or `none`.
5. Respond.

## On: before a tool call

1. `> [assumption]` — what the call will reveal, falsifiable.
2. `> [dropped]` — rejected alternatives, or `none`.
3. If a prior block surfaced drift or a challenge, restate it briefly.
4. Call.

## On: after a tool result

1. `> [observe]` — classify `match` / `drift` / `abandon` in the first line.
2. **Match** — state the next assumption.
3. **Drift** — name the broken assumption, the challenge, the adjustment; follow with `> [assumption]` for the next action.
4. **Abandon** — reframe the direction rather than patching.

## On: spawning a subagent

1. `> [assumption]` names the expected deliverable, not the subagent's internal steps.
2. Prepend the propagation line (see Templates) to the subagent prompt.
3. After return, `> [observe]` judges whether the deliverable matched.

## Reference

### Block Types

| Block | Role |
|---|---|
| `> [reading]` | Verdict of the thinking-channel reading+neutralize pipeline on a user message. Numbered claims; each evaluative claim carries its neutralized question, the critical `fails if:` condition, and the `found:` verdict from checking it. Pass-through claims marked `— pass`. |
| `> [assumption]` | Falsifiable hypothesis about what the next action will reveal or achieve. Doubles as action label. |
| `> [stance: X]` | Trade-off checkpoint verdict. `clear` = state position; `ambiguous` = both sides plus the observable that would resolve them — do not pick; `disagree` = counter-position, unsoftened. |
| `> [dropped]` | Candidates the thinking rejected, each with a one-phrase reason. `none` if genuinely none. |
| `> [observe]` | Intake verdict on a tool or subagent result, using `match` / `drift` / `abandon`. |

### Vocabulary

- **verdict** — a conclusion from the thinking channel, short enough to survive context compaction, falsifiable enough for a later reader to challenge it. The unit of monologue output.
- **leak** — derivation prose that escaped from the thinking channel into monologue output. Identified by hedging and process language ("maybe", "let me think", "on the other hand"). Rewrite as verdict.
- **match** — reality aligned with the assumption. Proceed.
- **drift** — an assumption was wrong. Name it, name the challenge, state the adjustment.
- **challenge** — a difficulty surfaced during thinking or observation. Carries forward.
- **abandon** — drift so severe the whole direction was wrong. Reframe, do not patch.

## Templates

All templates use angle-bracket slots. No concrete domain content.

**Block format.**

```
> [<type>]
> <verdict in English>
```

No closing tag. Multiple blocks per turn are expected.

**User-message sequence.**

```
> [reading]
> (1) <pass-through claim> — pass
> (2) <evaluative claim>
>     question: <neutralized form>
>     fails if: <condition checked>  →  found: <what evidence showed>
> (3) <request> — pass

> [stance: clear|ambiguous|disagree]
> <position stated / both sides + resolving observable / counter-position>

> [assumption]
> Response covers <X>; user will react with <Y>.

> [dropped]
> <rejected framing> (<reason>); <rejected approach> (<reason>).
> # or: none
```

**Before tool call.**

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
the thinking evaporates before the next turn. The thinking channel
carries derivation — decomposition, neutralization, challenge, resolve
— and monologue carries its conclusions as durable falsifiable pairs:
assumption before action, observation after. When a rule is ambiguous,
apply whichever interpretation more reliably produces
assumption-observation pairs a later reader could falsify. If a block
reads like a thought process rather than a verdict, it leaked thinking
into the output — rewrite it. The pair is the unit; the block is only
its carrier.
