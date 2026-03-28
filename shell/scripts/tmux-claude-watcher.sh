#!/usr/bin/env bash
# tmux-claude-watcher.sh — background daemon for Claude Code activity indicators.
#
# Per-pane state machine:
#   S (spinning) → A (active output, content still changing) → G (grace) → D (done ✅)
#   S → G when content stops changing (skip A)
#   Any state → S on spinner | P on prompt | R on retry
#   D/G → clear when window becomes active
#
# Timing: full scan every ~2s, spinner animation at ~3fps between scans.
# All tmux writes batched into a single source-file call.
#
# Start from tmux.conf:  run-shell -b '~/.devenv-scripts/tmux-claude-watcher.sh'
# Use in format string:  #{@claude-indicator}

FRAMES=(🌑 🌒 🌓 🌔 🌕 🌖 🌗 🌘)
CLAUDE_SPINNER_RE='[·✢✳✶✻✽].*…'
CLAUDE_RETRY_RE='Retrying in [0-9]+ seconds'

# Cross-platform hash command
if command -v md5 &>/dev/null; then
  _hash() { md5 -q; }
elif command -v md5sum &>/dev/null; then
  _hash() { md5sum | cut -d' ' -f1; }
else
  _hash() { cksum | cut -d' ' -f1; }
fi

# Per-pane content hash stored as CW_H_<pane> in tmux env (bash 3.2 compat)
# Retrieved via lookup() from the all_env snapshot — no extra IPC

# ── Instance management via token ───────────────────────────────────────────────
TOKEN="$$"
tmux set-environment -g CLAUDE_WATCHER_TOKEN "$TOKEN"

yield_check() {
  local cur
  cur=$(tmux show-environment -g CLAUDE_WATCHER_TOKEN 2>/dev/null) || exit 0
  [[ "${cur#*=}" != "$TOKEN" ]] && exit 0
}

# ── Helpers ─────────────────────────────────────────────────────────────────────
repeat_str() {
  local s="" i
  for ((i = 0; i < $2; i++)); do s+="$1"; done
  printf '%s' "$s"
}

