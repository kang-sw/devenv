# ws MCP Runtime Reference

Reference for the host-neutral `ws-mcp` runtime used by the `agents-plugin`
candidate.

## Purpose

`ws-mcp` replaces implicit `ws-*` command availability with an explicit MCP
server process that a host can launch. The first contract is intentionally small
and read-oriented: it gives skills access to project memory, bundled convention
documents, and spec helper surfaces without depending on plugin PATH injection or
repository-local plugin source paths.

The current source tree is:

```text
agents-plugin-tool/
  cmd/ws-mcp/       # command entry point
  internal/mcp/     # stdio JSON-RPC/MCP loop
  internal/wsdoc/   # project document helper logic
  internal/wsprompt/ # embedded prompt bundle and resolver
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
ws-mcp runtime info
ws-mcp serve --stdio --root <repo-root>
ws-mcp subquery --root <repo-root> [--deep-research] <question>  # starts async; returns subquery_key
```

`doctor` is a host-independent smoke check. In this repository it verifies the
repository root, `ai-docs/`, `agents-plugin/`, and `ai-docs/_index.md`;
downstream projects may not have the same plugin source layout and should rely
on MCP tools for conventions.

## Skill Porting Policy

Shared `agents-plugin` skill text assumes ws MCP is available. Porting should
preserve the source skill's wording and flow where possible, changing only the
host-specific tool calls, shell interpolations, slash-command syntax, and local
paths that would break outside Claude.

