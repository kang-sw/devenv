#!/usr/bin/env bash
# tmux-claude-indicator.sh — spinner on window tab when Claude Code is actively outputting
# Usage (in window-status-format): #(~/.devenv-scripts/tmux-claude-indicator.sh '#S:#I' '#{window_active}')
#
# Priority: 🔥 prompt > 🌑 spinner > ✅ completed (background only)
# State format (tmux env): was_spinning|completed|frame

FRAMES=(🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘)

# Claude Code thinking spinner chars (from binary):
#   [·, ✢, ✳, ✶, ✻, ✽] — spinner char + word + "…"
CLAUDE_SPINNER_RE='[·✢✳✶✻✽].*…'

WINDOW="${1:-}"
[[ -z "$WINDOW" ]] && exit 0

IS_ACTIVE="${2:-0}"
WIN_IDX="${WINDOW##*:}"
STATE_KEY="MYTMUX_WINDOW_${WIN_IDX}"

# ── Read persisted state ──────────────────────────────────────────────────────
state=$(tmux show-environment -g "$STATE_KEY" 2>/dev/null)
state="${state#*=}"
IFS='|' read -r was_spinning completed frame <<<"$state"

# ── Scan all panes for Claude indicators ──────────────────────────────────────
# Process detection (ps -t tty) is unreliable on WSL where claude runs behind
# powershell.exe with a different controlling tty. Instead, scan pane content
# directly — the spinner/prompt patterns are specific enough to avoid false positives.
has_prompt=""
has_spinner=""

while IFS= read -r pane_id; do
  content=$(tmux capture-pane -t "$pane_id" -p 2>/dev/null) || continue

  printf '%s' "$content" | grep -qE "$CLAUDE_SPINNER_RE" && has_spinner=1
  printf '%s\n' "$content" | awk '/1\. Yes/{y=1} /[0-9]+\. No/{n=1} END{exit !(y && n)}' && has_prompt=1

  # Early exit: both found, no need to check remaining panes
  [[ -n "$has_spinner" && -n "$has_prompt" ]] && break
done < <(tmux list-panes -t "$WINDOW" -F '#{pane_id}' 2>/dev/null)

# ── Output indicator ──────────────────────────────────────────────────────────
if [[ -n "$has_prompt" ]]; then
  printf ' 🔥'
  # Prompt means user interaction required — not a background completion.
  # Clear spinning state so answering the prompt won't trigger ✅.
  was_spinning=""
  completed=""
elif [[ -n "$has_spinner" ]]; then
  frame=$(((${frame:-0} + 1) % ${#FRAMES[@]}))
  printf ' %s' "${FRAMES[$frame]}"
  was_spinning=1
  completed=""
elif [[ "$was_spinning" == "1" ]]; then
  # Spinner just stopped — show ✅ only if user isn't watching this window
  was_spinning=""
  if [[ "$IS_ACTIVE" != "1" ]]; then
    completed=1
    printf ' ✅'
  fi
elif [[ "$completed" == "1" ]]; then
  # Keep ✅ until user switches to this window
  if [[ "$IS_ACTIVE" == "1" ]]; then
    completed=""
  else
    printf ' ✅'
  fi
fi

tmux set-environment -g "$STATE_KEY" "${was_spinning}|${completed}|${frame}"
