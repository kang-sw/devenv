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
- Codex plugins can declare bundled MCP server configuration by setting
  `"mcpServers": "./.mcp.json"` in `.codex-plugin/plugin.json` and placing the
  server configuration in plugin-local `.mcp.json`. Verified against the official
  `openai/plugins` examples (`build-ios-apps`, `cloudflare`) and current Codex CLI
  MCP commands.
- Codex plugin cache refresh for this repo-local plugin remains human-in-the-loop:
  the user must uninstall/install the plugin in the Codex UI or start a fresh
  session after plugin bundle changes. Agents should signal the user before any
  verification step that depends on the refreshed installed plugin cache.

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

### Result (d6a3d1b) - 2026-05-03

Created `agents-plugin-tool/` as the contained Go source tree for native
MCP/tooling work, leaving `agents-plugin/` as the plugin distribution candidate
and avoiding loose root-level Go module files. Added `ws-mcp` with `version`,
`doctor --root`, and `serve --stdio`.

Implemented a minimal dependency-free stdio JSON-RPC/MCP loop for initialize,
tools/list, and tools/call. The first tools are read-only:

- `ws.project_tree` renders the current project document map, spec inventory, and
  active ticket queue without invoking the existing Python helper.
- `ws.infra.read` reads a `claude-plugin/infra/` convention document by bare stem
  or filename.

Added Go tests for the MCP loop and project document helpers, plus
`agents-plugin-tool/scripts/smoke-ws-mcp.sh` for a host-free JSONL smoke test.
Added Go to `install.sh` and `ai-docs/spec/personal-devenv.md` for local
development. Target deployment still expects prebuilt binaries; users should not
need Go installed.

Verification:

- `go test ./...` from `agents-plugin-tool/`
- `go build -o /tmp/ws-mcp ./cmd/ws-mcp` from `agents-plugin-tool/`
- `/tmp/ws-mcp doctor --root ..`
- `scripts/smoke-ws-mcp.sh ..`
- `git diff --check`

### Phase 2: Tool surface inventory

Document the first MCP contract for `agents-plugin` skills:

- plugin-managed MCP packaging shape:
  `.codex-plugin/plugin.json` points to plugin-local `.mcp.json`
- MCP resources or tools for project memory and ticket queue
- MCP tools/resources for ticket, spec, and mental-model conventions
- MCP helper for spec stem lookup
- explicit CLI fallback names for Claude compatibility
- deferred write-capable operations and why they are out of scope
- host verification boundary: Codex UI uninstall/install is required before
  testing changed plugin-managed MCP config from the installed plugin cache

Success criteria:

- `ai-docs/ref/codex-integration.md` or a new MCP reference document records the
  exact tool names and expected host configuration shape.
- The epic references this child ticket as the runtime boundary implementation
  slice instead of a planned placeholder.

### Result (8a0c6ae) - 2026-05-03

Added `ai-docs/ref/ws-mcp.md` as the first explicit runtime contract for the
`agents-plugin` MCP boundary. The document records:

- stdio process model for `ws-mcp serve --stdio --root <repo-root>`
- current host-independent commands: `version`, `doctor`, and `serve`
- plugin-managed MCP configuration shape for Codex plugins
- HITM Codex plugin cache refresh boundary for validating installed MCP config
- implemented tool contracts for `ws.project_tree` and `ws.infra.read`
- Claude-compatible fallback helpers: `ws-proj-tree` and `ws-print-infra`
- reserved candidate surfaces for project index, ticket queue, spec stems, and
  mental-model listing
- deferred write-capable operations and version drift design boundary

Updated `ai-docs/ref/codex-integration.md` and `_index.md` so future sessions can
find the new MCP contract from the normal recovery path.

### Phase 3: Distribution design

Define the portable binary distribution plan:

- release asset naming for Windows, macOS, and Linux
- curl/PowerShell installer behavior
- install location and MCP client config expectations
- `install-ws-plugin` skill behavior for preparing or updating the `ws-mcp`
  binary that plugin-local `.mcp.json` points at
- version drift detection between the installed plugin bundle and the local
  `ws-mcp` binary, likely through a small plugin runtime contract file read by
  `ws-mcp doctor` and server startup
- CI cross-compilation matrix
- manual host smoke checklist

Success criteria:

- The design does not require Go, Python, Node, Cargo, or Visual Studio Build
  Tools on target user machines.
- Windows installation is described as downloading a prebuilt `.exe`, not building
  locally.
- Runtime drift produces an actionable diagnostic instead of silently exposing
  tools that are too old for the installed skill documents.
