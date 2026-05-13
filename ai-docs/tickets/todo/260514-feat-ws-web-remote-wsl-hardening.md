---
title: ws web remote and WSL hardening
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260513-research-streamable-http-mcp-transport: adjacent long-running daemon and remote transport research
  260514-feat-ws-web-server-link-forwarding: linked-daemon behavior to verify across WSL and remote environments
related-mental-model:
  - plugin-runtime
  - mcp-runtime
---

# ws web remote and WSL hardening

## Background

The primary workflow is personal use, but the dashboard should also work when
the daemon runs inside WSL or on a remote server and the browser connects
through a tunnel. This ticket verifies the operational envelope after the daemon,
workspace, frontend, terminal, agent, editor, and linked-server substrates exist.

## Phases

### Phase 1: Verify tunnel mode

Verify loopback daemon access through SSH port forwarding or equivalent tunnel
usage, including pairing, session cookies, WebSockets, and reconnect behavior.

### Phase 2: Verify WSL behavior

Check WSL folder paths, browser access from the Windows host, Git worktree
discovery, terminal behavior, browser-native editor behavior, and the preferred
linked-daemon path for WSL process visibility.

### Phase 3: Verify public-mode failure cases

Check that public bind requires explicit opt-in and authentication, and that
unsafe public startup attempts fail closed with actionable output.

### Phase 4: Record deployment guidance

Capture recommended local, tunnel, and public deployment commands plus known
limitations for future docs or a follow-up hardening epic.
