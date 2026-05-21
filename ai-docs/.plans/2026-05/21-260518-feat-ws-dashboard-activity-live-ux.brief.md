# Brief: 260518-feat-ws-dashboard-activity-live-ux

## Intent

Adopt the backend Activity Console event stream in the frontend so the WorkRoot
Activity Console updates while visible without relying on always-on full-list
polling. Preserve the existing read-only shell, command-routed controls, local
dirty acknowledgement behavior, and stale-root protection.

## Scope Boundary

Selected scope: `Phase 1: Adopt Activity Console live stream in the frontend`.

In scope:

- EventSource/SSE subscription for the selected workRoot Activity Console while
  the console pane is visible or otherwise actively used.
- Source-neutral event parsing and merge behavior for `itemUpserted`,
  `itemRemoved`, `snapshotInvalidated`, `transcriptUpdated`, `modeChanged`, and
  `heartbeat`.
- Snapshot refetch on invalidation or missed/reset state.
- Selected transcript refresh/backfill only when the selected activity matches
  `transcriptUpdated`.
- Stale workRoot event rejection, stream teardown, fallback polling transition,
  and browser-local acknowledgement/dirty cue preservation.
- Frontend route/helper tests and browser acceptance coverage for live updates.

Out of scope:

- Backend stream changes, native watcher mode, or daemon authority changes.
- Agent controls, terminate/cancel/retry, exec job source implementation, and
  transcript source adapter expansion.
- Mobile layout changes or broad Activity Console redesign.

## Caller-Visible Contract

When a user opens the Activity Console for a workRoot, the browser subscribes to
that workRoot's activity event stream:

```text
GET /api/dashboard/work-roots/{workRootId}/activity/events?after={cursor}
```

Visible behavior:

- Newly upserted activity appears in the ribbon without browser reload.
- Removed activity disappears; selection is preserved when the selected item
  still exists and reconciled when it no longer exists.
- `snapshotInvalidated` triggers a bounded read-model refetch rather than
  browser-side reconstruction.
- `transcriptUpdated` refreshes the selected transcript only when the event's
  `activityId` matches the currently selected item.
- `modeChanged` controls whether bounded fallback polling is active.
- Stream/poll updates newer than the local acknowledgement watermark may turn
  on the ribbon dirty cue; selecting or acknowledging an item clears only local
  UI state and sends no daemon read receipt.
- Events for a stale workRoot after root switch or pane close are ignored.

The feature remains read-only and must not expose raw SSE payloads, backend
paths, cache paths, source ids, host paths, or control actions in UI state.

## Contract Instructions

- Reuse existing Activity Console state and helpers in
  `ws-dashboard/frontend/src/ActivityConsole.tsx` and
  `ws-dashboard/frontend/src/workRootActivity.ts`; do not create a parallel
  activity state model.
- Add typed frontend helpers for Activity Console events, stream endpoints, and
  event application/merge decisions. Keep public names source-neutral.
- Ensure the stream uses opaque `workRootId` only. Do not place host/cache paths
  or backend-native identifiers into frontend state, command payloads, logs, or
  DOM-visible data.
- The old recent-activity polling path may remain only as fallback when the
  stream is unavailable or daemon `modeChanged` reports `pollFallback`. It must
  not stay as the normal always-on live update path while a stream is healthy.
- Keep visible Activity Console controls routed through existing command
  dispatch. Background stream/poll merges are data effects, not user commands.
- Guard asynchronous stream, poll, snapshot, and transcript completions by
  workRoot id, selected activity id, and request identity so stale responses
  cannot overwrite newer root/selection state.
- Teardown EventSource/subscriptions when the Activity Console pane is closed,
  when the workRoot changes, or when the component unmounts.
- Preserve browser-local acknowledgement/dirty state across stream and snapshot
  merges. Do not add daemon read receipts.
- Keep the static UI shell usable when live stream setup fails; surface bounded
  fallback/error state without in-app tutorial text.

## Integration Test Instructions

Required boundary type: frontend route/helper tests plus browser-level
daemon-served UI acceptance.

Extend existing frontend helper tests and browser acceptance tests. Coverage
must prove:

- Event endpoint construction encodes opaque workRoot ids and `after` cursor.
- Activity Console events parse/validate source-neutral payloads.
- `itemUpserted`/`itemRemoved` merge into the feed while preserving or
  reconciling selection.
- `snapshotInvalidated` requests a read-model refetch.
- `transcriptUpdated` refreshes only the selected matching transcript.
- `modeChanged: pollFallback` activates bounded fallback polling and healthy
  stream mode suppresses always-on full-list polling.
