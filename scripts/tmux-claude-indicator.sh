#!/usr/bin/env bash
# tmux-claude-indicator.sh — spinner on window tab when Claude Code is actively outputting
# Usage (in window-status-format): #(~/.config/nvim/scripts/tmux-claude-indicator.sh '#S:#I' '#{window_active}')
#
# Debug: tmux show-environment -g | grep MYTMUX_WINDOW
#   State format: _|last_active_epoch|last_check_epoch|frame_index|has_prompt|was_active|completed|has_spinner

EVAL_INTERVAL=3 # seconds between full ps/capture-pane checks
COOLDOWN=3      # seconds to keep spinner after last detected change
FRAMES=(🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘)

# Claude Code thinking spinner chars (extracted from binary v2.1.81):
#   ["·","✢","✳","✶","✻","✽"]  — detect these in pane to confirm Claude is working
# Match pattern: spinner char + word + "…" (e.g. "✻ Thinking…", "✽ Researching…")
# NOTE: do NOT use tail -N to limit checked lines — task lists push the spinner
#       out of the last few rows, causing missed detections (tested & reverted).
CLAUDE_SPINNER_RE='[✢✳✶✻✽] [A-Za-z].*…'

WINDOW="${1:-}"
[[ -z "$WINDOW" ]] && exit 0

IS_ACTIVE="${2:-0}"
WIN_IDX="${WINDOW##*:}"
STATE_KEY="MYTMUX_WINDOW_${WIN_IDX}"

now=$(date +%s)
state=$(tmux show-environment -g "$STATE_KEY" 2>/dev/null | cut -d= -f2-)
IFS='|' read -r _unused last_active last_check frame has_prompt was_active completed has_spinner <<<"$state"

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
    content=$(tmux capture-pane -t "$claude_pane" -p 2>/dev/null)

    if printf '%s' "$content" | grep -q '1. Yes'; then
      has_prompt=1
    else
      has_prompt=""
    fi

    # Detect Claude's thinking spinner in pane content
    # Only spinner presence drives last_active (not hash change),
    # so user typing alone won't trigger the moon-phase indicator.
    if printf '%s' "$content" | grep -qE "$CLAUDE_SPINNER_RE"; then
      has_spinner=1
      last_active=$now
    else
      has_spinner=""
    fi
  else
    last_active="" frame="" has_prompt="" has_spinner=""
  fi
fi

# ── Indicator output (every tick) ───────────────────────────
if [[ -n "$has_prompt" ]]; then
  printf ' 🔥'
  # Spinner-char detection: Claude is visibly busy → track for ✅
  [[ "$has_spinner" == "1" ]] && was_active=1
  # Rollback: IS_ACTIVE approach (uncomment & remove spinner line above)
  # [[ "$IS_ACTIVE" != "1" ]] && was_active=1
  completed=""
elif [[ -n "$last_active" && $((now - last_active)) -le $COOLDOWN ]]; then
  frame=$(((${frame:-0} + 1) % ${#FRAMES[@]}))
  printf ' %s' "${FRAMES[$frame]}"
  [[ "$has_spinner" == "1" ]] && was_active=1
  # [[ "$IS_ACTIVE" != "1" ]] && was_active=1
  completed=""
elif [[ "$was_active" == "1" ]]; then
  # spinner just stopped — mark completed if user isn't watching
  was_active=""
  frame=""
  if [[ "$IS_ACTIVE" != "1" ]]; then
    completed=1
    printf ' ✅'
  fi
elif [[ "$completed" == "1" ]]; then
  # stay ✅ until user switches to this window
  if [[ "$IS_ACTIVE" == "1" ]]; then
    completed=""
  else
    printf ' ✅'
  fi
fi

tmux set-environment -g "$STATE_KEY" "|${last_active}|${last_check}|${frame}|${has_prompt}|${was_active}|${completed}|${has_spinner}"