Do not point shared skill text at repository-local legacy paths. Infra and
convention documents are distributed with the MCP runtime and read through
`ws/infra.read` or `ws/convention.read`, so downstream projects can use the same
skill text without carrying this repository's source tree.

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
      "command": "python3",
      "cwd": ".",
      "args": ["./bin/ws-mcp-launcher.py", "serve", "--stdio"],
      "startup_timeout_sec": 30,
      "tool_timeout_sec": 600
    }
  }
}
```

The current POC adds `agents-plugin/bin/ws-mcp-launcher.py` and
`agents-plugin/runtime.json`. The launcher is intentionally quiet on stdout:
stdout belongs to the MCP JSON-RPC stream, while diagnostics go to stderr.
The plugin MCP config sets a 30-second startup timeout and a 600-second per-tool
timeout so long-running named-agent result reads can align with ws runtime
defaults. Codex config names these fields as `mcp_servers.<id>.startup_timeout_sec`
and `mcp_servers.<id>.tool_timeout_sec`; plugin-local `.mcp.json` carries the
same server fields under the bundled `mcpServers` entry.

The preferred Codex design is a plugin-local launcher, not a mandatory separate
install skill. The launcher runs from the installed plugin cache, reads a
plugin-local runtime contract, downloads or copies and verifies the prebuilt
`ws-mcp` binary when missing or incompatible, then execs
`ws-mcp serve --stdio`.

Codex does not resolve plugin-managed MCP `command` relative paths against the
plugin cache by default. The POC must set `"cwd": "."`; Codex normalizes that
value to the installed plugin cache directory, after which
`command: "python3"` starts the plugin-local launcher successfully.

The shared `.mcp.json` uses `python3` because the MCP config format does not
provide an OS selector. The Python launcher keeps one plugin-managed MCP command
for macOS, Linux, and native Windows while the runtime itself remains a prebuilt
Go binary. Native Windows users may need to install Python 3 once when the
Windows Store alias exists but no interpreter is installed.

All fallback paths must preserve the same `runtime.json` compatibility contract,
cache-local runtime location, release asset naming, and checksum verification
policy.

Current launcher inputs:

| Variable | Purpose |
|----------|---------|
| `WS_MCP_RUNTIME_DIR` | Override the runtime binary directory; defaults to plugin-local `.runtime/<os>-<arch>`. |
| `WS_MCP_RUNTIME_BINARY` | Exact repaired runtime binary path exported by the launcher for async worker recovery. |
| `WS_MCP_BOOTSTRAP_BINARY` | Copy a prebuilt local binary into the runtime directory. Used by the current dev POC. |
| `WS_MCP_BOOTSTRAP_URL` | Download a prebuilt binary when no runtime binary exists. |
| `WS_MCP_BOOTSTRAP_SHA256` | Optional SHA-256 checksum for `WS_MCP_BOOTSTRAP_URL`. |
| `WS_MCP_RELEASE_REPOSITORY` | Override the GitHub release repository from `runtime.json`, for example `kang-sw/devenv`. |
| `WS_MCP_RELEASE_TAG` | Override the release tag from `runtime.json`, for example `v0.26.4`. |
| `WS_MCP_RELEASE_BASE_URL` | Override the full release asset base URL; useful for local file or HTTP smoke tests. |
| `WS_MCP_LAUNCHER_DEBUG` | Print launcher diagnostics to stderr when set to `1`. |
| `WS_MCP_PROJECT_ROOT` | Project root used as the default when a tool or CLI command omits `root`; normally derived by the launcher from the parent Codex process. |
| `WS_MCP_NO_AGENT` | Product-mode gate for agentless distributions. When set to `1`, `true`, `yes`, or `on`, agent-backed MCP tools and CLI commands are hidden or disabled. Unset preserves the full ws surface. |
| `WS_MCP_NAMESPACE` | User-facing MCP namespace text override. Empty or unset defaults to `ws`; wsflow sets this to `wsflow`. |
| `WS_MCP_SETUP_TOOL` | Advertised setup tool name override. Empty or unset defaults to `ws.setup`; wsflow sets this to `setup`. |

The plugin-managed MCP path is proven for `codex exec` when `.mcp.json` sets
`cwd: "."`. Without that field, Codex registers the server but startup fails
with `No such file or directory` because the relative launcher argument is
interpreted from the workspace process context, not the plugin cache.

The full `agents-plugin/.mcp.json` intentionally does not set
`WS_MCP_NO_AGENT`, `WS_MCP_NAMESPACE`, or `WS_MCP_SETUP_TOOL`; installed ws
therefore continues to advertise the full `ws` MCP server surface and
`ws.setup`. Agentless derivative packages inject those variables in their own
package-local `.mcp.json` instead of changing the shared ws plugin config.

Because `cwd: "."` points the MCP process at the installed plugin cache, tools
must not treat process cwd as the downstream project root. The Python launcher
derives `WS_MCP_PROJECT_ROOT` from the parent Codex process `PWD` when possible,
and `ws-mcp` uses that environment variable whenever a tool or command omits
`root` or passes `"."`. Skills may still pass `root` explicitly, but omitted
root should resolve to the active project in plugin-managed Codex sessions.

At tool-call time, root-aware MCP tools resolve omitted `root` arguments in this
order: explicit compatibility `root`, volatile root set by `ws.setup`, Codex
`_meta.x-codex-turn-metadata.workspaces` when it contains exactly one workspace,
explicit non-dot server startup root, `WS_MCP_PROJECT_ROOT`, then the server
startup root. If Codex metadata contains multiple workspaces and no
higher-priority root is available, the tool returns actionable `ws.setup`
guidance instead of guessing. The session root exists only in the current stdio
server process and is not written to user config, ws cache config, or repository
files.

## Orchestrator Authority

`ws-mcp` defaults to the full `lead` tool surface. It does not assign
orchestration authority from worktree-local locks or startup-root ownership,
because plugin-managed hosts can start MCP servers from cache directories and can
fail to propagate environment variables to delegated sessions.

`WS_MCP_TOOL_PROFILE` is an optional profile filter, not an authority boundary.
When the host propagates it, `delegate` and `leaf` hide and reject selected
lead-owned orchestration or mutation tools. Delegate may use
`agents.wait/result/status/tail/cancel/print` only for generated `subquery-*`
agents. Leaf also hides `subquery`.

`WS_MCP_NO_AGENT=1` is separate from tool profiles. It is a product-mode gate for
agentless derivative packages: tools/list, explicit tools/call dispatch,
`runtime capabilities`, and CLI command dispatch hide or disable `agents.*`,
`subquery`, `config.agents_tier`, and agent-backed API documentation tools while
keeping read-only surfaces such as `api.list`.

If profile propagation fails, delegated agents may see the full lead MCP surface.
Containment therefore depends on prompt-level role rules such as delegate
orientation and lead-owned orchestration instructions. `WS_MCP_ALLOWED_TOOLS`
can narrow the selected profile for tests or debugging, but it cannot expand
access beyond that profile.

For repo-local Codex plugin iteration, changed plugin-managed MCP configuration
requires a human-in-the-loop cache refresh: the user must uninstall/install the
plugin in the Codex UI or start a fresh session before installed-plugin
verification. Agents should explicitly ask for that refresh when validation
depends on the installed plugin cache.

## MCP Tool Contract

### `ws/runtime.info`

Return runtime metadata used for plugin compatibility checks.

Output:

- MCP text content with labeled `version`, `source_commit`, and prompt bundle
  metadata.
- Pass `format: "json"` for structured compatibility output containing
  `version`, `source_commit`, and `prompt_bundle`.

Behavior:

- The POSIX plugin launcher compares `prompt_bundle.content_sha256` with
  `agents-plugin/runtime.json`.
- A mismatch causes the launcher to repair the cache-local runtime binary just
  as tool or command drift does.

### `ws/ws.setup`

Configure volatile ws MCP session state for the current server process.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Git worktree root to store for later root-omitted ws MCP tool calls."
    },
    "format": {
      "type": "string",
      "description": "Optional output format. Use \"json\" for structured compatibility output."
    }
  }
}
```

Behavior:

- When `root` is present, validates it with Git and stores the canonical
  worktree root in memory.
- Affects later root-omitted root-aware tool calls handled by the same server.
- Omitting `root` reports current setup state without guessing or persisting a
  root.
- Does not persist beyond the MCP server process and does not write config files.
- Legacy `session.set_default_root` and `session.get_default_root` may remain as
  hidden compatibility dispatch, but they are not advertised in `tools/list` or
  `agents-plugin/runtime.json`.

Output:

- MCP text content with labeled setup fields by default.
- Pass `format: "json"` for structured compatibility output with `root`,
  `has_root`, `session_harness`, `env_project_root`, and `server_root`.

### `ws/api.list`

Return sorted third-party API documentation cache domain names. Defaults to one
domain per line; pass `format: "json"` for the structured domain array.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
    "format": { "type": "string" }
  }
}
```

Output:

- MCP text content with one domain directory name per line.
- Dot-prefixed directories and non-directories are excluded. Missing `.deps`
  returns an empty response.
- Pass `format: "json"` for a structured domain array.

### `ws/api.ask`

Ask third-party API documentation questions through runtime-managed per-domain
manager sessions.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
    "prompt": {
      "type": "string",
      "description": "API documentation question to answer."
    },
    "domain_hint": {
      "type": "string",
      "description": "Optional API documentation domain hint."
    }
  },
  "required": ["prompt"]
}
```

