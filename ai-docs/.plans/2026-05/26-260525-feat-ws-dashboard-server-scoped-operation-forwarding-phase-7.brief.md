# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 7

## Intent

Forward live terminal WebSocket transport for linked-server terminals through
the local dashboard gateway. After Phase 7, a remote terminal pane connects to
the local gateway's server-scoped WebSocket route, and the gateway connects
upstream to the linked daemon with bearer auth.

## Scope Boundary

Selected scope: Phase 7, "Remote terminal WebSocket gatewaying."

In scope:

- Register the explicit server-scoped terminal WebSocket route:
  `/api/dashboard/servers/{serverId}/terminals/{terminalId}/socket`.
- Preserve `server-local` behavior by dispatching to the existing local
  `terminal_websocket` handler.
- For linked servers, proxy WebSocket upgrade and bidirectional frames between
  the browser and upstream linked daemon terminal WebSocket route.
- Apply the existing linked-server auth/refusal boundary before accepting the
  browser upgrade when the linked server is unknown, unauthenticated,
  tunnel-required, or unreachable.
- Connect upstream with the linked daemon bearer token and preserve the
  terminal WebSocket message protocol without translating frame payloads.
- Preserve cleanup when either browser or upstream side closes.
- Tests for upgrade auth/refusal states, bearer-auth upstream connection,
  bidirectional output/input/resize behavior, close/disconnect cleanup, and
  browser-visible remote terminal WebSocket evidence.

Deferred:

- Larger terminal UX redesign.
- Native-Windows control-key polish beyond evidence needed to prove remote live
  transport.
- Credential persistence, deployment automation, public endpoint hardening,
  document translation forwarding, and agent controls.

## Caller-Visible Contract

For a linked-server terminal, the browser opens a WebSocket only to the local
gateway route under `/api/dashboard/servers/{serverId}/terminals/{terminalId}/socket`.
The local gateway authenticates the owner, resolves linked-server bearer state,
then opens an upstream WebSocket to the linked daemon's legacy
`/api/dashboard/terminals/{terminalId}/socket` route. PTY output/status/exit
frames flow from upstream to browser, and raw input/resize frames flow from the
browser to upstream.

The browser must not call linked daemon endpoints directly. Terminal ids remain
upstream-owned opaque ids, and pane/session identity remains server-scoped.

## Contract Instructions

Do not add a wildcard WebSocket or HTTP proxy. The only upgrade route added in
this phase is the terminal WebSocket alias.

Do not buffer WebSocket traffic through HTTP polling helpers. Use a real
bidirectional WebSocket relay with bounded cleanup when either side closes.

For `server-local`, keep the existing local terminal WebSocket validation and
behavior. For linked servers, refusal responses should happen before accepting
the browser WebSocket upgrade when the gateway can determine the linked server
is unavailable.

## Integration Test Instructions

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

Final verification should include full cargo and the affected frontend/browser
commands.

## Implementation Strategy Decisions

- Keep server-scoped WebSocket routing explicit and terminal-only.
- Reuse existing linked-server resolution/refusal semantics.
- Use the existing upstream terminal WebSocket protocol without inventing a new
  frame envelope.
- Keep local browser terminal helpers unchanged unless tests expose a
  serverId-propagation gap.
- Treat native Windows live-terminal dogfood as useful evidence when feasible,
  but not as a blocker for the explicit route/proxy implementation.

## Rejected Alternatives

- Generic WebSocket proxying under `/api/dashboard/servers/{serverId}/...`:
  rejected by the ticket and mental-model boundary.
- Translating terminal frames into a new gateway protocol: rejected because the
  local terminal WebSocket message contract already exists.
- Mapping remote terminal ids into gateway-local ids: rejected unless a concrete
  cleanup/security bug appears; frontend server-scoped identity already
  prevents collisions.

## Approach

- Add server-scoped router registration for the terminal WebSocket alias.
- Add a linked-server WebSocket relay helper in the server gateway layer.
- Preserve `after` cursor query forwarding.
- Add backend tests for local alias behavior, linked refusal states before
  upgrade, bearer-auth upstream upgrade, bidirectional frames, and close/error
  cleanup.
- Extend browser acceptance with a linked-server terminal WebSocket case that
  proves the browser connects to the local gateway server-scoped URL and sees
  remote output/input behavior through the relay.
- Run focused verification, then full cargo/frontend/browser smoke.

## Constraints

- All AI-authored docs/comments must be English.
- Do not broaden translation, agent control, deployment, or public endpoint
  behavior.
- Do not record private endpoints, hostnames, paths, tokens, or sensitive
  screenshots in shared verification notes.

## Survey References

### Must

- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ws-dashboard/crates/daemon/src/router.rs`
- `ws-dashboard/crates/daemon/src/servers.rs`
- `ws-dashboard/crates/daemon/src/terminal.rs`
- `ws-dashboard/crates/daemon/src/auth.rs`
- `ws-dashboard/crates/daemon/tests/routes.rs`
- `ws-dashboard/frontend/src/terminals.ts`
- `ws-dashboard/frontend/src/App.tsx`
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`

### Maybe

- `ai-docs/ref/ws-dashboard-playwright.local.md`
- `ai-docs/tickets/todo/260517-bug-ws-dashboard-windows-terminal-control-keys.md`
