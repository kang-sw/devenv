#!/usr/bin/env bash
# install.sh — dotfiles + dev environment bootstrap
# Supports: macOS, WSL (Ubuntu/Debian), native Linux
#
# Usage:
#   ./install.sh          Full bootstrap (packages + config + symlinks)
#   ./install.sh update   Config + symlinks only (no packages, no sudo)
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODE="${1:-full}"

# ══════════════════════════════════════════════════════════════════════════════
# Helpers
# ══════════════════════════════════════════════════════════════════════════════

info() { printf "\033[1;34m  ➜ %s\033[0m\n" "$*"; }
success() { printf "\033[1;32m  ✔ %s\033[0m\n" "$*"; }
muted() { printf "\033[90m  ✔ %s\033[0m\n" "$*"; }
warn() { printf "\033[1;33m  ⚠ %s\033[0m\n" "$*"; }
die() {
  printf "\033[1;31m  ✘ %s\033[0m\n" "$*" >&2
  exit 1
}
has() { command -v "$1" &>/dev/null; }

# ══════════════════════════════════════════════════════════════════════════════
# Platform detection
# ══════════════════════════════════════════════════════════════════════════════

PLATFORM="linux"
if [[ "$(uname)" == "Darwin" ]]; then
  PLATFORM="macos"
elif grep -qi microsoft /proc/version 2>/dev/null; then
  PLATFORM="wsl"
fi

info "Platform: $PLATFORM  Mode: $MODE"

# ══════════════════════════════════════════════════════════════════════════════
# Phases 1-7: Package installation (skipped in update mode)
# ══════════════════════════════════════════════════════════════════════════════

if [[ "$MODE" == "update" ]]; then
  info "Update mode — skipping package installation"
