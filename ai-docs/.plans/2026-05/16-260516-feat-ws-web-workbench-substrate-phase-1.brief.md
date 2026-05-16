# Brief: 260516-feat-ws-web-workbench-substrate Phase 1

## Intent

Create the frontend workbench substrate's internal contract: a dashboard-owned
surface registry and adapter boundary around Dockview. This phase should make
future split-group rendering possible without letting Dockview own dashboard
resource identity, surface lifecycle, placement policy, or terminal sizing.

## Approach

- Add the selected Dockview dependency to the frontend package if it is not
  already present.
- Introduce frontend modules for:
  - surface kinds and durable/transient row policy;
  - layout attachment identity separate from daemon resource ids;
  - default surface registry entries for agent, persistent terminal, editor,
    viewer, diff, diagnostics, logs/events, task view, and inspector;
  - sanitized workbench layout serialization shape;
  - a Dockview bridge/adapter API boundary that future phases can connect to
    the actual component tree.
- Keep Phase 1 mostly type/model/test focused. Do not replace the current
  three-panel shell yet.
- Add focused TypeScript tests for registry defaults, layout identity
  separation, serialization shape, and prohibited raw lifecycle assumptions.

## Constraints

- Scope is Phase 1 only.
- Do not render the full workRoot split-group shell; Phase 2 owns that.
- Do not implement file-open placement, dedupe/focus behavior, or detach
  command behavior beyond contracts; Phase 3 owns behavior.
- Do not encode daemon server/workspace/workRoot/instance identity inside
  Dockview layout JSON.
- Do not expose raw Dockview floating, popout, free docking, or restore APIs as
  product behavior.
- Preserve existing resource fetch behavior, route normalization, command ids,
  and visible shell behavior.

## Out of scope

- WorkRoot combined bar and split-group UI.
- Free split manipulation UI.
- Live terminal, agent, editor, viewer, task, diagnostics, or inspector
  implementations.
- Persistent layout storage.
- PTY logical resize implementation beyond explicit adapter contracts.

## Details

Surface registry should distinguish at least:

- durable pinned surfaces: `agent`, `persistentTerminal`;
- opened/support surfaces: `editor`, `viewer`, `diff`, `diagnostics`,
  `eventsLog`, `taskView`, `inspector`.

The adapter contract should make these separations explicit:

- `AttachmentId` or equivalent frontend layout id;
- daemon resource reference as optional metadata, not layout identity;
- surface kind;
- row policy: `pinned` or `opened`;
- lifecycle owner: browser attachment, daemon process, daemon projection, or
  future document provider;
- close policy, with daemon-backed close represented as detach by default;
- serialized layout containing arrangement and attachment ids only.

Dockview-specific API calls should be isolated behind a bridge boundary so
future phases can validate placement, sanitize `fromJSON`, disable/free-control
floating/popout behavior, and implement keyboard focus movement without leaking
Dockview into the product model.

Verification should run frontend build and focused route/workbench model tests.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-workroot-workbench-substrate`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260516-ws-web-dashboard-core-resource-vocabulary`, and
  `260516-ws-web-dashboard-protected-frontend-shell`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - frontend ownership,
  route/resource identity split, and existing route-basis/test patterns.
- [Must] `ai-docs/tickets/ready/260516-feat-ws-web-workbench-substrate.md` -
  Phase 1 scope and Dockview adapter constraints.
- [Must] `ws-dashboard/frontend/package.json` and package lock - dependency
  and script surface.
- [Must] `ws-dashboard/frontend/src/App.tsx` and
  `ws-dashboard/frontend/src/routeBasis.ts` - existing frontend patterns and
  test style.
- [Must] Dockview docs/API from the layout spike: `addPanel`, `addGroup`,
  `toJSON`, `fromJSON`, `onWillDrop`, `onWillDragPanel`,
  `onWillDragGroup`, `onWillShowOverlay`, `moveToNext`, `moveToPrevious`, and
  options such as `disableDnd` and `disableFloatingGroups` must stay behind the
  dashboard adapter.
