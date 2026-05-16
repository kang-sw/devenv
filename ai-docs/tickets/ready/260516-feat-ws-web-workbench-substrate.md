---
title: ws web dashboard workbench substrate implementation
parent: 260516-epic-ws-web-dashboard-workbench-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workbench-substrate: containing workbench substrate epic
  260516-research-ws-web-workbench-layout-spike: prerequisite layout-library decision
  260514-research-ws-web-dashboard-direction: workbench IA research
spec:
  - 260516-ws-web-dashboard-workroot-workbench-substrate
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard workbench substrate implementation

## Background

After the visual system, stable route basis, and layout spike are settled, the
frontend should replace the first visible shell's temporary panel structure
with a workRoot-scoped workbench substrate. The substrate should preserve the
existing authenticated resource API while making room for later terminal,
agent, editor, viewer, task, diagnostics, and inspector surfaces.

## Decisions

- Use Dockview as the selected layout substrate behind a dashboard-owned
  adapter. FlexLayout remains a fallback only if implementation exposes
  unacceptable Dockview policy complexity.
- The left nav selects server/workspace/workRoot locations. It may show compact
  badges, but it should not expand main instances or sub instances as the
  default hierarchy.
- The opened workRoot owns sibling split groups. Each group has a pinned row
  for durable agent and persistent terminal surfaces and an opened row for
  editor, viewer, diff, diagnostics, logs/events, task view, and inspector
  surfaces.
- Main instances are durable workRoot-local surfaces. Sub instances are
  projections on main instances through badges, popovers, cards, or drawers.
- A frontend panel is an attachment, not a backend instance. Closing a panel
  detaches the view by default; explicit terminate commands own daemon-backed
  lifecycle shutdown.
- Task visibility should aggregate through a workRoot-scoped task view and
  main-instance badges/popovers. Individual tasks should not become top-level
  split-group tabs by default.
- Pinned/opened rows, durable/transient surface meaning, placement policy,
  detach semantics, and logical terminal resize policy are dashboard adapter
  responsibilities. Dockview provides split/tab mechanics and serialized
  arrangement only.
- Raw Dockview floating, popout, free docking, and restore APIs should not be
  exposed as product behavior. The adapter validates creation, move, drop, and
  restored layout state.

## Phases

### Phase 1: Workbench Adapter And Surface Registry

Introduce a dashboard-owned adapter over the selected layout library and a
surface registry that records allowed surface kinds, default placement, durable
versus transient behavior, and layout attachment identity. Keep resource
identity authoritative in daemon APIs and browser routes, not in layout JSON.

### Phase 2: Split-Group Shell

Render the current resource shell through the `left nav | workRoot workbench`
model. Provide the default two-split preset, group-local pinned/opened rows,
and workRoot-level combined bar for breadcrumbs/status plus viewer, task view,
diagnostics/events, and layout toggles.

### Phase 3: Placement And Lifecycle Semantics

Implement initial placement behavior: file/editor opens prefer the second or
later split group, already-open surfaces focus the existing tab, and
agent/persistent-terminal surfaces default to the first or focused group.

Represent panel close as detach for daemon-backed resources in the UI contract,
and reserve explicit terminate commands for lifecycle shutdown. Preserve the
stable PTY/TUI logical resize policy in the adapter even if live PTY
implementation remains deferred.
