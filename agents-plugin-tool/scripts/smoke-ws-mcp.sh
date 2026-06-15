#!/usr/bin/env bash
# Smoke-test the ws-mcp stdio server without a host client.
set -euo pipefail

TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$TOOL_DIR"

ROOT="${1:-..}"
export WS_CACHE_HOME="${WS_CACHE_HOME:-$(mktemp -d)}"
export WS_RSRC_ROOT="${WS_RSRC_ROOT:-$TOOL_DIR/../agents-plugin/rsrc}"

python3 - "$ROOT" <<'PY'
import json
import subprocess
import sys

root = sys.argv[1]
proc = subprocess.Popen(
    ["go", "run", "./cmd/ws-mcp", "serve", "--stdio", "--root", root],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    text=True,
)


def call(payload):
    assert proc.stdin is not None
    assert proc.stdout is not None
    proc.stdin.write(json.dumps(payload, separators=(",", ":")) + "\n")
    proc.stdin.flush()
    line = proc.stdout.readline()
    if not line:
        stderr = proc.stderr.read() if proc.stderr is not None else ""
        raise SystemExit(f"ws-mcp stdio smoke: no response for id={payload.get('id')}: {stderr}")
    response = json.loads(line)
    if "error" in response:
        raise SystemExit(f"ws-mcp stdio smoke: JSON-RPC error for id={payload.get('id')}: {response['error']}")
    result = response.get("result", {})
    if result.get("isError"):
        text = "".join(part.get("text", "") for part in result.get("content", []))
        raise SystemExit(f"ws-mcp stdio smoke: tool error for id={payload.get('id')}: {text}")
    return result


call({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}})
call({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
login = call(
    {
        "jsonrpc": "2.0",
        "id": 3,
        "method": "tools/call",
        "params": {"name": "ws.lead.login", "arguments": {"root": root, "format": "json"}},
    }
)
login_text = "".join(part.get("text", "") for part in login.get("content", []))
session_key = json.loads(login_text)["session_key"]
call(
    {
        "jsonrpc": "2.0",
        "id": 4,
        "method": "tools/call",
        "params": {"name": "project_tree", "arguments": {"session_key": session_key}},
    }
)
call(
    {
        "jsonrpc": "2.0",
        "id": 5,
        "method": "tools/call",
        "params": {
            "name": "path.generate",
            "arguments": {"session_key": session_key, "kind": "review", "stems": ["smoke"]},
        },
    }
)
call({"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": {"name": "runtime.info", "arguments": {}}})

assert proc.stdin is not None
proc.stdin.close()
return_code = proc.wait(timeout=10)
if return_code != 0:
    stderr = proc.stderr.read() if proc.stderr is not None else ""
    raise SystemExit(f"ws-mcp stdio smoke exited {return_code}: {stderr}")
PY

go run ./cmd/ws-mcp path generate --root "$ROOT" --kind review smoke-cli
go run ./cmd/ws-mcp runtime info
go run ./cmd/ws-mcp mercenary register --root "$ROOT" --name smoke-reviewer
