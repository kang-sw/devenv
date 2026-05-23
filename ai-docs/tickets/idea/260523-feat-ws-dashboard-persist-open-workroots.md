---
title: Persist dashboard open workRoots across daemon restarts
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-bug-ws-dashboard-dev-run-ctrl-c-shutdown: dogfood now encourages restarting the local server frequently
  260523-feat-ws-dashboard-linked-worktree-discovery: adjacent resource-discovery persistence concern
related-mental-model:
  - ws-web-dashboard
---

# Persist dashboard open workRoots across daemon restarts

## Background

Dogfood feedback found that repeatedly starting and stopping
`ws-dashboard/dev.sh run` is inconvenient because the dashboard returns to an
empty live resource tree after every daemon restart. The current implementation
keeps `OpenedWorkRoots` as an in-memory map in daemon state; browser workbench
arrangement is also presentation state held in React state. That matches the
early substrate but makes normal development restarts lose useful context.

The first persistence step should restore the owner's recent/open workRoot list
after daemon restart. This is different from keeping live terminal PTYs alive
across daemon process death; terminal processes remain daemon-owned live
sessions and cannot survive that process boundary without a separate supervisor
design.

## Discussion

Likely first behavior:

- Persist opened workRoot paths in a daemon-owned local state file below the
  ws-dashboard/cache/config area, not in browser localStorage alone.
- On daemon startup, seed `OpenedWorkRoots` from that state so
  `GET /api/dashboard/resources` shows the previous roots before the user opens
  one manually.
- Re-run normal discovery at load time so moved, offline, inaccessible,
  primary-root, and linked-worktree states stay honest.
- Keep pairing/session cookies ephemeral; persistence should remember owner
  workspace context, not bypass owner authentication.
- Leave terminal process survival, Activity acknowledgement state, and exact
  workbench pane layout as follow-up decisions unless this ticket is promoted
  with a broader restore scope.

Open questions:

- Should closed workRoots be removed from persisted state immediately, and do we
  need an explicit close/remove affordance before persistence ships?
- Should browser-side workbench arrangement be persisted per workRoot in the
  same daemon state file, browser localStorage, or a later profile/settings
  store?
- What retention limit should apply to stale/offline remembered roots?
