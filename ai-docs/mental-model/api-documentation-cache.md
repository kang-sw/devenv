---
domain: api-documentation-cache
description: "API docs domain routing, manager agents, cache ownership, sync asks, and async jobs."
sources:
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/internal/wsprompt/
  - ai-docs/.deps/
related:
  named-agent-runtime: "API docs asks and async jobs use transient router agents and durable per-domain manager agents."
  mcp-runtime: "API docs exposes both synchronous and recoverable async MCP tool surfaces."
  prompt-bundle: "API docs behavior is mostly prompt-owned and prompt stems are hard-coded."
---

# API Documentation Cache

## Entry Points

- `api.list` returns non-hidden domain directories under `ai-docs/.deps/`. {#260505-api-docs-mcp-surface}
- `api.ask` resolves domains, starts a pre-router if needed, fans out manager calls, and aggregates sections. {#260505-api-docs-domain-routing}
- `api.ask_async`, `api.status`, `api.result`, and `api.cancel` wrap the same routing/manager path in recoverable job state. {#260508-api-docs-async-jobs} {#260508-api-documentation-async-mcp-tools}
- `api-doc-manager`, `pre-router`, and `api-doc-cargo-brief` prompts own cache behavior and answer quality.

## Module Contracts

- `prompt` is required; `domain_hint` only bypasses routing when it exactly matches an existing domain directory.
- Pre-router output is domain slugs only, one per line. Any valid slug can cause a manager/cache directory to be created.
- Same root/domain manager calls are serialized process-locally; different domains run concurrently and results are reassembled in original domain order. {#260505-api-docs-synchronous-aggregation}
- Partial failure is intentional: mixed success returns text with `ERROR:` sections; all-domain failure returns a tool error. Sync and async result formatting share one formatter, so aggregation changes must keep both paths aligned.
- Go runtime manages agent/session lifecycle; manager prompts own cache bootstrap, staleness checks, and fetching. {#260505-api-docs-staleness-fetch-bootstrap}
- Async jobs persist state under the worktree ws state before returning, then run in a process-local worker; status/result reconcile stale nonterminal records from worker heartbeat metadata rather than resuming work in a new process.

## Coupling

- `api.ask` depends on named-agent registration, result timeout, ephemeral pre-router cleanup, and per-domain manager reuse. Inactive managers older than the five-minute hot-cache TTL are erased and re-registered; active managers are preserved. {#260505-api-docs-manager-sessions}
- Async cancellation must reach both pre-router and per-domain manager waits through `wsagent.Manager.Cancel`; merely marking the API job cancelled leaves router/manager agents running until their own timeout.
- Prompt stems in `api_docs.go` must match embedded prompt filenames and runtime bundle metadata.
- `api-doc-cargo-brief` is conditional on a binary existing on `PATH`; changing this requires `ConditionalPromptRef` behavior. {#260505-api-docs-conditional-prompts}
- Cache domain names are filesystem directory names; validation must remain strict enough to prevent path traversal.

## Extension Points & Change Recipes

- **Change routing behavior**: edit `pre-router.md` but preserve slug-only output or update `parseAPIRouterDomains`.
- **Change cache policy**: edit `api-doc-manager.md`; Go code does not inspect `meta.yaml` or fetch docs itself.
- **Change async job lifecycle**: update `api_async.go`, MCP dispatch/schema, runtime metadata, and recovery/cancellation tests together; preserve synchronous `api.ask` behavior.
- **Add a conditional brief**: add an embedded infra prompt, wire `ConditionalPromptRef`, and update prompt bundle metadata.

## Common Mistakes

- Hand-editing or committing `ai-docs/.deps/` as ordinary workflow output; use `ws/api.ask` unless modifying the cache mechanism. {#260505-api-docs-worker-guidance}
- Assuming fuzzy `domain_hint` creates or selects a domain directly.
- Treating async jobs as cross-process resumable work; fresh servers recover job records and terminal results, but stale active workers are reconciled to failed or cancelled.
- Removing `## Domain: <domain>` aggregation headers and breaking caller/test boundaries.
- Editing API docs prompts without refreshing `agents-plugin/runtime.json`.

## Technical Debt

- The repository currently has only an empty `ai-docs/.deps/ws-mcp/` domain; cache content is not yet populated.
- Prose recovery only runs when the router produced zero valid slugs, so a partially valid bad response can misroute.
