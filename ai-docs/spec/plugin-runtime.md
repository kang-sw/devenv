---
title: Plugin Runtime
summary: Codex plugin packaging, runtime metadata, launcher repair, release assets, and runtime CLI surfaces for ws.
---

# Plugin Runtime

The ws plugin runtime gives Codex users a packaged `ws` plugin with bundled
workflow skills and a local MCP server entrypoint. It also defines how plugin
installations discover, verify, repair, and run the native `ws-mcp` runtime.

## Codex Plugin Manifest And Skill Bundle {#260505-codex-plugin-manifest-skill-bundle}

The Codex plugin manifest exposes the plugin as `ws`, declares the plugin
version, points Codex at the bundled `skills/` directory, and references the
plugin-local MCP server configuration.

Callers installing the plugin observe a Codex-facing skill namespace whose
workflow skills use the `lead-*` naming convention. The manifest describes the
plugin as an interactive/write-capable productivity plugin and includes default
prompts that point users at skill authoring workflows.

## Plugin-Local MCP Server Configuration {#260505-plugin-local-mcp-server-config}

The plugin ships an MCP configuration that starts the `ws` MCP server through
the plugin-local Python launcher:

```text
python3 ./bin/ws-mcp-launcher.py serve --stdio
```

The Claude-facing compatibility manifest inside `agents-plugin/` uses the same
Python launcher instead of the POSIX shell wrapper so native Windows startup
does not depend on `/bin/sh`.

The MCP server runs with the plugin directory as its configured working
directory. The bundled configuration gives the server a 30-second startup
timeout and a 600-second per-tool timeout so long-running ws orchestration calls
can complete without relying on user-local Codex configuration.

## Runtime Contract Metadata {#260505-runtime-contract-metadata}

The plugin runtime contract declares the plugin version, compatible `ws-mcp`
version range, release repository and tag, embedded prompt bundle metadata, and
the required MCP tool and CLI command surfaces.

The launcher and release checks use this metadata as the caller-visible
compatibility contract. A runtime is considered stale when it cannot satisfy the
declared version range, tool list, command list, or prompt bundle hash.

## wsflow Runtime Contract Mode {#260513-wsflow-runtime-contract-mode}

The shared runtime capability probe supports validating a reduced wsflow
surface. When wsflow starts the shared launcher with `WS_MCP_NO_AGENT=1`,
`WS_MCP_NAMESPACE=wsflow`, and `WS_MCP_SETUP_TOOL=setup`,
`runtime capabilities` reports the agentless tool and command list rather than
the full ws contract.

Unset environment variables preserve the full ws contract. wsflow contract
validation does not treat tool profiles as product modes; profile filters remain
containment and test surfaces only.

## 🚧 wsflow Agentless Plugin Package {#260513-wsflow-agentless-plugin-package}

The repository will ship `agents-plugin-wsflow/` as a curated internal
derivative plugin package named `wsflow`. Its Codex and Claude manifests expose
the wsflow name, its MCP configuration registers the server under the `wsflow`
key, and its package-local runtime contract requires the wsflow agentless
surface.

The package reuses the shared `ws-mcp` binary and launcher. Distributed
wsflow package text presents wsflow naming to users and does not describe the
package as a ws-lite or ws-compatible mode. Repository maintenance documents,
tests, compatibility comments, and hidden implementation details may still name
the shared ws implementation surface when that is the precise behavior under
test.

## Runtime Launcher Repair And Project-Root Detection {#260505-runtime-launcher-repair-project-root}

The plugin launcher resolves the current operating system and architecture,
selects the matching cache-local `ws-mcp` binary path, and ensures an executable
compatible runtime is present before delegating to it.

When the binary is missing or incompatible, the launcher can install a runtime
from an explicit bootstrap binary, a bootstrap URL, a local devenv runtime, or
the release asset URL declared in the runtime contract. Downloaded release
assets are verified against `SHA256SUMS` before becoming executable.

