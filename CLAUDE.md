# CLAUDE.md — devenv

## Project Summary

**devenv** — Personal developer environment bootstrap and Claude Code workflow
authoring. Neovim (LazyVim), Claude Code skills/agents, tmux, WezTerm, shell
dotfiles. One-shot `install.sh` for macOS/WSL/Linux.

This repo is **not a software project** — it is a configuration and template
repository. Tickets here track skill design research, not software
implementation. The skills and agents defined here are consumed by other
projects via symlink (`install.sh` handles this).

Read `ai-docs/_index.md` at session start for reference docs, infra layout,
and skill inventory.

## Workspace

```
ai-docs/   — Skill design research tickets (non-standard structure)
claude/    — Claude Code skills, agents, infra, CLAUDE.template.md
  skills/  — Skill definitions (discuss, write-ticket, write-plan, implement, etc.)
  infra/agents/ — Shared subagent role files (implementer, planner, reviewer, etc.)
  infra/   — Shared implementation references (impl-playbook, impl-process, ask.sh)
  agents/  — Agent definitions (rust-api-lookup, etc.)
  CLAUDE.template.md — Template CLAUDE.md for downstream projects
nvim/      — Neovim config (LazyVim, plugins, colorscheme)
shell/     — tmux, WezTerm, starship, zsh dotfiles, helper scripts
install.sh — Idempotent bootstrap (packages, symlinks, cleanup)
claude/migration-guide/ — Convention change guides for downstream projects
```

## What Happens Here

Sessions in this repo typically involve:
- **Skill/agent authoring** — editing workflow definitions in `claude/skills/` and `claude/agents/`
- **Workflow design discussion** — reasoning about use-cases, skill interactions, and convention changes
- **Template maintenance** — updating `CLAUDE.template.md` and migration guides
- **Dotfile/config changes** — nvim, tmux, shell, WezTerm configurations

The workflow skills (discuss, write-ticket, write-skeleton, write-plan,
implement, etc.) are **authored** here but **used** in downstream projects.
When editing skills, think about how they compose in the canonical flows:
- Full ceremony: `/discuss` → `/write-ticket` → `/write-skeleton` → (`/write-plan`) → `/implement`
- Direct: `/implement <description>`
- Delegation: `/delegated-implement` — implementer + reviewer cycle (planned)

## Commit Rules

Auto-create git commits, each covering one logical unit of change.
Include an **AI context** section in every commit message recording design decisions,
alternatives considered, and trade-offs — focus on _why_ this approach was chosen.

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>
```

**Language:** All AI-authored artifacts — commit messages, code comments,
documents — must be in English regardless of conversation language.
