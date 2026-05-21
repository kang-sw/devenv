# Brief: 260518-feat-ws-dashboard-activity-console-ui

## Intent

Build the route-backed Activity Console UI shell inside the existing WorkRoot
Activity pane. Replace the vertical named-agent card dump with a reusable
read-only console composed of a compact horizontal Activity Ribbon and selected
Transcript Block viewer, using the implemented Activity Feed and Transcript
read model.

## Scope Boundary

Implement only Phase 1 from
`ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-console-ui.md`.

In scope:
- Reusable Activity Ribbon, Transcript Block viewer, and Activity Console
  composition, either as extracted components or local modules.
- WorkRoot Activity pane integration using `fetchWorkRootActivity` and
  `fetchWorkRootActivityTranscript`.
- Default selection of the first live/attention item, or latest item when no
  live/attention item exists.
- Selection preservation across snapshot refreshes when the selected item still
  exists.
- Browser-local dirty/acknowledgement state based on item timestamp or cursor.
- Scroll-position transcript backfill/loading for selected items.
- Command-routed visible controls for ribbon selection, transcript refresh or
  fallback load-more, and detail expansion.
- Component/helper tests plus daemon-served browser verification.

Out of scope:
- Live SSE/watch stream consumption or live UX merge semantics.
- Native Codex/Claude/Gemini transcript resolver expansion.
- Exec job implementation. Exec blocks may render terminal-style when present
  in deterministic UI fixtures or data, but no exec backend source is added.
- Agent start, interrupt, cancel, erase, retry, terminate, or any other control
  action.
- Mobile-specific layout. Constrained desktop/narrow widths use horizontal
  ribbon scroll.

## Caller-Visible Contract

Opening WorkRoot Activity shows a dense read-only Activity Console. A compact
horizontal ribbon lists live/latest activity items, and selecting an item shows
its normalized transcript below. Ribbon item text fits within a three-line
compact shape and the ribbon scrolls horizontally at constrained widths. Newly
updated or attention-worthy items can show a small breathing green cue that is
cleared by local acknowledgement or selection. Transcript blocks render
source-aware summaries with inline detail expansion.

All visible Activity Console controls expose stable command ids and route
clicked behavior through the dashboard command dispatch path. The console
continues to close as a reversible WorkRoot Activity pane with no daemon side
effects.

## Contract Instructions

Public behavior:
- Implement spec `{#260521-ws-dashboard-activity-console-ui-shell}`.
- Use Activity Feed `items` and selected transcript responses from the
  read-model helper. Keep the legacy `agents` projection only for compatibility
  or fallback; do not build the new shell around named-agent-only rows.
- Default selection should prefer live/attention items, then latest updated
  items, then stable order.
- Preserve selection across feed refreshes when the item still exists; fall
  back to the default-selection rule when it disappears.
- On initial feed load, compare local browser acknowledgement state with item
  `updatedAt`, transcript cursor, or feed/item cursor-equivalent data to mark
  items dirty. Store this in browser-local state only.
- Selecting or acknowledging an item clears local dirty state for that item.
- Transcript loading should be selected-item scoped, bounded, and stale-safe:
  a late response for an old selected item or old workRoot must not replace the
  current transcript.
- Scroll-position near the transcript end should load more when `hasMore` is
  true. Explicit refresh/load-more affordances are fallback/error controls.
- Agent transcript rendering: dialogue/assistant/output-like blocks expanded by
  default; tool calls, MCP activity, command runs, status blocks, and errors may
  default to compact one-line summaries with inline detail expansion.
- Exec transcript blocks, when present in deterministic component state, render
  in a terminal-output style.
- Empty, loading, degraded, unavailable, running, completed, and error states
  must be visible and bounded.

Command path:
- Add command builders/types if needed in `commands.ts` for
  `activity.selectItem`, `activity.transcript.loadMore`, `activity.refresh`,
  and `activity.detail.toggle`.
- Click handlers for selection, fallback load-more/refresh, and detail toggles
  must call the shared dashboard command dispatcher. Do not add click-only
  behavior that future keybindings would have to rediscover.
- Background fetch/merge effects are data effects, not commands.

Visual/layout:
- Use the dashboard dark visual system and semantic CSS tokens in
  `frontend/src/styles.css` and `frontend/DESIGN.md`.
- Avoid a one-hue green UI; the green cue is small and semantic.
- Do not add visible tutorial/instruction text.
- Ribbon buttons must keep stable dimensions and prevent text overlap.
- Mobile is not targeted; constrained desktop widths should keep horizontal
  ribbon scroll and preserve the transcript viewer below it.

## Integration Test Instructions

Required TypeScript/component/helper coverage:
- Activity item ordering and default selection.
- Selection preservation and fallback when selected item disappears.
- Local dirty/acknowledgement initialization and clearing.
- Command dispatch parity for ribbon selection, load-more/refresh fallback,
  and detail expansion.
- Transcript block rendering modes for agent action units, tool/status/error,
  output, and exec terminal-style blocks.
- Stale transcript response guards and scroll-position load-more trigger logic.
- Long labels/status text truncation assumptions where testable without a
  browser layout engine.

Required browser-level coverage:
- Daemon-served production frontend opens a workRoot and the WorkRoot Activity
  pane renders the Activity Console.
- Ribbon uses the three-line compact item shape and horizontal overflow at a
  constrained desktop width.
