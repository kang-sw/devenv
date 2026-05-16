# Brief: 260516-bug-ws-web-terminal-websocket-transport

## Intent

Replace the dashboard terminal's live HTTP input plus output polling path with
an owner-authenticated WebSocket attachment for daemon-owned PTY sessions, and
prove in the browser gate that xterm behaves like a normal interactive
terminal for common editing/control keys.

## Scope Boundary

Implement the whole ready ticket, Phase 1 through Phase 4, on the current
implementation branch. This includes backend WebSocket route behavior, frontend
xterm attachment behavior, input fidelity fixes, browser acceptance coverage,
and dogfood evidence. Existing skeleton contracts from `93a725f` are binding.

## Caller-Visible Contract

- Authenticated owners can attach a browser WebSocket to an existing opaque
  terminal id at `/api/dashboard/terminals/{terminal_id}/socket`.
- The terminal WebSocket is the normal live path for xterm output, input,
  status, and resize while connected.
- Periodic HTTP output polling must not continue for a terminal while its
  WebSocket is connected.
- HTTP terminal output may remain for initial replay, refresh/reload
  reconstruction, deterministic tests, or fallback after WebSocket failure.
- The WebSocket must reject unauthenticated callers, invalid Host/Origin
  upgrade requests, unknown terminal ids, and closed terminal sessions before
  accepting an attachment.
- Browser terminal input must preserve byte-stream behavior for Backspace,
  left/right cursor movement, shell history navigation, Ctrl-C, safe Ctrl-D or
  EOF behavior, Ctrl-L or clear-screen behavior, paste, and ordinary prompt
  editing in a real shell.
- Closing a terminal through the existing lifecycle still terminates the
  daemon-owned PTY; a browser WebSocket attachment does not own process
  lifecycle.
- Browser evidence must show owner pairing, WebSocket connection, no live output
  polling while connected, ANSI/control rendering, resize behavior,
  close-as-terminate, reload reconstruction/backfill, no mock terminal surface,
  and keystroke echo no longer bounded by the prior polling interval.

## Implementation Strategy Decisions

- Preserve daemon-owned terminal lifecycle and existing create/list/resize/close
  routes.
- Preserve xterm.js as the browser emulator surface and keep xterm `onData`
  as the raw input source.
- Use the skeleton route path and frame concepts; normalize internals as needed
  without changing caller-visible route identity or the WebSocket-primary
  contract.
- Continue using opaque terminal ids and daemon-owned workRoot identity; do not
  expose host paths, process ids, PTY handles, or root paths to the browser.
- Keep resize bounded by the existing PTY size contract and avoid continuous
  logical resize during visual split drag.
- Keep browser gate artifacts generated outside tracked source; record paths
  and pass/fail observations in a dogfood artifact.

## Rejected Alternatives

- Further shortening the HTTP poll interval is rejected; it does not satisfy
  interactive PTY input expectations.
- A mock or fixture-only terminal path is rejected; acceptance must exercise a
  real daemon-served workRoot and PTY.
- Per-key HTTP input requests as the normal live path are rejected once the
  WebSocket is connected.
- New agent presets, named-agent controls, a root picker redesign, write-back
  editing, and broad file-manager behavior are out of scope.

## Approach

- Implement the backend terminal WebSocket handler from the skeleton: authenticate
  before upgrade, resolve only live daemon terminal sessions, forward PTY output
  and lifecycle status to the socket, and forward raw input plus bounded resize
  messages to the session.
- Add backend route tests for unauthenticated rejection before upgrade,
  Host/Origin rejection, unknown/closed terminal rejection, successful owner
  upgrade, input forwarding, output forwarding, resize forwarding, and clean
  close behavior.
- Implement frontend WebSocket connection helpers and terminal-pane state so a
  connected xterm consumes WebSocket output directly and sends xterm `onData`
  plus resize messages over the socket.
