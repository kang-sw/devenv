---
title: "Rename /implement skill to /edit"
spec:
  - 260422-edit-skill
---

# Rename /implement skill to /edit

## Background

The `/implement` skill performs owner-direct single-scope editing — the main agent
reads source, edits directly, and commits with no subagent delegation. The name
`/implement` implies it is the canonical implementation path, but delegation
(`/delegate-implement`) is the actual default. Renaming the direct-edit shortcut
to `/edit` frees `/implement` for the upcoming `/delegate-implement` rename (phase 2).

This ticket covers phase 1 only: renaming the `/implement` skill to `/edit`.
Phase 2 (`/delegate-implement` → `/implement`) is a separate ticket that follows
after this rename is verified clean.

## Decisions

- **`/edit` over other names** (`/tweak`, `/patch`, `/direct`): `/edit` is neutral —
  it does not undersell the capability (unlike `/tweak`) and does not describe the
  mechanism rather than the intent (unlike `/direct`).
- **Phase 1 before phase 2**: avoids a transient state where neither
  `/implement` nor `/edit` exist, or where both exist simultaneously.

## Phases

### Phase 1: Rename /implement → /edit across all files

Rename the skill directory and update all cross-references.

Changes required:

- `git mv claude/skills/implement/ claude/skills/edit/`
- Inside `claude/skills/edit/SKILL.md`: update the skill name/title and any
  self-references from `/implement` to `/edit`.
- Skill files confirmed to contain `/implement` references (not part of
  `/delegate-implement`): `enter-session/SKILL.md`, `discuss/SKILL.md`,
  `write-skeleton/SKILL.md`, `proceed/SKILL.md`. Update each to `/edit`.
- `claude/.claude-plugin/plugin.json`: update skill registration entry if
  the skill name appears there.
- `ai-docs/_index.md`: update Skill Inventory and Canonical Flows sections.
- `CLAUDE.md` (project and global): update any `/implement` references. The global
  `~/.claude/CLAUDE.md` is outside the repo and is not covered by the success grep —
  verify it manually.
- After the directory rename, run `claude plugin update ws@ws` to propagate
  the renamed skill to the plugin cache. Without this step, `/implement`
  continues to resolve at runtime against the stale cache.

Constraint: do not rename `/delegate-implement` in this phase.
Constraint: the implementation commit's `## Spec` section should reference
`260422-edit-skill` for traceability (the slug-change commit is 52f76d8).

Success: the following grep returns only hits that are part of `/delegate-implement`
or `/parallel-implement`, or prose mentioning the old name as historical context:

```
grep -rE '/implement([^-]|$)' claude/ ai-docs/_index.md CLAUDE.md
```

### Result (0577f75) - 2026-04-22

Rename completed as specified. All 5 skill files updated; directory renamed via
git mv. plugin.json uses auto-discovery — no manual update needed. Correctness
review clean; grep verification passed. Global ~/.claude/CLAUDE.md contained
no `/implement` references. Phase 2 (`/delegate-implement` → `/implement`) proceeds
as a separate ticket.
