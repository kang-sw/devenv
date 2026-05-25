# Implementation Plan: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 7

## Scope

Implement only Phase 7 remote terminal WebSocket gatewaying. Preserve deferred
scope: larger terminal UX redesign, native-Windows control-key polish, generic
WebSocket proxying, document translation forwarding, agent controls, credential
persistence, deployment automation, and public endpoint hardening.

## Concrete File Map

- `ai-docs/spec/ws-web-dashboard/index.md` — Server-scoped gateway contract now
  treats terminal HTTP lifecycle as implemented and leaves terminal WebSocket
  gatewaying as the remaining planned route.
- `ai-docs/mental-model/ws-web-dashboard.md` — Modification rules require
  explicit server-scoped gateway aliases, serverId terminal identity, browser
  evidence for visible UI behavior, platform-explicit terminal tests, and no
  generic proxying.
- `ws-dashboard/crates/daemon/src/router.rs` — Protected router owns owner-auth
  and Host/Origin gated route registration. Phase 7 adds only the
  server-scoped terminal `/socket` route here.
- `ws-dashboard/crates/daemon/src/servers.rs` — Linked-server resolution,
  bounded refusal states, and server-scoped terminal HTTP aliases live here.
  Add the terminal WebSocket gateway helper beside the existing explicit
  gateway operations, not as a generic proxy.
- `ws-dashboard/crates/daemon/src/terminal.rs` — Legacy local terminal
  WebSocket handler owns the local PTY protocol and should remain the
  `server-local` path.
- `ws-dashboard/crates/daemon/src/auth.rs` — Auth middleware gates WebSocket
  upgrade requests before handler execution; do not weaken Host/Origin or owner
  session checks.
- `ws-dashboard/crates/daemon/Cargo.toml` and workspace dependencies — Runtime
  daemon code needs a WebSocket client dependency if the relay uses
  `tokio-tungstenite`; currently it is only a daemon dev-dependency.
- `ws-dashboard/crates/daemon/tests/routes.rs` — Existing local WebSocket tests
  cover legacy attach, input, resize, close, offline/unavailable, and auth
  gating. Add server-scoped local alias and linked gateway tests here.
- `ws-dashboard/frontend/src/terminals.ts` — `terminalWebSocketUrl` already
  accepts `serverId`; preserve the route helper shape.
