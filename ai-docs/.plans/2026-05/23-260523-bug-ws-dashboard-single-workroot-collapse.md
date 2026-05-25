# Survey: 260523-bug-ws-dashboard-single-workroot-collapse

## Reusable Components
- `ws-dashboard/frontend/src/App.tsx#L3484-L3555` — `WorkspaceRows`: current left-nav renderer for workspace/workRoot rows; contains the stale `compactMainInstance` branch and the expanded multi-workRoot rendering path.
- `ws-dashboard/frontend/src/App.tsx#L3557-L3599` — `ResourceRow`: shared selectable nav-row button; selection is command-routed through `resource.select` with the supplied entity id.
- `ws-dashboard/frontend/src/App.tsx#L3881-L3897` — `compactMainInstance`: existing compact-row predicate; currently requires one compactable workRoot, exactly one main instance, and no sub instances.
- `ws-dashboard/frontend/src/resourceModel.ts#L126-L177` — `flattenEntities`: pure resource-to-left-nav entity flattening; already omits main/sub instances while keeping workRoot metadata and instance counts.
- `ws-dashboard/frontend/src/resourceModel.ts#L179-L207` — `preferredSelection`/`reconcileSelectedId`: selection helpers that default to and preserve concrete workRoot ids across resource refreshes.
- `ws-dashboard/frontend/src/App.tsx#L3797-L3832` — `resolveWorkbenchSelection`: maps selected workspace/workRoot ids to the concrete `WorkRootView` consumed by file explorer, Activity, terminal, and workbench surfaces.
- `ws-dashboard/frontend/src/App.tsx#L3856-L3870` — `resourceEntityForWorkRoot`: local adapter from `WorkRootView` to workRoot `ResourceEntity`, preserving kind/activation/availability/status metadata.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L320-L341` — `openWorkRootInBrowser`/`selectWorkRootInBrowser`: browser-gate helpers for daemon-served production UI; current selector expects `.row-eyebrow` text `workRoot`.

## Existing Patterns
- Pure route/model tests use simple Node assertions, not a DOM harness: see `ws-dashboard/frontend/src/resourceModel.test.ts#L97-L156` — covers flattening, instance omission, workRoot metadata, and selection identity.
- Frontend package scripts run focused TypeScript route/model tests before browser evidence: see `ws-dashboard/frontend/package.json#L7-L18` — `test:resource-model` compiles route tests and runs `resourceModel.test.js` plus `resourceRefresh.test.js`.
- Browser acceptance opens a real workRoot through the owner-paired daemon-served production frontend: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L393-L407` — suitable place for visible left-nav evidence.
- The live daemon provider sets `WorkspaceView.compactable` from single-workRoot membership but leaves `WorkRootView.compactable` false for live roots: see `ws-dashboard/crates/daemon/src/discovery.rs#L147-L169` — current frontend compact predicate therefore cannot compact normal live opened roots even before the main-instance check.
- The static mock fixture contains a compactable single-workRoot workspace with a main instance: see `ws-dashboard/crates/daemon/tests/fixtures/dashboard_resources.json#L220-L279` — useful as historical shape only; canonical route should not rely on it.

## Relevant Interfaces
- `ws-dashboard/frontend/src/resourceModel.ts#L34-L55` — `WorkspaceView`/`WorkRootView`: frontend contract includes `compactable`, `workRoots`, `kind`, `activation`, `availability`, `status`, and `mainInstances`.
- `ws-dashboard/frontend/src/resourceModel.ts#L82-L124` — `ResourceEntity`: flattened workRoot entities carry `path.workRootId`, `kind`, `activation`, `availability`, `status`, and `instanceCount`; instance entity type still exists in the union but flattening does not emit it.
- `ws-dashboard/crates/core/src/view_model.rs#L9-L52` — core serialized resource model: daemon contract preserves full `server -> workspace -> workRoot -> mainInstances -> subInstances` hierarchy and camelCase JSON.
- `ws-dashboard/crates/daemon/src/resources.rs#L16-L29` — canonical `/api/dashboard/resources` route: owner-authenticated live provider, not mock fixture, and returns server plus live workspaces.
- `ws-dashboard/crates/daemon/src/discovery.rs#L115-L150` — live workRoot rows: activation/availability/status/state/actions are computed from durable registry discovery; `main_instances` appear only when `WS_DASHBOARD_E2E_AGENT_FIXTURE` is set.
- `ai-docs/spec/ws-web-dashboard/index.md#L207-L213` — planned contract: single workspace + one workRoot renders one compact row selected by concrete workRoot id; main/sub instances stay out of default recursive nav rows.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L101-L104` — resource API must preserve the full hierarchy; compaction is presentation policy, not URL identity. This line still says singleton `workspace -> workRoot -> mainInstance` and may be stale against the brief.
- `ai-docs/spec/ws-web-dashboard/index.md#L114-L121` — availability and activation are distinct; offline activation and missing/inaccessible availability must remain visible row metadata.
- `ai-docs/spec/ws-web-dashboard/index.md#L217-L230` — left nav selects server/workspace/concrete workRoot locations; main instances are durable workRoot-local workbench surfaces.
- `ai-docs/mental-model/ws-web-dashboard.md#L87-L88` — dashboard mental model keeps left nav at workspace/workRoot identity and routes main/sub instances into workbench surfaces/projections.
- `ai-docs/mental-model/ws-web-dashboard.md#L173-L174` — risk rule: do not reintroduce mainInstance/subInstance rows or direct click-state mutation outside command dispatch.
- `ai-docs/spec/ws-web-dashboard/index.md#L537-L549` — visible browser changes require daemon-served production frontend browser evidence through `npm run test:browser`.
- `ai-docs/spec/ws-web-dashboard/index.md#L687-L704` — selected-workRoot file explorer sits below resource identity; compact nav must still leave selected workRoot identity visible for explorer behavior.

## Risk Signals
- `ws-dashboard/frontend/src/App.tsx#L3493-L3510` — Possible contract risk: compact rendering includes `compactMain.instance.kind` metadata and depends on `compactMainInstance`; this conflicts with the brief's workspace/workRoot-only decision rule.
- `ws-dashboard/frontend/src/App.tsx#L3881-L3897` — Possible shortcut risk: `compactMainInstance` rejects roots with zero main instances or any sub instances, so the common live opened-workRoot case cannot compact.
- `ws-dashboard/crates/daemon/src/discovery.rs#L127-L149` — Possible test risk: live roots normally have no `mainInstances`, while an env-gated E2E fixture can add one; browser evidence should avoid depending on that fixture if proving no-main compaction.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L330-L336` — Possible test risk: existing workRoot selector searches eyebrow text `workRoot`; if the compact row keeps eyebrow `compact workRoot`, selectors may need to accept both without losing multi-root assertions.
- `ws-dashboard/frontend/package.json#L7-L18` — Possible test harness risk: route-independent frontend tests are pure Node/TypeScript and no React DOM test dependency is present; component-level row-count tests may need pure helper extraction or browser assertions rather than a new test stack.
- `ai-docs/spec/ws-web-dashboard/index.md#L101-L104` — Possible documentation risk: the resource contract still names `workspace -> workRoot -> mainInstance` as the compact singleton chain, which produced a wrong premise for this bug.

## Opinion
- The codebase already centralizes the visible bug in `WorkspaceRows`/`compactMainInstance`; daemon-side hierarchy and route identity seams are well separated and should not need discovery changes.
- The most notable uncertainty is test placement: existing pure frontend tests cover `resourceModel`, while the row compaction behavior currently lives inside non-exported React component code in `App.tsx`.
