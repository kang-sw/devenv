# Brief: 260516-feat-ws-web-workroot-file-navigation Phase 2

## Intent

Implement the left-nav file explorer draft for the selected workRoot by
consuming the Phase 1 workRoot file listing API. The owner should be able to
expand and refresh directories below the selected workRoot from the lower
portion of the left navigation area.

## Scope Boundary

Selected slice: `Phase 2: Left-Nav File Explorer Draft`.

Phase 1 API is already implemented. This slice is frontend UI and API
consumption only. Do not add read-only file content APIs, text pane surfaces,
terminal sessions, or workbench restore behavior.

## Caller-Visible Contract

When a workRoot is selected, the dashboard shows a visually subordinate file
explorer below the existing server/workspace/workRoot navigation. The explorer
uses the selected workRoot's opaque `workRootId` and the workRoot-relative
listing path; it never treats raw host paths as browser identity.

The draft supports:

- initial root listing for the selected workRoot;
- directory expand/collapse or an equivalent navigable tree behavior;
- explicit refresh;
- loading, error, and empty states;
- file and directory rows with enough affordance for later read-only file open;
- command ids for mouse-triggered actions.

Readable file open may remain disabled or stubbed until the read-only text pane
ticket exists. The UI must not imply write-back editing or general file-manager
operations.

## Implementation Strategy Decisions

- Keep the explorer in the lower left navigation area, below resource identity.
- Use the existing frontend resource/workRoot selection as the source of
  selected workRoot identity.
- Consume the Phase 1 API from the frontend rather than duplicating fixture file
  data.
- Style with existing dashboard semantic tokens and dense operational layout.
- Preserve the existing workbench panes and tab movement behavior.

## Rejected Alternatives

- Do not add save/open text pane behavior in this ticket.
- Do not add delete, rename, move, copy, chmod, or recursive file-manager
  operations.
- Do not add a large separate global file-browser page.
- Do not infer workRoot identity from browser routes or raw paths.

## Approach

- Add small frontend types and fetch helper for the workRoot file listing API.
- Add state keyed by selected `workRootId` for expanded directories, loaded
  directory data, loading/error states, and refresh.
- Render the explorer below existing resource navigation without hiding the
  server/workspace/workRoot rows.
- Use `data-command-id` for expand/collapse, refresh, and disabled/stubbed file
  open actions.
- Add focused frontend tests where existing test setup supports pure logic or
  route-independent state helpers; otherwise keep verification to TypeScript
  build plus existing tests.

## Constraints

- Keep text fitted in narrow layouts.
- Preserve dark visual system tokens; avoid raw light palette, decorative
  cards, gradients, or large marketing-like UI.
- Preserve Phase 1 path contract: API calls use `workRootId` plus relative
  paths only.
- Do not change daemon API behavior unless a small frontend-consumption issue is
  found and covered by tests.

## Out of scope

- Read-only file content preview.
- Workbench file panes and placement policy.
- Terminal sessions.
- Persistent browser restore for explorer expansion beyond normal component
  state unless it is already trivial in local state.

## Details

Phase 1 route shape:

```text
GET /api/dashboard/work-roots/{workRootId}/files?path=<relative-path>
```

Root listing uses an absent or empty relative path. The response is camelCase
and contains the selected `workRootId`, current relative path, status, and
entries.

## Verification Contract

- Run frontend tests/build checks relevant to touched TypeScript/CSS.
- Run daemon route tests only if daemon API code changes.
- If possible, run the local frontend shell against the Vite/dev or production
  build enough to catch obvious render errors.
- Report any tooling blocker explicitly.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - protected frontend shell, inspectable navigation shell, workbench substrate, dark visual system, implemented listing API, and planned file explorer contracts.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - frontend command-id, workbench, styling, and workRoot file listing invariants.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-workroot-file-navigation.md` - selected Phase 2 scope and Phase 1 result.
- [Must] `ai-docs/tickets/todo/260516-epic-ws-web-dashboard-workroot-io-substrate.md` - milestone non-scope and file-manager exclusions.
- [Maybe] `ai-docs/tickets/todo/260516-feat-ws-web-readonly-text-pane.md` - later file open handoff.
- [Maybe] `ai-docs/tickets/todo/260516-feat-ws-web-workroot-io-workbench-integration.md` - later command/placement integration.
