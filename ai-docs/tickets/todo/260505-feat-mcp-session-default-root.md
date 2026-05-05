---
title: MCP session default root
related:
  260505-bug-plugin-managed-default-root-discovery: recovery path for plugin-managed MCP calls without caller root
spec:
  - 260505-mcp-session-default-root
related-mental-model:
  - mcp-runtime
---

# MCP session default root

## Background

Plugin-managed MCP servers can run from the installed plugin cache instead of the
caller repository. On native Windows Codex installs, omitted `root` arguments can
therefore resolve to the plugin cache or `"."` instead of the active project.

Codex CLI 0.128.0 did not declare MCP `roots` capability in `initialize` during
local sniffing. It did include Codex-specific workspace metadata on `tools/call`
under `_meta.x-codex-turn-metadata.workspaces`, but that is a host-specific
fallback rather than a portable contract.

The workflow needs an explicit, easy-to-discover recovery tool so an agent can
set the repository root once for the current MCP server process and then omit
`root` on later calls.

## Decisions

- Add a volatile session-level default root, not a persisted user or project
  setting.
- Prefer `session.set_default_root` over `set_cwd_root` because the tool does
  not change the process working directory; it only changes the default
  repository root used by ws MCP tools when `root` is omitted.
- Keep the value in MCP server memory only. It disappears when the stdio server
  process exits.
- Treat Codex `_meta.x-codex-turn-metadata.workspaces` as best-effort fallback,
  not as the canonical root contract.
- Keep explicit tool arguments highest priority.

## Phases

### Phase 1: Volatile default-root tools

Add MCP tools:

- `session.set_default_root(root: string)`
- `session.get_default_root()`

`session.set_default_root` should validate that `root` resolves to a Git
worktree root, store the canonical path in the current `Server` instance, and
return the effective root. It must not write user config, ws cache config, or
repo files.

`session.get_default_root` should report the current volatile value and enough
fallback state to explain whether no default has been set.

Suggested root resolution priority:

1. Explicit tool argument `root`.
2. Volatile session default root.
3. `WS_MCP_PROJECT_ROOT`.
4. Host metadata fallback, currently Codex
   `_meta.x-codex-turn-metadata.workspaces` when exactly one workspace is
   present.
5. Server startup root / process cwd fallback.
6. Actionable error asking the caller to pass `root` or call
   `session.set_default_root`.

### Phase 2: Host metadata fallback and diagnostics

Capture Codex workspace metadata from `tools/call.params._meta` and use it only
when it is unambiguous. If multiple workspaces are present, do not guess; return
an actionable diagnostic that names the ambiguity and asks for explicit `root` or
`session.set_default_root`.

Preserve room for a future Claude adapter by keeping host metadata parsing behind
a small resolver boundary rather than scattering Codex-specific `_meta` lookups
through every tool branch.

### Phase 3: Documentation and smoke coverage

Document the default-root resolver chain in the MCP spec/reference docs and add
tests that cover:

- explicit `root` overriding all fallbacks;
- `session.set_default_root` affecting later root-omitted calls in the same MCP
  server process;
- the value not persisting across new server instances;
- Codex single-workspace metadata fallback;
- Codex multi-workspace metadata refusing to guess.
