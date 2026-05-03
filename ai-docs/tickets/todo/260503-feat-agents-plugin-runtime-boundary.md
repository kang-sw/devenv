---
title: agents-plugin runtime and MCP boundary
parent: 260503-epic-agents-plugin-skill-porting
related:
  260429-research-host-neutral-ws-plugin: research anchor for host-neutral ws plugin architecture
  260502-feat-agents-plugin-workflow-skill-drafts: draft skills waiting on helper/runtime reconstruction
---

# agents-plugin runtime and MCP boundary

## Background

`agents-plugin/` can now expose draft `ws` skills in Codex, but those skills still
avoid operational helper behavior. The core runtime problem is that the current
Claude package relies on `ws-*` scripts becoming available through plugin install
and shell `PATH` behavior. Codex local plugin installs do not provide an equivalent
PATH injection contract, and company Windows deployments should not assume Python,
Node, Cargo, or Visual Studio Build Tools are present.

The next slice should create a small, portable MCP baseline that lets hosts call
`ws` project helpers through an explicit server command instead of through implicit
PATH mutation. The implementation language decision is Go: ship prebuilt native
binaries per OS/architecture, use curl or PowerShell only as bootstrap download
mechanisms, and keep the runtime free of user-installed language dependencies.

## Decisions

- Use Go for the first MCP runtime because it can produce small single-file
  binaries for Windows, macOS, and Linux without requiring a language runtime on
  the user's machine.
- Start with stdio MCP, not an OS daemon. Codex, Claude, or another MCP client
  launches `ws-mcp serve --stdio` for the session and communicates over
  stdin/stdout JSON-RPC. The MCP stdio transport is newline-delimited JSON-RPC
  according to the 2025-03-26 specification:
  https://modelcontextprotocol.io/specification/2025-03-26/basic/transports
- Keep the first MCP tools read-oriented. They should replace the most common
  context-gathering helper assumptions before write-capable workflow tooling is
  designed.
- Keep existing `claude-plugin/bin/ws-*` scripts as compatibility fallbacks until
  each replacement MCP surface and CLI wrapper path is documented.
- Avoid CGO and native dependencies in the baseline so CI cross-compilation remains
  simple.

## Phases

### Phase 1: Go MCP baseline

Create the initial Go module and `ws-mcp` command with:

- a contained `agents-plugin-tool/` source tree so the repo root keeps only the
  plugin candidate and its companion native tooling directory
- `ws-mcp version`
- `ws-mcp doctor --root <repo>`
- `ws-mcp serve --stdio`
- a minimal stdio JSON-RPC/MCP loop that supports initialize, tools/list, and
  tools/call
- read-oriented tools for project tree and infra document reading
- Go unit tests for core project document helpers
- an integration smoke path for the stdio MCP server

Success criteria:

- `go test ./...` passes in a local Go environment.
- `ws-mcp doctor --root <repo>` verifies the expected `ai-docs/` and plugin
  directories.
- A JSONL smoke request can initialize the server, list tools, and call
  `ws.project_tree`.
- The implementation does not require Python or shell helper scripts for the
  initial tools.

### Phase 2: Tool surface inventory

Document the first MCP contract for `agents-plugin` skills:

- MCP resources or tools for project memory and ticket queue
- MCP tools/resources for ticket, spec, and mental-model conventions
- MCP helper for spec stem lookup
- explicit CLI fallback names for Claude compatibility
- deferred write-capable operations and why they are out of scope

Success criteria:

- `ai-docs/ref/codex-integration.md` or a new MCP reference document records the
  exact tool names and expected host configuration shape.
- The epic references this child ticket as the runtime boundary implementation
  slice instead of a planned placeholder.

### Phase 3: Distribution design

Define the portable binary distribution plan:

- release asset naming for Windows, macOS, and Linux
- curl/PowerShell installer behavior
- install location and MCP client config expectations
- CI cross-compilation matrix
- manual host smoke checklist

Success criteria:

- The design does not require Go, Python, Node, Cargo, or Visual Studio Build
  Tools on target user machines.
- Windows installation is described as downloading a prebuilt `.exe`, not building
  locally.