- Stale workRoot events and late stream/poll responses are ignored after root
  switch or pane close.
- Local dirty/acknowledgement state survives stream and snapshot merges.
- Browser acceptance shows Activity Console live update behavior without
  reload, constrained ribbon still scrolls, command ids remain present, and
  existing terminal/browser acceptance does not regress.

Run at minimum:

```text
cd ws-dashboard/frontend && npm run test:work-root-activity
cd ws-dashboard/frontend && npm run test:commands
cd ws-dashboard/frontend && npm run test:workbench
cd ws-dashboard/frontend && npm run build
cd ws-dashboard/frontend && npm run test:browser
```

Add any new targeted route/helper test commands to the completion report.

## Implementation Strategy Decisions

- Use browser EventSource/SSE for the normal live stream path.
- Treat backend `pollFallback` mode as fallback transition, not as a reason to
  keep unconditional full-list polling while the stream is healthy.
- Prefer snapshot refetch for invalidation/reset over reconstructing missed
  state in the browser.
- Transcript live behavior in this slice is selected-transcript refresh, not
  block-level incremental append.
- Keep stream event handling source-neutral; named agents are only the first
  feed source.

## Rejected Alternatives

- Keeping the existing always-on recent full-list poll as the normal live mode:
  rejected because the backend stream now provides invalidation/update events.
- Browser-side reconstruction after `snapshotInvalidated`: rejected because the
  daemon explicitly asks for a read-model refetch.
- Adding visible agent controls while wiring live behavior: rejected as outside
  the read-only Activity Console scope.
- Reading wsstate/wsagent paths directly from the browser: rejected because the
  daemon owns all activity source resolution.

## Approach

- Locate current Activity Console fetch/refresh/poll state in `App.tsx`,
  `ActivityConsole.tsx`, and `workRootActivity.ts`.
- Add frontend event types/helpers and pure merge functions first, with tests.
- Wire Activity Console lifecycle to EventSource creation/teardown for the
  selected workRoot and visible pane.
- Convert old recent-activity polling to a fallback path that is disabled while
  the stream is healthy.
- Add browser acceptance fixtures or route interception proving stream events
  update the ribbon/transcript without reload.

## Constraints

- Do not change backend stream semantics in this ticket.
- Do not regress command dispatch identities for Activity Console controls.
- Keep all text and UI elements fitting in existing desktop/constrained layouts.
- Avoid raw light palette values or unrelated visual redesign.
- Browser-facing logs/DOM/test fixtures must not include private host/cache
  paths beyond existing accepted test fixture paths.

## Out of scope

- Backend watcher or SSE route implementation.
- Native watcher mode.
- Agent control actions.
- Exec job source support.
- Transcript source adapter expansion.
- Mobile layout work.

## Details

Backend event categories expected by the frontend:

```ts
type ActivityConsoleEvent =
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

Use local TypeScript names that match existing frontend conventions.

## Verification Contract

Implementation is complete only when:

- Helper tests prove event merge/stale-response decisions.
- Browser acceptance proves visible live Activity Console updates without
  reload and no regression of existing dashboard browser flows.
- `npm run build` passes.
- Review relay is clean across correctness, fit, and test partitions.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md`:
  `{#260521-ws-dashboard-activity-console-live-ux}`,
  `{#260521-ws-dashboard-activity-console-ui-shell}`,
  `{#260521-ws-dashboard-activity-console-watch-stream}`,
  `{#260521-ws-dashboard-activity-console-read-model}`,
  `{#260517-ws-dashboard-workroot-activity-pane}`,
  `{#260516-ws-web-dashboard-inspectable-navigation-shell}`,
  `{#260516-ws-web-dashboard-browser-ui-acceptance-gate}`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - ActivityConsole,
  workRootActivity helpers, command dispatch, stream JSON, stale async guards,
  and browser verification rules.
- [Must] `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-live-ux.md`
  - selected phase scope and verification checklist.
- [Must] `ai-docs/tickets/todo/260518-epic-ws-dashboard-activity-console.md`
  - cross-child decisions and read-only/source-neutral boundaries.
- [Maybe] `ai-docs/tickets/todo/260518-feat-ws-dashboard-activity-transcript-api.md`
  - future transcript source expansion boundary.
- [Maybe] `ai-docs/tickets/todo/260513-feat-async-exec-output-reader.md`
  - future exec source; do not implement now.
