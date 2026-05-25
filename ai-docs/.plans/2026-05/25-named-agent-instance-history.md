# Implementation Plan: 260525-feat-named-agent-instance-history

## Scope Evidence
- `ai-docs/.plans/2026-05/25-named-agent-instance-history.brief.md#L20-L42` — Public `agents.*` APIs stay name-based; SQLite must own role, instance, path, retention, cleanup, and tombstone metadata while payload bytes remain file-backed.
- `ai-docs/spec/named-agent-runtime.md#L13-L37` — Current spec still says re-registering replaces payload directory and metadata; closeout must update this to role-pointer/instance-history semantics.
- `ai-docs/mental-model/named-agent-runtime.md#L26-L33` — Existing runtime model uses `AgentInternalKey(actorID, publicName)` and treats `agent.json` as read-only import, with `current/state.json` as active-call authority.

## Source Touch Points
- `agents-plugin-tool/internal/wsstore/store.go#L64-L87` — Replace or split `AgentDefinition` as role plus instance metadata; include stable role key, instance id/key, current pointer, state path, created/seen/call/output fields, ephemeral, child actor, and retention/cleanup fields.
- `agents-plugin-tool/internal/wsstore/store.go#L263-L342` — Add migrations for new role/instance tables and compatibility migration from existing `agent_defs`; keep short SQLite writes using the existing migration and retry patterns.
- `agents-plugin-tool/internal/wsstore/store.go#L435-L512` — Replace single-row `UpsertAgentDefinition`/`AgentDefinition`/`DeleteAgentDefinition` APIs or wrap them with role resolution helpers such as current-role lookup, create-instance, pointer advance, hide-role, and instance-retention updates.
- `agents-plugin-tool/internal/wsstore/store.go#L711-L858` — Reuse the bounded `PruneExpired`/tombstone shape for agent-instance cleanup, but query recorded instance paths from SQLite; do not walk agent directories.
- `agents-plugin-tool/internal/wsstore/metadata_inventory.go#L57-L66` — Keep `AgentInternalKey` as role identity; add instance-id semantics without weakening actor/global namespace separation.
- `agents-plugin-tool/internal/wsagent/agent.go#L513-L576` — Centralize current-role resolution here so all manager operations get a layout built from the current instance `StatePath`, not from public name alone.
- `agents-plugin-tool/internal/wsagent/agent.go#L619-L679` — Change registration from remove-and-overwrite to: resolve current role, reject active current instance, build a new instance payload dir, write files, create instance, then advance role pointer as the final transactional step.
- `agents-plugin-tool/internal/wsagent/agent.go#L909-L1007` — Ensure async call setup writes/runs against the resolved current instance and keeps hidden actor id propagation intact.
- `agents-plugin-tool/internal/wsagent/agent.go#L1323-L1402` — Successful ephemeral result should hide/remove the role pointer and mark the instance retention-eligible, not delete the payload directory synchronously.
- `agents-plugin-tool/internal/wsagent/agent.go#L1415-L1709` — `wait`, `status`, `tail`, and diagnostic streams must all resolve the current instance before reading `current/state.json`, events, logs, streams, or output.
- `agents-plugin-tool/internal/wsagent/agent.go#L1852-L1927` and `#L2202-L2255` — `cancel` and inbox delivery must resolve to the same instance as `call` and worker hooks.
- `agents-plugin-tool/internal/wsagent/agent.go#L2319-L2342` — Change erase from metadata delete plus `RemoveAll` to role hide/removal plus retention scheduling.
- `agents-plugin-tool/internal/wsagent/agent.go#L2348-L2401` — Layout derivation currently falls back to public-name directories and only consults `StatePath` for actor scoped rows; update this path to resolve current instance state paths for global and actor roles, while generating unique new instance dirs for registration.
- `agents-plugin-tool/internal/mcp/server.go#L847-L1053` — MCP dispatch already passes actor scope for most agent tools; preserve schemas and route through updated manager APIs. Include deprecated `agents.print` in actor-scoped role resolution for root-omitted calls because the ticket keeps the whole public `agents.*(name)` surface name-based.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L986-L1085` — Hidden `--actor-id` worker and check-inbox CLI paths must keep resolving the current instance selected by parent MCP dispatch.

## Test Plan
- `agents-plugin-tool/internal/wsstore/store_test.go#L587-L617` — Replace single `AgentDefinition` round-trip with role+instance round-trip, current pointer movement, failed pointer-update rollback, actor/global collision, and legacy migration/import coverage.
- `agents-plugin-tool/internal/wsstore/store_test.go#L136-L242` and `#L662-L734` — Add agent-instance retention cleanup cases: due retired instance deletes only recorded payload path, skips current/active/pinned/recovery, records retry fences on cleanup failure, and never depends on directory discovery.
- `agents-plugin-tool/internal/wsagent/agent_test.go#L1992-L2032` — Rewrite reset-on-register test to assert old output/payload dir survives, a new instance dir is current, and active current instance still rejects registration.
- `agents-plugin-tool/internal/wsagent/agent_test.go#L1575-L1633` — Update ephemeral result consumption to assert role hidden/removed while payload dir remains until retention cleanup.
- `agents-plugin-tool/internal/wsagent/agent_test.go#L2429-L2455` — Extend legacy `agent.json` import to verify a global role plus first instance is created and `agent.json` stays retired as write authority.
- `agents-plugin-tool/internal/wsagent/agent_test.go#L2457-L2560` — Extend actor-scoped same-name and inbox/hook tests to prove current-instance resolution after re-registration under actor and global namespaces.
- `agents-plugin-tool/internal/mcp/server_test.go#L2098-L2189` — Extend public MCP lifecycle coverage for actor-scoped/global same-name re-registration, result/wait/tail/cancel/erase semantics, and explicit-root global compatibility.
- `agents-plugin-tool/cmd/ws-mcp/main_test.go#L426-L426` — Add or update CLI mirror smoke coverage for hidden worker/check-inbox actor-id paths if layout resolution changes affect CLI behavior.

