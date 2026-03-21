---
name: discuss
description: Open-ended project discussion. Loads mental-model context on demand; captures conclusions as tickets or doc updates when appropriate.
argument-hint: "[topic, ticket path, or question — optional]"
---

# Discuss

Topic: $ARGUMENTS

## Constraints

- **Read-only.** No source edits. Documentation/ticket writes only in Step 2.
- **Lazy context.** Read `ai-docs/_index.md` at start. Load other mental-model
  docs only when the conversation reaches them.
- **No direct source reading.** Delegate to subagents when details beyond
  mental-model docs are needed.
- **Honest uncertainty.** If docs are stale or insufficient, say so and suggest
  `/rebuild-mental-model` rather than speculating.

## Step 0: Orient

1. Read `ai-docs/_index.md` for project structure and mental-model inventory.
2. If `$ARGUMENTS` references a ticket, read it.
3. **If `$ARGUMENTS` is empty:** Read WIP tickets and run
   `git log --oneline -10`. Survey project state and propose 2-3 topics
   for the user to pick from (or let them raise something else).
4. If the topic spans independent subsystems, propose splitting into
   separate tickets before diving in.
5. Broad/ambiguous topics: state your understanding and confirm before
   proceeding. Focused questions: answer directly.

## Step 1: Discuss

Brainstorm iteratively. Build on the user's ideas, propose alternatives,
help refine implementation details through back-and-forth.

- Load mental-model docs on demand as modules become relevant.
- Dispatch subagents for implementation details beyond what docs cover.
- Actively contribute: suggest approaches, point out analogies, sketch
  concrete shapes for vague ideas.
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

- **New ticket** — `ai-docs/tickets/todo/YYMMDD-<category>-<name>.md`
- **Ticket update** — Append design notes to an existing ticket phase.
- **Mental-model update** — Revise a doc if architectural understanding changed.

For ticket writes, consider whether the phase needs integration-test criteria
(end-to-end scenarios to verify after implementation). Skip for internal refactors.

Write only what the user approves. No artifact needed for exploratory discussions.

**Language:** All written artifacts must be in English regardless of conversation language.
