---
title: ws dashboard agent-client activity sources
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260605-research-ws-native-subagent-pivot: supersedes dashboard deprecation and preserves the web dashboard while moving agent visibility away from removed agents.* surfaces
  260523-feat-ws-dashboard-main-session-activity-source: prior main-session freshness gap that must be re-grounded on host-owned agent/client activity
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: serverId forwarding must carry Activity source identity before remote provider streams are transparent
spec:
  - 260521-ws-dashboard-activity-console-read-model
  - 260521-ws-dashboard-activity-console-ui-shell
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
---

# ws dashboard agent-client activity sources

## Background

The dashboard was briefly marked for deprecation during the native-subagent
pivot, then explicitly retained as a usable web-tmux-style surface. The retained
surface should not restart direct harness development. As of the 2026-06-24
dashboard direction discussion, this structured Activity adapter track is
deferred back to idea level. The nearer todo milestone is
`260624-feat-ws-dashboard-managed-cli-terminal`, which provides a terminal-first
managed vendor CLI surface and browser-side composer without treating provider
protocol adapters as the first milestone.

When this idea is revisited, the dashboard still needs a small, source-neutral
activity bridge that can read host-owned agent surfaces such as Codex app-server
and OpenCode ACP, normalize their session, turn, message, tool, permission, and
status events, and render them through the existing WorkRoot Activity / Activity
Console model. OpenCode serve remains a useful read-only observation surface,
but it is not the primary counterpart to Codex app-server when the dashboard
needs an interactive agent-client bridge.

The existing Activity Console read model is the right public shape: source-
neutral `ActivityItem` rows, selected transcript backfill, bounded diagnostics,
and compatibility `agents` projection for the legacy named-agent pane. The stale
part is the assumption that named-agent wsstate/SQLite data is the main activity
authority. After the 260605 pivot, ws-owned subprocess agents survive only as the
scoped `ws.mercenary.*` path, while native Codex/OpenCode work should be treated
as host-owned agent-client data.

## Decisions

- Preserve the dashboard as a browser control plane. Do not replace it with a
  TUI and do not strip Activity visibility merely because `agents.*` was removed.
- Avoid direct harness/runtime development. The common layer is a dashboard-owned
  ACP-shaped `AgentClientProvider` contract plus Activity projection, not a new
  model loop, edit engine, permission runtime, MCP authority, or ws MCP agent
  runtime.
- Use Codex app-server and OpenCode ACP as the primary interactive provider
  sources. Codex app-server supplies JSON-RPC thread/turn/item/event data over
  its own protocol; OpenCode ACP supplies editor-agent JSON-RPC over stdio.
  Adapters translate both into the dashboard provider contract before projecting
  Activity rows and transcript blocks.
- Define an ACP-shaped internal subset instead of adopting any provider wire
  protocol as the dashboard API. The subset should cover provider
  initialization/capabilities, session list/create/resume, prompt/send,
  assistant/user messages, tool activity, permission/blocked states,
  interruption/cancellation status, file-change summaries, transcript backfill,
  and provider metadata. Codex-specific or OpenCode-specific fields remain
  adapter-private unless a browser-facing dashboard contract needs them.
- Treat OpenCode serve as an optional observation/read-only supplement. It can
  help discover sessions or stream HTTP/OpenAPI/SSE state, but the first
  OpenCode counterpart to Codex app-server is `opencode acp`, not `opencode
  serve`.
- Keep browser identity dashboard-native: `serverId`, `workspaceId`,
  `workRootId`, `activityId`, and transcript cursors. Do not expose provider
  session ids, ws `session_key` values, cache paths, transcript paths, process
  ids, or raw provider event ids as browser authority.
- Keep browser Activity read-only for this track. The provider contract may model
  interactive capabilities so Codex app-server and OpenCode ACP can share one
  adapter shape, but exposing start, interrupt, cancel, retry, erase, permission
  approval, or provider-specific steering controls in the dashboard UI requires
  later high-friction control tickets.

## Prior Art

- `ActivityFeed.items`, `ActivityTranscript`, and `TranscriptBlock` are already
  source-neutral enough to carry native agent-client activity.
- `ActivityFeed.agents` is a compatibility projection for the existing named-
  agent pane; new provider activity should flow through `items`.
- The current event vocabulary already supports item upserts/removals,
  transcript updates, snapshot invalidation, mode changes, and heartbeats.
- The dashboard daemon is explicitly not ws MCP root, harness, model-backend, or
  agent-session authority. It consumes provider state through daemon-owned view
  models and provider adapters.

## Constraints

- Provider adapters must degrade unknown or malformed events into bounded status
  or diagnostic transcript blocks instead of leaking raw JSON, paths, session ids,
  or command output payloads outside the selected transcript surface.
- Adapter contracts must be fixture-backed by captured provider events or schema
  snapshots because Codex app-server, OpenCode ACP, and OpenCode serve surfaces
  can drift by installed version.
- The first implemented subset is limited to read/list/stream/render behavior:
  source connection state, thread/session/activity rows, turn lifecycle,
  assistant/user messages, tool or command summaries, file-change summaries,
  blocked/approval-needed state, model/cwd/git metadata, and transcript backfill.
  Interactive provider methods may exist in the internal contract only when
  needed to represent the shared Codex app-server/OpenCode ACP lifecycle; the
  browser must not expose those controls in this track.
- Provider-specific controls remain out of scope. Adding UI controls before the
  read model and provider lifecycle mapping are stable would recreate the
  discarded harness-development problem.
