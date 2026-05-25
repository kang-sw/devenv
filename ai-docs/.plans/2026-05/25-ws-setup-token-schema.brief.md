# Brief: ws setup token schema

## Intent

Implement the two selected setup-runtime tickets in one runtime slice: shorten
`ws.setup` actor recovery tokens and hide the setup-only `format` affordance
from the public MCP schema while preserving hidden compatibility.

## Scope Boundary

Selected scope:

- `ai-docs/tickets/ready/260525-bug-ws-setup-actor-token-format.md` Phase 1:
  Short setup actor recovery tokens.
- `ai-docs/tickets/ready/260525-bug-ws-setup-format-schema-attention.md`
  Phase 1: Hide setup-only format affordance.

Do not implement `260525-bug-ws-setup-cwd-plugin-cache-root`. Do not change
non-setup tool schemas. Do not change actor authority semantics, named-agent
public names, or named-agent instance-history behavior.

## Caller-Visible Contract

`ws.setup(method: "lead-workflow-bootstrap", root:
"<absolute-working-directory>")` should return a compact authority-prefixed
actor id such as `lead-k9f2p7qx`. The random payload must be lowercase and
case-insensitive in practice; prefer `a-z0-9` generated payload characters and
avoid uppercase. `-` should remain the authority separator.

`ws.setup(id: "<actor-id>")` must recover newly minted lead, delegate, and
reader actors after MCP restart. Existing long actor ids already persisted in
runtime state should remain recoverable if practical; if not practical, stop and
escalate before dropping compatibility.

The public `ws.setup` input schema must omit `format`. Dispatch must still
accept explicit hidden `format: "json"` and return the existing structured JSON
body. Default readable setup output remains unchanged.

## Contract Instructions

- Update setup actor minting/parsing/recovery in `agents-plugin-tool/internal/mcp/server.go`.
- Do not parse worktree routing details out of newly minted visible actor ids.
  Runtime state must resolve the actor token to stored actor/worktree metadata.
- Preserve `validActorAuthority` semantics for `lead`, `delegate`, and `reader`.
- Keep hidden `format` dispatch in setup handling, but remove only setup's
  public schema property.
- Keep setup alias behavior coherent when `WS_MCP_SETUP_TOOL` changes the
  advertised setup name.
- Reuse existing `wsstore.Actor` persistence and existing schema helpers before
  adding new abstractions.
- Do not hide `format` from `runtime.info`, Git, spec, ticket, mental-model, or
  other structured-output tools.

## Integration Test Instructions

Extend existing Go tests rather than adding a separate test harness unless the
current files cannot express the behavior.

Required coverage:

- New actor ids match the compact lowercase authority-prefixed token shape.
- Bootstrap/restart recovery works for new short lead actor ids.
- Child actor prompt injection and child recovery work for new short delegate
  ids.
- Root-omitted actor-scoped named-agent or subquery dispatch still uses actor
  scope.
- Actor id collision retry or deterministic collision coverage exists.
- `tools/list` public schema for `ws.setup` omits `format`.
- Hidden `ws.setup` dispatch still accepts `format: "json"`.
- Setup alias schema behavior remains coherent when `WS_MCP_SETUP_TOOL` is set.

Likely targeted commands:

```text
go test ./agents-plugin-tool/internal/mcp -run 'TestRawPublicAgentToolSchemasOmitRoot|TestServeStdioToolsListAndCall|TestServeStdioFiltersToolsByProfile|TestServeStdioDelegateProfileRejectsSetupMutation|TestServeStdioNoAgentModeHidesAgentBackedTools|TestServeStdioSetupRootAndExplicitOverride|TestServeStdioActorSetupBootstrapAndRecovery|TestServeStdioChildActorPromptInjection'
go test ./agents-plugin-tool/internal/wsagent -run 'TestSubqueryInjectsChildActorSetupWithoutDelegateOrientation|TestAgentMetadataImportsLegacyAgentJSONReadOnly|TestCorruptLegacyAgentJSONReportsBoundedRecovery|TestActorScopedRegistrationsWithSameNameDoNotCollide|TestActorScopedSubqueryRegistersAndCallsSameScope|TestSelfWorkerStarterPropagatesHiddenActorID'
go test ./agents-plugin-tool/internal/wsstore -run 'TestAgentInternalKeyScopesPublicNamesByActor|TestAgentDefinitionsPersistSQLiteMetadata|TestAgentRolePointerHistoryAndCollision'
```

