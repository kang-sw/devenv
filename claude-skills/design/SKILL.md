---
name: design
description: Design discussion leading to ticket creation or update. Loads mental-model docs on demand and delegates source exploration to subagents, keeping the main context window focused on the conversation.
argument-hint: "[topic, ticket path, or question]"
---

# Design

Topic: $ARGUMENTS

## Constraints

- **Read-only.** No source file edits. Documentation/ticket writes allowed only
  when capturing conclusions (Step 2).
- **Lazy context.** Read `ai-docs/_index.md` at start; load specific mental-model
  docs only when the conversation reaches them.
- **No direct source reading.** Delegate to subagents (Explore, general-purpose)
  when details beyond mental-model docs are needed.
- **Honest uncertainty.** If docs are stale or insufficient, say so. Suggest
  `/rebuild-mental-model` rather than speculating.

## Step 0: Orient

1. Read `ai-docs/_index.md` for project structure and mental-model inventory.
2. If `$ARGUMENTS` references a ticket, read it.
3. If the topic spans multiple independent subsystems, propose splitting into
   separate tickets before diving into details.
4. For broad/ambiguous topics, state your understanding and confirm before
   proceeding. For focused questions, answer directly.

## Step 1: Discuss

Engage in free-form conversation:

- Load mental-model documents on demand as modules become relevant.
- Dispatch subagents for implementation details beyond what docs cover.
- Surface trade-offs, architectural constraints, and risks when relevant.

Continue until the user indicates the discussion is done.

## Step 2: Capture conclusions

Ask the user how (or whether) to persist the outcome:

- **New ticket** — `ai-docs/tickets/todo/YYMMDD-<name>.md`
- **Ticket update** — Append design notes to an existing ticket phase.
- **Mental-model update** — Revise a document if architectural understanding changed.

When writing or updating a ticket, review whether the phase needs integration
test criteria — what scenarios should be verified end-to-end after implementation.
Not every phase needs this; skip for purely internal refactors.

Only write what the user approves. No artifact needed for purely exploratory discussions.
