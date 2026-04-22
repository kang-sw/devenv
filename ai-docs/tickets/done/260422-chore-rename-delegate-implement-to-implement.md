---
title: "Rename /delegate-implement skill to /implement"
related:
  260422-chore-rename-implement-to-edit: prerequisite
spec:
  - 260422-implement-skill
  - 260422-implement-feature-branch-mode
---

# Rename /delegate-implement skill to /implement

## Background

Phase 1 renamed the owner-direct shortcut from `/implement` to `/edit`, freeing
the name `/implement` for the canonical delegation path. This ticket (phase 2)
completes the rename by promoting `/delegate-implement` to `/implement`.

After this ticket, the names reflect their roles:
- `/edit` — exception-path, direct authorship by the main agent
- `/implement` — default path, delegated implementer + reviewer cycle

Depends on `260422-chore-rename-implement-to-edit` (done, merged 0577f75).

## Phases

### Phase 1: Rename /delegate-implement → /implement across all files

Rename the skill directory and update all cross-references.

Changes required:

- `git mv claude/skills/delegate-implement/ claude/skills/implement/`
- Inside `claude/skills/implement/SKILL.md`: update the skill name/title and any
  self-references from `/delegate-implement` to `/implement`; update `ws:delegate-implement`
  invocation references to `ws:implement`.
- Scan all skill files for `/delegate-implement` references and update to `/implement`.
  Files likely to contain hits: `edit/SKILL.md`, `write-skeleton/SKILL.md`,
  `write-plan/SKILL.md`, `proceed/SKILL.md`, `parallel-implement/SKILL.md`,
  `team-lead/SKILL.md`, `enter-session/SKILL.md`, `discuss/SKILL.md`.
  Also check `ws:delegate-implement` as a `subagent_type` or skill invocation
  target — update to `ws:implement`.
- `ai-docs/_index.md`: update Skill Inventory and Canonical Flows sections.
- `CLAUDE.md` (project and global): update any `/delegate-implement` references.
  The global `~/.claude/CLAUDE.md` is outside the repo — verify manually.
- `claude/.claude-plugin/plugin.json`: no update needed — skills are auto-discovered
  from the directory; renaming `delegate-implement/` to `implement/` is sufficient.
- After the directory rename, run `claude plugin update ws@ws` to propagate
  the renamed skill to the plugin cache.

Constraint: do not rename `/parallel-implement` or `/edit`.

Success: both of the following greps return zero results:

```
grep -rE '/delegate-implement' claude/ ai-docs/_index.md CLAUDE.md
grep -rE 'ws:delegate-implement' claude/ ai-docs/_index.md CLAUDE.md
```

Note: `ai-docs/spec/workflow-skills.md` was pre-updated in commit 5e5b6fc to describe
`/implement` as if implemented. That spec state is intentionally transient — it resolves
the moment this ticket lands.

The implementation commit's `## Spec` section should reference both
`260422-implement-skill` and `260422-implement-feature-branch-mode`.

### Result (e29ae63) - 2026-04-22

Merged to `main` in two commits (`4f1b23f`, `e6c5a76`, merge `e29ae63`).

All 10 cross-reference files updated. Both success greps return zero results.

Deviation: `ai-docs/spec/agent-system.md:100` retained the old name because the success grep was scoped to `claude/ ai-docs/_index.md CLAUDE.md` and did not cover `ai-docs/spec/`. Fixed in second commit `e6c5a76`. Future rename tickets should include `ai-docs/spec/` in the grep scope.

Mental-model docs were already clean — no updates needed.
