---
title: ws web dashboard terminal session substrate
parent: 260516-epic-ws-web-dashboard-workroot-io-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workroot-io-substrate: containing milestone
  260516-feat-ws-web-instance-event-stream: existing authenticated event scaffold
  260516-epic-ws-web-dashboard-workbench-substrate: workbench placement substrate
spec:
  - 260516-ws-web-dashboard-terminal-registry-pty-spawn
  - 260516-ws-web-dashboard-terminal-io-transport
  - 260516-ws-web-dashboard-terminal-pane
  - 260516-ws-web-dashboard-terminal-close-termination
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard terminal session substrate

## Background

The dashboard needs live terminal use before it needs a harness-specific agent
UI. The terminal substrate should let the owner create shell terminals inside a
selected workRoot, interact with them through the browser, refresh the page
without losing live sessions, and explicitly close a session when it is no
longer needed.

## Decisions

- Implement shell terminal sessions first. Do not hardcode Codex, Claude, or
  other agent presets in this ticket.
- A terminal session is daemon-owned. Browser refresh or route re-entry must
  not destroy it.
- Closing a terminal panel terminates that terminal session. Do not create
  hidden detached terminal state or a restore list for closed terminals.
- Reserve confirmation hooks for close/terminate, especially for future
  foreground-process detection, but keep the first substrate focused on correct
  lifecycle and I/O.
- Terminal logical dimensions must not churn continuously during split dragging.
  Resize forwarding should be bounded and deliberate.

## Phases

### Phase 1: Daemon Terminal Registry And PTY Spawn

Add a daemon-owned terminal registry scoped by workRoot. It should create shell
PTY sessions with the selected workRoot as the run directory, list live
sessions for refresh restore, and close sessions explicitly. Session ids must be
opaque browser-facing ids, not process ids or host paths.

### Phase 2: Terminal I/O Transport

Add authenticated terminal output, input, and resize transport. The transport
may reuse the existing event envelope where suitable, but it must support input
and resize without pretending output-only polling is enough for an interactive
terminal. Unauthenticated callers must be rejected before stream or upgrade
acceptance.

### Phase 3: Frontend Terminal Pane

Render terminal sessions inside workbench panes with an xterm-style terminal
surface. Creating a terminal should open or focus a terminal pane for the
selected workRoot. Refresh should reconstruct panes from daemon live session
state and browser arrangement where possible.

### Phase 4: Close Semantics And Verification

Wire terminal close to session termination and keep detached restore UX absent.
Add tests and dogfood checks for create, stream, input, resize, refresh
persistence, explicit close, unauthenticated rejection, and narrow/wide layout
behavior.
