#!/bin/bash
input=$(cat)

MODEL=$(echo "$input" | jq -r '.model.display_name')
DIR=$(echo "$input" | jq -r '.workspace.current_dir')
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
PCT_RAW=$(echo "$input" | jq -r '.context_window.used_percentage // 0')
PCT=$(echo "$PCT_RAW" | awk '{printf "%d", $1}')
TOKENS_USED=$(echo "$input" | jq -r '.context_window.current_usage | (.input_tokens + .output_tokens + .cache_creation_input_tokens + .cache_read_input_tokens)')
TOKENS_K=$(awk "BEGIN {printf \"%.0f\", $TOKENS_USED / 1000}")
CTX_MAX=$(echo "$input" | jq -r '.context_window.context_window_size // 0')
MAX_K=$(awk "BEGIN {printf \"%.0f\", $CTX_MAX / 1000}")
OUTPUT_TOKENS=$(echo "$input" | jq -r '.context_window.total_output_tokens // 0')
DURATION_MS=$(echo "$input" | jq -r '.cost.total_duration_ms // 0')
API_MS=$(echo "$input" | jq -r '.cost.total_api_duration_ms // 0')
LINES_ADDED=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
LINES_REMOVED=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')
RATE_5HR=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // 0' | awk '{printf "%d", $1}')
RATE_7D=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // 0' | awk '{printf "%d", $1}')

# Colors
CYAN='\033[36m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
LIGHTBLUE='\033[94m'
PINK='\033[95m'
WHITE='\033[97m'
DIM='\033[2m'
RESET='\033[0m'

# Green → yellow → red gradient (ANSI 256-color)
pct_color() {
  awk -v p="$1" 'BEGIN {
    v = p + 0
    if (v < 0)   v = 0
    if (v > 100) v = 100
    if (v <= 50) {
      steps[0]=46; steps[1]=82; steps[2]=118; steps[3]=154; steps[4]=190; steps[5]=226
      idx = int(v / 50 * 5 + 0.5)
    } else {
      steps[0]=226; steps[1]=220; steps[2]=214; steps[3]=208; steps[4]=202; steps[5]=196
      idx = int((v - 50) / 50 * 5 + 0.5)
    }
    printf "\033[38;5;%dm", steps[idx]
  }'
}
PCT_COLOR="\033[48;5;236m$(pct_color "$PCT_RAW")"
RATE_5HR_COLOR=$(pct_color "$RATE_5HR")
RATE_7D_COLOR=$(pct_color "$RATE_7D")

# Context gauge — 10 chars using block characters (▏▎▍▌▋▊▉█)
GAUGE=$(awk -v p="$PCT_RAW" 'BEGIN {
  v = p + 0
  if (v < 0)   v = 0
  if (v > 100) v = 100
  width = 10
  filled = v / 100.0 * width
  full = int(filled)
  frac = filled - full

  split("▏ ▎ ▍ ▌ ▋ ▊ ▉ █", blk, " ")

  pct_s = sprintf("%d%%", v)
  pct_len = length(pct_s)
  label_pos = full + (frac > 0.0625 ? 1 : 0)
  label_fits = (label_pos + pct_len <= width)

  out = ""
  li = 0  # index into pct_s
  for (i = 0; i < width; i++) {
    if (label_fits && i >= label_pos && li < pct_len) {
      out = out substr(pct_s, li + 1, 1)
      li++
    } else if (i < full) {
      out = out "█"
    } else if (i == full && full < width) {
      idx = int(frac * 8 + 0.5)
      if (idx >= 8)     out = out "█"
      else if (idx > 0) out = out blk[idx]
      else              out = out " "
    } else {
      out = out " "
    }
  }
  printf "%s", out
}')

# Time formatting
HRS=$((DURATION_MS / 3600000))
MINS=$(((DURATION_MS % 3600000) / 60000))
SECS=$(((DURATION_MS % 60000) / 1000))
API_HRS=$((API_MS / 3600000))
API_MINS=$(((API_MS % 3600000) / 60000))
API_SECS=$(((API_MS % 60000) / 1000))

fmt_time() {
  local h=$1 m=$2 s=$3
  if [ "$h" -gt 0 ]; then
    echo "${h}h ${m}m ${s}s"
  elif [ "$m" -gt 0 ]; then
    echo "${m}m ${s}s"
  else echo "${s}s"; fi
}
TIME_FMT=$(fmt_time "$HRS" "$MINS" "$SECS")
API_TIME_FMT=$(fmt_time "$API_HRS" "$API_MINS" "$API_SECS")

# Output tokens/sec
TOK_SEC=$(awk "BEGIN {
  a = $API_MS + 0; t = $OUTPUT_TOKENS + 0
  if (a > 0) printf \"%.1f\", t / (a / 1000)
  else printf \"0.0\"
}")

# Git info
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

# Session delta (appended to last line)
SESSION_DELTA=""
[ "$LINES_ADDED" -gt 0 ] 2>/dev/null && SESSION_DELTA="${SESSION_DELTA}${LIGHTBLUE}+${LINES_ADDED}${RESET} "
[ "$LINES_REMOVED" -gt 0 ] 2>/dev/null && SESSION_DELTA="${SESSION_DELTA}${PINK}-${LINES_REMOVED}${RESET}"
SPLIT_SESSION=""
if [[ $SESSION_DELTA ]]; then SPLIT_SESSION=" | "; fi

# Output
echo -e "${CYAN}[$MODEL]${RESET} 📁 ${DIR##*/}"
if [[ $BRANCH ]]; then
  echo -e "$BRANCH"
fi
COST_FMT=$(printf '$%.2f' "$COST")
BAR_BG="\033[48;5;236m"
BAR_BG_FG="\033[38;5;236m"
LCAP=""
RCAP=""
echo -e "${BAR_BG_FG}${PCT_COLOR}${LCAP}${GAUGE}${RESET}${BAR_BG_FG}${RCAP}${RESET} ${WHITE}${TOKENS_K}${RESET}/${MAX_K}k ${YELLOW}${COST_FMT}${RESET} | ${DIM}${RATE_5HR_COLOR}${RATE_5HR}%${RESET}${DIM}/5hr ${RATE_7D_COLOR}${RATE_7D}%${RESET}${DIM}/week${RESET}"
echo -e "⏱️ ${TIME_FMT} | 🤔 ${API_TIME_FMT} ${DIM}(${TOK_SEC}tok/s)${RESET}${SPLIT_SESSION}${SESSION_DELTA}"
