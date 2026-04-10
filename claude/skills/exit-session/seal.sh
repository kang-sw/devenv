#!/usr/bin/env bash
# Seal script for /exit-session. Creates or overwrites the
# continuation payload file with only the mechanical header line:
#   <!-- HEAD: <sha7> · Written: <ISO> -->
#
# Runs BEFORE the agent composes the body, so the agent's subsequent
# Read (to satisfy the Write tool's read-before-write requirement)
# loads only this tiny fresh stub rather than the prior session's
# stale payload. The agent then Writes the full file with the header
# preserved verbatim from the Read plus the composed body.
#
# Invoked from SKILL.md via `bash <skill-dir>/seal.sh <path>`.

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: seal.sh <payload-path>" >&2
  exit 2
fi

payload="$1"
dir=$(dirname "${payload}")
mkdir -p "${dir}"

sha=$(git rev-parse --short HEAD)
written=$(date -Iseconds)
header="<!-- HEAD: ${sha} · Written: ${written} -->"

printf '%s\n' "${header}" > "${payload}"

echo "header staged: ${payload} (${sha} @ ${written})"
