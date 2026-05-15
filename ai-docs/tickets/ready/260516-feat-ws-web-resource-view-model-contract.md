---
title: ws web dashboard resource view-model contract
parent: 260515-epic-ws-web-dashboard-first-visible-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260515-epic-ws-web-dashboard-first-visible-substrate: coordinating first visible substrate epic
  260514-research-ws-web-dashboard-direction: source research for resource model and shell boundaries
  260514-feat-ws-web-daemon-foundation: authenticated daemon foundation this API extends
spec:
  - 260516-ws-web-dashboard-resource-view-model-contract
  - 260516-ws-web-dashboard-mock-view-model-fixtures
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
---

# ws web dashboard resource view-model contract

## Background

The first visible dashboard needs a stable backend contract before frontend,
discovery, or event-stream work can proceed without re-deriving the same
resource vocabulary. The daemon currently provides an authenticated shell, but
it does not expose the server, workspace, workRoot, main-instance, and
sub-instance hierarchy through an API.

This ticket creates the first authenticated dashboard data contract and a mock
provider that can drive frontend work before live workspace discovery, PTY, or
named-agent integrations exist.

## Decisions

- Preserve the hierarchy
  `server -> workspace -> workRoot -> mainInstance -> subInstance`.
- Define `workspace` as a daemon-discovered project group, not a user-created
  category.
- Define `workRoot` as the physical open, spawn, and run directory with
  additive kind metadata: `plainDirectory`, `gitPrimaryRoot`, or
  `gitLinkedWorktree`.
- Use opaque ids in API paths. Host paths, Git roots, workRoot keys, wsstate
  paths, and runtime session identifiers remain daemon-owned state exposed only
  through authenticated view models.
- Treat UI compaction as presentation policy only. API responses and golden
  fixtures keep the full hierarchy even when singleton chains would render as
  one compact row.
- Keep mock and live providers behind the same daemon API contract.

## Phases

### Phase 1: Core resource vocabulary

Replace or compatibility-wrap the scaffolded worktree vocabulary so the core
dashboard model speaks in workRoots. Define the durable ids, resource path,
workRoot kind, workRoot status, instance role, instance kind, and interaction
mode primitives needed by later API, frontend, discovery, and event-stream
tickets.

The implementation may keep a narrow internal alias only if that reduces churn
without leaking `WorktreeId` or `worktree_id` into the public dashboard API.
Any retained alias must be explicitly temporary and contained inside core
internals.

Success criteria:

- Core types represent `WorkRootId`, `WorkRootKind`, and workRoot status.
- Resource paths and serialized field names use `workRoot`, not `worktree`.
- Existing daemon foundation tests continue to pass.

### Phase 2: Authenticated view-model API and mock fixtures

Add protected daemon API routes that return deterministic dashboard view models
for the first visible shell. The API should expose enough data for a navigation
tree and detail pane: server identity, workspace rows, workRoot rows,
main-instance rows, sub-instance rows, status fields, loading/error/stale
fields, and command/action hints where the frontend should render affordances.

Provide a mock provider and golden fixtures that exercise singleton chains,
multi-root workspaces, Git primary roots, linked worktrees, plain directories,
offline or inaccessible workRoots, main instances, and sub instances. Tests
must verify that the new API routes remain behind owner authentication.

Success criteria:

- Frontend work can consume one stable API shape in mock mode before live
  discovery exists.
- Golden fixtures preserve the full hierarchy and include compactable cases.
- Unauthenticated callers cannot access dashboard view-model API routes.
