# Brief: 260516-feat-ws-web-workbench-editor-chrome-polish Phase 1

## Intent

Correct the dashboard workRoot workbench from an explanatory topology mock into
compact editor-like chrome. The visible result must read as an IDE/workbench
surface: a left resource nav beside split editor groups, thin tab strips, and
dominant pane bodies. The phase is frontend-only and does not add live PTY,
editor, viewer, task, diagnostics, inspector, or daemon lifecycle behavior.

## Scope Boundary

Implement only Phase 1: Editor-Like Workbench Chrome.

Do not complete Phase 2 visual contract verification as a ticket result. It is
acceptable to add or adjust automated tests that make Phase 1 behavior safe, but
paired screenshot capture and final visual-gate reporting remain Phase 2 unless
the lead explicitly expands scope.

## References

Must:
- `ai-docs/spec/ws-web-dashboard/index.md`:
  `260516-ws-web-dashboard-workroot-workbench-substrate` and
  `260516-ws-web-dashboard-dark-visual-system`.
- `ai-docs/mental-model/ws-web-dashboard.md`: workbench adapter boundary,
  left-nav identity, placement/lifecycle policy, PTY logical sizing, and dark
  token rules.
- `ws-dashboard/frontend/src/App.tsx`: current workbench shell, toolbar,
  resource selection, command ids, and placeholder surfaces.
- `ws-dashboard/frontend/src/styles.css`: dashboard semantic tokens and current
  workbench chrome CSS.
- `ws-dashboard/frontend/src/workbench/`: surface registry, layout
  serialization, Dockview bridge, and placement policy.
- `ws-dashboard/frontend/DESIGN.md`: dark-first dense operational visual rules.

Maybe:
- `ws-dashboard/frontend/src/workbench/workbenchModel.test.ts`: update existing
  workbench adapter/policy expectations when tab movement behavior changes.
- `ws-dashboard/frontend/package.json`: frontend test and build commands.

## Required Product Contract

- Keep the information architecture as `left nav | workRoot workbench`.
- Keep default left nav at server/workspace/workRoot identity. Do not re-add
  mainInstance or subInstance rows to the default left nav.
- Replace the current large `Primary`, `Support`, `Pinned row`, and
  `Opened row` visual treatments. Those are internal model concepts, not large
  user-facing labels.
- Preserve the default two-split preset concept, but render each split as an
  editor group with a thin tab/header strip and a large selected pane body.
- Treat pane body as the primary surface. Agent, terminal, editor, viewer,
  task, diagnostics, and inspector placeholders may be fixture-backed or empty,
  but they must resemble real workbench panes rather than cards.
- Main agent and persistent terminal surfaces are durable/pinned workRoot
  surfaces. Render them as compact tabs/chips/icons in an editor group, not as
  tall rows.
- Opened/support surfaces render as ordinary editor/workbench tabs or compact
  toolbar affordances, not card grids.
- Clicking a visible tab must select the active pane inside its group.
- Where practical, use Dockview frontend tab movement so tabs can reorder
  inside a group and move between split groups. This updates only browser
  workbench arrangement state.
- Keep floating/popout groups disabled.
- Do not present fake controls. If a control appears clickable or draggable, it
  must perform the corresponding frontend behavior or be absent/clearly
  disabled.

## Implementation Direction

- Prefer using Dockview's real tab and content rendering surface for the
  workbench groups instead of hand-rendering topology rows in React. If a
  local wrapper is needed, keep it dashboard-owned and keep raw Dockview panel
  or group handles out of exported workbench APIs.
- Update the Dockview bridge/options as needed so frontend tab movement is
  permitted while floating groups remain disabled. Existing tests that assert
  all raw drag/drop is disabled should be revised to the new contract: tab
  movement is allowed, floating/raw lifecycle exposure is not.
- Keep dashboard logical surface keys, layout serialization, close-as-detach,
  terminate reservation, and PTY logical size policy intact.
- Use the existing resource data to seed deterministic placeholder surfaces:
  main agent, persistent terminal, selected resource viewer, editor/detail,
  task view, diagnostics/events, and inspector are enough.
- Surface contract/debug details such as `close: detach` and `pty: 80x24` only
  as subtle status text, tooltip copy, or pane metadata. They must not be major
  chrome.
- Preserve `data-command-id` behavior for existing toolbar/actions. New visible
  action buttons should use command ids if they dispatch through the same
  command layer.
- Keep dark-first semantic tokens and dense square geometry. Do not introduce
  rounded cards, gradients, heavy shadows, marketing-scale text, or raw light
  palette values.

## Out Of Scope

- Live PTY, live editor, live viewer, live task, diagnostics, inspector, file
  explorer, or backend lifecycle behavior.
- Persisting restored layout to storage beyond existing model/test behavior.
- Daemon start/stop/terminate semantics.
- PTY logical column/row resizing from visual split drag.
- Floating/popout windows.
- Keyboard navigation and tmux/vim command binding layers.
- Final Phase 2 screenshot-gate capture and ticket closure.

## Acceptance Criteria

- The workbench no longer exposes `split group`, `Pinned row`, or `Opened row`
  as large visible labels.
- The selected pane body is present and visually dominant in each visible split.
- Surface placeholders look like pane bodies, not dashboard cards.
- Visible tabs switch active panes.
- Dockview tab movement is available where practical for reorder and
  cross-split moves; if a Dockview limitation blocks this, the implementation
  must record the exact limitation and remove misleading drag affordances.
- Floating/popout remains disabled.
- Left nav remains server/workspace/workRoot only by default.
- Existing route/resource/auth behavior is unchanged.
- Tests/build pass:
  `cd ws-dashboard/frontend && npm run test:routes && npm run test:workbench && npm run build`.
