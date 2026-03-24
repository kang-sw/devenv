#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | jq -r '.model.display_name')
DIR=$(echo "$input" | jq -r '.workspace.current_dir')
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
TOKENS_USED=$(echo "$input" | jq -r '.context_window.current_usage | (.input_tokens + .output_tokens + .cache_creation_input_tokens + .cache_read_input_tokens)')
TOKENS_K=$(awk "BEGIN {printf \"%.0f\", $TOKENS_USED / 1000}")
CTX_MAX=$(echo "$input" | jq -r '.context_window.context_window_size // 0')
MAX_K=$(awk "BEGIN {printf \"%.0f\", $CTX_MAX / 1000}")
VISUAL_PCT=$(awk "BEGIN {v = $PCT; if (v > 100) v = 100; printf \"%.0f\", v}")
DURATION_MS=$(echo "$input" | jq -r '.cost.total_duration_ms // 0')
API_MS=$(echo "$input" | jq -r '.cost.total_api_duration_ms // 0')
LINES_ADDED=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
LINES_REMOVED=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')

CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
LIGHTBLUE='\033[94m'
PINK='\033[95m'
RESET='\033[0m'

# Pick bar color based on visual (sqrt-scaled) context usage
if [ "$VISUAL_PCT" -ge 90 ]; then
  BAR_COLOR="$RED"
elif [ "$VISUAL_PCT" -ge 70 ]; then
  BAR_COLOR="$YELLOW"
else BAR_COLOR="$GREEN"; fi

FILLED=$((VISUAL_PCT / 10))
EMPTY=$((10 - FILLED))
BAR=$(printf "%${FILLED}s" | tr ' ' '█')$(printf "%${EMPTY}s" | tr ' ' '░')

HRS=$((DURATION_MS / 3600000))
MINS=$(((DURATION_MS % 3600000) / 60000))
SECS=$(((DURATION_MS % 60000) / 1000))
API_HRS=$((API_MS / 3600000))
API_MINS=$(((API_MS % 3600000) / 60000))
API_SECS=$(((API_MS % 60000) / 1000))

fmt_time() {
  local h=$1 m=$2 s=$3
  if [ "$h" -gt 0 ]; then echo "${h}h ${m}m ${s}s"
  elif [ "$m" -gt 0 ]; then echo "${m}m ${s}s"
  else echo "${s}s"; fi
}
TIME_FMT=$(fmt_time "$HRS" "$MINS" "$SECS")
API_TIME_FMT=$(fmt_time "$API_HRS" "$API_MINS" "$API_SECS")

BRANCH=""
if git rev-parse --git-dir >/dev/null 2>&1; then
  BRANCH_NAME=$(git branch --show-current 2>/dev/null)
  DIFF_STAT=$(git diff --numstat 2>/dev/null | awk '{a+=$1; d+=$2} END {printf "%d %d", a+0, d+0}')
  ADDED=$(echo "$DIFF_STAT" | cut -d' ' -f1)
  DELETED=$(echo "$DIFF_STAT" | cut -d' ' -f2)
  MODIFIED=$(git diff --name-only 2>/dev/null | wc -l | tr -d ' ')
  UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')
  GIT_IND=""
  [ "$ADDED" -gt 0 ] 2>/dev/null && GIT_IND="${GIT_IND}${GREEN}++${ADDED}${RESET} "
  [ "$DELETED" -gt 0 ] 2>/dev/null && GIT_IND="${GIT_IND}${RED}--${DELETED}${RESET} "
  [ "$MODIFIED" -gt 0 ] 2>/dev/null && GIT_IND="${GIT_IND}${YELLOW}*${MODIFIED} files${RESET} "
  [ "$UNTRACKED" -gt 0 ] 2>/dev/null && GIT_IND="${GIT_IND}${CYAN}?${UNTRACKED} untracked${RESET} "
  AHEAD_BEHIND=""
  if git rev-parse --verify "@{u}" >/dev/null 2>&1; then
    AHEAD=$(git rev-list --count "@{u}..HEAD" 2>/dev/null || echo 0)
    BEHIND=$(git rev-list --count "HEAD..@{u}" 2>/dev/null || echo 0)
    [ "$AHEAD" -gt 0 ] 2>/dev/null && AHEAD_BEHIND="${AHEAD_BEHIND}${GREEN}↑${AHEAD}${RESET} "
    [ "$BEHIND" -gt 0 ] 2>/dev/null && AHEAD_BEHIND="${AHEAD_BEHIND}${YELLOW}↓${BEHIND}${RESET} "
  fi
  SPLIT_1=""
  SPLIT_2=""
  if [[ $AHEAD_BEHIND ]]; then SPLIT_1="| "; fi
  if [[ $GIT_IND ]]; then SPLIT_2="| "; fi
  SESSION_DELTA=""
  [ "$LINES_ADDED" -gt 0 ] 2>/dev/null && SESSION_DELTA="${SESSION_DELTA}${LIGHTBLUE}+${LINES_ADDED}${RESET} "
  [ "$LINES_REMOVED" -gt 0 ] 2>/dev/null && SESSION_DELTA="${SESSION_DELTA}${PINK}-${LINES_REMOVED}${RESET} "
  SPLIT_3=""
  if [[ $SESSION_DELTA ]]; then SPLIT_3="| "; fi
  BRANCH="🌿 ${BRANCH_NAME} ${SPLIT_1}${AHEAD_BEHIND}${SPLIT_2}${GIT_IND}${SPLIT_3}${SESSION_DELTA}"
fi

echo -e "${CYAN}[$MODEL]${RESET} 📁 ${DIR##*/}"
if [[ $BRANCH ]]; then
  echo -e "$BRANCH"
fi
COST_FMT=$(printf '$%.2f' "$COST")
echo -e "${BAR_COLOR}${BAR}${RESET} ${VISUAL_PCT}% (${TOKENS_K}/${MAX_K}k) | ${YELLOW}${COST_FMT}${RESET}"
echo -e "⏱️ ${TIME_FMT} | 🤔 ${API_TIME_FMT}"
