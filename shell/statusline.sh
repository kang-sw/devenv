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

# ── Why this file avoids subshells ─────────────────────────────────────────
# Every $(...) forks a subprocess. The statusline can re-render several times
# a second while a turn streams, so a render with dozens of substitutions —
# the previous version had ~39, several sitting inside per-pill loops — adds
# up to visible latency even on native Linux, where forking is cheap compared
# to platforms without native fork (e.g. Git Bash/MSYS2, where the same
# rewrite measured 3.4s → 0.54s per render).
#
# This version holds it to 5 external processes (3 on a git cache hit):
# one jq for the payload, one tail|jq for the transcript, one awk for every
# derived number, and two git calls behind a short cache.
#
# When editing, keep to the same rules:
#   - never $(...) what bash can do itself: printf -v, ${var//x/y}, $((...))
#   - new numeric/date formatting goes inside the single awk program
#   - helpers that only build a string must assign to a global, not echo

# stdin. `input=$(cat)` costs a fork and, worse, blocks forever if the
# producer keeps the pipe open. A builtin read with a timeout does neither.
input=''
IFS= read -r -d '' -t 5 input

# Real ESC byte, built once via ANSI-C quoting — never routed through an
# escape-interpreting emitter (echo -e), so backslash sequences that show up
# in dynamic content (e.g. literal "\n" inside a Windows-style directory
# name) can never be misread as control codes.
ESC=$'\033'

