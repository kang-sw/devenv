---
title: ws web dashboard workbench editor chrome polish
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: completed substrate milestone that introduced the current shell
  260516-feat-ws-web-workbench-substrate: completed implementation whose visible shell needs UI correction
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
plans:
  phase-1: 2026-05/16-ws-web-workbench-editor-chrome-polish-phase-1
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-16
---

# ws web dashboard workbench editor chrome polish

## Background

The completed workbench substrate implementation made the intended data model
visible, but the visible frontend chrome reads like an explanatory dashboard
mock rather than an editor/workbench interface. Internal model terms such as
split groups, pinned rows, and opened rows became heavy visible section labels.
The result technically demonstrates the substrate topology, but it does not
match the settled product direction: a constrained VS Code-inspired workRoot
workbench where tabs, thin strips, and pane bodies carry the interaction.

This ticket corrects the presentation layer while preserving the implemented
resource API, route identity, workbench policy model, and backend scope.

## Decisions

- Treat `split group`, `pinned row`, and `opened row` as product model concepts,
  not large user-facing section labels.
- Keep the `left nav | workRoot workbench` information architecture and the
  left-nav identity boundary: server/workspace/workRoot only by default.
- Preserve the default two-split preset, but make each split feel like an
  editor group: a thin tab/header strip above a dominant content body.
- Treat pane bodies as the primary visible product surface. Even when live
  backends are deferred, each selected agent, terminal, editor, or viewer
  placeholder must occupy the group body like a real workbench pane rather than
  leaving the shell as chrome-only topology.
- Main agent and persistent terminal affordances should appear as compact
  pinned tabs/chips/icons within a split group, not as a tall explanatory row.
- Opened/support surfaces should appear as ordinary editor/workbench tabs or
  compact toolbar affordances, not as card grids.
- Use Dockview's frontend tab movement affordances where practical. Tab
  dragging may reorder tabs inside a split group or move a tab to another split
  group, but this updates only browser workbench arrangement state and must not
  imply daemon lifecycle changes.
- Contract/debug details such as `close: detach` and `pty: 80x24` should not be
  permanently prominent chrome. They may appear in a subtle status area,
  tooltip, inspector detail, or debug-only surface.
- A visible tab or selector affordance must be honest. If it looks clickable, it
  must at least switch the active surface in that split; if it looks draggable,
  drag behavior must exist or the affordance must be clearly absent/disabled.
- Placeholder surfaces must preserve the intended interaction metaphor. A
  non-live agent/editor/terminal pane may be empty or fixture-backed, but it
  must not pretend that static topology labels are interactive controls.

## Constraints

- Do not add live PTY, editor, viewer, task, diagnostics, inspector, file
  explorer, or backend lifecycle behavior.
- Do not change authenticated resource fetching, pairing, stable route
  normalization, daemon resource identity, or command-id routing.
- Do not weaken the Phase 1/3 workbench policies around serialized layout,
  logical surface keys, close-as-detach, terminate reservation, or PTY logical
  sizing.
- Keep floating/popout groups disabled unless a later ticket explicitly adds
  them. Frontend tab movement is allowed; raw Dockview lifecycle handles remain
  behind the dashboard-owned adapter boundary.
- Do not reintroduce mainInstance or subInstance rows into the default left nav.
- Keep the dark-first semantic token system and dense operational visual style.
- Avoid card-heavy dashboard presentation, nested cards, marketing-scale text,
  decorative gradients, or explanatory labels that consume editor body space.
- Do not present fake controls. Deferred behaviors should be either omitted,
  disabled with clear state, or represented as non-interactive status text.

## Phases

### Phase 1: Editor-Like Workbench Chrome

Replace the visible workbench presentation with compact editor-like chrome:
thin split group tab/header strips, compact pinned/opened surface selectors,
and content panes that occupy most of the workbench area. Remove or demote
large `Primary`, `Support`, `Pinned row`, and `Opened row` treatments when they
act as explanatory model labels rather than useful product controls.

The implementation should still make the substrate inspectable, but through
realistic product chrome rather than topology visualization. Placeholder
content is acceptable only when it resembles empty agent/editor/terminal/viewer
pane bodies, not dashboard cards.

