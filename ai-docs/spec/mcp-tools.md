---
title: MCP Tools
summary: Host-neutral ws MCP tool contracts for project context, workflow state, Git, documentation, and named-agent orchestration.
---

# MCP Tools

The ws MCP server exposes workflow capabilities through named MCP tools rather
than host-specific shell commands or repository-local paths. Tool outputs are
plain text or JSON-in-text MCP content that callers can use from Codex, Claude,
or another MCP-capable host.

## MCP Server Protocol Surface {#260505-mcp-server-protocol-surface}

The `ws-mcp serve --stdio` process implements a stdio JSON-RPC MCP server. It
responds to `initialize`, `tools/list`, and `tools/call`, advertises MCP
protocol version `2025-03-26`, and declares tool capability.

Unknown methods and profile-rejected tools return JSON-RPC errors. Tool-level
runtime failures return MCP text content with `isError: true`, preserving a
normal MCP response envelope while still making the failure visible to callers.

The MCP server detects the host harness from observable MCP payloads before
relying on environment variables. It inspects `initialize.params` and request
metadata for high-confidence Codex or Claude markers, treats
`tools/call.params._meta.x-codex-turn-metadata` as a Codex signal, and records
conflicting signals in diagnostics instead of silently changing the session
harness. The detected harness is exposed through session inspection output.
{#260508-mcp-payload-harness-detection}

## Runtime And Debug Metadata Tools {#260505-runtime-debug-metadata-tools}

`runtime.info` returns runtime compatibility metadata, including the runtime
version, source commit, and prompt bundle metadata. Launchers and workflow
checks use this output to detect stale or incompatible runtime binaries.

`runtime.debug_events` returns recent in-process MCP debug events as JSONL. The
tool is bounded by an optional limit parameter and is intended for diagnosing MCP
server behavior without reading process-local files directly.

## MCP Session Root Defaults {#260505-mcp-session-default-root}

Root-aware MCP tools resolve omitted `root` arguments through the current MCP
server session before falling back to startup state. The priority is:

1. Explicit tool argument `root`.
2. Volatile session default root.
3. Host workspace metadata when the host provides exactly one workspace.
4. Explicit non-dot server startup root.
5. `WS_MCP_PROJECT_ROOT`.
6. Server startup root.

`session.set_default_root` validates that its `root` is inside a Git worktree,
stores the canonical worktree root in the current server process, and returns
that effective root. The value is volatile and does not write user config, ws
cache config, or repository files. `session.get_default_root` reports whether a
session default is set plus fallback state such as the server root and
`WS_MCP_PROJECT_ROOT`. It also reports the detected session harness when one has
been observed.

When host metadata names multiple workspaces and no higher-priority root exists,
root-aware tools refuse to guess and return an actionable error asking the caller
to pass `root` explicitly or call `session.set_default_root`.

An explicit non-dot server startup root is treated as authoritative before the
launcher-provided project-root environment fallback. If that explicit startup
root is invalid, root-aware tools fail closed instead of silently falling back to
`WS_MCP_PROJECT_ROOT`.

## Config Tools {#260505-config-tools}

`config.show` returns the resolved ws user-local configuration path and current
configuration without modifying it.

`config.agents_tier` is the compatibility surface for updating the default
backend/model mapping for a model alias. Callers provide `tier` as the alias
name and may provide a backend, a concrete model, or both. When backend is
omitted, ws infers it from the model family where possible.

Configuration exposes harness-aware model alias mappings. `light`, `core`, and
`deep` map to backend/model defaults per harness, existing tier-shaped config is
migrated or wrapped for compatibility, and new documentation speaks in terms of
model aliases rather than workload tiers. {#260508-model-alias-config-tools}

## Project Context And Convention Tools {#260505-project-context-convention-tools}

`project_tree` renders the project document map, spec inventory, and active
ticket queue for the current repository.

`infra.read` reads repository-local ws infra documents by bare stem or filename.
`convention.read` reads bundled convention documents shipped with the runtime,
such as ticket, spec, or mental-model conventions. Shared workflow skills use
these tools instead of hard-coded repository-local convention paths.

## Spec Discovery Tools {#260505-spec-discovery-tools}

`spec_stem.generate` returns a collision-free spec anchor stem for a descriptive
slug.

`spec_index.verify` checks the spec corpus for anchor-index health problems such
as duplicate stems.

`specs.list`, `specs.find`, and `specs.status` provide read-only spec discovery.
They expose spec file metadata, anchors, ticket references, marker context, query
matches, and exact-stem status without requiring callers to scan the spec tree
manually.

## Ticket Discovery Tools {#260505-ticket-discovery-tools}

`tickets.list` returns ticket paths and structured status metadata across ticket
status directories. Active discovery includes `ready/`, `todo/`, and `idea/` by
default; archived `.done/` and `.dropped/` tickets are omitted unless explicitly
requested. `ready/` identifies spec-gated implementation work, while `todo/`
remains accepted backlog.

`tickets.find` locates tickets by text query, exact ticket stem, mentioned
ticket stem, and optional status filters. `tickets.status` returns structured
metadata for a single ticket stem and can optionally include archived done or
dropped tickets.

## Mental-Model Discovery Tools {#260505-mental-model-discovery-tools}

`mental_models.list` returns available mental-model documents with domain,
description, and source metadata.

`mental_models.find` locates mental-model paths by text query, domain, or spec
stem reference. `mental_models.status` returns path-first metadata for documents
selected by domain or path.

## Reference Trace Tool {#260505-reference-trace-tool}

`references.trace` returns the reference graph reachable from exactly one ticket
stem or spec stem. The result connects tickets, specs, and mental-model
documents so callers can inspect traceability without manually searching each
document system.

## Git Workflow Tools {#260505-git-workflow-tools}

`git.status` returns the current branch and worktree status.

`git.diff` returns read-only diff output. It defaults to stat mode for context
control and supports explicit `mode: "full"` for patch content or
`mode: "name_only"` for path listings. Range-less diffs include untracked files
where applicable.

`git.log` returns a bounded commit log with an optional body flag. `git.merge_base`
returns the merge base for two revisions.

`git.commit` creates a workflow-aware commit from explicit paths and structured
message fields. It stages only the requested paths and formats commit messages
with required AI Context and optional ticket, spec, or mental-model update
sections.

## Workflow State And Delegation Tools {#260505-workflow-state-delegation-tools}

`subquery` starts an asynchronous scoped codebase or documentation query and
returns a generated subquery key immediately. Callers collect the result through
the named-agent result/status/tail/cancel surfaces.

`path.generate` allocates worktree-scoped writable artifact paths, such as review
files, so workflow agents can exchange file paths without inventing cache
locations.

## Named-Agent MCP Tools {#260505-named-agent-mcp-tools}

The `agents.*` tool family exposes durable named-agent orchestration.

`agents.register` creates or updates an agent record with backend, model alias
or compatibility tier field, resolved model, prompt references, or materialized
system prompt text. `agents.call` starts an asynchronous call and returns
immediately.

`agents.register` prefers `model` as the public model-selection field.
`model: "light"`, `model: "core"`, and `model: "deep"` select portable
aliases; concrete provider model names select a one-off backend model. The
`tier` field remains a deprecated compatibility input.
{#260508-agents-register-model-alias-field}

`agents.wait` waits for one or more agents to become ready and returns readiness
metadata, not final output. `agents.result` is the result-consumption surface and
may optionally wait for completion; successful ephemeral agents are erased after
their result is consumed.

`agents.status`, `agents.tail`, and `agents.cancel` inspect or control current
agent work. Normal `agents.tail` is context-bounded. Raw diagnostic inspection is
available through `agents.debug.tail`, `agents.debug.stdout`,
`agents.debug.stderr`, `agents.debug.runtime_log`, and `agents.debug.events`.

`agents.interrupt` queues a redirect message for a running agent. `agents.print`
remains a deprecated compatibility reader, and `agents.erase` removes an agent
directory for the current worktree.

## API Documentation MCP Tools {#260505-api-documentation-mcp-tools}

`api.list` returns sorted API documentation cache domain names under
`ai-docs/.deps`.

`api.ask` asks cached or fetchable third-party API documentation through
per-domain manager sessions. Callers provide a prompt and may provide a domain
hint. The tool owns the API-doc routing and aggregation behavior internally and
returns a synchronous answer to the caller.

## Tool Profile Gating {#260505-tool-profile-gating}

The MCP server defaults to the `lead` tool surface. It does not derive authority
from worktree-local locks or startup-root ownership, because plugin-managed hosts
can start the server from cache directories and can fail to propagate
environment variables consistently.

`WS_MCP_TOOL_PROFILE` is an optional profile filter, not an authority boundary.
When the host successfully propagates it, `delegate` and `leaf` receive narrower
tool sets for dogfood containment and tests. Delegate access to generated
subquery agents is scoped to subquery result, status, tail, cancel, and
print-style operations. Leaf also hides recursive orchestration and selected
mutation tools.

When profile environment propagation fails, delegated agents may see the full
lead MCP surface. Workflow containment therefore depends on prompt rules such as
delegate orientation and lead-owned orchestration instructions, not on MCP
tool-surface filtering. `WS_MCP_ALLOWED_TOOLS` can further narrow the visible
surface for tests or debugging, but it cannot expand access beyond the selected
profile.

## CLI Mirror Coverage {#260505-cli-mirror-coverage}

The `ws-mcp` binary mirrors selected MCP behavior as CLI commands for smoke
tests, compatibility probes, and fallback usage.

CLI mirrors exist for runtime info, config, path generation, subquery, named
agents, Git, tickets, specs, selected mental-model discovery, and reference
tracing. Not every MCP tool has a CLI mirror; the MCP surface is the canonical
host-neutral interface, and CLI coverage is limited to the surfaces needed for
runtime checks and workflow fallback use.
