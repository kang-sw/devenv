---
title: ws web terminal WebSocket transport
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260516-bug-ws-web-dashboard-ui-acceptance-recovery: xterm surface recovery that remains merge-blocked by polling transport and input fidelity
  260516-feat-ws-web-terminal-session-substrate: completed daemon terminal lifecycle substrate currently exposed through HTTP routes
  260516-feat-ws-web-instance-event-stream: completed authenticated stream scaffold whose auth/stream boundaries inform WebSocket route behavior
spec:
  - 260516-ws-web-dashboard-terminal-websocket-transport
  - 260516-ws-web-dashboard-terminal-websocket-input-fidelity
  - 260516-ws-web-dashboard-terminal-websocket-browser-gate
skeletons:
  phase-1: 93a725f
related-mental-model:
  - ws-web-dashboard
---

# ws web terminal WebSocket transport

## Background

Dashboard terminal recovery added an xterm browser surface, but live terminal
I/O still uses HTTP input calls plus short-polled output. User dogfood shows
that this does not meet terminal expectations: responsiveness remains poor, and
interactive editing keys such as Backspace, cursor movement, and shell line
editing do not behave like a normal terminal.

This is not a cosmetic tuning problem. The dashboard MVP direction already
called for WebSockets with server-managed PTY processes, and the daemon
foundation only prepared WebSocket authentication boundaries without exposing a
live terminal WebSocket endpoint. The current polling path is acceptable as a
fallback/backfill substrate, but it is the wrong primary transport for an
interactive PTY terminal.

Do not merge the dashboard UI acceptance branch as a terminal-quality result
until this ticket or an equivalent live terminal transport recovery passes.

## Decisions

- The primary live browser terminal transport must be an owner-authenticated
  WebSocket, not HTTP output polling.
- The existing HTTP terminal output route may remain as a reload/reconnect
  backfill, deterministic test helper, or fallback path, but it must not be the
  normal live xterm output path when WebSocket is available.
- Terminal acceptance includes byte-stream input fidelity, not only colored
  output rendering. Backspace, cursor keys, Ctrl-key sequences, paste, prompt
  editing, and shell history/navigation must behave like a normal PTY-backed
  terminal.
- WebSocket conversion should happen before further polling responsiveness
  tuning. If terminal input remains wrong after WebSocket transport, the
  remaining issue is a true terminal wiring or PTY mode bug and should be
  debugged on top of the streaming transport.

## Constraints

- Preserve daemon-owned terminal lifecycle: create/list/resize/close remain
  daemon-owned, explicit close terminates the PTY, and browser attachment state
  does not own process lifecycle.
- Apply owner authentication, Host/Origin checks, and upgrade rejection before
  accepting any terminal WebSocket.
- Preserve opaque terminal ids and workRoot resource identity. Do not expose
  host paths, process ids, or raw PTY handles as browser authority.
- Preserve xterm.js as the browser terminal emulator surface.
- Preserve bounded resize behavior. Visual split dragging must not continuously
  rewrite PTY dimensions.
- Do not add agent presets, named-agent controls, a root picker redesign,
  write-back editing, or broad file-manager behavior in this ticket.

## Phases

### Phase 1: Add authenticated terminal WebSocket route

Add a daemon WebSocket endpoint for an existing terminal session. The route must
authenticate before upgrade acceptance, resolve only daemon-owned terminal ids,
and close cleanly when the terminal closes or the owner connection disconnects.

The protocol should carry ordered PTY output/status/exit data from daemon to
browser and raw input plus bounded resize requests from browser to daemon.
Choose text or binary frame shapes that preserve terminal byte semantics and
are testable without making browser routes or host paths authoritative.

Success means route tests cover unauthenticated rejection before upgrade,
unknown/closed terminal rejection, successful owner upgrade, input forwarding,
output forwarding, resize forwarding, and close behavior.

### Phase 2: Make xterm use WebSocket as the live path

Switch the browser terminal pane so an active xterm attaches to the terminal
WebSocket for live output, input, status, and resize messages. WebSocket frames
should feed `terminal.write(...)` directly and xterm `onData(...)` should send
raw terminal input over the socket without per-key HTTP requests.

The HTTP output route may be used for initial replay, reload reconstruction, or
fallback when WebSocket is unavailable, but the normal connected live terminal
must not depend on periodic output polling.

Success means the frontend no longer runs periodic output HTTP polling for a
terminal while its WebSocket is connected, and closing the terminal still
terminates the daemon PTY through the existing lifecycle contract.

### Phase 3: Recover interactive terminal input fidelity

Verify and fix terminal byte-stream behavior for ordinary shell editing and
interactive control keys. The acceptance set must include Backspace, left/right
cursor movement, command history navigation, Ctrl-C, Ctrl-D or EOF behavior
where safe, Ctrl-L or clear-screen behavior, paste, and ordinary prompt editing
inside a real shell.

Success means browser evidence shows the terminal behaves like a normal
PTY-backed xterm for those interactions. If one behavior remains platform- or
shell-specific, document the exact environment and residual constraint rather
than hiding it behind a passing output-rendering check.

### Phase 4: Browser gate and dogfood evidence

Extend the daemon-served browser acceptance gate and dogfood artifact for the
WebSocket terminal path. The gate must prove owner pairing, WebSocket
connection, no live output polling while connected, terminal input fidelity,
ANSI/control rendering, resize behavior, close-as-terminate, reconnect or
reload reconstruction, and no mock terminal surfaces.

Evidence should include measured local keystroke echo responsiveness or an
equivalent timing note sufficient to show the terminal is no longer bounded by
the old polling interval. Generated screenshots/traces remain gitignored; the
artifact records paths and pass/fail observations.

Success means the browser gate and dogfood evidence would have failed the
current polling-based terminal path for responsiveness or input fidelity, and
the branch can be reconsidered for merge only after those checks pass.
