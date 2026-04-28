---
name: write-skeleton
description: >
  /proceed dispatches this after /write-ticket when public contracts need
  to crystallize before implementation. Crystallize public contracts as
  interface stubs and integration tests.
argument-hint: [ticket-path]
---

# Write Skeleton

Target: $ARGUMENTS

## Invariants

- Skeleton = the first code change for a ticket. No implementation code — only interface stubs and integration tests.
- Lead is a lightweight coordinator: identify the ticket, pass contract directives, review, commit.
- The delegate owns design: reads the ticket, explores the codebase, decides type shapes and test structure.
- Contract directives = lead's judgment on points the delegate cannot derive from ticket + code alone.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- The delegate does not commit — lead reviews and commits.
- Register the skeleton-writer agent once per invocation via `ws-new-named-agent`; resume via `ws-call-named-agent` for amendment rounds.

## On: invoke

### 1. Identify contract directives

1. Read the ticket. Note ambiguities that need lead judgment — scope boundaries, design choices between alternatives, integration constraints not obvious from the ticket alone.
2. Skim relevant mental-model docs only if needed to resolve those ambiguities.
3. Formulate **contract directives**: 2–5 binding decisions. Not a full design — just fences and choices the delegate must follow.

### 2. Delegate

**Register (one Bash call):**

```bash
ws-new-named-agent skeleton-writer --model opus --system-prompt skeleton-writer.md
```

**Spawn (one Bash call):**

```bash
ws-call-named-agent skeleton-writer - <<'PROMPT'
Ticket: <ticket-path>

## Contract directives
- <binding decisions only — things not derivable from ticket + codebase>
PROMPT
```

### 3. Review

1. Run `git diff HEAD` and `git status --short` to review the skeleton output. Read specific files only if a reported deviation warrants deeper inspection.
2. Verify contracts match the ticket intent and honor the directives.
3. Run build to confirm compilation. Do not run tests — tests will fail by design because stubs are unimplemented. Passing tests is the implementor's responsibility, not the skeleton's.
4. If issues found:
   - **Minor** — fix directly.
   - **Structural** — relay amended directives via a follow-up call (session resumes with full context):
     ```bash
     ws-call-named-agent skeleton-writer - <<'PROMPT'
     Amend: <issues and revised directives>
     PROMPT
     ```
     Re-review after each round.

### 4. Commit

1. Commit stubs and tests together as one logical unit.
2. Commit message: `feat(<scope>): skeleton — <what contracts are established>`
3. Include `## AI Context` with key contract decisions.
4. Include `## Ticket Updates` with the ticket stem and what future phases must know.
5. Update the ticket's `skeletons:` frontmatter with the phase and commit hash (e.g., `phase-1: abc1234`). Only add entries for phases that have a skeleton — no null placeholders.

### 5. Suggest next step

Based on implementation complexity and session warmth on the target:
- **Wide** (multiple independent modules): suggest `/implement` (one scope at a time) or ask the user to split into separate tickets.
- **Narrow + warm** (single module, main agent already engaged the code): suggest `/edit`.
- **Narrow + cold** (single module, main agent is cold on the target): suggest `/implement`.

Warmth is a property of the current session — has the main agent read files in the target scope this session, or did the user explicitly signal direct authorship? If ambiguous, suggest `/proceed` and let its routing judges decide.

Present the recommendation with brief rationale. Do not auto-invoke.

## Judgments

### judge: test-scope

| Layer | Default | Condition |
|---|---|---|
| Structural seam tests | Always | Every cross-module boundary |
| Behavioral tests | Include when ticket specifies behavior | Any behavior the ticket describes — drop the "complex" qualifier |
| Error / edge case tests | Opt-in | Only when the ticket explicitly specifies error contracts or edge conditions |

### judge: stub-granularity

| Level | When |
|-------|------|
| Module-level (types + top-level functions) | Most cases |
| Method-level (all public methods stubbed) | When the ticket specifies detailed API surface |

## Doctrine

The skeleton optimizes for **contract-first delegation with minimal
coordinator overhead** — the lead passes only binding decisions the
delegate cannot derive, the delegate owns design and exploration. This
keeps the coordinator's context light while locking public interfaces
and acceptance criteria in code before implementation begins. When a rule
is ambiguous, apply whichever interpretation better preserves contract
stability while minimizing what the coordinator must serialize.
