# Implementation Plan: 260508-feat-api-ask-async-jobs

## Scope

Implement the async API documentation job skeleton in `agents-plugin-tool/internal/mcp/api_async.go` while preserving the synchronous `api.ask` path. The plan targets the skeleton tests in `agents-plugin-tool/internal/mcp/api_async_test.go` and the spec contracts in `ai-docs/spec/api-documentation-cache.md#L66-L81` and `ai-docs/spec/mcp-tools.md#L203-L209`.

## Durable State Shape

- Store job JSON under worktree-scoped ws state from `wsstate.Manager.Ensure(root)`, not under `ai-docs/.deps`; `Layout` already exposes worktree state dirs and metadata in `agents-plugin-tool/internal/wsstate/paths.go#L30-L48` and `Ensure` creates worktree-owned dirs at `agents-plugin-tool/internal/wsstate/paths.go#L148-L164`.
- Add an API-job directory such as `<layout.WorktreeDir>/api-jobs/<api_job_key>/state.json` plus optional `lock` file; keep cache ownership separate from API docs cache files under `ai-docs/.deps`.
- Extend `apiJobRecord` in `agents-plugin-tool/internal/mcp/api_async.go#L61-L75` only as needed for lifecycle bookkeeping: schema version, prompt, domain hint, root, resolved domains, per-domain status/errors, result text, final error text, cancellation flag, timestamps, and worker identity/cancel generation if needed.
- Use a random or timestamp+random slug key validated as a safe path segment. Never derive the key from prompt/domain text.
- Write records with atomic temp-file replace, following existing JSON write patterns in `wsstate.upsertJSON` (`agents-plugin-tool/internal/wsstate/paths.go#L266-L325`) or `wsagent.writeCurrentCall` (`agents-plugin-tool/internal/wsagent/agent.go#L2045-L2063`).

## Async Execution Lifecycle

1. `startAPIJob` (`agents-plugin-tool/internal/mcp/api_async.go#L81-L96`) trims/validates prompt, resolves the canonical tool root already passed from `server.go`, creates a durable queued record, starts a goroutine, and returns `apiJobStartResponse` immediately with non-empty `api_job_key` and `result_ready: false`.
2. The goroutine owns one job runner context and updates durable state through: `queued -> routing -> running -> succeeded|partial_failed|failed|cancelled`.
3. Reuse the current routing and manager semantics rather than calling `askAPI` as a black box if per-domain progress is needed: `resolveAPIDomains` handles exact hints and pre-router output (`agents-plugin-tool/internal/mcp/api_docs.go#L187-L205`), manager calls are fanned out and aggregated in stable domain order (`agents-plugin-tool/internal/mcp/api_docs.go#L124-L185`).
4. During routing, persist resolved domains once known and initialize each domain as `pending`; then mark domains `running`, `succeeded`, `failed`, or `cancelled` as manager calls complete.
5. Preserve aggregation text exactly enough for existing tests: header `api.ask results`, each `## Domain: <domain>` section, `ERROR: <message>` for failed domains, and final error `api.ask failed for all resolved domains` when successes are zero.
6. `statusAPIJob` reads durable JSON by key and maps `apiJobRecord` to `apiJobStatusResponse`; it must work from a fresh `NewServer(root, ...)` without in-memory state.
7. `resultAPIJob` returns non-error final text for `succeeded` and `partial_failed`; returns a tool error with preserved result text plus final error for `failed`; returns a cancellation tool error for `cancel_requested/cancelled`; returns a clear not-ready tool error for active states.

## Cancellation

- Keep a process-local registry of active job cancel funcs keyed by root+job key. `api.cancel` should set `cancel_requested` durably even if the goroutine is not found in the current process.
- When the active cancel func exists, call it so the manager context passed to `AskManager` observes `ctx.Done()`; the cancellation-aware skeleton runtime depends on this (`agents-plugin-tool/internal/mcp/api_async_test.go#L152-L188`).
- Runners must check the durable cancel flag before routing, before starting each domain call, and after manager calls return. Treat `context.Canceled`/requested cancellation as job `cancelled`, mark unfinished domains `cancelled`, set `completed_at`, and make `api.result` a cancellation tool error.
- Best-effort cancellation does not need to kill durable manager agents directly; existing manager calls already receive context through `apiRuntime.AskManager` (`agents-plugin-tool/internal/mcp/api_docs.go#L55-L83`).

## MCP Surface And Metadata

- `server.go` already dispatches and lists `api.ask_async`, `api.status`, `api.result`, and `api.cancel` (`agents-plugin-tool/internal/mcp/server.go#L340-L370`, `agents-plugin-tool/internal/mcp/server.go#L1041-L1088`). Confirm no schema/dispatch gaps remain.
- `LeadToolNames` derives runtime capabilities from `tools()` (`agents-plugin-tool/internal/mcp/server.go#L1554-L1564`), but `agents-plugin/runtime.json` currently lists only `api.list` and `api.ask`; update the `tools` metadata there if this implementation owns runtime metadata.
- Do not add CLI mirrors for these tools unless separately required; `cmd/ws-mcp` runtime command names are CLI-only and currently omit API docs commands.

## Tests To Satisfy

- `TestAPIAsyncMCPToolsListed`: tool schemas include all async tools and `api_job_key` (`agents-plugin-tool/internal/mcp/api_async_test.go#L15-L35`).
- `TestAPIAskAsyncImmediateStartReturnsRecoverableJobKeyAndStatus`: start returns within 100ms, persists prompt/hint, and status is recoverable by a fresh server (`agents-plugin-tool/internal/mcp/api_async_test.go#L37-L89`).
- `TestAPIAsyncPollingResultPreservesPartialFailureAggregation`: one failed domain yields `partial_failed`, exposes resolved domains, returns text not tool error, and reuses routing/manager calls (`agents-plugin-tool/internal/mcp/api_async_test.go#L91-L124`).
- `TestAPIAsyncAllDomainFailureReturnsToolErrorWithMetadata`: all failed domains yield failed status and result tool error with domain sections plus final all-failed metadata (`agents-plugin-tool/internal/mcp/api_async_test.go#L126-L150`).
- `TestAPIAsyncCancelStopsActiveWorkBestEffort`: cancellation marks state, cancels active context, settles `cancelled`, and makes result a cancellation error (`agents-plugin-tool/internal/mcp/api_async_test.go#L152-L188`).
- `TestAPIAsyncPreservesSynchronousAPIAskCompatibility`: `api.ask` behavior remains unchanged while async tools coexist (`agents-plugin-tool/internal/mcp/api_async_test.go#L191-L218`).

## Verification

```sh
cd agents-plugin-tool && go test ./internal/mcp
cd agents-plugin-tool && go test ./internal/wsagent ./internal/wsstate
cd agents-plugin-tool && go test ./cmd/ws-mcp
cd agents-plugin-tool && go test ./...
git diff --check
```

## Risks / Notes

- In-process goroutines are enough for the skeleton's fresh-server status recovery, but they do not survive MCP process exit. The durable record must make the timeout recovery handle truthful even if result collection resumes later; process-exit resumability beyond status/result of completed records may require a follow-up worker model.
- Avoid introducing a second API aggregation implementation that drifts from `askAPI`; if helper extraction is small, share the per-domain execution/aggregation logic between sync and async paths.
