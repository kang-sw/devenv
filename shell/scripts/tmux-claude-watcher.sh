#!/usr/bin/env bash
# tmux-claude-watcher.sh — single background daemon that scans all windows for
# Claude Code activity and sets per-window @claude-indicator option.
#
# Perf budget (10 windows × 2 panes = 20 panes):
#   - 1 list-panes -a call  (was N+1 tmux calls)
#   - 20 capture-pane calls  (bottom 10 lines only)
#   - 0 forks for pattern matching  (bash [[ =~ ]] + glob, was 2 forks/pane)
#   - 10 show-environment + 20 set calls
#   Total: ~51 tmux IPC calls/s, 0 forks  (was ~61 calls + ~40 forks)
#
# Start from tmux.conf:  run-shell -b '~/.devenv-scripts/tmux-claude-watcher.sh'
# Use in format string:  #{@claude-indicator}

FRAMES=(🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘)
CLAUDE_SPINNER_RE='[·✢✳✶✻✽].*…'

# ── Single-instance guard: kill-and-replace on reload ───────────────────────────
PIDFILE="/tmp/tmux-claude-watcher-$(id -u).pid"

cleanup() { rm -f "$PIDFILE"; }
trap cleanup EXIT          # runs on any exit (including from the line below)
trap 'exit 0' INT TERM     # bash traps DON'T exit by default — explicit exit required

# Kill previous instance if running (enables config reload to pick up changes)
if [[ -f "$PIDFILE" ]]; then
  old_pid=$(cat "$PIDFILE" 2>/dev/null)
  if [[ -n "$old_pid" ]] && kill -0 "$old_pid" 2>/dev/null; then
    kill "$old_pid" 2>/dev/null
    for _ in 1 2 3 4 5; do
      kill -0 "$old_pid" 2>/dev/null || break
      sleep 0.1
    done
  fi
fi
echo $$ > "$PIDFILE"

# ── Helpers ─────────────────────────────────────────────────────────────────────
# Flush accumulated scan results for the previous window
flush_window() {
  [[ -z "$prev_win" ]] && return

  local safe_key="${prev_win//:/_}"
  local state_key="CW_${safe_key}"
  local raw was_spinning completed frame indicator

  raw=$(tmux show-environment -g "$state_key" 2>/dev/null) || true
  raw="${raw#*=}"
  IFS='|' read -r was_spinning completed frame <<<"$raw"

  indicator=""
  if [[ -n "$has_prompt" ]]; then
    indicator=' 🔥'; was_spinning=""; completed=""
  elif [[ -n "$has_spinner" ]]; then
    frame=$(( (${frame:-0} + 1) % ${#FRAMES[@]} ))
    indicator=" ${FRAMES[$frame]}"; was_spinning=1; completed=""
  elif [[ "${was_spinning:-}" == "1" ]]; then
    was_spinning=""
    if [[ "$prev_active" != "1" ]]; then completed=1; indicator=' ✅'; fi
  elif [[ "${completed:-}" == "1" ]]; then
    if [[ "$prev_active" == "1" ]]; then completed=""; else indicator=' ✅'; fi
  fi

  tmux set-environment -g "$state_key" "${was_spinning:-}|${completed:-}|${frame:-}" 2>/dev/null || true
  tmux set-option -wq -t "$prev_win" @claude-indicator "$indicator" 2>/dev/null || true
}

# ── Main loop ───────────────────────────────────────────────────────────────────
while tmux list-sessions &>/dev/null; do
  prev_win=""
  prev_active=""
  has_prompt=""
  has_spinner=""

  # Single call: every pane across all sessions (output grouped by window)
  while IFS=$'\t' read -r win_target active pane_id pane_height; do
    # Window boundary → flush previous window's results
    if [[ "$win_target" != "$prev_win" ]]; then
      flush_window
      prev_win="$win_target"
      prev_active="$active"
      has_prompt=""
      has_spinner=""
    fi

    # Both patterns found — skip remaining panes in this window
    [[ "$has_prompt" == "1" && "$has_spinner" == "1" ]] && continue

    # Capture only bottom 10 lines (spinner/prompt is always near bottom)
    start=$(( pane_height - 10 ))
    (( start < 0 )) && start=0
    content=$(tmux capture-pane -t "$pane_id" -p -S "$start" 2>/dev/null) || continue

    # Pattern matching — pure bash, no forks
    [[ "$content" =~ $CLAUDE_SPINNER_RE ]] && has_spinner=1
    [[ "$content" == *"1. Yes"* ]] && has_prompt=1

  done < <(tmux list-panes -a -F "#{session_name}:#{window_index}	#{window_active}	#{pane_id}	#{pane_height}" 2>/dev/null)

  flush_window  # last window

  sleep 1
done
