---
title: ws web dashboard workbench layout spike
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
