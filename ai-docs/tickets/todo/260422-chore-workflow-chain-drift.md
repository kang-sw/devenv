---
title: "workflow skills: fix stale canonical chain references in discuss, write-spec, write-skeleton"
---

# workflow skills: fix stale canonical chain references in discuss, write-spec, write-skeleton

## Background

A full sweep of workflow skill docs (triggered during 260422-chore-write-ticket-workflow-drift)
found three files with stale or contradictory canonical chain references. The authoritative
chain is:

```
/discuss → /write-spec → /write-ticket → /proceed
                                             ↓
               /write-skeleton? → /write-plan? → /implement
                                              → /delegate-implement
                                              → /parallel-implement
```

All three files below pre-date the chain reorder and were not included in the prior fix.

## Drifted locations

### Critical

- `claude/skills/discuss/SKILL.md` — "Workflow Context" block (line ~76) states the chain as
  `/discuss → /write-spec → /write-ticket → /write-skeleton → /implement`. Two errors:
  1. `/proceed` omitted entirely — the required auto-router after `/write-ticket`.
  2. `/write-skeleton` presented as mandatory and directly chained, not optional via `/proceed`.

### Important

- `claude/skills/write-spec/SKILL.md` — frontmatter `description` field says
  "when chained from /write-ticket after a phase that changes public behavior."
  Implies `/write-spec` runs after `/write-ticket` — the old reversed order.
  Should reflect that `/write-spec` runs before `/write-ticket` on the canonical chain.

- `claude/skills/write-skeleton/SKILL.md` — frontmatter `description` field says
  "After /write-ticket, before /implement or /delegate-implement."
  Omits `/proceed` as the mediating router. Should note that `/write-skeleton` is
  invoked by `/proceed`, not directly after `/write-ticket`.

## Phases

### Phase 1: Fix discuss/SKILL.md Workflow Context block

Update the canonical chain statement to match the authoritative chain:
- Include `/proceed` as the router after `/write-ticket`.
- Mark `/write-skeleton` and `/write-plan` as optional (`?`).
- Include all three implementation paths (`/implement`, `/delegate-implement`, `/parallel-implement`).

Read `claude/skills/discuss/SKILL.md` in full before editing — only change the chain
statement, do not alter any other section.

Success: no `/write-skeleton → /implement` direct chain in the file; `/proceed` present.

### Phase 2: Fix write-spec/SKILL.md and write-skeleton/SKILL.md frontmatter descriptions

- `write-spec`: update frontmatter `description` to reflect that it runs before
  `/write-ticket` on the canonical chain, not after.
- `write-skeleton`: update frontmatter `description` to note that it is invoked by
  `/proceed` rather than directly after `/write-ticket`.

Keep descriptions concise — frontmatter description fields are brief (1–2 lines).

Success: neither description implies the old chain order; `/proceed`'s role is visible.
