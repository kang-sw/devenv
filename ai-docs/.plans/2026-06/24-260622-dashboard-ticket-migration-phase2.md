# Survey: 260622-dashboard-ticket-migration-phase2

## Reusable Components
- `ws-dashboard/crates/core/src/activity.rs#L10-L23` — `ActivityFeed`: browser-facing feed already separates source-neutral `items` from compatibility `agents`; directly relevant to documenting the future/public Activity shape.
- `ws-dashboard/crates/core/src/activity.rs#L90-L150` — `ActivityItem`, `ActivityTranscript`, `TranscriptBlock`: current opaque `activity_id` + normalized transcript block contract; useful as ground truth for route/payload wording.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L75-L83` — `WorkRootActivityProjectionConfig`: daemon-private `WS_CACHE_HOME`/Codex-home inputs are already kept out of browser identity.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L187-L263` — Activity route handlers: existing endpoints resolve `workRootId` to daemon-owned root paths before projecting feed, transcript, and SSE events.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L436-L460` — wsstate layout resolver: derives `<cacheHome>/proj/<worktreeKey>/agents` compatibility source from Git identity.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L462-L508` — `project_blocking`: current implementation maps registry records to compatibility `agents`, extends source-neutral `items`, and returns one `ActivityFeed`.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L60-L109` — `read_activity_agent_records`: read-only SQLite `agent_defs` compatibility metadata source.
- `ws-dashboard/crates/daemon/src/work_root_activity_registry.rs#L111-L174` — `read_activity_agent_instance_records`: read-only SQLite retained-instance compatibility source for historical Activity items.
- `agents-plugin-tool/internal/mcp/server.go#L661-L667` — `spec_index.verify` dispatch: verification tool resolves the project root through the normal root-aware MCP path.

## Existing Patterns
- Source-neutral feed with compatibility projection: see `ai-docs/spec/ws-web-dashboard/index.md#L523-L568` — spec already says Activity Items are public and `agents` is compatibility; adjacent prose can be audited against this pattern.
- Current SQLite compatibility source with stable anchor: see `ai-docs/spec/ws-web-dashboard/index.md#L570-L600` — preserves `{#260525-ws-dashboard-sqlite-agent-activity-source}` while describing implemented registry behavior.
- Bounded transcript adapters: see `ai-docs/spec/ws-web-dashboard/index.md#L718-L752` — native records normalize into `TranscriptBlock` and avoid raw paths/session ids.
- Daemon-private route identity: see `ai-docs/spec/ws-web-dashboard/index.md#L97-L110` — browser APIs use opaque ids rather than host paths, wsstate paths, or runtime identifiers.
- Linked-server gateway locality: see `ai-docs/spec/ws-web-dashboard/index.md#L126-L145` — local gateway exposes `serverId` and bounded remote state without leaking endpoint/cache/path details.
- Documentation verification convention: see `agents-plugin-tool/internal/wsdoc/conventions/spec-conventions.md#L63-L68` — spec writes are followed by `ws/spec_index.verify`.

