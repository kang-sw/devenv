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
root/worktree, main-instance, and sub-instance model through stable daemon-owned view
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

- Planned: resource and view-model API substrate - define the server,
  workspace, root, main-instance, and sub-instance JSON shapes, opaque-id rules,
  root-kind metadata, route map, error/loading/stale fields, and golden
  fixtures.
- Planned: mock provider and contract test substrate - let the daemon serve
  deterministic dashboard data without live wsstate, PTY, or harness coupling,
  and verify fixtures plus protected API routes.
- Planned: minimal authenticated frontend shell - attach React/Vite to the
  daemon and render the first inspectable navigation tree plus detail pane from
  the same mock/live view-model contract.
- Planned: local workspace discovery substrate - connect the view model to real
  local project, plain-directory, Git-root, and Git-worktree discovery while
  preserving opaque ids and offline, moved, or inaccessible root states.
- Planned: event stream substrate - define the shared instance event envelope,
  reconnect/backfill behavior, and transcript fixture shape for later PTY,
  named-agent, exec, and diagnostic streams.

## Cross-Child Decisions

- Treat this epic as the UX contract-setting pass for the first visible
  dashboard. Child tickets should not block on basic navigation, resource
  vocabulary, empty/loading/error treatment, or mock/live data boundaries.
- Preserve the resource hierarchy as
  `server -> workspace -> root/worktree -> mainInstance -> subInstance` while
  the first child ticket settles final API vocabulary. Reserve `session` for
  auth/browser and external protocol sessions.
- Treat the concrete openable project target as a shared root component with
  additive capabilities for `plainDirectory`, `gitRootDir`, and `gitWorktree`.
  Git root directories and linked Git worktrees should share core UI and
  spawning behavior while preserving metadata that lets the UI distinguish their
  lifecycle and repository role.
- Use opaque ids in API paths. Host paths, Git roots, worktree keys, and wsstate
  storage details stay daemon-owned and appear only through authenticated view
  models.
- Do not offer generic folder deletion from the dashboard. Destructive root
  lifecycle actions may be exposed only as explicit Git-aware worktree actions
  for linked worktrees, with dirty/untracked safeguards left to the implementing
  child ticket.
- Keep mock and live providers behind the same daemon API contract so frontend
  work can start before live integrations are complete and tests can verify
  behavior without host-specific state.
- Require all dashboard HTTP APIs and future streams to remain behind the owner
  auth boundary created by the daemon foundation.
- Make frontend verification visual but narrow: the shell must expose enough
  real layout, density, status, and selection behavior for human steering, not
  a marketing page or a static mock.
- Use the restrained operational visual system from `ai-docs/ref/design.md`:
  square geometry, hairline separators, practical density, and clear hierarchy.
- Keep PTY terminal implementation out until resource identity, shell layout,
  and stream envelope decisions are stable enough to avoid making terminal panes
  the accidental root of the UI model.

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
