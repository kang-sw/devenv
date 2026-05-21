# Brief: 260518-feat-ws-dashboard-activity-watch-stream

## Intent

Implement the backend Activity Console watch stream for opened workRoots. The
daemon should expose a read-only, owner-authenticated, workRoot-scoped event
stream that reports Activity Feed and transcript invalidations without forcing
the frontend to poll the full activity snapshot repeatedly.

## Scope Boundary

Selected scope: `Phase 1: Implement backend Activity Console watch and SSE
stream`.

In scope:

- Backend watcher abstraction for WorkRoot Activity sources.
- Owner-authenticated SSE endpoint:
  `GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}`.
- Event cursor handling, heartbeat behavior, event coalescing/debounce, and
  polling fallback mode when filesystem watching is unavailable or unreliable.
- Source-neutral event payloads for feed item changes, transcript invalidation,
  snapshot invalidation, mode changes, and heartbeat.
- Backend tests proving auth, cursor/fallback, event normalization, and private
  field redaction.

Out of scope:

- Frontend subscription, merge, stale-root rejection, and dirty cue behavior;
  that belongs to `260518-feat-ws-dashboard-activity-live-ux`.
- Agent control actions, exec job implementation, new transcript source
  adapters, and mobile UI behavior.
- Bidirectional WebSocket semantics unless implementation records a concrete
  need and escalates before widening the contract.

## Caller-Visible Contract

Authenticated callers can subscribe to a workRoot activity event stream by
opaque `workRootId` and optional `after` cursor. The stream emits bounded,
source-neutral Activity Console events that allow a browser to update or
invalidate its Activity Feed/transcript view:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}
```

Event categories must cover:

- `itemUpserted` with cursor and public `ActivityItem`.
- `itemRemoved` with cursor and activity id.
- `transcriptUpdated` with cursor, activity id, and transcript cursor metadata.
- `snapshotInvalidated` with cursor and bounded reason such as overflow,
  watch reset, or fallback.
- `modeChanged` with cursor and update mode such as watch, polling fallback, or
  snapshot.
- `heartbeat` with cursor.

The route must reject unauthenticated callers before accepting the stream. It
must never expose host paths, cache paths, backend-native transcript records,
session ids, process ids, stdout/stderr paths, stream paths, raw filesystem
events, or file contents.

## Contract Instructions

- Extend the existing WorkRoot Activity backend instead of creating a parallel
  activity authority. Reuse the Activity Feed and transcript public shapes from
  `ws-dashboard/crates/core/src/activity.rs`,
  `ws-dashboard/crates/daemon/src/work_root_activity.rs`, and
  `ws-dashboard/frontend/src/workRootActivity.ts` where applicable.
- Add the stream route inside the existing protected dashboard router so owner
  auth, bearer exceptions, Host/Origin checks, and unknown-workRoot handling
  follow current daemon route patterns.
- Prefer SSE for the new route. Do not use WebSocket unless the implementation
  escalates a concrete bidirectional requirement.
- Watch only selected or otherwise subscribed workRoots. Do not start global
  watchers for every remembered or opened root.
- Normalize file-watch signals into Activity Console events. The event stream
  must not pass through raw watcher events, filesystem paths, backend cache
  paths, or backend-native named-agent payloads.
- Handle atomic renames, nested `current/` file changes, missing directories,
  agent erasure, and recreated agent directories by producing coalesced
  invalidation or public row-update events.
- Provide bounded polling fallback when the watcher cannot be started or when a
  missed/overflow cursor makes event replay unreliable.
- Preserve source-neutral vocabulary. Named agents are the first event source,
  but new logic should not make named-agent internals part of the public stream
  contract.
- Keep the stream read-only. It must not add agent start, interrupt, cancel,
  erase, retry, exec launch, or terminal-control behavior.

## Integration Test Instructions

Required boundary type: daemon HTTP/SSE route, Activity Console event contract,
and watcher/fallback lifecycle.

Extend existing Rust daemon/core tests where possible, and add route-level tests
for the new stream endpoint. Coverage must prove:

- Unauthenticated requests are rejected before stream acceptance.
- Unknown or unopened workRoot ids return a bounded public error.
- A selected/subscribed workRoot can receive heartbeat and mode events.
- Named-agent file/state/output changes emit source-neutral item or transcript
  invalidations without private paths or backend-native records.
- Agent deletion and recreated agent directories produce item removal/upsert or
  snapshot invalidation as appropriate.
- Missed/overflow/reset conditions produce a snapshot invalidation and/or
  fallback mode rather than unbounded replay.
- Cursor handling accepts a last-observed cursor and keeps reconnection finite.
- Platform-independent tests do not require native Windows evidence; if native
  watcher behavior needs host dogfood, record that as a ticket Result note, not
  as a hidden untested assumption.

Run the relevant Rust verification commands. At minimum include:

```text
cargo test -p ws-dashboard-core activity
cargo test -p ws-dashboard-daemon work_root_activity
```

Add any new daemon stream/watch test filters to the completion report.

## Implementation Strategy Decisions

- SSE is the default transport for this read-only backend stream.
- The backend may emit invalidation events instead of reconstructing every
  browser merge state; frontend merge behavior is a later ticket.
- Event cursors are for bounded reconnect and replay decisions, not a durable
  long-term audit log.
- Polling fallback is accepted when watcher behavior is unavailable,
  overflowed, or cannot be proven for a platform in this slice.
- Existing instance event stream scaffolding is useful precedent, but the
  public vocabulary must be Activity Feed/Activity Console, not the older
  instance-events scaffold product name.

## Rejected Alternatives

- Always-on browser full-list polling: rejected as the durable model because it
  rebuilds a growing feed repeatedly.
- Watching every remembered or opened workRoot eagerly: rejected because the
  epic scopes watchers to selected or otherwise visible workRoots.
- Streaming raw filesystem events or backend cache records: rejected because the
  browser must consume daemon-owned public Activity Console semantics only.
- Adding frontend live stream consumption in this slice: rejected because
  `260518-feat-ws-dashboard-activity-live-ux` owns merge, stale-root, and UI
  behavior.

## Approach

- Locate the current activity read model route, route tests, and instance event
  stream scaffold.
- Define public Activity Console event types in the core or daemon boundary that
  matches existing serialization style.
- Add a daemon stream endpoint under the protected router with bounded cursor
  parsing and SSE response formatting.
- Build a small watcher/fallback service that can coalesce source changes into
  public invalidation events for subscribed workRoots.
- Add tests before or alongside implementation for auth, redaction, fallback,
  cursor, and named-agent invalidation behavior.

## Constraints

- Keep all route identity by opaque `workRootId`.
- Keep cache/session/process/path details daemon-private.
- Avoid blocking Axum async workers while scanning Git roots, wsstate, agent
  cache state, or transcript files.
- Do not regress the existing Activity Feed snapshot, transcript endpoint,
  compatibility `agents` projection, top-bar badge, or route helper tests.
- Keep cross-platform watcher behavior explicit; do not assert native Windows
  support without evidence.

## Out of scope

- Frontend EventSource or stream merge code.
- Activity Console visual changes.
- Agent control actions.
- Exec job source implementation.
- Native Codex transcript adapter expansion.

## Details

The endpoint shape is:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}
```

