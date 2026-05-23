# Survey: 260523-feat-ws-dashboard-workroot-registry-activation Phase 2

## Reusable Components
- `ws-dashboard/crates/daemon/src/resources.rs#L22-L45` — `dashboard_resources` / `live_dashboard_resources`: canonical `GET /api/dashboard/resources` handler already clones registry state, runs live discovery off async worker threads, and shares the aggregated live view builder with open-workRoot responses.
- `ws-dashboard/crates/daemon/src/discovery.rs#L55-L89` — `LocalDashboardResourcesProvider::dashboard_resources`: recomputes each candidate's live filesystem/Git availability on every call and emits the server-level `refresh` action hint.
- `ws-dashboard/crates/daemon/src/discovery.rs#L115-L150` — `WorkspaceBuilder::push`: preserves passed activation while setting discovery-derived availability/state/actions; this is the availability-vs-activation seam for refresh tests.
- `ws-dashboard/crates/daemon/src/discovery.rs#L216-L316` — `discover_work_root` / `discover_existing_dir` / `discovered_unusable`: classifies missing, moved, inaccessible, and available roots without dropping the candidate.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L19-L142` — `OpenedWorkRoots`: in-memory registry with activation-preserving `candidate_roots`, deterministic sorting, `register_registry_entry`, and `set_activation`.
- `ws-dashboard/frontend/src/commands.ts#L1-L81` — dashboard command model/builders: already has stable `dashboard.refresh`, `workRoot.open`, and `workRoot.activation.set` command ids with logical payloads.
- `ws-dashboard/frontend/src/App.tsx#L196-L222` — `loadResources`: current canonical resource fetch path for explicit refresh; failures set error without clearing existing `resources`.
- `ws-dashboard/frontend/src/resourceModel.ts#L126-L207` — `flattenEntities`, `preferredSelection`, `reconcileSelectedId`: pure resource reconciliation that preserves selections still in the entity set and drops stale selections.

## Existing Patterns
- Protected route registration: see `ws-dashboard/crates/daemon/src/router.rs#L43-L63` — dashboard resource, open-workRoot, and activation routes live inside the owner-auth protected router.
- Open-workRoot immediate reconciliation: see `ws-dashboard/crates/daemon/src/root_picker.rs#L96-L164` and `ws-dashboard/frontend/src/openWorkRoot.ts#L15-L33` — open validates one candidate, persists registry membership, returns aggregated resources plus `x-ws-dashboard-opened-work-root-id`.
- Activation command execution: see `ws-dashboard/frontend/src/App.tsx#L411-L445` — `executeCommand` injects executable handlers for refresh and activation before dispatching through command observer/logging.
- Visible refresh command surface: see `ws-dashboard/frontend/src/App.tsx#L648-L666` and `ws-dashboard/frontend/src/App.tsx#L520-L537` — both initial fetch-failed refresh and resource action buttons use command ids instead of direct-only state mutation.
- Bounded polling precedent: see `ws-dashboard/frontend/src/App.tsx#L1542-L1629` — terminal output polling uses refs, in-flight suppression, interval cleanup, and selective state replacement.
- Activity fallback polling precedent: see `ws-dashboard/frontend/src/App.tsx#L1531-L1537` and `ai-docs/spec/ws-web-dashboard/index.md#L421-L451` — Activity Console treats polling as bounded fallback/snapshot refresh rather than authority.

