---
domain: api-documentation-cache
description: "Local API documentation cache discovery after retired agent-backed ask tools."
sources:
  - agents-plugin-tool/internal/mcp/api_docs.go
  - agents-plugin-tool/internal/mcp/server.go
  - ai-docs/.deps/
related:
  mcp-runtime: "api.list remains a deterministic MCP tool; ask/async tools are removed."
  prompt-bundle: "API documentation prompt stems were removed from the rsrc tree with the agent-backed ask path."
---

# API Documentation Cache

## Entry Points

- `api.list` returns non-hidden domain directories under `ai-docs/.deps/`. {#260505-api-docs-mcp-surface}
- `api.ask`, `api.ask_async`, `api.status`, `api.result`, and `api.cancel` are retired and must stay absent from MCP schemas, dispatch, runtime metadata, and shipped guidance. {#260508-api-documentation-async-mcp-tools}
- `api-doc-manager`, `pre-router`, and `api-doc-cargo-brief` rsrc playbooks were deleted with the manager-session path. {#260505-api-docs-conditional-prompts}

## Module Contracts

- `api.list` is deterministic read-only discovery. It lists directory names only,
  excludes hidden entries and non-directories, sorts the result, and treats a
  missing `ai-docs/.deps/` directory as an empty list.
- The runtime no longer creates cache directories, routes API documentation
  prompts, starts API-doc agents, fetches upstream documentation, or performs
  staleness checks. {#260505-api-docs-domain-routing}
- Workflow guidance sends actual third-party API documentation questions to
  scoped native exploration or direct official documentation lookup with cited
  evidence and version/staleness caveats. {#260505-api-docs-worker-guidance}

## Coupling

- Removing or reintroducing API documentation tools requires reviewing
  `server.go` schemas/dispatch, `runtime.json`, runtime capability tests, rsrc
  guidance, specs, and this mental model together.
- `api.list` depends only on filesystem reads under the caller's resolved
  worktree root; it must not import or depend on `wsagent`.
- The rsrc manifest and wsflow mirror must stay free of retired API-doc prompt
  stems; leaving stale prompt files makes `playbook.print` able to load a dead
  agent-backed path.

## Extension Points & Change Recipes

- **Change cache-domain listing**: edit `apiListDomains` and the retained
  `api.list` MCP tests. Preserve missing-directory-as-empty behavior unless a
  spec update changes it.
- **Design future API documentation tooling**: create a new ticket/spec under
  the future pure-tooling API namespace. Do not reintroduce model delegation,
  pre-router agents, per-domain manager agents, or async API job orchestration
  as part of routine cache-listing work.
- **Update shipped API documentation guidance**: edit
  `lead-workflow-manual`, `delegate-orientation`, the bootstrap template if
  needed, regenerate `manifest.json`, and regenerate the wsflow rsrc mirror.

## Common Mistakes

- Treating `ai-docs/.deps/` as durable project memory; cache data is local and
  Git-ignored.
- Reintroducing `ws/api.ask` as a compatibility shim; the removal deliberately
  makes stale callers fail loudly.
- Leaving references to retired API-doc prompt stems in specs, mental models, or
  shipped guidance after deleting the files.

## Technical Debt

- The future API namespace is intentionally out of this removal scope. It may
  become a pure documentation/memory tooling surface later, but it should not
  restore MCP-owned model routing.
