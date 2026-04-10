#!/usr/bin/env bash
# Dispatcher for /enter-session. Emits the fast-path resume body plus
# the continuation payload when ai-docs/_continue.local.md exists and
# its recorded HEAD SHA equals current HEAD; otherwise emits the
# bootstrap body. Non-destructive: never deletes the continuation file.
#
# Invoked from SKILL.md via `!`bash "${CLAUDE_SKILL_DIR}/dispatch.sh"``.

set -u

skill_dir="${CLAUDE_SKILL_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
continue_file="ai-docs/_continue.local.md"

emit_bootstrap() {
  cat "${skill_dir}/bootstrap.md"
}

if [ ! -f "${continue_file}" ]; then
  emit_bootstrap
  exit 0
fi

stored=$(sed -n '1s/.*HEAD: \([a-f0-9]\{1,\}\).*/\1/p' "${continue_file}")
current=$(git rev-parse --short HEAD 2>/dev/null || true)

# Extract Written timestamp and compute elapsed-time flavor line.
# Graceful degradation: if date parsing fails (e.g. BSD date without -d),
# the flavor line is simply omitted — resume still works.
emit_flavor() {
  local written w_epoch n_epoch secs elapsed now
  written=$(sed -n '1s/.*Written: \([^ ]*\).*/\1/p' "${continue_file}")
  [ -n "${written}" ] || return 0
  w_epoch=$(date -d "${written}" +%s 2>/dev/null) || return 0
  n_epoch=$(date +%s)
  secs=$((n_epoch - w_epoch))
  [ "${secs}" -ge 0 ] || return 0
  if [ "${secs}" -lt 3600 ]; then
    elapsed="$((secs / 60))m"
  elif [ "${secs}" -lt 86400 ]; then
    local h m
    h=$((secs / 3600))
    m=$(((secs % 3600) / 60))
    if [ "${m}" -eq 0 ]; then elapsed="${h}h"; else elapsed="${h}h ${m}m"; fi
  else
    local d h
    d=$((secs / 86400))
    h=$(((secs % 86400) / 3600))
    if [ "${h}" -eq 0 ]; then elapsed="${d}d"; else elapsed="${d}d ${h}h"; fi
  fi
  now=$(date -Iseconds 2>/dev/null) || return 0
  printf '_Resumed %s after seal. Current time: %s._\n' "${elapsed}" "${now}"
}

if [ -n "${stored}" ] && [ "${stored}" = "${current}" ]; then
  cat "${skill_dir}/resume.md"
  printf '\n\n### Continuation payload\n\n'
  # Print header line, flavor line (if computable), then body from line 2 onward.
  sed -n '1p' "${continue_file}"
  emit_flavor
  sed -n '2,$p' "${continue_file}"
else
  emit_bootstrap
fi