Run broader package tests if targeted changes expose additional affected tests.

## Implementation Strategy Decisions

- Treat actor ids as opaque recovery handles. Do not expose worktree routing in
  the visible token.
- Keep generated payloads lowercase to reduce recovery entry errors by humans
  or agents.
- Prefer generated payload alphabet `a-z0-9`; avoid uppercase even if matching
  remains case-sensitive internally.
- Use collision retry on mint instead of lengthening tokens to UUID scale.
- Keep `format` accepted by dispatch as a hidden compatibility argument.
- Scope schema hiding to setup only.

## Rejected Alternatives

- UUID-style actor ids: rejected because they are too long for human-visible
  recovery guidance and global uniqueness is not required.
- Case-sensitive base62/nanoid payloads: rejected because recovery tokens may be
  remembered or re-entered and uppercase increases input ambiguity.
- Hiding `format` from every MCP tool: rejected for this slice because discovery
  and Git tools have stronger structured-consumer use cases.
- Removing setup JSON output: rejected because tests and compatibility callers
  may still depend on it.

## Approach

- Survey current `mintActorID`, `actorWorktreeKey`, setup restore, and child
  actor minting flows.
- Add the smallest runtime lookup/index needed for short actor recovery without
  visible worktree parsing.
- Update setup schema construction so setup omits `format` publicly while
  dispatch still reads it.
- Extend targeted tests for token shape, recovery, child actors, hidden format,
  alias schema, and collision behavior.

## Constraints

- Preserve legacy long actor recovery when practical.
- Preserve SQLite-backed actor metadata authority.
- Preserve file-backed named-agent payload boundaries.
- Keep MCP output as text content; JSON setup format remains a text payload.
- Do not introduce root guessing or cwd placeholder behavior changes.

## Out of scope

- `260525-bug-ws-setup-cwd-plugin-cache-root`.
- Global actor namespace redesign.
- Non-setup schema cleanup.
- Runtime release packaging.
- Dashboard behavior.

## Details

The current long token shape includes enough data for `actorWorktreeKey` to open
worktree state. New short ids cannot rely on that visible parse. If a global or
cache-local actor lookup index is needed, keep it metadata-only and bounded to
actor recovery. Preserve authority validation and reject malformed ids with
clear setup errors.

`format` should disappear only from the schema returned to callers. Existing
call paths that already pass `format` must continue to work.

## Verification Contract

Before reporting completion:

- Run the targeted MCP, wsagent, and wsstore tests listed above, or explain any
  replaced/broadened command.
- Read the full output of each command.
- Include the exact commands and pass/fail result in the implementer report.
- Commit logical source/test changes on this implementation branch.

## References

- [Must] `ai-docs/mental-model/mcp-runtime.md` - setup, schema, actor binding,
  and MCP tool-surface invariants.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - child actor metadata,
  actor-scoped named-agent dispatch, and file-backed payload boundaries.
- [Must] `agents-plugin-tool/internal/mcp/server.go:222-223,327-378,1142-1318,2092-2124,2729-2830` - setup dispatch, schema, actor mint/recover, alias plumbing.
- [Must] `agents-plugin-tool/internal/wsagent/agent.go:442-562,620-676,906-952,1225-1302,2345-2415` - child actor metadata and actor-scoped agent/subquery flow.
- [Must] `agents-plugin-tool/internal/wsstore/store.go:55-63,476-507` - actor persistence boundary.
- [Must] `agents-plugin-tool/internal/wsstore/metadata_inventory.go:57-65` - actor metadata inventory.
- [Maybe] `agents-plugin-tool/internal/mcp/server_test.go:61-83,85-220,520-570,576-750,790-860,1078-1145` - setup/schema/recovery tests to extend.
- [Maybe] `agents-plugin-tool/internal/wsagent/agent_test.go:2157-2203,2457-2503,2506-2660` - actor-scoped child/subquery tests.
- [Maybe] `agents-plugin-tool/internal/wsstore/store_test.go:527-549,587-640` - actor key and persistence tests.
