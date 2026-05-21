# Survey: 260518-feat-ws-dashboard-activity-live-ux

## Reusable Components
- `ws-dashboard/crates/core/src/activity.rs#L27-L74` — backend `ActivityConsoleEvent` JSON contract: source-neutral event union and camelCase `pollFallback`/`snapshotInvalidated` values the frontend should mirror.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L214-L238` — Activity events route: existing protected SSE endpoint shape and `after` query consumed by the browser stream helper.
- `ws-dashboard/frontend/src/workRootActivity.ts#L23-L105` — frontend activity shapes: existing `ActivityItem`, transcript availability, transcript, and `WorkRootActivityView` types already mirror the daemon read model.
- `ws-dashboard/frontend/src/workRootActivity.ts#L116-L178` — route helpers/fetchers: existing encoded workRoot/activity endpoint construction and JSON error handling patterns.
- `ws-dashboard/frontend/src/workRootActivity.ts#L180-L213` — `mergeWorkRootActivityViews`: current recent-poll merge helper for snapshot-like updates; useful precedent but not sufficient for item removal/event application.
- `ws-dashboard/frontend/src/workRootActivity.ts#L229-L305` — item ordering, selection preservation, dirty revision, and acknowledgement helpers used by the Activity Console ribbon.
- `ws-dashboard/frontend/src/workRootActivity.ts#L307-L328` — transcript stale-response guards: existing workRoot/activity/request tuple check for late transcript responses.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L60-L116` — Activity Console local state reset boundary for workRoot changes, dirty state, expanded details, and transcript state.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L150-L285` — selected transcript load/append/replace flow with request identity guard and selected-revision effect.
- `ws-dashboard/frontend/src/App.tsx#L1086-L1111` — selected-root activity state and per-workRoot Activity pane open flag; current stream lifecycle can key off this visibility state.
- `ws-dashboard/frontend/src/App.tsx#L1206-L1297` — current initial fetch plus recent-activity polling hotfix, including stale root guard on state update.
- `ws-dashboard/frontend/src/App.tsx#L1733-L1779` — open/close Activity pane lifecycle that should correspond to stream subscription and teardown.

