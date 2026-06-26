#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
tool_dir=$(CDPATH= cd -- "$script_dir/.." && pwd -P)

version=${1:-}
if [ -z "$version" ]; then
  if git -C "$tool_dir" describe --tags --exact-match >/dev/null 2>&1; then
    version=$(git -C "$tool_dir" describe --tags --exact-match)
  else
    version=0.30.12-dev
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

host_os=$(uname -s 2>/dev/null | tr '[:upper:]' '[:lower:]')
host_arch=$(uname -m 2>/dev/null)
case "$host_os" in
  darwin) host_os=darwin ;;
  linux) host_os=linux ;;
  msys*|mingw*|cygwin*) host_os=windows ;;
  *) host_os= ;;
esac
case "$host_arch" in
  arm64|aarch64) host_arch=arm64 ;;
  x86_64|amd64) host_arch=amd64 ;;
  *) host_arch= ;;
esac
host_ext=
[ "$host_os" = "windows" ] && host_ext=.exe
host_asset=$dist_dir/ws-mcp-$host_os-$host_arch$host_ext
runtime_json=$tool_dir/../agents-plugin/runtime.json
if [ -n "$host_os" ] && [ -n "$host_arch" ] && [ -x "$host_asset" ] && [ -f "$runtime_json" ] && command -v python3 >/dev/null 2>&1; then
  runtime_info=$("$host_asset" runtime info)
  RUNTIME_INFO=$runtime_info RUNTIME_JSON=$runtime_json python3 - <<'PY'
import json
import os

info = json.loads(os.environ["RUNTIME_INFO"])
path = os.environ["RUNTIME_JSON"]
with open(path, "r", encoding="utf-8") as f:
    contract = json.load(f)
contract.setdefault("prompt_bundle", {})
contract["prompt_bundle"]["content_sha256"] = info["prompt_bundle"]["content_sha256"]
contract["prompt_bundle"]["prompts"] = info["prompt_bundle"]["prompts"]
with open(path, "w", encoding="utf-8") as f:
    json.dump(contract, f, indent=2)
    f.write("\n")
PY
fi

(
  cd "$dist_dir"
  shasum -a 256 ws-mcp-* > SHA256SUMS
)

printf '%s\n' "$dist_dir"
