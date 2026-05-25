---
title: Add durable dashboard workspace and workRoot registry activation
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: first narrow persistence step to absorb into the durable registry model
  260523-feat-ws-dashboard-linked-worktree-discovery: linked worktree discovery depends on durable membership and activation semantics
  260523-research-ws-dashboard-persistable-ui-state-map: broader persistence backlog that should build on this spine
spec:
  - 260523-dashboard-workroot-registry-activation
plans:
  phase-1: 2026-05/23-260523-feat-ws-dashboard-workroot-registry-activation
  phase-2: 2026-05/23-260523-feat-ws-dashboard-workroot-registry-activation-phase-2
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-23
---

# Add durable dashboard workspace and workRoot registry activation

## Background

The dashboard currently persists only opened workRoot paths and treats the live
opened-workRoot map as the primary route authority. That is enough for simple
restart recovery, but it does not support a consistent workspace model where a
workspace remains known even when every workRoot is offline, linked worktrees
remain visible after discovery, and current workRoot status follows the
filesystem/Git state without silently hiding known roots.

The intended model separates durable membership, current availability, and
dashboard activation:

- Membership is persistent: once a workspace/workRoot is manually opened or
  discovered, it remains visible until an explicit future forget/remove action
  or root-folder deletion policy handles it.
- Availability is live-derived: filesystem and Git worktree state are
  recomputed on refresh/poll. Missing, inaccessible, prunable, or moved
  workRoots remain visible as degraded rows instead of becoming invisible.
- Activation is user-controlled: workRoots default offline unless explicitly
  brought online. Online workRoots participate in file, Activity, and terminal
  APIs; offline workRoots remain selectable rows with activation actions. A
  workspace with all workRoots offline is still valid.

The current public `WorkRootStatus` vocabulary already uses `online`,
`offline`, `moved`, and `inaccessible` for reachability/availability. That
shape must not be reused as activation state. The implementation should split
the public model so callers can distinguish a reachable-but-offline workRoot
from a missing/inaccessible workRoot.

## Decisions

- Do not introduce an "invisible worktree" state. Known workRoots are either
  visible with current availability or explicitly forgotten by a future
  high-friction action.
- Keep availability and activation separate. UI labels may choose different
  wording later, but the model must distinguish "reachable but offline" from
  "missing or inaccessible".
- Use `activation: online | offline` for user-controlled dashboard targeting.
  Use an initial `availability` vocabulary covering available, missing, moved,
  inaccessible, and unknown states. Git-specific states such as prunable can
  remain details or follow-up vocabulary unless the linked-worktree discovery
  ticket promotes them into first-class public values.
- Do not reinterpret the existing `WorkRootStatus::Online/Offline` enum as the
  new activation layer. Introduce explicit public fields or vocabulary for
  availability and activation, then update frontend resource types, fixtures,
  route tests, and mental-model/spec language together.
- Treat explicit refresh and bounded polling as the initial live-status update
  mechanism. Filesystem watchers may later become invalidation hints only; they
  are not the source of truth.
- Browser state must not become resource authority. Durable registry state
  seeds the daemon/resource model, and refresh recomputes current availability
  from filesystem/Git.
- Route/API errors should distinguish unknown workRoots from known inactive or
  unavailable workRoots: unknown ids remain 404, known-but-offline workRoots
  should return a bounded "workRoot offline" style error, and online roots whose
  filesystem/Git availability degraded should return a bounded unavailable
  error without host paths.

## Phases

### Phase 1: Durable registry and activation spine

Replace the opened-workRoot-only persistence path with a daemon-local
workspace/workRoot registry that records known workspace/workRoot membership,
provenance, and activation state. Preserve the existing opened workRoot
behavior by treating currently opened roots as online registry entries during
migration.

Split the resource view-model contract before relying on activation semantics.
The public model should expose availability separately from activation, and
frontend `resourceModel.ts`, Rust serde tests, mock fixtures, route tests, and
resource detail UI should be updated in the same slice. Keep current row
identity stable where possible so remembered workRoots do not churn selection
or pane state.

The resource view should expose known workRoots even when offline, while
gating file, Activity, and terminal APIs to online workRoots. Explicit online
and offline transitions should be command-routable so future keybindings use
the same control path as mouse actions.

Verification should cover migration from the existing opened-workRoots state,
all-workRoots-offline workspace visibility, public availability/activation
serialization, online/offline API gating including error-code distinctions,
and current availability recomputation for missing or inaccessible roots.

### Result (05b8c07) - 2026-05-23

Implemented the durable workRoot registry and activation spine. The daemon now
persists versioned workRoot registry entries with provenance metadata, migrates
existing opened-workRoots state as online registry membership, exposes
availability separately from activation in the public resource model, and keeps
known offline or unavailable workRoots visible.

File, Activity, terminal HTTP, and terminal WebSocket paths now distinguish
unknown workRoot ids from known offline activation and online-but-unavailable
workRoots. Offline/unavailable responses remain bounded and avoid host paths.
Activation changes are exposed through command-routed dashboard actions, and
open-workRoot responses include the daemon-owned opened workRoot id header so
the frontend does not reconstruct opaque ids from paths.

Review follow-up hardened activation persistence rollback, open-root
persistence failure handling, terminal-id route gating, already-open terminal
WebSocket input/output gating, and ambiguous same-label opened-root selection.

Verification passed:

- `cargo fmt --all --check`
- `cargo test -p ws-dashboard-core`
- `cargo test -p ws-dashboard-daemon`
- `npm run build`
- `npm run test:open-work-root`
- `npm run test:resource-model`
- `npm run test:commands`
- `npm run test:browser`

Phase 2 remains open for bounded live status polling and refresh cadence.

### Phase 2: Explicit refresh and bounded live status polling

Add refresh semantics that recompute availability from filesystem/Git for every
known workRoot without changing activation state. Add bounded polling while the
dashboard is open so external filesystem/Git changes update visible status
without making polling the only correctness path.

Polling should be conservative: selected or online workRoots may refresh more
often than offline workRoots, large registries should back off, and explicit
refresh remains the deterministic recovery path. Later filesystem watches can
only trigger refresh-needed invalidations and must not replace recomputation.

### Result (67611367) - 2026-05-23

Implemented explicit refresh and bounded live status polling over the canonical
`GET /api/dashboard/resources` endpoint. The daemon route already recomputes
known workRoot availability on each resource load, so the implementation
preserved that route as the refresh authority and added route coverage proving
availability changes do not remove registry membership or mutate activation.

The frontend now uses a resource refresh coordinator for initial load,
explicit `dashboard.refresh`, open-workRoot reconciliation, activation
responses, and bounded polling. The coordinator suppresses overlapping polling
requests, queues foreground refresh behind an in-flight poll, ignores stale
poll responses after newer open/activation resource views, preserves the last
known tree on refresh failure, and stops mounted-dashboard polling after
unmount.

Verification passed:

- `cargo fmt --all --check`
- `cargo test -p ws-dashboard-daemon`
- `npm run build`
- `npm run test:resource-model`
- `npm run test:commands`
- `npm run test:open-work-root`
- `npm run test:browser`
