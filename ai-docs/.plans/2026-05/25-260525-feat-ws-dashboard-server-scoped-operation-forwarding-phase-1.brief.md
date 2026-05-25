# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 1

## Intent

Prepare the ws-dashboard frontend for server-scoped operations by making
`serverId` an explicit API-helper and frontend identity dimension before any
remote backend forwarding is added.

## Scope Boundary

Selected scope: Phase 1, "Frontend server identity and endpoint helpers", from
`ai-docs/tickets/ready/260525-feat-ws-dashboard-server-scoped-operation-forwarding.md`.

In scope:
- Frontend route helper APIs for the canonical server-scoped dashboard operation
  route shapes.
- Collision-safe frontend identities for workRoot/workspace/file/activity/
  terminal surfaces where the same bare id can exist on more than one server.
- Tests proving canonical route construction and identity collision behavior.
- Compatibility for current local behavior through `server-local` defaults or
  explicit local aliases.

Out of scope:
- Backend server-scoped route registration.
- Linked-server forwarding behavior.
- SSE proxying for document or Activity streams.
- Terminal WebSocket gatewaying.
- Browser-visible remote filesystem behavior.
- Credential persistence, remote deployment, public hardening, and federation.

## Caller-Visible Contract

The frontend exposes canonical helper functions for these route families:

```text
/api/dashboard/servers/{serverId}/root-picker
/api/dashboard/servers/{serverId}/root-picker/directories
/api/dashboard/servers/{serverId}/root-picker/pins
/api/dashboard/servers/{serverId}/work-roots/open
/api/dashboard/servers/{serverId}/workspaces/{workspaceId}/...
/api/dashboard/servers/{serverId}/work-roots/{workRootId}/...
/api/dashboard/servers/{serverId}/terminals/{terminalId}/...
```

Existing local helper behavior remains compatible with current backend routes
for `server-local`; this phase must not require backend routes that do not yet
exist. New helper APIs should accept `serverId` or a full `ResourcePath` where
that resource identity is available.

Frontend logical identities must not collapse resources from two servers that
reuse the same bare `workspaceId`, `workRootId`, `activityId`, or `terminalId`.
Persisted local-only state may remain readable as `server-local` state.

## Contract Instructions

- Reuse the existing `ResourcePath` type in `ws-dashboard/frontend/src/resourceModel.ts`.
- Prefer adding focused frontend identity/route helpers over constructing URLs
  inline in React components.
- Preserve existing local route constants or compatibility functions where
  current callers/tests depend on local-only backend routes.
- Do not add temporary mock-data, backend fallback, or generic proxy behavior.
- Do not put host paths into command payloads, localStorage identities, URLs
  beyond authenticated request bodies, or workbench logical keys.

## References

- [Must] `ai-docs/tickets/ready/260525-feat-ws-dashboard-server-scoped-operation-forwarding.md`
- [Must] `ai-docs/spec/ws-web-dashboard/index.md`
- [Must] `ai-docs/mental-model/ws-web-dashboard.md`
- [Must] `ws-dashboard/frontend/src/resourceModel.ts`
- [Must] `ws-dashboard/frontend/src/rootPicker.ts`
- [Must] `ws-dashboard/frontend/src/openWorkRoot.ts`
- [Must] `ws-dashboard/frontend/src/workRootFiles.ts`
- [Must] `ws-dashboard/frontend/src/workRootActivity.ts`
- [Must] `ws-dashboard/frontend/src/terminals.ts`
- [Must] `ws-dashboard/frontend/src/gitToolbar.ts`
- [Must] `ws-dashboard/frontend/src/gitWorktreeAdd.ts`
- [Must] `ws-dashboard/frontend/src/commands.ts`
- [Maybe] `ws-dashboard/frontend/src/App.tsx`
- [Maybe] `ws-dashboard/frontend/src/*test.ts`

## Integration Test Instructions

Run focused route/helper tests affected by the implementation:

```text
npm --prefix ws-dashboard/frontend run test:root-picker
npm --prefix ws-dashboard/frontend run test:open-work-root
npm --prefix ws-dashboard/frontend run test:work-root-files
npm --prefix ws-dashboard/frontend run test:work-root-activity
npm --prefix ws-dashboard/frontend run test:terminals
npm --prefix ws-dashboard/frontend run test:commands
```

Also run:

```text
npm --prefix ws-dashboard/frontend run build
```

Browser acceptance is not required for this phase unless visible UI behavior is
changed.

## Details

The implementation should cover representative helpers for root picker,
open-workRoot, workspace/Git worktree routes, Git toolbar routes, file/document
routes, Activity routes, and terminal HTTP/WebSocket route strings. Route tests
should assert correct encoding of `serverId` plus nested ids and query strings.

Identity tests should prove same bare ids on different servers produce
different keys for read-only file panes, terminal panes, Activity stream
requests/lookups, and any command payloads changed in this phase.

Existing local-only persisted state should not be made unreadable unless the
implementation includes a bounded compatibility migration to `server-local`.