Behavior:

- An exact `domain_hint` matching an existing `ai-docs/.deps/<domain>/`
  directory skips routing and calls only that domain.
- Otherwise the runtime invokes the embedded `pre-router` prompt with the hint,
  existing domains, and original prompt; router output is parsed as one domain
  slug per non-empty line.
- Each resolved domain is handled by persistent manager `api-doc-<domain>` using
  the embedded `api-doc-manager` prompt. The manager owns stale checking and any
  official-documentation fetching inside its domain cache.
- Same-process calls for the same domain serialize through a process-local
  per-domain lock. Distinct domains may run concurrently.
- The public tool call is synchronous: it waits for routed domain managers and
  aggregates their results before returning.
- Output preserves per-domain boundaries. If at least one domain succeeds, failed
  domains are included as explicit error blocks; if all domains fail, the tool
  returns a tool error.

Constraints:

- The public MCP surface intentionally exposes API lookup, async job control,
  and domain listing only; refresh and stale-check operations remain
  manager-internal.
- Worker-facing guidance should use these tools and should not direct ordinary
  workers to read `ai-docs/.deps/` directly.

### `ws/api.ask_async`

Start a recoverable asynchronous third-party API documentation job and return an
`api_job_key` immediately.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
    "prompt": {
      "type": "string",
      "description": "API documentation question to answer asynchronously."
    },
    "domain_hint": {
      "type": "string",
      "description": "Optional API documentation domain hint."
    }
  },
  "required": ["prompt"]
}
```

Behavior:

- The tool persists a job record under worktree-scoped ws state before returning.
- The returned `api_job_key` is the recovery handle for later status, result, or
  cancellation calls.
- Async jobs reuse the same domain routing, per-domain manager sessions, cache
  ownership, and aggregation format as `ws/api.ask`.

### `ws/api.status`

Return JSON status for an async API documentation job by `api_job_key`.

### `ws/api.result`

Return the final async API documentation answer by `api_job_key`. Partial
success returns answer text; all-domain failure and cancellation return tool
errors with preserved status text.

### `ws/api.cancel`

Best-effort cancel an async API documentation job by `api_job_key`. Cancellation
is durable in job state and also propagates to active pre-router or domain
manager workers when they are still running in the current process.

### `ws/git.diff`

Return read-only Git diff output.

Behavior:

- Defaults to `mode: "stat"` to avoid returning patch content unless requested.
- `mode: "full"` returns patch content.
- `mode: "name_only"` returns changed paths.
- Defaults to direct diff text. Pass `format: "json"` for structured
  compatibility output containing `mode`, `range`, `paths`, and `output`.
- With no `range`, worktree output includes untracked files so `ws/git.diff`
  does not hide files visible in `ws/git.status`.
- With a `range`, output is revision-scoped and does not include unrelated
  worktree untracked files.

### `ws/git.commit`

Create a workflow-aware Git commit from explicit paths and structured message
fields.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "paths": { "type": "array", "items": { "type": "string" } },
    "title": { "type": "string" },
    "description": { "type": "string" },
    "ai_context": { "type": "array", "items": { "type": "string" } },
    "updated_tickets": { "type": "array", "items": { "type": "string" } },
    "updated_specs": { "type": "array", "items": { "type": "string" } },
    "updated_mental_models": { "type": "array", "items": { "type": "string" } }
  },
  "required": ["paths", "title", "ai_context"]
}
```

Behavior:

- Stages only `paths` through `git add -- <paths>`.
- When a requested path is a ticket file, expands staging to same-stem changed
  ticket paths so status-directory moves commit old and new paths together.
- Refuses option-like, absolute, or repository-escaping paths.
- Refuses commits when unrelated staged paths exist.
- Builds a commit message with title, optional description, `## AI Context`,
  and optional document-update sections.
- If `updated_tickets` is omitted, staged ticket moves and added `### Result`
  or `#### Edition` headings under `ai-docs/tickets/` are detected and
  summarized in `## Updated Tickets`.
- Returns JSON containing the new hash, paths, title, and detected
  `ticket_changes`.

Compatibility fallback:

```bash
ws-mcp git commit --path <path> --title <title> --ai-context <bullet>
```

Constraints:

- This is a constrained commit builder, not a full Git wrapper.
- It does not reset, checkout, clean, merge, push, or mutate ticket files.
- Leaf-role MCP servers cannot call `ws/git.commit`.

### `ws/tickets.list`

