# CLAUDE.md — devenv

## Project Summary

**devenv** — Personal developer environment bootstrap. Neovim (LazyVim), Claude Code skills/agents, tmux, WezTerm, shell dotfiles. One-shot `install.sh` for macOS/WSL/Linux.

## Workspace

```
nvim/      — Neovim config (LazyVim, plugins, colorscheme)
claude/    — Claude Code skills, agents, CLAUDE.template.md
shell/     — tmux, WezTerm, starship, zsh dotfiles, helper scripts
install.sh — Idempotent bootstrap (packages, symlinks, cleanup)
```

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
