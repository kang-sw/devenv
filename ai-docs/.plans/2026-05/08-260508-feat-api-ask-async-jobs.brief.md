# Brief: 260508-feat-api-ask-async-jobs

## Intent

Add recoverable asynchronous API documentation jobs so long-running cache
bootstrap or multi-domain lookups can outlive the initial MCP tool call while
leaving synchronous `ws/api.ask` behavior unchanged.

## Approach

- Implement the skeleton contracts in `agents-plugin-tool/internal/mcp/api_async.go`.
- Persist job records under worktree-scoped ws state so `api_job_key` survives a fresh MCP server instance.
- Start work asynchronously and return a job key immediately.
- Reuse current API-doc domain routing, manager calls, and aggregation semantics instead of duplicating manager behavior.
- Wire `api.status`, `api.result`, and `api.cancel` through the durable job record.

## Constraints

- `api.ask` remains the synchronous quick path and must keep current output/error behavior.
- Partial domain success returns answer text; all-domain failure makes `api.result` a tool error with preserved metadata.
- Cancellation is best effort and must surface cancellation state through status/result.
- Root scoping must follow existing MCP root resolution and ws state path rules.
- Public tool additions must remain visible through `tools/list` and runtime capability metadata where applicable.

## Out of scope

- Do not replace `ws/api.ask` with async behavior.
- Do not add CLI mirrors unless the existing runtime capability contract requires metadata updates for exposed commands.
- Do not change API-doc manager prompt semantics or cache file ownership.

## Details

The public MCP tools are:

- `api.ask_async(prompt, domain_hint?, root?) -> JSON apiJobStartResponse`
- `api.status(api_job_key, root?) -> JSON apiJobStatusResponse`
- `api.result(api_job_key, root?) -> final text or tool error`
- `api.cancel(api_job_key, root?) -> JSON apiJobStatusResponse`

Durable state should record prompt, domain hint, resolved domains, per-domain
progress, final text, error text, timestamps, and cancellation state. A fresh
`NewServer(root, ...)` must recover job state by key.

Acceptance tests are the skeleton tests in
`agents-plugin-tool/internal/mcp/api_async_test.go`. They should pass by the
end of implementation.

## References

- [Must] `ai-docs/tickets/ready/260508-feat-api-ask-async-jobs.md` - target ticket and skeleton reference.
- [Must] `ai-docs/spec/api-documentation-cache.md` - async job and existing routing/aggregation behavior.
- [Must] `ai-docs/spec/mcp-tools.md` - async MCP tool surface contract.
- [Must] `ai-docs/mental-model/api-documentation-cache.md` - API docs runtime coupling and common mistakes.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP tool registration and runtime metadata coupling.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - durable async state/result/cancel patterns.
- [Must] `agents-plugin-tool/internal/mcp/api_async.go` - skeleton stubs.
- [Must] `agents-plugin-tool/internal/mcp/api_async_test.go` - skeleton acceptance tests.
- [Must] `agents-plugin-tool/internal/mcp/server.go` - MCP dispatch and schemas.
- [Must] `agents-plugin-tool/internal/mcp/api_docs.go` - synchronous API docs routing, manager dispatch, and aggregation.
- [Must] `agents-plugin-tool/internal/mcp/server_test.go` - MCP test helpers and fake API runtime.
- [Must] `agents-plugin-tool/internal/wsagent/agent.go` - durable async lifecycle pattern.
- [Must] `agents-plugin-tool/internal/wsagent/subquery.go` - generated-key async wrapper pattern.
- [Must] `agents-plugin-tool/internal/wsstate/paths.go` - worktree-scoped state path rules.
- [Maybe] `agents-plugin/runtime.json` - update only if runtime metadata must list the new MCP tools.
- [Maybe] `agents-plugin-tool/cmd/ws-mcp/main.go` - consult only if CLI mirrors are added.

## Verification

Run:

```sh
cd agents-plugin-tool && go test ./internal/mcp
cd agents-plugin-tool && go test ./internal/wsagent ./internal/wsstate
cd agents-plugin-tool && go test ./cmd/ws-mcp
cd agents-plugin-tool && go test ./...
git diff --check
```
