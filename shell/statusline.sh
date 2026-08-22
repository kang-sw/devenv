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

# EPOCHSECONDS is a bash 5 builtin. macOS ships bash 3.2, where it is unset —
# so derive a NOW once from `date` there (one fork on that platform only) and
# use it everywhere below. Linux / Git-Bash keep the fork-free builtin path.
NOW=${EPOCHSECONDS:-$(date +%s)}

# The date pills below lean on gawk's mktime/strftime. macOS ships BSD awk (and
# some Linux boxes ship mawk); both abort the WHOLE program the instant either
# function is called, which blanks every derived field — the "no data on Mac"
# symptom. Probe once: when the functions are missing, the awk program skips the
# time calls (guarded by `hastime`) and the three date strings are filled by
# `date` afterward instead. gawk hosts keep the single-awk, zero-date-fork path.
if awk 'BEGIN { strftime("%H"); mktime("1970 01 01 00 00 00") }' >/dev/null 2>&1; then
  HAS_AWK_TIME=1
else
  HAS_AWK_TIME=0
fi

# Resolve a jq binary. Claude Code runs this statusline in a non-interactive
# shell, which does NOT source ~/.zshrc / ~/.bash_profile — so PATH additions
# that only exist there (e.g. linuxbrew's `brew shellenv`) are absent and a
# bare `jq` fails even though it works in an interactive terminal. Probe the
# common install locations directly, and fall back to a Windows jq.exe on WSL.
if command -v jq >/dev/null 2>&1; then
  JQ=jq
else
  for _c in /home/linuxbrew/.linuxbrew/bin/jq /opt/homebrew/bin/jq \
    /usr/local/bin/jq /usr/bin/jq "$(command -v jq.exe 2>/dev/null)"; do
    if [[ -n $_c && -x $_c ]]; then
      JQ=$_c
      break
    fi
  done
fi

