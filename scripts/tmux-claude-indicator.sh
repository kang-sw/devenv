#!/usr/bin/env bash
# tmux-claude-indicator.sh — show ● on window tab when Claude Code is actively outputting
# Usage (in window-status-format): #(~/.config/nvim/scripts/tmux-claude-indicator.sh '#S:#I')

WINDOW="${1:-}"
[[ -z "$WINDOW" ]] && exit 0

DIR="${TMPDIR:-/tmp}/tmux-claude-indicator"
KEY="${WINDOW//[^a-zA-Z0-9_]/_}"

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
    rm -f "$DIR/${KEY}.hash" 2>/dev/null
    exit 0
fi

# ── Detect active output via content hash ───────────────────
mkdir -p "$DIR" 2>/dev/null
hash=$(tmux capture-pane -t "$claude_pane" -p -S -5 2>/dev/null | cksum)
prev=$(cat "$DIR/${KEY}.hash" 2>/dev/null)
printf '%s' "$hash" > "$DIR/${KEY}.hash"

if [[ -n "$prev" && "$hash" != "$prev" ]]; then
    printf ' ●'
fi
