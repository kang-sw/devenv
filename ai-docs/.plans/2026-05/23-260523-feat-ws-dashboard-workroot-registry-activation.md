# Survey: 260523-feat-ws-dashboard-workroot-registry-activation

## Reusable Components
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L19-L70` — `OpenedWorkRoots`: current in-memory opaque `WorkRootId -> PathBuf` registry with deterministic `candidate_paths`; useful migration input but currently only stores opened roots.
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L12-L55` — `DashboardStateStore`: existing daemon-local JSON state store with load/persist entrypoints for opened roots; likely successor/migration home for durable registry state.
- `ws-dashboard/crates/daemon/src/discovery.rs#L44-L79` — `LocalDashboardResourcesProvider`: converts local candidates into `DashboardResourcesView`; reusable discovery-to-view seam for deriving availability/kind from registry rows.
- `ws-dashboard/crates/daemon/src/discovery.rs#L204-L285` — `discover_work_root` path classifier: maps missing/moved/inaccessible/online status and error text without exposing paths; relevant to availability derivation.
- `ws-dashboard/crates/daemon/src/discovery.rs#L287-L340` — `GitDiscovery`: classifies `gitPrimaryRoot` vs `gitLinkedWorktree` using `git rev-parse`; reusable for availability/kind recompute while leaving linked worktree expansion out of scope.
- `ws-dashboard/frontend/src/commands.ts#L1-L38` — dashboard command union: central command id/payload surface to extend for workRoot activation controls.
- `ws-dashboard/frontend/src/commands.ts#L158-L199` — `dispatchDashboardCommand`/labels: existing observer+handler route used by clickable controls and command log.
- `ws-dashboard/frontend/src/resourceModel.ts#L126-L201` — `flattenEntities`, `preferredSelection`, `reconcileSelectedId`: pure resource model helpers that must carry new activation/availability fields into nav rows.
- `ws-dashboard/crates/daemon/src/mock.rs#L5-L18` — `MockDashboardResourcesProvider`: fixture-backed deterministic provider, explicitly not production; useful for model/fixture contract updates.

## Existing Patterns
- Live resources route: see `ws-dashboard/crates/daemon/src/resources.rs#L16-L45` — canonical `/api/dashboard/resources` runs live discovery on `spawn_blocking` from daemon state and must not use mock data.
- Open-root path: see `ws-dashboard/crates/daemon/src/root_picker.rs#L87-L130` — validates a single discovered root, registers it, persists state, then returns aggregated live resources.
- Startup restore: see `ws-dashboard/crates/daemon/src/server.rs#L58-L68` — daemon loads persisted state before building `AppState`; current behavior seeds `OpenedWorkRoots` only.
- Persistent JSON format tests: see `ws-dashboard/crates/daemon/src/persistent_state.rs#L177-L216` — local state tests cover dedupe, version, malformed/missing degradation.
- Route tests use paired-cookie router calls: see `ws-dashboard/crates/daemon/tests/routes.rs#L192-L224` and `ws-dashboard/crates/daemon/tests/routes.rs#L529-L643` — pattern for protected API assertions and resource JSON shape checks.
- Restart seed route coverage: see `ws-dashboard/crates/daemon/tests/routes.rs#L827-L874` — current test simulates persisted opened roots, reloads them, and asserts resources include remembered roots.
- File route gating baseline: see `ws-dashboard/crates/daemon/tests/routes.rs#L953-L1103` — tests auth, successful listing, traversal redaction, and unknown workRoot errors.
- Command-routed UI controls: see `ws-dashboard/frontend/src/App.tsx#L392-L420` and `ws-dashboard/frontend/src/App.tsx#L2228-L2240` — controls dispatch commands with `data-command-id` and handler injection rather than direct-only side effects.
- Resource row/detail rendering: see `ws-dashboard/frontend/src/App.tsx#L3412-L3477` and `ws-dashboard/frontend/src/App.tsx#L3525-L3583` — workRoot UI currently renders `status` as a single chip/detail field.

