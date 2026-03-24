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

# Green → yellow → red gradient (ANSI 256-color, fg only)
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

# Context gauge — 10 chars using block characters
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

# Git info (raw data for powerline segments)
BRANCH_NAME=""
GIT_AHEAD=0
GIT_BEHIND=0
GIT_ADDED=0
GIT_DELETED=0
GIT_MODIFIED=0
GIT_UNTRACKED=0
if git rev-parse --git-dir >/dev/null 2>&1; then
  BRANCH_NAME=$(git branch --show-current 2>/dev/null)
  DIFF_STAT=$(git diff --numstat 2>/dev/null | awk '{a+=$1; d+=$2} END {printf "%d %d", a+0, d+0}')
  GIT_ADDED=$(echo "$DIFF_STAT" | cut -d' ' -f1)
  GIT_DELETED=$(echo "$DIFF_STAT" | cut -d' ' -f2)
  GIT_MODIFIED=$(git diff --name-only 2>/dev/null | wc -l | tr -d ' ')
  GIT_UNTRACKED=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')
  if git rev-parse --verify "@{u}" >/dev/null 2>&1; then
    GIT_AHEAD=$(git rev-list --count "@{u}..HEAD" 2>/dev/null || echo 0)
    GIT_BEHIND=$(git rev-list --count "HEAD..@{u}" 2>/dev/null || echo 0)
  fi
fi

# Powerline glyphs & Nerd Font icons (hex bytes for bash 3.2 compat)
SEP=$'\xee\x82\xb0'         # U+E0B0
LCAP=$'\xee\x82\xb6'        # U+E0B6
RCAP=$'\xee\x82\xb4'        # U+E0B4
ICON_DIR=$'\xef\x81\xbc'    # U+F07C
ICON_BRANCH=$'\xee\x82\xa0' # U+E0A0
ICON_GIT=$'\xee\x9c\x82'    # U+E702
ICON_CLOCK=$'\xef\x80\x97'  # U+F017
ICON_BOLT=$'\xef\x83\xa7'   # U+F0E7
COST_FMT=$(printf '$%.2f' "$COST")

# === Line 1: Model → Dir ===
L1="\033[38;5;53m${LCAP}"
L1+="\033[48;5;53;38;5;255;1m ${MODEL} \033[22m"
L1+="\033[48;5;239;38;5;53m${SEP}"
L1+="\033[48;5;239;38;5;255m 📁 ${DIR##*/} "
L1+="\033[0m\033[38;5;239m${RCAP}\033[0m"

# === Line 2: Git (optional) ===
L_GIT=""
if [[ -n $BRANCH_NAME ]]; then
  L_GIT="\033[38;5;237m${LCAP}"
  L_GIT+="\033[48;5;237;38;5;202m \033[38;5;114m🌿 ${BRANCH_NAME}"
  [ "$GIT_AHEAD" -gt 0 ] 2>/dev/null && L_GIT+=" \033[38;5;114m↑${GIT_AHEAD}"
  [ "$GIT_BEHIND" -gt 0 ] 2>/dev/null && L_GIT+=" \033[38;5;214m↓${GIT_BEHIND}"
  L_GIT+=" "
  _gc=""
  [ "$GIT_ADDED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;114m+${GIT_ADDED} "
  [ "$GIT_DELETED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;203m-${GIT_DELETED} "
  [ "$GIT_MODIFIED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;214m~${GIT_MODIFIED} "
  [ "$GIT_UNTRACKED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;75m?${GIT_UNTRACKED} "
  if [[ -n $_gc ]]; then
    L_GIT+="\033[48;5;235;38;5;237m${SEP}\033[48;5;235m ${_gc}\033[0m\033[38;5;235m${RCAP}"
  else
    L_GIT+="\033[0m\033[38;5;237m${RCAP}"
  fi
  L_GIT+="\033[0m"
fi

# === Line 2: Gauge capsule + Tokens → Cost → Rates ===
BAR_FG="\033[38;5;236m"
L2="${BAR_FG}${PCT_COLOR}${LCAP}${GAUGE}\033[0m${BAR_FG}${RCAP}"
L2+=" \033[38;5;239m${LCAP}"
L2+="\033[48;5;239;38;5;255m ${TOKENS_K}/${MAX_K}k "
L2+="\033[48;5;237;38;5;239m${SEP}"
L2+="\033[48;5;237;38;5;214m ${COST_FMT} "
L2+="\033[48;5;235;38;5;237m${SEP}\033[48;5;235m "
L2+="${RATE_5HR_COLOR}${RATE_5HR}%\033[48;5;235;38;5;245m/5h "
L2+="${RATE_7D_COLOR}${RATE_7D}%\033[48;5;235;38;5;245m/wk "
L2+="\033[0m\033[38;5;235m${RCAP}\033[0m"

# === Line 3: Time → API → Delta ===
L3="\033[38;5;239m${LCAP}"
L3+="\033[48;5;239;38;5;255m ${ICON_CLOCK} ${TIME_FMT} "
L3+="\033[48;5;237;38;5;239m${SEP}"
L3+="\033[48;5;237;38;5;255m ${ICON_BOLT} ${API_TIME_FMT} \033[38;5;245m${TOK_SEC}t/s "
_dl=""
[ "$LINES_ADDED" -gt 0 ] 2>/dev/null && _dl+="\033[38;5;75m+${LINES_ADDED} "
[ "$LINES_REMOVED" -gt 0 ] 2>/dev/null && _dl+="\033[38;5;204m-${LINES_REMOVED} "
if [[ -n $_dl ]]; then
  L3+="\033[48;5;235;38;5;237m${SEP}\033[48;5;235m ${_dl}\033[0m\033[38;5;235m${RCAP}"
else
  L3+="\033[0m\033[38;5;237m${RCAP}"
fi
L3+="\033[0m"

echo -e "$L1"
[[ -n $L_GIT ]] && echo -e "$L_GIT"
echo -e "$L2"
echo -e "$L3"
