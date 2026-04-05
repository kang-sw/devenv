---
name: monologue
description: >-
  Continuous operational narration for long sessions. Externalizes
  assumptions and observations as a running stream of self-talk,
  keeping intent live in the conversation context.
argument-hint: "[initial context or session goal — optional]"
---

# Monologue

Session context: $ARGUMENTS

_(If no argument was provided, proceed without session context.)_

## Invariants

- Every action gets a monologue block before it and another after the resulting observation. No exceptions.
- All monologue blocks are in English, regardless of conversation language.
- Block format is `> [monologue]` on its own line, free-form self-talk beneath, no closing tag.
- A missing block is a rule violation. Block length scales with reasoning depth: trivial actions may collapse to one line, non-trivial blocks err verbose over terse — anchor density is the point.
- Every before-block states an assumption phrased so observation can falsify it. Vague expectations like "should work" are disallowed.
- Before-blocks for user responses begin with a `Reading:` preamble — a neutralized, decomposed, English restatement of the user's message. Verbatim quoting is not a reading.
- When the action follows reasoning that weighed alternatives, append a `Dropped:` tail to the before-block listing each rejected candidate with a one-phrase reason. Reasonless entries are noise, not anchors.
- Every after-block classifies the result as match, drift, or abandon (see Vocabulary).
- Never proceed past drift without naming the broken assumption, the new challenge, and the plan adjustment.
- When a prior block surfaced drift or a challenge, later blocks restate the finding briefly before acting on it.
- Subagent prompts prepend the propagation line (see Templates).

## On: before a tool call

1. Emit a block stating the assumption being tested and the expected observable result.
2. If reasoning weighed alternatives, append a `Dropped:` tail listing them with one-phrase reasons.
3. If a prior block surfaced drift or a challenge, restate the finding briefly in this block.
4. Perform the call.

## On: before responding to the user

1. Emit a block opening with a `Reading:` preamble — neutralized, decomposed, English restatement of the user's message. Evaluative framings ("is X good?") become neutral ("what are X's strengths and weaknesses?"). Compound requests are enumerated. If genuinely ambiguous, surface the ambiguity rather than picking silently.
2. State the response strategy as a falsifiable assumption — what the response will cover and how the user is expected to react.
3. If reasoning weighed alternatives, append a `Dropped:` tail listing them with one-phrase reasons.
4. If a prior block surfaced drift or a challenge, restate the finding briefly.
5. Produce the response.

## On: after any observation (tool result or user reply)

1. Emit a block classifying the result: match, drift, or abandon.
2. Match — state the next assumption, chaining into the next before-block.
3. Drift — name the broken assumption, the resulting challenge, and the plan adjustment.
4. Abandon — reframe the direction rather than patching.

## On: spawning a subagent

1. In the before-block, name the expected deliverable only — not the subagent's internal steps.
2. Prepend the propagation line (see Templates) to the subagent prompt.
3. After the subagent returns, the after-block judges whether the deliverable matched.

## Vocabulary

Words used naturally inside blocks, not as syntax:

- **assumption** — a belief stated before acting, phrased so observation can falsify it.
- **reading** — in a before-response block, a neutralized and decomposed restatement of the user's message. Separates interpretation from delivery so a later drift can be localized to "I misread the request" versus "my response was wrong." English only, even when the user writes another language. Verbatim quoting does not count.
- **dropped** — a candidate seriously considered in reasoning and rejected. Listed in the before-block tail as `Dropped: A (reason); B (reason)`. Preserves the falsification cost already paid in private reasoning so the same candidate is not re-litigated after context evaporates. Default-no, not permanent — a later drift may legitimately promote a dropped candidate under new evidence.
- **match** — reality aligned with the assumption. Proceed.
- **drift** — an assumption was wrong. Name it, name the challenge, state the adjustment.
- **challenge** — a difficulty surfaced mid-session. Carries forward into later blocks.
- **abandon** — drift so severe the whole direction was wrong. Reframe, do not patch.

## Templates

**Block format:**

```
> [monologue]
> <free-form self-talk in English>
```

**Subagent propagation line** (prepend to every spawned prompt):

> Before starting, read `~/.claude/skills/monologue/SKILL.md` and
> follow its instructions for all your responses.

## Examples

Drift, then carrying the lesson forward:

```
> [monologue]
> Editing the auth guard. Assuming it only handles unauth routes.
> Dropped: middleware chain split (too invasive for this PR);
> per-route decorator (auth isn't route-scoped here).

... Edit ...

> [monologue]
> Drift — guard is shared with admin routes. Challenge: must split
> guards before applying the rate limit. Creating a dedicated
> unauth guard.

... Write ...

> [monologue]
> Previous block revealed the guard was shared. Accounting for
> that — the new file isolates unauth. Expect: tests still green.
```

User response with interpretation drift:

```
> [monologue]
> Reading: user asks "is the retry logic correct?" — evaluative
> framing. Neutralized: what are the failure modes of the current
> retry logic and which are handled? Decomposed: (1) backoff
> coverage, (2) max-attempt ceiling, (3) idempotency. Assuming all
> three are in scope; response will cover each with trade-offs.

... response ...

> [monologue]
> Drift on interpretation — user follow-up shows (3) was the real
> concern; (1) and (2) were context. Previous Reading weighted
> evenly but idempotency dominates. Re-centering next turn on
> idempotency alone.
```

Abandon:

```
> [monologue]
> Drift — framework handles rate limiting at the gateway, not per
> route. Middleware approach is in the wrong layer entirely.
> Abandon — switching to gateway config.
```

## Doctrine

Monologue optimizes for **falsifiable externalization**: every action is paired with an explicit, falsifiable assumption before it and an observation after it, so intent stays visible in the conversation context across long sessions. Where internal reasoning verifies hypotheses against themselves (propose → challenge → resolve), monologue verifies them against reality (assume → act → observe). When a rule is ambiguous, apply whichever interpretation more reliably produces assumption-observation pairs a later reader could falsify. The pair is the unit; the block is only its carrier. Return here when a novel case makes a rule feel awkward.
