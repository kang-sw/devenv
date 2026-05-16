---
title: ws web dashboard workbench layout spike
completed: 2026-05-16
parent: 260516-epic-ws-web-dashboard-workbench-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: containing workbench substrate epic
  260514-research-ws-web-dashboard-direction: prior workbench layout research
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard workbench layout spike

## Background

The workbench substrate should support a `left nav | workRoot workbench`
information architecture with sibling split groups, group-local pinned and
opened rows, dashboard-owned placement policy, and stable terminal logical
width behavior. Dockview is the leading candidate, with FlexLayout as the
comparison fallback if policy enforcement becomes noisy.

This spike should decide whether the chosen layout library can support the
dashboard model without turning the product into a full IDE platform or making
the layout library own resource identity, route identity, auth, runtime
authority, or command semantics.

## Evaluation Criteria

- Supports workRoot-scoped sibling split groups with group-local tab
  selection.
- Can represent or be adapted to pinned durable surfaces and opened transient
  surfaces without confusing them with daemon-owned resource lifecycle.
- Allows a default two-split preset: side-by-side on wide screens and stacked
  on narrow screens.
- Allows later free splitting while the first implementation can hide or limit
  broad split manipulation UI.
- Supports layout serialization without storing authoritative resource
  identity in layout JSON.
- Provides enough control to prevent or intercept unconstrained floating,
  popout, arbitrary docking, invalid moves, and invalid restored layouts.
- Supports keyboard focus movement and future `ctrl+b` prefix composition
  without forcing editor or terminal local bindings into one global mode.
- Lets the dashboard keep workRoot-level controls outside split-group tabs.
- Lets the dashboard enforce default placement, especially file opens into the
  second or later split group and agent/persistent-terminal opens into the
  first or focused group.
- Can preserve PTY/TUI logical width stability during visual drag by allowing
  committed resize, presets, or an explicit fit command instead of continuous
  column changes.

## Expected Output

Record a recommendation for Dockview or FlexLayout and the minimum adapter
shape the implementation child should use. The output should call out any
unacceptable policy gaps before the workbench substrate implementation starts.

## Result

Recommend Dockview as the workbench substrate library, behind a
dashboard-owned adapter. FlexLayout remains a viable fallback if Dockview policy
code becomes noisy, but Dockview better matches the desired
VS Code-editor-group-like shell because its groups, tabbed panels,
serialization, programmatic add/move APIs, drag/drop hooks, theme surface, and
focus movement APIs map directly to the workRoot split-group direction.

Dockview support verified from official cached docs:

- sibling split groups through `api.addGroup(...)` and `api.addPanel(...)`
  with positional `direction` values;
- group-local tabs through groups that hold multiple panels, `direction:
  "within"`, `index`, and tab-group add/move helpers;
- layout persistence through `api.toJSON()` and `api.fromJSON(...)`;
- programmatic panel/group control through `addPanel`, `addGroup`,
  `removePanel`, `removeGroup`, `addFloatingGroup`, `addPopoutGroup`, and
  panel/group `api.moveTo(...)`;
- drag/drop interception through `onWillDragGroup`, `onWillDragPanel`,
  `onWillDrop`, and `onWillShowOverlay`;
- focus movement through `api.moveToNext({ includePanel })` and
  `api.moveToPrevious({ includePanel })`;
- fixed workRoot/global controls by composition outside `<DockviewReact>`,
  driven from the captured `DockviewApi`.

Policy caveats:

- Pinned/opened rows are not native product semantics. Treat them as
  dashboard-owned surface metadata and render/adapt them above Dockview panel
  content rather than letting Dockview own durable/transient meaning.
- Dockview does not expose a documented `disablePopoutGroups` equivalent in
  the checked docs. Do not expose `addPopoutGroup` in the dashboard adapter,
  disable floating by default where possible, intercept drag/drop/UI paths, and
  sanitize restored JSON before applying `fromJSON`.
- Drop prevention hooks can cancel invalid moves, but docs warn that preventing
  some drops may cause unexpected behavior. Keep invalid placement policy in the
  dashboard adapter and prefer constrained creation/move commands over raw
  free-form user docking in the first implementation.
- Layout JSON stores arrangement only. It must not become authoritative
  resource identity; daemon APIs and `/servers/:serverId/...` routes remain the
  source of truth.
- PTY/TUI logical columns must remain separate from visual panel resizing.
  The workbench adapter should expose committed resize, presets, or a later
  explicit fit command instead of streaming visual drag sizes into terminals.

Minimum adapter shape for the implementation child:

```text
SurfaceRegistry
  surface kind, durability, row policy, component, lifecycle ownership

PlacementPolicy
  default group, pinned/opened row, focused-group override, dedupe/focus rules

SplitGroupModel
  dashboard splitGroupId, pinned surfaces, opened surfaces, active selections

DockviewBridge
  only layer allowed to call Dockview API, serialize/restore, and validate moves

WorkbenchAdapter
  openSurface, focusSurface, closeSurface(detach), validatePlacement,
  serialize, restore, moveFocus, commitLogicalTerminalResize
```

No unacceptable Dockview policy gap was found for the next implementation
child, provided raw Dockview docking, popout, restore, and persistence APIs stay
behind the dashboard-owned adapter.
