---
title: ws dashboard Activity Console watch stream
parent: 260518-epic-ws-dashboard-activity-console
spec:
  - 260521-ws-dashboard-activity-console-watch-stream
completed: 2026-05-21
related:
  260518-feat-ws-dashboard-activity-read-model: supplies feed item and transcript cursor contracts consumed by stream events
  260518-feat-ws-dashboard-activity-live-ux: consumes this backend stream in the frontend console
  260517-feat-ws-dashboard-workroot-activity-live-refresh: absorbed narrow SSE/filewatch follow-up
  260524-epic-async-exec-job-surface: future exec activity may share feed invalidation and fallback behavior
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard Activity Console watch stream

## Background

The current Activity pane uses a bounded polling hotfix while open: it fetches
recently modified named-agent rows and merges them into the initial full
projection. That unblocks dogfood but is not the durable dashboard model. The
Activity Console needs a workRoot-scoped watcher and read-only backend event
stream so the frontend can update the ribbon and selected transcript without
browser reloads or repeated full-list polling.

## Decisions

- Prefer SSE for read-only feed events. Use WebSocket only if implementation
  records a concrete bidirectional need.
- Watch only selected or otherwise visible workRoots. Do not eagerly watch
  every remembered or opened root.
- Keep a bounded polling fallback for platforms or filesystems where watcher
  behavior is unavailable, lossy, or difficult to prove.
- Stream item invalidations or row updates, not raw file paths or
  backend-native event payloads.

## Constraints

- File watching must be cross-platform and handle atomic renames, nested
  `current/` files, missing directories, agent erasure, and recreated agent
  directories on macOS, Linux, and Windows.
- Watch events must be coalesced or debounced enough to avoid repeatedly
  rebuilding a monotonically growing full feed for a single backend write.
- Stream events must remain owner-authenticated and must not begin before
  route auth succeeds.
- Frontend event consumption and merge behavior belongs to
  `260518-feat-ws-dashboard-activity-live-ux`.

## API Sketch

The feed stream should be a read-only workRoot-scoped endpoint:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}
```

SSE is the preferred transport unless implementation finds a concrete
bidirectional need. Candidate event payloads:

```ts
type ActivityFeedEvent =
  | {
      type: "itemUpserted";
      cursor: string;
      item: ActivityItem;
    }
  | {
      type: "itemRemoved";
      cursor: string;
      activityId: string;
    }
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
  | {
      type: "heartbeat";
      cursor: string;
    };
```

The event stream may intentionally ask the browser to refetch the snapshot when
events were missed or coalesced. It must not stream raw filesystem paths,
backend-native transcript records, or cache file contents.

## Phases

### Phase 1: Implement backend Activity Console watch and SSE stream

Introduce the daemon-side watcher abstraction, fallback mode, workRoot-scoped
authenticated SSE endpoint, event cursor handling, heartbeat behavior, and
event coalescing needed for Activity Feed and transcript invalidations.

Verification should cover watch-target selection, unavailable directory
fallback, deletion/recreation handling, platform-independent event
normalization, unauthenticated rejection, selected workRoot subscription,
heartbeat/fallback behavior, agent file change invalidation, reconnect after a
missed cursor, and private-field redaction. Native Windows evidence may be
recorded as a later implementation note when the watcher crate or OS behavior
requires host dogfood.

### Result (2acdf8a3) - 2026-05-21

Implemented the backend Activity Console event stream at
`GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}`.
The route is owner-authenticated before stream acceptance and emits
source-neutral SSE `activity` frames for mode changes, snapshot invalidation,
item upsert/removal, transcript updates, and heartbeat events.

The completed backend stream uses bounded polling fallback mode and announces
`pollFallback`; no native filesystem watcher dependency was added in this
slice. Tests cover missing-agent-directory fallback, subscribed workRoot
scoping against sibling workRoot cache mutations, reconnect cursor invalidation,
private-field redaction, unknown workRoot handling, auth rejection, and SSE
frame metadata.

Deferred scope remains frontend live UX consumption, native watcher mode,
agent controls, exec job source support, and expanded transcript source
adapters.
