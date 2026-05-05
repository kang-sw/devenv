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
- Codex and Claude plugin runtime paths must remain adapter-specific. Codex uses
  plugin cache copies and plugin-managed MCP configuration; Claude currently uses
  the stable `claude-plugin/` package, plugin update timing, and `bin/`/PATH
  helper behavior. Shared runtime contracts may converge later, but installation
  and update mechanics should not be forced into one path during this ticket.
- Windows plugin-managed launcher behavior is a deferred host-smoke item, not a
  blocker for continuing Unix/macOS skill migration. If Codex on Windows does not
  resolve `./bin/ws-mcp-launcher` to `./bin/ws-mcp-launcher.exe`, fallback in this
  order: native Go launcher with shared runtime behavior, host-specific
  `.mcp.json` adapter artifacts, then a documented one-time `codex mcp add` or
  repair/setup skill path that points directly at the downloaded Windows
  `ws-mcp` binary.

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

### Phase 3: Plugin-local launcher POC

Prototype Codex plugin-managed MCP through a plugin-local launcher instead of a
mandatory separate install skill. The preferred Codex shape is:

```text
agents-plugin/
  .codex-plugin/plugin.json   # references "./.mcp.json"
  .mcp.json                   # runs a plugin-local launcher
  bin/                        # launcher scripts or launcher binary
  runtime.json                # plugin/runtime compatibility contract
```

The launcher should:

- run from the installed Codex plugin cache when Codex starts the MCP server
- read the plugin-local `runtime.json`
- detect OS and architecture
- check whether a compatible `ws-mcp` binary already exists in the selected
  cache-local or user-local runtime location
- download a prebuilt binary and checksum when the binary is missing or
  incompatible; automatic first-run download is allowed for this plugin
- verify the checksum before executing downloaded binaries
- exec `ws-mcp serve --stdio` without writing non-MCP text to stdout
- print actionable diagnostics to stderr when download, checksum, permission, or
  version checks fail

This phase must answer these host questions with a small Codex POC:

- whether `.mcp.json` command paths can be relative to the installed plugin cache
- whether Codex starts the MCP server with the plugin cache as the working
  directory, or whether the launcher must discover its own location
- whether the POSIX `sh` launcher is sufficient as the canonical macOS/Linux
  entrypoint, with internal `uname`-based selection of native `ws-mcp` binaries
- whether Windows resolves `command: "./bin/ws-mcp-launcher"` to
  `./bin/ws-mcp-launcher.exe`; if not, the project needs a Windows-specific plugin
  artifact/manifest or a one-time global MCP setup path
- whether a fresh `codex exec` can see and call `ws.project_tree` from the
  installed plugin-managed MCP server after the user refreshes the plugin cache

Initial dev POC status:

- `agents-plugin/.codex-plugin/plugin.json` now references plugin-local
  `.mcp.json`.
- `agents-plugin/.mcp.json` runs `./bin/ws-mcp-launcher` with `cwd: "."`.
- `agents-plugin/runtime.json` records the plugin/runtime compatibility contract.
- `agents-plugin/bin/ws-mcp-launcher` can copy a local bootstrap binary or download
  a runtime binary, verify basic version compatibility, and exec
  `ws-mcp serve --stdio` without writing launcher diagnostics to stdout.
- A temporary global Codex MCP registration using the launcher confirmed that a
  fresh `codex exec` can call `ws.project_tree` through the `ws-mcp` server.
- OpenAI Codex plugin docs, Codex config reference, official plugin examples, and
  MCP transport docs do not show a standard OS/platform selector for `.mcp.json`.
  Platform variance should live inside the launcher or in host-specific plugin
  artifacts, not in the MCP config schema.
- User refreshed the Codex plugin cache through uninstall/install. Installed
  plugin-managed MCP verification succeeded after adding `cwd: "."` to
  `.mcp.json`.

`install-ws-plugin` is no longer a required setup skill if the launcher POC works.
It may be dropped, deferred, or re-scoped later as a repair/doctor skill for
offline environments, corporate proxy failures, permission issues, or manual
runtime recovery.

Success criteria:

- The POC does not require Go, Python, Node, Cargo, or Visual Studio Build Tools on
  target user machines.
- Plugin install in Codex can trigger MCP startup through plugin-managed config
  without a mandatory user-run setup skill.
- A fresh `codex exec` can list or call the `ws` MCP server after the user
  refreshes the installed plugin cache.
- The ticket records whether relative command paths and platform-specific
  launchers are viable for production.

### Result (d1ab6b0) - 2026-05-03

Completed the macOS plugin-managed MCP POC for Codex.

Key findings:

- Codex installs the local `ws` plugin cache with `.mcp.json`,
  `bin/ws-mcp-launcher`, `runtime.json`, and the ignored dev runtime binary when
  the user refreshes the plugin through UI uninstall/install.
