---
title: ws dashboard Activity Console read model
parent: 260518-epic-ws-dashboard-activity-console
spec:
  - 260521-ws-dashboard-activity-console-read-model
related:
  260517-feat-ws-dashboard-workroot-activity: source projection to generalize from named agents to feed items
  260518-feat-ws-dashboard-activity-feed-api: absorbed feed-only ticket scope into this read-model slice
  260518-feat-ws-dashboard-activity-console-ui: consumes this read model for the static console shell
  260513-feat-async-exec-output-reader: future exec jobs should fit the feed item and transcript source model after that runtime exists
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
completed: 2026-05-21
---

# ws dashboard Activity Console read model

## Background

The current WorkRoot Activity endpoint returns a named-agent list sorted by
agent id and only reports whether recent output exists. The full-scale Activity
Console needs one backend read model that supplies both the live/latest
Activity Feed snapshot and the selected activity transcript backfill used by
the UI shell.

This ticket absorbs the feed-only API ticket because feed items and selected
transcript backfill share source resolution, redaction, degraded-state handling,
and UI contract tests.

## Decisions

- Keep the existing route family workRoot-scoped. The current
  `/api/dashboard/work-roots/{workRootId}/activity` route may evolve into the
  feed snapshot route, but compatibility and tests must make the migration
  explicit.
- Name the public concepts `ActivityFeed`, `ActivityItem`,
  `ActivityTranscript`, and `TranscriptBlock`.
- Represent named agents as the first `ActivityItem` and transcript source kind.
  Do not build a named-agent-only shape that later sources cannot use.
- Feed items carry enough state for ribbon rendering without requiring a
  transcript fetch: id, kind, label, status, live flag, attention flag,
  timestamps, source display metadata, and transcript availability.
- Selected transcript backfill returns normalized blocks for rendering, not raw
  backend JSON, markdown, stdout/stderr paths, or cache file contents.

## Constraints

- The browser must never receive cache paths, host paths, session ids, process
  ids, stdout/stderr paths, stream paths, or backend-native transcript paths.
- Ordering must favor active/live/attention items and then latest updated
  activity. Alphabetical ordering is only a stable tie-breaker.
- The read model should preserve bounded degraded states for malformed or
  unavailable agent records instead of failing the whole feed.
- Transcript output must be bounded by cursor, block count, byte count, or a
  combination of those controls.
- Codex, Claude, Gemini, and exec native transcript expansion is out of scope
  except for reserving source kinds and resolver boundaries.

## API Sketch

Activity Feed snapshot:

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

Selected activity transcript backfill:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/items/{activityId}/transcript?after={cursor}&limit={n}
```

Candidate response shape:

```ts
type ActivityTranscript = {
  workRootId: string;
  activityId: string;
  status: "live" | "complete" | "unavailable" | "degraded" | string;
  sourceStatus: "ok" | "missing" | "unsupported" | "degraded" | string;
  blocks: TranscriptBlock[];
  nextCursor: string | null;
  hasMore: boolean;
  live: boolean;
};

type TranscriptBlock = {
  blockId: string;
  cursor: string;
  timestamp: string | null;
  kind:
    | "user"
    | "assistant"
    | "toolCall"
    | "toolResult"
    | "status"
    | "error"
    | "output"
    | string;
  title: string | null;
  text: string | null;
  data: Record<string, unknown> | null;
  degraded: boolean;
};
```

The exact field names may change during spec work, but the settled contract is
that feed snapshots expose selectable activity items, transcript backfill
returns normalized blocks, and neither endpoint leaks daemon source paths or
backend-native session details.

## Phases

### Phase 1: Implement the backend Activity Console read model

Add the backend and frontend types, route helpers, named-agent projection,
ordering rules, selected transcript backfill, cursor bounds, and degraded-state
handling needed by the static Activity Console UI shell.

Verification should cover camelCase serialization, private-field redaction,
ordering rules for live/failed/blocked/recent/idle items, primary and linked
Git workRoots, malformed records, unknown activity ids, empty/unavailable
transcripts, bounded backfill, owner-auth rejection, and a mixed ordering case
that would have rendered poorly under A-Z sorting. The implementation should
also preserve the current WorkRoot Activity route compatibility or record any
explicit response migration in tests and docs.

### Result (b48d54f9) - 2026-05-21

Phase 1 implemented the Activity Console read model as source-neutral Activity
Feed and Activity Transcript contracts while preserving the existing
named-agent `agents` projection for the current WorkRoot Activity pane. The
workRoot Activity route now returns selectable Activity Items with feed cursor,
selected-item hint, update mode, source metadata, transcript availability, and
ordering suitable for the later ribbon. A selected activity transcript route
returns bounded normalized Transcript Blocks for named-agent output with
explicit source status, cursor pagination, and unavailable/degraded states.

The implementation kept Activity Console UI, live stream behavior, native
Codex/Claude/Gemini transcript resolvers, exec jobs, and agent controls out of
scope. Verification passed `cargo test -p ws-dashboard-core activity`,
`cargo test -p ws-dashboard-daemon work_root_activity`,
`npm run test:work-root-activity`, `npm run build`, and the broader
`cargo test`; the frontend production build kept the existing large-chunk
warning.
