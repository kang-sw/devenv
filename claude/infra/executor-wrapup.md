# Executor Wrapup

Shared wrapup procedure for all executor-series skills (ws:edit, ws:implement, ws:parallel-implement).
Load and follow after the implementation commit is made and any merge step is done.

## Invariants

- Wait for _index.md refresh and any ticket updates to complete before running the doc-commit gate.
- Doc-commit gate always runs — even when no changes are expected, prior steps may have dirtied ai-docs/.
- Commit message for doc updates follows CLAUDE.md commit rules; type is `docs`.
- Ancestor loading: any agent that reads `ai-docs/mental-model/<domain>/<sub>.md` must read `ai-docs/mental-model/<domain>/index.md` first. Ancestor docs are loaded before the sub-domain doc so inherited `## Domain Rules` are visible. Executor skills propagate this rule to every subagent they spawn.

## §Ancestor Loading

Shared across all executor-series skills. When preparing or spawning work
that touches a sub-domain mental-model doc
(`ai-docs/mental-model/<domain>/<sub>.md`), the reading agent must load
the parent `ai-docs/mental-model/<domain>/index.md` before the sub-domain
doc. `list-mental-model` emits the parent alongside a matching child
automatically; callers relying on manual paths must apply the rule
themselves. Executor skills that delegate to subagents include this rule
verbatim in the spawn prompt so the subagent observes the contract inside
its own read ordering.

## §Doc Pipeline

1. Refresh `ai-docs/_index.md` — update inventory, descriptions, and layout to reflect current state.

## §Doc Commit Gate

Run after §Doc Pipeline and any §Ticket Update complete:

```bash
git status --porcelain ai-docs/
```

- If output is non-empty: create a commit covering all `ai-docs/` changes. Commit message type `docs`, scope reflects what changed (e.g., `docs(spec): ...`, `docs(mental-model): ...`, or `docs: update doc pipeline outputs` when mixed).
- If output is empty: no-op — proceed.

## §Ticket Update  *(ticket-driven only)*

1. Append `### Result (<short-hash>) - YYYY-MM-DD` to each completed phase. Content: what was implemented, deviations from plan, key findings for future phases. Short hash = implementation commit (or merge commit when applicable).
2. If all phases are complete: `git mv` ticket to the next status directory.

## Doctrine

This playbook optimizes for **complete doc-state capture** — every executor skill exits with a clean git working tree for `ai-docs/`. When a rule is ambiguous, apply whichever interpretation ensures no doc-pipeline output is left uncommitted.
