---
title: ws web dashboard MVP
related:
  260427-chore-claude-dash-windows: prior PTY dashboard surface and Windows stability motivation
  260513-research-streamable-http-mcp-transport: adjacent long-running daemon and remote transport research
  260513-feat-async-exec-output-reader: adjacent persisted process output and reader-agent pattern
related-mental-model:
  - developer-environment-tools
  - named-agent-runtime
  - mcp-runtime
  - plugin-runtime
---

# ws web dashboard MVP

## Scope

Build a personal ws-aware web dashboard MVP for controlling a host machine
through a browser UI. The primary target is one owner using local, WSL, or
remote-server sessions, not a multi-user web service.

The MVP should cover:

- A Rust Axum daemon that serves the web UI and owns host process lifecycle.
- Browser-based terminal panes backed by server-managed PTY processes.
- Host folder and Git worktree discovery so the owner can open a repo and start
  an agent-oriented workspace from the web UI.
- Read-only ws runtime state views backed by the existing wsstate layout under
  `~/.cache/ws@kang-sw-devenv/`, including named-agent status, current calls,
  recent events, and diagnostic tails where available.
- A dashboard layout that can show multiple workspaces, terminals, and agent
  views without depending on terminal-window behavior.
- Conservative authentication for every HTTP and WebSocket entrypoint, with
  permissive owner authorization after authentication.

## Non-Scope

- Multi-user accounts, teams, RBAC, tenancy, or browser-side permission roles.
- Replacing the existing MCP stdio plugin transport.
- Making the dashboard the canonical ws MCP session authority.
- Public internet deployment without explicit opt-in, authentication, and a
  TLS or reverse-proxy story.
- Full IDE/editor parity with VS Code.
- Direct browser interpretation of internal cache files as a stable API.

## Child Tickets

- Planned: daemon skeleton and security model for local, tunnel, and public bind
  modes.
- Planned: PTY session manager and xterm.js WebSocket bridge.
- Planned: workspace/repo picker with Git worktree discovery.
- Planned: wsstate-backed named-agent dashboard API and event stream.
- Planned: frontend MVP shell with terminals, workspace panels, agent views, and
  multi-dashboard layout, using `ai-docs/ref/design.md` as the initial visual
  direction reference.
- Planned: remote/WSL usage verification through SSH forwarding or equivalent
  loopback tunneling.

## Cross-Child Decisions

- Treat the product as a personal control plane: authentication is conservative,
  while an authenticated owner session has broad host-control authority.
- Require authentication for localhost as well as remote access. Preserve
  convenience through a `ws-web open` style command that creates a one-time
  pairing URL and then installs a normal session cookie.
- Prefer `127.0.0.1` bind for local and tunnel modes. Require explicit opt-in
  for `0.0.0.0` public bind and fail closed if public mode lacks authentication.
- Use Rust, Axum, Tokio, WebSockets, and a cross-platform PTY layer for the
  daemon. Use React, TypeScript, Vite, and xterm.js for the browser UI unless a
  later child ticket records a stronger reason to change stacks.
- Use CodeMirror 6 as the initial browser-native editor base, with Vim-like
  modal editing through an extension or custom ws modal layer. Keep terminal
  nvim available as an optional PTY workflow, but do not make it the primary
  editor path because Windows PTY behavior is a known stability concern.
- The daemon exposes stable view-model APIs over wsstate and wsagent behavior.
  The browser must not treat the cache layout itself as the public contract.
- Keep MCP root, harness, and session state scoped by project/worktree/session
  rather than making the web daemon a global authority over ws runtime state.
- Treat `ai-docs/ref/design.md` as the initial visual system reference for the
  web UI. Preserve its restrained, square-corner, hairline-driven operational
  style while adapting density and component choices for dashboard use rather
  than marketing-page composition.
- For frontend UI implementation delegated through ws named agents, register the
  implementer and reviewer with `model: "opus"` unless the user overrides that
  choice for a specific child ticket.

## Completion Criteria

- Done: child tickets deliver a usable authenticated local/tunnel web dashboard
  with PTY terminals, browser-native modal editing, workspace selection, and ws
  named-agent visibility.
- Dropped: a different host UI direction replaces the web dashboard approach or
  the MVP proves impractical for the intended personal workflow.
- Deferred: multi-user access, full public deployment hardening, desktop shell
  packaging, and complete IDE/editor features belong to later epics.
