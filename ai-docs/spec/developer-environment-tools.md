---
title: Developer Environment Tools
summary: Personal development bootstrap, shell and terminal configuration, tmux helpers, Neovim setup, Claude statusline, and local Claude TUI tools.
---

# Developer Environment Tools

The developer environment tools configure a personal ws-oriented workstation:
package bootstrap, shell and editor dotfiles, tmux status helpers, Claude
statusline integration, and local Rust TUIs for inspecting or multiplexing
Claude sessions.

## Install And Update Bootstrap {#260505-developer-install-update-bootstrap}

`install.sh` supports a full bootstrap mode and an `update` mode. Full bootstrap
installs platform prerequisites, package manager dependencies, shell tooling,
editor dependencies, fonts, zsh plugins, dotfiles, Claude configuration, and
local plugin artifacts.

`./install.sh update` skips package installation phases and reruns configuration
and symlink phases so local dotfiles and plugin snapshots can be refreshed
without repeating privileged setup.

## Platform Handling {#260505-developer-platform-handling}

The installer detects macOS, WSL, and native Linux. macOS setup uses Xcode
Command Line Tools, Homebrew, and cask font installation. WSL and Linux setup
use apt prerequisites and Linuxbrew; WSL/Linux users receive host-font guidance
when fonts must be installed outside the guest environment.

The shell and terminal configuration includes platform branches for macOS,
Windows/WSL, and Linux where key bindings, paths, clipboard helpers, and
terminal integration differ.

## Dotfile And Symlink Management {#260505-developer-dotfile-symlink-management}

The installer manages dotfiles idempotently. It replaces or appends marked
blocks in `.zshrc`, skips already-correct symlinks, relinks wrong symlinks, and
backs up real files before replacing them.

Managed links include tmux, WezTerm, Starship, lf, scripts, Neovim, VSCode
Neovim settings, and Claude home instructions.

## Shell Environment {#260505-developer-shell-environment}

The generated zsh configuration sets history behavior, completion, word
navigation and deletion keys, fzf integration, bat as the man pager, zoxide,
delta, zsh autosuggestions, history substring search, syntax highlighting, and
Starship.

For WSL paths under `/mnt/<drive>/`, the shell selects a Starship no-git
configuration to avoid slow prompt-time Git checks on mounted Windows
filesystems.

## tmux Claude Activity Watcher {#260505-tmux-claude-activity-watcher}

tmux configuration starts a single background Claude watcher daemon. The
watcher scans panes, detects Claude prompt, spinner, retry, completion, and
content-change states, and writes batched tmux option updates for statusline
display.

The tmux statusline reads the watcher state to show animated Claude activity
without running expensive polling commands inside the statusline itself.

## tmux Helper Scripts {#260505-tmux-helper-scripts}

tmux helper scripts provide Git status, cross-window pane navigation, and
tmux-fzf command selection.

The Git status helper reports branch, ahead/behind counts, and dirty-file
counts, while skipping WSL Windows mounts. Cross-window pane navigation is
opt-in through `TMUX_ENABLE_PANE_NAVIGATION_OVER_WINDOW=1`. The fzf helper wraps
tmux command selection in a popup-friendly script.

## WezTerm Terminal Behavior {#260505-wezterm-terminal-behavior}

The WezTerm configuration adapts to macOS, Windows, and Linux. It defines
platform-specific modifier behavior, tmux-like prefix tables, copy mode, word
movement and deletion, tab titles, popups, IME switching hooks where available,
and a visible prefix status indicator.

## Neovim LazyVim Configuration {#260505-neovim-lazyvim-configuration}

The Neovim configuration builds on LazyVim and enables development extras for
languages and workflows used in the environment, including Rust, C/C++, Python,
debugging, Markdown, Typst, testing, and Claude Code integration.

Custom configuration adds tmux-aware navigation, terminal toggles, formatting
keymaps, macOS input-method switching, Typst preview behavior, DAP, neotest,
Mason-managed tools, render-markdown customization, and UI/plugin overrides.

## Claude Statusline Script {#260505-claude-statusline-script}

`shell/statusline.sh` reads Claude Code status JSON and renders a compact
statusline. It reports model, cost, context usage, output tokens, elapsed
duration, rate-limit timing, cache hit state, Git status, and line delta
information.

## Developer Claude Plugin Config Install {#260505-developer-claude-plugin-config-install}

The installer integrates Claude configuration for this development environment.
It links Claude home instructions, merges Claude settings, registers hooks and
environment values, configures teammate mode, snapshots the Claude plugin into
the local Claude plugin cache, writes marketplace metadata, and installs the
local ws plugin when the Claude CLI is available.

## Claude Watch Session Viewer {#260505-claude-watch-session-viewer}

`tools/claude-watch` is a Rust TUI for inspecting Claude session history and
active Claude subprocesses. It discovers Claude project session directories,
handles git worktree project directories, scans all project dirs on Windows,
parses JSONL session files, and renders selectable session/message panels.

The viewer supports keyboard and mouse interaction, scrolling, click selection,
active-process highlighting, and live subprocess detection based on Claude
session/resume arguments.

## Claude Dash Worktree TUI {#260505-claude-dash-worktree-tui}

`tools/claude-dash` is a Rust TUI for running and switching between Claude
sessions across Git worktrees. It spawns Claude in PTYs, tracks worktrees,
supports prefix commands for tabs/worktrees/slots, shows named-agent JSONL
slots, and owns subprocess lifecycle controls such as restart and close flows.

The dashboard polls Git worktrees, exposes provisional worktree tabs, renders
interactive terminal output through a virtual terminal screen, and supports
mouse/key navigation for its dashboard regions.

## 🚧 Claude Dash Windows Verification {#260505-claude-dash-windows-verification}

`claude-dash` is intended to run natively on Windows as an executable that
spawns `claude.exe`, reads Windows worktree paths, handles session directory
escaping, and works with Windows Terminal mouse events.

Native Windows build and runtime verification remains planned until the active
Windows verification ticket confirms startup, PTY subprocess behavior, path
escaping, worktree parsing, mouse handling, and any fixes found during that
verification.
