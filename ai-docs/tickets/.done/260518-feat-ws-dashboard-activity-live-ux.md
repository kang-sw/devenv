---
title: ws dashboard Activity Console live UX
parent: 260518-epic-ws-dashboard-activity-console
spec:
  - 260521-ws-dashboard-activity-console-live-ux
completed: 2026-05-21
related:
  260518-feat-ws-dashboard-activity-console-ui: provides the static Activity Console shell to make live
  260518-feat-ws-dashboard-activity-watch-stream: supplies backend feed and transcript invalidation events
  260518-feat-ws-dashboard-activity-read-model: supplies snapshot and backfill routes used after stream invalidations
related-mental-model:
  - ws-web-dashboard
---

# ws dashboard Activity Console live UX

## Background

After the Activity Console UI shell exists and the daemon can stream Activity
Feed events, the frontend must adopt that stream without regressing the
polished console experience. This is a separate slice from backend watching
because the review and failure modes are frontend state merging, stale workRoot
protection, selection preservation, and fallback behavior.

## Decisions

- Subscribe to the workRoot Activity stream only while the Activity Console is
  visible or otherwise actively used.
- Merge `itemUpserted` and `itemRemoved` events into the current feed while
  preserving selected item state when the selected item still exists.
- Treat `snapshotInvalidated` as an instruction to refetch the read model rather
  than attempting to reconstruct missing state in the browser.
- Treat `transcriptUpdated` as an instruction to refresh or incrementally
  backfill the selected transcript only when the selected item matches.
- Use bounded polling only when the daemon reports fallback mode or the stream
  is unavailable.
- Preserve the UI shell's browser-local acknowledgement watermark. Streamed or
  polled updates newer than the saved watermark may turn on the ribbon's
  breathing attention cue; selecting or acknowledging the item clears the local
  dirty state without sending a daemon read receipt.

## Constraints

- Ignore events for stale workRoots after the user switches roots.
- Do not expose raw SSE payloads, backend paths, cache paths, or source ids in
  UI state that can leak to the browser surface.
- Do not add agent control affordances while wiring live behavior.
- Keep the static UI shell usable when live updates are unavailable.

## Phases

### Phase 1: Adopt Activity Console live stream in the frontend

Wire the Activity Console state layer to the backend Activity stream. Implement
event merge, selected transcript refresh, snapshot invalidation refetch,
fallback polling transition, stale workRoot event rejection, stream teardown,
and visible-console subscription lifecycle.

Verification should prove that newly registered or called named agents appear
in the ribbon without browser reload, call status transitions update while a
call runs or completes, transcript updates refresh only the selected matching
item, stale root updates are ignored after switching workRoots, fallback mode
uses bounded polling, local dirty/acknowledgement state survives snapshot and
stream merges correctly, the old always-on interval path is removed or limited
to fallback mode, and desktop/constrained-width browser checks still pass.

### Result (82b5c51f) - 2026-05-21

Implemented frontend Activity Console live stream adoption for visible workRoot
Activity panes. The browser subscribes with `EventSource`, consumes named
`event: activity` frames, applies source-neutral item upsert/removal events,
refetches snapshots on invalidation, refreshes selected transcripts only for
matching transcript events, and tears down stale streams after root switches or
pane close.

The previous pane-open recent activity polling path is now fallback-only rather
than the normal live path. Local dirty/acknowledgement state is preserved across
stream and snapshot merges, and selected streamed revisions stay dirty until
explicit acknowledgement. The implementation remains read-only and adds no
agent controls, backend stream changes, exec source support, or transcript
adapter expansion.

Verification passed frontend helper tests, command tests, workbench tests,
build, browser acceptance with real `event: activity` SSE fixture frames, and
partitioned correctness/fit/test review relay.
