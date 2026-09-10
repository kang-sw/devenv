---
title: API Documentation Cache
summary: Local API documentation cache discovery after removal of agent-backed ask tools.
---

# API Documentation Cache

The API documentation cache is local project data under `ai-docs/.deps/`.
The ws runtime currently exposes only deterministic cache-domain discovery for
this area. It does not own third-party API question answering, domain routing,
documentation fetching, staleness analysis, or manager-agent orchestration.

## API Docs MCP Surface {#260505-api-docs-mcp-surface}

`ws/api.list` returns sorted domain names from the project cache under
`ai-docs/.deps/`. Hidden entries and non-directories are excluded. The default
response is one domain per line; callers can request structured JSON through the
standard `format: "json"` compatibility path.

The retired agent-backed API documentation tools are not part of the ws MCP
surface: `ws/api.ask`, `ws/api.ask_async`, `ws/api.status`, `ws/api.result`, and
`ws/api.cancel` are unknown tools in full ws and are absent from runtime
capability metadata.

## API Docs Cache Layout {#260505-api-docs-cache-layout}

API documentation cache entries live under:

```text
ai-docs/.deps/<domain>/
```

The runtime lists existing domain directories but does not inspect domain
metadata, mutate cache contents, fetch upstream documentation, or decide whether
cache data is stale. Bootstrap guidance treats `ai-docs/.deps/` as Git-ignored
local cache data, not durable project memory.

## API Docs Domain Routing {#260505-api-docs-domain-routing}

Runtime-owned API documentation domain routing is retired. Agents that need
third-party API documentation should use scoped host-native exploration or
direct official documentation lookup, with exact package/version context when
known and cited evidence in the result.

There is no MCP-owned pre-router, fuzzy domain selector, or automatic manager
session creation path after the removal of `ws/api.ask`.

## API Docs Synchronous Aggregation {#260505-api-docs-synchronous-aggregation}

Runtime-owned synchronous answer aggregation is retired. The ws runtime no
longer fans out API documentation prompts across cache domains or combines
answers under `## Domain: <domain>` headings.

## API Docs Async Jobs {#260508-api-docs-async-jobs}

The recoverable API documentation job surface is retired. `ws/api.ask_async`,
`ws/api.status`, `ws/api.result`, and `ws/api.cancel` are not advertised and do
not create or recover API documentation work.

## API Docs Manager Sessions {#260505-api-docs-manager-sessions}

API documentation manager sessions are retired. The runtime no longer registers
`api-doc-<domain>` agents, a pre-router agent, or domain-specific API-doc
workers.

## API Docs Staleness, Fetch, And Bootstrap {#260505-api-docs-staleness-fetch-bootstrap}

The Go runtime does not own API documentation staleness checks, upstream
fetching, or cache bootstrap. Future API documentation tooling may add
deterministic helpers for these concerns, but it must not reintroduce
MCP-owned model delegation or agent-backed routing.

## API Docs Conditional Prompts {#260505-api-docs-conditional-prompts}

Conditional API-doc manager prompts are retired with the manager-session path.
The shipped rsrc tree no longer contains `api-doc-manager`, `pre-router`, or
`api-doc-cargo-brief` playbooks.

## API Docs Worker Guidance {#260505-api-docs-worker-guidance}

Workflow and delegate guidance must not route ordinary external API
documentation questions through ws API ask tools. Use `ws/api.list` only when
local cache domain discovery matters. For actual dependency/API documentation
questions, use scoped native exploration or official documentation lookup and
return cited evidence plus version or staleness caveats.
