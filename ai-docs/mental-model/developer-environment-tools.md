---
domain: developer-environment-tools
description: "Personal bootstrap, shell/editor config, tmux helpers, and Claude terminal TUIs."
sources:
  - install.sh
  - shell/
  - nvim/
  - tools/
related:
  claude-compatibility: "install.sh also refreshes the Claude plugin snapshot and marketplace registration."
---

# Developer Environment Tools

## Entry Points

- `install.sh` owns full/update bootstrap, package setup, symlinks, Claude plugin cache install, and local config generation. {#260505-developer-install-update-bootstrap}
- `shell/` owns tmux, WezTerm, statusline, and helper scripts. {#260505-developer-shell-environment}
- `nvim/` owns LazyVim plugin/config layers. {#260505-neovim-lazyvim-configuration}
- `tools/claude-watch` and `tools/claude-dash` are Rust TUIs for Claude sessions and worktrees. {#260505-claude-watch-session-viewer} {#260505-claude-dash-worktree-tui}

## Module Contracts

- `install.sh update` is the refresh path for dotfiles and the Claude plugin snapshot; package installation is mostly full-bootstrap territory. {#260505-developer-claude-plugin-config-install}
- `shell/scripts` is symlinked as `~/.devenv-scripts`; tmux config hardcodes that path for helper execution. {#260505-developer-dotfile-symlink-management}
- tmux Claude activity is daemon-driven: tmux reads `@claude-indicator`, while `tmux-claude-watcher.sh` writes it. Per-statusline polling defeats the batching contract. {#260505-tmux-claude-activity-watcher}
- Cross-window tmux/Neovim navigation is opt-in through `TMUX_ENABLE_PANE_NAVIGATION_OVER_WINDOW=1`. {#260505-tmux-helper-scripts}
- `claude-watch` and `claude-dash` both depend on Claude JSONL session shape; parser changes should check both crates.

## Coupling

- Starship WSL no-git behavior is split between installer-generated config and zsh runtime selection.
- `claude-dash` UI layout, mouse hit-testing, slot labels, and PTY forwarding have duplicated geometry assumptions.
- Named-agent panels in `claude-dash` read the ws-framework named-agent registry and Claude JSONL session files; Codex MCP agents are not visible there.
- `claude-dash` worktree tab reconciliation depends on git worktree discovery plus provisional tab markers.

## Extension Points & Change Recipes

- **Add a managed dotfile**: add a `link` source/destination pair in `install.sh`, then test update mode.
- **Add a tmux helper**: put it under `shell/scripts` or update all `~/.devenv-scripts` references.
- **Add Neovim external tools**: update Mason tool installer and any DAP/plugin path assumptions.
- **Add a claude-dash prefix command**: update event handling and the prefix help overlay together.

## Common Mistakes

- Editing installed dotfiles or plugin cache directly instead of repo sources.
- Adding tmux helpers outside `shell/scripts` and breaking hardcoded tmux paths.
- Changing `claude-watch` list layout without updating mouse hit-test math.
- Treating native Windows `claude-dash` behavior as verified; it remains planned. {#260505-claude-dash-windows-verification}

## Technical Debt

- `statusline.sh` assumes valid JSON and `jq`, then computes display widths manually around ANSI and glyphs.
- Some Neovim plugin specs are disabled scaffolds and should not be documented as active behavior.
