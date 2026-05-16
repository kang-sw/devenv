---
title: ws web dashboard workbench editor chrome polish
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: completed substrate milestone that introduced the current shell
  260516-feat-ws-web-workbench-substrate: completed implementation whose visible shell needs UI correction
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
related-mental-model:
  - ws-web-dashboard
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
