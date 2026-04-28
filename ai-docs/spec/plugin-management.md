---
title: Plugin Management
summary: Local .claude-plugin/skills/ tools for maintaining and auditing the ws plugin documents and infrastructure.
---

# Plugin Management

Local `.claude-plugin/skills/` skills for maintaining and auditing the ws plugin — its documents, agents, and infrastructure.

## `/polish-plugin-docs` {#260424-polish-plugin-docs}

User invokes `/polish-plugin-docs` to run an iterative review + simplification cycle on ws plugin documentation.

**Scope:** `claude-plugin/skills/`, `claude-plugin/agents/`, `claude-plugin/infra/`, `claude-plugin/infra/prompts/` (`.md` files only). Excludes `claude-plugin/CLAUDE.home.md` and `claude-plugin/bin/`.

The skill operates on a dedicated branch. It runs an initial review for consistency, operational breakage, and authoring-rule compliance. It then iterates (2–3 rounds): a writer simplifies files without changing behavioral meaning, reviewers confirm findings are resolved and no new issues appear. The skill exits when both reviewers produce a clean Final report. User merges the branch with `--no-ff`.

## `/lint` {#260425-lint-skill}

Audits the plugin documentation for consistency, broken references, and downstream portability. Three reviewer agents run in parallel, each with a non-overlapping mandate:

- **Logical consistency** — contradictions, missing invariants, doc drift (`ws:document-reviewer`).
- **Broken references** — stale tool names, missing files, dead cross-references (`ws:document-reviewer`).
- **Downstream portability** — bare `claude-plugin/infra/` paths, renamed tools, hardcoded CWD assumptions (`ws:code-reviewer`). A known-renamed-tools list is embedded in the reviewer prompt to catch stale references by name.

**Scope:** `claude-plugin/skills/`, `claude-plugin/agents/`, `claude-plugin/infra/prompts/`, and `claude-plugin/bin/`.

Output: flat severity-ordered report (`Critical / Important / Minor`) aggregated from all three reviewers.