## Risk Points
- `agents-plugin-tool/internal/wsagent/agent.go#L630-L633` — Current registration deletes the payload directory before creating the replacement; this directly violates history preservation and must be removed without weakening active-call rejection.
- `agents-plugin-tool/internal/wsagent/agent.go#L668-L679` — Registration currently writes metadata before the final event; if file writes/event writes fail after pointer movement, the brief requires failed registration not to advance the role pointer.
- `agents-plugin-tool/internal/wsagent/agent.go#L2319-L2342` — Current erase synchronously deletes payloads; changing this affects `oneShot` and ephemeral result tests that currently expect directory removal.
- `agents-plugin-tool/internal/mcp/server.go#L1037-L1044` — MCP `agents.print` ignores actor scope while the brief includes `agents.print` in name-based APIs; implement it as actor-scoped for root-omitted calls and keep hidden explicit-root compatibility global.
- `agents-plugin-tool/internal/wsstore/store.go#L775-L778` — `Count` allowlist excludes `agent_defs`; tests may need explicit helpers/count support for role/instance tables without exposing runtime internals broadly.
- `agents-plugin-tool/internal/wsstore/store.go#L861-L870` — `removeArtifactPath` removes one path; agent instance payload cleanup likely needs directory removal semantics and Windows-safe retry behavior.

## Verification Commands
- `go test -count=1 ./internal/wsstore ./internal/wsagent ./internal/mcp` from `agents-plugin-tool/`.
- `go test -count=1 ./...` from `agents-plugin-tool/`.
- Native Windows coverage for cleanup/path timing: run the same targeted package command on the configured Windows host and include at least the agent-instance retention tests.

## Lead/Closeout Notes
- No unresolved design blocker found in source survey. Lead decision: deprecated `agents.print` is still part of the public `agents.*` family for this ticket, so it must resolve through actor-scoped role pointers on root-omitted MCP calls.
- Closeout must update `ai-docs/spec/named-agent-runtime.md#L22-L37` and `ai-docs/mental-model/named-agent-runtime.md#L26-L33` after implementation because they still describe mutable single-agent records/reset behavior.
