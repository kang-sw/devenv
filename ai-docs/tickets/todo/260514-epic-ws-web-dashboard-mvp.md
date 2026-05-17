---
title: ws web dashboard MVP
related:
  260427-chore-claude-dash-windows: prior PTY dashboard surface and Windows stability motivation
  260513-research-streamable-http-mcp-transport: adjacent long-running daemon and remote transport research
  260513-feat-async-exec-output-reader: adjacent persisted process output and reader-agent pattern
  260514-research-ws-web-dashboard-direction: absorbed provisional dashboard child backlog and future direction
  260515-epic-ws-web-dashboard-first-visible-substrate: first visible dashboard substrate milestone
  260516-epic-ws-web-dashboard-workbench-substrate: next frontend workbench substrate milestone
  260516-epic-ws-web-dashboard-workroot-io-substrate: next workRoot filesystem and terminal substrate milestone
  260516-feat-ws-web-resource-view-model-contract: first child of the visible substrate milestone
  260516-feat-ws-web-minimal-frontend-shell: inspectable frontend child of the visible substrate milestone
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
- Host workRoot discovery so the owner can open a plain directory, Git primary
  root, or linked Git worktree and start an agent-oriented workspace from the
  web UI.
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

- `260514-feat-ws-web-daemon-foundation` - done; daemon shell, owner auth,
  WebSocket auth, local/tunnel/public bind-mode guards, and foundation security
  smoke. First substrate.
- `260514-research-ws-web-dashboard-direction` - research holding the absorbed
  provisional child backlog, refined resource model, document viewer ideas,
  keyboard/navigation direction, and future harness-library split points.
  Recreate implementation children from that research only when the boundaries
  are ready.
- `260515-epic-ws-web-dashboard-first-visible-substrate` - done; first
  hybrid milestone for resource/view-model APIs, mock/live data boundaries, a
  minimal authenticated frontend shell, local workspace discovery, and event
  stream substrate.
- `260516-feat-ws-web-resource-view-model-contract` - done; first child of the
  first visible substrate and next implementation-order blocker for stable
  dashboard resource APIs, mock fixtures, and workRoot vocabulary.
- `260516-feat-ws-web-minimal-frontend-shell` - done; child of the first
  visible substrate for the first inspectable authenticated browser shell.
- `260516-feat-ws-web-local-workspace-discovery` - done; child of the first
  visible substrate for live local plain-directory, Git-primary-root, and
  Git-linked-worktree discovery.
- `260516-feat-ws-web-instance-event-stream` - done; child of the first visible
  substrate for a shared authenticated instance event envelope and
  fixture-backed stream scaffold.
- Planned: next dashboard epic should start with a dark-first frontend visual
  system step that turns `ai-docs/ref/design.md` into a dashboard-specific
  `DESIGN.md`-style theme guideline under `ws-dashboard/frontend/` before
  deeper UI feature work.
- `260516-epic-ws-web-dashboard-workbench-substrate` - done; frontend milestone
  for dark-first theme setup, token-free stable browser entry after pairing,
  server-scoped route identity, and a constrained VS Code-inspired workbench
  substrate.
- `260516-epic-ws-web-dashboard-workroot-io-substrate` - done; opened
  workRoots now drive the primary dashboard resource API and browser resource
  model, with file, read-only pane, terminal, refresh, and dogfood coverage.
- `260516-bug-ws-web-dashboard-ui-acceptance-recovery` - done; recovered
  browser-level dashboard usability before merging workRoot IO UI work,
  including terminal tabs, live initial terminal state, real xterm behavior,
  pane fill, file explorer affordance, and visual/browser acceptance evidence.
- `260516-bug-ws-web-terminal-websocket-transport` - done; replaced polling
  terminal I/O with an owner-authenticated WebSocket live path and recover
  interactive input fidelity before considering dashboard terminal quality
  mergeable.
- `260516-bug-ws-web-terminal-cross-platform-portability` - done; make
  dashboard terminal shell selection, PTY commands, browser harness behavior,
  and acceptance evidence honestly cross-platform or explicitly OS-scoped.
- `260517-bug-ws-dashboard-dockview-workbench-parity` - done; corrective
  substrate work to make Dockview the visible workbench layout owner while
  preserving recovered file and terminal pane behavior.
- `260517-bug-ws-dashboard-dockview-dynamic-groups` - done; make Dockview split
  drops create durable dynamic dashboard groups, with initial two-group defaults
  and constrained automatic placement policy.
