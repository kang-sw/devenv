#!/usr/bin/env bash
# Smoke-test the ws-mcp stdio server without a host client.
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$TOOL_DIR"

ROOT="${1:-..}"
export WS_CACHE_HOME="${WS_CACHE_HOME:-$(mktemp -d)}"

printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
  "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"project_tree\",\"arguments\":{\"root\":\"$ROOT\"}}}" \
  "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"path.generate\",\"arguments\":{\"root\":\"$ROOT\",\"kind\":\"review\",\"stems\":[\"smoke\"]}}}" \
  '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}' |
  go run ./cmd/ws-mcp serve --stdio --root "$ROOT"

go run ./cmd/ws-mcp path generate --root "$ROOT" --kind review smoke-cli
go run ./cmd/ws-mcp runtime info
go run ./cmd/ws-mcp agents register --root "$ROOT" --name smoke-reviewer --prompt code-reviewer --prompt code-review-correctness
