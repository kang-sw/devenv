---
title: ws dashboard Activity Console transcript expansion
parent: 260518-epic-ws-dashboard-activity-console
spec:
  - 260522-ws-dashboard-activity-console-transcript-expansion
related:
  260518-feat-ws-dashboard-activity-read-model: supplies the MVP transcript backfill and resolver boundary
  260518-feat-ws-dashboard-activity-watch-stream: supplies transcript invalidation events
  260518-feat-ws-dashboard-activity-live-ux: consumes transcript update behavior in the frontend
  260513-feat-async-exec-output-reader: future exec output should become a transcript source after that runtime exists
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard Activity Console transcript expansion

## Background

The Activity Console read model starts with ws named-agent transcript backfill
from stable ws-owned runtime files. Full-scale console polish also needs deeper
transcript sources and more precise live transcript behavior, but those should
land after the read model, UI shell, backend stream, and frontend live adoption
establish the core console foundation.

Backend harnesses such as Codex, Claude, and Gemini may have their own native
session transcript files. Those formats must be resolved behind daemon-owned
adapters and normalized into `TranscriptBlock` values without making the
browser understand cache layout or backend-native file formats.

## Decisions

- Add backend-native session resolvers behind the Activity Transcript source
  abstraction rather than hardcoding browser-visible paths.
- Start with Codex native session JSONL if the implementation can verify a
  stable fixture format without requiring a live Codex invocation.
- Keep Claude and Gemini native transcript handling deferred unless their
  stable file formats are documented or fixture-backed during implementation.
- Preserve raw backend JSON or markdown as adapter input, not as the public
  browser contract.
- Block-level transcript append may be introduced when it improves visible live
  transcript behavior beyond feed-level `transcriptUpdated` invalidations.

## Constraints

- The API must not expose backend session paths, cache paths, host paths, pids,
  session ids, stdout/stderr paths, stream paths, or native transcript paths.
- Malformed backend transcript lines should degrade individual blocks or source
  status without breaking the entire selected activity detail when possible.
- The resolver interface must not assume Codex-only semantics.
- Exec transcript source integration remains blocked on
  `260513-feat-async-exec-output-reader`.

## API Sketch

Optional selected activity transcript live append:

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

The implementation may continue using feed-level `transcriptUpdated` events and
bounded selected backfill if block-level append would add complexity without a
clear UX win.

## Phases

### Phase 1: Expand Activity Console transcript sources and live transcript behavior

Add verified native transcript resolver support behind the existing
`ActivityTranscript` and `TranscriptBlock` contracts, starting with Codex
session fixtures when stable. Add block-level live append only if it materially
improves the Activity Console transcript UX over feed-level invalidation plus
bounded backfill.

Verification should cover fixture-backed native transcript parsing, missing or
unreadable session files, malformed event lines, degraded source status,
private-field redaction, optional block-level append cursor behavior, and no
requirement for a live backend invocation.
