# Survey: 16-260516-bug-ws-web-terminal-websocket-transport

## Reusable Components
- `ws-dashboard/crates/daemon/src/router.rs#L31-L90` — `build_router`: terminal socket route is already mounted inside the protected owner-auth router at `/api/dashboard/terminals/{terminal_id}/socket`.
- `ws-dashboard/crates/daemon/src/router.rs#L125-L146` — `require_owner_auth`: distinguishes WebSocket upgrades by `Upgrade: websocket` and calls the pre-upgrade auth gate before handlers run.
- `ws-dashboard/crates/daemon/src/auth.rs#L144-L169` — `authenticate_browser_entrypoint` / `authenticate_websocket_upgrade`: shared owner session/bearer auth plus Host/Origin checks; missing headers are tolerated, invalid loopback boundaries reject with 403.
- `ws-dashboard/crates/daemon/src/auth.rs#L247-L337` — Host/Origin helpers: allow localhost/loopback authorities including bracketed IPv6 and reject non-loopback hosts/origins.
- `ws-dashboard/crates/daemon/src/terminal.rs#L28-L71` — `TerminalRegistry`: shared registry for live daemon PTY sessions; `get` returns the `Arc<TerminalSession>` used by HTTP and skeleton socket routes.
- `ws-dashboard/crates/daemon/src/terminal.rs#L121-L153` — `TerminalWebSocketServerMessage` / `TerminalWebSocketClientMessage`: skeleton JSON frame contract for output/status/exit and input/resize.
- `ws-dashboard/crates/daemon/src/terminal.rs#L199-L319` — terminal HTTP handlers: create/list/output/input/resize/close behavior and error responses to preserve for replay/fallback and lifecycle.
- `ws-dashboard/crates/daemon/src/terminal.rs#L411-L448` — `TerminalSession::write_input` / `resize`: existing bounded raw input and PTY resize operations reusable from WebSocket frames.
- `ws-dashboard/crates/daemon/src/terminal.rs#L450-L500` — terminal status transitions: terminate/error/exited drop writer/master/child and set lifecycle status.
- `ws-dashboard/frontend/src/terminals.ts#L26-L42` — TypeScript WebSocket frame types matching the Rust skeleton contract.
- `ws-dashboard/frontend/src/terminals.ts#L77-L104` — endpoint helpers including `terminalWebSocketEndpoint` and `terminalWebSocketUrl` with ws/wss selection from browser location.
- `ws-dashboard/frontend/src/terminals.ts#L231-L259` — output append and idle-poll predicate helpers; useful when converting WebSocket output into pane state and suppressing polling while attached.
- `ws-dashboard/frontend/src/App.tsx#L1441-L1592` — `TerminalPaneBody`: owns xterm construction, `onData`, output writes, fit/resize debounce, and terminate button wiring.
- `ws-dashboard/frontend/e2e/daemonHarness.ts` — daemon-served Playwright harness used by `npm run test:browser`; startup pairing URL and production static frontend are already handled there.

## Existing Patterns
- Protected routes stay behind owner middleware: see `ws-dashboard/crates/daemon/src/router.rs#L35-L85` — add/keep terminal WebSocket behavior under `protected`, not beside `/pair`.
- Terminal route tests use `tower::ServiceExt` against `build_router`: see `ws-dashboard/crates/daemon/tests/routes.rs#L97-L130` and `ws-dashboard/crates/daemon/tests/routes.rs#L1347-L1585` — helper patterns exist for pairing, opening a workRoot, creating a PTY, polling output, and asserting no host-path exposure.
- Auth gate tests for future WebSocket upgrades exist: see `ws-dashboard/crates/daemon/tests/routes.rs#L1812-L1830` and `ws-dashboard/crates/daemon/tests/routes.rs#L1894-L1907` — currently target unauthorized upgrade rejection before endpoint behavior.
- Frontend terminal reconciliation/listing is rooted in the selected workRoot: see `ws-dashboard/frontend/src/App.tsx#L866-L879` — live daemon terminal listing reconstructs panes after refresh.
- Current HTTP output polling loop is centralized: see `ws-dashboard/frontend/src/App.tsx#L881-L964` — connected WebSocket panes need to be excluded here rather than leaving periodic live polling active.
- Current xterm input path is raw `onData` to action callback: see `ws-dashboard/frontend/src/App.tsx#L1483-L1487` — no line buffering is currently inserted in the emulator.
- Resize forwarding is debounced and only records success: see `ws-dashboard/frontend/src/App.tsx#L1489-L1539` — the socket path should preserve this retryable bounded-resize behavior.
- Browser acceptance already drives a real daemon-served workRoot and PTY: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L105-L115` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L199-L223` — skeleton comments mark where WebSocket/no-poll/input-fidelity evidence belongs.
- Browser gate evidence is written outside tracked source: see `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L15-L23` and `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L54-L62`.

