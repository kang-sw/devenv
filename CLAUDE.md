# CLAUDE.md — dotfiles

## Project Summary

Personal development environment: Neovim (LazyVim), tmux, WezTerm, Zsh, and
shell tooling, maintained as a single git repo. Primary target is macOS; partial
compatibility with WSL/Linux. `install.sh` bootstraps a fresh machine by
symlinking dotfiles and installing dependencies via Homebrew.

## Tech Stack

Lua + LazyVim (Neovim config). tmux config, WezTerm Lua, Bash scripts,
Starship TOML, Zsh (plugins via Homebrew — no Oh My Zsh).

## Workspace

```
lua/config/       — Neovim core: options, keymaps, autocmds
lua/plugins/      — Plugin specs (one concern per file)
scripts/          — Shell scripts shared by tmux and Neovim keymaps
claude-skills/    — Custom Claude Code skill definitions  → symlinked to ~/.claude/skills/
claude-agents/    — Custom Claude Code agent definitions → symlinked to ~/.claude/agents
.tmux.conf        — tmux config (prefix=C-b, vi-mode, tmux-fzf) → symlinked to ~/.tmux.conf
.wezterm.lua      — WezTerm config → symlinked to ~/.wezterm.lua
starship.toml     — Shell prompt → symlinked to ~/.config/starship.toml
install.sh        — Bootstrap: Homebrew deps, symlinks, zsh snippet injection
```

`~/.config/nvim` is the repo itself — no symlink needed for Neovim.

## Architecture Rules

1. **LazyVim as base.** Do not override LazyVim defaults without a specific
   reason. Prefer spec-level overrides (`opts`, `keys`, `dependencies`) over
   rewriting plugin configs wholesale.
2. **One concern per plugin file.** Each file in `lua/plugins/` covers one
   plugin or one tightly related group. Do not consolidate unrelated plugins.
3. **tmux owns multiplexing.** Pane/window/session management lives in
   `.tmux.conf`. WezTerm handles font, colors, and OS integration only. Do not
   duplicate multiplexer logic between the two.
4. **Stable script paths.** Files in `scripts/` are referenced by both
   `.tmux.conf` and Neovim keymaps. Renaming requires updating all callers
   simultaneously.
5. **Skills and agents are code.** `claude-skills/` and `claude-agents/` are
   first-class source — edit with the same care as Lua code.
6. **Never edit plugin sources directly.** Customizations go in the repo
   (wrapper scripts, spec-level overrides). Plugin updates must not overwrite
   local patches.

---

## Project Knowledge

No `ai-docs/` directory. For a config repo of this size the `# MEMORY` section
below is the cross-session context store — update it after non-trivial sessions.

## Code Standards

1. **Simplicity.** Write the simplest code that works. Implement fully when the
   spec is clear — judge scope by AI effort, not human-hours.
2. **Surgical changes.** Change only what the task requires. Follow existing
   style. Every changed line must trace to the request.
3. **Module structure.** Split files at ~300 lines. Extract an entry file
   (e.g. `mod.rs`, `index.ts`, `__init__.py`) containing doc comments and
   public re-exports only — reading it alone conveys the module's interface.
4. **Hot-path performance.** In performance-critical paths, prefer minimal
   allocation and data locality over convenience abstractions. Apply only when
   benefit clearly outweighs complexity cost.

## Workflow

### Approval Protocol

- **Auto-proceed**: Bug fixes, pattern-following additions, boilerplate,
  refactoring within a single module.
- **Ask first**: New component/protocol additions, architectural changes,
  cross-module interface changes, anything that changes observable behavior.
- **Always ask**: Deleting existing functionality, changing protocol/API
  semantics, modifying symlink targets in `install.sh`.

### Commit Rules

Auto-create git commits, each covering one logical unit of change.
Include an **AI context** section in every commit message recording design
decisions, alternatives considered, and trade-offs — focus on _why_ this
approach was chosen.

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>
```

### Session Start

- Run `git log --oneline -10` for recent changes.

### Dependency API Notes

- **`ai-docs/deps/<package>[v<ver>].md`** stores verified API facts for
  libraries whose actual API differs from training knowledge or is too recent.
- **When to read:** Before writing code that uses a package listed in
  `# MEMORY → Documented Dependencies`.
- **When to write/update:** After discovering API drift (wrong arg count,
  renamed types, removed methods).

### Response Discipline

- **Evidence before claims.** Run verification commands and read output before
  stating success. Never use "should pass", "probably works", or "looks
  correct."
- **No performative agreement.** Never respond with "Great point!", "You're
  absolutely right!", or similar. Restate the technical requirement, verify
  against the codebase, then act (or push back with reasoning).
- **Actions over words.** "Fixed. [what changed]" or just show the diff.
  Skip gratitude expressions and filler.

### Context Window Discipline

- Keep context small. Load only the module docs relevant to the current task.
- Source code is the ground truth; docs supplement it.
- When a module doc drifts from source, update the doc (or flag it).

---

# MEMORY

<!-- AI-maintained. Update after each non-trivial session. Prune aggressively. -->

## Build & Workflow

- No build step.
- Neovim: `:Lazy sync` to update plugins; `lazy-lock.json` tracks lockfile —
  commit after intentional updates.
- tmux: `tmux source ~/.config/nvim/.tmux.conf` to reload.
- WezTerm: reloads automatically on file save.
- Shell: `exec zsh` or restart terminal after `.zshrc` changes.
- Fresh machine: `bash install.sh` from repo root.

## Recent Work

- tmux `prefix+:` fzf command picker: custom wrapper `scripts/tmux-fzf-command.sh`
  adds trailing space after selection so cursor isn't glued to command name.
  Plugin source is untouched; binding in `.tmux.conf` points to the wrapper.
- tmux copy mode mouse ergonomics: `MouseDown1Pane → clear-selection` (click
  deselects), `MouseDragEnd1Pane → stop-selection` (drag-end fixes selection
  endpoint so wheel scroll doesn't extend it).

## Workspace Reference

- Neovim entry: `init.lua` → `lua/config/lazy.lua`
- tmux config: `.tmux.conf` (symlinked to `~/.tmux.conf` by `install.sh`)
- Key scripts:
  - `scripts/tmux-cross-window.sh` — vim-tmux boundary-crossing window navigation
  - `scripts/tmux-fzf-command.sh` — fzf command picker wrapper (trailing space fix)
- Skills: `claude-skills/` → `~/.claude/skills/` (each subdirectory symlinked)
- Agents: `claude-agents/` → `~/.claude/agents/` (directory symlinked)

## Documented Dependencies

-
