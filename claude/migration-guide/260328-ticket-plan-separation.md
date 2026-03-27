# Migration: Ticket-Plan Responsibility Separation

**Date:** 2026-03-28
**Affects:** All active tickets (in `idea/`, `todo/`, `wip/`) and their associated plans.

## What Changed

The boundary between tickets and plans has been redefined:

| Before | After |
|--------|-------|
| Tickets held concrete data specs (formats, API contracts, byte layouts) alongside intent | Tickets hold **intent only**: goals, constraints, rejected alternatives, rationale |
| Plans held implementation tactics (file paths, signatures, delegation) | Plans hold **intent + concrete specs**: data formats, API contracts, schemas, plus implementation tactics |
| No formal link between ticket phases and plans | Ticket frontmatter includes `plans:` mapping phases to plan paths |
| Plan path format: `YYMM/DD-HHMM-<plan-name>.md` | Plan path format: `YYYY-MM/DD-hhmm.<plan-name>.md` |

## Frontmatter Change

Tickets now support a `plans:` field mapping each phase to its plan:

```yaml
plans:
  - phase-1: 2026-03/28-1430.event-serialization
  - phase-2: null  # not yet planned
```

The value is the plan's relative path stem under `ai-docs/plans/` (without `.md`).
Use `null` for phases not yet planned.

## Migration Steps

For each active ticket (not in `done/` or `dropped/`):

1. **Audit phase content.** Identify any concrete specifications that belong
   in a plan rather than the ticket:
   - Byte layouts, wire formats, schema definitions
   - Specific API signatures or endpoint contracts
   - Concrete data structures beyond intent-level description

2. **If a plan already exists for that phase:**
   - Move concrete specs from the ticket phase into the plan's Context section.
   - Add the `plans:` entry to the ticket frontmatter.

3. **If no plan exists yet:**
   - Leave concrete specs in the ticket for now (they'll migrate naturally
     when `/write-plan` runs for that phase).
   - Add `plans:` with `null` for that phase.

4. **Rephrase ticket phase content** to focus on intent:
   - Keep: "We need versioned binary serialization; JSON rejected for perf."
   - Move to plan: "Header: 4-byte magic + u16 version, payload: LZ4-compressed..."

5. **Link existing orphan plans.** If plans exist that implement a ticket
   phase but aren't referenced, add the `plans:` mapping.

6. **Rename existing plans** from old format (`YYMM/DD-HHMM.<name>.md`) to
   new format (`YYYY-MM/DD-hhmm-<name>.md`). Use `git mv` to preserve
   history. Update any `@<plan-path>` references in plan-mode files.

## Scope

- **Apply to:** `idea/`, `todo/`, `wip/` tickets.
- **Skip:** `done/` and `dropped/` tickets (historical, no action needed).
- **Timing:** Apply during the next session that touches each ticket.
  No bulk migration required — migrate on contact.
