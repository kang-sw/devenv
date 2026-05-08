---
title: Claude Compatibility
summary: Compatibility shims, legacy Claude plugin packaging, CLI fallbacks, install behavior, and Windows shim coverage for ws.
---

# Claude Compatibility

Claude compatibility preserves the existing Claude Code workflow package while
the shared ws workflow moves toward host-neutral Agents/Codex conventions. The
canonical shared context is `AGENTS.md`; `claude-plugin/` remains the stable
Claude package and legacy reference surface.

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

`claude-plugin/` is the stable Claude compatibility package. It contains Claude
skills, prompts, infra/convention documents, hooks, and bin scripts used by
existing Claude workflows.

The package remains the legacy reference while new shared guidance moves toward
MCP tools and host-neutral skill text. Claude-specific commands and paths are
treated as adapter or fallback behavior rather than the preferred shared
surface.

## Downstream Claude Bootstrap Shim {#260505-downstream-claude-bootstrap-shim}

The Codex bootstrap flow creates or upgrades downstream projects with
`AGENTS.md` as the canonical workflow context and `CLAUDE.md` as an
`@AGENTS.md` shim for Claude compatibility.

When a downstream project already has Claude-centered context, bootstrap
migration preserves relevant content by moving it into the canonical
`AGENTS.md` context instead of leaving divergent root instructions.

## Claude Install And Update Snapshot {#260505-claude-install-update-snapshot}

`install.sh` installs and updates the local Claude plugin environment. It
installs global Claude home instructions, maintains user configuration,
generates marketplace metadata, snapshots `claude-plugin/` into the Claude
plugin cache, registers the local marketplace, and installs the ws plugin for
Claude Code.

Update mode refreshes configuration and plugin artifacts without repeating the
full package installation. The installed Claude plugin is an isolated snapshot
of `claude-plugin/`, not a live reference to the working tree.

## Claude Global Home Instructions {#260505-claude-global-home-instructions}

The installer links the repository copy of Claude home instructions from
`claude-plugin/CLAUDE.home.md` into the user's Claude home configuration.

Those instructions are Claude-specific. They define global Claude behavior such
as thinking/verdict guidance while repository workflow semantics remain rooted
in the project `AGENTS.md`.

## Claude ws CLI Fallback Suite {#260505-claude-ws-cli-fallback-suite}

`claude-plugin/bin/` provides legacy `ws-*` CLI fallbacks for Claude workflows
and local debugging. These tools cover project and infra reads, spec and
mental-model helpers, review path allocation, named and one-shot agents,
subquery execution, API documentation lookup, and workflow Git helpers.

Shared Codex/Agents skills prefer MCP tools when an equivalent exists. The CLI
suite remains available for Claude compatibility and for fallback paths until a
replacement MCP surface exists and the compatibility path is intentionally
retired.

## Claude Named-Agent CLI Runtime {#260505-claude-named-agent-cli-runtime}

The Claude named-agent CLI runtime provides durable named-agent workflows for
Claude-centered sessions. It resolves prompt references, maintains agent
registry/session state, supports named calls and erasure, resumes Claude
sessions, and exposes compatibility wrappers such as `ws-new-named-agent`,
`ws-call-named-agent`, and `ws-oneshot-agent`.

The runtime defaults to Claude behavior where appropriate and also contains a
Codex backend bridge for host-neutral migration and parity testing.

## Claude Bin Windows Shims {#260505-claude-bin-windows-shims}

Every new script added to `claude-plugin/bin/` must include a Windows-compatible
variant, such as a `.cmd` shim or equivalent wrapper. Existing bin tools include
matching Windows shims so Claude compatibility workflows can be invoked from
PowerShell or Cmd.

The shims dispatch to the underlying shell or Python implementation and preserve
the same caller-facing command names where Windows execution requires a
platform-specific wrapper.

## Claude API Docs CLI Fallback {#260505-claude-api-docs-cli-fallback}

Claude compatibility includes the legacy `ws-ask-api` and
`ws-ask-api-internal` CLI path. The CLI supports listing cached domains,
checking or refreshing staleness, exact domain-hint routing, pre-router
dispatch, parallel multi-domain calls, per-domain manager sessions, and
per-domain locking behavior.

The Codex-first API documentation surface is `ws/api.list`, `ws/api.ask`, and
the async API job tools `ws/api.ask_async`, `ws/api.status`, `ws/api.result`, and
`ws/api.cancel`.
The Claude CLI remains the compatibility fallback for Claude workflows and
older guidance that still invokes `ws-ask-api`.

## Claude Plugin Manifest And Marketplace {#260505-claude-plugin-manifest-marketplace}

The Claude package declares a Claude plugin manifest under
`claude-plugin/.claude-plugin/`. Installer-managed marketplace metadata exposes
the local ws plugin to Claude Code, and validation can be run against plugin
manifests before installation.

The Claude package identity is separate from the Codex plugin manifest and
runtime contract. It exists to preserve Claude Code installation and invocation
behavior while the Codex-first package evolves under `agents-plugin/`.
