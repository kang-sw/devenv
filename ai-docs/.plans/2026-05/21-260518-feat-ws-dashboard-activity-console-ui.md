# Survey: 260518-feat-ws-dashboard-activity-console-ui

## Reusable Components
- `ws-dashboard/frontend/src/workRootActivity.ts#L38-L75` — `ActivityItem`, `ActivityTranscript`, `TranscriptBlock`: source-neutral frontend types already mirror the read model and should be the basis for ribbon/transcript rendering instead of `NamedAgentActivityView`.
- `ws-dashboard/frontend/src/workRootActivity.ts#L116-L178` — `workRootActivityEndpoint`, `workRootActivityTranscriptEndpoint`, `fetchWorkRootActivity`, `fetchWorkRootActivityTranscript`: existing encoded route helpers for feed and selected transcript loads, including cursor/limit query support.
- `ws-dashboard/frontend/src/workRootActivity.ts#L180-L213` — `mergeWorkRootActivityViews`: existing refresh merge path keeps feed-level `updateMode`, `feedCursor`, and `selectedItemId` from the newer snapshot; relevant when preserving console selection across pane refreshes.
- `ws-dashboard/frontend/src/commands.ts#L1-L38` — dashboard command ids/payload variants already include `activity.selectItem`, `activity.transcript.loadMore`, `activity.refresh`, and `activity.detail.toggle`; builders are the missing reusable layer.
- `ws-dashboard/frontend/src/commands.ts#L123-L164` — `dispatchDashboardCommand` and `dashboardCommandLabel`: shared observer/handler path already labels Activity Console command payloads, so visible controls can enter the command log without a new dispatcher.
- `ws-dashboard/frontend/src/App.tsx#L381-L408` — `executeCommand`: existing app-level dispatcher augments generic select/refresh commands, observes all commands, and can receive control-specific handlers from Activity Console click handlers.
- `ws-dashboard/frontend/src/App.tsx#L1733-L1759` — `openWorkRootActivityPane`: existing Activity badge path routes through `buildWorkbenchOpenActivityCommand` and `decideSurfaceOpenWithDynamicGroups`, focusing duplicate panes instead of creating duplicates.
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts#L99-L108` — `workRootActivity` registry entry: pane is already an opened daemon projection with `releaseProjection` close and no confirmation.
- `ws-dashboard/frontend/src/workbench/policy.ts#L205-L234` — `decideSurfaceOpenWithDynamicGroups`: `workRootActivity` already has a group-1 placement exception and focus-existing behavior for the logical surface key.
- `ws-dashboard/frontend/e2e/daemonHarness.ts#L166-L190` — `startDaemon` harness contract: browser gate must boot or attach to a daemon serving the production `frontend/dist`, not a Vite/dev fixture page.

## Existing Patterns
- Command-routed visible controls: see `ws-dashboard/frontend/src/App.tsx#L1930-L1940` and `ws-dashboard/frontend/src/App.tsx#L1970-L1979` — badge and terminal controls call `onCommand(builder(), { handler })` while exposing `data-command-id` on the button.
- Stale async response guard: see `ws-dashboard/frontend/src/App.tsx#L1209-L1233` — selected-workRoot activity fetches capture `rootId` plus a cancellation flag so root switches do not apply old feed state.
- Recent activity refresh while pane is open: see `ws-dashboard/frontend/src/App.tsx#L1235-L1297` — polling is gated by selected root, pane-open state, document visibility, and in-flight suppression before merging results.
- WorkRoot Activity pane integration: see `ws-dashboard/frontend/src/App.tsx#L2080-L2113` — pane construction supplies workbench state/meta/body and currently labels detail as named-agent activity.
- Current replace target: see `ws-dashboard/frontend/src/App.tsx#L2116-L2184` — `WorkRootActivityPane` renders loading/error/legacy named-agent detail plus the explicit empty running-commands section.
- Browser Activity pane verification: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L498-L664` — existing Playwright step checks daemon-served pane open/focus/close, group-1 placement, duplicate-open focus, and empty/populated activity bodies.
- Root-switch/layout isolation browser pattern: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L911-L960` — second workRoot checks can be extended so stale prior-root Activity Console content is not visible after switching roots.
- Overflow containment pattern: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L666-L909` — long file/explorer checks assert scroll stays inside the pane rather than creating document-level scroll; similar DOM probes fit transcript overflow.
- Dark dense visual style: see `ws-dashboard/frontend/DESIGN.md#L10-L14`, `ws-dashboard/frontend/DESIGN.md#L76-L123`, and tokens in `ws-dashboard/frontend/src/styles.css#L1-L58` — new ribbon/transcript CSS should reuse semantic tokens, square corners, compact spacing, and restrained state colors.
- Existing Activity pane CSS height chain: see `ws-dashboard/frontend/src/styles.css#L1388-L1404` — `.workbench-pane[data-surface-kind="workRootActivity"] .workbench-pane-content` and `.workroot-activity-pane` already establish a flex/min-height chain, but current pane itself scrolls all content.