# Single jq call to extract all fields (17 → 1 subprocess). Fed by here-string
# rather than `echo "$input" |` so the pipeline does not add a second process.
IFS=$'\x1f' read -r MODEL DIR PROJECT_DIR COST TOKENS_USED CTX_MAX OUTPUT_TOKENS \
  DURATION_MS LINES_ADDED LINES_REMOVED _RATE_5HR RATE_5HR_RESETS \
  RATE_7D_RAW RATE_7D_RESETS CACHE_CREATE CACHE_READ TRANSCRIPT_PATH \
  <<<"$(jq -r '[
  (.model.display_name // ""),
  (.workspace.current_dir // ""),
  (.workspace.project_dir // ""),
  (.cost.total_cost_usd // 0),
  ((.context_window.current_usage | (.input_tokens + .output_tokens + .cache_creation_input_tokens + .cache_read_input_tokens)) // 0),
  (.context_window.context_window_size // 0),
  (.context_window.total_output_tokens // 0),
  (.cost.total_duration_ms // 0),
  (.cost.total_lines_added // 0),
  (.cost.total_lines_removed // 0),
  (.rate_limits.five_hour.used_percentage // 0),
  (.rate_limits.five_hour.resets_at // 0),
  (.rate_limits.seven_day.used_percentage // 0),
  (.rate_limits.seven_day.resets_at // 0),
  (.context_window.current_usage.cache_creation_input_tokens // 0),
  (.context_window.current_usage.cache_read_input_tokens // 0),
  (.transcript_path // "")
] | join([31] | implode)' <<<"$input")"

# ═══════════════════════════════════════════════════════════
# Style parameters — edit these to customize appearance
# ANSI 256-color codes: https://www.ditig.com/256-colors-cheat-sheet
# (Declared before the awk block because it needs RCOL for the progress bar.)
# ═══════════════════════════════════════════════════════════

# Layout
RCOL=70 # Total display width (right edge column)

# Segment backgrounds
MODEL_BG=53        # Model name (purple)
L1_BG=235          # Directory
L_GIT_BG=235       # Git branch
GIT_CHANGES_BG=236 # Git file changes sub-segment
L2_BG=234          # Context progress bar
TOKENS_BG=236      # Token count
RATE_5H_BG=236     # 5h rate limit
RATE_7D_BG=236     # Weekly rate limit
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
OUTPUT_TOK_FG=73 # Output token count (dim cyan)

# Last assistant-turn timestamp (for the "output tokens last updated" pill) —
# read from the transcript itself; no separate state/cache file needed since
# the transcript already records when output_tokens last changed.
#
# Separators are normalized here rather than at the DIR/PROJECT_DIR block far
# below, because this path is consumed immediately and Windows hands it over
# backslash-separated like the other paths in the same payload.
#
# Read forward (tail | jq) rather than reversed (tac | head | jq): tac is absent
# on macOS, and reversing puts the newest line first, so a single partially
# written line — normal while the transcript is being appended to — aborts jq
# before any timestamp is emitted and blanks the pill. The original's closing
# `| tail -n 1` is done by ${var##*$'\n'} instead of a third process.
TRANSCRIPT_PATH="${TRANSCRIPT_PATH//\\//}"
LAST_MSG_ISO=""
if [[ -n $TRANSCRIPT_PATH && -r $TRANSCRIPT_PATH ]]; then
  _ts_all=$(tail -n 30 "$TRANSCRIPT_PATH" 2>/dev/null |
    jq -r 'select(.type == "assistant" and .timestamp != null) | .timestamp' 2>/dev/null)
  LAST_MSG_ISO=${_ts_all##*$'\n'}
fi

# ───────────────────────────────────────────────────────────
# Every derived number, colour and date in one awk program.
# Replaces 13 awk + 9 date + 6 pct_color subshells.
# Fields come back \x1f-separated, in the order listed at the printf.
# Date math (iso_to_local_hm) uses gawk's mktime/strftime, so this assumes
# `awk` resolves to gawk (default on this machine/WSL Ubuntu).
# ───────────────────────────────────────────────────────────
IFS=$'\x1f' read -r TOKENS_USED_FMT CTX_MAX_FMT OUTPUT_TOKENS_FMT PCT CACHE_HIT \
  DELTA_5HR DELTA_7D PCT_COLOR_FWD RATE_5HR_COLOR RATE_7D_COLOR CACHE_HIT_COLOR \
  RATE_5HR RATE_7D RATE_5HR_RESET_FMT RATE_7D_TTL LAST_UPD_ABS BAR \
  <<<"$(awk -v tokens="$TOKENS_USED" -v ctxmax="$CTX_MAX" -v outtok="$OUTPUT_TOKENS" \
           -v cread="$CACHE_READ" -v ccreate="$CACHE_CREATE" \
           -v r5="$_RATE_5HR" -v r5reset="$RATE_5HR_RESETS" \
           -v r7="$RATE_7D_RAW" -v r7reset="$RATE_7D_RESETS" \
           -v nowep="$EPOCHSECONDS" -v iso="$LAST_MSG_ISO" -v rcol="$RCOL" '
function commafy(n,   s, r, l, i) {
  s = sprintf("%d", int(n)); r = ""; l = length(s)
  for (i = 1; i <= l; i++) {
    if (i > 1 && (l - i + 1) % 3 == 0) r = r ","
    r = r substr(s, i, 1)
  }
  return r
}
function fmtmax(v) {
  v = v + 0
  if (v >= 1000000 && v % 1000000 == 0) return sprintf("%dM", v / 1000000)
  if (v >= 1000000)                     return sprintf("%.1fM", v / 1000000)
  if (v >= 1000 && v % 1000 == 0)       return sprintf("%dK", v / 1000)
  return sprintf("%d", v)
}
# Green → yellow → red gradient (ANSI 256-color). mode 38 = fg, 48 = bg.
function pct_color(p, mode,   v, steps, idx) {
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
  return sprintf("\033[%d;5;%dm", mode, steps[idx])
}
# Actual usage vs the linear safe-line (elapsed fraction of window × 100).
# Negative = under budget (good), positive = over budget.
function budget_delta(used, elapsed, window,   d) {
  d = int(used - elapsed / window * 100)
  if (d > 0) return sprintf("+%d%%", d)
  if (d < 0) return sprintf("%d%%", d)
  return ""
}
function make_bar(p, w, label,   v, filled, full, frac, blk, lbl_len, lpos, out, li, i, idx) {
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
  return out
}
# The transcript stamps UTC ("...Z") but mktime() reads local time, so shift by
# the current UTC offset — the same result `date -d "$iso" +%s` produced.
function iso_to_local_hm(s,   off, y, mo, d, h, mi, sec) {
  if (s == "") return ""
  if (s !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]/) return ""
  off = nowep - mktime(strftime("%Y %m %d %H %M %S", nowep, 1))
  y = substr(s, 1, 4); mo = substr(s, 6, 2); d   = substr(s, 9, 2)
  h = substr(s, 12, 2); mi = substr(s, 15, 2); sec = substr(s, 18, 2)
  return strftime("%H:%M", mktime(y " " mo " " d " " h " " mi " " sec) + off)
}
BEGIN {
  SEP = sprintf("%c", 31)

  # Percentage from token counts for decimal precision
  # (API used_percentage is integer-only)
  raw     = (ctxmax > 0) ? tokens / ctxmax * 100 : 0
  pct_raw = sprintf("%.2f", raw) + 0
  pct     = (ctxmax > 0) ? sprintf("%.1f", raw) : "0.0"

  # Cache hit rate: cache_read / (cache_read + cache_creation).
  # High = good, so invert before feeding the green→red gradient.
  cache_hit = ""; cache_hit_color = ""
  ctotal = cread + ccreate
  if (ctotal > 0) {
    cache_hit = sprintf("%.1f", cread / ctotal * 100)
    cache_hit_color = pct_color(sprintf("%.1f", 100 - cache_hit) + 0, 38)
  }

  # Integer part only, matching the shell${var%%.*} truncation this replaces
  r5i = int(r5)
  r7i = int(r7)

  e5 = nowep - (r5reset - 18000)
  if (e5 < 0)     e5 = 0
  if (e5 > 18000) e5 = 18000
  e7 = nowep - (r7reset - 604800)
  if (e7 < 0)      e7 = 0
  if (e7 > 604800) e7 = 604800

  printf "%s", commafy(tokens) SEP fmtmax(ctxmax) SEP commafy(outtok) SEP \
    pct SEP cache_hit SEP \
    budget_delta(r5, e5, 18000) SEP budget_delta(r7, e7, 604800) SEP \
    pct_color(pct_raw, 38) SEP pct_color(r5i, 38) SEP pct_color(r7i, 38) SEP \
    cache_hit_color SEP r5i SEP r7i SEP \
    strftime("%HH", r5reset) SEP strftime("%a", r7reset) SEP \
    iso_to_local_hm(iso) SEP \
    make_bar(pct_raw, rcol - 3, " " pct "%")
}')"

# Delta colors: over budget → red, under budget → green
_DC_5HR=$FG_DIMMER
[[ "$DELTA_5HR" == +* ]] && _DC_5HR=203
[[ "$DELTA_5HR" == -* ]] && _DC_5HR=114
_DC_7D=$FG_DIMMER
[[ "$DELTA_7D" == +* ]] && _DC_7D=203
[[ "$DELTA_7D" == -* ]] && _DC_7D=114

# Time formatting — plain integer arithmetic, no reason to leave the shell
HRS=$((DURATION_MS / 3600000))
MINS=$(((DURATION_MS % 3600000) / 60000))
SECS=$(((DURATION_MS % 60000) / 1000))
if ((HRS > 0)); then
  TIME_FMT="${HRS}h ${MINS}m ${SECS}s"
elif ((MINS > 0)); then
  TIME_FMT="${MINS}m ${SECS}s"
else
  TIME_FMT="${SECS}s"
fi

# Normalize Windows-style separators so basename/relative-path splitting
# below (which only recognizes "/") works on backslash paths too.
DIR="${DIR//\\//}"
PROJECT_DIR="${PROJECT_DIR//\\//}"

# Relative path: current_dir relative to project_dir
DIR_REL=""
if [[ "$DIR" == "$PROJECT_DIR"/* && "$DIR" != "$PROJECT_DIR" ]]; then
  DIR_REL="${DIR#"$PROJECT_DIR"}"
fi

# ── Git info ───────────────────────────────────────────────────────────────
# The two git calls are what is left of the runtime (status and diff each take
# a noticeable fraction on larger repos) and the bar re-renders several times
# a second while a turn streams, so the parsed result is cached per working
# directory for 2s. Several sessions can render at once, so a torn write is
# possible: the cache ends with an OK sentinel and anything without it is
# treated as a miss. Parsing is done with builtins — the grep -c and the
# awk over `git diff --numstat` were two more processes.
BRANCH_NAME=""
GIT_AHEAD=0
GIT_BEHIND=0
GIT_ADDED=0
GIT_DELETED=0
GIT_MODIFIED=0
GIT_UNTRACKED=0

_cache_dir="$HOME/.claude/cache/statusline"
[[ -d $_cache_dir ]] || mkdir -p "$_cache_dir" 2>/dev/null
_cache_file="$_cache_dir/${PWD//[^a-zA-Z0-9]/_}"
_cache_hit=0
if [[ -r $_cache_file ]]; then
  {
    read -r _c_ts && read -r _c_branch && read -r _c_ahead && read -r _c_behind &&
      read -r _c_add && read -r _c_del && read -r _c_mod && read -r _c_unt &&
      read -r _c_end
  } <"$_cache_file" 2>/dev/null
  if [[ $_c_end == OK && $_c_ts =~ ^[0-9]+$ ]] && ((EPOCHSECONDS - _c_ts < 2)); then
    BRANCH_NAME=$_c_branch
    GIT_AHEAD=$_c_ahead
    GIT_BEHIND=$_c_behind
    GIT_ADDED=$_c_add
    GIT_DELETED=$_c_del
    GIT_MODIFIED=$_c_mod
    GIT_UNTRACKED=$_c_unt
    _cache_hit=1
  fi
fi

if ((_cache_hit == 0)); then
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
    while IFS= read -r _l; do
      [[ $_l == '??'* ]] && ((GIT_UNTRACKED++))
    done <<<"$_git_status"
    # numstat writes "-" for binary files; the awk this replaces summed those
    # as 0 and still counted the file, so keep both behaviours.
    while IFS=$'\t' read -r _a _d _f; do
      [[ -n $_f ]] || continue
      [[ $_a == '-' ]] && _a=0
      [[ $_d == '-' ]] && _d=0
      ((GIT_ADDED += _a, GIT_DELETED += _d, GIT_MODIFIED++))
    done <<<"$(git diff --numstat 2>/dev/null)"
    printf '%s\n' "$EPOCHSECONDS" "$BRANCH_NAME" "$GIT_AHEAD" "$GIT_BEHIND" \
      "$GIT_ADDED" "$GIT_DELETED" "$GIT_MODIFIED" "$GIT_UNTRACKED" OK \
      >"$_cache_file" 2>/dev/null
  }
fi

# Pill glyphs (Nerd Font round caps)
LCAP=$'\xee\x82\xb6' # U+E0B6 (left round cap)
RCAP=$'\xee\x82\xb4' # U+E0B4 (right round cap)
printf -v COST_FMT '$%.2f' "$COST"

# ───────────────────────────────────────────────────────────
# Layout engine: build pills, compute widths, pad to RCOL
# Each pill: _PBG<i>=bg  _PC<i>=content  _PW<i>=visible_width
# _layout <N> writes a line padded to RCOL (proportional fill) to _LAYOUT_OUT.
# It assigns instead of echoing: `L1=$(_layout 2)` would fork, and per-pill
# cap helpers that echoed instead of assigning would fork twice more per pill.
# Both are now plain string interpolation.
# ───────────────────────────────────────────────────────────
_layout() {
  local n=$1 total=0 tw=0 i
  for ((i = 0; i < n; i++)); do
    local wv="_PW${i}"
    total=$((total + ${!wv} + 2))
    tw=$((tw + ${!wv}))
  done
  total=$((total + n - 1)) # inter-pill gaps
  local remain=$((RCOL - total))
  [ "$remain" -lt 0 ] && remain=0
  local line="" used=0
  for ((i = 0; i < n; i++)); do
    local bv="_PBG${i}" cv="_PC${i}" wv="_PW${i}"
    local bg=${!bv} c=${!cv} w=${!wv} pad fill
    if [ $i -eq $((n - 1)) ]; then
      pad=$((remain - used))
    elif [ "$tw" -gt 0 ]; then
      pad=$((remain * w / tw))
      used=$((used + pad))
    else
      pad=0
    fi
    fill=""
    [ "$pad" -gt 0 ] && printf -v fill '%*s' "$pad" ''
    [ $i -gt 0 ] && line+=" "
    line+="${ESC}[38;5;${bg}m${LCAP}${ESC}[48;5;${bg}m${c}${fill}${ESC}[0m${ESC}[38;5;${bg}m${RCAP}${ESC}[0m"
  done
  _LAYOUT_OUT="${ESC}[0m${line}${ESC}[0m"
}

# ── Pill content + visible width for each segment ──
# Width formula: count display columns of visible text inside pill
# (emoji 📁🌿🔄 = 2 cols / 1 char → +1; ⌛️ = 2 cols / 2 chars → +0)

# === L1: [Model] [Dir] ===
_PC0="${ESC}[38;5;${FG};1m ${MODEL} ${ESC}[22m"
_PW0=$((${#MODEL} + 2))
_PBG0=$MODEL_BG

_dir_name="${DIR##*/}"
_PC1="${ESC}[38;5;${FG}m 📁 ${_dir_name}"
_PW1=$((5 + ${#_dir_name})) # " 📁(2col) name "
[[ -n $DIR_REL ]] && {
  _PC1+=" ${ESC}[38;5;${FG_DIM}m${DIR_REL}"
  _PW1=$((_PW1 + 1 + ${#DIR_REL}))
}
_PC1+=" "
_PBG1=$L1_BG

_layout 2
L1=$_LAYOUT_OUT

# === L_GIT: [Branch] [Changes] (optional) ===
L_GIT=""
if [[ -n $BRANCH_NAME ]]; then
  _PC0="${ESC}[38;5;${GIT_BRANCH_FG}m 🌿 ${BRANCH_NAME}"
  _PW0=$((5 + ${#BRANCH_NAME})) # " 🌿(2col) branch "
  [ "$GIT_AHEAD" -gt 0 ] 2>/dev/null && {
    _PC0+=" ${ESC}[38;5;${GIT_AHEAD_FG}m↑${GIT_AHEAD}"
    _PW0=$((_PW0 + 2 + ${#GIT_AHEAD}))
  }
  [ "$GIT_BEHIND" -gt 0 ] 2>/dev/null && {
    _PC0+=" ${ESC}[38;5;${GIT_BEHIND_FG}m↓${GIT_BEHIND}"
    _PW0=$((_PW0 + 2 + ${#GIT_BEHIND}))
  }
  _PC0+=" "
  _PBG0=$L_GIT_BG

  _gc="" _gcw=1 # leading space
  [ "$GIT_ADDED" -gt 0 ] 2>/dev/null && {
    _gc+="${ESC}[38;5;${GIT_ADD_FG}m+${GIT_ADDED} "
    _gcw=$((_gcw + 2 + ${#GIT_ADDED}))
  }
  [ "$GIT_DELETED" -gt 0 ] 2>/dev/null && {
    _gc+="${ESC}[38;5;${GIT_DEL_FG}m-${GIT_DELETED} "
    _gcw=$((_gcw + 2 + ${#GIT_DELETED}))
  }
  [ "$GIT_MODIFIED" -gt 0 ] 2>/dev/null && {
    _gc+="${ESC}[38;5;${GIT_MOD_FG}m~${GIT_MODIFIED} "
    _gcw=$((_gcw + 2 + ${#GIT_MODIFIED}))
  }
  [ "$GIT_UNTRACKED" -gt 0 ] 2>/dev/null && {
    _gc+="${ESC}[38;5;${GIT_UNT_FG}m?${GIT_UNTRACKED} "
    _gcw=$((_gcw + 2 + ${#GIT_UNTRACKED}))
  }
  if [[ -n $_gc ]]; then
    _PC1=" ${_gc}"
    _PW1=$_gcw
    _PBG1=$GIT_CHANGES_BG
  else
    _PC1="${ESC}[38;5;${FG_MUTED}m working tree clean "
    _PW1=20
    _PBG1=$L_GIT_BG
  fi

  _layout 2
  L_GIT=$_LAYOUT_OUT
fi

# === L2: Context progress bar (special: gradient-colored left cap) ===
L2="${ESC}[0m${PCT_COLOR_FWD}${LCAP}${ESC}[48;5;${L2_BG}m${BAR} ${ESC}[0m${ESC}[38;5;${L2_BG}m${RCAP}${ESC}[0m${ESC}[0m"

# === L2b: [Tokens] [5h Rate] [7d Rate] [TotalOut] ===
_PC0="${PCT_COLOR_FWD} ${TOKENS_USED_FMT} ${ESC}[38;5;${FG_DIM}m/ ${CTX_MAX_FMT} "
_PW0=$((${#TOKENS_USED_FMT} + ${#CTX_MAX_FMT} + 5)) # " TOK / MAX "
_PBG0=$TOKENS_BG

_PC1=" ${RATE_5HR_COLOR}${RATE_5HR}%${ESC}[38;5;${FG_DIM}m/${ESC}[38;5;${FG}m${RATE_5HR_RESET_FMT}"
_PW1=$((4 + ${#RATE_5HR} + ${#RATE_5HR_RESET_FMT})) # " N%/NNH "
[[ -n $DELTA_5HR ]] && {
  _PC1+=" ${ESC}[38;5;${_DC_5HR}m(${DELTA_5HR})"
  _PW1=$((_PW1 + 3 + ${#DELTA_5HR}))
}
_PC1+=" "
_PBG1=$RATE_5H_BG

_PC2=" ${RATE_7D_COLOR}${RATE_7D}%${ESC}[38;5;${FG_DIM}m/${ESC}[38;5;${FG}m${RATE_7D_TTL}"
_PW2=$((4 + ${#RATE_7D} + ${#RATE_7D_TTL})) # " N%/Day "
[[ -n $DELTA_7D ]] && {
  _PC2+=" ${ESC}[38;5;${_DC_7D}m(${DELTA_7D})"
  _PW2=$((_PW2 + 3 + ${#DELTA_7D}))
}
_PC2+=" "
_PBG2=$RATE_7D_BG

_PC3="${ESC}[38;5;${OUTPUT_TOK_FG}m ↑${OUTPUT_TOKENS_FMT} "
_PW3=$((3 + ${#OUTPUT_TOKENS_FMT})) # " ↑OUT "
_PBG3=$TOKENS_BG

_layout 4
L2b=$_LAYOUT_OUT

# === L3: [Time+Cache] [Last Update] [Delta?] [Cost] ===
_n3=0

_PC0="${ESC}[38;5;${FG}m ⌛️ ${TIME_FMT} "
_PW0=$((5 + ${#TIME_FMT})) # " ⌛️ TIME " (⌛️: 2col/2char → no adj)
if [[ -n $CACHE_HIT ]]; then
  _ch_pct="${CACHE_HIT}%"
  _PC0+="${CACHE_HIT_COLOR}${_ch_pct} "
  _PW0=$((_PW0 + ${#_ch_pct} + 1))
fi
_PBG0=$TIME_BG
_n3=1

_PC1="${ESC}[38;5;${FG}m 🔄 "
if [[ -n $LAST_UPD_ABS ]]; then
  _upd_body="$LAST_UPD_ABS"
  _PC1+="$LAST_UPD_ABS"
else
  _upd_body="--"
  _PC1+="${ESC}[38;5;${FG_DIM}m--"
fi
_PC1+=" "
_PW1=$((5 + ${#_upd_body})) # " 🔄(+1) BODY " (BODY = "HH:MM" or "--")
_PBG1=$API_BG
_n3=2

_dl="" _dlw=1
[ "$LINES_ADDED" -gt 0 ] 2>/dev/null && {
  _dl+="${ESC}[38;5;${LINES_ADD_FG}m+${LINES_ADDED} "
  _dlw=$((_dlw + 2 + ${#LINES_ADDED}))
}
[ "$LINES_REMOVED" -gt 0 ] 2>/dev/null && {
  _dl+="${ESC}[38;5;${LINES_DEL_FG}m-${LINES_REMOVED} "
  _dlw=$((_dlw + 2 + ${#LINES_REMOVED}))
}
if [[ -n $_dl ]]; then
  eval "_PC${_n3}=\" \${_dl}\""
  eval "_PW${_n3}=\$_dlw"
  eval "_PBG${_n3}=\$DELTA_BG"
  _n3=$((_n3 + 1))
fi

eval "_PC${_n3}=\"${ESC}[38;5;\${COST_FG};1m \${COST_FMT} ${ESC}[22m\""
eval "_PW${_n3}=\$((${#COST_FMT} + 2))"
eval "_PBG${_n3}=\$COST_BG"
_n3=$((_n3 + 1))

_layout $_n3
L3=$_LAYOUT_OUT

# Emit
# printf '%s\n' (not echo -e) so backslash sequences inside dynamic content
# (directory names, branch names) are never reinterpreted as escapes — the
# ESC bytes above are already real control characters, not literal text.
printf '%s\n' "$L2"
printf '%s\n' "$L2b"
printf '%s\n' "$L3"
printf '%s\n' "$L1"
if [[ -n $L_GIT ]]; then
  printf '%s\n' "$L_GIT"
fi

# Exit 0 explicitly. The git pill is optional, and writing its emit as
# `[[ -n $L_GIT ]] && printf ...` made the short-circuit the last command in
# the script — so outside a repository the whole statusline exited 1 and the
# client discarded a perfectly good render.
exit 0
