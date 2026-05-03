# ws MCP Runtime Reference

Reference for the host-neutral `ws-mcp` runtime used by the `agents-plugin`
candidate.

## Purpose

`ws-mcp` replaces implicit `ws-*` command availability with an explicit MCP
server process that a host can launch. The first contract is intentionally small
and read-oriented: it gives skills access to project memory and convention
documents without depending on plugin PATH injection.

The current source tree is:

```text
agents-plugin-tool/
  cmd/ws-mcp/       # command entry point
  internal/mcp/     # stdio JSON-RPC/MCP loop
  internal/wsdoc/   # project document helper logic
```

The plugin distribution candidate remains `agents-plugin/`. The native tooling
source remains `agents-plugin-tool/` so this migration does not add loose Go
module files or command directories at the repository root.

## Process Model

The baseline server is stdio MCP, not a background OS daemon.

```bash
ws-mcp serve --stdio --root <repo-root>
```

The host launches the command and communicates over stdin/stdout JSON-RPC. The
server advertises MCP protocol version `2025-03-26` and tool capability only.

Supported local commands:

```bash
ws-mcp version
ws-mcp doctor --root <repo-root>
ws-mcp serve --stdio --root <repo-root>
```

`doctor` is a host-independent smoke check. It verifies the repository root,
`ai-docs/`, `agents-plugin/`, `claude-plugin/`, and `ai-docs/_index.md`.

## Codex Plugin Configuration

Codex plugin bundles can reference plugin-local MCP configuration:

```json
{
  "skills": "./skills/",
  "mcpServers": "./.mcp.json"
}
```

The plugin-local `.mcp.json` uses an MCP server map:

```json
{
  "mcpServers": {
    "ws": {
      "command": "ws-mcp",
      "args": ["serve", "--stdio", "--root", "<repo-root>"]
    }
  }
}
```

The exact command path is not locked yet. Phase 3 must decide whether this points
at a stable user-local binary path, a wrapper, or a host-specific install path.
Do not add a production `.mcp.json` that requires Go, Python, Node, Cargo, or
Visual Studio Build Tools on target user machines.

For repo-local Codex plugin iteration, changed plugin-managed MCP configuration
requires a human-in-the-loop cache refresh: the user must uninstall/install the
plugin in the Codex UI or start a fresh session before installed-plugin
verification. Agents should explicitly ask for that refresh when validation
depends on the installed plugin cache.

## MCP Tool Contract

### `ws.project_tree`

Render the ws project document map, spec inventory, and active ticket queue.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    }
  }
}
```

Output:

- MCP text content.
- A Markdown-like plain text report with:
  - `ai-docs/` document tree, excluding `spec/` and `tickets/` from the generic
    tree section.
  - `spec:` inventory with frontmatter title/summary and feature/WIP counts.
  - `tickets:` active queue for `todo` and `idea` tickets.

Error behavior:

- Tool-level errors return `isError: true` with text content.
- Missing or invalid `ai-docs/` is reported as a tool error.

Compatibility fallback:

```bash
ws-proj-tree
```

The MCP output does not have to be byte-identical to the legacy helper, but it
must preserve the same workflow purpose: compact project orientation for
discussion and ticket planning.

### `ws.infra.read`

Read a ws infra convention document by bare stem or filename.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Infra document stem or filename, for example ticket-conventions."
    }
  },
  "required": ["name"]
}
```

Output:

- MCP text content containing the Markdown document body.

Constraints:

- `name` must be a bare stem or filename.
- Path separators are rejected.
- `.md` is appended when absent.
- The document is read from `claude-plugin/infra/` until host-neutral convention
  documents replace that authority.

Error behavior:

- Tool-level errors return `isError: true` with text content.
- Missing document, empty name, and path traversal attempts are tool errors.

Compatibility fallback:

```bash
ws-print-infra <doc>
```

## Candidate Read Surfaces

These names are reserved as likely next additions, but they are not implemented in
the current baseline.

| Candidate | Purpose | Current fallback |
|-----------|---------|------------------|
| `ws.project_index.read` | Read `ai-docs/_index.md` as project memory. | `cat ai-docs/_index.md` |
| `ws.ticket_queue.list` | Return active ticket stems grouped by status. | `ws-proj-tree` or direct file listing |
| `ws.spec_stems.list` | List spec anchors and headings for a spec file or all specs. | `ws-list-spec-stems [spec-file]` |
| `ws.spec_stem.generate` | Generate a collision-free spec anchor for a slug. | `ws-generate-spec-stem <slug>` |
| `ws.mental_models.list` | List relevant mental-model documents for target paths. | `ws-list-mental-model [paths...]` |

Convention access should remain through `ws.infra.read` until the convention
documents move to host-neutral locations. If Codex and other hosts expose MCP
resources consistently enough for static documents, these convention documents
may later become resources; for now tools are the stable contract.

## Deferred Write-Capable Operations

The following behavior is intentionally out of scope for the first MCP contract:

- creating or editing tickets
- generating or mutating spec indexes
- writing mental-model updates
- branch management, merge helpers, release helpers, and ship automation
- spawning or coordinating implementation/review agents

These operations have workflow semantics beyond file access. They should be
designed only after the read surfaces and plugin-managed MCP distribution path are
validated.

## Version And Drift Boundary

The current binary reports `0.1.0-dev`. Phase 3 must add a runtime contract that
lets `ws-mcp doctor` and server startup detect drift between the installed plugin
bundle and the local `ws-mcp` binary.

Expected direction:

- plugin documents carry a small runtime contract file
- `ws-mcp` reads that file during `doctor` and startup
- major/minor compatibility is strict, patch compatibility can be flexible
- stale binaries produce actionable diagnostics that point to the
  `install-ws-plugin` skill or update command

Until that exists, skill documents must not assume that installed plugin text and
installed `ws-mcp` binary are automatically in sync.