- `command: "./bin/ws-mcp-launcher"` without `cwd` is not sufficient. Codex
  registers the MCP server, but startup fails with `No such file or directory`
  because the relative command is not resolved from the plugin cache.
- Adding `cwd: "."` to plugin-local `.mcp.json` makes Codex normalize cwd to the
  installed plugin cache directory. With that field, a fresh `codex exec` called
  server `ws`, tool `ws.project_tree`, arguments
  `{"root":"/Users/kang-sw/devenv"}`, and received project-tree output beginning
  with `ai-docs/`.
- The installed launcher itself also passes direct JSON-RPC smoke from the plugin
  cache and exposes `ws.project_tree` and `ws.infra.read`.
- macOS/Linux production can continue with the POSIX `sh` launcher direction.
  Windows remains a Phase 4 or separate host-smoke risk: verify whether Codex on
  Windows can use `./bin/ws-mcp-launcher.exe` or needs a Windows-specific
  manifest/artifact path.

Verification:

- `find ~/.codex/plugins/cache/kang-sw-devenv/ws/0.1.0 -maxdepth 4 -type f`
- `codex mcp get ws`
- direct installed-cache JSON-RPC smoke through
  `~/.codex/plugins/cache/kang-sw-devenv/ws/0.1.0/bin/ws-mcp-launcher`
- `codex exec --dangerously-bypass-approvals-and-sandbox --json` tool call to
  `ws.project_tree`

### Phase 4: Release distribution design

Define the portable binary distribution plan after the launcher POC proves the
host mechanics:

- release asset naming for Windows, macOS, and Linux
- checksum file format and verification behavior
- runtime binary location policy for cache-local versus user-local storage
- `runtime.json` compatibility fields and drift behavior
- launcher update policy: first-run download is allowed, but routine update checks
  should avoid surprising network work unless the runtime contract requires it
- offline/proxy failure UX and any optional repair skill
- CI cross-compilation matrix
- manual host smoke checklist for macOS, Linux, and Windows

Success criteria:

- Windows installation is described as downloading a prebuilt `.exe`, not building
  locally.
- Runtime drift produces an actionable diagnostic or verified automatic first-run
  repair instead of silently exposing tools that are too old for the installed
  skill documents.
- Offline/proxy failure behavior is documented.

### Result (2514242) - 2026-05-03

Defined and scaffolded the release distribution path.

Decisions:

- Release assets use `ws-mcp-<os>-<arch>[.exe]` names.
- The initial build set is darwin arm64/amd64, linux arm64/amd64, and windows
  arm64/amd64.
- Checksums are published as a single `SHA256SUMS` file generated by
  `shasum -a 256`.
- Runtime binaries stay plugin cache-local under
  `<installed-plugin-cache>/.runtime/<os>-<arch>/ws-mcp[.exe]`.
- Plugin reinstall or version update may redownload the runtime binary because a
  fresh plugin cache copy has no runtime binary. Normal MCP startup should avoid
  network work when the cache-local binary exists and satisfies `runtime.json`.
- Offline/proxy failures should fail MCP startup with stderr diagnostics while
  preserving stdout for MCP JSON-RPC only.
- GitHub Actions should build assets on branch pushes and pull requests for
  pre-merge validation, while publishing release assets only from pushed `v*`
  tags.

Implementation:

- `agents-plugin-tool/scripts/build-release-assets.sh` cross-compiles release
  assets and generates `SHA256SUMS`.
- `.github/workflows/ws-mcp-release.yml` runs Go tests, builds release assets,
  uploads workflow artifacts for branch/PR validation, and publishes assets to
  the GitHub release for pushed `v*` tags.
- `ws-mcp` version is now build-time injectable with Go `-ldflags`.
- `agents-plugin-tool/dist/` is ignored as generated output.

Open host item:

- Windows plugin-managed startup still needs Parallels/manual verification. The
  cross-compiled `ws-mcp-windows-amd64.exe` can verify the runtime, but native
  plugin startup also needs `bin/ws-mcp-launcher.exe` or a Windows-specific
  adapter path because the current committed launcher is POSIX `sh`.

Verification:

- `go test ./...` from `agents-plugin-tool/`
- `agents-plugin-tool/scripts/build-release-assets.sh 0.1.0-dev`
- `dist/ws-mcp-darwin-arm64 version`
- `shasum -a 256 -c SHA256SUMS` from `agents-plugin-tool/dist/`
- `git diff --check`

### Phase 5: Launcher release download

Implement the production download branch of the POSIX launcher now that Phase 4
has fixed the release asset naming and checksum format.

The launcher should:

- derive the default GitHub release asset base URL from `runtime.json`
- allow environment overrides for local smoke tests and emergency repair
- download `ws-mcp-<os>-<arch>[.exe]` and `SHA256SUMS`
- verify the matching checksum entry before installing the runtime binary
- skip network access when a compatible cache-local binary already exists
- replace an incompatible cache-local binary only after successful download and
  checksum verification
- keep stdout reserved for MCP JSON-RPC