else

  # Use sudo only when not already root
  if [[ $EUID -eq 0 ]]; then
    SUDO=""
  else
    SUDO="sudo"
  fi

  # ══════════════════════════════════════════════════════════════════════════════
  # 1. System prerequisites
  # ══════════════════════════════════════════════════════════════════════════════

  if [[ "$PLATFORM" == "wsl" || "$PLATFORM" == "linux" ]]; then
    info "Updating apt and installing prerequisites..."
    $SUDO apt-get update -qq
    $SUDO apt-get install -y -qq \
      curl git unzip wget ca-certificates gnupg \
      build-essential gcc g++ make cmake pkg-config \
      libssl-dev zlib1g-dev \
      xclip xsel
    success "apt prerequisites installed"
  fi

  if [[ "$PLATFORM" == "macos" ]]; then
    if ! xcode-select -p &>/dev/null; then
      info "Installing Xcode Command Line Tools..."
      xcode-select --install || true
      until xcode-select -p &>/dev/null; do sleep 5; done
      success "Xcode CLT installed"
    else
      muted "Xcode CLT already present"
    fi
  fi

  # ══════════════════════════════════════════════════════════════════════════════
  # 2. Homebrew
  # ══════════════════════════════════════════════════════════════════════════════

  if ! has brew; then
    info "Installing Homebrew..."
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    if [[ "$PLATFORM" == "macos" ]]; then
      eval "$(/opt/homebrew/bin/brew shellenv 2>/dev/null || /usr/local/bin/brew shellenv)"
    else
      eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
    fi
    success "Homebrew installed"
  else
    muted "Homebrew already present"
    brew update --quiet
  fi

  # Ensure brew is on PATH for the rest of this script
  if [[ "$PLATFORM" == "macos" ]]; then
    eval "$(/opt/homebrew/bin/brew shellenv 2>/dev/null || /usr/local/bin/brew shellenv 2>/dev/null || true)"
  else
    eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv 2>/dev/null || true)"
  fi

  brew_install() {
    local pkg="$1"
    if brew list --formula "$pkg" &>/dev/null; then
      muted "$pkg already installed (brew)"
    else
      info "brew install $pkg"
      brew install "$pkg"
      success "$pkg installed"
    fi
  }

  # ══════════════════════════════════════════════════════════════════════════════
  # 3. Zsh (no Oh My Zsh — plugins installed via brew instead)
  # ══════════════════════════════════════════════════════════════════════════════

  if ! has zsh; then
    info "Installing zsh..."
    if [[ "$PLATFORM" == "macos" ]]; then
      brew_install zsh
    else
      $SUDO apt-get install -y -qq zsh
    fi
  fi

  ZSH_BIN="$(command -v zsh)"

  if [[ "$SHELL" != "$ZSH_BIN" ]]; then
    info "Setting zsh as default shell ($ZSH_BIN)..."
    if ! grep -qF "$ZSH_BIN" /etc/shells; then
      echo "$ZSH_BIN" | $SUDO tee -a /etc/shells
    fi
    chsh -s "$ZSH_BIN" 2>/dev/null || warn "chsh failed — run manually: chsh -s $ZSH_BIN"
  fi
  success "zsh: $ZSH_BIN"

  # ══════════════════════════════════════════════════════════════════════════════
  # 4. Core CLI tools
  # ══════════════════════════════════════════════════════════════════════════════

  info "Installing core CLI tools..."

  brew_install fzf
  brew_install ripgrep
  brew_install fd
  brew_install bat
  brew_install tmux

  # fzf shell integration (~/.fzf.zsh)
  FZF_PREFIX="$(brew --prefix fzf 2>/dev/null || echo "")"
  if [[ -n "$FZF_PREFIX" && -f "$FZF_PREFIX/install" ]]; then
    "$FZF_PREFIX/install" --key-bindings --completion --no-update-rc --no-bash --no-fish 2>/dev/null || true
  fi

  # ══════════════════════════════════════════════════════════════════════════════
  # 5. Neovim + LazyVim dependencies
  # ══════════════════════════════════════════════════════════════════════════════

  info "Installing Neovim and LazyVim dependencies..."

  brew_install neovim
  brew_install tree-sitter
  brew_install lazygit
  brew_install node
  brew_install python3

  # ══════════════════════════════════════════════════════════════════════════════
  # 6. Quality-of-life extras
  # ══════════════════════════════════════════════════════════════════════════════

  info "Installing extras..."

  brew_install eza
  brew_install zoxide
  brew_install delta
  brew_install starship
  brew_install lf

  # Nerd Font (macOS cask only; on Linux/WSL install on the host terminal side)
  if [[ "$PLATFORM" == "macos" ]]; then
    if ! brew list --cask font-jetbrains-mono-nerd-font &>/dev/null; then
      info "Installing JetBrainsMono Nerd Font..."
      brew install --cask font-jetbrains-mono-nerd-font
      success "JetBrainsMono Nerd Font installed"
    else
      muted "JetBrainsMono Nerd Font already installed"
    fi
  else
    warn "Linux/WSL: install a Nerd Font on the host/Windows terminal side for icon rendering."
  fi

  # ══════════════════════════════════════════════════════════════════════════════
  # 7. Zsh plugins (via brew — replaces Oh My Zsh)
  # ══════════════════════════════════════════════════════════════════════════════

  info "Installing zsh plugins..."

  brew_install zsh-autosuggestions
  brew_install zsh-syntax-highlighting
  brew_install zsh-history-substring-search

fi # end of: if [[ "$MODE" != "update" ]]

# ══════════════════════════════════════════════════════════════════════════════
# 8. Zsh config (~/.zshrc)  — 항상 최신 내용으로 덮어씀 (위치 보존)
# ══════════════════════════════════════════════════════════════════════════════

ZSHRC="$HOME/.zshrc"
ZSH_START_MARKER="# >>> dotfiles bootstrap >>>"
ZSH_END_MARKER="# <<< dotfiles bootstrap <<<"

[[ ! -f "$ZSHRC" ]] && touch "$ZSHRC"

# 새 스니펫을 임시 파일에 작성
_SNIPPET_TMP=$(mktemp)
cat >"$_SNIPPET_TMP" <<'EOF'
# >>> dotfiles bootstrap >>>

# ── brew (Linux / WSL) ───────────────────────────────────────────────────────
if [[ -f /home/linuxbrew/.linuxbrew/bin/brew ]]; then
    eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
fi

# ── History ──────────────────────────────────────────────────────────────────
HISTFILE=~/.zsh_history
HISTSIZE=50000
SAVEHIST=50000
setopt HIST_IGNORE_DUPS       # 중복 기록 안 함
setopt HIST_IGNORE_SPACE      # 앞에 스페이스 붙이면 기록 안 함
setopt HIST_REDUCE_BLANKS     # 불필요한 공백 제거
setopt INC_APPEND_HISTORY     # 즉시 파일에 append, 방향키는 로컬 세션 순서 유지

