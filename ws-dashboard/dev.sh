#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$ROOT/frontend"

usage() {
  cat <<'USAGE'
Usage: ./dev.sh <command> [args]

Commands:
  run [serve-args]    Build the frontend and run the protected dashboard daemon
  build               Build the frontend production assets
  test                Run frontend build and Rust workspace tests
  frontend-dev [args] Run the Vite development server
  help                Show this help

Examples:
  ./dev.sh run
  ./dev.sh run --port 8787
  ./dev.sh frontend-dev --host 127.0.0.1
USAGE
}

ensure_frontend_deps() {
  local npm_state="$FRONTEND_DIR/node_modules/.package-lock.json"
  local package_json="$FRONTEND_DIR/package.json"
  local package_lock="$FRONTEND_DIR/package-lock.json"

  if [[ -d "$FRONTEND_DIR/node_modules" &&
        -f "$npm_state" &&
        ! "$package_json" -nt "$npm_state" &&
        ( ! -f "$package_lock" || ! "$package_lock" -nt "$npm_state" ) ]]; then
    return
  fi

  if [[ -f "$package_lock" ]]; then
    (cd "$FRONTEND_DIR" && npm ci)
  else
    (cd "$FRONTEND_DIR" && npm install)
  fi
}

build_frontend() {
  ensure_frontend_deps
  (cd "$FRONTEND_DIR" && npm run build)
}

command="${1:-help}"
if [[ $# -gt 0 ]]; then
  shift
fi

case "$command" in
  run)
    build_frontend
    cd "$ROOT"
    exec cargo run -p ws-dashboard-daemon -- serve --static-dir frontend/dist "$@"
    ;;
  build)
    build_frontend
    ;;
  test)
    build_frontend
    (cd "$ROOT" && cargo test --workspace)
    ;;
  frontend-dev)
    ensure_frontend_deps
    cd "$FRONTEND_DIR"
    exec npm run dev -- "$@"
    ;;
  help|-h|--help)
    usage
    ;;
  *)
    echo "unknown command: $command" >&2
    usage >&2
    exit 2
    ;;
esac
