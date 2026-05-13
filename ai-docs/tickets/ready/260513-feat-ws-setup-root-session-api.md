---
title: ws.setup root session API
related:
  260505-bug-plugin-managed-default-root-discovery: observed plugin-managed root discovery friction
spec:
  - 260505-mcp-session-default-root
  - 260505-named-agent-mcp-tools
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# ws.setup root session API

## Background

Root-aware MCP calls currently expose optional `root` parameters and a
`session.set_default_root` / `session.get_default_root` tool pair. The behavior
works, but the public workflow surface encourages callers to repeat `root`
arguments and uses a session-specific name for what should become the general
setup entrypoint.

The desired direction is a single public setup surface such as
`ws.setup(root?, default_model?, ...)`. Setup should update only the fields the
caller provides. For the root field, it should store the same volatile
per-server root currently used by the resolver.

## Decisions

- `ws.setup` becomes the canonical public MCP setup entrypoint.
- `root` setup is volatile session state, not repository config or user-local
  persistent config.
- `session.set_default_root` should not remain the canonical public surface; the
  migration should remove or hide it rather than teach two equivalent setup
  paths.
- Root resolution diagnostics should point callers to `ws.setup(root = current
  directory)` when no root can be resolved.
- `agents.*` public MCP guidance should stop exposing `root` as the normal
  caller path; root setup should happen once per MCP server session.

## Constraints

- Keep the existing resolver safety properties: do not guess between multiple
  host workspaces, fail closed on invalid authoritative roots, and preserve
  explicit root behavior where needed for compatibility or adapter surfaces.
- Update runtime metadata, launcher compatibility expectations, tests, specs,
  mental models, and reference docs together because this is a public MCP tool
  contract change.
- Decide during implementation whether CLI `--root` remains as an adapter-only
  convenience separate from the MCP public contract.

## Phases

### Phase 1: Replace public root setup with ws.setup

Add the `ws.setup` MCP surface, route its `root` field through the existing
session-root state, update root-missing diagnostics and tool guidance, and
migrate or remove the old `session.set_default_root` / `session.get_default_root`
public contract.

Acceptance criteria:

- A caller can initialize the MCP session root through `ws.setup(root: <path>)`
  and then call root-aware tools without passing `root`.
- Root resolution failures consistently instruct callers to run `ws.setup`.
- Runtime metadata and launcher compatibility data match the exposed MCP tool
  surface.
- Tests cover setup root storage, omitted-root follow-up calls, multi-workspace
  ambiguity, invalid root failure, and compatibility expectations.
- Specs, mental models, reference docs, and workflow skill examples no longer
  present `session.set_default_root` as the canonical setup path.
