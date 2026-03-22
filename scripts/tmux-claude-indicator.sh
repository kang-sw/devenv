#!/usr/bin/env bash
# tmux-claude-indicator.sh — spinner on window tab when Claude Code is actively outputting
# Usage (in window-status-format): #(~/.config/nvim/scripts/tmux-claude-indicator.sh '#S:#I')
#
# Debug: tmux show-environment -g | grep MYTMUX_WINDOW

COOLDOWN=5 # seconds — keep spinner visible after last detected change
FRAMES=(⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏)

WINDOW="${1:-}"
[[ -z "$WINDOW" ]] && exit 0

WIN_IDX="${WINDOW##*:}"
HASH_KEY="MYTMUX_WINDOW_HASH_${WIN_IDX}"
ACTIVE_KEY="MYTMUX_WINDOW_ACTIVE_${WIN_IDX}"
FRAME_KEY="MYTMUX_WINDOW_FRAME_${WIN_IDX}"

# ── Find a pane running claude in this window ───────────────
claude_pane=""
while IFS='|' read -r pane_id pane_tty; do
  [[ -z "$pane_tty" ]] && continue
  if ps -t "${pane_tty#/dev/}" -o comm= 2>/dev/null | grep -qx claude; then
    claude_pane="$pane_id"
    break
  fi
done < <(tmux list-panes -t "$WINDOW" -F '#{pane_id}|#{pane_tty}' 2>/dev/null)

if [[ -z "$claude_pane" ]]; then
  tmux set-environment -g "$HASH_KEY" "" 2>/dev/null
  tmux set-environment -g "$ACTIVE_KEY" "" 2>/dev/null
  tmux set-environment -g "$FRAME_KEY" "" 2>/dev/null
  exit 0
fi

# ── Detect active output via content hash ───────────────────
now=$(date +%s)
hash=$(tmux capture-pane -t "$claude_pane" -p -S -5 2>/dev/null | cksum | cut -d' ' -f1)
prev=$(tmux show-environment -g "$HASH_KEY" 2>/dev/null | cut -d= -f2-)
last_active=$(tmux show-environment -g "$ACTIVE_KEY" 2>/dev/null | cut -d= -f2-)

tmux set-environment -g "$HASH_KEY" "$hash"

active=false
if [[ -n "$prev" && "$hash" != "$prev" ]]; then
  tmux set-environment -g "$ACTIVE_KEY" "$now"
  active=true
elif [[ -n "$last_active" && $((now - last_active)) -lt $COOLDOWN ]]; then
  active=true
fi

if $active; then
  frame=$(tmux show-environment -g "$FRAME_KEY" 2>/dev/null | cut -d= -f2-)
  frame=$(( (${frame:-0} + 1) % ${#FRAMES[@]} ))
  tmux set-environment -g "$FRAME_KEY" "$frame"
  printf ' %s' "${FRAMES[$frame]}"
else
  tmux set-environment -g "$FRAME_KEY" "" 2>/dev/null
fi