- `260517-feat-ws-dashboard-workbench-tab-polish` - done; polish workbench tab
  close affordances, insertion/focus policy, empty-state cleanup, and
  preview-to-pinned read-only file behavior after Dockview workbench parity is
  restored and before introducing a richer editor library.

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
- Use `ws-dashboard/` as the root project directory. The current scaffold
  contains core, harness-core, harness-cli, daemon, and frontend slots; reserve
  short names such as `wsdash` for command aliases rather than the source tree.
- Use CodeMirror 6 as the initial browser-native editor base, with Vim-like
  modal editing through an extension or custom ws modal layer. Keep terminal
  nvim available as an optional PTY workflow, but do not make it the primary
  editor path because Windows PTY behavior is a known stability concern.
- The daemon exposes stable view-model APIs over wsstate and wsagent behavior.
  The browser must not treat the cache layout itself as the public contract.
- Model dashboard resources around one main user interaction point per
  workRoot:
  `server -> workspace -> workRoot -> mainInstance -> subInstance`. A server is
  a physical or logical host environment such as local machine, WSL distro, or
  remote host. A workspace is a daemon-discovered project group, not a
  user-created category. A workRoot is the concrete physical open, spawn, and
  run directory and can be online, offline, moved, or inaccessible. A main
  instance is the user-facing conversation or control point; sub instances are
  delegated or auxiliary work such as ws agents, exec jobs, document viewers,
  translation tasks, diagnostics, or subprocesses.
- Reserve `session` for auth/browser sessions and external protocol sessions
  such as MCP or model backend sessions. Do not use `session` for dashboard
  terminal/editor/agent resources.
- Route APIs through explicit server/workspace/workRoot/instance identifiers.
- Browser routes should mirror that explicit hierarchy with
  `/servers/:serverId/...` paths. Do not encode server identity inside
  workspace, workRoot, or instance ids.
- Use opaque ids in API paths. Do not expose host paths as workspace ids; keep
  root paths, Git roots, workRoot keys, and link details in daemon-owned state
  and view models.
- Keep MCP root, harness, and protocol session state scoped by
  project/workRoot/instance
  rather than making the web daemon a global authority over ws runtime state.
- Treat Git primary roots and linked Git worktrees as additive workRoot kinds
  under workspaces. The left navigation should group by server, then workspace,
  then concrete workRoot entries without hiding offline or inaccessible
  workRoots that remain useful recent context. Singleton chains may render as
  compact rows, but the API model remains fully hierarchical.
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
- Do not apply `ai-docs/ref/design.md` as a light-mode palette by default.
  Dashboard frontend work should derive a dark-first, Carbon-inspired product
  theme with semantic tokens before additional large UI surfaces are built.
- The frontend workbench should be VS Code-inspired but constrained around a
  `left nav | workRoot workbench` information architecture. The left nav
  should select server/workspace/workRoot locations, while each opened
  workRoot owns sibling split groups with group-local pinned and opened rows.
  Main instances are durable workRoot-local surfaces, sub instances are
  badge/popover/card projections attached to a main instance, and utility
  panels such as viewer, task view, diagnostics, events, and inspector open as
  support surfaces rather than permanent global regions. Prefer a
  layout-library substrate such as Dockview only behind a dashboard-owned panel
  registry and adapter. Keep FlexLayout as the comparison fallback if Dockview
  policy code becomes noisy.
- The default workbench preset should use two split groups: side-by-side on
  wide screens and stacked on narrow screens. File opens should prefer the
  second or later split group to avoid replacing the active agent view, while
  agent and persistent terminal surfaces default to the first or focused group.
- A frontend panel is an attachment, not a backend instance. Backend lifecycle
  should be daemon-owned. Terminal panels are the first deliberate exception to
  close-as-detach: a terminal survives browser refresh because the daemon owns
  the session, but explicitly closing the terminal panel terminates that
  terminal session. Hidden detached terminal restore UX is out of scope unless
  a later ticket reintroduces it.
- Pairing tokens should remain one-time startup entry URLs. After successful
  pairing, the browser should land on a token-free stable app URL and rely on
  the HTTP-only owner cookie for refresh-safe navigation.
- Terminal and agent TUI panes must avoid continuous logical width changes
  during visual layout drag. Logical PTY/TUI columns should change only through
  explicit presets, committed resize, or a later user-invoked fit command.
- Put future dashboard specs under `ai-docs/spec/ws-web-dashboard/`. Add
  `ai-docs/mental-model/ws-web-dashboard/` only after implementation creates
  real dashboard subdomains; do not prefill speculative mental-model material.
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
