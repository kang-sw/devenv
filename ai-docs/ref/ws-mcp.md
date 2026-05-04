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
ws-mcp subquery --root <repo-root> [--deep-research] <question>
```

`doctor` is a host-independent smoke check. In this repository it verifies the
repository root, `ai-docs/`, `agents-plugin/`, `claude-plugin/`, and
`ai-docs/_index.md`; downstream projects may not have the same plugin source
layout and should rely on MCP tools for conventions.

## Skill Porting Policy

Shared `agents-plugin` skill text assumes ws MCP is available. Porting should
preserve the source skill's wording and flow where possible, changing only the
host-specific tool calls, shell interpolations, slash-command syntax, and local
paths that would break outside Claude.

Do not point shared skill text at repository-local paths such as
`claude-plugin/infra/spec-conventions.md`. Convention documents are distributed
with the MCP runtime and read through `ws/convention.read`, so downstream
projects can use the same skill text without carrying this repository's
`claude-plugin/` source tree.

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
      "command": "./bin/ws-mcp-launcher",
      "cwd": ".",
      "args": ["serve", "--stdio"]
    }
  }
}
```

The current POC adds `agents-plugin/bin/ws-mcp-launcher` and
`agents-plugin/runtime.json`. The launcher is intentionally quiet on stdout:
stdout belongs to the MCP JSON-RPC stream, while diagnostics go to stderr.

The preferred Codex design is a plugin-local launcher, not a mandatory separate
install skill. The launcher runs from the installed plugin cache, reads a
plugin-local runtime contract, downloads or copies and verifies the prebuilt
`ws-mcp` binary when missing or incompatible, then execs
`ws-mcp serve --stdio`.

Codex does not resolve plugin-managed MCP `command` relative paths against the
plugin cache by default. The POC must set `"cwd": "."`; Codex normalizes that
value to the installed plugin cache directory, after which
`command: "./bin/ws-mcp-launcher"` starts successfully.

For macOS and Linux, `./bin/ws-mcp-launcher` is the canonical entrypoint and may
remain a POSIX `sh` script. A single script can branch internally on `uname -s`
and `uname -m`, then select an OS/architecture-specific native `ws-mcp` binary.
The command name can stay stable even though the selected binary differs.

Windows remains a separate production risk. The `.mcp.json` format does not
provide an OS selector, so the production path depends on whether Codex on
Windows resolves `command: "./bin/ws-mcp-launcher"` to
`./bin/ws-mcp-launcher.exe`. Windows verification is deferred until a host is
available; it should not block Unix/macOS skill migration.

If Windows does not resolve the extensionless launcher command to `.exe`, use
this fallback order:

1. Prefer a native Go launcher at `bin/ws-mcp-launcher.exe` plus the existing
   release-download/runtime-check logic. Keep the POSIX launcher for macOS/Linux.
2. If Codex still cannot select the Windows launcher from the shared
   plugin-local `.mcp.json`, split the installed adapter artifact: keep shared
   skills/runtime metadata, but publish host-specific `.mcp.json` manifests for
   Windows and Unix-like hosts.
3. If plugin-managed Windows MCP remains blocked by Codex host behavior, provide
   a repair/setup skill or documented one-time `codex mcp add` path that points
   directly at the downloaded `ws-mcp-windows-<arch>.exe`.

All fallback paths must preserve the same `runtime.json` compatibility contract,
cache-local runtime location, release asset naming, and checksum verification
policy.

Current launcher inputs:

