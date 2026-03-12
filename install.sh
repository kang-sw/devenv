#!/usr/bin/env bash
# install.sh — dotfiles + dev environment bootstrap
# Supports: macOS, WSL (Ubuntu/Debian), native Linux
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ══════════════════════════════════════════════════════════════════════════════
# Helpers
# ══════════════════════════════════════════════════════════════════════════════

info()    { printf "\033[1;34m  ➜ %s\033[0m\n" "$*"; }
success() { printf "\033[1;32m  ✔ %s\033[0m\n" "$*"; }
warn()    { printf "\033[1;33m  ⚠ %s\033[0m\n" "$*"; }
die()     { printf "\033[1;31m  ✘ %s\033[0m\n" "$*" >&2; exit 1; }
has()     { command -v "$1" &>/dev/null; }

# ══════════════════════════════════════════════════════════════════════════════
# Platform detection
# ══════════════════════════════════════════════════════════════════════════════

PLATFORM="linux"
if [[ "$(uname)" == "Darwin" ]]; then
    PLATFORM="macos"
elif grep -qi microsoft /proc/version 2>/dev/null; then
    PLATFORM="wsl"
fi

info "Platform: $PLATFORM"

# ══════════════════════════════════════════════════════════════════════════════
# 1. System prerequisites
# ══════════════════════════════════════════════════════════════════════════════

if [[ "$PLATFORM" == "wsl" || "$PLATFORM" == "linux" ]]; then
    info "Updating apt and installing prerequisites..."
    sudo apt-get update -qq
    sudo apt-get install -y -qq \
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
        success "Xcode CLT already present"
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
    success "Homebrew already present"
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
        success "$pkg already installed (brew)"
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
        sudo apt-get install -y -qq zsh
    fi
fi

ZSH_BIN="$(command -v zsh)"

if [[ "$SHELL" != "$ZSH_BIN" ]]; then
    info "Setting zsh as default shell ($ZSH_BIN)..."
    if ! grep -qF "$ZSH_BIN" /etc/shells; then
        echo "$ZSH_BIN" | sudo tee -a /etc/shells
    fi
    chsh -s "$ZSH_BIN" || warn "chsh failed — run manually: chsh -s $ZSH_BIN"
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

# Nerd Font (macOS cask only; on Linux/WSL install on the host terminal side)
if [[ "$PLATFORM" == "macos" ]]; then
    if ! brew list --cask font-jetbrains-mono-nerd-font &>/dev/null; then
        info "Installing JetBrainsMono Nerd Font..."
        brew install --cask font-jetbrains-mono-nerd-font
        success "JetBrainsMono Nerd Font installed"
    else
        success "JetBrainsMono Nerd Font already installed"
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

# ══════════════════════════════════════════════════════════════════════════════
# 8. Zsh config (~/.zshrc)  — 항상 최신 내용으로 덮어씀 (위치 보존)
# ══════════════════════════════════════════════════════════════════════════════

ZSHRC="$HOME/.zshrc"
ZSH_START_MARKER="# >>> dotfiles bootstrap >>>"
ZSH_END_MARKER="# <<< dotfiles bootstrap <<<"

[[ ! -f "$ZSHRC" ]] && touch "$ZSHRC"

# 새 스니펫을 임시 파일에 작성
_SNIPPET_TMP=$(mktemp)
cat > "$_SNIPPET_TMP" << 'EOF'
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
setopt SHARE_HISTORY          # 세션 간 히스토리 공유
setopt APPEND_HISTORY

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
bindkey '^[^?'    backward-kill-word     # Ctrl+Backspace (via WezTerm: ESC+DEL)
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

# ── eza ──────────────────────────────────────────────────────────────────────
if command -v eza &>/dev/null; then
    alias ls='eza --icons'
    alias ll='eza -lah --icons --git'
    alias lt='eza --tree --icons -L 2'
    alias la='eza -a --icons'
fi

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

# ── starship prompt (must be after syntax-highlighting) ──────────────────────
if command -v starship &>/dev/null; then
    eval "$(starship init zsh)"
fi

# <<< dotfiles bootstrap <<<
EOF

# 마커가 이미 있으면 해당 위치에서 교체, 없으면 파일 끝에 추가
if grep -qF "$ZSH_START_MARKER" "$ZSHRC"; then
    info "Updating zsh config snippet in $ZSHRC (in-place)..."
    python3 - "$ZSHRC" "$_SNIPPET_TMP" << 'PYEOF'
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
    cat "$_SNIPPET_TMP" >> "$ZSHRC"
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
        success "already linked: $dst"
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

link "$REPO_DIR/.tmux.conf"         "$HOME/.tmux.conf"
link "$REPO_DIR/.wezterm.lua"       "$HOME/.wezterm.lua"
link "$REPO_DIR/.vscode-neovim.lua" "$HOME/.vscode-neovim.lua"
link "$REPO_DIR/starship.toml"      "$HOME/.config/starship.toml"
# NOTE: ~/.config/nvim is this repo itself — no symlink needed

# Claude Code skills — link each skill folder individually
mkdir -p "$HOME/.claude/skills"
for skill_dir in "$REPO_DIR/claude-skills"/*/; do
    [ -d "$skill_dir" ] || continue
    skill_name="$(basename "$skill_dir")"
    link "$skill_dir" "$HOME/.claude/skills/$skill_name"
done

# Claude Code agents
link "$REPO_DIR/claude-agents" "$HOME/.claude/agents"

# ══════════════════════════════════════════════════════════════════════════════

echo ""
success "All done! Restart your shell or run: exec zsh"