- Keep HTTP output replay/backfill for reload or fallback, but stop periodic
  output polling while a live socket is connected.
- Extend TypeScript helper tests for endpoint/protocol/state behavior.
- Extend the daemon-served Playwright browser gate with WebSocket/no-polling
  assertions, terminal input fidelity interactions, timing evidence, reload
  reconstruction, resize, ANSI/control rendering, and close-as-terminate checks.

## Constraints

- Owner auth, Host checks, and Origin checks must happen before WebSocket upgrade
  acceptance.
- Browser attachment state must not terminate or own the daemon PTY except when
  the user invokes the existing explicit close lifecycle.
- The implementation must not make the dashboard daemon the ws MCP session
  authority.
- Preserve existing dashboard frontend layout and file explorer behavior; this
  task is terminal transport and fidelity only.
- Avoid broad refactors unless they are required to make the WebSocket terminal
  contract correct and testable.

## Out of scope

- Multi-user authorization, RBAC, public internet hardening beyond the existing
  dashboard auth boundary.
- Agent preset UI, named-agent spawning controls, editor write-back behavior,
  root-picker UI redesign, and general file-manager operations.
- Replacing xterm.js or the existing daemon terminal registry.

## Verification Contract

- Backend route/unit tests covering WebSocket auth, terminal id/state rejection,
  input/output/resize forwarding, close behavior, and no host path exposure.
- Frontend TypeScript tests covering WebSocket endpoint/protocol/state helpers
  and no-live-polling state transitions.
- Production frontend build.
- Daemon-served Playwright browser gate via `npm run test:browser`.
- Terminal-focused frontend tests via `npm run test:terminals`.
- Cargo workspace or daemon-specific tests sufficient to prove backend route and
  terminal substrate behavior.
- Dogfood evidence artifact recording the browser command, viewports,
  WebSocket/no-polling observations, input fidelity checks, timing note,
  screenshots/traces paths if generated, and pass/fail observations.

## References

- [Must] `ai-docs/spec/ws-web-dashboard/index.md` —
  `260516-ws-web-dashboard-terminal-registry-pty-spawn`,
  `260516-ws-web-dashboard-terminal-io-transport`,
  `260516-ws-web-dashboard-terminal-websocket-transport`,
  `260516-ws-web-dashboard-browser-terminal-emulator-behavior`,
  `260516-ws-web-dashboard-terminal-websocket-input-fidelity`,
  `260516-ws-web-dashboard-browser-ui-acceptance-gate`, and
  `260516-ws-web-dashboard-terminal-websocket-browser-gate`.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - dashboard owner-auth
  boundary, Host/Origin checks, terminal route ownership, and browser-shell
  constraints.
- [Must] `ws-dashboard/crates/daemon/src/router.rs` - protected route mounting
  and owner-auth middleware boundary.
- [Must] `ws-dashboard/crates/daemon/src/auth.rs` - owner auth plus Host/Origin
  upgrade gate behavior.
- [Must] `ws-dashboard/crates/daemon/src/terminal.rs` - daemon terminal
  registry, PTY input/output/resize lifecycle, and skeleton WebSocket route.
- [Must] `ws-dashboard/crates/daemon/tests/routes.rs` - backend route/auth and
  terminal lifecycle tests.
- [Must] `ws-dashboard/frontend/src/terminals.ts` - terminal endpoint, protocol,
  and pane state helpers.
- [Must] `ws-dashboard/frontend/src/App.tsx` - xterm attachment, output polling,
  input, resize, and pane lifecycle wiring.
- [Must] `ws-dashboard/frontend/src/terminals.test.ts` - terminal helper tests.
- [Must] `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` -
  daemon-served browser acceptance gate.
- [Maybe] `ai-docs/spec/ws-web-dashboard/index.md` -
  `260516-ws-web-dashboard-terminal-pane` and
  `260516-ws-web-dashboard-workroot-io-dogfood-verification`.