| Variable | Purpose |
|----------|---------|
| `WS_MCP_RUNTIME_DIR` | Override the runtime binary directory; defaults to plugin-local `.runtime/<os>-<arch>`. |
| `WS_MCP_BOOTSTRAP_BINARY` | Copy a prebuilt local binary into the runtime directory. Used by the current dev POC. |
| `WS_MCP_BOOTSTRAP_URL` | Download a prebuilt binary when no runtime binary exists. |
| `WS_MCP_BOOTSTRAP_SHA256` | Optional SHA-256 checksum for `WS_MCP_BOOTSTRAP_URL`. |
| `WS_MCP_RELEASE_REPOSITORY` | Override the GitHub release repository from `runtime.json`, for example `kang-sw/devenv`. |
| `WS_MCP_RELEASE_TAG` | Override the release tag from `runtime.json`, for example `v0.1.0`. |
| `WS_MCP_RELEASE_BASE_URL` | Override the full release asset base URL; useful for local file or HTTP smoke tests. |
| `WS_MCP_LAUNCHER_DEBUG` | Print launcher diagnostics to stderr when set to `1`. |
| `WS_MCP_PROJECT_ROOT` | Project root used as the default when a tool or CLI command omits `root`; normally derived by the launcher from the parent Codex process. |

The macOS plugin-managed MCP POC is proven for `codex exec` when `.mcp.json`
sets `cwd: "."`. Without that field, Codex registers the server but startup fails
with `No such file or directory` because the relative command is interpreted from
the workspace process context, not the plugin cache. Windows extensionless `.exe`
resolution remains a later host verification item.

Because `cwd: "."` points the MCP process at the installed plugin cache, tools
must not treat process cwd as the downstream project root. The POSIX launcher
derives `WS_MCP_PROJECT_ROOT` from the parent Codex process `PWD` when possible,
and `ws-mcp` uses that environment variable whenever a tool or command omits
`root` or passes `"."`. Skills may still pass `root` explicitly, but omitted
root should resolve to the active project in plugin-managed Codex sessions.

## Orchestrator Authority

`ws-mcp` assigns lead orchestration authority through a worktree-local lock under
the ws cache root:

```text
~/.cache/ws@kang-sw-devenv/proj/<worktree-key>/locks/orchestrator.lock
```

The first live MCP server for a worktree owns the lock and receives the base
`lead` role. Later MCP servers for the same worktree receive the base
`delegate` role. Linked worktrees use different worktree keys, so each linked
worktree may have an independent lead server.

`WS_MCP_TOOL_PROFILE` is an additional restriction only. The effective role is
the minimum of the lock-derived base role and the requested profile, ordered
`lead > delegate > leaf`. A non-owner cannot regain lead tools by setting
`WS_MCP_TOOL_PROFILE=lead`; a lock owner can still voluntarily reduce itself to
`delegate` or `leaf`.

Delegate and leaf roles hide and reject lead-owned orchestration or mutation
tools, currently `agents.*` and `config.*`. Leaf also hides `subquery`.
`WS_MCP_ALLOWED_TOOLS` can narrow the resulting visible surface for tests or
debugging, but it cannot bypass the effective role.

For repo-local Codex plugin iteration, changed plugin-managed MCP configuration
requires a human-in-the-loop cache refresh: the user must uninstall/install the
plugin in the Codex UI or start a fresh session before installed-plugin
verification. Agents should explicitly ask for that refresh when validation
depends on the installed plugin cache.

## MCP Tool Contract

### `ws/runtime.info`

Return runtime metadata used for plugin compatibility checks.

Output:

- MCP text content containing JSON with `version`, `source_commit`, and
  `prompt_bundle`.
- `prompt_bundle` contains `source_commit`, `content_sha256`, and `prompts`.

Behavior:

- The POSIX plugin launcher compares `prompt_bundle.content_sha256` with
  `agents-plugin/runtime.json`.
- A mismatch causes the launcher to repair the cache-local runtime binary just
  as tool or command drift does.

### `ws/api.list`

Return sorted third-party API documentation cache domain names.

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

- MCP text content containing a JSON array of domain directory names from
  `ai-docs/.deps/`.
- Dot-prefixed directories and non-directories are excluded. Missing `.deps`
  returns an empty array.

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
- Output preserves per-domain boundaries. If at least one domain succeeds, failed
  domains are included as explicit error blocks; if all domains fail, the tool
  returns a tool error.

Constraints:

- The public MCP surface intentionally exposes only `ws/api.list` and
  `ws/api.ask`; refresh and stale-check operations remain manager-internal.
