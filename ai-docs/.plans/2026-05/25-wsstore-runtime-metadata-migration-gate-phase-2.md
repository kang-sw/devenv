# Survey: 25-wsstore-runtime-metadata-migration-gate-phase-2

## Reusable Components
- `agents-plugin-tool/internal/wsstore/metadata_inventory.go#L57-L66` — `AgentInternalKey`: already implements the Phase 1 actor-scoped key shape (`actor:<escaped actor id>:name:<escaped public name>`) and global compatibility key shape.
- `agents-plugin-tool/internal/wsstore/metadata_inventory.go#L68-L87` — `ClassifyFileBackedPayload`: classifies missing path-indexed payload files as recoverable consistency states without moving payload bytes into SQLite.
- `agents-plugin-tool/internal/wsstore/store.go#L98-L132` — `Manager.Open`: resolves the worktree state database through `wsstate.Ensure`, configures SQLite, runs migrations, and shares process-local write serialization per database path.
- `agents-plugin-tool/internal/wsstore/store.go#L508-L513` and `agents-plugin-tool/internal/wsstore/retry.go#L22-L49` — `execWrite` / `withSQLiteRetry`: short write helper with bounded `SQLITE_BUSY`/`SQLITE_LOCKED` retries suitable for metadata writes.
- `agents-plugin-tool/internal/wsstate/paths.go#L141-L164` and `agents-plugin-tool/internal/wsstate/paths.go#L174-L195` — `Manager.Ensure` / `layoutFor`: derives the worktree cache layout; existing agent payload directories are under `Layout.AgentsDir`.
- `agents-plugin-tool/internal/wsagent/agent.go#L2252-L2257` — `AgentKey`: legacy file-backed public-name-to-path normalization used for existing `agent.json` compatibility/import lookups.
- `agents-plugin-tool/internal/wsagent/agent.go#L2259-L2287` — `readAgent` / `writeAgent`: current JSON metadata helpers; `readAgent` is useful as bounded read-only legacy import input, while `writeAgent` is the file-backed metadata write path to retire/avoid.
- `agents-plugin-tool/internal/wsagent/agent.go#L2316-L2347` — `readCurrentCall` / `writeCurrentCall`: current-call JSON helpers; current-call can stay file-backed unless touched to preserve behavior, but active-call checks depend on these helpers today.
- `agents-plugin-tool/internal/wsagent/agent.go#L1047-L1093` — `withChildSetupInstruction` / `ensureAgentChildSetup`: maintains child actor setup blocks and updates child actor metadata in agent records when actor-bound calls discover older agents.