## Relevant Interfaces
- `ws-dashboard/crates/core/src/resources.rs#L16-L34` — `WorkRootAvailability` / `WorkRootActivation`: public enums that keep discovery-derived availability distinct from user activation.
- `ws-dashboard/crates/core/src/view_model.rs#L13-L52` — `DashboardResourcesView` / `WorkRootView`: serialized resource contract carrying `activation`, `availability`, `status`, `state`, and actions.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L45-L65` — `DashboardStateStore`: loads/persists registry entries; refresh should not need to persist unless membership/activation changes.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L131-L213` — registry v1/v2 migration and write path: v1 opened roots migrate online, v2 stores path/activation/provenance only.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L478-L520` — `resolve_online_available_work_root`: route gate distinguishes unknown/offline/unavailable by current registry membership, activation, and live filesystem readability.
- `ws-dashboard/frontend/src/resourceModel.ts#L43-L55` — `WorkRootView`: frontend availability union includes `unknown`, while `status` union currently omits `unknown`.
- `ws-dashboard/frontend/src/App.tsx#L2066-L2108` — shell rendering: loading/error notices only replace the UI when no resources exist; with resources present, failures surface as inline notices.
- `ws-dashboard/frontend/package.json#L6-L18` — focused frontend verification scripts: `test:resource-model`, `test:commands`, `test:open-work-root`, `test:browser`, plus `build`.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L106-L145` — known workRoots remain visible through missing/inaccessible/moved/inactive states; explicit refresh recomputes availability without changing activation; bounded polling is planned but not authoritative.
- `ai-docs/spec/ws-web-dashboard/index.md#L656-L663` — after open, the resources endpoint is canonical for subsequent refreshes and the open response header supplies the daemon-owned opened id.
- `ai-docs/mental-model/ws-web-dashboard.md#L69-L74` — `resources.rs`, `discovery.rs`, `root_picker.rs`, `resourceModel.ts`, `openWorkRoot.ts`, and `App.tsx` are the named ownership seams for this behavior.
- `ai-docs/mental-model/ws-web-dashboard.md#L118-L124` — resource route must never fall back to mock data and open-workRoot response must remain aggregated with daemon-owned id header.
- `ai-docs/mental-model/ws-web-dashboard.md#L151-L153` — local discovery preserves identity from the remembered candidate path, not canonical symlink target.
- `ai-docs/mental-model/ws-web-dashboard.md#L311-L314` — common mistakes include duplicating resource fixtures in React state, hiding post-load refresh errors, or trusting browser route basis over `/api/dashboard/resources`.

## Risk Signals
- `ws-dashboard/frontend/src/App.tsx#L196-L218` — Possible stale-result risk: `loadResources` has no request sequence/abort identity today, so slower poll responses could overwrite a newer explicit refresh or open response.
- `ws-dashboard/frontend/src/App.tsx#L411-L445` — Possible error-handling risk: activation uses `.then(setResources)` without `.catch`, unlike `loadResources`; adding polling nearby could repeat unbounded/unhandled failure behavior if not centralized.
- `ws-dashboard/frontend/src/App.tsx#L2248-L2260` — Possible command-routing risk: toolbar `refresh` actions dispatch `resource.action.refresh`, not `dashboard.refresh`; handlers still execute because payload type is `refresh`, but stable command-id expectations may need planner/lead inspection if a visible explicit refresh control changes.
- `ws-dashboard/frontend/src/resourceModel.ts#L43-L55` — Possible contract drift risk: `WorkRootAvailability` includes `unknown`, but `WorkRootView.status` excludes `unknown`; if daemon ever serializes `WorkRootStatus::Unknown` or maps unknown status, TS types/tests will need adjustment.
- `ws-dashboard/crates/daemon/src/discovery.rs#L154-L167` — Possible stale-label risk: any degraded workRoot makes workspace `state.error` say `one or more workRoots need refresh`, even though Phase 2 makes refresh/polling normal; visible wording may become misleading.
- `ws-dashboard/crates/daemon/tests/routes.rs#L877-L913` — Possible test gap: missing-root coverage accepts either `missing` or `moved`, but there is not yet a route test that starts available, changes availability, refreshes, and proves activation/membership preservation in one sequence.
- `ws-dashboard/frontend/src/App.tsx#L196-L222` and `ws-dashboard/frontend/src/resourceModel.test.ts#L173-L196` — Possible frontend test gap: resource-model tests cover selection reconciliation, but no component/lifecycle test currently exercises polling overlap, cleanup on unmount, failure preservation, or stale async guards.

## Opinion
- The daemon side already appears close to the Phase 2 refresh contract because the canonical route recomputes availability from registry candidates on every call; the main unknown is whether tests should tighten this rather than adding a new endpoint.
- The frontend polling slice is the highest-risk area because `App.tsx` owns resource fetch, command dispatch, workbench state, terminal polling, and Activity polling in one large component with limited route-independent lifecycle tests.
