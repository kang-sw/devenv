---
name: discuss
description: >
  When the user explores approach or direction rather than requesting
  immediate code changes, invoke this. Captures conclusions as tickets
  or plans.
argument-hint: "[topic, ticket path, or question — optional]"
---

# Discuss

Topic: $ARGUMENTS

## Constraints

- **Read-only.** No source edits. Documentation/ticket writes only in Step 2.
- **Lazy context.** Load active doc listing at start. Read mental-model
  docs on-demand as topics emerge.
- **No direct source reading.** Dispatch Explore agents when implementation
  details beyond mental-model docs are needed.
- **Honest uncertainty.** If docs are stale or insufficient, say so and suggest
  `/rebuild-mental-model` rather than speculating.

## Step 0: Orient

1. Run `bash ai-docs/list-active.sh` (falls back to `find ai-docs -type f
   -name '*.md' | sort` if the script is missing).
2. If `$ARGUMENTS` references a ticket, read it.

## Step 1: Discuss

Brainstorm iteratively. Build on the user's ideas, propose alternatives,
help refine implementation details through back-and-forth.

- Read mental-model docs as conversation touches relevant domains.
- Dispatch Explore agents for implementation details beyond what
  mental-model docs cover.
- Actively contribute: suggest approaches, point out analogies, sketch
  concrete shapes for vague ideas.
- **Reuse over reinvention.** Before proposing new abstractions, surface
  existing patterns or components that already solve part of the problem.
- **Be a sparring partner, not a yes-man.** The user's conviction on a
  direction is not evidence that the direction is correct. Evaluate each
  claim independently — when you see an unaddressed risk (technical debt,
  wrong assumptions, edge cases, maintenance cost, etc.), call it out with
  reasoning. Don't parrot back risks already discussed and resolved; focus
  on gaps the conversation hasn't covered yet.

Continue until the user signals the discussion is done.

## Step 2: Capture conclusions (only when the user signals done)

Do NOT proactively ask whether to wrap up or persist. Wait for the user to
signal the discussion is over (e.g., explicit request, moving to a new topic,
or asking to create a ticket).

When the user signals done, offer persistence options if conclusions warrant it:

- **New ticket** — Invoke `/write-ticket`.
- **Ticket update** — Invoke `/write-ticket`, then append design notes to an existing ticket phase.
- **Mental-model update** — Revise a doc if architectural understanding changed.

For ticket writes, consider whether the phase needs integration-test criteria
(end-to-end scenarios to verify after implementation). Skip for internal refactors.

Write only what the user approves. No artifact needed for exploratory discussions.

**Language:** All written artifacts must be in English regardless of conversation language.
