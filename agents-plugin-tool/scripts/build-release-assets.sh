#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
tool_dir=$(CDPATH= cd -- "$script_dir/.." && pwd -P)

version=${1:-}
if [ -z "$version" ]; then
  if git -C "$tool_dir" describe --tags --exact-match >/dev/null 2>&1; then
    version=$(git -C "$tool_dir" describe --tags --exact-match)
  else
    version=0.35.7-dev
  fi
fi
version=${version#v}
source_commit=$(git -C "$tool_dir" rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')

dist_dir=$tool_dir/dist
rm -rf "$dist_dir"
mkdir -p "$dist_dir"

build_one() {
  goos=$1
  goarch=$2
  ext=$3
  asset="ws-mcp-$goos-$goarch$ext"
  printf 'building %s\n' "$asset" >&2
  (
    cd "$tool_dir"
    CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build \
      -trimpath \
      -ldflags "-s -w -X main.version=$version -X main.sourceCommit=$source_commit" \
      -o "$dist_dir/$asset" \
      ./cmd/ws-mcp
  )
}

build_one darwin arm64 ""
build_one darwin amd64 ""
build_one linux amd64 ""
build_one linux arm64 ""
build_one windows amd64 ".exe"
build_one windows arm64 ".exe"

(
  cd "$dist_dir"
  shasum -a 256 ws-mcp-* > SHA256SUMS
)

printf '%s\n' "$dist_dir"
