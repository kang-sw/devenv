# devenv — Personal Developer Environment

A batteries-included developer workstation setup. One-shot bootstrap script for macOS, WSL, and Linux.

## Quick Start

```sh
git clone https://github.com/kang-sw/devenv.git ~/devenv
cd ~/devenv
bash install.sh          # idempotent — safe to re-run
bash install.sh --update # skip packages & sudo, refresh symlinks only
```

> Neovim 0.10+ is required but not installed by the script.

## What's Inside

```
nvim/     Neovim config (LazyVim distro, language support, debugger, etc.)
claude-plugin/   Claude Code skills, agents, and per-project CLAUDE.md template
shell/    tmux, WezTerm, starship, zsh dotfiles, helper scripts
```

- **Neovim** — LazyVim-based config with LSP, formatter, DAP, and test runner support for Rust, C/C++, Python, Markdown, Typst. VSCode Dark+ colorscheme with semantic token overrides.
- **Claude Code** — Custom skills (implement, discuss, write-plan, write-ticket, etc.) and subagents (mental-model, dependency docs, etc.) symlinked into `~/.claude/`. Includes a `CLAUDE.template.md` for bootstrapping per-project AI context.
- **tmux** — Vim-aware pane navigation, cross-window jumping, vi copy-mode, platform-aware clipboard.
- **WezTerm** — JetBrainsMono Nerd Font, tmux-style keybindings, IME auto-switching.
- **Shell** — Starship prompt, eza, zoxide, delta, bat, fzf, zsh plugins.

## install.sh

Detects the platform and handles: Homebrew, CLI tools, zsh plugins, dotfile symlinks, and Claude Code skill/agent symlinks. Stale symlinks are cleaned up automatically.

## Claude Code Plugin (`ws`)

The `claude-plugin/` directory is packaged as the `ws` Claude Code plugin. Install it on any machine:

```sh
claude plugin marketplace add kang-sw/devenv
claude plugin install ws@kang-sw-devenv
```

After `install.sh` runs on the home machine, re-run the above if the plugin is not already present.

## License

Personal configuration — use freely, no warranty implied.