## Existing Patterns
- Command-routed controls: see `ws-dashboard/frontend/src/commands.ts#L1-L38` and `ws-dashboard/frontend/src/ActivityConsole.tsx#L287-L322` — visible Activity Console actions already build stable commands and dispatch handlers.
- Browser-local dirty cue: see `ws-dashboard/frontend/src/ActivityConsole.tsx#L79-L88` and `ws-dashboard/frontend/src/ActivityConsole.tsx#L364-L396` — dirty items are a computed local set rendered as `data-dirty` on ribbon buttons.
- Initial and stale root fetch guard: see `ws-dashboard/frontend/src/App.tsx#L1209-L1233` and `ws-dashboard/frontend/src/App.tsx#L1259-L1273` — current async completions are rejected by cancellation/root id before applying state.
- Workbench pane visibility model: see `ws-dashboard/frontend/src/App.tsx#L2231-L2236` — the Activity pane is only included when the selected workRoot's `activityPaneOpenByRoot` flag is true.
- Frontend route/helper tests: see `ws-dashboard/frontend/src/workRootActivity.test.ts#L53-L70`, `ws-dashboard/frontend/src/workRootActivity.test.ts#L263-L329`, and `ws-dashboard/frontend/src/workRootActivity.test.ts#L494-L545` — endpoint encoding, merge behavior, dirty acknowledgements, and stale transcript guards are already covered with pure TypeScript tests.
- Browser acceptance fixtures: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L560-L721` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L722-L816` — current Activity Console browser test intercepts activity snapshot and transcript routes with source-neutral fixture data.
- Browser UI assertions: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L830-L929` — existing gate checks ribbon count/scrolling, command ids, dirty acknowledgement, transcript expansion/load-more, duplicate focus, and close behavior.

## Relevant Interfaces
- `ws-dashboard/frontend/src/workRootActivity.ts#L93-L105` — `WorkRootActivityView`: state container to update for feed cursor, update mode, selected hint, summary, items, and compatibility agents.
- `ws-dashboard/frontend/src/workRootActivity.ts#L260-L274` — selection helpers: default and preserve logic for reconciling selected item after `itemRemoved` or snapshot refetch.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L137-L148` — selected item and selected revision are currently private to `ActivityConsole`, not visible to `App` stream effects.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L269-L285` — transcript refresh is currently triggered by selected item/revision changes, not by an externally supplied `transcriptUpdated` event.
- `ws-dashboard/frontend/src/App.tsx#L101-L107` — App imports only fetch/merge/badge activity helpers today; stream helpers can live in `workRootActivity.ts` if route-test compilation includes them.
- `ws-dashboard/frontend/tsconfig.route-tests.json#L1-L28` — route test compilation has an explicit include list; any new helper file outside current includes needs this config updated.
- `ws-dashboard/frontend/package.json#L6-L18` — verification scripts required by the brief already exist, including `test:work-root-activity`, `test:commands`, `test:workbench`, `build`, and `test:browser`.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L560-L816` — route interception boundary for Activity Console fixtures; stream tests need an additional `/activity/events` fixture path.

## Constraints
- `ws-dashboard/frontend/src/App.tsx#L1235-L1297` — the current recent-poll effect runs whenever the pane is open; it must become conditional fallback rather than normal healthy stream behavior.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L106-L116` — acknowledgement and transcript state reset on `workRootId`; stream snapshot merges for the same root should not accidentally wipe local dirty state.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L125-L135` — `seenRevisions` only initializes unseen items; existing seen items whose revision changes become dirty via helper comparison.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L269-L285` — current selected-revision effect acknowledges selected item and reloads transcript automatically; this interacts with streamed updates to the selected item.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L327-L359` — Activity Console has no prop for stream status, fallback status, or externally requested transcript refresh today.
- `ws-dashboard/frontend/src/App.tsx#L1733-L1779` — Activity pane close only flips local pane state and removes pane order; stream teardown must follow this state rather than requiring a daemon action.
- `ws-dashboard/frontend/playwright.config.ts#L1-L20` — browser acceptance runs against the daemon-served production frontend, so stream fixtures must work in real browser APIs, not only Node helper tests.

## Risk Signals
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L137-L148` — Possible integration risk: selected activity is local to `ActivityConsole`, but `transcriptUpdated` handling needs to know whether the event matches the selected item.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L269-L285` — Possible dirty-state risk: selected revision changes currently trigger `acknowledgeSelected`, which may clear dirty state for a streamed update before explicit user acknowledgement.
- `ws-dashboard/frontend/src/workRootActivity.ts#L180-L213` — Possible merge risk: existing merge helper replaces `items` from the latest snapshot and never models `itemRemoved`; direct reuse would not satisfy event-by-event feed behavior.
- `ws-dashboard/frontend/src/App.tsx#L1235-L1297` — Possible fallback risk: current pane-open polling starts immediately and repeatedly; leaving it enabled while EventSource is healthy would violate the normal stream path contract.
- `ws-dashboard/frontend/src/App.tsx#L1209-L1233` — Possible stale-response risk: initial snapshot fetch is guarded only by effect cancellation; later stream-triggered refetches need their own request identity/root guard.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L560-L816` — Possible browser-test risk: current Activity Console browser fixtures only mock JSON fetch routes; EventSource/SSE interception needs separate evidence that works under Playwright and the production frontend.
- `ws-dashboard/frontend/tsconfig.route-tests.json#L1-L28` — Possible test-coverage risk: creating a new helper module without adding it to the route-test include list can leave pure helpers uncompiled by `test:work-root-activity`.

## Opinion
- The codebase has enough reusable pure helper and browser fixture structure for this frontend slice, but the selected transcript state boundary is the tightest coupling point.
- Backend stream contract exists in source; no research escalation is needed for event names, route shape, or fallback mode vocabulary.
