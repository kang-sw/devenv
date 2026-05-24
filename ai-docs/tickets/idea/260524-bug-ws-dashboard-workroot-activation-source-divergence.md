---
title: Unify dashboard workRoot activation lookup across opened and discovered roots
related:
  260523-feat-ws-dashboard-workroot-registry-activation: activation source-of-truth contract
  260523-feat-ws-dashboard-linked-worktree-discovery: discovered linked workRoot projection
---

# Unify dashboard workRoot activation lookup across opened and discovered roots

## Background

The linked-worktree activation fix in `17dd4e6c` preserved in-memory activation
for discovered linked workRoots when rebuilding `/api/dashboard/resources`.
That closes the simple "linked child goes offline but cannot expose an online
action" regression, but the activation lookup still depends on provenance:
opened owner roots flow through candidate activation, while discovered linked
roots flow through a discovered-only activation map and otherwise default to
online.

This leaves a follow-up risk when the same workRoot can be both explicitly
opened and discovered as a sibling. The live projection may encounter the
discovered sibling first, miss the opened activation because the discovered map
excludes opened provenance, default the row to online, and then skip the later
owner projection because the duplicate workRoot id is already present. In that
case the resource view can again disagree with route gates, even though action
and state rendering are otherwise centralized through `WorkspaceBuilder::push`.

## Phases

### Phase 1: Registry-wide activation lookup

Replace provenance-specific activation lookup during live dashboard resource
construction with a single registry activation lookup by `WorkRootId`.

Preserve the current ownership boundary:

- Explicitly opened roots remain the only persisted owner roots.
- Discovered linked workRoots remain in-memory discovery rows.
- Route gates, resource projection, and activation actions must agree for any
  registered workRoot id regardless of whether the row was reached through the
  opened candidate path or linked-worktree discovery.

Add a regression test where both primary and linked workRoots are opened, one is
offline, and the live resource projection agrees with backend route gating
regardless of path ordering.