- Server-scoped dashboard operation is a dependency for linked remote hosts.
  Activity source ids, stream subscriptions, transcript routes, and persisted UI
  state must include or derive `serverId` before remote Codex/OpenCode activity
  can be transparent.

## Public Interface Briefing

This track intentionally touches the dashboard's browser-facing Activity
interface, but the first implementation should keep the route set stable.
Existing routes remain the public entrypoints:

- `GET /api/dashboard/work-roots/{workRootId}/activity`
- `GET /api/dashboard/work-roots/{workRootId}/activity/items/{activityId}/transcript`
- `GET /api/dashboard/work-roots/{workRootId}/activity/events`

The public change is inside the source-neutral payload contract. `ActivityFeed`
continues to expose `items` as the primary Activity list and `agents` as the
legacy named-agent compatibility projection. New Codex app-server activity must
appear through `items`; it must not be forced into `agents`. `activityId` values
may gain a new dashboard-owned source prefix, but provider thread ids, turn ids,
session ids, raw event ids, process ids, and cache/transcript paths must remain
daemon-private. `ActivitySourceDisplay.kind`, item `kind`, and transcript
`renderKind` may gain new string values, so frontend parsers and tests must keep
unknown-value tolerance while rendering useful labels for known Codex values.

The watch stream may emit existing `ActivityConsoleEvent` variants for Codex and
OpenCode rows: item upserts/removals, transcript updates, snapshot invalidation,
mode changes, and heartbeats. The event vocabulary should not grow until a
concrete Codex app-server or OpenCode ACP behavior cannot be represented by the
current variants.

Transcript payloads continue to use `TranscriptBlock` as the only browser
backfill format. Codex app-server item/notification records and OpenCode ACP
messages/events must normalize into bounded user, assistant, tool/command,
file-change, approval-needed, status, and diagnostic blocks. Unknown provider
records degrade to bounded diagnostic/status blocks rather than leaking raw
provider JSON.

## Phases

### Phase 1: Agent-client provider and Activity source contract

Define the dashboard-owned ACP-shaped `AgentClientProvider` subset and the
Activity source projection that consumes it. This phase should update the ws web
dashboard spec and mental model so the intended source split is recoverable:
legacy/ws mercenary state is one compatibility source, Codex app-server is one
interactive provider source, OpenCode ACP is one interactive provider source,
OpenCode serve is an optional observation source, and the browser sees only the
dashboard Activity model.

Verification boundary: documentation and type-level tests or fixtures are enough
for this phase; no provider process needs to run. The result must make it clear
which methods and fields belong to the internal provider contract, which fields
are dashboard browser identity, which provider ids stay daemon-private, which
OpenCode serve data is observation-only, and which stale named-agent/SQLite
assumptions remain compatibility-only. Route and TypeScript contract tests should
assert that mixed source rows keep using the existing Activity routes and that
browser payloads tolerate unknown future source/provider kinds without treating
them as named agents.

### Phase 2: Codex app-server read adapter

Add a Codex app-server provider that can connect through a local transport, read
or subscribe to thread/turn/item state, map Codex lifecycle concepts into the
ACP-shaped provider subset, and project native Codex activity into `ActivityItem`
rows and `TranscriptBlock` backfill. Prefer generated or captured schema
fixtures over handwritten assumptions. For the first dogfood path, prefer the
default stdio transport and a minimal JSON-RPC subset: initialize, read/list
stored threads when available, start a thread/turn for smoke verification, and
consume thread/turn/item notifications into Activity rows and transcript blocks.
Unknown event types must degrade without breaking the whole Activity feed.

Verification boundary: fixture projection tests for representative Codex
thread/turn/item sequences, route tests proving browser payloads omit provider
session ids and raw paths, and a local smoke path that can be run when Codex
app-server is available. The WSL smoke should run against the locally installed
`codex app-server --stdio` binary and prove that a real turn can appear in the
dashboard Activity Console without requiring dashboard owner pairing when the
daemon is explicitly started through the loopback-only no-auth debug profile.

### Phase 3: OpenCode ACP provider adapter

Add an OpenCode ACP provider that starts or attaches to `opencode acp`, speaks
the ACP stdio JSON-RPC flow, maps OpenCode sessions/messages/tool activity and
permission states into the same ACP-shaped provider subset, and projects the
result into the same Activity model as Codex app-server. Keep this adapter
independent of Codex-specific assumptions; the common contract is the dashboard
provider subset and Activity projection, not either provider's wire protocol.
OpenCode serve may be used only as an optional observation/discovery supplement
if a concrete gap appears that ACP does not cover cheaply.

Verification boundary: fixture projection tests for OpenCode ACP messages/events,
bounded degradation tests for missing binary/auth, subprocess startup failure,
unreachable or incompatible ACP server state, and version drift, plus route tests
matching the same privacy and identity constraints as the Codex adapter.

### Phase 4: Activity UI and server-scoped integration

Lift the visible Activity UI from named-agent wording to source-neutral
agent-client activity. Preserve the existing Activity Console ergonomics: dense
ribbon, selected transcript, local dirty acknowledgement, bounded tail/backfill,
and read-only behavior. Thread `serverId` through Activity source selection and
stream keys before linked remote providers are considered transparent.

Verification boundary: frontend route/model tests for source-neutral labels and
identity keys, browser-level acceptance evidence for mixed source rows and
transcript rendering, and server-scoped route tests showing local compatibility
aliases still map to `server-local`.
