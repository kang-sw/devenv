# devenv — Personal Developer Environment

A batteries-included developer workstation setup: Neovim (LazyVim), Claude Code AI, tmux, WezTerm, and a full shell environment. One-shot bootstrap script for macOS, WSL, and Linux.

---

## Features at a Glance

- **LazyVim distro** with curated extras for Rust, C/C++, Python, Markdown, and Typst
- **Claude Code** integration (official plugin + custom skills and agents)
- **tmux** — vim-aware pane navigation, cross-window jumping, copy-mode keybindings
- **WezTerm** config with tmux-style bindings and IME auto-switching
- **VSCode Dark+** colorscheme, heavily tuned with semantic token highlights
- **Debugger (DAP)** for Rust/C/C++ (codelldb) and Python (debugpy)
- **macOS bilingual (Korean/English) IME** auto-switching on mode change
- **One-shot install script** that sets up the entire environment from scratch

---

## Quick Start

```sh
git clone https://github.com/kang-sw/devenv.git ~/devenv
cd ~/devenv
bash install.sh
```

The install script is idempotent — safe to re-run after updates. It creates
symlinks for `~/.config/nvim`, shell dotfiles, scripts, and Claude Code
skills/agents.

> **Note:** Neovim 0.10+ is required. The script will not install Neovim itself; install it via your package manager or from [neovim.io](https://neovim.io) first.

---

## What `install.sh` Does

A ~375-line bootstrap script that handles everything from system packages to dotfile symlinks. It detects the current platform (macOS / WSL / native Linux) and adapts accordingly.

| Phase | What happens |
|---|---|
| System prerequisites | Xcode CLT on macOS, apt updates on Linux |
| Homebrew | Installs Homebrew if missing; adds it to PATH |
| Core CLI tools | fzf, ripgrep, fd, bat, tmux, tree-sitter, lazygit, node, python3 |
| Quality-of-life extras | eza, zoxide, delta, starship, Nerd Fonts (macOS only) |
| Zsh plugins | zsh-autosuggestions, zsh-syntax-highlighting, zsh-history-substring-search |
| `.zshrc` setup | Injects config block (idempotent, marker-guarded) |
| Dotfile symlinks | `~/.config/nvim`, `~/.tmux.conf`, `~/.wezterm.lua`, `~/.vscode-neovim.lua`, `~/.config/starship.toml`, `~/.devenv-scripts` |
| Claude Code skills | Symlinks each skill folder into `~/.claude/skills/` |
| Claude Code agents | Symlinks each agent file into `~/.claude/agents/` |
| Dead link cleanup | Removes stale symlinks in `~/.claude/skills/` and `~/.claude/agents/` |

---

## Neovim Plugin Overview

### Plugin Manager

**Lazy.nvim** via the **LazyVim** distro. `init.lua` is two lines — LazyVim handles all bootstrap and sensible defaults.

### Language Support

| Language | LSP | Formatter | Debugger | Test runner |
|---|---|---|---|---|
| Rust | rust-analyzer | rustfmt | codelldb | neotest-rust |
| C / C++ | clangd | clang-format | codelldb | — |
| Python | pyright | black | debugpy | — |
| Markdown | marksman | prettier | — | — |
| Typst | tinymist | — | — | — |

### Key Plugins

| Plugin | Purpose |
|---|---|
| **blink.cmp** | Async completion engine (replaces nvim-cmp) |
| **conform.nvim** | Code formatting (Prettier, Taplo, clang-format) |
| **trouble.nvim** | Floating diagnostic panel |
| **snacks.nvim** | File picker, explorer, terminal |
| **neotest** | Test runner UI (Rust) |
| **nvim-dap** | Debug Adapter Protocol client |
| **render-markdown.nvim** | In-buffer Markdown rendering |
| **bullets.vim** | Auto-renumber Markdown lists |
| **vim-tmux-navigator** | Seamless vim ↔ tmux pane navigation |
| **focus.nvim** | Pane layout management |
| **incline.nvim** | Breadcrumb filename in splits |
| **noice.nvim** | LSP UI polish |
| **claudecode.nvim** | Claude Code terminal (100-col vertical split) |

---

## Keybinding Highlights

### Editor

| Key | Action |
|---|---|
| `<Tab>` | Accept completion |
| `F1` | Format buffer |
| `<leader>"` | Horizontal split |
| `<leader>%` | Vertical split |
| `<C-\><C-\>` | Exit terminal mode |
| `` <C-`> `` | Toggle floating terminal |
| `<leader>zf` | Fold all functions (treesitter-aware) |

### LSP

| Key | Action |
|---|---|
| `gh` | Hover documentation |
| `gy` | Go to type definition |
| `<leader>ca` | Code action |

### Testing (Rust)

| Key | Action |
|---|---|
| `<leader>tt` | Run nearest test |
| `<leader>tf` | Run file tests |
| `<leader>ts` | Test summary panel |
| `<leader>to` | Test output |
| `<leader>tw` | Watch mode |

### macOS-style Insert Mode

| Key | Action |
|---|---|
| `M-b` / `M-f` | Word backward / forward |
| `M-BS` | Delete word backward |
| `C-a` / `C-e` | Line start / end |

### Pane Navigation (vim + tmux)

| Key | Action |
|---|---|
| `C-M-h/j/k/l` | Move across vim splits and tmux panes |

---

## tmux Configuration

`.tmux.conf` provides a vim-centric tmux setup:

- **Heavy border style** (┃ ━ ┏ ┓) with active pane highlighted in lime (`#aaff00`)
- **Copy mode** turns the border red; full vi-style bindings (v/V/C-v, y, /, ?)
- **Clipboard** works on macOS (`pbcopy`), WSL (`clip.exe`), and Linux (`xclip`/`xsel`)
- **Cross-window pane jumping** via `scripts/tmux-cross-window.sh`
- **Pane border status** shows: pane index, running command, dimensions, current path
- **Prefix indicator** in the status bar turns dark red while prefix key is held
- **Quick pane select**: F1–F8

Key bindings:

| Key | Action |
|---|---|
| `PREFIX \|` | Vertical split (current path) |
| `PREFIX -` | Horizontal split (current path) |
| `PREFIX n/p` | Next / previous window |
| `PREFIX {/}` | Window wrap-around |
| `PREFIX F1–F8` | Jump to pane by index |

---

## Shell Environment

After running `install.sh`, the shell gets:

- **Starship** prompt with Nerd Font icons, git status, command duration
- **eza** aliases: `ls`, `ll` (long + git info), `lt` (tree depth 2), `la`
- **zoxide** for smart directory jumping (`z foo`)
- **delta** as the git pager with syntax highlighting
- **bat** with Monokai Extended theme; replaces `man` pages
- **fzf** (40% height, multiselect, preview toggle)
- **Ctrl+Space** to accept autosuggestion; Up/Down for history substring search

---

## Claude Code Integration

### Plugin

[claudecode.nvim](https://github.com/coder/claudecode.nvim) opens a 100-column vertical terminal. Diffs open in a vertical split while keeping terminal focus.

### Skills (`~/.claude/skills/`)

Custom skills symlinked from `claude/skills/`:

| Skill | Purpose |
|---|---|
| `implement` | Structured feature implementation with task tracking |
| `bugfix` | Root-cause analysis and fix workflow |
| `discuss` | Design discussion with codebase context |
| `document-dependency` | Generate API delta docs for dependencies |
| `rebuild-mental-model` | Rebuild `ai-docs/mental-model/` for a project |

### Agents (`~/.claude/agents/`)

Specialized subagents for AI-assisted documentation tasks (mental model verification, dependency API drift detection).

### Project Template

`claude/CLAUDE.template.md` is a copyable template for per-project `CLAUDE.md` files, covering: project summary, tech stack, workspace layout, architecture rules, code standards, and workflow conventions.

---

## WezTerm

`.wezterm.lua` provides:

- **JetBrainsMono Nerd Font** with CJK fallbacks
- tmux-style keybindings with CMD/ALT/SUPER as PREFIX
- IME auto-switching on Neovim insert mode change
- 95% background opacity, clean tab bar

---

## VSCode Neovim

`.vscode-neovim.lua` adds vim motions inside VSCode via the Neovim extension:

- Fold commands (`zc/zo/za/zR/zM`) mapped to VSCode native actions
- `c/s/x` in visual mode route to blackhole register (no clipboard clobber)
- Same IME switching logic as native Neovim

---

## Colorscheme

VSCode Dark+ theme (`tokyodark` base) with ~190 lines of custom overrides:

- Semantic token highlights for Rust, C++, Python
- TextMate scope alignment (keywords, functions, types, modifiers)
- Custom Rust modifier combinations: `static`, `trait`, `constant`, `lifetime`
- Markdown heading backgrounds for `render-markdown.nvim`

---

## Directory Layout

```
~/devenv/
├── install.sh             # Full environment setup script
├── nvim/                  # → ~/.config/nvim
│   ├── init.lua           # Bootstraps lazy.nvim (2 lines)
│   ├── lua/
│   │   ├── config/        # keymaps, options, autocmds, lazy
│   │   └── plugins/       # Per-plugin configuration (~23 files)
│   ├── lazy-lock.json
│   └── lazyvim.json
├── claude/
│   ├── skills/            # → ~/.claude/skills/* (per-dir symlinks)
│   ├── agents/            # → ~/.claude/agents/*.md (per-file symlinks)
│   └── CLAUDE.template.md # Template for per-project CLAUDE.md
├── shell/
│   ├── scripts/           # → ~/.devenv-scripts
│   ├── .tmux.conf         # → ~/.tmux.conf
│   ├── .wezterm.lua       # → ~/.wezterm.lua
│   ├── .vscode-neovim.lua # → ~/.vscode-neovim.lua
│   ├── starship.toml      # → ~/.config/starship.toml
│   ├── statusline.sh      # Claude Code status display for tmux
│   └── lfrc               # → ~/.config/lf/lfrc
```

---

## Requirements

| Tool | Notes |
|---|---|
| Neovim 0.10+ | Not installed by `install.sh` |
| Git | Required before cloning |
| Homebrew | Auto-installed on macOS; used on Linux/WSL too |
| Node.js | Auto-installed; needed by several LSPs |
| [im-select](https://github.com/daipeihust/im-select) | macOS only; needed for IME auto-switching |

---

## License

Personal configuration — use freely, no warranty implied.