- `ws-dashboard/frontend/src/App.tsx` — `TerminalPaneBody` already passes
  `pane.session.serverId` into `terminalWebSocketUrl`; browser evidence should
  prove the linked-server URL is used.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` — Add focused
  daemon-served browser evidence for linked-server terminal WebSocket behavior.

## Sequencing

1. **Add the dependency deliberately.** Move or add `tokio-tungstenite` to
   daemon runtime dependencies if used for upstream WebSocket connections.
   Avoid introducing a broader HTTP/WebSocket proxy library unless needed.
2. **Register one explicit route.** Add
   `/api/dashboard/servers/{server_id}/terminals/{terminal_id}/socket` to the
   protected router and import only the terminal WebSocket handler needed for
   this phase.
3. **Implement server-local alias dispatch.** For `server-local`, call the
   existing `terminal_websocket` handler with the extracted terminal id, query,
   and `WebSocketUpgrade`, preserving local pre-upgrade validation.
4. **Resolve linked servers before upgrade.** For linked servers, reuse
   `resolve_server_scoped_forwarding`. Unknown/auth-required/tunnel-required
   states should return bounded HTTP errors before accepting the browser
   WebSocket upgrade.
5. **Connect upstream with bearer auth.** Build the linked daemon legacy
   terminal socket URL from the stored endpoint and
   `/api/dashboard/terminals/{terminalId}/socket?after=...`, switching
   `http`/`https` to `ws`/`wss`. Include `Authorization: Bearer <token>` on the
   upstream WebSocket request.
6. **Handle upstream pre-upgrade failures.** If the upstream terminal WebSocket
   rejects before upgrade, return a bounded HTTP error before accepting the
   browser upgrade when practical. Preserve meaningful upstream status for
   terminal-specific 404/Gone style failures where the client can act on them.
7. **Relay frames without translating payloads.** After both sides are
   accepted, relay text, binary, ping, pong, and close frames in both
   directions. Do not parse or re-envelope terminal protocol JSON frames in the
   gateway.
8. **Clean up on either-side close.** When browser or upstream closes/errors,
   close the opposite side and end the task. Do not keep orphaned upstream
   terminal WebSockets or local browser sockets alive.
9. **Preserve HTTP fallback behavior.** Once the remote WebSocket connects, the
   existing frontend should suppress periodic HTTP output polling through
   `shouldPollTerminalOutput`; add tests only if a serverId gap appears.
10. **Add backend tests.** Cover protected auth/refusal, `server-local` alias
    parity, linked bearer upstream upgrade, output from upstream to browser,
    input and resize browser-to-upstream, upstream pre-upgrade error mapping,
    and cleanup when either side closes.
11. **Add browser evidence.** Add or update a Playwright case named around
    "linked server terminal WebSocket" that opens a remote terminal pane, proves
    the browser WebSocket URL is local gateway server-scoped, observes remote
    output, sends input/resize frames through the socket, and confirms legacy
    local terminal routes are not used.
12. **Run focused verification, then full smoke.** Use the brief's focused
    commands, full cargo, affected frontend tests, frontend build, and browser
    WebSocket gate.

## Backend Strategy

- Keep the relay private to terminal WebSocket forwarding. A helper may be
  route-neutral internally only if callers still pass an explicit terminal
  operation; do not expose a wildcard upgrade proxy.
- The linked-server resolver returns enough state for known refusal responses
  before browser upgrade. Network/upstream upgrade failures may require an
  upstream connect attempt before returning `upgrade.on_upgrade(...)`.
- The terminal protocol already uses JSON text frames for structured messages
  and binary frames as raw input. Relay payloads faithfully instead of parsing
  and reserializing them.
- Use existing terminal route access checks on the upstream daemon. The gateway
  should not maintain its own remote terminal registry or synthesize terminal
  ids.

## Frontend Strategy

- `terminalWebSocketUrl(terminalId, after, locationLike, serverId)` already
  generates `/api/dashboard/servers/{serverId}/terminals/{terminalId}/socket`
  for linked servers.
- `TerminalPaneBody` should keep using `pane.session.serverId` and
  `terminalWebSocketCursor(pane)` so remote sockets connect to the local
  gateway with the correct resume cursor.
- Browser evidence should assert server-scoped socket URL usage and live
  behavior. Avoid route mocks that hide whether a real browser WebSocket opened
  unless they also capture the browser WebSocket event.

## Tests

### Backend route tests

- Add unauthenticated server-scoped terminal socket coverage alongside existing
  terminal route auth tests.
- Extend server-scoped local terminal coverage so
  `/api/dashboard/servers/server-local/terminals/{terminalId}/socket` attaches
  and behaves like the legacy local route.
- Add linked-server tests with a mock upstream WebSocket server that verifies
  bearer auth and path/query forwarding.
- Assert upstream-to-browser output/status/exit frames pass through unchanged.
- Assert browser-to-upstream input and resize frames pass through unchanged.
- Assert upstream pre-upgrade rejection maps to a bounded HTTP error before
  browser upgrade when practical.
- Assert closing browser side closes upstream, and upstream close ends browser
  relay without hanging.

### Frontend and browser tests

- `npm --prefix ws-dashboard/frontend run test:terminals`: preserve helper URL
  coverage for server-scoped WebSocket routes.
- `npm --prefix ws-dashboard/frontend run test:commands`: preserve terminal
  create command `serverId` coverage.
- `npm --prefix ws-dashboard/frontend run test:browser -- -g "linked server terminal WebSocket"`:
  prove the visible remote terminal opens a WebSocket to the local gateway
  server-scoped route and live output/input/resize flow through that connection.

### Commands

Focused commands:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml server_scoped
cargo test --manifest-path ws-dashboard/Cargo.toml linked_server
cargo test --manifest-path ws-dashboard/Cargo.toml terminal
npm --prefix ws-dashboard/frontend run test:terminals
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:browser -- -g "linked server terminal WebSocket"
```

Final verification:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run test:terminals
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:browser -- -g "linked server terminal WebSocket"
```

## Browser Verification

- Browser evidence must prove the browser connects only to the local gateway
  server-scoped WebSocket URL for the linked server.
- Prefer a real daemon-served frontend with a local mock upstream WebSocket
  endpoint controlled by the backend or test harness. Do not commit endpoints,
  tokens, hostnames, or screenshots containing private paths.
- The existing full terminal browser gate proves local WebSocket terminal input
  fidelity; Phase 7 browser evidence should prove remote gateway routing and
  basic live relay behavior, not re-run every local terminal editing scenario.

## Risks

- **Upgrade timing risk**: accepting the browser upgrade before upstream
  refusal can turn actionable HTTP errors into opaque connected-then-closed
  sockets.
- **Scope risk**: a generic WebSocket proxy would expose unreviewed daemon
  routes under the server-scoped namespace.
- **Frame fidelity risk**: parsing/re-serializing terminal frames can alter raw
  input, binary data, close frames, or ping/pong behavior.
- **Cleanup risk**: one side closing must end the other side; otherwise remote
  PTY attachments can leak.
- **Dependency risk**: adding `tokio-tungstenite` as a runtime dependency should
  be deliberate and tested, because it was previously test-only.

## Lead Notes

- No lead decision is required if implementation stays within the explicit
  terminal WebSocket route and relay contract.
- Escalate before implementing generic upgrade proxying, remote terminal id
  synthesis, native-Windows control-key fixes, or public bind/security policy
  changes.