## Relevant Interfaces
- `ai-docs/tickets/ready/260622-chore-ws-dashboard-existing-ticket-migration.md#L32-L44` — Phase contract: documentation-only cleanup, no caller-visible behavior, preserve implemented contracts while recovering daemon-private ferrule/session invariant.
- `ai-docs/tickets/ready/260622-chore-ws-dashboard-existing-ticket-migration.md#L92-L115` — Phase 2 questions: target stale named-agent/SQLite authority, three-id separation, daemon-private binding, and compatibility-vs-provider wording.
- `ai-docs/tickets/todo/260622-epic-ws-dashboard-session-key-realignment.md#L77-L97` — cross-child decisions: top-level harness owns lead key; daemon stores `ws.ferrule(root)` result privately; `wsSessionKey`, `providerSessionId`, and `activityId` stay distinct.
- `ai-docs/tickets/todo/260622-research-ws-dashboard-ferrule-session-binding.md#L51-L87` — identity terms: exact meanings and privacy boundary for `wsSessionKey`, `providerSessionId`, and `activityId`.
- `ai-docs/tickets/todo/260622-research-ws-dashboard-ferrule-session-binding.md#L88-L103` — linked-server boundary: selected target daemon mints/resolves keys locally; browser continues with `serverId`, `workRootId`, `activityId`.
- `ai-docs/tickets/todo/260622-research-ws-dashboard-ferrule-session-binding.md#L105-L136` — current dashboard impact: existing Activity shape is reusable; daemon/frontend source layer still has named-agent compatibility assumptions.
- `ai-docs/tickets/todo/260624-feat-ws-dashboard-managed-cli-terminal.md#L35-L63` — deferred managed CLI boundary: terminal-first milestone, no structured Activity parsing, no browser exposure of ferrule/bootstrap/session keys.
- `ai-docs/tickets/idea/260620-feat-ws-dashboard-agent-client-activity-sources.md#L41-L47` — deferred provider-adapter premise: current named-agent wsstate/SQLite is stale as main authority, while `items`/transcript shape remains useful.
- `ai-docs/tickets/idea/260620-feat-ws-dashboard-agent-client-activity-sources.md#L73-L93` — future browser identity and prior art: keep provider ids/session keys private; new providers flow through `items`, not `agents`.
- `ai-docs/mental-model/ws-web-dashboard.md#L35-L36` — Activity entry points: core/daemon/frontend files that own feed, stream, merge, and transcript adapters.
- `ai-docs/mental-model/ws-web-dashboard.md#L60-L60` — module contract: daemon is not ws MCP authority; WorkRoot Activity consumes wsstate/wsagent as daemon projection.
- `ai-docs/mental-model/ws-web-dashboard.md#L151-L151` — change recipe: Activity changes must preserve source-neutral `items`, compatibility `agents`, redaction, cursoring, and route tests.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L33-L36` — spec currently forbids the daemon from becoming canonical ws MCP root, harness, model backend, or named-agent session owner.
- `ai-docs/spec/ws-web-dashboard/index.md#L542-L550` — implementation-gap note already marks provider adapters as missing and forbids provider session ids/ws session keys/cache paths/process ids/raw event ids as browser authority.
- `ai-docs/spec/ws-web-dashboard/index.md#L564-L568` — Activity read model remains read-only and opaque-id based; no agent/exec control actions are added by this documentation phase.
- `ai-docs/mental-model/documentation-system.md#L21-L24` — spec identity is body-anchor based and `spec_index.verify` checks duplicate anchors only, not stale behavior.
- `ai-docs/mental-model/mcp-runtime.md#L11-L20` — root-aware ws MCP tools require mandatory `session_key`; `ws.ferrule` is the only root acceptor and session records are filesystem-backed.
- `ai-docs/mental-model/named-agent-runtime.md#L81-L91` — named-agent registry metadata is SQLite-backed while payload bodies remain file-backed; dashboard Activity depends on that compatibility layout.

## Risk Signals
- `ai-docs/spec/ws-web-dashboard/index.md#L473-L486` — Possible contract risk: WorkRoot Activity Projection still frames the projection as “read-only named-agent activity” from wsstate/wsagent; this may obscure that named-agent data is compatibility behavior, not future Activity authority.
- `ai-docs/spec/ws-web-dashboard/index.md#L488-L521` — Possible contract risk: badge and pane sections still say named-agent counts/rows/refreshes; future source-neutral Activity readers may inherit legacy copy unless the compatibility boundary is explicit.
- `ai-docs/spec/ws-web-dashboard/index.md#L570-L577` — Possible contract risk: SQLite section says SQLite registry is the named-agent metadata authority; lead/planner should inspect wording so it remains current compatibility source, not long-term provider authority.
- `ai-docs/mental-model/ws-web-dashboard.md#L7-L8` — Possible mental-model risk: related-domain summary names wsstate named-agent records without the newer ferrule/session/private-binding split.
- `ai-docs/mental-model/ws-web-dashboard.md#L77-L78` — Possible shortcut risk: a long module contract accurately describes current wsstate/SQLite mechanics, but does not locally name `wsSessionKey`/`providerSessionId`/`activityId`; implementers may miss the new identity split.
- `ai-docs/mental-model/ws-web-dashboard.md#L213-L215` — Possible shortcut risk: common mistakes forbid exposing old wsstate/session/native records, but do not yet mention ws session keys or provider session ids explicitly.

## Opinion
- `ws/mental_models.find` was unavailable in this delegated tool context (`tool_search` found no matching callable tool); direct mental-model files were read instead.
- The survey did not find a need to escalate: the brief’s scope is documentation-only and the cited tickets/specs provide enough contract evidence to distinguish implemented compatibility behavior from deferred provider-adapter work.
- `_index.md` already lists this ticket in focus with the same Phase 2 scope; refresh only if the implementer materially changes status/focus wording.
