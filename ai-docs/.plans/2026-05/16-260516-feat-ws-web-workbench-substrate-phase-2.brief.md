# Brief: 260516-feat-ws-web-workbench-substrate Phase 2

## Intent

Replace the temporary `nav | detail | viewer` center presentation with the
first visible `left nav | workRoot workbench` shell. The result should make the
workRoot-scoped sibling split model inspectable in the browser while preserving
the existing authenticated resource fetch, route normalization, and command-id
layer.

## Approach

- Add dashboard-owned React components and CSS for a workRoot workbench shell:
  - a single left navigation rail using the existing resource hierarchy and
    selection behavior;
  - a workRoot-level combined toolbar row for breadcrumb/status and reserved
    viewer/task/diagnostics/events/layout toggles;
  - the default two-split preset in the workbench area, with responsive
    horizontal layout on wide screens and vertical stacking on narrow screens;
  - group-local pinned and opened rows for each split group;
  - visible placeholder surfaces for the selected main agent, persistent
    terminal, editor/viewer/detail, task view, diagnostics/events, and
    inspector concepts using current fixture data only.
- Reuse the Phase 1 registry/model where practical for row labels and policy.
- Keep the shell deterministic and inspectable with no persistent layout
  storage yet.
- Preserve existing user-facing commands by continuing to route mouse actions
  through command ids.

## Constraints

- Scope is Phase 2 only.
- Do not implement Phase 3 placement behavior: no file-open preference logic,
  no already-open dedupe/focus, no close-as-detach command behavior, and no
  explicit terminate commands.
- Do not add live PTY, live terminal, editor, viewer, task, diagnostics, or
  inspector backends.
- Do not make the browser route or serialized layout authoritative over daemon
  server/workspace/workRoot/instance identity.
- Do not expose raw Dockview floating, popout, or free docking behavior as
  product UI. If Dockview is wired, keep it behind the dashboard-owned boundary;
  a dashboard-owned split shell is acceptable for this visible phase if that
  keeps the policy boundary cleaner.
- Preserve `/api/dashboard/resources` as the frontend source of truth,
  `normalizeServerRouteLocation` behavior, existing route tests, existing
  command ids, and the dark visual token system.

## Out of scope

- Layout persistence.
- Drag/drop split editing.
- Free-form panel docking.
- Keyboard navigation.
- Real file explorer/editor integration.
- Live daemon-backed lifecycle commands.

## Details

The default visible shell should still be useful with the current fixture data:

- selecting a workspace/workRoot/main instance in the left nav should update the
  workRoot toolbar and visible split surfaces;
- the first split should bias toward the selected or default main instance and
  show pinned agent/terminal affordances;
- the second split should reserve opened/support surfaces such as resource
  detail, viewer, task view, diagnostics/events, and inspector;
- sub instances should remain secondary context, not first-class left-nav
  hierarchy expansion beyond the existing resource rows.

Keep CSS dense and dark-first. Avoid nested cards, decorative gradients, and
large marketing-scale text. The center workbench area should avoid frequent
agent-pane width changes by using stable split dimensions and responsive
breakpoints rather than arbitrary live resizing.

## Verification

- Run `cd ws-dashboard/frontend && npm run test:routes && npm run test:workbench && npm run build`.
- Add focused frontend model/component tests only if the implementation
  introduces pure helpers worth locking down.
- Because this phase changes visible UI, capture desktop and narrow viewport
  screenshots or document the exact blocker if screenshot tooling is not
  available in the repository.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-workroot-workbench-substrate`,
  `260516-ws-web-dashboard-resource-view-model-contract`, and
  `260516-ws-web-dashboard-protected-frontend-shell`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - route/resource identity,
  command ids, visual tokens, and workbench adapter boundary.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-workbench-substrate.md` -
  Phase 2 scope and Phase 1 result.
- [Must] `ws-dashboard/frontend/src/App.tsx` and
  `ws-dashboard/frontend/src/styles.css` - existing visible shell and styling
  patterns.
- [Must] `ws-dashboard/frontend/src/workbench/` - Phase 1 registry,
  serialization, and Dockview bridge contracts.
