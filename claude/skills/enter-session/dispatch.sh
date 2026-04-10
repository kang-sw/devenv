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

if [ -n "${stored}" ] && [ "${stored}" = "${current}" ]; then
  cat "${skill_dir}/resume.md"
  printf '\n\n### Continuation payload\n\n'
  cat "${continue_file}"
else
  emit_bootstrap
fi
