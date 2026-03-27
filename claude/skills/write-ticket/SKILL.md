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
- `related:` — optional list of related ticket stems with freeform inline
  comments (e.g., `260301-feat-foo  # prerequisite`).
- `plans:` — maps phases to their plan path stems under `ai-docs/plans/`
  (without `.md`). Use `null` for phases not yet planned.
  ```yaml
  plans:
    - phase-1: 2026-03/28-1430-event-serialization
    - phase-2: null
  ```
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

## Phase Structure

A ticket may contain multiple phases (`### Phase N: <title>`). Each phase
should be **independently implementable** — one `/write-plan` invocation
can cover it.

Phase scoping rules:
- One phase touches **one cohesive component** (crate, package, directory,
  or logical subsystem) unless the change is inherently cross-component
  (e.g., defining an interface that two components share).
- Each phase has its own success criteria or test surface.
- Later phases may depend on earlier results; note dependencies explicitly.
- Phase numbers are sequential and **stable** — do not renumber after
  creation. Mark dropped phases as `[dropped]` instead of removing them.

When in doubt, prefer more phases over fewer. An overly granular ticket is
cheaper to merge than an oversized phase that stalls mid-implementation.

## Phase Content

> A ticket is the primary context-recovery artifact — a fresh session with no
> prior conversation must be able to reconstruct the full decision context from
> the ticket alone.

Phases carry **intent-level decisions**: goals, constraints, rationale, and
rejected alternatives. Concrete implementation approaches from discussion
(e.g., "use Pratt parsing for operator precedence", "lock-free ring buffer
for the event queue") belong here — they are design decisions, not
implementation tactics.

Leave to the plan: concrete data specifications (byte layouts, wire formats,
schema definitions, API signatures), file paths, function signatures,
delegation strategies, testing classifications — anything that depends on
the current code layout or requires codebase research to finalize.

When `/write-plan` runs for a phase, it pulls intent from the ticket and
produces the concrete spec. The `plans:` frontmatter field links them.

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
5. **Intent review** — Re-read the written/edited ticket and verify against
   the preceding conversation:
   - Are key decisions, constraints, and rejected alternatives captured?
   - Does the ticket distort or omit any agreed-upon intent?
   - Fix gaps in-place, then present a brief summary of what was
     added/corrected (or confirm nothing was missed) to the user.

**Language:** All ticket content must be in English regardless of conversation language.
