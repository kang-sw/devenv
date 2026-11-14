---
title: "Isolate dashboard file explorer instances"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261110-feat-ws-dashboard-file-preview: "current read-only Files-tab and preview behavior to preserve"
---

# Isolate dashboard file explorer instances

## Background

The current dashboard shares one `#files` instance between the conversation
surface and toolbar references (`dashboard.js` around the `#files` setup;
`cards.js` around the conversation and toolbar references). The reusable
explorer requires isolated instances so one card's state cannot overwrite
another card's state. This is the first technical slice of the approved file
explorer epic.

## Decisions

- Isolate explorer instance ownership while preserving the current user-visible
  behavior and security boundary.
- Preserve the existing normal Files guarded roots and read-only filesystem
  behavior. This refactor does not widen roots, add filesystem mutation, or
  turn linked-file capabilities into browser authority. The separately approved
  workspace picker is local-viewer-only and directories-only; this refactor
  leaves that future picker consumer's boundary intact but does not implement
  the picker.
- Keep browser-owned state separate per instance. Do not introduce a daemon or
  shared persistence store for this refactor.
- Compatibility tier: T0 for this browser-instance refactor. It has no
  protocol, on-disk shared-store, or root-policy change.

## Constraints

- The current Files tab and toolbar entry points must continue to work while
  instances become independent.
- The refactor must not decide the later explorer's unconfirmed UI grouping,
  rendering mechanism, storage schema, endpoint shape, or dependency choice.
- No future backend compatibility tier is assigned here. Any backend proposal
  must be additive when necessary, with semantics and authorization settled
  separately before classification; no existing root or API semantic rewrite is
  authorized.

## Prior Art

- Current singleton references: `dashboard.js` around the `#files` setup and
  `cards.js` around the conversation and toolbar references.
- Current browser card state: `dashboard.js` around its storage and card-state
  helpers.
- Existing read-only root and file behavior: the `/fs` list, preview, raw, find,
  and search surfaces. Browse-root validation is discoverable in `fs_routes.go`
  through `browseRootParam` and `containedDir`; normal Files root guards remain
  unchanged. The separately approved workspace picker is local-viewer-only and
  directories-only, not blanket file-content root widening. Host-action and
  capability checks remain in `fs_act_route.go`, with narrow outside-linked-file
  grants in `fs_outside.go`; do not add new symlink permissions.

## Phases

### Phase 1: Isolate explorer instances without regression

Separate explorer instance ownership and state so multiple instances can
coexist without cross-talk, while retaining the current Files-tab and toolbar
behavior, read-only operations, normal Files root guards, and existing security
checks. Do not implement the separately approved workspace picker or add its
picker UI here; do not add the later feature slice's view modes, search
expansion, preview formats, or planned child scope here.

Verification should demonstrate independent instance state and preservation of
current entry points and access checks. Product source changes, tests, and
runtime verification belong to the later implementation run; this draft
records no implementation or verification result.