List ticket paths and structured status metadata without returning full document
bodies.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "statuses": { "type": "array", "items": { "type": "string" } },
    "include_done": { "type": "boolean" },
    "include_dropped": { "type": "boolean" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Defaults to active ticket statuses: `ready`, `todo`, and `idea`.
- `include_done` separately opts into `ai-docs/tickets/.done/`.
- `include_dropped` separately opts into `ai-docs/tickets/.dropped/`.
- Defaults to compact status lines with path, title, unresolved phases, snippets,
  and important flags where relevant.
- Pass `format: "json"` for structured objects with `stem`, `path`,
  directory-derived `status`, title, parent, related ticket stems,
  spec/spec-remove frontmatter, plans, skeletons, completed date, phase
  headings, unresolved phases, and result-present status.
- Status filters may use `done`/`.done` and `dropped`/`.dropped`, but archived
  statuses are still hidden unless their matching include flag is true.

Compatibility fallback:

```bash
ws-mcp tickets list [--status ready] [--include-done] [--include-dropped]
```

### `ws/tickets.find`

Find ticket paths by query, exact ticket stem, or mentions of another ticket
stem.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "statuses": { "type": "array", "items": { "type": "string" } },
    "include_done": { "type": "boolean" },
    "include_dropped": { "type": "boolean" },
    "query": { "type": "string" },
    "ticket_stem": { "type": "string" },
    "mentions_ticket_stem": { "type": "string" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Uses `ticket_stem`; ticket tools reject `spec_stem` to keep ticket and spec
  anchor identifiers distinct.
- `mentions_ticket_stem` finds tickets whose raw text references the supplied
  ticket stem, avoiding ad hoc shell pipelines.
- `query` is a case-insensitive match over stem, path, title, and ticket text.
  Results include short matching snippets for disambiguation, not full bodies.
- Defaults to compact status lines. Pass `format: "json"` for structured
  metadata.

Compatibility fallback:

```bash
ws-mcp tickets find --mentions-ticket-stem <ticket-stem>
```

### `ws/tickets.status`

Return status metadata for one ticket stem.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "ticket_stem": { "type": "string" },
    "include_done": { "type": "boolean" },
    "include_dropped": { "type": "boolean" },
    "format": { "type": "string" }
  },
  "required": ["ticket_stem"]
}
```

Behavior:

- Looks up a ticket by stable stem, not by path.
- Active statuses are searched by default; `.done/` and `.dropped/` require the
  matching include flag.
- Defaults to a compact status line with path, title, unresolved phases, and
  important flags where relevant.
- Pass `format: "json"` for the same path-first metadata shape as
  `ws/tickets.list`.

Compatibility fallback:

```bash
ws-mcp tickets status <ticket-stem> [--include-done] [--include-dropped]
```

### `ws/specs.list`

List spec files with path-first metadata and anchor locations without returning
full spec bodies.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Scans `ai-docs/spec/` recursively.
- Defaults to compact spec summary lines with anchor counts, ticket refs,
  snippets, and marker contexts where relevant.
- Pass `format: "json"` for structured objects with relative path,
  duplicate-safe filename, title, summary, anchors, ticket references found in
  frontmatter or feature entries, and WIP/planned marker contexts.
- Anchor entries include `spec_stem`, line number, nearest heading, and marker
  context when detectable.
- Does not return full document bodies.

Compatibility fallback:

```bash
ws-mcp specs list
```

### `ws/specs.find`

Find spec files by query, spec anchor stem, or ticket stem reference.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "query": { "type": "string" },
    "spec_stem": { "type": "string" },
    "ticket_stem": { "type": "string" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Uses `spec_stem` for spec anchors and `ticket_stem` for ticket references.
- `query` is a case-insensitive match over path, filename, title, summary, and
  spec text. Results include short snippets for disambiguation, not full bodies.
- `ticket_stem` matches ticket references found in spec frontmatter or feature
  entries, and also falls back to raw text containment for existing prose refs.
- Defaults to compact spec summary lines. Pass `format: "json"` for structured
  metadata.

Compatibility fallback:

```bash
ws-mcp specs find --spec-stem <spec-stem>
ws-mcp specs find --ticket-stem <ticket-stem>
```

### `ws/specs.status`

Return locations and file metadata for one spec anchor stem.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "spec_stem": { "type": "string" },
    "format": { "type": "string" }
  },
  "required": ["spec_stem"]
}
```

Behavior:

- Looks up exact spec anchors by `spec_stem`.
- Returns all locations so duplicate anchors remain visible to callers.
- Defaults to compact location and file summary text. Pass `format: "json"` for
  structured metadata.
- Rejects ticket-only parameters such as `ticket_stem`; ticket and spec
  identifiers stay distinct.

Compatibility fallback:

```bash
ws-mcp specs status <spec-stem>
```

### `ws/project_tree`

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

The MCP output must preserve the workflow purpose: compact project orientation
for discussion and ticket planning.

### `ws/convention.read`

Read a bundled ws convention document by bare stem or filename.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Convention document stem or filename, for example spec-conventions."
    }
  },
  "required": ["name"]
}
```

Output:

- MCP text content containing the bundled Markdown convention document.

Constraints:

- `name` must be a bare stem or filename.
- Path separators are rejected.
- `.md` is appended when absent.
- The document is read from the runtime bundle, not from the downstream project
  filesystem.

Current bundled documents:

- `ticket-conventions`
- `spec-conventions`
- `mental-model-conventions`

### `ws/infra.read`

Read a bundled ws infra document by bare stem or filename.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Infra document stem or filename, for example impl-playbook."
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
- The document is read from the runtime's bundled infra document set.
- Use `ws/convention.read` for ticket, spec, and mental-model conventions.

Error behavior:

- Tool-level errors return `isError: true` with text content.
- Missing document, empty name, and path traversal attempts are tool errors.

## Candidate Read Surfaces

These names are reserved as likely next additions, but they are not implemented in
the current baseline.

| Candidate | Purpose | Current fallback |
|-----------|---------|------------------|
| `ws/project_index.read` | Read `ai-docs/_index.md` as project memory. | `cat ai-docs/_index.md` |
| `ws/ticket_queue.list` | Return active ticket stems grouped by status. | Direct file listing |

### `ws/spec_stem.generate`

Generate a collision-free spec anchor stem for a descriptive slug.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "slug": {
      "type": "string",
      "description": "Descriptive slug seed."
    },
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    }
  },
  "required": ["slug"]
}
```

