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
  - ws-web-dashboard
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

Completed milestone boards and foundation children:

- `260514-feat-ws-web-daemon-foundation` - done; daemon shell, owner auth,
  bind-mode guards, protected routes, and serving foundation.
- `260515-epic-ws-web-dashboard-first-visible-substrate` - done; resource
  vocabulary, view-model API, mock/live provider boundary, authenticated
  frontend shell, local discovery, and event envelope substrate.
- `260516-epic-ws-web-dashboard-workbench-substrate` - done; dark-first
  frontend setup, token-free paired routes, server-scoped route identity, and
  constrained Dockview-backed workbench substrate.
- `260516-epic-ws-web-dashboard-workroot-io-substrate` - done; opened workRoots
  drive resource APIs, file explorer/read-only panes, terminals, refresh, and
  browser dogfood coverage.
- `260518-epic-ws-dashboard-activity-console` - done; WorkRoot Activity became
  a reusable read-only Activity Console with live ribbon, selected transcript
  blocks, stream-backed updates, and backend-owned transcript resolution.

Completed follow-up clusters:

- Workbench and terminal recovery: UI acceptance recovery, WebSocket terminal
  transport, cross-platform terminal evidence, Dockview parity/dynamic groups,
  tab polish, scroll/IME verification, split-scroll reset, and bounded daemon
  shutdown behavior are done.
- WorkRoot continuity: persisted open workRoots, terminal tab restore,
  registry activation, workspace root pruning, linked-worktree discovery,
  read-only file pane restore, and explicit workspace forget/remove UI are
  done.
- Root picker and visual polish: explorer-style root picker, React Aria root
  picker pilot, visual building blocks, icon-first navigation/topbar polish,
  icon chrome refinement, and context surface hierarchy are done.

Active or planned product tracks:

- `260524-feat-ws-dashboard-document-viewer-editor-substrate` - ready; next
  implementation track. Add a reusable markdown document viewer, translation-
  ready block model, panel-local view/edit mode, raw-text editing boundary, and
  same-document save fan-out contract. This track is expected to proceed as
  three conservative slices: markdown viewer/block interaction, daemon-backed
  translation provider MVP, then raw text edit/save fan-out. This track should
  be polished before the next large management or agent-panel slice.
- WorkRoot management - planned child track. Continue from the completed
  registry/root-picker/workspace policies into practical owner operations for
  managing active, unavailable, linked, remembered, pinned, and recoverable
  workRoots without turning the picker into a generic file manager.
  `260524-feat-ws-dashboard-add-git-worktree-ui` is the first planned slice:
  move workspace removal behind an overflow menu and add Git worktree creation
  with daemon-resolved branch/path preview.
  `260524-feat-ws-dashboard-git-aware-workroot-toolbar` is the companion
  toolbar slice for selected-workRoot branch/status chips plus fetch, push, and
  fast-forward-only pull controls.
- Agent view panel - planned child track. Promote read-only Activity Console
  foundations into a dedicated agent-oriented panel for named-agent/main-
  session/subtask visibility while keeping control actions such as interrupt,
  cancel, erase, retry, or terminate behind separate high-friction tickets.
- Multi-server management - planned design track. Define the minimum MVP shape
  for local, WSL, and remote linked daemons, server capability/status display,
  forwarding under the same `serverId` namespace, and auth/pairing boundaries.
  Broader remote hardening and server federation remain future scope.
- Dashboard persistence map - research track.
  `260523-research-ws-dashboard-persistable-ui-state-map` remains the backlog
  map for selected resources,
  file explorer state, workbench layout, Activity Console acknowledgement and
  scroll state, command preferences, chrome preferences, and privacy-sensitive
  root-picker history.
- Diagnostics, events, and task surfaces - planned support track. Keep these as
  system-observation panels that can share Activity/document/workbench
  primitives without becoming separate top-level navigation roots.
