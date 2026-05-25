# Implementation Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 6

## Scope

Implement only Phase 6 remote terminal HTTP lifecycle forwarding through
explicit server-scoped local gateway routes. Preserve deferred scope: terminal
WebSocket gatewaying, larger terminal UX redesign, native-Windows control-key
polish, agent controls, document translation forwarding, credential
persistence, deployment automation, and public endpoint hardening.

## Concrete File Map

- `ai-docs/spec/ws-web-dashboard/index.md` — Contract requires server id in
  route/UI identity, local-gateway-only browser calls, explicit backend aliases,
  bounded linked-server refusals, and terminal HTTP lifecycle forwarding before
  WebSocket gatewaying.
- `ai-docs/mental-model/ws-web-dashboard.md` — Domain rules require browser
  evidence for visible UI work, terminal cross-platform caution, explicit
  server-scoped gateway aliases, and terminal lifecycle preservation.
- `ws-dashboard/crates/daemon/src/router.rs` — Protected route registration
  already has server-scoped aliases for root-picker/open/files/documents,
  Activity, Git, workspace, and Git worktree-add. Phase 6 adds only terminal
  HTTP aliases here and must not register `/socket` under
  `/api/dashboard/servers/{server_id}/...`.
- `ws-dashboard/crates/daemon/src/servers.rs` — `ServerScopedForwardOperation`
  is the allowlist, `forward_server_scoped_operation` is the ordinary linked
  HTTP path, `resolve_server_scoped_forwarding` centralizes unknown/auth/tunnel
  refusals, and existing local aliases show the JSON content-type preservation
  pattern.
- `ws-dashboard/crates/daemon/src/terminal.rs` — Owns terminal create/list,
  output polling, input, resize, WebSocket, close, request/query types,
  validation, workRoot access re-checks, and process termination.
- `ws-dashboard/crates/daemon/tests/routes.rs` — Existing terminal tests cover
  legacy auth, create/list/output/input/resize/close, and WebSocket boundaries.
  Existing server-scoped tests cover local aliases, linked forwarding, refusals,
  bearer propagation, upstream errors, and deferred route 404s.
- `ws-dashboard/frontend/src/terminals.ts` — Terminal endpoint helpers and fetch
  functions already accept optional `serverId`; this phase should harden fetch
  tests and session mapping for remote calls.
- `ws-dashboard/frontend/src/terminals.test.ts` — Existing route helper and
  identity tests cover server-scoped terminal URLs and same-id pane collisions;
  add fetch-call/body/error coverage where missing.
- `ws-dashboard/frontend/src/commands.ts` — `buildTerminalCreateCommand`
  carries `serverId`, but the payload union currently keeps terminal create as
  `{ type: "terminal.create"; workRootId: string }` with optional top-level
  `serverId`; preserve observer compatibility and add/adjust tests if command
  call sites change.
