#!/usr/bin/env bash
# tmux-claude-watcher.sh — single background daemon that scans all windows for
# Claude Code activity and sets per-window @claude-indicator option.
#
# Supports multiple agents per window:
#   2 spinning + 1 prompt + 1 done → " 🔥🌓🌓✅"
#
# Timing: full scan every ~2s, spinner animation at ~3fps between scans.
#
# State per window (tmux env CW_<key>): spin_count|done_count|frame
#
# Start from tmux.conf:  run-shell -b '~/.devenv-scripts/tmux-claude-watcher.sh'
# Use in format string:  #{@claude-indicator}

FRAMES=(🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘)
CLAUDE_SPINNER_RE='[·✢✳✶✻✽].*…'

# ── Instance management via token ───────────────────────────────────────────────
TOKEN="$$"
tmux set-environment -g CLAUDE_WATCHER_TOKEN "$TOKEN"

yield_check() {
  local cur
  cur=$(tmux show-environment -g CLAUDE_WATCHER_TOKEN 2>/dev/null) || exit 0
  [[ "${cur#*=}" != "$TOKEN" ]] && exit 0
}

# ── Helpers ─────────────────────────────────────────────────────────────────────
# Repeat a string N times: repeat_str "🔥" 3 → "🔥🔥🔥"
repeat_str() {
  local s="" i
  for ((i=0; i<$2; i++)); do s+="$1"; done
  printf '%s' "$s"
}

# Build N moon spinners with +1 phase offset each: build_moons 2 3 → "🌓🌔🌕"
build_moons() {
  local base=$1 count=$2 s="" i f
  for ((i=0; i<count; i++)); do
    f=$(( (base + i) % ${#FRAMES[@]} ))
    s+="${FRAMES[$f]}"
  done
  printf '%s' "$s"
}

flush_window() {
  [[ -z "$prev_win" ]] && return

  local safe_key="${prev_win//:/_}"
  local state_key="CW_${safe_key}"
  local raw prev_spin done_count frame

  raw=$(tmux show-environment -g "$state_key" 2>/dev/null) || true
  raw="${raw#*=}"
  IFS='|' read -r prev_spin done_count frame <<<"$raw"
  prev_spin="${prev_spin:-0}"
  done_count="${done_count:-0}"
  frame="${frame:-0}"

  # ── Accumulate completions ────────────────────────────────────────────
  if (( spin_count < prev_spin )); then
    local newly_done=$(( prev_spin - spin_count ))
    if [[ "$prev_active" != "1" ]]; then
      done_count=$(( done_count + newly_done ))
    fi
  fi

  # Active window clears done indicators
  if [[ "$prev_active" == "1" ]]; then
    done_count=0
  fi

  # ── Build indicator: 🔥… 🌑… ✅… ───────────────────────────────────
  local indicator=""
  (( prompt_count > 0 )) && indicator+="$(repeat_str '🔥' "$prompt_count")"
  (( spin_count > 0 ))   && indicator+="$(build_moons "$frame" "$spin_count")"
  (( done_count > 0 ))   && indicator+="$(repeat_str '✅' "$done_count")"
  [[ -n "$indicator" ]] && indicator=" $indicator"

  # ── Track spinning windows for animation ──────────────────────────────
  if (( spin_count > 0 )); then
    spinning_wins+=("$prev_win")
    spinning_frames+=("$frame")
    spinning_counts+=("$spin_count")
    spinning_prefixes+=("$(repeat_str '🔥' "$prompt_count")")
    spinning_suffixes+=("$(repeat_str '✅' "$done_count")")
    spinning_states+=("${spin_count}|${done_count}")
  fi

  tmux set-environment -g "$state_key" "${spin_count}|${done_count}|${frame}" 2>/dev/null || true
  tmux set-option -wq -t "$prev_win" @claude-indicator "$indicator" 2>/dev/null || true
}

# ── Main loop ───────────────────────────────────────────────────────────────────
while tmux list-sessions &>/dev/null; do
  yield_check

  prev_win=""
  prev_active=""
  spin_count=0
  prompt_count=0
  spinning_wins=()
  spinning_frames=()
  spinning_counts=()
  spinning_prefixes=()
  spinning_suffixes=()
  spinning_states=()

  # ── Full scan: capture panes, count patterns per window ───────────────
  while IFS=$'\t' read -r win_target active pane_id; do
    if [[ "$win_target" != "$prev_win" ]]; then
      flush_window
      prev_win="$win_target"
      prev_active="$active"
      spin_count=0
      prompt_count=0
    fi

    content=$(tmux capture-pane -t "$pane_id" -p 2>/dev/null) || continue

    # Prompt takes priority over spinner per pane
    if printf '%s' "$content" | grep -qE '^\s*1\. Yes'; then
      prompt_count=$((prompt_count + 1))
    elif printf '%s' "$content" | grep -qE "$CLAUDE_SPINNER_RE"; then
      spin_count=$((spin_count + 1))
    fi

  done < <(tmux list-panes -a -F "#{session_name}:#{window_index}	#{window_active}	#{pane_id}" 2>/dev/null)

  flush_window  # last window

  # ── Animate spinners between scans (~2s at ~3fps) ─────────────────────
  for _ in 1 2 3 4 5 6; do
    sleep 0.33
    yield_check

    for i in "${!spinning_wins[@]}"; do
      f="${spinning_frames[$i]}"
      f=$(( (f + 1) % ${#FRAMES[@]} ))
      spinning_frames[$i]=$f

      moon="$(build_moons "$f" "${spinning_counts[$i]}")"
      tmux set-option -wq -t "${spinning_wins[$i]}" @claude-indicator \
        " ${spinning_prefixes[$i]}${moon}${spinning_suffixes[$i]}" 2>/dev/null || true
    done
  done

  # Persist final frame so next scan continues smoothly
  for i in "${!spinning_wins[@]}"; do
    safe_key="${spinning_wins[$i]//:/_}"
    tmux set-environment -g "CW_${safe_key}" "${spinning_states[$i]}|${spinning_frames[$i]}" 2>/dev/null || true
  done
done