# ── Zsh options ──────────────────────────────────────────────────────────────
setopt AUTO_CD                # 디렉토리 이름만 입력해도 cd
setopt CORRECT                # 명령어 오타 교정 제안
setopt NO_BEEP                # 경고음 없음

# ── Completion ───────────────────────────────────────────────────────────────
autoload -Uz compinit && compinit
zstyle ':completion:*' menu select
zstyle ':completion:*' matcher-list 'm:{a-z}={A-Z}'
zstyle ':completion:*' list-colors "${(s.:.)LS_COLORS}"

# ── Word navigation & deletion (Ctrl+Arrow / Ctrl+Backspace / Ctrl+Delete) ──
bindkey '^[[1;5C' forward-word           # Ctrl+Right
bindkey '^[[1;5D' backward-word          # Ctrl+Left
bindkey '^H'      backward-kill-word     # Ctrl+Backspace (Windows Terminal / WSL2)
bindkey '^[^?'    backward-kill-word     # Option+Backspace (macOS)
bindkey '^[[3;5~' kill-word              # Ctrl+Delete

# ── fzf ──────────────────────────────────────────────────────────────────────
export FZF_DEFAULT_OPTS="--height=40% --border -m --bind='ctrl-/:toggle-preview'"
export FZF_DEFAULT_COMMAND='fd --type f --hidden --follow --exclude .git'
export FZF_CTRL_T_COMMAND="$FZF_DEFAULT_COMMAND"
export FZF_ALT_C_COMMAND='fd --type d --hidden --follow --exclude .git'
[ -f ~/.fzf.zsh ] && source ~/.fzf.zsh

# ── bat ──────────────────────────────────────────────────────────────────────
export MANPAGER="sh -c 'col -bx | bat -l man -p'"
export BAT_THEME="Monokai Extended"

# ── zoxide ───────────────────────────────────────────────────────────────────
if command -v zoxide &>/dev/null; then
    eval "$(zoxide init zsh)"
fi

# ── delta ────────────────────────────────────────────────────────────────────
if command -v delta &>/dev/null; then
    export GIT_PAGER="delta"
fi

# ── zsh-autosuggestions ──────────────────────────────────────────────────────
_ZSH_AUTOSUGGEST="${HOMEBREW_PREFIX:-$(brew --prefix 2>/dev/null)}/share/zsh-autosuggestions/zsh-autosuggestions.zsh"
if [[ -f "$_ZSH_AUTOSUGGEST" ]]; then
    source "$_ZSH_AUTOSUGGEST"
    ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE='fg=8'
    ZSH_AUTOSUGGEST_STRATEGY=(history completion)
    ZSH_AUTOSUGGEST_BUFFER_MAX_SIZE=20
    bindkey '^ ' autosuggest-accept   # Ctrl+Space
    bindkey '^f'  autosuggest-accept  # Ctrl+F
fi

# ── zsh-history-substring-search ─────────────────────────────────────────────
_ZSH_HIST_SEARCH="${HOMEBREW_PREFIX:-$(brew --prefix 2>/dev/null)}/share/zsh-history-substring-search/zsh-history-substring-search.zsh"
if [[ -f "$_ZSH_HIST_SEARCH" ]]; then
    source "$_ZSH_HIST_SEARCH"
    bindkey '^[[A' history-substring-search-up
    bindkey '^[[B' history-substring-search-down
    bindkey -M vicmd 'k' history-substring-search-up
    bindkey -M vicmd 'j' history-substring-search-down
fi

# ── zsh-syntax-highlighting (must be last) ───────────────────────────────────
_ZSH_SYNTAX="${HOMEBREW_PREFIX:-$(brew --prefix 2>/dev/null)}/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh"
if [[ -f "$_ZSH_SYNTAX" ]]; then
    source "$_ZSH_SYNTAX"
fi

