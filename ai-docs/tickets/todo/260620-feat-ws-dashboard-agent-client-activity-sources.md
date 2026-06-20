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
surface should not restart direct harness development. Instead, the dashboard
needs a small, source-neutral activity bridge that can read host-owned agent
surfaces such as Codex app-server and OpenCode serve, normalize their thread,
turn, message, tool, and status events, and render them through the existing
WorkRoot Activity / Activity Console model.

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
  read-model adapter, not a new execution harness, ACP implementation, or ws MCP
  agent runtime.
- Use Codex app-server and OpenCode serve as provider sources. Codex app-server
  supplies JSON-RPC thread/turn/item/event data; OpenCode serve supplies
  programmatic HTTP/OpenAPI/SSE-style data. Adapters normalize provider records
  into dashboard Activity, not the other way around.
- Treat ACP as prior art only. The dashboard may borrow its editor-to-agent
  vocabulary where it fits, but ws should define a smaller internal subset
  because Codex app-server and OpenCode serve do not expose the same wire
  contract.
- Keep browser identity dashboard-native: `serverId`, `workspaceId`,
  `workRootId`, `activityId`, and transcript cursors. Do not expose provider
  session ids, ws `session_key` values, cache paths, transcript paths, process
  ids, or raw provider event ids as browser authority.
- Keep Activity read-only for this track. Start, interrupt, cancel, retry, erase,
  permission approval, and provider-specific steering actions require later
  high-friction control tickets.

## Prior Art

- `ActivityFeed.items`, `ActivityTranscript`, and `TranscriptBlock` are already
  source-neutral enough to carry native agent-client activity.
- `ActivityFeed.agents` is a compatibility projection for the existing named-
  agent pane; new provider activity should flow through `items`.
- The current event vocabulary already supports item upserts/removals,
  transcript updates, snapshot invalidation, mode changes, and heartbeats.
- The dashboard daemon is explicitly not ws MCP root, harness, model-backend, or
  agent-session authority. It consumes runtime state through daemon-owned view
  models.

## Constraints

- Provider adapters must degrade unknown or malformed events into bounded status
  or diagnostic transcript blocks instead of leaking raw JSON, paths, session ids,
  or command output payloads outside the selected transcript surface.
- Adapter contracts must be fixture-backed by captured provider events or schema
  snapshots because Codex app-server schemas and OpenCode OpenAPI/event surfaces
  can drift by installed version.
- The first common subset is limited to read/list/stream/render behavior:
  source connection state, thread/session/activity rows, turn lifecycle,
  assistant/user messages, tool or command summaries, file-change summaries,
  blocked/approval-needed state, model/cwd/git metadata, and transcript backfill.
- Provider-specific controls remain out of scope. Adding controls before the
  read model is stable would recreate the discarded harness-development problem.
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

The watch stream may emit existing `ActivityConsoleEvent` variants for Codex
rows: item upserts/removals, transcript updates, snapshot invalidation, mode
changes, and heartbeats. The event vocabulary should not grow until a concrete
Codex or OpenCode behavior cannot be represented by the current variants.

Transcript payloads continue to use `TranscriptBlock` as the only browser
backfill format. Codex app-server item and notification records must normalize
into bounded user, assistant, tool/command, file-change, approval-needed,
status, and diagnostic blocks. Unknown provider records degrade to bounded
diagnostic/status blocks rather than leaking raw provider JSON.

## Phases

### Phase 1: Activity source contract

Define the dashboard-owned Activity source contract and normalize existing
named-agent/mercenary projection language around it. This phase should update
the ws web dashboard spec and mental model so the intended source split is
recoverable: legacy/ws mercenary state is one source, Codex app-server is another
source, OpenCode serve is another source, and the browser sees only the
dashboard Activity model.

Verification boundary: documentation and type-level tests or fixtures are enough
for this phase; no provider process needs to run. The result must make it clear
which fields are dashboard identity, which provider ids stay daemon-private, and
which stale named-agent/SQLite assumptions remain compatibility-only.
Route and TypeScript contract tests should assert that mixed source rows keep
using the existing Activity routes and that browser payloads tolerate unknown
future source kinds without treating them as named agents.

### Phase 2: Codex app-server read adapter

Add a Codex app-server source that can connect through a local transport, read or
subscribe to thread/turn/item state, and project native Codex activity into
`ActivityItem` rows and `TranscriptBlock` backfill. Prefer generated or captured
schema fixtures over handwritten assumptions. For the first dogfood path, prefer
the default stdio transport and a minimal JSON-RPC subset: initialize, read/list
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

### Phase 3: OpenCode serve read adapter

Add an OpenCode serve source that reads available session/message/event data from
the OpenCode server API and projects it into the same Activity model. Keep this
adapter independent of Codex-specific assumptions; the common contract is the
dashboard Activity source interface, not an ACP-compatible wire protocol.

Verification boundary: fixture projection tests for OpenCode responses/events,
bounded degradation tests for missing auth/unreachable server/version drift, and
route tests matching the same privacy and identity constraints as the Codex
adapter.

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
