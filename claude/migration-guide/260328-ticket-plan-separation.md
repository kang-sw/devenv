# Migration: Ticket-Plan Responsibility Separation

**Date:** 2026-03-28
**Affects:** All active tickets (in `idea/`, `todo/`, `wip/`) and their associated plans.

## What Changed

The boundary between tickets and plans has been redefined:

| Before | After |
|--------|-------|
| No clear boundary between ticket and plan content | Tickets hold **decisions and suggested approaches from discussion**; plans hold **codebase-derived** mapping and evaluate candidates |
| No formal link between ticket phases and plans | Ticket frontmatter includes `plans:` mapping phases to plan paths |
| Plan path format: `YYMM/DD-HHMM.<name>.md` | Plan path format: `YYYY-MM/DD-hhmm.<plan-name>.md` |

## Frontmatter Change

Tickets now support a `plans:` field mapping each phase to its plan:

```yaml
plans:
  phase-1: 2026-03/28-1430.event-serialization
  phase-2: null  # not yet planned
```

The value is the plan's relative path stem under `ai-docs/plans/` (without `.md`).
Use `null` for phases not yet planned.

## Migration Steps

For each active ticket (not in `done/` or `dropped/`):

1. **Add `plans:` frontmatter** to each ticket with phases. Use `null` for
   phases not yet planned; fill in plan path stems for existing plans.

2. **Audit phase content.** Identify any codebase-derived details that
   shouldn't be in the ticket (file paths, specific type reuse decisions
   that depend on current code layout). These belong in the plan.
   Keep everything from discussion — including suggested approaches,
   data formats, struct shapes, pseudo code, algorithm sketches.

3. **Link existing orphan plans.** If plans exist that implement a ticket
   phase but aren't referenced, add the `plans:` mapping.

4. **Rename existing plans** from old format (`YYMM/DD-HHMM.<name>.md`) to
   new format (`YYYY-MM/DD-hhmm.<name>.md`). Use `git mv` to preserve
   history. Update any `@<plan-path>` references in plan-mode files.

## Scope

- **Apply to:** `idea/`, `todo/`, `wip/` tickets.
- **Skip:** `done/` and `dropped/` tickets (historical, no action needed).
- **Timing:** Apply during the next session that touches each ticket.
  No bulk migration required — migrate on contact.
