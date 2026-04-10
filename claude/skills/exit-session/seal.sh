#!/usr/bin/env bash
# Seal script for /exit-session. Prepends a minimal header to the
# continuation payload: <!-- HEAD: <sha7> · Written: <ISO> -->.
#
# Idempotent: if the first line already matches the header pattern,
# it is replaced in place; otherwise the header is prepended with a
# blank line separator.
#
# Invoked from SKILL.md via `bash "${CLAUDE_SKILL_DIR}/seal.sh" <path>`.

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: seal.sh <payload-path>" >&2
  exit 2
fi

payload="$1"

if [ ! -f "${payload}" ]; then
  echo "seal.sh: payload not found: ${payload}" >&2
  exit 1
fi

sha=$(git rev-parse --short HEAD)
written=$(date -Iseconds)
header="<!-- HEAD: ${sha} · Written: ${written} -->"

tmp=$(mktemp)
trap 'rm -f "${tmp}"' EXIT

first_line=$(sed -n '1p' "${payload}")

if printf '%s' "${first_line}" | grep -q '^<!-- HEAD: '; then
  # Replace existing header line; keep body verbatim.
  printf '%s\n' "${header}" > "${tmp}"
  sed -n '2,$p' "${payload}" >> "${tmp}"
else
  # Prepend header plus blank-line separator, keep body verbatim.
  printf '%s\n\n' "${header}" > "${tmp}"
  cat "${payload}" >> "${tmp}"
fi

mv "${tmp}" "${payload}"
trap - EXIT

echo "sealed: ${payload} (${sha} @ ${written})"
