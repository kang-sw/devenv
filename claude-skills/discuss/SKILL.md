---
name: discuss
description: Start a design discussion session with full project context. Loads mental-model docs on demand and delegates source exploration to subagents, keeping the main context window focused on the conversation.
argument-hint: "[topic, ticket path, or question]"
---

# Design Discussion

Topic: $ARGUMENTS

## Principles

- **Do not modify code.** This is a discussion-only session. Do not create, edit, or
  delete any source files. Documentation and ticket writes are allowed only when
  capturing conclusions (Step 2).
- **Lazy context loading.** Read `ai-docs/_index.md` at the start to learn what
  mental-model documents exist. Load specific documents only when the conversation
  reaches them — never all at once.
- **Do not read source directly.** When the discussion needs details beyond what
  mental-model documents provide, delegate source exploration to subagents (Explore,
  general-purpose Agent). Keep the main context window reserved for the conversation.
- **Honest uncertainty.** If mental-model docs are stale or insufficient for the
  topic, say so explicitly. Suggest `/rebuild-mental-model` rather than speculating.

## Step 0: Orient

1. Read `ai-docs/_index.md` for project structure and mental-model document inventory.
2. If `$ARGUMENTS` references a ticket, read it.
3. For broad or ambiguous topics, briefly state your understanding and confirm with the
   user before proceeding. For focused questions, answer directly.

## Step 1: Discuss

Engage in free-form conversation with the user.

- Load mental-model documents on demand as new modules become relevant to the
  discussion (`ai-docs/mental-model/<module>/index.md`).
- When implementation details are needed beyond what docs cover, dispatch a subagent
  to explore source — do not read source yourself.
- Offer trade-off analysis, surface constraints from existing architecture, and flag
  risks when relevant.

Continue until the user indicates the discussion is done.

## Step 2: Capture conclusions

When the discussion wraps up, ask the user how (or whether) to persist the outcome:

- **New ticket** — `ai-docs/tickets/todo/YYMMDD-<name>.md` for a decided feature.
- **Ticket update** — Append design notes to an existing ticket phase.
- **Mental-model update** — Revise a mental-model document if the discussion changed
  architectural understanding.

Only write what the user approves. If the discussion was purely exploratory, no
artifact is needed.
