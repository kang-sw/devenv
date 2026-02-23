#!/usr/bin/env bash
# setup.sh — dotfiles symlink 설치 스크립트

set -e

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 심링크 생성 헬퍼
link() {
  local src="$1"
  local dst="$2"

  mkdir -p "$(dirname "$dst")"

  if [ -L "$dst" ]; then
    echo "  ✔ already linked: $dst"
  elif [ -e "$dst" ]; then
    echo "  ⚠ backup: $dst → $dst.bak"
    mv "$dst" "$dst.bak"
    ln -s "$src" "$dst"
    echo "  ✔ linked: $dst"
  else
    ln -s "$src" "$dst"
    echo "  ✔ linked: $dst"
  fi
}

echo "📦 dotfiles: $REPO_DIR"
echo ""

# ── Home dotfiles ──────────────────────────────
echo "[home]"
link "$REPO_DIR/.tmux.conf"         "$HOME/.tmux.conf"
link "$REPO_DIR/.wezterm.lua"       "$HOME/.wezterm.lua"
link "$REPO_DIR/.vscode-neovim.lua" "$HOME/.vscode-neovim.lua"

echo ""
echo "✅ done!"
