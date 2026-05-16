# Implementation Plan: 260516-bug-ws-web-dashboard-live-resource-api-connection

Connect `GET /api/dashboard/resources` and the browser resource tree to live
opened workRoots. The live discovery provider, open-workRoot route, file/text/
terminal substrates already exist; only the *default resource authority* is
still mock-backed.

## Codebase Facts (grounding)

- `ws-dashboard/crates/daemon/src/resources.rs#L8-L16` — `DashboardResourcesProvider`
  trait (provider seam) and `dashboard_resources` handler. Handler currently
  hardcodes `MockDashboardResourcesProvider::default()` and takes no state.
- `ws-dashboard/crates/daemon/src/mock.rs#L5-L16` — `MockDashboardResourcesProvider`
  deserializes the golden fixture `tests/fixtures/dashboard_resources.json`.
- `ws-dashboard/crates/daemon/src/discovery.rs#L25-L77` —
  `LocalDashboardResourcesProvider::new(Vec<LocalWorkRootCandidate>)` already maps
  candidate paths into `DashboardResourcesView`; empty candidates yield a server
  with `workspaces: []`. `LocalWorkRootCandidate::new` at `#L14-L23`.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L18-L38` — `OpenedWorkRoots`
  holds `Arc<RwLock<HashMap<WorkRootId, PathBuf>>>`; exposes only `register` and
  `resolve`. No accessor to list registered roots.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L87-L120` — `open_work_root`
  builds a single-candidate `LocalDashboardResourcesProvider`, validates Online,
  calls `state.opened_work_roots.register(...)`, and returns a view containing
  only the just-opened root.
- `ws-dashboard/crates/daemon/src/router.rs#L23-L29` `AppState`; `#L37` registers
  `/api/dashboard/resources` in the protected router. State extraction is
  automatic for handlers that add `State(state): State<AppState>`.
- `ws-dashboard/crates/daemon/src/server.rs#L41-L47` builds `AppState` with
  `OpenedWorkRoots::default()`.
- `ws-dashboard/crates/core/src/view_model.rs#L10-L15` — `DashboardResourcesView`.
- WorkRootId is a stable FNV path hash (`discovery.rs#L334-L345`,
  `WorkspaceBuilder::push#L102-L132`). `open_work_root` registers under
  `work_root.id` from the same provider, so rebuilding the provider from
  registered paths yields identical ids — refresh keeps stable identity.
- Frontend: `ws-dashboard/frontend/src/App.tsx#L210-L234` `loadResources` fetches
  the canonical endpoint; `#L313-L340` `executeCommand` already refetches on
  `refresh`; `#L246-L253` resets `selectedId` when the prior selection leaves the
  entity set; `flattenEntities#L2033-L2078` and `resolveWorkbenchSelection#L2091-L2121`
  derive nav/workbench selection. `preferredSelection` is used at `#L251`.
- Frontend pure-function tests: `frontend/src/*.test.ts` compiled by
  `tsconfig.route-tests.json` (`include` list) and run per-file via `package.json`
  `test:*` scripts. `dev.sh test` runs only `cargo test --workspace` + frontend
  build, not the `test:*` scripts.

## Constraints

- `OpenedWorkRoots` backing store is a `HashMap` (unordered). Any aggregated
  provider build must sort candidates (e.g. by path string) so route responses
  and route tests are deterministic.
- The route test `dashboard_resources_api_returns_mock_hierarchy_with_owner_cookie`
  (`crates/daemon/tests/routes.rs#L448-L540`) asserts the route equals the mock
  fixture; it must be repurposed once the route goes live.
- The golden fixture `crates/daemon/tests/fixtures/dashboard_resources.json` and
  `MockDashboardResourcesProvider` must stay so `core` serde/contract tests and
  frontend fixtures stay deterministic — keep mock for tests, not the live route.
- Do not introduce host paths as identity; keep opaque `serverId`/`workspaceId`/
  `workRootId`. The existing provider already obeys this.
- The frontend currently has **no open-workRoot browser flow** (no root-picker
  UI, no caller of `/api/dashboard/root-picker*` or `/api/dashboard/work-roots/open`).
  See Opinion — Phase 2 must add a minimal open affordance.

## Phase 1 — Live resources endpoint

Goal: `GET /api/dashboard/resources` returns live opened workRoot state; honest
empty live view before any workRoot is opened.

1. `work_root_files.rs` `OpenedWorkRoots` — add a deterministic accessor:
   `pub fn candidate_paths(&self) -> Vec<PathBuf>` returning registered paths
   sorted by path string (stable order for tests). Keep `register`/`resolve`.