## Relevant Interfaces
- `ws-dashboard/crates/core/src/activity.rs#L8-L23` — `ActivityFeed`: public source-neutral feed shape includes `items` plus legacy `agents`; console should consume `items`.
- `ws-dashboard/crates/core/src/activity.rs#L39-L74` — `ActivityItem` and `ActivityTranscriptAvailability`: feed items carry live/attention flags, timing, source display metadata, transcript status/cursor, diagnostics, and metadata for compact ribbon display.
- `ws-dashboard/crates/core/src/activity.rs#L76-L101` — `ActivityTranscript` and `TranscriptBlock`: transcript responses expose status/sourceStatus/live/blocks/nextCursor/hasMore/diagnostics and block render data without backend-private paths.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L742-L761` — `transcript_blocks_from_output`: current named-agent backend emits line-based `renderKind: "markdown"` blocks, so UI classification needs graceful defaults for markdown/text before future richer block kinds arrive.
- `ws-dashboard/frontend/src/workRootActivity.test.ts#L1-L13` — existing helper test imports all activity helpers and types; this is the nearest route-test home for pure selection, dirty acknowledgement, transcript load, stale guard, and render-classification helpers.
- `ws-dashboard/frontend/src/commands.test.ts#L1-L13` and `ws-dashboard/frontend/src/commands.test.ts#L75-L97` — command tests already validate builder/observer/handler parity for migrated commands; add Activity Console builders to this matrix.
- `ws-dashboard/frontend/package.json#L6-L16` — verification scripts already include `test:work-root-activity`, `test:commands`, `test:workbench`, `build`, and `test:browser` exactly matching the brief.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L309-L337` — the UI shell contract is settled: horizontal three-line ribbon, local browser acknowledgement, selected transcript viewer, scroll-driven load-more, command-routed controls, read-only/no live SSE.
- `ai-docs/spec/ws-web-dashboard/index.md#L357-L370` — UI-facing dashboard work must be verified against the daemon-served frontend; TypeScript/build-only evidence is insufficient.
- `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-console-ui.md#L12-L25` — components should be reusable beyond the WorkRoot Activity pane and should not be coupled to named-agent-specific metadata.
- `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-console-ui.md#L69-L80` — Phase 1 verification includes ordering, selection preservation, dirty cue acknowledgement, transcript render modes, scroll loading, duplicate pane focus, immediate close, root switching, command ids, and desktop DOM/screenshot evidence.
- `ai-docs/mental-model/ws-web-dashboard.md#L66-L70` — WorkRoot Activity remains read-only, path-private, source-neutral through `items`, and visible mouse actions must carry command identity plus dispatcher routing.
- `ai-docs/mental-model/ws-web-dashboard.md#L117-L117` — changes to Activity feed/transcript/pane must keep source-neutral `items`, avoid agent controls, avoid stale prior-root activity, keep reversible close, and preserve group-1 placement.
- `ws-dashboard/crates/daemon/tests/routes.rs#L1660-L1688` — transcript route tests enforce bounded transcript JSON and forbid root/cache/session/output path leaks; UI labels/tooltips/details should not reintroduce private fields from `metadata` or diagnostics.

## Risk Signals
- `ws-dashboard/frontend/src/App.tsx#L2116-L2184` — Possible contract risk: current pane renders legacy `agents` rows and an explanatory Running Commands empty section, while the brief requires the Activity Console to be built around source-neutral `items`/transcripts and no visible tutorial/instruction text.
- `ws-dashboard/frontend/src/commands.ts#L1-L38` — Possible command risk: Activity command ids and payload variants exist, but there are no exported builders; visible controls may be tempted to construct commands ad hoc unless builders are added and covered in `commands.test.ts`.
- `ws-dashboard/frontend/src/App.tsx#L1209-L1297` — Possible stale-load risk: feed fetches have root cancellation, but selected transcript fetch/load-more will need a separate guard keyed by workRoot id, activity id, cursor, and request sequence.
- `ws-dashboard/frontend/src/styles.css#L1388-L1404` — Possible layout overflow risk: current pane scrolls as a single body; a fixed ribbon plus independently scrollable transcript viewer may need a stricter flex/min-height/overflow chain to prevent Dockview/page growth.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L563-L615` — Possible fixture risk: current browser Activity route interception returns only `agents` and omits `items`, `feedCursor`, `selectedItemId`, and transcript routes; new browser checks need deterministic route-backed item/transcript fixture data without making mock data canonical.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L742-L761` — Possible render-mode risk: current real backend provides mostly `markdown` output lines, so tool/status/error/exec terminal-style block rendering will need deterministic component/browser fixture states until backend source expansion lands.
- `ai-docs/mental-model/ws-web-dashboard.md#L98-L98` — Possible merge risk: feed metadata must travel with the newest item snapshot; selection/dirty helpers should not preserve stale `feedCursor`/`selectedItemId` alongside fresher items.

## Opinion
- No broader research or UI/UX decision blocker surfaced: the brief, ticket, and spec already settle ribbon shape, transcript defaults, command path, local acknowledgement, and browser verification scope.
- Most implementation risk is integration discipline rather than contract ambiguity: extract pure helper functions in `workRootActivity.ts` or a local Activity Console module, then integrate into `App.tsx` after command builders exist so browser work can assert command ids and command-log evidence.
- The browser gate likely needs both route interception for deterministic rich Activity Console states and an unmocked plain-directory path for daemon-served pane lifecycle; keep both behind the existing daemon harness rather than adding a detached demo page.
