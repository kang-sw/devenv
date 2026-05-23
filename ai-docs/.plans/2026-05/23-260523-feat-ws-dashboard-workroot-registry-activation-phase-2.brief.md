# Brief: 260523-feat-ws-dashboard-workroot-registry-activation Phase 2

## Intent

Implement Phase 2 of the dashboard workRoot registry activation ticket: add
explicit resource refresh semantics and conservative bounded live availability
polling so known workRoots update their filesystem/Git availability while
preserving durable membership and user-controlled activation.

## Scope Boundary

Selected scope is Phase 2: Explicit refresh and bounded live status polling.

Implement a deterministic refresh path that recomputes availability for every
known workRoot without changing activation state. Add bounded polling while the
dashboard is open so external filesystem or Git worktree changes become visible
without making polling the only correctness path.

Leave linked-worktree discovery expansion, filesystem watcher integration,
forget/remove UX, read-only file pane restore, and broader UI persistence out of
scope.

## Caller-Visible Contract

Dashboard resource refresh recomputes live availability from filesystem/Git for
known workRoots and leaves each workRoot's activation unchanged.

Known workRoots stay visible after refresh or polling even when availability
changes to missing, moved, inaccessible, unknown, or back to available.
Activation remains the user-controlled online/offline targeting state.

While the dashboard is open, the frontend performs bounded availability polling
for resource state. Polling must be conservative and recoverable: explicit
refresh remains the deterministic path, polling does not become authority, and
poll failures must not erase the last known resource tree.

## Contract Instructions

Preserve the existing Phase 1 registry and activation contract. Do not collapse
availability into activation, do not hide offline or unavailable known roots,
and do not reintroduce opened-workRoot-only route authority.

Expected daemon surfaces include:

- `ws-dashboard/crates/daemon/src/resources.rs`
- `ws-dashboard/crates/daemon/src/discovery.rs`
- `ws-dashboard/crates/daemon/src/persistent_state.rs`
- `ws-dashboard/crates/daemon/src/root_picker.rs`
- route tests under `ws-dashboard/crates/daemon/tests/`

Expected frontend surfaces include:

- `ws-dashboard/frontend/src/resourceModel.ts`
- `ws-dashboard/frontend/src/App.tsx`
- frontend resource/open-workRoot tests that cover refresh reconciliation
- browser acceptance tests if visible resource status refresh behavior changes

Reuse the canonical `GET /api/dashboard/resources` live resource route for
refresh wherever possible. It is the daemon-owned source of truth for current
resource state. Browser state may select, render, and poll, but it must not
become durable membership or availability authority.

If an explicit refresh command/control is added or changed, route it through the
dashboard command dispatch path with a stable command id and logical target.
Mouse/click behavior must not bypass command dispatch.

Polling should be bounded by interval, in-flight request count, and failure
handling. It must avoid overlapping resource refresh requests and must stop or
be cleaned up when the dashboard surface unmounts. Large-registry backoff may be
minimal for this slice, but the implementation must not introduce an unbounded
tight loop.

Forbidden temporary wiring:

- no mock-only production refresh path
- no browser-local mutation of availability as source of truth
- no polling-only correctness where explicit refresh cannot recover
- no host paths in browser-visible errors or diagnostics
- no linked-worktree list expansion under this phase

## Integration Test Instructions

Add or extend integration coverage at the daemon/resource boundary and the
frontend refresh/polling boundary.

Required daemon assertions:

- known registry workRoots remain visible after availability recomputation
- refresh/resource loading preserves activation while availability changes
- missing or inaccessible known workRoots produce degraded availability without
  removing registry membership
- open-workRoot responses still reconcile through the aggregated canonical
  resources route behavior

Required frontend assertions:

- explicit resource refresh uses the canonical resource fetch path and preserves
  valid selection or drops only selections that leave the entity set
- polling is bounded: no overlapping resource refresh requests and cleanup on
  unmount