## Existing Patterns
- Registration materializes prompt payloads and metadata separately: see `agents-plugin-tool/internal/wsagent/agent.go#L413-L514` — prompt bytes go to `system.md`, events to `events.jsonl`, and registry metadata currently goes to `agent.json`.
- Call setup keeps long-running execution outside metadata critical sections: see `agents-plugin-tool/internal/wsagent/agent.go#L740-L828` — writes prompt/current-call metadata, appends an event, starts the async worker, then records the worker pid.
- Backend session id persistence is incremental metadata mutation during stream parsing: see `agents-plugin-tool/internal/wsagent/agent.go#L639-L659` — `OnSessionID` updates agent metadata and current-call metadata before appending diagnostics.
- Result consumption requires terminal call state plus output file before returning text: see `agents-plugin-tool/internal/wsagent/agent.go#L1145-L1194` — completed calls read `output.md`; successful ephemeral results erase the agent.
- Status combines registry metadata and current-call metadata in stable text output: see `agents-plugin-tool/internal/wsagent/agent.go#L1358-L1446` — callers currently see backend/tier/model/session plus call paths and follow-up guidance.
- Tail/debug streams stay file-backed and tolerate missing payload files: see `agents-plugin-tool/internal/wsagent/agent.go#L1474-L1519` and `agents-plugin-tool/internal/wsagent/agent.go#L1617-L1645` — missing event/runtime/stdout/stderr/output paths render `(missing)`.
- MCP agent dispatch is a thin adapter over `wsagent.Manager`: see `agents-plugin-tool/internal/mcp/server.go#L846-L897` and `agents-plugin-tool/internal/mcp/server.go#L913-L1031` — register/call/result/status/tail/cancel/erase all resolve roots then call manager methods.
- Actor bootstrap/recovery persists actors in wsstore and binds session state in memory: see `agents-plugin-tool/internal/mcp/server.go#L1165-L1227` and `agents-plugin-tool/internal/mcp/server.go#L1238-L1271`.
- Child actor reuse is currently discovered through `wsagent.Agent(root,name)`: see `agents-plugin-tool/internal/mcp/server.go#L1319-L1331` — this path will need the migrated registry lookup semantics to preserve child actor reuse.
- Explicit-root compatibility and root-omitted actor gating are already tested: see `agents-plugin-tool/internal/mcp/server_test.go#L600-L640` and `agents-plugin-tool/internal/mcp/server_test.go#L643-L739`.
- Registration/call/ephemeral result behavior has package-level tests to extend: see `agents-plugin-tool/internal/wsagent/agent_test.go#L217-L252`, `agents-plugin-tool/internal/wsagent/agent_test.go#L897-L932`, and `agents-plugin-tool/internal/wsagent/agent_test.go#L1576-L1634`.
- Wsstore inventory and key-shape tests already parse source instead of importing runtime consumers: see `agents-plugin-tool/internal/wsstore/store_test.go#L412-L433` and `agents-plugin-tool/internal/wsstore/store_test.go#L527-L549`.

## Relevant Interfaces
- `agents-plugin-tool/internal/wsagent/agent.go#L80-L94` — `RegisterOptions`: carries public name, backend/model/prompt inputs, ephemeral flag, and child actor setup metadata.
- `agents-plugin-tool/internal/wsagent/agent.go#L101-L108` — `CallOptions`: public call input plus optional child actor metadata used when MCP actor binding is active.
- `agents-plugin-tool/internal/wsagent/agent.go#L147-L154` — `ResultOptions`: result timeout/context and ephemeral erase callback used by MCP to mark reader actors inactive.
- `agents-plugin-tool/internal/wsagent/agent.go#L336-L356` — `Agent`: exact registry metadata fields now in `agent.json`, including backend/model/effort, session id, prompt/system/output path indexes, child actor metadata, capabilities, and ephemeral flag.
- `agents-plugin-tool/internal/wsagent/agent.go#L367-L385` — `CurrentCall`: file-backed current-call fields; only migrate or mirror if needed for registration/status/result/erase/restart behavior.
- `agents-plugin-tool/internal/wsagent/agent.go#L387-L403` — `Layout`: canonical file payload paths for agent directory, current streams, output, events, and system prompt.
- `agents-plugin-tool/internal/wsstore/store.go#L53-L61` — `Actor`: actor ids and root/worktree binding used to decide actor-local registry keys.
- `agents-plugin-tool/internal/wsstore/store.go#L63-L73` — `Artifact`: generic artifact path/byte-count metadata row; relevant if path indexes are recorded separately from agent registry rows.
- `agents-plugin-tool/internal/wsstore/store.go#L538-L589` — current schema includes placeholder `agent_defs`, `agent_calls`, and `exec_jobs` tables with only key/path/status columns; Phase 2 registry metadata needs more fields or an extension table.
- `agents-plugin-tool/internal/mcp/server.go#L30-L41` — `Server` session state: `sessionRoot`, `sessionHarness`, `sessionActorID`, and `sessionActorAuthority` are the available actor-scope inputs at MCP dispatch time.
- `agents-plugin-tool/internal/mcp/server.go#L1418-L1440` — `actorGate` / `rootOmittedActorTool`: only `agents.register`, `agents.call`, and `subquery` require actor setup when root is omitted; other `agents.*` reads use compatibility root resolution.

