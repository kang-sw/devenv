---
title: Add durable dashboard workspace and workRoot registry activation
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-feat-ws-dashboard-persist-open-workroots: first narrow persistence step to absorb into the durable registry model
  260523-feat-ws-dashboard-linked-worktree-discovery: linked worktree discovery depends on durable membership and activation semantics
  260523-research-ws-dashboard-persistable-ui-state-map: broader persistence backlog that should build on this spine
related-mental-model:
  - ws-web-dashboard
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

## Decisions

- Do not introduce an "invisible worktree" state. Known workRoots are either
  visible with current availability or explicitly forgotten by a future
  high-friction action.
- Keep availability and activation separate. UI labels may choose different
  wording later, but the model must distinguish "reachable but offline" from
  "missing or inaccessible".
- Treat explicit refresh and bounded polling as the initial live-status update
  mechanism. Filesystem watchers may later become invalidation hints only; they
  are not the source of truth.
- Browser state must not become resource authority. Durable registry state
  seeds the daemon/resource model, and refresh recomputes current availability
  from filesystem/Git.

## Phases

### Phase 1: Durable registry and activation spine

Replace the opened-workRoot-only persistence path with a daemon-local
workspace/workRoot registry that records known workspace/workRoot membership,
provenance, and activation state. Preserve the existing opened workRoot
behavior by treating currently opened roots as online registry entries during
migration.

The resource view should expose known workRoots even when offline, while
gating file, Activity, and terminal APIs to online workRoots. Explicit online
and offline transitions should be command-routable so future keybindings use
the same control path as mouse actions.

Verification should cover migration from the existing opened-workRoots state,
all-workRoots-offline workspace visibility, online/offline API gating, and
current availability recomputation for missing or inaccessible roots.

### Phase 2: Explicit refresh and bounded live status polling

Add refresh semantics that recompute availability from filesystem/Git for every
known workRoot without changing activation state. Add bounded polling while the
dashboard is open so external filesystem/Git changes update visible status
without making polling the only correctness path.

Polling should be conservative: selected or online workRoots may refresh more
often than offline workRoots, large registries should back off, and explicit
refresh remains the deterministic recovery path. Later filesystem watches can
only trigger refresh-needed invalidations and must not replace recomputation.
