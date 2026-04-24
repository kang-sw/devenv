---
title: Plugin Management
summary: Local .claude/skills/ tools for maintaining and auditing the ws plugin documents and infrastructure.
features:
  - `/polish-plugin-docs`
---

# Plugin Management

Local `.claude/skills/` skills for maintaining and auditing the ws plugin — its documents, agents, and infrastructure.

## `/polish-plugin-docs` {#260424-polish-plugin-docs}

User invokes `/polish-plugin-docs` to run an iterative review + simplification cycle on ws plugin documentation.

**Scope:** `claude/skills/`, `claude/agents/`, `claude/infra/` (`.md` files only). Excludes `claude/CLAUDE.home.md` and `claude/bin/`.

The skill operates on a dedicated branch. It runs an initial review for consistency, operational breakage, and authoring-rule compliance. It then iterates (2–3 rounds): a writer simplifies files without changing behavioral meaning, reviewers confirm findings are resolved and no new issues appear. The skill exits when both reviewers produce a clean Final report. User merges the branch with `--no-ff`.
