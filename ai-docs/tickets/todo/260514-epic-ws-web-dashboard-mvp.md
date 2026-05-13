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

- `260514-feat-ws-web-daemon-foundation` - daemon shell, owner auth,
  WebSocket auth, and local/tunnel/public bind-mode guards. First substrate.
- `260514-feat-ws-web-frontend-substrate` - extension-ready React shell, panel
  and command registries, dock layout, design primitives, and mock/live data
  boundary. Depends on daemon serving shape and server/workspace/instance scope;
  UI work uses `model: "opus"` for delegated ws agents.
- `260514-feat-ws-web-workspace-substrate` - host folder selection, Git
  root/worktree discovery, recent workspace state, opaque workspace ids, and
  workspace boundary model, including flat navigation entries for worktrees.
- `260514-feat-ws-web-terminal-substrate` - PTY session manager, xterm.js
  WebSocket bridge, and terminal panel contribution.
- `260514-feat-ws-web-agent-dashboard-substrate` - wsstate-backed agent status,
  current-call, event, and diagnostic view-model APIs plus panels.
- `260514-feat-ws-web-editor-substrate` - CodeMirror 6 browser-native editor
  substrate with Vim-like modal editing and future editor extension hooks. UI
  work uses `model: "opus"` for delegated ws agents.
- `260514-feat-ws-web-server-link-forwarding` - authenticated daemon-to-daemon
  linking and forwarding so local, WSL, or remote ws web servers can appear in
  one dashboard without making native Windows process scraping the primary
  integration path.
- `260514-feat-ws-web-remote-wsl-hardening` - remote tunnel, WSL, and public
  bind verification after the core substrates and linked-server behavior exist.

## Cross-Child Decisions

- Treat the product as a personal control plane: authentication is conservative,
  while an authenticated owner auth session has broad host-control authority.
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
- Model dashboard resources from the start as `server -> workspace -> instance`.
  A server is a physical or logical host environment such as local machine, WSL
  distro, or remote host. A workspace is a project root or Git worktree root on
  a server. An instance is a running program or task identified within a
  workspace, such as a terminal, editor, agent, or task.
- Reserve `session` for auth/browser sessions and external protocol sessions
  such as MCP or model backend sessions. Do not use `session` for dashboard
  terminal/editor/agent resources.
- Route APIs through explicit server/workspace/instance identifiers such as
  `/api/servers/:serverId/workspaces/:workspaceId/instances/:instanceId`.
- Use opaque ids in API paths. Do not expose host paths as workspace ids; keep
  root paths, Git roots, worktree keys, and link details in daemon-owned state
  and view models.
- Keep MCP root, harness, and protocol session state scoped by
  project/worktree/instance
  rather than making the web daemon a global authority over ws runtime state.
- Treat Git worktrees as first-class workspace entries. The left navigation
  should group entries by server, then show a flat list of workspaces and
  worktree workspaces within each server group. Each server group should have an
  action surface such as `[+]` for creating or adding workspaces on that server.
  Workspace rows should support adding new instances within that workspace. The
  exact row labels, grouping chrome, and worktree notation remain TBA because
  the scenarios need more design discussion.
- Prefer linked ws web daemons over host-specific scraping for cross-environment
  visibility. Native Windows may use WSL-exposed tools as a fallback or
  discovery aid, but WSL process and workspace control should primarily happen
  inside a daemon running in WSL.
- Forwarding should preserve the same API and frontend resource shape: local
  requests are handled in-process, while linked-server requests are routed to
  the target daemon behind the same `serverId` namespace.
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
  packaging, complete IDE/editor features, and broad multi-owner server
  federation belong to later epics.
