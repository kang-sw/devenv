#!/bin/sh
# One-shot stamp rewriter for the whiteboard file:// probe (macOS, Linux, WSL).
# Rewrites the page version in probe.html, then probe.stamp.js, each through a
# temp file and rename: the order and the write the whiteboard render uses.
set -eu
dir=$(cd "$(dirname "$0")" && pwd)

# Epoch milliseconds; macOS date has no %N, so fall back to perl, then seconds.
v=$(date +%s%3N 2>/dev/null || true)
case $v in '' | *[!0-9]*) v=$(perl -MTime::HiRes=time -e 'printf "%d", time*1000' 2>/dev/null || true) ;; esac
case $v in '' | *[!0-9]*) v="$(date +%s)000" ;; esac

sed "s/__PV_[0-9]*__/__PV_${v}__/" "$dir/probe.html" > "$dir/probe.html.tmp"
mv -f "$dir/probe.html.tmp" "$dir/probe.html"
printf 'wbStamp("%s");\n' "$v" > "$dir/probe.stamp.js.tmp"
mv -f "$dir/probe.stamp.js.tmp" "$dir/probe.stamp.js"
echo "stamp $v"