For plugin-managed Codex sessions, the launcher detects the caller project root
from the parent process environment when possible and exports it as
`WS_MCP_PROJECT_ROOT`, while avoiding the plugin cache directory itself as the
project root.

The launcher exports `WS_MCP_RUNTIME_BINARY` with the repaired runtime binary
path before executing `ws-mcp`. Async named-agent workers may use that path, or a
current plugin-cache launcher discovered from a stale executable path, when the
parent MCP process was launched from a plugin cache directory that has since
been replaced.

Compatible runtime startup avoids repeated full surface validation on the hot
path. Once a cache-local runtime binary and runtime contract have been validated
together, later launcher invocations can reuse that compatibility result until
the contract or binary identity changes, while install or repair paths still
fail closed into full validation before handoff.
{#260506-launcher-hot-path-compatibility-cache}

## Release Asset Build And Checksum Pipeline {#260505-release-asset-build-checksum-pipeline}

The runtime build script produces cross-platform `ws-mcp-*` release assets for
macOS, Linux, and Windows on supported arm64/amd64 targets, embedding the
runtime version and source commit into each binary.

The release artifact set includes `SHA256SUMS`. The release workflow verifies
release asset construction and includes Windows executable smoke coverage so
published runtime assets can be consumed by plugin installations.
Windows executable smoke uses a single `ws-mcp smoke --root <repo>` process that
performs version, doctor, runtime metadata, and stdio MCP checks internally.

## Runtime Version Bump Helper {#260505-runtime-version-bump-helper}

The version bump helper accepts a semantic `X.Y.Z` release version and updates
the plugin manifests, runtime contract version range, release tag, launcher
compatibility glob, Go runtime development version, release workflow references,
and selected project documentation references in one command.

The helper is the expected way to keep the plugin version, runtime version,
release tag, and compatibility range synchronized for a ws release.

## Runtime CLI Entrypoints {#260505-runtime-cli-entrypoints}

The `ws-mcp` binary exposes direct CLI entrypoints for local smoke tests,
fallback usage, and launcher compatibility checks.

Top-level commands include:

```text
version
doctor
runtime
serve
smoke
subquery
config
path
agents
git
tickets
specs
mental-models
references
```

`serve --stdio` is the MCP server entrypoint. `runtime info` reports runtime
version and prompt bundle metadata. `doctor` reports repository health. The
grouped commands mirror MCP behavior where a CLI fallback is part of the public
runtime surface.

### Single-Probe Runtime Capabilities {#260506-runtime-capabilities-single-probe}

`ws-mcp runtime capabilities` reports the runtime surfaces the plugin
launcher needs for compatibility checks in one JSON response. The response will
include the runtime version, source commit, MCP protocol version, prompt bundle
metadata, the exposed MCP tool names, and the public CLI command surface.

The capability response is the launcher-facing compatibility probe for new
runtimes. It must describe the full lead runtime surface used by the plugin
contract, independent of caller-local tool profile filters. Compatible new
runtimes can be validated without starting a temporary MCP server or invoking
each CLI command separately.

Old runtimes that do not provide the command are not silently trusted. During
the transition, the launcher either falls back to the existing bounded full
validation path or repairs the runtime before writing a compatibility stamp.

## Windows Plugin-Managed Startup {#260505-windows-plugin-managed-startup}

The runtime publishes Windows assets and verifies Windows executable startup in
release smoke coverage.

Native Windows plugin-managed startup uses the same Python launcher as macOS and
Linux. Codex materializes plugin-managed MCP entries during plugin install or
refresh, so changed `.mcp.json` launcher commands require plugin reinstall or
refresh before Windows startup verification.

Windows users need a working `python3` command on `PATH`. On Windows 11 the
Python Store alias may appear before Python is installed; if MCP startup reports
that Python cannot run the launcher, install Python 3 and refresh the plugin.
