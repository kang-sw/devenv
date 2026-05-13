# Executor Wrapup

Shared wrapup procedure for executor-series skills. Load and follow after a
completed implementation phase has a reviewable result commit.

## Invariants

- Wait for `_index.md` refresh and any ticket updates to complete before running the doc-commit gate.
- Doc-commit gate always runs; prior steps may have dirtied `ai-docs/`.
- Commit message for doc updates follows AGENTS.md commit rules; type is `docs`.
- Ancestor loading: any agent that reads `ai-docs/mental-model/<domain>/<sub>.md` must read `ai-docs/mental-model/<domain>/index.md` first.

## Ancestor Loading

1. Callers using `ws/mental_models.find` should read returned parent docs before child docs.
2. Callers using manual paths must read the parent before the child.
3. Delegation prompts must include the ancestor-loading rule when subagents read mental models.

## Doc Pipeline

1. Refresh `ai-docs/_index.md` to reflect current inventory, descriptions, and queue state.

## Doc Commit Gate

Run after Doc Pipeline and any ticket update complete:

```bash
git status --porcelain ai-docs/
```

- If output is non-empty: create a commit covering all `ai-docs/` changes.
- If output is empty: no-op and proceed.

## Ticket Update

Ticket-driven only:

1. Append `### Result (<short-hash>) - YYYY-MM-DD` to each completed phase.
   Use the result commit supplied by the caller.
2. Move completed tickets to the next status directory when all phases complete.
3. Remove completed ready tickets from the `## Ticket Queue` section in `ai-docs/_index.md`.

## Doctrine

This playbook optimizes for complete doc-state capture: every executor skill
exits with a clean git working tree for `ai-docs/`. When a rule is ambiguous,
apply whichever interpretation ensures no doc-pipeline output is left
uncommitted.
