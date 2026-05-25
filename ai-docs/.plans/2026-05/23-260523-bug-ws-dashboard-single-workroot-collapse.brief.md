# Brief: 260523-bug-ws-dashboard-single-workroot-collapse

## Intent

Restore browser left-navigation compaction for the common case where the
dashboard has exactly one workspace and one workRoot. The compact row must be a
workRoot-selected location row and must not depend on main/sub instance data.

## Scope Boundary

Selected scope is Phase 1: Restore workspace/workRoot singleton compaction.

Implement only the browser presentation fix and required tests/documentation
cleanup. Do not change the daemon resource hierarchy, route identity, workRoot
registry membership, linked-worktree discovery, or workbench surface ownership.

## Caller-Visible Contract

A dashboard resource tree with one workspace and one workRoot renders one
compact left-nav row. The row selects the concrete workRoot id, not the
workspace id, so file explorer, Activity, terminal, workbench, and browser-route
behavior keep using workRoot identity.

The compact row displays enough identity and status to remain useful:
workspace/workRoot identity, workRoot kind, availability, and activation.
Offline or unavailable single workRoots still display their availability and
activation state clearly.

Workspaces with multiple workRoots continue to render a workspace row plus
separate workRoot rows for comparison. Main/sub instances do not participate in
default left-nav compaction and must not reappear as recursive left-nav rows.
When present, main instances remain workbench surfaces and sub instances remain
workbench projections.

## Contract Instructions

Expected frontend surfaces include:

- `ws-dashboard/frontend/src/App.tsx`
- `ws-dashboard/frontend/src/resourceModel.ts` only if pure flattening or
  compactability helpers need adjustment
- route-independent frontend tests around resource navigation rendering
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` or existing browser
  gate coverage if visible browser behavior changes

Preserve the daemon's full serialized hierarchy. Do not pre-collapse resources
in Rust, fixtures, route handlers, or the daemon provider. Compaction is a
browser presentation rule only.

Replace the stale `compactMainInstance` premise with a workspace/workRoot
singleton rule. The compact row should not require `mainInstances.length === 1`.
If the implementation keeps a helper, name and shape it around
workspace/workRoot compaction rather than main-instance compaction.

Forbidden temporary wiring:

- no daemon-side hierarchy collapse
- no workspace-id selection for the compact workRoot row
- no main/sub instance rows in the default left nav
- no hiding offline/unavailable single workRoots
- no browser fixture duplication that bypasses the normal resource model path

## Integration Test Instructions

Add or extend frontend coverage for:

- single workspace + single workRoot + no main instances renders one compact
  workRoot row selected by workRoot id
- multi-workRoot workspace remains expanded with separate rows
- offline or unavailable single workRoot compact row still shows availability
  and activation metadata
- main/sub instance presence is not required for compaction and does not
  reintroduce recursive left-nav rows

Because this changes visible browser UI, run the production browser acceptance
gate or add focused browser evidence that exercises the daemon-served frontend.

Run at minimum:

- `npm run build`
- relevant frontend route/component tests changed by the implementation
- `npm run test:browser`

## Implementation Strategy Decisions

- Treat singleton compaction as `workspace + workRoot` presentation, not
  `workspace + workRoot + mainInstance`.
- Keep workRoot id as the selectable command target.
- Preserve multi-root expanded navigation for comparison.
- Keep main/sub instance semantics in the workbench model, not the default
  left nav.

## Rejected Alternatives

- Daemon-side pre-collapse is rejected because the resource view-model contract
  preserves the full hierarchy as data.
- Keeping the current main-instance requirement is rejected as stale because
  main instances moved to durable workbench surfaces.
- Selecting the workspace id from the compact row is rejected because downstream
  file, Activity, terminal, and workbench flows require concrete workRoot
  identity.

## Approach

- Inspect the current `WorkspaceRows` and `compactMainInstance` path.
- Replace or refactor the compact helper to decide on workspace/workRoot
  singleton shape.
- Keep compact row metadata aligned with existing workRoot row metadata.
- Add focused frontend tests for singleton/no-main, multi-root, and degraded
  metadata behavior.
- Add or extend browser acceptance evidence for visible left-nav compaction.
- Update stale docs only if implementation confirms wording drift.

## Constraints

- Preserve owner-auth and route identity behavior.
- Preserve the full server/workspace/workRoot/mainInstance/subInstance data
  model.
- Preserve dashboard command dispatch for row selection.
- Keep the visual change local to left navigation.

## Out of scope

- Linked Git worktree discovery.
- WorkRoot forget/remove UI.
- Daemon resource model changes.
- Workbench layout or pane persistence changes.
- Main/sub instance lifecycle behavior.

## Details

The compact row should be visually comparable to the existing compact row style
where possible, but its decision rule must be based on a single workRoot under a
single workspace. If a main instance exists, it may contribute metadata only if
that does not make compaction depend on it or reintroduce main/sub rows.

## Verification Contract

Implementation is not complete until focused frontend tests and browser
acceptance evidence pass for the visible navigation change.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260523-ws-dashboard-single-workroot-nav-collapse`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260523-dashboard-workroot-registry-activation`,
  `260516-ws-web-dashboard-workroot-workbench-substrate`, and
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard left-nav,
  workbench, resource hierarchy, and browser verification invariants.
- [Must] `ai-docs/tickets/ready/260523-bug-ws-dashboard-single-workroot-collapse.md`
  - selected Phase 1 scope and accepted dogfood behavior.
- [Maybe] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-mock-view-model-fixtures` for fixture-backed model
  patterns and `260516-ws-web-dashboard-workroot-file-explorer` for adjacent
  left-nav identity context.