# Single jq call to extract all fields (13 → 1 subprocess). Fed by here-string
# rather than `echo "$input" |` so the pipeline does not add a second process.
IFS=$'\x1f' read -r MODEL DIR PROJECT_DIR COST TOKENS_USED CTX_MAX \
  _RATE_5HR RATE_5HR_RESETS RATE_7D_RAW RATE_7D_RESETS \
  CACHE_CREATE CACHE_READ EFFORT_LEVEL \
  <<<"$("$JQ" -r '[
  (.model.display_name // ""),
  (.workspace.current_dir // ""),
  (.workspace.project_dir // ""),
  (.cost.total_cost_usd // 0),
  ((.context_window.current_usage | (.input_tokens + .output_tokens + .cache_creation_input_tokens + .cache_read_input_tokens)) // 0),
  (.context_window.context_window_size // 0),
  (.rate_limits.five_hour.used_percentage // 0),
  (.rate_limits.five_hour.resets_at // 0),
  (.rate_limits.seven_day.used_percentage // 0),
  (.rate_limits.seven_day.resets_at // 0),
  (.context_window.current_usage.cache_creation_input_tokens // 0),
  (.context_window.current_usage.cache_read_input_tokens // 0),
  (.effort.level // "")
] | join([31] | implode)' <<<"$input")"

# ═══════════════════════════════════════════════════════════
# Style parameters — edit these to customize appearance
# ANSI 256-color codes: https://www.ditig.com/256-colors-cheat-sheet
# ═══════════════════════════════════════════════════════════

# Segment backgrounds
MODEL_BG=53        # Model name (purple)
L1_BG=235          # Directory
L_GIT_BG=235       # Git branch
GIT_CHANGES_BG=236 # Git file changes sub-segment
TOKENS_BG=236      # Token count
RATE_5H_BG=236     # 5h rate limit
RATE_7D_BG=236     # Weekly rate limit
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
COST_FG=184 # Cost (yellow)

# ───────────────────────────────────────────────────────────
# Every derived number and colour in one awk program.
# Fields come back \x1f-separated, in the order listed at the printf.
# Date formatting (reset hour / weekday) uses gawk's strftime; a time-less awk
# (BSD awk, mawk) leaves those two fields empty for the `date` fallback below.
# ───────────────────────────────────────────────────────────
IFS=$'\x1f' read -r TOKENS_USED_FMT CTX_MAX_FMT CACHE_HIT \
  DELTA_5HR DELTA_7D PCT_COLOR_FWD RATE_5HR_COLOR RATE_7D_COLOR CACHE_HIT_COLOR \
  RATE_5HR RATE_7D RATE_5HR_RESET_FMT RATE_7D_TTL \
  <<<"$(awk -v tokens="$TOKENS_USED" -v ctxmax="$CTX_MAX" \
           -v cread="$CACHE_READ" -v ccreate="$CACHE_CREATE" \
           -v r5="$_RATE_5HR" -v r5reset="$RATE_5HR_RESETS" \
           -v r7="$RATE_7D_RAW" -v r7reset="$RATE_7D_RESETS" \
           -v nowep="$NOW" -v hastime="$HAS_AWK_TIME" '
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
BEGIN {
  SEP = sprintf("%c", 31)

  # Percentage from token counts for decimal precision
  # (API used_percentage is integer-only)
  raw     = (ctxmax > 0) ? tokens / ctxmax * 100 : 0
  pct_raw = sprintf("%.2f", raw) + 0

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

  printf "%s", commafy(tokens) SEP fmtmax(ctxmax) SEP cache_hit SEP \
    budget_delta(r5, e5, 18000) SEP budget_delta(r7, e7, 604800) SEP \
    pct_color(pct_raw, 38) SEP pct_color(r5i, 38) SEP pct_color(r7i, 38) SEP \
    cache_hit_color SEP r5i SEP r7i SEP \
    (hastime ? strftime("%HH", r5reset) : "") SEP \
    (hastime ? strftime("%a", r7reset) : "")
}')"

# Fallback for time-function-less awk (macOS BSD awk, mawk): the program left
# RATE_5HR_RESET_FMT / RATE_7D_TTL empty, so fill them with `date`. BSD
# `date -r EPOCH` and GNU `date -d @EPOCH` disagree, so try both. This path
# only forks on platforms without a time-capable awk.
if ((HAS_AWK_TIME == 0)); then
  _fmt_epoch() { # $1=epoch  $2=strftime format
    date -r "$1" "+$2" 2>/dev/null || date -d "@$1" "+$2" 2>/dev/null
  }
  [[ $RATE_5HR_RESETS =~ ^[0-9]+$ ]] && RATE_5HR_RESET_FMT=$(_fmt_epoch "$RATE_5HR_RESETS" '%HH')
  [[ $RATE_7D_RESETS =~ ^[0-9]+$ ]] && RATE_7D_TTL=$(_fmt_epoch "$RATE_7D_RESETS" '%a')
fi

# Delta colors: over budget → red, under budget → green
_DC_5HR=$FG_DIMMER
[[ "$DELTA_5HR" == +* ]] && _DC_5HR=203
[[ "$DELTA_5HR" == -* ]] && _DC_5HR=114
_DC_7D=$FG_DIMMER
[[ "$DELTA_7D" == +* ]] && _DC_7D=203
[[ "$DELTA_7D" == -* ]] && _DC_7D=114

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
  if [[ $_c_end == OK && $_c_ts =~ ^[0-9]+$ ]] && ((NOW - _c_ts < 2)); then
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
    printf '%s\n' "$NOW" "$BRANCH_NAME" "$GIT_AHEAD" "$GIT_BEHIND" \
      "$GIT_ADDED" "$GIT_DELETED" "$GIT_MODIFIED" "$GIT_UNTRACKED" OK \
      >"$_cache_file" 2>/dev/null
  }
fi

# Pill glyphs (Nerd Font round caps)
LCAP=$'\xee\x82\xb6' # U+E0B6 (left round cap)
RCAP=$'\xee\x82\xb4' # U+E0B4 (right round cap)
printf -v COST_FMT '$%.2f' "$COST"

# ───────────────────────────────────────────────────────────
# Layout engine: build pills at their natural width — no alignment/padding.
# Each pill: _PBG<i>=bg  _PC<i>=content. _layout <N> joins them with a single
# space gap and writes the line to _LAYOUT_OUT. It assigns instead of echoing:
# `L1=$(_layout 2)` would fork, and per-pill cap helpers that echoed instead of
# assigning would fork twice more per pill. Both are now plain interpolation.
# ───────────────────────────────────────────────────────────
_layout() {
  local n=$1 i line=""
  for ((i = 0; i < n; i++)); do
    local bv="_PBG${i}" cv="_PC${i}"
    local bg=${!bv} c=${!cv}
    [ $i -gt 0 ] && line+=" "
    line+="${ESC}[38;5;${bg}m${LCAP}${ESC}[48;5;${bg}m${c}${ESC}[0m${ESC}[38;5;${bg}m${RCAP}${ESC}[0m"
  done
  _LAYOUT_OUT="${ESC}[0m${line}${ESC}[0m"
}

# ── Pill content for each segment ──

# === L1: [Model [effort]] [Dir] ===
_PC0="${ESC}[38;5;${FG};1m ${MODEL} ${ESC}[22m"
[[ -n $EFFORT_LEVEL ]] && _PC0+="${ESC}[38;5;${FG_DIM}m[${EFFORT_LEVEL}] "
_PBG0=$MODEL_BG

_dir_name="${DIR##*/}"
_PC1="${ESC}[38;5;${FG}m 📁 ${_dir_name}"
[[ -n $DIR_REL ]] && _PC1+=" ${ESC}[38;5;${FG_DIM}m${DIR_REL}"
_PC1+=" "
_PBG1=$L1_BG

_layout 2
L1=$_LAYOUT_OUT

# === L_GIT: [Branch] [Changes] (optional) ===
L_GIT=""
if [[ -n $BRANCH_NAME ]]; then
  _PC0="${ESC}[38;5;${GIT_BRANCH_FG}m 🌿 ${BRANCH_NAME}"
  [ "$GIT_AHEAD" -gt 0 ] 2>/dev/null && _PC0+=" ${ESC}[38;5;${GIT_AHEAD_FG}m↑${GIT_AHEAD}"
  [ "$GIT_BEHIND" -gt 0 ] 2>/dev/null && _PC0+=" ${ESC}[38;5;${GIT_BEHIND_FG}m↓${GIT_BEHIND}"
  _PC0+=" "
  _PBG0=$L_GIT_BG

  _gc=""
  [ "$GIT_ADDED" -gt 0 ] 2>/dev/null && _gc+="${ESC}[38;5;${GIT_ADD_FG}m+${GIT_ADDED} "
  [ "$GIT_DELETED" -gt 0 ] 2>/dev/null && _gc+="${ESC}[38;5;${GIT_DEL_FG}m-${GIT_DELETED} "
  [ "$GIT_MODIFIED" -gt 0 ] 2>/dev/null && _gc+="${ESC}[38;5;${GIT_MOD_FG}m~${GIT_MODIFIED} "
  [ "$GIT_UNTRACKED" -gt 0 ] 2>/dev/null && _gc+="${ESC}[38;5;${GIT_UNT_FG}m?${GIT_UNTRACKED} "
  if [[ -n $_gc ]]; then
    _PC1=" ${_gc}"
    _PBG1=$GIT_CHANGES_BG
  else
    _PC1="${ESC}[38;5;${FG_MUTED}m working tree clean "
    _PBG1=$L_GIT_BG
  fi

  _layout 2
  L_GIT=$_LAYOUT_OUT
fi

# === L_TOK: [Tokens] [5h Rate] [7d Rate] [Cost + cache-hit] ===
_PC0="${PCT_COLOR_FWD} ${TOKENS_USED_FMT} ${ESC}[38;5;${FG_DIM}m/ ${CTX_MAX_FMT} "
_PBG0=$TOKENS_BG

_PC1=" ${RATE_5HR_COLOR}${RATE_5HR}%${ESC}[38;5;${FG_DIM}m/${ESC}[38;5;${FG}m${RATE_5HR_RESET_FMT}"
[[ -n $DELTA_5HR ]] && _PC1+=" ${ESC}[38;5;${_DC_5HR}m(${DELTA_5HR})"
_PC1+=" "
_PBG1=$RATE_5H_BG

_PC2=" ${RATE_7D_COLOR}${RATE_7D}%${ESC}[38;5;${FG_DIM}m/${ESC}[38;5;${FG}m${RATE_7D_TTL}"
[[ -n $DELTA_7D ]] && _PC2+=" ${ESC}[38;5;${_DC_7D}m(${DELTA_7D})"
_PC2+=" "
_PBG2=$RATE_7D_BG

# Cost, with cache-hit rate tucked in alongside it.
_PC3="${ESC}[38;5;${COST_FG};1m ${COST_FMT} ${ESC}[22m"
[[ -n $CACHE_HIT ]] && _PC3+="${CACHE_HIT_COLOR}${CACHE_HIT}% "
_PBG3=$COST_BG

_layout 4
L_TOK=$_LAYOUT_OUT

# Emit
# printf '%s\n' (not echo -e) so backslash sequences inside dynamic content
# (directory names, branch names) are never reinterpreted as escapes — the
# ESC bytes above are already real control characters, not literal text.
printf '%s\n' "$L_TOK"
printf '%s\n' "$L1"
if [[ -n $L_GIT ]]; then
  printf '%s\n' "$L_GIT"
fi

# Exit 0 explicitly. The git pill is optional, and writing its emit as
# `[[ -n $L_GIT ]] && printf ...` made the short-circuit the last command in
# the script — so outside a repository the whole statusline exited 1 and the
# client discarded a perfectly good render.
exit 0
