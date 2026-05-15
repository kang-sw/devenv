---
title: ws web dashboard local workspace discovery
parent: 260515-epic-ws-web-dashboard-first-visible-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260515-epic-ws-web-dashboard-first-visible-substrate: coordinating first visible substrate epic
  260516-feat-ws-web-resource-view-model-contract: required view-model and workRoot contract
  260514-research-ws-web-dashboard-direction: source research for discovery and root picker boundaries
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
---

# ws web dashboard local workspace discovery

## Background

The dashboard needs real local project discovery after the mock resource
contract exists. Discovery should connect physical directories to the
workspace/workRoot view model without making the browser a generic file
manager or exposing host paths as public URL identity.

## Decisions

- Discover workspaces mechanically from daemon-owned state and filesystem/Git
  facts. Do not introduce user-created logical workspace categories.
- Treat a workRoot as the physical directory used for open, spawn, and run
  operations.
- Preserve workRoot identity across kind changes, such as a plain directory
  becoming Git-backed or Git metadata disappearing.
- Re-detect workRoot kind through manual refresh and opportunistic refresh when
  selecting, opening, or spawning from a workRoot.
- Keep the explorer as a root picker. It may create an empty folder as a narrow
  new-workRoot affordance, but it must not expose delete, rename, move, copy,
  or generic recursive folder deletion.
- Defer broad filesystem watcher behavior until visible roots and watcher
  scope can be constrained.

## Phases

### Phase 1: Local workRoot discovery provider

Implement a live local provider that can identify plain directories, Git
primary roots, and linked Git worktrees and map them into the resource
view-model contract. The provider should report online, offline, moved, or
inaccessible states without dropping recent context prematurely.

Success criteria:

- The live provider can populate the same workRoot fields used by mock
  fixtures.
- Git primary roots and linked worktrees share core workRoot behavior while
  preserving repository-role metadata.
- Manual and opportunistic refresh paths update kind/status without requiring a
  broad watcher.

### Phase 2: Root picker and add-empty-folder affordance

Add the backend support needed for a cross-platform root picker that lists
server filesystem locations as open candidates and lets the owner open a
directory even when it is not a Git repository. Include only the narrow
`Create empty folder` operation needed to create a new workRoot candidate.

Success criteria:

- The owner can open an existing plain directory or Git-backed directory into
  the dashboard model.
- The owner can create an empty folder candidate through the picker.
- Destructive or broad file-manager operations remain unavailable.
