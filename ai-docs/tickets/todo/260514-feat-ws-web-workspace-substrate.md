---
title: ws web workspace substrate
parent: 260514-epic-ws-web-dashboard-mvp
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
---

# ws web workspace substrate

## Background

The dashboard needs a stable workspace model before terminals, agents, and
editors can share context. The workspace layer should discover host folders and
Git worktrees while preserving server/project/worktree/session boundaries.

Workspace APIs should use opaque workspace ids under explicit server ids. Host
paths remain daemon-owned metadata, not URL identifiers.

Detailed picker UX and persistence rules need follow-up discussion before this
ticket is promoted to `ready/`.

## Phases

### Phase 1: Add host folder discovery

Expose a daemon API for browsing allowed host folders and selecting a workspace
root without giving the browser direct filesystem authority.

### Phase 2: Add Git root and worktree discovery

Detect Git repository roots, common roots, linked worktrees, worktree labels,
and invalid-root errors in a form the frontend can render.

### Phase 3: Add workspace state model

Persist recent workspaces, selected workspace state, opaque workspace ids, and
per-workspace UI state keys without leaking ws runtime root or harness state
across projects.

### Phase 4: Verify workspace boundary behavior

Add tests or smoke checks for non-Git folders, linked worktrees, moved folders,
permission errors, and multiple open workspaces.
