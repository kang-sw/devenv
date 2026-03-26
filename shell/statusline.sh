#!/bin/bash
# Claude Code statusline — available JSON fields (v2.1.83):
#
# session_id                                       string   session UUID
# transcript_path                                  string   path to .jsonl transcript
# cwd                                              string   current working directory
# model.id                                         string   e.g. "claude-opus-4-6[1m]"
# model.display_name                               string   e.g. "Opus 4.6 (1M context)"
# workspace.current_dir                            string
# workspace.project_dir                            string
# version                                          string   CLI version
# output_style.name                                string   e.g. "default"
# cost.total_cost_usd                              float    cumulative session cost
# cost.total_duration_ms                           int      wall-clock time
# cost.total_api_duration_ms                       int      API round-trip time
# cost.total_lines_added                           int
# cost.total_lines_removed                         int
# context_window.total_input_tokens                int      cumulative input tokens
# context_window.total_output_tokens               int      cumulative output tokens
# context_window.context_window_size               int      max context (e.g. 1000000)
# context_window.current_usage.input_tokens        int      current turn input
# context_window.current_usage.output_tokens       int      current turn output
# context_window.current_usage.cache_creation_input_tokens  int
# context_window.current_usage.cache_read_input_tokens      int
# context_window.used_percentage                   int      context % used
# context_window.remaining_percentage              int
# rate_limits.five_hour.used_percentage            float    5h rate limit %
# rate_limits.five_hour.resets_at                  int      epoch seconds
# rate_limits.seven_day.used_percentage            float/int  weekly rate limit %
# rate_limits.seven_day.resets_at                  int      epoch seconds

input=$(cat)