build_moons() {
  local base=$1 count=$2 s="" i f
  for ((i = 0; i < count; i++)); do
    f=$(((base + i) % ${#FRAMES[@]}))
    s+="${FRAMES[$f]}"
  done
  printf '%s' "$s"
}

# Lookup key from cached env snapshot (pure bash, no forks)
lookup() {
  local key="$1" line
  while IFS= read -r line; do
    [[ "$line" == "${key}="* ]] && {
      printf '%s' "${line#*=}"
      return
    }
  done <<<"$all_env"
}

flush_window() {
  [[ -z "$prev_win" ]] && return

  local safe_key="${prev_win//:/_}"
  local frame_key="CW_W_${safe_key}"
  local frame indicator

  frame=$(lookup "$frame_key")
  frame="${frame:-0}"

  # Build indicator: 🔥… 🌑… ❌… ✅…
  indicator=""
  ((prompt_count > 0)) && indicator+="$(repeat_str '🔥' "$prompt_count")"
  ((spin_count > 0)) && indicator+="$(build_moons "$frame" "$spin_count")"
  ((retry_count > 0)) && indicator+="$(repeat_str '❌' "$retry_count")"
  ((done_count > 0)) && indicator+="$(repeat_str '✅' "$done_count")"
  [[ -n "$indicator" ]] && indicator=" $indicator"

  # Track spinning windows for animation
  if ((spin_count > 0)); then
    spinning_wins+=("$prev_win")
    spinning_frames+=("$frame")
    spinning_counts+=("$spin_count")
    spinning_prefixes+=("$(repeat_str '🔥' "$prompt_count")")
    spinning_suffixes+=("$(repeat_str '❌' "$retry_count")$(repeat_str '✅' "$done_count")")
  fi

  printf "set-environment -g %s %s\n" "$frame_key" "$frame" >>"$batch"
  printf "set-option -wq -t '%s' @claude-indicator '%s'\n" "$prev_win" "$indicator" >>"$batch"
}

# ── Main loop ───────────────────────────────────────────────────────────────────
while tmux list-sessions &>/dev/null; do
  yield_check

  # Snapshot all CW_ vars in one IPC call
  all_env=$(tmux show-environment -g 2>/dev/null | grep '^CW_' || true)

  batch=$(mktemp)

  prev_win=""
  prev_active=""
  spin_count=0
  prompt_count=0
  done_count=0
  retry_count=0
  spinning_wins=()
  spinning_frames=()
  spinning_counts=()
  spinning_prefixes=()
  spinning_suffixes=()

  # ── Full scan: per-pane state machine ─────────────────────────────────
  while IFS=$'\t' read -r win_target active pane_id; do
    if [[ "$win_target" != "$prev_win" ]]; then
      flush_window
      prev_win="$win_target"
      prev_active="$active"
      spin_count=0
      prompt_count=0
      done_count=0
      retry_count=0
    fi

    content=$(tmux capture-pane -t "$pane_id" -p 2>/dev/null) || continue
    safe_pane="${pane_id#%}"

    # Content change detection (hash comparison)
    cur_hash=$(printf '%s' "$content" | sed 's/[[:space:]]*$//' | _hash)
    hash_key="CW_H_${safe_pane}"
    prev_hash=$(lookup "$hash_key")
    content_changed=""
    [[ "$cur_hash" != "$prev_hash" ]] && content_changed=1

    # Detect current pane activity
    has_prompt=""
    has_spinner=""
    has_retry=""
    if printf '%s\n' "$content" | awk '/❯.*1\. Yes/{y=1}END{exit !(y)}'; then
      has_prompt=1
    fi
    if printf '%s' "$content" | grep -qE "$CLAUDE_RETRY_RE"; then
      has_retry=1
    fi
    if printf '%s' "$content" | grep -qE "$CLAUDE_SPINNER_RE"; then
      has_spinner=1
    fi

    # Per-pane state machine: S/R → A → G → D
    pane_key="CW_P_${safe_pane}"
    prev_state=$(lookup "$pane_key")
    new_state=""

    if [[ -n "$has_prompt" ]]; then
      new_state="P"
    elif [[ -n "$has_retry" ]]; then
      new_state="R"
    elif [[ -n "$has_spinner" ]]; then
      new_state="S"
    else
      case "$prev_state" in
      S | R)
        # Spinner/retry stopped — if content still changing, enter A (active output)
        [[ -n "$content_changed" ]] && new_state="A" || new_state="G"
        ;;
      A)
        # Active output — stay while content keeps changing
        [[ -n "$content_changed" ]] && new_state="A" || new_state="G"
        ;;
      G) [[ "$active" == "1" ]] && new_state="" || new_state="D" ;;
      D) [[ "$active" == "1" ]] && new_state="" || new_state="D" ;;
      *) new_state="" ;;
      esac
    fi

    # Batch pane state + hash write
    if [[ -n "$new_state" ]]; then
      printf "set-environment -g %s %s\n" "$pane_key" "$new_state" >>"$batch"
    else
      printf "set-environment -gu %s\n" "$pane_key" >>"$batch"
    fi
    printf "set-environment -g %s %s\n" "$hash_key" "$cur_hash" >>"$batch"

    # Aggregate counts for window indicator
    case "$new_state" in
    P) prompt_count=$((prompt_count + 1)) ;;
    R) retry_count=$((retry_count + 1)) ;;
    S | A | G) spin_count=$((spin_count + 1)) ;;
    D) done_count=$((done_count + 1)) ;;
    esac

  done < <(tmux list-panes -a -F "#{session_name}:#{window_index}	#{window_active}	#{pane_id}" 2>/dev/null)

  flush_window # last window

  # Execute all writes in one IPC call
  tmux source-file "$batch" 2>/dev/null || true
  rm -f "$batch"

  # ── Animate spinners between scans (~2s at ~3fps) ─────────────────────
  for _ in 1 2 3 4 5 6; do
    sleep 0.33
    yield_check

    for i in "${!spinning_wins[@]}"; do
      f="${spinning_frames[$i]}"
      f=$(((f + 1) % ${#FRAMES[@]}))
      spinning_frames[$i]=$f

      moon="$(build_moons "$f" "${spinning_counts[$i]}")"
      tmux set-option -wq -t "${spinning_wins[$i]}" @claude-indicator \
        " ${spinning_prefixes[$i]}${moon}${spinning_suffixes[$i]}" 2>/dev/null || true
    done
  done

  # Persist final animation frames
  for i in "${!spinning_wins[@]}"; do
    safe_key="${spinning_wins[$i]//:/_}"
    tmux set-environment -g "CW_W_${safe_key}" "${spinning_frames[$i]}" 2>/dev/null || true
  done
done
