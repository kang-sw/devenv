# Implementation Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 3

## Scope

Implement only Phase 3 remote root picker and open WorkRoot behavior. Preserve deferred scope: credential persistence, deployment automation, public endpoint hardening, file/document/Activity/Git/terminal operation coverage, SSE forwarding, and terminal WebSocket gatewaying.

## Concrete File Map

- `ws-dashboard/frontend/src/App.tsx#L1191-L1468` — `OpenWorkRootControl` owns root-picker modal lifecycle; it currently calls `fetchRootPicker`, `requestOpenWorkRoot`, create, pin, and unpin without a server context and labels the affordance as host-local.
- `ws-dashboard/frontend/src/App.tsx#L1492-L1821` — Root picker UI, places list, navigation controls, create folder, pin/unpin, and submit buttons all share the same modal state and command-dispatched handlers.
- `ws-dashboard/frontend/src/App.tsx#L2395-L2585` — `ResourceNavigation`/`ServerRows` render server rows and the connected linked-server `openRoot` icon; this is the handoff point for passing the clicked server id into `OpenWorkRootControl`.
- `ws-dashboard/frontend/src/App.tsx#L345-L475` — App resource state, selected server id, refresh coordinator, and `handleWorkRootOpened`; successful remote open should apply the rewritten response, select the returned opened WorkRoot id, refresh servers/resources, and keep `selectedServerIdRef` aligned with `openedView.server.id`.
- `ws-dashboard/frontend/src/resourceRefresh.ts#L14-L29` — `requestDashboardResources(serverId)` already fetches `/api/dashboard/servers/{serverId}/resources` for linked servers; reuse through the coordinator instead of adding direct remote calls.
- `ws-dashboard/frontend/src/rootPicker.ts#L52-L155` — Phase 1 helpers already accept optional `serverId` for list/create/pin/unpin and route non-local ids through `/api/dashboard/servers/{serverId}/root-picker...`.
- `ws-dashboard/frontend/src/openWorkRoot.ts#L11-L47` — `requestOpenWorkRoot(path, serverId)` already supports server-scoped open and returns `x-ws-dashboard-opened-work-root-id`; the UI must supply the selected linked-server id.
- `ws-dashboard/frontend/src/resourceModel.ts#L58-L73` — `localCompatibleDashboardApiRoute` and `serverScopedIdentity` define local-compatible route shapes and scoped local state keys.
- `ws-dashboard/crates/daemon/src/router.rs#L95-L135` — Protected server-scoped aliases for root picker, root-picker mutations, and open WorkRoot are already registered beside legacy local routes.
- `ws-dashboard/crates/daemon/src/servers.rs#L477-L650` — Phase 2 forwarding helper maps server-scoped root-picker/open WorkRoot aliases to legacy upstream routes and dispatches `server-local` in-process.
- `ws-dashboard/crates/daemon/src/servers.rs#L702-L735` and `#L937-L959` — Forwarded open WorkRoot responses preserve the opened-id header where available and rewrite `DashboardResourcesView` server ids to the linked server.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L112-L277` — Server-local root picker/open WorkRoot behavior and opened-id header contract; Phase 3 must keep this unchanged.
- `ws-dashboard/frontend/src/rootPicker.test.ts#L59-L89` — Existing route-helper tests cover local-compatible and linked-server root picker endpoints.
- `ws-dashboard/frontend/src/openWorkRoot.test.ts#L83-L126` — Existing open helper tests cover server-scoped endpoint encoding and same-label opened-id header selection.
- `ws-dashboard/frontend/src/resourceRefresh.test.ts#L117-L190` — Existing coordinator tests cover stale poll behavior and external open-response application.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2271-L2655` — Existing backend tests cover protected server-scoped local aliases, bounded refusals, forwarding, resource rewriting, and opened-id header preservation.

## Sequencing

1. **Thread server context into the open-root affordance.** Add an optional `server`/`serverId` prop to `OpenWorkRootControl` in `ws-dashboard/frontend/src/App.tsx#L1191-L1204`, pass `server.id` from `ServerRows` at `ws-dashboard/frontend/src/App.tsx#L2577-L2585`, and keep the default `server-local` path for any existing local/section usage.
2. **Scope all picker operations.** Update `OpenWorkRootControl` calls at `ws-dashboard/frontend/src/App.tsx#L1222-L1246`, `#L1360-L1393`, and `#L1400-L1438` to pass the selected server id into `fetchRootPicker`, `createRootPickerDirectory`, `pinRootPickerDirectory`, and `unpinRootPickerDirectory`.
3. **Scope open WorkRoot submission.** Update `requestOpenWorkRoot` use at `ws-dashboard/frontend/src/App.tsx#L1327-L1358` to pass the selected server id and keep selection based on `result.openedWorkRootId`, not label/path/order.
4. **Reset picker session on server changes.** In `OpenWorkRootControl`, ensure opening the picker for a new server starts a fresh picker session (`pickerView`, path fields, history, errors, pending path state) so a prior local or different linked-server path is not reused against the new server.
5. **Make connected linked-server UX explicit.** Adjust title/context strings around `ws-dashboard/frontend/src/App.tsx#L1453-L1523` to indicate local host vs selected linked server without exposing endpoint hints or private transport details; owner-visible picker paths from the remote daemon may still display inside the modal.
6. **Preserve selected-server refresh flow.** Keep `handleWorkRootOpened` in `ws-dashboard/frontend/src/App.tsx#L446-L475` as the reconciliation point; if changes are needed, keep `selectedServerIdRef.current = openedView.server.id`, `applyExternalResources(openedView)`, opened-id selection, `loadServers()`, and `loadResources("open")` in that order.
7. **Backend only if tests expose a gap.** Prefer no backend changes because Phase 2 routes exist in `ws-dashboard/crates/daemon/src/router.rs#L95-L135` and `ws-dashboard/crates/daemon/src/servers.rs#L539-L609`; if a gap appears, keep changes inside the existing allowlisted one-shot forwarding helper and do not add generic proxy/SSE/WebSocket coverage.
8. **Keep command payloads path-safe.** Existing command builders still receive paths for labels/command logging; do not add server endpoint, SSH target, bearer token, or host transport fields to command payloads or logs.