Success criteria:

- Missing runtime binary can be repaired from a local release-asset directory
  using the same asset names and `SHA256SUMS` format as the GitHub release path.
- Existing compatible runtime binary runs without download.
- Incompatible runtime binary triggers replacement or a clear stderr failure.
- Level 1 and Level 2 verification still pass.

### Result (9502c69) - 2026-05-03

Implemented the production download branch in the POSIX launcher.

Changes:

- Added `release_repository` and `release_tag` to `agents-plugin/runtime.json`.
- The launcher now derives the default GitHub release asset base URL from those
  fields, with `WS_MCP_RELEASE_REPOSITORY`, `WS_MCP_RELEASE_TAG`, and
  `WS_MCP_RELEASE_BASE_URL` overrides for local smoke tests and repair workflows.
- Missing runtime binaries download `ws-mcp-<os>-<arch>[.exe]` and `SHA256SUMS`,
  verify the matching checksum line, install the binary under the cache-local
  runtime directory, and then exec it.
- Compatible cache-local `0.1.x` binaries run without network access.
- Incompatible cache-local binaries are replaced only through the same verified
  download path.
- Checksum verification supports `shasum` and `sha256sum`.

Verification:

- `go test ./...` from `agents-plugin-tool/`
- `scripts/smoke-ws-mcp.sh ..` from `agents-plugin-tool/`
- `jq . agents-plugin/runtime.json`
- `jq . agents-plugin/.mcp.json`
- `sh -n agents-plugin/bin/ws-mcp-launcher`
- `agents-plugin-tool/scripts/build-release-assets.sh 0.1.0-dev`
- `shasum -a 256 -c SHA256SUMS` from `agents-plugin-tool/dist/`
- launcher missing-binary repair from `file:///Users/kang-sw/devenv/agents-plugin-tool/dist`
- launcher compatible-binary reuse with a deliberately invalid release base URL
- launcher incompatible-binary replacement from local release assets
- launcher `serve --stdio` smoke through the verified download path

### Phase 6: Windows Go runtime smoke

Verify the cross-compiled Windows runtime through a native Windows process when a
Windows host or WSL2 bridge is available.

Scope:

- Run `ws-mcp-windows-amd64.exe version`.
- Run `ws-mcp-windows-amd64.exe doctor --root <repo>`.
- Run a stdio MCP smoke test against `ws-mcp-windows-amd64.exe serve --stdio`.

### Result (manual) - 2026-05-04

Verified the cross-compiled Windows amd64 runtime from WSL2 by running the
Windows executable through `cmd.exe`.

Findings:

- `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build` produced a PE32+ Windows
  console executable.
- Running the executable from Windows TEMP through `cmd.exe` returned
  `0.1.0-dev` for `version`.
- `doctor --root \\wsl.localhost\Ubuntu\home\swkang\devenv` passed and found the
  repo root, `ai-docs`, `agents-plugin`, `claude-plugin`, and project index.
- A JSONL stdio smoke through `cmd.exe` initialized the server, listed tools,
  returned `runtime.info`, and called `project_tree`.

Limits:

- This verifies the Go runtime and MCP stdio path under a native Windows process.
- This does not verify Codex on Windows resolving plugin-managed MCP command paths
  or choosing a Windows launcher artifact.

Verification:

- `cd agents-plugin-tool && go test ./...`
- `cd agents-plugin-tool && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o /tmp/ws-mcp-windows-amd64.exe ./cmd/ws-mcp`
- `cmd.exe /C "C:\Users\user\AppData\Local\Temp\ws-mcp-windows-amd64.exe version"`
- `cmd.exe /C "C:\Users\user\AppData\Local\Temp\ws-mcp-windows-amd64.exe doctor --root \\wsl.localhost\Ubuntu\home\swkang\devenv"`
- `cmd.exe /C "type C:\Users\user\AppData\Local\Temp\ws-mcp-smoke.jsonl | C:\Users\user\AppData\Local\Temp\ws-mcp-windows-amd64.exe serve --stdio --root \\wsl.localhost\Ubuntu\home\swkang\devenv"`

### Phase 7: Windows plugin-managed startup [deferred]

Verify the Windows plugin-managed startup assumptions when a native Windows Codex
host is available.

Scope:

- Use GitHub Actions `windows-latest` smoke as a preflight for the Windows
  executable before native plugin testing.
- Test whether Codex plugin-managed MCP resolves
  `command: "./bin/ws-mcp-launcher"` to `./bin/ws-mcp-launcher.exe`.
- If extensionless resolution fails, choose the documented fallback path:
  native Go launcher, host-specific `.mcp.json` adapter artifact, or one-time
  global MCP setup/repair skill.

This phase remains deferred. The runtime contract is considered adequate to
resume skill migration because the Go MCP baseline, plugin cache launcher path,
release asset build, checksum verification, production download branch, and
Windows Go runtime smoke have all been verified.
