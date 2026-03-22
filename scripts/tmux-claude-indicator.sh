#!/usr/bin/env bash
# tmux-claude-indicator.sh — spinner on window tab when Claude Code is actively outputting
# Usage (in window-status-format): #(~/.config/nvim/scripts/tmux-claude-indicator.sh '#S:#I')
#
# Debug: tmux show-environment -g | grep MYTMUX_WINDOW
#   State format: hash|last_active_epoch|last_check_epoch|frame_index

EVAL_INTERVAL=2 # seconds between full ps/capture-pane checks
COOLDOWN=2      # seconds to keep spinner after last detected change
FRAMES=(🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘)

WINDOW="${1:-}"
[[ -z "$WINDOW" ]] && exit 0

WIN_IDX="${WINDOW##*:}"
STATE_KEY="MYTMUX_WINDOW_${WIN_IDX}"

now=$(date +%s)
state=$(tmux show-environment -g "$STATE_KEY" 2>/dev/null | cut -d= -f2-)
IFS='|' read -r prev_hash last_active last_check frame <<<"$state"

# ── Full evaluation every EVAL_INTERVAL seconds ────────────
if [[ -z "$last_check" || $((now - last_check)) -ge $EVAL_INTERVAL ]]; then
  last_check=$now

  claude_pane=""
  while IFS='|' read -r pane_id pane_tty; do
    [[ -z "$pane_tty" ]] && continue
    if ps -t "${pane_tty#/dev/}" -o comm= 2>/dev/null | grep -qx claude; then
      claude_pane="$pane_id"
      break
    fi
  done < <(tmux list-panes -t "$WINDOW" -F '#{pane_id}|#{pane_tty}' 2>/dev/null)

  if [[ -n "$claude_pane" ]]; then
    hash=$(tmux capture-pane -t "$claude_pane" -p -S -5 2>/dev/null | cksum | cut -d' ' -f1)
    [[ -n "$prev_hash" && "$hash" != "$prev_hash" ]] && last_active=$now
    prev_hash=$hash
  else
    prev_hash="" last_active="" frame=""
  fi
fi

# ── Spinner output (every tick) ─────────────────────────────
if [[ -n "$last_active" && $((now - last_active)) -lt $COOLDOWN ]]; then
  frame=$(((${frame:-0} + 1) % ${#FRAMES[@]}))
  printf ' %s' "${FRAMES[$frame]}"
else
  frame=""
fi

tmux set-environment -g "$STATE_KEY" "${prev_hash}|${last_active}|${last_check}|${frame}"