- Terminal UX - deferred track. Existing command-based terminal workflow remains
  usable, but native-Windows control-key behavior and browser focus regression
  tickets remain open stabilization work before a larger terminal UX redesign.
- React Aria and visual system refresh - research track.
  `260524-research-ws-dashboard-react-aria-ui-primitives` and
  `260524-research-ws-dashboard-visual-design-system-refresh` remain future
  component-system quality work after the root-picker pilot and first
  visual-polishing line.

Implementation sequence:

1. Editor polishing: markdown viewer, document mode substrate, raw-text editing
   boundary, translation-ready overlays, and save fan-out.
2. WorkRoot management: owner operations and recovery flows over the durable
   workspace/workRoot registry.
3. Agent view panel: dedicated agent-oriented visibility over Activity Console
   and future main-session/subtask sources.
4. Multi-server management, diagnostics/task panels, persistence expansion,
   terminal UX redesign, and broader visual-system research should be split
   into child tickets when their boundaries are ready.

Longer-range product direction stays in
`260514-research-ws-web-dashboard-direction`: harness/runtime library
directions, remote hardening, keyboard model, rich document platform ideas,
server federation, and other future scope should be promoted from that research
ticket only when they become implementation-ready.

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
- Model dashboard resources around one owner-managed workspace root and one main
  user interaction point per workRoot:
  `server -> workspace -> workRoot -> mainInstance -> subInstance`. A server is
  a physical or logical host environment such as local machine, WSL distro, or
  remote host. A workspace is an owner-managed root scope anchored by a root
  workRoot. A workRoot is the concrete physical open, spawn, and run directory
  and can be online, offline, moved, or inaccessible. Derived workRoots such as
  linked Git worktrees belong under their owning workspace until an explicit
  derive/promote operation creates a new workspace. A main instance is the
  user-facing conversation or control point; sub instances are delegated or
  auxiliary work such as ws agents, exec jobs, document viewers, translation
  tasks, diagnostics, or subprocesses.
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
- If a workspace's root workRoot is unavailable while at least one child
  workRoot remains active, keep the workspace visible in a disabled or
  recovery-needed state. Automatically prune a workspace only when it has no
  active workRoots. This automatic empty-workspace cleanup is distinct from
  explicit owner forget/remove UI.
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
- Activity visibility remains read-only. Control actions such as start,
  interrupt, cancel, erase, and retry stay out of the dashboard Activity
  Console; any future terminate affordance requires a separate high-friction
  control ticket.
- Persisted dashboard state should store logical, user-visible descriptors and
  preferences, then revalidate through daemon/resource/file APIs on restore.
  Browser state must not become authority for daemon resources, and persistence
  should avoid raw output, transcripts, host paths, cache paths, backend
  session paths, process ids, or stale daemon terminal ids unless a child ticket
  explicitly defines a bounded privacy-reviewed format.
- WorkRoot resource modeling separates durable membership, live-derived
  availability, and user-controlled activation. Existing `WorkRootStatus`
  online/offline vocabulary is reachability-flavored and must not be reused as
  the activation layer without a public model split. Known workRoots remain
  visible while their workspace remains visible even when missing, inaccessible,
  prunable, or offline; there is no invisible discovered-worktree state.
  Explicit refresh and bounded polling recompute filesystem/Git availability
  and may trigger the automatic no-active-workRoot workspace prune policy, while
  future filesystem watchers may only act as refresh-needed hints.

## Completion Criteria

- Done: child tickets deliver a usable authenticated local/tunnel web dashboard
  with PTY terminals, document viewing/raw-text editing, workspace and workRoot
  management, and ws named-agent visibility.
- Dropped: a different host UI direction replaces the web dashboard approach or
  the MVP proves impractical for the intended personal workflow.
- Deferred: multi-user access, full public deployment hardening, desktop shell
  packaging, complete IDE/editor features, and broad multi-owner server
  federation belong to later epics.