## Tests

- `ws-dashboard/frontend/src/rootPicker.test.ts#L59-L89` — Extend helper-level coverage if route helpers change; assert remote list/create/pin/unpin continue targeting `/api/dashboard/servers/{serverId}/root-picker...` while `server-local` uses legacy local paths.
- `ws-dashboard/frontend/src/openWorkRoot.test.ts#L83-L126` — Keep/extend same-label opened-id header coverage for server-scoped open; assert the wrapper sends the linked server route and returns the header id unchanged.
- `ws-dashboard/frontend/src/resourceRefresh.test.ts#L117-L190` — Add/adjust coordinator coverage if `handleWorkRootOpened` behavior changes; prove external open response prevents stale poll overwrite and selected server resource refresh uses the linked-server id.
- `ws-dashboard/frontend/src/App.tsx#L1191-L1821` — Add App/root-picker lifecycle tests in the existing frontend test style or a focused new test if available: clicking a connected linked-server open-root icon calls remote list, navigate, create, pin/unpin, and open endpoints with that server id; clicking the local server remains compatible.
- `ws-dashboard/frontend/src/App.tsx#L446-L475` — Test that successful remote open applies rewritten resources, selects `openedWorkRootId`, and leaves the opened WorkRoot under the clicked linked server.
- `ws-dashboard/crates/daemon/tests/routes.rs#L2271-L2655` — Extend backend coverage only if source changes: linked root-picker list/create/pins/open requests should still forward bearer auth, preserve bounded upstream errors, rewrite resource views on open, and preserve the opened-id header.

Focused commands from the brief:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml linked_server
cargo test --manifest-path ws-dashboard/Cargo.toml forwarding
cargo test --manifest-path ws-dashboard/Cargo.toml root_picker
npm --prefix ws-dashboard/frontend run test:root-picker
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
```

Final verification:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:root-picker
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:commands
```

## UI / Browser Verification

- `ai-docs/mental-model/ws-web-dashboard.md#L10-L13` — Browser-visible dashboard UI changes require browser-level visual/interaction verification; run `npm --prefix ws-dashboard/frontend run test:browser` if the root-picker UI/selection behavior changes beyond pure route plumbing.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` — If browser coverage is added, exercise the daemon-served production frontend: pair, select a connected linked server fixture if available, open its root picker, navigate/create/pin/unpin/open, and assert the new WorkRoot appears beneath the linked server.
- `ws-dashboard/frontend/e2e/daemonHarness.ts` — Reuse the existing Playwright daemon harness; if no real remote daemon fixture exists, report local-gateway automated coverage only and do not claim native remote/Windows dogfood.

## Risks

- `ws-dashboard/frontend/src/App.tsx#L1222-L1438` — Server-context risk: the current modal methods omit `serverId`, so a linked-server row can accidentally operate on the local filesystem unless every picker/open helper call is threaded.
- `ws-dashboard/frontend/src/App.tsx#L1453-L1523` — Stale-session risk: reopening the same modal for a different server can reuse path/history/state from a previous server unless state reset is tied to the picker server context.
- `ws-dashboard/frontend/src/App.tsx#L446-L475` — Selection risk: fallback selection from resource diffs can be ambiguous; remote open should use the daemon-returned opened-id header whenever present.
- `ws-dashboard/crates/daemon/src/servers.rs#L477-L650` — Scope risk: Phase 2 backend forwarding is deliberately allowlisted one-shot HTTP/JSON; do not broaden it into file/document/Activity/Git/terminal/SSE/WebSocket forwarding while wiring the UI.
- `ws-dashboard/crates/daemon/src/servers.rs#L702-L735` — Header/rewrite risk: forwarded open must preserve `x-ws-dashboard-opened-work-root-id` and rewrite resource server ids, or the frontend can select the wrong tree or treat remote roots as local.
- `ws-dashboard/crates/daemon/src/root_picker.rs#L145-L205` — Persistence-boundary risk: remote pin/unpin persists in the linked daemon, not local state; frontend tests should assert route targeting rather than local pinned state side effects.
- `ws-dashboard/frontend/src/App.tsx#L3536-L3826` and `#L3843-L3944` — Deferred-scope risk: Activity/document SSE and terminal polling/WebSocket paths already use server ids in helpers, but Phase 3 must not attempt remote stream/gateway coverage.

## Lead Notes

- No lead decision is required if the implementation stays limited to passing server identity through the existing root picker/open WorkRoot helpers and Phase 2 one-shot gateway routes.
- Escalate before implementation if a real browser-level remote linked-server fixture is required, because the brief allows automated local-gateway coverage and explicitly forbids claiming native Windows/remote dogfood without executing it.