Clickable selector controls must maintain active state inside their split group.
The minimum acceptable behavior is selecting which placeholder pane body is
active. Tab labels should also support frontend-only Dockview movement where
practical: reorder within a split group and move between split groups while
preserving dashboard-owned placement and serialization policy. The movement does
not start, stop, terminate, or resize daemon-backed resources; it only changes
browser workbench arrangement.

### Result (92d642a) - 2026-05-16

Implemented the Phase 1 workbench chrome correction. The visible workRoot
workbench now renders compact editor-like split groups with thin tab strips and
dominant pane bodies instead of large `Primary`, `Support`, `Pinned row`, and
`Opened row` topology labels. Placeholder surfaces now read as agent,
terminal, viewer, editor/detail, task, diagnostics, and inspector panes rather
than dashboard cards.

Visible tabs switch the active pane and support frontend drag/drop movement for
reordering within a split and moving panes across splits. Cross-split
membership persists across rerenders, empty split groups render an honest drop
target instead of crashing, and pure workbench model coverage exercises click
selection, drag/drop resolution, order restoration, cross-split moves, and
active-pane reconciliation.

The implementation kept live PTY/editor/viewer/task backends out of scope,
kept floating/popout groups disabled, preserved left-nav server/workspace/
workRoot identity, and retained daemon lifecycle and PTY logical-sizing
boundaries. Verification passed:
`cd ws-dashboard/frontend && npm run test:routes && npm run test:workbench &&
npm run build`.

#### Edition (c79d8a7) - 2026-05-16

Fixed a follow-up UI contract gap found during dogfooding. The first Phase 1
implementation removed the thick explanatory pinned/opened rows but also
flattened all tabs into one undifferentiated row. The follow-up restores a
compact structured header: durable/pinned surfaces and opened/transient
surfaces render in separate thin lanes without returning to the previous heavy
row presentation. Main agent and persistent terminal remain pinned by default;
viewer, editor/detail, task, diagnostics, and inspector remain opened by
default.

The same pass fixed the side-by-side split layout so pane bodies fill the
available height and the footer/status row sits at the bottom instead of leaving
awkward unused space. Verification passed:
`cd ws-dashboard/frontend && npm run test:routes && npm run test:workbench &&
npm run build`.

### Phase 2: Visual Contract Verification

Add or run visual verification that checks the product contract, not just
presence of sections. The gate should explicitly fail if:

- internal model terms are exposed as large visible labels;
- pinned/opened areas consume substantial vertical space before pane content;
- the selected pane body is absent or visually secondary to chrome;
- surface placeholders render as large cards instead of pane bodies;
- visible tab/selector buttons cannot change the active pane;
- visible tabs cannot be reordered or moved between split groups, unless the UI
  clearly omits drag affordances and the implementation records why Dockview
  movement was blocked;
- the workbench cannot plausibly host an agent, terminal, or editor without a
  second redesign;
- narrow viewport behavior introduces horizontal overflow or unreadable chrome.

Capture real paired desktop and narrow screenshots, or record the exact blocker
if browser tooling is unavailable.

### Result (be9f885) - 2026-05-16

Completed visual contract verification with real authenticated dashboard
captures through the daemon-served frontend. Desktop and narrow screenshots
confirmed that large topology labels such as `split group`, `Pinned row`,
`Opened row`, `Primary`, and `Support` are no longer exposed, compact
pinned/opened tab lanes remain visible, tabs select pane bodies, and the
side-by-side desktop split keeps pane bodies dominant with footer/status rows
anchored at the bottom.

The first narrow capture found a visual contract issue: the first stacked split
collapsed to a very short pane body, making the body secondary to chrome. Commit
`be9f885` fixed the narrow stacked layout with minimum group and pane-body
height while preserving desktop layout, pinned/opened lanes, tab movement, and
footer alignment. After the fix, narrow metrics showed no horizontal overflow
and pane body heights of 189px and 202px for the stacked groups.

Artifacts:
- Desktop screenshot:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/026c4673-01-workbench-phase-2-desktop-after.png`
- Narrow screenshot:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/026c4673-02-workbench-phase-2-narrow-after.png`
- Capture report:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/026c4673-03-workbench-phase-2-report-after.md`

Verification passed:
`cd ws-dashboard/frontend && npm run test:routes && npm run test:workbench &&
npm run build`.
