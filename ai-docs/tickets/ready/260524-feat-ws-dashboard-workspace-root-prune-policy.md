---
title: Add dashboard workspace root and auto-prune policy
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-workroot-registry-activation: durable registry and activation spine that this policy refines
  260523-feat-ws-dashboard-linked-worktree-discovery: child workRoots need clear ownership and pruning semantics
  260524-feat-ws-dashboard-workspace-forget-remove-ui: explicit workspace owner cleanup must remain separate from automatic empty-workspace pruning
spec:
  - 260524-dashboard-workspace-root-prune-policy
related-mental-model:
  - ws-web-dashboard
---

# Add dashboard workspace root and auto-prune policy

## Background

Dashboard dogfood after durable workRoot registry activation exposed an
ambiguous resource lifecycle. The current model keeps known workRoots visible
until explicit forget/remove semantics exist, but the user intent is sharper:
workspaces should be owner-managed root scopes, while workRoots under a
workspace may be automatically detected from the root scope.

The policy must also separate automatic cleanup from explicit owner deletion.
Temporary or stale dashboard workspaces with no usable workRoot should not stay
visible indefinitely, but a workspace whose root workRoot is unavailable may
still contain a live child or dangling workRoot that can be used to reconnect or
derive a replacement workspace.

## Decisions

- A workspace is an owner-managed root scope, not merely an arbitrary
  daemon-discovered grouping.
- Each workspace has a root workRoot anchor. The root may be an owner-added
  directory, a Git primary root, or another explicit root directory accepted by
  the dashboard.
- Child workRoots, including linked Git worktrees, are derived from the
  workspace root and remain workRoots rather than independent workspaces unless
  an explicit derivation or promotion operation creates a new workspace.
- `active workRoot` means a workRoot that is both targetable and currently
  usable for dashboard work: activation permits targeting and availability is
  usable.
- Automatic pruning is based on `activeWorkRootCount == 0`, not on the root
  workRoot alone.
- If the root workRoot is unavailable while at least one child workRoot remains
  active, the workspace stays visible as disabled or recovery-needed. That state
  is room for reconnecting the root or deriving a new workspace from a dangling
  child; it is not a normal active workspace.
- Disabled or recovery-needed workspaces remain visible and selectable in the
  left navigation, but ordinary file, terminal, and Activity operations target
  only active child workRoots.
- Automatic pruning does not ask for confirmation because it is not a
  user-initiated destructive action. If the selected workspace/workRoot is
  pruned, browser selection reconciles to the next available workRoot, or to the
  server/root empty state when none remains.
- Explicit workspace forget/remove UI remains separate. Automatic pruning of
  empty workspaces must not become a broad filesystem delete operation and must
  not replace high-friction owner cleanup controls for workspace roots.

## Constraints

- Keep host paths daemon-private. Browser-visible state may expose labels,
  opaque ids, kind, availability, activation, and bounded diagnostics, not raw
  root paths or Git metadata paths.
- Keep the public vocabulary at `workspace` and `workRoot`. Do not introduce
  `worktreeId` or a Git-specific public identity layer.
- Do not collapse activation and availability. Offline activation, unavailable
  filesystem state, disabled workspace state, and automatic pruning have
  distinct meanings.
- Do not remove a workspace that still has at least one active child workRoot,
  even when its root workRoot is unavailable.
- Pruning should clear dependent browser-only selection and workbench state for
  pruned resources so stale ids do not survive the next resource refresh.

## Phases

### Phase 1: Implement workspace root lifecycle and auto-prune

Update the dashboard resource registry and live resource view so a workspace has
a root workRoot anchor and an active-workRoot count. Resource refresh, open, and
activation responses should apply the settled policy:

- `activeWorkRootCount == 0` removes the workspace from the visible resource
  tree through automatic pruning.
- root workRoot unavailable with at least one active child workRoot keeps the
  workspace visible in a disabled or recovery-needed state.
- disabled or recovery-needed workspaces remain selectable enough for recovery
  affordances, but ordinary workRoot operations still target only online and
  available workRoots.
- pruning reconciles stale browser selections and dependent workbench state
  without prompting the user.
- linked or dangling child workRoots must not silently become independent
  workspaces without an explicit future derive/promote operation.
- explicit workspace forget/remove UI remains out of scope for this phase, and
  child workRoots do not receive direct owner forget/remove controls.

Suggested implementation strategy: model the root workRoot relationship in the
daemon-owned registry/resource view rather than in frontend-only row logic, then
let the frontend render the resulting workspace state and prune outcomes from
`/api/dashboard/resources`.

Verification should include daemon route tests for resource refresh pruning,
root-unavailable-with-active-child disabled state, activation/availability
separation, and no host-path leakage. Frontend verification should cover
selection reconciliation when a workspace is pruned and disabled workspace
rendering when a child workRoot remains active.
