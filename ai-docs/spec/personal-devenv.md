---
title: Personal Dev Environment
summary: Machine bootstrapping system that installs tooling, configures the shell, symlinks dotfiles, and registers the ws plugin on the developer's machine.
---

# Personal Dev Environment

`install.sh` bootstraps the developer's machine in one command — installing tools via Homebrew, configuring Zsh, symlinking dotfiles, and registering the `ws` Claude Code plugin.

## Install System

### Full Install {#260421-full-install}

`./install.sh` runs nine phases in order:

1. **System prerequisites** — apt packages (`curl`, `git`, `build-essential`, SSL/zlib dev libs, clipboard tools) on Linux/WSL; Xcode CLT on macOS.
2. **Homebrew** — installs if absent; runs `brew update` if present.
3. **Zsh** — installs via Homebrew or apt; sets as default shell via `chsh`.
4. **Core CLI tools** — `fzf` (with key-binding install), `ripgrep`, `fd`, `bat`, `tmux`.
5. **Neovim toolchain** — `neovim`, `tree-sitter`, `lazygit`, `node`, `python3`.
6. **Quality-of-life extras** — `eza`, `zoxide`, `delta`, `starship`, `lf`, JetBrains Mono Nerd Font.
7. **Zsh plugins** — `zsh-autosuggestions`, `zsh-syntax-highlighting`, `zsh-history-substring-search`.
8. **`.zshrc` injection** — see [Shell Environment](#shell-environment).
9. **Symlinks + Claude Code config** — see [Dotfile Configs](#dotfile-configs) and [Claude Code Config](#claude-code-config).

### Update Mode {#260421-update-mode}

`./install.sh update` skips phases 1–7 (no package installs, no sudo). Runs phases 8–9 only: re-injects the `.zshrc` snippet and refreshes all symlinks. Safe to run from a terminal with no tty.

### Platform Support {#260421-platform-support}

Three platforms are detected at runtime: macOS, WSL (via `/proc/version`), and native Linux.

| Step | macOS | WSL | Linux |
|---|---|---|---|
| System prerequisites | Xcode CLT | apt packages | apt packages |
| Homebrew prefix | `/opt/homebrew` | `/home/linuxbrew` | `/home/linuxbrew` |
| Nerd Font | cask install | warning (install on host) | warning |
| Root / container | sudo no-op | sudo no-op | sudo no-op |

### Idempotency {#260421-install-idempotency}

Every install step is safe to re-run:

| Step | Guard |
|---|---|
| `brew install` | `brew list` check before each package |
| `.zshrc` injection | In-place marker replacement; surrounding user config preserved |
| Symlinks | Skip if already correct; back up real files to `<dst>.bak` |
| `settings.json` / `claude.json` | JSON merge; writes only when values differ |
| Plugin install | Checks `installed_plugins.json` before running `claude plugin install` |

## Installed Tools {#260421-installed-tools}

Tools installed via Homebrew during a full install:

| Category | Tools |
|---|---|
| Core CLI | `fzf`, `ripgrep`, `fd`, `bat`, `tmux` |
| Neovim toolchain | `neovim`, `tree-sitter`, `lazygit`, `node`, `python3` |
| Extras | `eza`, `zoxide`, `delta`, `starship`, `lf` |
| Font (macOS only) | JetBrains Mono Nerd Font |
| Zsh plugins | `zsh-autosuggestions`, `zsh-syntax-highlighting`, `zsh-history-substring-search` |

## Shell Environment

The `.zshrc` injection bounded by `# >>> dotfiles bootstrap >>>` markers configures the following behaviors.

### History {#260421-shell-history}

- 50,000-entry history file.
- Dedup and space-prefix suppression (`HIST_IGNORE_DUPS`, `HIST_IGNORE_SPACE`).
- `INC_APPEND_HISTORY`: each command writes immediately; arrow-key recall is session-local. `fzf Ctrl+R` still searches across all sessions via the shared history file.

### Completion and Key Bindings {#260421-shell-completion-keybindings}

- `compinit` with menu-select and case-insensitive matching.
- `Ctrl+Left`/`Ctrl+Right` word navigation; `Ctrl+Backspace`/`Ctrl+Delete` word deletion — wired end-to-end through WezTerm key remaps, tmux `terminal-features extkeys`, and zsh `bindkey`.

### Tool Initialization {#260421-shell-tool-init}

- `fzf` key bindings and completion sourced from `~/.fzf.zsh`.
- `bat` as man pager (`MANPAGER`); `BAT_THEME=Monokai Extended`.
- `zoxide` init.
- `delta` as `GIT_PAGER`.
- `starship` prompt init (loaded after syntax-highlighting).
- `zsh-autosuggestions` with `Ctrl+Space`/`Ctrl+F` accept bindings.
- `zsh-history-substring-search` on up/down arrows and vi-mode `k`/`j`.
- `zsh-syntax-highlighting` loaded last.

## tmux Scripts

Scripts symlinked to `~/.devenv-scripts/` and sourced by `~/.tmux.conf`.

### Claude Activity Indicator {#260421-tmux-claude-watcher}

`tmux-claude-watcher.sh` — background daemon tracking Claude Code activity per tmux window. Writes state to tmux window options (`#{@claude-indicator}`), read synchronously by the status bar.

States displayed:
- Moon spinner (phase-offset per agent) during active generation.
- `❌` during API retry.
- `✅` when a background-window session completes.

Multiple Claude agents in the same window each get their own spinner with a per-agent phase offset.

### Git Status {#260421-tmux-git-status}

`tmux-git-status.sh` — outputs branch name, ahead/behind counts, and staged/modified/untracked file counts for the tmux status bar. Exits immediately with `—` on WSL2 Windows mounts (`/mnt/[a-z]/`) to avoid NTFS latency.

### Cross-Window Navigation {#260421-tmux-cross-window}

`tmux-cross-window.sh` — `Ctrl+Arrow` pane navigation across tmux window boundaries. Opt-in via `TMUX_ENABLE_PANE_NAVIGATION_OVER_WINDOW=1`.

### fzf Command Selector {#260421-tmux-fzf-selector}

`tmux-fzf-command.sh` — fzf-powered picker for tmux commands.

### macOS Askpass {#260421-macos-askpass}

`macos-askpass.sh` — `osascript` password dialog for `SSH_ASKPASS` on macOS.

## Dotfile Configs

All configs are symlinked by `install.sh`.

### tmux {#260421-dotfile-tmux}

`~/.tmux.conf` — 3-row status bar: git status row (top), window tabs (middle), separator (bottom). Claude activity indicator via `#{@claude-indicator}` window options. `prefix+g` opens lazygit; shows a status-bar notice instead on WSL2 Windows mounts.

### WezTerm {#260421-dotfile-wezterm}

`~/.wezterm.lua` — key remaps for word navigation (feeds `extkeys` to tmux and zsh for end-to-end consistency).

### Starship {#260421-dotfile-starship}

`~/.config/starship.toml` — shell prompt configuration.

### lf {#260421-dotfile-lf}

`~/.config/lf/lfrc` — terminal file manager key bindings.

### Neovim {#260421-dotfile-nvim}

`~/.config/nvim` — LazyVim base with plugins for: blink completion, Claude Code integration, DAP, formatting, LSP (Mason), Neotest, noice, snacks, tmux-navigator, and Typst. Includes a custom `render-markdown` monkey-patch that scales table columns to window width using extmark-based overlays.

> [!note] Planned 🚧
> Make the Neovim config Windows-compatible. Current config targets macOS and Linux/WSL only.

## Claude Code Config {#260421-claude-code-env-setup}

`install.sh` merges Claude Code settings without overwriting unrelated user config:

- `~/.claude/settings.json` — agent team feature flags, `TeammateIdle` hook registration, `ws` marketplace and plugin entries.
- `~/.claude.json` — `teammateMode: "in-process"`.

Full behavioral spec for the registered plugin and hook: [Plugin Infrastructure](plugin-infra.md).

### Plugin Snapshot Isolation {#260428-plugin-snapshot-isolation}

`install.sh` copies `claude-plugin/` to `~/.claude/plugins/ws-plugin/claude/` via `rsync --delete` and registers that path as the plugin `installLocation`. Claude Code reads the plugin from the snapshot cache, not the live repo.

Live edits to `claude-plugin/` are not immediately visible to the running plugin. To propagate changes, re-run `./install.sh update` or run `claude plugin update ws@kang-sw-devenv`.