## Relevant Interfaces
- `ws-dashboard/crates/daemon/src/terminal.rs#L73-L90` — `TerminalSession` / `TerminalSessionInner`: private session state includes status, output buffer/cursor, writer, master, and child.
- `ws-dashboard/crates/daemon/src/terminal.rs#L92-L119` — `TerminalSessionView`, `TerminalOutputView`, `TerminalOutputChunk`: public JSON shapes for session state and HTTP replay/backfill.
- `ws-dashboard/crates/daemon/src/terminal.rs#L286-L308` — `terminal_websocket`: skeleton handler currently validates terminal id/live status before returning 501; this is the implementation hook.
- `ws-dashboard/crates/daemon/src/terminal.rs#L396-L409` — `TerminalSession::output_after`: ordered output backfill by cursor; likely needed for initial socket replay or reload fallback semantics.
- `ws-dashboard/crates/daemon/src/terminal.rs#L461-L476` — `append_output`: reader thread records output chunks and bounded history; no wakeup/notification mechanism exists yet.
- `ws-dashboard/crates/daemon/src/terminal.rs#L503-L517` — `spawn_reader`: blocking reader thread is the only PTY output producer today.
- `ws-dashboard/crates/daemon/src/terminal.rs#L520-L526` — `validate_size`: daemon PTY resize bounds are 1..=300 columns and 1..=120 rows.
- `ws-dashboard/frontend/src/terminals.ts#L44-L52` — `TerminalPaneState`: current pane state has no socket connection status field.
- `ws-dashboard/frontend/src/terminals.ts#L54-L75` — `terminalSizeBounds` / `clampTerminalSize`: frontend mirror of daemon PTY bounds.
- `ws-dashboard/frontend/src/terminals.test.ts#L43-L117` — helper tests cover endpoints, pane keys, poll-change behavior, close errors, and size validation; import list lacks the WebSocket URL helper today.
- `ws-dashboard/frontend/package.json#L7-L15` — verification scripts: `test:terminals`, `build`, and `test:browser` are the relevant frontend gates.
- `ws-dashboard/crates/daemon/Cargo.toml#L15-L15` — Axum is already compiled with the `ws` feature.

## Constraints
- `ai-docs/spec/ws-web-dashboard/index.md#L221-L228` — planned browser gate must prove WebSocket primary transport, no periodic output polling while connected, input fidelity, ANSI/control rendering, resize, close-as-terminate, reload reconstruction, and timing evidence.
- `ai-docs/spec/ws-web-dashboard/index.md#L341-L356` — HTTP output may remain only for initial replay, reload reconstruction, deterministic tests, or fallback; connected xterm path must not depend on periodic polling.
- `ai-docs/spec/ws-web-dashboard/index.md#L393-L399` — acceptance must include Backspace, cursor movement, history, Ctrl-C, Ctrl-D/EOF safe behavior, Ctrl-L/clear-screen, paste, and ordinary prompt editing.
- `ai-docs/mental-model/ws-web-dashboard.md#L65-L70` — authenticated browser requests and WebSocket upgrades pass conservative Host/Origin checks; missing headers are tolerated, clearly non-loopback hosts/origins fail before handler or upgrade.
- `ai-docs/mental-model/ws-web-dashboard.md#L85-L89` — dashboard daemon must not become ws MCP session authority.
- `ai-docs/mental-model/ws-web-dashboard.md#L126-L130` — terminal ids are opaque, sessions are daemon-owned live resources, and close must terminate/reap rather than detach.
- `ai-docs/mental-model/ws-web-dashboard.md#L159-L164` — terminal pane currently remounts xterm per tab and writes deltas from `pane.output`; switching tabs replays buffered output.
- `ai-docs/mental-model/ws-web-dashboard.md#L177-L179` — visual PTY size and logical daemon dimensions are separate; split drag should not continuously rewrite stable rows/columns.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L89-L94` — existing test helper assumes polling cycles; WebSocket timing assertions should update this stale assumption.

## Opinion
- Backend output fanout is the main hidden gap: `spawn_reader` only appends to a `VecDeque` and has no notify/broadcast path, so live sockets need a small session-local notification or channel design without taking over process lifecycle.
- The frontend has a clean socket insertion point (`TerminalPaneBody` plus terminal helpers), but `TerminalPaneState` needs an explicit connected/fallback state so the top-level poll loop can filter connected panes deterministically.
- The browser gate already contains TODO-style comments for this ticket; the risk is making assertions robust against real shell differences while still exercising byte-stream editing instead of fixture-only text checks.