# ── starship: disable git on Windows mounts ──────────────────────────────────
_starship_select_config() {
  if [[ "$PWD" == /mnt/[a-z]/* ]]; then
    export STARSHIP_CONFIG="$HOME/.config/starship-no-git.toml"
  else
    unset STARSHIP_CONFIG
  fi
}
autoload -U add-zsh-hook
add-zsh-hook chpwd _starship_select_config
_starship_select_config

# ── starship prompt (must be after syntax-highlighting) ──────────────────────
if command -v starship &>/dev/null; then
    eval "$(starship init zsh)"
fi

# <<< dotfiles bootstrap <<<
EOF

# 마커가 이미 있으면 해당 위치에서 교체, 없으면 파일 끝에 추가
if grep -qF "$ZSH_START_MARKER" "$ZSHRC"; then
  info "Updating zsh config snippet in $ZSHRC (in-place)..."
  python3 - "$ZSHRC" "$_SNIPPET_TMP" <<'PYEOF'
import sys

zshrc_path, snippet_path = sys.argv[1], sys.argv[2]
START = "# >>> dotfiles bootstrap >>>"
END   = "# <<< dotfiles bootstrap <<<"

with open(zshrc_path) as f:
    lines = f.readlines()
with open(snippet_path) as f:
    new_block = f.read()

out, skip = [], False
for line in lines:
    if START in line:
        skip = True
        out.append(new_block)   # 마커 포함한 새 스니펫 삽입
        continue
    if END in line:
        skip = False
        continue                # 구 end 마커 제거 (new_block에 포함됨)
    if not skip:
        out.append(line)

with open(zshrc_path, 'w') as f:
    f.writelines(out)
PYEOF
  success "zsh config snippet updated"
else
  info "Appending zsh config to $ZSHRC..."
  cat "$_SNIPPET_TMP" >>"$ZSHRC"
  success "zsh config appended"
fi

rm -f "$_SNIPPET_TMP"

# ══════════════════════════════════════════════════════════════════════════════
# 9. Symlinks (dotfiles)
# ══════════════════════════════════════════════════════════════════════════════

link() {
  local src="$1" dst="$2"
  mkdir -p "$(dirname "$dst")"
  if [ -L "$dst" ]; then
    local cur
    cur="$(readlink "$dst")"
    if [ "$cur" = "$src" ]; then
      muted "already linked: $dst"
    else
      warn "relink: $dst (was → $cur)"
      rm "$dst"
      ln -s "$src" "$dst"
      success "linked: $dst"
    fi
  elif [ -e "$dst" ]; then
    warn "backup: $dst → $dst.bak"
    mv "$dst" "$dst.bak"
    ln -s "$src" "$dst"
    success "linked: $dst"
  else
    ln -s "$src" "$dst"
    success "linked: $dst"
  fi
}

echo ""
info "Symlinking dotfiles..."

# Shell dotfiles
link "$REPO_DIR/shell/.tmux.conf" "$HOME/.tmux.conf"
link "$REPO_DIR/shell/.wezterm.lua" "$HOME/.wezterm.lua"
link "$REPO_DIR/shell/.vscode-neovim.lua" "$HOME/.vscode-neovim.lua"
link "$REPO_DIR/shell/starship.toml" "$HOME/.config/starship.toml"

# Generated: no-git variant for /mnt/ paths (re-created on every install/update)
# On WSL, also swaps the Apple icon (U+E711) → Windows icon (U+E62A)
python3 - "$REPO_DIR/shell/starship.toml" "$PLATFORM" "$HOME/.config/starship-no-git.toml" <<'PYEOF2'
import re, sys
src, platform, dst = sys.argv[1], sys.argv[2], sys.argv[3]
content = open(src).read()
if platform == 'wsl':
    content = content.replace('', '')
content = re.sub(r'(\[git_branch\]\n)', r'\1disabled = true\n', content)
content = re.sub(r'(\[git_status\]\n)', r'\1disabled = true\n', content)
with open(dst, 'w') as f:
    f.write(content)
PYEOF2
muted "starship-no-git.toml generated"
link "$REPO_DIR/shell/lfrc" "$HOME/.config/lf/lfrc"

# Scripts (single directory symlink)
link "$REPO_DIR/shell/scripts" "$HOME/.devenv-scripts"

# Neovim config
link "$REPO_DIR/nvim" "$HOME/.config/nvim"

# Claude Code CLAUDE.md — global instructions
link "$REPO_DIR/claude-plugin/CLAUDE.home.md" "$HOME/.claude/CLAUDE.md"

# Claude Code blueprint plugin — clean up old per-file symlinks we created
# (skills/agents/infra were previously symlinked individually; now the plugin handles them).
# Only removes symlinks whose target is inside REPO_DIR — never touches foreign symlinks.
info "Cleaning up old blueprint skill/agent/infra symlinks..."
for dir in "$HOME/.claude/skills" "$HOME/.claude/agents" "$HOME/.claude/infra"; do
  [ -d "$dir" ] || continue
  for entry in "$dir"/*; do
    [ -L "$entry" ] || continue
    if [[ "$(readlink "$entry")" == "$REPO_DIR"* ]]; then
      rm "$entry"
      muted "removed own symlink: $entry"
    fi
  done
done
# Remove old agents folder symlink if still present from a very old install
if [ -L "$HOME/.claude/agents" ] && [[ "$(readlink "$HOME/.claude/agents")" == "$REPO_DIR"* ]]; then
  warn "removing old agents folder symlink"
  rm "$HOME/.claude/agents"
fi

# Claude Code hooks — link hook scripts
mkdir -p "$HOME/.claude/hooks"
for hook_file in "$REPO_DIR/claude-plugin/hooks"/*.sh; do
  [ -f "$hook_file" ] || continue
  hook_name="$(basename "$hook_file")"
  link "$hook_file" "$HOME/.claude/hooks/$hook_name"
done

# Claude Code settings — ensure required config is set
info "Ensuring Claude Code settings..."
mkdir -p "$HOME/.claude"
python3 - "$HOME/.claude/settings.json" "$HOME/.claude.json" "$REPO_DIR" <<'PYEOF'
import json, sys, os

settings_path, claude_json_path, repo_dir = sys.argv[1], sys.argv[2], sys.argv[3]

# ── settings.json (project-level: env vars) ──────────────────────────────────
required_env = {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1",
    "ENABLE_TOOL_SEARCH": "1"
}

if os.path.isfile(settings_path):
    with open(settings_path) as f:
        settings = json.load(f)
else:
    settings = {}

env = settings.setdefault("env", {})
changed = False
for key, val in required_env.items():
    if env.get(key) != val:
        env[key] = val
        changed = True

# ── hooks (merge without overwriting user hooks) ─────────────────────────────
required_hooks = {
    "TeammateIdle": [
        {
            "matcher": "",
            "hooks": [
                {
                    "type": "command",
                    "command": "bash ~/.claude/hooks/teammate-idle-token-tracker.sh",
                    "timeout": 10,
                }
            ],
        }
    ],
}

hooks = settings.setdefault("hooks", {})
for event, entries in required_hooks.items():
    existing = hooks.get(event, [])
    # Check if our hook command is already present
    our_commands = {h["command"] for e in entries for h in e.get("hooks", [])}
    already = any(
        h.get("command") in our_commands
        for e in existing
        for h in e.get("hooks", [])
    )
    if not already:
        hooks[event] = existing + entries
        changed = True

# ── blueprint plugin registration ────────────────────────────────────────────
required_marketplaces = {
    "kang-sw-devenv": {
        "source": {"source": "directory", "path": repo_dir}
    }
}
required_plugins = {"ws@kang-sw-devenv": True}

obsolete_marketplaces = {"ws"}
obsolete_plugins = {"ws@ws"}

marketplaces = settings.setdefault("extraKnownMarketplaces", {})
for name in obsolete_marketplaces:
    if name in marketplaces:
        del marketplaces[name]
        changed = True
for name, cfg in required_marketplaces.items():
    if marketplaces.get(name) != cfg:
        marketplaces[name] = cfg
        changed = True

plugins = settings.setdefault("enabledPlugins", {})
for name in obsolete_plugins:
    if name in plugins:
        del plugins[name]
        changed = True
for name, val in required_plugins.items():
    if plugins.get(name) != val:
        plugins[name] = val
        changed = True

if changed:
    with open(settings_path, "w") as f:
        json.dump(settings, f, indent=2)
        f.write("\n")
    print("  \033[1;32m  ✔ Claude Code settings.json updated\033[0m")
else:
    print("  \033[90m  ✔ Claude Code settings.json already current\033[0m")

# ── claude.json (global user config) ─────────────────────────────────────────
required_global = {
    "teammateMode": "in-process",
}

if os.path.isfile(claude_json_path):
    with open(claude_json_path) as f:
        global_cfg = json.load(f)
else:
    global_cfg = {}

changed = False
for key, val in required_global.items():
    if global_cfg.get(key) != val:
        global_cfg[key] = val
        changed = True

if changed:
    with open(claude_json_path, "w") as f:
        json.dump(global_cfg, f, indent=2)
        f.write("\n")
    print("  \033[1;32m  ✔ Claude Code claude.json updated\033[0m")
else:
    print("  \033[90m  ✔ Claude Code claude.json already current\033[0m")
PYEOF

# ── ws plugin snapshot copy ───────────────────────────────────────────────────
# Copy claude-plugin/ to a stable cache; Claude Code reads from the cache, not the live repo.
# Re-run install.sh update (or claude plugin update ws@kang-sw-devenv) after changes.
PLUGIN_CACHE="$HOME/.claude/plugins/ws-plugin"
info "Syncing ws plugin snapshot to $PLUGIN_CACHE/claude/..."
mkdir -p "$PLUGIN_CACHE/claude"
rsync -a --delete "$REPO_DIR/claude-plugin/" "$PLUGIN_CACHE/claude/"
success "ws plugin snapshot synced"

# Pre-register marketplace in known_marketplaces.json so `claude plugin install` can resolve it
# before Claude Code has had a chance to process extraKnownMarketplaces itself.
mkdir -p "$HOME/.claude/plugins"
python3 - "$HOME/.claude/plugins/known_marketplaces.json" "$PLUGIN_CACHE" "$REPO_DIR" <<'PYEOF'
import json, sys, os
from datetime import datetime, timezone

km_path, plugin_cache, repo_dir = sys.argv[1], sys.argv[2], sys.argv[3]
km = json.load(open(km_path)) if os.path.isfile(km_path) else {}

entry = {
    "source": {"source": "directory", "path": repo_dir},
    "installLocation": plugin_cache,
    "lastUpdated": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.") + f"{datetime.now(timezone.utc).microsecond // 1000:03d}Z"
}
if km.get("kang-sw-devenv") == entry:
    print("  \033[90m  ✔ kang-sw-devenv marketplace already registered\033[0m")
else:
    km["kang-sw-devenv"] = entry
    with open(km_path, "w") as f:
        json.dump(km, f, indent=2)
        f.write("\n")
    print("  \033[1;32m  ✔ kang-sw-devenv marketplace registered\033[0m")
PYEOF

if has claude; then
  INSTALLED_PLUGINS="$HOME/.claude/plugins/installed_plugins.json"
  if [[ -f "$INSTALLED_PLUGINS" ]] && python3 -c "
import json, sys
d = json.load(open(sys.argv[1]))
p = d.get('plugins', {}).get('ws@kang-sw-devenv', {})
sys.exit(0 if p and p.get('installLocation') == sys.argv[2] else 1)
" "$INSTALLED_PLUGINS" "$PLUGIN_CACHE" 2>/dev/null; then
    muted "ws plugin already installed at snapshot path"
  else
    info "Installing ws plugin..."
    # Remove stale entry so claude plugin install writes a fresh installLocation.
    # Version equality would otherwise cause the install to no-op without updating the path.
    if [[ -f "$INSTALLED_PLUGINS" ]]; then
      python3 - "$INSTALLED_PLUGINS" <<'PYEOF'
import json, sys
path = sys.argv[1]
d = json.load(open(path))
if 'ws@kang-sw-devenv' in d.get('plugins', {}):
    del d['plugins']['ws@kang-sw-devenv']
    with open(path, 'w') as f:
        json.dump(d, f, indent=2)
        f.write('\n')
PYEOF
    fi
    claude plugin install ws@kang-sw-devenv && success "ws plugin installed" || warn "ws plugin install failed — run manually: claude plugin install ws@kang-sw-devenv"
  fi
else
  warn "claude not found — run manually after install: claude plugin install ws@kang-sw-devenv"
fi

# Clean up dead symlinks (skills and agents)
cleanup_dead_links() {
  local dir="$1"
  for entry in "$dir"/*; do
    [ -L "$entry" ] || continue
    if [ ! -e "$entry" ]; then
      warn "removing dead link: $entry"
      rm "$entry"
    fi
  done
}
cleanup_dead_links "$HOME/.claude/skills"
cleanup_dead_links "$HOME/.claude/agents"

# ══════════════════════════════════════════════════════════════════════════════

echo ""
success "All done! Restart your shell or run: exec zsh"
