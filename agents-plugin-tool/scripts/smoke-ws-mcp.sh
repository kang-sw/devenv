#!/usr/bin/env bash
# Smoke-test the ws-mcp stdio server without a host client.
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$TOOL_DIR"

ROOT="${1:-..}"

printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"ws.project_tree\",\"arguments\":{\"root\":\"$ROOT\"}}}" |
  go run ./cmd/ws-mcp serve --stdio --root "$ROOT"
