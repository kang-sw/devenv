---
title: ws dashboard Activity Transcript API
parent: 260518-epic-ws-dashboard-activity-console
related:
  260518-feat-ws-dashboard-activity-feed-api: feed items select transcript sources
  260518-feat-ws-dashboard-activity-watch-stream: feed stream announces transcript updates
  260513-feat-async-exec-output-reader: future exec output should become a transcript source after that runtime exists
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard Activity Transcript API

## Background

The current WorkRoot Activity projection only reports that recent output exists.
The desired Activity Console selects a ribbon item and renders its transcript as
structured blocks. Backend harnesses such as Codex, Claude, and Gemini may have
their own native session transcript files, while ws named-agent state also has
stable files such as `events.jsonl`, `current/stdout`, `current/stderr`, and
`output.md`.

The dashboard needs a daemon-owned transcript API that can backfill and live
append normalized blocks without making the browser understand cache layout or
backend-native file formats.

## Decisions

- Treat transcript rendering as selected-activity behavior, not as a generic
  file tail. Use `ActivityTranscript` and `TranscriptBlock` vocabulary.
- Start from ws named-agent runtime files because they are the stable ws-owned
  surface. Add backend-native session resolvers behind a daemon abstraction
  rather than hardcoding browser-visible paths.
- Normalize transcript blocks for rendering: user, assistant, tool call, tool
  result, status, error, and output are acceptable starting categories.
- Keep raw backend JSON or markdown as adapter input, not as the public
  browser contract.

## Constraints

- The API must not expose backend session paths, cache paths, host paths, pids,
  session ids, stdout/stderr paths, or stream paths.
- Transcript output must be bounded by cursor, block count, byte count, or a
  combination of those controls.
- Malformed backend transcript lines should degrade individual blocks or source
  status without breaking the entire selected activity detail when possible.
- Claude/Gemini native transcript handling may be deferred if their stable file
  formats are not yet documented, but the resolver interface must not assume
  Codex-only semantics.

## API Sketch

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

Selected activity transcript live append:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/items/{activityId}/transcript/events?after={cursor}
```

Candidate SSE payloads:

```ts
type ActivityTranscriptEvent =
  | { type: "blockAppended"; cursor: string; block: TranscriptBlock }
  | { type: "blockUpdated"; cursor: string; block: TranscriptBlock }
  | { type: "statusChanged"; cursor: string; status: ActivityTranscript["status"] }
  | { type: "snapshotInvalidated"; cursor: string; reason: string }
  | { type: "end"; cursor: string };
```

The implementation may first ship `transcriptUpdated` feed events plus bounded
backfill polling for the selected item, but the durable UX target is block-level
live append.

## Phases

### Phase 1: Define transcript source and block contracts

Add backend-neutral `ActivityTranscript`, `TranscriptBlock`, cursor, and source
status contracts. Define how an `ActivityItem` points at a transcript source
without exposing internal paths.

Verification should cover serialization, cursor behavior, block bounds,
private-field redaction, and empty/unavailable transcript states.

### Phase 2: Implement ws named-agent transcript sources

Resolve named-agent items to ws-owned files: lifecycle `events.jsonl`,
`output.md`, and current-call stdout/stderr/runtime logs where appropriate.
Return normalized blocks suitable for the Activity Console instead of raw
diagnostic text.

Verification should cover completed output, running stdout/stderr growth,
lifecycle events, missing files, malformed event lines, and bounded backfill.

### Phase 3: Add Codex native session transcript resolver

When a Codex-backed named agent has a known thread id, locate the corresponding
Codex session JSONL and normalize useful event blocks such as task start,
assistant message, tool call, and tool result. Keep this resolver optional and
degraded when the Codex session file is missing or not readable.

Verification should use fixture JSONL for the documented Codex session format
and must not require a live Codex invocation.

### Phase 4: Add selected transcript backfill and stream endpoints

Expose selected activity transcript backfill and live append endpoints under
the workRoot Activity API. The live path may stream block events directly or
cooperate with the feed stream by emitting transcript-update notifications that
trigger bounded backfill.

Verification should cover auth rejection, unknown activity ids, cursor
reconnect, running transcript append, completed transcript end status, and
private-field redaction.