2. `resources.rs` `dashboard_resources` — change to
   `pub async fn dashboard_resources(State(state): State<AppState>) -> Json<DashboardResourcesView>`.
   Build `LocalDashboardResourcesProvider::new(state.opened_work_roots.candidate_paths()
   .into_iter().map(LocalWorkRootCandidate::new).collect())` and return its
   `dashboard_resources()`. Empty registry → server with `workspaces: []`
   (honest empty live view, no mock fixture).
3. `mock.rs` — keep `MockDashboardResourcesProvider`; it is no longer used by the
   live route. Do not delete (tests/fixtures still consume it). Optionally add a
   one-line unit test that it still deserializes the fixture so the golden
   artifact stays covered after the route stops exercising it.
4. `root_picker.rs` `open_work_root` — after `register(...)`, rebuild the provider
   from `state.opened_work_roots.candidate_paths()` and return that aggregated
   `DashboardResourcesView` (all opened workRoots), not the single-candidate view.
   Keep the existing single-candidate discovery only for the Online validation
   gate before registering. This makes the immediately-returned view consistent
   with the canonical endpoint.
5. `router.rs` `#L37` — no signature change needed; axum injects `State`.
   `server.rs` unaffected (`OpenedWorkRoots::default()` already wired).

Decision to confirm with lead: fallback is an **empty live view** (recommended,
minimal). An opt-in fixture/development mode (CLI flag / `ServeConfig` field
selecting `MockDashboardResourcesProvider`) is allowed by the brief but adds
surface; default this off if added.

## Phase 2 — Frontend refresh / live resource tree

Goal: after a workRoot is opened from the browser, the nav/resource tree shows
the real opened workRoot and stops showing mock workspace state.

1. Add a **minimal** open-workRoot affordance in `App.tsx` (left nav, near
   `ResourceNavigation`/`PanelHeader#L383-L427`). Minimum viable: a path-input
   "Open workRoot" control that POSTs `/api/dashboard/work-roots/open`. A small
   `/api/dashboard/root-picker` browser panel is acceptable but keep it narrow
   (no file-manager verbs) per the scope boundary. Route the click through a
   `data-command-id` (e.g. `workRoot.open`) consistent with existing command-id
   wiring.
2. On a successful open response, reconcile: call `loadResources()`
   (`App.tsx#L210-L230`) so the canonical endpoint becomes the source of truth.
   Optionally `setResources(openResponseView)` first for an immediate update,
   then `loadResources()` — the canonical endpoint stays authoritative for later
   refresh/re-entry.
3. Selection: the existing effect (`App.tsx#L246-L253`) already resets
   `selectedId` via `preferredSelection` when the prior selection leaves the
   entity set, so the mock workspace cannot remain selected after the tree turns
   live. Verify no separate stale-selection path remains; do not duplicate
   resources into other React state.
4. Empty state: `ResourceNavigation#L469-L471` already renders
   "Empty / no workspaces" when `workspaces.length === 0` — this is the honest
   pre-open live state; keep it.
5. Extract the testable seam: move `flattenEntities` + `preferredSelection`
   (and any selection reconcile helper) into a new pure module
   `frontend/src/resourceModel.ts` and re-export into `App.tsx`. This enables a
   pure-function test for the mock→live transition without React rendering
   machinery.

## Phase 3 — Dogfood evidence

New artifact:
`ai-docs/.plans/2026-05/16-260516-bug-ws-web-dashboard-live-resource-api-connection.dogfood.md`
(same directory and shape as
`16-260516-feat-ws-web-workroot-io-workbench-integration.dogfood.md`).

Record this daemon-served sequence:

1. `cd ws-dashboard && ./dev.sh run --port 8787` (production frontend build,
   daemon serves `frontend/dist` behind owner auth).
2. Pair via `/pair?token=...`; capture owner session cookie.
3. `GET /api/dashboard/resources` **before opening** — show `workspaces: []`
   (honest empty live view); confirm no mock `workspace-devenv` workspace.
4. `POST /api/dashboard/work-roots/open` with a real directory path
   (e.g. a temp dir or `/Users/kang-sw/devenv`); capture the opaque workRoot id.
5. `GET /api/dashboard/resources` **after opening** — confirm the opened
   workRoot id is present and the mock fixture workspace is absent.