MODEL=$(echo "$input" | jq -r '.model.display_name')
DIR=$(echo "$input" | jq -r '.workspace.current_dir')
PROJECT_DIR=$(echo "$input" | jq -r '.workspace.project_dir')
COST=$(echo "$input" | jq -r '.cost.total_cost_usd // 0')
PCT_RAW=$(echo "$input" | jq -r '.context_window.used_percentage // 0')
PCT=$(echo "$PCT_RAW" | awk '{printf "%d", $1}')
TOKENS_USED=$(echo "$input" | jq -r '.context_window.current_usage | (.input_tokens + .output_tokens + .cache_creation_input_tokens + .cache_read_input_tokens)')
TOKENS_USED_FMT=$(awk "BEGIN {
  s = sprintf(\"%d\", int($TOKENS_USED)); r = \"\"; l = length(s)
  for (i = 1; i <= l; i++) {
    if (i > 1 && (l - i + 1) % 3 == 0) r = r \",\"
    r = r substr(s, i, 1)
  }
  printf \"%s\", r
}")
CTX_MAX=$(echo "$input" | jq -r '.context_window.context_window_size // 0')
OUTPUT_TOKENS=$(echo "$input" | jq -r '.context_window.total_output_tokens // 0')
DURATION_MS=$(echo "$input" | jq -r '.cost.total_duration_ms // 0')
API_MS=$(echo "$input" | jq -r '.cost.total_api_duration_ms // 0')
LINES_ADDED=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
LINES_REMOVED=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')
RATE_5HR=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // 0' | awk '{printf "%d", $1}')
RATE_5HR_RESETS=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // 0')
RATE_7D_RAW=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // 0')
RATE_7D_RESETS=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // 0')
RATE_7D=$(echo "$RATE_7D_RAW" | awk '{printf "%d", $1}')

# Weekly rate daily delta tracking via shared state file
_WK_STATE="/tmp/claude-statusline-weekly-${USER}"
_WK_TODAY=$(date +%Y-%m-%d)
_WK_NOW=$(date +%s)
_WK_PIVOT_RATE="$RATE_7D_RAW"
DELTA_7D=""

if [[ -f "$_WK_STATE" ]]; then
  _WK_SAVED_DATE=$(cut -d' ' -f1 <"$_WK_STATE")
  _WK_SAVED_PIVOT=$(cut -d' ' -f2 <"$_WK_STATE")
  _WK_SAVED_LAST=$(cut -d' ' -f3 <"$_WK_STATE")
  if [[ "$_WK_SAVED_DATE" != "$_WK_TODAY" ]]; then
    # Date changed — pivot from the last known fresh value
    _WK_PIVOT_RATE="${_WK_SAVED_LAST:-$RATE_7D_RAW}"
  else
    _WK_PIVOT_RATE="${_WK_SAVED_PIVOT:-$RATE_7D_RAW}"
  fi
fi
echo "$_WK_TODAY $_WK_PIVOT_RATE $RATE_7D_RAW $_WK_NOW" >"$_WK_STATE"

DELTA_7D=$(awk "BEGIN {
  d = int($RATE_7D_RAW - $_WK_PIVOT_RATE)
  if (d > 0)      printf \"+%d%%\", d
  else if (d < 0) printf \"%d%%\", d
}")

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
PCT_COLOR_FWD="$(pct_color "$PCT_RAW")"
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

# 5h rate limit reset time (HH:MM)
RATE_5HR_RESET_FMT=$(date -r "$RATE_5HR_RESETS" "+%HH" 2>/dev/null || echo "??")

# 7d rate limit reset remaining (Nd Nh)
_NOW_EPOCH=$(date +%s)
_7D_REMAIN_S=$((RATE_7D_RESETS - _NOW_EPOCH))
[ "$_7D_REMAIN_S" -lt 0 ] 2>/dev/null && _7D_REMAIN_S=0
_7D_RD=$((_7D_REMAIN_S / 86400))
_7D_RH=$(((_7D_REMAIN_S % 86400) / 3600))
RATE_7D_TTL="~"
[ "$_7D_RD" -gt 0 ] && RATE_7D_TTL+="${_7D_RD}d"
RATE_7D_TTL+="${_7D_RH}h"

# Relative path: current_dir relative to project_dir
DIR_REL=""
if [[ "$DIR" == "$PROJECT_DIR"/* && "$DIR" != "$PROJECT_DIR" ]]; then
  DIR_REL="${DIR#"$PROJECT_DIR"}"
fi

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
SEP=$'\xee\x82\xb8'         # U+E0B8 (lower-left diagonal, / angle)
LCAP=$'\xee\x82\xb6'        # U+E0B6
RCAP=$'\xee\x82\xb4'        # U+E0B4
DIAG=$'\xee\x82\xbe'        # U+E0BE (upper-right diagonal, / left cap)
RDIAG=$'\xee\x82\xb8'       # U+E0B8 (lower-left diagonal, / right cap)
ICON_DIR=$'\xef\x81\xbc'    # U+F07C
ICON_BRANCH=$'\xee\x82\xa0' # U+E0A0
ICON_GIT=$'\xee\x9c\x82'    # U+E702
ICON_CLOCK=$'\xef\x80\x97'  # U+F017
ICON_BOLT=$'\xef\x83\xa7'   # U+F0E7
COST_FMT=$(printf '$%.2f' "$COST")

# === Line 1: Model → Dir ===
L1="\033[38;5;53m${DIAG}"
L1+="\033[48;5;53;38;5;255;1m ${MODEL} \033[22m"
L1+="\033[48;5;237;38;5;53m${SEP}"
L1+="\033[48;5;237;38;5;255m 📁 ${DIR##*/}"
[[ -n $DIR_REL ]] && L1+=" \033[38;5;245m${DIR_REL}"
L1+=" "
L1_BG=237

# === Line 2: Git (optional) ===
L_GIT=""
L_GIT_BG=237
if [[ -n $BRANCH_NAME ]]; then
  L_GIT="\033[38;5;237m${DIAG}"
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
    L_GIT+="\033[48;5;235;38;5;237m${SEP}\033[48;5;235m ${_gc}"
    L_GIT_BG=235
  else
    L_GIT+="\033[38;5;242m working tree clean "
  fi
fi

# === Line 3: Gauge + Tokens → 5h Rate → Weekly Rate ===
L2="${PCT_COLOR_FWD}${DIAG}${PCT_COLOR}${GAUGE}\033[0m"
L2+="\033[48;5;235;38;5;236m${SEP}"
# L2+="\033[48;5;237;38;5;255m ${TOKENS_USED} "
L2+="${PCT_COLOR}\033[48;5;235m ${TOKENS_USED_FMT} "
L2+="\033[48;5;233;38;5;235m${SEP}"
L2+="\033[48;5;233m ${RATE_5HR_COLOR}${RATE_5HR}%\033[48;5;233;38;5;245m/5h/\033[38;5;255m${RATE_5HR_RESET_FMT} "
L2+="\033[48;5;235;38;5;233m${SEP}"
L2+="\033[48;5;235m ${RATE_7D_COLOR}${RATE_7D}%\033[48;5;235;38;5;245m/wk/\033[38;5;255m${RATE_7D_TTL}"
[[ -n $DELTA_7D ]] && L2+=" \033[38;5;243m(${DELTA_7D})"
L2+=" "
L2_BG=235

# === Line 4: Time → API → Delta ===
L3="\033[38;5;236m${DIAG}"
L3+="\033[48;5;236;38;5;255m ⌛️ ${TIME_FMT} "
L3+="\033[48;5;237;38;5;236m${SEP}"
L3+="\033[48;5;237;38;5;255m 🤔 ${API_TIME_FMT} \033[38;5;245m${TOK_SEC}t/s "
L3_BG=237
_dl=""
[ "$LINES_ADDED" -gt 0 ] 2>/dev/null && _dl+="\033[38;5;75m+${LINES_ADDED} "
[ "$LINES_REMOVED" -gt 0 ] 2>/dev/null && _dl+="\033[38;5;204m-${LINES_REMOVED} "
if [[ -n $_dl ]]; then
  L3+="\033[48;5;235;38;5;237m${SEP}\033[48;5;235m ${_dl}"
  L3_BG=235
fi
L3+="\033[48;5;236;38;5;${L3_BG}m${SEP}"
L3+="\033[48;5;236;38;5;214m ${COST_FMT} "
L3_BG=236

# Emit line: indent + content (bg active) + space padding + right diagonal cap
RCOL=70
_emit() {
  local i=$1 line=$2 bg=$3 rcol=$4
  local pad=$(printf '%*s' "$i" '')
  local full="\033[0m${pad}${line}"
  # Measure visible width (strip ANSI escapes, count chars)
  local stripped=$(echo -ne "$full" | sed $'s/\x1b\[[0-9;]*m//g')
  local w=${#stripped}
  # Adjust for wide emojis (2 display cols but 1 char)
  [[ "$stripped" == *📁* ]] && ((w++))
  [[ "$stripped" == *🌿* ]] && ((w++))
  [[ "$stripped" == *🤔* ]] && ((w++))
  # Pad with spaces (inherits last segment bg) to reach right cap
  local need=$((rcol - 1 - w))
  local fill=""
  [ "$need" -gt 0 ] && fill=$(printf '%*s' "$need" '')
  echo -e "${full}${fill}\033[0m\033[38;5;${bg}m${RDIAG}\033[0m"
}

if [[ -n $L_GIT ]]; then
  _emit 0 "$L1" $L1_BG $((RCOL - 3))
  _emit 1 "$L_GIT" $L_GIT_BG $((RCOL - 2))
  _emit 2 "$L2" $L2_BG $((RCOL - 1))
  _emit 3 "$L3" $L3_BG $((RCOL))
else
  _emit 0 "$L1" $L1_BG $((RCOL - 2))
  _emit 1 "$L2" $L2_BG $((RCOL - 1))
  _emit 2 "$L3" $L3_BG $((RCOL))
fi
