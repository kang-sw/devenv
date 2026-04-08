# CLAUDE.md — devenv

## Project Memory

Read in this order at every session start, before any other action:

1. **Preamble** — read `ai-docs/_index.md`. Project-level truth that no
   session should re-derive. Prune aggressively: if derivable from code
   or commit history, delete.
2. **Local** — read `ai-docs/_index.local.md` if it exists. .gitignored.
   Machine-bound context (paths, env vars, build config) and personal
   session notes.
3. **Project arc** — run `git log --oneline --graph -50`. Trajectory and
   topic clusters at a glance.
4. **Recent history** — run `git log -10`. Decision rationale via AI Context
   sections. Fades as history grows.

## Response Discipline

- **Evidence before claims.** Run verification commands and read output before
  stating success. Never use "should pass", "probably works", or "looks correct."
- **No performative agreement.** Never respond with "Great point!", "You're
  absolutely right!", or similar. Restate the technical requirement, verify
  against the codebase, then act (or push back with reasoning).
- **Actions over words.** "Fixed. [what changed]" or just show the diff.
  Skip gratitude expressions and filler.

## Code Standards

Skill and agent documents follow `ai-docs/ref/skill-authoring.md`.

## Workflow

### Approval Protocol

- **Auto-proceed**: Typo fixes, formatting, dotfile tweaks, single-skill edits
  that follow existing patterns.
- **Ask first**: New skills/agents, cross-skill interface changes, template
  changes that affect downstream projects, convention changes.
- **Always ask**: Deleting skills/agents, changing canonical flows, modifying
  migration checklist semantics.

### Commit Rules

Auto-create git commits, each covering one logical unit of change.
Include an **AI context** section in every commit message recording design decisions,
alternatives considered, and trade-offs — focus on _why_ this approach was chosen.

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem>[: <optional-label>]
  > Forward: <what future phases must know>
```

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the current task. Update drifted docs on contact.

---

## Project Summary

**devenv** — Personal developer environment bootstrap and Claude Code workflow
authoring. Neovim (LazyVim), Claude Code skills/agents, tmux, WezTerm, shell
dotfiles. One-shot `install.sh` for macOS/WSL/Linux.

This repo is **not a software project** — it is a configuration and template
repository. Tickets here track skill design research, not software
implementation. The skills and agents defined here are consumed by other
projects via symlink (`install.sh` handles this).

## Workspace

```
ai-docs/   — Skill design research tickets (non-standard structure)
claude/    — Claude Code skills, agents, infra, CLAUDE.template.md
  skills/  — Skill definitions (discuss, write-ticket, write-plan, implement, etc.)
  agents/  — Native agent definitions (implementer, reviewer, planner, clerk, etc.)
  infra/   — Shared implementation references (impl-playbook, impl-process, ask.sh)
  infra/agents/ — Subagent dispatch rules (caller-injected)
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
- **Template maintenance** — updating `CLAUDE.template.md` and migration guides.
  When adding a checklist item that supersedes an earlier one, mark the old
  item `[obsoleted by vNNNN]`.
- **Dotfile/config changes** — nvim, tmux, shell, WezTerm configurations

The workflow skills (discuss, write-ticket, write-skeleton, write-plan,
implement, etc.) are **authored** here but **used** in downstream projects.
When editing skills, think about how they compose in the canonical flows:
- Full ceremony: `/discuss` → `/write-ticket` → `/write-skeleton` → (`/write-plan`) → `/implement`
- Direct: `/implement <description>`
- Delegation: `/delegated-implement` — implementer + reviewer cycle

## Project Knowledge

**Language:** All AI-authored artifacts — commit messages, code comments,
documents — must be in English regardless of conversation language.

<!-- Inclusion test: if breaking this rule makes a skill produce
     wrong results, it belongs here. Everything else goes in
     _index.md (context) or skills (process). -->

<!-- Template Version: v0011 -->
