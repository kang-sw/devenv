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
plans:
  phase-1: 2026-05/16-260516-feat-ws-web-workroot-file-navigation
  phase-2: 2026-05/16-260516-feat-ws-web-workroot-file-navigation-phase-2
related-mental-model:
  - ws-web-dashboard
completed: 2026-05-16
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

### Result (b8801c1d) - 2026-05-16

Implemented an owner-authenticated daemon listing API for opened workRoots. The
route lists one directory level by opaque `workRootId`, resolves host paths only
through daemon-owned opened-workRoot state, rejects traversal and unknown roots,
and keeps listing read-only without exposing absolute host paths in the response
contract.

Verification covered formatting, focused route tests, the full daemon crate test
suite, workspace check, and delegated correctness/fit/test review.

### Phase 2: Left-Nav File Explorer Draft

Render the selected workRoot's file hierarchy in the lower left navigation
area. The draft should support expand/collapse or equivalent navigation,
explicit refresh, loading/error/empty states, and selection. It should stay
visually subordinate to the server/workspace/workRoot navigation while still
being usable for common project browsing.

Clicking a readable text file may call into the read-only text pane ticket once
that route exists. Until then, the explorer may expose a disabled or stubbed
open command without pretending editing works.

### Result (a9936895) - 2026-05-16

Implemented the lower-left selected-workRoot file explorer draft in the
frontend. The explorer consumes the Phase 1 listing API by opaque `workRootId`
and relative path, keeps server/workspace/workRoot identity above it, supports
root loading, expand/collapse, refresh, loading/error/empty states, and exposes
pending disabled file-open affordances without adding editing behavior.

Verification covered frontend route/workbench/workRoot-files tests, production
build, delegated correctness/fit/test review, and a follow-up test-coverage
fix for listing error handling plus explorer load-decision helpers.