Output:

- MCP text content containing the generated `YYMMDD-slug` stem.

### `ws/spec_index.verify`

Verify basic spec anchor index health.

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

- MCP text content reporting `Spec index: ok` or duplicate anchor findings.

This initial MCP surface is verification-only. It does not yet reproduce every
mutation performed by `ws-spec-build-index`; shared skills should still call the
MCP tool name so the runtime can grow behind a stable contract.

### `ws/mental_models.list`

List mental-model documents with domains, descriptions, and sources.

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

- MCP text content containing a compact `mental-models:` catalog.

### `ws/mental_models.find`

Find mental-model paths by query, spec anchor reference, or domain.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "query": { "type": "string" },
    "spec_stem": { "type": "string" },
    "domain": { "type": "string" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Scans `ai-docs/mental-model/` recursively.
- Defaults to compact mental-model summary lines with sources, ancestor/index
  hints, and short query snippets where relevant.
- Pass `format: "json"` for structured objects with path, domain, description,
  sources, matching spec refs, ancestor directory hints, nearby index hints, and
  short query snippets.
- `spec_stem` matches explicit spec anchors in frontmatter sources/spec fields
  or in document text.
- Does not return full mental-model bodies.

Compatibility fallback:

```bash
ws-mcp mental-models find --spec-stem <spec-stem>
ws-mcp mental-models find --domain <domain>
```

### `ws/mental_models.status`

Return path-first metadata for mental-model documents selected by domain or
path.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "domain": { "type": "string" },
    "path": { "type": "string" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Requires `domain` or a relative `path` under `ai-docs/mental-model/`.
- Defaults to compact mental-model summary lines. Pass `format: "json"` for the
  same metadata shape as `ws/mental_models.find`.
- Rejects spec-selection parameters; spec filtering belongs to
  `ws/mental_models.find`.

Compatibility fallback:

```bash
ws-mcp mental-models status --domain <domain>
ws-mcp mental-models status --path ai-docs/mental-model/<file>.md
```

### `ws/references.trace`

Trace ticket, spec, and mental-model references from one identifier.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": { "type": "string" },
    "ticket_stem": { "type": "string" },
    "spec_stem": { "type": "string" },
    "format": { "type": "string" }
  }
}
```

Behavior:

- Requires exactly one of `ticket_stem` or `spec_stem`.
- Defaults to compact text sections for tickets, specs, and mental models. Pass
  `format: "json"` for structured graph output.
- From `ticket_stem`, returns the ticket, specs referenced by ticket
  frontmatter or spec feature ticket refs, and mental models linked to those
  spec anchors.
- From `spec_stem`, returns spec anchor locations, tickets mentioning the spec
  stem, and mental models linked to the spec anchor.
- Composes the path-first metadata returned by the domain-specific discovery
  tools; it does not return full document bodies or edit documents.

Compatibility fallback:

```bash
ws-mcp references trace --ticket-stem <ticket-stem>
ws-mcp references trace --spec-stem <spec-stem>
```

### `ws/subquery`

Start a scoped one-turn codebase or documentation query and return immediately
with a subquery key.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
    "question": {
      "type": "string",
      "description": "Scoped question to answer."
    },
    "deep_research": {
      "type": "boolean",
      "description": "Use deep model alias for broad tracing or research."
    }
  },
  "required": ["question"]
}
```

Behavior:

- The tool is the MCP replacement for the old `ws-subquery` CLI.
- It registers a generated `subquery-tmp<base36-id>` named agent and starts it through
  the same async path as `ws/agents.call`.
- Default model alias is `light`; `deep_research: true` uses `deep`.
- The system prompt is runtime-owned and self-contained.
- The delegate is instructed to answer one scoped question with cited English
  output, assumptions when inferred, and searched gaps when evidence is missing.
- Output contains `subquery_key`, agent status, worker pid, and follow-up
  `ws/agents.result` / `ws/agents.status` / `ws/agents.tail` /
  `ws/agents.cancel` calls.
- Retrieve the final answer with `ws/agents.result(name: "<subquery-key>",
  timeout_seconds: 600)`.

### `ws/path.generate`

