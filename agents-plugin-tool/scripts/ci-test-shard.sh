#!/usr/bin/env bash
# Runs one shard of the Go test suite for ws-mcp CI.
#
#   ci-test-shard.sh all        every package except the long group (ubuntu)
#   ci-test-shard.sh rest       every package except internal/mcp and long
#   ci-test-shard.sh long       only the explicitly requested long group
#   ci-test-shard.sh mcp I N    top-level internal/mcp tests whose sorted index
#                               is I mod N (subtests run whole under their parent)
#
# internal/mcp runs its tests serially and takes ~15 min on Windows, so the
# Windows leg splits it across parallel jobs; `rest` plus every `mcp I N` for
# I in 0..N-1 covers the same tests as `all`. Adding `long` to either partition
# covers the entire suite. Extra arguments after the shard selector are passed
# to `go test` (for example -list to preview a shard without running tests).
#
# -count=1 bypasses cached test results: CI restores GOCACHE, and a cached pass
# would skip tests whose inputs go test cannot see (git subprocesses, the
# runner's tools). The restored compile cache still applies.
set -euo pipefail

cd "$(dirname "$0")/.."

# Keep this script compatible with macOS /bin/bash 3.2 for ship pre-flight.
long_pkgs=(./internal/wsledger ./internal/wstrackerhost)

mode="${1:?usage: ci-test-shard.sh all|rest|long|mcp I N [go test args...]}"
shift

case "$mode" in
  all|rest)
    packages="$(go list ./...)"
    pkgs=()
    while IFS= read -r pkg; do
      skip=false
      for long_pkg in "${long_pkgs[@]}"; do
        if [[ "$pkg" == */"${long_pkg#./}" ]]; then
          skip=true
        fi
      done
      if [[ "$mode" == rest && "$pkg" == */internal/mcp ]]; then
        skip=true
      fi
      if [[ "$skip" == false ]]; then
        pkgs+=("$pkg")
      fi
    done <<< "$packages"
    exec go test -count=1 -timeout 30m "$@" "${pkgs[@]}"
    ;;
  long)
    exec go test -count=1 -timeout 30m "$@" "${long_pkgs[@]}"
    ;;
  mcp)
    index="${1:?mcp shard needs an index}"
    count="${2:?mcp shard needs a count}"
    shift 2
    listed="$(go test -list . ./internal/mcp | grep -E '^(Test|Example|Fuzz)' | LC_ALL=C sort)"
    names=()
    while IFS= read -r name; do
      names+=("$name")
    done <<< "$listed"
    picked=()
    for i in "${!names[@]}"; do
      if (( i % count == index )); then
        picked+=("${names[$i]}")
      fi
    done
    if (( ${#picked[@]} == 0 )); then
      echo "shard $index/$count selected no tests" >&2
      exit 1
    fi
    pattern="^($(IFS='|'; echo "${picked[*]}"))\$"
    exec go test -count=1 -timeout 30m -run "$pattern" "$@" ./internal/mcp
    ;;
  *)
    echo "unknown shard mode: $mode" >&2
    exit 2
    ;;
esac
