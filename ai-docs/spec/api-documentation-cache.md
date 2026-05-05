---
title: API Documentation Cache
summary: Host-neutral API documentation lookup through cached domain docs, routing, manager sessions, and worker guidance.
---

# API Documentation Cache

The API documentation cache lets workflow agents answer third-party API
questions through project-local cached documentation instead of ad hoc direct
web lookup. The Codex-first surface is the `ws/api.*` MCP tool family, with
domain routing and cache maintenance delegated to API-doc manager agents.

## API Docs MCP Surface {#260505-api-docs-mcp-surface}

`ws/api.list` returns sorted domain names from the project cache under
`ai-docs/.deps/`. Hidden entries and non-directories are excluded.

`ws/api.ask` answers an API documentation question. The caller provides a
prompt and may provide a `domain_hint`. The tool owns domain resolution,
manager dispatch, and aggregation, then returns a synchronous answer to the
caller.

## API Docs Cache Layout {#260505-api-docs-cache-layout}

API documentation cache entries live under:

```text
ai-docs/.deps/<domain>/
```

The runtime creates domain directories as needed and lists existing domain
directories. The manager prompt owns the detailed cache contents. Expected
manager-owned files include domain overview material, version metadata,
layered documentation summaries, subdomain Markdown files, and helper scripts
for version detection, fetching, and staleness checks.

Workflow guidance treats `.deps` contents as a managed cache. Ordinary workers
ask through `ws/api.ask` rather than reading or editing cached files directly.

## API Docs Domain Routing {#260505-api-docs-domain-routing}

`ws/api.ask` resolves one or more cache domains before dispatching manager
queries. An exact `domain_hint` matching an existing cache domain bypasses the
pre-router. Unknown, fuzzy, or absent hints route through a transient
`pre-router` agent.

The pre-router receives the caller prompt, optional domain hint, and existing
domain list. Its output is parsed as one domain slug per line. If the router
returns malformed prose, the runtime can recover explicit mentions of existing
domains from the router output; otherwise malformed routing is reported as an
error.

## API Docs Synchronous Aggregation {#260505-api-docs-synchronous-aggregation}

`ws/api.ask` is synchronous from the caller's perspective. Internally, it fans
out resolved domains in parallel, runs one manager query per domain, and
combines successful answers into sections headed `## Domain: <domain>`.

Same-domain work is serialized within the MCP process. Different domains can
run concurrently. Partial success is preserved: if at least one domain returns
an answer, failed domains are reported alongside successful sections. The tool
returns an error only when all resolved domains fail.

## API Docs Manager Sessions {#260505-api-docs-manager-sessions}

Each cache domain uses a named manager session:

```text
api-doc-<domain>
```

Manager sessions use the Codex backend, the `core` workload tier, and the
embedded `api-doc-manager` prompt. Public delegate orientation is suppressed
because the manager prompt is a complete domain-specific system prompt.

Inactive manager sessions are reused while warm. If an idle inactive manager is
older than the API-doc hot-cache TTL, the runtime erases and re-registers it
before answering the next request. Active managers are preserved.

## API Docs Staleness, Fetch, And Bootstrap {#260505-api-docs-staleness-fetch-bootstrap}

Staleness, fetching, and cache bootstrapping are prompt-owned manager behavior.
The Go runtime does not inspect cache metadata or run fetch scripts itself.

The `api-doc-manager` prompt instructs managers to bootstrap missing domain
cache files, run staleness checks, fetch official documentation when the cache
is absent or stale, update cached summaries, and answer using cached paths or
official source URLs as citations.

## API Docs Conditional Prompts {#260505-api-docs-conditional-prompts}

API-doc manager registration can include conditional prompt material based on
available local tooling.

When `cargo-brief` is available on `PATH`, the runtime appends the embedded
`api-doc-cargo-brief` prompt to API-doc managers. That prompt guides Rust API
lookups toward the local cargo-brief workflow before falling back to ordinary
cache behavior.

## API Docs Worker Guidance {#260505-api-docs-worker-guidance}

Workflow and delegate guidance direct agents to use `ws/api.ask` for external
API documentation questions. `ws/api.list` is used when the caller needs to
inspect available cache domains or choose a precise `domain_hint`.

Workers should not browse or fetch third-party API docs directly when
`ws/api.ask` can answer the question. They should pass a domain hint only when
the intended cache domain is known; otherwise the pre-router owns domain
selection.
