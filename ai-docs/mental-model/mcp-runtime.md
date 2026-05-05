---
domain: mcp-runtime
description: "ws-mcp stdio server, MCP tool registry, CLI mirror, concurrency, and tool-profile gates."
sources:
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/cmd/ws-mcp/
related:
  plugin-runtime: "runtime.info and tools/list are consumed by launcher compatibility checks."
  named-agent-runtime: "agents.*, subquery, and api.ask route through wsagent lifecycle APIs."
  git-workflow-tools: "git.* MCP tools and CLI mirrors delegate to internal/wsgit."
---

# MCP Runtime

## Entry Points

- `cmd/ws-mcp/main.go` is the binary entry point for `serve --stdio`, `runtime info`, CLI mirrors, and local diagnostics. {#260505-runtime-cli-entrypoints}
- `internal/mcp/server.go` owns MCP JSON-RPC request handling, tool schemas, tool dispatch, role filtering, and cancellation. {#260505-mcp-server-protocol-surface}
- `runtime.info` is launcher-facing compatibility data, including embedded prompt bundle metadata. {#260505-runtime-debug-metadata-tools}

## Module Contracts

- `ServeStdio` handles requests concurrently and serializes only response writes; long-running waits must not block `tools/list`.
- Cancellation depends on exact JSON-RPC id stringification; changing id formatting breaks `notifications/cancelled`.
- Tool results are returned as MCP text content, even when the text is JSON. Callers parse text, not structured content arrays.
- `toolTextResponse` errors are successful JSON-RPC responses with `isError: true`; unknown tools/profile violations are JSON-RPC errors.
- The server root is captured at `NewServer`; CLI default root can come from `WS_MCP_PROJECT_ROOT` when invoked from plugin runtime.
- Orchestrator lock errors are startup diagnostics, not delegate evidence; an invalid default root must not hide lead tools that can accept an explicit `root`. {#260505-tool-profile-gating}

## Coupling

- Tool additions require both `callTool` and `tools()` updates; role/profile filtering and runtime metadata must also be reviewed. {#260505-tool-profile-gating}
- CLI mirrors are separate adapters. MCP behavior changes do not update `cmd/ws-mcp` handlers automatically. {#260505-cli-mirror-coverage}
- `api.ask` and `subquery` use named-agent runtime semantics; changes to agent result/wait behavior must keep MCP tool descriptions and follow-up text coherent. {#260505-workflow-state-delegation-tools}
- Config tools read/write user-local config through `wsconfig`; tier names and defaults must match agent registration behavior. {#260505-config-tools}

## Extension Points & Change Recipes

- **Add an MCP tool**: add schema in `tools()`, dispatch in `callTool`, profile permissions in `roleAllowsTool`, tests for lead/delegate/leaf visibility, and `runtime.json`.
- **Add a CLI mirror**: add the top-level or group subcommand in `cmd/ws-mcp`, map flags to the same internal package as MCP, and add command smoke tests.
- **Restrict a tool**: update role tables and add tests proving allowlists cannot regain a tool hidden by role.

## Common Mistakes

- Advertising a tool in `tools()` without a dispatch case creates a visible broken tool.
- Adding dispatch without schema makes the tool callable only by guessing the name.
- Assuming delegate agents can inspect arbitrary agents; delegate profile can use selected `agents.*` tools only for `subquery-*` names, while leaf profile cannot use `agents.*`.
- Treating a lock acquisition error as delegate authority hides repair tools in plugin-managed sessions whose default root was misdetected.
- Treating `domain_hint` in `api.ask` as a direct domain selector; only exact existing domain names bypass routing. {#260505-api-documentation-mcp-tools}

## Technical Debt

- MCP input still uses `bufio.Scanner`; very large single-line MCP requests can hit scanner token limits before tool handling.
- Some compatibility docs mention broader future repair semantics, but `spec_index.verify` currently checks duplicate anchors only.