Allocate worktree-scoped writable paths for file-backed workflow artifacts.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
    "kind": {
      "type": "string",
      "description": "Generated path kind. Initially supports review."
    },
    "stems": {
      "type": "array",
      "description": "Logical file stems to allocate in stable order.",
      "items": {
        "type": "string"
      }
    }
  },
  "required": ["kind", "stems"]
}
```

Behavior:

- Initial supported kind is `review`.
- Review paths are allocated under the worktree-local `review-paths/` state
  directory.
- Stems are sanitized before path use.
- Returned paths are reserved as empty writable files and preserve input order.
- Filenames use `<seed8>-<index>-<stem>.md`; invocation time is intentionally
  omitted from the filename to reduce prompt and review-path context cost.

### `ws/config.show`

Return the current ws user-local configuration and the absolute path where it is read from.

Input fields: none.

Behavior:

- The tool is read-only and does not create or modify `config.json`.
- If no config file exists, the response contains the default config shape and
  the path where config would be read from. Default Codex model aliases are
  `light` → `gpt-5.4-mini`, `core` → `gpt-5.5`, and `deep` → `gpt-5.5`;
  default Claude aliases use `haiku`, `sonnet`, and `opus`.
- The default response is compact labeled text. Pass `format: "json"` for the
  structured `path` and `config` fields.
- Delegate and leaf MCP tool profiles hide and reject this tool together with
  the other `config.*` tools when optional profile filtering is active.

### `ws/config.agents_tier`

Compatibility surface for configuring the user-local backend/model mapping for
a model alias in the selected or current harness.

Input fields:

- `tier` is required for compatibility and accepts `light`, `core`, or `deep`;
  it names the alias to configure.
- `model` is the concrete model to use when an agent registration resolves to
  that alias.
- `backend` is optional. When omitted, ws infers it from recognizable model
  names: `gpt-*` or names containing `codex` use `codex`; names containing
  `gemini` use `gemini`; names containing `haiku`, `sonnet`, `opus`, or
  `claude` use `claude`.
- `harness` is optional and accepts `codex`, `claude`, `gemini`, or `default`.
  When omitted, MCP calls use the detected session harness when one is known;
  callers without a detected harness update `default`.
- `effort` is optional and accepts `low`, `medium`, `high`, or `xhigh`. Empty,
  omitted, or `none` means no backend effort override for this alias.

Behavior:

- Configuration is written to `~/.cache/ws@kang-sw-devenv/config.json`, or to
  `$WS_CACHE_HOME/config.json` when `WS_CACHE_HOME` is set.
- The stored alias entry is `model_aliases.<tier>.<target>`, where `<target>` is
  the explicit harness, detected MCP session harness, or `default`. A non-empty
  effort is stored on that alias entry with the resolved backend and model.
- Missing alias mappings in an existing config are backfilled from default Codex
  and Claude alias mappings without overwriting user-provided entries.
- Updating an alias with empty, omitted, or `none` effort clears any previously
  stored effort, preserving the no-override behavior.
- `agents.register` applies alias mapping after explicit model selection and
  prompt frontmatter resolution. Concrete model names bypass alias mapping while
  still allowing backend inference when `backend` is omitted. Resolved alias
  effort is stored with the agent and applied by backend runners only when
  non-empty; Codex uses `model_reasoning_effort=<value>` and Claude uses
  `--effort <value>`.

Public `agents.*` schemas intentionally omit `root`; establish the current
worktree with `ws/ws.setup` before normal calls. Explicit `root` arguments may
still work as a compatibility override, but they are not the advertised caller
surface.

### `ws/agents.register`

Register or replace one task-scoped named agent.

Important input fields:

- `name` is required.
- `backend` is optional; when omitted, model aliases use the detected MCP
  harness and concrete model names infer a backend before falling back to
  `codex`.
- `model` may be a portable alias (`light`, `core`, or `deep`) or a concrete
  backend model override. Concrete model names take precedence over aliases.
- `tier` is a deprecated compatibility alias selector used only when `model` is
  absent.
- There is no direct `effort` input. Named-agent effort is configured only on
  model aliases through `ws/config.agents_tier` or the CLI mirror
  `ws-mcp config agents-tier --effort`.
- `prompts` is the canonical prompt chain field.
- `prompt_refs` is a migration alias for older callers.
- `system_prompt_text` appends materialized system instructions after resolved
  prompt bodies.

Prompt resolution:

- Bare prompt stems resolve from the embedded runtime prompt bundle.
- Absolute paths read directly from the local filesystem.
- Ambiguous relative paths and traversal-like prompt specs are rejected.
- YAML frontmatter is stripped before materialization.
- Prompt bodies are concatenated in input order with `---` separators.
- The materialized prompt is written to the agent's `system.md`.
- Public `agents.register` calls prepend `delegate-orientation`; internal
  helpers such as `subquery` suppress that orientation and use their own scoped
  system prompt.

Current embedded prompt stems:

- `api-doc-cargo-brief`
- `api-doc-manager`
- `code-review-correctness`
- `code-review-fit`
- `code-review-test`
- `code-reviewer`
- `delegate-orientation`
- `impl-playbook`
- `implementer`
- `mental-model-updater`
- `plan-populator-research`
- `plan-populator-survey`
- `pre-router`
- `project-survey`
- `skeleton-populator`
- `skeleton-reviewer`
- `sprint-survey`

### `ws/agents.call`

Start an asynchronous call for a registered ws agent and return immediately.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Agent name."
    },
    "prompt": {
      "type": "string",
      "description": "Prompt to send to the agent."
    }
  },
  "required": ["name", "prompt"]
}
```

Output:

- MCP text content containing the agent name, current-call status, and worker pid.

Behavior:

- The tool writes the prompt snapshot to the named agent's `current/prompt.md`.
- The runtime starts a separate `ws-mcp agents run-current` worker process and
  returns before the backend finishes.
- Backend stdout and stderr are captured in the agent's `current/stdout` and
  `current/stderr` files.
- Completion updates `agent.json`, `output.md`, `events.jsonl`, and
  `current/state.json`.
- A second active call for the same named agent is rejected as busy.

### `ws/agents.wait`

