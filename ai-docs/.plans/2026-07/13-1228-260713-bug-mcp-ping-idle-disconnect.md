# Plan: 260713-bug-mcp-ping-idle-disconnect — Phase 1: Add ping handling and validate idle liveness

## Relevant Ticket Contract
- `ping` is a base-protocol JSON-RPC request: preserve the request id and return `{"jsonrpc":"2.0","id":<original>,"result":{}}` promptly.
- Keep the existing stdio concurrency model and serialized response writes; do not emit unsolicited blank lines or other keepalive traffic.
- Do not add `ping` to `tools/list`, runtime inventories, schemas, or ws/wsflow product-mode filtering.
- Verify protocol conformance with focused stdio coverage and the existing Go, plugin, and wsflow suites.
- Native-Windows acceptance requires at least 20 minutes without MCP tool activity, survival of the original Python and `ws-mcp` PIDs, and a successful later tool call without manual restart. Persisted session state alone is not survival evidence.
- Update the local MCP protocol-surface spec during closeout. If ping compliance does not stop termination, close only the conformance portion and capture a separate evidence-backed lifecycle follow-up.

## Out of Scope
- Streamable HTTP, reconnect behavior, daemon lifecycle, launcher timers, and speculative host-lifecycle fixes.
- Any MCP tool/schema/inventory addition or ws/wsflow product-mode behavior change.

## Codebase Findings
- `agents-plugin-tool/internal/mcp/server.go#L79-L103` — Requests and responses already retain IDs as `json.RawMessage`; returning an empty `map[string]any{}` supplies the required empty-object result without converting string or numeric IDs.
- `agents-plugin-tool/internal/mcp/server.go#L128-L180` — `ServeStdio` dispatches requests concurrently and protects the shared encoder with `writeMu`; ping should reuse this path without adding a separate writer or idle goroutine.
- `agents-plugin-tool/internal/mcp/server.go#L209-L229` — Base-protocol dispatch is centralized in `handle`; the current default produces method-not-found, so a single `ping` case is the narrow integration point.
- `agents-plugin-tool/internal/mcp/server_test.go#L828-L853` — Existing response helpers index both string and numeric raw IDs, which can support focused ID-preservation assertions without new test infrastructure.
- `ai-docs/spec/mcp-tools.md#L18-L27` — The local protocol-surface description currently lists only `initialize`, `tools/list`, and `tools/call`; it must add `ping` after implementation.
- `ai-docs/ship/ws.md#L43-L51` — Repository pre-flight names the applicable suite commands: plugin unittest, wsflow unittest, Go tests, and the MCP smoke script.

## Implementation Plan
1. Add a `ping` branch to `Server.handle` in `agents-plugin-tool/internal/mcp/server.go#L209-L229`, returning the existing response envelope with `req.ID` and a non-nil empty object; leave notification handling, tool registration, filtering, and write coordination unchanged.
2. Add a focused stdio protocol test in `agents-plugin-tool/internal/mcp/server_test.go#L828-L950` that sends ping requests with representative numeric and string IDs, decodes each line, and asserts an exact JSON-RPC success envelope with an empty object, no error, and the original ID. Include or retain a `tools/list` assertion that `ping` is not advertised as a tool.
3. Update `ai-docs/spec/mcp-tools.md#L18-L27` so the MCP Server Protocol Surface includes protocol-compliant `ping` handling without presenting it as a tool or changing the discoverable inventory contract.
4. Record implementation verification and the Windows A/B evidence under the selected phase in `ai-docs/tickets/ready/260713-bug-mcp-ping-idle-disconnect.md#L74-L84`: Claude Code version, start/end timestamps, original Python and `ws-mcp` PIDs, post-idle PID checks, subsequent tool-call result, and relevant MCP log tail. If either original PID dies, create the separate evidence-backed host-lifecycle follow-up required by the ticket rather than widening this implementation.

## Verification Plan
- `cd agents-plugin-tool && go test ./internal/mcp -run 'TestServeStdio.*Ping' -count=1`
- `cd agents-plugin-tool && go test ./...`
- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `cd agents-plugin-tool && scripts/smoke-ws-mcp.sh ..`
- On native Windows Claude Code, connect wsflow, capture the Python and `ws-mcp` PIDs plus timestamp/version, perform no MCP tool calls for at least 20 minutes, confirm those exact PIDs remain alive, then make a wsflow tool call and retain the relevant MCP log tail.

## Escalations
- None.