## Relevant Interfaces
- `ws-dashboard/crates/core/src/resources.rs#L5-L14` — `WorkRootStatus`: public serialized status currently covers `online`, `offline`, `moved`, `inaccessible`; brief says not to reuse this as activation.
- `ws-dashboard/crates/core/src/view_model.rs#L37-L49` — `WorkRootView`: public Rust view model has `kind`, `status`, state, instances, actions; expected location for distinct activation/availability fields.
- `ws-dashboard/frontend/src/resourceModel.ts#L43-L53` — `WorkRootView`: frontend model mirrors Rust and currently has only `status` for workRoot state.
- `ws-dashboard/crates/daemon/src/router.rs#L28-L36` — `AppState`: shared daemon state currently carries `opened_work_roots` plus `dashboard_state`; registry activation state will need to be available to resources/files/terminal/activity routes.
- `ws-dashboard/crates/daemon/src/router.rs#L54-L98` — protected route list: work-root open, files, terminals, Activity, and possible activation command routes are all nested under owner auth.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L126-L190` — file list/read routes resolve `opened_work_roots` first and return `unknown workRoot` for misses; this is the current gate to replace/augment.
- `ws-dashboard/crates/daemon/src/terminal.rs#L321-L365` — terminal create/list routes resolve `opened_work_roots`; create spawns a PTY under the resolved path.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L183-L257` — Activity snapshot/transcript/events routes resolve `opened_work_roots` before projecting wsstate.
- `ws-dashboard/crates/daemon/tests/fixtures/dashboard_resources.json#L30-L47` — golden fixture currently serializes workRoot `status` only; frontend/model contract tests will need fixture updates.
- `ws-dashboard/frontend/src/workRootFiles.ts#L80-L110` and `ws-dashboard/frontend/src/terminals.ts#L129-L157` — frontend file/terminal fetch wrappers surface backend error strings through shared error parsing.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L106-L133` requires known workRoots to stay visible, `availability` and `activation` to be separate public fields, offline activation to produce bounded offline route responses, degraded availability to produce bounded unavailable route responses, and activation transitions to be logical dashboard commands.
- `ai-docs/mental-model/ws-web-dashboard.md#L56-L76` keeps daemon resources owner-auth protected, browser resources sourced from `/api/dashboard/resources`, and workRoot controls command-routed.
- `ai-docs/mental-model/ws-web-dashboard.md#L120-L133` says changing resource vocabulary/view shape must update core structs, fixtures, mock provider, route tests, and frontend types together.
- `ai-docs/mental-model/ws-web-dashboard.md#L141-L145` warns against hashing canonical paths for remembered ids, duplicating fixtures in React state, using mocks for production resources, or returning only just-opened roots.
- `ai-docs/mental-model/ws-web-dashboard.md#L174-L176` requires file and Activity routes to avoid traversal/private path leaks and to keep Activity as read-only projection, not control authority.
- `ws-dashboard/crates/daemon/src/terminal.rs#L790-L827` validates terminal cwd hints as workRoot-relative and rejects outside/missing cwd; activation gating should preserve this route-local safety.

## Risk Signals
- `ws-dashboard/crates/daemon/src/persistent_state.rs#L58-L69` — Possible migration risk: state file is version 1 with only `{ path }`; registry migration must preserve existing restart behavior while adding activation/membership without losing v1 reads.
- `ws-dashboard/crates/daemon/src/discovery.rs#L120-L139` — Possible contract risk: `enabled` and action id currently derive from `WorkRootStatus::Online`; activation/offline controls could be accidentally conflated with availability if this logic is reused unchanged.
- `ws-dashboard/crates/daemon/src/work_root_files.rs#L131-L167` — Possible route-gating risk: known-but-offline and known-but-unavailable roots currently cannot be distinguished from unknown because resolution is only opened-map membership.
- `ws-dashboard/crates/daemon/src/terminal.rs#L321-L365` — Possible route-gating risk: terminal create/list use only opened-map membership; offline activation must not spawn/list as if online, and degraded availability must fail before PTY spawn.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs#L183-L257` — Possible route-gating risk: Activity routes use opened-map membership and then projection; known offline/unavailable states need bounded errors before wsstate/cache projection.
- `ws-dashboard/frontend/src/App.tsx#L3433-L3436` and `ws-dashboard/frontend/src/App.tsx#L3573-L3581` — Possible UI contract risk: workRoot rows/details display only current `status`; new fields must be visibly distinct enough for tests/users to tell activation from availability.
- `ws-dashboard/frontend/src/commands.ts#L1-L38` — Possible command risk: no activation command ids/payloads exist yet; direct click handlers would violate the command-routed control invariant.
- `ws-dashboard/crates/daemon/tests/fixtures/dashboard_resources.json#L169-L185` — Possible fixture risk: mock fixture already uses `status: offline` for an unavailable-looking root, which could mask accidental overloading of offline activation versus availability.

## Opinion
- The survey found enough concrete seams for implementation; no research escalation is needed.
- The largest ambiguity is compatibility shape: the brief allows retaining/deprecating `status`, but tests must make the new `activation` and `availability` authoritative.
