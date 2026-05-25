# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 3

## Intent

Make the connected linked-server folder/open-root affordance use the
server-scoped root picker and open WorkRoot flow through the local gateway. A
user should be able to open a root picker for a connected linked server,
navigate remote paths, create/pin/unpin remote directories, submit open
WorkRoot, and see the resulting WorkRoot selected under that linked server.

## Scope Boundary

Selected scope: Phase 3, "Remote root picker and open WorkRoot."

In scope:

- Connected linked-server row folder/open-root affordance opens a picker scoped
  to that server.
- Root picker list/navigate/create-directory/pin/unpin requests target the
  selected server through local gateway server-scoped routes.
- Open WorkRoot requests target the selected server through the local gateway.
- Successful remote open returns resources rewritten to the linked-server id and
  refreshes/selects that linked server.
- Server-local root picker/open WorkRoot behavior remains unchanged.
- Tests for remote route helper behavior, App/root-picker lifecycle, resource
  refresh/selection, and backend gateway coverage as needed.

Deferred:

- Credential persistence.
- Remote deployment automation.
- Public endpoint hardening.
- File/document/Activity/Git/terminal operation coverage beyond opening a
  remote WorkRoot.
- SSE and WebSocket forwarding.

## Caller-Visible Contract

The browser continues to call only the local gateway. When the user opens the
folder/open-root affordance for a linked server, all picker and open requests
carry that server identity. Remote paths are interpreted by the linked daemon,
not the local host. After a successful remote open, the dashboard resource tree
shows the opened WorkRoot under the selected linked server and selects it using
the daemon-returned opened WorkRoot id when available.

Local `server-local` picker/open behavior must remain compatible with the
existing local flow.

## Contract Instructions

Reuse Phase 1 frontend server-scoped helpers and Phase 2 protected backend
one-shot routes. Do not add browser direct calls to linked endpoints.

Reuse the resource refresh coordinator and existing opened-id header behavior.
Do not infer opened WorkRoot selection from display labels, paths, or row order.

Keep host paths out of command payload identity and final logs. UI may display
owner-visible remote paths in the picker, but command identities and persisted
state should remain server/path scoped as established by earlier phases.

## Integration Test Instructions

Focused test commands:

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

Final verification should include:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:root-picker
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:commands
```

Run `npm --prefix ws-dashboard/frontend run test:browser` if visible App/root
picker behavior changes are substantial enough to require browser-level proof.

## Implementation Strategy Decisions

- Use the selected server id as the root-picker session context.
- Let linked-server root picker operations ride Phase 2 server-scoped backend
  routes.
- Select remote opened WorkRoots from the returned rewritten resource view and
  opened-id header, not local registry state.
- Preserve local compatibility and avoid changing server-local UX except where
  required to share server-scoped helper call shapes.

## Rejected Alternatives

- Browser direct calls to linked daemon endpoints: rejected by the gateway
  product model.
- Treating remote open as a local registry mutation: rejected because linked
  daemon registry state is authoritative for remote roots.
- Deferring UI affordance wiring after backend forwarding: rejected because
  Phase 3's deliverable is user-visible remote root picker/open behavior.

## Approach

- Inspect current linked-server row actions and root-picker modal state.
- Thread selected server identity into root-picker modal requests and labels.
- Ensure open WorkRoot response handling refreshes/selects the selected linked
  server using rewritten resources.
- Add route/helper tests and App/root-picker tests for linked-server request
  targeting and selection.
- Run focused backend/frontend tests, then full cargo and frontend build smoke.

## Constraints

- All AI-authored docs and comments must be English.
- Do not implement file browsing/editing, Activity, Git, terminal, SSE, or
  WebSocket remote behavior in this phase.
- Do not claim native Windows dogfood unless actually executed against a remote
  Windows daemon; otherwise report automated local-gateway coverage only.

## Survey References

### Must

- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ws-dashboard/crates/daemon/src/servers.rs`
- `ws-dashboard/crates/daemon/src/root_picker.rs`
- `ws-dashboard/crates/daemon/src/resources.rs`
- `ws-dashboard/crates/daemon/src/persistent_state.rs`
- `ws-dashboard/frontend/src/resourceModel.ts`
- `ws-dashboard/frontend/src/rootPicker.ts`
- `ws-dashboard/frontend/src/openWorkRoot.ts`
- `ws-dashboard/frontend/src/resourceRefresh.ts`
- `ws-dashboard/frontend/src/App.tsx`

### Maybe

- `ai-docs/tickets/idea/260514-research-ws-web-dashboard-direction.md`
- `ai-docs/tickets/idea/260523-research-ws-dashboard-persistable-ui-state-map.md`
- `ai-docs/tickets/idea/260524-research-ws-dashboard-react-aria-ui-primitives.md`
