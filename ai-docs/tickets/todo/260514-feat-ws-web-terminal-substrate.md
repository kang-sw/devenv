---
title: ws web terminal substrate
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260427-chore-claude-dash-windows: prior PTY dashboard surface and Windows stability motivation
related-mental-model:
  - developer-environment-tools
---

# ws web terminal substrate

## Background

The dashboard still needs terminal panes for shell workflows, command output,
and optional terminal nvim sessions. Terminal support should be a panel
contribution over the frontend substrate and should not become the primary code
editor path.

## Phases

### Phase 1: Add PTY session manager

Create server-managed terminal sessions with spawn, resize, input, output,
close, restart, and cleanup behavior.

### Phase 2: Add xterm.js WebSocket bridge

Define the terminal WebSocket protocol and connect it to frontend terminal
panes, including authentication, resize events, reconnect behavior, and
backpressure handling.

### Phase 3: Add terminal panel integration

Register terminal panels and commands for new terminal, split terminal, rename,
close, restart, and workspace-local default shell selection.

### Phase 4: Verify cross-platform terminal behavior

Smoke local macOS/Linux behavior and record Windows/WSL risks separately from
the browser-native editor path.
