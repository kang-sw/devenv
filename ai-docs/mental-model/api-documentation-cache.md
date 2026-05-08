---
domain: api-documentation-cache
description: "ws/api.ask domain routing, manager agents, cache ownership, and API docs prompts."
sources:
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/internal/wsprompt/
  - ai-docs/.deps/
related:
  named-agent-runtime: "api.ask uses transient router agents and durable per-domain manager agents."
  prompt-bundle: "API docs behavior is mostly prompt-owned and prompt stems are hard-coded."
---

# API Documentation Cache

## Entry Points

- `api.list` returns non-hidden domain directories under `ai-docs/.deps/`. {#260505-api-docs-mcp-surface}
- `api.ask` resolves domains, starts a pre-router if needed, fans out manager calls, and aggregates sections. {#260505-api-docs-domain-routing}
- `api-doc-manager`, `pre-router`, and `api-doc-cargo-brief` prompts own cache behavior and answer quality.

## Module Contracts

- `prompt` is required; `domain_hint` only bypasses routing when it exactly matches an existing domain directory.
- Pre-router output is domain slugs only, one per line. Any valid slug can cause a manager/cache directory to be created.
- Same root/domain manager calls are serialized process-locally; different domains run concurrently and results are reassembled in original domain order. {#260505-api-docs-synchronous-aggregation}
- Partial failure is intentional: mixed success returns text with `ERROR:` sections; all-domain failure returns a tool error.
- Go runtime manages agent/session lifecycle; manager prompts own cache bootstrap, staleness checks, and fetching. {#260505-api-docs-staleness-fetch-bootstrap}

## Coupling

- `api.ask` depends on named-agent registration, result timeout, ephemeral pre-router cleanup, and per-domain manager reuse. Inactive managers older than the five-minute hot-cache TTL are erased and re-registered; active managers are preserved. {#260505-api-docs-manager-sessions}
- Prompt stems in `api_docs.go` must match embedded prompt filenames and runtime bundle metadata.
- `api-doc-cargo-brief` is conditional on a binary existing on `PATH`; changing this requires `ConditionalPromptRef` behavior. {#260505-api-docs-conditional-prompts}
- Cache domain names are filesystem directory names; validation must remain strict enough to prevent path traversal.

## Extension Points & Change Recipes

- **Change routing behavior**: edit `pre-router.md` but preserve slug-only output or update `parseAPIRouterDomains`.
- **Change cache policy**: edit `api-doc-manager.md`; Go code does not inspect `meta.yaml` or fetch docs itself.
- **Add a conditional brief**: add an embedded infra prompt, wire `ConditionalPromptRef`, and update prompt bundle metadata.

## Common Mistakes

- Hand-editing or committing `ai-docs/.deps/` as ordinary workflow output; use `ws/api.ask` unless modifying the cache mechanism. {#260505-api-docs-worker-guidance}
- Assuming fuzzy `domain_hint` creates or selects a domain directly.
- Removing `## Domain: <domain>` aggregation headers and breaking caller/test boundaries.
- Editing API docs prompts without refreshing `agents-plugin/runtime.json`.

## Technical Debt

- The repository currently has only an empty `ai-docs/.deps/ws-mcp/` domain; cache content is not yet populated.
- Prose recovery only runs when the router produced zero valid slugs, so a partially valid bad response can misroute.