- polling failures keep the last known resource tree visible with a bounded
  error or stale state instead of clearing resources
- any visible refresh control dispatches a dashboard command id instead of a
  direct-only click side effect

Run at minimum:

- `cargo fmt --all --check`
- `cargo test -p ws-dashboard-daemon`
- `npm run build`
- relevant frontend resource/command tests changed by the implementation
- `npm run test:browser` if visible browser resource behavior changes

## Implementation Strategy Decisions

- Treat `GET /api/dashboard/resources` as the explicit refresh mechanism unless
  a small command wrapper is needed for frontend command routing.
- Keep availability recomputation on the daemon side through existing discovery
  classification and live resource building.
- Keep activation persisted and independent from recomputation.
- Implement polling as a frontend lifecycle behavior over the canonical
  resource endpoint, with overlap/failure cleanup guards.
- Prefer extending existing resource model and command tests over adding a
  second resource authority or a separate refresh model.

## Rejected Alternatives

- Filesystem watchers as the source of truth are rejected for this phase; later
  watchers may only become refresh-needed hints.
- Polling as the only correctness path is rejected; explicit resource refresh
  must remain deterministic.
- Browser-side availability inference is rejected because the daemon registry
  and discovery provider own resource authority.
- Expanding linked Git worktree discovery is rejected for this phase because it
  depends on, but is not part of, the registry activation spine.

## Approach

- Inspect the existing daemon live resources route and registry-backed
  discovery to confirm whether explicit refresh is already recomputing all
  known workRoots.
- Add or tighten daemon tests proving refresh recomputation preserves activation
  and membership across availability changes.
- Add frontend refresh/polling lifecycle around the canonical resource fetch,
  with request identity or in-flight guards to prevent stale or overlapping
  updates.
- Route any visible refresh affordance through `commands.ts`.
- Extend frontend tests for polling bounds, failure preservation, and selection
  reconciliation.
- Run focused Rust and frontend verification, then browser verification if UI
  behavior is visibly changed.

## Constraints

- Preserve owner-auth protections and opaque ids.
- Preserve Phase 1 terminal/file/Activity route access gates.
- Keep resource model authority in daemon state plus discovery.
- Keep polling conservative and stoppable.
- Do not expose host paths, Git internals, cache paths, pairing URLs, or
  daemon-private ids.

## Out of scope

- Linked Git worktree enumeration from `git worktree list`.
- Filesystem watcher implementation.
- Forget/remove/delete UX for known roots.
- Read-only file pane restore.
- Broader workbench layout persistence.
- Terminal PTY survival across daemon restart.

## Details

Availability changes should be observable through the existing public
`availability` field. Activation should remain whatever the durable registry
record says before and after recomputation.

Polling cadence is implementation-owned, but it should be clearly bounded and
testable. A short fixed interval with overlap prevention is acceptable if tests
prove cleanup and failure behavior.

Frontend refresh completion must guard against stale async results so a slower
poll response cannot overwrite a newer explicit refresh or root-opening
response.

## Verification Contract

Implementation is not complete until the changed daemon and frontend tests pass
and the required verification commands have been run. If browser-visible
resource refresh or status behavior changes, run the Playwright browser gate.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260523-dashboard-workroot-registry-activation`,
  `260516-ws-web-dashboard-resource-view-model-contract`,
  `260516-ws-web-dashboard-local-workroot-discovery-provider`, and
  `260516-ws-web-dashboard-open-workroot-resource-refresh`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard resource,
  persistent registry, command routing, route access, and browser verification
  invariants.
- [Must] `ai-docs/tickets/ready/260523-feat-ws-dashboard-workroot-registry-activation.md`
  - selected Phase 2 scope only; do not reopen completed Phase 1 plan text.
- [Maybe] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-workroot-file-listing-api` for route boundary
  precedent.
- [Maybe] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260521-ws-dashboard-activity-console-watch-stream` for bounded polling and
  snapshot refresh precedent.