- Worker-facing guidance should use these tools and should not direct ordinary
  workers to read `ai-docs/.deps/` directly.

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

Compatibility fallback:

```bash
ws-proj-tree
```

The MCP output does not have to be byte-identical to the legacy helper, but it
must preserve the same workflow purpose: compact project orientation for
discussion and ticket planning.

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
- The document is read from this repository's local `claude-plugin/infra/` tree.
  Shared `agents-plugin` skills should use `ws/convention.read` instead.

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
| `ws/project_index.read` | Read `ai-docs/_index.md` as project memory. | `cat ai-docs/_index.md` |
| `ws/ticket_queue.list` | Return active ticket stems grouped by status. | `ws-proj-tree` or direct file listing |

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

### `ws/subquery`

Run a scoped one-turn codebase or documentation query through a temporary ws
delegate.

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
      "description": "Use deep workload tier for broad tracing or research."
    }
  },
  "required": ["question"]
}
```

Behavior:

- The tool is the MCP replacement for the old `ws-subquery` CLI.
- It composes over `agents.oneshot`; no named agent session persists.
- Default workload tier is `light`; `deep_research: true` uses `deep`.
- The system prompt is runtime-owned and self-contained.
- The delegate is instructed to answer one scoped question with cited English
  output, assumptions when inferred, and searched gaps when evidence is missing.

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
  the path where config would be read from. Default Codex tier mappings are
  `light` → `gpt-5.4-mini`, `core` → `gpt-5.5`, and `deep` → `gpt-5.5`.
- The JSON response contains `path` and `config` fields.
- Delegate and leaf MCP tool profiles hide and reject this tool together with
  the other `config.*` tools.

### `ws/config.agents_tier`

Configure the user-local backend/model mapping for a workload tier.

Input fields:

- `tier` is required and accepts `light`, `core`, or `deep`.
- `model` is the concrete model to use when an agent registration resolves to
  that tier.
- `backend` is optional. When omitted, ws infers it from recognizable model
  names: `gpt-*` or names containing `codex` use `codex`; names containing
  `gemini` use `gemini`; names containing `haiku`, `sonnet`, `opus`, or
  `claude` use `claude`.

Behavior:

- Configuration is written to `~/.cache/ws@kang-sw-devenv/config.json`, or to
  `$WS_CACHE_HOME/config.json` when `WS_CACHE_HOME` is set.
- Missing tier mappings in an existing config are backfilled from the default
  Codex tier mappings without overwriting user-provided entries.
- `agents.register` applies this mapping after prompt frontmatter tier
  resolution. If no concrete model is provided by the call or tier mapping, the
  backend falls back to `codex`.
- Explicit `model` on `agents.register` bypasses tier configuration while still
  allowing backend inference when `backend` is omitted.

### `ws/agents.register`

Register or replace one task-scoped named agent.

Important input fields:

- `name` is required.
- `backend` is optional; when omitted, ws uses tier configuration or infers a
  backend from recognizable concrete model names before falling back to `codex`.
- `tier` may be `light`, `core`, or `deep`; omitted tier may be inferred from a
  prompt frontmatter model and defaults to `core`.
- `model` is an optional concrete backend model override. If `model` is present,
  it takes precedence over tier configuration.
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

- `code-reviewer`
- `implementer`
- `plan-populator-research`
- `plan-populator-survey`
- `project-survey`
- `skeleton-writer`
- `code-review-correctness`
- `code-review-fit`
- `code-review-test`
- `delegate-orientation`
- `impl-playbook`

### `ws/agents.call`

Start an asynchronous call for a registered ws agent and return immediately.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
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

Wait for the current async call for a registered ws agent.

Input schema:

```json
{
  "type": "object",
  "properties": {
    "root": {
      "type": "string",
      "description": "Repository root. Defaults to the server root."
    },
    "name": {
      "type": "string",
      "description": "Agent name."
    },
    "timeout_seconds": {
      "type": "number",
      "description": "Maximum seconds to wait. Defaults to no timeout."
    }
  },
  "required": ["name"]
}
```

Output:

- Completed calls return the final plain-text `output.md`.
- Timed-out calls return `timeout` followed by `ws/agents.status` text.
- Failed or cancelled calls return status text.

### `ws/agents.status`

Return current agent and current-call state without invoking a backend.

Output includes stable line keys such as `agent`, `agent_status`, `backend`,
`session_id`, `call_status`, `pid`, `started_at`, `updated_at`, `finished_at`,
`exit_code`, and `error` when those fields exist.

### `ws/agents.tail`

Return recent diagnostic lines without invoking a backend.

Input adds optional `lines` per section, defaulting to `40`. Output contains
sections for `events`, `stdout`, `stderr`, and `output`.

### `ws/agents.cancel`

Best-effort cancel the current async call for a registered ws agent.

Behavior:

- If `current/state.json` is `queued` or `running`, the runtime attempts to kill
  the stored local worker pid when one exists.
- The agent is marked idle and the current call is marked `cancelled`.
- The tool returns `ws/agents.status` text after the cancellation attempt.
- After an MCP server restart, cancellation is only as strong as the retained
  local pid. Backend-specific process-group cleanup is not implemented yet.

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

The current binary reports `0.1.0-dev`. Phase 3 must add a runtime contract that
lets `ws-mcp doctor` and server startup detect drift between the installed plugin
bundle and the local `ws-mcp` binary.

Expected direction:

- plugin documents carry a small runtime contract file
- the plugin-local launcher reads that file before starting `ws-mcp`
- `ws-mcp doctor` can also read that file for direct diagnostics
- major/minor compatibility is strict, patch compatibility can be flexible
- missing or incompatible binaries can trigger automatic first-run download when
  network access is available
- stale binaries produce actionable diagnostics when automatic repair is not
  possible

Until that exists, skill documents must not assume that installed plugin text and
installed `ws-mcp` binary are automatically in sync.

## Release Distribution

`ws-mcp` is distributed as prebuilt native binaries produced from
`agents-plugin-tool/`. End users should not need Go, Python, Node, Cargo, or
Visual Studio Build Tools to run the MCP server.

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
`release_tag`. The POSIX launcher derives the release base URL as:

```text
https://github.com/<release_repository>/releases/download/<release_tag>
```

It then downloads `ws-mcp-<os>-<arch>[.exe]` and `SHA256SUMS`. The launcher
checks an existing cache-local binary first; compatible `0.1.x` binaries run
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
- Incompatible tool surface: call `tools/list` before exec and compare it against
  `runtime.json.tools`; replace the binary if any required tool is missing.
- Offline/proxy failure: keep stdout clean, fail startup, and tell the user which
  URL or runtime directory needs manual repair.

Local devenv development exception:

- When the installed plugin path is under
  `~/.codex/plugins/cache/kang-sw-devenv/ws/` and the installed plugin cache
  contains `.local-devenv-runtime`, the POSIX launcher may copy a local runtime
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
cross-compiles assets, and uploads workflow artifacts on branch pushes and pull
requests that touch the workflow or `agents-plugin-tool/`. It publishes assets to
the GitHub release only for pushed `v*` tags. `workflow_dispatch` is present for
post-merge manual runs, but GitHub only accepts that trigger when the workflow
file exists on the default branch. The workflow currently uses official GitHub
actions `actions/checkout@v5`, `actions/setup-go@v6`, and
`actions/upload-artifact@v7`.

Windows host verification remains separate from Go cross-compilation and is a
deferred host-smoke item. Parallels can verify that `ws-mcp-windows-amd64.exe`
runs `version`, `doctor`, and stdio MCP smoke tests. Plugin-managed Windows
startup additionally needs either extensionless `.exe` resolution, a native
launcher artifact at `bin/ws-mcp-launcher.exe`, an adapter-specific manifest, or
a documented one-time global MCP setup fallback; the POSIX `sh` launcher only
proves the macOS/Linux path.

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
scripts/build-release-assets.sh 0.1.0-dev
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
