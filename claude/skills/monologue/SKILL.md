---
name: monologue
description: >-
  Structured operational narration for long sessions. Wraps every
  action batch in a declare-act-observe cycle, keeping assumptions
  and expectations live in the conversation context.
---

# Monologue

## Purpose

In long sessions, the *why* behind each decision evaporates. This
skill externalizes a compact decision record around every action,
keeping assumptions, expectations, and lessons visible as durable
context anchors.

This is the **residue of reasoning**, not reasoning itself: what
was assumed, what was decided, and — after observation — what held
and what broke.

## The Beat

A **beat** is one hypothesis cycle: declare → act → observe.

An **action batch** is one or more tool calls sent together in a
single turn, or a response to the user — any moment the agent
commits to output. Parallel calls in one turn are one batch.
Sequential calls serving the same intent may share a batch.

Even a trivial batch — a single file read, a `mv` command — gets
a beat. A one-line beat is fine; a missing beat is not. No
exceptions.

### Opening — `> [beat: <label>]`

State what you will do, why, and what you expect. Free-form. Make
assumptions explicit enough that the closing can judge them.

```
> [beat: read auth middleware]
>     Need the registration point before editing guard logic.
>     Assuming file hasn't moved. Expect app.use() around L40-60.
```

### Closing — `> [/beat: match | drift | open]`

Compare reality against the opening.

- **match** — assumptions held, proceed.
- **drift** — name which assumption broke, what challenge this
  surfaces, and how the plan adjusts. Proceeding without
  acknowledging drift is not allowed. When drift is severe enough
  that the entire beat's direction was wrong, state so explicitly
  and reframe — this is an **abandon**, a natural extreme of drift.
- **open** — no observable result yet (e.g. response awaiting user
  reply). Note what to watch for. When the result arrives, the
  next beat's opening should reference the resolution.

```
> [/beat: drift]
>     Expected the DB schema to have a `users.role` column but
>     found a separate `roles` junction table. Assumption "flat
>     role field" was wrong. Need to join through the junction
>     table — adjusting query plan.
```

## Principles

1. **Declare to falsify.** State expectations that can be proven
   wrong. Vague expectations ("should work") defeat the purpose.
2. **Drift is signal, not failure.** Wrong assumptions caught early
   are valuable. The closing turns them into named challenges that
   propagate forward.
3. **Carry lessons.** When a beat surfaces a broken assumption —
   or a surprisingly confirmed one — later beats reference it.
   "Previous beat revealed X, accounting for that here."
4. **Concise over complete.** One to three lines per block is the
   norm. Write decisions, not prose.
5. **Opening before action.** The hypothesis must appear before the
   tool calls or response text in the same turn — earlier in the
   output, not in a prior turn.
6. **Every beat closes.** Do not batch multiple closes together.
7. **English only.** Monologue blocks in English regardless of
   conversation language.

## Examples

**Match:**
```
> [beat: grep rate-limit config]
>     Looking for RATE_LIMIT in config/. Assuming project convention
>     puts it in config/default.ts. Expect a numeric value.

... Grep ...

> [/beat: match]
>     Found RATE_LIMIT=100 at config/default.ts:23. Next: edit guard.
```

**Drift — challenge surfaces:**
```
> [beat: edit rate-limit guard]
>     Capping unauth requests to 10/min per spec.
>     Assuming guard handles unauth routes only.

... Edit ...

> [/beat: drift]
>     Guard is shared with admin routes. Wrong assumption: "unauth-only."
>     Challenge: split guards first. Revising plan.
```

**Open — awaiting user, then resolved:**
```
> [beat: clarify architecture options]
>     Presenting A vs B trade-offs. Risk: user may mean C, not A/B.

... response ...

> [/beat: open]
>     Watching for: user picks A/B or redirects to C.
```
```
> [beat: implement option A]
>     User confirmed A in previous reply, resolving the open beat.
>     Starting implementation per the trade-offs discussed.
```

**Lesson carried forward:**
```
> [beat: create unauth rate-limit guard]
>     Previous beat: existing guard is shared across route groups.
>     Creating a dedicated unauth guard instead.
>     Expect: new file, existing tests unaffected.

... Write + Bash ...

> [/beat: match]
>     New guard created. Existing test suite passes. Next: wire it
>     into the unauth router.
```

**Subagent spawn:**
```
> [beat: delegate test-suite verification]
>     Spawning test-runner agent to verify nothing broke after the
>     guard split. Expect: all green, or a list of failures with
>     file paths.

... Agent tool call ...

> [/beat: match]
>     Agent reports all 47 tests pass. Proceeding to PR.
```

## Subagent Propagation

Subagents run their own monologue independently. The parent beat
names the expected deliverable, not the subagent's internal steps.

When spawning subagents, prepend to every prompt:

> Before starting, read `~/.claude/skills/monologue/SKILL.md` and
> follow its instructions for all your responses.
