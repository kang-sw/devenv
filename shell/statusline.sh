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

# Single jq call to extract all fields (15 → 1 subprocess)
IFS=$'\x1f' read -r MODEL DIR PROJECT_DIR COST TOKENS_USED CTX_MAX OUTPUT_TOKENS \
  DURATION_MS API_MS LINES_ADDED LINES_REMOVED _RATE_5HR RATE_5HR_RESETS \
  RATE_7D_RAW RATE_7D_RESETS <<<"$(echo "$input" | jq -r '[
  (.model.display_name // ""),
  (.workspace.current_dir // ""),
  (.workspace.project_dir // ""),
  (.cost.total_cost_usd // 0),
  ((.context_window.current_usage | (.input_tokens + .output_tokens + .cache_creation_input_tokens + .cache_read_input_tokens)) // 0),
  (.context_window.context_window_size // 0),
  (.context_window.total_output_tokens // 0),
  (.cost.total_duration_ms // 0),
  (.cost.total_api_duration_ms // 0),
  (.cost.total_lines_added // 0),
  (.cost.total_lines_removed // 0),
  (.rate_limits.five_hour.used_percentage // 0),
  (.rate_limits.five_hour.resets_at // 0),
  (.rate_limits.seven_day.used_percentage // 0),
  (.rate_limits.seven_day.resets_at // 0)
] | join("\u001f")')"
RATE_5HR=${_RATE_5HR%%.*}
RATE_7D=${RATE_7D_RAW%%.*}
TOKENS_USED_FMT=$(awk "BEGIN {
  s = sprintf(\"%d\", int($TOKENS_USED)); r = \"\"; l = length(s)
  for (i = 1; i <= l; i++) {
    if (i > 1 && (l - i + 1) % 3 == 0) r = r \",\"
    r = r substr(s, i, 1)
  }
  printf \"%s\", r
}")
CTX_MAX_FMT=$(awk "BEGIN {
  v = $CTX_MAX + 0
  if (v >= 1000000 && v % 1000000 == 0) printf \"%dM\", v / 1000000
  else if (v >= 1000000) printf \"%.1fM\", v / 1000000
  else if (v >= 1000 && v % 1000 == 0) printf \"%dK\", v / 1000
  else printf \"%d\", v
}")
# Compute percentage from token counts for decimal precision
# (API used_percentage is integer-only)
PCT_RAW=$(awk "BEGIN { if ($CTX_MAX > 0) printf \"%.2f\", $TOKENS_USED / $CTX_MAX * 100; else print 0 }")
PCT=$(awk "BEGIN { if ($CTX_MAX > 0) printf \"%.1f\", $TOKENS_USED / $CTX_MAX * 100; else print \"0.0\" }")

# ═══════════════════════════════════════════════════════════
# Style parameters — edit these to customize appearance
# ANSI 256-color codes: https://www.ditig.com/256-colors-cheat-sheet
# ═══════════════════════════════════════════════════════════

# Layout
RCOL=70 # Total display width (right edge column)

# Segment backgrounds
MODEL_BG=53        # Model name (purple)
L1_BG=236          # Directory
L_GIT_BG=235       # Git branch
GIT_CHANGES_BG=235 # Git file changes sub-segment
L2_BG=233          # Context progress bar
TOKENS_BG=236      # Token count
RATE_5H_BG=236     # 5h rate limit
RATE_7D_BG=236     # Weekly rate limit
L2b_BG=$RATE_7D_BG # (unused in pills layout, kept for reference)
TIME_BG=235        # Wall-clock time
API_BG=235         # API time
DELTA_BG=235       # Lines-changed delta
COST_BG=53         # Cost

# Foreground colors
FG=255        # Primary text (white)
FG_DIM=245    # Labels / secondary
FG_DIMMER=243 # Annotations
FG_MUTED=242  # Muted ("working tree clean")

# Git status
GIT_BRANCH_FG=114 # Branch name (green)
GIT_AHEAD_FG=114  # Ahead count
GIT_BEHIND_FG=214 # Behind count (yellow)
GIT_ADD_FG=114    # Added
GIT_DEL_FG=203    # Deleted (red)
GIT_MOD_FG=214    # Modified
GIT_UNT_FG=75     # Untracked (blue)

# Accents
COST_FG=184      # Cost (yellow)
LINES_ADD_FG=75  # Lines added (blue)
LINES_DEL_FG=204 # Lines removed (pink)
CAP_BG=53        # (unused in pills layout, kept for reference)

# Rate limit budget deltas (actual usage vs linear safe-line)
# Safe-line = elapsed fraction of window × 100
# Negative = under budget (good), positive = over budget
_NOW_EPOCH=$(date +%s)

_5H_ELAPSED=$((_NOW_EPOCH - (RATE_5HR_RESETS - 18000)))
[ "$_5H_ELAPSED" -lt 0 ] && _5H_ELAPSED=0
[ "$_5H_ELAPSED" -gt 18000 ] && _5H_ELAPSED=18000
DELTA_5HR=$(awk "BEGIN {
  d = int($_RATE_5HR - $_5H_ELAPSED / 18000.0 * 100)
  if (d > 0)      printf \"+%d%%\", d
  else if (d < 0) printf \"%d%%\", d
}")

_7D_ELAPSED=$((_NOW_EPOCH - (RATE_7D_RESETS - 604800)))
[ "$_7D_ELAPSED" -lt 0 ] && _7D_ELAPSED=0
[ "$_7D_ELAPSED" -gt 604800 ] && _7D_ELAPSED=604800
DELTA_7D=$(awk "BEGIN {
  d = int($RATE_7D_RAW - $_7D_ELAPSED / 604800.0 * 100)
  if (d > 0)      printf \"+%d%%\", d
  else if (d < 0) printf \"%d%%\", d
}")

# Delta colors: over budget → red, under budget → green
_DC_5HR=$FG_DIMMER
[[ "$DELTA_5HR" == +* ]] && _DC_5HR=203
[[ "$DELTA_5HR" == -* ]] && _DC_5HR=114
_DC_7D=$FG_DIMMER
[[ "$DELTA_7D" == +* ]] && _DC_7D=203
[[ "$DELTA_7D" == -* ]] && _DC_7D=114

# Green → yellow → red gradient (ANSI 256-color)
# Usage: pct_color <percent> [48]  — default fg (38), pass 48 for bg
pct_color() {
  local mode="${2:-38}"
  awk -v p="$1" -v m="$mode" 'BEGIN {
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
    printf "\033[%d;5;%dm", m, steps[idx]
  }'
}
PCT_COLOR="\033[48;5;${L2_BG}m$(pct_color "$PCT_RAW")"
PCT_COLOR_FWD="$(pct_color "$PCT_RAW")"
PCT_COLOR_BG="$(pct_color "$PCT_RAW" 48)"
RATE_5HR_COLOR=$(pct_color "$RATE_5HR")
RATE_7D_COLOR=$(pct_color "$RATE_7D")

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

# 7d rate limit reset weekday
RATE_7D_TTL=$(date -r "$RATE_7D_RESETS" "+%a" 2>/dev/null || echo "??")

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

# Git info — consolidated (8 → 2 subprocesses via git status + git diff)
BRANCH_NAME=""
GIT_AHEAD=0
GIT_BEHIND=0
GIT_ADDED=0
GIT_DELETED=0
GIT_MODIFIED=0
GIT_UNTRACKED=0
_git_status=$(git status --porcelain -b 2>/dev/null) && {
  # Header: ## branch...origin/branch [ahead N, behind M]
  _git_header="${_git_status%%$'\n'*}"
  BRANCH_NAME="${_git_header#\#\# }"
  BRANCH_NAME="${BRANCH_NAME%%...*}"
  case "$BRANCH_NAME" in
  "HEAD (no branch)"* | "No commits yet"* | "Initial commit"*) BRANCH_NAME="" ;;
  esac
  [[ "$_git_header" =~ ahead\ ([0-9]+) ]] && GIT_AHEAD=${BASH_REMATCH[1]}
  [[ "$_git_header" =~ behind\ ([0-9]+) ]] && GIT_BEHIND=${BASH_REMATCH[1]}
  GIT_UNTRACKED=$(echo "$_git_status" | grep -c '^??')
  # Line counts + modified file count from diff --numstat
  _diff=$(git diff --numstat 2>/dev/null | awk '{a+=$1; d+=$2; n++} END {print a+0, d+0, n+0}')
  GIT_ADDED=${_diff%% *}
  _diff="${_diff#* }"
  GIT_DELETED=${_diff%% *}
  GIT_MODIFIED=${_diff##* }
}

# Pill glyphs (Nerd Font round caps)
LCAP=$'\xee\x82\xb6' # U+E0B6 (left round cap)
RCAP=$'\xee\x82\xb4' # U+E0B4 (right round cap)
COST_FMT=$(printf '$%.2f' "$COST")

# Pill helpers — open/close a colored pill segment
# Usage: $(po BG) content $(pc BG)  with a space between pills
po() { printf "\033[38;5;%dm${LCAP}\033[48;5;%dm" "$1" "$1"; }
pc() { printf "\033[0m\033[38;5;%dm${RCAP}\033[0m" "$1"; }

# === Line 1: [Model] [Dir] ===
L1="$(po $MODEL_BG)\033[38;5;${FG};1m ${MODEL} \033[22m$(pc $MODEL_BG) "
L1+="$(po $L1_BG)\033[38;5;${FG}m 📁 ${DIR##*/}"
[[ -n $DIR_REL ]] && L1+=" \033[38;5;${FG_DIM}m${DIR_REL}"
L1+=" $(pc $L1_BG)"

# === Line 2: [Git branch] [changes] (optional) ===
L_GIT=""
if [[ -n $BRANCH_NAME ]]; then
  L_GIT="$(po $L_GIT_BG)\033[38;5;${GIT_BRANCH_FG}m 🌿 ${BRANCH_NAME}"
  [ "$GIT_AHEAD" -gt 0 ] 2>/dev/null && L_GIT+=" \033[38;5;${GIT_AHEAD_FG}m↑${GIT_AHEAD}"
  [ "$GIT_BEHIND" -gt 0 ] 2>/dev/null && L_GIT+=" \033[38;5;${GIT_BEHIND_FG}m↓${GIT_BEHIND}"
  L_GIT+=" $(pc $L_GIT_BG) "
  _gc=""
  [ "$GIT_ADDED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;${GIT_ADD_FG}m+${GIT_ADDED} "
  [ "$GIT_DELETED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;${GIT_DEL_FG}m-${GIT_DELETED} "
  [ "$GIT_MODIFIED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;${GIT_MOD_FG}m~${GIT_MODIFIED} "
  [ "$GIT_UNTRACKED" -gt 0 ] 2>/dev/null && _gc+="\033[38;5;${GIT_UNT_FG}m?${GIT_UNTRACKED} "
  if [[ -n $_gc ]]; then
    L_GIT+="$(po $GIT_CHANGES_BG) ${_gc}$(pc $GIT_CHANGES_BG)"
  else
    L_GIT+="$(po $L_GIT_BG)\033[38;5;${FG_MUTED}m working tree clean $(pc $L_GIT_BG)"
  fi
fi

# === Context progress bar ===
BAR_WIDTH=$((RCOL - 6))
BAR_LABEL=" ${PCT}%"
BAR=$(awk -v p="$PCT_RAW" -v w="$BAR_WIDTH" -v label="$BAR_LABEL" 'BEGIN {
  v = p + 0
  if (v < 0)   v = 0
  if (v > 100) v = 100
  filled = v / 100.0 * w
  full = int(filled)
  frac = filled - full
  split("▏ ▎ ▍ ▌ ▋ ▊ ▉ █", blk, " ")
  lbl_len = length(label)
  lpos = full + (frac > 0.0625 ? 1 : 0)
  if (lpos + lbl_len > w) lpos = w - lbl_len
  if (lpos < 0) lpos = 0
  out = ""
  li = 0
  for (i = 0; i < w; i++) {
    if (i >= lpos && li < lbl_len) {
      out = out substr(label, li + 1, 1)
      li++
    } else if (i < full) {
      out = out "█"
    } else if (i == full && full < w) {
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
L2="${PCT_COLOR_FWD}${LCAP}\033[48;5;${L2_BG}m${BAR} $(pc $L2_BG)"

# === Tokens / Rate limits ===
L2b="$(po $TOKENS_BG)${PCT_COLOR_FWD} ${TOKENS_USED_FMT} \033[38;5;${FG_DIM}m/ ${CTX_MAX_FMT} $(pc $TOKENS_BG) "
L2b+="$(po $RATE_5H_BG) ${RATE_5HR_COLOR}${RATE_5HR}%\033[38;5;${FG_DIM}m/5h/\033[38;5;${FG}m${RATE_5HR_RESET_FMT}"
[[ -n $DELTA_5HR ]] && L2b+=" \033[38;5;${_DC_5HR}m(${DELTA_5HR})"
L2b+=" $(pc $RATE_5H_BG) "
L2b+="$(po $RATE_7D_BG) ${RATE_7D_COLOR}${RATE_7D}%\033[38;5;${FG_DIM}m/wk/\033[38;5;${FG}m${RATE_7D_TTL}"
[[ -n $DELTA_7D ]] && L2b+=" \033[38;5;${_DC_7D}m(${DELTA_7D})"
L2b+=" $(pc $RATE_7D_BG)"

# === Time / API / Delta / Cost ===
L3="$(po $TIME_BG)\033[38;5;${FG}m ⌛️ ${TIME_FMT} $(pc $TIME_BG) "
L3+="$(po $API_BG)\033[38;5;${FG}m 🤔 ${API_TIME_FMT} \033[38;5;${FG_DIM}m${TOK_SEC}t/s $(pc $API_BG)"
_dl=""
[ "$LINES_ADDED" -gt 0 ] 2>/dev/null && _dl+="\033[38;5;${LINES_ADD_FG}m+${LINES_ADDED} "
[ "$LINES_REMOVED" -gt 0 ] 2>/dev/null && _dl+="\033[38;5;${LINES_DEL_FG}m-${LINES_REMOVED} "
[[ -n $_dl ]] && L3+=" $(po $DELTA_BG) ${_dl}$(pc $DELTA_BG)"
L3+=" $(po $COST_BG)\033[38;5;${COST_FG};1m ${COST_FMT} \033[22m$(pc $COST_BG)"

# Emit lines
_line() { echo -e "\033[0m$1\033[0m"; }

if [[ -n $L_GIT ]]; then
  _line "$L1"
  _line "$L_GIT"
  _line "$L2"
  _line "$L2b"
  _line "$L3"
else
  _line "$L1"
  _line "$L2"
  _line "$L2b"
  _line "$L3"
fi
