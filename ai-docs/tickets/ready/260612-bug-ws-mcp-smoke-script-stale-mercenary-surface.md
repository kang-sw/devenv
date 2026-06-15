---
title: ws-mcp smoke script still calls retired agents CLI surface
related:
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: Phase 7 renamed agents.* to ws.mercenary.* / mercenary.*
spec:
  - 260610-ephemeral-session-auth-model
  - 260610-mercenary-delegation-surface
related-mental-model:
  - mcp-runtime
  - plugin-runtime
  - named-agent-runtime
---

# ws-mcp smoke script still calls retired agents CLI surface

## Background

Local dogfooding during a Codex MCP startup failure analysis showed
`agents-plugin-tool/scripts/smoke-ws-mcp.sh ..` failing after the stdio server
portion had already initialized and listed the current tool surface.

The final CLI step still runs:

```bash
go run ./cmd/ws-mcp agents register --root "$ROOT" --name smoke-reviewer --prompt code-reviewer --prompt code-review-correctness
```

That command is stale after the Phase 7 rename and prompt-registration
retirement:

- CLI group `agents` became `mercenary`.
- Register no longer accepts prompt stems; callers should register a
  self-contained prompt rendered through the current playbook surface.
- Root-aware behavior now routes through session keys in MCP; CLI smoke should
  exercise the current supported CLI mirror intentionally rather than preserving
  removed compatibility syntax.

The observed failure was:

```text
usage: ws-mcp <version|doctor|runtime|serve|smoke|config|path|mercenary|git|tickets|specs|mental-models|references>
exit status 2
```

This is not the direct cause of Codex MCP startup failure in the same session;
that failure came from stale installed plugin-cache `runtime.json` versus a
source-built current runtime. This ticket tracks only the source-tree smoke
script drift that made Level 1 verification noisy.

Current dogfood note: the script also still sends removed per-tool `root`
arguments to root-aware MCP calls. The refreshed smoke should use the current
`ws.lead.login(root) -> session_key` flow before calling root-aware tools.

## Phases

### Phase 1: Refresh ws-mcp smoke script for the current mercenary surface

Update `agents-plugin-tool/scripts/smoke-ws-mcp.sh` so it verifies the current
runtime surface instead of the retired `agents register --prompt` command.

The refreshed smoke should either:

- exercise only stable host-independent CLI commands that still exist after the
  mercenary rename; or
- render a current self-contained prompt and register a mercenary through the
  supported `mercenary register` path without depending on removed prompt-stem
  registration.

Verification boundary:

- `agents-plugin-tool/scripts/smoke-ws-mcp.sh ..` exits 0 from this repository.
- `go test ./...` under `agents-plugin-tool/` remains green.
- The script no longer contains `ws-mcp agents` or `--prompt` register syntax.
