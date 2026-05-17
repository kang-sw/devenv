# Brief: 260517-feat-ws-dashboard-workroot-activity Phase 3

## Intent

Add the WorkRoot Activity detail pane so the compact top-bar badge can open a
read-only workRoot-owned activity surface without making terminal panes or a
future agent GUI own the activity model.

## Scope Boundary

Implement Phase 3 only: the reversible WorkRoot Activity workbench pane and the
top-bar badge click/focus behavior that opens it. Phase 1 projection and Phase 2
badge behavior are already implemented and should only be touched where needed
to wire the pane entrypoint.

Do not add agent lifecycle controls, command execution, running-command rows
backed by real jobs, or a future agent GUI. Running Commands must remain absent
or explicitly empty until `260513-feat-async-exec-output-reader` provides that
source.

## Caller-Visible Contract

Clicking the WorkRoot Activity badge for the selected workRoot opens a
reversible WorkRoot Activity workbench pane. If the pane already exists for that
selected workRoot, clicking the badge focuses the existing pane instead of
creating a duplicate.

New WorkRoot Activity panes default to group 1, the agent/terminal-side split.
This is an explicit exception for this reversible projection surface; it must
not change the default group-2 placement for editor/read-only file panes or the
group-3+ preservation policy.

The pane shows the selected workRoot's detailed read-only named-agent activity
projection using the daemon API. It must show bounded status/timing/model
metadata from the projection without exposing host cache paths, stream paths,
pids, session ids, or control actions. Closing the pane detaches the browser
view immediately with no confirmation and does not affect daemon named-agent
state.

## Implementation Strategy Decisions

- Reuse the existing `fetchWorkRootActivity` API/helper as the pane data source;
  the browser must not read ws cache files directly.
- Add a distinct workbench surface kind for WorkRoot Activity through the
  dashboard workbench registry/policy rather than bypassing Dockview or storing
  new daemon authority in layout JSON.
- Treat WorkRoot Activity pane identity as selected-workRoot scoped. The logical
  key should be stable enough that duplicate opens focus the existing pane.
- Render the pane as a read-only projection. Empty/no-agent states are valid and
  should be visible without implying an error.
- Keep the compact badge's toolbar height and summary-only contract from Phase 2
  intact while adding the click entrypoint.

## Rejected Alternatives

- Do not make terminal panes or agent panes own the activity detail UI.
- Do not add agent start/cancel/interrupt/erase controls in this phase.
- Do not implement real running-command activity before the async exec job model.
- Do not use a second custom tab/split shell outside the existing Dockview
  workbench substrate.

## Approach

- Extend the workbench surface registry/model with a reversible WorkRoot
  Activity surface kind, close-confirmation policy, and group-1 placement.
- Add App-level state/actions that open or focus the selected workRoot's Activity
  pane when the top-bar badge is clicked.
- Render a WorkRoot Activity pane body that fetches and displays the selected
  workRoot's detailed named-agent projection, including empty and unavailable
  states.
- Add or update workbench model tests for group-1 placement, duplicate-open
  focus, surface metadata, and immediate close policy.
- Extend the daemon-served Playwright acceptance gate for badge click, group-1
  placement, duplicate focus/no duplicate, close-without-confirmation, and
  visible empty/no-agent activity detail.

## Constraints

- Visible UI changes require Playwright evidence against the daemon-served
  production frontend.
- Keep WorkRoot Activity read-only and daemon-owned.
- Preserve existing editor/read-only file placement behavior and terminal group
  behavior.
- Do not increase top-bar height or introduce a second dashboard tab system.

## Out of scope

- Agent lifecycle controls.
- Running command data backed by async exec jobs.
- Agent GUI replacement for the terminal.
- General workbench layout redesign.

## Verification Contract

Required verification:
- `npm run test:workbench`
- `npm run test:work-root-activity`
- `npm run build`
- `npm run test:browser`
- `git diff --check`

The browser gate must prove badge click opens the Activity pane, the pane lands
in group 1, a second badge click focuses the existing pane without duplicating
it, and close happens immediately without a confirmation popover.

## References

- [Must] `ai-docs/tickets/ready/260517-feat-ws-dashboard-workroot-activity.md` - Phase 3 contract and exclusions.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260517-ws-dashboard-workroot-activity-pane` - planned pane behavior and group-1 exception.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260516-ws-dashboard-workroot-workbench-substrate` - workbench placement, duplicate focus, close policy, and Dockview ownership.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard modification rules for WorkRoot Activity and workbench surface changes.
- [Must] `ws-dashboard/frontend/src/workbench/` - workbench registry, adapter, layout, placement policy, and model tests.
- [Must] `ws-dashboard/frontend/src/App.tsx` - badge rendering, selected workRoot state, pane rendering, and workbench action wiring.
- [Must] `ws-dashboard/frontend/src/styles.css` - Activity pane and workbench visual integration.
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` - daemon-served browser acceptance gate.
- [Maybe] `ai-docs/tickets/todo/260513-feat-async-exec-output-reader.md` - future running-command source kept out of this phase.
- [Maybe] `ws-dashboard/frontend/src/workRootActivity.ts` and `ws-dashboard/frontend/src/workRootActivity.test.ts` - existing activity helper and formatter tests.
