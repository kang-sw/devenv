---
title: ws web dashboard workRoot file navigation
parent: 260516-epic-ws-web-dashboard-workroot-io-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260516-epic-ws-web-dashboard-workroot-io-substrate: containing milestone
  260516-feat-ws-web-local-workspace-discovery: workRoot discovery prerequisite
spec:
  - 260516-ws-web-dashboard-workroot-file-listing-api
  - 260516-ws-web-dashboard-workroot-file-explorer
related-mental-model:
  - ws-web-dashboard
---

# ws web dashboard workRoot file navigation

## Background

The dashboard can select workRoots and show a workbench shell, but the opened
workRoot is not yet navigable. The next usable slice needs a small
workRoot-local file explorer that helps the owner inspect project files without
turning the browser into a general-purpose file manager.

## Decisions

- Scope the explorer to the selected workRoot.
- Place the first file explorer draft in the lower portion of the left
  navigation area so server/workspace/workRoot identity remains visible above
  it.
- Expose listing and refresh behavior only. Do not add delete, rename, move,
  copy, chmod, recursive folder deletion, or broad filesystem management.
- Preserve host paths as authenticated daemon-private data. Browser routes and
  stable resource ids must not use raw filesystem paths as identity.
- Treat Git/plain-directory kind changes as provider state that refresh can
  detect, not as a separate user-authored workspace model.

## Phases

### Phase 1: Authenticated WorkRoot Listing API

Add an authenticated daemon API for listing directories below a selected
workRoot. The response should expose enough metadata for a browser tree or
compact file list: names, file/directory kind, basic status, and previewability
or read eligibility where cheap. The API must stay rooted below the workRoot,
reject traversal, and surface inaccessible or unreadable paths honestly.

This phase should keep filesystem mutation out of scope.

### Phase 2: Left-Nav File Explorer Draft

Render the selected workRoot's file hierarchy in the lower left navigation
area. The draft should support expand/collapse or equivalent navigation,
explicit refresh, loading/error/empty states, and selection. It should stay
visually subordinate to the server/workspace/workRoot navigation while still
being usable for common project browsing.

Clicking a readable text file may call into the read-only text pane ticket once
that route exists. Until then, the explorer may expose a disabled or stubbed
open command without pretending editing works.
