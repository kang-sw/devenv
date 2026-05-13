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
agents-plugin/   ws plugin package for Codex and Claude-compatible plugin installs
agents-plugin-wsflow/   agentless wsflow plugin package for Codex and Claude-compatible installs
agents-plugin-tool/   native ws MCP runtime and tooling source
shell/    tmux, WezTerm, starship, zsh dotfiles, helper scripts
```

- **Neovim** — LazyVim-based config with LSP, formatter, DAP, and test runner support for Rust, C/C++, Python, Markdown, Typst. VSCode Dark+ colorscheme with semantic token overrides.
- **ws plugin** — Codex-first workflow skills plus the native `ws-mcp` runtime. Claude Code compatibility uses the `agents-plugin/` package metadata, not a separate legacy source tree.
- **wsflow plugin** — Agentless workflow skills that reuse the shared runtime without ws managed-agent orchestration.
- **tmux** — Vim-aware pane navigation, cross-window jumping, vi copy-mode, platform-aware clipboard.
- **WezTerm** — JetBrainsMono Nerd Font, tmux-style keybindings, IME auto-switching.
- **Shell** — Starship prompt, eza, zoxide, delta, bat, fzf, zsh plugins.

## install.sh

Detects the platform and handles: Homebrew, CLI tools, zsh plugins, dotfile symlinks, and local ws plugin cache setup. Stale symlinks are cleaned up automatically.

## Workflow Plugins

The active ws plugin package lives in `agents-plugin/`. The agentless wsflow
package lives in `agents-plugin-wsflow/`. Local bootstrap registers
Claude-compatible snapshots for both packages when Claude Code is available:

```sh
claude plugin install ws@kang-sw-devenv
claude plugin install wsflow@kang-sw-devenv
```

Codex installs use the repository marketplace entries under `.agents/plugins/`.

## License

Personal configuration — use freely, no warranty implied.
