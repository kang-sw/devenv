# Brief: 260525-feat-ws-dashboard-server-scoped-operation-forwarding Phase 6

## Intent

Make terminal HTTP lifecycle operations transparent for linked-server WorkRoots
through explicit server-scoped local gateway routes. This phase lets the
browser create, list, poll output, send input, resize, and close a remote
terminal by talking only to the local dashboard gateway.

## Scope Boundary

Selected scope: Phase 6, "Remote terminal HTTP lifecycle."

In scope:

- Server-scoped terminal create and list routes for a WorkRoot.
- Server-scoped terminal output polling, input, resize, and close routes for a
  terminal id.
- `server-local` aliases that preserve existing local terminal behavior.
- Linked-server forwarding through the local gateway with bearer auth and
  bounded refusal/error behavior.
- Frontend terminal call sites, command payloads, panes, restore intents, and
  stale-output guards carrying `serverId` where ids can collide.
- Tests for backend forwarding/refusals/local aliases, frontend route helpers
  and fetch calls, terminal pane identity collisions, and browser or integration
  evidence that a remote terminal can be created and closed through the local
  gateway.

Deferred:

- Terminal WebSocket live transport and upgrade proxying.
- Larger terminal UX redesign.
- Native-Windows control-key polish beyond what HTTP lifecycle transparency
  requires.
- Agent control actions, document translation forwarding, credential
  persistence, deployment automation, and public endpoint hardening.

## Caller-Visible Contract

For a linked-server WorkRoot, terminal create/list/output/input/resize/close
requests go from the browser to `/api/dashboard/servers/{serverId}/...` on the
local gateway. The local gateway either dispatches `server-local` in-process or
forwards linked-server requests to the upstream daemon's legacy terminal HTTP
routes using the stored bearer token.

Terminal ids are opaque and not globally unique. Frontend panes, restore
intents, output polling, input, resize, and close operations must use
`serverId + terminalId` or `serverId + workRootId + terminalId` identity as
appropriate. Closing a remote terminal through the local gateway closes the
upstream terminal, not a local same-id terminal.

## Contract Instructions

Add only explicit protected server-scoped terminal HTTP aliases. Do not add the
server-scoped terminal WebSocket route in this phase, and do not introduce a
wildcard proxy.

Preserve existing local terminal route behavior, including owner auth,
workRoot activation/availability checks, JSON content-type boundaries for JSON
POST routes, terminal size validation, output cursor semantics, and close
termination semantics.

For linked servers, preserve the Phase 2/4/5 forwarding boundary: unknown,
unauthenticated, tunnel-required, or unreachable servers return bounded gateway
errors; forwarded requests carry the linked bearer token; upstream
status/body/content-type are preserved where practical.

Keep browser calls on the local gateway. The frontend must not construct or
call linked daemon endpoints directly.

## Integration Test Instructions

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

Final verification should include full cargo plus affected frontend tests and
the browser evidence required for visible terminal UI behavior:

```bash
cargo test --manifest-path ws-dashboard/Cargo.toml
npm --prefix ws-dashboard/frontend run test:terminals
npm --prefix ws-dashboard/frontend run test:commands
npm --prefix ws-dashboard/frontend run build
npm --prefix ws-dashboard/frontend run test:browser -- -g "linked server terminal HTTP lifecycle"
```

## Implementation Strategy Decisions

- Keep forwarding allowlisted and operation-specific.
- Use ordinary one-shot forwarding for terminal HTTP routes; leave WebSocket
  upgrade forwarding to Phase 7.
- Preserve local compatibility for omitted `serverId` and `server-local`.
- Treat returned terminal ids as upstream-owned opaque ids; the frontend adds
  selected server identity to its session state.
- Keep terminal WebSocket route helpers available in frontend from Phase 1, but
  do not register the backend server-scoped WebSocket route until Phase 7.

## Rejected Alternatives

- Generic `/api/dashboard/servers/{serverId}/*` proxy: rejected by the ticket
  and mental-model boundary.
- Implementing terminal WebSocket forwarding now: rejected as Phase 7 scope.
- Mapping remote terminal ids into local synthetic ids: rejected unless proven
  necessary; frontend server-scoped pane identity already prevents collisions.
- Native-Windows control-key dogfood as a blocker: rejected for this HTTP-only
  slice; Phase 7 and the existing Windows control-key ticket own live input
  fidelity.

## Approach

- Extend backend protected route registration for terminal HTTP aliases only.
- Extend `servers.rs` operation allowlist and local alias dispatch for
  create/list/output/input/resize/close.
- Preserve query strings for output polling and JSON body/content-type behavior
  for create/input/resize.
- Audit frontend terminal command creation and terminal operations so the
  selected WorkRoot or terminal pane server id is propagated everywhere.
- Add backend, frontend, and browser tests for remote route targeting,
  same-bare-id isolation, bounded failures, and close semantics.
- Run focused verification before full cargo/frontend build smoke.

## Constraints

- All AI-authored docs/comments must be English.
- Do not broaden WebSocket, agent control, translation, deployment, or public
  endpoint behavior.
- Do not record private endpoints, hostnames, paths, tokens, or sensitive
  screenshots in shared verification notes.

## Survey References

### Must

- `ai-docs/spec/ws-web-dashboard/index.md`
- `ai-docs/mental-model/ws-web-dashboard.md`
- `ws-dashboard/crates/daemon/src/router.rs`
- `ws-dashboard/crates/daemon/src/servers.rs`
- `ws-dashboard/crates/daemon/src/terminal.rs`
- `ws-dashboard/crates/daemon/tests/routes.rs`
- `ws-dashboard/frontend/src/terminals.ts`
- `ws-dashboard/frontend/src/terminals.test.ts`
- `ws-dashboard/frontend/src/commands.ts`
- `ws-dashboard/frontend/src/commands.test.ts`
- `ws-dashboard/frontend/src/App.tsx`
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`

### Maybe

- `ai-docs/ref/ws-dashboard-playwright.local.md`
- `ai-docs/tickets/todo/260517-bug-ws-dashboard-windows-terminal-control-keys.md`
