---
name: write-ticket
description: Create or update repository workflow tickets. Use when the user asks to write, create, edit, promote, drop, or close a ticket, or when a discussion needs to be captured as a durable ticket.
---

# Write Ticket

## Invariants

- Read the repository ticket conventions before creating or changing tickets.
- Treat ticket stems as stable identifiers and avoid renaming stems to change dates.
- Move ticket status by moving files between status directories.
- Do not edit a phase after it contains a `### Result` section.
- Preserve user intent, constraints, rejected alternatives, and verification limits in ticket text.
- Keep implementation-plan details out of tickets unless they are needed to explain scope.
- Keep all AI-authored ticket content in English.
- Use `ws.convention.read` for ticket conventions.

## On: Create Ticket

1. Call MCP tool `ws.convention.read` with `{"name":"ticket-conventions"}`.
2. Classify the request with `judge: ticket-kind`.
3. Choose initial status with `judge: initial-status`.
4. Generate a `YYMMDD-<category>-<slug>.md` stem using today's date and a short stable slug.
5. Write frontmatter with `title` and any concrete `related` references.
6. Write `## Background` with the problem, goal, and reason this ticket exists.
7. Add `## Decisions`, `## Constraints`, or `## Prior Art` only when they preserve settled context.
8. For actionable tickets, add stable `### Phase N: <title>` sections with success criteria.
9. For research tickets, add freeform topic headings instead of phases.
10. Add the ticket to `ai-docs/_index.md` `## Ticket Queue` when the initial status is `todo`.
11. Commit only the created ticket and directly required index change.
12. Report the created path as `Ticket: ai-docs/tickets/<status>/<stem>.md`.

## On: Edit Ticket

1. Call MCP tool `ws.convention.read` with `{"name":"ticket-conventions"}`.
2. Read the target ticket before editing it.
3. Apply only the requested change and any required consistency update.
4. Use `git mv` for status transitions when possible.
5. Add `completed: YYYY-MM-DD` when moving a ticket to `.done`.
6. Update `ai-docs/_index.md` when queue membership changes.
7. Commit only the changed ticket and directly required index change.
8. Report the updated path as `Ticket: ai-docs/tickets/<status>/<stem>.md`.

## Judgments

### judge: ticket-kind

Use `research` when the work is exploratory and has no implementation phases. Use `epic` when the body decomposes broad scope into child tickets. Use `feat`, `bug`, `refactor`, or `chore` when the work is actionable and phaseable.

### judge: initial-status

Use `idea` when the goal, scope, or acceptance criteria are still unsettled. Use `todo` when the ticket is actionable without another design conversation. When uncertain, choose `idea`.

### judge: phase-size

One phase should cover one cohesive component or reviewable behavior. Split phases when a single phase would mix unrelated files, unrelated risks, or separate verification surfaces.

### judge: spec-linkage

If a ticket affects caller-visible behavior, identify the relevant spec document and existing spec stems by direct document inspection. If no spec entry exists, state the missing behavior and ask whether to write or update the spec before continuing.

## Templates

### Ticket Path

```text
Ticket: ai-docs/tickets/<status>/<YYMMDD-category-slug>.md
```

### Todo Queue Entry

```markdown
`<stem>` — <one-line purpose and dependency notes>
```

## Doctrine

Ticket writing optimizes for the future session's limited recovery budget: the ticket must preserve the decisions, constraints, and unresolved risks that would otherwise require rereading the whole conversation. When a rule is ambiguous, apply whichever interpretation better preserves the future session's limited recovery budget.