- `ws-dashboard/frontend/src/App.tsx` — Terminal list, restore, create, output
  polling, input, resize, and close mostly pass `serverId`; the Workbench
  toolbar "New terminal" button still needs auditing because one visible call
  builds the command with only `root.id`.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` — Owns daemon-served
  Playwright evidence for visible terminal behavior. Add a focused mocked
  linked-server terminal HTTP lifecycle case or a similarly strong integration
  gate.

## Sequencing

1. **Add explicit protected backend routes.** In `router.rs`, register:
   `/api/dashboard/servers/{server_id}/work-roots/{work_root_id}/terminals`
   for GET/POST, `/api/dashboard/servers/{server_id}/terminals/{terminal_id}/output`
   for GET, `/input` for POST, `/resize` for POST, and DELETE
   `/api/dashboard/servers/{server_id}/terminals/{terminal_id}`. Do not add the
   server-scoped `/socket` route in Phase 6.
2. **Extend the forwarding allowlist.** Add operation constructors in
   `servers.rs` for terminal list/create, output with preserved `after` query,
   input, resize, and close. Use `ForwardResponseRewrite::None`; frontend
   session mapping owns `serverId`.
3. **Implement server-local aliases first.** Dispatch `server-local` to
   `list_terminals`, `create_terminal`, `terminal_output`, `terminal_input`,
   `terminal_resize`, and `close_terminal`. For JSON POST routes, preserve the
   same Axum `Json` extractor boundary by requiring `application/json` before
   manual deserialization.
4. **Forward linked ordinary HTTP operations.** Route non-local operations
   through `forward_server_scoped_operation`, forwarding method, query,
   content-type, request body, status, body, and content-type. Terminal close
   must be forwarded upstream and must not remove a local same-id terminal.
5. **Keep WebSocket deferred.** Leave
   `/api/dashboard/servers/{server_id}/terminals/{terminal_id}/socket` as a
   protected-router miss. Frontend helper existence from Phase 1 does not imply
   backend Phase 6 support.
6. **Audit frontend command propagation.** Make visible terminal creation
   commands use the selected WorkRoot server id. Ensure create/list/output/input/
   resize/close continue to use the session or root `serverId` and never direct
   linked daemon endpoints.
7. **Harden same-id terminal state.** Preserve `terminalPaneLogicalKey`,
   `terminalPaneId`, restore intents, output polling, and close/error state as
   server-scoped. Add tests if the current coverage does not prove remote and
   local sessions with the same bare `terminalId` stay isolated.
8. **Add backend tests.** Cover auth/refusal for each new HTTP terminal alias,
   `server-local` parity with legacy terminal routes, linked-server bearer
   forwarding for all methods, upstream status/body/content-type preservation,
   JSON content-type rejection for local POST aliases, close forwarding, and
   the deferred server-scoped socket route remaining 404.
9. **Add frontend tests.** Extend terminal tests for `createTerminal`,
   `listTerminals`, `fetchTerminalOutput`, `sendTerminalInput`,
   `resizeTerminal`, and `closeTerminal` with remote `serverId`, request body,
   method, and local-gateway URL assertions. Extend command/App coverage for
   the visible toolbar create command server id if needed.
10. **Add browser/integration evidence.** Prefer a daemon-served Playwright case
    named around "linked server terminal HTTP lifecycle" that opens a linked
    WorkRoot fixture, clicks the visible "New terminal" control, verifies POST
    to the local gateway server-scoped terminal route, verifies the tab/session
    is keyed to the linked server, and closes it through the server-scoped
    DELETE route. A mocked linked-server route is acceptable if the evidence
    does not claim WebSocket/live shell forwarding.
11. **Run focused verification.** Run the brief's focused cargo/frontend
    commands, then full cargo and the browser command. Read outputs before
    claiming pass.

## Backend Strategy

- Ordinary terminal HTTP forwarding should stay allowlisted and route-specific;
  do not add a generic `/api/dashboard/servers/{server_id}/*` proxy.
- Local aliases should reuse terminal handlers so workRoot activation,
  availability, unknown terminal, invalid size, input-size, output cursor, and
  close-as-terminate semantics stay identical to legacy routes.
- For POST aliases, manual body parsing must not accidentally accept requests
  the legacy `Json` extractor rejects without `Content-Type: application/json`.
- Terminal output `after` is a query string and must be preserved through
  `legacy_path_with_query`.
- Terminal responses do not need linked-server resource rewriting; terminal
  session ids remain upstream-owned and the browser attaches selected
  `serverId`.

## Frontend Strategy

- `terminals.ts` helpers already construct local-compatible server-scoped
  routes. Add fetch-level tests so route helper coverage is not the only proof.
- `createTerminal` and `listTerminals` should continue normalizing remote
  sessions to include the selected `serverId`.
- `App.tsx` should pass `root.resourcePath.serverId` to visible terminal create
  command builders, matching restore flow and createTerminalPane behavior.
- Output polling must remain keyed by pane logical key and call
  `fetchTerminalOutput` with `pane.serverId`; same bare terminal ids on two
  servers must not share in-flight state or close errors.
- Phase 6 browser evidence should tolerate the absence of the server-scoped
  WebSocket route. It should prove HTTP lifecycle targeting, not live xterm I/O.

## Tests

### Backend route tests

- Extend `server_scoped_one_shot_routes_return_bounded_refusals` so Phase 6
  terminal HTTP routes are protected and return bounded linked-server refusals,
  while terminal socket remains in the deferred 404 list.
- Add or extend a local alias test for server-scoped terminal
  create/list/output/input/resize/close. Use platform-aware terminal commands
  if input/output are exercised.
- Add linked forwarding coverage with either a real remote test app or a focused
  mock app that verifies bearer auth, method/path/query/body/content-type, and
  upstream status/body/content-type preservation for create/list/output/input/
  resize/close.
- Add explicit tests that closing a linked terminal forwards DELETE upstream and
  leaves any local same-id terminal untouched when practical.
- Add JSON content-type boundary tests for server-local create/input/resize.

### Frontend tests

- `npm --prefix ws-dashboard/frontend run test:terminals`: add fetch-level
  remote tests for create/list/output/input/resize/close and preserve existing
  endpoint/key/restore tests.
- `npm --prefix ws-dashboard/frontend run test:commands`: assert terminal
  create command payload carries server id and that dispatch/observer behavior
  still works.
- Add App or browser coverage for the visible "New terminal" button using the
  selected remote WorkRoot server id.

### Commands

Focused commands:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml forwarding
cargo test --manifest-path ws-dashboard/Cargo.toml linked_server
cargo test --manifest-path ws-dashboard/Cargo.toml terminal
npm --prefix ws-dashboard/frontend run test:terminals
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
```

Final verification:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run test:terminals
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:browser -- -g "linked server terminal HTTP lifecycle"
```

## Browser Verification

- Browser evidence is required because Phase 6 changes visible terminal
  creation/close behavior for linked WorkRoots.
- Use daemon-served production frontend route mocks or an integration fixture so
  the browser still talks only to local gateway URLs under
  `/api/dashboard/servers/{serverId}/...`.
- Do not claim Phase 7 behavior. If a terminal pane attempts the server-scoped
  WebSocket helper and the backend still returns 404, the test should not treat
  that as live transport success; verify create/list/close HTTP lifecycle and
  server-scoped pane identity instead.
- Do not commit private endpoints, hostnames, paths, tokens, or sensitive
  screenshots.

## Risks

- **Scope risk**: registering the server-scoped terminal `/socket` route in
  Phase 6 would silently start Phase 7 without its upgrade/cleanup plan.
- **Content-type risk**: server-local aliases that manually parse JSON can
  broaden legacy behavior if they skip `application/json` validation.
- **Collision risk**: terminal ids are daemon-local. Any route, pane, restore,
  poll, input, resize, or close state keyed only by `terminalId` can target the
  wrong server.
- **Close risk**: linked close must be upstream-owned and must not terminate a
  local same-id terminal.
- **Evidence risk**: xterm live I/O depends on WebSocket behavior. Phase 6
  evidence should prove HTTP lifecycle targeting without claiming remote live
  shell input/output.

## Lead Notes

- No lead decision is required if implementation stays within explicit terminal
  HTTP aliases and frontend server-id propagation.
- Escalate before implementing WebSocket upgrade proxying, terminal id mapping,
  generic proxy behavior, credentials/deployment/public hardening, or native
  Windows control-key fixes.
