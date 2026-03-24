---
name: write-ticket
description: Create or edit a ticket following project conventions. Also serves as the canonical ticket convention reference — other skills should load this before ticket operations.
argument-hint: "[topic/description for new ticket, or ticket path to edit]"
---

# Write Ticket

Target: $ARGUMENTS

## Ticket Conventions

**Location:** `ai-docs/tickets/<status>/YYMMDD-<category>-<name>.md`

**Naming:**
- `YYMMDD` is the **creation date** — never changes when the ticket moves.
- Categories: `bug`, `feat`, `refactor`, `chore`, `research`.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by
  full path including the status directory. This keeps references stable across moves.

**Frontmatter:**
- Requires `title`.
- Add `started: YYYY-MM-DD` on move to `wip/`.
- Add `completed: YYYY-MM-DD` on move to `done/`.

**Status is directory-based only:** `idea/` → `todo/` → `wip/` → `done/` (or `dropped/`).
The containing directory is the single source of truth — do not duplicate status
in frontmatter or elsewhere.

**Moving tickets:** `git mv` to the new status directory in the same commit.
Since references use stems, no cross-link updates are needed.

**Result entries:** After completing a ticket phase, append a
`### Result (<short-hash>) - YY-MM-DD` subsection recording what was implemented,
deviations from the plan, and key findings for future phases.

## Steps

1. If `$ARGUMENTS` references an existing ticket, read it.
2. If creating a new ticket:
   - Determine category from the topic.
   - Choose initial status directory (`idea/` for vague, `todo/` for actionable).
   - Write the ticket with frontmatter and clear problem/goal statement.
3. If editing an existing ticket:
   - Read the ticket first.
   - Apply the requested changes (append result, update phase, move status).
4. For ticket moves, use `git mv` and update frontmatter dates as needed.

**Language:** All ticket content must be in English regardless of conversation language.
