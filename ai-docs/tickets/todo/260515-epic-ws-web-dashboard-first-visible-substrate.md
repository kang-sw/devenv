---
title: ws web dashboard first visible substrate
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260514-research-ws-web-dashboard-direction: source research for resource model, UI shell, and deferred substrate ideas
  260514-feat-ws-web-daemon-foundation: completed authenticated daemon foundation this milestone builds on
  260513-feat-async-exec-output-reader: adjacent event/output reader pattern for later instance streams
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
  - developer-environment-tools
---

# ws web dashboard first visible substrate

## Scope

Create the first dashboard milestone that is both backend-contract heavy and
visually inspectable. This epic should turn the authenticated daemon foundation
into a narrow dashboard shell where the owner can see the server, workspace,
workRoot, main-instance, and sub-instance model through stable daemon-owned view
models.

The milestone should make future child tickets non-blocked on unresolved
dashboard UX primitives. It does not need to implement terminal, editor,
named-agent, or remote-server feature depth, but it must settle enough UI/UX,
API, fixture, and event-shape decisions that those children can proceed without
reopening the basic dashboard frame.

## Non-Scope

- Full terminal, editor, named-agent, or document-viewer feature completion.
- Public internet deployment, TLS, multi-user accounts, RBAC, or tenancy.
- Linked remote daemon federation beyond preserving the server-id namespace.
- Replacing ws MCP stdio transport or making the dashboard the canonical ws MCP
  session authority.
- Broad harness-library implementation or API-backed model execution.
- Pixel-final visual design beyond the minimum needed to validate density,
  hierarchy, navigation, loading, empty, and error states.

## Child Tickets

- `260516-feat-ws-web-resource-view-model-contract` - done; first child and
  implementation blocker for the rest of this epic. Defines the
  server/workspace/workRoot/main-instance/sub-instance view-model contract,
  workRoot vocabulary, protected API routes, mock provider, and golden
  fixtures.
- `260516-feat-ws-web-minimal-frontend-shell` - done; attaches the real
  React/Vite shell to the daemon and renders the first inspectable navigation
  and detail surface from the shared view-model API.
- `260516-feat-ws-web-local-workspace-discovery` - ready; connects the
  view-model contract to local plain-directory, Git-primary-root, and
  Git-linked-worktree discovery, including manual and opportunistic refresh.
- `260516-feat-ws-web-instance-event-stream` - todo; defines the authenticated
  instance event envelope, transcript fixtures, and reconnect/backfill scaffold
  for later PTY, named-agent, exec, diagnostic, viewer, and translation streams.

## Cross-Child Decisions

- Treat this epic as the UX contract-setting pass for the first visible
  dashboard. Child tickets should not block on basic navigation, resource
  vocabulary, empty/loading/error treatment, or mock/live data boundaries.
- Preserve the resource hierarchy as
  `server -> workspace -> workRoot -> mainInstance -> subInstance`.
  Reserve `session` for auth/browser and external protocol sessions.
- Define `workspace` as a daemon-discovered project group, not a user-created
  category. A workspace groups one or more workRoots and is usually inferred
  from a Git repository group or a single plain directory.
- Define `workRoot` as the physical directory used as an open, spawn, and run
  target. Each workRoot has additive capabilities for `plainDirectory`,
  `gitPrimaryRoot`, and `gitLinkedWorktree`; Git primary roots and linked Git
  worktrees should share core UI and spawning behavior while preserving metadata
  that lets the UI distinguish their lifecycle and repository role.
- Use opaque ids in API paths. Host paths, Git roots, workRoot keys, and wsstate
  storage details stay daemon-owned and appear only through authenticated view
  models.
- Keep UI compaction as a presentation policy, not the resource model:
  singleton `workspace -> workRoot -> mainInstance` chains may render as one
  compact row, while APIs and fixtures preserve the full hierarchy.
- Do not offer generic folder deletion from the dashboard. Destructive workRoot
  lifecycle actions may be exposed only as explicit Git-aware worktree actions
  for linked worktrees, with dirty/untracked safeguards left to the implementing
  child ticket.
- Keep the explorer as a root picker rather than a file manager. It may expose a
  narrow `Create empty folder` action for creating a new workRoot candidate, but
  generic delete, rename, move, and copy actions stay out of scope.
- Re-detect workRoot kind through manual refresh and opportunistic refresh when
  selecting, opening, or spawning from a workRoot. Broad filesystem watcher
  behavior is deferred.
- Keep mock and live providers behind the same daemon API contract so frontend
  work can start before live integrations are complete and tests can verify
  behavior without host-specific state.
- Require all dashboard HTTP APIs and future streams to remain behind the owner
  auth boundary created by the daemon foundation.
- Make frontend verification visual but narrow: the shell must expose enough
  real layout, density, status, and selection behavior for human steering, not
  a marketing page or a static mock.
- Route mouse actions and keyboard actions through command ids so the shell can
  later support tmux-style leader navigation. Reserve `^b` to mean ctrl plus
  lowercase `b`; do not treat it as `Ctrl+Shift+b`.
- Use the restrained operational visual system from `ai-docs/ref/design.md`:
  square geometry, hairline separators, practical density, and clear hierarchy.
- Keep PTY terminal implementation out until resource identity, shell layout,
  and stream envelope decisions are stable enough to avoid making terminal panes
  the accidental root of the UI model.
- Treat the first child as the only remaining implementation-order blocker:
  resource/view-model API and mock fixtures should land before frontend,
  discovery, or stream work. After that child, the other child tickets can be
  implemented independently when their spec coverage is ready.

## Completion Criteria

- Done: child tickets deliver authenticated API/view-model contracts, mock
  fixtures, an inspectable frontend shell, local workspace discovery, and the
  first shared event-stream substrate for later live features.
- Dropped: the dashboard MVP abandons the web shell direction or a different
  milestone replaces this first-visible substrate structure.
- Deferred: terminal depth, browser-native editing, named-agent controls,
  document viewing, translation, linked remote daemons, WSL hardening, and
  harness-library capability belong to later epics or child tickets unless a
  child explicitly pulls a minimal dependency forward.