Wait for one or more registered ws agents to become ready.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Agent name. Compatibility alias for a single name."
    },
    "names": {
      "type": "array",
      "description": "Agent names to wait for.",
      "items": {
        "type": "string"
      }
    },
    "timeout_seconds": {
      "type": "number",
      "description": "Maximum seconds to wait. Defaults to 600."
    }
  }
}
```

Output:

- Ready calls return metadata with `ready: true`, `terminal: true`, and
  `result_available: true` only for completed calls.
- Running or queued calls return metadata with `ready: false` and `active: true`.
- Timed-out calls return `wait_timeout: true` followed by readiness metadata for
  every requested agent.
- `wait` never returns final output.
- Default timeout is 600 seconds; explicit bounded waits for normal agent work
  should use at least 600 seconds.

### `ws/agents.result`

Return one completed agent result, optionally waiting first.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Agent name."
    },
    "timeout_seconds": {
      "type": "number",
      "description": "Maximum seconds to wait. Omit or set 0 for non-blocking."
    }
  },
  "required": ["name"]
}
```

Output:

- Completed calls return the final plain-text `output.md`.
- Omitted or zero timeout returns a non-ready status immediately when output is
  not yet available.
- Timed-out calls return `result_timeout: true` followed by status text.
- Failed and cancelled calls return non-consuming status text.
- Successful result reads erase agents marked ephemeral, such as subqueries.

### `ws/agents.status`

Return current agent and current-call state without invoking a backend.

Output includes stable line keys such as `agent`, `agent_status`, `backend`,
`session_id`, `call_status`, `pid`, `started_at`, `updated_at`, `finished_at`,
`exit_code`, and `error` when those fields exist.

### `ws/agents.tail`

Return context-bounded recent diagnostic lines without invoking a backend.

Input adds optional `lines` per section, defaulting to `40`. Output contains
sections for `events`, `stdout`, `stderr`, and `output`.

Normal tail output truncates large stream fields such as `aggregated_output`
and long lines with an explicit `ws-tail truncated` marker. Use
`ws/agents.debug.tail` or stream-specific `ws/agents.debug.*` tools only when
raw diagnostic content is required.

Workflow skills should pass `lines: 3` for routine progress checks. Larger
tails are reserved for diagnosing concrete failures where recent timestamps and
status lines are insufficient.

### `ws/agents.cancel`

Best-effort cancel the current async call for a registered ws agent.

Behavior:

- If `current/state.json` is `queued` or `running`, the runtime attempts to kill
  the stored local worker pid when one exists.
- The agent is marked idle and the current call is marked `cancelled`.
- The tool returns `ws/agents.status` text after the cancellation attempt.
- Cancelled status text includes a `cancel_recovery_tip` telling callers to
  retry `agents.call` on the same registered agent when cancellation followed a
  no-result timeout.
- After an MCP server restart, cancellation is only as strong as the retained
  local pid. Backend-specific process-group cleanup is not implemented yet.
- This is not the normal continuation or redirect path; use `ws/agents.call` for
  ordinary next turns and `ws/agents.interrupt` for active redirects.

## Deferred Write-Capable Operations

The following behavior is intentionally out of scope for the first MCP contract:

- creating or editing tickets
- mutating spec indexes beyond verification
- writing mental-model updates
- branch management, merge helpers, release helpers, and ship automation
- advanced agent operations beyond the current subquery and register/call/
  call_async/wait/status/tail/cancel/oneshot/print/erase prototype, including
  list, interrupts, runtime locks, and message queues

These operations have workflow semantics beyond file access. They should be
designed only after the read surfaces and plugin-managed MCP distribution path are
validated.

## Version And Drift Boundary

The current binary reports `0.26.4-dev`. The runtime contract lets launcher
startup detect drift between the installed plugin bundle and the local `ws-mcp`
binary.

Use `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>` to keep the
plugin/runtime version contract synchronized. The script updates the Codex and
Claude adapter manifests, `agents-plugin/runtime.json`, `ws-mcp` dev defaults,
release workflow defaults, and current runtime documentation. It intentionally
preserves historical ticket result text.

Expected direction:

- plugin documents carry a small runtime contract file
- the plugin-local launcher reads that file before starting `ws-mcp`
- `ws-mcp doctor` can also read that file for direct diagnostics
- runtime cache reuse is strict to the plugin patch version; development
  binaries such as `X.Y.Z-dev` satisfy plugin `X.Y.Z`, but older or newer patch
  releases are stale
- missing or incompatible binaries can trigger automatic first-run download when
  network access is available
- contracts can opt into exact runtime capability matching with
  `runtime_capabilities.match: exact`; exact contracts reject extra tool or
  command names and do not use weaker fallback surface checks after a capability
  mismatch
- stale binaries produce actionable diagnostics when automatic repair is not
  possible

Until that exists, skill documents must not assume that installed plugin text and
installed `ws-mcp` binary are automatically in sync.

## Release Distribution

`ws-mcp` is distributed as prebuilt native binaries produced from
`agents-plugin-tool/`. End users should not need Go, Node, Cargo, or Visual
Studio Build Tools to run the MCP server. Plugin-managed startup does require a
working `python3` command for the small cross-platform launcher.

Release asset names follow this pattern:

```text
ws-mcp-<os>-<arch>[.exe]
```

Initial assets:

```text
ws-mcp-darwin-arm64
ws-mcp-darwin-amd64
ws-mcp-linux-amd64
ws-mcp-linux-arm64
ws-mcp-windows-amd64.exe
ws-mcp-windows-arm64.exe
SHA256SUMS
```