6. Exercise the real workRoot id through `…/files`, `…/files/read`, terminal
   create/output/list/close, and a resources refresh — proving file explorer,
   read-only pane, and terminal flow run against the real workRoot.
7. If interactive browser/screenshot tooling is unavailable, record that as an
   explicit blocker section (as the prior artifact did), but still include the
   HTTP/curl evidence for every step above.

## Tests

Backend — `crates/daemon/tests/routes.rs`:

- Keep `dashboard_resources_api_is_owner_authenticated` (`#L431-L446`).
- Repurpose `dashboard_resources_api_returns_mock_hierarchy_with_owner_cookie`
  (`#L448-L540`) into:
  - `dashboard_resources_api_returns_empty_live_view_before_open` — paired GET
    yields `workspaces: []`, server present, and no `workspace-devenv`.
  - `dashboard_resources_api_includes_opened_work_root` — pair, open a real temp
    workRoot via `open_work_root_for_test` (`#L902-L929`), then GET resources;
    assert the opened workRoot id appears and the mock fixture workspace does not
    (the brief's required 1→2→3 sequence).
- If `open_work_root` now returns the aggregated view, extend an existing
  open-workRoot test to assert the response includes all registered roots.

Core — `crates/core`: `view_model.rs` serde tests unchanged. Add (or keep) a
unit test that `MockDashboardResourcesProvider` still deserializes
`dashboard_resources.json` so the golden artifact stays covered.

Frontend — new `frontend/src/resourceModel.test.ts`: prove `flattenEntities` +
`preferredSelection` transition from a mock-style `DashboardResourcesView` to a
live opened-workRoot view selects the live workRoot and does not retain the mock
workspace as the active selection. Add `src/resourceModel.ts` and
`src/resourceModel.test.ts` to `tsconfig.route-tests.json` `include`, and add a
`test:resource-model` script to `frontend/package.json` mirroring `test:routes`.

## Verification Commands

- `cd ws-dashboard && cargo fmt --all`
- `cd ws-dashboard && cargo clippy --workspace --all-targets` (or `cargo check --workspace`)
- `cd ws-dashboard && cargo test -p ws-dashboard-daemon`
- `cd ws-dashboard && cargo test -p ws-dashboard-core`
- `cd ws-dashboard/frontend && npm run build`
- `cd ws-dashboard/frontend && npm run test:resource-model` (new) and
  `npm run test:routes`
- `cd ws-dashboard && ./dev.sh test` (workspace cargo tests + frontend build) as
  the package-level check.
- Dogfood: `./dev.sh run --port 8787` sequence captured in the new
  `.dogfood.md` artifact.

## Opinion

- **Scope gap — no browser open flow exists.** The brief's Phase 2 / contract
  ("after a workRoot is opened from the browser flow") presumes a browser open
  affordance, but `App.tsx` has none and never calls `/api/dashboard/root-picker*`
  or `/api/dashboard/work-roots/open`. With Phase 1's empty-live default there
  will be *zero* nav rows and no way to bootstrap one from the browser. Phase 2
  must add at least a minimal "Open workRoot" control; the implementer should
  confirm with the lead how rich it may be, since the scope boundary excludes a
  "broad root-picker redesign" but a minimal opener is unavoidable for the
  contract to hold. A bare path-input opener is the lowest-risk choice.
- **`open_work_root` view inconsistency.** Today it returns only the single
  just-opened root while the live canonical endpoint (Phase 1) aggregates all
  opened roots. Returning the aggregated view from `open_work_root` (plan step
  P1.4) avoids a frontend reconcile that would otherwise drop previously opened
  roots if the frontend trusts the immediate response.
- **Mental-model drift.** `ai-docs/mental-model/ws-web-dashboard.md#L78` states
  "`/api/dashboard/resources` remains mock-backed" — that line and the related
  `{#260516-ws-web-dashboard-live-resource-authority}` /
  `{#…-open-workroot-resource-refresh}` /
  `{#…-live-resource-dogfood-verification}` "Planned 🚧" spec notes become stale
  on completion. Flag for the lead's spec/mental-model update step
  (lead-owned; not done here).
- **Determinism risk.** `OpenedWorkRoots` is a `HashMap`; without sorted
  `candidate_paths()` the aggregated route output order is nondeterministic and
  route tests will be flaky. Plan step P1.1 addresses this — do not skip it.
- The discovery provider already covers plain/git/moved/offline/inaccessible
  classification with tests (`discovery.rs#L347-L504`); Phase 1 is a wiring
  change, not new discovery logic — keep the change surgical.
