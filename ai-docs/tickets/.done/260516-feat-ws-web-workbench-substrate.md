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
plans:
  phase-1: 2026-05/16-260516-feat-ws-web-workbench-substrate-phase-1.brief
  phase-2: 2026-05/16-260516-feat-ws-web-workbench-substrate-phase-2.brief
  phase-3: 2026-05/16-260516-feat-ws-web-workbench-substrate-phase-3.brief
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-16
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

### Result (eeaf2cc) - 2026-05-16

Added the Dockview dependency, a dashboard-owned workbench surface registry,
branded attachment/resource ids, sanitized workbench layout serialization, and
the initial Dockview bridge boundary. The serialized workbench layout records
only attachment ids, arrangement, and active attachment identity; daemon
resource ids, surface kinds, and registry-derived row policy stay outside the
layout JSON. The bridge returns dashboard-owned handles instead of raw Dockview
panel/group handles so future placement and lifecycle behavior can remain behind
the adapter.

Review caught and fixed two boundary issues before close: row policy was
initially serialized with layout state, and raw Dockview handles were initially
returned by the bridge. Focused frontend tests now cover registry defaults,
identity separation, serialization sanitization, and the non-raw bridge handle
contract.

### Phase 2: Split-Group Shell

Render the current resource shell through the `left nav | workRoot workbench`
model. Provide the default two-split preset, group-local pinned/opened rows,
and workRoot-level combined bar for breadcrumbs/status plus viewer, task view,
diagnostics/events, and layout toggles.

### Result (23c145a) - 2026-05-16

Rendered the first visible `left nav | workRoot workbench` shell. The browser
now keeps the left nav at workspace/workRoot identity while showing main
instances as pinned durable workbench surfaces and sub instances as secondary
workbench projections. The workbench includes a workRoot toolbar with
breadcrumb/status/action/toggle affordances, the default Primary/Support
two-split preset, group-local pinned/opened rows, and fixture-backed placeholder
surfaces for agent, persistent terminal, editor/detail, viewer, task view,
diagnostics/events, and inspector concepts.

Review initially found that the left nav still rendered main/sub instance rows
and defaulted selection to a main instance. Follow-up commit `23c145a` fixed the
nav identity boundary so compact singleton rows and default selection target the
workRoot. Phase 3 placement, close/detach, persistence, drag/drop, live backend,
and keyboard navigation behavior remains deferred.

Verification passed with frontend route tests, workbench model tests, and
frontend production build. Delegated visual checks captured real paired desktop
and narrow viewport screenshots:

- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/workbench-phase-2-desktop.png`
- `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/workbench-phase-2-narrow-real.png`

### Phase 3: Placement And Lifecycle Semantics

Implement initial placement behavior: file/editor opens prefer the second or
later split group, already-open surfaces focus the existing tab, and
agent/persistent-terminal surfaces default to the first or focused group.

Represent panel close as detach for daemon-backed resources in the UI contract,
and reserve explicit terminate commands for lifecycle shutdown. Preserve the
stable PTY/TUI logical resize policy in the adapter even if live PTY
implementation remains deferred.

### Result (3256199) - 2026-05-16

Added the dashboard-owned workbench placement and lifecycle policy layer.
Opened/support surfaces now resolve to the second or later split group when one
exists, durable agent and persistent-terminal surfaces prefer the focused group
and otherwise fall back to the first group, and already-open logical surface keys
focus the existing attachment instead of creating duplicates. The policy layer
also resolves daemon-backed close to detach by default, reserves explicit
`workbench.lifecycle.terminate` commands separately, and preserves stable PTY
logical dimensions while visual split resize data remains deferred.

The visible Phase 2 shell received only small contract chips for close policy
and stable PTY logical dimensions. No live PTY/editor/viewer/task backend,
layout persistence, drag/drop editing, free docking, keyboard navigation, or raw
Dockview lifecycle exposure was added.

Verification passed with frontend route tests, workbench model tests, frontend
production build, and a delegated Phase 3 policy/scope review.
