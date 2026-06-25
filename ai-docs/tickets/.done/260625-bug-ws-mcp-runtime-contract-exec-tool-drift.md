---
title: ws MCP runtime contract lists hidden exec tools and blocks Codex dev startup
related:
  260605-research-ws-native-subagent-pivot: playbook/MCP runtime surface migration anchor
  260524-chore-exec-surface-runtime-contract: exec tool contract follow-up
completed: 2026-06-25
---

# ws MCP runtime contract lists hidden exec tools and blocks Codex dev startup

## Surprise

During Codex dogfood on 2026-06-25, the installed `ws@0.30.7` dev cache failed
to load its MCP server. The launcher breadcrumb reported:

```text
local devenv runtime was forced but no compatible local runtime could be installed
```

With `WS_MCP_LAUNCHER_DEBUG=1`, the forced local source build succeeded, but the
compatibility check rejected the binary because `runtime.json` still requires
`exec.*` tools that the current runtime intentionally hides from the public MCP
surface:

```text
runtime capabilities missing required tool: exec.raw.read
runtime missing required MCP tool: exec.abort
```

## Evidence

- `agents-plugin/runtime.json` lists `exec.spawn`, `exec.shell`, `exec.status`,
  `exec.result`, `exec.abort`, `exec.raw.tail`, `exec.raw.read`, and
  `exec.raw.grep` as required tools.
- `agents-plugin-tool/internal/mcp/server.go` marks every `exec.*` tool as
  permanently hidden from the public surface while the exec surface is still
  unstable.
- `ws-mcp runtime capabilities` reports public lead tool names through
  `mcp.LeadToolNames()`, which filters permanently hidden tools, so `exec.*`
  is absent from the compatibility payload.

## Follow-Up

Clarify whether hidden/experimental tools belong in `runtime.json` at all. If
they remain intentionally hidden, remove them from the required runtime contract
or add a separate non-public capability channel so launcher compatibility checks
do not reject valid dev builds.

## Resolution (2026-06-25)

Removed the hidden `exec.*` MCP tools from `agents-plugin/runtime.json` so the
launcher compatibility contract matches `ws-mcp runtime capabilities`. The
tools remain implemented but intentionally absent from the public MCP surface
until the exec surface is documented and shipped.
