---
name: monologue
description: >-
  Structured operational narration for long sessions. Produces a
  distilled decision record at every decision point — responses,
  plans, and tool-call sequences alike — so intent and expectations
  stay live in the conversation context.
---

# Monologue

## Purpose

In long sessions, the *why* behind each decision evaporates. This
skill externalizes a compact decision record at every decision point,
keeping intent, ruled-out alternatives, and expected outcomes visible
as durable context anchors.

This is **not** a reasoning layer — that is `manual-think`'s domain.
This is the **residue of reasoning**: the final decisions and
expectations that the next step needs to reference.

```
[manual-think]  exploratory scratchpad   →  hidden deliberation
      ↓  distilled
[monologue]     structured decision log  →  stays in context
```

## Core Directive

**Wrap every decision point in `> [beat]` / `> [/beat]` markers.**

A **beat** is a narrative unit: a moment where you commit to a course
of action and can later verify whether it landed.

### Triggers — open a beat before:

- Responding to a non-trivial user message
- A tool-call sequence
- Formulating or revising a plan
- Any direction change mid-task

### Before the beat

```
> [beat: <label>]
>     intent: <what you are committing to and why — one line>
>     risks: <considered risks and why each was dismissed>
>     expect: <what success looks like>
```

- `<label>` is a short verb phrase: `answer architecture question`,
  `read config`, `edit handler`, `revise plan`, `spawn subagent`.
- `risks` may be omitted only when there are genuinely none worth
  recording. When in doubt, include it.

### After the beat

```
> [/beat: match | drift | open]
>     observed: <one-line summary of what came back — omit if open>
>     next: <plan continues as-is, or adjustment and reason>
```

- `match` — result aligned with `expect`.
- `drift` — result deviated. `next` **must** describe the plan
  adjustment; proceeding without acknowledging drift is not allowed.
- `open` — no observable result yet (e.g. response written, awaiting
  user reply). `observed` may be omitted; `next` states what to
  watch for.

## Rules

1. **No beat without a pre-block.** The `> [beat]` block must appear
   before the action, never after.
2. **No skipping post-blocks.** Close every beat with `> [/beat]`.
   Do not batch multiple closes together.
3. **Concise over complete.** Each field is one line. Write decisions,
   not prose.
4. **Language.** Monologue blocks must be in English regardless of
   conversation language.
5. **Nesting.** If a beat spawns a subagent, the subagent runs its
   own monologue independently. The parent's `expect` names the
   subagent's deliverable, not its internal steps.

## Examples

**Tool call — match:**
```
> [beat: read auth middleware]
>     intent: locate registration point before editing guard logic
>     risks: file may have moved — grep first if read returns 404
>     expect: find app.use() calls, middleware order visible

... Read tool call ...

> [/beat: match]
>     observed: middleware registered at line 52, order confirmed
>     next: proceed to step 2 — edit rate-limit guard at line 78
```

**Tool call — drift:**
```
> [beat: edit rate-limit guard]
>     intent: cap unauthenticated requests to 10/min per spec
>     risks: touching shared middleware may break auth tests
>     expect: clean edit, no adjacent logic disturbed

... Edit tool call ...

> [/beat: drift]
>     observed: guard is shared with admin routes — limit would apply to admins too
>     next: split into two guards before editing; revise plan
```

**Response — open:**
```
> [beat: answer architecture question]
>     intent: explain trade-offs of option A vs B to unblock decision
>     risks: user may be asking about C, not A/B — address ambiguity
>     expect: user can make a decision or asks a clarifying follow-up

... response text ...

> [/beat: open]
>     next: if user picks A, proceed to write-plan; if unclear, re-clarify
```

## Subagent Propagation

When spawning subagents via the Agent tool, prepend the following to
every subagent prompt:

> Before starting, read `~/.claude/skills/monologue/SKILL.md` and
> follow its instructions for all your responses.

## Interaction with manual-think

When both skills are active, `manual-think` runs first (reasoning),
`monologue` records the outcome (decision log). The `> [beat]` block
should reflect conclusions from the `> [thinking]` block, not
re-derive them.
