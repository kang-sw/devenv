# Brief: 260517-feat-ws-dashboard-workroot-activity Phase 2

## Intent

Add the compact WorkRoot Activity summary badge to the existing workRoot top-bar
badge row so users can see named-agent activity without opening a separate pane.

## Scope Boundary

Implement Phase 2 only: top-bar activity badge projection. Reuse the Phase 1
`GET /api/dashboard/work-roots/{workRootId}/activity` route and frontend helper.
Do not implement the Phase 3 WorkRoot Activity detail pane, group-1 pane
placement, running-command rows, or agent lifecycle controls.

## Caller-Visible Contract

When a workRoot is selected/opened, the workbench top bar shows a compact
activity badge in the existing metadata badge row. The badge summarizes
named-agent activity counts from the Phase 1 daemon projection, such as total
and active/blocked/failed counts when present.

Adding the badge must not create a new toolbar row or increase the existing
top-bar height under covered browser-gate viewports. Under constrained width,
the badge may compact, truncate, or hide secondary text rather than wrapping the
toolbar.

## Implementation Strategy Decisions

- Fetch activity through `fetchWorkRootActivity`; do not read ws cache files in
  the browser and do not add a second activity data source.
- Keep the badge as a summary/entrypoint only. If it is clickable in this phase,
  it may be a reserved command with no detail pane creation; Phase 3 owns pane
  open/focus behavior.
- Preserve current workbench toolbar density and layout. Reuse existing badge or
  chip styling where possible.
- Treat loading, error, and unavailable activity states as bounded compact badge
  states; do not let long diagnostics appear in the top bar.

## Rejected Alternatives

- Do not add a new top-bar row.
- Do not embed activity projection inside terminal panes or future agent panes.
- Do not implement the WorkRoot Activity detail pane in this phase.
- Do not show running commands before `260513-feat-async-exec-output-reader`.

## Approach

- Add frontend state/effect logic for the selected workRoot activity view.
- Render a compact activity chip inside `WorkbenchToolbar`'s existing
  `workbench-toolbar-meta` row.
- Add pure route/helper or formatting tests for activity summary labeling.
- Extend browser acceptance to prove the top-bar height remains stable and the
  activity badge renders without growing the toolbar.

## Constraints

- Visible UI changes require Playwright evidence against the daemon-served
  production frontend.
- The top-bar badge must not reduce terminal/workbench usable height beyond the
  existing toolbar footprint.
- Keep Phase 1 projection behavior unchanged.

## Out of scope

- WorkRoot Activity detail pane and group-1 placement.
- Agent controls.
- Running command rows.
- Broader toolbar redesign.

## Verification Contract

Required verification:
- `npm run test:work-root-activity`
- `npm run build`
- `npm run test:browser`
- `git diff --check`

The browser gate must include assertions that the badge is in the existing
toolbar metadata row and that toolbar height does not increase under the covered
viewports.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260517-ws-dashboard-workroot-activity-projection` - Phase 1 API to consume.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260517-ws-dashboard-workroot-activity-topbar-badge` - direct Phase 2 behavior.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` / `260516-ws-web-dashboard-browser-ui-acceptance-gate` - browser evidence requirement.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - visible UI and browser gate guidance.
