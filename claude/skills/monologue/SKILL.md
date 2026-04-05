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

## Purpose

In long sessions, the *why* behind each decision evaporates. This
skill externalizes a running stream of self-talk around every action,
keeping assumptions, expectations, and lessons visible as durable
context anchors.

Where internal reasoning verifies hypotheses against itself (propose
→ challenge → resolve), monologue verifies them against **external
reality** (assume → act → observe). It records what was assumed,
what was decided, and — after observation — what held and what broke.

## Core Directive

**Every action gets narration. No exceptions. In English only.**

Before tool calls, after results, between steps, before responding
to the user, after responding — narrate in `> [monologue]` blocks.
Each action needs at least a before-block and an after-block. A
trivial action gets a one-line block; a missing block is a rule
violation.

```
> [monologue]
> <free-form self-talk in English>
```

No closing tag. The blockquote ends when the monologue ends. Multiple
blocks per turn are natural and expected.

## Vocabulary

Use these terms naturally within monologue blocks — they are words,
not syntax:

- **assumption** — something you believe before acting. State it
  explicitly so observation can falsify it.
- **match** — reality aligned with your assumptions. Proceed.
- **drift** — an assumption was wrong. Name it, describe what
  difficulty this creates, and state the plan adjustment. Proceeding
  past drift without acknowledging it is not allowed.
- **challenge** — a difficulty or constraint discovered during the
  session, whether from drift, tool errors, or new information.
  Carries forward to later blocks.
- **abandon** — drift so severe the entire direction was wrong.
  Reframe rather than patch.

## Principles

1. **Declare to falsify.** State expectations that can be proven
   wrong. Vague expectations ("should work") defeat the purpose.
2. **Drift is signal, not failure.** Wrong assumptions caught early
   are valuable. Name the broken assumption, name the challenge,
   adjust the plan.
3. **Carry lessons.** When a block surfaces a broken assumption —
   or a surprisingly confirmed one — later blocks reference it
   by restating the finding briefly: "Previous block revealed X,
   accounting for that here."
4. **Concise over complete.** One to three lines per block is the
   norm. Write decisions, not prose.
5. **Narrate before acting.** The hypothesis must appear before the
   tool calls or response in the same turn.
6. **Narrate after observing.** After every tool result or user
   response, a monologue block states whether reality matched and
   what comes next.
7. **English only — no exceptions.** All monologue blocks MUST be
   in English regardless of conversation language. User-facing text
   outside the blocks matches the user's language.

## Examples

```
> [monologue]
> Looking for RATE_LIMIT in config/. Assuming project convention
> puts it in config/default.ts.

... Grep ...

> [monologue]
> Match — found RATE_LIMIT=100 at config/default.ts:23. Editing
> the guard next. Assuming it only handles unauth routes.

... Edit ...

> [monologue]
> Drift — guard is shared with admin routes. My "unauth-only"
> assumption was wrong. Challenge: need to split guards before
> applying the rate limit. Creating a dedicated unauth guard.

... Write ...

> [monologue]
> Match — new guard created. Previous challenge addressed.
> Verifying tests still pass.

... Bash (test runner) ...

> [monologue]
> All 47 tests pass. Wiring the new guard into the unauth router.
```

**Abandon:**
```
> [monologue]
> Drift — the middleware approach won't work at all. The framework
> handles rate limiting at the gateway level, not per-route.
> Everything I've done so far is in the wrong layer. Abandon —
> switching to gateway config instead of middleware guards.
```

**Carrying a lesson forward:**
```
> [monologue]
> Previous block revealed the guard is shared across route groups.
> Accounting for that — creating a dedicated guard instead of
> modifying the shared one. Expect: new file, no test breakage.
```

**Responding to the user:**
```
> [monologue]
> User asked about A vs B. Presenting trade-offs. Risk: they
> might actually be asking about C — addressing that ambiguity.

... response text ...

> [monologue]
> Waiting for user's decision. If A, proceed to implementation.
> If they redirect to C, re-scope.
```

**Subagent spawn:**
```
> [monologue]
> Spawning test-runner to verify nothing broke after the guard
> split. Expect: all green or a failure list with file paths.

... Agent tool call ...

> [monologue]
> Agent reports all tests pass. Proceeding to PR.
```

## Subagent Propagation

Subagents run their own monologue independently. The parent block
names the expected deliverable, not the subagent's internal steps.

When spawning subagents, prepend to every prompt:

> Before starting, read `~/.claude/skills/monologue/SKILL.md` and
> follow its instructions for all your responses.