`SHA256SUMS` is a plain `shasum -a 256` manifest covering every release binary.
The launcher should download the selected binary and `SHA256SUMS`, verify the
matching checksum line before chmod/exec, and print failures to stderr only.

`agents-plugin/runtime.json` currently carries `release_repository` and
`release_tag`. The Python launcher derives the release base URL as:

```text
https://github.com/<release_repository>/releases/download/<release_tag>
```

It then downloads `ws-mcp-<os>-<arch>[.exe]` and `SHA256SUMS`. The launcher
checks an existing cache-local binary first; compatible `0.26.x` binaries run
without network access, while missing or incompatible binaries trigger repair.

Runtime binaries live in the installed plugin cache by default:

```text
<installed-plugin-cache>/.runtime/<os>-<arch>/ws-mcp[.exe]
```

This keeps plugin document updates and runtime binaries version-scoped together.
When a plugin reinstall or plugin version update creates a fresh cache copy, the
next MCP startup may redownload the binary for that plugin/runtime contract. If
the binary already exists and `ws-mcp version` satisfies `runtime.json`, startup
should not perform routine network work.

Update/drift behavior:

- Missing binary: download the release asset for the installed plugin/runtime
  contract when a release URL is configured.
- Compatible binary: run without network access.
- Incompatible version: replace it if release download and checksum verification
  succeed; otherwise fail with an actionable stderr diagnostic.
- Incompatible tool surface: prefer `runtime capabilities`; replace the binary
  if any required tool or command is missing, or if an exact contract reports
  any extra tool or command. Legacy `tools/list` and CLI fan-out checks are a
  fallback only for non-exact contracts.
- Offline/proxy failure: keep stdout clean, fail startup, and tell the user which
  URL or runtime directory needs manual repair.

Local devenv development exception:

- When the installed plugin path is under
  `~/.codex/plugins/cache/kang-sw-devenv/ws/` and the installed plugin cache
  contains `.local-devenv-runtime`, the Python launcher may copy a local runtime
  binary from `~/devenv/agents-plugin-tool/dist/` or
  `~/devenv/agents-plugin/.runtime/` before attempting GitHub release download.
  If those local binaries are absent or fail the tool-surface check, it may build
  `~/devenv/agents-plugin-tool/cmd/ws-mcp` directly into the cache-local runtime
  path when Go is available.
- This path exists only for the repository-local Codex plugin development loop.
  The marker file is gitignored and should exist only in the local source tree;
  it does not trigger for normal GitHub release installs, downstream
  repositories, or Windows.

The local build script is:

```bash
agents-plugin-tool/scripts/build-release-assets.sh [version]
```

GitHub Actions workflow `.github/workflows/ws-mcp-release.yml` runs tests,
cross-compiles assets, uploads workflow artifacts, and publishes GitHub release
assets for pushed `v*` tags. Pull requests that touch the workflow or
plugin/runtime paths run the same checks without publishing release assets.
`workflow_dispatch` is present for manual verification runs, but GitHub only
accepts that trigger when the workflow file exists on the default branch. The
workflow currently uses official GitHub actions `actions/checkout@v5`,
`actions/setup-go@v6`, and `actions/upload-artifact@v7`.

Windows host verification remains separate from Go cross-compilation. The
release workflow builds a native Windows `ws-mcp` executable and runs one
`smoke --root <repo>` process, which performs version, doctor, runtime info, and
stdio MCP checks internally. Parallels can still verify plugin-managed startup
from the installed plugin cache with `python3 ./bin/ws-mcp-launcher.py version`.
If `python3` resolves to the Windows Store alias instead of an installed
interpreter, install Python 3 and refresh the plugin so Codex rematerializes the
plugin-managed MCP entry.

## Development Verification

Use three verification levels while developing `ws-mcp` and the plugin-managed
runtime.

Level 1 validates the Go runtime and host-independent MCP server:

```bash
cd agents-plugin-tool
go test ./...
scripts/smoke-ws-mcp.sh ..
```

Level 2 validates the local release assets without waiting for GitHub Actions:

```bash
cd agents-plugin-tool
scripts/build-release-assets.sh 0.26.4-dev
dist/ws-mcp-darwin-arm64 version
cd dist
shasum -a 256 -c SHA256SUMS
```

This is the same build/checksum path used by the GitHub Actions workflow. CI is
a wrapper around the local script, not the only way to validate release assets.

Level 3 validates Codex plugin-managed MCP startup from the installed plugin
cache. This level requires a human-in-the-loop Codex plugin cache refresh when
plugin files change: uninstall/install `ws` in the Codex UI or start a fresh
session after cache refresh is available. Then run:

```bash
codex mcp get ws
codex exec --dangerously-bypass-approvals-and-sandbox --json \
  'There is an enabled MCP server named ws. Use its tool named project_tree with arguments {"root":"/Users/kang-sw/devenv"}. Do not use shell commands. Reply with the exact server name, exact tool name, and the first non-empty line of the tool result.' \
  < /dev/null
```

Success means the JSONL output contains an MCP tool call with server `ws`, tool
`project_tree`, and a result whose first non-empty line is `ai-docs/`.

Use Level 1 for ordinary Go/MCP changes, Level 2 for release/build changes, and
Level 3 whenever `.codex-plugin/plugin.json`, `.mcp.json`, launcher behavior, or
installed plugin packaging changes.