- Selecting a ribbon item changes the selected transcript.
- Breathing/dirty indicator appears for locally dirty or attention-worthy items
  and clears on selection/acknowledgement.
- Transcript blocks render and inline detail expansion works.
- Scroll-position loading or fallback load-more path is exercised.
- Duplicate WorkRoot Activity pane opens focus the existing pane.
- Closing the WorkRoot Activity pane remains immediate with no daemon side
  effect.
- Switching roots does not show stale prior-root activity.
- Command ids are present and click behavior goes through command dispatch.

Verification commands:
- `cd ws-dashboard/frontend && npm run test:work-root-activity`
- `cd ws-dashboard/frontend && npm run test:commands`
- `cd ws-dashboard/frontend && npm run test:workbench`
- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:browser`

Run relevant Rust activity tests only if backend routes/types are changed. The
expected implementation should be frontend-only plus tests/styles, consuming
the existing read model.

## Implementation Strategy Decisions

- Integrate into the existing `workRootActivity` frontend helper/module and the
  current WorkRoot Activity pane path rather than creating a separate page.
- Prefer extracting pure selection/dirty/transcript helper functions so most
  behavior can be covered in route/component-style TypeScript tests.
- Extend the existing Playwright dashboard acceptance gate for visible behavior
  instead of creating a detached demo page.
- Use deterministic browser-gate fixture data where needed, but keep the
  browser gate daemon-served and route-backed. Do not make mock data the
  canonical production path.

## Rejected Alternatives

- Implementing live SSE updates in this shell ticket: rejected; live behavior
  belongs to `260518-feat-ws-dashboard-activity-live-ux`.
- Adding terminate/cancel/agent-control buttons: rejected; the console remains
  read-only.
- Building mobile-specific layout: rejected; constrained desktop uses
  horizontal ribbon scroll.
- Keeping the old vertical named-agent card dump as the primary pane UI:
  rejected; the Activity Console is ribbon plus selected transcript.

## Approach

- Add or extend frontend types/helpers in `workRootActivity.ts` for selection,
  dirty acknowledgement, transcript view state, and render classification.
- Extract Activity Console rendering from `App.tsx` if doing so reduces
  complexity and gives focused tests.
- Replace `WorkRootActivityPane` body with Activity Console composition while
  preserving loading/error/empty states and workbench lifecycle behavior.
- Fetch selected transcripts with stale-response guards keyed by workRoot id,
  activity id, and request sequence.
- Route visible control handlers through `DashboardCommandDispatcher`.
- Add focused CSS for ribbon layout, item text truncation, dirty cue, transcript
  block layout, inline detail, terminal-style output, and constrained width.
- Extend route/helper tests and Playwright browser assertions.

## Constraints

- No visible UI controls outside the Activity Console shell unless necessary to
  preserve existing WorkRoot Activity entry/close behavior.
- No daemon authority expansion; browser code consumes daemon routes only.
- No private path/session/process/cache leakage in UI labels, tooltips, details,
  test logs, or browser evidence.
- Browser verification artifacts may exist under the existing e2e artifacts
  location; do not stage generated artifacts unless they are already tracked and
  intentionally updated.

## Out of scope

- SSE/watch backend and frontend live merge.
- Transcript source expansion beyond existing named-agent read model.
- Agent controls or exec job execution.
- Mobile-specific UX.

## Details

The implementation should keep the WorkRoot Activity pane as the same
workbench surface kind and title unless a small label tweak is needed. The
user-facing first viewport inside the pane should be the actual Activity
Console, not explanatory copy.

Dirty state can be implemented with in-memory browser state for this phase.
Persistent localStorage is optional only if it stays small, workRoot-scoped,
and path-free. Daemon read receipts are out of scope.

The read model currently exposes named-agent transcript blocks primarily from
normalized output. The UI shell should support source-neutral block kinds and
not assume every item is a named agent.

## Verification Contract

The implementation is acceptable only when the listed TypeScript, build, and
browser-gate commands pass, or when a browser-gate failure is escalated with
exact blocker evidence. Since this is visible UI work, pure TypeScript tests
and `npm run build` are not sufficient.

## References

- `ai-docs/spec/ws-web-dashboard/index.md` — [Must]
  `{#260521-ws-dashboard-activity-console-ui-shell}`,
  `{#260521-ws-dashboard-activity-console-read-model}`, browser acceptance, and
  workbench specs.
- `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-console-ui.md` —
  [Must] target ticket and Phase 1 verification expectations.
- `ai-docs/tickets/todo/260518-epic-ws-dashboard-activity-console.md` —
  [Must] cross-child vocabulary and read-only/non-scope constraints.
- `ai-docs/mental-model/ws-web-dashboard.md` — [Must] dashboard UI, command,
  route, browser-gate, and visual-system modification rules.
- `ws-dashboard/frontend/src/App.tsx` — [Must] current WorkRoot Activity pane
  integration.
- `ws-dashboard/frontend/src/workRootActivity.ts` — [Must] read-model frontend
  helper/types.
- `ws-dashboard/frontend/src/commands.ts` — [Must] command dispatch builders and
  ids.
- `ws-dashboard/frontend/src/styles.css` and `ws-dashboard/frontend/DESIGN.md`
  — [Must] visual system and existing Activity pane styling.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` and
  `ws-dashboard/frontend/e2e/daemonHarness.ts` — [Must] browser-gate patterns.
