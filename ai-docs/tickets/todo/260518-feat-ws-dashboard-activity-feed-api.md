---
title: ws dashboard Activity Feed API
parent: 260518-epic-ws-dashboard-activity-console
related:
  260517-feat-ws-dashboard-workroot-activity: source projection to generalize from named agents to feed items
  260513-feat-async-exec-output-reader: future exec jobs should fit the feed item model after that runtime exists
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard Activity Feed API

## Background

The current WorkRoot Activity endpoint returns a named-agent list sorted by
agent id and the frontend renders it as a dense card dump. The Activity Console
needs a daemon-owned Activity Feed snapshot that represents live/latest
selectable activity items, starting with named agents and leaving room for
future main agent sessions, exec jobs, diagnostics, and other readable
activity.

## Decisions

- Keep the existing route family workRoot-scoped. The current
  `/api/dashboard/work-roots/{workRootId}/activity` route may evolve into the
  feed snapshot route, but browser compatibility and tests must make the
  migration explicit.
- Name the public concept `ActivityFeed`, not `ActivityRoom` or
  `InstanceEvents`.
- Represent named agents as the first `ActivityItem` kind. Do not build a
  named-agent-only API shape that later sources cannot use.
- Feed items should carry enough state for ribbon rendering without requiring a
  transcript fetch: id, kind, label, status, live flag, attention flag,
  timestamps, source metadata, and transcript availability.

## Constraints

- The browser must never receive cache paths, host paths, session ids, process
  ids, stdout/stderr paths, or backend-native transcript paths.
- Ordering must favor active/live/attention items and then latest updated
  activity. Alphabetical ordering is only a stable tie-breaker.
- The API should preserve bounded degraded states for malformed or unavailable
  agent records instead of failing the whole feed.

## API Sketch

The existing WorkRoot Activity route should become the snapshot entrypoint:

```text
GET /api/dashboard/work-roots/{workRootId}/activity
```

Candidate response shape:

```ts
type ActivityFeedSnapshot = {
  workRootId: string;
  status: "ok" | "degraded" | "unavailable" | string;
  summary: ActivityFeedSummary;
  items: ActivityItem[];
  selectedItemId: string | null;
  cursor: string | null;
  updateMode: "watch" | "pollFallback" | "snapshot";
};

type ActivityItem = {
  activityId: string;
  kind: "namedAgent" | "mainAgent" | "exec" | "diagnostic" | string;
  label: string;
  status: "running" | "idle" | "blocked" | "failed" | "completed" | "unavailable" | string;
  live: boolean;
  attention: boolean;
  updatedAt: string | null;
  startedAt: string | null;
  finishedAt: string | null;
  source: {
    type: "wsNamedAgent" | "codexSession" | "claudeSession" | "geminiSession" | "execJob" | string;
    display: string | null;
  };
  transcript: {
    available: boolean;
    cursor: string | null;
    status: "live" | "complete" | "unavailable" | string;
  };
  metadata: Record<string, string | number | boolean | null>;
  diagnostics: string[];
};
```

The exact field names may change during spec work, but the child ticket must
preserve the settled contract: feed snapshots expose selectable activity items,
not named-agent cards, and carry enough state for the ribbon without a
transcript fetch.

## Phases

### Phase 1: Define the feed snapshot contract

Add the core and frontend types for a workRoot Activity Feed snapshot. Capture
summary counts, selected-item hints, update mode, and item ordering semantics.
The shape should be compatible with the existing named-agent projection while
making `items[]` the canonical consumer model.

Verification should cover camelCase serialization, private-field redaction, and
ordering rules for live, failed, blocked, recently completed, and old idle
items.

### Phase 2: Project named agents into activity items

Map the existing named-agent wsstate/wsagent scan into Activity Feed items.
Keep current call status, backend/model metadata, session presence, diagnostics,
and transcript availability as bounded item metadata. Derive stable item ids
from daemon-owned source identity rather than display names.

Verification should cover primary and linked Git workRoots, malformed records,
blocked/failed/unavailable statuses, and a mixed ordering case that would have
rendered poorly under A-Z sorting.

### Phase 3: Migrate route and frontend helper consumption

Update the route and frontend helper layer so Activity Console work consumes
the feed snapshot contract. Preserve or deliberately replace the current
`agents[]` projection with tests that prevent accidental cache-path exposure and
stale prior-root rendering.

This phase should not implement streaming or transcript bodies; those belong to
the watcher/stream and transcript API children.
