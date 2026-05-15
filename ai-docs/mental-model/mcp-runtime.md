---
domain: mcp-runtime
description: "ws-mcp stdio server, MCP tool registry, CLI mirror, concurrency, and tool-profile gates."
sources:
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/cmd/ws-mcp/
related:
  plugin-runtime: "runtime.capabilities is the launcher fast path; runtime.info, tools/list, and CLI probes remain fallback compatibility checks."
  named-agent-runtime: "agents.*, subquery, and api.ask route through wsagent lifecycle APIs."
  git-workflow-tools: "git.* MCP tools and CLI mirrors delegate to internal/wsgit."
---

# MCP Runtime

## Domain Rules

- MCP tool output is primarily consumed by LLMs: default responses should be
  compact, readable text unless callers need stable machine parsing, protocol
  metadata, or compatibility-preserving JSON.

## Entry Points

- `cmd/ws-mcp/main.go` is the binary entry point for `serve --stdio`, `runtime info`, CLI mirrors, and local diagnostics. {#260505-runtime-cli-entrypoints}
- `ws-mcp smoke --root <repo>` is the single-process executable smoke entrypoint; keep it aligned with release workflow checks. {#260505-runtime-cli-entrypoints}
- `internal/mcp/server.go` owns MCP JSON-RPC request handling, tool schemas, tool dispatch, optional profile filtering, and cancellation. {#260505-mcp-server-protocol-surface}
- `internal/mcp/api_async.go` owns recoverable API documentation job state behind the `api.ask_async` tool family. {#260508-api-documentation-async-mcp-tools}
- `runtime.info` and `runtime.capabilities` are launcher-facing compatibility data; capabilities adds MCP protocol, lead tool names, and CLI commands. {#260505-runtime-debug-metadata-tools} {#260506-runtime-capabilities-single-probe}

## Module Contracts

- `ServeStdio` handles requests concurrently and serializes only response writes; long-running waits must not block `tools/list`.
- Cancellation depends on exact JSON-RPC id stringification; changing id formatting breaks `notifications/cancelled`.
- Tool results are returned as MCP text content, even when the text is JSON. Callers parse text, not structured content arrays.
- `toolTextResponse` errors are successful JSON-RPC responses with `isError: true`; unknown tools/profile violations are JSON-RPC errors.
- The server root is captured at `NewServer`; root-aware MCP tool calls use a resolver chain of explicit `root`, volatile session default root, unambiguous host workspace metadata, explicit non-dot startup root, `WS_MCP_PROJECT_ROOT`, and then startup root. Invalid explicit startup roots fail closed instead of falling through to the environment fallback. {#260505-mcp-session-default-root}
- `runtime.capabilities` must report the full lead launcher contract surface even when `WS_MCP_TOOL_PROFILE` or `WS_MCP_ALLOWED_TOOLS` is inherited; use `LeadToolNames`, not filtered server tools. {#260506-runtime-capabilities-single-probe}
- `WS_MCP_NO_AGENT=1` is a product-mode surface, not a profile filter: tools/list, tools/call, CLI command gates, and `runtime.capabilities` all hide agent-backed surfaces together while environment-unset full ws behavior stays unchanged. {#260513-wsflow-agentless-runtime-mode}
- Empty `WS_MCP_NAMESPACE` and `WS_MCP_SETUP_TOOL` values are treated as unset, preserving `ws` namespace text and the `ws.setup` advertised setup tool.
- MCP starts with the lead tool surface; worktree locks are not an authority signal for tool visibility. {#260505-tool-profile-gating}
- `WS_MCP_TOOL_PROFILE` is an optional containment filter. If host environment propagation fails, delegated agents may see lead tools and must follow prompt-level role rules.
- `ws.setup(root)` is the public root-session setup surface; it stores a canonical Git worktree root in the current server instance only and does not change process cwd or write config. Hidden `session.*` dispatch can exist for compatibility but must not be advertised as canonical.
- Plugin-managed MCP calls may lack a caller repository root on native Windows; if `WS_MCP_PROJECT_ROOT` and host metadata are unavailable, tools need an explicit compatibility `root` or `ws.setup(root)` rather than the user's shell cwd.
- The server records a session harness from MCP payloads, not as an authority boundary: `initialize.params` may identify Claude/Codex clients, and `tools/call._meta.x-codex-turn-metadata` is a Codex signal. Conflicts are debug events and do not silently switch the stored harness. {#260508-mcp-payload-harness-detection}

## Coupling

- Tool additions, removals, or intentionally hidden compatibility paths require both `callTool` and `tools()` review; role/profile filtering and runtime metadata must also be reviewed. `runtime.capabilities` derives MCP tool names from `tools()`, but `runtime.json` still must be updated. {#260505-tool-profile-gating}
- CLI mirrors are separate adapters. MCP behavior changes do not update `cmd/ws-mcp` handlers automatically, and public launcher-required CLI commands must also be kept in `runtimeCapabilityCommandNames` plus `runtime.json.commands`. {#260505-cli-mirror-coverage}
- `api.ask`, async API jobs, and `subquery` use named-agent runtime semantics; changes to agent result/wait/cancel behavior must keep MCP tool descriptions, async job reconciliation, and follow-up text coherent. {#260505-workflow-state-delegation-tools}
- Config tools read/write user-local config through `wsconfig`; compatibility tier names, model aliases, optional effort metadata, and harness-aware defaults must match agent registration behavior and readable `config.show` output. {#260505-config-tools} {#260508-model-alias-config-tools}

## Extension Points & Change Recipes

- **Add an MCP tool**: add schema in `tools()`, dispatch in `callTool`, optional profile permissions in `roleAllowsTool`, visibility tests when filtered, and `runtime.json`.
- **Add a CLI mirror**: add the top-level or group subcommand in `cmd/ws-mcp`, map flags to the same internal package as MCP, and add command smoke tests.
- **Restrict a tool under a profile**: update profile tables and add tests proving allowlists cannot regain a hidden tool.
- **Add or change a product-mode gate**: update MCP tool filtering, explicit call errors, CLI command dispatch, `runtimeCapabilityCommandNames`, and both default and mode-specific tests.
- **Change wsflow no-agent mode**: update `agents-plugin-wsflow/runtime.json`, package tests, and launcher contract expectations in the same logical change.

## Common Mistakes

- Advertising a tool in `tools()` without a dispatch case creates a visible broken tool.
- Adding dispatch without schema makes the tool callable only by guessing the name.
- Treating MCP profile filters as an authority boundary creates false safety; prompt-level delegate rules remain the durable containment mechanism.
- Assuming delegate agents can inspect arbitrary agents when `WS_MCP_TOOL_PROFILE` is applied; delegate profile can use selected `agents.*` tools only for `subquery-*` names, while leaf profile cannot use `agents.*`.
- Treating `domain_hint` in `api.ask` as a direct domain selector; only exact existing domain names bypass routing. {#260505-api-documentation-mcp-tools}
- Adding API-doc async tools without updating `agents-plugin/runtime.json`; launcher compatibility checks compare the required MCP tool surface against runtime metadata.
- Assuming MCP tool calls know the user's shell cwd; plugin-managed server cwd can be the plugin cache.
- Guessing among multiple host workspaces creates cross-project writes; root resolution must ask for explicit compatibility `root` or `ws.setup(root)` instead.
- Letting `WS_MCP_PROJECT_ROOT` shadow an explicit non-dot server startup root makes tests pass in this dogfooding repo while plugin-managed calls target the wrong project.
- Treating namespace override as a tool rename; wsflow changes user-facing namespace text and advertised setup alias, while generic MCP tool identifiers stay stable.

## Technical Debt

- MCP input still uses `bufio.Scanner`; very large single-line MCP requests can hit scanner token limits before tool handling.
- Some compatibility docs mention broader future repair semantics, but `spec_index.verify` currently checks duplicate anchors only.
