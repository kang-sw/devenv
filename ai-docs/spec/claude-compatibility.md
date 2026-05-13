---
title: Claude Compatibility
summary: Root Claude shim, agents-plugin compatibility metadata, install behavior, and retired legacy boundaries for ws.
---

# Claude Compatibility

Claude compatibility preserves the root `CLAUDE.md` shim and the
Claude-compatible metadata inside `agents-plugin/`. The canonical shared
context is `AGENTS.md`; the former `claude-plugin/` source tree is retired and
must not be reintroduced as a mirror target for Codex workflow behavior.
{#260510-claude-plugin-retirement-freeze}

## Claude Root Compatibility Shim {#260505-claude-root-compatibility-shim}

At the repository root, `AGENTS.md` is the canonical workflow context.
`CLAUDE.md` is a compatibility shim whose body is:

```text
@AGENTS.md
```

Claude sessions therefore load the same root workflow context as other agents.
When shared host-neutral guidance and Claude compatibility guidance conflict,
the conservative behavior applies and the conflict is surfaced before workflow
semantics change.

## Claude Plugin Compatibility Package {#260505-claude-plugin-compatibility-package}

The former `claude-plugin/` package has been removed from the live source tree.
Its skills, prompts, infra documents, hooks, bin scripts, and manifest are not
distributed or edited as active workflow surfaces.

Claude-compatible plugin behavior uses `agents-plugin/.claude-plugin/plugin.json`
and the shared `agents-plugin/` package. Historical material is preserved only
under `ai-docs/ref/` or git history.

## wsflow Claude-Compatible Package {#260513-wsflow-claude-compatible-package}

The wsflow derivative package carries its own package-local
`.claude-plugin/plugin.json` metadata and uses the same shared launcher pattern
as the active ws plugin package. Claude compatibility for wsflow must not
reintroduce a live `claude-plugin/` source tree or fork workflow behavior away
from the wsflow Codex manifest and runtime contract.

Claude-compatible wsflow metadata remains package-local to
`agents-plugin-wsflow/`, uses the `wsflow` MCP server key, and injects
`WS_MCP_NO_AGENT=1`, `WS_MCP_NAMESPACE=wsflow`, and
`WS_MCP_SETUP_TOOL=setup`. Validation for changes that touch Claude plugin
metadata should include the wsflow package when the derivative surface is
affected.

## Downstream Claude Bootstrap Shim {#260505-downstream-claude-bootstrap-shim}

The Codex bootstrap flow creates or upgrades downstream projects with
`AGENTS.md` as the canonical workflow context and `CLAUDE.md` as an
`@AGENTS.md` shim for Claude compatibility.

When a downstream project already has Claude-centered context, bootstrap
migration preserves relevant content by moving it into the canonical
`AGENTS.md` context instead of leaving divergent root instructions.

## Claude Install And Update Snapshot {#260505-claude-install-update-snapshot}

`install.sh` installs and updates the local ws plugin environment. It maintains
user configuration, generates marketplace metadata, snapshots `agents-plugin/`
into the Claude plugin cache when Claude Code is available, registers the local
marketplace, and installs the ws plugin for Claude Code.

Update mode refreshes configuration and plugin artifacts without repeating the
full package installation. The installed plugin is an isolated snapshot of
`agents-plugin/`, not a live reference to the working tree.

## Claude Global Home Instructions {#260505-claude-global-home-instructions}

The former installer-managed Claude home instructions are retained only as
historical reference material at `ai-docs/ref/claude-home-legacy.md`.

The installer no longer links repository-managed global Claude instructions.
Repository workflow semantics remain rooted in `AGENTS.md`, with root
`CLAUDE.md` as an `@AGENTS.md` shim.

## Claude ws CLI Fallback Suite {#260505-claude-ws-cli-fallback-suite}

The legacy `ws-*` CLI fallback suite was retired with the former
`claude-plugin/` source tree. Shared Codex/Agents skills use MCP tools and
bundled runtime documents instead of relying on host PATH injection.

## Claude Named-Agent CLI Runtime {#260505-claude-named-agent-cli-runtime}

Named-agent workflows are provided by the ws MCP runtime and its file-backed
agent implementation. The former Claude CLI wrappers are retired source
material.

## Claude Bin Windows Shims {#260505-claude-bin-windows-shims}

The retired legacy bin shims are historical only. New runtime command behavior
belongs in `agents-plugin-tool/cmd/ws-mcp` and must be covered by the runtime
CLI contract and cross-platform release checks.

## Claude API Docs CLI Fallback {#260505-claude-api-docs-cli-fallback}

The Codex-first API documentation surface is `ws/api.list`, `ws/api.ask`, and
the async API job tools `ws/api.ask_async`, `ws/api.status`, `ws/api.result`, and
`ws/api.cancel`.

## Claude Plugin Manifest And Marketplace {#260505-claude-plugin-manifest-marketplace}

The active plugin package declares a Claude compatibility manifest under
`agents-plugin/.claude-plugin/`. Installer-managed marketplace metadata exposes
the local ws plugin to Claude Code, and validation can be run against plugin
manifests before installation.

The Claude-compatible metadata is part of the active `agents-plugin/` package.
It must not fork workflow behavior away from the Codex plugin manifest and
runtime contract.
