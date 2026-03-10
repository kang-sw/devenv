#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | jq -r '.model.display_name')
DIR=$(echo "$input" | jq -r '.workspace.current_dir')
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
PCT=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
DURATION_MS=$(echo "$input" | jq -r '.cost.total_duration_ms // 0')

CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
RESET='\033[0m'

# Pick bar color based on context usage
if [ "$PCT" -ge 90 ]; then
  BAR_COLOR="$RED"
elif [ "$PCT" -ge 70 ]; then
  BAR_COLOR="$YELLOW"
else BAR_COLOR="$GREEN"; fi

FILLED=$((PCT / 10))
EMPTY=$((10 - FILLED))
BAR=$(printf "%${FILLED}s" | tr ' ' '█')$(printf "%${EMPTY}s" | tr ' ' '░')

MINS=$((DURATION_MS / 60000))
SECS=$(((DURATION_MS % 60000) / 1000))

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
  BRANCH="🌿 ${BRANCH_NAME} ${SPLIT_1}${AHEAD_BEHIND}${SPLIT_2}${GIT_IND}"
fi

echo -e "${CYAN}[$MODEL]${RESET} 📁 ${DIR##*/}"
if [[ $BRANCH ]]; then
  echo -e "$BRANCH"
fi
COST_FMT=$(printf '$%.2f' "$COST")
echo -e "${BAR_COLOR}${BAR}${RESET} ${PCT}% | ${YELLOW}${COST_FMT}${RESET} | ⏱️ ${MINS}m ${SECS}s"