Candidate JSON event payloads may follow the ticket sketch:

```ts
type ActivityFeedEvent =
  | { type: "itemUpserted"; cursor: string; item: ActivityItem }
  | { type: "itemRemoved"; cursor: string; activityId: string }
  | {
      type: "transcriptUpdated";
      cursor: string;
      activityId: string;
      transcriptCursor: string | null;
    }
  | {
      type: "snapshotInvalidated";
      cursor: string;
      reason: "overflow" | "watchReset" | "fallback" | string;
    }
  | {
      type: "modeChanged";
      cursor: string;
      updateMode: "watch" | "pollFallback" | "snapshot";
    }
  | { type: "heartbeat"; cursor: string };
```

Adjust Rust naming and serde casing to match existing dashboard conventions,
but keep the observable contract equivalent.

## Verification Contract

Implementation is complete only when:

- The new route and event types are covered by Rust tests.
- Existing Activity read model tests still pass.
- Auth rejection and private-field redaction are explicitly tested.
- Fallback/reset/cursor behavior is covered by deterministic tests.
- Completion report lists commands run and any platform watcher evidence gap.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md`:
  `{#260521-ws-dashboard-activity-console-watch-stream}`,
  `{#260521-ws-dashboard-activity-console-read-model}`,
  `{#260517-ws-dashboard-workroot-activity-projection}`,
  `{#260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold}`,
  `{#260516-ws-web-dashboard-instance-event-envelope-fixtures}`,
  `{#260515-ws-web-daemon-foundation}`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard activity,
  auth route, stream scaffold, source-neutral feed, command/privacy, and test
  coupling rules.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - wsstate/wsagent cache
  layout and named-agent output/state lifecycle.
- [Must] `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-watch-stream.md`
  - selected phase scope and verification checklist.
- [Must] `ai-docs/tickets/todo/260518-epic-ws-dashboard-activity-console.md`
  - cross-child decisions and non-scope.
- [Maybe] `ai-docs/tickets/todo/260518-feat-ws-dashboard-activity-live-ux.md`
  - frontend consumer boundary only.
- [Maybe] `ai-docs/tickets/todo/260518-feat-ws-dashboard-activity-transcript-api.md`
  - future transcript source expansion boundary.
- [Maybe] `ai-docs/tickets/todo/260513-feat-async-exec-output-reader.md`
  - future exec source; do not implement in this slice.