## Constraints
- `ai-docs/spec/named-agent-runtime.md#L10-L26` — Agent metadata moves to SQLite authority, but prompt text, system prompt text, stdout/stderr/runtime logs/events/final outputs remain file-backed payloads.
- `ai-docs/spec/mcp-tools.md#L64-L90` — Public `agents.*` schemas remain rootless and root resolution flows through session setup/compatibility, not a visible root argument.
- `ai-docs/mental-model/named-agent-runtime.md#L14-L33` — Actor-bound registry keys must include actor scope; global compatibility keys are only for unbound compatibility.
- `ai-docs/mental-model/mcp-runtime.md#L63-L73` — SQLite is metadata/control-plane only and long subprocess/model calls must not hold transactions.
- `agents-plugin-tool/internal/wsagent/agent.go#L458-L467` — Re-registering currently refuses active calls before resetting the entire agent directory; migrated registration must preserve the active-call safety invariant without relying only on removing the directory.
- `agents-plugin-tool/internal/wsagent/agent.go#L1181-L1188` — `Result` currently returns an error if `output.md` is missing on a completed call, not a formatted recoverable status.
- `agents-plugin-tool/internal/wsagent/agent.go#L2122-L2128` — `Erase` only removes the file directory today; migrated registry rows will need separate deletion/tombstone handling to avoid stale SQLite registrations.
- `agents-plugin-tool/internal/wsagent/agent.go#L2252-L2257` — Legacy path keys can collide after normalization; actor-scoped SQLite keys must not reuse this as the authoritative identity.

## Risk Signals
- `agents-plugin-tool/internal/wsstore/store.go#L561-L578` — Possible contract risk: `agent_defs` and `agent_calls` currently store only skeletal columns, while the brief requires SQLite authority for nearly every `Agent` metadata field; planner/implementer should inspect whether to alter schema or add a dedicated metadata table.
- `agents-plugin-tool/internal/wsagent/agent.go#L503-L504`, `agents-plugin-tool/internal/wsagent/agent.go#L587-L589`, `agents-plugin-tool/internal/wsagent/agent.go#L645-L648`, and `agents-plugin-tool/internal/wsagent/agent.go#L721-L727` — Possible shortcut risk: many lifecycle paths call `writeAgent`, so leaving one path behind would preserve `agent.json` as a parallel write authority.
- `agents-plugin-tool/internal/mcp/server.go#L929-L1031` — Possible actor-scope risk: read/diagnostic tools (`agents.status`, `wait`, `result`, `tail`, `cancel`, `erase`) currently do not pass actor id to `wsagent`, so same-name actor-local lookup cannot be implemented inside `wsagent` without threading scope through these calls or a resolver.
- `agents-plugin-tool/internal/wsagent/agent.go#L1070-L1093` — Possible payload/metadata risk: `ensureAgentChildSetup` updates both `system.md` bytes and child actor metadata; after migration, this must split file-backed prompt edits from SQLite metadata writes.
- `agents-plugin-tool/internal/wsagent/agent.go#L465-L468` and `agents-plugin-tool/internal/wsagent/agent.go#L2122-L2128` — Possible file-payload risk: directory removal currently deletes metadata and payloads together; migrated SQLite rows plus file-backed payloads need coordinated cleanup without treating missing payloads as missing registrations.
- `agents-plugin-tool/internal/wsagent/agent.go#L1177-L1184` — Possible recoverability risk: missing completed output currently returns `read output` error; the brief asks missing file-backed payload paths to report a recoverable consistency state.

## Opinion
- The survey found enough code evidence to support implementation without research. The biggest codebase reality mismatch is that actor scope currently lives in MCP server memory while `wsagent.Manager` APIs are mostly `(root, name)`; this is an integration seam, not a contract ambiguity.
- Phase 1 docs still say named agents are file/JSON-backed until a migration ticket rewires `wsagent` (`ai-docs/mental-model/named-agent-runtime.md#L12-L16`); for this brief that statement is a stale pre-Phase-2 assumption, not an implementation blocker.
